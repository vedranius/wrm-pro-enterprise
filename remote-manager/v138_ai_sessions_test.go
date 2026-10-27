// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// v13.8.0: approvals kept for later run at once, an explicit Deny in the AI app is final,
// a decision on a request that no longer waits says what happened, and the AI sessions view.

// mcpDeferredCall starts a command over the engine, waits for its approval and lets the
// caller stop waiting, as an AI app does when its own tool timeout passes.
func mcpDeferredCall(t *testing.T, s *aiSession, command string) (*aiApproval, aiToolResult) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	in, _ := json.Marshal(map[string]string{"command": command, "reason": "test"})
	done := make(chan aiToolResult, 1)
	go func() {
		done <- s.CallTool(ctx, aiToolCall{ID: "mcp-" + randomToken(4), Name: "run_command", Input: in})
	}()
	a := (&aiEnv{t: t}).waitApproval(s)
	cancel()
	res := <-done
	return a, res
}

func hostLines(t *testing.T, h *fakeHost, name string) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(h.home, name))
	if err != nil {
		return 0
	}
	return strings.Count(string(b), "\n")
}

func TestMCPLaterApprovalRunsAtOnce(t *testing.T) {
	e := newMCPEnv(t)
	tok, _ := e.pat("ask", []string{"read_logs", "run_with_approval"}, nil)
	c := e.client(tok)
	c.initialize(false)
	sid := c.open("demo-api")
	s := mcpSessionOf(sid)
	cmd := "echo ran >> ~/later.txt"

	a, res := mcpDeferredCall(t, s, cmd)
	if !res.IsError || !strings.Contains(res.Content, "Not run yet") || !strings.Contains(res.Content, "WRM runs it as soon as they approve it") {
		t.Fatalf("deferred answer: %s", res.Content)
	}
	if p := pendingOf(s); p == nil || !p.Later || !p.RunNow || p.ID != a.ID {
		t.Fatalf("the request must stay open to run at once: %+v", p)
	}
	code, v := e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/approvals/%s", s.ID, a.ID), map[string]interface{}{"decision": "approve"})
	if code != 200 {
		t.Fatalf("approve later: %d %v", code, v)
	}
	aiWaitUntil(t, "the approved command to run", 20*time.Second, func() bool { return hostLines(t, e.host, "later.txt") == 1 })
	if mcpAuditCount("ai.later_run", a.ID) != 1 {
		t.Error("the run after a later approval is not audited")
	}

	// the AI repeats the call: it gets that output, nothing runs a second time
	text, isErr := c.call("run_command", map[string]interface{}{"session_id": sid, "command": cmd, "reason": "test"})
	if isErr || !strings.Contains(text, "it was not run again") || !strings.Contains(text, "<tool_output") {
		t.Fatalf("repeated call: %v %s", isErr, text)
	}
	if n := hostLines(t, e.host, "later.txt"); n != 1 {
		t.Fatalf("the command ran %d times", n)
	}

	// deciding again says what happened instead of a bare error
	code, v = e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/approvals/%s", s.ID, a.ID), map[string]interface{}{"decision": "deny"})
	dec := asMap(asMap(v)["decided"])
	if code != 409 || dec["outcome"] != "approved" || dec["by"] != usernameOf(e.user.userID) || !strings.Contains(fmt.Sprint(asMap(v)["error"]), "already approved by") {
		t.Fatalf("second decision: %d %v", code, v)
	}
	code, v = e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/approvals/nope", s.ID), map[string]interface{}{"decision": "approve"})
	if code != 409 || !strings.Contains(fmt.Sprint(asMap(v)["error"]), "no longer waiting") {
		t.Fatalf("unknown approval: %d %v", code, v)
	}
}

func TestMCPLaterApprovalRepeatPolicy(t *testing.T) {
	t.Setenv("WRM_AI_MCP_LATER_APPROVAL", "repeat")
	e := newMCPEnv(t)
	tok, _ := e.pat("ask", []string{"read_logs", "run_with_approval"}, nil)
	c := e.client(tok)
	c.initialize(false)
	sid := c.open("demo-api")
	s := mcpSessionOf(sid)
	cmd := "echo ran >> ~/repeat.txt"
	a, res := mcpDeferredCall(t, s, cmd)
	if !strings.Contains(res.Content, "it then runs once without asking again") {
		t.Fatalf("deferred answer: %s", res.Content)
	}
	e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/approvals/%s", s.ID, a.ID), map[string]interface{}{"decision": "approve"})
	time.Sleep(300 * time.Millisecond)
	if hostLines(t, e.host, "repeat.txt") != 0 {
		t.Fatal("with the repeat policy an approval must not run anything by itself")
	}
	if text, isErr := c.call("run_command", map[string]interface{}{"session_id": sid, "command": cmd, "reason": "test"}); isErr || hostLines(t, e.host, "repeat.txt") != 1 {
		t.Fatalf("repeated call: %v %s", isErr, text)
	}
}

