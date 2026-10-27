// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// addonZip builds an archive from path->content pairs.
func addonZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func addonDemoManifest(id string) string {
	return fmt.Sprintf(`{"id":%q,"name":"Demo panel","version":"1.0.0","description":"test addon","author":"tests","license":"MIT","capabilities":["ui.panel","ui.theme"],"ui":{"panel":"panel.html","theme":"theme.css","title":"Demo"}}`, id)
}

func addonByID(list []addonRecord, id string) *addonRecord {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

func TestV130AddonInstallEnableServeRemove(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "addon-admin", true)
	user := newTestUser(t, srv, "addon-user", false)
	zipBody := addonZip(t, map[string]string{
		"addon.json":  addonDemoManifest("demo.panel"),
		"panel.html":  `<!doctype html><body><p id="marker">demo panel</p></body>`,
		"theme.css":   ":root{--accent:#123456}",
		"logo.svg":    `<svg xmlns="http://www.w3.org/2000/svg"></svg>`,
		"notes.txt":   "not served as html",
		"secret.json": "{}",
	})
	install := func(c *testClient) int {
		resp := c.do("POST", "/api/addons/install", bytes.NewReader(zipBody), "application/zip")
		defer resp.Body.Close()
		return resp.StatusCode
	}
	// v13.9: a normal user installs personal addons only while addon_personal
	// allows it (covered in addons_v2_test.go); here the policy is off.
	setSetting("addon_personal", "admins")
	defer setSetting("addon_personal", "all")
	if code := install(user); code != 403 {
		t.Fatalf("a normal user installed an addon: %d", code)
	}
	if code := install(admin); code != 200 {
		t.Fatalf("install: %d", code)
	}
	var list struct {
		Addons []addonRecord `json:"addons"`
	}
	if code := admin.jsonDo("GET", "/api/addons", nil, &list); code != 200 {
		t.Fatalf("list: %d", code)
	}
	if rec := addonByID(list.Addons, "demo.panel"); rec == nil || rec.Enabled {
		t.Fatalf("unexpected list: %+v", list.Addons)
	} else if rec.Checksum == "" {
		t.Fatal("the install did not record the archive checksum")
	}
	// A normal user does not see a disabled addon.
	var userList struct {
		Addons []addonRecord `json:"addons"`
	}
	if code := user.jsonDo("GET", "/api/addons", nil, &userList); code != 200 || addonByID(userList.Addons, "demo.panel") != nil {
		t.Fatalf("disabled addon was visible to a normal user: %d %+v", code, userList.Addons)
	}
	getAsset := func(c *testClient, rel string) (int, string, string) {
		resp := c.do("GET", "/api/addons/demo.panel/asset/"+rel, nil, "")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw), resp.Header.Get("Content-Security-Policy")
	}
	if code, _, _ := getAsset(user, "panel.html"); code != 404 {
		t.Fatalf("a disabled addon served files: %d", code)
	}
	if code := admin.jsonDo("POST", "/api/addons/demo.panel/enable", nil, nil); code != 200 {
		t.Fatalf("enable: %d", code)
	}
	code, body, csp := getAsset(user, "panel.html")
	if code != 200 || !strings.Contains(body, "demo panel") {
		t.Fatalf("panel: %d %q", code, body)
	}
	if !strings.Contains(csp, "sandbox") {
		t.Fatalf("panel html has no sandbox CSP: %q", csp)
	}
	if code, css, _ := getAsset(user, "theme.css"); code != 200 || !strings.Contains(css, "--accent") {
		t.Fatalf("theme: %d %q", code, css)
	}
	// Only known asset types are served, and nothing outside the folder.
	if code, _, _ := getAsset(user, "payload.bin"); code != 400 {
		t.Fatalf("an unknown file type was served: %d", code)
	}
	if code, _, _ := getAsset(user, "%2e%2e%2f%2e%2e%2fmain.go"); code == 200 {
		t.Fatal("path traversal left the addon folder")
	}
	if addonSafeRel("../addon.json") || addonSafeRel("/etc/passwd") {
		t.Fatal("addonSafeRel accepted an escaping path")
	}
	if code := user.jsonDo("GET", "/api/addons", nil, &userList); code != 200 || addonByID(userList.Addons, "demo.panel") == nil {
		t.Fatalf("enabled addon missing for a normal user: %+v", userList.Addons)
	}
	if code := admin.jsonDo("POST", "/api/addons/demo.panel/disable", nil, nil); code != 200 {
		t.Fatalf("disable: %d", code)
	}
	if code, _, _ := getAsset(admin, "panel.html"); code != 404 {
		t.Fatalf("a disabled addon still served files: %d", code)
	}
	resp := admin.do("DELETE", "/api/addons/demo.panel", nil, "")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("remove: %d", resp.StatusCode)
	}
	if code, _, _ := getAsset(admin, "panel.html"); code != 404 {
		t.Fatalf("a removed addon still served files: %d", code)
	}
}

