// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"net/url"
	"strings"
	"testing"
)

func testAddonZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestAddonArchiveTotalLimit covers SEC-10: the sum of uncompressed entries is bounded,
// so 500 small files cannot fill the disk even though each one is below 8 MB.
func TestAddonArchiveTotalLimit(t *testing.T) {
	old := addonMaxTotal
	addonMaxTotal = 1000
	defer func() { addonMaxTotal = old }()

	body := testAddonZip(t, map[string][]byte{
		"addon.json": []byte(`{"apiVersion":1,"id":"demo","name":"Demo","version":"1.0.0"}`),
		"a.bin":      bytes.Repeat([]byte("a"), 600),
		"b.bin":      bytes.Repeat([]byte("b"), 600),
	})
	if _, _, err := addonReadArchive(body); err == nil || !strings.Contains(err.Error(), "uncompressed") {
		t.Fatalf("oversized archive: err=%v", err)
	}
}

// TestAddonHTTPRejectsEncodedTraversal covers the %2e%2e bypass of http.paths.
func TestAddonHTTPRejectsEncodedTraversal(t *testing.T) {
	rec := addonRecord{HTTP: &addonHTTP{Origins: []string{"https://example.com"}, Paths: []string{"/api/*"}}}
	grants := map[string]addonHTTPGrant{"https://example.com": {}}

	bad, _ := url.Parse("https://example.com/api/%2e%2e/admin")
	if _, err := addonHTTPCheckURL(rec, grants, bad); err == nil {
		t.Fatal("percent-encoded parent traversal was accepted")
	}
	bad, _ = url.Parse("https://example.com/api/%2E%2E/%2E%2E/etc")
	if _, err := addonHTTPCheckURL(rec, grants, bad); err == nil {
		t.Fatal("uppercase percent-encoded traversal was accepted")
	}
	good, _ := url.Parse("https://example.com/api/status")
	if _, err := addonHTTPCheckURL(rec, grants, good); err != nil {
		t.Fatalf("valid path refused: %v", err)
	}
}
