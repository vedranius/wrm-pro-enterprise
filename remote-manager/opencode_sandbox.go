// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/cpu"
)

// ─── OPENCODE: DOCKER SANDBOX ────────────────────────
//
// With ai_opencode_sandbox an OpenCode agent can also use its own tools (shell, files, web
// fetch), but only inside a Docker container on the WRM host: the image is
// ai_opencode_sandbox_image, the only mount is the session's own folder (/work), all
// capabilities are dropped, the root file system is read-only, memory, CPU and processes
// are limited. OpenCode itself runs in the container (the Linux build of the installed
// version), publishes its API on 127.0.0.1 only and reaches WRM's MCP server through the
// Docker host gateway. Every shell command, file change and web fetch asks OpenCode's
// permission system, which WRM turns into the usual approval card in the chat (ocAsk):
// nothing runs in the container without the user's approval, also with full access.
// Server actions still go through the wrm_ tools and WRM's permission engine.

const opencodeSandboxDefaultImage = "debian:stable-slim"

// opencodeSandboxTools are OpenCode's own tools that the sandbox turns on. The shell, file
// changes and web fetches ask; reading inside /work does not.
var opencodeSandboxTools = []string{"bash", "read", "glob", "grep", "list", "edit", "write", "patch", "apply_patch", "webfetch", "todowrite"}

// opencodeSandboxAsk are the OpenCode permissions that become a WRM approval.
var opencodeSandboxAsk = []string{"bash", "edit", "webfetch"}

var ocDocker struct {
	sync.Mutex
	at        time.Time
	os, arch  string
	err       error
	gateway   string
	gatewayAt time.Time
}

// opencodeDockerServer is the Docker server's OS and architecture (cached for a minute).
func opencodeDockerServer() (string, string, error) {
	ocDocker.Lock()
	defer ocDocker.Unlock()
	if time.Since(ocDocker.at) < time.Minute {
		return ocDocker.os, ocDocker.arch, ocDocker.err
	}
	ocDocker.at = time.Now()
	ocDocker.os, ocDocker.arch, ocDocker.err = "", "", nil
	cli := mediumDockerCLI()
	if cli == "" {
		ocDocker.err = errors.New("Docker is not installed on the WRM host (Medium step 2 can install it)")
		return "", "", ocDocker.err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cli, "version", "--format", "{{.Server.Os}}/{{.Server.Arch}}").Output()
	if err != nil {
		ocDocker.err = errors.New("Docker does not answer on the WRM host: start it and allow WRM's account to use it")
		return "", "", ocDocker.err
	}
	ocDocker.os, ocDocker.arch, _ = strings.Cut(strings.TrimSpace(string(out)), "/")
	if ocDocker.os != "linux" {
		ocDocker.err = errors.New("Docker runs " + ocDocker.os + " containers; switch it to Linux containers")
	}
	return ocDocker.os, ocDocker.arch, ocDocker.err
}

// opencodeSandboxPackage is the npm package of the Linux build for the Docker server.
func opencodeSandboxPackage(arch string) string {
	switch arch {
	case "amd64":
		if !cpu.X86.HasAVX2 {
			return "opencode-linux-x64-baseline"
		}
		return "opencode-linux-x64"
	case "arm64":
		return "opencode-linux-arm64"
	}
	return ""
}

