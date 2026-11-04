// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// storageTestEnv starts the app and an S3 stub and returns a signed-in user with one storage
// pointing at the stub.
func storageTestEnv(t *testing.T, name string, tweak func(body map[string]interface{})) (*testClient, *s3Stub, int) {
	t.Helper()
	if _, err := setSetting("storages_enabled", "1"); err != nil {
		t.Fatal(err)
	}
	stub := newS3Stub(t, "wrm-test-bucket", "AKIASTORAGE", "secret-storage-key")
	s3srv := httptest.NewServer(stub)
	t.Cleanup(s3srv.Close)
	app := httptest.NewServer(newRouter())
	t.Cleanup(app.Close)
	u := newTestUser(t, app, name, false)
	body := map[string]interface{}{
		"name": "hetzner", "endpoint": s3srv.URL, "region": "us-east-1", "bucket": "wrm-test-bucket",
		"prefix": "team-a", "path_style": true, "access_key": "AKIASTORAGE", "secret": "secret-storage-key",
		"allow_write": true, "allow_delete": false, "allow_upload": false,
	}
	if tweak != nil {
		tweak(body)
	}
	var created Storage
	payload, _ := json.Marshal(body)
	resp := u.do("POST", "/api/storages", bytes.NewReader(payload), "application/json")
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create storage: %d %s", resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("create storage answer: %v (%s)", err, raw)
	}
	return u, stub, created.ID
}

func TestStorageCRUDAndSecretMasking(t *testing.T) {
	u, _, id := storageTestEnv(t, "storage-crud", nil)

	var list []Storage
	if code := u.jsonDo("GET", "/api/storages", nil, &list); code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list: %d %+v", code, list)
	}
	if !list[0].SecretSet {
		t.Error("secret_set is false after create")
	}
	raw, _ := json.Marshal(list[0])
	if strings.Contains(string(raw), "secret-storage-key") {
		t.Fatalf("the secret key is in the API answer: %s", raw)
	}

	// An update without a secret keeps the stored one and still passes the preflight.
	var updated Storage
	if code := u.jsonDo("PUT", "/api/storages/"+strconv.Itoa(id), map[string]interface{}{
		"name": "hetzner-2", "endpoint": list[0].Endpoint, "region": "us-east-1", "bucket": "wrm-test-bucket",
		"prefix": "team-a", "path_style": true, "access_key": "AKIASTORAGE",
		"allow_write": true, "allow_delete": true, "allow_upload": true,
	}, &updated); code != http.StatusOK {
		t.Fatalf("update: %d", code)
	}
	if updated.Name != "hetzner-2" || !updated.AllowDelete || !updated.SecretSet {
		t.Fatalf("update result: %+v", updated)
	}

	if code := u.jsonDo("DELETE", "/api/storages/"+strconv.Itoa(id), nil, nil); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}
	if code := u.jsonDo("GET", "/api/storages/"+strconv.Itoa(id), nil, nil); code != http.StatusNotFound {
		t.Fatalf("deleted storage still readable: %d", code)
	}
}

func TestStorageValidationRefusesTraversal(t *testing.T) {
	u, _, _ := storageTestEnv(t, "storage-validate", nil)
	cases := []map[string]interface{}{
		{"prefix": "a/../b"},
		{"prefix": "../x"},
		{"bucket": "Bad_Bucket"},
		{"endpoint": "http://169.254.169.254"},
		{"name": ""},
	}
	for _, patch := range cases {
		body := map[string]interface{}{
			"name": "bad", "endpoint": "http://127.0.0.1:1", "region": "us-east-1", "bucket": "wrm-test-bucket",
			"access_key": "k", "secret": "s", "path_style": true,
		}
		for k, v := range patch {
			body[k] = v
		}
		if code := u.jsonDo("POST", "/api/storages", body, nil); code != http.StatusBadRequest {
			t.Errorf("%v accepted: %d", patch, code)
		}
	}
}

func TestStorageKeysStayInsideThePrefix(t *testing.T) {
	u, _, id := storageTestEnv(t, "storage-keys", func(b map[string]interface{}) { b["allow_write"] = true })
	base := "/api/storages/" + strconv.Itoa(id)
	for _, key := range []string{"../other", "/etc/passwd", "a/../../b", ""} {
		if code := u.jsonDo("GET", base+"/text?key="+url.QueryEscape(key), nil, nil); code != http.StatusBadRequest {
			t.Errorf("key %q accepted: %d", key, code)
		}
	}
}

