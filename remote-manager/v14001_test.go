// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"
)

// TestSecurity2FALockout covers SEC-01: a correct password followed by wrong second
// factors must count against the account (and lock it), not reset the IP counter first.
func TestSecurity2FALockout(t *testing.T) {
	srv := newTestServer(t)
	old := settingInt("login_max_failures")
	if _, err := setSetting("login_max_failures", "3"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { setSetting("login_max_failures", fmt.Sprint(old)) })
	recordLoginResult("127.0.0.1", true) // start from a clean IP window
	t.Cleanup(func() { recordLoginResult("127.0.0.1", true) })

	name := fmt.Sprintf("sec01-%d", time.Now().UnixNano())
	pw := "pw-" + name + "-123456"
	h, err := hashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("12345678901234567890")
	res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at, totp_enabled, totp_secret) VALUES (?,?,0,?,1,?)`,
		name, h, time.Now().UTC().Format(time.RFC3339), encryptValue(strings.TrimRight(base32.StdEncoding.EncodeToString(secret), "=")))
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	do := func(path string, body interface{}) (int, map[string]interface{}, string) {
		t.Helper()
		data, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", srv.URL+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", srv.URL)
		req.Header.Set("X-WRM-Request", "1")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var out map[string]interface{}
		json.Unmarshal(raw, &out)
		return resp.StatusCode, out, string(raw)
	}

	code, out, raw := do("/api/auth/login", map[string]string{"username": name, "password": pw})
	if code != 200 || out["mfa_required"] != true {
		t.Fatalf("login: %d %s", code, raw)
	}
	tok, _ := out["mfa_token"].(string)
	if tok == "" {
		t.Fatalf("no mfa token: %s", raw)
	}

	// A code that is not valid in the accepted window (Â±1 step), so the test does not
	// depend on the current TOTP value.
	now := time.Now().Unix() / 30
	bad := "000000"
	valid := map[string]bool{}
	for _, step := range []int64{now - 1, now, now + 1} {
		valid[totpCodeAt(secret, step)] = true
	}
	for i := 0; valid[bad]; i++ {
		bad = fmt.Sprintf("%06d", (i*7919+13)%1000000)
	}

	for i := 0; i < 3; i++ {
		if code, _, raw := do("/api/auth/mfa", map[string]string{"mfa_token": tok, "code": bad}); code != 401 {
			t.Fatalf("wrong code %d: %d %s", i+1, code, raw)
		}
	}
	var failed int
	var locked string
	db.QueryRow(`SELECT failed_logins, COALESCE(locked_until,'') FROM users WHERE id=?`, uid).Scan(&failed, &locked)
	if locked == "" {
		t.Fatalf("account was not locked after repeated wrong 2FA codes (failed_logins=%d)", failed)
	}

	// The password is still correct, but the account lock must stop the next attempt.
	recordLoginResult("127.0.0.1", true) // isolate the account lock from the IP limiter
	if code, _, raw := do("/api/auth/login", map[string]string{"username": name, "password": pw}); code != 429 {
		t.Fatalf("login while locked: %d %s", code, raw)
	}

	// After the lock is cleared, the right code signs in and resets the counters.
	db.Exec(`UPDATE users SET failed_logins=0, locked_until='' WHERE id=?`, uid)
	code, out, raw = do("/api/auth/login", map[string]string{"username": name, "password": pw})
	if code != 200 {
		t.Fatalf("second login: %d %s", code, raw)
	}
	tok, _ = out["mfa_token"].(string)
	good := totpCodeAt(secret, time.Now().Unix()/30)
	if code, _, raw := do("/api/auth/mfa", map[string]string{"mfa_token": tok, "code": good}); code != 200 {
		t.Fatalf("correct code: %d %s", code, raw)
	}
	db.QueryRow(`SELECT failed_logins, COALESCE(locked_until,'') FROM users WHERE id=?`, uid).Scan(&failed, &locked)
	if failed != 0 || locked != "" {
		t.Fatalf("successful sign-in did not reset counters: failed=%d locked=%q", failed, locked)
	}
}
