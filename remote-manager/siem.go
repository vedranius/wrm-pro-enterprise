// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

// Audit export to a SIEM (v12.6.26).
//
// Every audit entry can be sent, as it is written, to one or more destinations:
//
//   syslog    RFC 5424 messages over TLS (RFC 5425 octet-counted framing), optional client certificate
//   jsonl     JSON lines appended to a file on the WRM host (rotated at 100 MB, 3 old files kept)
//   splunk    Splunk HTTP Event Collector
//   elastic   Elasticsearch / OpenSearch _bulk API
//   sentinel  Microsoft Sentinel through the Azure Monitor Logs Ingestion API (Entra app)
//   http      any HTTPS endpoint: a JSON array per batch, optional bearer token and HMAC signature
//
// Delivery is at least once and in order: each destination keeps a cursor (the last audit id it
// delivered) in the database, so nothing is lost across a restart or while the SIEM is down;
// failures are retried with a growing delay and shown to the administrator with the number of
// entries waiting. Each event carries the identity fields (user, role, directory identity), the
// source address and the audit hash chain (prev_hash, hash), so the SIEM can check that nothing
// was removed.
//
// Secrets (tokens, API keys, client secrets, client keys) are encrypted at rest, never returned
// to the browser and never sent to a changed address: changing a destination's address requires
// the secret again.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// siemFileMax: a JSON lines file is rotated beyond this size; siemEntraLogin is the Entra token
// endpoint for Sentinel (variables so tests can use a fake).
var (
	siemFileMax    int64 = 100 << 20
	siemEntraLogin       = "https://login.microsoftonline.com"
)

var siemKinds = []string{"syslog", "jsonl", "splunk", "elastic", "sentinel", "http"}

const (
	siemBatch       = 200
	siemFileKeep    = 3
	siemHTTPTimeout = 15 * time.Second
	siemMaxDest     = 20
)

type siemConfig struct {
	Index      string   `json:"index,omitempty"`      // elastic index, splunk index
	SourceType string   `json:"sourcetype,omitempty"` // splunk
	TenantID   string   `json:"tenant_id,omitempty"`  // sentinel
	ClientID   string   `json:"client_id,omitempty"`  // sentinel
	DCRID      string   `json:"dcr_id,omitempty"`     // sentinel: data collection rule immutable ID
	Stream     string   `json:"stream,omitempty"`     // sentinel: Custom-WRMAudit_CL
	Facility   int      `json:"facility,omitempty"`   // syslog (default 10, authpriv)
	CAPEM      string   `json:"ca_pem,omitempty"`     // syslog / https: extra trusted CA
	ClientCert string   `json:"client_cert,omitempty"`
	Actions    []string `json:"actions,omitempty"` // action prefixes to send; empty = everything
}

type siemDest struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	URL         string     `json:"url"` // syslog: host:port; jsonl: file path; others: https URL
	Config      siemConfig `json:"config"`
	secret      string
	HasSecret   bool   `json:"has_secret"`
	Enabled     bool   `json:"enabled"`
	Cursor      int64  `json:"cursor"`
	Sent        int64  `json:"sent"`
	Failed      int64  `json:"failed"`
	LastOKAt    string `json:"last_ok_at"`
	LastError   string `json:"last_error"`
	LastErrorAt string `json:"last_error_at"`
	CreatedAt   string `json:"created_at"`
}

const siemCols = `id, name, kind, url, config, secret, enabled, cursor, sent, failed, last_ok_at, last_error, last_error_at, created_at`

func scanSIEM(sc interface{ Scan(...interface{}) error }) (*siemDest, error) {
	var d siemDest
	var cfg, sec string
	var en int
	if err := sc.Scan(&d.ID, &d.Name, &d.Kind, &d.URL, &cfg, &sec, &en, &d.Cursor, &d.Sent, &d.Failed, &d.LastOKAt, &d.LastError, &d.LastErrorAt, &d.CreatedAt); err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(cfg), &d.Config)
	d.secret = decryptValue(sec)
	d.HasSecret, d.Enabled = d.secret != "", en == 1
	return &d, nil
}

func loadSIEM(id int) (*siemDest, error) {
	return scanSIEM(db.QueryRow(`SELECT `+siemCols+` FROM siem_destinations WHERE id=?`, id))
}

