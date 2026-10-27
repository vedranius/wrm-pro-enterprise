// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Addon platform v2, step 2: proxied HTTP with an origin allowlist, the
// administrator's approval, the private-network guard and secrets.

func addonHTTPManifest(id, version, origins, extra string) string {
	return fmt.Sprintf(`{"apiVersion":2,"id":%q,"name":"HTTP demo","version":%q,
		"capabilities":["ui.panel","http"],"ui":{"panel":"panel.html"},
		"http":{"origins":[%s],"paths":["/api/*","/redir","/big"],"methods":["GET","POST"],
		"secrets":["token","header:X-Api-Key:api_key","basic:login"]}%s}`, id, version, origins, extra)
}

func addonHTTPZip(t *testing.T, id, version, origins string) []byte {
	return addonZip(t, map[string]string{"addon.json": addonHTTPManifest(id, version, origins, ""), "panel.html": addonV2Panel})
}

// addonUpstream is a fake web site: it echoes what it received.
func addonUpstream(t *testing.T, tlsOn bool, hits *int32) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/echo", func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		if r.URL.Query().Get("slow") == "1" {
			time.Sleep(300 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "upstream=1")
		json.NewEncoder(w).Encode(map[string]string{"method": r.Method, "auth": r.Header.Get("Authorization"),
			"key": r.Header.Get("X-Api-Key"), "cookie": r.Header.Get("Cookie"), "accept": r.Header.Get("Accept"),
			"origin": r.Header.Get("Origin"), "ua": r.Header.Get("User-Agent"), "path": r.URL.Path})
	})
	mux.HandleFunc("/api/bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte{0, 1, 2, 255})
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(strings.Repeat("x", addonHTTPMaxResponse+100)))
	})
	mux.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Query().Get("to"), http.StatusFound)
	})
	var srv *httptest.Server
	if tlsOn {
		srv = httptest.NewTLSServer(mux)
		addonHTTPTestRoots = srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	} else {
		srv = httptest.NewServer(mux)
	}
	t.Cleanup(srv.Close)
	return srv
}

func addonProxy(c *testClient, key string, iid int, req map[string]interface{}) addonResp {
	b, _ := json.Marshal(req)
	return addonReq(c, "POST", fmt.Sprintf("/api/addons/%s/instances/%d/http", key, iid), string(b))
}

func addonEcho(t *testing.T, r addonResp) map[string]string {
	t.Helper()
	if r.Code != 200 || r.Body["ok"] != true {
		t.Fatalf("proxied request failed: %d %s", r.Code, r.Raw)
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(r.Body["body"].(string)), &out); err != nil {
		t.Fatalf("echo body: %v %s", err, r.Raw)
	}
	return out
}

