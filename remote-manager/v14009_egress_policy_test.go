// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// TestEgressPrivatePolicy covers the rest of SEC-03: an administrator can require public
// targets only, with an explicit allowlist for the on-premise Git, NetBox, AI or OIDC host.
func TestEgressPrivatePolicy(t *testing.T) {
	restore := func(key, old string) { t.Cleanup(func() { setSetting(key, old) }) }
	oldPolicy, oldList := getSetting("egress_policy"), getSetting("egress_allow_hosts")
	restore("egress_policy", oldPolicy)
	restore("egress_allow_hosts", oldList)

	if _, err := setSetting("egress_policy", "allow_private"); err != nil {
		t.Fatal(err)
	}
	if _, err := setSetting("egress_allow_hosts", ""); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"http://10.0.0.1", "http://127.0.0.1:11434", "https://netbox.lan/", "http://192.168.1.5"} {
		if err := guardEgressURL(raw); err != nil {
			t.Fatalf("allow_private refused %s: %v", raw, err)
		}
	}

	if _, err := setSetting("egress_policy", "public_only"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"http://10.0.0.1", "https://192.168.1.5/", "http://127.0.0.1:11434", "http://[fd00::1]/", "http://localhost:8080/"} {
		if err := guardEgressURL(raw); err == nil || !strings.Contains(err.Error(), "private address") {
			t.Fatalf("public_only accepted %s (%v)", raw, err)
		}
	}
	if err := guardEgressURL("https://gitlab.example.com"); err != nil {
		t.Fatalf("public target refused: %v", err)
	}

	if _, err := setSetting("egress_allow_hosts", "10.0.0.0/8, 192.168.1.5"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"http://10.1.2.3/", "https://192.168.1.5/"} {
		if err := guardEgressURL(raw); err != nil {
			t.Fatalf("allowlisted %s refused: %v", raw, err)
		}
	}
	if err := guardEgressURL("http://127.0.0.1:11434"); err == nil {
		t.Fatal("a loopback target outside the allowlist was accepted")
	}
}
