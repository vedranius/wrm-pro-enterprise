// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── deploy-config from Git through a WRM Git source (fake GitLab / GitHub / Gitea) ───

// mdFakeConfigWrites answers the commit APIs the config push uses and applies them to main.
func mdFakeConfigWrites(g *fakeGit, wtoken string, calls *[]string) func(w http.ResponseWriter, r *http.Request) bool {
	trees := map[string]map[string]string{}
	commit := func(files map[string]string) string {
		c := &fakeCommit{sha: commitSHA(fmt.Sprint("cfg", len(g.branches["main"]))), date: "2026-10-22", files: files}
		for _, v := range files {
			g.blobs[blobSHA(v)] = v
		}
		g.branches["main"] = append([]*fakeCommit{c}, g.branches["main"]...)
		return c.sha
	}
	clone := func(m map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range m {
			out[k] = v
		}
		return out
	}
	return func(w http.ResponseWriter, r *http.Request) bool {
		p := r.URL.EscapedPath()
		isWrite := r.Method != "GET"
		if !isWrite && !(g.kind == "github" && strings.Contains(p, "/git/commits/")) {
			return false
		}
		auth := r.Header.Get("PRIVATE-TOKEN")
		if g.kind == "github" {
			auth = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		} else if g.kind == "gitea" {
			auth = strings.TrimPrefix(r.Header.Get("Authorization"), "token ")
		}
		if isWrite && auth != wtoken {
			w.WriteHeader(403)
			w.Write([]byte(`{"message":"write token needed"}`))
			return true
		}
		*calls = append(*calls, r.Method+" "+p)
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		out := func(v interface{}) { json.NewEncoder(w).Encode(v) }
		head := g.branches["main"][0]
		switch {
		case g.kind == "gitlab" && r.Method == "POST" && strings.HasSuffix(p, "/repository/commits"):
			files := clone(head.files)
			for _, a := range body["actions"].([]interface{}) {
				a := a.(map[string]interface{})
				if a["action"] == "delete" {
					delete(files, a["file_path"].(string))
				} else {
					files[a["file_path"].(string)] = a["content"].(string)
				}
			}
			out(map[string]string{"id": commit(files)})
		case g.kind == "github" && r.Method == "GET":
			sha := p[strings.LastIndex(p, "/")+1:]
			trees["tree-"+sha] = g.commit(sha).files
			out(map[string]interface{}{"tree": map[string]string{"sha": "tree-" + sha}})
		case g.kind == "github" && strings.HasSuffix(p, "/git/trees"):
			files := clone(trees[body["base_tree"].(string)])
			for _, e := range body["tree"].([]interface{}) {
				e := e.(map[string]interface{})
				if e["sha"] == nil && e["content"] == nil {
					delete(files, e["path"].(string))
				} else {
					files[e["path"].(string)] = e["content"].(string)
				}
			}
			id := fmt.Sprint("newtree", len(trees))
			trees[id] = files
			out(map[string]string{"sha": id})
		case g.kind == "github" && strings.HasSuffix(p, "/git/commits"):
			if body["parents"].([]interface{})[0] != head.sha {
				w.WriteHeader(422)
				return true
			}
			out(map[string]string{"sha": commit(trees[body["tree"].(string)])})
		case g.kind == "github" && r.Method == "PATCH":
			if body["force"] != false || body["sha"] != g.branches["main"][0].sha {
				w.WriteHeader(422)
				return true
			}
			out(map[string]string{"ref": "refs/heads/main"})
		case g.kind == "gitea" && strings.HasSuffix(p, "/contents"):
			files := clone(head.files)
			for _, f := range body["files"].([]interface{}) {
				f := f.(map[string]interface{})
				name := f["path"].(string)
				if f["operation"] != "create" && f["sha"] != blobSHA(files[name]) {
					w.WriteHeader(409)
					return true
				}
				if f["operation"] == "delete" {
					delete(files, name)
				} else {
					b, _ := base64.StdEncoding.DecodeString(f["content"].(string))
					files[name] = string(b)
				}
			}
			out(map[string]interface{}{"commit": map[string]string{"sha": commit(files)}})
		default:
			w.WriteHeader(404)
		}
		return true
	}
}

