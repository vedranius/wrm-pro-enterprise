// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// ─── native deployment tests (invented names; the destinations are local directories,
// or a fake SSH host) ───

type mdDeployEnv struct {
	t      *testing.T
	uid    int
	cfgDir string
	root   string // servers' directories below root/srv
	src    string // the prepared source
	host   func(server string) mdHost
	commit int
}

func newMdDeployEnv(t *testing.T, uid int, project string) *mdDeployEnv {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not installed")
	}
	e := &mdDeployEnv{t: t, uid: uid, root: t.TempDir(), src: t.TempDir()}
	e.cfgDir = mediumConfigDir(uid)
	os.RemoveAll(mediumUserDir(uid))
	t.Cleanup(func() { os.RemoveAll(mediumUserDir(uid)) })
	project = strings.ReplaceAll(project, "{srv}", filepath.ToSlash(filepath.Join(e.root, "srv")))
	for name, content := range map[string]string{"servers.yaml": mdServersYAML, "environments.yaml": mdEnvsYAML, "projects/demo-api.yaml": project} {
		p := filepath.Join(e.cfgDir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e.host = func(string) mdHost { return mdLocal{} }
	old := mdOpenHost
	t.Cleanup(func() { mdOpenHost = old })
	mdOpenHost = func(uid int, cfg *mdConfig, server string, fn func(mdHost, string) error) error {
		return fn(e.host(server), "local "+server)
	}
	return e
}

const mdDeployProject = `name: demo-api
repo: git@git.example.com:team/demo-api.git
ignore: [.env, storage/]
targets:
  dev: app-01:{srv}/a/demo-api
  prod:
    destinations:
      - app-01:{srv}/p/demo-api
      - app-02:{srv}/b/demo-api
    post_deploy: pwd > .post-deploy-ran
    allowed_refs: null
`

func (e *mdDeployEnv) dir(sub string) string { return filepath.Join(e.root, "srv", sub, "demo-api") }

// setSource writes the source as an export would: files 0664, executables 0775, dirs 0775.
func (e *mdDeployEnv) setSource(spec map[string]string) {
	os.RemoveAll(e.src)
	os.MkdirAll(e.src, 0o775)
	for p, v := range spec {
		full := filepath.Join(e.src, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o775)
		switch {
		case v == "/":
			os.MkdirAll(full, 0o775)
		case strings.HasPrefix(v, "->"):
			os.Symlink(strings.TrimPrefix(v, "->"), full)
		case strings.HasPrefix(v, "x:"):
			os.WriteFile(full, []byte(v[2:]), 0o775)
			os.Chmod(full, 0o775)
		default:
			os.WriteFile(full, []byte(v), 0o664)
			os.Chmod(full, 0o664)
		}
	}
	filepath.Walk(e.src, func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.IsDir() {
			os.Chmod(p, 0o775)
		}
		return nil
	})
}

func (e *mdDeployEnv) plan(env string, excluded []string) (*mdPlanResult, mdTree) {
	e.t.Helper()
	tree, err := mdLocalTree(e.src)
	if err != nil {
		e.t.Fatal(err)
	}
	cfg, err := mdLoadConfig(e.cfgDir)
	if err != nil {
		e.t.Fatal(err)
	}
	proj := cfg.project("demo-api")
	target := proj.target(env)
	ex, err := mdCleanExcluded(excluded)
	if err != nil {
		e.t.Fatal(err)
	}
	e.commit++
	res := &mdPlanResult{Project: proj.Name, Environment: target.Name, Ref: "refs/heads/main", Commit: fmt.Sprintf("%07x", e.commit) + strings.Repeat("0", 33),
		SourceFrom: "git_source", RefAllowed: true, Confirm: target.Confirm, Excluded: ex, Deployable: true}
	for _, d := range target.Destinations {
		sp := mdPlanServer(e.uid, cfg, proj, target, d, tree, nil, ex, false)
		if sp.Error != "" {
			e.t.Fatalf("plan %s: %s", d.label(), sp.Error)
		}
		res.Servers = append(res.Servers, sp)
	}
	res.Revision = mdPlanRevision(res)
	return res, tree
}

func (e *mdDeployEnv) context(kind string, dry bool, project, env string) (*mdDeployContext, *mdJob) {
	cfg, err := mdLoadConfig(e.cfgDir)
	if err != nil {
		e.t.Fatal(err)
	}
	proj := cfg.project(project)
	j := &mdJob{}
	j.Kind, j.DryRun, j.Outcomes = kind, dry, []mdOutcome{}
	return &mdDeployContext{uid: e.uid, cfg: cfg, proj: proj, target: proj.target(env), dryRun: dry, job: j}, j
}

func (e *mdDeployEnv) deploy(res *mdPlanResult, tree mdTree, dry bool) *mdJob {
	e.t.Helper()
	dc, j := e.context("deploy", dry, res.Project, res.Environment)
	_, _, targets, err := mdDeployTargets(dc.cfg, res)
	if err != nil {
		e.t.Fatal(err)
	}
	// deployments of one test need distinct ids: the time advances a second per call
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(e.commit) * time.Second)
	id, _ := mdMakeDeployID(now, res.Commit)
	branch, kind := mdRefNames(res.Ref, res.Commit)
	dc.entry = mdHistoryEntry{Action: "DEPLOY"}
	dc.entry.mdRecord = mdRecord{Project: res.Project, Environment: res.Environment, Version: res.Commit[:7], Commit: res.Commit, Branch: branch, RefKind: kind,
		DeployID: id, DeployedAt: now.Format(mdTimeFormat), DeployedBy: "md-deployer", Excluded: []string{}}
	dc.src, dc.excluded = tree, res.Excluded
	j.DeployID = id
	mdDeployDestinations(dc, targets)
	return j
}

