//go:build !windows

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import "crypto/x509"

func mediumHostRoots() ([]*x509.Certificate, error) { return mediumHostRootsUnix() }
