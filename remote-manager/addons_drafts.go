// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ─── ADDON DRAFTS ────────────────────────────────────
//
// The AI addon loop writes into <DB>.addons/.drafts/<id>/ instead of the live
// addon folder. A draft is never served and never executed: the assistant edits
// files there, the user reviews a diff in Settings → Addons and installs it.
// Installing zips the folder and goes through addonInstallArchive, exactly like
// an uploaded ZIP, so the manifest, layout, capabilities and WebAssembly checks
// are the same. A new addon is installed disabled; nothing here enables it.

const addonDraftDiffLimit = 200 << 10

type addonDraftFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type addonDraftView struct {
	ID               string           `json:"id"`
	Name             string           `json:"name,omitempty"`
	Version          string           `json:"version,omitempty"`
	Description      string           `json:"description,omitempty"`
	Author           string           `json:"author,omitempty"`
	License          string           `json:"license,omitempty"`
	Homepage         string           `json:"homepage,omitempty"`
	Capabilities     []string         `json:"capabilities,omitempty"`
	Files            []addonDraftFile `json:"files"`
	UpdatedAt        string           `json:"updated_at"`
	Valid            bool             `json:"valid"`
	Error            string           `json:"error,omitempty"`
	Installed        bool             `json:"installed"`
	InstalledVersion string           `json:"installed_version,omitempty"`
	InstalledEnabled bool             `json:"installed_enabled"`
}

func addonsDraftsRoot() string { return filepath.Join(addonsRoot(), ".drafts") }

func addonDraftDir(id string) string { return filepath.Join(addonsDraftsRoot(), id) }

