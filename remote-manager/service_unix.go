//go:build !windows

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// Service installers for Linux (systemd), macOS (launchd), FreeBSD and OpenBSD (rc.d).

func systemdAvailable() bool {
	if fi, err := os.Stat("/run/systemd/system"); err != nil || !fi.IsDir() {
		return false
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}

func servicePlatformSupported() bool {
	switch runtime.GOOS {
	case "linux":
		return systemdAvailable()
	case "darwin", "freebsd", "openbsd":
		return true
	}
	return false
}

func serviceManagerName(scope string) string {
	switch runtime.GOOS {
	case "linux":
		return "a systemd unit"
	case "darwin":
		if scope == "user" {
			return "a launchd LaunchAgent"
		}
		return "launchd"
	case "freebsd", "openbsd":
		return "an rc.d script"
	}
	return "no service manager"
}

func serviceFilePath(o serviceOptions) string {
	switch runtime.GOOS {
	case "linux":
		return "/etc/systemd/system/" + o.Name + ".service"
	case "darwin":
		label := serviceSpec{Name: o.Name}.launchdLabel()
		if o.Scope == "user" {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
		}
		return "/Library/LaunchDaemons/" + label + ".plist"
	case "freebsd":
		return "/usr/local/etc/rc.d/" + o.Name
	case "openbsd":
		return "/etc/rc.d/" + o.Name
	}
	return ""
}

func serviceInstalled(name string) bool {
	for _, scope := range []string{"system", "user"} {
		if p := serviceFilePath(serviceOptions{Name: name, Scope: scope}); p != "" {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
	}
	return false
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s == "" {
			s = err.Error()
		}
		return s, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), s)
	}
	return s, nil
}

