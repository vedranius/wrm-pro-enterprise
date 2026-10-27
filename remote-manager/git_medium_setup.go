// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func mediumAnyRunning() bool {
	mediumMgr.mu.Lock()
	defer mediumMgr.mu.Unlock()
	for _, p := range mediumMgr.processes {
		select {
		case <-p.done:
		default:
			return true
		}
	}
	return false
}

// Uploads are retained before dependency setup. A failed/closed browser request
// cannot lose approved archives; the same button resumes pending configuration.
func apiMediumSetup(w http.ResponseWriter, r *http.Request, uid int) {
	if !requireMethod(w, r, "POST") {
		return
	}
	if r.URL.Query().Get("confirm") != "1" {
		jsonError(w, "Confirm that you trust the deploy tool/configuration", 400)
		return
	}
	h, err := mediumBeginOp(uid, "setup", "")
	if err != nil {
		jsonError(w, err.Error(), 409)
		return
	}
	handed := false
	var failure error
	defer func() {
		if !handed {
			mediumEndOp(h, failure)
		}
	}()
	// every refusal before the background job is the operation's outcome too
	w = &mediumFailWriter{ResponseWriter: w, fail: &failure}
	mediumSetStage(uid, "upload", true, nil)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, "Cannot read ZIP uploads (20 MiB each)", 400)
		return
	}
	defer r.MultipartForm.RemoveAll()
	if err := os.MkdirAll(mediumRoot(), 0700); err != nil {
		jsonError(w, "Cannot create private runtime directory", 500)
		return
	}
	staged := map[string]string{}
	digests := map[string]string{}
	defer func() {
		for _, p := range staged {
			os.RemoveAll(p)
		}
	}()
	for _, kind := range []string{"tool", "config", "ssh"} {
		f, _, err := r.FormFile(kind)
		if err == http.ErrMissingFile {
			continue
		}
		if err != nil {
			jsonError(w, "Invalid ZIP upload", 400)
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, mediumArchiveLimit+1))
		f.Close()
		if err != nil || len(data) > mediumArchiveLimit {
			jsonError(w, "ZIP exceeds 20 MiB", 413)
			return
		}
		if kind == "tool" && !isAdminUser(uid) {
			jsonError(w, "Only administrators can install deploy-tool code", 403)
			return
		}
		p, err := os.MkdirTemp(mediumRoot(), "one-click-")
		if err != nil {
			jsonError(w, "Cannot stage ZIP", 500)
			return
		}
		staged[kind] = p
		if err := mediumExtract(data, p, kind); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if kind == "config" {
			// WRM's own reader checks deploy-config at once; the original tool checks it
			// again before it is used
			if _, err := mdLoadConfig(p); err != nil {
				jsonError(w, "deploy-config is not valid: "+err.Error(), 400)
				return
			}
		}
		sum := sha256.Sum256(data)
		digests[kind] = hex.EncodeToString(sum[:])
	}
	if mediumAnyRunning() {
		jsonError(w, "Stop active Medium instances before combined setup, including retained pending imports", 409)
		return
	}
	if staged["tool"] == "" && !mediumState(uid).Imported {
		jsonError(w, "Choose the trusted deploy-tool ZIP", 400)
		return
	}
	if !isAdminUser(uid) && !mediumRuntimeReady() {
		jsonError(w, "An administrator must prepare the shared runtime first", 403)
		return
	}
	if !mediumState(uid).Config && staged["config"] == "" && !mediumFileExists(filepath.Join(mediumUserDir(uid), "pending-config", "servers.yaml")) {
		jsonError(w, "Choose the deploy-config ZIP", 400)
		return
	}
	for _, kind := range []string{"tool", "config", "ssh"} {
		if p := staged[kind]; p != "" {
			var err error
			if kind == "tool" {
				err = mediumReplaceDir(p, mediumToolDir())
			} else {
				err = mediumKeepPending(uid, kind, p)
			}
			if err != nil {
				jsonError(w, "Cannot retain approved ZIP files", 500)
				return
			}
			delete(staged, kind)
			if kind == "tool" {
				os.WriteFile(filepath.Join(mediumRoot(), "tool.sha256"), []byte(digests[kind]), 0600)
				os.WriteFile(filepath.Join(mediumRoot(), "setup-pending"), []byte("pending"), 0600)
			}
		}
	}
	handed = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
		defer cancel()
		mediumEndOp(h, mediumSetup(ctx, uid))
	}()
	auditLog(r, uid, "git.medium_setup_started", "", map[string]interface{}{"archives": len(digests)})
	w.WriteHeader(http.StatusAccepted)
	jsonOK(w, mediumState(uid))
}
func mediumSetup(ctx context.Context, uid int) error {
	if mediumBackend() == "docker" {
		step := func(st string) { mediumSetStage(uid, st, true, nil) }
		step("docker")
		if err := mediumEnsureDocker(ctx, step); err != nil {
			return err
		}
	}
	if !mediumRuntimeReady() {
		if err := mediumPrepareRuntime(ctx, func(step string) { mediumSetStage(uid, step, true, nil) }); err != nil {
			return err
		}
	}
	if err := mediumApplyPending(ctx, uid); err != nil {
		return err
	}
	return mediumStartForSetup(uid, true)
}

