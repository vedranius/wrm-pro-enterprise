// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeHook records the requests of a fake chat / push service.
type fakeHook struct {
	mu     sync.Mutex
	reqs   []fakeReq
	status int
	srv    *httptest.Server
}

type fakeReq struct {
	Path, Query string
	Header      http.Header
	Body        string
}

func newFakeHook(t *testing.T) *fakeHook {
	h := &fakeHook{status: 200}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		h.mu.Lock()
		h.reqs = append(h.reqs, fakeReq{Path: r.URL.Path, Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: string(b)})
		st := h.status
		h.mu.Unlock()
		w.WriteHeader(st)
		if st >= 300 {
			w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found"}`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *fakeHook) all() []fakeReq {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]fakeReq(nil), h.reqs...)
}

func (h *fakeHook) reset() {
	h.mu.Lock()
	h.reqs = nil
	h.mu.Unlock()
}

// fakeSMTP is a minimal SMTP server (EHLO, AUTH PLAIN, MAIL, RCPT, DATA, QUIT).
type fakeSMTP struct {
	mu    sync.Mutex
	auth  []string
	from  []string
	rcpt  []string
	data  []string
	addr  string
	ln    net.Listener
	needs string // "user\x00pass" when AUTH is required
}

func newFakeSMTP(t *testing.T, user, pass string) *fakeSMTP {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{addr: ln.Addr().String(), ln: ln}
	if user != "" {
		s.needs = user + "\x00" + pass
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	rd := bufio.NewReader(c)
	say := func(l string) { io.WriteString(c, l+"\r\n") }
	say("220 fake.example.com ESMTP")
	authed := s.needs == ""
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			say("250-fake.example.com")
			say("250 AUTH PLAIN")
		case strings.HasPrefix(up, "AUTH PLAIN"):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line[len("AUTH PLAIN"):]))
			got := strings.TrimPrefix(string(raw), "\x00")
			s.mu.Lock()
			s.auth = append(s.auth, got)
			s.mu.Unlock()
			if got == s.needs {
				authed = true
				say("235 2.7.0 Authentication successful")
			} else {
				say("535 5.7.8 Authentication credentials invalid")
			}
		case strings.HasPrefix(up, "MAIL FROM:"):
			if !authed {
				say("530 5.7.0 Authentication required")
				continue
			}
			s.mu.Lock()
			s.from = append(s.from, line[10:])
			s.mu.Unlock()
			say("250 OK")
		case strings.HasPrefix(up, "RCPT TO:"):
			s.mu.Lock()
			s.rcpt = append(s.rcpt, line[8:])
			s.mu.Unlock()
			say("250 OK")
		case up == "DATA":
			say("354 go ahead")
			var b strings.Builder
			for {
				l, err := rd.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			s.mu.Lock()
			s.data = append(s.data, b.String())
			s.mu.Unlock()
			say("250 OK queued")
		case up == "QUIT":
			say("221 bye")
			return
		default:
			say("250 OK")
		}
	}
}

func (s *fakeSMTP) messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.data...)
}

func createChannel(t *testing.T, admin *testClient, body map[string]interface{}) notifyChannel {
	t.Helper()
	var ch notifyChannel
	if code := admin.jsonDo("POST", "/api/admin/notify/channels", body, &ch); code != 200 || ch.ID == 0 {
		t.Fatalf("create channel %v: %d %+v", body["kind"], code, ch)
	}
	return ch
}

