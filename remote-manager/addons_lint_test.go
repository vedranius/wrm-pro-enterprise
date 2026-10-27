// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Addon platform v2, step 3: lint, templates, schema, llms.txt and the dev
// server.

func lintCodes(rep addonLintReport) map[string]addonDiag {
	out := map[string]addonDiag{}
	for _, d := range rep.Diagnostics {
		if _, ok := out[d.Code]; !ok {
			out[d.Code] = d
		}
	}
	return out
}

func TestAddonLintTemplates(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "lint-tpl-admin", true)
	user := newTestUser(t, srv, "lint-tpl-user", false)
	for _, tpl := range addonTemplateList() {
		body, err := addonTemplateZip(tpl.Name, "")
		if err != nil {
			t.Fatalf("%s: %v", tpl.Name, err)
		}
		rep := addonLintArchive(context.Background(), body)
		if !rep.OK || rep.Warnings != 0 {
			t.Fatalf("template %s does not lint clean: %+v", tpl.Name, rep.Diagnostics)
		}
		// Every template installs: server code for the organisation, the
		// rest as somebody's personal addon.
		id := "lt." + strings.ReplaceAll(tpl.Name, "-", "")
		body, err = addonTemplateZip(tpl.Name, id)
		if err != nil {
			t.Fatal(err)
		}
		who := user
		if tpl.Server {
			who = admin
		}
		if r := addonPost(who, "/api/addons/install", body); r.Code != 200 || r.Body["id"] != id {
			t.Fatalf("template %s does not install: %d %s", tpl.Name, r.Code, r.Raw)
		}
	}
	if _, err := addonTemplateZip("blank", "Bad Id"); err == nil {
		t.Fatal("a bad id was accepted")
	}
	if _, err := addonTemplateZip("../etc", ""); err == nil {
		t.Fatal("an unknown template was accepted")
	}
	// The placeholder module answers through the real runner.
	if r := addonReq(admin, "POST", "/api/addons/lt.wasmhook/enable", ""); r.Code != 200 {
		t.Fatalf("enable hook: %d %s", r.Code, r.Raw)
	}
	if r := addonReq(admin, "POST", "/api/addons/lt.wasmhook/call/status", "{}"); r.Code != 200 || !strings.Contains(r.Raw, `ok`) {
		t.Fatalf("call hook: %d %s", r.Code, r.Raw)
	}
	// wrm -addon-new writes the same files and refuses a folder in use.
	dir := filepath.Join(t.TempDir(), "my-addon")
	var out bytes.Buffer
	if code := runAddonNew("fetch-and-render", "me.status", dir, &out); code != 0 {
		t.Fatalf("addon-new: %d %s", code, out.String())
	}
	if code := runAddonNew("blank", "", dir, &out); code == 0 {
		t.Fatal("addon-new wrote into a folder that is not empty")
	}
	out.Reset()
	if code := runAddonLint(dir, true, &out); code != 0 {
		t.Fatalf("addon-lint of a fresh template: %d %s", code, out.String())
	}
	var rep addonLintReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || rep.ID != "me.status" || !rep.OK {
		t.Fatalf("lint JSON: %v %s", err, out.String())
	}
}

