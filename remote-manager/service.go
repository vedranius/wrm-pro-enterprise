// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── RUN AS A SERVICE ────────────────────────────────
//
// WRM installs itself as a service that starts at boot: a systemd unit (Linux), a launchd
// LaunchDaemon or LaunchAgent (macOS), an rc.d script (FreeBSD, OpenBSD) or a Windows service
// (SCM, service_windows.go). The OS-specific installers are in service_unix.go and
// service_windows.go; this file holds what every platform shares:
//   - the console prompt on an interactive start and the remembered answer (<DB_PATH>.service.json);
//   - the environment file (<data dir>/<name>.env) that gives the service the PORT, DB_PATH,
//     key files and WRM_* settings of the interactive start;
//   - the service files themselves (pure functions, tested against golden files);
//   - versions: parsing, comparing, and the newest valid wrm-pro-v<semver>-<os>-<arch>[.exe]
//     next to the binary, which a service start switches to;
//   - restarts (self-update, "Restart to v…"): through the service manager when one supervises
//     WRM, otherwise by running the binary again in place.

const (
	serviceDisplayName = "Web Remote Manager PRO"
	serviceDescription = "Web Remote Manager PRO: SSH, SFTP, remote desktop and server management in the browser."
	serviceDocsURL     = "https://github.com/vedranius/wrm-pro-enterprise"
	// exitRestart asks a supervising service manager to start WRM again (EX_TEMPFAIL).
	exitRestart = 75
)

// Service managers (the value of -service, set in the service definitions).
const (
	svcSystemd    = "systemd"
	svcLaunchd    = "launchd"
	svcRCD        = "rcd"         // FreeBSD rc.d with daemon(8) -r: supervised
	svcRCDOpenBSD = "rcd-openbsd" // OpenBSD rc.d: not supervised
	svcWindows    = "windows"
)

var serviceNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,39}$`)

// serviceSupervised reports whether the service manager starts WRM again after it exits
// with exitRestart.
func serviceSupervised(mode string) bool {
	switch mode {
	case svcSystemd, svcLaunchd, svcRCD, svcWindows:
		return true
	}
	return false
}

// serviceSpec describes one installation of the service.
type serviceSpec struct {
	Name    string   // service / unit name, e.g. "wrm"
	Exe     string   // absolute path of the binary
	WorkDir string   // working directory of the interactive start (relative paths resolve the same)
	DBPath  string   // database file used by this installation
	DataDir string   // directory of the database
	EnvFile string   // KEY=value lines: PORT, DB_PATH, key files, WRM_* settings
	User    string   // Unix account of the service ("" = root / LocalSystem)
	Scope   string   // "system" or "user" (macOS LaunchAgent)
	RWPaths []string // writable paths under systemd's ProtectSystem=strict
	LogFile string   // launchd / rc.d / Windows service log
	LowPort bool     // listens on a port below 1024 (systemd gets CAP_NET_BIND_SERVICE)
}

func (s serviceSpec) launchdLabel() string { return "local." + s.Name }

// serviceArgs are the arguments the service manager passes to the binary. systemd reads the
// environment file and sets the working directory itself.
func serviceArgs(s serviceSpec, mode string) []string {
	switch mode {
	case svcSystemd:
		return []string{"-service=" + mode}
	case svcWindows:
		return []string{"-service=" + mode, "-service-name", s.Name, "-env-file", s.EnvFile, "-workdir", s.WorkDir}
	}
	return []string{"-service=" + mode, "-env-file", s.EnvFile, "-workdir", s.WorkDir}
}

// ─── service files (golden-tested) ───────────────────

func systemdQuote(s string) string {
	// ExecStart expands $VAR even inside quotes; $$ is its literal dollar escape.
	return systemdPathWord(strings.ReplaceAll(s, "$", "$$"))
}

func systemdPathWord(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'\\$%;") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`)
	return `"` + r.Replace(s) + `"`
}

