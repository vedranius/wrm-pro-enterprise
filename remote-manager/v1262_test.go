// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Full access (v12.6.2): with the administrator's ai_full_access and the connection's
// full_access scope nothing is refused outright, but everything WRM would refuse asks first,
// every time, also in automatic mode; read-only stays read-only.
func TestV1262FullAccessAlwaysAsks(t *testing.T) {
	e := newMCPEnv(t)
	t.Setenv("WRM_AI_BLOCKED_COMMANDS", "touch *blocked*")
	scopes := []string{"read_logs", "run_with_approval", "edit_file_with_approval", "full_access"}
	// the administrator has not allowed it
	body := map[string]interface{}{"name": "x", "conn_ids": []int{e.connID}, "mode": "ask", "scopes": scopes}
	if code, v := e.user.aiDo("POST", "/api/mcp/grants", body); code == 200 {
		t.Fatalf("full access without the policy: %v", v)
	}
	t.Setenv("WRM_AI_FULL_ACCESS", "1")
	t.Setenv("WRM_AI_FULL_ACCESS_MODE", "connection")
	tok, _ := e.pat("auto", scopes, map[string]interface{}{"confirm_auto": true,
		"auto": map[string]interface{}{"allow": []string{"*"}, "path_allow": []string{"/**"}, "until_grant": true}})
	c := e.client(tok)
	c.initialize(true)
	var asked []string
	decision := "approve"
	c.onRequest = func(method string, params map[string]interface{}) map[string]interface{} {
		asked = append(asked, fmt.Sprint(params["message"]))
		if decision == "decline" {
			return map[string]interface{}{"action": "decline"}
		}
		return map[string]interface{}{"action": "accept", "content": map[string]interface{}{"decision": "approve"}}
	}
	sid := c.open("demo-api")
	// an ordinary command still runs automatically
	if text, isErr := c.call("run_command", map[string]interface{}{"session_id": sid, "command": "touch ~/plain", "reason": "x"}); isErr || len(asked) != 0 {
		t.Fatalf("plain command: %v %s %v", isErr, text, asked)
	}
	// a blocked command asks, even with "*" in automatic mode, and runs when approved
	text, isErr := c.call("run_command", map[string]interface{}{"session_id": sid, "command": "touch ~/blocked-ok", "reason": "x"})
	if isErr || !aiFileExists(filepath.Join(e.host.home, "blocked-ok")) || len(asked) != 1 || !strings.Contains(asked[0], "FULL ACCESS") {
		t.Fatalf("blocked command with full access: %v %s %v", isErr, text, asked)
	}
	// the built-in destructive list asks too (declined here, so nothing runs)
	decision = "decline"
	text, isErr = c.call("run_command", map[string]interface{}{"session_id": sid, "command": "rm -rf --no-preserve-root /", "reason": "x"})
	if !isErr || len(asked) != 2 || !strings.Contains(asked[1], "rm -rf") || !strings.Contains(text, "Not run yet") {
		t.Fatalf("destructive command with full access: %v %s", isErr, text)
	}
	// secret files even when the policy blocks them
	t.Setenv("WRM_AI_SENSITIVE_READS", "deny")
	decision = "approve"
	os.MkdirAll(filepath.Join(e.host.home, ".aws"), 0o700)
	writeHostFile(t, e.host, ".aws/credentials", "region = eu-west-1\naws_secret_access_key = zzSECRETzz123456\n")
	text, isErr = c.call("read_file", map[string]interface{}{"session_id": sid, "path": "~/.aws/credentials"})
	if isErr || !strings.Contains(text, "eu-west-1") || strings.Contains(text, "zzSECRETzz123456") || len(asked) != 3 {
		t.Fatalf("secret file with full access: %v %s", isErr, text)
	}
	// the administrator turns it off: refused again, without asking
	t.Setenv("WRM_AI_FULL_ACCESS", "0")
	text, isErr = c.call("run_command", map[string]interface{}{"session_id": sid, "command": "touch ~/blocked-2", "reason": "x"})
	if !isErr || aiFileExists(filepath.Join(e.host.home, "blocked-2")) || len(asked) != 3 || !strings.Contains(text, "full access") {
		t.Fatalf("after the policy is off: %v %s", isErr, text)
	}
}

// A read-only AI connection with full access stays read-only.
func TestV1262FullAccessReadOnly(t *testing.T) {
	e := newMCPEnv(t)
	t.Setenv("WRM_AI_FULL_ACCESS", "1")
	t.Setenv("WRM_AI_FULL_ACCESS_MODE", "connection")
	tok, _ := e.pat("read_only", []string{"read_logs", "run_readonly", "full_access"}, nil)
	c := e.client(tok)
	c.initialize(true)
	c.onRequest = func(method string, params map[string]interface{}) map[string]interface{} {
		t.Errorf("asked in a read-only session: %v", params["message"])
		return map[string]interface{}{"action": "decline"}
	}
	sid := c.open("demo-api")
	if text, isErr := c.call("run_command", map[string]interface{}{"session_id": sid, "command": "touch ~/ro", "reason": "x"}); !isErr || aiFileExists(filepath.Join(e.host.home, "ro")) {
		t.Fatalf("read-only with full access: %s", text)
	}
}
