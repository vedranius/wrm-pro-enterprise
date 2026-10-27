// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── PROXIES ─────────────────────────────────────────
//
// A proxy is defined once and chosen per connection, like a vault credential. WRM reaches
// the connection's host:port through it:
//
//   WRM ──► [jump hosts] ──► proxy (SOCKS5 / SOCKS4 / SOCKS4a / HTTP CONNECT) ──► target
//
// With jump hosts the proxy is reached through them (the last jump host opens the TCP
// connection to the proxy). Every TCP path of a connection uses the same route: the first
// SSH hop in dialSSH (terminal, SFTP, search, transfers, tunnels, key deploy, rotation,
// network tools from a server, connection test), FTP's dial function, the local relay of
// remote desktops, web interfaces, live status and Redfish. IPMI and Serial-over-LAN use
// UDP and cannot go through a proxy.
//
// The type "WRM SOCKS tunnel" uses a dynamic SSH tunnel (-D) of one of the user's
// connections: WRM starts it when needed and talks SOCKS5 to it on the WRM machine.
//
// Proxies can be shared (grants or everybody) without revealing the password. A shared
// proxy with a password is not reached through the grantee's own jump hosts, which could
// read the (plain text) proxy login. The policy "proxies" decides who may define proxies.

const (
	maxProxiesPerUser = 200
	proxyDialTimeout  = 15 * time.Second
)

var proxyKinds = map[string]bool{"socks5": true, "socks4": true, "http": true, "wrm_tunnel": true}

type proxyDef struct {
	ID           int      `json:"id"`
	OwnerID      int      `json:"owner_id"`
	Owner        string   `json:"owner"`
	Mine         bool     `json:"mine"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"` // socks5 | socks4 | http | wrm_tunnel
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	Username     string   `json:"username"`
	CredentialID *int     `json:"credential_id,omitempty"` // log in with a vault credential
	CredName     string   `json:"credential_name,omitempty"`
	RemoteDNS    bool     `json:"remote_dns"` // let the proxy resolve host names (SOCKS4a, SOCKS5 domain)
	TunnelID     *int     `json:"tunnel_id,omitempty"`
	TunnelName   string   `json:"tunnel_name,omitempty"`
	TunnelConnID int      `json:"tunnel_conn_id,omitempty"`
	TunnelConn   string   `json:"tunnel_conn,omitempty"`
	Description  string   `json:"description"`
	SharedAll    bool     `json:"shared_all"`
	Grants       []int    `json:"grants"`
	GrantNames   []string `json:"grant_names"`
	HasPassword  bool     `json:"has_password"`
	UsedBy       int      `json:"used_by"`
	URL          string   `json:"url"`               // socks5://host:port (shown in routes)
	Warning      string   `json:"warning,omitempty"` // docker_loopback
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
	password     string   // encrypted
}

const proxyCols = `id,owner_id,name,kind,host,port,username,password,credential_id,remote_dns,tunnel_id,description,shared_all,created_at,updated_at`

func scanProxy(sc interface{ Scan(...interface{}) error }) (proxyDef, error) {
	var p proxyDef
	var dns, shared int
	err := sc.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Kind, &p.Host, &p.Port, &p.Username, &p.password, &p.CredentialID, &dns, &p.TunnelID,
		&p.Description, &shared, &p.CreatedAt, &p.UpdatedAt)
	p.RemoteDNS, p.SharedAll = dns == 1, shared == 1
	p.HasPassword = p.password != ""
	p.URL = proxyURL(p)
	return p, err
}

func loadProxy(id int) (proxyDef, error) {
	p, err := scanProxy(db.QueryRow(`SELECT `+proxyCols+` FROM proxies WHERE id=?`, id))
	if err != nil {
		return p, fmt.Errorf("proxy not found")
	}
	return p, nil
}

// proxyURL is the address shown in routes and the audit log, e.g. socks5://10.1.1.1:1080.
func proxyURL(p proxyDef) string {
	switch p.Kind {
	case "wrm_tunnel":
		name := p.Name
		if p.TunnelID != nil {
			var conn string
			db.QueryRow(`SELECT c.name FROM connection_tunnels t JOIN connections c ON c.id=t.conn_id WHERE t.id=?`, *p.TunnelID).Scan(&conn)
			if conn != "" {
				name = conn
			}
		}
		return "socks5://wrm-tunnel(" + name + ")"
	case "socks4":
		if p.RemoteDNS {
			return "socks4a://" + net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
		}
		return "socks4://" + net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	}
	return p.Kind + "://" + net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
}

