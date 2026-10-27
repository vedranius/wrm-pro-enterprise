// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite the golden files in testdata/service")

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "service", name+".golden")
	if *updateGolden {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -run TestServiceFilesGolden -update-golden)", err)
	}
	if string(want) != got {
		t.Fatalf("%s differs from %s:\n--- got ---\n%s\n--- want ---\n%s", name, path, got, want)
	}
}

// TestServiceFilesGolden checks the service definitions for every OS against golden files.
func TestServiceFilesGolden(t *testing.T) {
	linux := serviceSpec{Name: "wrm", Exe: "/opt/wrm/wrm-pro-v11.7.0-linux-amd64", WorkDir: "/opt/wrm", DataDir: "/var/lib/wrm",
		EnvFile: "/var/lib/wrm/wrm.env", User: "wrm", Scope: "system", RWPaths: []string{"/opt/wrm", "/var/lib/wrm"},
		LogFile: "/var/lib/wrm/wrm-service.log"}
	checkGolden(t, "systemd.service", systemdUnit(linux))

	spaces := linux
	spaces.Exe, spaces.WorkDir, spaces.DataDir = "/srv/remote tools/wrm-pro-v11.7.0-linux-arm64", "/srv/remote tools", "/srv/remote tools"
	spaces.EnvFile, spaces.RWPaths, spaces.LowPort, spaces.User = "/srv/remote tools/wrm.env", []string{"/srv/remote tools"}, true, "ops"
	checkGolden(t, "systemd-spaces-lowport.service", systemdUnit(spaces))

	mac := serviceSpec{Name: "wrm", Exe: "/Users/demo/wrm/wrm-pro-v11.7.0-darwin-arm64", WorkDir: "/Users/demo/wrm", DataDir: "/Users/demo/wrm",
		EnvFile: "/Users/demo/wrm/wrm.env", User: "demo", Scope: "system", LogFile: "/Users/demo/wrm/wrm-service.log"}
	checkGolden(t, "launchd-daemon.plist", launchdPlist(mac))
	mac.Scope = "user"
	checkGolden(t, "launchd-agent.plist", launchdPlist(mac))

	bsd := serviceSpec{Name: "wrm", Exe: "/usr/local/wrm/wrm-pro-v11.7.0-freebsd-amd64", WorkDir: "/usr/local/wrm", DataDir: "/var/db/wrm",
		EnvFile: "/var/db/wrm/wrm.env", User: "wrm", LogFile: "/var/db/wrm/wrm-service.log"}
	checkGolden(t, "freebsd.rc", freebsdRCScript(bsd))
	bsd.Exe = "/usr/local/wrm/wrm-pro-v11.7.0-openbsd-amd64"
	checkGolden(t, "openbsd.rc", openbsdRCScript(bsd))

	win := serviceSpec{Name: "wrm", Exe: `C:\Program Files\WRM\wrm-pro-v11.7.0-windows-amd64.exe`, WorkDir: `C:\ProgramData\WRM`,
		DataDir: `C:\ProgramData\WRM`, EnvFile: `C:\ProgramData\WRM\wrm.env`, LogFile: `C:\ProgramData\WRM\wrm-service.log`}
	checkGolden(t, "windows.txt", windowsServiceConfig(win))

	env := formatEnvFile("wrm", map[string]string{"PORT": "8443", "DB_PATH": "/var/lib/wrm/remote_manager.db", "HTTPS_SELF_SIGNED": "1",
		"WRM_UPDATE_CHECK": "off", "WRM_GIT_BACKUP_WORDS": `a "quoted" value`})
	checkGolden(t, "wrm.env", env)
}

func TestEnvFileRoundTrip(t *testing.T) {
	in := map[string]string{"A": "plain", "B": "with spaces", "C": `quote " and back\slash`, "D": "$HOME#x", "E": "ünïcode"}
	out := parseEnvFile(formatEnvFile("wrm", in))
	for k, v := range in {
		if out[k] != v {
			t.Fatalf("%s: got %q, want %q", k, out[k], v)
		}
	}
	got := parseEnvFile("# comment\nexport X=1\r\nY = 'single quoted'\nbad line\n=empty\n")
	if got["X"] != "1" || got["Y"] != "single quoted" || len(got) != 2 {
		t.Fatalf("parse: %v", got)
	}

	dir := t.TempDir()
	f := filepath.Join(dir, "wrm.env")
	os.WriteFile(f, []byte("WRM_TEST_ENV_A=from-file\nWRM_TEST_ENV_B=from-file\n"), 0o600)
	t.Setenv("WRM_TEST_ENV_A", "from-environment")
	os.Unsetenv("WRM_TEST_ENV_B")
	defer os.Unsetenv("WRM_TEST_ENV_B")
	if err := loadEnvFile(f); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("WRM_TEST_ENV_A") != "from-environment" || os.Getenv("WRM_TEST_ENV_B") != "from-file" {
		t.Fatalf("environment wins over the file: A=%q B=%q", os.Getenv("WRM_TEST_ENV_A"), os.Getenv("WRM_TEST_ENV_B"))
	}
}

