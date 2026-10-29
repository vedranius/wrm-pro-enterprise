// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── SSH TUNNELS (port forwarding) ───────────────────
//
// A connection can carry any number of tunnels. They work like OpenSSH -L / -R / -D and
// like the SSH tunnels of PuTTY and mRemoteNG:
//
//   local    -L  a port on the WRM machine  → (SSH) → target host:port as seen from the server
//                e.g. 127.0.0.1:2443 → 10.0.0.5:443 opens an internal web interface
//   remote   -R  a port on the SSH server   → (SSH) → target host:port as seen from WRM
//   dynamic  -D  a SOCKS5 proxy on the WRM machine; every destination is reached via the server
//
// Tunnels go through the connection's jump hosts too. Each running tunnel keeps its own
// SSH connection, checks it every 30 seconds and reconnects automatically. Start modes:
// manual, "connect" (while a terminal of the connection is open) and "always" (from the
// start of WRM). Listening on other addresses than loopback needs an administrator or the
// tunnel_bind_any policy, remote forwards need the tunnel_remote_forward policy. Every
// start, stop and failure is audited with the transferred bytes.

type tunnelDef struct {
	ID         int    `json:"id"`
	ConnID     int    `json:"conn_id"`
	UserID     int    `json:"-"`
	Name       string `json:"name"`
	Kind       string `json:"kind"` // local | remote | dynamic
	BindHost   string `json:"bind_host"`
	BindPort   int    `json:"bind_port"` // 0 = choose a free port automatically
	TargetHost string `json:"target_host"`
	TargetPort int    `json:"target_port"`
	OpenScheme string `json:"open_scheme"` // http | https | "" — "Open in browser" button
	OpenPath   string `json:"open_path"`
	StartMode  string `json:"start_mode"` // manual | connect | always
	Sort       int    `json:"sort"`
}

const tunnelCols = `id, conn_id, user_id, name, kind, bind_host, bind_port, target_host, target_port, open_scheme, open_path, start_mode, sort`

func scanTunnelDef(sc interface{ Scan(...interface{}) error }) (tunnelDef, error) {
	var d tunnelDef
	err := sc.Scan(&d.ID, &d.ConnID, &d.UserID, &d.Name, &d.Kind, &d.BindHost, &d.BindPort, &d.TargetHost, &d.TargetPort,
		&d.OpenScheme, &d.OpenPath, &d.StartMode, &d.Sort)
	return d, err
}

func loadTunnelDefs(where string, args ...interface{}) []tunnelDef {
	out := []tunnelDef{}
	rows, err := db.Query(`SELECT `+tunnelCols+` FROM connection_tunnels WHERE `+where+` ORDER BY conn_id, sort, id`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		if d, err := scanTunnelDef(rows); err == nil {
			out = append(out, d)
		}
	}
	return out
}

func loadTunnelDef(id int) (tunnelDef, error) {
	return scanTunnelDef(db.QueryRow(`SELECT `+tunnelCols+` FROM connection_tunnels WHERE id=?`, id))
}

func copyTunnelDefs(fromConn, toConn int) {
	db.Exec(`INSERT INTO connection_tunnels (conn_id, user_id, name, kind, bind_host, bind_port, target_host, target_port, open_scheme, open_path, start_mode, sort, created_at)
		SELECT ?, user_id, name, kind, bind_host, bind_port, target_host, target_port, open_scheme, open_path, 'manual', sort, ? FROM connection_tunnels WHERE conn_id=?`,
		toConn, time.Now().UTC().Format(time.RFC3339), fromConn)
}

// ── policies ──

func tunnelsAllowedFor(userID int) error {
	if !settingBool("tunnels_enabled") {
		return fmt.Errorf("SSH tunnels are turned off by the administrator")
	}
	if v := getSetting("tunnel_users"); !policyAllows(userID, v) {
		return fmt.Errorf("SSH tunnels are limited to %s", policyLimitText(v))
	}
	return nil
}

