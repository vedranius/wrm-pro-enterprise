// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/cpu"
)

// ─── OPENCODE: INSTALLATION ──────────────────────────
//
// OpenCode (MIT licence, opencode.ai) is one self-contained binary per platform (~180 MB),
// published on the npm registry as opencode-<os>-<arch>[-baseline][-musl]. WRM does not bundle
// it: an administrator presses "Install OpenCode" once (POST /api/admin/opencode/install) and
// WRM downloads the pinned version (ai_opencode_version) from the registry
// (ai_opencode_registry), checks the tarball against the registry's sha512 integrity, unpacks
// the binary to <DB>.opencode/bin/<version>/ and runs it once with --version. Every step is
// written to a log the admin page shows; the result is audited. ai_opencode_path can point at
// an OpenCode that is already installed instead (offline hosts).

const opencodeDefaultVersion = "1.19.3"

func opencodeRoot() string {
	p, _ := filepath.Abs(resolveDBPath())
	return p + ".opencode"
}

// opencodePackage is the npm package for this host, or "" when OpenCode has no build for it.
func opencodePackage() string {
	exe := ""
	baseline := ""
	if runtime.GOARCH == "amd64" && !cpu.X86.HasAVX2 {
		baseline = "-baseline"
	}
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		exe = "opencode-linux-x64" + baseline
	case "linux/arm64":
		exe = "opencode-linux-arm64"
	case "darwin/amd64":
		exe = "opencode-darwin-x64" + baseline
	case "darwin/arm64":
		exe = "opencode-darwin-arm64"
	case "windows/amd64":
		exe = "opencode-windows-x64" + baseline
	case "windows/arm64":
		exe = "opencode-windows-arm64"
	default:
		return ""
	}
	if runtime.GOOS == "linux" && opencodeMusl() {
		exe += "-musl"
	}
	return exe
}

// opencodeMusl reports a musl libc system (Alpine and the WRM container image).
func opencodeMusl() bool {
	if _, err := os.Stat("/etc/alpine-release"); err == nil {
		return true
	}
	m, _ := filepath.Glob("/lib/ld-musl-*.so.1")
	return len(m) > 0
}

func opencodeBinName() string {
	if runtime.GOOS == "windows" {
		return "opencode.exe"
	}
	return "opencode"
}

type opencodeInstallState struct {
	Version     string `json:"version"`
	Path        string `json:"path"`
	Package     string `json:"package"`
	SHA512      string `json:"sha512"`
	InstalledAt string `json:"installed_at"`
	InstalledBy string `json:"installed_by"`
	// Sandbox: the Linux build for OpenCode's Docker sandbox, by package (when it differs
	// from the host's own build)
	Sandbox map[string]string `json:"sandbox,omitempty"`
}

var opencodeInst struct {
	sync.Mutex
	running bool
	log     []string
	err     string
	started time.Time
}

func opencodeLogf(format string, a ...interface{}) {
	line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, a...)
	opencodeInst.Lock()
	opencodeInst.log = append(opencodeInst.log, line)
	if len(opencodeInst.log) > 200 {
		opencodeInst.log = opencodeInst.log[len(opencodeInst.log)-200:]
	}
	opencodeInst.Unlock()
}

func opencodeLoadState() (opencodeInstallState, bool) {
	var st opencodeInstallState
	b, err := os.ReadFile(filepath.Join(opencodeRoot(), "installed.json"))
	if err != nil || json.Unmarshal(b, &st) != nil || st.Path == "" {
		return st, false
	}
	if fi, err := os.Stat(st.Path); err != nil || fi.IsDir() {
		return st, false
	}
	return st, true
}

// opencodeBinary returns the OpenCode executable WRM runs, or the reason there is none.
func opencodeBinary() (string, string, error) {
	if p := strings.TrimSpace(getSetting("ai_opencode_path")); p != "" {
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			return "", "", fmt.Errorf("ai_opencode_path %s does not exist", p)
		}
		return p, "path", nil
	}
	st, ok := opencodeLoadState()
	if !ok {
		return "", "", errors.New("OpenCode is not installed yet: an administrator installs it once under Administration → AI → OpenCode")
	}
	return st.Path, st.Version, nil
}