func TestV130AddonAcceptsBOM(t *testing.T) {
	// An editor on Windows writes addon.json with a BOM; the installer accepts it.
	body := addonZip(t, map[string]string{
		"addon.json": "\xef\xbb\xbf" + addonDemoManifest("demo.bom"),
		"panel.html": "<p>bom</p>",
		"theme.css":  "",
	})
	m, _, err := addonReadArchive(body)
	if err != nil || m.ID != "demo.bom" {
		t.Fatalf("a BOM refused the manifest: %v %+v", err, m)
	}
}

func TestV130AddonUIHooks(t *testing.T) {
	page, _ := os.ReadFile("static/index.html")
	js, _ := os.ReadFile("static/addons.js")
	p, a := string(page), string(js)
	for _, want := range []string{
		`data-tab="st-addons"`, `id="addons-pane"`, `src="/static/addons.js"`,
		`if (id === 'st-addons') wrmAddons.pane();`,
		`wo.mode === 'addon-panel'`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	for _, want := range []string{
		`frame.setAttribute('sandbox', 'allow-scripts')`, // never allow-same-origin
		`'wrm:notify'`, `'wrm:context'`, `applyThemes`, `/api/addons/`, `add_disclaimer`,
		`wrmAddons.run`, `add_run_d`, `/call/`,
		`wrmAddons.aiPrompt`, `add_ai_q`, `openAIHome`, `wrm_alloc(size)`, `postMessage`,
	} {
		if !strings.Contains(a, want) {
			t.Errorf("addons.js lacks %s", want)
		}
	}
	if strings.Contains(a, "allow-same-origin") {
		t.Error("a panel must never get allow-same-origin")
	}
}

func TestV130AddonRejectsUnsafeArchives(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "addon-bad", true)
	install := func(body []byte) int {
		resp := admin.do("POST", "/api/addons/install", bytes.NewReader(body), "application/zip")
		resp.Body.Close()
		return resp.StatusCode
	}
	cases := map[string][]byte{
		"traversal": addonZip(t, map[string]string{
			"addon.json":     addonDemoManifest("bad.traversal"),
			"panel.html":     "<p>x</p>",
			"theme.css":      "",
			"../../evil.txt": "x",
		}),
		"absolute": addonZip(t, map[string]string{
			"addon.json": addonDemoManifest("bad.absolute"),
			"panel.html": "<p>x</p>",
			"theme.css":  "",
			"/etc/evil":  "x",
		}),
		"no manifest": addonZip(t, map[string]string{
			"panel.html": "<p>x</p>",
			"theme.css":  "",
		}),
		"unsupported capability": addonZip(t, map[string]string{
			"addon.json": `{"id":"bad.cap","name":"Bad","version":"1","capabilities":["server.native"]}`,
			"panel.html": "<p>x</p>",
		}),
		"server code": addonZip(t, map[string]string{
			"addon.json": `{"id":"bad.server","name":"Bad","version":"1","capabilities":["ui.panel"],"ui":{"panel":"panel.html"},"server":{"module":"x.wasm"}}`,
			"panel.html": "<p>x</p>",
		}),
		"missing entry file": addonZip(t, map[string]string{
			"addon.json": addonDemoManifest("bad.missing"),
			"theme.css":  "",
		}),
	}
	for name, body := range cases {
		if code := install(body); code != 400 {
			t.Fatalf("%s was accepted: %d", name, code)
		}
	}
	for _, a := range addonRecords(false) {
		if strings.HasPrefix(a.ID, "bad.") {
			t.Fatalf("a refused archive left a record: %+v", a)
		}
	}
	if _, err := os.Stat(addonDir("bad.traversal")); err == nil {
		t.Fatal("a refused archive left its folder")
	}
}
