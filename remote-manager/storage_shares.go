// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─── STORAGE SHARES (read-only links to a prefix) ─────────────────────────────────────────
//
// A share is a token that opens one prefix of one storage in a browser without a WRM account.
// The token is stored hashed, expires, is read-only, and every request through it stays inside
// the share prefix (which is itself inside the storage prefix).

const (
	storageShareDefaultHours = 24
	storageShareMaxHours     = 720
)

type storageShare struct {
	ID        int    `json:"id"`
	StorageID int    `json:"storage_id"`
	Prefix    string `json:"prefix"`
	CanWrite  bool   `json:"can_write"`
	CanDelete bool   `json:"can_delete"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
	URL       string `json:"url,omitempty"` // only in the answer that creates it
}

func storageSharesFor(storageID int) []storageShare {
	rows, err := db.Query(`SELECT id, storage_id, prefix, can_write, can_delete, expires_at, created_at
		FROM storage_shares WHERE storage_id=? ORDER BY id DESC`, storageID)
	if err != nil {
		return []storageShare{}
	}
	defer rows.Close()
	out := []storageShare{}
	for rows.Next() {
		var s storageShare
		var w, d int
		if rows.Scan(&s.ID, &s.StorageID, &s.Prefix, &w, &d, &s.ExpiresAt, &s.CreatedAt) == nil {
			s.CanWrite, s.CanDelete = w == 1, d == 1
			out = append(out, s)
		}
	}
	return out
}

// apiStorageShares serves POST/GET /api/storages/{id}/shares and DELETE .../shares/{shareId}.
func apiStorageShares(w http.ResponseWriter, r *http.Request) {
	s, userID, ok := requireStorage(w, r, "read")
	if !ok {
		return
	}
	if s.UserID != userID && !isAdminUser(userID) {
		jsonError(w, "Not found", http.StatusNotFound)
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/api/storages/%d/shares", s.ID)), "/")
	if rest != "" {
		if r.Method != http.MethodDelete {
			jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id, err := strconv.Atoi(strings.Split(rest, "/")[0])
		if err != nil {
			jsonError(w, "Not found", http.StatusNotFound)
			return
		}
		res, err := db.Exec(`DELETE FROM storage_shares WHERE id=? AND storage_id=?`, id, s.ID)
		if err != nil {
			jsonError(w, "Server error", http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			jsonError(w, "Not found", http.StatusNotFound)
			return
		}
		auditLog(r, userID, "storage.share_revoked", s.Name, map[string]interface{}{"share": id})
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, storageSharesFor(s.ID))
	case http.MethodPost:
		var in struct {
			Prefix string `json:"prefix"`
			Hours  int    `json:"hours"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
			jsonError(w, "Invalid request", http.StatusBadRequest)
			return
		}
		prefix, err := normalizePrefix(in.Prefix)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		hours := in.Hours
		if hours <= 0 {
			hours = storageShareDefaultHours
		}
		if hours > storageShareMaxHours {
			jsonError(w, fmt.Sprintf("a link can be valid for at most %d hours", storageShareMaxHours), http.StatusBadRequest)
			return
		}
		if n := len(storageSharesFor(s.ID)); n >= 50 {
			jsonError(w, "too many links for this storage; revoke one first", http.StatusBadRequest)
			return
		}
		raw := randomToken(24)
		now := time.Now().UTC()
		exp := now.Add(time.Duration(hours) * time.Hour)
		res, err := db.Exec(`INSERT INTO storage_shares (storage_id, token_hash, prefix, can_write, can_delete, expires_at, created_by, created_at)
			VALUES (?,?,?,0,0,?,?,?)`, s.ID, tokenHash(raw), prefix, exp.Format(time.RFC3339), userID, now.Format(time.RFC3339))
		if err != nil {
			jsonError(w, "Server error", http.StatusInternalServerError)
			return
		}
		id, _ := res.LastInsertId()
		auditLog(r, userID, "storage.share_created", s.Name, map[string]interface{}{"prefix": prefix, "hours": hours, "share": id})
		jsonOK(w, storageShare{ID: int(id), StorageID: s.ID, Prefix: prefix,
			ExpiresAt: exp.Format(time.RFC3339), CreatedAt: now.Format(time.RFC3339), URL: "/s3/" + raw})
	default:
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// storageShareByToken returns the storage a token opens, with the prefix narrowed to the share.
func storageShareByToken(token string) (Storage, storageShare, error) {
	var sh storageShare
	var storageID int
	err := db.QueryRow(`SELECT id, storage_id, prefix, can_write, can_delete, expires_at, created_at
		FROM storage_shares WHERE token_hash=?`, tokenHash(token)).
		Scan(&sh.ID, &storageID, &sh.Prefix, new(int), new(int), &sh.ExpiresAt, &sh.CreatedAt)
	if err != nil {
		return Storage{}, sh, fmt.Errorf("not found")
	}
	if exp, ok := parseTS(sh.ExpiresAt); !ok || time.Now().After(exp) {
		return Storage{}, sh, fmt.Errorf("this link has expired")
	}
	var owner int
	if err := db.QueryRow(`SELECT user_id FROM storages WHERE id=?`, storageID).Scan(&owner); err != nil {
		return Storage{}, sh, fmt.Errorf("not found")
	}
	s, err := loadStorage(storageID, owner)
	if err != nil {
		return Storage{}, sh, fmt.Errorf("not found")
	}
	// The share prefix is relative to the storage prefix; the effective client prefix is both.
	if sh.Prefix != "" {
		if s.Prefix == "" {
			s.Prefix = sh.Prefix
		} else {
			s.Prefix = s.Prefix + "/" + sh.Prefix
		}
	}
	s.AllowWrite, s.AllowDelete, s.AllowUpload = false, false, false
	return s, sh, nil
}

