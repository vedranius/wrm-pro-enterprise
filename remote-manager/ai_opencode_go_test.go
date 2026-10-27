// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGoCatalog is a models.dev-style catalog with the Zen API classification;
// the Go override must win for Qwen and MiniMax M3.
func fakeGoCatalog(t *testing.T) {
	t.Helper()
	doc := `{"opencode":{"id":"opencode","npm":"@ai-sdk/openai-compatible","models":{
		"kimi-k3":{"id":"kimi-k3","name":"Kimi K3","tool_call":true,"cost":{"input":0.95,"output":4},"provider":{"npm":"@ai-sdk/openai-compatible"}},
		"claude-haiku-5-5":{"id":"claude-haiku-5-5","tool_call":true,"cost":{"input":0.1,"output":0.5},"provider":{"npm":"@ai-sdk/anthropic"}},
		"qwen3.8-max":{"id":"qwen3.8-max","tool_call":true,"cost":{"input":2,"output":6},"provider":{"npm":"@ai-sdk/openai-compatible"}},
		"mimo-v2.5":{"id":"mimo-v2.5","tool_call":true,"cost":{"input":0.14,"output":0.28}},
		"gpt-6-luna":{"id":"gpt-6-luna","tool_call":true,"cost":{"input":0.1,"output":0.5},"provider":{"npm":"@ai-sdk/openai"}},
		"gemini-4-argon-contributor":{"id":"gemini-4-argon-contributor","tool_call":true,"cost":{"input":4,"output":20},"provider":{"npm":"@ai-sdk/google"}}
	}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(doc))
	}))
	t.Cleanup(srv.Close)
	old := opencodeCatalogURL
	opencodeCatalogURL = srv.URL
	resetOpencodeCatalog()
	t.Cleanup(func() { opencodeCatalogURL = old; resetOpencodeCatalog() })
}

func TestOpencodeGoModelsAndRouting(t *testing.T) {
	fakeGoCatalog(t)
	ctx := context.Background()

	models := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("model list path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer go-key" {
			t.Errorf("model list auth: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"kimi-k3"},{"id":"claude-haiku-5-5"},{"id":"qwen3.8-max"},{"id":"gpt-6-luna"}]}`))
	}))
	defer models.Close()

	p := aiProvider{Kind: "opencode_go", BaseURL: models.URL}
	p.secret.APIKey = "go-key"
	list, err := opencodeGoListModels(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]opencodeModel{}
	for _, m := range list {
		byID[m.ID] = m
	}
	if byID["qwen3.8-max"].API != "messages" {
		t.Fatalf("Qwen must use Messages on Go: %+v", byID["qwen3.8-max"])
	}
	if byID["kimi-k3"].API != "chat" {
		t.Fatalf("Kimi must use Chat on Go: %+v", byID["kimi-k3"])
	}
	if _, ok := byID["gpt-6-luna"]; ok {
		t.Fatal("a Responses model was offered on Go")
	}
	if _, ok := byID["gemini-4-argon-contributor"]; ok {
		t.Fatal("a Google model was offered on Go")
	}

	f := newFakeLLM(t, nil)
	p.BaseURL = f.srv.URL
	req := aiLLMRequest{Model: "kimi-k3", MaxTokens: 50, SessionID: "conv-123", Messages: []aiMsg{{Role: "user", Text: "hi"}}}
	if _, err := aiComplete(ctx, p, req, nil); err != nil {
		t.Fatal(err)
	}
	if f.paths[0] != "/openai/v1/chat/completions" || f.headers[0].Get("Authorization") != "Bearer go-key" {
		t.Fatalf("chat request: %s %q", f.paths[0], f.headers[0].Get("Authorization"))
	}
	if got := f.headers[0].Get("x-opencode-session"); got != "wrm-conv-123" {
		t.Fatalf("session header: %q", got)
	}
	if ua := f.headers[0].Get("User-Agent"); !strings.Contains(ua, "WRM-PRO/") {
		t.Fatalf("user agent: %q", ua)
	}

	req.Model = "qwen3.8-max"
	if _, err := aiComplete(ctx, p, req, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(f.paths); n != 2 || f.paths[1] != "/anthropic/v1/messages" || f.headers[1].Get("x-api-key") != "go-key" {
		t.Fatalf("messages request: %v %q", f.paths, f.headers[len(f.headers)-1].Get("x-api-key"))
	}

	req.Model = "gpt-6-luna"
	if _, err := aiComplete(ctx, p, req, nil); err == nil || !strings.Contains(err.Error(), "Responses") {
		t.Fatalf("unsupported Go model: %v", err)
	}

	p.secret.APIKey = ""
	if _, err := aiComplete(ctx, p, req, nil); err == nil || !strings.Contains(err.Error(), "OpenCode Go API key") {
		t.Fatalf("missing Go key: %v", err)
	}
}

