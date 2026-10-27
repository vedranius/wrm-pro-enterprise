// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// Fuzz tests for the parsers that read files users upload or paste. The seeds run with every
// `go test`; the nightly workflow (.github/workflows/nightly.yml) fuzzes each target for a
// while. A parser may reject any input, but it must not panic or hang.
//
//	go test -run '^$' -fuzz '^FuzzParseTableFile$' -fuzztime 60s .

var fuzzUser struct {
	once sync.Once
	id   int
}

// fuzzUserID is an account the importers write their connections to.
func fuzzUserID(f *testing.F) int {
	fuzzUser.once.Do(func() {
		h, _ := hashPassword("pw-fuzz-importer-123456")
		res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?,?,0,?)`,
			fmt.Sprintf("fuzz-importer-%d", time.Now().UnixNano()), h, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			f.Fatal(err)
		}
		id, _ := res.LastInsertId()
		fuzzUser.id = int(id)
	})
	return fuzzUser.id
}

func FuzzImportMRemoteNG(f *testing.F) {
	uid := fuzzUserID(f)
	f.Add([]byte(`<?xml version="1.0" encoding="utf-8"?><Connections Name="Connections" Export="false" EncryptionEngine="AES" BlockCipherMode="GCM" KdfIterations="1000" ConfVersion="2.6"><Node Name="demo-api" Type="Connection" Hostname="demo-api.example.com" Protocol="SSH2" Port="22" Username="deploy" Password="" /><Node Name="Team" Type="Container"><Node Name="demo-web" Type="Connection" Hostname="10.0.0.5" Protocol="RDP" Port="3389" /></Node></Connections>`), "")
	f.Add([]byte(`<Connections Name="x"><Node Type="Connection" Hostname="h" Password="AAAA" /></Connections>`), "secret")
	f.Add([]byte(`<Root/>`), "")
	f.Fuzz(func(t *testing.T, data []byte, password string) {
		if len(data) > 64<<10 || strings.Contains(string(data), "KdfIterations") && len(data) > 4<<10 {
			return // key derivation is deliberately slow; long inputs with it only slow the fuzzer
		}
		importMRemoteNG(uid, data, password)
	})
}

func FuzzImportSSHConfig(f *testing.F) {
	uid := fuzzUserID(f)
	f.Add("Host demo-api\n  HostName demo-api.example.com\n  User deploy\n  Port 2222\n  IdentityFile ~/.ssh/id_ed25519\n  ProxyJump bastion\n\nHost bastion\n  HostName 192.0.2.10\n")
	f.Add("Match host *.example.com\n  User x\nHost *\n  ServerAliveInterval 30\n")
	f.Add("Host a b c\nHostName=10.0.0.1\nPort=abc\n")
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 64<<10 {
			return
		}
		importSSHConfig(uid, text, "")
	})
}

func FuzzParseTableFile(f *testing.F) {
	f.Add("hosts.csv", []byte("name,host,port,user,tags\ndemo-api,demo-api.example.com,22,deploy,\"web,prod\"\ndemo-web,10.0.0.5,,root,\n"), "")
	f.Add("hosts.csv", []byte("name;host\n\"a;b\";c\n"), "")
	f.Add("hosts.xlsx", []byte("PK\x03\x04"), "Sheet1")
	f.Add("hosts.txt", []byte("\xef\xbb\xbfname\thost\nx\ty\n"), "")
	f.Fuzz(func(t *testing.T, name string, data []byte, sheet string) {
		if len(data) > 256<<10 {
			return
		}
		parseTableFile(name, data, sheet)
	})
}

func FuzzParsePuttyReg(f *testing.F) {
	f.Add("Windows Registry Editor Version 5.00\r\n\r\n[HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\demo-api]\r\n\"HostName\"=\"demo-api.example.com\"\r\n\"PortNumber\"=dword:00000016\r\n\"UserName\"=\"deploy\"\r\n\"Protocol\"=\"ssh\"\r\n")
	f.Add("[HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\a%20b]\n\"PortNumber\"=dword:zz\n")
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 256<<10 {
			return
		}
		parsePuttyReg(text)
	})
}

func FuzzParseWinSCPBookmarks(f *testing.F) {
	f.Add("[Configuration\\Bookmarks\\Local]\nRemoteDirectory=/var/www\n[Sessions\\deploy@demo-api]\nHostName=demo-api.example.com\nRemoteDirectory=/opt/demo-api\n")
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 256<<10 {
			return
		}
		parseWinSCPBookmarks(text)
	})
}

func FuzzParseGitConfig(f *testing.F) {
	f.Add("config/app.ini", []byte("[database]\nhost = db.example.com\nport=5432\n; comment\n[cache]\nenabled=true\n"))
	f.Add("config/app.yaml", []byte("database:\n  host: db.example.com\n  ports: [1, 2]\n"))
	f.Add("config/app.json", []byte(`{"database":{"host":"db.example.com","port":5432}}`))
	f.Add("config/.env", []byte("DB_HOST=db.example.com\nexport DB_PORT=5432\n"))
	f.Fuzz(func(t *testing.T, rel string, data []byte) {
		if len(data) > 256<<10 {
			return
		}
		parseGitConfig(rel, data)
	})
}

func FuzzMediumConfigYAML(f *testing.F) {
	f.Add([]byte("name: demo-api\nservers:\n  - prod-1\nsource: ./build\ntarget: /opt/demo-api\nexclude:\n  - '*.log'\npost_deploy:\n  - systemctl restart demo-api\n"))
	f.Add([]byte("a: &x [*x]\n"))
	f.Add([]byte("- - - - -\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			return
		}
		mdParseYAML(data, "projects/demo-api.yaml")
	})
}
