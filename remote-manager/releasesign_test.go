// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testReleaseKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestReleaseSignatureRoundTrip(t *testing.T) {
	pub, priv := testReleaseKey(t)
	other, _ := testReleaseKey(t)
	data := []byte(strings.Repeat("a", 64) + "  wrm-pro-v1.0.0-linux-amd64\n")
	sig := signRelease(priv, "dist/SHA256SUMS.txt", data)

	id, err := verifyReleaseSignature("SHA256SUMS.txt", data, sig, []ed25519.PublicKey{other, pub})
	if err != nil || id != releaseKeyID(pub) {
		t.Fatalf("verify: %q %v", id, err)
	}
	bad := []struct {
		name string
		file string
		data []byte
		sig  []byte
		keys []ed25519.PublicKey
		want string
	}{
		{"changed file", "SHA256SUMS.txt", append([]byte("0"), data[1:]...), sig, []ed25519.PublicKey{pub}, "does not verify"},
		{"other key", "SHA256SUMS.txt", data, sig, []ed25519.PublicKey{other}, "does not verify"},
		{"no key", "SHA256SUMS.txt", data, sig, nil, "no release key"},
		{"other file name", "notes.txt", data, sig, []ed25519.PublicKey{pub}, "is for"},
		{"not a signature", "SHA256SUMS.txt", data, []byte("hello\n"), []ed25519.PublicKey{pub}, "not a WRM release signature"},
		{"empty", "SHA256SUMS.txt", data, nil, []ed25519.PublicKey{pub}, "empty"},
		{"broken signature", "SHA256SUMS.txt", data, bytes.Replace(sig, []byte("signature: "), []byte("signature: !!"), 1), []ed25519.PublicKey{pub}, "malformed"},
		{"too large", "SHA256SUMS.txt", data, append(sig, make([]byte, maxReleaseSigFileSize)...), []ed25519.PublicKey{pub}, "too large"},
	}
	for _, c := range bad {
		if _, err := verifyReleaseSignature(c.file, c.data, c.sig, c.keys); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// A signature over the bare file (without WRM's context) is not accepted.
	forged := []byte(releaseSigMagic + "\nfile: SHA256SUMS.txt\nsignature: " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n")
	if _, err := verifyReleaseSignature("SHA256SUMS.txt", data, forged, []ed25519.PublicKey{pub}); err == nil {
		t.Error("a signature without the WRM context verified")
	}
}

func TestReleaseKeyParsing(t *testing.T) {
	for _, s := range builtinReleaseKeys { // a typo here would turn signature checks off
		if _, err := parseReleasePublicKey(s); err != nil {
			t.Fatalf("built-in key %q: %v", s, err)
		}
	}
	pub, priv := testReleaseKey(t)
	s := formatReleasePublicKey(pub)
	if k, err := parseReleasePublicKey(" " + s + "\n"); err != nil || !k.Equal(pub) {
		t.Fatalf("public: %v", err)
	}
	for _, bad := range []string{"", "wrm-ed25519:", "wrm-ed25519:AAAA", strings.TrimPrefix(s, releasePublicPrefix), "ssh-ed25519 AAAA"} {
		if _, err := parseReleasePublicKey(bad); err == nil {
			t.Errorf("public key %q accepted", bad)
		}
	}
	if _, err := parseReleasePrivateKey(s); err == nil {
		t.Error("a public key accepted as private")
	}
	keys, err := parseReleaseKeyList(s + ", " + s + "\n")
	if err != nil || len(keys) != 2 {
		t.Fatalf("list: %v %d", err, len(keys))
	}
	if k, err := parseReleasePrivateKey(releasePrivatePrefix + base64.StdEncoding.EncodeToString(priv.Seed())); err != nil || !k.Equal(priv) {
		t.Fatalf("private: %v", err)
	}

	newTestServer(t)
	if _, err := setSetting("update_signing_keys", "wrm-ed25519:nope"); err == nil {
		t.Fatal("invalid update_signing_keys accepted")
	}
	if _, err := setSetting("update_signing_keys", s+" "+s); err != nil {
		t.Fatal(err)
	}
	if got := getSetting("update_signing_keys"); got != s+","+s {
		t.Fatalf("normalized: %q", got)
	}
	setSetting("update_signing_keys", "")
}

func TestReleaseCommands(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "release.key")
	var out bytes.Buffer
	if code := runReleaseKeygen(keyFile, &out); code != 0 {
		t.Fatalf("keygen: %s", out.String())
	}
	if fi, err := os.Stat(keyFile); err != nil || (fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/') {
		t.Fatalf("key file: %v %v", fi, err)
	}
	pubLine := ""
	for _, l := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(l, releasePublicPrefix) {
			pubLine = l
		}
	}
	if pubLine == "" {
		t.Fatalf("no public key printed: %s", out.String())
	}
	out.Reset()
	if code := runReleaseKeygen(keyFile, &out); code == 0 {
		t.Fatal("an existing key file was overwritten")
	}

	bin := []byte("binary")
	h := sha256.Sum256(bin)
	os.WriteFile(filepath.Join(dir, "wrm-pro-v1-linux-amd64"), bin, 0o644)
	sums := filepath.Join(dir, "SHA256SUMS.txt")
	os.WriteFile(sums, []byte(hex.EncodeToString(h[:])+"  wrm-pro-v1-linux-amd64\n"+strings.Repeat("b", 64)+"  ../outside\n"), 0o644)

	t.Setenv(releaseSigningKeyEnv, "")
	out.Reset()
	if code := runReleaseSign(sums, "", &out); code == 0 {
		t.Fatal("signed without a key")
	}
	out.Reset()
	if code := runReleaseSign(sums, keyFile, &out); code != 0 {
		t.Fatalf("sign: %s", out.String())
	}
	key, _ := os.ReadFile(keyFile)
	t.Setenv(releaseSigningKeyEnv, string(key))
	out.Reset()
	if code := runReleaseSign(sums, "", &out); code != 0 {
		t.Fatalf("sign from the environment: %s", out.String())
	}

	out.Reset()
	if code := runReleaseVerify(sums, "", &out); code == 0 && len(builtinReleaseKeys) == 0 {
		t.Fatal("verified without any key")
	}
	out.Reset()
	if code := runReleaseVerify(sums, pubLine, &out); code != 0 || !strings.Contains(out.String(), "Signature OK") || !strings.Contains(out.String(), "OK: wrm-pro-v1-linux-amd64") {
		t.Fatalf("verify: %d %s", code, out.String())
	}
	os.WriteFile(filepath.Join(dir, "wrm-pro-v1-linux-amd64"), []byte("changed"), 0o644)
	out.Reset()
	if code := runReleaseVerify(sums, pubLine, &out); code == 0 || !strings.Contains(out.String(), "MISMATCH") {
		t.Fatalf("changed file: %d %s", code, out.String())
	}
	os.WriteFile(sums, []byte("tampered\n"), 0o644)
	out.Reset()
	if code := runReleaseVerify(sums, pubLine, &out); code == 0 || !strings.Contains(out.String(), "NOT VALID") {
		t.Fatalf("tampered sums: %d %s", code, out.String())
	}
}

