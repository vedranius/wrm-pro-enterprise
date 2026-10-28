// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"
)

func TestCheckoutFourEyes(t *testing.T) {
	migrateCheckouts()
	now := time.Now().UTC().Format(time.RFC3339)
	ru, _ := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES ('co-requester', 'x', 0, ?)`, now)
	au, _ := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES ('co-approver', 'x', 1, ?)`, now)
	rid, _ := ru.LastInsertId()
	aid, _ := au.LastInsertId()
	requesterID := int(rid)
	approverID := int(aid)
	defer db.Exec(`DELETE FROM users WHERE id IN (?,?)`, requesterID, approverID)

	cr, _ := db.Exec(`INSERT INTO credentials (owner_id, name, dual_control) VALUES (?, 'dc-cred', 1)`, requesterID)
	cid, _ := cr.LastInsertId()
	credID := int(cid)
	defer db.Exec(`DELETE FROM credentials WHERE id=?`, credID)

	ck, err := requestCheckout(credID, requesterID)
	if err != nil {
		t.Fatal(err)
	}
	// Four-eyes: the requester cannot approve their own checkout.
	if ok, _ := approveCheckout(ck, requesterID, true); ok {
		t.Fatal("self-approval was allowed")
	}
	// Another user who is an administrator can approve.
	if ok, _ := approveCheckout(ck, approverID, true); !ok {
		t.Fatal("administrator approval was refused")
	}
	if got := approvedCheckout(credID, requesterID); got != ck {
		t.Fatalf("approvedCheckout = %d, want %d", got, ck)
	}
	consumeCheckout(ck)
	if got := approvedCheckout(credID, requesterID); got != 0 {
		t.Fatalf("checkout not consumed: %d", got)
	}
}
