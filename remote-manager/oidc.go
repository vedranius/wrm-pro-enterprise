// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── OIDC SIGN-IN ────────────────────────────────────
//
// Companies sign in with their directory (Microsoft Entra ID, ADFS, Okta, Keycloak, Google
// Workspace, any OpenID Connect provider) using the authorization code flow with PKCE.
//
//   - A directory user is keyed by an immutable ID, never by name: Entra's tenant + object ID
//     (tid + oid) when the token carries them, otherwise issuer + subject.
//   - Mapping rules turn app roles, group IDs or any other claim into a WRM role and teams; the
//     strongest matched role wins (Administrator > Security auditor > Operator > User). Accounts
//     are created on first sign-in only for users a rule matches; others are refused with a
//     message that says why. Role and SSO team memberships follow the directory at every sign-in.
//   - A provider can require MFA proven by the token (amr, or the acr / Entra acrs values).
//   - Directory sign-ins are revalidated: with a refresh token WRM asks the provider every
//     oidc_revalidate_minutes whether the user may still sign in; a refused refresh (account
//     disabled, password reset, consent revoked) ends the user's sessions and terminals. Without a
//     successful check for oidc_unverified_hours the sessions end too.
//   - local_login=break_glass limits password sign-in to break-glass administrators once a
//     provider is enabled.
//
// The client secret and refresh tokens are encrypted at rest and never sent to the browser.

type oidcRule struct {
	Claim string   `json:"claim"`
	Value string   `json:"value"`
	Role  string   `json:"role"`
	Teams []string `json:"teams"`
}

type oidcProvider struct {
	ID            int        `json:"id"`
	Name          string     `json:"name"`
	Issuer        string     `json:"issuer"`
	ClientID      string     `json:"client_id"`
	secret        string     // decrypted client secret ("" for a public client)
	HasSecret     bool       `json:"has_secret"`
	Scopes        string     `json:"scopes"`
	RedirectURI   string     `json:"redirect_uri"`
	UsernameClaim string     `json:"username_claim"`
	RequireMFA    bool       `json:"require_mfa"`
	MFAValues     string     `json:"mfa_values"` // accepted acr / acrs values, comma separated
	JIT           bool       `json:"jit"`
	Rules         []oidcRule `json:"rules"`
	Enabled       bool       `json:"enabled"`
	CreatedAt     string     `json:"created_at"`
	UpdatedAt     string     `json:"updated_at"`
}

const oidcDefaultScopes = "openid profile email offline_access"

const oidcProviderCols = `id, name, issuer, client_id, client_secret, scopes, redirect_uri, username_claim, require_mfa, mfa_values, jit, rules, enabled, created_at, updated_at`

