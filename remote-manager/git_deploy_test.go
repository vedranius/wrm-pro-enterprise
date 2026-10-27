// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestGitDeployFormats(t *testing.T) {
	line := pyJSONLine(map[string]interface{}{"user": "deploy", "app": "demo-api", "files": []string{"run.py", "lib/ü.py"}, "time": "2026-10-09T10:00:00", "env": ""})
	if line != `{"app": "demo-api", "env": "", "files": ["run.py", "lib/\u00fc.py"], "time": "2026-10-09T10:00:00", "user": "deploy"}` {
		t.Fatalf("jsonl: %s", line)
	}
	tg := gitTarget{App: "demo-api", Version: "v1.10", Branch: "main", RefKind: "tag", Commit: "abcdef0123456789", CommitDate: "2026-09-10",
		Protected: []string{"config*"}, Files: map[string]gitTargetFile{
			"run.py":      {History: []gitHistEntry{{Commit: "abcdef0123", Date: "2026-09-10", Tag: "v1.10"}, {Commit: "1111111111", Date: "2026-09-01", Tag: "v1.9"}}},
			"lib/util.py": {History: []gitHistEntry{{Commit: "2222222222", Date: "2026-08-01"}}},
			"config.ini":  {History: []gitHistEntry{{Commit: "3333333333", Date: "2026-08-01"}}},
		}}
	plan := []planFile{{Path: "run.py", State: "old", Behind: 1}, {Path: "lib/util.py", State: "ok"}, {Path: "config.ini", State: "protected"}}
	when := time.Date(2026, 10, 9, 21, 5, 7, 0, time.Local)
	got := string(versionMD(tg, versionRows(tg, plan, nil), when, "app-01", "deploy", "wrm-20261009-2105", false))
	want := "# demo-api - v1.10\n\n" +
		"- **Servis**:      demo-api\n" +
		"- **Verzija**:     v1.10\n" +
		"- **Grana/ref**:   main (tag)\n" +
		"- **Commit**:      abcdef0123 (2026-09-10)\n" +
		"- **Azurirano**:   2026-10-09 21:05:07\n" +
		"- **Host**:        app-01\n" +
		"- **Korisnik**:    deploy\n" +
		"- **Bundle**:      wrm-20261009-2105\n\n" +
		"## Services (3)\n\n" +
		"| Datoteka    | Verzija    | Datum      |\n" +
		"| ----------- | ---------- | ---------- |\n" +
		"| config.ini  | (per-host) | -          |\n" +
		"| lib/util.py | 2222222222 | 2026-08-01 |\n" +
		"| run.py      | v1.9       | 2026-09-01 |\n"
	if got != want {
		t.Fatalf("VERSION.md:\n%s\nwant:\n%s", got, want)
	}
	if v := parseVersionMD(strings.Split(got, "\n")); v.Version != "v1.10" || v.Commit != "abcdef0123 (2026-09-10)" || v.Bundle != "wrm-20261009-2105" {
		t.Fatalf("parse: %+v", v)
	}
	if crlf := string(versionMD(tg, nil, when, "h", "u", "b", true)); !strings.Contains(crlf, "## Services (0)\r\n") || strings.Contains(strings.ReplaceAll(crlf, "\r\n", ""), "\n") {
		t.Fatalf("crlf: %q", crlf)
	}
	if gitBackupName("wrm-20261009-2105", when) != "wrm-20261009-2105-20261009-210507" {
		t.Fatal("backup name")
	}
	// dangling imports
	newUtil := []byte("import os\nfrom x import (a,\n  b as bb)\nVALUE = 1\nc: int = 3\n\ndef util():\n    helper = 2\n    return helper\nclass Box:\n    pass\ntry:\n    import json\nexcept ImportError:\n    fallback = None\n")
	names, ok := pyModuleNames(newUtil)
	for _, n := range []string{"os", "a", "bb", "VALUE", "c", "util", "Box", "json", "fallback"} {
		if !names[n] {
			t.Errorf("name %s not found (%v, %v)", n, names, ok)
		}
	}
	if names["helper"] || !ok {
		t.Fatalf("names: %v %v", names, ok)
	}
	imports := []pyImport{
		{"main.py", "from lib.util import util, helper as h"},
		{"lib/other.py", "from .util import (Box,\n VALUE, gone)"},
		{"tools/x.py", "from util import *"},
		{"lib/util.py", "from lib.util import nothing"}, // the module itself
		{"web.py", "from library.util import helper"},   // another module
	}
	d := danglingImports(map[string][]byte{"lib/util.py": newUtil}, imports, map[string]bool{})
	if strings.Join(d, "|") != "lib/other.py: from .util import gone (lib/util.py no longer defines gone)|main.py: from lib.util import helper (lib/util.py no longer defines helper)" {
		t.Fatalf("dangling: %q", d)
	}
	if d := danglingImports(map[string][]byte{"pkg/__init__.py": []byte("X = 1\n")}, []pyImport{{"app.py", "from pkg import sub, X, Y"}}, map[string]bool{"pkg/sub.py": true}); len(d) != 1 || !strings.Contains(d[0], "import Y") {
		t.Fatalf("package: %q", d)
	}
	if _, ok := pyModuleNames([]byte("def __getattr__(name):\n    return 1\n")); ok {
		t.Fatal("module __getattr__ must disable the check")
	}
}

