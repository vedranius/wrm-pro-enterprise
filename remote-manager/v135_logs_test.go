// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestV135LogFileAccepted(t *testing.T) {
	s := defaultLogSettings()
	for name, want := range map[string]bool{
		"app.log": true, "error.log.1": true, "app.log.2026-10-22": true, "access_log": true, "error-log.txt": true,
		"syslog": true, "messages": true, "worker.out": true, "events.jsonl": true,
		"cleanup.py": false, "run.sh": false, "settings.yaml": false, "ruby_logo.aa": false, "catalog.json": false,
		"wtmp": false, "lastlog": false, "system.journal": false, "app.log.gz": false, ".hidden.log": false,
	} {
		if got := logFileAccepted(name, false, s); got != want {
			t.Errorf("logFileAccepted(%q) = %v, want %v", name, got, want)
		}
	}
	// Inside a log folder, extensionless and dated files are logs; scripts are not.
	for name, want := range map[string]bool{"info": true, "warn": true, "2026-10-22.txt": true, "rotate.py": false, "job.js": false, "data.db": false} {
		if got := logFileAccepted(name, true, s); got != want {
			t.Errorf("in log folder: logFileAccepted(%q) = %v, want %v", name, got, want)
		}
	}
	s.Compressed, s.Rotated = true, false
	if !logFileAccepted("app.log.gz", false, s) || logFileAccepted("app.log.1", false, s) {
		t.Error("compressed/rotated switches are not applied")
	}
	s.Extra = []string{"*.trc"}
	if !logFileAccepted("db.trc", false, s) {
		t.Error("extra name pattern not applied")
	}
}

func TestV135LogLabelPatterns(t *testing.T) {
	l := newLogLabeler(defaultLogSettings(), "/data/apps")
	for dir, want := range map[string][2]string{
		"/home/demo-project/ops-team/demo-api/logs":      {"demo-api", "ops-team"},
		"/home/ops-team/demo-api/logs":                   {"demo-api", "ops-team"},
		"/home/ops-team/demo-api/logs/archive":           {"demo-api", "ops-team"},
		"/home/demo-web/log":                             {"demo-web", ""},
		"/srv/ops-team/demo-web/logs":                    {"demo-web", "ops-team"},
		"/var/log/nginx":                                 {"nginx", ""},
		"/data/apps/ops-team/demo-api/logs":              {"demo-api", "ops-team"},
		"/var/www/shop/var/log":                          {"shop", ""},
		"/home/demo-project/ops-team/demo-api/app/logs/": {"demo-api", ""},
	} {
		svc, op := l.label(dir)
		if svc != want[0] || op != want[1] {
			t.Errorf("label(%s) = %q/%q, want %q/%q", dir, svc, op, want[0], want[1])
		}
	}
	custom := defaultLogSettings()
	custom.Patterns = []string{"/home/{project}/{operator}/{service}/**"}
	if svc, op := newLogLabeler(custom).label("/home/p/o/s/logs"); svc != "s" || op != "o" {
		t.Errorf("custom pattern not preferred: %s/%s", svc, op)
	}
	if _, err := parseLogPattern("/home/{nope}"); err == nil {
		t.Error("unknown placeholder accepted")
	}
	if _, err := parseLogPattern("/home/*"); err == nil {
		t.Error("pattern without a label accepted")
	}
}

