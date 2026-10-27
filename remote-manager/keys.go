// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── SSH KEY STORE ───────────────────────────────────
//
// Every user has a key store: SSH key pairs generated in WRM or imported, and public keys
// of other people (so they can be deployed). Private keys are encrypted at rest like the
// other secrets and never sent to the browser, except when the owner exports one after
// entering the account password again.
//
// A connection logs in with a stored key with the auth method KEY_REF (connections.key_id);
// vault credentials can use a stored key too.
//
// Deploy appends a public key to ~/.ssh/authorized_keys on selected servers, like
// ssh-copy-id: WRM logs in with each connection's current credentials (through its jump
// hosts), adds the key once, and can then switch the connection to the key. Revoke
// removes the key again. "Who has access" lists the authorized_keys of a connection's login
// user, recognises the keys of the key store and removes single entries.
//
// The remote side is plain POSIX sh + awk. The key travels on stdin, never on a command line.

const (
	maxKeysPerUser    = 500
	maxKeyDeployHosts = 200
	remoteCmdTimeout  = 45 * time.Second
)

type sshKey struct {
	ID          int          `json:"id"`
	UserID      int          `json:"-"`
	Name        string       `json:"name"`
	KeyType     string       `json:"key_type"`
	Bits        int          `json:"bits"`
	PublicKey   string       `json:"public_key"` // authorized_keys line
	Fingerprint string       `json:"fingerprint"`
	Comment     string       `json:"comment"`
	HasPrivate  bool         `json:"has_private"`
	CreatedAt   string       `json:"created_at"`
	LastUsedAt  string       `json:"last_used_at"`
	UsedBy      []string     `json:"used_by"`               // connections and credentials that log in with it
	Deployed    []int        `json:"deployed"`              // connections WRM installed it on
	Certificate string       `json:"certificate,omitempty"` // OpenSSH user certificate (public, ssh_cert.go)
	CertInfo    *sshCertInfo `json:"cert,omitempty"`
	private     string       // encrypted private key
}

const sshKeyCols = `id,user_id,name,key_type,bits,public_key,private_key,fingerprint,comment,created_at,last_used_at,certificate`

func scanSSHKey(sc interface{ Scan(...interface{}) error }) (sshKey, error) {
	var k sshKey
	err := sc.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyType, &k.Bits, &k.PublicKey, &k.private, &k.Fingerprint, &k.Comment, &k.CreatedAt, &k.LastUsedAt, &k.Certificate)
	k.HasPrivate = k.private != ""
	if k.Certificate != "" {
		if cert, cerr := parseUserCert(k.Certificate); cerr == nil {
			k.CertInfo = certInfoOf(cert)
		}
	}
	return k, err
}

// storedKeyCert returns the certificate saved with a stored key ("" = none).
func storedKeyCert(keyID *int, userID int) string {
	if keyID == nil {
		return ""
	}
	var cert string
	db.QueryRow(`SELECT certificate FROM ssh_keys WHERE id=? AND user_id=?`, *keyID, userID).Scan(&cert)
	return cert
}

func loadSSHKey(id, userID int) (sshKey, error) {
	k, err := scanSSHKey(db.QueryRow(`SELECT `+sshKeyCols+` FROM ssh_keys WHERE id=? AND user_id=?`, id, userID))
	if err != nil {
		return k, fmt.Errorf("key not found")
	}
	return k, nil
}

func loadSSHKeys(userID int) []sshKey {
	out := []sshKey{}
	rows, err := db.Query(`SELECT `+sshKeyCols+` FROM ssh_keys WHERE user_id=? ORDER BY name COLLATE NOCASE, id`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		if k, err := scanSSHKey(rows); err == nil {
			out = append(out, k)
		}
	}
	return out
}

// storedKeyPEM returns the decrypted private key of a key in userID's key store.
func storedKeyPEM(keyID *int, userID int) (string, error) {
	if keyID == nil || *keyID <= 0 {
		return "", fmt.Errorf("no SSH key selected")
	}
	k, err := loadSSHKey(*keyID, userID)
	if err != nil {
		return "", fmt.Errorf("the SSH key of this connection no longer exists")
	}
	if !k.HasPrivate {
		return "", fmt.Errorf("SSH key %q is a public key only (no private key in WRM)", k.Name)
	}
	p := decryptValue(k.private)
	if p == "" {
		return "", fmt.Errorf("SSH key %q cannot be decrypted (was the encryption key changed?)", k.Name)
	}
	return p, nil
}