// opencodeSandboxAllowed: may this user start an agent with the Docker sandbox, and why not.
func opencodeSandboxAllowed(userID int) (bool, string) {
	switch v := getSetting("ai_opencode_sandbox"); {
	case v == "off":
		return false, "the Docker sandbox for OpenCode is off (policy ai_opencode_sandbox)"
	case !policyAllows(userID, v):
		return false, "the Docker sandbox for OpenCode is limited to " + policyLimitText(v)
	}
	if mediumInContainer() {
		return false, "WRM itself runs in a container; the Docker sandbox needs WRM on the host"
	}
	if _, err := opencodeSandboxBinary(); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// opencodeSandboxBinary is the Linux OpenCode binary mounted into the container.
func opencodeSandboxBinary() (string, error) {
	_, arch, err := opencodeDockerServer()
	if err != nil {
		return "", err
	}
	pkg := opencodeSandboxPackage(arch)
	if pkg == "" {
		return "", fmt.Errorf("OpenCode has no Linux build for %s", arch)
	}
	if p := strings.TrimSpace(getSetting("ai_opencode_path")); p != "" && runtime.GOOS == "linux" && runtime.GOARCH == arch {
		// an own binary on a Linux host of the same architecture runs in the container too
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	st, ok := opencodeLoadState()
	if !ok {
		return "", errors.New("OpenCode is not installed yet: an administrator installs it once under Administration → AI → OpenCode")
	}
	if st.Package == pkg {
		return st.Path, nil
	}
	if p := st.Sandbox[pkg]; p != "" {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("the Linux build of OpenCode for the Docker sandbox (%s) is not installed: press Install in Administration → AI → OpenCode", pkg)
}

// opencodeInstallSandbox installs the Linux build for the Docker server when the host's own
// build cannot run there, pulls the sandbox image and runs opencode --version inside it.
func opencodeInstallSandbox(ctx context.Context, version string) error {
	_, arch, err := opencodeDockerServer()
	if err != nil {
		return err
	}
	pkg := opencodeSandboxPackage(arch)
	if pkg == "" {
		return fmt.Errorf("OpenCode has no Linux build for %s", arch)
	}
	st, ok := opencodeLoadState()
	if !ok {
		return errors.New("install OpenCode first")
	}
	bin := st.Path
	if st.Package != pkg {
		bin = filepath.Join(opencodeRoot(), "bin", version, pkg, "opencode")
		opencodeLogf("Docker sandbox: installing the Linux build %s", pkg)
		if _, err := opencodeFetch(ctx, pkg, version, bin); err != nil {
			return err
		}
	}
	image := opencodeSandboxImage()
	opencodeLogf("Docker sandbox: pulling %s", image)
	pctx, pcancel := context.WithTimeout(ctx, 10*time.Minute)
	defer pcancel()
	if out, err := exec.CommandContext(pctx, mediumDockerCLI(), "pull", image).CombinedOutput(); err != nil {
		return fmt.Errorf("docker pull %s failed: %v %s", image, err, truncateStr(strings.TrimSpace(string(out)), 400))
	}
	vctx, vcancel := context.WithTimeout(ctx, 2*time.Minute)
	defer vcancel()
	out, err := exec.CommandContext(vctx, mediumDockerCLI(), "run", "--rm", "--network", "none", "--volume", bin+":/opt/opencode/opencode:ro",
		"--entrypoint", "/opt/opencode/opencode", image, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("OpenCode does not start in %s: %v %s", image, err, truncateStr(strings.TrimSpace(string(out)), 400))
	}
	opencodeLogf("Docker sandbox: opencode --version in %s: %s", image, strings.TrimSpace(truncateStr(string(out), 100)))
	if st.Package != pkg {
		st, _ = opencodeLoadState()
		if st.Sandbox == nil {
			st.Sandbox = map[string]string{}
		}
		st.Sandbox[pkg] = bin
		return opencodeSaveState(st)
	}
	return nil
}

func opencodeSandboxImage() string {
	return nonEmpty(strings.TrimSpace(getSetting("ai_opencode_sandbox_image")), opencodeSandboxDefaultImage)
}

// opencodeSandboxMCPURL is the MCP address a sandbox container uses. On Linux WRM listens
// on the Docker bridge's gateway address (host.docker.internal inside the container); with
// Docker Desktop host.docker.internal reaches the host's loopback listener.
func opencodeSandboxMCPURL() (string, error) {
	bind := "127.0.0.1"
	if runtime.GOOS == "linux" {
		gw, err := opencodeDockerGateway()
		if err != nil {
			return "", err
		}
		bind = gw
	}
	port, err := opencodeMCPListen(bind)
	if err != nil {
		return "", err
	}
	return "http://host.docker.internal:" + strconv.Itoa(port) + "/mcp", nil
}

// opencodeDockerGateway is the gateway address of Docker's default bridge network.
func opencodeDockerGateway() (string, error) {
	ocDocker.Lock()
	if ocDocker.gateway != "" && time.Since(ocDocker.gatewayAt) < 10*time.Minute {
		gw := ocDocker.gateway
		ocDocker.Unlock()
		return gw, nil
	}
	ocDocker.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, mediumDockerCLI(), "network", "inspect", "bridge", "--format", "{{range .IPAM.Config}}{{.Gateway}} {{end}}").Output()
	if err != nil {
		return "", fmt.Errorf("cannot read Docker's bridge network: %v", err)
	}
	for _, f := range strings.Fields(string(out)) {
		if !strings.Contains(f, ":") { // the IPv4 gateway
			ocDocker.Lock()
			ocDocker.gateway, ocDocker.gatewayAt = f, time.Now()
			ocDocker.Unlock()
			return f, nil
		}
	}
	return "", errors.New("Docker's bridge network has no IPv4 gateway")
}

// opencodeSandboxLabel marks this WRM's sandbox containers (several WRMs can share Docker).
func opencodeSandboxLabel() string {
	h := sha256.Sum256([]byte(opencodeRoot()))
	return "wrm.opencode=" + hex.EncodeToString(h[:8])
}

// opencodeSandboxCA writes the WRM host's trusted roots into the session folder, so OpenCode
// in the container trusts what the host trusts (proxies that inspect HTTPS).
func opencodeSandboxCA(dir string) bool {
	roots, err := mediumHostRoots()
	if err != nil || len(roots) == 0 {
		return false
	}
	var b strings.Builder
	now := time.Now()
	for _, c := range roots {
		if now.Before(c.NotAfter) {
			b.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}))
		}
	}
	return os.WriteFile(filepath.Join(dir, "ca.pem"), []byte(b.String()), 0o644) == nil
}

