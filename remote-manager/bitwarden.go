// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Bitwarden's official CLI handles authentication and vault cryptography. The
// unlock key stays in memory; the isolated CLI cache contains encrypted vault data.
type bitwardenSession struct {
	mu   sync.Mutex
	key  string
	used time.Time
}

var bitwardenSessions = struct {
	sync.Mutex
	users map[string]*bitwardenSession
}{users: map[string]*bitwardenSession{}}
var bitwardenItemIDRe = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func bitwardenDir(uid int) string {
	p, _ := filepath.Abs(resolveDBPath())
	return filepath.Join(p+".bitwarden", fmt.Sprint(uid))
}
func bitwardenState(uid int) *bitwardenSession {
	bitwardenSessions.Lock()
	defer bitwardenSessions.Unlock()
	k := bitwardenDir(uid)
	s := bitwardenSessions.users[k]
	if s == nil {
		s = &bitwardenSession{}
		bitwardenSessions.users[k] = s
	}
	return s
}
func bitwardenAllowed(uid int) bool {
	return policyWhoAllows(uid, "bitwarden")
}
func bitwardenCLI() string {
	if p := os.Getenv("WRM_BITWARDEN_CLI"); p != "" {
		return p
	}
	p, _ := exec.LookPath("bw")
	return p
}

type boundedCLIOutput struct {
	bytes.Buffer
	max int
}

func (b *boundedCLIOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		return 0, fmt.Errorf("CLI output limit reached")
	}
	return b.Buffer.Write(p)
}
func bitwardenRun(ctx context.Context, uid int, session string, extra []string, args ...string) ([]byte, error) {
	cli := bitwardenCLI()
	if cli == "" {
		return nil, fmt.Errorf("Install the official Bitwarden CLI (bw) on the WRM server or set WRM_BITWARDEN_CLI")
	}
	dir := bitwardenDir(uid)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, fmt.Errorf("Cannot create private Bitwarden cache")
	}
	if e := protectBitwardenCache(dir); e != nil {
		return nil, e
	}
	if e := restoreBitwardenCache(dir); e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, append(args, "--nointeraction")...)
	// Pass runtime paths and certificate configuration, never another account's credentials.
	for _, n := range []string{"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "WINDIR", "TMP", "TEMP", "LANG", "LD_LIBRARY_PATH", "NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if v := os.Getenv(n); v != "" {
			cmd.Env = append(cmd.Env, n+"="+v)
		}
	}
	cmd.Env = append(cmd.Env, "BITWARDENCLI_APPDATA_DIR="+dir, "BW_SESSION="+session, "BW_NOINTERACTION=true")
	cmd.Env = append(cmd.Env, extra...)
	var out boundedCLIOutput
	out.max = 32 << 20
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	runErr := cmd.Run()
	if e := protectBitwardenCache(dir); e != nil {
		return nil, e
	}
	if e := runErr; e != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("Bitwarden request timed out or was canceled")
		}
		return nil, fmt.Errorf("Bitwarden %s failed; check account, vault lock, token/password, MFA and CLI version", args[0])
	}
	return out.Bytes(), nil
}
func bitwardenValidServer(s string) (string, error) {
	if s == "" {
		return "https://vault.bitwarden.com", nil
	}
	u, e := url.Parse(strings.TrimRight(s, "/"))
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("Use an HTTPS Bitwarden server URL without credentials or query parameters")
	}
	return u.String(), nil
}
func bitwardenExpire(uid int, s *bitwardenSession) {
	if s.key != "" && time.Since(s.used) > time.Duration(settingInt("bitwarden_idle_minutes"))*time.Minute {
		s.key = ""
		_, _ = bitwardenRun(context.Background(), uid, "", nil, "lock")
	}
}
func bitwardenCheckKey(uid int, s *bitwardenSession) error {
	bitwardenExpire(uid, s)
	if !bitwardenAllowed(uid) {
		s.key = ""
		return fmt.Errorf("Bitwarden access is disabled by policy")
	}
	if s.key == "" {
		return fmt.Errorf("Unlock your Bitwarden vault in WRM before connecting")
	}
	return nil
}

type bitwardenItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   int    `json:"type"`
	SSHKey struct {
		PrivateKey  string `json:"privateKey"`
		PublicKey   string `json:"publicKey"`
		Fingerprint string `json:"keyFingerprint"`
	} `json:"sshKey"`
}
type bitwardenKeyView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