func (e *mdDeployEnv) rollback(env string, servers []string, dry bool) *mdJob {
	e.t.Helper()
	dc, j := e.context("rollback", dry, "demo-api", env)
	e.commit++
	rid, _ := mdMakeDeployID(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC).Add(time.Duration(e.commit)*time.Second), "fffffff")
	dc.entry = mdHistoryEntry{Action: "ROLLBACK"}
	dc.entry.mdRecord = mdRecord{Project: "demo-api", Environment: env, DeployID: rid, DeployedBy: "md-deployer"}
	var targets []mdRollbackTarget
	for _, d := range dc.target.Destinations {
		for _, s := range servers {
			if s == d.Server {
				entries, err := mdReadHistory(mdLocal{}, d, mdHistoryWindow)
				if err != nil {
					e.t.Fatal(err)
				}
				targets = append(targets, mdRollbackTarget{d: d, choice: mdChooseRollback(entries), postDeploy: d.PostDeploy})
			}
		}
	}
	mdRollbackDestinations(dc, targets)
	return j
}

func mdJobLog(j *mdJob) string {
	var sb strings.Builder
	for _, l := range j.Lines {
		sb.WriteString(l.Level + " " + l.Text + "\n")
	}
	return sb.String()
}

func mdWantStatus(t *testing.T, j *mdJob, want ...string) {
	t.Helper()
	if len(j.Outcomes) != len(want) {
		t.Fatalf("outcomes %+v, want %v\n%s", j.Outcomes, want, mdJobLog(j))
	}
	for i, w := range want {
		if j.Outcomes[i].Status != w {
			t.Fatalf("outcome %d: %+v, want %s\n%s", i, j.Outcomes[i], w, mdJobLog(j))
		}
	}
}

// mdSnapshot describes a tree: type, content or link target, and permission bits.
func mdSnapshot(t *testing.T, root string, withDirs bool) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			out[rel] = "link:" + l
		case fi.IsDir():
			if withDirs {
				out[rel] = fmt.Sprintf("dir:%o", fi.Mode()&(os.ModePerm|os.ModeSetgid))
			}
		default:
			b, _ := os.ReadFile(p)
			out[rel] = fmt.Sprintf("file:%s:%o", b, fi.Mode().Perm())
		}
		return nil
	})
	return out
}

func mdSnapDiff(a, b map[string]string) string {
	var out []string
	for k, v := range a {
		if b[k] != v {
			out = append(out, fmt.Sprintf("%s: %q vs %q", k, v, b[k]))
		}
	}
	for k, v := range b {
		if _, ok := a[k]; !ok {
			out = append(out, fmt.Sprintf("%s: missing vs %q", k, v))
		}
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

func mdCopyTree(t *testing.T, from, to string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(to), 0o755)
	if out, err := exec.Command("cp", "-a", from, to).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v %s", err, out)
	}
}

func TestMediumStateFormat(t *testing.T) {
	rec := mdRecord{Project: "demo-api", Environment: "prod", Version: "v1.2.0", Commit: strings.Repeat("a", 40), Branch: "v1.2.0", RefKind: strPtr("tag"),
		DeployID: "20261001-120000-aaaaaaa", DeployedAt: "2026-10-01T12:00:00Z", DeployedBy: "ana", Path: strPtr("/srv/www/demo-api")}
	if got := string(mdMarshalLine(&rec)); got != `{"project":"demo-api","environment":"prod","version":"v1.2.0","commit":"`+strings.Repeat("a", 40)+`","branch":"v1.2.0","ref_kind":"tag","deploy_id":"20261001-120000-aaaaaaa","deployed_at":"2026-10-01T12:00:00Z","deployed_by":"ana","excluded":[],"path":"/srv/www/demo-api"}`+"\n" {
		t.Fatalf("record: %s", got)
	}
	e := mdHistoryEntry{mdRecord: rec, Action: "DEPLOY", Server: strPtr("app-01"), Current: boolPtr(true)}
	e.Excluded = []string{"a<b>&c", "č.txt"}
	mdHistoryFiles(&e, []string{"new/"}, nil, []string{"x\xff"})
	got := string(mdMarshalLine(&e))
	for _, want := range []string{`"excluded":["a<b>&c","č.txt"]`, `"action":"DEPLOY","server":"app-01","note":null,"rolled_back":null,"current":true,"previous":null,"new":["new/"],"changed":[],"deleted":["x\\#377"],"counts":{"changed":0,"deleted":1,"new":1}}`} {
		if !strings.Contains(got, want) {
			t.Fatalf("entry %s\nmissing %s", got, want)
		}
	}
	if !strings.HasPrefix(got, `{"project":"demo-api","environment":"prod",`) {
		t.Fatalf("order: %s", got)
	}
	// "previous" present (null) and absent are told apart
	list := mdParseHistory(got + "not json\n" + `{"project":"p","environment":"e","version":"v","commit":"c","branch":"b","deploy_id":"20261001-1200-bbbbbbb","deployed_at":"x","deployed_by":"y"}` + "\n")
	if len(list) != 2 || list[1].DeployID != "20261001-120000-aaaaaaa" || !list[1].hasPrevious || list[0].hasPrevious || list[0].Action != "DEPLOY" {
		t.Fatalf("parse: %+v", list)
	}
	if !mdDeployIDRe.MatchString("20261001-1200-bbbbbbb") || mdDeployIDRe.MatchString("20261001-120000-ABCDEFG") {
		t.Fatal("deploy id pattern")
	}
	if id, _ := mdMakeDeployID(time.Date(2026, 10, 7, 12, 12, 14, 0, time.UTC), "8E3963Bffff"); id != "20261007-121214-8e3963b" {
		t.Fatalf("id %s", id)
	}
	if mdEscapePathControls("a\nb") != `a\#012b` {
		t.Fatal(mdEscapePathControls("a\nb"))
	}
	owner := mdLockOwner{User: "ana", Host: "WRM h", Since: "2026-10-01T12:00:00Z", DeployID: strPtr("20261001-120000-aaaaaaa")}
	if !strings.Contains(string(mdMarshalLine(&owner)), `"deploy_id":"20261001-120000-aaaaaaa","path":null}`) {
		t.Fatal(string(mdMarshalLine(&owner)))
	}
}