func isLoopbackHost(h string) bool {
	h = strings.Trim(strings.TrimSpace(h), "[]")
	if h == "" || strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func validHostName(h string) bool {
	if h == "" || len(h) > 253 || strings.ContainsAny(h, " /\\@\t\r\n") {
		return false
	}
	return true
}

// validateTunnelDef normalises a tunnel and checks it against the policies.
func validateTunnelDef(d *tunnelDef, userID int) error {
	d.Name = truncateStr(strings.TrimSpace(d.Name), 80)
	d.Kind = strings.ToLower(strings.TrimSpace(d.Kind))
	d.BindHost = strings.Trim(strings.TrimSpace(d.BindHost), "[]")
	d.TargetHost = strings.Trim(strings.TrimSpace(d.TargetHost), "[]")
	d.OpenScheme = strings.ToLower(strings.TrimSpace(d.OpenScheme))
	d.OpenPath = strings.TrimSpace(d.OpenPath)
	d.StartMode = strings.ToLower(strings.TrimSpace(d.StartMode))
	switch d.Kind {
	case "local", "remote", "dynamic":
	default:
		return fmt.Errorf("unknown tunnel type %q", d.Kind)
	}
	if d.BindHost == "" || d.BindHost == "localhost" {
		d.BindHost = "127.0.0.1"
	}
	if d.BindHost == "*" {
		d.BindHost = "0.0.0.0"
	}
	if net.ParseIP(d.BindHost) == nil {
		return fmt.Errorf("listen address must be an IP address (127.0.0.1, 0.0.0.0, …)")
	}
	if d.BindPort < 0 || d.BindPort > 65535 {
		return fmt.Errorf("invalid listen port %d", d.BindPort)
	}
	if d.Kind != "remote" {
		if d.BindPort != 0 && d.BindPort < 1024 {
			return fmt.Errorf("listen port must be 1024–65535 (or 0 for automatic)")
		}
		if !isLoopbackHost(d.BindHost) && !isAdminUser(userID) && !settingBool("tunnel_bind_any") {
			return fmt.Errorf("only administrators may listen on %s (others: 127.0.0.1)", d.BindHost)
		}
	} else {
		switch v := getSetting("tunnel_remote_forward"); {
		case v == "off":
			return fmt.Errorf("remote port forwarding is turned off by the administrator")
		case !policyAllows(userID, v):
			return fmt.Errorf("remote port forwarding is limited to %s", policyLimitText(v))
		}
	}
	if d.Kind == "dynamic" {
		d.TargetHost, d.TargetPort = "", 0
	} else {
		if !validHostName(d.TargetHost) {
			return fmt.Errorf("invalid target host")
		}
		if d.TargetPort < 1 || d.TargetPort > 65535 {
			return fmt.Errorf("invalid target port")
		}
	}
	if d.Kind != "local" || (d.OpenScheme != "http" && d.OpenScheme != "https") {
		d.OpenScheme, d.OpenPath = "", ""
	}
	if d.OpenPath != "" && !strings.HasPrefix(d.OpenPath, "/") {
		d.OpenPath = "/" + d.OpenPath
	}
	if len(d.OpenPath) > 500 || strings.ContainsAny(d.OpenPath, " \t\r\n\\") {
		return fmt.Errorf("invalid path")
	}
	switch d.StartMode {
	case "", "manual":
		d.StartMode = "manual"
	case "connect", "always":
	default:
		return fmt.Errorf("unknown start mode %q", d.StartMode)
	}
	if d.Name == "" {
		d.Name = defaultTunnelName(*d)
	}
	return nil
}

func defaultTunnelName(d tunnelDef) string {
	switch d.Kind {
	case "dynamic":
		return "SOCKS proxy"
	case "remote":
		return fmt.Sprintf("Remote %d → %s", d.BindPort, net.JoinHostPort(d.TargetHost, strconv.Itoa(d.TargetPort)))
	}
	return net.JoinHostPort(d.TargetHost, strconv.Itoa(d.TargetPort))
}

func (d tunnelDef) describe() string {
	bind := net.JoinHostPort(d.BindHost, strconv.Itoa(d.BindPort))
	switch d.Kind {
	case "dynamic":
		return "SOCKS5 " + bind
	case "remote":
		return "server " + bind + " → " + net.JoinHostPort(d.TargetHost, strconv.Itoa(d.TargetPort))
	}
	return bind + " → " + net.JoinHostPort(d.TargetHost, strconv.Itoa(d.TargetPort))
}

// ── runtime ──

type tunnelRun struct {
	key         string
	def         tunnelDef
	conn        Connection
	route       string
	ownerID     int
	startedByID int
	startedBy   string
	reason      string // manual | connect | always | web
	ephemeral   bool
	useRoute    bool       // dial the target over shown's route (jump hosts and / or proxy), not over conn's SSH
	forConn     int        // web connection a temporary tunnel was opened for
	shown       Connection // connection shown in lists (the web connection, not the jump host it runs over)
	idleLimit   time.Duration
	startedAt   time.Time
	ip          string

	mu        sync.Mutex
	state     string // starting | up | reconnecting | error | stopped
	lastErr   string
	boundAddr string
	ln        net.Listener
	client    *ssh.Client
	rt        *targetRoute // with useRoute
	stopCh    chan struct{}
	stopped   bool

	active, total     atomic.Int64
	bytesUp, bytesDwn atomic.Int64
	lastActivity      atomic.Int64
}

type tunnelManager struct {
	mu    sync.Mutex
	runs  map[string]*tunnelRun
	holds map[int]int // connection → open terminals (for start mode "connect")
}

var tunnelMgr = &tunnelManager{runs: map[string]*tunnelRun{}, holds: map[int]int{}}

// proxyTunnelIdle: a tunnel started for a proxy stops after this time without traffic.
var proxyTunnelIdle = 30 * time.Minute

// tunnelConnectGrace: "connect" tunnels stop this long after the last terminal closed.
var tunnelConnectGrace = 15 * time.Second

// localListenerOwner returns the owner of a running tunnel that listens on the WRM machine
// on port (0 = none).
func (m *tunnelManager) localListenerOwner(port int) int {
	m.mu.Lock()
	runs := make([]*tunnelRun, 0, len(m.runs))
	for _, t := range m.runs {
		runs = append(runs, t)
	}
	m.mu.Unlock()
	ps := strconv.Itoa(port)
	for _, t := range runs {
		if t.def.Kind == "remote" { // listens on the SSH server
			continue
		}
		t.mu.Lock()
		a := t.boundAddr
		t.mu.Unlock()
		if _, p, err := net.SplitHostPort(a); err == nil && p == ps {
			return t.ownerID
		}
	}
	return 0
}

// guardLoopbackTunnel refuses a connection target that is a loopback port currently held
// by another user's tunnel. Without it, any user could reach that tunnel (and the owner's
// SSH login and network position) through an ordinary connection or a connection test.
func guardLoopbackTunnel(c Connection) error {
	host, portStr, err := net.SplitHostPort(ensurePort(c.Host, c.Protocol))
	if err != nil {
		return nil
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		if !strings.EqualFold(host, "localhost") {
			return nil
		}
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil
	}
	if owner := tunnelMgr.localListenerOwner(port); owner != 0 && owner != c.UserID {
		return fmt.Errorf("port %d on this host belongs to another user's tunnel", port)
	}
	return nil
}

func (m *tunnelManager) get(key string) *tunnelRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[key]
}

