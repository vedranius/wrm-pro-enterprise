// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

// ─── GIT: .gitignore HELPER ──────────────────────────
//
// Per service it gathers candidates for the repository's .gitignore: files that exist only on
// servers (state extra, from the latest checks), standard patterns of the stacks found in the
// repository tree and the catalog's protected globs. It warns about per-host or secret-looking
// files that are committed, previews the result as a diff against the current .gitignore and
// hands it out as a download. A merge request (GitLab) / pull request (GitHub) is created only
// when the user explicitly asks for it and confirms the repository by name; it needs the
// source's optional write token (policy git_gitignore_mr, audit git.gitignore_mr).

const (
	gitIgnoreMaxPatterns = 500
	gitIgnoreMaxCurrent  = 256 << 10
)

type gitIgnoreCandidate struct {
	Path    string   `json:"path"`
	Size    int64    `json:"size"`
	Servers []string `json:"servers"`
	Pattern string   `json:"pattern"`
}

type gitIgnoreStack struct {
	Name     string   `json:"name"`
	Detected bool     `json:"detected"`
	Reason   string   `json:"reason,omitempty"` // the file it was detected by
	Patterns []string `json:"patterns"`
}

type gitIgnoreWarning struct {
	Path     string `json:"path"`
	Reason   string `json:"reason"`             // secret | protected | log
	Template string `json:"template,omitempty"` // suggested name of the template to commit instead
}

type gitIgnoreInfo struct {
	App        string               `json:"app"`
	InCatalog  bool                 `json:"in_catalog"`
	SourceID   int                  `json:"source_id"`
	SourceKind string               `json:"source_kind"`
	Repo       string               `json:"repo"` // host/project, as named in the confirmation
	Project    string               `json:"project"`
	Branch     string               `json:"branch"`
	File       string               `json:"file"` // path of the .gitignore in the repository
	Current    string               `json:"current"`
	HasCurrent bool                 `json:"has_current"`
	Note       string               `json:"note,omitempty"` // why the repository could not be read
	Extra      []gitIgnoreCandidate `json:"extra"`
	Stacks     []gitIgnoreStack     `json:"stacks"`
	Protected  []string             `json:"protected"`
	Exclude    []string             `json:"exclude"`
	Warnings   []gitIgnoreWarning   `json:"warnings"`
	CanMR      bool                 `json:"can_mr"`
	MRNote     string               `json:"mr_note,omitempty"`
	// internal: the state of the repository the info was built from
	blob     string
	head     string
	provider *gitProvider
	src      gitSource
}

// gitIgnoreStacks are the standard patterns per language or stack. markers are file names
// (or *.ext) that reveal the stack in the repository tree.
var gitIgnoreStacks = []struct {
	name     string
	markers  []string
	patterns []string
}{
	{"Python", []string{"requirements.txt", "pyproject.toml", "setup.py", "setup.cfg", "Pipfile", "*.py"},
		[]string{"__pycache__/", "*.py[cod]", ".venv/", "venv/", "*.egg-info/", ".pytest_cache/", ".mypy_cache/", "build/", "dist/"}},
	{"Node", []string{"package.json", "*.js", "*.ts", "*.mjs"},
		[]string{"node_modules/", "npm-debug.log*", "yarn-error.log*", ".npm/", "coverage/", "dist/"}},
	{"Go", []string{"go.mod", "*.go"},
		[]string{"*.exe", "*.test", "*.out", "/bin/"}},
	{"Java", []string{"pom.xml", "build.gradle", "build.gradle.kts", "*.java"},
		[]string{"target/", "*.class", "*.jar", ".gradle/", "build/"}},
	{"Docker", []string{"Dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yaml"},
		[]string{".env", "docker-compose.override.yml"}},
	{"IDE", nil, []string{".idea/", ".vscode/", "*.swp", "*~", ".DS_Store", "Thumbs.db"}},
	// detected by any log file in the tree (isLogPath), whatever its depth
	{"Logs", nil, []string{"log/", "logs/", "*.log", "*.log.*"}},
}