// A later approval is checked again before it runs: here the policy made the session read-only.
func TestMCPLaterApprovalRecheckedBeforeRun(t *testing.T) {
	e := newMCPEnv(t)
	tok, _ := e.pat("ask", []string{"read_logs", "run_with_approval"}, nil)
	c := e.client(tok)
	c.initialize(false)
	sid := c.open("demo-api")
	s := mcpSessionOf(sid)
	cmd := "echo ran >> ~/recheck.txt"
	a, _ := mcpDeferredCall(t, s, cmd)
	t.Setenv("WRM_AI_MCP_MODES", "read_only") // the administrator narrows the policy meanwhile
	e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/approvals/%s", s.ID, a.ID), map[string]interface{}{"decision": "approve"})
	time.Sleep(300 * time.Millisecond)
	if hostLines(t, e.host, "recheck.txt") != 0 {
		t.Fatal("a later approval ran although the session is read-only now")
	}
	lr := s.takeLaterRun(a.key)
	if lr == nil {
		t.Fatal("no later run kept")
	}
	<-lr.done
	if !lr.res.IsError || !strings.Contains(lr.res.Content, "Denied by WRM when it was to run") {
		t.Fatalf("later run result: %+v", lr.res)
	}
}

// Deny chosen in the app's own question is final; closing the question keeps it for later.
func TestMCPElicitationDenyIsFinal(t *testing.T) {
	e := newMCPEnv(t)
	tok, _ := e.pat("ask", []string{"run_with_approval"}, nil)
	c := e.client(tok)
	c.initialize(true)
	sid := c.open("demo-api")
	s := mcpSessionOf(sid)
	c.onRequest = func(method string, params map[string]interface{}) map[string]interface{} {
		if method != "elicitation/create" {
			return nil
		}
		return map[string]interface{}{"action": "accept", "content": map[string]interface{}{"decision": "deny", "note": "not now"}}
	}
	text, isErr := c.call("run_command", map[string]interface{}{"session_id": sid, "command": "touch ~/denied-in-app", "reason": "test"})
	if !isErr || !strings.Contains(text, "denied this action in the AI app") || !strings.Contains(text, "not now") {
		t.Fatalf("deny in the app: %s", text)
	}
	if p := pendingOf(s); p != nil {
		t.Fatalf("an explicit Deny must not be kept for later: %+v", p)
	}
	if aiFileExists(filepath.Join(e.host.home, "denied-in-app")) {
		t.Fatal("a denied command ran")
	}
}

