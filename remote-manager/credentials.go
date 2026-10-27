// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── CREDENTIALS VAULT ───────────────────────────────
//
// A credential is a user name with a password (and/or a stored SSH key) that many
// connections log in with ("root@dc1"): connections choose the auth method CREDENTIAL and
// a credential_id instead of storing their own secret. Changing the credential changes it
// for all of them.
//
// The owner can grant a credential to other users (or, administrators, to everybody).
// They can use it in their connections but never see or change the secret. Because a
// password goes to the server at login, a credential can be restricted to host patterns
// (names with * and ?, or CIDR ranges): then it only works for matching hosts, so it cannot
// be sent to a server somebody else controls.
//
// Rotation changes the password on every SSH server that uses the credential: WRM first
// logs in to all of them (pre-flight), then runs passwd on each server (prompts answered
// over a PTY), logs in with the new password to verify, and stores it. When a server
// fails, the servers already changed are changed back (all or nothing). If even that
// fails, the credential keeps the old password and remembers the new one as "pending", so
// neither is lost. Rotation runs in the background; the browser polls its progress.

const (
	maxCredentialsPerUser = 500
	maxRotationHosts      = 300
)

type credential struct {
	ID             int      `json:"id"`
	OwnerID        int      `json:"owner_id"`
	Owner          string   `json:"owner"`
	Mine           bool     `json:"mine"`
	Name           string   `json:"name"`
	Username       string   `json:"username"`
	KeyID          *int     `json:"key_id"`
	KeyName        string   `json:"key_name,omitempty"`
	Description    string   `json:"description"`
	Hosts          string   `json:"hosts"`
	SharedAll      bool     `json:"shared_all"`
	Grants         []int    `json:"grants"`
	GrantNames     []string `json:"grant_names"`
	RotateDays     int      `json:"rotate_days"`
	RotatedAt      string   `json:"rotated_at"`
	RotationDue    bool     `json:"rotation_due"`
	RotationStatus string   `json:"rotation_status"`
	Incomplete     bool     `json:"rotation_incomplete"`
	HasPassword    bool     `json:"has_password"`
	UsedBy         int      `json:"used_by"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	password       string   // encrypted
	pending        string   // encrypted; new password of an incomplete rotation
}

const credentialCols = `id,owner_id,name,username,password,key_id,description,hosts,shared_all,rotate_days,rotated_at,pending_password,rotation_status,created_at,updated_at`

func scanCredential(sc interface{ Scan(...interface{}) error }) (credential, error) {
	var c credential
	var shared int
	err := sc.Scan(&c.ID, &c.OwnerID, &c.Name, &c.Username, &c.password, &c.KeyID, &c.Description, &c.Hosts, &shared, &c.RotateDays,
		&c.RotatedAt, &c.pending, &c.RotationStatus, &c.CreatedAt, &c.UpdatedAt)
	c.SharedAll = shared == 1
	c.HasPassword = c.password != ""
	c.Incomplete = c.pending != ""
	if c.RotateDays > 0 {
		last := c.RotatedAt
		if last == "" {
			last = c.CreatedAt
		}
		if t, err := time.Parse(time.RFC3339, last); err == nil && time.Since(t) > time.Duration(c.RotateDays)*24*time.Hour {
			c.RotationDue = true
		}
	}
	return c, err
}

func loadCredential(id int) (credential, error) {
	c, err := scanCredential(db.QueryRow(`SELECT `+credentialCols+` FROM credentials WHERE id=?`, id))
	if err != nil {
		return c, fmt.Errorf("credential not found")
	}
	return c, nil
}

// credentialAccessible: owner, granted, or shared with everybody.
func credentialAccessible(c credential, userID int) bool {
	if c.OwnerID == userID || c.SharedAll {
		return true
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM credential_grants WHERE credential_id=? AND user_id=?`, c.ID, userID).Scan(&n)
	return n > 0
}

func credentialsForUser(userID int) []credential {
	out := []credential{}
	rows, err := db.Query(`SELECT `+credentialCols+` FROM credentials
		WHERE owner_id=? OR shared_all=1 OR id IN (SELECT credential_id FROM credential_grants WHERE user_id=?)
		ORDER BY name COLLATE NOCASE, id`, userID, userID)
	if err != nil {
		return out
	}
	var list []credential
	for rows.Next() {
		if c, err := scanCredential(rows); err == nil {
			list = append(list, c)
		}
	}
	rows.Close()
	for _, c := range list {
		c.Mine = c.OwnerID == userID
		c.Owner = usernameOf(c.OwnerID)
		c.Grants, c.GrantNames = []int{}, []string{}
		if c.KeyID != nil {
			db.QueryRow(`SELECT name FROM ssh_keys WHERE id=? AND user_id=?`, *c.KeyID, c.OwnerID).Scan(&c.KeyName)
		}
		if c.Mine {
			if gr, err := db.Query(`SELECT g.user_id, u.username FROM credential_grants g JOIN users u ON u.id=g.user_id WHERE g.credential_id=? ORDER BY u.username`, c.ID); err == nil {
				for gr.Next() {
					var id int
					var n string
					gr.Scan(&id, &n)
					c.Grants = append(c.Grants, id)
					c.GrantNames = append(c.GrantNames, n)
				}
				gr.Close()
			}
			db.QueryRow(`SELECT COUNT(*) FROM connections WHERE auth_method='CREDENTIAL' AND credential_id=?`, c.ID).Scan(&c.UsedBy)
		} else {
			db.QueryRow(`SELECT COUNT(*) FROM connections WHERE auth_method='CREDENTIAL' AND credential_id=? AND user_id=?`, c.ID, userID).Scan(&c.UsedBy)
			c.RotationStatus = ""
		}
		out = append(out, c)
	}
	return out
}

