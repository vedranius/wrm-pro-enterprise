// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// TestAuditChainIsKeyed covers SEC-06: new entries use an HMAC keyed from the installation
// key, while the legacy SHA-256 form stays readable.
func TestAuditChainIsKeyed(t *testing.T) {
	h := auditHashKeyed("prev", "2026-01-01T00:00:00Z", 1, "admin", "127.0.0.1", "test", "target", "{}", 0, 0)
	if !strings.HasPrefix(h, "h2:") {
		t.Fatalf("new audit hash is not keyed: %s", h)
	}
	legacy := auditHash("prev", "2026-01-01T00:00:00Z", 1, "admin", "127.0.0.1", "test", "target", "{}", 0, 0)
	if strings.HasPrefix(legacy, "h2:") {
		t.Fatal("legacy hash changed shape")
	}
	if h == legacy {
		t.Fatal("keyed and legacy hashes are identical")
	}
}
