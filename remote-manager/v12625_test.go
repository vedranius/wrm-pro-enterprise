// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ─── a fake LDAP / Active Directory server ───────────

type fakeLDAPEntry struct {
	dn       string
	attrs    map[string][]string // lower-case names
	guid     []byte
	password string
}

type fakeLDAP struct {
	t       *testing.T
	ln      net.Listener
	tlsConf *tls.Config
	caPEM   string
	mu      sync.Mutex
	entries []*fakeLDAPEntry
	binds   []string
	ad      bool
}

const fakeLDAPBase = "DC=example,DC=com"

func newFakeLDAP(t *testing.T, ad bool) *fakeLDAP {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "ldap.example.com"}, DNSNames: []string{"ldap.example.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeLDAP{t: t, ad: ad}
	f.caPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	f.tlsConf = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	f.ln, _ = net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { f.ln.Close() })
	go func() {
		for {
			c, err := f.ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	old := ldapDialHook
	ldapDialHook = func(network, addr string) (net.Conn, error) {
		if !strings.HasPrefix(addr, "ldap.example.com:") {
			return nil, fmt.Errorf("unexpected address %s", addr)
		}
		return net.Dial("tcp", f.ln.Addr().String())
	}
	t.Cleanup(func() { ldapDialHook = old })
	f.add("DC=example,DC=com", "", map[string][]string{"objectClass": {"domain"}})
	f.add("CN=svc-wrm,OU=Service,"+fakeLDAPBase, "svc-secret", map[string][]string{"objectClass": {"user"}, "sAMAccountName": {"svc-wrm"}})
	return f
}

func (f *fakeLDAP) add(dn, password string, attrs map[string][]string) *fakeLDAPEntry {
	e := &fakeLDAPEntry{dn: dn, password: password, attrs: map[string][]string{}, guid: make([]byte, 16)}
	rand.Read(e.guid)
	for k, v := range attrs {
		e.attrs[strings.ToLower(k)] = v
	}
	f.mu.Lock()
	f.entries = append(f.entries, e)
	f.mu.Unlock()
	return e
}

func (f *fakeLDAP) user(login, password string, groups ...string) *fakeLDAPEntry {
	dn := "CN=" + login + ",OU=People," + fakeLDAPBase
	e := f.add(dn, password, map[string][]string{"objectClass": {"top", "person", "user"}, "objectCategory": {"person"},
		"sAMAccountName": {login}, "uid": {login}, "userPrincipalName": {login + "@example.com"}, "mail": {login + "@example.com"},
		"displayName": {strings.ToUpper(login[:1]) + login[1:] + " Demo"}, "userAccountControl": {"512"}})
	for _, g := range groups {
		f.addMember(g, dn)
	}
	return e
}

func (f *fakeLDAP) group(name string, memberOf ...string) string {
	dn := "CN=" + name + ",OU=Groups," + fakeLDAPBase
	f.add(dn, "", map[string][]string{"objectClass": {"top", "group", "groupOfNames"}, "cn": {name}})
	for _, g := range memberOf {
		f.addMember(g, dn)
	}
	return dn
}

func (f *fakeLDAP) addMember(groupDN, member string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.entries {
		if strings.EqualFold(e.dn, groupDN) {
			e.attrs["member"] = append(e.attrs["member"], member)
		}
	}
}

func (f *fakeLDAP) removeMember(groupDN, member string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.entries {
		if strings.EqualFold(e.dn, groupDN) {
			var keep []string
			for _, m := range e.attrs["member"] {
				if !strings.EqualFold(m, member) {
					keep = append(keep, m)
				}
			}
			e.attrs["member"] = keep
		}
	}
}

func (f *fakeLDAP) set(e *fakeLDAPEntry, attr string, vals ...string) {
	f.mu.Lock()
	e.attrs[strings.ToLower(attr)] = vals
	f.mu.Unlock()
}

// peekConn lets the server look at the first byte (TLS handshake or LDAP message).
type peekConn struct {
	net.Conn
	r *bufio.Reader
}

func (p *peekConn) Read(b []byte) (int, error) { return p.r.Read(b) }

func (f *fakeLDAP) serve(raw net.Conn) {
	defer raw.Close()
	pc := &peekConn{Conn: raw, r: bufio.NewReader(raw)}
	var c net.Conn = pc
	if first, err := pc.r.Peek(1); err != nil {
		return
	} else if first[0] == 0x16 { // ldaps://: TLS from the start
		tc := tls.Server(pc, f.tlsConf)
		if tc.Handshake() != nil {
			return
		}
		c = tc
	}
	var rd io.Reader = c
	var w io.Writer = c
	for {
		p, err := berRead(rd)
		if err != nil || len(p.Children) < 2 {
			return
		}
		id := p.Children[0].Int()
		op := p.Children[1]
		reply := func(op *berPacket, controls ...*berPacket) {
			msg := berNew(berTagSequence, berInt(berTagInteger, id), op)
			if len(controls) > 0 {
				msg.Children = append(msg.Children, berNew(berContext, controls...))
			}
			w.Write(msg.Bytes())
		}
		result := func(tag byte, code int, msg string) *berPacket {
			return berNew(tag, berInt(berTagEnumerated, int64(code)), berStr(""), berStr(msg))
		}
		switch op.Tag {
		case ldapOpExtendedReq | berConstructed:
			reply(result(ldapOpExtendedResp, 0, ""))
			tc := tls.Server(c, f.tlsConf)
			if tc.Handshake() != nil {
				return
			}
			rd, w = tc, tc
		case ldapOpBindRequest | berConstructed:
			dn, pw := op.child(1).Str(), op.child(2).Str()
			code := 49
			f.mu.Lock()
			f.binds = append(f.binds, dn)
			for _, e := range f.entries {
				if strings.EqualFold(e.dn, dn) && e.password != "" && e.password == pw {
					code = 0
				}
			}
			f.mu.Unlock()
			reply(result(ldapOpBindResponse, code, ""))
		case ldapOpUnbind:
			return
		case ldapOpSearchRequest | berConstructed:
			f.search(id, op, p, reply, result)
		default:
			return
		}
	}
}

func (f *fakeLDAP) search(id int64, op, msg *berPacket, reply func(*berPacket, ...*berPacket), result func(byte, int, string) *berPacket) {
	base, scope, filter := strings.ToLower(op.child(0).Str()), op.child(1).Int(), op.child(6)
	var want []string
	for _, a := range op.child(7).Children {
		want = append(want, strings.ToLower(a.Str()))
	}
	pageSize, offset := 0, 0
	if len(msg.Children) > 2 {
		for _, ctl := range msg.Children[2].Children {
			if ctl.child(0).Str() == ldapOIDPagedResults {
				v := ctl.Children[len(ctl.Children)-1]
				pp, _ := berRead(bytes.NewReader(v.Value))
				pageSize = int(pp.child(0).Int())
				offset, _ = strconv.Atoi(pp.child(1).Str())
			}
		}
	}
	f.mu.Lock()
	var hits []*fakeLDAPEntry
	for _, e := range f.entries {
		dn := strings.ToLower(e.dn)
		if (scope == 0 && dn != base) || (scope != 0 && !strings.HasSuffix(dn, base)) {
			continue
		}
		if f.match(e, filter) {
			hits = append(hits, e)
		}
	}
	f.mu.Unlock()
	next := ""
	if pageSize > 0 && offset+pageSize < len(hits) {
		next = strconv.Itoa(offset + pageSize)
		hits = hits[offset : offset+pageSize]
	} else if offset < len(hits) {
		hits = hits[offset:]
	} else {
		hits = nil
	}
	for _, e := range hits {
		attrs := berNew(berTagSequence)
		addAttr := func(name string, vals [][]byte) {
			set := berNew(berTagSet)
			for _, v := range vals {
				set.Children = append(set.Children, berOctets(berTagOctetString, v))
			}
			attrs.Children = append(attrs.Children, berNew(berTagSequence, berStr(name), set))
		}
		wanted := func(k string) bool {
			if len(want) == 0 {
				return true
			}
			for _, x := range want {
				if x == k {
					return true
				}
			}
			return false
		}
		for k, v := range e.attrs {
			if !wanted(k) || k == "member" && !wanted("member") {
				continue
			}
			var vals [][]byte
			for _, s := range v {
				vals = append(vals, []byte(s))
			}
			addAttr(k, vals)
		}
		if wanted("objectguid") {
			addAttr("objectGUID", [][]byte{e.guid})
		}
		if wanted("entryuuid") {
			addAttr("entryUUID", [][]byte{[]byte(fmt.Sprintf("%x", e.guid))})
		}
		if !f.ad && wanted("memberof") {
			var mo [][]byte
			for _, g := range f.groupsOf(e.dn) {
				mo = append(mo, []byte(g))
			}
			if len(mo) > 0 {
				addAttr("memberOf", mo)
			}
		}
		reply(berNew(ldapOpSearchEntry, berStr(e.dn), attrs))
	}
	pv := berNew(berTagSequence, berInt(berTagInteger, 0), berStr(next))
	reply(result(ldapOpSearchDone, 0, ""), berNew(berTagSequence, berStr(ldapOIDPagedResults), berOctets(berTagOctetString, pv.Bytes())))
}

func (f *fakeLDAP) groupsOf(dn string) []string {
	var out []string
	for _, e := range f.entries {
		for _, m := range e.attrs["member"] {
			if strings.EqualFold(m, dn) {
				out = append(out, e.dn)
			}
		}
	}
	return out
}

func (f *fakeLDAP) inChain(group *fakeLDAPEntry, dn string, depth int) bool {
	if depth > 10 {
		return false
	}
	for _, m := range group.attrs["member"] {
		if strings.EqualFold(m, dn) {
			return true
		}
		for _, e := range f.entries {
			if strings.EqualFold(e.dn, m) && f.inChain(e, dn, depth+1) {
				return true
			}
		}
	}
	return false
}

func (f *fakeLDAP) values(e *fakeLDAPEntry, attr string) [][]byte {
	attr = strings.ToLower(attr)
	var out [][]byte
	switch attr {
	case "objectguid":
		return [][]byte{e.guid}
	case "entryuuid":
		return [][]byte{[]byte(fmt.Sprintf("%x", e.guid))}
	case "distinguishedname":
		return [][]byte{[]byte(e.dn)}
	}
	for _, v := range e.attrs[attr] {
		out = append(out, []byte(v))
	}
	return out
}

func (f *fakeLDAP) match(e *fakeLDAPEntry, p *berPacket) bool {
	switch p.Tag {
	case berContext | berConstructed:
		for _, c := range p.Children {
			if !f.match(e, c) {
				return false
			}
		}
		return true
	case berContext | berConstructed | 1:
		for _, c := range p.Children {
			if f.match(e, c) {
				return true
			}
		}
		return false
	case berContext | berConstructed | 2:
		return !f.match(e, p.child(0))
	case berContext | berConstructed | 3:
		for _, v := range f.values(e, p.child(0).Str()) {
			if bytes.EqualFold(v, p.child(1).Value) {
				return true
			}
		}
		return false
	case berContext | 7:
		return strings.EqualFold(p.Str(), "objectClass") || len(f.values(e, p.Str())) > 0
	case berContext | berConstructed | 4:
		for _, v := range f.values(e, p.child(0).Str()) {
			s, ok := strings.ToLower(string(v)), true
			for _, sub := range p.child(1).Children {
				part := strings.ToLower(string(sub.Value))
				switch sub.Tag {
				case berContext:
					ok = ok && strings.HasPrefix(s, part)
				case berContext | 2:
					ok = ok && strings.HasSuffix(s, part)
				default:
					ok = ok && strings.Contains(s, part)
				}
			}
			if ok {
				return true
			}
		}
		return false
	case berContext | berConstructed | 9:
		var rule, attr string
		var val []byte
		for _, c := range p.Children {
			switch c.Tag {
			case berContext | 1:
				rule = c.Str()
			case berContext | 2:
				attr = c.Str()
			case berContext | 3:
				val = c.Value
			}
		}
		if rule == "1.2.840.113556.1.4.1941" && strings.EqualFold(attr, "member") {
			return f.inChain(e, string(val), 0)
		}
		return false
	}
	return false
}

// ─── helpers ─────────────────────────────────────────

func keepDirectorySettings(t *testing.T) {
	t.Helper()
	keys := []string{"local_login", "ldap_sync_minutes", "oidc_unverified_hours", "require_step_up", "require_2fa", "login_max_failures"}
	saved := map[string]string{}
	for _, k := range keys {
		saved[k] = getSetting(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			setSetting(k, v)
		}
		db.Exec(`DELETE FROM ldap_identities`)
		db.Exec(`DELETE FROM ldap_directories`)
		resetLoginLimiter()
	})
}

