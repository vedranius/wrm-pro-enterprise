// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

// Lockdown and Stop user (v12.6.26).
//
// Lockdown is the emergency brake for the whole installation: while it is on, only break-glass
// administrators can sign in or keep a session, the AI kill switch is in force (AI sessions
// and AI connections end and cannot start), shares and guest links are paused, and the
// terminals, tunnels and rooms of everyone else end. It is turned on and off
//
//   - in Admin → Security (step-up, with a reason; off only by a break-glass administrator,
//     the only accounts that can still sign in),
//   - with "wrm -lockdown on|off [-lockdown-reason …]" on the WRM host, also while WRM runs
//     (the running server reads the state from the database every few seconds), and
//   - with the environment variable WRM_LOCKDOWN=1, which forces it on until it is removed.
//
// Stop user ends everything one account has open and disables it, in one step, and says what
// was ended.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const lockdownEnv = "WRM_LOCKDOWN"

type lockdownState struct {
	On     bool   `json:"on"`
	Reason string `json:"reason,omitempty"`
	By     string `json:"by,omitempty"`
	At     string `json:"at,omitempty"`
	Source string `json:"source,omitempty"` // ui | cli | env
}

var lockdownCache = struct {
	sync.RWMutex
	st     lockdownState
	loaded bool
}{}

func lockdownFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(lockdownEnv))) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

func readLockdownState() lockdownState {
	var st lockdownState
	if raw := getInternal("lockdown"); raw != "" {
		json.Unmarshal([]byte(raw), &st)
	}
	return st
}

// lockdown returns the effective state: the environment variable wins.
func lockdown() lockdownState {
	if lockdownFromEnv() {
		return lockdownState{On: true, Reason: "set by the environment variable " + lockdownEnv, Source: "env"}
	}
	lockdownCache.RLock()
	st, loaded := lockdownCache.st, lockdownCache.loaded
	lockdownCache.RUnlock()
	if !loaded {
		st = readLockdownState()
		lockdownCache.Lock()
		lockdownCache.st, lockdownCache.loaded = st, true
		lockdownCache.Unlock()
	}
	return st
}

func lockdownActive() bool { return lockdown().On }

// lockdownRefuses: during a lockdown only break-glass administrators may use WRM.
func lockdownRefuses(userID int) bool {
	return lockdownActive() && !userIsBreakGlass(userID)
}

func userIsBreakGlass(userID int) bool {
	if userID <= 0 {
		return false
	}
	var bg, admin, disabled int
	if db.QueryRow(`SELECT COALESCE(break_glass,0), is_admin, disabled FROM users WHERE id=?`, userID).Scan(&bg, &admin, &disabled) != nil {
		return false
	}
	return bg == 1 && admin == 1 && disabled == 0
}

const lockdownMessage = "WRM is in lockdown: only break-glass administrators can sign in. Ask your security team."

