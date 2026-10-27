// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// ─── fake Git server (GitLab API v4 and GitHub REST) ──

type fakeCommit struct {
	sha, date string
	files     map[string]string // path → content
}

type fakeGit struct {
	kind     string
	srv      *httptest.Server
	token    string
	mu       sync.Mutex
	branches map[string][]*fakeCommit // newest first
	tags     map[string]*fakeCommit
	blobs    map[string]string
	blobReqs atomic.Int64
	treeReqs atomic.Int64
	// extra routes of a test (write API, CI triggers); returns true when it answered
	extra func(w http.ResponseWriter, r *http.Request) bool
}

func blobSHA(content string) string {
	s := sha1.Sum([]byte("blob " + content))
	return hex.EncodeToString(s[:])
}

func commitSHA(n string) string {
	s := sha1.Sum([]byte("commit " + n))
	return hex.EncodeToString(s[:])
}

const (
	runV19  = "print('v1.9')\n"
	runV110 = "print('v1.10')\n"
	runHead = "print('head')\n"
	utilV1  = "def util():\n    return 1\n"
)

// newFakeGit serves project demo/demo-api: main has v1.9 → v1.10 → (untagged) → head;
// v2.0 is tagged on a feature branch only.
func newFakeGit(t *testing.T, kind string) *fakeGit {
	g := &fakeGit{kind: kind, token: "tok-" + kind, branches: map[string][]*fakeCommit{}, tags: map[string]*fakeCommit{}, blobs: map[string]string{}}
	mk := func(n, date, run string) *fakeCommit {
		c := &fakeCommit{sha: commitSHA(n), date: date, files: map[string]string{
			"README.md": "# demo", "linux/run.py": run, "linux/lib/util.py": utilV1, "linux/config.ini": "[main]\nhost=example.com\n", "docs/x.md": "x"}}
		for _, v := range c.files {
			g.blobs[blobSHA(v)] = v
		}
		return c
	}
	c1 := mk("c1", "2026-09-01", runV19)
	c2 := mk("c2", "2026-09-10", runV110)
	c3 := mk("c3", "2026-09-20", runV110)
	c4 := mk("c4", "2026-09-30", runHead)
	f1 := mk("f1", "2026-09-25", "print('v2')\n")
	g.branches["main"] = []*fakeCommit{c4, c3, c2, c1}
	g.branches["feature"] = []*fakeCommit{f1, c2, c1}
	g.tags["v1.9"], g.tags["v1.10"], g.tags["v2.0"] = c1, c2, f1
	g.srv = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGit) commit(sha string) *fakeCommit {
	for _, list := range g.branches {
		for _, c := range list {
			if c.sha == sha {
				return c
			}
		}
	}
	return nil
}

