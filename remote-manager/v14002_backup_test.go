// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPruneBackups covers ENT-03: scheduled snapshots keep only the newest backup_keep files.
func TestPruneBackups(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"wrm-v14.0.2-20260101-030000.db",
		"wrm-v14.0.2-20260102-030000.db",
		"wrm-v14.0.2-20260103-030000.db",
		"wrm-v14.0.2-20260104-030000.db",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pruneBackups(dir, 2)
	left, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 {
		t.Fatalf("%d backups left, want 2", len(left))
	}
	for _, e := range left {
		if e.Name() != names[2] && e.Name() != names[3] {
			t.Errorf("kept %s, want only the two newest", e.Name())
		}
	}
}
