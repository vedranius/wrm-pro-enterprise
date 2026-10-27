// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

// SCIM 2.0 provisioning (RFC 7643 / 7644), v12.6.25.
//
// An identity provider (Microsoft Entra ID, Okta, …) creates, changes, disables and deletes
// WRM accounts and groups through /scim/v2 with a bearer token an administrator created in
// WRM. Only accounts the provisioning client created are visible to it: it can never read or
// change local accounts, and names never link accounts.
//
//   - Users become WRM accounts without a password; they sign in through the directory
//     (OIDC: the account is linked on its first sign-in when SCIM's externalId equals the
//     token's oid (Entra) or sub).
//   - active=false or DELETE disables the account at once and ends all its access
//     (endUserAccess); WRM keeps the account so its work can be handed over.
//   - Groups become WRM teams; membership follows the group. Mapping rules on the client turn
//     group names into WRM roles (the strongest wins).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	scimSchemaUser      = "urn:ietf:params:scim:schemas:core:2.0:User"
	scimSchemaGroup     = "urn:ietf:params:scim:schemas:core:2.0:Group"
	scimSchemaList      = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	scimSchemaError     = "urn:ietf:params:scim:api:messages:2.0:Error"
	scimSchemaPatch     = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	scimClientKeyPrefix = "wrm_scim_"
	scimMaxBody         = 1 << 20
	scimMaxGroupMembers = 20000
)

type scimClient struct {
	ID    int
	Name  string
	Rules []oidcRule
}

func scimError(w http.ResponseWriter, status int, scimType, detail string) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)
	body := map[string]interface{}{"schemas": []string{scimSchemaError}, "status": strconv.Itoa(status), "detail": detail}
	if scimType != "" {
		body["scimType"] = scimType
	}
	json.NewEncoder(w).Encode(body)
}

func scimJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// scimAuth finds the provisioning client of a bearer token.
func scimAuth(r *http.Request) (*scimClient, bool) {
	h := r.Header.Get("Authorization")
	if len(h) < 8 || !strings.EqualFold(h[:7], "bearer ") {
		return nil, false
	}
	tok := strings.TrimSpace(h[7:])
	if !strings.HasPrefix(tok, scimClientKeyPrefix) || len(tok) > 200 {
		return nil, false
	}
	var c scimClient
	var rules, lastUsed string
	err := db.QueryRow(`SELECT id, name, rules, last_used_at FROM scim_clients WHERE token_hash=? AND revoked_at=''`, sha256Hex(tok)).Scan(&c.ID, &c.Name, &rules, &lastUsed)
	if err != nil {
		return nil, false
	}
	json.Unmarshal([]byte(rules), &c.Rules)
	if t, ok := parseTS(lastUsed); !ok || time.Since(t) > 30*time.Second {
		db.Exec(`UPDATE scim_clients SET last_used_at=?, last_ip=? WHERE id=?`, nowRFC(), clientIP(r), c.ID)
	}
	return &c, true
}

