// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── guacd on Windows and macOS ─────────────────────────────────────────────
//
// Linux installs guacd as a normal service. Windows and macOS usually do not have
// it, so WRM starts the official guacd container in Docker Desktop and keeps it
// running (restart policy "unless-stopped"). The container publishes guacd on
// 127.0.0.1:4822 only, so it is not reachable from the network. Both the browser
// and WRM keep speaking the Guacamole protocol; nothing else in the desktop path
// changes.
//
// Automatic setup runs only when guacd_address points at the local machine and
// guacd_auto is on. A custom guacd (another host) is never touched.

const (
	guacdManagedContainer = "wrm-guacd"
	guacdManagedImage     = "guacamole/guacd:1.5.5"
	guacdContainerPort    = "4822"
)

var (
	errGuacdNoDocker          = errors.New("Docker Desktop is not installed on this computer")
	errGuacdDockerDown        = errors.New("Docker Desktop is installed but is not running")
	errGuacdWindowsContainers = errors.New("Docker Desktop is using Windows containers")
	errGuacdNotLocal          = errors.New("guacd_address is not a local address")
	errGuacdAutoOff           = errors.New("automatic guacd setup is off")
)

// Replaced in tests.
var (
	guacdGOOS          = runtime.GOOS
	guacdDockerCLI     = mediumDockerCLI
	guacdReady         = func() error { _, err := guacdCheck(); return err }
	guacdDockerCommand = func(ctx context.Context, args ...string) ([]byte, error) {
		cli := guacdDockerCLI()
		if cli == "" {
			return nil, errGuacdNoDocker
		}
		// cli comes from mediumDockerCLI()'s fixed local paths and every argument is
		// built by WRM (container/image/port names), never by a remote user.
		// #nosec G204 -- reviewed Docker subprocess with fixed local executable.
		return exec.CommandContext(ctx, cli, args...).CombinedOutput()
	}
)

var (
	guacdSetupGate = make(chan struct{}, 1)
	guacdManaged   = struct {
		sync.Mutex
		starting bool
		lastErr  string
		lastTry  time.Time
		lastOK   time.Time
	}{}
)

