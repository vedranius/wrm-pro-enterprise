// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// ─── WEBATHN / FIDO2 (RFC 0001) ──────────────────────────────────────
// Passkeys and security keys as a second factor at sign-in and step-up, alongside TOTP.
// The RP ID defaults to the request host; override it with the webauthn_rpid setting.
// See docs/rfcs/0001-webauthn.md for the design.

const webauthnDisplayName = "Web Remote Manager"

func migrateWebAuthn() {
	mustExec(`CREATE TABLE IF NOT EXISTS webauthn_credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		credential_id TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		credential TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT '',
		UNIQUE(user_id, credential_id)
	)`)
	mustExec(`CREATE INDEX IF NOT EXISTS idx_webauthn_user ON webauthn_credentials(user_id)`)
}

// webauthnRPID returns the relying-party ID for this request: the host without a port,
// or the administrator's override (webauthn_rpid) when set.
func webauthnRPID(r *http.Request) string {
	if v := strings.TrimSpace(getSetting("webauthn_rpid")); v != "" {
		return v
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.Trim(host, "[]")
}

func webauthnOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || secureTransport(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func newWebAuthn(r *http.Request) (*webauthn.WebAuthn, error) {
	return webauthn.New(&webauthn.Config{
		RPDisplayName: webauthnDisplayName,
		RPID:          webauthnRPID(r),
		RPOrigins:     []string{webauthnOrigin(r)},
	})
}

// webauthnUser adapts a userRow to the webauthn.User interface.
type webauthnUser struct {
	u     *userRow
	creds []webauthn.Credential
}

func (w *webauthnUser) WebAuthnID() []byte { return []byte(strconv.Itoa(w.u.ID)) }
func (w *webauthnUser) WebAuthnName() string {
	return w.u.Username
}
func (w *webauthnUser) WebAuthnDisplayName() string {
	if w.u.DisplayName != "" {
		return w.u.DisplayName
	}
	return w.u.Username
}
func (w *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return w.creds }

func loadWebAuthnCredentials(userID int) []webauthn.Credential {
	rows, err := db.Query(`SELECT credential FROM webauthn_credentials WHERE user_id=? ORDER BY id`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []webauthn.Credential
	for rows.Next() {
		var j string
		if rows.Scan(&j) != nil {
			continue
		}
		var c webauthn.Credential
		if json.Unmarshal([]byte(j), &c) == nil {
			out = append(out, c)
		}
	}
	return out
}

func newWebAuthnUser(u *userRow) *webauthnUser {
	return &webauthnUser{u: u, creds: loadWebAuthnCredentials(u.ID)}
}

func userHasPasskey(userID int) bool {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM webauthn_credentials WHERE user_id=?`, userID).Scan(&n)
	return n > 0
}

func storeWebAuthnCredential(userID int, name string, c *webauthn.Credential) error {
	j, err := json.Marshal(c)
	if err != nil {
		return err
	}
	id := base64.StdEncoding.EncodeToString(c.ID)
	_, err = db.Exec(`INSERT INTO webauthn_credentials (user_id, credential_id, name, credential, created_at) VALUES (?,?,?,?,?)`,
		userID, id, name, string(j), time.Now().UTC().Format(time.RFC3339))
	return err
}

// updateWebAuthnCredential persists the credential after an assertion, so the signature
// counter (Authenticator.SignCount) advances and a replayed assertion is detected.
func updateWebAuthnCredential(userID int, c *webauthn.Credential) error {
	j, err := json.Marshal(c)
	if err != nil {
		return err
	}
	id := base64.StdEncoding.EncodeToString(c.ID)
	_, err = db.Exec(`UPDATE webauthn_credentials SET credential=? WHERE user_id=? AND credential_id=?`,
		string(j), userID, id)
	return err
}

// In-memory ceremony state (begin → complete), like mfaAttempts.
var webauthnCeremonies = struct {
	sync.Mutex
	m map[string]webauthn.SessionData
}{m: map[string]webauthn.SessionData{}}

func webauthnStoreSession(s webauthn.SessionData) string {
	tok := randomToken(24)
	webauthnCeremonies.Lock()
	webauthnCeremonies.m[tok] = s
	if len(webauthnCeremonies.m) > 10000 {
		webauthnCeremonies.m = map[string]webauthn.SessionData{}
	}
	webauthnCeremonies.Unlock()
	return tok
}

func webauthnTakeSession(tok string) (webauthn.SessionData, bool) {
	webauthnCeremonies.Lock()
	defer webauthnCeremonies.Unlock()
	s, ok := webauthnCeremonies.m[tok]
	delete(webauthnCeremonies.m, tok)
	if !ok || time.Now().After(s.Expires) {
		return webauthn.SessionData{}, false
	}
	return s, true
}

// POST /api/auth/webauthn/register/begin — requires a fresh step-up confirmation.
func apiAuthWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	s, err := lookupSession(r)
	if err != nil || s.Restricted != "" {
		jsonError(w, "Unauthorized", 401)
		return
	}
	if !requireStepUpAlways(w, r, "add a passkey") {
		return // 403 {step_up: true} written by requireStepUpAlways
	}
	u, err := loadUser("id=?", s.UserID)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return
	}
	wa, err := newWebAuthn(r)
	if err != nil {
		jsonError(w, "WebAuthn is not available at this address (use a hostname or set webauthn_rpid): "+err.Error(), 500)
		return
	}
	wu := newWebAuthnUser(u)
	creation, session, err := wa.BeginRegistration(wu,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationRequired,
		}),
		webauthn.WithExclusions(webauthn.Credentials(wu.creds).CredentialDescriptors()),
	)
	if err != nil {
		jsonError(w, "WebAuthn registration failed to start: "+err.Error(), 500)
		return
	}
	jsonOK(w, map[string]interface{}{"options": creation.Response, "webauthn_token": webauthnStoreSession(*session)})
}

// POST /api/auth/webauthn/register/complete {webauthn_token, name, credential}
func apiAuthWebAuthnRegisterComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	s, err := lookupSession(r)
	if err != nil || s.Restricted != "" {
		jsonError(w, "Unauthorized", 401)
		return
	}
	var in struct {
		Token      string          `json:"webauthn_token"`
		Name       string          `json:"name"`
		Credential json.RawMessage `json:"credential"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	session, ok := webauthnTakeSession(in.Token)
	if !ok {
		jsonError(w, "The passkey registration expired. Start again.", 400)
		return
	}
	u, err := loadUser("id=?", s.UserID)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return
	}
	wa, err := newWebAuthn(r)
	if err != nil {
		jsonError(w, "WebAuthn is not available at this address: "+err.Error(), 500)
		return
	}
	req := &http.Request{Body: io.NopCloser(bytes.NewReader(in.Credential))}
	cred, err := wa.FinishRegistration(newWebAuthnUser(u), session, req)
	if err != nil {
		recordLoginResult(clientIP(r), false)
		auditLogAs(r, u.ID, u.Username, "auth.webauthn_register_failed", "", map[string]string{"reason": err.Error()})
		jsonError(w, "Passkey registration failed: "+err.Error(), 400)
		return
	}
	if err := storeWebAuthnCredential(u.ID, strings.TrimSpace(in.Name), cred); err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	auditLogAs(r, u.ID, u.Username, "auth.webauthn_registered", "", map[string]int{"total": len(loadWebAuthnCredentials(u.ID))})
	jsonOK(w, map[string]interface{}{"registered": true, "total": len(loadWebAuthnCredentials(u.ID))})
}

// POST /api/auth/webauthn/login/begin {mfa_token}
func apiAuthWebAuthnLoginBegin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var in struct {
		Token string `json:"mfa_token"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	uid, _, ok := parseMFAToken(in.Token)
	if !ok {
		jsonError(w, "The sign-in step expired. Enter your password again.", 401)
		return
	}
	u, err := loadUser("id=?", uid)
	if err != nil || u.Disabled || !userHasPasskey(u.ID) {
		jsonError(w, "Invalid sign-in", 401)
		return
	}
	wa, err := newWebAuthn(r)
	if err != nil {
		jsonError(w, "WebAuthn is not available at this address: "+err.Error(), 500)
		return
	}
	assertion, session, err := wa.BeginLogin(newWebAuthnUser(u), webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		jsonError(w, "WebAuthn sign-in failed to start: "+err.Error(), 500)
		return
	}
	jsonOK(w, map[string]interface{}{"options": assertion.Response, "webauthn_token": webauthnStoreSession(*session)})
}

// POST /api/auth/webauthn/login/complete {mfa_token, webauthn_token, credential}
func apiAuthWebAuthnLoginComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
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
	var in struct {
		MFAToken      string          `json:"mfa_token"`
		WebAuthnToken string          `json:"webauthn_token"`
		Credential    json.RawMessage `json:"credential"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	uid, nonce, ok := parseMFAToken(in.MFAToken)
	if !ok {
		jsonError(w, "The sign-in step expired. Enter your password again.", 401)
		return
	}
	mfaAttempts.Lock()
	mfaAttempts.m[nonce]++
	attempts := mfaAttempts.m[nonce]
	mfaAttempts.Unlock()
	if attempts > 5 {
		jsonError(w, "Too many wrong attempts. Enter your password again.", 401)
		return
	}
	session, ok := webauthnTakeSession(in.WebAuthnToken)
	if !ok {
		jsonError(w, "The passkey sign-in expired. Start again.", 400)
		return
	}
	u, err := loadUser("id=?", uid)
	if err != nil || u.Disabled || !userHasPasskey(u.ID) {
		jsonError(w, "Invalid sign-in", 401)
		return
	}
	wa, err := newWebAuthn(r)
	if err != nil {
		jsonError(w, "WebAuthn is not available at this address: "+err.Error(), 500)
		return
	}
	req := &http.Request{Body: io.NopCloser(bytes.NewReader(in.Credential))}
	cred, err := wa.FinishLogin(newWebAuthnUser(u), session, req)
	if err != nil {
		recordLoginResult(ip, false)
		auditLogAs(r, u.ID, u.Username, "auth.login_failed", "", map[string]string{"reason": "webauthn", "detail": err.Error()})
		jsonError(w, "Passkey verification failed", 401)
		return
	}
	if err := updateWebAuthnCredential(u.ID, cred); err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	recordLoginResult(ip, true)
	finishLogin(w, r, u, "webauthn")
}

// POST /api/auth/step-up/webauthn/begin — start a WebAuthn assertion for step-up.
func apiAuthStepUpWebAuthnBegin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	s, err := lookupSession(r)
	if err != nil || s.Restricted != "" {
		jsonError(w, "Unauthorized", 401)
		return
	}
	u, err := loadUser("id=?", s.UserID)
	if err != nil || !userHasPasskey(u.ID) {
		jsonError(w, "No passkey registered", 400)
		return
	}
	wa, err := newWebAuthn(r)
	if err != nil {
		jsonError(w, "WebAuthn is not available at this address: "+err.Error(), 500)
		return
	}
	assertion, session, err := wa.BeginLogin(newWebAuthnUser(u), webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		jsonError(w, "WebAuthn step-up failed to start: "+err.Error(), 500)
		return
	}
	jsonOK(w, map[string]interface{}{"options": assertion.Response, "webauthn_token": webauthnStoreSession(*session)})
}

// POST /api/auth/step-up/webauthn/complete {password, webauthn_token, credential, action}
func apiAuthStepUpWebAuthnComplete(w http.ResponseWriter, r *http.Request) {
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
		Password      string          `json:"password"`
		WebAuthnToken string          `json:"webauthn_token"`
		Action        string          `json:"action"`
		Credential    json.RawMessage `json:"credential"`
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
	session, ok := webauthnTakeSession(in.WebAuthnToken)
	if !ok {
		jsonError(w, "The passkey step-up expired. Start again.", 400)
		return
	}
	wa, err := newWebAuthn(r)
	if err != nil {
		jsonError(w, "WebAuthn is not available at this address: "+err.Error(), 500)
		return
	}
	req := &http.Request{Body: io.NopCloser(bytes.NewReader(in.Credential))}
	cred, err := wa.FinishLogin(newWebAuthnUser(u), session, req)
	if err != nil {
		fail("webauthn", "Passkey verification failed")
		return
	}
	if err := updateWebAuthnCredential(u.ID, cred); err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	recordLoginResult(ip, true)
	db.Exec(`UPDATE auth_sessions SET step_up_at=? WHERE id=?`, nowRFC(), s.ID)
	auditLogAs(r, u.ID, u.Username, "auth.step_up", action, map[string]string{"method": "password+webauthn"})
	jsonOK(w, map[string]interface{}{"ok": true, "minutes": settingInt("step_up_minutes")})
}
