// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ─── ADDON PLATFORM v2, STEP 2: PROXIED HTTP AND SECRETS ─────
//
// A panel has no network of its own (connect-src 'none'). With the http
// capability it describes a request (wrm:http) and WRM performs it: only to
// the origins and paths addon.json lists, only after an administrator
// approved those origins, never to loopback, link-local, private or metadata
// addresses unless the administrator allowed private network access for that
// origin, with a secret WRM adds itself. The panel references a secret by
// name and never sees its value.

const (
	addonHTTPMaxOrigins   = 8
	addonHTTPMaxSecrets   = 8
	addonHTTPMaxBody      = 256 << 10
	addonHTTPMaxResponse  = 8 << 20
	addonHTTPTimeout      = 10 * time.Second
	addonHTTPMaxRedirects = 3
	addonHTTPPerMinute    = 120
	addonHTTPConcurrent   = 20
	addonHTTPCoalesce     = 2 * time.Second
	addonSecretMaxBytes   = 4 << 10
	addonHTTPLogKeep      = 20000
)

var (
	addonSecretNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{0,31}$`)
	addonHeaderNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,39}$`)
	addonHTTPMethods       = map[string]bool{"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
	// Headers a secret may not be sent as: they belong to the connection,
	// to WRM or to the browser.
	addonForbiddenHeaders = map[string]bool{"host": true, "cookie": true, "set-cookie": true, "content-length": true,
		"transfer-encoding": true, "connection": true, "upgrade": true, "te": true, "trailer": true, "origin": true,
		"referer": true, "keep-alive": true, "content-type": true, "accept": true, "range": true}
	// Request headers a panel may set; everything else is dropped.
	addonHTTPRequestHeaders = []string{"Accept", "Content-Type", "Range"}
	// Response headers a panel gets back.
	addonHTTPResponseHeaders = []string{"Content-Type", "Content-Length", "Content-Range", "ETag", "Last-Modified",
		"Cache-Control", "Date", "Retry-After"}
	// addonHTTPTestRoots lets tests trust their own TLS server.
	addonHTTPTestRoots *x509.CertPool
)

// addonHTTP is the http block of addon.json.
type addonHTTP struct {
	Origins []string `json:"origins"`
	Paths   []string `json:"paths,omitempty"`
	Methods []string `json:"methods,omitempty"`
	Secrets []string `json:"secrets,omitempty"`
}

func (h *addonHTTP) methods() []string {
	if h == nil || len(h.Methods) == 0 {
		return []string{"GET"}
	}
	return h.Methods
}

func (h *addonHTTP) paths() []string {
	if h == nil || len(h.Paths) == 0 {
		return []string{"/*"}
	}
	return h.Paths
}

// addonSecretSpec is one entry of http.secrets: "name" (sent as a bearer
// token), "bearer:name", "basic:name" (the value is user:password) or
// "header:Header-Name:name".
type addonSecretSpec struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Header string `json:"header,omitempty"`
}

func addonParseSecret(s string) (addonSecretSpec, error) {
	parts := strings.Split(s, ":")
	var sp addonSecretSpec
	switch {
	case len(parts) == 1:
		sp = addonSecretSpec{Name: parts[0], Kind: "bearer"}
	case len(parts) == 2 && (parts[0] == "bearer" || parts[0] == "basic"):
		sp = addonSecretSpec{Name: parts[1], Kind: parts[0]}
	case len(parts) == 3 && parts[0] == "header":
		if !addonHeaderNamePattern.MatchString(parts[1]) || addonForbiddenHeaders[strings.ToLower(parts[1])] ||
			strings.HasPrefix(strings.ToLower(parts[1]), "proxy-") || strings.HasPrefix(strings.ToLower(parts[1]), "x-forwarded-") {
			return sp, fmt.Errorf("http.secrets: %q cannot carry a secret", parts[1])
		}
		sp = addonSecretSpec{Name: parts[2], Kind: "header", Header: parts[1]}
	default:
		return sp, fmt.Errorf("http.secrets entries are name, bearer:name, basic:name or header:Header-Name:name, not %q", s)
	}
	if !addonSecretNamePattern.MatchString(sp.Name) {
		return sp, fmt.Errorf("http.secrets: the name %q must be 1-32 lowercase letters, digits or underscores", sp.Name)
	}
	return sp, nil
}

func (h *addonHTTP) secretSpecs() []addonSecretSpec {
	out := []addonSecretSpec{}
	if h == nil {
		return out
	}
	for _, s := range h.Secrets {
		if sp, err := addonParseSecret(s); err == nil {
			out = append(out, sp)
		}
	}
	return out
}

func (h *addonHTTP) secretSpec(name string) (addonSecretSpec, bool) {
	for _, sp := range h.secretSpecs() {
		if sp.Name == name {
			return sp, true
		}
	}
	return addonSecretSpec{}, false
}

// addonNormalizeOrigin returns scheme://host[:port] in lowercase with the
// default port left out, or an error for anything that is not a bare origin.
func addonNormalizeOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("http.origins: %q is not an origin like https://example.com or https://example.com:8443", raw)
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		return u.Scheme + "://" + host + ":" + port, nil
	}
	return u.Scheme + "://" + host, nil
}

func addonValidPathPattern(p string) bool {
	if !strings.HasPrefix(p, "/") || len(p) > 128 || strings.ContainsAny(p, "?#\\ ") || strings.Contains(p, "..") {
		return false
	}
	if i := strings.Index(p, "*"); i >= 0 && i != len(p)-1 {
		return false
	}
	return true
}

