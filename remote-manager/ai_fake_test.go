// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// ─── fake model providers ────────────────────────────
//
// fakeLLM answers Anthropic Messages (/v1/messages) and OpenAI Chat Completions
// (/chat/completions) requests with streamed responses from a script: each turn is
// text plus tool calls; the tool arguments are streamed in several chunks.

type fakeCall struct {
	Name  string
	Input string // JSON
}

type fakeTurn struct {
	Text  string
	Calls []fakeCall
	Stop  string // override the stop reason
}

type fakeLLM struct {
	mu      sync.Mutex
	srv     *httptest.Server
	script  func(n int, body map[string]interface{}) fakeTurn
	bodies  []map[string]interface{}
	headers []http.Header
	paths   []string
}

func newFakeLLM(t *testing.T, script func(n int, body map[string]interface{}) fakeTurn) *fakeLLM {
	f := &fakeLLM{script: script}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLLM) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

func (f *fakeLLM) body(i int) map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[i]
}

func (f *fakeLLM) rawBody(i int) string {
	b, _ := json.Marshal(f.body(i))
	return string(b)
}

func chunks(s string, n int) []string {
	var out []string
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	return append(out, s)
}

func (f *fakeLLM) serve(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	var body map[string]interface{}
	json.Unmarshal(b, &body)
	f.mu.Lock()
	n := len(f.bodies)
	f.bodies = append(f.bodies, body)
	f.headers = append(f.headers, r.Header.Clone())
	f.paths = append(f.paths, r.URL.RequestURI())
	f.mu.Unlock()
	turn := fakeTurn{Text: "done"}
	if f.script != nil {
		turn = f.script(n, body)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fl := w.(http.Flusher)
	send := func(event string, v interface{}) {
		d, _ := json.Marshal(v)
		if event != "" {
			fmt.Fprintf(w, "event: %s\n", event)
		}
		fmt.Fprintf(w, "data: %s\n\n", d)
		fl.Flush()
	}
	if strings.HasSuffix(r.URL.Path, "/chat/completions") {
		if turn.Text != "" {
			for _, c := range chunks(turn.Text, 7) {
				send("", map[string]interface{}{"choices": []interface{}{map[string]interface{}{"index": 0, "delta": map[string]interface{}{"content": c}}}})
			}
		}
		for i, c := range turn.Calls {
			send("", map[string]interface{}{"choices": []interface{}{map[string]interface{}{"index": 0, "delta": map[string]interface{}{
				"tool_calls": []interface{}{map[string]interface{}{"index": i, "id": fmt.Sprintf("call_%d_%d", n, i), "type": "function", "function": map[string]interface{}{"name": c.Name, "arguments": ""}}}}}}})
			for _, part := range chunks(c.Input, 5) {
				send("", map[string]interface{}{"choices": []interface{}{map[string]interface{}{"index": 0, "delta": map[string]interface{}{
					"tool_calls": []interface{}{map[string]interface{}{"index": i, "function": map[string]interface{}{"arguments": part}}}}}}})
			}
		}
		fin := "stop"
		if len(turn.Calls) > 0 {
			fin = "tool_calls"
		}
		send("", map[string]interface{}{"choices": []interface{}{map[string]interface{}{"index": 0, "delta": map[string]interface{}{}, "finish_reason": fin}}})
		send("", map[string]interface{}{"choices": []interface{}{}, "usage": map[string]interface{}{"prompt_tokens": 120, "completion_tokens": 30}})
		fmt.Fprintf(w, "data: [DONE]\n\n")
		return
	}
	// Anthropic Messages
	send("message_start", map[string]interface{}{"type": "message_start", "message": map[string]interface{}{"id": "msg_1", "role": "assistant",
		"usage": map[string]interface{}{"input_tokens": 100, "output_tokens": 1, "cache_read_input_tokens": 50}}})
	idx := 0
	// a thinking block that must be passed back unchanged
	send("content_block_start", map[string]interface{}{"type": "content_block_start", "index": idx, "content_block": map[string]interface{}{"type": "thinking", "thinking": ""}})
	if body["thinking"] != nil {
		send("content_block_delta", map[string]interface{}{"type": "content_block_delta", "index": idx, "delta": map[string]interface{}{"type": "thinking_delta", "thinking": "Looking at the request."}})
	}
	send("content_block_delta", map[string]interface{}{"type": "content_block_delta", "index": idx, "delta": map[string]interface{}{"type": "signature_delta", "signature": fmt.Sprintf("sig-%d", n)}})
	send("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": idx})
	idx++
	if turn.Text != "" {
		send("content_block_start", map[string]interface{}{"type": "content_block_start", "index": idx, "content_block": map[string]interface{}{"type": "text", "text": ""}})
		for _, c := range chunks(turn.Text, 7) {
			send("content_block_delta", map[string]interface{}{"type": "content_block_delta", "index": idx, "delta": map[string]interface{}{"type": "text_delta", "text": c}})
		}
		send("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": idx})
		idx++
	}
	for i, c := range turn.Calls {
		send("content_block_start", map[string]interface{}{"type": "content_block_start", "index": idx, "content_block": map[string]interface{}{
			"type": "tool_use", "id": fmt.Sprintf("toolu_%d_%d", n, i), "name": c.Name, "input": map[string]interface{}{}}})
		for _, part := range chunks(c.Input, 5) {
			send("content_block_delta", map[string]interface{}{"type": "content_block_delta", "index": idx, "delta": map[string]interface{}{"type": "input_json_delta", "partial_json": part}})
		}
		send("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": idx})
		idx++
	}
	stop := "end_turn"
	if len(turn.Calls) > 0 {
		stop = "tool_use"
	}
	if turn.Stop != "" {
		stop = turn.Stop
	}
	send("message_delta", map[string]interface{}{"type": "message_delta", "delta": map[string]interface{}{"stop_reason": stop}, "usage": map[string]interface{}{"output_tokens": 40}})
	send("message_stop", map[string]interface{}{"type": "message_stop"})
}

// lastToolResults returns the tool_result contents of the last user message of a
// request (Anthropic or OpenAI format).
func lastToolResults(body map[string]interface{}) []string {
	msgs, _ := body["messages"].([]interface{})
	var out []string
	for i := len(msgs) - 1; i >= 0; i-- {
		m, _ := msgs[i].(map[string]interface{})
		if m["role"] == "tool" {
			out = append([]string{fmt.Sprint(m["content"])}, out...)
			continue
		}
		if len(out) > 0 {
			return out
		}
		if m["role"] != "user" {
			return out
		}
		blocks, _ := m["content"].([]interface{})
		for _, b := range blocks {
			bm, _ := b.(map[string]interface{})
			if bm["type"] == "tool_result" {
				out = append(out, fmt.Sprint(bm["content"]))
			}
		}
		return out
	}
	return out
}
