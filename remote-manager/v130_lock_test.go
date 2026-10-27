// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

// v13.2 (#115): the screen lock asks for the password of the signed-in account
// and audits the attempt; the browser session and the running sessions stay.
func TestV130LockVerify(t *testing.T) {
	srv := newTestServer(t)
	c := newTestUser(t, srv, "lock-verify-user", false)
	hash, err := hashPassword("Lock!Pass123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET password_hash=? WHERE username='lock-verify-user'`, hash); err != nil {
		t.Fatal(err)
	}
	post := func(body string) int {
		resp := c.do("POST", "/api/auth/lock/verify", strings.NewReader(body), "application/json")
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := post(`{"password":"wrong"}`); code != 403 {
		t.Fatalf("a wrong password unlocked the screen: %d", code)
	}
	if code := post(`{"password":"Lock!Pass123"}`); code != 200 {
		t.Fatalf("the right password did not unlock: %d", code)
	}
	if code := post(`{}`); code != 400 {
		t.Fatalf("an empty password was accepted: %d", code)
	}
	js, _ := os.ReadFile("static/lock.js")
	p := string(js)
	for _, want := range []string{"lock-screen", "/api/auth/lock/verify", "btn-lock", "wrm_lock_minutes", "lock_note"} {
		if !strings.Contains(p, want) {
			t.Errorf("lock.js lacks %s", want)
		}
	}
	page, _ := os.ReadFile("static/index.html")
	if !strings.Contains(string(page), `src="/static/lock.js"`) {
		t.Error("index.html does not load lock.js")
	}
}
