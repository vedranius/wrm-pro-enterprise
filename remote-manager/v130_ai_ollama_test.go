// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Ollama's OpenAI-compatible endpoint rejects an assistant message whose
// content is null: "invalid message content type: <nil>". A turn that only
// calls tools has no text, and every provider accepts an empty string, so the
// request builder must never emit null.
func TestV130OllamaEmptyAssistantContentIsAString(t *testing.T) {
	msgs := openaiMessages("system prompt", []aiMsg{
		{Role: "user", Text: "check the disk"},
		{Role: "assistant", Calls: []aiToolCall{{ID: "call_1", Name: "read_log", Input: json.RawMessage(`{"service":"nginx"}`)}}},
		{Role: "assistant", Results: []aiToolResult{{ID: "call_1", Name: "read_log", Content: "ok"}}},
		{Role: "assistant", Text: "All good."},
	})
	raw, err := json.Marshal(msgs)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"content":null`) {
		t.Fatalf("a message carries null content: %s", raw)
	}
	seen := 0
	for _, m := range msgs {
		c, ok := m["content"].(string)
		if !ok {
			t.Fatalf("%v content is %T, not a string", m["role"], m["content"])
		}
		if m["role"] == "assistant" {
			seen++
			if len(m) > 0 && c == "" {
				// an empty string is exactly what Ollama needs here
				continue
			}
		}
	}
	if seen < 2 {
		t.Fatalf("the tool-call turns were not included: %s", raw)
	}
	// The tool call itself must still be there next to the empty content.
	if !strings.Contains(string(raw), `"tool_calls"`) || !strings.Contains(string(raw), fmt.Sprintf("%q", "read_log")) {
		t.Fatalf("the tool call was lost: %s", raw)
	}
}

// The assistant chat shows one collapsible work log per request: open while the
// model works, listing each tool step, collapsed when the answer is there.
func TestV130AssistantWorkLog(t *testing.T) {
	js, _ := os.ReadFile("static/ai.js")
	p := string(js)
	for _, want := range []string{
		"data-worklog", "function workLog(", "function workLogStep(", "function workLogDone(",
		"ai_worklog_working", "workLogStep(id,", "workLogDone(id)", "workLog(id); break;",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("ai.js lacks %s", want)
		}
	}
	if strings.Contains(p, "ai-wait-") {
		t.Error("the standalone working note should be replaced by the work log")
	}
}
