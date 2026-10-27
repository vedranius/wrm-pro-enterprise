// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ─── Medium screens: file details, line endings and the deploy-config editor ───

func TestMediumFileDiff(t *testing.T) {
	lines, o, n, _, add, rem := mdFileDiff([]byte("a\nb\nc\n"), 6, []byte("a\nB\nc\n"), 6)
	if o.State != "text" || n.State != "text" || add != 1 || rem != 1 || len(lines) == 0 || lines[0].Op != "@" {
		t.Fatalf("changed: %v %v %d %d %+v", o, n, add, rem, lines)
	}
	if _, o, n, _, add, rem := mdFileDiff([]byte("a\nb\n"), 4, nil, 0); o.State != "text" || n.State != "missing" || add != 0 || rem != 2 {
		t.Fatalf("deleted file: %v %v %d %d", o, n, add, rem)
	}
	if lines, o, _, _, _, _ := mdFileDiff([]byte("a\x00b"), 3, []byte("x"), 1); o.State != "binary" || len(lines) != 0 {
		t.Fatalf("binary: %v", o)
	}
	if _, o, n, _, add, rem := mdFileDiff([]byte("a\r\nb\r\n"), 6, []byte("a\nb\n"), 4); !o.CRLF || n.CRLF || add+rem != 0 {
		t.Fatalf("line endings only: %v %v %d %d", o, n, add, rem)
	}
	if _, _, n, _, _, _ := mdFileDiff(nil, 0, make([]byte, mdDiffMax+1), mdDiffMax+1); n.State != "too_large" {
		t.Fatalf("too large: %v", n)
	}
}

func TestMediumEOLOnly(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "md-eol-user", true)
	e := newMdDeployEnv(t, u.userID, mdDeployProject)
	mdMk(t, e.dir("a"), map[string]string{"crlf.php": "a\r\nb\r\n", "real.php": "a\r\nb\r\n", "same.php": "s", "bin.dat": "x\x00\r\n"})
	e.setSource(map[string]string{"crlf.php": "a\nb\n", "real.php": "a\nc\n", "same.php": "s", "bin.dat": "x\x00\n"})
	res, tree := e.plan("dev", nil)
	sp := res.Servers[0]
	marked := map[string]bool{}
	for _, c := range sp.Changes {
		if c.EOL {
			marked[c.Path] = true
		}
	}
	if !marked["crlf.php"] || marked["real.php"] || marked["bin.dat"] || len(marked) != 1 || sp.Counts["eol"] != 1 || sp.Counts["changed"] != 3 {
		t.Fatalf("marked %v counts %v changes %+v", marked, sp.Counts, sp.Changes)
	}
	// the deployment still sends the file, and history counts it like the original tool
	j := e.deploy(res, tree, false)
	mdWantStatus(t, j, "ok")
	if b, _ := os.ReadFile(filepath.Join(e.dir("a"), "crlf.php")); string(b) != "a\nb\n" {
		t.Fatalf("crlf.php after the deployment: %q", b)
	}
	b, _ := os.ReadFile(filepath.Join(filepath.Dir(e.dir("a")), ".deploy", "demo-api", "history.jsonl"))
	if !strings.Contains(string(b), `"eol":1`) {
		t.Fatalf("history counts: %s", b)
	}
}

func TestMediumEOLOverSSH(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "md-eol-ssh", true)
	h := startFakeHost(t, testSSHUser, testSSHPass)
	conn := u.addConnection("demo eol", h.addr)
	e := newMdDeployEnv(t, u.userID, strings.Replace(mdDeployProject, "{srv}", filepath.ToSlash(filepath.Join(h.home, "srv")), -1))
	mdOpenHost = func(uid int, cfg *mdConfig, server string, fn func(mdHost, string) error) error {
		c, err := loadConnection(conn)
		if err != nil {
			return err
		}
		p, err := acquireSFTP(c)
		if err != nil {
			return err
		}
		err = fn(mdSFTP{cl: p.sftp, ssh: p.ssh}, c.Name)
		releaseSFTP(p, err)
		return err
	}
	a := filepath.Join(h.home, "srv", "a", "demo-api")
	mdMk(t, a, map[string]string{"x.php": "1\r\n2\r\n", "ime s razmakom.txt": "q\r\n", "y.php": "1\r\n"})
	e.setSource(map[string]string{"x.php": "1\n2\n", "ime s razmakom.txt": "q\n", "y.php": "2\n"})
	res, _ := e.plan("dev", nil)
	sp := res.Servers[0]
	if sp.Counts["eol"] != 2 {
		t.Fatalf("eol over SSH: %v %+v %v", sp.Counts, sp.Changes, sp.Warnings)
	}
}

