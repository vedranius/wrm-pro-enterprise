// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestGuacCodec(t *testing.T) {
	raw := guacEncode("clipboard", "č😀x", "", "a,b;c")
	if string(raw) != "9.clipboard,3.č😀x,0.,5.a,b;c;" {
		t.Fatalf("encode: %q", raw)
	}
	ins, raws, err := parseGuacMessage(append(raw, guacEncode("sync", "42")...))
	if err != nil || len(ins) != 2 || ins[0][1] != "č😀x" || ins[0][3] != "a,b;c" || ins[1][0] != "sync" || string(raws[1]) != "4.sync,2.42;" {
		t.Fatalf("parse: %v %q %v", ins, raws, err)
	}
	for _, bad := range []string{"4.sync", "x.sync;", "4.syn;", "4.sync,2.42"} {
		if _, _, err := parseGuacMessage([]byte(bad)); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if m := guacErrorText([]string{"error", "Aborted", "769"}); !strings.Contains(m, "wrong user name or password") {
		t.Fatalf("error text: %q", m)
	}
}

// fakeGuacd answers the handshake like guacd 1.3, reports the connect parameters and the
// instructions it receives, and (if dialTarget) connects to hostname:port and reports
// the first bytes it reads there.
type fakeGuacd struct {
	addr     string
	mu       sync.Mutex
	params   map[string]string
	received []string
	sizes    []string
}

func startFakeGuacd(t *testing.T, dialTarget bool) *fakeGuacd {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	f := &fakeGuacd{addr: l.Addr().String()}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go f.serve(c, dialTarget)
		}
	}()
	return f
}

func (f *fakeGuacd) serve(c net.Conn, dialTarget bool) {
	defer c.Close()
	gr := newGuacReader(c)
	_, el, err := gr.next()
	if err != nil || el[0] != "select" {
		return
	}
	names := []string{"VERSION_1_3_0", "hostname", "port", "username", "password", "domain", "security", "ignore-cert", "read-only"}
	c.Write(guacEncode("args", names...))
	params := map[string]string{}
	for {
		_, el, err := gr.next()
		if err != nil {
			return
		}
		if el[0] == "size" {
			f.mu.Lock()
			f.sizes = append(f.sizes, strings.Join(el[1:], "x"))
			f.mu.Unlock()
		}
		if el[0] == "connect" {
			for i, n := range names {
				if i+1 < len(el) {
					params[n] = el[i+1]
				}
			}
			break
		}
	}
	f.mu.Lock()
	f.params = params
	f.mu.Unlock()
	if params["password"] == "wrong" {
		c.Write(guacEncode("error", "Authentication failure", "769"))
		return
	}
	c.Write(guacEncode("ready", "$fake-id"))
	c.Write(append(guacEncode("size", "0", "1024", "768"), guacEncode("sync", "1000")...))
	if dialTarget {
		tc, err := net.DialTimeout("tcp", net.JoinHostPort(params["hostname"], params["port"]), 5*time.Second)
		if err == nil {
			tc.SetReadDeadline(time.Now().Add(5 * time.Second))
			line, _ := bufio.NewReader(tc).ReadString('\n')
			tc.Close()
			c.Write(guacEncode("wrm-test-target", strings.TrimSpace(line)))
		} else {
			c.Write(guacEncode("wrm-test-target", "dial error: "+err.Error()))
		}
	}
	for {
		_, el, err := gr.next()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.received = append(f.received, strings.Join(el, " "))
		f.mu.Unlock()
		if el[0] == "key" {
			c.Write(guacEncode("sync", "2000"))
		}
	}
}

func (f *fakeGuacd) got(s string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.received {
		if r == s {
			return true
		}
	}
	return false
}

type desktopClient struct {
	ws  *websocket.Conn
	mu  sync.Mutex
	out strings.Builder
	end chan struct{}
}

func (c *testClient) openDesktop(connID int) *desktopClient {
	c.t.Helper()
	u := "ws" + strings.TrimPrefix(c.srv.URL, "http") + "/ws/desktop?id=" + strconv.Itoa(connID) + "&width=1280&height=720&dpi=96&tz=Europe/Zagreb"
	hdr := http.Header{}
	hdr.Set("Origin", c.srv.URL)
	hdr.Set("Cookie", c.cookie.Name+"="+c.cookie.Value)
	d := websocket.Dialer{Subprotocols: []string{"guacamole"}}
	ws, resp, err := d.Dial(u, hdr)
	if err != nil {
		c.t.Fatal(err)
	}
	if resp.Header.Get("Sec-WebSocket-Protocol") != "guacamole" {
		c.t.Fatalf("subprotocol not accepted: %q", resp.Header.Get("Sec-WebSocket-Protocol"))
	}
	dc := &desktopClient{ws: ws, end: make(chan struct{})}
	go func() {
		defer close(dc.end)
		for {
			_, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if _, _, perr := parseGuacMessage(msg); perr != nil {
				dc.mu.Lock()
				dc.out.WriteString("<<INCOMPLETE MESSAGE>>")
				dc.mu.Unlock()
			}
			dc.mu.Lock()
			dc.out.Write(msg)
			dc.mu.Unlock()
		}
	}()
	return dc
}

