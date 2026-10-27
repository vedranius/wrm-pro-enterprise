// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── SERVER HOSTNAMES ────────────────────────────────
//
// The Git workspace names a server by its real hostname, not by the name the user gave the
// connection. The hostname is read with the plain `hostname` command over the SSH login
// that a check, a comparison or a plan already makes (nothing is changed on the server) and
// cached per connection in git_hostnames. When it is not known yet, the connection's host
// is shown when it is a DNS name; an IP address is never the label (the connection name is
// shown instead, the address stays visible as secondary text), so one list never mixes
// hostnames and bare IP addresses.

const gitHostnameTTL = 6 * time.Hour

var hostnameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]{0,251}[A-Za-z0-9])?$`)

// hostnameScript prints the hostname behind a marker, so a login script that writes to
// stdout (a banner, a profile echo) cannot be taken for the hostname.
const hostnameScript = `printf 'WRM_HOSTNAME %s\n' "$(hostname 2>/dev/null || uname -n 2>/dev/null)"`

// parseHostnameOutput finds the marked hostname line; output without a marker (an older
// script) falls back to its first line.
func parseHostnameOutput(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "WRM_HOSTNAME "); ok {
			return cleanHostname(rest)
		}
	}
	return cleanHostname(out)
}

// cleanHostname returns the first line of s when it is a valid hostname, else "".
func cleanHostname(s string) string {
	s = strings.TrimSpace(strings.SplitN(strings.TrimSpace(s), "\n", 2)[0])
	if hostnameRe.MatchString(s) {
		return s
	}
	return ""
}

func storeHostname(connID int, h string) {
	if h == "" || connID <= 0 {
		return
	}
	db.Exec(`INSERT INTO git_hostnames (conn_id, hostname, updated_at) VALUES (?,?,?)
		ON CONFLICT(conn_id) DO UPDATE SET hostname=excluded.hostname, updated_at=excluded.updated_at`, connID, h, nowStamp())
}

// hostnameFresh tells whether a hostname was read for the connection recently.
func hostnameFresh(connID int) bool {
	var at string
	if db.QueryRow(`SELECT updated_at FROM git_hostnames WHERE conn_id=?`, connID).Scan(&at) != nil {
		return false
	}
	t, err := time.Parse(time.RFC3339, at)
	return err == nil && time.Since(t) < gitHostnameTTL
}

// cacheServerHostname reads the hostname over an open login (at most every few hours per
// connection) and stores it; a failure leaves the cache as it is.
func cacheServerHostname(connID int, cl *ssh.Client) {
	if cl == nil || hostnameFresh(connID) {
		return
	}
	out, err := runRemoteScript(cl, hostnameScript, "", 15*time.Second)
	if err != nil {
		return
	}
	storeHostname(connID, parseHostnameOutput(out))
}

// hostLabels maps the connection names of a user to what the Git workspace shows for the
// server: the cached hostname, else the connection's host when it is a DNS name. A
// connection known only by an IP address has no entry: the client shows its name.
func hostLabels(userID int) map[string]string {
	out := map[string]string{}
	rows, err := db.Query(`SELECT c.name, c.host, COALESCE(h.hostname,'') FROM connections c LEFT JOIN git_hostnames h ON h.conn_id=c.id WHERE c.user_id=?`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var name, host, hn string
		if rows.Scan(&name, &host, &hn) != nil {
			continue
		}
		switch {
		case hn != "":
			out[name] = hn
		case strings.TrimSpace(host) != "" && net.ParseIP(hostOnly(strings.TrimSpace(host))) == nil:
			out[name] = strings.TrimSpace(host)
		}
	}
	return out
}
