// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/pkg/sftp"
)

// loadConnectionRaw reads a connection by ID and decrypts its secrets.
func loadConnectionRaw(connID int) (Connection, error) {
	var c Connection
	var uid *int
	err := db.QueryRow(`SELECT id,name,protocol,host,username,auth_method,password,private_key,key_path,folder_id,user_id,jump_conn_id,web_path,key_id,credential_id,proxy_id FROM connections WHERE id=?`, connID).
		Scan(&c.ID, &c.Name, &c.Protocol, &c.Host, &c.Username, &c.AuthMethod, &c.Password, &c.PrivateKey, &c.KeyPath, &c.FolderID, &uid, &c.JumpID, &c.WebPath, &c.KeyID, &c.CredentialID, &c.ProxyID)
	if err != nil {
		return c, fmt.Errorf("connection not found")
	}
	if uid != nil {
		c.UserID = *uid
	}
	var options string
	db.QueryRow("SELECT COALESCE(options,'') FROM connections WHERE id=?", connID).Scan(&options)
	json.Unmarshal([]byte(options), &c.Options)
	decryptConnectionSecrets(&c)
	applyFolderDefaults(&c)
	return c, nil
}

// ─── FOLDER DEFAULTS ─────────────────────────────────
//
// A folder can carry a default jump host and a default proxy. Its connections (also new
// ones) use them unless they choose their own: connections.jump_conn_id / proxy_id are
// NULL for "as the folder", -1 for "none" and an id otherwise. Older binaries treat NULL
// and -1 as "none", so the database stays readable by them.

// applyFolderDefaults turns the stored choice of c (in JumpID / ProxyID) into the
// effective route, with the defaults of c's folder.
func applyFolderDefaults(c *Connection) {
	var fj, fp *int
	if c.FolderID != nil && c.UserID > 0 {
		db.QueryRow(`SELECT jump_conn_id, proxy_id FROM folders WHERE id=? AND user_id=?`, *c.FolderID, c.UserID).Scan(&fj, &fp)
	}
	resolveRouteDefaults(c, fj, fp)
}

func resolveRouteDefaults(c *Connection, folderJump, folderProxy *int) {
	c.jumpRaw, c.proxyRaw = c.JumpID, c.ProxyID
	eff := func(raw, def *int, self int) (*int, bool) {
		if raw == nil {
			if def != nil && *def > 0 && *def != self {
				v := *def
				return &v, true
			}
			return nil, false
		}
		if *raw <= 0 {
			return nil, false
		}
		return raw, false
	}
	c.JumpID, c.jumpInherited = eff(c.jumpRaw, folderJump, c.ID)
	if c.proxyRaw == nil && folderProxy != nil && *folderProxy > 0 && isOwnTunnelProxy(*folderProxy, c.ID) {
		folderProxy = nil // the connection of a WRM tunnel proxy does not go through its own tunnel
	}
	c.ProxyID, c.proxyInherit = eff(c.proxyRaw, folderProxy, 0)
	if c.Protocol == "SERIAL" {
		c.JumpID, c.ProxyID, c.jumpInherited, c.proxyInherit = nil, nil, false, false
	}
}