// mediumPendingMu serializes the renames of kept ZIPs (pending-<kind>): an upload that
// is kept while an operation runs never mixes with the operation taking its snapshot.
var mediumPendingMu sync.Mutex

// mediumKeepPending makes stage the user's kept <kind> ZIP (replacing an older one).
func mediumKeepPending(uid int, kind, stage string) error {
	mediumPendingMu.Lock()
	defer mediumPendingMu.Unlock()
	return mediumReplaceDir(stage, filepath.Join(mediumUserDir(uid), "pending-"+kind))
}

// mediumApplyPending checks kept deploy-config with the original tool and applies kept
// deploy-config and SSH profile ZIPs (setup, start, or an import when nothing else runs).
// It works on a snapshot, so a ZIP kept meanwhile waits for the next start. A deploy-config
// the original tool rejects is discarded (the previous one stays in use); a check that
// could not run keeps the ZIP for a retry.
func mediumApplyPending(ctx context.Context, uid int) error {
	for _, kind := range []string{"config", "ssh"} {
		pending := filepath.Join(mediumUserDir(uid), "pending-"+kind)
		if st, e := os.Stat(pending); e != nil || !st.IsDir() {
			continue
		}
		if !mediumRuntimeReady() {
			return fmt.Errorf("The original tool's runtime is not prepared yet, so the imported %s cannot be checked; an administrator runs \"Prepare runtime\" first", kind)
		}
		snap := pending + ".applying"
		mediumPendingMu.Lock()
		os.RemoveAll(snap)
		e := os.Rename(pending, snap)
		mediumPendingMu.Unlock()
		if e != nil {
			continue // replaced or taken meanwhile
		}
		// keepForRetry puts the snapshot back unless a newer ZIP was kept meanwhile
		keepForRetry := func() {
			mediumPendingMu.Lock()
			defer mediumPendingMu.Unlock()
			if _, e := os.Stat(pending); e == nil || os.Rename(snap, pending) != nil {
				os.RemoveAll(snap)
			}
		}
		dest := filepath.Join(mediumUserDir(uid), "config")
		if kind == "config" {
			if mediumBackend() == "docker" {
				mediumSetStage(uid, "docker", true, nil)
				if err := mediumDockerCheck(ctx); err != nil {
					keepForRetry()
					return err
				}
			}
			mediumSetStage(uid, "config_check", true, nil)
			if err := mediumValidateConfig(ctx, snap); err != nil {
				if ctx.Err() != nil || !mediumToolRejected(err) {
					keepForRetry()
					if ctx.Err() != nil {
						return fmt.Errorf("The deploy-config check was cancelled or took too long; your ZIP is kept, try again")
					}
					return fmt.Errorf("The original tool's deploy-config check could not run (%s); your ZIP is kept, try again", truncateStr(err.Error(), 300))
				}
				os.RemoveAll(snap)
				return fmt.Errorf("The original tool rejected deploy-config (its output in the Medium log names the file and the problem). Fix the ZIP and import it again; the previous deploy-config stays in use")
			}
		} else {
			mediumSetStage(uid, "ssh_apply", true, nil)
			dest = filepath.Join(mediumUserDir(uid), "home", ".ssh")
		}
		if mediumState(uid).Running {
			keepForRetry()
			return fmt.Errorf("Stop the original deploy tool before its %s is replaced", kind)
		}
		if err := mediumReplaceDir(snap, dest); err != nil {
			keepForRetry()
			return err
		}
		if kind == "config" {
			if prefs := mediumPreferences(uid); prefs["DEPLOY_CONFIG_REPO"] != "" {
				delete(prefs, "DEPLOY_CONFIG_REPO")
				if err := os.WriteFile(filepath.Join(mediumUserDir(uid), "preferences.enc"), []byte(encryptValue(string(jsonMarshal(prefs)))), 0600); err != nil {
					return err
				}
			}
		}
		if err := os.WriteFile(filepath.Join(mediumUserDir(uid), kind+"-revision"), []byte(randomToken(16)), 0600); err != nil {
			return err
		}
	}
	return nil
}

// mediumToolRejected: did the check run and the original tool's Python exit non-zero
// (not Docker or the runtime failing to start it)?
func mediumToolRejected(err error) bool {
	var x *exec.ExitError
	if !errors.As(err, &x) {
		return false
	}
	switch x.ExitCode() {
	case 125, 126, 127, -1: // docker run's own errors, not executable, not found, killed
		return false
	}
	return true
}

// mediumFailWriter remembers the message of an error response as the operation's outcome.
type mediumFailWriter struct {
	http.ResponseWriter
	fail   *error
	status int
}

func (m *mediumFailWriter) WriteHeader(code int) {
	m.status = code
	m.ResponseWriter.WriteHeader(code)
}
func (m *mediumFailWriter) Write(b []byte) (int, error) {
	if m.status >= 400 && *m.fail == nil {
		var v struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &v) == nil && v.Error != "" {
			*m.fail = fmt.Errorf("%s", v.Error)
		}
	}
	return m.ResponseWriter.Write(b)
}