// The AI sessions view: every own session (here an AI app's), search, save, export and
// continuing an AI app's session in the WRM assistant with the personal default provider.
func TestAISessionsView(t *testing.T) {
	t.Setenv("WRM_AI_ASSISTANT", "all")
	t.Setenv("WRM_AI_AUTO_TITLES", "0")
	e := newMCPEnv(t)
	llm := newFakeLLM(t, func(n int, body map[string]interface{}) fakeTurn { return fakeTurn{Text: "continued"} })
	code, v := e.admin.aiDo("POST", "/api/admin/ai/providers", map[string]interface{}{
		"name": "Org Claude", "kind": "anthropic", "base_url": llm.srv.URL, "api_key": fakeAPIKey, "models": "claude-opus-5-5", "default_model": "claude-opus-5-5"})
	if code != 200 {
		t.Fatalf("provider: %d %v", code, v)
	}
	provID := int(asMap(v)["id"].(float64))
	tok, _ := e.pat("read_only", []string{"read_logs"}, nil)
	c := e.client(tok)
	c.initialize(false)
	sid := c.open("demo-api")
	s := mcpSessionOf(sid)
	writeHostFile(t, e.host, "inventory-notes.txt", "demo\n")
	if text, isErr := c.call("read_file", map[string]interface{}{"session_id": sid, "path": "~/inventory-notes.txt"}); isErr {
		t.Fatalf("read_file: %s", text)
	}

	list := func(q string) []interface{} {
		t.Helper()
		code, v := e.user.aiDo("GET", "/api/ai/sessions?view=all&"+q, nil)
		if code != 200 {
			t.Fatalf("list %s: %d %v", q, code, v)
		}
		return asMap(v)["sessions"].([]interface{})
	}
	found := func(l []interface{}) map[string]interface{} {
		for _, x := range l {
			if int64(asMap(x)["id"].(float64)) == s.ID {
				return asMap(x)
			}
		}
		return nil
	}
	m := found(list("kind=mcp"))
	if m == nil || m["kind"] != "mcp" || m["live"] != true || !strings.Contains(fmt.Sprint(m["preview"]), "inventory-notes.txt") {
		t.Fatalf("MCP session in the view: %v", m)
	}
	if found(list("kind=assistant")) != nil || found(list("q=inventory-notes")) == nil || found(list("q=no-such-text")) != nil {
		t.Fatal("kind filter or search")
	}
	if code, _ := e.user.aiDo("GET", "/api/ai/sessions?view=all&kind=bogus", nil); code != 400 {
		t.Fatalf("unknown kind: %d", code)
	}
	// another user sees nothing of it
	other := newTestUser(t, e.user.srv, fmt.Sprintf("aiss-other-%d", mcpUserSeq), false)
	if code, v := other.aiDo("GET", "/api/ai/sessions?view=all", nil); code != 200 || len(asMap(v)["sessions"].([]interface{})) != 0 {
		t.Fatalf("another user's view: %d %v", code, v)
	}
	if code, _ := other.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/save", s.ID), map[string]bool{"saved": true}); code != 404 {
		t.Fatalf("another user saved it: %d", code)
	}
	resp := other.do("GET", fmt.Sprintf("/api/ai/sessions/%d/export", s.ID), nil, "")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("another user exported it: %d", resp.StatusCode)
	}

	// save and export
	if code, v := e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/save", s.ID), map[string]bool{"saved": true}); code != 200 {
		t.Fatalf("save: %d %v", code, v)
	}
	if m := found(list("kind=saved")); m == nil || m["saved"] != true {
		t.Fatal("saved filter")
	}
	resp = e.user.do("GET", fmt.Sprintf("/api/ai/sessions/%d/export?format=md", s.ID), nil, "")
	md, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Disposition"), "wrm-ai-session-") || !strings.Contains(string(md), "read_file") {
		t.Fatalf("markdown export: %d %s", resp.StatusCode, md)
	}
	resp = e.user.do("GET", fmt.Sprintf("/api/ai/sessions/%d/export?format=json", s.ID), nil, "")
	var exp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&exp)
	resp.Body.Close()
	if len(exp["messages"].([]interface{})) == 0 || asMap(exp["session"])["transport"] != "mcp" {
		t.Fatalf("json export: %v", exp)
	}

	// personal default provider, then continue the AI app's work in the WRM assistant
	if code, v := e.user.aiDo("PUT", "/api/me/ui/ai", map[string]interface{}{"value": map[string]interface{}{"provider_id": provID, "thinking": false}}); code != 200 {
		t.Fatalf("prefs: %d %v", code, v)
	}
	if p := aiPrefsOf(e.user.userID); p.ProviderID != provID || aiThinkingWanted(e.user.userID) {
		t.Fatalf("prefs read back: %+v", p)
	}
	s.Kill(0, "test: the app closed the session")
	code, v = e.user.aiDo("POST", "/api/ai/sessions", map[string]interface{}{"resume_from": s.ID, "conn_id": e.connID})
	if code != 200 || asMap(v)["transport"] != "panel" {
		t.Fatalf("continue an MCP session: %d %v", code, v)
	}
	ns := aiGet(int64(asMap(v)["id"].(float64)))
	ns.mu.Lock()
	hist := ns.history
	prov := ns.provider.ID
	ns.mu.Unlock()
	all := ""
	for _, h := range hist {
		all += h.Text + "\n"
	}
	if prov != provID || len(hist) == 0 || hist[0].Role != "user" || !strings.Contains(hist[0].Text, "an AI app (test-client)") || !strings.Contains(all, "earlier tool call: read_file") {
		t.Fatalf("continued history (provider %d): %+v", prov, hist)
	}

	// saved sessions outlive the transcript retention
	var plain int64
	res, _ := db.Exec(`INSERT INTO ai_sessions (uid, user_id, username, conn_id, conn_name, host, transport, provider_name, provider_kind, model, mode, status, started_at, last_activity)
		VALUES (?,?,?,?,?,?,'panel','x','x','x','ask','ended','2001-01-01T00:00:00Z','2001-01-01T00:00:00Z')`, randomToken(6), e.user.userID, usernameOf(e.user.userID), e.connID, "demo-api", "h")
	plain, _ = res.LastInsertId()
	db.Exec(`UPDATE ai_sessions SET started_at='2001-01-01T00:00:00Z' WHERE id=?`, s.ID)
	cleanupAITranscripts()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM ai_sessions WHERE id=?`, s.ID).Scan(&n)
	if n != 1 {
		t.Fatal("a saved session was removed by the retention")
	}
	db.QueryRow(`SELECT COUNT(*) FROM ai_sessions WHERE id=?`, plain).Scan(&n)
	if n != 0 {
		t.Fatal("an old unsaved session was kept")
	}
}
