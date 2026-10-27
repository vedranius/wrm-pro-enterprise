// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// demoCA writes a self-signed root like the one an HTTPS-inspecting proxy installs.
func demoCA(t *testing.T, notAfter time.Time) string {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "Demo Inspecting Proxy CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, e := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	f := filepath.Join(t.TempDir(), "proxy-ca.pem")
	if e := os.WriteFile(f, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); e != nil {
		t.Fatal(e)
	}
	return f
}

func TestMediumCABundleReachesContainers(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	t.Setenv("WRM_MEDIUM_RUNTIME", "docker")
	t.Setenv("WRM_MEDIUM_HOST_CA", "0")
	if e := os.MkdirAll(mediumRoot(), 0700); e != nil {
		t.Fatal(e)
	}
	// nothing to trust: no bundle, containers start as before
	if n, e := mediumWriteCABundle(); e != nil || n != 0 || mediumFileExists(mediumCAPath()) {
		t.Fatalf("empty bundle: %d %v", n, e)
	}
	if s := strings.Join(mediumDockerStartArgs(7, 19007, "x", nil), " "); strings.Contains(s, mediumCAMount) || !strings.Contains(s, "--entrypoint /wrm-medium/docker-venv/bin/python ") {
		t.Fatal(s)
	}
	// an extra CA file (setting medium_ca_file); an expired one and duplicates are left out
	ca := demoCA(t, time.Now().Add(24*time.Hour))
	old := demoCA(t, time.Now().Add(-time.Minute))
	both := filepath.Join(t.TempDir(), "both.pem")
	a, _ := os.ReadFile(ca)
	b, _ := os.ReadFile(old)
	os.WriteFile(both, append(append(append([]byte{}, a...), b...), a...), 0644)
	t.Setenv("WRM_MEDIUM_CA_FILE", both)
	if n, e := mediumWriteCABundle(); e != nil || n != 1 {
		t.Fatalf("bundle: %d %v", n, e)
	}
	got, _ := os.ReadFile(mediumCAPath())
	if len(mediumParsePEM(got)) != 1 || !strings.Contains(string(got), "BEGIN CERTIFICATE") {
		t.Fatal(string(got))
	}
	args := mediumDockerStartArgs(7, 19007, "x", []string{"WRM_MEDIUM_KEY"})
	s := strings.Join(args, "\n")
	for _, want := range []string{mediumCAPath() + ":" + mediumCAMount + ":ro", "--env\nPIP_CERT=/tmp/wrm-ca.pem", "--env\nSSL_CERT_FILE=/tmp/wrm-ca.pem", "--entrypoint\n/bin/sh\n" + mediumImage() + "\n-c\n" + mediumCAScript + "\nsh\n/wrm-medium/docker-venv/bin/python\n-m\nuvicorn"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in\n%s", want, s)
		}
	}
	// the image's own roots stay: the container combines them with the host's
	if !strings.Contains(mediumCAScript, "/etc/ssl/certs/ca-certificates.crt") {
		t.Fatal(mediumCAScript)
	}
	// a file that is not PEM is refused with the setting named
	bad := filepath.Join(t.TempDir(), "bad.pem")
	os.WriteFile(bad, []byte("not a certificate"), 0644)
	t.Setenv("WRM_MEDIUM_CA_FILE", bad)
	if _, e := mediumWriteCABundle(); e == nil || !strings.Contains(e.Error(), "medium_ca_file") {
		t.Fatal(e)
	}
	t.Setenv("WRM_MEDIUM_CA_FILE", filepath.Join(t.TempDir(), "missing.pem"))
	if _, e := mediumWriteCABundle(); e == nil || !strings.Contains(e.Error(), "Cannot read") {
		t.Fatal(e)
	}
}

func TestMediumHostRootsRead(t *testing.T) {
	roots, e := mediumHostRoots()
	if e != nil {
		t.Skip("no CA bundle on this host:", e)
	}
	if len(roots) == 0 {
		t.Fatal("no roots")
	}
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	t.Setenv("WRM_MEDIUM_HOST_CA", "1")
	t.Setenv("WRM_MEDIUM_CA_FILE", "")
	os.MkdirAll(mediumRoot(), 0700)
	if n, e := mediumWriteCABundle(); e != nil || n == 0 {
		t.Fatalf("host bundle: %d %v", n, e)
	}
}
