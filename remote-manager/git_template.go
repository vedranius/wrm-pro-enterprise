// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// ─── GIT INSTALL: templates of protected (per-host) files ───
//
// A fresh installation needs its protected files (configuration, .env, …), which the
// repository does not carry. They are filled from a template in the repository
// (config.ini.example, .env.sample, settings.example.py, …: its placeholders and key = value
// lines become form fields), copied from another installation of the same service, or
// entered by hand. Field values may come from the credentials vault; they are used on the
// server only and never stored with the run.

const (
	gitMaxTemplate   = 256 << 10
	gitMaxFillFile   = 1 << 20
	gitMaxFields     = 200
	gitMaxSlots      = 100
	gitProtectedMode = "640"
)

var gitTemplateRe = regexp.MustCompile(`(?i)^(.+?)[._-](?:example|sample|template|tmpl|tpl|dist|default)(\.[^.]+)?$`)

// templateDest returns the file a template fills (config.ini.example → config.ini,
// .env.sample → .env, settings.example.py → settings.py), or "" for other files.
func templateDest(rel string) string {
	base := path.Base(rel)
	m := gitTemplateRe.FindStringSubmatch(base)
	if m == nil || m[1] == "." || m[1] == "" {
		return ""
	}
	dest := m[1] + m[2]
	if dir := path.Dir(rel); dir != "." {
		return dir + "/" + dest
	}
	return dest
}

// tmplField is one form field of a template: a placeholder ({{ name }}, ${NAME}, __NAME__,
// @NAME@, <NAME>) or the value of a key = value / key: value line.
type tmplField struct {
	ID      string `json:"id"` // the placeholder as written, or "line:<n>"
	Name    string `json:"name"`
	Kind    string `json:"kind"` // placeholder | key
	Default string `json:"default"`
	Secret  bool   `json:"secret"`
}

var (
	tmplPlaceholderRe = regexp.MustCompile(`\{\{\s*([A-Za-z_][\w.-]*)\s*\}\}|\$\{([A-Za-z_]\w*)(?::?-([^}]*))?\}|__([A-Z][A-Z0-9_]*[A-Z0-9])__|@([A-Z][A-Z0-9_]*)@|<([A-Z][A-Z0-9_]{2,})>`)
	tmplKeyRe         = regexp.MustCompile(`^(\s*(?:export\s+)?)("?)([A-Za-z_][\w.\-]*)("?)(\s*[=:]\s*)(.*?)\s*$`)
	tmplSectionRe     = regexp.MustCompile(`^\s*\[([^\]]+)\]\s*$`)
	tmplSecretRe      = regexp.MustCompile(`(?i)pass|pwd|secret|token|api_?key|private|credential|auth`)
)

// tmplLine is a key line of a template: prefix, quotes, separator and value.
type tmplLine struct {
	pre, key, sep, value, quote, suffix string
}

func parseTmplLine(l string) (tmplLine, bool) {
	l = strings.TrimSuffix(l, "\r")
	t := strings.TrimSpace(l)
	if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "//") || strings.HasPrefix(t, "[") || strings.HasPrefix(t, "-") {
		return tmplLine{}, false
	}
	m := tmplKeyRe.FindStringSubmatch(l)
	if m == nil || m[2] != m[4] {
		return tmplLine{}, false
	}
	x := tmplLine{pre: m[1] + m[2], key: m[3], sep: m[4] + m[5], value: m[6]}
	if m[2] == `"` && strings.HasSuffix(x.value, ",") { // JSON: "key": "value",
		x.value, x.suffix = strings.TrimSuffix(x.value, ","), ","
	}
	if strings.Contains(x.sep, ":") && x.value == "" {
		return tmplLine{}, false // a YAML / JSON parent key
	}
	if strings.HasPrefix(x.value, "{") || strings.HasPrefix(x.value, "[") {
		return tmplLine{}, false
	}
	if len(x.value) >= 2 && (x.value[0] == '"' || x.value[0] == '\'') && x.value[len(x.value)-1] == x.value[0] {
		x.quote = x.value[:1]
		x.value = x.value[1 : len(x.value)-1]
	} else if i := strings.Index(x.value, " #"); i >= 0 {
		x.suffix = x.value[i:] + x.suffix
		x.value = strings.TrimSpace(x.value[:i])
	}
	return x, true
}

// templateFields lists the fields of a template: placeholders first, then key lines
// whose value has no placeholder (INI keys are named <section>.<key>).
func templateFields(content []byte) []tmplField {
	text := string(content)
	fields := []tmplField{}
	seen := map[string]bool{}
	for _, m := range tmplPlaceholderRe.FindAllStringSubmatch(text, -1) {
		if seen[m[0]] || len(fields) >= gitMaxFields {
			continue
		}
		seen[m[0]] = true
		name := ""
		for _, g := range []int{1, 2, 4, 5, 6} {
			if m[g] != "" {
				name = m[g]
			}
		}
		fields = append(fields, tmplField{ID: m[0], Name: name, Kind: "placeholder", Default: m[3], Secret: tmplSecretRe.MatchString(name)})
	}
	section := ""
	for i, l := range strings.Split(text, "\n") {
		if m := tmplSectionRe.FindStringSubmatch(strings.TrimSuffix(l, "\r")); m != nil {
			section = strings.TrimSpace(m[1])
			continue
		}
		if len(fields) >= gitMaxFields {
			break
		}
		x, ok := parseTmplLine(l)
		if !ok || tmplPlaceholderRe.MatchString(x.value) {
			continue
		}
		name := x.key
		if section != "" {
			name = section + "." + x.key
		}
		fields = append(fields, tmplField{ID: fmt.Sprintf("line:%d", i+1), Name: name, Kind: "key", Default: x.value, Secret: tmplSecretRe.MatchString(x.key)})
	}
	return fields
}

