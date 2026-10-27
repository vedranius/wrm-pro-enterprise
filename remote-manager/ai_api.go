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
	"time"
)

// ─── AI: HTTP API ────────────────────────────────────
//
// User API (/api/ai/…):
//   GET  config                              what the user may do (providers, modes, policy)
//   GET  connections/{id}?share_token=        allowed modes and the "what can the AI see" summary
//   GET|POST|PUT|DELETE providers[/{id}]      personal providers (policy ai_personal_keys)
//   POST providers/{id}/test
//   GET  sessions                             own sessions (open and recent)
//   POST sessions                             start {conn_id, provider_id, model, mode, auto, confirm_auto, share_notes}
//   GET  sessions/{id}                        session + transcript
//   GET  sessions/{id}/events?since=N         server-sent events
//   POST sessions/{id}/prompt                 {text}
//   POST sessions/{id}/mode                   {mode, auto, confirm}
//   POST sessions/{id}/approvals/{aid}        {decision: approve|deny, command?, content?, note?}
//   POST sessions/{id}/stop                   cancel the running request / command
//   POST sessions/{id}/kill                   end the session
//   POST kill-all                             end every own session
//   GET  ollama/status[?url]                  detect Ollama (admins may probe another URL), per-OS commands
//   POST ollama/action                        {action: install|start|pull, model?, confirm} admin + policy ai_ollama_manage
//   GET  ollama/runs[/{id}]                   recorded Ollama actions (admin)
// Admin API (/api/admin/ai/…):
//   GET|POST|PUT|DELETE providers[/{id}], POST providers/{id}/test, GET providers/{id}/models   organisation providers
//   GET  sessions?user&conn&status&limit      every session (active ones marked live)
//   GET  sessions/{id}                        transcript
//   POST sessions/{id}/kill
//   POST users/{id}/kill                      end every session of a user
//   PUT  users/{id}                           {blocked: true|false}
//   GET  summary                              usage per user, active sessions, the destructive rules

func apiAIHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/ai"), "/")
	parts := strings.Split(rest, "/")
	switch parts[0] {
	case "servers":
		aiServerPickerHandler(w, r, userID)
	case "context":
		aiContextHandler(w, r, userID)
	case "config":
		aiConfigHandler(w, r, userID)
	case "delegate":
		aiDelegateHandler(w, r, userID)
	case "connections":
		if len(parts) != 2 {
			jsonError(w, "Not found", 404)
			return
		}
		id, _ := strconv.Atoi(parts[1])
		aiConnectionHandler(w, r, userID, id)
	case "providers":
		handleAIProviders(w, r, userID, "user", strings.TrimPrefix(rest, "providers"))
	case "ollama":
		apiOllamaHandler(w, r, userID, parts[1:])
	case "sessions":
		aiSessionsHandler(w, r, userID, parts[1:])
	case "kill-all":
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", 405)
			return
		}
		n := aiKillWhere(func(s *aiSession) bool { return s.UserID == userID }, userID, "all sessions ended by the user")
		auditLog(r, userID, "ai.kill_all", "", map[string]interface{}{"sessions": n, "scope": "own"})
		jsonOK(w, map[string]int{"ended": n})
	default:
		jsonError(w, "Not found", 404)
	}
}

func aiConfigHandler(w http.ResponseWriter, r *http.Request, userID int) {
	allowed, why := aiUserAllowed(userID)
	modes, _ := normalizeModeList(getSetting("ai_modes"))
	out := map[string]interface{}{
		"enabled": allowed, "reason": why, "providers": []interface{}{}, "modes": modes, "default_mode": getSetting("ai_default_mode"),
		"personal_keys": aiPersonalKeysAllowed(userID), "provider_kinds": splitPatterns(getSetting("ai_provider_kinds")),
		"redact_output": settingBool("ai_redact_output"), "share_notes": settingBool("ai_share_notes"),
		"approval_timeout_seconds": settingInt("ai_approval_timeout_seconds"), "auto_max_minutes": settingInt("ai_auto_max_minutes"),
		"auto_max_actions": settingInt("ai_auto_max_actions"), "kill_switch": settingBool("ai_kill_switch"),
		"read_max_kb":   settingInt("ai_output_max_kb"),
		"show_thinking": settingBool("ai_show_thinking"), "prefs": aiPrefsOf(userID),
	}
	if allowed {
		out["providers"] = providersFor(userID)
		out["delegate"] = aiDelegateView(r.Context(), userID)
		ocOK, ocWhy := opencodeAllowed(userID)
		sbOK, sbWhy := false, ""
		if ocOK && getSetting("ai_opencode_sandbox") != "off" {
			sbOK, sbWhy = opencodeSandboxAllowed(userID)
		}
		out["opencode"] = map[string]interface{}{"available": ocOK, "reason": ocWhy, "kinds": opencodeProviderKinds, "agents": []string{"build", "plan"},
			"sandbox": map[string]interface{}{"offered": getSetting("ai_opencode_sandbox") != "off", "available": sbOK, "reason": sbWhy, "image": opencodeSandboxImage()}}
	}
	active := []map[string]interface{}{}
	for _, s := range aiActiveSessions(func(s *aiSession) bool { return s.UserID == userID && s.Transport != "mcp" }) {
		active = append(active, s.view())
	}
	out["active"] = active
	jsonOK(w, out)
}

