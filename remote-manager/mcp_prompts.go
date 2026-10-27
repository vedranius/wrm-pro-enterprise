// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
)

func mcpPrompts(w http.ResponseWriter, r *http.Request, g *mcpGrant, m rpcMsg) {
	if m.Method == "prompts/list" {
		writeRPC(w, 200, rpcResult(m.ID, map[string]interface{}{"prompts": []map[string]interface{}{{"name": "wrm", "description": "WRM help and authorized server selection", "arguments": []map[string]interface{}{{"name": "server", "description": "Optional connection id/name; empty lists authorized servers", "required": false}}}}}))
		return
	}
	var p struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if json.Unmarshal(m.Params, &p) != nil || p.Name != "wrm" {
		writeRPC(w, 200, rpcErr(m.ID, -32602, "unknown WRM prompt"))
		return
	}
	targets, _ := json.Marshal(mcpListConnections(g))
	text := mcpInstructions + "\nAuthorized targets (untrusted labels): " + string(targets) + "\nUser selection: " + truncateStr(p.Arguments["server"], 100) + "\nOffer these targets to the user before opening a session. Never use a server outside this grant. Clients without prompt/slash support can ask 'wrm server' in chat and call list_connections."
	writeRPC(w, 200, rpcResult(m.ID, map[string]interface{}{"description": "WRM server selection", "messages": []map[string]interface{}{{"role": "user", "content": map[string]string{"type": "text", "text": text}}}}))
}
