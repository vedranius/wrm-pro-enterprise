// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ─── LOGIN SESSIONS ──────────────────────────────────
//
// The browser holds a random 256-bit token in an HttpOnly cookie; the database stores only
// its SHA-256 hash, so a leaked database does not leak usable sessions. Sessions expire
// after session_max_days and after session_idle_hours without activity.
//
// A session can be "restricted": after an administrator reset the password
// (restricted="password") or when 2FA is required but not yet set up (restricted="mfa").
// A restricted session can only call the endpoints needed to fix that.

var (
	errUnauthenticated = errors.New("unauthenticated")
	errRestricted      = errors.New("restricted session")
)

type authSession struct {
	ID         int
	UserID     int
	Restricted string
	LockedAt   string // screen lock (v13.7): set while this sign-in session is locked
}

func tokenHash(raw string) string { return "h:" + sha256Hex(raw) }

func createAuthSession(w http.ResponseWriter, r *http.Request, userID int, restricted string) error {
	raw := randomToken(32)
	now := time.Now().UTC()
	exp := now.AddDate(0, 0, settingInt("session_max_days"))
	// Signing in counts as a fresh re-authentication for the step-up window.
	_, err := db.Exec(`INSERT INTO auth_sessions (user_id, token, expires_at, last_active_at, created_at, ip, user_agent, restricted, step_up_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		userID, tokenHash(raw), exp.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339), clientIP(r), truncateStr(r.UserAgent(), 300), restricted, now.Format(time.RFC3339))
	if err != nil {
		return err
	}
	setCookie(w, r, sessionCookieName, raw, settingInt("session_max_days")*86400)
	return nil
}

func clearAuthCookie(w http.ResponseWriter, r *http.Request) {
	setCookie(w, r, sessionCookieName, "", -1)
}

func parseTS(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}

func lookupSession(r *http.Request) (*authSession, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" || len(cookie.Value) > 200 {
		return nil, errUnauthenticated
	}
	h := tokenHash(cookie.Value)
	var s authSession
	var expiresAt, lastActive string
	var disabled int
	err = db.QueryRow(`SELECT a.id, a.user_id, COALESCE(a.restricted,''), a.expires_at, COALESCE(a.last_active_at,''), u.disabled, COALESCE(a.locked_at,'')
		FROM auth_sessions a JOIN users u ON u.id=a.user_id WHERE a.token=?`, h).
		Scan(&s.ID, &s.UserID, &s.Restricted, &expiresAt, &lastActive, &disabled, &s.LockedAt)
	if err != nil {
		return nil, errUnauthenticated
	}
	now := time.Now()
	expired := false
	if exp, ok := parseTS(expiresAt); ok && now.After(exp) {
		expired = true
	}
	la, laOK := parseTS(lastActive)
	if laOK && now.Sub(la) > time.Duration(settingInt("session_idle_hours"))*time.Hour {
		expired = true
	}
	if expired || disabled == 1 || lockdownRefuses(s.UserID) {
		db.Exec(`DELETE FROM auth_sessions WHERE id=?`, s.ID)
		return nil, errUnauthenticated
	}
	if !laOK || now.Sub(la) > time.Minute {
		db.Exec(`UPDATE auth_sessions SET last_active_at=? WHERE id=?`, now.UTC().Format(time.RFC3339), s.ID)
	}
	return &s, nil
}

// currentUserID returns the signed-in user; restricted sessions do not count as signed in.
func currentUserID(r *http.Request) (int, error) {
	s, err := lookupSession(r)
	if err != nil {
		return 0, err
	}
	if s.Restricted != "" {
		return 0, errRestricted
	}
	return s.UserID, nil
}

var restrictedAllowedPaths = map[string]bool{
	"/api/auth/me": true, "/api/auth/logout": true, "/api/auth/keepalive": true, "/api/auth/password": true,
	"/api/auth/2fa/setup": true, "/api/auth/2fa/enable": true,
}

func requireAuth(w http.ResponseWriter, r *http.Request) (int, bool) {
	s, err := lookupSession(r)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return 0, false
	}
	if s.Restricted != "" && !restrictedAllowedPaths[r.URL.Path] {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]string{"error": restrictedMessage(s.Restricted), "restricted": s.Restricted})
		return 0, false
	}
	if screenLockRefuses(s, r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusLocked)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "WRM is locked; unlock it first", "screen_locked": true})
		return 0, false
	}
	return s.UserID, true
}

func restrictedMessage(reason string) string {
	if reason == "password" {
		return "You must change your password first"
	}
	return "Two-factor authentication must be set up first"
}

func isAdminUser(userID int) bool {
	var isAdmin int
	db.QueryRow("SELECT is_admin FROM users WHERE id=? AND disabled=0", userID).Scan(&isAdmin)
	return isAdmin == 1
}

func requireAdmin(w http.ResponseWriter, r *http.Request) (int, bool) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return 0, false
	}
	if !isAdminUser(userID) {
		jsonError(w, "Admin access required", 403)
		return 0, false
	}
	return userID, true
}

func usernameOf(userID int) string {
	var u string
	db.QueryRow(`SELECT username FROM users WHERE id=?`, userID).Scan(&u)
	return u
}

func revokeUserSessions(userID int, exceptSessionID int) int64 {
	res, err := db.Exec(`DELETE FROM auth_sessions WHERE user_id=? AND id != ?`, userID, exceptSessionID)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}

func cleanupAuthSessions() {
	rows, err := db.Query(`SELECT a.id, a.expires_at, COALESCE(a.last_active_at,'') FROM auth_sessions a`)
	if err != nil {
		return
	}
	var dead []int
	idle := time.Duration(settingInt("session_idle_hours")) * time.Hour
	now := time.Now()
	for rows.Next() {
		var id int
		var exp, la string
		rows.Scan(&id, &exp, &la)
		if t, ok := parseTS(exp); ok && now.After(t) {
			dead = append(dead, id)
		} else if t, ok := parseTS(la); ok && now.Sub(t) > idle {
			dead = append(dead, id)
		}
	}
	rows.Close()
	for _, id := range dead {
		db.Exec(`DELETE FROM auth_sessions WHERE id=?`, id)
	}
}

// migrateSessionTokens hashes session tokens stored in clear text by older versions.
func migrateSessionTokens() {
	rows, err := db.Query(`SELECT id, token FROM auth_sessions WHERE token NOT LIKE 'h:%'`)
	if err != nil {
		return
	}
	type row struct {
		id    int
		token string
	}
	var list []row
	for rows.Next() {
		var x row
		rows.Scan(&x.id, &x.token)
		list = append(list, x)
	}
	rows.Close()
	for _, x := range list {
		db.Exec(`UPDATE auth_sessions SET token=? WHERE id=?`, tokenHash(x.token), x.id)
	}
}

// ─── PASSWORDS ───────────────────────────────────────

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(hash), err
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// dummyHash is compared against when a username does not exist, so a failed login takes
// the same time whether or not the account exists (no username enumeration).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("wrm-dummy-password"), 12)

func randomPassword() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	b := []byte(randomToken(24))
	out := make([]byte, 16)
	for i := range out {
		out[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(out)
}

// ─── SIGN-IN ─────────────────────────────────────────

type userRow struct {
	ID            int
	Username      string
	Hash          string
	IsAdmin       bool
	Disabled      bool
	TOTPEnabled   bool
	TOTPSecret    string
	TOTPLastStep  int64
	Recovery      string
	MustChange    bool
	FailedLogins  int
	LockedUntil   string
	DisplayName   string
	CreatedAt     string
	LastLoginAt   string
	LastLoginIP   string
	PasswordSetAt string
	Role          string // admin, auditor, operator or user
	BreakGlass    bool
}

func loadUser(where string, arg interface{}) (*userRow, error) {
	var u userRow
	var isAdmin, disabled, totpEnabled, mustChange, breakGlass int
	var role string
	err := db.QueryRow(`SELECT id, username, password_hash, is_admin, disabled, totp_enabled, COALESCE(totp_secret,''), totp_last_step,
		COALESCE(recovery_codes,''), must_change_password, failed_logins, COALESCE(locked_until,''), COALESCE(display_name,''),
		created_at, COALESCE(last_login_at,''), COALESCE(last_login_ip,''), COALESCE(password_changed_at,''), COALESCE(role,''), COALESCE(break_glass,0)
		FROM users WHERE `+where, arg).Scan(&u.ID, &u.Username, &u.Hash, &isAdmin, &disabled, &totpEnabled, &u.TOTPSecret, &u.TOTPLastStep,
		&u.Recovery, &mustChange, &u.FailedLogins, &u.LockedUntil, &u.DisplayName, &u.CreatedAt, &u.LastLoginAt, &u.LastLoginIP, &u.PasswordSetAt, &role, &breakGlass)
	if err != nil {
		return nil, err
	}
	u.IsAdmin, u.Disabled, u.TOTPEnabled, u.MustChange = isAdmin == 1, disabled == 1, totpEnabled == 1, mustChange == 1
	u.Role, u.BreakGlass = roleOf(u.IsAdmin, role), breakGlass == 1 && u.IsAdmin
	return &u, nil
}

func mfaRequiredByPolicy(isAdmin bool) bool {
	switch getSetting("require_2fa") {
	case "all":
		return true
	case "admins":
		return isAdmin
	}
	return false
}

func restrictionFor(u *userRow) string {
	if u.MustChange {
		return "password"
	}
	if !u.TOTPEnabled && (mfaRequiredByPolicy(u.IsAdmin) || u.BreakGlass) {
		return "mfa"
	}
	return ""
}

func mePayload(userID int, restricted string) map[string]interface{} {
	u, err := loadUser("id=?", userID)
	if err != nil {
		return nil
	}
	return map[string]interface{}{
		"id": u.ID, "username": u.Username, "display_name": u.DisplayName, "created_at": u.CreatedAt,
		"is_admin": u.IsAdmin, "totp_enabled": u.TOTPEnabled, "must_change_password": u.MustChange,
		"restricted": restricted, "mfa_required": mfaRequiredByPolicy(u.IsAdmin) || u.BreakGlass,
		"role": u.Role, "is_auditor": u.Role == userRoleAuditor, "break_glass": u.BreakGlass, "teams": userTeams(u.ID),
		"sso_only": userSSOOnly(u.ID), "sso": userSSOProviders(u.ID), "directory_password": userLDAPIdentity(u.ID) != nil,
		"step_up":             map[string]interface{}{"required": settingBool("require_step_up"), "minutes": settingInt("step_up_minutes"), "totp": u.TOTPEnabled},
		"recovery_codes_left": recoveryCodesLeft(u.Recovery),
		"webauthn_count":      len(loadWebAuthnCredentials(u.ID)),
		"password_min_length": settingInt("password_min_length"),
		"screen_lock":         screenLockPolicy(u.ID),
		"appearance":          map[string]interface{}{"default_theme": getSetting("appearance_default_theme"), "custom_css": settingBool("appearance_custom_css")},
		"policy": map[string]interface{}{
			"server_keys":      u.IsAdmin || settingBool("allow_server_keys"),
			"link_shares":      u.IsAdmin || settingBool("allow_link_shares"),
			"secret_export":    u.IsAdmin || settingBool("allow_secret_export"),
			"voice":            settingBool("voice_enabled"),
			"broadcast":        settingBool("broadcast_enabled"),
			"desktop":          settingBool("desktop_enabled"),
			"bmc":              settingBool("bmc_enabled"),
			"serial":           serialAllowed(u.ID),
			"network_tools":    networkToolsAllowed(u.ID),
			"notifications":    settingBool("notifications_enabled"),
			"proxies":          proxiesAllowed(u.ID),
			"git":              gitAllowed(),
			"git_checks":       gitChecksAllowed(u.ID),
			"git_update":       gitActionAllowed(u.ID, "git_update"),
			"git_upgrade":      gitActionAllowed(u.ID, "git_upgrade"),
			"git_rollback":     gitActionAllowed(u.ID, "git_rollback"),
			"git_restart":      gitActionAllowed(u.ID, "git_restart"),
			"git_install":      gitActionAllowed(u.ID, "git_install"),
			"bitwarden":        bitwardenAllowed(u.ID),
			"git_medium":       mediumAllowed(u.ID),
			"git_transfer":     gitActionAllowed(u.ID, "git_transfer"),
			"git_gitignore_mr": gitActionAllowed(u.ID, "git_gitignore_mr"),
			"ai":               aiAllowedFlag(u.ID),
			"ai_personal_keys": aiPersonalKeysAllowed(u.ID),
			"mcp":              mcpAllowedFlag(u.ID),
			"mcp_enabled":      settingBool("ai_mcp_enabled"),
		},
	}
}

// finishLogin creates the session after all checks passed.
func finishLogin(w http.ResponseWriter, r *http.Request, u *userRow, method string) {
	if lockdownRefuses(u.ID) {
		auditLogAs(r, u.ID, u.Username, "auth.login_failed", "", map[string]string{"method": method, "reason": "lockdown"})
		jsonError(w, lockdownMessage, 403)
		return
	}
	restricted := restrictionFor(u)
	if err := createAuthSession(w, r, u.ID, restricted); err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	db.Exec(`UPDATE users SET last_login_at=?, last_login_ip=?, failed_logins=0, locked_until='' WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339), clientIP(r), u.ID)
	auditLogAs(r, u.ID, u.Username, "auth.login", "", map[string]string{"method": method, "user_agent": truncateStr(r.UserAgent(), 160)})
	if u.BreakGlass {
		breakGlassSignedIn(r, u)
	}
	jsonOK(w, mePayload(u.ID, restricted))
}

