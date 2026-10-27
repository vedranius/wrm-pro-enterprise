// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ─── MCP: OAUTH 2.1 ──────────────────────────────────
//
// WRM is its own authorization server for MCP clients (OAuth 2.1 with PKCE):
//   /.well-known/oauth-protected-resource[/mcp]   RFC 9728: where to get a token for /mcp
//   /.well-known/oauth-authorization-server       RFC 8414 metadata
//   POST /oauth/register                          RFC 7591 dynamic client registration
//   GET  /oauth/authorize                         checks the request, then shows WRM's consent
//                                                 dialog (/?mcp_authorize=<id>) after sign-in
//   POST /oauth/token                             authorization_code + PKCE (S256 only)
//   POST /oauth/revoke                            RFC 7009
// Redirect URIs must match the ai_mcp_redirect_uris allowlist (https, or http on loopback
// only). The consent dialog is the user's normal WRM sign-in (with 2FA): there the user picks
// the connections, the mode, the scopes and the lifetime — the client cannot choose more
// than the user grants. The access token is an AI connection of kind "oauth"; there are no
// refresh tokens: when it expires the user approves again.

const (
	mcpCodeTTL    = 5 * time.Minute
	mcpPendingTTL = 15 * time.Minute
)

type mcpPendingAuth struct {
	ID          string
	ClientID    string
	ClientName  string
	RedirectURI string
	State       string
	Challenge   string
	Scopes      []string
	Resource    string
	expires     time.Time
}

type mcpAuthCode struct {
	UserID      int
	ClientID    string
	ClientName  string
	RedirectURI string
	Challenge   string
	Resource    string
	Req         mcpGrantRequest
	expires     time.Time
	used        bool
	grantID     int64
}

var mcpPending = struct {
	sync.Mutex
	m map[string]*mcpPendingAuth
}{m: map[string]*mcpPendingAuth{}}

var mcpCodes = struct {
	sync.Mutex
	m map[string]*mcpAuthCode
}{m: map[string]*mcpAuthCode{}}

// mcpBaseURL is the public address of WRM (policy ai_mcp_public_url, else the request).
func mcpBaseURL(r *http.Request) string {
	if v := strings.TrimRight(strings.TrimSpace(getSetting("ai_mcp_public_url")), "/"); v != "" {
		return v
	}
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	host := r.Host
	if trustProxy {
		if h := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); h != "" {
			host = h
		}
	}
	return scheme + "://" + host
}

func mcpOAuthAvailable() bool {
	return settingBool("ai_mcp_enabled") && settingBool("ai_mcp_oauth")
}

// ─── simple per-address rate limit ───────────────────

type rateWindow struct {
	n     int
	start time.Time
}

var mcpRate = struct {
	sync.Mutex
	m map[string]*rateWindow
}{m: map[string]*rateWindow{}}

func mcpRateOK(key string, max int, per time.Duration) bool {
	mcpRate.Lock()
	defer mcpRate.Unlock()
	now := time.Now()
	if len(mcpRate.m) > 10000 {
		mcpRate.m = map[string]*rateWindow{}
	}
	w := mcpRate.m[key]
	if w == nil || now.Sub(w.start) > per {
		w = &rateWindow{start: now}
		mcpRate.m[key] = w
	}
	w.n++
	return w.n <= max
}

// ─── metadata ────────────────────────────────────────

func mcpMetadataCORS(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, MCP-Protocol-Version")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	if r.Method == http.MethodOptions {
		w.WriteHeader(204)
		return true
	}
	return false
}

func mcpProtectedResourceHandler(w http.ResponseWriter, r *http.Request) {
	if !settingBool("ai_mcp_enabled") {
		http.NotFound(w, r)
		return
	}
	if mcpMetadataCORS(w, r) {
		return
	}
	base := mcpBaseURL(r)
	out := map[string]interface{}{
		"resource": base + "/mcp", "bearer_methods_supported": []string{"header"}, "scopes_supported": mcpScopeOrder,
		"resource_name": "WRM PRO", "resource_documentation": "https://github.com/vedranius/wrm-pro-enterprise#ai-desktop-apps-mcp",
	}
	if mcpOAuthAvailable() {
		out["authorization_servers"] = []string{base}
	}
	jsonOK(w, out)
}

