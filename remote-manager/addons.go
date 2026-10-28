// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─── ADDONS ──────────────────────────────────────────
//
// An addon is a folder that is dropped in and discovered at runtime, in the
// spirit of Omarchy: <DB_PATH>.addons/<id>/addon.json plus the files it
// declares. Installing a ZIP is the only way in, so nothing executes at
// install time. The manifest declares capabilities; this build accepts
// interface hooks (a sandboxed panel and a theme) and WebAssembly server code
// (server.wasm, run through wazero in addons_wasm.go).

const (
	addonMaxArchive = 20 << 20
	addonMaxFiles   = 500
	addonMaxFile    = 8 << 20
)

// addonMaxTotal caps the sum of all uncompressed entries. A ZIP with 500 files of
// 8 MB each would otherwise write about 4 GB to <DB>.addons; the per-entry limit
// alone is not a disk bound. It is a variable so tests can lower it.
var addonMaxTotal uint64 = 64 << 20

var (
	addonIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)
	// addonCapabilities is the whole trusted surface of an addon. A capability
	// that is not listed here refuses the install instead of being ignored.
	addonCapabilities = map[string]string{
		"ui.panel":    "show a panel in the WRM interface",
		"ui.theme":    "offer colour themes users can pick in Settings → Appearance",
		"server.wasm": "run a WebAssembly hook in the sandbox",
		"store":       "keep its settings on the WRM server (apiVersion 2)",
		"notify":      "show short notifications from its panel (apiVersion 2)",
		"http":        "reach the web sites its addon.json lists, through WRM and after an administrator approved them (apiVersion 2)",
	}
	addonExportPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	// addonReservedIDs are path words of /api/addons/ that an addon id would
	// shadow.
	addonReservedIDs = map[string]bool{"dev": true, "install": true, "drafts": true, "community": true, "users": true}
)

type addonUI struct {
	Panel       string       `json:"panel,omitempty"` // panel.html
	Theme       string       `json:"theme,omitempty"` // theme.css
	Title       string       `json:"title,omitempty"`
	Icon        string       `json:"icon,omitempty"`
	Themes      []addonTheme `json:"themes,omitempty"`
	DefaultSize *addonSize   `json:"defaultSize,omitempty"`
	MinSize     *addonSize   `json:"minSize,omitempty"`
}

type addonServer struct {
	Module  string   `json:"module"`  // plugin.wasm
	Exports []string `json:"exports"` // functions the host may call
}

type addonManifest struct {
	APIVersion   int          `json:"apiVersion,omitempty"`
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description,omitempty"`
	Author       string       `json:"author,omitempty"`
	License      string       `json:"license,omitempty"`
	Homepage     string       `json:"homepage,omitempty"`
	Capabilities []string     `json:"capabilities"`
	UI           *addonUI     `json:"ui,omitempty"`
	Server       *addonServer `json:"server,omitempty"`
	Store        *addonStore  `json:"store,omitempty"`
	HTTP         *addonHTTP   `json:"http,omitempty"`
}

type addonRecord struct {
	APIVersion   int          `json:"apiVersion,omitempty"`
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description,omitempty"`
	Author       string       `json:"author,omitempty"`
	License      string       `json:"license,omitempty"`
	Homepage     string       `json:"homepage,omitempty"`
	Capabilities []string     `json:"capabilities"`
	UI           *addonUI     `json:"ui,omitempty"`
	Enabled      bool         `json:"enabled"`
	InstalledAt  string       `json:"installed_at"`
	InstalledBy  string       `json:"installed_by,omitempty"`
	Source       string       `json:"source,omitempty"`
	Checksum     string       `json:"checksum"`
	Server       *addonServer `json:"server,omitempty"`
	Store        *addonStore  `json:"store,omitempty"`
	HTTP         *addonHTTP   `json:"http,omitempty"`
	// Key names the addon in URLs: the id of an organisation addon, "~id" for
	// the signed-in user's personal addon. AssetBase is where its files are.
	Key       string `json:"key"`
	AssetBase string `json:"asset_base"`
	Personal  bool   `json:"personal,omitempty"`
	OwnerID   int    `json:"owner_id,omitempty"`
	Owner     string `json:"owner,omitempty"`
	// NeedsApproval: an update asked for more than was approved; the addon
	// stays disabled until someone allowed to enable it approves the sheet.
	NeedsApproval bool     `json:"needs_approval,omitempty"`
	Permissions   []string `json:"permissions"`
	Added         []string `json:"permissions_added,omitempty"`
	// Network access (http capability): the administrator's approvals per
	// origin, whether an origin still waits for one, and the secrets.
	HTTPGrants  []addonHTTPGrant  `json:"http_grants,omitempty"`
	HTTPWaiting bool              `json:"http_waiting,omitempty"`
	Secrets     []addonSecretSpec `json:"secrets,omitempty"`
	files       map[string]string // rel path -> sha256 hex, pinned at install
	approved    []string          // permission atoms approved when it was last enabled
}