func TestServiceEnvironmentAndSpec(t *testing.T) {
	t.Setenv("PORT", "443")
	t.Setenv("WRM_UPDATE_CHECK", "off")
	t.Setenv("WRM_SWITCHED_FROM", "/old/binary")
	env := serviceEnvironment("/data/remote_manager.db")
	if env["DB_PATH"] != "/data/remote_manager.db" || env["PORT"] != "443" || env["WRM_UPDATE_CHECK"] != "off" {
		t.Fatalf("env: %v", env)
	}
	if _, ok := env["WRM_SWITCHED_FROM"]; ok {
		t.Fatal("WRM_SWITCHED_FROM must not reach the service")
	}

	dir := t.TempDir()
	t.Setenv("DB_PATH", filepath.Join(dir, "remote_manager.db"))
	s, err := buildServiceSpec(serviceOptions{Name: "wrm_test"})
	if err != nil {
		t.Fatal(err)
	}
	if s.EnvFile != filepath.Join(dir, "wrm_test.env") || s.DataDir != dir || !s.LowPort || s.Scope != "system" {
		t.Fatalf("spec: %+v", s)
	}
	fi, err := os.Stat(s.EnvFile)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("env file mode %v", fi.Mode().Perm())
	}
	data, _ := os.ReadFile(s.EnvFile)
	if got := parseEnvFile(string(data)); got["DB_PATH"] != filepath.Join(dir, "remote_manager.db") || got["PORT"] != "443" {
		t.Fatalf("env file: %s", data)
	}
	for _, bad := range []string{"", "1wrm", "wrm-pro", "wrm service", strings.Repeat("a", 41)} {
		if _, err := buildServiceSpec(serviceOptions{Name: bad}); err == nil {
			t.Fatalf("service name %q accepted", bad)
		}
	}
	if _, err := buildServiceSpec(serviceOptions{Name: "wrm", Scope: "galaxy"}); err == nil {
		t.Fatal("bad scope accepted")
	}

	// The prompt's answer is remembered next to the database; tests have no terminal on stdin.
	if serviceAnswerRemembered() {
		t.Fatal("no answer yet")
	}
	rememberServiceAnswer("never")
	if !serviceAnswerRemembered() {
		t.Fatal("answer not remembered")
	}
	if _, err := os.Stat(filepath.Join(dir, "remote_manager.db.service.json")); err != nil {
		t.Fatal(err)
	}
	if shouldPromptService(false, "wrm") || shouldPromptService(true, "wrm") {
		t.Fatal("no prompt without a terminal / with -no-service-prompt")
	}
	if a := readAnswer(strings.NewReader("Yes\n"), time.Second); a != "yes" {
		t.Fatalf("answer %q", a)
	}
}

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v11.7.0", "v11.6.1", 1},
		{"v11.6.1", "v11.7.0", -1},
		{"v11.10.0", "v11.9.9", 1},
		{"v12.0.0", "v11.99.99", 1},
		{"11.7.0", "v11.7.0", 0},
		{"v11.7.0-1a2b3c4", "v11.7.0", -1},
		{"v11.7.0", "v11.7.0-rc.1", 1},
		{"v11.7.0-rc.2", "v11.7.0-rc.10", -1},
		{"v11.7.0-alpha", "v11.7.0-beta", -1},
		{"v11.7.0-rc.1", "v11.7.0-rc", 1},
		{"v11.7.0+build.5", "v11.7.0", 0},
		{"dev", "v11.7.0", 0},
	}
	for _, c := range cases {
		if got := compareSemver(c.a, c.b); got != c.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	for _, bad := range []string{"", "dev", "v1.2", "v1.2.3.4", "1.2.x"} {
		if _, ok := parseVersion(bad); ok {
			t.Errorf("parseVersion(%q) accepted", bad)
		}
	}
	if !versionAnswerMatches("v11.8.0", "v11.8.0") || !versionAnswerMatches("v11.8.0-1a2b3c4", "v11.8.0") || versionAnswerMatches("v11.7.0", "v11.8.0") || versionAnswerMatches("", "v11.8.0") {
		t.Fatal("versionAnswerMatches")
	}
}