// hostAllowed reports whether host (with or without port) matches the patterns of a
// credential: empty = any host; otherwise names with * ? [..] or CIDR ranges, separated by
// commas, spaces or new lines.
func hostAllowed(patterns, host string) bool {
	pats := strings.FieldsFunc(strings.ToLower(patterns), func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t' })
	if len(pats) == 0 {
		return true
	}
	h := strings.ToLower(hostOnly(host))
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	ip := net.ParseIP(h)
	for _, p := range pats {
		if strings.Contains(p, "/") {
			if _, n, err := net.ParseCIDR(p); err == nil && ip != nil && n.Contains(ip) {
				return true
			}
			continue
		}
		if ok, _ := path.Match(p, h); ok {
			return true
		}
	}
	return false
}

func validHostPatterns(patterns string) error {
	for _, p := range strings.FieldsFunc(patterns, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t' }) {
		if strings.Contains(p, "/") {
			if _, _, err := net.ParseCIDR(p); err != nil {
				return fmt.Errorf("invalid CIDR range %q", p)
			}
			continue
		}
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("invalid host pattern %q", p)
		}
	}
	return nil
}

// credentialFor returns the credential connection c (of c.UserID) may log in with.
func credentialFor(c Connection) (credential, error) {
	if c.CredentialID == nil || *c.CredentialID <= 0 {
		return credential{}, fmt.Errorf("no credential selected")
	}
	cr, err := loadCredential(*c.CredentialID)
	if err != nil {
		return cr, fmt.Errorf("the credential of this connection no longer exists")
	}
	if !credentialAccessible(cr, c.UserID) {
		return cr, fmt.Errorf("credential %q is no longer shared with you", cr.Name)
	}
	if !hostAllowed(cr.Hosts, c.Host) {
		return cr, fmt.Errorf("credential %q may only be used for hosts matching %q", cr.Name, cr.Hosts)
	}
	// A jump host decides where the connection really goes: with a credential shared by
	// somebody else, the jump hosts must match the host patterns too, or the user could
	// lead the login (and the password) to a server of their own.
	if cr.OwnerID != c.UserID && strings.TrimSpace(cr.Hosts) != "" {
		for _, h := range jumpHostsRaw(c) {
			if !hostAllowed(cr.Hosts, h) {
				return cr, fmt.Errorf("credential %q is shared for hosts matching %q: the jump host %s does not match", cr.Name, cr.Hosts, h)
			}
		}
	}
	return cr, nil
}

// jumpHostsRaw lists the hosts of c's jump chain (without resolving their logins).
func jumpHostsRaw(c Connection) []string {
	var hosts []string
	seen := map[int]bool{c.ID: true}
	next := c.JumpID
	for i := 0; next != nil && *next > 0 && i < maxJumpDepth && !seen[*next]; i++ {
		seen[*next] = true
		j, err := loadConnectionRaw(*next)
		if err != nil {
			break
		}
		hosts = append(hosts, j.Host)
		next = j.JumpID
	}
	return hosts
}

// resolveConnectionAuth replaces KEY_REF and CREDENTIAL by the secrets they point to. On
// failure the connection keeps its method and authErr explains why it cannot log in.
func resolveConnectionAuth(c *Connection) {
	switch c.AuthMethod {
	case "BITWARDEN":
		resolveBitwardenAuth(c)
	case "KEY_REF":
		p, err := storedKeyPEM(c.KeyID, c.UserID)
		if err != nil {
			c.authErr = err.Error()
			return
		}
		c.AuthMethod, c.PrivateKey, c.usedKeyID, c.keyCert = "KEY", p, *c.KeyID, storedKeyCert(c.KeyID, c.UserID)
	case "CREDENTIAL":
		cr, err := credentialFor(*c)
		if err != nil {
			c.authErr = err.Error()
			return
		}
		if cr.Username != "" {
			c.Username = cr.Username
		}
		c.Password = decryptValue(cr.password)
		c.PrivateKey = ""
		c.usedCredID = cr.ID
		if cr.KeyID != nil {
			p, err := storedKeyPEM(cr.KeyID, cr.OwnerID)
			if err != nil {
				c.authErr = fmt.Sprintf("credential %q: %v", cr.Name, err)
				return
			}
			c.AuthMethod, c.PrivateKey, c.usedKeyID, c.keyCert = "KEY", p, *cr.KeyID, storedKeyCert(cr.KeyID, cr.OwnerID)
		} else {
			c.AuthMethod = "PASSWORD"
		}
	}
}

