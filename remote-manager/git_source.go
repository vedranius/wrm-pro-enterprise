// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── GIT SOURCES: GitLab API v4, GitHub REST and Gitea API v1 ───
//
// API calls are kept low: one recursive tree listing per commit (cached forever by commit,
// trees are immutable) gives the blob ID of every file, and file contents are fetched only
// for blobs WRM has not seen before (cached by blob ID per source).

const (
	gitMaxBlobStore  = 8 << 20  // larger contents keep only their hashes
	gitMaxBlobFetch  = 64 << 20 // refuse larger blobs
	gitMaxTreeFiles  = 50000
	gitCommitPages   = 5 // up to 500 commits of a branch for branch-aware tags
	gitDefaultDepth  = 10
	gitMaxDepth      = 50
	gitAPIPageLimit  = 50
	gitMaxProjects   = 2000
	gitFetchParallel = 4
)

type gitSource struct {
	ViewerID      int           `json:"-"`
	Owned         bool          `json:"owned"`
	Relay         *gitRelayMeta `json:"client_cache,omitempty"`
	ID            int           `json:"id"`
	Kind          string        `json:"kind"` // gitlab | github | gitea | bundle
	Name          string        `json:"name"`
	URL           string        `json:"url"`
	HasToken      bool          `json:"has_token"`
	HasWriteToken bool          `json:"has_write_token"`
	HookID        string        `json:"hook_id,omitempty"` // incoming webhook: /api/hooks/git/<hook_id>
	HookAt        string        `json:"hook_at,omitempty"` // last accepted webhook
	CreatedAt     string        `json:"created_at"`
	LastError     string        `json:"last_error,omitempty"`
	// bundles
	BundleID   string   `json:"bundle_id,omitempty"`
	Created    string   `json:"created,omitempty"`
	CreatedBy  string   `json:"created_by,omitempty"`
	Origin     string   `json:"origin,omitempty"`
	Apps       []string `json:"apps,omitempty"`
	token      string
	writeToken string
}

func loadGitSources(userID int) []gitSource {
	out := []gitSource{}
	rows, err := db.Query(`SELECT id, kind, name, url, token, info, created_at, last_error, write_token, hook_id, hook_at, user_id FROM git_sources s WHERE user_id=? OR EXISTS(SELECT 1 FROM git_relay_members m JOIN git_relay_meta r ON r.source_id=m.source_id WHERE m.source_id=s.id AND m.user_id=? AND r.enabled=1) ORDER BY kind='bundle', id`, userID, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var s gitSource
		var tok, info string
		var owner int
		rows.Scan(&s.ID, &s.Kind, &s.Name, &s.URL, &tok, &info, &s.CreatedAt, &s.LastError, &s.writeToken, &s.HookID, &s.HookAt, &owner)
		s.HasToken = tok != ""
		s.HasWriteToken = s.writeToken != ""
		s.token = tok
		s.Owned = owner == userID
		s.ViewerID = userID
		meta := loadGitRelayMeta(s.ID)
		if meta.Enabled {
			meta.Live = liveGitRelay(s.ID, meta.Published) != nil
			s.Relay = &meta
			s.token = ""
			s.writeToken = ""
			s.HasToken = false
			s.HasWriteToken = false
		}
		if s.Kind == "bundle" {
			var m gitBundleManifest
			if json.Unmarshal([]byte(info), &m) == nil {
				s.BundleID, s.Created, s.CreatedBy, s.Origin = m.BundleID, m.Created, m.CreatedBy, m.Source
				for a := range m.Apps {
					s.Apps = append(s.Apps, a)
				}
				sort.Strings(s.Apps)
			}
		}
		out = append(out, s)
	}
	return out
}

func loadGitSource(userID, id int) (gitSource, bool) {
	for _, s := range loadGitSources(userID) {
		if s.ID == id {
			return s, true
		}
	}
	return gitSource{}, false
}

// ─── provider ────────────────────────────────────────

type gitTag struct {
	Name   string
	Commit string
	Date   string
}

type gitCommit struct {
	SHA  string
	Date string // YYYY-MM-DD
}

type gitProvider struct {
	kind   string
	api    string // API base URL
	token  string
	client *http.Client
	calls  int
	mu     sync.Mutex
}

// gitAPIBase derives the API URL: GitLab <url>/api/v4; GitHub api.github.com or
// <url>/api/v3 (Enterprise); Gitea <url>/api/v1. A URL that already contains /api/ is used
// as it is.
func gitAPIBase(kind, raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("enter the address of the Git server (https://…)")
	}
	if err := guardEgressURL(raw); err != nil {
		return "", err
	}
	if strings.Contains(u.Path, "/api/") {
		return raw, nil
	}
	if kind == "github" {
		if strings.EqualFold(u.Hostname(), "github.com") || strings.EqualFold(u.Hostname(), "api.github.com") {
			return "https://api.github.com", nil
		}
		return raw + "/api/v3", nil
	}
	if kind == "gitea" {
		return raw + "/api/v1", nil
	}
	return raw + "/api/v4", nil
}

