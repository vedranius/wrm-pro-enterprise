// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─── ROLES ───────────────────────────────────────────
//
// Every account has one built-in role:
//   admin    – everything (users.is_admin = 1, as before)
//   auditor  – Security auditor: reads the audit log, terminal sessions and recordings, file
//              transfers and the security posture; cannot open connections, use the AI
//              assistant or change settings
//   operator – a user who passes the policies set to "operators"
//   user     – as before (the default)
// is_admin stays the source of truth for administrators, so a previous binary still sees the
// same administrators; users.role holds the other roles ('' = user).

const (
	userRoleAdmin    = "admin"
	userRoleAuditor  = "auditor"
	userRoleOperator = "operator"
	userRoleUser     = "user"
)

var userRoles = []string{userRoleAdmin, userRoleAuditor, userRoleOperator, userRoleUser}

func validUserRole(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	for _, r := range userRoles {
		if r == v {
			return r, true
		}
	}
	return "", false
}

// roleOf maps the stored columns to the role.
func roleOf(isAdmin bool, stored string) string {
	if isAdmin {
		return userRoleAdmin
	}
	switch stored {
	case userRoleAuditor, userRoleOperator:
		return stored
	}
	return userRoleUser
}

// userRole returns the role of an active account ("" when it does not exist or is disabled).
func userRole(userID int) string {
	var isAdmin, disabled int
	var stored string
	if db.QueryRow(`SELECT is_admin, disabled, COALESCE(role,'') FROM users WHERE id=?`, userID).Scan(&isAdmin, &disabled, &stored) != nil || disabled == 1 {
		return ""
	}
	return roleOf(isAdmin == 1, stored)
}

func isAuditorUser(userID int) bool { return userRole(userID) == userRoleAuditor }

// requireAuditRead lets administrators and security auditors read audit data (GET only).
func requireAuditRead(w http.ResponseWriter, r *http.Request) (int, bool) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return 0, false
	}
	switch userRole(userID) {
	case userRoleAdmin:
		return userID, true
	case userRoleAuditor:
		if r.Method == http.MethodGet {
			return userID, true
		}
		jsonError(w, "Security auditors have read-only access", 403)
		return 0, false
	}
	jsonError(w, "Admin or security auditor access required", 403)
	return 0, false
}

// ─── WHO POLICIES ────────────────────────────────────
//
// Policies that say who may use a feature accept, besides off / admins / all:
//   operators         – administrators and operators
//   teams:ops,network – administrators and members of the named teams

var whoPolicies = map[string]bool{
	"tunnel_users": true, "tunnel_remote_forward": true, "serial_ports": true, "network_tools": true, "proxies": true,
	"git_checks": true, "git_update": true, "git_upgrade": true, "git_rollback": true, "git_restart": true, "git_install": true,
	"git_medium": true, "medium_native": true, "git_transfer": true, "git_gitignore_mr": true, "bitwarden": true,
	"ai_assistant": true, "ai_personal_keys": true, "ai_opencode": true, "ai_opencode_sandbox": true, "ai_mcp": true,
}

var teamNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]{0,39}$`)

// validateWhoValue accepts operators and teams:<names>; the names must be teams that exist.
func validateWhoValue(v string) (string, error) {
	if strings.EqualFold(v, "operators") {
		return "operators", nil
	}
	if len(v) < 6 || !strings.EqualFold(v[:6], "teams:") {
		return "", fmt.Errorf("not a who-value")
	}
	names := []string{}
	seen := map[string]bool{}
	for _, n := range strings.Split(v[6:], ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		var name string
		if db.QueryRow(`SELECT name FROM teams WHERE name=? COLLATE NOCASE`, n).Scan(&name) != nil {
			return "", fmt.Errorf("unknown team %q", n)
		}
		if !seen[strings.ToLower(name)] {
			seen[strings.ToLower(name)] = true
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("name at least one team: teams:<name>,<name>")
	}
	return "teams:" + strings.Join(names, ","), nil
}

// policyAllows reports whether userID passes a who-policy value.
func policyAllows(userID int, value string) bool {
	role := userRole(userID)
	if role == "" || role == userRoleAuditor {
		return false // auditors read; they do not use features
	}
	switch {
	case value == "all":
		return true
	case value == "admins":
		return role == userRoleAdmin
	case value == "operators":
		return role == userRoleAdmin || role == userRoleOperator
	case strings.HasPrefix(value, "teams:"):
		if role == userRoleAdmin {
			return true
		}
		for _, n := range strings.Split(value[6:], ",") {
			if n = strings.TrimSpace(n); n != "" && userInTeamNamed(userID, n) {
				return true
			}
		}
	}
	return false
}

// policyWhoAllows applies the who-policy setting key.
func policyWhoAllows(userID int, key string) bool { return policyAllows(userID, getSetting(key)) }

// policyLimitText describes who a who-policy value allows, for refusal messages.
func policyLimitText(value string) string {
	switch {
	case value == "admins":
		return "administrators"
	case value == "operators":
		return "administrators and operators"
	case strings.HasPrefix(value, "teams:"):
		return "administrators and the teams " + strings.ReplaceAll(value[6:], ",", ", ")
	}
	return value
}

// ─── STEP-UP ─────────────────────────────────────────
//
// With require_step_up on, dangerous actions (policies, the security level, roles, disabling
// or deleting users, password and 2FA resets, revealing stored secrets) need a fresh
// confirmation: the password, plus a 2FA code when the account has 2FA. It is valid for
// step_up_minutes in this sign-in session only; signing in counts as a fresh confirmation.

// stepUpFresh: did this session confirm recently enough?
func stepUpFresh(sessionID int) bool {
	var at string
	db.QueryRow(`SELECT COALESCE(step_up_at,'') FROM auth_sessions WHERE id=?`, sessionID).Scan(&at)
	t, ok := parseTS(at)
	return ok && time.Since(t) < time.Duration(settingInt("step_up_minutes"))*time.Minute
}

// requireStepUp answers 403 {step_up: true} when the action needs a confirmation first.
func requireStepUp(w http.ResponseWriter, r *http.Request, action string) bool {
	if !settingBool("require_step_up") {
		return true
	}
	return requireStepUpAlways(w, r, action)
}

// requireStepUpAlways asks for a fresh confirmation even when the step-up policy is off,
// for actions that add a way to sign in (linking a directory account).
func requireStepUpAlways(w http.ResponseWriter, r *http.Request, action string) bool {
	s, err := lookupSession(r)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return false
	}
	if stepUpFresh(s.ID) {
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(403)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   "Confirm with your password (and 2FA code) to continue",
		"step_up": true, "action": action, "totp": userHasTOTP(s.UserID),
		"password": userHasPassword(s.UserID), "sso": userSSOProviders(s.UserID),
		"webauthn": userHasPasskey(s.UserID),
	})
	return false
}

func userHasTOTP(userID int) bool {
	var on int
	db.QueryRow(`SELECT totp_enabled FROM users WHERE id=?`, userID).Scan(&on)
	return on == 1
}

// POST /api/auth/step-up {password, code}
func apiAuthStepUpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	s, err := lookupSession(r)
	if err != nil || s.Restricted != "" {
		jsonError(w, "Unauthorized", 401)
		return
	}
	ip := clientIP(r)
	if blocked, wait := loginBlocked(ip); blocked {
		jsonError(w, fmt.Sprintf("Too many failed attempts. Try again in %d s.", int(wait.Seconds())+1), 429)
		return
	}
	var in struct {
		Password string `json:"password"`
		Code     string `json:"code"`
		Action   string `json:"action"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	u, err := loadUser("id=?", s.UserID)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return
	}
	action := truncateStr(strings.TrimSpace(in.Action), 60)
	fail := func(reason, msg string) {
		recordLoginResult(ip, false)
		auditLogAs(r, u.ID, u.Username, "auth.step_up_failed", action, map[string]string{"reason": reason})
		jsonError(w, msg, 403)
	}
	if !userCheckPassword(u, in.Password) {
		fail("wrong password", "Wrong password")
		return
	}
	method := "password"
	if u.TOTPEnabled {
		code := strings.TrimSpace(in.Code)
		if okCode, step := totpVerify(decryptValue(u.TOTPSecret), code, u.TOTPLastStep); okCode {
			db.Exec(`UPDATE users SET totp_last_step=? WHERE id=?`, step, u.ID)
			method = "password+totp"
		} else if okRec, remaining := useRecoveryCode(u.Recovery, code); okRec {
			db.Exec(`UPDATE users SET recovery_codes=? WHERE id=?`, remaining, u.ID)
			method = "password+recovery_code"
		} else {
			fail("wrong 2FA code", "Invalid verification code")
			return
		}
	}
	recordLoginResult(ip, true)
	db.Exec(`UPDATE auth_sessions SET step_up_at=? WHERE id=?`, nowRFC(), s.ID)
	auditLogAs(r, u.ID, u.Username, "auth.step_up", action, map[string]string{"method": method})
	jsonOK(w, map[string]interface{}{"ok": true, "minutes": settingInt("step_up_minutes")})
}