// addonPathAllowed: "/api/*" allows /api/ and below, "/status" only /status.
func addonPathAllowed(path string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(p, "*") {
			if strings.HasPrefix(path, strings.TrimSuffix(p, "*")) {
				return true
			}
		} else if path == p {
			return true
		}
	}
	return false
}

func addonValidateHTTP(m *addonManifest, seen map[string]bool) error {
	h := m.HTTP
	if h == nil {
		if seen["http"] {
			return fmt.Errorf("the http capability needs an http block with origins")
		}
		return nil
	}
	if !seen["http"] {
		return fmt.Errorf("an http block requires the http capability")
	}
	if m.APIVersion < 2 || m.UI == nil || m.UI.Panel == "" {
		return fmt.Errorf("the http capability is used from a panel: declare \"apiVersion\": 2 and ui.panel")
	}
	if len(h.Origins) == 0 || len(h.Origins) > addonHTTPMaxOrigins {
		return fmt.Errorf("http.origins must list 1-%d origins", addonHTTPMaxOrigins)
	}
	dup := map[string]bool{}
	for _, o := range h.Origins {
		n, err := addonNormalizeOrigin(o)
		if err != nil {
			return err
		}
		if n != o {
			return fmt.Errorf("http.origins: write %q as %q", o, n)
		}
		if dup[n] {
			return fmt.Errorf("http.origins: %q is listed twice", o)
		}
		dup[n] = true
	}
	if len(h.Paths) > 16 {
		return fmt.Errorf("http.paths may list at most 16 paths")
	}
	for _, p := range h.Paths {
		if !addonValidPathPattern(p) {
			return fmt.Errorf("http.paths: %q must start with / and may end with * (no .., ? or #)", p)
		}
	}
	for _, x := range h.Methods {
		if !addonHTTPMethods[x] {
			return fmt.Errorf("http.methods: %q is not one of GET, HEAD, POST, PUT, PATCH, DELETE", x)
		}
	}
	if len(h.Secrets) > addonHTTPMaxSecrets {
		return fmt.Errorf("http.secrets may list at most %d secrets", addonHTTPMaxSecrets)
	}
	names := map[string]bool{}
	for _, s := range h.Secrets {
		sp, err := addonParseSecret(s)
		if err != nil {
			return err
		}
		if names[sp.Name] {
			return fmt.Errorf("http.secrets: %q is listed twice", sp.Name)
		}
		names[sp.Name] = true
	}
	return nil
}

// addonHTTPPermissions: one atom per origin and secret, plus the methods and
// paths, so an update that reaches further asks again.
func addonHTTPPermissions(h *addonHTTP) []string {
	if h == nil {
		return nil
	}
	out := []string{}
	for _, o := range h.Origins {
		out = append(out, "http:"+o)
	}
	ms := append([]string(nil), h.methods()...)
	sort.Strings(ms)
	ps := append([]string(nil), h.paths()...)
	sort.Strings(ps)
	out = append(out, "http.methods:"+strings.Join(ms, ","), "http.paths:"+strings.Join(ps, ","))
	for _, sp := range h.secretSpecs() {
		out = append(out, "secret:"+sp.Name)
	}
	return out
}

// addonHTTPPermCovered handles http.methods (a subset is covered) and
// http.paths (every path within an approved pattern is covered).
func addonHTTPPermCovered(kind, val, bv string) bool {
	switch kind {
	case "http.methods":
		have := map[string]bool{}
		for _, x := range strings.Split(bv, ",") {
			have[x] = true
		}
		for _, x := range strings.Split(val, ",") {
			if !have[x] {
				return false
			}
		}
		return true
	case "http.paths":
		base := strings.Split(bv, ",")
		for _, p := range strings.Split(val, ",") {
			if !addonPathAllowed(p, base) && !addonPathAllowed(strings.TrimSuffix(p, "*"), base) {
				return false
			}
			if strings.HasSuffix(p, "*") && !addonPatternWithin(p, base) {
				return false
			}
		}
		return true
	}
	return false
}

// addonPatternWithin: the wildcard pattern p allows nothing outside base.
func addonPatternWithin(p string, base []string) bool {
	prefix := strings.TrimSuffix(p, "*")
	for _, b := range base {
		if strings.HasSuffix(b, "*") && strings.HasPrefix(prefix, strings.TrimSuffix(b, "*")) {
			return true
		}
	}
	return false
}

// ── schema ───────────────────────────────────────────

func initAddonHTTPSchema() {
	// An administrator's approval of one origin of one addon, and whether
	// private network addresses (and plain http) are allowed for it.
	ensureTable("addon_http_grants", `CREATE TABLE IF NOT EXISTS addon_http_grants (
		addon_owner INTEGER NOT NULL DEFAULT 0,
		addon_id TEXT NOT NULL,
		origin TEXT NOT NULL,
		private INTEGER NOT NULL DEFAULT 0,
		approved_by TEXT NOT NULL DEFAULT '',
		approved_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (addon_owner, addon_id, origin)
	)`)
	// Secret values per instance: user_id 0 is the instance's value, a user
	// id is that user's own value in a shared instance. Encrypted at rest.
	ensureTable("addon_secrets", `CREATE TABLE IF NOT EXISTS addon_secrets (
		instance_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL DEFAULT 0,
		name TEXT NOT NULL,
		value TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		updated_by TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (instance_id, user_id, name)
	)`)
	ensureTable("addon_http_log", `CREATE TABLE IF NOT EXISTS addon_http_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		at TEXT NOT NULL DEFAULT '',
		user_id INTEGER NOT NULL DEFAULT 0,
		addon TEXT NOT NULL DEFAULT '',
		instance_id INTEGER NOT NULL DEFAULT 0,
		method TEXT NOT NULL DEFAULT '',
		origin TEXT NOT NULL DEFAULT '',
		path TEXT NOT NULL DEFAULT '',
		status INTEGER NOT NULL DEFAULT 0,
		bytes INTEGER NOT NULL DEFAULT 0,
		ms INTEGER NOT NULL DEFAULT 0,
		code TEXT NOT NULL DEFAULT ''
	)`)
	mustExec(`CREATE INDEX IF NOT EXISTS idx_addon_http_log_instance ON addon_http_log(instance_id, id)`)
}