// passwordOrDirectoryConfirmed: the account's password, or for an account that signs in only
// through the directory (no password) a fresh confirmation there (step-up window).
func passwordOrDirectoryConfirmed(u *userRow, password string, sessionID int) bool {
	if userCheckPassword(u, password) {
		return true
	}
	return userSSOOnly(u.ID) && stepUpFresh(sessionID)
}

// Two-factor step: after a correct password the browser gets a short-lived signed token
// and must present it together with a TOTP or recovery code.
var mfaAttempts = struct {
	sync.Mutex
	m map[string]int
}{m: map[string]int{}}

func newMFAToken(userID int) string {
	payload := fmt.Sprintf("%d.%d.%s", userID, time.Now().Add(5*time.Minute).Unix(), randomToken(12))
	return payload + "." + signValue("mfa", payload)
}

func parseMFAToken(tok string) (int, string, bool) {
	i := strings.LastIndex(tok, ".")
	if i < 0 {
		return 0, "", false
	}
	payload, sig := tok[:i], tok[i+1:]
	if !verifySigned(sig, "mfa", payload) {
		return 0, "", false
	}
	parts := strings.Split(payload, ".")
	if len(parts) != 3 {
		return 0, "", false
	}
	uid, _ := strconv.Atoi(parts[0])
	exp, _ := strconv.ParseInt(parts[1], 10, 64)
	if uid <= 0 || time.Now().Unix() > exp {
		return 0, "", false
	}
	return uid, parts[2], true
}