func (m *tunnelManager) notify(userID int) {
	if userID > 0 {
		hub.sendTo(userID, []byte(`{"type":"tunnels_changed"}`))
	}
}

func (t *tunnelRun) setState(state, errMsg string) {
	t.mu.Lock()
	t.state, t.lastErr = state, errMsg
	t.mu.Unlock()
	tunnelMgr.notify(t.ownerID)
}

func (t *tunnelRun) touch() { t.lastActivity.Store(time.Now().Unix()) }

func (t *tunnelRun) isStopped() bool {
	select {
	case <-t.stopCh:
		return true
	default:
		return false
	}
}

func (t *tunnelRun) auditRef() auditRef { return auditRef{ConnID: t.conn.ID} }

func (t *tunnelRun) auditDetails() map[string]interface{} {
	d := map[string]interface{}{"tunnel": t.def.Name, "type": t.def.Kind, "listen": t.boundAddr, "mode": t.reason}
	if t.def.Kind != "dynamic" {
		d["target"] = net.JoinHostPort(t.def.TargetHost, strconv.Itoa(t.def.TargetPort))
	}
	if t.route != "" {
		d["route"] = t.route
	}
	return d
}

// start launches a tunnel and waits until it runs (or fails). The tunnel then keeps
// itself alive until it is stopped.
func (m *tunnelManager) start(key string, def tunnelDef, c Connection, startedByID int, reason string, r *http.Request) (*tunnelRun, error) {
	if err := tunnelsAllowedFor(c.UserID); err != nil {
		return nil, err
	}
	if startedByID != c.UserID {
		return nil, fmt.Errorf("only the owner of the connection can start its tunnels")
	}
	if !isSSHProtocol(c) {
		return nil, fmt.Errorf("tunnels need an SSH connection")
	}
	m.mu.Lock()
	if old := m.runs[key]; old != nil {
		m.mu.Unlock()
		return old, nil // already running
	}
	t := &tunnelRun{key: key, def: def, conn: c, route: jumpPath(c), ownerID: c.UserID, startedByID: startedByID,
		startedBy: usernameOf(startedByID), reason: reason, startedAt: time.Now(), state: "starting", stopCh: make(chan struct{})}
	if r != nil {
		t.ip = clientIP(r)
	}
	// The idle limit applies to tunnels started by hand; "connect" tunnels end with their
	// terminal and "always" tunnels are meant to stay up.
	if mins := settingInt("tunnel_idle_minutes"); mins > 0 && reason == "manual" {
		t.idleLimit = time.Duration(mins) * time.Minute
	}
	// A SOCKS tunnel started for a "WRM SOCKS tunnel" proxy stops when it is no longer used.
	if reason == "proxy" && def.StartMode != "always" {
		t.idleLimit = proxyTunnelIdle
	}
	t.touch()
	m.runs[key] = t
	m.mu.Unlock()
	m.notify(t.ownerID)

	if err := t.open(); err != nil {
		t.close()
		m.remove(t)
		t.setState("error", err.Error())
		auditLogRef(r, startedByID, t.startedBy, "tunnel.error", c.Name, mergeMap(t.auditDetails(), map[string]interface{}{"error": truncateStr(err.Error(), 300)}), t.auditRef())
		return nil, err
	}
	t.setState("up", "")
	auditLogRef(r, startedByID, t.startedBy, "tunnel.start", c.Name, t.auditDetails(), t.auditRef())
	go t.supervise()
	return t, nil
}

func mergeMap(a, b map[string]interface{}) map[string]interface{} {
	for k, v := range b {
		a[k] = v
	}
	return a
}

func (m *tunnelManager) remove(t *tunnelRun) {
	m.mu.Lock()
	if m.runs[t.key] == t {
		delete(m.runs, t.key)
	}
	m.mu.Unlock()
	m.notify(t.ownerID)
}

// open connects SSH and starts listening.
func (t *tunnelRun) open() error {
	var ln net.Listener
	if t.def.Kind != "remote" {
		// Listen first, so a busy port is reported at once.
		l, err := net.Listen("tcp", net.JoinHostPort(t.def.BindHost, strconv.Itoa(t.def.BindPort)))
		if err != nil {
			if strings.Contains(err.Error(), "address already in use") || strings.Contains(err.Error(), "Only one usage") {
				return fmt.Errorf("port %d on %s is already in use", t.def.BindPort, t.def.BindHost)
			}
			return err
		}
		ln = l
	}
	client, rt, err := t.connect()
	if err != nil {
		if ln != nil {
			ln.Close()
		}
		return err
	}
	if t.def.Kind == "remote" {
		l, err := client.Listen("tcp", net.JoinHostPort(t.def.BindHost, strconv.Itoa(t.def.BindPort)))
		if err != nil {
			client.Close()
			return fmt.Errorf("the server refused remote port forwarding on %s (AllowTcpForwarding / GatewayPorts in sshd_config?): %v",
				net.JoinHostPort(t.def.BindHost, strconv.Itoa(t.def.BindPort)), err)
		}
		ln = l
	}
	t.mu.Lock()
	t.client, t.rt, t.ln = client, rt, ln
	t.boundAddr = ln.Addr().String()
	if t.def.Kind == "remote" {
		// The server reports the address it listens on; keep the configured host.
		if _, p, err := net.SplitHostPort(ln.Addr().String()); err == nil {
			t.boundAddr = net.JoinHostPort(t.def.BindHost, p)
		}
	}
	t.mu.Unlock()
	go t.acceptLoop(ln)
	return nil
}

