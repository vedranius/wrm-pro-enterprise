// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// In the sandbox OpenCode's own tools are on, the shell, file changes and web fetches ask,
// paths outside the sandbox folder are refused and a local provider is reached from inside
// the container.
func TestOpencodeSandboxConfig(t *testing.T) {
	p := aiProvider{Kind: "openai_compatible", Name: "Local", BaseURL: "http://127.0.0.1:11434/v1"}
	b, _, _, _, err := opencodeConfig(p, "m", "http://host.docker.internal:9/mcp", "wrm_pat_x", true)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	json.Unmarshal(b, &cfg)
	tools := cfg["tools"].(map[string]interface{})
	for _, k := range []string{"bash", "edit", "read", "webfetch"} {
		if tools[k] != true {
			t.Fatalf("tool %s not on in the sandbox: %v", k, tools)
		}
	}
	perm := cfg["permission"].(map[string]interface{})
	for _, k := range opencodeSandboxAsk {
		if perm[k] != "ask" {
			t.Fatalf("permission %s = %v, want ask", k, perm[k])
		}
	}
	if perm["external_directory"] != "deny" {
		t.Fatalf("external_directory: %v", perm["external_directory"])
	}
	if !strings.Contains(string(b), "http://host.docker.internal:11434/v1") || strings.Contains(string(b), "127.0.0.1:11434") {
		t.Fatalf("local provider URL not rewritten for the container: %s", b)
	}
	// without the sandbox nothing changes
	b, _, _, _, _ = opencodeConfig(p, "m", "http://127.0.0.1:9/mcp", "wrm_pat_x", false)
	if !strings.Contains(string(b), "127.0.0.1:11434") {
		t.Fatalf("host provider URL changed: %s", b)
	}
}

// The container is locked down and no secret value appears in the docker command line.
func TestOpencodeSandboxArgs(t *testing.T) {
	newTestServer(t)
	args := strings.Join(opencodeSandboxArgs("wrm-opencode-1-ab", "/data/s1", "/data/bin/opencode", 40001,
		[]string{"WRM_OPENCODE_KEY", "WRM_OPENCODE_MCP_TOKEN"}, true), " ")
	for _, want := range []string{"--rm", "--cap-drop ALL", "--security-opt no-new-privileges", "--read-only", "--pids-limit 512",
		"--memory 2g", "--publish 127.0.0.1:40001:4096", "--volume /data/s1:/work", "/opt/opencode/opencode:ro",
		"--env WRM_OPENCODE_KEY ", "--env NODE_EXTRA_CA_CERTS=/work/ca.pem", opencodeSandboxLabel(), opencodeSandboxDefaultImage + " serve"} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing %q in: %s", want, args)
		}
	}
	if strings.Contains(args, "--privileged") || strings.Contains(args, "docker.sock") || strings.Contains(args, "=wrm_pat") {
		t.Fatalf("unsafe arguments: %s", args)
	}
}

func TestOpencodeSandboxSettings(t *testing.T) {
	newTestServer(t)
	for _, bad := range []string{"Debian", "debian:stable;rm", "../x", "img --privileged", "-v/:/host"} {
		if _, err := setSetting("ai_opencode_sandbox_image", bad); err == nil {
			t.Fatalf("image %q accepted", bad)
		}
	}
	if _, err := setSetting("ai_opencode_sandbox_image", "registry.example.com/team/base:1.2@sha256:abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := setSetting("ai_opencode_sandbox", "everyone"); err == nil {
		t.Fatal("unknown sandbox policy accepted")
	}
	setSetting("ai_opencode_sandbox", "off")
	if ok, why := opencodeSandboxAllowed(1); ok || why == "" {
		t.Fatalf("sandbox allowed while off: %v %q", ok, why)
	}
}

// A permission request of a sandboxed OpenCode becomes a WRM approval that cannot be edited;
// OpenCode gets "once" only after the user approves, otherwise "reject" with the reason.
func TestOpencodeSandboxAsk(t *testing.T) {
	e := newAIEnv(t, nil)
	var mu sync.Mutex
	replies := map[string]map[string]string{}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/permission/") {
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			replies[strings.Split(r.URL.Path, "/")[2]] = body
			mu.Unlock()
			w.Write([]byte("true"))
			return
		}
		http.NotFound(w, r)
	}))
	defer fake.Close()
	s := e.start("ask", nil)
	o := &ocProc{base: fake.URL, sandbox: true, tools: map[string]string{}, parts: map[string]string{}, roles: map[string]string{}, started: map[string]bool{}}
	reply := func(id string) map[string]string {
		var r map[string]string
		aiWaitUntil(t, "OpenCode to get an answer for "+id, 10*time.Second, func() bool {
			mu.Lock()
			defer mu.Unlock()
			r = replies[id]
			return r != nil
		})
		return r
	}
	approval := func(callID string) map[string]interface{} {
		var a map[string]interface{}
		aiWaitUntil(t, "the approval of "+callID, 10*time.Second, func() bool {
			evs, _, _ := s.eventsSince(0)
			for _, ev := range evs {
				if ev.Type == "approval_required" {
					if m := asMapAny(ev.Data["approval"]); m["call_id"] == callID {
						a = m
						return true
					}
				}
			}
			return false
		})
		return a
	}

	// a shell command: approved (an edited command is ignored)
	p := ocPermission{ID: "per_1", Permission: "bash", Patterns: []string{"ls -la"}, Metadata: map[string]interface{}{"command": "ls -la"}}
	p.Tool.CallID = "call_1"
	go s.ocAsk(o, p)
	a := approval("oc-call_1")
	if a["command"] != "ls -la" || a["no_edit"] != true {
		t.Fatalf("approval: %v", a)
	}
	code, v := e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/approvals/%v", s.ID, a["id"]), map[string]interface{}{"decision": "approve", "command": "rm -rf /work"})
	if code != 200 {
		t.Fatalf("approve: %d %v", code, v)
	}
	if r := reply("per_1"); r["reply"] != "once" {
		t.Fatalf("reply: %v", r)
	}
	evs, _, _ := s.eventsSince(0)
	if strings.Contains(fmt.Sprint(evs), "rm -rf /work") {
		t.Fatal("the edited command reached the events")
	}

	// a file change: denied with a note
	p = ocPermission{ID: "per_2", Permission: "edit", Metadata: map[string]interface{}{"filepath": "/work/work/notes.txt", "diff": "+hello\n"}}
	p.Tool.CallID = "call_2"
	go s.ocAsk(o, p)
	a = approval("oc-call_2")
	if a["path"] != "/work/work/notes.txt" {
		t.Fatalf("edit approval: %v", a)
	}
	e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/approvals/%v", s.ID, a["id"]), map[string]interface{}{"decision": "deny", "note": "not now"})
	if r := reply("per_2"); r["reply"] != "reject" || !strings.Contains(r["message"], "not now") {
		t.Fatalf("reply: %v", r)
	}

	// anything else is refused without asking
	go s.ocAsk(o, ocPermission{ID: "per_3", Permission: "external_directory", Patterns: []string{"/etc/*"}})
	if r := reply("per_3"); r["reply"] != "reject" {
		t.Fatalf("reply: %v", r)
	}
}

