// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPasswordConfirmationRateLimited covers SEC-02: confirming a password for a sensitive
// action must obey the sign-in limits and lock the account, not allow endless guessing.
func TestPasswordConfirmationRateLimited(t *testing.T) {
	old := settingInt("login_max_failures")
	if _, err := setSetting("login_max_failures", "3"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { setSetting("login_max_failures", fmt.Sprint(old)) })
	recordLoginResult("127.0.0.1", true)
	t.Cleanup(func() { recordLoginResult("127.0.0.1", true) })

	name := fmt.Sprintf("confirm-%d", time.Now().UnixNano())
	pw := "pw-" + name + "-123456"
	h, _ := hashPassword(pw)
	res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?,?,0,?)`,
		name, h, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id=?`, uid) })

	u, err := loadUser("id=?", uid)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/credentials/1/reveal", nil)
	req.RemoteAddr = "127.0.0.1:1234"

	for i := 0; i < 3; i++ {
		if got := checkPasswordLimited(req, u, "wrong"); got != pwWrong {
			t.Fatalf("attempt %d: got %v, want pwWrong", i+1, got)
		}
	}
	var locked string
	db.QueryRow(`SELECT COALESCE(locked_until,'') FROM users WHERE id=?`, uid).Scan(&locked)
	if locked == "" {
		t.Fatal("the account was not locked after repeated wrong confirmations")
	}
	if got := checkPasswordLimited(req, u, pw); got != pwLocked {
		t.Fatalf("locked account accepted a password: %v", got)
	}

	// After the lock expires, the right password works and resets the counters.
	db.Exec(`UPDATE users SET failed_logins=0, locked_until='' WHERE id=?`, uid)
	recordLoginResult("127.0.0.1", true)
	u, _ = loadUser("id=?", uid)
	if got := checkPasswordLimited(req, u, pw); got != pwOK {
		t.Fatalf("correct password after the lock: %v", got)
	}
}