// The original tool's own history entries decide the rollback the same way.
func TestMediumRollbackChoiceOriginalHistory(t *testing.T) {
	lines := []string{
		// an old entry without previous, current and counts
		`{"project":"p","environment":"e","version":"v1","commit":"a","branch":"main","deploy_id":"20261001-1000-aaaaaaa","deployed_at":"t","deployed_by":"u","action":"DEPLOY"}`,
		// a failure before the transfer (the original's note): no files changed
		`{"project":"p","environment":"e","version":"v2","commit":"b","branch":"main","deploy_id":"20261001-110000-bbbbbbb","deployed_at":"t","deployed_by":"u","action":"DEPLOY","note":"deploy nije uspio: lock","current":false}`,
		// post_deploy failed: files are deployed and current.json was set
		`{"project":"p","environment":"e","version":"v3","commit":"c","branch":"main","deploy_id":"20261001-120000-ccccccc","deployed_at":"t","deployed_by":"u","action":"DEPLOY","note":"post_deploy nije uspio (izlazni kod 1)","counts":{"new":1,"changed":0,"deleted":0},"previous":{"project":"p","environment":"e","version":"v1","commit":"a","branch":"main","deploy_id":"20261001-1000-aaaaaaa","deployed_at":"t","deployed_by":"u"}}`,
		// an incomplete rollback of v3: v3 stays the one to roll back
		`{"project":"p","environment":"e","version":"v1","commit":"a","branch":"main","deploy_id":"20261001-130000-ccccccc","deployed_at":"t","deployed_by":"u","action":"ROLLBACK","rolled_back":"20261001-120000-ccccccc","current":false}`,
	}
	entries := mdParseHistory(strings.Join(lines, "\n"))
	c := mdChooseRollback(entries)
	if c.Deploy == nil || c.Deploy.DeployID != "20261001-120000-ccccccc" || c.Restores == nil || c.Restores.DeployID != "20261001-1000-aaaaaaa" {
		t.Fatalf("choice: %+v", c)
	}
	// a complete rollback of v3: v1 is next, and it has no "previous": nothing before it
	entries = mdParseHistory(strings.Join(append(lines, `{"project":"p","environment":"e","version":"v1","commit":"a","branch":"main","deploy_id":"20261001-140000-ccccccc","deployed_at":"t","deployed_by":"u","action":"ROLLBACK","rolled_back":"20261001-120000-ccccccc","current":true}`), "\n"))
	c = mdChooseRollback(entries)
	if c.Deploy == nil || c.Deploy.DeployID != "20261001-1000-aaaaaaa" || c.Restores != nil || c.After() != "the state before the first deployment" {
		t.Fatalf("choice after rollback: %+v", c)
	}
	if c := mdChooseRollback(nil); c.Deploy != nil || c.Reason == "" {
		t.Fatalf("empty: %+v", c)
	}
}