// touchKeyUsed records that a stored key was used to log in (at most once a minute).
func touchKeyUsed(id int) {
	if id <= 0 {
		return
	}
	now := time.Now().UTC()
	db.Exec(`UPDATE ssh_keys SET last_used_at=? WHERE id=? AND (last_used_at='' OR last_used_at<?)`,
		now.Format(time.RFC3339), id, now.Add(-time.Minute).Format(time.RFC3339))
}

// keyUsage lists what logs in with a key: the owner's connections and credentials.
func keyUsage(keyID, userID int) []string {
	used := []string{}
	if rows, err := db.Query(`SELECT name FROM connections WHERE user_id=? AND auth_method='KEY_REF' AND key_id=? ORDER BY name`, userID, keyID); err == nil {
		for rows.Next() {
			var n string
			rows.Scan(&n)
			used = append(used, n)
		}
		rows.Close()
	}
	if rows, err := db.Query(`SELECT name FROM credentials WHERE owner_id=? AND key_id=? ORDER BY name`, userID, keyID); err == nil {
		for rows.Next() {
			var n string
			rows.Scan(&n)
			used = append(used, "🗝 "+n)
		}
		rows.Close()
	}
	return used
}

func keyDeployments(keyID int) []int {
	ids := []int{}
	if rows, err := db.Query(`SELECT d.conn_id FROM ssh_key_deployments d JOIN connections c ON c.id=d.conn_id WHERE d.key_id=? ORDER BY d.conn_id`, keyID); err == nil {
		for rows.Next() {
			var id int
			rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
	}
	return ids
}

func setKeyDeployed(keyID, connID, userID int, deployed bool) {
	if deployed {
		db.Exec(`INSERT OR REPLACE INTO ssh_key_deployments (key_id, conn_id, user_id, deployed_at) VALUES (?,?,?,?)`,
			keyID, connID, userID, time.Now().UTC().Format(time.RFC3339))
	} else {
		db.Exec(`DELETE FROM ssh_key_deployments WHERE key_id=? AND conn_id=?`, keyID, connID)
	}
}

// ─── KEY MATERIAL ────────────────────────────────────

var keyCommentRe = regexp.MustCompile(`[^\p{L}\p{N} @._+:=,()/-]`)

func cleanKeyComment(s string) string {
	s = keyCommentRe.ReplaceAllString(strings.TrimSpace(s), "")
	return truncateStr(strings.Join(strings.Fields(s), " "), 100)
}

func publicKeyBits(pub ssh.PublicKey) int {
	cpk, ok := pub.(ssh.CryptoPublicKey)
	if !ok {
		return 0
	}
	switch k := cpk.CryptoPublicKey().(type) {
	case *rsa.PublicKey:
		return k.N.BitLen()
	case *ecdsa.PublicKey:
		return k.Curve.Params().BitSize
	case ed25519.PublicKey:
		return 256
	}
	return 0
}

// keyTypeLabel is a short name for a key type ("ED25519", "RSA 4096", ...).
func keyTypeLabel(t string, bits int) string {
	switch {
	case t == ssh.KeyAlgoED25519:
		return "ED25519"
	case t == ssh.KeyAlgoRSA:
		return fmt.Sprintf("RSA %d", bits)
	case strings.HasPrefix(t, "ecdsa-"):
		return fmt.Sprintf("ECDSA %d", bits)
	case strings.HasPrefix(t, "sk-"):
		return "Security key"
	}
	return t
}

func authorizedLine(pub ssh.PublicKey, comment string) string {
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	if comment != "" {
		line += " " + comment
	}
	return line
}

// newKeyPair generates a private key: ed25519 (default), rsa (2048/3072/4096) or ecdsa (256/384/521).
func newKeyPair(kind string, bits int) (interface{}, error) {
	switch strings.ToLower(kind) {
	case "", "ed25519":
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		return priv, err
	case "rsa":
		if bits == 0 {
			bits = 4096
		}
		if bits != 2048 && bits != 3072 && bits != 4096 {
			return nil, fmt.Errorf("RSA keys have 2048, 3072 or 4096 bits")
		}
		return rsa.GenerateKey(rand.Reader, bits)
	case "ecdsa":
		curves := map[int]elliptic.Curve{0: elliptic.P256(), 256: elliptic.P256(), 384: elliptic.P384(), 521: elliptic.P521()}
		c, ok := curves[bits]
		if !ok {
			return nil, fmt.Errorf("ECDSA keys have 256, 384 or 521 bits")
		}
		return ecdsa.GenerateKey(c, rand.Reader)
	}
	return nil, fmt.Errorf("unknown key type %q (ed25519, rsa or ecdsa)", kind)
}

// fillFromPrivate sets the public part and the (unencrypted, OpenSSH format) private key.
func (k *sshKey) fillFromPrivate(raw interface{}) error {
	signer, err := ssh.NewSignerFromKey(raw)
	if err != nil {
		return fmt.Errorf("unsupported key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(raw, k.Comment)
	if err != nil {
		return fmt.Errorf("unsupported key type (use ED25519, RSA or ECDSA): %v", err)
	}
	k.fillFromPublic(signer.PublicKey())
	k.private = string(pem.EncodeToMemory(block))
	k.HasPrivate = true
	return nil
}

func (k *sshKey) fillFromPublic(pub ssh.PublicKey) {
	k.KeyType = pub.Type()
	k.Bits = publicKeyBits(pub)
	k.Fingerprint = ssh.FingerprintSHA256(pub)
	k.PublicKey = authorizedLine(pub, k.Comment)
}

type passphraseNeeded struct{}

func (passphraseNeeded) Error() string { return "this private key is protected with a passphrase" }

// parseImportedKey accepts an OpenSSH / PEM private key (with its passphrase) or a public key line.
func parseImportedKey(text, passphrase string) (raw interface{}, pub ssh.PublicKey, comment string, err error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil, "", fmt.Errorf("paste a private key or a public key")
	}
	if strings.Contains(text, "PuTTY-User-Key-File") {
		return nil, nil, "", fmt.Errorf("PuTTY keys (.ppk): convert with PuTTYgen → Conversions → Export OpenSSH key, then paste that")
	}
	if strings.Contains(text, "PRIVATE KEY") {
		if passphrase != "" {
			raw, err = ssh.ParseRawPrivateKeyWithPassphrase([]byte(text), []byte(passphrase))
			if errors.Is(err, x509.IncorrectPasswordError) {
				return nil, nil, "", fmt.Errorf("wrong passphrase")
			}
		} else {
			raw, err = ssh.ParseRawPrivateKey([]byte(text))
			var pm *ssh.PassphraseMissingError
			if errors.As(err, &pm) {
				return nil, nil, "", passphraseNeeded{}
			}
		}
		if err != nil {
			if strings.Contains(err.Error(), "decryption password incorrect") {
				return nil, nil, "", fmt.Errorf("wrong passphrase")
			}
			return nil, nil, "", fmt.Errorf("cannot read the private key: %v", err)
		}
		return raw, nil, "", nil
	}
	pk, cmt, _, _, perr := ssh.ParseAuthorizedKey([]byte(text))
	if perr != nil {
		return nil, nil, "", fmt.Errorf("not an SSH key (paste a private key or a line like ssh-ed25519 AAAA… comment)")
	}
	return nil, pk, cmt, nil
}

// loginKeyFingerprint returns the fingerprint of the key a (resolved) connection logs in
// with, or "" for password logins.
func loginKeyFingerprint(c Connection) string {
	if c.AuthMethod == "PASSWORD" || c.AuthMethod == "" || c.authErr != "" {
		return ""
	}
	kd, err := resolveKeyMaterial(c)
	if err != nil {
		return ""
	}
	s, err := parsePrivateKeyBytes(kd)
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(s.PublicKey())
}

// ─── REMOTE COMMANDS ─────────────────────────────────

type capBuffer struct {
	bytes.Buffer
	max int
}

func (b *capBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room > 0 {
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil
}

// runRemoteScript runs a POSIX sh script on an SSH client, feeding stdin, and returns stdout.
func runRemoteScript(cl *ssh.Client, script, stdin string, timeout time.Duration) (string, error) {
	s, err := cl.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	out, errb := &capBuffer{max: 2 << 20}, &capBuffer{max: 64 << 10}
	s.Stdin = strings.NewReader(stdin)
	s.Stdout, s.Stderr = out, errb
	done := make(chan error, 1)
	go func() { done <- s.Run("sh -c " + shellQuote(script)) }()
	select {
	case err = <-done:
	case <-time.After(timeout):
		s.Close()
		return "", fmt.Errorf("the server did not finish within %s", timeout)
	}
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), errors.New(truncateStr(msg, 400))
	}
	return out.String(), nil
}

// Scripts for ~/.ssh/authorized_keys. Line 1 of stdin is the key line or the key blob.
const (
	scriptAddKey = `umask 077
d="$HOME/.ssh"; f="$d/authorized_keys"
IFS= read -r k || [ -n "$k" ] || exit 3
b=$(printf '%s\n' "$k" | awk '{print $2}')
[ -n "$b" ] || exit 3
mkdir -p "$d" || exit 4
[ -f "$f" ] || : > "$f" || exit 4
if awk -v b="$b" '{for(i=1;i<=NF;i++) if($i==b) f=1} END{exit !f}' "$f"; then echo WRM_ALREADY; exit 0; fi
if [ -s "$f" ] && [ -n "$(tail -c 1 "$f")" ]; then echo >> "$f"; fi
printf '%s\n' "$k" >> "$f" || exit 5
if command -v restorecon >/dev/null 2>&1; then restorecon -F "$d" "$f" >/dev/null 2>&1; fi
echo WRM_ADDED`
	scriptRemoveKey = `f="$HOME/.ssh/authorized_keys"
IFS= read -r b || [ -n "$b" ] || exit 3
[ -n "$b" ] || exit 3
[ -f "$f" ] || { echo "WRM_REMOVED 0"; exit 0; }
n=$(awk -v b="$b" '{for(i=1;i<=NF;i++) if($i==b){c++; next}} END{print c+0}' "$f") || exit 4
[ "$n" -gt 0 ] || { echo "WRM_REMOVED 0"; exit 0; }
t="$f.wrm.$$"
(umask 077; awk -v b="$b" '{for(i=1;i<=NF;i++) if($i==b) next} {print}' "$f" > "$t") && cat "$t" > "$f"; r=$?
rm -f "$t"
[ $r -eq 0 ] || exit 5
echo "WRM_REMOVED $n"`
	scriptReadKeys = `f="$HOME/.ssh/authorized_keys"
echo "WRM_USER $(id -un 2>/dev/null)"
echo "WRM_FILE $f"
if [ -f "$f" ]; then echo WRM_BEGIN; cat "$f"; echo; echo WRM_END; else echo WRM_NONE; fi`
)

func keyBlob(line string) string {
	f := strings.Fields(line)
	if len(f) < 2 {
		return ""
	}
	return f[1]
}

// authorizedEntry is one line of a remote authorized_keys file.
type authorizedEntry struct {
	Line        int    `json:"line"`
	KeyType     string `json:"key_type"`
	Bits        int    `json:"bits"`
	Fingerprint string `json:"fingerprint"`
	Comment     string `json:"comment"`
	Options     string `json:"options,omitempty"`
	KeyID       int    `json:"key_id,omitempty"`   // matching key of the key store
	KeyName     string `json:"key_name,omitempty"` // its name
	Current     bool   `json:"current"`            // the key this connection logs in with
	Invalid     bool   `json:"invalid,omitempty"`
	Text        string `json:"text,omitempty"` // unparsable lines (shortened)
	blob        string
}

// parseAuthorizedKeys parses an authorized_keys file.
func parseAuthorizedKeys(data string) []authorizedEntry {
	out := []authorizedEntry{}
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pub, comment, opts, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			out = append(out, authorizedEntry{Line: i + 1, Invalid: true, Text: truncateStr(line, 80)})
			continue
		}
		e := authorizedEntry{Line: i + 1, KeyType: pub.Type(), Bits: publicKeyBits(pub), Fingerprint: ssh.FingerprintSHA256(pub),
			Comment: comment, Options: truncateStr(strings.Join(opts, ","), 200)}
		e.blob = keyBlob(strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))))
		out = append(out, e)
	}
	return out
}

