// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─── ADDON LINT ──────────────────────────────────────
//
// addonLintArchive checks an addon exactly as an install would (manifest,
// files, WebAssembly) and then reads its panel code for what the sandbox will
// refuse at run time: remote scripts, inline handlers, eval, own network,
// browser storage, secrets in the ZIP, sites missing from http.origins. Every
// finding is {level, code, message, file, line, hint}, so an AI that wrote
// the addon can repair it without guessing. Nothing in the addon is executed.

type addonDiag struct {
	Level   string `json:"level"` // error, warning, info
	Code    string `json:"code"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

type addonLintReport struct {
	OK          bool        `json:"ok"`
	ID          string      `json:"id,omitempty"`
	Name        string      `json:"name,omitempty"`
	Version     string      `json:"version,omitempty"`
	APIVersion  int         `json:"apiVersion,omitempty"`
	Files       int         `json:"files"`
	Bytes       int64       `json:"bytes"`
	Errors      int         `json:"errors"`
	Warnings    int         `json:"warnings"`
	Diagnostics []addonDiag `json:"diagnostics"`
	Permissions []string    `json:"permissions"`
	Summary     []string    `json:"summary"`
	CSP         string      `json:"csp,omitempty"`
	manifest    *addonManifest
}

var addonLintHints = map[string]string{
	"E_LINT_REMOTE":         "The panel's CSP only loads files from the ZIP: copy the script, stylesheet, font or image into the ZIP.",
	"E_LINT_INLINE_HANDLER": "Inline event handlers do not run under the CSP: give the element an id and use addEventListener in a script.",
	"E_LINT_JS_URL":         "javascript: links do not run under the CSP: use a button with addEventListener.",
	"E_LINT_EVAL":           "eval, new Function and string timers are blocked by the CSP: call functions directly.",
	"E_LINT_NETWORK":        "A panel has no network of its own: declare the site in http and send {v:2, type:'wrm:http', …} to WRM.",
	"E_LINT_STORAGE":        "The panel runs with an opaque origin, so localStorage, sessionStorage, IndexedDB and cookies throw: use wrm:store.set and wrm:store.get.",
	"E_LINT_SECRET":         "Never ship tokens or passwords in the ZIP: declare them in http.secrets and enter the value in WRM (🔑 Secrets).",
	"E_LINT_ORIGIN":         "Add the site to http.origins (exact scheme://host[:port]) or remove the URL.",
	"E_LINT_READY":          "Send {v:2, type:'wrm:ready'} when the panel has loaded, or WRM shows 'did not start' after 5 seconds.",
	"E_LINT_HEARTBEAT":      "Send {v:2, type:'wrm:heartbeat'} every 15 seconds, or WRM shows 'not responding'.",
	"E_LINT_CAPABILITY":     "Add the capability to addon.json, or the bridge refuses the message with E_CAPABILITY.",
	"E_LINT_UNSERVED":       "Only html, css, js, json, images, fonts, txt and md files are served; other files stay unused.",
	"E_LINT_V1":             "apiVersion 2 gives the panel a store, notifications, web requests and a watchdog.",
	"E_LINT_SERVER":         "Personal addons cannot carry server code; an administrator installs it for the organisation.",
	"E_LINT_HIDDEN":         "Hidden files and folders are left out of the ZIP the dev tools build.",
	"E_WASM":                "Export memory, wrm_alloc(size) and every function in server.exports; the only import is wrm.log.",
}

var (
	addonLintRemoteScript = regexp.MustCompile(`(?i)<script\b[^>]*\bsrc\s*=\s*["']?\s*(https?:)?//`)
	addonLintRemoteLink   = regexp.MustCompile(`(?i)<link\b[^>]*\bhref\s*=\s*["']?\s*(https?:)?//`)
	addonLintRemoteImg    = regexp.MustCompile(`(?i)<(img|source|video|audio|iframe)\b[^>]*\bsrc\s*=\s*["']?\s*(https?:)?//`)
	addonLintRemoteCSS    = regexp.MustCompile(`(?i)(@import\s+(url\()?|url\()\s*["']?\s*(https?:)?//`)
	addonLintInlineEvent  = regexp.MustCompile(`(?i)<[a-z][a-z0-9-]*\b[^>]*\son[a-z]{3,24}\s*=`)
	addonLintJSURL        = regexp.MustCompile(`(?i)\b(href|src|action)\s*=\s*["']\s*javascript:`)
	addonLintEval         = regexp.MustCompile(`\beval\s*\(|\bnew\s+Function\s*\(|\bset(Timeout|Interval)\s*\(\s*["'\x60]`)
	addonLintNetwork      = regexp.MustCompile(`\bfetch\s*\(|\bXMLHttpRequest\b|\bnew\s+WebSocket\s*\(|\bnew\s+EventSource\s*\(|\bnavigator\.sendBeacon\s*\(`)
	addonLintStorage      = regexp.MustCompile(`\b(localStorage|sessionStorage|indexedDB)\b|\bdocument\.cookie\b`)
	addonLintTokenShape   = regexp.MustCompile(`\b(glsa_[A-Za-z0-9_]{16,}|glc_[A-Za-z0-9+/=]{20,}|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|xox[abprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,})`)
	addonLintSecretAssign = regexp.MustCompile(`(?i)["']?\b(api[_-]?key|apikey|access[_-]?token|auth[_-]?token|token|password|passwd|secret|bearer)\b["']?\s*[:=]\s*["']([A-Za-z0-9_\-.=+/]{12,})["']`)
	addonLintURL          = regexp.MustCompile(`(?i)\bhttps?://[a-z0-9.-]+(:[0-9]{1,5})?`)
	addonLintScriptOrHTML = map[string]bool{".html": true, ".htm": true, ".js": true, ".mjs": true}
)

// addonLintIgnoredOrigins appear in code without being requests (XML
// namespaces, documentation placeholders).
var addonLintIgnoredOrigins = map[string]bool{
	"http://www.w3.org": true, "https://www.w3.org": true,
	"http://example.com": true, "https://example.com": true,
}

func (r *addonLintReport) add(level, code, file string, line int, format string, args ...interface{}) {
	hint := addonLintHints[code]
	if hint == "" {
		hint = addonErrorHints[code]
	}
	r.Diagnostics = append(r.Diagnostics, addonDiag{Level: level, Code: code, Message: fmt.Sprintf(format, args...), File: file, Line: line, Hint: hint})
}

func (r *addonLintReport) finish() addonLintReport {
	r.Errors, r.Warnings = 0, 0
	for _, d := range r.Diagnostics {
		switch d.Level {
		case "error":
			r.Errors++
		case "warning":
			r.Warnings++
		}
	}
	rank := map[string]int{"error": 0, "warning": 1, "info": 2}
	sort.SliceStable(r.Diagnostics, func(i, j int) bool {
		return rank[r.Diagnostics[i].Level] < rank[r.Diagnostics[j].Level]
	})
	r.OK = r.Errors == 0
	if r.Diagnostics == nil {
		r.Diagnostics = []addonDiag{}
	}
	if r.Permissions == nil {
		r.Permissions = []string{}
	}
	if r.Summary == nil {
		r.Summary = []string{}
	}
	return *r
}

// addonLintManifestRaw finds addon.json in the archive the way the installer
// does, so a JSON error can be given a line number.
func addonLintManifestRaw(body []byte) []byte {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil
	}
	var pick *zip.File
	for _, f := range zr.File {
		n := strings.TrimSuffix(f.Name, "/")
		if n == "addon.json" {
			pick = f
			break
		}
		if strings.Count(n, "/") == 1 && strings.HasSuffix(n, "/addon.json") && pick == nil {
			pick = f
		}
	}
	if pick == nil || pick.UncompressedSize64 > 256<<10 {
		return nil
	}
	rc, err := pick.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	raw, _ := io.ReadAll(io.LimitReader(rc, 256<<10))
	return bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
}

