// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// ─── CONNECTION SECRETS AND HOSTNAMES ────────────────
//
// The owner of a connection can see what it logs in with (Edit connection → Show, and
// Copy password in the connection menu) after confirming with their own WRM password: the
// local one, or the directory password of a domain account. Accounts that only sign in through
// an identity provider confirm there instead (step-up). Every reveal is audited and can raise
// the "secret revealed" alert, and the administrator can turn it off for users with
// allow_secret_export, like the vault's Reveal.
//
// A new connection can also take its name from the server: WRM logs in once and reads the
// hostname (SSH / SFTP), or looks the address up in DNS for the other protocols.

// connSecret is what a connection logs in with, for its owner.
type connSecret struct {
	Username   string `json:"username"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
	KeyPath    string `json:"key_path,omitempty"`
	Source     string `json:"source"`                // connection | key_store | vault | bitwarden | server_key
	SourceName string `json:"source_name,omitempty"` // name of the key or credential
}

// connectionSecretOf resolves the secret of c (decrypted, loaded by loadConnectionRaw) for its
// owner. Stored keys and credentials are only shown to their own owner.
func connectionSecretOf(c Connection, userID int) (connSecret, error) {
	out := connSecret{Username: c.Username, Source: "connection"}
	switch c.AuthMethod {
	case "KEY_REF":
		k, err := loadSSHKey(derefInt(c.KeyID), userID)
		if err != nil {
			return out, fmt.Errorf("the SSH key of this connection no longer exists")
		}
		p, err := storedKeyPEM(c.KeyID, userID)
		if err != nil {
			return out, err
		}
		out.PrivateKey, out.Source, out.SourceName = p, "key_store", k.Name
	case "CREDENTIAL":
		cr, err := credentialFor(c)
		if err != nil {
			return out, err
		}
		if cr.OwnerID != userID {
			return out, fmt.Errorf("credential %q belongs to another user: only its owner can show what it logs in with", cr.Name)
		}
		if cr.Username != "" {
			out.Username = cr.Username
		}
		out.Password, out.Source, out.SourceName = decryptValue(cr.password), "vault", cr.Name
		if cr.KeyID != nil {
			if p, err := storedKeyPEM(cr.KeyID, cr.OwnerID); err == nil {
				out.PrivateKey = p
			}
		}
	case "BITWARDEN":
		resolveBitwardenAuth(&c)
		if c.authErr != "" {
			return out, fmt.Errorf("%s", c.authErr)
		}
		out.PrivateKey, out.Source = c.PrivateKey, "bitwarden"
	case "KEY_FILE", "KEY_AUTO":
		// The key is a file of the WRM server: name it, never read it for the browser.
		out.KeyPath, out.Source = c.KeyPath, "server_key"
		out.Password = c.Password
	default:
		out.Password, out.PrivateKey = c.Password, c.PrivateKey
	}
	return out, nil
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// POST /api/connections/{id}/secret {password, purpose: show|copy}
func connectionSecretHandler(w http.ResponseWriter, r *http.Request, userID int, cur Connection) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var in struct {
		Password string `json:"password"`
		Purpose  string `json:"purpose"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	purpose := "show"
	if in.Purpose == "copy" {
		purpose = "copy"
	}
	ip := clientIP(r)
	if blocked, wait := loginBlocked(ip); blocked {
		jsonError(w, fmt.Sprintf("Too many failed attempts. Try again in %d s.", int(wait.Seconds())+1), 429)
		return
	}
	u, err := loadUser("id=?", userID)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return
	}
	if userHasPassword(u.ID) {
		if !userCheckPassword(u, in.Password) {
			recordLoginResult(ip, false)
			auditLogRef(r, userID, u.Username, "connection.secret_denied", cur.Name, map[string]interface{}{"id": cur.ID, "reason": "wrong confirmation"}, auditRef{ConnID: cur.ID})
			jsonError(w, "Wrong password", 403)
			return
		}
		recordLoginResult(ip, true)
	} else if !requireStepUpAlways(w, r, "connection_secret") {
		// Directory-only accounts (OIDC) have no password in WRM: they confirm with their provider.
		return
	}
	if !u.IsAdmin && !settingBool("allow_secret_export") {
		jsonError(w, "Showing stored passwords is disabled by the administrator", 403)
		return
	}
	if !requireStepUp(w, r, "connection_secret") {
		return
	}
	sec, err := connectionSecretOf(cur, userID)
	if err != nil {
		jsonError(w, err.Error(), 409)
		return
	}
	auditLogRef(r, userID, u.Username, "connection.secret_revealed", cur.Name, map[string]interface{}{"id": cur.ID, "purpose": purpose, "source": sec.Source}, auditRef{ConnID: cur.ID})
	jsonOK(w, sec)
}

// serverHostname finds the name of the server behind c: its own hostname over SSH, else the
// DNS name of its address (or the host name the user typed, without the domain).
func serverHostname(c Connection) (string, error) {
	switch c.Protocol {
	case "SSH", "SFTP":
		cl, err := dialSSH(c, nil)
		if err != nil {
			return "", err
		}
		defer cl.Close()
		out, err := runRemoteScript(cl, hostnameScript, "", 15*time.Second)
		if err != nil {
			return "", fmt.Errorf("reading the hostname failed: %v", err)
		}
		h := parseHostnameOutput(out)
		if h == "" {
			return "", fmt.Errorf("the server did not return a valid hostname")
		}
		storeHostname(c.ID, h)
		return h, nil
	}
	host := hostOnly(c.Host)
	if net.ParseIP(host) == nil {
		if h := cleanHostname(host); h != "" {
			return strings.SplitN(h, ".", 2)[0], nil
		}
		return "", fmt.Errorf("no hostname found for %s", host)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, host)
	if err != nil || len(names) == 0 {
		return "", fmt.Errorf("no DNS name found for %s", host)
	}
	if h := cleanHostname(strings.TrimSuffix(names[0], ".")); h != "" {
		return strings.SplitN(h, ".", 2)[0], nil
	}
	return "", fmt.Errorf("no DNS name found for %s", host)
}

// POST /api/connections/{id}/name-from-hostname → {name, hostname}
func connectionNameFromHostnameHandler(w http.ResponseWriter, r *http.Request, userID int, id int) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	c, err := loadConnection(id)
	if err != nil || c.UserID != userID {
		jsonError(w, "Not found", 404)
		return
	}
	h, err := serverHostname(c)
	if err != nil {
		if hk := asHostKeyError(err); hk != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": hk.Error(), "hostkey": hk})
			return
		}
		jsonError(w, err.Error(), 502)
		return
	}
	if h != c.Name {
		db.Exec(`UPDATE connections SET name=? WHERE id=? AND user_id=?`, h, id, userID)
		auditLogRef(r, userID, usernameOf(userID), "connection.updated", h, map[string]interface{}{"id": id, "changed": []string{"name"}, "from": c.Name}, auditRef{ConnID: id})
	}
	jsonOK(w, map[string]string{"name": h, "hostname": h})
}