func apiAuthConfigHandler(w http.ResponseWriter, r *http.Request) {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	out := map[string]interface{}{
		"version":           AppVersion,
		"first_run":         n == 0,
		"registration_open": n == 0 || getSetting("registration") == "open",
	}
	if n == 0 {
		// The first-run screen offers the security level (preselected when -security-level chose one).
		out["security_level"], out["security_level_fixed"] = securityLevel(), levelChosen() || settingLocked("security_level")
		out["secure_transport"] = secureTransport(r)
	}
	if msg := signInTransportError(r); msg != "" {
		out["https_required"] = msg
	}
	// Directory sign-in buttons; local_login_limited hides the password form behind "Emergency sign-in".
	out["sso"], out["local_login_limited"] = oidcSignInOptions(), localLoginLimited()
	out["ldap"] = ldapSignInOptions()
	out["saml"] = samlSignInOptions()
	if st := lockdown(); st.On {
		out["lockdown"] = map[string]string{"reason": st.Reason, "at": st.At}
	}
	// The sign-in page's version badge: an available update and its release page.
	if info := lastUpdateInfo(); settingBool("update_check") && updateAvailable(info) {
		out["update"], out["update_url"] = info.Latest, info.URL
	}
	jsonOK(w, out)
}

func apiAuthRegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var payload struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		SecurityLevel string `json:"security_level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	if msg := signInTransportError(r); msg != "" {
		jsonError(w, msg, 403)
		return
	}
	ip := clientIP(r)
	if blocked, wait := loginBlocked(ip); blocked {
		jsonError(w, fmt.Sprintf("Too many attempts. Try again in %d s.", int(wait.Seconds())+1), 429)
		return
	}
	var existing int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&existing)
	first := existing == 0
	if !first && getSetting("registration") != "open" {
		jsonError(w, "Registration is disabled. Ask an administrator to create an account for you.", 403)
		return
	}
	if !first && localLoginLimited() {
		jsonError(w, "Accounts come from your organisation's directory: sign in with its button.", 403)
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	if err := validUsername(payload.Username); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := validatePassword(payload.Password); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	hash, err := hashPassword(payload.Password)
	if err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at, password_changed_at) VALUES (?,?,?,?,?)`,
		payload.Username, hash, boolToInt(first), now, now)
	if err != nil {
		recordLoginResult(ip, false)
		jsonError(w, "Username already exists", 400)
		return
	}
	id, _ := res.LastInsertId()
	auditLogAs(r, int(id), payload.Username, "auth.register", "", map[string]bool{"admin": first})
	if first && payload.SecurityLevel != "" {
		applyFirstRunLevel(r, int(id), payload.SecurityLevel)
	}
	u, err := loadUser("id=?", id)
	if err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	finishLogin(w, r, u, "register")
}

func apiAuthLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	if msg := signInTransportError(r); msg != "" {
		jsonError(w, msg, 403)
		return
	}
	ip := clientIP(r)
	if blocked, wait := loginBlocked(ip); blocked {
		jsonError(w, fmt.Sprintf("Too many failed attempts. Try again in %d s.", int(wait.Seconds())+1), 429)
		return
	}
	username := strings.TrimSpace(payload.Username)
	u, err := loadUser("username=?", username)
	// Directory (LDAP / AD) accounts: a WRM account linked to a directory, or a name that is
	// not a WRM user and may be a directory user.
	if ldapAnyEnabled() && (err != nil || userLDAPIdentity(u.ID) != nil) {
		var linked *userRow
		if err == nil {
			linked = u
			if t, ok := parseTS(u.LockedUntil); ok && time.Now().Before(t) {
				recordLoginResult(ip, false)
				jsonError(w, fmt.Sprintf("Account temporarily locked after repeated failed sign-ins. Try again in %d min or ask an administrator.", int(time.Until(t).Minutes())+1), 429)
				return
			}
		}
		if res, handled := ldapSignIn(r, username, payload.Password, linked); handled {
			ldapFinishLogin(w, r, ip, username, linked, res)
			return
		}
	}
	if err != nil {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(payload.Password))
		recordLoginResult(ip, false)
		auditLogAs(r, 0, username, "auth.login_failed", "", map[string]string{"reason": "unknown user"})
		jsonError(w, "Invalid credentials", 401)
		return
	}
	if t, ok := parseTS(u.LockedUntil); ok && time.Now().Before(t) {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(payload.Password))
		recordLoginResult(ip, false)
		jsonError(w, fmt.Sprintf("Account temporarily locked after repeated failed sign-ins. Try again in %d min or ask an administrator.", int(time.Until(t).Minutes())+1), 429)
		return
	}
	ok := checkPassword(u.Hash, payload.Password)
	if !ok && strings.TrimSpace(payload.Password) != payload.Password {
		ok = checkPassword(u.Hash, strings.TrimSpace(payload.Password)) // accounts registered by versions that trimmed passwords
	}
	if !ok {
		recordLoginResult(ip, false)
		fails := u.FailedLogins + 1
		locked := ""
		if fails >= settingInt("login_max_failures") {
			locked = time.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339)
			fails = 0
			auditLogAs(r, u.ID, u.Username, "auth.account_locked", "", map[string]string{"minutes": "15"})
		}
		db.Exec(`UPDATE users SET failed_logins=?, locked_until=? WHERE id=?`, fails, locked, u.ID)
		auditLogAs(r, u.ID, u.Username, "auth.login_failed", "", map[string]string{"reason": "wrong password"})
		jsonError(w, "Invalid credentials", 401)
		return
	}
	if u.Disabled {
		auditLogAs(r, u.ID, u.Username, "auth.login_failed", "", map[string]string{"reason": "account disabled"})
		jsonError(w, "This account is disabled. Contact an administrator.", 403)
		return
	}
	if localLoginLimited() && !u.BreakGlass {
		auditLogAs(r, u.ID, u.Username, "auth.login_failed", "", map[string]string{"reason": "local sign-in limited to break-glass administrators"})
		jsonError(w, "Sign in with your organisation's account. Sign-in with a local WRM password is kept for emergency administrators only.", 403)
		return
	}
	if u.BreakGlass && !breakGlassIPAllowed(ip) {
		auditLogAs(r, u.ID, u.Username, "auth.login_failed", "", map[string]string{"reason": "break-glass account from a network not in break_glass_ips"})
		jsonError(w, "This emergency (break-glass) account may only sign in from the networks the administrator allowed.", 403)
		return
	}
	recordLoginResult(ip, true)
	hasPasskey := userHasPasskey(u.ID)
	if u.TOTPEnabled || hasPasskey {
		jsonOK(w, map[string]interface{}{"mfa_required": true, "mfa_token": newMFAToken(u.ID), "webauthn": hasPasskey})
		return
	}
	finishLogin(w, r, u, "password")
}

// POST /api/auth/mfa {mfa_token, code}  — code is a 6-digit TOTP or a recovery code
func apiAuthMFAHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var payload struct {
		Token string `json:"mfa_token"`
		Code  string `json:"code"`
	}
	json.NewDecoder(r.Body).Decode(&payload)
	if msg := signInTransportError(r); msg != "" {
		jsonError(w, msg, 403)
		return
	}
	ip := clientIP(r)
	if blocked, wait := loginBlocked(ip); blocked {
		jsonError(w, fmt.Sprintf("Too many failed attempts. Try again in %d s.", int(wait.Seconds())+1), 429)
		return
	}
	uid, nonce, ok := parseMFAToken(payload.Token)
	if !ok {
		jsonError(w, "The sign-in step expired. Enter your password again.", 401)
		return
	}
	mfaAttempts.Lock()
	mfaAttempts.m[nonce]++
	attempts := mfaAttempts.m[nonce]
	if len(mfaAttempts.m) > 10000 {
		mfaAttempts.m = map[string]int{}
	}
	mfaAttempts.Unlock()
	if attempts > 5 {
		jsonError(w, "Too many wrong codes. Enter your password again.", 401)
		return
	}
	u, err := loadUser("id=?", uid)
	if err != nil || u.Disabled || !u.TOTPEnabled {
		jsonError(w, "Invalid sign-in", 401)
		return
	}
	code := strings.TrimSpace(payload.Code)
	method := "totp"
	if okCode, step := totpVerify(decryptValue(u.TOTPSecret), code, u.TOTPLastStep); okCode {
		db.Exec(`UPDATE users SET totp_last_step=? WHERE id=?`, step, u.ID)
	} else if okRec, remaining := useRecoveryCode(u.Recovery, code); okRec {
		db.Exec(`UPDATE users SET recovery_codes=? WHERE id=?`, remaining, u.ID)
		method = "recovery_code"
		auditLogAs(r, u.ID, u.Username, "auth.recovery_code_used", "", map[string]int{"left": recoveryCodesLeft(remaining)})
	} else {
		recordLoginResult(ip, false)
		auditLogAs(r, u.ID, u.Username, "auth.login_failed", "", map[string]string{"reason": "wrong 2FA code"})
		jsonError(w, "Invalid verification code", 401)
		return
	}
	recordLoginResult(ip, true)
	if identityID := oidcMFAIdentity(nonce); identityID > 0 {
		// The second step of a directory sign-in: the session belongs to that identity.
		finishLogin(w, r, u, "oidc+"+method)
		db.Exec(`UPDATE auth_sessions SET identity_id=? WHERE id=(SELECT MAX(id) FROM auth_sessions WHERE user_id=?) AND identity_id=0`, identityID, u.ID)
		return
	}
	finishLogin(w, r, u, method)
}

func apiAuthLogoutHandler(w http.ResponseWriter, r *http.Request) {
	if s, err := lookupSession(r); err == nil {
		db.Exec("DELETE FROM auth_sessions WHERE id=?", s.ID)
		auditLog(r, s.UserID, "auth.logout", "", nil)
		lockBitwardenUser(s.UserID)
	}
	clearAuthCookie(w, r)
	jsonOK(w, map[string]string{"status": "ok"})
}

func apiAuthMeHandler(w http.ResponseWriter, r *http.Request) {
	s, err := lookupSession(r)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return
	}
	me := mePayload(s.UserID, s.Restricted)
	if me == nil {
		jsonError(w, "Unauthorized", 401)
		return
	}
	me["screen_locked"] = s.LockedAt != "" && screenLockMode() != "browser"
	jsonOK(w, me)
}

func apiAuthKeepaliveHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	jsonOK(w, map[string]interface{}{"user_id": userID, "status": "alive"})
}