func systemdUnit(s serviceSpec) string {
	var b strings.Builder
	w := func(format string, a ...interface{}) { fmt.Fprintf(&b, format+"\n", a...) }
	w("# %s service, written by %s -install-service.", serviceDisplayName, filepath.Base(s.Exe))
	w("# Settings: %s (edit, then: systemctl restart %s)", s.EnvFile, s.Name)
	w("[Unit]")
	w("Description=%s", serviceDisplayName)
	w("Documentation=%s", serviceDocsURL)
	w("After=network-online.target")
	w("Wants=network-online.target")
	w("StartLimitIntervalSec=60")
	w("StartLimitBurst=5")
	w("")
	w("[Service]")
	w("Type=simple")
	if s.User != "" {
		w("User=%s", s.User)
	}
	// These directives take the entire value as one path; quote characters become
	// part of the path. Only specifiers expand, so escape % and retain literal $.
	w("WorkingDirectory=%s", strings.ReplaceAll(s.WorkDir, "%", "%%"))
	w("EnvironmentFile=%s", strings.ReplaceAll(s.EnvFile, "%", "%%"))
	args := []string{systemdQuote(s.Exe)}
	for _, a := range serviceArgs(s, svcSystemd) {
		args = append(args, systemdQuote(a))
	}
	w("ExecStart=%s", strings.Join(args, " "))
	w("Restart=on-failure")
	w("RestartSec=3")
	w("# Exit code %d: WRM asks to be started again (self-update, newer binary in its folder).", exitRestart)
	w("SuccessExitStatus=%d", exitRestart)
	w("RestartForceExitStatus=%d", exitRestart)
	w("UMask=0077")
	w("NoNewPrivileges=true")
	w("ProtectSystem=strict")
	w("ProtectHome=read-only")
	paths := make([]string, 0, len(s.RWPaths))
	for _, p := range s.RWPaths {
		paths = append(paths, systemdPathWord(p))
	}
	w("ReadWritePaths=%s", strings.Join(paths, " "))
	w("PrivateTmp=true")
	w("ProtectKernelTunables=true")
	w("ProtectKernelModules=true")
	w("ProtectControlGroups=true")
	w("RestrictSUIDSGID=true")
	w("LockPersonality=true")
	if s.LowPort && s.User != "" {
		w("AmbientCapabilities=CAP_NET_BIND_SERVICE")
	}
	w("LimitNOFILE=65536")
	w("")
	w("[Install]")
	w("WantedBy=multi-user.target")
	return b.String()
}