func mdSetPreferences(t *testing.T, uid int, prefs map[string]string) {
	t.Helper()
	dir := mediumUserDir(uid)
	os.MkdirAll(dir, 0o700)
	b, _ := json.Marshal(prefs)
	if err := os.WriteFile(filepath.Join(dir, "preferences.enc"), []byte(encryptValue(string(b))), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMediumConfigGit(t *testing.T) {
	for _, kind := range []string{"gitlab", "github", "gitea"} {
		t.Run(kind, func(t *testing.T) { mdTestConfigGit(t, kind) })
	}
}

func mdTestConfigGit(t *testing.T, kind string) {
	srv := newTestServer(t)
	g := newFakeGit(t, kind)
	u := setupGitUser(t, srv, "md-cfggit-"+kind, g)
	setSetting("medium_native", "all")
	defer setSetting("medium_native", "admins")
	setSetting("git_medium", "all")
	defer setSetting("git_medium", "admins")
	os.RemoveAll(mediumUserDir(u.userID))
	t.Cleanup(func() { os.RemoveAll(mediumUserDir(u.userID)) })
	project := "name: demo-api\nrepo: git@git.example.com:team/demo-api.git\ntargets:\n  dev: app-01:/srv/a/demo-api\n"
	addHead(g, map[string]string{"servers.yaml": mdServersYAML, "environments.yaml": mdEnvsYAML, "projects/demo-api.yaml": project})
	var v mdGitConfigView
	if u.jsonDo("GET", "/api/git/medium/native/config/git", nil, &v); v.Available || v.Error == "" {
		t.Fatalf("without a repository: %+v", v)
	}
	mdSetPreferences(t, u.userID, map[string]string{"DEPLOY_CONFIG_REPO": g.srv.URL + "/demo/demo-api.git", "DEPLOY_USER_EMAIL": "dev@example.com"})
	if u.jsonDo("GET", "/api/git/medium/native/config/git", nil, &v); !v.Available || v.Active || v.Branch != "main" || v.Head == "" {
		t.Fatalf("available: %+v", v)
	}
	// pull: WRM's copy becomes the configuration
	if code := u.jsonDo("POST", "/api/git/medium/native/config/git/pull", map[string]interface{}{}, &v); code != 200 || !v.Active || v.Commit != v.Head || len(v.Changes) != 0 {
		t.Fatalf("pull: %d %+v", code, v)
	}
	if mdConfigDir(u.userID) != mdGitConfigDir(u.userID) {
		t.Fatal("the Git copy is not the configuration in use")
	}
	var cfg struct {
		OK bool `json:"ok"`
	}
	if u.jsonDo("GET", "/api/git/medium/native/config", nil, &cfg); !cfg.OK {
		t.Fatal("the pulled configuration does not load")
	}
	// edit locally (through the editor), then push
	var f struct {
		Text, Hash string
	}
	u.jsonDo("GET", "/api/git/medium/native/config/file?name=projects/demo-api.yaml", nil, &f)
	var res map[string]interface{}
	u.jsonDo("POST", "/api/git/medium/native/config/files", map[string]interface{}{"name": "projects/demo-api.yaml", "text": "# edited in WRM\n" + f.Text, "base_hash": f.Hash}, &res)
	u.jsonDo("POST", "/api/git/medium/native/config/files", map[string]interface{}{"name": "projects/demo-web.yaml", "text": strings.ReplaceAll(project, "demo-api", "demo-web")}, &res)
	if u.jsonDo("GET", "/api/git/medium/native/config/git", nil, &v); len(v.Changes) != 2 {
		t.Fatalf("changes: %+v", v.Changes)
	}
	if code := u.jsonDo("POST", "/api/git/medium/native/config/git/pull", map[string]interface{}{}, nil); code != 409 {
		t.Fatalf("pull over local edits: %d", code)
	}
	var calls []string
	g.mu.Lock()
	g.extra = mdFakeConfigWrites(g, "wtok-"+kind, &calls)
	g.mu.Unlock()
	if code := u.jsonDo("POST", "/api/git/medium/native/config/git/push", map[string]interface{}{"message": "add demo-web"}, nil); code != 400 {
		t.Fatalf("push without a write token: %d", code)
	}
	var st gitStateResp
	u.jsonDo("GET", "/api/git", nil, &st)
	u.jsonDo("PUT", fmt.Sprintf("/api/git/sources/%d", st.Sources[0].ID), map[string]interface{}{"kind": kind, "name": st.Sources[0].Name, "url": g.srv.URL, "write_token": "wtok-" + kind}, nil)
	if code := u.jsonDo("POST", "/api/git/medium/native/config/git/push", map[string]interface{}{"message": "add demo-web"}, &v); code != 200 || !v.Active || len(v.Changes) != 0 || v.Behind {
		t.Fatalf("push: %d %+v %v", code, v, calls)
	}
	g.mu.Lock()
	got := g.branches["main"][0].files
	g.mu.Unlock()
	if !strings.HasPrefix(got["projects/demo-api.yaml"], "# edited in WRM\n") || got["projects/demo-web.yaml"] == "" || got["README.md"] != "# demo" {
		t.Fatalf("pushed files: %v", got)
	}
	// somebody else changes the same file on Git: the push stops, nothing is sent
	head := addHead(g, map[string]string{"projects/demo-api.yaml": project + "# theirs\n"})
	u.jsonDo("GET", "/api/git/medium/native/config/file?name=projects/demo-api.yaml", nil, &f)
	u.jsonDo("POST", "/api/git/medium/native/config/files", map[string]interface{}{"name": "projects/demo-api.yaml", "text": f.Text + "# mine\n", "base_hash": f.Hash}, &res)
	n := len(calls)
	if code := u.jsonDo("POST", "/api/git/medium/native/config/git/push", map[string]interface{}{}, &res); code != 409 || !strings.Contains(fmt.Sprint(res["error"]), "projects/demo-api.yaml") {
		t.Fatalf("conflicting push: %d %v", code, res)
	}
	writes := 0
	for _, c := range calls[n:] {
		if !strings.HasPrefix(c, "GET ") {
			writes++
		}
	}
	if writes != 0 {
		t.Fatalf("a conflicting push wrote: %v", calls[n:])
	}
	// discard and pull: the copy follows Git again
	if code := u.jsonDo("POST", "/api/git/medium/native/config/git/pull", map[string]interface{}{"discard": true}, &v); code != 200 || v.Commit != head || len(v.Changes) != 0 {
		t.Fatalf("discard and pull: %d %+v", code, v)
	}
	// a broken configuration on Git is never applied
	addHead(g, map[string]string{"servers.yaml": "servers: 7\n"})
	if code := u.jsonDo("POST", "/api/git/medium/native/config/git/pull", map[string]interface{}{}, &res); code != 400 || !strings.Contains(fmt.Sprint(res["error"]), "keeps the current one") {
		t.Fatalf("broken pull: %d %v", code, res)
	}
	if b, _ := os.ReadFile(filepath.Join(mdGitConfigDir(u.userID), "servers.yaml")); string(b) != mdServersYAML {
		t.Fatalf("broken configuration applied: %q", b)
	}
	// stop: back to the imported configuration
	if code := u.jsonDo("POST", "/api/git/medium/native/config/git/stop", map[string]interface{}{}, &v); code != 200 || v.Active {
		t.Fatalf("stop: %d %+v", code, v)
	}
	if mdConfigDir(u.userID) == mdGitConfigDir(u.userID) {
		t.Fatal("the Git copy is still in use")
	}
	var audits int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE user_id=? AND action IN ('git.medium_config_pull','git.medium_config_push','git.medium_config_git_stop')`, u.userID).Scan(&audits)
	if audits < 4 {
		t.Fatalf("audit rows: %d", audits)
	}
	// the policy
	setSetting("medium_native", "admins")
	if code := u.jsonDo("POST", "/api/git/medium/native/config/git/pull", map[string]interface{}{}, nil); code != 403 {
		t.Fatalf("policy: %d", code)
	}
}