// readAuthorizedKeys logs in to c and reads the login user's authorized_keys.
func readAuthorizedKeys(c Connection) (remoteUser, path string, entries []authorizedEntry, err error) {
	cl, err := dialSSH(c, nil)
	if err != nil {
		return "", "", nil, err
	}
	defer cl.Close()
	out, err := runRemoteScript(cl, scriptReadKeys, "", remoteCmdTimeout)
	if err != nil {
		return "", "", nil, fmt.Errorf("cannot read authorized_keys: %v", err)
	}
	var body []string
	in := false
	for _, l := range strings.Split(out, "\n") {
		switch {
		case !in && strings.HasPrefix(l, "WRM_USER "):
			remoteUser = strings.TrimSpace(strings.TrimPrefix(l, "WRM_USER "))
		case !in && strings.HasPrefix(l, "WRM_FILE "):
			path = strings.TrimSpace(strings.TrimPrefix(l, "WRM_FILE "))
		case !in && l == "WRM_BEGIN":
			in = true
		case in && l == "WRM_END":
			in = false
		case in:
			body = append(body, l)
		}
	}
	return remoteUser, path, parseAuthorizedKeys(strings.Join(body, "\n")), nil
}

// ─── DEPLOY / REVOKE ─────────────────────────────────

type keyHostResult struct {
	ConnID   int    `json:"conn_id"`
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Already  bool   `json:"already,omitempty"`
	Removed  int    `json:"removed,omitempty"`
	Switched bool   `json:"switched,omitempty"`
	Error    string `json:"error,omitempty"`
}

