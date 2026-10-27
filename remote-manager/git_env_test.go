// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// envTest is the deploy environment of git_deploy_test.go with an environment "test" of
// demo-api: two destinations on the fake SSH host (app-01).
type envTest struct {
	*deployEnv
	dirs []string
	cat  map[string]interface{}
}

func newEnvTest(t *testing.T, name string, env map[string]interface{}) *envTest {
	e := &envTest{deployEnv: newDeployEnv(t, name)}
	e.dirs = []string{e.dir, filepath.Join(e.h.home, "srv", "demo-api")}
	e.cat = demoCatalog(e.g.srv.URL)
	e.cat["server_roots"] = []string{filepath.Join(e.h.home, "opt")}
	if env == nil {
		env = map[string]interface{}{}
	}
	env["name"] = "test"
	switch env["destinations"] {
	case nil:
		env["destinations"] = []map[string]string{{"server": "app-01", "path": e.dirs[0]}, {"server": "app-01", "path": e.dirs[1]}}
	case "single":
		env["destinations"] = []map[string]string{{"server": "app-01", "path": e.dirs[0]}}
	}
	e.cat["apps"].([]map[string]interface{})[0]["environments"] = []map[string]interface{}{env}
	if code := e.u.jsonDo("PUT", "/api/git/catalog", e.cat, nil); code != 200 {
		t.Fatalf("catalog: %d", code)
	}
	if code := e.u.jsonDo("POST", "/api/git/targets/refresh", map[string]interface{}{}, nil); code != 200 {
		t.Fatalf("refresh: %d", code)
	}
	return e
}

func (e *envTest) plan(t *testing.T, ref string) *gitEnvPlan {
	t.Helper()
	var p gitEnvPlan
	if code := e.u.jsonDo("POST", "/api/git/env/plan", map[string]interface{}{"app": "demo-api", "env": "test", "ref": ref}, &p); code != 200 {
		t.Fatalf("plan: %d", code)
	}
	return &p
}

func (e *envTest) deploy(t *testing.T, p *gitEnvPlan, env map[string]interface{}) (*gitRunView, int) {
	t.Helper()
	env["app"], env["env"], env["plan_id"] = "demo-api", "test", p.ID
	return e.run(t, map[string]interface{}{"kind": "update", "env": env})
}

func (e *envTest) rollback(t *testing.T, id string, env map[string]interface{}) (*gitRunView, int) {
	t.Helper()
	env["app"], env["env"], env["deploy_id"] = "demo-api", "test", id
	return e.run(t, map[string]interface{}{"kind": "rollback", "env": env})
}

func fileState(dp *gitEnvDestPlan, rel string) string {
	for _, f := range dp.Files {
		if f.Path == rel {
			return f.State
		}
	}
	return ""
}

func readState(t *testing.T, stateDir string) gitEnvState {
	t.Helper()
	var st gitEnvState
	if b, err := os.ReadFile(filepath.Join(stateDir, "state.json")); err == nil {
		json.Unmarshal(b, &st)
	}
	return st
}

func readHistory(t *testing.T, stateDir string) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	b, _ := os.ReadFile(filepath.Join(stateDir, "history.jsonl"))
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l == "" {
			continue
		}
		m := map[string]interface{}{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("history line %q: %v", l, err)
		}
		out = append(out, m)
	}
	return out
}

func logText(it *gitRunItem) string {
	var b strings.Builder
	for _, l := range it.Log {
		fmt.Fprintf(&b, "%s %s %s\n", l.Step, l.Status, l.Msg)
	}
	return b.String()
}