func TestPlatformLabelsAndAssetNames(t *testing.T) {
	if l := platformLabels("darwin", "arm64", ""); strings.Join(l, ",") != "darwin-arm64,darwin-universal" {
		t.Fatal(l)
	}
	if l := platformLabels("linux", "arm", "6"); strings.Join(l, ",") != "linux-armv6" {
		t.Fatal(l)
	}
	if l := platformLabels("linux", "arm", "7"); strings.Join(l, ",") != "linux-armv7,linux-armv6" {
		t.Fatal(l)
	}
	if n := releaseAssetName("v11.7.0", "windows-amd64"); n != "wrm-pro-v11.7.0-windows-amd64.exe" {
		t.Fatal(n)
	}
	if n := releaseAssetName("v11.7.0", "linux-386"); n != "wrm-pro-v11.7.0-linux-386" {
		t.Fatal(n)
	}
}

// writeFakeBinary writes a shell script that answers -version like a WRM binary.
func writeFakeBinary(t *testing.T, path, answer string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho "+answer+"\n"), mode); err != nil {
		t.Fatal(err)
	}
}

func TestNewestBinaryInFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell scripts as fake binaries")
	}
	dir := t.TempDir()
	labels := platformLabels("linux", "amd64", "")
	self := filepath.Join(dir, "wrm-pro-v11.7.0-linux-amd64")
	writeFakeBinary(t, self, "v11.7.0", 0o755)
	writeFakeBinary(t, filepath.Join(dir, "wrm-pro-v11.6.0-linux-amd64"), "v11.6.0", 0o755)         // older
	writeFakeBinary(t, filepath.Join(dir, "wrm-pro-v11.9.0-linux-amd64"), "v11.7.0", 0o755)         // answers the wrong version
	os.WriteFile(filepath.Join(dir, "wrm-pro-v12.0.0-linux-amd64"), []byte("not a program"), 0o755) // does not run
	writeFakeBinary(t, filepath.Join(dir, "wrm-pro-v13.0.0-linux-arm64"), "v13.0.0", 0o755)         // other architecture
	writeFakeBinary(t, filepath.Join(dir, "wrm-pro-v13.0.0-windows-amd64.exe"), "v13.0.0", 0o755)   // other OS
	writeFakeBinary(t, filepath.Join(dir, "wrm-pro-v13.0.0-linux-amd64.bak"), "v13.0.0", 0o755)     // not a release name
	writeFakeBinary(t, filepath.Join(dir, "wrm-pro-v13-linux-amd64"), "v13.0.0", 0o755)             // not a semantic version
	os.Mkdir(filepath.Join(dir, "wrm-pro-v14.0.0-linux-amd64"), 0o755)                              // a directory
	writeFakeBinary(t, filepath.Join(dir, "wrm-pro-v11.8.0-linux-amd64"), "v11.8.0", 0o644)         // copied in without +x

	nb := findNewerBinary(dir, "v11.7.0", self, labels)
	if nb == nil || nb.Version != "v11.8.0" || nb.File != "wrm-pro-v11.8.0-linux-amd64" {
		t.Fatalf("newest valid binary: %+v", nb)
	}
	if fi, _ := os.Stat(nb.Path); fi.Mode()&0o100 == 0 {
		t.Fatal("the copied-in binary was not made executable")
	}
	if nb := findNewerBinary(dir, "v11.8.0", self, labels); nb != nil {
		t.Fatalf("nothing newer than v11.8.0 is valid, got %+v", nb)
	}
	// A branch build of the same version is older than the release.
	if nb := findNewerBinary(dir, "v11.8.0-1a2b3c4", self, labels); nb == nil || nb.Version != "v11.8.0" {
		t.Fatalf("release after branch build: %+v", nb)
	}
	// The running binary itself is never chosen, whatever its name says.
	if nb := findNewerBinary(dir, "v11.0.0", filepath.Join(dir, "wrm-pro-v11.8.0-linux-amd64"), labels); nb == nil || nb.Version != "v11.7.0" {
		t.Fatalf("self excluded: %+v", nb)
	}
	if nb := findNewerBinary(filepath.Join(dir, "missing"), "v11.7.0", self, labels); nb != nil {
		t.Fatal("missing folder")
	}
}