func xmlText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func launchdPlist(s serviceSpec) string {
	var b strings.Builder
	w := func(line string) { b.WriteString(line + "\n") }
	str := func(v string) string { return "<string>" + xmlText(v) + "</string>" }
	w(`<?xml version="1.0" encoding="UTF-8"?>`)
	w(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`)
	w(`<!-- ` + serviceDisplayName + ` service, written by -install-service. Settings: ` + xmlText(s.EnvFile) + ` -->`)
	w(`<plist version="1.0">`)
	w(`<dict>`)
	w("\t<key>Label</key>")
	w("\t" + str(s.launchdLabel()))
	w("\t<key>ProgramArguments</key>")
	w("\t<array>")
	w("\t\t" + str(s.Exe))
	for _, a := range serviceArgs(s, svcLaunchd) {
		w("\t\t" + str(a))
	}
	w("\t</array>")
	w("\t<key>WorkingDirectory</key>")
	w("\t" + str(s.WorkDir))
	if s.Scope != "user" && s.User != "" {
		w("\t<key>UserName</key>")
		w("\t" + str(s.User))
	}
	w("\t<key>RunAtLoad</key>")
	w("\t<true/>")
	w("\t<key>KeepAlive</key>")
	w("\t<dict>")
	w("\t\t<key>SuccessfulExit</key>")
	w("\t\t<false/>")
	w("\t</dict>")
	w("\t<key>ThrottleInterval</key>")
	w("\t<integer>5</integer>")
	w("\t<key>Umask</key>")
	w("\t<integer>63</integer>")
	w("\t<key>StandardOutPath</key>")
	w("\t" + str(s.LogFile))
	w("\t<key>StandardErrorPath</key>")
	w("\t" + str(s.LogFile))
	w(`</dict>`)
	w(`</plist>`)
	return b.String()
}

func shQuote(s string) string {
	if s != "" && regexp.MustCompile(`^[A-Za-z0-9_./:=@%+-]+$`).MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func shJoin(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = shQuote(a)
	}
	return strings.Join(q, " ")
}

// freebsdRCScript runs WRM under daemon(8), which restarts it when it exits (-r).
func freebsdRCScript(s serviceSpec) string {
	user := s.User
	if user == "" {
		user = "root"
	}
	prog := shJoin(append([]string{s.Exe}, serviceArgs(s, svcRCD)...))
	return fmt.Sprintf(`#!/bin/sh
#
# PROVIDE: %[1]s
# REQUIRE: LOGIN NETWORKING
# KEYWORD: shutdown
#
# %[2]s service, written by -install-service.
# Settings: %[3]s
# Enable: sysrc %[1]s_enable=YES

. /etc/rc.subr

name="%[1]s"
rcvar="%[1]s_enable"

load_rc_config $name

: ${%[1]s_enable:="NO"}
: ${%[1]s_user:="%[4]s"}

pidfile="/var/run/${name}.pid"
command="/usr/sbin/daemon"
command_args="-r -R 3 -P ${pidfile} -u ${%[1]s_user} -o %[5]s -- %[6]s"

run_rc_command "$1"
`, s.Name, serviceDisplayName, s.EnvFile, user, shQuote(s.LogFile), strings.ReplaceAll(prog, `"`, `\"`))
}

// openbsdRCScript uses rc.subr(8); OpenBSD has no supervisor, so WRM restarts itself in place.
func openbsdRCScript(s serviceSpec) string {
	user := s.User
	if user == "" {
		user = "root"
	}
	return fmt.Sprintf(`#!/bin/ksh
#
# %[1]s service, written by -install-service.
# Settings: %[2]s
# Enable: rcctl enable %[3]s

daemon=%[4]s
daemon_flags=%[5]s
daemon_user=%[6]s
daemon_logger="daemon.info"

. /etc/rc.d/rc.subr

rc_bg=YES
rc_reload=NO

rc_cmd $1
`, serviceDisplayName, s.EnvFile, s.Name, shQuote(s.Exe), shQuote(shJoin(serviceArgs(s, svcRCDOpenBSD))), shQuote(user))
}

// windowsCommandLine quotes the binary path and arguments for the SCM (CommandLineToArgvW rules).
func windowsCommandLine(exe string, args []string) string {
	q := func(s string) string {
		if s != "" && !strings.ContainsAny(s, " \t\"") {
			return s
		}
		var b strings.Builder
		b.WriteByte('"')
		slashes := 0
		for _, c := range s {
			switch c {
			case '\\':
				slashes++
			case '"':
				b.WriteString(strings.Repeat(`\`, slashes*2+1))
				b.WriteRune(c)
				slashes = 0
				continue
			default:
				b.WriteString(strings.Repeat(`\`, slashes))
				slashes = 0
				b.WriteRune(c)
				continue
			}
		}
		b.WriteString(strings.Repeat(`\`, slashes*2))
		b.WriteByte('"')
		return b.String()
	}
	parts := []string{q(exe)}
	for _, a := range args {
		parts = append(parts, q(a))
	}
	return strings.Join(parts, " ")
}

// Windows service recovery: restart after a failure (also a non-crash exit code, so that
// exitRestart starts WRM again).
var windowsRecovery = struct {
	Delay       time.Duration
	Actions     int
	ResetPeriod time.Duration
}{3 * time.Second, 3, time.Hour}

// windowsServiceConfig is the configuration the Windows installer registers with the SCM, as
// text (shown by -install-service and compared with a golden file).
func windowsServiceConfig(s serviceSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Name: %s\n", s.Name)
	fmt.Fprintf(&b, "DisplayName: %s\n", serviceDisplayName)
	fmt.Fprintf(&b, "Description: %s\n", serviceDescription)
	fmt.Fprintf(&b, "StartType: automatic (delayed start)\n")
	fmt.Fprintf(&b, "Account: LocalSystem\n")
	fmt.Fprintf(&b, "CommandLine: %s\n", windowsCommandLine(s.Exe, serviceArgs(s, svcWindows)))
	fmt.Fprintf(&b, "Recovery: restart after %s (%d times), reset after %s, also when WRM exits with an error code\n",
		windowsRecovery.Delay, windowsRecovery.Actions, windowsRecovery.ResetPeriod)
	fmt.Fprintf(&b, "Log: %s\n", s.LogFile)
	return b.String()
}

// ─── environment file ────────────────────────────────

// serviceEnvNames are the variables WRM reads; every WRM_* variable is copied as well.
var serviceEnvNames = []string{"PORT", "LISTEN_ADDR", "DB_PATH", "ENCRYPTION_KEY", "ENCRYPTION_KEY_FILE",
	"HTTPS_CERT_FILE", "HTTPS_KEY_FILE", "HTTPS_SELF_SIGNED", "HTTPS_SELF_SIGNED_HOSTS",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "TZ"}

func serviceEnvironment(dbPath string) map[string]string {
	env := map[string]string{}
	names := append([]string{}, serviceEnvNames...)
	for _, s := range settingSpecs {
		if s.Alias != "" {
			names = append(names, s.Alias)
		}
	}
	for _, n := range names {
		if v, ok := os.LookupEnv(n); ok && v != "" {
			env[n] = v
		}
	}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(k, "WRM_") && v != "" && k != "WRM_SWITCHED_FROM" {
			env[k] = v
		}
	}
	env["DB_PATH"] = dbPath
	if env["PORT"] == "" && env["LISTEN_ADDR"] == "" {
		env["PORT"] = "8080"
	}
	return env
}

func envFileQuote(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\"'\\#$`") {
		return v
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}

func formatEnvFile(name string, env map[string]string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	fmt.Fprintf(&b, "# Environment of the %q WRM service (written by -install-service).\n", name)
	b.WriteString("# Edit it and restart the service to change ports, paths or WRM_* settings.\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, envFileQuote(env[k]))
	}
	return b.String()
}

// parseEnvFile reads KEY=value lines (comments, "export ", double or single quotes).
func parseEnvFile(data string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" || strings.ContainsAny(k, " \t") {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(v[1 : len(v)-1])
		} else if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
			v = v[1 : len(v)-1]
		}
		out[k] = v
	}
	return out
}

// loadEnvFile sets the variables of an environment file; variables already set win.
func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for k, v := range parseEnvFile(string(data)) {
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
	return nil
}

// ─── versions ────────────────────────────────────────

type semVersion struct {
	Major, Minor, Patch int
	Hotfix              string
	Pre                 string
}

var semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)([a-z]?)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

func parseVersion(s string) (semVersion, bool) {
	m := semverRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return semVersion{}, false
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	c, _ := strconv.Atoi(m[3])
	return semVersion{Major: a, Minor: b, Patch: c, Hotfix: m[4], Pre: m[5]}, true
}

func (v semVersion) core() string {
	return fmt.Sprintf("v%d.%d.%d%s", v.Major, v.Minor, v.Patch, v.Hotfix)
}

// compareSemver orders semantic versions: -1 (a older), 0, 1. A pre-release (or a branch
// build, v11.7.0-1a2b3c4) comes before its release. Unparsable versions compare equal.
func compareSemver(a, b string) int {
	va, ok1 := parseVersion(a)
	vb, ok2 := parseVersion(b)
	if !ok1 || !ok2 {
		return 0
	}
	for _, d := range [][2]int{{va.Major, vb.Major}, {va.Minor, vb.Minor}, {va.Patch, vb.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	if va.Hotfix != vb.Hotfix {
		if va.Hotfix < vb.Hotfix {
			return -1
		}
		return 1
	}
	switch {
	case va.Pre == vb.Pre:
		return 0
	case va.Pre == "":
		return 1
	case vb.Pre == "":
		return -1
	}
	pa, pb := strings.Split(va.Pre, "."), strings.Split(vb.Pre, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, e1 := strconv.Atoi(pa[i])
		nb, e2 := strconv.Atoi(pb[i])
		switch {
		case e1 == nil && e2 == nil:
			if na != nb {
				return map[bool]int{true: -1, false: 1}[na < nb]
			}
		case e1 == nil:
			return -1
		case e2 == nil:
			return 1
		case pa[i] != pb[i]:
			return map[bool]int{true: -1, false: 1}[pa[i] < pb[i]]
		}
	}
	switch {
	case len(pa) < len(pb):
		return -1
	case len(pa) > len(pb):
		return 1
	}
	return 0
}

// ─── release file names and the newest binary in the folder ──

var binaryNameRe = regexp.MustCompile(`^wrm-pro-(v\d+\.\d+\.\d+[a-z]?)-([a-z0-9]+)-([a-z0-9]+)(\.exe)?$`)

// buildGOARM is the GOARM the binary was built with ("" when unknown).
var buildGOARM = func() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "GOARM" {
				return strings.TrimSuffix(strings.TrimSuffix(s.Value, ",softfloat"), ",hardfloat")
			}
		}
	}
	return ""
}()

// platformLabels are the release labels (see .github/workflows/build.yml) this binary can
// replace itself with; the first one is the label of its own release asset.
func platformLabels(goos, goarch, goarm string) []string {
	arch := goarch
	if goarch == "arm" {
		switch goarm {
		case "6":
			return []string{goos + "-armv6"}
		case "7":
			return []string{goos + "-armv7", goos + "-armv6"}
		}
		return []string{goos + "-armv7", goos + "-armv6"}
	}
	labels := []string{goos + "-" + arch}
	if goos == "darwin" {
		labels = append(labels, "darwin-universal")
	}
	return labels
}

func binaryExt(goos string) string {
	if goos == "windows" {
		return ".exe"
	}
	return ""
}

// releaseAssetName is the release file of a version for this platform.
func releaseAssetName(version, label string) string {
	goos, _, _ := strings.Cut(label, "-")
	return "wrm-pro-" + version + "-" + label + binaryExt(goos)
}

// probeBinaryVersion runs a binary with -version (a sanity check before WRM switches to it).
var probeBinaryVersion = func(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-version")
	cmd.Env = append(os.Environ(), "WRM_VERSION_PROBE=1")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(line), nil
}

// versionAnswerMatches checks the -version answer of a binary against the expected version
// (a branch build answers v11.7.0-1a2b3c4 for v11.7.0).
func versionAnswerMatches(answer, want string) bool {
	va, ok1 := parseVersion(answer)
	vw, ok2 := parseVersion(want)
	return ok1 && ok2 && va.core() == vw.core() && (vw.Pre == "" || va.Pre == vw.Pre)
}

type localBinary struct {
	Path    string `json:"-"`
	File    string `json:"file"`
	Version string `json:"version"`
}

// findNewerBinary returns the newest wrm-pro-v<semver>-<os>-<arch>[.exe] in dir that is newer
// than current and answers -version with the version in its name. Files of other platforms,
// older or equal versions, the running binary itself and files that do not answer correctly
// are ignored.
func findNewerBinary(dir, current, self string, labels []string) *localBinary {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	ext := binaryExt(strings.SplitN(labels[0], "-", 2)[0])
	var found []localBinary
	for _, e := range entries {
		m := binaryNameRe.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() || m[4] != ext || !slices.Contains(labels, m[2]+"-"+m[3]) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if sameFile(p, self) || compareSemver(m[1], current) <= 0 {
			continue
		}
		found = append(found, localBinary{Path: p, File: e.Name(), Version: m[1]})
	}
	sort.SliceStable(found, func(i, j int) bool { return compareSemver(found[i].Version, found[j].Version) > 0 })
	for _, f := range found {
		makeExecutable(f.Path)
		answer, err := probeBinaryVersion(f.Path)
		if err == nil && versionAnswerMatches(answer, f.Version) {
			f := f
			return &f
		}
	}
	return nil
}

func sameFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	fa, err1 := os.Stat(a)
	fb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(fa, fb)
}

