// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

// LDAP / Active Directory sign-in (v12.6.25).
//
//   - The connection is always encrypted: ldaps:// or ldap:// with StartTLS. Certificates
//     are checked against the system store and an optional CA certificate per directory.
//   - WRM searches with a service account and then binds as the user (the password is never
//     stored). Accounts are keyed by the directory's immutable ID (objectGUID, entryUUID…),
//     never by name.
//   - Groups (nested AD groups through LDAP_MATCHING_RULE_IN_CHAIN, otherwise followed level
//     by level) and attributes map to WRM roles and teams with the same rules as OIDC.
//   - A periodic sync checks every linked account: deleted, disabled, locked or expired
//     accounts and accounts that left all mapped groups lose their access at once.
//
// The service account password is encrypted at rest and never sent to the browser.

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ldapDirectory struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"` // ad | ldap
	URL        string     `json:"url"`
	CAPEM      string     `json:"ca_pem"`
	BindDN     string     `json:"bind_dn"`
	bindPW     string     // decrypted service account password
	HasBindPW  bool       `json:"has_bind_password"`
	BaseDN     string     `json:"base_dn"`
	UserFilter string     `json:"user_filter"`
	LoginAttr  string     `json:"login_attr"`
	IDAttr     string     `json:"id_attr"`
	GroupBase  string     `json:"group_base"`
	Nested     bool       `json:"nested"`
	JIT        bool       `json:"jit"`
	Rules      []oidcRule `json:"rules"`
	Enabled    bool       `json:"enabled"`
	CreatedAt  string     `json:"created_at"`
	UpdatedAt  string     `json:"updated_at"`
	LastSyncAt string     `json:"last_sync_at"`
	LastSync   string     `json:"last_sync"`
}

const ldapDirectoryCols = `id, name, kind, url, ca_pem, bind_dn, bind_password, base_dn, user_filter, login_attr, id_attr, group_base, nested, jit, rules, enabled, created_at, updated_at, last_sync_at, last_sync`

// Defaults per directory type. {user} is replaced by the escaped sign-in name.
var ldapKindDefaults = map[string]struct{ Filter, Login, ID string }{
	"ad":   {"(&(objectCategory=person)(objectClass=user)(|(sAMAccountName={user})(userPrincipalName={user})))", "sAMAccountName", "objectGUID"},
	"ldap": {"(&(objectClass=person)(|(uid={user})(mail={user})))", "uid", "entryUUID"},
}

// ldapDialHook lets tests reach a fake server; nil uses the network.
var ldapDialHook func(network, addr string) (net.Conn, error)

func scanLDAPDirectory(sc interface{ Scan(...interface{}) error }) (*ldapDirectory, error) {
	var d ldapDirectory
	var pw, rules string
	var nested, jit, enabled int
	if err := sc.Scan(&d.ID, &d.Name, &d.Kind, &d.URL, &d.CAPEM, &d.BindDN, &pw, &d.BaseDN, &d.UserFilter, &d.LoginAttr, &d.IDAttr, &d.GroupBase,
		&nested, &jit, &rules, &enabled, &d.CreatedAt, &d.UpdatedAt, &d.LastSyncAt, &d.LastSync); err != nil {
		return nil, err
	}
	if pw != "" {
		d.bindPW = decryptValue(pw)
		d.HasBindPW = true
	}
	d.Nested, d.JIT, d.Enabled = nested == 1, jit == 1, enabled == 1
	json.Unmarshal([]byte(rules), &d.Rules)
	if d.Rules == nil {
		d.Rules = []oidcRule{}
	}
	return &d, nil
}

func loadLDAPDirectory(id int) (*ldapDirectory, error) {
	return scanLDAPDirectory(db.QueryRow(`SELECT `+ldapDirectoryCols+` FROM ldap_directories WHERE id=?`, id))
}

func ldapDirectories(enabledOnly bool) []*ldapDirectory {
	q := `SELECT ` + ldapDirectoryCols + ` FROM ldap_directories`
	if enabledOnly {
		q += ` WHERE enabled=1`
	}
	rows, err := db.Query(q + ` ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*ldapDirectory
	for rows.Next() {
		if d, err := scanLDAPDirectory(rows); err == nil {
			out = append(out, d)
		}
	}
	return out
}

func ldapAnyEnabled() bool {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM ldap_directories WHERE enabled=1`).Scan(&n)
	return n > 0
}

func (d *ldapDirectory) filter() string {
	if d.UserFilter != "" {
		return d.UserFilter
	}
	return ldapKindDefaults[d.Kind].Filter
}

func (d *ldapDirectory) loginAttr() string {
	if d.LoginAttr != "" {
		return d.LoginAttr
	}
	return ldapKindDefaults[d.Kind].Login
}

func (d *ldapDirectory) idAttr() string {
	if d.IDAttr != "" {
		return d.IDAttr
	}
	return ldapKindDefaults[d.Kind].ID
}

func (d *ldapDirectory) groupBase() string {
	if d.GroupBase != "" {
		return d.GroupBase
	}
	return d.BaseDN
}

// connect opens a TLS connection and binds the service account.
func (d *ldapDirectory) connect() (*ldapConn, error) {
	c, err := ldapDial(ldapDialOptions{URL: d.URL, CAPEM: d.CAPEM, DialFunc: ldapDialHook})
	if err != nil {
		return nil, err
	}
	if err := c.Bind(d.BindDN, d.bindPW); err != nil {
		c.Close()
		return nil, fmt.Errorf("service account: %w", err)
	}
	return c, nil
}

