// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// ─── TERMINAL SESSIONS & RECORDING ───────────────────
//
// Every terminal (SSH shell) is a row in terminal_sessions: who, from where, which server,
// when, how it ended. While session_recording is on, the terminal output is recorded in
// asciicast v2 format (https://docs.asciinema.org/manual/asciicast/v2/), gzip-compressed,
// to <recordings dir>/YYYY/MM/<uid>.cast.gz (mode 0600). Recording is written by a
// separate goroutine through a bounded queue, so it never slows the terminal down.
//
// Passwords: the remote shell does not echo passwords, so they never appear in the output.
// Keystrokes are recorded only with session_recording_input, and input typed at a
// password/passphrase/PIN prompt is masked.

const recordingFormat = "asciicast-v2+gzip"

func recordingsDir() string {
	if d := strings.TrimSpace(os.Getenv("WRM_RECORDINGS_DIR")); d != "" {
		return d
	}
	return filepath.Join(filepath.Dir(resolveDBPath()), "recordings")
}

// ── recorder ──

type castEvent struct {
	at   time.Duration
	kind byte // 'o' output, 'i' input, 'r' resize
	data []byte
}

type recordingInfo struct {
	RelPath    string
	Size       int64 // stored (compressed) bytes
	DataBytes  int64 // uncompressed asciicast bytes
	SHA256     string
	DurationMs int64
	Truncated  bool
	Input      bool
	Dropped    int64
}

type recorder struct {
	start       time.Time
	ch          chan castEvent
	done        chan struct{}
	recordInput bool
	maxBytes    int64

	mu       sync.Mutex // serialises producers (stdout + stderr pumps, input, resize)
	closed   bool
	outCarry []byte
	inCarry  []byte
	tail     []byte // recent output, to detect password prompts
	masking  bool
	dropped  atomic.Int64

	// owned by the writer goroutine
	relPath   string
	file      *os.File
	gz        *gzip.Writer
	bw        *bufio.Writer
	sum       hash.Hash
	stored    int64
	dataBytes int64
	truncated bool
	lastAt    time.Duration
	err       error
}

type countWriter struct {
	w io.Writer
	n *int64
}

func (c countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	*c.n += int64(n)
	return n, err
}

func newRecorder(uid string, cols, rows int, title, term string, recordInput bool, maxBytes int64) (*recorder, error) {
	now := time.Now()
	rel := filepath.ToSlash(filepath.Join(now.UTC().Format("2006"), now.UTC().Format("01"), uid+".cast.gz"))
	abs := filepath.Join(recordingsDir(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	rec := &recorder{
		start: now, ch: make(chan castEvent, 8192), done: make(chan struct{}),
		recordInput: recordInput, maxBytes: maxBytes, relPath: rel, file: f, sum: sha256.New(),
	}
	rec.gz, _ = gzip.NewWriterLevel(countWriter{io.MultiWriter(f, rec.sum), &rec.stored}, gzip.BestSpeed)
	rec.bw = bufio.NewWriterSize(rec.gz, 64*1024)
	if term == "" {
		term = "xterm-256color"
	}
	hdr, _ := json.Marshal(struct {
		Version   int               `json:"version"`
		Width     int               `json:"width"`
		Height    int               `json:"height"`
		Timestamp int64             `json:"timestamp"`
		Title     string            `json:"title,omitempty"`
		Env       map[string]string `json:"env"`
	}{2, cols, rows, now.Unix(), title, map[string]string{"TERM": term}})
	rec.writeLine(hdr)
	go rec.loop()
	return rec, nil
}

func (rec *recorder) writeLine(b []byte) {
	if rec.err != nil {
		return
	}
	if _, err := rec.bw.Write(b); err != nil {
		rec.err = err
		return
	}
	if err := rec.bw.WriteByte('\n'); err != nil {
		rec.err = err
	}
	rec.dataBytes += int64(len(b)) + 1
}

func (rec *recorder) writeEvent(ev castEvent) {
	if rec.truncated && ev.kind != 'r' {
		return
	}
	data, _ := json.Marshal(string(ev.data))
	line := make([]byte, 0, len(data)+24)
	line = append(line, '[')
	line = strconv.AppendFloat(line, ev.at.Seconds(), 'f', 6, 64)
	line = append(line, ", \""...)
	line = append(line, ev.kind)
	line = append(line, "\", "...)
	line = append(line, data...)
	line = append(line, ']')
	rec.writeLine(line)
	rec.lastAt = ev.at
	if rec.maxBytes > 0 && rec.dataBytes > rec.maxBytes && !rec.truncated {
		rec.truncated = true
		note, _ := json.Marshal("\r\n\x1b[33m[WRM: recording stopped, size limit reached]\x1b[0m\r\n")
		rec.writeLine([]byte(fmt.Sprintf("[%s, \"o\", %s]", strconv.FormatFloat(ev.at.Seconds(), 'f', 6, 64), note)))
	}
}

func (rec *recorder) loop() {
	defer close(rec.done)
	flush := time.NewTicker(2 * time.Second)
	defer flush.Stop()
	for {
		select {
		case ev, ok := <-rec.ch:
			if !ok {
				return
			}
			rec.writeEvent(ev)
		case <-flush.C:
			// Keep the file readable up to the last few seconds if the server stops.
			if rec.err == nil {
				if err := rec.bw.Flush(); err == nil {
					rec.gz.Flush()
				}
			}
		}
	}
}

func (rec *recorder) enqueue(ev castEvent) {
	select {
	case rec.ch <- ev:
	default:
		rec.dropped.Add(int64(len(ev.data)))
	}
}

// splitUTF8 returns the complete UTF-8 prefix of b and the bytes of a rune cut at the end.
func splitUTF8(b []byte) ([]byte, []byte) {
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				return b[:i], b[i:]
			}
			break
		}
	}
	return b, nil
}

