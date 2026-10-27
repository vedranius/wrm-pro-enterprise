// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

// mrEncrypt produces mRemoteNG's current format: base64(salt | nonce | AES-256-GCM(ct+tag)),
// key = PBKDF2-HMAC-SHA1(password, salt, iterations), associated data = salt.
func mrEncrypt(t *testing.T, plain, password string) string {
	salt, nonce := make([]byte, 16), make([]byte, 16)
	rand.Read(salt)
	rand.Read(nonce)
	key := pbkdf2.Key([]byte(password), salt, 1000, 32, sha1.New)
	block, _ := aes.NewCipher(key)
	aead, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil {
		t.Fatal(err)
	}
	out := append(append(append([]byte{}, salt...), nonce...), aead.Seal(nil, nonce, []byte(plain), salt)...)
	return base64.StdEncoding.EncodeToString(out)
}

// mrEncryptCBC produces the old format: base64(iv | AES-CBC(MD5(password), PKCS7)).
func mrEncryptCBC(plain, password string) string {
	key := md5.Sum([]byte(password))
	block, _ := aes.NewCipher(key[:])
	pad := 16 - len(plain)%16
	pt := append([]byte(plain), []byte(strings.Repeat(string(rune(pad)), pad))...)
	iv := make([]byte, 16)
	rand.Read(iv)
	ct := make([]byte, len(pt))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, pt)
	return base64.StdEncoding.EncodeToString(append(iv, ct...))
}

