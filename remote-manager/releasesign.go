// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ─── SIGNED RELEASES ─────────────────────────────────
//
// Every release carries SHA256SUMS.txt (the SHA-256 of each file) and SHA256SUMS.txt.sig, an
// ed25519 signature of SHA256SUMS.txt made in CI with the project's release key. The public
// half is built into WRM (builtinReleaseKeys). The self-update then refuses a release whose
// signature is missing or does not verify, so a changed release file or a hijacked release
// page cannot install code; administrators of a fork add their own public key in
// update_signing_keys. Until a key is known, the update verifies the SHA-256 only and says
// that the signature was not checked.
//
//   wrm -release-keygen FILE      new key pair: the private key goes to FILE (0600), the
//                                 public key is printed (built into WRM / update_signing_keys)
//   wrm -release-sign FILE        writes FILE.sig (key from WRM_RELEASE_SIGNING_KEY or
//                                 -release-key-file)
//   wrm -release-verify FILE      checks FILE.sig, and the SHA-256 of every listed file that
//                                 is present next to FILE

const (
	releaseSigMagic       = "WRM release signature v1"
	releasePublicPrefix   = "wrm-ed25519:"
	releasePrivatePrefix  = "wrm-ed25519-private:"
	releaseSignatureExt   = ".sig"
	releaseSigningKeyEnv  = "WRM_RELEASE_SIGNING_KEY"
	maxReleaseSigFileSize = 4 << 10
)

// builtinReleaseKeys are the public keys that sign this project's releases (newest first; an
// old key stays listed while releases signed with it are still offered).
var builtinReleaseKeys = []string{
	"wrm-ed25519:dDQxQPC0rcL16tuoDJ35/XC9b0p73Vdy6XB4mcL3+/w=", // project release key, 2026-10-17
}

// releaseSignedMessage is what the key signs: a fixed context and the exact file bytes, so a
// signature made for something else cannot be passed off as a release signature.
func releaseSignedMessage(name string, data []byte) []byte {
	var b bytes.Buffer
	b.WriteString(releaseSigMagic + "\x00" + filepath.Base(name) + "\x00")
	b.Write(data)
	return b.Bytes()
}

func releaseKeyID(pub ed25519.PublicKey) string {
	h := sha256.Sum256(pub)
	return hex.EncodeToString(h[:8])
}

func parseReleasePublicKey(s string) (ed25519.PublicKey, error) {
	s = strings.TrimSpace(s)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, releasePublicPrefix))
	if !strings.HasPrefix(s, releasePublicPrefix) || err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("not a WRM release public key (expected %s<base64>)", releasePublicPrefix)
	}
	return ed25519.PublicKey(raw), nil
}

func parseReleasePrivateKey(s string) (ed25519.PrivateKey, error) {
	s = strings.TrimSpace(s)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, releasePrivatePrefix))
	if !strings.HasPrefix(s, releasePrivatePrefix) || err != nil || len(raw) != ed25519.SeedSize {
		return nil, fmt.Errorf("not a WRM release private key (expected %s<base64>)", releasePrivatePrefix)
	}
	return ed25519.NewKeyFromSeed(raw), nil
}

func formatReleasePublicKey(pub ed25519.PublicKey) string {
	return releasePublicPrefix + base64.StdEncoding.EncodeToString(pub)
}

// parseReleaseKeyList reads public keys separated by commas, spaces or new lines.
func parseReleaseKeyList(v string) ([]ed25519.PublicKey, error) {
	var out []ed25519.PublicKey
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\r' || r == '\t' }) {
		k, err := parseReleasePublicKey(f)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

// releaseTrustedKeys are the keys a release of the configured repository must be signed with:
// the built-in keys for this project's repository, plus the administrator's update_signing_keys.
func releaseTrustedKeys() []ed25519.PublicKey {
	var out []ed25519.PublicKey
	if updateRepo() == defaultUpdateRepo {
		for _, s := range builtinReleaseKeys {
			if k, err := parseReleasePublicKey(s); err == nil {
				out = append(out, k)
			}
		}
	}
	extra, _ := parseReleaseKeyList(getSetting("update_signing_keys"))
	return append(out, extra...)
}

func signRelease(priv ed25519.PrivateKey, name string, data []byte) []byte {
	pub := priv.Public().(ed25519.PublicKey)
	sig := ed25519.Sign(priv, releaseSignedMessage(name, data))
	return []byte(fmt.Sprintf("%s\nfile: %s\nkey: %s\nsignature: %s\n", releaseSigMagic, filepath.Base(name), releaseKeyID(pub), base64.StdEncoding.EncodeToString(sig)))
}

// verifyReleaseSignature checks sigFile for data (the file called name) against keys and
// returns the ID of the key that signed it.
func verifyReleaseSignature(name string, data, sigFile []byte, keys []ed25519.PublicKey) (string, error) {
	if len(sigFile) > maxReleaseSigFileSize {
		return "", errors.New("the signature file is too large")
	}
	fields := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(sigFile))
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first {
			if line != releaseSigMagic {
				return "", errors.New("not a WRM release signature")
			}
			first = false
			continue
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	if first {
		return "", errors.New("the signature file is empty")
	}
	if fields["file"] != filepath.Base(name) {
		return "", fmt.Errorf("the signature is for %q, not %s", fields["file"], filepath.Base(name))
	}
	sig, err := base64.StdEncoding.DecodeString(fields["signature"])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return "", errors.New("the signature is malformed")
	}
	if len(keys) == 0 {
		return "", errors.New("no release key is known")
	}
	msg := releaseSignedMessage(name, data)
	for _, k := range keys {
		if ed25519.Verify(k, msg, sig) {
			return releaseKeyID(k), nil
		}
	}
	return "", fmt.Errorf("the signature (key %s) does not verify with any trusted release key", fields["key"])
}

