// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// This published Linux image already contains Python 3.12, git, SSH and rsync.
// Its WRM entrypoint is overridden; only the privately imported tool is executed.
const mediumDefaultImage = "ghcr.io/vedranius/wrm-pro@sha256:ebf63dc6df555ea142853d247cfeb74047ab1d3fb3e513bad29b08f3f2ed7652"

func mediumDockerCLI() string {
	if s := os.Getenv("WRM_MEDIUM_DOCKER"); s != "" {
		return s
	}
	if s, e := exec.LookPath("docker"); e == nil {
		return s
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{filepath.Join(os.Getenv("ProgramFiles"), "Docker", "Docker", "resources", "bin", "docker.exe"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "DockerDesktop", "resources", "bin", "docker.exe")}
	case "darwin":
		// a service's PATH often lacks these
		home, _ := os.UserHomeDir()
		candidates = []string{"/usr/local/bin/docker", "/opt/homebrew/bin/docker", "/Applications/Docker.app/Contents/Resources/bin/docker", filepath.Join(home, "Applications", "Docker.app", "Contents", "Resources", "bin", "docker"), filepath.Join(home, ".docker", "bin", "docker")}
	}
	for _, p := range candidates {
		if mediumFileExists(p) {
			return p
		}
	}
	return ""
}

func mediumImage() string {
	if s := strings.TrimSpace(os.Getenv("WRM_MEDIUM_CONTAINER_IMAGE")); s != "" {
		return s
	}
	return mediumDefaultImage
}

// mediumSelectBackend: Docker on every system that can run it (the same runtime
// everywhere, installed by WRM when missing). The host runtime stays when chosen
// (WRM_MEDIUM_RUNTIME=native), on systems without Docker, and where it was prepared
// before and the Docker runtime never was, so an existing installation keeps working.
func mediumSelectBackend(goos, configured string, nativePrepared, dockerPrepared bool) string {
	switch {
	case configured == "docker" || goos == "windows":
		return "docker"
	case configured == "native" || (goos != "linux" && goos != "darwin"):
		return "native"
	case nativePrepared && !dockerPrepared:
		return "native"
	}
	return "docker"
}

func mediumBackend() string {
	return mediumSelectBackend(runtime.GOOS, os.Getenv("WRM_MEDIUM_RUNTIME"), mediumFileExists(mediumVenvPython()), mediumFileExists(filepath.Join(mediumRoot(), "docker-ready")))
}

func mediumRuntimeReady() bool {
	if mediumFileExists(filepath.Join(mediumRoot(), "setup-pending")) {
		return false
	}
	if mediumBackend() == "docker" {
		b, e := os.ReadFile(filepath.Join(mediumRoot(), "docker-ready"))
		return e == nil && strings.TrimSpace(string(b)) == mediumImage()
	}
	return mediumFileExists(mediumVenvPython())
}

func mediumDockerCheck(ctx context.Context) error {
	if mediumDockerCLI() == "" {
		return fmt.Errorf("Medium on Windows requires Docker Desktop with Linux containers; install/start Docker on the WRM host, then click Prepare runtime. No Windows rsync/Python installation is needed")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, mediumDockerCLI(), "version", "--format", "{{.Server.Os}}")
	b, e := cmd.Output()
	if e != nil {
		return fmt.Errorf("Start Docker on the WRM host and allow the WRM process to access it, then click Prepare runtime")
	}
	if strings.TrimSpace(string(b)) != "linux" {
		return fmt.Errorf("Switch Docker Desktop to Linux containers; the original deploy tool needs a Linux runtime")
	}
	return nil
}

func mediumContainerPath(host string) string {
	rel, e := filepath.Rel(mediumRoot(), host)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return host
	}
	if rel == "." {
		return "/wrm-medium"
	}
	return "/wrm-medium/" + filepath.ToSlash(rel)
}

// Preparation/validation mounts only the private Medium data root. Runtime
// processes mount the approved source/venv read-only and their own user data.
func mediumDockerPython(ctx context.Context, python string, args ...string) error {
	name := "wrm-medium-setup-" + randomToken(8)
	defer mediumDockerStop(name)
	argv := []string{"run", "--rm", "--init", "--no-healthcheck", "--env", "PYTHONDONTWRITEBYTECODE=1", "--name", name, "--platform", "linux/amd64", "--user", "0:0", "--volume", mediumRoot() + ":/wrm-medium", "--volume", mediumDockerRuntimeVolume() + ":/wrm-medium/docker-venv", "--workdir", "/wrm-medium/tool"}
	argv = append(argv, mediumCAArgs()...)
	cargs := make([]string, 0, len(args))
	for _, arg := range args {
		if filepath.IsAbs(arg) {
			arg = mediumContainerPath(arg)
		}
		cargs = append(cargs, arg)
	}
	entry, cargs := mediumCACommand(python, cargs)
	argv = append(append(argv, "--entrypoint", entry, mediumImage()), cargs...)
	return mediumCommand(ctx, mediumRoot(), mediumDockerCLI(), argv...)
}