func TestGitEnvDeployDryRunAndRollback(t *testing.T) {
	e := newEnvTest(t, "env-deployer", map[string]interface{}{"confirm": true})
	d0, d1 := e.dirs[0], e.dirs[1]
	writeFile(t, d0+"/run.py", runV19)                                        // one version behind
	writeFile(t, d0+"/lib/util.py", strings.ReplaceAll(utilV1, "\n", "\r\n")) // CRLF only
	writeFile(t, d0+"/local.txt", "kept\n")                                   // not in the repository
	writeFile(t, d0+"/config.ini", "[main]\nhost=app-01.example.com\n")       // protected
	// d1 does not exist: the first deploy creates it

	p := e.plan(t, "")
	if len(p.Errors) > 0 || len(p.Dests) != 2 || !p.Confirm || p.RefName != "refs/tags/v1.10" {
		t.Fatalf("plan: %+v", p)
	}
	a, b := p.Dests[0], p.Dests[1]
	if fileState(a, "run.py") != "changed" || fileState(a, "lib/util.py") != "eol" || fileState(a, "local.txt") != "extra" || fileState(a, "config.ini") != "protected" {
		t.Fatalf("dest 0: %+v", a.Files)
	}
	if a.Counts["eol"] != 1 || a.Counts["same"] != 0 || a.Fingerprint == "" {
		t.Fatalf("counts: %+v", a.Counts)
	}
	if b.Exists || fileState(b, "run.py") != "new" || fileState(b, "lib/util.py") != "new" || !strings.Contains(strings.Join(b.Warnings, " "), "first deploy") {
		t.Fatalf("dest 1: %+v", b)
	}

	// a dry run: same options, no lock, nothing written, no confirmation
	v, code := e.deploy(t, p, map[string]interface{}{"dry_run": true, "post_deploy": false})
	if code != 200 || v.State != "done" || v.Env == nil || !v.Env.DryRun {
		t.Fatalf("dry run: %d %+v", code, v)
	}
	if l := logText(v.Items[0]); !strings.Contains(l, "would change (2): lib/util.py, run.py") || !strings.Contains(l, "would add") && !strings.Contains(l, "dry_run") {
		t.Fatalf("dry run log: %s", l)
	}
	if readFile(t, d0+"/run.py") != runV19 || fileExists(d0+"/.deploy-bak") || fileExists(d1) {
		t.Fatal("the dry run changed the server")
	}

	// the real deploy needs the typed environment name
	if _, code := e.deploy(t, p, map[string]interface{}{}); code != 400 {
		t.Fatalf("without confirmation: %d", code)
	}
	v, code = e.deploy(t, p, map[string]interface{}{"confirm": "test"})
	if code != 200 || v.State != "done" {
		t.Fatalf("deploy: %d %+v %s %s", code, v, v.Message, logText(v.Items[0]))
	}
	id := v.Env.DeployID
	if id == "" || v.Items[0].DeployID != id || v.Items[1].Path != d1 {
		t.Fatalf("items: %+v", v.Items)
	}
	if readFile(t, d0+"/run.py") != runV110 || readFile(t, d0+"/lib/util.py") != utilV1 || readFile(t, d0+"/local.txt") != "kept\n" || !strings.Contains(readFile(t, d0+"/config.ini"), "app-01") {
		t.Fatal("dest 0 files")
	}
	if readFile(t, d1+"/run.py") != runV110 || !strings.Contains(readFile(t, d1+"/VERSION.md"), "v1.10") || fileExists(d1+"/config.ini") {
		t.Fatal("dest 1 files")
	}
	sd0, sd1 := d0+"/.deploy-bak", d1+"/.deploy-bak"
	if fileExists(sd0+"/.lock") || fileExists(sd1+"/.lock") {
		t.Fatal("a lock is left")
	}
	if readFile(t, sd0+"/"+id+"/run.py") != runV19 || !strings.Contains(readFile(t, sd0+"/"+id+"/lib/util.py"), "\r\n") || !fileExists(sd0+"/deploys/"+id+".json") {
		t.Fatal("backup / manifest")
	}
	st := readState(t, sd0)
	if st.Current == nil || st.Current.DeployID != id || st.Current.Version != "v1.10" || st.Current.By != "env-deployer" || st.Previous != nil {
		t.Fatalf("state: %+v", st)
	}
	h := readHistory(t, sd1)
	if len(h) != 1 || h[0]["action"] != "DEPLOY" || h[0]["id"] != id || h[0]["previous"] != nil || len(h[0]["new"].([]interface{})) != 2 {
		t.Fatalf("history: %+v", h)
	}
	if c := h[0]["counts"].(map[string]interface{}); c["new"] != 2.0 || c["changed"] != 0.0 {
		t.Fatalf("history counts: %+v", c)
	}
	if h := readHistory(t, sd0); h[0]["counts"].(map[string]interface{})["eol"] != 1.0 {
		t.Fatalf("eol count: %+v", h[0])
	}
	if list := loadGitInstalls(e.u.userID, false, "i.path=?", d1); len(list) != 1 || list[0].Env != "test" {
		t.Fatalf("registered: %+v", list)
	}
	// one deploy per reviewed plan
	if _, code := e.deploy(t, p, map[string]interface{}{"confirm": "test"}); code != 409 {
		t.Fatalf("plan used twice: %d", code)
	}

	// history over the API (merged)
	var hist struct {
		Entries []map[string]interface{} `json:"entries"`
		Dests   []struct {
			Current string `json:"current"`
		} `json:"dests"`
	}
	if code := e.u.jsonDo("GET", "/api/git/env/history?app=demo-api&env=test", nil, &hist); code != 200 || len(hist.Entries) != 2 || histByDest(hist.Entries, 1)["path"] != d1 || histByDest(hist.Entries, 0) == nil || hist.Dests[1].Current != id {
		t.Fatalf("history api: %d %+v", code, hist)
	}
	// the overview cell
	var cell struct {
		Drift bool             `json:"drift"`
		Dests []gitEnvCellDest `json:"dests"`
	}
	if code := e.u.jsonDo("GET", "/api/git/env/cell?app=demo-api&env=test", nil, &cell); code != 200 || cell.Drift || len(cell.Dests) != 2 || cell.Dests[0].Current == nil || cell.Dests[0].Behind != 0 {
		t.Fatalf("cell: %d %+v", code, cell)
	}

	// dry-run rollback, then the rollback (reverse destination order)
	v, code = e.rollback(t, id, map[string]interface{}{"dry_run": true})
	if code != 200 || v.State != "done" || v.Items[0].Path != d1 || !strings.Contains(logText(v.Items[0]), "would restore") || readFile(t, d1+"/run.py") != runV110 {
		t.Fatalf("dry rollback: %d %+v", code, v)
	}
	if _, code := e.rollback(t, id, map[string]interface{}{}); code != 400 {
		t.Fatalf("rollback without confirmation: %d", code)
	}
	v, code = e.rollback(t, id, map[string]interface{}{"confirm": "TEST"})
	if code != 200 || v.State != "done" {
		t.Fatalf("rollback: %d %+v %s", code, v, logText(v.Items[0]))
	}
	if readFile(t, d0+"/run.py") != runV19 || !strings.Contains(readFile(t, d0+"/lib/util.py"), "\r\n") || readFile(t, d0+"/local.txt") != "kept\n" {
		t.Fatal("dest 0 not restored")
	}
	aside := sd1 + "/backups/rollback-" + id
	if fileExists(d1+"/run.py") || readFile(t, aside+"/run.py") != runV110 || !fileExists(aside+"/VERSION.md") || readFile(t, sd0+"/backups/rollback-"+id+"/run.py") != runV110 {
		t.Fatal("added files must be moved aside, current versions kept")
	}
	if st := readState(t, sd0); st.Current != nil || st.RolledBack != id {
		t.Fatalf("state after rollback: %+v", st)
	}
	h = readHistory(t, sd0)
	if len(h) != 2 || h[1]["action"] != "ROLLBACK" || h[1]["rolled_back"] != id || h[1]["complete"] != true {
		t.Fatalf("rollback history: %+v", h)
	}
	// it is not the latest any more
	if v, _ := e.rollback(t, id, map[string]interface{}{"confirm": "test"}); v.State != "failed" || !strings.Contains(v.Message, "not the latest") {
		t.Fatalf("second rollback: %+v", v)
	}
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestGitEnvSelectedDestinations(t *testing.T) {
	e := newEnvTest(t, "env-selected", map[string]interface{}{"confirm": true})
	for _, dir := range e.dirs {
		writeFile(t, dir+"/run.py", runV19)
	}
	p := e.plan(t, "")
	for _, selected := range [][]int{{}, {-1}, {2}, {1, 1}} {
		if _, code := e.deploy(t, p, map[string]interface{}{"confirm": "test", "dests": selected}); code != 400 {
			t.Fatalf("invalid destinations %v: %d", selected, code)
		}
	}
	if _, code := e.deploy(t, p, map[string]interface{}{"dests": []int{1}}); code != 400 {
		t.Fatalf("selected deploy without confirmation: %d", code)
	}
	v, code := e.deploy(t, p, map[string]interface{}{"confirm": "test", "dests": []int{1}, "exclude": []string{"lib/util.py"}})
	if code != 200 || v.State != "done" || len(v.Items) != 1 || v.Items[0].Dest != 1 || len(v.Env.Dests) != 1 || v.Env.Dests[0] != 1 {
		t.Fatalf("selected deploy: %d %+v", code, v)
	}
	if readFile(t, e.dirs[1]+"/run.py") != runV110 || fileExists(e.dirs[1]+"/lib/util.py") || readFile(t, e.dirs[0]+"/run.py") != runV19 || fileExists(e.dirs[0]+"/.deploy-bak") || fileExists(e.dirs[0]+"/VERSION.md") {
		t.Fatal("deployment changed an unselected destination or an excluded file")
	}
	id := v.Env.DeployID
	for _, selected := range [][]int{{}, {-1}, {2}, {1, 1}} {
		if _, code := e.rollback(t, id, map[string]interface{}{"confirm": "test", "dests": selected}); code != 400 {
			t.Fatalf("invalid rollback destinations %v: %d", selected, code)
		}
	}
	v, code = e.rollback(t, id, map[string]interface{}{"confirm": "test", "dests": []int{1}})
	if code != 200 || v.State != "done" || len(v.Items) != 1 || v.Items[0].Dest != 1 || readFile(t, e.dirs[1]+"/run.py") != runV19 || fileExists(e.dirs[0]+"/.deploy-bak") {
		t.Fatalf("selected rollback: %d %+v", code, v)
	}
	if fileExists(e.dirs[1]+"/.deploy-bak/.lock") || readState(t, e.dirs[1]+"/.deploy-bak").Current != nil {
		t.Fatal("rollback left the current deployment or a lock behind")
	}
}

func TestGitEnvSelectHealthyDestination(t *testing.T) {
	e := newEnvTest(t, "env-select-healthy", nil)
	configured := e.cat["apps"].([]map[string]interface{})[0]["environments"].([]map[string]interface{})[0]
	configured["destinations"] = []map[string]string{{"server": "unavailable", "path": e.dirs[0]}, {"server": "app-01", "path": e.dirs[1]}}
	put := func() {
		t.Helper()
		if code := e.u.jsonDo("PUT", "/api/git/catalog", e.cat, nil); code != 200 {
			t.Fatalf("catalog: %d", code)
		}
	}
	put()
	p := e.plan(t, "")
	if len(p.Errors) != 0 || len(p.Dests[0].Errors) == 0 || len(p.Dests[1].Errors) != 0 {
		t.Fatalf("plan with unavailable destination: %+v", p)
	}
	for _, options := range []map[string]interface{}{{}, {"dests": []int{0}}} {
		if _, code := e.deploy(t, p, options); code != 400 {
			t.Fatalf("deploy including unavailable destination: %d", code)
		}
	}
	// A change even to an unselected destination invalidates the reviewed environment.
	configured["destinations"].([]map[string]string)[0]["path"] = e.dirs[0] + "-moved"
	put()
	if _, code := e.deploy(t, p, map[string]interface{}{"dests": []int{1}}); code != 409 {
		t.Fatalf("changed environment accepted: %d", code)
	}
	configured["destinations"].([]map[string]string)[0]["path"] = e.dirs[0]
	put()
	v, code := e.deploy(t, p, map[string]interface{}{"dests": []int{1}})
	if code != 200 || v.State != "done" || len(v.Items) != 1 || v.Items[0].Dest != 1 || readFile(t, e.dirs[1]+"/run.py") != runV110 || fileExists(e.dirs[0]) {
		t.Fatalf("healthy destination: %d %+v", code, v)
	}
}

func TestGitEnvSelectedPostDeployFailureAndRollback(t *testing.T) {
	e := newEnvTest(t, "env-selected-failure", map[string]interface{}{"post_deploy": "echo health-check-failed; exit 3"})
	for _, dir := range e.dirs {
		writeFile(t, dir+"/run.py", runV19)
	}
	v, code := e.deploy(t, e.plan(t, ""), map[string]interface{}{"dests": []int{1}, "post_deploy": true})
	if code != 200 || v.State != "failed" || len(v.Items) != 1 || !strings.Contains(v.Items[0].PostDeploy, "failed") {
		t.Fatalf("post-deploy failure: %d %+v", code, v)
	}
	id := v.Env.DeployID
	history := readHistory(t, e.dirs[1]+"/.deploy-bak")
	if len(history) != 1 || history[0]["result"] != "post_deploy_failed" || readFile(t, e.dirs[1]+"/run.py") != runV110 || fileExists(e.dirs[0]+"/.deploy-bak") {
		t.Fatal("failed health check was not recorded per destination")
	}
	v, code = e.rollback(t, id, map[string]interface{}{"dests": []int{1}})
	if code != 200 || v.State != "done" || readFile(t, e.dirs[1]+"/run.py") != runV19 || readFile(t, e.dirs[0]+"/run.py") != runV19 || fileExists(e.dirs[0]+"/.deploy-bak") {
		t.Fatalf("rollback after failed health check: %d %+v", code, v)
	}
}

func TestGitEnvConnectionChangeInvalidatesPlan(t *testing.T) {
	e := newEnvTest(t, "env-connection-binding", nil)
	writeFile(t, e.dirs[0]+"/run.py", runV19)
	p := e.plan(t, "")
	other := startFakeHost(t, testSSHUser, testSSHPass)
	if _, err := db.Exec(`UPDATE connections SET host=? WHERE id=?`, other.addr, e.conn); err != nil {
		t.Fatal(err)
	}
	if _, code := e.deploy(t, p, map[string]interface{}{"dests": []int{0}}); code != 409 {
		t.Fatalf("retargeted connection was accepted: %d", code)
	}
	if readFile(t, e.dirs[0]+"/run.py") != runV19 || fileExists(e.dirs[0]+"/.deploy-bak") {
		t.Fatal("a rejected plan wrote to the server")
	}
	if _, err := db.Exec(`UPDATE connections SET host=? WHERE id=?`, e.h.addr, e.conn); err != nil {
		t.Fatal(err)
	}
	v, code := e.deploy(t, p, map[string]interface{}{"dests": []int{0}})
	if code != 200 || v.State != "done" || readFile(t, e.dirs[0]+"/run.py") != runV110 {
		t.Fatalf("original connection binding failed: %d %+v", code, v)
	}
}

func TestGitEnvFingerprintLocksAndExclusions(t *testing.T) {
	e := newEnvTest(t, "env-locker", map[string]interface{}{"destinations": nil})
	d0 := e.dirs[0]
	sd := d0 + "/.deploy-bak"
	writeFile(t, d0+"/run.py", runV19)
	writeFile(t, d0+"/lib/util.py", utilV1)

	// the server changes after the review: the deploy stops under the lock
	p := e.plan(t, "")
	writeFile(t, d0+"/lib/util.py", "def util():\n    return 2\n")
	v, _ := e.deploy(t, p, map[string]interface{}{})
	if v.State != "failed" || !strings.Contains(v.Message, "changed since the plan") || readFile(t, d0+"/run.py") != runV19 || fileExists(sd+"/.lock") {
		t.Fatalf("fingerprint: %+v", v)
	}
	if _, code := e.deploy(t, p, map[string]interface{}{}); code != 409 {
		t.Fatalf("a failed deploy used the plan: %d", code)
	}

	// a fresh lock of another deploy: the deploy waits for it (fails) and leaves it alone
	writeFile(t, d0+"/lib/util.py", utilV1)
	os.MkdirAll(sd+"/.lock", 0o755)
	writeFile(t, sd+"/.lock/owner", fmt.Sprintf("id=other-1\nuser=colleague\nfrom=wrm-b\nepoch=%d\n", time.Now().Unix()))
	p = e.plan(t, "")
	if l := p.Dests[0].Lock; l == nil || l.ID != "other-1" || l.User != "colleague" || l.Stale {
		t.Fatalf("lock in plan: %+v", l)
	}
	v, _ = e.deploy(t, p, map[string]interface{}{})
	if v.State != "failed" || !strings.Contains(v.Message, "locked by colleague") || !strings.Contains(readFile(t, sd+"/.lock/owner"), "other-1") {
		t.Fatalf("contention: %+v", v)
	}
	// releasing only one's own lock
	conn, _ := loadConnection(e.conn)
	cl, err := dialSSH(conn, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if ok, err := envUnlock(cl, sd, "mine-2"); ok || err != nil || !fileExists(sd+"/.lock") {
		t.Fatalf("released a foreign lock: %v %v", ok, err)
	}
	// a stale lock (server clock) is removed only on request and only when unchanged
	writeFile(t, sd+"/.lock/owner", fmt.Sprintf("id=other-1\nuser=colleague\nfrom=wrm-b\nepoch=%d\n", time.Now().Unix()-int64(gitEnvStaleAfter()/time.Second)-60))
	p = e.plan(t, "")
	if l := p.Dests[0].Lock; l == nil || !l.Stale {
		t.Fatalf("stale: %+v", l)
	}
	if code := e.u.jsonDo("POST", "/api/git/env/unlock", map[string]interface{}{"app": "demo-api", "env": "test", "dest": 0, "lock_id": "someone-else"}, nil); code != 409 || !fileExists(sd+"/.lock") {
		t.Fatalf("unlock with another id: %d", code)
	}
	if code := e.u.jsonDo("POST", "/api/git/env/unlock", map[string]interface{}{"app": "demo-api", "env": "test", "dest": 0, "lock_id": "other-1"}, nil); code != 200 || fileExists(sd+"/.lock") {
		t.Fatalf("unlock: %d", code)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='git.lock_removed'`).Scan(&n)
	if n != 1 {
		t.Fatalf("audit: %d", n)
	}

	// per-run exclusion, remembered with who and when
	v, _ = e.deploy(t, p, map[string]interface{}{"exclude": []string{"run.py"}})
	if v.State != "done" || readFile(t, d0+"/run.py") != runV19 {
		t.Fatalf("excluded deploy: %+v %s", v, logText(v.Items[0]))
	}
	if h := readHistory(t, sd); len(h[0]["excluded"].([]interface{})) != 1 {
		t.Fatalf("history excluded: %+v", h[0])
	}
	p = e.plan(t, "")
	if x, ok := p.Excluded["run.py"]; !ok || x.By != "env-locker" || x.At == "" || fileState(p.Dests[0], "run.py") != "changed" {
		t.Fatalf("remembered: %+v", p.Excluded)
	}
	var cleared struct {
		Excluded map[string]gitEnvExclusion `json:"excluded"`
	}
	if code := e.u.jsonDo("DELETE", "/api/git/env/exclusions?app=demo-api&env=test", nil, &cleared); code != 200 || len(cleared.Excluded) != 0 {
		t.Fatalf("clear: %d %+v", code, cleared)
	}

	// permanent exclude: anchored, escaped, skipped when already covered
	var res struct {
		Pattern string `json:"pattern"`
		Added   bool   `json:"added"`
	}
	if code := e.u.jsonDo("POST", "/api/git/env/exclude", map[string]interface{}{"app": "demo-api", "env": "test", "path": "run.py", "scope": "service"}, &res); code != 200 || res.Pattern != "/run.py" || !res.Added {
		t.Fatalf("exclude service: %d %+v", code, res)
	}
	if code := e.u.jsonDo("POST", "/api/git/env/exclude", map[string]interface{}{"app": "demo-api", "env": "test", "path": "run.py", "scope": "env"}, &res); code != 200 || res.Added || res.Pattern != "/run.py" {
		t.Fatalf("already covered: %d %+v", code, res)
	}
	if code := e.u.jsonDo("POST", "/api/git/env/exclude", map[string]interface{}{"app": "demo-api", "env": "test", "path": "data/a[1]*.txt", "scope": "env"}, &res); code != 200 || res.Pattern != "/data/a[[]1][*].txt" || !res.Added {
		t.Fatalf("escaped: %d %+v", code, res)
	}
	cat := loadGitCatalog(e.u.userID)
	if a, _ := cat.app("demo-api"); !strings.Contains(strings.Join(a.Exclude, " "), "/run.py") || a.Environments[0].Ignore[0] != "/data/a[[]1][*].txt" {
		t.Fatalf("catalog: %+v", a)
	}
	if !globHitDir("data/a[1]*.txt", []string{"/data/a[[]1][*].txt"}) || globHitDir("data/a1x.txt", []string{"/data/a[[]1][*].txt"}) || globHit("x/run.py", []string{"/run.py"}) {
		t.Fatal("anchored patterns")
	}
	p = e.plan(t, "")
	if fileState(p.Dests[0], "run.py") != "ignored" {
		t.Fatalf("permanently excluded: %+v", p.Dests[0].Files)
	}
}

func TestGitEnvRulesAndValidation(t *testing.T) {
	e := newEnvTest(t, "env-rules", map[string]interface{}{"allowed_refs": []string{"refs/heads/release*"}})
	p := e.plan(t, "")
	if len(p.Errors) != 1 || !strings.Contains(p.Errors[0], "refs/tags/v1.10 is not allowed") {
		t.Fatalf("allowed refs: %+v", p.Errors)
	}
	if _, code := e.deploy(t, p, map[string]interface{}{}); code != 400 {
		t.Fatalf("deploy of a ref that is not allowed: %d", code)
	}
	tg := gitTarget{RefKind: "tag", Version: "v2.1", Branch: "main"}
	if !refAllowed(tg, []string{"refs/tags/v*"}) || !refAllowed(tg, []string{"v2.*"}) || refAllowed(tg, []string{"refs/heads/main"}) || !refAllowed(tg, nil) {
		t.Fatal("refAllowed tags")
	}
	if br := (gitTarget{RefKind: "branch", Branch: "main"}); !refAllowed(br, []string{"refs/heads/main"}) || refAllowed(br, []string{"refs/tags/*"}) {
		t.Fatal("refAllowed branches")
	}
	// typed confirmation for a production-like environment name
	if !(gitEnvironment{Name: "prod"}).needsConfirm() || (gitEnvironment{Name: "test"}).needsConfirm() {
		t.Fatal("needsConfirm")
	}

	// nested paths and state directories
	app := func(name, dir string, exclude []string, env map[string]interface{}) map[string]interface{} {
		env["name"] = "test"
		if env["destinations"] == nil {
			env["destinations"] = []map[string]string{{"server": "app-01", "path": dir}}
		}
		return map[string]interface{}{"name": name, "project": "demo/" + name, "exclude": exclude, "environments": []map[string]interface{}{env}}
	}
	put := func(apps ...map[string]interface{}) int {
		return e.u.jsonDo("PUT", "/api/git/catalog", map[string]interface{}{"apps": apps}, nil)
	}
	if code := put(app("demo-api", "/opt/demo", nil, map[string]interface{}{}), app("demo-web", "/opt/demo/web", nil, map[string]interface{}{})); code != 400 {
		t.Fatalf("nested: %d", code)
	}
	if code := put(app("demo-api", "/opt/demo", []string{"/web"}, map[string]interface{}{}), app("demo-web", "/opt/demo/web", nil, map[string]interface{}{})); code != 200 {
		t.Fatalf("nested but excluded: %d", code)
	}
	if code := put(app("demo-api", "/opt/demo", nil, map[string]interface{}{}), app("demo-web", "/opt/demo-web", nil, map[string]interface{}{"state_dir": "/opt/demo/state"})); code != 400 {
		t.Fatalf("state dir inside an app path: %d", code)
	}
	if code := put(app("demo-api", "/opt/demo", nil, map[string]interface{}{"state_dir": "/var/lib/wrm-state"})); code != 200 {
		t.Fatalf("state dir outside: %d", code)
	}
	if code := put(app("demo-api", "/opt/demo", nil, map[string]interface{}{"destinations": []map[string]string{{"server": "app-01", "path": "/etc"}}})); code != 400 {
		t.Fatalf("system directory: %d", code)
	}
	if code := put(app("demo-api", "/opt/demo", nil, map[string]interface{}{"post_deploy": "a\nb"})); code != 400 {
		t.Fatalf("post_deploy on two lines: %d", code)
	}
	if got := envStateDir(gitEnvironment{StateDir: "/var/lib/wrm-state"}, "demo-api", "/opt/demo"); got != "/var/lib/wrm-state/demo-api/opt_demo" {
		t.Fatalf("state dir: %s", got)
	}

	// type changes: excluding something inside, or not deleting it, is a plan error
	dp := &gitEnvDestPlan{Server: "app-01", Files: []gitEnvFile{{Path: "conf", State: "new"}, {Path: "conf/a.ini", State: "removed"}, {Path: "conf/b.ini", State: "removed"}},
		Conflicts: []gitEnvConflict{{Path: "conf", Kind: "dir_to_file", Server: []string{"conf/a.ini", "conf/b.ini"}}}}
	if _, _, err := envSelection(dp, map[string]bool{"conf/a.ini": true}, true); err == nil || !strings.Contains(err.Error(), "excluding conf/a.ini is not possible") {
		t.Fatalf("exclusion inside: %v", err)
	}
	if _, _, err := envSelection(dp, nil, false); err == nil || !strings.Contains(err.Error(), "must be deleted") {
		t.Fatalf("not deleted: %v", err)
	}
	if w, d, err := envSelection(dp, nil, true); err != nil || len(w) != 1 || len(d) != 2 {
		t.Fatalf("selection: %v %v %v", w, d, err)
	}
}

func TestGitEnvPartialTransferAndPostDeploy(t *testing.T) {
	e := newEnvTest(t, "env-partial", map[string]interface{}{"post_deploy": "echo post-$((1+1)); exit 3"})
	d0, d1 := e.dirs[0], e.dirs[1]
	writeFile(t, d0+"/run.py", runV19)
	old := gitEnvReceiveHook
	gitEnvReceiveHook = "printf 'E\\tthe connection was lost\\n'; exit 0\n"
	p := e.plan(t, "")
	v, _ := e.deploy(t, p, map[string]interface{}{})
	gitEnvReceiveHook = old
	it := v.Items[0]
	if v.State != "failed" || !it.Partial || it.MovedCount != 3 || !strings.Contains(v.Message, "3 of 3 files had arrived") || v.Items[1].State != "skipped" {
		t.Fatalf("partial: %+v %+v", v, it)
	}
	sd := d0 + "/.deploy-bak"
	if readFile(t, d0+"/run.py") != runV19 || fileExists(sd+"/state.json") || fileExists(d0+"/.wrm-incoming-"+it.DeployID) || fileExists(sd+"/.lock") {
		t.Fatal("a failed transfer must not change the installation or the state")
	}
	if h := readHistory(t, sd); len(h) != 1 || h[0]["result"] != "partial" || len(h[0]["moved"].([]interface{})) != 3 {
		t.Fatalf("partial history: %+v", h)
	}

	// post_deploy fails: noted, the files stay, the next destination is not started
	p = e.plan(t, "")
	v, _ = e.deploy(t, p, map[string]interface{}{"post_deploy": true})
	it = v.Items[0]
	if v.State != "failed" || it.State != "failed" || !strings.Contains(it.Note, "already on the server") || v.Items[1].State != "skipped" || !strings.HasPrefix(it.PostDeploy, "failed") {
		t.Fatalf("post_deploy: %+v %+v", v, it)
	}
	if !strings.Contains(logText(it), "post-2") || readFile(t, d0+"/run.py") != runV110 || fileExists(d1) {
		t.Fatalf("post_deploy log: %s", logText(it))
	}
	if st := readState(t, sd); st.Current == nil || st.Current.DeployID != it.DeployID {
		t.Fatalf("state after post_deploy failure: %+v", st)
	}
	if h := readHistory(t, sd); h[1]["result"] != "post_deploy_failed" || !strings.HasPrefix(h[1]["post_deploy"].(string), "failed") {
		t.Fatalf("history: %+v", h[1])
	}
	// post_deploy only on request and only when the environment has one
	if _, code := e.run(t, map[string]interface{}{"kind": "rollback", "env": map[string]interface{}{"app": "demo-api", "env": "test", "deploy_id": it.DeployID, "post_deploy": true, "dests": []int{9}}}); code != 400 {
		t.Fatalf("unknown destination: %d", code)
	}
}

func TestGitEnvRollbackSymlinksDirsAndBackups(t *testing.T) {
	e := newEnvTest(t, "env-rollback", map[string]interface{}{"destinations": "single", "keep_backups": 1})
	// v1.9 has legacy/old.py; v1.10 does not
	e.g.mu.Lock()
	c1 := e.g.tags["v1.9"]
	c1.files["linux/legacy/old.py"] = "OLD = 1\n"
	e.g.blobs[blobSHA("OLD = 1\n")] = "OLD = 1\n"
	e.g.mu.Unlock()
	db.Exec(`DELETE FROM git_trees`) // the refresh cached the old tree
	d0 := e.dirs[0]
	sd := d0 + "/.deploy-bak"

	p := e.plan(t, "tag:v1.9")
	if p.RefName != "refs/tags/v1.9" || fileState(p.Dests[0], "legacy/old.py") != "new" {
		t.Fatalf("plan v1.9: %+v", p)
	}
	v1, _ := e.deploy(t, p, map[string]interface{}{})
	if v1.State != "done" || v1.Kind != "upgrade" || readFile(t, d0+"/legacy/old.py") != "OLD = 1\n" {
		t.Fatalf("deploy v1.9: %+v %s", v1, logText(v1.Items[0]))
	}
	os.Chmod(d0+"/legacy", 0o750)
	writeFile(t, d0+"/notes.txt", "local\n")

	p = e.plan(t, "")
	if fileState(p.Dests[0], "legacy/old.py") != "removed" || fileState(p.Dests[0], "notes.txt") != "extra" || fileState(p.Dests[0], "run.py") != "changed" {
		t.Fatalf("plan v1.10: %+v", p.Dests[0].Files)
	}
	v2, _ := e.deploy(t, p, map[string]interface{}{"delete_removed": true})
	id := v2.Env.DeployID
	if v2.State != "done" || fileExists(d0+"/legacy") || readFile(t, d0+"/notes.txt") != "local\n" {
		t.Fatalf("deploy v1.10: %+v %s", v2, logText(v2.Items[0]))
	}
	// keep_backups 1: the backup of the first deploy is gone
	if fileExists(sd+"/"+v1.Env.DeployID) || fileExists(sd+"/deploys/"+v1.Env.DeployID+".json") || !fileExists(sd+"/"+id) {
		t.Fatal("backups not pruned")
	}
	var man gitEnvManifest
	json.Unmarshal([]byte(readFile(t, sd+"/deploys/"+id+".json")), &man)
	if len(man.RemovedDirs) != 1 || man.RemovedDirs[0].Path != "legacy" || man.RemovedDirs[0].Mode != "750" || man.Previous == nil || man.Previous.DeployID != v1.Env.DeployID {
		t.Fatalf("manifest: %+v", man)
	}
	if h := readHistory(t, sd); h[1]["previous"].(map[string]interface{})["deploy_id"] != v1.Env.DeployID || len(h[1]["deleted"].([]interface{})) != 1 {
		t.Fatalf("history previous: %+v", h[1])
	}
	// six older rollback backups: five are kept
	for i := 1; i <= 6; i++ {
		d := fmt.Sprintf("%s/backups/rollback-old%d", sd, i)
		os.MkdirAll(d, 0o755)
		at := time.Now().Add(-time.Duration(10-i) * time.Hour)
		os.Chtimes(d, at, at)
	}

	// run.py became a symlink on the server: skipped, the rollback is incomplete
	os.Remove(d0 + "/run.py")
	writeFile(t, filepath.Join(e.h.home, "elsewhere.py"), "print('elsewhere')\n")
	os.Symlink(filepath.Join(e.h.home, "elsewhere.py"), d0+"/run.py")
	v, _ := e.rollback(t, id, map[string]interface{}{})
	it := v.Items[0]
	if v.State != "failed" || len(it.Skipped) != 1 || it.Skipped[0] != "run.py" || !strings.Contains(v.Message, "incomplete") {
		t.Fatalf("incomplete rollback: %+v %+v", v, it)
	}
	if readFile(t, filepath.Join(e.h.home, "elsewhere.py")) != "print('elsewhere')\n" {
		t.Fatal("the rollback acted through a symlink")
	}
	if fi, err := os.Stat(d0 + "/legacy"); err != nil || fi.Mode().Perm() != 0o750 || readFile(t, d0+"/legacy/old.py") != "OLD = 1\n" {
		t.Fatalf("removed directory not re-created: %v %v", fi, err)
	}
	if st := readState(t, sd); st.Current == nil || st.Current.DeployID != id {
		t.Fatalf("an incomplete rollback must keep the state: %+v", st)
	}
	if h := readHistory(t, sd); h[len(h)-1]["complete"] != false {
		t.Fatalf("history: %+v", h[len(h)-1])
	}
	// fixed on the server: roll back again
	os.Remove(d0 + "/run.py")
	writeFile(t, d0+"/run.py", runV110)
	v, _ = e.rollback(t, id, map[string]interface{}{})
	if v.State != "done" || readFile(t, d0+"/run.py") != runV19 {
		t.Fatalf("second rollback: %+v %s", v, logText(v.Items[0]))
	}
	if st := readState(t, sd); st.Current == nil || st.Current.DeployID != v1.Env.DeployID || st.RolledBack != id {
		t.Fatalf("state: %+v", st)
	}
	list, _ := os.ReadDir(sd + "/backups")
	names := []string{}
	for _, x := range list {
		names = append(names, x.Name())
	}
	if len(list) != gitEnvKeepRollback || !strings.Contains(strings.Join(names, " "), "rollback-"+id) || strings.Contains(strings.Join(names, " "), "rollback-old1 ") {
		t.Fatalf("rollback backups: %v", names)
	}
	// the old backup list of the Update flow does not show the environment directories
	conn, _ := loadConnection(e.conn)
	cl, err := dialSSH(conn, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	bks, err := listBackups(cl, d0)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bks {
		if b.Name == "backups" || b.Name == "deploys" {
			t.Fatalf("listBackups: %+v", bks)
		}
	}
}

func TestGitEnvDoctorAndOverview(t *testing.T) {
	e := newEnvTest(t, "env-doctor", nil)
	writeFile(t, e.dirs[0]+"/run.py", runV19)
	os.MkdirAll(e.dirs[0]+"/.git", 0o755)
	var doc struct {
		Dests []gitDoctorDest `json:"dests"`
	}
	if code := e.u.jsonDo("POST", "/api/git/env/doctor", map[string]interface{}{"app": "demo-api", "env": "test"}, &doc); code != 200 || len(doc.Dests) != 2 {
		t.Fatalf("doctor: %d %+v", code, doc)
	}
	get := func(d gitDoctorDest, name string) gitDoctorCheck {
		for _, c := range d.Checks {
			if c.Name == name {
				return c
			}
		}
		return gitDoctorCheck{}
	}
	a, b := doc.Dests[0], doc.Dests[1]
	if get(a, "ssh").Status != "ok" || get(a, "shell").Status != "ok" || get(a, "path").Status != "ok" || get(a, "method").Status != "warn" || !strings.Contains(get(a, "method").Msg, ".git") {
		t.Fatalf("doctor dest 0: %+v", a.Checks)
	}
	if c := get(b, "path"); c.Status != "ok" || !strings.Contains(c.Msg, "nearest existing parent") {
		t.Fatalf("doctor missing path: %+v", b.Checks)
	}
	if get(a, "sha256sum").Status != "ok" || get(a, "lock").Status != "ok" || (get(a, "rsync").Status != "ok" && get(a, "rsync").Status != "info") {
		t.Fatalf("tools: %+v", a.Checks)
	}
	if !looksPublic("/var/www/demo") || !looksPublic("/srv/site/public_html") || looksPublic("/opt/demo-api") {
		t.Fatal("looksPublic")
	}
	var ov struct {
		Services []struct {
			App  string `json:"app"`
			Envs []struct {
				Name string `json:"name"`
			} `json:"envs"`
		} `json:"services"`
		Envs []string `json:"envs"`
	}
	if code := e.u.jsonDo("GET", "/api/git/env/overview", nil, &ov); code != 200 || len(ov.Services) != 1 || ov.Envs[0] != "test" {
		t.Fatalf("overview: %d %+v", code, ov)
	}
	// drift: the destinations run different versions. It is detected from the code, not VERSION.md.
	for _, d := range e.dirs {
		writeFile(t, d+"/lib/util.py", utilV1)
		writeFile(t, d+"/config.ini", "[main]\nhost=app-01.example.com\n")
	}
	writeFile(t, e.dirs[0]+"/run.py", runV110)
	writeFile(t, e.dirs[1]+"/run.py", runV19)
	writeFile(t, e.dirs[1]+"/VERSION.md", "# demo-api - v1.9\n\n- **Verzija**:     v1.9\n- **Commit**:      "+commitSHA("c1")[:10]+" (2026-09-01)\n")
	writeFile(t, e.dirs[0]+"/VERSION.md", "# demo-api - v1.10\n\n- **Verzija**:     v1.10\n- **Commit**:      "+commitSHA("c2")[:10]+" (2026-09-10)\n")
	var cell struct {
		Drift bool             `json:"drift"`
		Dests []gitEnvCellDest `json:"dests"`
	}
	if code := e.u.jsonDo("GET", "/api/git/env/cell?"+url.Values{"app": {"demo-api"}, "env": {"test"}}.Encode(), nil, &cell); code != 200 || !cell.Drift || cell.Dests[1].VersionM != "v1.9" || cell.Dests[1].Detected.Version != "v1.9" || cell.Dests[1].Behind != 1 {
		t.Fatalf("drift: %d %+v", code, cell)
	}
}

// histByDest finds a merged history entry by destination index. Entries are
// newest first, so two destinations deployed across a second boundary may come
// in either order.
func histByDest(entries []map[string]interface{}, dest int) map[string]interface{} {
	for _, e := range entries {
		if e["dest"] == float64(dest) {
			return e
		}
	}
	return nil
}
