// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ─── in-process SSH + SFTP server ────────────────────

const (
	testSSHUser = "tester"
	testSSHPass = "s3cret-pass"
	testSudoPW  = "hunter2-sudo"
)

func startTestSSHServer(t *testing.T) string {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
		if c.User() == testSSHUser && string(pass) == testSSHPass {
			return nil, nil
		}
		return nil, fmt.Errorf("denied")
	}}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	// Every test server has a new host key, but the kernel can hand out a port an earlier
	// test server used; forget the key remembered for that address (trust on first use)
	// so the new server is not refused as a changed host key.
	if db != nil {
		db.Exec(`DELETE FROM known_hosts WHERE host=?`, normalizeHostKeyHost(ln.Addr().String()))
	}
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				sconn, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				go serveGlobalRequests(sconn, reqs)
				for nch := range chans {
					switch nch.ChannelType() {
					case "session":
						ch, creqs, err := nch.Accept()
						if err != nil {
							continue
						}
						go serveTestSession(ch, creqs)
					case "direct-tcpip": // ssh -L / ProxyJump
						var p struct {
							Host     string
							Port     uint32
							OrigHost string
							OrigPort uint32
						}
						if ssh.Unmarshal(nch.ExtraData(), &p) != nil {
							nch.Reject(ssh.ConnectionFailed, "bad payload")
							continue
						}
						out, err := net.DialTimeout("tcp", net.JoinHostPort(p.Host, strconv.Itoa(int(p.Port))), 5*time.Second)
						if err != nil {
							nch.Reject(ssh.ConnectionFailed, err.Error())
							continue
						}
						ch, creqs, err := nch.Accept()
						if err != nil {
							out.Close()
							continue
						}
						directTCPIPCount.Add(1)
						go ssh.DiscardRequests(creqs)
						go testPipe(ch, out)
					default:
						nch.Reject(ssh.UnknownChannelType, "not supported")
					}
				}
			}()
		}
	}()
	return ln.Addr().String()
}

var directTCPIPCount atomic.Int64

func testPipe(a io.ReadWriteCloser, b io.ReadWriteCloser) {
	done := make(chan struct{}, 2)
	go func() { io.Copy(a, b); done <- struct{}{} }()
	go func() { io.Copy(b, a); done <- struct{}{} }()
	<-done
	a.Close()
	b.Close()
}

