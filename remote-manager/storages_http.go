// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ─── STORAGE OBJECT ROUTES ────────────────────────────────────────────────────────────────
//
// Reading is always allowed; saving needs allow_write, adding a new object needs allow_upload
// and deleting needs allow_delete with an explicit confirmation. Everything is relative to the
// storage prefix: a key can never address anything outside it.

// storageRoute splits the part of the path after /api/storages/{id}/.
func storageRoute(r *http.Request) []string {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/storages/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		return nil
	}
	return parts[1:]
}

func storageObjectBytes(s Storage) int64 { return int64(s.MaxObjectMB) << 20 }

func storageEditBytes() int64 {
	kb := settingInt("storage_edit_kb")
	if kb <= 0 {
		kb = storageDefaultEditKB
	}
	if kb > storageMaxEditKB {
		kb = storageMaxEditKB
	}
	return int64(kb) << 10
}

// storageQueryKey reads and validates the key parameter.
func storageQueryKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := r.URL.Query().Get("key")
	if err := validStorageKey(key); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return "", false
	}
	return key, true
}

// apiStorageObjects lists one page of a prefix.
func apiStorageObjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s, _, ok := requireStorage(w, r, "read")
	if !ok {
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("prefix"))
	rel = strings.TrimPrefix(rel, "/")
	if rel != "" {
		if err := validStorageKey(strings.TrimSuffix(rel, "/")); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !strings.HasSuffix(rel, "/") {
			rel += "/"
		}
	}
	c, err := s.clientFor()
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := c.list(r.Context(), rel, r.URL.Query().Get("cursor"), 500)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	jsonOK(w, map[string]interface{}{
		"prefix": rel, "prefixes": res.Prefixes, "objects": res.Objects,
		"next": res.Next, "truncated": res.Truncated,
		"capabilities": s.capabilities(), "max_object_mb": s.MaxObjectMB,
	})
}