func TestAddonLintFindings(t *testing.T) {
	ctx := context.Background()
	manifest := `{
  "apiVersion": 2,
  "id": "lint.bad",
  "name": "Bad",
  "version": "1.0.0",
  "capabilities": ["ui.panel", "http"],
  "ui": {"panel": "panel.html"},
  "http": {"origins": ["https://api.example.com"], "secrets": ["api_token"]}
}`
	panel := `<!doctype html>
<script src="https://cdn.example.net/lib.js"></script>
<button onclick="go()">Go</button>
<a href="javascript:go()">x</a>
<img src="https://cdn.example.net/logo.png">
<script>
const apiKey = "abcdefghijklmnop1234";
const token = "glsa_abcdefghijklmnopqrstuvwxyz012345";
fetch('https://other.example.org/data');
localStorage.setItem('x', 1);
eval('1+1');
window.parent.postMessage({v:2, type:'wrm:http', id:'1', url:'https://api.example.com/x', secret:'api_token'}, '*');
window.parent.postMessage({v:2, type:'wrm:notify', text:'hi'}, '*');
</script>`
	css := "@import url('https://fonts.example.net/a.css');\n"
	rep := addonLintArchive(ctx, addonZip(t, map[string]string{"addon.json": manifest, "panel.html": panel, "style.css": css, "notes.bin": "x"}))
	if rep.OK || rep.Errors != 2 {
		t.Fatalf("want 2 errors (remote script, token): %+v", rep.Diagnostics)
	}
	want := map[string]int{
		"E_LINT_REMOTE": 2, "E_LINT_INLINE_HANDLER": 3, "E_LINT_JS_URL": 4, "E_LINT_SECRET": 8,
		"E_LINT_NETWORK": 9, "E_LINT_ORIGIN": 9, "E_LINT_STORAGE": 10, "E_LINT_EVAL": 11,
		"E_LINT_READY": 0, "E_LINT_HEARTBEAT": 0, "E_LINT_CAPABILITY": 0, "E_LINT_UNSERVED": 0,
	}
	got := lintCodes(rep)
	for code, line := range want {
		d, ok := got[code]
		if !ok {
			t.Errorf("missing %s: %+v", code, rep.Diagnostics)
			continue
		}
		if line > 0 && d.Line != line {
			t.Errorf("%s on line %d, want %d", code, d.Line, line)
		}
		if d.Hint == "" {
			t.Errorf("%s has no hint", code)
		}
	}
	// The secret named in wrm:http is not a leaked secret; the real token is.
	tokens := 0
	for _, d := range rep.Diagnostics {
		if d.Code == "E_LINT_SECRET" {
			tokens++
			if d.Line == 12 {
				t.Errorf("a declared secret name was reported: %+v", d)
			}
			if strings.Contains(d.Message, "abcdefghijklmnopqrstuvwxyz") {
				t.Errorf("the report repeats the token: %s", d.Message)
			}
		}
	}
	if tokens != 2 {
		t.Errorf("want 2 secret findings, got %d", tokens)
	}
	css2 := lintCodes(addonLintArchive(ctx, addonZip(t, map[string]string{"addon.json": manifest, "panel.html": "<p>wrm:ready wrm:heartbeat</p>", "style.css": css})))
	if d := css2["E_LINT_REMOTE"]; d.File != "style.css" || d.Line != 1 {
		t.Errorf("remote CSS: %+v", d)
	}

	// Manifest errors carry a line: unknown field, invalid JSON, a rule.
	for _, tc := range []struct {
		manifest, code string
		line           int
	}{
		{"{\n \"apiVersion\": 2,\n \"id\": \"lint.x\",\n \"nmae\": \"x\"\n}", "E_MANIFEST_FIELD", 4},
		{"{\n \"apiVersion\": 2,\n \"id\": \"lint.x\",,\n}", "E_MANIFEST", 3},
		{"{\n \"apiVersion\": 2,\n \"id\": \"lint.x\",\n \"name\": \"X\",\n \"version\": \"1\",\n \"capabilities\": [\"ui.panel\", \"telepathy\"],\n \"ui\": {\"panel\": \"p.html\"}\n}", "E_MANIFEST", 6},
		{`{"apiVersion":2,"id":"lint.x","name":"X","version":"1","capabilities":["ui.panel"],"ui":{"panel":"missing.html"}}`, "E_MISSING_FILE", 0},
		{`{"apiVersion":2,"id":"dev","name":"X","version":"1","capabilities":["ui.panel"],"ui":{"panel":"p.html"}}`, "E_MANIFEST", 0},
	} {
		rep := addonLintArchive(ctx, addonZip(t, map[string]string{"addon.json": tc.manifest, "p.html": "<p>x</p>"}))
		if rep.OK || len(rep.Diagnostics) != 1 || rep.Diagnostics[0].Code != tc.code || (tc.line > 0 && rep.Diagnostics[0].Line != tc.line) {
			t.Errorf("%s: %+v", tc.code, rep.Diagnostics)
		}
	}

	// A folder: hidden entries are left out and listed; a symlink refuses.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "addon.json"), []byte(`{"apiVersion":2,"id":"lint.dir","name":"D","version":"1","capabilities":["ui.panel"],"ui":{"panel":"p.html"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "p.html"), []byte("<script>parent.postMessage({v:2,type:'wrm:ready'},'*');setInterval(()=>parent.postMessage({v:2,type:'wrm:heartbeat'},'*'),15000)</script>"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("x"), 0o644)
	rep = addonLintPath(ctx, dir)
	if !rep.OK || lintCodes(rep)["E_LINT_HIDDEN"].File != ".git" || rep.Files != 2 {
		t.Fatalf("folder lint: %+v", rep)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "link.txt")); err == nil {
		if rep := addonLintPath(ctx, dir); rep.OK {
			t.Fatal("a symbolic link was accepted")
		}
	}
}

