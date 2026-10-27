// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// v13.14.0: Git workspace names, log files outside checks, folder sorting (#134), file edit
// history (#129) and SSH user certificates / opkssh (#119).

func TestV13140HostnameAndLabels(t *testing.T) {
	if h := parseHostnameOutput("Welcome to demo\nWRM_HOSTNAME demo-api-01\n"); h != "demo-api-01" {
		t.Fatalf("marked hostname: %q", h)
	}
	if h := parseHostnameOutput("demo-web\n"); h != "demo-web" {
		t.Fatalf("plain hostname: %q", h)
	}
	if h := parseHostnameOutput("WRM_HOSTNAME bad name!\n"); h != "" {
		t.Fatalf("invalid hostname: %q", h)
	}
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "v13140-hosts", false)
	u.addConnection("by-ip", "203.0.113.7:22")
	u.addConnection("by-ipv6", "[2001:db8::7]:22")
	u.addConnection("by-name", "demo-web.example.com:22")
	labels := hostLabels(u.userID)
	if _, ok := labels["by-ip"]; ok {
		t.Fatalf("an IP address must not be the label: %v", labels)
	}
	if _, ok := labels["by-ipv6"]; ok {
		t.Fatalf("an IPv6 address must not be the label: %v", labels)
	}
	if labels["by-name"] != "demo-web.example.com:22" {
		t.Fatalf("DNS name label: %v", labels)
	}
}

func TestV13140LogPaths(t *testing.T) {
	for p, want := range map[string]bool{
		"log/app.log": true, "logs/2026/worker.txt": true, "app.log": true, "worker.log.1": true, "err.log.gz": true,
		"LOG/Out": true, "log/__init__.py": false, "logs/settings.json": false, "logger.py": false, "catalog/x": false,
		"blog/post.md": false, "logging.config": false,
	} {
		if got := isLogPath(p); got != want {
			t.Errorf("isLogPath(%q) = %v, want %v", p, got, want)
		}
	}
	// a target never takes log files, unless the include list names them
	tree := map[string]string{"linux/run.py": "a", "linux/log/old.log": "b", "linux/logs/x.txt": "c"}
	if got := selectFiles(tree, gitCatalogApp{Subdir: "linux"}); len(got) != 1 || got["run.py"] == "" {
		t.Fatalf("target with logs: %v", got)
	}
	if got := selectFiles(tree, gitCatalogApp{Subdir: "linux", Include: []string{"run.py", "log"}}); len(got) != 2 || got["log/old.log"] == "" {
		t.Fatalf("included log: %v", got)
	}
}

