// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQuickConnectAndNotes(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "quick-user", false)
	sshAddr := startTestSSHServer(t)
	var v connView
	if code := u.jsonDo("POST", "/api/connections/quick", map[string]interface{}{"target": testSSHUser + "@" + sshAddr, "password": testSSHPass}, &v); code != 200 ||
		!v.Temporary || v.Protocol != "SSH" || v.Username != testSSHUser || v.Host != sshAddr || !strings.HasPrefix(v.Name, "⚡ tester@127.0.0.1") {
		t.Fatalf("quick connect: %d %+v", code, v)
	}
	tc := u.openTerminal(v.ID)
	tc.waitFor("Welcome to the test server")
	tc.ws.Close()
	for target, want := range map[string]string{"rdp://admin@10.0.0.5": "RDP 10.0.0.5 admin", "https://10.0.0.6/ui": "HTTPS 10.0.0.6 ", "telnet://sw1:2323": "TELNET sw1:2323 "} {
		var q connView
		if code := u.jsonDo("POST", "/api/connections/quick", map[string]interface{}{"target": target}, &q); code != 200 || q.Protocol+" "+q.Host+" "+q.Username != want {
			t.Errorf("quick %s: %d %+v", target, code, q)
		}
	}
	for _, bad := range []string{"", "two hosts", "host/../x"} {
		if code := u.jsonDo("POST", "/api/connections/quick", map[string]interface{}{"target": bad}, &map[string]interface{}{}); code != 400 {
			t.Errorf("bad target %q accepted: %d", bad, code)
		}
	}
	// quick connections are not exported
	r := u.do("GET", "/api/config/export", nil, "")
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if strings.Contains(string(body), "⚡") {
		t.Fatalf("quick connections exported")
	}
	// saving makes it a normal connection, with notes
	notes := "## Runbook\nRestart: `systemctl restart app`\nOn call: +385 1 234 5678"
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/connections/%d", v.ID), map[string]interface{}{"name": "app-01", "host": sshAddr, "username": testSSHUser,
		"temporary": false, "notes": notes}, nil); code != 200 {
		t.Fatalf("save: %d", code)
	}
	id := v.ID
	v = connView{}
	u.jsonDo("GET", fmt.Sprintf("/api/connections/%d", id), nil, &v)
	if v.Temporary || v.Notes != notes || !v.HasNotes || !v.Monitor {
		t.Fatalf("saved: %+v", v)
	}
	var list []connView
	u.jsonDo("GET", "/api/connections", nil, &list)
	for _, c := range list {
		if c.ID == v.ID && (c.Notes != "" || !c.HasNotes) {
			t.Fatalf("list must flag notes without sending them: %+v", c)
		}
	}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/connections/%d", v.ID), map[string]interface{}{"name": "app-01", "host": sshAddr, "notes": strings.Repeat("x", maxNotesLength+1)}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("too long notes: %d", code)
	}
	var dup connView
	u.jsonDo("POST", fmt.Sprintf("/api/connections/%d/duplicate", v.ID), nil, &dup)
	if loadNotes(dup.ID) != notes {
		t.Fatalf("duplicate lost the notes")
	}
	// expired quick connections go away, unless a terminal is open
	var q1, q2 connView
	u.jsonDo("POST", "/api/connections/quick", map[string]interface{}{"target": "old-host"}, &q1)
	u.jsonDo("POST", "/api/connections/quick", map[string]interface{}{"target": testSSHUser + "@" + sshAddr, "password": testSSHPass}, &q2)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	db.Exec(`UPDATE connections SET temp_until=? WHERE id IN (?,?)`, past, q1.ID, q2.ID)
	tc = u.openTerminal(q2.ID)
	tc.waitFor("Welcome")
	db.Exec(`UPDATE connections SET temp_until=? WHERE id=?`, past, q2.ID)
	cleanupQuickConnections()
	if userOwnsConnection(q1.ID, u.userID) {
		t.Fatalf("expired quick connection kept")
	}
	if !userOwnsConnection(q2.ID, u.userID) {
		t.Fatalf("quick connection with an open terminal deleted")
	}
	tc.ws.Close()
	// at most maxQuickPerUser
	for i := 0; i < maxQuickPerUser+3; i++ {
		u.jsonDo("POST", "/api/connections/quick", map[string]interface{}{"target": fmt.Sprintf("10.9.0.%d", i)}, nil)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM connections WHERE user_id=? AND temp_until<>''`, u.userID).Scan(&n)
	if n != maxQuickPerUser {
		t.Fatalf("quick connections: %d", n)
	}
}

func TestNetworkTools(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "tools-user", false)
	other := newTestUser(t, srv, "tools-other", false)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	openPort := ln.Addr().(*net.TCPAddr).Port
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	closedPort := closed.Addr().(*net.TCPAddr).Port
	closed.Close()
	type portsResp struct {
		Source string       `json:"source"`
		Ports  []portResult `json:"ports"`
		Error  string       `json:"error"`
	}
	check := func(source int) portsResp {
		var pr portsResp
		if code := u.jsonDo("POST", "/api/nettools", map[string]interface{}{"tool": "ports", "target": "127.0.0.1", "ports": fmt.Sprintf("%d, %d", openPort, closedPort), "source_id": source}, &pr); code != 200 {
			t.Fatalf("ports: %d %+v", code, pr)
		}
		return pr
	}
	pr := check(0)
	if pr.Source != "WRM server" || len(pr.Ports) != 2 || pr.Ports[0].State+pr.Ports[1].State != map[bool]string{true: "openclosed", false: "closedopen"}[openPort < closedPort] {
		t.Fatalf("ports from WRM: %+v", pr)
	}
	sshAddr := startTestSSHServer(t)
	jump := u.addConnection("app-01", sshAddr)
	before := directTCPIPCount.Load()
	pr = check(jump)
	states := map[int]string{}
	for _, p := range pr.Ports {
		states[p.Port] = p.State
	}
	if pr.Source != "app-01" || states[openPort] != "open" || states[closedPort] != "closed" || directTCPIPCount.Load() <= before {
		t.Fatalf("ports from a server: %+v", pr)
	}
	// HTTP / TLS check, also from the server
	hs := httptest.NewTLSServer(nil)
	defer hs.Close()
	for _, src := range []int{0, jump} {
		var hr struct {
			HTTP  *httpResult `json:"http"`
			Error string      `json:"error"`
		}
		u.jsonDo("POST", "/api/nettools", map[string]interface{}{"tool": "http", "target": hs.URL + "/x", "source_id": src}, &hr)
		if hr.HTTP == nil || hr.HTTP.Status != 404 || hr.HTTP.Trusted || hr.HTTP.TrustError == "" || !strings.HasPrefix(hr.HTTP.TLS, "TLS 1.") || hr.HTTP.DaysLeft <= 0 {
			t.Fatalf("http check (source %d): %+v %s", src, hr.HTTP, hr.Error)
		}
	}
	// DNS on the WRM server
	var dr struct {
		DNS *dnsResult `json:"dns"`
	}
	u.jsonDo("POST", "/api/nettools", map[string]interface{}{"tool": "dns", "target": "localhost"}, &dr)
	if dr.DNS == nil || len(dr.DNS.Addresses) == 0 {
		t.Fatalf("dns: %+v", dr.DNS)
	}
	// commands on a server (real sh on the fake host): ping / traceroute answer, also when missing
	h := startFakeHost(t, "ops", "ops-pass")
	src := u.addConnection("ops-box", h.addr)
	db.Exec(`UPDATE connections SET username='ops', password=? WHERE id=?`, encryptValue("ops-pass"), src)
	for _, tool := range []string{"ping", "traceroute"} {
		var out map[string]interface{}
		if code := u.jsonDo("POST", "/api/nettools", map[string]interface{}{"tool": tool, "target": "127.0.0.1", "source_id": src}, &out); code != 200 || fmt.Sprint(out["output"]) == "" {
			t.Fatalf("%s from a server: %d %v", tool, code, out)
		}
	}
	// validation, ownership, policy
	for _, bad := range []map[string]interface{}{
		{"tool": "ping", "target": "-oProxyCommand=x"},
		{"tool": "ping", "target": "a;reboot"},
		{"tool": "ports", "target": "127.0.0.1", "ports": "0"},
		{"tool": "ports", "target": "127.0.0.1", "ports": "1-200"},
		{"tool": "teleport", "target": "x"},
		{"tool": "http", "target": "ftp://x/"},
	} {
		var e map[string]interface{}
		if code := u.jsonDo("POST", "/api/nettools", bad, &e); code == 200 && e["error"] == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	if code := other.jsonDo("POST", "/api/nettools", map[string]interface{}{"tool": "ports", "target": "127.0.0.1", "ports": "22", "source_id": jump}, &map[string]interface{}{}); code != 404 {
		t.Fatalf("someone else's connection as source: %d", code)
	}
	if _, err := setSetting("network_tools", "admins"); err != nil {
		t.Fatal(err)
	}
	defer setSetting("network_tools", "all")
	if code := u.jsonDo("POST", "/api/nettools", map[string]interface{}{"tool": "dns", "target": "localhost"}, &map[string]interface{}{}); code != 403 {
		t.Fatalf("policy admins: %d", code)
	}
}

func TestPWAEndpoints(t *testing.T) {
	srv := newTestServer(t)
	res, err := srv.Client().Get(srv.URL + "/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	json.NewDecoder(res.Body).Decode(&m)
	res.Body.Close()
	if m["start_url"] != "/" || m["display"] != "standalone" || !strings.Contains(res.Header.Get("Content-Type"), "manifest+json") {
		t.Fatalf("manifest: %v %s", m, res.Header.Get("Content-Type"))
	}
	res, _ = srv.Client().Get(srv.URL + "/sw.js")
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(body), "wrm-"+AppVersion) || !strings.Contains(res.Header.Get("Content-Type"), "javascript") || !strings.Contains(string(body), "'/static/'") && !strings.Contains(string(body), "/static/") {
		t.Fatalf("service worker: %s", res.Header.Get("Content-Type"))
	}
	// the page links the manifest
	res, _ = srv.Client().Get(srv.URL + "/")
	page, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(page), `rel="manifest"`) {
		t.Fatalf("index has no manifest link")
	}
}
