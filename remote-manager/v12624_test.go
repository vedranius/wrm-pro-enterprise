// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// ─── a fake OpenID Connect provider ──────────────────

type fakeIdP struct {
	t      *testing.T
	srv    *httptest.Server
	rsa    *rsa.PrivateKey
	ec     *ecdsa.PrivateKey
	kid    string
	useEC  bool
	mu     sync.Mutex
	claims map[string]interface{} // the user who signs in next
	codes  map[string]fakeCode
	// refresh tokens: token → claims for the new ID token ("" key when refused)
	refresh   map[string]map[string]interface{}
	refused   map[string]bool
	authCount int
	lastAuth  url.Values
	secret    string
	noRefresh bool
	audience  string
}

type fakeCode struct {
	challenge, redirect, nonce string
	claims                     map[string]interface{}
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	f := &fakeIdP{t: t, kid: "k1", codes: map[string]fakeCode{}, refresh: map[string]map[string]interface{}{}, refused: map[string]bool{}, secret: "demo-client-secret"}
	f.rsa, _ = rsa.GenerateKey(rand.Reader, 2048)
	f.ec, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	mux := http.NewServeMux()
	f.srv = httptest.NewTLSServer(mux)
	t.Cleanup(f.srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/authorize", "token_endpoint": f.srv.URL + "/token",
			"jwks_uri": f.srv.URL + "/jwks", "code_challenge_methods_supported": []string{"S256"},
			"scopes_supported": []string{"openid", "profile", "email", "offline_access"}, "token_endpoint_auth_methods_supported": []string{"client_secret_post"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		b := func(x []byte) string { return base64.RawURLEncoding.EncodeToString(x) }
		pad := func(x *big.Int) []byte {
			out := make([]byte, 32)
			xb := x.Bytes()
			copy(out[32-len(xb):], xb)
			return out
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"keys": []map[string]string{
			{"kty": "RSA", "kid": f.kid, "use": "sig", "alg": "RS256", "n": b(f.rsa.N.Bytes()), "e": b(big.NewInt(int64(f.rsa.E)).Bytes())},
			{"kty": "EC", "kid": "ec1", "use": "sig", "alg": "ES256", "crv": "P-256", "x": b(pad(f.ec.X)), "y": b(pad(f.ec.Y))},
		}})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.mu.Lock()
		f.authCount++
		f.lastAuth = q
		code := randomToken(16)
		f.codes[code] = fakeCode{challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"), nonce: q.Get("nonce"), claims: f.claims}
		f.mu.Unlock()
		http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		fail := func(e string) {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": e})
		}
		if r.Form.Get("client_secret") != f.secret || r.Form.Get("client_id") != "wrm-demo" {
			fail("invalid_client")
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		var claims map[string]interface{}
		nonce := ""
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			c, ok := f.codes[r.Form.Get("code")]
			delete(f.codes, r.Form.Get("code"))
			if !ok || c.redirect != r.Form.Get("redirect_uri") || pkceS256(r.Form.Get("code_verifier")) != c.challenge {
				fail("invalid_grant")
				return
			}
			claims, nonce = c.claims, c.nonce
		case "refresh_token":
			rt := r.Form.Get("refresh_token")
			if f.refused[rt] || f.refresh[rt] == nil {
				fail("invalid_grant")
				return
			}
			claims = f.refresh[rt]
		default:
			fail("unsupported_grant_type")
			return
		}
		out := map[string]interface{}{"access_token": "at-" + randomToken(8), "token_type": "Bearer", "id_token": f.sign(claims, nonce)}
		if !f.noRefresh {
			rt := "rt-" + randomToken(12)
			f.refresh[rt] = claims
			out["refresh_token"] = rt
		}
		json.NewEncoder(w).Encode(out)
	})
	return f
}

func (f *fakeIdP) sign(claims map[string]interface{}, nonce string) string {
	c := map[string]interface{}{"iss": f.srv.URL, "aud": "wrm-demo", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "auth_time": time.Now().Unix()}
	if f.audience != "" {
		c["aud"] = f.audience
	}
	for k, v := range claims {
		c[k] = v
	}
	if nonce != "" {
		c["nonce"] = nonce
	}
	return f.signRaw(c)
}