func ensureTeam(t *testing.T, admin *testClient, name string) {
	t.Helper()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM teams WHERE name=?`, name).Scan(&n)
	if n == 0 {
		newTeam(t, admin, name)
	}
}

func newLDAPDirectory(t *testing.T, admin *testClient, f *fakeLDAP, extra map[string]interface{}) int {
	t.Helper()
	kind := "ldap"
	if f.ad {
		kind = "ad"
	}
	body := map[string]interface{}{"name": "Demo " + kind, "kind": kind, "url": "ldaps://ldap.example.com", "ca_pem": f.caPEM,
		"bind_dn": "CN=svc-wrm,OU=Service," + fakeLDAPBase, "bind_password": "svc-secret", "base_dn": fakeLDAPBase, "nested": true, "jit": true, "enabled": true,
		"rules": []map[string]interface{}{
			{"claim": "group", "value": "CN=WRM Admins,OU=Groups," + fakeLDAPBase, "role": "admin"},
			{"claim": "group", "value": "WRM Operators", "role": "operator", "teams": []string{"LDAP Ops"}},
		}}
	for k, v := range extra {
		body[k] = v
	}
	var out map[string]interface{}
	if code := admin.jsonDo("POST", "/api/admin/ldap", body, &out); code != 200 {
		t.Fatalf("create directory: %d %v", code, out)
	}
	return int(out["id"].(float64))
}

// passwordLogin signs in with the password form; on success it returns a client for the new session.
func passwordLogin(t *testing.T, srv *httptest.Server, user, pw string) (int, map[string]interface{}, *testClient) {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"username": user, "password": pw})
	req, _ := http.NewRequest("POST", srv.URL+"/api/auth/login", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	req.Header.Set("X-WRM-Request", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&out)
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			id := 0
			if v, ok := out["id"].(float64); ok {
				id = int(v)
			}
			return resp.StatusCode, out, &testClient{t: t, srv: srv, cookie: c, userID: id}
		}
	}
	return resp.StatusCode, out, nil
}

func resetLoginLimiter() {
	loginAttemptsMu.Lock()
	loginAttempts = map[string]*loginAttempt{}
	loginAttemptsMu.Unlock()
}

// ─── tests ───────────────────────────────────────────

func TestLDAPProtocolPieces(t *testing.T) {
	for _, n := range []int64{0, 1, 127, 128, 255, 256, -1, -129, 1 << 40} {
		p := berInt(berTagInteger, n)
		q, err := berRead(bytes.NewReader(p.Bytes()))
		if err != nil || q.Int() != n {
			t.Fatalf("integer %d: %v %d", n, err, q.Int())
		}
	}
	long := berStr(strings.Repeat("x", 70000))
	if q, err := berRead(bytes.NewReader(long.Bytes())); err != nil || len(q.Value) != 70000 {
		t.Fatalf("long string: %v", err)
	}
	if got := ldapEscape(`a*b(c)\d` + "\x00"); got != `a\2ab\28c\29\5cd\00` {
		t.Fatalf("escape: %s", got)
	}
	for _, f := range []string{"(&(objectClass=user)(|(sAMAccountName=jdoe)(mail=j*@example.com)))", "(!(cn=x))", "(member:1.2.840.113556.1.4.1941:=CN=a\\2cb)", "uid=jdoe", "(cn>=a)"} {
		if _, err := ldapCompileFilter(f); err != nil {
			t.Errorf("filter %s: %v", f, err)
		}
	}
	for _, f := range []string{"(&(cn=a)", "(cn=a))", "((cn=a)", "(=a)", "(cn=\\zz)"} {
		if _, err := ldapCompileFilter(f); err == nil {
			t.Errorf("bad filter %s accepted", f)
		}
	}
	if got := ldapRDNValue(`CN=WRM\, Admins,OU=Groups,DC=example,DC=com`); got != "WRM, Admins" {
		t.Fatalf("rdn: %q", got)
	}
	if ldapLoginName(`EXAMPLE\jdoe`) != "jdoe" {
		t.Fatal("netbios name")
	}
	for _, u := range []string{"http://ldap.example.com", "ldaps://user@ldap.example.com", "ldaps://ldap.example.com/dc=example", "ldaps://"} {
		if _, _, _, err := ldapParseURL(u); err == nil {
			t.Errorf("url %s accepted", u)
		}
	}
	// An empty password is never sent as a bind (it would be an anonymous bind that succeeds).
	c := &ldapConn{}
	if err := c.Bind("CN=x", ""); !ldapInvalidCredentials(err) {
		t.Fatalf("empty password: %v", err)
	}
	if ldapAccountBlocked(&ldapEntry{Attrs: map[string][][]byte{"useraccountcontrol": {[]byte("514")}}}) == "" {
		t.Fatal("disabled AD account not seen")
	}
	if ldapAccountBlocked(&ldapEntry{Attrs: map[string][][]byte{"accountexpires": {[]byte("116444736000000001")}}}) == "" {
		t.Fatal("expired AD account not seen")
	}
	if ldapAccountBlocked(&ldapEntry{Attrs: map[string][][]byte{"accountexpires": {[]byte("9223372036854775807")}, "useraccountcontrol": {[]byte("512")}}}) != "" {
		t.Fatal("enabled AD account blocked")
	}
}

func TestLDAPSignInAndMapping(t *testing.T) {
	keepRoleSettings(t)
	keepDirectorySettings(t)
	srv := newTestServer(t)
	f := newFakeLDAP(t, true)
	admin := newTestUser(t, srv, "ld-admin", true)
	ensureTeam(t, admin, "LDAP Ops")
	ops := f.group("WRM Operators")
	help := f.group("Helpdesk", ops) // nested: Helpdesk members are operators
	admins := f.group("WRM Admins")
	f.user("jdoe", "pw-jdoe", help)
	f.user("boss", "pw-boss", admins)
	off := f.user("gone", "pw-gone", ops)
	f.set(off, "userAccountControl", "514")
	f.user("outsider", "pw-out")
	newTestUser(t, srv, "jdoe", false) // a local account with the same name stays separate

	// The connection test explains each step.
	var chk map[string]interface{}
	admin.jsonDo("POST", "/api/admin/ldap/check", map[string]interface{}{"kind": "ad", "url": "ldaps://ldap.example.com", "ca_pem": f.caPEM,
		"bind_dn": "CN=svc-wrm,OU=Service," + fakeLDAPBase, "bind_password": "svc-secret", "base_dn": fakeLDAPBase, "nested": true, "username": "jdoe"}, &chk)
	if chk["ok"] != true {
		t.Fatalf("check: %v", chk)
	}
	// Without the CA certificate the TLS check fails with a hint.
	admin.jsonDo("POST", "/api/admin/ldap/check", map[string]interface{}{"kind": "ad", "url": "ldaps://ldap.example.com",
		"bind_dn": "CN=svc-wrm,OU=Service," + fakeLDAPBase, "bind_password": "svc-secret", "base_dn": fakeLDAPBase}, &chk)
	if chk["ok"] != false || !strings.Contains(fmt.Sprint(chk["steps"]), "certificate") {
		t.Fatalf("untrusted certificate: %v", chk)
	}
	// A wrong service password cannot turn the directory on.
	var bad map[string]interface{}
	if code := admin.jsonDo("POST", "/api/admin/ldap", map[string]interface{}{"name": "Broken", "kind": "ad", "url": "ldaps://ldap.example.com", "ca_pem": f.caPEM,
		"bind_dn": "CN=svc-wrm,OU=Service," + fakeLDAPBase, "bind_password": "wrong", "base_dn": fakeLDAPBase, "enabled": true}, &bad); code != 400 {
		t.Fatalf("enabled with a wrong service password: %d %v", code, bad)
	}
	did := newLDAPDirectory(t, admin, f, nil)
	// The stored service password is never sent to a new server address.
	if code := admin.jsonDo("PUT", fmt.Sprintf("/api/admin/ldap/%d", did), map[string]interface{}{"url": "ldaps://other.example.com"}, &bad); code != 400 {
		t.Fatalf("new address without the password: %d %v", code, bad)
	}
	admin.jsonDo("POST", "/api/admin/ldap/check", map[string]interface{}{"directory": did, "url": "ldaps://other.example.com"}, &chk)
	if chk["ok"] != false || !strings.Contains(fmt.Sprint(chk["steps"]), "password again") {
		t.Fatalf("check against a new address with the stored password: %v", chk)
	}

	// The service password never reaches the browser.
	var list map[string]interface{}
	admin.jsonDo("GET", "/api/admin/ldap", nil, &list)
	if raw, _ := json.Marshal(list); strings.Contains(string(raw), "svc-secret") || !strings.Contains(string(raw), `"has_bind_password":true`) {
		t.Fatalf("directory list: %s", raw)
	}

	// Test sign-in shows the groups and the mapping, and creates nothing.
	var test map[string]interface{}
	admin.jsonDo("POST", fmt.Sprintf("/api/admin/ldap/%d/test", did), map[string]string{"username": "jdoe", "password": "pw-jdoe"}, &test)
	if test["ok"] != true || !strings.Contains(fmt.Sprint(test["groups"]), "WRM Operators") || test["mapping"].(map[string]interface{})["role"] != "operator" {
		t.Fatalf("test sign-in: %v", test)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM ldap_identities`).Scan(&n)
	if n != 0 {
		t.Fatal("test sign-in linked an account")
	}

	// The local "jdoe" signs in with its own password; the directory jdoe signs in with the domain name.
	if code, _, _ := passwordLogin(t, srv, "jdoe", "pw-jdoe-123456"); code != 200 {
		t.Fatalf("local jdoe: %d", code)
	}
	code, me, cl := passwordLogin(t, srv, `EXAMPLE\jdoe`, "pw-jdoe")
	if code != 200 || cl == nil {
		t.Fatalf("directory jdoe: %d %v", code, me)
	}
	if me["role"] != "operator" || me["username"] == "jdoe" || me["directory_password"] != true {
		t.Fatalf("directory jdoe payload: %v", me)
	}
	var teams []string
	rows, _ := db.Query(`SELECT t.name FROM team_members m JOIN teams t ON t.id=m.team_id WHERE m.user_id=? AND m.source='ldap'`, cl.userID)
	for rows.Next() {
		var s string
		rows.Scan(&s)
		teams = append(teams, s)
	}
	rows.Close()
	if fmt.Sprint(teams) != "[LDAP Ops]" {
		t.Fatalf("teams: %v", teams)
	}
	// The same directory account signs in again as the same WRM account (also by UPN and by its WRM name).
	for _, name := range []string{"jdoe@example.com", me["username"].(string)} {
		if code, again, _ := passwordLogin(t, srv, name, "pw-jdoe"); code != 200 || again["id"] != me["id"] {
			t.Fatalf("again as %s: %d %v", name, code, again)
		}
	}
	// Admin group by DN.
	if code, b, _ := passwordLogin(t, srv, "boss", "pw-boss"); code != 200 || b["role"] != "admin" {
		t.Fatalf("boss: %d %v", code, b)
	}
	resetLoginLimiter()
	if code, _, _ := passwordLogin(t, srv, "jdoe@example.com", "wrong"); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	if code, out, _ := passwordLogin(t, srv, "gone", "pw-gone"); code != 403 || !strings.Contains(fmt.Sprint(out["error"]), "disabled") {
		t.Fatalf("disabled account: %d %v", code, out)
	}
	if code, _, _ := passwordLogin(t, srv, "outsider", "pw-out"); code != 403 {
		t.Fatalf("user without a mapped group: %d", code)
	}
	if code, _, _ := passwordLogin(t, srv, "nobody", "x"); code != 401 {
		t.Fatalf("unknown user: %d", code)
	}
	resetLoginLimiter()

	// Directory accounts confirm step-up with their directory password; the WRM password form refuses changes.
	setSetting("require_step_up", "1")
	if code := cl.jsonDo("POST", "/api/auth/step-up", map[string]string{"password": "pw-jdoe"}, nil); code != 200 {
		t.Fatalf("step-up with the directory password: %d", code)
	}
	if code := cl.jsonDo("POST", "/api/auth/step-up", map[string]string{"password": "wrong"}, nil); code != 403 {
		t.Fatalf("step-up with a wrong password: %d", code)
	}
	resetLoginLimiter()
	var pw map[string]interface{}
	if code := cl.jsonDo("POST", "/api/auth/password", map[string]string{"current_password": "pw-jdoe", "new_password": "something-new-123"}, &pw); code != 400 || !strings.Contains(fmt.Sprint(pw["error"]), "directory") {
		t.Fatalf("password change: %d %v", code, pw)
	}

	// With local sign-in limited to break-glass, directory passwords still work, local ones do not.
	setSetting("local_login", "break_glass")
	if code, _, _ := passwordLogin(t, srv, "jdoe@example.com", "pw-jdoe"); code != 200 {
		t.Fatalf("directory sign-in with limited local sign-in: %d", code)
	}
	if code, _, _ := passwordLogin(t, srv, "jdoe", "pw-jdoe-123456"); code != 403 {
		t.Fatalf("local sign-in while limited: %d", code)
	}
}

