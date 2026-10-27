// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func previewRequest(t *testing.T, u *testClient, method, endpoint, byteRange string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, u.srv.URL+endpoint, nil)
	req.AddCookie(u.cookie)
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return res, b
}

func TestRemotePreviewRangesTextAndPermissions(t *testing.T) {
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "preview-owner", false)
	id := u.addConnection("media", h.addr)
	contents := strings.Repeat("0123456789", 100000)
	writeFile(t, h.home+"/clip.mp4", contents)
	endpoint := fmt.Sprintf("/api/remote/preview?id=%d&path=%s", id, url.QueryEscape(h.home+"/clip.mp4"))
	for _, tc := range []struct {
		method, r string
		code      int
		body, cr  string
	}{{"GET", "bytes=100-109", 206, "0123456789", "bytes 100-109/1000000"}, {"GET", "bytes=-5", 206, "56789", "bytes 999995-999999/1000000"}, {"GET", "bytes=999995-", 206, "56789", "bytes 999995-999999/1000000"}, {"HEAD", "", 200, "", ""}, {"GET", "bytes=1000000-", 416, "", "bytes */1000000"}} {
		res, b := previewRequest(t, u, tc.method, endpoint, tc.r)
		if res.StatusCode != tc.code || (tc.code < 400 && string(b) != tc.body) || res.Header.Get("Content-Range") != tc.cr {
			t.Fatalf("%s %s: %d %q %v", tc.method, tc.r, res.StatusCode, b, res.Header)
		}
		if tc.code < 400 && (res.Header.Get("Content-Type") != "video/mp4" || res.Header.Get("Accept-Ranges") != "bytes") {
			t.Fatalf("headers: %v", res.Header)
		}
	}
	writeFile(t, h.home+"/page.html", "<script>window.top.pwned=true</script>"+strings.Repeat("x", remotePreviewTextMax))
	textEndpoint := fmt.Sprintf("/api/remote/preview?id=%d&path=%s", id, url.QueryEscape(h.home+"/page.html"))
	res, b := previewRequest(t, u, "GET", textEndpoint+"&text=1", "")
	var text remotePreviewInfo
	json.Unmarshal(b, &text)
	if res.StatusCode != 200 || !text.Truncated || len(text.Text) > remotePreviewTextMax || !strings.Contains(text.Text, "<script>") {
		t.Fatalf("text: %d %+v", res.StatusCode, text)
	}
	res, _ = previewRequest(t, u, "GET", textEndpoint, "")
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/plain") || res.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(res.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("active document can execute: %v", res.Header)
	}
	other := newTestUser(t, srv, "preview-other", false)
	res, _ = previewRequest(t, other, "GET", endpoint, "")
	if res.StatusCode != 403 && res.StatusCode != 404 {
		t.Fatalf("foreign preview: %d", res.StatusCode)
	}
	sh := mustShare(t, u.userID, "members", RoleViewer, "")
	db.Exec(`INSERT INTO share_items(share_id,connection_id) VALUES(?,?)`, sh.ID, id)
	db.Exec(`INSERT INTO share_members(share_id,user_id,role) VALUES(?,?,?)`, sh.ID, other.userID, RoleViewer)
	shared := endpoint + "&share_token=" + sh.Token
	res, b = previewRequest(t, other, "GET", shared, "bytes=0-9")
	if res.StatusCode != 206 || string(b) != "0123456789" {
		t.Fatalf("viewer preview: %d %s", res.StatusCode, b)
	}
	db.Exec(`UPDATE share_members SET role=? WHERE share_id=? AND user_id=?`, RoleObserver, sh.ID, other.userID)
	res, _ = previewRequest(t, other, "GET", shared, "")
	if res.StatusCode != 403 {
		t.Fatalf("observer preview: %d", res.StatusCode)
	}
	writeFile(t, h.home+"/unknown.exe", "data")
	res, _ = previewRequest(t, u, "GET", fmt.Sprintf("/api/remote/preview?id=%d&path=%s", id, url.QueryEscape(h.home+"/unknown.exe")), "")
	if res.StatusCode != 415 {
		t.Fatal(res.StatusCode)
	}
}

