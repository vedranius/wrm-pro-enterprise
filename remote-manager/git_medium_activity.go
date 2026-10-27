// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Medium activity (v12.6.5): what ran in Medium, who started it, when, how long it took,
// its steps and, as the policy medium_log_output allows, the redacted tail of its output.
//
// Operations that take the shared mediumOperation lock (setup, runtime preparation, tool
// import, start, config validation) go through mediumBeginOp / mediumEndOp, which also
// record who holds the lock, so a refused click says what is running, by whom and since
// when. Short actions (stop, imports kept for later, settings, previews, requests made in
// the original tool, unexpected exits) are single rows written by mediumRecord.

const mediumActivityKeep = 1000
const mediumOutputLines = 200

func initMediumActivitySchema() {
	ensureTable("medium_activity", `CREATE TABLE IF NOT EXISTS medium_activity (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		at TEXT NOT NULL,
		user_id INTEGER NOT NULL DEFAULT 0,
		op TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'running',
		ms INTEGER NOT NULL DEFAULT 0,
		detail TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		steps TEXT NOT NULL DEFAULT '',
		output TEXT NOT NULL DEFAULT '',
		shared INTEGER NOT NULL DEFAULT 0)`,
		"id", "at", "user_id", "op", "status", "ms", "detail", "error", "steps", "output", "shared")
}

// mediumStep is one step of an operation, with its start and duration.
type mediumStep struct {
	Step    string `json:"step"`
	Status  string `json:"status"` // running | ok | error
	Started string `json:"started"`
	Ms      int64  `json:"ms"`
	Error   string `json:"error,omitempty"`
	at      time.Time
}

// mediumOpInfo is the operation holding the shared Medium lock.
type mediumOpInfo struct {
	ID      int64  `json:"id"`
	Op      string `json:"op"`
	UserID  int    `json:"user_id"`
	User    string `json:"user"`
	Started string `json:"started"`
	Mine    bool   `json:"mine"`
	at      time.Time
	out     *mediumRing
	shared  bool
}

// mediumRing keeps the last lines written to it (command and process output).
type mediumRing struct {
	mu      sync.Mutex
	lines   []string
	partial string
}

func (r *mediumRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.partial + strings.ReplaceAll(string(p), "\r\n", "\n")
	parts := strings.Split(s, "\n")
	r.partial = parts[len(parts)-1]
	if len(r.partial) > 4096 {
		parts = append(parts[:len(parts)-1], r.partial, "")
		r.partial = ""
	}
	for _, l := range parts[:len(parts)-1] {
		r.add(l)
	}
	return len(p), nil
}
func (r *mediumRing) add(l string) {
	l = truncateStr(strings.TrimRight(l, "\r"), 500)
	r.lines = append(r.lines, l)
	if len(r.lines) > mediumOutputLines {
		r.lines = r.lines[len(r.lines)-mediumOutputLines:]
	}
}

// Note adds a line written by WRM itself (marked with ▸).
func (r *mediumRing) Note(format string, a ...interface{}) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.add(time.Now().UTC().Format("15:04:05") + " ▸ " + fmt.Sprintf(format, a...))
	r.mu.Unlock()
}

// Tail returns the redacted last lines.
func (r *mediumRing) Tail() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	lines := append([]string{}, r.lines...)
	if r.partial != "" {
		lines = append(lines, truncateStr(r.partial, 500))
	}
	r.mu.Unlock()
	if len(lines) > mediumOutputLines {
		lines = lines[len(lines)-mediumOutputLines:]
	}
	return redactText(strings.Join(lines, "\n"))
}

var mediumOps = struct {
	sync.Mutex
	holder *mediumOpInfo
}{}

// mediumOpShared: operations on the runtime every user shares (shown to everyone).
func mediumOpShared(op string) bool { return op == "prepare" || op == "tool_import" || op == "docker" }

// mediumBusyError says which operation holds the shared lock.
func mediumBusyError() error {
	mediumOps.Lock()
	h := mediumOps.holder
	mediumOps.Unlock()
	if h == nil {
		return fmt.Errorf("Another Medium operation is running; wait for it to finish (its steps are shown in the Medium status)")
	}
	return fmt.Errorf("Medium is busy: %s, started by %s at %s UTC (%s ago). Wait for it to finish; its steps are shown in the Medium status", mediumOpLabel(h.Op), h.User, h.at.UTC().Format("15:04:05"), time.Since(h.at).Round(time.Second))
}

func mediumOpLabel(op string) string {
	switch op {
	case "setup":
		return "prepare and start"
	case "prepare":
		return "runtime preparation"
	case "tool_import":
		return "deploy-tool import and runtime preparation"
	case "start":
		return "start of the original tool"
	case "config_import":
		return "deploy-config check"
	case "ssh_import":
		return "SSH profile import"
	case "docker":
		return "Docker installation"
	}
	return op
}

