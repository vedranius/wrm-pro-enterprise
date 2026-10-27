// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

// ─── SSH WEBSOCKET ────────────────────────────────────
//
// Protocol (v9.10):
//   server → client  binary frame : raw terminal output (never split-UTF-8 sensitive)
//                    text frame   : JSON control {"type":"status"|"error"|"exit", ...}
//   client → server  binary frame : keyboard input
//                    text frame   : JSON control {"type":"resize"|"pause"|"resume"|"ping"|"broadcast"|"cwd"}
//                                   (non-JSON text is treated as keyboard input for
//                                    backwards compatibility)
//
// Close codes: 1000 = shell exited normally, 4001 = could not connect,
//              4002 = SSH connection to the server was lost.

const (
	wsCloseConnectFailed = 4001
	wsCloseSSHLost       = 4002

	wsReadTimeout    = 90 * time.Second
	wsPingInterval   = 25 * time.Second
	sshKeepaliveTick = 30 * time.Second
	flowAutoResume   = 5 * time.Second
)

type termControl struct {
	Type  string `json:"type"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
	On    bool   `json:"on"`    // broadcast: this terminal joined (true) or left (false) a broadcast group
	Peers int    `json:"peers"` // broadcast: number of terminals in the group
}

func sshHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	ws.SetReadLimit(4 << 20)

	var wsMu sync.Mutex
	writeWS := func(mt int, data []byte) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		ws.SetWriteDeadline(time.Now().Add(30 * time.Second))
		return ws.WriteMessage(mt, data)
	}
	printTerm := func(s string) { writeWS(websocket.BinaryMessage, []byte(s)) }
	sendCtl := func(v interface{}) { writeWS(websocket.TextMessage, jsonMarshal(v)) }
	closeWS := func(code int, reason string) {
		wsMu.Lock()
		defer wsMu.Unlock()
		ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, truncateStr(reason, 120)), time.Now().Add(2*time.Second))
	}
	var ta *termAudit // audit trail of this terminal (created once the connection is known)
	fail := func(msg string) {
		if ta != nil {
			ta.failed(msg)
		}
		printTerm("\r\n\x1b[31m" + msg + "\x1b[0m\r\n")
		sendCtl(map[string]interface{}{"type": "error", "message": msg})
		closeWS(wsCloseConnectFailed, msg)
	}

	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	acc, _, denyMsg := authorizeConnection(r, id, PermTerminal)
	if acc == nil {
		fail(denyMsg)
		return
	}
	c, err := loadConnection(id)
	if err != nil {
		fail("Connection not found.")
		return
	}
	if isFTP(c) {
		fail("FTP connections do not support a terminal. Use the File Manager instead.")
		return
	}
	if isWeb(c) {
		fail("This is a web interface connection. Open it with 🌐 (double-click) instead of a terminal.")
		return
	}
	if isDesktopProtocol(c.Protocol) {
		fail("This is a remote desktop connection (" + c.Protocol + "). Open it with a double-click.")
		return
	}
	// Consoles (BMC / Serial-over-LAN) and serial ports reach the hardware itself: only the
	// connection's owner opens them, not share members.
	console := r.URL.Query().Get("console")
	serial := c.Protocol == "SERIAL"
	if console != "" && console != "bmc" && console != "sol" {
		fail("Unknown console.")
		return
	}
	if (console != "" || serial) && acc.Share != nil {
		fail("Consoles and serial ports can only be opened by the owner of the connection.")
		return
	}
	if console != "" && !settingBool("bmc_enabled") {
		fail("Out-of-band management is turned off by the administrator.")
		return
	}
	if serial && !serialAllowed(c.UserID) {
		fail("Serial ports of the WRM server are not allowed for this account (policy serial_ports).")
		return
	}
	route := jumpPath(c)
	touchQuick(c.ID)
	ta = startTerminalSession(r, acc, c)
	if route != "" {
		db.Exec(`UPDATE terminal_sessions SET jump_path=? WHERE id=?`, route, ta.ID)
	}
	if kind := map[string]string{"bmc": "console", "sol": "sol"}[console]; kind != "" || serial {
		if serial {
			kind = "serial"
		}
		db.Exec(`UPDATE terminal_sessions SET protocol=? WHERE id=?`, kind, ta.ID)
	}

	// The browser tells us its real size in the URL, so the PTY is correct from the very
	// first byte (nano/vim/less/htop draw correctly without waiting for a resize).
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	haveSize := cols > 0 && rows > 0
	if !haveSize {
		cols, rows = 80, 24
	}
	term := r.URL.Query().Get("term")
	if term == "" {
		term = "xterm-256color"
	}

	sendCtl(map[string]interface{}{"type": "status", "state": "connecting", "host": c.Host, "route": route})
	target := c.Host
	switch {
	case console == "bmc":
		target = "the console of " + c.Name + " (BMC SSH)"
	case console == "sol":
		target = "the console of " + c.Name + " (Serial-over-LAN)"
	}
	if route != "" && !serial {
		printTerm("\r\n\x1b[36mConnecting to " + target + " via " + route + "...\x1b[0m\r\n")
	} else {
		printTerm("\r\n\x1b[36mConnecting to " + target + "...\x1b[0m\r\n")
	}
	onNewKey := func(host, fp string) {
		printTerm("\x1b[33mNew host " + host + " — key " + fp + " saved (trust on first use).\x1b[0m\r\n")
	}
	var tio *termIO
	var sshCl *ssh.Client // the terminal's SSH connection (a cwd lookup opens another channel on it)
	switch {
	case serial:
		tio, err = openSerial(c)
	case console != "":
		tio, err = openConsole(c, console, cols, rows, term, onNewKey)
	default:
		var cl *ssh.Client
		if cl, err = dialSSH(c, onNewKey); err == nil {
			sshCl = cl
			var command string
			command, err = terminalDirectoryStart(cl, c, r.URL.Query().Get("cwd"), r.URL.Query().Get("log_path"))
			if err == nil {
				tio, err = sshTerm(cl, cols, rows, term, command)
			} else {
				cl.Close()
			}
		}
	}
	if err != nil {
		if hk := asHostKeyError(err); hk != nil {
			sendCtl(map[string]interface{}{"type": "hostkey", "kind": hk.Kind, "host": hk.Host, "key_type": hk.KeyType,
				"expected": hk.Expected, "fingerprint": hk.Got, "conn_id": c.ID, "can_accept": acc.Share == nil})
			printTerm("\r\n\x1b[41;97m " + map[string]string{"mismatch": "WARNING: HOST KEY CHANGED", "unknown": "UNKNOWN HOST"}[hk.Kind] + " \x1b[0m\r\n")
			fail(hk.Error())
			return
		}
		fail("Connection failed: " + err.Error())
		return
	}
	defer tio.close()

	// Register the terminal so administrators can see/end it and access can be revoked.
	actorID, actorName := acc.actor()
	ts := &termSession{ID: randomToken(9), UserID: actorID, User: actorName, ConnID: c.ID, ConnName: c.Name, Host: c.Host, IP: clientIP(r), Started: time.Now()}
	if acc.Share != nil {
		ts.ShareID, ts.Share, ts.PKey, ts.Ctx = acc.Share.Share.ID, acc.Share.Share.Name, acc.Share.PKey, acc.Share.Ctx
	}
	var endStatus atomic.Value
	endStatus.Store("closed")
	var finalExit *int
	var killOnce sync.Once
	ts.kill = func(reason string) {
		killOnce.Do(func() {
			endStatus.Store("killed")
			printTerm("\r\n\x1b[31m" + reason + "\x1b[0m\r\n")
			sendCtl(map[string]interface{}{"type": "error", "message": reason})
			wsMu.Lock()
			closeWSRevoked(ws, reason)
			wsMu.Unlock()
			tio.close()
			ws.Close()
		})
	}
	registerTerminal(ts)
	defer unregisterTerminal(ts.ID)
	// Tunnels of this connection that start "on connect" run while a terminal is open.
	if acc.Share == nil && !serial && console == "" {
		tunnelMgr.holdConn(c.ID, actorID, r)
		defer tunnelMgr.releaseConn(c.ID)
	}
	defer func() { ta.end(endStatus.Load().(string), finalExit) }()

	done := make(chan struct{})
	var doneOnce sync.Once
	finish := func() { doneOnce.Do(func() { close(done) }) }

	// ── Flow control: the browser asks us to pause when xterm.js falls behind ──
	var paused atomic.Bool
	resumeCh := make(chan struct{}, 1)
	resume := func() {
		paused.Store(false)
		select {
		case resumeCh <- struct{}{}:
		default:
		}
	}

	// ── WebSocket reader ──
	stdinCh := make(chan []byte, 1024)
	broadcasting := false
	gotSize := make(chan struct{})
	var sizeOnce sync.Once
	readDone := make(chan struct{})
	ws.SetReadDeadline(time.Now().Add(wsReadTimeout))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(wsReadTimeout))
		return nil
	})
	go func() {
		defer close(readDone)
		for {
			mt, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			ws.SetReadDeadline(time.Now().Add(wsReadTimeout))
			if mt == websocket.TextMessage && len(msg) > 1 && msg[0] == '{' {
				var ctl termControl
				if json.Unmarshal(msg, &ctl) == nil && ctl.Type != "" {
					switch ctl.Type {
					case "resize":
						if ctl.Cols > 0 && ctl.Rows > 0 && ctl.Cols < 2000 && ctl.Rows < 1000 {
							tio.resize(ctl.Cols, ctl.Rows)
							ta.resize(ctl.Cols, ctl.Rows)
							sizeOnce.Do(func() { close(gotSize) })
						}
					case "pause":
						paused.Store(true)
					case "resume":
						resume()
					case "ping":
						sendCtl(map[string]string{"type": "pong"})
					case "cwd":
						// The 📂 menu's "Open here" without OSC 7: ask the server for the shell's directory.
						go func() {
							p, err := terminalCwd(sshCl)
							if err != nil {
								sendCtl(map[string]string{"type": "cwd", "error": err.Error()})
								return
							}
							sendCtl(map[string]string{"type": "cwd", "path": p})
						}()
					case "broadcast":
						// Broadcast input is done by the browser (it sends the same keystrokes to
						// every terminal of the group); the server records who did it, where and when.
						if ctl.On && !settingBool("broadcast_enabled") {
							sendCtl(map[string]interface{}{"type": "broadcast", "allowed": false, "message": "Broadcast input is turned off by the administrator."})
							continue
						}
						if ctl.On != broadcasting {
							broadcasting = ctl.On
							actorID, actorName := acc.actor()
							auditLogRef(r, actorID, actorName, "terminal.broadcast", c.Name, map[string]interface{}{"on": ctl.On, "terminals": ctl.Peers},
								auditRef{ConnID: c.ID, SessionID: int(ta.ID)})
						}
					}
					continue
				}
			}
			select {
			case stdinCh <- msg:
			case <-done:
				return
			}
		}
	}()

	// ── stdin writer (decoupled so a slow remote never blocks control messages) ──
	go func() {
		for {
			select {
			case msg := <-stdinCh:
				ta.input(msg)
				if _, err := tio.stdin.Write(msg); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	// Old clients do not send the size in the URL: wait briefly for the first resize.
	if !haveSize {
		select {
		case <-gotSize:
		case <-time.After(2 * time.Second):
		case <-readDone:
			return
		}
	}

	if err := tio.start(); err != nil {
		fail("Shell error: " + err.Error())
		return
	}
	log.Printf("Terminal connected: %s@%s%s", c.Username, c.Host, map[bool]string{true: " (" + console + ")"}[console != ""])
	recorded := ta.connected(cols, rows, term)
	if tio.banner != "" {
		printTerm("\x1b[2m" + tio.banner + "\x1b[0m\r\n")
	}
	if recorded {
		printTerm("\x1b[2m● This session is recorded.\x1b[0m\r\n")
	}
	sendCtl(map[string]interface{}{"type": "status", "state": "connected", "host": c.Host, "recording": recorded, "session_id": ta.ID, "console": firstNonEmpty(console, map[bool]string{true: "serial"}[serial])})

	// ── Output pump ──
	var lastOutput atomic.Int64 // unix nanoseconds of the last output (run on connect waits for a quiet prompt)
	outDone := make(chan struct{})
	pump := func(src interface{ Read([]byte) (int, error) }, signal chan struct{}) {
		if signal != nil {
			defer close(signal)
		}
		buf := make([]byte, 32*1024)
		for {
			if signal != nil && paused.Load() { // only the main stdout pump is flow-controlled
				select {
				case <-resumeCh:
				case <-time.After(flowAutoResume):
					paused.Store(false)
				case <-done:
					return
				}
				continue
			}
			n, err := src.Read(buf)
			if n > 0 {
				lastOutput.Store(time.Now().UnixNano())
				ta.output(buf[:n])
				if writeWS(websocket.BinaryMessage, buf[:n]) != nil {
					finish()
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
	go pump(tio.stdout, outDone)
	if tio.stderr != nil {
		go pump(tio.stderr, nil)
	}

	// ── BMC console: the console command is typed once the BMC's prompt is quiet ──
	if tio.typeCmd != "" {
		go func() {
			start := time.Now()
			for time.Since(start) < 8*time.Second {
				if last := lastOutput.Load(); last > 0 && time.Since(time.Unix(0, last)) >= 600*time.Millisecond {
					break
				}
				select {
				case <-done:
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
			select {
			case stdinCh <- []byte(tio.typeCmd + "\r"):
			case <-done:
			}
		}()
	}

	// ── Run on connect: the start directory bookmark (a cd) and the snippets of the
	// connection's owner, typed once the prompt is quiet ──
	autos := []Snippet{}
	if console == "" && !serial {
		if sb, ok := startBookmark(c); ok {
			autos = append(autos, Snippet{Name: "★ " + sb.Name, Command: sb.Cd, literal: true})
		}
		autos = append(autos, autoRunSnippets(c)...)
	}
	if len(autos) > 0 {
		go func() {
			start := time.Now()
			for time.Since(start) < 5*time.Second {
				if last := lastOutput.Load(); last > 0 && time.Since(time.Unix(0, last)) >= 400*time.Millisecond {
					break
				}
				select {
				case <-done:
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
			names := make([]string, 0, len(autos))
			for _, sn := range autos {
				cmd := sn.Command
				if !sn.literal {
					cmd = expandSnippet(cmd, c, actorName)
				}
				select {
				case stdinCh <- snippetKeystrokes(cmd):
				case <-done:
					return
				}
				names = append(names, sn.Name)
			}
			sendCtl(map[string]interface{}{"type": "autorun", "snippets": names})
			auditLogRef(r, actorID, actorName, "terminal.auto_run", c.Name, map[string]interface{}{"snippets": names},
				auditRef{ConnID: c.ID, SessionID: int(ta.ID)})
		}()
	}

	// ── WebSocket ping + SSH keepalive ──
	var sshLost atomic.Bool
	go func() {
		pingT := time.NewTicker(wsPingInterval)
		kaT := time.NewTicker(sshKeepaliveTick)
		defer pingT.Stop()
		defer kaT.Stop()
		misses := 0
		for {
			select {
			case <-done:
				return
			case <-pingT.C:
				wsMu.Lock()
				err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
				wsMu.Unlock()
				if err != nil {
					finish()
					return
				}
			case <-kaT.C:
				if tio.alive == nil || tio.alive() {
					misses = 0
					continue
				}
				misses++
				if misses >= 3 {
					log.Printf("SSH keepalive failed: %s@%s", c.Username, c.Host)
					sshLost.Store(true)
					tio.close()
					return
				}
			}
		}
	}()

	select {
	case <-outDone:
		// Remote side closed: either the shell exited or the connection dropped.
		exitCode, ok := tio.wait()
		if !ok {
			sshLost.Store(true)
		}
		if sshLost.Load() {
			endStatus.Store("lost")
			if serial {
				printTerm("\r\n\x1b[31mThe serial port was closed.\x1b[0m\r\n")
			} else {
				printTerm("\r\n\x1b[31mSSH connection to the server was lost.\x1b[0m\r\n")
			}
			sendCtl(map[string]interface{}{"type": "exit", "lost": true})
			closeWS(wsCloseSSHLost, "SSH connection lost")
		} else {
			code := exitCode
			finalExit = &code
			sendCtl(map[string]interface{}{"type": "exit", "code": exitCode})
			closeWS(websocket.CloseNormalClosure, "session ended")
		}
	case <-readDone:
	case <-done:
	}
	finish()
	log.Printf("Terminal disconnected: %s@%s", c.Username, c.Host)
}