func addonLintLineAt(raw []byte, offset int64) int {
	if offset <= 0 || int(offset) > len(raw) {
		return 0
	}
	return bytes.Count(raw[:offset], []byte("\n")) + 1
}

// addonLintManifestLine guesses the line of a manifest error: the JSON
// offset for syntax and type errors, otherwise the first line naming the
// field the message quotes.
var addonLintQuoted = regexp.MustCompile(`"([A-Za-z0-9_.:\-]{1,64})"`)

func addonLintManifestLine(raw []byte, msg string) int {
	if raw == nil {
		return 0
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		var se *json.SyntaxError
		var te *json.UnmarshalTypeError
		switch {
		case errors.As(err, &se):
			return addonLintLineAt(raw, se.Offset)
		case errors.As(err, &te):
			return addonLintLineAt(raw, te.Offset)
		}
		return 0
	}
	for _, m := range addonLintQuoted.FindAllStringSubmatch(msg, -1) {
		name := m[1]
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		if i := bytes.Index(raw, []byte(`"`+name+`"`)); i >= 0 {
			return addonLintLineAt(raw, int64(i))
		}
	}
	return 0
}

// addonLintArchive lints a ZIP as Settings → Addons would receive it.
func addonLintArchive(ctx context.Context, body []byte) addonLintReport {
	rep := &addonLintReport{Bytes: int64(len(body))}
	if len(body) > addonMaxArchive {
		rep.add("error", "E_LIMIT_SIZE", "", 0, "The archive is larger than 20 MB")
		return rep.finish()
	}
	m, prefix, err := addonReadArchive(body)
	if err != nil {
		code, msg := "E_MANIFEST", err.Error()
		var ae *addonError
		if errors.As(err, &ae) {
			code = ae.Code
		}
		file, line := "addon.json", 0
		if code == "E_MISSING_FILE" {
			file = ""
		} else if strings.Contains(msg, "addon.json") || strings.HasPrefix(code, "E_MANIFEST") || code == "E_API_VERSION" {
			line = addonLintManifestLine(addonLintManifestRaw(body), msg)
		} else {
			file = ""
		}
		rep.add("error", code, file, line, "%s", msg)
		return rep.finish()
	}
	rep.manifest = m
	rep.ID, rep.Name, rep.Version, rep.APIVersion = m.ID, m.Name, m.Version, m.APIVersion
	if rep.APIVersion == 0 {
		rep.APIVersion = 1
	}
	tmp, err := os.MkdirTemp("", "wrm-addon-lint-")
	if err != nil {
		rep.add("error", "E_LINT_IO", "", 0, "A temporary folder cannot be created: %v", err)
		return rep.finish()
	}
	defer os.RemoveAll(tmp)
	if err := addonWriteFiles(body, prefix, tmp); err != nil {
		rep.add("error", "E_LINT_IO", "", 0, "The archive cannot be unpacked: %v", err)
		return rep.finish()
	}
	addonLintTree(ctx, rep, m, tmp)
	return rep.finish()
}