func proxyAccessible(p proxyDef, userID int) bool {
	if p.OwnerID == userID || p.SharedAll {
		return true
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM proxy_grants WHERE proxy_id=? AND user_id=?`, p.ID, userID).Scan(&n)
	return n > 0
}

func proxiesAllowed(userID int) bool {
	return policyWhoAllows(userID, "proxies")
}

// inContainer reports whether WRM runs in a Docker / Podman container, where 127.0.0.1 is
// the container itself and not the host.
var inContainer = func() bool {
	for _, f := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	return false
}

func proxyWarning(p proxyDef) string {
	if p.Kind != "wrm_tunnel" && isLoopbackHost(p.Host) && inContainer() {
		return "docker_loopback"
	}
	return ""
}

func proxiesForUser(userID int) []proxyDef {
	out := []proxyDef{}
	rows, err := db.Query(`SELECT `+proxyCols+` FROM proxies
		WHERE owner_id=? OR shared_all=1 OR id IN (SELECT proxy_id FROM proxy_grants WHERE user_id=?)
		ORDER BY name COLLATE NOCASE, id`, userID, userID)
	if err != nil {
		return out
	}
	var list []proxyDef
	for rows.Next() {
		if p, err := scanProxy(rows); err == nil {
			list = append(list, p)
		}
	}
	rows.Close()
	for _, p := range list {
		out = append(out, decorateProxy(p, userID))
	}
	return out
}

func decorateProxy(p proxyDef, userID int) proxyDef {
	p.Mine = p.OwnerID == userID
	p.Owner = usernameOf(p.OwnerID)
	p.Grants, p.GrantNames = []int{}, []string{}
	p.Warning = proxyWarning(p)
	if p.CredentialID != nil {
		db.QueryRow(`SELECT name FROM credentials WHERE id=?`, *p.CredentialID).Scan(&p.CredName)
	}
	if p.TunnelID != nil {
		db.QueryRow(`SELECT t.name, t.conn_id, c.name FROM connection_tunnels t JOIN connections c ON c.id=t.conn_id WHERE t.id=?`, *p.TunnelID).
			Scan(&p.TunnelName, &p.TunnelConnID, &p.TunnelConn)
	}
	if p.Mine {
		if gr, err := db.Query(`SELECT g.user_id, u.username FROM proxy_grants g JOIN users u ON u.id=g.user_id WHERE g.proxy_id=? ORDER BY u.username`, p.ID); err == nil {
			for gr.Next() {
				var id int
				var n string
				gr.Scan(&id, &n)
				p.Grants = append(p.Grants, id)
				p.GrantNames = append(p.GrantNames, n)
			}
			gr.Close()
		}
		db.QueryRow(`SELECT COUNT(*) FROM connections WHERE proxy_id=?`, p.ID).Scan(&p.UsedBy)
	} else {
		db.QueryRow(`SELECT COUNT(*) FROM connections WHERE proxy_id=? AND user_id=?`, p.ID, userID).Scan(&p.UsedBy)
		p.Username, p.CredentialID, p.CredName = "", nil, "" // the login stays with the owner
	}
	return p
}

// ─── RESOLVING ───────────────────────────────────────

// resolvedProxy is a proxy ready to dial, with its login.
type resolvedProxy struct {
	p          proxyDef
	user, pass string
	label      string
	forUser    int  // the user the route is opened for
	behindJump bool // reached through jump hosts (not from the WRM machine)
}

type proxyError struct {
	label, target string
	targetSide    bool // the proxy works, but cannot reach the target
	msg           string
}

func (e *proxyError) Error() string {
	if e.targetSide {
		return fmt.Sprintf("proxy %s cannot reach %s: %s", e.label, e.target, e.msg)
	}
	return fmt.Sprintf("proxy %s: %s", e.label, e.msg)
}

// proxyOf returns the proxy connection c is reached through (nil = none). behindJump: the
// proxy is reached through jump hosts of the user.
func proxyOf(c Connection, behindJump bool) (*resolvedProxy, error) {
	if c.ProxyID == nil || *c.ProxyID <= 0 {
		return nil, nil
	}
	p, err := loadProxy(*c.ProxyID)
	if err != nil {
		return nil, fmt.Errorf("the proxy of %q no longer exists", c.Name)
	}
	if c.UserID > 0 && !proxyAccessible(p, c.UserID) {
		return nil, fmt.Errorf("proxy %q is no longer shared with you", p.Name)
	}
	rp := &resolvedProxy{p: p, label: p.URL, forUser: c.UserID, behindJump: behindJump}
	if rp.forUser <= 0 {
		rp.forUser = p.OwnerID
	}
	switch p.Kind {
	case "wrm_tunnel":
		if behindJump {
			return nil, fmt.Errorf("proxy %q is a WRM tunnel: it runs on the WRM server and cannot be reached through a jump host", p.Name)
		}
		return rp, nil
	}
	if p.CredentialID != nil {
		cr, err := loadCredential(*p.CredentialID)
		if err != nil || !credentialAccessible(cr, p.OwnerID) {
			return nil, fmt.Errorf("proxy %q: its vault credential is no longer available", p.Name)
		}
		if !hostAllowed(cr.Hosts, p.Host) {
			return nil, fmt.Errorf("proxy %q: credential %q may only be used for hosts matching %q", p.Name, cr.Name, cr.Hosts)
		}
		rp.user, rp.pass = cr.Username, decryptValue(cr.password)
	} else {
		rp.user, rp.pass = p.Username, decryptValue(p.password)
	}
	if behindJump && p.OwnerID != c.UserID && rp.pass != "" {
		return nil, fmt.Errorf("proxy %q is shared with a password: it cannot be reached through your own jump hosts", p.Name)
	}
	return rp, nil
}

// validateProxyChoice checks a proxy choice when a connection is saved.
func validateProxyChoice(userID, connID int, c *Connection) error {
	if c.ProxyID == nil || *c.ProxyID <= 0 {
		return nil
	}
	if c.Protocol == "SERIAL" {
		c.ProxyID = nil
		return nil
	}
	p, err := loadProxy(*c.ProxyID)
	if err != nil || !proxyAccessible(p, userID) {
		return fmt.Errorf("proxy not found")
	}
	hasJump := c.JumpID != nil && *c.JumpID > 0
	if p.Kind == "wrm_tunnel" {
		if hasJump {
			return fmt.Errorf("a WRM tunnel proxy runs on the WRM server: it cannot be combined with a jump host")
		}
		if p.TunnelID != nil && connID > 0 {
			if d, err := loadTunnelDef(*p.TunnelID); err == nil && d.ConnID == connID {
				return fmt.Errorf("a connection cannot use its own tunnel as its proxy")
			}
		}
	}
	if hasJump && p.OwnerID != userID && p.HasPassword {
		return fmt.Errorf("proxy %q is shared with a password: it cannot be reached through your own jump hosts", p.Name)
	}
	return nil
}

// validateRoute checks the route a connection would use (its own choices or its folder's
// defaults). c holds the stored choice and is not changed, except SERIAL dropping a proxy.
func validateRoute(userID, connID int, c *Connection) error {
	if c.Protocol == "SERIAL" {
		c.ProxyID = nil
		return nil
	}
	eff := *c
	eff.ID, eff.UserID = connID, userID
	applyFolderDefaults(&eff)
	if eff.jumpInherited {
		if _, err := jumpChain(eff); err != nil {
			return fmt.Errorf("the folder's jump host: %v", err)
		}
	}
	return validateProxyChoice(userID, connID, &eff)
}

// ─── DIALING ─────────────────────────────────────────

func directDial(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, proxyDialTimeout)
}

// dialTimeout runs a dial function that has no timeout of its own (an SSH channel).
func dialTimeout(dial dialFunc, addr string, d time.Duration) (net.Conn, error) {
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := dial("tcp", addr)
		ch <- res{c, err}
	}()
	select {
	case r := <-ch:
		return r.c, r.err
	case <-time.After(d):
		go func() {
			if r := <-ch; r.c != nil {
				r.c.Close()
			}
		}()
		return nil, fmt.Errorf("timeout")
	}
}

// dial opens a connection to target (host:port) through the proxy; base reaches the proxy.
func (rp *resolvedProxy) dial(base dialFunc, target string) (net.Conn, error) {
	kind, addr, user, pass, remoteDNS := rp.p.Kind, net.JoinHostPort(rp.p.Host, strconv.Itoa(rp.p.Port)), rp.user, rp.pass, rp.p.RemoteDNS
	if kind == "wrm_tunnel" {
		a, err := ensureProxyTunnel(rp.p)
		if err != nil {
			return nil, &proxyError{label: rp.label, msg: err.Error()}
		}
		kind, addr, user, pass, remoteDNS, base = "socks5", a, "", "", true, directDial
	}
	if base == nil {
		base = directDial
	}
	conn, err := dialTimeout(base, addr, proxyDialTimeout)
	if err != nil {
		return nil, &proxyError{label: rp.label, msg: "not reachable: " + shortNetError(err)}
	}
	// Tunnel listeners on the WRM machine have no login of their own: a proxy may not lead
	// into another user's SSH tunnel (their SSH login and network).
	if rp.p.Kind != "wrm_tunnel" && !rp.behindJump {
		if ta, ok := conn.RemoteAddr().(*net.TCPAddr); ok && isLocalIP(ta.IP) {
			if owner := tunnelMgr.localListenerOwner(ta.Port); owner != 0 && owner != rp.forUser {
				conn.Close()
				return nil, &proxyError{label: rp.label, msg: "this address is an SSH tunnel of another user on the WRM server"}
			}
		}
	}
	// SSH channels (a proxy behind a jump host) have no deadlines: a timer closes the
	// connection when the handshake takes too long.
	conn.SetDeadline(time.Now().Add(proxyDialTimeout))
	timer := time.AfterFunc(proxyDialTimeout, func() { conn.Close() })
	var out net.Conn = conn
	switch kind {
	case "socks5":
		err = socks5Connect(conn, target, user, pass, remoteDNS)
	case "socks4":
		err = socks4Connect(conn, target, user, remoteDNS)
	case "http":
		out, err = httpConnect(conn, target, user, pass, remoteDNS)
	default:
		err = fmt.Errorf("unknown proxy type %q", kind)
	}
	if !timer.Stop() {
		conn.Close()
		return nil, &proxyError{label: rp.label, msg: "no answer to the proxy handshake (timeout)"}
	}
	if err != nil {
		conn.Close()
		if pe, ok := err.(*proxyError); ok {
			pe.label, pe.target = rp.label, target
			return nil, pe
		}
		return nil, &proxyError{label: rp.label, target: target, msg: shortNetError(err)}
	}
	conn.SetDeadline(time.Time{})
	return out, nil
}

// isLocalIP: ip is an address of the WRM machine (loopback, unspecified or an interface address).
func isLocalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
			return true
		}
	}
	return false
}

func splitTarget(target string, remoteDNS bool) (host string, ip net.IP, port int, err error) {
	h, p, err := net.SplitHostPort(target)
	if err != nil {
		return "", nil, 0, err
	}
	port, err = strconv.Atoi(p)
	if err != nil || port < 1 || port > 65535 {
		return "", nil, 0, fmt.Errorf("invalid port in %q", target)
	}
	if ip = net.ParseIP(h); ip != nil {
		return h, ip, port, nil
	}
	if !remoteDNS {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, h)
		cancel()
		if err != nil || len(addrs) == 0 {
			return "", nil, 0, &proxyError{targetSide: true, msg: "host name not found (resolved on the WRM server; turn on \"DNS at the proxy\" to let the proxy resolve it)"}
		}
		for _, a := range addrs {
			if a.IP.To4() != nil {
				return h, a.IP, port, nil
			}
		}
		return h, addrs[0].IP, port, nil
	}
	return h, nil, port, nil
}

var socks5Replies = map[byte]string{1: "general failure", 2: "not allowed by the proxy's rules", 3: "network unreachable", 4: "host unreachable",
	5: "connection refused", 6: "TTL expired", 7: "command not supported", 8: "address type not supported"}

// socks5Connect performs a SOCKS5 CONNECT (RFC 1928) with optional user name / password
// authentication (RFC 1929).
func socks5Connect(c net.Conn, target, user, pass string, remoteDNS bool) error {
	host, ip, port, err := splitTarget(target, remoteDNS)
	if err != nil {
		return err
	}
	methods := []byte{0}
	if user != "" {
		methods = []byte{0, 2}
	}
	if _, err := c.Write(append([]byte{5, byte(len(methods))}, methods...)); err != nil {
		return &proxyError{msg: "connection closed by the proxy: " + shortNetError(err)}
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(c, resp); err != nil {
		return &proxyError{msg: "no SOCKS5 answer (is it a SOCKS5 proxy?): " + shortNetError(err)}
	}
	if resp[0] != 5 {
		return &proxyError{msg: fmt.Sprintf("not a SOCKS5 proxy (version %d in the answer)", resp[0])}
	}
	switch resp[1] {
	case 0:
	case 2:
		if user == "" {
			return &proxyError{msg: "authentication required: set a user name and password for the proxy"}
		}
		b := []byte{1, byte(len(user))}
		b = append(b, user...)
		b = append(b, byte(len(pass)))
		b = append(b, pass...)
		if _, err := c.Write(b); err != nil {
			return &proxyError{msg: shortNetError(err)}
		}
		ar := make([]byte, 2)
		if _, err := io.ReadFull(c, ar); err != nil {
			return &proxyError{msg: "authentication failed (the proxy closed the connection)"}
		}
		if ar[1] != 0 {
			return &proxyError{msg: "authentication failed: wrong user name or password"}
		}
	case 0xff:
		if user == "" {
			return &proxyError{msg: "authentication required: set a user name and password for the proxy"}
		}
		return &proxyError{msg: "the proxy accepts none of the offered authentication methods"}
	default:
		return &proxyError{msg: fmt.Sprintf("the proxy chose an unsupported authentication method %d", resp[1])}
	}
	req := []byte{5, 1, 0}
	switch {
	case ip != nil && ip.To4() != nil:
		req = append(append(req, 1), ip.To4()...)
	case ip != nil:
		req = append(append(req, 4), ip.To16()...)
	default:
		if len(host) > 255 {
			return fmt.Errorf("host name too long")
		}
		req = append(append(req, 3, byte(len(host))), host...)
	}
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	if _, err := c.Write(req); err != nil {
		return &proxyError{msg: shortNetError(err)}
	}
	rep := make([]byte, 4)
	if _, err := io.ReadFull(c, rep); err != nil {
		return &proxyError{msg: "no answer to CONNECT: " + shortNetError(err)}
	}
	if rep[1] != 0 {
		msg, ok := socks5Replies[rep[1]]
		if !ok {
			msg = "error"
		}
		return &proxyError{targetSide: rep[1] != 2 && rep[1] != 7 && rep[1] != 8, msg: fmt.Sprintf("%s (SOCKS5 reply %d)", msg, rep[1])}
	}
	skip := 0
	switch rep[3] {
	case 1:
		skip = 4
	case 4:
		skip = 16
	case 3:
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return &proxyError{msg: shortNetError(err)}
		}
		skip = int(l[0])
	}
	if _, err := io.ReadFull(c, make([]byte, skip+2)); err != nil {
		return &proxyError{msg: shortNetError(err)}
	}
	return nil
}

// socks4Connect performs a SOCKS4 CONNECT, or SOCKS4a with the host name sent to the proxy.
func socks4Connect(c net.Conn, target, user string, remoteDNS bool) error {
	host, ip, port, err := splitTarget(target, remoteDNS)
	if err != nil {
		return err
	}
	req := binary.BigEndian.AppendUint16([]byte{4, 1}, uint16(port))
	switch {
	case ip != nil && ip.To4() != nil:
		req = append(req, ip.To4()...)
	case ip != nil:
		return &proxyError{msg: "SOCKS4 supports IPv4 addresses only"}
	default:
		req = append(req, 0, 0, 0, 1) // SOCKS4a: 0.0.0.x, the name follows the user id
	}
	req = append(append(req, user...), 0)
	if ip == nil {
		req = append(append(req, host...), 0)
	}
	if _, err := c.Write(req); err != nil {
		return &proxyError{msg: shortNetError(err)}
	}
	rep := make([]byte, 8)
	if _, err := io.ReadFull(c, rep); err != nil {
		return &proxyError{msg: "no SOCKS4 answer (is it a SOCKS4 proxy?): " + shortNetError(err)}
	}
	switch rep[1] {
	case 0x5a:
		return nil
	case 0x5b:
		return &proxyError{targetSide: true, msg: "request rejected or failed (SOCKS4 reply 91)"}
	case 0x5c, 0x5d:
		return &proxyError{msg: fmt.Sprintf("rejected: the proxy could not confirm the user id with identd (SOCKS4 reply %d)", rep[1])}
	}
	return &proxyError{msg: fmt.Sprintf("not a SOCKS4 proxy (answer %d %d)", rep[0], rep[1])}
}

// bufferedConn returns bytes read ahead by a bufio.Reader before reading from the conn.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.r.Read(p) }

// httpConnect opens a tunnel with HTTP CONNECT (RFC 9110), with Basic authentication.
func httpConnect(c net.Conn, target, user, pass string, remoteDNS bool) (net.Conn, error) {
	host, ip, port, err := splitTarget(target, remoteDNS)
	if err != nil {
		return nil, err
	}
	if ip != nil {
		host = ip.String()
	}
	hp := net.JoinHostPort(host, strconv.Itoa(port))
	req := "CONNECT " + hp + " HTTP/1.1\r\nHost: " + hp + "\r\nUser-Agent: WRM-PRO/" + AppVersion + "\r\n"
	if user != "" {
		req += "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)) + "\r\n"
	}
	if _, err := io.WriteString(c, req+"\r\n"); err != nil {
		return nil, &proxyError{msg: shortNetError(err)}
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return nil, &proxyError{msg: "no HTTP answer to CONNECT (is it an HTTP proxy?): " + shortNetError(err)}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &bufferedConn{Conn: c, r: br}, nil
	}
	resp.Body.Close()
	status := strings.TrimSpace(resp.Status)
	switch resp.StatusCode {
	case http.StatusProxyAuthRequired:
		if user == "" {
			return nil, &proxyError{msg: "authentication required: set a user name and password for the proxy (HTTP " + status + ")"}
		}
		return nil, &proxyError{msg: "authentication failed: wrong user name or password (HTTP " + status + ")"}
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusNotFound:
		return nil, &proxyError{targetSide: true, msg: "HTTP " + status}
	}
	return nil, &proxyError{msg: "refused the tunnel: HTTP " + status}
}

// ensureProxyTunnel starts the dynamic tunnel of a "WRM SOCKS tunnel" proxy when needed and
// returns the address it listens on.
func ensureProxyTunnel(p proxyDef) (string, error) {
	if p.TunnelID == nil {
		return "", fmt.Errorf("no tunnel selected")
	}
	def, err := loadTunnelDef(*p.TunnelID)
	if err != nil || def.Kind != "dynamic" || def.UserID != p.OwnerID {
		return "", fmt.Errorf("the SOCKS tunnel of this proxy no longer exists")
	}
	key := "t" + strconv.Itoa(def.ID)
	t := tunnelMgr.get(key)
	if t == nil {
		c, err := loadConnection(def.ConnID)
		if err != nil {
			return "", fmt.Errorf("the connection of the SOCKS tunnel no longer exists")
		}
		// The tunnel's own route must not need a WRM tunnel proxy (that could wait for itself).
		chain, err := jumpChain(c)
		if err != nil {
			return "", fmt.Errorf("the SOCKS tunnel of %s: %v", c.Name, err)
		}
		for _, hop := range append(chain, c) {
			if hop.ProxyID != nil && *hop.ProxyID > 0 {
				if cp, err := loadProxy(*hop.ProxyID); err == nil && cp.Kind == "wrm_tunnel" {
					return "", fmt.Errorf("connection %q of this tunnel is reached through a WRM tunnel proxy itself", hop.Name)
				}
			}
		}
		// start returns the run that is already registered (also one that is still starting).
		t, err = tunnelMgr.start(key, def, c, def.UserID, "proxy", nil)
		if err != nil {
			return "", fmt.Errorf("starting the SOCKS tunnel of %s failed: %v", c.Name, err)
		}
	}
	t.touch()
	waitUntilUp := time.Now().Add(20 * time.Second)
	for {
		v := t.view()
		if v.State == "up" && v.Listen != "" {
			h, port, _ := net.SplitHostPort(v.Listen)
			if h == "" || h == "0.0.0.0" || h == "::" {
				h = "127.0.0.1"
			}
			return net.JoinHostPort(h, port), nil
		}
		if v.State == "error" || v.State == "stopped" || time.Now().After(waitUntilUp) {
			return "", fmt.Errorf("the SOCKS tunnel of %s is not running: %s", v.ConnName, firstNonEmpty(v.Error, v.State))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// ─── ROUTES ──────────────────────────────────────────

// targetRoute reaches c's host:port (and other addresses as seen from there) through the
// jump hosts and the proxy of c.
type targetRoute struct {
	via   *ssh.Client // last jump host (nil = from the WRM server)
	proxy *resolvedProxy
}

// needsRoute: c is not reached directly from the WRM server.
func needsRoute(c Connection) bool {
	return (c.JumpID != nil && *c.JumpID > 0) || (c.ProxyID != nil && *c.ProxyID > 0)
}

func openTargetRoute(c Connection) (*targetRoute, error) {
	chain, err := jumpChain(c)
	if err != nil {
		return nil, err
	}
	rt := &targetRoute{}
	if rt.proxy, err = proxyOf(c, len(chain) > 0); err != nil {
		return nil, err
	}
	if len(chain) > 0 {
		last := chain[len(chain)-1]
		if rt.via, err = dialSSH(last, nil); err != nil {
			return nil, fmt.Errorf("jump host %s: %w", last.Name, err)
		}
	}
	return rt, nil
}

func (rt *targetRoute) Dial(network, addr string) (net.Conn, error) {
	var base dialFunc = directDial
	if rt.via != nil {
		base = rt.via.Dial
	}
	if rt.proxy != nil {
		return rt.proxy.dial(base, addr)
	}
	return dialTimeout(base, addr, proxyDialTimeout)
}

func (rt *targetRoute) Close() {
	if rt != nil && rt.via != nil {
		rt.via.Close()
	}
}

// ─── API ─────────────────────────────────────────────

// GET    /api/proxies                 proxies I can use (no secrets)
// POST   /api/proxies                 create (policy "proxies")
// PUT    /api/proxies/{id}            change (owner)
// DELETE /api/proxies/{id}            (owner; refused while connections or folders use it)
// POST   /api/proxies/{id}/test       {target} connect to the proxy (and through it to target)
func apiProxiesHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/proxies"), "/")
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			jsonOK(w, map[string]interface{}{"proxies": proxiesForUser(userID), "can_define": proxiesAllowed(userID), "container": inContainer()})
		case http.MethodPost:
			saveProxy(w, r, userID, nil)
		default:
			jsonError(w, "Method not allowed", 405)
		}
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "Bad ID", 400)
		return
	}
	p, err := loadProxy(id)
	if err != nil || !proxyAccessible(p, userID) {
		jsonError(w, "Not found", 404)
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	owner := p.OwnerID == userID
	switch {
	case action == "" && r.Method == http.MethodPut:
		if !owner {
			jsonError(w, "Only the owner of the proxy can change it", 403)
			return
		}
		saveProxy(w, r, userID, &p)
	case action == "" && r.Method == http.MethodDelete:
		if !owner {
			jsonError(w, "Only the owner of the proxy can delete it", 403)
			return
		}
		var n, nf int
		db.QueryRow(`SELECT COUNT(*) FROM connections WHERE proxy_id=?`, id).Scan(&n)
		db.QueryRow(`SELECT COUNT(*) FROM folders WHERE proxy_id=?`, id).Scan(&nf)
		if n+nf > 0 {
			jsonError(w, fmt.Sprintf("%d connections and %d folders use this proxy. Choose another one for them first.", n, nf), 409)
			return
		}
		db.Exec(`DELETE FROM proxy_grants WHERE proxy_id=?`, id)
		db.Exec(`DELETE FROM proxies WHERE id=?`, id)
		auditLog(r, userID, "proxy.deleted", p.Name, map[string]interface{}{"proxy_id": id, "url": p.URL})
		notifyProxiesChanged(p, nil)
		jsonOK(w, map[string]bool{"ok": true})
	case action == "test" && r.Method == http.MethodPost:
		var in struct {
			Target string `json:"target"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		in.Target = strings.TrimSpace(in.Target)
		start := time.Now()
		rp, err := proxyOf(Connection{UserID: userID, ProxyID: &p.ID, Name: p.Name}, false)
		msg := ""
		if err == nil {
			if in.Target != "" {
				if _, _, e := net.SplitHostPort(in.Target); e != nil || !validHostName(hostOnly(in.Target)) {
					jsonError(w, "Target: use host:port", 400)
					return
				}
				var conn net.Conn
				if conn, err = rp.dial(directDial, in.Target); err == nil {
					conn.Close()
					msg = "Connected to " + in.Target + " through " + p.URL
				}
			} else if p.Kind == "wrm_tunnel" {
				var a string
				if a, err = ensureProxyTunnel(p); err == nil {
					msg = "The SOCKS tunnel runs on " + a
				}
			} else {
				var conn net.Conn
				if conn, err = directDial("tcp", net.JoinHostPort(p.Host, strconv.Itoa(p.Port))); err == nil {
					conn.Close()
					msg = "The proxy " + p.URL + " accepts connections"
				} else {
					err = &proxyError{label: p.URL, msg: "not reachable: " + shortNetError(err)}
				}
			}
		}
		ms := time.Since(start).Milliseconds()
		auditLog(r, userID, "proxy.tested", p.Name, map[string]interface{}{"proxy_id": id, "target": in.Target, "ok": err == nil})
		if err != nil {
			out := map[string]interface{}{"ok": false, "message": err.Error(), "latency_ms": ms}
			if warn := proxyWarning(p); warn != "" {
				out["warning"] = warn
			}
			jsonOK(w, out)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true, "message": msg, "latency_ms": ms})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func notifyProxiesChanged(p proxyDef, grants []int) {
	msg := []byte(`{"type":"proxies_changed"}`)
	if p.SharedAll {
		hub.sendAll(msg)
		return
	}
	hub.sendTo(p.OwnerID, msg)
	for _, g := range append(grants, p.Grants...) {
		if g != p.OwnerID {
			hub.sendTo(g, msg)
		}
	}
}

type proxyInput struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	ClearPassword bool   `json:"clear_password"`
	CredentialID  *int   `json:"credential_id"`
	RemoteDNS     bool   `json:"remote_dns"`
	TunnelID      *int   `json:"tunnel_id"`
	Description   string `json:"description"`
	SharedAll     bool   `json:"shared_all"`
	Grants        []int  `json:"grants"`
}