func TestRemotePreviewContentDetection(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		text bool
	}{{"utf8", []byte("hello\n"), true}, {"empty", nil, true}, {"nul", []byte{0, 1, 2}, false},
		{"png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}, false},
		{"utf16", []byte{0xFF, 0xFE, 'h', 0}, true}, {"utf16le", []byte{'h', 0, 'i', 0}, true},
		{"ansi", []byte("\x1b[31mred\x1b[0m\n"), true},
		{"compressed", []byte{0x1F, 0x8B, 8, 0}, false}} {
		if got := remotePreviewIsText(tc.data); got != tc.text {
			t.Fatalf("%s: %v; want %v", tc.name, got, tc.text)
		}
	}
	if got := remotePreviewDecodeText([]byte{'h', 0, 'i', 0}); got != "hi" {
		t.Fatalf("UTF-16LE without BOM: %q", got)
	}
	for _, tc := range []struct {
		name string
		data []byte
		lang string
	}{{"template.smil", []byte("<smil><body/></smil>"), "markup"}, {"notes.custom", []byte(`{"a":1}`), "json"},
		{"run", []byte("#!/bin/sh\necho hi\n"), "bash"}, {"app.py", []byte("print(1)\n"), "python"},
		{"conf", []byte("[main]\nkey=value\n"), "ini"}} {
		if got := remotePreviewLanguage(tc.name, tc.data); got != tc.lang {
			t.Fatalf("%s language: %q; want %q", tc.name, got, tc.lang)
		}
	}
}

func TestRemotePreviewDetectsTextAndLanguages(t *testing.T) {
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "preview-text", false)
	id := u.addConnection("files", h.addr)
	cases := []struct{ name, content, lang string }{
		{"template.smil", `<smil><body><seq>Hi</seq></body></smil>`, "markup"},
		{"notes.custom", `{"enabled":true}`, "json"},
		{"run-me", "#!/usr/bin/env bash\necho hi\n", "bash"},
		{"app.py", "print('hi')\n", "python"},
		{"plain.dat", "name=value\n", ""},
	}
	var info remotePreviewInfo
	for _, tc := range cases {
		writeFile(t, h.home+"/"+tc.name, tc.content)
		endpoint := fmt.Sprintf("/api/remote/preview?id=%d&path=%s&metadata=1", id, url.QueryEscape(h.home+"/"+tc.name))
		res, b := previewRequest(t, u, "GET", endpoint, "")
		info = remotePreviewInfo{}
		json.Unmarshal(b, &info)
		if res.StatusCode != 200 || info.Kind != "text" || info.Lang != tc.lang {
			t.Fatalf("%s: %d %+v", tc.name, res.StatusCode, info)
		}
		res, b = previewRequest(t, u, "GET", strings.TrimSuffix(endpoint, "&metadata=1")+"&text=1", "")
		info = remotePreviewInfo{}
		json.Unmarshal(b, &info)
		if res.StatusCode != 200 || info.Text != tc.content {
			t.Fatalf("%s text: %d %+v", tc.name, res.StatusCode, info)
		}
	}
	os.WriteFile(h.home+"/blob.custom", []byte{0, 1, 2, 3}, 0o644)
	res, _ := previewRequest(t, u, "GET", fmt.Sprintf("/api/remote/preview?id=%d&path=%s&metadata=1", id, url.QueryEscape(h.home+"/blob.custom")), "")
	if res.StatusCode != 415 {
		t.Fatalf("binary candidate: %d", res.StatusCode)
	}
	os.WriteFile(h.home+"/utf16.dat", []byte{0xFF, 0xFE, 'h', 0, 'i', 0}, 0o644)
	res, b := previewRequest(t, u, "GET", fmt.Sprintf("/api/remote/preview?id=%d&path=%s&text=1", id, url.QueryEscape(h.home+"/utf16.dat")), "")
	info = remotePreviewInfo{}
	json.Unmarshal(b, &info)
	if res.StatusCode != 200 || info.Text != "hi" {
		t.Fatalf("UTF-16 text: %d %+v", res.StatusCode, info)
	}
}

