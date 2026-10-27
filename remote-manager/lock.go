// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
)

// ─── SCREEN LOCK (#115) ──────────────────────────────
//
// The lock detaches the interface from the screen after a period without
// activity. It is a presence check, not a second session: the cookie and the
// running terminals, tunnels and AI sessions stay alive, and every request
// still passes the normal authentication and CSRF checks. Unlocking asks for
// the signed-in account's password; the attempt is audited either way.
//
// v13.7: the lock is remembered per sign-in session (auth_sessions.locked_at), so reloading
// the page or opening a new tab shows the lock screen again. screen_lock_mode decides how
// strict it is:
//   - browser:  only the page covers itself (the behaviour before v13.7);
//   - remember: the server remembers the lock, but the API keeps answering;
//   - server:   while locked, this sign-in session can only reach the endpoints in
//               screenLockAllowedPaths (423 Locked otherwise). Terminals, tunnels, AI runs
//               and MCP connections that are already running keep going.
// Unlocking accepts the account password (local or directory) or, when allowed, a personal
// PIN. screen_lock_max_failures signs the session out after that many wrong attempts.

var lockAttempts = struct {
	sync.Mutex
	m map[int][]time.Time
}{m: map[int][]time.Time{}}

// lockThrottled allows five attempts per minute per account.
func lockThrottled(userID int) bool {
	lockAttempts.Lock()
	defer lockAttempts.Unlock()
	now := time.Now()
	keep := []time.Time{}
	for _, t := range lockAttempts.m[userID] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= 5 {
		lockAttempts.m[userID] = keep
		return true
	}
	lockAttempts.m[userID] = append(keep, now)
	return false
}

func screenLockMode() string { return getSetting("screen_lock_mode") }

var screenLockAllowedPaths = map[string]bool{
	"/api/auth/me": true, "/api/auth/logout": true, "/api/auth/keepalive": true,
	"/api/auth/lock": true, "/api/auth/lock/verify": true, "/api/me/ui/lock": true, "/api/me/ui/appearance": true,
}

// screenLockRefuses: this request comes from a locked sign-in session and must wait for the unlock.
func screenLockRefuses(s *authSession, r *http.Request) bool {
	if s == nil || s.LockedAt == "" || screenLockAllowedPaths[r.URL.Path] {
		return false
	}
	return screenLockMode() == "server"
}

// screenLockPolicy is what the page needs to know to lock itself the way the administrator wants.
func screenLockPolicy(userID int) map[string]interface{} {
	var pin string
	db.QueryRow(`SELECT COALESCE(lock_pin_hash,'') FROM users WHERE id=?`, userID).Scan(&pin)
	return map[string]interface{}{
		"mode": screenLockMode(), "max_minutes": settingInt("screen_lock_max_minutes"),
		"pin_allowed": settingBool("screen_lock_pin"), "has_pin": pin != "" && settingBool("screen_lock_pin"),
		"max_failures": settingInt("screen_lock_max_failures"),
	}
}

var screenLockReasons = map[string]bool{"manual": true, "idle": true, "hidden": true, "os_locked": true, "sleep": true, "start": true, "shortcut": true, "other_tab": true}

// GET  /api/auth/lock            → {"locked": bool, "mode": "..."}
// POST /api/auth/lock {"reason"} → locks this sign-in session (the page covers itself too)
func apiAuthLockHandler(w http.ResponseWriter, r *http.Request) {
	s, err := lookupSession(r)
	if err != nil || s.Restricted != "" {
		jsonError(w, "Unauthorized", 401)
		return
	}
	mode := screenLockMode()
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, map[string]interface{}{"locked": s.LockedAt != "" && mode != "browser", "mode": mode})
	case http.MethodPost:
		var p struct {
			Reason string `json:"reason"`
		}
		json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&p)
		reason := "manual"
		if screenLockReasons[p.Reason] {
			reason = p.Reason
		}
		if s.LockedAt == "" {
			if mode != "browser" {
				db.Exec(`UPDATE auth_sessions SET locked_at=?, lock_failures=0 WHERE id=?`, nowRFC(), s.ID)
			}
			auditLog(r, s.UserID, "auth.locked", "", map[string]string{"reason": reason, "mode": mode})
		}
		jsonOK(w, map[string]interface{}{"locked": true, "mode": mode})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// POST /api/auth/lock/verify {"password": "..."} or {"pin": "..."}