func (g *fakeGit) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.extra != nil && g.extra(w, r) {
		return
	}
	auth := r.Header.Get("PRIVATE-TOKEN")
	if g.kind == "github" {
		auth = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	} else if g.kind == "gitea" {
		auth = strings.TrimPrefix(r.Header.Get("Authorization"), "token ")
	}
	if auth != g.token {
		w.WriteHeader(401)
		w.Write([]byte(`{"message":"401 Unauthorized"}`))
		return
	}
	p := r.URL.EscapedPath()
	q := r.URL.Query()
	out := func(v interface{}) { json.NewEncoder(w).Encode(v) }
	var proj, rest string
	if g.kind == "gitlab" {
		p = strings.TrimPrefix(p, "/api/v4")
		if p == "/groups/demo/projects" {
			out([]map[string]interface{}{{"path_with_namespace": "demo/demo-api"}, {"path_with_namespace": "demo/old", "archived": true}})
			return
		}
		if !strings.HasPrefix(p, "/projects/demo%2Fdemo-api") {
			w.WriteHeader(404)
			return
		}
		proj, rest = "demo/demo-api", strings.TrimPrefix(p, "/projects/demo%2Fdemo-api")
		rest = strings.TrimPrefix(rest, "/repository")
	} else {
		p = strings.TrimPrefix(strings.TrimPrefix(p, "/api/v3"), "/api/v1")
		if p == "/orgs/demo/repos" {
			out([]map[string]interface{}{{"full_name": "demo/demo-api"}})
			return
		}
		if !strings.HasPrefix(p, "/repos/demo/demo-api") {
			w.WriteHeader(404)
			return
		}
		proj, rest = "demo/demo-api", strings.TrimPrefix(p, "/repos/demo/demo-api")
		rest = strings.TrimPrefix(rest, "/git")
	}
	_ = proj
	switch {
	case rest == "":
		out(map[string]string{"default_branch": "main"})
	case rest == "/branches":
		var l []map[string]string
		for b := range g.branches {
			l = append(l, map[string]string{"name": b})
		}
		out(l)
	case strings.HasPrefix(rest, "/branches/"):
		list := g.branches[strings.TrimPrefix(rest, "/branches/")]
		if len(list) == 0 {
			w.WriteHeader(404)
			return
		}
		out(map[string]interface{}{"name": strings.TrimPrefix(rest, "/branches/"), "commit": map[string]string{"id": list[0].sha, "sha": list[0].sha}})
	case rest == "/tags":
		var l []map[string]interface{}
		for n, c := range g.tags {
			l = append(l, map[string]interface{}{"name": n, "commit": map[string]string{"id": c.sha, "sha": c.sha, "committed_date": c.date + "T10:00:00Z"}})
		}
		out(l)
	case rest == "/commits":
		b := q.Get("ref_name")
		if g.kind != "gitlab" {
			b = q.Get("sha")
		}
		var l []map[string]interface{}
		for _, c := range g.branches[b] {
			if g.kind == "gitlab" {
				l = append(l, map[string]interface{}{"id": c.sha, "committed_date": c.date + "T10:00:00Z"})
			} else {
				l = append(l, map[string]interface{}{"sha": c.sha, "commit": map[string]interface{}{"committer": map[string]string{"date": c.date + "T10:00:00Z"}}})
			}
		}
		if l == nil {
			w.WriteHeader(404)
			return
		}
		out(l)
	case rest == "/tree" || strings.HasPrefix(rest, "/trees/"):
		g.treeReqs.Add(1)
		sha := q.Get("ref")
		if g.kind != "gitlab" {
			sha = strings.TrimPrefix(rest, "/trees/")
		}
		c := g.commit(sha)
		if c == nil {
			w.WriteHeader(404)
			return
		}
		var l []map[string]string
		for f, v := range c.files {
			if g.kind == "gitlab" {
				l = append(l, map[string]string{"path": f, "type": "blob", "id": blobSHA(v)})
			} else {
				l = append(l, map[string]string{"path": f, "type": "blob", "sha": blobSHA(v)})
			}
		}
		if g.kind == "gitlab" {
			out(l)
		} else {
			out(map[string]interface{}{"tree": l, "truncated": false})
		}
	case rest == "/compare" || strings.HasPrefix(rest, "/compare/"):
		from, to := q.Get("from"), q.Get("to")
		if g.kind != "gitlab" {
			from, to, _ = strings.Cut(strings.TrimPrefix(rest, "/compare/"), "...")
		}
		resolve := func(x string) *fakeCommit {
			if c, ok := g.tags[x]; ok {
				return c
			}
			for _, c := range g.branches["main"] {
				if x != "" && strings.HasPrefix(c.sha, x) {
					return c
				}
			}
			return nil
		}
		a, b := resolve(from), resolve(to)
		if a == nil || b == nil {
			w.WriteHeader(404)
			return
		}
		var between []*fakeCommit // newest first
		on := false
		for _, c := range g.branches["main"] {
			if c == b {
				on = true
			}
			if c == a {
				break
			}
			if on {
				between = append(between, c)
			}
		}
		var l []map[string]interface{}
		for i := len(between) - 1; i >= 0; i-- {
			c := between[i]
			if g.kind == "gitlab" {
				l = append(l, map[string]interface{}{"id": c.sha, "title": "change " + c.date, "author_name": "Dev", "committed_date": c.date + "T10:00:00Z"})
			} else {
				l = append(l, map[string]interface{}{"sha": c.sha, "commit": map[string]interface{}{"message": "change " + c.date + "\n\nbody", "author": map[string]string{"name": "Dev", "date": c.date + "T10:00:00Z"}}})
			}
		}
		if g.kind == "gitlab" {
			out(map[string]interface{}{"commits": l})
		} else {
			out(map[string]interface{}{"total_commits": len(l), "commits": l})
		}
	case strings.HasPrefix(rest, "/blobs/"):
		g.blobReqs.Add(1)
		sha := strings.TrimSuffix(strings.TrimPrefix(rest, "/blobs/"), "/raw")
		v, ok := g.blobs[sha]
		if !ok {
			w.WriteHeader(404)
			return
		}
		if g.kind == "gitlab" {
			w.Write([]byte(v))
		} else {
			out(map[string]interface{}{"content": base64.StdEncoding.EncodeToString([]byte(v)), "encoding": "base64"})
		}
	default:
		w.WriteHeader(404)
	}
}

