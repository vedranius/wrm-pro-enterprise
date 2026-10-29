// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCSPHashesInlineScripts covers the CSP half of SEC-08: the inline script of the UI is
// allowed by a hash, so script-src no longer needs 'unsafe-inline'; the handler attributes
// keep working through script-src-attr.
func TestCSPHashesInlineScripts(t *testing.T) {
	hashes := inlineScriptHashes()
	if len(hashes) == 0 {
		t.Fatal("no inline script hash was computed")
	}

	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	var inline []byte
	for _, m := range inlineScriptRE.FindAllSubmatch(data, -1) {
		if len(m) < 3 || len(m[2]) == 0 || strings.Contains(strings.ToLower(string(m[1])), "src=") {
			continue
		}
		inline = m[2]
		break
	}
	if inline == nil {
		t.Fatal("the UI page has no inline script")
	}
	sum := sha256.Sum256(inline)
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	found := false
	for _, h := range hashes {
		if h == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("%s is not among the hashes %v", want, hashes)
	}

	csp := contentSecurityPolicy(httptest.NewRequest("GET", "/", nil))
	var scriptSrc string
	for _, d := range strings.Split(csp, "; ") {
		if strings.HasPrefix(d, "script-src ") {
			scriptSrc = d
		}
	}
	if scriptSrc == "" {
		t.Fatalf("no script-src in %q", csp)
	}
	if strings.Contains(scriptSrc, "'unsafe-inline'") {
		t.Fatalf("script-src still allows inline scripts: %q", scriptSrc)
	}
	if !strings.Contains(scriptSrc, "'sha256-") {
		t.Fatalf("script-src has no hash: %q", scriptSrc)
	}
	if !strings.Contains(csp, "script-src-attr 'unsafe-inline'") {
		t.Fatalf("handler attributes lost their allowance: %q", csp)
	}
}
