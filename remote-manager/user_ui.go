// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// Per-user interface state that follows the user to every browser: the panel layout
// (v12.6.8) and the Workboard. The server only stores it; the page decides what it means.
//
// GET /api/me/ui/{key}   → {"value": <stored JSON or null>, "updated_at": "…"}
// PUT /api/me/ui/{key}   body {"value": <JSON object>}; DELETE resets it.

// "ai" holds the personal AI settings (ai_sessions_view.go).
var userUIKeys = map[string]bool{"layout": true, "workboard": true, "appearance": true, "lock": true, "ai": true}

// v12.6.10: 256 KiB, the Workboard also keeps saved canvas layouts.
const userUIMaxBytes = 256 << 10

func apiUserUIHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireAuth(w, r)
	if !ok {
		return
	}
	key := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/me/ui/"), "/")
	if !userUIKeys[key] {
		jsonError(w, "Not found", 404)
		return
	}
	switch r.Method {
	case http.MethodGet:
		var v, at string
		db.QueryRow(`SELECT value, updated_at FROM user_ui WHERE user_id=? AND key=?`, uid, key).Scan(&v, &at)
		out := map[string]interface{}{"value": nil, "updated_at": at}
		if v != "" {
			out["value"] = json.RawMessage(v)
		}
		jsonOK(w, out)
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, userUIMaxBytes+1))
		if err != nil || len(body) > userUIMaxBytes {
			jsonError(w, "Too large (256 KiB at most)", 400)
			return
		}
		var req struct {
			Value json.RawMessage `json:"value"`
		}
		var obj map[string]interface{}
		if json.Unmarshal(body, &req) != nil || json.Unmarshal(req.Value, &obj) != nil || obj == nil {
			jsonError(w, "value must be a JSON object", 400)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := db.Exec(`INSERT INTO user_ui (user_id, key, value, updated_at) VALUES (?,?,?,?)
			ON CONFLICT(user_id, key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, uid, key, string(req.Value), now); err != nil {
			jsonError(w, "Cannot save", 500)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true, "updated_at": now})
	case http.MethodDelete:
		db.Exec(`DELETE FROM user_ui WHERE user_id=? AND key=?`, uid, key)
		jsonOK(w, map[string]interface{}{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}
