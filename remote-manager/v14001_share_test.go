// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
)

// TestLinkSharePolicyDoesNotWiden covers SEC-07: turning guest links off must deny the
// link, not silently turn it into "any signed-in account".
func TestLinkSharePolicyDoesNotWiden(t *testing.T) {
	old := getSetting("allow_link_shares")
	if _, err := setSetting("allow_link_shares", "0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { setSetting("allow_link_shares", old) })

	share := ShareLink{ID: 1, OwnerID: 999999, AccessMode: "link", Active: true, Name: "test"}
	_, d := resolveShareAccessCtx(share, accessCtx{})
	if d == nil {
		t.Fatal("a disabled guest link was accepted")
	}
	if d.Reason != "link_shares_disabled" {
		t.Fatalf("denied with %q, want link_shares_disabled", d.Reason)
	}

	if _, err := setSetting("allow_link_shares", "1"); err != nil {
		t.Fatal(err)
	}
	// With the policy on, the same link is not refused by the policy branch (it may still
	// ask for the guest cookie or a password).
	if _, d := resolveShareAccessCtx(share, accessCtx{}); d != nil && d.Reason == "link_shares_disabled" {
		t.Fatal("a link is refused while guest links are enabled")
	}
}
