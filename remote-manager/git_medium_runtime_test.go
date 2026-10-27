// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediumDockerArgumentsAndReadiness(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "with spaces", "test.db"))
	t.Setenv("WRM_MEDIUM_RUNTIME", "docker")
	if e := os.MkdirAll(mediumRoot(), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(mediumRoot(), "docker-ready"), []byte(mediumImage()), 0600); e != nil {
		t.Fatal(e)
	}
	if !mediumRuntimeReady() {
		t.Fatal("container readiness missing")
	}
	_ = os.WriteFile(filepath.Join(mediumRoot(), "setup-pending"), []byte("pending"), 0600)
	if mediumRuntimeReady() {
		t.Fatal("failed/pending setup reported ready")
	}
	args := mediumDockerStartArgs(7, 19007, "wrm-medium-7-test", []string{"WRM_MEDIUM_KEY", "DEPLOY_CONFIG_REPO"})
	s := strings.Join(args, "\n")
	for _, want := range []string{"127.0.0.1:19007:8080", mediumDockerVolume(7) + ":/wrm-medium/users/7", mediumToolDir() + ":/wrm-medium/tool:ro", "--env\nWRM_MEDIUM_KEY", "/wrm-medium/docker-venv/bin/python"} {
		if !strings.Contains(s, want) {
			t.Fatal("missing container boundary:", want)
		}
	}
	if strings.Contains(s, "/var/run/docker.sock") || strings.Contains(s, "users/8") {
		t.Fatal("unexpected mount")
	}
	if got := mediumContainerPath(filepath.Join(mediumRoot(), "import-123")); got != "/wrm-medium/import-123" {
		t.Fatal(got)
	}
	if got := mediumContainerPath("https://example.com/config.git"); got != "https://example.com/config.git" {
		t.Fatal(got)
	}
	if !strings.Contains(mediumSSHCommand(7), "/wrm-medium/users/7/home/.ssh/config") {
		t.Fatal("SSH uses host paths")
	}
}

func TestMediumMissingDockerPreservesImportedSource(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	t.Setenv("WRM_MEDIUM_RUNTIME", "docker")
	t.Setenv("PATH", t.TempDir())
	if e := os.MkdirAll(mediumToolDir(), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(mediumToolDir(), "approved-source"), []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	e := mediumPrepareRuntime(context.Background(), nil)
	if e == nil || !strings.Contains(e.Error(), "Docker") {
		t.Fatal(e)
	}
	if !mediumFileExists(filepath.Join(mediumToolDir(), "approved-source")) || mediumRuntimeReady() {
		t.Fatal("source lost or failed setup ready")
	}
}

func TestLetterHotfixVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"v12.4.3b", "v12.4.3", 1}, {"v12.4.3b", "v12.4.4", -1}, {"v12.4.3a", "v12.4.3b", -1}, {"v12.4.3b-test", "v12.4.3b", -1},
	} {
		if got := compareSemver(c.a, c.b); got != c.want {
			t.Fatalf("%+v: %d", c, got)
		}
	}
	if !versionAnswerMatches("v12.4.3b", "v12.4.3b") || versionAnswerMatches("v12.4.3b", "v12.4.3") {
		t.Fatal("hotfix identity")
	}
	if !binaryNameRe.MatchString("wrm-pro-v12.4.3b-windows-amd64.exe") {
		t.Fatal("hotfix filename rejected")
	}
}
