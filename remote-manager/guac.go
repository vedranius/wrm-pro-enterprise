// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

// ─── REMOTE DESKTOP: RDP / VNC / TELNET via guacd ────
//
// WRM speaks the Guacamole protocol to guacd, the proxy daemon of Apache Guacamole that
// implements RDP, VNC and Telnet, and relays it over a WebSocket to the browser, where
// guacamole-common-js draws the remote screen and sends keyboard and mouse input:
//
//   browser ──/ws/desktop (WebSocket)── WRM ──TCP 4822── guacd ──RDP / VNC / Telnet── server
//
// The browser never chooses the target. WRM performs the handshake itself with the
// stored connection (host, port, user, password, domain and options); afterwards only
// display and input instructions pass, and input instructions are checked against an
// allow-list. Behind a jump host WRM opens a temporary tunnel through the jump hosts and
// gives guacd its local address, so guacd must run on the WRM machine (or in the same
// network namespace — see docker-compose.yml).
//
// The display stream (guacd → browser) is the session recording: it is stored like
// terminal recordings (gzip, SHA-256 in the database) and replayed in the browser.

const guacRecordingFormat = "guacamole+gzip"

var errGuacSyntax = errors.New("guacamole protocol error")

// guacEncode builds one instruction: LENGTH.VALUE,LENGTH.VALUE,…; (lengths in characters).
func guacEncode(op string, args ...string) []byte {
	var b bytes.Buffer
	el := func(s string) {
		b.WriteString(strconv.Itoa(utf8.RuneCountInString(s)))
		b.WriteByte('.')
		b.WriteString(s)
	}
	el(op)
	for _, a := range args {
		b.WriteByte(',')
		el(a)
	}
	b.WriteByte(';')
	return b.Bytes()
}

// guacReader reads complete instructions from a stream.
type guacReader struct {
	r   *bufio.Reader
	max int
}

func newGuacReader(r io.Reader) *guacReader {
	return &guacReader{r: bufio.NewReaderSize(r, 64*1024), max: 4 << 20}
}

// next returns the raw bytes of the next instruction and its elements (opcode first).
func (g *guacReader) next() ([]byte, []string, error) {
	var raw bytes.Buffer
	var elems []string
	for {
		n, digits := 0, 0
		for {
			c, err := g.r.ReadByte()
			if err != nil {
				return nil, nil, err
			}
			raw.WriteByte(c)
			if c == '.' {
				break
			}
			if c < '0' || c > '9' || digits >= 9 {
				return nil, nil, errGuacSyntax
			}
			n = n*10 + int(c-'0')
			digits++
		}
		if digits == 0 || raw.Len()+n > g.max {
			return nil, nil, errGuacSyntax
		}
		start := raw.Len()
		for i := 0; i < n; i++ { // n characters, each 1–4 bytes of UTF-8
			b0, err := g.r.ReadByte()
			if err != nil {
				return nil, nil, err
			}
			raw.WriteByte(b0)
			extra := 0
			switch {
			case b0 >= 0xF0:
				extra = 3
			case b0 >= 0xE0:
				extra = 2
			case b0 >= 0xC0:
				extra = 1
			}
			for ; extra > 0; extra-- {
				b, err := g.r.ReadByte()
				if err != nil {
					return nil, nil, err
				}
				raw.WriteByte(b)
			}
		}
		elems = append(elems, string(raw.Bytes()[start:]))
		term, err := g.r.ReadByte()
		if err != nil {
			return nil, nil, err
		}
		raw.WriteByte(term)
		switch term {
		case ';':
			return raw.Bytes(), elems, nil
		case ',':
		default:
			return nil, nil, errGuacSyntax
		}
	}
}

// parseGuacMessage splits a WebSocket message from the browser into instructions.
func parseGuacMessage(msg []byte) ([][]string, [][]byte, error) {
	g := &guacReader{r: bufio.NewReader(bytes.NewReader(msg)), max: 4 << 20}
	var all [][]string
	var raws [][]byte
	for {
		raw, el, err := g.next()
		if err == io.EOF && len(all) > 0 {
			return all, raws, nil
		}
		if err != nil {
			return nil, nil, errGuacSyntax
		}
		all = append(all, el)
		raws = append(raws, append([]byte(nil), raw...))
	}
}

// Instructions the browser may send after the handshake (input, clipboard, file and
// audio streams, acknowledgements). Everything else is dropped.
var guacClientOps = map[string]bool{
	"ack": true, "argv": true, "audio": true, "blob": true, "clipboard": true, "disconnect": true, "end": true,
	"file": true, "get": true, "key": true, "mouse": true, "nest": true, "nop": true, "pipe": true, "put": true,
	"size": true, "sync": true, "touch": true,
}

