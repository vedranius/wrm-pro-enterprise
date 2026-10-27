// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakeWrites records the write calls of the fake Git server (branches, commits, merge /
// pull requests) and checks the write token.
type fakeWrites struct {
	calls []string
	body  map[string]map[string]interface{}
	auth  []string
}

// addHead puts a new commit on top of main (the helper reads the head of the branch, the
// target stays at the latest tag).
func addHead(g *fakeGit, files map[string]string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	c := &fakeCommit{sha: commitSHA(fmt.Sprint("head", len(g.branches["main"]))), date: "2026-10-01", files: map[string]string{}}
	for k, v := range g.branches["main"][0].files {
		c.files[k] = v
	}
	for k, v := range files {
		c.files[k] = v
		g.blobs[blobSHA(v)] = v
	}
	g.branches["main"] = append([]*fakeCommit{c}, g.branches["main"]...)
	return c.sha
}

func (fw *fakeWrites) handler(g *fakeGit, wtoken string) func(w http.ResponseWriter, r *http.Request) bool {
	fw.body = map[string]map[string]interface{}{}
	return func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == "GET" {
			return false
		}
		auth := r.Header.Get("PRIVATE-TOKEN")
		if g.kind == "github" {
			auth = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		fw.auth = append(fw.auth, auth)
		if auth != wtoken {
			w.WriteHeader(403)
			w.Write([]byte(`{"message":"read-only token"}`))
			return true
		}
		var b map[string]interface{}
		json.NewDecoder(r.Body).Decode(&b)
		p := strings.TrimPrefix(strings.TrimPrefix(r.URL.EscapedPath(), "/api/v4"), "/api/v3")
		key := r.Method + " " + p
		fw.calls = append(fw.calls, key)
		fw.body[key] = b
		w.WriteHeader(201)
		switch {
		case strings.HasSuffix(p, "/merge_requests"):
			w.Write([]byte(`{"web_url":"https://git.example.com/demo/demo-api/-/merge_requests/7"}`))
		case strings.HasSuffix(p, "/pulls"):
			w.Write([]byte(`{"html_url":"https://github.example.com/demo/demo-api/pull/7"}`))
		default:
			w.Write([]byte(`{}`))
		}
		return true
	}
}