// ldapLoginName strips a NetBIOS domain ("EXAMPLE\jdoe" → "jdoe").
func ldapLoginName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, `\`); i >= 0 {
		s = s[i+1:]
	}
	return s
}

func (d *ldapDirectory) userAttrs() []string {
	attrs := []string{d.loginAttr(), d.idAttr(), "displayName", "cn", "mail", "userPrincipalName", "memberOf",
		"userAccountControl", "msDS-User-Account-Control-Computed", "accountExpires", "nsAccountLock", "pwdAccountLockedTime", "loginDisabled"}
	seen := map[string]bool{}
	for _, a := range attrs {
		seen[strings.ToLower(a)] = true
	}
	for _, r := range d.Rules {
		if k := strings.ToLower(r.Claim); k != "group" && k != "groups" && !seen[k] {
			seen[k] = true
			attrs = append(attrs, r.Claim)
		}
	}
	return attrs
}

var errLDAPNoUser = errors.New("no such directory user")

// findUser looks a sign-in name up with the user filter (exactly one entry must match).
func (d *ldapDirectory) findUser(c *ldapConn, login string) (*ldapEntry, error) {
	login = ldapLoginName(login)
	if login == "" || len(login) > 256 {
		return nil, errLDAPNoUser
	}
	f := strings.ReplaceAll(d.filter(), "{user}", ldapEscape(login))
	list, err := c.Search(d.BaseDN, ldapScopeSub, f, d.userAttrs(), 2)
	if err != nil {
		return nil, err
	}
	switch len(list) {
	case 0:
		return nil, errLDAPNoUser
	case 1:
		return list[0], nil
	}
	return nil, fmt.Errorf("the name %q matches more than one directory entry; make the user filter stricter", login)
}

// immutableID returns the directory's stable key of an entry ("" when it has none).
func (d *ldapDirectory) immutableID(e *ldapEntry) string {
	attr := d.idAttr()
	v := e.raw(attr)
	if len(v) == 0 {
		return ""
	}
	if strings.EqualFold(attr, "objectGUID") || strings.EqualFold(attr, "msDS-ConsistencyGuid") {
		if len(v) != 16 {
			return ""
		}
		return "guid:" + hex.EncodeToString(v)
	}
	return strings.ToLower(attr) + ":" + string(v)
}

// findByID finds an entry by its immutable key (for the sync and for renamed accounts).
func (d *ldapDirectory) findByID(c *ldapConn, key string) (*ldapEntry, error) {
	var f string
	switch {
	case strings.HasPrefix(key, "guid:"):
		raw, err := hex.DecodeString(strings.TrimPrefix(key, "guid:"))
		if err != nil {
			return nil, err
		}
		f = "(" + d.idAttr() + "=" + ldapEscapeBytes(raw) + ")"
	default:
		i := strings.IndexByte(key, ':')
		if i < 0 {
			return nil, errors.New("bad directory key")
		}
		f = "(" + key[:i] + "=" + ldapEscape(key[i+1:]) + ")"
	}
	list, err := c.Search(d.BaseDN, ldapScopeSub, f, d.userAttrs(), 2)
	if err != nil {
		return nil, err
	}
	if len(list) != 1 {
		return nil, errLDAPNoUser
	}
	return list[0], nil
}

// accountBlocked explains why a directory account may not sign in ("" when it may).
func ldapAccountBlocked(e *ldapEntry) string {
	if v, err := strconv.ParseInt(e.get("userAccountControl"), 10, 64); err == nil && v&0x2 != 0 {
		return "the account is disabled in the directory"
	}
	if v, err := strconv.ParseInt(e.get("msDS-User-Account-Control-Computed"), 10, 64); err == nil && v&0x10 != 0 {
		return "the account is locked in the directory"
	}
	if v, err := strconv.ParseInt(e.get("accountExpires"), 10, 64); err == nil && v > 0 && v != 0x7FFFFFFFFFFFFFFF {
		// Windows FILETIME: 100 ns since 1601-01-01
		exp := time.Unix((v-116444736000000000)/10000000, 0)
		if time.Now().After(exp) {
			return "the account expired in the directory"
		}
	}
	for _, a := range []string{"nsAccountLock", "loginDisabled"} {
		if strings.EqualFold(e.get(a), "true") {
			return "the account is disabled in the directory"
		}
	}
	if e.get("pwdAccountLockedTime") != "" {
		return "the account is locked in the directory"
	}
	return ""
}

// groups returns the DNs of all groups of a user, nested ones included.
func (d *ldapDirectory) groups(c *ldapConn, e *ldapEntry) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(dn string) bool {
		k := strings.ToLower(dn)
		if dn == "" || seen[k] {
			return false
		}
		seen[k] = true
		out = append(out, dn)
		return true
	}
	if d.Kind == "ad" && d.Nested {
		list, err := c.Search(d.groupBase(), ldapScopeSub, "(&(objectClass=group)(member:1.2.840.113556.1.4.1941:="+ldapEscape(e.DN)+"))", []string{"cn"}, 5000)
		if err != nil {
			return nil, err
		}
		for _, g := range list {
			add(g.DN)
		}
		return out, nil
	}
	for _, dn := range e.all("memberOf") {
		add(dn)
	}
	if len(out) == 0 {
		f := "(|(member=" + ldapEscape(e.DN) + ")(uniqueMember=" + ldapEscape(e.DN) + ")"
		if login := e.get(d.loginAttr()); login != "" {
			f += "(memberUid=" + ldapEscape(login) + ")"
		}
		list, err := c.Search(d.groupBase(), ldapScopeSub, f+")", []string{"cn"}, 5000)
		if err != nil {
			return nil, err
		}
		for _, g := range list {
			add(g.DN)
		}
	}
	if !d.Nested {
		return out, nil
	}
	// Follow groups that are members of other groups, level by level.
	frontier := append([]string(nil), out...)
	for depth := 0; depth < 10 && len(frontier) > 0 && len(out) < 5000; depth++ {
		var next []string
		for _, g := range frontier {
			list, err := c.Search(d.groupBase(), ldapScopeSub, "(|(member="+ldapEscape(g)+")(uniqueMember="+ldapEscape(g)+"))", []string{"cn"}, 5000)
			if err != nil {
				return nil, err
			}
			for _, p := range list {
				if add(p.DN) {
					next = append(next, p.DN)
				}
			}
		}
		frontier = next
	}
	return out, nil
}

// ldapRDNValue is the value of the first RDN ("CN=WRM Admins,OU=…" → "WRM Admins").
func ldapRDNValue(dn string) string {
	first := dn
	for i := 0; i < len(dn); i++ {
		if dn[i] == '\\' {
			i++
			continue
		}
		if dn[i] == ',' {
			first = dn[:i]
			break
		}
	}
	if j := strings.IndexByte(first, '='); j >= 0 {
		first = first[j+1:]
	}
	return strings.ReplaceAll(strings.TrimSpace(first), `\`, "")
}

// claims turns an entry and its groups into the claim map the mapping rules read:
// "group" holds every group's DN and name; every other rule claim is an attribute.
func (d *ldapDirectory) claims(e *ldapEntry, groups []string) oidcClaims {
	c := oidcClaims{}
	var g []interface{}
	for _, dn := range groups {
		g = append(g, dn, ldapRDNValue(dn))
	}
	c["group"], c["groups"] = g, g
	for _, r := range d.Rules {
		k := strings.ToLower(r.Claim)
		if k == "group" || k == "groups" {
			continue
		}
		var vals []interface{}
		for _, v := range e.all(r.Claim) {
			vals = append(vals, v)
		}
		c[r.Claim] = vals
	}
	c["dn"] = e.DN
	c["login"] = e.get(d.loginAttr())
	return c
}

func ldapDisplayName(e *ldapEntry) string {
	for _, a := range []string{"displayName", "cn"} {
		if v := e.get(a); v != "" {
			return v
		}
	}
	return ""
}

func ldapEmail(e *ldapEntry) string {
	if v := e.get("mail"); v != "" {
		return v
	}
	return e.get("userPrincipalName")
}

// ─── identities ──────────────────────────────────────

type ldapIdentity struct {
	ID          int
	UserID      int
	DirectoryID int
	Key         string
	DN          string
	Login       string
	LastOKAt    string
	Revoked     bool
}

func loadLDAPIdentity(where string, args ...interface{}) (*ldapIdentity, error) {
	var x ldapIdentity
	var revoked int
	err := db.QueryRow(`SELECT id, user_id, directory_id, object_key, dn, login, last_ok_at, revoked FROM ldap_identities WHERE `+where, args...).
		Scan(&x.ID, &x.UserID, &x.DirectoryID, &x.Key, &x.DN, &x.Login, &x.LastOKAt, &revoked)
	if err != nil {
		return nil, err
	}
	x.Revoked = revoked == 1
	return &x, nil
}

// userLDAPIdentity is the account's link to an enabled directory (nil when it has none).
func userLDAPIdentity(userID int) *ldapIdentity {
	id, err := loadLDAPIdentity(`user_id=? AND directory_id IN (SELECT id FROM ldap_directories WHERE enabled=1) ORDER BY id LIMIT 1`, userID)
	if err != nil {
		return nil
	}
	return id
}

func ldapSaveIdentity(userID int, d *ldapDirectory, key string, e *ldapEntry) int {
	now := nowRFC()
	db.Exec(`INSERT INTO ldap_identities (user_id, directory_id, object_key, dn, login, email, display, created_at, last_login_at, last_check_at, last_ok_at, last_error, revoked)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,'',0)
		ON CONFLICT(directory_id, object_key) DO UPDATE SET dn=excluded.dn, login=excluded.login, email=excluded.email, display=excluded.display,
		last_login_at=excluded.last_login_at, last_check_at=excluded.last_check_at, last_ok_at=excluded.last_ok_at, last_error='', revoked=0`,
		userID, d.ID, key, truncateStr(e.DN, 500), truncateStr(e.get(d.loginAttr()), 200), truncateStr(ldapEmail(e), 200), truncateStr(ldapDisplayName(e), 100), now, now, now, now)
	var id int
	db.QueryRow(`SELECT id FROM ldap_identities WHERE directory_id=? AND object_key=?`, d.ID, key).Scan(&id)
	return id
}

// ldapUsername picks a free WRM username for a new directory user.
func ldapUsername(d *ldapDirectory, e *ldapEntry, key string) string {
	return oidcUsername(&oidcProvider{UsernameClaim: "preferred_username"}, oidcClaims{
		"preferred_username": e.get(d.loginAttr()), "email": ldapEmail(e), "name": ldapDisplayName(e), "iss": "ldap", "sub": key,
	})
}

// ─── sign-in ─────────────────────────────────────────

type ldapAuthResult struct {
	User     *userRow
	Identity int
	Status   int    // HTTP status when the sign-in is refused
	Reason   string // audit reason
	Message  string // shown to the user
}

// ldapVerify binds as the directory user and checks that the account may sign in.
// It returns the entry, its key and its groups.
func ldapVerify(d *ldapDirectory, login, password string, byKey string) (*ldapEntry, string, []string, error) {
	c, err := d.connect()
	if err != nil {
		return nil, "", nil, err
	}
	defer c.Close()
	var e *ldapEntry
	if byKey != "" {
		e, err = d.findByID(c, byKey)
	} else {
		e, err = d.findUser(c, login)
	}
	if err != nil {
		return nil, "", nil, err
	}
	key := d.immutableID(e)
	if key == "" {
		return nil, "", nil, fmt.Errorf("the entry has no %s attribute (the immutable ID WRM uses); set the ID attribute of this directory", d.idAttr())
	}
	// The user bind runs on its own connection, so the service account's searches stay theirs.
	uc, err := ldapDial(ldapDialOptions{URL: d.URL, CAPEM: d.CAPEM, DialFunc: ldapDialHook})
	if err != nil {
		return nil, "", nil, err
	}
	err = uc.Bind(e.DN, password)
	uc.Close()
	if err != nil {
		return e, key, nil, err
	}
	groups, err := d.groups(c, e)
	if err != nil {
		return e, key, nil, fmt.Errorf("groups: %w", err)
	}
	return e, key, groups, nil
}

// ldapErrorSummary describes a directory error with fixed text only (no server message,
// no names), for logs and audit entries written during a sign-in.
func ldapErrorSummary(err error) string {
	var le *ldapError
	var ne net.Error
	msg := err.Error()
	switch {
	case errors.As(err, &le):
		switch le.Code {
		case 32:
			return "LDAP error: no such object"
		case 49:
			return "LDAP error: invalid credentials"
		case 50:
			return "LDAP error: insufficient access rights"
		case 51, 52:
			return "LDAP error: server busy or unavailable"
		case 53:
			return "LDAP error: unwilling to perform"
		}
		return "LDAP error"
	case errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	case strings.Contains(msg, "x509") || strings.Contains(msg, "certificate"):
		return "TLS certificate not trusted"
	case strings.Contains(msg, "connection refused"):
		return "connection refused"
	case strings.Contains(msg, "no such host"):
		return "host not found"
	case errors.Is(err, io.EOF) || strings.Contains(msg, "connection reset"):
		return "connection closed by the server"
	}
	return "connection or protocol error"
}

func ldapInvalidCredentials(err error) bool {
	var le *ldapError
	return errors.As(err, &le) && le.Code == ldapResultInvalidCrd
}

// ldapSignIn handles a password sign-in for a directory account. linked is the account the
// name belongs to in WRM (nil when the name is not a WRM user with a directory link).
func ldapSignIn(r *http.Request, typed, password string, linked *userRow) (*ldapAuthResult, bool) {
	refuse := func(status int, reason, msg string) (*ldapAuthResult, bool) {
		return &ldapAuthResult{Status: status, Reason: reason, Message: msg}, true
	}
	var dirs []*ldapDirectory
	var ident *ldapIdentity
	if linked != nil {
		ident = userLDAPIdentity(linked.ID)
		if ident == nil {
			return nil, false
		}
		d, err := loadLDAPDirectory(ident.DirectoryID)
		if err != nil {
			return nil, false
		}
		dirs = []*ldapDirectory{d}
	} else {
		dirs = ldapDirectories(true)
	}
	for _, d := range dirs {
		byKey := ""
		if ident != nil {
			byKey = ident.Key
		}
		e, key, groups, err := ldapVerify(d, typed, password, byKey)
		if errors.Is(err, errLDAPNoUser) && ident == nil {
			continue
		}
		if err != nil {
			if ldapInvalidCredentials(err) {
				return refuse(401, "wrong directory credentials", "Invalid credentials")
			}
			if errors.Is(err, errLDAPNoUser) {
				return refuse(401, "directory account not found", "Invalid credentials")
			}
			// the sign-in error can carry the typed name, so only its kind is logged and audited
			kind := ldapErrorSummary(err)
			log.Printf("LDAP %s: sign-in failed: %s", d.Name, kind)
			return refuse(502, "directory unreachable: "+kind, "WRM could not reach the directory "+d.Name+". Try again later or ask your administrator.")
		}
		if why := ldapAccountBlocked(e); why != "" {
			return refuse(403, why, "Your directory account may not sign in ("+why+").")
		}
		claims := d.claims(e, groups)
		m := directoryMap(d.Rules, claims)
		hasRules := len(d.Rules) > 0
		existing, idErr := loadLDAPIdentity(`directory_id=? AND object_key=?`, d.ID, key)
		if hasRules && !m.Matched {
			if idErr == nil {
				endUserAccess(r, existing.UserID, 0, "the directory account is no longer in a group that may use WRM")
			}
			return refuse(403, "no mapping rule matched", "Your directory account is not in a group that may use WRM. Ask your administrator.")
		}
		var userID int
		switch {
		case idErr == nil:
			userID = existing.UserID
		case !d.JIT || !hasRules:
			return refuse(403, "no linked account", "There is no WRM account for your directory account yet. Ask your administrator to create one or to allow automatic accounts.")
		default:
			username := ldapUsername(d, e, key)
			now := nowRFC()
			res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, role, display_name, created_at, password_changed_at, sso_only) VALUES (?,?,?,?,?,?,?,1)`,
				username, "!sso", boolToInt(m.Role == userRoleAdmin), storedRole(m.Role), truncateStr(ldapDisplayName(e), 100), now, now)
			if err != nil {
				return refuse(500, "create account: "+err.Error(), "WRM could not create your account. Ask your administrator.")
			}
			nid, _ := res.LastInsertId()
			userID = int(nid)
			auditLogAs(r, userID, username, "ldap.account_created", username, map[string]string{"directory": d.Name, "role": m.Role, "dn": e.DN})
		}
		u, err := loadUser("id=?", userID)
		if err != nil {
			return refuse(403, "account missing", "Your WRM account no longer exists. Ask your administrator.")
		}
		if u.BreakGlass {
			return refuse(403, "break-glass", "A break-glass account must use its local WRM sign-in.")
		}
		if hasRules {
			directoryApplyMapping(r, userID, m, "ldap")
		}
		idID := ldapSaveIdentity(userID, d, key, e)
		u, _ = loadUser("id=?", userID)
		return &ldapAuthResult{User: u, Identity: idID}, true
	}
	return nil, false
}

