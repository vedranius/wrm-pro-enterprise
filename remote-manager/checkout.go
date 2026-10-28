// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ─── FOUR-EYES CHECKOUT (RFC 0005) ──────────────────────────────────
// A credential marked dual_control cannot be revealed by its owner alone: the owner requests
// a checkout and a second, different user (another administrator or a grantee with the right)
// approves it. The approved checkout is single-use. See docs/rfcs/0005-checkout-four-eyes.md.

func migrateCheckouts() {
	ensureTable("credential_checkouts", `CREATE TABLE IF NOT EXISTS credential_checkouts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		credential_id INTEGER NOT NULL,
		requester_id INTEGER NOT NULL,
		approver_id INTEGER DEFAULT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at TEXT NOT NULL DEFAULT '',
		decided_at TEXT NOT NULL DEFAULT '')`,
		"id", "credential_id", "requester_id", "approver_id", "status", "created_at", "decided_at")
}

// requestCheckout opens a pending checkout for a dual-control credential (caller re-authenticated).
func requestCheckout(credID, requesterID int) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO credential_checkouts (credential_id, requester_id, status, created_at) VALUES (?,?, 'pending', ?)`,
		credID, requesterID, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

// approveCheckout decides a pending checkout. approverID must differ from the requester, and
// the approver must be an administrator or own the credential.
func approveCheckout(checkoutID, approverID int, approve bool) (bool, error) {
	var credID, requesterID int
	var status string
	if err := db.QueryRow(`SELECT credential_id, requester_id, status FROM credential_checkouts WHERE id=?`, checkoutID).
		Scan(&credID, &requesterID, &status); err != nil {
		return false, err
	}
	if status != "pending" {
		return false, nil
	}
	if requesterID == approverID {
		return false, nil // four-eyes: you cannot approve your own checkout
	}
	cr, err := loadCredential(credID)
	if err != nil {
		return false, err
	}
	u, err := loadUser("id=?", approverID)
	if err != nil || (!u.IsAdmin && cr.OwnerID != approverID) {
		return false, nil // not authorized to approve
	}
	decision := "denied"
	if approve {
		decision = "approved"
	}
	_, err = db.Exec(`UPDATE credential_checkouts SET status=?, approver_id=?, decided_at=? WHERE id=?`,
		decision, approverID, time.Now().UTC().Format(time.RFC3339), checkoutID)
	return err == nil, err
}

// approvedCheckout returns the id of an approved, not-yet-used checkout for the credential,
// or 0.
func approvedCheckout(credID, userID int) int {
	var id int
	if err := db.QueryRow(`SELECT id FROM credential_checkouts WHERE credential_id=? AND requester_id=? AND status='approved' ORDER BY id DESC LIMIT 1`,
		credID, userID).Scan(&id); err != nil {
		return 0
	}
	return id
}

func consumeCheckout(id int) {
	db.Exec(`UPDATE credential_checkouts SET status='used' WHERE id=?`, id)
}

// POST /api/credentials/{id}/checkout — the credential owner requests a checkout.
func requestCheckoutHandler(w http.ResponseWriter, r *http.Request, userID int, cr credential) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if !requireStepUp(w, r, "credential_checkout") {
		return
	}
	id, err := requestCheckout(cr.ID, userID)
	if err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	auditLog(r, userID, "credential.checkout_requested", cr.Name, map[string]interface{}{"cred_id": cr.ID, "checkout_id": id})
	jsonOK(w, map[string]interface{}{"checkout_id": id})
}

// GET /api/credentials/checkouts — pending checkouts this user may approve.
// POST /api/credentials/checkouts/{id} {approve: bool} — decide.
func apiCheckoutsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/credentials/checkouts")
	if rest == "" && r.Method == http.MethodGet {
		rows, err := db.Query(`SELECT c.id, c.credential_id, cr.name, c.requester_id, COALESCE(u.username,''), c.created_at
			FROM credential_checkouts c
			JOIN credentials cr ON cr.id=c.credential_id
			LEFT JOIN users u ON u.id=c.requester_id
			WHERE c.status='pending' ORDER BY c.id DESC LIMIT 200`)
		if err != nil {
			jsonError(w, "Server error", 500)
			return
		}
		defer rows.Close()
		out := []map[string]interface{}{}
		for rows.Next() {
			var id, credID, reqID int
			var name, reqName, created string
			rows.Scan(&id, &credID, &name, &reqID, &reqName, &created)
			out = append(out, map[string]interface{}{"id": id, "credential_id": credID, "credential": name, "requester": reqName, "created_at": created})
		}
		jsonOK(w, out)
		return
	}
	if rest != "" && r.Method == http.MethodPost {
		id, err := strconv.Atoi(strings.TrimPrefix(rest, "/"))
		if err != nil {
			jsonError(w, "Bad ID", 400)
			return
		}
		var in struct {
			Approve bool `json:"approve"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		ok, err := approveCheckout(id, userID, in.Approve)
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		if !ok {
			jsonError(w, "You cannot approve this checkout", 403)
			return
		}
		action := "credential.checkout_denied"
		if in.Approve {
			action = "credential.checkout_approved"
		}
		auditLog(r, userID, action, "", map[string]interface{}{"checkout_id": id})
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	jsonError(w, "Method not allowed", 405)
}
