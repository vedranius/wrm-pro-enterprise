// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ─── QUICK CONNECT & NOTES ───────────────────────────
//
// Quick connect opens user@host:port (or ssh://, rdp://, vnc://, telnet://, https:// …)
// without filling in the connection dialog. WRM keeps it as a temporary connection
// (connections.temp_until): it appears under "Quick connections", works like any other
// connection (jump hosts, vault credentials, recording, audit) and is deleted 24 hours after
// its last use — unless the user saves it.
//
// Notes (connections.notes) are the runbook of a connection: contacts, procedures, links.
// Only the owner sees them (not share members).

const (
	quickTTL         = 24 * time.Hour
	maxQuickPerUser  = 20
	maxNotesLength   = 20000
	quickCleanupTick = 10 * time.Minute
)

func quickUntil() string { return time.Now().UTC().Add(quickTTL).Format(time.RFC3339) }

// POST /api/connections/quick {target, protocol, password, credential_id, key_id, jump_id}
func apiQuickConnectHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var in struct {
		Target       string `json:"target"`
		Protocol     string `json:"protocol"`
		Password     string `json:"password"`
		CredentialID *int   `json:"credential_id"`
		KeyID        *int   `json:"key_id"`
		JumpID       *int   `json:"jump_id"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	target := strings.TrimSpace(in.Target)
	if target == "" {
		jsonError(w, "Enter user@host:port", 400)
		return
	}
	if strings.ContainsAny(target, " \t\r\n") {
		jsonError(w, "One address without spaces, e.g. root@10.0.0.5:22", 400)
		return
	}
	host, user, proto, path := splitHostField(target)
	if p := strings.ToUpper(strings.TrimSpace(in.Protocol)); p != "" && proto == "" {
		proto = p
	}
	if proto == "" {
		proto = "SSH"
	}
	label := hostOnly(host)
	if user != "" {
		label = user + "@" + label
	}
	c := Connection{Name: truncateStr("⚡ "+label, 120), Host: host, Username: user,
		Protocol: proto, AuthMethod: "PASSWORD", Password: in.Password, WebPath: path, JumpID: in.JumpID}
	switch {
	case in.CredentialID != nil && *in.CredentialID > 0:
		c.AuthMethod, c.CredentialID, c.Password = "CREDENTIAL", in.CredentialID, ""
	case in.KeyID != nil && *in.KeyID > 0:
		c.AuthMethod, c.KeyID, c.Password = "KEY_REF", in.KeyID, ""
	}
	if err := normalizeConnection(&c); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if c.Protocol == "SERIAL" {
		jsonError(w, "Serial ports need a saved connection", 400)
		return
	}
	if err := validateJump(userID, 0, c.JumpID); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := checkAuthRefs(&c, userID); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	// Keep at most maxQuickPerUser quick connections: the oldest unused ones go first.
	db.Exec(`DELETE FROM connections WHERE id IN (SELECT id FROM connections WHERE user_id=? AND temp_until<>'' ORDER BY temp_until DESC LIMIT -1 OFFSET ?)`,
		userID, maxQuickPerUser-1)
	plain := c
	plain.UserID = userID
	encryptConnectionSecrets(&c)
	res, err := db.Exec(`INSERT INTO connections (name,protocol,host,username,auth_method,password,private_key,key_path,folder_id,user_id,jump_conn_id,web_path,key_id,credential_id,monitor,temp_until)
		VALUES (?,?,?,?,?,?,'','',NULL,?,?,?,?,?,0,?)`,
		c.Name, c.Protocol, c.Host, c.Username, c.AuthMethod, c.Password, userID, c.JumpID, c.WebPath, c.KeyID, c.CredentialID, quickUntil())
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	id, _ := res.LastInsertId()
	plain.ID = int(id)
	auditLogRef(r, userID, usernameOf(userID), "connection.quick", c.Name, map[string]string{"host": c.Host, "protocol": c.Protocol, "route": jumpPath(plain)}, auditRef{ConnID: plain.ID})
	jsonOK(w, plain.view())
}

// touchQuick extends the life of a quick connection while it is used.
func touchQuick(connID int) {
	db.Exec(`UPDATE connections SET temp_until=? WHERE id=? AND temp_until<>''`, quickUntil(), connID)
}

// cleanupQuickConnections deletes expired quick connections without open terminals.
func cleanupQuickConnections() {
	rows, err := db.Query(`SELECT id FROM connections WHERE temp_until<>'' AND temp_until<?`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		busy := false
		terms.Lock()
		for _, t := range terms.m {
			busy = busy || t.ConnID == id
		}
		terms.Unlock()
		if busy {
			touchQuick(id)
			continue
		}
		db.Exec(`DELETE FROM ssh_key_deployments WHERE conn_id=?`, id)
		db.Exec(`UPDATE connections SET jump_conn_id=NULL WHERE jump_conn_id=?`, id)
		db.Exec(`UPDATE folders SET jump_conn_id=NULL WHERE jump_conn_id=?`, id)
		db.Exec(`DELETE FROM connections WHERE id=? AND temp_until<>''`, id)
	}
	if len(ids) > 0 {
		log.Printf("Quick connect: removed %d expired quick connection(s)", len(ids))
	}
}

func runQuickCleanup() {
	for {
		cleanupQuickConnections()
		time.Sleep(quickCleanupTick)
	}
}

func loadNotes(connID int) string {
	var n string
	db.QueryRow(`SELECT COALESCE(notes,'') FROM connections WHERE id=?`, connID).Scan(&n)
	return n
}

func validNotes(n string) error {
	if len(n) > maxNotesLength {
		return fmt.Errorf("notes are limited to %d characters", maxNotesLength)
	}
	return nil
}
