// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"testing"
	"time"
)

// TestConnectionDeleteRemovesDependentRows covers DATA-04: deleting a connection must
// also remove its monitoring samples, monitor profile, log settings and server health,
// not only the connection row.
func TestConnectionDeleteRemovesDependentRows(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, fmt.Sprintf("conn-clean-%d", time.Now().UnixNano()), false)
	cid := u.addConnection("cleanup-target", "10.9.0.1:22")

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO monitor_samples (conn_id, user_id, at, data) VALUES (?,?,?,?)`, cid, u.userID, now, "{}"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO monitor_profiles (conn_id, user_id, config, updated_at) VALUES (?,?,?,?)`, cid, u.userID, "{}", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO log_settings (conn_id, user_id, config, updated_at) VALUES (?,?,?,?)`, cid, u.userID, "{}", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO git_server_health (user_id, conn_id, data) VALUES (?,?,?)`, u.userID, cid, "{}"); err != nil {
		t.Fatal(err)
	}

	if code := u.jsonDo("DELETE", fmt.Sprintf("/api/connections/%d", cid), nil, &map[string]interface{}{}); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	for _, table := range []string{"monitor_samples", "monitor_profiles", "log_settings", "git_server_health"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE conn_id=?`, cid).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s still has %d row(s) for the deleted connection", table, n)
		}
	}
}