// ldapFinishLogin answers a directory password sign-in.
func ldapFinishLogin(w http.ResponseWriter, r *http.Request, ip, typed string, linked *userRow, res *ldapAuthResult) {
	if res.User == nil {
		uid, name := 0, typed
		if linked != nil {
			uid, name = linked.ID, linked.Username
			if res.Status == 401 {
				fails, locked := linked.FailedLogins+1, ""
				if fails >= settingInt("login_max_failures") {
					locked, fails = time.Now().Add(15*time.Minute).UTC().Format(time.RFC3339), 0
					auditLogAs(r, linked.ID, linked.Username, "auth.account_locked", "", map[string]string{"minutes": "15"})
				}
				db.Exec(`UPDATE users SET failed_logins=?, locked_until=? WHERE id=?`, fails, locked, linked.ID)
			}
		}
		if res.Status == 401 {
			recordLoginResult(ip, false)
		}
		auditLogAs(r, uid, name, "auth.login_failed", "", map[string]string{"method": "ldap", "reason": res.Reason})
		jsonError(w, res.Message, res.Status)
		return
	}
	u := res.User
	if u.Disabled {
		auditLogAs(r, u.ID, u.Username, "auth.login_failed", "", map[string]string{"method": "ldap", "reason": "account disabled"})
		jsonError(w, "This account is disabled. Contact an administrator.", 403)
		return
	}
	recordLoginResult(ip, true)
	if u.TOTPEnabled {
		jsonOK(w, map[string]interface{}{"mfa_required": true, "mfa_token": newMFAToken(u.ID)})
		return
	}
	finishLogin(w, r, u, "ldap")
}

