// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ─── MEDIUM: native preview and comparison with the original tool (read-only) ───
//
// The native preview reads the operator's deploy-config (mdLoadConfig), prepares the source
// of the chosen branch or tag from a WRM Git source whose address matches the project's
// repo, and compares it with every chosen destination through the WRM connection linked
// to that server (mdWalk, mdDiff, mdReadState). Nothing is written to any server.
//
// The comparison runs the original tool's own comparison through the Medium wrapper
// (/__wrm_plan) and the native engine on the very source the original prepared (its
// export, after its build), so the two change lists can be put side by side per server.

func initMediumNativeSchema() {
	ensureTable("medium_server_links", `CREATE TABLE IF NOT EXISTS medium_server_links (
		user_id INTEGER NOT NULL,
		server TEXT NOT NULL,
		conn_id INTEGER NOT NULL,
		PRIMARY KEY (user_id, server))`,
		"user_id", "server", "conn_id")
}

// ─── paths as the original tool writes them in JSON ───

// mdEscapePath is the original escape_path: bytes that are not UTF-8 as rsync's \#ooo and a
// literal "\#" as "\#134#", so a name survives JSON unchanged.
func mdEscapePath(p string) string {
	if utf8.ValidString(p) && !strings.Contains(p, `\#`) {
		return p
	}
	var sb strings.Builder
	for i := 0; i < len(p); {
		r, n := utf8.DecodeRuneInString(p[i:])
		if r == utf8.RuneError && n == 1 {
			fmt.Fprintf(&sb, `\#%03o`, p[i])
			i++
			continue
		}
		if p[i] == '\\' && i+1 < len(p) && p[i+1] == '#' {
			sb.WriteString(`\#134`)
			i++
			continue
		}
		sb.WriteString(p[i : i+n])
		i += n
	}
	return sb.String()
}

var mdEscapeRe = regexp.MustCompile(`(?:\\#[0-3][0-7]{2})+`)

// mdUnescapePath turns \#ooo sequences back into the bytes of the name.
func mdUnescapePath(p string) string {
	if !strings.Contains(p, `\#`) {
		return p
	}
	return mdEscapeRe.ReplaceAllStringFunc(p, func(m string) string {
		var b []byte
		for _, code := range strings.Split(m, `\#`)[1:] {
			v, _ := strconv.ParseUint(code, 8, 8)
			b = append(b, byte(v))
		}
		return string(b)
	})
}

// ─── repository address → WRM Git source ───

// mdRepoKey returns the host (lower case, no port) and project path of a Git address:
// git@host:group/app.git, ssh://git@host:2222/group/app.git or https://host/group/app.git.
func mdRepoKey(repo string) (string, string) {
	repo = strings.TrimSpace(repo)
	var host, p string
	if u, err := url.Parse(repo); err == nil && u.Host != "" && strings.Contains(repo, "://") {
		host, p = u.Hostname(), u.Path
	} else if at := strings.Index(repo, "@"); at >= 0 && strings.Contains(repo[at:], ":") {
		rest := repo[at+1:]
		i := strings.Index(rest, ":")
		host, p = rest[:i], rest[i+1:]
	} else {
		return "", ""
	}
	p = strings.TrimSuffix(strings.Trim(p, "/"), ".git")
	return strings.ToLower(host), p
}

type mdSourceMatch struct {
	src     gitSource
	project string
}

// mdFindSource finds the user's API Git source that serves the project's repository.
func mdFindSource(uid int, repo string) (mdSourceMatch, bool) {
	host, project := mdRepoKey(repo)
	if host == "" || project == "" {
		return mdSourceMatch{}, false
	}
	for _, s := range loadGitSources(uid) {
		if !gitAPIKind(s.Kind) {
			continue
		}
		u, err := url.Parse(strings.TrimSpace(s.URL))
		if err != nil {
			continue
		}
		h := strings.ToLower(u.Hostname())
		if s.Kind == "github" && h == "api.github.com" {
			h = "github.com"
		}
		if h != host {
			continue
		}
		rel := project
		if prefix := strings.Trim(u.Path, "/"); prefix != "" && !strings.Contains(prefix, "api/") {
			if !strings.HasPrefix(rel, prefix+"/") {
				continue
			}
			rel = strings.TrimPrefix(rel, prefix+"/")
		}
		return mdSourceMatch{src: s, project: rel}, true
	}
	return mdSourceMatch{}, false
}