// ── grants ───────────────────────────────────────────

type addonHTTPGrant struct {
	Origin     string `json:"origin"`
	Private    bool   `json:"private"`
	ApprovedBy string `json:"approved_by"`
	ApprovedAt string `json:"approved_at"`
}

func addonHTTPGrants(ref addonRef) map[string]addonHTTPGrant {
	out := map[string]addonHTTPGrant{}
	rows, err := db.Query(`SELECT origin, private, approved_by, approved_at FROM addon_http_grants WHERE addon_owner=? AND addon_id=?`, ref.Owner, ref.ID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var g addonHTTPGrant
		var p int
		if rows.Scan(&g.Origin, &p, &g.ApprovedBy, &g.ApprovedAt) == nil {
			g.Private = p == 1
			out[g.Origin] = g
		}
	}
	return out
}

// addonHTTPPrivateSet checks that every origin allowed the private network
// is one of the addon's.
func addonHTTPPrivateSet(rec addonRecord, private []string) (map[string]bool, error) {
	priv := map[string]bool{}
	listed := map[string]bool{}
	if rec.HTTP != nil {
		for _, o := range rec.HTTP.Origins {
			listed[o] = true
		}
	}
	for _, p := range private {
		n, err := addonNormalizeOrigin(p)
		if err != nil {
			return nil, err
		}
		if !listed[n] {
			return nil, fmt.Errorf("%s is not an origin of this addon", n)
		}
		priv[n] = true
	}
	return priv, nil
}

// addonHTTPApprove records an administrator's approval of every origin the
// addon lists; private names the origins that may reach private addresses.
func addonHTTPApprove(r *http.Request, userID int, rec addonRecord, private []string) error {
	if rec.HTTP == nil {
		return nil
	}
	ref := addonRef{Owner: rec.OwnerID, ID: rec.ID}
	priv, err := addonHTTPPrivateSet(rec, private)
	if err != nil {
		return err
	}
	by, now := usernameOf(userID), time.Now().UTC().Format(time.RFC3339)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	tx.Exec(`DELETE FROM addon_http_grants WHERE addon_owner=? AND addon_id=?`, ref.Owner, ref.ID)
	for _, o := range rec.HTTP.Origins {
		tx.Exec(`INSERT INTO addon_http_grants (addon_owner, addon_id, origin, private, approved_by, approved_at) VALUES (?,?,?,?,?,?)`,
			ref.Owner, ref.ID, o, boolToInt(priv[o]), by, now)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	privList := []string{}
	for p := range priv {
		privList = append(privList, p)
	}
	sort.Strings(privList)
	details := map[string]interface{}{"origins": rec.HTTP.Origins, "private_network": privList}
	if rec.OwnerID > 0 {
		details["personal_owner"] = usernameOf(rec.OwnerID)
	}
	auditLog(r, userID, "addon.http_approved", ref.Key(), details)
	return nil
}

func addonHTTPRevoke(r *http.Request, userID int, ref addonRef) {
	db.Exec(`DELETE FROM addon_http_grants WHERE addon_owner=? AND addon_id=?`, ref.Owner, ref.ID)
	auditLog(r, userID, "addon.http_revoked", ref.Key(), nil)
}

// addonHTTPStatus fills what the interface shows about network access.
func addonHTTPStatus(rec *addonRecord) {
	if rec.HTTP == nil {
		return
	}
	g := addonHTTPGrants(addonRef{Owner: rec.OwnerID, ID: rec.ID})
	rec.HTTPGrants = []addonHTTPGrant{}
	for _, o := range rec.HTTP.Origins {
		if x, ok := g[o]; ok {
			rec.HTTPGrants = append(rec.HTTPGrants, x)
		} else {
			rec.HTTPWaiting = true
		}
	}
	rec.Secrets = rec.HTTP.secretSpecs()
}

// addonHTTPGrantHandler: POST {private: [origins]} approves the addon's
// origins, DELETE withdraws the approval. Administrators only.
func addonHTTPGrantHandler(w http.ResponseWriter, r *http.Request, userID int, ref addonRef) {
	rec, ok := addonGetRef(ref)
	if !ok {
		addonFail(w, addonErrf("E_NOT_FOUND", "Addon not found"), 404)
		return
	}
	if rec.HTTP == nil {
		addonFail(w, addonErrf("E_CAPABILITY", "The addon does not use the http capability"), 400)
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Private []string `json:"private"`
		}
		if r.ContentLength != 0 {
			if json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req) != nil {
				jsonError(w, "Invalid request", 400)
				return
			}
		}
		if err := addonHTTPApprove(r, userID, rec, req.Private); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true, "grants": addonHTTPGrants(ref)})
	case http.MethodDelete:
		addonHTTPRevoke(r, userID, ref)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// ── secrets ──────────────────────────────────────────

