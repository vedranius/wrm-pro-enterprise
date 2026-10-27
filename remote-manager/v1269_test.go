// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// v12.6.9: every WRM notification is kept in the user's in-app list, whatever their
// notification channels, and the list is per user.
func TestUserEventsInbox(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	a := newTestUser(t, srv, "wb-ev-a", false)
	b := newTestUser(t, srv, "wb-ev-b", false)

	list := func(c *testClient) (int, []userEvent, int) {
		resp := c.do("GET", "/api/me/events?limit=50", nil, "")
		defer resp.Body.Close()
		var out struct {
			Events []userEvent `json:"events"`
			Unread int         `json:"unread"`
		}
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out.Events, out.Unread
	}

	// notifyUser records even when no channel is configured (notifications_enabled off)
	notifyUser(a.userID, "status.down", func(lang string) (string, string) { return "demo-api is down", "connection refused" })
	notifyUser(a.userID, "status.up", func(lang string) (string, string) { return "demo-api is up", "" })
	code, ev, unread := list(a)
	if code != 200 || len(ev) != 2 || unread != 2 || ev[0].Title != "demo-api is up" || ev[1].Body != "connection refused" {
		t.Fatalf("inbox: %d %+v unread=%d", code, ev, unread)
	}
	if _, ev, _ := list(b); len(ev) != 0 {
		t.Fatalf("user b sees user a's events: %+v", ev)
	}

	// reminders are kept once a day, status changes every time
	for i := 0; i < 3; i++ {
		recordUserEvent(a.userID, "credential.expiring", "Key demo-key expires", "in 5 days")
		recordUserEvent(a.userID, "status.down", "demo-web is down", "")
	}
	if _, ev, _ := list(a); len(ev) != 2+1+3 {
		t.Fatalf("dedupe: %d events", len(ev))
	}

	// mark one read, then all
	id := ev[0].ID
	resp := a.do("POST", "/api/me/events/read", strings.NewReader(`{"ids":[`+itoa(id)+`]}`), "application/json")
	resp.Body.Close()
	if _, _, unread := list(a); unread != 5 {
		t.Fatalf("after marking one: unread=%d", unread)
	}
	// user b cannot mark a's events
	resp = b.do("POST", "/api/me/events/read", strings.NewReader(`{"all":true}`), "application/json")
	resp.Body.Close()
	if _, _, unread := list(a); unread != 5 {
		t.Fatalf("user b changed a's events: unread=%d", unread)
	}
	resp = a.do("POST", "/api/me/events/read", strings.NewReader(`{"all":true}`), "application/json")
	resp.Body.Close()
	if _, _, unread := list(a); unread != 0 {
		t.Fatalf("after marking all: unread=%d", unread)
	}
	resp = a.do("POST", "/api/me/events/read", strings.NewReader(`nope`), "application/json")
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("bad json: %d", resp.StatusCode)
	}

	// only the newest are kept
	for i := 0; i < userEventsKeep+15; i++ {
		recordUserEvent(b.userID, "status.down", "demo "+itoa(i), "")
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM user_events WHERE user_id=?`, b.userID).Scan(&n)
	if n != userEventsKeep {
		t.Fatalf("kept %d events, want %d", n, userEventsKeep)
	}

	// clear, and deleting the user removes the rest
	resp = a.do("DELETE", "/api/me/events", nil, "")
	resp.Body.Close()
	if _, ev, _ := list(a); len(ev) != 0 {
		t.Fatalf("not cleared: %+v", ev)
	}
	deleteUserCompletely(b.userID)
	db.QueryRow(`SELECT COUNT(*) FROM user_events WHERE user_id=?`, b.userID).Scan(&n)
	if n != 0 {
		t.Fatalf("events left after deleting the user: %d", n)
	}
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

// The SSH output of the metrics script (two /proc/stat reads one second apart).
func TestParseWorkboardMetrics(t *testing.T) {
	out := "Linux\n@@0.52 0.40 0.31 1/234 5678\n@@cpu  1000 0 500 8000 500 0 0 0 0 0\ncpu  1100 0 550 8300 550 0 0 0 0 0\n" +
		"@@MemTotal:       8000000 kB\nMemFree:  1000000 kB\nMemAvailable:    2000000 kB\n" +
		"@@Filesystem     1024-blocks      Used Available Capacity Mounted on\n/dev/sda1 100000 81000 19000 81% /\n/dev/sdb1 500000 50000 450000 10% /srv/data dir\n" +
		"/dev/loop0 1000 1000 0 100% /snap/core/1\n/dev/sda1 100000 81000 19000 81% /\ntmp 10 1 9 10% /run/user/1000\n" +
		"@@123456.78 400000.00\n@@4\n"
	m, err := parseWBMetrics(out)
	if err != nil {
		t.Fatal(err)
	}
	// busy 150 of 500 jiffies (idle+iowait grew by 350) → 30 %
	if m.CPU == nil || *m.CPU != 30 {
		t.Fatalf("cpu: %v", m.CPU)
	}
	if m.Memory == nil || *m.Memory != 75 || m.MemTotal != 8000000*1024 {
		t.Fatalf("memory: %v %d", m.Memory, m.MemTotal)
	}
	if m.Load == nil || *m.Load != 0.52 || m.CPUCount != 4 || m.UptimeSec != 123456 {
		t.Fatalf("load/cpus/uptime: %v %d %d", m.Load, m.CPUCount, m.UptimeSec)
	}
	if len(m.Disks) != 2 || m.Disks[0].Mount != "/" || m.Disks[0].Percent != 81 || m.Disks[1].Mount != "/srv/data dir" || m.Disks[1].SizeKB != 500000 {
		t.Fatalf("disks: %+v", m.Disks)
	}
	if _, err := parseWBMetrics("FreeBSD\n@@@@@@@@@@@@\n"); err == nil || !strings.Contains(err.Error(), "Monitoring") {
		t.Fatalf("non-Linux: %v", err)
	}
	if _, err := parseWBMetrics("garbage"); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestWorkboardMetricsEndpoint(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	addr := startTestSSHServer(t)
	a := newTestUser(t, srv, "wb-m-a", false)
	b := newTestUser(t, srv, "wb-m-b", false)
	id := a.addConnection("demo-api", addr)

	get := func(c *testClient, q string) (int, map[string]interface{}) {
		resp := c.do("GET", "/api/workboard/metrics?"+q, nil, "")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var out map[string]interface{}
		json.Unmarshal(raw, &out)
		return resp.StatusCode, out
	}
	// only the owner's connections
	if code, _ := get(b, "id="+itoa(id)); code != 404 {
		t.Fatalf("other user's connection: %d", code)
	}
	if code, _ := get(a, "id=999999"); code != 404 {
		t.Fatalf("missing connection: %d", code)
	}
	web, _ := db.Exec(`INSERT INTO connections (name, protocol, host, user_id) VALUES ('demo-web','HTTPS','https://example.com',?)`, a.userID)
	wid, _ := web.LastInsertId()
	if code, _ := get(a, "id="+itoa(int(wid))); code != 400 {
		t.Fatalf("web connection: %d", code)
	}

	// over SSH: the test server runs no commands, so the error is reported in the result
	code, out := get(a, "id="+itoa(id))
	if code != 200 || out["source"] != "ssh" || out["error"] == nil || out["error"] == "" {
		t.Fatalf("ssh metrics: %d %v", code, out)
	}

	// administrator turned SSH reading off
	setSetting("workboard_ssh_metrics", "0")
	defer setSetting("workboard_ssh_metrics", "1")
	if code, out := get(a, "id="+itoa(id)); code != 403 || !strings.Contains(out["error"].(string), "Monitoring") {
		t.Fatalf("policy off: %d %v", code, out)
	}

	// a fresh Monitoring sample is used without SSH (also with SSH reading off)
	cfg := defaultMonitorConfig()
	cfg.Enabled = true
	db.Exec(`INSERT INTO monitor_profiles(conn_id,user_id,config,updated_at) VALUES(?,?,?,?)`, id, a.userID, encryptValue(string(jsonMarshal(cfg))), nowRFC())
	v := func(x float64) *float64 { return &x }
	s := monitorSample{ConnID: id, At: time.Now().UTC().Format(time.RFC3339), State: "healthy", Metrics: map[string]monitorMetric{
		"cpu_percent": {Value: v(12.5)}, "memory_percent": {Value: v(40)}, "memory_total": {Value: v(4 << 30)}, "load": {Value: v(0.7)},
		"cpu_count": {Value: v(2)}, "uptime": {Value: v(3600)}, "disk:/": {Value: v(55)}, "disk:/var": {Value: v(91)}, "disk:/broken": {Error: "no data"},
	}}
	db.Exec(`INSERT INTO monitor_samples(conn_id,user_id,at,data) VALUES(?,?,?,?)`, id, a.userID, s.At, string(jsonMarshal(s)))
	code, out = get(a, "id="+itoa(id))
	disks, _ := out["disks"].([]interface{})
	if code != 200 || out["source"] != "zabbix" || out["cpu_percent"] != 12.5 || out["memory_total"] != float64(4<<30) || out["cpu_count"] != float64(2) || len(disks) != 2 {
		t.Fatalf("zabbix metrics: %d %v", code, out)
	}
	// an old sample is not used
	old := s
	old.At = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	db.Exec(`INSERT INTO monitor_samples(conn_id,user_id,at,data) VALUES(?,?,?,?)`, id, a.userID, old.At, string(jsonMarshal(old)))
	if code, _ := get(a, "id="+itoa(id)); code != 403 {
		t.Fatalf("stale sample used: %d", code)
	}
}

// The page loads the Workboard and the layout knows dynamic panels.
func TestWorkboardScriptWired(t *testing.T) {
	page, _ := os.ReadFile("static/index.html")
	js, _ := os.ReadFile("static/workboard.js")
	lay, _ := os.ReadFile("static/layout.js")
	for _, want := range []string{`<script src="/static/workboard.js"></script>`, `id="btn-workboard"`, `id="wb-badge"`, `wrmWorkboard.init()`, `wrmWorkboard.bus(m.type)`, `'btn-workboard'`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	for _, want := range []string{"/api/me/ui/workboard", "/api/me/events", "/api/workboard/metrics", "/api/git/activity", "wrmLayout.register"} {
		if !strings.Contains(string(js), want) {
			t.Errorf("workboard.js lacks %s", want)
		}
	}
	for _, want := range []string{"register, remove, placed", "wrm-layout-loaded"} {
		if !strings.Contains(string(lay), want) {
			t.Errorf("layout.js lacks %s", want)
		}
	}
}
