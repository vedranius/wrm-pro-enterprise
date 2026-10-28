// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
)

// ─── SAML 2.0 SERVICE PROVIDER ───────────────────────
//
// WRM acts as a SAML 2.0 service provider: an administrator points it at one identity
// provider (saml_idp_metadata, a metadata URL or inline XML) and users sign in through
// that IdP. Unlike OIDC/LDAP there is no per-provider administration — SAML is a single
// global configuration in Settings → Policies.
//
//   - GET  /saml/metadata        serves this SP's metadata (register it at the IdP).
//   - GET  /api/auth/saml/start  starts a sign-in: builds an AuthnRequest and redirects
//     the browser to the IdP, storing the request ID under a random RelayState.
//   - POST /saml/acs             the assertion consumer service: crewjam/saml verifies the
//     response (signature, Issuer, Destination, InResponseTo, SubjectConfirmation Recipient,
//     NotBefore/NotOnOrAfter and AudienceRestriction), then the username attribute is linked
//     to a WRM account and finishLogin creates the session.
//
// The login is bound to the RelayState/request ID issued in /start: the ACS accepts only a
// response whose RelayState matches an outstanding sign-in (single-use, short-lived) and
// whose InResponseTo matches the request ID stored against it, which prevents login CSRF
// and replay. Raw SAML XML is never returned or written to the browser; on failure the
// browser sees a generic message and the detail goes to the server log only.
//
// Frontend follow-up: apiAuthConfigHandler returns out["saml"] (id/name) exactly like
// out["sso"] (OIDC) and out["ldap"], but static/index.html does not render a "Sign in with
// SAML" button yet. When it does, the button is a full-page link to /api/auth/saml/start.

const samlPendingTTL = 10 * time.Minute

type samlPending struct {
	RequestID string
	Created   time.Time
}

var samlState = struct {
	sync.Mutex
	pending map[string]*samlPending
}{pending: map[string]*samlPending{}}

func samlPrune() {
	for k, v := range samlState.pending {
		if time.Since(v.Created) > samlPendingTTL {
			delete(samlState.pending, k)
		}
	}
}

// samlEnabled reports whether SAML sign-in is configured and turned on.
func samlEnabled() bool {
	return settingBool("saml_enabled") && strings.TrimSpace(getSetting("saml_idp_metadata")) != ""
}

// ─── IdP metadata ────────────────────────────────────

var samlMetaCache = struct {
	sync.Mutex
	m map[string]samlMetaEntry
}{m: map[string]samlMetaEntry{}}

type samlMetaEntry struct {
	meta *saml.EntityDescriptor
	err  error
	at   time.Time
}

// samlIDPMetadata returns the parsed IdP metadata, fetched from the configured URL (cached
// for a while) or parsed from the inline XML.
func samlIDPMetadata() (*saml.EntityDescriptor, error) {
	raw := strings.TrimSpace(getSetting("saml_idp_metadata"))
	if raw == "" {
		return nil, errors.New("the SAML identity provider metadata is not configured")
	}
	samlMetaCache.Lock()
	defer samlMetaCache.Unlock()
	if len(samlMetaCache.m) > 64 {
		samlMetaCache.m = map[string]samlMetaEntry{}
	}
	if e, ok := samlMetaCache.m[raw]; ok && time.Since(e.at) < 30*time.Minute {
		return e.meta, e.err
	}
	meta, err := samlParseMetadata(raw)
	samlMetaCache.m[raw] = samlMetaEntry{meta: meta, err: err, at: time.Now()}
	return meta, err
}

func samlParseMetadata(raw string) (*saml.EntityDescriptor, error) {
	if u, err := url.Parse(raw); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		// The URL is chosen by an administrator (a trusted setting), so http is allowed here.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		meta, err := samlsp.FetchMetadata(ctx, &http.Client{Timeout: 10 * time.Second}, *u)
		if err != nil {
			return nil, fmt.Errorf("cannot fetch the identity provider metadata: %v", err)
		}
		return meta, nil
	}
	meta, err := samlsp.ParseMetadata([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("cannot parse the identity provider metadata: %v", err)
	}
	return meta, nil
}

// ─── SP signing key ──────────────────────────────────
//
// A self-signed key pair is generated on first use and stored (like the server secret) so
// that encrypted assertions keep decrypting across restarts and the published metadata stays
// stable.

const (
	samlSPKeyName  = "saml_sp_key"
	samlSPCertName = "saml_sp_cert"
)

var samlKey = struct {
	sync.Mutex
	key  *rsa.PrivateKey
	cert *x509.Certificate
}{}

