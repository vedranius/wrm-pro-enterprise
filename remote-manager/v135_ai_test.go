// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func hasEvent(s *aiSession, typ string) bool {
	for _, e := range eventTypes(s) {
		if e == typ {
			return true
		}
	}
	return false
}

// Thinking is requested from adaptive Claude models and shown as events; the
// first answer gets a short generated title; an ended session can be continued
// with its conversation.
func TestV135AIThinkingTitleAndResume(t *testing.T) {
	e := newAIEnv(t, func(n int, body map[string]interface{}) fakeTurn {
		if sys := fmt.Sprint(body["system"]); strings.Contains(sys, "short titles") {
			return fakeTurn{Text: "\"Disk space check.\""}
		}
		return fakeTurn{Text: "The disk has 40 GB free."}
	})
	t.Setenv("WRM_AI_AUTO_TITLES", "1")
	s := e.start("ask", nil)
	e.prompt(s, "How much disk space is left on the root file system? Please check.")
	e.waitIdle(s)
	if th, _ := e.llm.body(0)["thinking"].(map[string]interface{}); th["type"] != "adaptive" || th["display"] != "summarized" {
		t.Fatalf("thinking not requested: %v", e.llm.body(0)["thinking"])
	}
	if !hasEvent(s, "thinking") {
		t.Fatalf("no thinking event: %v", eventTypes(s))
	}
	aiWaitUntil(t, "the title", 10*time.Second, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.Title != "" })
	if s.Title != "Disk space check" {
		t.Fatalf("title %q", s.Title)
	}
	var stored string
	db.QueryRow(`SELECT title FROM ai_sessions WHERE id=?`, s.ID).Scan(&stored)
	if stored != "Disk space check" {
		t.Fatalf("stored title %q", stored)
	}
	// rename
	if code, v := e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/title", s.ID), map[string]string{"title": "Root disk"}); code != 200 {
		t.Fatalf("rename: %d %v", code, v)
	}
	old := s.ID
	s.Kill(0, "idle for more than 30m0s")
	// The event stream of an ended session says so (no endless reconnect).
	resp := e.user.do("GET", fmt.Sprintf("/api/ai/sessions/%d/events", old), nil, "")
	buf := make([]byte, 4096)
	nr, _ := resp.Body.Read(buf)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(buf[:nr]), "event: ended") || !strings.Contains(string(buf[:nr]), "resumable") {
		t.Fatalf("ended stream: %d %s", resp.StatusCode, buf[:nr])
	}
	before := e.llm.requests()
	code, v := e.user.aiDo("POST", "/api/ai/sessions", map[string]interface{}{"resume_from": old})
	if code != 200 {
		t.Fatalf("resume: %d %v", code, v)
	}
	s2 := aiGet(int64(asMap(v)["id"].(float64)))
	if s2 == nil || s2.ResumedFrom != old || s2.Title != "Root disk" || !hasEvent(s2, "history") {
		t.Fatalf("resumed session: %+v", s2)
	}
	e.prompt(s2, "And /var?")
	e.waitIdle(s2)
	body := e.llm.rawBody(before)
	if !strings.Contains(body, "How much disk space is left") || !strings.Contains(body, "40 GB free") || !strings.Contains(body, "resumed") {
		t.Fatalf("history not carried over: %s", body)
	}
	// A list per purpose; the terminal sessions are listed without the others.
	code, v = e.user.aiDo("GET", "/api/ai/sessions?purpose=terminal", nil)
	if code != 200 || len(v.([]interface{})) < 2 {
		t.Fatalf("list: %d %v", code, v)
	}
}