func initAddonSchema() {
	ensureTable("addons", `CREATE TABLE IF NOT EXISTS addons (
		id TEXT PRIMARY KEY,
		version TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 0,
		manifest TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT '',
		checksum TEXT NOT NULL DEFAULT '',
		installed_at TEXT NOT NULL DEFAULT '',
		installed_by TEXT NOT NULL DEFAULT ''
	)`)
	// v13.9: file hashes pinned at install, the permission sheet that was
	// approved, and what an update added on top of it.
	db.Exec(`ALTER TABLE addons ADD COLUMN files TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE addons ADD COLUMN approved TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE addons ADD COLUMN pending TEXT NOT NULL DEFAULT ''`)
	initAddonV2Schema()
}

func addonsRoot() string { return resolveDBPath() + ".addons" }

// addonDir is the folder of one addon. Ids are checked when a manifest is
// read; the check is repeated here so no caller can reach outside the root.
func addonDir(id string) string {
	if !addonIDPattern.MatchString(id) || strings.Contains(id, "..") {
		return filepath.Join(addonsRoot(), ".invalid")
	}
	return filepath.Join(addonsRoot(), id)
}

// addonRef names one installed addon: an organisation addon (Owner 0) or the
// personal addon of one user. Personal addons live in their own table and
// folder (<addons>/.users/<user id>/<id>), so an organisation addon and a
// personal one may share an id and an older binary never sees personal ones.
type addonRef struct {
	Owner int
	ID    string
}

func (r addonRef) Key() string {
	if r.Owner > 0 {
		return "~" + r.ID
	}
	return r.ID
}

func addonRefDir(r addonRef) string {
	if r.Owner <= 0 {
		return addonDir(r.ID)
	}
	if !addonIDPattern.MatchString(r.ID) || strings.Contains(r.ID, "..") {
		return filepath.Join(addonsRoot(), ".invalid")
	}
	return filepath.Join(addonsRoot(), ".users", strconv.Itoa(r.Owner), r.ID)
}

// addonParseKey turns a URL segment into a reference for the signed-in user.
func addonParseKey(key string, userID int) (addonRef, bool) {
	if strings.HasPrefix(key, "~") {
		id := key[1:]
		return addonRef{Owner: userID, ID: id}, addonIDPattern.MatchString(id)
	}
	return addonRef{ID: key}, addonIDPattern.MatchString(key)
}

func addonRecordFrom(owner int, id string, enabled int, manifest, source, checksum, installedAt, installedBy, files, approved, pending string) (addonRecord, bool) {
	var m addonManifest
	if json.Unmarshal([]byte(manifest), &m) != nil {
		return addonRecord{}, false
	}
	ref := addonRef{Owner: owner, ID: id}
	rec := addonRecord{
		APIVersion: m.APIVersion, ID: id, Name: m.Name, Version: m.Version, Description: m.Description, Author: m.Author,
		License: m.License, Homepage: m.Homepage, Capabilities: m.Capabilities, UI: m.UI, Server: m.Server, Store: m.Store, HTTP: m.HTTP,
		Enabled: enabled == 1, InstalledAt: installedAt, InstalledBy: installedBy, Source: source, Checksum: checksum,
		Key: ref.Key(), AssetBase: "/api/addons/" + ref.Key() + "/asset/", Personal: owner > 0, OwnerID: owner,
		Permissions: addonPermissions(&m),
	}
	if owner > 0 {
		rec.Owner = usernameOf(owner)
	}
	if files != "" {
		json.Unmarshal([]byte(files), &rec.files)
	}
	if pending != "" {
		var added []string
		if json.Unmarshal([]byte(pending), &added) == nil && len(added) > 0 {
			rec.NeedsApproval, rec.Added = true, added
		}
	}
	if approved != "" {
		json.Unmarshal([]byte(approved), &rec.approved)
	}
	addonHTTPStatus(&rec)
	return rec, true
}