var (
	ansiRe     = regexp.MustCompile(`\x1b(\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(\x07|\x1b\\)|[@-Z\\-_])`)
	passwordRe = regexp.MustCompile(`(?i)(password|passphrase|pass phrase|passcode|\bpin\b|token|otp|one-time|verification code|secret)[^\n]*[:?]\s*$`)
)

// Output records terminal output (safe to call from several goroutines).
func (rec *recorder) Output(p []byte) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.closed || len(p) == 0 {
		return
	}
	buf := append(rec.outCarry, p...)
	complete, carry := splitUTF8(buf)
	rec.outCarry = append([]byte(nil), carry...)
	if len(complete) == 0 {
		return
	}
	rec.enqueue(castEvent{at: time.Since(rec.start), kind: 'o', data: append([]byte(nil), complete...)})
	if rec.recordInput {
		rec.tail = append(rec.tail, complete...)
		if len(rec.tail) > 512 {
			rec.tail = append([]byte(nil), rec.tail[len(rec.tail)-512:]...)
		}
	}
}

// atPasswordPrompt reports whether the last output line looks like a secret prompt.
func (rec *recorder) atPasswordPrompt() bool {
	s := ansiRe.ReplaceAllString(string(rec.tail), "")
	if i := strings.LastIndexAny(s, "\r\n"); i >= 0 {
		s = s[i+1:]
	}
	return passwordRe.MatchString(s)
}

// Input records keystrokes when input recording is enabled. Typing at a password prompt
// is masked until Enter.
func (rec *recorder) Input(p []byte) {
	if !rec.recordInput {
		return
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.closed || len(p) == 0 {
		return
	}
	if !rec.masking && rec.atPasswordPrompt() {
		rec.masking = true
	}
	data := append(append([]byte(nil), rec.inCarry...), p...)
	if rec.masking {
		masked := make([]byte, 0, len(data))
		for _, c := range data {
			switch {
			case c == '\r' || c == '\n':
				masked = append(masked, c)
				rec.masking = false
				rec.tail = rec.tail[:0]
			case rec.masking && c >= 0x20 && c != 0x7f && (c < 0x80 || utf8.RuneStart(c)):
				masked = append(masked, '*')
			case rec.masking:
				// continuation bytes of a masked rune, control keys: drop
			default:
				masked = append(masked, c)
			}
		}
		rec.inCarry = nil
		data = masked
	} else {
		var carry []byte
		data, carry = splitUTF8(data)
		rec.inCarry = append([]byte(nil), carry...)
	}
	if len(data) > 0 {
		rec.enqueue(castEvent{at: time.Since(rec.start), kind: 'i', data: data})
	}
}

// Resize records a terminal size change.
func (rec *recorder) Resize(cols, rows int) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.closed {
		return
	}
	rec.enqueue(castEvent{at: time.Since(rec.start), kind: 'r', data: []byte(fmt.Sprintf("%dx%d", cols, rows))})
}

