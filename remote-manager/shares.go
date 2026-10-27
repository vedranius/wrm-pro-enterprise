// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ─── SHARING MODEL ───────────────────────────────────
//
// A share gives other people access to some of your connections through a link
// (/share/<token>) and opens a live collaboration room (chat, voice, terminal sharing).
//
// Access mode
//   members – only the users added as members (sign-in required)
//   users   – every signed-in user of this WRM server who has the link
//   link    – anyone with the link, also people without an account (guests)
// An optional password protects the users/link modes (members never need it).
//
// Roles (what someone can do in a share)
//   observer  – chat, voice, watch shared terminals; no access to the connections
//   viewer    – + browse and download files (read-only)
//   operator  – + terminals, upload/edit/delete files, share own terminal, receive control
//   moderator – + manage people in the room: roles, mute, kick, ban
//   owner     – the creator of the share: everything, incl. settings and members

const (
	RoleObserver  = "observer"
	RoleViewer    = "viewer"
	RoleOperator  = "operator"
	RoleModerator = "moderator"
	RoleOwner     = "owner"

	PermFilesRead   = "files_read"
	PermTerminal    = "terminal"
	PermFilesWrite  = "files_write"
	PermShareScreen = "share_screen"
	PermControl     = "control"
	PermModerate    = "moderate"
	PermManage      = "manage"
)

var roleRank = map[string]int{RoleObserver: 1, RoleViewer: 2, RoleOperator: 3, RoleModerator: 4, RoleOwner: 5}

var permMinRank = map[string]int{
	PermFilesRead: 2, PermTerminal: 3, PermFilesWrite: 3, PermShareScreen: 3, PermControl: 3, PermModerate: 4, PermManage: 5,
}

func roleCan(role, perm string) bool {
	need, ok := permMinRank[perm]
	return ok && roleRank[role] >= need
}

func rolePerms(role string) map[string]bool {
	out := map[string]bool{}
	for p := range permMinRank {
		out[p] = roleCan(role, p)
	}
	return out
}

// validAssignableRole: roles that can be given to members / participants.
func validAssignableRole(role string, maxRank int) bool {
	r, ok := roleRank[role]
	return ok && role != RoleOwner && r <= maxRank
}