// deployEnv is an admin with the demo catalog, a fake Git server and a fake SSH host with
// one installation of demo-api.
type deployEnv struct {
	u       *testClient
	g       *fakeGit
	h       *fakeHost
	conn    int
	dir     string
	units   string
	logFile string
	sysLog  string
}

func newDeployEnv(t *testing.T, name string) *deployEnv {
	srv := newTestServer(t)
	g := newFakeGit(t, "gitlab")
	e := &deployEnv{g: g, h: startFakeHost(t, testSSHUser, testSSHPass)}
	e.u = newTestUser(t, srv, name, true)
	e.conn = e.u.addConnection("app-01", e.h.addr)
	opt := filepath.Join(e.h.home, "opt")
	e.dir = filepath.Join(opt, "demo-api")
	cat := demoCatalog(g.srv.URL)
	cat["server_roots"] = []string{opt}
	if code := e.u.jsonDo("PUT", "/api/git/catalog", cat, nil); code != 200 {
		t.Fatalf("catalog: %d", code)
	}
	if code := e.u.jsonDo("POST", "/api/git/sources", map[string]interface{}{"kind": "gitlab", "name": "demo", "url": g.srv.URL, "token": g.token}, nil); code != 200 {
		t.Fatalf("source: %d", code)
	}
	e.units = t.TempDir()
	writeFile(t, e.units+"/demo-api.service", "[Service]\nWorkingDirectory="+e.dir+"\n")
	oldSys, oldSup, oldLog, oldSettle, oldSudo := gitSystemdDirs, gitSupervisorDirs, gitUpdateLogPaths, gitRestartSettle, gitSudoPrelude
	gitSystemdDirs, gitSupervisorDirs = []string{e.units}, []string{e.units}
	gitUpdateLogPaths = []string{filepath.Join(t.TempDir(), "missing", "updates.jsonl"), "{install}/.deploy-bak/updates.jsonl", filepath.Join(t.TempDir(), "updates.jsonl")}
	gitRestartSettle, gitSudoPrelude = 0, "SU="
	t.Cleanup(func() {
		gitSystemdDirs, gitSupervisorDirs, gitUpdateLogPaths, gitRestartSettle, gitSudoPrelude = oldSys, oldSup, oldLog, oldSettle, oldSudo
	})
	// fake systemctl: logs its arguments, units are active
	bin := t.TempDir()
	e.sysLog = filepath.Join(bin, "systemctl.log")
	writeFile(t, bin+"/systemctl", "#!/bin/sh\necho \"$@\" >> "+e.sysLog+"\n[ \"$1\" = is-active ] && echo active\nexit 0\n")
	os.Chmod(bin+"/systemctl", 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	e.logFile = e.dir + "/.deploy-bak/updates.jsonl"
	return e
}

func (e *deployEnv) check(t *testing.T) gitInstall {
	t.Helper()
	if code := e.u.jsonDo("POST", "/api/git/check", map[string]interface{}{"conn_ids": []int{e.conn}, "discover": true, "refresh": true}, nil); code != 200 {
		t.Fatalf("check: %d", code)
	}
	list := loadGitInstalls(e.u.userID, true, "i.path=?", e.dir)
	if len(list) != 1 {
		t.Fatalf("installs: %+v", list)
	}
	return list[0]
}

func (e *deployEnv) run(t *testing.T, body map[string]interface{}) (*gitRunView, int) {
	t.Helper()
	var v gitRunView
	code := e.u.jsonDo("POST", "/api/git/runs", body, &v)
	if code != 200 {
		return nil, code
	}
	return e.wait(t, v.ID), code
}

func (e *deployEnv) wait(t *testing.T, id int) *gitRunView {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		var v gitRunView
		e.u.jsonDo("GET", fmt.Sprintf("/api/git/runs/%d", id), nil, &v)
		if !v.Running && v.State != "running" && v.State != "starting" {
			return &v
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("run %d did not finish", id)
	return nil
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func backupDirs(t *testing.T, dir string) []string {
	var out []string
	list, _ := os.ReadDir(filepath.Join(dir, ".deploy-bak"))
	for _, d := range list {
		if d.IsDir() {
			out = append(out, d.Name())
		}
	}
	return out
}

func TestGitDeployUpdateRollbackStamp(t *testing.T) {
	e := newDeployEnv(t, "git-deployer")
	crlfV19 := strings.ReplaceAll(runV19, "\n", "\r\n")
	writeFile(t, e.dir+"/run.py", crlfV19) // one version behind, CRLF
	os.Chmod(e.dir+"/run.py", 0o750)
	writeFile(t, e.dir+"/config.ini", "[main]\nhost=app-01.example.com\n")
	writeFile(t, e.dir+"/main.py", "from lib.util import util\n")
	// lib/util.py is missing
	db.Exec(`UPDATE connections SET tags='env:prod' WHERE id=?`, e.conn)
	if code := e.u.jsonDo("POST", "/api/git/installs", map[string]interface{}{"conn_id": e.conn, "path": e.dir, "app": "demo-api"}, nil); code != 200 {
		t.Fatalf("add install: %d", code)
	}
	in := e.check(t)
	if in.State != "update" || len(in.Units) != 1 {
		t.Fatalf("install: %+v", in)
	}

	// plan: old + missing are selected by default, the protected file never
	var plan struct {
		Items []gitPlanItem `json:"items"`
	}
	if code := e.u.jsonDo("POST", "/api/git/plan", map[string]interface{}{"kind": "update", "install_ids": []int{in.ID}}, &plan); code != 200 || len(plan.Items) != 1 {
		t.Fatalf("plan: %d %+v", code, plan)
	}
	sel := map[string]string{}
	for _, f := range plan.Items[0].Files {
		if f.Selected {
			sel[f.Path] = f.State
		}
	}
	if len(sel) != 2 || sel["run.py"] != "old" || sel["lib/util.py"] != "missing" || !plan.Items[0].Prod {
		t.Fatalf("plan files: %+v", plan.Items[0])
	}

	// production: the server name must be typed
	body := map[string]interface{}{"kind": "update", "items": []map[string]interface{}{{"install_id": in.ID}}}
	if _, code := e.run(t, body); code != 400 {
		t.Fatalf("unconfirmed production run: %d", code)
	}
	body["confirm"] = map[string]string{fmt.Sprint(in.ID): "app-01"}
	v, code := e.run(t, body)
	if code != 200 || v.State != "done" || v.Items[0].State != "ok" {
		t.Fatalf("update: %d %+v", code, v)
	}
	it := v.Items[0]
	if got := readFile(t, e.dir+"/run.py"); got != strings.ReplaceAll(runV110, "\n", "\r\n") {
		t.Fatalf("run.py: %q (CRLF must stay)", got)
	}
	if st, _ := os.Stat(e.dir + "/run.py"); st.Mode().Perm() != 0o750 {
		t.Fatalf("mode: %v", st.Mode())
	}
	if got := readFile(t, e.dir+"/lib/util.py"); got != utilV1 {
		t.Fatalf("new file: %q", got)
	}
	if got := readFile(t, e.dir+"/config.ini"); got != "[main]\nhost=app-01.example.com\n" {
		t.Fatalf("protected file changed: %q", got)
	}
	bk := backupDirs(t, e.dir)
	if len(bk) != 1 || !regexp.MustCompile(`^wrm-\d{8}-\d{4}-\d{8}-\d{6}$`).MatchString(bk[0]) || it.MadeBackup != e.dir+"/.deploy-bak/"+bk[0] {
		t.Fatalf("backup: %v %s", bk, it.MadeBackup)
	}
	if got := readFile(t, e.dir+"/.deploy-bak/"+bk[0]+"/run.py"); got != crlfV19 {
		t.Fatalf("backup copy: %q", got)
	}
	if _, err := os.Stat(e.dir + "/.deploy-bak/" + bk[0] + "/lib/util.py"); err == nil {
		t.Fatal("a new file must not be in the backup")
	}
	c2 := commitSHA("c2")[:10]
	vmd := readFile(t, e.dir+"/VERSION.md")
	for _, want := range []string{"# demo-api - v1.10\n", "- **Grana/ref**:   main (tag)\n", "- **Commit**:      " + c2 + " (2026-09-10)\n",
		"- **Bundle**:      " + strings.Split(bk[0], "-")[0] + "-" + strings.Split(bk[0], "-")[1] + "-" + strings.Split(bk[0], "-")[2] + "\n",
		"## Services (3)\n", "| config.ini  | (per-host) | -          |\n", "| lib/util.py | v1.10      | 2026-09-10 |\n"} {
		if !strings.Contains(vmd, want) {
			t.Fatalf("VERSION.md lacks %q:\n%s", want, vmd)
		}
	}
	logLines := strings.Split(strings.TrimSpace(readFile(t, e.logFile)), "\n")
	if len(logLines) != 1 || !regexp.MustCompile(`^\{"app": "demo-api", "backup": "[^"]+", "branch": "main", "bundle": "wrm-\d{8}-\d{4}", "env": "", "files": \["lib/util.py", "run.py"\], "from_version": "-", "host": "[^"]+", "install": "[^"]+", "services": \["systemctl demo-api"\], "time": "\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d", "to_version": "`+c2+` \(2026-09-10\)", "user": "[^"]+"\}$`).MatchString(logLines[0]) {
		t.Fatalf("updates.jsonl: %s", logLines[0])
	}
	if it.UpdateLog != e.logFile {
		t.Fatalf("log location: %s", it.UpdateLog)
	}
	in = loadGitInstalls(e.u.userID, true, "i.id=?", in.ID)[0]
	if in.State != "ok" || in.VersionMD.Version != "v1.10" || in.RestartPending == "" {
		t.Fatalf("after the update: %+v", in)
	}

	// stamp: VERSION.md exists → skipped; without it → written
	if v, _ := e.run(t, map[string]interface{}{"kind": "stamp", "items": []map[string]interface{}{{"install_id": in.ID}}, "confirm": map[string]string{fmt.Sprint(in.ID): "app-01"}}); v.Items[0].State != "skipped" {
		t.Fatalf("stamp over VERSION.md: %+v", v.Items[0])
	}
	os.Remove(e.dir + "/VERSION.md")
	if v, _ := e.run(t, map[string]interface{}{"kind": "stamp", "items": []map[string]interface{}{{"install_id": in.ID}}, "confirm": map[string]string{fmt.Sprint(in.ID): "app-01"}}); v.State != "done" || v.Items[0].State != "ok" {
		t.Fatalf("stamp: %+v", v.Items[0])
	}
	if vmd := readFile(t, e.dir+"/VERSION.md"); !strings.Contains(vmd, "# demo-api - v1.10\n") || !strings.Contains(vmd, "| run.py      | v1.10      | 2026-09-10 |") {
		t.Fatalf("stamped VERSION.md:\n%s", vmd)
	}
	// a file changed by hand: stamp skips it, unless the stamp is "as it is"
	runPy := readFile(t, e.dir+"/run.py")
	os.WriteFile(e.dir+"/run.py", []byte(runPy+"# local\n"), 0644)
	stampAll := map[string]interface{}{"kind": "stamp", "force": true, "items": []map[string]interface{}{{"install_id": in.ID}}, "confirm": map[string]string{fmt.Sprint(in.ID): "app-01"}}
	if v, _ := e.run(t, stampAll); v.Items[0].State != "skipped" || !strings.Contains(v.Items[0].Error, "not up to date") {
		t.Fatalf("stamp of a changed installation: %+v", v.Items[0])
	}
	stampAll["stamp_partial"] = true
	if v, _ := e.run(t, stampAll); v.State != "done" || v.Items[0].State != "ok" {
		t.Fatalf("partial stamp: %+v", v.Items[0])
	}
	if vmd := readFile(t, e.dir+"/VERSION.md"); !strings.Contains(vmd, "- **Napomena**:") || !strings.Contains(vmd, "| run.py      | (local)") || !strings.Contains(vmd, "| lib/util.py | v1.10") {
		t.Fatalf("partially stamped VERSION.md:\n%s", vmd)
	}
	os.WriteFile(e.dir+"/run.py", []byte(runPy), 0644)

	// rollback to the backup: run.py comes back, the new file and VERSION.md go away
	var bl struct {
		Backups []gitBackup `json:"backups"`
	}
	if code := e.u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/backups", in.ID), nil, &bl); code != 200 || len(bl.Backups) != 1 || strings.Join(bl.Backups[0].LogFiles, ",") != "lib/util.py,run.py" {
		t.Fatalf("backups: %d %+v", code, bl)
	}
	v, code = e.run(t, map[string]interface{}{"kind": "rollback", "items": []map[string]interface{}{{"install_id": in.ID, "backup": bk[0]}}, "confirm": map[string]string{fmt.Sprint(in.ID): "APP-01"}})
	if code != 200 || v.State != "done" {
		t.Fatalf("rollback: %d %+v", code, v)
	}
	if got := readFile(t, e.dir+"/run.py"); got != crlfV19 {
		t.Fatalf("rolled back run.py: %q %+v", got, v.Items[0])
	}
	for _, gone := range []string{"/lib/util.py", "/lib", "/VERSION.md"} {
		if _, err := os.Stat(e.dir + gone); err == nil {
			t.Fatalf("%s should be gone after the rollback", gone)
		}
	}
	if logLines = strings.Split(strings.TrimSpace(readFile(t, e.logFile)), "\n"); len(logLines) != 2 || !strings.Contains(logLines[1], `"files": ["run.py", "lib/util.py"]`) {
		t.Fatalf("rollback log: %v", logLines)
	}

	// a failing check rolls back automatically and removes the backup
	before := backupDirs(t, e.dir)
	body = map[string]interface{}{"kind": "update", "items": []map[string]interface{}{{"install_id": in.ID}}, "confirm": map[string]string{fmt.Sprint(in.ID): "app-01"},
		"checks": map[string]string{"demo-api": "echo broken >&2; exit 3"}}
	v, _ = e.run(t, body)
	if v.State != "failed" || v.Items[0].State != "rolled_back" || !strings.Contains(v.Items[0].Error, "broken") {
		t.Fatalf("failed check: %+v", v.Items[0])
	}
	if got := readFile(t, e.dir+"/run.py"); got != crlfV19 {
		t.Fatalf("after the automatic rollback: %q", got)
	}
	if _, err := os.Stat(e.dir + "/lib/util.py"); err == nil {
		t.Fatal("new file left after the automatic rollback")
	}
	if after := backupDirs(t, e.dir); len(after) != len(before) {
		t.Fatalf("backups after the automatic rollback: %v → %v", before, after)
	}
	var st gitStateResp
	e.u.jsonDo("GET", "/api/git", nil, &st)
	if st.Settings.CheckCommands["demo-api"] == "" {
		t.Fatal("the custom check is not remembered")
	}

	// dangling import: main.py imports a name the new lib/util.py does not define
	writeFile(t, e.dir+"/main.py", "from lib.util import util, helper\n")
	e.check(t)
	body = map[string]interface{}{"kind": "update", "items": []map[string]interface{}{{"install_id": in.ID}}, "confirm": map[string]string{fmt.Sprint(in.ID): "app-01"}}
	v, _ = e.run(t, body)
	if v.Items[0].State != "failed" || len(v.Items[0].Dangling) != 1 || !strings.Contains(v.Items[0].Dangling[0], "main.py: from lib.util import helper") {
		t.Fatalf("dangling: %+v", v.Items[0])
	}
	if got := readFile(t, e.dir+"/run.py"); got != crlfV19 || len(backupDirs(t, e.dir)) != len(before) {
		t.Fatal("nothing may change when imports dangle")
	}
	body["ignore_imports"] = true
	if v, _ = e.run(t, body); v.State != "done" {
		t.Fatalf("ignore imports: %+v", v.Items[0])
	}

	// a file changed by hand is written only when ticked as such
	writeFile(t, e.dir+"/lib/util.py", "def util():\n    return 2\n")
	e.check(t)
	body = map[string]interface{}{"kind": "update", "items": []map[string]interface{}{{"install_id": in.ID, "files": []string{"lib/util.py"}}}, "confirm": map[string]string{fmt.Sprint(in.ID): "app-01"}, "ignore_imports": true}
	if v, _ = e.run(t, body); v.Items[0].State != "failed" || !strings.Contains(v.Items[0].Error, "changed on the server") {
		t.Fatalf("modified without a tick: %+v", v.Items[0])
	}
	body["items"] = []map[string]interface{}{{"install_id": in.ID, "files": []string{"lib/util.py", "config.ini"}, "modified_ok": []string{"lib/util.py"}}}
	if v, _ = e.run(t, body); v.Items[0].State != "failed" || !strings.Contains(v.Items[0].Error, "protected") {
		t.Fatalf("protected: %+v", v.Items[0])
	}
	body["items"] = []map[string]interface{}{{"install_id": in.ID, "files": []string{"lib/util.py"}, "modified_ok": []string{"lib/util.py"}}}
	if v, _ = e.run(t, body); v.State != "done" || readFile(t, e.dir+"/lib/util.py") != utilV1 {
		t.Fatalf("modified with a tick: %+v", v.Items[0])
	}

	// audit and policies
	for _, act := range []string{"git.update", "git.rollback", "git.stamp"} {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action=?`, act).Scan(&n)
		if n == 0 {
			t.Errorf("no audit entry %s", act)
		}
	}
	db.Exec(`UPDATE users SET is_admin=0 WHERE id=?`, e.u.userID)
	if code := e.u.jsonDo("POST", "/api/git/plan", map[string]interface{}{"kind": "update", "install_ids": []int{in.ID}}, nil); code != 403 {
		t.Fatalf("policy: %d", code)
	}
	setSetting("git_update", "all")
	if code := e.u.jsonDo("POST", "/api/git/plan", map[string]interface{}{"kind": "update", "install_ids": []int{in.ID}}, nil); code != 200 {
		t.Fatalf("policy all: %d", code)
	}
	setSetting("git_update", "admins")
}

func TestGitDeployScheduledRestart(t *testing.T) {
	e := newDeployEnv(t, "git-restarter")
	writeFile(t, e.dir+"/run.py", runV19)
	writeFile(t, e.dir+"/lib/util.py", utilV1)
	in := e.check(t)
	hook := newFakeHook(t)
	admin := newTestUser(t, newTestServer(t), "git-restart-admin", true)
	ch := createChannel(t, admin, map[string]interface{}{"name": "Git deploy hook", "kind": "webhook", "config": map[string]string{"url": hook.srv.URL + "/d"}})
	defer db.Exec(`DELETE FROM notify_channels WHERE id=?`, ch.ID)
	subs := []map[string]interface{}{}
	for _, ev := range []string{"git.deploy_started", "git.deploy_done", "git.restart_scheduled", "git.restart_done", "git.job_upcoming"} {
		subs = append(subs, map[string]interface{}{"event": ev, "channel_id": ch.ID})
	}
	if code := e.u.jsonDo("PUT", "/api/notify", map[string]interface{}{"subscriptions": subs}, nil); code != 200 {
		t.Fatalf("subscribe: %d", code)
	}
	defer db.Exec(`DELETE FROM notify_pending WHERE user_id=?`, e.u.userID)
	events := func() map[string]int {
		out := map[string]int{}
		rows, _ := db.Query(`SELECT event FROM notify_pending WHERE user_id=?`, e.u.userID)
		for rows.Next() {
			var ev string
			rows.Scan(&ev)
			out[ev]++
		}
		rows.Close()
		return out
	}

	// update now, restart at a time: the restart is a stored run waiting for its time
	at := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	v, code := e.run(t, map[string]interface{}{"kind": "update", "items": []map[string]interface{}{{"install_id": in.ID}}, "restart": map[string]interface{}{"mode": "at", "at": at}})
	if code != 200 || v.State != "done" {
		t.Fatalf("update: %d %+v", code, v)
	}
	var runs struct {
		Runs []*gitRunView `json:"runs"`
	}
	e.u.jsonDo("GET", "/api/git/runs", nil, &runs)
	var child *gitRunView
	for _, r := range runs.Runs {
		if r.ParentID == v.ID {
			child = r
		}
	}
	if child == nil || child.State != "scheduled" || child.Kind != "restart" || child.ScheduledAt != at {
		t.Fatalf("restart run: %+v", runs.Runs)
	}
	// The run reads as done before finishGitRun sends its notifications: wait for them.
	for deadline := time.Now().Add(10 * time.Second); events()["git.restart_scheduled"] == 0 && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
	if ev := events(); ev["git.deploy_started"] != 1 || ev["git.deploy_done"] != 1 || ev["git.restart_scheduled"] != 1 {
		t.Fatalf("events: %v", ev)
	}
	if got, _ := os.ReadFile(e.sysLog); strings.Contains(string(got), "restart ") {
		t.Fatal("restarted too early")
	}
	// a notice before, then the restart at its time (as after a WRM restart: it is stored)
	when, _ := time.Parse(time.RFC3339, at)
	gitSchedulerTick(when.Add(-10 * time.Minute))
	gitSchedulerTick(when.Add(-5 * time.Minute))
	if ev := events(); ev["git.job_upcoming"] != 1 {
		t.Fatalf("upcoming notice: %v", ev)
	}
	gitSchedulerTick(when.Add(time.Minute))
	r := e.wait(t, child.ID)
	if r.State != "done" || len(r.Items) != 1 || len(r.Items[0].Health) != 1 || !r.Items[0].Health[0].OK {
		t.Fatalf("restart: %+v", r)
	}
	if got := withoutFunctionalityReads(readFile(t, e.sysLog)); got != "restart demo-api.service\nis-active demo-api.service\n" {
		t.Fatalf("systemctl calls: %q", got)
	}
	if in = loadGitInstalls(e.u.userID, false, "i.id=?", in.ID)[0]; in.RestartPending != "" {
		t.Fatal("the pending restart was not cleared")
	}
	for deadline := time.Now().Add(10 * time.Second); events()["git.restart_done"] == 0 && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
	if ev := events(); ev["git.restart_done"] != 1 {
		t.Fatalf("restart notification: %v", ev)
	}

	// scheduled restart: cancelled, and one that WRM missed is skipped
	body := map[string]interface{}{"kind": "restart", "items": []map[string]interface{}{{"install_id": in.ID}}, "restart": map[string]interface{}{"mode": "delay", "delay_minutes": 30}}
	var s1, s2 gitRunView
	if code := e.u.jsonDo("POST", "/api/git/runs", body, &s1); code != 200 || s1.State != "scheduled" {
		t.Fatalf("schedule: %d %+v", code, s1)
	}
	if code := e.u.jsonDo("POST", fmt.Sprintf("/api/git/runs/%d/cancel", s1.ID), nil, &s1); code != 200 || s1.State != "cancelled" {
		t.Fatalf("cancel: %d %+v", code, s1)
	}
	e.u.jsonDo("POST", "/api/git/runs", body, &s2)
	gitSchedulerTick(time.Now().Add(3 * time.Hour))
	if r := e.wait(t, s2.ID); r.State != "skipped" || !strings.Contains(r.Message, "not running") {
		t.Fatalf("late run: %+v", r)
	}
	if got := readFile(t, e.sysLog); strings.Count(got, "restart") != 1 {
		t.Fatalf("cancelled or late restarts ran: %q", got)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='git.restart'`).Scan(&n)
	if n == 0 {
		t.Fatal("no git.restart audit entry")
	}
}

// Runtime inspection is allowed before a scheduled restart. Keep assertions about
// mutation ordering and the existing post-restart health check.
func withoutFunctionalityReads(log string) string {
	var out strings.Builder
	for _, line := range strings.Split(log, "\n") {
		if line == "" || strings.HasPrefix(line, "is-active -- ") || strings.HasPrefix(line, "list-units ") {
			continue
		}
		out.WriteString(line + "\n")
	}
	return out.String()
}
