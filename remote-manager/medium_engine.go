// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ─── MEDIUM: native comparison engine ───
//
// The native engine computes what the original tool's
//
//	rsync -rlc --delete --itemize-changes --dry-run --exclude=<pattern>... <source>/ <server>:<path>/
//
// would report, from two trees: the prepared source (path, type, size, SHA-256, link target)
// and the destination read through WRM's own SSH connection (SFTP listing, SHA-256 of the
// files whose sizes match computed on the server). It is read-only: nothing is written.
//
// rsync's rules reproduced here (checked against real rsync in medium_native_test.go):
//   - excluded paths (and everything under an excluded directory) are skipped on both sides;
//   - a new item is "new" (a directory as "dir/"), a file with different content and a
//     symlink with a different target are "changed"; permissions and times do not count;
//   - an item whose type differs on the server is "new" (rsync replaces it); a directory
//     replaced by a file is not listed itself, only its contents as deleted;
//   - server items not in the source are "deleted" (a directory as "dir/", a symlink like a
//     file), except a directory that still holds excluded items, which rsync keeps.

const (
	mdMaxEntries   = 200000
	mdWalkParallel = 8
	mdHashBatch    = 1500
	mdStateMax     = 1 << 20
	mdHistoryTail  = 256 << 10
)

type mdEntry struct {
	Type string `json:"type"` // file | dir | symlink | other
	Size int64  `json:"size,omitempty"`
	Hash string `json:"sha256,omitempty"`
	Link string `json:"link,omitempty"`

	perm os.FileMode            // destination: permission bits with setuid, setgid and sticky
	exec bool                   // source: executable (Git mode 100755)
	open func() ([]byte, error) // source: the file's content (deployments)
}

// mdTree maps a path relative to the root (no leading or trailing slash) to its entry.
type mdTree map[string]*mdEntry

type mdChange struct {
	Path       string `json:"path"`   // directories end with "/" (as rsync prints them)
	Action     string `json:"action"` // new | changed | deleted
	Kind       string `json:"kind"`   // file | dir | symlink | other (a deleted symlink is "file", like rsync)
	Link       string `json:"link_target,omitempty"`
	ServerKind string `json:"server_kind,omitempty"` // what the server has there when it is replaced or deleted
	EOL        bool   `json:"eol_only,omitempty"`    // changed only in line endings (CRLF/LF)
}

func (c mdChange) key() string { return c.Action + " " + c.Path }

// mdRemote is the destination side: an SFTP session (and SSH for hashing) in production,
// the local file system in tests.
type mdRemote interface {
	Stat(p string) (os.FileInfo, error)
	ReadDir(p string) ([]os.FileInfo, error)
	ReadLink(p string) (string, error)
	ReadFile(p string, max int64) ([]byte, error) // nil, nil when the file does not exist
	ReadTail(p string, max int64) ([]byte, error)
	Hashes(root string, rels []string) (map[string]string, error)
}

func mdTypeOf(m os.FileMode) string {
	switch {
	case m.IsDir():
		return "dir"
	case m&os.ModeSymlink != 0:
		return "symlink"
	case m.IsRegular():
		return "file"
	}
	return "other"
}

// mdFilterTree drops the excluded entries of a source tree and everything below an
// excluded directory, the way rsync's sender builds its file list.
func mdFilterTree(t mdTree, rules []*mdRule) mdTree {
	if len(rules) == 0 {
		return t
	}
	out := mdTree{}
	dropped := map[string]bool{}
	paths := make([]string, 0, len(t))
	for p := range t {
		paths = append(paths, p)
	}
	sort.Strings(paths) // a parent sorts before its children
	for _, p := range paths {
		if parent := path.Dir(p); parent != "." && (dropped[parent] || t[parent] == nil && mdAncestorDropped(parent, dropped)) {
			dropped[p] = true
			continue
		}
		e := t[p]
		if mdExcluded(rules, p, e.Type == "dir") {
			dropped[p] = true
			continue
		}
		out[p] = e
	}
	return out
}

func mdAncestorDropped(p string, dropped map[string]bool) bool {
	for p != "." && p != "" {
		if dropped[p] {
			return true
		}
		p = path.Dir(p)
	}
	return false
}

// mdWithDirs adds the directories implied by the paths of a file list (Git trees and
// manifests list files only, or files and directories).
func mdWithDirs(t mdTree) mdTree {
	for p := range t {
		for d := path.Dir(p); d != "." && d != "/" && d != ""; d = path.Dir(d) {
			if _, ok := t[d]; ok {
				break
			}
			t[d] = &mdEntry{Type: "dir"}
		}
	}
	return t
}