// forEachConn runs fn for up to 8 connections at a time and returns the results in order.
func forEachConn(ids []int, fn func(id int) keyHostResult) []keyHostResult {
	res := make([]keyHostResult, len(ids))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res[i] = fn(id)
		}(i, id)
	}
	wg.Wait()
	return res
}

func uniqueIDs(ids []int, max int) ([]int, error) {
	seen := map[int]bool{}
	out := []int{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("select at least one connection")
	}
	if len(out) > max {
		return nil, fmt.Errorf("at most %d connections at once", max)
	}
	return out, nil
}

// deployKey installs k on connection id (owned by userID); with useKey the connection is
// switched to the key after a successful test login with it.
func deployKey(r *http.Request, userID int, k sshKey, id int, useKey bool) keyHostResult {
	res := keyHostResult{ConnID: id}
	if !userOwnsConnection(id, userID) || isAuditorUser(userID) {
		res.Error = "connection not found"
		return res
	}
	c, err := loadConnection(id)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Name = c.Name
	if !isSSHProtocol(c) {
		res.Error = "not an SSH connection"
		return res
	}
	fail := func(e error) keyHostResult {
		res.Error = e.Error()
		auditLogRef(r, userID, usernameOf(userID), "ssh_key.deploy_failed", k.Name, map[string]interface{}{"key": k.Fingerprint, "conn": c.Name, "error": res.Error}, auditRef{ConnID: id})
		return res
	}
	cl, err := dialSSH(c, nil)
	if err != nil {
		return fail(err)
	}
	out, err := runRemoteScript(cl, scriptAddKey, k.PublicKey+"\n", remoteCmdTimeout)
	cl.Close()
	if err != nil {
		return fail(fmt.Errorf("cannot write ~/.ssh/authorized_keys: %v", err))
	}
	switch {
	case strings.Contains(out, "WRM_ALREADY"):
		res.Already = true
	case strings.Contains(out, "WRM_ADDED"):
	default:
		return fail(fmt.Errorf("unexpected answer from the server: %s", truncateStr(strings.TrimSpace(out), 200)))
	}
	res.OK = true
	setKeyDeployed(k.ID, id, userID, true)
	if useKey && k.HasPrivate {
		test := c
		test.AuthMethod, test.PrivateKey, test.Password = "KEY", decryptValue(k.private), ""
		if tc, err := dialSSH(test, nil); err != nil {
			res.Error = "the key was added, but logging in with it failed: " + err.Error()
		} else {
			tc.Close()
			db.Exec(`UPDATE connections SET auth_method='KEY_REF', key_id=?, credential_id=NULL, username=? WHERE id=? AND user_id=?`, k.ID, c.Username, id, userID)
			res.Switched = true
			tunnelMgr.restartConn(id)
		}
	}
	auditLogRef(r, userID, usernameOf(userID), "ssh_key.deployed", k.Name, map[string]interface{}{"key": k.Fingerprint, "conn": c.Name,
		"host": c.Host, "user": c.Username, "already_present": res.Already, "switched": res.Switched}, auditRef{ConnID: id})
	return res
}

