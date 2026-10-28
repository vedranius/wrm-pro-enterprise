// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// ─── OFFLINE BUNDLES ─────────────────────────────────
//
// A bundle is a .tar.gz with one top directory <name>/ that holds bundle.json (the manifest:
// per service the target ref, the hash and the history of every file) and payload/<app>/<rel>
// (the target files without protected ones). deploytool.py and README.txt may be present
// and are ignored. WRM imports bundles built elsewhere (when it cannot reach the Git server)
// and exports the same format for servers that only a file-based tool can reach.

const (
	gitMaxBundle   = 200 << 20
	gitMaxManifest = 64 << 20
)

type gitBundleFile struct {
	Hash    string         `json:"hash"`
	Oldest  int            `json:"oldest,omitempty"`
	History []gitHistEntry `json:"history"`
}

type gitBundleApp struct {
	Project     string                   `json:"project"`
	Branch      string                   `json:"branch"`
	RefKind     string                   `json:"ref_kind"`
	Version     string                   `json:"version"`
	LatestTag   *string                  `json:"latest_tag"`
	Commit      string                   `json:"commit"`
	CommitDate  string                   `json:"commit_date"`
	Subdir      string                   `json:"subdir"`
	Kind        string                   `json:"kind"`
	Protected   []string                 `json:"protected"`
	Fingerprint []string                 `json:"fingerprint"`
	InstallHint []string                 `json:"install_hint"`
	Exclude     []string                 `json:"exclude"`
	Files       map[string]gitBundleFile `json:"files"`
	Versions    []gitTargetVersion       `json:"versions,omitempty"`
}

type gitBundleManifest struct {
	BundleID     string                  `json:"bundle_id"`
	Created      string                  `json:"created"`
	CreatedBy    string                  `json:"created_by"`
	Source       string                  `json:"source"`
	ServerRoots  []string                `json:"server_roots"`
	HistoryDepth int                     `json:"history_depth"`
	Groups       []string                `json:"groups"`
	Apps         map[string]gitBundleApp `json:"apps"`
}

func bundleBlobID(hash string) string { return "sha256:" + hash }

// safeBundlePath splits a tar entry name into top directory and the rest, refusing
// absolute paths and "..".
func safeBundlePath(name string) (string, string, bool) {
	name = strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "./")
	if strings.HasPrefix(name, "/") {
		return "", "", false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return "", "", false
		}
	}
	i := strings.Index(name, "/")
	if i < 0 {
		return name, "", true
	}
	return name[:i], strings.Trim(name[i+1:], "/"), true
}

// validBundleRel reports whether a path from bundle.json may become a server path.
// The manifest is written by another tool (or by an attacker) and its keys are used
// verbatim for staging, backup and mv, so absolute paths, parent traversal, control
// characters and non-canonical forms are refused at import.
func validBundleRel(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.ContainsAny(rel, "\x00\n\r\\") {
		return false
	}
	if strings.Trim(rel, "/") != rel {
		return false
	}
	_, err := cleanRelPath(rel)
	return err == nil
}

type bundleImportResult struct {
	Source   gitSource `json:"source"`
	Files    int       `json:"files"`
	Payload  int       `json:"payload"`
	Warnings []string  `json:"warnings"`
}