// makeExecutable adds the owner's execute bit to a copied-in binary (Unix).
func makeExecutable(path string) {
	if runtime.GOOS == "windows" {
		return
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o100 == 0 {
		os.Chmod(path, fi.Mode()|0o100)
	}
}

// selfExecutable is the path of the running binary (a variable for tests).
var selfExecutable = func() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return filepath.Abs(p)
}

// newerLocalBinary is the newest valid binary next to the running one, cached for a minute
// (the UI asks often; probing runs the files).
var localBinaryCache = struct {
	sync.Mutex
	at  time.Time
	bin *localBinary
}{}

func newerLocalBinary() *localBinary {
	localBinaryCache.Lock()
	defer localBinaryCache.Unlock()
	if time.Since(localBinaryCache.at) < time.Minute {
		return localBinaryCache.bin
	}
	exe, err := selfExecutable()
	if err != nil {
		return nil
	}
	localBinaryCache.bin = findNewerBinary(filepath.Dir(exe), AppVersion, exe, platformLabels(runtime.GOOS, runtime.GOARCH, buildGOARM))
	localBinaryCache.at = time.Now()
	return localBinaryCache.bin
}

func forgetLocalBinary() {
	localBinaryCache.Lock()
	localBinaryCache.at = time.Time{}
	localBinaryCache.Unlock()
}

