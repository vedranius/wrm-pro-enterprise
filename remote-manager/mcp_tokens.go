// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─── MCP: AI CONNECTIONS (TOKENS) ────────────────────
//
// An "AI connection" (table mcp_tokens) lets an MCP client — a desktop or web chat app —
// act on chosen connections of one user through the v12.0.0 permission engine. It binds:
//   the user, the connections, one permission mode, the scopes, an expiry (default
//   ai_mcp_token_hours, at most ai_mcp_max_token_hours), and how it was made: a personal
//   access token ("pat") created in Settings → AI connections, or an OAuth 2.1
//   authorization ("oauth") the user approved in WRM.
// Only the SHA-256 of the bearer token is stored; the token is shown once. Revoking or
// expiring an AI connection ends its AI sessions and its MCP sessions at once.
//
// User API (/api/mcp/…):
//   GET    config                 what the user may choose (connections, modes, scopes, hours)
//   GET    grants                 own AI connections
//   POST   grants                 create a personal access token {name, conn_ids, mode, scopes, hours, auto}
//   DELETE grants/{id}            revoke
//   POST   revoke-all             revoke every own AI connection
//   GET    active                 live AI connections and pending approvals (top bar)
//   POST   claude-desktop         the Claude Desktop extension wrm-pro.mcpb {token} (mcp_bundle.go)
//   POST   full-access            the user's own full access {on} (policy ai_full_access_mode = user)
//   GET|POST authorize/{id}        OAuth consent (mcp_oauth.go)
// Admin API (/api/admin/mcp/…):
//   GET grants, DELETE grants/{id}, POST revoke-all, GET clients, DELETE clients/{id}

const (
	mcpAllScopes        = "read_logs,run_readonly,run_with_approval,edit_file_with_approval,transfer,addons.manage,full_access"
	mcpMaxGrantsPerUser = 25
	mcpMaxConnsPerGrant = 50
	mcpPATPrefix        = "wrm_pat_"
	mcpOAuthPrefix      = "wrm_oat_"
)

var mcpScopeOrder = strings.Split(mcpAllScopes, ",")

// The redirect addresses of the common hosted clients plus loopback for desktop clients
// and the stdio bridge's OAuth helpers. Administrators can extend the list.
const mcpDefaultRedirectURIs = "https://claude.ai/api/mcp/auth_callback,https://claude.com/api/mcp/auth_callback," +
	"https://chatgpt.com/connector_platform_oauth_redirect,https://chat.openai.com/connector_platform_oauth_redirect," +
	"http://localhost:*,http://127.0.0.1:*,http://[::1]:*"

