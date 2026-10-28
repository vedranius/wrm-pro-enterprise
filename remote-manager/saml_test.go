// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"html"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlidp"
)

func samlBackupSettings(t *testing.T) {
	t.Helper()
	keys := []string{"saml_enabled", "saml_idp_metadata", "saml_entity_id", "saml_attr_username", "saml_attr_groups"}
	vals := map[string]string{}
	for _, k := range keys {
		vals[k] = getSetting(k)
	}
	t.Cleanup(func() {
		for k, v := range vals {
			setSetting(k, v)
		}
	})
}

func newSAMLTestCert(t *testing.T, cn string) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return key, cert
}

// inlineIdpMetadata builds a minimal but valid IdP EntityDescriptor as XML, suitable for
// the saml_idp_metadata setting.
func inlineIdpMetadata(t *testing.T, cert *x509.Certificate) string {
	t.Helper()
	ed := saml.EntityDescriptor{
		EntityID: "urn:test:idp",
		IDPSSODescriptors: []saml.IDPSSODescriptor{{
			SSODescriptor: saml.SSODescriptor{
				RoleDescriptor: saml.RoleDescriptor{
					ProtocolSupportEnumeration: "urn:oasis:names:tc:SAML:2.0:protocol",
					KeyDescriptors: []saml.KeyDescriptor{{
						Use: "signing",
						KeyInfo: saml.KeyInfo{X509Data: saml.X509Data{X509Certificates: []saml.X509Certificate{{
							Data: base64.StdEncoding.EncodeToString(cert.Raw),
						}}}},
					}},
				},
			},
			SingleSignOnServices: []saml.Endpoint{{Binding: saml.HTTPRedirectBinding, Location: "http://idp.example.com/sso"}},
		}},
	}
	b, err := xml.Marshal(ed)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSAMLSettings(t *testing.T) {
	spec := func(key string) settingSpec {
		s, ok := settingSpecFor(key)
		if !ok {
			t.Fatalf("missing setting %q", key)
		}
		return s
	}
	if s := spec("saml_enabled"); s.Kind != "bool" || s.Default != "0" {
		t.Errorf("saml_enabled: %+v", s)
	}
	if s := spec("saml_attr_username"); s.Kind != "string" || s.Default != "uid" {
		t.Errorf("saml_attr_username: %+v", s)
	}
	if s := spec("saml_attr_groups"); s.Kind != "string" || s.Default != "groups" {
		t.Errorf("saml_attr_groups: %+v", s)
	}
	for _, k := range []string{"saml_idp_metadata", "saml_entity_id"} {
		if s := spec(k); s.Kind != "string" || s.Default != "" {
			t.Errorf("%s: %+v", k, s)
		}
	}
}

func TestSAMLMetadataAndBogusACS(t *testing.T) {
	samlBackupSettings(t)
	_, cert := newSAMLTestCert(t, "test-idp")
	setSetting("saml_enabled", "1")
	setSetting("saml_idp_metadata", inlineIdpMetadata(t, cert))
	setSetting("saml_attr_username", "uid")
	wrm := newTestServer(t)

	// The metadata endpoint serves this SP's metadata as XML.
	resp, err := http.Get(wrm.URL + "/saml/metadata")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/samlmetadata+xml" || !strings.Contains(string(body), "EntityDescriptor") {
		t.Fatalf("metadata endpoint: %d %s %.300s", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}

	// A start redirects to the IdP and issues a RelayState.
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err = noRedirect.Get(wrm.URL + "/api/auth/saml/start")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || !strings.Contains(resp.Header.Get("Location"), "RelayState=") {
		t.Fatalf("start: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	// A bogus (unparseable / unsigned) response is rejected with 4xx and no XML leak.
	relayState := "test-relay-state"
	samlState.Lock()
	samlState.pending[relayState] = &samlPending{RequestID: "id-test", Created: time.Now()}
	samlState.Unlock()
	resp, err = http.PostForm(wrm.URL+"/saml/acs", url.Values{"SAMLResponse": {"not a real saml response"}, "RelayState": {relayState}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 400 || resp.StatusCode >= 500 {
		t.Fatalf("bogus ACS: want 4xx, got %d", resp.StatusCode)
	}
	if strings.Contains(string(body), "not a real saml response") {
		t.Fatalf("raw SAML response leaked to the browser")
	}
}

func TestSAMLSignInRoundTrip(t *testing.T) {
	samlBackupSettings(t)
	idpKey, idpCert := newSAMLTestCert(t, "test-idp")

	// Stand up an in-process IdP (the library's test IdP).
	idpSrv := httptest.NewUnstartedServer(nil)
	idpURL, _ := url.Parse("http://" + idpSrv.Listener.Addr().String())
	idp, err := samlidp.New(samlidp.Options{URL: *idpURL, Key: idpKey, Certificate: idpCert, Store: &samlidp.MemoryStore{}})
	if err != nil {
		t.Fatal(err)
	}
	idpSrv.Config.Handler = idp
	idpSrv.Start()
	t.Cleanup(idpSrv.Close)

	// Configure WRM to use the IdP's metadata URL.
	setSetting("saml_enabled", "1")
	setSetting("saml_idp_metadata", idpSrv.URL+"/metadata")
	setSetting("saml_entity_id", "") // derive from the request
	setSetting("saml_attr_username", "uid")
	wrm := newTestServer(t)

	// Register WRM (this SP) with the IdP using WRM's own metadata.
	mr, err := http.Get(wrm.URL + "/saml/metadata")
	if err != nil {
		t.Fatal(err)
	}
	spMeta, _ := io.ReadAll(mr.Body)
	mr.Body.Close()
	if mr.StatusCode != 200 {
		t.Fatalf("WRM metadata: %d %.300s", mr.StatusCode, spMeta)
	}
	req, _ := http.NewRequest("PUT", idpSrv.URL+"/services/wrm-sp", bytes.NewReader(spMeta))
	if pr, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else if pr.StatusCode != 204 {
		body, _ := io.ReadAll(pr.Body)
		pr.Body.Close()
		t.Fatalf("register SP: %d %.300s", pr.StatusCode, body)
	} else {
		pr.Body.Close()
	}

	// An IdP user whose username ("alice") will come back in the uid attribute.
	req, _ = http.NewRequest("PUT", idpSrv.URL+"/users/alice", strings.NewReader(`{"name":"alice","password":"secret","email":"alice@example.com","common_name":"Alice"}`))
	if pr, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else if pr.StatusCode != 204 {
		t.Fatalf("create IdP user: %d", pr.StatusCode)
	} else {
		pr.Body.Close()
	}

	// Log in to the IdP to establish its session cookie.
	lr, err := http.PostForm(idpSrv.URL+"/login", url.Values{"user": {"alice"}, "password": {"secret"}})
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, lr.Body)
	lr.Body.Close()
	var sessionCookie string
	for _, c := range lr.Cookies() {
		if c.Name == "session" {
			sessionCookie = c.Value
		}
	}
	if sessionCookie == "" {
		t.Fatal("no IdP session cookie")
	}

	// Drive the WRM login flow with a cookie jar so the session cookie is captured.
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// 1. Start the sign-in at WRM.
	sr, err := client.Get(wrm.URL + "/api/auth/saml/start")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, sr.Body)
	sr.Body.Close()
	if sr.StatusCode != http.StatusFound {
		t.Fatalf("start: %d", sr.StatusCode)
	}
	ssoURL := sr.Header.Get("Location")
	if ssoURL == "" {
		t.Fatal("start did not redirect to the IdP")
	}

	// 2. Follow to the IdP SSO endpoint with the IdP session cookie.
	ssoReq, _ := http.NewRequest("GET", ssoURL, nil)
	ssoReq.AddCookie(&http.Cookie{Name: "session", Value: sessionCookie})
	ssor, err := client.Do(ssoReq)
	if err != nil {
		t.Fatal(err)
	}
	ssoBody, _ := io.ReadAll(ssor.Body)
	ssor.Body.Close()
	if ssor.StatusCode != 200 {
		t.Fatalf("IdP SSO: %d %.300s", ssor.StatusCode, ssoBody)
	}
	samlResponse := formField(ssoBody, "SAMLResponse")
	relayState := formField(ssoBody, "RelayState")
	if samlResponse == "" || relayState == "" {
		t.Fatalf("IdP response form missing SAMLResponse/RelayState: %.300s", ssoBody)
	}
	// The IdP's auto-submit form HTML-escapes the base64 (+ → &#43;, / → &#47;, = → &#61;).
	samlResponse = html.UnescapeString(samlResponse)

	// 3. POST the assertion to the ACS.
	ar, err := client.PostForm(wrm.URL+"/saml/acs", url.Values{"SAMLResponse": {samlResponse}, "RelayState": {relayState}})
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, ar.Body)
	ar.Body.Close()
	if ar.StatusCode != 200 {
		t.Fatalf("ACS: %d", ar.StatusCode)
	}

	// A session was created (finishLogin reached) and the account is directory-only.
	mer, err := client.Get(wrm.URL + "/api/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	var me map[string]interface{}
	if err := json.NewDecoder(mer.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	mer.Body.Close()
	if me["username"] != "alice" || me["sso_only"] != true {
		t.Fatalf("me after SAML sign-in: %v", me)
	}
	var ph string
	var ssoOnly, isAdmin int
	db.QueryRow(`SELECT password_hash, sso_only, is_admin FROM users WHERE username='alice'`).Scan(&ph, &ssoOnly, &isAdmin)
	if ph != "!sso" || ssoOnly != 1 || isAdmin != 0 {
		t.Fatalf("account: password_hash=%q sso_only=%d is_admin=%d", ph, ssoOnly, isAdmin)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='auth.login' AND user_id=(SELECT id FROM users WHERE username='alice')`).Scan(&n)
	if n == 0 {
		t.Fatal("no auth.login audit entry")
	}

	// Replaying the same assertion must fail: the RelayState was already consumed.
	rr, err := client.PostForm(wrm.URL+"/saml/acs", url.Values{"SAMLResponse": {samlResponse}, "RelayState": {relayState}})
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, rr.Body)
	rr.Body.Close()
	if rr.StatusCode < 400 {
		t.Fatalf("replayed assertion accepted: %d", rr.StatusCode)
	}
}

func formField(body []byte, name string) string {
	re := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]*)"`)
	if m := re.FindSubmatch(body); m != nil {
		return string(m[1])
	}
	return ""
}