func newGitProvider(s gitSource) (*gitProvider, error) {
	if !gitAPIKind(s.Kind) {
		return nil, fmt.Errorf("not an API source")
	}
	api, err := gitAPIBase(s.Kind, s.URL)
	if err != nil {
		return nil, err
	}
	meta := loadGitRelayMeta(s.ID)
	if meta.Enabled {
		return &gitProvider{kind: s.Kind, api: api, client: &http.Client{Transport: &gitRelayTransport{Source: s.ID, UserID: s.ViewerID, Generation: meta.Published, API: api}}}, nil
	}
	u, _ := url.Parse(api)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if u.Scheme == "https" {
		tr.TLSClientConfig = pinnedTLSConfig("git://", u.Host)
	}
	return &gitProvider{kind: s.Kind, api: api, token: decryptValue(s.token),
		client: &http.Client{Timeout: 60 * time.Second, Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || req.URL.Host != u.Host {
				return http.ErrUseLastResponse // never send the token to another host
			}
			return nil
		}}}, nil
}

// gitAPIKind tells whether a source kind is a Git server with an API.
func gitAPIKind(kind string) bool { return kind == "gitlab" || kind == "github" || kind == "gitea" }

// hub tells whether the provider speaks the GitHub-style REST API (GitHub and Gitea).
func (p *gitProvider) hub() bool { return p.kind != "gitlab" }

type gitHTTPError struct {
	Status int
	Msg    string
	// rate limit headers of the answer (429, or 403 with no calls left)
	Remaining string
	Reset     string
}

func (e *gitHTTPError) Error() string { return e.Msg }

func (p *gitProvider) get(rel string, accept string) (*http.Response, error) {
	req, err := http.NewRequest("GET", p.api+rel, nil)
	if err != nil {
		return nil, err
	}
	if accept == "" {
		accept = "application/json"
		if p.kind == "github" {
			accept = "application/vnd.github+json"
		}
	}
	return p.do(req, accept)
}

// do sends a request with the token and turns an answer other than 2xx into a gitHTTPError.
func (p *gitProvider) do(req *http.Request, accept string) (*http.Response, error) {
	if p.token != "" {
		if p.kind == "gitlab" {
			req.Header.Set("PRIVATE-TOKEN", p.token)
		} else if p.kind == "gitea" {
			req.Header.Set("Authorization", "token "+p.token)
		} else {
			req.Header.Set("Authorization", "Bearer "+p.token)
		}
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "WRM-PRO/"+AppVersion)
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	resp, err := p.client.Do(req)
	if err != nil {
		if _, relay := p.client.Transport.(*gitRelayTransport); relay {
			if wrapped, ok := err.(*url.Error); ok {
				err = wrapped.Err
			}
			return nil, fmt.Errorf("Client GitLab cache: %s", truncateStr(err.Error(), 500))
		}
		return nil, fmt.Errorf("cannot reach the Git server: %s", shortNetError(err))
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		msg := fmt.Sprintf("Git server answered HTTP %d", resp.StatusCode)
		var e struct {
			Message interface{} `json:"message"`
			Error   string      `json:"error"`
		}
		if json.Unmarshal(body, &e) == nil {
			m := strings.TrimSpace(e.Error)
			if e.Message != nil {
				m = strings.TrimSpace(fmt.Sprint(e.Message) + " " + m)
			}
			if m != "" {
				msg += ": " + truncateStr(m, 200)
			}
		}
		switch resp.StatusCode {
		case 401:
			msg += " (check the token)"
		case 403:
			msg += " (no access, or the API rate limit was reached)"
		case 404:
			msg += " (not found, or no access)"
		case 429:
			msg += " (the API rate limit was reached)"
		}
		return nil, &gitHTTPError{Status: resp.StatusCode, Msg: msg, Remaining: rateRemaining(resp.Header), Reset: rateReset(resp.Header)}
	}
	return resp, nil
}

func (p *gitProvider) getJSON(rel string, out interface{}) (http.Header, error) {
	resp, err := p.get(rel, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out); err != nil {
		return nil, fmt.Errorf("unexpected answer from the Git server: %v", err)
	}
	return resp.Header, nil
}

// pages calls fn for every page of a list endpoint (rel already has a query string).
func (p *gitProvider) pages(rel string, maxPages int, fn func(raw json.RawMessage) (int, error)) error {
	for page := 1; page <= maxPages; page++ {
		var raw json.RawMessage
		h, err := p.getJSON(rel+p.pageQuery(page), &raw)
		if err != nil {
			return err
		}
		n, err := fn(raw)
		if err != nil {
			return err
		}
		if p.kind == "gitlab" {
			if next := h.Get("X-Next-Page"); next == "" || next == "0" {
				return nil
			}
		} else if !strings.Contains(h.Get("Link"), `rel="next"`) {
			return nil
		}
		if n == 0 {
			return nil
		}
	}
	return nil
}

