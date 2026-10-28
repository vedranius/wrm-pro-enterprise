// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"testing"
	"time"
)

func bundleArchive(t *testing.T, manifest string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	name := "demo/bundle.json"
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(manifest))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestBundleManifestRejectsUnsafePaths covers DATA-02: paths and the bundle id from
// bundle.json must never become server paths without validation.
func TestBundleManifestRejectsUnsafePaths(t *testing.T) {
	name := fmt.Sprintf("bundle-fix-%d", time.Now().UnixNano())
	res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?,?,0,?)`,
		name, "x", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	uid64, _ := res.LastInsertId()
	uid := int(uid64)
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id=?`, uid) })

	bad := map[string]string{
		"parent traversal": `{"bundle_id":"20260101-1200","apps":{"demo":{"files":{"../evil.conf":{"hash":"00"}}}}}`,
		"absolute path":    `{"bundle_id":"20260101-1200","apps":{"demo":{"files":{"/etc/passwd":{"hash":"00"}}}}}`,
		"dot segment":      `{"bundle_id":"20260101-1200","apps":{"demo":{"files":{"conf/./app.ini":{"hash":"00"}}}}}`,
		"empty segment":    `{"bundle_id":"20260101-1200","apps":{"demo":{"files":{"conf//app.ini":{"hash":"00"}}}}}`,
		"backslash":        `{"bundle_id":"20260101-1200","apps":{"demo":{"files":{"conf\\app.ini":{"hash":"00"}}}}}`,
		"invalid id":       `{"bundle_id":"../escape","apps":{"demo":{"files":{"conf/app.ini":{"hash":"00"}}}}}`,
	}
	for label, manifest := range bad {
		if _, err := importBundle(uid, bytes.NewReader(bundleArchive(t, manifest))); err == nil {
			t.Errorf("%s: bundle was accepted", label)
		}
	}

	good := `{"bundle_id":"20260101-1200","apps":{"demo":{"commit":"abc","files":{"conf/app.ini":{"hash":"00"},"bin/run.sh":{"hash":"00"}}}}}`
	out, err := importBundle(uid, bytes.NewReader(bundleArchive(t, good)))
	if err != nil {
		t.Fatalf("valid bundle refused: %v", err)
	}
	if out.Source.ID > 0 {
		deleteGitSource(out.Source.ID)
	}
}