// opencodeSandboxArgs are the docker run arguments of one sandbox. Values of envNames come
// from the docker CLI's environment, so keys and tokens never appear in the command line.
func opencodeSandboxArgs(name, dir, bin string, port int, envNames []string, ca bool) []string {
	user := "0:0"
	if runtime.GOOS != "windows" {
		user = strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	}
	args := []string{"run", "--rm", "--init", "--name", name, "--label", opencodeSandboxLabel(), "--user", user,
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--read-only", "--tmpfs", "/tmp:rw,size=512m",
		"--memory", "2g", "--cpus", "2", "--pids-limit", "512",
		"--add-host", "host.docker.internal:host-gateway",
		"--publish", "127.0.0.1:" + strconv.Itoa(port) + ":4096",
		"--volume", dir + ":/work", "--volume", bin + ":/opt/opencode/opencode:ro", "--workdir", "/work/work"}
	for _, k := range envNames {
		args = append(args, "--env", k)
	}
	if ca {
		args = append(args, "--env", "NODE_EXTRA_CA_CERTS=/work/ca.pem")
	}
	return append(args, "--entrypoint", "/opt/opencode/opencode", opencodeSandboxImage(), "serve", "--port", "4096", "--hostname", "0.0.0.0")
}

// opencodeSandboxEnv is the environment inside the container (paths under /work).
func opencodeSandboxEnv() []string {
	home := "/work/home"
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + home + "/.config", "XDG_DATA_HOME=" + home + "/.local/share",
		"XDG_STATE_HOME=" + home + "/.local/state", "XDG_CACHE_HOME=" + home + "/.cache", "TMPDIR=/tmp",
		"OPENCODE_DISABLE_AUTOUPDATE=1", "OPENCODE_DISABLE_PROJECT_CONFIG=1", "OPENCODE_DISABLE_LSP_DOWNLOAD=1", "OPENCODE_DISABLE_CLAUDE_CODE=1",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS=1", "OPENCODE_DISABLE_EXTERNAL_SKILLS=1", "OPENCODE_DISABLE_TERMINAL_TITLE=1"}
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "TZ"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// ocDockerRemove removes a sandbox container (it may still run after its docker CLI died).
func ocDockerRemove(name string) {
	if name == "" || mediumDockerCLI() == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	exec.CommandContext(ctx, mediumDockerCLI(), "rm", "--force", name).Run()
}

// opencodeSandboxCleanup removes sandbox containers a previous run of this WRM left behind.
func opencodeSandboxCleanup() {
	if mediumDockerCLI() == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, mediumDockerCLI(), "ps", "--all", "--quiet", "--filter", "label="+opencodeSandboxLabel()).Output()
	if err != nil {
		return
	}
	for _, id := range strings.Fields(string(out)) {
		exec.CommandContext(ctx, mediumDockerCLI(), "rm", "--force", id).Run()
		log.Printf("OpenCode: removed sandbox container %s left from a previous run", id)
	}
}

// ocPermission is OpenCode's permission request (event permission.asked).
type ocPermission struct {
	ID         string                 `json:"id"`
	SessionID  string                 `json:"sessionID"`
	Permission string                 `json:"permission"`
	Type       string                 `json:"type"`
	Patterns   []string               `json:"patterns"`
	Metadata   map[string]interface{} `json:"metadata"`
	Tool       struct {
		MessageID string `json:"messageID"`
		CallID    string `json:"callID"`
	} `json:"tool"`
}

// ocSandboxAsks reports an OpenCode tool whose use in the sandbox needs an approval.
func ocSandboxAsks(tool string) bool {
	return oneOf(tool, "bash", "edit", "write", "patch", "apply_patch", "webfetch")
}