// /scim/v2/...
func scimHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !mcpRateOK("scim:"+clientIP(r), 600, time.Minute) {
		scimError(w, 429, "", "Too many requests")
		return
	}
	c, ok := scimAuth(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="WRM SCIM"`)
		scimError(w, 401, "", "A valid SCIM bearer token is required (Admin panel → Directory sign-in → SCIM).")
		return
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, scimMaxBody)
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/scim/v2"), "/")
	parts := strings.Split(rest, "/")
	switch {
	case rest == "ServiceProviderConfig":
		scimJSON(w, 200, scimServiceProviderConfig(r))
	case rest == "ResourceTypes":
		scimJSON(w, 200, scimListOf(scimResourceTypes(r)))
	case rest == "Schemas":
		scimJSON(w, 200, scimListOf([]interface{}{map[string]interface{}{"id": scimSchemaUser, "name": "User"}, map[string]interface{}{"id": scimSchemaGroup, "name": "Group"}}))
	case parts[0] == "Users" && len(parts) <= 2:
		id := ""
		if len(parts) == 2 {
			id = parts[1]
		}
		scimUsers(w, r, c, id)
	case parts[0] == "Groups" && len(parts) <= 2:
		id := ""
		if len(parts) == 2 {
			id = parts[1]
		}
		scimGroups(w, r, c, id)
	default:
		scimError(w, 404, "", "Not found")
	}
}

func scimListOf(items []interface{}) map[string]interface{} {
	return map[string]interface{}{"schemas": []string{scimSchemaList}, "totalResults": len(items), "startIndex": 1, "itemsPerPage": len(items), "Resources": items}
}

func scimServiceProviderConfig(r *http.Request) map[string]interface{} {
	return map[string]interface{}{
		"schemas":          []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"documentationUri": "https://github.com/vedranius/wrm-pro-enterprise#scim-provisioning",
		"patch":            map[string]bool{"supported": true},
		"bulk":             map[string]interface{}{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":           map[string]interface{}{"supported": true, "maxResults": 200},
		"changePassword":   map[string]bool{"supported": false},
		"sort":             map[string]bool{"supported": false},
		"etag":             map[string]bool{"supported": false},
		"authenticationSchemes": []map[string]interface{}{{
			"type": "oauthbearertoken", "name": "Bearer token", "description": "A token created in WRM (Admin panel → Directory sign-in → SCIM).", "primary": true,
		}},
		"meta": map[string]string{"resourceType": "ServiceProviderConfig", "location": mcpBaseURL(r) + "/scim/v2/ServiceProviderConfig"},
	}
}

func scimResourceTypes(r *http.Request) []interface{} {
	base := mcpBaseURL(r) + "/scim/v2"
	return []interface{}{
		map[string]interface{}{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"}, "id": "User", "name": "User", "endpoint": "/Users", "schema": scimSchemaUser, "meta": map[string]string{"resourceType": "ResourceType", "location": base + "/ResourceTypes/User"}},
		map[string]interface{}{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"}, "id": "Group", "name": "Group", "endpoint": "/Groups", "schema": scimSchemaGroup, "meta": map[string]string{"resourceType": "ResourceType", "location": base + "/ResourceTypes/Group"}},
	}
}

// scimFilter parses the one filter form provisioning clients use: attr eq "value".
func scimFilter(f string) (attr, value string, err error) {
	f = strings.TrimSpace(f)
	if f == "" {
		return "", "", nil
	}
	i := strings.Index(strings.ToLower(f), " eq ")
	if i <= 0 {
		return "", "", errors.New("only filters of the form attribute eq \"value\" are supported")
	}
	attr = strings.TrimSpace(f[:i])
	v := strings.TrimSpace(f[i+4:])
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		if err := json.Unmarshal([]byte(v), &value); err != nil {
			return "", "", errors.New("bad filter value")
		}
	} else {
		value = v
	}
	return attr, value, nil
}

func scimPaging(r *http.Request) (start, count int) {
	start, count = atoiOr(r.URL.Query().Get("startIndex"), 1), atoiOr(r.URL.Query().Get("count"), 100)
	if start < 1 {
		start = 1
	}
	if count < 0 {
		count = 0
	}
	if count > 200 {
		count = 200
	}
	return
}

// scimBool reads a SCIM boolean (Entra sends "True"/"False" strings in PATCH).
func scimBool(v interface{}) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		switch strings.ToLower(strings.TrimSpace(b)) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}

func scimStr(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(s)
	}
	return ""
}

// ─── users ───────────────────────────────────────────

type scimUser struct {
	UserID     int
	ClientID   int
	SCIMID     string
	ExternalID string
	UserName   string
	Active     bool
	Data       map[string]interface{} // name, displayName, emails, title… as sent
	Created    string
	Updated    string
}

const scimUserCols = `u.user_id, u.client_id, u.scim_id, u.external_id, u.user_name, u.active, u.data, u.created_at, u.updated_at`

func scanSCIMUser(sc interface{ Scan(...interface{}) error }) (*scimUser, error) {
	var s scimUser
	var active int
	var data string
	if err := sc.Scan(&s.UserID, &s.ClientID, &s.SCIMID, &s.ExternalID, &s.UserName, &active, &data, &s.Created, &s.Updated); err != nil {
		return nil, err
	}
	s.Active = active == 1
	json.Unmarshal([]byte(data), &s.Data)
	if s.Data == nil {
		s.Data = map[string]interface{}{}
	}
	return &s, nil
}

func loadSCIMUser(clientID int, scimID string) (*scimUser, error) {
	return scanSCIMUser(db.QueryRow(`SELECT `+scimUserCols+` FROM scim_users u WHERE u.client_id=? AND u.scim_id=? AND u.deleted=0`, clientID, scimID))
}

func (s *scimUser) resource(r *http.Request) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range s.Data {
		out[k] = v
	}
	out["schemas"] = []string{scimSchemaUser}
	out["id"], out["userName"], out["active"] = s.SCIMID, s.UserName, s.Active
	if s.ExternalID != "" {
		out["externalId"] = s.ExternalID
	}
	var groups []map[string]string
	rows, err := db.Query(`SELECT g.scim_id, g.display_name FROM scim_group_members m JOIN scim_groups g ON g.id=m.group_id WHERE m.user_id=? AND g.client_id=? ORDER BY g.display_name`, s.UserID, s.ClientID)
	if err == nil {
		for rows.Next() {
			var id, name string
			rows.Scan(&id, &name)
			groups = append(groups, map[string]string{"value": id, "display": name})
		}
		rows.Close()
	}
	if len(groups) > 0 {
		out["groups"] = groups
	}
	out["meta"] = map[string]string{"resourceType": "User", "created": s.Created, "lastModified": s.Updated, "location": mcpBaseURL(r) + "/scim/v2/Users/" + s.SCIMID}
	return out
}

// scimUserFields are the attributes WRM keeps as sent (and shows); others are ignored.
var scimUserFields = map[string]bool{"name": true, "displayName": true, "emails": true, "title": true, "preferredLanguage": true, "nickName": true}

func scimDisplayName(s *scimUser) string {
	if v := scimStr(s.Data["displayName"]); v != "" {
		return v
	}
	if n, ok := s.Data["name"].(map[string]interface{}); ok {
		if v := scimStr(n["formatted"]); v != "" {
			return v
		}
		return strings.TrimSpace(scimStr(n["givenName"]) + " " + scimStr(n["familyName"]))
	}
	return ""
}

func scimEmail(s *scimUser) string {
	list, _ := s.Data["emails"].([]interface{})
	first := ""
	for _, e := range list {
		m, _ := e.(map[string]interface{})
		v := scimStr(m["value"])
		if p, _ := scimBool(m["primary"]); p && v != "" {
			return v
		}
		if first == "" {
			first = v
		}
	}
	return first
}

// scimApplyUser copies a full user resource (POST, PUT) into s.
func scimApplyUser(s *scimUser, in map[string]interface{}) error {
	name := strings.TrimSpace(scimStr(in["userName"]))
	if name == "" || len(name) > 256 {
		return errors.New("userName is required")
	}
	s.UserName = name
	s.ExternalID = truncateStr(strings.TrimSpace(scimStr(in["externalId"])), 256)
	s.Active = true
	if v, ok := in["active"]; ok {
		if b, ok := scimBool(v); ok {
			s.Active = b
		}
	}
	s.Data = map[string]interface{}{}
	for k, v := range in {
		if scimUserFields[k] {
			s.Data[k] = v
		}
	}
	return nil
}

// scimPatchUser applies PATCH operations to s.
func scimPatchUser(s *scimUser, ops []scimOp) error {
	for _, op := range ops {
		kind := strings.ToLower(op.Op)
		if kind != "add" && kind != "replace" && kind != "remove" {
			return fmt.Errorf("unknown PATCH op %q", op.Op)
		}
		if op.Path == "" {
			m, ok := op.Value.(map[string]interface{})
			if !ok {
				return errors.New("a PATCH operation without path needs an object value")
			}
			for k, v := range m {
				if err := scimSetUserPath(s, kind, k, v); err != nil {
					return err
				}
			}
			continue
		}
		if err := scimSetUserPath(s, kind, op.Path, op.Value); err != nil {
			return err
		}
	}
	return nil
}

func scimSetUserPath(s *scimUser, kind, path string, v interface{}) error {
	p := strings.TrimPrefix(path, scimSchemaUser+":")
	switch lp := strings.ToLower(p); {
	case lp == "active":
		if kind == "remove" {
			return nil
		}
		b, ok := scimBool(v)
		if !ok {
			return errors.New("active must be true or false")
		}
		s.Active = b
	case lp == "username":
		if n := strings.TrimSpace(scimStr(v)); n != "" && kind != "remove" {
			s.UserName = truncateStr(n, 256)
		}
	case lp == "externalid":
		if kind == "remove" {
			s.ExternalID = ""
		} else {
			s.ExternalID = truncateStr(strings.TrimSpace(scimStr(v)), 256)
		}
	case strings.HasPrefix(lp, "name."):
		n, _ := s.Data["name"].(map[string]interface{})
		if n == nil {
			n = map[string]interface{}{}
		}
		key := p[len("name."):]
		if kind == "remove" {
			delete(n, key)
		} else {
			n[key] = scimStr(v)
		}
		s.Data["name"] = n
	case strings.HasPrefix(lp, "emails"):
		// emails[type eq "work"].value or emails
		if kind == "remove" {
			delete(s.Data, "emails")
			return nil
		}
		if list, ok := v.([]interface{}); ok {
			s.Data["emails"] = list
			return nil
		}
		typ := "work"
		if i, j := strings.Index(p, `"`), strings.LastIndex(p, `"`); i >= 0 && j > i {
			typ = p[i+1 : j]
		}
		s.Data["emails"] = []interface{}{map[string]interface{}{"value": scimStr(v), "type": typ, "primary": true}}
	default:
		top := p
		if i := strings.IndexAny(top, ".["); i >= 0 {
			top = top[:i]
		}
		if scimUserFields[top] {
			if kind == "remove" {
				delete(s.Data, top)
			} else if top == p {
				s.Data[top] = v
			}
		}
		// Other attributes (enterprise extension, phone numbers…) are accepted and ignored.
	}
	return nil
}

type scimOp struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

func scimDecode(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, scimMaxBody)).Decode(v); err != nil {
		scimError(w, 400, "invalidSyntax", "Bad JSON")
		return false
	}
	return true
}

func scimUsers(w http.ResponseWriter, r *http.Request, c *scimClient, id string) {
	switch {
	case id == "" && r.Method == http.MethodGet:
		attr, value, err := scimFilter(r.URL.Query().Get("filter"))
		if err != nil {
			scimError(w, 400, "invalidFilter", err.Error())
			return
		}
		q := `SELECT ` + scimUserCols + ` FROM scim_users u WHERE u.client_id=? AND u.deleted=0`
		args := []interface{}{c.ID}
		switch strings.ToLower(attr) {
		case "":
		case "username":
			q += ` AND u.user_name=? COLLATE NOCASE`
			args = append(args, value)
		case "externalid":
			q += ` AND u.external_id=?`
			args = append(args, value)
		case "id":
			q += ` AND u.scim_id=?`
			args = append(args, value)
		default:
			scimError(w, 400, "invalidFilter", "filter on userName, externalId or id")
			return
		}
		scimListUsers(w, r, q, args)
	case id == "" && r.Method == http.MethodPost:
		var in map[string]interface{}
		if !scimDecode(w, r, &in) {
			return
		}
		s := &scimUser{ClientID: c.ID}
		if err := scimApplyUser(s, in); err != nil {
			scimError(w, 400, "invalidValue", err.Error())
			return
		}
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM scim_users WHERE client_id=? AND user_name=? COLLATE NOCASE AND deleted=0`, c.ID, s.UserName).Scan(&n)
		if n > 0 {
			scimError(w, 409, "uniqueness", "A user with this userName already exists")
			return
		}
		if err := scimCreateUser(r, c, s); err != nil {
			scimError(w, 500, "", err.Error())
			return
		}
		scimJSON(w, 201, s.resource(r))
	case id != "":
		s, err := loadSCIMUser(c.ID, id)
		if err != nil {
			scimError(w, 404, "", "User not found")
			return
		}
		switch r.Method {
		case http.MethodGet:
			scimJSON(w, 200, s.resource(r))
		case http.MethodPut:
			var in map[string]interface{}
			if !scimDecode(w, r, &in) {
				return
			}
			before := *s
			if err := scimApplyUser(s, in); err != nil {
				scimError(w, 400, "invalidValue", err.Error())
				return
			}
			scimSaveUser(r, c, &before, s)
			scimJSON(w, 200, s.resource(r))
		case http.MethodPatch:
			var in struct {
				Operations []scimOp `json:"Operations"`
			}
			if !scimDecode(w, r, &in) {
				return
			}
			before := *s
			before.Data = map[string]interface{}{}
			for k, v := range s.Data {
				before.Data[k] = v
			}
			if err := scimPatchUser(s, in.Operations); err != nil {
				scimError(w, 400, "invalidValue", err.Error())
				return
			}
			scimSaveUser(r, c, &before, s)
			scimJSON(w, 200, s.resource(r))
		case http.MethodDelete:
			scimDeleteUser(r, c, s)
			w.WriteHeader(204)
		default:
			scimError(w, 405, "", "Method not allowed")
		}
	default:
		scimError(w, 405, "", "Method not allowed")
	}
}

