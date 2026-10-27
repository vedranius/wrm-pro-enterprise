// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ─── FILE EDIT HISTORY (#129) ────────────────────────
//
// Every file saved from the file editor and every file the AI assistant writes leaves an entry
// in file_edits: who, when, which server and path, from where (editor, AI session, share) and
// the unified diff of the change, the same diff the editor shows before saving. The diff is
// computed on the WRM server from the content before and after the write (the browser cannot
// supply it), secret-looking values are masked (redactText) before it is stored, and it is
// encrypted at rest. Files larger than file_edit_max_kb or binary files are recorded without
// a diff (sizes and SHA-256 only). Entries follow the audit retention; administrators and
// security auditors read them in Admin → File edits (filters: user, server, source, session,
// time) and as CSV. The policy file_edit_history switches the diffs off; the audit log keeps
// its file.upload entries either way.

func initFileEditsSchema() {
	ensureTable("file_edits", `CREATE TABLE IF NOT EXISTS file_edits (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts TEXT NOT NULL,
		user_id INTEGER DEFAULT NULL,
		username TEXT NOT NULL DEFAULT '',
		client_ip TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'editor',
		session TEXT NOT NULL DEFAULT '',
		share_id INTEGER DEFAULT NULL,
		conn_id INTEGER DEFAULT NULL,
		conn_name TEXT NOT NULL DEFAULT '',
		host TEXT NOT NULL DEFAULT '',
		path TEXT NOT NULL DEFAULT '',
		created INTEGER NOT NULL DEFAULT 0,
		old_size INTEGER NOT NULL DEFAULT 0,
		new_size INTEGER NOT NULL DEFAULT 0,
		old_sha256 TEXT NOT NULL DEFAULT '',
		new_sha256 TEXT NOT NULL DEFAULT '',
		added INTEGER NOT NULL DEFAULT 0,
		removed INTEGER NOT NULL DEFAULT 0,
		diff TEXT NOT NULL DEFAULT '',
		note TEXT NOT NULL DEFAULT '')`,
		"id", "ts", "user_id", "username", "client_ip", "source", "session", "share_id", "conn_id", "conn_name", "host", "path",
		"created", "old_size", "new_size", "old_sha256", "new_sha256", "added", "removed", "diff", "note")
	db.Exec(`CREATE INDEX IF NOT EXISTS ix_file_edits_ts ON file_edits(ts)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS ix_file_edits_user ON file_edits(user_id, ts)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS ix_file_edits_conn ON file_edits(conn_id, ts)`)
}

// fileEditMax is the largest file (each side) whose diff is kept.
func fileEditMax() int { return settingInt("file_edit_max_kb") << 10 }

type fileEditRec struct {
	UserID  int
	User    string
	IP      string
	Source  string // editor | ai
	Session string // AI session or share
	ShareID int
	Conn    Connection
	Path    string
	Created bool
	Old     []byte
	New     []byte
	OldBig  bool // the old content was larger than fileEditMax (not read completely)
	NewBig  bool
	OldSize int64
	NewSize int64
}