// POST /api/auth/password {current_password, new_password}
func apiAuthPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	s, err := lookupSession(r)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return
	}
	var p struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	json.NewDecoder(r.Body).Decode(&p)
	u, err := loadUser("id=?", s.UserID)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return
	}
	if userLDAPIdentity(u.ID) != nil {
		jsonError(w, "Your password is managed by your organisation's directory; change it there", 400)
		return
	}
	if !checkPassword(u.Hash, p.Current) {
		auditLogAs(r, u.ID, u.Username, "auth.password_change_failed", "", nil)
		jsonError(w, "Current password is wrong", 400)
		return
	}
	if err := validatePassword(p.New); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if p.New == p.Current {
		jsonError(w, "The new password must be different", 400)
		return
	}
	hash, err := hashPassword(p.New)
	if err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	db.Exec(`UPDATE users SET password_hash=?, must_change_password=0, password_changed_at=? WHERE id=?`, hash, time.Now().UTC().Format(time.RFC3339), u.ID)
	n := revokeUserSessions(u.ID, s.ID)
	u.MustChange = false
	restricted := restrictionFor(u)
	db.Exec(`UPDATE auth_sessions SET restricted=? WHERE id=?`, restricted, s.ID)
	auditLogAs(r, u.ID, u.Username, "auth.password_changed", "", map[string]int64{"other_sessions_revoked": n})
	jsonOK(w, mePayload(u.ID, restricted))
}

// ─── TWO-FACTOR MANAGEMENT ───────────────────────────
// POST /api/auth/2fa/setup                     → {secret, uri, qr}
// POST /api/auth/2fa/enable  {code}            → {recovery_codes}
// POST /api/auth/2fa/disable {password, code}
// POST /api/auth/2fa/recovery {password}       → {recovery_codes}

func apiAuth2FAHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	s, err := lookupSession(r)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/api/auth/2fa/")
	if s.Restricted == "password" || (s.Restricted == "mfa" && action != "setup" && action != "enable") {
		jsonError(w, restrictedMessage(s.Restricted), 403)
		return
	}
	u, err := loadUser("id=?", s.UserID)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return
	}
	var p struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&p)
	switch action {
	case "setup":
		if u.TOTPEnabled {
			jsonError(w, "Two-factor authentication is already enabled", 400)
			return
		}
		secret := newTOTPSecret()
		db.Exec(`UPDATE users SET totp_pending=? WHERE id=?`, encryptValue(secret), u.ID)
		uri := totpURI(u.Username, secret)
		jsonOK(w, map[string]string{"secret": secret, "uri": uri, "qr": qrDataURL(uri)})
	case "enable":
		var pending string
		db.QueryRow(`SELECT COALESCE(totp_pending,'') FROM users WHERE id=?`, u.ID).Scan(&pending)
		secret := decryptValue(pending)
		if secret == "" {
			jsonError(w, "Start the setup again", 400)
			return
		}
		ok, step := totpVerify(secret, p.Code, 0)
		if !ok {
			jsonError(w, "Invalid code — check the time on your phone and try again", 400)
			return
		}
		codes, hashed := newRecoveryCodes()
		db.Exec(`UPDATE users SET totp_secret=?, totp_enabled=1, totp_pending='', totp_last_step=?, recovery_codes=? WHERE id=?`,
			encryptValue(secret), step, hashed, u.ID)
		if s.Restricted == "mfa" {
			db.Exec(`UPDATE auth_sessions SET restricted='' WHERE id=?`, s.ID)
		}
		auditLogAs(r, u.ID, u.Username, "auth.2fa_enabled", "", nil)
		jsonOK(w, map[string]interface{}{"recovery_codes": codes})
	case "disable":
		if !u.TOTPEnabled {
			jsonError(w, "Two-factor authentication is not enabled", 400)
			return
		}
		if mfaRequiredByPolicy(u.IsAdmin) {
			jsonError(w, "Two-factor authentication is required by the security policy", 403)
			return
		}
		if !passwordOrDirectoryConfirmed(u, p.Password, s.ID) {
			jsonError(w, "Wrong password", 400)
			return
		}
		ok, _ := totpVerify(decryptValue(u.TOTPSecret), p.Code, u.TOTPLastStep)
		if !ok {
			ok, _ = useRecoveryCode(u.Recovery, p.Code)
		}
		if !ok {
			jsonError(w, "Invalid verification code", 400)
			return
		}
		db.Exec(`UPDATE users SET totp_secret='', totp_enabled=0, totp_pending='', recovery_codes='' WHERE id=?`, u.ID)
		auditLogAs(r, u.ID, u.Username, "auth.2fa_disabled", "", nil)
		jsonOK(w, map[string]bool{"ok": true})
	case "recovery":
		if !u.TOTPEnabled {
			jsonError(w, "Two-factor authentication is not enabled", 400)
			return
		}
		if !passwordOrDirectoryConfirmed(u, p.Password, s.ID) {
			jsonError(w, "Wrong password", 400)
			return
		}
		codes, hashed := newRecoveryCodes()
		db.Exec(`UPDATE users SET recovery_codes=? WHERE id=?`, hashed, u.ID)
		auditLogAs(r, u.ID, u.Username, "auth.recovery_codes_regenerated", "", nil)
		jsonOK(w, map[string]interface{}{"recovery_codes": codes})
	default:
		jsonError(w, "Not found", 404)
	}
}

// ─── OWN SIGN-IN SESSIONS ────────────────────────────
// GET    /api/auth/sessions           list my sessions (devices)
// DELETE /api/auth/sessions/{id}      sign out one session
// DELETE /api/auth/sessions           sign out all other sessions

