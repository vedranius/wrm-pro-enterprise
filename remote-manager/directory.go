// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
)

// endUserAccess ends everything a user has open in WRM: sign-in sessions, terminals,
// tunnels, collaboration rooms, AI sessions and AI connections (MCP tokens). It is the one
// path for every way an account can be stopped: an administrator disabling it, the
// directory refusing it (OIDC revalidation, LDAP sync) and SCIM deprovisioning.
func endUserAccess(r *http.Request, userID, byID int, reason string) map[string]interface{} {
	sessions := revokeUserSessions(userID, 0)
	terminals, rooms, tunnels := disconnectUserEverywhere(userID, "Your access ended: "+reason)
	ai := aiKillWhere(func(s *aiSession) bool { return s.UserID == userID }, byID, reason)
	mcpShutdown(byID, reason, func(uid int) bool { return uid == userID })
	tokens := mcpRevokeWhere(r, "user_id=?", []interface{}{userID}, byID, reason)
	jobs := mdCancelUserJobs(userID, "access ended: "+reason)
	return map[string]interface{}{"sessions_ended": sessions, "terminals_ended": terminals, "rooms_left": rooms, "tunnels_stopped": tunnels,
		"ai_sessions_ended": ai, "ai_connections_revoked": tokens, "medium_jobs_stopped": jobs}
}