// Input instructions (not allowed for read-only viewers).
var guacInputOps = map[string]bool{"key": true, "mouse": true, "touch": true, "clipboard": true, "file": true, "pipe": true, "put": true, "argv": true, "audio": true}

func isDesktopProtocol(p string) bool {
	switch strings.ToUpper(p) {
	case "RDP", "VNC", "TELNET":
		return true
	}
	return false
}

// ─── connection options ───────────────────────────────

// desktopOptionSpec lists the options a connection may set, with allowed values ("" = free text).
var desktopOptionSpec = map[string]map[string][]string{
	"RDP": {
		"domain": nil, "security": {"any", "nla", "nla-ext", "tls", "rdp", "vmconnect"}, "ignore_cert": {"true", "false"},
		"console": {"true", "false"}, "color_depth": {"8", "16", "24", "32"}, "resize": {"display-update", "reconnect", "none"},
		"layout": {"en-us-qwerty", "en-gb-qwerty", "de-de-qwertz", "de-ch-qwertz", "fr-fr-azerty", "fr-ch-qwertz", "fr-be-azerty",
			"it-it-qwerty", "es-es-qwerty", "es-latam-qwerty", "pt-br-qwerty", "pt-pt-qwerty", "sv-se-qwerty", "da-dk-qwerty",
			"no-no-qwerty", "fi-fi-qwerty", "hu-hu-qwertz", "ja-jp-qwerty", "tr-tr-qwerty", "failsafe"},
		"audio": {"true", "false"}, "clipboard": {"true", "false"}, "wallpaper": {"true", "false"}, "initial_program": nil,
		"gateway_host": nil, "gateway_port": nil, "gateway_user": nil, "gateway_domain": nil,
	},
	"VNC": {
		"color_depth": {"8", "16", "24", "32"}, "read_only": {"true", "false"}, "clipboard": {"true", "false"},
		"cursor": {"local", "remote"},
	},
	"TELNET": {
		"font_size": {"10", "11", "12", "13", "14", "16", "18", "20"}, "color_scheme": {"gray-black", "black-white", "green-black", "white-black"},
		"username_regex": nil, "password_regex": nil,
	},
}

// normalizeDesktopOptions keeps the options valid for the protocol.
func normalizeDesktopOptions(protocol string, in map[string]string) (map[string]string, error) {
	spec := desktopOptionSpec[strings.ToUpper(protocol)]
	out := map[string]string{}
	for k, v := range in {
		v = strings.TrimSpace(v)
		allowed, ok := spec[k]
		if !ok || v == "" {
			continue
		}
		if len(v) > 200 || strings.ContainsAny(v, "\r\n") {
			return nil, fmt.Errorf("option %s is too long", k)
		}
		if allowed != nil {
			found := false
			for _, a := range allowed {
				found = found || a == v
			}
			if !found {
				return nil, fmt.Errorf("invalid value for %s", k)
			}
		}
		if k == "gateway_port" {
			if p, err := strconv.Atoi(v); err != nil || p < 1 || p > 65535 {
				return nil, fmt.Errorf("invalid gateway port")
			}
		}
		out[k] = v
	}
	return out, nil
}

func loadDesktopOptions(connID int) map[string]string {
	var raw string
	db.QueryRow(`SELECT COALESCE(options,'') FROM connections WHERE id=?`, connID).Scan(&raw)
	out := map[string]string{}
	if raw != "" {
		json.Unmarshal([]byte(raw), &out)
	}
	return out
}

