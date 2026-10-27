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
)

func TestBookmarksAPI(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	user := newTestUser(t, srv, "bm-user", false)
	other := newTestUser(t, srv, "bm-other", false)
	res, _ := db.Exec(`INSERT INTO folders (name, user_id) VALUES (?,?)`, "Prod", user.userID)
	fid, _ := res.LastInsertId()
	conn := user.addConnection("app-01", "10.0.0.5:2222")
	db.Exec(`UPDATE connections SET folder_id=?, tags='env:prod' WHERE id=?`, fid, conn)
	plain := user.addConnection("plain", "10.0.0.6")
	otherConn := other.addConnection("theirs", "10.0.0.9")

	mk := func(body map[string]interface{}) Bookmark {
		t.Helper()
		var b Bookmark
		if code := user.jsonDo("POST", "/api/bookmarks", body, &b); code != 200 || b.ID == 0 {
			t.Fatalf("create %v: %d %+v", body, code, b)
		}
		return b
	}
	global := mk(map[string]interface{}{"name": "Downloads", "path": "/opt/app/servers/{name}/downloads", "shared": true})
	if global.Scope != "global" || !global.Mine {
		t.Fatalf("global: %+v", global)
	}
	folder := mk(map[string]interface{}{"path": "/srv/{host}/logs", "scope": "folder", "scope_id": fid})
	if folder.Name != "logs" {
		t.Fatalf("default name: %q", folder.Name)
	}
	tagged := mk(map[string]interface{}{"name": "Home", "path": "~/it's here", "scope": "tag", "tag": "Env:Prod"})
	if tagged.Tag != "env:prod" || tagged.ScopeID != 0 {
		t.Fatalf("tag: %+v", tagged)
	}
	start := mk(map[string]interface{}{"name": "App", "path": "/home/$USER/app", "scope": "connection", "scope_id": conn, "start_dir": true, "color": "#3b82f6"})

	bad := []map[string]interface{}{
		{"path": ""},
		{"path": "/x\ny"},
		{"path": "/x", "scope": "connection", "scope_id": otherConn}, // not my connection
		{"path": "/x", "scope": "folder", "scope_id": 99999},
		{"path": "/x", "scope": "tag", "tag": "  "},
		{"path": "/x", "scope": "galaxy"},
		{"path": "/x", "scope": "folder", "scope_id": fid, "shared": true}, // only global ones are shared
		{"path": "/x", "color": "red"},
	}
	for i, b := range bad {
		if code := user.jsonDo("POST", "/api/bookmarks", b, &map[string]interface{}{}); code != 400 {
			t.Fatalf("bad bookmark %d accepted: %d", i, code)
		}
	}

	// bookmarks for a connection: scopes, resolved variables, start directory, shell-quoted cd
	var forConn struct {
		Bookmarks []Bookmark `json:"bookmarks"`
		Start     *struct {
			ID   int    `json:"id"`
			Path string `json:"path"`
		} `json:"start"`
	}
	if code := user.jsonDo("GET", "/api/bookmarks?conn="+strconv.Itoa(conn), nil, &forConn); code != 200 || len(forConn.Bookmarks) != 4 {
		t.Fatalf("for conn: %d %+v", code, forConn)
	}
	want := map[int][2]string{
		global.ID: {"/opt/app/servers/app-01/downloads", "cd -- '/opt/app/servers/app-01/downloads'"},
		folder.ID: {"/srv/10.0.0.5/logs", "cd -- '/srv/10.0.0.5/logs'"},
		tagged.ID: {"~/it's here", `cd -- ~/'it'\''s here'`},
		start.ID:  {"/home/tester/app", "cd -- '/home/tester/app'"},
	}
	for _, b := range forConn.Bookmarks {
		if w := want[b.ID]; b.Resolved != w[0] || b.Cd != w[1] {
			t.Fatalf("bookmark %q: %q / %q, want %v", b.Name, b.Resolved, b.Cd, w)
		}
	}
	if forConn.Start == nil || forConn.Start.ID != start.ID || forConn.Start.Path != "/home/tester/app" {
		t.Fatalf("start: %+v", forConn.Start)
	}
	user.jsonDo("GET", "/api/bookmarks?conn="+strconv.Itoa(plain), nil, &forConn)
	if len(forConn.Bookmarks) != 1 || forConn.Bookmarks[0].ID != global.ID || forConn.Start != nil {
		t.Fatalf("plain conn: %+v", forConn)
	}
	// not my connection, not my bookmark
	if code := other.jsonDo("GET", "/api/bookmarks?conn="+strconv.Itoa(conn), nil, &forConn); code != 404 {
		t.Fatalf("foreign connection: %d", code)
	}
	if code := other.jsonDo("PUT", "/api/bookmarks/"+strconv.Itoa(global.ID), map[string]interface{}{"path": "/tmp"}, nil); code != 404 {
		t.Fatalf("foreign update: %d", code)
	}
	if code := other.jsonDo("POST", "/api/bookmarks/resolve", map[string]interface{}{"id": global.ID, "conns": []int{otherConn}}, nil); code != 404 {
		t.Fatalf("foreign resolve: %d", code)
	}
	var list []Bookmark
	other.jsonDo("GET", "/api/bookmarks", nil, &list)
	if len(list) != 0 {
		t.Fatalf("other user sees %+v", list)
	}

	// resolve one bookmark for a broadcast group
	var resolved map[string]map[string]string
	if code := user.jsonDo("POST", "/api/bookmarks/resolve", map[string]interface{}{"id": global.ID, "conns": []int{conn, plain, otherConn}}, &resolved); code != 200 || len(resolved) != 2 ||
		resolved[strconv.Itoa(plain)]["cd"] != "cd -- '/opt/app/servers/plain/downloads'" {
		t.Fatalf("resolve: %d %v", code, resolved)
	}

	// rename + move between scopes, reorder
	var upd Bookmark
	if code := user.jsonDo("PUT", "/api/bookmarks/"+strconv.Itoa(folder.ID), map[string]interface{}{"name": "Logs", "path": "/srv/logs", "scope": "connection", "scope_id": plain}, &upd); code != 200 || upd.Scope != "connection" || upd.Name != "Logs" {
		t.Fatalf("update: %d %+v", code, upd)
	}
	user.jsonDo("POST", "/api/bookmarks/reorder", map[string]interface{}{"ids": []int{start.ID, tagged.ID, folder.ID, global.ID}}, nil)
	user.jsonDo("GET", "/api/bookmarks", nil, &list)
	if len(list) != 4 || list[0].ID != start.ID || list[3].ID != global.ID {
		t.Fatalf("reorder: %+v", list)
	}

	// duplicate copies connection bookmarks; delete removes them
	var dup map[string]interface{}
	user.jsonDo("POST", "/api/connections/"+strconv.Itoa(conn)+"/duplicate", nil, &dup)
	dupID := int(dup["id"].(float64))
	if n := len(loadBookmarks("scope='connection' AND scope_id=?", dupID)); n != 1 {
		t.Fatalf("duplicate copied %d bookmarks", n)
	}
	user.jsonDo("DELETE", "/api/connections/"+strconv.Itoa(dupID), nil, nil)
	if n := len(loadBookmarks("scope='connection' AND scope_id=?", dupID)); n != 0 {
		t.Fatalf("delete left %d bookmarks", n)
	}

	// export / import keeps bookmarks and maps their folder / connection
	var exp map[string]json.RawMessage
	user.jsonDo("GET", "/api/config/export", nil, &exp)
	if !strings.Contains(string(exp["bookmarks"]), "/opt/app/servers/{name}/downloads") {
		t.Fatalf("export without bookmarks: %s", exp["bookmarks"])
	}
	fresh := newTestUser(t, srv, "bm-fresh", false)
	var payload map[string]interface{}
	raw, _ := json.Marshal(exp)
	json.Unmarshal(raw, &payload)
	var ires map[string]int
	fresh.jsonDo("POST", "/api/config/import", payload, &ires)
	if ires["bookmarks"] != 4 {
		t.Fatalf("import result %v", ires)
	}
	for _, b := range loadBookmarks("user_id=?", fresh.userID) {
		if b.Scope == "connection" && !userOwnsConnection(b.ScopeID, fresh.userID) {
			t.Fatalf("imported bookmark not mapped: %+v", b)
		}
	}
	// a second import adds nothing
	fresh.jsonDo("POST", "/api/config/import", payload, &ires)
	if ires["bookmarks"] != 0 {
		t.Fatalf("re-import duplicated bookmarks: %v", ires)
	}

	// delete
	if code := user.jsonDo("DELETE", "/api/bookmarks/"+strconv.Itoa(tagged.ID), nil, nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action LIKE 'bookmark.%' AND user_id=?`, user.userID).Scan(&n)
	if n < 6 {
		t.Fatalf("bookmark audit entries: %d", n)
	}
}

func TestBookmarksThroughShare(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	owner := newTestUser(t, srv, "bm-owner", false)
	member := newTestUser(t, srv, "bm-member", false)
	conn := owner.addConnection("shared-01", "10.0.0.7")
	sh := mustShare(t, owner.userID, "members", RoleViewer, "")
	db.Exec(`INSERT INTO share_members (share_id, user_id, role) VALUES (?,?,?)`, sh.ID, member.userID, RoleViewer)
	db.Exec(`INSERT INTO share_items (share_id, connection_id) VALUES (?,?)`, sh.ID, conn)

	var shared, private Bookmark
	owner.jsonDo("POST", "/api/bookmarks", map[string]interface{}{"name": "Pub", "path": "/srv/{name}", "shared": true}, &shared)
	owner.jsonDo("POST", "/api/bookmarks", map[string]interface{}{"name": "Priv", "path": "/root/secret"}, &private)

	q := "/api/bookmarks?conn=" + strconv.Itoa(conn) + "&share_token=" + sh.Token
	var got struct {
		Bookmarks []Bookmark `json:"bookmarks"`
	}
	if code := member.jsonDo("GET", q, nil, &got); code != 200 || len(got.Bookmarks) != 1 || got.Bookmarks[0].ID != shared.ID ||
		got.Bookmarks[0].Mine || got.Bookmarks[0].Owner != "bm-owner" || got.Bookmarks[0].Resolved != "/srv/shared-01" {
		t.Fatalf("member view: %d %+v", code, got)
	}
	// read-only: a member cannot change the owner's bookmark, and resolves only shared ones
	if code := member.jsonDo("PUT", "/api/bookmarks/"+strconv.Itoa(shared.ID), map[string]interface{}{"path": "/tmp"}, nil); code != 404 {
		t.Fatalf("member update: %d", code)
	}
	rq := "/api/bookmarks/resolve?share_token=" + sh.Token
	if code := member.jsonDo("POST", rq, map[string]interface{}{"id": private.ID, "conns": []int{conn}}, nil); code != 404 {
		t.Fatalf("member resolved a private bookmark: %d", code)
	}
	if code := member.jsonDo("POST", rq, map[string]interface{}{"id": shared.ID, "conns": []int{conn}}, nil); code != 200 {
		t.Fatalf("member resolve shared: %d", code)
	}
	// without the token the connection is not theirs
	if code := member.jsonDo("GET", "/api/bookmarks?conn="+strconv.Itoa(conn), nil, &got); code != 404 {
		t.Fatalf("member without token: %d", code)
	}
}

func TestBookmarkCdAndVariables(t *testing.T) {
	c := Connection{Name: "web-01", Host: "[2001:db8::1]:22", Username: "ops"}
	if got := expandBookmarkPath("/d/{name}/{host}/$USER/${USER}/{user}/{other}", c); got != "/d/web-01/2001:db8::1/ops/ops/ops/{other}" {
		t.Fatalf("expand: %q", got)
	}
	c.Username = ""
	if got := expandBookmarkPath("/home/$USER", c); got != "/home/$USER" {
		t.Fatalf("expand without user: %q", got)
	}
	cases := map[string]string{
		"/opt/app":          "cd -- '/opt/app'",
		"~":                 "cd -- ~",
		"~/":                "cd -- ~/",
		"~/a b":             "cd -- ~/'a b'",
		"/x'; rm -rf / #":   `cd -- '/x'\''; rm -rf / #'`,
		"/$(reboot)/`id`":   "cd -- '/$(reboot)/`id`'",
		"~root/not-a-home":  "cd -- '~root/not-a-home'",
		"-rf":               "cd -- '-rf'",
		"/with\\backslash/": `cd -- '/with\backslash/'`,
	}
	for in, want := range cases {
		if got := bookmarkCd(in); got != want {
			t.Fatalf("cd %q: %q, want %q", in, got, want)
		}
	}
}

