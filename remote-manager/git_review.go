// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"time"
)

func initGitReview() {
	ensureTable("git_review_manifests", `CREATE TABLE IF NOT EXISTS git_review_manifests(user_id INTEGER NOT NULL,hash TEXT NOT NULL,data TEXT NOT NULL,PRIMARY KEY(user_id,hash))`, "user_id", "hash", "data")
	ensureTable("git_review_tickets", `CREATE TABLE IF NOT EXISTS git_review_tickets(id TEXT PRIMARY KEY,user_id INTEGER NOT NULL,install_id INTEGER NOT NULL,kind TEXT NOT NULL,ref TEXT NOT NULL,manifest TEXT NOT NULL,expires_at TEXT NOT NULL)`, "id", "user_id", "install_id", "kind", "ref", "manifest", "expires_at")
}
func storeGitReview(uid, install int, kind, ref string, target gitTarget) (string, error) {
	raw, err := json.Marshal(target)
	if err != nil || len(raw) > 8<<20 {
		return "", fmt.Errorf("review target exceeds 8 MiB")
	}
	hash := sha256Hex(string(raw))
	ticket := randomToken(24)
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	tx.Exec(`DELETE FROM git_review_tickets WHERE expires_at<?`, nowRFC())
	tx.Exec(`DELETE FROM git_review_tickets WHERE user_id=? AND id NOT IN (SELECT id FROM git_review_tickets WHERE user_id=? ORDER BY expires_at DESC LIMIT 127)`, uid, uid)
	if _, err := tx.Exec(`INSERT OR IGNORE INTO git_review_manifests(user_id,hash,data) VALUES(?,?,?)`, uid, hash, encryptValue(string(raw))); err != nil {
		return "", err
	}
	if _, err := tx.Exec(`INSERT INTO git_review_tickets(id,user_id,install_id,kind,ref,manifest,expires_at) VALUES(?,?,?,?,?,?,?)`, sha256Hex(ticket), uid, install, kind, ref, hash, time.Now().Add(2*time.Hour).UTC().Format(time.RFC3339)); err != nil {
		return "", err
	}
	tx.Exec(`DELETE FROM git_review_manifests WHERE NOT EXISTS(SELECT 1 FROM git_review_tickets t WHERE t.user_id=git_review_manifests.user_id AND t.manifest=git_review_manifests.hash)`)
	return ticket, tx.Commit()
}
func loadGitReview(uid, install int, kind, ref, ticket string) (gitTarget, error) {
	var data string
	var target gitTarget
	err := db.QueryRow(`SELECT m.data FROM git_review_tickets t JOIN git_review_manifests m ON m.user_id=t.user_id AND m.hash=t.manifest WHERE t.id=? AND t.user_id=? AND t.install_id=? AND t.kind=? AND t.ref=? AND t.expires_at>?`, sha256Hex(ticket), uid, install, kind, ref, nowRFC()).Scan(&data)
	if err != nil || json.Unmarshal([]byte(decryptValue(data)), &target) != nil {
		return target, fmt.Errorf("review expired or does not match this installation/action; review the plan again")
	}
	src, ok := loadGitSource(uid, target.SourceID)
	if !ok {
		return target, fmt.Errorf("reviewed source is no longer authorized")
	}
	if src.Kind != "bundle" && target.Source != src.Kind+":"+src.URL {
		return target, fmt.Errorf("source changed since review; review the plan again")
	}
	return target, nil
}
func reviewedContentHash(t gitTarget) string {
	t.ComputedAt = ""
	t.APICalls = 0
	t.Warnings = nil
	return sha256Hex(string(jsonMarshal(t)))
}