// pageQuery is the paging part of a list request: Gitea takes limit (at most 50 by default).
func (p *gitProvider) pageQuery(page int) string {
	if p.kind == "gitea" {
		return "&limit=50&page=" + strconv.Itoa(page)
	}
	return "&per_page=100&page=" + strconv.Itoa(page)
}

func (p *gitProvider) gl(project string) string { return "/projects/" + url.PathEscape(project) }

func (p *gitProvider) gh(project string) string {
	parts := strings.SplitN(project, "/", 2)
	if len(parts) != 2 {
		return "/repos/" + url.PathEscape(project)
	}
	return "/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1])
}

func dateOnly(s string) string {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format("2006-01-02")
	}
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// defaultBranch returns the project's default branch.
func (p *gitProvider) defaultBranch(project string) (string, error) {
	var v struct {
		DefaultBranch string `json:"default_branch"`
	}
	rel := p.gl(project)
	if p.hub() {
		rel = p.gh(project)
	}
	if _, err := p.getJSON(rel, &v); err != nil {
		return "", err
	}
	if v.DefaultBranch == "" {
		return "", fmt.Errorf("%s has no default branch (empty repository?)", project)
	}
	return v.DefaultBranch, nil
}

func (p *gitProvider) branches(project string) ([]string, error) {
	out := []string{}
	rel := p.gl(project) + "/repository/branches?x=1"
	if p.hub() {
		rel = p.gh(project) + "/branches?x=1"
	}
	err := p.pages(rel, gitAPIPageLimit, func(raw json.RawMessage) (int, error) {
		var list []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return 0, err
		}
		for _, b := range list {
			out = append(out, b.Name)
		}
		return len(list), nil
	})
	return out, err
}

func (p *gitProvider) tags(project string) ([]gitTag, error) {
	out := []gitTag{}
	rel := p.gl(project) + "/repository/tags?x=1"
	if p.hub() {
		rel = p.gh(project) + "/tags?x=1"
	}
	err := p.pages(rel, gitAPIPageLimit, func(raw json.RawMessage) (int, error) {
		var list []struct {
			Name   string `json:"name"`
			Commit struct {
				ID            string `json:"id"`
				SHA           string `json:"sha"`
				CommittedDate string `json:"committed_date"`
				Created       string `json:"created"` // Gitea
			} `json:"commit"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return 0, err
		}
		for _, t := range list {
			c := t.Commit.ID
			if c == "" {
				c = t.Commit.SHA
			}
			d := t.Commit.CommittedDate
			if d == "" {
				d = t.Commit.Created
			}
			out = append(out, gitTag{Name: t.Name, Commit: c, Date: dateOnly(d)})
		}
		return len(list), nil
	})
	return out, err
}

// branchCommits lists the newest commits of a branch (newest first).
func (p *gitProvider) branchCommits(project, branch string, pages int) ([]gitCommit, error) {
	out := []gitCommit{}
	rel := p.gl(project) + "/repository/commits?ref_name=" + url.QueryEscape(branch)
	if p.hub() {
		rel = p.gh(project) + "/commits?sha=" + url.QueryEscape(branch)
	}
	if p.kind == "gitea" {
		rel += "&stat=false&verification=false&files=false"
	}
	err := p.pages(rel, pages, func(raw json.RawMessage) (int, error) {
		var list []struct {
			ID            string `json:"id"`
			SHA           string `json:"sha"`
			CommittedDate string `json:"committed_date"`
			Commit        struct {
				Committer struct {
					Date string `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return 0, err
		}
		for _, c := range list {
			if c.ID != "" {
				out = append(out, gitCommit{SHA: c.ID, Date: dateOnly(c.CommittedDate)})
			} else {
				out = append(out, gitCommit{SHA: c.SHA, Date: dateOnly(c.Commit.Committer.Date)})
			}
		}
		return len(list), nil
	})
	return out, err
}

