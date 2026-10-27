// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// fullAccessCall runs a command WRM refuses (blocked by ai_blocked_commands) on a fresh
// "ask" AI connection without the full_access scope and reports whether it was asked.
func fullAccessCall(t *testing.T, e *mcpEnv, file string) (asked []string, text string, isErr bool) {
	t.Helper()
	tok, _ := e.pat("ask", []string{"read_logs", "run_with_approval"}, nil)
	c := e.client(tok)
	c.initialize(true)
	c.onRequest = func(method string, params map[string]interface{}) map[string]interface{} {
		asked = append(asked, fmt.Sprint(params["message"]))
		return map[string]interface{}{"action": "accept", "content": map[string]interface{}{"decision": "approve"}}
	}
	sid := c.open("demo-api")
	text, isErr = c.call("run_command", map[string]interface{}{"session_id": sid, "command": "touch ~/" + file, "reason": "x"})
	return
}

// Mode "all" (the default): one administrator switch gives every AI session full access,
// no scope or user switch needed; it still asks.
func TestV1263FullAccessModeAll(t *testing.T) {
	e := newMCPEnv(t)
	t.Setenv("WRM_AI_BLOCKED_COMMANDS", "touch *blocked*")
	if getSetting("ai_full_access_mode") != "all" {
		t.Fatalf("default mode: %q", getSetting("ai_full_access_mode"))
	}
	if asked, text, isErr := fullAccessCall(t, e, "blocked-off"); !isErr || len(asked) != 0 || aiFileExists(filepath.Join(e.host.home, "blocked-off")) {
		t.Fatalf("switch off: %v %s %v", isErr, text, asked)
	}
	t.Setenv("WRM_AI_FULL_ACCESS", "1")
	asked, text, isErr := fullAccessCall(t, e, "blocked-on")
	if isErr || len(asked) != 1 || !strings.Contains(asked[0], "FULL ACCESS") || !aiFileExists(filepath.Join(e.host.home, "blocked-on")) {
		t.Fatalf("switch on: %v %s %v", isErr, text, asked)
	}
	// the scope is not offered in this mode
	if oneOf("full_access", mcpAllowedScopes()...) {
		t.Fatalf("full_access scope offered in mode all: %v", mcpAllowedScopes())
	}
}

// Mode "connection": only AI connections with the full_access scope.
func TestV1263FullAccessModeConnection(t *testing.T) {
	e := newMCPEnv(t)
	t.Setenv("WRM_AI_BLOCKED_COMMANDS", "touch *blocked*")
	t.Setenv("WRM_AI_FULL_ACCESS", "1")
	t.Setenv("WRM_AI_FULL_ACCESS_MODE", "connection")
	if !oneOf("full_access", mcpAllowedScopes()...) {
		t.Fatalf("full_access scope missing: %v", mcpAllowedScopes())
	}
	if asked, text, isErr := fullAccessCall(t, e, "blocked-noscope"); !isErr || len(asked) != 0 {
		t.Fatalf("without the scope: %v %s %v", isErr, text, asked)
	}
}

// Mode "user": each user turns on their own full access in WRM.
func TestV1263FullAccessModeUser(t *testing.T) {
	e := newMCPEnv(t)
	t.Setenv("WRM_AI_BLOCKED_COMMANDS", "touch *blocked*")
	// not allowed while the administrator has not chosen this mode
	if code, v := e.user.aiDo("POST", "/api/mcp/full-access", map[string]bool{"on": true}); code != 403 {
		t.Fatalf("switch without the policy: %d %v", code, v)
	}
	t.Setenv("WRM_AI_FULL_ACCESS", "1")
	t.Setenv("WRM_AI_FULL_ACCESS_MODE", "user")
	if asked, _, isErr := fullAccessCall(t, e, "blocked-user-off"); !isErr || len(asked) != 0 {
		t.Fatalf("user switch off: %v %v", isErr, asked)
	}
	if code, v := e.user.aiDo("POST", "/api/mcp/full-access", map[string]bool{"on": true}); code != 200 {
		t.Fatalf("turn on: %d %v", code, v)
	}
	_, v := e.user.aiDo("GET", "/api/mcp/config", nil)
	if fa := asMap(asMap(v)["full_access"]); fa["mine"] != true || fa["mode"] != "user" {
		t.Fatalf("config: %v", v)
	}
	asked, text, isErr := fullAccessCall(t, e, "blocked-user-on")
	if isErr || len(asked) != 1 || !strings.Contains(asked[0], "FULL ACCESS") {
		t.Fatalf("user switch on: %v %s %v", isErr, text, asked)
	}
	// another user's switch does not count
	if aiUserFullAccess(e.admin.userID) {
		t.Fatal("admin got the user's switch")
	}
	if mcpAuditCount("mcp.full_access", "true") < 1 {
		t.Fatal("not audited")
	}
	// turning it off is always allowed
	t.Setenv("WRM_AI_FULL_ACCESS", "0")
	if code, _ := e.user.aiDo("POST", "/api/mcp/full-access", map[string]bool{"on": false}); code != 200 || aiUserFullAccess(e.user.userID) {
		t.Fatalf("turn off: %d", code)
	}
}