func TestV13140FolderSort(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "v13140-folders", false)
	other := newTestUser(t, srv, "v13140-folders-other", false)
	mk := func(name string, parent int) int {
		t.Helper()
		body := map[string]interface{}{"name": name}
		if parent > 0 {
			body["parent_id"] = parent
		}
		var f Folder
		if code := u.jsonDo("POST", "/api/folders", body, &f); code != 201 || f.ID == 0 {
			t.Fatalf("create %s: %d", name, code)
		}
		return f.ID
	}
	order := func(parent int) string {
		var names []string
		for _, f := range loadFolders(u.userID) {
			if (parent == 0 && f.ParentID == nil) || (f.ParentID != nil && *f.ParentID == parent) {
				names = append(names, f.Name)
			}
		}
		return strings.Join(names, ",")
	}
	srv10 := mk("srv10", 0)
	mk("srv2", 0)
	mk("Alpha", 0)
	mk("z", srv10)
	mk("b", srv10)
	if order(0) != "srv10,srv2,Alpha" {
		t.Fatalf("manual order: %s", order(0))
	}
	var res struct {
		Moved   int  `json:"moved"`
		KeepAll bool `json:"keep_all"`
	}
	if code := u.jsonDo("POST", "/api/folders/sort", map[string]interface{}{"folder_id": 0, "recursive": false}, &res); code != 200 || order(0) != "Alpha,srv2,srv10" || res.Moved == 0 {
		t.Fatalf("sorted top level: %d %s %+v", code, order(0), res)
	}
	if order(srv10) != "z,b" {
		t.Fatalf("one level only: %s", order(srv10))
	}
	if code := other.jsonDo("POST", "/api/folders/sort", map[string]interface{}{"folder_id": srv10}, nil); code != 404 {
		t.Fatalf("another user's folder: %d", code)
	}
	// keep srv10 sorted: new children take their place by name
	if code := u.jsonDo("POST", "/api/folders/sort", map[string]interface{}{"folder_id": srv10, "keep": true}, nil); code != 200 || order(srv10) != "b,z" {
		t.Fatalf("keep sorted: %d %s", code, order(srv10))
	}
	mk("a", srv10)
	if order(srv10) != "a,b,z" {
		t.Fatalf("auto sort below a kept folder: %s", order(srv10))
	}
	mk("Beta", 0) // the top level is not kept sorted
	if order(0) != "Alpha,srv2,srv10,Beta" {
		t.Fatalf("top level not kept: %s", order(0))
	}
	if code := u.jsonDo("POST", "/api/folders/sort", map[string]interface{}{"folder_id": 0, "keep": true}, &res); code != 200 || !res.KeepAll || order(0) != "Alpha,Beta,srv2,srv10" {
		t.Fatalf("keep all: %d %+v %s", code, res, order(0))
	}
	mk("0-first", 0)
	if order(0) != "0-first,Alpha,Beta,srv2,srv10" {
		t.Fatalf("keep all on create: %s", order(0))
	}
	var st struct {
		KeepAll bool `json:"keep_all"`
	}
	if code := u.jsonDo("GET", "/api/folders/sort", nil, &st); code != 200 || !st.KeepAll {
		t.Fatalf("keep all state: %d %+v", code, st)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='folder.sorted' AND username='v13140-folders'`).Scan(&n)
	if n < 3 {
		t.Fatalf("sorting not audited: %d", n)
	}
	if naturalCompare("srv2", "SRV10") >= 0 || naturalCompare("a01", "a1") != 0 || naturalCompare("b", "a") <= 0 {
		t.Fatal("natural compare")
	}
}

func TestV13140FileEditHistory(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "v13140-editor", false)
	admin := newTestUser(t, srv, "v13140-edit-admin", true)
	conn := u.addConnection("edit-host", sshAddr)
	dir := t.TempDir()
	save := func(content string, edit bool) {
		t.Helper()
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("file", "app.ini")
		fw.Write([]byte(content))
		mw.Close()
		q := "/api/remote/upload?id=" + strconv.Itoa(conn) + "&path=" + dir + "&overwrite=true"
		if edit {
			q += "&edit=1"
		}
		resp := u.do("POST", q, &body, mw.FormDataContentType())
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("save: %d", resp.StatusCode)
		}
	}
	count := func() int {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM file_edits WHERE username='v13140-editor'`).Scan(&n)
		return n
	}
	save("[main]\nport=80\n", true) // a new file
	save("[main]\nport=8080\npassword=hunter2-secret\n", true)
	save("[main]\nport=9090\n", false) // a plain upload: no edit entry
	if count() != 2 {
		t.Fatalf("edits: %d", count())
	}
	var list []fileEditView
	if code := u.jsonDo("GET", "/api/admin/file-edits", nil, &list); code != 403 {
		t.Fatalf("a normal user must not read the edit history: %d", code)
	}
	if code := admin.jsonDo("GET", "/api/admin/file-edits?user=v13140-editor", nil, &list); code != 200 || len(list) != 2 {
		t.Fatalf("list: %d %+v", code, list)
	}
	last, first := list[0], list[1]
	if !first.Created || first.Added != 2 || last.Created || last.Added != 2 || last.Removed != 1 || last.Path != dir+"/app.ini" || last.Source != "editor" ||
		last.ConnName != "edit-host" || last.Diff != "" || last.OldSHA == "" || last.NewSHA == "" {
		t.Fatalf("entries: %+v / %+v", first, last)
	}
	var raw string
	db.QueryRow(`SELECT diff FROM file_edits WHERE id=?`, last.ID).Scan(&raw)
	if strings.Contains(raw, "port=8080") {
		t.Fatal("the diff must be encrypted at rest")
	}
	var one fileEditView
	if code := admin.jsonDo("GET", fmt.Sprintf("/api/admin/file-edits/%d", last.ID), nil, &one); code != 200 ||
		!strings.Contains(one.Diff, "-port=80") || !strings.Contains(one.Diff, "+port=8080") || strings.Contains(one.Diff, "hunter2-secret") {
		t.Fatalf("diff: %d %q", code, one.Diff)
	}
	var audited int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='file_edit.viewed' AND username='v13140-edit-admin'`).Scan(&audited)
	if audited != 1 {
		t.Fatalf("viewing a diff must be audited: %d", audited)
	}
	if code := admin.jsonDo("GET", "/api/admin/file-edits?source=ai&user=v13140-editor", nil, &list); code != 200 || len(list) != 0 {
		t.Fatalf("source filter: %d %+v", code, list)
	}
	resp := admin.do("GET", "/api/admin/file-edits?user=v13140-editor&format=csv", nil, "")
	csvBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(csvBody), "app.ini") || strings.Contains(string(csvBody), "hunter2") {
		t.Fatalf("csv: %s", csvBody)
	}
	// the policy switches the history off
	setSetting("file_edit_history", "0")
	defer setSetting("file_edit_history", "1")
	save("[main]\nport=1\n", true)
	if count() != 2 {
		t.Fatalf("history off: %d", count())
	}
	// recent entries are append-only
	if _, err := db.Exec(`DELETE FROM file_edits WHERE id=?`, last.ID); err == nil {
		t.Fatal("a recent file edit could be deleted")
	}
}

// fakeCertHost accepts user certificates signed by ca for user.
func fakeCertHost(t *testing.T, ca ssh.PublicKey, user string) string {
	t.Helper()
	_, hpriv, _ := ed25519.GenerateKey(rand.Reader)
	hs, _ := ssh.NewSignerFromKey(hpriv)
	checker := &ssh.CertChecker{IsUserAuthority: func(k ssh.PublicKey) bool { return bytes.Equal(k.Marshal(), ca.Marshal()) }}
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if _, ok := k.(*ssh.Certificate); !ok {
			return nil, fmt.Errorf("certificates only")
		}
		return checker.Authenticate(c, k)
	}}
	cfg.AddHostKey(hs)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	db.Exec(`DELETE FROM known_hosts WHERE host=?`, normalizeHostKeyHost(ln.Addr().String()))
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				go func() {
					for nch := range chans {
						nch.Reject(ssh.Prohibited, "no")
					}
				}()
				sc.Wait()
			}()
		}
	}()
	return ln.Addr().String()
}

func TestV13140SSHCertificates(t *testing.T) {
	_, caPriv, _ := ed25519.GenerateKey(rand.Reader)
	caSigner, _ := ssh.NewSignerFromKey(caPriv)
	_, userPriv, _ := ed25519.GenerateKey(rand.Reader)
	userSigner, _ := ssh.NewSignerFromKey(userPriv)
	block, _ := ssh.MarshalPrivateKey(userPriv, "demo")
	keyPEM := string(pem.EncodeToMemory(block))
	sign := func(pub ssh.PublicKey, until time.Time) string {
		t.Helper()
		c := &ssh.Certificate{Key: pub, CertType: ssh.UserCert, KeyId: "demo@example.com", ValidPrincipals: []string{"demo"},
			ValidAfter: uint64(time.Now().Add(-time.Hour).Unix()), ValidBefore: uint64(until.Unix()),
			Permissions: ssh.Permissions{Extensions: map[string]string{"permit-pty": ""}}}
		if err := c.SignCert(rand.Reader, caSigner); err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(c))) + " demo-cert"
	}
	certLine := sign(userSigner.PublicKey(), time.Now().Add(time.Hour))
	expired := sign(userSigner.PublicKey(), time.Now().Add(-time.Minute))
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	otherSigner, _ := ssh.NewSignerFromKey(otherPriv)
	foreign := sign(otherSigner.PublicKey(), time.Now().Add(time.Hour))

	// signers: certificate first, then the plain key
	c := Connection{Name: "cert", AuthMethod: "KEY", PrivateKey: keyPEM + "\n" + certLine}
	if s, err := certSigners(c, "", userSigner); err != nil || len(s) != 2 {
		t.Fatalf("signers: %v %d", err, len(s))
	} else if _, ok := s[0].PublicKey().(*ssh.Certificate); !ok {
		t.Fatal("the certificate must be offered first")
	}
	if _, err := certSigners(Connection{AuthMethod: "KEY", PrivateKey: keyPEM + "\n" + foreign}, "", userSigner); err == nil {
		t.Fatal("a certificate of another key must be refused")
	}
	if s, err := certSigners(Connection{AuthMethod: "KEY", PrivateKey: keyPEM + "\n" + expired}, "", userSigner); err != nil || len(s) != 1 {
		t.Fatalf("expired certificate: %v %d", err, len(s))
	}
	if kd, _, err := resolveKeyMaterialPath(c); err != nil || strings.Contains(string(kd), "cert-v01") {
		t.Fatalf("key text without the certificate: %v", err)
	}

	// a real login with the certificate: pasted, from a key file and from the key store
	addr := fakeCertHost(t, caSigner.PublicKey(), "demo")
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "v13140-cert", true) // key files on the WRM server: administrators
	dial := func(c Connection) error {
		c.Host, c.Protocol, c.Username, c.UserID = addr, "SSH", "demo", u.userID
		cl, err := dialSSH(c, nil)
		if err == nil {
			cl.Close()
		}
		return err
	}
	if err := dial(Connection{Name: "pasted", AuthMethod: "KEY", PrivateKey: keyPEM + "\n" + certLine}); err != nil {
		t.Fatalf("login with a pasted certificate: %v", err)
	}
	if err := dial(Connection{Name: "no-cert", AuthMethod: "KEY", PrivateKey: keyPEM}); err == nil {
		t.Fatal("the host accepts certificates only")
	}
	err := dial(Connection{Name: "expired", AuthMethod: "KEY", PrivateKey: keyPEM + "\n" + expired})
	if err == nil || !strings.Contains(err.Error(), "certificate") || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("an expired certificate must be named: %v", err)
	}
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "id_ed25519")
	os.WriteFile(keyFile, []byte(keyPEM), 0o600)
	os.WriteFile(keyFile+"-cert.pub", []byte(certLine+"\n"), 0o644)
	if err := dial(Connection{Name: "file", AuthMethod: "KEY_FILE", KeyPath: keyFile}); err != nil {
		t.Fatalf("login with a key file and its -cert.pub: %v", err)
	}
	if err := dial(Connection{Name: "cert-file", AuthMethod: "KEY_FILE", KeyPath: keyFile + "-cert.pub"}); err != nil {
		t.Fatalf("login with the path of the certificate: %v", err)
	}

	// key store: the certificate must belong to the key
	var k sshKey
	if code := u.jsonDo("POST", "/api/keys", map[string]interface{}{"name": "opk", "key": keyPEM}, &k); code != 200 && code != 201 {
		t.Fatalf("import key: %d", code)
	}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/keys/%d", k.ID), map[string]interface{}{"certificate": foreign}, nil); code != 400 {
		t.Fatalf("foreign certificate: %d", code)
	}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/keys/%d", k.ID), map[string]interface{}{"certificate": strings.Fields(keyPEM)[0]}, nil); code != 400 {
		t.Fatalf("not a certificate: %d", code)
	}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/keys/%d", k.ID), map[string]interface{}{"certificate": certLine}, nil); code != 200 {
		t.Fatalf("set certificate: %d", code)
	}
	var got sshKey
	if code := u.jsonDo("GET", fmt.Sprintf("/api/keys/%d", k.ID), nil, &got); code != 200 || got.CertInfo == nil || got.CertInfo.KeyID != "demo@example.com" ||
		got.CertInfo.Expired || len(got.CertInfo.Principals) != 1 || got.Name != "opk" {
		t.Fatalf("key with certificate: %d %+v", code, got)
	}
	kid := k.ID
	if err := dial(Connection{Name: "store", AuthMethod: "KEY_REF", KeyID: &kid}); err != nil {
		t.Fatalf("login with a stored key and its certificate: %v", err)
	}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/keys/%d", k.ID), map[string]interface{}{"certificate": ""}, nil); code != 200 || storedKeyCert(&kid, u.userID) != "" {
		t.Fatalf("remove certificate: %d", code)
	}
}