func scimListUsers(w http.ResponseWriter, r *http.Request, q string, args []interface{}) {
	start, count := scimPaging(r)
	var total int
	db.QueryRow(`SELECT COUNT(*) FROM (`+q+`)`, args...).Scan(&total)
	rows, err := db.Query(q+` ORDER BY u.created_at, u.user_id LIMIT ? OFFSET ?`, append(args, count, start-1)...)
	items := []interface{}{}
	if err == nil {
		var list []*scimUser
		for rows.Next() {
			if s, err := scanSCIMUser(rows); err == nil {
				list = append(list, s)
			}
		}
		rows.Close()
		for _, s := range list {
			items = append(items, s.resource(r))
		}
	}
	scimJSON(w, 200, map[string]interface{}{"schemas": []string{scimSchemaList}, "totalResults": total, "startIndex": start, "itemsPerPage": len(items), "Resources": items})
}

func scimCreateUser(r *http.Request, c *scimClient, s *scimUser) error {
	username := oidcUsername(&oidcProvider{UsernameClaim: "preferred_username"}, oidcClaims{
		"preferred_username": s.UserName, "email": scimEmail(s), "name": scimDisplayName(s), "iss": "scim", "sub": s.UserName,
	})
	now := nowRFC()
	role := userRoleUser // groups (and the role rules) follow with the group requests
	res, err := db.Exec(`INSERT INTO users (username, password_hash, is_admin, role, display_name, created_at, password_changed_at, sso_only, disabled) VALUES (?,?,?,?,?,?,?,1,?)`,
		username, "!sso", boolToInt(role == userRoleAdmin), storedRole(role), truncateStr(scimDisplayName(s), 100), now, now, boolToInt(!s.Active))
	if err != nil {
		return errors.New("WRM could not create the account")
	}
	uid, _ := res.LastInsertId()
	s.UserID, s.SCIMID, s.Created, s.Updated = int(uid), randomUUIDish(), now, now
	data, _ := json.Marshal(s.Data)
	if _, err := db.Exec(`INSERT INTO scim_users (user_id, client_id, scim_id, external_id, user_name, active, data, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		s.UserID, c.ID, s.SCIMID, s.ExternalID, s.UserName, boolToInt(s.Active), string(data), now, now); err != nil {
		db.Exec(`DELETE FROM users WHERE id=?`, s.UserID)
		return errors.New("WRM could not create the account")
	}
	auditLogAs(r, s.UserID, username, "scim.user_created", username, map[string]interface{}{"client": c.Name, "user_name": s.UserName, "active": s.Active})
	return nil
}

func scimSaveUser(r *http.Request, c *scimClient, before, s *scimUser) {
	now := nowRFC()
	s.Updated = now
	data, _ := json.Marshal(s.Data)
	db.Exec(`UPDATE scim_users SET external_id=?, user_name=?, active=?, data=?, updated_at=? WHERE user_id=?`, s.ExternalID, s.UserName, boolToInt(s.Active), string(data), now, s.UserID)
	if dn := scimDisplayName(s); dn != "" {
		db.Exec(`UPDATE users SET display_name=? WHERE id=?`, truncateStr(dn, 100), s.UserID)
	}
	username := usernameOf(s.UserID)
	changes := map[string]interface{}{"client": c.Name}
	if before.UserName != s.UserName {
		changes["user_name"] = s.UserName
	}
	if before.ExternalID != s.ExternalID {
		changes["external_id_changed"] = true
	}
	if before.Active != s.Active {
		changes["active"] = s.Active
		scimSetActive(r, s.UserID, s.Active, "the identity provider deactivated the account ("+c.Name+")")
	}
	auditLogAs(r, s.UserID, username, "scim.user_updated", username, changes)
}

// scimSetActive enables or disables a provisioned account; disabling ends all its access.
func scimSetActive(r *http.Request, userID int, active bool, reason string) {
	if active {
		db.Exec(`UPDATE users SET disabled=0 WHERE id=?`, userID)
		return
	}
	if isAdminUser(userID) && activeAdminCount() <= 1 {
		auditLogAs(r, userID, usernameOf(userID), "scim.disable_refused", usernameOf(userID), map[string]string{"reason": "last active administrator"})
		return
	}
	db.Exec(`UPDATE users SET disabled=1 WHERE id=?`, userID)
	ended := endUserAccess(r, userID, 0, reason)
	auditLogAs(r, userID, usernameOf(userID), "scim.user_disabled", usernameOf(userID), ended)
}

// scimDeleteUser: the account is disabled and hidden from SCIM; WRM keeps it for handover.
func scimDeleteUser(r *http.Request, c *scimClient, s *scimUser) {
	scimSetActive(r, s.UserID, false, "the identity provider removed the account ("+c.Name+")")
	db.Exec(`UPDATE scim_users SET deleted=1, active=0, updated_at=? WHERE user_id=?`, nowRFC(), s.UserID)
	db.Exec(`DELETE FROM scim_group_members WHERE user_id=?`, s.UserID)
	scimSyncUserMapping(r, c, s.UserID)
	auditLogAs(r, s.UserID, usernameOf(s.UserID), "scim.user_deleted", usernameOf(s.UserID), map[string]string{"client": c.Name})
}

func randomUUIDish() string {
	h := randomToken(16)
	h = strings.NewReplacer("-", "a", "_", "b").Replace(h)
	for len(h) < 32 {
		h += "0"
	}
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// ─── groups ──────────────────────────────────────────

type scimGroup struct {
	ID          int
	ClientID    int
	SCIMID      string
	ExternalID  string
	DisplayName string
	TeamID      int
	Created     string
	Updated     string
}

const scimGroupCols = `id, client_id, scim_id, external_id, display_name, team_id, created_at, updated_at`

func scanSCIMGroup(sc interface{ Scan(...interface{}) error }) (*scimGroup, error) {
	var g scimGroup
	err := sc.Scan(&g.ID, &g.ClientID, &g.SCIMID, &g.ExternalID, &g.DisplayName, &g.TeamID, &g.Created, &g.Updated)
	return &g, err
}

func loadSCIMGroup(clientID int, scimID string) (*scimGroup, error) {
	return scanSCIMGroup(db.QueryRow(`SELECT `+scimGroupCols+` FROM scim_groups WHERE client_id=? AND scim_id=?`, clientID, scimID))
}

func (g *scimGroup) resource(r *http.Request, withMembers bool) map[string]interface{} {
	out := map[string]interface{}{"schemas": []string{scimSchemaGroup}, "id": g.SCIMID, "displayName": g.DisplayName,
		"meta": map[string]string{"resourceType": "Group", "created": g.Created, "lastModified": g.Updated, "location": mcpBaseURL(r) + "/scim/v2/Groups/" + g.SCIMID}}
	if g.ExternalID != "" {
		out["externalId"] = g.ExternalID
	}
	if withMembers {
		members := []map[string]string{}
		rows, err := db.Query(`SELECT su.scim_id, su.user_name FROM scim_group_members m JOIN scim_users su ON su.user_id=m.user_id WHERE m.group_id=? AND su.deleted=0 ORDER BY su.user_name`, g.ID)
		if err == nil {
			for rows.Next() {
				var id, name string
				rows.Scan(&id, &name)
				members = append(members, map[string]string{"value": id, "display": name})
			}
			rows.Close()
		}
		out["members"] = members
	}
	return out
}

// scimTeamFor returns the WRM team a group stands for, creating it when it is missing.
func scimTeamFor(r *http.Request, c *scimClient, name string) int {
	name = truncateStr(strings.TrimSpace(name), 60)
	if name == "" {
		return 0
	}
	// A team someone created by hand is never taken over by a SCIM group of the same name
	// (map groups to existing teams with the client's rules instead).
	desc := "Provisioned by " + c.Name + " (SCIM)"
	for _, cand := range []string{name, truncateStr(name, 52) + " (SCIM)"} {
		var id int
		var d string
		if db.QueryRow(`SELECT id, description FROM teams WHERE name=? COLLATE NOCASE`, cand).Scan(&id, &d) == nil {
			if d == desc {
				return id
			}
			continue
		}
		name = cand
		break
	}
	res, err := db.Exec(`INSERT INTO teams (name, description, created_at) VALUES (?,?,?)`, name, desc, nowRFC())
	if err != nil {
		return 0
	}
	nid, _ := res.LastInsertId()
	auditLog(r, 0, "scim.team_created", name, map[string]string{"client": c.Name})
	return int(nid)
}

// scimSyncUserMapping recomputes the role and the SCIM team memberships of one account from
// its groups.
func scimSyncUserMapping(r *http.Request, c *scimClient, userID int) {
	var managed int
	db.QueryRow(`SELECT COUNT(*) FROM scim_users WHERE user_id=? AND client_id=?`, userID, c.ID).Scan(&managed)
	if managed == 0 {
		return
	}
	var names []interface{}
	var teams []string
	rows, err := db.Query(`SELECT g.display_name, g.external_id, COALESCE(t.name,'') FROM scim_group_members m JOIN scim_groups g ON g.id=m.group_id
		LEFT JOIN teams t ON t.id=g.team_id WHERE m.user_id=? AND g.client_id=?`, userID, c.ID)
	if err != nil {
		return
	}
	for rows.Next() {
		var dn, ext, team string
		rows.Scan(&dn, &ext, &team)
		names = append(names, dn)
		if ext != "" {
			names = append(names, ext)
		}
		if team != "" {
			teams = append(teams, team)
		}
	}
	rows.Close()
	m := directoryMap(c.Rules, oidcClaims{"group": names, "groups": names})
	if !m.Matched {
		m.Role = userRoleUser
	}
	seen := map[string]bool{}
	for _, t := range m.Teams {
		seen[strings.ToLower(t)] = true
	}
	for _, t := range teams {
		if !seen[strings.ToLower(t)] {
			seen[strings.ToLower(t)] = true
			m.Teams = append(m.Teams, t)
		}
	}
	sort.Strings(m.Teams)
	directoryApplyMapping(r, userID, m, "scim")
}

func scimGroups(w http.ResponseWriter, r *http.Request, c *scimClient, id string) {
	switch {
	case id == "" && r.Method == http.MethodGet:
		attr, value, err := scimFilter(r.URL.Query().Get("filter"))
		if err != nil {
			scimError(w, 400, "invalidFilter", err.Error())
			return
		}
		q := `SELECT ` + scimGroupCols + ` FROM scim_groups WHERE client_id=?`
		args := []interface{}{c.ID}
		switch strings.ToLower(attr) {
		case "":
		case "displayname":
			q += ` AND display_name=? COLLATE NOCASE`
			args = append(args, value)
		case "externalid":
			q += ` AND external_id=?`
			args = append(args, value)
		case "id":
			q += ` AND scim_id=?`
			args = append(args, value)
		default:
			scimError(w, 400, "invalidFilter", "filter on displayName, externalId or id")
			return
		}
		start, count := scimPaging(r)
		var total int
		db.QueryRow(`SELECT COUNT(*) FROM (`+q+`)`, args...).Scan(&total)
		withMembers := !strings.Contains(r.URL.Query().Get("excludedAttributes"), "members")
		rows, err := db.Query(q+` ORDER BY created_at, id LIMIT ? OFFSET ?`, append(args, count, start-1)...)
		items := []interface{}{}
		if err == nil {
			var list []*scimGroup
			for rows.Next() {
				if g, err := scanSCIMGroup(rows); err == nil {
					list = append(list, g)
				}
			}
			rows.Close()
			for _, g := range list {
				items = append(items, g.resource(r, withMembers))
			}
		}
		scimJSON(w, 200, map[string]interface{}{"schemas": []string{scimSchemaList}, "totalResults": total, "startIndex": start, "itemsPerPage": len(items), "Resources": items})
	case id == "" && r.Method == http.MethodPost:
		var in struct {
			DisplayName string `json:"displayName"`
			ExternalID  string `json:"externalId"`
			Members     []struct {
				Value string `json:"value"`
			} `json:"members"`
		}
		if !scimDecode(w, r, &in) {
			return
		}
		name := truncateStr(strings.TrimSpace(in.DisplayName), 60)
		if name == "" {
			scimError(w, 400, "invalidValue", "displayName is required")
			return
		}
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM scim_groups WHERE client_id=? AND display_name=? COLLATE NOCASE`, c.ID, name).Scan(&n)
		if n > 0 {
			scimError(w, 409, "uniqueness", "A group with this displayName already exists")
			return
		}
		now := nowRFC()
		g := &scimGroup{ClientID: c.ID, SCIMID: randomUUIDish(), ExternalID: truncateStr(in.ExternalID, 256), DisplayName: name, TeamID: scimTeamFor(r, c, name), Created: now, Updated: now}
		res, err := db.Exec(`INSERT INTO scim_groups (client_id, scim_id, external_id, display_name, team_id, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
			g.ClientID, g.SCIMID, g.ExternalID, g.DisplayName, g.TeamID, now, now)
		if err != nil {
			scimError(w, 500, "", "WRM could not create the group")
			return
		}
		gid, _ := res.LastInsertId()
		g.ID = int(gid)
		var vals []string
		for _, m := range in.Members {
			vals = append(vals, m.Value)
		}
		scimSetMembers(r, c, g, "add", vals)
		auditLog(r, 0, "scim.group_created", name, map[string]interface{}{"client": c.Name, "members": len(vals)})
		scimJSON(w, 201, g.resource(r, true))
	case id != "":
		g, err := loadSCIMGroup(c.ID, id)
		if err != nil {
			scimError(w, 404, "", "Group not found")
			return
		}
		switch r.Method {
		case http.MethodGet:
			scimJSON(w, 200, g.resource(r, !strings.Contains(r.URL.Query().Get("excludedAttributes"), "members")))
		case http.MethodPut:
			var in struct {
				DisplayName string `json:"displayName"`
				ExternalID  string `json:"externalId"`
				Members     []struct {
					Value string `json:"value"`
				} `json:"members"`
			}
			if !scimDecode(w, r, &in) {
				return
			}
			scimRenameGroup(r, c, g, in.DisplayName)
			g.ExternalID = truncateStr(in.ExternalID, 256)
			db.Exec(`UPDATE scim_groups SET external_id=?, updated_at=? WHERE id=?`, g.ExternalID, nowRFC(), g.ID)
			var vals []string
			for _, m := range in.Members {
				vals = append(vals, m.Value)
			}
			scimSetMembers(r, c, g, "replace", vals)
			scimJSON(w, 200, g.resource(r, true))
		case http.MethodPatch:
			var in struct {
				Operations []scimOp `json:"Operations"`
			}
			if !scimDecode(w, r, &in) {
				return
			}
			if err := scimPatchGroup(r, c, g, in.Operations); err != nil {
				scimError(w, 400, "invalidValue", err.Error())
				return
			}
			g, _ = loadSCIMGroup(c.ID, id)
			scimJSON(w, 200, g.resource(r, true))
		case http.MethodDelete:
			members := scimGroupMembers(g.ID)
			db.Exec(`DELETE FROM scim_group_members WHERE group_id=?`, g.ID)
			db.Exec(`DELETE FROM scim_groups WHERE id=?`, g.ID)
			for _, uid := range members {
				scimSyncUserMapping(r, c, uid)
			}
			auditLog(r, 0, "scim.group_deleted", g.DisplayName, map[string]string{"client": c.Name})
			w.WriteHeader(204)
		default:
			scimError(w, 405, "", "Method not allowed")
		}
	default:
		scimError(w, 405, "", "Method not allowed")
	}
}

func scimGroupMembers(groupID int) []int {
	var out []int
	rows, err := db.Query(`SELECT user_id FROM scim_group_members WHERE group_id=?`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		rows.Scan(&id)
		out = append(out, id)
	}
	return out
}

func scimRenameGroup(r *http.Request, c *scimClient, g *scimGroup, name string) {
	name = truncateStr(strings.TrimSpace(name), 60)
	if name == "" || name == g.DisplayName {
		return
	}
	old := g.DisplayName
	g.DisplayName, g.TeamID = name, scimTeamFor(r, c, name)
	db.Exec(`UPDATE scim_groups SET display_name=?, team_id=?, updated_at=? WHERE id=?`, g.DisplayName, g.TeamID, nowRFC(), g.ID)
	for _, uid := range scimGroupMembers(g.ID) {
		scimSyncUserMapping(r, c, uid)
	}
	auditLog(r, 0, "scim.group_renamed", name, map[string]string{"client": c.Name, "from": old})
}

// scimSetMembers adds, removes or replaces group members (SCIM user ids of this client).
func scimSetMembers(r *http.Request, c *scimClient, g *scimGroup, kind string, scimIDs []string) {
	if len(scimIDs) > scimMaxGroupMembers {
		scimIDs = scimIDs[:scimMaxGroupMembers]
	}
	want := map[int]bool{}
	for _, sid := range scimIDs {
		var uid int
		if db.QueryRow(`SELECT user_id FROM scim_users WHERE client_id=? AND scim_id=? AND deleted=0`, c.ID, strings.TrimSpace(sid)).Scan(&uid) == nil {
			want[uid] = true
		}
	}
	touched := map[int]bool{}
	switch kind {
	case "replace":
		for _, uid := range scimGroupMembers(g.ID) {
			if !want[uid] {
				db.Exec(`DELETE FROM scim_group_members WHERE group_id=? AND user_id=?`, g.ID, uid)
				touched[uid] = true
			}
		}
		fallthrough
	case "add":
		for uid := range want {
			if res, err := db.Exec(`INSERT OR IGNORE INTO scim_group_members (group_id, user_id) VALUES (?,?)`, g.ID, uid); err == nil {
				if n, _ := res.RowsAffected(); n > 0 {
					touched[uid] = true
				}
			}
		}
	case "remove":
		for uid := range want {
			db.Exec(`DELETE FROM scim_group_members WHERE group_id=? AND user_id=?`, g.ID, uid)
			touched[uid] = true
		}
	}
	for uid := range touched {
		scimSyncUserMapping(r, c, uid)
	}
	if len(touched) > 0 {
		db.Exec(`UPDATE scim_groups SET updated_at=? WHERE id=?`, nowRFC(), g.ID)
	}
}

func scimMemberValues(v interface{}) []string {
	var out []string
	switch x := v.(type) {
	case []interface{}:
		for _, m := range x {
			if mm, ok := m.(map[string]interface{}); ok {
				out = append(out, scimStr(mm["value"]))
			}
		}
	case map[string]interface{}:
		out = append(out, scimStr(x["value"]))
	}
	return out
}

func scimPatchGroup(r *http.Request, c *scimClient, g *scimGroup, ops []scimOp) error {
	for _, op := range ops {
		kind := strings.ToLower(op.Op)
		path := strings.TrimPrefix(op.Path, scimSchemaGroup+":")
		lp := strings.ToLower(path)
		switch {
		case kind != "add" && kind != "replace" && kind != "remove":
			return fmt.Errorf("unknown PATCH op %q", op.Op)
		case lp == "":
			m, ok := op.Value.(map[string]interface{})
			if !ok {
				return errors.New("a PATCH operation without path needs an object value")
			}
			if dn := scimStr(m["displayName"]); dn != "" {
				scimRenameGroup(r, c, g, dn)
			}
			if ext, ok := m["externalId"]; ok {
				db.Exec(`UPDATE scim_groups SET external_id=? WHERE id=?`, truncateStr(scimStr(ext), 256), g.ID)
			}
			if mem, ok := m["members"]; ok {
				scimSetMembers(r, c, g, kind, scimMemberValues(mem))
			}
		case lp == "displayname":
			scimRenameGroup(r, c, g, scimStr(op.Value))
		case lp == "externalid":
			db.Exec(`UPDATE scim_groups SET external_id=? WHERE id=?`, truncateStr(scimStr(op.Value), 256), g.ID)
		case lp == "members":
			if kind == "remove" && op.Value == nil {
				scimSetMembers(r, c, g, "replace", nil)
				continue
			}
			scimSetMembers(r, c, g, kind, scimMemberValues(op.Value))
		case strings.HasPrefix(lp, "members[") && kind == "remove":
			// members[value eq "id"]
			_, val, err := scimFilter(strings.TrimSuffix(path[len("members["):], "]"))
			if err != nil {
				return err
			}
			scimSetMembers(r, c, g, "remove", []string{val})
		default:
			// Other attributes are accepted and ignored.
		}
	}
	return nil
}

// ─── OIDC link ───────────────────────────────────────

// scimUserForOIDC finds a provisioned account whose externalId is the directory ID in an
// ID token (Entra oid, or sub), so its first directory sign-in links to it.
func scimUserForOIDC(c oidcClaims) int {
	var ids []string
	if oid := c.str("oid"); oid != "" {
		ids = append(ids, oid)
	}
	if sub := c.str("sub"); sub != "" {
		ids = append(ids, sub)
	}
	for _, v := range ids {
		var uid int
		if db.QueryRow(`SELECT user_id FROM scim_users WHERE external_id=? COLLATE NOCASE AND deleted=0 AND active=1
			AND user_id NOT IN (SELECT user_id FROM user_identities) ORDER BY user_id LIMIT 1`, v).Scan(&uid) == nil {
			return uid
		}
	}
	return 0
}

// ─── admin API ───────────────────────────────────────

// GET    /api/admin/scim               clients, provisioned accounts, the endpoint address
// POST   /api/admin/scim               {name, rules} create a client; answers the token once
// PUT    /api/admin/scim/{id}          {name, rules}
// POST   /api/admin/scim/{id}/rotate   new token (the old one stops working)
// DELETE /api/admin/scim/{id}          revoke the client (its accounts stay)
func apiAdminSCIMHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/scim"), "/")
	parts := strings.Split(rest, "/")
	var in struct {
		Name  string      `json:"name"`
		Rules *[]oidcRule `json:"rules"`
	}
	decode := func() bool {
		if r.Body == nil {
			return true
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil && err != io.EOF {
			jsonError(w, "Bad JSON", 400)
			return false
		}
		return true
	}
	newToken := func() (string, string) {
		tok := scimClientKeyPrefix + randomToken(32)
		return tok, sha256Hex(tok)
	}
	switch {
	case rest == "" && r.Method == http.MethodGet:
		jsonOK(w, map[string]interface{}{"clients": scimClientList(), "users": scimUserList(), "endpoint": mcpBaseURL(r) + "/scim/v2"})
	case rest == "" && r.Method == http.MethodPost:
		if !requireStepUp(w, r, "scim.client") || !decode() {
			return
		}
		name := truncateStr(strings.TrimSpace(in.Name), 60)
		if name == "" {
			jsonError(w, "Enter a name, for example Entra ID provisioning", 400)
			return
		}
		rules := []oidcRule{}
		if in.Rules != nil {
			var err error
			if rules, err = validateOIDCRules(*in.Rules); err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
		}
		rj, _ := json.Marshal(rules)
		tok, hash := newToken()
		res, err := db.Exec(`INSERT INTO scim_clients (name, token_hash, rules, created_by, created_at) VALUES (?,?,?,?,?)`, name, hash, string(rj), adminID, nowRFC())
		if err != nil {
			jsonError(w, "A SCIM client with this name already exists", 400)
			return
		}
		id, _ := res.LastInsertId()
		auditLog(r, adminID, "scim.client_created", name, map[string]interface{}{"scim_client": id, "rules": len(rules)})
		jsonOK(w, map[string]interface{}{"id": id, "token": tok, "endpoint": mcpBaseURL(r) + "/scim/v2"})
	case len(parts) == 2 && parts[1] == "rotate" && r.Method == http.MethodPost:
		if !requireStepUp(w, r, "scim.client") {
			return
		}
		id := atoiOr(parts[0], 0)
		tok, hash := newToken()
		res, _ := db.Exec(`UPDATE scim_clients SET token_hash=?, revoked_at='' WHERE id=?`, hash, id)
		if n, _ := res.RowsAffected(); n == 0 {
			jsonError(w, "Not found", 404)
			return
		}
		auditLog(r, adminID, "scim.token_rotated", scimClientName(id), map[string]int{"scim_client": id})
		jsonOK(w, map[string]interface{}{"id": id, "token": tok})
	case len(parts) == 1 && rest != "" && r.Method == http.MethodPut:
		if !requireStepUp(w, r, "scim.client") || !decode() {
			return
		}
		id := atoiOr(parts[0], 0)
		if scimClientName(id) == "" {
			jsonError(w, "Not found", 404)
			return
		}
		if name := truncateStr(strings.TrimSpace(in.Name), 60); name != "" {
			if _, err := db.Exec(`UPDATE scim_clients SET name=? WHERE id=?`, name, id); err != nil {
				jsonError(w, "A SCIM client with this name already exists", 400)
				return
			}
		}
		if in.Rules != nil {
			rules, err := validateOIDCRules(*in.Rules)
			if err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
			rj, _ := json.Marshal(rules)
			db.Exec(`UPDATE scim_clients SET rules=? WHERE id=?`, string(rj), id)
			c := &scimClient{ID: id, Name: scimClientName(id), Rules: rules}
			rows, err := db.Query(`SELECT user_id FROM scim_users WHERE client_id=? AND deleted=0`, id)
			var uids []int
			if err == nil {
				for rows.Next() {
					var uid int
					rows.Scan(&uid)
					uids = append(uids, uid)
				}
				rows.Close()
			}
			for _, uid := range uids {
				scimSyncUserMapping(r, c, uid)
			}
		}
		auditLog(r, adminID, "scim.client_updated", scimClientName(id), map[string]int{"scim_client": id})
		jsonOK(w, map[string]bool{"ok": true})
	case len(parts) == 1 && rest != "" && r.Method == http.MethodDelete:
		if !requireStepUp(w, r, "scim.client") {
			return
		}
		id := atoiOr(parts[0], 0)
		res, _ := db.Exec(`UPDATE scim_clients SET revoked_at=? WHERE id=? AND revoked_at=''`, nowRFC(), id)
		if n, _ := res.RowsAffected(); n == 0 {
			jsonError(w, "Not found", 404)
			return
		}
		auditLog(r, adminID, "scim.client_revoked", scimClientName(id), map[string]int{"scim_client": id})
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Not found", 404)
	}
}

func scimClientName(id int) string {
	var name string
	db.QueryRow(`SELECT name FROM scim_clients WHERE id=?`, id).Scan(&name)
	return name
}

func scimClientList() []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := db.Query(`SELECT c.id, c.name, c.rules, c.created_at, c.last_used_at, c.last_ip, c.revoked_at,
		(SELECT COUNT(*) FROM scim_users u WHERE u.client_id=c.id AND u.deleted=0), (SELECT COUNT(*) FROM scim_groups g WHERE g.client_id=c.id)
		FROM scim_clients c ORDER BY c.name`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, users, groups int
		var name, rules, created, lastUsed, lastIP, revoked string
		if rows.Scan(&id, &name, &rules, &created, &lastUsed, &lastIP, &revoked, &users, &groups) != nil {
			continue
		}
		var rl []oidcRule
		json.Unmarshal([]byte(rules), &rl)
		if rl == nil {
			rl = []oidcRule{}
		}
		out = append(out, map[string]interface{}{"id": id, "name": name, "rules": rl, "created_at": created, "last_used_at": lastUsed, "last_ip": lastIP,
			"revoked": revoked != "", "users": users, "groups": groups})
	}
	return out
}

func scimUserList() []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := db.Query(`SELECT s.user_id, COALESCE(u.username,''), s.user_name, s.external_id, s.active, s.deleted, s.updated_at, COALESCE(c.name,''), COALESCE(u.disabled,0)
		FROM scim_users s LEFT JOIN users u ON u.id=s.user_id LEFT JOIN scim_clients c ON c.id=s.client_id ORDER BY s.updated_at DESC LIMIT 500`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var uid, active, deleted, disabled int
		var username, userName, ext, updated, client string
		if rows.Scan(&uid, &username, &userName, &ext, &active, &deleted, &updated, &client, &disabled) != nil {
			continue
		}
		out = append(out, map[string]interface{}{"user_id": uid, "username": username, "user_name": userName, "external_id": ext,
			"active": active == 1, "deleted": deleted == 1, "disabled": disabled == 1, "updated_at": updated, "client": client})
	}
	return out
}
