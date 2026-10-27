// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitTemplatesAndRules(t *testing.T) {
	for in, want := range map[string]string{"config.ini.example": "config.ini", "app/.env.sample": "app/.env", "settings.example.py": "settings.py",
		"conf/db_template.yml": "conf/db.yml", "run.py": "", "example.py": "", "README.md": ""} {
		if got := templateDest(in); got != want {
			t.Errorf("templateDest(%s) = %q, want %q", in, got, want)
		}
	}
	tmpl := "# demo\r\n[db]\r\nhost = localhost\r\npassword = \"change me\"\r\nurl: https://{{ api_host }}/v1\r\nTOKEN=${API_TOKEN:-none}\r\nlist:\r\n  - a\r\n"
	fields := templateFields([]byte(tmpl))
	byName := map[string]tmplField{}
	for _, f := range fields {
		byName[f.Name] = f
	}
	if len(fields) != 4 || byName["api_host"].ID != "{{ api_host }}" || byName["API_TOKEN"].Default != "none" || !byName["API_TOKEN"].Secret ||
		byName["db.host"].Default != "localhost" || byName["db.password"].Default != "change me" || !byName["db.password"].Secret || byName["db.host"].Secret {
		t.Fatalf("fields: %+v", fields)
	}
	out, left := fillTemplate([]byte(tmpl), map[string]string{byName["db.password"].ID: `s3"cret`, "{{ api_host }}": "api.example.com"})
	want := "# demo\r\n[db]\r\nhost = localhost\r\npassword = \"s3\\\"cret\"\r\nurl: https://api.example.com/v1\r\nTOKEN=${API_TOKEN:-none}\r\nlist:\r\n  - a\r\n"
	if string(out) != want || len(left) != 1 || left[0] != "${API_TOKEN:-none}" {
		t.Fatalf("fill:\n%q\n%q %v", out, want, left)
	}
	if got := string(retargetVersionMD([]byte("# x\n- **Host**:        app-01\r\n- **Korisnik**:    old\n"), "app-02", "deploy")); got != "# x\n- **Host**:        app-02\r\n- **Korisnik**:    deploy\n" {
		t.Fatalf("VERSION.md: %q", got)
	}
	words := strings.Split(defaultBackupWords, ",")
	for rel, want := range map[string]string{"logs/app.txt": "log", "app.log": "log", "app.log.1": "log", "lib/__pycache__/x.pyc": "cache", "run.py.bak": "backup",
		"config_old.ini": "backup", "data_BKP/x": "backup", ".deploy-bak/a/run.py": "backup", "tmp_cache/x": "ignored", "run.py": "", "copy.py": "", "old_tools/x.py": "backup",
		"lib/util.py": "", ".env": "", "config.ini": ""} {
		if got := transferExcluded(rel, []string{"tmp_*"}, words); got != want {
			t.Errorf("transferExcluded(%s) = %q, want %q", rel, got, want)
		}
	}
	for p, ok := range map[string]bool{"/opt/demo-api": true, "/srv/x/y": true, "/opt": false, "/etc": false, "relative/x": false, "/proc/x/y": false, "/opt/../etc/x": false, "/usr/lib": false} {
		if _, err := validInstallDir(p); (err == nil) != ok {
			t.Errorf("validInstallDir(%s): %v", p, err)
		}
	}
	u := &gitUnitSpec{Kind: "systemd", Name: "demo-api.service", User: "svc", Command: "/usr/bin/python3 run.py --port 80%", Enable: true}
	if err := u.normalize("/opt/demo-api"); err != nil {
		t.Fatal(err)
	}
	if name, content := u.file(); name != "demo-api.service" || !strings.Contains(content, "User=svc\nWorkingDirectory=/opt/demo-api\nExecStart=/usr/bin/python3 run.py --port 80%%\n") {
		t.Fatalf("unit %s:\n%s", name, content)
	}
	for _, bad := range []*gitUnitSpec{{Kind: "cron", Name: "x", Command: "x"}, {Kind: "systemd", Name: "../x", Command: "x"}, {Kind: "supervisor", Name: "x", Command: "a\nb"}, {Kind: "systemd", Name: "x", User: "a b", Command: "x"}} {
		if bad.normalize("/opt/x") == nil {
			t.Errorf("unit accepted: %+v", bad)
		}
	}
}