func TestParseWinSCPBookmarks(t *testing.T) {
	ini := "\uFEFF[Configuration\\Interface]\r\nRemote=/ignored\r\n" +
		"[Configuration\\Bookmarks\\Remote\\ops%40web.example.com]\r\n" +
		"Logs=/var/log\r\n" +
		"1=/opt/app/servers/myServer/downloads\r\n" +
		"With%20space=/srv/my%20dir\r\n" +
		"Options=1\r\n" +
		"[Configuration\\Bookmarks\\Local\\x]\r\nLocal=C:%5CUsers\r\n" +
		"[Configuration\\Bookmarks\\Remote\\other]\r\nLogs=/var/log\r\nHome=~/x\r\n"
	got := parseWinSCPBookmarks(ini)
	if len(got) != 4 {
		t.Fatalf("parsed %d: %+v", len(got), got)
	}
	if got[0].Name != "Logs" || got[0].Path != "/var/log" || got[0].Note != "WinSCP: ops@web.example.com" {
		t.Fatalf("first: %+v", got[0])
	}
	if got[1].Name != "downloads" || got[2].Name != "With space" || got[2].Path != "/srv/my dir" || got[3].Path != "~/x" {
		t.Fatalf("parsed: %+v", got)
	}

	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	user := newTestUser(t, srv, "bm-winscp", false)
	resp := user.do("POST", "/api/bookmarks/import-winscp", strings.NewReader(ini), "text/plain")
	var res map[string]int
	json.NewDecoder(resp.Body).Decode(&res)
	resp.Body.Close()
	if resp.StatusCode != 200 || res["imported"] != 4 {
		t.Fatalf("import: %d %v", resp.StatusCode, res)
	}
	resp = user.do("POST", "/api/bookmarks/import-winscp", strings.NewReader(ini), "text/plain")
	json.NewDecoder(resp.Body).Decode(&res)
	resp.Body.Close()
	if res["imported"] != 0 || res["skipped"] != 4 {
		t.Fatalf("re-import: %v", res)
	}
}