func (t *tunnelRun) acceptLoop(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return // listener closed (stop or SSH connection lost)
		}
		t.touch()
		go t.handle(c)
	}
}

func (t *tunnelRun) currentClient() *ssh.Client {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.client
}

// handle forwards one accepted connection.
func (t *tunnelRun) handle(in net.Conn) {
	t.active.Add(1)
	t.total.Add(1)
	defer t.active.Add(-1)
	var out net.Conn
	var err error
	switch t.def.Kind {
	case "local":
		out, err = t.dialRemote(net.JoinHostPort(t.def.TargetHost, strconv.Itoa(t.def.TargetPort)))
	case "remote":
		out, err = net.DialTimeout("tcp", net.JoinHostPort(t.def.TargetHost, strconv.Itoa(t.def.TargetPort)), 15*time.Second)
	case "dynamic":
		out, err = t.socks5(in)
	}
	if err != nil {
		in.Close()
		t.mu.Lock()
		t.lastErr = truncateStr(err.Error(), 300)
		t.mu.Unlock()
		return
	}
	pipeConns(in, out, &t.bytesUp, &t.bytesDwn, t.touch)
}

// dialRemote opens a connection from the SSH server to addr.
func (t *tunnelRun) dialRemote(addr string) (net.Conn, error) {
	t.mu.Lock()
	rt := t.rt
	t.mu.Unlock()
	if t.useRoute {
		if rt == nil {
			return nil, fmt.Errorf("the connection is not ready")
		}
		return rt.Dial("tcp", addr)
	}
	cl := t.currentClient()
	if cl == nil {
		return nil, fmt.Errorf("SSH connection is not ready")
	}
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := cl.Dial("tcp", addr)
		ch <- res{c, err}
	}()
	select {
	case r := <-ch:
		return r.c, r.err
	case <-time.After(20 * time.Second):
		go func() {
			if r := <-ch; r.c != nil {
				r.c.Close()
			}
		}()
		return nil, fmt.Errorf("timeout connecting to %s", addr)
	}
}

// socks5 implements the server side of a SOCKS5 CONNECT (RFC 1928, no authentication;
// the proxy only listens on the address configured for the tunnel).
func (t *tunnelRun) socks5(c net.Conn) (net.Conn, error) {
	c.SetDeadline(time.Now().Add(20 * time.Second))
	defer c.SetDeadline(time.Time{})
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return nil, err
	}
	if hdr[0] != 5 {
		return nil, fmt.Errorf("not a SOCKS5 client (version %d)", hdr[0])
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return nil, err
	}
	noAuth := false
	for _, m := range methods {
		if m == 0 {
			noAuth = true
		}
	}
	if !noAuth {
		c.Write([]byte{5, 0xff})
		return nil, fmt.Errorf("SOCKS client requires authentication")
	}
	c.Write([]byte{5, 0})
	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return nil, err
	}
	reply := func(code byte) { c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0}) }
	if req[1] != 1 { // only CONNECT
		reply(7)
		return nil, fmt.Errorf("SOCKS command %d not supported", req[1])
	}
	var host string
	switch req[3] {
	case 1:
		b := make([]byte, 4)
		if _, err := io.ReadFull(c, b); err != nil {
			return nil, err
		}
		host = net.IP(b).String()
	case 4:
		b := make([]byte, 16)
		if _, err := io.ReadFull(c, b); err != nil {
			return nil, err
		}
		host = net.IP(b).String()
	case 3:
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return nil, err
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(c, b); err != nil {
			return nil, err
		}
		host = string(b)
	default:
		reply(8)
		return nil, fmt.Errorf("SOCKS address type %d not supported", req[3])
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(c, pb); err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb))))
	out, err := t.dialRemote(addr)
	if err != nil {
		reply(5) // connection refused
		return nil, err
	}
	reply(0)
	return out, nil
}

// pipeConns copies in both directions until both sides are done.
func pipeConns(a, b net.Conn, up, down *atomic.Int64, touch func()) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn, ctr *atomic.Int64) {
		buf := make([]byte, 32*1024)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					break
				}
				ctr.Add(int64(n))
				touch()
			}
			if err != nil {
				break
			}
		}
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		} else {
			dst.Close()
		}
		done <- struct{}{}
	}
	go cp(b, a, up)
	go cp(a, b, down)
	<-done
	select {
	case <-done:
	case <-time.After(60 * time.Second):
	}
	a.Close()
	b.Close()
}

// supervise keeps the SSH connection alive, reconnects after failures and enforces the
// idle limit.
func (t *tunnelRun) supervise() {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-t.stopCh:
			return
		case <-tick.C:
		}
		if t.idleLimit > 0 && t.active.Load() == 0 && time.Since(time.Unix(t.lastActivity.Load(), 0)) > t.idleLimit {
			tunnelMgr.stop(t.key, 0, "idle")
			return
		}
		cl := t.currentClient()
		if t.useRoute && cl == nil {
			continue // proxy only: nothing to keep alive, every connection dials anew
		}
		if cl != nil && sshAlive(cl, 15*time.Second) {
			continue
		}
		t.reconnect()
	}
}