func TestMediumNativeLock(t *testing.T) {
	root := t.TempDir()
	d := &mdDestination{Server: "app-01", Path: filepath.ToSlash(filepath.Join(root, "srv", "demo-api"))}
	h := mdLocal{}
	owner := mdLockOwner{User: "ana", Host: "WRM test", Since: mdNowISO(), Project: "demo-api", Environment: "dev", DeployID: strPtr("20261001-120000-aaaaaaa")}
	if err := mdAcquireLock(h, d, owner); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(d.statePath() + "/lock/owner.json")
	if !strings.Contains(string(b), `"deploy_id":"20261001-120000-aaaaaaa","path":"`+d.appDir()+`"`) {
		t.Fatalf("owner.json: %s", b)
	}
	err := mdAcquireLock(h, d, owner)
	le, ok := err.(*mdLockedError)
	if !ok || le.Owner == nil || le.Owner.User != "ana" {
		t.Fatalf("second lock: %v", err)
	}
	// another deployment's release leaves the lock
	mdReleaseLock(h, d, "20261001-120001-bbbbbbb")
	if _, err := os.Stat(d.statePath() + "/lock"); err != nil {
		t.Fatal("released by another id")
	}
	// young
	if err := mdReleaseStaleLock(h, d, "20261001-120000-aaaaaaa"); err == nil || !strings.Contains(err.Error(), "minutes old") {
		t.Fatalf("young: %v", err)
	}
	old := time.Now().Add(-3 * time.Hour)
	os.Chtimes(d.statePath()+"/lock", old, old)
	if err := mdReleaseStaleLock(h, d, "20261001-120001-bbbbbbb"); err == nil || !strings.Contains(err.Error(), "another deployment") {
		t.Fatalf("other owner: %v", err)
	}
	if err := mdReleaseStaleLock(h, d, "20261001-120000-aaaaaaa"); err != nil {
		t.Fatal(err)
	}
	if err := mdReleaseStaleLock(h, d, ""); err == nil || !strings.Contains(err.Error(), "no longer locked") {
		t.Fatalf("gone: %v", err)
	}
	// a lock without owner.json
	os.MkdirAll(d.statePath()+"/lock", 0o755)
	if o, _ := mdReadLock(h, d); o == nil || o.User != "unknown" {
		t.Fatalf("unknown owner: %+v", o)
	}
	if err := mdReleaseLock(h, d, "20261001-120000-aaaaaaa"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d.statePath() + "/lock"); err != nil {
		t.Fatal("a lock without owner must stay for a deployment's own release")
	}
}

// A deployment leaves the server exactly as rsync -rlc --delete --backup would, and two
// rollbacks bring it back to where it started.
func TestMediumNativeDeployAndRollback(t *testing.T) {
	needRsync(t)
	e := newMdDeployEnv(t, 990101, mdDeployProject)
	a := e.dir("a")
	mdMk(t, a, map[string]string{"index.php": "v1", "old.php": "o", ".env": "secret", "gone/f": "g", "gone/sub/f2": "g2", "becomes-dir": "file",
		"becomes-file/in": "i", "lnk": "->a", "secret.conf": "s1", "same.txt": "same", "storage/cache": "c", "oldlink": "->x"})
	os.Chmod(filepath.Join(a, "secret.conf"), 0o600)
	os.Chmod(filepath.Join(a, "gone"), 0o750)
	e.setSource(map[string]string{"index.php": "v2", "new/dir/file": "n", "becomes-dir/x": "x", "becomes-file": "now a file", "lnk": "->b",
		"secret.conf": "s2", "same.txt": "same", "run.sh": "x:#!/bin/sh\n", "čćž/ü.txt": "u"})
	before := mdSnapshot(t, a, true)

	// what rsync does to a copy of the server
	ref := filepath.Join(e.root, "rsync", "demo-api")
	refBackup := filepath.Join(e.root, "rsync-backup")
	mdCopyTree(t, a, ref)
	if out, err := exec.Command("rsync", "-rlc", "--delete", "--backup", "--backup-dir="+refBackup, "--exclude=.env", "--exclude=storage/", "--exclude=storage", e.src+"/", ref+"/").CombinedOutput(); err != nil {
		t.Fatalf("rsync: %v %s", err, out)
	}

	res, tree := e.plan("dev", nil)
	if res.Servers[0].Counts["new"] == 0 || res.Servers[0].Counts["deleted"] == 0 {
		t.Fatalf("plan: %s", jsonMarshal(res.Servers[0]))
	}
	// a dry run changes nothing
	j := e.deploy(res, tree, true)
	mdWantStatus(t, j, "dry-run")
	if d := mdSnapDiff(before, mdSnapshot(t, a, true)); d != "" {
		t.Fatalf("dry run changed the server:\n%s", d)
	}
	if _, err := os.Stat(filepath.Join(e.root, "srv", "a", ".deploy")); err == nil {
		t.Fatal("dry run wrote state")
	}
	j = e.deploy(res, tree, false)
	mdWantStatus(t, j, "ok")
	id1 := j.DeployID
	if d := mdSnapDiff(mdSnapshot(t, ref, true), mdSnapshot(t, a, true)); d != "" {
		t.Fatalf("differs from rsync:\n%s\n%s", d, mdJobLog(j))
	}
	state := filepath.Join(e.root, "srv", "a", ".deploy", "demo-api")
	if d := mdSnapDiff(mdSnapshot(t, refBackup, false), mdSnapshot(t, filepath.Join(state, "backups", id1), false)); d != "" {
		t.Fatalf("backup differs from rsync's:\n%s", d)
	}
	// no upload is left behind
	for p := range mdSnapshot(t, a, true) {
		if strings.Contains(p, ".wrm") {
			t.Fatalf("temporary file left: %s", p)
		}
	}
	cur, _ := os.ReadFile(filepath.Join(state, "current.json"))
	if !strings.Contains(string(cur), `"deploy_id":"`+id1+`"`) || !strings.Contains(string(cur), `"path":"`+filepath.ToSlash(a)+`"`) || !strings.Contains(string(cur), `"ref_kind":"branch"`) {
		t.Fatalf("current.json: %s", cur)
	}
	hist, _ := os.ReadFile(filepath.Join(state, "history.jsonl"))
	if !strings.Contains(string(hist), `"previous":null`) || !strings.Contains(string(hist), `"current":true`) || !strings.Contains(string(hist), `"server":"app-01"`) {
		t.Fatalf("history: %s", hist)
	}
	added, _ := os.ReadFile(filepath.Join(state, "backups", id1+".added"))
	for _, want := range []string{"new/\n", "new/dir/file\n", "becomes-dir/\n", "becomes-file\n", "run.sh\n", "čćž/ü.txt\n"} {
		if !strings.Contains(string(added), want) {
			t.Fatalf(".added lacks %q:\n%s", want, added)
		}
	}
	dirs, _ := os.ReadFile(filepath.Join(state, "backups", id1+".dirs"))
	if !strings.Contains(string(dirs), "750\t") || !strings.Contains(string(dirs), "\tgone/\n") || !strings.Contains(string(dirs), "becomes-file/\n") {
		t.Fatalf(".dirs: %s", dirs)
	}
	if _, err := os.Stat(state + "/lock"); err == nil {
		t.Fatal("the lock stayed")
	}
	afterFirst := mdSnapshot(t, a, true)

	// a second deployment, then roll both back
	e.setSource(map[string]string{"index.php": "v3", "becomes-dir/x": "x", "becomes-file": "now a file", "lnk": "->c", "secret.conf": "s3", "same.txt": "same", "run.sh": "x:#!/bin/sh\n", "extra/e": "e"})
	res, tree = e.plan("dev", nil)
	j = e.deploy(res, tree, false)
	mdWantStatus(t, j, "ok")
	id2 := j.DeployID
	hist, _ = os.ReadFile(filepath.Join(state, "history.jsonl"))
	if !strings.Contains(string(hist), `"previous":{"project":"demo-api"`) {
		t.Fatalf("previous: %s", hist)
	}
	// a dry rollback changes nothing
	afterSecond := mdSnapshot(t, a, true)
	j = e.rollback("dev", []string{"app-01"}, true)
	mdWantStatus(t, j, "dry-run")
	if d := mdSnapDiff(afterSecond, mdSnapshot(t, a, true)); d != "" {
		t.Fatalf("dry rollback changed the server:\n%s", d)
	}
	j = e.rollback("dev", []string{"app-01"}, false)
	mdWantStatus(t, j, "ok")
	if d := mdSnapDiff(afterFirst, mdSnapshot(t, a, true)); d != "" {
		t.Fatalf("rollback of %s:\n%s\n%s", id2, d, mdJobLog(j))
	}
	cur, _ = os.ReadFile(filepath.Join(state, "current.json"))
	if !strings.Contains(string(cur), `"deploy_id":"`+id1+`"`) {
		t.Fatalf("current after the rollback: %s", cur)
	}
	j = e.rollback("dev", []string{"app-01"}, false)
	mdWantStatus(t, j, "ok")
	if d := mdSnapDiff(before, mdSnapshot(t, a, true)); d != "" {
		t.Fatalf("rollback of %s:\n%s\n%s", id1, d, mdJobLog(j))
	}
	if _, err := os.Stat(filepath.Join(state, "current.json")); err == nil {
		t.Fatal("current.json must be removed: there was none before the first deployment")
	}
	j = e.rollback("dev", []string{"app-01"}, false)
	mdWantStatus(t, j, "failed")
	if !strings.Contains(j.Outcomes[0].Message, "no deployment to roll back") {
		t.Fatalf("third rollback: %+v", j.Outcomes[0])
	}
}

