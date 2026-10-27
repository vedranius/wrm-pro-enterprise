// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// keepRoleSettings restores the settings the role and team tests change.
func keepRoleSettings(t *testing.T) {
	t.Helper()
	keys := []string{"require_step_up", "step_up_minutes", "break_glass_ips", "network_tools", "tunnel_users", "ai_assistant", "require_2fa"}
	saved := map[string]string{}
	for _, k := range keys {
		saved[k] = getSetting(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			setSetting(k, v)
		}
	})
}

func newTeam(t *testing.T, admin *testClient, name string, members ...int) int {
	t.Helper()
	list := []map[string]interface{}{}
	for i, m := range members {
		list = append(list, map[string]interface{}{"user_id": m, "lead": i == 0})
	}
	var res struct {
		ID int `json:"id"`
	}
	if code := admin.jsonDo("POST", "/api/admin/teams", map[string]interface{}{"name": name, "members": list}, &res); code != 200 || res.ID == 0 {
		t.Fatalf("create team %s: %d", name, code)
	}
	return res.ID
}

func setRole(t *testing.T, userID int, role string) {
	t.Helper()
	db.Exec(`UPDATE users SET is_admin=?, role=? WHERE id=?`, boolToInt(role == userRoleAdmin), storedRole(role), userID)
}

func TestWhoPolicyValues(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "wp-admin", true)
	op := newTestUser(t, srv, "wp-operator", false)
	member := newTestUser(t, srv, "wp-member", false)
	plain := newTestUser(t, srv, "wp-user", false)
	auditor := newTestUser(t, srv, "wp-auditor", false)
	setRole(t, op.userID, userRoleOperator)
	setRole(t, auditor.userID, userRoleAuditor)
	newTeam(t, admin, "WP Network", member.userID)

	if v, err := validateWhoValue("teams: wp network , WP Network"); err != nil || v != "teams:WP Network" {
		t.Fatalf("team value: %q %v", v, err)
	}
	if _, err := validateWhoValue("teams:no-such-team"); err == nil {
		t.Fatal("an unknown team was accepted")
	}
	if _, err := validateWhoValue("teams:"); err == nil {
		t.Fatal("an empty team list was accepted")
	}
	cases := []struct {
		value string
		user  *testClient
		want  bool
	}{
		{"all", plain, true}, {"all", auditor, false},
		{"admins", admin, true}, {"admins", op, false},
		{"operators", op, true}, {"operators", admin, true}, {"operators", plain, false}, {"operators", auditor, false},
		{"teams:WP Network", member, true}, {"teams:WP Network", admin, true}, {"teams:WP Network", plain, false}, {"teams:WP Network", op, false},
		{"off", admin, false},
	}
	for _, c := range cases {
		if got := policyAllows(c.user.userID, c.value); got != c.want {
			t.Errorf("policyAllows(%s, %q) = %v", usernameOf(c.user.userID), c.value, got)
		}
	}

	// Through the settings API: a who-policy takes the new values, others do not.
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"network_tools": "teams:wp network"}, nil); code != 200 || getSetting("network_tools") != "teams:WP Network" {
		t.Fatalf("network_tools teams: %d %q", code, getSetting("network_tools"))
	}
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"tunnel_users": "operators"}, nil); code != 200 {
		t.Fatalf("tunnel_users operators: %d", code)
	}
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"registration": "operators"}, nil); code != 400 {
		t.Fatalf("operators accepted for a non-who setting: %d", code)
	}
	var me map[string]interface{}
	member.jsonDo("GET", "/api/auth/me", nil, &me)
	if me["role"] != "user" || len(me["teams"].([]interface{})) != 1 {
		t.Fatalf("me: role %v teams %v", me["role"], me["teams"])
	}

	// A team named in a policy cannot be deleted or renamed.
	var teams []map[string]interface{}
	admin.jsonDo("GET", "/api/admin/teams", nil, &teams)
	id := int(teams[0]["id"].(float64))
	if pols := teams[0]["policies"].([]interface{}); len(pols) != 1 || pols[0] != "network_tools" {
		t.Fatalf("team policies: %v", pols)
	}
	if code := admin.jsonDo("DELETE", fmt.Sprintf("/api/admin/teams/%d", id), nil, nil); code != 409 {
		t.Fatalf("delete of a team in a policy: %d", code)
	}
	if code := admin.jsonDo("PUT", fmt.Sprintf("/api/admin/teams/%d", id), map[string]interface{}{"name": "Other"}, nil); code != 409 {
		t.Fatalf("rename of a team in a policy: %d", code)
	}
	setSetting("network_tools", "admins")
	if code := admin.jsonDo("DELETE", fmt.Sprintf("/api/admin/teams/%d", id), nil, nil); code != 200 {
		t.Fatalf("delete team: %d", code)
	}
	if code := plain.jsonDo("GET", "/api/admin/teams", nil, nil); code != 403 {
		t.Fatalf("teams admin API for a user: %d", code)
	}
	if code := plain.jsonDo("GET", "/api/teams", nil, nil); code != 200 {
		t.Fatalf("team directory: %d", code)
	}
}

