// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ─── SECURITY LEVELS ─────────────────────────────────
//
// One WRM fits a home lab, a small company and a large organisation. The administrator picks a
// level (on the first-run screen, with -security-level, WRM_SECURITY_LEVEL or later in
// Admin → Security). A level is a floor: a set of minimum values for security settings.
//
//   - Home (default) keeps the behaviour of earlier versions and only recommends.
//   - Business is strong enough that ordinary attacks do not get through.
//   - Enterprise is provable and centrally managed.
//
// Raising the level first shows every change and who is affected. An administrator may keep a
// single setting looser than the level; that is a deviation with a required note, audited and
// listed on the security posture page and in the auditor report. A setting below the level with
// no deviation cannot exist: WRM raises it at start (unless WRM_<KEY> forces it, which the posture
// page then reports).

var securityLevels = []string{"home", "business", "enterprise"}

func levelIndex(level string) int {
	for i, l := range securityLevels {
		if l == level {
			return i
		}
	}
	return 0
}

func securityLevel() string { return getSetting("security_level") }

// levelRule is the minimum one setting must meet at each level.
type levelRule struct {
	Key     string
	Order   []string  // bool/enum: values from the loosest to the strictest
	Less    bool      // int: a smaller value is stricter (lifetimes, failures)
	ZeroOff bool      // int: 0 means "no limit", the loosest value
	Need    [3]string // home, business, enterprise; "" = no requirement
	Rec     string    // recommended at Home (the posture page shows it, nothing is enforced)
}

var levelRules = []levelRule{
	{Key: "require_https", Order: []string{"0", "1"}, Need: [3]string{"", "1", "1"}},
	{Key: "require_2fa", Order: []string{"off", "admins", "all"}, Need: [3]string{"", "all", "all"}, Rec: "admins"},
	{Key: "password_min_length", Need: [3]string{"", "12", "14"}},
	{Key: "session_idle_hours", Less: true, Need: [3]string{"", "12", "8"}},
	{Key: "session_max_days", Less: true, Need: [3]string{"", "7", "1"}},
	{Key: "login_max_failures", Less: true, Need: [3]string{"", "5", "5"}},
	{Key: "registration", Order: []string{"open", "closed"}, Need: [3]string{"", "closed", "closed"}, Rec: "closed"},
	{Key: "host_key_policy", Order: []string{"off", "tofu", "strict"}, Need: [3]string{"", "tofu", "strict"}, Rec: "tofu"},
	{Key: "allow_server_keys", Order: []string{"1", "0"}, Need: [3]string{"", "0", "0"}},
	{Key: "allow_secret_export", Order: []string{"1", "0"}, Need: [3]string{"", "0", "0"}},
	{Key: "allow_link_shares", Order: []string{"1", "0"}, Need: [3]string{"", "", "0"}},
	{Key: "link_share_max_days", Less: true, ZeroOff: true, Need: [3]string{"", "30", "7"}},
	{Key: "ai_full_access", Order: []string{"1", "0"}, Need: [3]string{"", "0", "0"}},
	{Key: "require_step_up", Order: []string{"0", "1"}, Need: [3]string{"", "1", "1"}},
	{Key: "screen_lock_mode", Order: []string{"browser", "remember", "server"}, Need: [3]string{"", "server", "server"}, Rec: "server"},
	{Key: "screen_lock_max_minutes", Less: true, ZeroOff: true, Need: [3]string{"", "30", "15"}},
	{Key: "screen_lock_pin", Order: []string{"1", "0"}, Need: [3]string{"", "", "0"}},
	{Key: "local_login", Order: []string{"all", "break_glass"}, Need: [3]string{"", "", "break_glass"}},
	{Key: "audit_enabled", Order: []string{"0", "1"}, Need: [3]string{"", "1", "1"}, Rec: "1"},
	{Key: "audit_retention_days", Need: [3]string{"", "365", "730"}},
	{Key: "session_recording", Order: []string{"0", "1"}, Need: [3]string{"", "1", "1"}},
	{Key: "recording_retention_days", Need: [3]string{"", "", "365"}},
}