// serveGlobalRequests implements "tcpip-forward" (ssh -R) and keepalives.
func serveGlobalRequests(sconn *ssh.ServerConn, reqs <-chan *ssh.Request) {
	listeners := map[string]net.Listener{}
	var mu sync.Mutex
	defer func() {
		mu.Lock()
		for _, l := range listeners {
			l.Close()
		}
		mu.Unlock()
	}()
	for req := range reqs {
		switch req.Type {
		case "tcpip-forward":
			var p struct {
				Addr string
				Port uint32
			}
			if ssh.Unmarshal(req.Payload, &p) != nil {
				req.Reply(false, nil)
				continue
			}
			ln, err := net.Listen("tcp", net.JoinHostPort(p.Addr, strconv.Itoa(int(p.Port))))
			if err != nil {
				req.Reply(false, nil)
				continue
			}
			port := uint32(ln.Addr().(*net.TCPAddr).Port)
			mu.Lock()
			listeners[net.JoinHostPort(p.Addr, strconv.Itoa(int(port)))] = ln
			mu.Unlock()
			req.Reply(true, ssh.Marshal(struct{ Port uint32 }{port}))
			go func(addr string, port uint32) {
				for {
					c, err := ln.Accept()
					if err != nil {
						return
					}
					ra := c.RemoteAddr().(*net.TCPAddr)
					ch, creqs, err := sconn.OpenChannel("forwarded-tcpip", ssh.Marshal(struct {
						Addr     string
						Port     uint32
						OrigAddr string
						OrigPort uint32
					}{addr, port, ra.IP.String(), uint32(ra.Port)}))
					if err != nil {
						c.Close()
						continue
					}
					go ssh.DiscardRequests(creqs)
					go testPipe(ch, c)
				}
			}(p.Addr, port)
		case "cancel-tcpip-forward":
			var p struct {
				Addr string
				Port uint32
			}
			ssh.Unmarshal(req.Payload, &p)
			mu.Lock()
			if l := listeners[net.JoinHostPort(p.Addr, strconv.Itoa(int(p.Port)))]; l != nil {
				l.Close()
			}
			mu.Unlock()
			req.Reply(true, nil)
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

func serveTestSession(ch ssh.Channel, reqs <-chan *ssh.Request) {
	for req := range reqs {
		switch req.Type {
		case "pty-req", "env", "window-change":
			if req.WantReply {
				req.Reply(true, nil)
			}
		case "shell":
			req.Reply(true, nil)
			go fakeShell(ch)
		case "subsystem":
			if len(req.Payload) > 4 && string(req.Payload[4:]) == "sftp" {
				req.Reply(true, nil)
				go func() {
					srv, err := sftp.NewServer(ch)
					if err == nil {
						srv.Serve()
					}
					ch.Close()
				}()
			} else {
				req.Reply(false, nil)
			}
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

// fakeShell echoes typed characters, answers commands, and asks for a sudo password
// without echoing it (like a real terminal with echo off).
func fakeShell(ch ssh.Channel) {
	w := func(s string) { ch.Write([]byte(s)) }
	w("Welcome to the test server — č ć ž š đ ✓\r\n$ ")
	var line []byte
	echo, pendingPW := true, false
	buf := make([]byte, 1024)
	for {
		n, err := ch.Read(buf)
		if err != nil {
			return
		}
		for _, b := range buf[:n] {
			if b == '\r' || b == '\n' {
				w("\r\n")
				cmd := string(line)
				line = line[:0]
				switch {
				case pendingPW:
					pendingPW, echo = false, true
					w("password accepted\r\n$ ")
				case cmd == "exit":
					ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
					ch.Close()
					return
				case strings.HasPrefix(cmd, "sudo"):
					w("[sudo] password for tester: ")
					echo, pendingPW = false, true
				default:
					w("you typed: " + cmd + "\r\n$ ")
				}
				continue
			}
			line = append(line, b)
			if echo {
				ch.Write([]byte{b})
			}
		}
	}
}

// ─── helpers ─────────────────────────────────────────

type testClient struct {
	t      *testing.T
	srv    *httptest.Server
	cookie *http.Cookie
	userID int
}

func newTestUser(t *testing.T, srv *httptest.Server, name string, admin bool) *testClient {
	t.Helper()
	h, _ := hashPassword("pw-" + name + "-123456")
	res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?,?,?,?)`,
		name, h, boolInt(admin), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	rec := httptest.NewRecorder()
	if err := createAuthSession(rec, httptest.NewRequest("GET", "/", nil), int(id), ""); err != nil {
		t.Fatal(err)
	}
	var ck *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			ck = c
		}
	}
	return &testClient{t: t, srv: srv, cookie: ck, userID: int(id)}
}

func (c *testClient) addConnection(name, host string) int {
	c.t.Helper()
	res, err := db.Exec(`INSERT INTO connections (name, protocol, host, username, auth_method, password, user_id) VALUES (?,?,?,?,?,?,?)`,
		name, "SSH", host, testSSHUser, "PASSWORD", encryptValue(testSSHPass), c.userID)
	if err != nil {
		c.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func (c *testClient) do(method, path string, body io.Reader, contentType string) *http.Response {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.srv.URL+path, body)
	req.AddCookie(c.cookie)
	req.Header.Set("Origin", c.srv.URL)
	req.Header.Set("X-WRM-Request", "1")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp
}

// termConn is a browser-like terminal WebSocket client.
type termConn struct {
	t      *testing.T
	ws     *websocket.Conn
	mu     sync.Mutex
	out    bytes.Buffer
	ctl    []map[string]interface{}
	closed chan struct{}
}

func (c *testClient) openTerminal(connID int) *termConn {
	c.t.Helper()
	return c.openTerminalQ(connID, "")
}

// openTerminalQ opens a terminal WebSocket with extra query parameters (e.g. "&console=sol").
func (c *testClient) openTerminalQ(connID int, extra string) *termConn {
	c.t.Helper()
	u := "ws" + strings.TrimPrefix(c.srv.URL, "http") + "/ws/ssh?id=" + strconv.Itoa(connID) + "&cols=100&rows=30" + extra
	hdr := http.Header{}
	hdr.Set("Origin", c.srv.URL)
	hdr.Set("Cookie", c.cookie.Name+"="+c.cookie.Value)
	ws, _, err := websocket.DefaultDialer.Dial(u, hdr)
	if err != nil {
		c.t.Fatal(err)
	}
	tc := &termConn{t: c.t, ws: ws, closed: make(chan struct{})}
	go func() {
		defer close(tc.closed)
		for {
			mt, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			tc.mu.Lock()
			if mt == websocket.BinaryMessage {
				tc.out.Write(msg)
			} else {
				var m map[string]interface{}
				if json.Unmarshal(msg, &m) == nil {
					tc.ctl = append(tc.ctl, m)
				}
			}
			tc.mu.Unlock()
		}
	}()
	return tc
}

func (tc *termConn) waitFor(s string) {
	tc.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		tc.mu.Lock()
		ok := strings.Contains(tc.out.String(), s)
		tc.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.t.Fatalf("timeout waiting for %q; output so far:\n%s", s, tc.out.String())
}

func (tc *termConn) send(s string) {
	tc.ws.WriteMessage(websocket.BinaryMessage, []byte(s))
}

func waitSessionEnded(t *testing.T, sid int64) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		var n int
		db.QueryRow(`SELECT status FROM terminal_sessions WHERE id=?`, sid).Scan(&status)
		db.QueryRow(`SELECT COUNT(*) FROM session_recordings WHERE session_id=?`, sid).Scan(&n)
		if status != "active" && status != "connecting" && status != "" && (n > 0 || !settingBool("session_recording")) {
			return status
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("session %d did not end", sid)
	return ""
}

type castFile struct {
	header map[string]interface{}
	events [][3]interface{}
}

func (cf castFile) stream(kind string) string {
	var b strings.Builder
	for _, e := range cf.events {
		if e[1] == kind {
			b.WriteString(e[2].(string))
		}
	}
	return b.String()
}

func readCast(t *testing.T, r io.Reader) castFile {
	t.Helper()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var cf castFile
	first := true
	for sc.Scan() {
		if first {
			if err := json.Unmarshal(sc.Bytes(), &cf.header); err != nil {
				t.Fatalf("bad header: %v", err)
			}
			first = false
			continue
		}
		var raw []interface{}
		if err := json.Unmarshal(sc.Bytes(), &raw); err != nil || len(raw) != 3 {
			t.Fatalf("bad event %q: %v", sc.Text(), err)
		}
		cf.events = append(cf.events, [3]interface{}{raw[0], raw[1], raw[2]})
	}
	return cf
}

// ─── tests ───────────────────────────────────────────

// Connect → work → disconnect produces exactly one session, its audit entries and a
// recording that replays the output and contains no password.
func TestTerminalSessionAuditAndRecording(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("WRM_RECORDINGS_DIR", dir)
	defer os.Unsetenv("WRM_RECORDINGS_DIR")
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()

	alice := newTestUser(t, srv, "rec-alice", false)
	connID := alice.addConnection("test-server", sshAddr)

	for _, withInput := range []bool{false, true} {
		setSetting("session_recording_input", map[bool]string{true: "1", false: "0"}[withInput])
		tc := alice.openTerminal(connID)
		tc.waitFor("$ ")
		tc.send("echo hi\r")
		tc.waitFor("you typed: echo hi")
		tc.send("sudo ls\r")
		tc.waitFor("password for tester: ")
		tc.send(testSudoPW + "\r")
		tc.waitFor("password accepted")
		tc.send("exit\r")
		select {
		case <-tc.closed:
		case <-time.After(10 * time.Second):
			t.Fatal("terminal did not close")
		}

		var sid int64
		var n int
		db.QueryRow(`SELECT COUNT(*), MAX(id) FROM terminal_sessions WHERE conn_id=?`, connID).Scan(&n, &sid)
		if want := map[bool]int{false: 1, true: 2}[withInput]; n != want {
			t.Fatalf("terminal_sessions for connection = %d, want %d", n, want)
		}
		if st := waitSessionEnded(t, sid); st != "closed" {
			t.Fatalf("status = %s, want closed", st)
		}
		var exitCode int
		db.QueryRow(`SELECT exit_code FROM terminal_sessions WHERE id=?`, sid).Scan(&exitCode)
		if exitCode != 0 {
			t.Fatalf("exit code %d", exitCode)
		}

		// audit entries linked to the session (pričekaj do 3s da pozadinska goroutina zapiše terminal.close pod -race)
		var actions []string
		deadline := time.Now().Add(3 * time.Second)
		for {
			actions = nil
			rows, _ := db.Query(`SELECT action, details FROM audit_log WHERE session_id=? ORDER BY id`, sid)
			for rows.Next() {
				var a, d string
				rows.Scan(&a, &d)
				actions = append(actions, a)
				if strings.Contains(d, testSudoPW) || strings.Contains(d, testSSHPass) {
					t.Fatalf("password in audit details: %s", d)
				}
			}
			rows.Close()
			if strings.Join(actions, ",") == "terminal.open,terminal.close" || time.Now().After(deadline) {
				break
			}
			time.Sleep(30 * time.Millisecond)
		}
		if strings.Join(actions, ",") != "terminal.open,terminal.close" {
			t.Fatalf("audit actions for session = %v", actions)
		}

		// recording: checksum matches, replays the output, no password
		var rel, sum string
		var size int64
		var input int
		if err := db.QueryRow(`SELECT path, sha256, size_bytes, input_recorded FROM session_recordings WHERE session_id=?`, sid).Scan(&rel, &sum, &size, &input); err != nil {
			t.Fatal(err)
		}
		if input != boolInt(withInput) {
			t.Fatalf("input_recorded = %d", input)
		}
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		raw, err := os.ReadFile(abs)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(raw)
		if hex.EncodeToString(h[:]) != sum || int64(len(raw)) != size {
			t.Fatalf("recording checksum/size mismatch")
		}
		if st, _ := os.Stat(abs); st.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
			t.Fatalf("recording file mode %v is not private", st.Mode().Perm())
		}
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		cf := readCast(t, gz)
		if cf.header["version"] != float64(2) || cf.header["width"] != float64(100) || cf.header["height"] != float64(30) {
			t.Fatalf("header = %v", cf.header)
		}
		out := cf.stream("o")
		for _, want := range []string{"Welcome to the test server — č ć ž š đ ✓", "you typed: echo hi", "password accepted"} {
			if !strings.Contains(out, want) {
				t.Fatalf("recording output lacks %q:\n%s", want, out)
			}
		}
		var last float64
		for _, e := range cf.events {
			ts := e[0].(float64)
			if ts < last {
				t.Fatalf("event times go backwards")
			}
			last = ts
		}
		in := cf.stream("i")
		if strings.Contains(string(raw), testSudoPW) || strings.Contains(out+in, testSudoPW) || strings.Contains(out+in, testSSHPass) {
			t.Fatalf("password found in the recording")
		}
		if withInput {
			if !strings.Contains(in, "echo hi\r") || !strings.Contains(in, strings.Repeat("*", len(testSudoPW))+"\r") {
				t.Fatalf("recorded input = %q", in)
			}
		} else if in != "" {
			t.Fatalf("input recorded while disabled: %q", in)
		}

		// API: the owner sees the session and can fetch the recording
		resp := alice.do("GET", "/api/recordings/"+strconv.FormatInt(sid, 10)+"/cast", nil, "")
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.HasPrefix(string(body), `{"version":2`) {
			t.Fatalf("cast API: %d %.80s", resp.StatusCode, body)
		}
	}
	setSetting("session_recording_input", "0")

	// Another user can neither list nor fetch it.
	var sid int64
	db.QueryRow(`SELECT MAX(id) FROM terminal_sessions WHERE conn_id=?`, connID).Scan(&sid)
	bob := newTestUser(t, srv, "rec-bob", false)
	for _, p := range []string{"/api/recordings/" + strconv.FormatInt(sid, 10), "/api/recordings/" + strconv.FormatInt(sid, 10) + "/cast"} {
		resp := bob.do("GET", p, nil, "")
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("%s for another user: %d", p, resp.StatusCode)
		}
	}
	resp := bob.do("GET", "/api/recordings", nil, "")
	var list []termSessionView
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	for _, s := range list {
		if s.ID == sid {
			t.Fatalf("another user's session listed")
		}
	}
	// ...but an administrator can.
	admin := newTestUser(t, srv, "rec-admin", true)
	resp = admin.do("GET", "/api/recordings/"+strconv.FormatInt(sid, 10), nil, "")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("admin: %d", resp.StatusCode)
	}
	var views int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='recording.view' AND session_id=?`, sid).Scan(&views)
	if views == 0 {
		t.Fatalf("viewing a recording was not audited")
	}

	// Recording off: the session is still audited, nothing is recorded.
	setSetting("session_recording", "0")
	defer setSetting("session_recording", "1")
	tc := alice.openTerminal(connID)
	tc.waitFor("$ ")
	tc.send("exit\r")
	<-tc.closed
	db.QueryRow(`SELECT MAX(id) FROM terminal_sessions WHERE conn_id=?`, connID).Scan(&sid)
	waitSessionEnded(t, sid)
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM session_recordings WHERE session_id=?`, sid).Scan(&n)
	if n != 0 {
		t.Fatalf("recorded although session_recording=0")
	}
}

// Uploads, downloads and server-to-server copies are logged with size and SHA-256.
func TestFileTransfersLogged(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	u := newTestUser(t, srv, "xfer-user", false)
	src := u.addConnection("xfer-src", sshAddr)
	dst := u.addConnection("xfer-dst", sshAddr)
	dirA, dirB := t.TempDir(), t.TempDir()
	content := bytes.Repeat([]byte("WRM transfer test čćž\n"), 5000)
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])

	seen := map[string]int64{}
	lastTransfer := func(direction string) (size int64, sha, status, srcPath, dstPath string) {
		// The row is written when the handler finishes, which can be just after the client
		// has received the last byte.
		for i := 0; i < 200; i++ {
			var id int64
			db.QueryRow(`SELECT id, size_bytes, sha256, status, src_path, dst_path FROM file_transfers WHERE direction=? AND username='xfer-user' ORDER BY id DESC LIMIT 1`,
				direction).Scan(&id, &size, &sha, &status, &srcPath, &dstPath)
			if id > seen[direction] {
				seen[direction] = id
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		return
	}

	// upload
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "data.txt")
	fw.Write(content)
	mw.Close()
	resp := u.do("POST", "/api/remote/upload?id="+strconv.Itoa(src)+"&path="+dirA, &body, mw.FormDataContentType())
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("upload: %d", resp.StatusCode)
	}
	if size, sha, status, _, dstPath := lastTransfer("upload"); size != int64(len(content)) || sha != want || status != "ok" || dstPath != dirA+"/data.txt" {
		t.Fatalf("upload row: %d %s %s %s", size, sha, status, dstPath)
	}

	// download
	resp = u.do("GET", "/api/remote/download?id="+strconv.Itoa(src)+"&path="+dirA+"/data.txt", nil, "")
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(got, content) {
		t.Fatalf("downloaded content differs")
	}
	if size, sha, status, srcPath, _ := lastTransfer("download"); size != int64(len(content)) || sha != want || status != "ok" || srcPath != dirA+"/data.txt" {
		t.Fatalf("download row: %d %s %s %s", size, sha, status, srcPath)
	}

	// server-to-server
	req, _ := json.Marshal(map[string]interface{}{"source_conn_id": src, "source_files": []string{dirA + "/data.txt"}, "dest_conn_id": dst, "dest_path": dirB})
	resp = u.do("POST", "/api/remote/transfer", bytes.NewReader(req), "application/json")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	copied, err := os.ReadFile(filepath.Join(dirB, "data.txt"))
	if err != nil || !bytes.Equal(copied, content) {
		t.Fatalf("server-to-server copy failed: %v", err)
	}
	if size, sha, status, srcPath, dstPath := lastTransfer("s2s"); size != int64(len(content)) || sha != want || status != "ok" ||
		srcPath != dirA+"/data.txt" || dstPath != dirB+"/data.txt" {
		t.Fatalf("s2s row: %d %s %s %s %s", size, sha, status, srcPath, dstPath)
	}

	// audit entries reference the connection
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE conn_id=? AND action IN ('file.upload','file.download','file.transfer')`, src).Scan(&n)
	if n != 3 {
		t.Fatalf("file audit entries for the connection = %d, want 3", n)
	}
}

// The audit trail cannot be changed or deleted by the application, and the hash chain
// detects tampering done directly in the database.
func TestAuditAppendOnlyAndChain(t *testing.T) {
	auditLogAs(nil, 0, "chain-test", "test.event", "a", map[string]string{"password": "should-not-appear", "n": "1"})
	auditLogAs(nil, 0, "chain-test", "test.event", "b", nil)
	var id int
	var det string
	db.QueryRow(`SELECT id, details FROM audit_log WHERE username='chain-test' ORDER BY id LIMIT 1`).Scan(&id, &det)
	if strings.Contains(det, "should-not-appear") || !strings.Contains(det, "[redacted]") {
		t.Fatalf("secret not redacted: %s", det)
	}
	if _, err := db.Exec(`UPDATE audit_log SET details='x' WHERE id=?`, id); err == nil {
		t.Fatal("UPDATE of audit_log succeeded")
	}
	if _, err := db.Exec(`DELETE FROM audit_log WHERE id=?`, id); err == nil {
		t.Fatal("DELETE of a recent audit entry succeeded")
	}
	for _, tbl := range []string{"file_transfers", "session_recordings"} {
		if _, err := db.Exec(`UPDATE ` + tbl + ` SET id=id`); err == nil {
			var n int
			db.QueryRow(`SELECT COUNT(*) FROM ` + tbl).Scan(&n)
			if n > 0 {
				t.Fatalf("UPDATE of %s succeeded", tbl)
			}
		}
	}
	// Old entries can be removed by the retention job.
	old := time.Now().UTC().AddDate(0, 0, -(auditMinRetentionDays + 1)).Format(time.RFC3339)
	db.Exec(`INSERT INTO audit_log (ts, username, action) VALUES (?, 'retention-test', 'test.old')`, old)
	if _, err := db.Exec(`DELETE FROM audit_log WHERE username='retention-test'`); err != nil {
		t.Fatalf("retention delete of an old entry failed: %v", err)
	}

	if res := verifyAuditChain(); !res.OK || res.Checked < 2 {
		t.Fatalf("chain not ok: %+v", res)
	}
	// Tamper directly in the database (bypassing the trigger) → detected, then restore.
	db.Exec(`DROP TRIGGER audit_log_no_update`)
	db.Exec(`UPDATE audit_log SET target='tampered' WHERE id=?`, id)
	res := verifyAuditChain()
	db.Exec(`UPDATE audit_log SET target='a' WHERE id=?`, id)
	createAppendOnlyTriggers()
	if res.OK || res.BrokenAt != id {
		t.Fatalf("tampering not detected: %+v", res)
	}
	if res := verifyAuditChain(); !res.OK {
		t.Fatalf("chain not ok after restore: %+v", res)
	}
}

func TestRecorderUTF8AndLimits(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("WRM_RECORDINGS_DIR", dir)
	defer os.Unsetenv("WRM_RECORDINGS_DIR")
	rec, err := newRecorder("unit-"+randomToken(4), 80, 24, "t", "", true, 4096)
	if err != nil {
		t.Fatal(err)
	}
	text := []byte("čćžšđ ✓ 日本")
	for i := range text { // byte by byte: every multi-byte character is split
		rec.Output(text[i : i+1])
	}
	rec.Output([]byte("\r\nEnter passphrase for key: "))
	rec.Input([]byte("topsecret"))
	rec.Input([]byte("\r"))
	rec.Output([]byte("ok\r\n$ "))
	rec.Input([]byte("ls\r"))
	rec.Resize(120, 40)
	rec.Output(bytes.Repeat([]byte("x"), 8192)) // over the 4 KiB limit
	info, err := rec.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !info.Truncated {
		t.Fatal("size limit not applied")
	}
	f, _ := os.Open(filepath.Join(dir, filepath.FromSlash(info.RelPath)))
	defer f.Close()
	gz, _ := gzip.NewReader(f)
	cf := readCast(t, gz)
	if out := cf.stream("o"); !strings.Contains(out, string(text)) || !strings.Contains(out, "size limit") {
		t.Fatalf("output = %q", out)
	}
	if in := cf.stream("i"); in != "*********\rls\r" {
		t.Fatalf("input = %q", in)
	}
	var resized bool
	for _, e := range cf.events {
		if e[1] == "r" && e[2] == "120x40" {
			resized = true
		}
	}
	if !resized {
		t.Fatal("resize not recorded")
	}
}