func TestSecurityAuditorRole(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "au-admin", true)
	aud := newTestUser(t, srv, "au-auditor", false)
	if code := admin.jsonDo("PUT", fmt.Sprintf("/api/admin/users/%d", aud.userID), map[string]interface{}{"role": "auditor"}, nil); code != 200 {
		t.Fatalf("set auditor: %d", code)
	}
	if userRole(aud.userID) != userRoleAuditor {
		t.Fatalf("role: %s", userRole(aud.userID))
	}
	for _, p := range []string{"/api/admin/audit", "/api/admin/security"} {
		if code := aud.jsonDo("GET", p, nil, nil); code != 200 {
			t.Errorf("auditor GET %s: %d", p, code)
		}
	}
	if code := aud.jsonDo("POST", "/api/admin/security/level", map[string]interface{}{"level": "business", "preview": true}, nil); code != 403 {
		t.Errorf("auditor changes the level: %d", code)
	}
	if code := aud.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"network_tools": "all"}, nil); code != 403 {
		t.Errorf("auditor changes settings: %d", code)
	}
	if code := aud.jsonDo("GET", "/api/admin/users", nil, nil); code != 403 {
		t.Errorf("auditor lists users: %d", code)
	}
	conn := aud.addConnection("au-conn", "127.0.0.1")
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(aud.cookie)
	if _, code, _ := authorizeConnection(req, conn, "terminal"); code != 403 {
		t.Errorf("auditor opens a connection: %d", code)
	}
	if ok, _ := aiUserAllowed(aud.userID); ok {
		t.Error("auditor may use the AI assistant")
	}
	var me map[string]interface{}
	aud.jsonDo("GET", "/api/auth/me", nil, &me)
	if me["role"] != "auditor" || me["is_auditor"] != true || me["is_admin"] == true {
		t.Fatalf("me: %v %v %v", me["role"], me["is_auditor"], me["is_admin"])
	}
	// Unknown roles are refused; an admin cannot demote themselves.
	if code := admin.jsonDo("PUT", fmt.Sprintf("/api/admin/users/%d", aud.userID), map[string]interface{}{"role": "root"}, nil); code != 400 {
		t.Errorf("unknown role: %d", code)
	}
	if code := admin.jsonDo("PUT", fmt.Sprintf("/api/admin/users/%d", admin.userID), map[string]interface{}{"role": "operator"}, nil); code != 400 {
		t.Errorf("self-demotion: %d", code)
	}
}