// removeKeyFromConn removes the key with the given blob from a connection's authorized_keys.
func removeKeyFromConn(c Connection, blob string) (int, error) {
	cl, err := dialSSH(c, nil)
	if err != nil {
		return 0, err
	}
	defer cl.Close()
	out, err := runRemoteScript(cl, scriptRemoveKey, blob+"\n", remoteCmdTimeout)
	if err != nil {
		return 0, fmt.Errorf("cannot change ~/.ssh/authorized_keys: %v", err)
	}
	i := strings.Index(out, "WRM_REMOVED ")
	if i < 0 {
		return 0, fmt.Errorf("unexpected answer from the server: %s", truncateStr(strings.TrimSpace(out), 200))
	}
	n, _ := strconv.Atoi(strings.Fields(out[i+len("WRM_REMOVED "):] + " 0")[0])
	return n, nil
}

func revokeKey(r *http.Request, userID int, k sshKey, id int) keyHostResult {
	res := keyHostResult{ConnID: id}
	if !userOwnsConnection(id, userID) || isAuditorUser(userID) {
		res.Error = "connection not found"
		return res
	}
	c, err := loadConnection(id)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Name = c.Name
	if !isSSHProtocol(c) {
		res.Error = "not an SSH connection"
		return res
	}
	if loginKeyFingerprint(c) == k.Fingerprint {
		res.Error = "this connection logs in with this key — switch it to another login first, or you would lock WRM out"
		return res
	}
	n, err := removeKeyFromConn(c, keyBlob(k.PublicKey))
	if err != nil {
		res.Error = err.Error()
		auditLogRef(r, userID, usernameOf(userID), "ssh_key.revoke_failed", k.Name, map[string]interface{}{"key": k.Fingerprint, "conn": c.Name, "error": res.Error}, auditRef{ConnID: id})
		return res
	}
	res.OK, res.Removed = true, n
	setKeyDeployed(k.ID, id, userID, false)
	auditLogRef(r, userID, usernameOf(userID), "ssh_key.revoked", k.Name, map[string]interface{}{"key": k.Fingerprint, "conn": c.Name,
		"host": c.Host, "user": c.Username, "removed_lines": n}, auditRef{ConnID: id})
	return res
}

