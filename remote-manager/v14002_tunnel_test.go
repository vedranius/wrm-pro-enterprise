// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// TestLoopbackTunnelOwnership covers SEC-04: a loopback port held by one user's tunnel must
// not be reachable through another user's connection.
func TestLoopbackTunnelOwnership(t *testing.T) {
	const port = 45678
	tr := &tunnelRun{key: "test-guard", def: tunnelDef{Kind: "local"}, ownerID: 1, boundAddr: "127.0.0.1:45678"}
	tunnelMgr.mu.Lock()
	tunnelMgr.runs[tr.key] = tr
	tunnelMgr.mu.Unlock()
	t.Cleanup(func() {
		tunnelMgr.mu.Lock()
		delete(tunnelMgr.runs, tr.key)
		tunnelMgr.mu.Unlock()
	})

	cases := []struct {
		host    string
		userID  int
		wantErr bool
	}{
		{"127.0.0.1:45678", 2, true},
		{"localhost:45678", 2, true},
		{"[::1]:45678", 2, true},
		{"127.0.0.1:45678", 1, false},
		{"10.0.0.1:45678", 2, false},
		{"127.0.0.1:45679", 2, false},
	}
	for _, c := range cases {
		err := guardLoopbackTunnel(Connection{Host: c.host, Protocol: "SSH", UserID: c.userID})
		if c.wantErr && (err == nil || !strings.Contains(err.Error(), "another user")) {
			t.Errorf("%s as user %d: err=%v, want refusal", c.host, c.userID, err)
		}
		if !c.wantErr && err != nil {
			t.Errorf("%s as user %d: unexpected refusal: %v", c.host, c.userID, err)
		}
	}
}
