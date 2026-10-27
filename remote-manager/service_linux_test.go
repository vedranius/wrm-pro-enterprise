//go:build linux

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemdUnitParser(t *testing.T) {
	analyzer, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("systemd-analyze is not installed")
	}
	dir := filepath.Join(t.TempDir(), "data 100% $cash")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s := serviceSpec{Name: "wrm_parser_test", Exe: exe, WorkDir: dir, EnvFile: filepath.Join(dir, "wrm.env"), RWPaths: []string{dir}}
	unit := systemdUnit(s)
	for _, directive := range []string{
		"WorkingDirectory=" + strings.ReplaceAll(dir, "%", "%%"),
		"EnvironmentFile=" + strings.ReplaceAll(s.EnvFile, "%", "%%"),
		`ReadWritePaths="` + strings.ReplaceAll(dir, "%", "%%") + `"`,
	} {
		if !strings.Contains(unit, directive+"\n") {
			t.Fatalf("incorrect single-path / path-list escaping: missing %q", directive)
		}
	}
	p := filepath.Join(t.TempDir(), "wrm_parser_test.service")
	if err := os.WriteFile(p, []byte(unit), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(analyzer, "--man=no", "verify", p)
	if output, err := cmd.CombinedOutput(); err != nil || len(output) != 0 {
		t.Fatalf("systemd rejected the generated unit or ignored a directive: %v\n%s", err, output)
	}
}

func TestSystemdServiceStartAndDiagnostics(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "commands.log")
	t.Setenv("WRM_TEST_SERVICE_LOG", logFile)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$WRM_TEST_SERVICE_LOG"
case "$1" in
restart) [ "$WRM_TEST_START_FAIL" != 1 ] || { echo 'start failed'; exit 1; };;
status) echo 'Active: failed'; exit 3;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte("#!/bin/sh\necho 'attempt to write a readonly database'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := serviceSpec{Name: "wrm_test", EnvFile: "/data/wrm_test.env", User: "service-test"}
	t.Setenv("WRM_TEST_START_FAIL", "0")
	if err := startSystemdService(s); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(logFile)
	if err != nil || string(calls) != "enable wrm_test.service\nreset-failed wrm_test.service\nrestart wrm_test.service\n" {
		t.Fatalf("installation must enable, clear old failures and restart the new unit: %q (%v)", calls, err)
	}
	t.Setenv("WRM_TEST_START_FAIL", "1")
	err = startSystemdService(s)
	if err == nil {
		t.Fatal("failed start was reported as successful")
	}
	for _, want := range []string{"Service status:", "Active: failed", "Recent service log:", "readonly database", "DB_PATH", s.EnvFile, s.User} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing startup diagnostic %q: %v", want, err)
		}
	}
}

func TestSystemdDatabaseAccessPreflight(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Exercise the real subprocess boundary without root or changing system services.
	// The shim checks the requested account, then runs test as the test runner's account.
	script := `#!/bin/sh
[ "$1" = "-u" ] && [ "$2" = "service-test" ] && [ "$3" = "--" ] || exit 2
shift 3
printf '%s\n' "$*" >> "$WRM_TEST_ACCESS_LOG"
[ "$2" = "$WRM_TEST_DENIED_ACCESS" ] && [ "$3" = "$WRM_TEST_DENIED_PATH" ] && exit 1
exec "$@"
`
	if err := os.WriteFile(filepath.Join(bin, "runuser"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WRM_TEST_ACCESS_LOG", filepath.Join(dir, "access.log"))
	data := filepath.Join(dir, "data with spaces")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(data, "wrm.db")
	t.Setenv("DB_PATH", dbPath)
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	envFile := filepath.Join(data, "wrm_test.env")
	if err := os.WriteFile(envFile, []byte("existing service settings\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, path, access, category string }{
		{"writable", "", "", ""},
		{"database-read", dbPath, "-r", "database file"},
		{"database-write", dbPath, "-w", "database file"},
		{"wal", dbPath + "-wal", "-w", "database file"},
		{"shm", dbPath + "-shm", "-w", "database file"},
		{"directory-write", data, "-w", "database directory"},
		{"directory-search", data, "-x", "database directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WRM_TEST_DENIED_PATH", tc.path)
			t.Setenv("WRM_TEST_DENIED_ACCESS", tc.access)
			s, err := collectServiceSpec(serviceOptions{Name: "wrm_test", User: "service-test"})
			if err != nil || s.DBPath != dbPath {
				t.Fatalf("service spec: %+v %v", s, err)
			}
			err = checkSystemdDatabaseAccess(s)
			if tc.category == "" && err != nil || tc.category != "" && (err == nil || !strings.Contains(err.Error(), tc.category) || !strings.Contains(err.Error(), tc.path) || !strings.Contains(err.Error(), "DB_PATH")) {
				t.Fatalf("preflight %s: %v", tc.name, err)
			}
			contents, err := os.ReadFile(envFile)
			if err != nil || string(contents) != "existing service settings\n" {
				t.Fatal("preflight overwrote existing service settings")
			}
		})
	}
	// Absent WAL/SHM files are normal; their parent directory still needs write/search access.
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dbPath + suffix); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("WRM_TEST_DENIED_ACCESS", "")
	t.Setenv("WRM_TEST_DENIED_PATH", "")
	s, err := collectServiceSpec(serviceOptions{Name: "wrm_test", User: "service-test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := checkSystemdDatabaseAccess(s); err != nil {
		t.Fatal(err)
	}
	if calls, _ := os.ReadFile(os.Getenv("WRM_TEST_ACCESS_LOG")); !strings.Contains(string(calls), "test -x "+data) {
		t.Fatal("directory traversal permissions were not checked")
	}
}