// The server changed after the review: nothing is changed, and nothing is recorded.
func TestMediumNativeDeployMismatch(t *testing.T) {
	e := newMdDeployEnv(t, 990102, mdDeployProject)
	a := e.dir("a")
	mdMk(t, a, map[string]string{"index.php": "v1"})
	e.setSource(map[string]string{"index.php": "v2", "new.php": "n"})
	res, tree := e.plan("dev", nil)
	os.WriteFile(filepath.Join(a, "late.php"), []byte("added after the review"), 0o644)
	before := mdSnapshot(t, a, true)
	j := e.deploy(res, tree, false)
	mdWantStatus(t, j, "mismatch")
	if d := mdSnapDiff(before, mdSnapshot(t, a, true)); d != "" {
		t.Fatalf("mismatch changed the server:\n%s", d)
	}
	state := filepath.Join(e.root, "srv", "a", ".deploy", "demo-api")
	for _, f := range []string{"history.jsonl", "current.json", "lock", "backups/" + j.DeployID + ".added"} {
		if _, err := os.Stat(filepath.Join(state, f)); err == nil {
			t.Fatalf("%s written on a mismatch", f)
		}
	}
	// another tool's lock: the destination is reported locked, nothing is written
	os.MkdirAll(filepath.Join(state, "lock"), 0o755)
	os.WriteFile(filepath.Join(state, "lock", "owner.json"), []byte(`{"user":"marko","host":"laptop","since":"2026-10-01T10:00:00Z","project":"demo-api","environment":"dev","deploy_id":"20261001-100000-ddddddd","path":null}`), 0o644)
	res, tree = e.plan("dev", nil)
	j = e.deploy(res, tree, false)
	mdWantStatus(t, j, "locked")
	if j.Outcomes[0].LockOwner == nil || j.Outcomes[0].LockOwner.User != "marko" {
		t.Fatalf("lock owner: %+v", j.Outcomes[0])
	}
}

