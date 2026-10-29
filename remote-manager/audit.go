// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── AUDIT LOG ───────────────────────────────────────
//
// Security-relevant events (sign-ins, account and policy changes, connections, shares,
// terminals, file transfers, collaboration moderation) are written to the audit_log table
// and to the server log as a single "AUDIT" line, so they can also be shipped to a SIEM
// via journald/syslog.
//
// The audit trail is append-only:
//   - database triggers refuse UPDATE of audit_log, file_transfers, file_edits and session_recordings,
//     and DELETE of entries younger than the minimum retention (7 days);
//   - only the retention job removes old entries;
//   - every entry carries the SHA-256 of the previous one (hash chain), so a changed or
//     removed entry is detected by Admin → Audit log → Verify integrity.
//
// audit_enabled = 0 stops recording normal events; administrator actions (admin.*) and
// security events (security.*: alerts, lockdown) are
// always recorded, including the change of that setting itself.

const auditMinRetentionDays = 7

var auditChain = struct {
	sync.Mutex
	loaded bool
	last   string
}{}

// auditRef links an audit entry to a connection and/or a terminal session.
type auditRef struct {
	ConnID    int
	SessionID int
}

func auditLog(r *http.Request, userID int, action, target string, details interface{}) {
	username := ""
	if userID > 0 {
		db.QueryRow(`SELECT username FROM users WHERE id=?`, userID).Scan(&username)
	}
	auditWrite(r, userID, username, action, target, details, auditRef{})
}

func auditLogAs(r *http.Request, userID int, username, action, target string, details interface{}) {
	auditWrite(r, userID, username, action, target, details, auditRef{})
}

func auditLogRef(r *http.Request, userID int, username, action, target string, details interface{}, ref auditRef) {
	auditWrite(r, userID, username, action, target, details, ref)
}

// secretKeys are never written to the audit log, whatever a caller passes in.
var secretKeys = []string{"password", "passwd", "secret", "private_key", "passphrase", "token", "credential", "totp"}

func redactDetails(details interface{}) interface{} {
	redact := func(k string) bool {
		k = strings.ToLower(k)
		for _, s := range secretKeys {
			if k == s || strings.HasSuffix(k, "_"+s) || strings.HasPrefix(k, s+"_") {
				return true
			}
		}
		return false
	}
	switch d := details.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(d))
		for k, v := range d {
			if redact(k) {
				out[k] = "[redacted]"
			} else {
				out[k] = v
			}
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(d))
		for k, v := range d {
			if redact(k) {
				out[k] = "[redacted]"
			} else {
				out[k] = v
			}
		}
		return out
	}
	return details
}

func auditEnabled(action string) bool {
	return settingBool("audit_enabled") || strings.HasPrefix(action, "admin.") || strings.HasPrefix(action, "security.")
}