func (t *tunnelRun) reconnect() {
	t.setState("reconnecting", "SSH connection lost")
	t.mu.Lock()
	if t.client != nil {
		t.client.Close()
		t.client = nil
	}
	t.rt = nil
	if t.def.Kind == "remote" && t.ln != nil {
		t.ln.Close() // a remote listener dies with its SSH connection
		t.ln = nil
	}
	t.mu.Unlock()
	backoff := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second}
	for i := 0; ; i++ {
		if t.isStopped() {
			return
		}
		client, rt, err := t.connect()
		if err == nil && t.def.Kind == "remote" {
			var ln net.Listener
			ln, err = client.Listen("tcp", net.JoinHostPort(t.def.BindHost, strconv.Itoa(t.def.BindPort)))
			if err != nil {
				client.Close()
			} else {
				t.mu.Lock()
				t.ln = ln
				t.mu.Unlock()
				go t.acceptLoop(ln)
			}
		}
		if err == nil {
			t.mu.Lock()
			if t.stopped {
				t.mu.Unlock()
				client.Close()
				return
			}
			t.client, t.rt = client, rt
			t.mu.Unlock()
			t.setState("up", "")
			auditLogRef(nil, t.startedByID, t.startedBy, "tunnel.reconnected", t.conn.Name, t.auditDetails(), t.auditRef())
			return
		}
		t.setState("reconnecting", truncateStr(err.Error(), 300))
		if i == 2 {
			auditLogRef(nil, t.startedByID, t.startedBy, "tunnel.error", t.conn.Name,
				mergeMap(t.auditDetails(), map[string]interface{}{"error": truncateStr(err.Error(), 300), "reconnecting": true}), t.auditRef())
		}
		wait := backoff[len(backoff)-1]
		if i < len(backoff) {
			wait = backoff[i]
		}
		select {
		case <-t.stopCh:
			return
		case <-time.After(wait):
		}
	}
}

// connect opens the SSH connection of the tunnel, or for useRoute the route to the target
// (whose SSH client, if any, is the last jump host).
func (t *tunnelRun) connect() (*ssh.Client, *targetRoute, error) {
	if t.useRoute {
		rt, err := openTargetRoute(t.shown)
		if err != nil {
			return nil, nil, err
		}
		return rt.via, rt, nil
	}
	cl, err := dialSSH(t.conn, nil)
	return cl, nil, err
}

func (t *tunnelRun) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.stopped {
		t.stopped = true
		close(t.stopCh)
	}
	if t.ln != nil {
		t.ln.Close()
	}
	if t.client != nil {
		t.client.Close()
	}
}

// stop ends a running tunnel. byUserID 0 = system (idle, policy, connection deleted…).
func (m *tunnelManager) stop(key string, byUserID int, reason string) bool {
	t := m.get(key)
	if t == nil {
		return false
	}
	t.close()
	m.remove(t)
	by, byName := byUserID, ""
	if by > 0 {
		byName = usernameOf(by)
	} else {
		by, byName = t.startedByID, t.startedBy
	}
	auditLogRef(nil, by, byName, "tunnel.stop", t.conn.Name, mergeMap(t.auditDetails(), map[string]interface{}{
		"reason": reason, "connections": t.total.Load(), "bytes_up": t.bytesUp.Load(), "bytes_down": t.bytesDwn.Load(),
		"minutes": int(time.Since(t.startedAt).Minutes())}), t.auditRef())
	return true
}

func (m *tunnelManager) keys(match func(*tunnelRun) bool) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for k, t := range m.runs {
		if match(t) {
			out = append(out, k)
		}
	}
	return out
}

func (m *tunnelManager) stopWhere(match func(*tunnelRun) bool, reason string) int {
	keys := m.keys(match)
	for _, k := range keys {
		m.stop(k, 0, reason)
	}
	return len(keys)
}

func (m *tunnelManager) stopConn(connID int, reason string) {
	m.stopWhere(func(t *tunnelRun) bool { return t.conn.ID == connID || t.forConn == connID }, reason)
}

func (m *tunnelManager) stopUser(userID int, reason string) int {
	return m.stopWhere(func(t *tunnelRun) bool { return t.ownerID == userID }, reason)
}

func (m *tunnelManager) stopAll(reason string) {
	m.stopWhere(func(*tunnelRun) bool { return true }, reason)
}

// restartConn restarts the running tunnels of a connection (its settings changed).
func (m *tunnelManager) restartConn(connID int) {
	m.mu.Lock()
	var list []*tunnelRun
	for _, t := range m.runs {
		if t.conn.ID == connID || t.forConn == connID {
			list = append(list, t)
		}
	}
	m.mu.Unlock()
	for _, t := range list {
		m.stop(t.key, 0, "connection settings changed")
		if t.ephemeral {
			continue // opened again on the next "Open"
		}
		if def, err := loadTunnelDef(t.def.ID); err == nil {
			if c, err := loadConnection(def.ConnID); err == nil {
				go m.start(t.key, def, c, t.startedByID, t.reason, nil)
			}
		}
	}
}

// applyPolicy stops tunnels that the current policies no longer allow.
func (m *tunnelManager) applyPolicy() {
	m.stopWhere(func(t *tunnelRun) bool {
		if tunnelsAllowedFor(t.ownerID) != nil {
			return true
		}
		d := t.def
		return validateTunnelDef(&d, t.ownerID) != nil
	}, "not allowed by the policies any more")
}

