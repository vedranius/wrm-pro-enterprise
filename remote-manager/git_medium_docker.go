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
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Docker for Medium on every operating system. WRM works out how this host can get
// Docker, installs it once an administrator agreed (accepting Docker Desktop's licence
// where it applies), starts it and waits until it answers. Every command and its output
// go to the operation's log; when WRM cannot do a step itself it says exactly what to run.

const mediumDockerLicenseURL = "https://www.docker.com/legal/docker-subscription-service-agreement/"

// Docker Inc.'s Apple developer team, which signs Docker Desktop for Mac.
const mediumDockerAppleTeam = "9BNSXJN65R"

// Replaced in tests.
var (
	mediumHostOS      = func() string { return runtime.GOOS }
	mediumHostArch    = func() string { return runtime.GOARCH }
	mediumEUID        = os.Geteuid
	mediumOSRelease   = "/etc/os-release"
	mediumDesktopBase = "https://desktop.docker.com"
	mediumInContainer = func() bool { return mediumFileExists("/.dockerenv") || mediumFileExists("/run/.containerenv") }
	mediumSystemd     = func() bool { st, err := os.Stat("/run/systemd/system"); return err == nil && st.IsDir() }
)

// mediumDockerInfo is what Medium step 2 shows about Docker on the WRM host.
type mediumDockerInfo struct {
	OS           string               `json:"os"`
	Installed    bool                 `json:"installed"`
	Product      string               `json:"product"`          // Docker Engine or Docker Desktop
	Method       string               `json:"method,omitempty"` // apt, dnf, docker-ce, zypper, pacman, apk, desktop
	Auto         bool                 `json:"auto"`             // WRM can install it here itself
	Allowed      bool                 `json:"allowed"`          // policy medium_docker_install
	NeedsLicense bool                 `json:"needs_license"`
	LicenseURL   string               `json:"license_url,omitempty"`
	Consent      *mediumDockerConsent `json:"consent,omitempty"`
	Why          string               `json:"why,omitempty"`     // why WRM cannot install it here
	Command      string               `json:"command,omitempty"` // what an administrator runs instead
	Note         string               `json:"note,omitempty"`    // what to expect during the installation
}

// mediumDockerConsent records the administrator who allowed the installation.
type mediumDockerConsent struct {
	UserID  int    `json:"user_id"`
	User    string `json:"user"`
	At      string `json:"at"`
	Product string `json:"product"`
	License bool   `json:"license"` // Docker's licence was accepted too
}

var errMediumDockerConsent = errors.New("Docker is not installed on the WRM host yet. An administrator presses \"Install Docker\" in step 2 once (accepting Docker Desktop's licence where it applies); WRM then installs it and continues. Your ZIPs are kept")

func mediumConsentPath() string { return filepath.Join(mediumRoot(), "docker-consent.json") }

func mediumDockerConsentGet() *mediumDockerConsent {
	b, err := os.ReadFile(mediumConsentPath())
	if err != nil {
		return nil
	}
	var c mediumDockerConsent
	if json.Unmarshal(b, &c) != nil || c.UserID == 0 {
		return nil
	}
	return &c
}

// mediumDockerInfo is cached for a minute while Docker is missing: the status is polled,
// and finding out the privileges runs sudo.
var mediumDockerCache struct {
	sync.Mutex
	at   time.Time
	info mediumDockerInfo
}

func mediumDockerInfoReset() {
	mediumDockerCache.Lock()
	mediumDockerCache.at = time.Time{}
	mediumDockerCache.Unlock()
}

func mediumDockerInfoCached() mediumDockerInfo {
	mediumDockerCache.Lock()
	defer mediumDockerCache.Unlock()
	if time.Since(mediumDockerCache.at) > time.Minute || mediumDockerCache.info.Installed != (mediumDockerCLI() != "") {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		mediumDockerCache.info = mediumDockerInfoNow(ctx)
		cancel()
		mediumDockerCache.at = time.Now()
	}
	info := mediumDockerCache.info
	info.Consent, info.Allowed = mediumDockerConsentGet(), settingBool("medium_docker_install")
	return info
}

