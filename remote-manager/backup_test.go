// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupDatabase(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "backup.db")
	if err := backupDatabase(dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	f, err := sql.Open("sqlite", dest)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var n int
	if err := f.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("backup is not a valid WRM database: %v", err)
	}
	if err := backupDatabase(dest); err == nil {
		t.Fatal("backupDatabase overwrote an existing file")
	}
}