// commitDate looks up one commit (a tag outside the listed branch history).
func (p *gitProvider) commitDate(project, sha string) string {
	var v struct {
		CommittedDate string `json:"committed_date"`
		Commit        struct {
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	rel := p.gl(project) + "/repository/commits/" + url.PathEscape(sha)
	if p.kind == "github" {
		rel = p.gh(project) + "/commits/" + url.PathEscape(sha)
	} else if p.kind == "gitea" {
		rel = p.gh(project) + "/git/commits/" + url.PathEscape(sha)
	}
	if _, err := p.getJSON(rel, &v); err != nil {
		return ""
	}
	if v.CommittedDate != "" {
		return dateOnly(v.CommittedDate)
	}
	return dateOnly(v.Commit.Committer.Date)
}

// rawTree lists every file of a commit: path → blob ID.
func (p *gitProvider) rawTree(project, commit string) (map[string]string, error) {
	out := map[string]string{}
	if p.kind == "gitea" {
		return p.giteaTree(project, commit)
	}
	if p.kind == "github" {
		var v struct {
			Tree []struct {
				Path string `json:"path"`
				Type string `json:"type"`
				SHA  string `json:"sha"`
			} `json:"tree"`
			Truncated bool `json:"truncated"`
		}
		if _, err := p.getJSON(p.gh(project)+"/git/trees/"+url.PathEscape(commit)+"?recursive=1", &v); err != nil {
			return nil, err
		}
		if v.Truncated {
			return nil, fmt.Errorf("%s: the repository tree is too large for one listing", project)
		}
		for _, e := range v.Tree {
			if e.Type == "blob" {
				out[e.Path] = e.SHA
			}
		}
		return out, nil
	}
	err := p.pages(p.gl(project)+"/repository/tree?recursive=true&ref="+url.QueryEscape(commit), 1000, func(raw json.RawMessage) (int, error) {
		var list []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return 0, err
		}
		for _, e := range list {
			if e.Type == "blob" {
				out[e.Path] = e.ID
			}
		}
		if len(out) > gitMaxTreeFiles {
			return 0, fmt.Errorf("%s: more than %d files", project, gitMaxTreeFiles)
		}
		return len(list), nil
	})
	return out, err
}

// giteaTree lists a Gitea tree page by page (the recursive listing is paginated there).
func (p *gitProvider) giteaTree(project, commit string) (map[string]string, error) {
	out := map[string]string{}
	for page := 1; page <= 1000; page++ {
		var v struct {
			Tree []struct {
				Path string `json:"path"`
				Type string `json:"type"`
				SHA  string `json:"sha"`
			} `json:"tree"`
			Truncated bool `json:"truncated"`
		}
		if _, err := p.getJSON(p.gh(project)+"/git/trees/"+url.PathEscape(commit)+"?recursive=true&per_page=1000&page="+strconv.Itoa(page), &v); err != nil {
			return nil, err
		}
		for _, e := range v.Tree {
			if e.Type == "blob" {
				out[e.Path] = e.SHA
			}
		}
		if len(out) > gitMaxTreeFiles {
			return nil, fmt.Errorf("%s: more than %d files", project, gitMaxTreeFiles)
		}
		if !v.Truncated || len(v.Tree) == 0 {
			break
		}
	}
	return out, nil
}

func (p *gitProvider) blob(project, sha string) ([]byte, error) {
	if p.hub() {
		var v struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
			Size     int64  `json:"size"`
		}
		if _, err := p.getJSON(p.gh(project)+"/git/blobs/"+url.PathEscape(sha), &v); err != nil {
			return nil, err
		}
		if v.Encoding != "base64" {
			return []byte(v.Content), nil
		}
		return base64.StdEncoding.DecodeString(strings.ReplaceAll(v.Content, "\n", ""))
	}
	resp, err := p.get(p.gl(project)+"/repository/blobs/"+url.PathEscape(sha)+"/raw", "*/*")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, gitMaxBlobFetch+1))
	if err != nil {
		return nil, err
	}
	if len(data) > gitMaxBlobFetch {
		return nil, fmt.Errorf("file larger than %d MB", gitMaxBlobFetch>>20)
	}
	return data, nil
}