func mediumDockerInfoNow(ctx context.Context) mediumDockerInfo {
	goos := mediumHostOS()
	info := mediumDockerInfo{OS: goos, Installed: mediumDockerCLI() != "", Allowed: settingBool("medium_docker_install"), Consent: mediumDockerConsentGet()}
	switch goos {
	case "linux":
		info.Product = "Docker Engine"
		if mediumInContainer() {
			info.Why = "WRM itself runs in a container. Give it the host's Docker instead: mount /var/run/docker.sock and the docker CLI into WRM's container"
			return info
		}
		method, _, _, why := mediumLinuxPlan()
		if method == "" {
			info.Why = why
			info.Command = "Install Docker Engine as described at https://docs.docker.com/engine/install/"
			return info
		}
		info.Method = method
		priv := mediumPrivilege(ctx)
		if priv == "" {
			info.Why = "WRM runs as " + mediumCurrentUser() + " without administrator rights, so it cannot install packages"
			info.Command = mediumSetupCommand()
			return info
		}
		info.Auto = true
		if mediumEUID() != 0 {
			info.Note = "WRM's account is added to the docker group; restart WRM once afterwards"
		}
	case "darwin":
		info.Product, info.Method, info.NeedsLicense, info.LicenseURL, info.Auto = "Docker Desktop", "desktop", true, mediumDockerLicenseURL, true
		if mediumPrivilege(ctx) == "" {
			info.Note = "WRM has no administrator rights here, so Docker Desktop goes to ~/Applications; confirm its first-start window on the Mac once"
		} else if mediumEUID() == 0 {
			info.Note = "Docker Desktop is installed for the user signed in on the Mac and runs in that user's session: someone must be signed in"
		}
	case "windows":
		info.Product, info.Method, info.NeedsLicense, info.LicenseURL = "Docker Desktop", "desktop", true, mediumDockerLicenseURL
		if serviceMode == "windows" {
			info.Why = "WRM runs as a Windows service, and Docker Desktop only runs in a signed-in user's session"
			info.Command = "Install Docker Desktop for the account that is signed in on this PC, or run WRM as that user (not as a service)"
			return info
		}
		info.Auto = true
		info.Note = "The first time Windows may ask once to enable WSL (an administrator prompt) and to restart"
	default:
		info.Product = "Docker"
		info.Why = "Docker is not available for " + goos
		info.Command = "Use the host runtime instead: WRM_MEDIUM_RUNTIME=native with Python 3.12+, git, SSH and rsync"
	}
	return info
}

// mediumConsoleUser: the user signed in on the Mac's screen ("" when nobody or root).
func mediumConsoleUser(ctx context.Context) string {
	b, err := exec.CommandContext(ctx, "stat", "-f", "%Su", "/dev/console").Output()
	if u := strings.TrimSpace(string(b)); err == nil && u != "root" {
		return u
	}
	return ""
}

func mediumCurrentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "this account"
}

// mediumSetupCommand is the one command an administrator runs when WRM has no rights.
func mediumSetupCommand() string {
	exe, err := os.Executable()
	if err != nil {
		exe = "wrm"
	}
	if p := os.Getenv("APPIMAGE"); os.Getenv("WRM_APPIMAGE") == "1" && p != "" {
		exe = p // the binary inside the AppImage's mount is not reachable for root
	}
	return "sudo " + shellQuote(exe) + " -setup-docker -service-user " + shellQuote(mediumCurrentUser())
}

