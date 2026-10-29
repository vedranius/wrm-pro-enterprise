// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"
)

// TestHostKeyAutoTrustScoped covers SEC-05: a user may auto-trust a host only when they own
// a connection to it (or are an administrator), so nobody can seed trust for someone else's
// host before the real owner connects.
func TestHostKeyAutoTrustScoped(t *testing.T) {
	mk := func(name string, admin int) int {
		res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?,?,?,?)`,
			name, "x", admin, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	suffix := time.Now().Format("150405.000000000")
	owner := mk("hk-owner-"+suffix, 0)
	other := mk("hk-other-"+suffix, 0)
	admin := mk("hk-admin-"+suffix, 1)
	defer db.Exec(`DELETE FROM users WHERE id IN (?,?,?)`, owner, other, admin)
	if _, err := db.Exec(`INSERT INTO connections (name, protocol, host, username, auth_method, user_id) VALUES (?,?,?,?,?,?)`,
		"hk-host", "SSH", "10.9.9.9:22", "root", "PASSWORD", owner); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DELETE FROM connections WHERE user_id=?`, owner)

	host := "10.9.9.9:22"
	if !canAutoTrustHost(owner, host) {
		t.Error("the owner cannot auto-trust their host")
	}
	if canAutoTrustHost(other, host) {
		t.Error("another user can auto-trust the host")
	}
	if !canAutoTrustHost(admin, host) {
		t.Error("an administrator cannot auto-trust a host")
	}
	if !canAutoTrustHost(0, "192.0.2.10:22") {
		t.Error("internal callers lost auto-trust")
	}

	if _, err := db.Exec(`UPDATE connections SET bmc=? WHERE user_id=?`, `{"type":"redfish","host":"10.9.9.9","ssh_port":2222}`, owner); err != nil {
		t.Fatal(err)
	}
	if !canAutoTrustHost(owner, "10.9.9.9:2222") {
		t.Error("the BMC owner cannot auto-trust the BMC address")
	}
	if canAutoTrustHost(other, "10.9.9.9:2222") {
		t.Error("another user can auto-trust the BMC address")
	}
}