type ShareLink struct {
	ID           int    `json:"id"`
	OwnerID      int    `json:"owner_id"`
	Token        string `json:"token"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Public       bool   `json:"public"` // listed under "Shared with me" for every user
	Active       bool   `json:"active"`
	CreatedAt    string `json:"created_at"`
	AccessMode   string `json:"access_mode"`
	GuestRole    string `json:"guest_role"`
	ExpiresAt    string `json:"expires_at"`
}

const shareCols = `id, owner_id, token, name, COALESCE(password_hash,''), public, active, created_at, access_mode, guest_role, expires_at`

func scanShare(sc interface{ Scan(...interface{}) error }) (ShareLink, error) {
	var s ShareLink
	err := sc.Scan(&s.ID, &s.OwnerID, &s.Token, &s.Name, &s.PasswordHash, &s.Public, &s.Active, &s.CreatedAt, &s.AccessMode, &s.GuestRole, &s.ExpiresAt)
	return s, err
}

func getShareByToken(token string) (ShareLink, error) {
	return scanShare(db.QueryRow(`SELECT `+shareCols+` FROM share_links WHERE token=?`, token))
}

func getShareByID(id int) (ShareLink, error) {
	return scanShare(db.QueryRow(`SELECT `+shareCols+` FROM share_links WHERE id=?`, id))
}

func (s ShareLink) expired() bool {
	t, ok := parseTS(s.ExpiresAt)
	return ok && time.Now().After(t)
}

// ─── ACCESS RESOLUTION ───────────────────────────────

// accessCtx is what identifies someone towards a share. It is kept by open room
// connections and terminals so access can be re-checked when the share changes.
type accessCtx struct {
	UserID    int
	GuestPKey string // "g:<hash>" for people without an account
	PwCookie  string // value of the share password cookie
	IP        string
}

type ShareAccess struct {
	Share    ShareLink
	Role     string
	PKey     string
	UserID   int
	Username string
	Name     string
	Guest    bool
	Member   bool
	IsOwner  bool
	Ctx      accessCtx
}

type shareDenied struct {
	Code    int    `json:"-"`
	Reason  string `json:"reason"`
	Message string `json:"error"`
}

func denied(code int, reason, msg string) *shareDenied {
	return &shareDenied{Code: code, Reason: reason, Message: msg}
}

const guestCookieName = "wrm_guest"

func sharePwCookieName(shareID int) string { return fmt.Sprintf("wrm_sp_%d", shareID) }

func sharePwCookieValue(s ShareLink) string {
	exp := strconv.FormatInt(time.Now().Add(7*24*time.Hour).Unix(), 10)
	return exp + "." + signValue("sharepw", strconv.Itoa(s.ID), s.PasswordHash, exp)
}

func sharePwCookieValid(s ShareLink, v string) bool {
	i := strings.IndexByte(v, '.')
	if i < 0 {
		return false
	}
	exp, sig := v[:i], v[i+1:]
	n, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > n {
		return false
	}
	return verifySigned(sig, "sharepw", strconv.Itoa(s.ID), s.PasswordHash, exp)
}

func guestPKeyFromRaw(raw string) string {
	if len(raw) < 16 || len(raw) > 100 {
		return ""
	}
	return "g:" + sha256Hex("wrm-guest:" + raw)[:32]
}

// ensureGuestCookie gives a browser without an account a stable anonymous identity.
func ensureGuestCookie(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(guestCookieName); err == nil && guestPKeyFromRaw(c.Value) != "" {
		return guestPKeyFromRaw(c.Value)
	}
	raw := randomToken(24)
	setCookie(w, r, guestCookieName, raw, 90*86400)
	r.AddCookie(&http.Cookie{Name: guestCookieName, Value: raw})
	return guestPKeyFromRaw(raw)
}

func accessCtxFromRequest(r *http.Request, share ShareLink) accessCtx {
	ctx := accessCtx{IP: clientIP(r)}
	if uid, err := currentUserID(r); err == nil {
		ctx.UserID = uid
	}
	if c, err := r.Cookie(guestCookieName); err == nil {
		ctx.GuestPKey = guestPKeyFromRaw(c.Value)
	}
	if c, err := r.Cookie(sharePwCookieName(share.ID)); err == nil {
		ctx.PwCookie = c.Value
	}
	return ctx
}

func resolveShareAccess(r *http.Request, share ShareLink) (*ShareAccess, *shareDenied) {
	return resolveShareAccessCtx(share, accessCtxFromRequest(r, share))
}

func resolveShareAccessCtx(share ShareLink, ctx accessCtx) (*ShareAccess, *shareDenied) {
	if share.ID == 0 {
		return nil, denied(404, "not_found", "Share not found")
	}
	if lockdownActive() && !userIsBreakGlass(ctx.UserID) {
		return nil, denied(403, "lockdown", "WRM is in lockdown: shares are paused")
	}
	a := &ShareAccess{Share: share, Ctx: ctx}
	if ctx.UserID > 0 {
		var username, dn string
		var disabled int
		if db.QueryRow(`SELECT username, COALESCE(display_name,''), disabled FROM users WHERE id=?`, ctx.UserID).Scan(&username, &dn, &disabled) != nil || disabled == 1 {
			ctx.UserID = 0
			a.Ctx.UserID = 0
		} else {
			a.UserID, a.Username, a.PKey = ctx.UserID, username, "u:"+strconv.Itoa(ctx.UserID)
			a.Name = username
			if dn != "" {
				a.Name = dn
			}
		}
	}
	if a.UserID > 0 && a.UserID == share.OwnerID {
		a.Role, a.IsOwner = RoleOwner, true
		return a, nil
	}
	if !share.Active {
		return nil, denied(403, "inactive", "This share is paused by its owner")
	}
	if share.expired() {
		return nil, denied(410, "expired", "This share has expired")
	}
	if a.UserID > 0 {
		var role string
		if db.QueryRow(`SELECT role FROM share_members WHERE share_id=? AND user_id=?`, share.ID, a.UserID).Scan(&role) == nil {
			if _, ok := roleRank[role]; !ok || role == RoleOwner {
				role = RoleOperator
			}
			a.Role, a.Member = role, true
		}
		// Through a team: the best role of the user's teams in this share, or the direct role
		// when that is higher.
		if tr := teamShareRole(share.ID, a.UserID); tr != "" && roleRank[tr] > roleRank[a.Role] {
			a.Role, a.Member = tr, true
		}
	}
	if !a.Member {
		mode := share.AccessMode
		if mode == "link" && !settingBool("allow_link_shares") && !isAdminUser(share.OwnerID) {
			mode = "users" // guest links were disabled by the administrator
		}
		switch mode {
		case "members":
			if a.UserID == 0 {
				return nil, denied(401, "login_required", "Sign in to open this share")
			}
			return nil, denied(403, "not_member", "You are not a member of this share. Ask its owner to add you.")
		case "users":
			if a.UserID == 0 {
				return nil, denied(401, "login_required", "Sign in to open this share")
			}
		default: // link
			if a.UserID == 0 {
				if ctx.GuestPKey == "" {
					return nil, denied(401, "guest_cookie", "Reload the page")
				}
				a.Guest, a.PKey = true, ctx.GuestPKey
			}
		}
		if share.PasswordHash != "" && !sharePwCookieValid(share, ctx.PwCookie) {
			return nil, denied(401, "password_required", "This share is password protected")
		}
		a.Role = share.GuestRole
		if !validAssignableRole(a.Role, roleRank[RoleOperator]) {
			a.Role = RoleViewer
		}
	}
	// Per-person override set by the owner or a moderator (role change, ban).
	var override string
	var banned int
	var pname string
	if db.QueryRow(`SELECT role, banned, name FROM share_participants WHERE share_id=? AND pkey=?`, share.ID, a.PKey).Scan(&override, &banned, &pname) == nil {
		if banned == 1 {
			return nil, denied(403, "banned", "You were removed from this share")
		}
		if override != "" && validAssignableRole(override, roleRank[RoleModerator]) {
			a.Role = override
		}
		if a.Guest && pname != "" {
			a.Name = pname
		}
	}
	if a.Guest && a.Name == "" {
		a.Name = "Guest-" + strings.ToUpper(a.PKey[2:6])
	}
	return a, nil
}

func resolveShareToken(r *http.Request, token string) (*ShareAccess, *shareDenied) {
	share, err := getShareByToken(strings.TrimSpace(token))
	if err != nil {
		return nil, denied(404, "not_found", "Share not found or deleted")
	}
	return resolveShareAccess(r, share)
}

// ─── CONNECTION AUTHORIZATION ────────────────────────

type SharedConn struct {
	FolderPath string `json:"folder_path,omitempty"`
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	Host       string `json:"host"`
	Username   string `json:"username"`
	FolderID   *int   `json:"folder_id"`
}

func sharedConnectionsFor(share ShareLink) []SharedConn {
	folders := loadFolders(share.OwnerID)
	rows, err := db.Query(`SELECT id, name, protocol, host, username, folder_id FROM connections
		WHERE user_id=? AND (id IN (SELECT connection_id FROM share_items WHERE share_id=? AND connection_id IS NOT NULL)
		OR folder_id IN (`+shareFolderSubtreeSQL+`)) ORDER BY name COLLATE NOCASE, name, id`,
		share.OwnerID, share.ID, share.ID, share.OwnerID, share.OwnerID)
	out := []SharedConn{}
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var c SharedConn
		rows.Scan(&c.ID, &c.Name, &c.Protocol, &c.Host, &c.Username, &c.FolderID)
		if c.FolderID != nil {
			c.FolderPath = folderPath(folders, *c.FolderID)
		}
		out = append(out, c)
	}
	return out
}

type SharedFolder struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ParentID  *int   `json:"parent_id"`
	Path      string `json:"path"`
	SortOrder int    `json:"sort_order"`
}

// sharedFoldersFor returns the folder skeleton of a share. A shared folder brings its
// whole subtree; a separately shared connection also brings the folders above it so the
// shared tree keeps the same hierarchy as the owner's tree.
func sharedFoldersFor(share ShareLink, conns []SharedConn) []SharedFolder {
	folders := loadFolders(share.OwnerID)
	byID := map[int]Folder{}
	for _, f := range folders {
		byID[f.ID] = f
	}
	allowed := map[int]bool{}
	if rows, err := db.Query(`SELECT folder_id FROM share_items WHERE share_id=? AND folder_id IS NOT NULL`, share.ID); err == nil {
		for rows.Next() {
			var id int
			if rows.Scan(&id) == nil {
				for _, sub := range folderSubtreeIDs(folders, id) {
					allowed[sub] = true
				}
			}
		}
		rows.Close()
	}
	for _, c := range conns {
		id := 0
		if c.FolderID != nil {
			id = *c.FolderID
		}
		for n := 0; id > 0 && n < folderMaxDepth; n++ {
			if allowed[id] {
				break
			}
			allowed[id] = true
			f, ok := byID[id]
			if !ok || f.ParentID == nil {
				break
			}
			id = *f.ParentID
		}
	}
	out := []SharedFolder{}
	for _, f := range folders {
		if !allowed[f.ID] {
			continue
		}
		parent := f.ParentID
		if parent != nil && !allowed[*parent] {
			parent = nil // the shared root starts here
		}
		out = append(out, SharedFolder{ID: f.ID, Name: f.Name, ParentID: parent, Path: f.Path, SortOrder: f.SortOrder})
	}
	return out
}

func shareHasConnection(share ShareLink, connID int) bool {
	var n int
	db.QueryRow(`SELECT COUNT(1) FROM connections WHERE id=? AND user_id=? AND (id IN (SELECT connection_id FROM share_items WHERE share_id=? AND connection_id IS NOT NULL)
		OR folder_id IN (`+shareFolderSubtreeSQL+`))`, connID, share.OwnerID, share.ID, share.ID, share.OwnerID, share.OwnerID).Scan(&n)
	return n > 0
}

type connAccess struct {
	UserID int          // signed-in user (0 for guests)
	Share  *ShareAccess // nil when the user owns the connection
}

func (a *connAccess) actor() (int, string) {
	if a.Share != nil {
		if a.Share.Guest {
			return 0, a.Share.Name + " (guest)"
		}
		return a.Share.UserID, a.Share.Username
	}
	return a.UserID, usernameOf(a.UserID)
}

// authorizeConnection checks that the request may use the connection for perm. Owners of
// the connection may do everything; others need a share_token whose role allows perm.
func authorizeConnection(r *http.Request, connID int, perm string) (*connAccess, int, string) {
	if connID <= 0 {
		return nil, 404, "Connection not found"
	}
	if userID, err := currentUserID(r); err == nil {
		if isAuditorUser(userID) {
			return nil, 403, "Security auditors cannot open connections"
		}
		if userOwnsConnection(connID, userID) {
			return &connAccess{UserID: userID}, 0, ""
		}
	}
	token := strings.TrimSpace(r.URL.Query().Get("share_token"))
	if token == "" {
		return nil, 404, "Connection not found"
	}
	sa, d := resolveShareToken(r, token)
	if d != nil {
		return nil, d.Code, d.Message
	}
	if !shareHasConnection(sa.Share, connID) {
		return nil, 404, "Connection not found"
	}
	if !roleCan(sa.Role, perm) {
		return nil, 403, fmt.Sprintf("Your role in this share (%s) does not allow this action", sa.Role)
	}
	return &connAccess{UserID: sa.UserID, Share: sa}, 0, ""
}

// ─── PARTICIPANT RECORDS ─────────────────────────────

func touchParticipant(sa *ShareAccess, ip string) {
	now := time.Now().UTC().Format(time.RFC3339)
	var uid interface{}
	if sa.UserID > 0 {
		uid = sa.UserID
	}
	db.Exec(`INSERT INTO share_participants (share_id, pkey, user_id, name, first_seen, last_seen, last_ip) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(share_id, pkey) DO UPDATE SET last_seen=excluded.last_seen, last_ip=excluded.last_ip,
		name=CASE WHEN share_participants.user_id IS NOT NULL OR share_participants.name='' THEN excluded.name ELSE share_participants.name END`,
		sa.Share.ID, sa.PKey, uid, sa.Name, now, now, ip)
}

// setParticipantRole stores a role for a person in a share. Members keep their role in
// share_members; everyone else gets a per-share override.
func setParticipantRole(shareID int, pkey, role string) {
	if strings.HasPrefix(pkey, "u:") {
		uid, _ := strconv.Atoi(pkey[2:])
		if res, err := db.Exec(`UPDATE share_members SET role=? WHERE share_id=? AND user_id=?`, role, shareID, uid); err == nil {
			if n, _ := res.RowsAffected(); n > 0 {
				return
			}
		}
	}
	db.Exec(`INSERT INTO share_participants (share_id, pkey, name, role, first_seen, last_seen) VALUES (?,?,?,?,?,?)
		ON CONFLICT(share_id, pkey) DO UPDATE SET role=excluded.role`, shareID, pkey, "", role, "", "")
}

func setParticipantBanned(shareID int, pkey string, banned bool) {
	db.Exec(`INSERT INTO share_participants (share_id, pkey, name, banned, first_seen, last_seen) VALUES (?,?,?,?,?,?)
		ON CONFLICT(share_id, pkey) DO UPDATE SET banned=excluded.banned`, shareID, pkey, "", boolToInt(banned), "", "")
	if banned && strings.HasPrefix(pkey, "u:") {
		uid, _ := strconv.Atoi(pkey[2:])
		db.Exec(`DELETE FROM share_members WHERE share_id=? AND user_id=?`, shareID, uid)
	}
}

// ─── SHARE API (owner side) ──────────────────────────

func canManageShare(userID int, s ShareLink) bool {
	return s.OwnerID == userID || isAdminUser(userID)
}

type memberInput struct {
	Username string `json:"username"`
	UserID   int    `json:"user_id"`
	Role     string `json:"role"`
}

type shareInput struct {
	Name          *string            `json:"name"`
	ConnectionIDs *[]int             `json:"connection_ids"`
	FolderIDs     *[]int             `json:"folder_ids"`
	AccessMode    *string            `json:"access_mode"`
	GuestRole     *string            `json:"guest_role"`
	Password      *string            `json:"password"`
	ClearPassword bool               `json:"clear_password"`
	Public        *bool              `json:"public"`
	ExpiresAt     *string            `json:"expires_at"`
	Active        *bool              `json:"active"`
	Members       *[]memberInput     `json:"members"`
	Teams         *[]teamMemberInput `json:"teams"`
	// legacy (v9) field
	RecipientUsernames []string `json:"recipient_usernames"`
}

func validateShareInput(in *shareInput, userID int) error {
	if in.AccessMode != nil {
		switch *in.AccessMode {
		case "members", "users":
		case "link":
			if !settingBool("allow_link_shares") && !isAdminUser(userID) {
				return fmt.Errorf("the administrator has disabled shares for people without an account")
			}
		default:
			return fmt.Errorf("invalid access mode")
		}
	}
	if in.GuestRole != nil && !validAssignableRole(*in.GuestRole, roleRank[RoleOperator]) {
		return fmt.Errorf("invalid default role")
	}
	if in.ExpiresAt != nil && *in.ExpiresAt != "" {
		t, ok := parseTS(*in.ExpiresAt)
		if !ok {
			return fmt.Errorf("invalid expiry date")
		}
		if t.Before(time.Now()) {
			return fmt.Errorf("the expiry date is in the past")
		}
		*in.ExpiresAt = t.UTC().Format(time.RFC3339)
	}
	if in.Name != nil {
		*in.Name = truncateStr(strings.TrimSpace(*in.Name), 120)
	}
	if in.Members != nil {
		for i, m := range *in.Members {
			if m.Role == "" {
				(*in.Members)[i].Role = RoleOperator
			} else if !validAssignableRole(m.Role, roleRank[RoleModerator]) {
				return fmt.Errorf("invalid role %q", m.Role)
			}
		}
	}
	if in.Teams != nil {
		for i, t := range *in.Teams {
			if t.Role == "" {
				(*in.Teams)[i].Role = RoleOperator
			} else if !validAssignableRole(t.Role, roleRank[RoleModerator]) {
				return fmt.Errorf("invalid role %q", t.Role)
			}
		}
	}
	return nil
}

func applyShareItems(tx *sql.Tx, shareID, ownerID int, connIDs, folderIDs []int) {
	tx.Exec(`DELETE FROM share_items WHERE share_id=?`, shareID)
	for _, cid := range connIDs {
		if userOwnsConnection(cid, ownerID) {
			tx.Exec(`INSERT INTO share_items (share_id, connection_id) VALUES (?,?)`, shareID, cid)
		}
	}
	for _, fid := range folderIDs {
		if userOwnsFolder(fid, ownerID) {
			tx.Exec(`INSERT INTO share_items (share_id, folder_id) VALUES (?,?)`, shareID, fid)
		}
	}
}

func applyShareMembers(tx *sql.Tx, shareID, ownerID int, members []memberInput) []string {
	var unknown []string
	keep := map[int]bool{}
	for _, m := range members {
		uid := m.UserID
		if uid == 0 {
			if tx.QueryRow(`SELECT id FROM users WHERE username=?`, strings.TrimSpace(m.Username)).Scan(&uid) != nil {
				if strings.TrimSpace(m.Username) != "" {
					unknown = append(unknown, m.Username)
				}
				continue
			}
		}
		if uid == ownerID || keep[uid] {
			continue
		}
		keep[uid] = true
		tx.Exec(`INSERT INTO share_members (share_id, user_id, role, added_at) VALUES (?,?,?,?)
			ON CONFLICT(share_id, user_id) DO UPDATE SET role=excluded.role`, shareID, uid, m.Role, time.Now().UTC().Format(time.RFC3339))
		tx.Exec(`UPDATE share_participants SET banned=0 WHERE share_id=? AND pkey=?`, shareID, "u:"+strconv.Itoa(uid))
	}
	rows, err := tx.Query(`SELECT user_id FROM share_members WHERE share_id=?`, shareID)
	if err == nil {
		var drop []int
		for rows.Next() {
			var uid int
			rows.Scan(&uid)
			if !keep[uid] {
				drop = append(drop, uid)
			}
		}
		rows.Close()
		for _, uid := range drop {
			tx.Exec(`DELETE FROM share_members WHERE share_id=? AND user_id=?`, shareID, uid)
		}
	}
	return unknown
}

func shareDetails(s ShareLink) map[string]interface{} {
	items := []map[string]interface{}{}
	connIDs, folderIDs := []int{}, []int{}
	if rows, err := db.Query(`SELECT si.connection_id, si.folder_id, COALESCE(c.name, f.name, '') FROM share_items si
		LEFT JOIN connections c ON c.id=si.connection_id LEFT JOIN folders f ON f.id=si.folder_id WHERE si.share_id=?`, s.ID); err == nil {
		for rows.Next() {
			var cid, fid sql.NullInt64
			var name string
			rows.Scan(&cid, &fid, &name)
			if cid.Valid {
				connIDs = append(connIDs, int(cid.Int64))
				items = append(items, map[string]interface{}{"type": "connection", "id": cid.Int64, "name": name})
			} else if fid.Valid {
				folderIDs = append(folderIDs, int(fid.Int64))
				items = append(items, map[string]interface{}{"type": "folder", "id": fid.Int64, "name": name})
			}
		}
		rows.Close()
	}
	members := []map[string]interface{}{}
	if rows, err := db.Query(`SELECT m.user_id, u.username, COALESCE(u.display_name,''), m.role, COALESCE(m.added_at,'') FROM share_members m
		JOIN users u ON u.id=m.user_id WHERE m.share_id=? ORDER BY u.username`, s.ID); err == nil {
		for rows.Next() {
			var uid int
			var username, dn, role, added string
			rows.Scan(&uid, &username, &dn, &role, &added)
			members = append(members, map[string]interface{}{"user_id": uid, "username": username, "display_name": dn, "role": role, "added_at": added})
		}
		rows.Close()
	}
	var owner string
	db.QueryRow(`SELECT username FROM users WHERE id=?`, s.OwnerID).Scan(&owner)
	return map[string]interface{}{
		"id": s.ID, "token": s.Token, "name": s.Name, "owner_id": s.OwnerID, "owner_username": owner,
		"access_mode": s.AccessMode, "guest_role": s.GuestRole, "public": s.Public,
		"password_protected": s.PasswordHash != "", "expires_at": s.ExpiresAt, "expired": s.expired(),
		"active": s.Active, "created_at": s.CreatedAt, "connection_ids": connIDs, "folder_ids": folderIDs,
		"items": items, "members": members, "teams": shareTeamsOf(s.ID), "online": roomOnlineCount(s.ID),
		"connections": len(sharedConnectionsFor(s)),
	}
}

// GET /api/shares (mine) · POST /api/shares (create)
func apiSharesHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT `+shareCols+` FROM share_links WHERE owner_id=? ORDER BY created_at DESC`, userID)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		var list []ShareLink
		for rows.Next() {
			if s, err := scanShare(rows); err == nil {
				list = append(list, s)
			}
		}
		rows.Close()
		out := []map[string]interface{}{}
		for _, s := range list {
			out = append(out, shareDetails(s))
		}
		jsonOK(w, out)
	case http.MethodPost:
		var in shareInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if in.AccessMode == nil {
			mode := "members"
			if in.Public != nil && *in.Public {
				mode = "link"
			}
			in.AccessMode = &mode
		}
		if in.Members == nil && len(in.RecipientUsernames) > 0 {
			ms := []memberInput{}
			for _, u := range in.RecipientUsernames {
				ms = append(ms, memberInput{Username: u, Role: RoleOperator})
			}
			in.Members = &ms
		}
		if err := validateShareInput(&in, userID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		name := "Shared connections"
		if in.Name != nil && *in.Name != "" {
			name = *in.Name
		}
		guestRole := RoleViewer
		if in.GuestRole != nil {
			guestRole = *in.GuestRole
		}
		var ph interface{}
		if in.Password != nil && *in.Password != "" {
			if len(*in.Password) > 72 {
				jsonError(w, "Password too long", 400)
				return
			}
			h, err := hashPassword(*in.Password)
			if err != nil {
				jsonError(w, "Server error", 500)
				return
			}
			ph = h
		}
		public := in.Public != nil && *in.Public
		expires := ""
		if in.ExpiresAt != nil {
			expires = *in.ExpiresAt
		}
		if err := checkLinkShareExpiry(*in.AccessMode, expires); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		token := randomToken(24)
		tx, err := db.Begin()
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		defer tx.Rollback()
		res, err := tx.Exec(`INSERT INTO share_links (owner_id, token, name, password_hash, public, active, created_at, access_mode, guest_role, expires_at) VALUES (?,?,?,?,?,1,?,?,?,?)`,
			userID, token, name, ph, boolToInt(public), time.Now().UTC().Format(time.RFC3339), *in.AccessMode, guestRole, expires)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		id64, _ := res.LastInsertId()
		shareID := int(id64)
		var connIDs, folderIDs []int
		if in.ConnectionIDs != nil {
			connIDs = *in.ConnectionIDs
		}
		if in.FolderIDs != nil {
			folderIDs = *in.FolderIDs
		}
		applyShareItems(tx, shareID, userID, connIDs, folderIDs)
		var unknown []string
		if in.Members != nil {
			unknown = applyShareMembers(tx, shareID, userID, *in.Members)
		}
		if in.Teams != nil {
			applyShareTeams(tx, shareID, *in.Teams)
		}
		if err := tx.Commit(); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		auditLog(r, userID, "share.created", name, map[string]interface{}{"share_id": shareID, "access_mode": *in.AccessMode, "guest_role": guestRole,
			"password": ph != nil, "connections": len(connIDs), "folders": len(folderIDs), "expires_at": expires})
		s, _ := getShareByID(shareID)
		out := shareDetails(s)
		out["share_url"] = "/share/" + token
		out["unknown_users"] = unknown
		jsonOK(w, out)
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// /api/shares/{id}                      GET · PUT · DELETE
// /api/shares/{id}/rotate               POST  (new link, old link stops working)
// /api/shares/{id}/members              GET · POST {username|user_id, role}
// /api/shares/{id}/members/{userID}     PUT {role} · DELETE
// /api/shares/{id}/participants         GET  (everyone who ever joined)
// /api/shares/{id}/participants/{pid}   PUT {role?, banned?}
func apiSharesByIDHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/shares/"), "/"), "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "Bad ID", 400)
		return
	}
	s, err := getShareByID(id)
	if err != nil || !canManageShare(userID, s) {
		jsonError(w, "Not found", 404)
		return
	}
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	subID := ""
	if len(parts) > 2 {
		subID = parts[2]
	}
	switch {
	case sub == "" && r.Method == http.MethodGet:
		jsonOK(w, shareDetails(s))
	case sub == "" && r.Method == http.MethodPut:
		var in shareInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if err := validateShareInput(&in, s.OwnerID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if (in.AccessMode != nil && *in.AccessMode != s.AccessMode) || (in.ExpiresAt != nil && *in.ExpiresAt != s.ExpiresAt) {
			mode, exp := s.AccessMode, s.ExpiresAt
			if in.AccessMode != nil {
				mode = *in.AccessMode
			}
			if in.ExpiresAt != nil {
				exp = *in.ExpiresAt
			}
			if err := checkLinkShareExpiry(mode, exp); err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
		}
		tx, err := db.Begin()
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		defer tx.Rollback()
		changes := map[string]interface{}{}
		if in.Name != nil && *in.Name != "" && *in.Name != s.Name {
			tx.Exec(`UPDATE share_links SET name=? WHERE id=?`, *in.Name, id)
			changes["name"] = *in.Name
		}
		if in.AccessMode != nil && *in.AccessMode != s.AccessMode {
			tx.Exec(`UPDATE share_links SET access_mode=? WHERE id=?`, *in.AccessMode, id)
			changes["access_mode"] = *in.AccessMode
		}
		if in.GuestRole != nil && *in.GuestRole != s.GuestRole {
			tx.Exec(`UPDATE share_links SET guest_role=? WHERE id=?`, *in.GuestRole, id)
			changes["guest_role"] = *in.GuestRole
		}
		if in.ClearPassword && s.PasswordHash != "" {
			tx.Exec(`UPDATE share_links SET password_hash=NULL WHERE id=?`, id)
			changes["password"] = "removed"
		} else if in.Password != nil && *in.Password != "" {
			h, err := hashPassword(*in.Password)
			if err != nil {
				jsonError(w, "Server error", 500)
				return
			}
			tx.Exec(`UPDATE share_links SET password_hash=? WHERE id=?`, h, id)
			changes["password"] = "changed"
		}
		if in.Public != nil && *in.Public != s.Public {
			tx.Exec(`UPDATE share_links SET public=? WHERE id=?`, boolToInt(*in.Public), id)
			changes["public"] = *in.Public
		}
		if in.ExpiresAt != nil && *in.ExpiresAt != s.ExpiresAt {
			tx.Exec(`UPDATE share_links SET expires_at=? WHERE id=?`, *in.ExpiresAt, id)
			changes["expires_at"] = *in.ExpiresAt
		}
		if in.Active != nil && *in.Active != s.Active {
			tx.Exec(`UPDATE share_links SET active=? WHERE id=?`, boolToInt(*in.Active), id)
			changes["active"] = *in.Active
		}
		if in.ConnectionIDs != nil || in.FolderIDs != nil {
			d := shareDetails(s)
			connIDs, folderIDs := d["connection_ids"].([]int), d["folder_ids"].([]int)
			if in.ConnectionIDs != nil {
				connIDs = *in.ConnectionIDs
			}
			if in.FolderIDs != nil {
				folderIDs = *in.FolderIDs
			}
			applyShareItems(tx, id, s.OwnerID, connIDs, folderIDs)
			changes["items"] = len(connIDs) + len(folderIDs)
		}
		var unknown []string
		if in.Members != nil {
			unknown = applyShareMembers(tx, id, s.OwnerID, *in.Members)
			changes["members"] = len(*in.Members)
		}
		if in.Teams != nil {
			applyShareTeams(tx, id, *in.Teams)
			changes["teams"] = len(*in.Teams)
		}
		if err := tx.Commit(); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if len(changes) > 0 {
			auditLog(r, userID, "share.updated", s.Name, map[string]interface{}{"share_id": id, "changes": changes})
		}
		revalidateShare(id)
		ns, _ := getShareByID(id)
		out := shareDetails(ns)
		out["unknown_users"] = unknown
		jsonOK(w, out)
	case sub == "" && r.Method == http.MethodDelete:
		closeShareEverywhere(id, "The share was deleted by its owner")
		db.Exec(`DELETE FROM share_links WHERE id=?`, id)
		auditLog(r, userID, "share.deleted", s.Name, map[string]int{"share_id": id})
		jsonOK(w, map[string]bool{"ok": true})
	case sub == "rotate" && r.Method == http.MethodPost:
		token := randomToken(24)
		db.Exec(`UPDATE share_links SET token=? WHERE id=?`, token, id)
		closeShareEverywhere(id, "The share link was changed by its owner")
		auditLog(r, userID, "share.link_rotated", s.Name, map[string]int{"share_id": id})
		ns, _ := getShareByID(id)
		out := shareDetails(ns)
		out["share_url"] = "/share/" + token
		jsonOK(w, out)
	case sub == "members" && subID == "" && r.Method == http.MethodGet:
		jsonOK(w, shareDetails(s)["members"])
	case sub == "members" && subID == "" && r.Method == http.MethodPost:
		var m memberInput
		json.NewDecoder(r.Body).Decode(&m)
		if m.Role == "" {
			m.Role = RoleOperator
		}
		if !validAssignableRole(m.Role, roleRank[RoleModerator]) {
			jsonError(w, "Invalid role", 400)
			return
		}
		uid := m.UserID
		if uid == 0 && db.QueryRow(`SELECT id FROM users WHERE username=?`, strings.TrimSpace(m.Username)).Scan(&uid) != nil {
			jsonError(w, "User not found", 404)
			return
		}
		if uid == s.OwnerID {
			jsonError(w, "The owner is always a member", 400)
			return
		}
		db.Exec(`INSERT INTO share_members (share_id, user_id, role, added_at) VALUES (?,?,?,?)
			ON CONFLICT(share_id, user_id) DO UPDATE SET role=excluded.role`, id, uid, m.Role, time.Now().UTC().Format(time.RFC3339))
		db.Exec(`UPDATE share_participants SET banned=0, role='' WHERE share_id=? AND pkey=?`, id, "u:"+strconv.Itoa(uid))
		auditLog(r, userID, "share.member_added", s.Name, map[string]interface{}{"share_id": id, "user": usernameOf(uid), "role": m.Role})
		revalidateShare(id)
		jsonOK(w, shareDetails(s)["members"])
	case sub == "members" && subID != "" && (r.Method == http.MethodPut || r.Method == http.MethodDelete):
		uid, _ := strconv.Atoi(subID)
		if r.Method == http.MethodDelete {
			db.Exec(`DELETE FROM share_members WHERE share_id=? AND user_id=?`, id, uid)
			auditLog(r, userID, "share.member_removed", s.Name, map[string]interface{}{"share_id": id, "user": usernameOf(uid)})
		} else {
			var m memberInput
			json.NewDecoder(r.Body).Decode(&m)
			if !validAssignableRole(m.Role, roleRank[RoleModerator]) {
				jsonError(w, "Invalid role", 400)
				return
			}
			db.Exec(`UPDATE share_members SET role=? WHERE share_id=? AND user_id=?`, m.Role, id, uid)
			auditLog(r, userID, "share.member_role", s.Name, map[string]interface{}{"share_id": id, "user": usernameOf(uid), "role": m.Role})
		}
		revalidateShare(id)
		jsonOK(w, shareDetails(s)["members"])
	case sub == "participants" && subID == "" && r.Method == http.MethodGet:
		jsonOK(w, shareParticipants(s))
	case sub == "participants" && subID != "" && r.Method == http.MethodPut:
		pid, _ := strconv.Atoi(subID)
		var pkey, name string
		if db.QueryRow(`SELECT pkey, name FROM share_participants WHERE id=? AND share_id=?`, pid, id).Scan(&pkey, &name) != nil {
			jsonError(w, "Not found", 404)
			return
		}
		var p struct {
			Role   *string `json:"role"`
			Banned *bool   `json:"banned"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		if p.Role != nil {
			if *p.Role != "" && !validAssignableRole(*p.Role, roleRank[RoleModerator]) {
				jsonError(w, "Invalid role", 400)
				return
			}
			if *p.Role == "" {
				db.Exec(`UPDATE share_participants SET role='' WHERE id=?`, pid)
			} else {
				setParticipantRole(id, pkey, *p.Role)
			}
		}
		if p.Banned != nil {
			setParticipantBanned(id, pkey, *p.Banned)
		}
		auditLog(r, userID, "share.participant_updated", s.Name, map[string]interface{}{"share_id": id, "participant": name, "role": p.Role, "banned": p.Banned})
		revalidateShare(id)
		jsonOK(w, shareParticipants(s))
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func shareParticipants(s ShareLink) []map[string]interface{} {
	out := []map[string]interface{}{}
	online := roomOnlinePKeys(s.ID)
	rows, err := db.Query(`SELECT p.id, p.pkey, COALESCE(p.user_id,0), p.name, p.role, p.banned, COALESCE(p.first_seen,''), COALESCE(p.last_seen,''), COALESCE(p.last_ip,''),
		COALESCE(u.username,''), COALESCE(m.role,'') FROM share_participants p LEFT JOIN users u ON u.id=p.user_id
		LEFT JOIN share_members m ON m.share_id=p.share_id AND m.user_id=p.user_id WHERE p.share_id=? AND p.first_seen != '' ORDER BY p.last_seen DESC`, s.ID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, uid, banned int
		var pkey, name, role, first, last, ip, username, memberRole string
		rows.Scan(&id, &pkey, &uid, &name, &role, &banned, &first, &last, &ip, &username, &memberRole)
		effective := role
		if effective == "" {
			effective = memberRole
		}
		if effective == "" {
			effective = s.GuestRole
		}
		if uid == s.OwnerID {
			effective = RoleOwner
		}
		out = append(out, map[string]interface{}{
			"id": id, "name": name, "username": username, "guest": strings.HasPrefix(pkey, "g:"), "member": memberRole != "",
			"role": effective, "role_override": role, "banned": banned == 1, "first_seen": first, "last_seen": last,
			"last_ip": ip, "online": online[pkey],
		})
	}
	return out
}

// ─── SHARE API (recipient side) ──────────────────────
// GET  /api/share/{token}          share info, my role, connections
// POST /api/share/{token}/unlock   {password}
// POST /api/share/{token}/profile  {name}   (guests choose how others see them)
func apiShareByTokenHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/share/"), "/"), "/")
	token := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	share, err := getShareByToken(token)
	if err != nil || token == "" {
		jsonError(w, "Share not found or deleted", 404)
		return
	}
	switch action {
	case "":
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		ensureGuestCookie(w, r)
		sa, d := resolveShareAccess(r, share)
		if d != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(d.Code)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": d.Message, "reason": d.Reason, "password_required": d.Reason == "password_required",
				"login_required": d.Reason == "login_required", "share": map[string]string{"name": share.Name},
			})
			return
		}
		var owner string
		db.QueryRow("SELECT username FROM users WHERE id=?", share.OwnerID).Scan(&owner)
		conns := []SharedConn{}
		folders := []SharedFolder{}
		if roleCan(sa.Role, PermFilesRead) {
			conns = sharedConnectionsFor(share)
			folders = sharedFoldersFor(share, conns)
		}
		jsonOK(w, map[string]interface{}{
			"share": map[string]interface{}{
				"id": share.ID, "token": share.Token, "name": share.Name, "owner_username": owner,
				"access_mode": share.AccessMode, "expires_at": share.ExpiresAt,
			},
			"me":          map[string]interface{}{"name": sa.Name, "role": sa.Role, "guest": sa.Guest, "member": sa.Member, "owner": sa.IsOwner, "perms": rolePerms(sa.Role), "named": !sa.Guest || participantHasName(share.ID, sa.PKey)},
			"connections": conns,
			"folders":     folders,
			"voice":       settingBool("voice_enabled"),
		})
	case "unlock":
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", 405)
			return
		}
		ip := clientIP(r)
		if blocked, wait := loginBlocked("share:" + ip); blocked {
			jsonError(w, fmt.Sprintf("Too many wrong passwords. Try again in %d s.", int(wait.Seconds())+1), 429)
			return
		}
		var p struct {
			Password string `json:"password"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		if share.PasswordHash == "" {
			jsonOK(w, map[string]bool{"ok": true})
			return
		}
		if !checkPassword(share.PasswordHash, p.Password) {
			recordLoginResult("share:"+ip, false)
			auditLogAs(r, 0, "", "share.unlock_failed", share.Name, map[string]int{"share_id": share.ID})
			jsonError(w, "Invalid password", 401)
			return
		}
		recordLoginResult("share:"+ip, true)
		setCookie(w, r, sharePwCookieName(share.ID), sharePwCookieValue(share), 7*86400)
		jsonOK(w, map[string]bool{"ok": true})
	case "profile":
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", 405)
			return
		}
		sa, d := resolveShareAccess(r, share)
		if d != nil {
			jsonError(w, d.Message, d.Code)
			return
		}
		if !sa.Guest {
			jsonError(w, "Signed-in users use their account name", 400)
			return
		}
		var p struct {
			Name string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		name := cleanDisplayName(p.Name)
		if name == "" {
			jsonError(w, "Enter a name", 400)
			return
		}
		sa.Name = name
		touchParticipant(sa, clientIP(r))
		db.Exec(`UPDATE share_participants SET name=? WHERE share_id=? AND pkey=?`, name, share.ID, sa.PKey)
		jsonOK(w, map[string]string{"name": name})
	default:
		jsonError(w, "Not found", 404)
	}
}

func participantHasName(shareID int, pkey string) bool {
	var name string
	db.QueryRow(`SELECT name FROM share_participants WHERE share_id=? AND pkey=?`, shareID, pkey).Scan(&name)
	return name != ""
}

// cleanDisplayName keeps printable characters only and limits the length.
func cleanDisplayName(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if r >= 32 && r != 127 && r != '<' && r != '>' {
			b.WriteRune(r)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if len([]rune(out)) > 40 {
		out = string([]rune(out)[:40])
	}
	return out
}

// GET /api/shared — shares I can open from my sidebar ("Shared with me")
func apiSharedHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", 405)
		return
	}
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rows, err := db.Query(`SELECT `+shareCols+` FROM share_links WHERE owner_id != ? AND active=1 AND (
			id IN (SELECT share_id FROM share_members WHERE user_id=?) OR (public=1 AND access_mode IN ('users','link'))
			OR id IN (SELECT st.share_id FROM share_teams st JOIN team_members tm ON tm.team_id=st.team_id WHERE tm.user_id=?)
		) ORDER BY created_at DESC`, userID, userID, userID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var list []ShareLink
	for rows.Next() {
		if s, err := scanShare(rows); err == nil && !s.expired() {
			list = append(list, s)
		}
	}
	rows.Close()
	results := []map[string]interface{}{}
	for _, s := range list {
		sa, d := resolveShareAccess(r, s)
		role := ""
		locked := false
		if d != nil {
			if d.Reason != "password_required" {
				continue
			}
			locked = true
		} else {
			role = sa.Role
		}
		var owner string
		db.QueryRow("SELECT username FROM users WHERE id=?", s.OwnerID).Scan(&owner)
		conns := []SharedConn{}
		folders := []SharedFolder{}
		if !locked && roleCan(role, PermFilesRead) {
			conns = sharedConnectionsFor(s)
			folders = sharedFoldersFor(s, conns)
		}
		results = append(results, map[string]interface{}{
			"share": map[string]interface{}{
				"id": s.ID, "token": s.Token, "name": s.Name, "owner_username": owner, "role": role,
				"perms": rolePerms(role), "locked": locked, "password_protected": s.PasswordHash != "",
				"access_mode": s.AccessMode,
			},
			"connections": conns,
			"folders":     folders,
		})
	}
	jsonOK(w, results)
}

// GET /api/admin/shares — every share on the server (admins can pause or delete them)
func apiAdminSharesHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}
	rows, err := db.Query(`SELECT ` + shareCols + ` FROM share_links ORDER BY created_at DESC`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var list []ShareLink
	for rows.Next() {
		if s, err := scanShare(rows); err == nil {
			list = append(list, s)
		}
	}
	rows.Close()
	out := []map[string]interface{}{}
	for _, s := range list {
		d := shareDetails(s)
		delete(d, "token")
		out = append(out, d)
	}
	jsonOK(w, out)
}

