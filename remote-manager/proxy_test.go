// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testProxy is an in-process SOCKS5 or HTTP CONNECT proxy. With a user name it requires
// authentication. It records the targets it connected to.
type testProxy struct {
	addr       string
	user, pass string
	mu         sync.Mutex
	targets    []string
	conns      atomic.Int64
	refuse     string // target that gets "connection refused"
}

func (p *testProxy) record(t string) {
	p.mu.Lock()
	p.targets = append(p.targets, t)
	p.mu.Unlock()
}

func (p *testProxy) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.targets...)
}

func startTestProxy(t *testing.T, kind, user, pass string) *testProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	p := &testProxy{addr: ln.Addr().String(), user: user, pass: pass}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			p.conns.Add(1)
			if kind == "http" {
				go p.serveHTTP(c)
			} else {
				go p.serveSOCKS5(c)
			}
		}
	}()
	return p
}

func (p *testProxy) serveSOCKS5(c net.Conn) {
	defer c.Close()
	h := make([]byte, 2)
	if _, err := io.ReadFull(c, h); err != nil || h[0] != 5 {
		return
	}
	methods := make([]byte, h[1])
	io.ReadFull(c, methods)
	want := byte(0)
	if p.user != "" {
		want = 2
	}
	ok := false
	for _, m := range methods {
		ok = ok || m == want
	}
	if !ok {
		c.Write([]byte{5, 0xff})
		return
	}
	c.Write([]byte{5, want})
	if want == 2 {
		v := make([]byte, 2)
		io.ReadFull(c, v)
		u := make([]byte, v[1])
		io.ReadFull(c, u)
		l := make([]byte, 1)
		io.ReadFull(c, l)
		pw := make([]byte, l[0])
		io.ReadFull(c, pw)
		if string(u) != p.user || string(pw) != p.pass {
			c.Write([]byte{1, 1})
			return
		}
		c.Write([]byte{1, 0})
	}
	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return
	}
	var host string
	switch req[3] {
	case 1:
		b := make([]byte, 4)
		io.ReadFull(c, b)
		host = net.IP(b).String()
	case 3:
		l := make([]byte, 1)
		io.ReadFull(c, l)
		b := make([]byte, l[0])
		io.ReadFull(c, b)
		host = string(b)
	case 4:
		b := make([]byte, 16)
		io.ReadFull(c, b)
		host = net.IP(b).String()
	}
	pb := make([]byte, 2)
	io.ReadFull(c, pb)
	target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb))))
	p.record(target)
	out, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil || target == p.refuse {
		c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer out.Close()
	c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
	testPipe(c, out)
}

func (p *testProxy) serveHTTP(c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil || req.Method != http.MethodConnect {
		io.WriteString(c, "HTTP/1.1 405 Method Not Allowed\r\n\r\n")
		return
	}
	if p.user != "" {
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(p.user+":"+p.pass))
		if req.Header.Get("Proxy-Authorization") != want {
			io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"test\"\r\nContent-Length: 0\r\n\r\n")
			return
		}
	}
	p.record(req.Host)
	out, err := net.DialTimeout("tcp", req.Host, 5*time.Second)
	if err != nil {
		io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer out.Close()
	io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
	testPipe(c, out)
}

func (c *testClient) addProxy(name, kind string, p *testProxy) int {
	c.t.Helper()
	host, port, _ := net.SplitHostPort(p.addr)
	pn, _ := strconv.Atoi(port)
	var out proxyDef
	if code := c.jsonDo("POST", "/api/proxies", map[string]interface{}{"name": name, "kind": kind, "host": host, "port": pn,
		"username": p.user, "password": p.pass, "remote_dns": true}, &out); code != 200 || out.ID == 0 {
		c.t.Fatalf("create proxy %s: %d %+v", name, code, out)
	}
	return out.ID
}

func setProxy(connID, proxyID int) {
	db.Exec(`UPDATE connections SET proxy_id=? WHERE id=?`, proxyID, connID)
}

