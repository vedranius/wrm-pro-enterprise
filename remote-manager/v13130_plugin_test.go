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
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const pluginDir = "../packaging/claude-plugin/wrm-pro"

// The Claude plugin carries the bridge as an MCP bundle without settings (the Claude app
// cannot ask for plugin settings). The committed bundle must be the one this code builds:
// WRM_WRITE_PLUGIN_MCPB=1 go test -run TestV13130PluginBundle rewrites it.
func TestV13130PluginBundle(t *testing.T) {
	want, err := buildPluginMCPB()
	if err != nil {
		t.Fatal(err)
	}
	again, _ := buildPluginMCPB()
	if !bytes.Equal(want, again) {
		t.Fatal("the plugin bundle is not reproducible")
	}
	path := filepath.Join(pluginDir, "server", "wrm.mcpb")
	if os.Getenv("WRM_WRITE_PLUGIN_MCPB") == "1" {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s is missing or out of date (err %v): run WRM_WRITE_PLUGIN_MCPB=1 go test -run TestV13130PluginBundle", path, err)
	}
	zr, err := zip.NewReader(bytes.NewReader(got), int64(len(got)))
	if err != nil {
		t.Fatal(err)
	}
	var man map[string]interface{}
	for _, f := range zr.File {
		if f.Name == "manifest.json" {
			rc, _ := f.Open()
			json.NewDecoder(rc).Decode(&man)
			rc.Close()
		}
	}
	if man["name"] != "wrm" || man["version"] != pluginMCPBVersion || man["user_config"] != nil || asMap(asMap(man["server"])["mcp_config"])["env"] != nil {
		t.Fatalf("plugin bundle manifest: %v", man)
	}

	var plugin map[string]interface{}
	b, _ := os.ReadFile(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"))
	if err := json.Unmarshal(b, &plugin); err != nil {
		t.Fatal(err)
	}
	if plugin["version"] != pluginMCPBVersion || plugin["mcpServers"] != "./server/wrm.mcpb" || plugin["userConfig"] != nil {
		t.Fatalf("plugin.json must use the bundle, carry version %s and declare no settings: %s", pluginMCPBVersion, b)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "bin")); err == nil {
		t.Fatal("a top-level bin/ directory stops claude.ai and Cowork from installing the plugin")
	}
	skill, err := os.ReadFile(filepath.Join(pluginDir, "skills", "wrm", "SKILL.md"))
	if err != nil || !strings.Contains(string(skill), "name: wrm\n") || !strings.Contains(string(skill), "wrm_connect") {
		t.Fatalf("skill wrm: %v", err)
	}
}

// lockedBuffer collects the bridge's stderr while the test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuffer) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

// bridgeProc runs the bridge with Node.js and reads its stdout as JSON-RPC lines.
type bridgeProc struct {
	t      *testing.T
	cmd    *exec.Cmd
	in     io.WriteCloser
	lines  chan map[string]interface{}
	stderr *lockedBuffer
}

func startBridge(t *testing.T, env ...string) *bridgeProc {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	script := filepath.Join(t.TempDir(), "index.js")
	os.WriteFile(script, []byte(mcpbBridgeJS), 0o600)
	cmd := exec.Command(node, script)
	cmd.Env = append(os.Environ(), env...)
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	p := &bridgeProc{t: t, cmd: cmd, in: in, lines: make(chan map[string]interface{}, 64), stderr: &lockedBuffer{}}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64<<10), 8<<20)
		for sc.Scan() {
			var m map[string]interface{}
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Errorf("not JSON on stdout: %q", sc.Text())
			}
			p.lines <- m
		}
		close(p.lines)
	}()
	t.Cleanup(func() { in.Close(); cmd.Process.Kill(); cmd.Wait() })
	return p
}

func (p *bridgeProc) send(m map[string]interface{}) {
	b, _ := json.Marshal(m)
	p.in.Write(append(b, '\n'))
}

func (p *bridgeProc) next() map[string]interface{} {
	select {
	case m := <-p.lines:
		return m
	case <-time.After(30 * time.Second):
		p.t.Fatalf("no answer from the bridge; stderr: %s", p.stderr.String())
	}
	return nil
}

