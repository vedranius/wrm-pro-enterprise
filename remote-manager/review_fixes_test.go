// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeFTP is a minimal FTP server with EPSV and LIST (one data connection at a time).
func startFakeFTP(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				rd := bufio.NewReader(c)
				say := func(l string) { io.WriteString(c, l+"\r\n") }
				say("220 fake FTP")
				var data net.Listener
				for {
					line, err := rd.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.ToUpper(strings.Fields(strings.TrimSpace(line) + " x")[0])
					switch cmd {
					case "USER":
						say("331 password please")
					case "PASS":
						say("230 logged in")
					case "FEAT":
						say("211 End")
					case "PWD":
						say(`257 "/" is the current directory`)
					case "CWD":
						say("250 OK")
					case "EPSV":
						data, _ = net.Listen("tcp", "127.0.0.1:0")
						say(fmt.Sprintf("229 Entering Extended Passive Mode (|||%d|)", data.Addr().(*net.TCPAddr).Port))
					case "LIST":
						if data == nil {
							say("425 no data connection")
							continue
						}
						say("150 Here comes the listing")
						dc, err := data.Accept()
						if err == nil {
							io.WriteString(dc, "-rw-r--r--    1 0        0              12 Jan 01 00:00 hello.txt\r\n")
							dc.Close()
						}
						data.Close()
						data = nil
						say("226 Done")
					case "QUIT":
						say("221 Bye")
						return
					default:
						say("200 OK")
					}
				}
			}(c)
		}
	}()
	return ln.Addr().String()
}

func TestFTPThroughProxyEPSV(t *testing.T) {
	ftpAddr := startFakeFTP(t)
	_, port, _ := net.SplitHostPort(ftpAddr)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "ftp-proxy", false)
	socks := startTestProxy(t, "socks5", "", "")
	sp := u.addProxy("ftp-socks", "socks5", socks)
	var cv connView
	// "localhost": the data connection must go to the FTP server's name, not to the proxy's address
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "ftp-px", "protocol": "FTP", "host": "localhost:" + port,
		"username": "anon", "password": "x", "auth_method": "PASSWORD", "proxy_id": sp}, &cv); code != 201 {
		t.Fatalf("create FTP connection: %d", code)
	}
	resp := u.do("GET", "/api/remote/list?id="+strconv.Itoa(cv.ID)+"&path=/", nil, "")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "hello.txt") {
		t.Fatalf("FTP list through the proxy: %d %s", resp.StatusCode, body)
	}
	seen := socks.seen()
	if len(seen) != 2 || seen[0] != "localhost:"+port || !strings.HasPrefix(seen[1], "localhost:") || seen[1] == seen[0] {
		t.Fatalf("FTP control and EPSV data connection targets through the proxy: %q", seen)
	}
}

