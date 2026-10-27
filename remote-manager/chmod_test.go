// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestChmodRejectsInvalidPathAndMode(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "chmod-validation", false)
	connID := u.addConnection("chmod-target", "example.com")
	path := "/api/remote/chmod?id=" + strconv.Itoa(connID)

	cases := []map[string]interface{}{
		{"path": "", "mode": "755"},
		{"path": "/", "mode": "755"},
		{"path": "/tmp/file", "mode": ""},
		{"path": "/tmp/file", "mode": "77"},
		{"path": "/tmp/file", "mode": "75555"},
		{"path": "/tmp/file", "mode": "888"},
	}
	for _, body := range cases {
		if code := u.jsonDo("POST", path, body, nil); code != 400 {
			t.Fatalf("invalid chmod accepted (%+v): %d", body, code)
		}
	}

	if code := u.jsonDo("GET", path, nil, nil); code != 405 {
		t.Fatalf("GET should be rejected: %d", code)
	}
}