func siemDestinations(enabledOnly bool) []*siemDest {
	q := `SELECT ` + siemCols + ` FROM siem_destinations`
	if enabledOnly {
		q += ` WHERE enabled=1`
	}
	rows, err := db.Query(q + ` ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*siemDest
	for rows.Next() {
		if d, err := scanSIEM(rows); err == nil {
			out = append(out, d)
		}
	}
	return out
}

// ─── events ──────────────────────────────────────────

type siemRow struct {
	ID                                                    int64
	TS, Username, IP, Action, Target, Details, Prev, Hash string
	UserID, ConnID, SessionID                             sql.NullInt64
}

var siemIdentityCache = struct {
	sync.Mutex
	m map[int64]siemIdentity
}{m: map[int64]siemIdentity{}}

type siemIdentity struct {
	Role, Email string
	Directory   map[string]string
	at          time.Time
}

func siemIdentityOf(userID int64) siemIdentity {
	siemIdentityCache.Lock()
	id, ok := siemIdentityCache.m[userID]
	siemIdentityCache.Unlock()
	if ok && time.Since(id.at) < time.Minute {
		return id
	}
	id = siemIdentity{at: time.Now()}
	var isAdmin int
	var role string
	db.QueryRow(`SELECT is_admin, COALESCE(role,'') FROM users WHERE id=?`, userID).Scan(&isAdmin, &role)
	id.Role = roleOf(isAdmin == 1, role)
	var provider, subject, email string
	if db.QueryRow(`SELECT p.name, i.subject, COALESCE(i.email,'') FROM user_identities i JOIN oidc_providers p ON p.id=i.provider_id WHERE i.user_id=? ORDER BY i.id LIMIT 1`, userID).Scan(&provider, &subject, &email) == nil {
		id.Directory, id.Email = map[string]string{"kind": "oidc", "provider": provider, "subject": subject}, email
	} else {
		var dir, dn, key string
		if db.QueryRow(`SELECT d.name, i.dn, i.object_key FROM ldap_identities i JOIN ldap_directories d ON d.id=i.directory_id WHERE i.user_id=? ORDER BY i.id LIMIT 1`, userID).Scan(&dir, &dn, &key) == nil {
			id.Directory = map[string]string{"kind": "ldap", "directory": dir, "dn": dn, "subject": key}
		}
	}
	siemIdentityCache.Lock()
	if len(siemIdentityCache.m) > 5000 {
		siemIdentityCache.m = map[int64]siemIdentity{}
	}
	siemIdentityCache.m[userID] = id
	siemIdentityCache.Unlock()
	return id
}

var siemHostname = func() string {
	h, _ := os.Hostname()
	if h == "" {
		h = "wrm"
	}
	return h
}()

func siemOutcome(action string) string {
	switch {
	case strings.HasSuffix(action, "_failed") || strings.HasSuffix(action, "_refused") || strings.HasSuffix(action, "_denied") || strings.HasSuffix(action, ".denied"):
		return "failure"
	}
	return "success"
}

// siemEvent is the JSON document every destination receives (ECS-like field names).
func siemEvent(r siemRow) map[string]interface{} {
	ev := map[string]interface{}{
		"@timestamp": r.TS,
		"event": map[string]interface{}{"id": r.ID, "action": r.Action, "category": strings.SplitN(r.Action, ".", 2)[0],
			"outcome": siemOutcome(r.Action), "kind": map[bool]string{true: "alert", false: "event"}[r.Action == "security.alert"]},
		"observer": map[string]string{"product": "WRM PRO", "version": AppVersion, "hostname": siemHostname},
		"hash":     map[string]string{"prev": r.Prev, "this": r.Hash},
	}
	user := map[string]interface{}{"name": r.Username}
	if r.UserID.Valid {
		user["id"] = r.UserID.Int64
		idn := siemIdentityOf(r.UserID.Int64)
		user["roles"] = []string{idn.Role}
		if idn.Email != "" {
			user["email"] = idn.Email
		}
		if idn.Directory != nil {
			user["directory"] = idn.Directory
		}
	}
	ev["user"] = user
	if r.IP != "" {
		ev["source"] = map[string]string{"ip": r.IP}
	}
	if r.Target != "" {
		ev["target"] = r.Target
	}
	if r.Details != "" {
		var d interface{}
		if json.Unmarshal([]byte(r.Details), &d) == nil {
			ev["details"] = d
		} else {
			ev["details"] = r.Details
		}
	}
	if r.ConnID.Valid {
		ev["connection_id"] = r.ConnID.Int64
	}
	if r.SessionID.Valid {
		ev["session_id"] = r.SessionID.Int64
	}
	return ev
}

func (d *siemDest) wants(action string) bool {
	if len(d.Config.Actions) == 0 {
		return true
	}
	for _, p := range d.Config.Actions {
		if p != "" && strings.HasPrefix(action, p) {
			return true
		}
	}
	return false
}

// ─── delivery ────────────────────────────────────────

var siemWake = make(chan struct{}, 1)

// siemNotify wakes the dispatcher after an audit write.
func siemNotify() {
	select {
	case siemWake <- struct{}{}:
	default:
	}
}

var siemBackoff = struct {
	sync.Mutex
	next  map[int]time.Time
	tries map[int]int
}{next: map[int]time.Time{}, tries: map[int]int{}}

var siemRunMu sync.Mutex

func siemLoop() {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-siemWake:
			time.Sleep(500 * time.Millisecond) // gather what is written together
		case <-t.C:
		}
		siemRunOnce()
	}
}

// siemRunOnce delivers what is waiting for every enabled destination.
func siemRunOnce() {
	siemRunMu.Lock()
	defer siemRunMu.Unlock()
	for _, d := range siemDestinations(true) {
		siemBackoff.Lock()
		wait := time.Now().Before(siemBackoff.next[d.ID])
		siemBackoff.Unlock()
		if wait {
			continue
		}
		for i := 0; i < 20; i++ { // at most 20 batches per round per destination
			n, err := siemDeliverBatch(d)
			if err != nil {
				siemBackoff.Lock()
				siemBackoff.tries[d.ID]++
				delay := time.Duration(5<<min(siemBackoff.tries[d.ID], 6)) * time.Second
				if delay > 5*time.Minute {
					delay = 5 * time.Minute
				}
				siemBackoff.next[d.ID] = time.Now().Add(delay)
				siemBackoff.Unlock()
				break
			}
			siemBackoff.Lock()
			delete(siemBackoff.tries, d.ID)
			delete(siemBackoff.next, d.ID)
			siemBackoff.Unlock()
			if n < siemBatch {
				break
			}
		}
	}
}

func siemDeliverBatch(d *siemDest) (int, error) {
	rows, err := db.Query(`SELECT id, ts, user_id, COALESCE(username,''), COALESCE(ip,''), action, COALESCE(target,''), COALESCE(details,''),
		conn_id, session_id, COALESCE(prev_hash,''), COALESCE(hash,'') FROM audit_log WHERE id > ? ORDER BY id LIMIT ?`, d.Cursor, siemBatch)
	if err != nil {
		return 0, err
	}
	var batch []siemRow
	var last int64
	n := 0
	for rows.Next() {
		var r siemRow
		if rows.Scan(&r.ID, &r.TS, &r.UserID, &r.Username, &r.IP, &r.Action, &r.Target, &r.Details, &r.ConnID, &r.SessionID, &r.Prev, &r.Hash) != nil {
			continue
		}
		n++
		last = r.ID
		if d.wants(r.Action) {
			batch = append(batch, r)
		}
	}
	rows.Close()
	if n == 0 {
		return 0, nil
	}
	if len(batch) > 0 {
		evs := make([]map[string]interface{}, len(batch))
		for i, r := range batch {
			evs[i] = siemEvent(r)
		}
		if err := siemSend(d, batch, evs); err != nil {
			msg := truncateStr(redactText(err.Error()), 400)
			db.Exec(`UPDATE siem_destinations SET failed=failed+1, last_error=?, last_error_at=? WHERE id=?`, msg, nowRFC(), d.ID)
			d.LastError = msg
			return 0, err
		}
	}
	db.Exec(`UPDATE siem_destinations SET cursor=?, sent=sent+?, last_ok_at=?, last_error='' WHERE id=? AND cursor < ?`, last, len(batch), nowRFC(), d.ID, last)
	d.Cursor = last
	return n, nil
}

func siemSend(d *siemDest, rows []siemRow, evs []map[string]interface{}) error {
	switch d.Kind {
	case "syslog":
		return siemSendSyslog(d, rows, evs)
	case "jsonl":
		return siemSendFile(d, evs)
	case "splunk":
		var b bytes.Buffer
		for i, ev := range evs {
			t, _ := time.Parse(time.RFC3339, rows[i].TS)
			item := map[string]interface{}{"time": t.Unix(), "host": siemHostname, "source": "wrm", "sourcetype": nonEmpty(d.Config.SourceType, "wrm:audit"), "event": ev}
			if d.Config.Index != "" {
				item["index"] = d.Config.Index
			}
			j, _ := json.Marshal(item)
			b.Write(j)
			b.WriteByte('\n')
		}
		return siemPost(d, siemSplunkURL(d.URL), "application/json", b.Bytes(), map[string]string{"Authorization": "Splunk " + d.secret})
	case "elastic":
		var b bytes.Buffer
		meta, _ := json.Marshal(map[string]interface{}{"create": map[string]string{"_index": nonEmpty(d.Config.Index, "wrm-audit")}})
		for _, ev := range evs {
			j, _ := json.Marshal(ev)
			b.Write(meta)
			b.WriteByte('\n')
			b.Write(j)
			b.WriteByte('\n')
		}
		h := map[string]string{}
		if d.secret != "" {
			h["Authorization"] = "ApiKey " + d.secret
		}
		return siemPost(d, strings.TrimRight(d.URL, "/")+"/_bulk", "application/x-ndjson", b.Bytes(), h)
	case "sentinel":
		tok, err := siemSentinelToken(d)
		if err != nil {
			return err
		}
		items := make([]map[string]interface{}, len(evs))
		for i, ev := range evs {
			raw, _ := json.Marshal(ev)
			items[i] = map[string]interface{}{"TimeGenerated": rows[i].TS, "EventId": rows[i].ID, "Action": rows[i].Action, "UserName": rows[i].Username,
				"SourceIp": rows[i].IP, "Target": rows[i].Target, "Hash": rows[i].Hash, "PrevHash": rows[i].Prev, "Event": string(raw)}
		}
		body, _ := json.Marshal(items)
		u := strings.TrimRight(d.URL, "/") + "/dataCollectionRules/" + url.PathEscape(d.Config.DCRID) + "/streams/" + url.PathEscape(nonEmpty(d.Config.Stream, "Custom-WRMAudit_CL")) + "?api-version=2023-01-01"
		return siemPost(d, u, "application/json", body, map[string]string{"Authorization": "Bearer " + tok})
	case "http":
		body, _ := json.Marshal(evs)
		h := map[string]string{}
		if d.secret != "" {
			h["Authorization"] = "Bearer " + d.secret
			mac := hmac.New(sha256.New, []byte(d.secret))
			mac.Write(body)
			h["X-WRM-Signature"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
		}
		return siemPost(d, d.URL, "application/json", body, h)
	}
	return fmt.Errorf("unknown kind %q", d.Kind)
}

// siemSplunkURL accepts the HEC base address or the full event endpoint.
func siemSplunkURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || strings.Trim(u.Path, "/") != "" {
		return raw
	}
	return strings.TrimRight(raw, "/") + "/services/collector/event"
}

func siemTLSConfig(d *siemDest, serverName string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
	if strings.TrimSpace(d.Config.CAPEM) != "" {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM([]byte(d.Config.CAPEM)) {
			return nil, errors.New("the CA certificate is not valid PEM")
		}
		cfg.RootCAs = pool
	}
	if d.Kind == "syslog" && strings.TrimSpace(d.Config.ClientCert) != "" {
		cert, err := tls.X509KeyPair([]byte(d.Config.ClientCert), []byte(d.secret))
		if err != nil {
			return nil, fmt.Errorf("client certificate and key do not match: %v", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

func siemClient(d *siemDest, host string) (*http.Client, error) {
	tc, err := siemTLSConfig(d, host)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{TLSClientConfig: tc, Proxy: http.ProxyFromEnvironment, TLSHandshakeTimeout: 10 * time.Second}
	return &http.Client{Transport: tr, Timeout: siemHTTPTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func siemPost(d *siemDest, raw, ctype string, body []byte, headers map[string]string) error {
	if err := oidcURLAllowed(raw); err != nil {
		return err
	}
	u, _ := url.Parse(raw)
	c, err := siemClient(d, u.Hostname())
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, raw, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", "WRM-PRO/"+AppVersion)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(strings.TrimSpace(string(rb)), 200))
	}
	if d.Kind == "elastic" {
		var res struct {
			Errors bool `json:"errors"`
		}
		if json.Unmarshal(rb, &res) == nil && res.Errors {
			return errors.New("Elasticsearch refused some documents (check the index and its mapping)")
		}
	}
	return nil
}

var siemTokens = struct {
	sync.Mutex
	m map[int]siemToken
}{m: map[int]siemToken{}}

// siemToken is a cached Entra token; key ties it to the settings it was issued for, so a
// destination changed while a delivery runs never reuses a token for another address.
type siemToken struct {
	tok, key string
	exp      time.Time
}

func siemTokenKey(d *siemDest) string {
	return sha256Hex(d.Config.TenantID + "\x00" + d.Config.ClientID + "\x00" + d.URL + "\x00" + d.secret)
}

// siemSentinelToken gets an Entra access token for the Logs Ingestion API (client credentials).
func siemSentinelToken(d *siemDest) (string, error) {
	siemTokens.Lock()
	t, ok := siemTokens.m[d.ID]
	siemTokens.Unlock()
	key := siemTokenKey(d)
	if ok && t.key == key && time.Until(t.exp) > time.Minute {
		return t.tok, nil
	}
	if d.Config.TenantID == "" || d.Config.ClientID == "" || d.secret == "" {
		return "", errors.New("Sentinel needs the tenant ID, the application (client) ID and its client secret")
	}
	form := url.Values{"client_id": {d.Config.ClientID}, "client_secret": {d.secret}, "grant_type": {"client_credentials"}, "scope": {"https://monitor.azure.com//.default"}}
	tokenURL := siemEntraLogin + "/" + url.PathEscape(d.Config.TenantID) + "/oauth2/v2.0/token"
	lu, _ := url.Parse(siemEntraLogin)
	c, err := siemClient(d, lu.Hostname())
	if err != nil {
		return "", err
	}
	resp, err := c.PostForm(tokenURL, form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Token   string `json:"access_token"`
		Expires int    `json:"expires_in"`
		Error   string `json:"error_description"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out)
	if resp.StatusCode != 200 || out.Token == "" {
		return "", fmt.Errorf("Entra refused the token request (HTTP %d): %s", resp.StatusCode, truncateStr(out.Error, 200))
	}
	siemTokens.Lock()
	siemTokens.m[d.ID] = siemToken{tok: out.Token, key: key, exp: time.Now().Add(time.Duration(out.Expires) * time.Second)}
	siemTokens.Unlock()
	return out.Token, nil
}