func TestLDAPSyncEndsAccess(t *testing.T) {
	keepRoleSettings(t)
	keepDirectorySettings(t)
	srv := newTestServer(t)
	f := newFakeLDAP(t, false) // OpenLDAP-style: memberOf, entryUUID, nested groups level by level
	admin := newTestUser(t, srv, "ls-admin", true)
	ensureTeam(t, admin, "LDAP Ops")
	ops := f.group("WRM Operators")
	help := f.group("Desk", ops)
	a := f.user("anna", "pw-anna", help)
	f.user("ben", "pw-ben", ops)
	cDN := f.user("cleo", "pw-cleo", ops).dn
	did := newLDAPDirectory(t, admin, f, nil)
	_, me, anna := passwordLogin(t, srv, "anna", "pw-anna")
	if anna == nil || me["role"] != "operator" {
		t.Fatalf("anna: %v", me)
	}
	_, _, ben := passwordLogin(t, srv, "ben", "pw-ben")
	_, _, cleo := passwordLogin(t, srv, "cleo", "pw-cleo")
	if ben == nil || cleo == nil {
		t.Fatal("sign-in")
	}
	// anna is locked in the directory, ben is deleted, cleo leaves the group.
	f.set(a, "pwdAccountLockedTime", "20261018000000Z")
	f.mu.Lock()
	for i, e := range f.entries {
		if strings.HasPrefix(e.dn, "CN=ben,") {
			f.entries = append(f.entries[:i], f.entries[i+1:]...)
			break
		}
	}
	f.mu.Unlock()
	f.removeMember(ops, cDN)
	var rep ldapSyncReport
	if code := admin.jsonDo("POST", fmt.Sprintf("/api/admin/ldap/%d/sync", did), nil, &rep); code != 200 || len(rep.Ended) != 3 {
		t.Fatalf("sync: %d %+v", code, rep)
	}
	for _, c := range []*testClient{anna, ben, cleo} {
		if code := c.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
			t.Fatalf("session after sync: %d", code)
		}
	}
	if code, _, _ := passwordLogin(t, srv, "anna", "pw-anna"); code != 403 {
		t.Fatalf("locked account signs in: %d", code)
	}
	// Unlocked again in the directory: the next sign-in works and the link is active again.
	f.set(a, "pwdAccountLockedTime")
	if code, _, _ := passwordLogin(t, srv, "anna", "pw-anna"); code != 200 {
		t.Fatalf("unlocked account: %d", code)
	}
	var revoked int
	db.QueryRow(`SELECT revoked FROM ldap_identities WHERE login='anna'`).Scan(&revoked)
	if revoked != 0 {
		t.Fatal("identity still revoked after a good sign-in")
	}
}