// mediumPrepareRuntime prepares the original tool's Python environment; step names each
// step as it starts (shown with its time in Medium).
func mediumPrepareRuntime(ctx context.Context, step func(string)) error {
	if step == nil {
		step = func(string) {}
	}
	if e := os.WriteFile(filepath.Join(mediumRoot(), "setup-pending"), []byte("pending"), 0600); e != nil {
		return e
	}
	if mediumBackend() == "docker" {
		step("docker")
		if e := mediumEnsureDocker(ctx, step); e != nil {
			return e
		}
		// the host's trusted certificates (proxies and antivirus that inspect HTTPS)
		if n, e := mediumWriteCABundle(); e != nil {
			return e
		} else if out := mediumOpOutput(); out != nil && n > 0 {
			out.Note("the runtime trusts %d certificates of the WRM host besides its own (policy medium_host_ca, setting medium_ca_file)", n)
		}
		// Docker pulls the pinned image automatically on the first invocation.
		step("image")
		if e := mediumDockerPython(ctx, "python3", "-m", "venv", "/wrm-medium/docker-venv"); e != nil {
			return fmt.Errorf("Cannot prepare the Linux runtime (Docker image %s); check that Docker can download images (internet or registry access), free disk space and the WRM data directory permissions", mediumImage())
		}
		step("packages")
		if e := mediumDockerPython(ctx, "/wrm-medium/docker-venv/bin/python", "-m", "pip", "install", "--disable-pip-version-check", "-r", filepath.Join(mediumToolDir(), "requirements.lock")); e != nil {
			if out := mediumOpOutput(); out != nil && strings.Contains(out.Tail(), "CERTIFICATE_VERIFY_FAILED") {
				return fmt.Errorf("pip inside Docker does not trust the certificate it gets for the package registry (certificate verify failed): a proxy or antivirus on this network inspects HTTPS. Turn on the policy medium_host_ca (trust the WRM host's certificates) or set medium_ca_file to the proxy's root certificate (PEM), then press Prepare runtime again")
			}
			return fmt.Errorf("Cannot install the original tool's Python packages inside Docker; check access to the package registry (pypi.org or your mirror) and the imported requirements.lock")
		}
		if e := os.WriteFile(filepath.Join(mediumRoot(), "docker-ready"), []byte(mediumImage()), 0600); e != nil {
			return e
		}
		return os.Remove(filepath.Join(mediumRoot(), "setup-pending"))
	}
	step("python")
	if mediumPython() == "" {
		return fmt.Errorf("Install Python 3.12+, git, SSH and rsync on the WRM host, or use the managed Docker runtime (WRM_MEDIUM_RUNTIME=docker)")
	}
	if e := mediumCommand(ctx, mediumRoot(), mediumPython(), "-c", "import sys;sys.exit(0 if sys.version_info >= (3,12) else 1)"); e != nil {
		return fmt.Errorf("Python 3.12+ is required; select it with WRM_MEDIUM_PYTHON or use the managed Docker runtime")
	}
	step("venv")
	if e := mediumCommand(ctx, mediumRoot(), mediumPython(), "-m", "venv", filepath.Join(mediumRoot(), "venv")); e != nil {
		return e
	}
	step("packages")
	if e := mediumCommand(ctx, mediumRoot(), mediumVenvPython(), "-m", "pip", "install", "--disable-pip-version-check", "-r", filepath.Join(mediumToolDir(), "requirements.lock")); e != nil {
		return fmt.Errorf("Cannot install the original tool's Python packages; check access to the package registry (pypi.org or your mirror) and the imported requirements.lock")
	}
	return os.Remove(filepath.Join(mediumRoot(), "setup-pending"))
}

func mediumValidateConfig(ctx context.Context, stage string) error {
	args := []string{"-c", "import sys; from pathlib import Path; from app.config import load_config; load_config(Path(sys.argv[1]))", stage}
	if mediumBackend() == "docker" {
		return mediumDockerPython(ctx, "/wrm-medium/docker-venv/bin/python", args...)
	}
	return mediumCommand(ctx, mediumToolDir(), mediumVenvPython(), args...)
}

