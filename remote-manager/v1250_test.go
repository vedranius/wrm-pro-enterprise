// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestV1250TrimSplitRune(t *testing.T) {
	for in, want := range map[string]string{"abc": "abc", "ab\xc4": "ab", "ab\xe2\x82": "ab", "ab€": "ab€", "": "", "a\xff": "a\xff"} {
		if got := trimSplitRune(in); got != want {
			t.Fatalf("trimSplitRune(%q) = %q, want %q", in, got, want)
		}
	}
}

// A CP1250 byte in a scanned segment is kept for the byte count and replaced only in the
// returned text; the whole segment is consumed.
func TestV1250LogScanInvalidUTF8(t *testing.T) {
	text := "2026-10-01 10:00:00 ERROR gre\x9aka\n2026-10-01 10:00:01 info ok\n"
	start := time.Now()
	out, _, matches, consumed, _ := filterMonitorLogSegment(text, "error", time.Time{}, time.Time{}, time.UTC, time.Time{})
	if consumed != len(text) || matches != 1 || !strings.Contains(out, "gre�ka") || time.Since(start) > time.Second {
		t.Fatalf("scan: consumed %d/%d, matches %d, out %q", consumed, len(text), matches, out)
	}
}

func v1250Zip(t *testing.T, files map[string]string) []byte {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for n, c := range files {
		w, _ := z.Create(n)
		w.Write([]byte(c))
	}
	z.Close()
	return b.Bytes()
}

func TestV1250MediumSSHZipCaseDuplicates(t *testing.T) {
	dir := t.TempDir()
	if err := mediumExtract(v1250Zip(t, map[string]string{"id_demo": "a", "ID_DEMO": "b"}), dir, "ssh"); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("case-only duplicates: %v", err)
	}
	if err := mediumExtract(v1250Zip(t, map[string]string{"config": "Host *\n", "known_hosts": ""}), t.TempDir(), "ssh"); err != nil {
		t.Fatalf("plain profile: %v", err)
	}
}

func TestV1250MediumReplaceDirDropsPrevious(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "ssh")
	for i, key := range []string{"old-key", "new-key"} {
		src := filepath.Join(root, "src"+string(rune('0'+i)))
		os.MkdirAll(src, 0700)
		os.WriteFile(filepath.Join(src, "id_demo"), []byte(key), 0600)
		if err := mediumReplaceDir(src, dst); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "id_demo")); string(b) != "new-key" {
		t.Fatalf("current profile: %q", b)
	}
	if _, err := os.Stat(dst + ".previous"); !os.IsNotExist(err) {
		t.Fatalf("the replaced profile (with its keys) is still on disk: %v", err)
	}
}

// A partial stamp records a note in VERSION.md that parseVersionMD ignores.
func TestV1250VersionMDNote(t *testing.T) {
	tg := gitTarget{App: "demo-api", Version: "v1.10", Branch: "main", RefKind: "branch", Commit: "0123456789abcdef"}
	md := string(versionMDNote(tg, nil, time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC), "app-01", "deploy", "wrm-1", "djelomicno: 1 datoteka se razlikuje od verzije v1.10", false))
	if !strings.Contains(md, "- **Napomena**:") {
		t.Fatalf("no note:\n%s", md)
	}
	if v := parseVersionMD(strings.Split(md, "\n")); v.Version != "v1.10" || v.App != "demo-api" {
		t.Fatalf("parsed: %+v", v)
	}
	if strings.Contains(string(versionMD(tg, nil, time.Now(), "h", "u", "b", false)), "Napomena") {
		t.Fatal("a normal stamp has no note")
	}
}