func mcpAuthServerMetadataHandler(w http.ResponseWriter, r *http.Request) {
	if !mcpOAuthAvailable() {
		http.NotFound(w, r)
		return
	}
	if mcpMetadataCORS(w, r) {
		return
	}
	base := mcpBaseURL(r)
	jsonOK(w, map[string]interface{}{
		"issuer": base, "authorization_endpoint": base + "/oauth/authorize", "token_endpoint": base + "/oauth/token",
		"registration_endpoint": base + "/oauth/register", "revocation_endpoint": base + "/oauth/revoke",
		"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code"},
		"code_challenge_methods_supported":           []string{"S256"},
		"token_endpoint_auth_methods_supported":      []string{"none", "client_secret_post", "client_secret_basic"},
		"revocation_endpoint_auth_methods_supported": []string{"none", "client_secret_post", "client_secret_basic"},
		"scopes_supported":                           mcpScopeOrder, "service_documentation": "https://github.com/vedranius/wrm-pro-enterprise#ai-desktop-apps-mcp",
		"authorization_response_iss_parameter_supported": true,
	})
}

// ─── redirect URI allowlist ──────────────────────────

// mcpRedirectAllowed checks a redirect URI against the ai_mcp_redirect_uris patterns:
// "*" matches any text. URIs with user info or a fragment are refused; http only on
// loopback addresses.
func mcpRedirectAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || u.Host == "" || len(raw) > 2000 {
		return false
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return false
	}
	for _, p := range splitPatterns(getSetting("ai_mcp_redirect_uris")) {
		if p == raw {
			return true
		}
		if strings.Contains(p, "*") {
			re := "^" + strings.ReplaceAll(regexp.QuoteMeta(p), `\*`, `[^#\s]*`) + "$"
			if ok, _ := regexp.MatchString(re, raw); ok {
				// the host must be the pattern's host: no wildcard may reach into it
				if pu, err := url.Parse(strings.ReplaceAll(p, "*", "0")); err == nil && strings.EqualFold(pu.Hostname(), u.Hostname()) {
					return true
				}
			}
		}
	}
	return false
}

// ─── dynamic client registration ─────────────────────

type mcpClient struct {
	ID           int
	ClientID     string
	Name         string
	RedirectURIs []string
	SecretHash   string
	AuthMethod   string
}

func loadMCPClient(clientID string) (*mcpClient, error) {
	c := &mcpClient{}
	var uris string
	err := db.QueryRow(`SELECT id, client_id, client_name, redirect_uris, secret_hash, auth_method FROM mcp_clients WHERE client_id=?`, clientID).
		Scan(&c.ID, &c.ClientID, &c.Name, &uris, &c.SecretHash, &c.AuthMethod)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(uris), &c.RedirectURIs)
	return c, nil
}

func oauthError(w http.ResponseWriter, code int, errCode, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": errCode, "error_description": desc})
}

var ctlChars = regexp.MustCompile(`[\x00-\x1f\x7f]`)

