// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mediumFakeHost replaces the host for Docker installation tests: goos, root or not, a
// PATH with only the given stand-in commands (and env), and a log of what they were run with.
func mediumFakeHost(t *testing.T, goos string, euid int, scripts map[string]string) (bin, logFile string) {
	t.Helper()
	if os.PathSeparator == '\\' {
		t.Skip("shell fixture")
	}
	bin = t.TempDir()
	logFile = filepath.Join(t.TempDir(), "calls.log")
	for name, body := range scripts {
		p := filepath.Join(bin, name)
		writeFile(t, p, "#!/bin/sh\necho \""+name+" $*\" >> \"$WRM_TEST_LOG\"\n"+body+"\n")
		os.Chmod(p, 0o700)
	}
	if env, err := filepathLookReal("env"); err == nil {
		os.Symlink(env, filepath.Join(bin, "env"))
	}
	t.Setenv("PATH", bin)
	t.Setenv("WRM_TEST_LOG", logFile)
	t.Setenv("WRM_TEST_BIN", bin)
	t.Setenv("WRM_MEDIUM_DOCKER", "")
	t.Setenv("WRM_MEDIUM_RUNTIME", "docker")
	t.Setenv("INVOCATION_ID", "")
	oldOS, oldEUID, oldCont, oldSysd, oldWait, oldBase := mediumHostOS, mediumEUID, mediumInContainer, mediumSystemd, mediumDockerWait, mediumDesktopBase
	mediumHostOS = func() string { return goos }
	mediumEUID = func() int { return euid }
	mediumInContainer = func() bool { return false }
	mediumSystemd = func() bool { return true }
	mediumDockerWait = 3 * time.Second
	mediumDockerInfoReset()
	t.Cleanup(func() {
		mediumHostOS, mediumEUID, mediumInContainer, mediumSystemd, mediumDockerWait, mediumDesktopBase = oldOS, oldEUID, oldCont, oldSysd, oldWait, oldBase
		mediumDockerInfoReset()
	})
	return bin, logFile
}

func filepathLookReal(name string) (string, error) {
	for _, d := range []string{"/usr/bin", "/bin"} {
		if p := filepath.Join(d, name); mediumFileExists(p) {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

// the docker stand-in an installer writes: it answers like a Linux engine
const mediumFakeDockerInstall = `printf '#!/bin/sh\necho "docker $*" >> "$WRM_TEST_LOG"\n[ "$1" = version ] && echo linux\nexit 0\n' > "$WRM_TEST_BIN/docker"; /bin/chmod 700 "$WRM_TEST_BIN/docker"`

func mediumCalls(t *testing.T, logFile string) string {
	b, _ := os.ReadFile(logFile)
	return string(b)
}

func mediumStepNames(s mediumStatus) []string {
	var out []string
	for _, st := range s.Steps {
		out = append(out, st.Step)
	}
	return out
}

func TestV1266BackendIsDockerEverywhere(t *testing.T) {
	for _, c := range []struct {
		goos, configured string
		prepared, docker bool
		want             string
	}{
		{"linux", "", false, false, "docker"}, // WRM installs Docker
		{"linux", "", false, true, "docker"},
		{"linux", "", true, true, "docker"},  // both prepared: Docker
		{"linux", "", true, false, "native"}, // an existing host runtime keeps working
		{"darwin", "", false, false, "docker"},
		{"windows", "native", true, false, "docker"},
		{"linux", "native", false, true, "native"},
		{"freebsd", "", false, false, "native"}, // no Docker there
		{"freebsd", "docker", false, true, "docker"},
	} {
		if got := mediumSelectBackend(c.goos, c.configured, c.prepared, c.docker); got != c.want {
			t.Errorf("%+v: %s", c, got)
		}
	}
}

// Linux as root: the administrator allows it once, WRM installs Docker Engine from the
// distribution's packages, starts its service and records every step with its output.
func TestV1266LinuxInstallAfterConsent(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "medium.db"))
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "dk-admin", true)
	user := newTestUser(t, srv, "dk-user", false)
	_, calls := mediumFakeHost(t, "linux", 0, map[string]string{
		"apt-get":   `case "$*" in *"install -y docker.io"*) echo "Setting up docker.io"; ` + mediumFakeDockerInstall + `;; esac; exit 0`,
		"systemctl": "exit 0",
	})
	var st mediumStatus
	admin.jsonDo("GET", "/api/git/medium", nil, &st)
	if st.Docker == nil || st.Docker.Installed || !st.Docker.Auto || st.Docker.Method != "apt" || st.Docker.Product != "Docker Engine" || st.Docker.NeedsLicense || st.Docker.Consent != nil {
		t.Fatalf("status before: %+v", st.Docker)
	}
	// without consent a preparation stops with what to do
	if err := mediumInstallDocker(context.Background(), func(string) {}); err != errMediumDockerConsent {
		t.Fatalf("without consent: %v", err)
	}
	if code := user.jsonDo("POST", "/api/git/medium/docker", map[string]bool{"accept": true}, nil); code != 403 {
		t.Fatalf("a user installed Docker: %d", code)
	}
	if code := admin.jsonDo("POST", "/api/git/medium/docker", map[string]bool{"accept": true}, nil); code != 202 {
		t.Fatalf("install: %d", code)
	}
	s := mediumWaitIdle(t, admin.userID)
	if s.Error != "" {
		t.Fatalf("install failed: %s\n%s", s.Error, mediumCalls(t, calls))
	}
	steps := strings.Join(mediumStepNames(s), ",")
	if !strings.Contains(steps, "docker,docker_install,docker_start") {
		t.Fatalf("steps: %s", steps)
	}
	log := mediumCalls(t, calls)
	for _, want := range []string{"apt-get update", "apt-get install -y docker.io", "systemctl enable --now docker", "docker version"} {
		if !strings.Contains(log, want) {
			t.Fatalf("missing %q in:\n%s", want, log)
		}
	}
	if c := mediumDockerConsentGet(); c == nil || c.UserID != admin.userID || c.License {
		t.Fatalf("consent: %+v", c)
	}
	if auditCount(admin.userID, "git.medium_docker_consent") != 1 {
		t.Fatal("consent not audited")
	}
	rows := mediumActivity(admin.userID, 10, true)
	if len(rows) == 0 || rows[0].Op != "docker" || rows[0].Status != "ok" || rows[0].Output == nil || !strings.Contains(*rows[0].Output, "Setting up docker.io") {
		t.Fatalf("activity: %+v", rows)
	}
	// installed now: the status no longer asks
	admin.jsonDo("GET", "/api/git/medium", nil, &st)
	if st.Docker == nil || !st.Docker.Installed {
		t.Fatalf("status after: %+v", st.Docker)
	}
}

