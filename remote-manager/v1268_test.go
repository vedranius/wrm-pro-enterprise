// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// v12.6.8: the panel layout is stored per user on the server (/api/me/ui/{key}).
func TestUserUIStore(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	a := newTestUser(t, srv, "ui-a", false)
	b := newTestUser(t, srv, "ui-b", false)

	get := func(c *testClient, key string) (int, map[string]interface{}) {
		resp := c.do("GET", "/api/me/ui/"+key, nil, "")
		defer resp.Body.Close()
		var out map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	put := func(c *testClient, key, body string) int {
		resp := c.do("PUT", "/api/me/ui/"+key, strings.NewReader(body), "application/json")
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}

	if code, out := get(a, "layout"); code != 200 || out["value"] != nil {
		t.Fatalf("empty layout: %d %v", code, out)
	}
	layout := `{"value":{"v":1,"locked":true,"zones":{"left":{"groups":[{"p":["conn","sess"],"a":"sess","s":1}]}}}}`
	if code := put(a, "layout", layout); code != 200 {
		t.Fatalf("put: %d", code)
	}
	code, out := get(a, "layout")
	v, _ := out["value"].(map[string]interface{})
	if code != 200 || v == nil || v["locked"] != true || out["updated_at"] == "" {
		t.Fatalf("stored layout: %d %v", code, out)
	}
	// per user: another user does not see it
	if _, out := get(b, "layout"); out["value"] != nil {
		t.Fatalf("user b sees user a's layout: %v", out)
	}
	// overwrite, then reset
	if code := put(a, "layout", `{"value":{"v":1,"locked":false}}`); code != 200 {
		t.Fatalf("overwrite: %d", code)
	}
	if _, out := get(a, "layout"); out["value"].(map[string]interface{})["locked"] != false {
		t.Fatalf("not overwritten: %v", out)
	}
	resp := a.do("DELETE", "/api/me/ui/layout", nil, "")
	resp.Body.Close()
	if _, out := get(a, "layout"); out["value"] != nil {
		t.Fatalf("not reset: %v", out)
	}
	// only objects, only known keys, bounded size
	for _, bad := range []string{`{"value":[1,2]}`, `{"value":"x"}`, `{"value":null}`, `not json`, `{"value":{"x":"` + strings.Repeat("a", userUIMaxBytes) + `"}}`} {
		if code := put(a, "layout", bad); code != 400 {
			t.Fatalf("bad body accepted (%d): %.40s", code, bad)
		}
	}
	if code, _ := get(a, "secrets"); code != 404 {
		t.Fatalf("unknown key: %d", code)
	}
	if code := put(a, "workboard", `{"value":{"tiles":[]}}`); code != 200 {
		t.Fatalf("workboard key: %d", code)
	}
	// signed out
	r, err := http.Get(srv.URL + "/api/me/ui/layout")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatalf("anonymous: %d", r.StatusCode)
	}
	// deleting the user removes the stored interface state
	deleteUserCompletely(a.userID)
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM user_ui WHERE user_id=?`, a.userID).Scan(&n)
	if n != 0 {
		t.Fatalf("user_ui rows left after delete: %d", n)
	}
}

// The page loads the layout script and every panel it manages exists in the markup.
func TestLayoutScriptWired(t *testing.T) {
	page, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	js, err := os.ReadFile("static/layout.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<script src="/static/layout.js"></script>`, `id="dz-left"`, `id="dz-right"`, `id="dz-bottom-g"`, `id="btn-panels"`,
		`id="pnl-conn"`, `id="pnl-sess"`, `id="pnl-snip"`, `id="rp-ai"`, `id="clip-panel"`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	for _, id := range []string{"'pnl-conn'", "'pnl-sess'", "'pnl-snip'", "'rp-ai'", "'clip-panel'", "/api/me/ui/layout"} {
		if !strings.Contains(string(js), id) {
			t.Errorf("layout.js lacks %s", id)
		}
	}
}
