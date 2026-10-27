// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mediumUploadZip posts a ZIP to /api/git/medium/{kind}?confirm=1.
func mediumUploadZip(t *testing.T, c *testClient, kind string, files map[string]string) (int, map[string]interface{}) {
	t.Helper()
	var body bytes.Buffer
	mp := multipart.NewWriter(&body)
	part, _ := mp.CreateFormFile("file", kind+".zip")
	part.Write(mediumTestZip(t, files))
	mp.Close()
	resp := c.do("POST", "/api/git/medium/"+kind+"?confirm=1", &body, mp.FormDataContentType())
	defer resp.Body.Close()
	var out map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// mediumFakeDocker writes a docker stand-in: version reports linux, the runtime setup
// prints pip-like output (and a secret, which must come back redacted), config checks pass
// and the original tool fails to start with a traceback.
func mediumFakeDocker(t *testing.T) {
	t.Helper()
	if os.PathSeparator == '\\' {
		t.Skip("shell fixture")
	}
	script := filepath.Join(t.TempDir(), "docker")
	writeFile(t, script, `#!/bin/sh
case "$1" in
 version) echo linux; exit 0;;
 run)
   case "$*" in
     *"pip install"*) echo "Collecting demo-package"; echo "token=zzSECRETzz99"; echo "Successfully installed demo-package"; exit 0;;
     *load_config*) if [ -n "$WRM_TEST_REJECT" ]; then echo "ValueError: servers.yaml: bad"; exit 1; fi; echo "config OK"; exit 0;;
     *uvicorn*) echo "Traceback (most recent call last):"; echo "ModuleNotFoundError: No module named 'app.deploy'"; exit 1;;
   esac;;
esac
exit 0
`)
	os.Chmod(script, 0700)
	t.Setenv("WRM_MEDIUM_RUNTIME", "docker")
	t.Setenv("WRM_MEDIUM_DOCKER", script)
}

func mediumWaitIdle(t *testing.T, uid int) mediumStatus {
	t.Helper()
	for until := time.Now().Add(20 * time.Second); time.Now().Before(until); time.Sleep(20 * time.Millisecond) {
		if s := mediumStateFull(uid); s.Operation == nil && !s.Busy {
			return s
		}
	}
	t.Fatal("Medium operation did not finish")
	return mediumStatus{}
}

var mediumGoodConfig = map[string]string{"servers.yaml": mdServersYAML, "environments.yaml": mdEnvsYAML}
var mediumTool = map[string]string{"app/main.py": "# demo", "app/config.py": "# demo", "app/__init__.py": "", "requirements.lock": "# demo"}

// The whole guided path: tool import prepares the runtime step by step (with output),
// deploy-config is checked by WRM at once and kept while the lock is held, a failed start
// keeps its steps, error and the tool's output, and everything is in the activity log.
func TestV1265MediumStepsActivityAndOutput(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "medium.db"))
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "medium-guide-admin", true)
	mediumFakeDocker(t)

	// an invalid deploy-config is refused at once, with WRM's reason
	if code, v := mediumUploadZip(t, admin, "config", map[string]string{"servers.yaml": "servers: []\n", "environments.yaml": mdEnvsYAML}); code != 400 || !strings.Contains(v["error"].(string), "servers.yaml") {
		t.Fatalf("invalid config: %d %v", code, v)
	}
	// a valid one before the runtime exists is kept for the first start
	if code, v := mediumUploadZip(t, admin, "config", mediumGoodConfig); code != 202 || v["pending"] != true {
		t.Fatalf("config before the runtime: %d %v", code, v)
	}
	if s := mediumState(admin.userID); !s.PendingConfig || s.Config {
		t.Fatalf("pending config: %+v", s)
	}
	// the tool import prepares the runtime in steps
	if code, v := mediumUploadZip(t, admin, "tool", mediumTool); code != 202 {
		t.Fatalf("tool: %d %v", code, v)
	}
	s := mediumWaitIdle(t, admin.userID)
	if !s.Tool || s.Op != "tool_import" || s.Error != "" {
		t.Fatalf("after the tool import: %+v", s)
	}
	steps := []string{}
	for _, st := range s.Steps {
		if st.Status != "ok" || st.Started == "" {
			t.Fatalf("step %+v", st)
		}
		steps = append(steps, st.Step)
	}
	if strings.Join(steps, ",") != "tool_unpack,docker,image,packages" {
		t.Fatalf("steps: %v", steps)
	}
	// start: the kept config is checked by the original tool and applied, then the tool
	// fails to come up; the status keeps the failed step and the error
	resp := admin.do("POST", "/api/git/medium/start", nil, "application/json")
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatalf("start: %d", resp.StatusCode)
	}
	s = mediumWaitIdle(t, admin.userID)
	if s.Running || s.Error == "" || !s.Config || s.PendingConfig {
		t.Fatalf("failed start: %+v", s)
	}
	last := s.Steps[len(s.Steps)-1]
	if last.Status != "error" || !strings.HasPrefix(last.Step, "start_") || s.Steps[0].Step != "docker" || s.Steps[1].Step != "config_check" {
		t.Fatalf("start steps: %+v", s.Steps)
	}
	// the activity log: newest first, with steps and redacted output
	// (the test database is shared: an administrator sees other tests' rows too)
	rows := []mediumActivityRow{}
	ops := []string{}
	for _, r := range mediumActivity(admin.userID, 50, true) {
		if r.Mine {
			rows = append(rows, r)
			ops = append(ops, r.Op+":"+r.Status)
		}
	}
	if strings.Join(ops, " ") != "start:error tool_import:ok config_import:ok config_import:error" {
		t.Fatalf("activity: %v", ops)
	}
	if rows[0].Output == nil || !strings.Contains(*rows[0].Output, "ModuleNotFoundError") || len(rows[0].Steps) == 0 {
		t.Fatalf("start output: %+v", rows[0])
	}
	if rows[1].Output == nil || !strings.Contains(*rows[1].Output, "Collecting demo-package") || strings.Contains(*rows[1].Output, "zzSECRETzz99") || rows[1].Ms <= 0 {
		t.Fatalf("tool import output: %+v", rows[1])
	}
}

