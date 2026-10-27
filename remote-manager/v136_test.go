// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// v13.7: the screen lock is remembered per sign-in session and, in the strict mode, the
// server refuses that session's requests until the unlock.
func TestV136ScreenLockServer(t *testing.T) {
	srv := newTestServer(t)
	defer setSetting("screen_lock_mode", "server")
	setSetting("screen_lock_mode", "server")
	c := newTestUser(t, srv, "lock136-a", false)
	code := func(method, path, body string) int {
		resp := c.do(method, path, strings.NewReader(body), "application/json")
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := code("GET", "/api/connections", ""); got != 200 {
		t.Fatalf("unlocked session: %d", got)
	}
	if got := code("POST", "/api/auth/lock", `{"reason":"idle"}`); got != 200 {
		t.Fatalf("lock: %d", got)
	}
	if got := code("GET", "/api/connections", ""); got != 423 {
		t.Fatalf("a locked session reached the API: %d", got)
	}
	for _, p := range []string{"/api/auth/me", "/api/auth/keepalive", "/api/auth/lock", "/api/me/ui/appearance"} {
		if got := code("GET", p, ""); got != 200 {
			t.Errorf("%s while locked: %d", p, got)
		}
	}
	resp := c.do("GET", "/api/auth/me", nil, "")
	var me map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&me)
	resp.Body.Close()
	if me["screen_locked"] != true {
		t.Errorf("/api/auth/me does not report the lock: %v", me["screen_locked"])
	}
	if pol, _ := me["screen_lock"].(map[string]interface{}); pol == nil || pol["mode"] != "server" {
		t.Errorf("screen_lock policy missing: %v", me["screen_lock"])
	}
	if got := code("POST", "/api/auth/lock/verify", `{"password":"wrong"}`); got != 403 {
		t.Fatalf("wrong password: %d", got)
	}
	if got := code("GET", "/api/connections", ""); got != 423 {
		t.Fatalf("a wrong password unlocked: %d", got)
	}
	if got := code("POST", "/api/auth/lock/verify", `{"password":"pw-lock136-a-123456"}`); got != 200 {
		t.Fatalf("right password: %d", got)
	}
	if got := code("GET", "/api/connections", ""); got != 200 {
		t.Fatalf("still refused after the unlock: %d", got)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action IN ('auth.locked','auth.unlocked','auth.unlock_failed') AND user_id=?`, c.userID).Scan(&n)
	if settingBool("audit_enabled") && n < 3 {
		t.Errorf("lock events not audited: %d", n)
	}
}

func TestV136ScreenLockModes(t *testing.T) {
	srv := newTestServer(t)
	defer setSetting("screen_lock_mode", "server")
	c := newTestUser(t, srv, "lock136-b", false)
	code := func(method, path, body string) int {
		resp := c.do(method, path, strings.NewReader(body), "application/json")
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	setSetting("screen_lock_mode", "remember")
	code("POST", "/api/auth/lock", `{}`)
	if got := code("GET", "/api/connections", ""); got != 200 {
		t.Errorf("remember mode refused the API: %d", got)
	}
	resp := c.do("GET", "/api/auth/lock", nil, "")
	var st map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&st)
	resp.Body.Close()
	if st["locked"] != true {
		t.Errorf("remember mode forgot the lock: %v", st)
	}
	code("POST", "/api/auth/lock/verify", `{"password":"pw-lock136-b-123456"}`)
	setSetting("screen_lock_mode", "browser")
	code("POST", "/api/auth/lock", `{}`)
	var locked string
	db.QueryRow(`SELECT locked_at FROM auth_sessions WHERE user_id=?`, c.userID).Scan(&locked)
	if locked != "" {
		t.Errorf("browser mode stored the lock: %q", locked)
	}
}

func TestV136ScreenLockPIN(t *testing.T) {
	srv := newTestServer(t)
	defer setSetting("screen_lock_pin", "1")
	defer setSetting("screen_lock_max_failures", "0")
	setSetting("screen_lock_mode", "server")
	setSetting("screen_lock_pin", "1")
	c := newTestUser(t, srv, "lock136-c", false)
	code := func(method, path, body string) int {
		resp := c.do(method, path, strings.NewReader(body), "application/json")
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := code("PUT", "/api/auth/lock/pin", `{"pin":"12a4","password":"pw-lock136-c-123456"}`); got != 400 {
		t.Errorf("a PIN with a letter was accepted: %d", got)
	}
	if got := code("PUT", "/api/auth/lock/pin", `{"pin":"482913","password":"wrong"}`); got != 403 {
		t.Errorf("a PIN was set without the password: %d", got)
	}
	if got := code("PUT", "/api/auth/lock/pin", `{"pin":"482913","password":"pw-lock136-c-123456"}`); got != 200 {
		t.Fatalf("set PIN: %d", got)
	}
	code("POST", "/api/auth/lock", `{}`)
	if got := code("POST", "/api/auth/lock/verify", `{"pin":"482913"}`); got != 200 {
		t.Fatalf("the PIN did not unlock: %d", got)
	}
	// With the PIN turned off by the administrator it no longer unlocks.
	setSetting("screen_lock_pin", "0")
	code("POST", "/api/auth/lock", `{}`)
	if got := code("POST", "/api/auth/lock/verify", `{"pin":"482913"}`); got != 403 {
		t.Errorf("a PIN unlocked while PINs are off: %d", got)
	}
	setSetting("screen_lock_pin", "1")
	// Too many wrong attempts end the browser session.
	setSetting("screen_lock_max_failures", "2")
	code("POST", "/api/auth/lock/verify", `{"pin":"000000"}`)
	if got := code("POST", "/api/auth/lock/verify", `{"pin":"000001"}`); got != 401 {
		t.Fatalf("the second wrong PIN did not sign out: %d", got)
	}
	if got := code("GET", "/api/auth/me", ""); got != 401 {
		t.Errorf("the session survived the sign-out: %d", got)
	}
}

func TestV136AppearanceSettings(t *testing.T) {
	srv := newTestServer(t)
	defer setSetting("appearance_default_theme", "graphite")
	for _, ok := range []string{"daylight", "hc-dark", "addon:demo-themes:ocean", ""} {
		if _, err := setSetting("appearance_default_theme", ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"Daylight", "x;y", "addon:demo:", "../theme", strings.Repeat("a", 40)} {
		if _, err := setSetting("appearance_default_theme", bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	c := newTestUser(t, srv, "appearance136", false)
	resp := c.do("PUT", "/api/me/ui/appearance", strings.NewReader(`{"value":{"theme":"daylight","cvd":"deutan"}}`), "application/json")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("save appearance: %d", resp.StatusCode)
	}
	resp = c.do("GET", "/api/me/ui/appearance", nil, "")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"deutan"`) {
		t.Errorf("appearance not stored: %s", b)
	}
	js, _ := os.ReadFile("static/appearance.js")
	for _, want := range []string{"hc-light", "achroma", "minimumContrastRatio", "setAddonThemes", "/api/me/ui/appearance", "url\\("} {
		if want == "minimumContrastRatio" {
			continue
		}
		if !strings.Contains(string(js), want) {
			t.Errorf("appearance.js lacks %s", want)
		}
	}
	page, _ := os.ReadFile("static/index.html")
	for _, want := range []string{`src="/static/appearance.js"`, `id="st-appearance"`, "minimumContrastRatio", "--red-rgb", "--font-mono"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
}
