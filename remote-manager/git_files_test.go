// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	setupGit    = "from setuptools import setup\nsetup(name='demo-api', version='1.10')\n"
	setupServer = "from setuptools import setup\nsetup(name='demo-api', version='1.9')\n"
)

// addRepoFile adds a file to every commit of the fake repository.
func addRepoFile(g *fakeGit, p, content string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, list := range g.branches {
		for _, c := range list {
			c.files[p] = content
		}
	}
	g.blobs[blobSHA(content)] = content
}

func TestGitInstallFilesAndDiff(t *testing.T) {
	for _, kind := range []string{"gitlab", "github"} {
		t.Run(kind, func(t *testing.T) {
			srv := newTestServer(t)
			g := newFakeGit(t, kind)
			addRepoFile(g, "linux/setup.py", setupGit)
			addRepoFile(g, "linux/tests/test_run.py", "def test():\n    pass\n")
			addRepoFile(g, "linux/logs/committed.log", "committed by mistake\n")
			u := setupGitUser(t, srv, "git-files-"+kind, g)
			h := startFakeHost(t, testSSHUser, testSSHPass)
			conn := u.addConnection("app-01", h.addr)
			opt := filepath.Join(h.home, "opt")
			dir := filepath.Join(opt, "demo-api")
			writeFile(t, dir+"/run.py", runV110)
			writeFile(t, dir+"/lib/util.py", strings.ReplaceAll(utilV1, "\n", "\r\n")) // CRLF only: same
			writeFile(t, dir+"/config.ini", "[main]\nhost=prod.example.com\n")
			writeFile(t, dir+"/setup.py", setupServer)
			writeFile(t, dir+"/extra.txt", "only here")
			writeFile(t, dir+"/log/app.log", "2026-10-26 started\n")
			writeFile(t, dir+"/worker.log.1", "rotated\n")
			writeFile(t, dir+"/logo.bin", "PNG\x00\x01\x02")
			writeFile(t, dir+"/VERSION.md", "# demo-api - v1.9\n\n- **Servis**:      demo-api\n- **Verzija**:     v1.9\n- **Commit**:      "+commitSHA("c1")[:10]+"\n")
			writeFile(t, dir+"/recyclebin/old.py", "x") // ignore_dirs
			writeFile(t, dir+"/lib/__pycache__/util.cpython.pyc", "x")
			os.Symlink("/etc/hostname", dir+"/link.txt")
			os.Symlink("run.py", dir+"/inner.py")
			os.Symlink("/", dir+"/out")
			writeFile(t, dir+"/secret.key", "k")
			os.Chmod(dir+"/secret.key", 0)
			cat := demoCatalog(g.srv.URL)
			app := cat["apps"].([]map[string]interface{})[0]
			app["exclude"] = []string{"*.md", "setup.py", "tests"}
			cat["server_roots"] = []string{opt}
			if code := u.jsonDo("PUT", "/api/git/catalog", cat, nil); code != 200 {
				t.Fatalf("catalog: %d", code)
			}
			if code := u.jsonDo("POST", "/api/git/check", map[string]interface{}{"conn_ids": []int{conn}, "discover": true, "refresh": true}, nil); code != 200 {
				t.Fatalf("check: %d", code)
			}
			var st gitStateResp
			u.jsonDo("GET", "/api/git", nil, &st)
			if len(st.Installs) != 1 {
				t.Fatalf("installs: %+v", st.Installs)
			}
			in := st.Installs[0]
			// log files are never part of a check: not even as extra files
			for _, f := range in.Files {
				if isLogPath(f.Path) {
					t.Fatalf("log file in the check: %+v", f)
				}
			}
			// setup.py differs, but the catalog excludes it: the installation is up to date
			if in.State != "ok" || in.LastOKState != "ok" || in.LastOKAt == "" {
				t.Fatalf("state: %+v", in)
			}

			var ls gitListing
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/files", in.ID), nil, &ls); code != 200 {
				t.Fatalf("files: %d", code)
			}
			by := map[string]gitListEntry{}
			for _, f := range ls.Files {
				by[f.Path] = f
			}
			check := func(p, state, reason, pattern string) gitListEntry {
				t.Helper()
				f, ok := by[p]
				if !ok || f.State != state || f.Reason != reason || f.Pattern != pattern {
					t.Fatalf("%s: %+v (want %s / %s / %s)", p, f, state, reason, pattern)
				}
				return f
			}
			if f := check("run.py", "ok", "tracked", ""); !f.Tracked || !f.InGit || f.Size != int64(len(runV110)) || f.MTime == "" {
				t.Fatalf("run.py: %+v", f)
			}
			check("lib/util.py", "ok", "tracked", "")
			check("config.ini", "protected", "protected", "config*")
			if f := check("setup.py", "outside", "excluded", "setup.py"); !f.Diffable || f.Tracked || !f.InGit {
				t.Fatalf("setup.py: %+v", f)
			}
			if f := check("tests/test_run.py", "outside", "excluded", "tests"); f.OnServer || !f.Diffable {
				t.Fatalf("git only: %+v", f)
			}
			check("extra.txt", "outside", "extra", "")
			check("VERSION.md", "outside", "tool", "")
			check("log/app.log", "outside", "logs", "")
			check("worker.log.1", "outside", "logs", "")
			if f := check("logs/committed.log", "outside", "logs", ""); f.OnServer || !f.InGit {
				t.Fatalf("committed log: %+v", f)
			}
			if f := check("link.txt", "symlink", "extra", ""); f.Link != "/etc/hostname" || !f.Outside || f.Diffable || f.Viewable {
				t.Fatalf("symlink: %+v", f)
			}
			if f := check("inner.py", "symlink", "extra", ""); f.Outside {
				t.Fatalf("inner symlink: %+v", f)
			}
			if _, ok := by["recyclebin/old.py"]; ok {
				t.Fatal("ignore_dirs not honoured")
			}
			if _, ok := by["lib/__pycache__/util.cpython.pyc"]; ok {
				t.Fatal("pruned directory listed")
			}
			if _, ok := by["out/etc/hostname"]; ok {
				t.Fatal("symlinked directory followed")
			}
			if os.Geteuid() != 0 { // root reads everything
				check("secret.key", "unreadable", "extra", "")
			}
			if ls.GitPartial || ls.Truncated || ls.Target != "v1.10" {
				t.Fatalf("listing: partial %v truncated %v target %s", ls.GitPartial, ls.Truncated, ls.Target)
			}

			// diff of a file the catalog excludes: informational
			var df struct {
				Lines         []diffLine `json:"lines"`
				Informational bool       `json:"informational"`
				Reason        string     `json:"reason"`
				Pattern       string     `json:"pattern"`
				State         string     `json:"state"`
				Same          bool       `json:"same"`
				Binary        bool       `json:"binary"`
				Missing       bool       `json:"missing_on_server"`
			}
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/diff?path=setup.py", in.ID), nil, &df); code != 200 || !df.Informational ||
				df.Reason != "excluded" || df.Pattern != "setup.py" || df.State != "differs" || len(df.Lines) != 4 || df.Lines[3].S != "setup(name='demo-api', version='1.10')" || df.Lines[3].Op != "+" {
				t.Fatalf("excluded diff: %d %+v", code, df)
			}
			df.Lines = nil
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/diff?path=lib/util.py", in.ID), nil, &df); code != 200 || len(df.Lines) != 0 || !df.Same || df.Informational {
				t.Fatalf("CRLF diff: %d %+v", code, df)
			}
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/diff?path=tests/test_run.py", in.ID), nil, &df); code != 200 || !df.Missing || df.State != "missing" || len(df.Lines) != 3 {
				t.Fatalf("git only diff: %d %+v", code, df)
			}
			for _, p := range []string{"extra.txt", "inner.py", "config.ini", "../etc/passwd"} {
				if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/diff?path=%s", in.ID, p), nil, nil); code != 400 {
					t.Fatalf("diff %s: %d", p, code)
				}
			}

			// viewing files
			var vw struct {
				Side   string `json:"side"`
				Text   string `json:"text"`
				Binary bool   `json:"binary"`
				Size   int    `json:"size"`
			}
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/file?path=extra.txt", in.ID), nil, &vw); code != 200 || vw.Text != "only here" || vw.Side != "server" {
				t.Fatalf("view: %d %+v", code, vw)
			}
			vw = struct {
				Side   string `json:"side"`
				Text   string `json:"text"`
				Binary bool   `json:"binary"`
				Size   int    `json:"size"`
			}{}
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/file?path=logo.bin", in.ID), nil, &vw); code != 200 || !vw.Binary || vw.Text != "" || vw.Size != 6 {
				t.Fatalf("binary view: %d %+v", code, vw)
			}
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/file?path=setup.py&side=git", in.ID), nil, &vw); code != 200 || vw.Text != setupGit || vw.Side != "git" {
				t.Fatalf("git view: %d %+v", code, vw)
			}
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/file?path=tests/test_run.py", in.ID), nil, &vw); code != 200 || vw.Side != "git" {
				t.Fatalf("git only view: %d %+v", code, vw)
			}
			for _, p := range []string{"link.txt", "inner.py", "out/etc/hostname", "../../etc/passwd", "lib"} {
				if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/file?path=%s", in.ID, p), nil, nil); code != 400 {
					t.Fatalf("view %s: %d", p, code)
				}
			}
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/file?path=config.ini&side=git", in.ID), nil, nil); code != 400 {
				t.Fatalf("protected from git: %d", code)
			}
			// a path with quotes and a semicolon is quoted, never run
			writeFile(t, dir+"/it's; touch pwned", "q")
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/file?path=%s", in.ID, "it%27s%3B%20touch%20pwned"), nil, &vw); code != 200 || vw.Text != "q" {
				t.Fatalf("quoted path: %d %+v", code, vw)
			}
			if _, err := os.Stat(filepath.Join(h.home, "pwned")); err == nil {
				t.Fatal("command injection")
			}

			// commits between the code and the target: the code on the server matches
			// v1.10, so nothing is between it and the target
			var cm struct {
				Count   int             `json:"count"`
				Commits []gitCommitInfo `json:"commits"`
				From    string          `json:"from"`
			}
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/commits", in.ID), nil, &cm); code != 200 || cm.Count != 0 || len(cm.Commits) != 0 ||
				cm.From != commitSHA("c2") {
				t.Fatalf("commits: %d %+v", code, cm)
			}
			// with md=1 VERSION.md (v1.9) is used: one commit to the target
			cm = struct {
				Count   int             `json:"count"`
				Commits []gitCommitInfo `json:"commits"`
				From    string          `json:"from"`
			}{}
			if code := u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/commits?md=1", in.ID), nil, &cm); code != 200 || cm.Count != 1 || len(cm.Commits) != 1 ||
				cm.Commits[0].SHA != commitSHA("c2") || cm.Commits[0].Title != "change 2026-09-10" || cm.From != commitSHA("c1")[:10] {
				t.Fatalf("commits md: %d %+v", code, cm)
			}
			// another user sees nothing
			other := newTestUser(t, srv, "git-files-other-"+kind, false)
			for _, p := range []string{"files", "file?path=run.py", "commits", "diff?path=setup.py"} {
				if code := other.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/%s", in.ID, p), nil, nil); code != 404 {
					t.Fatalf("foreign %s: %d", p, code)
				}
			}
			var n int
			db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='git.file_viewed' AND user_id=?`, u.userID).Scan(&n)
			if n == 0 {
				t.Fatal("file views not audited")
			}
		})
	}
}

// waitJob polls the current check job until it ends.
func waitJob(t *testing.T, u *testClient) gitJobView {
	t.Helper()
	for i := 0; i < 400; i++ {
		var r struct {
			Job *gitJobView `json:"job"`
		}
		u.jsonDo("GET", "/api/git/check/jobs/current", nil, &r)
		if r.Job != nil && r.Job.State != "running" {
			return *r.Job
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("the job did not end")
	return gitJobView{}
}

func TestGitCheckJobs(t *testing.T) {
	srv := newTestServer(t)
	g := newFakeGit(t, "gitlab")
	u := setupGitUser(t, srv, "git-jobs", g)
	h := startFakeHost(t, testSSHUser, testSSHPass)
	conn := u.addConnection("app-02", h.addr)
	opt := filepath.Join(h.home, "opt")
	names := []string{"demo-api_a", "demo-api_b", "demo-api_c", "demo-api_d"}
	for _, n := range names {
		writeFile(t, filepath.Join(opt, n, "run.py"), runV110)
		writeFile(t, filepath.Join(opt, n, "lib/util.py"), utilV1)
	}
	cat := demoCatalog(g.srv.URL)
	cat["server_roots"] = []string{opt}
	u.jsonDo("PUT", "/api/git/catalog", cat, nil)
	u.jsonDo("PUT", "/api/git/settings", map[string]interface{}{"conn_ids": []int{conn}}, nil)

	// everything, with discovery and targets: results stream in per installation
	gitJobTestHook = func(int) { time.Sleep(100 * time.Millisecond) }
	var jv gitJobView
	if code := u.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"all": true, "discover": true, "refresh": true}, &jv); code != 200 || jv.State != "running" {
		t.Fatalf("start: %d %+v", code, jv)
	}
	if code := u.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"all": true}, nil); code != 409 {
		t.Fatalf("second job: %d", code)
	}
	jv = waitJob(t, u)
	gitJobTestHook = nil
	if jv.State != "done" || jv.Total != 4 || jv.Done != 4 || len(jv.Installs) != 4 {
		t.Fatalf("job: %+v", jv)
	}
	byName := map[string]gitInstall{}
	for _, in := range jv.Installs {
		byName[filepath.Base(in.Path)] = in
		if in.State != "ok" {
			t.Fatalf("state: %+v", in)
		}
	}
	a, b := byName["demo-api_a"], byName["demo-api_b"]

	// a partial check of one installation keeps the others as they are
	writeFile(t, filepath.Join(opt, "demo-api_a", "run.py"), runV19)
	time.Sleep(1100 * time.Millisecond) // checked_at has a resolution of a second
	if code := u.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"install_ids": []int{a.ID}}, &jv); code != 200 {
		t.Fatalf("partial: %d", code)
	}
	jv = waitJob(t, u)
	if jv.Total != 1 || len(jv.Installs) != 1 || jv.Installs[0].State != "update" {
		t.Fatalf("partial job: %+v", jv)
	}
	list := loadGitInstalls(u.userID, false, "i.id=?", b.ID)
	if list[0].CheckedAt != b.CheckedAt || list[0].State != "ok" {
		t.Fatalf("other installation changed: %+v", list[0])
	}
	// "since" returns only installations finished later
	var r struct {
		Job *gitJobView `json:"job"`
	}
	u.jsonDo("GET", fmt.Sprintf("/api/git/check/jobs/current?since=%d", jv.Seq), nil, &r)
	if r.Job == nil || len(r.Job.Installs) != 0 {
		t.Fatalf("since: %+v", r.Job)
	}

	// only stale installations: b, c and d are older than a
	db.Exec(`UPDATE git_installs SET checked_at=? WHERE id<>? AND user_id=?`, time.Now().Add(-3*time.Hour).UTC().Format(time.RFC3339), a.ID, u.userID)
	db.Exec(`UPDATE git_installs SET checked_at='' WHERE id=?`, byName["demo-api_d"].ID)
	u.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"stale_minutes": 60}, nil)
	jv = waitJob(t, u)
	if jv.Total != 3 {
		t.Fatalf("stale: %+v", jv.Items)
	}
	db.Exec(`UPDATE git_installs SET checked_at='' WHERE id=?`, byName["demo-api_d"].ID)
	u.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"only_never": true}, nil)
	if jv = waitJob(t, u); jv.Total != 1 || jv.Items[0].InstallID != byName["demo-api_d"].ID {
		t.Fatalf("never checked: %+v", jv.Items)
	}

	// the concurrency limit per server
	var running, peak atomic.Int32
	gitJobTestHook = func(int) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(150 * time.Millisecond)
		running.Add(-1)
	}
	defer func() { gitJobTestHook = nil }()
	setSetting("git_check_per_server", "2")
	u.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"conn_ids": []int{conn}}, nil)
	jv = waitJob(t, u)
	if jv.Total != 4 || jv.MaxActive != 2 || peak.Load() != 2 {
		t.Fatalf("limit 2: max %d peak %d", jv.MaxActive, peak.Load())
	}
	peak.Store(0)
	setSetting("git_check_per_server", "4")
	setSetting("git_check_parallel", "1")
	u.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"conn_ids": []int{conn}}, nil)
	if jv = waitJob(t, u); jv.MaxActive != 1 || peak.Load() != 1 {
		t.Fatalf("overall limit 1: max %d peak %d", jv.MaxActive, peak.Load())
	}
	setSetting("git_check_parallel", "8")
	setSetting("git_check_per_server", "2")

	// cancel: the running scan is dropped, queued ones are skipped, results stay
	release := make(chan struct{})
	started := make(chan int, 8)
	gitJobTestHook = func(id int) { started <- id; <-release }
	before := map[int]gitInstall{}
	for _, in := range loadGitInstalls(u.userID, false, "") {
		before[in.ID] = in
	}
	setSetting("git_check_per_server", "1")
	if code := u.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"conn_ids": []int{conn}}, nil); code != 200 {
		t.Fatalf("start right after the previous job ended: %d", code)
	}
	select {
	case <-started:
	case <-time.After(30 * time.Second):
		t.Fatal("the scan did not start")
	}
	if code := u.jsonDo("POST", "/api/git/check/jobs/current/cancel", nil, nil); code != 200 {
		t.Fatalf("cancel: %d", code)
	}
	close(release)
	jv = waitJob(t, u)
	if jv.State != "cancelled" {
		t.Fatalf("cancelled job: %+v", jv)
	}
	for _, it := range jv.Items {
		if it.State != "cancelled" {
			t.Fatalf("item after cancel: %+v", it)
		}
	}
	for _, in := range loadGitInstalls(u.userID, false, "") {
		if in.CheckedAt != before[in.ID].CheckedAt || in.State != before[in.ID].State {
			t.Fatalf("result changed by a cancelled job: %+v", in)
		}
	}
	setSetting("git_check_per_server", "2")
	gitJobTestHook = nil
	if code := u.jsonDo("POST", "/api/git/check/jobs/current/cancel", nil, nil); code != 400 {
		t.Fatalf("cancel without a job: %d", code)
	}

	// an unreachable server: the installations keep their last good result
	db.Exec(`UPDATE connections SET host='127.0.0.1:1' WHERE id=?`, conn)
	u.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"install_ids": []int{b.ID}}, nil)
	jv = waitJob(t, u)
	got := loadGitInstalls(u.userID, true, "i.id=?", b.ID)[0]
	if jv.Items[0].State != "error" || got.State != "error" || got.LastOKState != "ok" || got.Counts["ok"] == 0 || len(got.Files) == 0 || got.Error == "" {
		t.Fatalf("error keeps the last result: %+v / %+v", jv.Items, got)
	}

	// foreign installations and servers, policy
	other := newTestUser(t, srv, "git-jobs-other", false)
	if code := other.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"install_ids": []int{a.ID}}, nil); code != 404 {
		t.Fatalf("foreign install: %d", code)
	}
	if code := other.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"conn_ids": []int{conn}}, nil); code != 404 {
		t.Fatalf("foreign server: %d", code)
	}
	setSetting("git_checks", "admins")
	if code := u.jsonDo("POST", "/api/git/check/jobs", map[string]interface{}{"all": true}, nil); code != 403 {
		t.Fatalf("policy: %d", code)
	}
	setSetting("git_checks", "all")
}

// An earlier git_sources table that v11.3.0 set aside comes back.
func TestGitSourcesRestored(t *testing.T) {
	newTestServer(t)
	db.Exec(`DELETE FROM git_sources`)
	db.Exec(`CREATE TABLE git_sources_old_20261009120000 (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '',
		url TEXT NOT NULL DEFAULT '', token TEXT NOT NULL DEFAULT '', info TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '')`)
	db.Exec(`INSERT INTO git_sources_old_20261009120000 (id, user_id, kind, name, url, token) VALUES (77, 1, 'gitlab', 'demo', 'https://git.example.com', 'enc')`)
	restoreSetAsideGitSources()
	var name string
	if db.QueryRow(`SELECT name FROM git_sources WHERE id=77`).Scan(&name); name != "demo" {
		t.Fatal("not restored")
	}
	db.Exec(`DELETE FROM git_sources`)
	restoreSetAsideGitSources() // once only
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM git_sources`).Scan(&n)
	if n != 0 {
		t.Fatal("restored twice")
	}
	db.Exec(`DROP TABLE IF EXISTS git_sources_restored_20261009120000`)
}
