// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGitHub serves the releases API of one repository ("demo/wrm") and the release files.
type fakeGitHub struct {
	srv      *httptest.Server
	mu       sync.Mutex
	tag      string
	files    map[string][]byte // release files (name → content)
	sums     string            // SHA256SUMS.txt; "" = the release has no checksums file
	requests []*http.Request
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	g := &fakeGitHub{files: map[string][]byte{}}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.requests = append(g.requests, r.Clone(r.Context()))
		switch {
		case r.URL.Path == "/repos/demo/wrm/releases/latest" || r.URL.Path == "/repos/demo/wrm/releases/tags/"+g.tag:
			assets := []map[string]interface{}{}
			for name, data := range g.files {
				assets = append(assets, map[string]interface{}{"name": name, "size": len(data), "browser_download_url": g.srv.URL + "/download/" + name})
			}
			if g.sums != "" {
				assets = append(assets, map[string]interface{}{"name": checksumsAsset, "size": len(g.sums), "browser_download_url": g.srv.URL + "/download/" + checksumsAsset})
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"tag_name": g.tag, "name": "Web Remote Manager PRO " + g.tag,
				"body": "## Added\n- Something new", "html_url": "https://example.com/releases/" + g.tag, "published_at": "2026-10-01T10:00:00Z", "assets": assets})
		case strings.HasPrefix(r.URL.Path, "/download/"):
			name := strings.TrimPrefix(r.URL.Path, "/download/")
			if name == checksumsAsset && g.sums != "" {
				w.Write([]byte(g.sums))
				return
			}
			if data, ok := g.files[name]; ok {
				w.Write(data)
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGitHub) set(f func()) {
	g.mu.Lock()
	f()
	g.mu.Unlock()
}

func binSHA256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func fakeScript(answer string) []byte { return []byte("#!/bin/sh\necho " + answer + "\n") }

// updateEnv points the updater at a fake GitHub and a fake binary in a temporary folder.
type updateEnv struct {
	t        *testing.T
	gh       *fakeGitHub
	dir, exe string
	asset    string
	restarts chan string
	admin    *testClient
	user     *testClient
}

func newUpdateEnv(t *testing.T) *updateEnv {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell scripts as fake binaries")
	}
	e := &updateEnv{t: t, gh: newFakeGitHub(t), dir: t.TempDir(), restarts: make(chan string, 4)}
	label := platformLabels(runtime.GOOS, runtime.GOARCH, buildGOARM)[0]
	e.exe = filepath.Join(e.dir, "wrm-pro-v11.7.0-"+label)
	os.WriteFile(e.exe, fakeScript("v11.7.0"), 0o755)
	e.asset = releaseAssetName("v11.8.0", label)

	oldBase, oldVersion, oldExe, oldHook, oldContainer := githubAPIBase, AppVersion, selfExecutable, restartHook, inContainer
	githubAPIBase, AppVersion = e.gh.srv.URL, "v11.7.0"
	selfExecutable = func() (string, error) { return e.exe, nil }
	restartHook = func(target string) { e.restarts <- target }
	inContainer = func() bool { return false }
	setSetting("update_repo", "demo/wrm")
	t.Cleanup(func() {
		githubAPIBase, AppVersion, selfExecutable, restartHook, inContainer = oldBase, oldVersion, oldExe, oldHook, oldContainer
		setSetting("update_repo", defaultUpdateRepo)
		setSetting("self_update", "admins")
		storeUpdateInfo(updateInfo{})
		setUpdateProgress(updateProgressInfo{State: "idle"})
		forgetLocalBinary()
		forgetPreviousBinary()
	})
	forgetLocalBinary()
	forgetPreviousBinary()
	srv := newTestServer(t)
	e.admin = newTestUser(t, srv, fmt.Sprintf("upd-admin-%d", time.Now().UnixNano()), true)
	e.user = newTestUser(t, srv, fmt.Sprintf("upd-user-%d", time.Now().UnixNano()), false)
	return e
}

