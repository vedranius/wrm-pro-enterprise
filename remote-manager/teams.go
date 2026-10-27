// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ─── TEAMS ───────────────────────────────────────────
//
// A team is a named group of users with optional leads. Teams are used in three places:
//   - shares: a share can include teams as members with a role, so every member of the team
//     gets that role (share_teams); the share's connections and folders stay with the team
//     when a person leaves (see handOver);
//   - who-policies: teams:<names> (see policyAllows);
//   - handover: the lead of a leaving user's team is the suggested new owner.
// Administrators manage teams in Admin → Teams.

type teamInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Members     *[]struct {
		UserID int  `json:"user_id"`
		Lead   bool `json:"lead"`
	} `json:"members"`
}

type teamMemberInput struct {
	TeamID int    `json:"team_id"`
	Role   string `json:"role"`
}

func validTeamName(n string) (string, error) {
	n = strings.Join(strings.Fields(n), " ")
	if !teamNameRe.MatchString(n) {
		return "", fmt.Errorf("a team name has 1–40 letters, digits, spaces, '.', '_' or '-' and starts with a letter or digit")
	}
	return n, nil
}

func teamsList(withMembers bool) []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := db.Query(`SELECT t.id, t.name, COALESCE(t.description,''), t.created_at,
		(SELECT COUNT(*) FROM team_members m WHERE m.team_id=t.id),
		(SELECT COUNT(*) FROM share_teams s WHERE s.team_id=t.id)
		FROM teams t ORDER BY t.name COLLATE NOCASE`)
	if err != nil {
		return out
	}
	type row struct {
		id, n, shares         int
		name, desc, createdAt string
	}
	var list []row
	for rows.Next() {
		var x row
		rows.Scan(&x.id, &x.name, &x.desc, &x.createdAt, &x.n, &x.shares)
		list = append(list, x)
	}
	rows.Close()
	for _, x := range list {
		t := map[string]interface{}{"id": x.id, "name": x.name, "description": x.desc, "created_at": x.createdAt, "member_count": x.n, "shares": x.shares}
		if withMembers {
			t["members"] = teamMembers(x.id)
			t["policies"] = teamPolicies(x.name)
		}
		out = append(out, t)
	}
	return out
}

func teamMembers(teamID int) []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := db.Query(`SELECT u.id, u.username, COALESCE(u.display_name,''), m.lead, u.disabled FROM team_members m JOIN users u ON u.id=m.user_id
		WHERE m.team_id=? ORDER BY u.username COLLATE NOCASE`, teamID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, lead, disabled int
		var username, dn string
		rows.Scan(&id, &username, &dn, &lead, &disabled)
		out = append(out, map[string]interface{}{"user_id": id, "username": username, "display_name": dn, "lead": lead == 1, "disabled": disabled == 1})
	}
	return out
}

// teamPolicies lists the who-policies that name the team.
func teamPolicies(name string) []string {
	out := []string{}
	for _, k := range sortedKeys(whoPolicies) {
		v := getSetting(k)
		if !strings.HasPrefix(v, "teams:") {
			continue
		}
		for _, n := range strings.Split(v[6:], ",") {
			if strings.EqualFold(strings.TrimSpace(n), name) {
				out = append(out, k)
				break
			}
		}
	}
	return out
}

// sharesOfTeam returns the shares a team is a member of.
func sharesOfTeam(teamID int) []int {
	var ids []int
	rows, err := db.Query(`SELECT share_id FROM share_teams WHERE team_id=?`, teamID)
	if err != nil {
		return ids
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

// teamShareRole is the best role a user gets in a share through their teams ("" if none).
func teamShareRole(shareID, userID int) string {
	rows, err := db.Query(`SELECT st.role FROM share_teams st JOIN team_members tm ON tm.team_id=st.team_id WHERE st.share_id=? AND tm.user_id=?`, shareID, userID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	best := ""
	for rows.Next() {
		var role string
		rows.Scan(&role)
		if !validAssignableRole(role, roleRank[RoleModerator]) {
			role = RoleOperator
		}
		if roleRank[role] > roleRank[best] {
			best = role
		}
	}
	return best
}

func applyShareTeams(tx *sql.Tx, shareID int, teams []teamMemberInput) {
	tx.Exec(`DELETE FROM share_teams WHERE share_id=?`, shareID)
	seen := map[int]bool{}
	for _, t := range teams {
		if t.TeamID <= 0 || seen[t.TeamID] {
			continue
		}
		var n int
		if tx.QueryRow(`SELECT COUNT(*) FROM teams WHERE id=?`, t.TeamID).Scan(&n); n == 0 {
			continue
		}
		seen[t.TeamID] = true
		tx.Exec(`INSERT INTO share_teams (share_id, team_id, role, added_at) VALUES (?,?,?,?)`, shareID, t.TeamID, t.Role, nowRFC())
	}
}

func shareTeamsOf(shareID int) []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := db.Query(`SELECT t.id, t.name, st.role, (SELECT COUNT(*) FROM team_members m WHERE m.team_id=t.id)
		FROM share_teams st JOIN teams t ON t.id=st.team_id WHERE st.share_id=? ORDER BY t.name COLLATE NOCASE`, shareID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, n int
		var name, role string
		rows.Scan(&id, &name, &role, &n)
		out = append(out, map[string]interface{}{"team_id": id, "name": name, "role": role, "member_count": n})
	}
	return out
}

// revalidateTeamShares re-checks everyone connected through the team's shares after the
// team's members changed or the team was removed.
func revalidateTeamShares(shareIDs []int) {
	for _, id := range shareIDs {
		go revalidateShare(id)
	}
}

// GET /api/teams — every team's id and name (for the share dialog), plus my teams.
func apiTeamsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", 405)
		return
	}
	all := []map[string]interface{}{}
	for _, t := range teamsList(false) {
		all = append(all, map[string]interface{}{"id": t["id"], "name": t["name"], "member_count": t["member_count"]})
	}
	jsonOK(w, map[string]interface{}{"teams": all, "mine": userTeams(userID)})
}