// wait waits until the server closes the desktop WebSocket.
func (dc *desktopClient) wait(t *testing.T) {
	t.Helper()
	select {
	case <-dc.end:
	case <-time.After(15 * time.Second):
		t.Fatalf("desktop WebSocket not closed by the server; received: %q", dc.text())
	}
}

func (dc *desktopClient) text() string {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	return dc.out.String()
}

func TestDesktopSession(t *testing.T) {
	g := startFakeGuacd(t, false)
	setSetting("guacd_address", g.addr)
	defer setSetting("guacd_address", "127.0.0.1:4822")
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "rdp-user", false)
	other := newTestUser(t, srv, "rdp-other", false)

	var cv connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "win-01", "protocol": "RDP", "host": "10.9.8.7:3390",
		"username": "administrator", "auth_method": "PASSWORD", "password": "Secret!", "options": map[string]string{"domain": "CORP", "security": "nla", "bogus": "x"}}, &cv); code != 201 {
		t.Fatalf("create RDP connection: %d", code)
	}
	if cv.Options["domain"] != "CORP" || cv.Options["bogus"] != "" {
		t.Fatalf("options: %v", cv.Options)
	}
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "bad", "protocol": "RDP", "host": "10.9.8.7",
		"auth_method": "PASSWORD", "options": map[string]string{"security": "plaintext"}}, nil); code != 400 {
		t.Fatalf("invalid option accepted: %d", code)
	}

	dc := u.openDesktop(cv.ID)
	waitUntil(t, "first instructions", func() bool { return strings.Contains(dc.text(), "4.sync,4.1000;") })
	txt := dc.text()
	if !strings.HasPrefix(txt, "0.,") || !strings.Contains(txt, "11.wrm-session,") {
		t.Fatalf("stream start: %q", txt)
	}
	if strings.Contains(txt, "INCOMPLETE") {
		t.Fatalf("a WebSocket message ended in the middle of an instruction")
	}
	g.mu.Lock()
	p := g.params
	sizes := g.sizes
	g.mu.Unlock()
	if p["hostname"] != "10.9.8.7" || p["port"] != "3390" || p["username"] != "administrator" || p["password"] != "Secret!" ||
		p["domain"] != "CORP" || p["security"] != "nla" || p["ignore-cert"] != "true" || p["VERSION_1_3_0"] != "VERSION_1_3_0" {
		t.Fatalf("connect parameters: %v", p)
	}
	if len(sizes) == 0 || sizes[0] != "1280x720x96" {
		t.Fatalf("size: %v", sizes)
	}
	// input passes, handshake instructions do not; pings are answered by WRM
	dc.ws.WriteMessage(websocket.TextMessage, guacEncode("key", "65307", "1"))
	dc.ws.WriteMessage(websocket.TextMessage, guacEncode("select", "ssh"))
	dc.ws.WriteMessage(websocket.TextMessage, guacEncode("", "ping", "1234"))
	dc.ws.WriteMessage(websocket.TextMessage, []byte("garbage"))
	waitUntil(t, "key at guacd", func() bool { return g.got("key 65307 1") })
	waitUntil(t, "pong", func() bool { return strings.Contains(dc.text(), "0.,4.ping,4.1234;") })
	time.Sleep(100 * time.Millisecond)
	if g.got("select ssh") {
		t.Fatalf("handshake instruction was forwarded to guacd")
	}
	// another user cannot open it
	odc := other.openDesktop(cv.ID)
	odc.wait(t)
	if !strings.Contains(odc.text(), "5.error,") {
		t.Fatalf("other user got: %q", odc.text())
	}
	dc.ws.Close()
	<-dc.end

	// session log and recording
	var sid int64
	var proto, status string
	waitUntil(t, "session closed", func() bool {
		db.QueryRow(`SELECT id, protocol, status FROM terminal_sessions WHERE conn_id=? ORDER BY id DESC LIMIT 1`, cv.ID).Scan(&sid, &proto, &status)
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM session_recordings WHERE session_id=?`, sid).Scan(&n)
		return status == "closed" && n == 1
	})
	if proto != "rdp" {
		t.Fatalf("session protocol %q", proto)
	}
	var rel, format string
	db.QueryRow(`SELECT path, format FROM session_recordings WHERE session_id=?`, sid).Scan(&rel, &format)
	if format != guacRecordingFormat {
		t.Fatalf("recording format %q", format)
	}
	f, err := os.Open(filepath.Join(recordingsDir(), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := gzip.NewReader(f)
	data, _ := io.ReadAll(zr)
	f.Close()
	if !strings.Contains(string(data), "4.sync,4.1000;") || strings.Contains(string(data), "wrm-session") || strings.Contains(string(data), "Secret!") {
		t.Fatalf("recording content: %q", data)
	}
	resp := u.do("GET", "/api/recordings/"+strconv.FormatInt(sid, 10)+"/guac", nil, "")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != string(data) {
		t.Fatalf("recording download: %d", resp.StatusCode)
	}
	if resp := u.do("GET", "/api/recordings/"+strconv.FormatInt(sid, 10)+"/cast", nil, ""); resp.StatusCode != 404 {
		t.Fatalf("guac recording served as asciicast: %d", resp.StatusCode)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action IN ('desktop.open','desktop.close') AND session_id=?`, sid).Scan(&n)
	if n != 2 {
		t.Fatalf("desktop audit entries: %d", n)
	}

	// wrong password: guacd's error reaches the browser, the session is marked failed
	db.Exec(`UPDATE connections SET password=? WHERE id=?`, encryptValue("wrong"), cv.ID)
	dc2 := u.openDesktop(cv.ID)
	dc2.wait(t)
	if !strings.Contains(dc2.text(), "wrong user name or password") {
		t.Fatalf("login error not shown: %q", dc2.text())
	}

	// terminals, file manager and transfers refuse desktop connections
	if resp := u.do("GET", "/api/remote/list?id="+strconv.Itoa(cv.ID)+"&path=/", nil, ""); resp.StatusCode != 400 {
		t.Fatalf("file manager on RDP: %d", resp.StatusCode)
	}

	// policy off
	setSetting("desktop_enabled", "0")
	dc3 := u.openDesktop(cv.ID)
	dc3.wait(t)
	setSetting("desktop_enabled", "1")
	if !strings.Contains(dc3.text(), "turned off") {
		t.Fatalf("policy not applied: %q", dc3.text())
	}
}