// addonLintTree reads the unpacked addon.
func addonLintTree(ctx context.Context, rep *addonLintReport, m *addonManifest, dir string) {
	caps := map[string]bool{}
	for _, c := range m.Capabilities {
		caps[c] = true
	}
	v2 := m.APIVersion >= 2
	listed := map[string]bool{}
	if m.HTTP != nil {
		for _, o := range m.HTTP.Origins {
			listed[o] = true
		}
	}
	secretNames := map[string]bool{}
	if m.HTTP != nil {
		for _, sp := range m.HTTP.secretSpecs() {
			secretNames[sp.Name] = true
		}
	}
	declared := map[string]bool{"addon.json": true}
	if m.Server != nil {
		declared[m.Server.Module] = true
	}
	// code is every script; own leaves out the bridge helper of the templates
	// (wrm-bridge.js), which mentions every message type.
	var code, own strings.Builder
	perFile := map[string]int{}
	report := func(level, ecode, file string, line int, format string, args ...interface{}) {
		k := file + "|" + ecode
		perFile[k]++
		if perFile[k] > 5 {
			return
		}
		rep.add(level, ecode, file, line, format, args...)
	}
	fsRoot, err := os.OpenRoot(dir)
	if err != nil {
		rep.add("error", "E_LINT_IO", "", 0, "%v", err)
		return
	}
	defer fsRoot.Close()
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		rep.Files++
		ext := strings.ToLower(path.Ext(rel))
		if _, ok := addonAssetType(rel); !ok && !declared[rel] {
			report("info", "E_LINT_UNSERVED", rel, 0, "%s is never served to the panel", rel)
		}
		if !addonLintScriptOrHTML[ext] && ext != ".css" && ext != ".json" && ext != ".md" && ext != ".txt" {
			return nil
		}
		b, err := fsRoot.ReadFile(filepath.FromSlash(rel))
		if err != nil {
			return nil
		}
		if addonLintScriptOrHTML[ext] {
			code.Write(b)
			code.WriteByte('\n')
			if path.Base(rel) != "wrm-bridge.js" {
				own.Write(b)
				own.WriteByte('\n')
			}
		}
		html := ext == ".html" || ext == ".htm"
		for i, ln := range strings.Split(string(b), "\n") {
			n := i + 1
			if tok := addonLintTokenShape.FindString(ln); tok != "" {
				report("error", "E_LINT_SECRET", rel, n, "This looks like an access token (%s…)", tok[:min(6, len(tok))])
			} else if sm := addonLintSecretAssign.FindStringSubmatch(ln); sm != nil && ext != ".md" && ext != ".txt" && !secretNames[sm[2]] {
				report("warning", "E_LINT_SECRET", rel, n, "A password or token seems to be written into the file")
			}
			if html {
				if addonLintRemoteScript.MatchString(ln) {
					report("error", "E_LINT_REMOTE", rel, n, "A script is loaded from another site; the CSP blocks it")
				}
				if addonLintRemoteLink.MatchString(ln) {
					report("warning", "E_LINT_REMOTE", rel, n, "A stylesheet or font is loaded from another site; the CSP blocks it")
				}
				if addonLintRemoteImg.MatchString(ln) {
					report("warning", "E_LINT_REMOTE", rel, n, "An image or media file is loaded from another site; the CSP blocks it")
				}
				if addonLintInlineEvent.MatchString(ln) {
					report("warning", "E_LINT_INLINE_HANDLER", rel, n, "An inline event handler (on…=) does not run")
				}
				if addonLintJSURL.MatchString(ln) {
					report("warning", "E_LINT_JS_URL", rel, n, "A javascript: link does not run")
				}
			}
			if html || ext == ".css" {
				if addonLintRemoteCSS.MatchString(ln) {
					report("warning", "E_LINT_REMOTE", rel, n, "CSS loads a file from another site; the CSP blocks it")
				}
			}
			if !addonLintScriptOrHTML[ext] {
				continue
			}
			if addonLintEval.MatchString(ln) {
				report("warning", "E_LINT_EVAL", rel, n, "eval, new Function or a string timer is blocked by the CSP")
			}
			if v2 && addonLintNetwork.MatchString(ln) {
				report("warning", "E_LINT_NETWORK", rel, n, "The panel tries to use the network itself (fetch, XMLHttpRequest, WebSocket); the CSP blocks it")
			}
			if addonLintStorage.MatchString(ln) {
				report("warning", "E_LINT_STORAGE", rel, n, "Browser storage is not available in the sandboxed panel")
			}
			// A remote file is reported above; only requests need http.origins.
			remote := html && (addonLintRemoteScript.MatchString(ln) || addonLintRemoteLink.MatchString(ln) || addonLintRemoteImg.MatchString(ln))
			if caps["http"] && !remote {
				for _, u := range addonLintURL.FindAllString(ln, -1) {
					o, err := addonNormalizeOrigin(u)
					if err != nil || listed[o] || addonLintIgnoredOrigins[o] {
						continue
					}
					report("warning", "E_LINT_ORIGIN", rel, n, "%s is not in http.origins; WRM refuses requests to it (E_HTTP_ORIGIN)", o)
				}
			}
		}
		return nil
	})
	all := code.String()
	if m.UI != nil && m.UI.Panel != "" {
		if html, err := fsRoot.ReadFile(filepath.FromSlash(m.UI.Panel)); err == nil && v2 {
			rep.CSP = addonPanelCSP(html)
		}
		if v2 {
			if !strings.Contains(all, "wrm:ready") {
				rep.add("warning", "E_LINT_READY", m.UI.Panel, 0, "The panel never sends wrm:ready")
			}
			if !strings.Contains(all, "wrm:heartbeat") {
				rep.add("warning", "E_LINT_HEARTBEAT", m.UI.Panel, 0, "The panel never sends wrm:heartbeat")
			}
		} else {
			rep.add("info", "E_LINT_V1", "addon.json", 0, "This is an apiVersion 1 addon")
		}
	}
	mine := own.String()
	for _, u := range []struct{ msg, helper, c string }{
		{"wrm:store.", "wrm.store", "store"}, {"wrm:notify", "wrm.notify(", "notify"}, {"wrm:http", "wrm.http(", "http"},
	} {
		if (strings.Contains(mine, u.msg) || strings.Contains(mine, u.helper)) && !caps[u.c] {
			rep.add("warning", "E_LINT_CAPABILITY", "addon.json", 0, "The code uses %s but the %s capability is not declared", strings.TrimSuffix(u.msg, "."), u.c)
		}
	}
	if m.Server != nil {
		rep.add("info", "E_LINT_SERVER", "addon.json", 0, "Server code: only an administrator can install this addon, for the organisation")
		if err := addonCheckWASM(ctx, m, dir); err != nil {
			rep.add("error", "E_WASM", m.Server.Module, 0, "%v", err)
		}
	}
	rep.Permissions = addonPermissions(m)
	for _, a := range rep.Permissions {
		rep.Summary = append(rep.Summary, addonPermissionText(a))
	}
}

