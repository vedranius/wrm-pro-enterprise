// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

// v13.1: the administrator can choose a recording level when creating an
// account. The server setting stays the master switch.
func TestV130UserRecordingLevel(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "level-admin", true)
	create := func(body string) int {
		resp := admin.do("POST", "/api/admin/users", strings.NewReader(body), "application/json")
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := create(`{"username":"level-input","role":"user","password":"Strong!Pass123","recording_level":"input"}`); code != 200 {
		t.Fatalf("create with a level: %d", code)
	}
	if code := create(`{"username":"level-default","role":"user","password":"Strong!Pass123"}`); code != 200 {
		t.Fatalf("create without a level: %d", code)
	}
	if code := create(`{"username":"level-bad","role":"user","password":"Strong!Pass123","recording_level":"everything"}`); code != 400 {
		t.Fatalf("an unknown level was accepted: %d", code)
	}
	var uid, plain int
	if err := db.QueryRow(`SELECT id FROM users WHERE username='level-input'`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	var level string
	db.QueryRow(`SELECT recording_level FROM users WHERE username='level-input'`).Scan(&level)
	if level != "input" {
		t.Fatalf("the level was not stored: %q", level)
	}
	db.QueryRow(`SELECT id FROM users WHERE username='level-default'`).Scan(&plain)

	setSetting("session_recording", "1")
	defer setSetting("session_recording", "1")
	setSetting("session_recording_input", "0")
	if lv, on := userRecordingLevel(uid); !on || lv != "input" {
		t.Fatalf("a per-account input level was not honoured: %q %v", lv, on)
	}
	if lv, on := userRecordingLevel(plain); !on || lv != "metadata" {
		t.Fatalf("an account without a level did not follow the server settings: %q %v", lv, on)
	}
	setSetting("session_recording", "0")
	if lv, on := userRecordingLevel(uid); on || lv != "none" {
		t.Fatalf("the server master switch did not win: %q %v", lv, on)
	}
	// The level can be changed later through the user update endpoint.
	setSetting("session_recording", "1")
	resp := admin.do("PUT", "/api/admin/users/"+strconv.Itoa(uid), strings.NewReader(`{"recording_level":"none"}`), "application/json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: %d", resp.StatusCode)
	}
	resp.Body.Close()
	if lv, on := userRecordingLevel(uid); on || lv != "none" {
		t.Fatalf("the updated level was not applied: %q %v", lv, on)
	}
	page, _ := os.ReadFile("static/index.html")
	for _, want := range []string{`id="nu-record"`, "recording_level: document.getElementById('nu-record').value", `"rec_level_input"`} {
		if !strings.Contains(string(page), want) {
			t.Fatalf("the new-user dialog lacks %s", want)
		}
	}
}
