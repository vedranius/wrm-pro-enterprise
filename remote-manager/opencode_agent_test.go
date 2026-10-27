// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The OpenCode configuration: one provider, WRM as the only MCP server, the host tools off,
// and no secret in the JSON (keys travel in the environment).
func TestOpencodeConfig(t *testing.T) {
	p := aiProvider{Kind: "openai_compatible", Name: "Local", BaseURL: "http://127.0.0.1:1/v1"}
	p.secret.APIKey = "sk-local-secret"
	b, pid, model, env, err := opencodeConfig(p, "local-model", "http://127.0.0.1:2/mcp", "wrm_pat_abc", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if pid != "wrm" || model != "local-model" || strings.Contains(s, "sk-local-secret") || strings.Contains(s, "wrm_pat_abc") {
		t.Fatalf("config leaks or wrong ids: %s %s %s", pid, model, s)
	}
	var cfg map[string]interface{}
	json.Unmarshal(b, &cfg)
	tools := cfg["tools"].(map[string]interface{})
	for _, k := range []string{"bash", "read", "edit", "write", "webfetch", "grep", "glob"} {
		if tools[k] != false {
			t.Fatalf("tool %s not off: %v", k, tools)
		}
	}
	mcp := cfg["mcp"].(map[string]interface{})["wrm"].(map[string]interface{})
	if mcp["url"] != "http://127.0.0.1:2/mcp" || !strings.Contains(fmt.Sprint(mcp["headers"]), "{env:WRM_OPENCODE_MCP_TOKEN}") {
		t.Fatalf("mcp: %v", mcp)
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "WRM_OPENCODE_KEY=sk-local-secret") || !strings.Contains(joined, "WRM_OPENCODE_MCP_TOKEN=wrm_pat_abc") {
		t.Fatalf("env: %v", env)
	}
	if _, _, _, _, err := opencodeConfig(aiProvider{Kind: "bedrock"}, "x", "u", "t", false); err == nil {
		t.Fatal("bedrock should be refused")
	}
	// the base environment carries nothing of WRM's own
	t.Setenv("WRM_SECRET_THING", "x")
	for _, kv := range opencodeBaseEnv(t.TempDir()) {
		if strings.HasPrefix(kv, "WRM_") {
			t.Fatalf("WRM variable passed to OpenCode: %s", kv)
		}
	}
}

// The installer verifies the registry's sha512 before it unpacks anything.
func TestOpencodeInstallVerifiesIntegrity(t *testing.T) {
	if opencodePackage() == "" {
		t.Skip("no OpenCode build for this platform")
	}
	var tgz bytes.Buffer
	gz := gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gz)
	script := "#!/bin/sh\necho 9.9.9\n"
	tw.WriteHeader(&tar.Header{Name: "package/bin/" + opencodeBinName(), Mode: 0o755, Size: int64(len(script)), Typeflag: tar.TypeReg})
	tw.Write([]byte(script))
	tw.Close()
	gz.Close()
	sum := sha512.Sum512(tgz.Bytes())
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
	good := true
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tgz") {
			w.Write(tgz.Bytes())
			return
		}
		in := integrity
		if !good {
			in = "sha512-AAAA"
		}
		fmt.Fprintf(w, `{"version":"9.9.9","license":"MIT","dist":{"tarball":"%s/pkg.tgz","integrity":"%s"}}`, srv.URL, in)
	}))
	defer srv.Close()
	old := aiHTTPClient
	aiHTTPClient = srv.Client()
	defer func() { aiHTTPClient = old }()
	newTestServer(t) // database and settings
	if _, err := setSetting("ai_opencode_registry", srv.URL); err != nil {
		t.Fatal(err)
	}
	setSetting("ai_opencode_version", "9.9.9")
	good = false
	if err := opencodeInstall("tester"); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("a wrong checksum must stop the install: %v", err)
	}
	if _, ok := opencodeLoadState(); ok {
		t.Fatal("installed despite a wrong checksum")
	}
	if runtime.GOOS == "windows" {
		return // the fake binary is a shell script
	}
	good = true
	if err := opencodeInstall("tester"); err != nil {
		t.Fatal(err)
	}
	st, ok := opencodeLoadState()
	if !ok || st.Version != "9.9.9" || st.InstalledBy != "tester" || !strings.HasPrefix(st.Path, opencodeRoot()) {
		t.Fatalf("state: %+v %v", st, ok)
	}
	if p, v, err := opencodeBinary(); err != nil || p != st.Path || v != "9.9.9" {
		t.Fatalf("binary: %s %s %v", p, v, err)
	}
}