// addonPermissionText is the English line of a permission atom, as the
// permission sheet shows it.
func addonPermissionText(a string) string {
	kind, val, _ := strings.Cut(a, ":")
	switch kind {
	case "panel":
		return "A window inside WRM, without a network of its own"
	case "themes":
		return val + " colour theme(s) users can pick in Settings → Appearance"
	case "store":
		kb, shared, _ := strings.Cut(val, ":")
		if shared == "shared" {
			return kb + " KB of settings per instance, shared by everybody and written by administrators"
		}
		return kb + " KB of settings per instance"
	case "notify":
		return "Short notifications"
	case "http":
		return "Requests to " + val + " through WRM, after an administrator approves the site"
	case "http.methods":
		return "Methods: " + strings.ReplaceAll(val, ",", ", ")
	case "http.paths":
		return "Paths: " + strings.ReplaceAll(val, ",", ", ")
	case "secret":
		return "Uses the secret " + val + " (entered in WRM, never shown to the panel)"
	case "server":
		return "WebAssembly server code with the exports " + strings.ReplaceAll(val, ",", ", ")
	}
	return a
}

// addonZipDir packs a development folder like a draft, leaving out hidden
// files and folders (.git, .wrm-dev, editor files).
func addonZipDir(dir string) ([]byte, []string, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	var skipped []string
	var total int64
	count := 0
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, err
	}
	fsRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, nil, err
	}
	defer fsRoot.Close()
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" {
			skipped = append(skipped, rel)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symbolic link; an addon may only contain regular files", rel)
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", rel)
		}
		if !addonSafeRel(rel) {
			return fmt.Errorf("unsafe path %q", rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > addonMaxFile {
			return fmt.Errorf("%s is larger than 8 MB", rel)
		}
		total += info.Size()
		count++
		if total > addonMaxArchive || count > addonMaxFiles {
			return fmt.Errorf("the folder is larger than an addon may be (20 MB, %d files)", addonMaxFiles)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: rel, Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return err
		}
		f, err := fsRoot.Open(filepath.FromSlash(rel))
		if err != nil {
			return err
		}
		_, err = io.Copy(w, io.LimitReader(f, addonMaxFile+1))
		f.Close()
		return err
	})
	if err != nil {
		return nil, skipped, err
	}
	if err := zw.Close(); err != nil {
		return nil, skipped, err
	}
	return buf.Bytes(), skipped, nil
}

