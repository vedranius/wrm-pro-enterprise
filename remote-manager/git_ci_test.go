// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// postHook sends a webhook delivery the way a CI server does: no session, no CSRF header.
func postHook(t *testing.T, base, path string, body []byte, hdr map[string]string) (int, map[string]interface{}) {
	t.Helper()
	req, _ := http.NewRequest("POST", base+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func auditCount(userID int, action string) int {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE user_id=? AND action=?`, userID, action).Scan(&n)
	return n
}

func TestGitWebhooks(t *testing.T) {
	e := newDeployEnv(t, "git-hooks")
	writeFile(t, e.dir+"/run.py", runV19)
	writeFile(t, e.dir+"/lib/util.py", utilV1)
	in := e.check(t)
	srv := e.u.srv
	var st gitStateResp
	e.u.jsonDo("GET", "/api/git", nil, &st)
	src := st.Sources[0]
	var hook struct {
		HookID string `json:"hook_id"`
		Path   string `json:"path"`
		Secret string `json:"secret"`
	}
	if code := e.u.jsonDo("POST", fmt.Sprintf("/api/git/sources/%d/hook", src.ID), nil, &hook); code != 200 || hook.Secret == "" || hook.Path != "/api/hooks/git/"+hook.HookID {
		t.Fatalf("enable hook: %d %+v", code, hook)
	}
	var raw string
	db.QueryRow(`SELECT hook_secret FROM git_sources WHERE id=?`, src.ID).Scan(&raw)
	if strings.Contains(raw, hook.Secret) {
		t.Fatal("hook secret stored in plain text")
	}
	e.u.jsonDo("GET", "/api/git", nil, &st)
	if st.Sources[0].HookID != hook.HookID {
		t.Fatalf("state: %+v", st.Sources[0])
	}
	// a delivery triggers a check of the service: wait until it ran and compared the installation
	checkedAfter := func(prev time.Time) time.Time {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			gitHookState.Lock()
			at, busy := gitHookState.lastCheck[e.u.userID], gitHookState.running[e.u.userID]
			gitHookState.Unlock()
			if at.After(prev) && !busy {
				if l := loadGitInstalls(e.u.userID, false, "i.id=?", in.ID); len(l) != 1 || l[0].CheckedAt == "" || l[0].State != "update" {
					t.Fatalf("installation after the hook check: %+v", l)
				}
				return at
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("the webhook did not check the installation")
		return prev
	}
	db.Exec(`UPDATE git_installs SET checked_at='' WHERE id=?`, in.ID)
	push := []byte(`{"object_kind":"push","ref":"refs/heads/main","project":{"path_with_namespace":"demo/demo-api"}}`)
	gl := map[string]string{"X-Gitlab-Token": "wrong", "X-Gitlab-Event": "Push Hook", "X-Gitlab-Event-UUID": "uuid-1"}
	if code, _ := postHook(t, srv.URL, hook.Path, push, gl); code != 401 || auditCount(e.u.userID, "git.webhook_rejected") != 1 {
		t.Fatalf("bad token: %d", code)
	}
	gl["X-Gitlab-Token"] = hook.Secret
	code, out := postHook(t, srv.URL, hook.Path, push, gl)
	if code != 202 || fmt.Sprint(out["services"]) != "[demo-api]" {
		t.Fatalf("gitlab push: %d %v", code, out)
	}
	last := checkedAfter(time.Time{})
	if code, _ := postHook(t, srv.URL, hook.Path, push, gl); code != 409 {
		t.Fatalf("replay: %d", code)
	}
	gl["X-Gitlab-Event-UUID"] = "uuid-2"
	if code, out := postHook(t, srv.URL, hook.Path, []byte(`{"object_kind":"push","project":{"path_with_namespace":"demo/other"}}`), gl); code != 202 || out["queued"] != false {
		t.Fatalf("other project: %d %v", code, out)
	}

	// GitHub: HMAC of the body
	ghPush := []byte(`{"ref":"refs/tags/v1.10","repository":{"full_name":"demo/demo-api"}}`)
	sig := "sha256=" + hmacHex(hook.Secret, ghPush)
	if code, _ := postHook(t, srv.URL, hook.Path, ghPush, map[string]string{"X-Hub-Signature-256": "sha256=00", "X-GitHub-Event": "push", "X-GitHub-Delivery": "d1"}); code != 401 {
		t.Fatalf("bad signature: %d", code)
	}
	if code, _ := postHook(t, srv.URL, hook.Path, []byte(`{}`), map[string]string{"X-Hub-Signature-256": "sha256=" + hmacHex(hook.Secret, []byte(`{}`)), "X-GitHub-Event": "ping", "X-GitHub-Delivery": "d0"}); code != 200 {
		t.Fatalf("ping: %d", code)
	}
	if code, out := postHook(t, srv.URL, hook.Path, ghPush, map[string]string{"X-Hub-Signature-256": sig, "X-GitHub-Event": "push", "X-GitHub-Delivery": "d1"}); code != 202 || out["queued"] != true {
		t.Fatalf("github push: %d %v", code, out)
	}
	last = checkedAfter(last)
	rel := []byte(`{"action":"edited","release":{"tag_name":"v1.10"},"repository":{"full_name":"demo/demo-api"}}`)
	if code, out := postHook(t, srv.URL, hook.Path, rel, map[string]string{"X-Hub-Signature-256": "sha256=" + hmacHex(hook.Secret, rel), "X-GitHub-Event": "release", "X-GitHub-Delivery": "d2"}); code != 202 || out["ignored"] != true {
		t.Fatalf("edited release: %d %v", code, out)
	}

	// generic: shared token, or an HMAC with a timestamp (replay protection)
	if code, out := postHook(t, srv.URL, hook.Path, nil, map[string]string{"X-WRM-Token": hook.Secret}); code != 202 || fmt.Sprint(out["services"]) != "[demo-api]" {
		t.Fatalf("generic token: %d %v", code, out)
	}
	checkedAfter(last)
	body := []byte(`{"services":["demo-api"]}`)
	old := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	if code, _ := postHook(t, srv.URL, hook.Path, body, map[string]string{"X-WRM-Timestamp": old, "X-WRM-Signature": "sha256=" + hmacHex(hook.Secret, []byte(old+"."), body)}); code != 401 {
		t.Fatalf("old timestamp: %d", code)
	}
	now := strconv.FormatInt(time.Now().Unix(), 10)
	signed := map[string]string{"X-WRM-Timestamp": now, "X-WRM-Signature": "sha256=" + hmacHex(hook.Secret, []byte(now+"."), body)}
	if code, _ := postHook(t, srv.URL, hook.Path, body, signed); code != 202 {
		t.Fatalf("generic HMAC: %d", code)
	}
	if code, _ := postHook(t, srv.URL, hook.Path, body, signed); code != 409 {
		t.Fatalf("generic replay: %d", code)
	}
	if code, _ := postHook(t, srv.URL, hook.Path, body, nil); code != 401 {
		t.Fatalf("unsigned: %d", code)
	}
	if auditCount(e.u.userID, "git.webhook") < 4 {
		t.Fatalf("audit: %d", auditCount(e.u.userID, "git.webhook"))
	}
	// rate limit per hook
	n := 0
	for i := 0; i < gitHookRatePerMin+5; i++ {
		if hookAllowed("hook:rate-test", time.Now()) {
			n++
		}
	}
	if n != gitHookRatePerMin {
		t.Fatalf("rate limit: %d", n)
	}
	// turned off: unknown
	e.u.jsonDo("DELETE", fmt.Sprintf("/api/git/sources/%d/hook", src.ID), nil, nil)
	if code, _ := postHook(t, srv.URL, hook.Path, nil, map[string]string{"X-WRM-Token": hook.Secret}); code != 404 {
		t.Fatalf("disabled hook: %d", code)
	}
	for gitHookStateBusy(e.u.userID) {
		time.Sleep(20 * time.Millisecond)
	}
}

func gitHookStateBusy(userID int) bool {
	gitHookState.Lock()
	defer gitHookState.Unlock()
	return gitHookState.running[userID]
}

func TestGitArtifactFeed(t *testing.T) {
	srv := newTestServer(t)
	g := newFakeGit(t, "gitlab")
	u := setupGitUser(t, srv, "git-feed", g)
	refreshDemo(t, u)
	resp := u.do("POST", "/api/git/bundles/export", strings.NewReader(`{"apps":["demo-api"]}`), "application/json")
	bundle, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || len(bundle) == 0 {
		t.Fatalf("export: %d", resp.StatusCode)
	}
	var mu sync.Mutex
	serve, hits := bundle, 0
	art := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		hits++
		if r.Header.Get("PRIVATE-TOKEN") != "art-token" || r.URL.Query().Get("job") != "bundle" {
			w.WriteHeader(401)
			return
		}
		w.Write(serve)
	}))
	defer art.Close()
	feedURL := art.URL + "/api/v4/projects/demo%2Fdemo-api/jobs/artifacts/main/raw/bundle.tar.gz?job=bundle"
	var st struct {
		Feeds   []gitFeed   `json:"feeds"`
		Sources []gitSource `json:"sources"`
	}
	if code := u.jsonDo("POST", "/api/git/feeds", map[string]interface{}{"name": "CI bundle", "url": feedURL, "header_name": "PRIVATE-TOKEN", "header_value": "art-token"}, &st); code != 200 || len(st.Feeds) != 1 || !st.Feeds[0].HasHeader {
		t.Fatalf("add feed: %d %+v", code, st.Feeds)
	}
	b, _ := json.Marshal(st)
	if strings.Contains(string(b), "art-token") {
		t.Fatal("header value sent to the browser")
	}
	id := st.Feeds[0].ID
	var res struct {
		Status string `json:"status"`
	}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/git/feeds/%d/fetch", id), nil, &res); code != 200 || res.Status != "imported" {
		t.Fatalf("fetch: %d %+v", code, res)
	}
	u.jsonDo("GET", "/api/git", nil, &st)
	bundles := 0
	for _, s := range st.Sources {
		if s.Kind == "bundle" && s.ID == st.Feeds[0].SourceID {
			bundles++
		}
	}
	if bundles != 1 || st.Feeds[0].LastOKAt == "" {
		t.Fatalf("bundle source: %+v %+v", st.Sources, st.Feeds)
	}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/git/feeds/%d/fetch", id), nil, &res); code != 200 || res.Status != "unchanged" {
		t.Fatalf("refetch: %d %+v", code, res)
	}
	mu.Lock()
	serve = []byte("not a bundle")
	mu.Unlock()
	if code := u.jsonDo("POST", fmt.Sprintf("/api/git/feeds/%d/fetch", id), nil, nil); code != 502 {
		t.Fatalf("bad bundle: %d", code)
	}
	u.jsonDo("GET", "/api/git", nil, &st)
	if st.Feeds[0].LastError == "" || auditCount(u.userID, "git.bundle_fetch_failed") != 1 || auditCount(u.userID, "git.bundle_fetched") != 1 {
		t.Fatalf("error recorded: %+v", st.Feeds[0])
	}
	// on a schedule
	mu.Lock()
	serve = bundle
	before := hits
	mu.Unlock()
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/git/feeds/%d", id), map[string]interface{}{"name": "CI bundle", "url": feedURL, "header_name": "PRIVATE-TOKEN", "interval_minutes": 15}, nil); code != 200 {
		t.Fatalf("interval: %d", code)
	}
	gitFeedsTick(time.Now().Add(time.Hour))
	deadline := time.Now().Add(10 * time.Second)
	for {
		mu.Lock()
		h := hits
		mu.Unlock()
		gitFeedBusy.Lock()
		busy := gitFeedBusy.m[id]
		gitFeedBusy.Unlock()
		if h > before && !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scheduled fetch did not run")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if code := u.jsonDo("POST", "/api/git/feeds", map[string]interface{}{"url": "ftp://example.com/x"}, nil); code != 400 {
		t.Fatalf("bad url: %d", code)
	}
}

// fakeJenkins: buildWithParameters → queue item → build (running, then the result).
type fakeJenkins struct {
	srv    *httptest.Server
	mu     sync.Mutex
	params url.Values
	result string
	polls  int
}

func newFakeJenkins(t *testing.T, result string) *fakeJenkins {
	j := &fakeJenkins{result: result}
	j.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		j.mu.Lock()
		defer j.mu.Unlock()
		if u, p, ok := r.BasicAuth(); !ok || u != "ci-bot" || p != "jenkins-api-token" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/job/deploy-demo/buildWithParameters":
			j.params = r.URL.Query()
			w.Header().Set("Location", "http://jenkins.internal.example.com/queue/item/7/") // its own root URL
			w.WriteHeader(201)
		case r.URL.Path == "/queue/item/7/api/json":
			j.polls++
			if j.polls < 2 {
				w.Write([]byte(`{"why":"waiting"}`))
				return
			}
			w.Write([]byte(`{"executable":{"number":12,"url":"http://jenkins.internal.example.com/job/deploy-demo/12/"}}`))
		case r.URL.Path == "/job/deploy-demo/12/api/json":
			j.polls++
			if j.polls < 4 {
				w.Write([]byte(`{"building":true,"result":null}`))
				return
			}
			w.Write([]byte(`{"building":false,"result":"` + j.result + `"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(j.srv.Close)
	return j
}

func TestGitCIPipeline(t *testing.T) {
	oldEvery := gitCIPollEvery
	gitCIPollEvery = 5 * time.Millisecond
	defer func() { gitCIPollEvery = oldEvery }()
	e := newDeployEnv(t, "git-ci")
	writeFile(t, e.dir+"/run.py", runV19)
	writeFile(t, e.dir+"/lib/util.py", utilV1)
	in := e.check(t)
	j := newFakeJenkins(t, "SUCCESS")
	pl := map[string]interface{}{"kind": "jenkins", "url": j.srv.URL + "/job/deploy-demo", "username": "ci-bot", "token": "jenkins-api-token"}
	if code := e.u.jsonDo("PUT", "/api/git/pipelines/demo-api", pl, nil); code != 200 {
		t.Fatalf("pipeline: %d", code)
	}
	var raw string
	db.QueryRow(`SELECT token FROM git_pipelines WHERE app='demo-api' AND user_id=?`, e.u.userID).Scan(&raw)
	if raw == "" || strings.Contains(raw, "jenkins-api-token") {
		t.Fatal("pipeline token not encrypted")
	}
	var plan struct{ Items []gitPlanItem }
	e.u.jsonDo("POST", "/api/git/plan", map[string]interface{}{"kind": "update", "install_ids": []int{in.ID}}, &plan)
	if len(plan.Items) != 1 || plan.Items[0].Pipeline == nil || plan.Items[0].Pipeline.Kind != "jenkins" {
		t.Fatalf("plan: %+v", plan.Items)
	}
	v, code := e.run(t, map[string]interface{}{"kind": "update", "items": []map[string]interface{}{{"install_id": in.ID}}, "restart": map[string]string{"mode": "now"}})
	if code != 200 || v.State != "done" {
		t.Fatalf("run: %d %+v", code, v)
	}
	it := v.Items[0]
	if it.Via != "ci" || it.CIStatus != "success" || it.CIURL != j.srv.URL+"/job/deploy-demo/12/" || it.ToVersion != "v1.10" || len(it.Written) != 0 || len(it.Health) != 0 {
		t.Fatalf("item: %+v", it)
	}
	j.mu.Lock()
	if j.params.Get("server") != "127.0.0.1" || j.params.Get("install_path") != e.dir || j.params.Get("version") != "v1.10" {
		t.Fatalf("params: %v", j.params)
	}
	j.mu.Unlock()
	if readFile(t, e.dir+"/run.py") != runV19 {
		t.Fatal("files were written although CI deploys the service")
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='git.update' AND user_id=? AND details LIKE '%"via":"ci"%'`, e.u.userID).Scan(&n)
	if n != 1 {
		t.Fatalf("audit: %d", n)
	}
	// a failed job fails the run
	j.mu.Lock()
	j.result, j.polls = "FAILURE", 0
	j.mu.Unlock()
	v, _ = e.run(t, map[string]interface{}{"kind": "update", "items": []map[string]interface{}{{"install_id": in.ID}}})
	if v.State != "failed" || v.Items[0].CIStatus != "failure" {
		t.Fatalf("failed job: %+v", v.Items[0])
	}

	// GitLab pipeline trigger; the status is read with the source's read token
	var trig url.Values
	gpolls := 0
	e.g.mu.Lock()
	e.g.extra = func(w http.ResponseWriter, r *http.Request) bool {
		p := r.URL.EscapedPath()
		switch {
		case r.Method == "POST" && p == "/api/v4/projects/demo%2Fdemo-api/trigger/pipeline":
			r.ParseForm()
			trig = r.PostForm
			w.WriteHeader(201)
			w.Write([]byte(`{"id":55,"web_url":"https://git.example.com/demo/demo-api/-/pipelines/55","status":"pending"}`))
			return true
		case p == "/api/v4/projects/demo%2Fdemo-api/pipelines/55":
			if r.Header.Get("PRIVATE-TOKEN") != e.g.token {
				w.WriteHeader(401)
				return true
			}
			gpolls++
			status := "running"
			if gpolls > 2 {
				status = "success"
			}
			w.Write([]byte(`{"status":"` + status + `","web_url":"https://git.example.com/demo/demo-api/-/pipelines/55"}`))
			return true
		}
		return false
	}
	e.g.mu.Unlock()
	if code := e.u.jsonDo("PUT", "/api/git/pipelines/demo-api", map[string]interface{}{"kind": "gitlab", "token": ""}, nil); code != 400 {
		t.Fatalf("missing trigger token: %d", code)
	}
	if code := e.u.jsonDo("PUT", "/api/git/pipelines/demo-api", map[string]interface{}{"kind": "gitlab", "token": "trigger-tok", "ref": "deploy"}, nil); code != 200 {
		t.Fatalf("gitlab pipeline: %d", code)
	}
	v, _ = e.run(t, map[string]interface{}{"kind": "update", "items": []map[string]interface{}{{"install_id": in.ID}}})
	if v.State != "done" || v.Items[0].CIStatus != "success" || !strings.HasSuffix(v.Items[0].CIURL, "/pipelines/55") {
		t.Fatalf("gitlab run: %+v", v.Items[0])
	}
	if trig.Get("token") != "trigger-tok" || trig.Get("ref") != "deploy" || trig.Get("variables[version]") != "v1.10" || trig.Get("variables[install_path]") != e.dir {
		t.Fatalf("trigger: %v", trig)
	}
	// back to files over SSH; pipelines need the update policy
	e.u.jsonDo("DELETE", "/api/git/pipelines/demo-api", nil, nil)
	if _, ok := loadGitPipeline(e.u.userID, "demo-api"); ok {
		t.Fatal("pipeline not deleted")
	}
	setSetting("git_update", "off")
	defer setSetting("git_update", "admins")
	if code := e.u.jsonDo("PUT", "/api/git/pipelines/demo-api", pl, nil); code != 403 {
		t.Fatalf("policy: %d", code)
	}
}