func TestMediumDiffAPI(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "md-diff-admin", true)
	other := newTestUser(t, srv, "md-diff-other", true)
	e := newMdDeployEnv(t, admin.userID, mdDeployProject)
	mdMk(t, e.dir("a"), map[string]string{"index.php": "<?php\necho 1;\n", "gone.php": "bye\n", "crlf.txt": "a\r\n"})
	e.setSource(map[string]string{"index.php": "<?php\necho 2;\n", "new.php": "hi\n", "crlf.txt": "a\n"})
	res, tree := e.plan("dev", nil)
	run := &mdRun{ID: "md-diff-run-1", Done: true, Result: res, uid: admin.userID, at: time.Now(), src: tree}
	mdRuns.Lock()
	mdRuns.m[run.ID] = run
	mdRuns.Unlock()
	t.Cleanup(func() {
		mdRuns.Lock()
		delete(mdRuns.m, run.ID)
		mdRuns.Unlock()
	})
	get := func(u *testClient, path string, out interface{}) int {
		return u.jsonDo("GET", "/api/git/medium/native/plan/"+run.ID+"/diff?server=app-01&path="+url.QueryEscape(path), nil, out)
	}
	var v mdDiffView
	if code := get(admin, "index.php", &v); code != 200 || v.Added != 1 || v.Removed != 1 || len(v.Lines) != 4 || v.Old.State != "text" || v.New.State != "text" {
		t.Fatalf("changed file: %d %+v", code, v)
	}
	v = mdDiffView{}
	if code := get(admin, "new.php", &v); code != 200 || v.Old.State != "missing" || v.Added != 1 {
		t.Fatalf("new file: %d %+v", code, v)
	}
	v = mdDiffView{}
	if code := get(admin, "gone.php", &v); code != 200 || v.New.State != "missing" || v.Removed != 1 {
		t.Fatalf("deleted file: %d %+v", code, v)
	}
	v = mdDiffView{}
	if code := get(admin, "crlf.txt", &v); code != 200 || !v.EOL || !v.Old.CRLF || v.New.CRLF {
		t.Fatalf("line endings: %d %+v", code, v)
	}
	for _, p := range []string{"../etc/passwd", "same.php", "/etc/passwd"} {
		if code := get(admin, p, nil); code != 400 {
			t.Fatalf("%s: %d", p, code)
		}
	}
	if code := get(other, "index.php", nil); code != 404 {
		t.Fatalf("another user's plan: %d", code)
	}
	if code := admin.jsonDo("GET", "/api/git/medium/native/plan/"+run.ID+"/diff?server=nope&path=index.php", nil, nil); code != 400 {
		t.Fatalf("unknown server: %d", code)
	}
}

