// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func mediumTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for n, v := range files {
		w, e := z.Create(n)
		if e != nil {
			t.Fatal(e)
		}
		w.Write([]byte(v))
	}
	z.Close()
	return b.Bytes()
}
func TestMediumZipBoundsAndURLRewrite(t *testing.T) {
	for _, name := range []string{"../app/main.py", "tool/../../escape", "/app/main.py", "tool\\app\\main.py", "C:/app/main.py"} {
		if e := mediumExtract(mediumTestZip(t, map[string]string{name: "bad"}), t.TempDir(), "tool"); e == nil {
			t.Fatalf("unsafe ZIP accepted: %s", name)
		}
	}
	files := map[string]string{"tool/app/main.py": "main", "tool/app/config.py": "config", "tool/app/__init__.py": "init", "tool/requirements.lock": "locked", "tool/private.env": "do not copy"}
	dir := t.TempDir()
	if e := mediumExtract(mediumTestZip(t, files), dir, "tool"); e != nil {
		t.Fatal(e)
	}
	if mediumFileExists(filepath.Join(dir, "private.env")) {
		t.Fatal("unrelated private file extracted")
	}
	html := `<a href="/">Home</a><form action='/postavke/yaml'><button hx-post="/usporedba/123/plan">Plan</button></form><script src="/static/app.js"></script><a href="https://example.com">External</a><a href="//example.com">External</a><div hx-target="#result" sse-connect="/events"></div>`
	got := string(mediumRewriteHTML([]byte(html)))
	for _, v := range []string{`href="/medium/"`, `action='/medium/postavke/yaml'`, `hx-post="/medium/usporedba/123/plan"`, `src="/medium/static/app.js"`, `href="https://example.com"`, `href="//example.com"`, `hx-target="#result"`, `sse-connect="/medium/events"`} {
		if !strings.Contains(got, v) {
			t.Fatalf("rewrite missing %s: %s", v, got)
		}
	}
	if twice := string(mediumRewriteHTML([]byte(got))); twice != got {
		t.Fatal("URL rewrite is not idempotent")
	}
}
func TestMediumProxyAuthHeadersActionsAndRedirect(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "medium-admin", true)
	other := newTestUser(t, srv, "medium-other", false)
	secret := "private-loopback-secret"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-WRM-Medium-Key") != secret || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Sec-Fetch-Site") != "same-origin" {
			t.Error("upstream auth/header boundary")
		}
		if r.URL.Path == "/postavke/osvjezi" {
			w.Header().Set("Location", "/postavke?osvjezeno=1")
			w.WriteHeader(303)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<a href="/usporedba">Compare</a>`)
	}))
	defer upstream.Close()
	p := &mediumProcess{base: upstream.URL, secret: secret, done: make(chan struct{})}
	mediumMgr.mu.Lock()
	mediumMgr.processes[mediumUserDir(u.userID)] = p
	mediumMgr.mu.Unlock()
	defer func() {
		mediumMgr.mu.Lock()
		delete(mediumMgr.processes, mediumUserDir(u.userID))
		mediumMgr.mu.Unlock()
	}()
	req, _ := http.NewRequest("GET", srv.URL+"/medium/", nil)
	req.AddCookie(u.cookie)
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(b) != `<a href="/medium/usporedba">Compare</a>` || len(resp.Header.Values("X-Frame-Options")) != 1 || resp.Header.Get("X-Frame-Options") != "SAMEORIGIN" || len(resp.Header.Values("Content-Security-Policy")) != 1 {
		t.Fatalf("proxy: %d %s %v", resp.StatusCode, b, resp.Header)
	}
	if code := other.jsonDo("GET", "/medium/", nil, nil); code != 403 {
		t.Fatal(code)
	}
	req, _ = http.NewRequest("GET", srv.URL+"/medium/", nil)
	req.AddCookie(u.cookie)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("cross-site read accepted")
	}
	req, _ = http.NewRequest("POST", srv.URL+"/medium/postavke/osvjezi", strings.NewReader(""))
	req.AddCookie(u.cookie)
	req.Header.Set("Origin", srv.URL)
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e = cl.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 303 || resp.Header.Get("Location") != "/medium/postavke?osvjezeno=1" {
		t.Fatalf("redirect: %d %v", resp.StatusCode, resp.Header)
	}
	for _, site := range []string{"same-origin", "same-site", "cross-site", ""} {
		req, _ = http.NewRequest("POST", srv.URL+"/medium/postavke/osvjezi", strings.NewReader(""))
		req.AddCookie(u.cookie)
		req.Header.Set("Origin", "null")
		req.Header.Set("Sec-Fetch-Site", site)
		resp, e = cl.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
		want := 403
		if site == "same-origin" {
			want = 303
		}
		if resp.StatusCode != want {
			t.Fatalf("opaque form origin (%s): %d", site, resp.StatusCode)
		}
	}
	req, _ = http.NewRequest("POST", srv.URL+"/medium/postavke/osvjezi", strings.NewReader(""))
	req.AddCookie(u.cookie)
	req.Header.Set("Origin", "https://evil.example")
	resp, _ = cl.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("cross-origin write accepted")
	}
}
func TestFolderManualOrderOwnershipHierarchyAndExport(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "folder-manual", false)
	other := newTestUser(t, srv, "folder-manual-other", false)
	var a, b, c Folder
	for i, n := range []string{"Zulu", "Alpha", "Bravo"} {
		out := []*Folder{&a, &b, &c}[i]
		if code := u.jsonDo("POST", "/api/folders", map[string]string{"name": n}, out); code != 201 {
			t.Fatal(code)
		}
	}
	if fs := loadFolders(u.userID); fs[0].ID != a.ID {
		t.Fatal("creation order lost")
	}
	if code := u.jsonDo("POST", "/api/folders/reorder", map[string]interface{}{"id": c.ID, "before_id": a.ID}, nil); code != 200 {
		t.Fatal(code)
	}
	if fs := loadFolders(u.userID); fs[0].ID != c.ID || fs[1].ID != a.ID {
		t.Fatalf("order: %+v", fs)
	}
	if code := other.jsonDo("POST", "/api/folders/reorder", map[string]interface{}{"id": c.ID}, nil); code != 404 {
		t.Fatal("ownership")
	}
	if code := u.jsonDo("POST", "/api/folders/reorder", map[string]interface{}{"id": b.ID, "parent_id": a.ID}, nil); code != 200 {
		t.Fatal(code)
	}
	if code := u.jsonDo("POST", "/api/folders/reorder", map[string]interface{}{"id": a.ID, "parent_id": b.ID}, nil); code != 400 {
		t.Fatal("cycle")
	}
	if code := u.jsonDo("POST", "/api/folders/reorder", map[string]interface{}{"id": a.ID, "before_id": b.ID}, nil); code != 400 {
		t.Fatal("non-sibling anchor")
	}
	var export map[string]interface{}
	u.jsonDo("GET", "/api/config/export", nil, &export)
	copyUser := newTestUser(t, srv, "manual-order-copy", false)
	if code := copyUser.jsonDo("POST", "/api/config/import", export, nil); code != 200 {
		t.Fatal(code)
	}
	fs := loadFolders(copyUser.userID)
	roots := []Folder{}
	for _, f := range fs {
		if f.ParentID == nil {
			roots = append(roots, f)
		}
	}
	if len(fs) != 3 || len(roots) != 2 || roots[0].Name != "Bravo" {
		t.Fatalf("order did not round trip: %+v", fs)
	}
}
func TestConfigProfilesInspectMaskAndOwnership(t *testing.T) {
	e := newDeployEnv(t, "config-profile")
	writeFile(t, e.dir+"/run.py", runV110)
	writeFile(t, e.dir+"/lib/util.py", utilV1)
	writeFile(t, e.dir+"/storage_backend.ini", "[operator-a]\nbackend=filesystem\nworkers=8\npassword=actual-super-secret\n")
	in := e.check(t)
	var profile gitConfigProfile
	if code := e.u.jsonDo("POST", "/api/git/config-profiles", map[string]interface{}{"name": "Storage", "app": "demo-api", "path": "storage_backend.ini", "content": "[operator-a]\nbackend=s3\nworkers=4\npassword=reference-super-secret\n"}, &profile); code != 201 {
		t.Fatal(code)
	}
	var stored string
	db.QueryRow("SELECT data FROM git_config_profiles WHERE id=?", profile.ID).Scan(&stored)
	if strings.Contains(stored, "reference-super-secret") {
		t.Fatal("reference secret stored")
	}
	var result struct {
		Profile gitConfigProfile         `json:"profile"`
		Rows    []gitConfigProfileResult `json:"rows"`
	}
	if code := e.u.jsonDo("POST", fmt.Sprintf("/api/git/config-profiles/%d/check", profile.ID), map[string]interface{}{"ids": []int{in.ID}}, &result); code != 200 {
		t.Fatal(code)
	}
	if len(result.Rows) != 1 || len(result.Rows[0].File.Entries) != 3 || strings.Contains(string(jsonMarshal(result)), "actual-super-secret") {
		t.Fatalf("inspection: %+v", result)
	}
	if result.Rows[0].File.Entries[0].Section != "operator-a" || result.Rows[0].File.Entries[0].Value != "filesystem" {
		t.Fatal("operator values lost")
	}
	other := newTestUser(t, e.u.srv, "profile-other", false)
	if code := other.jsonDo("POST", fmt.Sprintf("/api/git/config-profiles/%d/check", profile.ID), map[string]interface{}{}, nil); code != 404 {
		t.Fatal("profile ownership")
	}
	if code := e.u.jsonDo("POST", "/api/git/config-profiles", map[string]interface{}{"name": "escape", "path": "../config.ini", "content": "x=1"}, nil); code != 400 {
		t.Fatal("path escape")
	}
}
func TestBitwardenLoginNativeKeyAndLock(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dir := t.TempDir()
	script := filepath.Join(dir, "bw")
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(priv)
	key := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	pub, _ := ssh.NewPublicKey(priv.Public())
	id := "12345678-1234-1234-1234-123456789abc"
	raw := map[string]interface{}{"id": id, "name": "Demo SSH key", "type": 5, "sshKey": map[string]string{"privateKey": key, "publicKey": string(ssh.MarshalAuthorizedKey(pub)), "keyFingerprint": ssh.FingerprintSHA256(pub)}}
	item := filepath.Join(dir, "item.json")
	os.WriteFile(item, jsonMarshal(raw), 0600)
	code := `#!/bin/sh
printf '%s\n' "$*" >> ` + shellQuote(filepath.Join(dir, "arguments.log")) + `
printf '{"accessToken":"mock-access-secret","vault":"encrypted-by-provider"}' > "$BITWARDENCLI_APPDATA_DIR/data.json"
case "$1" in
 status) printf '{"status":"locked","serverUrl":"https://vault.bitwarden.com","userEmail":"demo@example.com"}';;
 config|logout|lock|sync) exit 0;;
 login|unlock) [ "$WRM_BW_PASSWORD" = 'test-master-password' ] || exit 2; printf 'session-test-unlock-key-1234567890';;
 list) printf '['; cat ` + shellQuote(item) + `; printf ']';;
 get) cat ` + shellQuote(item) + `;;
 *) exit 3;;
