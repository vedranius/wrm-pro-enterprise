// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var startedAt = time.Now()

// ─── ACTIVE TERMINAL REGISTRY ────────────────────────
// Every open SSH terminal is registered so administrators can see and end it, and so
// access can be revoked instantly (user disabled, share removed, role downgraded).

type termSession struct {
	ID       string    `json:"id"`
	UserID   int       `json:"user_id"`
	User     string    `json:"user"`
	ShareID  int       `json:"share_id,omitempty"`
	Share    string    `json:"share,omitempty"`
	PKey     string    `json:"-"`
	Ctx      accessCtx `json:"-"`
	ConnID   int       `json:"conn_id"`
	ConnName string    `json:"connection"`
	Host     string    `json:"host"`
	IP       string    `json:"ip"`
	Started  time.Time `json:"started"`
	kill     func(reason string)
}

var terms = struct {
	sync.Mutex
	m map[string]*termSession
}{m: map[string]*termSession{}}

func registerTerminal(t *termSession) {
	terms.Lock()
	terms.m[t.ID] = t
	terms.Unlock()
}

func unregisterTerminal(id string) {
	terms.Lock()
	delete(terms.m, id)
	terms.Unlock()
}

// killTerminals ends every terminal matching filter and returns how many were ended.
func killTerminals(filter func(*termSession) bool, reason string) int {
	terms.Lock()
	var list []*termSession
	for _, t := range terms.m {
		if filter(t) {
			list = append(list, t)
		}
	}
	terms.Unlock()
	for _, t := range list {
		t.kill(reason)
	}
	return len(list)
}

// wsCloseRevoked tells the browser not to reconnect automatically.
const wsCloseRevoked = 4003

func closeWSRevoked(ws *websocket.Conn, reason string) {
	ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(wsCloseRevoked, truncateStr(reason, 120)), time.Now().Add(2*time.Second))
}

func userOnline(userID int) bool {
	terms.Lock()
	for _, t := range terms.m {
		if t.UserID == userID {
			terms.Unlock()
			return true
		}
	}
	terms.Unlock()
	if userInRooms(userID) {
		return true
	}
	var n int
	db.QueryRow(`SELECT COUNT(1) FROM auth_sessions WHERE user_id=? AND last_active_at > ?`, userID, time.Now().Add(-3*time.Minute).UTC().Format(time.RFC3339)).Scan(&n)
	return n > 0
}

// ─── STATUS ──────────────────────────────────────────

// GET /api/admin/status
func apiAdminStatusHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}
	terms.Lock()
	tl := []*termSession{}
	for _, t := range terms.m {
		tl = append(tl, t)
	}
	terms.Unlock()
	var users, admins, with2FA, shares, conns int
	db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(is_admin),0), COALESCE(SUM(totp_enabled),0) FROM users WHERE disabled=0`).Scan(&users, &admins, &with2FA)
	db.QueryRow(`SELECT COUNT(*) FROM share_links`).Scan(&shares)
	db.QueryRow(`SELECT COUNT(*) FROM connections`).Scan(&conns)
	var dbSize int64
	if st, err := os.Stat(resolveDBPath()); err == nil {
		dbSize = st.Size()
	}
	warnings := []string{}
	if !isHTTPS(r) && !strings.HasPrefix(hostOnly(r.Host), "127.") && hostOnly(r.Host) != "localhost" {
		warnings = append(warnings, "no_https")
	}
	if getSetting("registration") == "open" {
		warnings = append(warnings, "registration_open")
	}
	if getSetting("host_key_policy") == "off" {
		warnings = append(warnings, "host_keys_off")
	}
	if getSetting("require_2fa") == "off" && with2FA < admins {
		warnings = append(warnings, "admins_without_2fa")
	}
	if os.Getenv("WRM_ALLOW_ANY_ORIGIN") == "1" {
		warnings = append(warnings, "any_origin")
	}
	vault := mergeCounts(mergeCounts(keysSummary(), credentialsSummary()), proxiesSummary())
	if vault["rotation_incomplete"] > 0 {
		warnings = append(warnings, "credential_rotation_incomplete")
	}
	if vault["rotation_due"] > 0 {
		warnings = append(warnings, "credential_rotation_due")
	}
	jsonOK(w, map[string]interface{}{
		"version": AppVersion, "go": runtime.Version(), "os": runtime.GOOS + "/" + runtime.GOARCH,
		"uptime_s": int(time.Since(startedAt).Seconds()), "db_path": resolveDBPath(), "db_size": dbSize,
		"encryption_key": encryptionKeySource, "https": isHTTPS(r), "trust_proxy": trustProxy,
		"users": users, "admins": admins, "users_2fa": with2FA, "shares": shares, "connections": conns,
		"terminals": tl, "rooms": roomsSummary(), "turn": turnStatus(), "warnings": warnings, "security_level": securityLevel(),
		"tunnels": tunnelMgr.runsWhere(func(*tunnelRun) bool { return true }),
		"guacd":   guacdStatus(),
		"vault":   vault,
		"notify":  notifySummary(),
		"ipmitool": func() string {
			if p, err := exec.LookPath(ipmitoolPath); err == nil {
				return p
			}
			return ""
		}(),
	})
}

// POST /api/admin/terminals/{id}/kill
func apiAdminTerminalsHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	id := strings.TrimSuffix(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/terminals/"), "/"), "/kill")
	var target *termSession
	terms.Lock()
	target = terms.m[id]
	terms.Unlock()
	if target == nil {
		jsonError(w, "Terminal not found", 404)
		return
	}
	target.kill("The session was ended by an administrator")
	auditLog(r, adminID, "admin.terminal_killed", target.ConnName, map[string]interface{}{"user": target.User, "host": target.Host})
	jsonOK(w, map[string]bool{"ok": true})
}

// guacdStatus reports the remote desktop proxy for the admin overview.
func guacdStatus() map[string]interface{} {
	st := map[string]interface{}{"enabled": settingBool("desktop_enabled"), "address": getSetting("guacd_address")}
	for k, v := range guacdManagedStatus() {
		st[k] = v
	}
	if !settingBool("desktop_enabled") {
		return st
	}
	if v, err := guacdCheck(); err != nil {
		st["ok"], st["error"] = false, truncateStr(err.Error(), 200)
	} else {
		st["ok"], st["version"] = true, v
	}
	return st
}

// POST /api/admin/guacd — start the managed guacd container now.
func apiAdminGuacdHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if !settingBool("desktop_enabled") {
		jsonError(w, "Remote desktop is turned off.", 400)
		return
	}
	if _, err := guacdCheck(); err == nil {
		jsonOK(w, map[string]interface{}{"started": false, "guacd": guacdStatus()})
		return
	}
	if !guacdAutoEnabled() {
		jsonError(w, "Automatic guacd setup is available on Windows and macOS when guacd_auto is on and guacd_address is local (127.0.0.1:4822).", 400)
		return
	}
	started := guacdStartAsync()
	auditLog(r, adminID, "admin.guacd_setup", guacdManagedContainer, map[string]interface{}{"address": getSetting("guacd_address")})
	jsonOK(w, map[string]interface{}{"started": started, "guacd": guacdStatus()})
}

func mergeCounts(maps ...map[string]int) map[string]int {
	out := map[string]int{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
