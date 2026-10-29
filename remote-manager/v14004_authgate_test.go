// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAPIRoutesDenyByDefault covers ARCH-01: /api and /ws are protected unless the path is
// public, so a handler (or a route added later) that forgets requireAuth is refused.
func TestAPIRoutesDenyByDefault(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()

	for _, p := range []string{"/api/connections", "/api/admin/users", "/api/does-not-exist", "/ws/ssh", "/ws/events"} {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s without a session: %d, want 401", p, resp.StatusCode)
		}
	}

	// Sign-in and token-based routes must stay reachable without a session cookie.
	for _, p := range []string{"/api/version", "/api/auth/config", "/api/share/missing"} {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Errorf("%s is public but answered 401", p)
		}
	}

	u := newTestUser(t, srv, "deny-by-default", false)
	if code := u.jsonDo("GET", "/api/connections", nil, nil); code != http.StatusOK {
		t.Fatalf("signed-in /api/connections: %d, want 200", code)
	}
}