func mcpRegisterHandler(w http.ResponseWriter, r *http.Request) {
	if !mcpOAuthAvailable() {
		http.NotFound(w, r)
		return
	}
	if !mcpTokenEndpointCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		oauthError(w, 405, "invalid_request", "use POST")
		return
	}
	if !mcpRateOK("reg:"+clientIP(r), 30, time.Hour) {
		oauthError(w, 429, "invalid_request", "too many registrations from this address")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var in struct {
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
		GrantTypes   []string `json:"grant_types"`
		Response     []string `json:"response_types"`
		AuthMethod   string   `json:"token_endpoint_auth_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		oauthError(w, 400, "invalid_client_metadata", "invalid JSON")
		return
	}
	if len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > 10 {
		oauthError(w, 400, "invalid_redirect_uri", "give 1 to 10 redirect_uris")
		return
	}
	for _, u := range in.RedirectURIs {
		if !mcpRedirectAllowed(u) {
			auditLog(r, 0, "mcp.client_refused", "", map[string]interface{}{"redirect_uri": truncateStr(u, 300), "client": truncateStr(in.ClientName, 100)})
			oauthError(w, 400, "invalid_redirect_uri", "the redirect URI "+truncateStr(u, 200)+" is not allowed by this WRM server (policy ai_mcp_redirect_uris)")
			return
		}
	}
	if len(in.GrantTypes) > 0 && !oneOf("authorization_code", in.GrantTypes...) {
		oauthError(w, 400, "invalid_client_metadata", "only the authorization_code grant is supported")
		return
	}
	if len(in.Response) > 0 && !oneOf("code", in.Response...) {
		oauthError(w, 400, "invalid_client_metadata", "only response_type code is supported")
		return
	}
	method := nonEmpty(in.AuthMethod, "none")
	if !oneOf(method, "none", "client_secret_post", "client_secret_basic") {
		oauthError(w, 400, "invalid_client_metadata", "token_endpoint_auth_method must be none, client_secret_post or client_secret_basic")
		return
	}
	name := strings.TrimSpace(ctlChars.ReplaceAllString(in.ClientName, ""))
	name = truncateStr(nonEmpty(name, "MCP client"), 100)
	cid := "wrm-mcp-" + randomToken(18)
	secret, secretHash := "", ""
	if method != "none" {
		secret = randomToken(32)
		secretHash = sha256Hex(secret)
	}
	uris, _ := json.Marshal(in.RedirectURIs)
	if _, err := db.Exec(`INSERT INTO mcp_clients (client_id, client_name, redirect_uris, secret_hash, auth_method, created_at, created_ip) VALUES (?,?,?,?,?,?,?)`,
		cid, name, string(uris), secretHash, method, nowRFC(), clientIP(r)); err != nil {
		oauthError(w, 500, "server_error", "could not store the client")
		return
	}
	auditLog(r, 0, "mcp.client_registered", name, map[string]interface{}{"client_id": cid, "redirect_uris": strings.Join(in.RedirectURIs, " "), "auth_method": method})
	out := map[string]interface{}{"client_id": cid, "client_id_issued_at": time.Now().Unix(), "client_name": name, "redirect_uris": in.RedirectURIs,
		"grant_types": []string{"authorization_code"}, "response_types": []string{"code"}, "token_endpoint_auth_method": method}
	if secret != "" {
		out["client_secret"] = secret
		out["client_secret_expires_at"] = 0
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(out)
}

// mcpTokenEndpointCORS lets browser-based MCP clients from the allowed origins call the
// registration and token endpoints. It returns false when the request is answered.
func mcpTokenEndpointCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if !mcpOriginAllowed(r, origin) {
		auditLog(r, 0, "mcp.origin_refused", "", map[string]interface{}{"origin": truncateStr(origin, 200), "path": r.URL.Path})
		oauthError(w, 403, "access_denied", "origin not allowed")
		return false
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Vary", "Origin")
	h.Set("Cross-Origin-Resource-Policy", "cross-origin")
	if r.Method == http.MethodOptions {
		h.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		h.Set("Access-Control-Max-Age", "600")
		w.WriteHeader(204)
		return false
	}
	return true
}

// ─── authorize ───────────────────────────────────────

func oauthPage(w http.ResponseWriter, code int, title, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>WRM PRO</title>
<style>body{font-family:system-ui,sans-serif;background:#0f1115;color:#e6e6e6;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;padding:16px;box-sizing:border-box}
div{max-width:520px;background:#181b22;border:1px solid #2a2f3a;border-radius:10px;padding:24px}h1{font-size:18px;margin:0 0 12px}p{color:#aab;line-height:1.5}a{color:#4f8cff}</style></head>
<body><div><h1>%s</h1><p>%s</p><p><a href="/">WRM PRO</a></p></div></body></html>`, html.EscapeString(title), html.EscapeString(msg))
}