// checkAuthRefs validates the key or credential a connection of userID refers to.
func checkAuthRefs(c *Connection, userID int) error {
	if c.AuthMethod != "KEY_REF" {
		c.KeyID = nil
	}
	if c.AuthMethod != "CREDENTIAL" {
		c.CredentialID = nil
	}
	switch c.AuthMethod {
	case "BITWARDEN":
		if !bitwardenAllowed(userID) {
			return fmt.Errorf("Bitwarden access is disabled by policy")
		}
		if c.Protocol != "SSH" && c.Protocol != "SFTP" {
			return fmt.Errorf("Bitwarden SSH keys require SSH or SFTP")
		}
		if !bitwardenItemIDRe.MatchString(c.Options["bitwarden_item"]) {
			return fmt.Errorf("Choose a Bitwarden SSH key")
		}
	case "KEY_REF":
		if c.KeyID == nil {
			return fmt.Errorf("Choose an SSH key")
		}
		k, err := loadSSHKey(*c.KeyID, userID)
		if err != nil {
			return fmt.Errorf("SSH key not found")
		}
		if !k.HasPrivate {
			return fmt.Errorf("SSH key %q is a public key only: WRM cannot log in with it", k.Name)
		}
	case "CREDENTIAL":
		probe := *c
		probe.UserID = userID
		if _, err := credentialFor(probe); err != nil {
			return fmt.Errorf("%s", strings.ToUpper(err.Error()[:1])+err.Error()[1:])
		}
	}
	return nil
}

// ─── PASSWORDS ───────────────────────────────────────

const pwLower, pwUpper, pwDigits, pwSymbols = "abcdefghijkmnopqrstuvwxyz", "ABCDEFGHJKLMNPQRSTUVWXYZ", "23456789", "-_.,:+=!@%^*"

// generatePassword returns a random password with lower and upper case letters, digits and
// symbols (accepted by the usual complexity rules), without look-alike characters.
func generatePassword(n int) string {
	if n < 12 {
		n = 12
	}
	all := pwLower + pwUpper + pwDigits + pwSymbols
	pick := func(set string) byte {
		i, _ := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
		return set[i.Int64()]
	}
	for {
		b := make([]byte, n)
		for i := range b {
			b[i] = pick(all)
		}
		s := string(b)
		if strings.ContainsAny(s, pwLower) && strings.ContainsAny(s, pwUpper) && strings.ContainsAny(s, pwDigits) &&
			strings.ContainsAny(s, pwSymbols) && !strings.ContainsAny(s[:1], pwSymbols) {
			return s
		}
	}
}

func validNewPassword(p string) error {
	if len(p) < 8 || len(p) > 128 {
		return fmt.Errorf("the new password needs 8 to 128 characters")
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("the new password contains control characters")
		}
	}
	return nil
}

// ─── ROTATION ────────────────────────────────────────

var (
	passwdCurrentRe = regexp.MustCompile(`(?i)(current|old|existing|\(current\)|login)[^:\n]*password[^:\n]*:\s*$`)
	passwdNewRe     = regexp.MustCompile(`(?i)(new|retype|re-enter|reenter|again|repeat|confirm|verify)[^:\n]*password[^:\n]*:\s*$|password[^:\n]*(again|confirm)[^:\n]*:\s*$`)
	passwdAnyRe     = regexp.MustCompile(`(?i)password[^:\n]*:\s*$`)
	passwdFailRe    = regexp.MustCompile(`(?i)bad password|token manipulation|authentication failure|password unchanged|must wait|too short|too simple|too similar|palindrome|dictionary|have exhausted|failed preliminary|not changed|sorry|incorrect|denied|permission`)
)