// mdResolveRef turns a branch, tag, full ref or commit into the commit and the full ref.
func mdResolveRef(p *gitProvider, project, ref string) (string, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "-") {
		return "", "", fmt.Errorf("choose a branch or a tag")
	}
	if regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(ref) {
		return ref, ref, nil
	}
	branch := func(name string) (string, error) {
		var v struct {
			Commit struct {
				ID  string `json:"id"`
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		rel := p.gl(project) + "/repository/branches/" + url.PathEscape(name)
		if p.hub() {
			rel = p.gh(project) + "/branches/" + url.PathEscape(name)
		}
		if _, err := p.getJSON(rel, &v); err != nil {
			return "", err
		}
		if v.Commit.ID != "" {
			return v.Commit.ID, nil
		}
		return v.Commit.SHA, nil
	}
	tag := func(name string) (string, error) {
		tags, err := p.tags(project)
		if err != nil {
			return "", err
		}
		for _, t := range tags {
			if t.Name == name {
				return t.Commit, nil
			}
		}
		return "", fmt.Errorf("tag %s not found", name)
	}
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		sha, err := branch(strings.TrimPrefix(ref, "refs/heads/"))
		return sha, ref, err
	case strings.HasPrefix(ref, "refs/tags/"):
		sha, err := tag(strings.TrimPrefix(ref, "refs/tags/"))
		return sha, ref, err
	}
	if sha, err := branch(ref); err == nil && sha != "" {
		return sha, "refs/heads/" + ref, nil
	}
	if sha, err := tag(ref); err == nil && sha != "" {
		return sha, "refs/tags/" + ref, nil
	}
	return "", "", fmt.Errorf("%s is neither a branch nor a tag of %s", ref, project)
}

type mdGitEntry struct {
	Blob string `json:"b"`
	Mode string `json:"m"`
}

// mdGitTreeModes lists a commit's files with their modes (symlinks are mode 120000);
// trees are immutable, so the listing is cached by commit.
func mdGitTreeModes(p *gitProvider, sourceID int, project, commit string) (map[string]mdGitEntry, error) {
	key := "medium-modes:" + project
	var data string
	if db.QueryRow(`SELECT data FROM git_trees WHERE source_id=? AND project=? AND commit_id=?`, sourceID, key, commit).Scan(&data) == nil {
		m := map[string]mdGitEntry{}
		if json.Unmarshal([]byte(data), &m) == nil {
			return m, nil
		}
	}
	out := map[string]mdGitEntry{}
	type item struct {
		Path string `json:"path"`
		Type string `json:"type"`
		Mode string `json:"mode"`
		SHA  string `json:"sha"`
		ID   string `json:"id"`
	}
	add := func(list []item) error {
		for _, e := range list {
			if e.Type != "blob" {
				continue
			}
			id := e.SHA
			if id == "" {
				id = e.ID
			}
			out[e.Path] = mdGitEntry{Blob: id, Mode: e.Mode}
		}
		if len(out) > gitMaxTreeFiles {
			return fmt.Errorf("%s: more than %d files", project, gitMaxTreeFiles)
		}
		return nil
	}
	switch {
	case p.kind == "github":
		var v struct {
			Tree      []item `json:"tree"`
			Truncated bool   `json:"truncated"`
		}
		if _, err := p.getJSON(p.gh(project)+"/git/trees/"+url.PathEscape(commit)+"?recursive=1", &v); err != nil {
			return nil, err
		}
		if v.Truncated {
			return nil, fmt.Errorf("%s: the repository tree is too large for one listing", project)
		}
		if err := add(v.Tree); err != nil {
			return nil, err
		}
	case p.kind == "gitea":
		for page := 1; page <= 1000; page++ {
			var v struct {
				Tree      []item `json:"tree"`
				Truncated bool   `json:"truncated"`
			}
			if _, err := p.getJSON(p.gh(project)+"/git/trees/"+url.PathEscape(commit)+"?recursive=true&per_page=1000&page="+strconv.Itoa(page), &v); err != nil {
				return nil, err
			}
			if err := add(v.Tree); err != nil {
				return nil, err
			}
			if !v.Truncated || len(v.Tree) == 0 {
				break
			}
		}
	default:
		err := p.pages(p.gl(project)+"/repository/tree?recursive=true&ref="+url.QueryEscape(commit), 1000, func(raw json.RawMessage) (int, error) {
			var list []item
			if err := json.Unmarshal(raw, &list); err != nil {
				return 0, err
			}
			return len(list), add(list)
		})
		if err != nil {
			return nil, err
		}
	}
	db.Exec(`INSERT OR REPLACE INTO git_trees (source_id, project, commit_id, data, fetched_at) VALUES (?,?,?,?,?)`,
		sourceID, key, commit, string(jsonMarshal(out)), nowStamp())
	return out, nil
}

// mdSafeTreePath: a relative path with no empty, "." or ".." components.
func mdSafeTreePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") {
		return false
	}
	for _, c := range strings.Split(p, "/") {
		if c == "" || c == "." || c == ".." {
			return false
		}
	}
	return true
}

