// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// v13.1: the community index is listed with trust levels, and an install is
// only accepted when the archive matches the SHA-256 in the index. A tampered
// or redirected download installs nothing.
func TestV130CommunityInstall(t *testing.T) {
	zipBody := addonZip(t, map[string]string{
		"addon.json": addonDemoManifest("community.demo"),
		"panel.html": "<p>community demo</p>",
		"theme.css":  ":root{--accent:#0088ff}",
	})
	sum := sha256.Sum256(zipBody)
	good := hex.EncodeToString(sum[:])

	var srv *httptest.Server
	index := func(sha string) string {
		entry := map[string]interface{}{
			"id": "community.demo", "name": "Community demo", "version": "1.0.0", "license": "MIT",
			"capabilities": []string{"ui.panel", "ui.theme"}, "trust": "reviewed",
			"url": srv.URL + "/demo.zip", "sha256": sha,
		}
		bad := map[string]interface{}{"id": "community.bad", "name": "Bad", "version": "1", "trust": "community", "url": srv.URL + "/demo.zip", "sha256": "abc"}
		b, _ := json.Marshal(map[string]interface{}{"addons": []interface{}{entry, bad}})
		return string(b)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(index(good)))
	})
	mux.HandleFunc("/demo.zip", func(w http.ResponseWriter, r *http.Request) { w.Write(zipBody) })
	srv = httptest.NewServer(mux)
	defer srv.Close()

	old := addonCommunityIndex
	addonCommunityIndex = srv.URL + "/index.json"
	addonCommunityCache.at = time.Time{}
	addonCommunityCache.list = nil
	defer func() {
		addonCommunityIndex = old
		addonCommunityCache.at = time.Time{}
		addonCommunityCache.list = nil
	}()

	server := newTestServer(t)
	admin := newTestUser(t, server, "comm-admin", true)
	user := newTestUser(t, server, "comm-user", false)
	var list struct {
		Addons []communityAddon `json:"addons"`
	}
	if code := user.jsonDo("GET", "/api/addons/community", nil, &list); code != 200 {
		t.Fatalf("list: %d", code)
	}
	// The entry with a malformed checksum is dropped.
	if len(list.Addons) != 1 || list.Addons[0].ID != "community.demo" || list.Addons[0].Trust != "reviewed" {
		t.Fatalf("unexpected index: %+v", list.Addons)
	}
	if code := user.jsonDo("POST", "/api/addons/community/install", map[string]string{"id": "community.demo"}, nil); code != 403 {
		t.Fatalf("a normal user installed a community addon: %d", code)
	}
	if code := admin.jsonDo("POST", "/api/addons/community/install", map[string]string{"id": "community.demo"}, nil); code != 200 {
		t.Fatalf("community install: %d", code)
	}
	rec, ok := addonGet("community.demo")
	if !ok || !strings.HasPrefix(rec.Source, "community:community.demo@reviewed") {
		t.Fatalf("the community source was not recorded: %+v", rec)
	}
	// A checksum that does not match the archive refuses the install.
	good = strings.Repeat("0", 64)
	addonCommunityCache.at = time.Time{}
	addonCommunityCache.list = nil
	resp := admin.do("POST", "/api/addons/community/install", bytes.NewReader([]byte(`{"id":"community.demo"}`)), "application/json")
	body := new(bytes.Buffer)
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 409 || !strings.Contains(body.String(), "checksum") {
		t.Fatalf("a mismatched checksum was not refused: %d %s", resp.StatusCode, body.String())
	}
	if code := admin.do("GET", "/api/addons/community", nil, ""); code.StatusCode != 200 {
		t.Fatalf("index after tamper: %d", code.StatusCode)
	}
	js, _ := readFileString("static/addons.js")
	for _, want := range []string{"wrmAddons.communityInstall", "add_community", "add_trust_reviewed", "/api/addons/community"} {
		if !strings.Contains(js, want) {
			t.Fatalf("addons.js lacks %s", want)
		}
	}
}

func readFileString(name string) (string, error) {
	b, err := os.ReadFile(name)
	return string(b), err
}
