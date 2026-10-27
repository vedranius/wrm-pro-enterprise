// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ─── deploy-config from Git, through the WRM Git source (no git, no SSH keys) ───
//
// With DEPLOY_CONFIG_REPO set and served by a WRM Git source, WRM keeps its own copy of the
// configuration in <user>/native-config: Pull replaces it with the branch head (only when
// there are no local edits, and only when the new configuration loads), Commit and push
// sends the local edits as one commit through the source's API and the write token.

const (
	mdGitConfigDirName = "native-config"
	mdGitStateFile     = ".wrm-git.json"
)

var mdGitConfigMu sync.Mutex

type mdGitConfigState struct {
	SourceID int               `json:"source_id"`
	Repo     string            `json:"repo"`
	Project  string            `json:"project"`
	Branch   string            `json:"branch"`
	Commit   string            `json:"commit"`
	Files    map[string]string `json:"files"`  // name → Git blob id at Commit
	Hashes   map[string]string `json:"hashes"` // name → sha256 of the pulled content
	PulledAt string            `json:"pulled_at"`
}

func mdGitConfigDir(uid int) string { return filepath.Join(mediumUserDir(uid), mdGitConfigDirName) }

func mdLoadGitConfigState(uid int) (*mdGitConfigState, bool) {
	b, err := os.ReadFile(filepath.Join(mdGitConfigDir(uid), mdGitStateFile))
	if err != nil {
		return nil, false
	}
	var st mdGitConfigState
	if json.Unmarshal(b, &st) != nil || st.Commit == "" {
		return nil, false
	}
	if st.Files == nil {
		st.Files = map[string]string{}
	}
	if st.Hashes == nil {
		st.Hashes = map[string]string{}
	}
	return &st, true
}

// mdConfigRepoSource finds the WRM Git source that serves DEPLOY_CONFIG_REPO.
func mdConfigRepoSource(uid int) (mdSourceMatch, string, error) {
	repo := strings.TrimSpace(mediumPreferences(uid)["DEPLOY_CONFIG_REPO"])
	if repo == "" {
		return mdSourceMatch{}, "", fmt.Errorf("set the configuration repository (DEPLOY_CONFIG_REPO) in the Medium settings first")
	}
	m, ok := mdFindSource(uid, repo)
	if !ok {
		return mdSourceMatch{}, repo, fmt.Errorf("no WRM Git source serves %s; add it under Git → Sources", redactURLText(repo))
	}
	return m, repo, nil
}

type mdGitChange struct {
	Name   string `json:"name"`
	Action string `json:"action"` // create | update | delete
}

