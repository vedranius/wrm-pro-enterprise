// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// endLockdown turns the lockdown off and refreshes the cached state.
func endLockdown(t *testing.T) {
	t.Helper()
	setLockdown(false, "test", "test", "test")
	lockdownCache.Lock()
	lockdownCache.loaded = false
	lockdownCache.Unlock()
}

func makeBreakGlass(t *testing.T, userID int) {
	t.Helper()
	db.Exec(`UPDATE users SET break_glass=1, is_admin=1, role='' WHERE id=?`, userID)
}

func TestLockdown(t *testing.T) {
	keepRoleSettings(t)
	t.Cleanup(func() { endLockdown(t) })
	resetLoginLimiter()
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "lock-admin", true)
	bg := newTestUser(t, srv, "lock-breakglass", true)
	user := newTestUser(t, srv, "lock-user", false)
	// Other tests in the shared database may have left break-glass accounts behind.
	var others []int
	if rows, err := db.Query(`SELECT id FROM users WHERE break_glass=1`); err == nil {
		for rows.Next() {
			var id int
			if rows.Scan(&id) == nil {
				others = append(others, id)
			}
		}
		rows.Close()
	}
	db.Exec(`UPDATE users SET break_glass=0 WHERE break_glass=1`)
	t.Cleanup(func() {
		for _, id := range others {
			db.Exec(`UPDATE users SET break_glass=1 WHERE id=?`, id)
		}
	})

	if code := admin.jsonDo("POST", "/api/admin/lockdown", map[string]interface{}{"on": true, "reason": "incident"}, nil); code != 409 {
		t.Fatalf("lockdown without any break-glass administrator: %d", code)
	}
	makeBreakGlass(t, bg.userID)
	if code := admin.jsonDo("POST", "/api/admin/lockdown", map[string]interface{}{"on": true}, nil); code != 400 {
		t.Fatalf("lockdown without a reason: %d", code)
	}
	var res map[string]interface{}
	if code := admin.jsonDo("POST", "/api/admin/lockdown", map[string]interface{}{"on": true, "reason": "incident 42"}, &res); code != 200 {
		t.Fatalf("lockdown on: %d %v", code, res)
	}
	if res["you_signed_out"] != true || res["on"] != true {
		t.Fatalf("lockdown answer: %v", res)
	}
	if code := user.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatalf("a user kept the session during lockdown: %d", code)
	}
	if code := admin.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatalf("a non-break-glass administrator kept the session: %d", code)
	}
	if code := bg.jsonDo("GET", "/api/auth/me", nil, nil); code != 200 {
		t.Fatalf("the break-glass administrator lost the session: %d", code)
	}
	code, out, _ := passwordLogin(t, srv, "lock-user", "pw-lock-user-123456")
	if code != 403 || !strings.Contains(fmt.Sprint(out["error"]), "lockdown") {
		t.Fatalf("sign-in during lockdown: %d %v", code, out)
	}
	var cfg map[string]interface{}
	if resp, err := http.Get(srv.URL + "/api/auth/config"); err == nil {
		json.NewDecoder(resp.Body).Decode(&cfg)
		resp.Body.Close()
	}
	if ld, _ := cfg["lockdown"].(map[string]interface{}); ld == nil || ld["reason"] != "incident 42" {
		t.Fatalf("the sign-in page does not show the lockdown: %v", cfg["lockdown"])
	}
	if ok, _ := aiUserAllowed(bg.userID); ok {
		t.Fatal("AI is allowed during lockdown")
	}
	if ok, _ := mcpUserAllowed(bg.userID); ok {
		t.Fatal("AI connections are allowed during lockdown")
	}
	if _, den := resolveShareAccessCtx(ShareLink{ID: 12345, Active: true}, accessCtx{}); den == nil || den.Reason != "lockdown" {
		t.Fatalf("a share works during lockdown: %+v", den)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='security.lockdown_on'`).Scan(&n)
	if n == 0 {
		t.Fatal("lockdown not audited")
	}
	// Only a break-glass administrator ends it.
	if code := bg.jsonDo("POST", "/api/admin/lockdown", map[string]interface{}{"on": false, "reason": "resolved"}, nil); code != 200 {
		t.Fatalf("lockdown off: %d", code)
	}
	if lockdownActive() {
		t.Fatal("lockdown still on")
	}
	resetLoginLimiter()
	if code, out, _ := passwordLogin(t, srv, "lock-user", "pw-lock-user-123456"); code != 200 {
		t.Fatalf("sign-in after the lockdown: %d %v", code, out)
	}

	// From the command line; a running server notices through the database.
	if err := lockdownCLI("on", "from the console"); err != nil {
		t.Fatal(err)
	}
	if !readLockdownState().On {
		t.Fatal("CLI did not store the lockdown")
	}
	lockdownCache.Lock()
	lockdownCache.loaded = false
	lockdownCache.Unlock()
	if !lockdownActive() {
		t.Fatal("CLI lockdown not active")
	}
	if err := lockdownCLI("off", ""); err != nil {
		t.Fatal(err)
	}
	if err := lockdownCLI("maybe", ""); err == nil {
		t.Fatal("a bad -lockdown value was accepted")
	}
	endLockdown(t)

	// The environment variable forces it and cannot be ended from the UI.
	t.Setenv(lockdownEnv, "1")
	if !lockdownActive() {
		t.Fatal("WRM_LOCKDOWN did not force the lockdown")
	}
	if code := bg.jsonDo("POST", "/api/admin/lockdown", map[string]interface{}{"on": false}, nil); code != 409 {
		t.Fatalf("ending an environment lockdown from the UI: %d", code)
	}
	os.Unsetenv(lockdownEnv)
}

func TestStopUser(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "stop-admin", true)
	victim := newTestUser(t, srv, "stop-user", false)
	if code := admin.jsonDo("POST", fmt.Sprintf("/api/admin/users/%d/stop", admin.userID), nil, nil); code != 400 {
		t.Fatalf("stopping yourself: %d", code)
	}
	var res struct {
		Ended map[string]interface{} `json:"ended"`
	}
	if code := admin.jsonDo("POST", fmt.Sprintf("/api/admin/users/%d/stop", victim.userID), map[string]string{"reason": "stolen laptop"}, &res); code != 200 {
		t.Fatalf("stop user: %d", code)
	}
	if res.Ended["sessions_ended"] != float64(1) || res.Ended["disabled"] != true {
		t.Fatalf("stop answer: %v", res.Ended)
	}
	for _, k := range []string{"terminals_ended", "rooms_left", "tunnels_stopped", "ai_sessions_ended", "ai_connections_revoked"} {
		if _, ok := res.Ended[k]; !ok {
			t.Errorf("stop answer misses %s", k)
		}
	}
	if code := victim.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatalf("the stopped user kept the session: %d", code)
	}
	var det string
	db.QueryRow(`SELECT details FROM audit_log WHERE action='admin.user_stopped' ORDER BY id DESC LIMIT 1`).Scan(&det)
	if !strings.Contains(det, "stolen laptop") {
		t.Fatalf("stop not audited with the reason: %q", det)
	}
}

func waitAlert(t *testing.T, rule string, after int64) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var det string
		if db.QueryRow(`SELECT details FROM audit_log WHERE action='security.alert' AND target=? AND id > ? ORDER BY id DESC LIMIT 1`, rule, after).Scan(&det) == nil {
			var d map[string]interface{}
			json.Unmarshal([]byte(det), &d)
			return d
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no %s alert", rule)
	return nil
}

func lastAuditID() int64 {
	var id int64
	db.QueryRow(`SELECT COALESCE(MAX(id),0) FROM audit_log`).Scan(&id)
	return id
}

func TestSecurityAlerts(t *testing.T) {
	saved := map[string]string{}
	for _, k := range []string{"alert_failed_signins", "alert_failed_signins_minutes", "audit_enabled"} {
		saved[k] = getSetting(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			setSetting(k, v)
		}
	})
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "alert-admin", true)
	setSetting("alert_failed_signins", "3")
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	req.RemoteAddr = "198.51.100.77:5000"

	start := lastAuditID()
	for i := 0; i < 3; i++ {
		auditLogAs(req, 0, fmt.Sprintf("guess%d", i), "auth.login_failed", "", map[string]string{"method": "password"})
	}
	d := waitAlert(t, "failed_signins", start)
	if !strings.Contains(fmt.Sprint(d["summary"]), "198.51.100.77") {
		t.Fatalf("failed sign-in alert: %v", d)
	}
	var n int
	for i := 0; i < 50 && n == 0; i++ { // the inbox entry follows the audit entry
		db.QueryRow(`SELECT COUNT(*) FROM user_events WHERE user_id=? AND event='security.alert'`, admin.userID).Scan(&n)
		if n == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if n == 0 {
		t.Fatal("administrators were not told about the alert")
	}

	cases := []struct {
		rule, action, target string
		det                  interface{}
	}{
		{"new_admin", "admin.user_created", "new-boss", map[string]interface{}{"admin": true, "role": "admin"}},
		{"new_admin", "oidc.role_synced", "promoted", map[string]string{"from": "user", "to": "admin"}},
		{"secret_reveal", "credential.revealed", "db-root", map[string]interface{}{"cred_id": 3}},
		{"secret_reveal", "config.exported", "", map[string]interface{}{"connections": 4, "with_secrets": true}},
		{"ai_full_access", "ai.approved", "", map[string]interface{}{"tool": "run_command", "rule": "full_access", "outcome": "approved"}},
		{"audit_off", "siem.destination_deleted", "splunk", map[string]interface{}{"siem_id": 1}},
		{"break_glass", "auth.break_glass_login", "emergency", map[string]string{"ip": "10.0.0.1"}},
	}
	for _, c := range cases {
		start := lastAuditID()
		auditLogAs(req, admin.userID, "alert-admin", c.action, c.target, c.det)
		d := waitAlert(t, c.rule, start)
		if d["source_action"] != c.action {
			t.Errorf("%s: %v", c.rule, d)
		}
	}
	// Turning the audit off is the last thing that is still audited (admin.* always is).
	start = lastAuditID()
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]interface{}{"audit_enabled": false}, nil); code != 200 {
		t.Fatalf("audit off: %d", code)
	}
	setSetting("audit_enabled", "1")
	waitAlert(t, "audit_off", start)
	// No alert for ordinary events, and a rule that is off stays quiet.
	setSetting("alert_secret_reveal", "0")
	defer setSetting("alert_secret_reveal", "1")
	start = lastAuditID()
	auditLogAs(req, admin.userID, "alert-admin", "credential.revealed", "x", map[string]interface{}{"cred_id": 9})
	auditLogAs(req, admin.userID, "alert-admin", "connection.created", "x", nil)
	time.Sleep(300 * time.Millisecond)
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='security.alert' AND id > ?`, start).Scan(&n)
	if n != 0 {
		t.Fatalf("%d unexpected alerts", n)
	}
	if len(recentAlerts(5)) == 0 {
		t.Fatal("recentAlerts is empty")
	}
}