// storageShareHandler serves the public page and the downloads of a share.
func storageShareHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/s3/")
	token, tail := rest, ""
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		token, tail = rest[:i], rest[i+1:]
	}
	if token == "" || len(token) > 96 {
		http.NotFound(w, r)
		return
	}
	s, sh, err := storageShareByToken(token)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !mcpRateOK("s3share:"+clientIP(r), 120, time.Minute) {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}
	if tail == "object" {
		storageShareObject(w, r, s, sh)
		return
	}
	if tail != "" {
		http.NotFound(w, r)
		return
	}
	storageSharePage(w, r, token, s, sh)
}

func storageShareObject(w http.ResponseWriter, r *http.Request, s Storage, sh storageShare) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := r.URL.Query().Get("key")
	if err := validStorageKey(key); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	c, err := s.clientFor()
	if err != nil {
		http.Error(w, "The storage is not available", http.StatusBadGateway)
		return
	}
	resp, err := c.get(r.Context(), key, r.Header.Get("Range"))
	if err != nil {
		http.Error(w, "The object could not be read", http.StatusNotFound)
		return
	}
	defer resp.Body.Close()
	ct := storageContentType(resp.Header.Get("Content-Type"), key)
	h := w.Header()
	if unsafeInline(ct) {
		h.Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		h.Set("Content-Type", ct)
		if r.URL.Query().Get("inline") == "1" {
			h.Set("Content-Disposition", "inline; filename=\""+path.Base(key)+"\"")
		} else {
			h.Set("Content-Disposition", "attachment; filename=\""+path.Base(key)+"\"")
		}
	}
	for _, name := range []string{"Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(name); v != "" {
			h.Set(name, v)
		}
	}
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Robots-Tag", "noindex, nofollow")
	if resp.StatusCode == http.StatusPartialContent {
		w.WriteHeader(http.StatusPartialContent)
	}
	io.Copy(w, resp.Body)
}