// holdConn is called when a terminal of the connection opens: tunnels with start mode
// "connect" start with the first terminal and stop shortly after the last one closes.
func (m *tunnelManager) holdConn(connID, userID int, r *http.Request) {
	m.mu.Lock()
	m.holds[connID]++
	first := m.holds[connID] == 1
	m.mu.Unlock()
	if !first || tunnelsAllowedFor(userID) != nil {
		return
	}
	for _, d := range loadTunnelDefs("conn_id=? AND start_mode='connect'", connID) {
		if c, err := loadConnection(connID); err == nil && c.UserID == userID {
			go m.start("t"+strconv.Itoa(d.ID), d, c, userID, "connect", r)
		}
	}
}

func (m *tunnelManager) releaseConn(connID int) {
	m.mu.Lock()
	if m.holds[connID] > 0 {
		m.holds[connID]--
	}
	m.mu.Unlock()
	// A short grace period keeps the tunnels across a terminal reconnect.
	time.AfterFunc(tunnelConnectGrace, func() {
		m.mu.Lock()
		n := m.holds[connID]
		if n == 0 {
			delete(m.holds, connID)
		}
		m.mu.Unlock()
		if n == 0 {
			m.stopWhere(func(t *tunnelRun) bool { return t.conn.ID == connID && t.reason == "connect" }, "the last terminal closed")
		}
	})
}

// startAlways starts the tunnels with start mode "always" (at server start).
func (m *tunnelManager) startAlways() {
	if !settingBool("tunnels_enabled") {
		return
	}
	defs := loadTunnelDefs("start_mode='always'")
	for _, d := range defs {
		var disabled int
		db.QueryRow(`SELECT disabled FROM users WHERE id=?`, d.UserID).Scan(&disabled)
		if disabled == 1 {
			continue
		}
		c, err := loadConnection(d.ConnID)
		if err != nil || c.UserID != d.UserID {
			continue
		}
		dd := d
		go func() {
			if _, err := m.start("t"+strconv.Itoa(dd.ID), dd, c, dd.UserID, "always", nil); err != nil {
				log.Printf("Tunnel %q of %q did not start: %v", dd.Name, c.Name, err)
			}
		}()
	}
	if len(defs) > 0 {
		log.Printf("Starting %d always-on SSH tunnel(s)", len(defs))
	}
}

// ── snapshot for the UI ──