// ─── restarts ────────────────────────────────────────

// serviceMode is the -service value ("" for an interactive start); currentServiceName the
// -service-name of this installation.
var (
	serviceMode        string
	currentServiceName = "wrm"
)

// errSwitchByRestart: the service definition now names a newer binary; exit with exitRestart.
var errSwitchByRestart = errors.New("restart to switch the binary")

var restartState = struct {
	sync.Mutex
	requested bool
	target    string
	ch        chan struct{}
}{ch: make(chan struct{})}

// restartHook replaces the real restart in tests.
var restartHook func(target string)

// requestRestart stops the server and starts WRM again: target "" starts the same file.
func requestRestart(target string) {
	if restartHook != nil {
		restartHook(target)
		return
	}
	restartState.Lock()
	defer restartState.Unlock()
	if restartState.requested {
		return
	}
	restartState.requested, restartState.target = true, target
	close(restartState.ch)
}

func restartRequested() (bool, string) {
	restartState.Lock()
	defer restartState.Unlock()
	return restartState.requested, restartState.target
}

// finishRestart runs after the server stopped: a supervised service exits with exitRestart
// (the manager starts it again, which switches to a newer binary in the folder); otherwise
// the process is replaced in place (Unix) or a new one is started (Windows).
func finishRestart(target string) int {
	if serviceSupervised(serviceMode) {
		fmt.Fprintf(os.Stderr, "Restarting through the service manager…\n")
		return exitRestart
	}
	exe, err := selfExecutable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "restart: %v\n", err)
		return 1
	}
	if target == "" {
		target = exe
	}
	if err := execBinary(target, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "restart with %s failed: %v\n", target, err)
		return 1
	}
	return 0
}