func mrFile(t *testing.T, master string) string {
	enc := func(s string) string { return mrEncrypt(t, s, master) }
	protected := "ThisIsNotProtected"
	if master != "mR3m" {
		protected = "ThisIsProtected"
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<mrng:Connections xmlns:mrng="http://mremoteng.org" Name="Connections" Export="false" EncryptionEngine="AES" BlockCipherMode="GCM" KdfIterations="1000" FullFileEncryption="false" Protected="` + enc(protected) + `" ConfVersion="2.6">
  <Node Name="DC1" Type="Container" Expanded="true" Username="root" Password="` + enc("secret1") + `">
    <Node Name="bastion" Type="Connection" Protocol="SSH2" Hostname="203.0.113.10" Port="22" InheritUsername="true" InheritPassword="true" />
    <Node Name="Rack A" Type="Container" Username="ops" Password="">
      <Node Name="web01" Type="Connection" Protocol="SSH2" Hostname="10.0.0.5" Port="2222" Username="admin" Password="` + enc("pw2-č") + `" SSHTunnelConnectionName="bastion" />
      <Node Name="idrac" Type="Connection" Protocol="HTTPS" Hostname="https://10.0.0.9/console" Port="443" SSHTunnelConnectionName="bastion" />
      <Node Name="win01" Type="Connection" Protocol="RDP" Hostname="10.0.0.20" Port="3389" />
    </Node>
  </Node>
  <Node Name="standalone" Type="Connection" Protocol="SSH2" Hostname="example.org" Port="22" Username="me" Password="" />
</mrng:Connections>`
}

func connByName(t *testing.T, userID int, name string) Connection {
	t.Helper()
	for _, c := range loadUserConnections(userID) {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("connection %q not imported", name)
	return Connection{}
}

func folderName(id *int) string {
	if id == nil {
		return ""
	}
	var n string
	db.QueryRow(`SELECT name FROM folders WHERE id=?`, *id).Scan(&n)
	return n
}

func TestImportMRemoteNG(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "mr-user", false)
	var res importResult
	if code := u.jsonDo("POST", "/api/config/import/mremoteng", map[string]string{"xml": mrFile(t, "mR3m")}, &res); code != 200 {
		t.Fatalf("import: %d %+v", code, res)
	}
	if res.Imported != 5 || res.Jumps != 2 || len(res.Skipped) != 0 || res.ByProto["RDP"] != 1 {
		t.Fatalf("result: %+v", res)
	}
	win := connByName(t, u.userID, "win01")
	if win.Protocol != "RDP" || win.Host != "10.0.0.20" {
		t.Fatalf("RDP connection: %+v", win)
	}
	if o := loadDesktopOptions(win.ID); o["ignore_cert"] != "true" {
		t.Fatalf("RDP options: %v", o)
	}
	b := connByName(t, u.userID, "bastion")
	if b.Username != "root" || b.Password != "secret1" || b.Host != "203.0.113.10" || folderName(b.FolderID) != "DC1" {
		t.Fatalf("bastion (inherited credentials): %+v folder %q", b, folderName(b.FolderID))
	}
	w := connByName(t, u.userID, "web01")
	if w.Host != "10.0.0.5:2222" || w.Username != "admin" || w.Password != "pw2-č" || w.JumpID == nil || *w.JumpID != b.ID || folderPathOf(u.userID, w.FolderID) != "DC1 / Rack A" {
		t.Fatalf("web01: %+v", w)
	}
	i := connByName(t, u.userID, "idrac")
	if i.Protocol != "HTTPS" || i.Host != "10.0.0.9" || i.WebPath != "/console" || i.JumpID == nil || *i.JumpID != b.ID {
		t.Fatalf("idrac: %+v", i)
	}
	// Importing the same file again does not create duplicates.
	var again importResult
	u.jsonDo("POST", "/api/config/import/mremoteng", map[string]string{"xml": mrFile(t, "mR3m")}, &again)
	if again.Imported != 0 {
		t.Fatalf("duplicates imported: %+v", again)
	}

	// A custom master password is needed, and checked.
	u2 := newTestUser(t, srv, "mr-user2", false)
	file := mrFile(t, "Corp-Master-1")
	var e map[string]interface{}
	if code := u2.jsonDo("POST", "/api/config/import/mremoteng", map[string]string{"xml": file}, &e); code != 400 || e["need_password"] != true {
		t.Fatalf("no password: %d %v", code, e)
	}
	if code := u2.jsonDo("POST", "/api/config/import/mremoteng", map[string]string{"xml": file, "password": "wrong"}, &e); code != 400 {
		t.Fatalf("wrong password accepted: %d", code)
	}
	if code := u2.jsonDo("POST", "/api/config/import/mremoteng", map[string]string{"xml": file, "password": "Corp-Master-1"}, &res); code != 200 || res.Imported != 5 {
		t.Fatalf("right password: %d %+v", code, res)
	}
	if connByName(t, u2.userID, "web01").Password != "pw2-č" {
		t.Fatalf("password with custom master password not decrypted")
	}

	// Old mRemoteNG files (AES-CBC, no EncryptionEngine attribute).
	u3 := newTestUser(t, srv, "mr-user3", false)
	old := `<?xml version="1.0"?><Connections Name="Connections" Protected="` + mrEncryptCBC("ThisIsNotProtected", "mR3m") + `" ConfVersion="2.4">
<Node Name="old-ssh" Type="Connection" Protocol="SSH2" Hostname="10.1.1.1" Port="22" Username="u" Password="` + mrEncryptCBC("legacy-pw", "mR3m") + `" /></Connections>`
	if code := u3.jsonDo("POST", "/api/config/import/mremoteng", map[string]string{"xml": old}, &res); code != 200 || res.Imported != 1 {
		t.Fatalf("legacy import: %d %+v", code, res)
	}
	if connByName(t, u3.userID, "old-ssh").Password != "legacy-pw" {
		t.Fatalf("legacy password not decrypted")
	}

	// Not a mRemoteNG file
	if code := u3.jsonDo("POST", "/api/config/import/mremoteng", map[string]string{"xml": "<html></html>"}, &e); code != 400 {
		t.Fatalf("bad file accepted: %d", code)
	}
}

func TestImportSSHConfig(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "sshcfg-user", false)
	cfg := `
# global defaults
Host *
  User defaultuser
  ServerAliveInterval 30

Host bastion jump
  HostName 203.0.113.10
  Port 2222
  User jumper

Host app
  HostName 10.0.0.5
  ProxyJump bastion
  LocalForward 8443 localhost:443
  LocalForward 127.0.0.1:5433 db.internal:5432
  DynamicForward 1080
  RemoteForward 9000 localhost:3000

Host legacy
  HostName=10.0.0.6
  ProxyCommand ssh -W %h:%p bastion

Host viaimplicit
  HostName 10.0.0.7
  ProxyJump ops@gw.example.com:2200

Match host foo
  User nobody

Host *.internal
  User internal
`
	var res importResult
	if code := u.jsonDo("POST", "/api/config/import/sshconfig", map[string]string{"text": cfg, "folder": "Laptop"}, &res); code != 200 {
		t.Fatalf("import: %d %+v", code, res)
	}
	if res.Imported != 5 || res.Jumps != 3 || res.Tunnels != 3 {
		t.Fatalf("result: %+v", res)
	}
	b := connByName(t, u.userID, "bastion")
	if b.Host != "203.0.113.10:2222" || b.Username != "jumper" || folderName(b.FolderID) != "Laptop" {
		t.Fatalf("bastion: %+v", b)
	}
	a := connByName(t, u.userID, "app")
	if a.Username != "defaultuser" || a.JumpID == nil || *a.JumpID != b.ID {
		t.Fatalf("app: %+v", a)
	}
	defs := loadTunnelDefs("conn_id=?", a.ID)
	if len(defs) != 3 || defs[0].BindPort != 8443 || defs[0].TargetHost != "localhost" || defs[0].OpenScheme != "https" ||
		defs[1].TargetHost != "db.internal" || defs[2].Kind != "dynamic" || defs[2].BindPort != 1080 || defs[0].StartMode != "connect" {
		t.Fatalf("app tunnels: %+v", defs)
	}
	var remoteNote bool
	for _, n := range res.Notes {
		if n.Name == "app" && strings.Contains(n.Reason, "remote port forwarding") {
			remoteNote = true
		}
	}
	if !remoteNote {
		t.Fatalf("RemoteForward refused by the policy should be reported: %+v", res.Notes)
	}
	if l := connByName(t, u.userID, "legacy"); l.JumpID == nil || *l.JumpID != b.ID || l.Host != "10.0.0.6" {
		t.Fatalf("legacy: %+v", l)
	}
	imp := connByName(t, u.userID, "ops@gw.example.com:2200")
	if imp.Host != "gw.example.com:2200" || imp.Username != "ops" {
		t.Fatalf("implicit jump host: %+v", imp)
	}
	if v := connByName(t, u.userID, "viaimplicit"); v.JumpID == nil || *v.JumpID != imp.ID {
		t.Fatalf("viaimplicit: %+v", v)
	}
}
