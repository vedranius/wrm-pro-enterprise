// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ─── AI SESSIONS VIEW AND PERSONAL AI SETTINGS ───────
//
// Every user sees all of their own AI sessions in one place, whatever started them: the
// assistant in a terminal, the log and file assistants, OpenCode agents and AI apps over
// MCP. A session can be searched, opened (transcript, live view), continued in the WRM
// assistant (also an MCP or OpenCode session: its tool calls become the earlier context),
// saved (kept beyond the transcript retention while ai_keep_saved_sessions is on) and
// exported as Markdown or JSON.
//
//   GET  /api/ai/sessions?view=all&kind=&q=&before=&limit=   own sessions, newest first
//   POST /api/ai/sessions/{id}/save    {saved}
//   GET  /api/ai/sessions/{id}/export?format=md|json
//
// Personal AI settings (/api/me/ui/ai, stored like the panel layout): the default provider,
// model, mode and engine for new sessions, and whether to show the model's thinking.

func initAISessionsViewSchema() {
	db.Exec(`ALTER TABLE ai_sessions ADD COLUMN saved INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`CREATE INDEX IF NOT EXISTS ix_ai_sessions_user ON ai_sessions(user_id, id)`)
}

// aiSessionKinds maps the filter of the AI sessions view to SQL.
var aiSessionKinds = map[string]string{
	"assistant": `s.transport = 'panel' AND s.purpose = ''`,
	"logs":      `s.purpose = 'logs'`,
	"file":      `s.purpose = 'file'`,
	"opencode":  `s.transport = 'opencode'`,
	"mcp":       `s.transport = 'mcp'`,
	"saved":     `s.saved = 1`,
	"live":      `s.status = 'active'`,
}

func aiSessionsViewList(w http.ResponseWriter, r *http.Request, userID int) {
	q := r.URL.Query()
	where, args := []string{"s.user_id = ?"}, []interface{}{userID}
	if k := q.Get("kind"); k != "" {
		cond, ok := aiSessionKinds[k]
		if !ok {
			jsonError(w, "Unknown kind", 400)
			return
		}
		where = append(where, cond)
	}
	if c, _ := strconv.Atoi(q.Get("conn_id")); c > 0 {
		where, args = append(where, "s.conn_id = ?"), append(args, c)
	}
	if b, _ := strconv.ParseInt(q.Get("before"), 10, 64); b > 0 {
		where, args = append(where, "s.id < ?"), append(args, b)
	}
	if text := strings.TrimSpace(q.Get("q")); text != "" {
		if len(text) > 200 {
			text = text[:200]
		}
		like := "%" + likeEscape(strings.ToLower(text)) + "%"
		where = append(where, `(LOWER(s.title) LIKE ? ESCAPE '\' OR LOWER(s.conn_name) LIKE ? ESCAPE '\' OR LOWER(s.host) LIKE ? ESCAPE '\'
			OR LOWER(s.model) LIKE ? ESCAPE '\' OR LOWER(s.provider_name) LIKE ? ESCAPE '\'
			OR EXISTS (SELECT 1 FROM ai_messages m WHERE m.session_id = s.id AND m.kind IN ('user','assistant','tool_call') AND LOWER(m.content) LIKE ? ESCAPE '\'))`)
		for i := 0; i < 6; i++ {
			args = append(args, like)
		}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	list := aiListSessions(strings.Join(where, " AND "), args, limit)
	for _, s := range list {
		id, _ := s["id"].(int64)
		var saved int
		var preview string
		db.QueryRow(`SELECT saved FROM ai_sessions WHERE id=?`, id).Scan(&saved)
		// the first question, or for an AI app the first thing it did
		db.QueryRow(`SELECT content FROM ai_messages WHERE session_id=? AND kind IN ('user','tool_call') ORDER BY CASE kind WHEN 'user' THEN 0 ELSE 1 END, seq LIMIT 1`, id).Scan(&preview)
		s["saved"] = saved == 1
		s["preview"] = truncateStr(strings.Join(strings.Fields(preview), " "), 160)
		s["kind"] = aiSessionKindOf(s)
		s["can_continue"] = !s["live"].(bool) && s["conn_id"].(int64) > 0
	}
	more := len(list) == limit
	jsonOK(w, map[string]interface{}{"sessions": list, "more": more, "keep_saved": settingBool("ai_keep_saved_sessions"),
		"retention_days": settingInt("ai_transcript_retention_days")})
}

func aiSessionKindOf(s map[string]interface{}) string {
	switch {
	case s["transport"] == "mcp":
		return "mcp"
	case s["transport"] == "opencode":
		return "opencode"
	case s["purpose"] == aiPurposeLogs:
		return "logs"
	case s["purpose"] == aiPurposeFile:
		return "file"
	}
	return "assistant"
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// aiSaveHandler marks an own session as saved (or not): POST {saved}.
func aiSaveHandler(w http.ResponseWriter, r *http.Request, userID int, idStr string) {
	id, _ := strconv.ParseInt(idStr, 10, 64)
	var in struct {
		Saved bool `json:"saved"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	res, err := db.Exec(`UPDATE ai_sessions SET saved=? WHERE id=? AND user_id=?`, boolInt(in.Saved), id, userID)
	if err != nil {
		jsonError(w, "Could not save", 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "AI session not found", 404)
		return
	}
	auditLog(r, userID, "ai.session_saved", "", map[string]interface{}{"ai_session": id, "saved": in.Saved})
	hub.sendTo(userID, jsonMarshal(map[string]interface{}{"type": "ai_sessions_changed"}))
	jsonOK(w, map[string]interface{}{"ok": true, "saved": in.Saved})
}

// aiExportHandler downloads an own session's transcript (already redacted when stored).
func aiExportHandler(w http.ResponseWriter, r *http.Request, userID int, idStr string) {
	id, _ := strconv.ParseInt(idStr, 10, 64)
	list := aiListSessions(`s.id = ? AND s.user_id = ?`, []interface{}{id, userID}, 1)
	if len(list) == 0 {
		jsonError(w, "AI session not found", 404)
		return
	}
	sess := list[0]
	type msg struct {
		Seq     int64                  `json:"seq"`
		TS      string                 `json:"ts"`
		Kind    string                 `json:"kind"`
		Actor   string                 `json:"actor"`
		Content string                 `json:"content"`
		Meta    map[string]interface{} `json:"meta,omitempty"`
	}
	msgs := []msg{}
	rows, err := db.Query(`SELECT seq, ts, kind, actor, content, meta FROM ai_messages WHERE session_id=? ORDER BY seq, id`, id)
	if err == nil {
		for rows.Next() {
			var m msg
			var meta string
			if rows.Scan(&m.Seq, &m.TS, &m.Kind, &m.Actor, &m.Content, &meta) != nil {
				continue
			}
			if meta != "" {
				json.Unmarshal([]byte(meta), &m.Meta)
			}
			msgs = append(msgs, m)
		}
		rows.Close()
	}
	format := r.URL.Query().Get("format")
	auditLog(r, userID, "ai.session_export", fmt.Sprint(sess["connection"]), map[string]interface{}{"ai_session": id, "format": nonEmpty(format, "md")})
	name := fmt.Sprintf("wrm-ai-session-%d", id)
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.json"`)
		json.NewEncoder(w).Encode(map[string]interface{}{"session": sess, "messages": msgs, "exported_at": time.Now().UTC().Format(time.RFC3339), "wrm_version": AppVersion})
		return
	}
	var b strings.Builder
	title := fmt.Sprint(sess["title"])
	if title == "" {
		title = fmt.Sprintf("AI session %d", id)
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- Server: %v (%v)\n- Started: %v\n- Ended: %v\n- %v / %v, mode %v\n- Tokens in/out: %v / %v\n\n",
		sess["connection"], sess["host"], sess["started_at"], nonEmpty(fmt.Sprint(sess["ended_at"]), "-"), sess["provider"], sess["model"], sess["mode"], sess["tokens_in"], sess["tokens_out"])
	heads := map[string]string{"user": "🧑 User", "assistant": "✦ Assistant", "tool_call": "🛠 Tool call", "tool_result": "📄 Result",
		"approval": "⏳ Approval", "decision": "⚖ Decision", "mode": "⚙ Mode", "error": "⚠ Error", "system": "ℹ System", "end": "⏻ End"}
	for _, m := range msgs {
		fmt.Fprintf(&b, "## %s · %s · %s\n\n", nonEmpty(heads[m.Kind], m.Kind), m.Actor, m.TS)
		switch m.Kind {
		case "user", "assistant":
			b.WriteString(m.Content + "\n\n")
		default:
			fence := "```"
			for strings.Contains(m.Content, fence) {
				fence += "`"
			}
			b.WriteString(fence + "\n" + m.Content + "\n" + fence + "\n\n")
		}
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.md"`)
	w.Write([]byte(b.String()))
}

// ── personal AI settings ────────────────────────────

type aiUserPrefs struct {
	ProviderID int    `json:"provider_id"`
	Model      string `json:"model"`
	Mode       string `json:"mode"`
	Engine     string `json:"engine"`
	Thinking   *bool  `json:"thinking"`
}

// aiPrefsOf reads the user's personal AI settings (empty when none are stored).
func aiPrefsOf(userID int) aiUserPrefs {
	var p aiUserPrefs
	var v string
	if db.QueryRow(`SELECT value FROM user_ui WHERE user_id=? AND key='ai'`, userID).Scan(&v) == nil {
		json.Unmarshal([]byte(v), &p)
	}
	return p
}

// aiDefaultProviderFor is the provider a session uses when none is given: the personal
// default if the user may still use it, else the first one offered.
func aiDefaultProviderFor(userID int) int {
	list := providersFor(userID)
	pref := aiPrefsOf(userID).ProviderID
	for _, p := range list {
		if id, _ := p["id"].(int); id == pref && pref > 0 {
			return id
		}
	}
	if len(list) > 0 {
		id, _ := list[0]["id"].(int)
		return id
	}
	return 0
}

// aiThinkingWanted: the policy allows showing the model's thinking and the user did not turn it off.
func aiThinkingWanted(userID int) bool {
	if !settingBool("ai_show_thinking") {
		return false
	}
	p := aiPrefsOf(userID)
	return p.Thinking == nil || *p.Thinking
}