func auditWrite(r *http.Request, userID int, username, action, target string, details interface{}, ref auditRef) {
	if !auditEnabled(action) {
		return
	}
	ip := ""
	if r != nil {
		ip = clientIP(r)
	}
	det := ""
	if details != nil {
		if s, ok := details.(string); ok {
			det = s
		} else if b, err := json.Marshal(redactDetails(details)); err == nil && string(b) != "null" && string(b) != "{}" {
			det = string(b)
		}
	}
	if len(det) > 4000 {
		det = det[:4000]
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	var uid, connID, sessID interface{}
	if userID > 0 {
		uid = userID
	}
	if ref.ConnID > 0 {
		connID = ref.ConnID
	}
	if ref.SessionID > 0 {
		sessID = ref.SessionID
	}

	auditChain.Lock()
	if !auditChain.loaded {
		auditChain.last = lastAuditHash()
		auditChain.loaded = true
	}
	prev := auditChain.last
	h := auditHashKeyed(prev, ts, userID, username, ip, action, target, det, ref.ConnID, ref.SessionID)
	_, err := db.Exec(`INSERT INTO audit_log (ts, user_id, username, ip, action, target, details, conn_id, session_id, prev_hash, hash)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, ts, uid, username, ip, action, target, det, connID, sessID, prev, h)
	if err == nil {
		auditChain.last = h
	}
	auditChain.Unlock()
	if err == nil {
		alertObserve(action, username, ip, target, det)
		siemNotify()
	}
	if err != nil {
		log.Printf("audit: %v", err)
	}
	extra := ""
	if ref.ConnID > 0 {
		extra += " conn=" + strconv.Itoa(ref.ConnID)
	}
	if ref.SessionID > 0 {
		extra += " session=" + strconv.Itoa(ref.SessionID)
	}
	// One audit event is one log line: line breaks in any field cannot start a fake entry.
	line := fmt.Sprintf("AUDIT action=%s user=%q ip=%s target=%q%s %s", action, username, ip, target, extra, det)
	line = strings.ReplaceAll(line, "\n", " ")
	line = strings.ReplaceAll(line, "\r", " ")
	log.Print(line)
}

func lastAuditHash() string {
	var h string
	db.QueryRow(`SELECT hash FROM audit_log WHERE hash != '' ORDER BY id DESC LIMIT 1`).Scan(&h)
	return h
}

// auditHash is the chain hash of one entry: SHA-256 over the previous hash and the fields.
func auditHash(prev, ts string, userID int, username, ip, action, target, details string, connID, sessionID int) string {
	b, _ := json.Marshal([]interface{}{prev, ts, userID, username, ip, action, target, details, connID, sessionID})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// auditHashKeyed is the current chain hash: HMAC-SHA256 with a key derived from the
// installation's encryption key. A copied database (or SQL write access) can no longer
// recompute the chain without the key. The "h2:" prefix keeps old chains verifiable.
func auditHashKeyed(prev, ts string, userID int, username, ip, action, target, details string, connID, sessionID int) string {
	material := append(append([]byte{}, encryptionKey...), []byte("wrm-audit-chain")...)
	key := sha256.Sum256(material)
	mac := hmac.New(sha256.New, key[:])
	b, _ := json.Marshal([]interface{}{prev, ts, userID, username, ip, action, target, details, connID, sessionID})
	mac.Write(b)
	return "h2:" + hex.EncodeToString(mac.Sum(nil))
}

// createAppendOnlyTriggers makes the audit tables append-only at the database level.
// Rows younger than auditMinRetentionDays cannot be deleted at all; older ones only by
// the retention job (which is the only code that deletes from these tables).
func createAppendOnlyTriggers() {
	cutoff := `strftime('%Y-%m-%dT%H:%M:%SZ','now','-` + strconv.Itoa(auditMinRetentionDays) + ` days')`
	for _, t := range []struct{ table, tsCol string }{
		{"audit_log", "ts"}, {"file_transfers", "ts"}, {"session_recordings", "created_at"}, {"file_edits", "ts"},
	} {
		for _, q := range []string{
			`CREATE TRIGGER IF NOT EXISTS ` + t.table + `_no_update BEFORE UPDATE ON ` + t.table +
				` BEGIN SELECT RAISE(ABORT, '` + t.table + ` is append-only'); END`,
			`CREATE TRIGGER IF NOT EXISTS ` + t.table + `_no_delete BEFORE DELETE ON ` + t.table +
				` WHEN OLD.` + t.tsCol + ` > ` + cutoff +
				` BEGIN SELECT RAISE(ABORT, '` + t.table + ` is append-only: recent entries cannot be deleted'); END`,
		} {
			if _, err := db.Exec(q); err != nil {
				log.Printf("Trigger warning (%s): %v", t.table, err)
			}
		}
	}
}

// cleanupAuditLog is the retention job: the only place that deletes audit data.
func cleanupAuditLog() {
	days := settingInt("audit_retention_days")
	if days < auditMinRetentionDays {
		days = auditMinRetentionDays
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	db.Exec(`DELETE FROM audit_log WHERE ts < ?`, cutoff)
	db.Exec(`DELETE FROM collab_messages WHERE ts < ?`, cutoff)
	db.Exec(`DELETE FROM file_transfers WHERE ts < ?`, cutoff)
	db.Exec(`DELETE FROM file_edits WHERE ts < ?`, cutoff)
	// Session metadata is kept as long as the audit log, but never while it still has a recording.
	db.Exec(`DELETE FROM terminal_sessions WHERE started_at < ? AND ended_at != ''
		AND id NOT IN (SELECT session_id FROM session_recordings)`, cutoff)
	cleanupRecordings()
}

// ─── QUERY & EXPORT ──────────────────────────────────

type auditEntry struct {
	ID        int    `json:"id"`
	TS        string `json:"ts"`
	UserID    *int   `json:"user_id"`
	Username  string `json:"username"`
	IP        string `json:"ip"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Details   string `json:"details"`
	ConnID    *int   `json:"conn_id"`
	SessionID *int   `json:"session_id"`
	Hash      string `json:"hash"`
}

func queryAudit(r *http.Request, limit int) ([]auditEntry, error) {
	q := r.URL.Query()
	where := []string{"1=1"}
	args := []interface{}{}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		where = append(where, "(action LIKE ? OR target LIKE ? OR details LIKE ? OR username LIKE ? OR ip LIKE ?)")
		like := "%" + s + "%"
		args = append(args, like, like, like, like, like)
	}
	action := strings.TrimSpace(q.Get("action"))
	if action == "" {
		action = strings.TrimSpace(q.Get("type"))
	}
	if action != "" {
		where = append(where, "action LIKE ?")
		args = append(args, action+"%")
	}
	if s := strings.TrimSpace(q.Get("user")); s != "" {
		where = append(where, "username = ?")
		args = append(args, s)
	}
	if n, err := strconv.Atoi(q.Get("conn")); err == nil && n > 0 {
		where = append(where, "conn_id = ?")
		args = append(args, n)
	}
	if n, err := strconv.Atoi(q.Get("session")); err == nil && n > 0 {
		where = append(where, "session_id = ?")
		args = append(args, n)
	}
	if s := strings.TrimSpace(q.Get("from")); s != "" {
		where = append(where, "ts >= ?")
		args = append(args, normalizeTimeBound(s, false))
	}
	if s := strings.TrimSpace(q.Get("to")); s != "" {
		where = append(where, "ts <= ?")
		args = append(args, normalizeTimeBound(s, true))
	}
	if before, err := strconv.Atoi(q.Get("before_id")); err == nil && before > 0 {
		where = append(where, "id < ?")
		args = append(args, before)
	}
	args = append(args, limit)
	rows, err := db.Query(`SELECT id, ts, user_id, username, ip, action, target, details, conn_id, session_id, hash FROM audit_log WHERE `+
		strings.Join(where, " AND ")+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []auditEntry{}
	for rows.Next() {
		var e auditEntry
		var uid, cid, sid sql.NullInt64
		rows.Scan(&e.ID, &e.TS, &uid, &e.Username, &e.IP, &e.Action, &e.Target, &e.Details, &cid, &sid, &e.Hash)
		e.UserID, e.ConnID, e.SessionID = nullIntPtr(uid), nullIntPtr(cid), nullIntPtr(sid)
		out = append(out, e)
	}
	return out, nil
}

func nullIntPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// normalizeTimeBound accepts "2026-10-01" or an RFC 3339 time and returns a value that
// compares correctly with stored RFC 3339 UTC timestamps. A bare date as upper bound
// includes that whole day.
func normalizeTimeBound(s string, upper bool) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		if upper {
			t = t.Add(24*time.Hour - time.Second)
		}
		return t.UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return s
}

// GET /api/admin/audit?q&action|type&user&conn&session&from&to&before_id&limit   (JSON)
// GET /api/admin/audit?format=csv                                                  (CSV download)
// GET /api/admin/audit/verify                                                      (hash chain check)
func apiAdminAuditHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAuditRead(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/verify") {
		res := verifyAuditChain()
		auditLog(r, adminID, "admin.audit_verify", "", map[string]interface{}{"ok": res.OK, "checked": res.Checked, "broken_at": res.BrokenAt})
		jsonOK(w, res)
		return
	}
	if r.URL.Query().Get("format") == "csv" {
		entries, err := queryAudit(r, 100000)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		auditLog(r, adminID, "admin.audit_export", "", map[string]interface{}{"rows": len(entries)})
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", attachmentHeader("wrm-audit-"+time.Now().Format("20060102-150405")+".csv"))
		cw := csv.NewWriter(w)
		cw.Write([]string{"id", "time_utc", "user", "ip", "action", "target", "details", "connection_id", "session_id", "hash"})
		for _, e := range entries {
			cw.Write([]string{strconv.Itoa(e.ID), e.TS, csvSafe(e.Username), e.IP, e.Action, csvSafe(e.Target), csvSafe(e.Details),
				intPtrStr(e.ConnID), intPtrStr(e.SessionID), e.Hash})
		}
		cw.Flush()
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	entries, err := queryAudit(r, limit)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, entries)
}