// release publishes v11.8.0 with the given binary and checksums file.
func (e *updateEnv) release(binary []byte, sums string, withAsset bool) {
	e.gh.set(func() {
		e.gh.tag = "v11.8.0"
		e.gh.files = map[string][]byte{"wrm-pro-v11.8.0-other-platform": []byte("x")}
		if withAsset {
			e.gh.files[e.asset] = binary
		}
		e.gh.sums = sums
	})
}

func (e *updateEnv) status(c *testClient) map[string]interface{} {
	var st map[string]interface{}
	if code := c.jsonDo("GET", "/api/update", nil, &st); code != 200 {
		e.t.Fatalf("status: %d", code)
	}
	return st
}

// apply starts an update and waits until the job ends.
func (e *updateEnv) apply() updateProgressInfo {
	e.t.Helper()
	if code := e.admin.jsonDo("POST", "/api/update/apply", map[string]string{"version": "v11.8.0"}, nil); code != 200 {
		e.t.Fatalf("apply: %d", code)
	}
	return e.waitJob()
}

func (e *updateEnv) waitJob() updateProgressInfo {
	e.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		updateMu.Lock()
		busy, p := updateMu.busy, updateMu.progress
		updateMu.Unlock()
		if !busy {
			return p
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.t.Fatal("update job did not finish")
	return updateProgressInfo{}
}

func (e *updateEnv) exeIs(answer string) {
	e.t.Helper()
	if data, _ := os.ReadFile(e.exe); string(data) != string(fakeScript(answer)) {
		e.t.Fatalf("binary: %q, want the %s binary", data, answer)
	}
}

func (e *updateEnv) noLeftovers() {
	e.t.Helper()
	if left, _ := filepath.Glob(filepath.Join(e.dir, ".wrm-update-*")); len(left) > 0 {
		e.t.Fatalf("temporary files left: %v", left)
	}
}

func updateAuditCount(action, target string) int {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action=? AND target=?`, action, target).Scan(&n)
	return n
}

func TestSelfUpdateFromFakeGitHub(t *testing.T) {
	e := newUpdateEnv(t)
	newBin := fakeScript("v11.8.0")
	e.release(newBin, binSHA256(newBin)+"  "+e.asset+"\n"+binSHA256([]byte("x"))+"  wrm-pro-v11.8.0-other-platform\n", true)

	// Check: the badge data for everyone, the update controls for administrators only.
	var st map[string]interface{}
	if code := e.admin.jsonDo("POST", "/api/update/check", nil, &st); code != 200 || st["latest"] != "v11.8.0" || st["available"] != true || st["can_update"] != true {
		t.Fatalf("check: %d %v", code, st)
	}
	if !strings.Contains(st["notes"].(string), "Something new") || st["url"] != "https://example.com/releases/v11.8.0" {
		t.Fatalf("notes: %v", st)
	}
	us := e.status(e.user)
	if us["available"] != true || us["latest"] != "v11.8.0" || us["can_update"] != nil || us["progress"] != nil {
		t.Fatalf("user status: %v", us)
	}
	if e.user.jsonDo("POST", "/api/update/check", nil, nil) != 403 || e.user.jsonDo("POST", "/api/update/apply", map[string]string{"version": "v11.8.0"}, nil) != 403 {
		t.Fatal("users must not check or update")
	}
	var ver map[string]string
	e.admin.jsonDo("GET", "/api/version", nil, &ver)
	if ver["version"] != "v11.7.0" || ver["update"] != "v11.8.0" {
		t.Fatalf("/api/version: %v", ver)
	}
	// No user data goes to GitHub: no cookies or credentials, a fixed User-Agent.
	e.gh.set(func() {
		for _, r := range e.gh.requests {
			if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != updateUserAgent || r.URL.RawQuery != "" {
				t.Errorf("request to GitHub carries more than the repository: %s %v", r.URL, r.Header)
			}
		}
	})
	if e.admin.jsonDo("POST", "/api/update/apply", map[string]string{"version": "v11.7.0"}, nil) != 400 {
		t.Fatal("not newer must be refused")
	}

	// Update: download, verify, -version, replace, keep the previous binary, restart.
	p := e.apply()
	if p.State != "restarting" || p.Error != "" {
		t.Fatalf("progress: %+v", p)
	}
	select {
	case target := <-e.restarts:
		if target != "" {
			t.Fatalf("restart target %q", target)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no restart")
	}
	e.exeIs("v11.8.0")
	if data, _ := os.ReadFile(e.exe + ".previous"); string(data) != string(fakeScript("v11.7.0")) {
		t.Fatalf("previous binary: %q", data)
	}
	if fi, _ := os.Stat(e.exe); fi.Mode()&0o111 == 0 {
		t.Fatal("new binary not executable")
	}
	e.noLeftovers()
	for _, a := range []string{"system.update_check", "system.update_started", "system.update_downloaded", "system.update_verified", "system.update_installed", "system.update_restart"} {
		target := "v11.8.0"
		if a == "system.update_check" {
			target = "demo/wrm"
		}
		if updateAuditCount(a, target) == 0 {
			t.Errorf("no audit entry %s", a)
		}
	}

	// Rollback to the previous binary (one click), then restart.
	forgetPreviousBinary()
	if st := e.status(e.admin); st["rollback"] != "v11.7.0" {
		t.Fatalf("rollback offer: %v", st["rollback"])
	}
	if code := e.admin.jsonDo("POST", "/api/update/rollback", nil, nil); code != 200 {
		t.Fatalf("rollback: %d", code)
	}
	if p := e.waitJob(); p.State != "restarting" {
		t.Fatalf("rollback progress: %+v", p)
	}
	<-e.restarts
	e.exeIs("v11.7.0")
	if data, _ := os.ReadFile(e.exe + ".previous"); string(data) != string(newBin) {
		t.Fatalf("after rollback the newer binary is the previous one: %q", data)
	}
	if updateAuditCount("system.update_rollback", "v11.7.0") == 0 {
		t.Fatal("rollback not audited")
	}
	e.noLeftovers()
}

func TestSelfUpdateRefusals(t *testing.T) {
	e := newUpdateEnv(t)
	newBin := fakeScript("v11.8.0")
	e.admin.jsonDo("POST", "/api/update/check", nil, nil)

	cases := []struct {
		name      string
		bin       []byte
		sums      string
		withAsset bool
		want      string
	}{
		{"missing checksums file", newBin, "", true, "no " + checksumsAsset},
		{"checksum mismatch", newBin, binSHA256([]byte("tampered")) + "  " + e.asset + "\n", true, "does not match"},
		{"no line for the file", newBin, binSHA256(newBin) + "  some-other-file\n", true, "has no line for"},
		{"missing asset", newBin, binSHA256(newBin) + "  " + e.asset + "\n", false, "no file for this platform"},
		{"wrong -version answer", fakeScript("v11.7.5"), binSHA256(fakeScript("v11.7.5")) + " *" + e.asset + "\n", true, "instead of v11.8.0"},
	}
	for _, c := range cases {
		e.release(c.bin, c.sums, c.withAsset)
		p := e.apply()
		if p.State != "failed" || !strings.Contains(p.Error, c.want) {
			t.Fatalf("%s: %+v", c.name, p)
		}
		e.exeIs("v11.7.0")
		e.noLeftovers()
		if _, err := os.Stat(e.exe + ".previous"); err == nil {
			t.Fatalf("%s: previous binary written", c.name)
		}
		select {
		case <-e.restarts:
			t.Fatalf("%s: restarted", c.name)
		default:
		}
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='system.update_failed' AND target='v11.8.0'`).Scan(&n)
	if n < len(cases) {
		t.Fatalf("failures audited: %d", n)
	}

	// Policy off: nobody updates.
	setSetting("self_update", "off")
	if e.admin.jsonDo("POST", "/api/update/apply", map[string]string{"version": "v11.8.0"}, nil) != 403 {
		t.Fatal("policy self_update=off")
	}
	if st := e.status(e.admin); st["can_update"] != false {
		t.Fatalf("can_update with the policy off: %v", st)
	}
	setSetting("self_update", "admins")
}