func TestLDAPStartTLSAndLink(t *testing.T) {
	keepRoleSettings(t)
	keepDirectorySettings(t)
	srv := newTestServer(t)
	f := newFakeLDAP(t, true)
	admin := newTestUser(t, srv, "lt-admin", true)
	ensureTeam(t, admin, "LDAP Ops")
	ops := f.group("WRM Operators")
	f.user("dora", "pw-dora", ops)
	did := newLDAPDirectory(t, admin, f, map[string]interface{}{"url": "ldap://ldap.example.com", "name": "Demo StartTLS"})
	// A local account links its directory account (after a fresh confirmation) and then signs in with it.
	local := newTestUser(t, srv, "lt-local", false)
	db.Exec(`UPDATE auth_sessions SET step_up_at='2000-01-01T00:00:00Z' WHERE user_id=?`, local.userID)
	var su map[string]interface{}
	if code := local.jsonDo("POST", "/api/me/ldap", map[string]interface{}{"directory": did, "username": "dora", "password": "pw-dora"}, &su); code != 403 || su["step_up"] != true {
		t.Fatalf("link without step-up: %d %v", code, su)
	}
	db.Exec(`UPDATE auth_sessions SET step_up_at=? WHERE user_id=?`, nowRFC(), local.userID)
	if code := local.jsonDo("POST", "/api/me/ldap", map[string]interface{}{"directory": did, "username": "dora", "password": "wrong"}, nil); code != 403 {
		t.Fatalf("link with a wrong password: %d", code)
	}
	resetLoginLimiter()
	if code := local.jsonDo("POST", "/api/me/ldap", map[string]interface{}{"directory": did, "username": "dora", "password": "pw-dora"}, nil); code != 200 {
		t.Fatalf("link: %d", code)
	}
	if code, me, _ := passwordLogin(t, srv, "lt-local", "pw-dora"); code != 200 || int(me["id"].(float64)) != local.userID {
		t.Fatalf("sign-in after link: %d %v", code, me)
	}
	if code, _, _ := passwordLogin(t, srv, "lt-local", "pw-lt-local-123456"); code != 401 {
		t.Fatalf("old local password after link: %d", code)
	}
	resetLoginLimiter()
	// Another WRM account cannot link the same directory account.
	other := newTestUser(t, srv, "lt-other", false)
	if code := other.jsonDo("POST", "/api/me/ldap", map[string]interface{}{"directory": did, "username": "dora", "password": "pw-dora"}, nil); code != 409 {
		t.Fatalf("second link: %d", code)
	}
}

