// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"

	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestV1245AutomaticLogsAndEventTimes(t *testing.T) {
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := newTestServer(t)
	u := newTestUser(t, srv, "log-discovery", false)
	id := u.addConnection("demo-stream", h.addr)
	// /home/<operator>/<operator>/<service>/log, as in production.
	root := filepath.Join(h.home, "demo-operator", "demo-api", "log")
	os.MkdirAll(root, 0700)
	file := filepath.Join(root, "events.log")
	writeFile(t, file, "2026-10-15T10:00:00Z ERROR old unique-event\n2026-10-15T11:00:00Z INFO ready\n")
	txt := filepath.Join(root, "ftp-2026-10-15.txt")
	writeFile(t, txt, "2026-10-15T10:30:00Z INFO ftp upload\n")
	writeFile(t, filepath.Join(root, "config.ini"), "password=not-a-log")
	outside := filepath.Join(h.home, "outside.log")
	writeFile(t, outside, "private")
	os.Symlink(outside, filepath.Join(root, "escape.log"))
	cfg := defaultMonitorConfig()
	cfg.Roots = []monitorLogRoot{}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/monitoring/config?id=%d", id), cfg, nil); code != 200 {
		t.Fatal(code)
	}
	saveGitWorkspace(u.userID, &gitCatalog{ServerRoots: []string{h.home}}, &gitSettings{})
	var catalog struct {
		Files []monitorLogFile `json:"files"`
		Roots []monitorLogRoot `json:"roots"`
	}
	// Discovery is an explicit action since v13: opening the log list never scans.
	if code := u.jsonDo("POST", fmt.Sprintf("/api/monitoring/logs/discover?id=%d", id), nil, nil); code != 200 {
		t.Fatal(code)
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs?id=%d", id), nil, &catalog); code != 200 {
		t.Fatal(code)
	}
	var log, txtLog *monitorLogFile
	for i := range catalog.Files {
		f := &catalog.Files[i]
		if f.Path == file {
			log = f
		}
		if f.Path == txt {
			txtLog = f
		}
		if strings.HasSuffix(f.Path, "config.ini") || strings.HasSuffix(f.Path, "escape.log") {
			t.Fatalf("non-log/symlink offered: %s", f.Path)
		}
	}
	if log == nil || log.Service != "demo-api" || log.Operator != "demo-operator" || log.Modified == 0 {
		t.Fatalf("discovery: %+v", catalog)
	}
	if txtLog == nil || txtLog.Service != "demo-api" || txtLog.Operator != "demo-operator" {
		t.Fatalf("FTP .txt log not discovered: %+v", catalog)
	}
	endpoint := fmt.Sprintf("/api/monitoring/log?id=%d&root_path=%s&path=%s&scan=1&offset=0&timezone=UTC&q=unique-event", id, url.QueryEscape(root), url.QueryEscape(file))
	var chunk monitorLogChunk
	if code := u.jsonDo("GET", endpoint, nil, &chunk); code != 200 || chunk.Matches != 1 || chunk.FirstTime != "2026-10-15T10:00:00Z" || chunk.LastTime != chunk.FirstTime || chunk.Modified == 0 {
		t.Fatalf("event times: %d %+v", code, chunk)
	}
	var dateOnly monitorLogChunk
	if code := u.jsonDo("GET", strings.Replace(endpoint, "&q=unique-event", "", 1)+"&from=2026-10-15T09:00&to=2026-10-15T12:00", nil, &dateOnly); code != 200 || dateOnly.Matches != 2 || dateOnly.Rows != 2 {
		t.Fatalf("date-only search %d %+v", code, dateOnly)
	}
	if code := u.jsonDo("GET", endpoint+"&from=2026-10-16T00:00&to=2026-10-14T00:00", nil, nil); code != 400 {
		t.Fatal("reversed time range", code)
	}
	// Changing roots invalidates prior discovered roots; reads must rediscover.
	cfg.Roots = []monitorLogRoot{{Path: filepath.Join(h.home, "new-logs")}}
	u.jsonDo("PUT", fmt.Sprintf("/api/monitoring/config?id=%d", id), cfg, nil)
	if code := u.jsonDo("GET", endpoint, nil, nil); code != 400 {
		t.Fatal("stale root accepted", code)
	}
}
func TestV1245LogFilenameRecognition(t *testing.T) {
	for _, name := range []string{"app.log", "app.log.1", "app.log.2026-10-15", "worker.out", "worker.err", "syslog", "messages"} {
		if !monitorLogName(name) {
			t.Fatal(name)
		}
	}
	for _, name := range []string{"config.ini", "settings.py", "private.key", "image.png"} {
		if monitorLogName(name) {
			t.Fatal(name)
		}
	}
}
func TestV1245MediumCombinedUploadRetainedOnDependencyFailure(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "setup-owner", true)
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "medium.db"))
	t.Setenv("WRM_MEDIUM_RUNTIME", "docker")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("WRM_MEDIUM_DOCKER", "")
	var body bytes.Buffer
	mp := multipart.NewWriter(&body)
	for kind, files := range map[string]map[string]string{"tool": {"app/main.py": "# demo", "app/config.py": "# demo", "app/__init__.py": "", "requirements.lock": "# empty"}, "config": {"servers.yaml": mdServersYAML, "environments.yaml": mdEnvsYAML}} {
		part, _ := mp.CreateFormFile(kind, kind+".zip")
		part.Write(mediumTestZip(t, files))
	}
	mp.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/api/git/medium/setup?confirm=1", bytes.NewReader(body.Bytes()))
	req.AddCookie(u.cookie)
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("X-WRM-Request", "1")
	req.Header.Set("Origin", srv.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatal(resp.StatusCode)
	}
	for until := time.Now().Add(5 * time.Second); time.Now().Before(until) && mediumState(u.userID).Busy; {
		time.Sleep(10 * time.Millisecond)
	}
	s := mediumState(u.userID)
	if s.Busy || s.Error == "" || !s.Imported || s.Tool {
		t.Fatalf("failed setup: %+v", s)
	}
	if !mediumFileExists(filepath.Join(mediumUserDir(u.userID), "pending-config", "servers.yaml")) {
		t.Fatal("config ZIP lost on dependency failure")
	}
	locked := false
	for until := time.Now().Add(time.Second); time.Now().Before(until); {
		if mediumOperation.TryLock() {
			locked = true
			mediumOperation.Unlock()
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !locked {
		t.Fatal("setup lock not released")
	}
}
func TestV1245MediumReadyDockerNeedsNoHostChanges(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "docker")
	writeFile(t, script, "#!/bin/sh\n[ \"$1\" = version ] && echo linux && exit 0\nexit 99\n")
	os.Chmod(script, 0700)
	t.Setenv("WRM_MEDIUM_DOCKER", script)
	if err := mediumEnsureDocker(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestV1245ContinuationEventTime(t *testing.T) {
	first, last := monitorEventRange("2026-10-15T10:00:00Z started\n ERROR continuation\n", "error", time.Time{}, time.Time{}, time.UTC, time.Time{})
	if first != "2026-10-15T10:00:00Z" || last != first {
		t.Fatal(first, last)
	}
}

func TestV1245CombinedSetupRejectsActiveRuntimeWithoutNewUploads(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "medium-active", true)
	key := mediumUserDir(u.userID)
	mediumMgr.mu.Lock()
	mediumMgr.processes[key] = &mediumProcess{done: make(chan struct{})}
	mediumMgr.mu.Unlock()
	defer func() { mediumMgr.mu.Lock(); delete(mediumMgr.processes, key); mediumMgr.mu.Unlock() }()
	var body bytes.Buffer
	mp := multipart.NewWriter(&body)
	mp.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/api/git/medium/setup?confirm=1", &body)
	req.AddCookie(u.cookie)
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("X-WRM-Request", "1")
	req.Header.Set("Origin", srv.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatalf("active runtime was not protected: %d", resp.StatusCode)
	}
}