// mdWalk lists the destination below root, skipping excluded items. keep holds the
// directories that contain excluded items (rsync does not delete those). src is the
// filtered source: a directory excluded only as a directory ("name/") where the source has
// a file or symlink is still listed, because rsync replaces it and deletes its contents.
func mdWalk(r mdRemote, root string, rules []*mdRule, src mdTree) (tree mdTree, keep map[string]bool, exists bool, err error) {
	tree, keep = mdTree{}, map[string]bool{}
	st, err := r.Stat(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) || strings.Contains(strings.ToLower(err.Error()), "no such file") {
			return tree, keep, false, nil
		}
		return nil, nil, false, err
	}
	if !st.IsDir() {
		return nil, nil, true, fmt.Errorf("%s exists on the server but is not a directory", root)
	}
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	sem := make(chan struct{}, mdWalkParallel)
	var visit func(rel string)
	visit = func(rel string) {
		defer wg.Done()
		sem <- struct{}{}
		dir := root
		if rel != "" {
			dir = root + "/" + rel
		}
		list, e := r.ReadDir(dir)
		<-sem
		mu.Lock()
		defer mu.Unlock()
		if e != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("cannot list %s: %v", dir, e)
			}
			return
		}
		for _, fi := range list {
			name := fi.Name()
			if name == "." || name == ".." || name == "" {
				continue
			}
			p := name
			if rel != "" {
				p = rel + "/" + name
			}
			typ := mdTypeOf(fi.Mode())
			replaced := typ == "dir" && src[p] != nil && src[p].Type != "dir"
			if !replaced && mdExcluded(rules, p, typ == "dir") {
				for d := rel; d != ""; d = mdParent(d) {
					keep[d] = true
				}
				continue
			}
			if len(tree) >= mdMaxEntries {
				if firstErr == nil {
					firstErr = fmt.Errorf("more than %d items under %s", mdMaxEntries, root)
				}
				return
			}
			tree[p] = &mdEntry{Type: typ, Size: fi.Size(), perm: fi.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)}
			if typ == "dir" {
				wg.Add(1)
				go visit(p)
			}
		}
	}
	wg.Add(1)
	go visit("")
	wg.Wait()
	if firstErr != nil {
		return nil, nil, true, firstErr
	}
	// symlink targets
	var links []string
	for p, e := range tree {
		if e.Type == "symlink" {
			links = append(links, p)
		}
	}
	for _, p := range links {
		target, e := r.ReadLink(root + "/" + p)
		if e != nil {
			return nil, nil, true, fmt.Errorf("cannot read the link %s: %v", p, e)
		}
		tree[p].Link = target
	}
	return tree, keep, true, nil
}

func mdParent(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

// mdDiff compares a filtered source tree with a filtered destination tree. dst must have
// hashes for the files whose sizes match the source (mdFillHashes). conflicts are the
// paths where the source has a file or symlink and the server a directory that holds
// excluded items: rsync cannot replace it and fails (exit code 23), and so does the plan.
func mdDiff(src, dst mdTree, keep map[string]bool) (out []mdChange, conflicts []string) {
	for p, s := range src {
		d := dst[p]
		name := p
		if s.Type == "dir" {
			name += "/"
		}
		if d == nil {
			out = append(out, mdChange{Path: name, Action: "new", Kind: s.Type, Link: s.Link})
			continue
		}
		if d.Type != s.Type {
			if d.Type == "dir" && keep[p] {
				conflicts = append(conflicts, p)
			}
			out = append(out, mdChange{Path: name, Action: "new", Kind: s.Type, Link: s.Link, ServerKind: d.Type})
			continue
		}
		switch s.Type {
		case "file":
			if s.Size != d.Size || s.Hash != d.Hash {
				out = append(out, mdChange{Path: name, Action: "changed", Kind: "file"})
			}
		case "symlink":
			if s.Link != d.Link {
				out = append(out, mdChange{Path: name, Action: "changed", Kind: "symlink", Link: s.Link})
			}
		}
	}
	for p, d := range dst {
		if s := src[p]; s != nil {
			continue
		}
		if d.Type == "dir" {
			if keep[p] {
				continue
			}
			out = append(out, mdChange{Path: p + "/", Action: "deleted", Kind: "dir", ServerKind: "dir"})
			continue
		}
		out = append(out, mdChange{Path: p, Action: "deleted", Kind: "file", ServerKind: d.Type})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path || out[i].Path == out[j].Path && out[i].Action < out[j].Action
	})
	sort.Strings(conflicts)
	return out, conflicts
}