func TestV135LogSettingsValidation(t *testing.T) {
	s := defaultLogSettings()
	if err := s.normalize(); err != nil {
		t.Fatalf("defaults: %v", err)
	}
	bad := []func(*logSettings){
		func(x *logSettings) { x.ScanPaths = []string{"relative"} },
		func(x *logSettings) { x.ScanPaths = []string{"/"} },
		func(x *logSettings) { x.Exclude = []string{"a/b"} },
		func(x *logSettings) { x.MaxDepth = 99 },
		func(x *logSettings) { x.Method = "rsh" },
		func(x *logSettings) { x.Timezone = "Mars/Base" },
		func(x *logSettings) { x.Roots = []monitorLogRoot{{Kind: "journal", Unit: "bad unit;rm"}} },
	}
	for i, f := range bad {
		x := defaultLogSettings()
		f(&x)
		if x.normalize() == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}

func TestV135MoreTimestampFormats(t *testing.T) {
	loc := time.UTC
	for line, want := range map[string]string{
		"2026-10-22 14:03:01,123 INFO x":                                     "2026-10-22T14:03:01Z",
		"[2026-10-22 14:03:01] ERROR y":                                      "2026-10-22T14:03:01Z",
		"2026/10/22 14:03:01 [error] 12#12: upstream timed out":              "2026-10-22T14:03:01Z",
		"22.10.2026 14:03:01 WARN z":                                         "2026-10-22T14:03:01Z",
		`127.0.0.1 - - [22/Oct/2026:14:03:01 +0000] "GET / HTTP/1.1" 200 12`: "2026-10-22T14:03:01Z",
		"INFO  [main] 2026-10-22T14:03:01Z started":                          "2026-10-22T14:03:01Z",
	} {
		at, ok := parseMonitorLogTime(line, loc)
		if !ok || at.UTC().Format(time.RFC3339) != want {
			t.Errorf("%q -> %v %v, want %s", line, at, ok, want)
		}
	}
}

// Discovery over SSH finds logs in any folder (no service needed), labels the
// operator from the path, skips scripts, and the grep endpoint searches them.
func TestV135FastDiscoveryAndGrep(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell")
	}
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := newTestServer(t)
	u := newTestUser(t, srv, "log-fast", false)
	id := u.addConnection("demo-fast", h.addr)
	base := filepath.Join(h.home, "apps")
	svc := filepath.Join(base, "ops-team", "demo-api", "logs")
	loose := filepath.Join(h.home, "misc", "mirror")
	for _, d := range []string{svc, loose} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(svc, "app.log"), "2026-10-20T10:00:00Z INFO ready\n2026-10-21T10:00:00Z ERROR database timeout\n")
	writeFile(t, filepath.Join(svc, "info"), "2026-10-21T11:00:00Z INFO queue drained\n")
	writeFile(t, filepath.Join(svc, "rotate.py"), "print('ERROR not a log')\n")
	writeFile(t, filepath.Join(loose, "transfer.log"), "22.10.2026 08:00:00 ERROR mirror failed\n")
	set := defaultLogSettings()
	set.ScanPaths = []string{base, filepath.Join(h.home, "misc")}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/monitoring/logs/settings?id=%d", id), set, nil); code != 200 {
		t.Fatalf("settings: %d", code)
	}
	var disc struct {
		Method string `json:"method"`
		Files  int    `json:"files"`
	}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/monitoring/logs/discover?id=%d", id), nil, &disc); code != 200 {
		t.Fatalf("discover: %d", code)
	}
	if disc.Method != "ssh" {
		t.Fatalf("expected the SSH discovery, got %+v", disc)
	}
	var list struct {
		Files []monitorLogFile `json:"files"`
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs?id=%d", id), nil, &list); code != 200 {
		t.Fatalf("list: %d", code)
	}
	got := map[string]monitorLogFile{}
	for _, f := range list.Files {
		got[filepath.Base(f.Path)] = f
	}
	if f := got["app.log"]; f.Service != "demo-api" || f.Operator != "ops-team" {
		t.Errorf("app.log labels: %+v", f)
	}
	if _, ok := got["info"]; !ok {
		t.Error("extensionless file in the log folder missing")
	}
	if _, ok := got["rotate.py"]; ok {
		t.Error("a script was listed as a log")
	}
	if f, ok := got["transfer.log"]; !ok || f.Service != "mirror" {
		t.Errorf("log outside any service folder missing or unlabeled: %+v", f)
	}
	var res struct {
		Hits []logGrepHit `json:"hits"`
	}
	body := map[string]interface{}{"files": []string{got["app.log"].Path, got["transfer.log"].Path, got["info"].Path}, "q": "error", "timezone": "UTC"}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/monitoring/logs/grep?id=%d", id), body, &res); code != 200 {
		t.Fatalf("grep: %d", code)
	}
	if len(res.Hits) != 2 {
		t.Fatalf("expected 2 hits, got %+v", res.Hits)
	}
	for _, hit := range res.Hits {
		if !strings.Contains(strings.ToLower(hit.Text), "error") || hit.Line == 0 || hit.Time == "" {
			t.Errorf("bad hit %+v", hit)
		}
	}
	body["from"] = "2026-10-22T00:00"
	if code := u.jsonDo("POST", fmt.Sprintf("/api/monitoring/logs/grep?id=%d", id), body, &res); code != 200 || len(res.Hits) != 1 || !strings.Contains(res.Hits[0].Text, "mirror") {
		t.Fatalf("time range not applied: %d %+v", code, res.Hits)
	}
	var lines struct {
		First int    `json:"first"`
		Text  string `json:"text"`
		Total int    `json:"total"`
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs/lines?id=%d&mode=around&path=%s&line=2&before=0&after=0", id, got["app.log"].Path), nil, &lines); code != 200 || lines.First != 2 || lines.Total != 2 || !strings.Contains(lines.Text, "database timeout") || strings.Contains(lines.Text, "ready") {
		t.Fatalf("lines: %d %+v", code, lines)
	}
	var tail struct {
		First int    `json:"first"`
		Text  string `json:"text"`
		Bytes int64  `json:"bytes"`
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs/lines?id=%d&path=%s&n=1", id, got["app.log"].Path), nil, &tail); code != 200 || tail.First != 2 || !strings.HasPrefix(tail.Text, "2026-10-21") || tail.Bytes == 0 {
		t.Fatalf("tail: %d %+v", code, tail)
	}
	appendFile := func(p, s string) {
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(s)
		f.Close()
	}
	appendFile(filepath.Join(svc, "app.log"), "2026-10-21T12:00:00Z WARN slow\npartial")
	var since struct {
		Text  string `json:"text"`
		Bytes int64  `json:"bytes"`
		Reset bool   `json:"reset"`
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs/lines?id=%d&path=%s&mode=since&bytes=%d", id, got["app.log"].Path, tail.Bytes), nil, &since); code != 200 || since.Text != "2026-10-21T12:00:00Z WARN slow" || since.Bytes != tail.Bytes+int64(len("2026-10-21T12:00:00Z WARN slow\n")) || since.Reset {
		t.Fatalf("since: %d %+v", code, since)
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs/lines?id=%d&path=%s&mode=since&bytes=99999999", id, got["app.log"].Path), nil, &since); code != 200 || !since.Reset {
		t.Fatalf("shrunk file not reported: %d %+v", code, since)
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs/lines?id=%d&path=/etc/passwd&line=1", id), nil, nil); code != 400 {
		t.Fatalf("undiscovered path read: %d", code)
	}
	// A path outside the discovered list is never searched.
	body = map[string]interface{}{"files": []string{"/etc/passwd"}, "q": "root"}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/monitoring/logs/grep?id=%d", id), body, nil); code != 400 {
		t.Fatalf("undiscovered path searched: %d", code)
	}
}