// opencodeStatus is the admin view: what is installed, what would be installed, the log.
func opencodeStatus() map[string]interface{} {
	st, ok := opencodeLoadState()
	opencodeInst.Lock()
	logCopy := append([]string{}, opencodeInst.log...)
	running, lastErr := opencodeInst.running, opencodeInst.err
	opencodeInst.Unlock()
	out := map[string]interface{}{"installed": ok, "package": opencodePackage(), "supported": opencodePackage() != "",
		"version_wanted": nonEmpty(getSetting("ai_opencode_version"), opencodeDefaultVersion), "installing": running, "error": lastErr,
		"log": logCopy, "root": opencodeRoot(), "os": runtime.GOOS + "/" + runtime.GOARCH, "path_setting": getSetting("ai_opencode_path"),
		"sessions": opencodeSessionCount(), "max_sessions": settingInt("ai_opencode_max_sessions")}
	if ok {
		out["version"], out["path"], out["installed_at"], out["installed_by"] = st.Version, st.Path, st.InstalledAt, st.InstalledBy
	}
	if p, src, err := opencodeBinary(); err == nil {
		out["binary"], out["source"] = p, src
	}
	if getSetting("ai_opencode_sandbox") != "off" {
		sb := map[string]interface{}{"image": opencodeSandboxImage()}
		if dos, arch, err := opencodeDockerServer(); err != nil {
			sb["error"] = err.Error()
		} else {
			sb["docker"], sb["package"] = dos+"/"+arch, opencodeSandboxPackage(arch)
			if bin, err := opencodeSandboxBinary(); err != nil {
				sb["error"] = err.Error()
			} else {
				sb["binary"] = bin
			}
		}
		out["sandbox"] = sb
	}
	return out
}

// opencodeInstall downloads, verifies and unpacks OpenCode (runs in the background), and
// the Linux build for the Docker sandbox when the sandbox is allowed (ai_opencode_sandbox).
func opencodeInstall(by string) error {
	pkg := opencodePackage()
	if pkg == "" && strings.TrimSpace(getSetting("ai_opencode_path")) == "" {
		return fmt.Errorf("OpenCode has no build for %s/%s; set ai_opencode_path to an OpenCode binary instead", runtime.GOOS, runtime.GOARCH)
	}
	version := nonEmpty(strings.TrimSpace(getSetting("ai_opencode_version")), opencodeDefaultVersion)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	st, _ := opencodeLoadState()
	if pkg != "" {
		opencodeLogf("Installing OpenCode %s (%s)", version, pkg)
		dir := filepath.Join(opencodeRoot(), "bin", version)
		bin := filepath.Join(dir, opencodeBinName())
		got, err := opencodeFetch(ctx, pkg, version, bin)
		if err != nil {
			return err
		}
		vctx, vcancel := context.WithTimeout(ctx, 60*time.Second)
		defer vcancel()
		cmd := exec.CommandContext(vctx, bin, "--version")
		cmd.Env = opencodeBaseEnv(filepath.Join(opencodeRoot(), "check"))
		os.MkdirAll(filepath.Join(opencodeRoot(), "check"), 0o700)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("the installed binary does not start: %v %s", err, truncateStr(string(out), 300))
		}
		opencodeLogf("opencode --version: %s", strings.TrimSpace(truncateStr(string(out), 100)))
		if st.Version != version {
			st.Sandbox = nil
		}
		st.Version, st.Path, st.Package, st.SHA512, st.InstalledAt, st.InstalledBy = version, bin, pkg, got, nowRFC(), by
		if err := opencodeSaveState(st); err != nil {
			return err
		}
		opencodeLogf("OpenCode %s is ready at %s", version, bin)
	}
	if getSetting("ai_opencode_sandbox") != "off" {
		if err := opencodeInstallSandbox(ctx, version); err != nil {
			opencodeLogf("Docker sandbox: %v", err)
			if pkg == "" {
				return err
			}
		}
	}
	return nil
}

func opencodeSaveState(st opencodeInstallState) error {
	b, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(filepath.Join(opencodeRoot(), "installed.json"), b, 0o600)
}