// normalizeProxyInput validates a proxy definition of userID.
func normalizeProxyInput(in *proxyInput, userID int) error {
	in.Name = truncateStr(strings.TrimSpace(in.Name), 120)
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	in.Host = strings.Trim(strings.TrimSpace(in.Host), "[]")
	in.Username = truncateStr(strings.TrimSpace(in.Username), 255)
	in.Description = truncateStr(in.Description, 2000)
	if in.Name == "" {
		return fmt.Errorf("Name is required")
	}
	if !proxyKinds[in.Kind] {
		return fmt.Errorf("Type: socks5, socks4, http or wrm_tunnel")
	}
	if in.CredentialID != nil && *in.CredentialID <= 0 {
		in.CredentialID = nil
	}
	if in.TunnelID != nil && *in.TunnelID <= 0 {
		in.TunnelID = nil
	}
	if len(in.Password) > 255 {
		return fmt.Errorf("Password too long (SOCKS5 allows 255 characters)")
	}
	if in.Kind == "wrm_tunnel" {
		if in.TunnelID == nil {
			return fmt.Errorf("Choose the SOCKS tunnel (-D) of one of your connections")
		}
		d, err := loadTunnelDef(*in.TunnelID)
		if err != nil || d.UserID != userID || d.Kind != "dynamic" {
			return fmt.Errorf("SOCKS tunnel not found (it must be a dynamic tunnel of your own connection)")
		}
		if in.SharedAll || len(in.Grants) > 0 {
			return fmt.Errorf("A WRM tunnel proxy uses your own SSH login: it cannot be shared")
		}
		in.Host, in.Port, in.Username, in.Password, in.CredentialID, in.RemoteDNS = "127.0.0.1", d.BindPort, "", "", nil, true
		return nil
	}
	in.TunnelID = nil
	if !validHostName(in.Host) || strings.Contains(in.Host, ":") && net.ParseIP(in.Host) == nil {
		return fmt.Errorf("Invalid proxy host")
	}
	if in.Port == 0 {
		in.Port = map[string]int{"socks5": 1080, "socks4": 1080, "http": 3128}[in.Kind]
	}
	if in.Port < 1 || in.Port > 65535 {
		return fmt.Errorf("Port: 1 to 65535")
	}
	if in.Kind == "socks4" {
		in.Password, in.CredentialID = "", nil // SOCKS4 knows a user id, no password
	}
	if in.CredentialID != nil {
		cr, err := loadCredential(*in.CredentialID)
		if err != nil || !credentialAccessible(cr, userID) {
			return fmt.Errorf("Credential not found")
		}
		if !hostAllowed(cr.Hosts, in.Host) {
			return fmt.Errorf("Credential %q may only be used for hosts matching %q", cr.Name, cr.Hosts)
		}
		in.Username, in.Password = "", ""
	}
	if len(in.Username) > 255 {
		return fmt.Errorf("User name too long")
	}
	return nil
}