// provisionEnv is deployEnv with a template in the repository and a second server.
func newProvisionEnv(t *testing.T, name string) (*deployEnv, *fakeHost, int) {
	e := newDeployEnv(t, name)
	tmpl := "DB_HOST=localhost\nDB_PASSWORD=\nAPI_URL=https://{{ api_host }}/v1\n"
	e.g.mu.Lock()
	e.g.blobs[blobSHA(tmpl)] = tmpl
	for _, list := range e.g.branches {
		for _, c := range list {
			c.files["linux/.env.example"] = tmpl
		}
	}
	e.g.mu.Unlock()
	b := startFakeHost(t, testSSHUser, testSSHPass)
	return e, b, e.u.addConnection("app-02", b.addr)
}

func (e *deployEnv) provision(t *testing.T, body map[string]interface{}) (*gitRunView, int, string) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp := e.u.do("POST", "/api/git/runs", bytes.NewReader(b), "application/json")
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode, string(data)
	}
	var v gitRunView
	json.Unmarshal(data, &v)
	return e.wait(t, v.ID), 200, ""
}

func TestGitInstallAndTransfer(t *testing.T) {
	e, hostB, connB := newProvisionEnv(t, "git-installer")
	os.MkdirAll(filepath.Dir(e.dir), 0o755)
	os.Remove(e.units + "/demo-api.service")
	if code := e.u.jsonDo("POST", "/api/git/targets/refresh", map[string]interface{}{}, nil); code != 200 {
		t.Fatalf("refresh: %d", code)
	}
	var cr credential
	if code := e.u.jsonDo("POST", "/api/credentials", map[string]interface{}{"name": "db@demo", "username": "demo", "password": "vault-s3cret"}, &cr); code != 200 {
		t.Fatalf("credential: %d", code)
	}

	// prepare: target, templates and pre-checks
	var prep struct {
		Roots    []gitRootInfo   `json:"roots"`
		Services []gitPrepareSvc `json:"services"`
	}
	req := map[string]interface{}{"kind": "install", "conn_id": e.conn, "services": []map[string]string{{"app": "demo-api", "path": e.dir}}}
	if code := e.u.jsonDo("POST", "/api/git/provision/prepare", req, &prep); code != 200 || len(prep.Services) != 1 || len(prep.Roots) != 1 || !prep.Roots[0].Exists {
		t.Fatalf("prepare: %d %+v", code, prep)
	}
	ps := prep.Services[0]
	slots := map[string]gitSlot{}
	for _, s := range ps.Slots {
		slots[s.Path] = s
	}
	env, cfg := slots[".env"], slots["config.ini"]
	if ps.Error != "" || ps.Target.Version != "v1.10" || ps.Files != 3 || ps.Check == nil || ps.Check.Exists || len(ps.Check.Errors) != 0 ||
		len(env.Templates) != 1 || len(env.Templates[0].Fields) != 3 || !cfg.InRepo || !cfg.Protected || len(cfg.Templates) != 1 || cfg.Templates[0].Fields[0].Name != "main.host" {
		t.Fatalf("prepared service: %+v", ps)
	}

	// install with a template (a vault secret), a typed file, a systemd unit and a start
	svc := map[string]interface{}{"app": "demo-api", "path": e.dir, "env": "test",
		"files": []map[string]interface{}{
			{"path": ".env", "mode": "template", "template": ".env.example", "values": map[string]interface{}{"line:2": map[string]int{"cred_id": cr.ID}, "{{ api_host }}": map[string]string{"value": "api.example.com"}}},
			{"path": "config.ini", "mode": "manual", "content": "[main]\nhost=typed.example.com\n"}},
		"unit": map[string]interface{}{"kind": "systemd", "name": "demo-api", "command": "/usr/bin/python3 run.py", "enable": true}}
	body := map[string]interface{}{"kind": "install", "provision": map[string]interface{}{"conn_id": e.conn, "services": []interface{}{svc}}, "restart": map[string]string{"mode": "now"}}
	v, code, msg := e.provision(t, body)
	if code != 200 || v.State != "done" || v.Items[0].State != "ok" || v.Items[0].InstallID == 0 {
		t.Fatalf("install: %d %s %+v", code, msg, v)
	}
	if readFile(t, e.dir+"/run.py") != runV110 || readFile(t, e.dir+"/lib/util.py") != utilV1 || readFile(t, e.dir+"/.env.example") == "" {
		t.Fatal("repository files")
	}
	if got := readFile(t, e.dir+"/.env"); got != "DB_HOST=localhost\nDB_PASSWORD=vault-s3cret\nAPI_URL=https://api.example.com/v1\n" {
		t.Fatalf(".env: %q", got)
	}
	if st, _ := os.Stat(e.dir + "/.env"); st.Mode().Perm() != 0o640 {
		t.Fatalf(".env mode %v", st.Mode())
	}
	if readFile(t, e.dir+"/config.ini") != "[main]\nhost=typed.example.com\n" || !strings.Contains(readFile(t, e.dir+"/VERSION.md"), "# demo-api - v1.10\n") {
		t.Fatal("config.ini or VERSION.md")
	}
	if !strings.Contains(readFile(t, v.Items[0].UpdateLog), `"install": "`+e.dir+`"`) || !strings.Contains(readFile(t, e.units+"/demo-api.service"), "WorkingDirectory="+e.dir+"\n") {
		t.Fatal("updates.jsonl or unit")
	}
	if got := withoutFunctionalityReads(readFile(t, e.sysLog)); got != "daemon-reload\nenable demo-api.service\nrestart demo-api.service\nis-active demo-api.service\n" {
		t.Fatalf("systemctl: %q", got)
	}
	if entries, _ := os.ReadDir(e.dir); len(entries) == 0 || strings.Contains(fmt.Sprint(entries), gitStagingPrefix) {
		t.Fatalf("staging left: %v", entries)
	}
	in := loadGitInstalls(e.u.userID, false, "i.id=?", v.Items[0].InstallID)[0]
	if in.State != "ok" || in.Env != "test" || len(in.Units) != 1 || !in.Manual {
		t.Fatalf("registered install: %+v", in)
	}
	var params string
	db.QueryRow(`SELECT params FROM git_runs WHERE id=?`, v.ID).Scan(&params)
	if strings.Contains(params, "typed.example.com") || strings.Contains(params, "api.example.com") || strings.Contains(params, "vault-s3cret") || !strings.Contains(params, `"cred_id"`) {
		t.Fatalf("stored params keep typed values: %s", params)
	}

	// an existing installation is refused, then replaced (backed up) when confirmed
	delete(body, "restart")
	svc["files"], svc["unit"] = nil, nil
	if v, _, _ = e.provision(t, body); v.State != "failed" || !strings.Contains(v.Items[0].Error, "already holds an installation") {
		t.Fatalf("existing target: %+v", v.Items[0])
	}
	body["provision"].(map[string]interface{})["overwrite"] = true
	if _, code, _ = e.provision(t, body); code != 400 {
		t.Fatalf("unconfirmed overwrite: %d", code)
	}
	writeFile(t, e.dir+"/run.py", "print('local')\n")
	body["provision"].(map[string]interface{})["confirm"] = "APP-01"
	if v, _, msg = e.provision(t, body); v == nil || v.State != "done" || v.Items[0].MadeBackup == "" {
		t.Fatalf("overwrite: %s %+v", msg, v)
	}
	if readFile(t, v.Items[0].MadeBackup+"/run.py") != "print('local')\n" || readFile(t, e.dir+"/run.py") != runV110 || readFile(t, e.dir+"/.env") == "" {
		t.Fatal("overwrite backup")
	}

	// install from an imported bundle (protected files have no payload there)
	resp := e.u.do("POST", "/api/git/bundles/export", strings.NewReader(`{"apps":["demo-api"]}`), "application/json")
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	resp = e.u.do("POST", "/api/git/bundles", bytes.NewReader(data), "application/gzip")
	var imp struct {
		Result bundleImportResult `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&imp)
	resp.Body.Close()
	dirB := filepath.Join(filepath.Dir(e.dir), "demo-api-bundle")
	bsvc := map[string]interface{}{"app": "demo-api", "ref": fmt.Sprintf("bundle:%d", imp.Result.Source.ID), "path": dirB}
	req["services"] = []interface{}{bsvc}
	if code := e.u.jsonDo("POST", "/api/git/provision/prepare", req, &prep); code != 200 || prep.Services[0].Target == nil || !strings.HasPrefix(prep.Services[0].Target.Source, "bundle:") {
		t.Fatalf("bundle prepare: %d %+v", code, prep.Services)
	}
	for _, s := range prep.Services[0].Slots {
		if s.Path == "config.ini" && (len(s.Templates) != 1 || s.Templates[0].Available || len(s.Copies) == 0) {
			t.Fatalf("bundle slot: %+v", s)
		}
	}
	bsvc["files"] = []map[string]interface{}{{"path": "config.ini", "mode": "copy", "from_install": in.ID}, {"path": ".env", "mode": "skip"}}
	if v, _, msg = e.provision(t, map[string]interface{}{"kind": "install", "provision": map[string]interface{}{"conn_id": e.conn, "services": []interface{}{bsvc}}}); v == nil || v.State != "done" {
		t.Fatalf("bundle install: %s %+v", msg, v)
	}
	if readFile(t, dirB+"/run.py") != runV110 || readFile(t, dirB+"/config.ini") != "[main]\nhost=typed.example.com\n" {
		t.Fatal("bundle install files")
	}
	if _, err := os.Stat(dirB + "/.env"); err == nil {
		t.Fatal("a skipped file was written")
	}

	// transfer A → B: per-host files come along, logs, backups and caches do not
	for rel, content := range map[string]string{"logs/app.txt": "x", "app.log": "x", "lib/__pycache__/util.cpython-311.pyc": "x", "run.py.bak": "x", "data_BKP/a.txt": "x", "notes.txt": "keep"} {
		writeFile(t, e.dir+"/"+rel, content)
	}
	target := filepath.Join(hostB.home, "srv", "demo-api")
	var tp struct {
		Source map[string]interface{} `json:"source"`
		Check  gitDirCheck            `json:"check"`
	}
	treq := map[string]interface{}{"kind": "transfer", "conn_id": connB, "source_install": in.ID, "path": target}
	if code := e.u.jsonDo("POST", "/api/git/provision/prepare", treq, &tp); code != 200 || tp.Source["excluded"].(float64) != 4 || len(tp.Check.Errors) != 0 {
		t.Fatalf("transfer prepare: %d %+v", code, tp)
	}
	tbody := map[string]interface{}{"kind": "transfer", "provision": map[string]interface{}{"conn_id": connB, "source_install": in.ID, "path": target,
		"unit": map[string]string{"kind": "supervisor", "name": "demo-b", "command": "python3 run.py"}}}
	v, code, msg = e.provision(t, tbody)
	if code != 200 || v.State != "done" || v.Items[0].SourcePath != e.dir || v.Items[0].ConnName != "app-02" {
		t.Fatalf("transfer: %d %s %+v", code, msg, v)
	}
	for _, rel := range []string{"run.py", "lib/util.py", ".env", "config.ini", "notes.txt", "VERSION.md"} {
		if readFile(t, target+"/"+rel) != readFile(t, e.dir+"/"+rel) && rel != "VERSION.md" {
			t.Errorf("%s differs after the transfer", rel)
		}
	}
	for _, rel := range []string{"logs", "app.log", "lib/__pycache__", "run.py.bak", "data_BKP", ".deploy-bak"} {
		if _, err := os.Stat(target + "/" + rel); err == nil {
			t.Errorf("%s was transferred", rel)
		}
	}
	tin := loadGitInstalls(e.u.userID, false, "i.id=?", v.Items[0].InstallID)
	if len(tin) != 1 || tin[0].ConnID != connB || tin[0].App != "demo-api" || tin[0].Env != "test" || len(tin[0].Units) != 1 || tin[0].Units[0].Name != "demo-b" || tin[0].RestartPending == "" {
		t.Fatalf("transferred install: %+v", tin)
	}
	if !strings.Contains(readFile(t, e.units+"/demo-b.conf"), "[program:demo-b]\ncommand=python3 run.py\ndirectory="+target+"\n") {
		t.Fatal("supervisor program")
	}
	// the target holds an installation now: refused without a confirmed overwrite
	tbody["provision"].(map[string]interface{})["unit"] = nil
	if v, _, _ = e.provision(t, tbody); v.State != "failed" || !strings.Contains(v.Items[0].Error, "already holds") {
		t.Fatalf("second transfer: %+v", v.Items[0])
	}

	// audit and policies
	for _, act := range []string{"git.install", "git.transfer"} {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action=?`, act).Scan(&n)
		if n == 0 {
			t.Errorf("no audit entry %s", act)
		}
	}
	db.Exec(`UPDATE users SET is_admin=0 WHERE id=?`, e.u.userID)
	defer db.Exec(`UPDATE users SET is_admin=1 WHERE id=?`, e.u.userID)
	if code := e.u.jsonDo("POST", "/api/git/provision/prepare", treq, nil); code != 403 {
		t.Fatalf("transfer policy: %d", code)
	}
	if _, code, _ := e.provision(t, body); code != 403 {
		t.Fatalf("install policy: %d", code)
	}
	setSetting("git_transfer", "all")
	defer setSetting("git_transfer", "admins")
	if code := e.u.jsonDo("POST", "/api/git/provision/prepare", treq, nil); code != 200 {
		t.Fatalf("transfer policy all: %d", code)
	}
}