// Another user's click while an operation runs says what runs, by whom and since when;
// that user's own deploy-config is kept meanwhile instead of failing.
func TestV1265MediumBusyNamesHolder(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "medium.db"))
	t.Setenv("WRM_GIT_MEDIUM", "all")
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "medium-holder", true)
	user := newTestUser(t, srv, "medium-waiter", false)
	mediumFakeDocker(t)
	os.MkdirAll(filepath.Join(mediumRoot()), 0700)
	writeFile(t, filepath.Join(mediumRoot(), "docker-ready"), mediumImage())
	os.MkdirAll(filepath.Join(mediumToolDir(), "app"), 0700)
	writeFile(t, filepath.Join(mediumToolDir(), "app", "main.py"), "# demo")

	h, err := mediumBeginOp(admin.userID, "prepare", "")
	if err != nil {
		t.Fatal(err)
	}
	mediumSetStage(admin.userID, "packages", true, nil)
	var v map[string]interface{}
	if code := user.jsonDo("POST", "/api/git/medium/start", map[string]string{}, &v); code != 409 || !strings.Contains(v["error"].(string), "medium-holder") || !strings.Contains(v["error"].(string), "runtime preparation") {
		t.Fatalf("busy start: %d %v", code, v)
	}
	var st mediumStatus
	user.jsonDo("GET", "/api/git/medium", nil, &st)
	if st.Operation == nil || st.Operation.User != "medium-holder" || st.Operation.Mine || st.Operation.Op != "prepare" {
		t.Fatalf("holder for the other user: %+v", st.Operation)
	}
	if code, v := mediumUploadZip(t, user, "config", mediumGoodConfig); code != 202 || !strings.Contains(v["note"].(string), "another Medium operation") {
		t.Fatalf("config while busy: %d %v", code, v)
	}
	mediumEndOp(h, nil)
	if mediumHolder(user.userID) != nil || !mediumOperation.TryLock() {
		t.Fatal("lock not released")
	}
	mediumOperation.Unlock()
	// rows: the user sees their own and the shared runtime's, not other users' own
	mediumRecord(admin.userID, "start", "ok", 5, "", "", "secret=hidden-value-1")
	seen := map[string]bool{}
	for _, r := range mediumActivity(user.userID, 50, true) {
		seen[r.Op+":"+r.User] = true
		if r.Op == "prepare" && r.Output == nil {
			t.Fatal("shared runtime output hidden from the user under the default policy")
		}
	}
	if !seen["prepare:medium-holder"] || !seen["config_import:medium-waiter"] || seen["start:medium-holder"] {
		t.Fatalf("visible rows: %v", seen)
	}
	// policy: only administrators see output
	t.Setenv("WRM_MEDIUM_LOG_OUTPUT", "admins")
	for _, r := range mediumActivity(user.userID, 50, true) {
		if r.Output != nil {
			t.Fatalf("output shown to a user with medium_log_output=admins: %+v", r)
		}
	}
	for _, r := range mediumActivity(admin.userID, 50, true) {
		if r.Op == "start" && (r.Output == nil || strings.Contains(*r.Output, "hidden-value-1")) {
			t.Fatalf("admin output: %+v", r)
		}
	}
}