// Close finishes the recording and returns its size and checksum.
func (rec *recorder) Close() (recordingInfo, error) {
	rec.mu.Lock()
	if rec.closed {
		rec.mu.Unlock()
		return recordingInfo{}, fmt.Errorf("already closed")
	}
	rec.closed = true
	if len(rec.outCarry) > 0 {
		rec.enqueue(castEvent{at: time.Since(rec.start), kind: 'o', data: rec.outCarry})
	}
	close(rec.ch)
	rec.mu.Unlock()
	<-rec.done
	if rec.err == nil {
		rec.err = rec.bw.Flush()
	}
	if err := rec.gz.Close(); err != nil && rec.err == nil {
		rec.err = err
	}
	if err := rec.file.Close(); err != nil && rec.err == nil {
		rec.err = err
	}
	info := recordingInfo{
		RelPath: rec.relPath, Size: rec.stored, DataBytes: rec.dataBytes, SHA256: hex.EncodeToString(rec.sum.Sum(nil)),
		DurationMs: rec.lastAt.Milliseconds(), Truncated: rec.truncated, Input: rec.recordInput, Dropped: rec.dropped.Load(),
	}
	if d := time.Since(rec.start).Milliseconds(); d > info.DurationMs {
		info.DurationMs = d
	}
	return info, rec.err
}

// ── terminal session lifecycle ──

type termAudit struct {
	ID      int64
	UID     string
	r       *http.Request
	userID  int
	user    string
	conn    Connection
	shareID int
	rec     atomic.Pointer[recorder]
	start   time.Time
	ended   atomic.Bool
	BytesIn atomic.Int64
	BytesOu atomic.Int64
}

func nullableInt(n int) interface{} {
	if n <= 0 {
		return nil
	}
	return n
}

// startTerminalSession records the start of a connection attempt.
func startTerminalSession(r *http.Request, acc *connAccess, c Connection) *termAudit {
	uid, name := acc.actor()
	t := &termAudit{UID: randomToken(12), r: r, userID: uid, user: name, conn: c, start: time.Now()}
	pkey, shareName := "", ""
	if acc.Share != nil {
		t.shareID, shareName, pkey = acc.Share.Share.ID, acc.Share.Share.Name, acc.Share.PKey
	}
	ua := r.UserAgent()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	res, err := db.Exec(`INSERT INTO terminal_sessions (uid, user_id, username, pkey, share_id, share_name, conn_id, conn_name, conn_owner_id,
		host, remote_user, protocol, client_ip, user_agent, started_at, status) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'connecting')`,
		t.UID, nullableInt(uid), name, pkey, nullableInt(t.shareID), shareName, c.ID, c.Name, nullableInt(c.UserID),
		c.Host, c.Username, "ssh", clientIP(r), ua, t.start.UTC().Format(time.RFC3339))
	if err != nil {
		log.Printf("terminal session: %v", err)
	} else {
		t.ID, _ = res.LastInsertId()
	}
	return t
}

func (t *termAudit) ref() auditRef { return auditRef{ConnID: t.conn.ID, SessionID: int(t.ID)} }

// validRecordingLevel reports whether an account may store this level.
func validRecordingLevel(v string) bool {
	switch v {
	case "", "none", "metadata", "input":
		return true
	}
	return false
}

// userRecordingLevel resolves the effective recording level for a user:
// "none", "metadata" (output only) or "input" (output and keystrokes). The
// server setting is the master switch; a level chosen on the account overrides
// the type, and an empty level follows session_recording_input.
func userRecordingLevel(userID int) (string, bool) {
	if !settingBool("session_recording") {
		return "none", false
	}
	level := ""
	if userID > 0 {
		db.QueryRow(`SELECT recording_level FROM users WHERE id=?`, userID).Scan(&level)
	}
	switch level {
	case "none":
		return "none", false
	case "metadata", "input":
		return level, true
	}
	if settingBool("session_recording_input") {
		return "input", true
	}
	return "metadata", true
}

// connected marks the session active and starts recording when enabled. It returns true
// when the session is being recorded.
func (t *termAudit) connected(cols, rows int, term string) bool {
	db.Exec(`UPDATE terminal_sessions SET status='active' WHERE id=?`, t.ID)
	d := map[string]interface{}{"host": t.conn.Host, "user": t.conn.Username}
	if t.shareID > 0 {
		d["share_id"] = t.shareID
	}
	if level, on := userRecordingLevel(t.userID); on && t.ID > 0 {
		maxBytes := int64(settingInt("recording_max_mb")) << 20
		title := t.conn.Username + "@" + t.conn.Host + " (" + t.conn.Name + ") — " + t.user
		rec, err := newRecorder(t.UID, cols, rows, title, term, level == "input", maxBytes)
		if err != nil {
			log.Printf("recording: %v", err)
			d["recording_error"] = err.Error()
		} else {
			t.rec.Store(rec)
			d["recorded"] = true
			d["recording_level"] = level
		}
	}
	auditLogRef(t.r, t.userID, t.user, "terminal.open", t.conn.Name, d, t.ref())
	return t.rec.Load() != nil
}