func demoCatalog(url string) map[string]interface{} {
	return map[string]interface{}{
		"gitlab": map[string]interface{}{"url": url, "groups": []string{"demo"}}, "fallback_branches": []string{"main"},
		"server_roots": []string{"/opt"},
		"apps": []map[string]interface{}{{"name": "demo-api", "project": "demo/demo-api", "ref": "tag:latest", "branch": "main", "tag_filter": `^v\d`,
			"subdir": "linux", "kind": "app", "include": []string{}, "exclude": []string{"*.md"}, "protected": []string{"config*"},
			"fingerprint": []string{"run.py", "lib/util.py"}, "install_hint": []string{"demo-api"}}},
	}
}

type gitStateResp struct {
	Catalog  gitCatalog       `json:"catalog"`
	Settings gitSettings      `json:"settings"`
	Sources  []gitSource      `json:"sources"`
	Targets  []gitTargetBrief `json:"targets"`
	Installs []gitInstall     `json:"installs"`
}

func setupGitUser(t *testing.T, srv *httptest.Server, name string, g *fakeGit) *testClient {
	u := newTestUser(t, srv, name, false)
	if code := u.jsonDo("PUT", "/api/git/catalog", demoCatalog(g.srv.URL), nil); code != 200 {
		t.Fatalf("catalog: %d", code)
	}
	if code := u.jsonDo("POST", "/api/git/sources", map[string]interface{}{"kind": g.kind, "name": "demo " + g.kind, "url": g.srv.URL, "token": g.token}, nil); code != 200 {
		t.Fatalf("source: %d", code)
	}
	return u
}

func refreshDemo(t *testing.T, u *testClient) gitTarget {
	t.Helper()
	if code := u.jsonDo("POST", "/api/git/targets/refresh", map[string]interface{}{}, nil); code != 200 {
		t.Fatalf("refresh: %d", code)
	}
	var tg gitTarget
	if code := u.jsonDo("GET", "/api/git/targets/demo-api", nil, &tg); code != 200 {
		t.Fatalf("target: %d", code)
	}
	return tg
}

