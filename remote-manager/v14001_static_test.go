// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"testing"
)

// TestStaticAssetsCompressedAndValidated covers UX-02: the embedded page and assets are
// served with an ETag (304 on reload) and gzip, instead of 1.2 MB of plain text each time.
func TestStaticAssetsCompressedAndValidated(t *testing.T) {
	srv := newTestServer(t)
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}

	get := func(path string, headers map[string]string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := get("/", map[string]string{"Accept-Encoding": "gzip"})
	if resp.StatusCode != 200 || resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("index: status %d, encoding %q", resp.StatusCode, resp.Header.Get("Content-Encoding"))
	}
	et := resp.Header.Get("ETag")
	if et == "" {
		t.Fatal("no ETag on the index")
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(zr)
	resp.Body.Close()
	if err != nil || !bytes.Contains(plain, []byte("Web Remote Manager")) {
		t.Fatalf("gzip body is not the app (err=%v, %d bytes)", err, len(plain))
	}

	resp = get("/", map[string]string{"If-None-Match": et})
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("reload with If-None-Match: %d, want 304", resp.StatusCode)
	}
	resp.Body.Close()

	resp = get("/", nil)
	if resp.Header.Get("Content-Encoding") != "" {
		t.Fatal("index compressed without Accept-Encoding")
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(raw, []byte("<html")) {
		t.Fatal("plain index body is not HTML")
	}

	resp = get("/static/git.js", map[string]string{"Accept-Encoding": "gzip"})
	if resp.StatusCode != 200 || resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("static js: status %d, encoding %q", resp.StatusCode, resp.Header.Get("Content-Encoding"))
	}
	resp.Body.Close()

	resp = get("/static/missing.js", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing asset: %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}