func addonHTTPAllowed(userID int) bool {
	v := getSetting("addon_http")
	return v != "off" && policyAllows(userID, v)
}

// addonSecretValue resolves the value a request uses: the user's own value in
// a shared instance first, then the instance's value.
func addonSecretValue(in addonInstance, userID int, name string) string {
	var v string
	if in.Shared {
		if db.QueryRow(`SELECT value FROM addon_secrets WHERE instance_id=? AND user_id=? AND name=?`, in.ID, userID, name).Scan(&v) == nil && v != "" {
			return decryptValue(v)
		}
	}
	if db.QueryRow(`SELECT value FROM addon_secrets WHERE instance_id=? AND user_id=0 AND name=?`, in.ID, name).Scan(&v) == nil {
		return decryptValue(v)
	}
	return ""
}

type addonSecretState struct {
	addonSecretSpec
	InstanceSet  bool   `json:"instance_set"`
	OwnSet       bool   `json:"own_set"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	SetInstance  bool   `json:"can_set_instance"`
	SetOwn       bool   `json:"can_set_own"`
	InstanceUser string `json:"instance_set_by,omitempty"`
}

// addonSecretStates lists the addon's secrets for an instance: whether a
// value is set, never the value.
func addonSecretStates(rec addonRecord, in addonInstance, userID int, admin bool) []addonSecretState {
	out := []addonSecretState{}
	canInstance := (in.Shared && admin) || (!in.Shared && in.OwnerID == userID)
	for _, sp := range rec.HTTP.secretSpecs() {
		st := addonSecretState{addonSecretSpec: sp, SetInstance: canInstance, SetOwn: in.Shared}
		var at, by string
		if db.QueryRow(`SELECT updated_at, updated_by FROM addon_secrets WHERE instance_id=? AND user_id=0 AND name=? AND value<>''`, in.ID, sp.Name).Scan(&at, &by) == nil {
			st.InstanceSet, st.UpdatedAt, st.InstanceUser = true, at, by
		}
		if in.Shared && db.QueryRow(`SELECT updated_at FROM addon_secrets WHERE instance_id=? AND user_id=? AND name=? AND value<>''`, in.ID, userID, sp.Name).Scan(&at) == nil {
			st.OwnSet = true
		}
		out = append(out, st)
	}
	return out
}

// addonSecretsHandler serves .../instances/{id}/secrets[/{name}].
func addonSecretsHandler(w http.ResponseWriter, r *http.Request, userID int, admin bool, rec addonRecord, in addonInstance, rest []string) {
	if rec.HTTP == nil || len(rec.HTTP.Secrets) == 0 {
		addonFail(w, addonErrf("E_CAPABILITY", "The addon declares no secrets"), 400)
		return
	}
	if len(rest) == 0 {
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		jsonOK(w, map[string]interface{}{"secrets": addonSecretStates(rec, in, userID, admin)})
		return
	}
	sp, ok := rec.HTTP.secretSpec(rest[0])
	if len(rest) != 1 || !ok {
		addonFail(w, addonErrf("E_NOT_FOUND", "The addon declares no such secret"), 404)
		return
	}
	scope := r.URL.Query().Get("scope")
	var req struct {
		Value string `json:"value"`
		Scope string `json:"scope"`
	}
	if r.Method == http.MethodPut {
		if json.NewDecoder(io.LimitReader(r.Body, addonSecretMaxBytes+512)).Decode(&req) != nil {
			jsonError(w, "Invalid request", 400)
			return
		}
		scope = req.Scope
	}
	owner := 0
	switch scope {
	case "", "instance":
		if !((in.Shared && admin) || (!in.Shared && in.OwnerID == userID)) {
			jsonError(w, "An administrator sets the values of a shared instance; you can set your own value", 403)
			return
		}
		scope = "instance"
	case "own":
		if !in.Shared {
			jsonError(w, "Your own value is for shared instances", 400)
			return
		}
		owner = userID
	default:
		jsonError(w, "scope is instance or own", 400)
		return
	}
	details := map[string]interface{}{"instance": in.ID, "name": sp.Name, "scope": scope}
	switch r.Method {
	case http.MethodPut:
		v := req.Value
		if v == "" || len(v) > addonSecretMaxBytes || strings.ContainsAny(v, "\r\n\x00") || !utf8.ValidString(v) {
			jsonError(w, fmt.Sprintf("The value must be 1-%d characters on one line", addonSecretMaxBytes), 400)
			return
		}
		if sp.Kind == "basic" && !strings.Contains(v, ":") {
			jsonError(w, "This value is user:password", 400)
			return
		}
		_, err := db.Exec(`INSERT INTO addon_secrets (instance_id, user_id, name, value, updated_at, updated_by) VALUES (?,?,?,?,?,?)
			ON CONFLICT(instance_id, user_id, name) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at, updated_by=excluded.updated_by`,
			in.ID, owner, sp.Name, encryptValue(v), time.Now().UTC().Format(time.RFC3339), usernameOf(userID))
		if err != nil {
			jsonError(w, "Cannot save", 500)
			return
		}
		addonHTTPForgetCache(in.ID)
		auditLog(r, userID, "addon.http_auth_set", rec.Key, details)
		jsonOK(w, map[string]bool{"ok": true})
	case http.MethodDelete:
		db.Exec(`DELETE FROM addon_secrets WHERE instance_id=? AND user_id=? AND name=?`, in.ID, owner, sp.Name)
		addonHTTPForgetCache(in.ID)
		auditLog(r, userID, "addon.http_auth_cleared", rec.Key, details)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// ── the proxy ────────────────────────────────────────

type addonHTTPRequest struct {
	ID       string            `json:"id,omitempty"`
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers,omitempty"`
	Body     string            `json:"body,omitempty"`
	Encoding string            `json:"encoding,omitempty"`
	Secret   string            `json:"secret,omitempty"`
}