// A deploy-config the original tool rejects is discarded and the previous one stays in use;
// a check that cannot run keeps the ZIP. Actions in the original tool are logged with its answer.
func TestV1265RejectedConfigAndKeptZip(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "medium.db"))
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "medium-reject-admin", true)
	mediumFakeDocker(t)
	if code, _ := mediumUploadZip(t, admin, "tool", mediumTool); code != 202 {
		t.Fatalf("tool: %d", code)
	}
	mediumWaitIdle(t, admin.userID)
	if code, v := mediumUploadZip(t, admin, "config", mediumGoodConfig); code != 200 {
		t.Fatalf("config with a ready runtime: %d %v", code, v)
	}
	t.Setenv("WRM_TEST_REJECT", "1")
	code, v := mediumUploadZip(t, admin, "config", mediumGoodConfig)
	if code != 400 || !strings.Contains(v["error"].(string), "previous deploy-config stays in use") {
		t.Fatalf("rejected config: %d %v", code, v)
	}
	if s := mediumState(admin.userID); s.PendingConfig || !s.Config {
		t.Fatalf("a rejected config must be discarded, the applied one kept: %+v", s)
	}
	// Docker unavailable: the check cannot run, the ZIP is kept for a retry
	t.Setenv("WRM_TEST_REJECT", "")
	t.Setenv("WRM_MEDIUM_DOCKER", filepath.Join(t.TempDir(), "missing-docker"))
	code, v = mediumUploadZip(t, admin, "config", mediumGoodConfig)
	if code != 400 || strings.Contains(v["error"].(string), "rejected") {
		t.Fatalf("check without Docker: %d %v", code, v)
	}
	if !mediumState(admin.userID).PendingConfig {
		t.Fatal("a config whose check could not run must stay kept")
	}
}

// A request kept for later can still be approved after the AI app closed its session; the
// approval then covers the same call once in the next session on that connection.
func TestV1265ApproveLaterAfterSessionClosed(t *testing.T) {
	e := newMCPEnv(t)
	tok, _ := e.pat("ask", []string{"read_logs", "run_with_approval"}, nil)
	c := e.client(tok)
	c.initialize(true)
	asked := 0
	c.onRequest = func(method string, params map[string]interface{}) map[string]interface{} {
		asked++
		return map[string]interface{}{"action": "decline"}
	}
	sid := c.open("demo-api")
	s := mcpSessionOf(sid)
	args := map[string]interface{}{"session_id": sid, "command": "touch ~/after-close", "reason": "test"}
	if _, isErr := c.call("run_command", args); !isErr {
		t.Fatal("declined call ran")
	}
	a := pendingOf(s)
	if a == nil || !a.Later {
		t.Fatalf("not kept for later: %+v", a)
	}
	c.call("close_session", map[string]interface{}{"session_id": sid})
	code, v := e.user.aiDo("GET", "/api/mcp/active", nil)
	if code != 200 || !strings.Contains(fmt.Sprint(v), a.ID) {
		t.Fatalf("the kept request of the closed session is not listed: %d %v", code, v)
	}
	if code, v := e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/approvals/%s", s.ID, a.ID), map[string]interface{}{"decision": "approve"}); code != 200 {
		t.Fatalf("approve after the session closed: %d %v", code, v)
	}
	sid2 := c.open("demo-api")
	args["session_id"] = sid2
	if text, isErr := c.call("run_command", args); isErr || !aiFileExists(filepath.Join(e.host.home, "after-close")) || asked != 1 {
		t.Fatalf("repeated in a new session: %v %s (asked %d)", isErr, text, asked)
	}
	// used once
	c.call("run_command", args)
	if asked != 2 {
		t.Fatalf("the approval was used twice (asked %d)", asked)
	}
}

// Patterns are not files: a grep regular expression must not look like a key file.
func TestV1265PatternsAreNotFiles(t *testing.T) {
	for cmd, ro := range map[string]bool{
		`grep -RhoE '^[[:space:]]*command[[:space:]]*=.*' /etc/supervisor`:     true,
		`grep -RIl --include='*.ini' 'storage_backend' /etc/supervisor /home`:  true,
		`find /home -maxdepth 5 -iname '*storage*backend*.ini' -printf '%p\n'`: true,
		`sed -n '1,20p' /etc/supervisor/supervisord.conf`:                      true,
		`grep -e 'x' ~/.ssh/id_rsa`:                                            false,
		`grep PRIVATE ~/.ssh/id_rsa`:                                           false,
		`cat /home/u/key.ppk`:                                                  false,
		`grep -f ~/.ssh/id_rsa /etc/hosts`:                                     false,
	} {
		if got := classifyReadOnly(cmd); got.ReadOnly != ro {
			t.Errorf("%s: read-only %v, want %v (%s)", cmd, got.ReadOnly, ro, got.Reason)
		}
	}
}
