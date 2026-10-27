//go:build windows

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os/exec"
)

type mdBuildAccount struct{ Name string }

// mdResolveBuildUser: Windows hosts do not run builds themselves (see mdHostBuildSupported).
func mdResolveBuildUser() (*mdBuildAccount, error) {
	if name := getSetting("medium_build_user"); name != "" {
		return nil, fmt.Errorf("the build user %s cannot be used on Windows; choose a Linux build server", name)
	}
	return nil, nil
}

func mdApplyBuildUser(cmd *exec.Cmd, acc *mdBuildAccount, root string) error { return nil }

func mdReclaimBuildDir(acc *mdBuildAccount, root string) error { return nil }
