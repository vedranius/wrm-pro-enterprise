// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── GIT: OPTIONAL CI INTEGRATION ────────────────────
//
// Everything here is opt-in; WRM works without any CI.
//   - Incoming webhook per Git source (/api/hooks/git/<hook id>): GitLab push / tag events
//     (X-Gitlab-Token), GitHub push / release events (HMAC X-Hub-Signature-256) or a generic
//     POST with the shared secret (X-WRM-Token, or an HMAC with a timestamp). It queues an
//     immediate check of the affected services. Rate-limited per hook, audited, deliveries
//     seen before are refused (replay protection where the provider sends a delivery ID).
//   - Artifact feeds: a bundle fetched from a URL (GitLab job artifacts API, Jenkins artifact
//     URL, any HTTPS URL) with an optional auth header, by hand or on a schedule, imported as
//     an offline bundle.
//   - Deploy method "CI pipeline" per service: an update or upgrade triggers a Jenkins job
//     (buildWithParameters) or a GitLab pipeline (trigger token) with server, install_path
//     and version, then WRM polls its status. Same confirmation, policies and audit as a
//     normal update.

const (
	gitHookMaxBody    = 5 << 20
	gitHookRatePerMin = 30
	gitHookClockSkew  = 5 * time.Minute
	gitHookReplayTTL  = 24 * time.Hour
	gitMaxFeeds       = 20
)

var (
	gitCIPollEvery   = 5 * time.Second
	gitCIPollTimeout = 2 * time.Hour
	gitHookLockWait  = 10 * time.Minute
)

func initGitCISchema() {
	ensureTable("git_feeds", `CREATE TABLE IF NOT EXISTS git_feeds (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		url TEXT NOT NULL DEFAULT '',
		header_name TEXT NOT NULL DEFAULT '',
		header_value TEXT NOT NULL DEFAULT '',
		interval_minutes INTEGER NOT NULL DEFAULT 0,
		source_id INTEGER NOT NULL DEFAULT 0,
		last_hash TEXT NOT NULL DEFAULT '',
		last_at TEXT NOT NULL DEFAULT '',
		last_ok_at TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '')`,
		"id", "user_id", "name", "url", "header_name", "header_value", "interval_minutes", "source_id", "last_hash", "last_at", "last_ok_at", "last_error", "created_at")
	ensureTable("git_pipelines", `CREATE TABLE IF NOT EXISTS git_pipelines (
		user_id INTEGER NOT NULL,
		app TEXT NOT NULL,
		kind TEXT NOT NULL DEFAULT '',
		url TEXT NOT NULL DEFAULT '',
		project TEXT NOT NULL DEFAULT '',
		ref TEXT NOT NULL DEFAULT '',
		username TEXT NOT NULL DEFAULT '',
		token TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (user_id, app))`,
		"user_id", "app", "kind", "url", "project", "ref", "username", "token", "updated_at")
	mustExec(`CREATE INDEX IF NOT EXISTS idx_git_sources_hook ON git_sources(hook_id)`)
}

// ciClient is an HTTP client for a CI or artifact URL: pinned TLS for self-signed servers and
// no redirects to another host (the auth header must not leave it).
func ciClient(raw string) (*http.Client, *url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return nil, nil, fmt.Errorf("enter an address like https://ci.example.com/…")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if u.Scheme == "https" {
		tr.TLSClientConfig = pinnedTLSConfig("ci://", u.Host)
	}
	return &http.Client{Timeout: 5 * time.Minute, Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Host != u.Host {
			return http.ErrUseLastResponse
		}
		return nil
	}}, u, nil
}

// onHost moves a URL that a CI server returned (queue item, build) onto the configured host,
// so credentials are only ever sent there.
func onHost(base *url.URL, raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u = base.ResolveReference(u)
	u.Scheme, u.Host, u.User = base.Scheme, base.Host, nil
	return u.String()
}

func jsonDecodeLimited(r io.Reader, out interface{}) error {
	if err := json.NewDecoder(io.LimitReader(r, 16<<20)).Decode(out); err != nil {
		return fmt.Errorf("unexpected answer: %v", err)
	}
	return nil
}

func ciError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	msg := strings.TrimSpace(string(body))
	if len(msg) > 0 && (msg[0] == '<' || len(msg) > 200) {
		msg = ""
	}
	switch resp.StatusCode {
	case 401, 403:
		msg = strings.TrimSpace(msg + " (check the user and the token)")
	case 404:
		msg = strings.TrimSpace(msg + " (not found)")
	}
	return fmt.Errorf("HTTP %d %s", resp.StatusCode, msg)
}

// ═══ incoming webhooks ═══════════════════════════════

type gitHookEvent struct {
	Provider string   // gitlab | github | generic
	Event    string   // push | tag | release | ping | other
	Delivery string   // delivery ID for replay protection
	Project  string   // path_with_namespace / full_name
	Ref      string   // refs/heads/main, refs/tags/v1, v1
	Services []string // generic: services named by the caller
}

var gitHookState = struct {
	sync.Mutex
	rate      map[string]*hookWindow
	seen      map[string]time.Time
	pending   map[int]map[string]bool
	running   map[int]bool
	lastCheck map[int]time.Time
}{rate: map[string]*hookWindow{}, seen: map[string]time.Time{}, pending: map[int]map[string]bool{}, running: map[int]bool{}, lastCheck: map[int]time.Time{}}

type hookWindow struct {
	start time.Time
	n     int
}