// mdLocalConfigChanges compares the copy with the files of the pulled commit.
func mdLocalConfigChanges(root *os.Root, st *mdGitConfigState) ([]mdGitChange, map[string][]byte) {
	out := []mdGitChange{}
	content := map[string][]byte{}
	seen := map[string]bool{}
	for _, f := range mdConfigFiles(root) {
		b, err := mdConfigRead(root, f.Name)
		if err != nil || b == nil {
			continue
		}
		seen[f.Name] = true
		content[f.Name] = b
		switch base, ok := st.Hashes[f.Name]; {
		case !ok:
			out = append(out, mdGitChange{f.Name, "create"})
		case base != mdHashBytes(b):
			out = append(out, mdGitChange{f.Name, "update"})
		}
	}
	for name := range st.Hashes {
		if !seen[name] {
			out = append(out, mdGitChange{name, "delete"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, content
}

// mdRemoteConfig reads the configuration files of a commit.
func mdRemoteConfig(p *gitProvider, m mdSourceMatch, commit string) (map[string][]byte, map[string]string, error) {
	tree, err := p.tree(m.src.ID, m.project, commit)
	if err != nil {
		return nil, nil, err
	}
	files, ids := map[string][]byte{}, map[string]string{}
	for name, id := range tree {
		if !mdConfigNameRe.MatchString(name) {
			continue
		}
		b, ok := blobContent(m.src.ID, id)
		if !ok {
			if b, err = p.blob(m.project, id); err != nil {
				return nil, nil, fmt.Errorf("%s: %v", name, err)
			}
			storeBlob(m.src.ID, id, b)
		}
		if len(b) > mdConfigMax {
			return nil, nil, fmt.Errorf("%s is larger than 1 MiB", name)
		}
		files[name], ids[name] = b, id
	}
	return files, ids, nil
}

// mdReplaceGitConfig writes the files and the state into a new directory and swaps it in.
func mdReplaceGitConfig(uid int, files map[string][]byte, st *mdGitConfigState) error {
	dir := mdGitConfigDir(uid)
	tmp := dir + ".new"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Join(tmp, mdProjectsDir), 0o700); err != nil {
		return err
	}
	for name, b := range files {
		if !mdConfigNameRe.MatchString(name) {
			continue
		}
		if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(name)), b, 0o600); err != nil {
			os.RemoveAll(tmp)
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, mdGitStateFile), jsonMarshal(st), 0o600); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	old := dir + ".old"
	os.RemoveAll(old)
	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, old); err != nil {
			os.RemoveAll(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.Rename(old, dir)
		return err
	}
	os.RemoveAll(old)
	return nil
}

// mdCheckConfigFiles loads a set of configuration files in a temporary directory.
func mdCheckConfigFiles(files map[string][]byte) (*mdConfig, error) {
	tmp, err := os.MkdirTemp("", "wrm-md-config-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Join(tmp, mdProjectsDir), 0o700); err != nil {
		return nil, err
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(name)), b, 0o600); err != nil {
			return nil, err
		}
	}
	cfg, err := mdLoadConfig(tmp)
	if err != nil {
		return nil, fmt.Errorf("%s", strings.ReplaceAll(err.Error(), tmp, "deploy-config"))
	}
	return cfg, nil
}

type mdGitConfigView struct {
	Available  bool          `json:"available"` // DEPLOY_CONFIG_REPO is served by a WRM Git source
	Active     bool          `json:"active"`    // WRM's own copy is the configuration in use
	Repo       string        `json:"repo,omitempty"`
	Source     string        `json:"source,omitempty"`
	Branch     string        `json:"branch,omitempty"`
	Commit     string        `json:"commit,omitempty"`
	Head       string        `json:"head,omitempty"`
	Behind     bool          `json:"behind"` // the branch has commits after the pulled one
	PulledAt   string        `json:"pulled_at,omitempty"`
	Changes    []mdGitChange `json:"changes"`
	CanPush    bool          `json:"can_push"`
	Error      string        `json:"error,omitempty"`
	HeadError  string        `json:"head_error,omitempty"`
	CommitName string        `json:"author,omitempty"`
}

func mdGitConfigStatus(uid int) mdGitConfigView {
	v := mdGitConfigView{Changes: []mdGitChange{}}
	m, repo, err := mdConfigRepoSource(uid)
	v.Repo = redactURLText(repo)
	st, active := mdLoadGitConfigState(uid)
	v.Active = active
	if err != nil {
		v.Error = err.Error()
		return v
	}
	v.Available, v.Source, v.CanPush = true, m.src.Name, m.src.HasWriteToken
	v.CommitName = usernameOf(uid)
	p, err := newGitProvider(m.src)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	branch := ""
	if active {
		branch = st.Branch
		v.Commit, v.PulledAt = st.Commit, st.PulledAt
		if root, err := os.OpenRoot(mdGitConfigDir(uid)); err == nil {
			v.Changes, _ = mdLocalConfigChanges(root, st)
			root.Close()
		}
	}
	if branch == "" {
		if branch, err = p.defaultBranch(m.project); err != nil {
			v.HeadError = err.Error()
			return v
		}
	}
	v.Branch = branch
	if v.Head, err = p.branchHead(m.project, branch); err != nil {
		v.HeadError = err.Error()
	}
	v.Behind = active && v.Head != "" && v.Head != st.Commit
	return v
}

