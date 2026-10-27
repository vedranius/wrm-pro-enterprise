// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf16"
)

const puttyReg = `Windows Registry Editor Version 5.00

[HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions]

[HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions\Default%20Settings]
"UserName"="ops"
"ProxyMethod"=dword:00000000
"ProxyDNS"=dword:00000001

[HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions\demo%20api]
"HostName"="10.0.0.5"
"PortNumber"=dword:00000016
"Protocol"="ssh"
"PublicKeyFile"="C:\\Users\\me\\keys\\demo.ppk"
"ProxyMethod"=dword:00000002
"ProxyHost"="proxy.example.com"
"ProxyPort"=dword:00000438
"ProxyUsername"="pu"
"ProxyPassword"="pp-\"x\""
"ProxyDNS"=dword:00000002

[HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions\demo-web]
"HostName"="admin@demo-web.example.com"
"PortNumber"=dword:000008ae
"Protocol"="ssh"
"ProxyMethod"=dword:00000002
"ProxyHost"="proxy.example.com"
"ProxyPort"=dword:00000438
"ProxyUsername"="pu"
"ProxyPassword"="pp-\"x\""
"ProxyDNS"=dword:00000002

[HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions\switch]
"HostName"="192.0.2.7"
"PortNumber"=dword:00000017
"Protocol"="telnet"
"ProxyMethod"=dword:00000003
"ProxyHost"="10.9.9.9"
"ProxyPort"=dword:00000c38
"ProxyDNS"=dword:00000000

[HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions\behind]
"HostName"="10.0.0.6"
"Protocol"="ssh"
"ProxyMethod"=dword:00000006
"ProxyHost"="demo api"

[HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions\console]
"Protocol"="serial"
"SerialLine"="COM3"

[HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\SshHostKeys]
"ssh-ed25519@22:10.0.0.5"="0x1234"
`

// utf16Reg encodes a .reg file the way regedit writes it: UTF-16 LE with a BOM, CRLF.
func utf16Reg(s string) []byte {
	u := utf16.Encode([]rune(strings.ReplaceAll(s, "\n", "\r\n")))
	out := []byte{0xFF, 0xFE}
	for _, c := range u {
		out = append(out, byte(c), byte(c>>8))
	}
	return out
}