// apiStorageObject serves, stores and deletes one object.
func apiStorageObject(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		storageServeObject(w, r, false)
	case http.MethodPut:
		storageStoreObject(w, r)
	case http.MethodDelete:
		storageDeleteObject(w, r)
	default:
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// unsafeInline reports types that must never be rendered in the WRM origin: an HTML or SVG
// document would run script with the signed-in user's session.
func unsafeInline(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch ct {
	case "text/html", "application/xhtml+xml", "image/svg+xml", "text/javascript",
		"application/javascript", "application/ecmascript", "text/ecmascript":
		return true
	}
	return false
}

func storageContentType(ct, key string) string {
	if ct == "" || strings.EqualFold(ct, "binary/octet-stream") || strings.EqualFold(ct, "application/octet-stream") {
		if byExt := mime.TypeByExtension(path.Ext(key)); byExt != "" {
			return byExt
		}
	}
	return ct
}

func storageServeObject(w http.ResponseWriter, r *http.Request, head bool) {
	s, _, ok := requireStorage(w, r, "read")
	if !ok {
		return
	}
	key, ok := storageQueryKey(w, r)
	if !ok {
		return
	}
	c, err := s.clientFor()
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	inline := r.URL.Query().Get("inline") == "1"
	if head {
		meta, err := c.head(r.Context(), key)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadGateway)
			return
		}
		ct := storageContentType(meta.ContentType, key)
		if unsafeInline(ct) {
			ct = "text/plain; charset=utf-8"
		}
		h := w.Header()
		h.Set("Content-Type", ct)
		h.Set("Content-Length", strconv.FormatInt(meta.Size, 10))
		h.Set("ETag", `"`+meta.ETag+`"`)
		h.Set("Accept-Ranges", "bytes")
		h.Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		return
	}
	resp, err := c.get(r.Context(), key, r.Header.Get("Range"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	ct := storageContentType(resp.Header.Get("Content-Type"), key)
	h := w.Header()
	if unsafeInline(ct) {
		// Never inline: the browser would execute it in the WRM origin.
		h.Set("Content-Type", "text/plain; charset=utf-8")
		h.Set("Content-Disposition", "attachment; filename=\""+path.Base(key)+"\"")
	} else {
		h.Set("Content-Type", ct)
		if inline {
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
	if h.Get("Accept-Ranges") == "" {
		h.Set("Accept-Ranges", "bytes")
	}
	if resp.StatusCode == http.StatusPartialContent {
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	io.Copy(w, resp.Body)
}

func storageStoreObject(w http.ResponseWriter, r *http.Request) {
	s, userID, ok := requireStorage(w, r, "read")
	if !ok {
		return
	}
	key, ok := storageQueryKey(w, r)
	if !ok {
		return
	}
	c, err := s.clientFor()
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	meta, headErr := c.head(r.Context(), key)
	exists := headErr == nil
	_ = meta
	capability := "upload"
	if exists {
		capability = "write"
	}
	if !s.capabilities()[capability] {
		auditLog(r, userID, "storage.denied", s.Name, map[string]interface{}{"capability": capability, "key": key})
		jsonError(w, capabilityMessage(capability), http.StatusForbidden)
		return
	}
	limit := storageObjectBytes(s)
	if limit <= 0 {
		limit = int64(storageDefaultMaxObjectMB) << 20
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		jsonError(w, fmt.Sprintf("the file is larger than %d MB", limit>>20), http.StatusRequestEntityTooLarge)
		return
	}
	if int64(len(body)) > limit {
		jsonError(w, fmt.Sprintf("the file is larger than %d MB", limit>>20), http.StatusRequestEntityTooLarge)
		return
	}
	ct := storageContentType(r.Header.Get("Content-Type"), key)
	if ct == "" {
		ct = "application/octet-stream"
	}
	etag, err := c.put(r.Context(), key, body, ct, r.Header.Get("If-Match"))
	if err != nil {
		jsonError(w, err.Error(), s3ErrorStatus(err))
		return
	}
	s.auditFile(r, userID, "storage.file_written", key, len(body), map[string]interface{}{"created": !exists})
	jsonOK(w, map[string]interface{}{"key": key, "etag": etag, "size": len(body)})
}

func storageDeleteObject(w http.ResponseWriter, r *http.Request) {
	s, userID, ok := requireStorage(w, r, "delete")
	if !ok {
		return
	}
	if r.Header.Get("X-WRM-Confirm") != "delete" {
		jsonError(w, "Deleting needs an explicit confirmation", http.StatusBadRequest)
		return
	}
	if !requireStepUp(w, r, "storage.delete") {
		return
	}
	key, ok := storageQueryKey(w, r)
	if !ok {
		return
	}
	c, err := s.clientFor()
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	var size int64
	if meta, err := c.head(r.Context(), key); err == nil {
		size = meta.Size
	}
	if err := c.delete(r.Context(), key); err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	s.auditFile(r, userID, "storage.file_deleted", key, int(size), nil)
	jsonOK(w, map[string]bool{"ok": true})
}

// apiStorageDir creates an empty "folder" (a zero-byte marker object).
func apiStorageDir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s, userID, ok := requireStorage(w, r, "write")
	if !ok {
		return
	}
	dir := strings.Trim(strings.TrimSpace(r.URL.Query().Get("prefix")), "/")
	if dir == "" {
		jsonError(w, "give the folder a name", http.StatusBadRequest)
		return
	}
	if err := validStorageKey(dir); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	c, err := s.clientFor()
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := c.put(r.Context(), dir+"/", nil, "application/x-directory", ""); err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	s.auditFile(r, userID, "storage.folder_created", dir+"/", 0, nil)
	jsonOK(w, map[string]string{"prefix": dir + "/"})
}

// apiStorageMove renames (copies and deletes) one object.
func apiStorageMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s, userID, ok := requireStorage(w, r, "write")
	if !ok {
		return
	}
	var in struct {
		From      string `json:"from"`
		To        string `json:"to"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
		jsonError(w, "Invalid request", http.StatusBadRequest)
		return
	}
	for _, k := range []string{in.From, in.To} {
		if err := validStorageKey(k); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if in.From == in.To {
		jsonOK(w, map[string]string{"key": in.To})
		return
	}
	c, err := s.clientFor()
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := c.head(r.Context(), in.From); err != nil {
		jsonError(w, "The source object does not exist", http.StatusNotFound)
		return
	}
	if _, err := c.head(r.Context(), in.To); err == nil && !in.Overwrite {
		jsonError(w, "An object with that name already exists", http.StatusConflict)
		return
	}
	if err := c.copy(r.Context(), in.From, in.To); err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := c.delete(r.Context(), in.From); err != nil {
		jsonError(w, "Copied, but the old object could not be removed: "+err.Error(), http.StatusBadGateway)
		return
	}
	s.auditFile(r, userID, "storage.file_moved", in.From, 0, map[string]interface{}{"to": in.To})
	jsonOK(w, map[string]string{"key": in.To})
}

// apiStorageText reads and writes a small text object for the editor, preserving the BOM and
// the line endings so a save does not rewrite the whole file.
func apiStorageText(w http.ResponseWriter, r *http.Request) {
	s, userID, ok := requireStorage(w, r, "read")
	if !ok {
		return
	}
	key, ok := storageQueryKey(w, r)
	if !ok {
		return
	}
	c, err := s.clientFor()
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	limit := storageEditBytes()
	switch r.Method {
	case http.MethodGet:
		resp, err := c.get(r.Context(), key, "")
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		if err != nil {
			jsonError(w, "The object could not be read", http.StatusBadGateway)
			return
		}
		if int64(len(raw)) > limit {
			jsonError(w, fmt.Sprintf("This file is larger than %d KB; open it as a download instead", limit>>10), http.StatusRequestEntityTooLarge)
			return
		}
		bom := len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF
		if bom {
			raw = raw[3:]
		}
		text, okText := utf8Text(raw)
		if !okText {
			jsonOK(w, map[string]interface{}{
				"key": key, "binary": true, "size": len(raw), "etag": strings.Trim(resp.Header.Get("ETag"), `"`),
			})
			return
		}
		crlf := strings.Contains(text, "\r\n")
		text = strings.ReplaceAll(text, "\r\n", "\n")
		jsonOK(w, map[string]interface{}{
			"key": key, "content": text, "etag": strings.Trim(resp.Header.Get("ETag"), `"`),
			"size": len(raw), "bom": bom, "crlf": crlf, "binary": false,
		})
	case http.MethodPut:
		if !s.AllowWrite {
			auditLog(r, userID, "storage.denied", s.Name, map[string]interface{}{"capability": "write", "key": key})
			jsonError(w, capabilityMessage("write"), http.StatusForbidden)
			return
		}
		var in struct {
			Content string `json:"content"`
			ETag    string `json:"etag"`
			BOM     bool   `json:"bom"`
			CRLF    bool   `json:"crlf"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit+16<<10)).Decode(&in); err != nil {
			jsonError(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if int64(len(in.Content)) > limit {
			jsonError(w, fmt.Sprintf("The text is larger than %d KB", limit>>10), http.StatusRequestEntityTooLarge)
			return
		}
		text := strings.ReplaceAll(in.Content, "\r\n", "\n")
		if in.CRLF {
			text = strings.ReplaceAll(text, "\n", "\r\n")
		}
		data := []byte(text)
		if in.BOM {
			data = append([]byte{0xEF, 0xBB, 0xBF}, data...)
		}
		etag, err := c.put(r.Context(), key, data, storageContentType("", key), in.ETag)
		if err != nil {
			jsonError(w, err.Error(), s3ErrorStatus(err))
			return
		}
		s.auditFile(r, userID, "storage.file_written", key, len(data), map[string]interface{}{"edited": true})
		jsonOK(w, map[string]interface{}{"key": key, "etag": etag, "size": len(data)})
	default:
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// utf8Text reports whether the bytes are valid UTF-8 and contain no NUL, so the editor can show
// them instead of a binary placeholder.
func utf8Text(b []byte) (string, bool) {
	if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
		return "", false
	}
	return string(b), true
}

// auditFile logs one object operation with the storage as the target and the key in the
// details, so the audit log stays readable and never holds the secret.
func (s Storage) auditFile(r *http.Request, userID int, action, key string, size int, extra map[string]interface{}) {
	d := map[string]interface{}{"key": key, "size": size}
	for k, v := range extra {
		d[k] = v
	}
	auditLog(r, userID, action, s.Name, d)
}