func initMCPSchema() {
	db.Exec(`ALTER TABLE users ADD COLUMN ai_full_access INTEGER NOT NULL DEFAULT 0`) // v12.6.3: the user's own full-access switch
	ensureTable("mcp_tokens", `CREATE TABLE IF NOT EXISTS mcp_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL DEFAULT 0,
		name TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT 'pat',
		token_hash TEXT NOT NULL DEFAULT '',
		hint TEXT NOT NULL DEFAULT '',
		client_id TEXT NOT NULL DEFAULT '',
		client_name TEXT NOT NULL DEFAULT '',
		conn_ids TEXT NOT NULL DEFAULT '',
		mode TEXT NOT NULL DEFAULT 'read_only',
		scopes TEXT NOT NULL DEFAULT '',
		auto_limits TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		created_ip TEXT NOT NULL DEFAULT '',
		expires_at TEXT NOT NULL DEFAULT '',
		last_used_at TEXT NOT NULL DEFAULT '',
		last_ip TEXT NOT NULL DEFAULT '',
		revoked_at TEXT NOT NULL DEFAULT '',
		revoked_by TEXT NOT NULL DEFAULT '',
		revoke_reason TEXT NOT NULL DEFAULT '')`,
		"id", "user_id", "name", "kind", "token_hash", "hint", "client_id", "client_name", "conn_ids", "mode", "scopes", "auto_limits",
		"created_at", "created_ip", "expires_at", "last_used_at", "last_ip", "revoked_at", "revoked_by", "revoke_reason")
	ensureTable("mcp_clients", `CREATE TABLE IF NOT EXISTS mcp_clients (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_id TEXT NOT NULL DEFAULT '',
		client_name TEXT NOT NULL DEFAULT '',
		redirect_uris TEXT NOT NULL DEFAULT '',
		secret_hash TEXT NOT NULL DEFAULT '',
		auth_method TEXT NOT NULL DEFAULT 'none',
		created_at TEXT NOT NULL DEFAULT '',
		created_ip TEXT NOT NULL DEFAULT '',
		last_used_at TEXT NOT NULL DEFAULT '')`,
		"id", "client_id", "client_name", "redirect_uris", "secret_hash", "auth_method", "created_at", "created_ip", "last_used_at")
	for _, q := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS ix_mcp_tokens_hash ON mcp_tokens(token_hash)`,
		`CREATE INDEX IF NOT EXISTS ix_mcp_tokens_user ON mcp_tokens(user_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ix_mcp_clients_id ON mcp_clients(client_id)`,
	} {
		db.Exec(q)
	}
	db.Exec(`ALTER TABLE ai_sessions ADD COLUMN mcp_token_id INTEGER DEFAULT NULL`)
}

// ─── policies ────────────────────────────────────────

// mcpUserAllowed reports whether a user may use (and create) AI connections.
func mcpUserAllowed(userID int) (bool, string) {
	if userID <= 0 {
		return false, "sign in first"
	}
	if settingBool("ai_kill_switch") {
		return false, "the AI assistant is stopped by the administrator (kill switch)"
	}
	if lockdownActive() {
		return false, "WRM is in lockdown: AI is stopped"
	}
	if !settingBool("ai_mcp_enabled") {
		return false, "AI connections (MCP) are turned off"
	}
	var disabled, blocked int
	if err := db.QueryRow(`SELECT disabled, COALESCE(ai_blocked,0) FROM users WHERE id=?`, userID).Scan(&disabled, &blocked); err != nil || disabled == 1 {
		return false, "the account is disabled"
	}
	switch v := getSetting("ai_mcp"); {
	case v == "off":
		return false, "AI connections are not allowed (policy ai_mcp)"
	case isAuditorUser(userID):
		return false, "security auditors cannot use AI connections"
	case !policyAllows(userID, v):
		return false, "AI connections are limited to " + policyLimitText(v)
	}
	if blocked == 1 {
		return false, "an administrator turned the AI assistant off for your account"
	}
	return true, ""
}

// mcpUserAllowedKind applies the policy of an AI connection's kind: the internal ones of
// OpenCode agents (kind opencode) follow the assistant and ai_opencode policies, not the
// policies for external MCP clients.
func mcpUserAllowedKind(userID int, kind string) (bool, string) {
	if kind == "opencode" {
		if ok, why := aiUserAllowed(userID); !ok {
			return false, why
		}
		if !policyWhoAllows(userID, "ai_opencode") {
			return false, "OpenCode agents are not allowed (policy ai_opencode)"
		}
		return true, ""
	}
	return mcpUserAllowed(userID)
}

func mcpAllowedFlag(userID int) bool {
	ok, _ := mcpUserAllowed(userID)
	return ok
}

// removeOne returns list without v, keeping the order.
func removeOne(list []string, v string) []string {
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func normalizeMCPScopes(v string) (string, error) {
	seen := map[string]bool{}
	for _, x := range strings.FieldsFunc(strings.ToLower(v), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		if !oneOf(x, mcpScopeOrder...) {
			return "", fmt.Errorf("unknown scope %q (use %s)", x, mcpAllScopes)
		}
		seen[x] = true
	}
	var out []string
	for _, s := range mcpScopeOrder {
		if seen[s] {
			out = append(out, s)
		}
	}
	return strings.Join(out, ","), nil
}

// mcpAllowedScopes: the policy's scopes; full_access only while the administrator allows it
// per AI connection (ai_full_access on, ai_full_access_mode = connection), whatever
// ai_mcp_scopes says.
func mcpAllowedScopes() []string {
	v, _ := normalizeMCPScopes(getSetting("ai_mcp_scopes"))
	var out []string
	for _, s := range splitPatterns(v) {
		if s != "full_access" {
			out = append(out, s)
		}
	}
	if settingBool("ai_full_access") && getSetting("ai_full_access_mode") == "connection" {
		out = append(out, "full_access")
	}
	return out
}

func mcpAllowedModes() []string {
	l, _ := normalizeModeList(getSetting("ai_mcp_modes"))
	return l
}

// validateRedirectPatterns checks the ai_mcp_redirect_uris policy.
func validateRedirectPatterns(v string) (string, error) {
	var out []string
	for _, p := range splitPatterns(v) {
		u, err := url.Parse(strings.ReplaceAll(p, "*", "0"))
		if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return "", fmt.Errorf("not a redirect URI pattern: %s", p)
		}
		if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
			return "", fmt.Errorf("redirect URIs must use https (http only for localhost): %s", p)
		}
		out = append(out, p)
	}
	return strings.Join(out, ","), nil
}

// ─── grants ──────────────────────────────────────────

type mcpGrant struct {
	ID         int64
	UserID     int
	Name       string
	Kind       string
	Hint       string
	ClientID   string
	ClientName string
	ConnIDs    []int
	Mode       string
	Scopes     []string
	Auto       *aiAutoLimits
	CreatedAt  string
	ExpiresAt  time.Time
	LastUsed   string
	LastIP     string
	RevokedAt  string
	RevokedBy  string
	Reason     string
}

const mcpGrantCols = `id, user_id, name, kind, hint, client_id, client_name, conn_ids, mode, scopes, auto_limits, created_at, expires_at, last_used_at, last_ip, revoked_at, revoked_by, revoke_reason`

func scanGrant(sc interface{ Scan(...interface{}) error }) (*mcpGrant, error) {
	g := &mcpGrant{}
	var conns, scopes, auto, exp string
	if err := sc.Scan(&g.ID, &g.UserID, &g.Name, &g.Kind, &g.Hint, &g.ClientID, &g.ClientName, &conns, &g.Mode, &scopes, &auto, &g.CreatedAt, &exp,
		&g.LastUsed, &g.LastIP, &g.RevokedAt, &g.RevokedBy, &g.Reason); err != nil {
		return nil, err
	}
	for _, x := range splitPatterns(conns) {
		if n, err := strconv.Atoi(x); err == nil && n > 0 {
			g.ConnIDs = append(g.ConnIDs, n)
		}
	}
	g.Scopes = splitPatterns(scopes)
	if g.Scopes == nil {
		g.Scopes = []string{}
	}
	if auto != "" {
		var a aiAutoLimits
		if json.Unmarshal([]byte(auto), &a) == nil {
			g.Auto = &a
		}
	}
	g.ExpiresAt, _ = time.Parse(time.RFC3339, exp)
	return g, nil
}

func loadGrant(id int64) (*mcpGrant, error) {
	return scanGrant(db.QueryRow(`SELECT `+mcpGrantCols+` FROM mcp_tokens WHERE id=?`, id))
}

func (g *mcpGrant) active(now time.Time) bool {
	return g.RevokedAt == "" && now.Before(g.ExpiresAt)
}

func (g *mcpGrant) state(now time.Time) string {
	switch {
	case g.RevokedAt != "":
		return "revoked"
	case !now.Before(g.ExpiresAt):
		return "expired"
	}
	return "active"
}

func (g *mcpGrant) hasConn(id int) bool {
	for _, c := range g.ConnIDs {
		if c == id {
			return true
		}
	}
	return false
}

// mcpGrantActive reports whether an AI connection may still be used.
func mcpGrantActive(id int64) bool {
	g, err := loadGrant(id)
	return err == nil && g.active(time.Now())
}

// view is an AI connection as the browser sees it (never the token).
func (g *mcpGrant) view(now time.Time) map[string]interface{} {
	conns := []map[string]interface{}{}
	for _, id := range g.ConnIDs {
		var name, host string
		if db.QueryRow(`SELECT name, host FROM connections WHERE id=? AND user_id=?`, id, g.UserID).Scan(&name, &host) == nil {
			conns = append(conns, map[string]interface{}{"id": id, "name": name, "host": host})
		}
	}
	sessions := []map[string]interface{}{}
	for _, s := range aiActiveSessions(func(s *aiSession) bool { return s.TokenID == g.ID }) {
		v := s.view()
		sessions = append(sessions, map[string]interface{}{"id": v["id"], "connection": v["connection"], "host": v["host"], "mode": v["mode"],
			"tool_calls": v["tool_calls"], "pending": v["pending"], "started_at": v["started_at"]})
	}
	clients := mcpConnsOf(g.ID)
	return map[string]interface{}{
		"id": g.ID, "user_id": g.UserID, "user": usernameOf(g.UserID), "name": g.Name, "kind": g.Kind, "hint": g.Hint, "client_id": g.ClientID,
		"client_name": g.ClientName, "connections": conns, "mode": g.Mode, "scopes": g.Scopes, "auto": aiAutoSummary(g.Auto),
		"created_at": g.CreatedAt, "expires_at": g.ExpiresAt.UTC().Format(time.RFC3339), "last_used_at": g.LastUsed, "last_ip": g.LastIP,
		"revoked_at": g.RevokedAt, "revoked_by": g.RevokedBy, "revoke_reason": g.Reason, "state": g.state(now),
		"sessions": sessions, "mcp_sessions": clients, "live": g.active(now) && (len(sessions) > 0 || clients > 0),
	}
}

type mcpGrantRequest struct {
	Name    string        `json:"name"`
	ConnIDs []int         `json:"conn_ids"`
	Mode    string        `json:"mode"`
	Scopes  []string      `json:"scopes"`
	Hours   int           `json:"hours"`
	Auto    *aiAutoLimits `json:"auto"`
	Confirm bool          `json:"confirm_auto"`
	editing bool
	kind    string // "" (pat, oauth) | opencode
}

// validate checks a request against the policies and the user's connections.
func (q *mcpGrantRequest) validate(userID int) error {
	if ok, why := mcpUserAllowedKind(userID, q.kind); !ok {
		return fmt.Errorf("%s", why)
	}
	q.Name = strings.TrimSpace(q.Name)
	if len(q.Name) > 100 {
		q.Name = truncateStr(q.Name, 100)
	}
	if len(q.ConnIDs) == 0 {
		return fmt.Errorf("choose at least one connection")
	}
	if len(q.ConnIDs) > mcpMaxConnsPerGrant {
		return fmt.Errorf("at most %d connections per AI connection", mcpMaxConnsPerGrant)
	}
	seen := map[int]bool{}
	var ids []int
	for _, id := range q.ConnIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		var proto string
		var temp sql.NullString
		if err := db.QueryRow(`SELECT protocol, temp_until FROM connections WHERE id=? AND user_id=?`, id, userID).Scan(&proto, &temp); err != nil {
			return fmt.Errorf("connection %d not found (only your own connections can be used)", id)
		}
		if !strings.EqualFold(proto, "SSH") {
			return fmt.Errorf("connection %d is not an SSH connection", id)
		}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	q.ConnIDs = ids
	modes := mcpAllowedModes()
	if q.Mode == "" {
		q.Mode = aiDefaultMode(modes)
	}
	if !oneOf(q.Mode, modes...) {
		return fmt.Errorf("the mode %q is not allowed for AI connections (allowed: %s)", q.Mode, strings.Join(modes, ", "))
	}
	if q.Mode == aiModeAuto {
		if !q.Confirm || q.Auto == nil {
			return fmt.Errorf("automatic mode needs an explicit opt-in with limits")
		}
		a := *q.Auto
		if err := normalizeAutoLimits(&a); err != nil {
			return err
		}
		q.Auto = &a
	} else {
		q.Auto = nil
	}
	sc, err := normalizeMCPScopes(strings.Join(q.Scopes, ","))
	if err != nil {
		return err
	}
	q.Scopes = splitPatterns(sc)
	if len(q.Scopes) == 0 {
		return fmt.Errorf("choose at least one scope")
	}
	allowed := mcpAllowedScopes()
	for _, s := range q.Scopes {
		if !oneOf(s, allowed...) {
			return fmt.Errorf("the scope %s is not allowed by the policy", s)
		}
	}
	if oneOf("addons.manage", q.Scopes...) && !isAdminUser(userID) {
		return fmt.Errorf("the addons.manage scope is for administrators only")
	}
	maxH := settingInt("ai_mcp_max_token_hours")
	if q.Hours <= 0 {
		q.Hours = min(settingInt("ai_mcp_token_hours"), maxH)
	}
	if q.Hours > maxH {
		return fmt.Errorf("the lifetime can be at most %d hours (policy)", maxH)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM mcp_tokens WHERE user_id=? AND revoked_at='' AND expires_at > ? AND kind<>'opencode'`, userID, nowRFC()).Scan(&n)
	if n >= mcpMaxGrantsPerUser && !q.editing && q.kind != "opencode" {
		return fmt.Errorf("you have %d active AI connections; revoke one first", n)
	}
	return nil
}