func TestReviewFixesRoutes(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	alice := newTestUser(t, srv, "rf-alice", false)
	bob := newTestUser(t, srv, "rf-bob", false)

	// Bob cannot reach Alice's running SOCKS tunnel through a proxy at 127.0.0.1
	gw := alice.addConnection("rf-gw", sshAddr)
	alice.jsonDo("PUT", "/api/connections/"+strconv.Itoa(gw)+"/tunnels", []map[string]interface{}{{"name": "dyn", "kind": "dynamic", "bind_host": "127.0.0.1", "bind_port": 0}}, &[]tunnelDef{})
	def := loadTunnelDefs("conn_id=?", gw)[0]
	gc, _ := loadConnection(gw)
	tr, err := tunnelMgr.start("t"+strconv.Itoa(def.ID), def, gc, alice.userID, "manual", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tunnelMgr.stop("t"+strconv.Itoa(def.ID), 0, "test")
	_, tport, _ := net.SplitHostPort(tr.view().Listen)
	tp, _ := strconv.Atoi(tport)
	var bp proxyDef
	if code := bob.jsonDo("POST", "/api/proxies", map[string]interface{}{"name": "steal", "kind": "socks5", "host": "127.0.0.1", "port": tp}, &bp); code != 200 {
		t.Fatalf("create proxy: %d", code)
	}
	var res map[string]interface{}
	bob.jsonDo("POST", "/api/connections/test", map[string]interface{}{"name": "x", "host": sshAddr, "username": testSSHUser, "password": testSSHPass,
		"auth_method": "PASSWORD", "protocol": "SSH", "proxy_id": bp.ID}, &res)
	if res["ok"] == true || !strings.Contains(fmt.Sprint(res["message"]), "SSH tunnel of another user") {
		t.Fatalf("another user's tunnel used as a proxy: %v", res)
	}
	// Alice herself may use her own tunnel that way
	var ap proxyDef
	alice.jsonDo("POST", "/api/proxies", map[string]interface{}{"name": "mine", "kind": "socks5", "host": "127.0.0.1", "port": tp}, &ap)
	alice.jsonDo("POST", "/api/connections/test", map[string]interface{}{"name": "x", "host": sshAddr, "username": testSSHUser, "password": testSSHPass,
		"auth_method": "PASSWORD", "protocol": "SSH", "proxy_id": ap.ID}, &res)
	if res["ok"] != true {
		t.Fatalf("own tunnel as a proxy: %v", res)
	}

	// the connection test does not reveal the name of a proxy that is not shared
	res = nil
	bob.jsonDo("POST", "/api/connections/test", map[string]interface{}{"name": "x", "host": sshAddr, "username": testSSHUser, "password": testSSHPass,
		"auth_method": "PASSWORD", "protocol": "SSH", "proxy_id": ap.ID}, &res)
	if res["ok"] == true || strings.Contains(fmt.Sprint(res["message"]), "mine") || !strings.Contains(fmt.Sprint(res["message"]), "proxy not found") {
		t.Fatalf("connection test with another user's proxy: %v", res)
	}

	// status checks through a proxy are per user
	k1, _ := statusKey(monConn{id: 1, userID: alice.userID, proxy: ap.ID, proto: "SSH", host: sshAddr}, false)
	k2, _ := statusKey(monConn{id: 2, userID: bob.userID, proxy: ap.ID, proto: "SSH", host: sshAddr}, false)
	if k1.key == k2.key {
		t.Fatalf("status key shared between users: %q", k1.key)
	}

	// a status check never starts a WRM tunnel proxy: the state is unknown while it is stopped
	var wp proxyDef
	alice.jsonDo("POST", "/api/proxies", map[string]interface{}{"name": "wrm-tun", "kind": "wrm_tunnel", "tunnel_id": def.ID}, &wp)
	tunnelMgr.stop("t"+strconv.Itoa(def.ID), 0, "test")
	tgt, _ := statusKey(monConn{id: 3, userID: alice.userID, proxy: wp.ID, proto: "SSH", host: sshAddr}, false)
	if r := probe(tgt, statusDialer(tgt, directDial, false)); r.State != "unknown" || tunnelMgr.get("t"+strconv.Itoa(def.ID)) != nil {
		t.Fatalf("status through a stopped WRM tunnel proxy: %+v (tunnel started: %v)", r, tunnelMgr.get("t"+strconv.Itoa(def.ID)) != nil)
	}

	// a folder whose default proxy is the WRM tunnel of one of its connections: that connection goes direct
	var f Folder
	alice.jsonDo("POST", "/api/folders", map[string]interface{}{"name": "rf-tun", "proxy_id": wp.ID}, &f)
	db.Exec(`UPDATE connections SET folder_id=? WHERE id=?`, f.ID, gw)
	if c, _ := loadConnection(gw); c.ProxyID != nil {
		t.Fatalf("the tunnel's own connection inherits its tunnel as proxy")
	}
	other := alice.addConnection("rf-other", sshAddr)
	db.Exec(`UPDATE connections SET folder_id=? WHERE id=?`, f.ID, other)
	if c, _ := loadConnection(other); c.ProxyID == nil || *c.ProxyID != wp.ID {
		t.Fatalf("other connection in the folder does not inherit the tunnel proxy")
	}
	var mon []monConn
	for _, m := range loadMonitoredConns() {
		if m.id == gw || m.id == other {
			mon = append(mon, m)
		}
	}
	for _, m := range mon {
		if (m.id == gw && m.proxy != 0) || (m.id == other && m.proxy != wp.ID) {
			t.Fatalf("effective proxy in the status query: %+v", m)
		}
	}

	// a PUT without jump_id / proxy_id keeps the stored choices
	var sp proxyDef
	alice.jsonDo("POST", "/api/proxies", map[string]interface{}{"name": "keep", "kind": "socks5", "host": "10.9.9.9", "port": 1080}, &sp)
	db.Exec(`UPDATE connections SET proxy_id=?, jump_conn_id=-1 WHERE id=?`, sp.ID, other)
	if code := alice.jsonDo("PUT", "/api/connections/"+strconv.Itoa(other), map[string]interface{}{"name": "rf-other", "protocol": "SSH", "host": sshAddr,
		"username": testSSHUser, "auth_method": "PASSWORD", "folder_id": f.ID, "notes": "hello"}, nil); code != 200 {
		t.Fatalf("PUT: %d", code)
	}
	var pj, jj *int
	db.QueryRow(`SELECT proxy_id, jump_conn_id FROM connections WHERE id=?`, other).Scan(&pj, &jj)
	if pj == nil || *pj != sp.ID || jj == nil || *jj != -1 {
		t.Fatalf("stored choices after a PUT without them: proxy %v jump %v", pj, jj)
	}

	// folder defaults that would make a loop are refused (folder update and bulk move), and nothing changes
	a := alice.addConnection("rf-a", sshAddr)
	b := alice.addConnectionVia("rf-b", sshAddr, a)
	var loop Folder
	alice.jsonDo("POST", "/api/folders", map[string]interface{}{"name": "rf-loop"}, &loop)
	db.Exec(`UPDATE connections SET folder_id=? WHERE id=?`, loop.ID, a)
	if code := alice.jsonDo("PUT", "/api/folders/"+strconv.Itoa(loop.ID), map[string]interface{}{"name": "rf-loop", "jump_id": b}, nil); code != 400 {
		t.Fatalf("folder default making a loop accepted: %d", code)
	}
	var fj *int
	db.QueryRow(`SELECT jump_conn_id FROM folders WHERE id=?`, loop.ID).Scan(&fj)
	if fj != nil {
		t.Fatalf("refused folder default was stored")
	}
	db.Exec(`UPDATE connections SET folder_id=NULL WHERE id=?`, a)
	var loop2 Folder
	alice.jsonDo("POST", "/api/folders", map[string]interface{}{"name": "rf-loop2", "jump_id": b}, &loop2)
	if code := alice.jsonDo("POST", "/api/connections/bulk", map[string]interface{}{"action": "move", "ids": []int{a}, "folder_id": loop2.ID}, nil); code != 400 {
		t.Fatalf("move into a folder whose default makes a loop accepted: %d", code)
	}
	var af *int
	db.QueryRow(`SELECT folder_id FROM connections WHERE id=?`, a).Scan(&af)
	if af != nil {
		t.Fatalf("refused move was kept")
	}

	// a quick connection cannot be a folder default jump host
	db.Exec(`UPDATE connections SET temp_until=? WHERE id=?`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339), b)
	if code := alice.jsonDo("POST", "/api/folders", map[string]interface{}{"name": "rf-quick", "jump_id": b}, nil); code != 400 {
		t.Fatalf("quick connection as folder default accepted: %d", code)
	}
	db.Exec(`UPDATE connections SET temp_until='' WHERE id=?`, b)
}