// ldapCheckPassword confirms a linked account's directory password (step-up, 2FA changes).
func ldapCheckPassword(userID int, password string) bool {
	ident := userLDAPIdentity(userID)
	if ident == nil || ident.Revoked || password == "" {
		return false
	}
	d, err := loadLDAPDirectory(ident.DirectoryID)
	if err != nil {
		return false
	}
	e, _, _, err := ldapVerify(d, "", password, ident.Key)
	return err == nil && ldapAccountBlocked(e) == ""
}

// userCheckPassword checks the password an account signs in with: the directory's for a
// linked directory account, otherwise the local one.
func userCheckPassword(u *userRow, password string) bool {
	if userLDAPIdentity(u.ID) != nil {
		return ldapCheckPassword(u.ID, password)
	}
	return checkPassword(u.Hash, password)
}

// userHasPassword: the account can confirm with a password (local or directory).
func userHasPassword(userID int) bool {
	return !userSSOOnly(userID) || userLDAPIdentity(userID) != nil
}

// ─── sync ────────────────────────────────────────────

var ldapSyncMu sync.Mutex

func ldapSyncLoop() {
	for {
		time.Sleep(time.Minute)
		every := time.Duration(settingInt("ldap_sync_minutes")) * time.Minute
		for _, d := range ldapDirectories(true) {
			if t, ok := parseTS(d.LastSyncAt); ok && time.Since(t) < every {
				continue
			}
			ldapSync(nil, d)
		}
	}
}

type ldapSyncReport struct {
	Checked int      `json:"checked"`
	Ended   []string `json:"ended"`
	Updated int      `json:"updated"`
	Error   string   `json:"error,omitempty"`
}