func TestStepUp(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "su-admin", true)
	other := newTestUser(t, srv, "su-user", false)
	setSetting("require_step_up", "1")

	// Signing in counts as a fresh confirmation.
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"step_up_minutes": "5"}, nil); code != 200 {
		t.Fatalf("right after sign-in: %d", code)
	}
	db.Exec(`UPDATE auth_sessions SET step_up_at='' WHERE user_id=?`, admin.userID)
	var res map[string]interface{}
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"step_up_minutes": "6"}, &res); code != 403 || res["step_up"] != true || res["action"] != "settings" {
		t.Fatalf("stale session: %d %v", code, res)
	}
	if code := admin.jsonDo("DELETE", fmt.Sprintf("/api/admin/users/%d", other.userID), nil, &res); code != 403 || res["step_up"] != true {
		t.Fatalf("delete without step-up: %d %v", code, res)
	}
	if userRole(other.userID) == "" {
		t.Fatal("the user was deleted without step-up")
	}
	if code := admin.jsonDo("POST", "/api/auth/step-up", map[string]string{"password": "wrong", "action": "settings"}, nil); code != 403 {
		t.Fatalf("wrong password: %d", code)
	}
	if code := admin.jsonDo("POST", "/api/auth/step-up", map[string]string{"password": "pw-su-admin-123456", "action": "settings"}, nil); code != 200 {
		t.Fatalf("step-up: %d", code)
	}
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"step_up_minutes": "6"}, nil); code != 200 {
		t.Fatalf("after step-up: %d", code)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action IN ('auth.step_up','auth.step_up_failed') AND user_id=?`, admin.userID).Scan(&n)
	if n != 2 {
		t.Fatalf("step-up audit rows: %d", n)
	}
	// Off: nothing is asked.
	setSetting("require_step_up", "0")
	db.Exec(`UPDATE auth_sessions SET step_up_at='' WHERE user_id=?`, admin.userID)
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"step_up_minutes": "10"}, nil); code != 200 {
		t.Fatalf("step-up off: %d", code)
	}
	// Business and Enterprise require it.
	for i, r := range levelRules {
		if r.Key == "require_step_up" && (levelRules[i].Need[1] != "1" || levelRules[i].Need[2] != "1") {
			t.Fatal("the levels do not require step-up")
		}
	}
}

func TestBreakGlassAdmin(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "bg-admin", true)
	bg := newTestUser(t, srv, "bg-emergency", true)
	plain := newTestUser(t, srv, "bg-user", false)
	if code := admin.jsonDo("PUT", fmt.Sprintf("/api/admin/users/%d", plain.userID), map[string]interface{}{"break_glass": true}, nil); code != 400 {
		t.Fatalf("break-glass for a non-admin: %d", code)
	}
	if code := admin.jsonDo("PUT", fmt.Sprintf("/api/admin/users/%d", bg.userID), map[string]interface{}{"break_glass": true}, nil); code != 200 {
		t.Fatalf("set break-glass: %d", code)
	}
	// Without 2FA its sessions end and the next sign-in has to set it up.
	if code := bg.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatalf("break-glass session kept without 2FA: %d", code)
	}
	login := func(fromIP string) (int, map[string]interface{}) {
		h := newRouter()
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"bg-emergency","password":"pw-bg-emergency-123456"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-WRM-Request", "1")
		req.RemoteAddr = fromIP + ":4000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	setSetting("break_glass_ips", "10.9.0.0/16")
	if code, _ := login("192.0.2.7"); code != 403 {
		t.Fatalf("break-glass from a network not allowed: %d", code)
	}
	code, out := login("10.9.1.2")
	if code != 200 {
		t.Fatalf("break-glass from an allowed network: %d %v", code, out)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='auth.break_glass_login'`).Scan(&n)
	if n == 0 {
		t.Fatal("the break-glass sign-in was not audited")
	}
	db.QueryRow(`SELECT COUNT(*) FROM user_events WHERE user_id=? AND event='security.break_glass_login'`, admin.userID).Scan(&n)
	if n == 0 {
		t.Fatal("other administrators were not told")
	}
	if _, err := validateIPList("10.0.0.1, 10.1.0.0/16"); err != nil {
		t.Fatal(err)
	}
	if _, err := validateIPList("10.0.0.300"); err == nil {
		t.Fatal("bad IP accepted")
	}
	if !breakGlassIPAllowed("10.9.200.1") || breakGlassIPAllowed("10.10.0.1") || breakGlassIPAllowed("bad") {
		t.Fatal("breakGlassIPAllowed")
	}
}