func TestProxyTerminalSFTPAndStatus(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "proxy-user", false)
	socks := startTestProxy(t, "socks5", "puser", "p-secret-5")
	httpp := startTestProxy(t, "http", "huser", "h-secret-7")
	sp := u.addProxy("socks-dc1", "socks5", socks)
	hp := u.addProxy("http-dc2", "http", httpp)

	// secrets are encrypted and never returned
	var list struct {
		Proxies []proxyDef `json:"proxies"`
	}
	u.jsonDo("GET", "/api/proxies", nil, &list)
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), "p-secret-5") || strings.Contains(string(raw), "h-secret-7") || len(list.Proxies) != 2 || !list.Proxies[0].HasPassword {
		t.Fatalf("proxy list: %s", raw)
	}
	var stored string
	db.QueryRow(`SELECT password FROM proxies WHERE id=?`, sp).Scan(&stored)
	if strings.Contains(stored, "p-secret-5") {
		t.Fatalf("proxy password stored in plain text")
	}

	// terminal through SOCKS5 with authentication
	app := u.addConnection("app-01", sshAddr)
	setProxy(app, sp)
	tc := u.openTerminal(app)
	tc.waitFor("$ ")
	tc.send("exit\r")
	<-tc.closed
	if s := socks.seen(); len(s) == 0 || s[0] != sshAddr {
		t.Fatalf("SOCKS5 proxy targets: %q", s)
	}
	var sid int64
	var route string
	db.QueryRow(`SELECT MAX(id) FROM terminal_sessions WHERE conn_id=?`, app).Scan(&sid)
	waitSessionEnded(t, sid)
	db.QueryRow(`SELECT jump_path FROM terminal_sessions WHERE id=?`, sid).Scan(&route)
	if route != "socks5://"+socks.addr {
		t.Fatalf("route of the session: %q", route)
	}
	var views []connView
	u.jsonDo("GET", "/api/connections", nil, &views)
	for _, v := range views {
		if v.ID == app && (v.Route != "socks5://"+socks.addr || v.ProxyName != "socks-dc1" || v.ProxyID == nil) {
			t.Fatalf("connection view: %+v", v)
		}
	}

	// SFTP through HTTP CONNECT with Basic authentication
	files := u.addConnection("files-01", sshAddr)
	setProxy(files, hp)
	resp := u.do("GET", "/api/remote/list?id="+strconv.Itoa(files)+"&path="+t.TempDir(), nil, "")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || len(httpp.seen()) == 0 {
		t.Fatalf("SFTP through the HTTP proxy: %d, targets %q", resp.StatusCode, httpp.seen())
	}

	// connection test and the route in its message
	var res map[string]interface{}
	u.jsonDo("POST", "/api/connections/test", map[string]interface{}{"id": files, "name": "files-01", "host": sshAddr, "username": testSSHUser,
		"auth_method": "PASSWORD", "protocol": "SSH", "proxy_id": hp}, &res)
	if res["ok"] != true || !strings.Contains(fmt.Sprint(res["message"]), "via http://"+httpp.addr) {
		t.Fatalf("connection test: %v", res)
	}

	// live status through the proxy (SSH banner read through SOCKS5)
	before := len(socks.seen())
	statusMon.round()
	var st struct {
		Connections map[string]hostStatus `json:"connections"`
	}
	u.jsonDo("GET", "/api/status", nil, &st)
	if s := st.Connections[strconv.Itoa(app)]; s.State != "up" || !strings.HasPrefix(s.Banner, "SSH-2.0-") || len(socks.seen()) == before {
		t.Fatalf("status through the proxy: %+v", s)
	}
	var now struct {
		Connections map[string]hostStatus `json:"connections"`
	}
	u.jsonDo("POST", "/api/status/check", map[string]interface{}{"ids": []int{files}}, &now)
	if s := now.Connections[strconv.Itoa(files)]; s.State != "up" || s.Via != "http://"+httpp.addr {
		t.Fatalf("check now through the proxy: %+v", s)
	}

	// duplicate keeps the proxy; export and import keep it by name
	var dup connView
	u.jsonDo("POST", "/api/connections/"+strconv.Itoa(app)+"/duplicate", nil, &dup)
	if dup.ProxyID == nil || *dup.ProxyID != sp {
		t.Fatalf("duplicate lost the proxy: %+v", dup.ProxyID)
	}
	resp = u.do("GET", "/api/config/export", nil, "")
	exp, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(exp), "p-secret-5") || !strings.Contains(string(exp), `"proxy_ref":"socks-dc1"`) || !strings.Contains(string(exp), `"proxies":[`) {
		t.Fatalf("export: %s", exp)
	}
	other := newTestUser(t, srv, "proxy-importer", false)
	var imp map[string]int
	if code := other.jsonDo("POST", "/api/config/import", json.RawMessage(exp), &imp); code != 200 || imp["proxies"] != 2 {
		t.Fatalf("import: %d %v", code, imp)
	}
	var linked int
	db.QueryRow(`SELECT COUNT(*) FROM connections c JOIN proxies p ON p.id=c.proxy_id WHERE c.user_id=? AND p.owner_id=? AND p.name='socks-dc1'`, other.userID, other.userID).Scan(&linked)
	if linked < 2 {
		t.Fatalf("imported connections not linked to the imported proxy: %d", linked)
	}

	// deleting a proxy in use is refused
	if code := u.jsonDo("DELETE", "/api/proxies/"+strconv.Itoa(sp), nil, nil); code != 409 {
		t.Fatalf("delete of a proxy in use: %d", code)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='proxy.created' AND details LIKE '%socks5://%'`).Scan(&n)
	if n == 0 {
		t.Fatalf("proxy creation not audited with its address")
	}
}

func TestProxyErrors(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "proxy-errors", false)
	socks := startTestProxy(t, "socks5", "puser", "right")
	httpp := startTestProxy(t, "http", "huser", "right")
	test := func(proxyID int, host string) string {
		var res map[string]interface{}
		u.jsonDo("POST", "/api/connections/test", map[string]interface{}{"name": "t", "host": host, "username": testSSHUser, "password": testSSHPass,
			"auth_method": "PASSWORD", "protocol": "SSH", "proxy_id": proxyID}, &res)
		if res["ok"] == true {
			t.Fatalf("test through proxy %d to %s succeeded", proxyID, host)
		}
		return fmt.Sprint(res["message"])
	}
	host, port, _ := net.SplitHostPort(socks.addr)
	pn, _ := strconv.Atoi(port)
	mk := func(kind, user, pass, h string, p int) int {
		var out proxyDef
		if code := u.jsonDo("POST", "/api/proxies", map[string]interface{}{"name": kind + user + pass + h, "kind": kind, "host": h, "port": p, "username": user, "password": pass, "remote_dns": true}, &out); code != 200 {
			t.Fatalf("create proxy: %d", code)
		}
		return out.ID
	}
	if m := test(mk("socks5", "puser", "wrong", host, pn), sshAddr); !strings.Contains(m, "proxy socks5://"+socks.addr+": authentication failed") {
		t.Fatalf("wrong SOCKS password: %q", m)
	}
	if m := test(mk("socks5", "", "", host, pn), sshAddr); !strings.Contains(m, "authentication required") {
		t.Fatalf("SOCKS without login: %q", m)
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := l.Addr().String()
	l.Close()
	if m := test(mk("socks5", "puser", "right", host, pn), closed); !strings.Contains(m, "cannot reach "+closed) || !strings.Contains(m, "connection refused (SOCKS5 reply 5)") {
		t.Fatalf("target refused through SOCKS: %q", m)
	}
	ch, cp, _ := net.SplitHostPort(closed)
	cpn, _ := strconv.Atoi(cp)
	if m := test(mk("socks5", "x", "y", ch, cpn), sshAddr); !strings.Contains(m, "proxy socks5://"+closed+": not reachable: connection refused") {
		t.Fatalf("proxy unreachable: %q", m)
	}
	hh, hport, _ := net.SplitHostPort(httpp.addr)
	hpn, _ := strconv.Atoi(hport)
	if m := test(mk("http", "huser", "bad", hh, hpn), sshAddr); !strings.Contains(m, "authentication failed") || !strings.Contains(m, "407") {
		t.Fatalf("HTTP 407: %q", m)
	}
	if m := test(mk("http", "huser", "right", hh, hpn), closed); !strings.Contains(m, "cannot reach "+closed) || !strings.Contains(m, "502") {
		t.Fatalf("HTTP 502: %q", m)
	}

	// policy: who may define proxies; using shared ones stays possible
	setSetting("proxies", "admins")
	if code := u.jsonDo("POST", "/api/proxies", map[string]interface{}{"name": "x", "kind": "socks5", "host": "127.0.0.1", "port": 1080}, nil); code != 403 {
		t.Fatalf("policy proxies=admins: %d", code)
	}
	setSetting("proxies", "all")

	// Docker: a proxy on 127.0.0.1 inside a container gets a warning
	inContainer = func() bool { return true }
	var pd proxyDef
	u.jsonDo("POST", "/api/proxies", map[string]interface{}{"name": "local", "kind": "socks5", "host": "127.0.0.1", "port": 1080}, &pd)
	inContainer = func() bool { return false }
	if pd.Warning != "docker_loopback" {
		t.Fatalf("no Docker warning: %+v", pd)
	}

	// sharing: grantees use it without seeing the login; with a password not through their jump hosts
	shared := mk("socks5", "puser", "right", host, pn)
	other := newTestUser(t, srv, "proxy-grantee", false)
	var cur proxyDef
	u.jsonDo("GET", "/api/proxies", nil, &map[string]interface{}{})
	if code := u.jsonDo("PUT", "/api/proxies/"+strconv.Itoa(shared), map[string]interface{}{"name": "shared-socks", "kind": "socks5", "host": host, "port": pn,
		"username": "puser", "remote_dns": true, "grants": []int{other.userID}}, &cur); code != 200 || len(cur.Grants) != 1 || !cur.HasPassword {
		t.Fatalf("share proxy: %d %+v", code, cur)
	}
	var theirs struct {
		Proxies []proxyDef `json:"proxies"`
	}
	other.jsonDo("GET", "/api/proxies", nil, &theirs)
	if len(theirs.Proxies) != 1 || theirs.Proxies[0].Username != "" || theirs.Proxies[0].Mine {
		t.Fatalf("grantee view: %+v", theirs.Proxies)
	}
	var cv connView
	if code := other.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "g-app", "protocol": "SSH", "host": sshAddr, "username": testSSHUser,
		"auth_method": "PASSWORD", "password": testSSHPass, "proxy_id": shared}, &cv); code != 201 {
		t.Fatalf("grantee connection with the shared proxy: %d", code)
	}
	tc := other.openTerminal(cv.ID)
	tc.waitFor("$ ")
	tc.send("exit\r")
	<-tc.closed
	bastion := other.addConnection("g-bastion", sshAddr)
	if code := other.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "g-app2", "protocol": "SSH", "host": sshAddr, "username": testSSHUser,
		"auth_method": "PASSWORD", "proxy_id": shared, "jump_id": bastion}, nil); code != 400 {
		t.Fatalf("shared proxy with password through the grantee's jump host: %d", code)
	}
	if code := other.jsonDo("PUT", "/api/proxies/"+strconv.Itoa(shared), map[string]interface{}{"name": "mine", "kind": "socks5", "host": host, "port": pn}, nil); code != 403 {
		t.Fatalf("grantee changed the proxy: %d", code)
	}
	// IPMI over UDP cannot use the proxy
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "ipmi-px", "protocol": "SSH", "host": sshAddr, "username": testSSHUser,
		"auth_method": "PASSWORD", "proxy_id": shared, "bmc": map[string]interface{}{"type": "ipmi", "host": "10.0.0.9", "username": "admin", "password": "x", "via_jump": true}}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("IPMI through a proxy accepted: %d", code)
	}
}

func TestProxyJumpHostWebDesktopAndTunnel(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "proxy-route", false)
	socks := startTestProxy(t, "socks5", "", "")
	sp := u.addProxy("dc-socks", "socks5", socks)

	// jump host + proxy: the proxy is reached through the jump host
	bastion := u.addConnection("bastion", sshAddr)
	app := u.addConnectionVia("app-02", sshAddr, bastion)
	setProxy(app, sp)
	before := directTCPIPCount.Load()
	tc := u.openTerminal(app)
	tc.waitFor("$ ")
	tc.waitFor("bastion → socks5://" + socks.addr)
	tc.send("exit\r")
	<-tc.closed
	if directTCPIPCount.Load() == before || len(socks.seen()) == 0 {
		t.Fatalf("proxy not reached through the jump host")
	}
	var sid int64
	db.QueryRow(`SELECT MAX(id) FROM terminal_sessions WHERE conn_id=?`, app).Scan(&sid)
	waitSessionEnded(t, sid)
	var route string
	db.QueryRow(`SELECT jump_path FROM terminal_sessions WHERE id=?`, sid).Scan(&route)
	c, _ := loadConnection(app)
	if route != "bastion → socks5://"+socks.addr || fullRoute(c) != "bastion → socks5://"+socks.addr+" → app-02" {
		t.Fatalf("route: %q / %q", route, fullRoute(c))
	}

	// web interface through the proxy: a local relay on 127.0.0.1
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "hello from the web ui") }))
	defer web.Close()
	var wc connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "web-ui", "protocol": "HTTP", "host": strings.TrimPrefix(web.URL, "http://"),
		"auth_method": "PASSWORD", "proxy_id": sp}, &wc); code != 201 {
		t.Fatalf("create web connection: %d", code)
	}
	var open map[string]interface{}
	if code := u.jsonDo("POST", "/api/connections/"+strconv.Itoa(wc.ID)+"/open-web", nil, &open); code != 200 || open["direct"] != false {
		t.Fatalf("open web through the proxy: %d %v", code, open)
	}
	if body := httpGetBody(t, fmt.Sprint(open["url"])); body != "hello from the web ui" {
		t.Fatalf("web interface through the proxy: %q", body)
	}
	if s := strings.Join(socks.seen(), " "); !strings.Contains(s, strings.TrimPrefix(web.URL, "http://")) {
		t.Fatalf("web traffic did not go through the proxy: %q", s)
	}

	// remote desktop relay through the proxy
	g := startFakeGuacd(t, true)
	setSetting("guacd_address", g.addr)
	defer setSetting("guacd_address", "127.0.0.1:4822")
	vnc, _ := net.Listen("tcp", "127.0.0.1:0")
	defer vnc.Close()
	go func() {
		for {
			c, err := vnc.Accept()
			if err != nil {
				return
			}
			c.Write([]byte("RFB 003.008\n"))
			time.Sleep(200 * time.Millisecond)
			c.Close()
		}
	}()
	var vc connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "vnc-px", "protocol": "VNC", "host": vnc.Addr().String(),
		"auth_method": "PASSWORD", "password": "vncpw", "proxy_id": sp}, &vc); code != 201 {
		t.Fatalf("create VNC connection: %d", code)
	}
	dc := u.openDesktop(vc.ID)
	waitUntil(t, "VNC through the proxy relay", func() bool { return strings.Contains(dc.text(), "wrm-test-target") })
	if !strings.Contains(dc.text(), "RFB 003.008") || !strings.Contains(strings.Join(socks.seen(), " "), vnc.Addr().String()) {
		t.Fatalf("desktop relay through the proxy: %q", dc.text())
	}

	// "WRM SOCKS tunnel of connection X": started when needed
	gw := u.addConnection("socks-gw", sshAddr)
	if code := u.jsonDo("PUT", "/api/connections/"+strconv.Itoa(gw)+"/tunnels", []map[string]interface{}{{"name": "dyn", "kind": "dynamic", "bind_host": "127.0.0.1", "bind_port": 0}}, &[]tunnelDef{}); code != 200 {
		t.Fatalf("create dynamic tunnel: %d", code)
	}
	defs := loadTunnelDefs("conn_id=?", gw)
	if len(defs) != 1 {
		t.Fatalf("tunnel defs: %+v", defs)
	}
	td := defs[0]
	var tp proxyDef
	if code := u.jsonDo("POST", "/api/proxies", map[string]interface{}{"name": "via-gw", "kind": "wrm_tunnel", "tunnel_id": td.ID}, &tp); code != 200 || tp.TunnelConn != "socks-gw" {
		t.Fatalf("create tunnel proxy: %d %+v", code, tp)
	}
	if code := u.jsonDo("PUT", "/api/proxies/"+strconv.Itoa(tp.ID), map[string]interface{}{"name": "via-gw", "kind": "wrm_tunnel", "tunnel_id": td.ID, "shared_all": false, "grants": []int{1}}, nil); code != 400 {
		t.Fatalf("sharing a tunnel proxy accepted: %d", code)
	}
	behind := u.addConnection("behind-gw", sshAddr)
	setProxy(behind, tp.ID)
	if tunnelMgr.get("t"+strconv.Itoa(td.ID)) != nil {
		t.Fatalf("tunnel running before it is needed")
	}
	tc = u.openTerminal(behind)
	tc.waitFor("$ ")
	tc.send("exit\r")
	<-tc.closed
	tr := tunnelMgr.get("t" + strconv.Itoa(td.ID))
	if tr == nil || tr.reason != "proxy" || tr.total.Load() == 0 {
		t.Fatalf("SOCKS tunnel not started for the proxy: %+v", tr)
	}
	tunnelMgr.stop("t"+strconv.Itoa(td.ID), 0, "test")
	if code := u.jsonDo("PUT", "/api/connections/"+strconv.Itoa(behind), map[string]interface{}{"name": "behind-gw", "protocol": "SSH", "host": sshAddr,
		"username": testSSHUser, "auth_method": "PASSWORD", "proxy_id": tp.ID, "jump_id": bastion}, nil); code != 400 {
		t.Fatalf("tunnel proxy with a jump host accepted: %d", code)
	}
	if code := u.jsonDo("PUT", "/api/connections/"+strconv.Itoa(gw), map[string]interface{}{"name": "socks-gw", "protocol": "SSH", "host": sshAddr,
		"username": testSSHUser, "auth_method": "PASSWORD", "proxy_id": tp.ID}, nil); code != 400 {
		t.Fatalf("connection using its own tunnel as proxy accepted: %d", code)
	}
}

func TestFolderDefaults(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "folder-defaults", false)
	socks := startTestProxy(t, "socks5", "", "")
	sp := u.addProxy("fd-socks", "socks5", socks)
	bastion := u.addConnection("fd-bastion", sshAddr)

	var f Folder
	if code := u.jsonDo("POST", "/api/folders", map[string]interface{}{"name": "DC1", "jump_id": bastion, "proxy_id": sp}, &f); code != 201 || f.JumpName != "fd-bastion" || f.ProxyName != "fd-socks" {
		t.Fatalf("create folder with defaults: %d %+v", code, f)
	}
	db.Exec(`UPDATE connections SET folder_id=? WHERE id=?`, f.ID, bastion) // the jump host itself lives in the folder too

	// a new connection in the folder inherits both
	var cv connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "fd-app", "protocol": "SSH", "host": sshAddr, "username": testSSHUser,
		"auth_method": "PASSWORD", "password": testSSHPass, "folder_id": f.ID}, &cv); code != 201 {
		t.Fatalf("create connection: %d", code)
	}
	if cv.JumpID == nil || *cv.JumpID != bastion || !cv.JumpFolder || cv.JumpChoice != nil || cv.ProxyID == nil || *cv.ProxyID != sp || !cv.ProxyFolder ||
		cv.Route != "socks5://"+socks.addr+" → fd-bastion → socks5://"+socks.addr { // the bastion is in the folder too: it uses the folder proxy
		t.Fatalf("inherited route: %+v", cv)
	}
	tc := u.openTerminal(cv.ID)
	tc.waitFor("$ ")
	tc.send("exit\r")
	<-tc.closed
	if len(socks.seen()) == 0 {
		t.Fatalf("inherited proxy not used")
	}
	// the jump host does not inherit itself (but does get the folder proxy)
	jb, _ := loadConnection(bastion)
	if jb.JumpID != nil || jb.ProxyID == nil {
		t.Fatalf("jump host in its own folder: jump %v proxy %v", jb.JumpID, jb.ProxyID)
	}

	// own choices win: none (-1) and another jump host
	var own connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "fd-direct", "protocol": "SSH", "host": sshAddr, "username": testSSHUser,
		"auth_method": "PASSWORD", "password": testSSHPass, "folder_id": f.ID, "jump_id": -1, "proxy_id": -1}, &own); code != 201 {
		t.Fatalf("create direct connection: %d", code)
	}
	if own.JumpID != nil || own.ProxyID != nil || own.JumpChoice == nil || *own.JumpChoice != -1 || own.Route != "" {
		t.Fatalf("own choice 'none': %+v", own)
	}
	var stored *int
	db.QueryRow(`SELECT jump_conn_id FROM connections WHERE id=?`, own.ID).Scan(&stored)
	if stored == nil || *stored != -1 {
		t.Fatalf("stored choice: %v", stored)
	}

	// status follows the effective route (checked behind the jump host: state of the jump host)
	setSetting("status_jump_checks", "0")
	statusMon.round()
	var st struct {
		Connections map[string]hostStatus `json:"connections"`
	}
	u.jsonDo("GET", "/api/status", nil, &st)
	if s := st.Connections[strconv.Itoa(cv.ID)]; s.Via != "fd-bastion" {
		t.Fatalf("status of an inheriting connection: %+v", s)
	}
	if s := st.Connections[strconv.Itoa(own.ID)]; s.State != "up" || s.Via != "" {
		t.Fatalf("status of a direct connection: %+v", s)
	}

	// changing the folder is audited; removing the defaults changes the route
	if code := u.jsonDo("PUT", "/api/folders/"+strconv.Itoa(f.ID), map[string]interface{}{"name": "DC1", "jump_id": bastion}, &f); code != 200 || f.ProxyID != nil {
		t.Fatalf("update folder: %d %+v", code, f)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='folder.updated' AND details LIKE '%default_proxy%'`).Scan(&n)
	if n == 0 {
		t.Fatalf("folder change not audited")
	}
	c, _ := loadConnection(cv.ID)
	if c.ProxyID != nil || c.JumpID == nil {
		t.Fatalf("route after the folder change: jump %v proxy %v", c.JumpID, c.ProxyID)
	}
	// a folder default that leads back to the connection is refused
	other := u.addConnection("fd-other", sshAddr)
	db.Exec(`UPDATE connections SET jump_conn_id=? WHERE id=?`, cv.ID, other)
	if code := u.jsonDo("PUT", "/api/connections/"+strconv.Itoa(cv.ID), map[string]interface{}{"name": "fd-app", "protocol": "SSH", "host": sshAddr,
		"username": testSSHUser, "auth_method": "PASSWORD", "folder_id": f.ID, "jump_id": nil}, nil); code != 200 {
		t.Fatalf("plain update: %d", code)
	}
	var f2 Folder
	if code := u.jsonDo("POST", "/api/folders", map[string]interface{}{"name": "loop", "jump_id": other}, &f2); code != 201 {
		t.Fatalf("folder 2: %d", code)
	}
	if code := u.jsonDo("PUT", "/api/connections/"+strconv.Itoa(cv.ID), map[string]interface{}{"name": "fd-app", "protocol": "SSH", "host": sshAddr,
		"username": testSSHUser, "auth_method": "PASSWORD", "folder_id": f2.ID}, nil); code != 400 {
		t.Fatalf("folder default leading back to the connection accepted: %d", code)
	}
	db.Exec(`UPDATE connections SET jump_conn_id=NULL WHERE id=?`, other)

	// export and import keep the defaults (jump host by connection, proxy by name)
	u.jsonDo("PUT", "/api/folders/"+strconv.Itoa(f.ID), map[string]interface{}{"name": "DC1", "jump_id": bastion, "proxy_id": sp}, nil)
	resp := u.do("GET", "/api/config/export", nil, "")
	exp, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	imp := newTestUser(t, srv, "folder-importer", false)
	if code := imp.jsonDo("POST", "/api/config/import", json.RawMessage(exp), nil); code != 200 {
		t.Fatalf("import: %d", code)
	}
	var jn, pn, ch string
	db.QueryRow(`SELECT COALESCE(j.name,''), COALESCE(p.name,'') FROM folders f LEFT JOIN connections j ON j.id=f.jump_conn_id LEFT JOIN proxies p ON p.id=f.proxy_id
		WHERE f.user_id=? AND f.name='DC1'`, imp.userID).Scan(&jn, &pn)
	db.QueryRow(`SELECT COALESCE(jump_conn_id,0) || '/' || COALESCE(proxy_id,0) FROM connections WHERE user_id=? AND name='fd-direct'`, imp.userID).Scan(&ch)
	if jn != "fd-bastion" || pn != "fd-socks" || ch != "-1/-1" {
		t.Fatalf("imported folder defaults: jump %q proxy %q, direct connection %q", jn, pn, ch)
	}
}