func TestGitHelpers(t *testing.T) {
	if compareVersions("v1.10", "v1.9") <= 0 || compareVersions("v2.0", "v1.10.3") <= 0 || compareVersions("v1.2", "v1.2") != 0 {
		t.Fatal("version sort")
	}
	for _, c := range []struct {
		name, pat string
		want      bool
	}{{"config.ini", "config*", true}, {"a/b/config.ini", "*.ini", true}, {"x.py", "[!a]*.py", true}, {"a.py", "[!a]*.py", false}, {"a/b", "a*", true}} {
		if fnmatch(c.name, c.pat) != c.want {
			t.Errorf("fnmatch(%q, %q)", c.name, c.pat)
		}
	}
	w := strings.Split(defaultBackupWords, ",")
	for _, s := range []string{"App_BKP", "backup_2024", ".deploy-bak", "App.old", "kopija", "bkp2024"} {
		if !looksLikeBackup(s, w) {
			t.Errorf("%s not a backup", s)
		}
	}
	for _, s := range []string{"App_prod", "golden", "older-api", "demo-api"} {
		if looksLikeBackup(s, w) {
			t.Errorf("%s taken for a backup", s)
		}
	}
	if envLabel("/srv/scripts", "/srv/scripts/test/App") != "test" || envLabel("/opt", "/opt/App_prod") != "prod" || envLabel("/opt", "/opt/App") != "" {
		t.Fatal("env label")
	}
	if normHash([]byte("a\r\nb\n")) != normHash([]byte("a\nb\n")) || trHash([]byte("a\rb")) != normHash([]byte("ab")) {
		t.Fatal("hashes")
	}
	v := parseVersionMD(strings.Split("# demo-api - v1.6.7\n\n- **Servis**:      demo-api\n- **Verzija**:     v1.6.7\n- **Grana/ref**:   main (tag)\n- **Bundle**:      20261005-1200\n", "\n"))
	if v.Version != "v1.6.7" || v.App != "demo-api" || v.Ref != "main (tag)" || v.Bundle != "20261005-1200" {
		t.Fatalf("VERSION.md: %+v", v)
	}
	if v := parseVersionMD([]string{"# demo-api - v2.1"}); v.Version != "v2.1" {
		t.Fatalf("title fallback: %+v", v)
	}
	lines, _ := unifiedDiff([]string{"a", "b", "c"}, []string{"a", "x", "c"}, 1)
	ops := ""
	for _, l := range lines {
		ops += l.Op
	}
	if ops != "@ -+ " {
		t.Fatalf("diff ops %q", ops)
	}
}

func TestGitTargets(t *testing.T) {
	srv := newTestServer(t)
	for _, kind := range []string{"gitlab", "github", "gitea"} {
		t.Run(kind, func(t *testing.T) {
			g := newFakeGit(t, kind)
			u := setupGitUser(t, srv, "git-"+kind, g)
			var st gitStateResp
			u.jsonDo("GET", "/api/git", nil, &st)
			if len(st.Sources) != 1 || !st.Sources[0].HasToken {
				t.Fatalf("sources: %+v", st.Sources)
			}
			var raw string
			db.QueryRow(`SELECT token FROM git_sources WHERE id=?`, st.Sources[0].ID).Scan(&raw)
			if strings.Contains(raw, g.token) {
				t.Fatal("token stored in plain text")
			}
			var pr struct{ Projects []string }
			if code := u.jsonDo("POST", fmt.Sprintf("/api/git/sources/%d/test", st.Sources[0].ID), nil, &pr); code != 200 || len(pr.Projects) != 1 || pr.Projects[0] != "demo/demo-api" {
				t.Fatalf("projects: %d %+v", code, pr)
			}
			tg := refreshDemo(t, u)
			// v1.10 > v1.9 numerically; v2.0 is not on main
			if tg.Error != "" || tg.Version != "v1.10" || tg.LatestTag != "v1.10" || tg.RefKind != "tag" || tg.Commit != commitSHA("c2")[:10] || tg.CommitDate != "2026-09-10" {
				t.Fatalf("target: %+v", tg)
			}
			if _, ok := tg.Files["README.md"]; ok || len(tg.Files) != 3 {
				t.Fatalf("files: %v", tg.Files)
			}
			run := tg.Files["run.py"]
			if run.Hash != normHash([]byte(runV110)) || len(run.History) != 2 || run.History[1].Tag != "v1.9" || run.History[1].Hash != normHash([]byte(runV19)) {
				t.Fatalf("history: %+v", run)
			}
			if len(tg.Files["lib/util.py"].History) != 1 {
				t.Fatal("duplicate hashes in history")
			}
			blobs, trees := g.blobReqs.Load(), g.treeReqs.Load()
			refreshDemo(t, u)
			if g.blobReqs.Load() != blobs || g.treeReqs.Load() != trees {
				t.Fatalf("cache not used: blobs %d→%d trees %d→%d", blobs, g.blobReqs.Load(), trees, g.treeReqs.Load())
			}
			// one branch for all: head of main
			if code := u.jsonDo("PUT", "/api/git/settings", map[string]interface{}{"ref_mode": "branch", "ref_branch": "main"}, nil); code != 200 {
				t.Fatal("settings")
			}
			tg = refreshDemo(t, u)
			if tg.RefKind != "branch" || tg.Version != commitSHA("c4")[:10] || tg.LatestTag != "v1.10" || len(tg.Files["run.py"].History) != 3 {
				t.Fatalf("branch target: %+v", tg)
			}
			// a missing branch falls back with a warning; a filter without tags uses the head
			u.jsonDo("PUT", "/api/git/settings", map[string]interface{}{"ref_mode": "catalog", "ref_overrides": map[string]string{"demo-api": "release-x"}}, nil)
			tg = refreshDemo(t, u)
			if tg.Branch != "main" || len(tg.Warnings) == 0 {
				t.Fatalf("fallback: %+v", tg.Warnings)
			}
			cat := demoCatalog(g.srv.URL)
			cat["apps"].([]map[string]interface{})[0]["tag_filter"] = "^release-"
			u.jsonDo("PUT", "/api/git/catalog", cat, nil)
			u.jsonDo("PUT", "/api/git/settings", map[string]interface{}{"ref_mode": "catalog"}, nil)
			tg = refreshDemo(t, u)
			if tg.RefKind != "branch" || tg.Version != commitSHA("c4")[:10] || len(tg.Warnings) == 0 {
				t.Fatalf("no tag: %+v", tg)
			}
			// a suggestion from the repository tree
			var sg struct {
				App gitCatalogApp `json:"app"`
			}
			if code := u.jsonDo("POST", "/api/git/suggest", map[string]string{"project": "demo/demo-api"}, &sg); code != 200 || sg.App.Subdir != "linux" ||
				len(sg.App.Fingerprint) == 0 || sg.App.Fingerprint[0] != "run.py" || !contains(sg.App.Protected, "config*") {
				t.Fatalf("suggest: %d %+v", code, sg.App)
			}
			// a wrong token is reported
			db.Exec(`UPDATE git_sources SET token=? WHERE id=?`, encryptValue("bad"), st.Sources[0].ID)
			if code := u.jsonDo("POST", fmt.Sprintf("/api/git/sources/%d/test", st.Sources[0].ID), nil, nil); code != 502 {
				t.Fatalf("bad token: %d", code)
			}
		})
	}
}

