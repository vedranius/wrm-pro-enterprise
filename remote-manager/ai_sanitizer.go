// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
)

// SanitizeAIHistory sprječava da model upadne u sirovi JSON loop nakon učitavanja povijesti iz baze
func SanitizeAIHistory(messages []map[string]interface{}) []map[string]interface{} {
	var clean []map[string]interface{}
	for _, m := range messages {
		content, ok := m["content"].(string)
		role, _ := m["role"].(string)
		if ok && role == "assistant" {
			trimmed := strings.TrimSpace(content)
			if strings.HasPrefix(trimmed, `{"name":`) && strings.Contains(trimmed, `"arguments":`) {
				var parsed struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}
				if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil && parsed.Name != "" {
					m["content"] = ""
					m["tool_calls"] = []map[string]interface{}{
						{
							"id":   "call_" + parsed.Name,
							"type": "function",
							"function": map[string]interface{}{
								"name":      parsed.Name,
								"arguments": string(parsed.Arguments),
							},
						},
					}
				}
			}
		}
		clean = append(clean, m)
	}
	return clean
}
