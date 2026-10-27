// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const relayMaxBody = 8 << 20
const relayMaxSnapshot = 256 << 20
const relayMaxRequests = 10000

type gitRelayMeta struct {
	Enabled     bool   `json:"enabled"`
	OwnerID     int    `json:"owner_id"`
	Generation  int64  `json:"generation"`
	Published   int64  `json:"published"`
	Contributor string `json:"contributor"`
	At          string `json:"at"`
	Error       string `json:"error"`
	Live        bool   `json:"live"`
	Stale       bool   `json:"stale"`
	Complete    bool   `json:"complete"`
}
type gitRelayResponse struct {
	Status int    `json:"status"`
	Body   []byte `json:"body"`
	Next   string `json:"next"`
	Type   string `json:"type"`
}
type gitRelayRequest struct {
	ID        int    `json:"id"`
	URL       string `json:"url"`
	response  chan gitRelayResponse
	completed bool
}
type gitRelayJob struct {
	mu             sync.Mutex
	ID             string
	KeyHash        string
	Source         gitSource
	UserID         int
	Generation     int64
	ctx            context.Context
	cancel         context.CancelFunc
	Created        time.Time
	Pending        map[int]*gitRelayRequest
	Stage          map[string]gitRelayResponse
	next, bytes    int
	PublishedCount int
	LastSeen       time.Time
	State, Error   string
	Projects       []string
}

var gitRelayJobs = struct {
	sync.Mutex
	m map[string]*gitRelayJob
}{m: map[string]*gitRelayJob{}}