// storageSharePage is a plain server-rendered listing: no script, so the strict CSP of the app
// does not need an exception for a page that an anonymous visitor can reach.
func storageSharePage(w http.ResponseWriter, r *http.Request, token string, s Storage, sh storageShare) {
	prefix := strings.Trim(strings.TrimSpace(r.URL.Query().Get("prefix")), "/")
	if prefix != "" {
		if err := validStorageKey(prefix); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		prefix += "/"
	}
	c, err := s.clientFor()
	if err != nil {
		http.Error(w, "The storage is not available", http.StatusBadGateway)
		return
	}
	list, err := c.list(r.Context(), prefix, "", 500)
	if err != nil {
		http.Error(w, "The storage is not available", http.StatusBadGateway)
		return
	}
	sort.Strings(list.Prefixes)
	sort.Slice(list.Objects, func(i, j int) bool { return list.Objects[i].Key < list.Objects[j].Key })

	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"robots\" content=\"noindex,nofollow\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	b.WriteString("<title>" + html.EscapeString(s.Name) + "</title><style>")
	b.WriteString("body{font-family:system-ui,sans-serif;margin:0;background:#0f1216;color:#e6edf3}" +
		"main{max-width:900px;margin:0 auto;padding:28px 20px}h1{font-size:18px;margin:0 0 4px}" +
		".muted{color:#8b949e;font-size:12px;margin-bottom:18px}ul{list-style:none;padding:0;margin:0}" +
		"li{display:flex;gap:12px;align-items:center;padding:9px 10px;border-radius:8px}" +
		"li:hover{background:#161b22}a{color:#58a6ff;text-decoration:none;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}" +
		".s{color:#8b949e;font-size:12px}footer{margin-top:24px;color:#8b949e;font-size:11px}" +
		"</style></head><body><main>")
	b.WriteString("<h1>" + html.EscapeString(s.Name) + "</h1>")
	b.WriteString("<div class=\"muted\">" + html.EscapeString(strings.TrimSuffix(s.Prefix, "/")) + " · read-only link</div>")
	b.WriteString("<ul>")
	if prefix != "" {
		parent := prefix
		if i := strings.LastIndex(strings.TrimSuffix(parent, "/"), "/"); i > 0 {
			parent = parent[:i+1]
		} else {
			parent = ""
		}
		b.WriteString("<li><a href=\"?prefix=" + url.QueryEscape(parent) + "\">⬆ ..</a></li>")
	}
	for _, p := range list.Prefixes {
		name := strings.TrimPrefix(p, prefix)
		b.WriteString("<li><a href=\"?prefix=" + url.QueryEscape(p) + "\">📁 " + html.EscapeString(name) + "</a></li>")
	}
	for _, o := range list.Objects {
		name := strings.TrimPrefix(o.Key, prefix)
		link := "/s3/" + token + "/object?key=" + url.QueryEscape(o.Key)
		b.WriteString("<li><a href=\"" + html.EscapeString(link) + "\">📄 " + html.EscapeString(name) + "</a>" +
			"<span class=\"s\">" + humanSize(o.Size) + "</span></li>")
	}
	if len(list.Prefixes) == 0 && len(list.Objects) == 0 {
		b.WriteString("<li class=\"muted\">This folder is empty.</li>")
	}
	b.WriteString("</ul><footer>Shared with WRM PRO · link expires " + html.EscapeString(sh.ExpiresAt) + "</footer></main></body></html>")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	io.WriteString(w, b.String())
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return strconv.FormatFloat(float64(n)/(1<<30), 'f', 1, 64) + " GB"
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
	case n >= 1<<10:
		return strconv.FormatFloat(float64(n)/(1<<10), 'f', 1, 64) + " KB"
	}
	return strconv.FormatInt(n, 10) + " B"
}

// storageSharesCleanup drops expired links; called from the status loop.
func storageSharesCleanup() {
	db.Exec(`DELETE FROM storage_shares WHERE expires_at<>'' AND expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
}