// opencodeFetch downloads one OpenCode package of the registry, checks it against the
// registry's sha512 integrity and writes its binary to dst. It returns the checksum.
func opencodeFetch(ctx context.Context, pkg, version, dst string) (string, error) {
	registry := strings.TrimRight(nonEmpty(getSetting("ai_opencode_registry"), "https://registry.npmjs.org"), "/")
	get := func(u string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "WRM-PRO/"+AppVersion)
		resp, err := aiHTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode/100 != 2 {
			resp.Body.Close()
			return nil, fmt.Errorf("%s answered HTTP %d", u, resp.StatusCode)
		}
		return resp, nil
	}
	resp, err := get(registry + "/" + pkg + "/" + version)
	if err != nil {
		return "", fmt.Errorf("cannot read the package metadata: %v (check the network, a proxy, or set ai_opencode_registry to a mirror)", err)
	}
	var meta struct {
		Version string `json:"version"`
		License string `json:"license"`
		Dist    struct {
			Tarball   string `json:"tarball"`
			Integrity string `json:"integrity"`
		} `json:"dist"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&meta)
	resp.Body.Close()
	if err != nil || meta.Dist.Tarball == "" {
		return "", fmt.Errorf("the registry returned unexpected package metadata")
	}
	want, ok := strings.CutPrefix(meta.Dist.Integrity, "sha512-")
	if !ok {
		return "", fmt.Errorf("the registry gave no sha512 integrity for the package")
	}
	lic := ""
	if meta.License != "" {
		lic = ", licence " + meta.License
	}
	opencodeLogf("Package %s %s%s from %s; downloading %s", pkg, meta.Version, lic, registry, meta.Dist.Tarball)
	resp, err = get(meta.Dist.Tarball)
	if err != nil {
		return "", fmt.Errorf("download failed: %v", err)
	}
	defer resp.Body.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(opencodeRoot(), "download-*.tgz")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	h := sha512.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, 600<<20))
	tmp.Close()
	if err != nil {
		return "", fmt.Errorf("download failed: %v", err)
	}
	got := base64.StdEncoding.EncodeToString(h.Sum(nil))
	if got != want {
		return "", fmt.Errorf("the download does not match the registry's sha512 integrity; nothing was installed")
	}
	opencodeLogf("Downloaded %d MB, sha512 verified", n>>20)
	name := "package/bin/opencode"
	if strings.Contains(pkg, "-windows-") {
		name += ".exe"
	}
	if err := opencodeExtract(tmp.Name(), name, dst); err != nil {
		return "", err
	}
	return got, nil
}

// opencodeExtract writes one file of a .tgz to dst (atomically, executable).
func opencodeExtract(tgz, name, dst string) error {
	f, err := os.Open(tgz)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("the download is not a gzip archive")
	}
	tr := tar.NewReader(gz)
	for {
		hd, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%s is missing in the package", name)
		}
		if err != nil {
			return fmt.Errorf("the package archive is damaged: %v", err)
		}
		if hd.Name != name || hd.Typeflag != tar.TypeReg {
			continue
		}
		tmp := dst + ".tmp"
		out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, io.LimitReader(tr, 600<<20)); err != nil {
			out.Close()
			os.Remove(tmp)
			return err
		}
		out.Close()
		return os.Rename(tmp, dst)
	}
}

// apiAdminOpencodeHandler: GET /api/admin/opencode (status), POST /api/admin/opencode/install {confirm}.
func apiAdminOpencodeHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/opencode"), "/")
	switch {
	case rest == "" && r.Method == http.MethodGet:
		jsonOK(w, opencodeStatus())
	case rest == "install" && r.Method == http.MethodPost:
		var in struct {
			Confirm bool `json:"confirm"`
		}
		json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in)
		if !in.Confirm {
			jsonError(w, "Confirm the download of OpenCode first", 400)
			return
		}
		opencodeInst.Lock()
		if opencodeInst.running {
			opencodeInst.Unlock()
			jsonError(w, "OpenCode is being installed already", 409)
			return
		}
		opencodeInst.running, opencodeInst.err, opencodeInst.log, opencodeInst.started = true, "", nil, time.Now()
		opencodeInst.Unlock()
		by := usernameOf(userID)
		auditLog(r, userID, "opencode.install_started", "", map[string]interface{}{"package": opencodePackage(), "version": nonEmpty(getSetting("ai_opencode_version"), opencodeDefaultVersion)})
		go func() {
			err := opencodeInstall(by)
			opencodeInst.Lock()
			opencodeInst.running = false
			if err != nil {
				opencodeInst.err = err.Error()
			}
			opencodeInst.Unlock()
			d := map[string]interface{}{"ok": err == nil}
			if err != nil {
				opencodeLogf("FAILED: %v", err)
				d["error"] = truncateStr(err.Error(), 300)
			} else if st, ok := opencodeLoadState(); ok {
				d["version"], d["sha512"] = st.Version, st.SHA512
			}
			auditLog(nil, userID, "opencode.installed", "", d)
		}()
		jsonOK(w, opencodeStatus())
	default:
		jsonError(w, "Not found", 404)
	}
}