func saveProxy(w http.ResponseWriter, r *http.Request, userID int, cur *proxyDef) {
	if !proxiesAllowed(userID) {
		jsonError(w, "Defining proxies is not allowed for this account (policy proxies)", 403)
		return
	}
	var in proxyInput
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	if err := normalizeProxyInput(&in, userID); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if in.SharedAll && !isAdminUser(userID) {
		jsonError(w, "Only administrators can share a proxy with everybody", 403)
		return
	}
	grants := []int{}
	seen := map[int]bool{userID: true}
	for _, g := range in.Grants {
		if g > 0 && !seen[g] {
			seen[g] = true
			var n int
			db.QueryRow(`SELECT COUNT(*) FROM users WHERE id=?`, g).Scan(&n)
			if n == 0 {
				jsonError(w, "Unknown user in the list of users", 400)
				return
			}
			grants = append(grants, g)
		}
	}
	password := ""
	if cur != nil {
		password = decryptValue(cur.password)
	}
	pwChanged := false
	if in.Password != "" || in.ClearPassword || in.CredentialID != nil || in.Kind == "socks4" || in.Kind == "wrm_tunnel" {
		pwChanged = in.Password != password
		password = in.Password
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var id int
	if cur == nil {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM proxies WHERE owner_id=?`, userID).Scan(&n)
		if n >= maxProxiesPerUser {
			jsonError(w, fmt.Sprintf("At most %d proxies per user", maxProxiesPerUser), 400)
			return
		}
		res, err := db.Exec(`INSERT INTO proxies (owner_id,name,kind,host,port,username,password,credential_id,remote_dns,tunnel_id,description,shared_all,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, userID, in.Name, in.Kind, in.Host, in.Port, in.Username, encryptValue(password), in.CredentialID,
			boolInt(in.RemoteDNS), in.TunnelID, in.Description, boolInt(in.SharedAll), now, now)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		nid, _ := res.LastInsertId()
		id = int(nid)
		saved, _ := loadProxy(id)
		auditLog(r, userID, "proxy.created", in.Name, map[string]interface{}{"proxy_id": id, "url": saved.URL, "login": in.Username != "" || in.CredentialID != nil,
			"shared_all": in.SharedAll, "granted_to": len(grants)})
	} else {
		id = cur.ID
		if _, err := db.Exec(`UPDATE proxies SET name=?, kind=?, host=?, port=?, username=?, password=?, credential_id=?, remote_dns=?, tunnel_id=?, description=?, shared_all=?, updated_at=? WHERE id=?`,
			in.Name, in.Kind, in.Host, in.Port, in.Username, encryptValue(password), in.CredentialID, boolInt(in.RemoteDNS), in.TunnelID, in.Description,
			boolInt(in.SharedAll), now, id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		changed := []string{}
		for f, same := range map[string]bool{"name": in.Name == cur.Name, "kind": in.Kind == cur.Kind, "host": in.Host == cur.Host, "port": in.Port == cur.Port,
			"username": in.Username == cur.Username, "password": !pwChanged, "login_ref": intPtrEq(in.CredentialID, cur.CredentialID),
			"remote_dns": in.RemoteDNS == cur.RemoteDNS, "tunnel": intPtrEq(in.TunnelID, cur.TunnelID), "shared_all": in.SharedAll == cur.SharedAll} {
			if !same {
				changed = append(changed, f)
			}
		}
		auditLog(r, userID, "proxy.updated", in.Name, map[string]interface{}{"proxy_id": id, "changed": changed, "granted_to": len(grants)})
		if len(changed) > 0 {
			restartProxyTunnels(id)
			statusMon.poke()
		}
	}
	old := map[int]bool{}
	if rows, err := db.Query(`SELECT user_id FROM proxy_grants WHERE proxy_id=?`, id); err == nil {
		for rows.Next() {
			var u int
			rows.Scan(&u)
			old[u] = true
		}
		rows.Close()
	}
	db.Exec(`DELETE FROM proxy_grants WHERE proxy_id=?`, id)
	for _, g := range grants {
		db.Exec(`INSERT OR IGNORE INTO proxy_grants (proxy_id, user_id) VALUES (?,?)`, id, g)
		if !old[g] {
			auditLog(r, userID, "proxy.granted", in.Name, map[string]interface{}{"proxy_id": id, "to": usernameOf(g)})
		}
		delete(old, g)
	}
	removed := []int{}
	for u := range old {
		removed = append(removed, u)
		auditLog(r, userID, "proxy.grant_revoked", in.Name, map[string]interface{}{"proxy_id": id, "from": usernameOf(u)})
	}
	saved, _ := loadProxy(id)
	notifyProxiesChanged(saved, append(grants, removed...))
	if cur != nil && cur.SharedAll && !in.SharedAll {
		hub.sendAll([]byte(`{"type":"proxies_changed"}`))
	}
	jsonOK(w, decorateProxy(saved, userID))
}

// restartProxyTunnels restarts running tunnels of connections that use a proxy (directly
// or through their jump hosts).
func restartProxyTunnels(proxyID int) {
	rows, err := db.Query(`SELECT id FROM connections WHERE proxy_id=? OR (proxy_id IS NULL AND folder_id IN (SELECT id FROM folders WHERE proxy_id=?))`, proxyID, proxyID)
	if err != nil {
		return
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		tunnelMgr.restartConn(id)
	}
}

func proxiesSummary() map[string]int {
	var total, shared int
	db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(shared_all),0) FROM proxies`).Scan(&total, &shared)
	return map[string]int{"proxies": total, "proxies_shared_all": shared}
}

// ─── EXPORT / IMPORT ─────────────────────────────────

type proxyExport struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"` // only in an export with secrets
	CredentialRef string `json:"credential_ref,omitempty"`
	RemoteDNS     bool   `json:"remote_dns"`
	TunnelID      int    `json:"tunnel_id,omitempty"` // "WRM SOCKS tunnel": id of the exported tunnel
	Description   string `json:"description,omitempty"`
}

func exportProxies(userID int, withSecrets bool) []proxyExport {
	out := []proxyExport{}
	rows, err := db.Query(`SELECT `+proxyCols+` FROM proxies WHERE owner_id=? ORDER BY name`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanProxy(rows)
		if err != nil {
			continue
		}
		e := proxyExport{Name: p.Name, Kind: p.Kind, Host: p.Host, Port: p.Port, Username: p.Username, RemoteDNS: p.RemoteDNS, Description: p.Description}
		if withSecrets {
			e.Password = decryptValue(p.password)
		}
		if p.CredentialID != nil {
			db.QueryRow(`SELECT name FROM credentials WHERE id=?`, *p.CredentialID).Scan(&e.CredentialRef)
		}
		if p.TunnelID != nil {
			e.TunnelID = *p.TunnelID
		}
		out = append(out, e)
	}
	return out
}

// proxyByName returns the id of a proxy userID can use: their own first, then shared ones.
func proxyByName(userID int, name string) int {
	var id int
	if db.QueryRow(`SELECT id FROM proxies WHERE owner_id=? AND name=? ORDER BY id LIMIT 1`, userID, name).Scan(&id) == nil {
		return id
	}
	for _, p := range proxiesForUser(userID) {
		if p.Name == name {
			return p.ID
		}
	}
	return 0
}

// importProxies creates the exported proxies the user does not have yet (by name). tunnels:
// the pass for "WRM SOCKS tunnel" proxies, with the ids of the imported tunnels.
func importProxies(r *http.Request, userID int, list []proxyExport, tunnels bool, newTunnels map[int]int) int {
	if !proxiesAllowed(userID) {
		return 0
	}
	n := 0
	for _, e := range list {
		if (e.Kind == "wrm_tunnel") != tunnels || proxyByName(userID, strings.TrimSpace(e.Name)) > 0 {
			continue
		}
		in := proxyInput{Name: e.Name, Kind: e.Kind, Host: e.Host, Port: e.Port, Username: e.Username, Password: e.Password, RemoteDNS: e.RemoteDNS, Description: e.Description}
		if e.CredentialRef != "" {
			for _, cr := range credentialsForUser(userID) {
				if cr.Name == e.CredentialRef {
					cid := cr.ID
					in.CredentialID = &cid
					break
				}
			}
		}
		if tunnels {
			nt, ok := newTunnels[e.TunnelID]
			if !ok {
				continue
			}
			in.TunnelID = &nt
		}
		if normalizeProxyInput(&in, userID) != nil {
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if res, err := db.Exec(`INSERT INTO proxies (owner_id,name,kind,host,port,username,password,credential_id,remote_dns,tunnel_id,description,shared_all,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,0,?,?)`, userID, in.Name, in.Kind, in.Host, in.Port, in.Username, encryptValue(in.Password), in.CredentialID,
			boolInt(in.RemoteDNS), in.TunnelID, in.Description, now, now); err == nil {
			id, _ := res.LastInsertId()
			p, _ := loadProxy(int(id))
			auditLog(r, userID, "proxy.created", in.Name, map[string]interface{}{"proxy_id": id, "url": p.URL, "imported": true})
			n++
		}
	}
	return n
}