// The log assistant is read-only, has the log tools (no run_command) and
// search_logs greps the discovered log files.
func TestV135AILogAssistant(t *testing.T) {
	calls := 0
	e := newAIEnv(t, func(n int, body map[string]interface{}) fakeTurn {
		if strings.Contains(fmt.Sprint(body["system"]), "short titles") {
			return fakeTurn{Text: "Database timeouts"}
		}
		calls++
		switch calls {
		case 1:
			return fakeTurn{Text: "Searching.", Calls: []fakeCall{{Name: "search_logs", Input: `{"grep":"timeout"}`}}}
		case 2:
			return fakeTurn{Calls: []fakeCall{{Name: "run_command", Input: `{"command":"systemctl restart demo-api","reason":"x"}`}}}
		}
		return fakeTurn{Text: "Two timeouts."}
	})
	logs := filepath.Join(e.host.home, "ops-team", "demo-api", "logs")
	os.MkdirAll(logs, 0700)
	writeFile(t, filepath.Join(logs, "app.log"), "2026-10-21T10:00:00Z ERROR database timeout\n2026-10-21T10:01:00Z INFO ok\n")
	set := defaultLogSettings()
	set.ScanPaths = []string{e.host.home}
	if code := e.user.jsonDo("PUT", fmt.Sprintf("/api/monitoring/logs/settings?id=%d", e.connID), set, nil); code != 200 {
		t.Fatalf("settings: %d", code)
	}
	if code := e.user.jsonDo("POST", fmt.Sprintf("/api/monitoring/logs/discover?id=%d", e.connID), nil, nil); code != 200 {
		t.Fatalf("discover: %d", code)
	}
	code, v := e.user.aiDo("POST", "/api/ai/sessions", map[string]interface{}{"conn_id": e.connID, "provider_id": e.provID, "mode": "ask", "purpose": "logs"})
	if code != 200 {
		t.Fatalf("start: %d %v", code, v)
	}
	s := aiGet(int64(asMap(v)["id"].(float64)))
	if s.Mode != aiModeReadOnly || s.Purpose != "logs" {
		t.Fatalf("log assistant mode %s purpose %s", s.Mode, s.Purpose)
	}
	for _, d := range s.toolDefs() {
		if d.Name == "run_command" || d.Name == "edit_file" {
			t.Fatalf("log assistant offers %s", d.Name)
		}
	}
	if !strings.Contains(s.systemPrompt(), "log assistant") {
		t.Fatal("purpose prompt missing")
	}
	e.prompt(s, "Why does demo-api fail?")
	e.waitIdle(s)
	r := toolResultFor(s, "toolu_0_")
	if r == nil || !strings.Contains(fmt.Sprint(r["output"]), "database timeout") {
		t.Fatalf("search_logs result: %v", r)
	}
	if !strings.Contains(e.llm.rawBody(2), "not one of the tools") {
		t.Fatalf("run_command was not refused: %s", e.llm.rawBody(2))
	}
	code, v = e.user.aiDo("GET", fmt.Sprintf("/api/ai/sessions?purpose=logs&conn_id=%d", e.connID), nil)
	if code != 200 || len(v.([]interface{})) != 1 {
		t.Fatalf("log sessions: %d %v", code, v)
	}
	// A file assistant needs the open file's absolute path.
	if code, _ := e.user.aiDo("POST", "/api/ai/sessions", map[string]interface{}{"conn_id": e.connID, "provider_id": e.provID, "mode": "ask", "purpose": "file", "file_path": "relative.conf"}); code == 200 {
		t.Fatal("file assistant without an absolute path started")
	}
	code, v = e.user.aiDo("POST", "/api/ai/sessions", map[string]interface{}{"conn_id": e.connID, "provider_id": e.provID, "mode": "ask", "purpose": "file", "file_path": "/etc/demo/app.conf"})
	if code != 200 {
		t.Fatalf("file assistant: %d %v", code, v)
	}
	f := aiGet(int64(asMap(v)["id"].(float64)))
	if !strings.Contains(f.systemPrompt(), "/etc/demo/app.conf") {
		t.Fatal("file path not in the prompt")
	}
}

func TestV135AdaptiveThinkingModels(t *testing.T) {
	for m, want := range map[string]bool{"claude-opus-5-5": true, "claude-sonnet-4-6": true, "anthropic.claude-haiku-5-5": true, "claude-haiku-4-5": false, "claude-sonnet-4-5-20250929": false, "claude-opus-4-1": false, "gpt-5": false, "claude-fable-5-1": true} {
		if anthropicAdaptiveThinking(m) != want {
			t.Errorf("%s: want %v", m, want)
		}
	}
	if aiFallbackTitle("Why does nginx return 502 errors on the shop server since this morning? It worked yesterday.") != "Why does nginx return 502 errors on the …" {
		t.Errorf("fallback title: %q", aiFallbackTitle("Why does nginx return 502 errors on the shop server since this morning? It worked yesterday."))
	}
}