// addonLintPath lints a folder or a .zip file (wrm -addon-lint).
func addonLintPath(ctx context.Context, p string) addonLintReport {
	st, err := os.Stat(p)
	if err != nil {
		rep := &addonLintReport{}
		rep.add("error", "E_LINT_IO", "", 0, "%v", err)
		return rep.finish()
	}
	var body []byte
	var skipped []string
	if st.IsDir() {
		body, skipped, err = addonZipDir(p)
	} else {
		body, err = os.ReadFile(filepath.Clean(p))
	}
	if err != nil {
		rep := &addonLintReport{}
		rep.add("error", "E_LINT_IO", "", 0, "%v", err)
		return rep.finish()
	}
	rep := addonLintArchive(ctx, body)
	for _, s := range skipped {
		rep.Diagnostics = append(rep.Diagnostics, addonDiag{Level: "info", Code: "E_LINT_HIDDEN", Message: s + " is left out", File: s, Hint: addonLintHints["E_LINT_HIDDEN"]})
	}
	return rep
}

// addonLintText formats a report for the console.
func addonLintText(rep addonLintReport) string {
	var b strings.Builder
	if rep.ID != "" {
		fmt.Fprintf(&b, "%s %s (%s), apiVersion %d, %d files\n", rep.Name, rep.Version, rep.ID, rep.APIVersion, rep.Files)
	}
	for _, d := range rep.Diagnostics {
		loc := d.File
		if d.Line > 0 {
			loc += ":" + strconv.Itoa(d.Line)
		}
		if loc != "" {
			loc += ": "
		}
		fmt.Fprintf(&b, "%-7s %s%s [%s]\n", d.Level, loc, d.Message, d.Code)
		if d.Hint != "" && d.Level != "info" {
			fmt.Fprintf(&b, "        hint: %s\n", d.Hint)
		}
	}
	if len(rep.Summary) > 0 {
		b.WriteString("Permission sheet:\n")
		for _, s := range rep.Summary {
			b.WriteString("  - " + s + "\n")
		}
	}
	if rep.OK {
		fmt.Fprintf(&b, "OK: %d warning(s)\n", rep.Warnings)
	} else {
		fmt.Fprintf(&b, "FAILED: %d error(s), %d warning(s)\n", rep.Errors, rep.Warnings)
	}
	return b.String()
}
