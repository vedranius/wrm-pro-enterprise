// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// POST /api/connections/test
// Body: a Connection (as in the edit form). When "id" is set and password/key fields are
// empty, the stored secrets of that connection are used.
func apiConnectionTestHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if isAuditorUser(userID) {
		jsonError(w, "Security auditors cannot open connections", 403)
		return
	}
	var c Connection
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	if c.ID > 0 && userOwnsConnection(c.ID, userID) {
		if stored, err := loadConnectionRaw(c.ID); err == nil {
			if c.Password == "" {
				c.Password = stored.Password
			}
			if c.PrivateKey == "" {
				c.PrivateKey = stored.PrivateKey
			}
		}
	}
	c.Host = strings.TrimSpace(c.Host)
	if c.Host == "" {
		jsonError(w, "Host is required", 400)
		return
	}
	if c.Name == "" {
		c.Name = c.Host
	}
	if err := normalizeConnection(&c); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := checkAuthRefs(&c, userID); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	c.UserID = userID
	if c.FolderID != nil && !userOwnsFolder(*c.FolderID, userID) {
		c.FolderID = nil
	}
	ownID := 0
	if c.ID > 0 && userOwnsConnection(c.ID, userID) {
		ownID = c.ID
	}
	// The proxy (own or folder default) must be one the user may use: "proxy not found"
	// otherwise, without its name.
	if err := validateRoute(userID, ownID, &c); err != nil {
		jsonOK(w, map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	applyFolderDefaults(&c) // test the route the connection would use
	c.Host = ensurePort(c.Host, c.Protocol)
	resolveConnectionAuth(&c)
	if c.authErr != "" {
		jsonOK(w, map[string]interface{}{"ok": false, "error": c.authErr})
		return
	}

	start := time.Now()
	var detail string
	var err error
	if isFTP(c) {
		fc, done, e := dialFTP(c)
		if e == nil {
			if wd, e2 := fc.CurrentDir(); e2 == nil {
				detail = "FTP login OK, working directory " + wd
			}
			done()
		}
		err = e
	} else if c.Protocol == "SERIAL" {
		if !serialAllowed(userID) {
			err = fmt.Errorf("serial ports of the WRM server are not allowed for this account")
		} else if tio, e := openSerialPath(serialDevDir+hostOnly(c.Host), serialOptionsFrom(c.Options)); e != nil {
			err = e
		} else {
			detail = tio.banner + " Opened."
			tio.close()
		}
	} else {
		if isWeb(c) || isDesktopProtocol(c.Protocol) {
			// Web interfaces and remote desktops: check that the port answers (through the jump hosts, if any).
			err = testWebReachable(c)
			if err == nil {
				detail = "Port " + c.Host + " is reachable"
				if route := jumpPath(c); route != "" {
					detail += " via " + route
				}
				if isDesktopProtocol(c.Protocol) {
					if v, gerr := guacdCheck(); gerr != nil {
						detail += " · ⚠ guacd (remote desktop proxy) is not reachable at " + getSetting("guacd_address")
					} else {
						detail += " · guacd " + v + " ready"
					}
				}
			}
		} else {
			if err = validateJump(userID, c.ID, c.JumpID); err == nil {
				newKeys := []string{}
				var client *ssh.Client
				client, err = dialSSH(c, func(host, fp string) { newKeys = append(newKeys, host+" "+fp) })
				if err == nil {
					detail = "SSH login OK (" + string(client.ServerVersion()) + ")"
					if route := jumpPath(c); route != "" {
						detail += " via " + route
					}
					if len(newKeys) > 0 {
						detail += " · new host key saved: " + strings.Join(newKeys, ", ")
					}
					client.Close()
				}
			}
		}
	}
	ms := time.Since(start).Milliseconds()
	if err != nil {
		out := map[string]interface{}{"ok": false, "message": err.Error(), "latency_ms": ms}
		if hk := asHostKeyError(err); hk != nil {
			out["hostkey"] = hk
			out["message"] = hk.Error()
		}
		jsonOK(w, out)
		return
	}
	jsonOK(w, map[string]interface{}{"ok": true, "message": detail, "latency_ms": ms})
}
