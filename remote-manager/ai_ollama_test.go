// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func fakeOllama(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	pulled := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			w.Write([]byte(`{"version":"0.9.9"}`))
		case "/api/tags":
			w.Write([]byte(`{"models":[{"name":"qwen2.5-coder:7b","size":4700000000},{"name":"demo-model:latest","size":1}]}`))
		case "/api/pull":
			var in struct{ Model string }
			json.NewDecoder(r.Body).Decode(&in)
			mu.Lock()
			pulled = append(pulled, in.Model)
			mu.Unlock()
			w.Write([]byte(`{"status":"success"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &pulled
}

func ollamaAuditCount(action string) int {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action=?`, action).Scan(&n)
	return n
}

func TestOllamaStatusDetection(t *testing.T) {
	t.Setenv("WRM_AI_ASSISTANT", "all")
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "oll-admin-1", true)
	user := newTestUser(t, srv, "oll-user-1", false)
	fake, _ := fakeOllama(t)

	code, v := admin.aiDo("GET", "/api/ai/ollama/status?url="+fake.URL, nil)
	m := asMap(v)
	if code != 200 || m["running"] != true || m["version"] != "0.9.9" || m["openai_base"] != fake.URL+"/v1" {
		t.Fatalf("admin status: %d %v", code, v)
	}
	if models, _ := m["models"].([]interface{}); len(models) != 2 || asMap(models[0])["name"] != "demo-model:latest" {
		t.Fatalf("models: %v", m["models"])
	}
	if steps, _ := m["steps"].([]interface{}); len(steps) != 3 {
		t.Fatalf("steps: %v", m["steps"])
	}

	// an ordinary user cannot make the server probe another URL
	_, v = user.aiDo("GET", "/api/ai/ollama/status?url="+fake.URL, nil)
	m = asMap(v)
	if m["url"] != ollamaDefaultURL || m["can_manage"] != false {
		t.Fatalf("user status must ignore the url: %v", v)
	}
	if code, _ := admin.aiDo("GET", "/api/ai/ollama/status?url=ftp://x", nil); code != 400 {
		t.Fatalf("bad url: %d", code)
	}
	if code, _ := admin.aiDo("GET", "/api/ai/ollama/status?url=http://user:pw@127.0.0.1:1", nil); code != 400 {
		t.Fatalf("credentials in url: %d", code)
	}
	// nothing listens here
	_, v = admin.aiDo("GET", "/api/ai/ollama/status?url=http://127.0.0.1:1", nil)
	if asMap(v)["running"] != false {
		t.Fatalf("not running: %v", v)
	}
}

func TestOllamaManagePermissionsAndAudit(t *testing.T) {
	t.Setenv("WRM_AI_ASSISTANT", "all")
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "oll-admin-2", true)
	user := newTestUser(t, srv, "oll-user-2", false)
	fake, _ := fakeOllama(t)
	body := map[string]interface{}{"action": "pull", "model": "demo-model:latest", "confirm": true, "url": fake.URL}

	before := ollamaAuditCount("admin.ai_ollama_denied")
	if code, _ := user.aiDo("POST", "/api/ai/ollama/action", body); code != 403 {
		t.Fatalf("a non-admin must be refused: %d", code)
	}
	if code, _ := admin.aiDo("POST", "/api/ai/ollama/action", body); code != 403 {
		t.Fatalf("the policy is off by default: %d", code)
	}
	if got := ollamaAuditCount("admin.ai_ollama_denied"); got != before+2 {
		t.Fatalf("denials must be audited: %d -> %d", before, got)
	}
	if code, _ := user.aiDo("GET", "/api/ai/ollama/runs", nil); code != 403 {
		t.Fatalf("runs are admin only: %d", code)
	}

	t.Setenv("WRM_AI_OLLAMA_MANAGE", "1")
	if code, _ := user.aiDo("POST", "/api/ai/ollama/action", body); code != 403 {
		t.Fatalf("policy on, still not admin: %d", code)
	}
	noConfirm := map[string]interface{}{"action": "pull", "model": "demo-model:latest", "url": fake.URL}
	if code, _ := admin.aiDo("POST", "/api/ai/ollama/action", noConfirm); code != 400 {
		t.Fatalf("confirmation is required: %d", code)
	}
	for _, bad := range []string{"", "x; rm -rf /", "a b", "$(id)", "../x", "-flag"} {
		if code, _ := admin.aiDo("POST", "/api/ai/ollama/action", map[string]interface{}{"action": "pull", "model": bad, "confirm": true, "url": fake.URL}); code != 400 {
			t.Fatalf("model %q must be refused: %d", bad, code)
		}
	}
	if code, _ := admin.aiDo("POST", "/api/ai/ollama/action", map[string]interface{}{"action": "rm -rf", "confirm": true}); code != 400 {
		t.Fatalf("unknown action: %d", code)
	}
	var runs int
	db.QueryRow(`SELECT COUNT(*) FROM ai_ollama_runs`).Scan(&runs)
	if runs != 0 {
		t.Fatalf("refused requests must not start runs: %d", runs)
	}
}