// mdGitSourceTree prepares the source of a project at a commit from a WRM Git source:
// the files below source_dir with their SHA-256 (and link targets), without a build.
func mdGitSourceTree(m mdSourceMatch, proj *mdProject, commit string) (mdTree, error) {
	p, err := newGitProvider(m.src)
	if err != nil {
		return nil, err
	}
	files, err := mdGitTreeModes(p, m.src.ID, m.project, commit)
	if err != nil {
		return nil, err
	}
	prefix := strings.Trim(mdNorm(proj.SourceDir), "/")
	if prefix == "." {
		prefix = ""
	}
	type job struct{ rel, blob, mode string }
	var jobs []job
	for p, e := range files {
		rel := p
		if prefix != "" {
			if !strings.HasPrefix(p, prefix+"/") {
				continue
			}
			rel = strings.TrimPrefix(p, prefix+"/")
		}
		if !mdSafeTreePath(rel) {
			return nil, fmt.Errorf("the repository has an unsafe path %q (absolute, empty or .. components); it cannot be deployed", rel)
		}
		jobs = append(jobs, job{rel, e.Blob, e.Mode})
	}
	if prefix != "" && len(jobs) == 0 {
		return nil, fmt.Errorf("source_dir %s does not exist in the commit", proj.SourceDir)
	}
	tree := mdTree{}
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	sem := make(chan struct{}, gitFetchParallel)
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			content, ok := blobContent(m.src.ID, j.blob)
			if !ok {
				data, err := p.blob(m.project, j.blob)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
				storeBlob(m.src.ID, j.blob, data)
				content = data
			}
			e := &mdEntry{Type: "file", Size: int64(len(content)), exec: j.mode == "100755"}
			if j.mode == "120000" {
				e = &mdEntry{Type: "symlink", Link: string(content)}
			} else {
				sum := sha256.Sum256(content)
				e.Hash = hex.EncodeToString(sum[:])
				srcID, blob, project := m.src.ID, j.blob, m.project
				e.open = func() ([]byte, error) {
					if b, ok := blobContent(srcID, blob); ok {
						return b, nil
					}
					b, err := p.blob(project, blob)
					if err == nil {
						storeBlob(srcID, blob, b)
					}
					return b, err
				}
			}
			mu.Lock()
			tree[j.rel] = e
			mu.Unlock()
		}(j)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return mdWithDirs(tree), nil
}

// ─── server → WRM connection ───

type mdConnChoice struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
	User string `json:"user"`
}

type mdLinkView struct {
	ConnID   int    `json:"conn_id"`
	ConnName string `json:"conn_name"`
	Auto     bool   `json:"auto"`
}

func mdUserConnections(uid int) []mdConnChoice {
	out := []mdConnChoice{}
	rows, err := db.Query(`SELECT id, name, protocol, host, username FROM connections WHERE user_id=? ORDER BY name COLLATE NOCASE, id`, uid)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var c Connection
		if rows.Scan(&c.ID, &c.Name, &c.Protocol, &c.Host, &c.Username) != nil || !isSSHProtocol(c) {
			continue
		}
		out = append(out, mdConnChoice{ID: c.ID, Name: c.Name, Host: ensurePort(c.Host, "SSH"), User: c.Username})
	}
	return out
}

// mdLinks maps every server of servers.yaml to a WRM connection: the one the user chose,
// or else the first connection with the same host, port and user.
func mdLinks(uid int, cfg *mdConfig, conns []mdConnChoice) map[string]mdLinkView {
	chosen := map[string]int{}
	if rows, err := db.Query(`SELECT server, conn_id FROM medium_server_links WHERE user_id=?`, uid); err == nil {
		for rows.Next() {
			var s string
			var id int
			if rows.Scan(&s, &id) == nil {
				chosen[s] = id
			}
		}
		rows.Close()
	}
	byID := map[int]mdConnChoice{}
	for _, c := range conns {
		byID[c.ID] = c
	}
	out := map[string]mdLinkView{}
	for _, s := range cfg.Servers {
		if id, ok := chosen[s.Name]; ok {
			if c, ok := byID[id]; ok {
				out[s.Name] = mdLinkView{ConnID: c.ID, ConnName: c.Name}
				continue
			}
		}
		want := strings.ToLower(s.Host) + ":" + strconv.Itoa(s.Port)
		for _, c := range conns {
			if strings.ToLower(c.Host) == want && c.User == s.User {
				out[s.Name] = mdLinkView{ConnID: c.ID, ConnName: c.Name, Auto: true}
				break
			}
		}
	}
	return out
}

// ─── the original tool's comparison (through the Medium wrapper) ───

type mdRefServer struct {
	Server   string                 `json:"server"`
	Path     string                 `json:"path"`
	Error    *string                `json:"error"`
	Warnings []string               `json:"warnings"`
	Excludes []string               `json:"excludes"`
	Changes  []mdChange             `json:"changes"`
	Current  map[string]interface{} `json:"current"`
}

type mdRefSource struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	Hash string `json:"sha256"`
	Link string `json:"link"`
}

type mdRefResult struct {
	Error      string        `json:"error"`
	Ref        string        `json:"ref"`
	Commit     string        `json:"commit"`
	Version    string        `json:"version"`
	Build      *string       `json:"build"`
	Warnings   []string      `json:"warnings"`
	Confirm    bool          `json:"confirm"`
	RefAllowed bool          `json:"ref_allowed"`
	Excludes   []string      `json:"excludes"`
	Servers    []mdRefServer `json:"servers"`
	Source     []mdRefSource `json:"source"`
}

func mdMediumProcess(uid int) *mediumProcess {
	mediumMgr.mu.Lock()
	p := mediumMgr.processes[mediumUserDir(uid)]
	mediumMgr.mu.Unlock()
	if p == nil {
		return nil
	}
	select {
	case <-p.done:
		return nil
	default:
	}
	return p
}

