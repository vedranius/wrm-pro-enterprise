// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"testing"
	"time"
)

// TestEnsureTableKeepsDeclaredColumns covers DATA-03: a NOT NULL column that the caller
// does not list but the CREATE statement declares must not make ensureTable set the table
// aside on every start (the v13.9-v13.14 addon_instances data loss).
func TestEnsureTableKeepsDeclaredColumns(t *testing.T) {
	name := fmt.Sprintf("schema_fix_%d", time.Now().UnixNano())
	mustExec(`CREATE TABLE ` + name + ` (id INTEGER PRIMARY KEY AUTOINCREMENT, payload TEXT NOT NULL)`)
	if _, err := db.Exec(`INSERT INTO ` + name + ` (payload) VALUES ('keep me')`); err != nil {
		t.Fatal(err)
	}
	ensureTable(name, `CREATE TABLE IF NOT EXISTS `+name+` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		payload TEXT NOT NULL
	)`)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ensureTable dropped the row (count=%d)", n)
	}
	var olds int
	db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE ?`, name+`_old_%`).Scan(&olds)
	if olds != 0 {
		t.Fatalf("ensureTable set the table aside %d time(s)", olds)
	}
	db.Exec(`DROP TABLE ` + name)
}