// changePasswordOnHost runs passwd on c (logged in with oldPW, or c's key) and answers its
// prompts: current password → oldPW, new / retype → newPW.
func changePasswordOnHost(c Connection, oldPW, newPW string) error {
	if c.AuthMethod == "PASSWORD" {
		c.Password = oldPW
	}
	cl, err := dialSSH(c, nil)
	if err != nil {
		return err
	}
	defer cl.Close()
	s, err := cl.NewSession()
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.RequestPty("dumb", 40, 200, ssh.TerminalModes{ssh.ECHO: 0, ssh.TTY_OP_ISPEED: 38400, ssh.TTY_OP_OSPEED: 38400}); err != nil {
		return fmt.Errorf("cannot open a terminal: %v", err)
	}
	stdin, err := s.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := s.StdoutPipe()
	if err != nil {
		return err
	}
	if err := s.Start("env LC_ALL=C LANG=C passwd"); err != nil {
		return fmt.Errorf("cannot run passwd: %v", err)
	}
	var mu sync.Mutex
	var all, pending strings.Builder
	newPrompts, failed := 0, ""
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				mu.Lock()
				chunk := string(buf[:n])
				all.WriteString(chunk)
				pending.WriteString(chunk)
				// Only the last line can be a prompt ("BAD PASSWORD: …" is not one).
				p := pending.String()
				if i := strings.LastIndexAny(p, "\r\n"); i >= 0 {
					p = p[i+1:]
				}
				p = strings.TrimRight(p, " \t")
				answer := ""
				switch {
				case passwdFailRe.MatchString(p) && !strings.HasSuffix(strings.ToLower(p), "password:"):
				case strings.HasPrefix(strings.ToLower(p), "bad password"):
				case passwdNewRe.MatchString(p):
					newPrompts++
					if newPrompts > 2 {
						failed = "the server asked for a new password again (rejected by its password rules)"
						s.Close()
					} else {
						answer = newPW
					}
				case passwdCurrentRe.MatchString(p):
					answer = oldPW
				case passwdAnyRe.MatchString(p):
					if newPrompts == 0 {
						answer = oldPW
					} else {
						answer = newPW
						newPrompts++
					}
				}
				if answer != "" {
					pending.Reset()
					io.WriteString(stdin, answer+"\n")
				}
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	waitErr := make(chan error, 1)
	go func() { waitErr <- s.Wait() }()
	var werr error
	select {
	case werr = <-waitErr:
	case <-time.After(40 * time.Second):
		s.Close()
		werr = fmt.Errorf("passwd did not finish within 40s")
	}
	select {
	case <-readDone:
	case <-time.After(2 * time.Second):
	}
	mu.Lock()
	out := all.String()
	mu.Unlock()
	out = strings.NewReplacer(oldPW, "•••", newPW, "•••").Replace(out)
	lines := []string{}
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if l = strings.TrimSpace(l); l != "" && !passwdAnyRe.MatchString(l) {
			lines = append(lines, l)
		}
	}
	tail := strings.Join(lines, " · ")
	if len(tail) > 300 {
		tail = "…" + tail[len(tail)-300:]
	}
	if failed != "" {
		return fmt.Errorf("%s: %s", failed, tail)
	}
	if werr != nil {
		if tail == "" {
			tail = werr.Error()
		}
		if newPrompts == 0 && !passwdFailRe.MatchString(out) && !strings.Contains(strings.ToLower(out), "password") {
			return fmt.Errorf("passwd is not available for this user: %s", tail)
		}
		return fmt.Errorf("passwd failed: %s", tail)
	}
	if newPrompts == 0 {
		return fmt.Errorf("passwd did not ask for a new password: %s", tail)
	}
	return nil
}

// verifyLogin logs in to c with a password (c's own auth when it uses a key).
func verifyLogin(c Connection, pw string) error {
	if c.AuthMethod == "PASSWORD" {
		c.Password = pw
	}
	cl, err := dialSSH(c, nil)
	if err != nil {
		return err
	}
	cl.Close()
	return nil
}

// verifyPasswordLogin always logs in with the password (also for key credentials).
func verifyPasswordLogin(c Connection, pw string) error {
	c.AuthMethod, c.Password, c.PrivateKey = "PASSWORD", pw, ""
	return verifyLogin(c, pw)
}

type rotationHost struct {
	Key    string   `json:"key"` // user@host:port
	Conns  []string `json:"connections"`
	State  string   `json:"state"` // pending | checking | ok | changed | failed | rolled_back | rollback_failed | skipped
	Error  string   `json:"error,omitempty"`
	conn   Connection
	connID int
}

type rotationJob struct {
	mu        sync.Mutex
	CredID    int             `json:"credential_id"`
	Mode      string          `json:"mode"`  // check | rotate
	Stage     string          `json:"stage"` // preflight | change | rollback | done
	Running   bool            `json:"running"`
	OK        bool            `json:"ok"`
	Message   string          `json:"message"`
	Hosts     []*rotationHost `json:"hosts"`
	StartedAt string          `json:"started_at"`
	EndedAt   string          `json:"ended_at"`
	userID    int
}

var rotationJobs = struct {
	sync.Mutex
	m map[int]*rotationJob
}{m: map[int]*rotationJob{}}

func (j *rotationJob) snapshot() map[string]interface{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	b, _ := json.Marshal(j)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	return m
}

func (j *rotationJob) set(f func()) {
	j.mu.Lock()
	f()
	j.mu.Unlock()
}