var (
	gitIgnoreDirs = map[string]bool{"logs": true, "log": true, "tmp": true, "temp": true, "cache": true, ".cache": true, "__pycache__": true,
		"venv": true, ".venv": true, "node_modules": true, "output": true, "out": true, "run": true, "pids": true}
	gitIgnoreExts = map[string]bool{".log": true, ".tmp": true, ".pyc": true, ".pyo": true, ".bak": true, ".swp": true, ".pid": true,
		".out": true, ".old": true, ".orig": true, ".sqlite": true, ".sqlite3": true, ".db": true}
	// committed files that look like secrets or per-host configuration
	gitSecretGlobs = []string{".env", "*.env", ".env.*", "*.pem", "*.key", "*.p12", "*.pfx", "*credentials*", "*secret*", "id_rsa*", "id_ed25519*", ".htpasswd", ".netrc", ".pgpass"}
)

// suggestIgnorePattern proposes a pattern for a file found only on servers: a known
// directory (logs/), a known extension (*.log) or the file itself (anchored).
func suggestIgnorePattern(rel string) string {
	parts := strings.Split(rel, "/")
	for i, seg := range parts[:len(parts)-1] {
		if gitIgnoreDirs[strings.ToLower(seg)] {
			return strings.Join(parts[:i+1], "/") + "/"
		}
	}
	base := parts[len(parts)-1]
	if strings.HasSuffix(base, "~") {
		return "*~"
	}
	if ext := strings.ToLower(path.Ext(base)); gitIgnoreExts[ext] {
		return "*" + ext
	}
	return "/" + rel
}

// isTemplateFile: config.ini.example, .env.sample, … are what should be committed.
func isTemplateFile(rel string) bool { return templateDest(rel) != "" }

