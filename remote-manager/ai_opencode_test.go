// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeOpencodeCatalog serves a models.dev-style catalog whose OpenCode models point at base.
func fakeOpencodeCatalog(t *testing.T, base string) {
	doc := fmt.Sprintf(`{"other":{"id":"other","models":{"x":{"id":"x"}}},
	"opencode":{"id":"opencode","npm":"@ai-sdk/openai-compatible","api":"%[1]s/o","models":{
	  "free-chat":{"id":"free-chat","name":"Free Chat","tool_call":true,"cost":{"input":0,"output":0},"provider":{"api":"%[1]s/o"}},
	  "old-free":{"id":"old-free","tool_call":true,"status":"deprecated","cost":{"input":0,"output":0}},
	  "claude-x":{"id":"claude-x","tool_call":true,"cost":{"input":3,"output":15},"provider":{"npm":"@ai-sdk/anthropic","api":"%[1]s/a"}},
	  "gpt-x":{"id":"gpt-x","tool_call":true,"cost":{"input":2,"output":8},"provider":{"npm":"@ai-sdk/openai","api":"%[1]s/o"}},
	  "no-tools-free":{"id":"no-tools-free","tool_call":false,"cost":{"input":0,"output":0}}}}}`, base)
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

func resetOpencodeCatalog() {
	opencodeCatalog.Lock()
	opencodeCatalog.models, opencodeCatalog.err, opencodeCatalog.fetched = nil, nil, time.Time{}
	opencodeCatalog.Unlock()
}

func TestOpencodeCatalogAndModelList(t *testing.T) {
	fakeOpencodeCatalog(t, "http://127.0.0.1:1")
	ctx := context.Background()
	p := aiProvider{Kind: "opencode"}
	list, err := opencodeListModels(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "free-chat" || !list[0].Free {
		t.Fatalf("keyless list should hold only the current free model with tools: %+v", list)
	}
	p.secret.APIKey = "zen-key"
	list, _ = opencodeListModels(ctx, p)
	var ids []string
	for _, m := range list {
		ids = append(ids, m.ID)
	}
	// free first; Responses-API and tool-less models are left out
	if strings.Join(ids, ",") != "free-chat,claude-x" {
		t.Fatalf("keyed list: %v", ids)
	}
	if in, out, ok := opencodePrice("claude-x"); !ok || in != 3 || out != 15 {
		t.Fatalf("price: %v %v %v", in, out, ok)
	}
	if c := aiCost(aiProvider{Kind: "opencode"}, "claude-x", aiUsage{In: 1000000}); c != 3 {
		t.Fatalf("cost: %v", c)
	}
}

func TestOpencodeCompleteRouting(t *testing.T) {
	f := newFakeLLM(t, nil)
	fakeOpencodeCatalog(t, f.srv.URL)
	ctx := context.Background()
	req := aiLLMRequest{Model: "free-chat", MaxTokens: 50, Messages: []aiMsg{{Role: "user", Text: "hi"}}}

	// no key: a free model goes to chat completions with the public key
	if _, err := aiComplete(ctx, aiProvider{Kind: "opencode"}, req, nil); err != nil {
		t.Fatal(err)
	}
	if f.paths[0] != "/o/chat/completions" || f.headers[0].Get("Authorization") != "Bearer public" {
		t.Fatalf("free request: %s %q", f.paths[0], f.headers[0].Get("Authorization"))
	}
	// no key: a paid model is refused before any request
	req.Model = "claude-x"
	if _, err := aiComplete(ctx, aiProvider{Kind: "opencode"}, req, nil); err == nil || !strings.Contains(err.Error(), "not a free") {
		t.Fatalf("paid model without a key: %v", err)
	}
	// with a key: Claude goes to the Messages API with the key
	p := aiProvider{Kind: "opencode"}
	p.secret.APIKey = "zen-secret-key"
	if _, err := aiComplete(ctx, p, req, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(f.paths); n != 2 || f.paths[1] != "/a/messages" || f.headers[1].Get("x-api-key") != "zen-secret-key" {
		t.Fatalf("claude request: %v %q", f.paths, f.headers[len(f.headers)-1].Get("x-api-key"))
	}
	// Responses-API models are reported, not sent
	req.Model = "gpt-x"
	if _, err := aiComplete(ctx, p, req, nil); err == nil || !strings.Contains(err.Error(), "Responses") {
		t.Fatalf("responses model: %v", err)
	}
	// a base URL override wins over the catalog
	p.BaseURL = f.srv.URL + "/gw"
	req.Model = "free-chat"
	if _, err := aiComplete(ctx, p, req, nil); err != nil {
		t.Fatal(err)
	}
	if f.paths[len(f.paths)-1] != "/gw/chat/completions" {
		t.Fatalf("override: %v", f.paths)
	}
}

func TestOpencodeFallbackWithoutCatalog(t *testing.T) {
	zen := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"big-pickle"},{"id":"glm-9"},{"id":"claude-y"},{"id":"gpt-z"},{"id":"kimi-free"}]}`))
	}))
	defer zen.Close()
	old := opencodeCatalogURL
	opencodeCatalogURL = "http://127.0.0.1:1/api.json"
	resetOpencodeCatalog()
	defer func() { opencodeCatalogURL = old; resetOpencodeCatalog() }()
	p := aiProvider{Kind: "opencode", BaseURL: zen.URL}
	list, err := opencodeListModels(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range list {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "big-pickle,kimi-free" {
		t.Fatalf("fallback keyless list: %v", ids)
	}
	if opencodeGuessAPI("claude-y") != "messages" || opencodeGuessAPI("gpt-z") != "responses" || opencodeGuessAPI("glm-9") != "chat" {
		t.Fatal("name rule")
	}
}

func TestOpencodeProviderValidation(t *testing.T) {
	name, kind, models := "Zen", "opencode", "big-pickle"
	var p aiProvider
	if err := (aiProviderInput{Name: &name, Kind: &kind, Models: &models}).apply(&p); err != nil {
		t.Fatalf("a keyless OpenCode provider must be valid: %v", err)
	}
}

// The assistant hands a subtask to a free helper model: the tool is offered only when the
// administrator turned delegation on, the helper gets redacted text and no tools, and the
// call is shown, audited and counted.
func TestAIDelegateToHelperModel(t *testing.T) {
	helper := newFakeLLM(t, func(n int, body map[string]interface{}) fakeTurn {
		return fakeTurn{Text: "summary: nginx restarted twice"}
	})
	fakeOpencodeCatalog(t, helper.srv.URL)
	offered := map[int]bool{}
	delegated := false
	e := newAIEnv(t, nil)
	e.llm.script = func(n int, body map[string]interface{}) fakeTurn {
		for _, tl := range body["tools"].([]interface{}) {
			if asMap(tl)["name"] == aiDelegateTool {
				offered[n] = true
			}
		}
		if offered[n] && !delegated {
			delegated = true
			return fakeTurn{Text: "Delegating.", Calls: []fakeCall{{aiDelegateTool, `{"task":"Summarise this log","context":"password=Hunter2Secret99 nginx restarted\nnginx restarted","model":"free-chat","reason":"long log"}`}}}
		}
		return fakeTurn{Text: "Done."}
	}
	// off: not offered
	s := e.start("read_only", nil)
	e.prompt(s, "hello")
	e.waitIdle(s)
	if offered[0] {
		t.Fatal("delegate_task offered while delegation is off")
	}
	aiKillAll(0, "next")

	code, v := e.admin.aiDo("POST", "/api/admin/ai/providers", map[string]interface{}{"name": "Zen", "kind": "opencode", "base_url": helper.srv.URL, "models": "free-chat"})
	if code != 200 {
		t.Fatalf("helper provider: %d %v", code, v)
	}
	hid := int(asMap(v)["id"].(float64))
	if code, v := e.admin.aiDo("PUT", "/api/admin/settings", map[string]interface{}{"ai_delegate": "1", "ai_delegate_provider": fmt.Sprint(hid), "ai_delegate_models": "free-chat"}); code != 200 {
		t.Fatalf("settings: %d %v", code, v)
	}
	// free-chat's data use is unknown: treated like a model that may train, so it is left out
	// until the user opts in
	before := e.llm.requests()
	s = e.start("read_only", nil)
	e.prompt(s, "hello")
	e.waitIdle(s)
	if offered[before] {
		t.Fatal("a model that may train on prompts was offered without the user's opt-in")
	}
	aiKillAll(0, "next")
	if code, v := e.user.aiDo("PUT", "/api/ai/delegate", map[string]interface{}{"training": true}); code != 200 || asMap(v)["training"] != true {
		t.Fatalf("opt-in: %d %v", code, v)
	}
	before = e.llm.requests()
	s = e.start("read_only", nil)
	e.prompt(s, "summarise the log")
	e.waitIdle(s)
	if !offered[before] {
		t.Fatal("delegate_task not offered")
	}
	if helper.requests() != 1 {
		t.Fatalf("helper requests: %d", helper.requests())
	}
	hb := helper.rawBody(0)
	if strings.Contains(hb, "Hunter2Secret99") || !strings.Contains(hb, "Summarise this log") || strings.Contains(hb, `"tools"`) {
		t.Fatalf("helper body: %s", hb)
	}
	if helper.headers[0].Get("Authorization") != "Bearer public" {
		t.Fatalf("keyless helper auth: %q", helper.headers[0].Get("Authorization"))
	}
	r := lastToolResults(e.llm.body(e.llm.requests() - 1))
	if len(r) != 1 || !strings.Contains(r[0], "nginx restarted twice") || !strings.Contains(r[0], "helper_output") {
		t.Fatalf("result for the assistant: %v", r)
	}
	if aiAuditCount("ai.delegated", s.ID) != 1 || aiAuditCount("ai.delegate_result", s.ID) != 1 {
		t.Fatal("delegation not audited")
	}
	if tr := toolResultFor(s, ""); tr == nil || tr["tool"] != aiDelegateTool {
		t.Fatalf("live view event: %v", tr)
	}
}