// rotationTargets lists the servers that use a credential, one per user@host:port.
func rotationTargets(cr credential) ([]*rotationHost, []string, error) {
	rows, err := db.Query(`SELECT id FROM connections WHERE auth_method='CREDENTIAL' AND credential_id=? ORDER BY name`, cr.ID)
	if err != nil {
		return nil, nil, err
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	byKey := map[string]*rotationHost{}
	var hosts []*rotationHost
	var problems []string
	for _, id := range ids {
		c, err := loadConnection(id)
		if err != nil {
			continue
		}
		owner := ""
		if c.UserID != cr.OwnerID {
			owner = " (" + usernameOf(c.UserID) + ")"
		}
		if !isSSHProtocol(c) {
			problems = append(problems, fmt.Sprintf("%s%s is %s, not SSH", c.Name, owner, c.Protocol))
			continue
		}
		if c.authErr != "" {
			problems = append(problems, fmt.Sprintf("%s%s: %s", c.Name, owner, c.authErr))
			continue
		}
		key := c.Username + "@" + c.Host
		if h := byKey[key]; h != nil {
			h.Conns = append(h.Conns, c.Name+owner)
			continue
		}
		h := &rotationHost{Key: key, Conns: []string{c.Name + owner}, State: "pending", conn: c, connID: id}
		byKey[key] = h
		hosts = append(hosts, h)
	}
	return hosts, problems, nil
}

// startRotation validates and starts a check or a rotation in the background.
func startRotation(r *http.Request, userID int, cr credential, mode, newPW string) (*rotationJob, error) {
	if !cr.HasPassword {
		return nil, fmt.Errorf("this credential has no password to rotate")
	}
	hosts, problems, err := rotationTargets(cr)
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 && mode == "rotate" {
		return nil, fmt.Errorf("the password cannot be changed automatically for: %s. Change it there yourself and use \"Set password\", or give these connections another login", strings.Join(problems, "; "))
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no SSH connection uses this credential: change the stored password with Edit")
	}
	if len(hosts) > maxRotationHosts {
		return nil, fmt.Errorf("at most %d servers per rotation", maxRotationHosts)
	}
	rotationJobs.Lock()
	if j := rotationJobs.m[cr.ID]; j != nil && j.Running {
		rotationJobs.Unlock()
		return nil, fmt.Errorf("a check or rotation of this credential is already running")
	}
	j := &rotationJob{CredID: cr.ID, Mode: mode, Stage: "preflight", Running: true, Hosts: hosts, StartedAt: time.Now().UTC().Format(time.RFC3339), userID: userID}
	if len(problems) > 0 {
		j.Message = "Not checked: " + strings.Join(problems, "; ")
	}
	rotationJobs.m[cr.ID] = j
	rotationJobs.Unlock()
	audit := func(action string, details map[string]interface{}) {
		details["cred_id"] = cr.ID
		auditLog(r, userID, action, cr.Name, details)
	}
	go runRotation(j, cr, newPW, audit)
	return j, nil
}

func runRotation(j *rotationJob, cr credential, newPW string, audit func(string, map[string]interface{})) {
	oldPW := decryptValue(cr.password)
	finish := func(ok bool, msg string) {
		j.set(func() {
			j.Running, j.OK, j.Stage, j.EndedAt = false, ok, "done", time.Now().UTC().Format(time.RFC3339)
			if j.Message != "" && msg != "" {
				msg += " — " + j.Message
			}
			j.Message = msg
		})
		hub.sendTo(j.userID, []byte(`{"type":"credentials_changed"}`))
		if j.Mode == "rotate" {
			go checkCredentialReminders() // an incomplete rotation is reported right away
		}
	}
	// 1. Pre-flight: log in everywhere with the current password.
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, h := range j.Hosts {
		wg.Add(1)
		go func(h *rotationHost) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			j.set(func() { h.State = "checking" })
			err := verifyLogin(h.conn, oldPW)
			if err == nil && h.conn.AuthMethod == "KEY" {
				err = verifyPasswordLogin(h.conn, oldPW)
				if err != nil {
					err = fmt.Errorf("logs in with the key, but the stored password is not accepted: %v", err)
				}
			}
			j.set(func() {
				if err != nil {
					h.State, h.Error = "failed", err.Error()
				} else {
					h.State = "ok"
				}
			})
		}(h)
	}
	wg.Wait()
	failed := []string{}
	for _, h := range j.Hosts {
		if h.State == "failed" {
			failed = append(failed, h.Key)
		}
	}
	if j.Mode == "check" {
		if len(failed) > 0 {
			finish(false, fmt.Sprintf("%d of %d servers did not accept the stored password", len(failed), len(j.Hosts)))
		} else {
			finish(true, fmt.Sprintf("All %d servers accept the stored password", len(j.Hosts)))
		}
		audit("credential.checked", map[string]interface{}{"servers": len(j.Hosts), "failed": failed})
		return
	}
	if len(failed) > 0 {
		db.Exec(`UPDATE credentials SET rotation_status=? WHERE id=?`, "failed "+time.Now().UTC().Format(time.RFC3339)+": pre-flight check failed on "+strings.Join(failed, ", "), cr.ID)
		finish(false, "Nothing was changed: the stored password does not work on "+strings.Join(failed, ", "))
		audit("credential.rotation_failed", map[string]interface{}{"stage": "preflight", "failed": failed})
		return
	}
	if newPW == "" {
		newPW = generatePassword(24)
	}
	// 2. Change it, one server after the other.
	j.set(func() { j.Stage = "change" })
	var changed []*rotationHost
	var failHost *rotationHost
	for _, h := range j.Hosts {
		j.set(func() { h.State = "changing" })
		err := changePasswordOnHost(h.conn, oldPW, newPW)
		if err == nil {
			if verr := verifyPasswordLogin(h.conn, newPW); verr != nil {
				err = fmt.Errorf("passwd succeeded, but logging in with the new password failed: %v", verr)
				changed = append(changed, h) // try to change it back
			}
		} else if verifyPasswordLogin(h.conn, newPW) == nil {
			// The same server under another name, changed a moment ago.
			err = nil
		}
		if err != nil {
			j.set(func() { h.State, h.Error = "failed", err.Error() })
			failHost = h
			break
		}
		j.set(func() { h.State = "changed" })
		changed = append(changed, h)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if failHost == nil {
		db.Exec(`UPDATE credentials SET password=?, pending_password='', rotated_at=?, updated_at=?, rotation_status=? WHERE id=?`,
			encryptValue(newPW), now, now, fmt.Sprintf("ok %s: %d servers", now, len(j.Hosts)), cr.ID)
		keys := []string{}
		for _, h := range j.Hosts {
			keys = append(keys, h.Key)
		}
		audit("credential.rotated", map[string]interface{}{"servers": keys})
		finish(true, fmt.Sprintf("The password was changed and verified on all %d servers", len(j.Hosts)))
		return
	}
	// 3. Roll back the servers already changed.
	j.set(func() { j.Stage = "rollback" })
	for _, h := range j.Hosts {
		if h.State == "pending" {
			j.set(func() { h.State = "skipped" })
		}
	}
	var stuck []string
	for _, h := range changed {
		err := changePasswordOnHost(withPassword(h.conn, newPW), newPW, oldPW)
		if err == nil {
			err = verifyPasswordLogin(h.conn, oldPW)
		}
		if err != nil {
			stuck = append(stuck, h.Key)
			j.set(func() {
				h.State = "rollback_failed"
				h.Error = strings.TrimPrefix(h.Error+" · ", " · ") + "changing back failed: " + err.Error()
			})
		} else if h != failHost {
			j.set(func() { h.State = "rolled_back" })
		}
	}
	if len(stuck) > 0 {
		db.Exec(`UPDATE credentials SET pending_password=?, updated_at=?, rotation_status=? WHERE id=?`, encryptValue(newPW), now,
			"incomplete "+now+": "+strings.Join(stuck, ", ")+" have the NEW password", cr.ID)
		audit("credential.rotation_incomplete", map[string]interface{}{"failed_on": failHost.Key, "new_password_on": stuck})
		finish(false, fmt.Sprintf("Rotation failed on %s and %s could not be changed back: they have the NEW password (kept as pending in the vault; the others have the old one)", failHost.Key, strings.Join(stuck, ", ")))
		return
	}
	db.Exec(`UPDATE credentials SET rotation_status=? WHERE id=?`, "failed "+now+": "+failHost.Key+": "+truncateStr(failHost.Error, 200), cr.ID)
	audit("credential.rotation_failed", map[string]interface{}{"stage": "change", "failed_on": failHost.Key, "error": failHost.Error, "rolled_back": len(changed)})
	finish(false, fmt.Sprintf("Rotation failed on %s; all servers keep the old password", failHost.Key))
}

func withPassword(c Connection, pw string) Connection {
	if c.AuthMethod == "PASSWORD" {
		c.Password = pw
	}
	return c
}

// ─── API ─────────────────────────────────────────────

// GET    /api/credentials                  credentials the user can use (no secrets)
// POST   /api/credentials                  create
// PUT    /api/credentials/{id}             change (owner); empty password keeps it
// DELETE /api/credentials/{id}             (owner; refused while connections use it)
// GET    /api/credentials/{id}/usage       connections that use it
// POST   /api/credentials/{id}/reveal      {password} → secret (owner, re-authentication)
// POST   /api/credentials/{id}/rotate      {mode: check|rotate, new_password} → starts a job
// GET    /api/credentials/{id}/rotation    state of the last check / rotation
func apiCredentialsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/credentials"), "/")
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			jsonOK(w, credentialsForUser(userID))
		case http.MethodPost:
			saveCredential(w, r, userID, nil)
		default:
			jsonError(w, "Method not allowed", 405)
		}
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "Bad ID", 400)
		return
	}
	cr, err := loadCredential(id)
	if err != nil || !credentialAccessible(cr, userID) {
		jsonError(w, "Not found", 404)
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	owner := cr.OwnerID == userID
	if !owner && !(action == "usage" && r.Method == http.MethodGet) {
		jsonError(w, "Only the owner of the credential can do that", 403)
		return
	}
	switch {
	case action == "" && r.Method == http.MethodPut:
		saveCredential(w, r, userID, &cr)
	case action == "" && r.Method == http.MethodDelete:
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM connections WHERE auth_method='CREDENTIAL' AND credential_id=?`, id).Scan(&n)
		if n > 0 {
			jsonError(w, fmt.Sprintf("%d connections log in with this credential. Give them another login first.", n), 409)
			return
		}
		db.Exec(`DELETE FROM credential_grants WHERE credential_id=?`, id)
		db.Exec(`DELETE FROM credentials WHERE id=?`, id)
		auditLog(r, userID, "credential.deleted", cr.Name, map[string]interface{}{"cred_id": id})
		notifyCredentialsChanged(cr, nil)
		jsonOK(w, map[string]bool{"ok": true})
	case action == "usage" && r.Method == http.MethodGet:
		q := `SELECT c.id, c.name, c.host, c.protocol, c.user_id, COALESCE(u.username,'') FROM connections c LEFT JOIN users u ON u.id=c.user_id
			WHERE c.auth_method='CREDENTIAL' AND c.credential_id=?`
		args := []interface{}{id}
		if !owner {
			q += ` AND c.user_id=?`
			args = append(args, userID)
		}
		rows, err := db.Query(q+` ORDER BY u.username, c.name`, args...)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		out := []map[string]interface{}{}
		for rows.Next() {
			var cid, uid int
			var name, host, proto, uname string
			rows.Scan(&cid, &name, &host, &proto, &uid, &uname)
			out = append(out, map[string]interface{}{"id": cid, "name": name, "host": host, "protocol": proto, "owner": uname, "mine": uid == userID,
				"host_allowed": hostAllowed(cr.Hosts, host)})
		}
		jsonOK(w, out)
	case action == "reveal" && r.Method == http.MethodPost:
		var in struct {
			Password string `json:"password"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		u, err := loadUser("id=?", userID)
		if err != nil || !userCheckPassword(u, in.Password) {
			auditLog(r, userID, "credential.reveal_denied", cr.Name, map[string]interface{}{"cred_id": id, "reason": "wrong password"})
			jsonError(w, "Wrong password", 403)
			return
		}
		if !u.IsAdmin && !settingBool("allow_secret_export") {
			jsonError(w, "Showing stored passwords is disabled by the administrator", 403)
			return
		}
		if !requireStepUp(w, r, "credential_reveal") {
			return
		}
		auditLog(r, userID, "credential.revealed", cr.Name, map[string]interface{}{"cred_id": id})
		jsonOK(w, map[string]string{"password": decryptValue(cr.password), "pending_password": decryptValue(cr.pending)})
	case action == "rotate" && r.Method == http.MethodPost:
		var in struct {
			Mode        string `json:"mode"`
			NewPassword string `json:"new_password"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if in.Mode != "check" {
			in.Mode = "rotate"
		}
		if in.NewPassword != "" {
			if err := validNewPassword(in.NewPassword); err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
		}
		if cr.Incomplete && in.Mode == "rotate" {
			jsonError(w, "The last rotation is incomplete: some servers have the pending password. Fix them first (Reveal shows both passwords), then save the right password with Edit", 409)
			return
		}
		j, err := startRotation(r, userID, cr, in.Mode, in.NewPassword)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		auditLog(r, userID, "credential.rotation_started", cr.Name, map[string]interface{}{"cred_id": id, "mode": in.Mode, "servers": len(j.Hosts)})
		jsonOK(w, j.snapshot())
	case action == "rotation" && r.Method == http.MethodGet:
		rotationJobs.Lock()
		j := rotationJobs.m[id]
		rotationJobs.Unlock()
		if j == nil {
			jsonOK(w, map[string]interface{}{"running": false, "stage": "", "hosts": []interface{}{}})
			return
		}
		jsonOK(w, j.snapshot())
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func notifyCredentialsChanged(cr credential, grants []int) {
	msg := []byte(`{"type":"credentials_changed"}`)
	if cr.SharedAll {
		hub.sendAll(msg)
		return
	}
	hub.sendTo(cr.OwnerID, msg)
	seen := map[int]bool{cr.OwnerID: true}
	for _, g := range append(grants, cr.Grants...) {
		if !seen[g] {
			seen[g] = true
			hub.sendTo(g, msg)
		}
	}
}

func saveCredential(w http.ResponseWriter, r *http.Request, userID int, cur *credential) {
	var in struct {
		Name          string `json:"name"`
		Username      string `json:"username"`
		Password      string `json:"password"`
		ClearPassword bool   `json:"clear_password"`
		KeyID         *int   `json:"key_id"`
		Description   string `json:"description"`
		Hosts         string `json:"hosts"`
		SharedAll     bool   `json:"shared_all"`
		Grants        []int  `json:"grants"`
		RotateDays    int    `json:"rotate_days"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	name := truncateStr(strings.TrimSpace(in.Name), 120)
	username := truncateStr(strings.TrimSpace(in.Username), 120)
	hosts := truncateStr(strings.TrimSpace(in.Hosts), 2000)
	if name == "" {
		jsonError(w, "Name is required", 400)
		return
	}
	if err := validHostPatterns(hosts); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if len(in.Password) > 1024 {
		jsonError(w, "Password too long", 400)
		return
	}
	if in.RotateDays < 0 || in.RotateDays > 3650 {
		jsonError(w, "Rotation interval: 0 to 3650 days", 400)
		return
	}
	if in.KeyID != nil && *in.KeyID <= 0 {
		in.KeyID = nil
	}
	if in.KeyID != nil {
		k, err := loadSSHKey(*in.KeyID, userID)
		if err != nil || !k.HasPrivate {
			jsonError(w, "SSH key not found (it must be a key pair in your key store)", 400)
			return
		}
	}
	if in.SharedAll && !isAdminUser(userID) {
		jsonError(w, "Only administrators can share a credential with everybody", 403)
		return
	}
	grants := []int{}
	seen := map[int]bool{userID: true}
	for _, g := range in.Grants {
		if g > 0 && !seen[g] {
			seen[g] = true
			var n int
			db.QueryRow(`SELECT COUNT(*) FROM users WHERE id=?`, g).Scan(&n)
			if n == 0 {
				jsonError(w, "Unknown user in the list of users", 400)
				return
			}
			grants = append(grants, g)
		}
	}
	password := ""
	if cur != nil {
		password = decryptValue(cur.password)
	}
	pwChanged := false
	if in.Password != "" || in.ClearPassword {
		pwChanged = in.Password != password
		password = in.Password
	}
	if password == "" && in.KeyID == nil {
		jsonError(w, "A credential needs a password or an SSH key", 400)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var id int
	if cur == nil {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM credentials WHERE owner_id=?`, userID).Scan(&n)
		if n >= maxCredentialsPerUser {
			jsonError(w, fmt.Sprintf("At most %d credentials per user", maxCredentialsPerUser), 400)
			return
		}
		res, err := db.Exec(`INSERT INTO credentials (owner_id,name,username,password,key_id,description,hosts,shared_all,rotate_days,rotated_at,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, userID, name, username, encryptValue(password), in.KeyID, truncateStr(in.Description, 2000), hosts,
			boolInt(in.SharedAll), in.RotateDays, now, now, now)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		nid, _ := res.LastInsertId()
		id = int(nid)
		auditLog(r, userID, "credential.created", name, map[string]interface{}{"cred_id": id, "user": username, "with_key": in.KeyID != nil,
			"hosts": hosts, "shared_all": in.SharedAll, "granted_to": len(grants)})
	} else {
		id = cur.ID
		q := `UPDATE credentials SET name=?, username=?, password=?, key_id=?, description=?, hosts=?, shared_all=?, rotate_days=?, updated_at=?`
		args := []interface{}{name, username, encryptValue(password), in.KeyID, truncateStr(in.Description, 2000), hosts, boolInt(in.SharedAll), in.RotateDays, now}
		if in.Password != "" {
			// A password set by hand (e.g. changed on the servers outside WRM) ends an
			// incomplete rotation, and counts as rotated when it is a new one.
			q += `, pending_password=''`
			if pwChanged {
				q += `, rotated_at=?, rotation_status=?`
				args = append(args, now, "set by hand "+now)
			}
		}
		args = append(args, id)
		if _, err := db.Exec(q+` WHERE id=?`, args...); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		changed := []string{}
		for f, same := range map[string]bool{"name": name == cur.Name, "username": username == cur.Username, "password": !pwChanged,
			"key": intPtrEq(in.KeyID, cur.KeyID), "hosts": hosts == cur.Hosts, "shared_all": in.SharedAll == cur.SharedAll, "rotate_days": in.RotateDays == cur.RotateDays} {
			if !same {
				changed = append(changed, f)
			}
		}
		auditLog(r, userID, "credential.updated", name, map[string]interface{}{"cred_id": id, "changed": changed, "granted_to": len(grants)})
		if pwChanged || username != cur.Username || !intPtrEq(in.KeyID, cur.KeyID) {
			restartCredentialTunnels(id)
		}
	}
	old := map[int]bool{}
	if rows, err := db.Query(`SELECT user_id FROM credential_grants WHERE credential_id=?`, id); err == nil {
		for rows.Next() {
			var u int
			rows.Scan(&u)
			old[u] = true
		}
		rows.Close()
	}
	db.Exec(`DELETE FROM credential_grants WHERE credential_id=?`, id)
	for _, g := range grants {
		db.Exec(`INSERT OR IGNORE INTO credential_grants (credential_id, user_id) VALUES (?,?)`, id, g)
		if !old[g] {
			auditLog(r, userID, "credential.granted", name, map[string]interface{}{"cred_id": id, "to": usernameOf(g)})
		}
		delete(old, g)
	}
	removed := []int{}
	for u := range old {
		removed = append(removed, u)
		auditLog(r, userID, "credential.grant_revoked", name, map[string]interface{}{"cred_id": id, "from": usernameOf(u)})
	}
	saved, _ := loadCredential(id)
	notifyCredentialsChanged(saved, append(grants, removed...))
	if cur != nil && cur.SharedAll && !in.SharedAll {
		hub.sendAll([]byte(`{"type":"credentials_changed"}`))
	}
	for _, c := range credentialsForUser(userID) {
		if c.ID == id {
			jsonOK(w, c)
			return
		}
	}
	jsonOK(w, map[string]interface{}{"id": id})
}

// restartCredentialTunnels restarts running tunnels of the connections that use a credential.
func restartCredentialTunnels(credID int) {
	rows, err := db.Query(`SELECT id FROM connections WHERE auth_method='CREDENTIAL' AND credential_id=?`, credID)
	if err != nil {
		return
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		tunnelMgr.restartConn(id)
	}
}

// credentialsDue counts credentials whose rotation interval has passed (admin overview).
func credentialsSummary() map[string]int {
	var total, due, incomplete, shared int
	rows, err := db.Query(`SELECT ` + credentialCols + ` FROM credentials`)
	if err == nil {
		for rows.Next() {
			if c, err := scanCredential(rows); err == nil {
				total++
				if c.RotationDue {
					due++
				}
				if c.Incomplete {
					incomplete++
				}
				if c.SharedAll {
					shared++
				}
			}
		}
		rows.Close()
	}
	return map[string]int{"credentials": total, "rotation_due": due, "rotation_incomplete": incomplete, "shared_all": shared}
}