// mediumPrivilege: how WRM runs a command as root. "root" (it is root), "systemd-run"
// (root in its sandboxed service: the command runs as its own unit outside the sandbox),
// "sudo" (passwordless sudo) or "" (no administrator rights).
func mediumPrivilege(ctx context.Context) string {
	if mediumHostOS() == "windows" {
		return ""
	}
	if mediumEUID() == 0 {
		if mediumHostOS() == "linux" && os.Getenv("INVOCATION_ID") != "" {
			if _, err := exec.LookPath("systemd-run"); err == nil {
				return "systemd-run"
			}
		}
		return "root"
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return ""
	}
	// sudo logs (and may mail) every refused attempt: probe it once, not on every status poll
	mediumSudo.Lock()
	defer mediumSudo.Unlock()
	if !mediumSudo.known {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		mediumSudo.ok = exec.CommandContext(cctx, "sudo", "-n", "true").Run() == nil
		cancel()
		mediumSudo.known = true
	}
	if mediumSudo.ok {
		return "sudo"
	}
	return ""
}

var mediumSudo struct {
	sync.Mutex
	known, ok bool
}

// mediumSudoReprobe: an administrator asks to install, so passwordless sudo set up since is found.
func mediumSudoReprobe() {
	mediumSudo.Lock()
	mediumSudo.known = false
	mediumSudo.Unlock()
}

func mediumAsRoot(priv string, argv ...string) []string {
	// systemd-run starts with a clean environment and sudo resets it: pass WRM's proxy on
	var proxy []string
	for _, k := range []string{"http_proxy", "https_proxy", "no_proxy", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		if v := os.Getenv(k); v != "" {
			proxy = append(proxy, k+"="+v)
		}
	}
	switch priv {
	case "systemd-run":
		out := []string{"systemd-run", "--wait", "--pipe", "--collect", "--quiet"}
		for _, kv := range proxy {
			out = append(out, "--setenv="+kv)
		}
		return append(append(out, "--"), argv...)
	case "sudo":
		out := []string{"sudo", "-n", "--"}
		if len(proxy) > 0 {
			out = append(append(out, "env"), proxy...)
		}
		return append(out, argv...)
	}
	return argv
}

func mediumReadOSRelease() (id, like string) {
	f, err := os.Open(mediumOSRelease)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			id = v
		case "ID_LIKE":
			like = v
		}
	}
	return id, like
}

// mediumLinuxPlan: the distribution's packages for Docker Engine (Docker's own
// repository on RHEL-like systems, which ship none), and how its service starts.
func mediumLinuxPlan() (method string, install, start [][]string, why string) {
	has := func(n string) bool { _, err := exec.LookPath(n); return err == nil }
	id, like := mediumReadOSRelease()
	switch {
	case has("systemctl") && mediumSystemd():
		start = [][]string{{"systemctl", "enable", "--now", "docker"}}
	case has("rc-service"):
		start = [][]string{{"rc-update", "add", "docker", "default"}, {"rc-service", "docker", "start"}}
	default:
		start = [][]string{{"service", "docker", "start"}}
	}
	switch {
	case has("apt-get"):
		apt := []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get"}
		return "apt", [][]string{append(append([]string{}, apt...), "update"), append(append([]string{}, apt...), "install", "-y", "docker.io")}, start, ""
	case has("dnf") && id == "fedora":
		return "dnf", [][]string{{"dnf", "install", "-y", "moby-engine"}}, start, ""
	case id == "amzn" && (has("dnf") || has("yum")):
		pm := "dnf"
		if !has("dnf") {
			pm = "yum"
		}
		return pm, [][]string{{pm, "install", "-y", "docker"}}, start, ""
	case has("dnf") || has("yum"):
		pm := "dnf"
		if !has("dnf") {
			pm = "yum"
		}
		dist := "centos"
		if id == "rhel" {
			dist = "rhel"
		} else if !strings.Contains(" "+like+" ", " rhel ") && !strings.Contains(" "+like+" ", " centos ") && !strings.Contains(" "+like+" ", " fedora ") && id != "centos" {
			return "", nil, start, "Docker Engine packages are not known for this distribution (" + id + ")"
		}
		return "docker-ce", [][]string{
			{"curl", "-fsSL", "-o", "/etc/yum.repos.d/docker-ce.repo", "https://download.docker.com/linux/" + dist + "/docker-ce.repo"},
			{pm, "install", "-y", "docker-ce", "docker-ce-cli", "containerd.io"},
		}, start, ""
	case has("zypper"):
		return "zypper", [][]string{{"zypper", "--non-interactive", "install", "docker"}}, start, ""
	case has("pacman"):
		return "pacman", [][]string{{"pacman", "-S", "--noconfirm", "--needed", "docker"}}, start, ""
	case has("apk"):
		return "apk", [][]string{{"apk", "add", "docker"}}, start, ""
	}
	return "", nil, start, "No supported package manager was found (apt, dnf, yum, zypper, pacman or apk)"
}

