// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── SSH HOST KEY VERIFICATION ───────────────────────
//
// Like OpenSSH's known_hosts: the first key a server presents is remembered (trust on
// first use) and every later connection must present the same key. A different key is
// refused as a possible man-in-the-middle attack until the owner of the connection or an
// administrator accepts the new key. Policies (Admin → Policies → host_key_policy):
//   tofu   – remember new hosts automatically (default)
//   strict – unknown hosts must be approved first
//   off    – no verification (not recommended)

type hostKeyError struct {
	Kind     string `json:"kind"` // "mismatch" | "unknown"
	Host     string `json:"host"`
	KeyType  string `json:"key_type"`
	Expected string `json:"expected,omitempty"`
	Got      string `json:"fingerprint"`
}

func (e *hostKeyError) Error() string {
	if e.Kind == "mismatch" {
		return fmt.Sprintf("HOST KEY VERIFICATION FAILED for %s: the server presented %s %s but %s was expected. "+
			"This can be a man-in-the-middle attack. If the server was reinstalled, accept the new key.", e.Host, e.KeyType, e.Got, e.Expected)
	}
	return fmt.Sprintf("Unknown host %s (%s %s). The host key must be approved before connecting.", e.Host, e.KeyType, e.Got)
}

type pendingHostKey struct {
	key  []byte // wire format for SSH, DER leaf certificate for TLS
	kind string
	seen time.Time
}

var pendingHostKeys = struct {
	sync.Mutex
	m map[string]pendingHostKey // host|fingerprint → key
}{m: map[string]pendingHostKey{}}

func rememberPendingKey(host, fp string, key []byte, kind string) {
	pendingHostKeys.Lock()
	defer pendingHostKeys.Unlock()
	for k, v := range pendingHostKeys.m {
		if time.Since(v.seen) > time.Hour {
			delete(pendingHostKeys.m, k)
		}
	}
	pendingHostKeys.m[host+"|"+fp] = pendingHostKey{key: key, kind: kind, seen: time.Now()}
}

func normalizeHostKeyHost(hostport string) string {
	h, p, err := net.SplitHostPort(hostport)
	if err != nil {
		return strings.ToLower(hostport)
	}
	return strings.ToLower(net.JoinHostPort(h, p))
}

type storedHostKey struct {
	ID          int
	KeyType     string
	Fingerprint string
	Key         string
}

func lookupHostKey(host string) (*storedHostKey, bool) {
	var k storedHostKey
	err := db.QueryRow(`SELECT id, key_type, fingerprint, public_key FROM known_hosts WHERE host=?`, host).Scan(&k.ID, &k.KeyType, &k.Fingerprint, &k.Key)
	return &k, err == nil
}

func storeHostKey(host, keyType, fp string, key []byte, by string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO known_hosts (host, key_type, fingerprint, public_key, first_seen, last_seen, added_by) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(host) DO UPDATE SET key_type=excluded.key_type, fingerprint=excluded.fingerprint, public_key=excluded.public_key,
		first_seen=excluded.first_seen, last_seen=excluded.last_seen, added_by=excluded.added_by`,
		host, keyType, fp, base64.StdEncoding.EncodeToString(key), now, now, by)
	return err
}

// hostKeyAlgorithmsFor keeps the server on the key type we already know, so a server with
// several host keys does not trigger a false "key changed" warning.
func hostKeyAlgorithmsFor(keyType string) []string {
	switch keyType {
	case ssh.KeyAlgoRSA:
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	case "":
		return nil
	}
	return []string{keyType}
}

// canAutoTrustHost says whether a first-use host key may be stored without an explicit
// accept: the owner of a connection to that host, or an administrator. A user must not be
// able to seed trust for a host that belongs to someone else.
func canAutoTrustHost(userID int, host string) bool {
	if userID <= 0 {
		return true // internal probes and command-line helpers
	}
	return isAdminUser(userID) || userOwnsConnectionToHost(userID, host) || userOwnsBMCHost(userID, host)
}

// userOwnsBMCHost reports whether the user has a connection whose BMC SSH address is host.
// The BMC console dials the BMC address (not the server), so it is a separate ownership
// check from the connection's own host.
func userOwnsBMCHost(userID int, host string) bool {
	rows, err := db.Query(`SELECT bmc FROM connections WHERE user_id=? AND COALESCE(bmc,'')<>''`, userID)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			continue
		}
		var b bmcConfig
		if json.Unmarshal([]byte(raw), &b) != nil || strings.TrimSpace(b.Host) == "" {
			continue
		}
		port := 22
		if b.SSHPort > 0 {
			port = b.SSHPort
		}
		if normalizeHostKeyHost(joinHostPortSafe(hostOnly(b.Host), strconv.Itoa(port))) == host {
			return true
		}
	}
	return false
}

// sshHostKeyCallback verifies the server key. onNew is called when a new key was trusted.
func sshHostKeyCallback(connHost string, userID int, onNew func(fp string)) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		policy := getSetting("host_key_policy")
		if policy == "off" {
			return nil
		}
		host := normalizeHostKeyHost(connHost)
		fp := ssh.FingerprintSHA256(key)
		stored, ok := lookupHostKey(host)
		if !ok {
			if policy == "strict" {
				rememberPendingKey(host, fp, key.Marshal(), key.Type())
				return &hostKeyError{Kind: "unknown", Host: host, KeyType: key.Type(), Got: fp}
			}
			if !canAutoTrustHost(userID, host) {
				rememberPendingKey(host, fp, key.Marshal(), key.Type())
				return &hostKeyError{Kind: "unknown", Host: host, KeyType: key.Type(), Got: fp}
			}
			if err := storeHostKey(host, key.Type(), fp, key.Marshal(), "tofu"); err == nil {
				auditLogAs(nil, 0, "", "hostkey.trusted", host, map[string]string{"fingerprint": fp, "type": key.Type(), "how": "first use"})
				if onNew != nil {
					onNew(fp)
				}
			}
			return nil
		}
		if stored.Key == base64.StdEncoding.EncodeToString(key.Marshal()) {
			db.Exec(`UPDATE known_hosts SET last_seen=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), stored.ID)
			return nil
		}
		rememberPendingKey(host, fp, key.Marshal(), key.Type())
		auditLogAs(nil, 0, "", "hostkey.mismatch", host, map[string]string{"expected": stored.Fingerprint, "got": fp})
		return &hostKeyError{Kind: "mismatch", Host: host, KeyType: key.Type(), Expected: stored.Fingerprint, Got: fp}
	}
}