func toolNames(m map[string]interface{}) []string {
	var out []string
	tools, _ := asMap(m["result"])["tools"].([]interface{})
	for _, tl := range tools {
		out = append(out, fmt.Sprint(asMap(tl)["name"]))
	}
	return out
}

func toolResultText(m map[string]interface{}) string {
	content, _ := asMap(m["result"])["content"].([]interface{})
	if len(content) == 0 {
		return ""
	}
	return fmt.Sprint(asMap(content[0])["text"])
}

// Without any settings the bridge starts, offers wrm_connect, signs in through WRM's consent
// page (the test plays the browser), saves the token in the connection file and from then on
// forwards to WRM; a second bridge finds the saved connection by itself.
func TestV13130BridgeSignIn(t *testing.T) {
	e := newMCPEnv(t)
	cfg := filepath.Join(t.TempDir(), "wrm", "mcp.json")
	p := startBridge(t, "WRM_MCP_URL=", "WRM_MCP_TOKEN=", "WRM_MCP_CONFIG="+cfg, "WRM_MCP_NO_BROWSER=1", "WRM_MCP_CONNECT_WAIT_MS=1500")
	p.send(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{"protocolVersion": "2025-06-18",
		"capabilities": map[string]interface{}{}, "clientInfo": map[string]interface{}{"name": "claude-ai"}}})
	m := p.next()
	if r := asMap(m["result"]); r["protocolVersion"] != "2025-06-18" || asMap(asMap(r["capabilities"])["tools"])["listChanged"] != true ||
		!strings.Contains(fmt.Sprint(r["instructions"]), "wrm_connect") {
		t.Fatalf("local initialize: %v", m)
	}
	p.send(map[string]interface{}{"jsonrpc": "2.0", "method": "notifications/initialized"})
	p.send(map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	if names := toolNames(p.next()); len(names) != 1 || names[0] != "wrm_connect" {
		t.Fatalf("tools before sign-in: %v", names)
	}
	p.send(map[string]interface{}{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]interface{}{"name": "list_connections", "arguments": map[string]interface{}{}}})
	if m := p.next(); asMap(m["result"])["isError"] != true || !strings.Contains(toolResultText(m), "wrm_connect") {
		t.Fatalf("a WRM tool before sign-in: %v", m)
	}
	// the address as a user types it: host and port only
	p.send(map[string]interface{}{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]interface{}{"name": "wrm_connect",
		"arguments": map[string]interface{}{"address": strings.TrimPrefix(e.srv, "http://")}}})
	m = p.next()
	link := regexp.MustCompile(`http://\S+/oauth/authorize\?\S+`).FindString(toolResultText(m))
	if link == "" || asMap(m["result"])["isError"] == true {
		t.Fatalf("wrm_connect: %v; stderr %s", m, p.stderr.String())
	}
	// the browser: WRM's authorize page, the user's consent, then the loopback redirect
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := noRedirect.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	loc := r.Header.Get("Location")
	if r.StatusCode != 302 || !strings.HasPrefix(loc, "/?mcp_authorize=") {
		t.Fatalf("authorize: %d %s", r.StatusCode, loc)
	}
	id := strings.TrimPrefix(loc, "/?mcp_authorize=")
	code, v := e.user.aiDo("GET", "/api/mcp/authorize/"+id, nil)
	if code != 200 || !strings.HasPrefix(fmt.Sprint(asMap(v)["client_name"]), "WRM PRO plugin") || asMap(v)["redirect_host"] == "" {
		t.Fatalf("consent info: %d %v", code, v)
	}
	code, v = e.user.aiDo("POST", "/api/mcp/authorize/"+id, map[string]interface{}{"decision": "approve", "conn_ids": []int{e.connID},
		"mode": "read_only", "scopes": []string{"read_logs", "run_readonly"}, "hours": 2})
	if code != 200 {
		t.Fatalf("consent: %d %v", code, v)
	}
	cb, err := http.Get(fmt.Sprint(asMap(v)["redirect"]))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(cb.Body)
	cb.Body.Close()
	if !strings.Contains(string(body), "Connected") {
		t.Fatalf("callback page: %s; stderr %s", body, p.stderr.String())
	}
	if m := p.next(); m["method"] != "notifications/tools/list_changed" {
		t.Fatalf("expected tools/list_changed: %v", m)
	}
	p.send(map[string]interface{}{"jsonrpc": "2.0", "id": 5, "method": "tools/list"})
	names := strings.Join(toolNames(p.next()), " ")
	if !strings.Contains(names, "open_session") || !strings.Contains(names, "wrm_connect") {
		t.Fatalf("tools after sign-in: %s", names)
	}
	p.send(map[string]interface{}{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": map[string]interface{}{"name": "list_connections", "arguments": map[string]interface{}{}}})
	if m := p.next(); asMap(m["result"])["isError"] == true || !strings.Contains(toolResultText(m), "demo-api") {
		t.Fatalf("list_connections after sign-in: %v", m)
	}
	saved, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var c struct{ URL, Token string }
	json.Unmarshal(saved, &c)
	if c.URL != e.srv+"/mcp" || !strings.HasPrefix(c.Token, "wrm_") {
		t.Fatalf("connection file: %s", saved)
	}
	if st, _ := os.Stat(cfg); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("connection file mode %v", st.Mode().Perm())
	}
	if strings.Contains(p.stderr.String(), c.Token) {
		t.Fatal("the token reached the log")
	}

	// a new session finds the saved connection
	p2 := startBridge(t, "WRM_MCP_URL=", "WRM_MCP_TOKEN=", "WRM_MCP_CONFIG="+cfg)
	p2.send(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{"protocolVersion": "2025-06-18",
		"capabilities": map[string]interface{}{}, "clientInfo": map[string]interface{}{"name": "claude-ai"}}})
	if m := p2.next(); !strings.Contains(fmt.Sprint(asMap(asMap(m["result"])["serverInfo"])["title"]), "Remote Manager") {
		t.Fatalf("saved connection: %v", m)
	}
	p2.send(map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	if names := strings.Join(toolNames(p2.next()), " "); !strings.Contains(names, "open_session") {
		t.Fatalf("tools with the saved connection: %s", names)
	}

	// the user denies: the tool says so and nothing is saved over the connection
	os.Remove(cfg)
	p3 := startBridge(t, "WRM_MCP_URL=", "WRM_MCP_TOKEN=", "WRM_MCP_CONFIG="+cfg, "WRM_MCP_NO_BROWSER=1", "WRM_MCP_CONNECT_WAIT_MS=1500")
	p3.send(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{}})
	p3.next()
	p3.send(map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]interface{}{"name": "wrm_connect", "arguments": map[string]interface{}{"address": e.srv}}})
	link = regexp.MustCompile(`http://\S+/oauth/authorize\?\S+`).FindString(toolResultText(p3.next()))
	r, _ = noRedirect.Get(link)
	r.Body.Close()
	id = strings.TrimPrefix(r.Header.Get("Location"), "/?mcp_authorize=")
	_, v = e.user.aiDo("POST", "/api/mcp/authorize/"+id, map[string]interface{}{"decision": "deny"})
	if cb, err := http.Get(fmt.Sprint(asMap(v)["redirect"])); err == nil {
		cb.Body.Close()
	}
	p3.send(map[string]interface{}{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]interface{}{"name": "wrm_connect", "arguments": map[string]interface{}{"address": e.srv}}})
	m = p3.next()
	if _, err := os.Stat(cfg); err == nil {
		t.Fatal("a denied sign-in saved a connection")
	}
	_ = m

	// an address without WRM: a clear error, no crash
	p4 := startBridge(t, "WRM_MCP_URL=", "WRM_MCP_TOKEN=", "WRM_MCP_CONFIG="+cfg, "WRM_MCP_NO_BROWSER=1")
	p4.send(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{}})
	p4.next()
	u, _ := url.Parse(e.srv)
	p4.send(map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]interface{}{"name": "wrm_connect", "arguments": map[string]interface{}{"address": "http://127.0.0.1:1"}}})
	if m := p4.next(); asMap(m["result"])["isError"] != true || !strings.Contains(toolResultText(m), "Could not start") {
		t.Fatalf("unreachable address: %v (%s)", m, u.Host)
	}
}