func TestAddonHTTPManifestValidation(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "ah-val-admin", true)
	base := `"apiVersion":2,"name":"x","version":"1","ui":{"panel":"p.html"}`
	cases := []struct{ name, manifest string }{
		{"block without capability", `{` + base + `,"id":"h.bad1","capabilities":["ui.panel"],"http":{"origins":["https://example.com"]}}`},
		{"capability without block", `{` + base + `,"id":"h.bad2","capabilities":["ui.panel","http"]}`},
		{"origin with a path", `{` + base + `,"id":"h.bad3","capabilities":["ui.panel","http"],"http":{"origins":["https://example.com/api"]}}`},
		{"origin not normalised", `{` + base + `,"id":"h.bad4","capabilities":["ui.panel","http"],"http":{"origins":["https://Example.com:443"]}}`},
		{"origin with user", `{` + base + `,"id":"h.bad5","capabilities":["ui.panel","http"],"http":{"origins":["https://u:p@example.com"]}}`},
		{"other scheme", `{` + base + `,"id":"h.bad6","capabilities":["ui.panel","http"],"http":{"origins":["ftp://example.com"]}}`},
		{"bad path", `{` + base + `,"id":"h.bad7","capabilities":["ui.panel","http"],"http":{"origins":["https://example.com"],"paths":["/a/*/b"]}}`},
		{"bad method", `{` + base + `,"id":"h.bad8","capabilities":["ui.panel","http"],"http":{"origins":["https://example.com"],"methods":["CONNECT"]}}`},
		{"secret as cookie", `{` + base + `,"id":"h.bad9","capabilities":["ui.panel","http"],"http":{"origins":["https://example.com"],"secrets":["header:Cookie:c"]}}`},
		{"secret name", `{` + base + `,"id":"h.bad10","capabilities":["ui.panel","http"],"http":{"origins":["https://example.com"],"secrets":["Bad-Name"]}}`},
		{"no panel", `{"apiVersion":2,"name":"x","version":"1","id":"h.bad11","capabilities":["ui.theme","http"],"ui":{"themes":[{"id":"t","name":"T","mode":"dark","tokens":{"--bg":"#000"}}]},"http":{"origins":["https://example.com"]}}`},
		{"v1 manifest", `{"name":"x","version":"1","id":"h.bad12","capabilities":["ui.panel","http"],"ui":{"panel":"p.html"},"http":{"origins":["https://example.com"]}}`},
	}
	for _, tc := range cases {
		r := addonPost(admin, "/api/addons/install", addonZip(t, map[string]string{"addon.json": tc.manifest, "p.html": "<p>x</p>"}))
		if r.Code != 400 {
			t.Errorf("%s: want 400, got %d %s", tc.name, r.Code, r.Raw)
		}
	}
	m := &addonManifest{HTTP: &addonHTTP{Origins: []string{"https://example.com"}, Paths: []string{"/api/*"}, Methods: []string{"POST", "GET"}, Secrets: []string{"token"}}}
	got := strings.Join(addonHTTPPermissions(m.HTTP), " ")
	if got != "http:https://example.com http.methods:GET,POST http.paths:/api/* secret:token" {
		t.Fatalf("permission atoms: %s", got)
	}
	base2 := addonHTTPPermissions(m.HTTP)
	if a := addonPermsAdded([]string{"http:https://example.com", "http.methods:GET", "http.paths:/api/v1/*", "secret:token"}, base2); len(a) != 0 {
		t.Fatalf("a narrower update asked again: %v", a)
	}
	if a := addonPermsAdded([]string{"http:https://example.org", "http.methods:DELETE,GET", "http.paths:/*", "secret:other"}, base2); len(a) != 4 {
		t.Fatalf("a wider update did not ask: %v", a)
	}
}

func TestAddonHTTPIPGuard(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "224.0.0.1", "::1", "fe80::1", "fd00::1", "255.255.255.255", "::ffff:127.0.0.1", "64:ff9b::a9fe:a9fe", "2002:c0a8:101::1"} {
		if !addonIPBlocked(net.ParseIP(s)) {
			t.Errorf("%s is not blocked", s)
		}
	}
	for _, s := range []string{"93.184.216.34", "1.1.1.1", "2606:4700:4700::1111", "64:ff9b::101:101"} {
		if addonIPBlocked(net.ParseIP(s)) {
			t.Errorf("%s is blocked", s)
		}
	}
	for i := 0; i < addonHTTPConcurrent; i++ {
		if !addonHTTPAcquire(-1) {
			t.Fatalf("request %d refused", i)
		}
	}
	if addonHTTPAcquire(-1) {
		t.Fatal("more than the concurrency cap")
	}
	for i := 0; i < addonHTTPConcurrent; i++ {
		addonHTTPRelease(-1)
	}
	for i := addonHTTPConcurrent; i < addonHTTPPerMinute; i++ {
		addonHTTPAcquire(-1)
		addonHTTPRelease(-1)
	}
	if addonHTTPAcquire(-1) {
		t.Fatal("more than the rate limit")
	}
}

