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

// Addon platform v2: manifest, personal addons, instances, store, windows,
// CSP, integrity and the permission sheet.

type addonResp struct {
	Code   int
	Body   map[string]interface{}
	Header map[string][]string
	Raw    string
}

func addonPost(c *testClient, path string, zipBody []byte) addonResp {
	resp := c.do("POST", path, bytes.NewReader(zipBody), "application/zip")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := addonResp{Code: resp.StatusCode, Header: resp.Header, Raw: string(raw)}
	json.Unmarshal(raw, &out.Body)
	return out
}

func addonReq(c *testClient, method, path, body string) addonResp {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	resp := c.do(method, path, rd, "application/json")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := addonResp{Code: resp.StatusCode, Header: resp.Header, Raw: string(raw)}
	json.Unmarshal(raw, &out.Body)
	return out
}

func addonV2Manifest(id, version, extra string) string {
	return fmt.Sprintf(`{"apiVersion":2,"id":%q,"name":"Demo v2","version":%q,"license":"MIT",
		"capabilities":["ui.panel","ui.theme","store","notify"],
		"ui":{"panel":"panel.html","title":"Demo v2","defaultSize":{"w":640,"h":400},"minSize":{"w":320,"h":200},
		"themes":[{"id":"ocean","name":"Ocean","name_hr":"Ocean","mode":"dark","tokens":{"--bg":"#0b1220","--accent":"#38bdf8"},"css":"ocean.css"}]}%s}`, id, version, extra)
}

const addonV2Panel = `<!doctype html><html><body><p id="marker">v2 panel</p>
<script>window.parent.postMessage({v:2,type:'wrm:ready'},'*');</script></body></html>`

func addonV2Zip(t *testing.T, id, version, extra string) []byte {
	return addonZip(t, map[string]string{
		"addon.json": addonV2Manifest(id, version, extra),
		"panel.html": addonV2Panel,
		"ocean.css":  ":root{--bg:#0b1220}",
	})
}

func TestAddonV2ManifestValidation(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "a2-val-admin", true)
	cases := []struct {
		name, manifest, code string
		files                map[string]string
	}{
		{"unknown field", `{"apiVersion":2,"id":"v2.bad1","name":"x","version":"1","capabilities":["ui.panel"],"ui":{"panel":"p.html"},"orgins":["https://example.com"]}`, "E_MANIFEST_FIELD", nil},
		{"unknown nested field", `{"apiVersion":2,"id":"v2.bad2","name":"x","version":"1","capabilities":["ui.panel"],"ui":{"panel":"p.html","colour":"red"}}`, "E_MANIFEST_FIELD", nil},
		{"future api", `{"apiVersion":3,"id":"v2.bad3","name":"x","version":"1","capabilities":["ui.panel"],"ui":{"panel":"p.html"}}`, "E_API_VERSION", nil},
		{"store on v1", `{"id":"v2.bad4","name":"x","version":"1","capabilities":["ui.panel","store"],"ui":{"panel":"p.html"}}`, "E_MANIFEST", nil},
		{"store quota", `{"apiVersion":2,"id":"v2.bad5","name":"x","version":"1","capabilities":["ui.panel","store"],"ui":{"panel":"p.html"},"store":{"quotaKb":4096}}`, "E_MANIFEST", nil},
		{"token value", `{"apiVersion":2,"id":"v2.bad6","name":"x","version":"1","capabilities":["ui.theme"],"ui":{"themes":[{"id":"t","name":"T","mode":"dark","tokens":{"--bg":"url(https://example.com/x)"}}]}}`, "E_MANIFEST", nil},
		{"token name", `{"apiVersion":2,"id":"v2.bad7","name":"x","version":"1","capabilities":["ui.theme"],"ui":{"themes":[{"id":"t","name":"T","mode":"dark","tokens":{"color":"red"}}]}}`, "E_MANIFEST", nil},
		{"theme mode", `{"apiVersion":2,"id":"v2.bad8","name":"x","version":"1","capabilities":["ui.theme"],"ui":{"themes":[{"id":"t","name":"T","mode":"dim","tokens":{"--bg":"#000"}}]}}`, "E_MANIFEST", nil},
		{"themes need capability", `{"apiVersion":2,"id":"v2.bad9","name":"x","version":"1","capabilities":["ui.panel"],"ui":{"panel":"p.html","themes":[{"id":"t","name":"T","mode":"dark","tokens":{"--bg":"#000"}}]}}`, "E_MANIFEST", nil},
		{"missing theme css", `{"apiVersion":2,"id":"v2.bad10","name":"x","version":"1","capabilities":["ui.theme"],"ui":{"themes":[{"id":"t","name":"T","mode":"dark","css":"t.css"}]}}`, "E_MISSING_FILE", map[string]string{}},
	}
	for _, tc := range cases {
		files := map[string]string{"addon.json": tc.manifest, "p.html": "<p>x</p>"}
		for k, v := range tc.files {
			files[k] = v
		}
		r := addonPost(admin, "/api/addons/install", addonZip(t, files))
		if r.Code != 400 || r.Body["code"] != tc.code {
			t.Errorf("%s: want 400 %s, got %d %s", tc.name, tc.code, r.Code, r.Raw)
		}
		if r.Code == 400 && r.Body["hint"] == "" {
			t.Errorf("%s: coded error without a hint: %s", tc.name, r.Raw)
		}
	}
	// A themes-only addon is valid and its themes reach the list unchanged.
	r := addonPost(admin, "/api/addons/install", addonZip(t, map[string]string{
		"addon.json": `{"apiVersion":2,"id":"v2.themes","name":"Themes","version":"1","capabilities":["ui.theme"],"ui":{"themes":[{"id":"sun","name":"Sun","name_hr":"Sunce","mode":"light","tokens":{"--bg":"#ffffff","--font-ui":"Inter, system-ui, sans-serif"}}]}}`,
	}))
	if r.Code != 200 {
		t.Fatalf("themes-only addon: %d %s", r.Code, r.Raw)
	}
	rec, ok := addonGet("v2.themes")
	if !ok || rec.UI == nil || len(rec.UI.Themes) != 1 || rec.UI.Themes[0].NameHR != "Sunce" || rec.UI.Themes[0].Tokens["--bg"] != "#ffffff" {
		t.Fatalf("themes not recorded: %+v", rec.UI)
	}
	if strings.Join(rec.Permissions, ",") != "themes:1" {
		t.Fatalf("permissions: %v", rec.Permissions)
	}
	addonRemoveCore(admin.userID, "v2.themes")
}