func TestNotificationChannels(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	admin := newTestUser(t, srv, "notify-admin", true)
	user := newTestUser(t, srv, "notify-user", false)
	hook := newFakeHook(t)
	mail := newFakeSMTP(t, "wrm", "smtp-secret-1")
	host, port, _ := net.SplitHostPort(mail.addr)

	// only administrators manage channels
	if code := user.jsonDo("GET", "/api/admin/notify/channels", nil, nil); code != 403 {
		t.Fatalf("user lists channels: %d", code)
	}
	if code := admin.jsonDo("POST", "/api/admin/notify/channels", map[string]interface{}{"name": "x", "kind": "slack", "config": map[string]string{"webhook_url": "ftp://x"}}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("invalid webhook URL accepted: %d", code)
	}
	if code := admin.jsonDo("POST", "/api/admin/notify/channels", map[string]interface{}{"name": "x", "kind": "telegram", "config": map[string]string{"chat_id": "1"}}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("telegram without token accepted: %d", code)
	}

	chans := map[string]notifyChannel{}
	chans["email"] = createChannel(t, admin, map[string]interface{}{"name": "Mail", "kind": "email", "user_address": true, "config": map[string]string{
		"host": host, "port": port, "security": "none", "username": "wrm", "password": "smtp-secret-1", "from": "WRM <wrm@example.com>", "to": "ops@example.com"}})
	chans["telegram"] = createChannel(t, admin, map[string]interface{}{"name": "Telegram", "kind": "telegram", "config": map[string]string{
		"bot_token": "123:tg-secret-token", "chat_id": "-1001", "api_url": hook.srv.URL}})
	chans["slack"] = createChannel(t, admin, map[string]interface{}{"name": "Mattermost", "kind": "slack", "config": map[string]string{"webhook_url": hook.srv.URL + "/hooks/slack-secret"}})
	chans["teams"] = createChannel(t, admin, map[string]interface{}{"name": "Teams", "kind": "teams", "config": map[string]string{"webhook_url": hook.srv.URL + "/workflows/teams"}})
	chans["discord"] = createChannel(t, admin, map[string]interface{}{"name": "Discord", "kind": "discord", "config": map[string]string{"webhook_url": hook.srv.URL + "/api/webhooks/1/discord"}})
	chans["ntfy"] = createChannel(t, admin, map[string]interface{}{"name": "ntfy", "kind": "ntfy", "user_address": true, "config": map[string]string{"server": hook.srv.URL, "topic": "wrm-alerts", "access_token": "tk_ntfy"}})
	chans["gotify"] = createChannel(t, admin, map[string]interface{}{"name": "Gotify", "kind": "gotify", "config": map[string]string{"server": hook.srv.URL + "/gotify", "app_token": "gotify-app"}})
	chans["pushover"] = createChannel(t, admin, map[string]interface{}{"name": "Pushover", "kind": "pushover", "config": map[string]string{"app_token": "po-app", "user_key": "po-user", "api_url": hook.srv.URL}})
	chans["webhook"] = createChannel(t, admin, map[string]interface{}{"name": "Hook", "kind": "webhook", "config": map[string]string{"url": hook.srv.URL + "/wrm", "hmac_secret": "hmac-key"}})

	// secrets are never returned, but are known to be set
	var list struct {
		Channels []notifyChannel `json:"channels"`
		Kinds    []notifyKind    `json:"kinds"`
	}
	admin.jsonDo("GET", "/api/admin/notify/channels", nil, &list)
	raw, _ := json.Marshal(list)
	for _, secret := range []string{"smtp-secret-1", "tg-secret-token", "slack-secret", "tk_ntfy", "gotify-app", "po-app", "hmac-key"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("channel list contains the secret %q", secret)
		}
	}
	if len(list.Channels) < 9 || len(list.Kinds) != len(notifyKinds) {
		t.Fatalf("channel list: %d channels, %d kinds", len(list.Channels), len(list.Kinds))
	}
	var storedCfg string
	db.QueryRow(`SELECT config FROM notify_channels WHERE id=?`, chans["telegram"].ID).Scan(&storedCfg)
	if strings.Contains(storedCfg, "tg-secret-token") {
		t.Fatalf("bot token stored in plain text: %s", storedCfg)
	}

	// Send test, per channel
	for kind, ch := range chans {
		hook.reset()
		if code := admin.jsonDo("POST", "/api/admin/notify/channels/"+strconv.Itoa(ch.ID)+"/test", map[string]string{}, nil); code != 200 {
			t.Fatalf("test %s: %d", kind, code)
		}
		if kind == "email" {
			continue
		}
		reqs := hook.all()
		if len(reqs) != 1 {
			t.Fatalf("test %s: %d requests", kind, len(reqs))
		}
		rq := reqs[0]
		switch kind {
		case "telegram":
			if rq.Path != "/bot123:tg-secret-token/sendMessage" || !strings.Contains(rq.Body, `"chat_id":"-1001"`) || !strings.Contains(rq.Body, "WRM test message") {
				t.Fatalf("telegram request: %+v", rq)
			}
		case "slack":
			if rq.Path != "/hooks/slack-secret" || !strings.Contains(rq.Body, `"text":"WRM test message`) {
				t.Fatalf("slack request: %+v", rq)
			}
		case "teams":
			if !strings.Contains(rq.Body, "application/vnd.microsoft.card.adaptive") || !strings.Contains(rq.Body, "AdaptiveCard") {
				t.Fatalf("teams request: %+v", rq)
			}
		case "discord":
			if !strings.Contains(rq.Body, `"content":"WRM test message`) {
				t.Fatalf("discord request: %+v", rq)
			}
		case "ntfy":
			if rq.Path != "/wrm-alerts" || rq.Header.Get("Authorization") != "Bearer tk_ntfy" || rq.Header.Get("Title") == "" || !strings.Contains(rq.Body, "test message") {
				t.Fatalf("ntfy request: %+v", rq)
			}
		case "gotify":
			if rq.Path != "/gotify/message" || rq.Header.Get("X-Gotify-Key") != "gotify-app" || !strings.Contains(rq.Body, `"title":"WRM test message"`) {
				t.Fatalf("gotify request: %+v", rq)
			}
		case "pushover":
			v, _ := url.ParseQuery(rq.Body)
			if rq.Path != "/1/messages.json" || v.Get("token") != "po-app" || v.Get("user") != "po-user" || v.Get("title") != "WRM test message" {
				t.Fatalf("pushover request: %+v", rq)
			}
		case "webhook":
			m := hmac.New(sha256.New, []byte("hmac-key"))
			m.Write([]byte(rq.Body))
			if rq.Header.Get("X-WRM-Signature") != "sha256="+hex.EncodeToString(m.Sum(nil)) {
				t.Fatalf("webhook signature %q does not match the body", rq.Header.Get("X-WRM-Signature"))
			}
			var p map[string]interface{}
			if json.Unmarshal([]byte(rq.Body), &p) != nil || p["source"] != "wrm" || p["events"] == nil {
				t.Fatalf("webhook body: %s", rq.Body)
			}
		}
	}
	msgs := mail.messages()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "Subject: WRM test message") || !strings.Contains(msgs[0], "To: ops@example.com") {
		t.Fatalf("test e-mail: %q", msgs)
	}
	if len(mail.auth) == 0 || mail.auth[0] != "wrm\x00smtp-secret-1" {
		t.Fatalf("SMTP login: %q", mail.auth)
	}

	// a failing service: a clear error, recorded on the channel, never with the bot token
	hook.mu.Lock()
	hook.status = 400
	hook.mu.Unlock()
	var e struct {
		Error string `json:"error"`
	}
	if code := admin.jsonDo("POST", "/api/admin/notify/channels/"+strconv.Itoa(chans["telegram"].ID)+"/test", map[string]string{}, &e); code != 502 ||
		!strings.Contains(e.Error, "HTTP 400") || !strings.Contains(e.Error, "chat not found") || strings.Contains(e.Error, "tg-secret-token") {
		t.Fatalf("failing channel: %d %q", code, e.Error)
	}
	hook.mu.Lock()
	hook.status = 200
	hook.mu.Unlock()

	// updating keeps secrets that are not sent again
	var upd notifyChannel
	if code := admin.jsonDo("PUT", "/api/admin/notify/channels/"+strconv.Itoa(chans["telegram"].ID), map[string]interface{}{"name": "Telegram ops", "kind": "telegram",
		"config": map[string]string{"chat_id": "-1002", "api_url": hook.srv.URL}}, &upd); code != 200 || upd.Name != "Telegram ops" {
		t.Fatalf("update channel: %d %+v", code, upd)
	}
	hook.reset()
	admin.jsonDo("POST", "/api/admin/notify/channels/"+strconv.Itoa(chans["telegram"].ID)+"/test", map[string]string{}, nil)
	if r := hook.all(); len(r) != 1 || r[0].Path != "/bot123:tg-secret-token/sendMessage" || !strings.Contains(r[0].Body, "-1002") {
		t.Fatalf("token lost on update: %+v", r)
	}

	// configuration changes are audited (admin.*), without secrets
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action IN ('admin.notify_channel_created','admin.notify_channel_updated','admin.notify_channel_tested')`).Scan(&n)
	if n < 10 {
		t.Fatalf("channel changes not audited: %d", n)
	}
	var leaked int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE details LIKE '%tg-secret-token%' OR details LIKE '%smtp-secret-1%'`).Scan(&leaked)
	if leaked > 0 {
		t.Fatalf("a channel secret is in the audit log")
	}

	// users see the enabled channels without configuration, and subscribe
	var mine struct {
		Channels []map[string]interface{} `json:"channels"`
		Events   []string                 `json:"events"`
	}
	user.jsonDo("GET", "/api/notify", nil, &mine)
	raw, _ = json.Marshal(mine)
	if len(mine.Channels) < 9 || strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "ops@example.com") || len(mine.Events) != len(notifyEvents) {
		t.Fatalf("user view of channels: %s", raw)
	}
	if code := user.jsonDo("PUT", "/api/notify", map[string]interface{}{"subscriptions": []map[string]interface{}{
		{"event": "status.down", "channel_id": chans["slack"].ID, "address": "someone-else"}}}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("own address on a fixed channel accepted: %d", code)
	}
	if code := user.jsonDo("PUT", "/api/notify", map[string]interface{}{"subscriptions": []map[string]interface{}{
		{"event": "status.down", "channel_id": chans["email"].ID, "address": "not an address"}}}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("invalid e-mail accepted: %d", code)
	}
	if code := user.jsonDo("POST", "/api/notify/test", map[string]interface{}{"channel_id": chans["email"].ID, "address": "me@example.com"}, nil); code != 200 {
		t.Fatalf("user test: %d", code)
	}
	if m := mail.messages(); len(m) != 2 || !strings.Contains(m[1], "To: me@example.com") {
		t.Fatalf("user test e-mail: %q", m)
	}
	if code := user.jsonDo("POST", "/api/notify/test", map[string]interface{}{"channel_id": chans["email"].ID, "address": "me@example.com"}, nil); code != 429 {
		t.Fatalf("user test not rate limited: %d", code)
	}

	// deleting a channel removes its subscriptions
	user.jsonDo("PUT", "/api/notify", map[string]interface{}{"subscriptions": []map[string]interface{}{{"event": "status.down", "channel_id": chans["discord"].ID}}}, nil)
	admin.jsonDo("DELETE", "/api/admin/notify/channels/"+strconv.Itoa(chans["discord"].ID), nil, nil)
	db.QueryRow(`SELECT COUNT(*) FROM notify_subscriptions WHERE channel_id=?`, chans["discord"].ID).Scan(&n)
	if n != 0 {
		t.Fatalf("subscriptions of a deleted channel remain")
	}
	for _, ch := range chans {
		db.Exec(`DELETE FROM notify_channels WHERE id=?`, ch.ID)
	}
	db.Exec(`DELETE FROM notify_subscriptions WHERE user_id=?`, user.userID)
}

