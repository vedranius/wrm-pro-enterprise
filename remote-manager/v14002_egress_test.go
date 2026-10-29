// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// TestEgressGuardBlocksMetadata covers SEC-03: server-side fetches for Git, NetBox, AI
// providers and OIDC must never reach a cloud metadata service or a link-local address.
func TestEgressGuardBlocksMetadata(t *testing.T) {
	bad := []string{
		"http://169.254.169.254/latest/meta-data/",
		"https://169.254.169.254/",
		"http://[fd00:ec2::254]/",
		"http://100.100.100.200/",
		"http://metadata.google.internal/computeMetadata/v1/",
		"file:///etc/passwd",
		"http://user:pass@example.com/",
	}
	for _, raw := range bad {
		if err := guardEgressURL(raw); err == nil {
			t.Errorf("%s was accepted", raw)
		}
	}
	for _, raw := range []string{"https://example.com", "http://127.0.0.1:11434", "https://netbox.lan/api"} {
		if err := guardEgressURL(raw); err != nil {
			t.Errorf("%s refused: %v", raw, err)
		}
	}

	if _, err := gitAPIBase("gitlab", "http://169.254.169.254"); err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Errorf("git source accepted metadata: %v", err)
	}
	if _, err := netboxBase("http://169.254.169.254"); err == nil {
		t.Error("NetBox accepted metadata")
	}
	if err := oidcURLAllowed("https://169.254.169.254/.well-known/openid-configuration"); err == nil {
		t.Error("OIDC accepted metadata")
	}
}