// mediumBeginOp takes the shared lock for an operation and starts its activity row. The
// caller must end it with mediumEndOp (also from a background goroutine).
func mediumBeginOp(uid int, op, detail string) (*mediumOpInfo, error) {
	if !mediumOperation.TryLock() {
		return nil, mediumBusyError()
	}
	h := &mediumOpInfo{Op: op, UserID: uid, User: usernameOf(uid), at: time.Now(), out: &mediumRing{}, shared: mediumOpShared(op)}
	h.Started = h.at.UTC().Format(time.RFC3339)
	if res, err := db.Exec(`INSERT INTO medium_activity (at, user_id, op, status, detail, shared) VALUES (?,?,?,?,?,?)`, h.Started, uid, op, "running", truncateStr(detail, 500), boolInt(h.shared)); err == nil {
		h.ID, _ = res.LastInsertId()
	}
	mediumOps.Lock()
	mediumOps.holder = h
	mediumOps.Unlock()
	mediumMgr.mu.Lock()
	s := mediumMgr.statuses[mediumUserDir(uid)]
	s.Op, s.OpStarted, s.Steps, s.Error = op, h.Started, nil, ""
	mediumMgr.statuses[mediumUserDir(uid)] = s
	mediumMgr.mu.Unlock()
	h.out.Note("%s started by %s", mediumOpLabel(op), h.User)
	log.Printf("Medium (%s): %s started", h.User, mediumOpLabel(op))
	return h, nil
}

// mediumEndOp closes the operation's steps, writes its outcome and releases the lock.
func mediumEndOp(h *mediumOpInfo, err error) {
	if h == nil {
		return
	}
	took := time.Since(h.at).Round(time.Second)
	if err != nil {
		h.out.Note("failed: %s", err.Error())
		log.Printf("Medium (%s): %s failed after %s: %s", h.User, mediumOpLabel(h.Op), took, redactText(err.Error()))
	} else {
		h.out.Note("finished")
		log.Printf("Medium (%s): %s finished in %s", h.User, mediumOpLabel(h.Op), took)
	}
	key := mediumUserDir(h.UserID)
	mediumMgr.mu.Lock()
	s := mediumMgr.statuses[key]
	mediumCloseStep(&s, err)
	steps := append([]mediumStep{}, s.Steps...)
	mediumMgr.statuses[key] = s
	mediumMgr.mu.Unlock()
	status, msg := "ok", ""
	if err != nil {
		status, msg = "error", truncateStr(redactText(err.Error()), 2000)
	}
	// the row is complete before the status says the operation ended, so a client that
	// sees it end finds its result, steps and output in the activity log
	db.Exec(`UPDATE medium_activity SET status=?, ms=?, error=?, steps=?, output=? WHERE id=?`, status, time.Since(h.at).Milliseconds(), msg, string(jsonMarshal(steps)), encryptValue(h.out.Tail()), h.ID)
	mediumPruneActivity()
	// Busy is cleared while the lock is still held (the holder still shows the operation),
	// then the holder and the lock are released together, with no I/O in between.
	mediumMgr.mu.Lock()
	s = mediumMgr.statuses[key]
	s.Busy = false
	if err != nil {
		s.Stage, s.Error = "error", msg
	} else if s.Stage != "" {
		s.Stage = "ready"
	}
	mediumMgr.statuses[key] = s
	mediumMgr.mu.Unlock()
	mediumOps.Lock()
	if mediumOps.holder == h {
		mediumOps.holder = nil
	}
	mediumOperation.Unlock()
	mediumOps.Unlock()
}

// mediumCloseStep ends the running step (ok, or error with err); mediumMgr.mu is held.
func mediumCloseStep(s *mediumStatus, err error) {
	if n := len(s.Steps); n > 0 && s.Steps[n-1].Status == "running" {
		st := &s.Steps[n-1]
		st.Ms = time.Since(st.at).Milliseconds()
		st.Status = "ok"
		if err != nil {
			st.Status, st.Error = "error", truncateStr(redactText(err.Error()), 500)
		}
	}
}

// mediumOpOutput: the output sink of the operation that holds the lock (nil when none).
func mediumOpOutput() *mediumRing {
	mediumOps.Lock()
	defer mediumOps.Unlock()
	if mediumOps.holder == nil {
		return nil
	}
	return mediumOps.holder.out
}

// mediumHolder returns the operation holding the lock, as the user sees it.
func mediumHolder(uid int) *mediumOpInfo {
	mediumOps.Lock()
	defer mediumOps.Unlock()
	if mediumOps.holder == nil {
		return nil
	}
	h := *mediumOps.holder
	h.Mine = h.UserID == uid
	return &h
}

