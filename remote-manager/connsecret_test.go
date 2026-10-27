// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestConnectionSecretReveal(t *testing.T) {
	srv := newTestServer(t)
	owner := newTestUser(t, srv, "cs-owner", false)
	other := newTestUser(t, srv, "cs-other", false)
	const pw = "pw-cs-owner-123456"

	var v connView
	if code := owner.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "db1", "host": "10.1.2.3:22", "protocol": "SSH", "username": "deploy", "auth_method": "PASSWORD", "password": "S3cret-conn"}, &v); code != 201 {
		t.Fatalf("create: %d", code)
	}
	path := fmt.Sprintf("/api/connections/%d/secret", v.ID)
	var sec connSecret
	if code := owner.jsonDo("POST", path, map[string]string{"password": "wrong"}, &sec); code != 403 {
		t.Fatalf("wrong confirmation: %d", code)
	}
	if code := owner.jsonDo("POST", path, map[string]string{"password": pw, "purpose": "copy"}, &sec); code != 200 || sec.Password != "S3cret-conn" || sec.Username != "deploy" || sec.Source != "connection" {
		t.Fatalf("reveal: %d %+v", code, sec)
	}
	if code := other.jsonDo("POST", path, map[string]string{"password": "pw-cs-other-123456"}, &map[string]interface{}{}); code != 404 {
		t.Fatalf("another user: %d", code)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='connection.secret_revealed' AND user_id=?`, owner.userID).Scan(&n)
	var denied int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='connection.secret_denied' AND user_id=?`, owner.userID).Scan(&denied)
	if n != 1 || denied != 1 {
		t.Fatalf("audit: revealed %d, denied %d", n, denied)
	}

	// the administrator can turn it off for users
	saved := getSetting("allow_secret_export")
	setSetting("allow_secret_export", "0")
	code := owner.jsonDo("POST", path, map[string]string{"password": pw}, &map[string]interface{}{})
	setSetting("allow_secret_export", saved)
	if code != 403 {
		t.Fatalf("policy off: %d", code)
	}

	// a vault credential shared by somebody else is never shown to the grantee
	var cr credential
	if code := other.jsonDo("POST", "/api/credentials", map[string]interface{}{"name": "shared-root", "username": "root", "password": "Vault-pw-1", "grants": []int{owner.userID}}, &cr); code != 200 {
		t.Fatalf("credential: %d", code)
	}
	var vc connView
	if code := owner.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "via-vault", "host": "10.1.2.4:22", "protocol": "SSH", "auth_method": "CREDENTIAL", "credential_id": cr.ID}, &vc); code != 201 {
		t.Fatalf("vault connection: %d", code)
	}
	var e map[string]interface{}
	if code := owner.jsonDo("POST", fmt.Sprintf("/api/connections/%d/secret", vc.ID), map[string]string{"password": pw}, &e); code != 409 || !strings.Contains(fmt.Sprint(e["error"]), "only its owner") {
		t.Fatalf("shared credential: %d %v", code, e)
	}
	// an own credential is shown with its user name
	var mine credential
	owner.jsonDo("POST", "/api/credentials", map[string]interface{}{"name": "own-root", "username": "admin", "password": "Own-pw-2"}, &mine)
	var oc connView
	owner.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "via-own", "host": "10.1.2.5:22", "protocol": "SSH", "auth_method": "CREDENTIAL", "credential_id": mine.ID}, &oc)
	if code := owner.jsonDo("POST", fmt.Sprintf("/api/connections/%d/secret", oc.ID), map[string]string{"password": pw}, &sec); code != 200 || sec.Password != "Own-pw-2" || sec.Username != "admin" || sec.Source != "vault" || sec.SourceName != "own-root" {
		t.Fatalf("own credential: %d %+v", code, sec)
	}
}

func TestConnectionNameFromHostname(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "hn-user", false)
	h := startFakeHost(t, "root", "Fake-pw-1")
	var v connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": h.addr, "host": h.addr, "protocol": "SSH", "username": "root", "auth_method": "PASSWORD", "password": "Fake-pw-1"}, &v); code != 201 {
		t.Fatalf("create: %d", code)
	}
	want, _ := os.Hostname()
	var out map[string]string
	if code := u.jsonDo("POST", fmt.Sprintf("/api/connections/%d/name-from-hostname", v.ID), nil, &out); code != 200 || out["name"] == "" || (cleanHostname(want) != "" && out["name"] != cleanHostname(want)) {
		t.Fatalf("hostname: %d %v (want %q)", code, out, want)
	}
	var name string
	db.QueryRow(`SELECT name FROM connections WHERE id=?`, v.ID).Scan(&name)
	if name != out["name"] {
		t.Fatalf("stored name %q", name)
	}
	// other protocols: a typed host name gives its first label
	var rdp connView
	u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "x", "host": "win-app01.corp.example.com:3389", "protocol": "RDP", "auth_method": "PASSWORD"}, &rdp)
	if code := u.jsonDo("POST", fmt.Sprintf("/api/connections/%d/name-from-hostname", rdp.ID), nil, &out); code != 200 || out["name"] != "win-app01" {
		t.Fatalf("rdp: %d %v", code, out)
	}
	// someone else's connection
	o := newTestUser(t, srv, "hn-other", false)
	if code := o.jsonDo("POST", fmt.Sprintf("/api/connections/%d/name-from-hostname", v.ID), nil, &out); code != 404 {
		t.Fatalf("other user: %d", code)
	}
}

func TestGitInstallsOfDeletedConnection(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "gi-user", false)
	keep := u.addConnection("keep-1", "10.3.0.1:22")
	gone := u.addConnection("gone-1", "10.3.0.2:22")
	for _, cid := range []int{keep, gone} {
		db.Exec(`INSERT INTO git_installs (user_id, conn_id, app, path, env, state, data, checked_at, discovered_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			u.userID, cid, "demo-api", "/opt/demo-api", "prod", "ok", "{}", "", nowRFC())
	}
	// a leftover of an older binary: its connection was deleted directly
	orphan := u.addConnection("orphan-1", "10.3.0.3:22")
	db.Exec(`INSERT INTO git_installs (user_id, conn_id, app, path, env, state, data, checked_at, discovered_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		u.userID, orphan, "demo-api", "/opt/demo-api", "prod", "ok", "{}", "", nowRFC())
	db.Exec(`DELETE FROM connections WHERE id=?`, orphan)
	if code := u.jsonDo("DELETE", fmt.Sprintf("/api/connections/%d", gone), nil, &map[string]interface{}{}); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	list := loadGitInstalls(u.userID, false, "")
	if len(list) != 1 || list[0].ConnID != keep || list[0].ConnName != "keep-1" {
		t.Fatalf("installs: %+v", list)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM git_installs WHERE conn_id=?`, gone).Scan(&n)
	if n != 0 {
		t.Fatalf("installs of the deleted connection kept: %d", n)
	}
}