// The real thing: OpenCode in a Docker container runs a shell command only after the user
// approves it in WRM, and the container is gone when the agent stops.
// WRM_TEST_OPENCODE_SANDBOX=<image> (with WRM_TEST_OPENCODE and a working docker) runs it.
func TestOpencodeSandboxEndToEnd(t *testing.T) {
	bin, image := os.Getenv("WRM_TEST_OPENCODE"), os.Getenv("WRM_TEST_OPENCODE_SANDBOX")
	if bin == "" || image == "" {
		t.Skip("set WRM_TEST_OPENCODE and WRM_TEST_OPENCODE_SANDBOX=<image> to run this test")
	}
	t.Setenv("WRM_AI_OPENCODE_PATH", bin)
	t.Setenv("WRM_AI_OPENCODE_SANDBOX", "all")
	t.Setenv("WRM_AI_OPENCODE_SANDBOX_IMAGE", image)
	e := newAIEnv(t, nil)
	e.llm.script = func(n int, body map[string]interface{}) fakeTurn {
		msgs := asSlice(body["messages"])
		last := asMap(msgs[len(msgs)-1])
		if last["role"] == "tool" {
			return fakeTurn{Text: "Sandbox said: " + fmt.Sprint(last["content"])}
		}
		if strings.Contains(fmt.Sprint(body["tools"]), "bash") {
			return fakeTurn{Calls: []fakeCall{{"bash", `{"command":"echo sandbox-$((40+2)) > proof.txt; cat proof.txt; id -u","description":"write a proof file"}`}}}
		}
		return fakeTurn{Text: "Title"}
	}
	code, v := e.admin.aiDo("POST", "/api/admin/ai/providers", map[string]interface{}{"name": "Local", "kind": "openai_compatible",
		"base_url": e.llm.srv.URL + "/v1", "models": "local-model"})
	if code != 200 {
		t.Fatalf("provider: %d %v", code, v)
	}
	pid := int(asMap(v)["id"].(float64))
	code, v = e.user.aiDo("POST", "/api/ai/sessions", map[string]interface{}{"conn_id": e.connID, "provider_id": pid, "mode": "ask", "engine": "opencode", "sandbox": true})
	if code != 200 {
		t.Fatalf("start: %d %v", code, v)
	}
	s := aiGet(int64(asMap(v)["id"].(float64)))
	container := s.oc.container
	e.prompt(s, "write a proof file in your sandbox")
	var appr map[string]interface{}
	aiWaitUntil(t, "the sandbox approval", 120*time.Second, func() bool {
		evs, _, _ := s.eventsSince(0)
		for _, ev := range evs {
			if ev.Type == "approval_required" {
				appr = asMapAny(ev.Data["approval"])
				return true
			}
		}
		return false
	})
	if !strings.Contains(fmt.Sprint(appr["command"]), "proof.txt") || appr["no_edit"] != true {
		t.Fatalf("approval: %v", appr)
	}
	code, v = e.user.aiDo("POST", fmt.Sprintf("/api/ai/sessions/%d/approvals/%v", s.ID, appr["id"]), map[string]interface{}{"decision": "approve"})
	if code != 200 {
		t.Fatalf("approve: %d %v", code, v)
	}
	aiWaitUntil(t, "the command to run in the container", 120*time.Second, func() bool {
		evs, _, _ := s.eventsSince(0)
		return idle(s) && strings.Contains(fmt.Sprint(evs), "sandbox-42")
	})
	s.Kill(0, "test done")
	aiWaitUntil(t, "the container to be removed", 60*time.Second, func() bool {
		out, _ := exec.Command(mediumDockerCLI(), "ps", "-a", "--filter", "name="+container, "--format", "{{.Names}}").Output()
		return strings.TrimSpace(string(out)) == ""
	})
}
