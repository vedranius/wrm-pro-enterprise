// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestV130LogDirectoryNames(t *testing.T) {
	yes := []string{"log", "logs", "LOG", "log-2026", "logs_archive", "service_log", "app-logs"}
	no := []string{"catalog", "dialog", "blog", "node_modules", "bin", "logging.conf"}
	for _, n := range yes {
		if !monitorLogDirName(n) {
			t.Fatalf("%q should be treated as a log folder", n)
		}
	}
	for _, n := range no {
		if monitorLogDirName(n) {
			t.Fatalf("%q should not be treated as a log folder", n)
		}
	}
}

func TestV130JournalRootConfig(t *testing.T) {
	cfg := defaultMonitorConfig()
	cfg.Roots = []monitorLogRoot{{Kind: "journal", Unit: "media-api.service", Service: "media-api"}}
	if err := cfg.normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.Roots[0].Path != "journal:media-api.service" {
		t.Fatalf("journal root was not canonicalized: %+v", cfg.Roots[0])
	}
	if cfg.Roots[0].Unit != "media-api.service" || cfg.Roots[0].Kind != "journal" {
		t.Fatalf("journal root lost its unit: %+v", cfg.Roots[0])
	}
	// A unit name reaches a remote command line, so anything outside the strict
	// pattern must be refused instead of quoted and hoped for.
	for _, unit := range []string{"", "bad unit", "x; rm -rf /", "../../etc/passwd", "a|b", "$(id)"} {
		cfg := defaultMonitorConfig()
		cfg.Roots = []monitorLogRoot{{Kind: "journal", Unit: unit}}
		if err := cfg.normalize(); err == nil {
			t.Fatalf("accepted unsafe journal unit %q", unit)
		}
	}
	// A file root that only looks like a journal address keeps the file rules.
	cfg = defaultMonitorConfig()
	cfg.Roots = []monitorLogRoot{{Path: "journal:media-api.service"}}
	if err := cfg.normalize(); err == nil {
		t.Fatal("a relative file root must be refused")
	}
}

func TestV130LogsNeedExplicitDiscovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		// The fake host exposes POSIX paths; like the other SSH log tests this
		// runs on the Linux CI runner.
		t.Skip("POSIX log roots")
	}
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := newTestServer(t)
	u := newTestUser(t, srv, "log-explicit", false)
	id := u.addConnection("demo-stream", h.addr)
	root := filepath.Join(h.home, "demo-operator", "media-api", "logs")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "app.log")
	writeFile(t, file, "2026-10-15T10:00:00Z INFO ready\n")
	cfg := defaultMonitorConfig()
	cfg.Roots = []monitorLogRoot{}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/monitoring/config?id=%d", id), cfg, nil); code != 200 {
		t.Fatalf("config: %d", code)
	}
	saveGitWorkspace(u.userID, &gitCatalog{ServerRoots: []string{h.home}}, &gitSettings{})
	var first struct {
		Files      []monitorLogFile `json:"files"`
		Discovered bool             `json:"discovered"`
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs?id=%d", id), nil, &first); code != 200 {
		t.Fatalf("list: %d", code)
	}
	if first.Discovered || len(first.Files) != 0 {
		t.Fatalf("opening the log list scanned the host: %+v", first)
	}
	var scan struct {
		Roots      []monitorLogRoot     `json:"roots"`
		Services   []monitorServiceInfo `json:"services"`
		Discovered bool                 `json:"discovered"`
	}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/monitoring/logs/discover?id=%d", id), nil, &scan); code != 200 {
		t.Fatalf("discover: %d", code)
	}
	if !scan.Discovered || len(scan.Roots) == 0 {
		t.Fatalf("discovery found nothing: %+v", scan)
	}
	found := false
	for _, r := range scan.Roots {
		if r.Path == root && r.Service == "media-api" {
			found = true
		}
	}
	if !found {
		t.Fatalf("service log folder not discovered: %+v", scan.Roots)
	}
	var second struct {
		Files      []monitorLogFile `json:"files"`
		Discovered bool             `json:"discovered"`
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs?id=%d", id), nil, &second); code != 200 {
		t.Fatalf("list after discovery: %d", code)
	}
	if !second.Discovered || len(second.Files) == 0 {
		t.Fatalf("discovered logs were not listed: %+v", second)
	}
	foundFile := false
	for _, f := range second.Files {
		if f.Path == file && f.Service == "media-api" {
			foundFile = true
		}
	}
	if !foundFile {
		t.Fatalf("the service log was not listed as media-api: %+v", second.Files)
	}
}

