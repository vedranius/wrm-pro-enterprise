// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// keepSecuritySettings restores the level, its settings and the deviations after a test.
func keepSecuritySettings(t *testing.T) {
	t.Helper()
	saved := map[string]string{"security_level": getSetting("security_level")}
	chosen := levelChosen()
	for _, r := range levelRules {
		saved[r.Key] = getSetting(r.Key)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			setSetting(k, v)
		}
		db.Exec(`DELETE FROM security_deviations`)
		if !chosen {
			settingsStore.Lock()
			delete(settingsStore.values, "security_level")
			settingsStore.Unlock()
			db.Exec(`DELETE FROM app_settings WHERE key='security_level'`)
		}
	})
}

func TestLevelMeets(t *testing.T) {
	cases := []struct {
		key, value, need string
		want             bool
	}{
		{"require_2fa", "admins", "all", false},
		{"require_2fa", "all", "all", true},
		{"require_2fa", "all", "admins", true},
		{"password_min_length", "14", "12", true},
		{"password_min_length", "10", "12", false},
		{"session_idle_hours", "8", "12", true},
		{"session_idle_hours", "168", "12", false},
		{"link_share_max_days", "0", "30", false}, // 0 = no limit, the loosest
		{"link_share_max_days", "7", "30", true},
		{"host_key_policy", "tofu", "strict", false},
		{"allow_secret_export", "0", "0", true},
		{"allow_secret_export", "1", "0", false},
		{"audit_enabled", "1", "1", true},
		{"registration", "open", "closed", false},
		{"password_min_length", "6", "", true},
	}
	for _, c := range cases {
		if got := levelMeets(c.key, c.value, c.need); got != c.want {
			t.Errorf("levelMeets(%s, %q, %q) = %v", c.key, c.value, c.need, got)
		}
	}
	// Every rule names a real setting, and every required value is valid for it.
	for _, r := range levelRules {
		spec, ok := settingSpecFor(r.Key)
		if !ok {
			t.Fatalf("rule for unknown setting %s", r.Key)
		}
		for i, need := range append(r.Need[:], r.Rec) {
			if need == "" {
				continue
			}
			if v, err := validateSetting(spec, need); err != nil || v != need {
				t.Errorf("%s level %d: %q is not a valid value (%v)", r.Key, i, need, err)
			}
		}
		for i := 1; i < len(securityLevels); i++ { // a higher level is never looser
			if lo, hi := r.Need[i-1], r.Need[i]; lo != "" && hi != "" && !levelMeets(r.Key, hi, lo) {
				t.Errorf("%s: %s is looser than %s", r.Key, securityLevels[i], securityLevels[i-1])
			}
		}
	}
}

