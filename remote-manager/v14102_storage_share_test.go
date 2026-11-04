// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// TestStorageShareToken covers STG-10: a share link opens one prefix read-only, expires, is
// revoked on demand and never reaches outside its prefix.
func TestStorageShareToken(t *testing.T) {
	u, stub, id := storageTestEnv(t, "storage-share", nil)
	stub.put("team-a/pub/one.txt", []byte("one"))
	stub.put("team-a/pub/two.txt", []byte("two"))
	stub.put("team-a/private/secret.txt", []byte("secret"))
	base := "/api/storages/" + strconv.Itoa(id) + "/shares"

	var created storageShare
	if code := u.jsonDo("POST", base, map[string]interface{}{"prefix": "pub", "hours": 2}, &created); code != http.StatusOK {
		t.Fatalf("create share: %d", code)
	}
	if !strings.HasPrefix(created.URL, "/s3/") || created.Prefix != "pub" {
		t.Fatalf("share answer: %+v", created)
	}

	// The page lists the share prefix only.
	page, err := http.Get(u.srv.URL + created.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if page.StatusCode != http.StatusOK {
		t.Fatalf("share page: %d", page.StatusCode)
	}
	if !strings.Contains(string(body), "one.txt") || strings.Contains(string(body), "secret.txt") {
		t.Fatalf("share page content: %s", body)
	}
	if strings.Contains(string(body), "<script") {
		t.Fatal("the share page renders a script")
	}

	// A direct download works, and a key outside the share prefix does not.
	dl, err := http.Get(u.srv.URL + created.URL + "/object?key=one.txt")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(dl.Body)
	dl.Body.Close()
	if dl.StatusCode != http.StatusOK || string(raw) != "one" {
		t.Fatalf("download: %d %q", dl.StatusCode, raw)
	}
	outside, err := http.Get(u.srv.URL + created.URL + "/object?key=../private/secret.txt")
	if err != nil {
		t.Fatal(err)
	}
	outside.Body.Close()
	if outside.StatusCode != http.StatusBadRequest {
		t.Fatalf("traversal through a share: %d", outside.StatusCode)
	}
	// Even a well-formed key that exists outside the prefix is not reachable.
	outside2, err := http.Get(u.srv.URL + created.URL + "/object?key=" + url.QueryEscape("private/secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	outside2.Body.Close()
	if outside2.StatusCode == http.StatusOK {
		t.Fatal("a key outside the share prefix was served")
	}

	// Revoking ends the link.
	if code := u.jsonDo("DELETE", base+"/"+strconv.Itoa(created.ID), nil, nil); code != http.StatusOK {
		t.Fatalf("revoke: %d", code)
	}
	gone, err := http.Get(u.srv.URL + created.URL)
	if err != nil {
		t.Fatal(err)
	}
	gone.Body.Close()
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked link: %d", gone.StatusCode)
	}

	// An expired link is a 404 as well.
	var second storageShare
	if code := u.jsonDo("POST", base, map[string]interface{}{"prefix": "", "hours": 1}, &second); code != http.StatusOK {
		t.Fatalf("second share: %d", code)
	}
	db.Exec(`UPDATE storage_shares SET expires_at='2000-01-01T00:00:00Z' WHERE id=?`, second.ID)
	expired, err := http.Get(u.srv.URL + second.URL)
	if err != nil {
		t.Fatal(err)
	}
	expired.Body.Close()
	if expired.StatusCode != http.StatusNotFound {
		t.Fatalf("expired link: %d", expired.StatusCode)
	}

	// A share is read-only: the write routes are not part of it.
	put, err := http.NewRequest("PUT", u.srv.URL+second.URL+"/object?key=x.txt", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("a share accepted a write")
	}
}