func TestGitIgnoreHelper(t *testing.T) {
	e := newDeployEnv(t, "git-ignore")
	writeFile(t, e.dir+"/run.py", runV110)
	writeFile(t, e.dir+"/lib/util.py", utilV1)
	writeFile(t, e.dir+"/config.ini", "[main]\nhost=app-01.example.com\n")
	writeFile(t, e.dir+"/logs/app.log", "started ok\n")
	writeFile(t, e.dir+"/local.txt", "x")
	e.check(t)
	head := addHead(e.g, map[string]string{"linux/.gitignore": "*.log\n", "linux/.env": "API_KEY=1\n", "linux/.env.example": "API_KEY=\n", "linux/log/worker.log": "committed\n"})

	var info gitIgnoreInfo
	if code := e.u.jsonDo("GET", "/api/git/gitignore/demo-api", nil, &info); code != 200 {
		t.Fatalf("info: %d", code)
	}
	extra := map[string]gitIgnoreCandidate{}
	for _, c := range info.Extra {
		extra[c.Path] = c
	}
	// log files are not extra files any more: the Logs patterns cover them
	if c, ok := extra["logs/app.log"]; ok {
		t.Fatalf("extra log: %+v", c)
	}
	if extra["local.txt"].Pattern != "/local.txt" || len(extra) != 1 {
		t.Fatalf("extra: %+v", info.Extra)
	}
	if info.Current != "*.log\n" || !info.HasCurrent || info.File != "linux/.gitignore" || info.Repo == "" || info.Branch != "main" {
		t.Fatalf("current: %+v", info)
	}
	py, logs := false, false
	for _, s := range info.Stacks {
		py = py || (s.Name == "Python" && s.Detected)
		logs = logs || (s.Name == "Logs" && s.Detected && s.Reason == "log/worker.log" && strings.Join(s.Patterns, " ") == "log/ logs/ *.log *.log.*")
	}
	if !py || !logs {
		t.Fatalf("stacks: %+v", info.Stacks)
	}
	warn := map[string]gitIgnoreWarning{}
	for _, w := range info.Warnings {
		warn[w.Path] = w
	}
	if warn[".env"].Reason != "secret" || warn[".env"].Template != ".env.example" || warn["config.ini"].Reason != "protected" ||
		warn["config.ini"].Template != "config.example.ini" || warn["log/worker.log"].Reason != "log" || len(warn) != 3 {
		t.Fatalf("warnings: %+v", info.Warnings)
	}
	if info.CanMR || !strings.Contains(info.MRNote, "write token") {
		t.Fatalf("MR without a write token: %+v", info)
	}

	// preview and download: only new patterns are appended
	pats := []string{"logs/", "*.log", "/local.txt", "__pycache__/"}
	var pv struct {
		Content string     `json:"content"`
		Added   []string   `json:"added"`
		Lines   []diffLine `json:"lines"`
	}
	if code := e.u.jsonDo("POST", "/api/git/gitignore/demo-api/preview", map[string]interface{}{"patterns": pats}, &pv); code != 200 {
		t.Fatalf("preview: %d", code)
	}
	if strings.Join(pv.Added, ",") != "logs/,/local.txt,__pycache__/" || !strings.HasPrefix(pv.Content, "*.log\n\n# Added with WRM PRO on ") ||
		!strings.HasSuffix(pv.Content, "\nlogs/\n/local.txt\n__pycache__/\n") || len(pv.Lines) == 0 {
		t.Fatalf("preview: %+v", pv)
	}
	resp := e.u.do("POST", "/api/git/gitignore/demo-api/download", strings.NewReader(`{"patterns":["logs/","*.log","/local.txt","__pycache__/"]}`), "application/json")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != pv.Content || !strings.Contains(resp.Header.Get("Content-Disposition"), ".gitignore") {
		t.Fatalf("download: %d %q", resp.StatusCode, body)
	}
	if code := e.u.jsonDo("POST", "/api/git/gitignore/demo-api/preview", map[string]interface{}{"patterns": []string{"a\nb"}}, nil); code != 400 {
		t.Fatalf("bad pattern: %d", code)
	}

	// a merge request only with a write token, the explicit request and the repository named
	fw := &fakeWrites{}
	e.g.mu.Lock()
	e.g.extra = fw.handler(e.g, "wtok-gitlab")
	e.g.mu.Unlock()
	mr := map[string]interface{}{"patterns": pats, "confirm": "demo/demo-api"}
	if code := e.u.jsonDo("POST", "/api/git/gitignore/demo-api/mr", mr, nil); code != 400 || len(fw.calls) != 0 {
		t.Fatalf("MR without write token: %d %v", code, fw.calls)
	}
	var st gitStateResp
	e.u.jsonDo("GET", "/api/git", nil, &st)
	src := st.Sources[0]
	if code := e.u.jsonDo("PUT", fmt.Sprintf("/api/git/sources/%d", src.ID), map[string]interface{}{"kind": "gitlab", "name": src.Name, "url": src.URL, "write_token": "wtok-gitlab"}, nil); code != 200 {
		t.Fatalf("write token: %d", code)
	}
	var raw string
	db.QueryRow(`SELECT write_token FROM git_sources WHERE id=?`, src.ID).Scan(&raw)
	if raw == "" || strings.Contains(raw, "wtok") {
		t.Fatal("write token not stored encrypted")
	}
	e.u.jsonDo("GET", "/api/git/gitignore/demo-api", nil, &info)
	if !info.CanMR {
		t.Fatalf("can_mr: %+v", info.MRNote)
	}
	if code := e.u.jsonDo("POST", "/api/git/gitignore/demo-api/mr", map[string]interface{}{"patterns": pats, "confirm": "demo/other"}, nil); code != 400 || len(fw.calls) != 0 {
		t.Fatalf("MR with the wrong repository: %d %v", code, fw.calls)
	}
	var res gitMRResult
	if code := e.u.jsonDo("POST", "/api/git/gitignore/demo-api/mr", mr, &res); code != 200 {
		t.Fatalf("MR: %d", code)
	}
	pr := "/projects/demo%2Fdemo-api"
	if res.Kind != "merge_request" || !strings.HasSuffix(res.URL, "/merge_requests/7") || len(fw.calls) != 3 ||
		fw.calls[0] != "POST "+pr+"/repository/branches" || fw.calls[2] != "POST "+pr+"/merge_requests" {
		t.Fatalf("MR calls: %+v %v", res, fw.calls)
	}
	if b := fw.body["POST "+pr+"/repository/branches"]; b["ref"] != head || b["branch"] != res.Branch {
		t.Fatalf("branch: %v", b)
	}
	act := fw.body["POST "+pr+"/repository/commits"]["actions"].([]interface{})[0].(map[string]interface{})
	if act["action"] != "update" || act["file_path"] != "linux/.gitignore" || act["content"] != pv.Content {
		t.Fatalf("commit: %v", act)
	}
	if b := fw.body["POST "+pr+"/merge_requests"]; b["target_branch"] != "main" || b["source_branch"] != res.Branch {
		t.Fatalf("merge request: %v", b)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='git.gitignore_mr' AND user_id=? AND details LIKE ?`, e.u.userID, "%merge_requests/7%").Scan(&n)
	if n != 1 {
		t.Fatalf("audit: %d", n)
	}
	setSetting("git_gitignore_mr", "off")
	defer setSetting("git_gitignore_mr", "admins")
	if code := e.u.jsonDo("POST", "/api/git/gitignore/demo-api/mr", mr, nil); code != 403 || len(fw.calls) != 3 {
		t.Fatalf("policy: %d", code)
	}
}

func TestGitIgnorePullRequest(t *testing.T) {
	srv := newTestServer(t)
	g := newFakeGit(t, "github")
	u := setupGitUser(t, srv, "git-ignore-gh", g)
	setSetting("git_gitignore_mr", "all") // not an administrator
	defer setSetting("git_gitignore_mr", "admins")
	refreshDemo(t, u)
	head := addHead(g, map[string]string{"linux/settings_local.py": "DEBUG = True\n"})
	fw := &fakeWrites{}
	g.mu.Lock()
	g.extra = fw.handler(g, "wtok-github")
	g.mu.Unlock()
	var st gitStateResp
	u.jsonDo("GET", "/api/git", nil, &st)
	u.jsonDo("PUT", fmt.Sprintf("/api/git/sources/%d", st.Sources[0].ID), map[string]interface{}{"kind": "github", "name": "gh", "url": g.srv.URL, "write_token": "wtok-github"}, nil)
	var info gitIgnoreInfo
	u.jsonDo("GET", "/api/git/gitignore/demo-api", nil, &info)
	if info.HasCurrent || !info.CanMR || len(info.Extra) != 0 {
		t.Fatalf("info: %+v", info)
	}
	var res gitMRResult
	if code := u.jsonDo("POST", "/api/git/gitignore/demo-api/mr", map[string]interface{}{"patterns": []string{"settings_local.py"}, "confirm": "demo/demo-api"}, &res); code != 200 {
		t.Fatalf("PR: %d", code)
	}
	pr := "/repos/demo/demo-api"
	if res.Kind != "pull_request" || len(fw.calls) != 3 || fw.calls[1] != "PUT "+pr+"/contents/linux/.gitignore" {
		t.Fatalf("PR calls: %+v %v", res, fw.calls)
	}
	if b := fw.body["POST "+pr+"/git/refs"]; b["sha"] != head || b["ref"] != "refs/heads/"+res.Branch {
		t.Fatalf("ref: %v", b)
	}
	put := fw.body["PUT "+pr+"/contents/linux/.gitignore"]
	content, _ := base64.StdEncoding.DecodeString(put["content"].(string))
	if _, hasSHA := put["sha"]; hasSHA || put["branch"] != res.Branch || !strings.HasSuffix(string(content), "\nsettings_local.py\n") {
		t.Fatalf("contents: %v %q", put, content)
	}
	if b := fw.body["POST "+pr+"/pulls"]; b["base"] != "main" || b["head"] != res.Branch {
		t.Fatalf("pull: %v", b)
	}
	for _, a := range fw.auth {
		if a != "wtok-github" {
			t.Fatalf("write call with token %q", a)
		}
	}
	// merge: the same patterns are not added twice
	got, added := mergeGitignore("a\r\n/b\r\n", []string{"a", "b", "c"}, time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
	if strings.Join(added, ",") != "c" || got != "a\r\n/b\r\n\r\n# Added with WRM PRO on 2026-10-09\r\nc\r\n" {
		t.Fatalf("merge: %q %v", got, added)
	}
}