func TestDisableEndsEverything(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "de-admin", true)
	u := newTestUser(t, srv, "de-user", false)
	db.Exec(`INSERT INTO mcp_tokens (user_id, name, token_hash, created_at, expires_at) VALUES (?,?,?,?,?)`, u.userID, "demo", "h-"+randomToken(8), nowRFC(), time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	var out map[string]interface{}
	if code := admin.jsonDo("PUT", fmt.Sprintf("/api/admin/users/%d", u.userID), map[string]interface{}{"disabled": true}, &out); code != 200 {
		t.Fatalf("disable: %d %v", code, out)
	}
	if code := u.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatalf("session of a disabled user: %d", code)
	}
	var open int
	db.QueryRow(`SELECT COUNT(*) FROM mcp_tokens WHERE user_id=? AND revoked_at=''`, u.userID).Scan(&open)
	if open != 0 {
		t.Fatal("AI connections of a disabled user stay valid")
	}
}

// ─── SCIM ────────────────────────────────────────────

type scimTestClient struct {
	t     *testing.T
	srv   *httptest.Server
	token string
}

func (s *scimTestClient) do(method, path string, body interface{}, out interface{}) int {
	s.t.Helper()
	var rd io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		rd = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, s.srv.URL+"/scim/v2"+path, rd)
	req.Header.Set("Content-Type", "application/scim+json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil {
		json.Unmarshal(data, out)
	}
	return resp.StatusCode
}