func bitwardenPrivateKey(uid int, id string) (string, error) {
	if !bitwardenItemIDRe.MatchString(id) {
		return "", fmt.Errorf("Choose a Bitwarden SSH key item")
	}
	s := bitwardenState(uid)
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := bitwardenCheckKey(uid, s); e != nil {
		return "", e
	}
	b, e := bitwardenRun(context.Background(), uid, s.key, nil, "get", "item", id)
	if e != nil {
		return "", e
	}
	var item bitwardenItem
	if json.Unmarshal(b, &item) != nil || item.ID != id || item.Type != 5 || item.SSHKey.PrivateKey == "" {
		return "", fmt.Errorf("The selected Bitwarden item is not an SSH key")
	}
	if _, e = ssh.ParsePrivateKey([]byte(item.SSHKey.PrivateKey)); e != nil {
		return "", fmt.Errorf("Bitwarden SSH key cannot be parsed; use an unencrypted native SSH key item")
	}
	return item.SSHKey.PrivateKey, nil
}
func resolveBitwardenAuth(c *Connection) {
	if c.Options == nil && c.ID > 0 {
		var raw string
		db.QueryRow("SELECT COALESCE(options,'') FROM connections WHERE id=? AND user_id=?", c.ID, c.UserID).Scan(&raw)
		json.Unmarshal([]byte(raw), &c.Options)
	}
	key, e := bitwardenPrivateKey(c.UserID, c.Options["bitwarden_item"])
	if e != nil {
		c.authErr = e.Error()
		return
	}
	c.AuthMethod, c.PrivateKey = "KEY", key
}
func apiBitwardenHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if !bitwardenAllowed(uid) {
		jsonError(w, "Bitwarden access is disabled by policy", 403)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/bitwarden")
	s := bitwardenState(uid)
	s.mu.Lock()
	defer s.mu.Unlock()
	bitwardenExpire(uid, s)
	if r.Method == "GET" && rest == "" {
		status := map[string]interface{}{"available": bitwardenCLI() != "", "status": "unauthenticated", "unlocked": s.key != "", "idle_minutes": settingInt("bitwarden_idle_minutes")}
		if bitwardenCLI() != "" {
			b, e := bitwardenRun(r.Context(), uid, s.key, nil, "status")
			if e == nil {
				var raw map[string]interface{}
				if json.Unmarshal(b, &raw) == nil {
					for _, k := range []string{"status", "serverUrl", "userEmail", "lastSync"} {
						status[k] = raw[k]
					}
				}
			}
		}
		jsonOK(w, status)
		return
	}
	if r.Method == "GET" && rest == "/keys" {
		if e := bitwardenCheckKey(uid, s); e != nil {
			jsonError(w, e.Error(), 409)
			return
		}
		b, e := bitwardenRun(r.Context(), uid, s.key, nil, "list", "items")
		if e != nil {
			jsonError(w, e.Error(), 400)
			return
		}
		var items []bitwardenItem
		if json.Unmarshal(b, &items) != nil {
			jsonError(w, "Invalid Bitwarden response", 502)
			return
		}
		out := []bitwardenKeyView{}
		for _, item := range items {
			if item.Type == 5 && bitwardenItemIDRe.MatchString(item.ID) {
				out = append(out, bitwardenKeyView{item.ID, item.Name, item.SSHKey.Fingerprint})
				if len(out) >= 2000 {
					break
				}
			}
		}
		s.used = time.Now()
		jsonOK(w, out)
		return
	}
	if !requireMethod(w, r, "POST") {
		return
	}
	if rest == "/lock" || rest == "/logout" {
		s.key = ""
		action := strings.TrimPrefix(rest, "/")
		_, e := bitwardenRun(r.Context(), uid, "", nil, action)
		auditLog(r, uid, "bitwarden."+action, "", nil)
		if e != nil {
			jsonError(w, e.Error(), 400)
		} else {
			jsonOK(w, map[string]bool{"ok": true})
		}
		return
	}
	if rest == "/sync" {
		if e := bitwardenCheckKey(uid, s); e != nil {
			jsonError(w, e.Error(), 409)
			return
		}
		_, e := bitwardenRun(r.Context(), uid, s.key, nil, "sync")
		if e != nil {
			jsonError(w, e.Error(), 400)
		} else {
			s.used = time.Now()
			auditLog(r, uid, "bitwarden.synced", "", nil)
			jsonOK(w, map[string]bool{"ok": true})
		}
		return
	}
	if rest != "/login" && rest != "/unlock" {
		jsonError(w, "Not found", 404)
		return
	}
	var in struct {
		Server       string `json:"server"`
		Email        string `json:"email"`
		Password     string `json:"master_password"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Method       string `json:"method"`
		Code         string `json:"code"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Password == "" || len(in.Password) > 4096 || len(in.Email) > 300 || len(in.ClientID) > 300 || len(in.ClientSecret) > 4096 || len(in.Code) > 100 {
		jsonError(w, "Master password and valid login fields are required", 400)
		return
	}
	var result []byte
	var e error
	env := []string{"WRM_BW_PASSWORD=" + in.Password}
	if rest == "/login" {
		server, err := bitwardenValidServer(in.Server)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if strings.HasPrefix(in.Email, "-") {
			jsonError(w, "Invalid email", 400)
			return
		}
		s.key = ""
		_, _ = bitwardenRun(r.Context(), uid, "", nil, "logout")
		if _, e = bitwardenRun(r.Context(), uid, "", nil, "config", "server", server); e == nil {
			if in.ClientID != "" && in.ClientSecret != "" {
				env = append(env, "BW_CLIENTID="+in.ClientID, "BW_CLIENTSECRET="+in.ClientSecret)
				_, e = bitwardenRun(r.Context(), uid, "", env, "login", "--apikey")
				if e == nil {
					result, e = bitwardenRun(r.Context(), uid, "", env, "unlock", "--passwordenv", "WRM_BW_PASSWORD", "--raw")
				}
			} else {
				if in.Email == "" {
					jsonError(w, "Email or personal API credentials are required", 400)
					return
				}
				args := []string{"login", in.Email, "--passwordenv", "WRM_BW_PASSWORD", "--raw"}
				if in.Code != "" {
					if in.Method != "0" && in.Method != "1" && in.Method != "3" {
						jsonError(w, "Choose authenticator, email or YubiKey MFA", 400)
						return
					}
					args = append(args, "--method", in.Method, "--code", in.Code)
				}
				result, e = bitwardenRun(r.Context(), uid, "", env, args...)
			}
		}
	} else {
		result, e = bitwardenRun(r.Context(), uid, "", env, "unlock", "--passwordenv", "WRM_BW_PASSWORD", "--raw")
	}
	if e != nil {
		auditLog(r, uid, "bitwarden.login_failed", "", nil)
		jsonError(w, e.Error(), 400)
		return
	}
	session := strings.TrimSpace(string(result))
	if len(session) < 16 || len(session) > 8192 || strings.ContainsAny(session, " \r\n\t") {
		jsonError(w, "Bitwarden did not return an unlock session; check CLI compatibility", 502)
		return
	}
	s.key, s.used = session, time.Now()
	auditLog(r, uid, "bitwarden.unlocked", "", nil)
	jsonOK(w, map[string]bool{"ok": true})
}

func runBitwardenJanitor() {
	for range time.Tick(time.Minute) {
		bitwardenSessions.Lock()
		users := map[string]*bitwardenSession{}
		for k, s := range bitwardenSessions.users {
			users[k] = s
		}
		bitwardenSessions.Unlock()
		for dir, s := range users {
			uid, _ := strconv.Atoi(filepath.Base(dir))
			s.mu.Lock()
			bitwardenExpire(uid, s)
			s.mu.Unlock()
		}
	}
}
func lockBitwardenUser(uid int) {
	s := bitwardenState(uid)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == "" {
		return
	}
	s.key = ""
	_, _ = bitwardenRun(context.Background(), uid, "", nil, "lock")
}

const bitwardenCacheLimit = 64 << 20

func protectBitwardenCache(dir string) error {
	filename := filepath.Join(dir, "data.json")
	f, e := os.Open(filename)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return fmt.Errorf("Cannot read Bitwarden CLI profile")
	}
	data, e := io.ReadAll(io.LimitReader(f, bitwardenCacheLimit+1))
	f.Close()
	if e != nil || len(data) > bitwardenCacheLimit {
		return fmt.Errorf("Bitwarden CLI profile exceeds 64 MiB or cannot be read")
	}
	encrypted, e := encryptWith(getEncryptionKey(), string(data))
	if e != nil {
		return fmt.Errorf("Cannot encrypt Bitwarden CLI profile")
	}
	temp := filename + ".enc.tmp"
	if e = os.WriteFile(temp, []byte(encrypted), 0600); e != nil {
		return fmt.Errorf("Cannot protect Bitwarden CLI profile")
	}
	if e = os.Rename(temp, filename+".enc"); e != nil {
		return fmt.Errorf("Cannot save encrypted Bitwarden CLI profile")
	}
	if e = os.Remove(filename); e != nil {
		return fmt.Errorf("Cannot remove plaintext Bitwarden CLI profile")
	}
	return nil
}
func restoreBitwardenCache(dir string) error {
	raw, e := os.ReadFile(filepath.Join(dir, "data.json.enc"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return fmt.Errorf("Cannot read encrypted Bitwarden CLI profile")
	}
	if !strings.HasPrefix(string(raw), "ENC:") {
		return fmt.Errorf("Invalid encrypted Bitwarden CLI profile")
	}
	plain, e := decryptWith(getEncryptionKey(), string(raw))
	if e != nil {
		return fmt.Errorf("Cannot decrypt Bitwarden CLI profile; restore its matching WRM encryption key")
	}
	if e = os.WriteFile(filepath.Join(dir, "data.json"), []byte(plain), 0600); e != nil {
		return fmt.Errorf("Cannot prepare Bitwarden CLI profile")
	}
	return nil
}

func protectBitwardenProfilesAtStartup() {
	root := filepath.Dir(bitwardenDir(1))
	entries, e := os.ReadDir(root)
	if e != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if _, e := strconv.Atoi(entry.Name()); e == nil {
				if e := protectBitwardenCache(filepath.Join(root, entry.Name())); e != nil {
					log.Printf("Bitwarden profile recovery: %v", e)
				}
			}
		}
	}
}