// importBundle reads a bundle and stores it as a source of the user.
func importBundle(userID int, r io.Reader) (*bundleImportResult, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("not a .tar.gz file")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	res, err := db.Exec(`INSERT INTO git_sources (user_id, kind, name, url, token, info, created_at, last_error) VALUES (?,?,?,?,?,?,?,?)`,
		userID, "bundle", "", "", "", "", nowStamp(), "")
	if err != nil {
		return nil, err
	}
	id64, _ := res.LastInsertId()
	srcID := int(id64)
	ok := false
	defer func() {
		if !ok {
			deleteGitSource(srcID)
		}
	}()
	top := ""
	var manifest *gitBundleManifest
	payload := map[string]string{} // app/rel → hash
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("damaged archive: %v", err)
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != '\x00' { // '\x00': regular file in old tar writers
			continue
		}
		dir, rest, safe := safeBundlePath(h.Name)
		if !safe {
			return nil, fmt.Errorf("unsafe path in the archive: %s", h.Name)
		}
		if rest == "" {
			continue
		}
		if top == "" {
			top = dir
		} else if dir != top {
			return nil, fmt.Errorf("the bundle must have one top directory (found %s and %s)", top, dir)
		}
		switch {
		case rest == "bundle.json":
			data, err := io.ReadAll(io.LimitReader(tr, gitMaxManifest+1))
			if err != nil || len(data) > gitMaxManifest {
				return nil, fmt.Errorf("bundle.json is too large or damaged")
			}
			var m gitBundleManifest
			if err := json.Unmarshal(data, &m); err != nil {
				return nil, fmt.Errorf("bundle.json: %v", err)
			}
			manifest = &m
		case strings.HasPrefix(rest, "payload/"):
			p := strings.TrimPrefix(rest, "payload/")
			if !strings.Contains(p, "/") {
				continue
			}
			if !validBundleRel(p) {
				return nil, fmt.Errorf("unsafe payload path %q", p)
			}
			if h.Size > gitMaxBlobFetch {
				return nil, fmt.Errorf("%s is too large", p)
			}
			data, err := io.ReadAll(io.LimitReader(tr, gitMaxBlobFetch+1))
			if err != nil {
				return nil, fmt.Errorf("damaged archive: %v", err)
			}
			hash := normHash(data)
			if _, have := lookupBlob(srcID, bundleBlobID(hash)); !have {
				storeBlob(srcID, bundleBlobID(hash), data)
			}
			payload[p] = hash
		}
	}
	if manifest == nil {
		return nil, fmt.Errorf("no %s/bundle.json in the archive", strings.TrimSuffix(top+"/", "/"))
	}
	if len(manifest.Apps) == 0 {
		return nil, fmt.Errorf("the bundle has no services")
	}
	if manifest.BundleID != "" && !gitNameRe.MatchString(manifest.BundleID) {
		return nil, fmt.Errorf("invalid bundle id %q", manifest.BundleID)
	}
	out := &bundleImportResult{Warnings: []string{}}
	for name, a := range manifest.Apps {
		if !gitNameRe.MatchString(name) {
			return nil, fmt.Errorf("invalid service name in the bundle: %q", name)
		}
		for rel := range a.Files {
			if !validBundleRel(rel) {
				return nil, fmt.Errorf("bundle %s: unsafe file path %q", name, rel)
			}
		}
		for rel, f := range a.Files {
			out.Files++
			if globHit(rel, a.Protected) {
				continue
			}
			ph, have := payload[name+"/"+rel]
			if !have {
				continue
			}
			out.Payload++
			if ph != f.Hash {
				out.Warnings = append(out.Warnings, fmt.Sprintf("%s/%s: payload does not match its hash in bundle.json", name, rel))
			}
		}
	}
	if len(out.Warnings) > 20 {
		out.Warnings = append(out.Warnings[:20], fmt.Sprintf("… %d more", len(out.Warnings)-20))
	}
	// a newer import of the same bundle replaces the older one
	var olds []int
	if rows, err := db.Query(`SELECT id, info FROM git_sources WHERE user_id=? AND kind='bundle' AND id<>?`, userID, srcID); err == nil {
		for rows.Next() {
			var oid int
			var info string
			rows.Scan(&oid, &info)
			var m gitBundleManifest
			if json.Unmarshal([]byte(info), &m) == nil && m.BundleID == manifest.BundleID && m.BundleID != "" {
				olds = append(olds, oid)
			}
		}
		rows.Close()
	}
	for _, oid := range olds {
		deleteGitSource(oid)
	}
	name := manifest.BundleID
	if name == "" {
		name = top
	}
	db.Exec(`UPDATE git_sources SET name=?, url=?, info=? WHERE id=?`, truncateStr(name, 100), truncateStr(manifest.Source, 300), string(jsonMarshal(manifest)), srcID)
	ok = true
	src, _ := loadGitSource(userID, srcID)
	out.Source = src
	return out, nil
}

