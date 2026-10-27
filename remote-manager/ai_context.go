// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Selected terminal text and files are data. Apply the same account/share checks,
// sensitive-path denial and redaction before offering them as assistant context.
func aiContextHandler(w http.ResponseWriter, r *http.Request, uid int) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if _, code, why := aiConnAccess(r, uid, id); code != 0 {
		jsonError(w, why, code)
		return
	}
	if len(aiModesFor("panel", id)) == 0 {
		jsonError(w, "The AI policy does not allow this connection", 403)
		return
	}
	if ok, why := aiUserAllowed(uid); !ok {
		jsonError(w, why, 403)
		return
	}
	var in struct {
		Text string `json:"text"`
		Path string `json:"path"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Text) > 32<<10 {
		jsonError(w, "Context exceeds 32 KiB", 400)
		return
	}
	c, err := loadConnection(id)
	if err != nil {
		jsonError(w, "Connection not found", 404)
		return
	}
	text := in.Text
	truncated := false
	if in.Path != "" {
		p, err := aiRequirePath(in.Path)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if aiReadDenied(p) != "" {
			jsonError(w, "Sensitive files cannot be sent to the assistant", 403)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		fs, done, err := openMonitorSFTP(ctx, c)
		if err != nil {
			jsonError(w, "SFTP context read unavailable", 502)
			return
		}
		defer done()
		if strings.HasPrefix(p, "~/") {
			home, err := fs.Getwd()
			if err != nil {
				jsonError(w, "Cannot resolve home directory", 400)
				return
			}
			p = strings.TrimSuffix(home, "/") + strings.TrimPrefix(p, "~")
		}
		if err := sftpNoSymlinks(fs, p); err != nil {
			jsonError(w, "Cannot read context through a symlink", 403)
			return
		}
		real, err := fs.RealPath(p)
		if err != nil || aiReadDenied(real) != "" {
			jsonError(w, "Sensitive or unavailable context file", 403)
			return
		}
		f, err := fs.Open(real)
		if err != nil {
			jsonError(w, "Context file not readable", 403)
			return
		}
		defer f.Close()
		max := min(aiReadMax(), int64(8192))
		b, err := io.ReadAll(io.LimitReader(f, max+1))
		if err != nil {
			jsonError(w, "Cannot read context file", 502)
			return
		}
		truncated = int64(len(b)) > max
		if truncated {
			b = b[:max]
		}
		if strings.ContainsRune(string(b), '\x00') {
			jsonError(w, "Binary files cannot be sent as text context", 415)
			return
		}
		text = strings.ToValidUTF8(string(b), "�")
		in.Path = real
	}
	text = redactText(text)
	if len(text) > 8192 {
		text = truncateStr(text, 8192)
		truncated = true
	}
	auditLogRef(r, uid, usernameOf(uid), "ai.context_selected", c.Name, map[string]interface{}{"bytes": len(text), "path": in.Path, "truncated": truncated}, auditRef{ConnID: id})
	jsonOK(w, map[string]interface{}{"text": text, "path": in.Path, "truncated": truncated, "conn_id": id})
}

func aiServerPickerHandler(w http.ResponseWriter, r *http.Request, uid int) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if ok, why := aiUserAllowed(uid); !ok {
		jsonError(w, why, 403)
		return
	}
	var in struct {
		IDs []int `json:"conn_ids"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.IDs) > 2000 {
		jsonError(w, "Invalid connection selection", 400)
		return
	}
	out := []connView{}
	seen := map[int]bool{}
	for _, id := range in.IDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, code, _ := aiConnAccess(r, uid, id); code != 0 {
			continue
		}
		if len(aiModesFor("panel", id)) == 0 {
			continue
		}
		c, err := loadConnectionRaw(id)
		if err == nil && oneOf(c.Protocol, "SSH", "SFTP") {
			out = append(out, c.view())
		}
	}
	jsonOK(w, out)
}