// mdReferencePlan asks the running original tool for its comparison and source listing.
var mdReferencePlan = func(uid int, req mdPlanRequest) (*mdRefResult, error) {
	p := mdMediumProcess(uid)
	if p == nil {
		return nil, fmt.Errorf("start Medium (the original tool) to compare with it")
	}
	body, _ := json.Marshal(map[string]interface{}{"project": req.Project, "environment": req.Environment, "ref": req.Ref, "servers": req.Servers})
	hr, _ := http.NewRequest("POST", p.base+"/__wrm_plan", bytes.NewReader(body))
	hr.Header.Set("X-WRM-Medium-Key", p.secret)
	hr.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: mediumTransport, Timeout: 40 * time.Minute}
	resp, err := client.Do(hr)
	if err != nil {
		return nil, fmt.Errorf("the original tool did not answer: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("restart Medium so that it runs with this WRM version's wrapper")
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("the original tool answered %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out mdRefResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("cannot read the original tool's answer: %v", err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("%s", out.Error)
	}
	return &out, nil
}

// ─── runs ───

type mdPlanRequest struct {
	Project     string   `json:"project"`
	Environment string   `json:"environment"`
	Ref         string   `json:"ref"`
	Servers     []string `json:"servers"`
	Compare     bool     `json:"compare"`
	Excluded    []string `json:"excluded"` // paths left out of this deployment (as the change list shows them)
}

type mdServerPlan struct {
	Server     string         `json:"server"`
	Path       string         `json:"path"`
	Label      string         `json:"label"`
	Connection string         `json:"connection,omitempty"`
	Error      string         `json:"error,omitempty"`
	Warnings   []string       `json:"warnings"`
	Excludes   []string       `json:"excludes"`
	PostDeploy *string        `json:"post_deploy"`
	Exists     bool           `json:"exists"`
	Changes    []mdChange     `json:"changes"`
	Counts     map[string]int `json:"counts"`
	State      *mdState       `json:"state,omitempty"`
	Compare    *mdCompare     `json:"compare,omitempty"`
	RefError   string         `json:"reference_error,omitempty"`
	RefCount   int            `json:"reference_count"`
	// ExcludedFor: the plan's excluded paths that leave a change out on this server
	// (current.json keeps them, and the next plan offers them excluded again)
	ExcludedFor []string `json:"excluded_for"`
	Platform    string   `json:"platform,omitempty"`
	Arch        string   `json:"arch,omitempty"`
}

type mdSourceCheck struct {
	Same    bool     `json:"same"`
	Missing []string `json:"missing"` // in the original's export, not in the Git source
	Extra   []string `json:"extra"`   // in the Git source, not in the export
	Differs []string `json:"differs"`
	Error   string   `json:"error,omitempty"`
}

type mdPlanResult struct {
	Project     string         `json:"project"`
	Environment string         `json:"environment"`
	Ref         string         `json:"ref"`
	Commit      string         `json:"commit"`
	Version     string         `json:"version,omitempty"`
	Build       *string        `json:"build"`
	SourceFrom  string         `json:"source_from"` // git_source | original
	SourceName  string         `json:"source_name,omitempty"`
	Files       int            `json:"files"`
	Confirm     bool           `json:"confirm"`
	RefAllowed  bool           `json:"ref_allowed"`
	Warnings    []string       `json:"warnings"`
	Compared    bool           `json:"compared"`
	Same        bool           `json:"same"`
	SourceCheck *mdSourceCheck `json:"source_check,omitempty"`
	Servers     []mdServerPlan `json:"servers"`
	NotCompared []string       `json:"not_compared"`
	Excluded    []string       `json:"excluded"`
	Revision    string         `json:"revision,omitempty"`
	Deployable  bool           `json:"deployable"` // a native deployment of this plan is possible
	// BuildLocation shows where a project's build ran: host, server, none, original (the
	// comparison with the original tool), or empty for a project without a build.
	BuildLocation string `json:"build_location,omitempty"`
	BuildServer   string `json:"build_server,omitempty"`
	BuildUser     string `json:"build_user,omitempty"`
	BuildPlatform string `json:"build_platform,omitempty"`
	BuildArch     string `json:"build_arch,omitempty"`
	BuildLog      string `json:"build_log,omitempty"`
}

type mdRun struct {
	ID        string        `json:"id"`
	Done      bool          `json:"done"`
	Stage     string        `json:"stage"`
	Error     string        `json:"error,omitempty"`
	Started   string        `json:"started"`
	Request   mdPlanRequest `json:"request"`
	Result    *mdPlanResult `json:"result,omitempty"`
	uid       int
	at        time.Time
	src       mdTree // the prepared source (with the files' content), for a deployment
	used      bool   // a deployment of this plan has started
	cleanup   func() // removes a build's temporary files when the run is discarded
	deploying bool   // a deployment job still needs src
}

var mdRuns = struct {
	sync.Mutex
	m      map[string]*mdRun
	active map[int]bool
}{m: map[string]*mdRun{}, active: map[int]bool{}}

func (r *mdRun) set(fn func()) {
	mdRuns.Lock()
	fn()
	mdRuns.Unlock()
}

func mdStartRun(uid int, req mdPlanRequest) (*mdRun, error) {
	mdRuns.Lock()
	defer mdRuns.Unlock()
	if mdRuns.active[uid] {
		return nil, fmt.Errorf("a native preview is already running")
	}
	for id, r := range mdRuns.m {
		if r.Done && !r.deploying && time.Since(r.at) > time.Hour {
			if r.cleanup != nil {
				r.cleanup()
			}
			delete(mdRuns.m, id)
		}
	}
	b := make([]byte, 8)
	rand.Read(b)
	run := &mdRun{ID: hex.EncodeToString(b), Stage: "configuration", Started: nowStamp(), Request: req, uid: uid, at: time.Now()}
	mdRuns.m[run.ID] = run
	mdRuns.active[uid] = true
	go func() {
		defer func() {
			if p := recover(); p != nil {
				run.set(func() { run.Error = fmt.Sprint("internal error: ", p) })
			}
			mdRuns.Lock()
			run.Done, run.Stage, run.at = true, "", time.Now()
			delete(mdRuns.active, uid)
			mdRuns.Unlock()
		}()
		res, src, cleanup, err := mdExecute(uid, req, func(stage string) { run.set(func() { run.Stage = stage }) })
		run.set(func() {
			run.Result, run.src, run.cleanup = res, src, cleanup
			if err != nil {
				run.Error = err.Error()
			}
		})
		// the activity log: what was previewed, when, and the outcome
		op, status, msg := "preview", "ok", ""
		if req.Compare {
			op = "compare"
		}
		if err != nil {
			status, msg = "error", err.Error()
		}
		mediumRecord(uid, op, status, time.Since(run.at).Milliseconds(), mdRunSummary(req, res), msg, "")
	}()
	return run, nil
}

// mdRunSummary: project, environment, ref and per-server change counts for the log.
func mdRunSummary(req mdPlanRequest, res *mdPlanResult) string {
	s := req.Project + " / " + req.Environment + " @ " + req.Ref
	if res == nil {
		return s
	}
	for _, sp := range res.Servers {
		if sp.Error != "" {
			s += fmt.Sprintf("; %s: error", sp.Server)
			continue
		}
		s += fmt.Sprintf("; %s: +%d ~%d -%d", sp.Server, sp.Counts["new"], sp.Counts["changed"], sp.Counts["deleted"])
	}
	return s
}

func mdGetRun(uid int, id string) (mdRun, bool) {
	mdRuns.Lock()
	defer mdRuns.Unlock()
	r, ok := mdRuns.m[id]
	if !ok || r.uid != uid {
		return mdRun{}, false
	}
	return *r, true
}

// mdOpenDestination connects to the server's linked WRM connection and runs fn on it.
var mdOpenDestination = func(uid int, cfg *mdConfig, server string, fn func(mdRemote, string) error) error {
	return mdOpenHost(uid, cfg, server, func(h mdHost, name string) error { return fn(h, name) })
}

func mdExecute(uid int, req mdPlanRequest, stage func(string)) (*mdPlanResult, mdTree, func(), error) {
	cfg, err := mdLoadConfig(mdConfigDir(uid))
	if err != nil {
		return nil, nil, nil, err
	}
	proj := cfg.project(req.Project)
	if proj == nil {
		return nil, nil, nil, fmt.Errorf("unknown project %s", req.Project)
	}
	target := proj.target(req.Environment)
	if target == nil {
		return nil, nil, nil, fmt.Errorf("project %s has no environment %s", proj.Name, req.Environment)
	}
	excluded, err := mdCleanExcluded(req.Excluded)
	if err != nil {
		return nil, nil, nil, err
	}
	var dests []*mdDestination
	if len(req.Servers) == 0 {
		dests = target.Destinations
	} else {
		want := map[string]bool{}
		for _, s := range req.Servers {
			if target.destination(s) == nil {
				return nil, nil, nil, fmt.Errorf("environment %s has no destination on %s", target.Name, s)
			}
			want[s] = true
		}
		for _, d := range target.Destinations {
			if want[d.Server] {
				dests = append(dests, d)
			}
		}
	}
	res := &mdPlanResult{Project: proj.Name, Environment: target.Name, Build: proj.Build, Confirm: target.Confirm, Warnings: []string{}, NotCompared: []string{}, Compared: req.Compare, Excluded: excluded}
	for _, d := range target.Destinations {
		in := false
		for _, x := range dests {
			in = in || x == d
		}
		if !in {
			res.NotCompared = append(res.NotCompared, d.label())
		}
	}
	var src mdTree
	var ref *mdRefResult
	var cleanup func()
	match, hasSource := mdFindSource(uid, proj.Repo)
	if req.Compare {
		stage("original")
		if ref, err = mdReferencePlan(uid, req); err != nil {
			return nil, nil, nil, err
		}
		res.Ref, res.Commit, res.Version, res.SourceFrom = ref.Ref, ref.Commit, ref.Version, "original"
		if proj.Build != nil {
			res.BuildLocation = "original"
		}
		res.Warnings = append(res.Warnings, ref.Warnings...)
		src = mdTree{}
		for _, e := range ref.Source {
			src[mdUnescapePath(e.Path)] = &mdEntry{Type: e.Type, Size: e.Size, Hash: e.Hash, Link: e.Link}
		}
		src = mdWithDirs(src)
		if ref.Confirm != target.Confirm {
			res.Warnings = append(res.Warnings, fmt.Sprintf("confirm differs: the original tool says %v, WRM reads %v", ref.Confirm, target.Confirm))
		}
		if hasSource && proj.Build == nil {
			stage("source")
			res.SourceCheck = mdCheckSource(match, proj, ref.Commit, src)
			res.SourceName = match.src.Name
		}
	} else {
		if !hasSource {
			return nil, nil, nil, fmt.Errorf("no WRM Git source serves %s; add the Git server under Git → Sources, or compare with the original tool", redactURLText(proj.Repo))
		}
		stage("source")
		p, err := newGitProvider(match.src)
		if err != nil {
			return nil, nil, nil, err
		}
		if res.Commit, res.Ref, err = mdResolveRef(p, match.project, req.Ref); err != nil {
			return nil, nil, nil, err
		}
		out, err := mdBuildProject(uid, match, proj, res.Commit, mdBuildOf(uid, proj.Name), stage)
		if err != nil {
			return nil, nil, nil, err
		}
		cleanup = out.cleanup
		src = out.Tree
		res.BuildLocation, res.BuildServer, res.BuildUser, res.BuildPlatform, res.BuildArch, res.BuildLog = out.Location, out.Server, out.User, out.Platform, out.Arch, out.Log
		res.SourceFrom, res.SourceName = "git_source", match.src.Name
	}
	res.RefAllowed = target.AllowedRefs == nil || mdRefAllowed(res.Ref, target.AllowedRefs)
	if !res.RefAllowed {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s is not allowed for %s (allowed: %s)", res.Ref, target.Name, strings.Join(target.AllowedRefs, ", ")))
	}
	if ref != nil && ref.RefAllowed != res.RefAllowed {
		res.Warnings = append(res.Warnings, fmt.Sprintf("allowed_refs: the original tool says %v, WRM reads %v", ref.RefAllowed, res.RefAllowed))
	}
	for _, e := range src {
		if e.Type == "file" {
			res.Files++
		}
	}

	stage("servers")
	res.Servers = make([]mdServerPlan, len(dests))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, d := range dests {
		wg.Add(1)
		go func(i int, d *mdDestination) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res.Servers[i] = mdPlanServer(uid, cfg, proj, target, d, src, ref, excluded, res.BuildLocation == "host" || res.BuildLocation == "server")
		}(i, d)
	}
	wg.Wait()
	if req.Compare {
		res.Same = res.SourceCheck == nil || res.SourceCheck.Same
		for _, s := range res.Servers {
			res.Same = res.Same && s.Compare != nil && s.Compare.Same && s.Error == "" && s.RefError == ""
		}
	}
	mdCheckBuildPlatforms(res, res.BuildLocation, res.BuildPlatform, res.BuildArch)
	res.Deployable = res.SourceFrom == "git_source" && len(res.Servers) > 0
	for _, s := range res.Servers {
		res.Deployable = res.Deployable && s.Error == ""
	}
	res.Revision = mdPlanRevision(res)
	return res, src, cleanup, nil
}