func readBundle(t *testing.T, data []byte) (string, map[string][]byte) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	top := ""
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		b, _ := io.ReadAll(tr)
		parts := strings.SplitN(h.Name, "/", 2)
		top = parts[0]
		files[parts[1]] = b
	}
	return top, files
}

func TestGitBundleRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	g := newFakeGit(t, "gitlab")
	u := setupGitUser(t, srv, "git-bundler", g)
	refreshDemo(t, u)
	b, _ := json.Marshal(map[string]interface{}{"apps": []string{"demo-api"}})
	resp := u.do("POST", "/api/git/bundles/export", bytes.NewReader(b), "application/json")
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("export: %d %s", resp.StatusCode, data)
	}
	top, files := readBundle(t, data)
	if !strings.HasPrefix(top, "wrm-bundle-") {
		t.Fatalf("top %q", top)
	}
	if _, ok := files["payload/demo-api/config.ini"]; ok {
		t.Fatal("protected file in the payload")
	}
	if string(files["payload/demo-api/run.py"]) != runV110 {
		t.Fatal("payload")
	}
	mj := string(files["bundle.json"])
	if !strings.HasPrefix(mj, "{\n \"apps\": {\n  \"demo-api\": {") || strings.Contains(mj, "hash2") || strings.Contains(mj, "blob") {
		t.Fatalf("bundle.json format:\n%s", mj[:200])
	}
	var m gitBundleManifest
	json.Unmarshal(files["bundle.json"], &m)
	a := m.Apps["demo-api"]
	if m.Source != "gitlab:"+g.srv.URL || a.Version != "v1.10" || a.LatestTag == nil || *a.LatestTag != "v1.10" || a.Commit != commitSHA("c2")[:10] ||
		a.Files["config.ini"].Hash == "" || len(a.Files["run.py"].History) != 2 || m.HistoryDepth != gitDefaultDepth || m.Groups[0] != "demo" {
		t.Fatalf("manifest: %+v", m)
	}

	// import as another user without any Git source: the bundle alone is enough
	other := newTestUser(t, srv, "git-offline", false)
	resp = other.do("POST", "/api/git/bundles", bytes.NewReader(data), "application/gzip")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("import: %d %s", resp.StatusCode, body)
	}
	var st gitStateResp
	other.jsonDo("GET", "/api/git", nil, &st)
	if len(st.Sources) != 1 || st.Sources[0].Kind != "bundle" || st.Sources[0].BundleID != m.BundleID || st.Sources[0].Origin != m.Source {
		t.Fatalf("bundle source: %+v", st.Sources)
	}
	tg := refreshDemo(t, other)
	if tg.Version != "v1.10" || tg.Files["run.py"].Hash != normHash([]byte(runV110)) || tg.Files["run.py"].Blob == "" || tg.Files["config.ini"].Blob != "" {
		t.Fatalf("bundle target: %+v", tg)
	}
	// importing the same bundle again replaces it
	other.do("POST", "/api/git/bundles", bytes.NewReader(data), "application/gzip").Body.Close()
	other.jsonDo("GET", "/api/git", nil, &st)
	if len(st.Sources) != 1 {
		t.Fatalf("duplicate bundle: %d", len(st.Sources))
	}
	// unsafe or broken archives are refused
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "x/../../etc/passwd", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	tw.Write([]byte("x"))
	tw.Close()
	gz.Close()
	for _, bad := range [][]byte{buf.Bytes(), []byte("not gzip")} {
		if r := other.do("POST", "/api/git/bundles", bytes.NewReader(bad), "application/gzip"); r.StatusCode != 400 {
			t.Fatalf("bad archive: %d", r.StatusCode)
		}
	}
}

