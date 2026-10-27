// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ─── fake server: real sh for scripts, simulated passwd ──

// fakeHost is an SSH server with one account. Commands run in a real sh with HOME set to a
// temporary directory (so ~/.ssh/authorized_keys is a real file); "passwd" is simulated
// over the PTY like Linux passwd for a normal user.
type fakeHost struct {
	nativeExec atomic.Bool // native OpenSSH/rsync keeps stdin open until command exit

	addr       string
	home       string
	user       string
	mu         sync.Mutex
	password   string
	history    []string // previous passwords (noReuse)
	failPasswd bool     // passwd fails after the prompts
	noReuse    bool     // passwd refuses passwords used before
	keyLogins  atomic.Int64
	pwLogins   atomic.Int64
	passwdRuns atomic.Int64
	shellDir   string // when set, "shell" runs a real sh on a PTY in this directory (Linux)
}

func (h *fakeHost) pw() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.password
}

func startFakeHost(t *testing.T, user, password string) *fakeHost {
	t.Helper()
	h := &fakeHost{user: user, password: password, home: t.TempDir()}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == h.user && string(pass) == h.pw() {
				h.pwLogins.Add(1)
				return nil, nil
			}
			return nil, fmt.Errorf("denied")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			data, _ := os.ReadFile(filepath.Join(h.home, ".ssh", "authorized_keys"))
			for len(data) > 0 {
				pub, _, _, rest, err := ssh.ParseAuthorizedKey(data)
				if err != nil {
					break
				}
				if c.User() == h.user && bytes.Equal(pub.Marshal(), key.Marshal()) {
					h.keyLogins.Add(1)
					return nil, nil
				}
				data = rest
			}
			return nil, fmt.Errorf("unknown key")
		},
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	h.addr = ln.Addr().String()
	// a new host key on a port an earlier test server may have used (see startTestSSHServer)
	if db != nil {
		db.Exec(`DELETE FROM known_hosts WHERE host=?`, normalizeHostKeyHost(h.addr))
	}
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for nch := range chans {
					if nch.ChannelType() != "session" {
						nch.Reject(ssh.UnknownChannelType, "no")
						continue
					}
					ch, creqs, err := nch.Accept()
					if err != nil {
						continue
					}
					go h.serveSession(ch, creqs)
				}
			}()
		}
	}()
	return h
}

func (h *fakeHost) serveSession(ch ssh.Channel, reqs <-chan *ssh.Request) {
	for req := range reqs {
		switch req.Type {
		case "pty-req", "env", "window-change":
			req.Reply(true, nil)
		case "shell":
			h.mu.Lock()
			dir := h.shellDir
			h.mu.Unlock()
			if dir == "" {
				req.Reply(false, nil)
				continue
			}
			req.Reply(true, nil)
			go h.runShell(ch, dir)
		case "subsystem":
			var p struct{ Name string }
			ssh.Unmarshal(req.Payload, &p)
			if p.Name != "sftp" {
				req.Reply(false, nil)
				continue
			}
			req.Reply(true, nil)
			go func() {
				if srv, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(h.home)); err == nil {
					srv.Serve()
				}
				ch.Close()
			}()
		case "exec":
			var p struct{ Cmd string }
			ssh.Unmarshal(req.Payload, &p)
			req.Reply(true, nil)
			go func() {
				code := 0
				if strings.Contains(p.Cmd, "passwd") {
					code = h.passwd(ch)
				} else {
					cmd := exec.Command("sh", "-c", p.Cmd)
					cmd.Env = []string{"HOME=" + h.home, "PATH=" + os.Getenv("PATH"), "USER=" + h.user}
					cmd.Stdin, cmd.Stdout, cmd.Stderr = ch, ch, ch.Stderr()
					if h.nativeExec.Load() {
						cmd.Stdin = nil
						input, err := cmd.StdinPipe()
						if err != nil {
							ch.Close()
							return
						}
						go func() { io.Copy(input, ch); input.Close() }()
					}
					if err := cmd.Run(); err != nil {
						code = 1
						if ee, ok := err.(*exec.ExitError); ok {
							code = ee.ExitCode()
						}
					}
				}
				ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(code)}))
				ch.Close()
			}()
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

// runShell runs an interactive sh on a pseudo terminal, like sshd does for a login shell.
func (h *fakeHost) runShell(ch ssh.Channel, dir string) {
	cmd := exec.Command("sh", "-i")
	cmd.Dir = dir
	cmd.Env = []string{"HOME=" + h.home, "PATH=" + os.Getenv("PATH"), "USER=" + h.user, "PS1=$ ", "ENV="}
	master, err := startInPTY(cmd, 100, 30)
	if err != nil {
		ch.Write([]byte(err.Error() + "\r\n"))
		ch.Close()
		return
	}
	go io.Copy(master, ch)
	io.Copy(ch, master)
	cmd.Process.Kill()
	cmd.Wait()
	master.Close()
	ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
	ch.Close()
}