func levelRuleFor(key string) (levelRule, bool) {
	for _, r := range levelRules {
		if r.Key == key {
			return r, true
		}
	}
	return levelRule{}, false
}

// levelNeed is the minimum the level sets for key ("" when it sets none).
func levelNeed(level, key string) string {
	r, ok := levelRuleFor(key)
	if !ok {
		return ""
	}
	return r.Need[levelIndex(level)]
}

// levelMeets reports whether value is at least as strict as need.
func levelMeets(key, value, need string) bool {
	r, ok := levelRuleFor(key)
	if !ok || need == "" {
		return true
	}
	if r.Order != nil {
		pos := func(v string) int {
			for i, o := range r.Order {
				if o == v {
					return i
				}
			}
			return -1
		}
		return pos(value) >= pos(need)
	}
	v, err1 := strconv.Atoi(value)
	n, err2 := strconv.Atoi(need)
	if err1 != nil || err2 != nil {
		return false
	}
	if r.ZeroOff && v == 0 {
		return n == 0
	}
	if r.Less {
		return v <= n
	}
	return v >= n
}

// ─── deviations ──────────────────────────────────────

type securityDeviation struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Required  string `json:"required"`
	Level     string `json:"level"`
	Note      string `json:"note"`
	UserID    int    `json:"user_id"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

func securityDeviations() map[string]securityDeviation {
	out := map[string]securityDeviation{}
	rows, err := db.Query(`SELECT key, value, required, level, note, user_id, username, created_at FROM security_deviations`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var d securityDeviation
		if rows.Scan(&d.Key, &d.Value, &d.Required, &d.Level, &d.Note, &d.UserID, &d.Username, &d.CreatedAt) == nil {
			out[d.Key] = d
		}
	}
	return out
}

func deviationNote(key string) string {
	var n string
	db.QueryRow(`SELECT note FROM security_deviations WHERE key=?`, key).Scan(&n)
	return n
}

const maxDeviationNote = 500

func recordDeviation(r *http.Request, userID int, key, note string) {
	if len(note) > maxDeviationNote {
		note = note[:maxDeviationNote]
	}
	level, value := securityLevel(), getSetting(key)
	need := levelNeed(level, key)
	username := "system"
	if userID > 0 {
		username = usernameOf(userID)
	}
	db.Exec(`INSERT INTO security_deviations (key, value, required, level, note, user_id, username, created_at) VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, required=excluded.required, level=excluded.level, note=excluded.note,
		user_id=excluded.user_id, username=excluded.username, created_at=excluded.created_at`,
		key, value, need, level, note, userID, username, time.Now().UTC().Format(time.RFC3339))
	auditLogAs(r, userID, username, "security.deviation", key, map[string]string{"value": value, "required": need, "level": level, "note": note})
}

func removeDeviation(r *http.Request, userID int, key, why string) {
	res, err := db.Exec(`DELETE FROM security_deviations WHERE key=?`, key)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		username := "system"
		if userID > 0 {
			username = usernameOf(userID)
		}
		auditLogAs(r, userID, username, "security.deviation_removed", key, map[string]string{"value": getSetting(key), "why": why})
	}
}

// syncDeviation records or removes the deviation of key after its value changed.
func syncDeviation(r *http.Request, userID int, key, note string) {
	need := levelNeed(securityLevel(), key)
	if need == "" || levelMeets(key, getSetting(key), need) {
		removeDeviation(r, userID, key, "the setting meets the security level")
		return
	}
	if len(note) < 3 {
		note = deviationNote(key)
	}
	recordDeviation(r, userID, key, note)
}

// ─── applying a level ────────────────────────────────

type levelPlanItem struct {
	Key  string `json:"key"`
	Old  string `json:"old"`
	New  string `json:"new"`
	Env  string `json:"env,omitempty"`
	Note string `json:"note,omitempty"`
}

type levelPlan struct {
	From     string          `json:"from"`
	To       string          `json:"to"`
	Locked   bool            `json:"level_locked"`
	Changes  []levelPlanItem `json:"changes"`
	Forced   []levelPlanItem `json:"forced"`   // below the level but set by WRM_<KEY>
	Kept     []levelPlanItem `json:"kept"`     // existing deviations that stay
	Blockers []levelPlanItem `json:"blockers"` // would lock this administrator out: keep with a note
	Removed  []string        `json:"removed"`  // deviations that no longer apply
	Affected map[string]int  `json:"affected"`
}

func requestFromLoopback(r *http.Request) bool {
	ip := net.ParseIP(clientIP(r))
	return ip != nil && ip.IsLoopback()
}

// secureTransport: HTTPS, or a browser on the WRM host itself.
func secureTransport(r *http.Request) bool { return isHTTPS(r) || requestFromLoopback(r) }

// signInTransportError refuses passwords over plain HTTP when the level requires HTTPS.
func signInTransportError(r *http.Request) string {
	if settingBool("require_https") && !secureTransport(r) {
		return "Signing in over plain HTTP is turned off on this server (security level). Open WRM with https://. An administrator sets up HTTPS with HTTPS_CERT_FILE and HTTPS_KEY_FILE, HTTPS_SELF_SIGNED=1 or a reverse proxy (WRM_TRUST_PROXY=1), or signs in on the WRM host itself (localhost)."
	}
	return ""
}

func planLevelChange(level string, r *http.Request, sessionID int) levelPlan {
	p := levelPlan{From: securityLevel(), To: level, Locked: settingLocked("security_level"), Affected: map[string]int{},
		Changes: []levelPlanItem{}, Forced: []levelPlanItem{}, Kept: []levelPlanItem{}, Blockers: []levelPlanItem{}, Removed: []string{}}
	devs := securityDeviations()
	for _, rule := range levelRules {
		need, cur := rule.Need[levelIndex(level)], getSetting(rule.Key)
		if need == "" || levelMeets(rule.Key, cur, need) {
			if _, ok := devs[rule.Key]; ok {
				p.Removed = append(p.Removed, rule.Key)
			}
			continue
		}
		it := levelPlanItem{Key: rule.Key, Old: cur, New: need}
		switch {
		case settingLocked(rule.Key):
			_, it.Env = settingEnv(rule.Key)
			p.Forced = append(p.Forced, it)
		case devs[rule.Key].Note != "":
			it.Note = devs[rule.Key].Note
			p.Kept = append(p.Kept, it)
		case rule.Key == "require_https" && !secureTransport(r):
			p.Blockers = append(p.Blockers, it)
		default:
			p.Changes = append(p.Changes, it)
		}
	}
	will := func(key string) (string, bool) {
		for _, c := range append(append([]levelPlanItem{}, p.Changes...), p.Blockers...) {
			if c.Key == key {
				return c.New, true
			}
		}
		return "", false
	}
	if v, ok := will("require_2fa"); ok {
		q := `SELECT COUNT(*) FROM users WHERE disabled=0 AND totp_enabled=0`
		if v == "admins" {
			q += ` AND is_admin=1`
		}
		var n int
		db.QueryRow(q).Scan(&n)
		p.Affected["users_need_2fa"] = n
	}
	if v, ok := will("session_idle_hours"); ok {
		h, _ := strconv.Atoi(v)
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM auth_sessions WHERE id != ? AND last_active_at != '' AND last_active_at < ?`,
			sessionID, time.Now().Add(-time.Duration(h)*time.Hour).UTC().Format(time.RFC3339)).Scan(&n)
		p.Affected["idle_sessions_end"] = n
	}
	if _, ok := will("allow_link_shares"); ok {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM share_links s JOIN users u ON u.id=s.owner_id WHERE s.active=1 AND s.access_mode='link' AND u.is_admin=0`).Scan(&n)
		p.Affected["link_shares_blocked"] = n
	}
	if v, ok := will("link_share_max_days"); ok {
		d, _ := strconv.Atoi(v)
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM share_links WHERE active=1 AND access_mode='link' AND (expires_at='' OR expires_at > ?)`,
			time.Now().AddDate(0, 0, d).UTC().Format(time.RFC3339)).Scan(&n)
		p.Affected["link_shares_long"] = n
	}
	if _, ok := will("password_min_length"); ok {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM users WHERE disabled=0`).Scan(&n)
		p.Affected["passwords_on_next_change"] = n
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM auth_sessions WHERE id != ?`, sessionID).Scan(&n)
	p.Affected["other_sessions"] = n
	return p
}