// guacParams builds the guacd connection parameters for c.
func guacParams(c Connection, opts map[string]string, host string, port int, tz string) map[string]string {
	on := func(k string, def bool) bool {
		if v, ok := opts[k]; ok {
			return v == "true"
		}
		return def
	}
	tf := func(b bool) string {
		if b {
			return "true"
		}
		return ""
	}
	p := map[string]string{"hostname": host, "port": strconv.Itoa(port), "username": c.Username, "password": c.Password}
	switch strings.ToUpper(c.Protocol) {
	case "RDP":
		p["domain"] = opts["domain"]
		p["security"] = firstNonEmpty(opts["security"], "any")
		p["ignore-cert"] = tf(on("ignore_cert", true))
		p["console"] = tf(on("console", false))
		p["color-depth"] = opts["color_depth"]
		p["resize-method"] = firstNonEmpty(opts["resize"], "display-update")
		if opts["resize"] == "none" {
			p["resize-method"] = ""
		}
		p["server-layout"] = opts["layout"]
		p["disable-audio"] = tf(!on("audio", true))
		p["disable-copy"] = tf(!on("clipboard", true))
		p["disable-paste"] = tf(!on("clipboard", true))
		p["enable-wallpaper"] = tf(on("wallpaper", false))
		p["enable-font-smoothing"] = "true"
		p["initial-program"] = opts["initial_program"]
		p["client-name"] = "WRM"
		p["timezone"] = tz
		p["gateway-hostname"] = opts["gateway_host"]
		p["gateway-port"] = opts["gateway_port"]
		p["gateway-username"] = opts["gateway_user"]
		p["gateway-domain"] = opts["gateway_domain"]
		if opts["gateway_host"] != "" { // the RD Gateway uses the same password
			p["gateway-password"] = c.Password
		}
	case "VNC":
		p["color-depth"] = opts["color_depth"]
		p["read-only"] = tf(on("read_only", false))
		p["cursor"] = firstNonEmpty(opts["cursor"], "local")
		if p["cursor"] == "local" {
			p["cursor"] = ""
		}
		p["disable-copy"] = tf(!on("clipboard", true))
		p["disable-paste"] = tf(!on("clipboard", true))
		p["autoretry"] = "2"
		p["clipboard-encoding"] = "UTF-8"
	case "TELNET":
		p["font-size"] = firstNonEmpty(opts["font_size"], "12")
		p["color-scheme"] = opts["color_scheme"]
		p["terminal-type"] = "xterm-256color"
		p["scrollback"] = "2000"
		p["username-regex"] = opts["username_regex"]
		p["password-regex"] = opts["password_regex"]
	}
	return p
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// guacHandshake selects the protocol, answers guacd's "args" with the parameters and
// waits for "ready". It returns guacd's connection id and the protocol version.
func guacHandshake(conn net.Conn, gr *guacReader, protocol string, params map[string]string, width, height, dpi int, tz string) (string, string, error) {
	conn.SetDeadline(time.Now().Add(20 * time.Second))
	defer conn.SetDeadline(time.Time{})
	if _, err := conn.Write(guacEncode("select", strings.ToLower(protocol))); err != nil {
		return "", "", err
	}
	_, el, err := gr.next()
	if err != nil {
		return "", "", fmt.Errorf("guacd: %v", err)
	}
	if el[0] == "error" {
		return "", "", fmt.Errorf("guacd: %s", strings.Join(el[1:], " "))
	}
	if el[0] != "args" {
		return "", "", fmt.Errorf("guacd: unexpected %q", el[0])
	}
	version := ""
	values := make([]string, len(el)-1)
	for i, name := range el[1:] {
		if strings.HasPrefix(name, "VERSION_") {
			version, values[i] = name, name
			continue
		}
		values[i] = params[name]
	}
	var hs bytes.Buffer
	hs.Write(guacEncode("size", strconv.Itoa(width), strconv.Itoa(height), strconv.Itoa(dpi)))
	hs.Write(guacEncode("audio", "audio/L8", "audio/L16"))
	hs.Write(guacEncode("video"))
	hs.Write(guacEncode("image", "image/png", "image/jpeg", "image/webp"))
	if version != "" && tz != "" { // protocol 1.1+
		hs.Write(guacEncode("timezone", tz))
	}
	hs.Write(guacEncode("connect", values...))
	if _, err := conn.Write(hs.Bytes()); err != nil {
		return "", "", err
	}
	for {
		_, el, err = gr.next()
		if err != nil {
			return "", "", fmt.Errorf("guacd: %v", err)
		}
		switch el[0] {
		case "ready":
			id := ""
			if len(el) > 1 {
				id = el[1]
			}
			return id, version, nil
		case "error":
			return "", "", fmt.Errorf("%s", guacErrorText(el))
		}
	}
}

// guacErrorText turns an "error" instruction into a readable message.
func guacErrorText(el []string) string {
	msg, code := "", 0
	if len(el) > 1 {
		msg = el[1]
	}
	if len(el) > 2 {
		code, _ = strconv.Atoi(el[2])
	}
	known := map[int]string{
		512: "remote desktop proxy error", 513: "the remote desktop proxy is busy", 514: "the server did not respond in time",
		515: "the server reported an error", 516: "not found", 518: "the connection was closed", 519: "the server is not reachable",
		520: "the server is unavailable", 521: "another session took over", 522: "the session timed out", 523: "the session was closed",
		769: "login failed (wrong user name or password)", 771: "access denied by the server", 776: "the browser did not respond in time",
	}
	if k, ok := known[code]; ok {
		if msg != "" && !strings.EqualFold(msg, k) {
			return k + " (" + msg + ")"
		}
		return k
	}
	if msg == "" {
		return "remote desktop error " + strconv.Itoa(code)
	}
	return msg
}

// guacdCheck tells whether guacd answers (for the admin overview and error messages).
func guacdCheck() (string, error) {
	addr := getSetting("guacd_address")
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	conn.Write(guacEncode("select", "rdp"))
	_, el, err := newGuacReader(conn).next()
	if err != nil {
		return "", err
	}
	if el[0] != "args" {
		return "", fmt.Errorf("unexpected answer %q", el[0])
	}
	if len(el) > 1 && strings.HasPrefix(el[1], "VERSION_") {
		return strings.ReplaceAll(strings.TrimPrefix(el[1], "VERSION_"), "_", "."), nil
	}
	return "1.0", nil
}

// ─── recording ────────────────────────────────────────

type guacRecorder struct {
	relPath   string
	file      *os.File
	gz        *gzip.Writer
	sum       hash.Hash
	stored    int64
	data      int64
	max       int64
	truncated bool
	start     time.Time
}

func newGuacRecorder(uid string, maxBytes int64) (*guacRecorder, error) {
	now := time.Now()
	rel := filepath.ToSlash(filepath.Join(now.UTC().Format("2006"), now.UTC().Format("01"), uid+".guac.gz"))
	abs := filepath.Join(recordingsDir(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	g := &guacRecorder{relPath: rel, file: f, sum: sha256.New(), max: maxBytes, start: now}
	g.gz, _ = gzip.NewWriterLevel(countWriter{io.MultiWriter(f, g.sum), &g.stored}, gzip.BestSpeed)
	return g, nil
}

func (g *guacRecorder) write(raw []byte) {
	if g.truncated {
		return
	}
	if g.max > 0 && g.stored >= g.max {
		g.truncated = true
		return
	}
	n, _ := g.gz.Write(raw)
	g.data += int64(n)
}

func (g *guacRecorder) close() recordingInfo {
	g.gz.Close()
	g.file.Close()
	return recordingInfo{RelPath: g.relPath, Size: g.stored, DataBytes: g.data, SHA256: hex.EncodeToString(g.sum.Sum(nil)),
		DurationMs: time.Since(g.start).Milliseconds(), Truncated: g.truncated}
}

// ─── WebSocket ───────────────────────────────────────

var desktopUpgrader = websocket.Upgrader{CheckOrigin: checkWSOrigin, ReadBufferSize: 32 * 1024, WriteBufferSize: 128 * 1024,
	Subprotocols: []string{"guacamole"}}

// GET /ws/desktop?id=&width=&height=&dpi=&tz=[&share_token=]
func desktopWSHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id, _ := strconv.Atoi(q.Get("id"))
	acc, _, denyMsg := authorizeConnection(r, id, PermTerminal)
	ws, err := desktopUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	ws.SetReadLimit(8 << 20)
	var wsMu sync.Mutex
	send := func(b []byte) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		ws.SetWriteDeadline(time.Now().Add(30 * time.Second))
		return ws.WriteMessage(websocket.TextMessage, b)
	}
	fail := func(msg string, code int) {
		send(guacEncode("", "wrm"))
		send(guacEncode("error", msg, strconv.Itoa(code)))
		time.Sleep(200 * time.Millisecond)
	}
	if acc == nil {
		fail(denyMsg, 771)
		return
	}
	if !settingBool("desktop_enabled") {
		fail("Remote desktop is turned off by the administrator.", 771)
		return
	}
	c, err := loadConnection(id)
	if err != nil || !isDesktopProtocol(c.Protocol) {
		fail("This is not a remote desktop connection.", 768)
		return
	}
	if c.authErr != "" {
		fail(c.authErr, 769)
		return
	}
	width, _ := strconv.Atoi(q.Get("width"))
	height, _ := strconv.Atoi(q.Get("height"))
	dpi, _ := strconv.Atoi(q.Get("dpi"))
	if width < 200 || width > 8192 {
		width = 1280
	}
	if height < 150 || height > 8192 {
		height = 800
	}
	if dpi < 48 || dpi > 480 {
		dpi = 96
	}
	tz := q.Get("tz")
	if len(tz) > 64 || strings.ContainsAny(tz, " \r\n,;") {
		tz = ""
	}

	touchQuick(c.ID)
	ta := startDesktopSession(r, acc, c)
	actorID, actorName := acc.actor()
	route := jumpPath(c)

	// Target address as guacd sees it: the server itself, or a tunnel through the jump hosts.
	addr := ensurePort(c.Host, c.Protocol)
	host, portS, err := net.SplitHostPort(addr)
	if err != nil {
		ta.failed("invalid host")
		fail("Invalid host in the connection.", 768)
		return
	}
	port, _ := strconv.Atoi(portS)
	var tunnelKey string
	if needsRoute(c) {
		// guacd connects to a local relay that goes through the jump hosts and / or the proxy.
		tunnelKey = "g" + ta.UID
		def := tunnelDef{ConnID: c.ID, UserID: c.UserID, Name: c.Name, Kind: "local", BindHost: firstNonEmpty(getSetting("desktop_tunnel_bind"), "127.0.0.1"),
			TargetHost: host, TargetPort: port, StartMode: "manual"}
		t, err := tunnelMgr.startEphemeral(tunnelKey, def, c, c.UserID, "desktop", r)
		if err != nil {
			ta.failed(err.Error())
			fail("Route "+route+": "+err.Error(), 519)
			return
		}
		defer tunnelMgr.stop(tunnelKey, 0, "desktop session ended")
		lh, lp, _ := net.SplitHostPort(t.view().Listen)
		if lh == "0.0.0.0" || lh == "::" || lh == "" {
			lh = "127.0.0.1"
		}
		host = lh
		port, _ = strconv.Atoi(lp)
	}

	gconn, err := net.DialTimeout("tcp", getSetting("guacd_address"), 5*time.Second)
	if err != nil {
		// On Windows and macOS WRM can start the official guacd container itself.
		// Give a stopped container a few seconds, then let the first click work.
		if ok, perr := guacdPrepare(12 * time.Second); ok {
			gconn, err = net.DialTimeout("tcp", getSetting("guacd_address"), 5*time.Second)
		} else {
			guacdStartAsync()
			ta.failed("guacd not reachable: " + err.Error())
			fail(guacdFailureMessage(perr), 512)
			return
		}
	}
	if err != nil {
		ta.failed("guacd not reachable: " + err.Error())
		fail(guacdFailureMessage(err), 512)
		return
	}
	defer gconn.Close()
	gr := newGuacReader(gconn)
	params := guacParams(c, loadDesktopOptions(c.ID), host, port, tz)
	if _, _, err := guacHandshake(gconn, gr, c.Protocol, params, width, height, dpi, tz); err != nil {
		ta.failed(err.Error())
		fail(err.Error(), 512)
		return
	}

	// Register like a terminal: administrators see it and can end it; revocation applies.
	ts := &termSession{ID: randomToken(9), UserID: actorID, User: actorName, ConnID: c.ID, ConnName: c.Name, Host: c.Host, IP: clientIP(r), Started: time.Now()}
	if acc.Share != nil {
		ts.ShareID, ts.Share, ts.PKey, ts.Ctx = acc.Share.Share.ID, acc.Share.Share.Name, acc.Share.PKey, acc.Share.Ctx
	}
	var endStatus atomic.Value
	endStatus.Store("closed")
	var once sync.Once
	ts.kill = func(reason string) {
		once.Do(func() {
			endStatus.Store("killed")
			send(guacEncode("error", reason, "523"))
			gconn.Close()
			ws.Close()
		})
	}
	registerTerminal(ts)
	defer unregisterTerminal(ts.ID)
	recorded := ta.desktopConnected(c.Protocol, route)
	defer func() { ta.desktopEnd(endStatus.Load().(string)) }()

	// The first message tells guacamole-common-js the tunnel is open (internal opcode + id).
	if send(guacEncode("", ta.UID)) != nil {
		return
	}
	// WRM's own instruction (ignored by guacamole-common-js, read by the WRM page): session info.
	send(guacEncode("wrm-session", strconv.FormatInt(ta.ID, 10), strconv.FormatBool(recorded), route))

	done := make(chan struct{})
	var doneOnce sync.Once
	finish := func() { doneOnce.Do(func() { close(done) }) }

	// guacd → browser: complete instructions, batched into WebSocket messages.
	go func() {
		defer finish()
		var batch bytes.Buffer
		for {
			raw, el, err := gr.next()
			if err != nil {
				if batch.Len() > 0 {
					send(batch.Bytes())
				}
				return
			}
			ta.desktopOutput(raw)
			if el[0] == "error" {
				msg := guacErrorText(el)
				auditLogRef(r, actorID, actorName, "desktop.error", c.Name, map[string]interface{}{"error": truncateStr(msg, 300)}, ta.ref())
				endStatus.Store("failed")
			}
			batch.Write(raw)
			if batch.Len() >= 16*1024 || gr.r.Buffered() == 0 {
				if send(batch.Bytes()) != nil {
					return
				}
				batch.Reset()
			}
			if el[0] == "disconnect" {
				return
			}
		}
	}()

	// browser → guacd: allowed instructions only; internal pings are answered here.
	go func() {
		defer finish()
		for {
			_, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			ins, raws, err := parseGuacMessage(msg)
			if err != nil {
				continue
			}
			for i, el := range ins {
				switch {
				case el[0] == "": // internal: ping → pong
					if len(el) > 1 && el[1] == "ping" {
						send(raws[i])
					}
				case guacClientOps[el[0]]:
					if guacInputOps[el[0]] {
						ta.BytesIn.Add(int64(len(raws[i])))
					}
					if _, err := gconn.Write(raws[i]); err != nil {
						return
					}
				}
			}
		}
	}()
	<-done
	gconn.Close()
	ws.Close()
}

// ─── session audit ───────────────────────────────────

var desktopRecs sync.Map // session uid → *guacRecorder

func startDesktopSession(r *http.Request, acc *connAccess, c Connection) *termAudit {
	t := startTerminalSession(r, acc, c)
	db.Exec(`UPDATE terminal_sessions SET protocol=? WHERE id=?`, strings.ToLower(c.Protocol), t.ID)
	return t
}

func (t *termAudit) desktopConnected(protocol, route string) bool {
	db.Exec(`UPDATE terminal_sessions SET status='active', jump_path=? WHERE id=?`, route, t.ID)
	d := map[string]interface{}{"host": t.conn.Host, "user": t.conn.Username, "protocol": strings.ToLower(protocol)}
	if route != "" {
		d["route"] = route
	}
	if t.shareID > 0 {
		d["share_id"] = t.shareID
	}
	if level, on := userRecordingLevel(t.userID); on && t.ID > 0 {
		rec, err := newGuacRecorder(t.UID, int64(settingInt("recording_max_mb"))<<20)
		if err != nil {
			log.Printf("recording: %v", err)
			d["recording_error"] = err.Error()
		} else {
			desktopRecs.Store(t.UID, rec)
			d["recorded"] = true
			d["recording_level"] = level
		}
	}
	auditLogRef(t.r, t.userID, t.user, "desktop.open", t.conn.Name, d, t.ref())
	_, ok := desktopRecs.Load(t.UID)
	return ok
}

func (t *termAudit) desktopOutput(raw []byte) {
	t.BytesOu.Add(int64(len(raw)))
	if v, ok := desktopRecs.Load(t.UID); ok {
		v.(*guacRecorder).write(raw)
	}
}

func (t *termAudit) desktopEnd(status string) {
	if !t.ended.CompareAndSwap(false, true) {
		return
	}
	d := map[string]interface{}{"host": t.conn.Host, "status": status, "seconds": int(time.Since(t.start).Seconds()),
		"bytes_out": t.BytesOu.Load(), "bytes_in": t.BytesIn.Load()}
	if v, ok := desktopRecs.LoadAndDelete(t.UID); ok {
		info := v.(*guacRecorder).close()
		db.Exec(`INSERT INTO session_recordings (session_id, format, path, size_bytes, data_bytes, sha256, duration_ms,
			input_recorded, truncated, created_at) VALUES (?,?,?,?,?,?,?,0,?,?)`,
			t.ID, guacRecordingFormat, info.RelPath, info.Size, info.DataBytes, info.SHA256, info.DurationMs,
			boolInt(info.Truncated), time.Now().UTC().Format(time.RFC3339))
		d["recording_sha256"] = info.SHA256
		d["recording_bytes"] = info.Size
	}
	db.Exec(`UPDATE terminal_sessions SET status=?, ended_at=?, bytes_in=?, bytes_out=? WHERE id=?`,
		status, time.Now().UTC().Format(time.RFC3339), t.BytesIn.Load(), t.BytesOu.Load(), t.ID)
	auditLogRef(t.r, t.userID, t.user, "desktop.close", t.conn.Name, d, t.ref())
}