// ─── BREAK-GLASS ─────────────────────────────────────
//
// A break-glass administrator is a local account kept for emergencies (the directory is down,
// lockdown). It must use 2FA, may be limited to break_glass_ips, and every sign-in is audited
// and sent to all other administrators.

func breakGlassIPAllowed(ip string) bool {
	list := strings.TrimSpace(getSetting("break_glass_ips"))
	if list == "" {
		return true
	}
	addr := net.ParseIP(strings.TrimSpace(ip))
	if addr == nil {
		return false
	}
	for _, p := range strings.Split(list, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.Contains(p, "/") {
			if q := net.ParseIP(p); q != nil && q.Equal(addr) {
				return true
			}
			continue
		}
		if _, n, err := net.ParseCIDR(p); err == nil && n.Contains(addr) {
			return true
		}
	}
	return false
}

func validateIPList(v string) (string, error) {
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "/") {
			if _, n, err := net.ParseCIDR(p); err == nil {
				out = append(out, n.String())
				continue
			}
		} else if ip := net.ParseIP(p); ip != nil {
			out = append(out, ip.String())
			continue
		}
		return "", fmt.Errorf("%q is not an IP address or network (CIDR)", p)
	}
	return strings.Join(out, ","), nil
}

// breakGlassSignedIn audits the sign-in and tells every other active administrator.
func breakGlassSignedIn(r *http.Request, u *userRow) {
	ip := clientIP(r)
	auditLogAs(r, u.ID, u.Username, "auth.break_glass_login", u.Username, map[string]string{"ip": ip})
	rows, err := db.Query(`SELECT id FROM users WHERE is_admin=1 AND disabled=0 AND id != ?`, u.ID)
	if err != nil {
		return
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	name := u.Username
	for _, id := range ids {
		notifyUser(id, "security.break_glass_login", func(lang string) (string, string) {
			if lang == "hr" {
				return "Prijava break-glass administratora", fmt.Sprintf("Break-glass račun %s prijavio se s adrese %s.", name, ip)
			}
			return "Break-glass administrator signed in", fmt.Sprintf("The break-glass account %s signed in from %s.", name, ip)
		})
	}
}

// ─── HANDOVER ────────────────────────────────────────
//
// When an account is deleted, its connections, folders, credentials, SSH keys, shares and proxies can
// go to another account (the lead of its team, or an administrator) instead of being deleted,
// so a person leaving does not take the team's access with them. Shares keep their members
// and teams, so the team keeps using the same connections.

type handoverCounts struct {
	Connections int `json:"connections"`
	Folders     int `json:"folders"`
	Credentials int `json:"credentials"`
	Shares      int `json:"shares"`
	Proxies     int `json:"proxies"`
	Keys        int `json:"keys"`
}

func ownedCounts(userID int) handoverCounts {
	var c handoverCounts
	db.QueryRow(`SELECT COUNT(*) FROM connections WHERE user_id=?`, userID).Scan(&c.Connections)
	db.QueryRow(`SELECT COUNT(*) FROM folders WHERE user_id=?`, userID).Scan(&c.Folders)
	db.QueryRow(`SELECT COUNT(*) FROM credentials WHERE owner_id=?`, userID).Scan(&c.Credentials)
	db.QueryRow(`SELECT COUNT(*) FROM share_links WHERE owner_id=?`, userID).Scan(&c.Shares)
	db.QueryRow(`SELECT COUNT(*) FROM proxies WHERE owner_id=?`, userID).Scan(&c.Proxies)
	db.QueryRow(`SELECT COUNT(*) FROM ssh_keys WHERE user_id=?`, userID).Scan(&c.Keys)
	return c
}

// suggestedHandover: the lead of the user's team (when exactly one other lead exists), else 0.
func suggestedHandover(userID int) (int, string) {
	rows, err := db.Query(`SELECT DISTINCT tm2.user_id FROM team_members tm JOIN team_members tm2 ON tm2.team_id=tm.team_id
		JOIN users u ON u.id=tm2.user_id WHERE tm.user_id=? AND tm2.lead=1 AND tm2.user_id != ? AND u.disabled=0`, userID, userID)
	if err != nil {
		return 0, ""
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 1 {
		return 0, ""
	}
	return ids[0], "team_lead"
}

// handOver moves what fromID owns to toID. Folder names keep apart by a suffix when toID
// already has a folder of the same name.
func handOver(fromID, toID int) (handoverCounts, error) {
	c := ownedCounts(fromID)
	from := usernameOf(fromID)
	tx, err := db.Begin()
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	steps := []string{
		`UPDATE folders SET name = name || ' (' || ? || ')' WHERE user_id=? AND name IN (SELECT name FROM folders WHERE user_id=?)`,
	}
	if _, err := tx.Exec(steps[0], from, fromID, toID); err != nil {
		return c, err
	}
	for _, q := range []string{
		`UPDATE folders SET user_id=? WHERE user_id=?`,
		`UPDATE connections SET user_id=? WHERE user_id=?`,
		`UPDATE credentials SET owner_id=? WHERE owner_id=?`,
		`UPDATE share_links SET owner_id=? WHERE owner_id=?`,
		`UPDATE proxies SET owner_id=? WHERE owner_id=?`,
		`UPDATE ssh_keys SET user_id=? WHERE user_id=?`,
	} {
		if _, err := tx.Exec(q, toID, fromID); err != nil {
			return c, err
		}
	}
	// The new owner does not need a grant or membership for what is now theirs.
	tx.Exec(`DELETE FROM credential_grants WHERE user_id=? AND credential_id IN (SELECT id FROM credentials WHERE owner_id=?)`, toID, toID)
	tx.Exec(`DELETE FROM proxy_grants WHERE user_id=? AND proxy_id IN (SELECT id FROM proxies WHERE owner_id=?)`, toID, toID)
	tx.Exec(`DELETE FROM share_members WHERE user_id=? AND share_id IN (SELECT id FROM share_links WHERE owner_id=?)`, toID, toID)
	return c, tx.Commit()
}

// ─── API: roles in /api/me and lists for pickers ─────

// userTeams returns the teams a user belongs to.
func userTeams(userID int) []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := db.Query(`SELECT t.id, t.name, tm.lead FROM team_members tm JOIN teams t ON t.id=tm.team_id WHERE tm.user_id=? ORDER BY t.name COLLATE NOCASE`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, lead int
		var name string
		rows.Scan(&id, &name, &lead)
		out = append(out, map[string]interface{}{"id": id, "name": name, "lead": lead == 1})
	}
	return out
}

func userInTeamNamed(userID int, name string) bool {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM team_members tm JOIN teams t ON t.id=tm.team_id WHERE tm.user_id=? AND t.name=? COLLATE NOCASE`, userID, name).Scan(&n)
	return n > 0
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
}

// storedRole is the users.role value ("" for user and admin, whose flag is is_admin).
func storedRole(role string) string {
	if role == userRoleAuditor || role == userRoleOperator {
		return role
	}
	return ""
}
