// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

func TestWebAuthnStorageRoundTrip(t *testing.T) {
	res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES ('wa-test-990001', 'x', 0, ?)`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	uid64, _ := res.LastInsertId()
	uid := int(uid64)
	defer db.Exec(`DELETE FROM users WHERE id=?`, uid)

	if userHasPasskey(uid) {
		t.Fatal("user should have no passkey")
	}
	cred := &webauthn.Credential{
		ID:        []byte("credential-id-1"),
		PublicKey: []byte{1, 2, 3, 4, 5},
		Transport: []protocol.AuthenticatorTransport{protocol.AuthenticatorTransport("usb")},
	}
	if err := storeWebAuthnCredential(uid, "test key", cred); err != nil {
		t.Fatal(err)
	}
	if !userHasPasskey(uid) {
		t.Fatal("user should have a passkey after storing one")
	}
	loaded := loadWebAuthnCredentials(uid)
	if len(loaded) != 1 || !bytes.Equal(loaded[0].ID, cred.ID) || !bytes.Equal(loaded[0].PublicKey, cred.PublicKey) {
		t.Fatalf("credential round-trip mismatch: %v", loaded)
	}
	// Advance the signature counter and persist it (what happens after an assertion).
	loaded[0].Authenticator.SignCount = 7
	if err := updateWebAuthnCredential(uid, &loaded[0]); err != nil {
		t.Fatal(err)
	}
	again := loadWebAuthnCredentials(uid)
	if len(again) != 1 || again[0].Authenticator.SignCount != 7 {
		t.Fatalf("sign count not persisted: %v", again)
	}
	db.Exec(`DELETE FROM webauthn_credentials WHERE user_id=?`, uid)
}

func TestWebAuthnRPIDDerivation(t *testing.T) {
	if got := webauthnRPID(&http.Request{Host: "wrm.example.com"}); got != "wrm.example.com" {
		t.Fatalf("rpid = %q", got)
	}
	if got := webauthnRPID(&http.Request{Host: "wrm.example.com:8443"}); got != "wrm.example.com" {
		t.Fatalf("rpid with port = %q", got)
	}
	if got := webauthnOrigin(&http.Request{Host: "wrm.example.com:8443"}); got != "http://wrm.example.com:8443" {
		t.Fatalf("origin = %q", got)
	}
}

func TestWebAuthnRegisterBeginRequiresSession(t *testing.T) {
	s := newTestServer(t)
	req, _ := http.NewRequest("POST", s.URL+"/api/auth/webauthn/register/begin", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", s.URL)
	req.Header.Set("X-WRM-Request", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("register begin without a session: %d", resp.StatusCode)
	}
}

func TestWebAuthnLoginBeginRejectsBadToken(t *testing.T) {
	s := newTestServer(t)
	body, _ := json.Marshal(map[string]string{"mfa_token": "garbage"})
	req, _ := http.NewRequest("POST", s.URL+"/api/auth/webauthn/login/begin", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", s.URL)
	req.Header.Set("X-WRM-Request", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("login begin with a bad mfa token: %d", resp.StatusCode)
	}
}