func TestOllamaActionsRecorded(t *testing.T) {
	t.Setenv("WRM_AI_ASSISTANT", "all")
	t.Setenv("WRM_AI_OLLAMA_MANAGE", "1")
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "oll-admin-3", true)
	fake, pulled := fakeOllama(t)

	oldOS, oldRun, oldLook, oldStat := ollamaOS, ollamaRun, ollamaLookPath, ollamaStat
	t.Cleanup(func() { ollamaOS, ollamaRun, ollamaLookPath, ollamaStat = oldOS, oldRun, oldLook, oldStat })
	ollamaStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	ollamaOS = "linux"
	var ran []string
	var rmu sync.Mutex
	ollamaRun = func(ctx context.Context, name string, args ...string) (string, error) {
		rmu.Lock()
		ran = append(ran, name+" "+strings.Join(args, " "))
		rmu.Unlock()
		return "fake output", nil
	}
	ollamaLookPath = func(file string) (string, error) {
		if file == "ollama" {
			return "", fmt.Errorf("not found")
		}
		return "/usr/bin/" + file, nil
	}

	runAction := func(body map[string]interface{}) map[string]interface{} {
		code, v := admin.aiDo("POST", "/api/ai/ollama/action", body)
		if code != 200 {
			t.Fatalf("action %v: %d %v", body["action"], code, v)
		}
		id := int(asMap(v)["run_id"].(float64))
		var last map[string]interface{}
		aiWaitUntil(t, "run to finish", 10*time.Second, func() bool {
			_, rv := admin.aiDo("GET", fmt.Sprintf("/api/ai/ollama/runs/%d", id), nil)
			last = asMap(rv)
			return last["status"] != "running"
		})
		if !ollamaBusy.TryLock() {
			t.Fatal("a completed run must release the action lock before publishing its status")
		}
		ollamaBusy.Unlock()
		return last
	}

	r := runAction(map[string]interface{}{"action": "install", "confirm": true, "url": fake.URL})
	if r["status"] != "done" || !strings.Contains(fmt.Sprint(r["output"]), "fake output") {
		t.Fatalf("install: %v", r)
	}
	if len(ran) != 1 || !strings.Contains(ran[0], "install.sh") {
		t.Fatalf("fixed install command expected: %v", ran)
	}

	// already running: start is a no-op that never calls a command
	r = runAction(map[string]interface{}{"action": "start", "confirm": true, "url": fake.URL})
	if r["status"] != "done" || !strings.Contains(fmt.Sprint(r["output"]), "already running") || len(ran) != 1 {
		t.Fatalf("start: %v %v", r, ran)
	}

	r = runAction(map[string]interface{}{"action": "pull", "model": "demo-model:latest", "confirm": true, "url": fake.URL})
	if r["status"] != "done" || len(*pulled) != 1 || (*pulled)[0] != "demo-model:latest" {
		t.Fatalf("pull: %v %v", r, *pulled)
	}

	// not running: pull fails and says why
	r = runAction(map[string]interface{}{"action": "pull", "model": "demo-model:latest", "confirm": true, "url": "http://127.0.0.1:1"})
	if r["status"] != "failed" || !strings.Contains(fmt.Sprint(r["output"]), "not running") {
		t.Fatalf("pull while down: %v", r)
	}

	for _, a := range []string{"admin.ai_ollama_install", "admin.ai_ollama_start", "admin.ai_ollama_pull", "admin.ai_ollama_finished"} {
		if ollamaAuditCount(a) == 0 {
			t.Fatalf("missing audit entry %s", a)
		}
	}
	var redacted int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action LIKE 'admin.ai_ollama_%' AND details LIKE '%[redacted]%'`).Scan(&redacted)
	if redacted != 0 {
		t.Fatalf("audit detail keys must be neutral, %d redacted", redacted)
	}
	_, v := admin.aiDo("GET", "/api/ai/ollama/runs", nil)
	if list, _ := v.([]interface{}); len(list) != 4 {
		t.Fatalf("runs: %v", v)
	}
}

func TestAIListModels(t *testing.T) {
	t.Setenv("WRM_AI_ASSISTANT", "all")
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "oll-admin-4", true)
	var gotAuth string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1beta/openai/models" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"data":[{"id":"models/gemini-demo-flash"},{"id":"gemini-demo-pro"},{"id":"bad name"},{"id":"gemini-demo-pro"}]}`))
	}))
	defer fake.Close()
	code, v := admin.aiDo("POST", "/api/admin/ai/providers", map[string]interface{}{"name": "Demo compat", "kind": "openai_compatible",
		"base_url": fake.URL + "/v1beta/openai", "api_key": "demo-key-0123456789", "models": "gemini-demo-flash"})
	if code != 200 {
		t.Fatalf("create: %d %v", code, v)
	}
	id := int(asMap(v)["id"].(float64))
	code, v = admin.aiDo("GET", fmt.Sprintf("/api/admin/ai/providers/%d/models", id), nil)
	m := asMap(v)
	list, _ := m["models"].([]interface{})
	if code != 200 || m["ok"] != true || len(list) != 2 || list[0] != "gemini-demo-flash" || list[1] != "gemini-demo-pro" {
		t.Fatalf("models: %d %v", code, v)
	}
	if gotAuth != "Bearer demo-key-0123456789" {
		t.Fatalf("auth header: %q", gotAuth)
	}
	if ollamaAuditCount("ai.provider_models_listed") == 0 {
		t.Fatal("model listing must be audited")
	}
	// a provider kind without a model list answers with a hint, not an error status
	_, v = admin.aiDo("POST", "/api/admin/ai/providers", map[string]interface{}{"name": "Demo Azure", "kind": "azure",
		"base_url": "https://demo.example.com", "api_key": "demo-key-0123456789", "models": "demo-deployment"})
	id = int(asMap(v)["id"].(float64))
	_, v = admin.aiDo("GET", fmt.Sprintf("/api/admin/ai/providers/%d/models", id), nil)
	if asMap(v)["ok"] != false {
		t.Fatalf("azure: %v", v)
	}
}
