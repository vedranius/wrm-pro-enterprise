// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The Claude Desktop extension: the user downloads it with their token filled in, it holds
// the manifest, the bridge and the icon, and nothing but a WRM token can be filled in.
func TestMCPClaudeDesktopExtension(t *testing.T) {
	e := newMCPEnv(t)
	tok, _ := e.pat("read_only", []string{"read_logs", "run_readonly"}, nil)
	body, _ := json.Marshal(map[string]string{"token": tok})
	resp := e.user.do("POST", "/api/mcp/claude-desktop", bytes.NewReader(body), "application/json")
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/zip" || !strings.Contains(resp.Header.Get("Content-Disposition"), "wrm-pro.mcpb") {
		t.Fatalf("download: %d %s %s", resp.StatusCode, resp.Header, raw)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		files[f.Name], _ = io.ReadAll(rc)
		rc.Close()
	}
	for _, n := range []string{"manifest.json", "server/index.js", "icon.png"} {
		if len(files[n]) == 0 {
			t.Fatalf("bundle lacks %s: %v", n, zr.File)
		}
	}
	var man map[string]interface{}
	if err := json.Unmarshal(files["manifest.json"], &man); err != nil {
		t.Fatal(err)
	}
	uc := asMap(man["user_config"])
	if man["name"] != "wrm-pro" || man["version"] != strings.TrimPrefix(AppVersion, "v") || asMap(man["server"])["type"] != "node" ||
		asMap(uc["wrm_url"])["default"] != e.srv+"/mcp" || asMap(uc["wrm_token"])["default"] != tok || asMap(uc["wrm_token"])["sensitive"] != true {
		t.Fatalf("manifest: %s", files["manifest.json"])
	}
	if !strings.Contains(string(files["server/index.js"]), "const VERSION = '"+strings.TrimPrefix(AppVersion, "v")+"';") {
		t.Fatal("the bridge does not carry the version")
	}
	if mcpAuditCount("mcp.desktop_extension", "%with_token%") == 0 {
		t.Fatal("the download is not audited")
	}
	// without a token the field stays empty for the install dialog
	resp = e.user.do("POST", "/api/mcp/claude-desktop", nil, "application/json")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("without a token: %d", resp.StatusCode)
	}
	for _, bad := range []string{"secret", `wrm_pat_x" , "evil": "1`, mcpPATPrefix + strings.Repeat("a", mcpbTokenMax)} {
		b, _ := json.Marshal(map[string]string{"token": bad})
		if code, _ := e.user.aiDo("POST", "/api/mcp/claude-desktop", json.RawMessage(b)); code != 400 {
			t.Fatalf("token %q accepted: %d", bad, code)
		}
	}
	if code, _ := e.user.aiDo("GET", "/api/mcp/claude-desktop", nil); code != 405 {
		t.Fatalf("GET: %d", code)
	}
	t.Setenv("WRM_AI_MCP_ENABLED", "0")
	if code, _ := e.user.aiDo("POST", "/api/mcp/claude-desktop", nil); code != 403 {
		t.Fatalf("MCP off: %d", code)
	}
}

// The extension's Node.js bridge talks to /mcp like Claude Desktop will run it: initialize,
// tools/list, an approval question over the event stream, and a clear error for a bad token.
func TestMCPClaudeDesktopBridgeNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	e := newMCPEnv(t)
	tok, _ := e.pat("ask", []string{"read_logs", "run_with_approval"}, nil)
	script := filepath.Join(t.TempDir(), "index.js")
	os.WriteFile(script, []byte(mcpbBridgeJS), 0o600)
	cmd := exec.Command(node, script)
	cmd.Env = append(os.Environ(), "WRM_MCP_URL="+e.srv, "WRM_MCP_TOKEN="+tok, "WRM_MCP_CA_FILE=${user_config.wrm_ca_file}")
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	lines := make(chan map[string]interface{}, 32)
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64<<10), 8<<20)
		for sc.Scan() {
			var m map[string]interface{}
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Errorf("not JSON on stdout: %q", sc.Text())
			}
			lines <- m
		}
		close(lines)
	}()
	send := func(m map[string]interface{}) {
		b, _ := json.Marshal(m)
		in.Write(append(b, '\n'))
	}
	next := func() map[string]interface{} {
		select {
		case m := <-lines:
			return m
		case <-time.After(20 * time.Second):
			t.Fatalf("no answer from the bridge; stderr: %s", stderr.String())
		}
		return nil
	}
	send(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{"protocolVersion": "2025-06-18",
		"capabilities": map[string]interface{}{"elicitation": map[string]interface{}{}}, "clientInfo": map[string]interface{}{"name": "claude-ai"}}})
	send(map[string]interface{}{"jsonrpc": "2.0", "method": "notifications/initialized"})
	send(map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	if m := next(); asMap(m["result"])["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize: %v", m)
	}
	if m := next(); len(asMap(m["result"])["tools"].([]interface{})) == 0 {
		t.Fatalf("tools/list: %v", m)
	}
	send(map[string]interface{}{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]interface{}{"name": "open_session", "arguments": map[string]interface{}{"connection": "demo-api"}}})
	m := next()
	var info map[string]interface{}
	json.Unmarshal([]byte(fmt.Sprint(asMap(asMap(m["result"])["content"].([]interface{})[0])["text"])), &info)
	sid := fmt.Sprint(info["session_id"])
	send(map[string]interface{}{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]interface{}{"name": "run_command",
		"arguments": map[string]interface{}{"session_id": sid, "command": "touch ~/via-mcpb", "reason": "test"}}})
	m = next()
	if m["method"] != "elicitation/create" {
		t.Fatalf("expected an elicitation: %v", m)
	}
	send(map[string]interface{}{"jsonrpc": "2.0", "id": m["id"], "result": map[string]interface{}{"action": "accept", "content": map[string]interface{}{"decision": "approve"}}})
	m = next()
	if m["id"].(float64) != 4 || asMap(m["result"])["isError"] == true || !aiFileExists(filepath.Join(e.host.home, "via-mcpb")) {
		t.Fatalf("run_command through the bridge: %v", m)
	}
	in.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("bridge exit: %v %s", err, stderr.String())
	}
	// a wrong token: the bridge still starts and offers wrm_connect, saying what was wrong
	bad := exec.Command(node, script)
	bad.Env = append(os.Environ(), "WRM_MCP_URL="+e.srv+"/mcp", "WRM_MCP_TOKEN=wrm_pat_wrong", "WRM_MCP_CONFIG="+filepath.Join(t.TempDir(), "none.json"))
	bad.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n")
	outb, _ := bad.Output()
	if strings.Contains(string(outb), `"error"`) || !strings.Contains(string(outb), "wrm_connect") || !strings.Contains(string(outb), "401") || !strings.Contains(string(outb), "AI connections") {
		t.Fatalf("wrong token: %s", outb)
	}
}