func TestMediumNativeExclusionsAndPostDeploy(t *testing.T) {
	e := newMdDeployEnv(t, 990103, mdDeployProject)
	a, b := e.dir("p"), e.dir("b")
	mdMk(t, a, map[string]string{"config.php": "server", "keep/local.txt": "mine", "index.php": "v1"})
	mdMk(t, b, map[string]string{"index.php": "v1"})
	e.setSource(map[string]string{"config.php": "git", "index.php": "v2"})
	res, tree := e.plan("prod", []string{"config.php", "keep/"})
	if len(res.Servers[0].ExcludedFor) != 2 || len(res.Servers[1].ExcludedFor) != 1 {
		t.Fatalf("excluded for: %v %v", res.Servers[0].ExcludedFor, res.Servers[1].ExcludedFor)
	}
	j := e.deploy(res, tree, false)
	mdWantStatus(t, j, "ok", "ok")
	if s := mdSnapshot(t, a, false); s["config.php"] != "file:server:644" || s["keep/local.txt"] == "" || !strings.HasPrefix(s["index.php"], "file:v2:") {
		t.Fatalf("exclusions not kept: %v", s)
	}
	cur, _ := os.ReadFile(filepath.Join(e.root, "srv", "p", ".deploy", "demo-api", "current.json"))
	if !strings.Contains(string(cur), `"excluded":["config.php","keep"]`) {
		t.Fatalf("current.json excluded: %s", cur)
	}
	// post_deploy ran in the application directory
	if got, _ := os.ReadFile(filepath.Join(b, ".post-deploy-ran")); strings.TrimSpace(string(got)) != filepath.ToSlash(b) {
		t.Fatalf("post_deploy: %q\n%s", got, mdJobLog(j))
	}
	// a failing post_deploy: the files are deployed, current.json is set, the note says so
	cfgFile := filepath.Join(e.cfgDir, "projects", "demo-api.yaml")
	text, _ := os.ReadFile(cfgFile)
	os.WriteFile(cfgFile, []byte(strings.Replace(string(text), "post_deploy: pwd > .post-deploy-ran", "post_deploy: echo broken; exit 3", 1)), 0o600)
	e.setSource(map[string]string{"config.php": "git", "index.php": "v3"})
	res, tree = e.plan("prod", []string{"config.php", "keep"})
	j = e.deploy(res, tree, false)
	mdWantStatus(t, j, "failed", "skipped")
	if !strings.Contains(j.Outcomes[0].Message, "post_deploy failed (exit code 3)") || !strings.Contains(mdJobLog(j), "broken") {
		t.Fatalf("post_deploy failure: %+v\n%s", j.Outcomes[0], mdJobLog(j))
	}
	hist, _ := os.ReadFile(filepath.Join(e.root, "srv", "p", ".deploy", "demo-api", "history.jsonl"))
	lines := strings.Split(strings.TrimSpace(string(hist)), "\n")
	if !strings.Contains(lines[len(lines)-1], `"note":"post_deploy failed (exit code 3)","rolled_back":null,"current":true`) {
		t.Fatalf("history: %s", lines[len(lines)-1])
	}
}

// mdFailingHost fails the move of one upload into place.
type mdFailingHost struct {
	mdLocal
	fail string
}

func (h *mdFailingHost) Rename(from, to string) error {
	if strings.HasSuffix(to, "/"+h.fail) && strings.Contains(from, ".wrm") {
		return fmt.Errorf("simulated failure")
	}
	return os.Rename(from, to)
}

func TestMediumNativePartialDeployRollsBack(t *testing.T) {
	e := newMdDeployEnv(t, 990104, mdDeployProject)
	a := e.dir("a")
	mdMk(t, a, map[string]string{"a.php": "1", "z.php": "1", "old.php": "o"})
	before := mdSnapshot(t, a, true)
	fh := &mdFailingHost{fail: "z.php"}
	e.host = func(string) mdHost { return fh }
	e.setSource(map[string]string{"a.php": "2", "z.php": "2", "new.php": "n"})
	res, tree := e.plan("dev", nil)
	j := e.deploy(res, tree, false)
	mdWantStatus(t, j, "failed")
	state := filepath.Join(e.root, "srv", "a", ".deploy", "demo-api")
	hist, _ := os.ReadFile(filepath.Join(state, "history.jsonl"))
	if !strings.Contains(string(hist), `"note":"deploy interrupted during transfer (partial): `) || !strings.Contains(string(hist), `"current":false`) || !strings.Contains(string(hist), `"counts":{`) {
		t.Fatalf("history: %s", hist)
	}
	if _, err := os.Stat(filepath.Join(state, "current.json")); err == nil {
		t.Fatal("current.json written after a partial deployment")
	}
	for p := range mdSnapshot(t, a, true) {
		if strings.Contains(p, ".wrm") {
			t.Fatalf("temporary file left: %s", p)
		}
	}
	// the rollback restores what was there
	e.host = func(string) mdHost { return mdLocal{} }
	j = e.rollback("dev", []string{"app-01"}, false)
	mdWantStatus(t, j, "ok")
	if d := mdSnapDiff(before, mdSnapshot(t, a, true)); d != "" {
		t.Fatalf("after the rollback:\n%s\n%s", d, mdJobLog(j))
	}
}

