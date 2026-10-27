// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"
)

// history.jsonl and state.json of two destinations of environment "test" (written on the
// fake SSH host as deploys would leave them), a WRM run and an installation without
// environment.
func setupWhere(t *testing.T, name string) *envTest {
	e := newEnvTest(t, name, nil)
	e.u.addConnection("app-02", e.h.addr)
	env := e.cat["apps"].([]map[string]interface{})[0]["environments"].([]map[string]interface{})[0]
	env["destinations"] = []map[string]string{{"server": "app-01", "path": e.dirs[0]}, {"server": "app-02", "path": e.dirs[1]}}
	if code := e.u.jsonDo("PUT", "/api/git/catalog", e.cat, nil); code != 200 {
		t.Fatalf("catalog: %d", code)
	}
	c1, c2 := commitSHA("c1"), commitSHA("c2")
	line := func(m map[string]interface{}) string { b, _ := json.Marshal(m); return string(b) + "\n" }
	d1 := func(at string) map[string]interface{} {
		return map[string]interface{}{"action": "DEPLOY", "id": "wrm-d1", "app": "demo-api", "env": "test", "version": "v1.9", "commit": c1, "branch": "main", "by": "ana", "at": at,
			"new": []string{"run.py", "lib/util.py"}, "changed": []string{}, "deleted": []string{}, "counts": map[string]int{"new": 2}, "result": "ok"}
	}
	d2 := func(at string) map[string]interface{} {
		return map[string]interface{}{"action": "DEPLOY", "id": "wrm-d2", "app": "demo-api", "env": "test", "version": "v1.10", "commit": c2, "branch": "main", "by": "bo", "at": at,
			"previous": map[string]string{"version": "v1.9", "deploy_id": "wrm-d1"}, "changed": []string{"run.py"}, "counts": map[string]int{"changed": 1}, "result": "ok"}
	}
	rb := map[string]interface{}{"action": "ROLLBACK", "id": "rollback-wrm-d2-1", "rolled_back": "wrm-d2", "app": "demo-api", "env": "test", "by": "ana", "at": "2026-09-21T08:00:00Z",
		"from": map[string]string{"version": "v1.10"}, "to": map[string]string{"version": "v1.9"}, "restored": []string{"run.py"}, "complete": true,
		"counts": map[string]int{"restored": 1}, "version": "v1.9", "commit": c1, "branch": "main"}
	writeFile(t, e.dirs[0]+"/.deploy-bak/history.jsonl", line(d1("2026-09-10T10:00:00Z"))+line(d2("2026-09-20T10:00:00Z"))+"not json\n")
	writeFile(t, e.dirs[1]+"/.deploy-bak/history.jsonl", line(d1("2026-09-10T10:00:30Z"))+line(d2("2026-09-20T10:01:00Z"))+line(rb))
	state := func(id, v, c, by string) string {
		b, _ := json.Marshal(gitEnvState{Current: &gitEnvRecord{DeployID: id, App: "demo-api", Env: "test", Version: v, Commit: c, Branch: "main", By: by, At: "2026-09-20T10:00:00Z"}})
		return string(b)
	}
	writeFile(t, e.dirs[0]+"/.deploy-bak/state.json", state("wrm-d2", "v1.10", c2, "bo"))
	writeFile(t, e.dirs[1]+"/.deploy-bak/state.json", state("wrm-d1", "v1.9", c1, "ana"))

	// a run on an installation without environment, an environment run and a scheduled run
	tg, _ := loadGitTarget(e.u.userID, "demo-api")
	items := []*gitRunItem{{ConnID: e.conn, ConnName: "app-01", App: "demo-api", Path: "/opt/demo-legacy", Env: "prod", State: "ok", Written: []string{"run.py"}, FromVersion: "v1.9"}}
	params := gitRunParams{Kind: "update", Items: items, Targets: map[string]gitTarget{"demo-api": tg}}
	for _, r := range []struct {
		kind, state string
		p           gitRunParams
	}{{"update", "done", params}, {"update", "done", gitRunParams{Kind: "update", Items: items, Env: &gitEnvRun{App: "demo-api", Env: "test"}}}, {"update", "scheduled", params}} {
		db.Exec(`INSERT INTO git_runs (user_id, kind, state, label, params, result, created_at, started_at, ended_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			e.u.userID, r.kind, r.state, "wrm-20260915-1000", string(jsonMarshal(r.p)), string(jsonMarshal(r.p.Items)), "2026-09-15T10:00:00Z", "2026-09-15T10:00:00Z", "2026-09-15T10:02:00Z")
	}
	// installations: one without environment, one that is a destination (not listed twice)
	data := gitInstallData{VersionMD: gitVersionInfo{Version: "v1.9", Commit: c1[:10] + " (2026-09-01)", Ref: "tag:v1.9", User: "ana", Updated: "2026-09-01 10:00"}}
	for _, p := range []string{"/opt/demo-legacy", e.dirs[0]} {
		db.Exec(`INSERT INTO git_installs (user_id, conn_id, app, path, env, state, data, checked_at, discovered_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			e.u.userID, e.conn, "demo-api", p, "prod", "update", string(jsonMarshal(data)), nowStamp(), nowStamp())
	}
	return e
}

func historyQ(v url.Values) string {
	v.Set("app", "demo-api")
	v.Set("env", "test")
	return v.Encode()
}

func TestGitDeployHistory(t *testing.T) {
	e := setupWhere(t, "hist-user")
	var runs struct {
		Rows []gitHistoryRow `json:"rows"`
	}
	if code := e.u.jsonDo("GET", "/api/git/history/runs", nil, &runs); code != 200 || len(runs.Rows) != 1 {
		t.Fatalf("runs: %d %+v", code, runs.Rows)
	}
	r := runs.Rows[0]
	if r.Action != "update" || r.Source != "run" || r.Server != "app-01" || r.Env != "prod" || r.Version != "v1.10" || r.Branch != "main" || r.Previous != "v1.9" ||
		strings.Join(r.Files["written"], ",") != "run.py" || !strings.Contains(r.CommitURL, "/demo/demo-api/-/commit/"+commitSHA("c2")) || r.By != "hist-user" {
		t.Fatalf("run row: %+v", r)
	}
	// history.jsonl of both destinations merged, newest first, one row per destination
	var cell gitHistoryCell
	if code := e.u.jsonDo("GET", "/api/git/history/cell?"+historyQ(url.Values{}), nil, &cell); code != 200 || len(cell.Rows) != 5 || len(cell.Errors) != 0 {
		t.Fatalf("cell: %d %+v", code, cell)
	}
	if cell.Rows[0].Action != "rollback" || cell.Rows[0].Server != "app-02" || cell.Rows[0].Previous != "v1.10" || cell.Rows[0].Version != "v1.9" || strings.Join(cell.Rows[0].Files["restored"], ",") != "run.py" {
		t.Fatalf("rollback row: %+v", cell.Rows[0])
	}
	if a, b := cell.Rows[1], cell.Rows[2]; a.Server != "app-02" || b.Server != "app-01" || a.ID != "wrm-d2" || b.Previous != "v1.9" || a.Counts["changed"] != 1 {
		t.Fatalf("deploy rows: %+v %+v", a, b)
	}
	last := cell.Rows[4]
	if last.ID != "wrm-d1" || last.Server != "app-01" || strings.Join(last.Files["new"], ",") != "run.py,lib/util.py" || last.Path != e.dirs[0] || last.CommitURL == "" {
		t.Fatalf("first deploy: %+v", last)
	}
	// filters
	for _, c := range []struct {
		q    url.Values
		want int
	}{
		{url.Values{"who": {"BO"}}, 2}, {url.Values{"action": {"rollback"}}, 1}, {url.Values{"action": {"deploy"}}, 4}, {url.Values{"version": {"v1.9"}}, 3},
		{url.Values{"version": {commitSHA("c2")[:8]}}, 2}, {url.Values{"server": {"app-02"}}, 3}, {url.Values{"branch": {"mai"}}, 5}, {url.Values{"branch": {"release"}}, 0},
		{url.Values{"since": {"2026-09-15"}}, 3}, {url.Values{"until": {"2026-09-15"}}, 2}, {url.Values{"since": {"2026-09-20"}, "until": {"2026-09-20"}}, 2},
	} {
		var fc gitHistoryCell
		if code := e.u.jsonDo("GET", "/api/git/history/cell?"+historyQ(c.q), nil, &fc); code != 200 || len(fc.Rows) != c.want {
			t.Errorf("filter %v: %d rows %d, want %d", c.q, code, len(fc.Rows), c.want)
		}
	}
	if code := e.u.jsonDo("GET", "/api/git/history/cell?"+historyQ(url.Values{"action": {"explode"}}), nil, nil); code != 400 {
		t.Fatalf("bad action: %d", code)
	}
	var fr struct {
		Rows []gitHistoryRow `json:"rows"`
	}
	if e.u.jsonDo("GET", "/api/git/history/runs?action=rollback", nil, &fr); len(fr.Rows) != 0 {
		t.Fatalf("runs filter: %+v", fr.Rows)
	}
	// the cell is cached: a new line shows only with refresh
	f, _ := os.OpenFile(e.dirs[0]+"/.deploy-bak/history.jsonl", os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(`{"action":"DEPLOY","id":"wrm-d3","by":"cy","at":"2026-09-25T10:00:00Z","version":"v2.0","result":"partial","moved":["run.py"],"counts":{"moved":1,"total":3}}` + "\n")
	f.Close()
	e.u.jsonDo("GET", "/api/git/history/cell?"+historyQ(url.Values{}), nil, &cell)
	if len(cell.Rows) != 5 {
		t.Fatalf("not cached: %d", len(cell.Rows))
	}
	e.u.jsonDo("GET", "/api/git/history/cell?"+historyQ(url.Values{"refresh": {"1"}}), nil, &cell)
	if len(cell.Rows) != 6 || cell.Rows[0].Result != "partial" || cell.Rows[0].Counts["total"] != 3 {
		t.Fatalf("refresh: %+v", cell.Rows[0])
	}
	// CSV: every source, filtered
	resp := e.u.do("GET", "/api/git/history/export.csv", nil, "")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\ufeff"))).ReadAll()
	if resp.StatusCode != 200 || err != nil || len(recs) != 8 || strings.Join(recs[0], ",") != strings.Join(historyCSVHeader, ",") {
		t.Fatalf("csv: %d %v %d", resp.StatusCode, err, len(recs))
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[h] = i
	}
	files := ""
	for _, r := range recs[1:] {
		if r[col["deploy_id"]] == "wrm-d1" && r[col["server"]] == "app-01" {
			files = r[col["files"]]
		}
	}
	if files != "new: run.py\nnew: lib/util.py" {
		t.Fatalf("csv files: %q", files)
	}
	resp = e.u.do("GET", "/api/git/history/export.csv?who=bo&app=demo-api", nil, "")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if recs, _ = csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\ufeff"))).ReadAll(); len(recs) != 3 {
		t.Fatalf("csv filtered: %d", len(recs))
	}
	// the SSH parts need the checks policy; the runs do not
	setSetting("git_checks", "off")
	defer setSetting("git_checks", "all")
	if code := e.u.jsonDo("GET", "/api/git/history/cell?"+historyQ(url.Values{}), nil, nil); code != 403 {
		t.Fatalf("policy: %d", code)
	}
	if code := e.u.jsonDo("GET", "/api/git/where/cell?"+historyQ(url.Values{}), nil, nil); code != 403 {
		t.Fatalf("policy where: %d", code)
	}
	if code := e.u.jsonDo("GET", "/api/git/history/runs", nil, &runs); code != 200 || len(runs.Rows) != 1 {
		t.Fatalf("runs without checks: %d", code)
	}
	resp = e.u.do("GET", "/api/git/history/export.csv", nil, "")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if recs, _ = csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\ufeff"))).ReadAll(); len(recs) != 2 {
		t.Fatalf("csv without checks: %d", len(recs))
	}
}