func TestSCIMProvisioning(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "sc-admin", true)
	newTestUser(t, srv, "sc-local", false)
	var created map[string]interface{}
	if code := admin.jsonDo("POST", "/api/admin/scim", map[string]interface{}{"name": "Demo provisioning",
		"rules": []map[string]interface{}{{"claim": "group", "value": "WRM Operators", "role": "operator"}}}, &created); code != 200 {
		t.Fatalf("create client: %d %v", code, created)
	}
	sc := &scimTestClient{t: t, srv: srv, token: created["token"].(string)}
	if code := (&scimTestClient{t: t, srv: srv, token: "wrm_scim_wrong"}).do("GET", "/Users", nil, nil); code != 401 {
		t.Fatalf("wrong token: %d", code)
	}
	var spc map[string]interface{}
	if code := sc.do("GET", "/ServiceProviderConfig", nil, &spc); code != 200 || spc["patch"] == nil {
		t.Fatalf("service provider config: %d %v", code, spc)
	}
	// Local accounts are invisible to the provisioning client.
	var lst map[string]interface{}
	sc.do("GET", `/Users?filter=userName+eq+"sc-local"`, nil, &lst)
	if lst["totalResults"].(float64) != 0 {
		t.Fatalf("local account visible: %v", lst)
	}
	// Create a user as Entra ID does.
	var u map[string]interface{}
	code := sc.do("POST", "/Users", map[string]interface{}{"schemas": []string{scimSchemaUser}, "userName": "sc-local", "externalId": "00000000-0000-0000-0000-00000000a001",
		"active": true, "displayName": "Ana Demo", "emails": []map[string]interface{}{{"value": "ana@example.com", "type": "work", "primary": true}},
		"name": map[string]string{"givenName": "Ana", "familyName": "Demo"}}, &u)
	if code != 201 || u["id"] == nil || u["userName"] != "sc-local" {
		t.Fatalf("create user: %d %v", code, u)
	}
	uid := u["id"].(string)
	var wrmUser userRow
	var userID int
	db.QueryRow(`SELECT user_id FROM scim_users WHERE scim_id=?`, uid).Scan(&userID)
	if pu, err := loadUser("id=?", userID); err != nil || pu.Username == "sc-local" || pu.Role != userRoleUser {
		t.Fatalf("provisioned account: %v %+v", err, pu)
	} else {
		wrmUser = *pu
	}
	if code := sc.do("POST", "/Users", map[string]interface{}{"userName": "sc-local"}, nil); code != 409 {
		t.Fatalf("duplicate userName: %d", code)
	}
	sc.do("GET", `/Users?filter=userName+eq+"SC-LOCAL"`, nil, &lst)
	if lst["totalResults"].(float64) != 1 {
		t.Fatalf("filter: %v", lst)
	}
	// A group with the user: a team and the operator role.
	var g map[string]interface{}
	if code := sc.do("POST", "/Groups", map[string]interface{}{"displayName": "WRM Operators", "members": []map[string]string{{"value": uid}}}, &g); code != 201 {
		t.Fatalf("create group: %d %v", code, g)
	}
	if pu, _ := loadUser("id=?", userID); pu.Role != userRoleOperator {
		t.Fatalf("role from group: %s", pu.Role)
	}
	var inTeam int
	db.QueryRow(`SELECT COUNT(*) FROM team_members m JOIN teams t ON t.id=m.team_id WHERE m.user_id=? AND t.name='WRM Operators' AND m.source='scim'`, userID).Scan(&inTeam)
	if inTeam != 1 {
		t.Fatal("team membership from group")
	}
	// A group named like a hand-made team gets its own team.
	newTeam(t, admin, "Hand Team")
	sc.do("POST", "/Groups", map[string]interface{}{"displayName": "Hand Team", "members": []map[string]string{{"value": uid}}}, nil)
	var handMember int
	db.QueryRow(`SELECT COUNT(*) FROM team_members m JOIN teams t ON t.id=m.team_id WHERE m.user_id=? AND t.name='Hand Team'`, userID).Scan(&handMember)
	if handMember != 0 {
		t.Fatal("SCIM group took over a hand-made team")
	}
	// Removing the member (Entra's PATCH form) drops the role again.
	gid := g["id"].(string)
	if code := sc.do("PATCH", "/Groups/"+gid, map[string]interface{}{"schemas": []string{scimSchemaPatch}, "Operations": []map[string]interface{}{
		{"op": "Remove", "path": `members[value eq "` + uid + `"]`}}}, nil); code != 200 {
		t.Fatalf("remove member: %d", code)
	}
	if pu, _ := loadUser("id=?", userID); pu.Role != userRoleUser {
		t.Fatalf("role after removal: %s", pu.Role)
	}
	// The account's first directory sign-in links it through externalId = oid.
	f := newFakeIdP(t)
	useFakeIdP(t, f)
	pid := newOIDCProvider(t, admin, f, map[string]interface{}{"rules": []map[string]interface{}{}})
	f.next(map[string]interface{}{"sub": "s-ana", "tid": "t1", "oid": "00000000-0000-0000-0000-00000000A001"})
	b := newBrowser(t, srv, f)
	b.get(fmt.Sprintf("%s/api/auth/oidc/start?provider=%d", srv.URL, pid))
	var me map[string]interface{}
	if code := b.jsonDo("GET", "/api/auth/me", nil, &me); code != 200 || int(me["id"].(float64)) != wrmUser.ID {
		t.Fatalf("OIDC sign-in of a provisioned account: %d %v", code, me)
	}
	// Entra deactivates the user: the account is disabled and its session ends.
	if code := sc.do("PATCH", "/Users/"+uid, map[string]interface{}{"schemas": []string{scimSchemaPatch}, "Operations": []map[string]interface{}{
		{"op": "Replace", "path": "active", "value": "False"}}}, &u); code != 200 || u["active"] != false {
		t.Fatalf("deactivate: %d %v", code, u)
	}
	if code := b.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatalf("session after deactivation: %d", code)
	}
	if pu, _ := loadUser("id=?", userID); !pu.Disabled {
		t.Fatal("account not disabled")
	}
	// Reactivated, then deleted: deleted accounts stay in WRM (disabled) and vanish from SCIM.
	sc.do("PATCH", "/Users/"+uid, map[string]interface{}{"Operations": []map[string]interface{}{{"op": "replace", "value": map[string]interface{}{"active": true, "name.givenName": "Anna"}}}}, &u)
	if pu, _ := loadUser("id=?", userID); pu.Disabled || u["name"].(map[string]interface{})["givenName"] != "Anna" {
		t.Fatalf("reactivate: %+v %v", pu, u)
	}
	if code := sc.do("DELETE", "/Users/"+uid, nil, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code := sc.do("GET", "/Users/"+uid, nil, nil); code != 404 {
		t.Fatalf("deleted user still visible: %d", code)
	}
	if pu, err := loadUser("id=?", userID); err != nil || !pu.Disabled {
		t.Fatal("deleted account should stay, disabled")
	}
	// A revoked client's token stops working.
	if code := admin.jsonDo("DELETE", fmt.Sprintf("/api/admin/scim/%d", int(created["id"].(float64))), nil, nil); code != 200 {
		t.Fatalf("revoke: %d", code)
	}
	if code := sc.do("GET", "/Users", nil, nil); code != 401 {
		t.Fatalf("revoked token: %d", code)
	}
	var adminList map[string]interface{}
	admin.jsonDo("GET", "/api/admin/scim", nil, &adminList)
	if raw, _ := json.Marshal(adminList); strings.Contains(string(raw), sc.token) {
		t.Fatal("the token is listed again")
	}
}