// OpenCode's internal AI connections only work on the loopback listener, and that listener
// accepts nothing else.
func TestOpencodeLoopbackTokens(t *testing.T) {
	t.Setenv("WRM_AI_ASSISTANT", "all")
	e := newMCPEnv(t)
	pat, _ := e.pat("read_only", []string{"read_logs"}, nil)
	q := mcpGrantRequest{ConnIDs: []int{e.connID}, Mode: "read_only", Scopes: []string{"read_logs"}, kind: "opencode"}
	if err := q.validate(e.user.userID); err != nil {
		t.Fatal(err)
	}
	_, octok, err := mcpCreateGrant(nil, e.user.userID, q, "opencode", "", "OpenCode (WRM)")
	if err != nil {
		t.Fatal(err)
	}
	loop, err := opencodeMCPURL()
	if err != nil {
		t.Fatal(err)
	}
	loopBase := strings.TrimSuffix(loop, "/mcp")
	init := map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{"protocolVersion": "2025-06-18", "capabilities": map[string]interface{}{}, "clientInfo": map[string]string{"name": "t", "version": "1"}}}
	try := func(base, tok string) int {
		c := newMCPClient(t, base, tok)
		code, _, _ := c.post(init)
		return code
	}
	if c := try(e.srv, octok); c != 401 {
		t.Fatalf("OpenCode token on the public endpoint: %d", c)
	}
	if c := try(loopBase, pat); c != 401 {
		t.Fatalf("personal token on the loopback listener: %d", c)
	}
	if c := try(loopBase, octok); c != 200 {
		t.Fatalf("OpenCode token on the loopback listener: %d", c)
	}
	// it works even with MCP for external clients turned off
	t.Setenv("WRM_AI_MCP_ENABLED", "0")
	if c := try(loopBase, octok); c != 200 {
		t.Fatalf("loopback with external MCP off: %d", c)
	}
}