// With a trusted release key the self-update requires a valid signature of SHA256SUMS.txt.
func TestSelfUpdateSignature(t *testing.T) {
	e := newUpdateEnv(t)
	pub, priv := testReleaseKey(t)
	_, otherPriv := testReleaseKey(t)
	newBin := fakeScript("v11.8.0")
	sums := binSHA256(newBin) + "  " + e.asset + "\n"
	e.admin.jsonDo("POST", "/api/update/check", nil, nil)

	// No key known: the update relies on the SHA-256 and says so.
	if st := e.status(e.admin); st["signature_keys"] != float64(0) {
		t.Fatalf("signature_keys without a key: %v", st["signature_keys"])
	}
	setSetting("update_signing_keys", formatReleasePublicKey(pub))
	t.Cleanup(func() { setSetting("update_signing_keys", "") })
	if st := e.status(e.admin); st["signature_keys"] != float64(1) {
		t.Fatalf("signature_keys: %v", st["signature_keys"])
	}

	sigName := checksumsAsset + releaseSignatureExt
	cases := []struct {
		name string
		sig  []byte
		want string
	}{
		{"unsigned", nil, "is not signed"},
		{"signed with another key", signRelease(otherPriv, checksumsAsset, []byte(sums)), "not valid"},
		{"signature of other content", signRelease(priv, checksumsAsset, []byte(sums+"\n")), "not valid"},
	}
	for _, c := range cases {
		e.release(newBin, sums, true)
		if c.sig != nil {
			e.gh.set(func() { e.gh.files[sigName] = c.sig })
		}
		p := e.apply()
		if p.State != "failed" || !strings.Contains(p.Error, c.want) {
			t.Fatalf("%s: %+v", c.name, p)
		}
		e.exeIs("v11.7.0")
		e.noLeftovers()
	}
	if n := updateAuditCount("system.update_failed", "v11.8.0"); n < len(cases) {
		t.Fatalf("refusals audited: %d", n)
	}

	e.release(newBin, sums, true)
	e.gh.set(func() { e.gh.files[sigName] = signRelease(priv, checksumsAsset, []byte(sums)) })
	if p := e.apply(); p.State != "restarting" {
		t.Fatalf("signed release: %+v", p)
	}
	select {
	case <-e.restarts:
	case <-time.After(5 * time.Second):
		t.Fatal("no restart")
	}
	e.exeIs("v11.8.0")
	var details string
	db.QueryRow(`SELECT details FROM audit_log WHERE action='system.update_verified' ORDER BY id DESC LIMIT 1`).Scan(&details)
	if !strings.Contains(details, "key "+releaseKeyID(pub)) {
		t.Fatalf("verified audit: %s", details)
	}
}