// mdFillHashes hashes on the server the destination files whose size matches the source
// (a different size is a change anyway, as with rsync -c).
func mdFillHashes(r mdRemote, root string, src, dst mdTree) error {
	var rels []string
	for p, d := range dst {
		if s := src[p]; s != nil && s.Type == "file" && d.Type == "file" && s.Size == d.Size {
			rels = append(rels, p)
		}
	}
	sort.Strings(rels)
	for i := 0; i < len(rels); i += mdHashBatch {
		end := i + mdHashBatch
		if end > len(rels) {
			end = len(rels)
		}
		got, err := r.Hashes(root, rels[i:end])
		if err != nil {
			return err
		}
		for _, p := range rels[i:end] {
			h, ok := got[p]
			if !ok {
				return fmt.Errorf("the server did not report a checksum for %s", p)
			}
			dst[p].Hash = h
		}
	}
	return nil
}

// ─── local destination (tests, and a source directory) ───

type mdLocal struct{}

func (mdLocal) Stat(p string) (os.FileInfo, error) { return os.Stat(p) }
func (mdLocal) ReadDir(p string) ([]os.FileInfo, error) {
	list, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := make([]os.FileInfo, 0, len(list))
	for _, e := range list {
		fi, err := e.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, fi)
	}
	return out, nil
}
func (mdLocal) ReadLink(p string) (string, error) { return os.Readlink(p) }
func (mdLocal) ReadFile(p string, max int64) ([]byte, error) {
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}
func (mdLocal) ReadTail(p string, max int64) ([]byte, error) {
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > max {
		f.Seek(st.Size()-max, io.SeekStart)
	}
	return io.ReadAll(io.LimitReader(f, max))
}
func (mdLocal) Hashes(root string, rels []string) (map[string]string, error) {
	out := map[string]string{}
	for _, rel := range rels {
		h, _, err := mdHashFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		out[rel] = h
	}
	return out, nil
}

func mdHashFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// mdLocalTree reads a local directory as a source tree (tests).
func mdLocalTree(root string) (mdTree, error) {
	t := mdTree{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		e := &mdEntry{Type: mdTypeOf(fi.Mode())}
		switch e.Type {
		case "file":
			if e.Hash, e.Size, err = mdHashFile(p); err != nil {
				return err
			}
			file := filepath.Clean(p)
			e.exec, e.open = fi.Mode()&0o100 != 0, func() ([]byte, error) { return os.ReadFile(file) }
		case "symlink":
			if e.Link, err = os.Readlink(p); err != nil {
				return err
			}
		}
		t[rel] = e
		return nil
	})
	return t, err
}

// ─── SSH destination ───

type mdSFTP struct {
	cl  *sftp.Client
	ssh *ssh.Client
}