// mediumInstallDocker installs Docker on this host, if the policy, the host and the
// administrator's consent allow it.
func mediumInstallDocker(ctx context.Context, step func(string)) error {
	info := mediumDockerInfoNow(ctx)
	defer mediumDockerInfoReset()
	if !info.Allowed {
		return fmt.Errorf("Docker is not installed on the WRM host, and the administrator switched automatic installation off (policy medium_docker_install). Install %s yourself, then press Install Docker or Prepare runtime; your ZIPs are kept", info.Product)
	}
	if !info.Auto {
		msg := "Docker is not installed on the WRM host, and WRM cannot install it here: " + info.Why
		if info.Command != "" {
			msg += ". Run once: " + info.Command
		}
		return errors.New(msg + ". Then press Prepare runtime (or import the tool ZIP); your ZIPs are kept")
	}
	if info.Consent == nil {
		return errMediumDockerConsent
	}
	if err := os.MkdirAll(mediumRoot(), 0700); err != nil {
		return err
	}
	if out := mediumOpOutput(); out != nil {
		out.Note("installing %s (%s), allowed by %s on %s", info.Product, info.Method, info.Consent.User, info.Consent.At)
	}
	switch info.OS {
	case "linux":
		return mediumInstallLinux(ctx, step)
	case "darwin":
		return mediumInstallMac(ctx, step)
	case "windows":
		return mediumInstallDesktop(ctx, step)
	}
	return fmt.Errorf("Docker cannot be installed on %s", info.OS)
}

func mediumRunArgv(ctx context.Context, argv []string) error {
	return mediumCommand(ctx, mediumRoot(), argv[0], argv[1:]...)
}

func mediumInstallLinux(ctx context.Context, step func(string)) error {
	method, install, start, why := mediumLinuxPlan()
	if method == "" {
		return errors.New(why)
	}
	priv := mediumPrivilege(ctx)
	step("docker_install")
	for _, c := range install {
		if err := mediumRunArgv(ctx, mediumAsRoot(priv, c...)); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			hint := ""
			if method == "docker-ce" {
				hint = "; on RHEL-like systems podman/runc conflict with containerd.io: remove them first (dnf remove podman runc) if the output names them"
			}
			return fmt.Errorf("Installing Docker Engine (%s) failed. The output shows why (package mirror or internet access, a proxy, disk space, another package manager running%s); fix it and press Install Docker again. Your ZIPs are kept", method, hint)
		}
	}
	step("docker_start")
	for _, c := range start {
		if err := mediumRunArgv(ctx, mediumAsRoot(priv, c...)); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("Docker Engine is installed, but its service did not start (%s). The output shows why; start it (for example: sudo %s), then press Install Docker or Prepare runtime. Your ZIPs are kept", strings.Join(c, " "), strings.Join(c, " "))
		}
	}
	if mediumEUID() != 0 && mediumDockerCheck(ctx) != nil {
		u := mediumCurrentUser()
		if err := mediumRunArgv(ctx, mediumAsRoot(priv, "usermod", "-aG", "docker", u)); err != nil {
			return fmt.Errorf("Docker Engine is installed and running, but WRM could not add its account %s to the docker group: run sudo usermod -aG docker %s, restart WRM and press Install Docker or Prepare runtime", u, u)
		}
		return fmt.Errorf("Docker Engine is installed and running. WRM's account %s was added to the docker group: restart WRM once so it gets access, then press Install Docker or Prepare runtime. Your ZIPs are kept", u)
	}
	return nil
}