func TestDesktopThroughJumpHost(t *testing.T) {
	g := startFakeGuacd(t, true)
	setSetting("guacd_address", g.addr)
	defer setSetting("guacd_address", "127.0.0.1:4822")
	// a "VNC server" behind the jump host
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
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "vnc-user", false)
	jump := u.addConnection("bastion", sshAddr)
	var cv connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "vnc-01", "protocol": "VNC", "host": vnc.Addr().String(),
		"auth_method": "PASSWORD", "password": "vncpw", "jump_id": jump}, &cv); code != 201 {
		t.Fatalf("create VNC connection: %d", code)
	}
	dc := u.openDesktop(cv.ID)
	waitUntil(t, "target reached through the tunnel", func() bool { return strings.Contains(dc.text(), "wrm-test-target") })
	if !strings.Contains(dc.text(), "RFB 003.008") {
		t.Fatalf("guacd did not reach the VNC server through the jump host: %q", dc.text())
	}
	g.mu.Lock()
	p := g.params
	g.mu.Unlock()
	if p["hostname"] != "127.0.0.1" || p["port"] == strings.Split(vnc.Addr().String(), ":")[1] {
		t.Fatalf("guacd should get the local tunnel, got %v", p)
	}
	// the temporary tunnel is listed while the session runs and stops with it
	var tl struct {
		Tunnels []tunnelView `json:"tunnels"`
	}
	u.jsonDo("GET", "/api/tunnels", nil, &tl)
	found := false
	for _, v := range tl.Tunnels {
		found = found || (v.Ephemeral && v.Reason == "desktop" && v.ConnName == "vnc-01")
	}
	if !found {
		t.Fatalf("desktop tunnel not listed: %+v", tl.Tunnels)
	}
	dc.ws.Close()
	<-dc.end
	waitUntil(t, "tunnel stopped", func() bool {
		u.jsonDo("GET", "/api/tunnels", nil, &tl)
		for _, v := range tl.Tunnels {
			if v.Reason == "desktop" {
				return false
			}
		}
		return true
	})

	// guacd not running: a clear message
	if _, err := setSetting("guacd_address", "127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	dc2 := u.openDesktop(cv.ID)
	dc2.wait(t)
	if !strings.Contains(dc2.text(), "guacd") {
		t.Fatalf("missing guacd not explained: %q", dc2.text())
	}
}