// enforceSecurityLevel raises every setting below the level that is neither forced by the
// environment nor kept as a deviation, and removes deviations that no longer apply.
func enforceSecurityLevel(r *http.Request, userID int, why string) map[string]string {
	level := securityLevel()
	devs := securityDeviations()
	changed := map[string]string{}
	for _, rule := range levelRules {
		need, cur := rule.Need[levelIndex(level)], getSetting(rule.Key)
		if need == "" || levelMeets(rule.Key, cur, need) {
			if _, ok := devs[rule.Key]; ok {
				removeDeviation(r, userID, rule.Key, "the security level no longer requires more")
			}
			continue
		}
		if settingLocked(rule.Key) || devs[rule.Key].Note != "" {
			continue
		}
		if v, err := setSetting(rule.Key, need); err == nil {
			changed[rule.Key] = v
		} else {
			log.Printf("security level: %v", err)
		}
	}
	if len(changed) > 0 {
		username := "system"
		if userID > 0 {
			username = usernameOf(userID)
		}
		auditLogAs(r, userID, username, "security.level_enforced", level, map[string]interface{}{"changed": changed, "why": why})
	}
	return changed
}

func validSecurityLevel(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	for _, l := range securityLevels {
		if l == v {
			return l, true // the constant, never the caller's string
		}
	}
	return "", false
}