// mdGitConfigPull makes the branch head WRM's copy. With local edits it refuses unless
// discard is set; a head whose configuration does not load is never applied.
func mdGitConfigPull(uid int, discard bool) (*mdGitConfigState, int, error) {
	m, repo, err := mdConfigRepoSource(uid)
	if err != nil {
		return nil, 400, err
	}
	p, err := newGitProvider(m.src)
	if err != nil {
		return nil, 400, err
	}
	st, active := mdLoadGitConfigState(uid)
	if active && st.SourceID != m.src.ID {
		active = false // the repository setting changed: start over from the new one
	}
	if active && !discard {
		if root, err := os.OpenRoot(mdGitConfigDir(uid)); err == nil {
			changes, _ := mdLocalConfigChanges(root, st)
			root.Close()
			if len(changes) > 0 {
				return nil, 409, fmt.Errorf("deploy-config has %d local change(s); commit and push them, or discard them, before pulling", len(changes))
			}
		}
	}
	branch := ""
	if active {
		branch = st.Branch
	}
	if branch == "" {
		if branch, err = p.defaultBranch(m.project); err != nil {
			return nil, 502, err
		}
	}
	head, err := p.branchHead(m.project, branch)
	if err != nil {
		return nil, 502, err
	}
	files, ids, err := mdRemoteConfig(p, m, head)
	if err != nil {
		return nil, 502, err
	}
	if _, err := mdCheckConfigFiles(files); err != nil {
		return nil, 400, fmt.Errorf("the configuration on Git (%s @ %s) does not load, so WRM keeps the current one: %v", branch, mdShort(head), err)
	}
	hashes := map[string]string{}
	for name, b := range files {
		hashes[name] = mdHashBytes(b)
	}
	ns := &mdGitConfigState{SourceID: m.src.ID, Repo: redactURLText(repo), Project: m.project, Branch: branch, Commit: head, Files: ids, Hashes: hashes, PulledAt: nowStamp()}
	if err := mdReplaceGitConfig(uid, files, ns); err != nil {
		return nil, 500, err
	}
	return ns, 200, nil
}

func mdShort(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}