// ocAsk turns a permission request of a sandboxed OpenCode into a WRM approval card and
// answers OpenCode with the user's decision. The command or change cannot be edited in
// WRM (OpenCode runs it as asked); it is approved once or refused, never automatically.
func (s *aiSession) ocAsk(o *ocProc, p ocPermission) {
	reply := func(answer, msg string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		body := map[string]string{"reply": answer}
		if msg != "" {
			body["message"] = msg
		}
		if err := o.call(ctx, http.MethodPost, "/permission/"+p.ID+"/reply", body, nil); err != nil {
			o.logf("permission reply failed: %v", err)
		}
	}
	meta := func(k string) string {
		v, _ := p.Metadata[k].(string)
		return v
	}
	if !oneOf(p.Permission, opencodeSandboxAsk...) {
		reply("reject", "WRM allows only the shell, file changes and web fetches in the sandbox.")
		s.audit(nil, "ai.opencode_refused", map[string]interface{}{"permission": p.Permission, "sandbox": true})
		return
	}
	tool := "opencode:" + p.Permission + " · sandbox"
	pl := &aiPlanned{NoEdit: true}
	switch p.Permission {
	case "bash":
		cmd := nonEmpty(meta("command"), strings.Join(p.Patterns, "; "))
		pl.Act, pl.Display = aiAction{Tool: tool, Kind: "command", Command: cmd}, cmd
	case "edit":
		path := nonEmpty(meta("filepath"), strings.Join(p.Patterns, ", "))
		diff, _ := redactSecrets(truncateStr(meta("diff"), 200000))
		pl.Write, pl.Path, pl.Diff, pl.Display = true, path, diff, "edit "+path
		pl.Act = aiAction{Tool: tool, Kind: "write", Path: path}
	case "webfetch":
		u := nonEmpty(meta("url"), strings.Join(p.Patterns, " "))
		pl.Act, pl.Display = aiAction{Tool: tool, Kind: "command", Command: "webfetch " + u}, "webfetch "+u
	}
	callID := "oc-perm-" + p.ID
	card := true
	if p.Tool.CallID != "" {
		callID = "oc-" + p.Tool.CallID
		o.mu.Lock()
		card = !o.cards[p.Tool.CallID]
		if o.cards == nil {
			o.cards = map[string]bool{}
		}
		o.cards[p.Tool.CallID] = true
		o.mu.Unlock()
	}
	why := "OpenCode's own " + p.Permission + " in its Docker sandbox on the WRM host (not on the server); every use needs your approval"
	if card {
		s.emit("tool_call", map[string]interface{}{"call_id": callID, "tool": tool, "display": redactText(pl.Display), "decision": "approve", "rule": "opencode_sandbox", "why": why})
	}
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	r := s.requestApproval(ctx, aiToolCall{ID: callID, Name: tool}, pl, "", why)
	if r.Outcome == "approved" {
		reply("once", "")
		return
	}
	msg := "The user did not approve this in WRM (" + r.Outcome + ")."
	if r.Note != "" {
		msg += " Note from the user: " + r.Note
	}
	reply("reject", msg)
}

// ocProxy carries a sandbox's requests to an AI provider on the WRM host's loopback
// address (Ollama, a local gateway): on Linux the container cannot reach 127.0.0.1, so WRM
// listens on the Docker gateway and forwards only connections from that container.
type ocProxy struct {
	ln    net.Listener
	allow atomic.Value // the container's IP address once it is known
}

func (f *ocProxy) Close() {
	if f != nil {
		f.ln.Close()
	}
}

// ocSandboxProvider returns the base URL the sandbox uses for a provider and, for a provider
// on the host's loopback address on Linux, the forwarder that serves it.
func ocSandboxProvider(base string) (string, *ocProxy, error) {
	u, err := url.Parse(base)
	if err != nil || runtime.GOOS != "linux" || !oneOf(u.Hostname(), "127.0.0.1", "localhost", "::1") {
		return base, nil, nil // opencodeConfig points loopback URLs at host.docker.internal
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"https": "443"}[u.Scheme]
		if port == "" {
			port = "80"
		}
	}
	target := net.JoinHostPort(u.Hostname(), port)
	gw, err := opencodeDockerGateway()
	if err != nil {
		return "", nil, err
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(gw, "0"))
	if err != nil {
		return "", nil, fmt.Errorf("cannot listen on the Docker gateway for the local AI provider: %v", err)
	}
	f := &ocProxy{ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			host, _, _ := net.SplitHostPort(c.RemoteAddr().String())
			if ip, _ := f.allow.Load().(string); ip == "" || host != ip {
				c.Close() // only this agent's container
				continue
			}
			go func() {
				defer c.Close()
				t, err := net.DialTimeout("tcp", target, 10*time.Second)
				if err != nil {
					return
				}
				defer t.Close()
				go func() { io.Copy(t, c); t.Close() }()
				io.Copy(c, t)
			}()
		}
	}()
	u.Host = net.JoinHostPort("127.0.0.1", strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))
	return u.String(), f, nil
}

// ocContainerIP is a running container's address on Docker's networks.
func ocContainerIP(name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	// The Docker CLI path is operator-configured and the container name is generated by WRM;
	// both are passed as separate arguments, never through a shell.
	out, _ := exec.CommandContext(ctx, mediumDockerCLI(), "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", name).Output()
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return ""
	}
	return f[0]
}
