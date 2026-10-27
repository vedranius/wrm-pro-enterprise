// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── GIT WORKSPACE: shared types and helpers ─────────
//
// The Git workspace compares services installed on servers with a Git repository (GitLab
// API v4, GitHub REST) or with an offline bundle. Phase 1 is read-only on servers: WRM
// discovers installations over SSH, hashes their files with POSIX tools and compares them
// file by file with the target ref and with the history of every file.
//
// The catalog JSON, the bundle format (bundle.json + payload) and VERSION.md match an
// existing file-based deploy tool, so both can be used side by side.

// gitCatalogApp is one service of the catalog (catalog JSON format).
type gitCatalogApp struct {
	Functionality *gitFunctionalitySpec `json:"functionality,omitempty"`
	Name          string                `json:"name"`
	Project       string                `json:"project"`
	Ref           string                `json:"ref"` // tag:latest | tag:vX | <branch>
	Branch        string                `json:"branch"`
	TagFilter     string                `json:"tag_filter"`
	Subdir        string                `json:"subdir"`
	Kind          string                `json:"kind"` // app | library | tool
	Include       []string              `json:"include"`
	Exclude       []string              `json:"exclude"`
	Protected     []string              `json:"protected"`
	Fingerprint   []string              `json:"fingerprint"`
	InstallHint   []string              `json:"install_hint"`
	// deploy environments (v11.5.0); a catalog without them works as before
	Environments []gitEnvironment `json:"environments,omitempty"`
}

type gitCatalogSource struct {
	URL    string   `json:"url"`
	Groups []string `json:"groups"`
}

// gitCatalog is the whole catalog of a user (catalog JSON format).
type gitCatalog struct {
	GitLab           gitCatalogSource `json:"gitlab"`
	FallbackBranches []string         `json:"fallback_branches"`
	ServerRoots      []string         `json:"server_roots"`
	Apps             []gitCatalogApp  `json:"apps"`
	IgnoreDirs       []string         `json:"ignore_dirs"`
}

var defaultIgnoreDirs = []string{"recyclebin", "trash", "*_BKP", "*_OLD", "BKP_*", "*backup*", "*.deploy-bak"}

var gitNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.+-]{0,99}$`)

func cleanList(in []string, max int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] || len(s) > 300 {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) >= max {
			break
		}
	}
	return out
}

func cleanRel(p string) string {
	p = strings.Trim(strings.ReplaceAll(strings.TrimSpace(p), "\\", "/"), "/")
	if p == "" || p == "." {
		return ""
	}
	return path.Clean(p)
}

func (a *gitCatalogApp) normalize() error {
	a.Name = strings.TrimSpace(a.Name)
	a.Project = strings.Trim(strings.TrimSpace(a.Project), "/")
	a.Ref = strings.TrimSpace(a.Ref)
	a.Branch = strings.TrimSpace(a.Branch)
	a.TagFilter = strings.TrimSpace(a.TagFilter)
	a.Subdir = cleanRel(a.Subdir)
	a.Kind = strings.ToLower(strings.TrimSpace(a.Kind))
	if !gitNameRe.MatchString(a.Name) {
		return fmt.Errorf("invalid service name %q (letters, digits, . _ + -)", a.Name)
	}
	if a.Project == "" || len(a.Project) > 300 || strings.Contains(a.Project, "..") {
		return fmt.Errorf("%s: enter the project (group/name)", a.Name)
	}
	if a.Ref == "" {
		a.Ref = "tag:latest"
	}
	if a.Kind == "" {
		a.Kind = "app"
	}
	if a.Kind != "app" && a.Kind != "library" && a.Kind != "tool" {
		return fmt.Errorf("%s: kind must be app, library or tool", a.Name)
	}
	if a.TagFilter != "" {
		if _, err := regexp.Compile(a.TagFilter); err != nil {
			return fmt.Errorf("%s: invalid tag filter: %v", a.Name, err)
		}
	}
	if strings.HasPrefix(a.Subdir, "..") {
		return fmt.Errorf("%s: invalid subdirectory", a.Name)
	}
	a.Include = cleanList(a.Include, 100)
	a.Exclude = cleanList(a.Exclude, 100)
	a.Protected = cleanList(a.Protected, 100)
	a.InstallHint = cleanList(a.InstallHint, 50)
	fp := []string{}
	for _, f := range cleanList(a.Fingerprint, 20) {
		if f = cleanRel(f); f != "" && !strings.HasPrefix(f, "..") {
			fp = append(fp, f)
		}
	}
	a.Fingerprint = fp
	if a.Functionality != nil {
		if err := a.Functionality.normalize(); err != nil {
			return fmt.Errorf("%s: %v", a.Name, err)
		}
	}
	envs, err := normalizeEnvs(a.Name, a.Environments)
	if err != nil {
		return err
	}
	a.Environments = envs
	return nil
}

func (c *gitCatalog) normalize() error {
	c.GitLab.URL = strings.TrimRight(strings.TrimSpace(c.GitLab.URL), "/")
	c.GitLab.Groups = cleanList(c.GitLab.Groups, 100)
	c.FallbackBranches = cleanList(c.FallbackBranches, 20)
	roots := []string{}
	for _, r := range cleanList(c.ServerRoots, 30) {
		if !strings.HasPrefix(r, "/") || strings.ContainsAny(r, "\n\r\t") {
			return fmt.Errorf("server roots must be absolute paths: %q", r)
		}
		roots = append(roots, path.Clean(r))
	}
	c.ServerRoots = roots
	if c.IgnoreDirs == nil {
		c.IgnoreDirs = append([]string(nil), defaultIgnoreDirs...)
	}
	c.IgnoreDirs = cleanList(c.IgnoreDirs, 100)
	if c.Apps == nil {
		c.Apps = []gitCatalogApp{}
	}
	if len(c.Apps) > 500 {
		return fmt.Errorf("at most 500 services")
	}
	seen := map[string]bool{}
	for i := range c.Apps {
		if err := c.Apps[i].normalize(); err != nil {
			return err
		}
		if seen[strings.ToLower(c.Apps[i].Name)] {
			return fmt.Errorf("duplicate service name %q", c.Apps[i].Name)
		}
		seen[strings.ToLower(c.Apps[i].Name)] = true
	}
	return checkEnvPaths(c.Apps)
}

func (c *gitCatalog) app(name string) (*gitCatalogApp, bool) {
	for i := range c.Apps {
		if c.Apps[i].Name == name {
			return &c.Apps[i], true
		}
	}
	return nil, false
}

// ─── fnmatch (Python semantics: * also matches /) ────

var fnmatchCache sync.Map

func fnmatchRe(pattern string) *regexp.Regexp {
	if re, ok := fnmatchCache.Load(pattern); ok {
		return re.(*regexp.Regexp)
	}
	var b strings.Builder
	b.WriteString("(?s)^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '[':
			j := i + 1
			if j < len(pattern) && pattern[j] == '!' {
				j++
			}
			if j < len(pattern) && pattern[j] == ']' {
				j++
			}
			for j < len(pattern) && pattern[j] != ']' {
				j++
			}
			if j >= len(pattern) {
				b.WriteString(`\[`)
				continue
			}
			stuff := pattern[i+1 : j]
			i = j
			stuff = strings.ReplaceAll(stuff, `\`, `\\`)
			if strings.HasPrefix(stuff, "!") {
				stuff = "^" + stuff[1:]
			} else if strings.HasPrefix(stuff, "^") {
				stuff = `\` + stuff
			}
			b.WriteString("[" + stuff + "]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		re = regexp.MustCompile("^" + regexp.QuoteMeta(pattern) + "$")
	}
	fnmatchCache.Store(pattern, re)
	return re
}

func fnmatch(name, pattern string) bool { return fnmatchRe(pattern).MatchString(name) }

// globHit matches a relative path or its base name (protected files, as the deploy tool does).
// A pattern starting with / is anchored: it matches the whole relative path only.
func globHit(rel string, globs []string) bool {
	base := path.Base(rel)
	for _, g := range globs {
		if strings.HasPrefix(g, "/") {
			if fnmatch(rel, g[1:]) {
				return true
			}
			continue
		}
		if fnmatch(rel, g) || fnmatch(base, g) {
			return true
		}
	}
	return false
}

// globHitDir is globHit that also matches every parent directory (include / exclude lists:
// "tests" excludes the whole directory).
func globHitDir(rel string, globs []string) bool {
	if globHit(rel, globs) {
		return true
	}
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		dir := strings.Join(parts[:i], "/")
		for _, g := range globs {
			if strings.HasPrefix(g, "/") {
				if fnmatch(dir, g[1:]) {
					return true
				}
				continue
			}
			if fnmatch(dir, g) || fnmatch(parts[i-1], g) {
				return true
			}
		}
	}
	return false
}

// ─── backups and ignored directories ─────────────────

var defaultBackupWords = "bkp,backup,bak,old,orig,prev,copy,kopija,stari,recyclebin,trash,snapshot"

func backupWords() []string {
	out := []string{}
	for _, w := range strings.Split(getSetting("git_backup_words"), ",") {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" {
			out = append(out, w)
		}
	}
	return out
}

func splitTokens(seg string) []string {
	return strings.FieldsFunc(strings.ToLower(seg), func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == ' ' || r == '~' || r == '(' || r == ')'
	})
}

// looksLikeBackup tells whether a path segment names a copy or backup (App_BKP, backup_2024,
// .deploy-bak, App.old, …): one of the words as a whole token of the segment.
func looksLikeBackup(seg string, words []string) bool {
	for _, tok := range splitTokens(seg) {
		for _, w := range words {
			if tok == w {
				return true
			}
		}
		// backup words glued to a date or number: bkp2024, old1
		for _, w := range words {
			if strings.HasPrefix(tok, w) && len(tok) > len(w) && tok[len(w)] >= '0' && tok[len(w)] <= '9' {
				return true
			}
		}
	}
	return false
}

// ignoredPath tells whether any segment of a relative path is a backup or matches the
// catalog's ignore_dirs.
func ignoredPath(rel string, ignore, words []string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" {
			continue
		}
		if looksLikeBackup(seg, words) {
			return true
		}
		for _, g := range ignore {
			if fnmatch(seg, g) || fnmatch(strings.ToLower(seg), strings.ToLower(g)) {
				return true
			}
		}
	}
	return false
}

// ─── versions ────────────────────────────────────────

var versionChunkRe = regexp.MustCompile(`\d+|\D+`)

// compareVersions sorts tags numerically chunk by chunk (v1.10 > v1.9).
func compareVersions(a, b string) int {
	ca, cb := versionChunkRe.FindAllString(a, -1), versionChunkRe.FindAllString(b, -1)
	for i := 0; i < len(ca) && i < len(cb); i++ {
		x, y := ca[i], cb[i]
		nx, ex := strconv.ParseUint(x, 10, 64)
		ny, ey := strconv.ParseUint(y, 10, 64)
		if ex == nil && ey == nil {
			if nx != ny {
				if nx < ny {
					return -1
				}
				return 1
			}
			continue
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(ca) < len(cb):
		return -1
	case len(ca) > len(cb):
		return 1
	}
	return 0
}

func sortVersionsDesc(tags []string) {
	sort.SliceStable(tags, func(i, j int) bool { return compareVersions(tags[i], tags[j]) > 0 })
}

// ─── hashes ──────────────────────────────────────────

// normHash is the SHA-256 of the content with CRLF → LF (the bundle format).
func normHash(content []byte) string {
	s := sha256.Sum256(bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n")))
	return hex.EncodeToString(s[:])
}

// trHash is the SHA-256 with every CR removed: what `tr -d '\r' < f | sha256sum` gives on a
// server. It differs from normHash only for files with a lone CR (binary files).
func trHash(content []byte) string {
	s := sha256.Sum256(bytes.ReplaceAll(content, []byte("\r"), nil))
	return hex.EncodeToString(s[:])
}

func short10(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

func nowStamp() string { return time.Now().UTC().Format(time.RFC3339) }

// jsonIndent1 writes JSON the way Python's json.dump(…, indent=1, sort_keys=True) does
// (maps are sorted by encoding/json; HTML characters are not escaped).
func jsonIndent1(v interface{}) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	enc.Encode(v)
	return bytes.TrimRight(buf.Bytes(), "\n")
}