// ─── SIEM ────────────────────────────────────────────

type siemSink struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   [][]byte
	status   int
	reply    string
}

func newSIEMSink(t *testing.T) (*siemSink, *httptest.Server, string) {
	t.Helper()
	s := &siemSink{status: 200}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.requests = append(s.requests, r)
		s.bodies = append(s.bodies, b)
		st, rep := s.status, s.reply
		s.mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token") {
			json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "entra-token-1", "expires_in": 3600})
			return
		}
		w.WriteHeader(st)
		io.WriteString(w, rep)
	}))
	t.Cleanup(srv.Close)
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
	return s, srv, ca
}

func (s *siemSink) take() ([]*http.Request, [][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, b := s.requests, s.bodies
	s.requests, s.bodies = nil, nil
	return r, b
}

func newSIEM(t *testing.T, admin *testClient, body map[string]interface{}) int {
	t.Helper()
	var res struct {
		ID    int    `json:"id"`
		Error string `json:"error"`
	}
	if code := admin.jsonDo("POST", "/api/admin/siem", body, &res); code != 200 || res.ID == 0 {
		t.Fatalf("create SIEM destination: %d %s", code, res.Error)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM siem_destinations WHERE id=?`, res.ID) })
	return res.ID
}

func siemFlush() {
	siemBackoff.Lock()
	siemBackoff.next = map[int]time.Time{}
	siemBackoff.Unlock()
	siemRunOnce()
}

func TestSIEMExportHTTPKinds(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "siem-admin", true)
	sink, fake, ca := newSIEMSink(t)

	// validation
	for _, bad := range []map[string]interface{}{
		{"name": "x", "kind": "http", "url": "http://siem.example.com/in"},
		{"name": "x", "kind": "nope", "url": fake.URL},
		{"name": "x", "kind": "splunk", "url": fake.URL},
		{"name": "x", "kind": "jsonl", "url": "relative/audit.jsonl"},
		{"name": "x", "kind": "syslog", "url": "siem.example.com"},
		{"name": "x", "kind": "sentinel", "url": fake.URL, "secret": "s"},
	} {
		if code := admin.jsonDo("POST", "/api/admin/siem", bad, nil); code != 400 {
			t.Errorf("accepted %v: %d", bad, code)
		}
	}

	id := newSIEM(t, admin, map[string]interface{}{"name": "Generic", "kind": "http", "url": fake.URL + "/ingest", "secret": "hook-secret",
		"config": map[string]interface{}{"ca_pem": ca}})
	auditLogAs(nil, admin.userID, "siem-admin", "connection.created", "web-1", map[string]interface{}{"conn": 7})
	siemFlush()
	reqs, bodies := sink.take()
	if len(reqs) != 1 {
		t.Fatalf("generic: %d requests", len(reqs))
	}
	mac := hmac.New(sha256.New, []byte("hook-secret"))
	mac.Write(bodies[0])
	if reqs[0].Header.Get("X-WRM-Signature") != "sha256="+hex.EncodeToString(mac.Sum(nil)) || reqs[0].Header.Get("Authorization") != "Bearer hook-secret" {
		t.Fatal("generic: signature or token missing")
	}
	var evs []map[string]interface{}
	if err := json.Unmarshal(bodies[0], &evs); err != nil || len(evs) == 0 {
		t.Fatalf("generic body: %v %s", err, bodies[0])
	}
	last := evs[len(evs)-1]
	ev := last["event"].(map[string]interface{})
	user := last["user"].(map[string]interface{})
	hash := last["hash"].(map[string]interface{})
	if ev["action"] != "connection.created" || user["name"] != "siem-admin" || hash["this"] == "" || last["target"] != "web-1" {
		t.Fatalf("generic event: %v", last)
	}
	if roles, _ := user["roles"].([]interface{}); len(roles) != 1 || roles[0] != "admin" {
		t.Fatalf("identity fields: %v", user)
	}
	// The secret never goes to a changed address, and is never returned.
	if code := admin.jsonDo("PUT", fmt.Sprintf("/api/admin/siem/%d", id), map[string]interface{}{"url": "https://other.example.com/ingest"}, nil); code != 400 {
		t.Fatalf("address change without the secret: %d", code)
	}
	var list struct {
		Destinations []map[string]interface{} `json:"destinations"`
	}
	admin.jsonDo("GET", "/api/admin/siem", nil, &list)
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), "hook-secret") || len(list.Destinations) == 0 || list.Destinations[0]["state"] != "ok" {
		t.Fatalf("destination list: %s", raw)
	}

	// A failing SIEM keeps the entries and shows the error; they arrive once it works again.
	sink.mu.Lock()
	sink.status, sink.reply = 503, "busy"
	sink.mu.Unlock()
	auditLogAs(nil, admin.userID, "siem-admin", "connection.deleted", "web-1", nil)
	siemFlush()
	sink.take()
	d, _ := loadSIEM(id)
	if !strings.Contains(d.LastError, "503") {
		t.Fatalf("error not recorded: %q", d.LastError)
	}
	if siemJSON(d)["state"] != "failing" || siemJSON(d)["waiting"].(int64) == 0 {
		t.Fatalf("failing state: %v", siemJSON(d))
	}
	sink.mu.Lock()
	sink.status, sink.reply = 200, ""
	sink.mu.Unlock()
	siemFlush()
	_, bodies = sink.take()
	if len(bodies) == 0 || !strings.Contains(string(bodies[0]), "connection.deleted") {
		t.Fatal("the entry was lost after the failure")
	}
	db.Exec(`UPDATE siem_destinations SET enabled=0 WHERE id=?`, id)

	// Splunk HEC
	sid := newSIEM(t, admin, map[string]interface{}{"name": "Splunk", "kind": "splunk", "url": fake.URL, "secret": "hec-token",
		"config": map[string]interface{}{"ca_pem": ca, "index": "security"}})
	auditLogAs(nil, admin.userID, "siem-admin", "auth.login", "", nil)
	siemFlush()
	reqs, bodies = sink.take()
	if len(reqs) != 1 || reqs[0].URL.Path != "/services/collector/event" || reqs[0].Header.Get("Authorization") != "Splunk hec-token" {
		t.Fatalf("splunk request: %v", reqs)
	}
	var item map[string]interface{}
	json.Unmarshal([]byte(strings.SplitN(string(bodies[0]), "\n", 2)[0]), &item)
	if item["index"] != "security" || item["sourcetype"] != "wrm:audit" || item["event"] == nil {
		t.Fatalf("splunk item: %v", item)
	}
	db.Exec(`UPDATE siem_destinations SET enabled=0 WHERE id=?`, sid)

	// Elasticsearch bulk; a refused document is a failure
	eid := newSIEM(t, admin, map[string]interface{}{"name": "Elastic", "kind": "elastic", "url": fake.URL, "secret": "es-key",
		"config": map[string]interface{}{"ca_pem": ca}})
	sink.mu.Lock()
	sink.reply = `{"errors":true}`
	sink.mu.Unlock()
	auditLogAs(nil, admin.userID, "siem-admin", "auth.logout", "", nil)
	siemFlush()
	reqs, bodies = sink.take()
	if len(reqs) != 1 || reqs[0].URL.Path != "/_bulk" || reqs[0].Header.Get("Authorization") != "ApiKey es-key" || !strings.Contains(string(bodies[0]), `"_index":"wrm-audit"`) {
		t.Fatalf("elastic request: %v", reqs)
	}
	if d, _ := loadSIEM(eid); !strings.Contains(d.LastError, "refused") {
		t.Fatalf("elastic errors not detected: %q", d.LastError)
	}
	sink.mu.Lock()
	sink.reply = `{"errors":false}`
	sink.mu.Unlock()
	siemFlush()
	sink.take()
	if d, _ := loadSIEM(eid); d.LastError != "" {
		t.Fatalf("elastic did not recover: %q", d.LastError)
	}
	db.Exec(`UPDATE siem_destinations SET enabled=0 WHERE id=?`, eid)

	// Microsoft Sentinel (Logs Ingestion API with an Entra token)
	saved := siemEntraLogin
	siemEntraLogin = fake.URL
	defer func() { siemEntraLogin = saved }()
	newSIEM(t, admin, map[string]interface{}{"name": "Sentinel", "kind": "sentinel", "url": fake.URL, "secret": "client-secret",
		"config": map[string]interface{}{"ca_pem": ca, "tenant_id": "tenant-1", "client_id": "app-1", "dcr_id": "dcr-abc"}})
	auditLogAs(nil, admin.userID, "siem-admin", "auth.login", "", nil)
	siemFlush()
	reqs, bodies = sink.take()
	if len(reqs) != 2 || !strings.HasSuffix(reqs[0].URL.Path, "/tenant-1/oauth2/v2.0/token") {
		t.Fatalf("sentinel requests: %d", len(reqs))
	}
	if reqs[1].URL.Path != "/dataCollectionRules/dcr-abc/streams/Custom-WRMAudit_CL" || reqs[1].Header.Get("Authorization") != "Bearer entra-token-1" {
		t.Fatalf("sentinel ingestion: %s %s", reqs[1].URL.Path, reqs[1].Header.Get("Authorization"))
	}
	var rows []map[string]interface{}
	json.Unmarshal(bodies[1], &rows)
	if len(rows) == 0 || rows[0]["TimeGenerated"] == nil || rows[0]["Hash"] == "" {
		t.Fatalf("sentinel rows: %s", bodies[1])
	}
}

func selfSignedTLS(t *testing.T, host string) (tls.Certificate, string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	cert, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}))
	if err != nil {
		t.Fatal(err)
	}
	return cert, string(certPEM)
}

func TestSIEMExportSyslogAndFile(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "siem-admin2", true)
	db.Exec(`UPDATE siem_destinations SET enabled=0`)

	cert, ca := selfSignedTLS(t, "127.0.0.1")
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 10)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				for {
					// RFC 5425: "<length> <message>"
					lenStr, err := br.ReadString(' ')
					if err != nil {
						return
					}
					n, _ := strconv.Atoi(strings.TrimSpace(lenStr))
					buf := make([]byte, n)
					if _, err := io.ReadFull(br, buf); err != nil {
						return
					}
					got <- string(buf)
				}
			}(c)
		}
	}()
	sid := newSIEM(t, admin, map[string]interface{}{"name": "Syslog", "kind": "syslog", "url": ln.Addr().String(),
		"config": map[string]interface{}{"ca_pem": ca, "actions": []string{"auth."}}})
	auditLogAs(nil, admin.userID, "siem-admin2", "connection.created", "skipped", nil)
	auditLogAs(nil, admin.userID, "siem-admin2", "auth.login_failed", "", map[string]string{"reason": "bad \"pw\"]"})
	siemFlush()
	select {
	case m := <-got:
		if !strings.HasPrefix(m, "<84>1 ") || !strings.Contains(m, " auth.login_failed [wrm@32473 ") || !strings.Contains(m, `user="siem-admin2"`) {
			t.Fatalf("syslog message: %s", m)
		}
		if strings.Contains(m, "connection.created") {
			t.Fatal("the action filter did not apply")
		}
	case <-time.After(5 * time.Second):
		d, _ := loadSIEM(sid)
		t.Fatalf("no syslog message (%s)", d.LastError)
	}
	// Without the CA the certificate is refused.
	db.Exec(`UPDATE siem_destinations SET config='{}' WHERE id=?`, sid)
	auditLogAs(nil, admin.userID, "siem-admin2", "auth.login", "", nil)
	siemFlush()
	if d, _ := loadSIEM(sid); !strings.Contains(d.LastError, "certificate") {
		t.Fatalf("untrusted syslog certificate accepted: %q", d.LastError)
	}
	db.Exec(`UPDATE siem_destinations SET enabled=0 WHERE id=?`, sid)

	// JSON lines file with rotation
	dir := t.TempDir()
	p := filepath.Join(dir, "audit.jsonl")
	saved := siemFileMax
	siemFileMax = 200
	defer func() { siemFileMax = saved }()
	newSIEM(t, admin, map[string]interface{}{"name": "File", "kind": "jsonl", "url": p, "start_from": "now"})
	for i := 0; i < 3; i++ {
		auditLogAs(nil, admin.userID, "siem-admin2", "file.uploaded", fmt.Sprintf("f%d", i), nil)
		siemFlush()
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p + ".1"); err != nil {
		t.Fatal("the file was not rotated")
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("file mode %v", fi.Mode())
	}
	var ev map[string]interface{}
	if err := json.Unmarshal([]byte(strings.SplitN(string(b), "\n", 2)[0]), &ev); err != nil || ev["@timestamp"] == nil {
		t.Fatalf("jsonl line: %v %s", err, b)
	}
	// A symbolic link is refused.
	link := filepath.Join(dir, "link.jsonl")
	os.Symlink(p, link)
	if code := admin.jsonDo("POST", "/api/admin/siem", map[string]interface{}{"name": "Link", "kind": "jsonl", "url": link}, nil); code != 400 {
		t.Fatalf("symlink accepted: %d", code)
	}
	// The test button reports the result.
	var tr map[string]interface{}
	fid := 0
	db.QueryRow(`SELECT id FROM siem_destinations WHERE name='File'`).Scan(&fid)
	admin.jsonDo("POST", fmt.Sprintf("/api/admin/siem/%d/test", fid), nil, &tr)
	if tr["ok"] != true {
		t.Fatalf("test event: %v", tr)
	}
}

// A destination that is down keeps its place and catches up in order.
func TestSIEMCatchUpAfterOutage(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "siem-outage", true)
	sink, fake, ca := newSIEMSink(t)
	id := newSIEM(t, admin, map[string]interface{}{"name": "Down", "kind": "http", "url": fake.URL + "/in",
		"config": map[string]interface{}{"ca_pem": ca, "actions": []string{"outage."}}})
	sink.mu.Lock()
	sink.status = 503
	sink.mu.Unlock()
	for i := 0; i < 3; i++ {
		auditLogAs(nil, admin.userID, "siem-outage", "outage.event", fmt.Sprintf("e%d", i), nil)
	}
	siemFlush()
	d, _ := loadSIEM(id)
	if d.LastError == "" || d.Sent != 0 {
		t.Fatalf("a refused delivery was not recorded: %+v", d)
	}
	if st := siemJSON(d); st["state"] != "failing" || st["retry_at"] == nil {
		t.Fatalf("status during the outage: %v", st)
	}
	sink.take()
	sink.mu.Lock()
	sink.status = 200
	sink.mu.Unlock()
	auditLogAs(nil, admin.userID, "siem-outage", "outage.event", "e3", nil)
	siemFlush()
	_, bodies := sink.take()
	var got []string
	for _, b := range bodies {
		var evs []map[string]interface{}
		json.Unmarshal(b, &evs)
		for _, ev := range evs {
			got = append(got, fmt.Sprint(ev["target"]))
		}
	}
	if strings.Join(got, ",") != "e0,e1,e2,e3" {
		t.Fatalf("after the outage WRM sent %v, want e0..e3 in order", got)
	}
	if d, _ = loadSIEM(id); d.LastError != "" || d.Sent != 4 || siemJSON(d)["state"] != "ok" {
		t.Fatalf("status after catching up: %+v", d)
	}
}