// The setup script for all AI apps: the server builds it with this WRM's address and the
// bridge (never a token); run with sh it saves the connection once and adds WRM to the apps it
// finds, keeping what the users' config files already hold.
func TestV13130SetupAllApps(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	e := newMCPEnv(t)
	resp := e.user.do("POST", "/api/mcp/setup-all", strings.NewReader(`{"os":"windows"}`), "application/json")
	ps, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Disposition"), "wrm-setup-all.ps1") || !bytes.HasPrefix(ps, []byte("\xef\xbb\xbf")) ||
		!strings.Contains(string(ps), "$wrmUrl = '"+e.srv+"/mcp'") || !strings.Contains(string(ps), "\r\n'@\r\n") || strings.Contains(string(ps), mcpPATPrefix) {
		t.Fatalf("windows script: %d %s", resp.StatusCode, resp.Header)
	}
	resp = e.user.do("POST", "/api/mcp/setup-all", strings.NewReader(`{"os":"unix"}`), "application/json")
	sh, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(string(sh), "#!/bin/sh\n") || !strings.Contains(string(sh), "wrm_url='"+e.srv+"/mcp'") {
		t.Fatalf("unix script: %d %s", resp.StatusCode, sh[:200])
	}
	if mcpAuditCount("mcp.setup_script", "%unix%") == 0 {
		t.Fatal("the download is not audited")
	}
	if code, _ := e.user.aiDo("GET", "/api/mcp/setup-all", nil); code != 405 {
		t.Fatalf("GET: %d", code)
	}

	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".cursor"), 0o755)
	os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755)
	os.WriteFile(filepath.Join(home, ".config", "opencode", "opencode.json"), []byte(`{"theme": "dark", "mcp": {"other": {"type": "remote", "url": "https://x.example.com"}}}`), 0o644)
	os.MkdirAll(filepath.Join(home, ".config", "Code", "User"), 0o755)
	os.WriteFile(filepath.Join(home, ".config", "Code", "User", "mcp.json"), []byte("// my servers\n{\n  \"servers\": {\n    \"keep\": {\"type\": \"http\", \"url\": \"https://k.example.com/mcp\"}, // a comment\n  },\n}\n"), 0o644)
	os.MkdirAll(filepath.Join(home, ".config", "Claude"), 0o755)
	os.WriteFile(filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"), []byte(`{"mcpServers": {"fs": {"command": "npx"}}, "globalShortcut": "Ctrl+Space"}`), 0o644)
	script := filepath.Join(t.TempDir(), "wrm-setup-all.sh")
	os.WriteFile(script, sh, 0o700)
	tok, _ := e.pat("read_only", []string{"read_logs"}, nil)
	// PATH holds node only: no AI CLIs of this machine are touched
	bin := t.TempDir()
	os.Symlink(node, filepath.Join(bin, "node"))
	cmd := exec.Command("sh", script)
	cmd.Env = []string{"HOME=" + home, "PATH=" + bin + ":/usr/bin:/bin", "WRM_MCP_TOKEN=" + tok}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("setup: %v\n%s", err, out)
	}
	if strings.Contains(string(out), tok) {
		t.Fatal("the token was printed")
	}
	for _, want := range []string{"OK: WRM PRO", "[ok] Cursor", "[ok] OpenCode", "[ok] VS Code", "[ok] Claude app", "[--] Codex CLI", "[--] Windsurf"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("setup output lacks %q:\n%s", want, out)
		}
	}
	bridge := filepath.Join(home, ".wrm", "wrm-mcp.js")
	if b, err := os.ReadFile(bridge); err != nil || !strings.Contains(string(b), "const VERSION = '"+strings.TrimPrefix(AppVersion, "v")+"';") {
		t.Fatalf("bridge file: %v", err)
	}
	var conn struct{ URL, Token string }
	b, _ := os.ReadFile(filepath.Join(home, ".wrm", "mcp.json"))
	json.Unmarshal(b, &conn)
	if conn.URL != e.srv+"/mcp" || conn.Token != tok {
		t.Fatalf("connection file: %s", b)
	}
	readJSON := func(p ...string) map[string]interface{} {
		var v map[string]interface{}
		b, err := os.ReadFile(filepath.Join(append([]string{home}, p...)...))
		if err != nil || json.Unmarshal(b, &v) != nil {
			t.Fatalf("%v: %v %s", p, err, b)
		}
		return v
	}
	isBridge := func(v interface{}) bool {
		args, _ := asMap(v)["args"].([]interface{})
		return len(args) == 1 && args[0] == bridge
	}
	if c := readJSON(".cursor", "mcp.json"); !isBridge(asMap(c["mcpServers"])["wrm"]) {
		t.Fatalf("cursor: %v", c)
	}
	if c := readJSON(".config", "Code", "User", "mcp.json"); !isBridge(asMap(c["servers"])["wrm"]) || asMap(c["servers"])["keep"] == nil || asMap(asMap(c["servers"])["wrm"])["type"] != "stdio" {
		t.Fatalf("vscode: %v", c)
	}
	if c := readJSON(".config", "Claude", "claude_desktop_config.json"); !isBridge(asMap(c["mcpServers"])["wrm"]) || asMap(c["mcpServers"])["fs"] == nil || c["globalShortcut"] != "Ctrl+Space" {
		t.Fatalf("claude desktop: %v", c)
	}
	c := readJSON(".config", "opencode", "opencode.json")
	oc := asMap(asMap(c["mcp"])["wrm"])
	if cmdv, _ := oc["command"].([]interface{}); oc["type"] != "local" || len(cmdv) != 2 || cmdv[1] != bridge || asMap(c["mcp"])["other"] == nil || c["theme"] != "dark" {
		t.Fatalf("opencode: %v", c)
	}
	if m, _ := filepath.Glob(filepath.Join(home, ".config", "opencode", "opencode.json.wrm-backup-*")); len(m) != 1 {
		t.Fatalf("no backup of the edited file: %v", m)
	}

	// the configured apps start the bridge exactly like this: it reaches WRM with the saved connection
	p := startBridge(t, "HOME="+home, "WRM_MCP_URL=", "WRM_MCP_TOKEN=", "WRM_MCP_CONFIG=")
	p.send(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{"protocolVersion": "2025-06-18", "capabilities": map[string]interface{}{}}})
	if m := p.next(); !strings.Contains(fmt.Sprint(asMap(asMap(m["result"])["serverInfo"])["title"]), "Remote Manager") {
		t.Fatalf("bridge with the saved connection: %v", m)
	}

	// without a token: sign-in in the browser (the test approves it)
	home2 := t.TempDir()
	setup := exec.Command(node, bridge, "--setup", "--url", e.srv, "--apps", "none")
	setup.Env = []string{"HOME=" + home2, "PATH=" + bin, "WRM_MCP_NO_BROWSER=1"}
	stdout, _ := setup.StdoutPipe()
	serr := &lockedBuffer{}
	setup.Stderr = serr
	if err := setup.Start(); err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(stdout)
	link := ""
	for link == "" && sc.Scan() {
		link = regexp.MustCompile(`http://\S+/oauth/authorize\?\S+`).FindString(sc.Text())
	}
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := noRedirect.Get(link)
	if err != nil {
		t.Fatalf("link %q: %v; %s", link, err, serr.String())
	}
	r.Body.Close()
	id := strings.TrimPrefix(r.Header.Get("Location"), "/?mcp_authorize=")
	_, v := e.user.aiDo("POST", "/api/mcp/authorize/"+id, map[string]interface{}{"decision": "approve", "conn_ids": []int{e.connID}, "mode": "read_only", "scopes": []string{"read_logs"}, "hours": 1})
	cb, err := http.Get(fmt.Sprint(asMap(v)["redirect"]))
	if err != nil {
		t.Fatal(err)
	}
	cb.Body.Close()
	rest, _ := io.ReadAll(stdout)
	if err := setup.Wait(); err != nil || !strings.Contains(string(rest), "Saved the connection") {
		t.Fatalf("setup with sign-in: %v %s %s", err, rest, serr.String())
	}
	b, _ = os.ReadFile(filepath.Join(home2, ".wrm", "mcp.json"))
	if !strings.Contains(string(b), `"token": "wrm_`) {
		t.Fatalf("connection file after sign-in: %s", b)
	}
}