func mdPlanServer(uid int, cfg *mdConfig, proj *mdProject, target *mdTarget, d *mdDestination, src mdTree, ref *mdRefResult, excluded []string, wantPlatform bool) mdServerPlan {
	sp := mdServerPlan{Server: d.Server, Path: d.appDir(), Label: d.label(), Warnings: []string{}, Changes: []mdChange{}, PostDeploy: d.PostDeploy,
		Excludes: proj.ignoreFor(target.Name, d.Server), Counts: map[string]int{"new": 0, "changed": 0, "deleted": 0, "eol": 0}, ExcludedFor: []string{}}
	err := mdOpenDestination(uid, cfg, d.Server, func(r mdRemote, connName string) error {
		sp.Connection = connName
		if wantPlatform {
			if h, ok := r.(mdHost); ok {
				if platform, arch, err := mdHostPlatform(h); err == nil {
					sp.Platform, sp.Arch = platform, arch
				} else {
					sp.Warnings = append(sp.Warnings, "platform: "+err.Error())
				}
			}
		}
		view, err := mdPlanOn(r, d.appDir(), sp.Excludes, excluded, src)
		if err != nil {
			return err
		}
		sp.Exists, sp.Changes = view.exists, view.changes
		if sp.Changes == nil {
			sp.Changes = []mdChange{}
		}
		if _, err := mdMarkEOL(r, d.appDir(), src, sp.Changes); err != nil {
			sp.Warnings = append(sp.Warnings, "line endings: "+err.Error())
		}
		for _, e := range view.excludedFor {
			sp.ExcludedFor = append(sp.ExcludedFor, mdEscapePath(e))
		}
		for p := range view.keep {
			if _, inSource := view.src[p]; !inSource && view.dst[p] != nil {
				sp.Warnings = append(sp.Warnings, fmt.Sprintf("%s/ is not in the source but stays on the server because it holds excluded items; everything else in it would be deleted", mdEscapePath(p)))
			}
		}
		sort.Strings(sp.Warnings)
		st, err := mdReadState(r, d)
		if err != nil {
			sp.Warnings = append(sp.Warnings, "state: "+err.Error())
		} else {
			sp.State = st
		}
		return nil
	})
	if err != nil {
		sp.Error = err.Error()
	}
	for i := range sp.Changes {
		sp.Changes[i].Path = mdEscapePath(sp.Changes[i].Path)
		sp.Counts[sp.Changes[i].Action]++
		if sp.Changes[i].EOL {
			sp.Counts["eol"]++
		}
	}
	if ref != nil {
		var rs *mdRefServer
		for i := range ref.Servers {
			if ref.Servers[i].Server == d.Server {
				rs = &ref.Servers[i]
			}
		}
		switch {
		case rs == nil:
			sp.RefError = "the original tool did not compare this server"
		case rs.Error != nil && *rs.Error != "":
			sp.RefError = *rs.Error
		default:
			sp.RefCount = len(rs.Changes)
			if sp.Error == "" {
				sp.Compare = mdCompareChanges(rs.Changes, sp.Changes)
			}
			if strings.Join(rs.Excludes, "\x00") != strings.Join(sp.Excludes, "\x00") {
				sp.Warnings = append(sp.Warnings, "exclude patterns differ from the original tool's: "+strings.Join(rs.Excludes, " "))
			}
			if sp.State != nil {
				a, _ := rs.Current["deploy_id"].(string)
				b, _ := sp.State.Current["deploy_id"].(string)
				if a != b {
					sp.Warnings = append(sp.Warnings, fmt.Sprintf("current.json differs: the original tool read %q, WRM read %q", a, b))
				}
			}
		}
	}
	return sp
}

