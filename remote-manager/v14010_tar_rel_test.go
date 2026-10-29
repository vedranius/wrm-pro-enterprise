// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"bytes"
	"testing"
)

// TestTarOfRefusesEscapingNames covers the rest of GIT-02: the deploy tar is extracted on
// the target, so a member name with ".." or an absolute path must never be packed.
func TestTarOfRefusesEscapingNames(t *testing.T) {
	ok, err := tarOf(map[string][]byte{"app/main.go": []byte("package main\n")}, map[string]int64{"app/main.go": 0o644})
	if err != nil {
		t.Fatalf("plain relative name refused: %v", err)
	}
	tr := tar.NewReader(bytes.NewReader(ok))
	if h, err := tr.Next(); err != nil || h.Name != "app/main.go" {
		t.Fatalf("packed member is %v (%v)", h, err)
	}

	for _, name := range []string{"../escape.txt", "/etc/passwd", "a/../../b", "a/./b", "", "a\\b"} {
		if _, err := tarOf(map[string][]byte{name: []byte("x")}, map[string]int64{name: 0o644}); err == nil {
			t.Errorf("%q was packed into the deploy tar", name)
		}
	}
}
