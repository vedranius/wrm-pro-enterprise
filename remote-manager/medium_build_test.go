// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func mdTestTree(t *testing.T, files map[string]string) mdTree {
	t.Helper()
	root := t.TempDir()
	for p, v := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if strings.HasPrefix(v, "->") {
			os.Symlink(strings.TrimPrefix(v, "->"), full)
			continue
		}
		if err := os.WriteFile(full, []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := mdLocalTree(root)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func mdSameTree(a, b mdTree) bool {
	if len(a) != len(b) {
		return false
	}
	for p, x := range a {
		y := b[p]
		if y == nil || x.Type != y.Type || x.Link != y.Link {
			return false
		}
		if x.Type == "file" && x.Hash != y.Hash {
			return false
		}
	}
	return true
}

func TestMediumBuildTreeRoundTrip(t *testing.T) {
	tree := mdTestTree(t, map[string]string{
		"index.html": "hello",
		"lib/app.js": "console.log(1)",
		"link":       "->lib",
	})
	root := t.TempDir()
	if err := mdWriteTree(root, tree); err != nil {
		t.Fatal(err)
	}
	got, err := mdLocalTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if !mdSameTree(tree, got) {
		t.Fatalf("written tree differs: %v", got)
	}
	archive := filepath.Join(t.TempDir(), "source.tar.gz")
	if err := mdWriteTreeArchive(archive, tree); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := mdExtractTarGz(archive, out); err != nil {
		t.Fatal(err)
	}
	extracted, err := mdLocalTree(out)
	if err != nil {
		t.Fatal(err)
	}
	if !mdSameTree(tree, extracted) {
		t.Fatalf("extracted tree differs: %v", extracted)
	}
}

func TestMediumBuildOnHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("host builds need a POSIX shell")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not installed")
	}
	tree := mdTestTree(t, map[string]string{"src/index.txt": "hello"})
	build := "mkdir -p dist && cp src/index.txt dist/index.txt && printf built > dist/out.txt"
	out, err := mdBuildOnHost(tree, &mdProject{Name: "demo-web", SourceDir: "dist", Build: &build})
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if out.Location != "host" || out.Platform != runtime.GOOS || out.Arch != mdNormalizeArch(runtime.GOARCH) {
		t.Fatalf("build info: %+v", out)
	}
	sum := sha256.Sum256([]byte("hello"))
	if e := out.Tree["index.txt"]; e == nil || e.Hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("built index: %+v", e)
	}
	if e := out.Tree["out.txt"]; e == nil || e.Size != 5 {
		t.Fatalf("built out: %+v", e)
	}
}

func TestMediumBuildOnServer(t *testing.T) {
	if runtime.GOOS == "linux" {
		// mdBuildOnServer only accepts Linux build servers.
	} else {
		t.Skip("the remote-build test runs a Linux build server locally")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not installed")
	}
	old := mdOpenConnHost
	mdOpenConnHost = func(uid, connID int, fn func(mdHost, string) error) error {
		return fn(mdLocal{}, "build-01")
	}
	t.Cleanup(func() { mdOpenConnHost = old })
	tree := mdTestTree(t, map[string]string{"src/index.txt": "hello"})
	build := "mkdir -p dist && cp src/index.txt dist/index.txt && printf built > dist/out.txt"
	out, err := mdBuildOnServer(1, 2, tree, &mdProject{Name: "demo-web", SourceDir: "dist", Build: &build})
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if out.Location != "server" || out.Server != "build-01" || out.Platform != "linux" {
		t.Fatalf("build info: %+v", out)
	}
	sum := sha256.Sum256([]byte("hello"))
	if e := out.Tree["index.txt"]; e == nil || e.Hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("built index: %+v", e)
	}
	if e := out.Tree["out.txt"]; e == nil || e.Size != 5 {
		t.Fatalf("built out: %+v", e)
	}
}

func TestMediumBuildPlatformCheck(t *testing.T) {
	res := &mdPlanResult{Servers: []mdServerPlan{
		{Server: "app-01", Platform: "linux", Arch: "amd64"},
		{Server: "app-02", Platform: "linux", Arch: "arm64"},
	}}
	mdCheckBuildPlatforms(res, "server", "linux", "amd64")
	if res.Servers[0].Error != "" || !strings.Contains(res.Servers[1].Error, "arm64") {
		t.Fatalf("platform check: %+v", res.Servers)
	}
	res = &mdPlanResult{Servers: []mdServerPlan{{Server: "app-01"}}}
	mdCheckBuildPlatforms(res, "host", "linux", "amd64")
	if len(res.Servers[0].Warnings) == 0 {
		t.Fatal("a destination without platform information should warn")
	}
}

func TestMediumBuildSettings(t *testing.T) {
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := newTestServer(t)
	u := newTestUser(t, srv, "md-build-settings", false)
	id := u.addConnection("build-01", h.addr)
	if _, err := mdSetBuild(u.userID, "demo-web", "none", 0); err != nil {
		t.Fatal(err)
	}
	if s := mdBuildOf(u.userID, "demo-web"); s.Location != "none" || s.ConnID != 0 {
		t.Fatalf("none: %+v", s)
	}
	if _, err := mdSetBuild(u.userID, "demo-web", "server", int(id)); err != nil {
		t.Fatal(err)
	}
	if s := mdBuildOf(u.userID, "demo-web"); s.Location != "server" || s.ConnID != int(id) || s.ConnName != "build-01" {
		t.Fatalf("server: %+v", s)
	}
	if _, err := mdSetBuild(u.userID, "demo-web", "server", 0); err == nil {
		t.Fatal("a build server without a connection must be refused")
	}
	if _, err := mdSetBuild(u.userID, "demo-web", "elsewhere", 0); err == nil {
		t.Fatal("an unknown build location must be refused")
	}
	if _, err := mdSetBuild(u.userID, "demo-web", "", 0); err != nil {
		t.Fatal(err)
	}
	if s := mdBuildOf(u.userID, "demo-web"); s.Location != "" {
		t.Fatalf("cleared: %+v", s)
	}
}

func TestMediumBuildNeedsLocation(t *testing.T) {
	cfg, err := mdLoadConfig(mdValidConfig(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	proj := cfg.project("demo-web")
	if _, err := mdBuildProject(1, mdSourceMatch{}, proj, "commit", mdBuildSetting{}, func(string) {}); err == nil || !strings.Contains(err.Error(), "choose where to build") {
		t.Fatalf("missing build location: %v", err)
	}
}
