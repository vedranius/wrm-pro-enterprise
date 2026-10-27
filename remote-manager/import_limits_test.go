// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimitForImports(t *testing.T) {
	for _, p := range []string{"/api/config/import", "/api/config/import/mremoteng", "/api/config/import/sshconfig",
		"/api/inventory/parse", "/api/inventory/import", "/api/inventory/netbox/import"} {
		if got := bodyLimitFor(p); got != maxImportBody {
			t.Errorf("bodyLimitFor(%q) = %d, want %d", p, got, maxImportBody)
		}
	}
	if got := bodyLimitFor("/api/connections"); got != 1<<20 {
		t.Errorf("default limit = %d", got)
	}
	if got := bodyLimitFor("/api/config/importer"); got != 1<<20 {
		t.Errorf("/api/config/importer must keep the default limit, got %d", got)
	}
	// The largest allowed file still fits into the body after base64 encoding.
	if base64.StdEncoding.EncodedLen(maxImportFile)+4096 > maxImportBody {
		t.Fatal("maxImportBody too small for a base64-encoded maxImportFile")
	}
}

// bigMRemoteNG builds a confCons.xml of at least size bytes: n SSH connections with long descriptions.
func bigMRemoteNG(size int) (string, int) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<mrng:Connections xmlns:mrng="http://mremoteng.org" Name="Connections" Export="false" EncryptionEngine="AES" BlockCipherMode="GCM" KdfIterations="1000" FullFileEncryption="false" ConfVersion="2.6">
  <Node Name="Big DC" Type="Container" Expanded="true">
`)
	pad := strings.Repeat("rack row power &quot;notes&quot; ", 60)
	n := 0
	for b.Len() < size {
		n++
		fmt.Fprintf(&b, `    <Node Name="demo-web-%05d" Type="Connection" Protocol="SSH2" Hostname="10.%d.%d.%d" Port="22" Username="ops" Password="" Descr="%s" />
`, n, n/65536%256, n/256%256, n%256, pad)
	}
	b.WriteString("  </Node>\n</mrng:Connections>\n")
	return b.String(), n
}

func TestLargeImportsThroughMiddleware(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "big-import", false)
	// Thousands of connections in the shared test database would slow down later tests
	// (the status monitor probes every monitored connection): remove them afterwards.
	t.Cleanup(func() {
		db.Exec(`DELETE FROM connections WHERE user_id=?`, u.userID)
		db.Exec(`DELETE FROM folders WHERE user_id=?`, u.userID)
	})

	// A multi-MB confCons.xml: above the old 1 MB middleware limit.
	xml, n := bigMRemoteNG(4 << 20)
	var res importResult
	if code := u.jsonDo("POST", "/api/config/import/mremoteng", map[string]string{"xml": xml}, &res); code != 200 || res.Imported != n {
		t.Fatalf("mRemoteNG import of %d bytes: %d imported %d of %d", len(xml), code, res.Imported, n)
	}

	// A large CSV inventory (several MB once base64-encoded).
	var csv strings.Builder
	csv.WriteString("Hostname,IP,Site,Comment\n")
	rows := 0
	for csv.Len() < 3<<20 {
		rows++
		fmt.Fprintf(&csv, "demo-api-%05d,10.50.%d.%d,zg1,%s\n", rows, rows/256%256, rows%256, strings.Repeat("x", 400))
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(csv.String()))
	var pr struct {
		Total   int            `json:"total"`
		Mapping map[string]int `json:"mapping"`
	}
	if code := u.jsonDo("POST", "/api/inventory/parse", map[string]interface{}{"file_name": "big.csv", "data": b64}, &pr); code != 200 || pr.Total != rows {
		t.Fatalf("parse large CSV: %d total %d, want %d", code, pr.Total, rows)
	}
	var ir importResult
	body := map[string]interface{}{"file_name": "big.csv", "data": b64, "mapping": pr.Mapping, "options": map[string]interface{}{"folder_mode": "none"}}
	if code := u.jsonDo("POST", "/api/inventory/import", body, &ir); code != 200 || ir.Imported != rows {
		t.Fatalf("import large CSV: %d imported %d of %d", code, ir.Imported, rows)
	}
}

func TestOversizedImportGets413(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "too-big", false)
	wantMsg := func(t *testing.T, path string, code int, data []byte) {
		t.Helper()
		var e struct {
			Error string `json:"error"`
		}
		json.Unmarshal(data, &e)
		if code != 413 || !strings.Contains(e.Error, "too large") || !strings.Contains(e.Error, "20.0 MB") {
			t.Fatalf("%s: %d %s", path, code, data)
		}
	}
	big := bytes.Repeat([]byte("a"), maxImportBody+1024)
	for _, path := range []string{"/api/config/import/mremoteng", "/api/inventory/parse", "/api/config/import"} {
		// With Content-Length: rejected before the body is read, the size is in the message.
		body := append(append([]byte(`{"xml":"`), big...), '"', '}')
		resp := u.do("POST", path, bytes.NewReader(body), "application/json")
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		wantMsg(t, path, resp.StatusCode, data)
		if !strings.Contains(string(data), "32.0 MB") {
			t.Fatalf("%s: size missing in %s", path, data)
		}
		// Chunked (no Content-Length): the middleware's MaxBytesReader stops it.
		resp = u.do("POST", path, io.MultiReader(strings.NewReader(`{"data":"`), bytes.NewReader(big), strings.NewReader(`"}`)), "application/json")
		data, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		wantMsg(t, path, resp.StatusCode, data)
	}

	// A file above maxImportFile whose request body still fits: the exact file size is reported.
	xml := strings.Repeat("x", maxImportFile+(1<<20))
	var e struct {
		Error string `json:"error"`
	}
	if code := u.jsonDo("POST", "/api/config/import/mremoteng", map[string]string{"xml": xml}, &e); code != 413 || !strings.Contains(e.Error, "21.0 MB") {
		t.Fatalf("file over limit: %d %q", code, e.Error)
	}
	raw := bytes.Repeat([]byte("h,1.2.3.4\n"), (maxImportFile+(1<<20))/10)
	if code := u.jsonDo("POST", "/api/inventory/parse", map[string]string{"file_name": "x.csv", "data": base64.StdEncoding.EncodeToString(raw)}, &e); code != 413 || !strings.Contains(e.Error, "21.0 MB") {
		t.Fatalf("inventory file over limit: %d %q", code, e.Error)
	}
}

// The browser checks the same file limit before sending (IMPORT_MAX_BYTES in index.html).
func TestImportLimitMatchesUI(t *testing.T) {
	html, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("const IMPORT_MAX_BYTES = %d * 1024 * 1024;", maxImportFile>>20)
	if !strings.Contains(string(html), want) {
		t.Fatalf("index.html must contain %q", want)
	}
}