func TestAddonV2PersonalAddons(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "a2-pers-admin", true)
	alice := newTestUser(t, srv, "a2-pers-alice", false)
	bob := newTestUser(t, srv, "a2-pers-bob", false)
	setSetting("addon_personal", "all")
	r := addonPost(alice, "/api/addons/install", addonV2Zip(t, "demo.mine", "1.0.0", ""))
	if r.Code != 200 || r.Body["key"] != "~demo.mine" || r.Body["enabled"] != false {
		t.Fatalf("personal install: %d %s", r.Code, r.Raw)
	}
	if _, err := os.Stat(filepath.Join(addonsRoot(), ".users", fmt.Sprint(alice.userID), "demo.mine", "addon.json")); err != nil {
		t.Fatalf("personal folder: %v", err)
	}
	if _, ok := addonGet("demo.mine"); ok {
		t.Fatal("a personal addon became an organisation addon")
	}
	var list struct {
		Addons []addonRecord `json:"addons"`
		Others []addonRecord `json:"personal_others"`
	}
	bob.jsonDo("GET", "/api/addons", nil, &list)
	if addonByID(list.Addons, "demo.mine") != nil {
		t.Fatal("another user sees a personal addon")
	}
	if r := addonReq(alice, "POST", "/api/addons/~demo.mine/enable", ""); r.Code != 200 {
		t.Fatalf("owner enables: %d %s", r.Code, r.Raw)
	}
	if r := addonReq(alice, "GET", "/api/addons/~demo.mine/asset/panel.html", ""); r.Code != 200 || !strings.Contains(r.Raw, "v2 panel") {
		t.Fatalf("owner asset: %d", r.Code)
	}
	// "~demo.mine" names Bob's own personal addon for Bob: there is none.
	if r := addonReq(bob, "GET", "/api/addons/~demo.mine/asset/panel.html", ""); r.Code != 404 {
		t.Fatalf("another user reached a personal addon: %d", r.Code)
	}
	if r := addonReq(bob, "POST", "/api/addons/demo.mine/enable", ""); r.Code != 403 {
		t.Fatalf("a user managed an organisation addon: %d", r.Code)
	}
	list.Addons, list.Others = nil, nil
	admin.jsonDo("GET", "/api/addons?personal=all", nil, &list)
	var seen *addonRecord
	for i := range list.Others {
		if list.Others[i].ID == "demo.mine" && list.Others[i].OwnerID == alice.userID {
			seen = &list.Others[i]
		}
	}
	if seen == nil || seen.Owner != "a2-pers-alice" {
		t.Fatalf("admin overview misses the personal addon: %+v", list.Others)
	}
	if r := addonReq(admin, "POST", fmt.Sprintf("/api/addons/users/%d/demo.mine/disable", alice.userID), ""); r.Code != 200 {
		t.Fatalf("admin kill switch: %d %s", r.Code, r.Raw)
	}
	if rec, _ := addonGetRef(addonRef{Owner: alice.userID, ID: "demo.mine"}); rec.Enabled {
		t.Fatal("the kill switch left the addon enabled")
	}
	// Personal addons carry no server code.
	wasmZip := addonZip(t, map[string]string{
		"addon.json":  `{"id":"demo.mywasm","name":"w","version":"1","capabilities":["server.wasm"],"server":{"module":"p.wasm","exports":["run"]}}`,
		"p.wasm":      "\x00asm\x01\x00\x00\x00",
		"placeholder": "x",
	})
	if r := addonPost(alice, "/api/addons/install", wasmZip); r.Code != 400 || r.Body["code"] != "E_PERSONAL_SERVER" {
		t.Fatalf("personal server code: %d %s", r.Code, r.Raw)
	}
	setSetting("addon_personal", "off")
	if r := addonPost(bob, "/api/addons/install", addonV2Zip(t, "demo.bob", "1.0.0", "")); r.Code != 403 || r.Body["code"] != "E_PERSONAL_OFF" {
		t.Fatalf("policy off: %d %s", r.Code, r.Raw)
	}
	if r := addonReq(alice, "POST", "/api/addons/~demo.mine/enable", ""); r.Code != 403 {
		t.Fatalf("enable while the policy is off: %d", r.Code)
	}
	setSetting("addon_personal", "all")
	// An administrator may install a personal addon too.
	if r := addonPost(admin, "/api/addons/install?scope=personal", addonV2Zip(t, "demo.mine", "1.0.0", "")); r.Code != 200 || r.Body["key"] != "~demo.mine" {
		t.Fatalf("admin personal install: %d %s", r.Code, r.Raw)
	}
	if r := addonReq(alice, "DELETE", "/api/addons/~demo.mine", ""); r.Code != 200 {
		t.Fatalf("owner removes: %d", r.Code)
	}
	if _, ok := addonGetRef(addonRef{Owner: admin.userID, ID: "demo.mine"}); !ok {
		t.Fatal("removing Alice's addon removed the admin's personal addon with the same id")
	}
	addonRemoveRef(admin.userID, addonRef{Owner: admin.userID, ID: "demo.mine"})
}

