//go:build !(linux && (amd64 || arm64 || arm || 386))

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func startInPTY(cmd *exec.Cmd, cols, rows int) (*os.File, error) {
	return nil, fmt.Errorf("local consoles on the WRM server need Linux; reach the BMC through a jump host instead")
}

func setPTYSize(f *os.File, cols, rows int) {}

func openPTYPair() (*os.File, string, error) {
	return nil, "", fmt.Errorf("pseudo terminals need Linux")
}

func openSerialPort(path string, so serialOptions) (*os.File, error) {
	return nil, fmt.Errorf("serial ports are supported when WRM runs on Linux")
}
