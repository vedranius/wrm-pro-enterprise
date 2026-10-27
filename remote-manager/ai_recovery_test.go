// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Gemini's OpenAI-compatible endpoint returns a thought_signature in extra_content
// of each tool call and rejects the next request when it is missing.
func TestOpenAIToolCallExtraContentRoundTrip(t *testing.T) {
	var sent []byte
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 2 {
			sent, _ = readAllBody(r)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"run_command","arguments":"{\"command\":\"uptime\"}"},"extra_content":{"google":{"thought_signature":"SIG123"}}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	p := aiProvider{Kind: "openai_compatible", BaseURL: srv.URL}
	req := aiLLMRequest{Model: "gemini", Messages: []aiMsg{{Role: "user", Text: "hi"}}}
	res, err := aiComplete(context.Background(), p, req, nil)
	if err != nil || len(res.Calls) != 1 || !strings.Contains(string(res.Calls[0].Extra), "SIG123") {
		t.Fatalf("%v %+v", err, res)
	}
	req.Messages = append(req.Messages, aiMsg{Role: "assistant", Calls: res.Calls},
		aiMsg{Role: "user", Results: []aiToolResult{{ID: "call_1", Name: "run_command", Content: "ok"}}})
	if _, err := aiComplete(context.Background(), p, req, nil); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Messages []struct {
			ToolCalls []struct {
				Extra json.RawMessage `json:"extra_content"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(sent, &body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range body.Messages {
		for _, c := range m.ToolCalls {
			found = found || strings.Contains(string(c.Extra), "SIG123")
		}
	}
	if !found {
		t.Fatalf("thought_signature was not sent back: %s", sent)
	}
}

func readAllBody(r *http.Request) ([]byte, error) {
	var b []byte
	buf := make([]byte, 4096)
	for {
		k, err := r.Body.Read(buf)
		b = append(b, buf[:k]...)
		if err != nil {
			return b, nil
		}
	}
}

func TestAIErrorClassification(t *testing.T) {
	if !aiRetryable(&aiAPIError{Status: 503}) || aiRetryable(&aiAPIError{Status: 401}) {
		t.Fatal("retryable statuses")
	}
	if !aiTooLarge(&aiAPIError{Status: 413}) || !aiTooLarge(&aiAPIError{Status: 400, Msg: "Prompt is too long"}) || aiTooLarge(&aiAPIError{Status: 401, Msg: "request too large"}) {
		t.Fatal("too large detection")
	}
}

func TestAIShrinkHistory(t *testing.T) {
	big := strings.Repeat("é", 5000)
	msgs := []aiMsg{{Role: "user", Results: []aiToolResult{{ID: "a", Content: big}, {ID: "b", Content: "short"}}}}
	if !aiShrinkHistory(msgs, 1000) {
		t.Fatal("expected a change")
	}
	got := msgs[0].Results[0].Content
	if len(got) > 1200 || !strings.Contains(got, "shortened") || msgs[0].Results[1].Content != "short" {
		t.Fatalf("%d %q", len(got), got[:40])
	}
	if aiShrinkHistory(msgs, 1000) {
		t.Fatal("already short")
	}
}

func TestOpenAIInlineToolCalls(t *testing.T) {
	tools := []aiToolDef{{Name: "system_info"}, {Name: "list_directory"}}
	for _, tc := range []struct {
		name, text, want string
		calls            int
	}{
		{"single", `{"name":"system_info","arguments":{}}`, "", 1},
		{"mixed-multiple", "Checking. {\n \"name\" : \"system_info\", \"arguments\": {}} then {\"name\":\"list_directory\",\"arguments\":{\"path\":\"/tmp/a}b\"}}", "Checking.  then", 2},
		{"fenced", "```json\n{\"name\":\"system_info\",\"arguments\":{}}\n```", "", 1},
		{"incomplete", `Result: {"name":"system_info","arguments":{"date":"2026-10-13"}`, `Result: {"name":"system_info","arguments":{"date":"2026-10-13"}`, 0},
		{"repairable-but-truncated", `{"name":"system_info","arguments":{`, `{"name":"system_info","arguments":{`, 0},
		{"ordinary-json-answer", `{"name":"system_info","date":"2026-10-13"}`, `{"name":"system_info","date":"2026-10-13"}`, 0},
		{"unoffered", `{"name":"edit_file","arguments":{}}`, `{"name":"edit_file","arguments":{}}`, 0},
		{"null-args", `{"name":"system_info","arguments":null}`, `{"name":"system_info","arguments":null}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, calls := extractInlineToolCalls(tc.text, tools)
			if text != tc.want || len(calls) != tc.calls {
				t.Fatalf("text %q (want %q), calls %d (want %d)", text, tc.want, len(calls), tc.calls)
			}
		})
	}
	if _, bad := parseToolInput("null"); bad == "" {
		t.Fatal("null tool arguments accepted as a JSON object")
	}
}

func TestOpenAIInlineToolLoop(t *testing.T) {
	e := newAIEnv(t, func(n int, body map[string]interface{}) fakeTurn {
		if n == 0 {
			return fakeTurn{Text: `Checking. {"name":"system_info","arguments":{}} {"name":"list_directory","arguments":{"path":"~"}}`}
		}
		return fakeTurn{Text: "The tools returned actual server data."}
	})
	code, v := e.admin.aiDo("POST", "/api/admin/ai/providers", map[string]interface{}{"name": "Inline local model", "kind": "openai_compatible", "base_url": e.llm.srv.URL + "/v1", "models": "test-model"})
	if code != 200 {
		t.Fatalf("provider: %d %v", code, v)
	}
	e.provID = int(asMap(v)["id"].(float64))
	s := e.start("read_only", nil)
	e.prompt(s, "Check the server")
	e.waitIdle(s)
	if e.llm.requests() != 2 || len(lastToolResults(e.llm.body(1))) != 2 {
		t.Fatal("inline tool calls did not complete the tool loop")
	}
	events, _, _ := s.eventsSince(0)
	var done []string
	for _, event := range events {
		if event.Type == "assistant_done" {
			done = append(done, fmt.Sprint(event.Data["text"]))
		}
	}
	if len(done) != 2 || done[0] != "Checking." || strings.Contains(strings.Join(done, " "), `"arguments"`) {
		t.Fatalf("final assistant messages were not cleaned: %v", done)
	}
}

func TestOpenAITruncatedTextDoesNotRun(t *testing.T) {
	var command, malformed string
	e := newAIEnv(t, func(n int, body map[string]interface{}) fakeTurn {
		if n == 0 {
			return fakeTurn{Text: malformed}
		}
		return fakeTurn{Text: "done"}
	})
	path := e.host.home + "/must-not-exist"
	command = "touch " + path
	// The old fallback appended two braces and executed this truncated response.
	malformed = `{"name":"run_command","arguments":{"command":"` + command + `"`
	code, v := e.admin.aiDo("POST", "/api/admin/ai/providers", map[string]interface{}{"name": "Truncated local model", "kind": "openai_compatible", "base_url": e.llm.srv.URL + "/v1", "models": "test-model"})
	if code != 200 {
		t.Fatalf("provider: %d %v", code, v)
	}
	e.provID = int(asMap(v)["id"].(float64))
	s := e.start("auto", map[string]interface{}{"allow": []string{command}, "minutes": 1, "max_actions": 1})
	e.prompt(s, "Check this response")
	e.waitIdle(s)
	if aiFileExists(path) || e.llm.requests() != 1 || s.toolCalls != 0 {
		t.Fatal("a truncated text response triggered a command")
	}
	events, _, _ := s.eventsSince(0)
	for _, event := range events {
		if event.Type == "assistant_done" && event.Data["text"] != malformed {
			t.Fatal("malformed model output was silently discarded")
		}
	}
}