func TestGitWhatIsWhere(t *testing.T) {
	e := setupWhere(t, "where-user")
	var cell gitWhereCell
	if code := e.u.jsonDo("GET", "/api/git/where/cell?"+historyQ(url.Values{}), nil, &cell); code != 200 || len(cell.Rows) != 2 {
		t.Fatalf("cell: %d %+v", code, cell)
	}
	a, b := cell.Rows[0], cell.Rows[1]
	c1, c2, c4 := commitSHA("c1"), commitSHA("c2"), commitSHA("c4")
	// main's head is c4: c2 is two commits behind, c1 three
	if a.Server != "app-01" || a.Branch != "main" || a.Commit != c2 || a.Version != "v1.10" || a.By != "bo" || a.DeployID != "wrm-d2" || a.Behind != 2 || a.Head != c4 ||
		!strings.HasSuffix(a.CompareURL, "/demo/demo-api/-/compare/"+c2+"..."+c4) || a.Drift {
		t.Fatalf("dest 0: %+v", a)
	}
	if b.Server != "app-02" || b.Commit != c1 || b.Behind != 3 || !b.Drift || !cell.Drift || b.CommitURL == "" || b.BranchURL == "" {
		t.Fatalf("dest 1: %+v drift %v", b, cell.Drift)
	}
	// installations without environment: from the stored check, a tag falls back to the service's branch
	var ic gitWhereCell
	if code := e.u.jsonDo("GET", "/api/git/where/installs?app=demo-api", nil, &ic); code != 200 || len(ic.Rows) != 1 {
		t.Fatalf("installs: %d %+v", code, ic)
	}
	if r := ic.Rows[0]; r.Path != "/opt/demo-legacy" || r.Kind != "install" || r.Env != "prod" || r.Branch != "main" || r.Commit != c1[:10] || r.Behind != 3 || r.State != "update" || r.By != "ana" {
		t.Fatalf("install row: %+v", r)
	}
	if code := e.u.jsonDo("GET", "/api/git/where/installs?app=nope", nil, nil); code != 404 {
		t.Fatalf("unknown service: %d", code)
	}
	// the same versions everywhere: no drift; cached until refresh
	writeFile(t, e.dirs[1]+"/.deploy-bak/state.json", string(jsonMarshal(gitEnvState{Current: &gitEnvRecord{DeployID: "wrm-d2", Version: "v1.10", Commit: c2, Branch: "main", By: "bo"}})))
	e.u.jsonDo("GET", "/api/git/where/cell?"+historyQ(url.Values{}), nil, &cell)
	if !cell.Drift {
		t.Fatal("not cached")
	}
	cell = gitWhereCell{}
	e.u.jsonDo("GET", "/api/git/where/cell?"+historyQ(url.Values{"refresh": {"1"}}), nil, &cell)
	if cell.Drift || cell.Rows[1].Drift || cell.Rows[1].Behind != 2 {
		t.Fatalf("refresh: %+v", cell)
	}
	// CSV of everything, and filtered by server
	resp := e.u.do("GET", "/api/git/where/export.csv", nil, "")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\ufeff"))).ReadAll()
	if resp.StatusCode != 200 || err != nil || len(recs) != 4 || strings.Join(recs[0], ",") != strings.Join(whereCSVHeader, ",") || recs[1][11] != "2" {
		t.Fatalf("csv: %d %v %v", resp.StatusCode, err, recs)
	}
	resp = e.u.do("GET", "/api/git/where/export.csv?server=app-02", nil, "")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if recs, _ = csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\ufeff"))).ReadAll(); len(recs) != 2 || recs[1][2] != "app-02" {
		t.Fatalf("csv server filter: %v", recs)
	}
	// helpers
	if branchOf("refs/heads/release/1.x", "main") != "release/1.x" || branchOf("tag:v1.9", "main") != "main" || branchOf("refs/tags/v1", "dev") != "dev" || branchOf("feature (abc)", "main") != "feature" {
		t.Fatal("branchOf")
	}
	rows := []gitWhereRow{{Exists: true, Commit: "a", Version: "1"}, {Exists: true, Commit: "b", Version: "2"}, {Exists: true, Commit: "b", Version: "2"}, {Exists: false}}
	if !markDrift(rows) || !rows[0].Drift || rows[1].Drift || rows[3].Drift {
		t.Fatalf("markDrift: %+v", rows)
	}
}
