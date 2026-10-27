// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// Databases used by earlier WRM builds can contain tables with the names v10 uses but a
// different layout. They must be kept aside and recreated, not break the upgrade.
func TestLegacyTablesUpgraded(t *testing.T) {
	legacy, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	for _, q := range []string{
		`CREATE TABLE collab_messages (id INTEGER PRIMARY KEY AUTOINCREMENT, share_token TEXT NOT NULL,
			user TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'chat', text TEXT NOT NULL DEFAULT '',
			file_name TEXT NOT NULL DEFAULT '', timestamp TEXT NOT NULL)`,
		`INSERT INTO collab_messages (share_token, user, text, timestamp) VALUES ('tok', 'ana', 'hello', '2025-01-01')`,
		`CREATE TABLE audit_log (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at TEXT NOT NULL, action TEXT NOT NULL)`,
		`CREATE INDEX ix_audit_ts ON audit_log(created_at)`,
		`CREATE TABLE app_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO app_settings (key, value) VALUES ('max_file_size_bytes', '100')`,
	} {
		if _, err := legacy.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}

	saved := db
	db = legacy
	defer func() { db = saved }()

	ensureTable("collab_messages", `CREATE TABLE IF NOT EXISTS collab_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT, share_id INTEGER NOT NULL, ts TEXT NOT NULL,
		pkey TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL DEFAULT 'chat',
		text TEXT NOT NULL DEFAULT '')`, "id", "share_id", "ts", "pkey", "name", "kind", "text")
	ensureTable("audit_log", `CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT NOT NULL, user_id INTEGER DEFAULT NULL,
		username TEXT NOT NULL DEFAULT '', ip TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
		target TEXT NOT NULL DEFAULT '', details TEXT NOT NULL DEFAULT '')`,
		"id", "ts", "user_id", "username", "ip", "action", "target", "details")
	ensureTable("app_settings", `CREATE TABLE IF NOT EXISTS app_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`, "key", "value")

	// New layout works.
	if _, err := db.Exec(`INSERT INTO collab_messages (share_id, ts, pkey, name, kind, text) VALUES (1, 'now', 'u:1', 'ana', 'chat', 'hi')`); err != nil {
		t.Fatalf("insert into upgraded collab_messages: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO audit_log (ts, action) VALUES ('now', 'test')`); err != nil {
		t.Fatalf("insert into upgraded audit_log: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS ix_audit_ts ON audit_log(ts)`); err != nil {
		t.Fatalf("index on upgraded audit_log: %v", err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='ix_audit_ts' AND tbl_name='audit_log'`).Scan(&n)
	if n != 1 {
		t.Fatalf("ix_audit_ts not on the new audit_log")
	}

	// Old data is kept under a new name.
	var old string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'collab_messages_old_%'`).Scan(&old); err != nil {
		t.Fatalf("legacy collab_messages not kept: %v", err)
	}
	var text string
	if err := db.QueryRow(`SELECT text FROM "` + old + `"`).Scan(&text); err != nil || text != "hello" {
		t.Fatalf("legacy data = %q, %v", text, err)
	}

	// A compatible table is left alone.
	var v string
	if err := db.QueryRow(`SELECT value FROM app_settings WHERE key='max_file_size_bytes'`).Scan(&v); err != nil || v != "100" {
		t.Fatalf("compatible app_settings was touched: %q, %v", v, err)
	}
	db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE 'app_settings_old_%'`).Scan(&n)
	if n != 0 {
		t.Fatalf("compatible app_settings was renamed")
	}
}

func TestStaticOldURLsRedirect(t *testing.T) {
	h := staticHandler()
	for path, want := range map[string]int{
		"/static/":                http.StatusFound,
		"/static/index.html":      http.StatusFound,
		"/static/vendor/":         http.StatusNotFound,
		"/static/nope.js":         http.StatusNotFound,
		"/static/vendor/xterm.js": http.StatusOK,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("%s: status %d, want %d", path, rec.Code, want)
		}
		if want == http.StatusFound && rec.Header().Get("Location") != "/" {
			t.Errorf("%s: redirects to %q, want /", path, rec.Header().Get("Location"))
		}
	}
}

func TestBrowserURL(t *testing.T) {
	for _, c := range []struct {
		addr string
		tls  bool
		want string
	}{
		{":8080", false, "http://localhost:8080/"},
		{"0.0.0.0:9000", true, "https://localhost:9000/"},
		{"127.0.0.1:8080", false, "http://127.0.0.1:8080/"},
		{"[::1]:8443", true, "https://[::1]:8443/"},
	} {
		if got := browserURL(c.addr, c.tls); got != c.want {
			t.Errorf("browserURL(%q, %v) = %q, want %q", c.addr, c.tls, got, c.want)
		}
	}
}