func mediumDockerStartArgs(uid, port int, name string, envNames []string) []string {
	args := []string{"run", "--rm", "--init", "--no-healthcheck", "--env", "PYTHONDONTWRITEBYTECODE=1", "--platform", "linux/amd64", "--name", name, "--label", "wrm.medium=true", "--user", "0:0", "--publish", fmt.Sprintf("127.0.0.1:%d:8080", port), "--volume", mediumToolDir() + ":/wrm-medium/tool:ro", "--volume", mediumDockerRuntimeVolume() + ":/wrm-medium/docker-venv:ro", "--volume", mediumDockerVolume(uid) + ":/wrm-medium/users/" + strconv.Itoa(uid), "--workdir", "/wrm-medium/users/" + strconv.Itoa(uid)}
	args = append(args, mediumCAArgs()...)
	for _, key := range envNames {
		args = append(args, "--env", key) // values/bridge secret stay out of argv
	}
	entry, cargs := mediumCACommand("/wrm-medium/docker-venv/bin/python", []string{"-m", "uvicorn", "medium_wrapper:app", "--host", "0.0.0.0", "--port", "8080", "--no-proxy-headers", "--no-access-log"})
	return append(append(args, "--entrypoint", entry, mediumImage()), cargs...)
}

func mediumDockerVolume(uid int) string {
	h := sha256.Sum256([]byte(mediumRoot()))
	return "wrm-medium-" + hex.EncodeToString(h[:8]) + "-u" + strconv.Itoa(uid)
}
func mediumDockerRuntimeVolume() string {
	h := sha256.Sum256([]byte(mediumRoot()))
	return "wrm-medium-" + hex.EncodeToString(h[:8]) + "-runtime"
}

// A real Linux volume preserves POSIX modes, symlinks and native deploy state on
// Windows. Never expose the Windows SSH profile bind mount to branch builds.
func mediumDockerSync(ctx context.Context, uid int) error {
	dir := mediumUserDir(uid)
	target := "/wrm-medium/users/" + strconv.Itoa(uid)
	statePath := filepath.Join(dir, "docker-sync.json")
	previous := map[string]string{}
	b, e := os.ReadFile(statePath)
	first := e != nil
	if !first && json.Unmarshal(b, &previous) != nil {
		return fmt.Errorf("Cannot read private Docker runtime sync state")
	}
	current := map[string]string{}
	for _, kind := range []string{"config", "ssh"} {
		b, _ := os.ReadFile(filepath.Join(dir, kind+"-revision"))
		current[kind] = string(b)
	}
	name := "wrm-medium-init-" + strconv.Itoa(uid) + "-" + randomToken(8)
	if e := mediumCommand(ctx, dir, mediumDockerCLI(), "create", "--platform", "linux/amd64", "--name", name, "--user", "0:0", "--volume", mediumDockerVolume(uid)+":"+target, "--entrypoint", "/bin/sh", mediumImage(), "-c", "chmod 700 "+target+"; chmod -R go-rwx "+target+"/home "+target+"/config; chmod 600 "+target+"/medium_wrapper.py"); e != nil {
		return e
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanup, mediumDockerCLI(), "rm", "-f", name).Run()
	}()
	copyIn := func(host, destination string) error {
		return mediumCommand(ctx, dir, mediumDockerCLI(), "cp", host, name+":"+destination)
	}
	if first {
		if e := copyIn(dir+string(filepath.Separator)+".", target); e != nil {
			return e
		}
	} else {
		for _, kind := range []string{"config", "ssh"} {
			if current[kind] == previous[kind] {
				continue
			}
			rel := "config"
			if kind == "ssh" {
				rel = filepath.Join("home", ".ssh")
			}
			// A ZIP import replaces that profile explicitly; avoid merging stale keys/settings.
			if e := mediumCommand(ctx, dir, mediumDockerCLI(), "run", "--rm", "--platform", "linux/amd64", "--user", "0:0", "--volume", mediumDockerVolume(uid)+":"+target, "--entrypoint", "python3", mediumImage(), "-c", "import shutil,sys;shutil.rmtree(sys.argv[1],ignore_errors=True)", target+"/"+filepath.ToSlash(rel)); e != nil {
				return e
			}
			if e := copyIn(filepath.Join(dir, rel), target+"/"+filepath.ToSlash(rel)); e != nil {
				return e
			}
		}
		if e := copyIn(filepath.Join(dir, "medium_wrapper.py"), target+"/medium_wrapper.py"); e != nil {
			return e
		}
	}
	if e := mediumCommand(ctx, dir, mediumDockerCLI(), "start", "--attach", name); e != nil {
		return e
	}
	return os.WriteFile(statePath, jsonMarshal(current), 0600)
}

func mediumDockerStop(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, mediumDockerCLI(), "stop", "--time", "2", name).Run()
}