// A real OpenCode agent (WRM_TEST_OPENCODE=/path/to/opencode) runs a WRM tool through the
// AI connection: the call is shown in the chat, the answer streams, ending the session stops
// the process, revokes the AI connection and removes its directory.
func TestOpencodeAgentEndToEnd(t *testing.T) {
	bin := os.Getenv("WRM_TEST_OPENCODE")
	if bin == "" {
		t.Skip("set WRM_TEST_OPENCODE to an opencode binary to run this test")
	}
	t.Setenv("WRM_AI_OPENCODE_PATH", bin)
	sawTool := false
	e := newAIEnv(t, nil)
	e.llm.script = func(n int, body map[string]interface{}) fakeTurn {
		var names []string
		for _, tl := range asSlice(body["tools"]) {
			names = append(names, fmt.Sprint(asMap(asMap(tl)["function"])["name"]))
		}
		msgs := asSlice(body["messages"])
		last := asMap(msgs[len(msgs)-1])
		if last["role"] == "tool" {
			return fakeTurn{Text: "The server runs Linux; checked through WRM."}
		}
		for _, nm := range names {
			if nm == "wrm_system_info" {
				sawTool = true
				sid := ""
				if m := regexp.MustCompile(`session_id \\?"(\d+)\\?"`).FindStringSubmatch(fmt.Sprint(body["messages"])); m != nil {
					sid = m[1]
				}
				return fakeTurn{Calls: []fakeCall{{"wrm_system_info", `{"session_id":"` + sid + `"}`}}}
			}
		}
		if strings.Contains(fmt.Sprint(body), "title") {
			return fakeTurn{Text: "Server check"}
		}
		return fakeTurn{Text: "no wrm tools: " + strings.Join(names, ",")}
	}
	code, v := e.admin.aiDo("POST", "/api/admin/ai/providers", map[string]interface{}{"name": "Local", "kind": "openai_compatible",
		"base_url": e.llm.srv.URL + "/v1", "models": "local-model"})
	if code != 200 {
		t.Fatalf("provider: %d %v", code, v)
	}
	pid := int(asMap(v)["id"].(float64))
	code, v = e.user.aiDo("POST", "/api/ai/sessions", map[string]interface{}{"conn_id": e.connID, "provider_id": pid, "mode": "read_only", "engine": "opencode", "agent": "build"})
	if code != 200 {
		t.Fatalf("start: %d %v", code, v)
	}
	s := aiGet(int64(asMap(v)["id"].(float64)))
	dir := s.oc.dir
	grant := s.oc.grantID
	e.prompt(s, "what system is this?")
	aiWaitUntil(t, "the agent to answer", 120*time.Second, func() bool {
		return idle(s) && strings.Contains(strings.Join(eventTypes(s), ","), "assistant_done")
	})
	evs, _, _ := s.eventsSince(0)
	var forwarded, answered bool
	for _, ev := range evs {
		if ev.Type == "tool_call" && ev.Data["tool"] == "system_info" && ev.Data["session_id"] != nil {
			forwarded = true
		}
		if ev.Type == "assistant_done" && strings.Contains(fmt.Sprint(ev.Data["text"]), "checked through WRM") {
			answered = true
		}
	}
	if !sawTool || !forwarded || !answered {
		t.Fatalf("tool offered %v, forwarded %v, answered %v; events %v\nlog:\n%s", sawTool, forwarded, answered, eventTypes(s), s.oc.logTail(40))
	}
	s.Kill(e.user.userID, "test done")
	aiWaitUntil(t, "OpenCode to stop", 30*time.Second, func() bool { _, err := os.Stat(dir); return os.IsNotExist(err) })
	if g, err := loadGrant(grant); err != nil || g.RevokedAt == "" {
		t.Fatalf("the AI connection is still active: %+v %v", g, err)
	}
	if _, err := os.Stat(filepath.Join(opencodeRoot(), "run")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func asSlice(v interface{}) []interface{} {
	s, _ := v.([]interface{})
	return s
}

// In ask mode a change the agent wants waits for the user's approval in the OpenCode chat.
func TestOpencodeAgentApproval(t *testing.T) {
	bin := os.Getenv("WRM_TEST_OPENCODE")
	if bin == "" {
		t.Skip("set WRM_TEST_OPENCODE to an opencode binary to run this test")
	}
	t.Setenv("WRM_AI_OPENCODE_PATH", bin)
	e := newAIEnv(t, nil)
	e.llm.script = func(n int, body map[string]interface{}) fakeTurn {
		msgs := asSlice(body["messages"])
		if asMap(msgs[len(msgs)-1])["role"] == "tool" {
			return fakeTurn{Text: "Restarted after your approval."}
		}
		if strings.Contains(fmt.Sprint(body["tools"]), "wrm_run_command") {
			m := regexp.MustCompile(`session_id \\?"(\d+)\\?"`).FindStringSubmatch(fmt.Sprint(body["messages"]))
			if m == nil {
				return fakeTurn{Text: "no session id"}
			}
			return fakeTurn{Calls: []fakeCall{{"wrm_run_command", `{"session_id":"` + m[1] + `","command":"touch ~/approved-by-user.txt","reason":"mark the change"}`}}}
		}
		return fakeTurn{Text: "Title"}
	}
	code, v := e.admin.aiDo("POST", "/api/admin/ai/providers", map[string]interface{}{"name": "Local", "kind": "openai_compatible",
		"base_url": e.llm.srv.URL + "/v1", "models": "local-model"})
	if code != 200 {
		t.Fatalf("provider: %d %v", code, v)
	}
	pid := int(asMap(v)["id"].(float64))
	code, v = e.user.aiDo("POST", "/api/ai/sessions", map[string]interface{}{"conn_id": e.connID, "provider_id": pid, "mode": "ask", "engine": "opencode"})
	if code != 200 {
		t.Fatalf("start: %d %v", code, v)
	}
	s := aiGet(int64(asMap(v)["id"].(float64)))
	defer s.Kill(0, "test done")
	e.prompt(s, "restart the service")
	var appr map[string]interface{}
	var child interface{}
	aiWaitUntil(t, "the approval in the OpenCode chat", 60*time.Second, func() bool {
		evs, _, _ := s.eventsSince(0)
		for _, ev := range evs {
			if ev.Type == "approval_required" {
				appr, child = asMapAny(ev.Data["approval"]), ev.Data["session_id"]
				return true
			}
		}
		return false
	})
	if !strings.Contains(fmt.Sprint(appr["command"]), "approved-by-user.txt") || child == nil {
		t.Fatalf("approval: %v (session %v)", appr, child)
	}
	code, v = e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%v/approvals/%v", child, appr["id"]), map[string]interface{}{"decision": "approve"})
	if code != 200 {
		t.Fatalf("approve: %d %v", code, v)
	}
	aiWaitUntil(t, "the agent to finish", 60*time.Second, func() bool {
		evs, _, _ := s.eventsSince(0)
		return idle(s) && strings.Contains(fmt.Sprint(evs), "Restarted after your approval")
	})
	if _, err := os.Stat(filepath.Join(e.host.home, "approved-by-user.txt")); err != nil {
		t.Fatalf("the approved command did not run: %v", err)
	}
}

// asMapAny turns a struct (as the approval is) into a map.
func asMapAny(v interface{}) map[string]interface{} {
	b, _ := json.Marshal(v)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	return m
}