func TestAddonHTTPProxy(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "ah-proxy-admin", true)
	user := newTestUser(t, srv, "ah-proxy-user", false)
	setSetting("addon_http", "all")
	var hits int32
	up := addonUpstream(t, true, &hits)
	origin := up.URL
	if r := addonPost(admin, "/api/addons/install", addonHTTPZip(t, "demo.http", "1.0.0", fmt.Sprintf("%q", origin))); r.Code != 200 {
		t.Fatalf("install: %d %s", r.Code, r.Raw)
	}
	defer addonRemoveCore(admin.userID, "demo.http")
	// Enabling approves the origin, without the private network.
	if r := addonReq(admin, "POST", "/api/addons/demo.http/enable", ""); r.Code != 200 {
		t.Fatalf("enable: %d %s", r.Code, r.Raw)
	}
	rec, _ := addonGet("demo.http")
	if len(rec.HTTPGrants) != 1 || rec.HTTPGrants[0].Private || rec.HTTPWaiting {
		t.Fatalf("grants after enable: %+v waiting=%v", rec.HTTPGrants, rec.HTTPWaiting)
	}
	shared := addonReq(admin, "POST", "/api/addons/demo.http/instances", `{"name":"NOC","shared":true}`)
	sid := int(shared.Body["id"].(float64))
	echo := map[string]interface{}{"method": "GET", "url": origin + "/api/echo"}

	// 127.0.0.1 is the local machine: refused until the private network is allowed.
	if r := addonProxy(user, "demo.http", sid, echo); r.Code != 403 || r.Body["code"] != "E_HTTP_BLOCKED" {
		t.Fatalf("loopback without approval: %d %s", r.Code, r.Raw)
	}
	var refused int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='addon.http_refused' AND target='demo.http'`).Scan(&refused)
	if refused == 0 {
		t.Fatal("a refused address was not audited")
	}
	if r := addonReq(user, "POST", "/api/addons/demo.http/http", `{"private":[]}`); r.Code != 403 {
		t.Fatalf("a user approved network access: %d", r.Code)
	}
	if r := addonReq(admin, "POST", "/api/addons/demo.http/http", `{"private":["https://other.example.com"]}`); r.Code != 400 {
		t.Fatalf("a foreign origin was approved: %d %s", r.Code, r.Raw)
	}
	if r := addonReq(admin, "POST", "/api/addons/demo.http/http", fmt.Sprintf(`{"private":[%q]}`, origin)); r.Code != 200 {
		t.Fatalf("approve private: %d %s", r.Code, r.Raw)
	}

	// Headers: only Accept, Content-Type and Range pass; WRM sets its own User-Agent.
	got := addonEcho(t, addonProxy(user, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/api/echo",
		"headers": map[string]string{"Accept": "application/json", "Cookie": "wrm_session=stolen", "Authorization": "Bearer forged", "Origin": "https://evil.example.com"}}))
	if got["accept"] != "application/json" || got["cookie"] != "" || got["auth"] != "" || got["origin"] != "" || !strings.HasPrefix(got["ua"], "WRM-PRO-addon/") {
		t.Fatalf("headers reaching the site: %+v", got)
	}

	// Allowlist: origin, path and method.
	for _, tc := range []struct {
		req  map[string]interface{}
		code string
	}{
		{map[string]interface{}{"method": "GET", "url": "https://example.com/api/echo"}, "E_HTTP_ORIGIN"},
		{map[string]interface{}{"method": "GET", "url": origin + "/admin"}, "E_HTTP_PATH"},
		{map[string]interface{}{"method": "GET", "url": origin + "/api/../admin"}, "E_HTTP_PATH"},
		{map[string]interface{}{"method": "DELETE", "url": origin + "/api/echo"}, "E_HTTP_METHOD"},
		{map[string]interface{}{"method": "GET", "url": origin + "/api/echo", "secret": "nope"}, "E_HTTP_SECRET"},
		{map[string]interface{}{"method": "GET", "url": origin + "/api/echo", "secret": "token"}, "E_SECRET_MISSING"},
		{map[string]interface{}{"method": "GET", "url": strings.Replace(origin, "https://", "https://u:p@", 1) + "/api/echo"}, "E_HTTP_ORIGIN"},
		{map[string]interface{}{"method": "POST", "url": origin + "/api/echo", "body": strings.Repeat("a", addonHTTPMaxBody+1)}, "E_LIMIT_SIZE"},
	} {
		if r := addonProxy(user, "demo.http", sid, tc.req); r.Body["code"] != tc.code {
			t.Errorf("%v: want %s, got %d %s", tc.req, tc.code, r.Code, r.Raw)
		}
	}

	// Secrets: the instance value is set by an administrator, a user may set their own.
	sec := fmt.Sprintf("/api/addons/demo.http/instances/%d/secrets", sid)
	if r := addonReq(user, "PUT", sec+"/token", `{"value":"user-instance"}`); r.Code != 403 {
		t.Fatalf("a user set the shared value: %d", r.Code)
	}
	if r := addonReq(admin, "PUT", sec+"/token", `{"value":"org-value-123"}`); r.Code != 200 {
		t.Fatalf("admin sets: %d %s", r.Code, r.Raw)
	}
	if r := addonReq(admin, "PUT", sec+"/login", `{"value":"no-colon"}`); r.Code != 400 {
		t.Fatalf("basic value without user:password accepted: %d", r.Code)
	}
	if r := addonReq(admin, "PUT", sec+"/api_key", `{"value":"line1\nline2"}`); r.Code != 400 {
		t.Fatalf("a value with a line break accepted: %d", r.Code)
	}
	addonReq(admin, "PUT", sec+"/api_key", `{"value":"key-abc"}`)
	addonReq(admin, "PUT", sec+"/login", `{"value":"svc:pw-xyz"}`)
	got = addonEcho(t, addonProxy(user, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/api/echo", "secret": "token"}))
	if got["auth"] != "Bearer org-value-123" {
		t.Fatalf("instance value not used: %+v", got)
	}
	got = addonEcho(t, addonProxy(user, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/api/echo", "secret": "api_key"}))
	if got["key"] != "key-abc" || got["auth"] != "" {
		t.Fatalf("header secret: %+v", got)
	}
	got = addonEcho(t, addonProxy(user, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/api/echo", "secret": "login"}))
	if got["auth"] != "Basic c3ZjOnB3LXh5eg==" {
		t.Fatalf("basic secret: %+v", got)
	}
	if r := addonReq(user, "PUT", sec+"/token", `{"value":"user-own-456","scope":"own"}`); r.Code != 200 {
		t.Fatalf("own value: %d %s", r.Code, r.Raw)
	}
	got = addonEcho(t, addonProxy(user, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/api/echo", "secret": "token"}))
	if got["auth"] != "Bearer user-own-456" {
		t.Fatalf("own value not preferred: %+v", got)
	}
	got = addonEcho(t, addonProxy(admin, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/api/echo", "secret": "token"}))
	if got["auth"] != "Bearer org-value-123" {
		t.Fatalf("another user's own value leaked: %+v", got)
	}
	// Values never leave the server, are encrypted at rest and never audited.
	lr := addonReq(user, "GET", sec, "")
	if lr.Code != 200 || strings.Contains(lr.Raw, "org-value-123") || strings.Contains(lr.Raw, "user-own-456") || !strings.Contains(lr.Raw, `"own_set":true`) {
		t.Fatalf("secret list: %d %s", lr.Code, lr.Raw)
	}
	var plain, audited int
	db.QueryRow(`SELECT COUNT(*) FROM addon_secrets WHERE value LIKE '%org-value%' OR value LIKE '%user-own%'`).Scan(&plain)
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE details LIKE '%org-value%' OR details LIKE '%user-own%' OR details LIKE '%key-abc%'`).Scan(&audited)
	if plain != 0 || audited != 0 {
		t.Fatalf("secret stored in plain text (%d) or audited (%d)", plain, audited)
	}

	// Redirects are checked again: same origin keeps the secret, another origin is refused.
	got = addonEcho(t, addonProxy(user, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/redir?to=/api/echo", "secret": "token"}))
	if got["path"] != "/api/echo" || got["auth"] != "Bearer user-own-456" {
		t.Fatalf("same-origin redirect: %+v", got)
	}
	if r := addonProxy(user, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/redir?to=https://example.com/", "secret": "token"}); r.Body["code"] != "E_HTTP_ORIGIN" {
		t.Fatalf("redirect to another origin: %d %s", r.Code, r.Raw)
	}
	if r := addonProxy(user, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/redir?to=/admin"}); r.Body["code"] != "E_HTTP_PATH" {
		t.Fatalf("redirect to another path: %d %s", r.Code, r.Raw)
	}

	// Binary bodies come back as base64, oversized ones cut with E_LIMIT_SIZE.
	r := addonProxy(user, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/api/bin"})
	if r.Body["encoding"] != "base64" || r.Body["body"] != "AAEC/w==" {
		t.Fatalf("binary: %s", r.Raw)
	}
	r = addonProxy(user, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/big"})
	if r.Body["truncated"] != true || r.Body["code"] != "E_LIMIT_SIZE" || len(r.Body["body"].(string)) != addonHTTPMaxResponse {
		t.Fatalf("big response: truncated=%v code=%v", r.Body["truncated"], r.Body["code"])
	}
	if r.Body["headers"].(map[string]interface{})["Set-Cookie"] != nil {
		t.Fatal("Set-Cookie reached the panel")
	}

	// Identical GETs inside two seconds share one call to the site.
	atomic.StoreInt32(&hits, 0)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			addonProxy(user, "demo.http", sid, map[string]interface{}{"method": "GET", "url": origin + "/api/echo?slow=1"})
		}()
	}
	wg.Wait()
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("coalescing: %d upstream calls", n)
	}

	// The request log, own entries for a user.
	var lg struct {
		Requests []addonHTTPLogEntry `json:"requests"`
	}
	user.jsonDo("GET", fmt.Sprintf("/api/addons/demo.http/instances/%d/http/log", sid), nil, &lg)
	if len(lg.Requests) == 0 || lg.Requests[0].Origin != origin {
		t.Fatalf("request log: %+v", lg.Requests)
	}

	// The per-instance rate limit: the minute's budget used up answers E_LIMIT_RATE.
	for i := 0; i < addonHTTPPerMinute; i++ {
		if !addonHTTPAcquire(sid) {
			break
		}
		addonHTTPRelease(sid)
	}
	if r := addonProxy(user, "demo.http", sid, echo); r.Code != 429 || r.Body["code"] != "E_LIMIT_RATE" {
		t.Fatalf("rate limit: %d %s", r.Code, r.Raw)
	}
	addonHTTPState.Lock()
	delete(addonHTTPState.limits, sid)
	addonHTTPState.Unlock()

	// Policy, revocation and the kill switch.
	setSetting("addon_http", "admins")
	if r := addonProxy(user, "demo.http", sid, echo); r.Body["code"] != "E_HTTP_OFF" {
		t.Fatalf("policy: %s", r.Raw)
	}
	setSetting("addon_http", "all")
	addonReq(admin, "DELETE", "/api/addons/demo.http/http", "")
	if r := addonProxy(user, "demo.http", sid, echo); r.Body["code"] != "E_HTTP_UNAPPROVED" {
		t.Fatalf("after revoke: %s", r.Raw)
	}
	addonReq(admin, "POST", "/api/addons/demo.http/disable", "")
	if r := addonProxy(user, "demo.http", sid, echo); r.Body["code"] != "E_DISABLED" && r.Code != 404 {
		t.Fatalf("disabled addon: %d %s", r.Code, r.Raw)
	}
	// Removing the addon removes its approvals and secrets.
	addonRemoveCore(admin.userID, "demo.http")
	var left int
	db.QueryRow(`SELECT (SELECT COUNT(*) FROM addon_http_grants WHERE addon_id='demo.http') + (SELECT COUNT(*) FROM addon_secrets WHERE instance_id=?)`, sid).Scan(&left)
	if left != 0 {
		t.Fatalf("%d approvals or secrets left after removal", left)
	}
}