// hookAllowed is a fixed one-minute window per hook (and per client address for unknown
// hooks, so probing cannot block a real one).
func hookAllowed(key string, now time.Time) bool {
	gitHookState.Lock()
	defer gitHookState.Unlock()
	if len(gitHookState.rate) > 10000 {
		for k, w := range gitHookState.rate {
			if now.Sub(w.start) > time.Minute {
				delete(gitHookState.rate, k)
			}
		}
	}
	w := gitHookState.rate[key]
	if w == nil || now.Sub(w.start) > time.Minute {
		gitHookState.rate[key] = &hookWindow{start: now, n: 1}
		return true
	}
	w.n++
	return w.n <= gitHookRatePerMin
}

// hookSeen records a delivery and tells whether it was seen before.
func hookSeen(key string, now time.Time) bool {
	gitHookState.Lock()
	defer gitHookState.Unlock()
	if len(gitHookState.seen) > 20000 {
		for k, at := range gitHookState.seen {
			if now.Sub(at) > gitHookReplayTTL {
				delete(gitHookState.seen, k)
			}
		}
	}
	if at, ok := gitHookState.seen[key]; ok && now.Sub(at) < gitHookReplayTTL {
		return true
	}
	gitHookState.seen[key] = now
	return false
}

func hmacHex(secret string, parts ...[]byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	for _, p := range parts {
		m.Write(p)
	}
	return hex.EncodeToString(m.Sum(nil))
}

func secretEqual(a, b string) bool {
	return a != "" && b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// verifyGitHook authenticates a delivery and returns its provider, event and delivery ID.
func verifyGitHook(h http.Header, body []byte, secret string, now time.Time) (gitHookEvent, error) {
	var ev gitHookEvent
	switch {
	case h.Get("X-Hub-Signature-256") != "":
		ev.Provider = "github"
		sig := strings.TrimPrefix(h.Get("X-Hub-Signature-256"), "sha256=")
		if !secretEqual(sig, hmacHex(secret, body)) {
			return ev, fmt.Errorf("bad signature")
		}
		ev.Delivery = h.Get("X-GitHub-Delivery")
		switch h.Get("X-GitHub-Event") {
		case "push":
			ev.Event = "push"
		case "release":
			ev.Event = "release"
		case "ping":
			ev.Event = "ping"
		default:
			ev.Event = "other"
		}
	case h.Get("X-Gitlab-Token") != "":
		ev.Provider = "gitlab"
		if !secretEqual(h.Get("X-Gitlab-Token"), secret) {
			return ev, fmt.Errorf("bad token")
		}
		ev.Delivery = h.Get("X-Gitlab-Event-UUID")
		if ev.Delivery == "" {
			ev.Delivery = h.Get("Idempotency-Key")
		}
		switch h.Get("X-Gitlab-Event") {
		case "Push Hook":
			ev.Event = "push"
		case "Tag Push Hook":
			ev.Event = "tag"
		case "Release Hook":
			ev.Event = "release"
		default:
			ev.Event = "other"
		}
	case h.Get("X-WRM-Signature") != "":
		ev.Provider, ev.Event = "generic", "push"
		ts, err := strconv.ParseInt(h.Get("X-WRM-Timestamp"), 10, 64)
		if err != nil {
			return ev, fmt.Errorf("missing timestamp")
		}
		if d := now.Sub(time.Unix(ts, 0)); d > gitHookClockSkew || d < -gitHookClockSkew {
			return ev, fmt.Errorf("timestamp too old or in the future")
		}
		sig := strings.TrimPrefix(h.Get("X-WRM-Signature"), "sha256=")
		if !secretEqual(sig, hmacHex(secret, []byte(strconv.FormatInt(ts, 10)), []byte("."), body)) {
			return ev, fmt.Errorf("bad signature")
		}
		ev.Delivery = "sig:" + sig
	case h.Get("X-WRM-Token") != "" || strings.HasPrefix(h.Get("Authorization"), "Bearer "):
		ev.Provider, ev.Event = "generic", "push"
		tok := h.Get("X-WRM-Token")
		if tok == "" {
			tok = strings.TrimPrefix(h.Get("Authorization"), "Bearer ")
		}
		if !secretEqual(strings.TrimSpace(tok), secret) {
			return ev, fmt.Errorf("bad token")
		}
		ev.Delivery = h.Get("X-WRM-Delivery")
	default:
		return ev, fmt.Errorf("no token or signature")
	}
	if len(ev.Delivery) > 200 {
		ev.Delivery = ev.Delivery[:200]
	}
	return ev, nil
}

// parseGitHookPayload reads the project and ref of a delivery.
func parseGitHookPayload(ev *gitHookEvent, body []byte) error {
	var p struct {
		ObjectKind string `json:"object_kind"`
		Ref        string `json:"ref"`
		Action     string `json:"action"`
		Tag        string `json:"tag"`
		Project    json.RawMessage
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Release struct {
			TagName string `json:"tag_name"`
		} `json:"release"`
		Services []string `json:"services"`
	}
	if len(bytes.TrimSpace(body)) == 0 && ev.Provider == "generic" {
		return nil
	}
	if err := json.Unmarshal(body, &p); err != nil {
		if ev.Provider == "generic" {
			return nil // a bare POST from a CI job: all services
		}
		return fmt.Errorf("the payload is not JSON")
	}
	ev.Ref = p.Ref
	switch ev.Provider {
	case "gitlab":
		var pr struct {
			PathWithNamespace string `json:"path_with_namespace"`
		}
		json.Unmarshal(p.Project, &pr)
		ev.Project = pr.PathWithNamespace
		if ev.Event == "release" {
			ev.Ref = p.Tag
			if p.Action != "" && p.Action != "create" && p.Action != "update" {
				ev.Event = "other"
			}
		}
	case "github":
		ev.Project = p.Repository.FullName
		if ev.Event == "release" {
			ev.Ref = p.Release.TagName
			switch p.Action {
			case "published", "released", "created", "prereleased":
			default:
				ev.Event = "other"
			}
		}
	default:
		var s string
		if json.Unmarshal(p.Project, &s) == nil {
			ev.Project = s
		}
		ev.Services = cleanList(p.Services, 100)
	}
	ev.Project = strings.Trim(strings.TrimSpace(ev.Project), "/")
	ev.Ref = truncateStr(ev.Ref, 200)
	return nil
}

// hookServices finds the services a delivery is about.
func hookServices(userID int, ev gitHookEvent) []string {
	out := []string{}
	want := map[string]bool{}
	for _, s := range ev.Services {
		want[s] = true
	}
	for _, a := range effectiveApps(userID, loadGitCatalog(userID)) {
		switch {
		case len(want) > 0:
			if want[a.Name] {
				out = append(out, a.Name)
			}
		case ev.Project != "":
			if strings.EqualFold(a.Project, ev.Project) {
				out = append(out, a.Name)
			}
		case ev.Provider == "generic":
			out = append(out, a.Name)
		}
	}
	sort.Strings(out)
	return out
}

// gitHookHandler receives webhooks: POST /api/hooks/git/<hook id>. It has no session; the
// hook ID finds the source and its secret authenticates the delivery.
func gitHookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	hookID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/hooks/git/"), "/")
	now := time.Now()
	if !hookAllowed("hook:"+hookID, now) || !hookAllowed("ip:"+clientIP(r), now) {
		w.Header().Set("Retry-After", "60")
		jsonError(w, "Too many requests", 429)
		return
	}
	var userID, srcID int
	var name, secretEnc string
	if hookID == "" || len(hookID) > 100 || db.QueryRow(`SELECT id, user_id, name, hook_secret FROM git_sources WHERE hook_id=? AND hook_id<>''`, hookID).Scan(&srcID, &userID, &name, &secretEnc) != nil {
		jsonError(w, "Not found", 404)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, gitHookMaxBody+1))
	if err != nil || len(body) > gitHookMaxBody {
		jsonError(w, "Payload too large", 413)
		return
	}
	username := usernameOf(userID)
	reject := func(code int, reason string, ev gitHookEvent) {
		auditLogRef(r, userID, username, "git.webhook_rejected", name, map[string]interface{}{"source_id": srcID, "provider": ev.Provider, "reason": reason}, auditRef{})
		jsonError(w, "Rejected: "+reason, code)
	}
	ev, err := verifyGitHook(r.Header, body, decryptValue(secretEnc), now)
	if err != nil {
		reject(401, err.Error(), ev)
		return
	}
	if !gitChecksAllowed(userID) {
		reject(403, "checks are not allowed for the owner of this source", ev)
		return
	}
	if ev.Delivery != "" && hookSeen(fmt.Sprintf("%d|%s", srcID, ev.Delivery), now) {
		reject(409, "delivery already received (replay)", ev)
		return
	}
	if err := parseGitHookPayload(&ev, body); err != nil {
		reject(400, err.Error(), ev)
		return
	}
	if ev.Event == "ping" {
		jsonOK(w, map[string]interface{}{"ok": true, "event": "ping"})
		return
	}
	details := map[string]interface{}{"source_id": srcID, "provider": ev.Provider, "event": ev.Event, "project": ev.Project, "ref": ev.Ref}
	if ev.Delivery != "" && !strings.HasPrefix(ev.Delivery, "sig:") {
		details["delivery"] = ev.Delivery
	}
	if ev.Event == "other" {
		details["result"] = "ignored"
		auditLogRef(r, userID, username, "git.webhook", name, details, auditRef{})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]interface{}{"ignored": true, "services": []string{}})
		return
	}
	apps := hookServices(userID, ev)
	details["services"] = apps
	details["result"] = map[bool]string{true: "queued", false: "no matching service"}[len(apps) > 0]
	db.Exec(`UPDATE git_sources SET hook_at=? WHERE id=?`, nowStamp(), srcID)
	auditLogRef(r, userID, username, "git.webhook", name, details, auditRef{})
	if len(apps) > 0 {
		queueHookCheck(userID, apps)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	json.NewEncoder(w).Encode(map[string]interface{}{"queued": len(apps) > 0, "services": apps})
}