func TestMediumConfigEditor(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "md-cfg-admin", true)
	user := newTestUser(t, srv, "md-cfg-user", false)
	e := newMdDeployEnv(t, admin.userID, mdDeployProject)
	var list struct {
		Files   []mdConfigFile `json:"files"`
		Valid   bool           `json:"valid"`
		CanEdit bool           `json:"can_edit"`
	}
	if code := admin.jsonDo("GET", "/api/git/medium/native/config/files", nil, &list); code != 200 || !list.Valid || !list.CanEdit || len(list.Files) != 3 {
		t.Fatalf("files: %d %+v", code, list)
	}
	var f struct {
		Text string `json:"text"`
		Hash string `json:"hash"`
	}
	if code := admin.jsonDo("GET", "/api/git/medium/native/config/file?name=projects/demo-api.yaml", nil, &f); code != 200 || !strings.Contains(f.Text, "name: demo-api") {
		t.Fatalf("file: %d %+v", code, f)
	}
	for _, n := range []string{"../servers.yaml", "projects/../servers.yaml", "secrets.txt", "projects/.hidden.yaml"} {
		if code := admin.jsonDo("GET", "/api/git/medium/native/config/file?name="+url.QueryEscape(n), nil, nil); code != 400 {
			t.Fatalf("%s: %d", n, code)
		}
	}
	file := filepath.Join(e.cfgDir, "projects", "demo-api.yaml")
	orig, _ := os.ReadFile(file)
	save := func(u *testClient, body map[string]interface{}, out interface{}) int {
		return u.jsonDo("POST", "/api/git/medium/native/config/files", body, out)
	}
	var res map[string]interface{}
	// an edit that breaks the configuration is refused and nothing is written
	if code := save(admin, map[string]interface{}{"name": "projects/demo-api.yaml", "text": "name: demo-api\ntargets: 7\n", "base_hash": f.Hash}, &res); code != 200 || res["ok"] != false || res["error"] == "" {
		t.Fatalf("invalid edit: %d %v", code, res)
	}
	if b, _ := os.ReadFile(file); string(b) != string(orig) {
		t.Fatal("an invalid edit was written")
	}
	edited := "# keep this comment\n" + string(orig)
	if code := save(admin, map[string]interface{}{"name": "projects/demo-api.yaml", "text": edited, "base_hash": f.Hash, "check_only": true}, &res); code != 200 || res["ok"] != true || res["checked"] != true {
		t.Fatalf("check: %d %v", code, res)
	}
	if b, _ := os.ReadFile(file); string(b) != string(orig) {
		t.Fatal("a check wrote the file")
	}
	if code := save(user, map[string]interface{}{"name": "projects/demo-api.yaml", "text": edited, "base_hash": f.Hash}, nil); code != 403 && code != 404 {
		t.Fatalf("a user without medium_native: %d", code)
	}
	if code := save(admin, map[string]interface{}{"name": "projects/demo-api.yaml", "text": edited, "base_hash": "stale"}, &res); code != 409 {
		t.Fatalf("stale base: %d %v", code, res)
	}
	if code := save(admin, map[string]interface{}{"name": "projects/demo-api.yaml", "text": edited, "base_hash": f.Hash}, &res); code != 200 || res["ok"] != true || res["action"] != "saved" {
		t.Fatalf("save: %d %v", code, res)
	}
	if b, _ := os.ReadFile(file); string(b) != edited {
		t.Fatalf("saved text: %q", b)
	}
	// CRLF files keep CRLF
	crlf := strings.ReplaceAll(mdServersYAML, "\n", "\r\n")
	os.WriteFile(filepath.Join(e.cfgDir, "servers.yaml"), []byte(crlf), 0o600)
	admin.jsonDo("GET", "/api/git/medium/native/config/file?name=servers.yaml", nil, &f)
	if code := save(admin, map[string]interface{}{"name": "servers.yaml", "text": "# x\n" + mdServersYAML, "base_hash": f.Hash}, &res); code != 200 || res["ok"] != true {
		t.Fatalf("crlf save: %d %v", code, res)
	}
	if b, _ := os.ReadFile(filepath.Join(e.cfgDir, "servers.yaml")); string(b) != "# x\r\n"+crlf {
		t.Fatalf("crlf kept: %q", b)
	}
	// a new project, then deleting it
	np := "name: demo-web\nrepo: git@git.example.com:team/demo-web.git\ntargets:\n  dev: app-01:/srv/x/demo-web\n"
	if code := save(admin, map[string]interface{}{"name": "projects/demo-web.yaml", "text": np, "base_hash": "x"}, &res); code != 409 {
		t.Fatalf("new file with a base: %d %v", code, res)
	}
	if code := save(admin, map[string]interface{}{"name": "projects/demo-web.yaml", "text": np}, &res); code != 200 || res["action"] != "created" {
		t.Fatalf("create: %d %v", code, res)
	}
	if code := save(admin, map[string]interface{}{"name": "projects/demo-web.yaml", "text": np}, &res); code != 409 {
		t.Fatalf("create twice: %d %v", code, res)
	}
	admin.jsonDo("GET", "/api/git/medium/native/config/file?name=projects/demo-web.yaml", nil, &f)
	if code := save(admin, map[string]interface{}{"name": "servers.yaml", "delete": true, "base_hash": f.Hash}, &res); code != 400 {
		t.Fatalf("delete servers.yaml: %d %v", code, res)
	}
	if code := save(admin, map[string]interface{}{"name": "projects/demo-web.yaml", "delete": true, "base_hash": f.Hash}, &res); code != 200 || res["action"] != "deleted" {
		t.Fatalf("delete: %d %v", code, res)
	}
	if _, err := os.Stat(filepath.Join(e.cfgDir, "projects", "demo-web.yaml")); !os.IsNotExist(err) {
		t.Fatal("the project file is still there")
	}
}

func TestMediumConfigEditorLinks(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "md-cfg-links", true)
	e := newMdDeployEnv(t, admin.userID, mdDeployProject)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "x.yaml"), []byte("secret: 1\n"), 0o600)
	os.Symlink(filepath.Join(outside, "x.yaml"), filepath.Join(e.cfgDir, "projects", "link.yaml"))
	if code := admin.jsonDo("GET", "/api/git/medium/native/config/file?name=projects/link.yaml", nil, nil); code != 404 {
		t.Fatalf("a linked file is read: %d", code)
	}
	// a projects directory that leads outside is never written through
	os.RemoveAll(filepath.Join(e.cfgDir, "projects"))
	os.Symlink(outside, filepath.Join(e.cfgDir, "projects"))
	var res map[string]interface{}
	admin.jsonDo("POST", "/api/git/medium/native/config/files", map[string]interface{}{"name": "projects/new.yaml", "text": "name: new\n"}, &res)
	if _, err := os.Stat(filepath.Join(outside, "new.yaml")); err == nil {
		t.Fatalf("written outside the configuration: %v", res)
	}
}
