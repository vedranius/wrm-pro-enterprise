// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── LIVE STATUS (up / down) ──────────────────────────
//
// A background monitor checks saved connections every status_interval_seconds with a TCP
// connect to their host:port (SSH, FTP or web port). Like Nagios check_ssh, an SSH check
// reads the server banner (SSH-2.0-OpenSSH_9.6 …) and answers with its own before closing,
// and an FTP check reads the 220 greeting and sends QUIT, so servers log a clean
// disconnect. Each distinct host:port is checked once per round, however many connections
// and users point to it; a failed check is retried once before a host is reported down.
//
// Connections behind a jump host are usually not reachable from WRM directly. By default
// they show the state of their jump host ("via bastion"); with status_jump_checks the
// monitor logs in to the jump host and checks the port through it (one SSH connection
// per jump host and round). "Check now" always checks through the jump hosts.
//
// Browsers get changes through /ws/events ({"type":"status_changed"}) and read the states
// with GET /api/status. Connections can opt out ("Monitor status" in the connection).

type hostStatus struct {
	State     string `json:"state"` // up | down | unknown
	LatencyMs int    `json:"latency_ms"`
	Since     string `json:"since"`      // when the state last changed
	CheckedAt string `json:"checked_at"` // last check
	Error     string `json:"error,omitempty"`
	Banner    string `json:"banner,omitempty"` // SSH server version / FTP greeting
	Via       string `json:"via,omitempty"`    // checked through, or shown for, this jump host
	ViaState  string `json:"via_state,omitempty"`
}

type statusMonitor struct {
	mu       sync.Mutex
	hosts    map[string]*hostStatus // "tcp|host:port" or "via<jumpID>|host:port"
	conns    map[int]string         // connection id → host key
	jumpOf   map[int]int            // connection id → its jump host (when not checked through it)
	lastConn map[int]string         // last reported state per connection (change detection)
	owners   map[int]int            // connection id → owner
	names    map[int]string         // connection id → name (for "via …")
	downAt   map[int]time.Time      // when a connection went down (for "up again after …")
	trigger  chan struct{}
	running  sync.Mutex // one round at a time
}

var statusMon = &statusMonitor{hosts: map[string]*hostStatus{}, conns: map[int]string{}, jumpOf: map[int]int{},
	lastConn: map[int]string{}, owners: map[int]int{}, names: map[int]string{}, downAt: map[int]time.Time{}, trigger: make(chan struct{}, 1)}

const (
	statusDialTimeout   = 4 * time.Second
	statusBannerTimeout = 3 * time.Second
	statusWorkers       = 32
)

func (m *statusMonitor) run() {
	time.Sleep(3 * time.Second) // let the server start first
	for {
		if settingBool("status_enabled") {
			m.round()
		}
		iv := settingInt("status_interval_seconds")
		if iv < 15 {
			iv = 15
		}
		select {
		case <-time.After(time.Duration(iv) * time.Second):
		case <-m.trigger:
		}
	}
}

// poke starts a round soon (e.g. after connections changed).
func (m *statusMonitor) poke() {
	select {
	case m.trigger <- struct{}{}:
	default:
	}
}

type statusTarget struct {
	key   string
	addr  string
	proto string
	jump  int // check through this jump connection (0 = directly)
	proxy int // and then through this proxy (0 = none)
	user  int // owner of the connection (proxy access)
}

type monConn struct {
	id, userID, jump, proxy int
	name, proto, host       string
}

