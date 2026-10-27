// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// askCwd sends {"type":"cwd"} on the terminal WebSocket and waits for the answer.
func (tc *termConn) askCwd() map[string]interface{} {
	tc.t.Helper()
	tc.mu.Lock()
	from := len(tc.ctl)
	tc.mu.Unlock()
	tc.ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"cwd"}`))
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		tc.mu.Lock()
		for _, m := range tc.ctl[from:] {
			if m["type"] == "cwd" {
				tc.mu.Unlock()
				return m
			}
		}
		tc.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	tc.t.Fatal("no cwd answer")
	return nil
}

// TestTerminalCwdLookup opens a terminal to the fake SSH host (a real sh on a PTY) and asks
// for the shell's working directory over a separate exec channel, as the 📂 menu does when
// the shell does not report it with OSC 7.
func TestTerminalCwdLookup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the fake host's shell needs a Linux PTY")
	}
	srv := newTestServer(t)
	u := newTestUser(t, srv, "cwd-user", false)
	h := startFakeHost(t, testSSHUser, testSSHPass)
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	sub := filepath.Join(dir, "with space", "logs")
	os.MkdirAll(sub, 0o755)
	h.set(func() { h.shellDir = dir })
	id := u.addConnection("cwd-host", h.addr)

	tc := u.openTerminal(id)
	defer tc.ws.Close()
	tc.waitFor("$ ")
	if m := tc.askCwd(); m["path"] != dir {
		t.Fatalf("start directory: %v", m)
	}
	tc.send("cd 'with space/logs'\r")
	tc.waitFor("logs")
	time.Sleep(200 * time.Millisecond)
	if m := tc.askCwd(); m["path"] != sub {
		t.Fatalf("after cd: %v", m)
	}
	// A foreground job counts: its directory is where the user is working.
	tc.send("(cd / && sleep 4)\r")
	time.Sleep(500 * time.Millisecond)
	if m := tc.askCwd(); m["path"] != "/" {
		t.Fatalf("foreground job: %v", m)
	}
	tc.send("\x03")

	// Without an interactive shell (a terminal of another kind) the lookup says why.
	if _, err := terminalCwd(nil); err == nil {
		t.Fatal("no SSH connection")
	}
}

func TestParseCwdAnswer(t *testing.T) {
	if p, err := parseCwdAnswer("motd line\nWRMCWD:/srv/app dir\n"); err != nil || p != "/srv/app dir" {
		t.Fatalf("%q %v", p, err)
	}
	for out, want := range map[string]string{
		"WRMCWD-ERR:noshell\n":    "shell was not found",
		"WRMCWD-ERR:unreadable\n": "cannot be read",
		"WRMCWD-ERR:ps\n":         "neither /proc nor ps",
		"sh: 1: awk: not found\n": "unexpected answer",
		"WRMCWD:relative\n":       "unexpected answer",
	} {
		if _, err := parseCwdAnswer(out); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q: %v", out, err)
		}
	}
	if !strings.HasPrefix(cwdLookupCommand, "sh -c '") {
		t.Fatal(cwdLookupCommand)
	}
}