// recordFileEdit stores one change; it returns the entry id (0 when nothing was stored).
func recordFileEdit(e fileEditRec) int64 {
	if !settingBool("file_edit_history") {
		return 0
	}
	sum := func(b []byte, big bool) string {
		if big {
			return ""
		}
		h := sha256.Sum256(b)
		return hex.EncodeToString(h[:])
	}
	if e.OldSize == 0 && !e.OldBig {
		e.OldSize = int64(len(e.Old))
	}
	if e.NewSize == 0 && !e.NewBig {
		e.NewSize = int64(len(e.New))
	}
	diff, note := "", ""
	added, removed := 0, 0
	switch {
	case e.OldBig || e.NewBig:
		note = "too_large"
	case isBinary(e.Old) || isBinary(e.New) || !utf8.Valid(e.Old) || !utf8.Valid(e.New):
		note = "binary"
	default:
		old := string(e.Old)
		if e.Created {
			old = ""
		}
		lines, _ := unifiedDiff(splitLines(old), splitLines(string(e.New)), 3)
		for _, l := range lines {
			switch l.Op {
			case "+":
				added++
			case "-":
				removed++
			}
		}
		if len(lines) == 0 {
			note = "no_change"
		}
		diff = encryptValue(redactText(aiUnifiedDiff(e.Path, old, string(e.New), e.Created)))
	}
	var share interface{}
	if e.ShareID > 0 {
		share = e.ShareID
	}
	var uid interface{}
	if e.UserID > 0 {
		uid = e.UserID
	}
	res, err := db.Exec(`INSERT INTO file_edits (ts, user_id, username, client_ip, source, session, share_id, conn_id, conn_name, host, path, created,
		old_size, new_size, old_sha256, new_sha256, added, removed, diff, note) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		nowStamp(), uid, e.User, e.IP, e.Source, truncateStr(e.Session, 120), share, e.Conn.ID, e.Conn.Name, e.Conn.Host, truncateStr(e.Path, 2000),
		map[bool]int{true: 1, false: 0}[e.Created], e.OldSize, e.NewSize, sum(e.Old, e.OldBig || e.Created), sum(e.New, e.NewBig),
		added, removed, diff, note)
	if err != nil {
		return 0
	}
	id, _ := res.LastInsertId()
	return id
}

// cappedRead reads at most max bytes; big reports that there was more.
func cappedRead(r io.Reader, max int) ([]byte, bool, error) {
	b, err := io.ReadAll(io.LimitReader(r, int64(max)+1))
	if len(b) > max {
		return b[:max], true, err
	}
	return b, false, err
}

// editTee keeps the first max bytes written through it (the new content of an edited file).
type editTee struct {
	buf bytes.Buffer
	max int
	big bool
}

func (t *editTee) Write(p []byte) (int, error) {
	if room := t.max - t.buf.Len(); room > 0 {
		if len(p) > room {
			t.buf.Write(p[:room])
			t.big = true
		} else {
			t.buf.Write(p)
		}
	} else if len(p) > 0 {
		t.big = true
	}
	return len(p), nil
}

// ─── reading ─────────────────────────────────────────

type fileEditView struct {
	ID       int64  `json:"id"`
	TS       string `json:"ts"`
	UserID   int    `json:"user_id,omitempty"`
	Username string `json:"username"`
	ClientIP string `json:"client_ip"`
	Source   string `json:"source"`
	Session  string `json:"session,omitempty"`
	ConnID   int    `json:"conn_id,omitempty"`
	ConnName string `json:"conn_name"`
	Host     string `json:"host"`
	Path     string `json:"path"`
	Created  bool   `json:"created"`
	OldSize  int64  `json:"old_size"`
	NewSize  int64  `json:"new_size"`
	OldSHA   string `json:"old_sha256"`
	NewSHA   string `json:"new_sha256"`
	Added    int    `json:"added"`
	Removed  int    `json:"removed"`
	Note     string `json:"note,omitempty"`
	Diff     string `json:"diff,omitempty"`
}

const fileEditCols = `id, ts, user_id, username, client_ip, source, session, conn_id, conn_name, host, path, created, old_size, new_size, old_sha256, new_sha256, added, removed, note`

func scanFileEdit(rows interface{ Scan(...interface{}) error }, withDiff bool) (fileEditView, error) {
	var v fileEditView
	var uid, cid sql.NullInt64
	var created int
	dest := []interface{}{&v.ID, &v.TS, &uid, &v.Username, &v.ClientIP, &v.Source, &v.Session, &cid, &v.ConnName, &v.Host, &v.Path, &created,
		&v.OldSize, &v.NewSize, &v.OldSHA, &v.NewSHA, &v.Added, &v.Removed, &v.Note}
	var diff string
	if withDiff {
		dest = append(dest, &diff)
	}
	if err := rows.Scan(dest...); err != nil {
		return v, err
	}
	v.UserID, v.ConnID, v.Created = int(uid.Int64), int(cid.Int64), created == 1
	if diff != "" {
		v.Diff = decryptValue(diff)
	}
	return v, nil
}

// GET /api/admin/file-edits?q=&user=&source=&conn=&session=&from=&to=&before_id=&format=csv
// GET /api/admin/file-edits/{id}  (with the diff)
func apiAdminFileEditsHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireAuditRead(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/file-edits"), "/"); rest != "" {
		id, err := strconv.ParseInt(rest, 10, 64)
		if err != nil || id <= 0 {
			jsonError(w, "Not found", 404)
			return
		}
		v, err := scanFileEdit(db.QueryRow(`SELECT `+fileEditCols+`, diff FROM file_edits WHERE id=?`, id), true)
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		auditLog(r, uid, "file_edit.viewed", v.Path, map[string]interface{}{"edit_id": v.ID, "host": v.Host})
		jsonOK(w, v)
		return
	}
	q := r.URL.Query()
	where := []string{"1=1"}
	args := []interface{}{}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		like := "%" + s + "%"
		where = append(where, "(username LIKE ? OR conn_name LIKE ? OR host LIKE ? OR path LIKE ? OR session LIKE ?)")
		args = append(args, like, like, like, like, like)
	}
	if s := strings.TrimSpace(q.Get("user")); s != "" {
		where = append(where, "username = ?")
		args = append(args, s)
	}
	if s := strings.TrimSpace(q.Get("source")); s != "" {
		where = append(where, "source = ?")
		args = append(args, s)
	}
	if s := strings.TrimSpace(q.Get("session")); s != "" {
		where = append(where, "session = ?")
		args = append(args, s)
	}
	if s := strings.TrimSpace(q.Get("server")); s != "" {
		where = append(where, "(conn_name = ? OR host = ?)")
		args = append(args, s, s)
	}
	if n, err := strconv.Atoi(q.Get("conn")); err == nil && n > 0 {
		where = append(where, "conn_id = ?")
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
	if n, err := strconv.Atoi(q.Get("before_id")); err == nil && n > 0 {
		where = append(where, "id < ?")
		args = append(args, n)
	}
	csvOut := q.Get("format") == "csv"
	limit, _ := strconv.Atoi(q.Get("limit"))
	if csvOut {
		limit = 100000
	} else if limit <= 0 || limit > 1000 {
		limit = 200
	}
	args = append(args, limit)
	rows, err := db.Query(`SELECT `+fileEditCols+` FROM file_edits WHERE `+strings.Join(where, " AND ")+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		jsonError(w, "Cannot read the file edits", 500)
		return
	}
	defer rows.Close()
	out := []fileEditView{}
	for rows.Next() {
		if v, err := scanFileEdit(rows, false); err == nil {
			out = append(out, v)
		}
	}
	if !csvOut {
		jsonOK(w, out)
		return
	}
	auditLog(r, uid, "file_edit.exported", "", map[string]interface{}{"rows": len(out)})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", attachmentHeader("wrm-file-edits.csv"))
	cw := csv.NewWriter(w)
	cw.Write([]string{"id", "time", "user", "client_ip", "source", "session", "connection", "host", "path", "created", "old_size", "new_size", "old_sha256", "new_sha256", "added", "removed", "note"})
	for _, v := range out {
		row := []string{fmt.Sprint(v.ID), v.TS, v.Username, v.ClientIP, v.Source, v.Session, v.ConnName, v.Host, v.Path, strconv.FormatBool(v.Created),
			fmt.Sprint(v.OldSize), fmt.Sprint(v.NewSize), v.OldSHA, v.NewSHA, fmt.Sprint(v.Added), fmt.Sprint(v.Removed), v.Note}
		for i := range row {
			row[i] = csvSafe(row[i])
		}
		cw.Write(row)
	}
	cw.Flush()
}
