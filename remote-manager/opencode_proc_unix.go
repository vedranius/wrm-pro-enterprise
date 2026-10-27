//go:build !windows

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os/exec"
	"syscall"
)

// ocSysProcAttr puts OpenCode in its own process group, so a terminal signal to WRM does not
// reach it and WRM decides when it stops.
func ocSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// ocKillTree kills OpenCode and everything it started (its process group).
func ocKillTree(cmd *exec.Cmd) {
	if cmd.Process.Pid > 0 {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.Process.Kill()
}