func TestMediumNativePruneBackups(t *testing.T) {
	e := newMdDeployEnv(t, 990105, mdDeployProject)
	mdMk(t, e.dir("a"), map[string]string{"index.php": "v0"})
	var ids []string
	for i := 1; i <= 7; i++ {
		e.setSource(map[string]string{"index.php": fmt.Sprint("v", i)})
		res, tree := e.plan("dev", nil)
		j := e.deploy(res, tree, false)
		mdWantStatus(t, j, "ok")
		ids = append(ids, j.DeployID)
	}
	backups := filepath.Join(e.root, "srv", "a", ".deploy", "demo-api", "backups")
	os.WriteFile(filepath.Join(backups, "notes.txt"), []byte("mine"), 0o644)
	e.setSource(map[string]string{"index.php": "v8"})
	res, tree := e.plan("dev", nil)
	j := e.deploy(res, tree, false)
	ids = append(ids, j.DeployID)
	list, _ := os.ReadDir(backups)
	var names []string
	for _, f := range list {
		names = append(names, f.Name())
	}
	// the last five applied (and the current one among them) stay, with their manifests
	for _, id := range ids[len(ids)-5:] {
		for _, s := range []string{"", ".added", ".dirs"} {
			if _, err := os.Stat(filepath.Join(backups, id+s)); err != nil {
				t.Fatalf("%s%s pruned: %v", id, s, names)
			}
		}
	}
	for _, id := range ids[:len(ids)-5] {
		if _, err := os.Stat(filepath.Join(backups, id+".added")); err == nil {
			t.Fatalf("%s kept: %v", id, names)
		}
	}
	if _, err := os.Stat(filepath.Join(backups, "notes.txt")); err != nil {
		t.Fatal("an unknown name was removed")
	}
}

// Over a real SSH connection (SFTP, posix-rename, the login shell for post_deploy).
func TestMediumNativeDeployOverSSH(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "md-ssh-deployer", true)
	h := startFakeHost(t, testSSHUser, testSSHPass)
	conn := u.addConnection("demo app-01", h.addr)
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
	mdMk(t, a, map[string]string{"index.php": "v1", "old.php": "o", "lnk": "->a"})
	before := mdSnapshot(t, a, true)
	e.setSource(map[string]string{"index.php": "v2", "lib/x.php": "x", "lnk": "->b"})
	res, tree := e.plan("dev", nil)
	j := e.deploy(res, tree, false)
	mdWantStatus(t, j, "ok")
	got := mdSnapshot(t, a, false)
	if !strings.HasPrefix(got["index.php"], "file:v2:") || got["lib/x.php"] == "" || got["lnk"] != "link:b" || got["old.php"] != "" {
		t.Fatalf("over SSH: %v\n%s", got, mdJobLog(j))
	}
	j = e.rollback("dev", []string{"app-01"}, false)
	mdWantStatus(t, j, "ok")
	if d := mdSnapDiff(before, mdSnapshot(t, a, true)); d != "" {
		t.Fatalf("rollback over SSH:\n%s\n%s", d, mdJobLog(j))
	}
}