func addonRecords(enabledOnly bool) []addonRecord {
	q := `SELECT id, enabled, manifest, source, checksum, installed_at, installed_by, files, approved, pending FROM addons`
	if enabledOnly {
		q += ` WHERE enabled=1`
	}
	rows, err := db.Query(q + ` ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []addonRecord{}
	for rows.Next() {
		var id, manifest, source, checksum, installedAt, installedBy, files, approved, pending string
		var enabled int
		if rows.Scan(&id, &enabled, &manifest, &source, &checksum, &installedAt, &installedBy, &files, &approved, &pending) != nil {
			continue
		}
		if rec, ok := addonRecordFrom(0, id, enabled, manifest, source, checksum, installedAt, installedBy, files, approved, pending); ok {
			out = append(out, rec)
		}
	}
	return out
}

// addonPersonalRecords lists the personal addons of one user, or of every
// user when owner is 0 (the administrator's overview).
func addonPersonalRecords(owner int, enabledOnly bool) []addonRecord {
	q := `SELECT owner_id, id, enabled, manifest, source, checksum, installed_at, installed_by, files, approved, pending FROM addons_personal WHERE (owner_id=? OR ?=0)`
	if enabledOnly {
		q += ` AND enabled=1`
	}
	rows, err := db.Query(q+` ORDER BY owner_id, id`, owner, owner)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []addonRecord{}
	for rows.Next() {
		var id, manifest, source, checksum, installedAt, installedBy, files, approved, pending string
		var uid, enabled int
		if rows.Scan(&uid, &id, &enabled, &manifest, &source, &checksum, &installedAt, &installedBy, &files, &approved, &pending) != nil {
			continue
		}
		if rec, ok := addonRecordFrom(uid, id, enabled, manifest, source, checksum, installedAt, installedBy, files, approved, pending); ok {
			out = append(out, rec)
		}
	}
	return out
}

func addonGet(id string) (addonRecord, bool) {
	return addonGetRef(addonRef{ID: id})
}

func addonGetRef(ref addonRef) (addonRecord, bool) {
	list := addonRecords(false)
	if ref.Owner > 0 {
		list = addonPersonalRecords(ref.Owner, false)
	}
	for _, a := range list {
		if a.ID == ref.ID && a.OwnerID == ref.Owner {
			return a, true
		}
	}
	return addonRecord{}, false
}

// validate checks a manifest against the capabilities this build implements.
func (m *addonManifest) validate() error {
	if !addonIDPattern.MatchString(m.ID) {
		return fmt.Errorf("the addon id must be 2-64 characters: lowercase letters, digits, dot, dash or underscore")
	}
	if addonReservedIDs[m.ID] {
		return fmt.Errorf("the addon id %q is reserved by WRM; choose another one", m.ID)
	}
	m.Name = strings.TrimSpace(m.Name)
	m.Version = strings.TrimSpace(m.Version)
	if m.Name == "" || len(m.Name) > 80 {
		return fmt.Errorf("a name of 1-80 characters is required")
	}
	if m.Version == "" || len(m.Version) > 32 {
		return fmt.Errorf("a version of 1-32 characters is required")
	}
	if len(m.Capabilities) == 0 {
		return fmt.Errorf("the addon declares no capabilities")
	}
	if len(m.Capabilities) > 8 {
		return fmt.Errorf("too many capabilities")
	}
	seen := map[string]bool{}
	for _, c := range m.Capabilities {
		if _, ok := addonCapabilities[c]; !ok {
			return fmt.Errorf("capability %q is not supported by this version", c)
		}
		if seen[c] {
			return fmt.Errorf("capability %q is listed twice", c)
		}
		seen[c] = true
	}
	if m.Server != nil {
		if !seen["server.wasm"] {
			return fmt.Errorf("a server block requires the server.wasm capability")
		}
		if !addonSafeRel(m.Server.Module) || !strings.HasSuffix(strings.ToLower(m.Server.Module), ".wasm") {
			return fmt.Errorf("server.module must be a relative .wasm file")
		}
		if len(m.Server.Exports) == 0 {
			return fmt.Errorf("server.exports must name at least one exported function")
		}
		if len(m.Server.Exports) > 8 {
			return fmt.Errorf("at most 8 exported functions are allowed")
		}
		xseen := map[string]bool{}
		for _, e := range m.Server.Exports {
			if !addonExportPattern.MatchString(e) {
				return fmt.Errorf("export %q must be letters, digits and underscore, starting with a letter or underscore", e)
			}
			if xseen[e] {
				return fmt.Errorf("export %q is listed twice", e)
			}
			xseen[e] = true
		}
	} else if seen["server.wasm"] {
		return fmt.Errorf("the server.wasm capability needs a server block")
	}
	if m.UI != nil {
		if m.UI.Panel != "" {
			if !seen["ui.panel"] {
				return fmt.Errorf("ui.panel requires the ui.panel capability")
			}
			if !addonSafeRel(m.UI.Panel) || !strings.HasSuffix(strings.ToLower(m.UI.Panel), ".html") {
				return fmt.Errorf("ui.panel must be a relative .html file")
			}
		}
		if m.UI.Theme != "" {
			if !seen["ui.theme"] {
				return fmt.Errorf("ui.theme requires the ui.theme capability")
			}
			if !addonSafeRel(m.UI.Theme) || !strings.HasSuffix(strings.ToLower(m.UI.Theme), ".css") {
				return fmt.Errorf("ui.theme must be a relative .css file")
			}
		}
		if len(m.UI.Themes) > 0 && !seen["ui.theme"] {
			return fmt.Errorf("ui.themes requires the ui.theme capability")
		}
		if m.UI.Panel == "" && m.UI.Theme == "" && len(m.UI.Themes) == 0 {
			return fmt.Errorf("the ui block declares neither a panel nor a theme")
		}
		if err := addonValidateUIv2(m.UI); err != nil {
			return err
		}
	}
	if m.UI == nil && m.Server == nil {
		return fmt.Errorf("the addon declares neither a ui panel/theme nor server code")
	}
	if err := addonValidateHTTP(m, seen); err != nil {
		return err
	}
	return addonValidateV2(m, seen)
}

// addonSafeRel accepts a relative path inside an addon folder and nothing else.
func addonSafeRel(p string) bool {
	if p == "" || len(p) > 300 || strings.ContainsAny(p, "\\\x00\r\n") || strings.HasPrefix(p, "/") {
		return false
	}
	clean := path.Clean(p)
	return clean == p && clean != "." && !strings.HasPrefix(clean, "../")
}

// servedFiles are the files the asset endpoint may return. Nothing else leaves
// an addon folder, so a stray note or key file stays private.
func addonAssetType(name string) (string, bool) {
	switch strings.ToLower(path.Ext(name)) {
	case ".html", ".htm":
		return "text/html; charset=utf-8", true
	case ".css":
		return "text/css; charset=utf-8", true
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8", true
	case ".json", ".map":
		return "application/json; charset=utf-8", true
	case ".svg":
		return "image/svg+xml", true
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	case ".ico":
		return "image/x-icon", true
	case ".woff2":
		return "font/woff2", true
	case ".txt", ".md":
		return "text/plain; charset=utf-8", true
	}
	return "", false
}

// ─── install ─────────────────────────────────────────

// addonReadArchive extracts an uploaded ZIP into a temporary folder and returns
// its manifest. Entry names are checked before anything is written: absolute
// paths, "..", backslashes, duplicates and oversized entries refuse the whole
// archive.
func addonReadArchive(body []byte) (*addonManifest, string, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, "", fmt.Errorf("not a ZIP archive")
	}
	if len(zr.File) == 0 || len(zr.File) > addonMaxFiles {
		return nil, "", fmt.Errorf("an addon must contain 1-%d files", addonMaxFiles)
	}
	names := map[string]bool{}
	var total uint64
	for _, f := range zr.File {
		n := strings.TrimSuffix(f.Name, "/")
		if !addonSafeRel(n) {
			return nil, "", fmt.Errorf("unsafe path in the archive: %q", f.Name)
		}
		key := strings.ToLower(n)
		if names[key] {
			return nil, "", fmt.Errorf("duplicate file in the archive: %q", f.Name)
		}
		names[key] = true
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > addonMaxFile {
			return nil, "", fmt.Errorf("%s is larger than 8 MB", n)
		}
		total += f.UncompressedSize64
		if total > addonMaxTotal {
			return nil, "", fmt.Errorf("the addon is larger than %d MB uncompressed", addonMaxTotal>>20)
		}
	}
	// The manifest sits at the root, or one folder down (a ZIP made from a
	// folder). No other layout is guessed.
	prefix := ""
	if !names["addon.json"] {
		top := ""
		for n := range names {
			i := strings.Index(n, "/")
			if i < 0 {
				return nil, "", fmt.Errorf("addon.json is missing")
			}
			if top == "" {
				top = n[:i]
			} else if top != n[:i] {
				return nil, "", fmt.Errorf("addon.json is missing")
			}
		}
		if top == "" || !names[strings.ToLower(top+"/addon.json")] {
			return nil, "", fmt.Errorf("addon.json is missing")
		}
		prefix = top + "/"
	}
	var raw []byte
	for _, f := range zr.File {
		n := strings.TrimSuffix(f.Name, "/")
		if n != prefix+"addon.json" {
			continue
		}
		if f.UncompressedSize64 > 256<<10 {
			return nil, "", fmt.Errorf("addon.json is larger than 256 KiB")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, "", fmt.Errorf("addon.json cannot be read")
		}
		raw, err = io.ReadAll(io.LimitReader(rc, 256<<10))
		rc.Close()
		if err != nil {
			return nil, "", fmt.Errorf("addon.json cannot be read")
		}
	}
	if raw == nil {
		return nil, "", fmt.Errorf("addon.json is missing")
	}
	// Editors on Windows like to add a byte order mark; JSON does not allow it.
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	m, err := addonDecodeManifest(raw)
	if err != nil {
		return nil, "", err
	}
	if err := m.validate(); err != nil {
		return nil, "", addonErrf("E_MANIFEST", "%v", err)
	}
	// Declared entries must be in the archive, or the addon would install and
	// then show nothing.
	declared := []string{}
	if m.UI != nil {
		declared = append(declared, m.UI.Panel, m.UI.Theme)
		for _, th := range m.UI.Themes {
			declared = append(declared, th.CSS)
		}
	}
	if m.Server != nil {
		declared = append(declared, m.Server.Module)
	}
	for _, want := range declared {
		if want != "" && !names[strings.ToLower(prefix+want)] {
			return nil, "", addonErrf("E_MISSING_FILE", "%s is declared in addon.json but missing from the archive", want)
		}
	}
	return m, prefix, nil
}

func addonWriteFiles(body []byte, prefix, dst string) error {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return err
	}
	// os.Root keeps every write inside dst, whatever the archive names.
	root, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, f := range zr.File {
		n := strings.TrimSuffix(f.Name, "/")
		if !strings.HasPrefix(n, prefix) {
			continue
		}
		rel := strings.TrimPrefix(n, prefix)
		if rel == "" || f.FileInfo().IsDir() {
			continue
		}
		target := filepath.FromSlash(rel)
		if f.UncompressedSize64 > addonMaxFile {
			return fmt.Errorf("%s is larger than 8 MB", rel)
		}
		if dir := filepath.Dir(target); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := root.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		written, err := io.Copy(out, io.LimitReader(rc, addonMaxFile+1))
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
		if written > addonMaxFile {
			return fmt.Errorf("%s is larger than 8 MB", rel)
		}
	}
	return nil
}

func addonInstall(w http.ResponseWriter, r *http.Request, userID int, owner int) {
	body, err := io.ReadAll(io.LimitReader(r.Body, addonMaxArchive+1))
	if err != nil || len(body) == 0 {
		jsonError(w, "Send the addon as a ZIP file", 400)
		return
	}
	if len(body) > addonMaxArchive {
		addonFail(w, addonErrf("E_LIMIT_SIZE", "The archive is larger than 20 MB"), 413)
		return
	}
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	out, status, err := addonInstallArchiveAs(r.Context(), r, userID, owner, body, source)
	if err != nil {
		addonFail(w, err, status)
		return
	}
	jsonOK(w, map[string]interface{}{"id": out.ID, "key": out.Key, "name": out.Name, "version": out.Version, "enabled": out.Enabled,
		"checksum": out.Checksum, "needs_approval": len(out.Added) > 0, "permissions_added": out.Added})
}

// addonInstallOutcome is what an install reports, whether it came from an
// upload, the community library or a draft.
type addonInstallOutcome struct {
	ID       string
	Key      string
	Name     string
	Version  string
	Checksum string
	Enabled  bool
	Added    []string // permissions an update asked for beyond the approved ones
}

// addonInstallArchive installs an organisation addon. The AI tools install a
// draft through the same function the upload endpoint uses.
func addonInstallArchive(ctx context.Context, r *http.Request, adminID int, body []byte, source string) (addonInstallOutcome, int, error) {
	return addonInstallArchiveAs(ctx, r, adminID, 0, body, source)
}

// addonInstallArchiveAs installs an archive as an organisation addon (owner 0)
// or as the personal addon of owner. A new addon is installed disabled. An
// update keeps the enabled state only while it asks for nothing beyond what
// was approved; otherwise it waits, disabled, for the sheet to be approved
// again. A lower version than the installed one needs allow_downgrade=1.
func addonInstallArchiveAs(ctx context.Context, r *http.Request, userID, owner int, body []byte, source string) (addonInstallOutcome, int, error) {
	var out addonInstallOutcome
	m, prefix, err := addonReadArchive(body)
	if err != nil {
		return out, 400, err
	}
	if owner > 0 && m.Server != nil {
		return out, 400, addonErrf("E_PERSONAL_SERVER", "Server code (server.wasm) is only allowed in an addon an administrator installs for the organisation")
	}
	ref := addonRef{Owner: owner, ID: m.ID}
	previous, hadPrevious := addonGetRef(ref)
	if hadPrevious && addonVersionLess(m.Version, previous.Version) && (r == nil || r.URL.Query().Get("allow_downgrade") != "1") {
		return out, 409, addonErrf("E_DOWNGRADE", "Version %s is older than the installed %s; confirm the downgrade to install it", m.Version, previous.Version)
	}
	sum := sha256.Sum256(body)
	checksum := "sha256:" + hex.EncodeToString(sum[:])
	final := addonRefDir(ref)
	parent := filepath.Dir(final)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return out, 500, fmt.Errorf("the addon folder cannot be created")
	}
	tmp, err := os.MkdirTemp(parent, ".install-")
	if err != nil {
		return out, 500, fmt.Errorf("the addon folder cannot be written")
	}
	defer os.RemoveAll(tmp)
	if err := addonWriteFiles(body, prefix, tmp); err != nil {
		return out, 500, fmt.Errorf("the archive cannot be written: %v", err)
	}
	// Compiling is not executing, and it turns a broken module into an install
	// error instead of a surprise on the first call.
	if m.Server != nil {
		vctx, vcancel := context.WithTimeout(ctx, 10*time.Second)
		err := addonCheckWASM(vctx, m, tmp)
		vcancel()
		if err != nil {
			return out, 400, err
		}
	}
	hashes, err := addonHashTree(tmp)
	if err != nil {
		return out, 500, fmt.Errorf("the addon files cannot be read back")
	}
	old := final + ".old"
	os.RemoveAll(old)
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, old); err != nil {
			return out, 500, fmt.Errorf("the previous version cannot be replaced")
		}
	}
	if err := os.Rename(tmp, final); err != nil {
		if _, statErr := os.Stat(old); statErr == nil {
			os.Rename(old, final)
		}
		return out, 500, fmt.Errorf("the addon folder cannot be written")
	}
	os.RemoveAll(old)
	manifest, _ := json.Marshal(m)
	filesJSON, _ := json.Marshal(hashes)
	if len(source) > 300 {
		source = source[:300]
	}
	enabled := 0
	var added []string
	approved := ""
	if hadPrevious {
		base := previous.approved
		if len(base) == 0 {
			base = previous.Permissions // installed before sheets were recorded
		}
		added = addonPermsAdded(addonPermissions(m), base)
		if previous.Enabled && len(added) == 0 {
			enabled = 1
		}
		if b, err := json.Marshal(base); err == nil {
			approved = string(b)
		}
	}
	pending := ""
	if len(added) > 0 {
		b, _ := json.Marshal(added)
		pending = string(b)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if owner > 0 {
		_, err = db.Exec(`INSERT INTO addons_personal (owner_id, id, version, enabled, manifest, source, checksum, installed_at, installed_by, files, approved, pending)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(owner_id, id) DO UPDATE SET version=excluded.version, enabled=excluded.enabled, manifest=excluded.manifest,
				source=excluded.source, checksum=excluded.checksum, installed_at=excluded.installed_at, installed_by=excluded.installed_by,
				files=excluded.files, approved=excluded.approved, pending=excluded.pending`,
			owner, m.ID, m.Version, enabled, string(manifest), source, checksum, now, usernameOf(userID), string(filesJSON), approved, pending)
	} else {
		_, err = db.Exec(`INSERT INTO addons (id, version, enabled, manifest, source, checksum, installed_at, installed_by, files, approved, pending)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET version=excluded.version, enabled=excluded.enabled, manifest=excluded.manifest,
				source=excluded.source, checksum=excluded.checksum, installed_at=excluded.installed_at, installed_by=excluded.installed_by,
				files=excluded.files, approved=excluded.approved, pending=excluded.pending`,
			m.ID, m.Version, enabled, string(manifest), source, checksum, now, usernameOf(userID), string(filesJSON), approved, pending)
	}
	if err != nil {
		return out, 500, fmt.Errorf("the addon cannot be recorded")
	}
	details := map[string]interface{}{
		"version": m.Version, "capabilities": m.Capabilities, "checksum": checksum, "enabled": enabled == 1, "api_version": m.APIVersion,
	}
	if owner > 0 {
		details["personal_owner"] = usernameOf(owner)
	}
	if len(added) > 0 {
		details["permissions_added"] = added
	}
	if hadPrevious {
		details["previous_version"] = previous.Version
	}
	auditLog(r, userID, "addon.installed", ref.Key(), details)
	out = addonInstallOutcome{ID: m.ID, Key: ref.Key(), Name: m.Name, Version: m.Version, Checksum: checksum, Enabled: enabled == 1, Added: added}
	return out, 200, nil
}

func addonSetEnabledCore(adminID int, id string, on bool) (addonRecord, int, error) {
	return addonSetEnabledRef(adminID, addonRef{ID: id}, on)
}

// addonSetEnabledRef enables or disables an addon. Enabling is the approval of
// its permission sheet: the sheet is stored, and a later update that asks for
// more disables the addon until it is approved again.
func addonSetEnabledRef(userID int, ref addonRef, on bool) (addonRecord, int, error) {
	rec, ok := addonGetRef(ref)
	if !ok {
		return rec, 404, fmt.Errorf("addon not found")
	}
	if on {
		// The folder comes from the stored record, not from the request.
		if _, err := os.Stat(filepath.Join(addonRefDir(addonRef{Owner: rec.OwnerID, ID: rec.ID}), "addon.json")); err != nil {
			return rec, 409, fmt.Errorf("the addon files are missing; install the archive again")
		}
	}
	v := 0
	if on {
		v = 1
	}
	var err error
	if on {
		approved, _ := json.Marshal(rec.Permissions)
		if ref.Owner > 0 {
			_, err = db.Exec(`UPDATE addons_personal SET enabled=1, approved=?, pending='' WHERE owner_id=? AND id=?`, string(approved), ref.Owner, ref.ID)
		} else {
			_, err = db.Exec(`UPDATE addons SET enabled=1, approved=?, pending='' WHERE id=?`, string(approved), ref.ID)
		}
	} else if ref.Owner > 0 {
		_, err = db.Exec(`UPDATE addons_personal SET enabled=? WHERE owner_id=? AND id=?`, v, ref.Owner, ref.ID)
	} else {
		_, err = db.Exec(`UPDATE addons SET enabled=? WHERE id=?`, v, ref.ID)
	}
	if err != nil {
		return rec, 500, fmt.Errorf("the addon cannot be updated")
	}
	action := "addon.disabled"
	details := map[string]interface{}{"version": rec.Version}
	if on {
		action = "addon.enabled"
		details["permissions"] = rec.Permissions
	}
	if ref.Owner > 0 {
		details["personal_owner"] = usernameOf(ref.Owner)
	}
	auditLog(nil, userID, action, ref.Key(), details)
	return rec, 200, nil
}

func addonSetEnabled(w http.ResponseWriter, r *http.Request, userID int, ref addonRef, on bool) {
	// An administrator who enables an addon with the http capability approves
	// its origins in the same step: {"private": [origins]} names those that
	// may reach the private network.
	var req struct {
		Private []string `json:"private"`
	}
	if on && r.ContentLength > 0 {
		if json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req) != nil {
			jsonError(w, "Invalid request", 400)
			return
		}
	}
	if cur, ok := addonGetRef(ref); ok && on && len(req.Private) > 0 {
		if _, err := addonHTTPPrivateSet(cur, req.Private); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
	}
	rec, status, err := addonSetEnabledRef(userID, ref, on)
	if err != nil {
		jsonError(w, err.Error(), status)
		return
	}
	if on && rec.HTTP != nil && isAdminUser(userID) {
		if err := addonHTTPApprove(r, userID, rec, req.Private); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
	}
	jsonOK(w, map[string]interface{}{"ok": true, "enabled": on, "id": rec.ID, "key": rec.Key})
}

func addonRemoveCore(adminID int, id string) (addonRecord, int, error) {
	return addonRemoveRef(adminID, addonRef{ID: id})
}

// addonRemoveRef deletes the addon with its files, instances, stored values
// and remembered windows.
func addonRemoveRef(userID int, ref addonRef) (addonRecord, int, error) {
	rec, ok := addonGetRef(ref)
	if !ok {
		return rec, 404, fmt.Errorf("addon not found")
	}
	var err error
	if ref.Owner > 0 {
		_, err = db.Exec(`DELETE FROM addons_personal WHERE owner_id=? AND id=?`, ref.Owner, ref.ID)
	} else {
		_, err = db.Exec(`DELETE FROM addons WHERE id=?`, ref.ID)
	}
	if err != nil {
		return rec, 500, fmt.Errorf("the addon cannot be removed")
	}
	os.RemoveAll(addonRefDir(addonRef{Owner: rec.OwnerID, ID: rec.ID}))
	addonForgetInstances(ref)
	db.Exec(`DELETE FROM addon_http_grants WHERE addon_owner=? AND addon_id=?`, rec.OwnerID, rec.ID)
	details := map[string]interface{}{"version": rec.Version}
	if ref.Owner > 0 {
		details["personal_owner"] = usernameOf(ref.Owner)
	}
	auditLog(nil, userID, "addon.removed", ref.Key(), details)
	return rec, 200, nil
}

func addonRemove(w http.ResponseWriter, r *http.Request, userID int, ref addonRef) {
	if _, status, err := addonRemoveRef(userID, ref); err != nil {
		jsonError(w, err.Error(), status)
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

// addonAssetHandler serves a file from an addon folder. Only an enabled addon
// is served, only known asset types, only from inside its own folder, and only
// a file whose SHA-256 still matches the one pinned at install.
func addonAssetHandler(w http.ResponseWriter, r *http.Request, ref addonRef, rel string) {
	rec, ok := addonGetRef(ref)
	if !ok || !rec.Enabled {
		jsonError(w, "Addon not found", 404)
		return
	}
	rel = path.Clean("/" + rel)[1:]
	if !addonSafeRel(rel) {
		jsonError(w, "Invalid file", 400)
		return
	}
	typ, ok := addonAssetType(rel)
	if !ok {
		jsonError(w, "Invalid file", 400)
		return
	}
	// os.Root keeps every lookup inside the addon folder, symlinks included.
	root, err := os.OpenRoot(addonRefDir(ref))
	if err != nil {
		jsonError(w, "Addon not found", 404)
		return
	}
	defer root.Close()
	f, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		jsonError(w, "Addon not found", 404)
		return
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > addonMaxFile {
		f.Close()
		jsonError(w, "Addon not found", 404)
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, addonMaxFile+1))
	f.Close()
	if err != nil {
		jsonError(w, "Addon not found", 404)
		return
	}
	if rec.files != nil {
		want, known := rec.files[rel]
		sum := sha256.Sum256(data)
		if !known || want != hex.EncodeToString(sum[:]) {
			addonFail(w, addonErrf("E_INTEGRITY", "%s changed after the addon was installed; install the addon again", rel), 409)
			return
		}
	}
	h := w.Header()
	h.Set("Content-Type", typ)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-cache")
	// The global middleware sends X-Frame-Options: DENY. A panel is the one
	// document WRM frames itself, and only from the same origin.
	h.Set("X-Frame-Options", "SAMEORIGIN")
	if strings.HasPrefix(typ, "text/html") {
		if rec.APIVersion >= 2 {
			// apiVersion 2: no network of its own (everything goes through the
			// host), inline scripts only by hash, nothing that leaves the frame.
			h.Set("Content-Security-Policy", addonPanelCSP(data))
		} else {
			// Belt and braces: the panel already runs in a sandboxed iframe.
			h.Set("Content-Security-Policy", "sandbox allow-scripts")
		}
	}
	http.ServeContent(w, r, rel, st.ModTime(), bytes.NewReader(data))
}

func apiAddonsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/addons"), "/")
	parts := strings.Split(rest, "/")
	admin := isAdminUser(userID)
	if rest == "" {
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		list := addonRecords(!admin)
		list = append(list, addonPersonalRecords(userID, false)...)
		sort.SliceStable(list, func(i, j int) bool { return list[i].Name < list[j].Name })
		out := map[string]interface{}{"addons": list, "capabilities": addonCapabilities,
			"personal_allowed": addonPersonalAllowed(userID), "api_versions": []int{1, 2}}
		if admin && r.URL.Query().Get("personal") == "all" {
			others := []addonRecord{}
			for _, a := range addonPersonalRecords(0, false) {
				if a.OwnerID != userID {
					others = append(others, a)
				}
			}
			out["personal_others"] = others
		}
		jsonOK(w, out)
		return
	}
	if parts[0] == "dev" {
		addonDevAPIHandler(w, r, userID, admin, parts[1:])
		return
	}
	if parts[0] == "drafts" {
		if !admin {
			jsonError(w, "Admin access required", 403)
			return
		}
		addonDraftsHandler(w, r, userID, parts[1:])
		return
	}
	if parts[0] == "community" {
		if len(parts) == 1 && r.Method == http.MethodGet {
			addonCommunityListHandler(w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "install" {
			if r.Method != http.MethodPost {
				jsonError(w, "Method not allowed", 405)
				return
			}
			if !admin {
				jsonError(w, "Admin access required", 403)
				return
			}
			addonCommunityInstallHandler(w, r, userID)
			return
		}
		jsonError(w, "Not found", 404)
		return
	}
	if parts[0] == "install" {
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", 405)
			return
		}
		// An administrator installs for the organisation unless asking for a
		// personal addon; everybody else installs a personal one.
		if admin && r.URL.Query().Get("scope") != "personal" {
			addonInstall(w, r, userID, 0)
			return
		}
		if !addonPersonalAllowed(userID) {
			addonFail(w, addonErrf("E_PERSONAL_OFF", "Personal addons are turned off by the administrator (addon_personal)"), 403)
			return
		}
		addonInstall(w, r, userID, userID)
		return
	}
	if parts[0] == "users" {
		// The administrator's kill switch for other users' personal addons:
		// /api/addons/users/{user id}/{addon id}[/disable].
		if !admin {
			jsonError(w, "Admin access required", 403)
			return
		}
		if len(parts) < 3 {
			jsonError(w, "Not found", 404)
			return
		}
		uid, err := strconv.Atoi(parts[1])
		if err != nil || uid <= 0 || !addonIDPattern.MatchString(parts[2]) {
			jsonError(w, "Not found", 404)
			return
		}
		ref := addonRef{Owner: uid, ID: parts[2]}
		switch {
		case len(parts) == 3 && r.Method == http.MethodDelete:
			addonRemove(w, r, userID, ref)
		case len(parts) == 4 && parts[3] == "disable" && r.Method == http.MethodPost:
			addonSetEnabled(w, r, userID, ref, false)
		case len(parts) == 4 && parts[3] == "http":
			addonHTTPGrantHandler(w, r, userID, ref)
		default:
			jsonError(w, "Not found", 404)
		}
		return
	}
	ref, valid := addonParseKey(parts[0], userID)
	if !valid {
		jsonError(w, "Addon not found", 404)
		return
	}
	if len(parts) >= 3 && parts[1] == "asset" {
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		addonAssetHandler(w, r, ref, strings.Join(parts[2:], "/"))
		return
	}
	if len(parts) >= 2 && parts[1] == "instances" {
		addonInstancesHandler(w, r, userID, admin, ref, parts[2:])
		return
	}
	if len(parts) == 3 && parts[1] == "call" {
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", 405)
			return
		}
		if !admin || ref.Owner > 0 {
			jsonError(w, "Admin access required", 403)
			return
		}
		addonCallHandler(w, r, userID, ref.ID, parts[2])
		return
	}
	// Managing an addon: the administrator for organisation addons, the owner
	// for a personal one (while personal addons are allowed; turning one off
	// stays possible either way).
	if ref.Owner == 0 && !admin {
		jsonError(w, "Admin access required", 403)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		addonRemove(w, r, userID, ref)
		return
	}
	// Network approvals are an administrator's, also for personal addons.
	if len(parts) == 2 && parts[1] == "http" {
		if !admin {
			jsonError(w, "Admin access required", 403)
			return
		}
		addonHTTPGrantHandler(w, r, userID, ref)
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	switch parts[1] {
	case "enable":
		if ref.Owner > 0 && !addonPersonalAllowed(userID) {
			addonFail(w, addonErrf("E_PERSONAL_OFF", "Personal addons are turned off by the administrator (addon_personal)"), 403)
			return
		}
		addonSetEnabled(w, r, userID, ref, true)
	case "disable":
		addonSetEnabled(w, r, userID, ref, false)
	default:
		jsonError(w, "Not found", 404)
	}
}