// output, input and resize feed the recording (no-ops while nothing is recorded).
func (t *termAudit) output(p []byte) {
	t.BytesOu.Add(int64(len(p)))
	if rec := t.rec.Load(); rec != nil {
		rec.Output(p)
	}
}

func (t *termAudit) input(p []byte) {
	t.BytesIn.Add(int64(len(p)))
	if rec := t.rec.Load(); rec != nil {
		rec.Input(p)
	}
}

func (t *termAudit) resize(cols, rows int) {
	if rec := t.rec.Load(); rec != nil {
		rec.Resize(cols, rows)
	}
}

// failed marks a connection attempt that never got a shell.
func (t *termAudit) failed(reason string) {
	if !t.ended.CompareAndSwap(false, true) {
		return
	}
	db.Exec(`UPDATE terminal_sessions SET status='failed', ended_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), t.ID)
	auditLogRef(t.r, t.userID, t.user, "terminal.connect_failed", t.conn.Name, map[string]interface{}{"host": t.conn.Host, "error": truncateStr(reason, 300)}, t.ref())
}

// end finishes the session: closes the recording, stores its checksum, writes the audit entry.
func (t *termAudit) end(status string, exitCode *int) {
	if !t.ended.CompareAndSwap(false, true) {
		return
	}
	d := map[string]interface{}{"host": t.conn.Host, "status": status, "seconds": int(time.Since(t.start).Seconds()),
		"bytes_out": t.BytesOu.Load(), "bytes_in": t.BytesIn.Load()}
	if rec := t.rec.Load(); rec != nil {
		info, err := rec.Close()
		if err != nil {
			log.Printf("recording %s: %v", t.UID, err)
			d["recording_error"] = err.Error()
		}
		if _, err := db.Exec(`INSERT INTO session_recordings (session_id, format, path, size_bytes, data_bytes, sha256, duration_ms,
			input_recorded, truncated, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			t.ID, recordingFormat, info.RelPath, info.Size, info.DataBytes, info.SHA256, info.DurationMs,
			boolInt(info.Input), boolInt(info.Truncated), time.Now().UTC().Format(time.RFC3339)); err != nil {
			log.Printf("recording row: %v", err)
		}
		d["recording_sha256"] = info.SHA256
		d["recording_bytes"] = info.Size
		if info.Truncated {
			d["recording_truncated"] = true
		}
		if info.Dropped > 0 {
			d["recording_dropped_bytes"] = info.Dropped
		}
	}
	var ec interface{}
	if exitCode != nil {
		ec = *exitCode
		d["exit_code"] = *exitCode
	}
	db.Exec(`UPDATE terminal_sessions SET status=?, ended_at=?, exit_code=?, bytes_in=?, bytes_out=? WHERE id=?`,
		status, time.Now().UTC().Format(time.RFC3339), ec, t.BytesIn.Load(), t.BytesOu.Load(), t.ID)
	auditLogRef(t.r, t.userID, t.user, "terminal.close", t.conn.Name, d, t.ref())
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// recoverTerminalSessions closes sessions left open by a crash or restart and registers
// their (partial) recordings, so nothing recorded is lost.
func recoverTerminalSessions() {
	rows, err := db.Query(`SELECT id, uid, started_at FROM terminal_sessions WHERE status IN ('connecting','active')`)
	if err != nil {
		return
	}
	type open struct {
		id           int64
		uid, started string
	}
	var list []open
	for rows.Next() {
		var o open
		if rows.Scan(&o.id, &o.uid, &o.started) == nil {
			list = append(list, o)
		}
	}
	rows.Close()
	for _, o := range list {
		ended := time.Now().UTC().Format(time.RFC3339)
		if t, err := time.Parse(time.RFC3339, o.started); err == nil {
			rel := filepath.ToSlash(filepath.Join(t.UTC().Format("2006"), t.UTC().Format("01"), o.uid+".cast.gz"))
			format := recordingFormat
			if _, err := os.Stat(filepath.Join(recordingsDir(), filepath.FromSlash(rel))); err != nil {
				rel = strings.TrimSuffix(rel, ".cast.gz") + ".guac.gz"
				format = guacRecordingFormat
			}
			abs := filepath.Join(recordingsDir(), filepath.FromSlash(rel))
			if st, err := os.Stat(abs); err == nil {
				ended = st.ModTime().UTC().Format(time.RFC3339)
				var n int
				db.QueryRow(`SELECT COUNT(*) FROM session_recordings WHERE session_id=?`, o.id).Scan(&n)
				if n == 0 {
					sum, size := fileSHA256(abs)
					db.Exec(`INSERT INTO session_recordings (session_id, format, path, size_bytes, data_bytes, sha256, duration_ms,
						input_recorded, truncated, created_at) VALUES (?,?,?,?,0,?,?,0,1,?)`,
						o.id, format, rel, size, sum, st.ModTime().Sub(t).Milliseconds(), ended)
				}
			}
		}
		db.Exec(`UPDATE terminal_sessions SET status='interrupted', ended_at=? WHERE id=?`, ended, o.id)
	}
	if len(list) > 0 {
		log.Printf("Recovered %d terminal session(s) interrupted by a server stop", len(list))
	}
}

func fileSHA256(p string) (string, int64) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0
	}
	defer f.Close()
	h := sha256.New()
	n, _ := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n
}

