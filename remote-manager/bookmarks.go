// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ─── FOLDER BOOKMARKS ────────────────────────────────
//
// A bookmark is a named remote directory: the file manager opens it with one click and a
// terminal gets `cd -- '<path>'` typed in. Scopes:
//   global     – offered on every SSH / SFTP / FTP connection of the user
//   folder     – on the connections of one WRM folder
//   tag        – on the connections with a tag
//   connection – on one connection
//
// Paths may use variables, filled in on the server for the connection they are used on:
//   {host} {name} {user} $USER ${USER}   (~ is left for the remote side: the home directory)
//
// A bookmark marked as the start directory opens the file manager there and is typed as a
// `cd` when a terminal connects (before the run-on-connect snippets). The most specific
// one wins: connection, then tag, then folder, then global.
//
// Global bookmarks can be shared: people who use the owner's connections through a share
// see them (read-only).

const maxBookmarksPerUser = 1000

type Bookmark struct {
	ID        int    `json:"id"`
	UserID    int    `json:"-"`
	Owner     string `json:"owner,omitempty"`
	Mine      bool   `json:"mine"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Scope     string `json:"scope"`    // global | folder | tag | connection
	ScopeID   int    `json:"scope_id"` // folder or connection id
	Tag       string `json:"tag"`      // scope "tag"
	Color     string `json:"color"`
	Note      string `json:"note"`
	StartDir  bool   `json:"start_dir"`
	Shared    bool   `json:"shared"`
	Sort      int    `json:"sort"`
	UpdatedAt string `json:"updated_at"`
	// Filled in for one connection (GET /api/bookmarks?conn=…).
	Resolved string `json:"resolved,omitempty"`
	Cd       string `json:"cd,omitempty"`
}

const bookmarkCols = `id, user_id, name, path, scope, scope_id, tag, color, note, start_dir, shared, sort, updated_at`

func scanBookmark(sc interface{ Scan(...interface{}) error }) (Bookmark, error) {
	var b Bookmark
	var start, shared int
	err := sc.Scan(&b.ID, &b.UserID, &b.Name, &b.Path, &b.Scope, &b.ScopeID, &b.Tag, &b.Color, &b.Note, &start, &shared, &b.Sort, &b.UpdatedAt)
	b.StartDir, b.Shared = start == 1, shared == 1
	return b, err
}

func loadBookmarks(where string, args ...interface{}) []Bookmark {
	out := []Bookmark{}
	rows, err := db.Query(`SELECT `+bookmarkCols+` FROM bookmarks WHERE `+where+` ORDER BY sort, name COLLATE NOCASE, id`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		if b, err := scanBookmark(rows); err == nil {
			out = append(out, b)
		}
	}
	return out
}

var bookmarkColorRe = regexp.MustCompile(`^(#[0-9a-fA-F]{6})?$`)

func validateBookmark(b *Bookmark, userID int) error {
	b.Name = strings.TrimSpace(b.Name)
	b.Path = strings.TrimSpace(b.Path)
	b.Note = strings.TrimSpace(b.Note)
	b.Color = strings.TrimSpace(b.Color)
	b.Scope = strings.ToLower(strings.TrimSpace(b.Scope))
	if b.Path == "" || len(b.Path) > 1024 {
		return fmt.Errorf("path is required (at most 1024 characters)")
	}
	for _, r := range b.Path {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("the path must not contain control characters")
		}
	}
	if b.Name == "" {
		b.Name = bookmarkDefaultName(b.Path)
	}
	if len(b.Name) > 120 || len(b.Note) > 1000 {
		return fmt.Errorf("name (120) or note (1000 characters) is too long")
	}
	if !bookmarkColorRe.MatchString(b.Color) {
		return fmt.Errorf("color must be #rrggbb")
	}
	tag := b.Tag
	b.Tag = ""
	switch b.Scope {
	case "", "global", "all":
		b.Scope, b.ScopeID = "global", 0
	case "folder":
		if b.ScopeID <= 0 || !userOwnsFolder(b.ScopeID, userID) {
			return fmt.Errorf("folder not found")
		}
	case "connection":
		if b.ScopeID <= 0 || !userOwnsConnection(b.ScopeID, userID) {
			return fmt.Errorf("connection not found")
		}
	case "tag":
		if b.Tag, b.ScopeID = normalizeTag(tag), 0; b.Tag == "" {
			return fmt.Errorf("tag is required")
		}
	default:
		return fmt.Errorf("unknown scope %q", b.Scope)
	}
	if b.Shared && b.Scope != "global" {
		return fmt.Errorf("only global bookmarks can be shared")
	}
	return nil
}

func bookmarkDefaultName(p string) string {
	s := strings.TrimRight(p, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	if s == "" {
		s = p
	}
	return truncateStr(s, 120)
}

// bookmarkVarRe matches {host} {name} {user} $USER ${USER}.
var bookmarkVarRe = regexp.MustCompile(`\{(host|name|user)\}|\$\{USER\}|\$USER\b`)

// expandBookmarkPath fills in the variables of a bookmark path for connection c. $USER
// stays as it is when the connection has no user name.
func expandBookmarkPath(p string, c Connection) string {
	host := c.Host
	if h, _, err := net.SplitHostPort(c.Host); err == nil {
		host = h
	}
	return bookmarkVarRe.ReplaceAllStringFunc(p, func(m string) string {
		switch m {
		case "{host}":
			return host
		case "{name}":
			return c.Name
		}
		if c.Username == "" {
			return m
		}
		return c.Username
	})
}

// bookmarkCd is the command typed into a terminal: cd -- '<path>'. A leading ~ stays
// outside the quotes so the remote shell expands it to the home directory.
func bookmarkCd(p string) string {
	switch {
	case p == "~":
		return "cd -- ~"
	case strings.HasPrefix(p, "~/"):
		rest := strings.TrimLeft(p[2:], "/")
		if rest == "" {
			return "cd -- ~/"
		}
		return "cd -- ~/" + shellQuote(rest)
	}
	return "cd -- " + shellQuote(p)
}

// bookmarkApplies reports whether b is offered on connection c (tags: the tags of c).
func bookmarkApplies(b Bookmark, c Connection, tags []string) bool {
	switch b.Scope {
	case "global":
		return true
	case "folder":
		return c.FolderID != nil && *c.FolderID == b.ScopeID
	case "connection":
		return c.ID == b.ScopeID
	case "tag":
		for _, t := range tags {
			if strings.EqualFold(t, b.Tag) {
				return true
			}
		}
	}
	return false
}

func connectionTags(connID int) []string {
	var s string
	db.QueryRow(`SELECT COALESCE(tags,'') FROM connections WHERE id=?`, connID).Scan(&s)
	return parseTags(s)
}

var bookmarkScopeRank = map[string]int{"global": 0, "folder": 1, "tag": 2, "connection": 3}

// bookmarksForConnection returns the bookmarks of the owner of c that apply to c, with
// the path resolved. sharedOnly limits them to shared bookmarks (access through a share).
func bookmarksForConnection(c Connection, sharedOnly bool) []Bookmark {
	where := `user_id=?`
	if sharedOnly {
		where += ` AND shared=1 AND scope='global'`
	}
	tags := connectionTags(c.ID)
	out := []Bookmark{}
	for _, b := range loadBookmarks(where, c.UserID) {
		if bookmarkApplies(b, c, tags) {
			b.Resolved = expandBookmarkPath(b.Path, c)
			b.Cd = bookmarkCd(b.Resolved)
			out = append(out, b)
		}
	}
	return out
}

// startBookmark returns the start directory bookmark of c (the most specific one).
func startBookmark(c Connection) (Bookmark, bool) {
	var best Bookmark
	found := false
	tags := connectionTags(c.ID)
	for _, b := range loadBookmarks(`user_id=? AND start_dir=1`, c.UserID) {
		if !bookmarkApplies(b, c, tags) {
			continue
		}
		if !found || bookmarkScopeRank[b.Scope] > bookmarkScopeRank[best.Scope] {
			best, found = b, true
		}
	}
	if found {
		best.Resolved = expandBookmarkPath(best.Path, c)
		best.Cd = bookmarkCd(best.Resolved)
	}
	return best, found
}

func notifyBookmarksChanged(userID int) {
	hub.sendTo(userID, []byte(`{"type":"bookmarks_changed"}`))
}

func insertBookmark(userID int, b Bookmark, now string) (int, error) {
	res, err := db.Exec(`INSERT INTO bookmarks (user_id, name, path, scope, scope_id, tag, color, note, start_dir, shared, sort, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		userID, b.Name, b.Path, b.Scope, b.ScopeID, b.Tag, b.Color, b.Note, boolInt(b.StartDir), boolInt(b.Shared), b.Sort, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func bookmarkCount(userID int) int {
	var n int
	db.QueryRow(`SELECT COUNT(1) FROM bookmarks WHERE user_id=?`, userID).Scan(&n)
	return n
}

func nextBookmarkSort(userID int) int {
	var n int
	db.QueryRow(`SELECT COALESCE(MAX(sort),0)+1 FROM bookmarks WHERE user_id=?`, userID).Scan(&n)
	return n
}

// ─── API ─────────────────────────────────────────────

// GET    /api/bookmarks                 my bookmarks
// GET    /api/bookmarks?conn=ID         bookmarks for one connection (resolved; share_token for shares)
// POST   /api/bookmarks                 create
// POST   /api/bookmarks/reorder         {"ids":[…]}
// POST   /api/bookmarks/resolve         {"id":…, "conns":[…]} → the path and cd per connection
// POST   /api/bookmarks/import-winscp   WinSCP.ini as the body → global bookmarks
// PUT    /api/bookmarks/{id}            update (rename, move between scopes, …)
// DELETE /api/bookmarks/{id}
func apiBookmarksHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/bookmarks"), "/")
	// Connection lists and resolving also work for share participants (share_token).
	if rest == "" && r.Method == http.MethodGet && r.URL.Query().Get("conn") != "" {
		bookmarksForConnHandler(w, r)
		return
	}
	if rest == "resolve" && r.Method == http.MethodPost {
		resolveBookmarkHandler(w, r)
		return
	}
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	switch rest {
	case "":
		switch r.Method {
		case http.MethodGet:
			jsonOK(w, loadBookmarks(`user_id=?`, userID))
		case http.MethodPost:
			var b Bookmark
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				jsonError(w, "Invalid request", 400)
				return
			}
			if err := validateBookmark(&b, userID); err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
			if bookmarkCount(userID) >= maxBookmarksPerUser {
				jsonError(w, fmt.Sprintf("at most %d bookmarks per user", maxBookmarksPerUser), 400)
				return
			}
			now := time.Now().UTC().Format(time.RFC3339)
			if b.Sort == 0 {
				b.Sort = nextBookmarkSort(userID)
			}
			id, err := insertBookmark(userID, b, now)
			if err != nil {
				jsonError(w, "Server error", 500)
				return
			}
			b.ID, b.Mine, b.UpdatedAt = id, true, now
			auditLog(r, userID, "bookmark.created", b.Name, bookmarkAuditDetails(b))
			notifyBookmarksChanged(userID)
			jsonOK(w, b)
		default:
			jsonError(w, "Method not allowed", 405)
		}
		return
	case "reorder":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req struct {
			IDs []int `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IDs) > maxBookmarksPerUser {
			jsonError(w, "Invalid request", 400)
			return
		}
		for i, id := range req.IDs {
			db.Exec(`UPDATE bookmarks SET sort=? WHERE id=? AND user_id=?`, i+1, id, userID)
		}
		notifyBookmarksChanged(userID)
		jsonOK(w, map[string]bool{"ok": true})
		return
	case "import-winscp":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		importWinSCPHandler(w, r, userID)
		return
	}
	id, err := strconv.Atoi(rest)
	if err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	old, err := scanBookmark(db.QueryRow(`SELECT `+bookmarkCols+` FROM bookmarks WHERE id=?`, id))
	if err != nil || old.UserID != userID {
		jsonError(w, "Not found", 404)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var b Bookmark
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			jsonError(w, "Invalid request", 400)
			return
		}
		if err := validateBookmark(&b, userID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		db.Exec(`UPDATE bookmarks SET name=?, path=?, scope=?, scope_id=?, tag=?, color=?, note=?, start_dir=?, shared=?, sort=?, updated_at=? WHERE id=?`,
			b.Name, b.Path, b.Scope, b.ScopeID, b.Tag, b.Color, b.Note, boolInt(b.StartDir), boolInt(b.Shared), b.Sort, now, id)
		b.ID, b.Mine, b.UpdatedAt = id, true, now
		auditLog(r, userID, "bookmark.updated", b.Name, bookmarkAuditDetails(b))
		notifyBookmarksChanged(userID)
		jsonOK(w, b)
	case http.MethodDelete:
		db.Exec(`DELETE FROM bookmarks WHERE id=?`, id)
		auditLog(r, userID, "bookmark.deleted", old.Name, map[string]interface{}{"id": id, "scope": old.Scope})
		notifyBookmarksChanged(userID)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func bookmarkAuditDetails(b Bookmark) map[string]interface{} {
	return map[string]interface{}{"id": b.ID, "path": truncateStr(b.Path, 300), "scope": b.Scope, "scope_id": b.ScopeID, "tag": b.Tag,
		"start_dir": b.StartDir, "shared": b.Shared}
}

// bookmarksForConnHandler: GET /api/bookmarks?conn=ID[&share_token=…]. The owner gets
// all of their bookmarks that apply; share participants get the owner's shared ones.
func bookmarksForConnHandler(w http.ResponseWriter, r *http.Request) {
	connID, _ := strconv.Atoi(r.URL.Query().Get("conn"))
	acc, code, msg := authorizeConnection(r, connID, PermFilesRead)
	if acc == nil {
		jsonError(w, msg, code)
		return
	}
	c, err := loadConnectionRaw(connID)
	if err != nil {
		jsonError(w, "Connection not found", 404)
		return
	}
	list := bookmarksForConnection(c, acc.Share != nil)
	owner := ""
	if acc.Share != nil {
		owner = usernameOf(c.UserID)
	}
	for i := range list {
		list[i].Mine = acc.Share == nil
		list[i].Owner = owner
	}
	resp := map[string]interface{}{"bookmarks": list, "start": nil}
	if sb, ok := startBookmark(c); ok {
		resp["start"] = map[string]interface{}{"id": sb.ID, "name": sb.Name, "path": sb.Resolved}
	}
	jsonOK(w, resp)
}

// resolveBookmarkHandler: POST /api/bookmarks/resolve {"id": bookmark, "conns": [ids]}
// fills in the path of one bookmark for several connections (a broadcast group).
func resolveBookmarkHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    int   `json:"id"`
		Conns []int `json:"conns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Conns) == 0 || len(req.Conns) > 200 {
		jsonError(w, "Invalid request", 400)
		return
	}
	b, err := scanBookmark(db.QueryRow(`SELECT `+bookmarkCols+` FROM bookmarks WHERE id=?`, req.ID))
	if err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	out := map[string]map[string]string{}
	for _, id := range req.Conns {
		acc, _, _ := authorizeConnection(r, id, PermFilesRead)
		if acc == nil {
			continue
		}
		c, err := loadConnectionRaw(id)
		if err != nil {
			continue
		}
		// The bookmark must belong to whoever owns the connection: the user's own, or a
		// shared one of the owner for share participants.
		if b.UserID != c.UserID || (acc.Share != nil && !(b.Shared && b.Scope == "global")) {
			continue
		}
		p := expandBookmarkPath(b.Path, c)
		out[strconv.Itoa(id)] = map[string]string{"path": p, "cd": bookmarkCd(p)}
	}
	if len(out) == 0 {
		jsonError(w, "Not found", 404)
		return
	}
	jsonOK(w, out)
}

// ─── WinSCP import ───────────────────────────────────

// parseWinSCPBookmarks reads the remote directory bookmarks of a WinSCP.ini: the values
// of the [Configuration\Bookmarks\Remote…] sections (one subsection per site). Names and
// values are %XX-escaped in WinSCP.ini.
func parseWinSCPBookmarks(text string) []Bookmark {
	out := []Bookmark{}
	seen := map[string]bool{}
	section, site := "", ""
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF"))
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if line[0] == '[' && strings.HasSuffix(line, "]") {
			section, site = "", ""
			name := line[1 : len(line)-1]
			parts := strings.Split(name, `\`)
			if len(parts) >= 3 && strings.EqualFold(parts[0], "Configuration") && strings.EqualFold(parts[1], "Bookmarks") && strings.EqualFold(parts[2], "Remote") {
				section = name
				if len(parts) > 3 {
					site = winscpUnescape(strings.Join(parts[3:], `\`))
				}
			}
			continue
		}
		if section == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = winscpUnescape(strings.TrimSpace(k)), winscpUnescape(strings.TrimSpace(v))
		if !strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "~") {
			continue // not a remote directory (options, shortcuts, …)
		}
		name := k
		if _, err := strconv.Atoi(k); err == nil || name == "" {
			name = bookmarkDefaultName(v)
		}
		if seen[name+"\x00"+v] {
			continue
		}
		seen[name+"\x00"+v] = true
		note := "WinSCP"
		if site != "" {
			note = "WinSCP: " + site
		}
		out = append(out, Bookmark{Name: name, Path: v, Scope: "global", Note: note})
	}
	return out
}

func winscpUnescape(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}

func importWinSCPHandler(w http.ResponseWriter, r *http.Request, userID int) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var sb strings.Builder
	if _, err := bufio.NewReader(r.Body).WriteTo(&sb); err != nil {
		jsonError(w, "The file is too large (4 MB at most)", 400)
		return
	}
	found := parseWinSCPBookmarks(sb.String())
	existing := map[string]bool{}
	for _, b := range loadBookmarks(`user_id=?`, userID) {
		existing[b.Name+"\x00"+b.Path] = true
	}
	now := time.Now().UTC().Format(time.RFC3339)
	imported, skipped := 0, 0
	sort := nextBookmarkSort(userID)
	n := bookmarkCount(userID)
	for _, b := range found {
		if existing[b.Name+"\x00"+b.Path] || n >= maxBookmarksPerUser || validateBookmark(&b, userID) != nil {
			skipped++
			continue
		}
		b.Sort = sort
		if _, err := insertBookmark(userID, b, now); err == nil {
			imported++
			sort++
			n++
			existing[b.Name+"\x00"+b.Path] = true
		}
	}
	if imported > 0 {
		notifyBookmarksChanged(userID)
	}
	auditLog(r, userID, "bookmark.imported", "WinSCP.ini", map[string]int{"imported": imported, "skipped": skipped})
	jsonOK(w, map[string]int{"found": len(found), "imported": imported, "skipped": skipped})
}

// ─── export / import, duplicate, delete ──────────────

// importBookmarks adds bookmarks of a configuration export. folderIDs / connIDs map the
// ids of the export to the new rows; bookmarks whose folder or connection was not
// imported are skipped, as are duplicates (same name, path and scope).
func importBookmarks(userID int, list []Bookmark, folderIDs, connIDs map[int]int) int {
	existing := map[string]bool{}
	key := func(b Bookmark) string { return b.Name + "\x00" + b.Path + "\x00" + b.Scope + "\x00" + b.Tag }
	for _, b := range loadBookmarks(`user_id=?`, userID) {
		existing[key(b)] = true
	}
	now := time.Now().UTC().Format(time.RFC3339)
	n, imported := bookmarkCount(userID), 0
	for _, b := range list {
		switch b.Scope {
		case "folder":
			b.ScopeID = folderIDs[b.ScopeID]
		case "connection":
			b.ScopeID = connIDs[b.ScopeID]
		}
		if validateBookmark(&b, userID) != nil || existing[key(b)] || n >= maxBookmarksPerUser {
			continue
		}
		if _, err := insertBookmark(userID, b, now); err == nil {
			existing[key(b)] = true
			imported++
			n++
		}
	}
	if imported > 0 {
		notifyBookmarksChanged(userID)
	}
	return imported
}

// copyBookmarksForConnection is used when a connection is duplicated.
func copyBookmarksForConnection(fromConn, toConn, userID int) {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, b := range loadBookmarks(`user_id=? AND scope='connection' AND scope_id=?`, userID, fromConn) {
		b.ScopeID = toConn
		insertBookmark(userID, b, now)
	}
}

// deleteBookmarksForScope removes bookmarks bound to a deleted connection or folder.
func deleteBookmarksForScope(scope string, id int) {
	db.Exec(`DELETE FROM bookmarks WHERE scope=? AND scope_id=?`, scope, id)
}