func apiAuthLockVerifyHandler(w http.ResponseWriter, r *http.Request) {
	s, err := lookupSession(r)
	if err != nil || s.Restricted != "" {
		jsonError(w, "Unauthorized", 401)
		return
	}
	userID := s.UserID
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var p struct {
		Password string `json:"password"`
		PIN      string `json:"pin"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&p) != nil || (p.Password == "" && p.PIN == "") {
		jsonError(w, "Enter your password", 400)
		return
	}
	if lockThrottled(userID) {
		jsonError(w, "Too many attempts; wait a minute", 429)
		return
	}
	u, err := loadUser("id=? AND disabled=0", userID)
	if err != nil {
		jsonError(w, "Account not found", 403)
		return
	}
	method, ok := "account", false // audit value: "account" (sign-in secret) or "pin"
	if p.PIN != "" {
		method = "pin"
		var pinHash string
		db.QueryRow(`SELECT COALESCE(lock_pin_hash,'') FROM users WHERE id=?`, userID).Scan(&pinHash)
		ok = settingBool("screen_lock_pin") && pinHash != "" && checkPassword(pinHash, p.PIN)
	} else {
		ok = userCheckPassword(u, p.Password)
	}
	if !ok {
		var fails int
		db.Exec(`UPDATE auth_sessions SET lock_failures=lock_failures+1 WHERE id=?`, s.ID)
		db.QueryRow(`SELECT lock_failures FROM auth_sessions WHERE id=?`, s.ID).Scan(&fails)
		auditLog(r, userID, "auth.unlock_failed", "", map[string]string{"method": method})
		if max := settingInt("screen_lock_max_failures"); max > 0 && fails >= max {
			db.Exec(`DELETE FROM auth_sessions WHERE id=?`, s.ID)
			clearAuthCookie(w, r)
			auditLog(r, userID, "auth.lock_signed_out", "", map[string]int{"failures": fails})
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "Too many wrong attempts; you were signed out", "signed_out": true})
			return
		}
		if method == "pin" {
			jsonError(w, "The PIN is not correct", 403)
		} else {
			jsonError(w, "The password is not correct", 403)
		}
		return
	}
	db.Exec(`UPDATE auth_sessions SET locked_at='', lock_failures=0 WHERE id=?`, s.ID)
	auditLog(r, userID, "auth.unlocked", "", map[string]string{"method": method})
	jsonOK(w, map[string]bool{"ok": true})
}

// validLockPIN: 4 to 12 digits.
func validLockPIN(pin string) bool {
	if len(pin) < 4 || len(pin) > 12 {
		return false
	}
	for _, c := range pin {
		if !unicode.IsDigit(c) || c > '9' {
			return false
		}
	}
	return true
}

// PUT    /api/auth/lock/pin {"pin": "...", "password": "..."} sets the unlock PIN
// DELETE /api/auth/lock/pin                                    removes it
// Setting a PIN needs the account password, or a fresh sign-in/step-up for accounts that only
// sign in through a directory provider.
func apiAuthLockPinHandler(w http.ResponseWriter, r *http.Request) {
	s, err := lookupSession(r)
	if err != nil || s.Restricted != "" {
		jsonError(w, "Unauthorized", 401)
		return
	}
	if screenLockRefuses(s, r) {
		w.WriteHeader(http.StatusLocked)
		return
	}
	switch r.Method {
	case http.MethodPut:
		if !settingBool("screen_lock_pin") {
			jsonError(w, "The administrator turned unlocking with a PIN off", 403)
			return
		}
		var p struct {
			PIN      string `json:"pin"`
			Password string `json:"password"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&p) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		p.PIN = strings.TrimSpace(p.PIN)
		if !validLockPIN(p.PIN) {
			jsonError(w, "The PIN must have 4 to 12 digits", 400)
			return
		}
		u, err := loadUser("id=?", s.UserID)
		if err != nil {
			jsonError(w, "Unauthorized", 401)
			return
		}
		if userHasPassword(u.ID) {
			if lockThrottled(u.ID) {
				jsonError(w, "Too many attempts; wait a minute", 429)
				return
			}
			if !userCheckPassword(u, p.Password) {
				auditLog(r, u.ID, "auth.lock_pin_failed", "", nil)
				jsonError(w, "The password is not correct", 403)
				return
			}
		} else if !stepUpFresh(s.ID) {
			jsonError(w, "Sign in again, then set the PIN within a few minutes", 403)
			return
		}
		h, err := hashPassword(p.PIN)
		if err != nil {
			jsonError(w, "Server error", 500)
			return
		}
		db.Exec(`UPDATE users SET lock_pin_hash=? WHERE id=?`, h, u.ID)
		auditLog(r, u.ID, "auth.lock_pin_set", "", nil)
		jsonOK(w, map[string]bool{"ok": true})
	case http.MethodDelete:
		db.Exec(`UPDATE users SET lock_pin_hash='' WHERE id=?`, s.UserID)
		auditLog(r, s.UserID, "auth.lock_pin_removed", "", nil)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}