// Minimal FTP endpoint that records REST offsets and serves real data connections.
func startPreviewFTP(t *testing.T, data string) (string, func() []int64) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	offsets := []int64{}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				rd := bufio.NewReader(c)
				say := func(s string) { fmt.Fprint(c, s+"\r\n") }
				say("220 preview FTP")
				var dl net.Listener
				var offset int64
				for {
					line, err := rd.ReadString('\n')
					if err != nil {
						return
					}
					parts := strings.SplitN(strings.TrimSpace(line), " ", 2)
					cmd := strings.ToUpper(parts[0])
					arg := ""
					if len(parts) > 1 {
						arg = parts[1]
					}
					switch cmd {
					case "USER":
						say("331 password")
					case "PASS":
						say("230 logged in")
					case "FEAT":
						say("211 End")
					case "TYPE":
						say("200 binary")
					case "SIZE":
						say(fmt.Sprintf("213 %d", len(data)))
					case "REST":
						offset, _ = strconv.ParseInt(arg, 10, 64)
						say("350 restart")
					case "EPSV":
						dl, _ = net.Listen("tcp", "127.0.0.1:0")
						say(fmt.Sprintf("229 (|||%d|)", dl.Addr().(*net.TCPAddr).Port))
					case "RETR":
						say("150 data")
						dc, err := dl.Accept()
						if err == nil {
							mu.Lock()
							offsets = append(offsets, offset)
							mu.Unlock()
							io.WriteString(dc, data[offset:])
							dc.Close()
						}
						dl.Close()
						offset = 0
						say("226 complete")
					case "QUIT":
						say("221 bye")
						return
					default:
						say("200 ok")
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String(), func() []int64 { mu.Lock(); defer mu.Unlock(); return append([]int64{}, offsets...) }
}
func TestRemotePreviewFTPRanges(t *testing.T) {
	addr, offsets := startPreviewFTP(t, "01234567890123456789")
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "preview-ftp", false)
	var c connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "ftp-media", "protocol": "FTP", "host": addr, "username": "demo", "password": "demo", "auth_method": "PASSWORD"}, &c); code != 201 {
		t.Fatal(code)
	}
	endpoint := fmt.Sprintf("/api/remote/preview?id=%d&path=/clip.mp3", c.ID)
	res, b := previewRequest(t, u, "GET", endpoint, "bytes=10-14")
	if res.StatusCode != 206 || string(b) != "01234" {
		t.Fatalf("FTP range: %d %s", res.StatusCode, b)
	}
	got := offsets()
	if len(got) != 1 || got[0] != 10 {
		t.Fatalf("FTP discarded prefix instead of seeking: %v", got)
	}
	textEndpoint := fmt.Sprintf("/api/remote/preview?id=%d&path=/notes.txt&text=1", c.ID)
	res, b = previewRequest(t, u, "GET", textEndpoint, "")
	var info remotePreviewInfo
	json.Unmarshal(b, &info)
	if res.StatusCode != 200 || info.Kind != "text" || info.Text != "01234567890123456789" {
		t.Fatalf("FTP text preview: %d %+v", res.StatusCode, info)
	}
}

func TestRemotePreviewNoSpecialFiles(t *testing.T) {
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "preview-directory", false)
	id := u.addConnection("files", h.addr)
	os.Mkdir(h.home+"/directory.mp4", 0700)
	res, _ := previewRequest(t, u, "GET", fmt.Sprintf("/api/remote/preview?id=%d&path=%s", id, url.QueryEscape(h.home+"/directory.mp4")), "")
	if res.StatusCode != 502 {
		t.Fatal(res.StatusCode)
	}
}

func TestRemotePreviewPDFNativeViewerPolicy(t *testing.T) {
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "preview-pdf", false)
	id := u.addConnection("documents", h.addr)
	writeFile(t, h.home+"/document.pdf", "%PDF-1.4\n")
	res, _ := previewRequest(t, u, "GET", fmt.Sprintf("/api/remote/preview?id=%d&path=%s", id, url.QueryEscape(h.home+"/document.pdf")), "")
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/pdf" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("PDF headers: %d %v", res.StatusCode, res.Header)
	}
	policy := res.Header.Get("Content-Security-Policy")
	if strings.Contains(policy, "sandbox") || !strings.Contains(policy, "default-src 'none'") {
		t.Fatalf("PDF native viewer is blocked or unrestricted: %s", policy)
	}
}