// addonDraftFiles lists the regular files of a draft, refusing anything that
// could leave its folder. Drafts are written by WRM, but a manual edit or a
// copy must not turn into a path traversal at install time.
func addonDraftFiles(dir string) ([]addonDraftFile, int64, error) {
	var out []addonDraftFile
	var total int64
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("a draft may not contain a symbolic link")
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("a draft may only contain regular files")
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !addonSafeRel(rel) {
			return fmt.Errorf("unsafe path in the draft: %q", rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > addonMaxFile {
			return fmt.Errorf("%s is larger than 8 MB", rel)
		}
		total += info.Size()
		if total > addonMaxArchive {
			return fmt.Errorf("the draft is larger than 20 MB")
		}
		out = append(out, addonDraftFile{Path: rel, Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, total, nil
}

func addonDraftManifest(dir string) (*addonManifest, error) {
	f, err := os.Open(filepath.Join(dir, "addon.json"))
	if err != nil {
		return nil, fmt.Errorf("addon.json is missing")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 256<<10+1))
	if err != nil {
		return nil, fmt.Errorf("addon.json cannot be read")
	}
	if len(raw) > 256<<10 {
		return nil, fmt.Errorf("addon.json is larger than 256 KiB")
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	m, err := addonDecodeManifest(raw)
	if err != nil {
		return nil, err
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// addonDraftZip turns a draft folder into the same archive an upload would
// carry. addonReadArchive then validates it with no second implementation.
func addonDraftZip(dir string) ([]byte, error) {
	files, total, err := addonDraftFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("the draft is empty")
	}
	if total > addonMaxArchive {
		return nil, fmt.Errorf("the draft is larger than 20 MB")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Path, Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return nil, err
		}
		src, err := os.Open(filepath.Join(dir, filepath.FromSlash(f.Path)))
		if err != nil {
			return nil, err
		}
		n, err := io.Copy(w, io.LimitReader(src, addonMaxFile+1))
		src.Close()
		if err != nil {
			return nil, err
		}
		if n > addonMaxFile {
			return nil, fmt.Errorf("%s is larger than 8 MB", f.Path)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if buf.Len() > addonMaxArchive {
		return nil, fmt.Errorf("the draft is larger than 20 MB")
	}
	return buf.Bytes(), nil
}

// addonDraftValidate checks a draft folder exactly as an install would, and
// compiles server code without running it.
func addonDraftValidate(ctx context.Context, dir string) (*addonManifest, error) {
	body, err := addonDraftZip(dir)
	if err != nil {
		return nil, err
	}
	m, _, err := addonReadArchive(body)
	if err != nil {
		return nil, err
	}
	if m.Server != nil {
		vctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := addonCheckWASM(vctx, m, dir); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func addonDraftCopyTree(src, dst string) error {
	files, _, err := addonDraftFiles(src)
	if err != nil {
		return err
	}
	for _, f := range files {
		in, err := os.Open(filepath.Join(src, filepath.FromSlash(f.Path)))
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			in.Close()
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(in, addonMaxFile+1))
		out.Close()
		in.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// addonDraftSave writes files into a draft. With merge the existing draft is
// the base; without it the draft is replaced. The new state is validated in a
// temporary folder first, so a broken write never destroys the previous draft.
func addonDraftSave(ctx context.Context, id string, files map[string]string, merge bool) (*addonManifest, error) {
	if !addonIDPattern.MatchString(id) {
		return nil, fmt.Errorf("the addon id must be 2-64 characters: lowercase letters, digits, dot, dash or underscore")
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files were given")
	}
	if err := os.MkdirAll(addonsDraftsRoot(), 0o755); err != nil {
		return nil, fmt.Errorf("the drafts folder cannot be created")
	}
	final := addonDraftDir(id)
	tmp, err := os.MkdirTemp(addonsDraftsRoot(), ".draft-")
	if err != nil {
		return nil, fmt.Errorf("the drafts folder cannot be written")
	}
	defer os.RemoveAll(tmp)
	if merge {
		if _, err := os.Stat(final); err != nil {
			return nil, fmt.Errorf("draft %s does not exist; create it first", id)
		}
		if err := addonDraftCopyTree(final, tmp); err != nil {
			return nil, err
		}
	}
	var total int64
	for rel, content := range files {
		rel = strings.TrimSpace(rel)
		if !addonSafeRel(rel) {
			return nil, fmt.Errorf("unsafe path %q: use a relative path inside the addon", rel)
		}
		if len(content) > addonMaxFile {
			return nil, fmt.Errorf("%s is larger than 8 MB", rel)
		}
		total += int64(len(content))
		if total > addonMaxArchive {
			return nil, fmt.Errorf("the draft is larger than 20 MB")
		}
		target := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return nil, err
		}
	}
	m, err := addonDraftValidate(ctx, tmp)
	if err != nil {
		return nil, err
	}
	if m.ID != id {
		return nil, fmt.Errorf("addon.json declares the id %q, not %q", m.ID, id)
	}
	old := final + ".old"
	os.RemoveAll(old)
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, old); err != nil {
			return nil, fmt.Errorf("the previous draft cannot be replaced")
		}
	}
	if err := os.Rename(tmp, final); err != nil {
		if _, statErr := os.Stat(old); statErr == nil {
			os.Rename(old, final)
		}
		return nil, fmt.Errorf("the draft folder cannot be written")
	}
	os.RemoveAll(old)
	return m, nil
}

func addonDraftRemove(id string) error {
	if !addonIDPattern.MatchString(id) {
		return fmt.Errorf("invalid addon id")
	}
	if _, err := os.Stat(addonDraftDir(id)); err != nil {
		return fmt.Errorf("draft not found")
	}
	return os.RemoveAll(addonDraftDir(id))
}

func addonDraftViewOf(id string) addonDraftView {
	dir := addonDraftDir(id)
	v := addonDraftView{ID: id, Files: []addonDraftFile{}}
	files, _, err := addonDraftFiles(dir)
	if err == nil {
		v.Files = files
	}
	if st, err := os.Stat(dir); err == nil {
		v.UpdatedAt = st.ModTime().UTC().Format(time.RFC3339)
	}
	if m, err := addonDraftManifest(dir); err == nil {
		v.Name, v.Version, v.Description, v.Author, v.License, v.Homepage = m.Name, m.Version, m.Description, m.Author, m.License, m.Homepage
		v.Capabilities = m.Capabilities
	} else {
		v.Error = err.Error()
	}
	if rec, ok := addonGet(id); ok {
		v.Installed, v.InstalledVersion, v.InstalledEnabled = true, rec.Version, rec.Enabled
	}
	// The list must show a manually broken draft instead of hiding it; validation
	// happens again at install time.
	if _, err := addonDraftValidate(context.Background(), dir); err != nil {
		v.Valid = false
		if v.Error == "" {
			v.Error = err.Error()
		}
	} else {
		v.Valid = true
		v.Error = ""
	}
	return v
}

func addonDraftList() []addonDraftView {
	out := []addonDraftView{}
	ents, err := os.ReadDir(addonsDraftsRoot())
	if err != nil {
		return out
	}
	for _, e := range ents {
		if !e.IsDir() || !addonIDPattern.MatchString(e.Name()) {
			continue
		}
		out = append(out, addonDraftViewOf(e.Name()))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func addonDraftText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	for _, r := range string(b) {
		if r == '\uFFFD' {
			return false
		}
	}
	return true
}

func addonDraftRead(dir, rel string) (string, bool) {
	f, err := os.Open(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return "", false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 2<<20+1))
	if err != nil || len(b) > 2<<20 || !addonDraftText(b) {
		return "", false
	}
	return string(b), true
}

// addonDraftDiff compares a draft with the installed addon, file by file.
func addonDraftDiff(id string) (string, error) {
	if !addonIDPattern.MatchString(id) {
		return "", fmt.Errorf("invalid addon id")
	}
	draft := addonDraftDir(id)
	if _, err := os.Stat(draft); err != nil {
		return "", fmt.Errorf("draft not found")
	}
	draftFiles, _, err := addonDraftFiles(draft)
	if err != nil {
		return "", err
	}
	installed := addonDir(id)
	installedFiles := map[string]bool{}
	if _, err := os.Stat(installed); err == nil {
		if fs, _, err := addonDraftFiles(installed); err == nil {
			for _, f := range fs {
				installedFiles[f.Path] = true
			}
		}
	}
	seen := map[string]bool{}
	names := []string{}
	for _, f := range draftFiles {
		names = append(names, f.Path)
		seen[f.Path] = true
	}
	for rel := range installedFiles {
		if !seen[rel] {
			names = append(names, rel)
		}
	}
	sort.Strings(names)
	var sb strings.Builder
	rec, hasRec := addonGet(id)
	if hasRec {
		fmt.Fprintf(&sb, "Draft %s (version %s) against installed version %s\n\n", id, addonDraftVersion(id), rec.Version)
	} else {
		fmt.Fprintf(&sb, "Draft %s (version %s); the addon is not installed\n\n", id, addonDraftVersion(id))
	}
	for _, rel := range names {
		newText, newOK := addonDraftRead(draft, rel)
		oldText, oldOK := addonDraftRead(installed, rel)
		switch {
		case newOK && oldOK:
			if newText == oldText {
				continue
			}
			sb.WriteString(aiUnifiedDiff(rel, oldText, newText, false))
		case newOK && !installedFiles[rel]:
			sb.WriteString(aiUnifiedDiff(rel, "", newText, true))
		case newOK && installedFiles[rel]:
			fmt.Fprintf(&sb, "Binary file %s changed (the installed file cannot be shown as text)\n", rel)
		case !newOK && oldOK:
			sb.WriteString(aiUnifiedDiff(rel, oldText, "", false))
		default:
			fmt.Fprintf(&sb, "Binary file %s added or changed\n", rel)
		}
	}
	out := sb.String()
	if strings.TrimSpace(out) == "" || !strings.Contains(out, "--- ") {
		return out + "(no changes)\n", nil
	}
	if len(out) > addonDraftDiffLimit {
		out = out[:addonDraftDiffLimit] + "\n… (diff truncated)\n"
	}
	return out, nil
}

func addonDraftVersion(id string) string {
	if m, err := addonDraftManifest(addonDraftDir(id)); err == nil {
		return m.Version
	}
	return "unknown"
}

// ─── API ─────────────────────────────────────────────

// GET /api/addons/drafts, GET /api/addons/drafts/{id}/diff,
// POST /api/addons/drafts/{id}/install, DELETE /api/addons/drafts/{id}.
func addonDraftsHandler(w http.ResponseWriter, r *http.Request, adminID int, parts []string) {
	if len(parts) == 0 {
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		jsonOK(w, map[string]interface{}{"drafts": addonDraftList()})
		return
	}
	id := parts[0]
	if !addonIDPattern.MatchString(id) {
		jsonError(w, "Invalid addon id", 400)
		return
	}
	if len(parts) == 2 && parts[1] == "diff" {
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		diff, err := addonDraftDiff(id)
		if err != nil {
			jsonError(w, err.Error(), 404)
			return
		}
		jsonOK(w, map[string]interface{}{"id": id, "diff": diff})
		return
	}
	if len(parts) == 2 && parts[1] == "install" {
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", 405)
			return
		}
		body, err := addonDraftZip(addonDraftDir(id))
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		out, status, err := addonInstallArchive(r.Context(), r, adminID, body, "draft:"+id)
		if err != nil {
			jsonError(w, err.Error(), status)
			return
		}
		jsonOK(w, map[string]interface{}{"id": out.ID, "name": out.Name, "version": out.Version, "enabled": out.Enabled, "checksum": out.Checksum})
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := addonDraftRemove(id); err != nil {
			jsonError(w, err.Error(), 404)
			return
		}
		auditLog(r, adminID, "addon.draft_discarded", id, nil)
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	jsonError(w, "Not found", 404)
}

// cleanDraftRel normalizes a path a caller gives for one draft file.
func cleanDraftRel(p string) (string, error) {
	p = strings.TrimSpace(p)
	if !addonSafeRel(p) {
		return "", fmt.Errorf("use a relative path inside the addon")
	}
	return path.Clean(p), nil
}
