//go:build windows

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

// ocSysProcAttr starts OpenCode without a console window.
func ocSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// ocKillTree kills OpenCode and the processes it started (taskkill /T), then the process itself.
func ocKillTree(cmd *exec.Cmd) {
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	kill.Run()
	cmd.Process.Kill()
}