// TestUIDemoDirectory serves WRM with a fake directory for a manual or scripted browser
// test: WRM_UI_DEMO_LDAP=127.0.0.1:8791 go test -run TestUIDemoDirectory -timeout 30m .
// It runs until the file named by WRM_UI_DEMO_DONE exists.
func TestUIDemoDirectory(t *testing.T) {
	addr := os.Getenv("WRM_UI_DEMO_LDAP")
	if addr == "" {
		t.Skip("set WRM_UI_DEMO_LDAP to run the browser demo")
	}
	keepRoleSettings(t)
	keepDirectorySettings(t)
	f := newFakeLDAP(t, true)
	ops := f.group("WRM Operators")
	f.user("jdoe", "pw-jdoe", f.group("Helpdesk", ops))
	f.user("outsider", "pw-out")
	h, _ := hashPassword("demo-admin-pass")
	res, _ := db.Exec(`INSERT INTO users (username, password_hash, is_admin, role, created_at) VALUES ('demo-admin', ?, 1, 'admin', ?)`, h, nowRFC())
	aid, _ := res.LastInsertId()
	srv := httptest.NewUnstartedServer(newRouter())
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	srv.Listener = ln
	srv.Start()
	defer srv.Close()
	rec := httptest.NewRecorder()
	createAuthSession(rec, httptest.NewRequest("GET", "/", nil), int(aid), "")
	var ck *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			ck = c
		}
	}
	admin := &testClient{t: t, srv: srv, cookie: ck, userID: int(aid)}
	ensureTeam(t, admin, "LDAP Ops")
	newLDAPDirectory(t, admin, f, map[string]interface{}{"name": "Example AD"})
	os.WriteFile(os.Getenv("WRM_UI_DEMO_DONE")+".ca.pem", []byte(f.caPEM), 0o600)
	for {
		if _, err := os.Stat(os.Getenv("WRM_UI_DEMO_DONE")); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}