// queueHookCheck adds services to the user's pending hook check and starts the worker that
// runs the checks one after another (deliveries arriving meanwhile are merged).
func queueHookCheck(userID int, apps []string) {
	gitHookState.Lock()
	if gitHookState.pending[userID] == nil {
		gitHookState.pending[userID] = map[string]bool{}
	}
	for _, a := range apps {
		gitHookState.pending[userID][a] = true
	}
	if gitHookState.running[userID] {
		gitHookState.Unlock()
		return
	}
	gitHookState.running[userID] = true
	gitHookState.Unlock()
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("git hook check: %v", rec)
				gitHookState.Lock()
				delete(gitHookState.running, userID)
				gitHookState.Unlock()
			}
		}()
		for {
			gitHookState.Lock()
			var list []string
			for a := range gitHookState.pending[userID] {
				list = append(list, a)
			}
			delete(gitHookState.pending, userID)
			if len(list) == 0 {
				delete(gitHookState.running, userID)
				gitHookState.Unlock()
				return
			}
			gitHookState.Unlock()
			sort.Strings(list)
			runHookCheck(userID, list)
		}
	}()
}

// runHookCheck refreshes the targets of the services and compares their installations now.
func runHookCheck(userID int, apps []string) {
	deadline := time.Now().Add(gitHookLockWait)
	for !gitLock(userID) {
		if time.Now().After(deadline) {
			log.Printf("git hook check for user %d: another check kept running", userID)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	defer gitUnlock(userID)
	ts, tchanges := refreshTargets(userID, apps)
	targets := map[string]gitTarget{}
	for _, t := range ts {
		if t.Error == "" {
			targets[t.App] = t
		}
	}
	want := map[string]bool{}
	for _, a := range apps {
		want[a] = true
	}
	var ids []int
	seen := map[int]bool{}
	for _, in := range loadGitInstalls(userID, false, "") {
		if want[in.App] && !seen[in.ConnID] && userOwnsConnection(in.ConnID, userID) {
			seen[in.ConnID] = true
			ids = append(ids, in.ConnID)
		}
	}
	var changes []gitChange
	if len(ids) > 0 {
		res := runGitCheck(userID, ids, false, targets)
		changes = res.Changes
		db.Exec(`INSERT OR IGNORE INTO git_workspace (user_id) VALUES (?)`, userID)
		db.Exec(`UPDATE git_workspace SET last_check_at=? WHERE user_id=?`, nowStamp(), userID)
	}
	notifyGitRound(userID, tchanges, changes)
	gitHookState.Lock()
	gitHookState.lastCheck[userID] = time.Now()
	gitHookState.Unlock()
	hub.sendTo(userID, []byte(`{"type":"git_run_changed"}`))
}

// ═══ artifact feeds ══════════════════════════════════

type gitFeed struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	URL             string `json:"url"`
	HeaderName      string `json:"header_name"`
	HasHeader       bool   `json:"has_header"`
	IntervalMinutes int    `json:"interval_minutes"`
	SourceID        int    `json:"source_id"`
	LastAt          string `json:"last_at,omitempty"`
	LastOKAt        string `json:"last_ok_at,omitempty"`
	LastError       string `json:"last_error,omitempty"`
	headerValue     string
	lastHash        string
	userID          int
}

func scanGitFeeds(where string, args ...interface{}) []gitFeed {
	out := []gitFeed{}
	rows, err := db.Query(`SELECT id, user_id, name, url, header_name, header_value, interval_minutes, source_id, last_hash, last_at, last_ok_at, last_error FROM git_feeds WHERE `+where+` ORDER BY id`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var f gitFeed
		rows.Scan(&f.ID, &f.userID, &f.Name, &f.URL, &f.HeaderName, &f.headerValue, &f.IntervalMinutes, &f.SourceID, &f.lastHash, &f.LastAt, &f.LastOKAt, &f.LastError)
		f.HasHeader = f.headerValue != ""
		out = append(out, f)
	}
	return out
}

func loadGitFeeds(userID int) []gitFeed { return scanGitFeeds("user_id=?", userID) }

var headerNameRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

var gitFeedBusy = struct {
	sync.Mutex
	m map[int]bool
}{m: map[int]bool{}}

// fetchGitFeed downloads the feed's bundle and imports it when it changed. It returns
// "imported" or "unchanged".
func fetchGitFeed(r *http.Request, f gitFeed) (string, *bundleImportResult, error) {
	gitFeedBusy.Lock()
	if gitFeedBusy.m[f.ID] {
		gitFeedBusy.Unlock()
		return "", nil, fmt.Errorf("this feed is being fetched already")
	}
	gitFeedBusy.m[f.ID] = true
	gitFeedBusy.Unlock()
	defer func() {
		gitFeedBusy.Lock()
		delete(gitFeedBusy.m, f.ID)
		gitFeedBusy.Unlock()
	}()
	status, res, err := doFetchGitFeed(f)
	msg := ""
	if err != nil {
		msg = truncateStr(err.Error(), 300)
	}
	db.Exec(`UPDATE git_feeds SET last_at=?, last_error=? WHERE id=?`, nowStamp(), msg, f.ID)
	details := map[string]interface{}{"feed_id": f.ID, "url": f.URL}
	switch {
	case err != nil:
		details["error"] = msg
		auditLogRef(r, f.userID, usernameOf(f.userID), "git.bundle_fetch_failed", f.Name, details, auditRef{})
	case status == "imported":
		db.Exec(`UPDATE git_feeds SET last_ok_at=? WHERE id=?`, nowStamp(), f.ID)
		details["bundle"], details["services"], details["files"], details["source_id"] = res.Source.BundleID, len(res.Source.Apps), res.Files, res.Source.ID
		auditLogRef(r, f.userID, usernameOf(f.userID), "git.bundle_fetched", f.Name, details, auditRef{})
	default:
		db.Exec(`UPDATE git_feeds SET last_ok_at=? WHERE id=?`, nowStamp(), f.ID)
	}
	return status, res, err
}

func doFetchGitFeed(f gitFeed) (string, *bundleImportResult, error) {
	client, _, err := ciClient(f.URL)
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequest("GET", f.URL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "WRM-PRO/"+AppVersion)
	if f.HeaderName != "" && f.headerValue != "" {
		req.Header.Set(f.HeaderName, decryptValue(f.headerValue))
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("cannot fetch the bundle: %s", shortNetError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", nil, fmt.Errorf("cannot fetch the bundle: %v", ciError(resp))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, gitMaxBundle+1))
	if err != nil {
		return "", nil, fmt.Errorf("cannot fetch the bundle: %s", shortNetError(err))
	}
	if len(data) > gitMaxBundle {
		return "", nil, fmt.Errorf("the bundle is larger than %d MB", gitMaxBundle>>20)
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if hash == f.lastHash {
		if _, ok := loadGitSource(f.userID, f.SourceID); ok {
			return "unchanged", nil, nil
		}
	}
	res, err := importBundle(f.userID, bytes.NewReader(data))
	if err != nil {
		return "", nil, fmt.Errorf("the fetched file is not a valid bundle: %v", err)
	}
	// the feed keeps only its newest bundle
	if f.SourceID != 0 && f.SourceID != res.Source.ID {
		if old, ok := loadGitSource(f.userID, f.SourceID); ok && old.Kind == "bundle" {
			deleteGitSource(old.ID)
		}
	}
	db.Exec(`UPDATE git_feeds SET last_hash=?, source_id=? WHERE id=?`, hash, res.Source.ID, f.ID)
	return "imported", res, nil
}

// gitFeedsTick fetches the feeds whose interval has passed (called by the Git scheduler).
func gitFeedsTick(now time.Time) {
	if !gitAllowed() {
		return
	}
	for _, f := range scanGitFeeds("interval_minutes > 0") {
		last, err := time.Parse(time.RFC3339, f.LastAt)
		if err == nil && now.Sub(last) < time.Duration(f.IntervalMinutes)*time.Minute {
			continue
		}
		var exists int
		db.QueryRow(`SELECT COUNT(*) FROM users WHERE id=?`, f.userID).Scan(&exists)
		if exists == 0 {
			continue
		}
		go fetchGitFeed(nil, f)
	}
}

// ═══ CI pipelines (deploy method per service) ═════════

type gitPipeline struct {
	App       string `json:"app"`
	Kind      string `json:"kind"`    // jenkins | gitlab
	URL       string `json:"url"`     // Jenkins: job URL; GitLab: server URL
	Project   string `json:"project"` // GitLab project
	Ref       string `json:"ref"`     // GitLab: branch to run (empty = the target's branch)
	Username  string `json:"username"`
	HasToken  bool   `json:"has_token"`
	UpdatedAt string `json:"updated_at"`
	token     string
}

func scanGitPipelines(userID int, where string, args ...interface{}) []gitPipeline {
	out := []gitPipeline{}
	q := `SELECT app, kind, url, project, ref, username, token, updated_at FROM git_pipelines WHERE user_id=?`
	if where != "" {
		q += " AND " + where
	}
	rows, err := db.Query(q+" ORDER BY app", append([]interface{}{userID}, args...)...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var p gitPipeline
		rows.Scan(&p.App, &p.Kind, &p.URL, &p.Project, &p.Ref, &p.Username, &p.token, &p.UpdatedAt)
		p.HasToken = p.token != ""
		out = append(out, p)
	}
	return out
}

func loadGitPipelines(userID int) []gitPipeline { return scanGitPipelines(userID, "") }

func loadGitPipeline(userID int, app string) (gitPipeline, bool) {
	l := scanGitPipelines(userID, "app=?", app)
	if len(l) == 0 {
		return gitPipeline{}, false
	}
	return l[0], true
}

type ciParams struct {
	Server, InstallPath, Version, Branch string
}

// ciRun is a triggered pipeline: where to poll it and where a person can look at it.
type ciRun struct {
	pollURL string // Jenkins: queue item, then build; GitLab: pipeline API
	WebURL  string
	Status  string
	build   bool // Jenkins: pollURL is the build (no longer the queue item)
}

// triggerPipeline starts the job or pipeline.
func triggerPipeline(userID int, pl gitPipeline, p ciParams) (*ciRun, error) {
	tok := decryptValue(pl.token)
	switch pl.Kind {
	case "jenkins":
		client, base, err := ciClient(pl.URL)
		if err != nil {
			return nil, err
		}
		q := url.Values{"server": {p.Server}, "install_path": {p.InstallPath}, "version": {p.Version}}
		if pl.Username == "" && tok != "" {
			q.Set("token", tok) // the job's "trigger builds remotely" token
		}
		req, _ := http.NewRequest("POST", strings.TrimRight(pl.URL, "/")+"/buildWithParameters?"+q.Encode(), nil)
		jenkinsAuth(req, pl, tok)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("cannot reach Jenkins: %s", shortNetError(err))
		}
		defer resp.Body.Close()
		if resp.StatusCode != 201 && resp.StatusCode != 200 {
			return nil, fmt.Errorf("Jenkins did not start the job: %v", ciError(resp))
		}
		run := &ciRun{WebURL: strings.TrimRight(pl.URL, "/") + "/", Status: "queued"}
		if loc := resp.Header.Get("Location"); loc != "" {
			run.pollURL = strings.TrimRight(onHost(base, loc), "/") + "/api/json"
		}
		return run, nil
	case "gitlab":
		api, err := gitAPIBase("gitlab", pl.URL)
		if err != nil {
			return nil, err
		}
		client, base, err := ciClient(api)
		if err != nil {
			return nil, err
		}
		ref := pl.Ref
		if ref == "" {
			ref = p.Branch
		}
		form := url.Values{"token": {tok}, "ref": {ref}, "variables[server]": {p.Server}, "variables[install_path]": {p.InstallPath}, "variables[version]": {p.Version}}
		resp, err := client.PostForm(api+"/projects/"+url.PathEscape(pl.Project)+"/trigger/pipeline", form)
		if err != nil {
			return nil, fmt.Errorf("cannot reach GitLab: %s", shortNetError(err))
		}
		defer resp.Body.Close()
		if resp.StatusCode != 201 && resp.StatusCode != 200 {
			return nil, fmt.Errorf("GitLab did not start the pipeline: %v", ciError(resp))
		}
		var v struct {
			ID     int    `json:"id"`
			WebURL string `json:"web_url"`
			Status string `json:"status"`
		}
		if err := jsonDecodeLimited(resp.Body, &v); err != nil {
			return nil, err
		}
		return &ciRun{pollURL: onHost(base, api+"/projects/"+url.PathEscape(pl.Project)+"/pipelines/"+strconv.Itoa(v.ID)), WebURL: v.WebURL, Status: v.Status}, nil
	}
	return nil, fmt.Errorf("unknown pipeline kind")
}

func jenkinsAuth(req *http.Request, pl gitPipeline, tok string) {
	if pl.Username != "" {
		req.SetBasicAuth(pl.Username, tok)
	}
	req.Header.Set("User-Agent", "WRM-PRO/"+AppVersion)
}

// readTokenFor finds the read token of the user's GitLab source on the same host (to read the
// status of a pipeline; trigger tokens cannot).
func readTokenFor(userID int, raw string) string {
	host := gitHostOf(raw)
	for _, s := range loadGitSources(userID) {
		if s.Kind == "gitlab" && strings.EqualFold(gitHostOf(s.URL), host) && s.token != "" {
			return decryptValue(s.token)
		}
	}
	return ""
}

// pollPipeline reads the state of a triggered pipeline: done tells it finished, ok whether
// it succeeded.
func pollPipeline(userID int, pl gitPipeline, run *ciRun) (done, ok bool, err error) {
	if run.pollURL == "" {
		return true, true, nil // nothing to follow (Jenkins sent no queue item)
	}
	tok := decryptValue(pl.token)
	client, base, err := ciClient(run.pollURL)
	if err != nil {
		return false, false, err
	}
	req, _ := http.NewRequest("GET", run.pollURL, nil)
	if pl.Kind == "jenkins" {
		jenkinsAuth(req, pl, tok)
	} else if rt := readTokenFor(userID, pl.URL); rt != "" {
		req.Header.Set("PRIVATE-TOKEN", rt)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, false, fmt.Errorf("cannot read the status: %s", shortNetError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, false, fmt.Errorf("cannot read the status: %v", ciError(resp))
	}
	if pl.Kind == "gitlab" {
		var v struct {
			Status string `json:"status"`
			WebURL string `json:"web_url"`
		}
		if err := jsonDecodeLimited(resp.Body, &v); err != nil {
			return false, false, err
		}
		run.Status = v.Status
		if v.WebURL != "" {
			run.WebURL = v.WebURL
		}
		switch v.Status {
		case "success":
			return true, true, nil
		case "failed", "canceled", "skipped", "manual", "scheduled":
			return true, false, nil
		}
		return false, false, nil
	}
	if !run.build {
		var q struct {
			Cancelled  bool `json:"cancelled"`
			Executable *struct {
				URL string `json:"url"`
			} `json:"executable"`
		}
		if err := jsonDecodeLimited(resp.Body, &q); err != nil {
			return false, false, err
		}
		if q.Cancelled {
			run.Status = "cancelled"
			return true, false, nil
		}
		if q.Executable != nil && q.Executable.URL != "" {
			u := strings.TrimRight(onHost(base, q.Executable.URL), "/") + "/"
			run.WebURL, run.pollURL, run.build, run.Status = u, u+"api/json", true, "running"
		}
		return false, false, nil
	}
	var b struct {
		Building bool    `json:"building"`
		Result   *string `json:"result"`
	}
	if err := jsonDecodeLimited(resp.Body, &b); err != nil {
		return false, false, err
	}
	if b.Building || b.Result == nil {
		run.Status = "running"
		return false, false, nil
	}
	run.Status = strings.ToLower(*b.Result)
	return true, *b.Result == "SUCCESS", nil
}

// pipelineItem is an update / upgrade of an installation whose service is deployed by CI.
func (run *gitRun) pipelineItem(it *gitRunItem) error {
	c := itemCtx{run, it}
	t, ok := run.P.Targets[it.App]
	if !ok {
		return fmt.Errorf("no target for %s", it.App)
	}
	pl, ok := loadGitPipeline(run.UserID, it.App)
	if !ok {
		return fmt.Errorf("the CI pipeline of %s is not configured any more", it.App)
	}
	list := loadGitInstalls(run.UserID, false, "i.id=?", it.InstallID)
	if len(list) == 0 {
		return fmt.Errorf("the installation is not known any more")
	}
	in := list[0]
	conn, err := loadConnection(in.ConnID)
	if err != nil || conn.UserID != run.UserID {
		return fmt.Errorf("connection not found")
	}
	from := in.VersionMD.Version
	if from == "" {
		from = "-"
	}
	run.set(func() { it.FromVersion, it.ToVersion, it.CIKind = from, t.Version, pl.Kind })
	c.step("ci")
	cr, err := triggerPipeline(run.UserID, pl, ciParams{Server: hostOnly(conn.Host), InstallPath: in.Path, Version: t.Version, Branch: t.Branch})
	if err != nil {
		c.log("ci", "fail", err.Error())
		return err
	}
	run.set(func() { it.CIURL, it.CIStatus = cr.WebURL, cr.Status })
	c.log("ci", "ok", fmt.Sprintf("%s triggered: server=%s install_path=%s version=%s", pl.Kind, hostOnly(conn.Host), in.Path, t.Version))
	deadline := time.Now().Add(gitCIPollTimeout)
	last := cr.Status
	for {
		run.mu.Lock()
		stop := run.cancel
		run.mu.Unlock()
		if stop {
			c.log("ci", "info", "stopped waiting for the pipeline (it keeps running in CI)")
			return fmt.Errorf("stopped waiting for the pipeline")
		}
		done, okRun, err := pollPipeline(run.UserID, pl, cr)
		run.set(func() { it.CIURL, it.CIStatus = cr.WebURL, cr.Status })
		if cr.Status != last && cr.Status != "" {
			last = cr.Status
			c.log("ci", "info", cr.Status)
		}
		if err != nil {
			if pl.Kind == "gitlab" {
				// a trigger token cannot read pipelines: without a read token the status stays unknown
				c.log("ci", "info", "the pipeline was triggered, its status cannot be read ("+err.Error()+"): follow it in GitLab")
				run.set(func() { it.CIStatus = "triggered" })
				break
			}
			c.log("ci", "fail", err.Error())
			return err
		}
		if done {
			if !okRun {
				c.log("ci", "fail", "pipeline finished: "+cr.Status)
				return fmt.Errorf("the pipeline finished with %s", cr.Status)
			}
			c.log("ci", "ok", "pipeline finished: "+cr.Status)
			break
		}
		if time.Now().After(deadline) {
			c.log("ci", "fail", "no result within the time limit")
			return fmt.Errorf("the pipeline did not finish within %s", gitCIPollTimeout)
		}
		time.Sleep(gitCIPollEvery)
	}
	// compare the installation again (best effort: CI may deploy somewhere WRM cannot log in)
	if isSSHProtocol(conn) {
		if cl, err := dialSSH(conn, nil); err == nil {
			refreshInstallState(run.UserID, in, cl)
			cl.Close()
		} else {
			c.log("scan", "info", "not compared again: "+err.Error())
		}
	}
	run.set(func() { it.State, it.Step = "ok", "" })
	return nil
}

// ═══ API ═════════════════════════════════════════════

// apiGitCI handles the CI routes of /api/git; it returns false for other routes.
//
//	POST   /api/git/sources/{id}/hook        → new hook ID and secret (shown once); DELETE turns it off
//	POST   /api/git/feeds                    {name, url, header_name, header_value, interval_minutes}
//	PUT    /api/git/feeds/{id}; DELETE /api/git/feeds/{id}
//	POST   /api/git/feeds/{id}/fetch         → fetch and import now
//	PUT    /api/git/pipelines/{app}          {kind, url, project, ref, username, token}; DELETE → deploy files again
func apiGitCI(w http.ResponseWriter, r *http.Request, userID int, rest string, parts []string) bool {
	switch {
	case len(parts) == 3 && parts[0] == "sources" && parts[2] == "hook" && (r.Method == http.MethodPost || r.Method == http.MethodDelete):
		if !gitChecksAllowed(userID) {
			jsonError(w, "Checks are not allowed for your account", 403)
			return true
		}
		id, _ := strconv.Atoi(parts[1])
		src, found := loadGitSource(userID, id)
		if !found || !src.Owned || src.Kind == "bundle" || src.Relay != nil {
			jsonError(w, "Source not found or client-cache source does not accept webhooks", 404)
			return true
		}
		if r.Method == http.MethodDelete {
			db.Exec(`UPDATE git_sources SET hook_id='', hook_secret='', hook_at='' WHERE id=?`, id)
			auditLog(r, userID, "git.webhook_disabled", src.Name, map[string]interface{}{"source_id": id})
			jsonOK(w, gitState(userID))
			return true
		}
		hookID, secret := randomToken(18), randomToken(24)
		db.Exec(`UPDATE git_sources SET hook_id=?, hook_secret=?, hook_at='' WHERE id=?`, hookID, encryptValue(secret), id)
		auditLog(r, userID, "git.webhook_enabled", src.Name, map[string]interface{}{"source_id": id, "renewed": src.HookID != ""})
		jsonOK(w, map[string]interface{}{"hook_id": hookID, "path": "/api/hooks/git/" + hookID, "secret": secret, "state": gitState(userID)})

	case parts[0] == "feeds" && (len(parts) == 1 && r.Method == http.MethodPost || len(parts) == 2 && (r.Method == http.MethodPut || r.Method == http.MethodDelete)):
		var cur gitFeed
		if len(parts) == 2 {
			id, _ := strconv.Atoi(parts[1])
			l := scanGitFeeds("id=? AND user_id=?", id, userID)
			if len(l) == 0 {
				jsonError(w, "Feed not found", 404)
				return true
			}
			cur = l[0]
		}
		if r.Method == http.MethodDelete {
			db.Exec(`DELETE FROM git_feeds WHERE id=?`, cur.ID)
			auditLog(r, userID, "git.feed_deleted", cur.Name, map[string]interface{}{"feed_id": cur.ID})
			jsonOK(w, gitState(userID))
			return true
		}
		var in struct {
			Name            string  `json:"name"`
			URL             string  `json:"url"`
			HeaderName      string  `json:"header_name"`
			HeaderValue     *string `json:"header_value"`
			IntervalMinutes int     `json:"interval_minutes"`
		}
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		in.URL, in.Name, in.HeaderName = strings.TrimSpace(in.URL), strings.TrimSpace(in.Name), strings.TrimSpace(in.HeaderName)
		if _, _, err := ciClient(in.URL); err != nil {
			jsonError(w, err.Error(), 400)
			return true
		}
		if in.HeaderName != "" && !headerNameRe.MatchString(in.HeaderName) {
			jsonError(w, "Invalid header name", 400)
			return true
		}
		if in.IntervalMinutes != 0 && (in.IntervalMinutes < 5 || in.IntervalMinutes > 10080) {
			jsonError(w, "The interval must be 5 minutes to 7 days (0 = only by hand)", 400)
			return true
		}
		if in.Name == "" {
			in.Name = gitHostOf(in.URL)
		}
		hv := cur.headerValue
		if in.HeaderValue != nil {
			hv = ""
			if v := strings.TrimSpace(*in.HeaderValue); v != "" {
				// "Basic user:token" is encoded for Jenkins
				if rest := strings.TrimPrefix(v, "Basic "); rest != v && strings.Contains(rest, ":") {
					v = "Basic " + base64.StdEncoding.EncodeToString([]byte(rest))
				}
				hv = encryptValue(v)
			}
		}
		if cur.ID == 0 {
			var n int
			db.QueryRow(`SELECT COUNT(*) FROM git_feeds WHERE user_id=?`, userID).Scan(&n)
			if n >= gitMaxFeeds {
				jsonError(w, fmt.Sprintf("at most %d feeds", gitMaxFeeds), 400)
				return true
			}
			res, err := db.Exec(`INSERT INTO git_feeds (user_id, name, url, header_name, header_value, interval_minutes, created_at) VALUES (?,?,?,?,?,?,?)`,
				userID, truncateStr(in.Name, 100), in.URL, in.HeaderName, hv, in.IntervalMinutes, nowStamp())
			if err != nil {
				jsonError(w, "Cannot save", 500)
				return true
			}
			id, _ := res.LastInsertId()
			auditLog(r, userID, "git.feed_added", in.Name, map[string]interface{}{"feed_id": id, "url": in.URL, "interval_minutes": in.IntervalMinutes})
		} else {
			db.Exec(`UPDATE git_feeds SET name=?, url=?, header_name=?, header_value=?, interval_minutes=? WHERE id=?`,
				truncateStr(in.Name, 100), in.URL, in.HeaderName, hv, in.IntervalMinutes, cur.ID)
			auditLog(r, userID, "git.feed_changed", in.Name, map[string]interface{}{"feed_id": cur.ID, "url": in.URL, "interval_minutes": in.IntervalMinutes,
				"new_header": in.HeaderValue != nil})
		}
		jsonOK(w, gitState(userID))

	case len(parts) == 3 && parts[0] == "feeds" && parts[2] == "fetch" && r.Method == http.MethodPost:
		id, _ := strconv.Atoi(parts[1])
		l := scanGitFeeds("id=? AND user_id=?", id, userID)
		if len(l) == 0 {
			jsonError(w, "Feed not found", 404)
			return true
		}
		status, res, err := fetchGitFeed(r, l[0])
		if err != nil {
			jsonError(w, err.Error(), 502)
			return true
		}
		jsonOK(w, map[string]interface{}{"status": status, "result": res, "state": gitState(userID)})

	case len(parts) == 2 && parts[0] == "pipelines" && (r.Method == http.MethodPut || r.Method == http.MethodDelete):
		if !gitActionAllowed(userID, "git_update") {
			jsonError(w, "Updates are not allowed for your account", 403)
			return true
		}
		app := parts[1]
		cur, have := loadGitPipeline(userID, app)
		if r.Method == http.MethodDelete {
			db.Exec(`DELETE FROM git_pipelines WHERE user_id=? AND app=?`, userID, app)
			auditLog(r, userID, "git.pipeline_deleted", app, map[string]interface{}{"kind": cur.Kind})
			jsonOK(w, gitState(userID))
			return true
		}
		var a *gitCatalogApp
		for _, x := range effectiveApps(userID, loadGitCatalog(userID)) {
			if x.Name == app {
				x := x
				a = &x
			}
		}
		if a == nil {
			jsonError(w, "Service not found", 404)
			return true
		}
		var in struct {
			Kind     string  `json:"kind"`
			URL      string  `json:"url"`
			Project  string  `json:"project"`
			Ref      string  `json:"ref"`
			Username string  `json:"username"`
			Token    *string `json:"token"`
		}
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		in.URL, in.Project, in.Ref, in.Username = strings.TrimRight(strings.TrimSpace(in.URL), "/"), strings.Trim(strings.TrimSpace(in.Project), "/"), strings.TrimSpace(in.Ref), strings.TrimSpace(in.Username)
		tok := cur.token
		if in.Token != nil {
			tok = ""
			if v := strings.TrimSpace(*in.Token); v != "" {
				tok = encryptValue(v)
			}
		}
		switch in.Kind {
		case "jenkins":
			in.Project, in.Ref = "", ""
		case "gitlab":
			in.Username = ""
			if in.URL == "" {
				_, st := loadGitWorkspace(userID)
				if api, _ := pickSources(userID, st); api != nil && api.Kind == "gitlab" {
					in.URL = api.URL
				}
			}
			if in.Project == "" {
				in.Project = a.Project
			}
			if tok == "" {
				jsonError(w, "Enter the pipeline trigger token", 400)
				return true
			}
			if strings.ContainsAny(in.Ref, " \n\r\t") || len(in.Ref) > 200 {
				jsonError(w, "Invalid branch", 400)
				return true
			}
		default:
			jsonError(w, "Kind must be jenkins or gitlab", 400)
			return true
		}
		if _, _, err := ciClient(in.URL); err != nil {
			jsonError(w, err.Error(), 400)
			return true
		}
		if len(in.Username) > 100 || len(in.Project) > 300 {
			jsonError(w, "Invalid request", 400)
			return true
		}
		db.Exec(`INSERT INTO git_pipelines (user_id, app, kind, url, project, ref, username, token, updated_at) VALUES (?,?,?,?,?,?,?,?,?)
			ON CONFLICT(user_id, app) DO UPDATE SET kind=excluded.kind, url=excluded.url, project=excluded.project, ref=excluded.ref,
			username=excluded.username, token=excluded.token, updated_at=excluded.updated_at`,
			userID, app, in.Kind, in.URL, in.Project, in.Ref, in.Username, tok, nowStamp())
		auditLog(r, userID, "git.pipeline_saved", app, map[string]interface{}{"kind": in.Kind, "url": in.URL, "project": in.Project, "new_secret": in.Token != nil, "changed": have})
		jsonOK(w, gitState(userID))

	default:
		return false
	}
	return true
}