func TestSecurityLevelChange(t *testing.T) {
	keepSecuritySettings(t)
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "sl-admin", true)
	other := newTestUser(t, srv, "sl-user", false)
	setSetting("require_2fa", "off")
	setSetting("password_min_length", "8")
	setSetting("allow_secret_export", "1")

	// The preview lists every change and who is affected, and changes nothing.
	var plan levelPlan
	if code := admin.jsonDo("POST", "/api/admin/security/level", map[string]interface{}{"level": "business", "preview": true}, &plan); code != 200 {
		t.Fatalf("preview: %d", code)
	}
	changes := map[string]levelPlanItem{}
	for _, c := range plan.Changes {
		changes[c.Key] = c
	}
	if changes["require_2fa"].New != "all" || changes["password_min_length"].New != "12" || changes["allow_secret_export"].Old != "1" {
		t.Fatalf("preview changes: %+v", plan.Changes)
	}
	if plan.Affected["users_need_2fa"] < 2 || plan.Affected["other_sessions"] < 1 || len(plan.Blockers) != 0 {
		t.Fatalf("preview affected/blockers: %+v %+v", plan.Affected, plan.Blockers)
	}
	if getSetting("require_2fa") != "off" || securityLevel() != "home" {
		t.Fatal("the preview changed settings")
	}

	// Applying keeps one setting looser, as a deviation with a note.
	var res map[string]interface{}
	code := admin.jsonDo("POST", "/api/admin/security/level", map[string]interface{}{
		"level": "business", "keep": map[string]string{"allow_secret_export": "Team exports keys for the DR site"}}, &res)
	if code != 200 || securityLevel() != "business" {
		t.Fatalf("apply: %d %v", code, res)
	}
	if getSetting("require_2fa") != "all" || getSetting("password_min_length") != "12" || getSetting("session_idle_hours") != "12" {
		t.Fatalf("settings not raised: %s %s %s", getSetting("require_2fa"), getSetting("password_min_length"), getSetting("session_idle_hours"))
	}
	if getSetting("allow_secret_export") != "1" || deviationNote("allow_secret_export") == "" {
		t.Fatal("the kept setting changed or has no deviation")
	}
	// Without "apply now" nobody is signed out.
	if code := other.jsonDo("GET", "/api/auth/me", nil, nil); code != 200 {
		t.Fatalf("other user signed out: %d", code)
	}

	// A looser value needs a note.
	var errOut struct {
		Error     string            `json:"error"`
		Deviation map[string]string `json:"deviation"`
	}
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"password_min_length": "8"}, &errOut); code != 400 || errOut.Deviation["required"] != "12" {
		t.Fatalf("loosening without a note: %d %+v", code, errOut)
	}
	if getSetting("password_min_length") != "12" {
		t.Fatal("the setting changed without a note")
	}
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"password_min_length": "10", "_deviation_notes": map[string]string{"password_min_length": "Old scanners type 10"}}, nil); code != 200 {
		t.Fatalf("loosening with a note: %d", code)
	}
	devs := securityDeviations()
	if d := devs["password_min_length"]; d.Value != "10" || d.Required != "12" || d.Note != "Old scanners type 10" || d.Username != "sl-admin" {
		t.Fatalf("deviation: %+v", d)
	}
	// The generic settings call cannot change the level.
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"security_level": "home"}, nil); code != 400 {
		t.Fatalf("level via settings: %d", code)
	}
	// Meeting the level again removes the deviation.
	admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"password_min_length": "16"}, nil)
	if _, ok := securityDeviations()["password_min_length"]; ok {
		t.Fatal("the deviation stayed after the setting met the level")
	}

	// The posture page grades against the level.
	var post struct {
		Level   string         `json:"level"`
		Checks  []postureCheck `json:"checks"`
		Summary map[string]int `json:"summary"`
	}
	if code := admin.jsonDo("GET", "/api/admin/security", nil, &post); code != 200 || post.Level != "business" {
		t.Fatalf("posture: %d %s", code, post.Level)
	}
	byID := map[string]postureCheck{}
	for _, c := range post.Checks {
		byID[c.ID] = c
	}
	if byID["require_2fa"].Status != "ok" || byID["allow_secret_export"].Status != "warn" || byID["allow_secret_export"].Deviation == nil {
		t.Fatalf("rule checks: %+v %+v", byID["require_2fa"], byID["allow_secret_export"])
	}
	if byID["admins_2fa"].Status != "bad" || byID["transport"].Value != "localhost" || byID["audit_chain"].Status == "" {
		t.Fatalf("checks: %+v %+v", byID["admins_2fa"], byID["transport"])
	}
	if code := other.jsonDo("GET", "/api/admin/security", nil, nil); code != 403 {
		t.Fatalf("posture for a non-admin: %d", code)
	}

	// Going back to Home changes no setting and drops deviations that no longer apply.
	if code := admin.jsonDo("POST", "/api/admin/security/level", map[string]interface{}{"level": "home"}, nil); code != 200 {
		t.Fatalf("back to home: %d", code)
	}
	if getSetting("require_2fa") != "all" || len(securityDeviations()) != 0 {
		t.Fatalf("home: %s %v", getSetting("require_2fa"), securityDeviations())
	}

	// "Apply now" signs everyone else out.
	if code := admin.jsonDo("POST", "/api/admin/security/level", map[string]interface{}{"level": "enterprise", "apply_now": true, "keep": map[string]string{"allow_secret_export": "DR site"}}, &res); code != 200 {
		t.Fatalf("enterprise: %d %v", code, res)
	}
	if getSetting("host_key_policy") != "strict" || getSetting("session_max_days") != "1" || getSetting("allow_link_shares") != "0" {
		t.Fatal("enterprise minimums not applied")
	}
	if code := other.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatalf("other user still signed in after apply now: %d", code)
	}
	if code := admin.jsonDo("GET", "/api/auth/me", nil, nil); code != 200 {
		t.Fatalf("the administrator was signed out: %d", code)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='security.level_changed'`).Scan(&n)
	if n < 3 {
		t.Fatalf("level changes audited: %d", n)
	}
}

// Over plain HTTP (not from the WRM host) requiring HTTPS would lock the administrator out:
// the change is refused unless that setting is kept as a deviation, and sign-in is refused.
func TestSecurityLevelHTTPS(t *testing.T) {
	keepSecuritySettings(t)
	trustProxy = true
	t.Cleanup(func() { trustProxy = false })
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "sl-https-admin", true)
	remote := func(method, path string, body interface{}, out interface{}) int {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(b))
		req.AddCookie(admin.cookie)
		req.Header.Set("Origin", srv.URL)
		req.Header.Set("X-WRM-Request", "1")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Real-IP", "192.0.2.50")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if out != nil {
			json.NewDecoder(resp.Body).Decode(out)
		}
		return resp.StatusCode
	}
	var plan levelPlan
	remote("POST", "/api/admin/security/level", map[string]interface{}{"level": "business", "preview": true}, &plan)
	if len(plan.Blockers) != 1 || plan.Blockers[0].Key != "require_https" {
		t.Fatalf("blockers: %+v", plan.Blockers)
	}
	if code := remote("POST", "/api/admin/security/level", map[string]interface{}{"level": "business"}, nil); code != 409 || securityLevel() != "home" {
		t.Fatalf("apply over HTTP: %d %s", code, securityLevel())
	}
	if code := remote("POST", "/api/admin/security/level", map[string]interface{}{"level": "business", "keep": map[string]string{"require_https": "HTTPS comes with the new proxy"}}, nil); code != 200 {
		t.Fatalf("apply with the deviation: %d", code)
	}
	if settingBool("require_https") || deviationNote("require_https") == "" {
		t.Fatal("require_https changed or has no deviation")
	}

	// With HTTPS required, a password over plain HTTP is refused (the WRM host itself may sign in).
	setSetting("require_https", "1")
	h, _ := hashPassword("pw-sl-https-user-123456789")
	db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES ('sl-https-user', ?, 0, ?)`, h, time.Now().UTC().Format(time.RFC3339))
	login := func(ip, proto string) int {
		req, _ := http.NewRequest("POST", srv.URL+"/api/auth/login", strings.NewReader(`{"username":"sl-https-user","password":"pw-sl-https-user-123456789"}`))
		req.Header.Set("Origin", srv.URL)
		req.Header.Set("X-WRM-Request", "1")
		req.Header.Set("Content-Type", "application/json")
		if ip != "" {
			req.Header.Set("X-Real-IP", ip)
		}
		if proto != "" {
			req.Header.Set("X-Forwarded-Proto", proto)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := login("192.0.2.51", ""); code != 403 {
		t.Fatalf("plain HTTP sign-in: %d", code)
	}
	if code := login("192.0.2.51", "https"); code != 200 {
		t.Fatalf("sign-in behind an HTTPS proxy: %d", code)
	}
	if code := login("", ""); code != 200 {
		t.Fatalf("sign-in on the WRM host: %d", code)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/auth/config", nil)
	req.RemoteAddr = "192.0.2.52:5555"
	apiAuthConfigHandler(rec, req)
	if !strings.Contains(rec.Body.String(), "https_required") {
		t.Fatalf("config does not explain the refusal: %s", rec.Body.String())
	}
}

// At start, a setting below the level without a deviation is raised (unless forced by the
// environment), and -security-level sets the level once.
func TestSecurityLevelAtStart(t *testing.T) {
	keepSecuritySettings(t)
	settingsStore.Lock()
	delete(settingsStore.values, "security_level")
	settingsStore.Unlock()
	db.Exec(`DELETE FROM app_settings WHERE key='security_level'`)
	setSetting("password_min_length", "8")
	t.Setenv("WRM_SESSION_IDLE_HOURS", "100")

	applySecurityLevelAtStart("business")
	if securityLevel() != "business" || getSetting("password_min_length") != "12" {
		t.Fatalf("start: %s %s", securityLevel(), getSetting("password_min_length"))
	}
	if getSetting("session_idle_hours") != "100" {
		t.Fatal("an environment-forced setting was changed")
	}
	// Once chosen, a later -security-level does not override the administrator.
	applySecurityLevelAtStart("enterprise")
	if securityLevel() != "business" {
		t.Fatal("-security-level overrode a chosen level")
	}
	plan := planLevelChange("enterprise", httptest.NewRequest("GET", "/", nil), 0)
	found := false
	for _, f := range plan.Forced {
		found = found || (f.Key == "session_idle_hours" && f.Env == "WRM_SESSION_IDLE_HOURS")
	}
	if !found {
		t.Fatalf("forced settings: %+v", plan.Forced)
	}
}

// The first administrator picks the level on the first-run screen.
func TestFirstRunSecurityLevel(t *testing.T) {
	keepSecuritySettings(t)
	settingsStore.Lock()
	delete(settingsStore.values, "security_level")
	settingsStore.Unlock()
	db.Exec(`DELETE FROM app_settings WHERE key='security_level'`)
	trustProxy = true
	t.Cleanup(func() { trustProxy = false })
	req := httptest.NewRequest("POST", "/api/auth/register", nil)
	req.Header.Set("X-Real-IP", "192.0.2.60")
	h, _ := hashPassword("pw-first-run-admin-12345")
	res, _ := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES ('sl-first', ?, 1, ?)`, h, time.Now().UTC().Format(time.RFC3339))
	id, _ := res.LastInsertId()
	applyFirstRunLevel(req, int(id), "business")
	if securityLevel() != "business" || getSetting("require_2fa") != "all" {
		t.Fatalf("first run: %s %s", securityLevel(), getSetting("require_2fa"))
	}
	// Chosen over plain HTTP: HTTPS is not required yet, as a recorded deviation.
	if settingBool("require_https") || !strings.Contains(deviationNote("require_https"), "plain HTTP") {
		t.Fatalf("require_https: %v %q", settingBool("require_https"), deviationNote("require_https"))
	}
}

func TestLinkShareExpiryLimit(t *testing.T) {
	keepSecuritySettings(t)
	setSetting("link_share_max_days", "30")
	soon := time.Now().AddDate(0, 0, 10).UTC().Format(time.RFC3339)
	late := time.Now().AddDate(0, 0, 40).UTC().Format(time.RFC3339)
	if checkLinkShareExpiry("link", "") == nil || checkLinkShareExpiry("link", late) == nil {
		t.Fatal("a guest link without a short expiry was accepted")
	}
	if checkLinkShareExpiry("link", soon) != nil || checkLinkShareExpiry("members", "") != nil {
		t.Fatal("a valid share was refused")
	}
	srv := newTestServer(t)
	u := newTestUser(t, srv, "sl-share", true)
	conn := u.addConnection("demo-api", "demo-api.example.com")
	if code := u.jsonDo("POST", "/api/shares", map[string]interface{}{"name": "x", "access_mode": "link", "connection_ids": []int{conn}}, nil); code != 400 {
		t.Fatalf("link without expiry: %d", code)
	}
	if code := u.jsonDo("POST", "/api/shares", map[string]interface{}{"name": "x", "access_mode": "link", "connection_ids": []int{conn}, "expires_at": soon}, nil); code != 200 {
		t.Fatalf("link with expiry: %d", code)
	}
}