// mcpCreateGrant stores a validated AI connection and returns it with its bearer token.
func mcpCreateGrant(r *http.Request, userID int, q mcpGrantRequest, kind, clientID, clientName string) (*mcpGrant, string, error) {
	prefix := mcpPATPrefix
	if kind == "oauth" {
		prefix = mcpOAuthPrefix
	}
	tok := prefix + randomToken(32)
	auto := ""
	if q.Auto != nil {
		b, _ := json.Marshal(q.Auto)
		auto = string(b)
	}
	conns := make([]string, 0, len(q.ConnIDs))
	for _, id := range q.ConnIDs {
		conns = append(conns, strconv.Itoa(id))
	}
	name := q.Name
	if name == "" {
		name = nonEmpty(clientName, "Personal access token")
	}
	exp := time.Now().Add(time.Duration(q.Hours) * time.Hour).UTC().Format(time.RFC3339)
	ip := ""
	if r != nil {
		ip = clientIP(r)
	}
	res, err := db.Exec(`INSERT INTO mcp_tokens (user_id, name, kind, token_hash, hint, client_id, client_name, conn_ids, mode, scopes, auto_limits,
		created_at, created_ip, expires_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, userID, name, kind, sha256Hex(tok), tok[len(tok)-4:],
		clientID, truncateStr(clientName, 100), strings.Join(conns, ","), q.Mode, strings.Join(q.Scopes, ","), auto, nowRFC(), ip, exp)
	if err != nil {
		return nil, "", fmt.Errorf("could not store the AI connection")
	}
	id, _ := res.LastInsertId()
	g, err := loadGrant(id)
	if err != nil {
		return nil, "", err
	}
	d := map[string]interface{}{"mcp_grant": id, "kind": kind, "conn_ids": strings.Join(conns, ","), "mode": q.Mode, "scopes": strings.Join(q.Scopes, ","),
		"expires": exp, "hours": q.Hours}
	if clientName != "" {
		d["client"] = clientName
	}
	if q.Auto != nil {
		d["auto"] = aiAutoSummary(q.Auto)
	}
	auditLog(r, userID, "mcp.grant_created", name, d)
	mcpNotifyUser(userID)
	return g, tok, nil
}

// mcpLookupBearer finds the AI connection of a bearer token.
func mcpLookupBearer(tok string) (*mcpGrant, string) {
	tok = strings.TrimSpace(tok)
	if !strings.HasPrefix(tok, mcpPATPrefix) && !strings.HasPrefix(tok, mcpOAuthPrefix) || len(tok) > 200 {
		return nil, "invalid token"
	}
	g, err := scanGrant(db.QueryRow(`SELECT `+mcpGrantCols+` FROM mcp_tokens WHERE token_hash=?`, sha256Hex(tok)))
	if err != nil {
		return nil, "invalid token"
	}
	switch g.state(time.Now()) {
	case "revoked":
		return nil, "the token was revoked"
	case "expired":
		return nil, "the token has expired"
	}
	return g, ""
}

func (g *mcpGrant) touch(r *http.Request) {
	if g.LastUsed != "" {
		if t, err := time.Parse(time.RFC3339, g.LastUsed); err == nil && time.Since(t) < 30*time.Second {
			return
		}
	}
	db.Exec(`UPDATE mcp_tokens SET last_used_at=?, last_ip=? WHERE id=?`, nowRFC(), clientIP(r), g.ID)
}

// mcpRevoke revokes an AI connection, ends its AI sessions and drops its MCP sessions.
func mcpRevoke(r *http.Request, id int64, byID int, reason string) bool {
	g, err := loadGrant(id)
	if err != nil || g.RevokedAt != "" {
		return false
	}
	by := "WRM"
	if byID > 0 {
		by = usernameOf(byID)
	}
	db.Exec(`UPDATE mcp_tokens SET revoked_at=?, revoked_by=?, revoke_reason=? WHERE id=? AND revoked_at=''`, nowRFC(), by, truncateStr(reason, 200), id)
	n := aiKillWhere(func(s *aiSession) bool { return s.TokenID == id }, byID, "the AI connection was revoked: "+reason)
	mcpDropConns(func(c *mcpConn) bool { return c.grantID == id })
	actor := byID
	if actor <= 0 {
		actor = g.UserID
	}
	auditLog(r, actor, "mcp.grant_revoked", g.Name, map[string]interface{}{"mcp_grant": id, "owner": usernameOf(g.UserID), "by": by, "reason": reason, "ended": n})
	mcpNotifyUser(g.UserID)
	return true
}

func mcpRevokeWhere(r *http.Request, where string, args []interface{}, byID int, reason string) int {
	rows, err := db.Query(`SELECT id FROM mcp_tokens WHERE revoked_at='' AND `+where, args...)
	if err != nil {
		return 0
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		if mcpRevoke(r, id, byID, reason) {
			n++
		}
	}
	return n
}

func mcpNotifyUser(userID int) {
	hub.sendTo(userID, jsonMarshal(map[string]interface{}{"type": "mcp_changed"}))
}

// mcpShutdown ends every MCP session (MCP turned off, policy changes); the internal ones of
// OpenCode agents follow their own policy.
func mcpShutdown(byID int, reason string, pred func(userID int) bool) int {
	n := aiKillWhere(func(s *aiSession) bool {
		return s.Transport == "mcp" && !ocIsChild(s) && (pred == nil || pred(s.UserID))
	}, byID, reason)
	mcpDropConns(func(c *mcpConn) bool { return !ocGrantLive(c.grantID) && (pred == nil || pred(c.userID)) })
	return n
}

// runMCPJanitor ends AI sessions of expired AI connections, drops idle MCP sessions and
// expired authorization codes, and deletes old AI connections.
func mcpJanitorTick(now time.Time) {
	for _, s := range aiActiveSessions(func(s *aiSession) bool { return s.Transport == "mcp" }) {
		if ok, why := s.userAllowed(); !ok {
			s.Kill(0, why)
		}
	}
	mcpDropConns(func(c *mcpConn) bool {
		c.mu.Lock()
		idle := now.Sub(c.lastSeen) > mcpConnIdle
		c.mu.Unlock()
		return idle || !mcpGrantActive(c.grantID)
	})
	mcpCodes.Lock()
	for k, c := range mcpCodes.m {
		if now.After(c.expires) {
			delete(mcpCodes.m, k)
		}
	}
	mcpCodes.Unlock()
	mcpPending.Lock()
	for k, p := range mcpPending.m {
		if now.After(p.expires) {
			delete(mcpPending.m, k)
		}
	}
	mcpPending.Unlock()
	cut := now.AddDate(0, 0, -settingInt("ai_transcript_retention_days")).UTC().Format(time.RFC3339)
	db.Exec(`DELETE FROM mcp_tokens WHERE expires_at < ? OR (revoked_at != '' AND revoked_at < ?)`, cut, cut)
	db.Exec(`DELETE FROM mcp_clients WHERE last_used_at = '' AND created_at < ?`, now.Add(-24*time.Hour).UTC().Format(time.RFC3339))
}

func runMCPJanitor() {
	for {
		time.Sleep(time.Minute)
		mcpJanitorTick(time.Now())
	}
}

// ─── user API ────────────────────────────────────────

func apiMCPHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/mcp"), "/")
	parts := strings.Split(rest, "/")
	switch parts[0] {
	case "config":
		mcpConfigHandler(w, r, userID)
	case "grants":
		if len(parts) == 1 {
			switch r.Method {
			case http.MethodGet:
				jsonOK(w, mcpListGrants(`user_id=?`, []interface{}{userID}, 100))
			case http.MethodPost:
				mcpCreatePATHandler(w, r, userID)
			default:
				jsonError(w, "Method not allowed", 405)
			}
			return
		}
		id, _ := strconv.ParseInt(parts[1], 10, 64)
		if !oneOf(r.Method, http.MethodDelete, http.MethodPut) || len(parts) != 2 {
			jsonError(w, "Method not allowed", 405)
			return
		}
		var owner int
		if db.QueryRow(`SELECT user_id FROM mcp_tokens WHERE id=?`, id).Scan(&owner) != nil || owner != userID {
			jsonError(w, "AI connection not found", 404)
			return
		}
		if r.Method == http.MethodPut {
			mcpEditGrant(w, r, id, userID)
			return
		}
		mcpRevoke(r, id, userID, "revoked by the user")
		jsonOK(w, map[string]bool{"ok": true})
	case "revoke-all":
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", 405)
			return
		}
		n := mcpRevokeWhere(r, `user_id=?`, []interface{}{userID}, userID, "revoked by the user (all)")
		jsonOK(w, map[string]int{"revoked": n})
	case "active":
		mcpActiveHandler(w, userID)
	case "claude-desktop":
		mcpClaudeDesktopHandler(w, r, userID)
	case "setup-all":
		mcpSetupAllHandler(w, r, userID)
	case "full-access":
		mcpFullAccessHandler(w, r, userID)
	case "authorize":
		if len(parts) != 2 {
			jsonError(w, "Not found", 404)
			return
		}
		mcpConsentHandler(w, r, userID, parts[1])
	default:
		jsonError(w, "Not found", 404)
	}
}

func mcpConfigHandler(w http.ResponseWriter, r *http.Request, userID int) {
	ok, why := mcpUserAllowed(userID)
	conns := []map[string]interface{}{}
	rows, err := db.Query(`SELECT id, name, host, tags FROM connections WHERE user_id=? AND protocol='SSH' AND (temp_until IS NULL OR temp_until='') ORDER BY name`, userID)
	if err == nil {
		for rows.Next() {
			var id int
			var name, host, tags string
			if rows.Scan(&id, &name, &host, &tags) == nil {
				conns = append(conns, map[string]interface{}{"id": id, "name": name, "host": host, "tags": parseTags(tags), "modes": aiModesFor("mcp", id)})
			}
		}
		rows.Close()
	}
	base := mcpBaseURL(r)
	scopes := mcpAllowedScopes()
	if !isAdminUser(userID) {
		// addons.manage changes WRM itself; only an administrator can grant it.
		scopes = removeOne(scopes, "addons.manage")
	}
	jsonOK(w, map[string]interface{}{
		"enabled": ok, "reason": why, "connections": conns, "modes": mcpAllowedModes(), "scopes": scopes, "all_scopes": mcpScopeOrder,
		"default_hours": min(settingInt("ai_mcp_token_hours"), settingInt("ai_mcp_max_token_hours")), "max_hours": settingInt("ai_mcp_max_token_hours"),
		"url": base + "/mcp", "base_url": base, "instructions": mcpInstructions, "description": mcpServerDescription, "version": AppVersion, "oauth": settingBool("ai_mcp_oauth"), "elicitation": settingBool("ai_mcp_elicitation"),
		"auto_max_minutes": settingInt("ai_auto_max_minutes"), "auto_max_actions": settingInt("ai_auto_max_actions"), "auto_until_grant": settingBool("ai_auto_until_grant"),
		"full_access": map[string]interface{}{"on": settingBool("ai_full_access"), "mode": getSetting("ai_full_access_mode"), "mine": aiUserFullAccess(userID)},
	})
}

func mcpCreatePATHandler(w http.ResponseWriter, r *http.Request, userID int) {
	var q mcpGrantRequest
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	if err := q.validate(userID); err != nil {
		auditLog(r, userID, "mcp.grant_refused", "", map[string]interface{}{"reason": err.Error()})
		jsonError(w, err.Error(), 400)
		return
	}
	g, tok, err := mcpCreateGrant(r, userID, q, "pat", "", "")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]interface{}{"grant": g.view(time.Now()), "token": tok, "url": mcpBaseURL(r) + "/mcp"})
}

func mcpListGrants(where string, args []interface{}, limit int) []map[string]interface{} {
	out := []map[string]interface{}{}
	cut := time.Now().AddDate(0, 0, -7).UTC().Format(time.RFC3339)
	rows, err := db.Query(`SELECT `+mcpGrantCols+` FROM mcp_tokens WHERE `+where+` AND ((revoked_at='' AND expires_at > ?) OR created_at > ?) ORDER BY id DESC LIMIT ?`,
		append(args, cut, cut, limit)...)
	if err != nil {
		return out
	}
	var gs []*mcpGrant
	for rows.Next() {
		if g, err := scanGrant(rows); err == nil {
			gs = append(gs, g)
		}
	}
	rows.Close()
	now := time.Now()
	for _, g := range gs {
		out = append(out, g.view(now))
	}
	return out
}

// mcpActiveHandler feeds the "AI connected" indicator: live AI connections and the
// pending approvals of their sessions.
func mcpActiveHandler(w http.ResponseWriter, userID int) {
	live := []map[string]interface{}{}
	pending := []map[string]interface{}{}
	if ok, _ := mcpUserAllowed(userID); ok {
		for _, g := range mcpListGrants(`user_id=? AND revoked_at='' AND expires_at > ?`, []interface{}{userID, nowRFC()}, 100) {
			if g["live"] == true {
				live = append(live, g)
			}
		}
	}
	for _, s := range aiActiveSessions(func(s *aiSession) bool { return s.UserID == userID }) {
		v := s.view()
		for _, a := range v["pending"].([]*aiApproval) {
			client := s.Model
			if s.Transport == "" || s.Transport == "panel" {
				client = "WRM · " + nonEmpty(s.Model, "assistant")
			}
			pending = append(pending, map[string]interface{}{"session": s.ID, "connection": s.ConnName, "host": s.Host, "client": client, "transport": s.Transport, "approval": a})
		}
	}
	pending = append(pending, aiStoredLaterFor(userID)...)
	jsonOK(w, map[string]interface{}{"live": live, "pending": pending})
}

// ─── admin API ───────────────────────────────────────

func apiAdminMCPHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/mcp"), "/")
	parts := strings.Split(rest, "/")
	switch parts[0] {
	case "grants":
		if len(parts) == 1 && r.Method == http.MethodGet {
			jsonOK(w, mcpListGrants(`1=1`, nil, 500))
			return
		}
		if len(parts) == 2 && r.Method == http.MethodDelete {
			id, _ := strconv.ParseInt(parts[1], 10, 64)
			if !mcpRevoke(r, id, adminID, "revoked by an administrator") {
				jsonError(w, "AI connection not found or already revoked", 404)
				return
			}
			jsonOK(w, map[string]bool{"ok": true})
			return
		}
		jsonError(w, "Method not allowed", 405)
	case "revoke-all":
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", 405)
			return
		}
		n := mcpRevokeWhere(r, `1=1`, nil, adminID, "revoked by an administrator (all)")
		auditLog(r, adminID, "mcp.revoke_all", "", map[string]interface{}{"revoked": n})
		jsonOK(w, map[string]int{"revoked": n})
	case "clients":
		if len(parts) == 1 && r.Method == http.MethodGet {
			out := []map[string]interface{}{}
			rows, err := db.Query(`SELECT c.id, c.client_id, c.client_name, c.redirect_uris, c.auth_method, c.created_at, c.created_ip, c.last_used_at,
				(SELECT COUNT(*) FROM mcp_tokens t WHERE t.client_id=c.client_id AND t.revoked_at='' AND t.expires_at > ?) FROM mcp_clients c ORDER BY c.id DESC LIMIT 500`, nowRFC())
			if err == nil {
				for rows.Next() {
					var id, n int
					var cid, name, uris, method, created, ip, used string
					if rows.Scan(&id, &cid, &name, &uris, &method, &created, &ip, &used, &n) == nil {
						var list []string
						json.Unmarshal([]byte(uris), &list)
						out = append(out, map[string]interface{}{"id": id, "client_id": cid, "client_name": name, "redirect_uris": list, "auth_method": method,
							"created_at": created, "created_ip": ip, "last_used_at": used, "active": n})
					}
				}
				rows.Close()
			}
			jsonOK(w, out)
			return
		}
		if len(parts) == 2 && r.Method == http.MethodDelete {
			id, _ := strconv.Atoi(parts[1])
			var cid, name string
			if db.QueryRow(`SELECT client_id, client_name FROM mcp_clients WHERE id=?`, id).Scan(&cid, &name) != nil {
				jsonError(w, "Client not found", 404)
				return
			}
			n := mcpRevokeWhere(r, `client_id=?`, []interface{}{cid}, adminID, "the OAuth client was removed")
			db.Exec(`DELETE FROM mcp_clients WHERE id=?`, id)
			auditLog(r, adminID, "mcp.client_removed", name, map[string]interface{}{"client_id": cid, "revoked": n})
			jsonOK(w, map[string]bool{"ok": true})
			return
		}
		jsonError(w, "Method not allowed", 405)
	default:
		jsonError(w, "Not found", 404)
	}
}

// mcpFullAccessHandler: POST /api/mcp/full-access {on} turns the user's own full access on or
// off, when the administrator lets users decide (ai_full_access on, mode user). Open AI
// sessions follow it at their next tool call.
func mcpFullAccessHandler(w http.ResponseWriter, r *http.Request, userID int) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var in struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	if in.On && (!settingBool("ai_full_access") || getSetting("ai_full_access_mode") != "user") {
		jsonError(w, "The administrator does not let users turn on full access", 403)
		return
	}
	db.Exec(`UPDATE users SET ai_full_access=? WHERE id=?`, boolInt(in.On), userID)
	auditLog(r, userID, "mcp.full_access", "", map[string]interface{}{"on": in.On})
	jsonOK(w, map[string]bool{"on": in.On})
}
