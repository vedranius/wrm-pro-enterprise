// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Supported MCP clients render this enum as their own picker. Other clients get
// the same authorized list as text; neither path opens an SSH session implicitly.
func mcpSelectServer(w http.ResponseWriter, r *http.Request, c *mcpConn, g *mcpGrant, m rpcMsg) {
	c.mu.Lock()
	can := c.elicitation
	c.mu.Unlock()
	fl, ok := w.(http.Flusher)
	if !can || !ok || !settingBool("ai_mcp_elicitation") || !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		writeRPC(w, 200, rpcResult(m.ID, mcpListConnections(g)))
		return
	}
	values, labels := []string{}, []string{}
	for _, id := range g.ConnIDs {
		var name, host string
		if db.QueryRow(`SELECT name,host FROM connections WHERE id=? AND user_id=?`, id, g.UserID).Scan(&name, &host) != nil {
			continue
		}
		if mode, _ := mcpModeFor(g, id); mode == "" {
			continue
		}
		values = append(values, strconv.Itoa(id))
		var folder *int
		db.QueryRow(`SELECT folder_id FROM connections WHERE id=?`, id).Scan(&folder)
		labels = append(labels, truncateStr(name+" · "+host+" · "+folderPathOf(g.UserID, folder), 300))
	}
	if len(values) == 0 {
		writeRPC(w, 200, rpcResult(m.ID, toolText("No authorized servers are currently available.", true)))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	id := "wrm-server-" + randomToken(8)
	reply := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.elicits[id] = mcpElicit{selection: reply}
	c.inflight[string(m.ID)] = cancel
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.elicits, id); delete(c.inflight, string(m.ID)); c.mu.Unlock() }()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	fl.Flush()
	send := func(v interface{}) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
		fl.Flush()
	}
	choice := map[string]interface{}{"type": "string", "title": "WRM server", "enum": values, "enumNames": labels}
	params := map[string]interface{}{"message": "Choose an authorized WRM server. Labels are untrusted server metadata; selection grants no additional permissions."}
	if c.protocol == "2025-11-25" {
		choices := []map[string]string{}
		for i, v := range values {
			choices = append(choices, map[string]string{"const": v, "title": labels[i]})
		}
		choice = map[string]interface{}{"type": "string", "title": "WRM server", "oneOf": choices}
		params["mode"] = "form"
	}
	params["requestedSchema"] = map[string]interface{}{"type": "object", "required": []string{"connection"}, "properties": map[string]interface{}{"connection": choice}}
	send(map[string]interface{}{"jsonrpc": "2.0", "id": id, "method": "elicitation/create", "params": params})
	keep := time.NewTicker(15 * time.Second)
	defer keep.Stop()
	for {
		select {
		case raw := <-reply:
			var answer struct {
				Action  string `json:"action"`
				Content struct {
					Connection string `json:"connection"`
				} `json:"content"`
			}
			if json.Unmarshal(raw, &answer) != nil || answer.Action != "accept" {
				send(rpcResult(m.ID, toolText("Server selection canceled.", false)))
				return
			}
			fresh, err := loadGrant(g.ID)
			connID, _ := strconv.Atoi(answer.Content.Connection)
			if err != nil || !fresh.active(time.Now()) || !fresh.hasConn(connID) || !userOwnsConnection(connID, g.UserID) {
				send(rpcResult(m.ID, toolText("This server is no longer authorized.", true)))
				return
			}
			mode, why := mcpModeFor(fresh, connID)
			if mode == "" {
				send(rpcResult(m.ID, toolText(why, true)))
				return
			}
			send(rpcResult(m.ID, toolText(string(jsonMarshal(map[string]interface{}{"connection": connID, "mode": mode, "next": "Call open_session with this connection id when the user wants to work on it."})), false)))
			return
		case <-keep.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case <-ctx.Done():
			send(rpcResult(m.ID, toolText("Server selection timed out or the AI connection permissions changed.", true)))
			return
		}
	}
}