// ─── command line ────────────────────────────────────

func runReleaseKeygen(path string, out io.Writer) int {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintln(out, "Error:", err)
		return 1
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		fmt.Fprintf(out, "Error: %v (an existing key is never overwritten)\n", err)
		return 1
	}
	_, err = fmt.Fprintf(f, "%s%s\n", releasePrivatePrefix, base64.StdEncoding.EncodeToString(priv.Seed()))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		fmt.Fprintln(out, "Error:", err)
		return 1
	}
	fmt.Fprintf(out, "Private key written to %s. Keep it secret: put its content in the repository secret %s and then store the file offline (or delete it).\n\n", path, "RELEASE_SIGNING_KEY")
	fmt.Fprintf(out, "Public key (key %s), to build into WRM or to add to update_signing_keys:\n%s\n", releaseKeyID(pub), formatReleasePublicKey(pub))
	return 0
}

func runReleaseSign(path, keyFile string, out io.Writer) int {
	keyText := os.Getenv(releaseSigningKeyEnv)
	if keyFile != "" {
		b, err := os.ReadFile(keyFile)
		if err != nil {
			fmt.Fprintln(out, "Error:", err)
			return 1
		}
		keyText = string(b)
	}
	if strings.TrimSpace(keyText) == "" {
		fmt.Fprintf(out, "Error: no signing key (set %s or use -release-key-file)\n", releaseSigningKeyEnv)
		return 1
	}
	priv, err := parseReleasePrivateKey(keyText)
	if err != nil {
		fmt.Fprintln(out, "Error:", err)
		return 1
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(out, "Error:", err)
		return 1
	}
	if err := os.WriteFile(path+releaseSignatureExt, signRelease(priv, path, data), 0o644); err != nil { // a signature is public
		fmt.Fprintln(out, "Error:", err)
		return 1
	}
	fmt.Fprintf(out, "Signed %s with key %s: %s\n", filepath.Base(path), releaseKeyID(priv.Public().(ed25519.PublicKey)), filepath.Base(path)+releaseSignatureExt)
	return 0
}

func runReleaseVerify(path, publicKeys string, out io.Writer) int {
	var keys []ed25519.PublicKey
	for _, s := range builtinReleaseKeys {
		if k, err := parseReleasePublicKey(s); err == nil {
			keys = append(keys, k)
		}
	}
	extra, err := parseReleaseKeyList(publicKeys)
	if err != nil {
		fmt.Fprintln(out, "Error: -release-public-key:", err)
		return 1
	}
	keys = append(keys, extra...)
	if len(keys) == 0 {
		fmt.Fprintln(out, "Error: this build has no release key; pass one with -release-public-key")
		return 1
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(out, "Error:", err)
		return 1
	}
	sig, err := os.ReadFile(path + releaseSignatureExt)
	if err != nil {
		fmt.Fprintf(out, "Error: %v (the signature is the release file %s)\n", err, filepath.Base(path)+releaseSignatureExt)
		return 1
	}
	id, err := verifyReleaseSignature(path, data, sig, keys)
	if err != nil {
		fmt.Fprintln(out, "SIGNATURE NOT VALID:", err)
		return 1
	}
	fmt.Fprintf(out, "Signature OK: %s is signed with release key %s.\n", filepath.Base(path), id)
	bad, checked := 0, 0
	dir := filepath.Dir(path)
	for name, want := range parseChecksums(bytes.NewReader(data)) {
		if name != filepath.Base(name) {
			continue
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		checked++
		if got := hex.EncodeToString(h.Sum(nil)); err != nil || got != want {
			fmt.Fprintf(out, "MISMATCH: %s\n", name)
			bad++
		} else {
			fmt.Fprintf(out, "OK: %s\n", name)
		}
	}
	if bad > 0 {
		return 1
	}
	if checked == 0 {
		fmt.Fprintln(out, "No listed file is next to it; check a download with its SHA-256 in the file.")
	}
	return 0
}