// fillTemplate replaces the placeholders and key values that have a value; it returns the
// filled content and the placeholders left unfilled.
func fillTemplate(content []byte, values map[string]string) ([]byte, []string) {
	lines := strings.Split(string(content), "\n")
	for i, l := range lines {
		v, ok := values[fmt.Sprintf("line:%d", i+1)]
		if !ok {
			continue
		}
		x, parsed := parseTmplLine(l)
		if !parsed {
			continue
		}
		cr := ""
		if strings.HasSuffix(l, "\r") {
			cr = "\r"
		}
		if x.quote != "" {
			v = strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), x.quote, `\`+x.quote)
		}
		lines[i] = x.pre + x.key + x.sep + x.quote + v + x.quote + x.suffix + cr
	}
	text := strings.Join(lines, "\n")
	ids := make([]string, 0, len(values))
	for id := range values {
		if !strings.HasPrefix(id, "line:") {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return len(ids[i]) > len(ids[j]) })
	for _, id := range ids {
		text = strings.ReplaceAll(text, id, values[id])
	}
	left := []string{}
	seen := map[string]bool{}
	for _, m := range tmplPlaceholderRe.FindAllStringSubmatch(text, -1) {
		if !seen[m[0]] {
			seen[m[0]] = true
			left = append(left, m[0])
		}
	}
	return []byte(text), left
}

// ─── slots: the protected files of a new installation ─

type gitTemplateInfo struct {
	Path      string      `json:"path"`
	Available bool        `json:"available"`
	Fields    []tmplField `json:"fields"`
	Error     string      `json:"error,omitempty"`
}

type gitSlotCopy struct {
	InstallID int    `json:"install_id"`
	ConnName  string `json:"conn_name"`
	Path      string `json:"path"`
}

// gitSlot is one protected file to fill: from a template, a copy or by hand.
type gitSlot struct {
	Path      string            `json:"path"`
	Protected bool              `json:"protected"`
	InRepo    bool              `json:"in_repo"` // the repository has the file itself (its content is a template too)
	Templates []gitTemplateInfo `json:"templates"`
	Copies    []gitSlotCopy     `json:"copies"`
}

// installSlots finds the files to fill for a new installation: templates whose file is not
// a regular file of the target, protected files of the repository, literal protected names
// and the protected files of other installations of the service.
func installSlots(userID int, t gitTarget, providers map[int]*gitProvider) []gitSlot {
	by := map[string]*gitSlot{}
	get := func(rel string) *gitSlot {
		if s := by[rel]; s != nil {
			return s
		}
		s := &gitSlot{Path: rel, Protected: globHit(rel, t.Protected), Templates: []gitTemplateInfo{}, Copies: []gitSlotCopy{}}
		by[rel] = s
		return s
	}
	rels := keysOf(t.Files)
	sort.Strings(rels)
	for _, rel := range rels {
		if toolFiles[rel] {
			continue
		}
		if dest := templateDest(rel); dest != "" {
			if _, regular := t.Files[dest]; !regular || globHit(dest, t.Protected) {
				get(dest).Templates = append(get(dest).Templates, gitTemplateInfo{Path: rel})
			}
		}
		if globHit(rel, t.Protected) && templateDest(rel) == "" {
			s := get(rel)
			s.InRepo = true
			s.Templates = append(s.Templates, gitTemplateInfo{Path: rel})
		}
	}
	for _, g := range t.Protected {
		if !strings.ContainsAny(g, "*?[") && !toolFiles[g] {
			get(g)
		}
	}
	for _, in := range loadGitInstalls(userID, true, "i.app=?", t.App) {
		for _, f := range in.Files {
			if f.State == "protected" && f.ServerHash != "" && f.ServerHash != "-" && !toolFiles[f.Path] {
				s := get(f.Path)
				s.Copies = append(s.Copies, gitSlotCopy{InstallID: in.ID, ConnName: in.ConnName, Path: in.Path})
			}
		}
	}
	out := make([]gitSlot, 0, len(by))
	for _, s := range by {
		for i := range s.Templates {
			ti := &s.Templates[i]
			ti.Fields = []tmplField{}
			data, err := gitFileContent(userID, t, ti.Path, providers)
			switch {
			case err != nil:
				ti.Error = "the content is not available (a protected file of a bundle)"
			case len(data) > gitMaxTemplate || strings.IndexByte(string(data), 0) >= 0:
				ti.Error = "not a text template"
			default:
				ti.Available = true
				ti.Fields = templateFields(data)
			}
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	if len(out) > gitMaxSlots {
		out = out[:gitMaxSlots]
	}
	return out
}