// isOwnTunnelProxy: proxy proxyID is the WRM SOCKS tunnel of connection connID.
func isOwnTunnelProxy(proxyID, connID int) bool {
	if connID <= 0 {
		return false
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM proxies p JOIN connection_tunnels t ON t.id=p.tunnel_id WHERE p.id=? AND p.kind='wrm_tunnel' AND t.conn_id=?`, proxyID, connID).Scan(&n)
	return n > 0
}

// validateRoutesOf checks the effective routes of connections (after a folder default or
// a folder changed): jump host chains without loops, and a usable proxy.
func validateRoutesOf(userID int, ids []int) error {
	for _, id := range ids {
		c, err := loadConnectionRaw(id)
		if err != nil {
			continue
		}
		if _, err := jumpChain(c); err != nil {
			return fmt.Errorf("%s: %v", c.Name, err)
		}
		if err := validateProxyChoice(userID, id, &c); err != nil {
			return fmt.Errorf("%s: %v", c.Name, err)
		}
	}
	return nil
}

// folderDefaultsOf returns the folders of a user with their defaults.
func folderDefaultsOf(userID int) map[int]Folder {
	out := map[int]Folder{}
	rows, err := db.Query(`SELECT id, name, jump_conn_id, proxy_id FROM folders WHERE user_id=?`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var f Folder
		if rows.Scan(&f.ID, &f.Name, &f.JumpID, &f.ProxyID) == nil {
			out[f.ID] = f
		}
	}
	return out
}

// Effective jump host / proxy of a row of connections c joined with folders f (SQL).
const (
	effJumpSQL  = `CASE WHEN c.protocol='SERIAL' THEN 0 WHEN c.jump_conn_id IS NULL THEN CASE WHEN f.jump_conn_id=c.id THEN 0 ELSE COALESCE(f.jump_conn_id,0) END WHEN c.jump_conn_id<0 THEN 0 ELSE c.jump_conn_id END`
	effProxySQL = `CASE WHEN c.protocol='SERIAL' THEN 0 WHEN c.proxy_id IS NULL THEN CASE WHEN EXISTS (SELECT 1 FROM proxies p JOIN connection_tunnels t ON t.id=p.tunnel_id
		WHERE p.id=f.proxy_id AND p.kind='wrm_tunnel' AND t.conn_id=c.id) THEN 0 ELSE COALESCE(f.proxy_id,0) END WHEN c.proxy_id<0 THEN 0 ELSE c.proxy_id END`
	folderJoin = `LEFT JOIN folders f ON f.id=c.folder_id AND f.user_id=c.user_id`
)

// loadConnection is loadConnectionRaw with the host normalised to host:port for dialing
// and a key of the key store or a vault credential resolved to the secret it points to.
func loadConnection(connID int) (Connection, error) {
	c, err := loadConnectionRaw(connID)
	if err != nil {
		return c, err
	}
	c.Host = ensurePort(c.Host, c.Protocol)
	resolveConnectionAuth(&c)
	return c, nil
}

func isFTP(c Connection) bool {
	p := strings.ToUpper(c.Protocol)
	return p == "FTP" || p == "FTPS"
}

func intPtrEq(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// truncateStr shortens s to at most n bytes without cutting a UTF-8 character.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// attachmentHeader builds a Content-Disposition value that is safe for any file name
// (quotes, non-ASCII characters, etc.).
func attachmentHeader(name string) string {
	v := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if v == "" {
		return `attachment; filename="download"`
	}
	return v
}

// joinRemote joins a remote directory and a name using forward slashes.
func joinRemote(dir, name string) string {
	if dir == "" || dir == "." {
		return name
	}
	return strings.TrimSuffix(dir, "/") + "/" + name
}

// ─── WEBSOCKET ORIGIN CHECK ──────────────────────────

// checkWSOrigin blocks cross-site WebSocket hijacking: the Origin host must match the
// Host (or X-Forwarded-Host) of the request. Set WRM_ALLOW_ANY_ORIGIN=1 to disable, or
// WRM_ALLOWED_ORIGINS=a.example.com,b.example.com to allow extra hosts.
func checkWSOrigin(r *http.Request) bool {
	return originAllowed(r, r.Header.Get("Origin"))
}

// originAllowed reports whether an Origin/Referer URL belongs to this server.
func originAllowed(r *http.Request, origin string) bool {
	if origin == "" || os.Getenv("WRM_ALLOW_ANY_ORIGIN") == "1" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	oh := strings.ToLower(hostOnly(u.Host))
	for _, h := range []string{r.Host, r.Header.Get("X-Forwarded-Host")} {
		for _, part := range strings.Split(h, ",") {
			if part = strings.TrimSpace(part); part != "" && strings.ToLower(hostOnly(part)) == oh {
				return true
			}
		}
	}
	for _, allowed := range strings.Split(os.Getenv("WRM_ALLOWED_ORIGINS"), ",") {
		if a := strings.TrimSpace(allowed); a != "" && strings.ToLower(hostOnly(a)) == oh {
			return true
		}
	}
	log.Printf("Cross-origin request rejected: %s (host %s)", origin, r.Host)
	return false
}

func hostOnly(hp string) string {
	if h, _, err := net.SplitHostPort(hp); err == nil {
		return strings.Trim(h, "[]")
	}
	return strings.Trim(hp, "[]")
}

// browserURL is the address to open in a browser for a listen address such as ":8080".
func browserURL(addr string, tls bool) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	scheme := "http"
	if tls {
		scheme = "https"
	}
	return scheme + "://" + net.JoinHostPort(host, port) + "/"
}

// ─── LOGIN RATE LIMITING ─────────────────────────────

type loginAttempt struct {
	fails       int
	first       time.Time
	lockedUntil time.Time
}

var (
	loginAttempts   = map[string]*loginAttempt{}
	loginAttemptsMu sync.Mutex
)

// Per IP address; accounts additionally lock after login_max_failures (policy).
const (
	loginMaxFails   = 20
	loginFailWindow = 10 * time.Minute
	loginLockout    = 5 * time.Minute
)

// loginBlocked reports whether the IP is temporarily locked out and for how long.
func loginBlocked(ip string) (bool, time.Duration) {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	a := loginAttempts[ip]
	if a == nil {
		return false, 0
	}
	if d := time.Until(a.lockedUntil); d > 0 {
		return true, d
	}
	return false, 0
}

func recordLoginResult(ip string, success bool) {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	if success {
		delete(loginAttempts, ip)
		return
	}
	now := time.Now()
	if len(loginAttempts) > 20000 { // bound memory under a distributed attack
		for k, v := range loginAttempts {
			if now.Sub(v.first) > loginFailWindow && now.After(v.lockedUntil) {
				delete(loginAttempts, k)
			}
		}
	}
	a := loginAttempts[ip]
	if a == nil || now.Sub(a.first) > loginFailWindow {
		a = &loginAttempt{first: now}
		loginAttempts[ip] = a
	}
	a.fails++
	if a.fails >= loginMaxFails {
		a.lockedUntil = now.Add(loginLockout)
		a.fails = 0
		a.first = now
		log.Printf("Login locked for %s for %s after repeated failures", ip, loginLockout)
	}
}

// ─── ERROR CLASSIFICATION ────────────────────────────

func isConnLostErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sftp.ErrSSHFxConnectionLost) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	s := err.Error()
	for _, frag := range []string{"connection lost", "broken pipe", "connection reset", "use of closed network connection", "EOF"} {
		if strings.Contains(s, frag) {
			return true
		}
	}
	return false
}

// loadFolders lists a user's folders with their defaults and the names they point to.
func loadFolders(userID int) []Folder {
	out := []Folder{}
	rows, err := db.Query(`SELECT f.id, f.name, f.parent_id, f.jump_conn_id, f.proxy_id, f.sort_order, f.auto_sort, COALESCE(j.name,''), COALESCE(p.name,'') FROM folders f
		LEFT JOIN connections j ON j.id=f.jump_conn_id LEFT JOIN proxies p ON p.id=f.proxy_id WHERE f.user_id=? ORDER BY f.sort_order, f.name COLLATE NOCASE, f.id`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var f Folder
		var auto int
		if rows.Scan(&f.ID, &f.Name, &f.ParentID, &f.JumpID, &f.ProxyID, &f.SortOrder, &auto, &f.JumpName, &f.ProxyName) == nil {
			f.AutoSort = auto == 1
			out = append(out, f)
		}
	}
	for i := range out {
		out[i].Path = folderPath(out, out[i].ID)
		out[i].Label = loadVisualLabel("folder", out[i].ID)
	}
	return out
}

// validateFolderDefaults checks the default jump host and proxy of a folder.
func validateFolderDefaults(userID int, f *Folder) error {
	if err := validateVisualLabel(f.Label); err != nil {
		return err
	}
	if err := validateFolderParent(userID, f); err != nil {
		return err
	}
	if f.JumpID != nil && *f.JumpID <= 0 {
		f.JumpID = nil
	}
	if f.ProxyID != nil && *f.ProxyID <= 0 {
		f.ProxyID = nil
	}
	if f.JumpID != nil {
		var temp string
		db.QueryRow(`SELECT COALESCE(temp_until,'') FROM connections WHERE id=?`, *f.JumpID).Scan(&temp)
		if temp != "" {
			return fmt.Errorf("a quick connection cannot be a folder's default jump host: save it first")
		}
	}
	if err := validateJump(userID, 0, f.JumpID); err != nil {
		return err
	}
	if f.ProxyID == nil {
		return nil
	}
	p, err := loadProxy(*f.ProxyID)
	if err != nil || !proxyAccessible(p, userID) {
		return fmt.Errorf("proxy not found")
	}
	if f.JumpID != nil && p.Kind == "wrm_tunnel" {
		return fmt.Errorf("a WRM tunnel proxy runs on the WRM server: it cannot be combined with a jump host")
	}
	if f.JumpID != nil && p.OwnerID != userID && p.HasPassword {
		return fmt.Errorf("proxy %q is shared with a password: it cannot be reached through your own jump hosts", p.Name)
	}
	return nil
}

// updateFolder implements PUT /api/folders/{id}: name and defaults.
func updateFolder(w http.ResponseWriter, r *http.Request, userID, id int) {
	var f Folder
	var raw map[string]json.RawMessage
	if json.NewDecoder(r.Body).Decode(&raw) != nil || json.Unmarshal(jsonMarshal(raw), &f) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	var cur Folder
	db.QueryRow(`SELECT name, parent_id, jump_conn_id, proxy_id FROM folders WHERE id=? AND user_id=?`, id, userID).Scan(&cur.Name, &cur.ParentID, &cur.JumpID, &cur.ProxyID)
	if _, present := raw["parent_id"]; !present {
		f.ParentID = cur.ParentID
	}
	f.ID = id
	f.Name = truncateStr(strings.TrimSpace(f.Name), 120)
	if f.Name == "" {
		jsonError(w, "Name required", 400)
		return
	}
	if err := validateFolderDefaults(userID, &f); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if _, err := db.Exec(`UPDATE folders SET name=?, parent_id=?, jump_conn_id=?, proxy_id=? WHERE id=? AND user_id=?`, f.Name, f.ParentID, f.JumpID, f.ProxyID, id, userID); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	saveVisualLabel(db, "folder", id, userID, f.Label)
	autoSortFolders(userID)
	changed := []string{}
	if f.Name != cur.Name {
		changed = append(changed, "name")
	}
	if !intPtrEq(f.ParentID, cur.ParentID) {
		changed = append(changed, "parent")
	}
	routeChanged := !intPtrEq(f.JumpID, cur.JumpID) || !intPtrEq(f.ProxyID, cur.ProxyID)
	var inheriting []int
	if routeChanged {
		if rows, err := db.Query(`SELECT id FROM connections WHERE folder_id=? AND user_id=? AND (jump_conn_id IS NULL OR proxy_id IS NULL)`, id, userID); err == nil {
			for rows.Next() {
				var cid int
				rows.Scan(&cid)
				inheriting = append(inheriting, cid)
			}
			rows.Close()
		}
		// The connections that inherit the new defaults must still have a valid route.
		if err := validateRoutesOf(userID, inheriting); err != nil {
			db.Exec(`UPDATE folders SET name=?, parent_id=?, jump_conn_id=?, proxy_id=? WHERE id=? AND user_id=?`, cur.Name, cur.ParentID, cur.JumpID, cur.ProxyID, id, userID)
			jsonError(w, "Not saved: the folder defaults would break connection "+err.Error(), 400)
			return
		}
	}
	if !intPtrEq(f.JumpID, cur.JumpID) {
		changed = append(changed, "default_jump")
	}
	if !intPtrEq(f.ProxyID, cur.ProxyID) {
		changed = append(changed, "default_proxy")
	}
	if len(changed) > 0 {
		auditLog(r, userID, "folder.updated", f.Name, map[string]interface{}{"folder_id": id, "changed": changed, "default_jump": f.JumpID, "default_proxy": f.ProxyID})
	}
	if routeChanged {
		// Connections that inherit the defaults now take another route.
		for _, cid := range inheriting {
			tunnelMgr.restartConn(cid)
		}
		statusMon.poke()
	}
	revalidateFolderShares(userID) // the folder may have left a shared subtree
	for _, x := range loadFolders(userID) {
		if x.ID == id {
			jsonOK(w, x)
			return
		}
	}
	jsonOK(w, f)
}