type tunnelView struct {
	Key        string `json:"key"`
	tunnelDef         // configuration (ID 0 for temporary web tunnels)
	ConnName   string `json:"connection"`
	ConnHost   string `json:"connection_host"`
	Route      string `json:"route,omitempty"`
	Owner      string `json:"owner,omitempty"`
	Running    bool   `json:"running"`
	State      string `json:"state"`
	Error      string `json:"error,omitempty"`
	Listen     string `json:"listen,omitempty"`
	StartedBy  string `json:"started_by,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Active     int64  `json:"active"`
	Total      int64  `json:"total"`
	BytesUp    int64  `json:"bytes_up"`
	BytesDown  int64  `json:"bytes_down"`
	Ephemeral  bool   `json:"ephemeral,omitempty"`
	LastActive string `json:"last_active,omitempty"`
}

func (t *tunnelRun) view() tunnelView {
	t.mu.Lock()
	defer t.mu.Unlock()
	shown := t.conn
	if t.shown.ID > 0 {
		shown = t.shown
	}
	return tunnelView{Key: t.key, tunnelDef: t.def, ConnName: shown.Name, ConnHost: shown.Host, Route: t.route, Owner: usernameOf(t.ownerID),
		Running: true, State: t.state, Error: t.lastErr, Listen: t.boundAddr, StartedBy: t.startedBy,
		StartedAt: t.startedAt.UTC().Format(time.RFC3339), Reason: t.reason, Active: t.active.Load(), Total: t.total.Load(),
		BytesUp: t.bytesUp.Load(), BytesDown: t.bytesDwn.Load(), Ephemeral: t.ephemeral,
		LastActive: time.Unix(t.lastActivity.Load(), 0).UTC().Format(time.RFC3339)}
}

func (m *tunnelManager) runsWhere(match func(*tunnelRun) bool) []tunnelView {
	m.mu.Lock()
	list := []*tunnelRun{}
	for _, t := range m.runs {
		if match(t) {
			list = append(list, t)
		}
	}
	m.mu.Unlock()
	out := []tunnelView{}
	for _, t := range list {
		out = append(out, t.view())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ConnName != out[j].ConnName {
			return out[i].ConnName < out[j].ConnName
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// tunnelsForUser lists every configured tunnel of the user's connections with its state,
// followed by temporary tunnels (web interfaces).
func tunnelsForUser(userID int) []tunnelView {
	names := map[int]Connection{}
	for _, c := range loadUserConnections(userID) {
		names[c.ID] = c
	}
	out := []tunnelView{}
	for _, d := range loadTunnelDefs("user_id=?", userID) {
		c, ok := names[d.ConnID]
		if !ok {
			continue
		}
		key := "t" + strconv.Itoa(d.ID)
		if t := tunnelMgr.get(key); t != nil {
			out = append(out, t.view())
			continue
		}
		out = append(out, tunnelView{Key: key, tunnelDef: d, ConnName: c.Name, ConnHost: c.Host, Route: jumpPath(c), State: "stopped"})
	}
	out = append(out, tunnelMgr.runsWhere(func(t *tunnelRun) bool { return t.ownerID == userID && t.ephemeral })...)
	return out
}

// ─── API ─────────────────────────────────────────────

// GET  /api/tunnels[?all=1]            my tunnels (configured + running); admins: all=1 → every running tunnel
// POST /api/tunnels/{key}/start|stop
func apiTunnelsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tunnels"), "/")
	if rest == "" {
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		if r.URL.Query().Get("all") == "1" && isAdminUser(userID) {
			jsonOK(w, tunnelMgr.runsWhere(func(*tunnelRun) bool { return true }))
			return
		}
		pol := map[string]interface{}{"enabled": settingBool("tunnels_enabled"), "allowed": tunnelsAllowedFor(userID) == nil,
			"bind_any": isAdminUser(userID) || settingBool("tunnel_bind_any"), "remote": policyWhoAllows(userID, "tunnel_remote_forward")}
		jsonOK(w, map[string]interface{}{"tunnels": tunnelsForUser(userID), "policy": pol})
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || r.Method != http.MethodPost {
		jsonError(w, "Not found", 404)
		return
	}
	key, action := parts[0], parts[1]
	switch action {
	case "stop":
		t := tunnelMgr.get(key)
		if t == nil {
			jsonOK(w, map[string]bool{"ok": true})
			return
		}
		if t.ownerID != userID && !isAdminUser(userID) {
			jsonError(w, "Not found", 404)
			return
		}
		reason := "stopped by the user"
		if t.ownerID != userID {
			reason = "stopped by an administrator"
		}
		tunnelMgr.stop(key, userID, reason)
		jsonOK(w, map[string]bool{"ok": true})
	case "start":
		id, err := strconv.Atoi(strings.TrimPrefix(key, "t"))
		if err != nil || !strings.HasPrefix(key, "t") {
			jsonError(w, "Not found", 404)
			return
		}
		def, err := loadTunnelDef(id)
		if err != nil || def.UserID != userID {
			jsonError(w, "Not found", 404)
			return
		}
		if err := validateTunnelDef(&def, userID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		c, err := loadConnection(def.ConnID)
		if err != nil || c.UserID != userID {
			jsonError(w, "Not found", 404)
			return
		}
		t, err := tunnelMgr.start(key, def, c, userID, "manual", r)
		if err != nil {
			jsonError(w, err.Error(), 502)
			return
		}
		jsonOK(w, t.view())
	default:
		jsonError(w, "Not found", 404)
	}
}

// GET /api/connections/{id}/tunnels   → the connection's tunnels
// PUT /api/connections/{id}/tunnels   → replace them: [{id?, name, kind, …}, …]
func connectionTunnelsHandler(w http.ResponseWriter, r *http.Request, userID int, c Connection) {
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, loadTunnelDefs("conn_id=?", c.ID))
	case http.MethodPut:
		var in []tunnelDef
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if len(in) > 50 {
			jsonError(w, "Too many tunnels (max. 50 per connection)", 400)
			return
		}
		if len(in) > 0 {
			if err := tunnelsAllowedFor(userID); err != nil {
				jsonError(w, err.Error(), 403)
				return
			}
			if !isSSHProtocol(c) {
				jsonError(w, "Tunnels need an SSH connection", 400)
				return
			}
		}
		existing := map[int]tunnelDef{}
		for _, d := range loadTunnelDefs("conn_id=?", c.ID) {
			existing[d.ID] = d
		}
		seenPort := map[string]bool{}
		for i := range in {
			if err := validateTunnelDef(&in[i], userID); err != nil {
				jsonError(w, fmt.Sprintf("Tunnel %d: %v", i+1, err), 400)
				return
			}
			if in[i].BindPort > 0 && in[i].Kind != "remote" {
				k := in[i].BindHost + ":" + strconv.Itoa(in[i].BindPort)
				if seenPort[k] {
					jsonError(w, fmt.Sprintf("Tunnel %d: port %d is used twice", i+1, in[i].BindPort), 400)
					return
				}
				seenPort[k] = true
			}
			if in[i].ID > 0 {
				if _, ok := existing[in[i].ID]; !ok {
					in[i].ID = 0
				}
			}
		}
		tx, err := db.Begin()
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		defer tx.Rollback()
		keep := map[int]bool{}
		now := time.Now().UTC().Format(time.RFC3339)
		changedKeys := []string{}
		for i, d := range in {
			if d.ID > 0 {
				keep[d.ID] = true
				old := existing[d.ID]
				d.UserID, d.ConnID, old.Sort, d.Sort = userID, c.ID, i, i
				if old != d {
					changedKeys = append(changedKeys, "t"+strconv.Itoa(d.ID))
				}
				tx.Exec(`UPDATE connection_tunnels SET name=?, kind=?, bind_host=?, bind_port=?, target_host=?, target_port=?, open_scheme=?, open_path=?, start_mode=?, sort=? WHERE id=? AND conn_id=?`,
					d.Name, d.Kind, d.BindHost, d.BindPort, d.TargetHost, d.TargetPort, d.OpenScheme, d.OpenPath, d.StartMode, i, d.ID, c.ID)
				continue
			}
			tx.Exec(`INSERT INTO connection_tunnels (conn_id, user_id, name, kind, bind_host, bind_port, target_host, target_port, open_scheme, open_path, start_mode, sort, created_at)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, c.ID, userID, d.Name, d.Kind, d.BindHost, d.BindPort, d.TargetHost, d.TargetPort, d.OpenScheme, d.OpenPath, d.StartMode, i, now)
		}
		for id := range existing {
			if !keep[id] {
				tx.Exec(`DELETE FROM connection_tunnels WHERE id=?`, id)
				changedKeys = append(changedKeys, "t"+strconv.Itoa(id))
			}
		}
		if err := tx.Commit(); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		// Running tunnels that were changed or removed are restarted / stopped.
		for _, k := range changedKeys {
			t := tunnelMgr.get(k)
			if t == nil {
				continue
			}
			reason := t.reason
			tunnelMgr.stop(k, userID, "tunnel settings changed")
			id, _ := strconv.Atoi(strings.TrimPrefix(k, "t"))
			if def, err := loadTunnelDef(id); err == nil {
				go tunnelMgr.start(k, def, c, userID, reason, nil)
			}
		}
		auditLogRef(r, userID, usernameOf(userID), "tunnel.configured", c.Name, map[string]interface{}{"tunnels": len(in)}, auditRef{ConnID: c.ID})
		tunnelMgr.notify(userID)
		jsonOK(w, loadTunnelDefs("conn_id=?", c.ID))
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// openWebHandler implements POST /api/connections/{id}/open-web for HTTP/HTTPS connections.
// Without jump hosts the browser opens the address directly. With jump hosts WRM starts
// (or reuses) a temporary local tunnel on 127.0.0.1 and returns its address; it closes
// after 30 minutes without traffic.
func openWebHandler(w http.ResponseWriter, r *http.Request, userID int, c Connection) {
	if !isWeb(c) {
		jsonError(w, "Not a web interface connection", 400)
		return
	}
	scheme := strings.ToLower(c.Protocol)
	host, port, err := net.SplitHostPort(ensurePort(c.Host, c.Protocol))
	if err != nil {
		jsonError(w, "Invalid host", 400)
		return
	}
	if !needsRoute(c) {
		auditLogRef(r, userID, usernameOf(userID), "web.open", c.Name, map[string]string{"url": scheme + "://" + net.JoinHostPort(host, port) + c.WebPath}, auditRef{ConnID: c.ID})
		jsonOK(w, map[string]interface{}{"url": scheme + "://" + net.JoinHostPort(host, port) + c.WebPath, "direct": true})
		return
	}
	if err := tunnelsAllowedFor(userID); err != nil {
		jsonError(w, err.Error(), 403)
		return
	}
	pn, _ := strconv.Atoi(port)
	key := "w" + strconv.Itoa(c.ID) + "u" + strconv.Itoa(userID)
	t := tunnelMgr.get(key)
	if t == nil {
		def := tunnelDef{ConnID: c.ID, UserID: userID, Name: c.Name, Kind: "local", BindHost: "127.0.0.1", BindPort: 0,
			TargetHost: host, TargetPort: pn, OpenScheme: scheme, OpenPath: c.WebPath, StartMode: "manual"}
		t, err = tunnelMgr.startEphemeral(key, def, c, userID, "web", r)
		if err != nil {
			jsonError(w, err.Error(), 502)
			return
		}
	}
	_, lport, _ := net.SplitHostPort(t.view().Listen)
	url := scheme + "://127.0.0.1:" + lport + c.WebPath
	auditLogRef(r, userID, usernameOf(userID), "web.open", c.Name, map[string]string{"url": url, "via": jumpPath(c), "target": net.JoinHostPort(host, port)}, auditRef{ConnID: c.ID})
	jsonOK(w, map[string]interface{}{"url": url, "direct": false, "listen": t.view().Listen, "route": jumpPath(c), "local_only": true})
}