func TestStorageCapabilities(t *testing.T) {
	u, stub, id := storageTestEnv(t, "storage-caps", func(b map[string]interface{}) {
		b["allow_write"], b["allow_delete"], b["allow_upload"] = false, false, false
	})
	stub.put("team-a/existing.json", []byte(`{"a":1}`))
	base := "/api/storages/" + strconv.Itoa(id)

	if code := u.jsonDo("PUT", base+"/text?key=existing.json", map[string]interface{}{"content": "{}"}, nil); code != http.StatusForbidden {
		t.Fatalf("write on a read-only storage: %d", code)
	}
	req, _ := http.NewRequest("PUT", u.srv.URL+base+"/object?key=new.json", strings.NewReader("{}"))
	req.AddCookie(u.cookie)
	req.Header.Set("X-WRM-Request", "1")
	req.Header.Set("Origin", u.srv.URL)
	if resp, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("upload on a storage without allow_upload: %d", resp.StatusCode)
		}
	}
	req, _ = http.NewRequest("DELETE", u.srv.URL+base+"/object?key=existing.json", nil)
	req.AddCookie(u.cookie)
	req.Header.Set("X-WRM-Request", "1")
	req.Header.Set("Origin", u.srv.URL)
	req.Header.Set("X-WRM-Confirm", "delete")
	if resp, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("delete on a storage without allow_delete: %d", resp.StatusCode)
		}
	}
}

func TestStorageDeleteNeedsConfirmation(t *testing.T) {
	u, stub, id := storageTestEnv(t, "storage-delete", func(b map[string]interface{}) { b["allow_delete"] = true })
	stub.put("team-a/gone.txt", []byte("bye"))
	base := "/api/storages/" + strconv.Itoa(id)

	req, _ := http.NewRequest("DELETE", u.srv.URL+base+"/object?key=gone.txt", nil)
	req.AddCookie(u.cookie)
	req.Header.Set("X-WRM-Request", "1")
	req.Header.Set("Origin", u.srv.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("delete without confirmation: %d", resp.StatusCode)
	}

	req, _ = http.NewRequest("DELETE", u.srv.URL+base+"/object?key=gone.txt", nil)
	req.AddCookie(u.cookie)
	req.Header.Set("X-WRM-Request", "1")
	req.Header.Set("Origin", u.srv.URL)
	req.Header.Set("X-WRM-Confirm", "delete")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirmed delete: %d", resp.StatusCode)
	}
	stub.mu.Lock()
	_, still := stub.objects["team-a/gone.txt"]
	stub.mu.Unlock()
	if still {
		t.Fatal("the object survived a confirmed delete")
	}
}

func TestStorageTextKeepsBOMAndLineEndings(t *testing.T) {
	u, stub, id := storageTestEnv(t, "storage-text", func(b map[string]interface{}) { b["allow_write"] = true })
	original := append([]byte{0xEF, 0xBB, 0xBF}, []byte("{\r\n  \"a\": 1\r\n}\r\n")...)
	stub.put("team-a/config.json", original)
	base := "/api/storages/" + strconv.Itoa(id)

	var got struct {
		Content string `json:"content"`
		ETag    string `json:"etag"`
		BOM     bool   `json:"bom"`
		CRLF    bool   `json:"crlf"`
		Binary  bool   `json:"binary"`
	}
	if code := u.jsonDo("GET", base+"/text?key=config.json", nil, &got); code != http.StatusOK {
		t.Fatalf("read text: %d", code)
	}
	if !got.BOM || !got.CRLF || got.Binary || strings.Contains(got.Content, "\r") {
		t.Fatalf("text flags: bom=%v crlf=%v binary=%v content=%q", got.BOM, got.CRLF, got.Binary, got.Content)
	}

	if code := u.jsonDo("PUT", base+"/text?key=config.json", map[string]interface{}{
		"content": "{\n  \"a\": 2\n}\n", "etag": got.ETag, "bom": true, "crlf": true,
	}, nil); code != http.StatusOK {
		t.Fatalf("save text: %d", code)
	}
	stub.mu.Lock()
	saved := stub.objects["team-a/config.json"]
	stub.mu.Unlock()
	if !bytes.HasPrefix(saved, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("the BOM was lost: %q", saved)
	}
	if !bytes.Contains(saved, []byte("\r\n")) || bytes.Contains(bytes.ReplaceAll(saved, []byte("\r\n"), nil), []byte("\n")) {
		t.Fatalf("the line endings were not preserved: %q", saved)
	}

	// The second save with the old etag must not overwrite the new content silently.
	if code := u.jsonDo("PUT", base+"/text?key=config.json", map[string]interface{}{
		"content": "{\n  \"a\": 3\n}\n", "etag": got.ETag,
	}, nil); code != http.StatusConflict {
		t.Fatalf("stale etag: %d, want 409", code)
	}
}