// cleanupRecordings deletes recordings older than recording_retention_days (retention job).
func cleanupRecordings() {
	days := settingInt("recording_retention_days")
	if days < auditMinRetentionDays {
		days = auditMinRetentionDays
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	rows, err := db.Query(`SELECT id, path FROM session_recordings WHERE created_at < ?`, cutoff)
	if err != nil {
		return
	}
	type old struct {
		id   int64
		path string
	}
	var list []old
	for rows.Next() {
		var o old
		if rows.Scan(&o.id, &o.path) == nil {
			list = append(list, o)
		}
	}
	rows.Close()
	for _, o := range list {
		if abs, ok := recordingAbsPath(o.path); ok {
			if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
				log.Printf("retention: %v", err)
				continue
			}
		}
		db.Exec(`DELETE FROM session_recordings WHERE id=?`, o.id)
	}
	if len(list) > 0 {
		log.Printf("Retention: removed %d session recording(s) older than %d days", len(list), days)
	}
}

// recordingAbsPath resolves a stored relative path inside the recordings directory.
func recordingAbsPath(rel string) (string, bool) {
	if rel == "" || strings.Contains(rel, "..") || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.Join(recordingsDir(), filepath.FromSlash(rel)), true
}

// ── file transfers ──

type transferRec struct {
	Direction string // upload | download | s2s
	SrcConn   *Connection
	SrcPath   string
	DstConn   *Connection
	DstPath   string
	Size      int64
	SHA256    string
	Status    string // ok | failed
	Error     string
}

// logFileTransfer stores one transferred file (with size and checksum) for the audit trail.
func logFileTransfer(r *http.Request, acc *connAccess, t transferRec) {
	uid, name := acc.actor()
	shareID := 0
	if acc.Share != nil {
		shareID = acc.Share.Share.ID
	}
	logFileTransferAs(uid, name, clientIP(r), shareID, t)
}

