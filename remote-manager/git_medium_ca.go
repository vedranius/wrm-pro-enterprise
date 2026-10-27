// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Medium's Docker runtime trusts the certificates the WRM host trusts. Behind a proxy or
// antivirus that inspects HTTPS, pip inside the container otherwise fails with
// "self-signed certificate in certificate chain": the inspecting root is in the host's
// store, not in the image. The bundle holds the host's trusted roots (policy
// medium_host_ca) and an extra PEM file (setting medium_ca_file); containers add it to the
// image's own roots at start (mediumCAScript), so public registries keep working.

const mediumCAMount = "/wrm-medium-ca.pem"

// mediumCAScript runs before the container's command: the image's roots plus the host's.
const mediumCAScript = `cat /etc/ssl/certs/ca-certificates.crt ` + mediumCAMount + ` >/tmp/wrm-ca.pem 2>/dev/null; exec "$@"`

func mediumCAPath() string { return filepath.Join(mediumRoot(), "host-ca.pem") }

// mediumCAEnv points pip, Python, requests and git at the combined bundle.
var mediumCAEnv = []string{"SSL_CERT_FILE=/tmp/wrm-ca.pem", "PIP_CERT=/tmp/wrm-ca.pem", "REQUESTS_CA_BUNDLE=/tmp/wrm-ca.pem", "GIT_SSL_CAINFO=/tmp/wrm-ca.pem", "CURL_CA_BUNDLE=/tmp/wrm-ca.pem"}

// mediumWriteCABundle writes (or removes) the bundle and returns how many certificates it holds.
func mediumWriteCABundle() (int, error) {
	var certs []*x509.Certificate
	if settingBool("medium_host_ca") {
		if roots, e := mediumHostRoots(); e == nil {
			certs = append(certs, roots...)
		} else if out := mediumOpOutput(); out != nil {
			out.Note("the WRM host's trusted certificates could not be read: %v", e)
		}
	}
	if f := strings.TrimSpace(getSetting("medium_ca_file")); f != "" {
		b, e := os.ReadFile(f)
		if e != nil {
			return 0, fmt.Errorf("Cannot read the CA file %s (setting medium_ca_file): %v", f, e)
		}
		extra := mediumParsePEM(b)
		if len(extra) == 0 {
			return 0, fmt.Errorf("The CA file %s (setting medium_ca_file) holds no PEM certificate (-----BEGIN CERTIFICATE-----)", f)
		}
		certs = append(certs, extra...)
	}
	var buf bytes.Buffer
	buf.WriteByte('\n') // the image's bundle may not end with a newline
	seen := map[[32]byte]bool{}
	now := time.Now()
	n := 0
	for _, c := range certs {
		h := sha256.Sum256(c.Raw)
		if seen[h] || now.After(c.NotAfter) {
			continue
		}
		seen[h] = true
		pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
		n++
	}
	path := mediumCAPath()
	if n == 0 {
		if e := os.Remove(path); e != nil && !os.IsNotExist(e) {
			return 0, e
		}
		return 0, nil
	}
	tmp := path + ".tmp"
	if e := os.WriteFile(tmp, buf.Bytes(), 0644); e != nil { // public certificates only
		return 0, e
	}
	return n, os.Rename(tmp, path)
}

// mediumCAArgs are the docker run arguments that hand the bundle to a container; the
// entrypoint becomes /bin/sh running mediumCAScript, so the caller passes the real
// entrypoint as the first command argument (see mediumCACommand).
func mediumCAArgs() []string {
	if !mediumFileExists(mediumCAPath()) {
		return nil
	}
	args := []string{"--volume", mediumCAPath() + ":" + mediumCAMount + ":ro"}
	for _, e := range mediumCAEnv {
		args = append(args, "--env", e)
	}
	return args
}

// mediumCACommand returns the entrypoint and the arguments placed after the image.
func mediumCACommand(entrypoint string, args []string) (string, []string) {
	if !mediumFileExists(mediumCAPath()) {
		return entrypoint, args
	}
	return "/bin/sh", append([]string{"-c", mediumCAScript, "sh", entrypoint}, args...)
}

func mediumParsePEM(b []byte) []*x509.Certificate {
	var out []*x509.Certificate
	for {
		var blk *pem.Block
		blk, b = pem.Decode(b)
		if blk == nil {
			return out
		}
		if blk.Type != "CERTIFICATE" {
			continue
		}
		if c, e := x509.ParseCertificate(blk.Bytes); e == nil {
			out = append(out, c)
		}
	}
}

// mediumHostRootsUnix reads the host's roots on macOS (keychains) and other Unix systems
// (the distribution's bundle). Windows is in git_medium_ca_windows.go.
func mediumHostRootsUnix() ([]*x509.Certificate, error) {
	if runtime.GOOS == "darwin" {
		var all []*x509.Certificate
		keychains := []string{"/Library/Keychains/System.keychain", "/System/Library/Keychains/SystemRootCertificates.keychain"}
		if home, e := os.UserHomeDir(); e == nil {
			keychains = append(keychains, filepath.Join(home, "Library", "Keychains", "login.keychain-db"))
		}
		for _, k := range keychains {
			if !mediumFileExists(k) {
				continue
			}
			out, e := exec.Command("/usr/bin/security", "find-certificate", "-a", "-p", k).Output()
			if e == nil {
				all = append(all, mediumParsePEM(out)...)
			}
		}
		if len(all) == 0 {
			return nil, fmt.Errorf("no certificate in the macOS keychains")
		}
		return all, nil
	}
	files := []string{os.Getenv("SSL_CERT_FILE"), "/etc/ssl/certs/ca-certificates.crt", "/etc/pki/tls/certs/ca-bundle.crt", "/etc/ssl/ca-bundle.pem", "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", "/etc/ssl/cert.pem", "/usr/local/share/certs/ca-root-nss.crt"}
	for _, f := range files {
		if f == "" {
			continue
		}
		if b, e := os.ReadFile(f); e == nil {
			if c := mediumParsePEM(b); len(c) > 0 {
				return c, nil
			}
		}
	}
	return nil, fmt.Errorf("no CA bundle found on the host")
}
