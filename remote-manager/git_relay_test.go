// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestV1244GitLabClientCompanionSnapshotIsolationAndFailedRefresh(t *testing.T) {
	content := "print('demo release')\n"
	hash := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(content), content)))
	blob := hex.EncodeToString(hash[:])
	commit := strings.Repeat("a", 40)
	var calls atomic.Int64
	var refused atomic.Bool
	git := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("PRIVATE-TOKEN") != "client-private-token" {
			t.Errorf("GitLab token left the wrong transport: %q", r.Header.Get("PRIVATE-TOKEN"))
			w.WriteHeader(401)
			return
		}
		if refused.Load() {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]string{"message": "expired token"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Next-Page", "")
		p := strings.TrimPrefix(r.URL.Path, "/api/v4/projects/demo/demo-api")
		switch {
		case p == "":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main", "path_with_namespace": "demo/demo-api"})
		case p == "/repository/branches":
			json.NewEncoder(w).Encode([]map[string]string{{"name": "main"}})
		case p == "/repository/tags":
			json.NewEncoder(w).Encode([]map[string]interface{}{{"name": "v1.0", "commit": map[string]string{"id": commit, "committed_date": "2026-10-14T12:00:00Z"}}})
		case p == "/repository/commits":
			json.NewEncoder(w).Encode([]map[string]string{{"id": commit, "committed_date": "2026-10-14T12:00:00Z"}})
		case strings.HasPrefix(p, "/repository/commits/"):
			json.NewEncoder(w).Encode(map[string]string{"id": commit, "committed_date": "2026-10-14T12:00:00Z"})
		case p == "/repository/tree":
			json.NewEncoder(w).Encode([]map[string]string{{"type": "blob", "path": "run.py", "id": blob}})
		case p == "/repository/blobs/"+blob+"/raw":
			fmt.Fprint(w, content)
		default:
			t.Errorf("unexpected client API %s", r.URL)
			w.WriteHeader(404)
		}
	}))
	defer git.Close()
	srv := newTestServer(t)
	u := newTestUser(t, srv, "relay-owner", false)
	member := newTestUser(t, srv, "relay-member", false)
	foreign := newTestUser(t, srv, "relay-foreign", false)
	result, err := db.Exec(`INSERT INTO git_sources(user_id,kind,name,url,token) VALUES(?,?,?,?,?)`, u.userID, "gitlab", "demo GitLab", git.URL, encryptValue("server-token-must-not-be-exported"))
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := result.LastInsertId()
	id := int(sid)
	cat := &gitCatalog{Apps: []gitCatalogApp{{Name: "demo-api", Project: "demo/demo-api", Ref: "main", Include: []string{"**"}}}, FallbackBranches: []string{"main"}}
	saveGitWorkspace(u.userID, cat, &gitSettings{SourceID: id, Mode: "live", RefMode: "catalog"})
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/git/client/%d", id), map[string]interface{}{"enabled": true, "members": []string{usernameOf(member.userID)}}, nil); code != 200 {
		t.Fatal(code)
	}
	src, ok := loadGitSource(u.userID, id)
	if !ok || src.HasToken || src.token != "" || src.Relay == nil {
		t.Fatal("server token exposed to relay source")
	}
	if len(loadGitSources(foreign.userID)) != 0 || len(loadGitSources(member.userID)) != 1 {
		t.Fatal("sharing not isolated")
	}
	job, key, err := newGitRelayJob(src, u.userID)
	if err != nil {
		t.Fatal(err)
	}
	defer job.cancel()
	if calls.Load() != 0 {
		t.Fatal("WRM accessed GitLab before client connected")
	}
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	tokenFile := filepath.Join(dir, "token")
	os.WriteFile(keyFile, []byte(key), 0600)
	os.WriteFile(tokenFile, []byte("client-private-token"), 0600)
	var output bytes.Buffer
	worker := srv.URL + "/gitlab-relay?job=" + url.QueryEscape(job.ID)
	if exit := runGitLabRelayClient(worker, git.URL, keyFile, tokenFile, "", &output); exit != 0 {
		t.Fatalf("companion: %d %s", exit, output.String())
	}
	meta := loadGitRelayMeta(id)
	if !meta.Complete || meta.Published != job.Generation {
		t.Fatalf("snapshot metadata %+v", meta)
	}
	oldCalls := calls.Load()
	src, _ = loadGitSource(u.userID, id)
	provider, err := newGitProvider(src)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := provider.rawTree("demo/demo-api", commit)
	if err != nil || tree["run.py"] != blob {
		t.Fatal("cached tree", tree, err)
	}
	raw, err := provider.blob("demo/demo-api", blob)
	if err != nil || string(raw) != content || calls.Load() != oldCalls {
		t.Fatal("cache depends on connected client", err)
	}
	if _, err := provider.blob("demo/demo-api", strings.Repeat("b", 40)); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatal("uncached data silently fetched", err)
	}
	var stored string
	db.QueryRow(`SELECT response FROM git_relay_cache WHERE source_id=? LIMIT 1`, id).Scan(&stored)
	if strings.Contains(stored, "client-private-token") || strings.Contains(stored, "server-token") {
		t.Fatal("token cached")
	}
	refused.Store(true)
	failed, failedKey, err := newGitRelayJob(src, u.userID)
	if err != nil {
		t.Fatal(err)
	}
	defer failed.cancel()
	os.WriteFile(keyFile, []byte(failedKey), 0600)
	output.Reset()
	if exit := runGitLabRelayClient(srv.URL+"/gitlab-relay?job="+failed.ID, git.URL, keyFile, tokenFile, "", &output); exit == 0 {
		t.Fatal("expired token refresh accepted")
	}
	if loadGitRelayMeta(id).Published != meta.Published {
		t.Fatal("failed refresh discarded good snapshot")
	}
	// A later successful publisher wins, regardless of completion order.
	newest := &gitRelayJob{Source: src, UserID: u.userID, Generation: meta.Generation + 3, ctx: context.Background(), Stage: map[string]gitRelayResponse{"/_wrm/projects": {Status: 200, Body: []byte(`["demo/demo-api"]`)}}}
	older := &gitRelayJob{Source: src, UserID: u.userID, Generation: meta.Generation + 2, ctx: context.Background(), Stage: map[string]gitRelayResponse{"/_wrm/projects": {Status: 200, Body: []byte(`["demo/demo-api"]`)}}}
	if err := newest.publish(); err != nil {
		t.Fatal(err)
	}
	if err := older.publish(); err == nil {
		t.Fatal("older job replaced newer snapshot")
	}
	if loadGitRelayMeta(id).Published != newest.Generation {
		t.Fatal("generation regressed")
	}
	if code := member.jsonDo("POST", fmt.Sprintf("/api/git/sources/%d/hook", id), nil, nil); code != 404 {
		t.Fatal("shared member changed owner webhook", code)
	}
	if code := member.jsonDo("PUT", fmt.Sprintf("/api/git/sources/%d", id), map[string]interface{}{"kind": "gitlab", "name": "foreign mutation", "url": git.URL}, nil); code != 404 {
		t.Fatal("active shared member edited owner source", code)
	}
	deleteGitUserData(member.userID)
	if _, ok := loadGitSource(u.userID, id); !ok {
		t.Fatal("member cleanup deleted owner source")
	}
	storeBlob(id, blob, []byte(content))
	target := gitTarget{SourceID: id, Files: map[string]gitTargetFile{"run.py": {Blob: blob}}}
	view := openRepoView(member.userID, target)
	memberSource, _ := loadGitSource(member.userID, id)
	memberProvider, _ := newGitProvider(memberSource)
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/git/client/%d", id), map[string]interface{}{"enabled": true, "members": []string{}}, nil); code != 200 {
		t.Fatal(code)
	}
	if _, err := view.content("run.py"); err == nil {
		t.Fatal("removed member read cached content")
	}
	var bundle bytes.Buffer
	if _, err := writeBundle(&bundle, member.userID, cat, gitSettings{}, []gitTarget{target}); err == nil {
		t.Fatal("removed member exported cached content")
	}
	if _, err := memberProvider.projects(nil); err == nil {
		t.Fatal("removed member retained cached access")
	}
	if code := member.jsonDo("PUT", fmt.Sprintf("/api/git/sources/%d", id), map[string]interface{}{"kind": "gitlab", "name": "foreign mutation", "url": git.URL}, nil); code != 404 {
		t.Fatal("shared member edited source", code)
	}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/git/sources/%d", id), map[string]interface{}{"kind": "github", "name": "changed origin", "url": git.URL}, nil); code != 200 {
		t.Fatal(code)
	}
	if meta := loadGitRelayMeta(id); meta.Enabled || meta.Published != 0 {
		t.Fatal("source kind change retained GitLab cache", meta)
	}
}