func samlSigningKey() (*rsa.PrivateKey, *x509.Certificate, error) {
	samlKey.Lock()
	defer samlKey.Unlock()
	if samlKey.key != nil && samlKey.cert != nil {
		return samlKey.key, samlKey.cert, nil
	}
	if kp, cp := getInternal(samlSPKeyName), getInternal(samlSPCertName); kp != "" && cp != "" {
		if kb, _ := pem.Decode([]byte(kp)); kb != nil {
			if k, err := x509.ParsePKCS8PrivateKey(kb.Bytes); err == nil {
				if rk, ok := k.(*rsa.PrivateKey); ok {
					if cb, _ := pem.Decode([]byte(cp)); cb != nil {
						if c, err := x509.ParseCertificate(cb.Bytes); err == nil {
							samlKey.key, samlKey.cert = rk, c
							return rk, c, nil
						}
					}
				}
			}
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "WRM SAML service provider"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	setInternal(samlSPKeyName, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})))
	setInternal(samlSPCertName, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
	samlKey.key, samlKey.cert = key, cert
	return key, cert, nil
}

// ─── ServiceProvider ─────────────────────────────────

func samlServiceProvider(r *http.Request) (*saml.ServiceProvider, error) {
	idpMeta, err := samlIDPMetadata()
	if err != nil {
		return nil, err
	}
	key, cert, err := samlSigningKey()
	if err != nil {
		return nil, err
	}
	base := mcpBaseURL(r)
	metaURL, _ := url.Parse(base + "/saml/metadata")
	acsURL, _ := url.Parse(base + "/saml/acs")
	return &saml.ServiceProvider{
		EntityID:    strings.TrimSpace(getSetting("saml_entity_id")),
		Key:         key,
		Certificate: cert,
		MetadataURL: *metaURL,
		AcsURL:      *acsURL,
		IDPMetadata: idpMeta,
	}, nil
}

// ─── sign-in options (login page) ────────────────────

func samlSignInOptions() []map[string]interface{} {
	if !samlEnabled() {
		return []map[string]interface{}{}
	}
	return []map[string]interface{}{{"id": 0, "name": "SAML"}}
}

// ─── HTTP: metadata ──────────────────────────────────

func samlMetadataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if !samlEnabled() {
		jsonError(w, "SAML sign-in is not configured", 404)
		return
	}
	sp, err := samlServiceProvider(r)
	if err != nil {
		jsonError(w, "SAML sign-in is not configured", 404)
		return
	}
	buf, err := xml.MarshalIndent(sp.Metadata(), "", "  ")
	if err != nil {
		jsonError(w, "Server error", 500)
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(buf) // #nosec G705 -- generated SAML metadata, not user input
}

// ─── HTTP: start sign-in ─────────────────────────────

func apiSAMLStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if msg := signInTransportError(r); msg != "" {
		oauthPage(w, 403, "Sign-in needs HTTPS", msg)
		return
	}
	if !samlEnabled() {
		oauthPage(w, 404, "Sign-in not available", "SAML sign-in is not set up or is turned off. Ask your WRM administrator.")
		return
	}
	if !mcpRateOK("saml:"+clientIP(r), 30, time.Minute) {
		oauthPage(w, 429, "Too many sign-ins", "Wait a minute and try again.")
		return
	}
	sp, err := samlServiceProvider(r)
	if err != nil {
		log.Printf("SAML: %v", err)
		oauthPage(w, 502, "Cannot reach the identity provider", "WRM could not read the SAML identity provider settings: "+err.Error())
		return
	}
	ssoURL := sp.GetSSOBindingLocation(saml.HTTPRedirectBinding)
	if ssoURL == "" {
		oauthPage(w, 502, "Cannot start sign-in", "The identity provider does not offer an HTTP-Redirect sign-in endpoint.")
		return
	}
	req, err := sp.MakeAuthenticationRequest(ssoURL, saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	if err != nil {
		log.Printf("SAML: %v", err)
		oauthPage(w, 502, "Cannot start sign-in", "WRM could not prepare the sign-in request.")
		return
	}
	relayState := randomToken(32)
	samlState.Lock()
	samlPrune()
	if len(samlState.pending) > 5000 {
		samlState.Unlock()
		oauthPage(w, 429, "Too many sign-ins", "Try again in a few minutes.")
		return
	}
	samlState.pending[relayState] = &samlPending{RequestID: req.ID, Created: time.Now()}
	samlState.Unlock()
	redirectURL, err := req.Redirect(relayState, sp)
	if err != nil {
		log.Printf("SAML: %v", err)
		oauthPage(w, 500, "Server error", "Try again.")
		return
	}
	http.Redirect(w, r, redirectURL.String(), http.StatusFound) // #nosec G710 -- IdP URL from the administrator-configured metadata
}