func TestNotificationDigestRateAndQuietHours(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	admin := newTestUser(t, srv, "digest-admin", true)
	user := newTestUser(t, srv, "digest-user", false)
	hook := newFakeHook(t)
	ch := createChannel(t, admin, map[string]interface{}{"name": "Digest hook", "kind": "slack", "config": map[string]string{"webhook_url": hook.srv.URL + "/d"}})
	defer db.Exec(`DELETE FROM notify_channels WHERE id=?`, ch.ID)
	if code := user.jsonDo("PUT", "/api/notify", map[string]interface{}{
		"subscriptions": []map[string]interface{}{{"event": "status.down", "channel_id": ch.ID}, {"event": "status.up", "channel_id": ch.ID}},
		"prefs":         map[string]interface{}{"quiet_enabled": false, "quiet_start": "22:00", "quiet_end": "07:00", "tz": "Europe/Zagreb", "lang": "hr"}}, nil); code != 200 {
		t.Fatalf("subscribe: %d", code)
	}
	defer db.Exec(`DELETE FROM notify_pending WHERE user_id=?`, user.userID)
	// not subscribed: credential events are not queued
	notifyUser(user.userID, "credential.rotation_due", func(string) (string, string) { return "x", "y" })
	for i := 0; i < 3; i++ {
		notifyStatusChange(user.userID, "web-0"+strconv.Itoa(i+1), "10.0.0.1:22", "down", "connection refused", 0)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM notify_pending WHERE user_id=?`, user.userID).Scan(&n)
	if n != 3 {
		t.Fatalf("pending: %d", n)
	}
	// waits for the debounce, then sends one digest in the user's language
	now := time.Now()
	if sent := notifier.dispatch(now, false); sent != 0 {
		t.Fatalf("sent during the debounce")
	}
	if sent := notifier.dispatch(now.Add(notifyDebounce+time.Second), false); sent != 1 {
		t.Fatalf("digest not sent: %d", sent)
	}
	reqs := hook.all()
	if len(reqs) != 1 || !strings.Contains(reqs[0].Body, "WRM: 3 obavijesti") || !strings.Contains(reqs[0].Body, "web-03 je nedostupan") {
		t.Fatalf("digest: %+v", reqs)
	}
	// rate limit: the next event waits for notify_min_interval_seconds
	notifyStatusChange(user.userID, "web-01", "10.0.0.1:22", "up", "", 90*time.Second)
	if sent := notifier.dispatch(time.Now().Add(notifyDebounce+time.Second), false); sent != 0 {
		t.Fatalf("rate limit ignored")
	}
	if sent := notifier.dispatch(time.Now().Add(61*time.Second), false); sent != 1 {
		t.Fatalf("message after the interval not sent")
	}
	if r := hook.all(); len(r) != 2 || !strings.Contains(r[1].Body, "web-01 je ponovno dostupan") || !strings.Contains(r[1].Body, "nakon 1 min") {
		t.Fatalf("up message: %+v", r)
	}
	// quiet hours hold messages until they end
	user.jsonDo("PUT", "/api/notify", map[string]interface{}{
		"subscriptions": []map[string]interface{}{{"event": "status.down", "channel_id": ch.ID}},
		"prefs":         map[string]interface{}{"quiet_enabled": true, "quiet_start": "22:00", "quiet_end": "07:00", "tz": "UTC", "lang": "en"}}, nil)
	notifyStatusChange(user.userID, "db-01", "10.0.0.2:22", "down", "timeout", 0)
	night := time.Date(2026, 1, 10, 23, 30, 0, 0, time.UTC)
	if sent := notifier.dispatch(night, true); sent != 0 {
		t.Fatalf("sent during quiet hours")
	}
	if sent := notifier.dispatch(time.Date(2026, 1, 11, 7, 5, 0, 0, time.UTC), true); sent != 1 {
		t.Fatalf("held message not sent after quiet hours")
	}
	if r := hook.all(); len(r) != 3 || !strings.Contains(r[2].Body, "db-01 is down") || !strings.Contains(r[2].Body, "timeout") {
		t.Fatalf("message after quiet hours: %+v", r)
	}
	p := notifyPrefs{QuietEnabled: true, QuietStart: "08:00", QuietEnd: "12:00", TZ: "Europe/Zagreb"}
	if !p.inQuietHours(time.Date(2026, 7, 1, 8, 30, 0, 0, time.UTC)) || p.inQuietHours(time.Date(2026, 7, 1, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("quiet hours in a time zone (Zagreb is UTC+2 in July)")
	}
	if code := user.jsonDo("PUT", "/api/notify", map[string]interface{}{"prefs": map[string]interface{}{"quiet_start": "25:00", "quiet_end": "07:00", "tz": "UTC"}}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("invalid quiet hours accepted: %d", code)
	}
	// a failing channel keeps the message and retries later
	hook.mu.Lock()
	hook.status = 500
	hook.mu.Unlock()
	notifyStatusChange(user.userID, "db-02", "10.0.0.3:22", "down", "timeout", 0)
	day := time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC)
	notifier.dispatch(day, true)
	db.QueryRow(`SELECT COUNT(*) FROM notify_pending WHERE user_id=?`, user.userID).Scan(&n)
	var lastErr string
	db.QueryRow(`SELECT last_error FROM notify_channels WHERE id=?`, ch.ID).Scan(&lastErr)
	if n != 1 || !strings.Contains(lastErr, "HTTP 500") {
		t.Fatalf("failed delivery: pending %d, error %q", n, lastErr)
	}
	hook.mu.Lock()
	hook.status = 200
	hook.mu.Unlock()
	if sent := notifier.dispatch(day, true); sent != 1 {
		t.Fatalf("retry not sent")
	}
	// the policy turns notifications off
	setSetting("notifications_enabled", "0")
	notifyStatusChange(user.userID, "db-03", "10.0.0.4:22", "down", "timeout", 0)
	setSetting("notifications_enabled", "1")
	db.QueryRow(`SELECT COUNT(*) FROM notify_pending WHERE user_id=?`, user.userID).Scan(&n)
	if n != 0 {
		t.Fatalf("queued while notifications are off")
	}
}

func TestNotificationEvents(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	admin := newTestUser(t, srv, "ev-admin", true)
	owner := newTestUser(t, srv, "ev-owner", false)
	other := newTestUser(t, srv, "ev-other", false)
	hook := newFakeHook(t)
	ch := createChannel(t, admin, map[string]interface{}{"name": "Events hook", "kind": "webhook", "config": map[string]string{"url": hook.srv.URL + "/ev"}})
	defer db.Exec(`DELETE FROM notify_channels WHERE id=?`, ch.ID)
	all := []map[string]interface{}{}
	for _, e := range notifyEvents {
		all = append(all, map[string]interface{}{"event": e, "channel_id": ch.ID})
	}
	owner.jsonDo("PUT", "/api/notify", map[string]interface{}{"subscriptions": all}, nil)
	other.jsonDo("PUT", "/api/notify", map[string]interface{}{"subscriptions": all}, nil)
	defer db.Exec(`DELETE FROM notify_pending`)

	// a server that goes down and comes back
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	flaky := ln.Addr().String()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	up := owner.addConnection("ev-web", flaky)
	off := owner.addConnection("ev-quiet", flaky)
	db.Exec(`UPDATE connections SET monitor=0 WHERE id=?`, off)
	behind := owner.addConnectionVia("ev-behind", sshAddr, up)
	defer db.Exec(`DELETE FROM connections WHERE id IN (?,?,?)`, up, off, behind)
	setSetting("status_jump_checks", "0")
	statusMon.round()
	statusMon.round()
	db.Exec(`DELETE FROM notify_pending`)
	ln.Close()
	statusMon.round()
	pending := func(uid int) []string {
		rows, _ := db.Query(`SELECT event || ' ' || title FROM notify_pending WHERE user_id=? ORDER BY id`, uid)
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			rows.Scan(&s)
			out = append(out, s)
		}
		return out
	}
	got := pending(owner.userID)
	if len(got) != 2 || !strings.Contains(strings.Join(got, "|"), "status.down 🔴 ev-web is down") || !strings.Contains(strings.Join(got, "|"), "ev-behind is down") {
		t.Fatalf("down events: %q", got)
	}
	if strings.Contains(strings.Join(got, "|"), "ev-quiet") {
		t.Fatalf("a connection with monitoring off was reported")
	}
	if o := pending(other.userID); len(o) != 0 {
		t.Fatalf("another user got events of connections that are not theirs: %q", o)
	}
	statusMon.round() // still down: no new event
	if len(pending(owner.userID)) != 2 {
		t.Fatalf("repeated down event")
	}
	notifier.dispatch(time.Now(), true)
	reqs := hook.all()
	if len(reqs) != 1 || !strings.Contains(reqs[0].Body, `"event":"status.down"`) {
		t.Fatalf("one webhook call for the round: %+v", reqs)
	}
	ln2, err := net.Listen("tcp", flaky)
	if err != nil {
		t.Skipf("cannot listen on %s again: %v", flaky, err)
	}
	go func() {
		for {
			c, err := ln2.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	defer ln2.Close()
	statusMon.round()
	// both come back: the server, and the connection behind it (its jump host is up again)
	if got := strings.Join(pending(owner.userID), "|"); strings.Count(got, "status.up") != 2 || !strings.Contains(got, "status.up 🟢 ev-web is up again") ||
		!strings.Contains(got, "status.up 🟢 ev-behind is up again") {
		t.Fatalf("up events: %q", got)
	}
	db.Exec(`DELETE FROM notify_pending`)

	// credential rotation due and incomplete, owner only, not repeated every hour
	now := time.Now().UTC()
	old := now.Add(-40 * 24 * time.Hour).Format(time.RFC3339)
	res, _ := db.Exec(`INSERT INTO credentials (owner_id,name,username,password,rotate_days,rotated_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		owner.userID, "ev-root", "root", encryptValue("x"), 30, old, old, old)
	due, _ := res.LastInsertId()
	res, _ = db.Exec(`INSERT INTO credentials (owner_id,name,username,password,pending_password,rotation_status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		owner.userID, "ev-app", "app", encryptValue("x"), encryptValue("y"), "incomplete: srv-2 has the NEW password", old, old)
	inc, _ := res.LastInsertId()
	defer db.Exec(`DELETE FROM credentials WHERE id IN (?,?)`, due, inc)
	checkCredentialReminders()
	got = pending(owner.userID)
	if len(got) != 2 || !strings.Contains(strings.Join(got, "|"), "credential.rotation_due ⏰ Rotation due: ev-root") ||
		!strings.Contains(strings.Join(got, "|"), "credential.rotation_incomplete ⚠ Rotation incomplete: ev-app") {
		t.Fatalf("credential events: %q", got)
	}
	checkCredentialReminders()
	if len(pending(owner.userID)) != 2 || len(pending(other.userID)) != 0 {
		t.Fatalf("credential reminders repeated or sent to another user")
	}
	// rotated in the meantime: the reminder state is cleared
	db.Exec(`UPDATE credentials SET rotated_at=? WHERE id=?`, now.Format(time.RFC3339), due)
	checkCredentialReminders()
	if v := getNotifyState("cred_due:" + strconv.FormatInt(due, 10)); v != "" {
		t.Fatalf("reminder state kept after rotation: %q", v)
	}
}