func TestAddonV2InstancesStoreWindows(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "a2-inst-admin", true)
	user := newTestUser(t, srv, "a2-inst-user", false)
	other := newTestUser(t, srv, "a2-inst-other", false)
	if r := addonPost(admin, "/api/addons/install", addonV2Zip(t, "demo.inst", "1.0.0", `,"store":{"quotaKb":1}`)); r.Code != 200 {
		t.Fatalf("install: %d %s", r.Code, r.Raw)
	}
	defer addonRemoveCore(admin.userID, "demo.inst")
	base := "/api/addons/demo.inst/instances"
	if r := addonReq(user, "GET", base, ""); r.Code != 404 {
		t.Fatalf("a disabled organisation addon showed instances to a user: %d", r.Code)
	}
	addonReq(admin, "POST", "/api/addons/demo.inst/enable", "")
	if r := addonReq(user, "POST", base, `{"name":"NOC","shared":true}`); r.Code != 403 {
		t.Fatalf("a user created a shared instance: %d", r.Code)
	}
	shared := addonReq(admin, "POST", base, `{"name":"NOC","shared":true}`)
	mine := addonReq(user, "POST", base, `{"name":"nova1"}`)
	if shared.Code != 200 || mine.Code != 200 {
		t.Fatalf("create: %s %s", shared.Raw, mine.Raw)
	}
	sid, mid := int(shared.Body["id"].(float64)), int(mine.Body["id"].(float64))
	var lst struct {
		Instances []addonInstance `json:"instances"`
	}
	other.jsonDo("GET", base, nil, &lst)
	if len(lst.Instances) != 1 || lst.Instances[0].ID != sid {
		t.Fatalf("another user sees: %+v", lst.Instances)
	}
	// Store: per user in a shared instance, the owner's in a personal one.
	put := func(c *testClient, iid int, key, val string) addonResp {
		return addonReq(c, "PUT", fmt.Sprintf("%s/%d/store/%s", base, iid, key), val)
	}
	if r := put(user, mid, "panels", `{"a":[1,2,3]}`); r.Code != 200 {
		t.Fatalf("put: %d %s", r.Code, r.Raw)
	}
	if r := put(other, mid, "panels", `1`); r.Code != 404 {
		t.Fatalf("another user wrote a personal instance: %d", r.Code)
	}
	if r := addonReq(admin, "GET", fmt.Sprintf("%s/%d/store", base, mid), ""); r.Code != 404 {
		t.Fatalf("an administrator read a user's private instance: %d", r.Code)
	}
	if r := put(user, mid, "wrm.secret", `1`); r.Code != 400 || r.Body["code"] != "E_STORE_KEY" {
		t.Fatalf("reserved key: %d %s", r.Code, r.Raw)
	}
	if r := put(user, mid, "bad", `{not json`); r.Code != 400 || r.Body["code"] != "E_STORE_VALUE" {
		t.Fatalf("not json: %d %s", r.Code, r.Raw)
	}
	if r := put(user, mid, "big", `"`+strings.Repeat("x", 1100)+`"`); r.Code != 413 || r.Body["code"] != "E_LIMIT_QUOTA" {
		t.Fatalf("quota: %d %s", r.Code, r.Raw)
	}
	put(user, sid, "view", `"mine"`)
	put(other, sid, "view", `"theirs"`)
	if r := addonReq(user, "GET", fmt.Sprintf("%s/%d/store/view", base, sid), ""); r.Body["value"] != "mine" {
		t.Fatalf("per-user values of a shared instance: %s", r.Raw)
	}
	if r := addonReq(user, "GET", fmt.Sprintf("%s/%d/store", base, mid), ""); !strings.Contains(r.Raw, `"panels"`) {
		t.Fatalf("store listing: %s", r.Raw)
	}
	// Windows are remembered per user and come back with the instance list.
	if r := addonReq(user, "PUT", fmt.Sprintf("%s/%d/window", base, mid), `{"open":true,"l":0.1,"t":0.1,"w":0.5,"h":0.4}`); r.Code != 200 {
		t.Fatalf("window: %d %s", r.Code, r.Raw)
	}
	lst.Instances = nil
	user.jsonDo("GET", base, nil, &lst)
	found := false
	for _, in := range lst.Instances {
		if in.ID == mid && strings.Contains(string(in.Window), `"open":true`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("window state not returned: %+v", lst.Instances)
	}
	// Export and import of one instance's values.
	exp := addonReq(user, "GET", fmt.Sprintf("%s/%d/export", base, mid), "")
	if exp.Code != 200 || exp.Body["format"] != "wrm-addon-config" {
		t.Fatalf("export: %d %s", exp.Code, exp.Raw)
	}
	put(user, mid, "panels", `0`)
	if r := addonReq(user, "POST", fmt.Sprintf("%s/%d/import", base, mid), exp.Raw); r.Code != 200 {
		t.Fatalf("import: %d %s", r.Code, r.Raw)
	}
	if r := addonReq(user, "GET", fmt.Sprintf("%s/%d/store/panels", base, mid), ""); !strings.Contains(r.Raw, `[1,2,3]`) {
		t.Fatalf("import did not restore: %s", r.Raw)
	}
	if r := addonReq(user, "POST", fmt.Sprintf("%s/%d/import", base, mid), `{"format":"wrm-addon-config","addon":"other.addon","values":{}}`); r.Code != 400 {
		t.Fatalf("import of another addon's file: %d", r.Code)
	}
	if r := addonReq(user, "DELETE", fmt.Sprintf("%s/%d", base, sid), ""); r.Code != 403 {
		t.Fatalf("a user removed a shared instance: %d", r.Code)
	}
	if r := addonReq(user, "DELETE", fmt.Sprintf("%s/%d", base, mid), ""); r.Code != 200 {
		t.Fatalf("delete: %d", r.Code)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM addon_store WHERE instance_id=?`, mid).Scan(&n)
	if n != 0 {
		t.Fatal("deleting an instance left its values")
	}
	// Removing the addon removes the shared instance with its values.
	addonRemoveCore(admin.userID, "demo.inst")
	db.QueryRow(`SELECT COUNT(*) FROM addon_instances WHERE id=?`, sid).Scan(&n)
	if n != 0 {
		t.Fatal("removing the addon left its instances")
	}
	// Shared values of a shared instance are written by administrators only.
	if r := addonPost(admin, "/api/addons/install", addonV2Zip(t, "demo.shared", "1.0.0", `,"store":{"shared":true}`)); r.Code != 200 {
		t.Fatalf("install shared: %s", r.Raw)
	}
	defer addonRemoveCore(admin.userID, "demo.shared")
	addonReq(admin, "POST", "/api/addons/demo.shared/enable", "")
	s2 := addonReq(admin, "POST", "/api/addons/demo.shared/instances", `{"name":"Wall","shared":true}`)
	id2 := int(s2.Body["id"].(float64))
	if r := addonReq(user, "PUT", fmt.Sprintf("/api/addons/demo.shared/instances/%d/store/url", id2), `"https://grafana.example.com"`); r.Code != 403 || r.Body["code"] != "E_STORE_READONLY" {
		t.Fatalf("a user wrote shared values: %d %s", r.Code, r.Raw)
	}
	if r := addonReq(admin, "PUT", fmt.Sprintf("/api/addons/demo.shared/instances/%d/store/url", id2), `"https://grafana.example.com"`); r.Code != 200 {
		t.Fatalf("admin writes shared values: %d %s", r.Code, r.Raw)
	}
	if r := addonReq(user, "GET", fmt.Sprintf("/api/addons/demo.shared/instances/%d/store/url", id2), ""); r.Body["value"] != "https://grafana.example.com" {
		t.Fatalf("a user does not read shared values: %s", r.Raw)
	}
}

func TestAddonV2CSPIntegrityApproval(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "a2-csp-admin", true)
	if r := addonPost(admin, "/api/addons/install", addonV2Zip(t, "demo.csp", "1.0.0", "")); r.Code != 200 {
		t.Fatalf("install: %s", r.Raw)
	}
	defer addonRemoveCore(admin.userID, "demo.csp")
	addonReq(admin, "POST", "/api/addons/demo.csp/enable", "")
	r := addonReq(admin, "GET", "/api/addons/demo.csp/asset/panel.html", "")
	csp := strings.Join(r.Header["Content-Security-Policy"], "")
	for _, want := range []string{"sandbox allow-scripts", "connect-src 'none'", "default-src 'none'", "form-action 'none'", "'sha256-"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("v2 CSP misses %q: %s", want, csp)
		}
	}
	if strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("v2 CSP allows script injection: %s", csp)
	}
	// A file changed on disk after the install is refused.
	os.WriteFile(filepath.Join(addonDir("demo.csp"), "panel.html"), []byte("<p>tampered</p>"), 0o644)
	if r := addonReq(admin, "GET", "/api/addons/demo.csp/asset/panel.html", ""); r.Code != 409 || r.Body["code"] != "E_INTEGRITY" {
		t.Fatalf("tampered file: %d %s", r.Code, r.Raw)
	}
	os.WriteFile(filepath.Join(addonDir("demo.csp"), "extra.js"), []byte("alert(1)"), 0o644)
	if r := addonReq(admin, "GET", "/api/addons/demo.csp/asset/extra.js", ""); r.Code != 409 {
		t.Fatalf("a file added after the install was served: %d", r.Code)
	}
	// Same permissions: the update stays enabled.
	if r := addonPost(admin, "/api/addons/install", addonV2Zip(t, "demo.csp", "1.1.0", "")); r.Code != 200 || r.Body["enabled"] != true || r.Body["needs_approval"] != false {
		t.Fatalf("same-permission update: %s", r.Raw)
	}
	if r := addonReq(admin, "GET", "/api/addons/demo.csp/asset/panel.html", ""); r.Code != 200 {
		t.Fatalf("reinstall did not restore the pinned files: %d", r.Code)
	}
	// A larger store is a new permission: the addon waits, disabled.
	r = addonPost(admin, "/api/addons/install", addonV2Zip(t, "demo.csp", "1.2.0", `,"store":{"quotaKb":512}`))
	if r.Code != 200 || r.Body["enabled"] != false || r.Body["needs_approval"] != true {
		t.Fatalf("growing update: %s", r.Raw)
	}
	rec, _ := addonGet("demo.csp")
	if !rec.NeedsApproval || len(rec.Added) != 1 || rec.Added[0] != "store:512" {
		t.Fatalf("pending permissions: %+v %v", rec.NeedsApproval, rec.Added)
	}
	addonReq(admin, "POST", "/api/addons/demo.csp/enable", "")
	if rec, _ := addonGet("demo.csp"); rec.NeedsApproval || !rec.Enabled {
		t.Fatal("approving did not clear the pending permissions")
	}
	// A smaller store needs no approval.
	if r := addonPost(admin, "/api/addons/install", addonV2Zip(t, "demo.csp", "1.3.0", `,"store":{"quotaKb":128}`)); r.Body["enabled"] != true {
		t.Fatalf("shrinking update: %s", r.Raw)
	}
	// Downgrades need an explicit confirmation.
	if r := addonPost(admin, "/api/addons/install", addonV2Zip(t, "demo.csp", "1.0.0", "")); r.Code != 409 || r.Body["code"] != "E_DOWNGRADE" {
		t.Fatalf("downgrade: %d %s", r.Code, r.Raw)
	}
	if r := addonPost(admin, "/api/addons/install?allow_downgrade=1", addonV2Zip(t, "demo.csp", "1.0.0", "")); r.Code != 200 {
		t.Fatalf("confirmed downgrade: %d %s", r.Code, r.Raw)
	}
	// v1 panels keep the v1 policy.
	v1 := addonZip(t, map[string]string{"addon.json": addonDemoManifest("demo.csp1"), "panel.html": "<p>v1</p>", "theme.css": ":root{}"})
	addonPost(admin, "/api/addons/install", v1)
	defer addonRemoveCore(admin.userID, "demo.csp1")
	addonReq(admin, "POST", "/api/addons/demo.csp1/enable", "")
	if r := addonReq(admin, "GET", "/api/addons/demo.csp1/asset/panel.html", ""); strings.Join(r.Header["Content-Security-Policy"], "") != "sandbox allow-scripts" {
		t.Fatalf("v1 CSP changed: %v", r.Header["Content-Security-Policy"])
	}
}

func TestAddonV2Helpers(t *testing.T) {
	for _, c := range [][2]string{{"1.9.3", "1.10.0"}, {"1.0", "1.0.1"}, {"v2.0.0", "2.1"}, {"1.0.0-beta", "1.0.0-rc"}} {
		if !addonVersionLess(c[0], c[1]) || addonVersionLess(c[1], c[0]) {
			t.Errorf("version order %s < %s", c[0], c[1])
		}
	}
	if addonVersionLess("1.0.0", "1.0.0") {
		t.Error("equal versions")
	}
	if got := addonPermsAdded([]string{"panel", "store:64", "server:a,b"}, []string{"panel", "store:128", "server:a"}); strings.Join(got, ",") != "server:a,b" {
		t.Errorf("perms added: %v", got)
	}
	if got := addonPermsAdded([]string{"store:64:shared"}, []string{"store:128"}); len(got) != 1 {
		t.Errorf("shared store needs approval: %v", got)
	}
	csp := addonPanelCSP([]byte(`<script src="a.js"></script><script>x()</script><SCRIPT type="module">y()</SCRIPT>`))
	if strings.Count(csp, "'sha256-") != 2 {
		t.Errorf("inline script hashes: %s", csp)
	}
	for _, bad := range []string{"red;}", "url(x)", "expression(1)", "@import x", "a<b", ""} {
		if addonSafeTokenValue(bad) {
			t.Errorf("token value %q accepted", bad)
		}
	}
	for _, ok := range []string{"#0b1220", "rgb(1 2 3 / .5)", "Inter, system-ui, sans-serif", "calc(4px + 1em)", "12px"} {
		if !addonSafeTokenValue(ok) {
			t.Errorf("token value %q refused", ok)
		}
	}
}
