// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain runs the tests against a throw-away database.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wrm-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("DB_PATH", filepath.Join(dir, "test.db"))
	os.Unsetenv("ENCRYPTION_KEY")
	initDB()
	loadAppSettings()
	initServerSecret()
	initEncryptionKey()
	code := m.Run()
	db.Close()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestTOTPRFC6238(t *testing.T) {
	// RFC 6238 test secret "12345678901234567890" (SHA-1), T = 59 s → 94287082 (8 digits).
	secret := []byte("12345678901234567890")
	if got := totpCodeAt(secret, 59/30); got != "287082" {
		t.Fatalf("totp = %s, want 287082", got)
	}
	if got := totpCodeAt(secret, 1111111109/30); got != "081804" {
		t.Fatalf("totp = %s, want 081804", got)
	}
}

func TestTOTPVerifyAndReplay(t *testing.T) {
	sec := newTOTPSecret()
	raw, err := decodeTOTPSecret(sec)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix() / totpPeriod
	code := totpCodeAt(raw, now)
	ok, step := totpVerify(sec, code, 0)
	if !ok || step != now {
		t.Fatalf("valid code rejected")
	}
	if ok, _ := totpVerify(sec, code, step); ok {
		t.Fatalf("replayed code accepted")
	}
	if ok, _ := totpVerify(sec, "000000", 0); ok && code != "000000" {
		t.Fatalf("wrong code accepted")
	}
}

func TestRecoveryCodesSingleUse(t *testing.T) {
	codes, stored := newRecoveryCodes()
	if len(codes) != 10 || recoveryCodesLeft(stored) != 10 {
		t.Fatalf("expected 10 codes")
	}
	ok, rest := useRecoveryCode(stored, strings.ToUpper(strings.ReplaceAll(codes[3], "-", "")))
	if !ok || recoveryCodesLeft(rest) != 9 {
		t.Fatalf("recovery code not accepted")
	}
	if ok, _ := useRecoveryCode(rest, codes[3]); ok {
		t.Fatalf("recovery code accepted twice")
	}
}

func TestRolePermissions(t *testing.T) {
	cases := []struct {
		role, perm string
		want       bool
	}{
		{RoleObserver, PermFilesRead, false},
		{RoleViewer, PermFilesRead, true},
		{RoleViewer, PermTerminal, false},
		{RoleViewer, PermFilesWrite, false},
		{RoleOperator, PermTerminal, true},
		{RoleOperator, PermFilesWrite, true},
		{RoleOperator, PermModerate, false},
		{RoleModerator, PermModerate, true},
		{RoleModerator, PermManage, false},
		{RoleOwner, PermManage, true},
		{"bogus", PermFilesRead, false},
	}
	for _, c := range cases {
		if got := roleCan(c.role, c.perm); got != c.want {
			t.Errorf("roleCan(%s,%s)=%v want %v", c.role, c.perm, got, c.want)
		}
	}
	if validAssignableRole(RoleOwner, 5) || !validAssignableRole(RoleOperator, roleRank[RoleOperator]) || validAssignableRole(RoleModerator, roleRank[RoleOperator]) {
		t.Errorf("validAssignableRole wrong")
	}
}