type addonHTTPResult struct {
	OK        bool              `json:"ok"`
	Status    int               `json:"status"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	Encoding  string            `json:"encoding"`
	Truncated bool              `json:"truncated,omitempty"`
	Code      string            `json:"code,omitempty"`
	Error     string            `json:"error,omitempty"`
	Shared    bool              `json:"coalesced,omitempty"`
}

// Per-instance rate limit, concurrency cap and coalescing of identical GETs.
type addonHTTPCall struct {
	done chan struct{}
	res  addonHTTPResult
	err  error
	at   time.Time
}

type addonHTTPLimiter struct {
	hits     []time.Time
	inflight int
}

var addonHTTPState = struct {
	sync.Mutex
	limits map[int]*addonHTTPLimiter
	calls  map[string]*addonHTTPCall
	logged int
}{limits: map[int]*addonHTTPLimiter{}, calls: map[string]*addonHTTPCall{}}

func addonHTTPAcquire(iid int) bool {
	addonHTTPState.Lock()
	defer addonHTTPState.Unlock()
	l := addonHTTPState.limits[iid]
	if l == nil {
		l = &addonHTTPLimiter{}
		addonHTTPState.limits[iid] = l
	}
	now := time.Now()
	keep := l.hits[:0]
	for _, t := range l.hits {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	l.hits = keep
	if len(l.hits) >= addonHTTPPerMinute || l.inflight >= addonHTTPConcurrent {
		return false
	}
	l.hits = append(l.hits, now)
	l.inflight++
	return true
}

func addonHTTPRelease(iid int) {
	addonHTTPState.Lock()
	if l := addonHTTPState.limits[iid]; l != nil && l.inflight > 0 {
		l.inflight--
	}
	addonHTTPState.Unlock()
}

func addonHTTPForgetCache(iid int) {
	addonHTTPState.Lock()
	prefix := strconv.Itoa(iid) + "|"
	for k, c := range addonHTTPState.calls {
		if strings.HasPrefix(k, prefix) && c.res.Status != 0 {
			delete(addonHTTPState.calls, k)
		}
	}
	addonHTTPState.Unlock()
}

// addonHTTPDialer refuses addresses the request may not reach. It resolves
// the name itself and connects to the address it checked, so a name that
// changes its answer between the check and the connection gets nowhere.
type addonHTTPDialer struct {
	allowPrivate map[string]bool // host:port → private network allowed
}

type addonBlockedError struct{ addr string }

func (e *addonBlockedError) Error() string {
	return "the address " + e.addr + " is in a private, local or reserved range"
}

func addonIPBlocked(ip net.IP) bool {
	// IPv6 forms that carry an IPv4 address (NAT64 64:ff9b::/96, 6to4 2002::/16)
	// are judged by that address.
	if len(ip) == net.IPv6len && ip.To4() == nil {
		if ip[0] == 0x00 && ip[1] == 0x64 && ip[2] == 0xff && ip[3] == 0x9b && addonAllZero(ip[4:12]) {
			return addonIPBlocked(net.IPv4(ip[12], ip[13], ip[14], ip[15]))
		}
		if ip[0] == 0x20 && ip[1] == 0x02 {
			return addonIPBlocked(net.IPv4(ip[2], ip[3], ip[4], ip[5]))
		}
	}
	return ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsPrivate() || addonIsCGNAT(ip) ||
		ip.Equal(net.IPv4bcast) || (ip.To4() != nil && ip.To4()[0] == 0) || (ip.To4() != nil && ip.To4()[0] >= 240)
}

// addonIsCGNAT reports 100.64.0.0/10 (turn.go's isCGNAT is not built on Android).
func addonIsCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64
}

func addonAllZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

func (d *addonHTTPDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	private := d.allowPrivate[strings.ToLower(net.JoinHostPort(host, port))]
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var dialer net.Dialer
	var lastErr error = fmt.Errorf("no address for %s", host)
	for _, ia := range ips {
		if !private && addonIPBlocked(ia.IP) {
			lastErr = &addonBlockedError{addr: ia.IP.String()}
			continue
		}
		c, err := dialer.DialContext(ctx, network, net.JoinHostPort(ia.IP.String(), port))
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func addonOriginOf(u *url.URL) string {
	n, err := addonNormalizeOrigin(u.Scheme + "://" + u.Host)
	if err != nil {
		return ""
	}
	return n
}

func addonOriginHostPort(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	return strings.ToLower(net.JoinHostPort(u.Hostname(), port))
}

// addonPathHasDotDot catches traversal after percent-decoding (%2e%2e), which the
// escaped path does not contain.
func addonPathHasDotDot(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." {
			return true
		}
	}
	return false
}

// addonHTTPCheckURL validates one URL (the request or a redirect) against
// the manifest and the approvals.
func addonHTTPCheckURL(rec addonRecord, grants map[string]addonHTTPGrant, u *url.URL) (string, error) {
	if u.User != nil {
		return "", addonErrf("E_HTTP_ORIGIN", "URLs with a user name or password are not allowed")
	}
	origin := addonOriginOf(u)
	listed := false
	for _, o := range rec.HTTP.Origins {
		listed = listed || o == origin
	}
	if !listed {
		return "", addonErrf("E_HTTP_ORIGIN", "%s is not in http.origins", origin)
	}
	rawPath := u.EscapedPath()
	if rawPath == "" {
		rawPath = "/"
	}
	path, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", addonErrf("E_HTTP_PATH", "The path %s is not valid", rawPath)
	}
	if addonPathHasDotDot(path) || !addonPathAllowed(path, rec.HTTP.paths()) {
		return "", addonErrf("E_HTTP_PATH", "The path %s is not in http.paths", path)
	}
	g, ok := grants[origin]
	if !ok {
		return "", addonErrf("E_HTTP_UNAPPROVED", "An administrator has not approved %s for this addon yet", origin)
	}
	if u.Scheme == "http" && !g.Private {
		return "", addonErrf("E_HTTP_SCHEME", "Plain http is only allowed for an origin the administrator approved for the private network")
	}
	return origin, nil
}

func addonHTTPClient(rec addonRecord, grants map[string]addonHTTPGrant, firstOrigin string, auth func(*http.Request)) *http.Client {
	priv := map[string]bool{}
	for o, g := range grants {
		if g.Private {
			priv[addonOriginHostPort(o)] = true
		}
	}
	tlsConf := &tls.Config{MinVersion: tls.VersionTLS12}
	if addonHTTPTestRoots != nil {
		tlsConf.RootCAs = addonHTTPTestRoots
	}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&addonHTTPDialer{allowPrivate: priv}).DialContext,
		TLSClientConfig:       tlsConf,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   addonHTTPTimeout,
		ResponseHeaderTimeout: addonHTTPTimeout,
	}
	return &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > addonHTTPMaxRedirects {
				return addonErrf("E_HTTP_REDIRECT", "More than %d redirects", addonHTTPMaxRedirects)
			}
			origin, err := addonHTTPCheckURL(rec, grants, req.URL)
			if err != nil {
				return err
			}
			// The secret goes only to the origin it was meant for.
			req.Header.Del("Authorization")
			for _, sp := range rec.HTTP.secretSpecs() {
				if sp.Header != "" {
					req.Header.Del(sp.Header)
				}
			}
			if origin == firstOrigin && auth != nil {
				auth(req)
			}
			return nil
		},
	}
}

func addonTextual(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "text/") || strings.Contains(ct, "json") || strings.Contains(ct, "xml") ||
		strings.Contains(ct, "javascript") || strings.Contains(ct, "csv") || strings.Contains(ct, "yaml") || ct == ""
}

var addonHTTPURLShape = regexp.MustCompile(`(?i)^https?://([a-z0-9.\-]+|\[[0-9a-f:.]+\])(:[0-9]{1,5})?([/?][^\s#]*)?$`)

// addonHTTPPrepared is a checked request: URL, origin, body and secret.
type addonHTTPPrepared struct {
	u                  *url.URL
	origin             string
	body               []byte
	auth               func(*http.Request)
	logOrigin, logPath string
}

// addonHTTPPrepare checks a panel's request against the manifest and the
// approvals: method, body, URL (origin, path, grant) and secret. secret
// returns the value of a declared secret, or "" when none is set. The status
// is the HTTP status to answer an error with.
func addonHTTPPrepare(rec addonRecord, grants map[string]addonHTTPGrant, req *addonHTTPRequest, secret func(name string) string) (addonHTTPPrepared, int, error) {
	var prep addonHTTPPrepared
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	if req.Method == "" {
		req.Method = "GET"
	}
	allowedMethod := false
	for _, m := range rec.HTTP.methods() {
		allowedMethod = allowedMethod || m == req.Method
	}
	if !allowedMethod {
		return prep, 403, addonErrf("E_HTTP_METHOD", "%s is not in http.methods", req.Method)
	}
	if req.Body != "" {
		if req.Encoding == "base64" {
			b, err := base64.StdEncoding.DecodeString(req.Body)
			if err != nil {
				return prep, 400, addonErrf("E_HTTP_REQUEST", "The body is not valid base64")
			}
			prep.body = b
		} else {
			prep.body = []byte(req.Body)
		}
	}
	if len(prep.body) > addonHTTPMaxBody {
		return prep, 413, addonErrf("E_LIMIT_SIZE", "The request body is %d bytes; %d KB at most", len(prep.body), addonHTTPMaxBody>>10)
	}
	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return prep, 400, addonErrf("E_HTTP_REQUEST", "url must be an absolute http(s) URL")
	}
	u.Fragment = ""
	prep.u = u
	prep.logOrigin, prep.logPath = strings.ToLower(u.Scheme+"://"+u.Host), u.EscapedPath()
	origin, err := addonHTTPCheckURL(rec, grants, u)
	if err != nil {
		return prep, 403, err
	}
	prep.origin = origin
	if req.Secret != "" {
		sp, ok := rec.HTTP.secretSpec(req.Secret)
		if !ok {
			return prep, 403, addonErrf("E_HTTP_SECRET", "%q is not in http.secrets", req.Secret)
		}
		value := secret(sp.Name)
		if value == "" {
			return prep, 409, addonErrf("E_SECRET_MISSING", "The secret %s has no value for this instance", sp.Name)
		}
		prep.auth = addonHTTPAuthFunc(sp, value)
	}
	return prep, 0, nil
}

