// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ─── ADDON TEMPLATES ─────────────────────────────────
//
// Starter addons built into the binary (addon_templates/): wrm -addon-new
// writes one into a folder, Settings → Addons offers them as ZIPs. Panels get
// the shared bridge helper (_shared/wrm-bridge.js); files ending in .tmpl lose
// the suffix (so the TinyGo source is not part of this Go module), and the
// WebAssembly template gets a placeholder module so it installs at once.

//go:embed all:addon_templates
var addonTemplatesFS embed.FS

type addonTemplateInfo struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Server      bool   `json:"server,omitempty"`
}

var addonTemplateOrder = []string{"blank", "fetch-and-render", "grafana-panel", "wasm-hook"}

func addonTemplateList() []addonTemplateInfo {
	out := []addonTemplateInfo{}
	for _, name := range addonTemplateOrder {
		raw, err := addonTemplatesFS.ReadFile("addon_templates/" + name + "/addon.json")
		if err != nil {
			continue
		}
		m, err := addonDecodeManifest(raw)
		if err != nil {
			continue
		}
		out = append(out, addonTemplateInfo{Name: name, ID: m.ID, Title: m.Name, Description: m.Description, Server: m.Server != nil})
	}
	return out
}

func addonTemplateKnown(name string) bool {
	for _, n := range addonTemplateOrder {
		if n == name {
			return true
		}
	}
	return false
}

// addonTemplateFiles returns the files of a template, with the manifest id
// replaced when id is given.
func addonTemplateFiles(name, id string) (map[string][]byte, error) {
	if !addonTemplateKnown(name) {
		return nil, fmt.Errorf("unknown template %q (one of %s)", name, strings.Join(addonTemplateOrder, ", "))
	}
	if id != "" && !addonIDPattern.MatchString(id) {
		return nil, fmt.Errorf("the addon id must be 2-64 characters: lowercase letters, digits, dot, dash or underscore")
	}
	root := "addon_templates/" + name
	files := map[string][]byte{}
	err := fs.WalkDir(addonTemplatesFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := addonTemplatesFS.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(p, root+"/"), ".tmpl")
		files[rel] = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	m, err := addonDecodeManifest(files["addon.json"])
	if err != nil {
		return nil, err
	}
	if m.UI != nil && m.UI.Panel != "" {
		b, err := addonTemplatesFS.ReadFile("addon_templates/_shared/wrm-bridge.js")
		if err != nil {
			return nil, err
		}
		files["wrm-bridge.js"] = b
	}
	if m.Server != nil {
		files[m.Server.Module] = addonStarterWASM(`{"ok":true}`, m.Server.Exports)
	}
	if id != "" {
		old := []byte(`"id": "` + m.ID + `"`)
		files["addon.json"] = bytes.Replace(files["addon.json"], old, []byte(`"id": "`+id+`"`), 1)
	}
	return files, nil
}

func addonTemplateZip(name, id string) ([]byte, error) {
	files, err := addonTemplateFiles(name, id)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[n]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// addonTemplateWrite creates dir (which must not exist or be empty) with the
// template's files (wrm -addon-new).
func addonTemplateWrite(name, id, dir string) ([]string, error) {
	files, err := addonTemplateFiles(name, id)
	if err != nil {
		return nil, err
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("%s is not empty", dir)
	}
	written := []string{}
	for rel, b := range files {
		if !addonSafeRel(rel) {
			return nil, fmt.Errorf("unsafe template path %q", rel)
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, b, 0o600); err != nil {
			return nil, err
		}
		written = append(written, rel)
	}
	sort.Strings(written)
	return written, nil
}

func addonWasmLeb(size int) []byte {
	var n uint32
	if size > 0 && int64(size) <= math.MaxUint32 {
		n = uint32(size)
	}
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n == 0 {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

func addonWasmSection(id byte, body []byte) []byte {
	out := append([]byte{id}, addonWasmLeb(len(body))...)
	return append(out, body...)
}

func addonWasmName(name string, kind byte, idx int) []byte {
	out := append(addonWasmLeb(len(name)), name...)
	out = append(out, kind)
	return append(out, addonWasmLeb(idx)...)
}

// addonStarterWASM is a minimal module: memory, wrm_alloc (a fixed input
// buffer) and every export returning the same NUL-terminated result.
func addonStarterWASM(result string, exports []string) []byte {
	data := append([]byte(result), 0)
	b := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	types := []byte{0x02, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f, 0x60, 0x01, 0x7f, 0x01, 0x7f}
	b = append(b, addonWasmSection(1, types)...)
	b = append(b, addonWasmSection(3, []byte{0x02, 0x01, 0x00})...) // wrm_alloc (type 1), result (type 0)
	b = append(b, addonWasmSection(5, []byte{0x01, 0x00, 0x02})...) // two pages; input at 64 KiB
	ex := addonWasmLeb(2 + len(exports))
	ex = append(ex, addonWasmName("memory", 0x02, 0)...)
	ex = append(ex, addonWasmName("wrm_alloc", 0x00, 0)...)
	for _, e := range exports {
		ex = append(ex, addonWasmName(e, 0x00, 1)...)
	}
	b = append(b, addonWasmSection(7, ex)...)
	code := []byte{0x02}
	code = append(code, 0x06, 0x00, 0x41, 0x80, 0x80, 0x04, 0x0b) // return 65536
	code = append(code, 0x04, 0x00, 0x41, 0x08, 0x0b)             // return 8
	b = append(b, addonWasmSection(10, code)...)
	seg := []byte{0x01, 0x00, 0x41, 0x08, 0x0b}
	seg = append(seg, addonWasmLeb(len(data))...)
	seg = append(seg, data...)
	return append(b, addonWasmSection(11, seg)...)
}

// addonTemplateName validates a template name taken from a URL.
func addonTemplateName(s string) (string, bool) {
	s = strings.TrimSuffix(s, ".zip")
	if path.Base(s) != s || !addonTemplateKnown(s) {
		return "", false
	}
	return s, true
}