func readLine(r io.Reader) (string, bool) {
	var b []byte
	one := make([]byte, 1)
	for {
		n, err := r.Read(one)
		if err != nil {
			return string(b), false
		}
		if n == 1 {
			if one[0] == '\n' || one[0] == '\r' {
				return string(b), true
			}
			b = append(b, one[0])
		}
	}
}

func (h *fakeHost) passwd(ch ssh.Channel) int {
	h.passwdRuns.Add(1)
	w := func(s string) { io.WriteString(ch, s) }
	w("Changing password for " + h.user + ".\r\nCurrent password: ")
	cur, ok := readLine(ch)
	if !ok || cur != h.pw() {
		w("\r\npasswd: Authentication token manipulation error\r\npasswd: password unchanged\r\n")
		return 10
	}
	var nw string
	for try := 0; ; try++ {
		if try == 3 {
			w("passwd: Have exhausted maximum number of retries for service\r\n")
			return 10
		}
		w("\r\nNew password: ")
		nw, _ = readLine(ch)
		h.mu.Lock()
		reused := h.noReuse && (nw == h.password || contains(h.history, nw))
		h.mu.Unlock()
		if len(nw) < 8 {
			w("\r\nBAD PASSWORD: The password is shorter than 8 characters")
			continue
		}
		if reused {
			w("\r\nPassword has been already used. Choose another.")
			continue
		}
		break
	}
	w("\r\nRetype new password: ")
	again, _ := readLine(ch)
	if again != nw {
		w("\r\nSorry, passwords do not match.\r\npasswd: Authentication token manipulation error\r\n")
		return 10
	}
	h.mu.Lock()
	fail := h.failPasswd
	h.mu.Unlock()
	if fail {
		w("\r\npasswd: Authentication token manipulation error\r\npasswd: password unchanged\r\n")
		return 10
	}
	h.mu.Lock()
	h.history = append(h.history, h.password)
	h.password = nw
	h.mu.Unlock()
	w("\r\npasswd: password updated successfully\r\n")
	return 0
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (h *fakeHost) authorizedKeys() string {
	b, _ := os.ReadFile(filepath.Join(h.home, ".ssh", "authorized_keys"))
	return string(b)
}

// ─── SSH key store ───────────────────────────────────

func TestSSHKeyStoreDeployRevoke(t *testing.T) {
	srv := newTestServer(t)
	user := newTestUser(t, srv, "keys-user", false)
	other := newTestUser(t, srv, "keys-other", false)
	h := startFakeHost(t, testSSHUser, testSSHPass)
	conn := user.addConnection("app-01", h.addr)
	// An entry that is not WRM's, with options: must survive every change.
	os.MkdirAll(filepath.Join(h.home, ".ssh"), 0o700)
	_, colleague, _ := ed25519.GenerateKey(rand.Reader)
	cs, _ := ssh.NewSignerFromKey(colleague)
	foreign := `from="10.0.0.0/8",no-agent-forwarding ` + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(cs.PublicKey()))) + " alice@laptop"
	os.WriteFile(filepath.Join(h.home, ".ssh", "authorized_keys"), []byte(foreign), 0o600) // no trailing new line

	// generate
	var k1 sshKey
	if code := user.jsonDo("POST", "/api/keys", map[string]interface{}{"name": "ops ed25519", "generate": map[string]interface{}{"type": "ed25519"}}, &k1); code != 200 || k1.ID == 0 || !k1.HasPrivate ||
		!strings.HasPrefix(k1.PublicKey, "ssh-ed25519 ") || !strings.HasPrefix(k1.Fingerprint, "SHA256:") || k1.Comment != "keys-user@wrm" {
		t.Fatalf("generate: %d %+v", code, k1)
	}
	var k2 sshKey
	if code := user.jsonDo("POST", "/api/keys", map[string]interface{}{"name": "rsa", "generate": map[string]interface{}{"type": "rsa", "bits": 2048}}, &k2); code != 200 || k2.Bits != 2048 || k2.KeyType != "ssh-rsa" {
		t.Fatalf("rsa: %d %+v", code, k2)
	}
	if code := user.jsonDo("POST", "/api/keys", map[string]interface{}{"generate": map[string]interface{}{"type": "rsa", "bits": 1024}}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("weak RSA accepted: %d", code)
	}
	// import with passphrase
	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	block, _ := ssh.MarshalPrivateKeyWithPassphrase(rk, "imported@host", []byte("open sesame"))
	pemText := string(pem.EncodeToMemory(block))
	var resp map[string]interface{}
	if code := user.jsonDo("POST", "/api/keys", map[string]interface{}{"name": "imp", "key": pemText}, &resp); code != 400 || resp["need_passphrase"] != true {
		t.Fatalf("missing passphrase: %d %v", code, resp)
	}
	if code := user.jsonDo("POST", "/api/keys", map[string]interface{}{"name": "imp", "key": pemText, "passphrase": "wrong"}, &resp); code != 400 || !strings.Contains(fmt.Sprint(resp["error"]), "passphrase") {
		t.Fatalf("wrong passphrase: %d %v", code, resp)
	}
	var k3 sshKey
	if code := user.jsonDo("POST", "/api/keys", map[string]interface{}{"name": "imp", "key": pemText, "passphrase": "open sesame"}, &k3); code != 200 || !k3.HasPrivate || k3.Comment != "imp" || !strings.HasSuffix(k3.PublicKey, " imp") {
		t.Fatalf("import: %d %+v", code, k3)
	}
	if code := user.jsonDo("POST", "/api/keys", map[string]interface{}{"name": "again", "key": pemText, "passphrase": "open sesame"}, &resp); code != 409 {
		t.Fatalf("duplicate import: %d", code)
	}
	// public key only (a colleague's key to deploy)
	_, bob, _ := ed25519.GenerateKey(rand.Reader)
	bs, _ := ssh.NewSignerFromKey(bob)
	var kPub sshKey
	if code := user.jsonDo("POST", "/api/keys", map[string]interface{}{"key": strings.TrimSpace(string(ssh.MarshalAuthorizedKey(bs.PublicKey()))) + " bob@desk"}, &kPub); code != 200 || kPub.HasPrivate || kPub.Name != "bob@desk" {
		t.Fatalf("public key: %d %+v", code, kPub)
	}
	// private keys are never listed; other users do not see the keys
	r := user.do("GET", "/api/keys", nil, "")
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if strings.Contains(string(body), "PRIVATE KEY") || strings.Contains(string(body), "private_key") {
		t.Fatalf("key list leaks private keys: %s", body)
	}
	var otherKeys []sshKey
	other.jsonDo("GET", "/api/keys", nil, &otherKeys)
	if len(otherKeys) != 0 {
		t.Fatalf("other user sees keys: %+v", otherKeys)
	}
	if code := other.jsonDo("POST", fmt.Sprintf("/api/keys/%d/deploy", k1.ID), map[string]interface{}{"conn_ids": []int{conn}}, &resp); code != 404 {
		t.Fatalf("other user deploys my key: %d", code)
	}
	// stored at rest encrypted
	var raw string
	db.QueryRow(`SELECT private_key FROM ssh_keys WHERE id=?`, k1.ID).Scan(&raw)
	if raw == "" || strings.Contains(raw, "PRIVATE KEY") {
		t.Fatalf("private key not encrypted at rest: %.40q", raw)
	}

	// deploy + switch the connection to the key
	type depResp struct {
		Results []keyHostResult `json:"results"`
		OK      int             `json:"ok"`
	}
	var dep depResp
	dep = depResp{}
	if code := user.jsonDo("POST", fmt.Sprintf("/api/keys/%d/deploy", k1.ID), map[string]interface{}{"conn_ids": []int{conn}, "use_key": true}, &dep); code != 200 || dep.OK != 1 || !dep.Results[0].Switched || dep.Results[0].Already {
		t.Fatalf("deploy: %d %+v", code, dep)
	}
	ak := h.authorizedKeys()
	if !strings.Contains(ak, foreign+"\n") || !strings.Contains(ak, k1.PublicKey+"\n") {
		t.Fatalf("authorized_keys after deploy:\n%s", ak)
	}
	if fi, _ := os.Stat(filepath.Join(h.home, ".ssh", "authorized_keys")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("authorized_keys mode %v", fi.Mode().Perm())
	}
	var am string
	var keyID *int
	db.QueryRow(`SELECT auth_method, key_id FROM connections WHERE id=?`, conn).Scan(&am, &keyID)
	if am != "KEY_REF" || keyID == nil || *keyID != k1.ID {
		t.Fatalf("connection not switched: %s %v", am, keyID)
	}
	before := h.keyLogins.Load()
	c, _ := loadConnection(conn)
	cl, err := dialSSH(c, nil)
	if err != nil {
		t.Fatalf("login with stored key: %v", err)
	}
	cl.Close()
	if h.keyLogins.Load() != before+1 {
		t.Fatalf("did not log in with the key")
	}
	var lastUsed string
	db.QueryRow(`SELECT last_used_at FROM ssh_keys WHERE id=?`, k1.ID).Scan(&lastUsed)
	if lastUsed == "" {
		t.Fatalf("last_used_at not set")
	}
	// deploying again does not duplicate; a public-only key can be deployed (logging in as before)
	dep = depResp{}
	if code := user.jsonDo("POST", fmt.Sprintf("/api/keys/%d/deploy", k1.ID), map[string]interface{}{"conn_ids": []int{conn}}, &dep); code != 200 || !dep.Results[0].Already {
		t.Fatalf("deploy again: %+v", dep)
	}
	dep = depResp{}
	if code := user.jsonDo("POST", fmt.Sprintf("/api/keys/%d/deploy", kPub.ID), map[string]interface{}{"conn_ids": []int{conn, 999999}, "use_key": true}, &dep); code != 200 || dep.OK != 1 ||
		!dep.Results[0].OK || dep.Results[0].Switched || dep.Results[1].OK {
		t.Fatalf("deploy public key: %+v", dep)
	}
	if strings.Count(h.authorizedKeys(), keyBlob(k1.PublicKey)) != 1 {
		t.Fatalf("key duplicated:\n%s", h.authorizedKeys())
	}

	// who has access
	var view struct {
		User     string            `json:"user"`
		Entries  []authorizedEntry `json:"entries"`
		LoginKey string            `json:"login_key"`
	}
	if code := user.jsonDo("GET", fmt.Sprintf("/api/connections/%d/authorized-keys", conn), nil, &view); code != 200 || len(view.Entries) != 3 || view.LoginKey != k1.Fingerprint {
		t.Fatalf("authorized keys: %d %+v", code, view)
	}
	byFP := map[string]authorizedEntry{}
	for _, e := range view.Entries {
		byFP[e.Fingerprint] = e
	}
	if e := byFP[k1.Fingerprint]; e.KeyID != k1.ID || !e.Current {
		t.Fatalf("own key entry: %+v", e)
	}
	if e := byFP[ssh.FingerprintSHA256(cs.PublicKey())]; e.KeyID != 0 || e.Comment != "alice@laptop" || !strings.Contains(e.Options, "no-agent-forwarding") {
		t.Fatalf("foreign entry: %+v", e)
	}
	if e := byFP[kPub.Fingerprint]; e.KeyName != "bob@desk" || e.Current {
		t.Fatalf("bob entry: %+v", e)
	}
	if code := other.jsonDo("GET", fmt.Sprintf("/api/connections/%d/authorized-keys", conn), nil, &view); code != 404 {
		t.Fatalf("other user reads authorized_keys: %d", code)
	}
	var keys []sshKey
	user.jsonDo("GET", "/api/keys", nil, &keys)
	for _, k := range keys {
		if k.ID == k1.ID && (len(k.Deployed) != 1 || len(k.UsedBy) != 1 || k.UsedBy[0] != "app-01") {
			t.Fatalf("key usage: %+v", k)
		}
	}

	// revoke: never the key WRM logs in with
	dep = depResp{}
	if code := user.jsonDo("POST", fmt.Sprintf("/api/keys/%d/revoke", k1.ID), map[string]interface{}{"conn_ids": []int{conn}}, &dep); code != 200 || dep.Results[0].OK || !strings.Contains(dep.Results[0].Error, "lock") {
		t.Fatalf("revoke login key: %+v", dep)
	}
	if code := user.jsonDo("DELETE", fmt.Sprintf("/api/connections/%d/authorized-keys?fingerprint=%s", conn, url.QueryEscape(k1.Fingerprint)), nil, &resp); code != 409 {
		t.Fatalf("remove login key entry: %d", code)
	}
	dep = depResp{}
	if code := user.jsonDo("POST", fmt.Sprintf("/api/keys/%d/revoke", kPub.ID), map[string]interface{}{"conn_ids": []int{conn}}, &dep); code != 200 || !dep.Results[0].OK || dep.Results[0].Removed != 1 {
		t.Fatalf("revoke: %+v", dep)
	}
	if code := user.jsonDo("DELETE", fmt.Sprintf("/api/connections/%d/authorized-keys?fingerprint=%s", conn, ssh.FingerprintSHA256(cs.PublicKey())), nil, &resp); code != 200 || resp["removed"] != float64(1) {
		t.Fatalf("remove foreign entry: %d %v", code, resp)
	}
	if ak := h.authorizedKeys(); ak != k1.PublicKey+"\n" {
		t.Fatalf("authorized_keys after revoke:\n%q", ak)
	}

	// delete: refused while in use
	if code := user.jsonDo("DELETE", fmt.Sprintf("/api/keys/%d", k1.ID), nil, &resp); code != 409 {
		t.Fatalf("delete key in use: %d", code)
	}
	if code := user.jsonDo("DELETE", fmt.Sprintf("/api/keys/%d", kPub.ID), nil, &resp); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	// export with re-authentication
	if code := user.jsonDo("POST", fmt.Sprintf("/api/keys/%d/export", k1.ID), map[string]string{"password": "nope"}, &resp); code != 403 {
		t.Fatalf("export without password: %d", code)
	}
	var exp map[string]string
	if code := user.jsonDo("POST", fmt.Sprintf("/api/keys/%d/export", k1.ID), map[string]string{"password": "pw-keys-user-123456", "passphrase": "pp-123"}, &exp); code != 200 {
		t.Fatalf("export: %d", code)
	}
	sg, err := ssh.ParsePrivateKeyWithPassphrase([]byte(exp["private_key"]), []byte("pp-123"))
	if err != nil || ssh.FingerprintSHA256(sg.PublicKey()) != k1.Fingerprint {
		t.Fatalf("exported key: %v", err)
	}
	r = user.do("GET", fmt.Sprintf("/api/keys/%d/public", k1.ID), nil, "")
	pubFile, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if string(pubFile) != k1.PublicKey+"\n" || !strings.Contains(r.Header.Get("Content-Disposition"), "ops_ed25519.pub") {
		t.Fatalf("public key download: %q %s", pubFile, r.Header.Get("Content-Disposition"))
	}
	// connection API: KEY_REF needs a key pair of my own
	if code := user.jsonDo("PUT", fmt.Sprintf("/api/connections/%d", conn), map[string]interface{}{"name": "app-01", "host": h.addr, "username": testSSHUser, "auth_method": "KEY_REF", "key_id": k3.ID}, &resp); code != 200 {
		t.Fatalf("switch key: %d %v", code, resp)
	}
	otherConn := other.addConnection("theirs", h.addr)
	if code := other.jsonDo("PUT", fmt.Sprintf("/api/connections/%d", otherConn), map[string]interface{}{"name": "theirs", "host": h.addr, "auth_method": "KEY_REF", "key_id": k1.ID}, &resp); code != 400 {
		t.Fatalf("using another user's key: %d", code)
	}
	for _, a := range []string{"ssh_key.created", "ssh_key.deployed", "ssh_key.revoked", "ssh_key.exported", "ssh_key.export_denied", "ssh_key.deleted"} {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action=? AND user_id=?`, a, user.userID).Scan(&n)
		if n == 0 {
			t.Errorf("no audit entry %s", a)
		}
	}
}

func TestParseAuthorizedKeysAndHostPatterns(t *testing.T) {
	_, k, _ := ed25519.GenerateKey(rand.Reader)
	s, _ := ssh.NewSignerFromKey(k)
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(s.PublicKey())))
	es := parseAuthorizedKeys("# comment\n\n" + line + " me@x\ngarbage here\ncommand=\"echo hi\" " + line + "\n")
	if len(es) != 3 || es[0].Comment != "me@x" || !es[1].Invalid || es[2].Options != `command="echo hi"` || es[2].Line != 5 {
		t.Fatalf("parse: %+v", es)
	}
	cases := []struct {
		pat, host string
		ok        bool
	}{
		{"", "anything:22", true},
		{"*.dc1.example.com", "db1.dc1.example.com:22", true},
		{"*.dc1.example.com", "db1.dc2.example.com", false},
		{"10.1.0.0/16, web-?", "10.1.4.7:2222", true},
		{"10.1.0.0/16, web-?", "web-1", true},
		{"10.1.0.0/16 web-?", "10.2.0.1", false},
		{"DB*", "db7.local", true},
		{"fd00::/8", "[fd00::1]:22", true},
	}
	for _, c := range cases {
		if got := hostAllowed(c.pat, c.host); got != c.ok {
			t.Errorf("hostAllowed(%q, %q) = %v", c.pat, c.host, got)
		}
	}
	if validHostPatterns("10.0.0.0/33") == nil || validHostPatterns("[a-") == nil || validHostPatterns("*.x, 10.0.0.0/8") != nil {
		t.Fatalf("pattern validation")
	}
	for i := 0; i < 50; i++ {
		p := generatePassword(24)
		if len(p) != 24 || !strings.ContainsAny(p, pwDigits) || !strings.ContainsAny(p, pwUpper) || !strings.ContainsAny(p, pwSymbols) {
			t.Fatalf("weak generated password %q", p)
		}
	}
}

// ─── credentials vault ───────────────────────────────

func waitRotation(t *testing.T, c *testClient, id int) map[string]interface{} {
	t.Helper()
	var st map[string]interface{}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		c.jsonDo("GET", fmt.Sprintf("/api/credentials/%d/rotation", id), nil, &st)
		if st["running"] == false && st["stage"] == "done" {
			return st
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("rotation did not finish: %v", st)
	return nil
}

func hostStates(st map[string]interface{}) string {
	var out []string
	hs, _ := st["hosts"].([]interface{})
	for _, x := range hs {
		m := x.(map[string]interface{})
		out = append(out, fmt.Sprint(m["state"]))
	}
	return strings.Join(out, ",")
}

func TestCredentialsVaultAndRotation(t *testing.T) {
	srv := newTestServer(t)
	owner := newTestUser(t, srv, "vault-owner", false)
	mate := newTestUser(t, srv, "vault-mate", false)
	stranger := newTestUser(t, srv, "vault-stranger", false)
	const oldPW = "Old-Pass-123"
	a := startFakeHost(t, "root", oldPW)
	b := startFakeHost(t, "root", oldPW)

	var cr credential
	if code := owner.jsonDo("POST", "/api/credentials", map[string]interface{}{"name": "root@dc1", "username": "root", "password": oldPW, "hosts": "127.0.0.1, *.dc1.example.com", "rotate_days": 90}, &cr); code != 200 || cr.ID == 0 || !cr.Mine || !cr.HasPassword {
		t.Fatalf("create: %d %+v", code, cr)
	}
	if code := owner.jsonDo("POST", "/api/credentials", map[string]interface{}{"name": "x", "username": "root"}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("credential without secret accepted: %d", code)
	}
	if code := owner.jsonDo("POST", "/api/credentials", map[string]interface{}{"name": "x", "password": "p", "shared_all": true}, &map[string]interface{}{}); code != 403 {
		t.Fatalf("non-admin shares with everybody: %d", code)
	}
	var stored string
	db.QueryRow(`SELECT password FROM credentials WHERE id=?`, cr.ID).Scan(&stored)
	if stored == "" || strings.Contains(stored, oldPW) {
		t.Fatalf("password not encrypted at rest")
	}
	// connections log in with it; the user name comes from the credential
	mk := func(cl *testClient, name, host string) (int, int) {
		var v connView
		code := cl.jsonDo("POST", "/api/connections", map[string]interface{}{"name": name, "host": host, "protocol": "SSH", "auth_method": "CREDENTIAL", "credential_id": cr.ID, "password": "ignored"}, &v)
		return code, v.ID
	}
	code, ca := mk(owner, "a-host", a.addr)
	_, cb := mk(owner, "b-host", b.addr)
	if code != 201 || ca == 0 || cb == 0 {
		t.Fatalf("connections: %d %d %d", code, ca, cb)
	}
	var v connView
	owner.jsonDo("GET", fmt.Sprintf("/api/connections/%d", ca), nil, &v)
	if v.AuthMethod != "CREDENTIAL" || v.CredName != "root@dc1" || v.CredUser != "root" || v.HasPassword {
		t.Fatalf("connection view: %+v", v)
	}
	if code, _ := mk(owner, "outside", "10.9.9.9:22"); code != 400 {
		t.Fatalf("host restriction not enforced: %d", code)
	}
	for _, id := range []int{ca, cb} {
		c, _ := loadConnection(id)
		if c.Username != "root" || c.AuthMethod != "PASSWORD" || c.Password != oldPW {
			t.Fatalf("resolved: %+v", c)
		}
		cl, err := dialSSH(c, nil)
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		cl.Close()
	}
	// sharing: only granted users see and use it, never the secret
	var list []credential
	mate.jsonDo("GET", "/api/credentials", nil, &list)
	if len(list) != 0 {
		t.Fatalf("not granted yet: %+v", list)
	}
	if code, _ := mk(mate, "mine", a.addr); code != 400 {
		t.Fatalf("ungranted use: %d", code)
	}
	if code := owner.jsonDo("PUT", fmt.Sprintf("/api/credentials/%d", cr.ID), map[string]interface{}{"name": "root@dc1", "username": "root", "hosts": cr.Hosts, "rotate_days": 90, "grants": []int{mate.userID}}, &cr); code != 200 || len(cr.Grants) != 1 {
		t.Fatalf("grant: %d %+v", code, cr)
	}
	r := mate.do("GET", "/api/credentials", nil, "")
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if strings.Contains(string(body), oldPW) || !strings.Contains(string(body), `"mine":false`) || strings.Contains(string(body), `"grants":[`+strconv.Itoa(mate.userID)) {
		t.Fatalf("grantee view: %s", body)
	}
	code, cm := mk(mate, "mate-a", a.addr)
	if code != 201 {
		t.Fatalf("grantee connection: %d", code)
	}
	// a jump host of their own could lead the login elsewhere: not with a shared credential
	evil := mate.addConnection("evil-jump", "10.66.0.1:22")
	var ej map[string]interface{}
	if code := mate.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "via-evil", "host": a.addr, "protocol": "SSH", "auth_method": "CREDENTIAL",
		"credential_id": cr.ID, "jump_id": evil}, &ej); code != 400 || !strings.Contains(fmt.Sprint(ej["error"]), "jump host") {
		t.Fatalf("grantee through an own jump host: %d %v", code, ej)
	}
	for _, try := range []struct{ method, path string }{{"PUT", ""}, {"DELETE", ""}, {"POST", "/reveal"}, {"POST", "/rotate"}} {
		if code := mate.jsonDo(try.method, fmt.Sprintf("/api/credentials/%d%s", cr.ID, try.path), map[string]interface{}{"name": "hijack", "password": "pw-vault-mate-123456"}, &map[string]interface{}{}); code != 403 {
			t.Fatalf("grantee %s %s: %d", try.method, try.path, code)
		}
	}
	if code := stranger.jsonDo("GET", fmt.Sprintf("/api/credentials/%d/usage", cr.ID), nil, &map[string]interface{}{}); code != 404 {
		t.Fatalf("stranger: %d", code)
	}
	// test connection with the credential (form, not saved)
	var tr map[string]interface{}
	mate.jsonDo("POST", "/api/connections/test", map[string]interface{}{"name": "t", "host": b.addr, "protocol": "SSH", "auth_method": "CREDENTIAL", "credential_id": cr.ID}, &tr)
	if tr["ok"] != true {
		t.Fatalf("test with credential: %v", tr)
	}
	// reveal: owner only, with the account password
	var rev map[string]string
	if code := owner.jsonDo("POST", fmt.Sprintf("/api/credentials/%d/reveal", cr.ID), map[string]string{"password": "bad"}, &rev); code != 403 {
		t.Fatalf("reveal without password: %d", code)
	}
	if code := owner.jsonDo("POST", fmt.Sprintf("/api/credentials/%d/reveal", cr.ID), map[string]string{"password": "pw-vault-owner-123456"}, &rev); code != 200 || rev["password"] != oldPW {
		t.Fatalf("reveal: %d %v", code, rev)
	}

	// check: logs in everywhere, changes nothing (a and the grantee's a are one server)
	var st map[string]interface{}
	if code := owner.jsonDo("POST", fmt.Sprintf("/api/credentials/%d/rotate", cr.ID), map[string]string{"mode": "check"}, &st); code != 200 {
		t.Fatalf("check: %d %v", code, st)
	}
	st = waitRotation(t, owner, cr.ID)
	if st["ok"] != true || hostStates(st) != "ok,ok" || a.passwdRuns.Load() != 0 {
		t.Fatalf("check result: %v", st)
	}
	// rotate
	if code := owner.jsonDo("POST", fmt.Sprintf("/api/credentials/%d/rotate", cr.ID), map[string]string{"mode": "rotate"}, &st); code != 200 {
		t.Fatalf("rotate: %d %v", code, st)
	}
	st = waitRotation(t, owner, cr.ID)
	if st["ok"] != true || hostStates(st) != "changed,changed" {
		t.Fatalf("rotation: %v", st)
	}
	owner.jsonDo("POST", fmt.Sprintf("/api/credentials/%d/reveal", cr.ID), map[string]string{"password": "pw-vault-owner-123456"}, &rev)
	newPW := rev["password"]
	if newPW == oldPW || len(newPW) != 24 || a.pw() != newPW || b.pw() != newPW {
		t.Fatalf("passwords after rotation: vault %q a %q b %q", newPW, a.pw(), b.pw())
	}
	for _, id := range []int{ca, cm} {
		c, _ := loadConnection(id)
		cl, err := dialSSH(c, nil)
		if err != nil {
			t.Fatalf("login after rotation: %v", err)
		}
		cl.Close()
	}
	owner.jsonDo("GET", "/api/credentials", nil, &list)
	if len(list) != 1 || list[0].RotatedAt == "" || list[0].RotationDue || !strings.HasPrefix(list[0].RotationStatus, "ok ") || list[0].UsedBy != 3 {
		t.Fatalf("after rotation: %+v", list)
	}

	// failure on b: a is changed back, nothing is stored
	b.set(func() { b.failPasswd = true })
	owner.jsonDo("POST", fmt.Sprintf("/api/credentials/%d/rotate", cr.ID), map[string]string{"mode": "rotate", "new_password": "Third-Pass-456"}, &st)
	st = waitRotation(t, owner, cr.ID)
	if st["ok"] != false || hostStates(st) != "rolled_back,failed" || a.pw() != newPW || b.pw() != newPW {
		t.Fatalf("rollback: %v a=%q b=%q", st, a.pw(), b.pw())
	}
	owner.jsonDo("POST", fmt.Sprintf("/api/credentials/%d/reveal", cr.ID), map[string]string{"password": "pw-vault-owner-123456"}, &rev)
	if rev["password"] != newPW || rev["pending_password"] != "" {
		t.Fatalf("vault changed after failed rotation: %v", rev)
	}
	// failure where changing back is refused too: the new password is kept as pending
	a.set(func() { a.noReuse = true })
	owner.jsonDo("POST", fmt.Sprintf("/api/credentials/%d/rotate", cr.ID), map[string]string{"mode": "rotate", "new_password": "Fourth-Pass-789"}, &st)
	st = waitRotation(t, owner, cr.ID)
	if st["ok"] != false || hostStates(st) != "rollback_failed,failed" || a.pw() != "Fourth-Pass-789" {
		t.Fatalf("incomplete: %v a=%q", st, a.pw())
	}
	owner.jsonDo("POST", fmt.Sprintf("/api/credentials/%d/reveal", cr.ID), map[string]string{"password": "pw-vault-owner-123456"}, &rev)
	if rev["password"] != newPW || rev["pending_password"] != "Fourth-Pass-789" {
		t.Fatalf("pending password: %v", rev)
	}
	if code := owner.jsonDo("POST", fmt.Sprintf("/api/credentials/%d/rotate", cr.ID), map[string]string{"mode": "rotate"}, &st); code != 409 {
		t.Fatalf("rotation over an incomplete one: %d", code)
	}
	// non-SSH connections block rotation
	db.Exec(`UPDATE connections SET protocol='RDP' WHERE id=?`, cm)
	owner.jsonDo("PUT", fmt.Sprintf("/api/credentials/%d", cr.ID), map[string]interface{}{"name": "root@dc1", "username": "root", "password": newPW, "hosts": cr.Hosts, "grants": []int{mate.userID}}, &cr)
	if cr.Incomplete {
		t.Fatalf("setting the password did not clear the pending one")
	}
	var e map[string]interface{}
	if code := owner.jsonDo("POST", fmt.Sprintf("/api/credentials/%d/rotate", cr.ID), map[string]string{"mode": "rotate"}, &e); code != 400 || !strings.Contains(fmt.Sprint(e["error"]), "RDP") {
		t.Fatalf("rotation with an RDP connection: %d %v", code, e)
	}
	// revoking the grant: the grantee's connection can no longer log in
	owner.jsonDo("PUT", fmt.Sprintf("/api/credentials/%d", cr.ID), map[string]interface{}{"name": "root@dc1", "username": "root", "hosts": cr.Hosts}, &cr)
	if c, _ := loadConnection(cm); c.authErr == "" || !strings.Contains(c.authErr, "no longer shared") {
		t.Fatalf("revoked grant still usable: %+v", c.authErr)
	}
	// delete: refused while used
	if code := owner.jsonDo("DELETE", fmt.Sprintf("/api/credentials/%d", cr.ID), nil, &e); code != 409 {
		t.Fatalf("delete in use: %d", code)
	}
	for _, act := range []string{"credential.created", "credential.granted", "credential.grant_revoked", "credential.revealed", "credential.checked", "credential.rotated", "credential.rotation_failed", "credential.rotation_incomplete"} {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action=?`, act).Scan(&n)
		if n == 0 {
			t.Errorf("no audit entry %s", act)
		}
	}
	var leaked int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE details LIKE ? OR details LIKE ?`, "%"+newPW+"%", "%Fourth-Pass%").Scan(&leaked)
	if leaked > 0 {
		t.Fatalf("password in the audit log")
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(newRouter())
	t.Cleanup(s.Close)
	return s
}

func (h *fakeHost) set(f func()) {
	h.mu.Lock()
	f()
	h.mu.Unlock()
}