func TestOpencodeGoBaseAndProviderValidation(t *testing.T) {
	p := aiProvider{Kind: "opencode_go"}
	if got := opencodeGoAPIBase(p, "chat"); got != "https://opencode.ai/inference/go/openai/v1" {
		t.Fatalf("default chat base: %s", got)
	}
	if got := opencodeGoAPIBase(p, "messages"); got != "https://opencode.ai/inference/go/anthropic/v1" {
		t.Fatalf("default messages base: %s", got)
	}
	p.BaseURL = "https://proxy.example/go/openai/v1"
	if got := opencodeGoAPIBase(p, "chat"); got != "https://proxy.example/go/openai/v1" {
		t.Fatalf("custom base was doubled: %s", got)
	}

	var prov aiProvider
	in := aiProviderInput{Name: strPtr("Go"), Kind: strPtr("opencode_go"), Models: strPtr("kimi-k3")}
	if err := in.apply(&prov); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("Go without a key was accepted: %v", err)
	}
	in.APIKey = strPtr("go-key")
	if err := in.apply(&prov); err != nil {
		t.Fatalf("Go with a key: %v", err)
	}

	cfg, pid, model, env, err := opencodeConfig(prov, "kimi-k3", "http://127.0.0.1/mcp", "token", false)
	if err != nil {
		t.Fatal(err)
	}
	if pid != "opencode-go" || model != "kimi-k3" || !strings.Contains(string(cfg), `"opencode-go/kimi-k3"`) {
		t.Fatalf("OpenCode Go config: pid=%s model=%s %s", pid, model, cfg)
	}
	if !strings.Contains(strings.Join(env, "\n"), "WRM_OPENCODE_KEY=go-key") {
		t.Fatalf("the Go key is not passed to OpenCode: %v", env)
	}
	var parsed map[string]interface{}
	json.Unmarshal(cfg, &parsed)
	if parsed["model"] != "opencode-go/kimi-k3" {
		t.Fatalf("model id: %v", parsed["model"])
	}
}

func TestOpencodeGoPolicyMigration(t *testing.T) {
	old := getSetting("ai_provider_kinds")
	if _, err := db.Exec(`INSERT INTO app_settings(key,value) VALUES('ai_provider_kinds',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, aiProviderKindsOldDefault); err != nil {
		t.Fatal(err)
	}
	loadAppSettings()
	if got := getSetting("ai_provider_kinds"); !strings.Contains(got, "opencode_go") {
		t.Fatalf("the old all-kinds value did not gain OpenCode Go: %q", got)
	}
	// A deliberately narrowed list is not widened.
	if _, err := db.Exec(`INSERT INTO app_settings(key,value) VALUES('ai_provider_kinds','anthropic,openai') ON CONFLICT(key) DO UPDATE SET value=excluded.value`); err != nil {
		t.Fatal(err)
	}
	loadAppSettings()
	if got := getSetting("ai_provider_kinds"); got != "anthropic,openai" {
		t.Fatalf("a custom provider list was changed: %q", got)
	}
	if _, err := db.Exec(`INSERT INTO app_settings(key,value) VALUES('ai_provider_kinds',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, old); err != nil {
		t.Fatal(err)
	}
	loadAppSettings()
}

func TestOpencodeGoFallbackWithoutModelList(t *testing.T) {
	old := opencodeCatalogURL
	opencodeCatalogURL = "http://127.0.0.1:1/api.json"
	resetOpencodeCatalog()
	defer func() { opencodeCatalogURL = old; resetOpencodeCatalog() }()
	// The model list endpoint is unreachable; the documented list still appears.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) }))
	defer dead.Close()
	p := aiProvider{Kind: "opencode_go", BaseURL: dead.URL}
	p.secret.APIKey = "go-key"
	list, err := opencodeGoListModels(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range list {
		if m.ID == "kimi-k3" {
			found = true
		}
		if m.ID == "gpt-6-luna" || m.ID == "gemini-4-argon-contributor" {
			t.Fatalf("unsupported model in the fallback: %+v", m)
		}
	}
	if !found {
		t.Fatalf("fallback list: %+v", list)
	}
}