// revalidateShare re-checks everyone who is currently connected through the share (room
// and terminals) after its settings, members or roles changed.
// revalidateFolderShares rechecks the owner's folder shares after a folder moved or was
// deleted: a connection that left a shared subtree loses its shared terminals.
func revalidateFolderShares(ownerID int) {
	rows, err := db.Query(`SELECT DISTINCT l.id FROM share_links l JOIN share_items i ON i.share_id=l.id WHERE l.owner_id=? AND i.folder_id IS NOT NULL`, ownerID)
	if err != nil {
		return
	}
	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		go revalidateShare(id)
	}
}

func revalidateShare(shareID int) {
	share, err := getShareByID(shareID)
	if err != nil {
		closeShareEverywhere(shareID, "The share was removed")
		return
	}
	check := func(ctx accessCtx) (*ShareAccess, *shareDenied) { return resolveShareAccessCtx(share, ctx) }
	killTerminals(func(t *termSession) bool {
		if t.ShareID != shareID {
			return false
		}
		sa, d := check(t.Ctx)
		return d != nil || !roleCan(sa.Role, PermTerminal) || !shareHasConnection(share, t.ConnID)
	}, "Your access to this terminal was revoked")
	revalidateRoom(shareID, check)
}

// closeShareEverywhere disconnects everyone using a share (deleted, rotated, owner removed).
func closeShareEverywhere(shareID int, reason string) {
	killTerminals(func(t *termSession) bool { return t.ShareID == shareID }, reason)
	closeRoom(shareID, reason)
}

// Owner-scoped recursion makes a shared folder include its nested folders.
const shareFolderSubtreeSQL = `WITH RECURSIVE shared_folders(id) AS (
 SELECT f.id FROM folders f JOIN share_items si ON si.folder_id=f.id WHERE si.share_id=? AND f.user_id=?
 UNION SELECT f.id FROM folders f JOIN shared_folders sf ON f.parent_id=sf.id WHERE f.user_id=?
) SELECT id FROM shared_folders`