// ─── syslog (RFC 5424 over TLS, RFC 5425 framing) ────

func siemSDValue(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `]`, `\]`, "\n", " ", "\r", " ")
	return r.Replace(s)
}

func siemSyslogLine(d *siemDest, r siemRow, ev map[string]interface{}) string {
	fac := d.Config.Facility
	if fac <= 0 || fac > 23 {
		fac = 10 // authpriv
	}
	sev := 5 // notice
	if r.Action == "security.alert" {
		sev = 2 // critical
	} else if siemOutcome(r.Action) == "failure" {
		sev = 4 // warning
	}
	msgID := r.Action
	if len(msgID) > 32 {
		msgID = msgID[:32]
	}
	j, _ := json.Marshal(ev)
	// 32473 is the private enterprise number reserved for documentation (RFC 5612).
	sd := fmt.Sprintf(`[wrm@32473 id="%d" user="%s" ip="%s" hash="%s"]`, r.ID, siemSDValue(r.Username), siemSDValue(r.IP), siemSDValue(r.Hash))
	return fmt.Sprintf("<%d>1 %s %s wrm - %s %s %s", fac*8+sev, r.TS, siemHostname, strings.ReplaceAll(msgID, " ", "_"), sd, j)
}

func siemSendSyslog(d *siemDest, rows []siemRow, evs []map[string]interface{}) error {
	host, _, err := net.SplitHostPort(d.URL)
	if err != nil {
		return fmt.Errorf("the syslog address must be host:port (for example siem.example.com:6514)")
	}
	tc, err := siemTLSConfig(d, host)
	if err != nil {
		return err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), siemHTTPTimeout)
	defer cancel()
	raw, err := dialer.DialContext(ctx, "tcp", d.URL)
	if err != nil {
		return err
	}
	conn := tls.Client(raw, tc)
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(siemHTTPTimeout))
	if err := conn.HandshakeContext(ctx); err != nil {
		return err
	}
	var b bytes.Buffer
	for i, r := range rows {
		line := siemSyslogLine(d, r, evs[i])
		b.WriteString(strconv.Itoa(len(line)))
		b.WriteByte(' ')
		b.WriteString(line)
	}
	_, err = conn.Write(b.Bytes())
	return err
}