func intPtrStr(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

// ─── INTEGRITY CHECK ─────────────────────────────────

type auditVerifyResult struct {
	OK       bool   `json:"ok"`
	Checked  int    `json:"checked"`
	FirstID  int    `json:"first_id"`
	LastID   int    `json:"last_id"`
	Unsigned int    `json:"unsigned"` // entries written without a hash (before v10.1)
	BrokenAt int    `json:"broken_at,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// verifyAuditChain recomputes every hash. The oldest remaining entry is accepted as the
// start of the chain (older entries may have been removed by the retention job).
func verifyAuditChain() auditVerifyResult {
	res := auditVerifyResult{OK: true}
	rows, err := db.Query(`SELECT id, ts, user_id, username, ip, action, target, details, conn_id, session_id, prev_hash, hash
		FROM audit_log ORDER BY id`)
	if err != nil {
		return auditVerifyResult{Reason: err.Error()}
	}
	defer rows.Close()
	prevHash := ""
	started := false
	for rows.Next() {
		var id int
		var ts, username, ip, action, target, details, prev, h string
		var uid, cid, sid sql.NullInt64
		if err := rows.Scan(&id, &ts, &uid, &username, &ip, &action, &target, &details, &cid, &sid, &prev, &h); err != nil {
			return auditVerifyResult{Reason: err.Error()}
		}
		if h == "" {
			if started {
				res.Unsigned++
			}
			continue
		}
		if res.FirstID == 0 {
			res.FirstID = id
		}
		if started && prev != prevHash {
			res.OK, res.BrokenAt, res.Reason = false, id, "an entry before this one was removed or changed"
			break
		}
		want := auditHash(prev, ts, int(uid.Int64), username, ip, action, target, details, int(cid.Int64), int(sid.Int64))
		if strings.HasPrefix(h, "h2:") {
			want = auditHashKeyed(prev, ts, int(uid.Int64), username, ip, action, target, details, int(cid.Int64), int(sid.Int64))
		}
		if want != h {
			res.OK, res.BrokenAt, res.Reason = false, id, "this entry was changed"
			break
		}
		started = true
		prevHash = h
		res.Checked++
		res.LastID = id
	}
	if res.OK && res.Unsigned > 0 {
		res.Reason = "entries without a hash were added after the chain started (written by an older WRM version?)"
	}
	return res
}

// csvSafe neutralises values that spreadsheet programs would interpret as formulas.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
