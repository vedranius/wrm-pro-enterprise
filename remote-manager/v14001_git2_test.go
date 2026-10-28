// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// TestCapBufferReportsTruncation covers GIT-02: a truncated server answer must be visible,
// not silently parsed as a complete one.
func TestCapBufferReportsTruncation(t *testing.T) {
	b := &capBuffer{max: 4}
	if _, err := b.Write([]byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	if b.String() != "abcd" || !b.truncated {
		t.Fatalf("buffer=%q truncated=%v", b.String(), b.truncated)
	}
	b2 := &capBuffer{max: 10}
	b2.Write([]byte("abc"))
	if b2.truncated {
		t.Fatal("small answer reported as truncated")
	}
}

// TestDeployLogHasNoTmpFallback covers the predictable /tmp/deploytool-updates.jsonl path.
func TestDeployLogHasNoTmpFallback(t *testing.T) {
	for _, p := range gitUpdateLogPaths {
		if strings.HasPrefix(p, "/tmp/") {
			t.Fatalf("deploy log still has the /tmp fallback: %s", p)
		}
	}
}