func TestImportPuTTY(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "putty-user", false)
	var res importResult
	body := map[string]string{"data": base64.StdEncoding.EncodeToString(utf16Reg(puttyReg))}
	if code := u.jsonDo("POST", "/api/config/import/putty", body, &res); code != 200 {
		t.Fatalf("import: %d %+v", code, res)
	}
	if res.Imported != 4 || res.Proxies != 2 || res.Jumps != 1 || len(res.Skipped) != 1 || res.ByProto["TELNET"] != 1 {
		t.Fatalf("result: %+v", res)
	}
	api := connByName(t, u.userID, "demo api")
	if api.Protocol != "SSH" || api.Host != "10.0.0.5" || api.Username != "ops" || folderName(api.FolderID) != "PuTTY" || api.ProxyID == nil {
		t.Fatalf("demo api (defaults, URL-decoded name): %+v", api)
	}
	p, _ := loadProxy(*api.ProxyID)
	if p.Kind != "socks5" || p.Host != "proxy.example.com" || p.Port != 1080 || p.Username != "pu" || decryptValue(p.password) != `pp-"x"` || !p.RemoteDNS || p.OwnerID != u.userID {
		t.Fatalf("proxy: %+v", p)
	}
	web := connByName(t, u.userID, "demo-web")
	if web.Host != "demo-web.example.com:2222" || web.Username != "admin" || web.ProxyID == nil || *web.ProxyID != *api.ProxyID {
		t.Fatalf("demo-web (user@host, shared proxy): %+v", web)
	}
	sw := connByName(t, u.userID, "switch")
	if sw.Protocol != "TELNET" || sw.Host != "192.0.2.7" || sw.ProxyID == nil {
		t.Fatalf("switch: %+v", sw)
	}
	if hp, _ := loadProxy(*sw.ProxyID); hp.Kind != "http" || hp.Port != 3128 || hp.RemoteDNS {
		t.Fatalf("http proxy: %+v", hp)
	}
	if b := connByName(t, u.userID, "behind"); b.JumpID == nil || *b.JumpID != api.ID || b.ProxyID != nil {
		t.Fatalf("SSH proxy → jump host: %+v", b)
	}
	notes := ""
	for _, n := range res.Notes {
		notes += n.Name + ": " + n.Reason + "\n"
	}
	if !strings.Contains(notes, `demo.ppk`) || !strings.Contains(res.Skipped[0].Reason, "COM3") {
		t.Fatalf("notes %q skipped %+v", notes, res.Skipped)
	}

	// Again (UTF-8 this time): no duplicate connections or proxies.
	var again importResult
	u.jsonDo("POST", "/api/config/import/putty", map[string]string{"data": base64.StdEncoding.EncodeToString([]byte(puttyReg))}, &again)
	if again.Imported != 0 || again.Proxies != 0 {
		t.Fatalf("re-import: %+v", again)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM proxies WHERE owner_id=?`, u.userID).Scan(&n)
	if n != 2 {
		t.Fatalf("%d proxies", n)
	}

	// Not a .reg file
	var e map[string]interface{}
	if code := u.jsonDo("POST", "/api/config/import/putty", map[string]string{"data": base64.StdEncoding.EncodeToString([]byte("hello"))}, &e); code != 400 {
		t.Fatalf("bad file accepted: %d", code)
	}
}

// mRemoteNG connections that name a PuTTY session get its proxy, in either import order.
func TestImportPuTTYWithMRemoteNG(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	mr := `<?xml version="1.0" encoding="utf-8"?>
<mrng:Connections xmlns:mrng="http://mremoteng.org" Name="Connections" EncryptionEngine="AES" BlockCipherMode="GCM" KdfIterations="1000" FullFileEncryption="false" ConfVersion="2.6">
  <Node Name="Lab" Type="Container" PuttySession="demo api">
    <Node Name="mr-a" Type="Connection" Protocol="SSH2" Hostname="10.1.0.1" Port="22" Username="x" InheritPuttySession="true" />
  </Node>
  <Node Name="mr-b" Type="Connection" Protocol="SSH2" Hostname="10.1.0.2" Port="22" Username="x" PuttySession="Default Settings" />
  <Node Name="mr-c" Type="Connection" Protocol="SSH2" Hostname="10.1.0.3" Port="22" Username="x" PuttySession="unknown" />
</mrng:Connections>`
	reg := base64.StdEncoding.EncodeToString(utf16Reg(puttyReg))
	for _, order := range []string{"putty-first", "mremoteng-first"} {
		u := newTestUser(t, srv, "pm-"+order, false)
		steps := []string{"putty", "mremoteng"}
		if order == "mremoteng-first" {
			steps = []string{"mremoteng", "putty"}
		}
		linked := 0
		for _, s := range steps {
			var res importResult
			body := map[string]string{"data": reg}
			if s == "mremoteng" {
				body = map[string]string{"xml": mr}
			}
			if code := u.jsonDo("POST", "/api/config/import/"+s, body, &res); code != 200 {
				t.Fatalf("%s: %s import %d", order, s, code)
			}
			linked += res.Linked
		}
		api := connByName(t, u.userID, "demo api")
		a := connByName(t, u.userID, "mr-a")
		if a.ProxyID == nil || *a.ProxyID != *api.ProxyID || linked != 1 {
			t.Fatalf("%s: mr-a proxy %v, want %v (linked %d)", order, a.ProxyID, *api.ProxyID, linked)
		}
		for _, name := range []string{"mr-b", "mr-c"} {
			if c := connByName(t, u.userID, name); c.ProxyID != nil {
				t.Fatalf("%s: %s got a proxy", order, name)
			}
		}
	}
}

func TestParsePuttyReg(t *testing.T) {
	if got := decodeRegFile(utf16Reg("ab€")); got != "ab€" {
		t.Fatalf("utf-16: %q", got)
	}
	if got := decodeRegFile([]byte("\xEF\xBB\xBFab")); got != "ab" {
		t.Fatalf("utf-8 bom: %q", got)
	}
	s, err := parsePuttyReg(puttyReg)
	if err != nil || len(s) != 6 || s[1].name != "demo api" || s[1].vals["proxyport"] != "1080" || s[1].vals["publickeyfile"] != `C:\Users\me\keys\demo.ppk` {
		t.Fatalf("%v %+v", err, s)
	}
	if _, err := parsePuttyReg("Windows Registry Editor Version 5.00\n\n[HKEY_CURRENT_USER\\Software\\Other]\n"); err == nil {
		t.Fatal("no sessions accepted")
	}
}