// ─── JSON lines file ─────────────────────────────────

var siemFileMu sync.Mutex

func siemCheckPath(p string) error {
	if !filepath.IsAbs(p) {
		return errors.New("enter an absolute file path, for example /var/log/wrm/audit.jsonl")
	}
	ext := strings.ToLower(filepath.Ext(p))
	if ext != ".jsonl" && ext != ".log" && ext != ".json" {
		return errors.New("the file name must end in .jsonl, .json or .log")
	}
	if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return errors.New("the file is a symbolic link; use the real path")
	}
	if fi, err := os.Stat(filepath.Dir(p)); err != nil || !fi.IsDir() {
		return errors.New("the folder " + filepath.Dir(p) + " does not exist")
	}
	return nil
}

func siemSendFile(d *siemDest, evs []map[string]interface{}) error {
	p := filepath.Clean(d.URL)
	if err := siemCheckPath(p); err != nil {
		return err
	}
	siemFileMu.Lock()
	defer siemFileMu.Unlock()
	if fi, err := os.Stat(p); err == nil && fi.Size() > siemFileMax {
		for i := siemFileKeep; i >= 1; i-- {
			from := p
			if i > 1 {
				from = p + "." + strconv.Itoa(i-1)
			}
			os.Rename(from, p+"."+strconv.Itoa(i))
		}
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	var b bytes.Buffer
	for _, ev := range evs {
		j, _ := json.Marshal(ev)
		b.Write(j)
		b.WriteByte('\n')
	}
	_, err = f.Write(b.Bytes())
	return err
}

// ─── admin API ───────────────────────────────────────

type siemInput struct {
	Name        *string     `json:"name"`
	Kind        *string     `json:"kind"`
	URL         *string     `json:"url"`
	Config      *siemConfig `json:"config"`
	Secret      *string     `json:"secret"` // "" keeps the stored one
	ClearSecret bool        `json:"clear_secret"`
	Enabled     *bool       `json:"enabled"`
	StartFrom   string      `json:"start_from"` // new destinations: now (default) | all
}

func siemSameAddress(kind, a, b string) bool {
	if kind == "jsonl" {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	if kind == "syslog" {
		return strings.EqualFold(a, b)
	}
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	return err1 == nil && err2 == nil && strings.EqualFold(ua.Host, ub.Host) && ua.Scheme == ub.Scheme
}

func (in *siemInput) apply(d *siemDest) error {
	oldURL, oldKind, hadSecret := d.URL, d.Kind, d.secret != ""
	if in.Name != nil {
		d.Name = truncateStr(strings.TrimSpace(*in.Name), 60)
	}
	if in.Kind != nil {
		d.Kind = strings.TrimSpace(*in.Kind)
	}
	if in.URL != nil {
		d.URL = truncateStr(strings.TrimSpace(*in.URL), 500)
	}
	if in.Config != nil {
		c := *in.Config
		c.Index, c.SourceType, c.Stream = truncateStr(strings.TrimSpace(c.Index), 100), truncateStr(strings.TrimSpace(c.SourceType), 100), truncateStr(strings.TrimSpace(c.Stream), 100)
		c.TenantID, c.ClientID, c.DCRID = truncateStr(strings.TrimSpace(c.TenantID), 100), truncateStr(strings.TrimSpace(c.ClientID), 100), truncateStr(strings.TrimSpace(c.DCRID), 100)
		c.CAPEM, c.ClientCert = truncateStr(strings.TrimSpace(c.CAPEM), 20000), truncateStr(strings.TrimSpace(c.ClientCert), 20000)
		var acts []string
		for _, a := range c.Actions {
			if a = strings.TrimSpace(a); a != "" && len(acts) < 50 {
				acts = append(acts, truncateStr(a, 60))
			}
		}
		c.Actions = acts
		d.Config = c
	}
	if d.Name == "" {
		return errors.New("enter a name, for example Splunk")
	}
	ok := false
	for _, k := range siemKinds {
		ok = ok || k == d.Kind
	}
	if !ok {
		return errors.New("unknown kind: use syslog, jsonl, splunk, elastic, sentinel or http")
	}
	switch d.Kind {
	case "syslog":
		if _, port, err := net.SplitHostPort(d.URL); err != nil || port == "" {
			return errors.New("enter the syslog server as host:port, for example siem.example.com:6514 (TLS)")
		}
	case "jsonl":
		if err := siemCheckPath(filepath.Clean(d.URL)); err != nil {
			return err
		}
	default:
		if err := oidcURLAllowed(d.URL); err != nil {
			return err
		}
	}
	if d.Kind == "sentinel" && (d.Config.TenantID == "" || d.Config.ClientID == "" || d.Config.DCRID == "") {
		return errors.New("Sentinel needs the tenant ID, the application (client) ID and the data collection rule's immutable ID")
	}
	if in.ClearSecret {
		d.secret = ""
	}
	if in.Secret != nil && *in.Secret != "" {
		d.secret = truncateStr(*in.Secret, 20000)
	} else if hadSecret && d.ID > 0 && (oldKind != d.Kind || !siemSameAddress(d.Kind, oldURL, d.URL)) {
		// a stored secret is never sent to another address
		return errors.New("Enter the secret again when you change the address")
	}
	if d.Kind == "splunk" && d.secret == "" {
		return errors.New("Splunk needs the HEC token")
	}
	if d.Kind == "sentinel" && d.secret == "" {
		return errors.New("Sentinel needs the client secret of the Entra application")
	}
	if d.Kind == "syslog" && d.Config.ClientCert != "" {
		if _, err := tls.X509KeyPair([]byte(d.Config.ClientCert), []byte(d.secret)); err != nil {
			return errors.New("the client certificate needs its private key (PEM) as the secret")
		}
	}
	if d.Config.CAPEM != "" && !x509.NewCertPool().AppendCertsFromPEM([]byte(d.Config.CAPEM)) {
		return errors.New("the CA certificate is not valid PEM")
	}
	return nil
}

func siemJSON(d *siemDest) map[string]interface{} {
	var maxID int64
	db.QueryRow(`SELECT COALESCE(MAX(id),0) FROM audit_log`).Scan(&maxID)
	waiting := maxID - d.Cursor
	if waiting < 0 {
		waiting = 0
	}
	siemBackoff.Lock()
	next := siemBackoff.next[d.ID]
	siemBackoff.Unlock()
	out := map[string]interface{}{"id": d.ID, "name": d.Name, "kind": d.Kind, "url": d.URL, "config": d.Config, "has_secret": d.HasSecret,
		"enabled": d.Enabled, "sent": d.Sent, "failed": d.Failed, "last_ok_at": d.LastOKAt, "last_error": d.LastError,
		"last_error_at": d.LastErrorAt, "created_at": d.CreatedAt, "waiting": waiting}
	if !next.IsZero() && time.Now().Before(next) {
		out["retry_at"] = next.UTC().Format(time.RFC3339)
	}
	switch {
	case !d.Enabled:
		out["state"] = "off"
	case d.LastError != "":
		out["state"] = "failing"
	case waiting > siemBatch*5:
		out["state"] = "behind"
	default:
		out["state"] = "ok"
	}
	return out
}

func saveSIEM(r *http.Request, adminID int, d *siemDest, startFrom string) error {
	cfg, _ := json.Marshal(d.Config)
	if d.ID == 0 {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM siem_destinations`).Scan(&n)
		if n >= siemMaxDest {
			return fmt.Errorf("at most %d destinations", siemMaxDest)
		}
		var cursor int64
		if startFrom != "all" {
			db.QueryRow(`SELECT COALESCE(MAX(id),0) FROM audit_log`).Scan(&cursor)
		}
		res, err := db.Exec(`INSERT INTO siem_destinations (name, kind, url, config, secret, enabled, cursor, created_by, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			d.Name, d.Kind, d.URL, string(cfg), encryptValue(d.secret), boolToInt(d.Enabled), cursor, adminID, nowRFC())
		if err != nil {
			return errors.New("a destination with this name already exists")
		}
		id, _ := res.LastInsertId()
		d.ID = int(id)
		auditLog(r, adminID, "siem.destination_created", d.Name, map[string]interface{}{"siem_id": d.ID, "kind": d.Kind, "enabled": d.Enabled, "start_from": nonEmpty(startFrom, "now")})
		return nil
	}
	if _, err := db.Exec(`UPDATE siem_destinations SET name=?, kind=?, url=?, config=?, secret=?, enabled=?, last_error='' WHERE id=?`,
		d.Name, d.Kind, d.URL, string(cfg), encryptValue(d.secret), boolToInt(d.Enabled), d.ID); err != nil {
		return errors.New("a destination with this name already exists")
	}
	siemTokens.Lock()
	delete(siemTokens.m, d.ID)
	siemTokens.Unlock()
	siemBackoff.Lock()
	delete(siemBackoff.next, d.ID)
	delete(siemBackoff.tries, d.ID)
	siemBackoff.Unlock()
	auditLog(r, adminID, "siem.destination_updated", d.Name, map[string]interface{}{"siem_id": d.ID, "kind": d.Kind, "enabled": d.Enabled})
	return nil
}

// siemTest sends one test event to a saved destination and reports the result.
func siemTest(d *siemDest, by string) error {
	r := siemRow{ID: 0, TS: nowRFC(), Username: by, Action: "siem.test", Target: d.Name, Details: `{"message":"Test event from WRM PRO"}`}
	return siemSend(d, []siemRow{r}, []map[string]interface{}{siemEvent(r)})
}

// /api/admin/siem, /api/admin/siem/{id}, /api/admin/siem/{id}/test
func apiAdminSIEMHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/siem"), "/")
	parts := strings.Split(rest, "/")
	var in siemInput
	decode := func() bool {
		if err := json.NewDecoder(io.LimitReader(r.Body, 128<<10)).Decode(&in); err != nil && err != io.EOF {
			jsonError(w, "Bad JSON", 400)
			return false
		}
		return true
	}
	switch {
	case rest == "" && r.Method == http.MethodGet:
		list := []map[string]interface{}{}
		for _, d := range siemDestinations(false) {
			list = append(list, siemJSON(d))
		}
		jsonOK(w, map[string]interface{}{"destinations": list, "kinds": siemKinds, "alerts": recentAlerts(20), "hostname": siemHostname})
	case rest == "" && r.Method == http.MethodPost:
		if !decode() || !requireStepUp(w, r, "siem.destination") {
			return
		}
		d := &siemDest{Enabled: true}
		if err := in.apply(d); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if in.Enabled != nil {
			d.Enabled = *in.Enabled
		}
		if err := saveSIEM(r, adminID, d, in.StartFrom); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		siemNotify()
		jsonOK(w, map[string]interface{}{"id": d.ID})
	case len(parts) == 1 && rest != "" && (r.Method == http.MethodPut || r.Method == http.MethodDelete):
		d, err := loadSIEM(atoiOr(parts[0], 0))
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		if !requireStepUp(w, r, "siem.destination") {
			return
		}
		if r.Method == http.MethodDelete {
			db.Exec(`DELETE FROM siem_destinations WHERE id=?`, d.ID)
			auditLog(r, adminID, "siem.destination_deleted", d.Name, map[string]interface{}{"siem_id": d.ID, "kind": d.Kind})
			jsonOK(w, map[string]bool{"ok": true})
			return
		}
		if !decode() {
			return
		}
		if err := in.apply(d); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if in.Enabled != nil {
			d.Enabled = *in.Enabled
		}
		if err := saveSIEM(r, adminID, d, ""); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		siemNotify()
		jsonOK(w, map[string]bool{"ok": true})
	case len(parts) == 2 && parts[1] == "test" && r.Method == http.MethodPost:
		d, err := loadSIEM(atoiOr(parts[0], 0))
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		by := usernameOf(adminID)
		if err := siemTest(d, by); err != nil {
			auditLog(r, adminID, "siem.test_failed", d.Name, map[string]interface{}{"siem_id": d.ID})
			jsonOK(w, map[string]interface{}{"ok": false, "error": truncateStr(redactText(err.Error()), 400)})
			return
		}
		auditLog(r, adminID, "siem.tested", d.Name, map[string]interface{}{"siem_id": d.ID})
		jsonOK(w, map[string]interface{}{"ok": true})
	default:
		jsonError(w, "Not found", 404)
	}
}