// ldapSync checks every account linked to a directory.
func ldapSync(r *http.Request, d *ldapDirectory) ldapSyncReport {
	ldapSyncMu.Lock()
	defer ldapSyncMu.Unlock()
	rep := ldapSyncReport{Ended: []string{}}
	rows, err := db.Query(`SELECT id FROM ldap_identities WHERE directory_id=? AND revoked=0`, d.ID)
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	now := nowRFC()
	finish := func() ldapSyncReport {
		summary := fmt.Sprintf("%d checked, %d ended", rep.Checked, len(rep.Ended))
		if rep.Error != "" {
			summary = "error: " + truncateStr(rep.Error, 250)
		}
		db.Exec(`UPDATE ldap_directories SET last_sync_at=?, last_sync=? WHERE id=?`, now, summary, d.ID)
		return rep
	}
	end := func(id *ldapIdentity, reason string) {
		username := usernameOf(id.UserID)
		endUserAccess(r, id.UserID, 0, "your directory account may no longer use WRM ("+reason+")")
		db.Exec(`UPDATE ldap_identities SET revoked=1, last_check_at=?, last_error=? WHERE id=?`, now, truncateStr(reason, 300), id.ID)
		auditLogAs(r, id.UserID, username, "ldap.access_ended", username, map[string]string{"directory": d.Name, "reason": reason})
		rep.Ended = append(rep.Ended, username)
	}
	if len(ids) == 0 {
		return finish()
	}
	c, err := d.connect()
	if err != nil {
		rep.Error = err.Error()
		grace := time.Duration(settingInt("oidc_unverified_hours")) * time.Hour
		for _, iid := range ids {
			id, err := loadLDAPIdentity(`id=?`, iid)
			if err != nil {
				continue
			}
			db.Exec(`UPDATE ldap_identities SET last_check_at=?, last_error=? WHERE id=?`, now, truncateStr(err.Error(), 300), id.ID)
			if t, ok := parseTS(id.LastOKAt); (!ok || time.Since(t) > grace) && userHasSessions(id.UserID) {
				end(id, "not verified with the directory for longer than oidc_unverified_hours")
			}
		}
		return finish()
	}
	defer c.Close()
	for _, iid := range ids {
		id, err := loadLDAPIdentity(`id=?`, iid)
		if err != nil {
			continue
		}
		rep.Checked++
		e, err := d.findByID(c, id.Key)
		if errors.Is(err, errLDAPNoUser) {
			end(id, "the account no longer exists in the directory")
			continue
		}
		if err != nil {
			db.Exec(`UPDATE ldap_identities SET last_check_at=?, last_error=? WHERE id=?`, now, truncateStr(err.Error(), 300), id.ID)
			continue
		}
		if why := ldapAccountBlocked(e); why != "" {
			end(id, why)
			continue
		}
		if len(d.Rules) > 0 {
			groups, err := d.groups(c, e)
			if err != nil {
				db.Exec(`UPDATE ldap_identities SET last_check_at=?, last_error=? WHERE id=?`, now, truncateStr(err.Error(), 300), id.ID)
				continue
			}
			m := directoryMap(d.Rules, d.claims(e, groups))
			if !m.Matched {
				end(id, "the account is no longer in a group that may use WRM")
				continue
			}
			directoryApplyMapping(r, id.UserID, m, "ldap")
		}
		db.Exec(`UPDATE ldap_identities SET dn=?, login=?, email=?, display=?, last_check_at=?, last_ok_at=?, last_error='' WHERE id=?`,
			truncateStr(e.DN, 500), truncateStr(e.get(d.loginAttr()), 200), truncateStr(ldapEmail(e), 200), truncateStr(ldapDisplayName(e), 100), now, now, id.ID)
		rep.Updated++
	}
	return finish()
}