func (f *fakeIdP) signRaw(c map[string]interface{}) string {
	alg, kid := "RS256", f.kid
	if f.useEC {
		alg, kid = "ES256", "ec1"
	}
	h, _ := json.Marshal(map[string]string{"alg": alg, "kid": kid, "typ": "JWT"})
	p, _ := json.Marshal(c)
	signed := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	sum := sha256.Sum256([]byte(signed))
	var sig []byte
	if f.useEC {
		r, s, _ := ecdsa.Sign(rand.Reader, f.ec, sum[:])
		sig = make([]byte, 64)
		r.FillBytes(sig[:32])
		s.FillBytes(sig[32:])
	} else {
		sig, _ = rsa.SignPKCS1v15(rand.Reader, f.rsa, crypto.SHA256, sum[:])
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (f *fakeIdP) next(claims map[string]interface{}) {
	f.mu.Lock()
	f.claims = claims
	f.mu.Unlock()
}

// useFakeIdP points WRM's provider client at the fake and resets the caches.
func useFakeIdP(t *testing.T, f *fakeIdP) {
	t.Helper()
	old := oidcHTTP
	oidcHTTP = f.srv.Client()
	oidcHTTP.Timeout = 10 * time.Second
	clearOIDCCache := func() {
		oidcCache.Lock()
		oidcCache.disc = map[string]oidcCachedDisc{}
		oidcCache.keys = map[string]oidcCachedKeys{}
		oidcCache.lastPull = map[string]time.Time{}
		oidcCache.Unlock()
	}
	clearOIDCCache()
	saved := map[string]string{}
	for _, k := range []string{"local_login", "oidc_revalidate_minutes", "oidc_unverified_hours", "require_step_up", "require_2fa"} {
		saved[k] = getSetting(k)
	}
	t.Cleanup(func() {
		oidcHTTP = old
		clearOIDCCache()
		for k, v := range saved {
			setSetting(k, v)
		}
		db.Exec(`DELETE FROM user_identities`)
		db.Exec(`DELETE FROM oidc_providers`)
	})
}

// browser follows redirects across WRM and the fake provider and keeps cookies.
type browser struct {
	t   *testing.T
	jar *cookiejar.Jar
	c   *http.Client
	wrm string
}

func newBrowser(t *testing.T, wrm *httptest.Server, f *fakeIdP) *browser {
	jar, _ := cookiejar.New(nil)
	tr := f.srv.Client().Transport
	b := &browser{t: t, jar: jar, wrm: wrm.URL}
	b.c = &http.Client{Jar: jar, Transport: tr, Timeout: 20 * time.Second}
	return b
}

// get follows redirects and returns the final status, URL (fragment included) and body.
func (b *browser) get(raw string) (int, string, string) {
	b.t.Helper()
	cur := raw
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest("GET", cur, nil)
		resp, err := (&http.Client{Jar: b.jar, Transport: b.c.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
		if err != nil {
			b.t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 302 && resp.StatusCode != 303 {
			return resp.StatusCode, cur, string(body)
		}
		loc, _ := resp.Location()
		cur = loc.String()
		if strings.Contains(cur, "#") {
			return resp.StatusCode, cur, ""
		}
	}
	b.t.Fatal("too many redirects")
	return 0, "", ""
}

func (b *browser) jsonDo(method, path string, body interface{}, out interface{}) int {
	b.t.Helper()
	var rd io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		rd = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, b.wrm+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", b.wrm)
	req.Header.Set("X-WRM-Request", "1")
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil {
		json.Unmarshal(data, out)
	}
	return resp.StatusCode
}

func (b *browser) setSession(c *testClient) {
	u, _ := url.Parse(b.wrm)
	b.jar.SetCookies(u, []*http.Cookie{{Name: c.cookie.Name, Value: c.cookie.Value, Path: "/"}})
}

// ssoTeam makes sure the team the default mapping rules name exists.
func ssoTeam(t *testing.T, admin *testClient) {
	t.Helper()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM teams WHERE name='SSO Ops'`).Scan(&n)
	if n == 0 {
		newTeam(t, admin, "SSO Ops")
	}
}

func newOIDCProvider(t *testing.T, admin *testClient, f *fakeIdP, extra map[string]interface{}) int {
	t.Helper()
	body := map[string]interface{}{"name": "Demo SSO", "issuer": f.srv.URL, "client_id": "wrm-demo", "client_secret": f.secret, "enabled": true, "jit": true,
		"rules": []map[string]interface{}{{"claim": "roles", "value": "WRM.Admin", "role": "admin"}, {"claim": "groups", "value": "g-ops", "role": "operator", "teams": []string{"SSO Ops"}}}}
	for k, v := range extra {
		body[k] = v
	}
	var out map[string]interface{}
	if code := admin.jsonDo("POST", "/api/admin/oidc", body, &out); code != 200 {
		t.Fatalf("create provider: %d %v", code, out)
	}
	return int(out["id"].(float64))
}

// ─── tests ───────────────────────────────────────────

func TestOIDCIDTokenVerification(t *testing.T) {
	f := newFakeIdP(t)
	useFakeIdP(t, f)
	d, err := oidcDiscover(f.srv.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	good := map[string]interface{}{"sub": "u1"}
	if _, err := oidcVerifyIDToken(f.sign(good, "n1"), d, "wrm-demo", "n1"); err != nil {
		t.Fatalf("RS256: %v", err)
	}
	f.useEC = true
	if _, err := oidcVerifyIDToken(f.sign(good, "n1"), d, "wrm-demo", "n1"); err != nil {
		t.Fatalf("ES256: %v", err)
	}
	f.useEC = false
	bad := func(name, tok, nonce string) {
		t.Helper()
		if _, err := oidcVerifyIDToken(tok, d, "wrm-demo", nonce); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	bad("wrong nonce", f.sign(good, "n1"), "n2")
	f.audience = "someone-else"
	bad("wrong audience", f.sign(good, ""), "")
	f.audience = ""
	bad("expired", f.signRaw(map[string]interface{}{"iss": f.srv.URL, "aud": "wrm-demo", "sub": "u1", "exp": time.Now().Add(-time.Hour).Unix()}), "")
	bad("other issuer", f.signRaw(map[string]interface{}{"iss": "https://evil.example.com", "aud": "wrm-demo", "sub": "u1", "exp": time.Now().Add(time.Hour).Unix()}), "")
	bad("no subject", f.signRaw(map[string]interface{}{"iss": f.srv.URL, "aud": "wrm-demo", "exp": time.Now().Add(time.Hour).Unix()}), "")
	tok := f.sign(good, "")
	parts := strings.Split(tok, ".")
	for _, alg := range []string{"none", "HS256"} {
		h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"` + alg + `","kid":"k1"}`))
		bad("alg "+alg, h+"."+parts[1]+"."+parts[2], "")
	}
	bad("tampered payload", parts[0]+"."+base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"`+f.srv.URL+`","aud":"wrm-demo","sub":"admin","exp":9999999999}`))+"."+parts[2], "")
	f.kid = "rotated"
	bad("unknown key", f.sign(good, ""), "")
	// After a key rotation the provider publishes the new key and WRM pulls it again.
	oidcCache.Lock()
	oidcCache.lastPull = map[string]time.Time{}
	oidcCache.Unlock()
	if _, err := oidcVerifyIDToken(f.sign(good, ""), d, "wrm-demo", ""); err != nil {
		t.Fatalf("rotated key: %v", err)
	}
	if oidcSubject(oidcClaims{"iss": "x", "sub": "s", "tid": "T", "oid": "O"}) != "entra:T:O" || oidcSubject(oidcClaims{"iss": "x", "sub": "s"}) != "x|s" {
		t.Fatal("oidcSubject")
	}
	if err := oidcURLAllowed("http://idp.example.com"); err == nil {
		t.Fatal("plain HTTP issuer accepted")
	}
}

func TestOIDCSignInAndMapping(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	f := newFakeIdP(t)
	useFakeIdP(t, f)
	admin := newTestUser(t, srv, "oidc-admin", true)
	ssoTeam(t, admin)
	pid := newOIDCProvider(t, admin, f, nil)

	var cfg map[string]interface{}
	newBrowser(t, srv, f).jsonDo("GET", "/api/auth/config", nil, &cfg)
	if sso, _ := cfg["sso"].([]interface{}); len(sso) != 1 {
		t.Fatalf("sign-in buttons: %v", cfg["sso"])
	}

	// A directory user with the WRM.Admin app role gets an administrator account on first sign-in.
	f.next(map[string]interface{}{"sub": "pairwise-1", "tid": "tenant-1", "oid": "object-1", "preferred_username": "ana@example.com", "name": "Ana Demo", "roles": []string{"WRM.Admin"}})
	b := newBrowser(t, srv, f)
	code, final, body := b.get(fmt.Sprintf("%s/api/auth/oidc/start?provider=%d", srv.URL, pid))
	if code != 200 || strings.TrimRight(final, "/") != srv.URL {
		t.Fatalf("sign-in ended at %d %s: %.300s", code, final, body)
	}
	if f.lastAuth.Get("code_challenge_method") != "S256" || f.lastAuth.Get("nonce") == "" || !strings.Contains(f.lastAuth.Get("scope"), "offline_access") {
		t.Fatalf("authorization request: %v", f.lastAuth)
	}
	var me map[string]interface{}
	if code := b.jsonDo("GET", "/api/auth/me", nil, &me); code != 200 || me["username"] != "ana@example.com" || me["role"] != "admin" || me["sso_only"] != true {
		t.Fatalf("me after sign-in: %d %v", code, me)
	}
	uid := int(me["id"].(float64))
	var subject string
	db.QueryRow(`SELECT subject FROM user_identities WHERE user_id=?`, uid).Scan(&subject)
	if subject != "entra:tenant-1:object-1" {
		t.Fatalf("identity key: %q", subject)
	}
	var ident int
	db.QueryRow(`SELECT COUNT(*) FROM auth_sessions WHERE user_id=? AND identity_id>0`, uid).Scan(&ident)
	if ident != 1 {
		t.Fatal("the session is not tied to the directory identity")
	}

	// Same person, now only in the ops group: the role follows the directory, the team is added.
	f.next(map[string]interface{}{"sub": "pairwise-1", "tid": "tenant-1", "oid": "object-1", "preferred_username": "ana.renamed@example.com", "groups": []string{"g-ops"}})
	b2 := newBrowser(t, srv, f)
	b2.get(fmt.Sprintf("%s/api/auth/oidc/start?provider=%d", srv.URL, pid))
	b2.jsonDo("GET", "/api/auth/me", nil, &me)
	if int(me["id"].(float64)) != uid || me["role"] != "operator" {
		t.Fatalf("second sign-in (renamed user must stay the same account): %v", me)
	}
	var src string
	db.QueryRow(`SELECT tm.source FROM team_members tm JOIN teams t ON t.id=tm.team_id WHERE t.name='SSO Ops' AND tm.user_id=?`, uid).Scan(&src)
	if src != "oidc" {
		t.Fatalf("team membership from the directory: %q", src)
	}

	// Nobody else gets in: no matching role or group means no account, and the refusal says why.
	f.next(map[string]interface{}{"sub": "stranger", "preferred_username": "eve@example.com", "groups": []string{"g-sales"}})
	code, _, body = newBrowser(t, srv, f).get(fmt.Sprintf("%s/api/auth/oidc/start?provider=%d", srv.URL, pid))
	if code != 403 || !strings.Contains(body, "not assigned to WRM") {
		t.Fatalf("unmapped user: %d %.200s", code, body)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE username LIKE 'eve%'`).Scan(&n)
	if n != 0 {
		t.Fatal("an account was created for an unmapped user")
	}
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='auth.oidc_refused'`).Scan(&n)
	if n == 0 {
		t.Fatal("the refusal was not audited")
	}

	// Removing the user from every WRM group ends their sessions at the next sign-in attempt.
	f.next(map[string]interface{}{"sub": "pairwise-1", "tid": "tenant-1", "oid": "object-1", "groups": []string{}})
	newBrowser(t, srv, f).get(fmt.Sprintf("%s/api/auth/oidc/start?provider=%d", srv.URL, pid))
	if code := b2.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatalf("session kept after losing the WRM group: %d", code)
	}

	// The callback only works in the browser that started the sign-in, and only once.
	f.next(map[string]interface{}{"sub": "pairwise-2", "groups": []string{"g-ops"}})
	b3 := newBrowser(t, srv, f)
	start := fmt.Sprintf("%s/api/auth/oidc/start?provider=%d", srv.URL, pid)
	req, _ := http.NewRequest("GET", start, nil)
	noFollow := &http.Client{Jar: b3.jar, Transport: b3.c.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, _ := noFollow.Do(req)
	resp.Body.Close()
	authz, _ := resp.Location()
	resp, _ = noFollow.Get(authz.String())
	resp.Body.Close()
	cb, _ := resp.Location()
	if code, _, body := newBrowser(t, srv, f).get(cb.String()); code != 400 || !strings.Contains(body, "not started in this browser") {
		t.Fatalf("callback in another browser: %d %.200s", code, body)
	}
	if code, _, _ := b3.get(cb.String()); code != 400 {
		t.Fatalf("callback replayed: %d", code)
	}

	// MFA proven by the token.
	if code := admin.jsonDo("PUT", fmt.Sprintf("/api/admin/oidc/%d", pid), map[string]interface{}{"require_mfa": true}, nil); code != 200 {
		t.Fatal("require MFA")
	}
	f.next(map[string]interface{}{"sub": "mfa-user", "groups": []string{"g-ops"}, "amr": []string{"pwd"}})
	if code, _, body := newBrowser(t, srv, f).get(start); code != 403 || !strings.Contains(body, "MFA") {
		t.Fatalf("sign-in without MFA: %d %.200s", code, body)
	}
	f.next(map[string]interface{}{"sub": "mfa-user", "groups": []string{"g-ops"}, "amr": []string{"pwd", "mfa"}})
	bm := newBrowser(t, srv, f)
	bm.get(start)
	if code := bm.jsonDo("GET", "/api/auth/me", nil, nil); code != 200 {
		t.Fatalf("sign-in with MFA: %d", code)
	}

	// The client secret never reaches the browser.
	resp = admin.do("GET", "/api/admin/oidc", nil, "")
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(raw), f.secret) || !strings.Contains(string(raw), `"has_secret":true`) {
		t.Fatalf("provider list leaks or lost the secret: %.300s", raw)
	}
}

func TestOIDCLocalTOTPAfterDirectorySignIn(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	f := newFakeIdP(t)
	useFakeIdP(t, f)
	admin := newTestUser(t, srv, "oidc-totp-admin", true)
	ssoTeam(t, admin)
	pid := newOIDCProvider(t, admin, f, nil)
	f.next(map[string]interface{}{"sub": "totp-user", "preferred_username": "totp.user", "groups": []string{"g-ops"}})
	b := newBrowser(t, srv, f)
	start := fmt.Sprintf("%s/api/auth/oidc/start?provider=%d", srv.URL, pid)
	b.get(start)
	var uid int
	db.QueryRow(`SELECT id FROM users WHERE username='totp.user'`).Scan(&uid)
	secret := []byte("12345678901234567890")
	db.Exec(`UPDATE users SET totp_enabled=1, totp_secret=?, totp_last_step=0 WHERE id=?`, encryptValue(strings.TrimRight(base32.StdEncoding.EncodeToString(secret), "=")), uid)
	// The provider does not prove MFA, so the account's own 2FA is asked for.
	b = newBrowser(t, srv, f)
	_, final, _ := b.get(start)
	i := strings.Index(final, "#sso-mfa=")
	if i < 0 {
		t.Fatalf("no 2FA step: %s", final)
	}
	tok, _ := url.QueryUnescape(final[i+len("#sso-mfa="):])
	if code := b.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatal("signed in before the 2FA code")
	}
	codeNow := totpCodeAt(secret, time.Now().Unix()/30)
	var out map[string]interface{}
	if code := b.jsonDo("POST", "/api/auth/mfa", map[string]string{"mfa_token": tok, "code": codeNow}, &out); code != 200 {
		t.Fatalf("2FA step: %d %v", code, out)
	}
	var ident int
	db.QueryRow(`SELECT COUNT(*) FROM auth_sessions WHERE user_id=? AND identity_id>0`, uid).Scan(&ident)
	if ident == 0 {
		t.Fatal("the session after the 2FA step is not tied to the directory identity")
	}
}

func TestOIDCRevalidation(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	f := newFakeIdP(t)
	useFakeIdP(t, f)
	admin := newTestUser(t, srv, "oidc-reval-admin", true)
	ssoTeam(t, admin)
	pid := newOIDCProvider(t, admin, f, nil)
	start := fmt.Sprintf("%s/api/auth/oidc/start?provider=%d", srv.URL, pid)
	signIn := func(sub string) (*browser, int) {
		f.next(map[string]interface{}{"sub": sub, "preferred_username": sub, "groups": []string{"g-ops"}})
		b := newBrowser(t, srv, f)
		b.get(start)
		var me map[string]interface{}
		if code := b.jsonDo("GET", "/api/auth/me", nil, &me); code != 200 {
			t.Fatalf("%s sign-in: %d", sub, code)
		}
		return b, int(me["id"].(float64))
	}
	due := func(uid int) {
		db.Exec(`UPDATE user_identities SET last_check_at='2000-01-01T00:00:00Z' WHERE user_id=?`, uid)
	}

	// Still allowed: the refresh works and the check time moves on.
	ok, okID := signIn("still-ok")
	due(okID)
	oidcRevalidateOnce()
	if code := ok.jsonDo("GET", "/api/auth/me", nil, nil); code != 200 {
		t.Fatal("a valid directory user was signed out")
	}
	var lastCheck string
	db.QueryRow(`SELECT last_check_at FROM user_identities WHERE user_id=?`, okID).Scan(&lastCheck)
	if strings.HasPrefix(lastCheck, "2000") {
		t.Fatal("the check was not recorded")
	}

	// Disabled in the directory: the provider refuses the refresh token, WRM ends the sessions.
	gone, goneID := signIn("disabled-in-directory")
	var enc string
	db.QueryRow(`SELECT refresh_token FROM user_identities WHERE user_id=?`, goneID).Scan(&enc)
	f.mu.Lock()
	f.refused[decryptValue(enc)] = true
	f.mu.Unlock()
	due(goneID)
	oidcRevalidateOnce()
	if code := gone.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatalf("a user disabled in the directory kept the session: %d", code)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='oidc.access_ended' AND target='disabled-in-directory'`).Scan(&n)
	if n == 0 {
		t.Fatal("ending the access was not audited")
	}

	// Removed from the WRM group: the refreshed ID token no longer matches a rule.
	moved, movedID := signIn("moved-out")
	db.QueryRow(`SELECT refresh_token FROM user_identities WHERE user_id=?`, movedID).Scan(&enc)
	f.mu.Lock()
	f.refresh[decryptValue(enc)] = map[string]interface{}{"sub": "moved-out", "groups": []string{"g-sales"}}
	f.mu.Unlock()
	due(movedID)
	oidcRevalidateOnce()
	if code := moved.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatalf("a user removed from the WRM group kept the session: %d", code)
	}

	// Without a refresh token the session lasts until oidc_unverified_hours.
	f.noRefresh = true
	nr, nrID := signIn("no-refresh")
	due(nrID)
	oidcRevalidateOnce()
	if code := nr.jsonDo("GET", "/api/auth/me", nil, nil); code != 200 {
		t.Fatal("ended before oidc_unverified_hours")
	}
	db.Exec(`UPDATE user_identities SET last_ok_at='2000-01-01T00:00:00Z' WHERE user_id=?`, nrID)
	due(nrID)
	oidcRevalidateOnce()
	if code := nr.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatal("an unverified directory session outlived oidc_unverified_hours")
	}
}

