// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestHostSessionCookieOverHTTPS covers the cookie half of SEC-08: over HTTPS the session
// cookie carries the __Host- prefix (Secure, Path=/, no Domain), is read back, and both
// forms are cleared on sign-out. Plain HTTP keeps the old name so LAN installs still sign in.
func TestHostSessionCookieOverHTTPS(t *testing.T) {
	name := "cookie-prefix-" + time.Now().Format("150405.000000000")
	res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?,?,?,?)`,
		name, "x", 0, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()
	defer db.Exec(`DELETE FROM users WHERE id=?`, uid)

	req := httptest.NewRequest("GET", "https://wrm.example.com/", nil)
	rec := httptest.NewRecorder()
	if err := createAuthSession(rec, req, int(uid), ""); err != nil {
		t.Fatal(err)
	}
	var ck *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == hostSessionCookieName {
			ck = c
		}
	}
	if ck == nil {
		t.Fatalf("no %s cookie: %v", hostSessionCookieName, rec.Result().Cookies())
	}
	if !ck.Secure || ck.Path != "/" || ck.Domain != "" {
		t.Fatalf("cookie is not __Host- shaped: secure=%v path=%q domain=%q", ck.Secure, ck.Path, ck.Domain)
	}

	next := httptest.NewRequest("GET", "/api/auth/me", nil)
	next.AddCookie(&http.Cookie{Name: hostSessionCookieName, Value: ck.Value})
	s, err := lookupSession(next)
	if err != nil || s.UserID != int(uid) {
		t.Fatalf("prefixed cookie not accepted: %v", err)
	}

	// A plain HTTP request still gets the unprefixed cookie.
	plain := httptest.NewRecorder()
	if err := createAuthSession(plain, httptest.NewRequest("GET", "http://wrm.lan/", nil), int(uid), ""); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range plain.Result().Cookies() {
		if c.Name == sessionCookieName {
			found = true
		}
	}
	if !found {
		t.Fatalf("plain HTTP lost %s: %v", sessionCookieName, plain.Result().Cookies())
	}

	// Signing out expires both forms.
	out := httptest.NewRecorder()
	clearAuthCookie(out, req)
	cleared := map[string]bool{}
	for _, c := range out.Result().Cookies() {
		if c.MaxAge < 0 {
			cleared[c.Name] = true
		}
	}
	if !cleared[hostSessionCookieName] || !cleared[sessionCookieName] {
		t.Fatalf("sign-out left a cookie behind: %v", out.Result().Cookies())
	}
}