// A folder under a broad root that holds a *.log is a service log folder: it
// must show up as a service and all of its files (extensionless "info" too)
// must be listed under that service.
func TestV130FolderUnderBroadRootIsAService(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX log roots")
	}
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := newTestServer(t)
	u := newTestUser(t, srv, "log-broad-root", false)
	id := u.addConnection("demo-stream", h.addr)
	root := filepath.Join(h.home, "varlog")
	dir := filepath.Join(root, "media-api")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "error.log"), "2026-10-15T10:00:00Z ERROR boom\n")
	writeFile(t, filepath.Join(dir, "info"), "2026-10-15T10:00:01Z INFO started\n")
	// Another folder without any *.log file stays out of the service list.
	empty := filepath.Join(root, "private")
	if err := os.MkdirAll(empty, 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(empty, "state"), "x\n")

	cfg := defaultMonitorConfig()
	cfg.Roots = []monitorLogRoot{{Path: root}}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/monitoring/config?id=%d", id), cfg, nil); code != 200 {
		t.Fatalf("config: %d", code)
	}
	var scan struct {
		Services []monitorServiceInfo `json:"services"`
	}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/monitoring/logs/discover?id=%d", id), nil, &scan); code != 200 {
		t.Fatalf("discover: %d", code)
	}
	found := false
	for _, s := range scan.Services {
		if s.Name == "media-api" && s.Kind == "folder" {
			found = true
		}
		if s.Name == "private" {
			t.Fatalf("folder without log files was listed as a service: %+v", s)
		}
	}
	if !found {
		t.Fatalf("log folder under a broad root was not discovered: %+v", scan.Services)
	}
	var list struct {
		Files []monitorLogFile `json:"files"`
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs?id=%d", id), nil, &list); code != 200 {
		t.Fatalf("list: %d", code)
	}
	got := map[string]string{}
	for _, f := range list.Files {
		// The SSH server is the test host itself, so its own log folders may be
		// visible under other roots; only this test's service is asserted.
		if f.Service == "media-api" && (filepath.Base(f.Path) == "error.log" || filepath.Base(f.Path) == "info") {
			got[filepath.Base(f.Path)] = f.Service
		}
	}
	for _, name := range []string{"error.log", "info"} {
		if got[name] != "media-api" {
			t.Fatalf("%s not listed with its folder as service: %+v", name, list.Files)
		}
	}
	for _, f := range list.Files {
		if strings.HasSuffix(f.Path, "/private/state") {
			t.Fatalf("file from a folder without logs was listed: %+v", f)
		}
	}
}

// The assistant reads logs through WRM's own discovery: find_logs lists the
// services, read_log opens a service's newest log without a guessed path.
func TestV130AssistantLogToolsRegistered(t *testing.T) {
	for _, name := range []string{"find_logs", "read_log"} {
		spec, ok := aiToolSpecFor(name)
		if !ok || spec.Kind != "read" {
			t.Fatalf("%s is not a read tool: %+v", name, spec)
		}
		if !aiToolInScopes(name, []string{"read_logs"}) {
			t.Fatalf("%s is not covered by the read_logs scope", name)
		}
		if aiToolInScopes(name, []string{"run_readonly"}) {
			t.Fatalf("%s must need the read_logs scope", name)
		}
		offered := false
		for _, def := range aiToolDefsFor(aiModeReadOnly) {
			if def.Name == name {
				offered = true
			}
		}
		if !offered {
			t.Fatalf("%s is not offered in a read-only session", name)
		}
	}
}