func (m mdSFTP) Stat(p string) (os.FileInfo, error)      { return m.cl.Stat(p) }
func (m mdSFTP) ReadDir(p string) ([]os.FileInfo, error) { return m.cl.ReadDir(p) }
func (m mdSFTP) ReadLink(p string) (string, error)       { return m.cl.ReadLink(p) }
func (m mdSFTP) ReadFile(p string, max int64) ([]byte, error) {
	f, err := m.cl.Open(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}
func (m mdSFTP) ReadTail(p string, max int64) ([]byte, error) {
	f, err := m.cl.Open(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > max {
		if _, err := f.Seek(st.Size()-max, io.SeekStart); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(f, max))
}

// Hashes runs sha256sum on the server for a batch of files (names as NUL-separated
// stdin) and falls back to reading the files over SFTP when the command is missing.
func (m mdSFTP) Hashes(root string, rels []string) (map[string]string, error) {
	var in bytes.Buffer
	for _, r := range rels {
		in.WriteString(r)
		in.WriteByte(0)
	}
	script := "cd " + shellQuote(root) + " && command -v sha256sum >/dev/null 2>&1 || exit 127; cd " + shellQuote(root) + " && xargs -0 sha256sum --"
	out, err := runRemoteScript(m.ssh, script, in.String(), 10*time.Minute)
	if err == nil {
		got := mdParseSHA256Sum(out)
		if len(got) >= len(rels) {
			return got, nil
		}
	}
	// fallback: read and hash over SFTP
	res := map[string]string{}
	for _, r := range rels {
		f, e := m.cl.Open(root + "/" + r)
		if e != nil {
			return nil, fmt.Errorf("cannot read %s: %v", r, e)
		}
		h := sha256.New()
		_, e = io.Copy(h, f)
		f.Close()
		if e != nil {
			return nil, fmt.Errorf("cannot read %s: %v", r, e)
		}
		res[r] = hex.EncodeToString(h.Sum(nil))
	}
	return res, nil
}

// mdParseSHA256Sum reads GNU sha256sum output; a line that starts with "\" has its name
// escaped (\\ and \n).
func mdParseSHA256Sum(out string) map[string]string {
	res := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		escaped := strings.HasPrefix(line, `\`)
		if escaped {
			line = line[1:]
		}
		if len(line) < 66 || line[64] != ' ' {
			continue
		}
		hash, name := line[:64], line[66:]
		if escaped {
			var sb strings.Builder
			for i := 0; i < len(name); i++ {
				if name[i] == '\\' && i+1 < len(name) {
					i++
					switch name[i] {
					case 'n':
						sb.WriteByte('\n')
					case 'r':
						sb.WriteByte('\r')
					default:
						sb.WriteByte(name[i])
					}
					continue
				}
				sb.WriteByte(name[i])
			}
			name = sb.String()
		}
		res[name] = hash
	}
	return res
}

// ─── server state (current.json, history.jsonl, lock/) ───

type mdState struct {
	Current  map[string]interface{}   `json:"current"`
	History  []map[string]interface{} `json:"history"`
	Lock     map[string]interface{}   `json:"lock"`
	Warnings []string                 `json:"warnings,omitempty"`
}

// mdReadState reads the destination's state files in the original tool's format. A record
// of another application (its "path" differs) is reported, not used.
func mdReadState(r mdRemote, d *mdDestination) (*mdState, error) {
	st := &mdState{History: []map[string]interface{}{}}
	dir := d.statePath()
	owner := func(rec map[string]interface{}) bool {
		p, _ := rec["path"].(string)
		return p == "" || mdNorm(p) == d.appDir()
	}
	b, err := r.ReadFile(dir+"/current.json", mdStateMax)
	if err != nil {
		return nil, fmt.Errorf("current.json: %v", err)
	}
	if b != nil {
		var rec map[string]interface{}
		if json.Unmarshal(bytes.TrimPrefix(bytes.TrimSpace(b), []byte("\xef\xbb\xbf")), &rec) != nil {
			st.Warnings = append(st.Warnings, "current.json is not valid JSON")
		} else if !owner(rec) {
			return nil, fmt.Errorf("the state directory %s on %s belongs to %v, not %s: two applications share their state", dir, d.Server, rec["path"], d.appDir())
		} else {
			st.Current = rec
		}
	}
	tail, err := r.ReadTail(dir+"/history.jsonl", mdHistoryTail)
	if err != nil {
		return nil, fmt.Errorf("history.jsonl: %v", err)
	}
	if tail != nil {
		lines := strings.Split(string(tail), "\n")
		if int64(len(tail)) >= mdHistoryTail && len(lines) > 0 {
			lines = lines[1:] // the first line may be cut
		}
		for i := len(lines) - 1; i >= 0 && len(st.History) < 50; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			var rec map[string]interface{}
			if json.Unmarshal([]byte(line), &rec) != nil || !owner(rec) {
				continue
			}
			for _, k := range []string{"new", "changed", "deleted"} {
				delete(rec, k) // the file lists stay on the server; counts say how many
			}
			st.History = append(st.History, rec)
		}
	}
	if fi, err := r.Stat(dir + "/lock"); err == nil && fi.IsDir() {
		lock := map[string]interface{}{"since": fi.ModTime().UTC().Format(time.RFC3339)}
		if b, _ := r.ReadFile(dir+"/lock/owner.json", 64<<10); b != nil {
			var rec map[string]interface{}
			if json.Unmarshal(bytes.TrimSpace(b), &rec) == nil {
				lock = rec
			}
		}
		st.Lock = lock
	}
	return st, nil
}

// ─── comparison with the original tool's result ───

type mdCompare struct {
	Same          bool       `json:"same"`
	Matched       int        `json:"matched"`
	OnlyReference []mdChange `json:"only_reference"`
	OnlyNative    []mdChange `json:"only_native"`
	KindDiffers   []string   `json:"kind_differs"`
}

func mdCompareChanges(reference, native []mdChange) *mdCompare {
	c := &mdCompare{OnlyReference: []mdChange{}, OnlyNative: []mdChange{}, KindDiffers: []string{}}
	ref := map[string]mdChange{}
	for _, r := range reference {
		ref[r.key()] = r
	}
	seen := map[string]bool{}
	for _, n := range native {
		r, ok := ref[n.key()]
		if !ok {
			c.OnlyNative = append(c.OnlyNative, n)
			continue
		}
		seen[n.key()] = true
		c.Matched++
		if r.Kind != n.Kind || r.Kind == "symlink" && r.Link != n.Link {
			c.KindDiffers = append(c.KindDiffers, n.Path)
		}
	}
	for _, r := range reference {
		if !seen[r.key()] {
			c.OnlyReference = append(c.OnlyReference, r)
		}
	}
	c.Same = len(c.OnlyReference) == 0 && len(c.OnlyNative) == 0 && len(c.KindDiffers) == 0
	return c
}