// levelChosen: a level was stored (by the first-run screen, -security-level or an administrator).
func levelChosen() bool {
	settingsStore.RLock()
	_, ok := settingsStore.values["security_level"]
	settingsStore.RUnlock()
	return ok
}

// initialSecurityLevelEnv carries -security-level from -install-service to the service, which
// applies it once, as long as no level was chosen yet.
const initialSecurityLevelEnv = "WRM_INITIAL_SECURITY_LEVEL"

var initialSecurityLevel string // -security-level

// applySecurityLevelAtStart sets the initial level (if none was chosen yet) and enforces it.
func applySecurityLevelAtStart(initial string) {
	if initial == "" {
		initial = os.Getenv(initialSecurityLevelEnv)
	}
	if initial != "" && !levelChosen() && !settingLocked("security_level") {
		if l, ok := validSecurityLevel(initial); !ok {
			log.Printf("WARNING: ignoring the initial security level from -security-level or %s: use home, business or enterprise", initialSecurityLevelEnv)
		} else if _, err := setSetting("security_level", l); err == nil {
			auditLogAs(nil, 0, "system", "security.level_changed", l, map[string]string{"from": "home", "to": l, "by": "-security-level"})
			log.Printf("Security level set to %s (-security-level)", securityLevels[levelIndex(l)])
		}
	}
	if changed := enforceSecurityLevel(nil, 0, "start"); len(changed) > 0 {
		log.Printf("Security level %s: raised %d settings to its minimum", securityLevels[levelIndex(securityLevel())], len(changed))
	}
}

// applyFirstRunLevel stores the level the first administrator chose on the first-run screen.
func applyFirstRunLevel(r *http.Request, userID int, level string) {
	l, ok := validSecurityLevel(level)
	if !ok || levelChosen() || settingLocked("security_level") {
		return
	}
	if _, err := setSetting("security_level", l); err != nil {
		return
	}
	auditLog(r, userID, "security.level_changed", l, map[string]string{"from": "home", "to": l, "by": "first-run screen"})
	// Choosing Business over plain HTTP must not lock the administrator out of the next sign-in.
	if levelNeed(l, "require_https") == "1" && !secureTransport(r) && !settingBool("require_https") && !settingLocked("require_https") {
		recordDeviation(r, userID, "require_https", "Chosen on the first-run screen over plain HTTP. Turn on HTTPS, then remove this deviation.")
	}
	settingsChanged(userID, enforceSecurityLevel(r, userID, "first-run screen"))
}