func breakGlassAdminCount() int {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE break_glass=1 AND is_admin=1 AND disabled=0`).Scan(&n)
	return n
}

// setLockdown stores the new state; enforcement follows from lockdownApply.
func setLockdown(on bool, reason, by, source string) (lockdownState, error) {
	st := lockdownState{On: on, Reason: truncateStr(strings.TrimSpace(reason), 300), By: by, At: nowRFC(), Source: source}
	b, _ := json.Marshal(st)
	if err := setInternal("lockdown", string(b)); err != nil {
		return st, err
	}
	return st, nil
}

// lockdownApply makes a state that just turned on take effect: everyone but the break-glass
// administrators loses their sessions, terminals, rooms and tunnels; all AI sessions and AI
// connections end. It returns what was ended.
func lockdownApply(r *http.Request, byID int, reason string) map[string]int {
	out := map[string]int{"sessions_ended": 0, "rooms_left": 0}
	msg := "WRM is in lockdown"
	if reason != "" {
		msg += ": " + reason
	}
	out["medium_jobs_stopped"] = mdCancelUserJobs(0, msg)
	rows, err := db.Query(`SELECT DISTINCT user_id FROM auth_sessions`)
	var ids []int
	if err == nil {
		for rows.Next() {
			var id int
			rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
	}
	for _, id := range ids {
		if userIsBreakGlass(id) {
			continue
		}
		out["sessions_ended"] += int(revokeUserSessions(id, 0))
	}
	out["terminals_ended"] = killTerminals(func(t *termSession) bool { return !userIsBreakGlass(t.UserID) }, msg)
	rooms.Lock()
	var rms []*room
	for _, rm := range rooms.m {
		rms = append(rms, rm)
	}
	rooms.Unlock()
	seen := map[int]bool{}
	for _, rm := range rms {
		rm.mu.Lock()
		for _, p := range rm.parts {
			if !userIsBreakGlass(p.sa.UserID) {
				seen[p.sa.UserID] = true
			}
		}
		rm.mu.Unlock()
	}
	for uid := range seen {
		out["rooms_left"] += kickUserFromRooms(uid, msg)
	}
	out["tunnels_stopped"] = tunnelMgr.stopWhere(func(t *tunnelRun) bool { return !userIsBreakGlass(t.ownerID) }, msg)
	out["ai_sessions_ended"] = aiKillAll(byID, "lockdown")
	out["ai_connections_ended"] = mcpShutdown(byID, "lockdown", nil)
	return out
}

// lockdownWatchLoop notices a lockdown set by "wrm -lockdown" (or by another instance on the
// same database) and applies it.
func lockdownWatchLoop() {
	wasOn := lockdownActive()
	if wasOn {
		res := lockdownApply(nil, 0, lockdown().Reason)
		auditLogAs(nil, 0, "system", "security.lockdown_enforced", "", map[string]interface{}{"at_start": true, "ended": res, "source": lockdown().Source})
	}
	for range time.Tick(5 * time.Second) {
		st := readLockdownState()
		lockdownCache.Lock()
		lockdownCache.st, lockdownCache.loaded = st, true
		lockdownCache.Unlock()
		now := lockdownActive()
		if now && !wasOn {
			eff := lockdown()
			res := lockdownApply(nil, 0, eff.Reason)
			auditLogAs(nil, 0, nonEmpty(eff.By, "system"), "security.lockdown_enforced", "", map[string]interface{}{"source": eff.Source, "ended": res})
		}
		wasOn = now
	}
}

func lockdownJSON() map[string]interface{} {
	st := lockdown()
	return map[string]interface{}{"on": st.On, "reason": st.Reason, "by": st.By, "at": st.At, "source": st.Source,
		"env": lockdownFromEnv(), "break_glass_admins": breakGlassAdminCount()}
}

// GET/POST /api/admin/lockdown {on, reason}
func apiAdminLockdownHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, lockdownJSON())
	case http.MethodPost:
		var in struct {
			On     bool   `json:"on"`
			Reason string `json:"reason"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&in) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if lockdownFromEnv() {
			jsonError(w, "Lockdown is forced by the environment variable "+lockdownEnv+"; remove it and restart WRM to end it", 409)
			return
		}
		if !requireStepUpAlways(w, r, "security.lockdown") {
			return
		}
		u, _ := loadUser("id=?", adminID)
		name := ""
		if u != nil {
			name = u.Username
		}
		if in.On {
			if strings.TrimSpace(in.Reason) == "" {
				jsonError(w, "Write why WRM goes into lockdown; everyone sees it", 400)
				return
			}
			if breakGlassAdminCount() == 0 {
				jsonError(w, "There is no break-glass administrator: nobody could sign in during the lockdown. Mark an administrator as break-glass first (Admin panel → Users).", 409)
				return
			}
			st, err := setLockdown(true, in.Reason, name, "ui")
			if err != nil {
				jsonError(w, "Server error", 500)
				return
			}
			lockdownCache.Lock()
			lockdownCache.st, lockdownCache.loaded = st, true
			lockdownCache.Unlock()
			res := lockdownApply(r, adminID, st.Reason)
			auditLog(r, adminID, "security.lockdown_on", "", map[string]interface{}{"reason": st.Reason, "ended": res})
			out := lockdownJSON()
			out["ended"] = res
			out["you_signed_out"] = u != nil && !u.BreakGlass
			jsonOK(w, out)
			return
		}
		if u == nil || !u.BreakGlass {
			jsonError(w, "Only a break-glass administrator can end the lockdown (or wrm -lockdown off on the WRM host)", 403)
			return
		}
		st, err := setLockdown(false, in.Reason, name, "ui")
		if err != nil {
			jsonError(w, "Server error", 500)
			return
		}
		lockdownCache.Lock()
		lockdownCache.st, lockdownCache.loaded = st, true
		lockdownCache.Unlock()
		auditLog(r, adminID, "security.lockdown_off", "", map[string]interface{}{"reason": st.Reason})
		jsonOK(w, lockdownJSON())
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// lockdownCLI implements "wrm -lockdown on|off".
func lockdownCLI(mode, reason string) error {
	var on bool
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "on", "1", "true":
		on = true
	case "off", "0", "false":
	case "status":
		st := readLockdownState()
		if lockdownFromEnv() {
			fmt.Printf("Lockdown: on (forced by %s)\n", lockdownEnv)
			return nil
		}
		if st.On {
			fmt.Printf("Lockdown: on since %s by %s: %s\n", st.At, nonEmpty(st.By, "?"), st.Reason)
		} else {
			fmt.Println("Lockdown: off")
		}
		return nil
	default:
		return errors.New("use -lockdown on, off or status")
	}
	if on && strings.TrimSpace(reason) == "" {
		reason = "turned on from the command line"
	}
	if on && breakGlassAdminCount() == 0 {
		log.Printf("WARNING: there is no break-glass administrator; nobody can sign in until the lockdown ends (wrm -lockdown off)")
	}
	if _, err := setLockdown(on, reason, "cli", "cli"); err != nil {
		return err
	}
	action := map[bool]string{true: "security.lockdown_on", false: "security.lockdown_off"}[on]
	auditLogAs(nil, 0, "cli", action, "", map[string]interface{}{"reason": reason, "cli": true})
	if on {
		fmt.Println("Lockdown is on. A running WRM applies it within a few seconds: only break-glass administrators can sign in, AI is stopped, shares are paused.")
	} else {
		fmt.Println("Lockdown is off.")
		if lockdownFromEnv() {
			fmt.Printf("Note: %s is set, so WRM started with it stays in lockdown.\n", lockdownEnv)
		}
	}
	return nil
}