// mdGitCommitFiles sends the changes as one commit on top of head and returns the new commit.
func mdGitCommitFiles(rp, p *gitProvider, project, branch, head, message, author, email string, changes []mdGitChange, content map[string][]byte, headIDs map[string]string) (string, error) {
	switch p.kind {
	case "gitlab":
		var actions []map[string]interface{}
		for _, c := range changes {
			a := map[string]interface{}{"action": c.Action, "file_path": c.Name}
			if c.Action != "delete" {
				a["content"] = string(content[c.Name])
			}
			actions = append(actions, a)
		}
		body := map[string]interface{}{"branch": branch, "commit_message": message, "actions": actions, "author_name": author}
		if email != "" {
			body["author_email"] = email
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := p.send("POST", p.gl(project)+"/repository/commits", body, &out); err != nil {
			return "", err
		}
		return out.ID, nil
	case "github":
		var hc struct {
			Tree struct {
				SHA string `json:"sha"`
			} `json:"tree"`
		}
		if _, err := rp.getJSON(rp.gh(project)+"/git/commits/"+url.PathEscape(head), &hc); err != nil {
			return "", err
		}
		var entries []map[string]interface{}
		for _, c := range changes {
			e := map[string]interface{}{"path": c.Name, "mode": "100644", "type": "blob"}
			if c.Action == "delete" {
				e["sha"] = nil
			} else {
				e["content"] = string(content[c.Name])
			}
			entries = append(entries, e)
		}
		var tree struct {
			SHA string `json:"sha"`
		}
		if err := p.send("POST", p.gh(project)+"/git/trees", map[string]interface{}{"base_tree": hc.Tree.SHA, "tree": entries}, &tree); err != nil {
			return "", err
		}
		cm := map[string]interface{}{"message": message, "tree": tree.SHA, "parents": []string{head}}
		if email != "" {
			cm["author"] = map[string]string{"name": author, "email": email}
		}
		var commit struct {
			SHA string `json:"sha"`
		}
		if err := p.send("POST", p.gh(project)+"/git/commits", cm, &commit); err != nil {
			return "", err
		}
		// not forced: the branch only moves forward from head
		if err := p.send("PATCH", p.gh(project)+"/git/refs/heads/"+escapePathSegments(branch), map[string]interface{}{"sha": commit.SHA, "force": false}, nil); err != nil {
			return "", err
		}
		return commit.SHA, nil
	case "gitea":
		var files []map[string]interface{}
		for _, c := range changes {
			f := map[string]interface{}{"operation": c.Action, "path": c.Name}
			if c.Action != "delete" {
				f["content"] = base64.StdEncoding.EncodeToString(content[c.Name])
			}
			if c.Action != "create" {
				f["sha"] = headIDs[c.Name]
			}
			files = append(files, f)
		}
		body := map[string]interface{}{"branch": branch, "message": message, "files": files}
		if email != "" {
			body["author"] = map[string]string{"name": author, "email": email}
		}
		var out struct {
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		if err := p.send("POST", p.gh(project)+"/contents", body, &out); err != nil {
			return "", err
		}
		return out.Commit.SHA, nil
	}
	return "", fmt.Errorf("commits are not supported for this source")
}

// mdGitConfigPush commits and pushes the local edits; the whole configuration must load,
// and a file somebody else changed on Git since the pull stops the push.
func mdGitConfigPush(uid int, message string) (string, []mdGitChange, int, error) {
	st, active := mdLoadGitConfigState(uid)
	if !active {
		return "", nil, 400, fmt.Errorf("pull deploy-config from Git first")
	}
	m, _, err := mdConfigRepoSource(uid)
	if err != nil {
		return "", nil, 400, err
	}
	if m.src.ID != st.SourceID {
		return "", nil, 409, fmt.Errorf("the configuration repository changed since the pull; pull again")
	}
	root, err := os.OpenRoot(mdGitConfigDir(uid))
	if err != nil {
		return "", nil, 500, err
	}
	changes, content := mdLocalConfigChanges(root, st)
	root.Close()
	if len(changes) == 0 {
		return "", nil, 400, fmt.Errorf("there are no local changes to commit")
	}
	if _, err := mdCheckConfigFiles(content); err != nil {
		return "", nil, 400, fmt.Errorf("the configuration does not load, so it is not committed: %v", err)
	}
	wp, err := newGitWriteProvider(m.src)
	if err != nil {
		return "", nil, 400, fmt.Errorf("%v: add a write token to the Git source %s", err, m.src.Name)
	}
	p, err := newGitProvider(m.src) // reads go with the read token
	if err != nil {
		return "", nil, 400, err
	}
	head, err := p.branchHead(m.project, st.Branch)
	if err != nil {
		return "", nil, 502, err
	}
	headIDs := st.Files
	if head != st.Commit {
		tree, err := p.tree(m.src.ID, m.project, head)
		if err != nil {
			return "", nil, 502, err
		}
		headIDs = map[string]string{}
		var clash []string
		for name, id := range tree {
			if mdConfigNameRe.MatchString(name) {
				headIDs[name] = id
			}
		}
		for _, c := range changes {
			if headIDs[c.Name] != st.Files[c.Name] {
				clash = append(clash, c.Name)
			}
		}
		if len(clash) > 0 {
			return "", nil, 409, fmt.Errorf("changed on Git since your pull: %s. Copy your edits, discard them, pull and apply them again", strings.Join(clash, ", "))
		}
	}
	if strings.TrimSpace(message) == "" {
		names := []string{}
		for i, c := range changes {
			if i == 3 {
				names = append(names, fmt.Sprintf("and %d more", len(changes)-3))
				break
			}
			names = append(names, path.Base(c.Name))
		}
		message = "deploy-config: " + strings.Join(names, ", ")
	}
	commit, err := mdGitCommitFiles(p, wp, m.project, st.Branch, head, strings.TrimSpace(message), usernameOf(uid), mediumPreferences(uid)["DEPLOY_USER_EMAIL"], changes, content, headIDs)
	if err != nil {
		return "", nil, 502, err
	}
	return commit, changes, 200, nil
}

// mdAPIConfigGit serves /config/git (GET state, POST pull | push | stop).
func mdAPIConfigGit(w http.ResponseWriter, r *http.Request, uid int, action string) {
	if action == "/config/git" && r.Method == "GET" {
		jsonOK(w, mdGitConfigStatus(uid))
		return
	}
	if r.Method != "POST" {
		jsonError(w, "not found", 404)
		return
	}
	if !mdNativeAllowed(uid) {
		jsonError(w, "deploy-config Git actions in WRM are disabled by policy (medium_native)", 403)
		return
	}
	var in struct {
		Discard bool   `json:"discard"`
		Message string `json:"message"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&in) != nil && r.ContentLength > 0 {
		jsonError(w, "invalid request", 400)
		return
	}
	mdGitConfigMu.Lock()
	defer mdGitConfigMu.Unlock()
	mdConfigEditMu.Lock()
	defer mdConfigEditMu.Unlock()
	switch action {
	case "/config/git/pull":
		st, code, err := mdGitConfigPull(uid, in.Discard)
		if err != nil {
			mediumRecord(uid, "config_pull", "error", 0, "", err.Error(), "")
			jsonError(w, err.Error(), code)
			return
		}
		auditLog(r, uid, "git.medium_config_pull", st.Project, map[string]interface{}{"branch": st.Branch, "commit": st.Commit, "discarded": in.Discard})
		mediumRecord(uid, "config_pull", "ok", 0, st.Project+" "+st.Branch+" @ "+mdShort(st.Commit), "", "")
		jsonOK(w, mdGitConfigStatus(uid))
	case "/config/git/push":
		if len(in.Message) > 5000 {
			jsonError(w, "the commit message is too long", 400)
			return
		}
		commit, changes, code, err := mdGitConfigPush(uid, in.Message)
		if err != nil {
			mediumRecord(uid, "config_push", "error", 0, "", err.Error(), "")
			jsonError(w, err.Error(), code)
			return
		}
		names := []string{}
		for _, c := range changes {
			names = append(names, c.Action+" "+c.Name)
		}
		st, _ := mdLoadGitConfigState(uid)
		auditLog(r, uid, "git.medium_config_push", st.Project, map[string]interface{}{"branch": st.Branch, "commit": commit, "files": names})
		mediumRecord(uid, "config_push", "ok", 0, st.Project+" "+st.Branch+" @ "+mdShort(commit)+": "+strings.Join(names, ", "), "", "")
		// the pushed commit (and anything else new on the branch) becomes the copy
		if _, _, err := mdGitConfigPull(uid, true); err != nil {
			v := mdGitConfigStatus(uid)
			v.Error = "pushed " + mdShort(commit) + ", but reading it back failed: " + err.Error()
			jsonOK(w, v)
			return
		}
		jsonOK(w, mdGitConfigStatus(uid))
	case "/config/git/stop":
		st, ok := mdLoadGitConfigState(uid)
		if !ok {
			jsonError(w, "WRM does not keep a Git copy of deploy-config", 400)
			return
		}
		if err := os.RemoveAll(mdGitConfigDir(uid)); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		auditLog(r, uid, "git.medium_config_git_stop", st.Project, map[string]interface{}{"commit": st.Commit})
		jsonOK(w, mdGitConfigStatus(uid))
	default:
		jsonError(w, "not found", 404)
	}
}
