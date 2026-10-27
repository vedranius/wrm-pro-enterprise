// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ─── SNIPPETS & RUN ON CONNECT ───────────────────────
//
// Snippets are saved commands that go into a terminal with one click (or Ctrl+Shift+Space).
// Every user has their own snippets; administrators can publish snippets for everybody
// ("shared": read-only for the others). A snippet applies to all connections, to the
// connections of one folder, or to one connection.
//
// Snippets marked "run on connect" are typed into the shell by the server right after a
// terminal of a matching connection connects (also after a reconnect), e.g. `sudo -i`,
// `cd /srv/app`, `tmux attach || tmux new`.
//
// Variables are filled in when the snippet is used:
//   {{host}} {{port}} {{user}} {{name}} {{folder}} {{wrm_user}} {{date}} {{time}}
//   {{?Label}} / {{?Label=default}}  asked in the browser before the snippet runs
//                                    (snippets with prompts cannot run on connect)

const maxSnippetsPerUser = 1000

type Snippet struct {
	ID          int    `json:"id"`
	UserID      int    `json:"-"`
	Owner       string `json:"owner,omitempty"`
	Mine        bool   `json:"mine"`
	Name        string `json:"name"`
	Command     string `json:"command"`
	Description string `json:"description"`
	Group       string `json:"group"`
	Scope       string `json:"scope"`    // all | folder | connection
	ScopeID     int    `json:"scope_id"` // folder or connection id
	AutoRun     bool   `json:"auto_run"`
	Shared      bool   `json:"shared"`
	Sort        int    `json:"sort"`
	UpdatedAt   string `json:"updated_at"`
	literal     bool   // typed as it is (a start directory cd), no variables
}

const snippetCols = `id, user_id, name, command, description, grp, scope, scope_id, auto_run, shared, sort, updated_at`

func scanSnippet(sc interface{ Scan(...interface{}) error }) (Snippet, error) {
	var s Snippet
	var auto, shared int
	err := sc.Scan(&s.ID, &s.UserID, &s.Name, &s.Command, &s.Description, &s.Group, &s.Scope, &s.ScopeID, &auto, &shared, &s.Sort, &s.UpdatedAt)
	s.AutoRun, s.Shared = auto == 1, shared == 1
	return s, err
}

func loadSnippets(where string, args ...interface{}) []Snippet {
	rows, err := db.Query(`SELECT `+snippetCols+` FROM snippets WHERE `+where+` ORDER BY grp, sort, name COLLATE NOCASE, id`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Snippet
	for rows.Next() {
		if s, err := scanSnippet(rows); err == nil {
			out = append(out, s)
		}
	}
	return out
}

// snippetsForUser returns the user's own snippets and the shared snippets of others.
func snippetsForUser(userID int) []Snippet {
	out := []Snippet{}
	names := map[int]string{}
	for _, s := range loadSnippets(`user_id=? OR shared=1`, userID) {
		s.Mine = s.UserID == userID
		if !s.Mine {
			if _, ok := names[s.UserID]; !ok {
				names[s.UserID] = usernameOf(s.UserID)
			}
			s.Owner = names[s.UserID]
		}
		out = append(out, s)
	}
	return out
}

var snippetPromptRe = regexp.MustCompile(`\{\{\s*\?[^}]*\}\}`)
var snippetVarRe = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)

