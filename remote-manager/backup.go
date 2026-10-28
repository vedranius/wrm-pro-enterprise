// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ─── BACKUP / RESTORE (RFC 0006) ─────────────────────────────────────
// A consistent database snapshot (SQLite VACUUM INTO) plus a copy of the encryption key,
// kept as a separate file. See docs/rfcs/0006-backup-restore.md.

// backupDatabase writes a consistent snapshot of the database to dest. dest must not exist;
// SQLite's VACUUM INTO refuses to overwrite, so the path is quoted and any single quote
// escaped (the destination is an operator-supplied path, never browser input).
func backupDatabase(dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("refusing to overwrite %s", dest)
	}
	if _, err := db.Exec(`VACUUM INTO '` + strings.ReplaceAll(dest, "'", "''") + `'`); err != nil {
		return err
	}
	return nil
}

// backupCLI backs up the database and, when the key is a file, its encryption key.
func backupCLI(dest string) error {
	initDB()
	if err := backupDatabase(dest); err != nil {
		return err
	}
	keyPath := resolveDBPath() + ".key"
	if _, err := os.Stat(keyPath); err == nil {
		b, err := os.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("backup written but key copy failed: %v", err)
		}
		if err := os.WriteFile(dest+".key", b, 0o600); err != nil {
			return fmt.Errorf("backup written but key copy failed: %v", err)
		}
		fmt.Printf("Backup written to %s (+ %s.key). Keep them in separate places.\n", dest, dest)
		return nil
	}
	fmt.Printf("Backup written to %s. (No key file here; back up the encryption key separately.)\n", dest)
	return nil
}

// restoreCLI restores the database from a validated backup. It runs before the server
// starts, so the database is not open yet.
func restoreCLI(src string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("backup file not found: %s", src)
	}
	f, err := sql.Open("sqlite", src+"?_pragma=busy_timeout(10000)")
	if err != nil {
		return err
	}
	var ok string
	if err := f.QueryRow(`PRAGMA integrity_check`).Scan(&ok); err != nil || ok != "ok" {
		f.Close()
		return fmt.Errorf("not a valid WRM database: %s", src)
	}
	f.Close()
	dbPath := resolveDBPath()
	if _, err := os.Stat(dbPath); err == nil {
		old := dbPath + ".pre-restore-" + time.Now().Format("20060102-150405")
		if err := os.Rename(dbPath, old); err != nil {
			return fmt.Errorf("could not move the current database aside: %v", err)
		}
		fmt.Printf("Current database kept as %s\n", old)
	}
	if err := os.Rename(src, dbPath); err != nil {
		return fmt.Errorf("could not place the restored database: %v", err)
	}
	fmt.Printf("Restored %s. Restore the matching encryption key before starting WRM.\n", dbPath)
	return nil
}

// POST /api/admin/backup — administrator only; returns a consistent database snapshot.
func apiAdminBackupHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	dir, err := os.MkdirTemp("", "wrm-backup-")
	if err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	defer os.RemoveAll(dir)
	name := "wrm-" + AppVersion + "-" + time.Now().UTC().Format("20060102-150405") + ".db"
	dest := filepath.Join(dir, name)
	if err := backupDatabase(dest); err != nil {
		jsonError(w, "Backup failed: "+err.Error(), 500)
		return
	}
	u, _ := loadUser("id=?", uid)
	username := ""
	if u != nil {
		username = u.Username
	}
	auditLogAs(r, uid, username, "admin.backup", "", nil)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, dest)
}