func TestGitCatalogImportExport(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "git-catalog", false)
	in := demoCatalog("https://git.example.com")
	in["ignore_dirs"] = defaultIgnoreDirs
	if code := u.jsonDo("POST", "/api/git/catalog/import", in, nil); code != 200 {
		t.Fatalf("import: %d", code)
	}
	resp := u.do("GET", "/api/git/catalog/export", nil, "")
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var out, want map[string]interface{}
	json.Unmarshal(data, &out)
	wb, _ := json.Marshal(in)
	json.Unmarshal(wb, &want)
	ob, _ := json.Marshal(out)
	wb, _ = json.Marshal(want)
	if string(ob) != string(wb) {
		t.Fatalf("round trip:\n%s\n%s", ob, wb)
	}
	// merge keeps other services; invalid entries are refused
	extra := map[string]interface{}{"apps": []map[string]interface{}{{"name": "demo-web", "project": "demo/demo-web", "ref": "main"}}}
	var st gitStateResp
	if code := u.jsonDo("POST", "/api/git/catalog/import?mode=merge", extra, &st); code != 200 || len(st.Catalog.Apps) != 2 {
		t.Fatalf("merge: %d %d", code, len(st.Catalog.Apps))
	}
	for _, bad := range []map[string]interface{}{
		{"apps": []map[string]interface{}{{"name": "../x", "project": "a/b"}}},
		{"apps": []map[string]interface{}{{"name": "x", "project": "a/b", "tag_filter": "("}}},
		{"server_roots": []string{"relative"}},
	} {
		if code := u.jsonDo("PUT", "/api/git/catalog", bad, nil); code != 400 {
			t.Fatalf("bad catalog accepted: %v", bad)
		}
	}
	if code := u.jsonDo("DELETE", "/api/git/catalog/apps/demo-web", nil, &st); code != 200 || len(st.Catalog.Apps) != 1 {
		t.Fatal("delete service")
	}
}