func TestV130AssistantReadsServiceLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX log roots")
	}
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := newTestServer(t)
	u := newTestUser(t, srv, "ai-logs", false)
	id := u.addConnection("demo-stream", h.addr)
	root := filepath.Join(h.home, "varlog")
	two := filepath.Join(root, "media-api")
	if err := os.MkdirAll(two, 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(two, "error.log"), "2026-10-15T10:00:00Z ERROR boom\n")
	writeFile(t, filepath.Join(two, "info"), "2026-10-15T10:00:01Z INFO started\n")
	one := filepath.Join(root, "payment-api")
	if err := os.MkdirAll(one, 0700); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(one, "app.log")
	writeFile(t, app, "2026-10-15T10:00:00Z INFO started\n2026-10-15T10:00:01Z ERROR charge failed\n")

	cfg := defaultMonitorConfig()
	cfg.Roots = []monitorLogRoot{{Path: root}}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/monitoring/config?id=%d", id), cfg, nil); code != 200 {
		t.Fatalf("config: %d", code)
	}
	c, err := loadConnection(id)
	if err != nil {
		t.Fatal(err)
	}
	s := &aiSession{ID: 1, UserID: u.userID, Username: "ai-logs", ConnID: id, ConnName: c.Name, Host: c.Host,
		Mode: aiModeAsk, conn: c, home: h.home}
	ctx := context.Background()

	text, code, err := s.findLogs(ctx, aiToolInput{})
	if err != nil || code != 0 {
		t.Fatalf("find_logs: %d %v", code, err)
	}
	for _, want := range []string{"media-api", "payment-api", two} {
		if !strings.Contains(text, want) {
			t.Fatalf("find_logs missed %q:\n%s", want, text)
		}
	}

	// A service with two files resolves to both, newest first.
	files, unit, err := s.aiServiceLogs(ctx, "media-api")
	if err != nil || unit != "" {
		t.Fatalf("resolve: %v %q", err, unit)
	}
	if len(files) != 2 {
		t.Fatalf("expected error.log and info, got %+v", files)
	}

	pl, err := s.plan(ctx, "read_log", aiToolInput{Service: "payment-api", Grep: "ERROR"})
	if err != nil {
		t.Fatal(err)
	}
	out, rc, err := s.exec(ctx, pl.Script, "", 0)
	if err != nil || rc != 0 {
		t.Fatalf("read_log: %d %v", rc, err)
	}
	if !strings.Contains(out, "ERROR charge failed") || strings.Contains(out, "INFO started") {
		t.Fatalf("read_log did not read the filtered log:\n%s", out)
	}
	if !strings.Contains(out, "WRM-REAL "+app) {
		t.Fatalf("read_log skipped the sensitive-path check:\n%s", out)
	}

	// An explicit path works the same way.
	pl, err = s.plan(ctx, "read_log", aiToolInput{Path: filepath.Join(two, "error.log")})
	if err != nil {
		t.Fatal(err)
	}
	if out, rc, err = s.exec(ctx, pl.Script, "", 0); err != nil || rc != 0 || !strings.Contains(out, "ERROR boom") {
		t.Fatalf("read_log path: %d %v\n%s", rc, err, out)
	}
	// An unknown service is a clear tool error, not an empty read.
	if _, err := s.plan(ctx, "read_log", aiToolInput{Service: "does-not-exist"}); err == nil {
		t.Fatal("an unknown service was accepted")
	}
}

func TestV130JournalSourceIsListed(t *testing.T) {
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := newTestServer(t)
	u := newTestUser(t, srv, "log-journal", false)
	id := u.addConnection("demo-stream", h.addr)
	cfg := defaultMonitorConfig()
	cfg.Roots = []monitorLogRoot{{Kind: "journal", Unit: "media-api.service", Service: "media-api"}}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/monitoring/config?id=%d", id), cfg, nil); code != 200 {
		t.Fatalf("config: %d", code)
	}
	var list struct {
		Files []monitorLogFile `json:"files"`
		Roots []monitorLogRoot `json:"roots"`
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs?id=%d", id), nil, &list); code != 200 {
		t.Fatalf("list: %d", code)
	}
	if len(list.Files) != 1 || list.Files[0].Kind != "journal" || list.Files[0].Unit != "media-api.service" {
		t.Fatalf("journal source missing from the catalog: %+v", list)
	}
	if !strings.HasPrefix(list.Files[0].Path, "journal:") {
		t.Fatalf("unexpected journal path: %+v", list.Files[0])
	}
}