func apiAuthSessionsHandler(w http.ResponseWriter, r *http.Request) {
	s, err := lookupSession(r)
	if err != nil || s.Restricted != "" {
		jsonError(w, "Unauthorized", 401)
		return
	}
	idStr := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/auth/sessions"), "/")
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, COALESCE(created_at,''), COALESCE(last_active_at,''), COALESCE(ip,''), COALESCE(user_agent,'') FROM auth_sessions WHERE user_id=? ORDER BY last_active_at DESC`, s.UserID)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		out := []map[string]interface{}{}
		for rows.Next() {
			var id int
			var created, last, ip, ua string
			rows.Scan(&id, &created, &last, &ip, &ua)
			out = append(out, map[string]interface{}{"id": id, "created_at": created, "last_active_at": last, "ip": ip, "user_agent": ua, "current": id == s.ID})
		}
		jsonOK(w, out)
	case http.MethodDelete:
		if idStr == "" {
			n := revokeUserSessions(s.UserID, s.ID)
			auditLog(r, s.UserID, "auth.sessions_revoked", "", map[string]int64{"count": n})
			jsonOK(w, map[string]int64{"revoked": n})
			return
		}
		id, _ := strconv.Atoi(idStr)
		res, _ := db.Exec(`DELETE FROM auth_sessions WHERE id=? AND user_id=?`, id, s.UserID)
		n, _ := res.RowsAffected()
		if n == 0 {
			jsonError(w, "Not found", 404)
			return
		}
		auditLog(r, s.UserID, "auth.session_revoked", strconv.Itoa(id), nil)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// ─── USER DIRECTORY (for sharing) ────────────────────

func apiUsersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if _, ok := requireAuth(w, r); !ok {
		return
	}
	q := "%" + strings.TrimSpace(r.URL.Query().Get("q")) + "%"
	rows, err := db.Query(`SELECT id, username, COALESCE(display_name,'') FROM users WHERE disabled=0 AND (username LIKE ? OR display_name LIKE ?) ORDER BY username LIMIT 200`, q, q)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	users := []map[string]interface{}{}
	for rows.Next() {
		var id int
		var username, dn string
		rows.Scan(&id, &username, &dn)
		users = append(users, map[string]interface{}{"id": id, "username": username, "display_name": dn})
	}
	jsonOK(w, users)
}

// ─── ADMIN: USER MANAGEMENT ──────────────────────────

func activeAdminCount() int {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin=1 AND disabled=0`).Scan(&n)
	return n
}

// ensureAdminExists promotes the oldest account when no active administrator is left.
func ensureAdminExists() {
	var users int
	db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users)
	if users > 0 && activeAdminCount() == 0 {
		db.Exec(`UPDATE users SET is_admin=1, disabled=0 WHERE id=(SELECT MIN(id) FROM users)`)
	}
}

func apiAdminUsersHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT u.id, u.username, COALESCE(u.display_name,''), u.is_admin, COALESCE(u.role,''), COALESCE(u.break_glass,0), u.disabled, u.totp_enabled, u.created_at,
			COALESCE(u.last_login_at,''), COALESCE(u.last_login_ip,''), COALESCE(u.locked_until,''), u.must_change_password,
			(SELECT COUNT(*) FROM connections c WHERE c.user_id=u.id),
			(SELECT COUNT(*) FROM share_links s WHERE s.owner_id=u.id),
			(SELECT COUNT(*) FROM auth_sessions a WHERE a.user_id=u.id)
			FROM users u ORDER BY u.username`)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		now := time.Now()
		users := []map[string]interface{}{}
		for rows.Next() {
			var id, isAdmin, breakGlass, disabled, totp, mustChange, nConn, nShares, nSess int
			var username, dn, role, created, lastLogin, lastIP, lockedUntil string
			rows.Scan(&id, &username, &dn, &isAdmin, &role, &breakGlass, &disabled, &totp, &created, &lastLogin, &lastIP, &lockedUntil, &mustChange, &nConn, &nShares, &nSess)
			locked := false
			if t, ok := parseTS(lockedUntil); ok && now.Before(t) {
				locked = true
			}
			users = append(users, map[string]interface{}{
				"id": id, "username": username, "display_name": dn, "is_admin": isAdmin == 1, "disabled": disabled == 1,
				"totp_enabled": totp == 1, "created_at": created, "last_login_at": lastLogin, "last_login_ip": lastIP,
				"locked": locked, "must_change_password": mustChange == 1, "connections": nConn, "shares": nShares,
				"sessions": nSess, "online": userOnline(id),
				"role": roleOf(isAdmin == 1, role), "break_glass": breakGlass == 1 && isAdmin == 1,
			})
		}
		rows.Close()
		for _, u := range users {
			id := u["id"].(int)
			u["teams"] = userTeams(id)
			sso := []string{}
			for _, p := range userSSOProviders(id) {
				sso = append(sso, p["name"].(string))
			}
			u["sso"], u["sso_only"] = sso, userSSOOnly(id)
		}
		jsonOK(w, users)
	case http.MethodPost:
		var p struct {
			Username       string `json:"username"`
			DisplayName    string `json:"display_name"`
			Password       string `json:"password"`
			IsAdmin        bool   `json:"is_admin"`
			Role           string `json:"role"`
			MustChange     *bool  `json:"must_change_password"`
			RecordingLevel string `json:"recording_level"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		p.Username = strings.TrimSpace(p.Username)
		if err := validUsername(p.Username); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		role := userRoleUser
		if p.IsAdmin {
			role = userRoleAdmin
		}
		if p.Role != "" {
			rv, ok := validUserRole(p.Role)
			if !ok {
				jsonError(w, "Unknown role (admin, auditor, operator or user)", 400)
				return
			}
			role = rv
		}
		p.IsAdmin = role == userRoleAdmin
		if (p.IsAdmin || role == userRoleAuditor) && !requireStepUp(w, r, "user_create_privileged") {
			return
		}
		generated := ""
		if p.Password == "" {
			generated = randomPassword()
			p.Password = generated
		} else if err := validatePassword(p.Password); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		mustChange := p.MustChange == nil || *p.MustChange
		p.RecordingLevel = strings.TrimSpace(p.RecordingLevel)
		if !validRecordingLevel(p.RecordingLevel) {
			jsonError(w, "Recording level must be empty (server settings), none, metadata or input", 400)
			return
		}
		hash, err := hashPassword(p.Password)
		if err != nil {
			jsonError(w, "Server error", 500)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(`INSERT INTO users (username, display_name, password_hash, is_admin, role, must_change_password, recording_level, created_at, password_changed_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			p.Username, truncateStr(strings.TrimSpace(p.DisplayName), 80), hash, boolToInt(p.IsAdmin), storedRole(role), boolToInt(mustChange), p.RecordingLevel, now, now)
		if err != nil {
			jsonError(w, "Username already exists", 400)
			return
		}
		id, _ := res.LastInsertId()
		auditLog(r, adminID, "admin.user_created", p.Username, map[string]interface{}{"admin": p.IsAdmin, "role": role, "must_change_password": mustChange, "recording_level": p.RecordingLevel})
		jsonOK(w, map[string]interface{}{"id": id, "username": p.Username, "generated_password": generated, "recording_level": p.RecordingLevel})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// /api/admin/users/{id}                 PUT {is_admin?, disabled?, display_name?} · DELETE
// /api/admin/users/{id}/reset-password  POST {password?, must_change_password?}
// /api/admin/users/{id}/reset-2fa       POST
// /api/admin/users/{id}/revoke-sessions POST
// /api/admin/users/{id}/unlock          POST
func apiAdminUserByIDHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/users/"), "/"), "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "Bad ID", 400)
		return
	}
	target, err := loadUser("id=?", id)
	if err != nil {
		jsonError(w, "User not found", 404)
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	lastAdmin := target.IsAdmin && !target.Disabled && activeAdminCount() <= 1
	switch {
	case action == "" && r.Method == http.MethodPut:
		var p struct {
			IsAdmin        *bool   `json:"is_admin"`
			Role           *string `json:"role"`
			BreakGlass     *bool   `json:"break_glass"`
			Disabled       *bool   `json:"disabled"`
			DisplayName    *string `json:"display_name"`
			RecordingLevel *string `json:"recording_level"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		newRole := target.Role
		if p.IsAdmin != nil && *p.IsAdmin != target.IsAdmin {
			newRole = map[bool]string{true: userRoleAdmin, false: userRoleUser}[*p.IsAdmin]
		}
		if p.Role != nil {
			rv, ok := validUserRole(*p.Role)
			if !ok {
				jsonError(w, "Unknown role (admin, auditor, operator or user)", 400)
				return
			}
			newRole = rv
		}
		privileged := newRole != target.Role || (p.Disabled != nil && *p.Disabled != target.Disabled) || (p.BreakGlass != nil && *p.BreakGlass != target.BreakGlass)
		if privileged && !requireStepUp(w, r, "user_update") {
			return
		}
		if p.BreakGlass != nil && *p.BreakGlass && newRole != userRoleAdmin {
			jsonError(w, "Only an administrator can be a break-glass account", 400)
			return
		}
		changes := map[string]interface{}{}
		if newRole != target.Role {
			if target.IsAdmin && newRole != userRoleAdmin && (id == adminID || lastAdmin) {
				jsonError(w, "You cannot remove admin rights from yourself or from the last administrator", 400)
				return
			}
			isAdmin := newRole == userRoleAdmin
			db.Exec(`UPDATE users SET is_admin=?, role=?, break_glass=CASE WHEN ?=1 THEN break_glass ELSE 0 END WHERE id=?`, boolToInt(isAdmin), storedRole(newRole), boolToInt(isAdmin), id)
			changes["role"] = newRole
			if isAdmin != target.IsAdmin {
				changes["is_admin"] = isAdmin
			}
			// Auditors cannot keep terminals or AI sessions open.
			if newRole == userRoleAuditor {
				disconnectUserEverywhere(id, "Your role changed to security auditor")
				aiKillWhere(func(s *aiSession) bool { return s.UserID == id }, adminID, "the account's role changed to security auditor")
			}
			hub.sendAll([]byte(`{"type":"ai_policy_changed"}`))
		}
		if p.BreakGlass != nil && *p.BreakGlass != target.BreakGlass {
			db.Exec(`UPDATE users SET break_glass=? WHERE id=?`, boolToInt(*p.BreakGlass), id)
			changes["break_glass"] = *p.BreakGlass
			if *p.BreakGlass && !target.TOTPEnabled {
				revokeUserSessions(id, 0) // the next sign-in must set up 2FA
			}
		}
		if p.Disabled != nil && *p.Disabled != target.Disabled {
			if *p.Disabled && (id == adminID || lastAdmin) {
				jsonError(w, "You cannot disable yourself or the last administrator", 400)
				return
			}
			db.Exec(`UPDATE users SET disabled=? WHERE id=?`, boolToInt(*p.Disabled), id)
			changes["disabled"] = *p.Disabled
			if *p.Disabled {
				for k, v := range endUserAccess(r, id, adminID, "the account was disabled") {
					changes[k] = v
				}
			}
		}
		if p.DisplayName != nil {
			db.Exec(`UPDATE users SET display_name=? WHERE id=?`, truncateStr(strings.TrimSpace(*p.DisplayName), 80), id)
			changes["display_name"] = *p.DisplayName
		}
		if p.RecordingLevel != nil {
			lv := strings.TrimSpace(*p.RecordingLevel)
			if !validRecordingLevel(lv) {
				jsonError(w, "Recording level must be empty (server settings), none, metadata or input", 400)
				return
			}
			db.Exec(`UPDATE users SET recording_level=? WHERE id=?`, lv, id)
			changes["recording_level"] = lv
		}
		auditLog(r, adminID, "admin.user_updated", target.Username, changes)
		jsonOK(w, map[string]bool{"ok": true})
	case action == "stop" && r.Method == http.MethodPost:
		stopUser(w, r, adminID, target)
	case action == "handover" && r.Method == http.MethodGet:
		to, why := suggestedHandover(id)
		jsonOK(w, map[string]interface{}{"owned": ownedCounts(id), "suggested_user_id": to, "suggested_username": usernameOf(to), "suggested_reason": why, "teams": userTeams(id)})
	case action == "" && r.Method == http.MethodDelete:
		if id == adminID || lastAdmin {
			jsonError(w, "You cannot delete your own account or the last administrator", 400)
			return
		}
		if !requireStepUp(w, r, "user_delete") {
			return
		}
		details := map[string]interface{}{}
		if to := atoiOr(r.URL.Query().Get("transfer_to"), 0); to > 0 {
			if to == id || userRole(to) == "" || userRole(to) == userRoleAuditor {
				jsonError(w, "Choose an active account (not a security auditor) to receive the connections", 400)
				return
			}
			moved, err := handOver(id, to)
			if err != nil {
				jsonError(w, "Handover failed: "+err.Error(), 500)
				return
			}
			details["transferred_to"], details["moved"] = usernameOf(to), moved
			auditLog(r, adminID, "admin.user_handover", target.Username, details)
		}
		deleteUserCompletely(id)
		auditLog(r, adminID, "admin.user_deleted", target.Username, details)
		jsonOK(w, map[string]interface{}{"ok": true, "moved": details["moved"]})
	case action == "reset-password" && r.Method == http.MethodPost:
		if !requireStepUp(w, r, "user_reset_password") {
			return
		}
		var p struct {
			Password   string `json:"password"`
			MustChange *bool  `json:"must_change_password"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		generated := ""
		if p.Password == "" {
			generated = randomPassword()
			p.Password = generated
		} else if err := validatePassword(p.Password); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		mustChange := p.MustChange == nil || *p.MustChange
		hash, _ := hashPassword(p.Password)
		db.Exec(`UPDATE users SET password_hash=?, must_change_password=?, failed_logins=0, locked_until='', password_changed_at=?, sso_only=0 WHERE id=?`,
			hash, boolToInt(mustChange), time.Now().UTC().Format(time.RFC3339), id)
		revokeUserSessions(id, 0)
		auditLog(r, adminID, "admin.user_password_reset", target.Username, map[string]bool{"must_change_password": mustChange})
		jsonOK(w, map[string]interface{}{"ok": true, "generated_password": generated})
	case action == "reset-2fa" && r.Method == http.MethodPost:
		if !requireStepUp(w, r, "user_reset_2fa") {
			return
		}
		db.Exec(`UPDATE users SET totp_secret='', totp_enabled=0, totp_pending='', recovery_codes='' WHERE id=?`, id)
		revokeUserSessions(id, 0)
		auditLog(r, adminID, "admin.user_2fa_reset", target.Username, nil)
		jsonOK(w, map[string]bool{"ok": true})
	case action == "revoke-sessions" && r.Method == http.MethodPost:
		n := revokeUserSessions(id, 0)
		disconnectUserEverywhere(id, "An administrator signed you out")
		auditLog(r, adminID, "admin.user_sessions_revoked", target.Username, map[string]int64{"count": n})
		jsonOK(w, map[string]interface{}{"ok": true, "revoked": n})
	case action == "unlock" && r.Method == http.MethodPost:
		db.Exec(`UPDATE users SET failed_logins=0, locked_until='' WHERE id=?`, id)
		auditLog(r, adminID, "admin.user_unlocked", target.Username, nil)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// deleteUserCompletely removes an account with everything it owns (connections with their
// encrypted secrets, folders, sessions, shares) and disconnects it everywhere.
func deleteUserCompletely(id int) {
	disconnectUserEverywhere(id, "Your account was deleted")
	aiKillWhere(func(s *aiSession) bool { return s.UserID == id }, 0, "the account was deleted")
	var shareIDs []int
	if rows, err := db.Query(`SELECT id FROM share_links WHERE owner_id=?`, id); err == nil {
		for rows.Next() {
			var sid int
			rows.Scan(&sid)
			shareIDs = append(shareIDs, sid)
		}
		rows.Close()
	}
	for _, sid := range shareIDs {
		closeShareEverywhere(sid, "The share was removed")
	}
	tx, err := db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	tx.Exec(`DELETE FROM share_teams WHERE share_id IN (SELECT id FROM share_links WHERE owner_id=?)`, id)
	tx.Exec(`DELETE FROM team_members WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM share_links WHERE owner_id=?`, id)
	tx.Exec(`DELETE FROM sessions WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM ssh_key_deployments WHERE conn_id IN (SELECT id FROM connections WHERE user_id=?) OR key_id IN (SELECT id FROM ssh_keys WHERE user_id=?)`, id, id)
	tx.Exec(`DELETE FROM connections WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM folders WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM snippets WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM user_ui WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM user_events WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM bookmarks WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM connection_tunnels WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM ssh_keys WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM credential_grants WHERE user_id=? OR credential_id IN (SELECT id FROM credentials WHERE owner_id=?)`, id, id)
	tx.Exec(`DELETE FROM credentials WHERE owner_id=?`, id)
	tx.Exec(`DELETE FROM inventory_sources WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM ai_providers WHERE scope='user' AND user_id=?`, id)
	tx.Exec(`DELETE FROM proxy_grants WHERE user_id=? OR proxy_id IN (SELECT id FROM proxies WHERE owner_id=?)`, id, id)
	tx.Exec(`DELETE FROM proxies WHERE owner_id=?`, id)
	tx.Exec(`DELETE FROM putty_sessions WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM notify_subscriptions WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM notify_prefs WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM notify_pending WHERE user_id=?`, id)
	for _, t := range []string{"git_workspace", "git_targets", "git_installs", "git_runs", "git_pending_restarts", "git_feeds", "git_pipelines", "visual_labels", "monitor_profiles", "monitor_samples", "git_review_tickets", "git_review_manifests"} {
		tx.Exec(`DELETE FROM `+t+` WHERE user_id=?`, id)
	}
	tx.Exec(`DELETE FROM git_blobs WHERE source_id IN (SELECT id FROM git_sources WHERE user_id=?)`, id)
	tx.Exec(`DELETE FROM git_trees WHERE source_id IN (SELECT id FROM git_sources WHERE user_id=?)`, id)
	for _, table := range []string{"git_relay_cache", "git_relay_members", "git_relay_meta"} {
		tx.Exec(`DELETE FROM `+table+` WHERE source_id IN(SELECT id FROM git_sources WHERE user_id=?)`, id)
	}
	tx.Exec(`DELETE FROM git_relay_members WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM git_sources WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM auth_sessions WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM user_identities WHERE user_id=?`, id)
	tx.Exec(`DELETE FROM users WHERE id=?`, id)
	tx.Commit()
}

// disconnectUserEverywhere closes the user's terminals and collaboration connections.
// disconnectUserEverywhere ends the user's terminals, room seats and tunnels and says how many.
func disconnectUserEverywhere(userID int, reason string) (terminals, rooms, tunnels int) {
	terminals = killTerminals(func(t *termSession) bool { return t.UserID == userID }, reason)
	rooms = kickUserFromRooms(userID, reason)
	tunnels = tunnelMgr.stopUser(userID, reason)
	return
}

// resetPasswordCLI implements "wrm -reset-password <username>" for locked-out admins.
func resetPasswordCLI(username string, reset2FA bool) error {
	u, err := loadUser("username=?", username)
	if err == sql.ErrNoRows || u == nil {
		return fmt.Errorf("user %q not found", username)
	}
	if err != nil {
		return err
	}
	pw := randomPassword()
	hash, err := hashPassword(pw)
	if err != nil {
		return err
	}
	db.Exec(`UPDATE users SET password_hash=?, must_change_password=1, failed_logins=0, locked_until='', disabled=0 WHERE id=?`, hash, u.ID)
	if reset2FA {
		db.Exec(`UPDATE users SET totp_secret='', totp_enabled=0, totp_pending='', recovery_codes='' WHERE id=?`, u.ID)
	}
	revokeUserSessions(u.ID, 0)
	auditLogAs(nil, 0, "cli", "admin.user_password_reset", u.Username, map[string]bool{"cli": true, "reset_2fa": reset2FA})
	fmt.Printf("New temporary password for %s: %s\n(The user must change it after signing in.)\n", u.Username, pw)
	return nil
}