// mdCheckSource compares the WRM Git source's files with the original tool's export.
func mdCheckSource(m mdSourceMatch, proj *mdProject, commit string, export mdTree) *mdSourceCheck {
	c := &mdSourceCheck{Missing: []string{}, Extra: []string{}, Differs: []string{}}
	tree, err := mdGitSourceTree(m, proj, commit)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	for p, e := range export {
		g := tree[p]
		switch {
		case g == nil:
			c.Missing = append(c.Missing, mdEscapePath(p))
		case g.Type != e.Type || g.Hash != e.Hash || g.Link != e.Link:
			c.Differs = append(c.Differs, mdEscapePath(p))
		}
	}
	for p := range tree {
		if export[p] == nil {
			c.Extra = append(c.Extra, mdEscapePath(p))
		}
	}
	sort.Strings(c.Missing)
	sort.Strings(c.Extra)
	sort.Strings(c.Differs)
	c.Same = len(c.Missing)+len(c.Extra)+len(c.Differs) == 0
	return c
}

// redactURLText hides a user name and token in a repository address.
func redactURLText(s string) string {
	if u, err := url.Parse(s); err == nil && u.User != nil {
		u.User = url.User("***")
		return u.String()
	}
	return s
}

// ─── API: /api/git/medium/native/… ───