func userHasSessions(userID int) bool {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM auth_sessions WHERE user_id=?`, userID).Scan(&n)
	return n > 0
}

// ─── admin API ───────────────────────────────────────

func ldapDirectoryJSON(d *ldapDirectory) map[string]interface{} {
	var linked int
	db.QueryRow(`SELECT COUNT(*) FROM ldap_identities WHERE directory_id=?`, d.ID).Scan(&linked)
	def := ldapKindDefaults[d.Kind]
	return map[string]interface{}{
		"id": d.ID, "name": d.Name, "kind": d.Kind, "url": d.URL, "ca_pem": d.CAPEM, "bind_dn": d.BindDN, "has_bind_password": d.HasBindPW,
		"base_dn": d.BaseDN, "user_filter": d.UserFilter, "login_attr": d.LoginAttr, "id_attr": d.IDAttr, "group_base": d.GroupBase,
		"nested": d.Nested, "jit": d.JIT, "rules": d.Rules, "enabled": d.Enabled, "linked_users": linked,
		"default_filter": def.Filter, "default_login_attr": def.Login, "default_id_attr": def.ID,
		"created_at": d.CreatedAt, "updated_at": d.UpdatedAt, "last_sync_at": d.LastSyncAt, "last_sync": d.LastSync,
	}
}

type ldapDirectoryInput struct {
	Name          *string     `json:"name"`
	Kind          *string     `json:"kind"`
	URL           *string     `json:"url"`
	CAPEM         *string     `json:"ca_pem"`
	BindDN        *string     `json:"bind_dn"`
	BindPassword  *string     `json:"bind_password"` // "" keeps the stored one
	BaseDN        *string     `json:"base_dn"`
	UserFilter    *string     `json:"user_filter"`
	LoginAttr     *string     `json:"login_attr"`
	IDAttr        *string     `json:"id_attr"`
	GroupBase     *string     `json:"group_base"`
	Nested        *bool       `json:"nested"`
	JIT           *bool       `json:"jit"`
	Rules         *[]oidcRule `json:"rules"`
	Enabled       *bool       `json:"enabled"`
	Directory     int         `json:"directory"` // check: use the stored password of this directory
	TestLogin     string      `json:"username"`
	TestPassword  string      `json:"password"`
	ClearBindPass bool        `json:"clear_bind_password"`
}

func (in *ldapDirectoryInput) apply(d *ldapDirectory) error {
	oldURL, hadPW := d.URL, d.bindPW != ""
	set := func(dst *string, v *string, max int) {
		if v != nil {
			*dst = truncateStr(strings.TrimSpace(*v), max)
		}
	}
	set(&d.Name, in.Name, 60)
	set(&d.Kind, in.Kind, 10)
	set(&d.URL, in.URL, 300)
	if in.CAPEM != nil {
		d.CAPEM = truncateStr(strings.TrimSpace(*in.CAPEM), 64<<10)
	}
	set(&d.BindDN, in.BindDN, 500)
	set(&d.BaseDN, in.BaseDN, 500)
	set(&d.UserFilter, in.UserFilter, 1000)
	set(&d.LoginAttr, in.LoginAttr, 60)
	set(&d.IDAttr, in.IDAttr, 60)
	set(&d.GroupBase, in.GroupBase, 500)
	if in.BindPassword != nil && *in.BindPassword != "" {
		d.bindPW = *in.BindPassword
	}
	if in.ClearBindPass {
		d.bindPW = ""
	}
	if in.Nested != nil {
		d.Nested = *in.Nested
	}
	if in.JIT != nil {
		d.JIT = *in.JIT
	}
	if in.Enabled != nil {
		d.Enabled = *in.Enabled
	}
	if in.Rules != nil {
		rules, err := validateOIDCRules(*in.Rules)
		if err != nil {
			return err
		}
		d.Rules = rules
	}
	// The stored service password only goes to the server it was entered for: a changed
	// address needs the password again.
	if hadPW && d.ID > 0 && !ldapSameServer(oldURL, d.URL) && (in.BindPassword == nil || *in.BindPassword == "") {
		return errors.New("enter the service account password again when you change the server address")
	}
	if d.Kind != "ad" && d.Kind != "ldap" {
		return errors.New("choose the directory type: Active Directory (ad) or another LDAP server (ldap)")
	}
	if _, _, _, err := ldapParseURL(d.URL); err != nil {
		return err
	}
	if d.UserFilter != "" {
		if !strings.Contains(d.UserFilter, "{user}") {
			return errors.New("the user filter must contain {user} where the sign-in name goes")
		}
		if _, err := ldapCompileFilter(strings.ReplaceAll(d.UserFilter, "{user}", "x")); err != nil {
			return fmt.Errorf("user filter: %w", err)
		}
	}
	for _, a := range []string{d.LoginAttr, d.IDAttr} {
		if a != "" && !ldapAttrName(a) {
			return fmt.Errorf("%q is not an attribute name", a)
		}
	}
	return nil
}

func ldapSameServer(a, b string) bool {
	ha, pa, ta, err1 := ldapParseURL(a)
	hb, pb, tb, err2 := ldapParseURL(b)
	return err1 == nil && err2 == nil && strings.EqualFold(ha, hb) && pa == pb && ta == tb
}

func ldapAttrName(s string) bool {
	if s == "" || len(s) > 60 {
		return false
	}
	for i, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && (c >= '0' && c <= '9' || c == '-' || c == ';')) {
			return false
		}
	}
	return true
}

// GET    /api/admin/ldap                  directories and linked accounts
// POST   /api/admin/ldap                  create a directory
// PUT    /api/admin/ldap/{id}             change a directory
// DELETE /api/admin/ldap/{id}             remove a directory and its links
// POST   /api/admin/ldap/check            step-by-step connection test (unsaved values allowed)
// POST   /api/admin/ldap/{id}/test        {username, password} what WRM would do (creates nothing)
// POST   /api/admin/ldap/{id}/sync        check every linked account now
// DELETE /api/admin/ldap/identities/{id}  unlink one account
func apiAdminLDAPHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/ldap"), "/")
	parts := strings.Split(rest, "/")
	decode := func() (*ldapDirectoryInput, bool) {
		var in ldapDirectoryInput
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
			jsonError(w, "Bad JSON", 400)
			return nil, false
		}
		return &in, true
	}
	switch {
	case rest == "" && r.Method == http.MethodGet:
		list := []map[string]interface{}{}
		for _, d := range ldapDirectories(false) {
			list = append(list, ldapDirectoryJSON(d))
		}
		jsonOK(w, map[string]interface{}{"directories": list, "identities": ldapIdentityList(0), "sync_minutes": settingInt("ldap_sync_minutes")})
	case rest == "check" && r.Method == http.MethodPost:
		in, ok := decode()
		if !ok {
			return
		}
		d := &ldapDirectory{Kind: "ad", Nested: true}
		if in.Directory > 0 {
			stored, err := loadLDAPDirectory(in.Directory)
			if err != nil {
				jsonError(w, "Not found", 404)
				return
			}
			d = stored
		}
		if err := in.apply(d); err != nil {
			jsonOK(w, map[string]interface{}{"ok": false, "steps": []oidcCheckStep{{Step: "settings", OK: false, Detail: err.Error()}}})
			return
		}
		jsonOK(w, ldapCheck(d, in.TestLogin))
	case rest == "" && r.Method == http.MethodPost:
		if !requireStepUp(w, r, "ldap.directory") {
			return
		}
		in, ok := decode()
		if !ok {
			return
		}
		saveLDAPDirectory(w, r, adminID, &ldapDirectory{Kind: "ad", Nested: true, JIT: true, Rules: []oidcRule{}}, in)
	case len(parts) == 2 && parts[0] == "identities" && r.Method == http.MethodDelete:
		if !requireStepUp(w, r, "ldap.unlink") {
			return
		}
		id, err := loadLDAPIdentity(`id=?`, atoiOr(parts[1], 0))
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		ldapUnlink(r, adminID, id)
		jsonOK(w, map[string]bool{"ok": true})
	case len(parts) == 2 && parts[1] == "test" && r.Method == http.MethodPost:
		d, err := loadLDAPDirectory(atoiOr(parts[0], 0))
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		in, ok := decode()
		if !ok {
			return
		}
		if in.Rules != nil {
			rules, err := validateOIDCRules(*in.Rules)
			if err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
			d.Rules = rules
		}
		jsonOK(w, ldapTestUser(r, adminID, d, in.TestLogin, in.TestPassword))
	case len(parts) == 2 && parts[1] == "sync" && r.Method == http.MethodPost:
		d, err := loadLDAPDirectory(atoiOr(parts[0], 0))
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		rep := ldapSync(r, d)
		auditLog(r, adminID, "ldap.synced", d.Name, map[string]interface{}{"checked": rep.Checked, "ended": len(rep.Ended), "failed": rep.Error != ""})
		jsonOK(w, rep)
	case len(parts) == 1 && rest != "" && r.Method == http.MethodPut:
		if !requireStepUp(w, r, "ldap.directory") {
			return
		}
		d, err := loadLDAPDirectory(atoiOr(parts[0], 0))
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		in, ok := decode()
		if !ok {
			return
		}
		saveLDAPDirectory(w, r, adminID, d, in)
	case len(parts) == 1 && rest != "" && r.Method == http.MethodDelete:
		if !requireStepUp(w, r, "ldap.directory") {
			return
		}
		d, err := loadLDAPDirectory(atoiOr(parts[0], 0))
		if err != nil {
			jsonError(w, "Not found", 404)
			return
		}
		rows, _ := db.Query(`SELECT id FROM ldap_identities WHERE directory_id=?`, d.ID)
		var ids []int
		if rows != nil {
			for rows.Next() {
				var id int
				rows.Scan(&id)
				ids = append(ids, id)
			}
			rows.Close()
		}
		for _, iid := range ids {
			if id, err := loadLDAPIdentity(`id=?`, iid); err == nil {
				ldapUnlink(r, adminID, id)
			}
		}
		db.Exec(`DELETE FROM ldap_directories WHERE id=?`, d.ID)
		auditLog(r, adminID, "ldap.directory_deleted", d.Name, nil)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Not found", 404)
	}
}

func saveLDAPDirectory(w http.ResponseWriter, r *http.Request, adminID int, d *ldapDirectory, in *ldapDirectoryInput) {
	oldPW := d.bindPW
	if err := in.apply(d); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if d.Name == "" {
		jsonError(w, "Enter a name", 400)
		return
	}
	if d.BaseDN == "" {
		jsonError(w, "Enter the base DN (where WRM searches for users)", 400)
		return
	}
	if d.Enabled {
		if d.BindDN == "" || d.bindPW == "" {
			jsonError(w, "Enter the service account (bind DN and password) before turning the directory on", 400)
			return
		}
		c, err := d.connect()
		if err != nil {
			jsonError(w, "WRM cannot sign in to the directory with the service account, so it stays off: "+err.Error(), 400)
			return
		}
		c.Close()
	}
	enc := ""
	if d.bindPW != "" {
		enc = encryptValue(d.bindPW)
	}
	rules, _ := json.Marshal(d.Rules)
	now := nowRFC()
	if d.ID == 0 {
		res, err := db.Exec(`INSERT INTO ldap_directories (name, kind, url, ca_pem, bind_dn, bind_password, base_dn, user_filter, login_attr, id_attr, group_base, nested, jit, rules, enabled, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, d.Name, d.Kind, d.URL, d.CAPEM, d.BindDN, enc, d.BaseDN, d.UserFilter, d.LoginAttr, d.IDAttr, d.GroupBase,
			boolToInt(d.Nested), boolToInt(d.JIT), string(rules), boolToInt(d.Enabled), now, now)
		if err != nil {
			jsonError(w, "A directory with this name already exists", 400)
			return
		}
		nid, _ := res.LastInsertId()
		d.ID = int(nid)
		auditLog(r, adminID, "ldap.directory_created", d.Name, map[string]interface{}{"url": d.URL, "enabled": d.Enabled, "rules": len(d.Rules)})
	} else {
		if _, err := db.Exec(`UPDATE ldap_directories SET name=?, kind=?, url=?, ca_pem=?, bind_dn=?, bind_password=?, base_dn=?, user_filter=?, login_attr=?, id_attr=?, group_base=?, nested=?, jit=?, rules=?, enabled=?, updated_at=? WHERE id=?`,
			d.Name, d.Kind, d.URL, d.CAPEM, d.BindDN, enc, d.BaseDN, d.UserFilter, d.LoginAttr, d.IDAttr, d.GroupBase, boolToInt(d.Nested), boolToInt(d.JIT), string(rules), boolToInt(d.Enabled), now, d.ID); err != nil {
			jsonError(w, "A directory with this name already exists", 400)
			return
		}
		auditLog(r, adminID, "ldap.directory_updated", d.Name, map[string]interface{}{"url": d.URL, "enabled": d.Enabled, "rules": len(d.Rules),
			"cred_changed": subtle.ConstantTimeCompare([]byte(oldPW), []byte(d.bindPW)) != 1})
	}
	saved, _ := loadLDAPDirectory(d.ID)
	if saved == nil {
		jsonError(w, "Server error", 500)
		return
	}
	jsonOK(w, ldapDirectoryJSON(saved))
}