// Without administrator rights WRM says the one command to run; the policy can switch the
// installation off.
func TestV1266NoRightsAndPolicyOff(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "medium.db"))
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "dk-admin2", true)
	mediumFakeHost(t, "linux", 1000, map[string]string{"apt-get": "exit 0", "systemctl": "exit 0"})
	var st mediumStatus
	admin.jsonDo("GET", "/api/git/medium", nil, &st)
	if st.Docker == nil || st.Docker.Auto || !strings.Contains(st.Docker.Command, "-setup-docker") || !strings.Contains(st.Docker.Why, "without administrator rights") {
		t.Fatalf("no rights: %+v", st.Docker)
	}
	var v map[string]interface{}
	if code := admin.jsonDo("POST", "/api/git/medium/docker", map[string]bool{"accept": true}, &v); code != 409 || !strings.Contains(v["error"].(string), "-setup-docker") {
		t.Fatalf("install without rights: %d %v", code, v)
	}
	t.Setenv("WRM_MEDIUM_DOCKER_INSTALL", "0")
	if code := admin.jsonDo("POST", "/api/git/medium/docker", map[string]bool{"accept": true}, &v); code != 403 || !strings.Contains(v["error"].(string), "medium_docker_install") {
		t.Fatalf("policy off: %d %v", code, v)
	}
	if err := mediumInstallDocker(context.Background(), func(string) {}); err == nil || !strings.Contains(err.Error(), "medium_docker_install") {
		t.Fatalf("policy off in preparation: %v", err)
	}
}

func mediumFakeDownloads(t *testing.T) *[]string {
	got := &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = append(*got, r.URL.Path)
		w.Write([]byte("installer bytes"))
	}))
	t.Cleanup(srv.Close)
	mediumDesktopBase = srv.URL
	return got
}