func TestReviewFixesNotify(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	admin := newTestUser(t, srv, "rf-nadmin", true)
	user := newTestUser(t, srv, "rf-nuser", false)
	hook := newFakeHook(t)
	mail := newFakeSMTP(t, "", "")
	host, port, _ := net.SplitHostPort(mail.addr)
	em := createChannel(t, admin, map[string]interface{}{"name": "rf-mail", "kind": "email", "user_address": true, "config": map[string]string{
		"host": host, "port": port, "security": "none", "from": "wrm@example.com"}})
	sl := createChannel(t, admin, map[string]interface{}{"name": "rf-slack", "kind": "slack", "config": map[string]string{"webhook_url": hook.srv.URL + "/s"}})
	dc := createChannel(t, admin, map[string]interface{}{"name": "rf-discord", "kind": "discord", "config": map[string]string{"webhook_url": hook.srv.URL + "/d"}})
	defer db.Exec(`DELETE FROM notify_channels WHERE id IN (?,?,?)`, em.ID, sl.ID, dc.ID)

	// user recipients are bare addresses: no header injection
	for _, bad := range []string{"x\r\nReply-To:evil@example.net\r\n<me@example.com>", "Me <me@example.com>", "me@example.com\r\nBcc:x@example.com"} {
		if code := user.jsonDo("POST", "/api/notify/test", map[string]interface{}{"channel_id": em.ID, "address": bad}, &map[string]interface{}{}); code != 400 {
			t.Fatalf("recipient %q accepted: %d", bad, code)
		}
	}
	// a channel without a default recipient needs the user's own one
	if code := user.jsonDo("PUT", "/api/notify", map[string]interface{}{"subscriptions": []map[string]interface{}{{"event": "status.down", "channel_id": em.ID}}}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("subscription without a recipient accepted: %d", code)
	}
	// subscriptions of a disabled channel survive saving the settings
	user.jsonDo("PUT", "/api/notify", map[string]interface{}{"subscriptions": []map[string]interface{}{{"event": "status.down", "channel_id": sl.ID}, {"event": "status.down", "channel_id": dc.ID}}}, nil)
	db.Exec(`UPDATE notify_channels SET enabled=0 WHERE id=?`, dc.ID)
	user.jsonDo("PUT", "/api/notify", map[string]interface{}{"subscriptions": []map[string]interface{}{{"event": "status.up", "channel_id": sl.ID}}}, nil)
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM notify_subscriptions WHERE user_id=? AND channel_id=?`, user.userID, dc.ID).Scan(&n)
	if n != 1 {
		t.Fatalf("subscription of a disabled channel removed")
	}
	db.Exec(`UPDATE notify_channels SET enabled=1 WHERE id=?`, dc.ID)

	// mentions of user-controlled names are neutralised
	hook.reset()
	ch, _ := loadNotifyChannel(sl.ID)
	sendNotification(ch, "", "<!channel> @here web is down", "@everyone <http://evil.example|bank>", nil)
	ch, _ = loadNotifyChannel(dc.ID)
	sendNotification(ch, "", "@everyone web is down", "", nil)
	reqs := hook.all()
	if len(reqs) != 2 || strings.Contains(reqs[0].Body, "<!channel>") || strings.Contains(reqs[0].Body, `"@here`) || strings.Contains(reqs[0].Body, "<http") ||
		!strings.Contains(reqs[1].Body, `"allowed_mentions":{"parse":[]}`) {
		t.Fatalf("mentions: %+v", reqs)
	}

	// a credential reminder without a subscription is not recorded as sent
	old := time.Now().Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)
	res, _ := db.Exec(`INSERT INTO credentials (owner_id,name,username,password,rotate_days,rotated_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		user.userID, "rf-root", "root", encryptValue("x"), 30, old, old, old)
	cid, _ := res.LastInsertId()
	defer db.Exec(`DELETE FROM credentials WHERE id=?`, cid)
	checkCredentialReminders()
	if v := getNotifyState("cred_due:" + strconv.FormatInt(cid, 10)); v != "" {
		t.Fatalf("reminder recorded without a subscription: %q", v)
	}
	user.jsonDo("PUT", "/api/notify", map[string]interface{}{"subscriptions": []map[string]interface{}{{"event": "credential.rotation_due", "channel_id": sl.ID}}}, nil)
	checkCredentialReminders()
	db.QueryRow(`SELECT COUNT(*) FROM notify_pending WHERE user_id=? AND event='credential.rotation_due'`, user.userID).Scan(&n)
	if n != 1 || getNotifyState("cred_due:"+strconv.FormatInt(cid, 10)) == "" {
		t.Fatalf("reminder after subscribing: %d", n)
	}
	db.Exec(`DELETE FROM notify_pending WHERE user_id=?`, user.userID)

	// UTF-8 safe truncation (ntfy, digests)
	if s := truncateStr("abč", 3); s != "ab" {
		t.Fatalf("truncateStr cut a character: %q", s)
	}
	b, _ := json.Marshal(slackEscape("a & b"))
	if !strings.Contains(string(b), "a \\u0026amp; b") {
		t.Fatalf("slackEscape: %s", b)
	}
}

// fakeSMTPLogin offers only AUTH LOGIN (like Exchange receive connectors).
func TestSMTPAuthLogin(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 4)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		rd := bufio.NewReader(c)
		say := func(l string) { io.WriteString(c, l+"\r\n") }
		read := func() string { l, _ := rd.ReadString('\n'); return strings.TrimRight(l, "\r\n") }
		say("220 fake")
		for {
			l := read()
			switch {
			case strings.HasPrefix(strings.ToUpper(l), "EHLO"):
				say("250-fake")
				say("250 AUTH LOGIN")
			case strings.ToUpper(l) == "AUTH LOGIN":
				say("334 VXNlcm5hbWU6") // "Username:"
				got <- read()
				say("334 UGFzc3dvcmQ6") // "Password:"
				got <- read()
				say("235 OK")
			case strings.HasPrefix(strings.ToUpper(l), "MAIL"), strings.HasPrefix(strings.ToUpper(l), "RCPT"):
				say("250 OK")
			case strings.ToUpper(l) == "DATA":
				say("354 go")
				for read() != "." {
				}
				say("250 queued")
			case strings.ToUpper(l) == "QUIT":
				say("221 bye")
				return
			case l == "":
				return
			default:
				say("250 OK")
			}
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	ch := notifyChannel{Kind: "email", cfg: map[string]string{"host": host, "port": port, "security": "none", "username": "wrm", "password": "pw-login",
		"from": "wrm@example.com", "to": "ops@example.com"}}
	if err := sendSMTP(ch, "subject", "body"); err != nil {
		t.Fatalf("AUTH LOGIN: %v", err)
	}
	if u, p := <-got, <-got; u != "d3Jt" || p != "cHctbG9naW4=" { // base64 of wrm / pw-login
		t.Fatalf("AUTH LOGIN exchange: %q %q", u, p)
	}
}