func TestTeamShareAndHandover(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "th-admin", true)
	lead := newTestUser(t, srv, "th-lead", false)
	leaver := newTestUser(t, srv, "th-leaver", false)
	outsider := newTestUser(t, srv, "th-outsider", false)
	teamID := newTeam(t, admin, "TH Ops", lead.userID, leaver.userID)

	// A members-only share with the team: every member gets the team's role.
	conn := leaver.addConnection("th-db", "127.0.0.1")
	var sh struct {
		ID int `json:"id"`
	}
	if code := leaver.jsonDo("POST", "/api/shares", map[string]interface{}{"name": "ops", "access_mode": "members", "connection_ids": []int{conn},
		"teams": []map[string]interface{}{{"team_id": teamID, "role": "operator"}}}, &sh); code != 200 || sh.ID == 0 {
		t.Fatalf("share with a team: %d", code)
	}
	share, _ := getShareByID(sh.ID)
	if a, d := resolveShareAccessCtx(share, accessCtx{UserID: lead.userID}); d != nil || a.Role != RoleOperator {
		t.Fatalf("team member: %+v %+v", a, d)
	}
	if _, d := resolveShareAccessCtx(share, accessCtx{UserID: outsider.userID}); d == nil {
		t.Fatal("an outsider got in")
	}
	var shared []map[string]interface{}
	lead.jsonDo("GET", "/api/shared", nil, &shared)
	if len(shared) != 1 {
		t.Fatalf("shared list for a team member: %v", shared)
	}
	if code := leaver.jsonDo("POST", "/api/shares", map[string]interface{}{"name": "bad", "access_mode": "members", "connection_ids": []int{conn},
		"teams": []map[string]interface{}{{"team_id": teamID, "role": "owner"}}}, nil); code != 400 {
		t.Fatalf("owner role for a team: %d", code)
	}

	// Handover: the lead is suggested and receives everything.
	db.Exec(`INSERT INTO folders (name, user_id) VALUES ('Prod', ?), ('Prod', ?)`, leaver.userID, lead.userID)
	var info struct {
		Owned       handoverCounts `json:"owned"`
		SuggestedID int            `json:"suggested_user_id"`
	}
	if code := admin.jsonDo("GET", fmt.Sprintf("/api/admin/users/%d/handover", leaver.userID), nil, &info); code != 200 || info.SuggestedID != lead.userID || info.Owned.Connections != 1 || info.Owned.Shares != 1 {
		t.Fatalf("handover info: %d %+v", code, info)
	}
	if code := admin.jsonDo("DELETE", fmt.Sprintf("/api/admin/users/%d?transfer_to=%d", leaver.userID, leaver.userID), nil, nil); code != 400 {
		t.Fatalf("handover to self: %d", code)
	}
	if code := admin.jsonDo("DELETE", fmt.Sprintf("/api/admin/users/%d?transfer_to=%d", leaver.userID, lead.userID), nil, nil); code != 200 {
		t.Fatalf("delete with handover: %d", code)
	}
	if !userOwnsConnection(conn, lead.userID) {
		t.Fatal("the connection did not move to the lead")
	}
	var owner, folders int
	db.QueryRow(`SELECT owner_id FROM share_links WHERE id=?`, sh.ID).Scan(&owner)
	db.QueryRow(`SELECT COUNT(*) FROM folders WHERE user_id=? AND name LIKE 'Prod%'`, lead.userID).Scan(&folders)
	if owner != lead.userID || folders != 2 {
		t.Fatalf("share owner %d, folders %d", owner, folders)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='admin.user_handover' AND target='th-leaver'`).Scan(&n)
	if n != 1 {
		t.Fatalf("handover audit: %d", n)
	}
	db.QueryRow(`SELECT COUNT(*) FROM team_members WHERE user_id=?`, leaver.userID).Scan(&n)
	if n != 0 {
		t.Fatal("team membership left behind")
	}
}