// defaultServiceUser: the user who ran sudo, else the current user (root gives "").
func defaultServiceUser() string {
	if os.Geteuid() == 0 {
		if u := os.Getenv("SUDO_USER"); u != "" && u != "root" {
			return u
		}
		if u := os.Getenv("DOAS_USER"); u != "" && u != "root" {
			return u
		}
		return ""
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

func chownToUser(path, name string) {
	if os.Geteuid() != 0 || name == "" {
		return
	}
	u, err := user.Lookup(name)
	if err != nil {
		return
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	os.Chown(path, uid, gid)
}

// checkSystemdDatabaseAccess catches a common service-install failure: WRM was previously
// started as root, leaving a root-owned SQLite database that the unprivileged service user
// can read but cannot migrate or write. runuser performs the check with the service account's
// real permissions (including ACLs); if it is unavailable, systemd's startup diagnostics remain
// the fallback.
func checkSystemdDatabaseAccess(s serviceSpec) error {
	if runtime.GOOS != "linux" || s.User == "" {
		return nil
	}
	runuser, err := exec.LookPath("runuser")
	if err != nil {
		return nil
	}
	for _, path := range []string{s.DBPath, s.DBPath + "-wal", s.DBPath + "-shm"} {
		if _, err := os.Stat(path); err == nil {
			for _, access := range []string{"-r", "-w"} {
				if _, err := run(runuser, "-u", s.User, "--", "test", access, path); err != nil {
					return fmt.Errorf("database file %q is not readable and writable by service user %q; set DB_PATH to the intended database or grant that user read/write access to the database files", path, s.User)
				}
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("checking database file %q: %w", path, err)
		}
	}
	for _, access := range []string{"-w", "-x"} {
		if _, err := run(runuser, "-u", s.User, "--", "test", access, s.DataDir); err != nil {
			return fmt.Errorf("database directory %q is not writable and searchable by service user %q; set DB_PATH to a writable location or grant that user write and search access to the directory", s.DataDir, s.User)
		}
	}
	return nil
}

func needRoot(o serviceOptions) error {
	if os.Geteuid() == 0 || (runtime.GOOS == "darwin" && o.Scope == "user") {
		return nil
	}
	return errNeedsAdmin{hint: "installing a system service needs root: run  sudo " + filepath.Base(os.Args[0]) + " -install-service"}
}

func startSystemdService(s serviceSpec) error {
	unit := s.Name + ".service"
	if _, err := run("systemctl", "enable", unit); err != nil {
		return err
	}
	// A corrected installation may still be rate-limited by the old unit's failures.
	// Restart also applies the new ExecStart when the old service is already running.
	run("systemctl", "reset-failed", unit)
	if _, err := run("systemctl", "restart", unit); err != nil {
		status, _ := run("systemctl", "status", "--no-pager", "--full", unit)
		journal, _ := run("journalctl", "-u", unit, "-n", "40", "--no-pager", "--output", "short-iso")
		var detail strings.Builder
		fmt.Fprintf(&detail, "%v", err)
		if status != "" {
			fmt.Fprintf(&detail, "\n\nService status:\n%s", status)
		}
		if journal != "" {
			fmt.Fprintf(&detail, "\n\nRecent service log:\n%s", journal)
			if strings.Contains(strings.ToLower(journal), "readonly database") {
				fmt.Fprintf(&detail, "\n\nSQLite could not write the database. Check DB_PATH in %s and make sure service user %q can write the database file and its directory.", s.EnvFile, s.User)
			}
		}
		return errors.New(detail.String())
	}
	return nil
}

func installService(o serviceOptions) error {
	if runtime.GOOS == "darwin" && o.Scope == "" {
		o.Scope = "system"
	}
	if o.User == "" && !(runtime.GOOS == "darwin" && o.Scope == "user") {
		o.User = defaultServiceUser()
	}
	if o.User != "" {
		if _, err := user.Lookup(o.User); err != nil {
			return fmt.Errorf("unknown user %q for the service", o.User)
		}
	}
	switch runtime.GOOS {
	case "linux":
		if !systemdAvailable() {
			s, err := buildServiceSpec(o)
			if err != nil {
				return err
			}
			return fmt.Errorf("systemd was not found (no /run/systemd/system), so WRM cannot install a service here.\n"+
				"Start it from your init system (OpenRC, runit, s6, cron @reboot …) with:\n  cd %s && %s -env-file %s\n"+
				"The environment file %s has been written", shQuote(s.WorkDir), shQuote(s.Exe), shQuote(s.EnvFile), s.EnvFile)
		}
	case "darwin", "freebsd", "openbsd":
	default:
		return fmt.Errorf("services are not supported on %s: start WRM from your init system or a terminal multiplexer (tmux / screen)", runtime.GOOS)
	}
	if err := needRoot(o); err != nil {
		return err
	}
	s, err := collectServiceSpec(o)
	if err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		if err := checkSystemdDatabaseAccess(s); err != nil {
			return err
		}
	}
	if err := writeEnvFile(s, serviceEnvironment(s.DBPath)); err != nil {
		return err
	}
	chownToUser(s.EnvFile, s.User)
	path := serviceFilePath(o)
	switch runtime.GOOS {
	case "linux":
		if err := os.WriteFile(path, []byte(systemdUnit(s)), 0o644); err != nil {
			return err
		}
		if _, err := run("systemctl", "daemon-reload"); err != nil {
			return err
		}
		if err := startSystemdService(s); err != nil {
			return err
		}
	case "darwin":
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(launchdPlist(s)), 0o644); err != nil {
			return err
		}
		chownToUser(s.LogFile, s.User)
		domain := launchdDomain(o.Scope)
		run("launchctl", "bootout", domain+"/"+s.launchdLabel())
		if _, err := run("launchctl", "bootstrap", domain, path); err != nil {
			return err
		}
		run("launchctl", "enable", domain+"/"+s.launchdLabel())
	case "freebsd":
		if err := os.WriteFile(path, []byte(freebsdRCScript(s)), 0o755); err != nil {
			return err
		}
		if _, err := run("sysrc", o.Name+"_enable=YES"); err != nil {
			return err
		}
		if _, err := run("service", o.Name, "start"); err != nil {
			return err
		}
	case "openbsd":
		if err := os.WriteFile(path, []byte(openbsdRCScript(s)), 0o555); err != nil {
			return err
		}
		if _, err := run("rcctl", "enable", o.Name); err != nil {
			return err
		}
		if _, err := run("rcctl", "start", o.Name); err != nil {
			return err
		}
	}
	fmt.Printf("Installed and started the service %q (%s).\n  Definition:  %s\n  Settings:    %s\n  Binary:      %s\n  Database:    %s\n",
		o.Name, serviceManagerName(o.Scope), path, s.EnvFile, s.Exe, s.DBPath)
	if s.User != "" {
		fmt.Printf("  Runs as:     %s\n", s.User)
	}
	fmt.Printf("Status: %s -service-status   Remove: %s -uninstall-service\n", filepath.Base(s.Exe), filepath.Base(s.Exe))
	return nil
}

func launchdDomain(scope string) string {
	if scope == "user" {
		return "gui/" + strconv.Itoa(os.Getuid())
	}
	return "system"
}

// installElevated runs the installer again with sudo (or doas); the environment goes through
// the environment file, since sudo resets it.
func installElevated(o serviceOptions) error {
	if runtime.GOOS == "darwin" && o.Scope == "" {
		o.Scope = "system"
	}
	if o.User == "" && !(runtime.GOOS == "darwin" && o.Scope == "user") {
		o.User = defaultServiceUser()
	}
	s, err := buildServiceSpec(o)
	if err != nil {
		return err
	}
	tool := ""
	for _, t := range []string{"sudo", "doas"} {
		if _, err := exec.LookPath(t); err == nil {
			tool = t
			break
		}
	}
	if tool == "" {
		return fmt.Errorf("installing a system service needs root, and neither sudo nor doas was found: run as root  %s -install-service", s.Exe)
	}
	args := []string{s.Exe, "-install-service", "-no-service-prompt", "-service-name", o.Name, "-env-file", s.EnvFile, "-workdir", s.WorkDir}
	if o.Scope != "" {
		args = append(args, "-service-scope", o.Scope)
	}
	if o.User != "" {
		args = append(args, "-service-user", o.User)
	}
	fmt.Printf("Installing the service needs root; running: %s %s\n", tool, shJoin(args))
	cmd := exec.Command(tool, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %v", tool, err)
	}
	return nil
}

func uninstallService(o serviceOptions) error {
	if runtime.GOOS == "darwin" && o.Scope == "" {
		if _, err := os.Stat(serviceFilePath(serviceOptions{Name: o.Name, Scope: "user"})); err == nil {
			o.Scope = "user"
		} else {
			o.Scope = "system"
		}
	}
	path := serviceFilePath(o)
	if path == "" {
		return fmt.Errorf("services are not supported on %s", runtime.GOOS)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("the service %q is not installed (%s not found)", o.Name, path)
	}
	if err := needRoot(o); err != nil {
		return errors.New(strings.Replace(err.Error(), "-install-service", "-uninstall-service", 1))
	}
	switch runtime.GOOS {
	case "linux":
		run("systemctl", "disable", "--now", o.Name+".service")
		if err := os.Remove(path); err != nil {
			return err
		}
		run("systemctl", "daemon-reload")
	case "darwin":
		run("launchctl", "bootout", launchdDomain(o.Scope)+"/"+serviceSpec{Name: o.Name}.launchdLabel())
		if err := os.Remove(path); err != nil {
			return err
		}
	case "freebsd":
		run("service", o.Name, "stop")
		run("sysrc", "-x", o.Name+"_enable")
		if err := os.Remove(path); err != nil {
			return err
		}
	case "openbsd":
		run("rcctl", "stop", o.Name)
		run("rcctl", "disable", o.Name)
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	fmt.Printf("Removed the service %q (%s). The data, the database and the environment file stay where they are.\n", o.Name, path)
	return nil
}

func serviceStatusText(o serviceOptions) string {
	if !servicePlatformSupported() {
		if runtime.GOOS == "linux" {
			return "systemd was not found: no service can be installed on this system.\n"
		}
		return fmt.Sprintf("Services are not supported on %s.\n", runtime.GOOS)
	}
	var b strings.Builder
	found := false
	for _, scope := range []string{"system", "user"} {
		if runtime.GOOS != "darwin" && scope == "user" {
			continue
		}
		so := o
		so.Scope = scope
		path := serviceFilePath(so)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		found = true
		state := "unknown"
		switch runtime.GOOS {
		case "linux":
			en, _ := run("systemctl", "is-enabled", o.Name+".service")
			act, _ := run("systemctl", "is-active", o.Name+".service")
			state = act + ", " + en + " at boot"
		case "darwin":
			out, err := run("launchctl", "print", launchdDomain(scope)+"/"+serviceSpec{Name: o.Name}.launchdLabel())
			state = "not loaded"
			if err == nil {
				state = "loaded"
				for _, l := range strings.Split(out, "\n") {
					if l = strings.TrimSpace(l); strings.HasPrefix(l, "state = ") {
						state = strings.TrimPrefix(l, "state = ")
						break
					}
				}
			}
		case "freebsd":
			if _, err := run("service", o.Name, "status"); err == nil {
				state = "running"
			} else {
				state = "stopped"
			}
		case "openbsd":
			if _, err := run("rcctl", "check", o.Name); err == nil {
				state = "running"
			} else {
				state = "stopped"
			}
		}
		fmt.Fprintf(&b, "Service %q is installed (%s): %s\n  Definition: %s\n", o.Name, serviceManagerName(scope), state, path)
	}
	if !found {
		fmt.Fprintf(&b, "Service %q is not installed. Install it with -install-service.\n", o.Name)
	}
	return b.String()
}

// execBinary replaces this process with path (same PID, so a service manager keeps tracking it).
func execBinary(path string, args []string) error {
	return syscall.Exec(path, append([]string{path}, args...), os.Environ())
}

func switchServiceBinary(path string) error { return execBinary(path, os.Args[1:]) }

// runAsWindowsService is only used on Windows.
func runAsWindowsService(name string, serve func(stop <-chan struct{}) int) (bool, int) {
	return false, 0
}