func TestAddonHTTPPlainAndPersonal(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "ah-pers-admin", true)
	alice := newTestUser(t, srv, "ah-pers-alice", false)
	setSetting("addon_personal", "all")
	setSetting("addon_http", "all")
	up := addonUpstream(t, false, nil)
	if r := addonPost(alice, "/api/addons/install", addonHTTPZip(t, "demo.phttp", "1.0.0", fmt.Sprintf("%q", up.URL))); r.Code != 200 {
		t.Fatalf("personal install: %d %s", r.Code, r.Raw)
	}
	if r := addonReq(alice, "POST", "/api/addons/~demo.phttp/enable", `{"private":[]}`); r.Code != 200 {
		t.Fatalf("owner enables: %d %s", r.Code, r.Raw)
	}
	var list struct {
		Addons []addonRecord `json:"addons"`
	}
	alice.jsonDo("GET", "/api/addons", nil, &list)
	if a := addonByID(list.Addons, "demo.phttp"); a == nil || !a.HTTPWaiting {
		t.Fatalf("a personal addon's network access is not waiting for an administrator: %+v", a)
	}
	in := addonReq(alice, "POST", "/api/addons/~demo.phttp/instances", `{"name":"mine"}`)
	iid := int(in.Body["id"].(float64))
	echo := map[string]interface{}{"method": "GET", "url": up.URL + "/api/echo", "secret": "token"}
	if r := addonProxy(alice, "~demo.phttp", iid, echo); r.Body["code"] != "E_HTTP_UNAPPROVED" {
		t.Fatalf("before approval: %s", r.Raw)
	}
	if r := addonReq(alice, "POST", "/api/addons/~demo.phttp/http", `{}`); r.Code != 403 {
		t.Fatalf("an owner approved their own network access: %d", r.Code)
	}
	// The owner sets the instance's value of their own instance.
	if r := addonReq(alice, "PUT", fmt.Sprintf("/api/addons/~demo.phttp/instances/%d/secrets/token", iid), `{"value":"alice-tok"}`); r.Code != 200 {
		t.Fatalf("owner secret: %d %s", r.Code, r.Raw)
	}
	// Approved without the private network: plain http is refused.
	users := fmt.Sprintf("/api/addons/users/%d/demo.phttp/http", alice.userID)
	if r := addonReq(admin, "POST", users, `{}`); r.Code != 200 {
		t.Fatalf("admin approves: %d %s", r.Code, r.Raw)
	}
	if r := addonProxy(alice, "~demo.phttp", iid, echo); r.Body["code"] != "E_HTTP_SCHEME" {
		t.Fatalf("plain http without private: %s", r.Raw)
	}
	if r := addonReq(admin, "POST", users, fmt.Sprintf(`{"private":[%q]}`, up.URL)); r.Code != 200 {
		t.Fatalf("admin approves private: %d %s", r.Code, r.Raw)
	}
	got := addonEcho(t, addonProxy(alice, "~demo.phttp", iid, echo))
	if got["auth"] != "Bearer alice-tok" {
		t.Fatalf("personal secret: %+v", got)
	}
	// The administrator cannot use or read another user's private instance.
	if r := addonProxy(admin, "~demo.phttp", iid, echo); r.Code != 404 {
		t.Fatalf("admin used alice's personal addon: %d", r.Code)
	}
	// An update that adds an origin waits for approval again.
	r := addonPost(alice, "/api/addons/install", addonZip(t, map[string]string{
		"addon.json": addonHTTPManifest("demo.phttp", "1.1.0", fmt.Sprintf("%q,%q", up.URL, "https://example.com"), ""), "panel.html": addonV2Panel}))
	if r.Code != 200 || r.Body["enabled"] != false {
		t.Fatalf("growing update: %d %s", r.Code, r.Raw)
	}
	addonReq(admin, "DELETE", fmt.Sprintf("/api/addons/users/%d/demo.phttp", alice.userID), "")
}
