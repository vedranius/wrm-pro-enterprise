// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func (c *testClient) addConnectionVia(name, host string, jump int) int {
	c.t.Helper()
	id := c.addConnection(name, host)
	if jump > 0 {
		db.Exec(`UPDATE connections SET jump_conn_id=? WHERE id=?`, jump, id)
	}
	return id
}

func (c *testClient) jsonDo(method, path string, body interface{}, out interface{}) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	resp := c.do(method, path, rd, "application/json")
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil {
		json.Unmarshal(data, out)
	}
	if resp.StatusCode >= 300 && out == nil {
		c.t.Logf("%s %s → %d %s", method, path, resp.StatusCode, data)
	}
	return resp.StatusCode
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func httpGetBody(t *testing.T, url string) string {
	t.Helper()
	cl := &http.Client{Timeout: 10 * time.Second}
	resp, err := cl.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// A connection two jump hosts away works for terminals, files and connection tests, and
// the route is recorded.
func TestJumpHostChain(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "jump-user", false)
	bastion := u.addConnection("bastion", sshAddr)
	mid := u.addConnectionVia("dc-gw", sshAddr, bastion)
	target := u.addConnectionVia("app-01", sshAddr, mid)

	before := directTCPIPCount.Load()
	tc := u.openTerminal(target)
	tc.waitFor("$ ")
	tc.waitFor("via bastion → dc-gw")
	tc.send("exit\r")
	<-tc.closed
	if n := directTCPIPCount.Load() - before; n < 2 {
		t.Fatalf("expected 2 hops through direct-tcpip, got %d", n)
	}
	var sid int64
	var route string
	db.QueryRow(`SELECT MAX(id) FROM terminal_sessions WHERE conn_id=?`, target).Scan(&sid)
	waitSessionEnded(t, sid)
	db.QueryRow(`SELECT jump_path FROM terminal_sessions WHERE id=?`, sid).Scan(&route)
	if route != "bastion → dc-gw" {
		t.Fatalf("jump_path = %q", route)
	}

	// SFTP through the chain
	dir := t.TempDir()
	resp := u.do("GET", "/api/remote/list?id="+strconv.Itoa(target)+"&path="+dir, nil, "")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("file list through jump hosts: %d", resp.StatusCode)
	}

	// Connection test reports the route
	var res map[string]interface{}
	u.jsonDo("POST", "/api/connections/test", map[string]interface{}{"id": target, "name": "app-01", "host": sshAddr, "username": testSSHUser,
		"auth_method": "PASSWORD", "protocol": "SSH", "jump_id": mid}, &res)
	if res["ok"] != true || !strings.Contains(fmt.Sprint(res["message"]), "via bastion → dc-gw") {
		t.Fatalf("connection test: %v", res)
	}

	// The connection list shows the route
	var views []connView
	u.jsonDo("GET", "/api/connections", nil, &views)
	for _, v := range views {
		if v.ID == target && v.Route != "bastion → dc-gw" {
			t.Fatalf("route in list = %q", v.Route)
		}
	}

	// Loops, self references and other users' connections are refused.
	put := func(id int, jump int) int {
		return u.jsonDo("PUT", "/api/connections/"+strconv.Itoa(id), map[string]interface{}{"name": "x" + strconv.Itoa(id), "host": sshAddr,
			"username": testSSHUser, "auth_method": "PASSWORD", "protocol": "SSH", "jump_id": jump}, nil)
	}
	if code := put(bastion, target); code != 400 {
		t.Fatalf("loop accepted: %d", code)
	}
	if code := put(mid, mid); code != 400 {
		t.Fatalf("self jump accepted: %d", code)
	}
	other := newTestUser(t, srv, "jump-other", false)
	foreign := other.addConnection("foreign", sshAddr)
	if code := put(target, foreign); code != 400 {
		t.Fatalf("another user's connection accepted as jump host: %d", code)
	}
	// Deleting a jump host detaches the connections that used it.
	u.jsonDo("DELETE", "/api/connections/"+strconv.Itoa(mid), nil, nil)
	var jump *int
	db.QueryRow(`SELECT jump_conn_id FROM connections WHERE id=?`, target).Scan(&jump)
	if jump != nil {
		t.Fatalf("jump host reference kept after delete")
	}
}

func socks5Get(t *testing.T, proxy, host string, port int, path string) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", proxy, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	c.Write([]byte{5, 1, 0})
	r := make([]byte, 2)
	if _, err := io.ReadFull(c, r); err != nil || r[1] != 0 {
		t.Fatalf("socks greeting: %v %v", r, err)
	}
	req := []byte{5, 1, 0, 3, byte(len(host))}
	req = append(req, host...)
	pb := make([]byte, 2)
	binary.BigEndian.PutUint16(pb, uint16(port))
	req = append(req, pb...)
	c.Write(req)
	rep := make([]byte, 10)
	if _, err := io.ReadFull(c, rep); err != nil || rep[1] != 0 {
		t.Fatalf("socks connect: %v %v", rep, err)
	}
	fmt.Fprintf(c, "GET %s HTTP/1.0\r\nHost: %s\r\n\r\n", path, host)
	b, _ := io.ReadAll(c)
	return string(b)
}