func initGitRelay() {
	ensureTable("git_relay_meta", `CREATE TABLE IF NOT EXISTS git_relay_meta(source_id INTEGER PRIMARY KEY,enabled INTEGER NOT NULL DEFAULT 0,generation INTEGER NOT NULL DEFAULT 0,published INTEGER NOT NULL DEFAULT 0,contributor TEXT NOT NULL DEFAULT '',at TEXT NOT NULL DEFAULT '',error TEXT NOT NULL DEFAULT '')`, "source_id", "enabled", "generation", "published", "contributor", "at", "error")
	ensureTable("git_relay_members", `CREATE TABLE IF NOT EXISTS git_relay_members(source_id INTEGER NOT NULL,user_id INTEGER NOT NULL,PRIMARY KEY(source_id,user_id))`, "source_id", "user_id")
	ensureTable("git_relay_cache", `CREATE TABLE IF NOT EXISTS git_relay_cache(source_id INTEGER NOT NULL,generation INTEGER NOT NULL,request TEXT NOT NULL,response TEXT NOT NULL,PRIMARY KEY(source_id,generation,request))`, "source_id", "generation", "request", "response")
}
func loadGitRelayMeta(id int) gitRelayMeta {
	m := gitRelayMeta{}
	db.QueryRow(`SELECT COALESCE(s.user_id,0),m.enabled,m.generation,m.published,m.contributor,m.at,m.error FROM git_sources s JOIN git_relay_meta m ON m.source_id=s.id WHERE s.id=?`, id).Scan(&m.OwnerID, &m.Enabled, &m.Generation, &m.Published, &m.Contributor, &m.At, &m.Error)
	m.Complete = m.Published > 0
	at, err := time.Parse(time.RFC3339, m.At)
	m.Stale = m.Complete && (err != nil || time.Since(at) > 24*time.Hour)
	return m
}
func relaySourceAllowed(uid, id int) bool {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM git_sources s WHERE id=? AND EXISTS(SELECT 1 FROM users u WHERE u.id=? AND u.disabled=0) AND (user_id=? OR EXISTS(SELECT 1 FROM git_relay_members m JOIN git_relay_meta r ON r.source_id=m.source_id WHERE m.source_id=s.id AND m.user_id=? AND r.enabled=1))`, id, uid, uid, uid).Scan(&n)
	return n > 0 && gitAllowed()
}
func relayRequestKey(api string, u *url.URL) (string, error) {
	base, err := url.Parse(api)
	if err != nil || u.Scheme != base.Scheme || u.Host != base.Host || !strings.HasPrefix(u.EscapedPath(), strings.TrimSuffix(base.EscapedPath(), "/")+"/") {
		return "", fmt.Errorf("relay endpoint is outside the configured GitLab API")
	}
	rel := strings.TrimPrefix(u.EscapedPath(), strings.TrimSuffix(base.EscapedPath(), "/"))
	if u.RawQuery != "" {
		rel += "?" + u.Query().Encode()
	}
	return rel, nil
}
func relayCached(source int, generation int64, key string) (gitRelayResponse, bool) {
	var raw string
	if db.QueryRow(`SELECT response FROM git_relay_cache WHERE source_id=? AND generation=? AND request=?`, source, generation, key).Scan(&raw) != nil {
		return gitRelayResponse{}, false
	}
	var res gitRelayResponse
	err := json.Unmarshal([]byte(decryptValue(raw)), &res)
	return res, err == nil
}
func relayHTTPResponse(req *http.Request, res gitRelayResponse) *http.Response {
	h := http.Header{}
	h.Set("Content-Type", res.Type)
	h.Set("X-Next-Page", res.Next)
	return &http.Response{StatusCode: res.Status, Header: h, Body: io.NopCloser(bytes.NewReader(res.Body)), ContentLength: int64(len(res.Body)), Request: req}
}

type gitRelayTransport struct {
	Source     int
	UserID     int
	Generation int64
	API        string
	Job        *gitRelayJob
}

var relaySHA = regexp.MustCompile(`^[a-fA-F0-9]{40}(?:[a-fA-F0-9]{24})?$`)

func relayImmutableKey(key string) bool {
	u, _ := url.Parse(key)
	for _, part := range strings.Split(u.Path, "/") {
		if relaySHA.MatchString(part) {
			return true
		}
	}
	return relaySHA.MatchString(u.Query().Get("ref"))
}
func (t *gitRelayTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return nil, fmt.Errorf("client GitLab sources are read-only; live writes require a direct server source with separate approval")
	}
	key, err := relayRequestKey(t.API, req.URL)
	if err != nil {
		return nil, err
	}
	if t.Job == nil {
		if !relaySourceAllowed(t.UserID, t.Source) {
			return nil, fmt.Errorf("shared source access was removed")
		}
		meta := loadGitRelayMeta(t.Source)
		if !meta.Enabled || meta.Published == 0 {
			return nil, fmt.Errorf("client GitLab cache is not available; connect an authorized client and synchronize")
		}
		generation := t.Generation
		if generation == 0 {
			generation = meta.Published
		}
		if res, ok := relayCached(t.Source, generation, key); ok {
			return relayHTTPResponse(req, res), nil
		}
		if live := liveGitRelay(t.Source, generation); live != nil {
			transport := *t
			transport.Job = live
			return transport.RoundTrip(req)
		}
		return nil, fmt.Errorf("repository data is not in the reviewed client snapshot; reconnect a client and refresh (endpoint %s)", truncateStr(key, 160))
	}
	j := t.Job
	if !relayJobPathAllowed(j, key) {
		return nil, fmt.Errorf("client relay is limited to repositories in the source owner catalog")
	}
	if !relayJobAccess(j) {
		return nil, fmt.Errorf("relay source access was removed")
	}
	j.mu.Lock()
	if cached, ok := j.Stage[key]; ok {
		j.mu.Unlock()
		return relayHTTPResponse(req, cached), nil
	}
	j.mu.Unlock()
	j.mu.Lock()
	live := j.State == "cached"
	j.mu.Unlock()
	if !live && relayImmutableKey(key) {
		meta := loadGitRelayMeta(t.Source)
		if res, ok := relayCached(t.Source, meta.Published, key); ok {
			j.mu.Lock()
			j.Stage[key] = res
			j.bytes += len(res.Body)
			j.mu.Unlock()
			return relayHTTPResponse(req, res), nil
		}
	}
	j.mu.Lock()
	if j.next >= relayMaxRequests || (!live && j.bytes >= relayMaxSnapshot) {
		j.mu.Unlock()
		return nil, fmt.Errorf("client snapshot limit reached; previous complete snapshot remains active")
	}
	j.next++
	pending := &gitRelayRequest{ID: j.next, URL: req.URL.String(), response: make(chan gitRelayResponse, 1)}
	j.Pending[pending.ID] = pending
	j.mu.Unlock()
	defer func() { j.mu.Lock(); delete(j.Pending, pending.ID); j.mu.Unlock() }()
	viewer := t.UserID
	if viewer == 0 {
		viewer = j.UserID
	}
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	policy := time.NewTicker(time.Second)
	defer policy.Stop()
	for {
		select {
		case res := <-pending.response:
			if !relaySourceAllowed(viewer, j.Source.ID) || !relayJobAccess(j) {
				return nil, fmt.Errorf("shared Git source access changed while waiting for the client")
			}
			if res.Status == 200 && strings.Contains(strings.SplitN(key, "?", 2)[0], "/repository/blobs/") && strings.HasSuffix(strings.SplitN(key, "?", 2)[0], "/raw") {
				parts := strings.Split(strings.SplitN(key, "?", 2)[0], "/")
				if err := verifyRelayBlob(parts[len(parts)-2], res.Body); err != nil {
					return nil, err
				}
			}
			if res.Status == 200 && !live {
				j.mu.Lock()
				j.Stage[key] = res
				j.bytes += len(res.Body)
				oversize := j.bytes > relayMaxSnapshot
				j.mu.Unlock()
				if oversize {
					return nil, fmt.Errorf("snapshot exceeds 256 MiB")
				}
			}
			return relayHTTPResponse(req, res), nil
		case <-policy.C:
			if !relaySourceAllowed(viewer, j.Source.ID) || !relayJobAccess(j) {
				return nil, fmt.Errorf("shared Git source access was removed")
			}
			continue
		case <-deadline.C:
			return nil, fmt.Errorf("client relay did not answer; reconnect the client or use the previous cached snapshot")
		case <-j.ctx.Done():
			return nil, j.ctx.Err()
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
}
func newGitRelayJob(src gitSource, uid int) (*gitRelayJob, string, error) {
	meta := loadGitRelayMeta(src.ID)
	if !meta.Enabled {
		return nil, "", fmt.Errorf("enable client GitLab access first")
	}
	cat, _ := loadGitWorkspace(meta.OwnerID)
	projects := map[string]bool{}
	for _, a := range cat.Apps {
		projects[a.Project] = true
	}
	if len(projects) == 0 || len(projects) > 100 {
		return nil, "", fmt.Errorf("configure 1–100 catalog repositories before client synchronization")
	}
	gitRelayJobs.Lock()
	defer gitRelayJobs.Unlock()
	active := 0
	for id, j := range gitRelayJobs.m {
		j.mu.Lock()
		state := j.State
		old := time.Since(j.Created) > time.Hour
		j.mu.Unlock()
		if old {
			j.cancel()
			delete(gitRelayJobs.m, id)
			continue
		}
		if state == "refreshing" {
			active++
			if j.UserID == uid && j.Source.ID == src.ID {
				return nil, "", fmt.Errorf("this source already has a refresh; cancel it first")
			}
		}
	}
	if active >= 3 {
		return nil, "", fmt.Errorf("at most three client refreshes at once")
	}
	var generation int64
	if err := db.QueryRow(`UPDATE git_relay_meta SET generation=generation+1,error='' WHERE source_id=? RETURNING generation`, src.ID).Scan(&generation); err != nil {
		return nil, "", err
	}
	key := randomToken(32)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	j := &gitRelayJob{ID: randomToken(20), KeyHash: sha256Hex(key), Source: src, UserID: uid, Generation: generation, ctx: ctx, cancel: cancel, Created: time.Now(), State: "refreshing", Pending: map[int]*gitRelayRequest{}, Stage: map[string]gitRelayResponse{}}
	for p := range projects {
		j.Projects = append(j.Projects, p)
	}
	sort.Strings(j.Projects)
	gitRelayJobs.m[j.ID] = j
	go j.run()
	return j, key, nil
}
func (j *gitRelayJob) run() {
	err := j.collect()
	if err == nil {
		err = j.publish()
	}
	if err == nil {
		_, st := loadGitWorkspace(j.UserID)
		src, _ := pickSources(j.UserID, st)
		if src != nil && src.ID == j.Source.ID {
			refreshTargets(j.UserID, nil)
		}
	}
	j.mu.Lock()
	if err != nil {
		j.State = "failed"
		j.Error = truncateStr(err.Error(), 500)
	} else {
		j.State = "cached"
	}
	j.PublishedCount = len(j.Stage)
	j.Stage = map[string]gitRelayResponse{}
	requests, transferred, jobError := j.next, j.bytes, j.Error
	j.mu.Unlock()
	if err != nil {
		j.cancel()
	}

	if err != nil {
		db.Exec(`UPDATE git_relay_meta SET error=? WHERE source_id=? AND generation=?`, truncateStr(err.Error(), 500), j.Source.ID, j.Generation)
	}
	auditLog(nil, j.UserID, "git.client_refreshed", j.Source.Name, map[string]interface{}{"source_id": j.Source.ID, "generation": j.Generation, "complete": err == nil, "requests": requests, "bytes": transferred, "error": jobError})
}
func (j *gitRelayJob) collect() error {
	api, err := gitAPIBase("gitlab", j.Source.URL)
	if err != nil {
		return err
	}
	p := &gitProvider{kind: "gitlab", api: api, client: &http.Client{Transport: &gitRelayTransport{Source: j.Source.ID, API: api, Job: j}}}
	for _, project := range j.Projects {
		if _, err := p.defaultBranch(project); err != nil {
			return err
		}
		branches, err := p.branches(project)
		if err != nil {
			return err
		}
		tags, err := p.tags(project)
		if err != nil {
			return err
		}
		if len(branches)+len(tags) > 200 {
			return fmt.Errorf("repository has more than 200 refs; snapshot incomplete")
		}
		commits := map[string]bool{}
		for _, branch := range branches {
			history, err := p.branchCommits(project, branch, gitCommitPages)
			if err != nil {
				return err
			}
			for i, c := range history {
				if i >= gitMaxDepth {
					break
				}
				if !relaySHA.MatchString(c.SHA) {
					return fmt.Errorf("invalid commit identity")
				}
				commits[c.SHA] = true
			}
		}
		for _, tag := range tags {
			if !relaySHA.MatchString(tag.Commit) {
				return fmt.Errorf("invalid tag commit identity")
			}
			commits[tag.Commit] = true
		}
		for commit := range commits {
			var info json.RawMessage
			if _, err := p.getJSON(p.gl(project)+"/repository/commits/"+commit, &info); err != nil {
				return err
			}
			tree, err := p.rawTree(project, commit)
			if err != nil {
				return err
			}
			for file, blob := range tree {
				if cleanRel(file) != file || strings.HasPrefix(file, "/") || !relaySHA.MatchString(blob) {
					return fmt.Errorf("invalid repository tree/blob identity")
				}
				data, err := p.blob(project, blob)
				if err != nil {
					return err
				}
				if len(data) > relayMaxBody {
					return fmt.Errorf("repository file exceeds 8 MiB; snapshot incomplete")
				}
				if err := verifyRelayBlob(blob, data); err != nil {
					return err
				}
			}
		}
	}
	j.mu.Lock()
	j.Stage["/_wrm/projects"] = gitRelayResponse{Status: 200, Body: jsonMarshal(j.Projects), Type: "application/json"}
	j.mu.Unlock()
	return nil
}
func (j *gitRelayJob) publish() error {
	if !relayJobAccess(j) {
		return fmt.Errorf("source access removed before publish")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.ctx.Err(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, res := range j.Stage {
		if _, err = tx.Exec(`INSERT INTO git_relay_cache(source_id,generation,request,response) VALUES(?,?,?,?)`, j.Source.ID, j.Generation, key, encryptValue(string(jsonMarshal(res)))); err != nil {
			return err
		}
	}
	result, err := tx.Exec(`UPDATE git_relay_meta SET published=?,at=?,contributor=?,error=CASE WHEN generation=? THEN '' ELSE error END WHERE source_id=? AND enabled=1 AND published<? AND EXISTS(SELECT 1 FROM git_sources s JOIN users u ON u.id=? AND u.disabled=0 WHERE s.id=git_relay_meta.source_id AND s.url=? AND s.kind='gitlab' AND (s.user_id=u.id OR EXISTS(SELECT 1 FROM git_relay_members m WHERE m.source_id=s.id AND m.user_id=u.id)))`, j.Generation, nowRFC(), usernameOf(j.UserID), j.Generation, j.Source.ID, j.Generation, j.UserID, j.Source.URL)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return fmt.Errorf("newer successful snapshot already published or source disabled")
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// Immutable trees/blobs used by reviewed deployments retain their existing cache.
	// Only older generations: a newer snapshot published meanwhile by another member stays.
	db.Exec(`DELETE FROM git_relay_cache WHERE source_id=? AND generation<? AND generation<> (SELECT MAX(generation) FROM git_relay_cache WHERE source_id=? AND generation<?)`, j.Source.ID, j.Generation, j.Source.ID, j.Generation)
	return nil
}
func (j *gitRelayJob) view() map[string]interface{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	requests := []map[string]interface{}{}
	for _, q := range j.Pending {
		if !q.completed {
			requests = append(requests, map[string]interface{}{"id": q.ID, "url": q.URL})
		}
	}
	sort.Slice(requests, func(i, k int) bool { return requests[i]["id"].(int) < requests[k]["id"].(int) })
	return map[string]interface{}{"id": j.ID, "source_id": j.Source.ID, "generation": j.Generation, "state": j.State, "error": j.Error, "requests": requests, "completed": max(len(j.Stage), j.PublishedCount), "bytes": j.bytes, "projects": j.Projects, "expires_at": j.Created.Add(30 * time.Minute).UTC().Format(time.RFC3339), "limits": map[string]int{"snapshot_mb": 256, "response_mb": 8, "refs_per_repository": 200, "history_commits_per_branch": 500, "history_tree_depth": 50}}
}
func gitRelayWorkerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	id := r.URL.Query().Get("job")
	gitRelayJobs.Lock()
	j := gitRelayJobs.m[id]
	gitRelayJobs.Unlock()
	key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if j == nil || subtle.ConstantTimeCompare([]byte(sha256Hex(key)), []byte(j.KeyHash)) != 1 || !relayJobAccess(j) {
		jsonError(w, "Relay job is not authorized", 403)
		return
	}
	if r.Method == http.MethodGet {
		j.mu.Lock()
		j.LastSeen = time.Now()
		j.mu.Unlock()
		jsonOK(w, j.view())
		return
	}
	if r.Method == http.MethodDelete {
		j.cancel()
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var in struct {
		RequestID int `json:"request_id"`
		gitRelayResponse
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Body) > relayMaxBody || in.Status < 100 || in.Status > 599 || len(in.Type) > 200 || len(in.Next) > 10 {
		jsonError(w, "Invalid relay response", 400)
		return
	}
	j.mu.Lock()
	q := j.Pending[in.RequestID]
	if !oneOf(j.State, "refreshing", "cached") || j.ctx.Err() != nil || q == nil || q.completed {
		j.mu.Unlock()
		jsonError(w, "Relay request expired or already answered", 409)
		return
	}
	q.completed = true
	j.mu.Unlock()
	q.response <- in.gitRelayResponse
	jsonOK(w, map[string]bool{"ok": true})
}
func apiGitRelay(w http.ResponseWriter, r *http.Request, uid int, rest string) bool {
	if !strings.HasPrefix(rest, "client/") {
		return false
	}
	parts := strings.Split(rest, "/")
	id, err := strconv.Atoi(parts[1])
	if err != nil || !relaySourceAllowed(uid, id) {
		jsonError(w, "Git source not found", 404)
		return true
	}
	src, ok := loadGitSource(uid, id)
	if !ok || src.Kind != "gitlab" {
		jsonError(w, "Client access needs a GitLab source", 400)
		return true
	}
	meta := loadGitRelayMeta(id)
	if len(parts) == 3 && parts[2] == "start" && r.Method == http.MethodPost {
		j, key, err := newGitRelayJob(src, uid)
		if err != nil {
			jsonError(w, err.Error(), 409)
		} else {
			jsonOK(w, map[string]interface{}{"job": j.view(), "relay_key": key, "url": "/gitlab-relay?job=" + j.ID})
		}
		return true
	}
	if len(parts) != 2 {
		jsonError(w, "Not found", 404)
		return true
	}
	if r.Method == http.MethodGet {
		meta.Live = liveGitRelay(id, meta.Published) != nil
		members := []string{}
		if src.Owned {
			rows, err := db.Query(`SELECT u.username FROM git_relay_members m JOIN users u ON u.id=m.user_id WHERE source_id=? ORDER BY u.username`, id)
			if err == nil {
				for rows.Next() {
					var user string
					rows.Scan(&user)
					members = append(members, user)
				}
				rows.Close()
			}
		}
		jsonOK(w, map[string]interface{}{"meta": meta, "members": members})
		return true
	}
	if r.Method != http.MethodPut || !src.Owned {
		jsonError(w, "Only the source owner can configure client access", 403)
		return true
	}
	var in struct {
		Enabled bool     `json:"enabled"`
		Members []string `json:"members"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Members) > 100 {
		jsonError(w, "Bad client access settings", 400)
		return true
	}
	tx, err := db.Begin()
	if err != nil {
		jsonError(w, "Cannot store client access", 500)
		return true
	}
	defer tx.Rollback()
	tx.Exec(`INSERT INTO git_relay_meta(source_id,enabled) VALUES(?,?) ON CONFLICT(source_id) DO UPDATE SET enabled=excluded.enabled`, id, in.Enabled)
	tx.Exec(`DELETE FROM git_relay_members WHERE source_id=?`, id)
	for _, name := range in.Members {
		var member int
		if tx.QueryRow(`SELECT id FROM users WHERE username=? AND disabled=0`, strings.TrimSpace(name)).Scan(&member) != nil {
			jsonError(w, "Shared cache member not found", 400)
			return true
		}
		tx.Exec(`INSERT OR IGNORE INTO git_relay_members(source_id,user_id) VALUES(?,?)`, id, member)
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, "Cannot save client access", 500)
		return true
	}
	auditLog(r, uid, "git.client_configured", src.Name, map[string]interface{}{"source_id": id, "enabled": in.Enabled, "members": len(in.Members)})
	jsonOK(w, loadGitRelayMeta(id))
	return true
}

