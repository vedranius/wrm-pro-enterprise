// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSnippetsAPI(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	admin := newTestUser(t, srv, "snip-admin", true)
	user := newTestUser(t, srv, "snip-user", false)
	other := newTestUser(t, srv, "snip-other", false)
	conn := user.addConnection("web-01", "10.0.0.5:2222")
	otherConn := other.addConnection("theirs", "10.0.0.9")

	var s Snippet
	if code := user.jsonDo("POST", "/api/snippets", map[string]interface{}{"name": "Disk", "command": "df -h", "group": "Diagnostics"}, &s); code != 200 || s.ID == 0 || !s.Mine || s.Scope != "all" {
		t.Fatalf("create: %d %+v", code, s)
	}
	// validation
	bad := []map[string]interface{}{
		{"name": "", "command": "ls"},
		{"name": "x", "command": "  "},
		{"name": "x", "command": "ls", "scope": "connection", "scope_id": otherConn}, // not my connection
		{"name": "x", "command": "ls", "scope": "folder", "scope_id": 99999},
		{"name": "x", "command": "ls", "scope": "galaxy"},
		{"name": "x", "command": "ls", "shared": true},                               // users cannot share
		{"name": "x", "command": "systemctl restart {{?Service}}", "auto_run": true}, // prompts cannot auto-run
	}
	for i, b := range bad {
		if code := user.jsonDo("POST", "/api/snippets", b, &map[string]interface{}{}); code != 400 {
			t.Fatalf("bad snippet %d accepted: %d", i, code)
		}
	}
	// admins share snippets with everybody (read-only for others, never auto-run)
	if code := admin.jsonDo("POST", "/api/snippets", map[string]interface{}{"name": "Shared", "command": "uptime", "shared": true, "auto_run": true}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("shared auto-run accepted: %d", code)
	}
	var shared Snippet
	if code := admin.jsonDo("POST", "/api/snippets", map[string]interface{}{"name": "Shared", "command": "uptime", "shared": true}, &shared); code != 200 {
		t.Fatalf("shared: %d", code)
	}
	var list []Snippet
	user.jsonDo("GET", "/api/snippets", nil, &list)
	if len(list) != 2 {
		t.Fatalf("user sees %d snippets: %+v", len(list), list)
	}
	for _, x := range list {
		if x.ID == shared.ID && (x.Mine || x.Owner != "snip-admin") {
			t.Fatalf("shared snippet view: %+v", x)
		}
	}
	other.jsonDo("GET", "/api/snippets", nil, &list)
	if len(list) != 1 || list[0].ID != shared.ID {
		t.Fatalf("other user sees: %+v", list)
	}
	// others cannot change or delete someone's snippet, or a shared one
	if code := other.jsonDo("PUT", "/api/snippets/"+strconv.Itoa(s.ID), map[string]interface{}{"name": "x", "command": "rm"}, nil); code != 404 {
		t.Fatalf("foreign update: %d", code)
	}
	if code := user.jsonDo("DELETE", "/api/snippets/"+strconv.Itoa(shared.ID), nil, nil); code != 404 {
		t.Fatalf("user deleted a shared snippet: %d", code)
	}
	// update with a connection scope
	var upd Snippet
	if code := user.jsonDo("PUT", "/api/snippets/"+strconv.Itoa(s.ID), map[string]interface{}{"name": "Disk", "command": "df -h {{?Path=/}}", "scope": "connection", "scope_id": conn}, &upd); code != 200 || upd.Scope != "connection" || upd.ScopeID != conn {
		t.Fatalf("update: %d %+v", code, upd)
	}
	// duplicate copies connection snippets; delete removes them
	var dup map[string]interface{}
	user.jsonDo("POST", "/api/connections/"+strconv.Itoa(conn)+"/duplicate", nil, &dup)
	dupID := int(dup["id"].(float64))
	if n := len(loadSnippets("scope='connection' AND scope_id=?", dupID)); n != 1 {
		t.Fatalf("duplicate copied %d snippets", n)
	}
	user.jsonDo("DELETE", "/api/connections/"+strconv.Itoa(dupID), nil, nil)
	if n := len(loadSnippets("scope='connection' AND scope_id=?", dupID)); n != 0 {
		t.Fatalf("delete left %d snippets", n)
	}
	// export / import keeps snippets and maps their connection
	var exp map[string]json.RawMessage
	user.jsonDo("GET", "/api/config/export", nil, &exp)
	if !strings.Contains(string(exp["snippets"]), "df -h") {
		t.Fatalf("export without snippets: %s", exp["snippets"])
	}
	fresh := newTestUser(t, srv, "snip-fresh", false)
	var payload map[string]interface{}
	b, _ := json.Marshal(exp)
	json.Unmarshal(b, &payload)
	var res map[string]int
	fresh.jsonDo("POST", "/api/config/import", payload, &res)
	if res["snippets"] != 1 {
		t.Fatalf("import result %v", res)
	}
	got := loadSnippets("user_id=?", fresh.userID)
	if len(got) != 1 || got[0].Scope != "connection" || !userOwnsConnection(got[0].ScopeID, fresh.userID) {
		t.Fatalf("imported snippets: %+v", got)
	}
	// audit
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action LIKE 'snippet.%' AND user_id=?`, user.userID).Scan(&n)
	if n < 2 {
		t.Fatalf("snippet audit entries: %d", n)
	}
}

func TestExpandSnippet(t *testing.T) {
	c := Connection{Name: "db-01", Host: "10.1.2.3:2200", Username: "ops", Protocol: "SSH"}
	got := expandSnippet("ssh {{user}}@{{host}} -p {{port}} # {{name}} {{ unknown }} {{?Ask}}", c, "alice")
	if got != "ssh ops@10.1.2.3 -p 2200 # db-01 {{ unknown }} {{?Ask}}" {
		t.Fatalf("expand: %q", got)
	}
	c.Host = "web.example.org"
	if got := expandSnippet("{{host}}:{{port}} by {{wrm_user}}", c, "bob"); got != "web.example.org:22 by bob" {
		t.Fatalf("expand default port: %q", got)
	}
	if got := string(snippetKeystrokes("cd /srv\r\nls -la")); got != "cd /srv\rls -la\r" {
		t.Fatalf("keystrokes: %q", got)
	}
}

func TestRunOnConnectAndBroadcastAudit(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	user := newTestUser(t, srv, "autorun-user", false)
	res, _ := db.Exec(`INSERT INTO folders (name, user_id) VALUES (?,?)`, "Prod", user.userID)
	fid, _ := res.LastInsertId()
	conn := user.addConnection("app-01", sshAddr)
	db.Exec(`UPDATE connections SET folder_id=? WHERE id=?`, fid, conn)
	plain := user.addConnection("plain", sshAddr)

	mk := func(body map[string]interface{}) {
		if code := user.jsonDo("POST", "/api/snippets", body, &map[string]interface{}{}); code != 200 {
			t.Fatalf("create %v: %d", body, code)
		}
	}
	mk(map[string]interface{}{"name": "conn", "command": "echo conn-{{name}}", "scope": "connection", "scope_id": conn, "auto_run": true})
	mk(map[string]interface{}{"name": "folder", "command": "echo folder-{{folder}}", "scope": "folder", "scope_id": fid, "auto_run": true})
	mk(map[string]interface{}{"name": "all", "command": "echo all\necho second", "auto_run": true})
	mk(map[string]interface{}{"name": "manual", "command": "echo never"})

	tc := user.openTerminal(conn)
	tc.waitFor("you typed: echo conn-app-01")
	tc.mu.Lock()
	out := tc.out.String()
	tc.mu.Unlock()
	iAll, iSecond, iFolder, iConn := strings.Index(out, "you typed: echo all"), strings.Index(out, "you typed: echo second"), strings.Index(out, "you typed: echo folder-Prod"), strings.Index(out, "you typed: echo conn-app-01")
	if !(iAll >= 0 && iAll < iSecond && iSecond < iFolder && iFolder < iConn) || strings.Contains(out, "echo never") {
		t.Fatalf("run on connect order/content wrong:\n%s", out)
	}
	waitUntil(t, "autorun control message", func() bool {
		tc.mu.Lock()
		defer tc.mu.Unlock()
		for _, m := range tc.ctl {
			if m["type"] == "autorun" {
				return true
			}
		}
		return false
	})
	// broadcast is recorded per terminal, and can be turned off by policy
	tc.ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"broadcast","on":true,"peers":3}`))
	tc.ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"broadcast","on":false,"peers":3}`))
	waitUntil(t, "broadcast audit", func() bool {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='terminal.broadcast' AND conn_id=?`, conn).Scan(&n)
		return n == 2
	})
	var autoAudit int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='terminal.auto_run' AND conn_id=? AND session_id IS NOT NULL`, conn).Scan(&autoAudit)
	if autoAudit != 1 {
		t.Fatalf("auto_run audit entries: %d", autoAudit)
	}
	setSetting("broadcast_enabled", "0")
	defer setSetting("broadcast_enabled", "1")
	tc.ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"broadcast","on":true,"peers":2}`))
	waitUntil(t, "broadcast refused", func() bool {
		tc.mu.Lock()
		defer tc.mu.Unlock()
		for _, m := range tc.ctl {
			if m["type"] == "broadcast" && m["allowed"] == false {
				return true
			}
		}
		return false
	})
	tc.send("exit\r")
	<-tc.closed

	// a connection outside the folder only gets the "all connections" snippet
	tc2 := user.openTerminal(plain)
	tc2.waitFor("you typed: echo second")
	time.Sleep(300 * time.Millisecond)
	tc2.mu.Lock()
	o := tc2.out.String()
	tc2.mu.Unlock()
	if strings.Contains(o, "folder-") || strings.Contains(o, "conn-") {
		t.Fatalf("wrong snippets ran on another connection:\n%s", o)
	}
	tc2.send("exit\r")
	<-tc2.closed
}