func loadMonitoredConns() []monConn {
	rows, err := db.Query(`SELECT c.id, COALESCE(c.user_id,0), c.name, c.protocol, c.host, ` + effJumpSQL + `, ` + effProxySQL + ` FROM connections c ` + folderJoin + `
		WHERE COALESCE(c.monitor,1)=1 AND c.protocol<>'SERIAL'`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []monConn
	for rows.Next() {
		var c monConn
		if rows.Scan(&c.id, &c.userID, &c.name, &c.proto, &c.host, &c.jump, &c.proxy) == nil && c.host != "" {
			out = append(out, c)
		}
	}
	return out
}

func statusKey(c monConn, throughJumps bool) (statusTarget, bool) {
	addr := ensurePort(c.host, c.proto)
	px := ""
	if c.proxy > 0 {
		px = "px" + strconv.Itoa(c.proxy) + "u" + strconv.Itoa(c.userID) + "|" // proxy access is per user
	}
	if c.jump <= 0 {
		if px != "" {
			return statusTarget{key: px + addr, addr: addr, proto: c.proto, proxy: c.proxy, user: c.userID}, true
		}
		return statusTarget{key: "tcp|" + addr, addr: addr, proto: c.proto}, true
	}
	if !throughJumps {
		return statusTarget{}, false
	}
	return statusTarget{key: "via" + strconv.Itoa(c.jump) + "|" + px + addr, addr: addr, proto: c.proto, jump: c.jump, proxy: c.proxy, user: c.userID}, true
}

// round checks every monitored connection once.
func (m *statusMonitor) round() {
	m.running.Lock()
	defer m.running.Unlock()
	through := settingBool("status_jump_checks")
	conns := loadMonitoredConns()
	targets := map[string]statusTarget{}
	connKeys, jumpOf, owners, names, hosts := map[int]string{}, map[int]int{}, map[int]int{}, map[int]string{}, map[int]string{}
	for _, c := range conns {
		owners[c.id] = c.userID
		names[c.id] = c.name
		hosts[c.id] = c.host
		if t, ok := statusKey(c, through); ok {
			targets[t.key] = t
			connKeys[c.id] = t.key
		} else {
			jumpOf[c.id] = c.jump
		}
	}
	results := m.probeAll(targets)
	m.mu.Lock()
	now := time.Now().UTC().Format(time.RFC3339)
	for key, r := range results {
		old := m.hosts[key]
		if old == nil || old.State != r.State {
			r.Since = now
			if old != nil && old.State != "" {
				if r.State == "down" {
					log.Printf("Status: %s is down (%s)", strings.SplitN(key, "|", 2)[1], r.Error)
				} else if r.State == "up" && old.State == "down" {
					log.Printf("Status: %s is up again", strings.SplitN(key, "|", 2)[1])
				}
			}
		} else {
			r.Since = old.Since
		}
		r.CheckedAt = now
		m.hosts[key] = r
	}
	for key := range m.hosts { // forget hosts no connection points to any more
		if _, ok := targets[key]; !ok {
			delete(m.hosts, key)
		}
	}
	m.conns, m.jumpOf, m.owners, m.names = connKeys, jumpOf, owners, names
	changedUsers := map[int]bool{}
	type transition struct {
		uid, id        int
		state, errText string
		downFor        time.Duration
	}
	var transitions []transition
	for id, uid := range owners {
		cs := m.connStatusLocked(id, 0)
		st := cs.State
		prev, seen := m.lastConn[id]
		if prev != st {
			if seen {
				changedUsers[uid] = true
				// Went down (from up, or from unknown behind a jump host), or came back from a
				// reported down: up again, or behind a jump host that is up again.
				_, reported := m.downAt[id]
				recovered := st == "up" || (st == "unknown" && cs.ViaState == "up")
				if st == "down" && !reported {
					m.downAt[id] = time.Now()
					transitions = append(transitions, transition{uid: uid, id: id, state: "down", errText: cs.Error})
				} else if recovered && reported {
					d := time.Since(m.downAt[id])
					delete(m.downAt, id)
					transitions = append(transitions, transition{uid: uid, id: id, state: "up", downFor: d})
				}
			}
			m.lastConn[id] = st
		}
	}
	for id := range m.lastConn {
		if _, ok := owners[id]; !ok {
			delete(m.lastConn, id)
			delete(m.downAt, id)
		}
	}
	m.mu.Unlock()
	for uid := range changedUsers {
		hub.sendTo(uid, []byte(`{"type":"status_changed"}`))
	}
	for _, tr := range transitions {
		notifyStatusChange(tr.uid, names[tr.id], hosts[tr.id], tr.state, tr.errText, tr.downFor)
	}
}

// probeAll checks targets with a bounded number of workers; targets behind the same jump
// host share one SSH connection.
func (m *statusMonitor) probeAll(targets map[string]statusTarget) map[string]*hostStatus {
	out := map[string]*hostStatus{}
	var mu sync.Mutex
	put := func(k string, r *hostStatus) {
		mu.Lock()
		out[k] = r
		mu.Unlock()
	}
	direct := make(chan statusTarget)
	byJump := map[int][]statusTarget{}
	for _, t := range targets {
		if t.jump > 0 {
			byJump[t.jump] = append(byJump[t.jump], t)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < statusWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range direct {
				put(t.key, probeWithRetry(t, statusDialer(t, directDial, false)))
			}
		}()
	}
	for jumpID, list := range byJump {
		wg.Add(1)
		go func(jumpID int, list []statusTarget) {
			defer wg.Done()
			for k, r := range probeViaJump(jumpID, list) {
				put(k, r)
			}
		}(jumpID, list)
	}
	for _, t := range targets {
		if t.jump == 0 {
			direct <- t
		}
	}
	close(direct)
	wg.Wait()
	return out
}

func probeViaJump(jumpID int, list []statusTarget) map[string]*hostStatus {
	out := map[string]*hostStatus{}
	jc, err := loadConnection(jumpID)
	if err != nil {
		for _, t := range list {
			out[t.key] = &hostStatus{State: "unknown", Error: "jump host not found"}
		}
		return out
	}
	cl, err := dialSSH(jc, nil)
	if err != nil {
		for _, t := range list {
			out[t.key] = &hostStatus{State: "unknown", Via: jc.Name, ViaState: "down", Error: truncateStr("jump host "+jc.Name+": "+err.Error(), 200)}
		}
		return out
	}
	defer cl.Close()
	for _, t := range list {
		r := probeWithRetry(t, statusDialer(t, cl.Dial, true))
		r.Via = jc.Name
		out[t.key] = r
	}
	return out
}

type dialFunc func(network, addr string) (net.Conn, error)

// statusDialer reaches a target through its proxy when it has one.
func statusDialer(t statusTarget, base dialFunc, behindJump bool) dialFunc {
	if t.proxy <= 0 {
		return base
	}
	px := t.proxy
	rp, err := proxyOf(Connection{UserID: t.user, ProxyID: &px, Name: t.addr}, behindJump)
	return func(network, addr string) (net.Conn, error) {
		if err != nil {
			return nil, statusUnknown{err.Error()} // the route cannot be used: not a "down" server
		}
		if rp.p.Kind == "wrm_tunnel" {
			// Never start a WRM tunnel for a status check, and do not keep it alive: check
			// over the SSH connection of a running tunnel, else the state is unknown.
			var tr *tunnelRun
			if rp.p.TunnelID != nil {
				tr = tunnelMgr.get("t" + strconv.Itoa(*rp.p.TunnelID))
			}
			if tr == nil || tr.currentClient() == nil {
				return nil, statusUnknown{"the WRM SOCKS tunnel of the proxy is not running"}
			}
			return tr.dialRemote(addr)
		}
		return rp.dial(base, addr)
	}
}

// statusUnknown: the check could not be made (no "down" state, no notification).
type statusUnknown struct{ msg string }

func (e statusUnknown) Error() string { return e.msg }

func probeWithRetry(t statusTarget, dial dialFunc) *hostStatus {
	r := probe(t, dial)
	if r.State == "down" {
		time.Sleep(time.Second)
		r = probe(t, dial)
	}
	return r
}

// probe connects to t.addr and, for SSH and FTP, reads the greeting.
func probe(t statusTarget, dial dialFunc) *hostStatus {
	start := time.Now()
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := dial("tcp", t.addr)
		ch <- res{c, err}
	}()
	var conn net.Conn
	select {
	case r := <-ch:
		if r.err != nil {
			if su, ok := r.err.(statusUnknown); ok {
				return &hostStatus{State: "unknown", Error: truncateStr(su.msg, 200)}
			}
			return &hostStatus{State: "down", Error: shortNetError(r.err)}
		}
		conn = r.c
	case <-time.After(statusDialTimeout):
		go func() {
			if r := <-ch; r.c != nil {
				r.c.Close()
			}
		}()
		return &hostStatus{State: "down", Error: "timeout"}
	}
	defer conn.Close()
	st := &hostStatus{State: "up", LatencyMs: int(time.Since(start).Milliseconds())}
	switch strings.ToUpper(t.proto) {
	case "SSH", "SFTP", "":
		conn.SetDeadline(time.Now().Add(statusBannerTimeout))
		line, err := bufio.NewReaderSize(conn, 512).ReadString('\n')
		if strings.HasPrefix(line, "SSH-") {
			st.Banner = truncateStr(strings.TrimSpace(line), 120)
			conn.Write([]byte("SSH-2.0-WRM_status_check\r\n")) // like check_ssh: identify, then close
		} else if err != nil && line == "" {
			st.Error = "no SSH banner"
		}
	case "FTP", "FTPS":
		conn.SetDeadline(time.Now().Add(statusBannerTimeout))
		line, _ := bufio.NewReaderSize(conn, 512).ReadString('\n')
		if strings.HasPrefix(line, "220") {
			st.Banner = truncateStr(strings.TrimSpace(line), 120)
			conn.Write([]byte("QUIT\r\n"))
		}
	}
	return st
}