func TestMediumNativeDeployAPI(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "md-api-admin", true)
	user := newTestUser(t, srv, "md-api-user", false)
	e := newMdDeployEnv(t, admin.userID, mdDeployProject)
	mdMk(t, e.dir("p"), map[string]string{"index.php": "v1"})
	mdMk(t, e.dir("b"), map[string]string{"index.php": "v1"})
	e.setSource(map[string]string{"index.php": "v2"})
	res, tree := e.plan("prod", nil)
	run := &mdRun{ID: "md-api-run-1", Done: true, Result: res, uid: admin.userID, at: time.Now(), src: tree}
	mdRuns.Lock()
	mdRuns.m[run.ID] = run
	mdRuns.Unlock()
	t.Cleanup(func() {
		mdRuns.Lock()
		delete(mdRuns.m, run.ID)
		mdRuns.Unlock()
	})
	body := map[string]interface{}{"run_id": run.ID, "revision": res.Revision}
	var out map[string]interface{}
	// projects stay on the original tool unless the native engine is the default or chosen
	setSetting("medium_default_engine", "original")
	if code := admin.jsonDo("POST", "/api/git/medium/native/deploy", body, &out); code != 400 || !strings.Contains(fmt.Sprint(out["error"]), "native engine first") {
		t.Fatalf("engine docker: %d %v", code, out)
	}
	setSetting("medium_default_engine", "native")
	if mdProjectEngine(admin.userID, "demo-api") != "native" || mdProjectEngine(user.userID, "demo-api") != "docker" {
		t.Fatalf("default engine: admin %s, user %s", mdProjectEngine(admin.userID, "demo-api"), mdProjectEngine(user.userID, "demo-api"))
	}
	if code := admin.jsonDo("POST", "/api/git/medium/native/engine", map[string]string{"project": "demo-api", "engine": "docker"}, nil); code != 200 || mdProjectEngine(admin.userID, "demo-api") != "docker" {
		t.Fatalf("explicit original tool: %d", code)
	}
	if code := user.jsonDo("POST", "/api/git/medium/native/engine", map[string]string{"project": "demo-api", "engine": "native"}, nil); code != 403 {
		t.Fatalf("a user may not deploy natively by default: %d", code)
	}
	if code := admin.jsonDo("POST", "/api/git/medium/native/engine", map[string]string{"project": "demo-api", "engine": "native"}, nil); code != 200 {
		t.Fatalf("engine: %d", code)
	}
	if code := admin.jsonDo("POST", "/api/git/medium/native/deploy", map[string]interface{}{"run_id": run.ID, "revision": "stale"}, &out); code != 400 {
		t.Fatalf("stale revision: %d %v", code, out)
	}
	if code := admin.jsonDo("POST", "/api/git/medium/native/deploy", body, &out); code != 400 || !strings.Contains(fmt.Sprint(out["error"]), "type the environment name") {
		t.Fatalf("confirm: %d %v", code, out)
	}
	if code := user.jsonDo("POST", "/api/git/medium/native/deploy", body, nil); code != 404 && code != 403 {
		t.Fatalf("another user's run: %d", code)
	}
	body["confirm"] = "prod"
	if code := admin.jsonDo("POST", "/api/git/medium/native/deploy", body, &out); code != 200 {
		t.Fatalf("deploy: %d %v", code, out)
	}
	jobID := fmt.Sprint(out["job_id"])
	var job mdJobView
	for i := 0; i < 200; i++ {
		admin.jsonDo("GET", "/api/git/medium/native/jobs/"+jobID, nil, &job)
		if job.Done {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !job.Done || len(job.Outcomes) != 2 || job.Outcomes[0].Status != "ok" || job.Outcomes[1].Status != "ok" {
		t.Fatalf("job: %+v", job)
	}
	if code := admin.jsonDo("POST", "/api/git/medium/native/deploy", body, &out); code != 409 {
		t.Fatalf("the same plan twice: %d %v", code, out)
	}
	var hist struct {
		Servers []mdHistoryView `json:"servers"`
	}
	if code := admin.jsonDo("GET", "/api/git/medium/native/history?project=demo-api&environment=prod", nil, &hist); code != 200 || len(hist.Servers) != 2 || hist.Servers[0].Choice == nil || hist.Servers[0].Choice.DeployID != job.DeployID {
		t.Fatalf("history: %d %+v", code, hist)
	}
	if code := admin.jsonDo("POST", "/api/git/medium/native/rollback", map[string]interface{}{"project": "demo-api", "environment": "prod", "deploy_id": job.DeployID, "servers": []string{"app-01", "app-02"}}, &out); code != 400 {
		t.Fatalf("rollback without confirm: %d %v", code, out)
	}
	if code := admin.jsonDo("POST", "/api/git/medium/native/rollback", map[string]interface{}{"project": "demo-api", "environment": "prod", "deploy_id": job.DeployID, "servers": []string{"app-01", "app-02"}, "confirm": "prod"}, &out); code != 200 {
		t.Fatalf("rollback: %d %v", code, out)
	}
	jobID = fmt.Sprint(out["job_id"])
	for i := 0; i < 200; i++ {
		admin.jsonDo("GET", "/api/git/medium/native/jobs/"+jobID, nil, &job)
		if job.Done {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !job.Done || len(job.Outcomes) != 2 || job.Outcomes[0].Status != "ok" || job.Outcomes[1].Status != "ok" {
		t.Fatalf("rollback job: %+v", job)
	}
	if b, _ := os.ReadFile(filepath.Join(e.dir("b"), "index.php")); string(b) != "v1" {
		t.Fatalf("rolled back content: %q", b)
	}
	var rows []map[string]interface{}
	r, _ := db.Query(`SELECT action FROM audit_log WHERE action LIKE 'git.medium_native_%' ORDER BY id DESC LIMIT 10`)
	for r.Next() {
		var a string
		r.Scan(&a)
		rows = append(rows, map[string]interface{}{"a": a})
	}
	r.Close()
	if len(rows) < 3 {
		t.Fatalf("audit rows: %v", rows)
	}
}

func TestMediumSafeTreePath(t *testing.T) {
	for p, want := range map[string]bool{"a/b.php": true, "čž/x": true, "": false, "/etc/x": false, "../x": false, "a/../b": false, "a//b": false, "./a": false, "a/": false} {
		if mdSafeTreePath(p) != want {
			t.Errorf("mdSafeTreePath(%q) != %v", p, want)
		}
	}
}

func TestMediumDefaultEngineMigration(t *testing.T) {
	newTestServer(t)
	reset := func() {
		db.Exec(`DELETE FROM app_settings WHERE key='medium_default_engine'`)
		settingsStore.Lock()
		delete(settingsStore.values, "medium_default_engine")
		settingsStore.Unlock()
	}
	reset()
	t.Cleanup(reset)
	// a new installation: native stays the default
	if _, err := os.Stat(mediumToolDir()); err == nil {
		t.Skip("another test left the original tool installed")
	}
	mdMigrateDefaultEngine()
	if getSetting("medium_default_engine") != "native" {
		t.Fatalf("new installation: %s", getSetting("medium_default_engine"))
	}
	// an upgraded installation with the original tool keeps it
	if err := os.MkdirAll(mediumToolDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(mediumToolDir()) })
	mdMigrateDefaultEngine()
	if getSetting("medium_default_engine") != "original" {
		t.Fatalf("upgrade: %s", getSetting("medium_default_engine"))
	}
	// an admin's later choice is kept
	setSetting("medium_default_engine", "native")
	mdMigrateDefaultEngine()
	if getSetting("medium_default_engine") != "native" {
		t.Fatal("the admin's choice was overwritten")
	}
}