// addonHTTPAuthFunc adds a secret to a request the way http.secrets declares it.
func addonHTTPAuthFunc(sp addonSecretSpec, value string) func(*http.Request) {
	return func(q *http.Request) {
		switch sp.Kind {
		case "basic":
			q.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(value)))
		case "header":
			q.Header.Set(sp.Header, value)
		default:
			q.Header.Set("Authorization", "Bearer "+value)
		}
	}
}

// addonHTTPDo performs a validated request.
func addonHTTPDo(ctx context.Context, rec addonRecord, grants map[string]addonHTTPGrant, req addonHTTPRequest, u *url.URL, origin string, body []byte, auth func(*http.Request)) (addonHTTPResult, error) {
	ctx, cancel := context.WithTimeout(ctx, addonHTTPTimeout)
	defer cancel()
	var rd io.Reader
	if len(body) > 0 {
		rd = bytes.NewReader(body)
	}
	// The URL passed addonHTTPCheckURL (listed origin, approved, allowed path); its
	// final form must still be a plain absolute URL without user info or whitespace.
	target := u.String()
	if !addonHTTPURLShape.MatchString(target) {
		return addonHTTPResult{}, addonErrf("E_HTTP_REQUEST", "url must be an absolute http(s) URL")
	}
	up, err := http.NewRequestWithContext(ctx, req.Method, target, rd)
	if err != nil {
		return addonHTTPResult{}, addonErrf("E_HTTP_REQUEST", "Invalid request: %v", err)
	}
	for _, h := range addonHTTPRequestHeaders {
		for k, v := range req.Headers {
			if strings.EqualFold(k, h) && !strings.ContainsAny(v, "\r\n") && len(v) <= 1024 {
				up.Header.Set(h, v)
			}
		}
	}
	up.Header.Set("User-Agent", "WRM-PRO-addon/"+strings.TrimPrefix(AppVersion, "v")+" ("+rec.ID+")")
	if auth != nil {
		auth(up)
	}
	resp, err := addonHTTPClient(rec, grants, origin, auth).Do(up)
	if err != nil {
		var ae *addonError
		var be *addonBlockedError
		switch {
		case errors.As(err, &ae):
			return addonHTTPResult{}, ae
		case errors.As(err, &be):
			return addonHTTPResult{}, addonErrf("E_HTTP_BLOCKED", "%s resolves to a private or local address (%s); an administrator can allow the private network for this site", origin, be.addr)
		case ctx.Err() == context.DeadlineExceeded:
			return addonHTTPResult{}, addonErrf("E_HTTP_TIMEOUT", "%s did not answer within %d seconds", origin, int(addonHTTPTimeout/time.Second))
		}
		return addonHTTPResult{}, addonErrf("E_HTTP_UPSTREAM", "%s cannot be reached: %s", origin, addonShortNetErr(err))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, addonHTTPMaxResponse+1))
	res := addonHTTPResult{OK: resp.StatusCode >= 200 && resp.StatusCode < 300, Status: resp.StatusCode, Headers: map[string]string{}}
	if len(data) > addonHTTPMaxResponse {
		data = data[:addonHTTPMaxResponse]
		res.Truncated, res.Code = true, "E_LIMIT_SIZE"
		res.Error = fmt.Sprintf("The response was cut at %d MB", addonHTTPMaxResponse>>20)
	}
	for _, h := range addonHTTPResponseHeaders {
		if v := resp.Header.Get(h); v != "" {
			res.Headers[h] = v
		}
	}
	if addonTextual(resp.Header.Get("Content-Type")) && utf8.Valid(data) {
		res.Body, res.Encoding = string(data), "text"
	} else {
		res.Body, res.Encoding = base64.StdEncoding.EncodeToString(data), "base64"
	}
	return res, nil
}

