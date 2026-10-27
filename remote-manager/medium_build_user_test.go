//go:build !windows

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediumBuildUser(t *testing.T) {
	defer setSetting("medium_build_user", "")
	if _, err := setSetting("medium_build_user", "root"); err == nil {
		t.Fatal("root accepted as build user")
	}
	if _, err := setSetting("medium_build_user", "bad name;"); err == nil {
		t.Fatal("invalid account name accepted")
	}
	if acc, err := mdResolveBuildUser(); acc != nil || err != nil {
		t.Fatalf("empty build user: %v %v", acc, err)
	}
	if _, err := setSetting("medium_build_user", "wrm-no-such-user"); err != nil {
		t.Fatal(err)
	}
	if _, err := mdResolveBuildUser(); err == nil {
		t.Fatal("a missing build user resolved")
	}
	if os.Geteuid() != 0 {
		if _, err := mdResolveBuildUser(); err == nil || !strings.Contains(err.Error(), "root") {
			t.Fatalf("not root: %v", err)
		}
		t.Skip("switching the build account needs root")
	}
	if _, err := user.Lookup("nobody"); err != nil {
		t.Skip("no nobody account")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not installed")
	}
	if _, err := setSetting("medium_build_user", "nobody"); err != nil {
		t.Fatal(err)
	}
	// The key file readable by everyone: the build user is refused.
	key := resolveDBPath() + ".key"
	if fi, err := os.Stat(key); err == nil {
		os.Chmod(key, 0o644)
		_, err := mdResolveBuildUser()
		os.Chmod(key, fi.Mode().Perm())
		if err == nil || !strings.Contains(err.Error(), "can read") {
			t.Fatalf("readable key not refused: %v", err)
		}
	}
	tree := mdTestTree(t, map[string]string{"src/index.txt": "hello"})
	build := "mkdir -p dist && id -u > dist/uid && printf %s \"$HOME|$WRM_TEST_SECRET\" > dist/env"
	os.Setenv("WRM_TEST_SECRET", "s3cret")
	defer os.Unsetenv("WRM_TEST_SECRET")
	out, err := mdBuildOnHost(tree, &mdProject{Name: "demo-web", SourceDir: "dist", Build: &build})
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if out.User != "nobody" {
		t.Fatalf("build user not reported: %+v", out)
	}
	if e := out.Tree["uid"]; e == nil || e.Size != int64(len("65534\n")) {
		t.Fatalf("build did not run as nobody: %+v", e)
	}
	e := out.Tree["env"]
	if e == nil || e.open == nil {
		t.Fatalf("env file: %+v", e)
	}
	if b, err := e.open(); err != nil || strings.Contains(string(b), "s3cret") || !strings.HasPrefix(string(b), "/") {
		t.Fatalf("build environment: %q %v", b, err)
	}
}

func TestMediumBuildUserHardLink(t *testing.T) {
	defer setSetting("medium_build_user", "")
	if os.Geteuid() != 0 {
		t.Skip("switching the build account needs root")
	}
	if _, err := user.Lookup("nobody"); err != nil {
		t.Skip("no nobody account")
	}
	if _, err := setSetting("medium_build_user", "nobody"); err != nil {
		t.Fatal(err)
	}
	// a root-owned file the build account may link to but not read (a stand-in for the key)
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("s3cret"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Chmod(filepath.Dir(secret), 0o755)
	acc, err := mdResolveBuildUser()
	if err != nil {
		t.Fatal(err)
	}
	// a hard link that is not the build account's own file is refused
	dir := t.TempDir()
	if err := os.Link(secret, filepath.Join(dir, "stolen")); err != nil {
		t.Fatal(err)
	}
	if err := mdReclaimBuildDir(acc, dir); err == nil || !strings.Contains(err.Error(), "hard link") {
		t.Fatalf("hard link not refused: %v", err)
	}
	// a build that leaves its files writable for others gets them back without that
	tree := mdTestTree(t, map[string]string{"src/index.txt": "hello"})
	build := "mkdir -p dist && cp src/index.txt dist/ && chmod 0777 dist && chmod 0666 dist/index.txt"
	out, err := mdBuildOnHost(tree, &mdProject{Name: "demo-web", SourceDir: "dist", Build: &build})
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if e := out.Tree["index.txt"]; e == nil || e.Size != 5 {
		t.Fatalf("built file: %+v", e)
	}
}

func TestMediumBuildUserLinkedOutput(t *testing.T) {
	defer setSetting("medium_build_user", "")
	if os.Geteuid() != 0 {
		t.Skip("switching the build account needs root")
	}
	if _, err := user.Lookup("nobody"); err != nil {
		t.Skip("no nobody account")
	}
	if _, err := setSetting("medium_build_user", "nobody"); err != nil {
		t.Fatal(err)
	}
	tree := mdTestTree(t, map[string]string{"src/index.txt": "hello"})
	// a link on the way to source_dir must not point the read-back outside the build
	build := "ln -s / a"
	if _, err := mdBuildOnHost(tree, &mdProject{Name: "demo-web", SourceDir: "a/etc", Build: &build}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("source_dir through a link: %v", err)
	}
	// the build cannot replace its own directory (the parent belongs to WRM)
	build = "d=$PWD; mv \"$d\" \"$d.x\" && ln -s / \"$d\"; mkdir -p etc"
	out, err := mdBuildOnHost(tree, &mdProject{Name: "demo-web", SourceDir: "etc", Build: &build})
	if err == nil {
		defer out.Close()
		if len(out.Tree) != 0 {
			t.Fatalf("read outside the build: %d entries", len(out.Tree))
		}
	}
}
