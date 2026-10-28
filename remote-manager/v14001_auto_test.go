// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"testing"
	"time"
)

// TestAutoLimitsContinueExistingWindow covers AI-02: opening a new session with a saved
// grant must continue its Used/Until window instead of restarting the budget.
func TestAutoLimitsContinueExistingWindow(t *testing.T) {
	until := time.Now().Add(5 * time.Minute).Truncate(time.Second)
	a := &aiAutoLimits{Allow: []string{"*"}, Minutes: 10, MaxActions: 5, Used: 2, Until: until}
	if err := normalizeAutoLimits(a); err != nil {
		t.Fatal(err)
	}
	if a.Used != 2 {
		t.Fatalf("Used reset to %d", a.Used)
	}
	if !a.Until.Equal(until) {
		t.Fatalf("Until moved from %s to %s", until, a.Until)
	}

	fresh := &aiAutoLimits{Allow: []string{"*"}, Minutes: 10, MaxActions: 5}
	if err := normalizeAutoLimits(fresh); err != nil {
		t.Fatal(err)
	}
	if fresh.Used != 0 || !fresh.Until.After(time.Now()) {
		t.Fatalf("fresh opt-in: Used=%d Until=%s", fresh.Used, fresh.Until)
	}
}

// TestPersistAutoLimits covers the write-back that makes the counter survive a reopen.
func TestPersistAutoLimits(t *testing.T) {
	res, err := db.Exec(`INSERT INTO mcp_tokens (user_id, name, mode, auto_limits, expires_at) VALUES (0,?,?,?,?)`,
		"auto-limit-test-"+time.Now().Format("150405.000000000"), aiModeAuto, "", time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	defer db.Exec(`DELETE FROM mcp_tokens WHERE id=?`, id)

	s := &aiSession{Transport: "mcp", TokenID: id, Auto: &aiAutoLimits{Allow: []string{"*"}, MaxActions: 5, Used: 3, Until: time.Now().Add(10 * time.Minute)}}
	s.persistAutoLimits()

	var raw string
	if err := db.QueryRow(`SELECT auto_limits FROM mcp_tokens WHERE id=?`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var got aiAutoLimits
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("stored auto_limits: %v (%q)", err, raw)
	}
	if got.Used != 3 {
		t.Fatalf("persisted Used=%d, want 3", got.Used)
	}
}
