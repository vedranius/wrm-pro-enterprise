// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"
	"time"
)

// TestForeignKeysOnEveryPooledConnection covers DATA-01: foreign_keys is a per-connection
// PRAGMA, so it must come from the DSN and apply to every pooled connection, not only the
// one that happened to run a one-off statement.
func TestForeignKeysOnEveryPooledConnection(t *testing.T) {
	ctx := context.Background()
	var conns []interface{ Close() error }
	for i := 0; i < 4; i++ {
		c, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
		var on int
		if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&on); err != nil {
			t.Fatal(err)
		}
		if on != 1 {
			t.Fatalf("connection %d has foreign_keys=%d, want 1", i, on)
		}
	}
	for _, c := range conns {
		c.Close()
	}

	// A child row with a missing parent must be refused on the shared pool.
	_, err := db.Exec(`INSERT INTO auth_sessions (user_id, token, expires_at) VALUES (?,?,?)`,
		999999999, "fk-test-"+time.Now().Format("150405.000000000"), time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	if err == nil {
		db.Exec(`DELETE FROM auth_sessions WHERE token LIKE 'fk-test-%'`)
		t.Fatal("auth_sessions accepted a user_id that does not exist")
	}

	// ON DELETE CASCADE must actually remove the child rows.
	res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?,?,0,?)`,
		"fk-cascade-"+time.Now().Format("150405.000000000"), "x", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO auth_sessions (user_id, token, expires_at) VALUES (?,?,?)`,
		uid, "fk-cascade-"+time.Now().Format("150405.000000000"), time.Now().Add(time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	var left int
	db.QueryRow(`SELECT COUNT(*) FROM auth_sessions WHERE user_id=?`, uid).Scan(&left)
	if left != 0 {
		t.Fatalf("ON DELETE CASCADE left %d session(s) behind", left)
	}
}