// gitIgnoreWarnings lists committed files that look like secrets, match the protected globs
// or are log files.
func gitIgnoreWarnings(rels []string, protected []string) []gitIgnoreWarning {
	out := []gitIgnoreWarning{}
	logs := 0
	for _, rel := range rels {
		if isTemplateFile(rel) || path.Base(rel) == ".gitignore" {
			continue
		}
		reason := ""
		switch {
		case globHit(rel, gitSecretGlobs):
			reason = "secret"
		case globHit(rel, protected):
			reason = "protected"
		case isLogPath(rel):
			if logs++; logs <= 20 { // a committed log folder is named by its first files
				out = append(out, gitIgnoreWarning{Path: rel, Reason: "log"})
			}
			continue
		default:
			continue
		}
		base, dir := path.Base(rel), path.Dir(rel)
		tmpl := base + ".example"
		if ext := path.Ext(base); ext != "" && ext != base && !strings.HasPrefix(base, ".") {
			tmpl = strings.TrimSuffix(base, ext) + ".example" + ext // settings.py → settings.example.py
		}
		if dir != "." {
			tmpl = dir + "/" + tmpl
		}
		out = append(out, gitIgnoreWarning{Path: rel, Reason: reason, Template: tmpl})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// detectStacks finds the stacks of a repository tree (relative paths).
func detectStacks(rels []string) []gitIgnoreStack {
	out := []gitIgnoreStack{}
	for _, s := range gitIgnoreStacks {
		st := gitIgnoreStack{Name: s.name, Patterns: s.patterns}
		if s.name == "Logs" {
			for _, rel := range rels {
				if isLogPath(rel) {
					st.Detected, st.Reason = true, rel
					break
				}
			}
		}
		for _, m := range s.markers {
			for _, rel := range rels {
				if strings.Count(rel, "/") > 2 {
					continue
				}
				if fnmatch(path.Base(rel), m) {
					st.Detected, st.Reason = true, rel
					break
				}
			}
			if st.Detected {
				break
			}
		}
		out = append(out, st)
	}
	return out
}

// extraCandidates gathers the files in state extra of the service's installations.
func extraCandidates(userID int, app string) []gitIgnoreCandidate {
	by := map[string]*gitIgnoreCandidate{}
	for _, in := range loadGitInstalls(userID, true, "i.app=?", app) {
		for _, f := range in.Files {
			if f.State != "extra" {
				continue
			}
			c := by[f.Path]
			if c == nil {
				c = &gitIgnoreCandidate{Path: f.Path, Servers: []string{}, Pattern: suggestIgnorePattern(f.Path)}
				by[f.Path] = c
			}
			if f.Size > c.Size {
				c.Size = f.Size
			}
			seen := false
			for _, s := range c.Servers {
				seen = seen || s == in.ConnName
			}
			if !seen {
				c.Servers = append(c.Servers, in.ConnName)
			}
		}
	}
	out := make([]gitIgnoreCandidate, 0, len(by))
	for _, c := range by {
		sort.Strings(c.Servers)
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	if len(out) > 2000 {
		out = out[:2000]
	}
	return out
}

func gitHostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

// gitIgnoreState builds the helper's view of one service.
func gitIgnoreState(userID int, app string) (*gitIgnoreInfo, int, error) {
	cat, st := loadGitWorkspace(userID)
	var a *gitCatalogApp
	for _, x := range effectiveApps(userID, cat) {
		if x.Name == app {
			x := x
			a = &x
		}
	}
	if a == nil {
		return nil, 404, fmt.Errorf("service not found")
	}
	_, inCat := cat.app(app)
	info := &gitIgnoreInfo{App: app, InCatalog: inCat, Project: a.Project, Branch: a.Branch, Extra: extraCandidates(userID, app),
		Protected: nonNil(a.Protected), Exclude: nonNil(a.Exclude), Warnings: []gitIgnoreWarning{}}
	info.File = ".gitignore"
	if a.Subdir != "" {
		info.File = a.Subdir + "/.gitignore"
	}
	t, haveT := loadGitTarget(userID, app)
	if haveT && t.Branch != "" {
		info.Branch = t.Branch
	}
	// the API source: the one the target came from, else the active one
	var api *gitSource
	if haveT {
		if s, ok := loadGitSource(userID, t.SourceID); ok && s.Kind != "bundle" {
			api = &s
		}
	}
	if api == nil {
		api, _ = pickSources(userID, st)
	}
	var rels []string
	if api == nil {
		info.Note = "no GitLab or GitHub source: the current .gitignore cannot be read (the file list comes from the bundle)"
		for rel := range t.Files {
			rels = append(rels, rel)
		}
	} else {
		info.SourceID, info.SourceKind, info.src = api.ID, api.Kind, *api
		info.Repo = gitHostOf(api.URL) + "/" + a.Project
		if err := info.readRepo(*api, a, &rels); err != nil {
			info.Note = err.Error()
			for rel := range t.Files {
				rels = append(rels, rel)
			}
		}
	}
	sort.Strings(rels)
	info.Stacks = detectStacks(rels)
	info.Warnings = gitIgnoreWarnings(rels, a.Protected)
	switch {
	case !gitActionAllowed(userID, "git_gitignore_mr"):
		info.MRNote = "not allowed for your account"
	case api == nil:
		info.MRNote = "needs a GitLab or GitHub source"
	case api.Kind == "gitea":
		info.MRNote = "not available for Gitea sources yet: download the file"
	case !api.HasWriteToken:
		info.MRNote = "the source has no write token (Settings → Git sources)"
	case info.head == "":
		info.MRNote = "the repository could not be read"
	default:
		info.CanMR = true
	}
	return info, 200, nil
}

// readRepo lists the service's part of the repository at the head of its branch and reads
// the current .gitignore (cached by commit and blob like everything else).
func (info *gitIgnoreInfo) readRepo(src gitSource, a *gitCatalogApp, rels *[]string) error {
	p, err := newGitProvider(src)
	if err != nil {
		return err
	}
	info.provider = p
	if info.Branch == "" {
		if info.Branch, err = p.defaultBranch(a.Project); err != nil {
			return err
		}
	}
	commits, err := p.branchCommits(a.Project, info.Branch, 1)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		return fmt.Errorf("branch %s has no commits", info.Branch)
	}
	tree, err := p.tree(src.ID, a.Project, commits[0].SHA)
	if err != nil {
		return err
	}
	prefix := ""
	if a.Subdir != "" {
		prefix = a.Subdir + "/"
	}
	for f := range tree {
		if strings.HasPrefix(f, prefix) {
			*rels = append(*rels, f[len(prefix):])
		}
	}
	if blob, ok := tree[info.File]; ok {
		data, have := blobContent(src.ID, blob)
		if !have {
			if data, err = p.blob(a.Project, blob); err != nil {
				return err
			}
			storeBlob(src.ID, blob, data)
		}
		if len(data) > gitIgnoreMaxCurrent {
			return fmt.Errorf("the current .gitignore is too large")
		}
		info.Current, info.HasCurrent, info.blob = string(data), true, blob
	}
	info.head = commits[0].SHA
	return nil
}

// cleanPatterns validates the chosen patterns (one line each, no comments).
func cleanPatterns(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		if strings.ContainsAny(p, "\n\r\x00") || len(p) > 300 || strings.HasPrefix(p, "#") {
			return nil, fmt.Errorf("invalid pattern %q", truncateStr(p, 60))
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) > gitIgnoreMaxPatterns {
		return nil, fmt.Errorf("at most %d patterns", gitIgnoreMaxPatterns)
	}
	return out, nil
}

// mergeGitignore appends the patterns the current .gitignore does not have yet, under a
// comment line; line endings follow the current file.
func mergeGitignore(current string, patterns []string, now time.Time) (string, []string) {
	have := map[string]bool{}
	for _, l := range strings.Split(strings.ReplaceAll(current, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			have[l] = true
			have[strings.TrimPrefix(l, "/")] = true
		}
	}
	added := []string{}
	for _, p := range patterns {
		if !have[p] && !have[strings.TrimPrefix(p, "/")] {
			have[p] = true
			added = append(added, p)
		}
	}
	if len(added) == 0 {
		return current, added
	}
	nl := "\n"
	if strings.Contains(current, "\r\n") {
		nl = "\r\n"
	}
	var b strings.Builder
	b.WriteString(current)
	if current != "" {
		if !strings.HasSuffix(current, "\n") {
			b.WriteString(nl)
		}
		b.WriteString(nl)
	}
	b.WriteString("# Added with WRM PRO on " + now.Format("2006-01-02") + nl)
	for _, p := range added {
		b.WriteString(p + nl)
	}
	return b.String(), added
}

// ─── merge request / pull request ────────────────────

func newGitWriteProvider(s gitSource) (*gitProvider, error) {
	p, err := newGitProvider(s)
	if err != nil {
		return nil, err
	}
	p.token = decryptValue(s.writeToken)
	if p.token == "" {
		return nil, fmt.Errorf("the source has no write token")
	}
	return p, nil
}

// send makes a write call (JSON body) and decodes the answer into out.
func (p *gitProvider) send(method, rel string, body, out interface{}) error {
	req, err := http.NewRequest(method, p.api+rel, bytes.NewReader(jsonMarshal(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	accept := "application/json"
	if p.kind == "github" {
		accept = "application/vnd.github+json"
	}
	resp, err := p.do(req, accept)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out != nil {
		return jsonDecodeLimited(resp.Body, out)
	}
	return nil
}

type gitMRResult struct {
	URL    string `json:"url"`
	Branch string `json:"branch"`
	Kind   string `json:"kind"` // merge_request | pull_request
}

// createGitignoreMR puts the new .gitignore on a new branch and opens a merge request
// (GitLab) or a pull request (GitHub) against the service's branch.
func createGitignoreMR(info *gitIgnoreInfo, content string, added []string, now time.Time) (gitMRResult, error) {
	if info.src.Kind == "gitea" {
		return gitMRResult{}, fmt.Errorf("pull requests from WRM are not available for Gitea sources yet: download the file instead")
	}
	p, err := newGitWriteProvider(info.src)
	if err != nil {
		return gitMRResult{}, err
	}
	branch := fmt.Sprintf("wrm/gitignore-%s-%s", strings.ToLower(info.App), now.UTC().Format("20060102-150405"))
	title := fmt.Sprintf("Update .gitignore for %s", info.App)
	desc := "Patterns added with the .gitignore helper of WRM PRO:\n\n"
	for _, a := range added {
		desc += "- `" + a + "`\n"
	}
	if p.kind == "gitlab" {
		pr := p.gl(info.Project)
		if err := p.send("POST", pr+"/repository/branches", map[string]string{"branch": branch, "ref": info.head}, nil); err != nil {
			return gitMRResult{}, fmt.Errorf("creating the branch: %v", err)
		}
		action := "create"
		if info.HasCurrent {
			action = "update"
		}
		commit := map[string]interface{}{"branch": branch, "commit_message": title,
			"actions": []map[string]string{{"action": action, "file_path": info.File, "content": content}}}
		if err := p.send("POST", pr+"/repository/commits", commit, nil); err != nil {
			return gitMRResult{}, fmt.Errorf("committing .gitignore: %v", err)
		}
		var mr struct {
			WebURL string `json:"web_url"`
		}
		if err := p.send("POST", pr+"/merge_requests", map[string]interface{}{"source_branch": branch, "target_branch": info.Branch, "title": title,
			"description": desc, "remove_source_branch": true}, &mr); err != nil {
			return gitMRResult{}, fmt.Errorf("opening the merge request: %v", err)
		}
		return gitMRResult{URL: mr.WebURL, Branch: branch, Kind: "merge_request"}, nil
	}
	pr := p.gh(info.Project)
	if err := p.send("POST", pr+"/git/refs", map[string]string{"ref": "refs/heads/" + branch, "sha": info.head}, nil); err != nil {
		return gitMRResult{}, fmt.Errorf("creating the branch: %v", err)
	}
	put := map[string]string{"message": title, "content": base64.StdEncoding.EncodeToString([]byte(content)), "branch": branch}
	if info.HasCurrent {
		put["sha"] = info.blob
	}
	if err := p.send("PUT", pr+"/contents/"+escapePathSegments(info.File), put, nil); err != nil {
		return gitMRResult{}, fmt.Errorf("committing .gitignore: %v", err)
	}
	var pull struct {
		HTMLURL string `json:"html_url"`
	}
	if err := p.send("POST", pr+"/pulls", map[string]string{"title": title, "head": branch, "base": info.Branch, "body": desc}, &pull); err != nil {
		return gitMRResult{}, fmt.Errorf("opening the pull request: %v", err)
	}
	return gitMRResult{URL: pull.HTMLURL, Branch: branch, Kind: "pull_request"}, nil
}

func escapePathSegments(p string) string {
	parts := strings.Split(p, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

// ─── API ─────────────────────────────────────────────

// apiGitIgnore handles the .gitignore routes of /api/git; it returns false for other routes.
//
//	GET  /api/git/gitignore/{app}                                → candidates, warnings, current file
//	POST /api/git/gitignore/{app}/preview   {patterns}           → content and diff
//	POST /api/git/gitignore/{app}/download  {patterns}           → .gitignore (download)
//	POST /api/git/gitignore/{app}/mr        {patterns, confirm}  → merge / pull request (confirm = project)
func apiGitIgnore(w http.ResponseWriter, r *http.Request, userID int, rest string, parts []string) bool {
	if len(parts) < 2 || parts[0] != "gitignore" {
		return false
	}
	app := parts[1]
	switch {
	case len(parts) == 2 && r.Method == http.MethodGet:
		info, code, err := gitIgnoreState(userID, app)
		if err != nil {
			jsonError(w, err.Error(), code)
			return true
		}
		jsonOK(w, info)

	case len(parts) == 3 && r.Method == http.MethodPost && (parts[2] == "preview" || parts[2] == "download" || parts[2] == "mr"):
		var in struct {
			Patterns []string `json:"patterns"`
			Confirm  string   `json:"confirm"`
		}
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		pats, err := cleanPatterns(in.Patterns)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return true
		}
		if parts[2] == "mr" && !gitActionAllowed(userID, "git_gitignore_mr") {
			jsonError(w, "Merge requests are not allowed for your account", 403)
			return true
		}
		info, code, err := gitIgnoreState(userID, app)
		if err != nil {
			jsonError(w, err.Error(), code)
			return true
		}
		now := time.Now()
		content, added := mergeGitignore(info.Current, pats, now)
		switch parts[2] {
		case "preview":
			lines, truncated := unifiedDiff(splitLines(info.Current), splitLines(content), 3)
			jsonOK(w, map[string]interface{}{"content": content, "added": added, "lines": lines, "truncated": truncated, "has_current": info.HasCurrent, "file": info.File})
		case "download":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename=".gitignore"`)
			w.Write([]byte(content))
		case "mr":
			if !info.CanMR {
				jsonError(w, "A merge request is not possible: "+info.MRNote, 400)
				return true
			}
			if strings.TrimSpace(in.Confirm) != info.Project {
				jsonError(w, fmt.Sprintf("Confirm with the repository name %q", info.Project), 400)
				return true
			}
			if len(added) == 0 {
				jsonError(w, "Nothing to add: the .gitignore already has these patterns", 400)
				return true
			}
			res, err := createGitignoreMR(info, content, added, now)
			details := map[string]interface{}{"app": app, "repo": info.Repo, "branch": res.Branch, "base": info.Branch, "file": info.File,
				"patterns": len(added), "source_id": info.SourceID}
			if err != nil {
				details["error"] = truncateStr(err.Error(), 300)
				auditLog(r, userID, "git.gitignore_mr", info.Repo, details)
				jsonError(w, err.Error(), 502)
				return true
			}
			details["url"] = res.URL
			auditLog(r, userID, "git.gitignore_mr", info.Repo, details)
			jsonOK(w, res)
		}

	default:
		jsonError(w, "Not found", 404)
	}
	return true
}