func deleteGitSource(id int) {
	for _, table := range []string{"git_relay_cache", "git_relay_members", "git_relay_meta"} {
		db.Exec(`DELETE FROM `+table+` WHERE source_id=?`, id)
	}
	db.Exec(`DELETE FROM git_blobs WHERE source_id=?`, id)
	db.Exec(`DELETE FROM git_trees WHERE source_id=?`, id)
	db.Exec(`DELETE FROM git_sources WHERE id=?`, id)
}

func loadBundleManifest(userID, srcID int) (*gitBundleManifest, error) {
	var info string
	if err := db.QueryRow(`SELECT info FROM git_sources WHERE id=? AND user_id=? AND kind='bundle'`, srcID, userID).Scan(&info); err != nil {
		return nil, fmt.Errorf("bundle not found")
	}
	var m gitBundleManifest
	if err := json.Unmarshal([]byte(info), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// bundleTarget turns a bundle's service into a comparison target.
func bundleTarget(src gitSource, m *gitBundleManifest, name string) (gitTarget, bool) {
	a, ok := m.Apps[name]
	if !ok {
		return gitTarget{}, false
	}
	t := gitTarget{App: name, SourceID: src.ID, Source: "bundle:" + m.BundleID, Project: a.Project, Branch: a.Branch, RefKind: a.RefKind,
		Version: a.Version, Commit: a.Commit, CommitDate: a.CommitDate, Subdir: a.Subdir, Kind: a.Kind,
		Protected: nonNil(a.Protected), Fingerprint: nonNil(a.Fingerprint), InstallHint: nonNil(a.InstallHint), Exclude: nonNil(a.Exclude), Include: []string{},
		Files: map[string]gitTargetFile{}, Versions: a.Versions, Warnings: []string{}, ComputedAt: nowStamp()}
	if a.LatestTag != nil {
		t.LatestTag = *a.LatestTag
	}
	t.Ref = a.Branch
	if a.RefKind == "tag" {
		t.Ref = "tag:" + a.Version
	}
	for rel, f := range a.Files {
		if !validBundleRel(rel) {
			continue // defence in depth for manifests stored before the import check
		}
		tf := gitTargetFile{Hash: f.Hash, Oldest: f.Oldest, History: f.History}
		if len(tf.History) == 0 {
			tf.History = []gitHistEntry{{Commit: a.Commit, CommitFull: a.Commit, Version: 0, Date: a.CommitDate, Hash: f.Hash}}
		}
		if !globHit(rel, a.Protected) {
			if b, have := lookupBlob(src.ID, bundleBlobID(f.Hash)); have {
				tf.Blob = bundleBlobID(f.Hash)
				tf.Size = b.Size
				if b.Hash2 != b.Hash {
					tf.Hash2 = b.Hash2
				}
			}
		}
		t.Files[rel] = tf
	}
	return t, true
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// bundleAppFromTarget is the manifest entry of a computed target.
func bundleAppFromTarget(t gitTarget) gitBundleApp {
	a := gitBundleApp{Project: t.Project, Branch: t.Branch, RefKind: t.RefKind, Version: t.Version, Commit: t.Commit, CommitDate: t.CommitDate,
		Subdir: t.Subdir, Kind: t.Kind, Protected: nonNil(t.Protected), Fingerprint: nonNil(t.Fingerprint), InstallHint: nonNil(t.InstallHint),
		Exclude: nonNil(t.Exclude), Files: map[string]gitBundleFile{}, Versions: t.Versions}
	if t.LatestTag != "" {
		lt := t.LatestTag
		a.LatestTag = &lt
	}
	for rel, f := range t.Files {
		h := make([]gitHistEntry, 0, len(f.History))
		for _, e := range f.History {
			e.Hash2 = ""
			h = append(h, e)
		}
		a.Files[rel] = gitBundleFile{Hash: f.Hash, Oldest: f.Oldest, History: h}
	}
	return a
}

// manifestJSON writes bundle.json like the deploy tool (indent 1, sorted keys).
func manifestJSON(m *gitBundleManifest) []byte {
	raw, _ := json.Marshal(m)
	var generic interface{}
	json.Unmarshal(raw, &generic)
	return jsonIndent1(generic)
}

// writeBundle writes the tar.gz of targets (contents come from the blob cache, fetched from
// the source when needed).
func writeBundle(w io.Writer, userID int, cat *gitCatalog, st gitSettings, targets []gitTarget) (string, error) {
	now := time.Now()
	host, _ := os.Hostname()
	if host == "" {
		host = "wrm"
	}
	m := &gitBundleManifest{BundleID: now.Format("20060102-1504"), Created: now.Format("2006-01-02T15:04:05"),
		CreatedBy: usernameOf(userID) + "@" + host, ServerRoots: nonNil(cat.ServerRoots), HistoryDepth: st.HistoryDepth,
		Groups: nonNil(cat.GitLab.Groups), Apps: map[string]gitBundleApp{}}
	contents := map[string][]byte{} // app/rel → content
	providers := map[int]*gitProvider{}
	for _, t := range targets {
		if loadGitRelayMeta(t.SourceID).OwnerID > 0 && !relaySourceAllowed(userID, t.SourceID) {
			return "", fmt.Errorf("shared Git source access has been removed")
		}
		if t.Error != "" {
			return "", fmt.Errorf("%s: %s", t.App, t.Error)
		}
		if m.Source == "" {
			m.Source = t.Source
		}
		m.Apps[t.App] = bundleAppFromTarget(t)
		for rel, f := range t.Files {
			if globHit(rel, t.Protected) {
				continue
			}
			data, ok := []byte(nil), false
			if f.Blob != "" {
				data, ok = blobContent(t.SourceID, f.Blob)
			}
			if !ok {
				src, found := loadGitSource(userID, t.SourceID)
				if !found || src.Kind == "bundle" || f.Blob == "" {
					return "", fmt.Errorf("%s/%s: the content is not available (large file or a bundle without payload)", t.App, rel)
				}
				p := providers[src.ID]
				if p == nil {
					var err error
					if p, err = newGitProvider(src); err != nil {
						return "", err
					}
					providers[src.ID] = p
				}
				var err error
				if data, err = p.blob(t.Project, f.Blob); err != nil {
					return "", fmt.Errorf("%s/%s: %v", t.App, rel, err)
				}
			}
			contents[t.App+"/"+rel] = data
		}
	}
	if len(m.Apps) == 0 {
		return "", errors.New("nothing to export")
	}
	top := "wrm-bundle-" + m.BundleID
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	add := func(name string, data []byte, mode int64) error {
		if err := tw.WriteHeader(&tar.Header{Name: top + "/" + name, Mode: mode, Size: int64(len(data)), ModTime: now, Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}
	tw.WriteHeader(&tar.Header{Name: top + "/", Mode: 0o755, ModTime: now, Typeflag: tar.TypeDir})
	if err := add("bundle.json", manifestJSON(m), 0o644); err != nil {
		return "", err
	}
	keys := make([]string, 0, len(contents))
	for k := range contents {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := add("payload/"+path.Clean(k), contents[k], 0o644); err != nil {
			return "", err
		}
	}
	if err := tw.Close(); err != nil {
		return "", err
	}
	return top + ".tar.gz", gz.Close()
}
