// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestMediumHostBuildAdminOnly covers GIT-01: a host build runs a shell as the WRM
// account, so only administrators may select it.
func TestMediumHostBuildAdminOnly(t *testing.T) {
	mk := func(name string, admin int) int {
		res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?,?,?,?)`,
			name, "x", admin, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	user := mk("medium-user-"+time.Now().Format("150405.000000000"), 0)
	admin := mk("medium-admin-"+time.Now().Format("150405.000000000"), 1)
	defer db.Exec(`DELETE FROM users WHERE id IN (?,?)`, user, admin)

	if _, err := mdSetBuild(user, "demo", "host", 0); err == nil || !strings.Contains(err.Error(), "administrators") {
		t.Fatalf("non-admin host build: err=%v", err)
	}
	if runtime.GOOS != "windows" {
		if _, err := mdSetBuild(admin, "demo", "host", 0); err != nil {
			t.Fatalf("admin host build refused: %v", err)
		}
		db.Exec(`DELETE FROM medium_build_settings WHERE user_id=?`, admin)
	}
	if _, err := mdSetBuild(user, "demo", "none", 0); err != nil {
		t.Fatalf("non-admin no-build refused: %v", err)
	}
}

// TestMediumBuildEnvScrubsWRMSecrets covers the environment half of GIT-01.
func TestMediumBuildEnvScrubsWRMSecrets(t *testing.T) {
	t.Setenv("WRM_TEST_SECRET", "do-not-leak")
	t.Setenv("ENCRYPTION_KEY", "do-not-leak")
	t.Setenv("ENCRYPTION_KEY_FILE", "/tmp/do-not-leak")
	t.Setenv("DB_PATH", "/tmp/do-not-leak.db")
	t.Setenv("PATH", "/usr/bin:/bin")
	env := mdBuildEnv()
	joined := strings.Join(env, "\n")
	for _, bad := range []string{"WRM_TEST_SECRET", "ENCRYPTION_KEY=", "ENCRYPTION_KEY_FILE=", "DB_PATH="} {
		if strings.Contains(joined, bad) {
			t.Errorf("build environment still contains %s", bad)
		}
	}
	if !strings.Contains(joined, "PATH=/usr/bin:/bin") {
		t.Error("build environment lost PATH")
	}
}
