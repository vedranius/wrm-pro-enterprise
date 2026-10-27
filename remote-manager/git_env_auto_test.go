// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestCleanHostname(t *testing.T) {
	for in, want := range map[string]string{
		"demo-web-01\n": "demo-web-01", "  app.example.com \nignored": "app.example.com", "": "", "bad host": "", "-x": "", "a;rm -rf /": "", "web_1": "web_1",
	} {
		if got := cleanHostname(in); got != want {
			t.Errorf("cleanHostname(%q) = %q, want %q", in, got, want)
		}
	}
}

// Installations found by the checker become derived environments in Simple mode only;
// manual definitions win, adoption stores them and is audited.
func TestGitAutoEnvironmentsAndHostnames(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "auto-user", true)
	uid := u.userID
	conn := u.addConnection("app-01", "10.0.0.5")
	u.addConnection("demo-web", "demo-web.example.com")
	app := gitCatalogApp{Name: "demo-api", Project: "demo/demo-api", Fingerprint: []string{"run.py"}}
	saveCat := func(envs ...gitEnvironment) {
		cat := loadGitCatalog(uid)
		a := app
		a.Environments = envs
		cat.Apps = []gitCatalogApp{a}
		if err := cat.normalize(); err != nil {
			t.Fatal(err)
		}
		saveGitWorkspace(uid, cat, nil)
	}
	saveCat()
	ins := func(conn int, p, env string) {
		db.Exec(`INSERT INTO git_installs (user_id, conn_id, app, path, env, state, data, checked_at, discovered_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			uid, conn, "demo-api", p, env, "ok", "", nowStamp(), nowStamp())
	}
	ins(conn, "/srv/demo-api", "test")
	ins(conn, "/srv/demo-api-2", "test")
	ins(conn, "/opt/demo-api_prod", "prod")
	ins(conn, "/opt/gone", "old")
	db.Exec(`UPDATE git_installs SET state='gone' WHERE path='/opt/gone'`)

	type ov struct {
		Services []struct {
			App  string `json:"app"`
			Envs []struct {
				Name  string       `json:"name"`
				Auto  bool         `json:"auto"`
				Dests []gitEnvDest `json:"destinations"`
				Conf  bool         `json:"confirm"`
			} `json:"envs"`
		} `json:"services"`
		Auto  int               `json:"auto"`
		Hosts map[string]string `json:"hosts"`
	}
	var plain, auto ov
	if code := u.jsonDo("GET", "/api/git/env/overview", nil, &plain); code != 200 || len(plain.Services) != 0 || plain.Auto != 0 {
		t.Fatalf("Advanced overview must list stored environments only: %d %+v", code, plain)
	}
	if code := u.jsonDo("GET", "/api/git/env/overview?auto=1", nil, &auto); code != 200 || len(auto.Services) != 1 || auto.Auto != 2 {
		t.Fatalf("auto overview: %d %+v", code, auto)
	}
	envs := auto.Services[0].Envs
	if len(envs) != 2 || envs[0].Name != "prod" || !envs[0].Auto || !envs[0].Conf || envs[1].Name != "test" || len(envs[1].Dests) != 2 {
		t.Fatalf("derived environments: %+v", envs)
	}
	if auto.Hosts["demo-web"] != "demo-web.example.com" {
		t.Fatalf("host fallback: %v", auto.Hosts)
	}
	storeHostname(conn, "demo-api-01.example.com")
	if h := hostLabels(uid)["app-01"]; h != "demo-api-01.example.com" {
		t.Fatalf("cached hostname: %q", h)
	}

	// a stored environment of the same name or with the same destination wins
	saveCat(gitEnvironment{Name: "test", Destinations: []gitEnvDest{{Server: "app-01", Path: "/srv/demo-api"}}})
	auto = ov{}
	if code := u.jsonDo("GET", "/api/git/env/overview?auto=1", nil, &auto); code != 200 || auto.Auto != 1 || len(auto.Services[0].Envs) != 2 {
		t.Fatalf("manual priority: %d %+v", code, auto)
	}
	if auto.Services[0].Envs[0].Name != "test" || auto.Services[0].Envs[0].Auto || len(auto.Services[0].Envs[0].Dests) != 1 {
		t.Fatalf("the stored test environment must stay as defined: %+v", auto.Services[0].Envs[0])
	}

	// adoption stores the rest, once, and is audited
	var res struct {
		Environments int `json:"environments"`
	}
	if code := u.jsonDo("POST", "/api/git/env/adopt", map[string]interface{}{}, &res); code != 200 || res.Environments != 1 {
		t.Fatalf("adopt: %d %+v", code, res)
	}
	cat := loadGitCatalog(uid)
	if a, _ := cat.app("demo-api"); len(a.Environments) != 2 || a.Environments[1].Name != "prod" || a.Environments[1].Auto {
		t.Fatalf("catalog after adoption: %+v", a.Environments)
	}
	if code := u.jsonDo("POST", "/api/git/env/adopt", map[string]interface{}{}, &res); code != 200 || res.Environments != 0 {
		t.Fatalf("adopt again: %d %+v", code, res)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='git.env_adopted'`).Scan(&n)
	if n != 2 {
		t.Fatalf("audit entries: %d", n)
	}
	if strings.Contains(strings.ToLower(string(jsonMarshal(res))), "password") {
		t.Fatal("unexpected")
	}
}