// switchToNewerBinary is called when a service starts: a newer valid binary in the folder
// takes over with the same arguments and environment (Unix: in place, so it does not return).
// True means: exit with exitRestart, the service manager starts the new binary (Windows).
func switchToNewerBinary() bool {
	if os.Getenv("WRM_SWITCHED_FROM") != "" {
		return false
	}
	nb := newerLocalBinary()
	if nb == nil {
		return false
	}
	exe, _ := selfExecutable()
	log.Printf("A newer binary is in the folder: %s (%s); switching to it.", nb.File, nb.Version)
	os.Setenv("WRM_SWITCHED_FROM", exe)
	err := switchServiceBinary(nb.Path)
	if errors.Is(err, errSwitchByRestart) {
		return true
	}
	log.Printf("Could not switch to %s: %v", nb.File, err)
	os.Unsetenv("WRM_SWITCHED_FROM")
	return false
}

// ─── install / uninstall / status (shared part) ──────

type serviceOptions struct {
	Name  string
	Scope string // macOS: system | user
	User  string // Unix account
}

// errNeedsAdmin: the installer needs root / administrator rights.
type errNeedsAdmin struct{ hint string }

func (e errNeedsAdmin) Error() string { return e.hint }

// buildServiceSpec collects the paths and writes the environment file of the service.
func buildServiceSpec(o serviceOptions) (serviceSpec, error) {
	s, err := collectServiceSpec(o)
	if err != nil {
		return serviceSpec{}, err
	}
	return s, writeEnvFile(s, serviceEnvironment(s.DBPath))
}