// mediumEnsureDocker makes Docker answer with Linux containers: it installs Docker when
// it is missing (mediumInstallDocker), starts it and waits for its engine.
func mediumEnsureDocker(ctx context.Context, step func(string)) error {
	if step == nil {
		step = func(string) {}
	}
	if mediumDockerCheck(ctx) == nil {
		return nil
	}
	if mediumDockerCLI() == "" {
		// a package manager killed mid-transaction leaves the host broken: the installation
		// has its own time budget and is not cut by the operation's deadline
		ictx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Minute)
		err := mediumInstallDocker(ictx, step)
		cancel()
		if err != nil {
			return err
		}
		if mediumDockerCheck(ctx) == nil {
			return nil
		}
	}
	cli := mediumDockerCLI()
	if cli == "" {
		return fmt.Errorf("The Docker installation did not finish: no docker command was found afterwards. The output shows the installer's messages; press Install Docker or Prepare runtime again. Your ZIPs are kept")
	}
	if err := mediumDockerAccess(ctx); err != nil {
		return err
	}
	step("docker_start")
	// Docker Desktop starts with its own command; Docker Engine is a service
	err := fmt.Errorf("linux")
	if mediumHostOS() != "linux" {
		startCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		err = mediumCommand(startCtx, mediumRoot(), cli, "desktop", "start", "--timeout", "50")
		cancel()
	}
	if err != nil {
		switch mediumHostOS() {
		case "windows":
			for _, base := range []string{filepath.Join(os.Getenv("ProgramFiles"), "Docker", "Docker"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "DockerDesktop")} {
				p := filepath.Join(base, "Docker Desktop.exe")
				if mediumFileExists(p) {
					cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Start-Process -FilePath '"+strings.ReplaceAll(p, "'", "''")+"'")
					_ = cmd.Run()
					break
				}
			}
		case "darwin":
			if mediumEUID() == 0 {
				// a root daemon has no screen session: open Docker Desktop in the signed-in user's
				if who := mediumConsoleUser(ctx); who != "" {
					if b, err := exec.CommandContext(ctx, "id", "-u", who).Output(); err == nil {
						_ = mediumCommand(ctx, mediumRoot(), "launchctl", "asuser", strings.TrimSpace(string(b)), "sudo", "-u", who, "open", "-a", "Docker")
					}
				}
			} else {
				_ = mediumCommand(ctx, mediumRoot(), "open", "-a", "Docker")
			}
		case "linux":
			priv := mediumPrivilege(ctx)
			for _, args := range [][]string{{"systemctl", "--user", "start", "docker"}, mediumAsRoot(priv, "systemctl", "start", "docker")} {
				cctx, stop := context.WithTimeout(ctx, 30*time.Second)
				_ = mediumRunArgv(cctx, args)
				stop()
				if mediumDockerCheck(ctx) == nil {
					return nil
				}
			}
		}
	}
	// Do not switch a host with running Windows containers or change the global
	// Docker context; those can belong to other applications.
	versionCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	b, _ := exec.CommandContext(versionCtx, cli, "version", "--format", "{{.Server.Os}}").Output()
	stop()
	if strings.TrimSpace(string(b)) == "windows" {
		return fmt.Errorf("Docker is using Windows containers. Switch to Linux containers in Docker Desktop, then resume; no existing containers were stopped")
	}
	step("docker_wait")
	deadline := time.NewTimer(mediumDockerWait)
	defer deadline.Stop()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		if mediumDockerCheck(ctx) == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			switch mediumHostOS() {
			case "linux":
				return fmt.Errorf("The Docker engine did not answer within %s. Check its service (sudo systemctl status docker; on WSL without systemd: sudo service docker start), then press Install Docker or Prepare runtime. Your ZIPs are kept", mediumDockerWait)
			case "darwin":
				return fmt.Errorf("Docker Desktop did not become ready within %s. On the Mac, finish Docker Desktop's first-start window if it is open, then press Install Docker or Prepare runtime. Your ZIPs are kept", mediumDockerWait)
			}
			return fmt.Errorf("The Docker Linux engine did not become ready within %s. Finish Docker Desktop's prompts on this PC (WSL, virtualisation) or restart Windows if it asked, then press Install Docker or Prepare runtime. Your ZIPs are kept", mediumDockerWait)
		case <-tick.C:
		}
	}
}

var mediumDockerWait = 3 * time.Minute

// mediumDockerAccess reports a Docker that runs but refuses WRM's account.
func mediumDockerAccess(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, _ := exec.CommandContext(cctx, mediumDockerCLI(), "version", "--format", "{{.Server.Os}}").CombinedOutput()
	if !strings.Contains(strings.ToLower(string(b)), "permission denied") {
		return nil
	}
	u := mediumCurrentUser()
	return fmt.Errorf("Docker runs, but WRM's account %s may not use it (permission denied on the Docker socket). Run once: %s (adds %s to the docker group and restarts WRM), then press Install Docker or Prepare runtime. Your ZIPs are kept", u, mediumSetupCommand(), u)
}