// ─── API ─────────────────────────────────────────────

// GET    /api/keys                    key store (no private keys)
// POST   /api/keys                    {name, comment, generate:{type,bits}} or {name, key, passphrase}
// PUT    /api/keys/{id}               {name} and / or {certificate} ("" removes it)
// DELETE /api/keys/{id}               (refused while connections or credentials use it)
// GET    /api/keys/{id}/public        public key as a file (authorized_keys line)
// POST   /api/keys/{id}/export        {password, passphrase} → private key (re-authentication)
// POST   /api/keys/{id}/deploy        {conn_ids, use_key} → per connection result
// POST   /api/keys/{id}/revoke        {conn_ids} → per connection result
func apiKeysHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/keys"), "/")
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			keys := loadSSHKeys(userID)
			for i := range keys {
				keys[i].UsedBy = keyUsage(keys[i].ID, userID)
				keys[i].Deployed = keyDeployments(keys[i].ID)
			}
			jsonOK(w, keys)
		case http.MethodPost:
			createKeyHandler(w, r, userID)
		default:
			jsonError(w, "Method not allowed", 405)
		}
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "Bad ID", 400)
		return
	}
	k, err := loadSSHKey(id, userID)
	if err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		k.UsedBy, k.Deployed = keyUsage(k.ID, userID), keyDeployments(k.ID)
		jsonOK(w, k)
	case action == "" && r.Method == http.MethodPut:
		var in struct {
			Name        *string `json:"name"`
			Certificate *string `json:"certificate"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || (in.Name == nil && in.Certificate == nil) {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if in.Certificate != nil {
			text := strings.TrimSpace(*in.Certificate)
			info := map[string]interface{}{"key_id": id, "key": k.Fingerprint}
			if text != "" {
				cert, err := parseUserCert(text)
				if err != nil {
					jsonError(w, err.Error(), 400)
					return
				}
				pub, _, _, _, perr := ssh.ParseAuthorizedKey([]byte(k.PublicKey))
				if perr != nil || !bytes.Equal(cert.Key.Marshal(), pub.Marshal()) {
					jsonError(w, "This certificate was not issued for this key", 400)
					return
				}
				text = strings.Join(strings.Fields(text)[:2], " ") // no comment
				ci := certInfoOf(cert)
				info["cert_key_id"], info["principals"], info["valid_before"], info["ca"] = ci.KeyID, ci.Principals, ci.ValidBefore, ci.CA
			}
			db.Exec(`UPDATE ssh_keys SET certificate=? WHERE id=? AND user_id=?`, text, id, userID)
			auditLog(r, userID, map[bool]string{true: "ssh_key.certificate_set", false: "ssh_key.certificate_removed"}[text != ""], k.Name, info)
		}
		if in.Name != nil {
			name := truncateStr(strings.TrimSpace(*in.Name), 120)
			if name == "" {
				jsonError(w, "Name is required", 400)
				return
			}
			db.Exec(`UPDATE ssh_keys SET name=? WHERE id=? AND user_id=?`, name, id, userID)
			if name != k.Name {
				auditLog(r, userID, "ssh_key.renamed", name, map[string]interface{}{"key_id": id, "old_name": k.Name})
			}
		}
		jsonOK(w, map[string]bool{"ok": true})
	case action == "" && r.Method == http.MethodDelete:
		if used := keyUsage(id, userID); len(used) > 0 {
			jsonError(w, "The key is in use by: "+strings.Join(used, ", ")+". Choose another login for them first.", 409)
			return
		}
		db.Exec(`DELETE FROM ssh_key_deployments WHERE key_id=?`, id)
		db.Exec(`DELETE FROM ssh_keys WHERE id=? AND user_id=?`, id, userID)
		auditLog(r, userID, "ssh_key.deleted", k.Name, map[string]interface{}{"key_id": id, "key": k.Fingerprint})
		jsonOK(w, map[string]bool{"ok": true})
	case action == "public" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", attachmentHeader(keyFileName(k.Name)+".pub"))
		w.Write([]byte(k.PublicKey + "\n"))
	case action == "export" && r.Method == http.MethodPost:
		var in struct {
			Password   string `json:"password"`
			Passphrase string `json:"passphrase"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		u, err := loadUser("id=?", userID)
		if err != nil || !userCheckPassword(u, in.Password) {
			auditLog(r, userID, "ssh_key.export_denied", k.Name, map[string]string{"reason": "wrong password"})
			jsonError(w, "Wrong password", 403)
			return
		}
		if !u.IsAdmin && !settingBool("allow_secret_export") {
			jsonError(w, "Exporting private keys is disabled by the administrator", 403)
			return
		}
		if !requireStepUp(w, r, "ssh_key_export") {
			return
		}
		if !k.HasPrivate {
			jsonError(w, "This is a public key only", 400)
			return
		}
		raw, err := ssh.ParseRawPrivateKey([]byte(decryptValue(k.private)))
		if err != nil {
			jsonError(w, "The private key cannot be decrypted", 500)
			return
		}
		var block *pem.Block
		if in.Passphrase != "" {
			block, err = ssh.MarshalPrivateKeyWithPassphrase(raw, k.Comment, []byte(in.Passphrase))
		} else {
			block, err = ssh.MarshalPrivateKey(raw, k.Comment)
		}
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		auditLog(r, userID, "ssh_key.exported", k.Name, map[string]interface{}{"key": k.Fingerprint, "with_passphrase": in.Passphrase != ""})
		jsonOK(w, map[string]string{"private_key": string(pem.EncodeToMemory(block)), "file_name": keyFileName(k.Name)})
	case (action == "deploy" || action == "revoke") && r.Method == http.MethodPost:
		var in struct {
			ConnIDs []int `json:"conn_ids"`
			UseKey  bool  `json:"use_key"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		ids, err := uniqueIDs(in.ConnIDs, maxKeyDeployHosts)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		var results []keyHostResult
		if action == "deploy" {
			results = forEachConn(ids, func(cid int) keyHostResult { return deployKey(r, userID, k, cid, in.UseKey) })
		} else {
			results = forEachConn(ids, func(cid int) keyHostResult { return revokeKey(r, userID, k, cid) })
		}
		okN := 0
		for _, res := range results {
			if res.OK {
				okN++
			}
		}
		jsonOK(w, map[string]interface{}{"results": results, "ok": okN, "failed": len(results) - okN})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func createKeyHandler(w http.ResponseWriter, r *http.Request, userID int) {
	var in struct {
		Name     string `json:"name"`
		Comment  string `json:"comment"`
		Generate *struct {
			Type string `json:"type"`
			Bits int    `json:"bits"`
		} `json:"generate"`
		Key        string `json:"key"`
		Passphrase string `json:"passphrase"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM ssh_keys WHERE user_id=?`, userID).Scan(&n)
	if n >= maxKeysPerUser {
		jsonError(w, fmt.Sprintf("At most %d keys per user", maxKeysPerUser), 400)
		return
	}
	k := sshKey{UserID: userID, Name: truncateStr(strings.TrimSpace(in.Name), 120), Comment: cleanKeyComment(in.Comment)}
	source := "generated"
	if in.Generate != nil {
		if k.Comment == "" {
			k.Comment = cleanKeyComment(usernameOf(userID) + "@wrm")
		}
		raw, err := newKeyPair(in.Generate.Type, in.Generate.Bits)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if err := k.fillFromPrivate(raw); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		raw, pub, cmt, err := parseImportedKey(in.Key, in.Passphrase)
		if err != nil {
			if _, ok := err.(passphraseNeeded); ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error(), "need_passphrase": true})
				return
			}
			jsonError(w, err.Error(), 400)
			return
		}
		if k.Comment == "" {
			k.Comment = cleanKeyComment(cmt)
		}
		if raw != nil {
			source = "imported"
			if k.Comment == "" {
				// The comment of a private key file is not readable here: name the key instead.
				k.Comment = cleanKeyComment(firstNonEmpty(k.Name, usernameOf(userID)+"@wrm"))
			}
			if err := k.fillFromPrivate(raw); err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
		} else {
			source = "public"
			k.fillFromPublic(pub)
		}
	}
	if k.Name == "" {
		k.Name = k.Comment
		if k.Name == "" {
			k.Name = keyTypeLabel(k.KeyType, k.Bits) + " " + strings.TrimPrefix(k.Fingerprint, "SHA256:")[:8]
		}
	}
	var dup string
	if db.QueryRow(`SELECT name FROM ssh_keys WHERE user_id=? AND fingerprint=?`, userID, k.Fingerprint).Scan(&dup) == nil {
		jsonError(w, fmt.Sprintf("This key is already in your key store (%q)", dup), 409)
		return
	}
	k.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO ssh_keys (user_id,name,key_type,bits,public_key,private_key,fingerprint,comment,created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		userID, k.Name, k.KeyType, k.Bits, k.PublicKey, encryptValue(k.private), k.Fingerprint, k.Comment, k.CreatedAt)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	id, _ := res.LastInsertId()
	k.ID = int(id)
	k.private = ""
	k.UsedBy, k.Deployed = []string{}, []int{}
	auditLog(r, userID, "ssh_key.created", k.Name, map[string]interface{}{"key_id": k.ID, "key": k.Fingerprint, "type": keyTypeLabel(k.KeyType, k.Bits), "source": source})
	jsonOK(w, k)
}

// GET    /api/connections/{id}/authorized-keys                 who has access (login user's authorized_keys)
// DELETE /api/connections/{id}/authorized-keys?fingerprint=…   remove that key from it
func authorizedKeysHandler(w http.ResponseWriter, r *http.Request, userID int, connID int) {
	c, err := loadConnection(connID)
	if err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	if !isSSHProtocol(c) {
		jsonError(w, "Only SSH connections have authorized_keys", 400)
		return
	}
	switch r.Method {
	case http.MethodGet:
		user, path, entries, err := readAuthorizedKeys(c)
		if err != nil {
			jsonError(w, err.Error(), 502)
			return
		}
		mine := map[string]sshKey{}
		for _, k := range loadSSHKeys(userID) {
			mine[k.Fingerprint] = k
		}
		current := loginKeyFingerprint(c)
		found := map[int]bool{}
		for i := range entries {
			e := &entries[i]
			if k, ok := mine[e.Fingerprint]; ok && !e.Invalid {
				e.KeyID, e.KeyName = k.ID, k.Name
				found[k.ID] = true
			}
			e.Current = current != "" && e.Fingerprint == current
		}
		// Keep the "deployed on" list of the key store in line with what is really there.
		for _, k := range mine {
			setKeyDeployed(k.ID, connID, userID, found[k.ID])
		}
		jsonOK(w, map[string]interface{}{"user": user, "path": path, "entries": entries, "login_key": current, "host": c.Host, "name": c.Name})
	case http.MethodDelete:
		// Fingerprints are base64 and never contain spaces: a "+" sent unescaped arrives as one.
		fp := strings.ReplaceAll(strings.TrimSpace(r.URL.Query().Get("fingerprint")), " ", "+")
		if fp == "" {
			jsonError(w, "fingerprint is required", 400)
			return
		}
		if fp == loginKeyFingerprint(c) {
			jsonError(w, "WRM logs in to this connection with this key — removing it would lock WRM out", 409)
			return
		}
		_, _, entries, err := readAuthorizedKeys(c)
		if err != nil {
			jsonError(w, err.Error(), 502)
			return
		}
		var hit *authorizedEntry
		for i := range entries {
			if entries[i].Fingerprint == fp && !entries[i].Invalid {
				hit = &entries[i]
				break
			}
		}
		if hit == nil {
			jsonError(w, "That key is not in authorized_keys (any more)", 404)
			return
		}
		n, err := removeKeyFromConn(c, hit.blob)
		if err != nil {
			jsonError(w, err.Error(), 502)
			return
		}
		var keyID int
		db.QueryRow(`SELECT id FROM ssh_keys WHERE user_id=? AND fingerprint=?`, userID, fp).Scan(&keyID)
		if keyID > 0 {
			setKeyDeployed(keyID, connID, userID, false)
		}
		auditLogRef(r, userID, usernameOf(userID), "ssh_key.revoked", firstNonEmpty(hit.Comment, fp), map[string]interface{}{"key": fp,
			"conn": c.Name, "host": c.Host, "user": c.Username, "removed_lines": n}, auditRef{ConnID: connID})
		jsonOK(w, map[string]interface{}{"ok": true, "removed": n})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// keysSummary is used by the admin overview: keys and deployments per user.
func keysSummary() map[string]int {
	var keys, priv, deployed int
	db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(private_key<>''),0) FROM ssh_keys`).Scan(&keys, &priv)
	db.QueryRow(`SELECT COUNT(*) FROM ssh_key_deployments`).Scan(&deployed)
	return map[string]int{"keys": keys, "key_pairs": priv, "deployments": deployed}
}

func keyFileName(name string) string {
	if n := strings.Trim(safeFileName(name), "._"); n != "" && n != "session" {
		return n
	}
	return "key"
}