// mediumRecord writes a finished single action.
func mediumRecord(uid int, op, status string, ms int64, detail, errText, output string) {
	db.Exec(`INSERT INTO medium_activity (at, user_id, op, status, ms, detail, error, output, shared) VALUES (?,?,?,?,?,?,?,?,?)`,
		nowStamp(), uid, op, status, ms, truncateStr(detail, 500), truncateStr(redactText(errText), 2000), encryptValue(redactText(output)), boolInt(mediumOpShared(op)))
	mediumPruneActivity()
}

// The newest rows are kept per user, and the shared runtime's rows separately, so one
// user's activity cannot push out everyone else's history.
func mediumPruneActivity() {
	db.Exec(`DELETE FROM medium_activity WHERE status != 'running' AND id IN (SELECT id FROM (SELECT id, ROW_NUMBER() OVER (PARTITION BY CASE WHEN shared = 1 THEN -1 ELSE user_id END ORDER BY id DESC) AS n FROM medium_activity) WHERE n > ?)`, mediumActivityKeep)
}

// mediumOutputVisible: may this user read the output of that row (policy medium_log_output)?
func mediumOutputVisible(uid, owner int, shared bool) bool {
	switch getSetting("medium_log_output") {
	case "none":
		return false
	case "admins":
		return isAdminUser(uid)
	}
	return isAdminUser(uid) || owner == uid || shared
}

type mediumActivityRow struct {
	ID     int64        `json:"id"`
	At     string       `json:"at"`
	User   string       `json:"user"`
	Mine   bool         `json:"mine"`
	Op     string       `json:"op"`
	Status string       `json:"status"`
	Ms     int64        `json:"ms"`
	Detail string       `json:"detail,omitempty"`
	Error  string       `json:"error,omitempty"`
	Steps  []mediumStep `json:"steps,omitempty"`
	Output *string      `json:"output,omitempty"`
	Shared bool         `json:"shared"`
}

// mediumActivity lists what the user may see: their own rows and the shared runtime's;
// administrators see every user's.
func mediumActivity(uid, limit int, withOutput bool) []mediumActivityRow {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, at, user_id, op, status, ms, detail, error, steps, output, shared FROM medium_activity`
	args := []interface{}{}
	if !isAdminUser(uid) {
		q += ` WHERE user_id=? OR shared=1`
		args = append(args, uid)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	out := []mediumActivityRow{}
	rows, err := db.Query(q, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	names := map[int]string{}
	for rows.Next() {
		var r mediumActivityRow
		var owner, shared int
		var steps, output string
		if rows.Scan(&r.ID, &r.At, &owner, &r.Op, &r.Status, &r.Ms, &r.Detail, &r.Error, &steps, &output, &shared) != nil {
			continue
		}
		if _, ok := names[owner]; !ok {
			names[owner] = usernameOf(owner)
		}
		r.User, r.Mine, r.Shared = names[owner], owner == uid, shared == 1
		if steps != "" {
			json.Unmarshal([]byte(steps), &r.Steps)
		}
		if withOutput && output != "" && mediumOutputVisible(uid, owner, r.Shared) {
			if v := decryptValue(output); v != "" {
				r.Output = &v
			}
		}
		out = append(out, r)
	}
	return out
}

// apiMediumActivity: GET /api/git/medium/activity[?limit=N&output=1], GET /api/git/medium/log
// (the running operation's and the original tool's output so far).
func apiMediumActivity(w http.ResponseWriter, r *http.Request, uid int, action string) {
	if !requireMethod(w, r, "GET") {
		return
	}
	if action == "/log" {
		out := map[string]interface{}{"policy": getSetting("medium_log_output")}
		if h := mediumHolder(uid); h != nil && h.out != nil && mediumOutputVisible(uid, h.UserID, h.shared) {
			out["operation"] = h.out.Tail()
		}
		if mediumOutputVisible(uid, uid, false) {
			mediumMgr.mu.Lock()
			p := mediumMgr.processes[mediumUserDir(uid)]
			mediumMgr.mu.Unlock()
			if p != nil && p.log != nil {
				out["process"] = p.log.Tail()
			}
		}
		jsonOK(w, out)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	jsonOK(w, map[string]interface{}{"rows": mediumActivity(uid, limit, r.URL.Query().Get("output") == "1"), "policy": getSetting("medium_log_output")})
}

// mediumActivityRunningFix marks rows left "running" by a stopped WRM as interrupted.
func mediumActivityRunningFix() {
	db.Exec(`UPDATE medium_activity SET status='interrupted' WHERE status='running'`)
}