// ldapUnlink removes a directory link and ends the account's access through it.
func ldapUnlink(r *http.Request, adminID int, id *ldapIdentity) {
	if userSSOOnly(id.UserID) {
		endUserAccess(r, id.UserID, adminID, "your directory sign-in was removed")
	}
	db.Exec(`DELETE FROM ldap_identities WHERE id=?`, id.ID)
	auditLog(r, adminID, "ldap.identity_unlinked", usernameOf(id.UserID), map[string]string{"dn": id.DN})
}

func ldapIdentityList(userID int) []map[string]interface{} {
	q := `SELECT i.id, i.user_id, COALESCE(u.username,''), i.directory_id, COALESCE(d.name,''), i.dn, i.login, i.email, i.display,
		i.last_login_at, i.last_check_at, i.last_ok_at, i.last_error, i.revoked FROM ldap_identities i
		LEFT JOIN users u ON u.id=i.user_id LEFT JOIN ldap_directories d ON d.id=i.directory_id`
	args := []interface{}{}
	if userID > 0 {
		q += ` WHERE i.user_id=?`
		args = append(args, userID)
	}
	out := []map[string]interface{}{}
	rows, err := db.Query(q+` ORDER BY u.username`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, uid, did, revoked int
		var username, dname, dn, login, email, display, lastLogin, lastCheck, lastOK, lastErr string
		if rows.Scan(&id, &uid, &username, &did, &dname, &dn, &login, &email, &display, &lastLogin, &lastCheck, &lastOK, &lastErr, &revoked) != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"id": id, "user_id": uid, "username": username, "directory_id": did, "directory": dname, "dn": dn, "login": login, "email": email,
			"display": display, "last_login_at": lastLogin, "last_check_at": lastCheck, "last_ok_at": lastOK, "last_error": lastErr, "revoked": revoked == 1,
		})
	}
	return out
}

// ldapCheck tests a directory without a user password: address, TLS, service account,
// base DN, user filter and groups. With a sign-in name it also looks that user up.
func ldapCheck(d *ldapDirectory, login string) map[string]interface{} {
	steps := []oidcCheckStep{}
	add := func(step string, ok bool, detail, fix string) {
		steps = append(steps, oidcCheckStep{step, ok, detail, fix})
	}
	done := func() map[string]interface{} {
		ok := true
		for _, s := range steps {
			ok = ok && s.OK
		}
		return map[string]interface{}{"ok": ok, "steps": steps}
	}
	host, port, implicitTLS, _ := ldapParseURL(d.URL)
	mode := "StartTLS"
	if implicitTLS {
		mode = "LDAPS"
	}
	add("address", true, fmt.Sprintf("%s port %s, %s.", host, port, mode), "")
	c, err := ldapDial(ldapDialOptions{URL: d.URL, CAPEM: d.CAPEM, DialFunc: ldapDialHook})
	if err != nil {
		fix := "WRM must reach " + net.JoinHostPort(host, port) + " (DNS, firewall, the directory's LDAPS/StartTLS settings)."
		if strings.Contains(err.Error(), "x509") || strings.Contains(err.Error(), "certificate") {
			fix = "The directory's certificate is not trusted or does not name " + host + ". Paste the issuing CA certificate (PEM) and use the host name the certificate is issued to."
		}
		add("connect", false, err.Error(), fix)
		return done()
	}
	detail := "Encrypted connection."
	if c.TLS != nil && len(c.TLS.PeerCertificates) > 0 {
		cert := c.TLS.PeerCertificates[0]
		detail = fmt.Sprintf("Encrypted connection (TLS %s), certificate %s, valid until %s.", tlsVersionName(c.TLS.Version), cert.Subject.CommonName, cert.NotAfter.Format("2006-01-02"))
	}
	add("tls", true, detail, "")
	if d.BindDN == "" || d.bindPW == "" {
		c.Close()
		add("service", false, "No service account.", "Enter the DN and password of a read-only account that may search users and groups.")
		return done()
	}
	if err := c.Bind(d.BindDN, d.bindPW); err != nil {
		c.Close()
		add("service", false, err.Error(), "Check the service account's DN (for AD also user@domain works) and password, and that it is not locked or expired.")
		return done()
	}
	add("service", true, "Signed in as "+d.BindDN+".", "")
	defer c.Close()
	if _, err := c.Search(d.BaseDN, ldapScopeBase, "(objectClass=*)", []string{"objectClass"}, 1); err != nil {
		add("base", false, err.Error(), "The base DN must exist and be readable by the service account, for example DC=example,DC=com.")
		return done()
	}
	add("base", true, "Base DN "+d.BaseDN+" is readable.", "")
	sample := strings.ReplaceAll(d.filter(), "{user}", "*")
	users, err := c.Search(d.BaseDN, ldapScopeSub, sample, []string{d.loginAttr()}, 5)
	switch {
	case err != nil:
		add("users", false, err.Error(), "Check the user filter.")
	case len(users) == 0:
		add("users", false, "The user filter finds no users under the base DN.", "Check the base DN and the user filter.")
	default:
		var names []string
		for _, u := range users {
			names = append(names, u.get(d.loginAttr()))
		}
		add("users", true, "Found users, for example: "+strings.Join(names, ", ")+".", "")
	}
	if login != "" {
		e, err := d.findUser(c, login)
		if err != nil {
			add("user", false, err.Error(), "Check the sign-in name and the user filter.")
			return done()
		}
		key := d.immutableID(e)
		if key == "" {
			add("user", false, "Found "+e.DN+" but it has no "+d.idAttr()+".", "Set the ID attribute (objectGUID for AD, entryUUID for OpenLDAP, nsUniqueId for 389 DS, ipaUniqueID for FreeIPA).")
			return done()
		}
		add("user", true, "Found "+e.DN+" (immutable ID "+key+").", "")
		groups, err := d.groups(c, e)
		if err != nil {
			add("groups", false, err.Error(), "The service account must be able to read group memberships.")
			return done()
		}
		var names []string
		for _, g := range groups {
			names = append(names, ldapRDNValue(g))
		}
		add("groups", true, fmt.Sprintf("%d group(s): %s", len(groups), truncateStr(strings.Join(names, ", "), 500)), "")
	}
	return done()
}