// Local, dynamic (SOCKS5) and remote forwards, web connections and start modes.
func TestTunnels(t *testing.T) {
	old := tunnelConnectGrace
	tunnelConnectGrace = 200 * time.Millisecond
	defer func() { tunnelConnectGrace = old }()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello from backend %s", r.URL.Path)
	}))
	defer backend.Close()
	_, bportS, _ := net.SplitHostPort(strings.TrimPrefix(backend.URL, "http://"))
	bport, _ := strconv.Atoi(bportS)

	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	admin := newTestUser(t, srv, "tun-admin", true)
	user := newTestUser(t, srv, "tun-user", false)
	site := admin.addConnection("site1", sshAddr)
	inner := admin.addConnectionVia("site1-web", sshAddr, site)

	var defs []tunnelDef
	code := admin.jsonDo("PUT", "/api/connections/"+strconv.Itoa(inner)+"/tunnels", []map[string]interface{}{
		{"name": "web admin", "kind": "local", "bind_port": 0, "target_host": "127.0.0.1", "target_port": bport, "open_scheme": "http", "open_path": "/ui"},
		{"name": "socks", "kind": "dynamic", "bind_port": 0},
		{"name": "back", "kind": "remote", "bind_host": "127.0.0.1", "bind_port": 0, "target_host": "127.0.0.1", "target_port": bport},
	}, &defs)
	if code != 200 || len(defs) != 3 {
		t.Fatalf("save tunnels: %d %v", code, defs)
	}
	start := func(d tunnelDef) tunnelView {
		var v tunnelView
		if code := admin.jsonDo("POST", "/api/tunnels/t"+strconv.Itoa(d.ID)+"/start", nil, &v); code != 200 || v.Listen == "" {
			t.Fatalf("start %s: %d %+v", d.Name, code, v)
		}
		return v
	}
	// local forward through the jump host
	lv := start(defs[0])
	if body := httpGetBody(t, "http://"+lv.Listen+"/ui"); body != "hello from backend /ui" {
		t.Fatalf("local tunnel body %q", body)
	}
	// SOCKS5
	sv := start(defs[1])
	if body := socks5Get(t, sv.Listen, "127.0.0.1", bport, "/socks"); !strings.Contains(body, "hello from backend /socks") {
		t.Fatalf("socks body %q", body)
	}
	// remote forward: the SSH server listens and forwards back to WRM's side
	rv := start(defs[2])
	if body := httpGetBody(t, "http://"+rv.Listen+"/remote"); body != "hello from backend /remote" {
		t.Fatalf("remote tunnel body %q", body)
	}
	// state and traffic are reported
	var list struct {
		Tunnels []tunnelView `json:"tunnels"`
	}
	admin.jsonDo("GET", "/api/tunnels", nil, &list)
	running := 0
	for _, v := range list.Tunnels {
		if v.Running && v.State == "up" {
			running++
			if v.Total < 1 || v.BytesDown == 0 || v.Route != "site1" {
				t.Fatalf("stats/route of %s: %+v", v.Name, v)
			}
		}
	}
	if running != 3 {
		t.Fatalf("running tunnels = %d", running)
	}
	// another user cannot stop them; the owner can
	if code := user.jsonDo("POST", "/api/tunnels/"+lv.Key+"/stop", nil, nil); code != 404 {
		t.Fatalf("other user stopped a tunnel: %d", code)
	}
	for _, k := range []string{lv.Key, sv.Key, rv.Key} {
		admin.jsonDo("POST", "/api/tunnels/"+k+"/stop", nil, nil)
	}
	if c, err := net.DialTimeout("tcp", lv.Listen, time.Second); err == nil {
		c.Close()
		t.Fatalf("local port still open after stop")
	}
	var starts, stops int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='tunnel.start' AND conn_id=?`, inner).Scan(&starts)
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='tunnel.stop' AND conn_id=? AND details LIKE '%bytes_down%'`, inner).Scan(&stops)
	if starts != 3 || stops != 3 {
		t.Fatalf("audit: %d starts, %d stops", starts, stops)
	}

	// policies: users may only listen on loopback, remote forwards are for admins
	ucon := user.addConnection("user-site", sshAddr)
	if code := user.jsonDo("PUT", "/api/connections/"+strconv.Itoa(ucon)+"/tunnels", []map[string]interface{}{
		{"kind": "local", "bind_host": "0.0.0.0", "bind_port": 18555, "target_host": "127.0.0.1", "target_port": 80}}, nil); code != 400 {
		t.Fatalf("non-admin bind on 0.0.0.0 accepted: %d", code)
	}
	if code := user.jsonDo("PUT", "/api/connections/"+strconv.Itoa(ucon)+"/tunnels", []map[string]interface{}{
		{"kind": "remote", "bind_port": 18556, "target_host": "127.0.0.1", "target_port": 80}}, nil); code != 400 {
		t.Fatalf("non-admin remote forward accepted: %d", code)
	}
	if code := user.jsonDo("PUT", "/api/connections/"+strconv.Itoa(ucon)+"/tunnels", []map[string]interface{}{
		{"kind": "local", "bind_port": 80, "target_host": "127.0.0.1", "target_port": 80}}, nil); code != 400 {
		t.Fatalf("privileged port accepted: %d", code)
	}

	// web interface connection behind the jump host
	res, _ := db.Exec(`INSERT INTO connections (name, protocol, host, username, auth_method, password, user_id, jump_conn_id, web_path) VALUES (?,?,?,?,?,?,?,?,?)`,
		"backend-ui", "HTTP", "127.0.0.1:"+bportS, "", "PASSWORD", "", admin.userID, site, "/dash")
	webID, _ := res.LastInsertId()
	var open map[string]interface{}
	if code := admin.jsonDo("POST", "/api/connections/"+strconv.FormatInt(webID, 10)+"/open-web", nil, &open); code != 200 {
		t.Fatalf("open-web: %d %v", code, open)
	}
	if body := httpGetBody(t, fmt.Sprint(open["url"])); body != "hello from backend /dash" {
		t.Fatalf("web tunnel body %q (url %v)", body, open["url"])
	}
	var again map[string]interface{}
	admin.jsonDo("POST", "/api/connections/"+strconv.FormatInt(webID, 10)+"/open-web", nil, &again)
	if again["url"] != open["url"] {
		t.Fatalf("web tunnel not reused: %v vs %v", again["url"], open["url"])
	}
	// the temporary tunnel is listed under the web connection, not the jump host it runs over
	var mine struct {
		Tunnels []tunnelView `json:"tunnels"`
	}
	admin.jsonDo("GET", "/api/tunnels", nil, &mine)
	foundWeb := false
	for _, v := range mine.Tunnels {
		if v.Ephemeral {
			foundWeb = true
			if v.ConnName != "backend-ui" || v.ConnID != int(webID) || !v.Running {
				t.Fatalf("temporary web tunnel view: %+v", v)
			}
		}
	}
	if !foundWeb {
		t.Fatalf("temporary web tunnel not listed: %+v", mine.Tunnels)
	}
	// without a jump host the address is opened directly
	db.Exec(`UPDATE connections SET jump_conn_id=NULL WHERE id=?`, webID)
	var direct map[string]interface{}
	admin.jsonDo("POST", "/api/connections/"+strconv.FormatInt(webID, 10)+"/open-web", nil, &direct)
	if direct["direct"] != true || direct["url"] != "http://127.0.0.1:"+bportS+"/dash" {
		t.Fatalf("direct web open: %v", direct)
	}
	tunnelMgr.stopConn(int(webID), "test")

	// start mode "connect": runs while a terminal of the connection is open
	var cdefs []tunnelDef
	admin.jsonDo("PUT", "/api/connections/"+strconv.Itoa(site)+"/tunnels", []map[string]interface{}{
		{"name": "with terminal", "kind": "local", "bind_port": 0, "target_host": "127.0.0.1", "target_port": bport, "start_mode": "connect"}}, &cdefs)
	key := "t" + strconv.Itoa(cdefs[0].ID)
	tc := admin.openTerminal(site)
	tc.waitFor("$ ")
	waitUntil(t, "connect tunnel to start", func() bool { tr := tunnelMgr.get(key); return tr != nil && tr.view().State == "up" })
	tc.send("exit\r")
	<-tc.closed
	waitUntil(t, "connect tunnel to stop", func() bool { return tunnelMgr.get(key) == nil })

	// deleting the connection stops its tunnels and removes their configuration
	start(defs[0])
	admin.jsonDo("DELETE", "/api/connections/"+strconv.Itoa(inner), nil, nil)
	if tunnelMgr.get("t"+strconv.Itoa(defs[0].ID)) != nil {
		t.Fatalf("tunnel still running after its connection was deleted")
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM connection_tunnels WHERE conn_id=?`, inner).Scan(&n)
	if n != 0 {
		t.Fatalf("tunnel definitions kept after delete")
	}

	// policy off: nothing can start
	setSetting("tunnels_enabled", "0")
	defer setSetting("tunnels_enabled", "1")
	if code := admin.jsonDo("POST", "/api/tunnels/"+key+"/start", nil, nil); code == 200 {
		t.Fatalf("tunnel started while tunnels are disabled")
	}
}