// mediumDownload fetches an official Docker installer (redirects stay on docker.com).
func mediumDownload(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 20 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || r.URL.Scheme != "https" || !(r.URL.Hostname() == "docker.com" || strings.HasSuffix(r.URL.Hostname(), ".docker.com")) {
			return fmt.Errorf("Unexpected Docker download redirect")
		}
		return nil
	}}
	out := mediumOpOutput()
	if out != nil {
		out.Note("downloading %s", url)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Cannot download the official Docker Desktop installer from %s; check the WRM host's outbound HTTPS (proxy, firewall), then press Install Docker or Prepare runtime. Your ZIPs are kept", url)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("The Docker Desktop download returned HTTP %d; press Install Docker or Prepare runtime again later. Your ZIPs are kept", resp.StatusCode)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, 2<<30+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil || n > 2<<30 {
		return fmt.Errorf("The Docker Desktop download failed or exceeds 2 GiB; check free disk space and press Install Docker or Prepare runtime again. Your ZIPs are kept")
	}
	if out != nil {
		out.Note("downloaded %d MiB", n>>20)
	}
	return nil
}

func mediumDesktopArch() string {
	if mediumHostArch() == "arm64" {
		return "arm64"
	}
	return "amd64"
}

func mediumInstallMac(ctx context.Context, step func(string)) error {
	dir, err := os.MkdirTemp(mediumRoot(), "docker-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	step("docker_download")
	dmg := filepath.Join(dir, "Docker.dmg")
	if err := mediumDownload(ctx, mediumDesktopBase+"/mac/main/"+mediumDesktopArch()+"/Docker.dmg", dmg); err != nil {
		return err
	}
	mnt := filepath.Join(dir, "mnt")
	if err := os.Mkdir(mnt, 0700); err != nil {
		return err
	}
	if err := mediumCommand(ctx, dir, "hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mnt, dmg); err != nil {
		return fmt.Errorf("Cannot open the downloaded Docker.dmg (hdiutil); the output shows why. Press Prepare again; your ZIPs are kept")
	}
	defer exec.Command("hdiutil", "detach", "-force", mnt).Run()
	app := filepath.Join(mnt, "Docker.app")
	step("docker_verify")
	sig, verr := exec.CommandContext(ctx, "codesign", "-dv", "--verbose=2", app).CombinedOutput()
	if out := mediumOpOutput(); out != nil {
		out.Write(sig)
	}
	if verr != nil || mediumCommand(ctx, dir, "codesign", "--verify", "--deep", "--strict", app) != nil || !strings.Contains(string(sig), "TeamIdentifier="+mediumDockerAppleTeam) {
		return fmt.Errorf("The downloaded Docker Desktop is not signed by Docker Inc.; WRM did not install it. Press Prepare to download it again")
	}
	step("docker_install")
	if priv := mediumPrivilege(ctx); priv != "" {
		who := mediumCurrentUser()
		if mediumEUID() == 0 {
			// Docker Desktop belongs to the user signed in on the Mac
			who = mediumConsoleUser(ctx)
			if who == "" {
				return fmt.Errorf("Nobody is signed in on the Mac: Docker Desktop is installed for, and runs in the session of, the signed-in user. Sign in on the Mac, then press Install Docker again. Your ZIPs are kept")
			}
		}
		if err := mediumRunArgv(ctx, mediumAsRoot(priv, filepath.Join(app, "Contents", "MacOS", "install"), "--accept-license", "--user="+who)); err != nil {
			return fmt.Errorf("The Docker Desktop installer did not finish; the output shows why. Press Prepare again; your ZIPs are kept")
		}
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(home, "Applications", "Docker.app")
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	if err := mediumCommand(ctx, dir, "ditto", app, dest); err != nil {
		return fmt.Errorf("Cannot copy Docker Desktop to %s; the output shows why. Press Prepare again; your ZIPs are kept", filepath.Dir(dest))
	}
	_ = mediumCommand(ctx, dir, "open", "-a", dest)
	return nil
}

// mediumInstallDesktop installs Docker Desktop for the signed-in Windows user, quietly,
// with the licence the administrator accepted in WRM.
func mediumInstallDesktop(ctx context.Context, step func(string)) error {
	dir, err := os.MkdirTemp(mediumRoot(), "docker-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	step("docker_download")
	p := filepath.Join(dir, "Docker Desktop Installer.exe")
	if err := mediumDownload(ctx, mediumDesktopBase+"/win/main/"+mediumDesktopArch()+"/Docker%20Desktop%20Installer.exe", p); err != nil {
		return err
	}
	quoted := "'" + strings.ReplaceAll(p, "'", "''") + "'"
	step("docker_verify")
	verify := "$ErrorActionPreference='Stop'; $s=Get-AuthenticodeSignature -LiteralPath " + quoted + "; $s.Status; $s.SignerCertificate.Subject; if($s.Status -ne 'Valid' -or $s.SignerCertificate.Subject -notmatch '(^|,\\s*)O=Docker Inc\\.?(,|$)'){throw 'Docker installer signature rejected'}"
	if err := mediumCommand(ctx, dir, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", verify); err != nil {
		return fmt.Errorf("The downloaded Docker Desktop installer is not signed by Docker Inc.; WRM did not run it. Press Prepare to download it again")
	}
	step("docker_install")
	install := "$ErrorActionPreference='Stop'; $x=Start-Process -FilePath " + quoted + " -ArgumentList 'install','--user','--quiet','--accept-license','--backend=wsl-2' -Wait -PassThru; 'installer exit code ' + $x.ExitCode; exit $x.ExitCode"
	if err := mediumCommand(ctx, dir, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", install); err != nil {
		return fmt.Errorf("The Docker Desktop installer did not finish. If Windows asked to enable WSL or to restart, do that, then press Install Docker or Prepare runtime; your ZIPs are kept")
	}
	return nil
}

// apiMediumDocker: an administrator allows WRM to install Docker (accepting Docker
// Desktop's licence where it applies) and starts the installation; accept=false withdraws it.
func apiMediumDocker(w http.ResponseWriter, r *http.Request, uid int) {
	if !isAdminUser(uid) {
		jsonError(w, "Only administrators can install Docker on the WRM host", 403)
		return
	}
	var body struct {
		Accept        bool `json:"accept"`
		AcceptLicense bool `json:"accept_license"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	if !body.Accept {
		os.Remove(mediumConsentPath())
		auditLog(r, uid, "git.medium_docker_consent_withdrawn", "", nil)
		jsonOK(w, mediumStateFull(uid))
		return
	}
	mediumSudoReprobe()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	info := mediumDockerInfoNow(ctx)
	cancel()
	if !info.Allowed {
		jsonError(w, "Automatic Docker installation is switched off (policy medium_docker_install)", 403)
		return
	}
	if !info.Installed && !info.Auto {
		msg := "WRM cannot install Docker here: " + info.Why
		if info.Command != "" {
			msg += ". Run once: " + info.Command
		}
		jsonError(w, msg, 409)
		return
	}
	if info.NeedsLicense && !body.AcceptLicense {
		jsonError(w, "Accept Docker's licence (Docker Subscription Service Agreement) to install "+info.Product, 400)
		return
	}
	h, err := mediumBeginOp(uid, "docker", info.Product)
	if err != nil {
		jsonError(w, err.Error(), 409)
		return
	}
	c := mediumDockerConsent{UserID: uid, User: usernameOf(uid), At: time.Now().UTC().Format(time.RFC3339), Product: info.Product, License: info.NeedsLicense}
	if err := os.MkdirAll(mediumRoot(), 0700); err != nil || os.WriteFile(mediumConsentPath(), jsonMarshal(c), 0600) != nil {
		mediumEndOp(h, fmt.Errorf("Cannot save the consent"))
		jsonError(w, "Cannot save the consent", 500)
		return
	}
	auditLog(r, uid, "git.medium_docker_consent", "", map[string]interface{}{"product": info.Product, "method": info.Method, "license": info.NeedsLicense, "license_url": info.LicenseURL})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		step := func(s string) { mediumSetStage(uid, s, true, nil) }
		step("docker")
		err := mediumEnsureDocker(ctx, step)
		// with the deploy tool already imported, its runtime is prepared at once
		if err == nil && mediumState(uid).Imported && !mediumRuntimeReady() && !mediumAnyRunning() {
			err = mediumPrepareRuntime(ctx, step)
		}
		mediumEndOp(h, err)
	}()
	w.WriteHeader(http.StatusAccepted)
	jsonOK(w, mediumStateFull(uid))
}

// runSetupDocker is "wrm -setup-docker": as root, install Docker Engine, let the WRM
// account use it and restart the WRM service. For hosts where WRM has no rights itself.
func runSetupDocker(account, svcName string, stdout io.Writer) int {
	if runtime.GOOS != "linux" {
		fmt.Fprintln(stdout, "On macOS and Windows, WRM installs Docker Desktop itself: Git workspace → Medium, step 2, \"Install Docker\".")
		return 0
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(stdout, "Run this as root, for example: sudo "+os.Args[0]+" -setup-docker")
		return 1
	}
	run := func(argv ...string) error {
		fmt.Fprintln(stdout, "$ "+strings.Join(argv, " "))
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdout, cmd.Stderr = stdout, stdout
		return cmd.Run()
	}
	method, install, start, why := mediumLinuxPlan()
	if _, err := exec.LookPath("docker"); err == nil {
		fmt.Fprintln(stdout, "Docker is already installed; making sure its service runs.")
		install = nil
	} else if method == "" {
		fmt.Fprintln(stdout, why+". Install Docker Engine as described at https://docs.docker.com/engine/install/")
		return 1
	}
	for _, c := range append(install, start...) {
		if err := run(c...); err != nil {
			fmt.Fprintf(stdout, "Failed: %v. The output above shows why.\n", err)
			return 1
		}
	}
	if account == "" {
		account = mediumServiceAccount(svcName)
	}
	if account != "" && account != "root" {
		if err := run("usermod", "-aG", "docker", account); err != nil {
			fmt.Fprintf(stdout, "Cannot add %s to the docker group: %v\n", account, err)
			return 1
		}
	}
	if serviceInstalled(svcName) {
		_ = run("systemctl", "try-restart", svcName)
	} else if account != "" && account != "root" {
		fmt.Fprintf(stdout, "Restart WRM (running as %s) so it can use Docker.\n", account)
	}
	fmt.Fprintln(stdout, "Done. In WRM, open Git workspace → Medium and press Install Docker or Prepare runtime.")
	return 0
}

// mediumServiceAccount: the User= of the installed WRM unit, else the sudo caller.
func mediumServiceAccount(svcName string) string {
	if b, err := os.ReadFile("/etc/systemd/system/" + svcName + ".service"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(l), "User="); ok {
				return strings.TrimSpace(v)
			}
		}
		return "root"
	}
	return os.Getenv("SUDO_USER")
}
