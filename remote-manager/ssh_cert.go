// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── SSH USER CERTIFICATES (#119, opkssh) ────────────
//
// A key can log in with an OpenSSH user certificate signed for it: by an SSH certificate
// authority, or by opkssh (OpenPubkey SSH), which turns an OpenID Connect sign-in into a
// short-lived certificate (`opkssh login` writes ~/.ssh/id_ecdsa and id_ecdsa-cert.pub). WRM
// offers the certificate first and the plain key after it. Where the certificate comes from:
//
//   - key files on the WRM server (KEY_FILE, KEY_AUTO): <private key>-cert.pub next to the key,
//     read again at every login, so a renewed certificate (opkssh login again) is used at once;
//     a path to the -cert.pub file itself works too;
//   - keys of the key store (KEY_REF, credentials with a key): the certificate saved with the
//     key (ssh_keys.certificate, PUT /api/keys/{id} {"certificate": "…"});
//   - a private key pasted into a connection (KEY): a certificate line pasted after the key
//     (what `cat id_ecdsa id_ecdsa-cert.pub` prints).
//
// A certificate must belong to its key (same public key) and be a user certificate. An expired
// certificate is skipped (the plain key is still offered) and a failed login names it.

const sshCertSuffix = "-cert-v01@openssh.com"

// parseUserCert parses one certificate line ("ecdsa-sha2-nistp256-cert-v01@openssh.com AAAA… comment").
func parseUserCert(text string) (*ssh.Certificate, error) {
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(text)))
	if err != nil {
		return nil, fmt.Errorf("not an SSH certificate: %v", err)
	}
	cert, ok := pk.(*ssh.Certificate)
	if !ok {
		return nil, fmt.Errorf("this is a public key, not a certificate (it starts with …%s)", sshCertSuffix)
	}
	if cert.CertType != ssh.UserCert {
		return nil, fmt.Errorf("this is a host certificate, not a user certificate")
	}
	return cert, nil
}

// splitKeyAndCert separates a certificate line from pasted key text.
func splitKeyAndCert(text string) (key, cert string) {
	var kb strings.Builder
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if cert == "" && len(f) >= 2 && strings.HasSuffix(f[0], sshCertSuffix) {
			cert = strings.TrimSpace(line)
			continue
		}
		kb.WriteString(line)
		kb.WriteByte('\n')
	}
	return strings.TrimSpace(kb.String()), cert
}

func certExpired(c *ssh.Certificate, now time.Time) bool {
	return c.ValidBefore != ssh.CertTimeInfinity && uint64(now.Unix()) >= c.ValidBefore
}

func certTime(v uint64) string {
	if v == ssh.CertTimeInfinity {
		return "forever"
	}
	if v > 1<<62 {
		return ""
	}
	return time.Unix(int64(v), 0).UTC().Format(time.RFC3339)
}

// sshCertInfo is what the browser sees of a certificate.
type sshCertInfo struct {
	Type        string   `json:"type"`
	KeyID       string   `json:"key_id"`
	Serial      uint64   `json:"serial"`
	Principals  []string `json:"principals"`
	ValidAfter  string   `json:"valid_after"`
	ValidBefore string   `json:"valid_before"`
	Expired     bool     `json:"expired"`
	CA          string   `json:"ca"` // fingerprint of the signing key
	OPKSSH      bool     `json:"opkssh,omitempty"`
}

func certInfoOf(c *ssh.Certificate) *sshCertInfo {
	if c == nil {
		return nil
	}
	_, opk := c.Permissions.Extensions["openpubkey-pkt"]
	return &sshCertInfo{Type: c.Type(), KeyID: c.KeyId, Serial: c.Serial, Principals: append([]string{}, c.ValidPrincipals...),
		ValidAfter: certTime(c.ValidAfter), ValidBefore: certTime(c.ValidBefore), Expired: certExpired(c, time.Now()),
		CA: ssh.FingerprintSHA256(c.SignatureKey), OPKSSH: opk}
}

// certFileFor is the certificate file next to a private key file.
func certFileFor(keyPath string) string { return keyPath + "-cert.pub" }

// readCertFile reads a certificate file; a missing file is no error.
func readCertFile(p string) (*ssh.Certificate, error) {
	if strings.Contains(p, "..") {
		return nil, fmt.Errorf("the certificate path %s must not contain '..'", p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read the certificate %s: %v", p, err)
	}
	cert, err := parseUserCert(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", p, err)
	}
	return cert, nil
}

// connCertificate finds the certificate of a resolved connection's key (nil = none).
// keyPath is the key file the key was read from ("" for keys from the database).
func connCertificate(c Connection, keyPath string) (*ssh.Certificate, string, error) {
	switch {
	case keyPath != "":
		p := certFileFor(keyPath)
		cert, err := readCertFile(p)
		return cert, p, err
	case c.keyCert != "":
		cert, err := parseUserCert(c.keyCert)
		return cert, "key store", err
	}
	if _, line := splitKeyAndCert(c.PrivateKey); line != "" {
		cert, err := parseUserCert(line)
		return cert, "connection", err
	}
	return nil, "", nil
}

// certSigners returns the signers of a key: its certificate first (when valid), then the key.
func certSigners(c Connection, keyPath string, sg ssh.Signer) ([]ssh.Signer, error) {
	cert, from, err := connCertificate(c, keyPath)
	if err != nil {
		return nil, err
	}
	if cert == nil {
		return []ssh.Signer{sg}, nil
	}
	if !bytes.Equal(cert.Key.Marshal(), sg.PublicKey().Marshal()) {
		return nil, fmt.Errorf("the SSH certificate (%s) was not issued for this key", from)
	}
	if certExpired(cert, time.Now()) {
		// the plain key is still offered; a refused login names the expired certificate (certLoginHint)
		return []ssh.Signer{sg}, nil
	}
	cs, err := ssh.NewCertSigner(cert, sg)
	if err != nil {
		return nil, fmt.Errorf("the SSH certificate (%s) cannot be used: %v", from, err)
	}
	return []ssh.Signer{cs, sg}, nil
}

// certLoginHint explains a refused login when the connection's certificate has expired.
func certLoginHint(c Connection) string {
	if c.AuthMethod == "PASSWORD" || c.AuthMethod == "" {
		return ""
	}
	_, keyPath, err := resolveKeyMaterialPath(c)
	if err != nil {
		return ""
	}
	cert, from, err := connCertificate(c, keyPath)
	if err != nil || cert == nil || !certExpired(cert, time.Now()) {
		return ""
	}
	hint := fmt.Sprintf("the SSH certificate (%s) expired at %s; renew it", from, certTime(cert.ValidBefore))
	if _, opk := cert.Permissions.Extensions["openpubkey-pkt"]; opk || keyPath != "" {
		hint += " (opkssh: run `opkssh login` again as the user WRM runs as)"
	}
	return hint
}
