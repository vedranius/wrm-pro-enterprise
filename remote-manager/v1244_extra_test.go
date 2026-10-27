// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestV1244HTTPProviderHeaderAndStreamCancellation(t *testing.T) {
	t.Setenv("WRM_AI_PROVIDER_HEADER_SECONDS", "30")
	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if r.URL.Path == "/stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
		}
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	}))
	defer srv.Close()
	defer close(stop)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := aiPost(ctx, srv.URL+"/headers", []byte(`{}`), nil, nil); err == nil || !strings.Contains(err.Error(), "request time limit") {
		t.Fatal(err)
	}
	_, err := aiPost(context.Background(), srv.URL+"/headers", []byte(`{}`), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "response-header time limit") {
		t.Fatal(err)
	}
	res, err := aiPost(context.Background(), srv.URL+"/stream", []byte(`{}`), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise a real blocked net/http Body.Read using a short idle period.
	res.Body.Close()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	req, _ := http.NewRequestWithContext(ctx2, "POST", srv.URL+"/stream", strings.NewReader(`{}`))
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body := newAIIdleBody(res.Body, 25*time.Millisecond, cancel2)
	defer body.Close()
	if _, err = io.ReadAll(body); err == nil || !strings.Contains(err.Error(), "stream interrupted") {
		t.Fatal(err)
	}
}
func TestV1244GitBlobVerification(t *testing.T) {
	data := []byte("demo\n")
	sum := sha1.Sum(append([]byte(fmt.Sprintf("blob %d\x00", len(data))), data...))
	hash := hex.EncodeToString(sum[:])
	if err := verifyRelayBlob(hash, data); err != nil {
		t.Fatal(err)
	}
	if verifyRelayBlob(hash, []byte("changed")) == nil || verifyRelayBlob("bad", data) == nil {
		t.Fatal("unverified client blob accepted")
	}
}
func TestV1244ReviewedTargetSurvivesRefresh(t *testing.T) {
	e := newDeployEnv(t, "review-snapshot")
	writeFile(t, e.dir+"/run.py", runV19)
	writeFile(t, e.dir+"/lib/util.py", utilV1)
	in := e.check(t)
	original, ok := loadGitTarget(e.u.userID, "demo-api")
	if !ok {
		t.Fatal("missing target")
	}
	ticket, err := storeGitReview(e.u.userID, in.ID, "update", "", original)
	if err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.Commit = strings.Repeat("f", 40)
	// Simulate a later failed refresh as well as changed reference identity.
	changed.Error = "new reference unavailable"
	db.Exec(`UPDATE git_targets SET data=? WHERE user_id=? AND app=?`, string(jsonMarshal(changed)), e.u.userID, "demo-api")
	req := httptest.NewRequest("POST", "/api/git/runs", nil)
	p := gitRunParams{Kind: "update", Items: []*gitRunItem{{InstallID: in.ID, ReviewID: ticket}}, ScheduleAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
	id, code, err := createGitRun(req, e.u.userID, p)
	if err != nil || code != 200 {
		t.Fatalf("run %d %d %v", id, code, err)
	}
	var raw string
	db.QueryRow(`SELECT params FROM git_runs WHERE id=?`, id).Scan(&raw)
	if !strings.Contains(raw, original.Commit) || strings.Contains(raw, changed.Commit) {
		t.Fatalf("review replaced: %s", raw)
	}
	if _, err = loadGitReview(e.u.userID, in.ID+1, "update", "", ticket); err == nil {
		t.Fatal("ticket allowed another installation")
	}
	if _, err = loadGitReview(e.u.userID+1, in.ID, "update", "", ticket); err == nil {
		t.Fatal("ticket allowed another user")
	}
}

func TestV1244MCPPromptPickerAndInventory(t *testing.T) {
	e := newMCPEnv(t)
	tok, _ := e.pat("read_only", []string{"read_logs", "run_readonly"}, nil)
	c := e.client(tok)
	c.initialize(false)
	prompts := c.request("prompts/list", map[string]interface{}{})
	if !strings.Contains(string(jsonMarshal(prompts)), `"wrm"`) {
		t.Fatal(prompts)
	}
	selected, bad := c.call("select_server", map[string]interface{}{})
	if bad || !strings.Contains(selected, "demo-api") {
		t.Fatal(selected, bad)
	}
	writeHostFile(t, e.host, "demo-config.ini", "[demo]\nenabled=true\npassword=private-demo-value\n")
	sid := c.open(e.connID)
	text, bad := c.call("inventory_files", map[string]interface{}{"session_id": sid, "path": e.host.home, "filter": "ini", "grep": "enabled"})
	if bad || !strings.Contains(text, "demo-config.ini") || strings.Contains(text, "private-demo-value") {
		t.Fatal(text, bad)
	}
}

func TestV1244ZabbixAPITLSAndActiveItems(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer demo-api-token" {
			w.WriteHeader(401)
			return
		}
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if req["method"] != "item.get" {
			t.Error(req)
		}
		out := []map[string]string{}
		for key, value := range map[string]string{"agent.ping": "1", "system.cpu.util": "25", "vm.memory.size[total]": "1000", "vm.memory.size[available]": "400", "system.uptime": "100", "net.if.in[eth0]": "120"} {
			out = append(out, map[string]string{"key_": key, "lastvalue": value, "lastclock": strconv.FormatInt(time.Now().Unix(), 10), "state": "0", "units": map[bool]string{true: "Bps"}[strings.HasPrefix(key, "net.if.")]})
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "result": out})
	}))
	defer srv.Close()
	cfg := defaultMonitorConfig()
	cfg.Mode = "api"
	cfg.APIURL = srv.URL
	cfg.APIHost = "42"
	cfg.APIToken = "demo-api-token"
	sample := monitorSample{At: nowRFC(), Metrics: map[string]monitorMetric{}}
	if monitorAPI(context.Background(), cfg, &sample) == nil {
		t.Fatal("untrusted CA accepted")
	}
	cfg.CA = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
	if err := monitorAPI(context.Background(), cfg, &sample); err != nil {
		t.Fatal(err)
	}
	finishMonitor(&sample)
	if sample.State != "healthy" || sample.Metrics["rate:net.if.in[eth0]"].Value == nil || *sample.Metrics["cpu_percent"].Value != 25 {
		t.Fatal(sample)
	}
	cfg.APIToken = "wrong"
	if monitorAPI(context.Background(), cfg, &sample) == nil {
		t.Fatal("API authentication ignored")
	}
}
func TestV1244TerminalLiteralDirectoryExecution(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "demo O'Brien;$(touch unexpected)")
	os.MkdirAll(dir, 0700)
	shell := filepath.Join(base, "demo-shell")
	os.WriteFile(shell, []byte("#!/bin/sh\npwd\n"), 0700)
	command, err := terminalDirectoryCommand(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(), "SHELL="+shell)
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != dir {
		t.Fatal(string(out), err)
	}
	if _, err := os.Stat(filepath.Join(base, "unexpected")); err == nil {
		t.Fatal("directory executed as code")
	}
}

func TestV1244LogTerminalRootAndLiteralPath(t *testing.T) {
	c := Connection{Options: map[string]string{"terminal_shell": "posix"}}
	file := "/var/log/demo O'Brien;$(touch unexpected).log"
	command, err := terminalDirectoryStart(nil, c, "/var/log", file)
	if err != nil || !strings.Contains(command, "tail -n 100 -F --") {
		t.Fatal(command, err)
	}
	if _, err := terminalDirectoryStart(nil, c, "", "/outside/demo.log"); err == nil {
		t.Fatal("log root ignored")
	}
	if _, err := terminalDirectoryStart(nil, c, "", "/var/log/demo\ncommand"); err == nil {
		t.Fatal("newline accepted")
	}
	c.Options["terminal_shell"] = "powershell"
	command, err = terminalDirectoryStart(nil, c, "", file)
	if err != nil || !strings.Contains(command, "-EncodedCommand") || strings.Contains(command, "touch") {
		t.Fatal(command, err)
	}
}
