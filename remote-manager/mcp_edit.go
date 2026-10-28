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

// Editing leaves the bearer hash and expiry intact. Active calls and approvals
// are cancelled; clients can reopen sessions with the same token and new scope.
func mcpEditGrant(w http.ResponseWriter, r *http.Request, id int64, userID int) {
	g, err := loadGrant(id)
	if err != nil || g.UserID != userID || !g.active(time.Now()) {
		jsonError(w, "Active AI connection not found", 404)
		return
	}
	var q mcpGrantRequest
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	// An edit can widen the token (mode, scopes, connections, automatic limits) without
	// changing the bearer string. It therefore needs the same fresh confirmation as the
	// other access-changing actions.
	if !requireStepUp(w, r, "ai_connection") {
		return
	}
	if q.Auto != nil && g.Auto != nil {
		if q.Auto.Deny == nil {
			q.Auto.Deny = g.Auto.Deny
		}
		if q.Auto.PathDeny == nil {
			q.Auto.PathDeny = g.Auto.PathDeny
		}
	}
	q.editing = true
	if err := q.validate(userID); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	ids := []string{}
	for _, id := range q.ConnIDs {
		ids = append(ids, strconv.Itoa(id))
	}
	auto := ""
	if q.Auto != nil {
		b, _ := json.Marshal(q.Auto)
		auto = string(b)
	}
	// Storage uses comma-separated scopes, matching scanGrant.
	_, err = db.Exec(`UPDATE mcp_tokens SET name=?, conn_ids=?, mode=?, scopes=?, auto_limits=? WHERE id=? AND user_id=? AND revoked_at=''`, q.Name, strings.Join(ids, ","), q.Mode, joinMCPScopes(q.Scopes), auto, id, userID)
	if err != nil {
		jsonError(w, "Cannot update AI connection", 500)
		return
	}
	n := aiKillWhere(func(s *aiSession) bool { return s.TokenID == id }, userID, "AI connection permissions changed; reopen the session")
	mcpDropConns(func(c *mcpConn) bool { return c.grantID == id })
	auditLog(r, userID, "mcp.grant_updated", q.Name, map[string]interface{}{"mcp_grant": id, "conn_ids": q.ConnIDs, "mode": q.Mode, "scopes": q.Scopes, "ended": n})
	mcpNotifyUser(userID)
	g, _ = loadGrant(id)
	jsonOK(w, g.view(time.Now()))
}

func joinMCPScopes(sc []string) string {
	b := ""
	for _, s := range sc {
		if b != "" {
			b += ","
		}
		b += s
	}
	return b
}

func mcpPolicySignature(g *mcpGrant) string {
	b, _ := json.Marshal(struct {
		C []int
		M string
		S []string
		A *aiAutoLimits
	}{g.ConnIDs, g.Mode, g.Scopes, g.Auto})
	return sha256Hex(string(b))
}