func shortNetError(err error) string {
	if pe, ok := err.(*proxyError); ok {
		return truncateStr(pe.Error(), 200) // says whether the proxy or the target failed
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "connection refused"):
		return "connection refused"
	case strings.Contains(s, "no such host"):
		return "host name not found"
	case strings.Contains(s, "network is unreachable"):
		return "network unreachable"
	case strings.Contains(s, "no route to host"):
		return "no route to host"
	case strings.Contains(s, "i/o timeout"), strings.Contains(s, "timeout"):
		return "timeout"
	case strings.Contains(s, "administratively prohibited"):
		return "refused by the jump host (forwarding not allowed)"
	}
	return truncateStr(s, 160)
}

// connStatusLocked returns the state shown for a connection. depth limits jump host lookups.
func (m *statusMonitor) connStatusLocked(id, depth int) hostStatus {
	if key, ok := m.conns[id]; ok {
		if h := m.hosts[key]; h != nil {
			return *h
		}
		return hostStatus{State: "unknown"}
	}
	jump, ok := m.jumpOf[id]
	if !ok || depth > maxJumpDepth {
		return hostStatus{State: "unknown"}
	}
	js := m.connStatusLocked(jump, depth+1)
	name := m.names[jump]
	if name == "" {
		name = "jump host"
	}
	h := hostStatus{State: "unknown", Via: name, ViaState: js.State, CheckedAt: js.CheckedAt, Since: js.Since}
	if js.State == "down" {
		h.State, h.Error = "down", "jump host "+name+" is down"
	}
	return h
}

