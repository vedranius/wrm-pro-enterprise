// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestV1244MCPGrantEditKeepsTokenAndInvalidatesSessions(t *testing.T) {
	e := newMCPEnv(t)
	tok, id := e.pat("read_only", []string{"read_logs", "run_readonly"}, nil)
	second := e.addConn("demo-stream")
	g, _ := loadGrant(id)
	s, err := aiStart(aiStartParams{UserID: e.user.userID, Username: "demo", ConnID: e.connID, Mode: "read_only", Transport: "mcp", TokenID: id, Scopes: g.Scopes, GrantPolicy: mcpPolicySignature(g)})
	if err != nil {
		t.Fatal(err)
	}
	var hash, expiry string
	db.QueryRow(`SELECT token_hash,expires_at FROM mcp_tokens WHERE id=?`, id).Scan(&hash, &expiry)
	body := map[string]interface{}{"name": "edited", "conn_ids": []int{second}, "mode": "ask", "scopes": []string{"read_logs", "run_with_approval"}}
	code, _ := e.user.aiDo("PUT", fmt.Sprintf("/api/mcp/grants/%d", id), body)
	if code != 200 {
		t.Fatal(code)
	}
	updated, why := mcpLookupBearer(tok)
	if why != "" || !updated.hasConn(second) || updated.hasConn(e.connID) {
		t.Fatalf("token/scope: %+v %s", updated, why)
	}
	var nextHash, nextExpiry string
	db.QueryRow(`SELECT token_hash,expires_at FROM mcp_tokens WHERE id=?`, id).Scan(&nextHash, &nextExpiry)
	if hash != nextHash || expiry != nextExpiry {
		t.Fatal("edit replaced bearer or expiry")
	}
	if ok, _ := s.userAllowed(); ok {
		t.Fatal("removed connection remained authorized")
	}
	res := s.CallTool(context.Background(), aiToolCall{ID: "old", Name: "system_info", Input: json.RawMessage(`{}`)})
	if !res.IsError {
		t.Fatal("old session still executes")
	}
	if code, _ := e.admin.aiDo("PUT", fmt.Sprintf("/api/mcp/grants/%d", id), body); code != 404 {
		t.Fatal("foreign owner edit allowed", code)
	}
}
func TestV1244VisualLabelsPersistenceValidationAndImport(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "labels-owner", false)
	other := newTestUser(t, srv, "labels-copy", false)
	v := &visualLabel{Color: "#1a2B3c", Icon: "broadcast"}
	var f Folder
	if code := u.jsonDo("POST", "/api/folders", map[string]interface{}{"name": "Demo operations", "label": v}, &f); code != 201 || f.Label == nil || f.Label.Icon != v.Icon {
		t.Fatalf("folder label %d %+v", code, f)
	}
	var c connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "demo-video", "host": "example.com", "protocol": "SSH", "label": v, "folder_id": f.ID}, &c); code != 201 || c.Label == nil {
		t.Fatalf("connection %d %+v", code, c)
	}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/connections/%d", c.ID), map[string]interface{}{"name": c.Name, "host": c.Host, "protocol": "SSH", "label": map[string]string{"color": "red;display:none", "icon": "broadcast"}}, nil); code != 400 {
		t.Fatal("CSS injection accepted", code)
	}
	var export struct {
		Folders     []Folder     `json:"folders"`
		Connections []Connection `json:"connections"`
	}
	u.jsonDo("GET", "/api/config/export", nil, &export)
	if len(export.Connections) != 1 || export.Connections[0].Label == nil {
		t.Fatal("export lost connection label")
	}
	if code := other.jsonDo("POST", "/api/config/import", export, nil); code != 200 {
		t.Fatal(code)
	}
	fs := loadFolders(other.userID)
	cs := loadUserConnections(other.userID)
	if fs[0].Label.Icon != v.Icon || cs[0].Label.Color != v.Color {
		t.Fatal("labels lost in import")
	}
	if code := other.jsonDo("PUT", fmt.Sprintf("/api/folders/%d", f.ID), map[string]interface{}{"name": "foreign", "label": v}, nil); code != 404 {
		t.Fatal("foreign edit", code)
	}
}
func TestV1244ProviderIdleCancellationAndLimits(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	body := newAIIdleBody(reader, 25*time.Millisecond)
	defer body.Close()
	start := time.Now()
	_, err := body.Read(make([]byte, 16))
	if err == nil || !strings.Contains(err.Error(), "stream interrupted") || time.Since(start) > time.Second {
		t.Fatal("idle reader did not cancel", err)
	}
	r, w := io.Pipe()
	body = newAIIdleBody(r, 100*time.Millisecond)
	defer body.Close()
	go func() { w.Write([]byte("hello")); w.Close() }()
	b, err := io.ReadAll(body)
	if err != nil || string(b) != "hello" {
		t.Fatal("active stream failed", err)
	}
}
func TestV1244ZabbixPassiveLegacyAndJSON(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprint(legacy), func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			var wg sync.WaitGroup
			done := make(chan struct{})
			defer close(done)
			go func() {
				for {
					conn, err := ln.Accept()
					if err != nil {
						return
					}
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer conn.Close()
						b, e := zabbixRead(conn)
						if e != nil {
							return
						}
						var q struct {
							Data []struct {
								Key string `json:"key"`
							} `json:"data"`
						}
						key := strings.TrimSpace(string(b))
						isJSON := json.Unmarshal(b, &q) == nil && len(q.Data) > 0
						if isJSON {
							key = q.Data[0].Key
						}
						value := ""
						switch key {
						case "agent.ping":
							value = "1"
						case "agent.version":
							value = "7.4.0"
						case "system.hostname":
							value = "demo-node"
						case "system.cpu.util[,idle]":
							value = "40"
						case "vm.memory.size[total]":
							value = "1000"
						case "vm.memory.size[available]":
							value = "250"
						case "system.uptime":
							value = "300"
						case "vfs.fs.get":
							value = `[{"fsname":"/","bytes":{"total":1000,"used":300,"pused":30}}]`
						case "net.if.discovery":
							value = `[{"{#IFNAME}":"eth0"}]`
						default:
							value = "20"
						}
						if legacy && isJSON {
							conn.Write(zabbixFrame([]byte("ZBX_NOTSUPPORTED\x00Unsupported item key.")))
							return
						}
						if isJSON {
							conn.Write(zabbixFrame(jsonMarshal(map[string]interface{}{"version": "7.4.0", "data": []map[string]interface{}{{"value": value}}})))
						} else {
							conn.Write(zabbixFrame([]byte(value)))
						}
					}()
				}
			}()
			cfg := defaultMonitorConfig()
			cfg.Mode = "direct"
			cfg.Address = ln.Addr().String()
			sample := collectMonitor(context.Background(), Connection{ID: 1, Host: "example.com"}, cfg)
			if sample.State != "healthy" || sample.Hostname != "demo-node" || sample.Metrics["cpu_percent"].Value == nil || *sample.Metrics["cpu_percent"].Value != 60 || *sample.Metrics["memory_percent"].Value != 75 || *sample.Metrics["disk:/"].Value != 30 {
				t.Fatalf("sample: %+v", sample)
			}
			ln.Close()
			wg.Wait()
		})
	}
}
func TestV1244MonitoringLogRootsSearchRotationAndOwnership(t *testing.T) {
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "monitor-logs", false)
	other := newTestUser(t, srv, "monitor-foreign", false)
	id := u.addConnection("demo-log", h.addr)
	root := filepath.Join(h.home, "logs")
	os.MkdirAll(root, 0700)
	p := filepath.Join(root, "demo.log")
	writeFile(t, p, "2026-10-14T12:00:00Z hello\n2026-10-14T12:01:00Z error event\n continuation error\n")
	cfg := defaultMonitorConfig()
	cfg.Roots = []monitorLogRoot{{Path: root, Service: "demo-api", Operator: "demo-operator"}}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/monitoring/config?id=%d", id), cfg, nil); code != 200 {
		t.Fatal(code)
	}
	endpoint := fmt.Sprintf("/api/monitoring/log?id=%d&root=0&path=%s", id, url.QueryEscape(p))
	var chunk monitorLogChunk
	if code := u.jsonDo("GET", endpoint+"&q=error", nil, &chunk); code != 200 || !strings.Contains(chunk.Text, "event") || chunk.Matches != 2 {
		t.Fatalf("search %d %+v", code, chunk)
	}
	if code := other.jsonDo("GET", endpoint, nil, nil); code != 404 && code != 403 {
		t.Fatal("foreign log access", code)
	}
	writeFile(t, p, "rotated\n")
	var rotated monitorLogChunk
	if code := u.jsonDo("GET", endpoint+fmt.Sprintf("&offset=%d&signature=%s", chunk.Offset, chunk.Signature), nil, &rotated); code != 200 || !rotated.Reset || rotated.Text != "rotated\n" {
		t.Fatalf("rotation %d %+v", code, rotated)
	}
	outside := filepath.Join(h.home, "outside.log")
	writeFile(t, outside, "private")
	os.Symlink(outside, filepath.Join(root, "escape.log"))
	escape := fmt.Sprintf("/api/monitoring/log?id=%d&root=0&path=%s", id, url.QueryEscape(filepath.Join(root, "escape.log")))
	if code := u.jsonDo("GET", escape, nil, nil); code != 403 {
		t.Fatal("symlink escaped root", code)
	}
}
func TestV1244LogTimestampBoundsKeepUnknown(t *testing.T) {
	loc := time.UTC
	from, _ := time.Parse(time.RFC3339, "2026-10-14T12:00:00Z")
	to := from.Add(time.Minute)
	text, unknown, n := filterMonitorLog("unknown first\n2026-10-14T11:00:00Z old\n continuation\n2026-10-14T12:00:20Z ERROR\n continuation ERROR\n", "", from, to, loc)
	if !strings.Contains(text, "unknown first") || strings.Contains(text, "old") || !strings.Contains(text, "continuation ERROR") || unknown < 2 || n != 0 {
		t.Fatalf("filter %q %d", text, unknown)
	}
}

func TestV1244TerminalDirectoryQuotesAndPowerShell(t *testing.T) {
	dir := `/srv/demo space/o'brien;$(touch injected)`
	command, err := terminalDirectoryCommand(dir, false)
	if err != nil || !strings.HasPrefix(command, "sh -c ") {
		t.Fatal(command, err)
	}
	ps, err := terminalDirectoryCommand(`C:\Demo Folder\O'Brien;$(Remove-Item data)`, true)
	if err != nil || !strings.HasPrefix(ps, "powershell.exe -NoLogo -NoExit -EncodedCommand ") {
		t.Fatal(ps, err)
	}
	if strings.Contains(ps, "Remove-Item") || strings.Contains(ps, "O'Brien") {
		t.Fatal("directory text appeared as command arguments")
	}
	if _, err := terminalDirectoryCommand("/tmp/dir\ncommand", false); err == nil {
		t.Fatal("multiline startup accepted")
	}
}