func scanOIDCProvider(sc interface{ Scan(...interface{}) error }) (*oidcProvider, error) {
	var p oidcProvider
	var secret, rules string
	var mfa, jit, enabled int
	if err := sc.Scan(&p.ID, &p.Name, &p.Issuer, &p.ClientID, &secret, &p.Scopes, &p.RedirectURI, &p.UsernameClaim, &mfa, &p.MFAValues, &jit, &rules, &enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	if secret != "" {
		p.secret = decryptValue(secret)
		p.HasSecret = true
	}
	p.RequireMFA, p.JIT, p.Enabled = mfa == 1, jit == 1, enabled == 1
	json.Unmarshal([]byte(rules), &p.Rules)
	if p.Rules == nil {
		p.Rules = []oidcRule{}
	}
	return &p, nil
}

func loadOIDCProvider(id int) (*oidcProvider, error) {
	return scanOIDCProvider(db.QueryRow(`SELECT `+oidcProviderCols+` FROM oidc_providers WHERE id=?`, id))
}

func oidcProviders(enabledOnly bool) []*oidcProvider {
	q := `SELECT ` + oidcProviderCols + ` FROM oidc_providers`
	if enabledOnly {
		q += ` WHERE enabled=1`
	}
	rows, err := db.Query(q + ` ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*oidcProvider
	for rows.Next() {
		if p, err := scanOIDCProvider(rows); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func oidcAnyEnabled() bool {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM oidc_providers WHERE enabled=1`).Scan(&n)
	return n > 0
}

// localLoginLimited: password sign-in only for break-glass administrators. It takes effect only
// while a provider is enabled, so nobody is locked out of a WRM without directory sign-in.
func localLoginLimited() bool {
	return getSetting("local_login") == "break_glass" && (oidcAnyEnabled() || ldapAnyEnabled())
}

func oidcRedirectURI(p *oidcProvider, r *http.Request) string {
	if p != nil && p.RedirectURI != "" {
		return p.RedirectURI
	}
	return mcpBaseURL(r) + "/api/auth/oidc/callback"
}

func oidcIsEntra(issuer string) bool {
	u, err := url.Parse(issuer)
	return err == nil && (u.Hostname() == "login.microsoftonline.com" || strings.HasSuffix(u.Hostname(), ".microsoftonline.com") || u.Hostname() == "login.microsoftonline.us")
}

// ─── mapping ─────────────────────────────────────────

var oidcRoleRank = map[string]int{userRoleUser: 1, userRoleOperator: 2, userRoleAuditor: 3, userRoleAdmin: 4}

type oidcMapResult struct {
	Matched bool     `json:"matched"`
	Role    string   `json:"role"`
	Teams   []string `json:"teams"`
	Rules   []int    `json:"rules"` // indexes of the matched rules
}

func oidcClaimHas(c oidcClaims, claim, value string) bool {
	for _, v := range c.list(claim) {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func oidcMap(p *oidcProvider, c oidcClaims) oidcMapResult {
	return directoryMap(p.Rules, c)
}

// directoryMap applies mapping rules to a user's claims (OIDC claims, LDAP attributes and
// groups, or SCIM groups): the strongest matched role wins, teams add up.
func directoryMap(rules []oidcRule, c oidcClaims) oidcMapResult {
	res := oidcMapResult{Teams: []string{}, Rules: []int{}}
	seen := map[string]bool{}
	for i, rule := range rules {
		if rule.Value == "" || !oidcClaimHas(c, rule.Claim, rule.Value) {
			continue
		}
		res.Matched = true
		res.Rules = append(res.Rules, i)
		if oidcRoleRank[rule.Role] > oidcRoleRank[res.Role] {
			res.Role = rule.Role
		}
		for _, t := range rule.Teams {
			if k := strings.ToLower(t); t != "" && !seen[k] {
				seen[k] = true
				res.Teams = append(res.Teams, t)
			}
		}
	}
	if res.Matched && res.Role == "" {
		res.Role = userRoleUser
	}
	return res
}

func validateOIDCRules(rules []oidcRule) ([]oidcRule, error) {
	out := []oidcRule{}
	for i, r := range rules {
		r.Claim, r.Value = strings.TrimSpace(r.Claim), strings.TrimSpace(r.Value)
		if r.Claim == "" && r.Value == "" {
			continue
		}
		if r.Claim == "" || r.Value == "" || len(r.Claim) > 100 || len(r.Value) > 300 {
			return nil, fmt.Errorf("rule %d: enter a claim (for example roles or groups) and a value", i+1)
		}
		role, ok := validUserRole(r.Role)
		if !ok {
			return nil, fmt.Errorf("rule %d: unknown role (admin, auditor, operator or user)", i+1)
		}
		r.Role = role
		teams := []string{}
		for _, t := range r.Teams {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			var name string
			if db.QueryRow(`SELECT name FROM teams WHERE name=? COLLATE NOCASE`, t).Scan(&name) != nil {
				return nil, fmt.Errorf("rule %d: unknown team %q (create it in Admin panel → Teams first)", i+1, t)
			}
			teams = append(teams, name)
		}
		r.Teams = teams
		out = append(out, r)
	}
	if len(out) > 200 {
		return nil, errors.New("at most 200 rules")
	}
	return out, nil
}

// oidcMFAProven reports whether the token shows a multi-factor sign-in.
func oidcMFAProven(p *oidcProvider, c oidcClaims) bool {
	for _, v := range c.list("amr") {
		switch strings.ToLower(v) {
		case "mfa", "otp", "hwk", "swk", "fpt", "face", "iris", "retina", "vbm", "sc", "pop":
			return true
		}
	}
	accepted := splitList(p.MFAValues)
	for _, want := range accepted {
		if c.str("acr") == want || oidcClaimHas(c, "acrs", want) {
			return true
		}
	}
	return false
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// oidcSubject is the immutable key of a directory user.
func oidcSubject(c oidcClaims) string {
	if tid, oid := c.str("tid"), c.str("oid"); tid != "" && oid != "" {
		return "entra:" + tid + ":" + oid
	}
	return c.str("iss") + "|" + c.str("sub")
}

func oidcGroupsOverage(c oidcClaims) bool {
	names, ok := c["_claim_names"].(map[string]interface{})
	if !ok {
		return false
	}
	_, has := names["groups"]
	return has
}

// ─── pending sign-ins ────────────────────────────────

type oidcPending struct {
	ProviderID  int
	Mode        string // login, link, stepup, test
	Verifier    string
	Nonce       string
	RedirectURI string
	UserID      int
	SessionID   int
	Created     time.Time
}

type oidcResult struct {
	UserID int
	At     time.Time
	Body   map[string]interface{}
}

var oidcState = struct {
	sync.Mutex
	pending map[string]*oidcPending
	results map[string]*oidcResult
}{pending: map[string]*oidcPending{}, results: map[string]*oidcResult{}}

const oidcPendingTTL = 10 * time.Minute

func oidcCookieName(state string) string { return "wrm_oidc_" + state[:10] }

func oidcPrune() {
	for k, v := range oidcState.pending {
		if time.Since(v.Created) > oidcPendingTTL {
			delete(oidcState.pending, k)
		}
	}
	for k, v := range oidcState.results {
		if time.Since(v.At) > oidcPendingTTL {
			delete(oidcState.results, k)
		}
	}
}

// oidcAuthURL prepares a sign-in at the provider and returns the address to send the browser to.
func oidcAuthURL(w http.ResponseWriter, r *http.Request, p *oidcProvider, mode string, userID, sessionID int) (string, string, error) {
	d, err := oidcDiscover(p.Issuer, false)
	if err != nil {
		return "", "", err
	}
	state, nonce, verifier := randomToken(24), randomToken(24), randomToken(48)
	redirect := oidcRedirectURI(p, r)
	oidcState.Lock()
	oidcPrune()
	if len(oidcState.pending) > 5000 {
		oidcState.Unlock()
		return "", "", errors.New("too many sign-ins in progress, try again in a few minutes")
	}
	oidcState.pending[state] = &oidcPending{ProviderID: p.ID, Mode: mode, Verifier: verifier, Nonce: nonce, RedirectURI: redirect, UserID: userID, SessionID: sessionID, Created: time.Now()}
	oidcState.Unlock()
	setCookie(w, r, oidcCookieName(state), state, int(oidcPendingTTL.Seconds()))
	scopes := strings.TrimSpace(p.Scopes)
	if scopes == "" {
		scopes = oidcDefaultScopes
	}
	if !strings.Contains(" "+scopes+" ", " openid ") {
		scopes = "openid " + scopes
	}
	q := url.Values{
		"response_type": {"code"}, "client_id": {p.ClientID}, "redirect_uri": {redirect}, "scope": {scopes},
		"state": {state}, "nonce": {nonce}, "code_challenge": {pkceS256(verifier)}, "code_challenge_method": {"S256"},
	}
	if mode == "stepup" {
		q.Set("prompt", "login")
		q.Set("max_age", "0")
	}
	if mode == "test" {
		q.Set("prompt", "select_account")
	}
	if vals := splitList(p.MFAValues); p.RequireMFA && len(vals) > 0 {
		if oidcIsEntra(p.Issuer) {
			// Entra asks for a Conditional Access authentication context through the claims request.
			claims, _ := json.Marshal(map[string]interface{}{"id_token": map[string]interface{}{"acrs": map[string]interface{}{"essential": true, "values": vals}}})
			q.Set("claims", string(claims))
		} else {
			q.Set("acr_values", strings.Join(vals, " "))
		}
	}
	return withQuery(d.AuthorizationEndpoint, q), state, nil
}

type oidcTokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// errOIDCRefused: the provider refused the grant (for a refresh token: the user may no longer sign in).
var errOIDCRefused = errors.New("refused by the identity provider")

func oidcTokenRequest(p *oidcProvider, d *oidcDiscovery, form url.Values) (*oidcTokenResponse, error) {
	basic := false
	if p.secret != "" {
		post, basicOK := false, len(d.TokenAuthMethods) == 0
		for _, m := range d.TokenAuthMethods {
			post = post || m == "client_secret_post"
			basicOK = basicOK || m == "client_secret_basic"
		}
		if post {
			form.Set("client_secret", p.secret)
		} else if basicOK {
			basic = true
		} else {
			form.Set("client_secret", p.secret)
		}
	}
	form.Set("client_id", p.ClientID)
	req, err := http.NewRequest(http.MethodPost, d.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if basic {
		req.SetBasicAuth(url.QueryEscape(p.ClientID), url.QueryEscape(p.secret))
	}
	resp, err := oidcHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the token endpoint: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr oidcTokenResponse
	json.Unmarshal(body, &tr)
	if resp.StatusCode != 200 || tr.Error != "" {
		if tr.Error == "" {
			return nil, fmt.Errorf("the token endpoint answered HTTP %d", resp.StatusCode)
		}
		msg := tr.Error
		if tr.ErrorDesc != "" {
			msg += ": " + truncateStr(tr.ErrorDesc, 300)
		}
		if tr.Error == "invalid_grant" || tr.Error == "unauthorized_client" || tr.Error == "interaction_required" || tr.Error == "login_required" {
			return &tr, fmt.Errorf("%w (%s)", errOIDCRefused, msg)
		}
		return nil, errors.New(msg)
	}
	return &tr, nil
}

// ─── identities ──────────────────────────────────────

type oidcIdentity struct {
	ID         int
	UserID     int
	ProviderID int
	Subject    string
	Email      string
	Refresh    string
	LastOKAt   string
}

func loadOIDCIdentity(where string, args ...interface{}) (*oidcIdentity, error) {
	var x oidcIdentity
	var refresh string
	err := db.QueryRow(`SELECT id, user_id, provider_id, subject, email, refresh_token, last_ok_at FROM user_identities WHERE `+where, args...).
		Scan(&x.ID, &x.UserID, &x.ProviderID, &x.Subject, &x.Email, &refresh, &x.LastOKAt)
	if err != nil {
		return nil, err
	}
	if refresh != "" {
		x.Refresh = decryptValue(refresh)
	}
	return &x, nil
}

// oidcUsername picks a free WRM username for a new directory user.
func oidcUsername(p *oidcProvider, c oidcClaims) string {
	claim := p.UsernameClaim
	if claim == "" {
		claim = "preferred_username"
	}
	cands := []string{c.str(claim), c.str("preferred_username"), c.str("upn"), c.str("email"), c.str("name")}
	base := ""
	for _, v := range cands {
		v = strings.TrimSpace(v)
		var b strings.Builder
		for i, ch := range v {
			switch {
			case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
				b.WriteRune(ch)
			case i > 0 && strings.ContainsRune("._@-", ch):
				b.WriteRune(ch)
			case ch == ' ' && i > 0:
				b.WriteRune('.')
			}
		}
		if s := truncateStr(b.String(), 56); validUsername(s) == nil {
			base = s
			break
		}
	}
	if base == "" {
		base = "sso-" + truncateStr(sha256Hex(oidcSubject(c)), 10)
	}
	name := base
	for i := 2; i < 1000; i++ {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM users WHERE username=? COLLATE NOCASE`, name).Scan(&n)
		if n == 0 {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return base + "-" + randomToken(4)
}

// oidcApplyMapping sets the role and the SSO team memberships of a directory user.
func oidcApplyMapping(r *http.Request, userID int, m oidcMapResult) {
	directoryApplyMapping(r, userID, m, "oidc")
}

// directoryApplyMapping sets the role and the team memberships that come from one source
// (oidc, ldap or scim). Hand-added memberships and other sources stay as they are.
func directoryApplyMapping(r *http.Request, userID int, m oidcMapResult, source string) {
	u, err := loadUser("id=?", userID)
	if err != nil || u.BreakGlass {
		return
	}
	if m.Role != u.Role {
		if u.IsAdmin && m.Role != userRoleAdmin && activeAdminCount() <= 1 {
			auditLogAs(r, u.ID, u.Username, source+".role_kept", u.Username, map[string]string{"reason": "last active administrator", "directory_role": m.Role})
		} else {
			db.Exec(`UPDATE users SET is_admin=?, role=? WHERE id=?`, boolToInt(m.Role == userRoleAdmin), storedRole(m.Role), userID)
			auditLogAs(r, u.ID, u.Username, source+".role_synced", u.Username, map[string]string{"from": u.Role, "to": m.Role})
		}
	}
	want := map[int]bool{}
	for _, name := range m.Teams {
		var id int
		if db.QueryRow(`SELECT id FROM teams WHERE name=? COLLATE NOCASE`, name).Scan(&id) == nil {
			want[id] = true
		}
	}
	now := nowRFC()
	for id := range want {
		db.Exec(`INSERT OR IGNORE INTO team_members (team_id, user_id, lead, added_at, source) VALUES (?,?,0,?,?)`, id, userID, now, source)
	}
	rows, err := db.Query(`SELECT team_id FROM team_members WHERE user_id=? AND source=?`, userID, source)
	if err != nil {
		return
	}
	var drop []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		if !want[id] {
			drop = append(drop, id)
		}
	}
	rows.Close()
	for _, id := range drop {
		db.Exec(`DELETE FROM team_members WHERE team_id=? AND user_id=? AND source=?`, id, userID, source)
	}
	if len(drop) > 0 {
		revalidateTeamShares(sharesOfUser(userID))
	}
}

func sharesOfUser(userID int) []int {
	var ids []int
	rows, err := db.Query(`SELECT DISTINCT st.share_id FROM share_teams st JOIN team_members tm ON tm.team_id=st.team_id WHERE tm.user_id=?`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

// oidcEndIdentity ends a directory user's sessions and terminals.
func oidcEndIdentity(identityID, userID int, reason string) {
	endUserAccess(nil, userID, 0, reason)
	db.Exec(`UPDATE user_identities SET revoked=1 WHERE id=?`, identityID)
}

// ─── HTTP: sign-in ───────────────────────────────────

// GET /api/auth/oidc/start?provider=ID   full-page sign-in (redirects to the provider)
// POST /api/auth/oidc/start {provider, mode}  link | stepup | test (signed in; answers {url, state})
func apiOIDCStartHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if msg := signInTransportError(r); msg != "" {
			oauthPage(w, 403, "Sign-in needs HTTPS", msg)
			return
		}
		if !mcpRateOK("oidc:"+clientIP(r), 30, time.Minute) {
			oauthPage(w, 429, "Too many sign-ins", "Wait a minute and try again.")
			return
		}
		p, err := loadOIDCProvider(atoiOr(r.URL.Query().Get("provider"), 0))
		if err != nil || !p.Enabled {
			oauthPage(w, 404, "Sign-in not available", "This directory sign-in is not set up or is turned off. Ask your WRM administrator.")
			return
		}
		u, _, err := oidcAuthURL(w, r, p, "login", 0, 0)
		if err != nil {
			log.Printf("OIDC %s: %v", p.Name, err)
			oauthPage(w, 502, "Cannot reach the identity provider", "WRM could not read the sign-in settings of "+p.Name+": "+err.Error())
			return
		}
		http.Redirect(w, r, u, http.StatusFound)
	case http.MethodPost:
		s, err := lookupSession(r)
		if err != nil || s.Restricted != "" {
			jsonError(w, "Unauthorized", 401)
			return
		}
		var in struct {
			Provider int    `json:"provider"`
			Mode     string `json:"mode"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		p, err := loadOIDCProvider(in.Provider)
		if err != nil {
			jsonError(w, "Unknown identity provider", 404)
			return
		}
		switch in.Mode {
		case "test":
			if !isAdminUser(s.UserID) {
				jsonError(w, "Admin access required", 403)
				return
			}
		case "link":
			if !p.Enabled {
				jsonError(w, "This identity provider is turned off", 400)
				return
			}
			if u, err := loadUser("id=?", s.UserID); err != nil || u.BreakGlass {
				jsonError(w, "A break-glass account stays local; it cannot be linked to a directory account", 400)
				return
			}
			if _, err := loadOIDCIdentity(`user_id=? AND provider_id=?`, s.UserID, p.ID); err == nil {
				jsonError(w, "Your account is already linked to this identity provider; unlink it first", 400)
				return
			}
			// Linking adds a way to sign in, so a stolen session must not be enough.
			if !requireStepUpAlways(w, r, "oidc.link") {
				return
			}
		case "stepup":
			if !p.Enabled {
				jsonError(w, "This identity provider is turned off", 400)
				return
			}
			if _, err := loadOIDCIdentity(`user_id=? AND provider_id=?`, s.UserID, p.ID); err != nil {
				jsonError(w, "Your account is not linked to this identity provider", 400)
				return
			}
		default:
			jsonError(w, "Unknown mode", 400)
			return
		}
		u, state, err := oidcAuthURL(w, r, p, in.Mode, s.UserID, s.ID)
		if err != nil {
			jsonError(w, "Cannot reach the identity provider: "+err.Error(), 502)
			return
		}
		jsonOK(w, map[string]interface{}{"url": u, "state": state})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// GET /api/auth/oidc/result?state=  the outcome of a link / step-up / test window
func apiOIDCResultHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	state := r.URL.Query().Get("state")
	oidcState.Lock()
	res := oidcState.results[state]
	_, pending := oidcState.pending[state]
	if res != nil && res.UserID == userID {
		delete(oidcState.results, state)
	}
	oidcState.Unlock()
	if res == nil || res.UserID != userID {
		jsonOK(w, map[string]interface{}{"done": false, "pending": pending})
		return
	}
	out := map[string]interface{}{"done": true}
	for k, v := range res.Body {
		out[k] = v
	}
	jsonOK(w, out)
}

func oidcPopupPage(w http.ResponseWriter, ok bool, title, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	icon := "✅"
	if !ok {
		icon = "⚠"
	}
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>WRM PRO</title>
<style>body{font-family:system-ui,sans-serif;background:#0f1115;color:#e6e6e6;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;padding:16px;box-sizing:border-box}
div{max-width:520px;background:#181b22;border:1px solid #2a2f3a;border-radius:10px;padding:24px}h1{font-size:18px;margin:0 0 12px}p{color:#aab;line-height:1.5}</style></head>
<body><div><h1>%s %s</h1><p>%s</p><p>WRM PRO: you can close this window.</p></div>%s</body></html>`, icon, html.EscapeString(title), html.EscapeString(msg),
		map[bool]string{true: `<script>setTimeout(function(){window.close()},800)</script>`, false: ""}[ok])
}

// GET /api/auth/oidc/callback?code&state  (the provider sends the browser back here)
func apiOIDCCallbackHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	oidcState.Lock()
	pend := oidcState.pending[state]
	delete(oidcState.pending, state)
	oidcState.Unlock()
	if pend == nil || time.Since(pend.Created) > oidcPendingTTL {
		oauthPage(w, 400, "Sign-in expired", "This sign-in link is no longer valid. Start the sign-in again from WRM.")
		return
	}
	cookie, err := r.Cookie(oidcCookieName(state))
	setCookie(w, r, oidcCookieName(state), "", -1)
	if err != nil || cookie.Value != state {
		oauthPage(w, 400, "Sign-in not recognised", "This sign-in was not started in this browser. Start it again from WRM.")
		return
	}
	popup := pend.Mode != "login"
	finishPopup := func(ok bool, title, msg string, body map[string]interface{}) {
		if body == nil {
			body = map[string]interface{}{}
		}
		body["ok"], body["mode"] = ok, pend.Mode
		if !ok {
			body["error"] = msg
		}
		oidcState.Lock()
		oidcState.results[state] = &oidcResult{UserID: pend.UserID, At: time.Now(), Body: body}
		oidcState.Unlock()
		oidcPopupPage(w, ok, title, msg)
	}
	p, err := loadOIDCProvider(pend.ProviderID)
	if err != nil {
		oauthPage(w, 404, "Sign-in not available", "The identity provider was removed.")
		return
	}
	refuse := func(reason, msg string, c oidcClaims) {
		details := map[string]string{"provider": p.Name, "reason": reason, "mode": pend.Mode}
		if c != nil {
			details["subject"] = oidcSubject(c)
		}
		auditLogAs(r, pend.UserID, "", "auth.oidc_refused", p.Name, details)
		if popup {
			finishPopup(false, "Not confirmed", msg, nil)
			return
		}
		oauthPage(w, 403, "Sign-in refused", msg)
	}
	if e := q.Get("error"); e != "" {
		refuse("provider error: "+truncateStr(e, 80), "The identity provider did not sign you in: "+truncateStr(e+" "+q.Get("error_description"), 400), nil)
		return
	}
	d, err := oidcDiscover(p.Issuer, false)
	if err != nil {
		refuse("discovery", "WRM could not read the identity provider's settings: "+err.Error(), nil)
		return
	}
	tr, err := oidcTokenRequest(p, d, url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "redirect_uri": {pend.RedirectURI}, "code_verifier": {pend.Verifier}})
	if err != nil {
		refuse("token exchange", "The identity provider did not accept the sign-in code: "+err.Error(), nil)
		return
	}
	if tr.IDToken == "" {
		refuse("no id_token", "The identity provider returned no ID token: check that the scope contains openid.", nil)
		return
	}
	c, err := oidcVerifyIDToken(tr.IDToken, d, p.ClientID, pend.Nonce)
	if err != nil {
		refuse("id_token: "+err.Error(), "The identity provider's answer could not be verified: "+err.Error(), nil)
		return
	}
	subject := oidcSubject(c)
	m := oidcMap(p, c)
	mfaOK := oidcMFAProven(p, c)
	overage := oidcGroupsOverage(c)

	switch pend.Mode {
	case "test":
		keep := map[string]interface{}{}
		for _, k := range []string{"iss", "sub", "tid", "oid", "name", "preferred_username", "upn", "email", "amr", "acr", "acrs", "auth_time"} {
			if v, ok := c[k]; ok {
				keep[k] = v
			}
		}
		for _, rule := range p.Rules {
			if v, ok := c[rule.Claim]; ok {
				keep[rule.Claim] = v
			}
		}
		for _, k := range []string{"roles", "groups"} {
			if v, ok := c[k]; ok {
				keep[k] = v
			}
		}
		existing := ""
		if id, err := loadOIDCIdentity(`provider_id=? AND subject=?`, p.ID, subject); err == nil {
			existing = usernameOf(id.UserID)
		}
		auditLog(r, pend.UserID, "oidc.test_sign_in", p.Name, map[string]interface{}{"matched": m.Matched, "role": m.Role})
		finishPopup(true, "Test sign-in complete", "Back in WRM you see what the provider sent and what WRM would do.", map[string]interface{}{
			"claims": keep, "subject": subject, "mapping": m, "mfa": mfaOK, "mfa_required": p.RequireMFA, "groups_overage": overage,
			"refresh_token": tr.RefreshToken != "", "account": existing, "would_create": existing == "" && p.JIT && m.Matched,
			"username": oidcUsername(p, c),
		})
		return
	case "link":
		if p.RequireMFA && !mfaOK {
			refuse("mfa not proven", "The identity provider did not confirm a multi-factor sign-in, which this WRM requires.", c)
			return
		}
		if other, err := loadOIDCIdentity(`provider_id=? AND subject=?`, p.ID, subject); err == nil && other.UserID != pend.UserID {
			refuse("already linked", "This directory account is already linked to another WRM account ("+usernameOf(other.UserID)+").", c)
			return
		}
		if _, err := loadOIDCIdentity(`provider_id=? AND user_id=?`, p.ID, pend.UserID); err == nil {
			refuse("already linked", "Your WRM account is already linked to "+p.Name+"; unlink it first.", c)
			return
		}
		oidcSaveIdentity(pend.UserID, p.ID, subject, c, tr.RefreshToken)
		auditLog(r, pend.UserID, "oidc.identity_linked", p.Name, map[string]string{"subject": subject})
		finishPopup(true, "Directory account linked", "You can now sign in to WRM with "+p.Name+".", nil)
		return
	case "stepup":
		id, err := loadOIDCIdentity(`provider_id=? AND subject=?`, p.ID, subject)
		if err != nil || id.UserID != pend.UserID {
			refuse("other account", "You confirmed with a different directory account than the one linked to your WRM account.", c)
			return
		}
		if at := c.num("auth_time"); at > 0 && time.Since(time.Unix(at, 0)) > 10*time.Minute {
			refuse("stale auth_time", "The identity provider did not ask for your sign-in again. Try once more.", c)
			return
		}
		if p.RequireMFA && !mfaOK {
			refuse("mfa not proven", "The identity provider did not confirm a multi-factor sign-in, which this WRM requires.", c)
			return
		}
		db.Exec(`UPDATE auth_sessions SET step_up_at=? WHERE id=? AND user_id=?`, nowRFC(), pend.SessionID, pend.UserID)
		auditLog(r, pend.UserID, "auth.step_up", "", map[string]string{"method": "oidc", "provider": p.Name})
		finishPopup(true, "Confirmed", "You can continue in WRM.", nil)
		return
	}

	// mode login
	if !p.Enabled {
		refuse("provider off", "This directory sign-in is turned off.", c)
		return
	}
	if p.RequireMFA && !mfaOK {
		refuse("mfa not proven", "Your organisation's sign-in did not confirm a second factor (MFA), which this WRM requires. Sign in again with MFA or ask your administrator.", c)
		return
	}
	hasRules := len(p.Rules) > 0
	id, idErr := loadOIDCIdentity(`provider_id=? AND subject=?`, p.ID, subject)
	if hasRules && !m.Matched {
		msg := "Your directory account is not assigned to WRM: no app role or group of yours is allowed here. Ask your administrator to assign you to WRM."
		if overage {
			msg += " (You are in too many groups for the token to list them; the administrator should map app roles instead of groups.)"
		}
		if idErr == nil {
			oidcEndIdentity(id.ID, id.UserID, "Your directory account is no longer assigned to WRM")
		}
		refuse("no mapping rule matched", msg, c)
		return
	}
	var userID int
	if idErr == nil {
		userID = id.UserID
	} else if uid := scimUserForOIDC(c); uid > 0 {
		// An account provisioned by SCIM with this directory ID: its first sign-in links it.
		userID = uid
		auditLogAs(r, uid, usernameOf(uid), "oidc.scim_account_linked", usernameOf(uid), map[string]string{"provider": p.Name, "subject": subject})
	} else {
		if !p.JIT || !hasRules {
			refuse("no linked account", "There is no WRM account for your directory account yet. Ask your administrator to create one or to allow automatic accounts.", c)
			return
		}
		username := oidcUsername(p, c)
		now := nowRFC()
		res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, role, display_name, created_at, password_changed_at, sso_only) VALUES (?,?,?,?,?,?,?,1)`,
			username, "!sso", boolToInt(m.Role == userRoleAdmin), storedRole(m.Role), truncateStr(c.str("name"), 100), now, now)
		if err != nil {
			refuse("create account: "+err.Error(), "WRM could not create your account. Ask your administrator.", c)
			return
		}
		nid, _ := res.LastInsertId()
		userID = int(nid)
		auditLogAs(r, userID, username, "oidc.account_created", username, map[string]string{"provider": p.Name, "role": m.Role, "subject": subject})
	}
	u, err := loadUser("id=?", userID)
	if err != nil {
		refuse("account missing", "Your WRM account no longer exists. Ask your administrator.", c)
		return
	}
	if u.Disabled {
		refuse("account disabled", "Your WRM account is disabled. Contact an administrator.", c)
		return
	}
	if u.BreakGlass {
		refuse("break-glass", "A break-glass account signs in with its local password only.", c)
		return
	}
	if hasRules {
		oidcApplyMapping(r, userID, m)
	}
	identityID := oidcSaveIdentity(userID, p.ID, subject, c, tr.RefreshToken)
	if u, err = loadUser("id=?", userID); err != nil {
		oauthPage(w, 500, "Server error", "Try again.")
		return
	}
	recordLoginResult(clientIP(r), true)
	// A second factor proven by the provider satisfies require_2fa; otherwise the local 2FA applies.
	if !(p.RequireMFA && mfaOK) && u.TOTPEnabled {
		http.Redirect(w, r, "/#sso-mfa="+url.QueryEscape(newMFATokenFor(u.ID, identityID)), http.StatusFound)
		return
	}
	if lockdownRefuses(u.ID) {
		auditLogAs(r, u.ID, u.Username, "auth.login_failed", "", map[string]string{"method": "oidc", "reason": "lockdown"})
		oauthPage(w, 403, "WRM is in lockdown", lockdownMessage)
		return
	}
	restricted := restrictionFor(u)
	if p.RequireMFA && mfaOK && restricted == "mfa" {
		restricted = ""
	}
	if err := createOIDCSession(w, r, u, restricted, identityID, p.Name); err != nil {
		oauthPage(w, 500, "Server error", "Try again.")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func createOIDCSession(w http.ResponseWriter, r *http.Request, u *userRow, restricted string, identityID int, provider string) error {
	if err := createAuthSession(w, r, u.ID, restricted); err != nil {
		return err
	}
	// The newest session of this user is the one just created.
	db.Exec(`UPDATE auth_sessions SET identity_id=? WHERE id=(SELECT MAX(id) FROM auth_sessions WHERE user_id=?)`, identityID, u.ID)
	db.Exec(`UPDATE users SET last_login_at=?, last_login_ip=?, failed_logins=0, locked_until='' WHERE id=?`, nowRFC(), clientIP(r), u.ID)
	auditLogAs(r, u.ID, u.Username, "auth.login", "", map[string]string{"method": "oidc", "provider": provider, "user_agent": truncateStr(r.UserAgent(), 160)})
	return nil
}

func oidcSaveIdentity(userID, providerID int, subject string, c oidcClaims, refresh string) int {
	now := nowRFC()
	email := c.str("email")
	if email == "" {
		email = c.str("upn")
	}
	enc := ""
	if refresh != "" {
		enc = encryptValue(refresh)
	}
	db.Exec(`INSERT INTO user_identities (user_id, provider_id, subject, email, display, refresh_token, created_at, last_login_at, last_check_at, last_ok_at, last_error, revoked)
		VALUES (?,?,?,?,?,?,?,?,?,?,'',0)
		ON CONFLICT(provider_id, subject) DO UPDATE SET email=excluded.email, display=excluded.display,
		refresh_token=CASE WHEN excluded.refresh_token != '' THEN excluded.refresh_token ELSE user_identities.refresh_token END,
		last_login_at=excluded.last_login_at, last_check_at=excluded.last_check_at, last_ok_at=excluded.last_ok_at, last_error='', revoked=0`,
		userID, providerID, subject, truncateStr(email, 200), truncateStr(c.str("name"), 100), enc, now, now, now, now)
	var id int
	db.QueryRow(`SELECT id FROM user_identities WHERE provider_id=? AND subject=?`, providerID, subject).Scan(&id)
	return id
}

// After a directory sign-in, an account with local 2FA (and a provider that does not prove MFA)
// confirms its code on the sign-in page; the signed MFA token then carries the identity, so the
// new session is tied to it and revalidated.
func newMFATokenFor(userID, identityID int) string {
	payload := fmt.Sprintf("%d.%d.%s~%d", userID, time.Now().Add(5*time.Minute).Unix(), randomToken(12), identityID)
	return payload + "." + signValue("mfa", payload)
}

// oidcMFAIdentity reads the identity from an MFA token's nonce (0 for a password sign-in).
func oidcMFAIdentity(nonce string) int {
	if i := strings.LastIndex(nonce, "~"); i >= 0 {
		return atoiOr(nonce[i+1:], 0)
	}
	return 0
}

// ─── revalidation ────────────────────────────────────

func oidcRevalidateLoop() {
	for {
		time.Sleep(5 * time.Minute)
		oidcRevalidateOnce()
	}
}

// oidcRevalidateOnce checks every directory identity that has a sign-in session.
func oidcRevalidateOnce() {
	rows, err := db.Query(`SELECT DISTINCT i.id FROM user_identities i JOIN auth_sessions a ON a.identity_id=i.id WHERE i.revoked=0`)
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
	every := time.Duration(settingInt("oidc_revalidate_minutes")) * time.Minute
	grace := time.Duration(settingInt("oidc_unverified_hours")) * time.Hour
	for _, iid := range ids {
		id, err := loadOIDCIdentity(`id=?`, iid)
		if err != nil {
			continue
		}
		var lastCheck string
		db.QueryRow(`SELECT last_check_at FROM user_identities WHERE id=?`, iid).Scan(&lastCheck)
		if t, ok := parseTS(lastCheck); ok && time.Since(t) < every {
			continue
		}
		oidcRevalidate(id, grace)
	}
}

func oidcRevalidate(id *oidcIdentity, grace time.Duration) {
	username := usernameOf(id.UserID)
	end := func(reason string) {
		oidcEndIdentity(id.ID, id.UserID, "Your directory account may no longer use WRM")
		auditLogAs(nil, id.UserID, username, "oidc.access_ended", username, map[string]string{"reason": reason})
	}
	stale := func(errText string) {
		db.Exec(`UPDATE user_identities SET last_check_at=?, last_error=? WHERE id=?`, nowRFC(), truncateStr(errText, 300), id.ID)
		if t, ok := parseTS(id.LastOKAt); !ok || time.Since(t) > grace {
			end("not verified with the identity provider for longer than oidc_unverified_hours: " + truncateStr(errText, 200))
		}
	}
	p, err := loadOIDCProvider(id.ProviderID)
	if err != nil || !p.Enabled {
		end("identity provider removed or turned off")
		return
	}
	if id.Refresh == "" {
		stale("no refresh token (add offline_access to the scopes)")
		return
	}
	d, err := oidcDiscover(p.Issuer, false)
	if err != nil {
		stale(err.Error())
		return
	}
	tr, err := oidcTokenRequest(p, d, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {id.Refresh}})
	if errors.Is(err, errOIDCRefused) {
		end("refresh refused: " + err.Error())
		return
	}
	if err != nil {
		stale(err.Error())
		return
	}
	if tr.IDToken != "" && len(p.Rules) > 0 {
		if c, err := oidcVerifyIDToken(tr.IDToken, d, p.ClientID, ""); err == nil && oidcSubject(c) == id.Subject {
			m := oidcMap(p, c)
			if !m.Matched {
				end("no longer assigned to WRM in the directory")
				return
			}
			oidcApplyMapping(nil, id.UserID, m)
		}
	}
	enc := encryptValue(id.Refresh)
	if tr.RefreshToken != "" {
		enc = encryptValue(tr.RefreshToken)
	}
	db.Exec(`UPDATE user_identities SET refresh_token=?, last_check_at=?, last_ok_at=?, last_error='' WHERE id=?`, enc, nowRFC(), nowRFC(), id.ID)
}

// ─── HTTP: administration ────────────────────────────

func oidcProviderJSON(p *oidcProvider, r *http.Request) map[string]interface{} {
	var linked, sessions int
	db.QueryRow(`SELECT COUNT(*) FROM user_identities WHERE provider_id=?`, p.ID).Scan(&linked)
	db.QueryRow(`SELECT COUNT(*) FROM auth_sessions a JOIN user_identities i ON i.id=a.identity_id WHERE i.provider_id=?`, p.ID).Scan(&sessions)
	return map[string]interface{}{
		"id": p.ID, "name": p.Name, "issuer": p.Issuer, "client_id": p.ClientID, "has_secret": p.HasSecret, "scopes": p.Scopes,
		"redirect_uri": p.RedirectURI, "redirect_uri_effective": oidcRedirectURI(p, r), "username_claim": p.UsernameClaim,
		"require_mfa": p.RequireMFA, "mfa_values": p.MFAValues, "jit": p.JIT, "rules": p.Rules, "enabled": p.Enabled,
		"entra": oidcIsEntra(p.Issuer), "linked_users": linked, "sessions": sessions, "created_at": p.CreatedAt, "updated_at": p.UpdatedAt,
	}
}

type oidcProviderInput struct {
	Name          *string     `json:"name"`
	Issuer        *string     `json:"issuer"`
	ClientID      *string     `json:"client_id"`
	ClientSecret  *string     `json:"client_secret"` // "" keeps the stored secret; use clear_secret to remove it
	ClearSecret   bool        `json:"clear_secret"`
	Scopes        *string     `json:"scopes"`
	RedirectURI   *string     `json:"redirect_uri"`
	UsernameClaim *string     `json:"username_claim"`
	RequireMFA    *bool       `json:"require_mfa"`
	MFAValues     *string     `json:"mfa_values"`
	JIT           *bool       `json:"jit"`
	Rules         *[]oidcRule `json:"rules"`
	Enabled       *bool       `json:"enabled"`
}

// GET    /api/admin/oidc                providers, linked identities, the default redirect address
// POST   /api/admin/oidc                create a provider
// PUT    /api/admin/oidc/{id}           change a provider (fields optional)
// DELETE /api/admin/oidc/{id}           remove a provider and its links (signs its users out)
// POST   /api/admin/oidc/check          {issuer} step-by-step connection test
// POST   /api/admin/oidc/{id}/preview   {claims} what WRM would do for a token with these claims
// DELETE /api/admin/oidc/identities/{id} unlink one directory account
func apiAdminOIDCHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/oidc"), "/")
	parts := strings.Split(rest, "/")
	switch {
	case rest == "" && r.Method == http.MethodGet:
		list := []map[string]interface{}{}
		for _, p := range oidcProviders(false) {
			list = append(list, oidcProviderJSON(p, r))
		}
		jsonOK(w, map[string]interface{}{
			"providers": list, "identities": oidcIdentityList(0), "default_redirect_uri": oidcRedirectURI(nil, r),
			"local_login": getSetting("local_login"), "local_login_limited": localLoginLimited(),
			"revalidate_minutes": settingInt("oidc_revalidate_minutes"), "unverified_hours": settingInt("oidc_unverified_hours"),
		})
	case rest == "" && r.Method == http.MethodPost:
		if !requireStepUp(w, r, "oidc.provider") {
			return
		}
		saveOIDCProvider(w, r, adminID, 0)
	case rest == "check" && r.Method == http.MethodPost:
		var in struct {
			Issuer string `json:"issuer"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		jsonOK(w, oidcCheck(in.Issuer, r))
	case len(parts) == 2 && parts[0] == "identities" && r.Method == http.MethodDelete:
		if !requireStepUp(w, r, "oidc.unlink") {
			return
		}
		id, err := loadOIDCIdentity(`id=?`, atoiOr(parts[1], 0))
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		oidcUnlink(r, adminID, id)
		jsonOK(w, map[string]bool{"ok": true})
	case len(parts) == 2 && parts[1] == "preview" && r.Method == http.MethodPost:
		p, err := loadOIDCProvider(atoiOr(parts[0], 0))
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		var in struct {
			Claims oidcClaims `json:"claims"`
			Rules  []oidcRule `json:"rules"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if in.Rules != nil {
			rules, err := validateOIDCRules(in.Rules)
			if err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
			p.Rules = rules
		}
		jsonOK(w, map[string]interface{}{"mapping": oidcMap(p, in.Claims), "mfa": oidcMFAProven(p, in.Claims)})
	case len(parts) == 1 && rest != "" && r.Method == http.MethodPut:
		if !requireStepUp(w, r, "oidc.provider") {
			return
		}
		saveOIDCProvider(w, r, adminID, atoiOr(parts[0], 0))
	case len(parts) == 1 && rest != "" && r.Method == http.MethodDelete:
		if !requireStepUp(w, r, "oidc.provider") {
			return
		}
		p, err := loadOIDCProvider(atoiOr(parts[0], 0))
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		rows, _ := db.Query(`SELECT id FROM user_identities WHERE provider_id=?`, p.ID)
		var ids []int
		if rows != nil {
			for rows.Next() {
				var id int
				rows.Scan(&id)
				ids = append(ids, id)
			}
			rows.Close()
		}
		for _, iid := range ids {
			if id, err := loadOIDCIdentity(`id=?`, iid); err == nil {
				oidcUnlink(r, adminID, id)
			}
		}
		db.Exec(`DELETE FROM oidc_providers WHERE id=?`, p.ID)
		auditLog(r, adminID, "oidc.provider_deleted", p.Name, nil)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Not found", 404)
	}
}

// oidcUnlink removes a directory link; the directory sessions of that account end.
func oidcUnlink(r *http.Request, adminID int, id *oidcIdentity) {
	var ssoOnly int
	db.QueryRow(`SELECT COALESCE(sso_only,0) FROM users WHERE id=?`, id.UserID).Scan(&ssoOnly)
	db.Exec(`DELETE FROM auth_sessions WHERE identity_id=?`, id.ID)
	db.Exec(`DELETE FROM user_identities WHERE id=?`, id.ID)
	if ssoOnly == 1 {
		disconnectUserEverywhere(id.UserID, "Your directory sign-in was removed")
	}
	auditLog(r, adminID, "oidc.identity_unlinked", usernameOf(id.UserID), map[string]string{"subject": id.Subject})
}

func oidcIdentityList(userID int) []map[string]interface{} {
	q := `SELECT i.id, i.user_id, COALESCE(u.username,''), i.provider_id, COALESCE(p.name,''), i.subject, i.email, i.display, i.refresh_token != '',
		i.last_login_at, i.last_check_at, i.last_ok_at, i.last_error, i.revoked FROM user_identities i
		LEFT JOIN users u ON u.id=i.user_id LEFT JOIN oidc_providers p ON p.id=i.provider_id`
	args := []interface{}{}
	if userID > 0 {
		q += ` WHERE i.user_id=?`
		args = append(args, userID)
	}
	rows, err := db.Query(q+` ORDER BY u.username`, args...)
	out := []map[string]interface{}{}
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, uid, pid, revoked int
		var hasRefresh bool
		var username, pname, subject, email, display, lastLogin, lastCheck, lastOK, lastErr string
		if rows.Scan(&id, &uid, &username, &pid, &pname, &subject, &email, &display, &hasRefresh, &lastLogin, &lastCheck, &lastOK, &lastErr, &revoked) != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"id": id, "user_id": uid, "username": username, "provider_id": pid, "provider": pname, "subject": subject, "email": email,
			"display": display, "refresh": hasRefresh, "last_login_at": lastLogin, "last_check_at": lastCheck, "last_ok_at": lastOK,
			"last_error": lastErr, "revoked": revoked == 1,
		})
	}
	return out
}

func saveOIDCProvider(w http.ResponseWriter, r *http.Request, adminID, id int) {
	var in oidcProviderInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	p := &oidcProvider{Scopes: oidcDefaultScopes, UsernameClaim: "preferred_username", JIT: true, Rules: []oidcRule{}}
	if id > 0 {
		var err error
		if p, err = loadOIDCProvider(id); err != nil {
			jsonError(w, "Not found", 404)
			return
		}
	}
	setStr := func(dst *string, v *string, max int) {
		if v != nil {
			*dst = truncateStr(strings.TrimSpace(*v), max)
		}
	}
	oldIssuer := p.Issuer
	setStr(&p.Name, in.Name, 60)
	setStr(&p.Issuer, in.Issuer, 300)
	setStr(&p.ClientID, in.ClientID, 300)
	setStr(&p.Scopes, in.Scopes, 300)
	setStr(&p.RedirectURI, in.RedirectURI, 300)
	setStr(&p.UsernameClaim, in.UsernameClaim, 60)
	setStr(&p.MFAValues, in.MFAValues, 300)
	p.Issuer = oidcIssuerTrim(p.Issuer)
	if in.RequireMFA != nil {
		p.RequireMFA = *in.RequireMFA
	}
	if in.JIT != nil {
		p.JIT = *in.JIT
	}
	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}
	// The stored client secret only goes to the provider it was entered for.
	if id > 0 && p.secret != "" && in.Issuer != nil && oidcIssuerTrim(*in.Issuer) != oidcIssuerTrim(oldIssuer) && (in.ClientSecret == nil || *in.ClientSecret == "") && !in.ClearSecret {
		jsonError(w, "Enter the client secret again when you change the issuer", 400)
		return
	}
	secretChanged := false
	if in.ClearSecret {
		p.secret, secretChanged = "", true
	} else if in.ClientSecret != nil && *in.ClientSecret != "" {
		p.secret, secretChanged = strings.TrimSpace(*in.ClientSecret), true
	}
	if in.Rules != nil {
		rules, err := validateOIDCRules(*in.Rules)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		p.Rules = rules
	}
	if p.Name == "" {
		jsonError(w, "Enter a name (shown on the sign-in button)", 400)
		return
	}
	if p.Issuer == "" || p.ClientID == "" {
		jsonError(w, "Enter the issuer and the client ID", 400)
		return
	}
	if err := oidcURLAllowed(p.Issuer); err != nil {
		jsonError(w, "Issuer: "+err.Error(), 400)
		return
	}
	if p.RedirectURI != "" {
		if err := oidcURLAllowed(p.RedirectURI); err != nil || !strings.HasSuffix(p.RedirectURI, "/api/auth/oidc/callback") {
			jsonError(w, "The redirect address must be https://<WRM address>/api/auth/oidc/callback", 400)
			return
		}
	}
	if p.UsernameClaim == "" {
		p.UsernameClaim = "preferred_username"
	}
	if p.Enabled {
		issuer, err := oidcRecordIssuer(p.Issuer)
		if err == nil {
			_, err = oidcDiscover(issuer, true)
		}
		if err != nil {
			jsonError(w, "Cannot enable: "+err.Error()+" (use Test connection)", 400)
			return
		}
	}
	rules, _ := json.Marshal(p.Rules)
	secret := ""
	if p.secret != "" {
		secret = encryptValue(p.secret)
	}
	now := nowRFC()
	var err error
	if id == 0 {
		var res sql.Result
		res, err = db.Exec(`INSERT INTO oidc_providers (name, issuer, client_id, client_secret, scopes, redirect_uri, username_claim, require_mfa, mfa_values, jit, rules, enabled, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.Name, p.Issuer, p.ClientID, secret, p.Scopes, p.RedirectURI, p.UsernameClaim,
			boolToInt(p.RequireMFA), p.MFAValues, boolToInt(p.JIT), string(rules), boolToInt(p.Enabled), now, now)
		if err == nil {
			nid, _ := res.LastInsertId()
			id = int(nid)
		}
	} else {
		_, err = db.Exec(`UPDATE oidc_providers SET name=?, issuer=?, client_id=?, client_secret=?, scopes=?, redirect_uri=?, username_claim=?, require_mfa=?, mfa_values=?, jit=?, rules=?, enabled=?, updated_at=? WHERE id=?`,
			p.Name, p.Issuer, p.ClientID, secret, p.Scopes, p.RedirectURI, p.UsernameClaim, boolToInt(p.RequireMFA), p.MFAValues, boolToInt(p.JIT), string(rules), boolToInt(p.Enabled), now, id)
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			jsonError(w, "A provider with this name already exists", 400)
			return
		}
		jsonError(w, "Server error", 500)
		return
	}
	auditLog(r, adminID, "oidc.provider_saved", p.Name, map[string]interface{}{"issuer": p.Issuer, "enabled": p.Enabled, "rules": len(p.Rules), "jit": p.JIT, "require_mfa": p.RequireMFA, "cred_changed": secretChanged})
	saved, _ := loadOIDCProvider(id)
	jsonOK(w, oidcProviderJSON(saved, r))
}

// oidcRecordIssuer records an issuer an administrator asked WRM to contact (tests and enabling
// a provider) and returns the stored value, so outbound requests only use issuers from WRM's
// own database. Entries older than 30 days are dropped.
func oidcRecordIssuer(issuer string) (string, error) {
	now := nowRFC()
	db.Exec(`DELETE FROM oidc_issuer_checks WHERE checked_at < ?`, time.Now().Add(-30*24*time.Hour).UTC().Format(time.RFC3339))
	if _, err := db.Exec(`INSERT INTO oidc_issuer_checks (issuer, checked_at) VALUES (?,?) ON CONFLICT(issuer) DO UPDATE SET checked_at=excluded.checked_at`, issuer, now); err != nil {
		return "", err
	}
	var stored string
	err := db.QueryRow(`SELECT issuer FROM oidc_issuer_checks WHERE issuer=?`, issuer).Scan(&stored)
	return stored, err
}

type oidcCheckStep struct {
	Step   string `json:"step"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// oidcCheck tests what can be tested without a user: discovery, endpoints, signing keys, PKCE.
func oidcCheck(issuer string, r *http.Request) map[string]interface{} {
	issuer = oidcIssuerTrim(issuer)
	steps := []oidcCheckStep{}
	add := func(step string, ok bool, detail, fix string) {
		steps = append(steps, oidcCheckStep{step, ok, detail, fix})
	}
	done := func() map[string]interface{} {
		ok := true
		for _, s := range steps {
			ok = ok && s.OK
		}
		return map[string]interface{}{"ok": ok, "steps": steps, "redirect_uri": oidcRedirectURI(nil, r)}
	}
	if err := oidcURLAllowed(issuer); err != nil {
		add("issuer", false, err.Error(), "Copy the issuer from the provider, for example https://login.microsoftonline.com/<tenant ID>/v2.0 or https://keycloak.example.com/realms/<realm>.")
		return done()
	}
	if strings.Contains(issuer, "/common/") || strings.HasSuffix(issuer, "/common") || strings.Contains(issuer, "/organizations") {
		add("issuer", false, "Multi-tenant Entra addresses (common, organizations) are not accepted.", "Use your own tenant: https://login.microsoftonline.com/<tenant ID>/v2.0.")
		return done()
	}
	stored, err := oidcRecordIssuer(issuer)
	if err != nil {
		add("issuer", false, err.Error(), "")
		return done()
	}
	d, err := oidcDiscover(stored, true)
	if err != nil {
		add("discovery", false, err.Error(), "WRM must reach "+issuer+"/.well-known/openid-configuration over HTTPS. Check the address, DNS, firewall and the proxy of the WRM host.")
		return done()
	}
	add("discovery", true, "Read "+issuer+"/.well-known/openid-configuration.", "")
	add("endpoints", true, "Sign-in: "+d.AuthorizationEndpoint+" · tokens: "+d.TokenEndpoint, "")
	keys, err := oidcKeys(d.JWKSURI, true)
	if err != nil {
		add("keys", false, err.Error(), "The provider's signing keys ("+d.JWKSURI+") must be reachable from the WRM host.")
	} else {
		add("keys", true, fmt.Sprintf("%d signing key(s) from %s.", len(keys), d.JWKSURI), "")
	}
	pkce := len(d.CodeChallengeMethods) == 0
	for _, m := range d.CodeChallengeMethods {
		pkce = pkce || m == "S256"
	}
	if pkce {
		add("pkce", true, "PKCE (S256) is supported.", "")
	} else {
		add("pkce", false, "The provider does not list PKCE S256.", "Turn on PKCE for the client, or use a provider version that supports it.")
	}
	offline := len(d.ScopesSupported) == 0
	for _, s := range d.ScopesSupported {
		offline = offline || s == "offline_access"
	}
	if offline {
		add("revalidation", true, "offline_access is offered: WRM can check signed-in users regularly.", "")
	} else {
		add("revalidation", true, "The provider does not list offline_access: users must sign in again after "+strconv.Itoa(settingInt("oidc_unverified_hours"))+" h.", "")
	}
	add("redirect", true, "Register this redirect address at the provider: "+oidcRedirectURI(nil, r), "")
	return done()
}

// GET /api/me/identities, DELETE /api/me/identities/{id}
func apiMeIdentitiesHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/me/identities"), "/")
	switch {
	case rest == "" && r.Method == http.MethodGet:
		providers := []map[string]interface{}{}
		for _, p := range oidcProviders(true) {
			providers = append(providers, map[string]interface{}{"id": p.ID, "name": p.Name})
		}
		jsonOK(w, map[string]interface{}{"identities": oidcIdentityList(userID), "providers": providers})
	case rest != "" && r.Method == http.MethodDelete:
		id, err := loadOIDCIdentity(`id=? AND user_id=?`, atoiOr(rest, 0), userID)
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		var ssoOnly int
		db.QueryRow(`SELECT COALESCE(sso_only,0) FROM users WHERE id=?`, userID).Scan(&ssoOnly)
		if ssoOnly == 1 {
			jsonError(w, "Your account has no password; it signs in only through the directory", 400)
			return
		}
		db.Exec(`DELETE FROM auth_sessions WHERE identity_id=? AND user_id=?`, id.ID, userID)
		db.Exec(`DELETE FROM user_identities WHERE id=?`, id.ID)
		auditLog(r, userID, "oidc.identity_unlinked", usernameOf(userID), map[string]string{"subject": id.Subject})
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Not found", 404)
	}
}

// oidcSignInOptions are the buttons on the sign-in page.
func oidcSignInOptions() []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, p := range oidcProviders(true) {
		out = append(out, map[string]interface{}{"id": p.ID, "name": p.Name})
	}
	return out
}

func userSSOOnly(userID int) bool {
	var v int
	db.QueryRow(`SELECT COALESCE(sso_only,0) FROM users WHERE id=?`, userID).Scan(&v)
	return v == 1
}

// userSSOProviders lists the enabled providers the account is linked to (for step-up).
func userSSOProviders(userID int) []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := db.Query(`SELECT p.id, p.name FROM user_identities i JOIN oidc_providers p ON p.id=i.provider_id WHERE i.user_id=? AND p.enabled=1 ORDER BY p.name`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name string
		rows.Scan(&id, &name)
		out = append(out, map[string]interface{}{"id": id, "name": name})
	}
	return out
}
