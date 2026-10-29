// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ─── SERVER SECRET & SIGNING ─────────────────────────
//
// A random per-installation secret (stored in the database) signs short-lived tokens:
// share password cookies, the two-factor login step and TURN credentials. It is never
// sent to a browser.

var serverSecret []byte

func initServerSecret() {
	if b, err := hex.DecodeString(getInternal("server_secret")); err == nil && len(b) >= 32 {
		serverSecret = b
		return
	}
	serverSecret = make([]byte, 32)
	if _, err := rand.Read(serverSecret); err != nil {
		log.Fatalf("cannot generate server secret: %v", err)
	}
	if err := setInternal("server_secret", hex.EncodeToString(serverSecret)); err != nil {
		log.Fatalf("cannot store server secret: %v", err)
	}
}

// signValue returns a URL-safe HMAC-SHA256 over the given parts.
func signValue(parts ...string) string {
	mac := hmac.New(sha256.New, serverSecret)
	for _, p := range parts {
		mac.Write([]byte(p))
		mac.Write([]byte{0})
	}
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verifySigned(sig string, parts ...string) bool {
	return hmac.Equal([]byte(sig), []byte(signValue(parts...)))
}

// randomToken returns n random bytes, URL-safe base64 encoded.
func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ─── ENCRYPTION KEY FOR STORED SECRETS ───────────────
//
// Connection passwords, private keys and 2FA secrets are encrypted with AES-256-GCM.
// Key sources, in order: ENCRYPTION_KEY (same derivation as before, so existing
// databases keep working), ENCRYPTION_KEY_FILE, or an automatically generated random key
// stored next to the database (<db>.key, mode 0600). Older versions used a public default
// key when ENCRYPTION_KEY was not set; such secrets are re-encrypted with the new key on
// the first start.

const legacyDefaultEncryptionKey = "change-this-to-a-secure-32-char!"

var encryptionKeySource string

func legacyDeriveKey(key string) []byte {
	if len(key) < 32 {
		key = key + strings.Repeat("0", 32-len(key))
	}
	return []byte(key)[:32]
}

func keyFromFileContent(b []byte) []byte {
	s := strings.TrimSpace(string(b))
	if k, err := hex.DecodeString(s); err == nil && len(k) == 32 {
		return k
	}
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func initEncryptionKey() {
	switch {
	case os.Getenv("ENCRYPTION_KEY") != "":
		encryptionKey = legacyDeriveKey(os.Getenv("ENCRYPTION_KEY"))
		encryptionKeySource = "environment (ENCRYPTION_KEY)"
	case os.Getenv("ENCRYPTION_KEY_FILE") != "":
		p := os.Getenv("ENCRYPTION_KEY_FILE")
		b, err := os.ReadFile(p)
		if err != nil || len(strings.TrimSpace(string(b))) < 16 {
			log.Fatalf("ENCRYPTION_KEY_FILE %s: cannot read a key (at least 16 characters): %v", p, err)
		}
		encryptionKey = keyFromFileContent(b)
		encryptionKeySource = "file " + p
	default:
		p := resolveDBPath() + ".key"
		if b, err := os.ReadFile(p); err == nil && len(strings.TrimSpace(string(b))) >= 16 {
			encryptionKey = keyFromFileContent(b)
		} else {
			k := make([]byte, 32)
			if _, err := rand.Read(k); err != nil {
				log.Fatalf("cannot generate encryption key: %v", err)
			}
			if err := os.WriteFile(p, []byte(hex.EncodeToString(k)+"\n"), 0o600); err != nil {
				log.Fatalf("cannot write encryption key file %s: %v (set ENCRYPTION_KEY instead)", p, err)
			}
			encryptionKey = k
			log.Printf("Generated a new random encryption key: %s — back it up: without it the stored passwords and keys cannot be decrypted. Keep that backup apart from database backups.", p)
		}
		encryptionKeySource = "file " + p
	}
	migrateStoredSecrets()
	protectBitwardenProfilesAtStartup()
}

func getEncryptionKey() []byte { return encryptionKey }

func encryptWith(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return "ENC:" + base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

func decryptWith(key []byte, value string) (string, error) {
	if !strings.HasPrefix(value, "ENC:") {
		return value, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "ENC:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(decoded) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	plain, err := gcm.Open(nil, decoded[:gcm.NonceSize()], decoded[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func encryptValue(plaintext string) string {
	if plaintext == "" {
		return ""
	}
	v, err := encryptWith(getEncryptionKey(), plaintext)
	if err != nil {
		log.Printf("encrypt: %v", err)
		return ""
	}
	return v
}

// decryptValue returns "" when a value cannot be decrypted (wrong key), so a broken secret
// is never used as a password.
func decryptValue(value string) string {
	if value == "" {
		return ""
	}
	v, err := decryptWith(getEncryptionKey(), value)
	if err != nil {
		return ""
	}
	return v
}

// migrateStoredSecrets re-encrypts secrets that were stored with the old public default
// key (or in plain text) with the current key.
func migrateStoredSecrets() {
	legacy := legacyDeriveKey(legacyDefaultEncryptionKey)
	current := getEncryptionKey()
	rows, err := db.Query(`SELECT id, COALESCE(password,''), COALESCE(private_key,'') FROM connections`)
	if err != nil {
		return
	}
	type upd struct {
		id       int
		pw, key  string
		changed  bool
		failures int
	}
	var list []upd
	for rows.Next() {
		var u upd
		rows.Scan(&u.id, &u.pw, &u.key)
		list = append(list, u)
	}
	rows.Close()
	fix := func(v string, u *upd) string {
		if v == "" {
			return v
		}
		if _, err := decryptWith(current, v); err == nil && strings.HasPrefix(v, "ENC:") {
			return v
		}
		plain := v
		if strings.HasPrefix(v, "ENC:") {
			p, err := decryptWith(legacy, v)
			if err != nil {
				u.failures++
				return v
			}
			plain = p
		}
		enc, err := encryptWith(current, plain)
		if err != nil {
			u.failures++
			return v
		}
		u.changed = true
		return enc
	}
	migrated, failed := 0, 0
	for i := range list {
		u := &list[i]
		u.pw = fix(u.pw, u)
		u.key = fix(u.key, u)
		failed += u.failures
		if u.changed {
			if _, err := db.Exec(`UPDATE connections SET password=?, private_key=? WHERE id=?`, u.pw, u.key, u.id); err == nil {
				migrated++
			}
		}
	}
	if migrated > 0 {
		log.Printf("Security: re-encrypted the secrets of %d connection(s) with the current encryption key", migrated)
	}
	if failed > 0 {
		log.Printf("WARNING: %d stored secret(s) cannot be decrypted with the current key — was ENCRYPTION_KEY changed? Re-enter those passwords/keys.", failed)
	}
}

// ─── HTTP HARDENING MIDDLEWARE ───────────────────────

// trustProxy makes WRM use X-Forwarded-For / X-Real-IP / X-Forwarded-Proto from a reverse
// proxy. Only enable it when WRM is reachable exclusively through that proxy.
var trustProxy = os.Getenv("WRM_TRUST_PROXY") == "1"

func clientIP(r *http.Request) string {
	if trustProxy {
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			return hostOnly(ip)
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return hostOnly(strings.TrimSpace(parts[len(parts)-1]))
		}
	}
	return hostOnly(r.RemoteAddr)
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || (trustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
}

// guardEgressURL refuses URLs that point at a cloud metadata service or a link-local
// address. They are never a legitimate Git, NetBox or AI provider target and are the
// fastest route from server-side request forgery to credentials.
func guardEgressURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return fmt.Errorf("the address must be an http(s) URL without credentials")
	}
	h := strings.ToLower(u.Hostname())
	switch h {
	case "metadata.google.internal", "metadata.goog", "instance-data":
		return fmt.Errorf("%s is a cloud metadata endpoint", h)
	}
	if ip := net.ParseIP(h); ip != nil {
		switch ip.String() {
		case "169.254.169.254", "100.100.100.200", "fd00:ec2::254":
			return fmt.Errorf("%s is a cloud metadata endpoint", h)
		}
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("%s is a link-local address", h)
		}
	}
	return nil
}

func setCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// sameOriginRequest reports whether the Origin (or Referer) of a request, when present,
// points to this server.
func sameOriginRequest(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "null" {
		return false
	}
	return originAllowed(r, origin)
}

func contentSecurityPolicy(r *http.Request) string {
	host := r.Host
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-inline'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:",
		"font-src 'self' data:",
		"media-src 'self' blob: data:",
		"connect-src 'self' ws://" + host + " wss://" + host + gitRelayBrowserOrigins(r),
		"worker-src 'self' blob:",
		"object-src 'none'",
		"base-uri 'none'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	}, "; ")
}

// Import limits. A file of up to maxImportFile bytes (WRM JSON, mRemoteNG XML, SSH config,
// CSV/XLSX inventory) fits into a request body of maxImportBody bytes even after JSON
// escaping or base64 encoding (4/3). The browser checks maxImportFile before sending.
const (
	maxImportFile = 20 << 20
	maxImportBody = 32 << 20
)

// isImportPath reports whether a request path is an import endpoint with the larger body limit.
func isImportPath(path string) bool {
	return path == "/api/config/import" || strings.HasPrefix(path, "/api/config/import/") ||
		path == "/api/inventory" || strings.HasPrefix(path, "/api/inventory/")
}

func bodyLimitFor(path string) int64 {
	switch {
	case path == "/api/addons/install":
		return addonMaxArchive + (64 << 10)
	case path == "/api/git/medium/setup":
		return 3*mediumArchiveLimit + (1 << 20)
	case strings.HasPrefix(path, "/api/git/medium/"):
		return mediumArchiveLimit + (1 << 20)
	case path == "/api/remote/upload":
		return -1 // streamed; limited per file by the max_upload_mb policy
	case path == "/api/git/bundles":
		return gitMaxBundle
	case strings.HasPrefix(path, "/api/hooks/"):
		return gitHookMaxBody
	case path == "/api/git/catalog" || path == "/api/git/catalog/import":
		return maxImportBody
	case isImportPath(path):
		return maxImportBody
	case strings.HasPrefix(path, "/api/sessions"):
		return 2 << 20
	}
	return 1 << 20
}

// publicAPIPaths are the /api and /ws routes that do not rely on a session cookie: sign-in
// and its helpers, token-based shares, hook secrets, and the SCIM, OAuth and MCP endpoints
// that present a bearer token. Everything else under /api/ and /ws/ needs a signed-in
// session, so a handler that forgets requireAuth is refused instead of exposed.
var publicAPIPaths = []string{
	"/api/version",
	"/api/auth/config",
	"/api/auth/register",
	"/api/auth/login",
	"/api/auth/logout",
	"/api/auth/mfa",
	"/api/auth/webauthn/login/",
	"/api/auth/oidc/callback",
	"/api/auth/oidc/result",
	"/api/auth/saml/start",
	"/api/share/",
	"/api/hooks/",
	"/api/mcp/",
	"/ws/share/",
	"/scim/v2/",
	"/saml/",
	"/mcp",
	"/oauth/",
	"/.well-known/",
}

func isPublicAPIPath(p string) bool {
	for _, prefix := range publicAPIPaths {
		if p == prefix || strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// authGate is the deny-by-default authorization layer. A request to /api or /ws without a
// valid session is refused before any handler runs, unless its path is listed above.
func authGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if (strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/ws/")) && !isPublicAPIPath(p) {
			if _, err := lookupSession(r); err != nil {
				jsonError(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "microphone=(self), camera=(), geolocation=(), payment=(), usb=(), interest-cohort=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Content-Security-Policy", contentSecurityPolicy(r))
		if isHTTPS(r) {
			h.Set("Strict-Transport-Security", "max-age=15552000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
			// CSRF protection: state-changing API calls must carry the custom header that
			// only same-origin JavaScript can set, and must not come from another site.
			// Incoming webhooks have no session: their secret authenticates them.
			if !safeMethod(r.Method) && !strings.HasPrefix(r.URL.Path, "/api/hooks/") && (r.Header.Get("X-WRM-Request") != "1" || !sameOriginRequest(r)) {
				jsonError(w, "Request blocked by CSRF protection — reload the page and try again", 403)
				return
			}
			if limit := bodyLimitFor(r.URL.Path); limit > 0 && r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// staticHandler serves the embedded UI assets without directory listings.
func staticHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/static/" || r.URL.Path == "/static/index.html" {
			// Up to v9 the app was also reachable at /static/: keep bookmarks working.
			target := "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if _, err := fs.Stat(staticFiles, name); err != nil {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/static/vendor/") {
			w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		serveEmbedded(w, r, name)
	})
}

// ─── SELF-SIGNED TLS (HTTPS_SELF_SIGNED=1) ───────────
//
// Browsers only allow the microphone on HTTPS pages (or localhost). For a quick LAN setup
// without a certificate WRM can create a self-signed one; browsers show a warning once.

func ensureSelfSignedCert() (string, string, error) {
	dir := filepath.Dir(resolveDBPath())
	certFile := filepath.Join(dir, "wrm-selfsigned.crt")
	keyFile := filepath.Join(dir, "wrm-selfsigned.key")
	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			return certFile, keyFile, nil
		}
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	hostname, _ := os.Hostname()
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Web Remote Manager (" + hostname + ")", Organization: []string{"Web Remote Manager PRO"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(3, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	if hostname != "" {
		tpl.DNSNames = append(tpl.DNSNames, hostname)
	}
	if extra := os.Getenv("HTTPS_SELF_SIGNED_HOSTS"); extra != "" {
		for _, h := range strings.Split(extra, ",") {
			if h = strings.TrimSpace(h); h == "" {
				continue
			} else if ip := net.ParseIP(h); ip != nil {
				tpl.IPAddresses = append(tpl.IPAddresses, ip)
			} else {
				tpl.DNSNames = append(tpl.DNSNames, h)
			}
		}
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && !ipn.IP.IsLinkLocalUnicast() {
				tpl.IPAddresses = append(tpl.IPAddresses, ipn.IP)
			}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(der)
	log.Printf("Created self-signed certificate %s (SHA-256 %s)", certFile, strings.ToUpper(hex.EncodeToString(sum[:])))
	return certFile, keyFile, nil
}

// ─── INPUT VALIDATION ────────────────────────────────

func validUsername(u string) error {
	if len(u) < 2 || len(u) > 64 {
		return fmt.Errorf("username must be 2–64 characters")
	}
	for i, ch := range u {
		ok := ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || (i > 0 && strings.ContainsRune("._@-", ch))
		if !ok {
			return fmt.Errorf("username may contain letters, digits and . _ @ - (not at the start)")
		}
	}
	return nil
}

func validatePassword(pw string) error {
	min := settingInt("password_min_length")
	if len([]rune(pw)) < min {
		return fmt.Errorf("password must be at least %d characters", min)
	}
	if len(pw) > 72 {
		return fmt.Errorf("password must be at most 72 bytes")
	}
	return nil
}

// formatMB formats a byte count as megabytes for error messages ("12.4 MB").
func formatMB(n int64) string {
	return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
}

// importTooLarge answers 413 with the size and the limit.
func importTooLarge(w http.ResponseWriter, size int64) {
	msg := "The file is too large"
	if size > 0 {
		msg += " (" + formatMB(size) + ")"
	}
	jsonError(w, msg+": the import limit is "+formatMB(maxImportFile), http.StatusRequestEntityTooLarge)
}

// decodeImportJSON decodes an import request body. When the body is larger than the
// middleware limit it answers 413 with the size and the limit instead of "Bad JSON".
func decodeImportJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if r.ContentLength > maxImportBody {
		importTooLarge(w, r.ContentLength)
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			importTooLarge(w, r.ContentLength)
			return false
		}
		jsonError(w, "Bad JSON", 400)
		return false
	}
	return true
}