func validateSnippet(s *Snippet, userID int) error {
	s.Name = strings.TrimSpace(s.Name)
	s.Group = strings.TrimSpace(s.Group)
	s.Description = strings.TrimSpace(s.Description)
	s.Command = strings.TrimRight(strings.ReplaceAll(s.Command, "\r\n", "\n"), " \t\n")
	s.Scope = strings.ToLower(strings.TrimSpace(s.Scope))
	if s.Name == "" || len(s.Name) > 120 {
		return fmt.Errorf("name is required (at most 120 characters)")
	}
	if strings.TrimSpace(s.Command) == "" || len(s.Command) > 20000 {
		return fmt.Errorf("command is required (at most 20000 characters)")
	}
	if len(s.Description) > 1000 || len(s.Group) > 60 {
		return fmt.Errorf("description or group is too long")
	}
	switch s.Scope {
	case "", "all":
		s.Scope, s.ScopeID = "all", 0
	case "folder":
		if s.ScopeID <= 0 || !userOwnsFolder(s.ScopeID, userID) {
			return fmt.Errorf("folder not found")
		}
	case "connection":
		if s.ScopeID <= 0 || !userOwnsConnection(s.ScopeID, userID) {
			return fmt.Errorf("connection not found")
		}
	default:
		return fmt.Errorf("unknown scope %q", s.Scope)
	}
	if s.Shared {
		if !isAdminUser(userID) {
			return fmt.Errorf("only administrators can share snippets with everybody")
		}
		if s.Scope != "all" {
			return fmt.Errorf("shared snippets apply to all connections")
		}
		if s.AutoRun {
			return fmt.Errorf("shared snippets cannot run on connect")
		}
	}
	if s.AutoRun && snippetPromptRe.MatchString(s.Command) {
		return fmt.Errorf("snippets with {{?…}} prompts cannot run on connect")
	}
	return nil
}

// expandSnippet fills in the variables of a snippet for connection c. Prompt variables
// ({{?…}}) are left as they are; unknown variables too.
func expandSnippet(cmd string, c Connection, wrmUser string) string {
	host, port := c.Host, ""
	if h, p, err := net.SplitHostPort(c.Host); err == nil {
		host, port = h, p
	} else if p := defaultPortFor(c.Protocol); p != "" {
		port = p
	}
	folder := ""
	if c.FolderID != nil {
		db.QueryRow(`SELECT name FROM folders WHERE id=?`, *c.FolderID).Scan(&folder)
	}
	now := time.Now()
	vals := map[string]string{
		"host": host, "port": port, "user": c.Username, "name": c.Name, "folder": folder,
		"wrm_user": wrmUser, "date": now.Format("2006-01-02"), "time": now.Format("15:04:05"),
	}
	return snippetVarRe.ReplaceAllStringFunc(cmd, func(m string) string {
		k := snippetVarRe.FindStringSubmatch(m)[1]
		if v, ok := vals[k]; ok {
			return v
		}
		return m
	})
}

func defaultPortFor(protocol string) string {
	switch strings.ToUpper(protocol) {
	case "FTP", "FTPS":
		return "21"
	case "HTTP":
		return "80"
	case "HTTPS":
		return "443"
	case "SSH", "SFTP", "":
		return "22"
	case "RDP":
		return "3389"
	case "VNC":
		return "5900"
	case "TELNET":
		return "23"
	}
	return ""
}

// autoRunSnippets returns the "run on connect" snippets of the owner of c that apply to c:
// all-connection snippets first, then folder snippets, then connection snippets.
func autoRunSnippets(c Connection) []Snippet {
	folderID := 0
	if c.FolderID != nil {
		folderID = *c.FolderID
	}
	list := loadSnippets(`user_id=? AND auto_run=1 AND shared=0 AND (scope='all' OR (scope='folder' AND scope_id=?) OR (scope='connection' AND scope_id=?))`,
		c.UserID, folderID, c.ID)
	rank := map[string]int{"all": 0, "folder": 1, "connection": 2}
	out := make([]Snippet, 0, len(list))
	for r := 0; r <= 2; r++ {
		for _, s := range list {
			if rank[s.Scope] == r {
				out = append(out, s)
			}
		}
	}
	return out
}

// snippetKeystrokes turns a command into what is typed into the shell: one line per
// command, each ended with Enter.
func snippetKeystrokes(cmd string) []byte {
	cmd = strings.ReplaceAll(cmd, "\r\n", "\n")
	return []byte(strings.ReplaceAll(cmd, "\n", "\r") + "\r")
}

func notifySnippetsChanged(userID int, shared bool) {
	msg := []byte(`{"type":"snippets_changed"}`)
	if shared {
		hub.sendAll(msg)
		return
	}
	hub.sendTo(userID, msg)
}

// ─── API ─────────────────────────────────────────────