// ldapTestUser shows what a sign-in would do for one user; it creates nothing.
func ldapTestUser(r *http.Request, adminID int, d *ldapDirectory, login, password string) map[string]interface{} {
	auditLog(r, adminID, "ldap.test_sign_in", d.Name, map[string]string{"login": truncateStr(login, 100)})
	out := map[string]interface{}{"ok": false}
	e, key, groups, err := ldapVerify(d, login, password, "")
	if err != nil {
		msg := err.Error()
		if ldapInvalidCredentials(err) {
			msg = "The directory refused the password."
		} else if errors.Is(err, errLDAPNoUser) {
			msg = "The user filter finds no such user."
		}
		out["error"] = msg
		return out
	}
	claims := d.claims(e, groups)
	m := directoryMap(d.Rules, claims)
	var gnames []string
	for _, g := range groups {
		gnames = append(gnames, ldapRDNValue(g))
	}
	attrs := map[string]interface{}{}
	for k, v := range e.Attrs {
		if k == "objectguid" || k == "objectsid" || k == "msds-consistencyguid" {
			continue
		}
		var vals []string
		for _, x := range v {
			vals = append(vals, truncateStr(string(x), 300))
		}
		attrs[k] = vals
	}
	var linkedUser string
	if id, err := loadLDAPIdentity(`directory_id=? AND object_key=?`, d.ID, key); err == nil {
		linkedUser = usernameOf(id.UserID)
	}
	out["ok"] = true
	out["dn"], out["key"], out["groups"], out["group_dns"], out["attributes"] = e.DN, key, gnames, groups, attrs
	out["blocked"], out["mapping"], out["linked_user"] = ldapAccountBlocked(e), m, linkedUser
	return out
}

func tlsVersionName(v uint16) string {
	switch v {
	case 0x0304:
		return "1.3"
	case 0x0303:
		return "1.2"
	}
	return "0x" + strconv.FormatUint(uint64(v), 16)
}

// ─── self-service link ───────────────────────────────

// POST /api/me/ldap {directory, username, password}  link the signed-in account
// DELETE /api/me/ldap/{id}                           unlink (accounts with a local password only)
func apiMeLDAPHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/me/ldap"), "/")
	switch {
	case rest == "" && r.Method == http.MethodGet:
		dirs := []map[string]interface{}{}
		for _, d := range ldapDirectories(true) {
			dirs = append(dirs, map[string]interface{}{"id": d.ID, "name": d.Name})
		}
		jsonOK(w, map[string]interface{}{"identities": ldapIdentityList(userID), "directories": dirs})
	case rest == "" && r.Method == http.MethodPost:
		var in struct {
			Directory int    `json:"directory"`
			Username  string `json:"username"`
			Password  string `json:"password"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		u, err := loadUser("id=?", userID)
		if err != nil || u.BreakGlass {
			jsonError(w, "A break-glass account stays local; it cannot be linked to a directory account", 400)
			return
		}
		d, err := loadLDAPDirectory(in.Directory)
		if err != nil || !d.Enabled {
			jsonError(w, "Unknown or disabled directory", 404)
			return
		}
		if _, err := loadLDAPIdentity(`user_id=? AND directory_id=?`, userID, d.ID); err == nil {
			jsonError(w, "Your account is already linked to this directory; unlink it first", 400)
			return
		}
		// Linking adds a way to sign in, so a stolen session must not be enough.
		if !requireStepUpAlways(w, r, "ldap.link") {
			return
		}
		ip := clientIP(r)
		if blocked, wait := loginBlocked(ip); blocked {
			jsonError(w, fmt.Sprintf("Too many failed attempts. Try again in %d s.", int(wait.Seconds())+1), 429)
			return
		}
		e, key, _, err := ldapVerify(d, in.Username, in.Password, "")
		if err != nil {
			recordLoginResult(ip, false)
			if ldapInvalidCredentials(err) || errors.Is(err, errLDAPNoUser) {
				jsonError(w, "The directory did not accept this name and password", 403)
				return
			}
			jsonError(w, "WRM could not check the directory: "+err.Error(), 502)
			return
		}
		if why := ldapAccountBlocked(e); why != "" {
			jsonError(w, "This directory account may not sign in: "+why, 403)
			return
		}
		if other, err := loadLDAPIdentity(`directory_id=? AND object_key=?`, d.ID, key); err == nil && other.UserID != userID {
			jsonError(w, "This directory account is already linked to another WRM account ("+usernameOf(other.UserID)+")", 409)
			return
		}
		recordLoginResult(ip, true)
		ldapSaveIdentity(userID, d, key, e)
		// From now on the account signs in with its directory password.
		db.Exec(`UPDATE users SET password_hash='!sso', sso_only=1 WHERE id=?`, userID)
		auditLog(r, userID, "ldap.identity_linked", d.Name, map[string]string{"dn": e.DN})
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Not found", 404)
	}
}

// ldapSignInOptions: the directories shown on the sign-in page (names only).
func ldapSignInOptions() []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, d := range ldapDirectories(true) {
		out = append(out, map[string]interface{}{"id": d.ID, "name": d.Name})
	}
	return out
}