func writeFile(t *testing.T, p, content string) {
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGitDiscoveryCompareAndNotify(t *testing.T) {
	srv := newTestServer(t)
	g := newFakeGit(t, "gitlab")
	u := setupGitUser(t, srv, "git-scan", g)
	admin := newTestUser(t, srv, "git-scan-admin", true)
	h := startFakeHost(t, testSSHUser, testSSHPass)
	conn := u.addConnection("app-01", h.addr)
	home := h.home
	opt, scripts := filepath.Join(home, "opt"), filepath.Join(home, "srv", "scripts")
	prod := filepath.Join(opt, "demo-api_prod")
	writeFile(t, prod+"/run.py", runV19) // one version behind
	writeFile(t, prod+"/lib/util.py", utilV1)
	writeFile(t, prod+"/config.ini", "[main]\nhost=prod.example.com\n")
	writeFile(t, prod+"/extra.txt", "only here")
	writeFile(t, prod+"/VERSION.md", "# demo-api - v1.9\n\n- **Servis**:      demo-api\n- **Verzija**:     v1.9\n")
	writeFile(t, prod+"/.deploy-bak/20260101/run.py", "old")
	test := filepath.Join(scripts, "test", "demo-api")
	writeFile(t, test+"/run.py", runV110)
	writeFile(t, test+"/lib/util.py", strings.ReplaceAll(utilV1, "\n", "\r\n")) // CRLF only: still ok
	writeFile(t, filepath.Join(opt, "demo-api_BKP")+"/run.py", runV19)
	writeFile(t, filepath.Join(opt, "demo-api_BKP")+"/lib/util.py", utilV1)
	units := t.TempDir()
	writeFile(t, units+"/demo-api.service", "[Service]\nWorkingDirectory="+prod+"\nExecStart="+prod+"/run.py\n")
	writeFile(t, units+"/other.service", "[Service]\nWorkingDirectory="+prod+"_old\n")
	oldSys, oldSup := gitSystemdDirs, gitSupervisorDirs
	gitSystemdDirs, gitSupervisorDirs = []string{units}, []string{units}
	defer func() { gitSystemdDirs, gitSupervisorDirs = oldSys, oldSup }()

	cat := demoCatalog(g.srv.URL)
	cat["server_roots"] = []string{opt, scripts}
	u.jsonDo("PUT", "/api/git/catalog", cat, nil)
	if code := u.jsonDo("POST", "/api/git/check", map[string]interface{}{"conn_ids": []int{conn}, "discover": true, "refresh": true}, nil); code != 200 {
		t.Fatalf("check: %d", code)
	}
	var st gitStateResp
	u.jsonDo("GET", "/api/git", nil, &st)
	if len(st.Installs) != 2 {
		t.Fatalf("installs: %+v", st.Installs)
	}
	byPath := map[string]gitInstall{}
	for _, in := range st.Installs {
		byPath[in.Path] = in
	}
	p, ts := byPath[prod], byPath[test]
	if p.Env != "prod" || p.State != "update" || p.Behind != 1 || p.VersionMD.Version != "v1.9" || p.Target != "v1.10" ||
		len(p.Units) != 1 || p.Units[0].Name != "demo-api.service" || p.Counts["extra"] != 1 || p.Counts["protected"] != 1 {
		t.Fatalf("prod: %+v", p)
	}
	if ts.Env != "test" || ts.State != "ok" || ts.VersionMD.Version != "" {
		t.Fatalf("test: %+v", ts)
	}
	var det gitInstall
	u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d", p.ID), nil, &det)
	states := map[string]string{}
	for _, f := range det.Files {
		states[f.Path] = f.State
	}
	if states["run.py"] != "old" || states["lib/util.py"] != "ok" || states["config.ini"] != "protected" || states["extra.txt"] != "extra" || len(states) != 4 {
		t.Fatalf("file states: %v", states)
	}

	// subscribe to Git events, then a hand edit on the server is drift
	hook := newFakeHook(t)
	ch := createChannel(t, admin, map[string]interface{}{"name": "Git hook", "kind": "slack", "config": map[string]string{"webhook_url": hook.srv.URL + "/g"}})
	defer db.Exec(`DELETE FROM notify_channels WHERE id=?`, ch.ID)
	subs := []map[string]interface{}{}
	for _, e := range []string{"git.new_version", "git.drift", "git.unreachable"} {
		subs = append(subs, map[string]interface{}{"event": e, "channel_id": ch.ID})
	}
	if code := u.jsonDo("PUT", "/api/notify", map[string]interface{}{"subscriptions": subs}, nil); code != 200 {
		t.Fatalf("subscribe: %d", code)
	}
	writeFile(t, prod+"/run.py", "print('hot fix')\n")
	g.mu.Lock() // a new release on main
	c5 := &fakeCommit{sha: commitSHA("c5"), date: "2026-10-05", files: map[string]string{"linux/run.py": "print('v1.11')\n", "linux/lib/util.py": utilV1, "linux/config.ini": "x"}}
	for _, v := range c5.files {
		g.blobs[blobSHA(v)] = v
	}
	g.branches["main"] = append([]*fakeCommit{c5}, g.branches["main"]...)
	g.tags["v1.11"] = c5
	g.mu.Unlock()
	res := runGitRound(u.userID, []int{conn}, true, true, true)
	if len(res.Changes) != 1 || res.Changes[0].Kind != "drift" {
		t.Fatalf("changes: %+v", res.Changes)
	}
	events := map[string]string{}
	rows, _ := db.Query(`SELECT event, title FROM notify_pending WHERE user_id=?`, u.userID)
	for rows.Next() {
		var e, title string
		rows.Scan(&e, &title)
		events[e] = title
	}
	rows.Close()
	if !strings.Contains(events["git.drift"], "demo-api") || !strings.Contains(events["git.new_version"], "v1.11") {
		t.Fatalf("events: %v", events)
	}
	// diff of the modified file: server → target
	u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d", p.ID), nil, &det)
	var df struct {
		Lines []diffLine `json:"lines"`
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/diff?path=run.py", p.ID), nil, &df); code != 200 || len(df.Lines) != 3 ||
		df.Lines[1].S != "print('hot fix')" || df.Lines[2].S != "print('v1.11')" {
		t.Fatalf("diff: %d %+v", code, df.Lines)
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/diff?path=config.ini", p.ID), nil, nil); code != 400 {
		t.Fatalf("protected diff: %d", code)
	}
	// an unreachable server is reported once
	db.Exec(`UPDATE connections SET host='127.0.0.1:1' WHERE id=?`, conn)
	db.Exec(`DELETE FROM notify_pending WHERE user_id=?`, u.userID)
	runGitRound(u.userID, []int{conn}, true, false, true)
	runGitRound(u.userID, []int{conn}, true, false, true)
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM notify_pending WHERE user_id=? AND event='git.unreachable'`, u.userID).Scan(&n)
	if n != 1 {
		t.Fatalf("unreachable notifications: %d", n)
	}
	db.Exec(`DELETE FROM notify_pending WHERE user_id=?`, u.userID)

	// other users cannot see or use the installation
	other := newTestUser(t, srv, "git-scan-other", false)
	if code := other.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d", p.ID), nil, nil); code != 404 {
		t.Fatalf("foreign install: %d", code)
	}
	if code := other.jsonDo("POST", "/api/git/check", map[string]interface{}{"conn_ids": []int{conn}}, nil); code != 404 {
		t.Fatalf("foreign server: %d", code)
	}
	// policies
	setSetting("git_checks", "admins")
	if code := u.jsonDo("POST", "/api/git/check", map[string]interface{}{"conn_ids": []int{conn}}, nil); code != 403 {
		t.Fatalf("checks policy: %d", code)
	}
	setSetting("git_checks", "all")
	setSetting("git_enabled", "0")
	if code := u.jsonDo("GET", "/api/git", nil, nil); code != 403 {
		t.Fatalf("git_enabled: %d", code)
	}
	var me map[string]interface{}
	u.jsonDo("GET", "/api/auth/me", nil, &me)
	if pol, _ := me["policy"].(map[string]interface{}); pol["git"] != false {
		t.Fatalf("policy flag: %v", me["policy"])
	}
	setSetting("git_enabled", "1")
	var acts []string
	rows, _ = db.Query(`SELECT DISTINCT action FROM audit_log WHERE action LIKE 'git.%'`)
	for rows.Next() {
		var a string
		rows.Scan(&a)
		acts = append(acts, a)
	}
	rows.Close()
	sort.Strings(acts)
	if !contains(acts, "git.checked") || !contains(acts, "git.diff_viewed") || !contains(acts, "git.source_added") {
		t.Fatalf("audit: %v", acts)
	}
	var leaked int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE details LIKE ?`, "%"+g.token+"%").Scan(&leaked)
	if leaked > 0 {
		t.Fatal("token in the audit log")
	}
}