// POST /api/admin/users/{id}/stop {reason}: end everything the account has open and disable it.
func stopUser(w http.ResponseWriter, r *http.Request, adminID int, target *userRow) {
	var in struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&in)
	}
	if target.ID == adminID {
		jsonError(w, "You cannot stop your own account", 400)
		return
	}
	if target.IsAdmin && !target.Disabled {
		var admins int
		db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin=1 AND disabled=0`).Scan(&admins)
		if admins <= 1 {
			jsonError(w, "The last active administrator cannot be stopped", 400)
			return
		}
	}
	if !requireStepUp(w, r, "admin.user_stop") {
		return
	}
	reason := truncateStr(strings.TrimSpace(in.Reason), 300)
	db.Exec(`UPDATE users SET disabled=1 WHERE id=?`, target.ID)
	why := "the account was stopped by an administrator"
	if reason != "" {
		why += ": " + reason
	}
	ended := endUserAccess(r, target.ID, adminID, why)
	d := map[string]interface{}{"reason": reason}
	for k, v := range ended {
		d[k] = v
	}
	auditLog(r, adminID, "admin.user_stopped", target.Username, d)
	ended["disabled"] = true
	jsonOK(w, map[string]interface{}{"ok": true, "user": target.Username, "ended": ended})
}