// addonShortNetErr keeps the part of a network error a person can act on.
func addonShortNetErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "the name " + dnsErr.Name + " cannot be resolved"
	}
	s := err.Error()
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func addonHTTPLog(userID int, rec addonRecord, in addonInstance, method, origin, path string, status, n int, d time.Duration, code string) {
	db.Exec(`INSERT INTO addon_http_log (at, user_id, addon, instance_id, method, origin, path, status, bytes, ms, code) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		time.Now().UTC().Format(time.RFC3339), userID, rec.Key, in.ID, method, origin, path, status, n, d.Milliseconds(), code)
	addonHTTPState.Lock()
	addonHTTPState.logged++
	prune := addonHTTPState.logged%500 == 0
	addonHTTPState.Unlock()
	if prune {
		db.Exec(`DELETE FROM addon_http_log WHERE id <= (SELECT MAX(id) FROM addon_http_log) - ?`, addonHTTPLogKeep)
	}
}

// addonHTTPHandler serves POST .../instances/{id}/http and GET .../http/log.
func addonHTTPHandler(w http.ResponseWriter, r *http.Request, userID int, admin bool, rec addonRecord, in addonInstance, rest []string) {
	if len(rest) == 1 && rest[0] == "log" && r.Method == http.MethodGet {
		addonHTTPLogHandler(w, userID, admin, in)
		return
	}
	if len(rest) != 0 || r.Method != http.MethodPost {
		jsonError(w, "Not found", 404)
		return
	}
	var req addonHTTPRequest
	var logOrigin, logPath string
	fail := func(err error, status int) {
		var ae *addonError
		code := ""
		if errors.As(err, &ae) {
			code = ae.Code
		}
		addonHTTPLog(userID, rec, in, req.Method, logOrigin, logPath, 0, 0, 0, code)
		addonFail(w, err, status)
	}
	if !rec.Enabled {
		addonFail(w, addonErrf("E_DISABLED", "The addon is disabled"), 409)
		return
	}
	if !addonHasCap(rec, "http") || rec.HTTP == nil {
		addonFail(w, addonErrf("E_CAPABILITY", "The addon does not declare the http capability"), 403)
		return
	}
	if !addonHTTPAllowed(userID) {
		addonFail(w, addonErrf("E_HTTP_OFF", "Addon network access is turned off for you (addon_http)"), 403)
		return
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, addonHTTPMaxBody*2+16<<10)).Decode(&req); err != nil {
		addonFail(w, addonErrf("E_HTTP_REQUEST", "The request is not valid JSON"), 400)
		return
	}
	ref := addonRef{Owner: rec.OwnerID, ID: rec.ID}
	grants := addonHTTPGrants(ref)
	prep, status, err := addonHTTPPrepare(rec, grants, &req, func(name string) string { return addonSecretValue(in, userID, name) })
	logOrigin, logPath = prep.logOrigin, prep.logPath
	if err != nil {
		fail(err, status)
		return
	}
	u, origin, body, auth := prep.u, prep.origin, prep.body, prep.auth
	if !addonHTTPAcquire(in.ID) {
		addonFail(w, addonErrf("E_LIMIT_RATE", "At most %d requests a minute and %d at a time per instance", addonHTTPPerMinute, addonHTTPConcurrent), 429)
		return
	}
	defer addonHTTPRelease(in.ID)

	start := time.Now()
	run := func() (addonHTTPResult, error) {
		return addonHTTPDo(r.Context(), rec, grants, req, u, origin, body, auth)
	}
	var res addonHTTPResult
	if req.Method == "GET" || req.Method == "HEAD" {
		hk := []string{}
		for k, v := range req.Headers {
			hk = append(hk, strings.ToLower(k)+"="+v)
		}
		sort.Strings(hk)
		scope := 0
		if in.Shared {
			scope = userID
		}
		key := fmt.Sprintf("%d|%d|%s|%s|%s|%s", in.ID, scope, req.Method, u.String(), strings.Join(hk, "&"), req.Secret)
		res, err = addonHTTPCoalesced(key, run)
	} else {
		res, err = run()
	}
	path := u.EscapedPath()
	if err != nil {
		var ae *addonError
		code, status := "E_HTTP_UPSTREAM", 502
		if errors.As(err, &ae) {
			code = ae.Code
		}
		switch code {
		case "E_HTTP_TIMEOUT":
			status = 504
		case "E_HTTP_ORIGIN", "E_HTTP_PATH", "E_HTTP_UNAPPROVED", "E_HTTP_SCHEME", "E_HTTP_BLOCKED", "E_HTTP_REDIRECT":
			status = 403
		}
		if code == "E_HTTP_BLOCKED" || code == "E_HTTP_REDIRECT" {
			auditLog(r, userID, "addon.http_refused", rec.Key, map[string]interface{}{"instance": in.ID, "origin": origin, "path": path, "code": code})
		}
		addonHTTPLog(userID, rec, in, req.Method, origin, path, 0, 0, time.Since(start), code)
		addonFail(w, err, status)
		return
	}
	addonHTTPLog(userID, rec, in, req.Method, origin, path, res.Status, len(res.Body), time.Since(start), res.Code)
	jsonOK(w, res)
}

// addonHTTPCoalesced shares one upstream call among identical GETs that
// arrive within two seconds.
func addonHTTPCoalesced(key string, run func() (addonHTTPResult, error)) (addonHTTPResult, error) {
	addonHTTPState.Lock()
	for k, c := range addonHTTPState.calls {
		select {
		case <-c.done:
			if time.Since(c.at) > addonHTTPCoalesce {
				delete(addonHTTPState.calls, k)
			}
		default:
		}
	}
	if c, ok := addonHTTPState.calls[key]; ok {
		addonHTTPState.Unlock()
		<-c.done
		res := c.res
		res.Shared = true
		return res, c.err
	}
	c := &addonHTTPCall{done: make(chan struct{}), at: time.Now()}
	addonHTTPState.calls[key] = c
	addonHTTPState.Unlock()
	c.res, c.err = run()
	addonHTTPState.Lock()
	c.at = time.Now()
	if c.err != nil {
		delete(addonHTTPState.calls, key)
	}
	addonHTTPState.Unlock()
	close(c.done)
	return c.res, c.err
}

type addonHTTPLogEntry struct {
	At     string `json:"at"`
	User   string `json:"user,omitempty"`
	Method string `json:"method"`
	Origin string `json:"origin"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	Bytes  int    `json:"bytes"`
	MS     int    `json:"ms"`
	Code   string `json:"code,omitempty"`
}

// addonHTTPLogHandler lists the instance's last 50 requests: your own, or
// everybody's for an administrator.
func addonHTTPLogHandler(w http.ResponseWriter, userID int, admin bool, in addonInstance) {
	q := `SELECT at, user_id, method, origin, path, status, bytes, ms, code FROM addon_http_log WHERE instance_id=?`
	args := []interface{}{in.ID}
	if !admin {
		q += ` AND user_id=?`
		args = append(args, userID)
	}
	rows, err := db.Query(q+` ORDER BY id DESC LIMIT 50`, args...)
	out := []addonHTTPLogEntry{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var e addonHTTPLogEntry
			var uid int
			if rows.Scan(&e.At, &uid, &e.Method, &e.Origin, &e.Path, &e.Status, &e.Bytes, &e.MS, &e.Code) == nil {
				if admin && uid != userID {
					e.User = usernameOf(uid)
				}
				out = append(out, e)
			}
		}
	}
	jsonOK(w, map[string]interface{}{"requests": out})
}