func gitRelayBrowserOrigins(r *http.Request) string {
	session, err := lookupSession(r)
	if err != nil || session.Restricted != "" {
		return ""
	}
	origins := map[string]bool{}
	for _, src := range loadGitSources(session.UserID) {
		if src.Relay == nil {
			continue
		}
		u, err := url.Parse(src.URL)
		if err != nil || u.User != nil || !hostnameRe.MatchString(u.Hostname()) || !oneOf(u.Scheme, "http", "https") {
			continue
		}
		if u.Port() != "" && !validHealthPort(u.Port()) {
			continue
		}
		origins[u.Scheme+"://"+u.Host] = true
	}
	out := []string{}
	for origin := range origins {
		out = append(out, origin)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return ""
	}
	return " " + strings.Join(out, " ")
}

func relayJobPathAllowed(j *gitRelayJob, key string) bool {
	p := strings.SplitN(key, "?", 2)[0]
	for _, project := range j.Projects {
		root := "/projects/" + url.PathEscape(project)
		if p == root || strings.HasPrefix(p, root+"/repository/") {
			return true
		}
	}
	return false
}
func liveGitRelay(source int, generation int64) *gitRelayJob {
	gitRelayJobs.Lock()
	defer gitRelayJobs.Unlock()
	var found *gitRelayJob
	for _, j := range gitRelayJobs.m {
		j.mu.Lock()
		live := j.Source.ID == source && j.Generation == generation && j.State == "cached" && j.ctx.Err() == nil && time.Since(j.LastSeen) < 5*time.Second
		j.mu.Unlock()
		if live {
			found = j
			break
		}
	}
	return found
}

func relayJobAccess(j *gitRelayJob) bool {
	if !relaySourceAllowed(j.UserID, j.Source.ID) || !loadGitRelayMeta(j.Source.ID).Enabled {
		return false
	}
	var raw, kind string
	if db.QueryRow(`SELECT url,kind FROM git_sources WHERE id=?`, j.Source.ID).Scan(&raw, &kind) != nil {
		return false
	}
	return raw == j.Source.URL && kind == "gitlab"
}

func verifyRelayBlob(blob string, data []byte) error {
	if !relaySHA.MatchString(blob) {
		return fmt.Errorf("invalid Git blob identity")
	}
	full := append([]byte(fmt.Sprintf("blob %d\x00", len(data))), data...)
	var actual string
	if len(blob) == 40 {
		sum := sha1.Sum(full)
		actual = hex.EncodeToString(sum[:])
	} else {
		sum := sha256.Sum256(full)
		actual = hex.EncodeToString(sum[:])
	}
	if !strings.EqualFold(blob, actual) {
		return fmt.Errorf("GitLab blob identity mismatch; refusing client content")
	}
	return nil
}