// Windows: Docker Desktop is downloaded, its signature checked and installed quietly with
// the licence the administrator accepted in WRM; a bad signature stops it.
func TestV1266WindowsDesktop(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "medium.db"))
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "dk-win", true)
	_, calls := mediumFakeHost(t, "windows", -1, map[string]string{
		"powershell.exe": `case "$*" in *Get-AuthenticodeSignature*) [ -n "$WRM_TEST_BADSIG" ] && { echo NotSigned; exit 1; }; echo Valid; echo "CN=Docker Inc, O=Docker Inc, C=US";; *--accept-license*) echo "installer exit code 0"; ` + mediumFakeDockerInstall + `;; esac; exit 0`,
	})
	downloads := mediumFakeDownloads(t)
	var st mediumStatus
	admin.jsonDo("GET", "/api/git/medium", nil, &st)
	if st.Docker == nil || !st.Docker.Auto || !st.Docker.NeedsLicense || st.Docker.LicenseURL != mediumDockerLicenseURL {
		t.Fatalf("status: %+v", st.Docker)
	}
	var v map[string]interface{}
	if code := admin.jsonDo("POST", "/api/git/medium/docker", map[string]bool{"accept": true}, &v); code != 400 || !strings.Contains(v["error"].(string), "licence") {
		t.Fatalf("without the licence: %d %v", code, v)
	}
	t.Setenv("WRM_TEST_BADSIG", "1")
	if code := admin.jsonDo("POST", "/api/git/medium/docker", map[string]bool{"accept": true, "accept_license": true}, nil); code != 202 {
		t.Fatalf("install: %d", code)
	}
	s := mediumWaitIdle(t, admin.userID)
	if !strings.Contains(s.Error, "not signed by Docker Inc") || strings.Contains(mediumCalls(t, calls), "--accept-license") {
		t.Fatalf("bad signature: %s\n%s", s.Error, mediumCalls(t, calls))
	}
	t.Setenv("WRM_TEST_BADSIG", "")
	if code := admin.jsonDo("POST", "/api/git/medium/docker", map[string]bool{"accept": true, "accept_license": true}, nil); code != 202 {
		t.Fatalf("install: %d", code)
	}
	s = mediumWaitIdle(t, admin.userID)
	if s.Error != "" {
		t.Fatalf("install failed: %s\n%s", s.Error, mediumCalls(t, calls))
	}
	if steps := strings.Join(mediumStepNames(s), ","); steps != "docker,docker_download,docker_verify,docker_install" {
		t.Fatalf("steps: %s", steps)
	}
	if !strings.Contains(mediumCalls(t, calls), "'install','--user','--quiet','--accept-license','--backend=wsl-2'") || len(*downloads) != 2 || (*downloads)[0] != "/win/main/amd64/Docker%20Desktop%20Installer.exe" && (*downloads)[0] != "/win/main/amd64/Docker Desktop Installer.exe" {
		t.Fatalf("installer: %v\n%s", *downloads, mediumCalls(t, calls))
	}
	if c := mediumDockerConsentGet(); c == nil || !c.License || c.Product != "Docker Desktop" {
		t.Fatalf("consent: %+v", c)
	}
}

// macOS as root: the image is mounted, Docker Inc.'s signature checked and Docker Desktop
// installed for the user signed in on the Mac, with the accepted licence.
func TestV1266MacDesktop(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "medium.db"))
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "dk-mac", true)
	_, calls := mediumFakeHost(t, "darwin", 0, map[string]string{
		// attach puts an app with its installer at the mount point
		"hdiutil":  `case "$1" in attach) while [ "$1" != -mountpoint ]; do shift; done; /bin/mkdir -p "$2/Docker.app/Contents/MacOS"; /bin/ln -s "$WRM_TEST_BIN/install" "$2/Docker.app/Contents/MacOS/install";; esac; exit 0`,
		"install":  mediumFakeDockerInstall,
		"codesign": `case "$*" in *-dv*) echo "Authority=Developer ID Application: Docker Inc (` + mediumDockerAppleTeam + `)" >&2; echo "TeamIdentifier=` + mediumDockerAppleTeam + `" >&2;; esac; exit 0`,
		"stat":     "echo demo",
	})
	downloads := mediumFakeDownloads(t)
	if code := admin.jsonDo("POST", "/api/git/medium/docker", map[string]bool{"accept": true, "accept_license": true}, nil); code != 202 {
		t.Fatalf("install: %d", code)
	}
	s := mediumWaitIdle(t, admin.userID)
	if s.Error != "" {
		t.Fatalf("install failed: %s\n%s", s.Error, mediumCalls(t, calls))
	}
	log := mediumCalls(t, calls)
	if !strings.Contains(log, "install --accept-license --user=demo") || !strings.Contains(log, "codesign --verify --deep --strict") || len(*downloads) != 1 || (*downloads)[0] != "/mac/main/amd64/Docker.dmg" {
		t.Fatalf("mac install: %v\n%s", *downloads, log)
	}
}

// A Docker that runs but refuses WRM's account names the command that fixes it.
func TestV1266PermissionDenied(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "medium.db"))
	mediumFakeHost(t, "linux", 1000, map[string]string{
		"docker": `echo "permission denied while trying to connect to the Docker daemon socket" >&2; exit 1`,
	})
	err := mediumEnsureDocker(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "permission denied") || !strings.Contains(err.Error(), "-setup-docker") {
		t.Fatalf("permission denied: %v", err)
	}
}

// wrm -setup-docker (run as root) installs Docker Engine and starts it.
func TestV1266SetupDockerCommand(t *testing.T) {
	if os.Geteuid() != 0 || os.PathSeparator == '\\' {
		t.Skip("needs root")
	}
	_, calls := mediumFakeHost(t, "linux", 0, map[string]string{
		"apt-get":   `case "$*" in *install*) ` + mediumFakeDockerInstall + `;; esac; exit 0`,
		"systemctl": "exit 0",
	})
	var out strings.Builder
	if code := runSetupDocker("root", "wrm-test-none", &out); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if !strings.Contains(mediumCalls(t, calls), "apt-get install -y docker.io") || !strings.Contains(out.String(), "Done") {
		t.Fatalf("setup-docker: %s\n%s", out.String(), mediumCalls(t, calls))
	}
}