// The schema must describe every manifest field, so an AI that follows it
// never writes a field WRM rejects or misses one it accepts.
func TestAddonSchemaCoversManifest(t *testing.T) {
	raw, err := addonTemplatesFS.ReadFile("addon_templates/_docs/addon.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
	props := func(node map[string]interface{}) map[string]interface{} {
		p, _ := node["properties"].(map[string]interface{})
		return p
	}
	check := func(where string, typ reflect.Type, node map[string]interface{}) {
		p := props(node)
		for i := 0; i < typ.NumField(); i++ {
			tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			if tag == "" || tag == "-" {
				continue
			}
			if _, ok := p[tag]; !ok {
				t.Errorf("schema %s lacks %q", where, tag)
			}
		}
		if len(p) != typ.NumField() {
			t.Errorf("schema %s has %d properties, the Go type %d", where, len(p), typ.NumField())
		}
	}
	top := props(schema)
	defs := schema["$defs"].(map[string]interface{})
	check("manifest", reflect.TypeOf(addonManifest{}), schema)
	check("ui", reflect.TypeOf(addonUI{}), top["ui"].(map[string]interface{}))
	check("store", reflect.TypeOf(addonStore{}), top["store"].(map[string]interface{}))
	check("http", reflect.TypeOf(addonHTTP{}), top["http"].(map[string]interface{}))
	check("server", reflect.TypeOf(addonServer{}), top["server"].(map[string]interface{}))
	check("theme", reflect.TypeOf(addonTheme{}), defs["theme"].(map[string]interface{}))
	check("size", reflect.TypeOf(addonSize{}), defs["size"].(map[string]interface{}))
	caps := top["capabilities"].(map[string]interface{})["items"].(map[string]interface{})["enum"].([]interface{})
	if len(caps) != len(addonCapabilities) {
		t.Errorf("schema capabilities %v, WRM knows %d", caps, len(addonCapabilities))
	}
	for _, c := range caps {
		if _, ok := addonCapabilities[c.(string)]; !ok {
			t.Errorf("schema capability %v is unknown", c)
		}
	}
	llms := addonLLMsText()
	for _, code := range []string{"E_LINT_REMOTE", "E_HTTP_BLOCKED", "wrm:ready", "wrm:http", AppVersion} {
		if !strings.Contains(llms, code) {
			t.Errorf("llms.txt lacks %s", code)
		}
	}
	for code := range addonLintHints {
		if !strings.Contains(llms, code) {
			t.Errorf("llms.txt does not list %s", code)
		}
	}
}

func TestAddonDevAPI(t *testing.T) {
	srv := newTestServer(t)
	user := newTestUser(t, srv, "lint-api-user", false)
	get := func(path string) (*http.Response, []byte) {
		resp := user.do("GET", path, nil, "")
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, b
	}
	if resp, b := get("/api/addons/dev/schema.json"); resp.StatusCode != 200 || !json.Valid(b) {
		t.Fatalf("schema: %d", resp.StatusCode)
	}
	if resp, b := get("/api/addons/dev/llms.txt"); resp.StatusCode != 200 || !strings.Contains(string(b), "# WRM PRO addons") {
		t.Fatalf("llms.txt: %d", resp.StatusCode)
	}
	if r := addonReq(user, "GET", "/api/addons/dev/templates", ""); r.Code != 200 || !strings.Contains(r.Raw, "grafana-panel") {
		t.Fatalf("templates: %d %s", r.Code, r.Raw)
	}
	resp, b := get("/api/addons/dev/templates/grafana-panel.zip?id=me.graf")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("template zip: %d", resp.StatusCode)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if strings.Join(names, ",") != "README.md,addon.json,panel.html,panel.js,style.css,wrm-bridge.js" {
		t.Fatalf("template files: %v", names)
	}
	if resp, _ := get("/api/addons/dev/templates/..%2Fx.zip"); resp.StatusCode != 404 {
		t.Fatalf("unknown template: %d", resp.StatusCode)
	}
	r := addonPost(user, "/api/addons/dev/lint", b)
	if r.Code != 200 || r.Body["ok"] != true || r.Body["id"] != "me.graf" {
		t.Fatalf("lint: %d %s", r.Code, r.Raw)
	}
	// Lint follows the personal-addon policy; reading the docs does not.
	setSetting("addon_personal", "admins")
	defer setSetting("addon_personal", "all")
	if r := addonPost(user, "/api/addons/dev/lint", b); r.Code != 403 || r.Body["code"] != "E_PERSONAL_OFF" {
		t.Fatalf("lint with personal addons off: %d %s", r.Code, r.Raw)
	}
	if resp, _ := get("/api/addons/dev/llms.txt"); resp.StatusCode != 200 {
		t.Fatalf("llms.txt with personal addons off: %d", resp.StatusCode)
	}
	if resp := httptestGet(t, srv.URL+"/api/addons/dev/llms.txt"); resp != 401 {
		t.Fatalf("llms.txt without a session: %d", resp)
	}
}

func httptestGet(t *testing.T, url string) int {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestAddonDevServer(t *testing.T) {
	up := addonUpstream(t, false, nil)
	dir := t.TempDir()
	files, err := addonTemplateFiles("fetch-and-render", "dev.test")
	if err != nil {
		t.Fatal(err)
	}
	files["addon.json"] = bytes.Replace(files["addon.json"], []byte("https://status.example.com"), []byte(up.URL), 1)
	files["addon.json"] = bytes.Replace(files["addon.json"], []byte(`"/api/*"`), []byte(`"/api/echo"`), 1)
	for rel, b := range files {
		os.WriteFile(filepath.Join(dir, rel), b, 0o644)
	}
	os.MkdirAll(filepath.Join(dir, ".wrm-dev"), 0o755)
	os.WriteFile(filepath.Join(dir, ".wrm-dev", "secret.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("x"), 0o644)

	d := &addonDevServer{dir: dir, token: "tok", store: map[string]json.RawMessage{}}
	hs := httptest.NewServer(d)
	defer hs.Close()
	d.port = hs.Listener.Addr().(*net.TCPAddr).Port
	call := func(method, path, token, host, body string) (int, string, http.Header) {
		req, _ := http.NewRequest(method, hs.URL+path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("X-WRM-Dev", token)
		}
		if host != "" {
			req.Host = host
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), resp.Header
	}
	if code, body, h := call("GET", "/", "", "", ""); code != 200 || !strings.Contains(body, "'tok'") || !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("host page: %d", code)
	}
	// Another host name (DNS rebinding) and a missing token are refused.
	if code, _, _ := call("GET", "/", "", "evil.example.com", ""); code != http.StatusMisdirectedRequest {
		t.Fatalf("foreign host: %d", code)
	}
	if code, _, _ := call("GET", "/__dev/state", "", "", ""); code != 403 {
		t.Fatalf("no token: %d", code)
	}
	code, body, _ := call("GET", "/__dev/state", "tok", "", "")
	if code != 200 || !strings.Contains(body, `"ok":true`) || !strings.Contains(body, `"api_token":false`) {
		t.Fatalf("state: %d %s", code, body)
	}
	code, _, h := call("GET", "/addon/panel.html", "", "", "")
	if code != 200 || !strings.Contains(h.Get("Content-Security-Policy"), "connect-src 'none'") {
		t.Fatalf("panel: %d %v", code, h)
	}
	for _, p := range []string{"/addon/.env", "/addon/.wrm-dev/secret.txt", "/addon/../addons.go", "/addon/addon.json/../../x"} {
		if code, _, _ := call("GET", p, "", "", ""); code != 404 {
			t.Fatalf("%s: %d", p, code)
		}
	}
	// Store in memory.
	if code, body, _ := call("PUT", "/__dev/store/config", "tok", "", `{"every":15}`); code != 200 {
		t.Fatalf("store put: %d %s", code, body)
	}
	if _, body, _ := call("GET", "/__dev/store", "tok", "", ""); !strings.Contains(body, `"every":15`) {
		t.Fatalf("store get: %s", body)
	}
	if code, _, _ := call("PUT", "/__dev/store/wrm.x", "tok", "", `1`); code != 400 {
		t.Fatalf("reserved key: %d", code)
	}
	// Web requests follow the manifest; the fake site is plain http on
	// 127.0.0.1, so it needs -addon-dev-private, and a missing secret is
	// reported.
	req := `{"method":"GET","url":"` + up.URL + `/api/echo"}`
	if code, body, _ := call("POST", "/__dev/http", "tok", "", req); code != 403 || !strings.Contains(body, "E_HTTP_SCHEME") {
		t.Fatalf("private address without the flag: %d %s", code, body)
	}
	d.allowPrivate = true
	if code, body, _ := call("POST", "/__dev/http", "tok", "", `{"method":"GET","url":"`+up.URL+`/api/other"}`); code != 403 || !strings.Contains(body, "E_HTTP_PATH") {
		t.Fatalf("path outside http.paths: %d %s", code, body)
	}
	if code, body, _ := call("POST", "/__dev/http", "tok", "", `{"method":"GET","url":"`+up.URL+`/api/echo","secret":"api_token"}`); code != 409 || !strings.Contains(body, "E_SECRET_MISSING") {
		t.Fatalf("missing secret: %d %s", code, body)
	}
	t.Setenv("WRM_ADDON_SECRET_API_TOKEN", "dev-value-1")
	code, body, _ = call("POST", "/__dev/http", "tok", "", `{"method":"GET","url":"`+up.URL+`/api/echo","secret":"api_token"}`)
	if code != 200 || !strings.Contains(body, `Bearer dev-value-1`) {
		t.Fatalf("proxied with secret: %d %s", code, body)
	}
	// A change on disk is a new version with a new report.
	_, before, _ := call("GET", "/__dev/state", "tok", "", "")
	os.WriteFile(filepath.Join(dir, "panel.js"), []byte("fetch('/x'); wrm.ready();"), 0o644)
	_, after, _ := call("GET", "/__dev/state", "tok", "", "")
	if before == after || !strings.Contains(after, "E_LINT_NETWORK") {
		t.Fatalf("no reload after a change: %s", after)
	}
}