func redirectWith(w http.ResponseWriter, r *http.Request, redirectURI string, params url.Values) {
	u, _ := url.Parse(redirectURI)
	q := u.Query()
	for k, v := range params {
		for _, x := range v {
			q.Add(k, x)
		}
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func mcpAuthorizeHandler(w http.ResponseWriter, r *http.Request) {
	if !mcpOAuthAvailable() {
		oauthPage(w, 404, "AI connections are turned off", "An administrator has not enabled AI connections (MCP) on this WRM server.")
		return
	}
	if !mcpRateOK("authz:"+clientIP(r), 60, 10*time.Minute) {
		oauthPage(w, 429, "Too many requests", "Wait a few minutes and start the connection in the application again.")
		return
	}
	q := r.URL.Query()
	c, err := loadMCPClient(q.Get("client_id"))
	if err != nil {
		oauthPage(w, 400, "Unknown application", "This application is not registered with WRM. Remove the connector in the application and add it again.")
		return
	}
	redirect := q.Get("redirect_uri")
	if redirect == "" && len(c.RedirectURIs) == 1 {
		redirect = c.RedirectURIs[0]
	}
	if !oneOf(redirect, c.RedirectURIs...) || !mcpRedirectAllowed(redirect) {
		auditLog(r, 0, "mcp.authorize_refused", c.Name, map[string]interface{}{"client_id": c.ClientID, "reason": "redirect_uri", "redirect_uri": truncateStr(redirect, 300)})
		oauthPage(w, 400, "Invalid redirect address", "The application asked WRM to send you to an address that it did not register or that this server does not allow.")
		return
	}
	state := q.Get("state")
	fail := func(code, desc string) {
		auditLog(r, 0, "mcp.authorize_refused", c.Name, map[string]interface{}{"client_id": c.ClientID, "reason": code + ": " + desc})
		p := url.Values{"error": {code}, "error_description": {desc}, "iss": {mcpBaseURL(r)}}
		if state != "" {
			p.Set("state", state)
		}
		redirectWith(w, r, redirect, p)
	}
	if q.Get("response_type") != "code" {
		fail("unsupported_response_type", "only response_type=code is supported")
		return
	}
	challenge := q.Get("code_challenge")
	if challenge == "" || q.Get("code_challenge_method") != "S256" {
		fail("invalid_request", "PKCE with code_challenge_method=S256 is required")
		return
	}
	if len(challenge) != 43 {
		fail("invalid_request", "invalid code_challenge")
		return
	}
	resource := q.Get("resource")
	if resource != "" && !mcpResourceOK(r, resource) {
		fail("invalid_target", "unknown resource")
		return
	}
	var scopes []string
	if sc := q.Get("scope"); sc != "" {
		for _, s := range strings.Fields(sc) {
			if oneOf(s, mcpScopeOrder...) {
				scopes = append(scopes, s)
			}
		}
	}
	p := &mcpPendingAuth{ID: randomToken(18), ClientID: c.ClientID, ClientName: c.Name, RedirectURI: redirect, State: state, Challenge: challenge,
		Scopes: scopes, Resource: resource, expires: time.Now().Add(mcpPendingTTL)}
	mcpPending.Lock()
	if len(mcpPending.m) > 5000 {
		mcpPending.m = map[string]*mcpPendingAuth{}
	}
	mcpPending.m[p.ID] = p
	mcpPending.Unlock()
	http.Redirect(w, r, "/?mcp_authorize="+url.QueryEscape(p.ID), http.StatusFound)
}

func mcpResourceOK(r *http.Request, res string) bool {
	base := mcpBaseURL(r)
	res = strings.TrimRight(res, "/")
	return strings.EqualFold(res, base+"/mcp") || strings.EqualFold(res, base)
}

// mcpConsentHandler is the consent dialog's API: GET describes the request, POST answers it.
func mcpConsentHandler(w http.ResponseWriter, r *http.Request, userID int, id string) {
	mcpPending.Lock()
	p := mcpPending.m[id]
	if p != nil && time.Now().After(p.expires) {
		delete(mcpPending.m, id)
		p = nil
	}
	mcpPending.Unlock()
	if p == nil {
		jsonError(w, "This authorization request has expired. Start the connection in the application again.", 404)
		return
	}
	redirectHost := ""
	if u, err := url.Parse(p.RedirectURI); err == nil {
		redirectHost = u.Host
	}
	switch r.Method {
	case http.MethodGet:
		ok, why := mcpUserAllowed(userID)
		jsonOK(w, map[string]interface{}{"client_name": p.ClientName, "client_id": p.ClientID, "redirect_host": redirectHost,
			"redirect_uri": p.RedirectURI, "scopes": p.Scopes, "enabled": ok, "reason": why})
	case http.MethodPost:
		var in struct {
			Decision string `json:"decision"`
			mcpGrantRequest
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		take := func() bool {
			mcpPending.Lock()
			defer mcpPending.Unlock()
			if mcpPending.m[id] != p {
				return false
			}
			delete(mcpPending.m, id)
			return true
		}
		params := url.Values{"iss": {mcpBaseURL(r)}}
		if p.State != "" {
			params.Set("state", p.State)
		}
		if in.Decision != "approve" {
			if !take() {
				jsonError(w, "This authorization request was already answered", 409)
				return
			}
			params.Set("error", "access_denied")
			params.Set("error_description", "the user denied the request")
			auditLog(r, userID, "mcp.authorize_denied", p.ClientName, map[string]interface{}{"client_id": p.ClientID})
			jsonOK(w, map[string]string{"redirect": withQuery(p.RedirectURI, params)})
			return
		}
		q := in.mcpGrantRequest
		if q.Name == "" {
			q.Name = p.ClientName
		}
		if err := q.validate(userID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if !take() {
			jsonError(w, "This authorization request was already answered", 409)
			return
		}
		code := randomToken(32)
		mcpCodes.Lock()
		mcpCodes.m[code] = &mcpAuthCode{UserID: userID, ClientID: p.ClientID, ClientName: p.ClientName, RedirectURI: p.RedirectURI,
			Challenge: p.Challenge, Resource: p.Resource, Req: q, expires: time.Now().Add(mcpCodeTTL)}
		mcpCodes.Unlock()
		db.Exec(`UPDATE mcp_clients SET last_used_at=? WHERE client_id=?`, nowRFC(), p.ClientID)
		auditLog(r, userID, "mcp.authorized", p.ClientName, map[string]interface{}{"client_id": p.ClientID, "redirect_host": redirectHost,
			"conn_ids": fmt.Sprint(q.ConnIDs), "mode": q.Mode, "scopes": strings.Join(q.Scopes, ","), "hours": q.Hours})
		params.Set("code", code)
		jsonOK(w, map[string]string{"redirect": withQuery(p.RedirectURI, params)})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func withQuery(raw string, params url.Values) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	for k, v := range params {
		for _, x := range v {
			q.Add(k, x)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// ─── token ───────────────────────────────────────────

func pkceS256(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

var pkceVerifierRe = regexp.MustCompile(`^[A-Za-z0-9\-._~]{43,128}$`)

// mcpClientAuth authenticates the client at the token and revocation endpoints.
func mcpClientAuth(r *http.Request) (*mcpClient, string) {
	cid, secret, basic := r.BasicAuth()
	if !basic {
		cid, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	} else {
		cid, _ = url.QueryUnescape(cid)
		secret, _ = url.QueryUnescape(secret)
	}
	c, err := loadMCPClient(cid)
	if err != nil {
		return nil, "unknown client"
	}
	if c.AuthMethod != "none" {
		if secret == "" || subtle.ConstantTimeCompare([]byte(sha256Hex(secret)), []byte(c.SecretHash)) != 1 {
			return nil, "client authentication failed"
		}
	}
	return c, ""
}

func mcpTokenHandler(w http.ResponseWriter, r *http.Request) {
	if !mcpOAuthAvailable() {
		http.NotFound(w, r)
		return
	}
	if !mcpTokenEndpointCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		oauthError(w, 405, "invalid_request", "use POST")
		return
	}
	if !mcpRateOK("tok:"+clientIP(r), 60, 10*time.Minute) {
		oauthError(w, 429, "invalid_request", "too many token requests from this address")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		oauthError(w, 400, "invalid_request", "invalid form")
		return
	}
	f := r.PostForm
	if f.Get("grant_type") != "authorization_code" {
		oauthError(w, 400, "unsupported_grant_type", "only authorization_code is supported; approve the application in WRM again when its access expires")
		return
	}
	c, why := mcpClientAuth(r)
	if c == nil {
		oauthError(w, 401, "invalid_client", why)
		return
	}
	code := f.Get("code")
	mcpCodes.Lock()
	ac := mcpCodes.m[code]
	if ac != nil && ac.used {
		// a code used twice: revoke what the first use issued (RFC 6749 §4.1.2)
		gid := ac.grantID
		mcpCodes.Unlock()
		if gid > 0 {
			mcpRevoke(r, gid, 0, "the authorization code was used twice")
		}
		oauthError(w, 400, "invalid_grant", "the authorization code was already used")
		return
	}
	if ac != nil {
		ac.used = true
	}
	mcpCodes.Unlock()
	if ac == nil || time.Now().After(ac.expires) || ac.ClientID != c.ClientID {
		oauthError(w, 400, "invalid_grant", "invalid or expired authorization code")
		return
	}
	if f.Get("redirect_uri") != ac.RedirectURI {
		oauthError(w, 400, "invalid_grant", "redirect_uri does not match")
		return
	}
	verifier := f.Get("code_verifier")
	if !pkceVerifierRe.MatchString(verifier) || subtle.ConstantTimeCompare([]byte(pkceS256(verifier)), []byte(ac.Challenge)) != 1 {
		auditLogAs(r, ac.UserID, usernameOf(ac.UserID), "mcp.pkce_failed", ac.ClientName, map[string]interface{}{"client_id": c.ClientID})
		oauthError(w, 400, "invalid_grant", "PKCE verification failed")
		return
	}
	if res := f.Get("resource"); res != "" && !mcpResourceOK(r, res) {
		oauthError(w, 400, "invalid_target", "unknown resource")
		return
	}
	q := ac.Req
	if err := q.validate(ac.UserID); err != nil {
		oauthError(w, 400, "invalid_grant", err.Error())
		return
	}
	g, tok, err := mcpCreateGrant(r, ac.UserID, q, "oauth", c.ClientID, c.Name)
	if err != nil {
		oauthError(w, 500, "server_error", err.Error())
		return
	}
	mcpCodes.Lock()
	ac.grantID = g.ID
	mcpCodes.Unlock()
	db.Exec(`UPDATE mcp_clients SET last_used_at=? WHERE client_id=?`, nowRFC(), c.ClientID)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	json.NewEncoder(w).Encode(map[string]interface{}{"access_token": tok, "token_type": "Bearer",
		"expires_in": int(time.Until(g.ExpiresAt).Seconds()), "scope": strings.Join(g.Scopes, " ")})
}

func mcpRevokeHandler(w http.ResponseWriter, r *http.Request) {
	if !mcpOAuthAvailable() {
		http.NotFound(w, r)
		return
	}
	if !mcpTokenEndpointCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		oauthError(w, 405, "invalid_request", "use POST")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		oauthError(w, 400, "invalid_request", "invalid form")
		return
	}
	if g, _ := mcpLookupBearer(r.PostForm.Get("token")); g != nil {
		if g.Kind == "oauth" {
			if c, why := mcpClientAuth(r); c == nil || c.ClientID != g.ClientID {
				oauthError(w, 401, "invalid_client", nonEmpty(why, "the token belongs to another client"))
				return
			}
		}
		mcpRevoke(r, g.ID, 0, "revoked by the application")
	}
	w.WriteHeader(200)
}
