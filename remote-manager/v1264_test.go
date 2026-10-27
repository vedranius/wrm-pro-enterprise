// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ai_full_access_blocked = deny keeps the administrator's blocked commands refused under full
// access, while the built-in destructive list still asks.
func TestV1264BlockedStaysDenied(t *testing.T) {
	e := newMCPEnv(t)
	t.Setenv("WRM_AI_BLOCKED_COMMANDS", "touch *blocked*")
	t.Setenv("WRM_AI_FULL_ACCESS", "1")
	if getSetting("ai_full_access_blocked") != "ask" {
		t.Fatalf("default: %q", getSetting("ai_full_access_blocked"))
	}
	t.Setenv("WRM_AI_FULL_ACCESS_BLOCKED", "deny")
	asked, text, isErr := fullAccessCall(t, e, "blocked-hard")
	if !isErr || len(asked) != 0 || aiFileExists(filepath.Join(e.host.home, "blocked-hard")) || !strings.Contains(text, "ai_full_access_blocked") {
		t.Fatalf("blocked list with deny: %v %s %v", isErr, text, asked)
	}
	// the built-in destructive list still becomes an approval
	dec := aiFullAccessDecision(aiEvaluate(aiModeAsk, nil, aiAction{Tool: "run_command", Kind: "command", Command: "rm -rf --no-preserve-root /"}, "/root", time.Now()), aiModeAsk)
	if dec.Action != aiApprove || dec.Rule != "full_access" {
		t.Fatalf("destructive with deny: %+v", dec)
	}
	t.Setenv("WRM_AI_FULL_ACCESS_BLOCKED", "ask")
	if asked, text, isErr := fullAccessCall(t, e, "blocked-ask"); isErr || len(asked) != 1 {
		t.Fatalf("blocked list with ask: %v %s %v", isErr, text, asked)
	}
}