// GET    /api/snippets          my snippets + shared snippets
// POST   /api/snippets          create
// PUT    /api/snippets/{id}     update (owner; administrators also shared snippets)
// DELETE /api/snippets/{id}
func apiSnippetsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/snippets"), "/")
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			jsonOK(w, snippetsForUser(userID))
		case http.MethodPost:
			var s Snippet
			if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
				jsonError(w, "Invalid request", 400)
				return
			}
			if err := validateSnippet(&s, userID); err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
			var n int
			db.QueryRow(`SELECT COUNT(1) FROM snippets WHERE user_id=?`, userID).Scan(&n)
			if n >= maxSnippetsPerUser {
				jsonError(w, fmt.Sprintf("at most %d snippets per user", maxSnippetsPerUser), 400)
				return
			}
			now := time.Now().UTC().Format(time.RFC3339)
			res, err := db.Exec(`INSERT INTO snippets (user_id, name, command, description, grp, scope, scope_id, auto_run, shared, sort, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				userID, s.Name, s.Command, s.Description, s.Group, s.Scope, s.ScopeID, boolInt(s.AutoRun), boolInt(s.Shared), s.Sort, now, now)
			if err != nil {
				jsonError(w, "Server error", 500)
				return
			}
			id, _ := res.LastInsertId()
			s.ID, s.Mine, s.UpdatedAt = int(id), true, now
			auditLog(r, userID, "snippet.created", s.Name, snippetAuditDetails(s))
			notifySnippetsChanged(userID, s.Shared)
			jsonOK(w, s)
		default:
			jsonError(w, "Method not allowed", 405)
		}
		return
	}
	id, err := strconv.Atoi(rest)
	if err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	old, err := scanSnippet(db.QueryRow(`SELECT `+snippetCols+` FROM snippets WHERE id=?`, id))
	if err != nil || (old.UserID != userID && !(old.Shared && isAdminUser(userID))) {
		jsonError(w, "Not found", 404)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var s Snippet
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			jsonError(w, "Invalid request", 400)
			return
		}
		// Scope and sharing are checked against the snippet's owner.
		if err := validateSnippet(&s, old.UserID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if s.Shared && !isAdminUser(userID) {
			jsonError(w, "only administrators can share snippets with everybody", 400)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		db.Exec(`UPDATE snippets SET name=?, command=?, description=?, grp=?, scope=?, scope_id=?, auto_run=?, shared=?, sort=?, updated_at=? WHERE id=?`,
			s.Name, s.Command, s.Description, s.Group, s.Scope, s.ScopeID, boolInt(s.AutoRun), boolInt(s.Shared), s.Sort, now, id)
		s.ID, s.UserID, s.UpdatedAt = id, old.UserID, now
		s.Mine = old.UserID == userID
		auditLog(r, userID, "snippet.updated", s.Name, snippetAuditDetails(s))
		notifySnippetsChanged(old.UserID, s.Shared || old.Shared)
		jsonOK(w, s)
	case http.MethodDelete:
		db.Exec(`DELETE FROM snippets WHERE id=?`, id)
		auditLog(r, userID, "snippet.deleted", old.Name, map[string]interface{}{"id": id, "shared": old.Shared})
		notifySnippetsChanged(old.UserID, old.Shared)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func snippetAuditDetails(s Snippet) map[string]interface{} {
	return map[string]interface{}{"id": s.ID, "scope": s.Scope, "scope_id": s.ScopeID, "auto_run": s.AutoRun, "shared": s.Shared,
		"command": truncateStr(s.Command, 300)}
}

// copySnippetsForConnection is used when a connection is duplicated: snippets bound to the
// original connection are copied to the new one.
func copySnippetsForConnection(fromConn, toConn, userID int) {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, s := range loadSnippets(`user_id=? AND scope='connection' AND scope_id=?`, userID, fromConn) {
		db.Exec(`INSERT INTO snippets (user_id, name, command, description, grp, scope, scope_id, auto_run, shared, sort, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,0,?,?,?)`,
			userID, s.Name, s.Command, s.Description, s.Group, "connection", toConn, boolInt(s.AutoRun), s.Sort, now, now)
	}
}

// deleteSnippetsForScope removes snippets bound to a deleted connection or folder.
func deleteSnippetsForScope(scope string, id int) {
	db.Exec(`DELETE FROM snippets WHERE scope=? AND scope_id=?`, scope, id)
}