// sshClientConfig builds the client configuration used for every SSH connection.
func sshClientConfig(c Connection, am []ssh.AuthMethod, onNew func(string)) *ssh.ClientConfig {
	cfg := &ssh.ClientConfig{
		User: c.Username, Auth: am,
		HostKeyCallback: sshHostKeyCallback(c.Host, c.UserID, onNew),
		Timeout:         15 * time.Second,
	}
	if getSetting("host_key_policy") != "off" {
		if stored, ok := lookupHostKey(normalizeHostKeyHost(c.Host)); ok && !strings.HasPrefix(stored.KeyType, "tls") {
			cfg.HostKeyAlgorithms = hostKeyAlgorithmsFor(stored.KeyType)
		}
	}
	return cfg
}

func asHostKeyError(err error) *hostKeyError {
	for e := err; e != nil; {
		if hk, ok := e.(*hostKeyError); ok {
			return hk
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			break
		}
		e = u.Unwrap()
	}
	return nil
}

// ─── FTPS CERTIFICATES ───────────────────────────────
// A certificate that is valid for the host name (public CA) is accepted. Otherwise
// (self-signed, internal CA) the certificate is pinned on first use like an SSH key.

func ftpsTLSConfig(c Connection) *tls.Config {
	return pinnedTLSConfig("ftps://", c.Host)
}

// ─── API ─────────────────────────────────────────────

// POST /api/hostkeys/accept {host, fingerprint}
// Accepts a key that a connection attempt just reported as new or changed. Allowed for
// administrators and for users who own a connection to that host.
func apiHostKeyAcceptHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var p struct {
		Host        string `json:"host"`
		Fingerprint string `json:"fingerprint"`
	}
	json.NewDecoder(r.Body).Decode(&p)
	host := strings.ToLower(strings.TrimSpace(p.Host))
	if !isAdminUser(userID) && !userOwnsConnectionToHost(userID, strings.TrimPrefix(host, "ftps://")) && !(strings.HasPrefix(host, "bmc://") && userOwnsBMC(userID, strings.TrimPrefix(host, "bmc://"))) {
		jsonError(w, "Only the owner of the connection or an administrator can accept a host key", 403)
		return
	}
	pendingHostKeys.Lock()
	pk, found := pendingHostKeys.m[host+"|"+p.Fingerprint]
	pendingHostKeys.Unlock()
	if !found {
		jsonError(w, "This key is no longer pending — connect again and retry", 404)
		return
	}
	old, hadOld := lookupHostKey(host)
	if err := storeHostKey(host, pk.kind, p.Fingerprint, pk.key, usernameOf(userID)); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	details := map[string]string{"fingerprint": p.Fingerprint, "type": pk.kind}
	if hadOld {
		details["previous"] = old.Fingerprint
	}
	auditLog(r, userID, "hostkey.accepted", host, details)
	jsonOK(w, map[string]bool{"ok": true})
}

func userOwnsConnectionToHost(userID int, host string) bool {
	rows, err := db.Query(`SELECT host, protocol FROM connections WHERE user_id=?`, userID)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var h, proto string
		rows.Scan(&h, &proto)
		if normalizeHostKeyHost(ensurePort(h, proto)) == host {
			return true
		}
	}
	return false
}

// GET /api/admin/known-hosts · DELETE /api/admin/known-hosts/{id}
func apiAdminKnownHostsHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	idStr := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/known-hosts"), "/")
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, host, key_type, fingerprint, first_seen, last_seen, COALESCE(added_by,'') FROM known_hosts ORDER BY host`)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		out := []map[string]interface{}{}
		for rows.Next() {
			var id int
			var host, kt, fp, first, last, by string
			rows.Scan(&id, &host, &kt, &fp, &first, &last, &by)
			out = append(out, map[string]interface{}{"id": id, "host": host, "key_type": kt, "fingerprint": fp, "first_seen": first, "last_seen": last, "added_by": by})
		}
		jsonOK(w, out)
	case http.MethodDelete:
		id, _ := strconv.Atoi(idStr)
		var host string
		db.QueryRow(`SELECT host FROM known_hosts WHERE id=?`, id).Scan(&host)
		res, _ := db.Exec(`DELETE FROM known_hosts WHERE id=?`, id)
		if n, _ := res.RowsAffected(); n == 0 {
			jsonError(w, "Not found", 404)
			return
		}
		auditLog(r, adminID, "hostkey.removed", host, nil)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}