// startEphemeral starts a temporary local tunnel to c's host:port over c's route (jump
// hosts and / or proxy), for web interfaces and remote desktops.
func (m *tunnelManager) startEphemeral(key string, def tunnelDef, c Connection, userID int, reason string, r *http.Request) (*tunnelRun, error) {
	via := c // shown as the connection it runs over: the last jump host, or c itself
	if chain, err := jumpChain(c); err != nil {
		return nil, err
	} else if len(chain) > 0 {
		via = chain[len(chain)-1]
	}
	m.mu.Lock()
	if old := m.runs[key]; old != nil {
		m.mu.Unlock()
		return old, nil
	}
	t := &tunnelRun{key: key, def: def, conn: via, route: jumpPath(c), ownerID: userID, startedByID: userID, startedBy: usernameOf(userID),
		reason: reason, ephemeral: true, useRoute: true, forConn: c.ID, shown: c, idleLimit: 30 * time.Minute, startedAt: time.Now(), state: "starting", stopCh: make(chan struct{}), ip: clientIP(r)}
	t.def.Name = c.Name
	t.touch()
	m.runs[key] = t
	m.mu.Unlock()
	if err := t.open(); err != nil {
		t.close()
		m.remove(t)
		auditLogRef(r, userID, t.startedBy, "tunnel.error", c.Name, mergeMap(t.auditDetails(), map[string]interface{}{"error": truncateStr(err.Error(), 300)}), auditRef{ConnID: c.ID})
		return nil, err
	}
	t.setState("up", "")
	auditLogRef(r, userID, t.startedBy, "tunnel.start", c.Name, t.auditDetails(), auditRef{ConnID: c.ID})
	go t.supervise()
	return t, nil
}

// testWebReachable checks that the port of a web connection accepts TCP connections
// (through the jump hosts when configured).
func testWebReachable(c Connection) error {
	addr := ensurePort(c.Host, c.Protocol)
	if !needsRoute(c) {
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return err
		}
		return conn.Close()
	}
	rt, err := openTargetRoute(c)
	if err != nil {
		return err
	}
	defer rt.Close()
	conn, err := rt.Dial("tcp", addr)
	if err != nil {
		if _, ok := err.(*proxyError); ok {
			return err
		}
		return fmt.Errorf("%s cannot reach %s: %v", jumpPath(c), addr, err)
	}
	return conn.Close()
}
