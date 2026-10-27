// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"testing"
)

// The MCP server introduces itself, so a client that is asked about "wrm" knows what it is;
// open_session and the scope denial name the cause and the fix.
func TestMCPServerExplainsWrmAndScopeGaps(t *testing.T) {
	e := newMCPEnv(t)
	tok, _ := e.pat("read_only", []string{"run_readonly"}, nil)
	c := e.client(tok)
	res := c.initialize(false)
	info := asMap(res["serverInfo"])
	if !strings.Contains(fmt.Sprint(info["description"]), "Remote Manager") {
		t.Fatalf("serverInfo lacks a description: %v", info)
	}
	ins := fmt.Sprint(res["instructions"])
	if !strings.Contains(ins, "\"wrm\"") || !strings.Contains(ins, "Settings → AI connections") {
		t.Fatalf("instructions do not explain wrm: %s", ins)
	}
	text, isErr := c.call("open_session", map[string]interface{}{"connection": "demo-api"})
	if isErr || !strings.Contains(text, "not_allowed") || !strings.Contains(text, "scope read_logs missing") || !strings.Contains(text, "how_to_allow") {
		t.Fatalf("open_session does not explain the missing scopes: %v %s", isErr, text)
	}
	if gaps := aiScopeGaps([]string{"read_logs", "run_readonly", "edit_file_with_approval"}, aiModeAsk); len(gaps) != 0 {
		t.Fatalf("no gaps expected: %v", gaps)
	}
	if gaps := aiScopeGaps(nil, aiModeReadOnly); gaps != nil {
		t.Fatalf("the built-in panel has no scope gaps: %v", gaps)
	}
}

// The settings wizard gets the agent instructions from the user's config endpoint.
func TestMCPConfigOffersInstructions(t *testing.T) {
	e := newMCPEnv(t)
	code, v := e.user.aiDo("GET", "/api/mcp/config", nil)
	m := asMap(v)
	if code != 200 || m["instructions"] != mcpInstructions || m["version"] != AppVersion {
		t.Fatalf("config: %d %v", code, v)
	}
}