func TestStartDirectoryOnConnect(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	user := newTestUser(t, srv, "bm-start", false)
	conn := user.addConnection("app-02", sshAddr)
	for _, body := range []map[string]interface{}{
		{"name": "G", "path": "/global", "start_dir": true},
		{"name": "C", "path": "/opt/{name}/it's", "scope": "connection", "scope_id": conn, "start_dir": true},
	} {
		if code := user.jsonDo("POST", "/api/bookmarks", body, &map[string]interface{}{}); code != 200 {
			t.Fatalf("create: %d", code)
		}
	}
	user.jsonDo("POST", "/api/snippets", map[string]interface{}{"name": "after", "command": "echo after", "auto_run": true}, &map[string]interface{}{})
	tc := user.openTerminal(conn)
	tc.waitFor("you typed: echo after")
	tc.mu.Lock()
	out := tc.out.String()
	tc.mu.Unlock()
	iCd := strings.Index(out, `you typed: cd -- '/opt/app-02/it'\''s'`)
	if iCd < 0 || iCd > strings.Index(out, "you typed: echo after") || strings.Contains(out, "/global") {
		t.Fatalf("start directory cd missing or wrong:\n%s", out)
	}
	tc.send("exit\r")
	select {
	case <-tc.closed:
	case <-time.After(5 * time.Second):
	}
}