// aiConnAccess checks that the user may use a connection for the assistant: own
// connections, or shared ones with a role that allows terminals.
func aiConnAccess(r *http.Request, userID, connID int) (int, int, string) {
	acc, code, msg := authorizeConnection(r, connID, PermTerminal)
	if acc == nil {
		return 0, code, msg
	}
	if acc.UserID != userID || userID <= 0 {
		return 0, 403, "Sign in to use the AI assistant"
	}
	shareID := 0
	if acc.Share != nil {
		shareID = acc.Share.Share.ID
		if !roleCan(acc.Share.Role, PermFilesWrite) {
			return 0, 403, "Your role in this share does not allow the AI assistant"
		}
	}
	return shareID, 0, ""
}

func aiConnectionHandler(w http.ResponseWriter, r *http.Request, userID, connID int) {
	if _, code, msg := aiConnAccess(r, userID, connID); code != 0 {
		jsonError(w, msg, code)
		return
	}
	facts := aiFactsOf(connID)
	allowed, rules := aiAllowedModes(facts)
	var notes string
	var proto string
	db.QueryRow(`SELECT notes, protocol FROM connections WHERE id=?`, connID).Scan(&notes, &proto)
	jsonOK(w, map[string]interface{}{
		"allowed_modes": allowed, "default_mode": aiDefaultMode(allowed), "rules": rules, "ssh": strings.EqualFold(proto, "SSH") || strings.EqualFold(proto, "SFTP"),
		"has_notes": strings.TrimSpace(notes) != "", "share_notes_allowed": settingBool("ai_share_notes"),
		"sees": aiVisibility(allowed),
	})
}

// aiVisibility is the "what can the AI see" summary.
func aiVisibility(allowed []string) map[string]interface{} {
	read := []string{}
	write := []string{}
	for _, t := range aiTools {
		if t.MCPOnly {
			continue
		}
		switch t.Kind {
		case "read":
			read = append(read, t.Def.Name)
		case "write":
			write = append(write, t.Def.Name)
		}
	}
	return map[string]interface{}{
		"read_tools": read, "write_tools": write, "command_tool": "run_command",
		"redacted": settingBool("ai_redact_output"), "read_max_kb": settingInt("ai_output_max_kb"),
		"never":         []string{"passwords and keys stored in WRM", "your WRM session", "other connections", "the terminal screen"},
		"denied_paths":  len(aiReadDenyDefaults) + len(splitPatterns(getSetting("ai_read_deny_paths"))),
		"allowed_modes": allowed,
	}
}

func aiSessionByID(w http.ResponseWriter, idStr string, userID int) *aiSession {
	id, _ := strconv.ParseInt(idStr, 10, 64)
	s := aiGet(id)
	if s == nil || s.UserID != userID {
		jsonError(w, "AI session not found or ended", 404)
		return nil
	}
	return s
}