// ─── HTTP: assertion consumer service ────────────────

func samlACSHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if !samlEnabled() {
		oauthPage(w, 404, "Sign-in not available", "SAML sign-in is not set up or is turned off.")
		return
	}
	if err := r.ParseForm(); err != nil {
		oauthPage(w, 400, "Sign-in failed", "The sign-in could not be read.")
		return
	}
	relayState := r.PostForm.Get("RelayState")
	samlState.Lock()
	pend := samlState.pending[relayState]
	delete(samlState.pending, relayState)
	samlState.Unlock()
	if pend == nil || time.Since(pend.Created) > samlPendingTTL {
		oauthPage(w, 400, "Sign-in expired", "This sign-in is no longer valid. Start the sign-in again from WRM.")
		return
	}
	sp, err := samlServiceProvider(r)
	if err != nil {
		log.Printf("SAML: %v", err)
		oauthPage(w, 502, "Cannot reach the identity provider", "WRM could not read the SAML identity provider settings.")
		return
	}
	assertion, err := sp.ParseResponse(r, []string{pend.RequestID})
	if err != nil {
		if ivr, ok := err.(*saml.InvalidResponseError); ok && ivr.PrivateErr != nil {
			log.Printf("SAML ACS: %v", ivr.PrivateErr)
		} else {
			log.Printf("SAML ACS: %v", err)
		}
		auditLogAs(r, 0, "", "auth.saml_refused", "", map[string]string{"reason": "unverified response"})
		oauthPage(w, 403, "Sign-in refused", "The identity provider's answer could not be verified.")
		return
	}

	username := strings.TrimSpace(samlAttribute(assertion, getSetting("saml_attr_username")))
	if username == "" && assertion.Subject != nil && assertion.Subject.NameID != nil {
		username = strings.TrimSpace(assertion.Subject.NameID.Value)
	}
	if validUsername(username) != nil {
		auditLogAs(r, 0, "", "auth.saml_refused", "", map[string]string{"reason": "unusable username attribute"})
		oauthPage(w, 403, "Sign-in refused", "The identity provider did not send a usable username.")
		return
	}

	// Link by username: an existing WRM account of that name signs in; otherwise the first
	// sign-in creates a directory-only account (enabled, no local password), like OIDC/LDAP.
	u, err := loadUser("username=?", username)
	if err == sql.ErrNoRows {
		if _, ierr := db.Exec(`INSERT INTO users (username, password_hash, is_admin, role, display_name, created_at, password_changed_at, sso_only) VALUES (?,?,?,?,?,?,?,1)`,
			username, "!sso", 0, storedRole(userRoleUser), truncateStr(samlDisplayName(assertion), 100), nowRFC(), nowRFC()); ierr == nil {
			u, err = loadUser("username=?", username)
		} else {
			u, err = loadUser("username=?", username) // lost the race; link to the existing account
		}
	}
	if err != nil {
		log.Printf("SAML ACS: load/create user: %v", err)
		oauthPage(w, 500, "Server error", "WRM could not create your account. Ask your administrator.")
		return
	}
	if u.Disabled {
		auditLogAs(r, u.ID, u.Username, "auth.login_failed", "", map[string]string{"method": "saml", "reason": "account disabled"})
		oauthPage(w, 403, "Sign-in refused", "Your WRM account is disabled. Contact an administrator.")
		return
	}
	if u.BreakGlass {
		auditLogAs(r, u.ID, u.Username, "auth.login_failed", "", map[string]string{"method": "saml", "reason": "break-glass"})
		oauthPage(w, 403, "Sign-in refused", "A break-glass account signs in with its local password only.")
		return
	}
	recordLoginResult(clientIP(r), true)
	finishLogin(w, r, u, "saml")
}

func samlAttribute(assertion *saml.Assertion, name string) string {
	for _, st := range assertion.AttributeStatements {
		for _, a := range st.Attributes {
			if a.Name == name || a.FriendlyName == name {
				for _, v := range a.Values {
					if s := strings.TrimSpace(v.Value); s != "" {
						return s
					}
				}
			}
		}
	}
	return ""
}

func samlDisplayName(assertion *saml.Assertion) string {
	for _, n := range []string{"displayName", "cn", "name", "mail"} {
		if v := samlAttribute(assertion, n); v != "" {
			return v
		}
	}
	return ""
}