func TestOIDCLocalLoginLimited(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	f := newFakeIdP(t)
	useFakeIdP(t, f)
	admin := newTestUser(t, srv, "ll-admin", true)
	newTestUser(t, srv, "ll-user", false)
	bg := newTestUser(t, srv, "ll-bg", true)
	db.Exec(`UPDATE users SET break_glass=1 WHERE id=?`, bg.userID)
	login := func(name string) int {
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"`+name+`","password":"pw-`+name+`-123456"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-WRM-Request", "1")
		req.RemoteAddr = "198.51.100.24:4000"
		rec := httptest.NewRecorder()
		newRouter().ServeHTTP(rec, req)
		return rec.Code
	}
	setSetting("local_login", "break_glass")
	// No provider yet: the limit is not in effect, so nobody is locked out.
	if code := login("ll-user"); code != 200 {
		t.Fatalf("password sign-in without a provider: %d", code)
	}
	ssoTeam(t, admin)
	newOIDCProvider(t, admin, f, nil)
	if code := login("ll-user"); code != 403 {
		t.Fatalf("password sign-in of a normal user with local_login=break_glass: %d", code)
	}
	setSetting("registration", "open")
	defer setSetting("registration", "closed")
	reg := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(`{"username":"ll-new","password":"a-long-password-123"}`))
	reg.Header.Set("Content-Type", "application/json")
	reg.Header.Set("X-WRM-Request", "1")
	reg.RemoteAddr = "198.51.100.25:4000"
	rec := httptest.NewRecorder()
	newRouter().ServeHTTP(rec, reg)
	if rec.Code != 403 {
		t.Fatalf("self-registration while password sign-in is limited: %d", rec.Code)
	}
	if code := login("ll-bg"); code != 200 {
		t.Fatalf("break-glass password sign-in: %d", code)
	}
	var cfg map[string]interface{}
	newBrowser(t, srv, f).jsonDo("GET", "/api/auth/config", nil, &cfg)
	if cfg["local_login_limited"] != true {
		t.Fatal("the sign-in page does not know password sign-in is limited")
	}
	if levelNeed("enterprise", "local_login") != "break_glass" || levelNeed("business", "local_login") != "" {
		t.Fatal("level rule for local_login")
	}
}