// projects lists the projects of groups (GitLab, with subgroups) or organisations / users
// (GitHub). Without groups: the projects the token is a member of.
func (p *gitProvider) projects(groups []string) ([]string, error) {
	if relay, ok := p.client.Transport.(*gitRelayTransport); ok && relay.Job == nil {
		if !relaySourceAllowed(relay.UserID, relay.Source) {
			return nil, fmt.Errorf("shared source access removed")
		}
		meta := loadGitRelayMeta(relay.Source)
		res, found := relayCached(relay.Source, meta.Published, "/_wrm/projects")
		if !found {
			return nil, fmt.Errorf("connect a client to synchronize catalog repositories first")
		}
		var projects []string
		json.Unmarshal(res.Body, &projects)
		if len(groups) == 0 {
			return projects, nil
		}
		out := []string{}
		for _, p := range projects {
			for _, g := range groups {
				if strings.HasPrefix(p, strings.TrimSuffix(g, "/")+"/") {
					out = append(out, p)
					break
				}
			}
		}
		return out, nil
	}

	seen := map[string]bool{}
	out := []string{}
	add := func(raw json.RawMessage) (int, error) {
		var list []struct {
			PathWithNamespace string `json:"path_with_namespace"`
			FullName          string `json:"full_name"`
			Archived          bool   `json:"archived"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return 0, err
		}
		for _, x := range list {
			n := x.PathWithNamespace
			if n == "" {
				n = x.FullName
			}
			if n != "" && !x.Archived && !seen[n] && len(out) < gitMaxProjects {
				seen[n] = true
				out = append(out, n)
			}
		}
		return len(list), nil
	}
	if len(groups) == 0 {
		rel := "/projects?membership=true&simple=true&archived=false"
		if p.hub() {
			rel = "/user/repos?sort=full_name"
		}
		if err := p.pages(rel, 20, add); err != nil {
			return nil, err
		}
	}
	for _, g := range groups {
		var err error
		if p.hub() {
			err = p.pages("/orgs/"+url.PathEscape(g)+"/repos?type=all", 20, add)
			var he *gitHTTPError
			if errors.As(err, &he) && he.Status == 404 {
				err = p.pages("/users/"+url.PathEscape(g)+"/repos?type=owner", 20, add)
			}
		} else {
			err = p.pages("/groups/"+url.PathEscape(g)+"/projects?include_subgroups=true&simple=true&archived=false", 20, add)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %v", g, err)
		}
	}
	sort.Strings(out)
	return out, nil
}

// ─── caches ──────────────────────────────────────────

// tree returns the cached tree of a commit, or lists it once.
func (p *gitProvider) tree(sourceID int, project, commit string) (map[string]string, error) {
	var data string
	if db.QueryRow(`SELECT data FROM git_trees WHERE source_id=? AND project=? AND commit_id=?`, sourceID, project, commit).Scan(&data) == nil {
		m := map[string]string{}
		if json.Unmarshal([]byte(data), &m) == nil {
			return m, nil
		}
	}
	m, err := p.rawTree(project, commit)
	if err != nil {
		return nil, err
	}
	db.Exec(`INSERT OR REPLACE INTO git_trees (source_id, project, commit_id, data, fetched_at) VALUES (?,?,?,?,?)`,
		sourceID, project, commit, string(jsonMarshal(m)), nowStamp())
	return m, nil
}

type gitBlobInfo struct {
	Hash  string
	Hash2 string
	Size  int64
}

func lookupBlob(sourceID int, blob string) (gitBlobInfo, bool) {
	var b gitBlobInfo
	err := db.QueryRow(`SELECT hash, hash2, size FROM git_blobs WHERE source_id=? AND blob=?`, sourceID, blob).Scan(&b.Hash, &b.Hash2, &b.Size)
	return b, err == nil
}

func blobContent(sourceID int, blob string) ([]byte, bool) {
	var c []byte
	if db.QueryRow(`SELECT content FROM git_blobs WHERE source_id=? AND blob=? AND content IS NOT NULL`, sourceID, blob).Scan(&c) != nil {
		return nil, false
	}
	return c, true
}

func storeBlob(sourceID int, blob string, content []byte) gitBlobInfo {
	b := gitBlobInfo{Hash: normHash(content), Hash2: trHash(content), Size: int64(len(content))}
	var stored interface{}
	if len(content) <= gitMaxBlobStore {
		stored = content
	}
	db.Exec(`INSERT OR REPLACE INTO git_blobs (source_id, blob, hash, hash2, size, content, fetched_at) VALUES (?,?,?,?,?,?,?)`,
		sourceID, blob, b.Hash, b.Hash2, b.Size, stored, nowStamp())
	return b
}

// ensureBlobs fetches the blobs not cached yet (a few in parallel).
func (p *gitProvider) ensureBlobs(sourceID int, project string, blobs []string) (map[string]gitBlobInfo, error) {
	out := map[string]gitBlobInfo{}
	var missing []string
	for _, b := range blobs {
		if _, done := out[b]; done {
			continue
		}
		if info, ok := lookupBlob(sourceID, b); ok {
			out[b] = info
		} else {
			out[b] = gitBlobInfo{}
			missing = append(missing, b)
		}
	}
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, gitFetchParallel)
	var wg sync.WaitGroup
	for _, b := range missing {
		wg.Add(1)
		go func(b string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			data, err := p.blob(project, b)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			out[b] = storeBlob(sourceID, b, data)
		}(b)
	}
	wg.Wait()
	return out, firstErr
}

// ─── targets ─────────────────────────────────────────

type gitHistEntry struct {
	Commit     string `json:"commit"`
	CommitFull string `json:"commit_full,omitempty"`
	Version    int    `json:"version,omitempty"` // index in gitTarget.Versions (0 = target)
	Date       string `json:"date"`
	Hash       string `json:"hash"`
	Tag        string `json:"tag,omitempty"`
	Hash2      string `json:"hash2,omitempty"` // WRM only (not in bundles)
}

type gitTargetFile struct {
	Hash    string         `json:"hash"`
	Hash2   string         `json:"hash2,omitempty"`
	Blob    string         `json:"blob,omitempty"`
	Size    int64          `json:"size,omitempty"`
	Oldest  int            `json:"oldest,omitempty"` // oldest version in Versions that still has this file
	History []gitHistEntry `json:"history"`
}

// gitTargetVersion is one known version of a service: the target and the earlier tags.
type gitTargetVersion struct {
	Commit     string `json:"commit"`
	CommitFull string `json:"commit_full,omitempty"`
	Date       string `json:"date"`
	Tag        string `json:"tag,omitempty"`
}

// gitTarget is what an installation is compared with: one service at one ref.
type gitTarget struct {
	App         string                   `json:"app"`
	SourceID    int                      `json:"source_id"`
	Source      string                   `json:"source"` // gitlab:<url> | github:<url> | bundle:<id>
	Project     string                   `json:"project"`
	Ref         string                   `json:"ref"`
	Branch      string                   `json:"branch"`
	RefKind     string                   `json:"ref_kind"` // tag | branch
	Version     string                   `json:"version"`
	LatestTag   string                   `json:"latest_tag"`
	Commit      string                   `json:"commit"`
	CommitFull  string                   `json:"commit_full,omitempty"` // the whole SHA (tree cache key, commit lists)
	CommitDate  string                   `json:"commit_date"`
	Subdir      string                   `json:"subdir"`
	Kind        string                   `json:"kind"`
	Protected   []string                 `json:"protected"`
	Fingerprint []string                 `json:"fingerprint"`
	InstallHint []string                 `json:"install_hint"`
	Exclude     []string                 `json:"exclude"`
	Include     []string                 `json:"include"`
	Files       map[string]gitTargetFile `json:"files"`
	Versions    []gitTargetVersion       `json:"versions,omitempty"` // target first, then earlier tags
	Warnings    []string                 `json:"warnings"`
	Error       string                   `json:"error,omitempty"`
	ComputedAt  string                   `json:"computed_at"`
	APICalls    int                      `json:"api_calls,omitempty"`
}

// effectiveRef applies the ref choice of a run to a catalog entry.
func effectiveRef(a gitCatalogApp, st gitSettings) string {
	if r := strings.TrimSpace(st.RefOverrides[a.Name]); r != "" {
		return r
	}
	switch st.RefMode {
	case "branch":
		if b := strings.TrimSpace(st.RefBranch); b != "" {
			return b
		}
	case "default":
		return ":default"
	}
	return a.Ref
}

// selectFiles maps a repository tree to the installation's relative paths.
func selectFiles(tree map[string]string, a gitCatalogApp) map[string]string {
	out := map[string]string{}
	prefix := ""
	if a.Subdir != "" {
		prefix = a.Subdir + "/"
	}
	for p, blob := range tree {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rel := p[len(prefix):]
		if rel == "" || (len(a.Include) > 0 && !globHitDir(rel, a.Include)) || globHitDir(rel, a.Exclude) {
			continue
		}
		// log files committed by mistake are never compared or deployed (a deploy would
		// overwrite the server's logs), unless the include list names them
		if isLogPath(rel) && len(a.Include) == 0 {
			continue
		}
		out[rel] = blob
	}
	return out
}

type gitVersionRef struct {
	commit, date, tag string
}

// computeTarget resolves the ref of a service and builds its files with per-file history.
func computeTarget(p *gitProvider, src gitSource, cat *gitCatalog, a gitCatalogApp, st gitSettings) gitTarget {
	t := gitTarget{App: a.Name, SourceID: src.ID, Source: src.Kind + ":" + src.URL, Project: a.Project, Subdir: a.Subdir, Kind: a.Kind,
		Protected: a.Protected, Fingerprint: a.Fingerprint, InstallHint: a.InstallHint, Exclude: a.Exclude, Include: a.Include,
		Files: map[string]gitTargetFile{}, Warnings: []string{}, ComputedAt: nowStamp()}
	if src.Relay != nil {
		t.Warnings = append(t.Warnings, fmt.Sprintf("Client GitLab snapshot #%d, refreshed %s by %s. %s", src.Relay.Published, src.Relay.At, src.Relay.Contributor, map[bool]string{true: "Cache is older than 24 hours; refresh from an authorized client before choosing a newer deployment.", false: "Deployments use the reviewed immutable content; live GitLab writes require a direct source."}[src.Relay.Stale]))
	}
	fail := func(err error) gitTarget {
		t.Error = err.Error()
		t.APICalls = p.calls
		return t
	}
	ref := effectiveRef(a, st)
	t.Ref = ref
	def, err := p.defaultBranch(a.Project)
	if err != nil {
		return fail(err)
	}
	branches, err := p.branches(a.Project)
	if err != nil {
		return fail(err)
	}
	has := map[string]bool{}
	for _, b := range branches {
		has[b] = true
	}
	pickBranch := func(want string) string {
		if want == "" || want == ":default" {
			return def
		}
		if has[want] {
			return want
		}
		for _, fb := range cat.FallbackBranches {
			if has[fb] {
				t.Warnings = append(t.Warnings, fmt.Sprintf("branch %s does not exist, using %s", want, fb))
				return fb
			}
		}
		t.Warnings = append(t.Warnings, fmt.Sprintf("branch %s does not exist, using the default branch %s", want, def))
		return def
	}
	isTagRef := strings.HasPrefix(ref, "tag:")
	if isTagRef {
		t.Branch = pickBranch(a.Branch)
	} else {
		t.Branch = pickBranch(ref)
	}
	commits, err := p.branchCommits(a.Project, t.Branch, gitCommitPages)
	if err != nil {
		return fail(err)
	}
	if len(commits) == 0 {
		return fail(fmt.Errorf("branch %s has no commits", t.Branch))
	}
	pos := map[string]int{}
	for i, c := range commits {
		if _, ok := pos[c.SHA]; !ok {
			pos[c.SHA] = i
		}
	}
	tags, err := p.tags(a.Project)
	if err != nil {
		return fail(err)
	}
	var filter *regexp.Regexp
	if a.TagFilter != "" {
		filter, _ = regexp.Compile(a.TagFilter)
	}
	// branch-aware tags: only tags whose commit is on the branch
	var onBranch []gitTag
	byName := map[string]gitTag{}
	for _, tg := range tags {
		byName[tg.Name] = tg
		if filter != nil && !filter.MatchString(tg.Name) {
			continue
		}
		if i, ok := pos[tg.Commit]; ok {
			tg.Date = commits[i].Date
			onBranch = append(onBranch, tg)
		}
	}
	sort.SliceStable(onBranch, func(i, j int) bool { return compareVersions(onBranch[i].Name, onBranch[j].Name) > 0 })
	if len(onBranch) > 0 {
		t.LatestTag = onBranch[0].Name
	}
	target := gitVersionRef{commit: commits[0].SHA, date: commits[0].Date}
	t.RefKind = "branch"
	switch {
	case ref == "tag:latest":
		if len(onBranch) == 0 {
			t.Warnings = append(t.Warnings, fmt.Sprintf("no matching tag on branch %s, using the branch head", t.Branch))
		} else {
			target = gitVersionRef{commit: onBranch[0].Commit, date: onBranch[0].Date, tag: onBranch[0].Name}
			t.RefKind = "tag"
		}
	case isTagRef:
		want := strings.TrimPrefix(ref, "tag:")
		tg, ok := byName[want]
		if !ok { // tag:v1 → the newest v1.x on the branch
			for _, x := range onBranch {
				if strings.HasPrefix(x.Name, want+".") || strings.HasPrefix(x.Name, want+"-") {
					tg, ok = x, true
					break
				}
			}
		}
		if !ok {
			t.Warnings = append(t.Warnings, fmt.Sprintf("tag %s not found, using the head of %s", want, t.Branch))
		} else {
			if tg.Date == "" {
				if i, on := pos[tg.Commit]; on {
					tg.Date = commits[i].Date
				} else {
					tg.Date = p.commitDate(a.Project, tg.Commit)
				}
			}
			if _, on := pos[tg.Commit]; !on {
				t.Warnings = append(t.Warnings, fmt.Sprintf("tag %s is not on branch %s", tg.Name, t.Branch))
			}
			target = gitVersionRef{commit: tg.Commit, date: tg.Date, tag: tg.Name}
			t.RefKind = "tag"
		}
	}
	t.Commit, t.CommitFull, t.CommitDate = short10(target.commit), target.commit, target.date
	t.Version = target.tag
	if t.Version == "" {
		t.Version = short10(target.commit)
	}
	// history: the target, then the earlier tags of the branch (newest first)
	depth := st.HistoryDepth
	versions := []gitVersionRef{target}
	tpos, onb := pos[target.commit]
	seenCommit := map[string]bool{target.commit: true}
	byPos := append([]gitTag(nil), onBranch...)
	sort.SliceStable(byPos, func(i, j int) bool { return pos[byPos[i].Commit] < pos[byPos[j].Commit] })
	for _, tg := range byPos {
		if len(versions) > depth {
			break
		}
		if seenCommit[tg.Commit] || (onb && pos[tg.Commit] < tpos) {
			continue
		}
		seenCommit[tg.Commit] = true
		versions = append(versions, gitVersionRef{commit: tg.Commit, date: tg.Date, tag: tg.Name})
	}
	trees := make([]map[string]string, len(versions))
	var blobs []string
	for i, v := range versions {
		tr, err := p.tree(src.ID, a.Project, v.commit)
		if err != nil {
			if i == 0 {
				return fail(err)
			}
			t.Warnings = append(t.Warnings, fmt.Sprintf("history of %s incomplete: %v", v.tag, err))
			break
		}
		trees[i] = selectFiles(tr, a)
		t.Versions = append(t.Versions, gitTargetVersion{Commit: short10(v.commit), CommitFull: v.commit, Date: v.date, Tag: v.tag})
		for _, b := range trees[i] {
			blobs = append(blobs, b)
		}
	}
	if len(trees[0]) == 0 {
		t.Warnings = append(t.Warnings, "no files at this ref (check the subdirectory and include list)")
	}
	info, err := p.ensureBlobs(src.ID, a.Project, blobs)
	if err != nil {
		return fail(err)
	}
	for rel, blob := range trees[0] {
		bi := info[blob]
		oldest := 0
		for i := range versions {
			if trees[i] == nil {
				break
			}
			if _, ok := trees[i][rel]; !ok {
				break
			}
			oldest = i
		}
		f := gitTargetFile{Hash: bi.Hash, Hash2: bi.Hash2, Blob: blob, Size: bi.Size, Oldest: oldest}
		lastHash := ""
		for i, v := range versions {
			if trees[i] == nil {
				break
			}
			b, ok := trees[i][rel]
			if !ok {
				continue
			}
			h := info[b]
			if i > 0 && h.Hash == lastHash {
				continue
			}
			lastHash = h.Hash
			e := gitHistEntry{Commit: short10(v.commit), CommitFull: v.commit, Version: i, Date: v.date, Hash: h.Hash, Tag: v.tag}
			if h.Hash2 != h.Hash {
				e.Hash2 = h.Hash2
			}
			f.History = append(f.History, e)
		}
		if f.Hash2 == f.Hash {
			f.Hash2 = ""
		}
		t.Files[rel] = f
	}
	t.APICalls = p.calls
	return t
}

// suggestApp builds a catalog entry from the repository tree of a project.
func suggestApp(p *gitProvider, src gitSource, project, ref string) (gitCatalogApp, []string, error) {
	a := gitCatalogApp{Name: path.Base(project), Project: project, Ref: "tag:latest", Kind: "app"}
	if !gitNameRe.MatchString(a.Name) {
		a.Name = "service"
	}
	def, err := p.defaultBranch(project)
	if err != nil {
		return a, nil, err
	}
	a.Branch = def
	if ref == "" {
		ref = def
	}
	commits, err := p.branchCommits(project, ref, 1)
	if err != nil || len(commits) == 0 {
		if err == nil {
			err = fmt.Errorf("no commits on %s", ref)
		}
		return a, nil, err
	}
	tree, err := p.tree(src.ID, project, commits[0].SHA)
	if err != nil {
		return a, nil, err
	}
	if tags, err := p.tags(project); err == nil && len(tags) == 0 {
		a.Ref = def
	}
	files := make([]string, 0, len(tree))
	for f := range tree {
		files = append(files, f)
	}
	sort.Strings(files)
	// a subdirectory for servers (linux/, server/, src/<name>/) when the root is only a wrapper
	for _, cand := range []string{"linux", "server", "deploy", "src/" + a.Name, a.Name} {
		n := 0
		for _, f := range files {
			if strings.HasPrefix(f, cand+"/") {
				n++
			}
		}
		if n > 0 && n*2 >= len(files) {
			a.Subdir = cand
			break
		}
	}
	rels := []string{}
	prefix := ""
	if a.Subdir != "" {
		prefix = a.Subdir + "/"
	}
	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
			rels = append(rels, f[len(prefix):])
		}
	}
	generic := map[string]bool{"readme.md": true, "readme": true, "readme.txt": true, "license": true, "license.md": true, "changelog.md": true,
		".gitignore": true, ".gitattributes": true, "requirements.txt": true, "version.md": true, ".gitlab-ci.yml": true, "makefile": true,
		"setup.cfg": true, "dockerfile": true, "jenkinsfile": true}
	exclude := []string{".git*", "README*", "*.md", "tests", "test", "docs", ".github", ".gitlab-ci.yml", "Jenkinsfile"}
	protected := []string{}
	for _, g := range []string{"config*", "*.ini", ".env", "*.env", "*.conf", "settings_local*", "*.local.*"} {
		for _, r := range rels {
			if globHit(r, []string{g}) && !globHit(r, exclude) {
				protected = append(protected, g)
				break
			}
		}
	}
	var fp []string
	score := func(r string) int {
		switch strings.ToLower(path.Ext(r)) {
		case ".py", ".sh", ".js", ".pl", ".rb", ".php", ".go", ".ps1":
			return 3
		case ".json", ".toml", ".cfg":
			return 1
		}
		return 0
	}
	var top []string
	for _, r := range rels {
		if !strings.Contains(r, "/") && !generic[strings.ToLower(r)] && !globHit(r, protected) && !globHit(r, exclude) {
			top = append(top, r)
		}
	}
	sort.SliceStable(top, func(i, j int) bool { return score(top[i]) > score(top[j]) })
	for _, r := range top {
		if len(fp) == 2 {
			break
		}
		fp = append(fp, r)
	}
	isLib := false
	for _, r := range rels {
		switch strings.ToLower(r) {
		case "setup.py", "pyproject.toml", "package.json":
			isLib = true
		}
	}
	if isLib && len(top) <= 3 {
		a.Kind = "library"
	} else if len(rels) <= 3 {
		a.Kind = "tool"
	}
	a.Fingerprint = fp
	a.Protected = protected
	a.Exclude = exclude
	a.InstallHint = []string{a.Name}
	a.Include = []string{}
	if len(rels) > 400 {
		rels = rels[:400]
	}
	return a, rels, nil
}