esac
`
	os.WriteFile(script, []byte(code), 0700)
	t.Setenv("WRM_BITWARDEN_CLI", script)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "bw-user", false)
	other := newTestUser(t, srv, "bw-other", false)
	login := map[string]string{"email": "demo@example.com", "master_password": "test-master-password"}
	if result := u.jsonDo("POST", "/api/bitwarden/login", login, nil); result != 200 {
		t.Fatal(result)
	}
	if mediumFileExists(filepath.Join(bitwardenDir(u.userID), "data.json")) {
		t.Fatal("plaintext Bitwarden profile left at rest")
	}
	cache, err := os.ReadFile(filepath.Join(bitwardenDir(u.userID), "data.json.enc"))
	if err != nil || !bytes.HasPrefix(cache, []byte("ENC:")) || bytes.Contains(cache, []byte("mock-access-secret")) {
		t.Fatal("CLI profile is not encrypted at rest")
	}
	var keys []bitwardenKeyView
	if code := u.jsonDo("GET", "/api/bitwarden/keys", nil, &keys); code != 200 || len(keys) != 1 || keys[0].ID != id {
		t.Fatalf("key listing: %d %+v", code, keys)
	}
	if code := other.jsonDo("GET", "/api/bitwarden/keys", nil, nil); code != 409 {
		t.Fatal("vault sessions shared between users")
	}
	host := startFakeHost(t, testSSHUser, testSSHPass)
	writeFile(t, filepath.Join(host.home, ".ssh", "authorized_keys"), string(ssh.MarshalAuthorizedKey(pub)))
	var cv connView
	if code := u.jsonDo("POST", "/api/connections", Connection{Name: "Demo", Protocol: "SSH", Host: host.addr, Username: host.user, AuthMethod: "BITWARDEN", Options: map[string]string{"bitwarden_item": id}}, &cv); code != 201 {
		t.Fatal(code)
	}
	c, e := loadConnection(cv.ID)
	if e != nil || c.PrivateKey != key || c.AuthMethod != "KEY" {
		t.Fatalf("key resolution: %v %s", e, c.authErr)
	}
	client, err := dialSSH(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if host.keyLogins.Load() == 0 {
		t.Fatal("external key was not used for SSH")
	}
	var stored string
	db.QueryRow("SELECT private_key FROM connections WHERE id=?", cv.ID).Scan(&stored)
	if stored != "" {
		t.Fatal("external private key copied to connection")
	}
	args, _ := os.ReadFile(filepath.Join(dir, "arguments.log"))
	if strings.Contains(string(args), "test-master-password") || strings.Contains(string(args), "session-test-unlock-key") {
		t.Fatal("secret in command arguments")
	}
	if raw := string(jsonMarshal(keys)); strings.Contains(raw, "PRIVATE KEY") {
		t.Fatal("private key sent to browser")
	}
	if code := u.jsonDo("POST", "/api/bitwarden/lock", map[string]string{}, nil); code != 200 {
		t.Fatal(code)
	}
	if _, e := bitwardenPrivateKey(u.userID, id); e == nil {
		t.Fatal("locked key accessible")
	}
	s := bitwardenState(u.userID)
	s.mu.Lock()
	s.key = "expired-session-key"
	s.used = time.Now().Add(-24 * time.Hour)
	s.mu.Unlock()
	if _, e := bitwardenPrivateKey(u.userID, id); e == nil {
		t.Fatal("idle session accepted")
	}
}

func TestNativeSSHAndRsyncInterop(t *testing.T) {
	for _, binary := range []string{"ssh", "ssh-keyscan", "rsync"} {
		if _, e := exec.LookPath(binary); e != nil {
			t.Skip("native SSH/rsync not installed")
		}
	}
	host := startFakeHost(t, testSSHUser, testSSHPass)
	host.nativeExec.Store(true)
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	pub, _ := ssh.NewPublicKey(private.Public())
	writeFile(t, filepath.Join(host.home, ".ssh", "authorized_keys"), string(ssh.MarshalAuthorizedKey(pub)))
	dir := t.TempDir()
	block, _ := ssh.MarshalPrivateKey(private, "Demo")
	keyPath := filepath.Join(dir, "id_ed25519")
	os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600)
	ip, port, _ := net.SplitHostPort(host.addr)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	known, e := exec.CommandContext(ctx, "ssh-keyscan", "-t", "ed25519", "-p", port, ip).Output()
	if e != nil || len(known) == 0 {
		t.Fatal("test host key scan failed")
	}
	knownPath := filepath.Join(dir, "known_hosts")
	os.WriteFile(knownPath, known, 0600)
	source := filepath.Join(dir, "source")
	writeFile(t, filepath.Join(source, "hello.txt"), "native interop\n")
	destination := filepath.Join(host.home, "native-rsync")
	os.MkdirAll(destination, 0700)
	transport := "ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=" + shellQuote(knownPath) + " -i " + shellQuote(keyPath) + " -p " + port
	cmd := exec.CommandContext(ctx, "rsync", "-rlc", "-e", transport, source+"/", host.user+"@"+ip+":"+destination+"/")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("native rsync: %v %s", e, b)
	}
	content, e := os.ReadFile(filepath.Join(destination, "hello.txt"))
	if e != nil || string(content) != "native interop\n" {
		t.Fatal("native transfer did not finish")
	}
}