// guacdLocalTarget returns the local port guacd should listen on.
func guacdLocalTarget() (port string, ok bool) {
	raw := strings.TrimSpace(getSetting("guacd_address"))
	if raw == "" {
		raw = "127.0.0.1:4822"
	}
	host, p, err := net.SplitHostPort(raw)
	if err != nil {
		return "", false
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 1 || n > 65535 {
		return "", false
	}
	host = strings.Trim(host, "[]")
	if host != "127.0.0.1" && host != "::1" && !strings.EqualFold(host, "localhost") {
		return "", false
	}
	return p, true
}

// guacdAutoEnabled says whether WRM may manage the local guacd container.
func guacdAutoEnabled() bool {
	if guacdGOOS != "windows" && guacdGOOS != "darwin" {
		return false
	}
	if !settingBool("desktop_enabled") || !settingBool("guacd_auto") {
		return false
	}
	_, ok := guacdLocalTarget()
	return ok
}

// guacdPrepare starts guacd if needed and waits up to wait for it. It is called
// from the WebSocket handler so a stopped container starts with the first click.
func guacdPrepare(wait time.Duration) (bool, error) {
	if err := guacdReady(); err == nil {
		return true, nil
	}
	if !guacdAutoEnabled() {
		return false, errGuacdAutoOff
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- guacdEnsure(ctx, false) }()
	select {
	case err := <-done:
		return err == nil, err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// guacdStartAsync starts the container in the background and returns immediately.
func guacdStartAsync() bool {
	if !guacdAutoEnabled() {
		return false
	}
	guacdManaged.Lock()
	if guacdManaged.starting {
		guacdManaged.Unlock()
		return false
	}
	guacdManaged.starting = true
	guacdManaged.lastTry = time.Now()
	guacdManaged.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		err := guacdEnsure(ctx, false)
		cancel()
		guacdManaged.Lock()
		guacdManaged.starting = false
		if err != nil {
			guacdManaged.lastErr = truncateStr(err.Error(), 300)
			log.Printf("guacd: automatic start failed: %v", err)
		} else {
			guacdManaged.lastErr = ""
			guacdManaged.lastOK = time.Now()
			log.Printf("guacd: ready at %s", getSetting("guacd_address"))
		}
		guacdManaged.Unlock()
	}()
	return true
}

// guacdEnsure makes the managed container run and waits until guacd answers.
// force is used by -setup-guacd and ignores guacd_auto, but still refuses a
// non-local guacd_address.
func guacdEnsure(ctx context.Context, force bool) error {
	if err := guacdReady(); err == nil {
		return nil
	}
	port, ok := guacdLocalTarget()
	if !ok {
		return fmt.Errorf("%w (%s); set guacd_address to 127.0.0.1:4822 or start guacd yourself", errGuacdNotLocal, getSetting("guacd_address"))
	}
	if !force && !guacdAutoEnabled() {
		return errGuacdAutoOff
	}
	select {
	case guacdSetupGate <- struct{}{}:
		defer func() { <-guacdSetupGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := guacdReady(); err == nil {
		return nil
	}
	if guacdDockerCLI() == "" {
		return errGuacdNoDocker
	}

	// A fast error here means the Docker engine (Docker Desktop) is not running.
	vctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	out, err := guacdDockerCommand(vctx, "version", "--format", "{{.Server.Os}}")
	cancel()
	if err != nil {
		return fmt.Errorf("%w (%s)", errGuacdDockerDown, guacdCommandError(out, err))
	}
	if engineOS := strings.ToLower(strings.TrimSpace(string(out))); engineOS != "" && engineOS != "linux" {
		return fmt.Errorf("%w (%s); switch Docker Desktop to Linux containers", errGuacdWindowsContainers, engineOS)
	}

	state := ""
	ictx, cancel := context.WithTimeout(ctx, 15*time.Second)
	out, err = guacdDockerCommand(ictx, "inspect", "--format", "{{.State.Running}}|{{.Config.Image}}", guacdManagedContainer)
	cancel()
	if err == nil {
		parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
		if len(parts) != 2 {
			return fmt.Errorf("Docker returned an unexpected answer for container %q: %s", guacdManagedContainer, guacdCommandError(out, nil))
		}
		state = strings.TrimSpace(parts[0])
		image := strings.TrimSpace(parts[1])
		if !strings.Contains(image, "guacamole/guacd") {
			return fmt.Errorf("the container %q already exists with image %q; remove it or set guacd_address to another guacd", guacdManagedContainer, image)
		}
	} else if !strings.Contains(strings.ToLower(string(out)), "no such") {
		return fmt.Errorf("Docker cannot inspect %s: %s", guacdManagedContainer, guacdCommandError(out, err))
	}

	switch state {
	case "true":
		// Running; only wait for the port.
	case "false":
		sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		out, err = guacdDockerCommand(sctx, "start", guacdManagedContainer)
		cancel()
		if err != nil {
			return fmt.Errorf("cannot start %s: %s", guacdManagedContainer, guacdCommandError(out, err))
		}
	default:
		rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		out, err = guacdDockerCommand(rctx, "run", "-d", "--name", guacdManagedContainer,
			"--restart", "unless-stopped", "--label", "wrm.managed=guacd",
			"-p", "127.0.0.1:"+port+":"+guacdContainerPort, guacdManagedImage)
		cancel()
		if err != nil {
			return fmt.Errorf("cannot start the guacd container: %s", guacdCommandError(out, err))
		}
	}

	wctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		if err := guacdReady(); err == nil {
			return nil
		}
		select {
		case <-wctx.Done():
			return fmt.Errorf("guacd was started but does not answer at %s: %v", getSetting("guacd_address"), wctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func guacdCommandError(out []byte, err error) string {
	s := strings.TrimSpace(string(out))
	if s == "" && err != nil {
		s = err.Error()
	}
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		s = "unknown error"
	}
	return truncateStr(s, 240)
}

// guacdFailureMessage is shown in the desktop window when guacd is not ready.
func guacdFailureMessage(err error) string {
	addr := getSetting("guacd_address")
	switch {
	case errors.Is(err, errGuacdNoDocker):
		return "Remote desktop needs Docker Desktop on this computer to run guacd. Install Docker Desktop (Medium → step 2), or set guacd_address to an existing guacd."
	case errors.Is(err, errGuacdDockerDown):
		return "Docker Desktop is installed but is not running. Start Docker Desktop, then try the remote desktop again."
	case errors.Is(err, errGuacdWindowsContainers):
		return "Docker Desktop is using Windows containers. Switch it to Linux containers (right-click the Docker icon → Switch to Linux containers), then try the remote desktop again."
	case errors.Is(err, errGuacdNotLocal):
		return "Remote desktop needs guacd at " + addr + ", but it is not reachable. Start it there, or set guacd_address to the WRM machine (127.0.0.1:4822) so WRM can start it."
	case guacdAutoEnabled():
		detail := ""
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			detail = " (" + truncateStr(err.Error(), 120) + ")"
		}
		return "WRM is starting guacd automatically at " + addr + ". The first start can take a minute; try again shortly." + detail
	default:
		return "Remote desktop needs guacd at " + addr + ", but it is not reachable. Install it on the WRM machine (apt install guacd, or the guacd service in docker-compose.yml) or set guacd_address."
	}
}

// guacdManagedStatus is added to the admin overview.
func guacdManagedStatus() map[string]interface{} {
	guacdManaged.Lock()
	defer guacdManaged.Unlock()
	st := map[string]interface{}{
		"auto":      guacdAutoEnabled(),
		"managed":   guacdDockerCLI() != "",
		"container": guacdManagedContainer,
		"starting":  guacdManaged.starting,
	}
	if guacdManaged.lastErr != "" {
		st["last_error"] = guacdManaged.lastErr
	}
	if !guacdManaged.lastTry.IsZero() {
		st["last_try"] = guacdManaged.lastTry.UTC().Format(time.RFC3339)
	}
	if !guacdManaged.lastOK.IsZero() {
		st["last_ok"] = guacdManaged.lastOK.UTC().Format(time.RFC3339)
	}
	return st
}

// guacdAutoStartLoop tries to start guacd when WRM starts. Docker Desktop may
// start after the service, so the desktop window and the admin button retry.
func guacdAutoStartLoop() {
	if !guacdAutoEnabled() {
		return
	}
	time.Sleep(5 * time.Second)
	for attempt := 0; attempt < 3; attempt++ {
		if err := guacdReady(); err == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		err := guacdEnsure(ctx, false)
		cancel()
		if err == nil {
			log.Printf("guacd: started automatically at %s", getSetting("guacd_address"))
			return
		}
		if errors.Is(err, errGuacdNoDocker) || errors.Is(err, errGuacdDockerDown) || errors.Is(err, errGuacdNotLocal) {
			log.Printf("guacd: automatic start unavailable: %v", err)
			return
		}
		log.Printf("guacd: automatic start failed (attempt %d/3): %v", attempt+1, err)
		if attempt < 2 {
			time.Sleep(2 * time.Minute)
		}
	}
}

// runSetupGuacd implements -setup-guacd.
func runSetupGuacd(stdout io.Writer) int {
	if err := guacdReady(); err == nil {
		fmt.Fprintf(stdout, "guacd already answers at %s.\n", getSetting("guacd_address"))
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := guacdEnsure(ctx, true); err != nil {
		fmt.Fprintf(stdout, "guacd setup failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "guacd is ready at %s (Docker container %s, restarts with Docker).\n", getSetting("guacd_address"), guacdManagedContainer)
	return 0
}