func TestStorageDeleteRequiresStepUp(t *testing.T) {
	u, stub, id := storageTestEnv(t, "storage-stepup", func(b map[string]interface{}) { b["allow_delete"] = true })
	stub.put("team-a/x.txt", []byte("x"))
	restore := func() { setSetting("require_step_up", "0") }
	t.Cleanup(restore)
	if _, err := setSetting("require_step_up", "1"); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE auth_sessions SET step_up_at='2000-01-01T00:00:00Z' WHERE user_id=?`, u.userID)

	req, _ := http.NewRequest("DELETE", u.srv.URL+"/api/storages/"+strconv.Itoa(id)+"/object?key=x.txt", nil)
	req.AddCookie(u.cookie)
	req.Header.Set("X-WRM-Request", "1")
	req.Header.Set("Origin", u.srv.URL)
	req.Header.Set("X-WRM-Confirm", "delete")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(body), `"step_up":true`) {
		t.Fatalf("stale confirmation: %d %s", resp.StatusCode, body)
	}
	db.Exec(`UPDATE auth_sessions SET step_up_at=? WHERE user_id=?`, nowRFC(), u.userID)
	req, _ = http.NewRequest("DELETE", u.srv.URL+"/api/storages/"+strconv.Itoa(id)+"/object?key=x.txt", nil)
	req.AddCookie(u.cookie)
	req.Header.Set("X-WRM-Request", "1")
	req.Header.Set("Origin", u.srv.URL)
	req.Header.Set("X-WRM-Confirm", "delete")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fresh confirmation: %d", resp.StatusCode)
	}
}

func TestStorageExportRedactsSecretsAndImport(t *testing.T) {
	u, _, _ := storageTestEnv(t, "storage-export", nil)

	req, _ := http.NewRequest("GET", u.srv.URL+"/api/config/export", nil)
	req.AddCookie(u.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export: %d", resp.StatusCode)
	}
	if strings.Contains(string(raw), "secret-storage-key") {
		t.Fatal("the export contains the secret key")
	}
	if !strings.Contains(string(raw), `"storages"`) {
		t.Fatal("the export has no storages section")
	}

	// An import does not preflight: the endpoint may be unreachable while restoring.
	payload := map[string]interface{}{
		"folders": []interface{}{}, "connections": []interface{}{},
		"storages": []map[string]interface{}{{
			"name": "imported", "endpoint": "http://127.0.0.1:1", "region": "us-east-1",
			"bucket": "imported-bucket", "access_key": "AK", "secret": "SK", "path_style": true,
		}},
	}
	var res map[string]int
	if code := u.jsonDo("POST", "/api/config/import", payload, &res); code != http.StatusOK {
		t.Fatalf("import: %d", code)
	}
	if res["storages"] != 1 {
		t.Fatalf("import result: %+v", res)
	}
	// A second import skips the duplicate instead of failing.
	if code := u.jsonDo("POST", "/api/config/import", payload, &res); code != http.StatusOK || res["storages_skipped"] != 1 {
		t.Fatalf("second import: %d %+v", code, res)
	}
}

func TestStorageObjectRangeAndListing(t *testing.T) {
	u, stub, id := storageTestEnv(t, "storage-range", nil)
	payload := []byte("0123456789abcdef")
	stub.put("team-a/movie.mp4", payload)
	stub.put("team-a/sub/nested.srt", []byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"))
	base := "/api/storages/" + strconv.Itoa(id)

	var list struct {
		Prefixes []string  `json:"prefixes"`
		Objects  []s3Entry `json:"objects"`
	}
	if code := u.jsonDo("GET", base+"/objects", nil, &list); code != http.StatusOK {
		t.Fatalf("list: %d", code)
	}
	if len(list.Objects) != 1 || list.Objects[0].Key != "movie.mp4" {
		t.Fatalf("objects: %+v", list.Objects)
	}
	if len(list.Prefixes) != 1 || list.Prefixes[0] != "sub/" {
		t.Fatalf("prefixes: %+v", list.Prefixes)
	}

	req, _ := http.NewRequest("GET", u.srv.URL+base+"/object?key=movie.mp4&inline=1", nil)
	req.AddCookie(u.cookie)
	req.Header.Set("Range", "bytes=4-7")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(body) != "4567" {
		t.Fatalf("range: %d %q", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges: %q", got)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "video/mp4") {
		t.Fatalf("content type: %q", ct)
	}
}

// TestS3ClientDirect exercises the signer against the stub without the HTTP layer of WRM.
func TestS3ClientDirect(t *testing.T) {
	stub := newS3Stub(t, "b", "AK", "SK")
	srv := httptest.NewServer(stub)
	defer srv.Close()
	c, err := newS3Client(s3Config{Endpoint: srv.URL, Region: "us-east-1", Bucket: "b", PathStyle: true,
		AccessKey: "AK", SecretKey: "SK"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.headBucket(rctx()); err != nil {
		t.Fatalf("headBucket: %v", err)
	}
	if _, err := c.put(rctx(), "a/b c.json", []byte(`{"x":"y"}`), "application/json", ""); err != nil {
		t.Fatalf("put with a space in the key: %v", err)
	}
	if _, err := c.list(rctx(), "", "", 10); err != nil {
		t.Fatalf("list: %v", err)
	}
	meta, err := c.head(rctx(), "a/b c.json")
	if err != nil || meta.Size != 9 {
		t.Fatalf("head: %+v %v", meta, err)
	}
	if err := c.copy(rctx(), "a/b c.json", "a/copy.json"); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if err := c.delete(rctx(), "a/copy.json"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := c.delete(rctx(), "a/copy.json"); err != nil {
		t.Fatalf("delete of a missing key: %v", err)
	}
}