// collectServiceSpec leaves existing service settings intact until preflight checks pass.
func collectServiceSpec(o serviceOptions) (serviceSpec, error) {
	if !serviceNameRe.MatchString(o.Name) {
		return serviceSpec{}, fmt.Errorf("invalid service name %q (letters, digits and _, starting with a letter)", o.Name)
	}
	if o.Scope == "" {
		o.Scope = "system"
	}
	if o.Scope != "system" && o.Scope != "user" {
		return serviceSpec{}, fmt.Errorf("-service-scope must be system or user")
	}
	exe, err := selfExecutable()
	if err != nil {
		return serviceSpec{}, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return serviceSpec{}, err
	}
	dbPath, err := filepath.Abs(resolveDBPath())
	if err != nil {
		return serviceSpec{}, err
	}
	dataDir := filepath.Dir(dbPath)
	if runtime.GOOS == "linux" {
		for _, p := range []string{wd, dbPath, exe} {
			if strings.ContainsAny(p, "\n\r") || strings.TrimSpace(p) != p || strings.HasSuffix(p, `\`) {
				return serviceSpec{}, fmt.Errorf("service path %q contains a line break, trailing whitespace or a trailing backslash that systemd cannot represent safely", p)
			}
		}
	}
	s := serviceSpec{Name: o.Name, Exe: exe, WorkDir: wd, DBPath: dbPath, DataDir: dataDir, User: o.User, Scope: o.Scope,
		EnvFile: filepath.Join(dataDir, o.Name+".env"), LogFile: filepath.Join(dataDir, o.Name+"-service.log")}
	for _, p := range []string{wd, dataDir, filepath.Dir(exe)} {
		if !slices.Contains(s.RWPaths, p) {
			s.RWPaths = append(s.RWPaths, p)
		}
	}
	env := serviceEnvironment(dbPath)
	if p, err := strconv.Atoi(env["PORT"]); err == nil && p > 0 && p < 1024 {
		s.LowPort = true
	}
	if _, port, err := splitListen(env["LISTEN_ADDR"]); err == nil && port > 0 && port < 1024 {
		s.LowPort = true
	}
	return s, nil
}

func splitListen(addr string) (string, int, error) {
	if addr == "" {
		return "", 0, errors.New("empty")
	}
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return "", 0, errors.New("no port")
	}
	p, err := strconv.Atoi(addr[i+1:])
	return addr[:i], p, err
}

func writeEnvFile(s serviceSpec, env map[string]string) error {
	if err := os.WriteFile(s.EnvFile, []byte(formatEnvFile(s.Name, env)), 0o600); err != nil {
		return fmt.Errorf("environment file: %v", err)
	}
	return os.Chmod(s.EnvFile, 0o600)
}

// ─── the console prompt ──────────────────────────────

type servicePromptMemory struct {
	Answer string `json:"answer"` // yes | never
	At     string `json:"at"`
}

func servicePromptFile() string { return resolveDBPath() + ".service.json" }

func rememberServiceAnswer(answer string) {
	b, _ := json.MarshalIndent(servicePromptMemory{Answer: answer, At: time.Now().UTC().Format(time.RFC3339)}, "", "  ")
	os.WriteFile(servicePromptFile(), append(b, '\n'), 0o600)
}

func serviceAnswerRemembered() bool {
	data, err := os.ReadFile(servicePromptFile())
	if err != nil {
		return false
	}
	var m servicePromptMemory
	return json.Unmarshal(data, &m) == nil && (m.Answer == "yes" || m.Answer == "never")
}

func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// shouldPromptService: an interactive start (a terminal on stdin), not a service, not in a
// container, a platform with a service manager, no remembered answer and nothing installed.
func shouldPromptService(noPrompt bool, name string) bool {
	if noPrompt || serviceMode != "" || os.Getenv("WRM_NO_SERVICE_PROMPT") != "" || inContainer() || !stdinIsTerminal() {
		return false
	}
	if !servicePlatformSupported() || serviceAnswerRemembered() {
		return false
	}
	return !serviceInstalled(name)
}

// readAnswer reads one line from stdin, or "" after the timeout.
func readAnswer(r io.Reader, timeout time.Duration) string {
	ch := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(r).ReadString('\n')
		ch <- strings.ToLower(strings.TrimSpace(line))
	}()
	select {
	case a := <-ch:
		return a
	case <-time.After(timeout):
		fmt.Println()
		return ""
	}
}

// promptService asks whether to install the service. It returns true when WRM was installed
// and started as a service, so this interactive process should exit.
func promptService(o serviceOptions) bool {
	fmt.Printf("\nWRM can run as a service that starts at boot (%s).\n", serviceManagerName(o.Scope))
	fmt.Print("Install it as a service now? [y]es / [n]o / [d]on't ask again (default: no): ")
	switch a := readAnswer(os.Stdin, 2*time.Minute); {
	case a == "d" || strings.HasPrefix(a, "don") || a == "never":
		rememberServiceAnswer("never")
		fmt.Println("OK, WRM will not ask again. Install the service later with -install-service.")
		return false
	case a != "y" && a != "yes":
		fmt.Println("Not now. (Install later with -install-service, or start with -no-service-prompt.)")
		return false
	}
	if runtime.GOOS == "darwin" && o.Scope == "" {
		fmt.Print("[s]ystem LaunchDaemon (starts at boot, needs sudo) or [u]ser LaunchAgent (starts at login)? (default: system): ")
		if a := readAnswer(os.Stdin, 2*time.Minute); a == "u" || a == "user" {
			o.Scope = "user"
		} else {
			o.Scope = "system"
		}
	}
	err := installService(o)
	var na errNeedsAdmin
	if errors.As(err, &na) {
		err = installElevated(o)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "The service was not installed: %v\nWRM continues in this window.\n\n", err)
		return false
	}
	rememberServiceAnswer("yes")
	fmt.Println("WRM now runs as a service; this window can be closed.")
	return true
}

// runServiceCommand handles -install-service, -uninstall-service and -service-status.
func runServiceCommand(install, uninstall, status bool, o serviceOptions) int {
	var err error
	switch {
	case install:
		err = installService(o)
		if err == nil {
			rememberServiceAnswer("yes")
		}
	case uninstall:
		err = uninstallService(o)
	case status:
		fmt.Print(serviceStatusText(o))
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

// openServiceLog: a Windows service has no console, so its log goes to
// <data dir>/<name>-service.log (launchd and rc.d redirect the output, systemd keeps it in the journal).
func openServiceLog() {
	if serviceMode != svcWindows {
		return
	}
	dbPath, err := filepath.Abs(resolveDBPath())
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(filepath.Dir(dbPath), currentServiceName+"-service.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	log.SetOutput(f)
}