type mdProjectView struct {
	Name          string   `json:"name"`
	Repo          string   `json:"repo"`
	Build         bool     `json:"build"`
	Source        string   `json:"source,omitempty"`
	Environments  []string `json:"environments"`
	BuildLocation string   `json:"build_location,omitempty"`
	BuildConnID   int      `json:"build_conn_id,omitempty"`
	BuildConnName string   `json:"build_conn_name,omitempty"`
}

func apiMediumNative(w http.ResponseWriter, r *http.Request, uid int, action string) {
	switch {
	case action == "/config" && r.Method == "GET":
		out := map[string]interface{}{"running": mdMediumProcess(uid) != nil}
		cfg, err := mdLoadConfig(mdConfigDir(uid))
		if err != nil {
			out["ok"], out["error"] = false, err.Error()
			jsonOK(w, out)
			return
		}
		conns := mdUserConnections(uid)
		names := map[int]string{}
		for _, c := range conns {
			names[c.ID] = c.Name
		}
		builds := mdBuildSettings(uid)
		projects := []mdProjectView{}
		for _, p := range cfg.Projects {
			v := mdProjectView{Name: p.Name, Repo: redactURLText(p.Repo), Build: p.Build != nil, Environments: []string{}}
			if m, ok := mdFindSource(uid, p.Repo); ok {
				v.Source = m.src.Name
			}
			if b, ok := builds[p.Name]; ok {
				v.BuildLocation, v.BuildConnID, v.BuildConnName = b.Location, b.ConnID, names[b.ConnID]
				if b.Location == "server" && v.BuildConnName == "" {
					// The saved connection was deleted or is no longer usable; make the
					// user choose one again instead of offering a preview that will fail.
					v.BuildConnID = 0
				}
			}
			for _, e := range cfg.Envs {
				if p.target(e.Name) != nil {
					v.Environments = append(v.Environments, e.Name)
				}
			}
			projects = append(projects, v)
			p.Repo = redactURLText(p.Repo)
		}
		out["ok"], out["config"], out["projects"] = true, cfg, projects
		out["links"], out["connections"] = mdLinks(uid, cfg, conns), conns
		out["engines"], out["engine_default"], out["can_deploy"], out["step_up"] = mdProjectEngines(uid), mdDefaultEngine(uid), mdNativeAllowed(uid), settingBool("require_step_up")
		out["host_build"] = mdHostBuildInfo()
		jsonOK(w, out)
	case action == "/build" && r.Method == "POST":
		if !mdNativeAllowed(uid) {
			jsonError(w, "Native deployments are not allowed for you by policy (medium_native)", 403)
			return
		}
		var in struct {
			Project  string `json:"project"`
			Location string `json:"location"`
			ConnID   int    `json:"conn_id"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in) != nil || !mdNameRe.MatchString(in.Project) {
			jsonError(w, "invalid request", 400)
			return
		}
		cfg, err := mdLoadConfig(mdConfigDir(uid))
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if cfg.project(in.Project) == nil {
			jsonError(w, "unknown project", 404)
			return
		}
		b, err := mdSetBuild(uid, in.Project, in.Location, in.ConnID)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		auditLog(r, uid, "git.medium_build_set", in.Project, map[string]interface{}{"location": b.Location, "conn_id": b.ConnID})
		jsonOK(w, map[string]interface{}{"ok": true, "build": b})
	case action == "/link" && r.Method == "POST":
		var in struct {
			Server string `json:"server"`
			ConnID int    `json:"conn_id"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in) != nil || !mdNameRe.MatchString(in.Server) {
			jsonError(w, "invalid request", 400)
			return
		}
		if in.ConnID == 0 {
			db.Exec(`DELETE FROM medium_server_links WHERE user_id=? AND server=?`, uid, in.Server)
			jsonOK(w, map[string]bool{"ok": true})
			return
		}
		conn, err := loadConnectionRaw(in.ConnID)
		if err != nil || conn.UserID != uid || !isSSHProtocol(conn) {
			jsonError(w, "connection not found", 404)
			return
		}
		db.Exec(`INSERT OR REPLACE INTO medium_server_links (user_id, server, conn_id) VALUES (?,?,?)`, uid, in.Server, in.ConnID)
		auditLog(r, uid, "git.medium_server_linked", in.Server, map[string]interface{}{"conn_id": in.ConnID})
		jsonOK(w, map[string]bool{"ok": true})
	case action == "/refs" && r.Method == "GET":
		cfg, err := mdLoadConfig(mdConfigDir(uid))
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		proj := cfg.project(r.URL.Query().Get("project"))
		if proj == nil {
			jsonError(w, "unknown project", 404)
			return
		}
		out := map[string]interface{}{"branches": []string{}, "tags": []string{}}
		m, ok := mdFindSource(uid, proj.Repo)
		if !ok {
			jsonOK(w, out)
			return
		}
		p, err := newGitProvider(m.src)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if b, err := p.branches(m.project); err == nil {
			sort.Strings(b)
			out["branches"] = b
		} else {
			out["error"] = err.Error()
		}
		if t, err := p.tags(m.project); err == nil {
			names := []string{}
			for _, x := range t {
				names = append(names, x.Name)
			}
			sortVersionsDesc(names)
			out["tags"] = names
		}
		jsonOK(w, out)
	case action == "/plan" && r.Method == "POST":
		var in mdPlanRequest
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil || in.Project == "" || in.Environment == "" || strings.TrimSpace(in.Ref) == "" || strings.HasPrefix(strings.TrimSpace(in.Ref), "-") {
			jsonError(w, "choose a project, an environment and a branch or tag", 400)
			return
		}
		in.Ref = strings.TrimSpace(in.Ref)
		run, err := mdStartRun(uid, in)
		if err != nil {
			jsonError(w, err.Error(), 409)
			return
		}
		auditLog(r, uid, "git.medium_native_preview", in.Project, map[string]interface{}{"environment": in.Environment, "ref": in.Ref, "compare": in.Compare, "servers": in.Servers})
		jsonOK(w, map[string]string{"id": run.ID})
	case strings.HasPrefix(action, "/plan/") && strings.HasSuffix(action, "/diff") && r.Method == "GET":
		mdAPIDiff(w, r, uid, strings.TrimSuffix(strings.TrimPrefix(action, "/plan/"), "/diff"))
	case action == "/config/git" || strings.HasPrefix(action, "/config/git/"):
		mdAPIConfigGit(w, r, uid, action)
	case action == "/config/files" || action == "/config/file":
		mdAPIConfigFiles(w, r, uid, action)
	case strings.HasPrefix(action, "/plan/") && r.Method == "GET":
		run, ok := mdGetRun(uid, strings.TrimPrefix(action, "/plan/"))
		if !ok {
			jsonError(w, "not found", 404)
			return
		}
		jsonOK(w, run)
	default:
		if !mdNativeAPI(w, r, uid, action) {
			jsonError(w, "not found", 404)
		}
	}
}

// mdConfigDir is the deploy-config the WRM engine reads: WRM's Git copy, else a kept ZIP (already checked by WRM's
// reader, waiting for the original tool) is the user's newest one, else the applied one.
func mdConfigDir(uid int) string {
	// WRM's own copy pulled from the configuration repository wins
	if _, ok := mdLoadGitConfigState(uid); ok {
		return mdGitConfigDir(uid)
	}
	for _, d := range []string{"pending-config", "pending-config.applying"} {
		if p := filepath.Join(mediumUserDir(uid), d); mediumFileExists(filepath.Join(p, "servers.yaml")) {
			return p
		}
	}
	return mediumConfigDir(uid)
}