// ─── link shares ─────────────────────────────────────

// checkLinkShareExpiry applies link_share_max_days to links for people without an account.
func checkLinkShareExpiry(mode, expires string) error {
	max := settingInt("link_share_max_days")
	if mode != "link" || max <= 0 {
		return nil
	}
	if expires == "" {
		return fmt.Errorf("links for people without an account need an expiry date on this server (at most %d days)", max)
	}
	t, ok := parseTS(expires)
	if !ok {
		return fmt.Errorf("invalid expiry date")
	}
	if t.After(time.Now().AddDate(0, 0, max).Add(time.Hour)) {
		return fmt.Errorf("links for people without an account may last at most %d days on this server", max)
	}
	return nil
}

// ─── admin API ───────────────────────────────────────

// GET  /api/admin/security        → level, rules, deviations and the security posture
// POST /api/admin/security/level  {level, preview, keep: {key: note}, apply_now}
func apiAdminSecurityHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAuditRead(w, r) // auditors read the posture; changing the level stays with administrators
	if !ok {
		return
	}
	if r.Method != http.MethodGet && !isAdminUser(adminID) {
		jsonError(w, "Admin access required", 403)
		return
	}
	sess, err := lookupSession(r)
	if err != nil {
		jsonError(w, "Unauthorized", 401)
		return
	}
	switch {
	case r.URL.Path == "/api/admin/security" && r.Method == http.MethodGet:
		jsonOK(w, securityPosture(r))
	case r.URL.Path == "/api/admin/security/level" && r.Method == http.MethodPost:
		var in struct {
			Level    string            `json:"level"`
			Preview  bool              `json:"preview"`
			Keep     map[string]string `json:"keep"`
			ApplyNow bool              `json:"apply_now"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		level, ok := validSecurityLevel(in.Level)
		if !ok {
			jsonError(w, "Unknown security level (home, business or enterprise)", 400)
			return
		}
		plan := planLevelChange(level, r, sess.ID)
		if in.Preview {
			jsonOK(w, plan)
			return
		}
		if !requireStepUp(w, r, "security_level") {
			return
		}
		if plan.Locked && level != securityLevel() {
			_, env := settingEnv("security_level")
			jsonError(w, "The security level is set by the environment variable "+env, 409)
			return
		}
		keep := map[string]string{}
		for k, v := range in.Keep {
			if v = strings.TrimSpace(v); len(v) >= 3 {
				keep[k] = v
			}
		}
		for _, b := range plan.Blockers {
			if keep[b.Key] == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "You are connected over plain HTTP: requiring HTTPS would stop your next sign-in. Turn on HTTPS first, or keep this setting as a deviation with a note.",
					"plan":  plan,
				})
				return
			}
		}
		from := securityLevel()
		if level != from {
			if _, err := setSetting("security_level", level); err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
		}
		kept := []string{}
		for _, c := range append(append([]levelPlanItem{}, plan.Changes...), plan.Blockers...) {
			if note := keep[c.Key]; note != "" {
				recordDeviation(r, adminID, c.Key, note)
				kept = append(kept, c.Key)
			}
		}
		changed := enforceSecurityLevel(r, adminID, "security level changed to "+level)
		signedOut := int64(0)
		if in.ApplyNow {
			if res, err := db.Exec(`DELETE FROM auth_sessions WHERE id != ?`, sess.ID); err == nil {
				signedOut, _ = res.RowsAffected()
			}
		}
		keys := []string{}
		for k := range changed {
			keys = append(keys, k)
		}
		auditLog(r, adminID, "security.level_changed", level, map[string]interface{}{
			"from": from, "to": level, "changed": keys, "kept": kept, "apply_now": in.ApplyNow, "signed_out": signedOut})
		settingsChanged(adminID, changed)
		jsonOK(w, map[string]interface{}{"ok": true, "level": level, "changed": changed, "kept": kept, "signed_out": signedOut})
	default:
		jsonError(w, "Not found", 404)
	}
}