// GET  /api/admin/teams          teams with members and the policies that name them
// POST /api/admin/teams          {name, description, members:[{user_id, lead}]}
// PUT  /api/admin/teams/{id}     same fields, each optional
// DELETE /api/admin/teams/{id}   (refused while a policy names the team)
func apiAdminTeamsHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/teams"), "/")
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			jsonOK(w, teamsList(true))
		case http.MethodPost:
			saveTeam(w, r, adminID, 0)
		default:
			jsonError(w, "Method not allowed", 405)
		}
		return
	}
	id, err := strconv.Atoi(rest)
	if err != nil || id <= 0 {
		jsonError(w, "Bad ID", 400)
		return
	}
	var name string
	if db.QueryRow(`SELECT name FROM teams WHERE id=?`, id).Scan(&name) != nil {
		jsonError(w, "Team not found", 404)
		return
	}
	switch r.Method {
	case http.MethodPut:
		saveTeam(w, r, adminID, id)
	case http.MethodDelete:
		if pols := teamPolicies(name); len(pols) > 0 {
			jsonError(w, "The team is named in the policies "+strings.Join(pols, ", ")+": change them first", 409)
			return
		}
		shares := sharesOfTeam(id)
		members := len(teamMembers(id))
		db.Exec(`DELETE FROM share_teams WHERE team_id=?`, id)
		db.Exec(`DELETE FROM team_members WHERE team_id=?`, id)
		db.Exec(`DELETE FROM teams WHERE id=?`, id)
		auditLog(r, adminID, "admin.team_deleted", name, map[string]int{"team_id": id, "members": members, "shares": len(shares)})
		revalidateTeamShares(shares)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func saveTeam(w http.ResponseWriter, r *http.Request, adminID, id int) {
	var in teamInput
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	var oldName string
	if id > 0 {
		db.QueryRow(`SELECT name FROM teams WHERE id=?`, id).Scan(&oldName)
	} else if in.Name == nil {
		jsonError(w, "Enter a team name", 400)
		return
	}
	name := oldName
	if in.Name != nil {
		n, err := validTeamName(*in.Name)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if id > 0 && !strings.EqualFold(n, oldName) && len(teamPolicies(oldName)) > 0 {
			jsonError(w, "The team is named in the policies "+strings.Join(teamPolicies(oldName), ", ")+": change them before renaming it", 409)
			return
		}
		name = n
	}
	tx, err := db.Begin()
	if err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	defer tx.Rollback()
	if id == 0 {
		res, err := tx.Exec(`INSERT INTO teams (name, description, created_at) VALUES (?,?,?)`, name, "", nowRFC())
		if err != nil {
			jsonError(w, "A team with this name already exists", 400)
			return
		}
		n, _ := res.LastInsertId()
		id = int(n)
	} else if name != oldName {
		if _, err := tx.Exec(`UPDATE teams SET name=? WHERE id=?`, name, id); err != nil {
			jsonError(w, "A team with this name already exists", 400)
			return
		}
	}
	if in.Description != nil {
		tx.Exec(`UPDATE teams SET description=? WHERE id=?`, truncateStr(strings.TrimSpace(*in.Description), 300), id)
	}
	changes := map[string]interface{}{"team_id": id}
	if name != oldName && oldName != "" {
		changes["renamed_from"] = oldName
	}
	removed := false
	if in.Members != nil {
		keep := map[int]bool{}
		leads := 0
		for _, m := range *in.Members {
			var exists int
			if tx.QueryRow(`SELECT COUNT(*) FROM users WHERE id=?`, m.UserID).Scan(&exists); exists == 0 || keep[m.UserID] {
				continue
			}
			keep[m.UserID] = true
			if m.Lead {
				leads++
			}
			tx.Exec(`INSERT INTO team_members (team_id, user_id, lead, added_at) VALUES (?,?,?,?)
				ON CONFLICT(team_id, user_id) DO UPDATE SET lead=excluded.lead`, id, m.UserID, boolToInt(m.Lead), nowRFC())
		}
		rows, err := tx.Query(`SELECT user_id FROM team_members WHERE team_id=?`, id)
		if err == nil {
			var drop []int
			for rows.Next() {
				var uid int
				rows.Scan(&uid)
				if !keep[uid] {
					drop = append(drop, uid)
				}
			}
			rows.Close()
			for _, uid := range drop {
				tx.Exec(`DELETE FROM team_members WHERE team_id=? AND user_id=?`, id, uid)
				removed = true
			}
		}
		changes["members"], changes["leads"] = len(keep), leads
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	action := "admin.team_updated"
	if oldName == "" {
		action = "admin.team_created"
	}
	auditLog(r, adminID, action, name, changes)
	if removed || (name != oldName && oldName != "") {
		revalidateTeamShares(sharesOfTeam(id))
	}
	if in.Members != nil {
		hub.sendAll([]byte(`{"type":"ai_policy_changed"}`)) // policy flags in /api/auth/me may differ now
	}
	jsonOK(w, map[string]interface{}{"ok": true, "id": id, "name": name})
}