func TestOIDCLinkStepUpAndTest(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	f := newFakeIdP(t)
	useFakeIdP(t, f)
	admin := newTestUser(t, srv, "lk-admin", true)
	other := newTestUser(t, srv, "lk-other", false)
	ssoTeam(t, admin)
	pid := newOIDCProvider(t, admin, f, map[string]interface{}{"rules": []map[string]interface{}{}})

	popup := func(c *testClient, mode string, claims map[string]interface{}) map[string]interface{} {
		t.Helper()
		b := newBrowser(t, srv, f)
		b.setSession(c)
		var st map[string]interface{}
		if code := b.jsonDo("POST", "/api/auth/oidc/start", map[string]interface{}{"provider": pid, "mode": mode}, &st); code != 200 {
			t.Fatalf("start %s: %d %v", mode, code, st)
		}
		f.next(claims)
		if code, _, body := b.get(st["url"].(string)); code != 200 && code != 403 {
			t.Fatalf("%s window: %d %.200s", mode, code, body)
		}
		var res map[string]interface{}
		b.jsonDo("GET", "/api/auth/oidc/result?state="+url.QueryEscape(st["state"].(string)), nil, &res)
		return res
	}

	// The administrator's test sign-in shows the claims and what WRM would do, and creates nothing.
	res := popup(admin, "test", map[string]interface{}{"sub": "tester", "preferred_username": "tester", "roles": []string{"WRM.Admin"}})
	if res["ok"] != true || res["claims"] == nil || res["subject"] == nil {
		t.Fatalf("test sign-in: %v", res)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE username='tester'`).Scan(&n)
	if n != 0 {
		t.Fatal("the test sign-in created an account")
	}
	if code := other.jsonDo("POST", "/api/auth/oidc/start", map[string]interface{}{"provider": pid, "mode": "test"}, nil); code != 403 {
		t.Fatalf("test sign-in by a non-admin: %d", code)
	}

	// Linking adds a way to sign in, so it needs a fresh confirmation even with the policy off.
	setSetting("require_step_up", "0")
	db.Exec(`UPDATE auth_sessions SET step_up_at='2000-01-01T00:00:00Z' WHERE user_id=?`, other.userID)
	var su map[string]interface{}
	if code := other.jsonDo("POST", "/api/auth/oidc/start", map[string]interface{}{"provider": pid, "mode": "link"}, &su); code != 403 || su["step_up"] != true {
		t.Fatalf("link with a stale session: %d %v", code, su)
	}
	db.Exec(`UPDATE auth_sessions SET step_up_at=? WHERE user_id=?`, nowRFC(), other.userID)

	// A local account links its directory account and can then sign in with it.
	if res := popup(other, "link", map[string]interface{}{"sub": "other-dir"}); res["ok"] != true {
		t.Fatalf("link: %v", res)
	}
	f.next(map[string]interface{}{"sub": "other-dir"})
	b := newBrowser(t, srv, f)
	b.get(fmt.Sprintf("%s/api/auth/oidc/start?provider=%d", srv.URL, pid))
	var me map[string]interface{}
	if code := b.jsonDo("GET", "/api/auth/me", nil, &me); code != 200 || int(me["id"].(float64)) != other.userID {
		t.Fatalf("sign-in with the linked account: %d %v", code, me)
	}
	// An existing link is never silently replaced by another directory account.
	if code := other.jsonDo("POST", "/api/auth/oidc/start", map[string]interface{}{"provider": pid, "mode": "link"}, nil); code != 400 {
		t.Fatalf("second link for the same account: %d", code)
	}
	// The same directory account cannot be linked to a second WRM account.
	if res := popup(admin, "link", map[string]interface{}{"sub": "other-dir"}); res["ok"] != false {
		t.Fatalf("second link of the same directory account: %v", res)
	}
	// Without rules, directory users with no WRM account are refused (no automatic accounts).
	f.next(map[string]interface{}{"sub": "unknown"})
	if code, _, _ := newBrowser(t, srv, f).get(fmt.Sprintf("%s/api/auth/oidc/start?provider=%d", srv.URL, pid)); code != 403 {
		t.Fatalf("unlinked user without rules: %d", code)
	}

	// Step-up through the directory, for accounts that sign in that way.
	setSetting("require_step_up", "1")
	db.Exec(`UPDATE auth_sessions SET step_up_at='2000-01-01T00:00:00Z' WHERE user_id=?`, other.userID)
	if res := popup(other, "stepup", map[string]interface{}{"sub": "someone-else"}); res["ok"] != false {
		t.Fatalf("step-up with a different directory account: %v", res)
	}
	if res := popup(other, "stepup", map[string]interface{}{"sub": "other-dir"}); res["ok"] != true {
		t.Fatalf("step-up: %v", res)
	}
	if f.lastAuth.Get("prompt") != "login" {
		t.Fatalf("step-up must ask the provider to sign in again: %v", f.lastAuth)
	}
	var fresh string
	db.QueryRow(`SELECT step_up_at FROM auth_sessions WHERE user_id=? ORDER BY step_up_at DESC LIMIT 1`, other.userID).Scan(&fresh)
	if strings.HasPrefix(fresh, "2000") {
		t.Fatal("step-up was not recorded")
	}

	// Unlinking ends the directory sessions.
	var ids struct {
		Identities []map[string]interface{} `json:"identities"`
	}
	other.jsonDo("GET", "/api/me/identities", nil, &ids)
	if len(ids.Identities) != 1 {
		t.Fatalf("identities: %v", ids)
	}
	if code := other.jsonDo("DELETE", fmt.Sprintf("/api/me/identities/%d", int(ids.Identities[0]["id"].(float64))), nil, nil); code != 200 {
		t.Fatal("unlink")
	}
	if code := b.jsonDo("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatal("directory session kept after unlinking")
	}
}

func TestOIDCAdminAPI(t *testing.T) {
	keepRoleSettings(t)
	srv := newTestServer(t)
	f := newFakeIdP(t)
	useFakeIdP(t, f)
	admin := newTestUser(t, srv, "oa-admin", true)
	user := newTestUser(t, srv, "oa-user", false)
	ssoTeam(t, admin)
	if code := user.jsonDo("GET", "/api/admin/oidc", nil, nil); code != 403 {
		t.Fatalf("non-admin reads providers: %d", code)
	}
	var chk struct {
		OK    bool            `json:"ok"`
		Steps []oidcCheckStep `json:"steps"`
	}
	admin.jsonDo("POST", "/api/admin/oidc/check", map[string]string{"issuer": f.srv.URL}, &chk)
	if !chk.OK || len(chk.Steps) < 5 {
		t.Fatalf("connection test: %+v", chk)
	}
	admin.jsonDo("POST", "/api/admin/oidc/check", map[string]string{"issuer": "https://login.microsoftonline.com/common/v2.0"}, &chk)
	if chk.OK || chk.Steps[0].Fix == "" {
		t.Fatalf("multi-tenant issuer: %+v", chk)
	}
	if code := admin.jsonDo("POST", "/api/admin/oidc", map[string]interface{}{"name": "Bad", "issuer": "http://idp.example.com", "client_id": "x"}, nil); code != 400 {
		t.Fatalf("plain HTTP issuer: %d", code)
	}
	if code := admin.jsonDo("POST", "/api/admin/oidc", map[string]interface{}{"name": "Bad team", "issuer": f.srv.URL, "client_id": "x",
		"rules": []map[string]interface{}{{"claim": "groups", "value": "g1", "role": "user", "teams": []string{"No such team"}}}}, nil); code != 400 {
		t.Fatalf("rule naming an unknown team: %d", code)
	}
	pid := newOIDCProvider(t, admin, f, nil)
	var pv struct {
		Mapping oidcMapResult `json:"mapping"`
	}
	admin.jsonDo("POST", fmt.Sprintf("/api/admin/oidc/%d/preview", pid), map[string]interface{}{"claims": map[string]interface{}{"roles": []string{"wrm.admin"}, "groups": "g-ops"}}, &pv)
	if !pv.Mapping.Matched || pv.Mapping.Role != "admin" || len(pv.Mapping.Teams) != 1 {
		t.Fatalf("preview (strongest role wins, case-insensitive values): %+v", pv.Mapping)
	}
	admin.jsonDo("POST", fmt.Sprintf("/api/admin/oidc/%d/preview", pid), map[string]interface{}{"claims": map[string]interface{}{"groups": []string{"g-sales"}}}, &pv)
	if pv.Mapping.Matched {
		t.Fatal("preview matched an unmapped group")
	}
	// Changing a provider without a new secret keeps the stored one.
	if code := admin.jsonDo("PUT", fmt.Sprintf("/api/admin/oidc/%d", pid), map[string]interface{}{"name": "Demo SSO 2", "client_secret": ""}, nil); code != 200 {
		t.Fatal("rename")
	}
	if p, _ := loadOIDCProvider(pid); p.secret != f.secret || p.Name != "Demo SSO 2" {
		t.Fatalf("secret lost on update: %+v", p)
	}
	var stored string
	db.QueryRow(`SELECT client_secret FROM oidc_providers WHERE id=?`, pid).Scan(&stored)
	if strings.Contains(stored, f.secret) {
		t.Fatal("client secret stored in plain text")
	}
	if code := admin.jsonDo("DELETE", fmt.Sprintf("/api/admin/oidc/%d", pid), nil, nil); code != 200 {
		t.Fatal("delete provider")
	}
	if oidcAnyEnabled() {
		t.Fatal("provider still enabled after delete")
	}
}