func (m *statusMonitor) snapshot(ids []int) map[string]hostStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]hostStatus{}
	for _, id := range ids {
		if _, monitored := m.owners[id]; !monitored {
			continue
		}
		out[strconv.Itoa(id)] = m.connStatusLocked(id, 0)
	}
	return out
}

// checkNow checks the given connections immediately, through their jump hosts.
func (m *statusMonitor) checkNow(cs []Connection) map[string]hostStatus {
	out := map[string]hostStatus{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for _, c := range cs {
		wg.Add(1)
		go func(c Connection) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			addr := ensurePort(c.Host, c.Protocol)
			t := statusTarget{addr: addr, proto: c.Protocol}
			var r *hostStatus
			if needsRoute(c) {
				if rt, err := openTargetRoute(c); err != nil {
					r = &hostStatus{State: "unknown", Via: jumpPath(c), ViaState: "down", Error: truncateStr(err.Error(), 200)}
				} else {
					r = probe(t, rt.Dial)
					r.Via = jumpPath(c)
					rt.Close()
				}
			} else {
				r = probeWithRetry(t, net.Dial)
			}
			r.CheckedAt = time.Now().UTC().Format(time.RFC3339)
			mu.Lock()
			out[strconv.Itoa(c.ID)] = *r
			mu.Unlock()
		}(c)
	}
	wg.Wait()
	// Direct results also refresh the shared monitor state.
	m.mu.Lock()
	for _, c := range cs {
		r, ok := out[strconv.Itoa(c.ID)]
		if !ok || needsRoute(c) {
			continue
		}
		key := "tcp|" + ensurePort(c.Host, c.Protocol)
		if old := m.hosts[key]; old != nil {
			if old.State == r.State {
				r.Since = old.Since
			} else {
				r.Since = r.CheckedAt
			}
			cp := r
			m.hosts[key] = &cp
			out[strconv.Itoa(c.ID)] = r
		}
	}
	m.mu.Unlock()
	return out
}

var statusCheckLimiter = struct {
	sync.Mutex
	last map[int]time.Time
}{last: map[int]time.Time{}}

// GET  /api/status            states of my monitored connections
// POST /api/status/check      {"ids":[…]} check now (also through jump hosts)
func apiStatusHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	switch {
	case r.URL.Path == "/api/status" && r.Method == http.MethodGet:
		ids := []int{}
		for _, c := range loadUserConnections(userID) {
			ids = append(ids, c.ID)
		}
		jsonOK(w, map[string]interface{}{"enabled": settingBool("status_enabled"), "interval": settingInt("status_interval_seconds"),
			"jump_checks": settingBool("status_jump_checks"), "connections": statusMon.snapshot(ids)})
	case r.URL.Path == "/api/status/check" && r.Method == http.MethodPost:
		var in struct {
			IDs []int `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.IDs) == 0 || len(in.IDs) > 500 {
			jsonError(w, "ids: 1–500 connection ids", 400)
			return
		}
		statusCheckLimiter.Lock()
		if time.Since(statusCheckLimiter.last[userID]) < 3*time.Second {
			statusCheckLimiter.Unlock()
			jsonError(w, "Please wait a moment before checking again", 429)
			return
		}
		statusCheckLimiter.last[userID] = time.Now()
		statusCheckLimiter.Unlock()
		var cs []Connection
		for _, id := range in.IDs {
			if c, err := loadConnection(id); err == nil && c.UserID == userID {
				cs = append(cs, c)
			}
		}
		jsonOK(w, map[string]interface{}{"connections": statusMon.checkNow(cs)})
	default:
		jsonError(w, "Not found", 404)
	}
}