func aiSessionsHandler(w http.ResponseWriter, r *http.Request, userID int, parts []string) {
	if len(parts) == 0 || parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("view") == "all" {
				aiSessionsViewList(w, r, userID)
				return
			}
			switch p := r.URL.Query().Get("purpose"); p {
			case "":
				jsonOK(w, aiListSessions(`s.user_id = ?`, []interface{}{userID}, 50))
			case "terminal":
				jsonOK(w, aiListSessions(`s.user_id = ? AND s.purpose = ''`, []interface{}{userID}, 100))
			case aiPurposeLogs, aiPurposeFile:
				where, args := `s.user_id = ? AND s.purpose = ?`, []interface{}{userID, p}
				if c, _ := strconv.Atoi(r.URL.Query().Get("conn_id")); c > 0 {
					where, args = where+` AND s.conn_id = ?`, append(args, c)
				}
				jsonOK(w, aiListSessions(where, args, 100))
			default:
				jsonError(w, "Unknown purpose", 400)
			}
		case http.MethodPost:
			aiStartHandler(w, r, userID)
		default:
			jsonError(w, "Method not allowed", 405)
		}
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		id, _ := strconv.ParseInt(parts[0], 10, 64)
		aiTranscriptHandler(w, id, `user_id = ?`, userID)
		return
	}
	if len(parts) == 2 && parts[1] == "title" && r.Method == http.MethodPost {
		aiTitleHandler(w, r, userID, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "save" && r.Method == http.MethodPost {
		aiSaveHandler(w, r, userID, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "export" && r.Method == http.MethodGet {
		aiExportHandler(w, r, userID, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet {
		if s := aiGet(parseID(parts[0])); (s == nil || s.UserID != userID) && aiEndedEvents(w, parseID(parts[0]), userID) {
			return
		}
		s := aiSessionByID(w, parts[0], userID)
		if s == nil {
			return
		}
		aiEventsSSE(w, r, s)
		return
	}
	if r.Method != http.MethodPost || len(parts) < 2 {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if parts[1] == "approvals" && len(parts) == 3 && aiGet(parseID(parts[0])) == nil {
		// a request kept for later whose session has ended (the AI app closed it)
		var in struct {
			Decision string `json:"decision"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || (in.Decision != "approve" && in.Decision != "deny") {
			jsonError(w, "decision must be approve or deny", 400)
			return
		}
		found, err := aiResolveStoredLater(parseID(parts[0]), parts[2], userID, in.Decision == "approve")
		if !found {
			var owner int
			if db.QueryRow(`SELECT COALESCE(user_id,0) FROM ai_sessions WHERE id=?`, parseID(parts[0])).Scan(&owner) != nil || owner != userID {
				jsonError(w, "AI session not found or ended", 404)
				return
			}
			aiDecisionError(w, aiNotPending(parseID(parts[0]), parts[2]))
			return
		}
		if err != nil {
			aiDecisionError(w, err)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true, "session_ended": true})
		return
	}
	s := aiSessionByID(w, parts[0], userID)
	if s == nil {
		return
	}
	// the connection must still be accessible (a share may have been revoked)
	if _, code, msg := aiConnAccess(r, userID, s.ConnID); code != 0 {
		s.Kill(0, "access to the connection was revoked")
		jsonError(w, msg, code)
		return
	}
	switch parts[1] {
	case "prompt":
		var in struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if err := s.Prompt(r, userID, in.Text); err != nil {
			jsonError(w, err.Error(), 409)
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	case "mode":
		var in struct {
			Mode    string        `json:"mode"`
			Auto    *aiAutoLimits `json:"auto"`
			Confirm bool          `json:"confirm"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if err := s.SetMode(r, userID, in.Mode, in.Auto, in.Confirm); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		jsonOK(w, s.view())
	case "approvals":
		if len(parts) != 3 {
			jsonError(w, "Not found", 404)
			return
		}
		var in struct {
			Decision string  `json:"decision"`
			Command  *string `json:"command"`
			Content  *string `json:"content"`
			Note     string  `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if in.Decision != "approve" && in.Decision != "deny" {
			jsonError(w, "decision must be approve or deny", 400)
			return
		}
		if err := s.Resolve(parts[2], userID, in.Decision == "approve", in.Command, in.Content, in.Note); err != nil {
			aiDecisionError(w, err)
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	case "stop":
		s.Stop(userID)
		jsonOK(w, map[string]bool{"ok": true})
	case "kill":
		s.Kill(userID, "ended by the user")
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Not found", 404)
	}
}

func aiStartHandler(w http.ResponseWriter, r *http.Request, userID int) {
	var in struct {
		ConnID      int           `json:"conn_id"`
		ProviderID  int           `json:"provider_id"`
		Model       string        `json:"model"`
		Mode        string        `json:"mode"`
		Auto        *aiAutoLimits `json:"auto"`
		ConfirmAuto bool          `json:"confirm_auto"`
		ShareNotes  bool          `json:"share_notes"`
		Engine      string        `json:"engine"` // "" (WRM) | opencode
		Agent       string        `json:"agent"`
		Sandbox     bool          `json:"sandbox"` // OpenCode with its own tools in a Docker sandbox
		Purpose     string        `json:"purpose"` // "" | logs | file (ai_chats.go)
		FilePath    string        `json:"file_path"`
		ResumeFrom  int64         `json:"resume_from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	transport := "panel"
	if in.Engine == "opencode" {
		transport = "opencode"
		if !reOCAgent.MatchString(nonEmpty(in.Agent, "build")) {
			jsonError(w, "Invalid agent name", 400)
			return
		}
	}
	if in.ResumeFrom > 0 {
		// Continue an ended session: what is not given comes from the old one.
		prev, err := aiLoadPrev(in.ResumeFrom, userID)
		if err != nil {
			jsonError(w, err.Error(), 404)
			return
		}
		if in.ConnID == 0 {
			in.ConnID = prev.ConnID
		}
		if in.ProviderID == 0 {
			in.ProviderID = prev.ProviderID
		}
		if in.Model == "" && prev.Transport != "mcp" && in.ProviderID == prev.ProviderID {
			in.Model = prev.Model
		}
		if in.Mode == "" && prev.Mode != aiModeAuto && oneOf(prev.Mode, aiModesFor("panel", in.ConnID)...) {
			in.Mode = prev.Mode
		}
		// an OpenCode or MCP session continues in the WRM assistant: what it did is the earlier context
		if transport == "opencode" {
			jsonError(w, "A session is continued in the WRM assistant, not in OpenCode", 400)
			return
		}
	}
	if in.ProviderID == 0 && transport != "opencode" {
		in.ProviderID = aiDefaultProviderFor(userID)
	}
	shareID, code, msg := aiConnAccess(r, userID, in.ConnID)
	if code != 0 {
		jsonError(w, msg, code)
		return
	}
	if shareID > 0 {
		in.ShareNotes = false // notes belong to the owner of the connection
	}
	s, err := aiStart(aiStartParams{UserID: userID, Username: usernameOf(userID), ConnID: in.ConnID, ShareID: shareID, ProviderID: in.ProviderID,
		Model: in.Model, Mode: in.Mode, Auto: in.Auto, ConfirmAut: in.ConfirmAuto, ShareNotes: in.ShareNotes, Transport: transport, Request: r, Agent: in.Agent,
		Purpose: in.Purpose, FilePath: in.FilePath, ResumeFrom: in.ResumeFrom})
	if err != nil {
		auditLog(r, userID, "ai.session_refused", "", map[string]interface{}{"conn_id": in.ConnID, "reason": err.Error()})
		jsonError(w, err.Error(), 403)
		return
	}
	if transport == "opencode" {
		if err := s.ocStart(r, in.Agent, in.Sandbox); err != nil {
			s.Kill(0, "OpenCode did not start")
			auditLog(r, userID, "ai.session_refused", "", map[string]interface{}{"conn_id": in.ConnID, "engine": "opencode", "reason": truncateStr(err.Error(), 500)})
			jsonError(w, err.Error(), 502)
			return
		}
	}
	hub.sendTo(userID, jsonMarshal(map[string]interface{}{"type": "ai_sessions_changed"}))
	jsonOK(w, s.view())
}

// aiEventsSSE streams the session's events as server-sent events.
func aiEventsSSE(w http.ResponseWriter, r *http.Request, s *aiSession) {
	fl, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "Streaming is not supported", 500)
		return
	}
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		since, _ = strconv.ParseInt(v, 10, 64)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	fmt.Fprintf(w, "retry: 2000\n\n")
	fl.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		evs, wait, ended := s.eventsSince(since)
		for _, e := range evs {
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.Seq, e.Type, b)
			since = e.Seq
		}
		if len(evs) > 0 {
			fl.Flush()
		}
		if ended {
			return
		}
		select {
		case <-wait:
		case <-ping.C:
			fmt.Fprintf(w, ": ping\n\n")
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// ─── lists and transcripts ───────────────────────────

func aiListSessions(where string, args []interface{}, limit int) []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := db.Query(`SELECT s.id, COALESCE(s.user_id,0), s.username, COALESCE(s.conn_id,0), s.conn_name, s.host, s.transport, s.provider_name, s.provider_kind,
		s.model, s.mode, s.status, s.end_reason, s.started_at, s.ended_at, s.last_activity, s.requests, s.tool_calls, s.approvals, s.denials,
		s.tokens_in, s.tokens_out, s.cost_usd, COALESCE(s.term_session_id,0), s.title, s.purpose, s.resumed_from, s.file_path
		FROM ai_sessions s WHERE `+where+` ORDER BY s.id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, uid, cid, req, tc, ap, dn, ti, to, tid int64
		var user, conn, host, tr, pn, pk, model, mode, status, reason, st, en, la string
		var cost float64
		var title, purpose, filePath string
		var resumed int64
		if rows.Scan(&id, &uid, &user, &cid, &conn, &host, &tr, &pn, &pk, &model, &mode, &status, &reason, &st, &en, &la, &req, &tc, &ap, &dn, &ti, &to, &cost, &tid, &title, &purpose, &resumed, &filePath) != nil {
			continue
		}
		live := aiGet(id) != nil
		if status == "active" && !live {
			status = "ended"
		}
		out = append(out, map[string]interface{}{"id": id, "user_id": uid, "user": user, "conn_id": cid, "connection": conn, "host": host,
			"transport": tr, "provider": pn, "provider_kind": pk, "model": model, "mode": mode, "status": status, "end_reason": reason,
			"started_at": st, "ended_at": en, "last_activity": la, "requests": req, "tool_calls": tc, "approvals": ap, "denials": dn,
			"tokens_in": ti, "tokens_out": to, "cost_usd": cost, "recording_session": tid, "live": live,
			"title": title, "purpose": purpose, "resumed_from": resumed, "file_path": filePath})
	}
	return out
}

func aiTranscriptHandler(w http.ResponseWriter, id int64, where string, arg interface{}) {
	var uid int
	q := `SELECT COALESCE(user_id,0) FROM ai_sessions WHERE id=?`
	qa := []interface{}{id}
	if where != "" {
		q += ` AND ` + where
		qa = append(qa, arg)
	}
	if err := db.QueryRow(q, qa...).Scan(&uid); err == sql.ErrNoRows || err != nil {
		jsonError(w, "AI session not found", 404)
		return
	}
	list := aiListSessions(`s.id = ?`, []interface{}{id}, 1)
	msgs := []map[string]interface{}{}
	rows, err := db.Query(`SELECT seq, ts, kind, actor, content, meta FROM ai_messages WHERE session_id=? ORDER BY seq, id`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var seq int64
			var ts, kind, actor, content, meta string
			if rows.Scan(&seq, &ts, &kind, &actor, &content, &meta) != nil {
				continue
			}
			m := map[string]interface{}{"seq": seq, "ts": ts, "kind": kind, "actor": actor, "content": content}
			if meta != "" {
				var mm map[string]interface{}
				if json.Unmarshal([]byte(meta), &mm) == nil {
					m["meta"] = mm
				}
			}
			msgs = append(msgs, m)
		}
	}
	out := map[string]interface{}{"messages": msgs}
	if len(list) > 0 {
		out["session"] = list[0]
	}
	if s := aiGet(id); s != nil {
		out["live"] = s.view()
	}
	jsonOK(w, out)
}

// ─── admin ───────────────────────────────────────────

func apiAdminAIHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/ai"), "/")
	parts := strings.Split(rest, "/")
	switch parts[0] {
	case "providers":
		handleAIProviders(w, r, adminID, "org", strings.TrimPrefix(rest, "providers"))
	case "summary":
		aiAdminSummary(w)
	case "sessions":
		if len(parts) == 1 {
			q := r.URL.Query()
			where, args := []string{"1=1"}, []interface{}{}
			if v := strings.TrimSpace(q.Get("user")); v != "" {
				where = append(where, "s.username = ?")
				args = append(args, v)
			}
			if v, err := strconv.Atoi(q.Get("conn")); err == nil && v > 0 {
				where = append(where, "s.conn_id = ?")
				args = append(args, v)
			}
			if v := q.Get("status"); v == "active" {
				where = append(where, "s.status = 'active'")
			} else if v != "" {
				where = append(where, "s.status = ?")
				args = append(args, v)
			}
			limit, _ := strconv.Atoi(q.Get("limit"))
			if limit <= 0 || limit > 500 {
				limit = 200
			}
			jsonOK(w, aiListSessions(strings.Join(where, " AND "), args, limit))
			return
		}
		id, _ := strconv.ParseInt(parts[1], 10, 64)
		if len(parts) == 2 && r.Method == http.MethodGet {
			auditLog(r, adminID, "ai.transcript_view", "", map[string]interface{}{"ai_session": id})
			aiTranscriptHandler(w, id, "", nil)
			return
		}
		if len(parts) == 3 && parts[2] == "kill" && r.Method == http.MethodPost {
			s := aiGet(id)
			if s == nil {
				jsonError(w, "The AI session is not active", 404)
				return
			}
			s.Kill(adminID, "killed by an administrator")
			auditLog(r, adminID, "ai.admin_kill", s.ConnName, map[string]interface{}{"ai_session": id, "user": s.Username})
			jsonOK(w, map[string]bool{"ok": true})
			return
		}
		jsonError(w, "Not found", 404)
	case "users":
		if len(parts) < 2 {
			jsonError(w, "Not found", 404)
			return
		}
		uid, _ := strconv.Atoi(parts[1])
		if uid <= 0 {
			jsonError(w, "Not found", 404)
			return
		}
		if len(parts) == 3 && parts[2] == "kill" && r.Method == http.MethodPost {
			n := aiKillWhere(func(s *aiSession) bool { return s.UserID == uid }, adminID, "killed by an administrator (all sessions of the user)")
			auditLog(r, adminID, "ai.kill_all", usernameOf(uid), map[string]interface{}{"sessions": n, "scope": "user", "user_id": uid})
			jsonOK(w, map[string]int{"ended": n})
			return
		}
		if len(parts) == 2 && r.Method == http.MethodPut {
			var in struct {
				Blocked bool `json:"blocked"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				jsonError(w, "Bad JSON", 400)
				return
			}
			db.Exec(`UPDATE users SET ai_blocked=? WHERE id=?`, boolInt(in.Blocked), uid)
			hub.sendTo(uid, []byte(`{"type":"ai_policy_changed"}`))
			n := 0
			if in.Blocked {
				n = aiKillWhere(func(s *aiSession) bool { return s.UserID == uid }, adminID, "the AI assistant was turned off for the user (kill switch)")
			}
			auditLog(r, adminID, "ai.user_blocked", usernameOf(uid), map[string]interface{}{"user_id": uid, "blocked": in.Blocked, "ended": n})
			jsonOK(w, map[string]interface{}{"ok": true, "ended": n})
			return
		}
		jsonError(w, "Not found", 404)
	case "kill-all":
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", 405)
			return
		}
		n := aiKillAll(adminID, "kill switch: every AI session ended by an administrator")
		auditLog(r, adminID, "ai.kill_all", "", map[string]interface{}{"sessions": n, "scope": "global"})
		jsonOK(w, map[string]int{"ended": n})
	default:
		jsonError(w, "Not found", 404)
	}
}

func aiAdminSummary(w http.ResponseWriter) {
	usage := []map[string]interface{}{}
	rows, err := db.Query(`SELECT u.id, u.username, u.ai_blocked, COUNT(s.id), COALESCE(SUM(s.tokens_in),0), COALESCE(SUM(s.tokens_out),0),
		COALESCE(SUM(s.cost_usd),0), COALESCE(MAX(s.started_at),'') FROM users u LEFT JOIN ai_sessions s ON s.user_id=u.id GROUP BY u.id ORDER BY u.username`)
	if err == nil {
		for rows.Next() {
			var id, blocked, n, ti, to int64
			var name, last string
			var cost float64
			if rows.Scan(&id, &name, &blocked, &n, &ti, &to, &cost, &last) == nil {
				usage = append(usage, map[string]interface{}{"user_id": id, "user": name, "blocked": blocked == 1, "sessions": n,
					"tokens_in": ti, "tokens_out": to, "cost_usd": cost, "last": last,
					"active": len(aiActiveSessions(func(s *aiSession) bool { return s.UserID == int(id) }))})
			}
		}
		rows.Close()
	}
	active := []map[string]interface{}{}
	for _, s := range aiActiveSessions(nil) {
		active = append(active, s.view())
	}
	rules := []map[string]string{}
	for _, r := range aiDestructiveRules {
		rules = append(rules, map[string]string{"id": r.ID, "what": r.What})
	}
	jsonOK(w, map[string]interface{}{"usage": usage, "active": active, "destructive_rules": rules, "kill_switch": settingBool("ai_kill_switch")})
}