// logFileTransferAs stores a transfer for an actor without a request (AI sessions).
func logFileTransferAs(uid int, name, ip string, shareID int, t transferRec) {
	if !settingBool("audit_enabled") {
		return
	}
	var srcID, dstID interface{}
	srcHost, dstHost := "", ""
	if t.SrcConn != nil {
		srcID, srcHost = t.SrcConn.ID, t.SrcConn.Host
	}
	if t.DstConn != nil {
		dstID, dstHost = t.DstConn.ID, t.DstConn.Host
	}
	if t.Status == "" {
		t.Status = "ok"
	}
	if _, err := db.Exec(`INSERT INTO file_transfers (ts, user_id, username, client_ip, share_id, direction, src_conn_id, src_host, src_path,
		dst_conn_id, dst_host, dst_path, size_bytes, sha256, status, error) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		time.Now().UTC().Format(time.RFC3339), nullableInt(uid), name, ip, nullableInt(shareID), t.Direction,
		srcID, srcHost, t.SrcPath, dstID, dstHost, t.DstPath, t.Size, t.SHA256, t.Status, truncateStr(t.Error, 300)); err != nil {
		log.Printf("file transfer log: %v", err)
	}
}

// hashingWriter computes the SHA-256 of everything written through it.
type hashingWriter struct {
	w io.Writer
	h hash.Hash
	n int64
}

func newHashingWriter(w io.Writer) *hashingWriter { return &hashingWriter{w: w, h: sha256.New()} }

func (hw *hashingWriter) Write(p []byte) (int, error) {
	n, err := hw.w.Write(p)
	hw.h.Write(p[:n])
	hw.n += int64(n)
	return n, err
}

func (hw *hashingWriter) Sum() string { return hex.EncodeToString(hw.h.Sum(nil)) }

// ─── API: SESSIONS & RECORDINGS ──────────────────────

type termSessionView struct {
	ID         int64                  `json:"id"`
	UID        string                 `json:"uid"`
	UserID     *int                   `json:"user_id"`
	Username   string                 `json:"username"`
	ShareName  string                 `json:"share"`
	ConnID     *int                   `json:"conn_id"`
	ConnName   string                 `json:"connection"`
	Host       string                 `json:"host"`
	RemoteUser string                 `json:"remote_user"`
	ClientIP   string                 `json:"client_ip"`
	UserAgent  string                 `json:"user_agent,omitempty"`
	StartedAt  string                 `json:"started_at"`
	EndedAt    string                 `json:"ended_at"`
	DurationMs int64                  `json:"duration_ms"`
	Status     string                 `json:"status"`
	Protocol   string                 `json:"protocol"`
	ExitCode   *int                   `json:"exit_code"`
	BytesOut   int64                  `json:"bytes_out"`
	BytesIn    int64                  `json:"bytes_in"`
	Recording  map[string]interface{} `json:"recording"`
}

// sessionVisibility limits non-admins to their own sessions and sessions on their connections.
func sessionVisibility(userID int, admin, mine bool) (string, []interface{}) {
	if admin && !mine {
		return "1=1", nil
	}
	return "(s.user_id = ? OR s.conn_owner_id = ?)", []interface{}{userID, userID}
}

const termSessionCols = `s.id, s.uid, s.user_id, s.username, s.share_name, s.conn_id, s.conn_name, s.host, s.remote_user, s.client_ip,
	s.user_agent, s.started_at, s.ended_at, s.status, s.exit_code, s.bytes_out, s.bytes_in, s.protocol,
	r.id, r.size_bytes, r.data_bytes, r.sha256, r.duration_ms, r.input_recorded, r.truncated, r.format`

func scanTermSession(sc interface{ Scan(...interface{}) error }) (termSessionView, error) {
	var v termSessionView
	var uid, cid, ec, rid, rsize, rdata, rdur, rin, rtr sql.NullInt64
	var rsha, rfmt sql.NullString
	err := sc.Scan(&v.ID, &v.UID, &uid, &v.Username, &v.ShareName, &cid, &v.ConnName, &v.Host, &v.RemoteUser, &v.ClientIP,
		&v.UserAgent, &v.StartedAt, &v.EndedAt, &v.Status, &ec, &v.BytesOut, &v.BytesIn, &v.Protocol,
		&rid, &rsize, &rdata, &rsha, &rdur, &rin, &rtr, &rfmt)
	if err != nil {
		return v, err
	}
	v.UserID, v.ConnID, v.ExitCode = nullIntPtr(uid), nullIntPtr(cid), nullIntPtr(ec)
	if st, err := time.Parse(time.RFC3339, v.StartedAt); err == nil {
		end := time.Now()
		if et, err := time.Parse(time.RFC3339, v.EndedAt); err == nil {
			end = et
		}
		v.DurationMs = end.Sub(st).Milliseconds()
	}
	if rid.Valid {
		v.Recording = map[string]interface{}{"id": rid.Int64, "size": rsize.Int64, "data_bytes": rdata.Int64, "sha256": rsha.String,
			"duration_ms": rdur.Int64, "input": rin.Int64 == 1, "truncated": rtr.Int64 == 1, "format": rfmt.String}
	}
	return v, nil
}

// GET /api/recordings?mine&user&conn&host&status&q&from&to&before_id&limit   list of terminal sessions
// GET /api/recordings/{id}                                                    one session + its audit entries
// GET /api/recordings/{id}/cast[?download=1]                                  the asciicast v2 recording
func apiRecordingsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", 405)
		return
	}
	admin := isAdminUser(userID) || isAuditorUser(userID) // auditors see every session read-only
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/recordings"), "/")
	if rest == "" {
		listTermSessions(w, r, userID, admin)
		return
	}
	parts := strings.Split(rest, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 || len(parts) > 2 {
		jsonError(w, "Not found", 404)
		return
	}
	vis, args := sessionVisibility(userID, admin, false)
	row := db.QueryRow(`SELECT `+termSessionCols+` FROM terminal_sessions s
		LEFT JOIN session_recordings r ON r.session_id = s.id WHERE s.id = ? AND `+vis, append([]interface{}{id}, args...)...)
	v, err := scanTermSession(row)
	if err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	if len(parts) == 1 {
		events := []auditEntry{}
		if rows, err := db.Query(`SELECT id, ts, user_id, username, ip, action, target, details, conn_id, session_id, hash
			FROM audit_log WHERE session_id = ? ORDER BY id`, id); err == nil {
			for rows.Next() {
				var e auditEntry
				var uid, cid, sid sql.NullInt64
				rows.Scan(&e.ID, &e.TS, &uid, &e.Username, &e.IP, &e.Action, &e.Target, &e.Details, &cid, &sid, &e.Hash)
				e.UserID, e.ConnID, e.SessionID = nullIntPtr(uid), nullIntPtr(cid), nullIntPtr(sid)
				events = append(events, e)
			}
			rows.Close()
		}
		jsonOK(w, map[string]interface{}{"session": v, "events": events})
		return
	}
	if (parts[1] != "cast" && parts[1] != "guac") || v.Recording == nil {
		jsonError(w, "No recording for this session", 404)
		return
	}
	isGuac := v.Recording["format"] == guacRecordingFormat
	if isGuac != (parts[1] == "guac") {
		jsonError(w, "This recording is available as /"+map[bool]string{true: "guac", false: "cast"}[isGuac], 404)
		return
	}
	var rel string
	db.QueryRow(`SELECT path FROM session_recordings WHERE session_id = ?`, id).Scan(&rel)
	abs, ok := recordingAbsPath(rel)
	if !ok {
		jsonError(w, "Recording not found", 404)
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		jsonError(w, "Recording file is missing", 404)
		return
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		jsonError(w, "Recording file is damaged", 500)
		return
	}
	defer gz.Close()
	download := r.URL.Query().Get("download") == "1"
	action := "recording.view"
	if download {
		action = "recording.download"
	}
	auditLogRef(r, userID, usernameOf(userID), action, v.ConnName, map[string]interface{}{"session_user": v.Username, "host": v.Host}, auditRef{ConnID: intOr0(v.ConnID), SessionID: int(v.ID)})
	w.Header().Set("Content-Type", "application/x-asciicast; charset=utf-8")
	ext := "cast"
	if isGuac {
		w.Header().Set("Content-Type", "application/octet-stream")
		ext = "guac"
	}
	w.Header().Set("Cache-Control", "no-store")
	if download {
		name := fmt.Sprintf("wrm-%s-%s-%d.%s", safeFileName(v.ConnName), strings.NewReplacer(":", "", "-", "").Replace(v.StartedAt), v.ID, ext)
		w.Header().Set("Content-Disposition", attachmentHeader(name))
	}
	// A recording cut off by a crash ends with a truncated gzip stream: serve what is readable.
	if _, err := io.Copy(w, gz); err != nil && err != io.ErrUnexpectedEOF {
		log.Printf("recording %d: %v", id, err)
	}
}

func intOr0(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func safeFileName(s string) string {
	out := strings.Map(func(r rune) rune {
		if r < 128 && (r == '-' || r == '_' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return r
		}
		return '_'
	}, s)
	if len(out) > 40 {
		out = out[:40]
	}
	if out == "" {
		out = "session"
	}
	return out
}

func listTermSessions(w http.ResponseWriter, r *http.Request, userID int, admin bool) {
	q := r.URL.Query()
	vis, args := sessionVisibility(userID, admin, q.Get("mine") == "1")
	where := []string{vis}
	if s := strings.TrimSpace(q.Get("user")); s != "" {
		where = append(where, "s.username = ?")
		args = append(args, s)
	}
	if n, err := strconv.Atoi(q.Get("conn")); err == nil && n > 0 {
		where = append(where, "s.conn_id = ?")
		args = append(args, n)
	}
	if s := strings.TrimSpace(q.Get("host")); s != "" {
		where = append(where, "s.host LIKE ?")
		args = append(args, "%"+s+"%")
	}
	if s := strings.TrimSpace(q.Get("status")); s != "" {
		where = append(where, "s.status = ?")
		args = append(args, s)
	}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		like := "%" + s + "%"
		where = append(where, "(s.username LIKE ? OR s.conn_name LIKE ? OR s.host LIKE ? OR s.share_name LIKE ? OR s.client_ip LIKE ?)")
		args = append(args, like, like, like, like, like)
	}
	if s := strings.TrimSpace(q.Get("from")); s != "" {
		where = append(where, "s.started_at >= ?")
		args = append(args, normalizeTimeBound(s, false))
	}
	if s := strings.TrimSpace(q.Get("to")); s != "" {
		where = append(where, "s.started_at <= ?")
		args = append(args, normalizeTimeBound(s, true))
	}
	if n, err := strconv.Atoi(q.Get("before_id")); err == nil && n > 0 {
		where = append(where, "s.id < ?")
		args = append(args, n)
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)
	rows, err := db.Query(`SELECT `+termSessionCols+` FROM terminal_sessions s
		LEFT JOIN session_recordings r ON r.session_id = s.id WHERE `+strings.Join(where, " AND ")+` ORDER BY s.id DESC LIMIT ?`, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []termSessionView{}
	for rows.Next() {
		if v, err := scanTermSession(rows); err == nil {
			v.UserAgent = ""
			out = append(out, v)
		}
	}
	jsonOK(w, out)
}

// ─── API: FILE TRANSFERS (admin) ─────────────────────

type fileTransferView struct {
	ID        int64  `json:"id"`
	TS        string `json:"ts"`
	UserID    *int   `json:"user_id"`
	Username  string `json:"username"`
	ClientIP  string `json:"client_ip"`
	Direction string `json:"direction"`
	SrcConnID *int   `json:"src_conn_id"`
	SrcHost   string `json:"src_host"`
	SrcPath   string `json:"src_path"`
	DstConnID *int   `json:"dst_conn_id"`
	DstHost   string `json:"dst_host"`
	DstPath   string `json:"dst_path"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Status    string `json:"status"`
	Error     string `json:"error"`
}

// GET /api/admin/transfers?q&user&conn&direction&from&to&before_id&limit[&format=csv]
func apiAdminTransfersHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAuditRead(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", 405)
		return
	}
	q := r.URL.Query()
	where := []string{"1=1"}
	args := []interface{}{}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		like := "%" + s + "%"
		where = append(where, "(username LIKE ? OR src_host LIKE ? OR src_path LIKE ? OR dst_host LIKE ? OR dst_path LIKE ? OR sha256 LIKE ?)")
		args = append(args, like, like, like, like, like, like)
	}
	if s := strings.TrimSpace(q.Get("user")); s != "" {
		where = append(where, "username = ?")
		args = append(args, s)
	}
	if n, err := strconv.Atoi(q.Get("conn")); err == nil && n > 0 {
		where = append(where, "(src_conn_id = ? OR dst_conn_id = ?)")
		args = append(args, n, n)
	}
	if s := strings.TrimSpace(q.Get("direction")); s != "" {
		where = append(where, "direction = ?")
		args = append(args, s)
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
	rows, err := db.Query(`SELECT id, ts, user_id, username, client_ip, direction, src_conn_id, src_host, src_path, dst_conn_id, dst_host,
		dst_path, size_bytes, sha256, status, error FROM file_transfers WHERE `+strings.Join(where, " AND ")+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []fileTransferView{}
	for rows.Next() {
		var v fileTransferView
		var uid, sid, did sql.NullInt64
		if rows.Scan(&v.ID, &v.TS, &uid, &v.Username, &v.ClientIP, &v.Direction, &sid, &v.SrcHost, &v.SrcPath, &did, &v.DstHost,
			&v.DstPath, &v.Size, &v.SHA256, &v.Status, &v.Error) == nil {
			v.UserID, v.SrcConnID, v.DstConnID = nullIntPtr(uid), nullIntPtr(sid), nullIntPtr(did)
			out = append(out, v)
		}
	}
	if !csvOut {
		jsonOK(w, out)
		return
	}
	auditLog(r, adminID, "admin.transfers_export", "", map[string]interface{}{"rows": len(out)})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", attachmentHeader("wrm-file-transfers-"+time.Now().Format("20060102-150405")+".csv"))
	cw := csv.NewWriter(w)
	cw.Write([]string{"id", "time_utc", "user", "ip", "direction", "source_host", "source_path", "destination_host", "destination_path", "bytes", "sha256", "status", "error"})
	for _, v := range out {
		cw.Write([]string{strconv.FormatInt(v.ID, 10), v.TS, csvSafe(v.Username), v.ClientIP, v.Direction, csvSafe(v.SrcHost), csvSafe(v.SrcPath),
			csvSafe(v.DstHost), csvSafe(v.DstPath), strconv.FormatInt(v.Size, 10), v.SHA256, v.Status, csvSafe(v.Error)})
	}
	cw.Flush()
}