func TestSelfUpdateAppImageBlocked(t *testing.T) {
	e := newUpdateEnv(t)
	t.Setenv("WRM_APPIMAGE", "1")
	st := e.status(e.admin)
	if st["blocked"] != "appimage" || st["can_update"] != false {
		t.Fatalf("AppImage must not replace its mounted or temporarily extracted binary: %v", st)
	}
	for _, endpoint := range []string{"apply", "rollback", "restart"} {
		if code := e.admin.jsonDo("POST", "/api/update/"+endpoint, map[string]string{"version": "v11.8.0"}, nil); code != 409 {
			t.Fatalf("AppImage %s: got HTTP %d, want 409", endpoint, code)
		}
	}
}

func TestSelfUpdateDockerAndLocalBinary(t *testing.T) {
	e := newUpdateEnv(t)
	newBin := fakeScript("v11.8.0")
	e.release(newBin, binSHA256(newBin)+"  "+e.asset+"\n", true)
	e.admin.jsonDo("POST", "/api/update/check", nil, nil)

	// In a container WRM never replaces itself: the badge stays, the update is refused.
	inContainer = func() bool { return true }
	st := e.status(e.admin)
	if st["docker"] != true || st["can_update"] != false || st["blocked"] != "docker" || st["available"] != true || !strings.HasPrefix(st["docker_image"].(string), "ghcr.io/demo/") {
		t.Fatalf("docker status: %v", st)
	}
	if e.admin.jsonDo("POST", "/api/update/apply", map[string]string{"version": "v11.8.0"}, nil) != 409 {
		t.Fatal("docker update must be refused")
	}
	e.exeIs("v11.7.0")
	inContainer = func() bool { return false }

	// A newer binary copied into the folder: "Restart to v…".
	label := platformLabels(runtime.GOOS, runtime.GOARCH, buildGOARM)[0]
	local := filepath.Join(e.dir, "wrm-pro-v11.9.0-"+label)
	os.WriteFile(local, fakeScript("v11.9.0"), 0o755)
	forgetLocalBinary()
	st = e.status(e.admin)
	lb, _ := st["local"].(map[string]interface{})
	if lb == nil || lb["version"] != "v11.9.0" || lb["file"] != filepath.Base(local) {
		t.Fatalf("local binary: %v", st["local"])
	}
	if code := e.admin.jsonDo("POST", "/api/update/restart", map[string]bool{"local": true}, nil); code != 200 {
		t.Fatalf("restart: %d", code)
	}
	select {
	case target := <-e.restarts:
		if target != local {
			t.Fatalf("restart target %q", target)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no restart")
	}
	if updateAuditCount("system.update_restart", "v11.9.0") == 0 {
		t.Fatal("restart not audited")
	}

	// The Windows rename of the last update becomes the rollback copy on the next start.
	os.WriteFile(e.exe+".old", fakeScript("v11.6.0"), 0o755)
	os.WriteFile(filepath.Join(e.dir, ".wrm-update-123"), []byte("partial"), 0o600)
	cleanupReplacedBinary()
	if data, _ := os.ReadFile(e.exe + ".previous"); string(data) != string(fakeScript("v11.6.0")) {
		t.Fatalf(".old → .previous: %q", data)
	}
	e.noLeftovers()

	// Settings: repository and proxy are validated.
	if _, err := setSetting("update_repo", "not a repo"); err == nil {
		t.Fatal("bad repository accepted")
	}
	if _, err := setSetting("update_proxy", "ftp://proxy"); err == nil {
		t.Fatal("bad proxy accepted")
	}
	if _, err := setSetting("update_proxy", "http://proxy.example.com:3128"); err != nil {
		t.Fatal(err)
	}
	setSetting("update_proxy", "")
}
