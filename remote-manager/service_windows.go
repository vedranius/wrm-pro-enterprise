//go:build windows

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// A real Windows service: registered with the Service Control Manager, automatic (delayed)
// start, restart on failure (also when WRM exits with exitRestart), log in the data folder.

func servicePlatformSupported() bool { return true }

func serviceManagerName(scope string) string { return "a Windows service" }

func isElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

// queryService opens a service with query rights only (works without administrator rights).
func queryService(name string) (svc.State, bool) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, false
	}
	defer windows.CloseServiceHandle(scm)
	np, _ := windows.UTF16PtrFromString(name)
	h, err := windows.OpenService(scm, np, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return 0, false
	}
	defer windows.CloseServiceHandle(h)
	var st windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(h, &st); err != nil {
		return 0, true
	}
	return svc.State(st.CurrentState), true
}

func serviceInstalled(name string) bool {
	_, ok := queryService(name)
	return ok
}

func adminHint(flag string) error {
	return errNeedsAdmin{hint: "this needs administrator rights: open PowerShell with \"Run as administrator\" and run  .\\" +
		filepath.Base(os.Args[0]) + " " + flag}
}

func installService(o serviceOptions) error {
	if !isElevated() {
		return adminHint("-install-service")
	}
	s, err := buildServiceSpec(o)
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("service manager: %v", err)
	}
	defer m.Disconnect()
	if old, err := m.OpenService(o.Name); err == nil {
		old.Close()
		return fmt.Errorf("the service %q is already installed (remove it first with -uninstall-service)", o.Name)
	}
	ws, err := m.CreateService(o.Name, s.Exe, mgr.Config{
		DisplayName: serviceDisplayName, Description: serviceDescription,
		StartType: mgr.StartAutomatic, DelayedAutoStart: true,
	}, serviceArgs(s, svcWindows)...)
	if err != nil {
		return fmt.Errorf("create service: %v", err)
	}
	defer ws.Close()
	actions := make([]mgr.RecoveryAction, windowsRecovery.Actions)
	for i := range actions {
		actions[i] = mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: windowsRecovery.Delay}
	}
	if err := ws.SetRecoveryActions(actions, uint32(windowsRecovery.ResetPeriod/time.Second)); err != nil {
		return fmt.Errorf("recovery actions: %v", err)
	}
	if err := ws.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return fmt.Errorf("recovery actions: %v", err)
	}
	if err := ws.Start(); err != nil {
		return fmt.Errorf("start: %v", err)
	}
	fmt.Printf("Installed and started the Windows service %q.\n%s  Settings: %s\nStatus: %s -service-status   Remove: %s -uninstall-service\n",
		o.Name, indent(windowsServiceConfig(s)), s.EnvFile, filepath.Base(s.Exe), filepath.Base(s.Exe))
	return nil
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n") + "\n"
}

// installElevated asks for administrator rights (UAC) and runs the installer in a new window,
// then waits for the service to appear.
func installElevated(o serviceOptions) error {
	s, err := buildServiceSpec(o)
	if err != nil {
		return err
	}
	params := windowsCommandLine("x", []string{"-install-service", "-no-service-prompt", "-service-name", o.Name, "-env-file", s.EnvFile, "-workdir", s.WorkDir})
	params = strings.TrimPrefix(params, "x ")
	verb, _ := windows.UTF16PtrFromString("runas")
	exe, _ := windows.UTF16PtrFromString(s.Exe)
	args, _ := windows.UTF16PtrFromString(params)
	dir, _ := windows.UTF16PtrFromString(s.WorkDir)
	fmt.Println("Windows asks for administrator rights to install the service…")
	if err := windows.ShellExecute(0, verb, exe, args, dir, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("administrator rights were not granted (%v); run  .\\%s -install-service  in PowerShell opened with \"Run as administrator\"", err, filepath.Base(s.Exe))
	}
	for i := 0; i < 120; i++ {
		if st, ok := queryService(o.Name); ok && st == svc.Running {
			return nil
		}
		time.Sleep(time.Second)
	}
	return errors.New("the service did not start within two minutes; check  -service-status")
}

func uninstallService(o serviceOptions) error {
	if !serviceInstalled(o.Name) {
		return fmt.Errorf("the service %q is not installed", o.Name)
	}
	if !isElevated() {
		return adminHint("-uninstall-service")
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("service manager: %v", err)
	}
	defer m.Disconnect()
	ws, err := m.OpenService(o.Name)
	if err != nil {
		return err
	}
	defer ws.Close()
	if st, err := ws.Query(); err == nil && st.State != svc.Stopped {
		ws.Control(svc.Stop)
		for i := 0; i < 30; i++ {
			if st, err := ws.Query(); err != nil || st.State == svc.Stopped {
				break
			}
			time.Sleep(time.Second)
		}
	}
	if err := ws.Delete(); err != nil {
		return fmt.Errorf("delete: %v", err)
	}
	fmt.Printf("Removed the Windows service %q. The data, the database and the environment file stay where they are.\n", o.Name)
	return nil
}

func serviceStatusText(o serviceOptions) string {
	st, ok := queryService(o.Name)
	if !ok {
		return fmt.Sprintf("Service %q is not installed. Install it with -install-service (as administrator).\n", o.Name)
	}
	names := map[svc.State]string{svc.Stopped: "stopped", svc.StartPending: "starting", svc.StopPending: "stopping",
		svc.Running: "running", svc.ContinuePending: "continuing", svc.PausePending: "pausing", svc.Paused: "paused"}
	return fmt.Sprintf("Service %q is installed (Windows service): %s\n", o.Name, names[st])
}

// execBinary starts the binary as a new process with this console; the caller exits.
func execBinary(path string, args []string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Start()
}

// switchServiceBinary points the service at a newer binary; the caller exits with
// exitRestart and the recovery actions start the new one.
func switchServiceBinary(path string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	ws, err := m.OpenService(currentServiceName)
	if err != nil {
		return err
	}
	defer ws.Close()
	cfg, err := ws.Config()
	if err != nil {
		return err
	}
	exe, _ := selfExecutable()
	if !strings.Contains(strings.ToLower(cfg.BinaryPathName), strings.ToLower(filepath.Base(exe))) {
		return fmt.Errorf("unexpected service command line %q", cfg.BinaryPathName)
	}
	cfg.BinaryPathName = windowsCommandLine(path, os.Args[1:])
	if err := ws.UpdateConfig(cfg); err != nil {
		return err
	}
	return errSwitchByRestart
}

type winService struct {
	serve func(stop <-chan struct{}) int
	code  int
}

func (w *winService) Execute(args []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	stop := make(chan struct{})
	done := make(chan int, 1)
	go func() { done <- w.serve(stop) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	stopping := false
	for {
		select {
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				if !stopping {
					stopping = true
					status <- svc.Status{State: svc.StopPending}
					close(stop)
				}
			}
		case code := <-done:
			w.code = code
			if code != 0 {
				// A non-zero exit code makes the recovery actions start WRM again.
				return true, uint32(code)
			}
			return false, 0
		}
	}
}

// runAsWindowsService runs serve under the SCM when this process was started as a service.
func runAsWindowsService(name string, serve func(stop <-chan struct{}) int) (bool, int) {
	isSvc, err := svc.IsWindowsService()
	if err != nil || !isSvc {
		return false, 0
	}
	currentServiceName = name
	serviceMode = svcWindows
	w := &winService{serve: serve}
	if err := svc.Run(name, w); err != nil {
		return true, 1
	}
	return true, w.code
}