func mustUser(t *testing.T, name string, admin bool) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?,?,?,?)`, name, "x", boolToInt(admin), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func mustShare(t *testing.T, owner int, mode, guestRole, pwHash string) ShareLink {
	t.Helper()
	var ph interface{}
	if pwHash != "" {
		ph = pwHash
	}
	res, err := db.Exec(`INSERT INTO share_links (owner_id, token, name, password_hash, public, active, created_at, access_mode, guest_role, expires_at) VALUES (?,?,?,?,0,1,?,?,?, '')`,
		owner, randomToken(24), "s", ph, time.Now().UTC().Format(time.RFC3339), mode, guestRole)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	s, err := getShareByID(int(id))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestShareAccessModes(t *testing.T) {
	owner := mustUser(t, "owner1", false)
	member := mustUser(t, "member1", false)
	other := mustUser(t, "other1", false)
	guest := accessCtx{GuestPKey: guestPKeyFromRaw(randomToken(24))}

	members := mustShare(t, owner, "members", RoleViewer, "")
	db.Exec(`INSERT INTO share_members (share_id, user_id, role) VALUES (?,?,?)`, members.ID, member, RoleOperator)
	if a, d := resolveShareAccessCtx(members, accessCtx{UserID: owner}); d != nil || a.Role != RoleOwner {
		t.Fatalf("owner: %+v %+v", a, d)
	}
	if a, d := resolveShareAccessCtx(members, accessCtx{UserID: member}); d != nil || a.Role != RoleOperator {
		t.Fatalf("member: %+v %+v", a, d)
	}
	if _, d := resolveShareAccessCtx(members, accessCtx{UserID: other}); d == nil || d.Reason != "not_member" {
		t.Fatalf("non-member must be refused: %+v", d)
	}
	if _, d := resolveShareAccessCtx(members, guest); d == nil || d.Reason != "login_required" {
		t.Fatalf("guest must sign in: %+v", d)
	}

	users := mustShare(t, owner, "users", RoleViewer, "")
	if a, d := resolveShareAccessCtx(users, accessCtx{UserID: other}); d != nil || a.Role != RoleViewer {
		t.Fatalf("signed-in user on users-share: %+v %+v", a, d)
	}
	if _, d := resolveShareAccessCtx(users, guest); d == nil {
		t.Fatalf("guest on users-share must be refused")
	}

	hash, _ := hashPassword("share-pass")
	link := mustShare(t, owner, "link", RoleObserver, hash)
	if _, d := resolveShareAccessCtx(link, guest); d == nil || d.Reason != "password_required" {
		t.Fatalf("password required: %+v", d)
	}
	g := guest
	g.PwCookie = "1"
	if _, d := resolveShareAccessCtx(link, g); d == nil {
		t.Fatalf("forged password cookie accepted")
	}
	g.PwCookie = sharePwCookieValue(link)
	a, d := resolveShareAccessCtx(link, g)
	if d != nil || a.Role != RoleObserver || !a.Guest {
		t.Fatalf("guest with valid cookie: %+v %+v", a, d)
	}
	// Ban and role override apply per person.
	setParticipantRole(link.ID, a.PKey, RoleOperator)
	if a2, _ := resolveShareAccessCtx(link, g); a2 == nil || a2.Role != RoleOperator {
		t.Fatalf("role override not applied")
	}
	setParticipantBanned(link.ID, a.PKey, true)
	if _, d := resolveShareAccessCtx(link, g); d == nil || d.Reason != "banned" {
		t.Fatalf("banned guest accepted: %+v", d)
	}
	// Paused and expired shares refuse everyone but the owner.
	db.Exec(`UPDATE share_links SET active=0 WHERE id=?`, users.ID)
	users, _ = getShareByID(users.ID)
	if _, d := resolveShareAccessCtx(users, accessCtx{UserID: other}); d == nil || d.Reason != "inactive" {
		t.Fatalf("paused share accepted")
	}
	if _, d := resolveShareAccessCtx(users, accessCtx{UserID: owner}); d != nil {
		t.Fatalf("owner refused on paused share")
	}
	db.Exec(`UPDATE share_links SET active=1, expires_at=? WHERE id=?`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), users.ID)
	users, _ = getShareByID(users.ID)
	if _, d := resolveShareAccessCtx(users, accessCtx{UserID: other}); d == nil || d.Reason != "expired" {
		t.Fatalf("expired share accepted")
	}
}

func TestCSRFMiddleware(t *testing.T) {
	h := securityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	do := func(method, path string, hdr map[string]string) int {
		req := httptest.NewRequest(method, "http://wrm.local"+path, strings.NewReader("{}"))
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := do("POST", "/api/connections", nil); c != 403 {
		t.Errorf("POST without header: %d", c)
	}
	if c := do("POST", "/api/connections", map[string]string{"X-WRM-Request": "1"}); c != 204 {
		t.Errorf("POST with header: %d", c)
	}
	if c := do("POST", "/api/connections", map[string]string{"X-WRM-Request": "1", "Origin": "https://evil.example"}); c != 403 {
		t.Errorf("foreign origin: %d", c)
	}
	if c := do("POST", "/api/connections", map[string]string{"X-WRM-Request": "1", "Origin": "http://wrm.local"}); c != 204 {
		t.Errorf("same origin: %d", c)
	}
	if c := do("GET", "/api/connections", nil); c != 204 {
		t.Errorf("GET: %d", c)
	}
	req := httptest.NewRequest("GET", "http://wrm.local/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	for _, hd := range []string{"Content-Security-Policy", "X-Frame-Options", "X-Content-Type-Options", "Referrer-Policy"} {
		if rec.Header().Get(hd) == "" {
			t.Errorf("missing header %s", hd)
		}
	}
}

func TestEncryptionAndLegacyMigration(t *testing.T) {
	enc := encryptValue("s3cret")
	if !strings.HasPrefix(enc, "ENC:") || decryptValue(enc) != "s3cret" {
		t.Fatalf("round trip failed")
	}
	// A value encrypted with the old public default key is re-encrypted at startup.
	legacy, err := encryptWith(legacyDeriveKey(legacyDefaultEncryptionKey), "old-password")
	if err != nil {
		t.Fatal(err)
	}
	uid := mustUser(t, "legacy1", false)
	res, _ := db.Exec(`INSERT INTO connections (name, host, password, user_id) VALUES ('c','h',?,?)`, legacy, uid)
	id, _ := res.LastInsertId()
	if decryptValue(legacy) != "" {
		t.Fatalf("legacy value must not decrypt with the new key")
	}
	migrateStoredSecrets()
	c, err := loadConnectionRaw(int(id))
	if err != nil || c.Password != "old-password" {
		t.Fatalf("migration failed: %q %v", c.Password, err)
	}
	if k := getEncryptionKey(); hex.EncodeToString(k) == hex.EncodeToString(legacyDeriveKey(legacyDefaultEncryptionKey)) {
		t.Fatalf("the public default key is still in use")
	}
}

func TestSignedValues(t *testing.T) {
	sig := signValue("a", "b")
	if !verifySigned(sig, "a", "b") || verifySigned(sig, "a", "c") || verifySigned(sig, "ab") {
		t.Fatalf("signature check wrong")
	}
	tok := newMFAToken(42)
	if uid, _, ok := parseMFAToken(tok); !ok || uid != 42 {
		t.Fatalf("mfa token not accepted")
	}
	if _, _, ok := parseMFAToken(strings.Replace(tok, "42.", "43.", 1)); ok {
		t.Fatalf("tampered mfa token accepted")
	}
}

func TestSettingsValidation(t *testing.T) {
	if _, err := setSetting("registration", "maybe"); err == nil {
		t.Errorf("invalid enum accepted")
	}
	if _, err := setSetting("password_min_length", "3"); err == nil {
		t.Errorf("out-of-range int accepted")
	}
	if _, err := setSetting("ice_servers", "not json"); err == nil {
		t.Errorf("invalid json accepted")
	}
	if _, err := setSetting("turn_relay_ports", "60000-50000"); err == nil {
		t.Errorf("invalid port range accepted")
	}
	if v, err := setSetting("allow_link_shares", "false"); err != nil || v != "0" || settingBool("allow_link_shares") {
		t.Errorf("bool setting: %v %v", v, err)
	}
	setSetting("allow_link_shares", "1")
	os.Setenv("WRM_REGISTRATION", "open")
	defer os.Unsetenv("WRM_REGISTRATION")
	if getSetting("registration") != "open" || !settingLocked("registration") {
		t.Errorf("environment override not applied")
	}
	if _, err := setSetting("registration", "closed"); err == nil {
		t.Errorf("locked setting changed")
	}
}

func TestInputValidation(t *testing.T) {
	for _, u := range []string{"alice", "a.b-c_d@corp", "Bob2"} {
		if validUsername(u) != nil {
			t.Errorf("valid username rejected: %s", u)
		}
	}
	for _, u := range []string{"", "a", "-bad", "with space", "<script>", strings.Repeat("x", 65)} {
		if validUsername(u) == nil {
			t.Errorf("invalid username accepted: %q", u)
		}
	}
	if cleanDisplayName("  <b>Ana</b>\x00  Kovač ") != "bAna/b Kovač" {
		t.Errorf("display name cleaning: %q", cleanDisplayName("  <b>Ana</b>\x00  Kovač "))
	}
	for _, s := range []string{"w1", "abc_DEF-9"} {
		if !validStreamID(s) {
			t.Errorf("valid stream id rejected: %s", s)
		}
	}
	for _, s := range []string{"", "a'b", "x\"y", "<s>", strings.Repeat("a", 33)} {
		if validStreamID(s) {
			t.Errorf("invalid stream id accepted: %q", s)
		}
	}
}
