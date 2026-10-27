// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
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
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ─── Medium screens served by WRM: file details (diff), line-ending marking and the
// deploy-config editor. The behaviour follows the original tool; the code is WRM's own. ───

const (
	mdDiffMax     = 512 << 10 // bytes read per side for the details view
	mdDiffContext = 3         // unchanged lines shown around a change
	mdEOLMax      = 8 << 20   // files above this size are not checked for line endings
	mdEOLFiles    = 2000      // at most this many changed files are checked per server
	mdConfigMax   = 1 << 20   // one deploy-config file
)

// ─── line endings (eol_only) ───

// mdHashNoCR is the sha256 of the content after removing every "\r" (the same as
// `tr -d '\r' < file | sha256sum` on the server).
func mdHashNoCR(b []byte) string {
	h := sha256.Sum256(bytes.ReplaceAll(b, []byte{'\r'}, nil))
	return hex.EncodeToString(h[:])
}

// mdCRHasher is a destination that hashes files without "\r" itself (one SSH command).
type mdCRHasher interface {
	HashesNoCR(root string, rels []string) (map[string]string, error)
}

// HashesNoCR reports, per file, the sha256 of its content without "\r"; a file it cannot
// read is left out. Without sha256sum on the server it returns an error and the caller
// reads the files over SFTP instead.
func (m mdSFTP) HashesNoCR(root string, rels []string) (map[string]string, error) {
	var in bytes.Buffer
	for _, r := range rels {
		in.WriteString(r)
		in.WriteByte(0)
	}
	inner := `for f do if [ -f "$f" ] && [ ! -L "$f" ]; then h=$(tr -d "\r" < "$f" | sha256sum) && printf "%s\0%s\0" "$f" "${h%% *}"; fi; done`
	script := "command -v sha256sum >/dev/null 2>&1 || exit 127; cd " + shellQuote(root) + " && xargs -0 sh -c " + shellQuote(inner) + " sh"
	out, err := runRemoteScript(m.ssh, script, in.String(), 10*time.Minute)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(out, "\x00")
	res := map[string]string{}
	for i := 0; i+1 < len(parts); i += 2 {
		if len(parts[i+1]) == 64 {
			res[parts[i]] = parts[i+1]
		}
	}
	return res, nil
}

// mdMarkEOL marks the changed files whose content differs from the source only in line
// endings (CRLF on the server, LF in Git, or the other way round). The plan is not changed:
// a deployment still sends those files, so the difference disappears afterwards.
func mdMarkEOL(r mdRemote, root string, src mdTree, changes []mdChange) (int, error) {
	local := map[string]string{}
	var rels []string
	for _, c := range changes {
		if len(rels) >= mdEOLFiles {
			break
		}
		e := src[c.Path]
		if c.Action != "changed" || c.Kind != "file" || e == nil || e.Type != "file" || e.open == nil || e.Size > mdEOLMax {
			continue
		}
		b, err := e.open()
		if err != nil || bytes.IndexByte(b, 0) >= 0 {
			continue
		}
		local[c.Path] = mdHashNoCR(b)
		rels = append(rels, c.Path)
	}
	if len(rels) == 0 {
		return 0, nil
	}
	sort.Strings(rels)
	var remote map[string]string
	if h, ok := r.(mdCRHasher); ok {
		remote, _ = h.HashesNoCR(root, rels)
	}
	if remote == nil {
		remote = map[string]string{}
		for _, rel := range rels {
			b, err := r.ReadFile(root+"/"+rel, mdEOLMax+1)
			if err != nil {
				return 0, fmt.Errorf("cannot read %s: %v", mdEscapePath(rel), err)
			}
			if b != nil && len(b) <= mdEOLMax && bytes.IndexByte(b, 0) < 0 {
				remote[rel] = mdHashNoCR(b)
			}
		}
	}
	n := 0
	for i := range changes {
		c := &changes[i]
		if h, ok := local[c.Path]; ok && remote[c.Path] == h {
			c.EOL = true
			n++
		}
	}
	return n, nil
}

// mdHistoryEOL counts, like the original tool, how many of the sent changed files differed
// only in line endings in the reviewed plan.
func mdHistoryEOL(e *mdHistoryEntry, plan *mdServerPlan, changed []string) {
	if plan == nil || e.Counts == nil {
		return
	}
	marked := map[string]bool{}
	for _, c := range plan.Changes {
		if c.EOL {
			marked[c.Path] = true
		}
	}
	n := 0
	for _, p := range changed {
		if marked[mdEscapePath(p)] {
			n++
		}
	}
	if n > 0 {
		e.Counts["eol"] = n
	}
}

// ─── file details: what the deployment changes in one file ───

type mdDiffSide struct {
	State string `json:"state"` // text | binary | too_large | missing | symlink | dir
	Size  int64  `json:"size"`
	Link  string `json:"link,omitempty"`
	CRLF  bool   `json:"crlf,omitempty"`
	text  string
}

type mdDiffView struct {
	Server    string     `json:"server"`
	Path      string     `json:"path"`
	Action    string     `json:"action"`
	EOL       bool       `json:"eol_only"`
	Old       mdDiffSide `json:"server_side"`
	New       mdDiffSide `json:"source_side"`
	Lines     []diffLine `json:"lines"`
	Truncated bool       `json:"truncated,omitempty"`
	Added     int        `json:"added"`
	Removed   int        `json:"removed"`
}

func mdDiffSideOf(b []byte, size int64) mdDiffSide {
	switch {
	case b == nil:
		return mdDiffSide{State: "missing"}
	case size > mdDiffMax || len(b) > mdDiffMax:
		return mdDiffSide{State: "too_large", Size: size}
	case bytes.IndexByte(b, 0) >= 0:
		return mdDiffSide{State: "binary", Size: size}
	}
	s := string(b)
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	return mdDiffSide{State: "text", Size: size, CRLF: strings.Contains(s, "\r\n"), text: s}
}

// mdFileDiff compares the server's file (old) with the source's (new) like the Git
// workspace's file comparison (line endings are compared separately).
func mdFileDiff(oldB []byte, oldSize int64, newB []byte, newSize int64) (lines []diffLine, o, n mdDiffSide, truncated bool, add, rem int) {
	o, n = mdDiffSideOf(oldB, oldSize), mdDiffSideOf(newB, newSize)
	lines = []diffLine{}
	if (o.State != "text" && o.State != "missing") || (n.State != "text" && n.State != "missing") {
		return
	}
	lines, truncated = unifiedDiff(splitLines(o.text), splitLines(n.text), mdDiffContext)
	for _, l := range lines {
		switch l.Op {
		case "+":
			add++
		case "-":
			rem++
		}
	}
	return
}

// mdAPIDiff: GET /plan/<run>/diff?server=…&path=… – one changed file of a finished plan,
// the server's version against the prepared source.
func mdAPIDiff(w http.ResponseWriter, r *http.Request, uid int, runID string) {
	run, ok := mdGetRun(uid, runID)
	if !ok || !run.Done || run.Result == nil {
		jsonError(w, "not found", 404)
		return
	}
	server, p := r.URL.Query().Get("server"), r.URL.Query().Get("path")
	var sp *mdServerPlan
	for i := range run.Result.Servers {
		if run.Result.Servers[i].Server == server {
			sp = &run.Result.Servers[i]
		}
	}
	if sp == nil {
		jsonError(w, "unknown server", 400)
		return
	}
	var ch *mdChange
	for i := range sp.Changes {
		if sp.Changes[i].Path == p {
			ch = &sp.Changes[i]
		}
	}
	if ch == nil || strings.HasSuffix(p, "/") {
		jsonError(w, "this file is not a change of the plan", 400)
		return
	}
	// the path and server come from the plan, not from the request
	rel := mdUnescapePath(ch.Path)
	if !mdSafeTreePath(rel) {
		jsonError(w, "invalid path", 400)
		return
	}
	server = sp.Server
	view := mdDiffView{Server: server, Path: ch.Path, Action: ch.Action, EOL: ch.EOL}
	var newB []byte
	var newSize int64
	if e := run.src[rel]; e != nil && ch.Action != "deleted" {
		switch e.Type {
		case "file":
			newSize = e.Size
			if e.Size <= mdDiffMax && e.open != nil {
				b, err := e.open()
				if err != nil {
					jsonError(w, "cannot read the source file: "+err.Error(), 502)
					return
				}
				newB = b
			} else {
				newB = []byte{}
			}
		case "symlink":
			view.New = mdDiffSide{State: "symlink", Link: e.Link}
		default:
			view.New = mdDiffSide{State: e.Type}
		}
	}
	cfg, err := mdLoadConfig(mdConfigDir(uid))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	var oldB []byte
	var oldSize int64
	err = mdOpenDestination(uid, cfg, server, func(rm mdRemote, _ string) error {
		full := strings.TrimSuffix(sp.Path, "/") + "/" + rel
		fi, err := rm.Stat(full)
		if mdIsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if lfi, e := rm.(interface {
			Lstat(string) (os.FileInfo, error)
		}); e {
			if l, err := lfi.Lstat(full); err == nil && l.Mode()&os.ModeSymlink != 0 {
				t, _ := rm.ReadLink(full)
				view.Old = mdDiffSide{State: "symlink", Link: t}
				return nil
			}
		}
		if fi.IsDir() {
			view.Old = mdDiffSide{State: "dir"}
			return nil
		}
		oldSize = fi.Size()
		if oldSize > mdDiffMax {
			oldB = []byte{}
			return nil
		}
		oldB, err = rm.ReadFile(full, mdDiffMax+1)
		if oldB == nil && err == nil {
			oldB = []byte{}
		}
		return err
	})
	if err != nil {
		jsonError(w, "cannot read the server's file: "+err.Error(), 502)
		return
	}
	lines, o, n, truncated, add, rem := mdFileDiff(oldB, oldSize, newB, newSize)
	if view.Old.State == "" {
		view.Old = o
	}
	if view.New.State == "" {
		view.New = n
	}
	view.Lines, view.Truncated, view.Added, view.Removed = lines, truncated, add, rem
	auditLog(r, uid, "git.medium_native_diff", run.Result.Project, map[string]interface{}{"server": server, "path": ch.Path})
	jsonOK(w, view)
}

// ─── deploy-config editor ───

var mdConfigEditMu sync.Mutex

var mdConfigNameRe = regexp.MustCompile(`^(servers\.yaml|environments\.yaml|projects/[A-Za-z0-9][A-Za-z0-9._-]{0,99}\.ya?ml)$`)

type mdConfigFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Hash string `json:"hash"`
}

// The editor reaches the configuration directory through os.Root: a link can never lead
// a read or a write outside it.

// mdConfigRead reads one deploy-config file: nil without error when it is missing; a link,
// a directory or a file above the limit is refused.
func mdConfigRead(root *os.Root, rel string) ([]byte, error) {
	fi, err := root.Lstat(rel)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", rel)
	}
	if fi.Size() > mdConfigMax {
		return nil, fmt.Errorf("%s is larger than 1 MiB", rel)
	}
	return root.ReadFile(rel)
}

func mdHashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func mdConfigFiles(root *os.Root) []mdConfigFile {
	out := []mdConfigFile{}
	add := func(rel string) {
		if b, err := mdConfigRead(root, rel); err == nil && b != nil {
			out = append(out, mdConfigFile{Name: rel, Size: int64(len(b)), Hash: mdHashBytes(b)})
		}
	}
	add("servers.yaml")
	add("environments.yaml")
	if d, err := root.Open(mdProjectsDir); err == nil {
		names, _ := d.Readdirnames(-1)
		d.Close()
		sort.Strings(names)
		for _, n := range names {
			if rel := mdProjectsDir + "/" + n; mdConfigNameRe.MatchString(rel) {
				add(rel)
			}
		}
	}
	return out
}

// mdCheckConfigEdit loads the configuration as it would be with text in rel (nil: rel
// removed), in a temporary copy: an edit is saved only when the whole config is valid.
func mdCheckConfigEdit(root *os.Root, rel string, text []byte) (*mdConfig, error) {
	tmp, err := os.MkdirTemp("", "wrm-md-config-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Join(tmp, mdProjectsDir), 0o700); err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, f := range mdConfigFiles(root) {
		if f.Name != rel {
			b, err := mdConfigRead(root, f.Name)
			if err != nil {
				return nil, err
			}
			files[f.Name] = b
		}
	}
	if text != nil {
		files[rel] = text
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(name)), b, 0o600); err != nil {
			return nil, err
		}
	}
	cfg, err := mdLoadConfig(tmp)
	if err != nil {
		return nil, fmt.Errorf("%s", strings.ReplaceAll(err.Error(), tmp, "deploy-config"))
	}
	return cfg, nil
}

// mdWriteConfigFile replaces the file through a temporary file in the same directory.
func mdWriteConfigFile(root *os.Root, rel string, text []byte) error {
	if err := root.MkdirAll(path.Dir(rel), 0o700); err != nil {
		return err
	}
	if fi, err := root.Lstat(rel); err == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", rel)
	}
	tmp := rel + ".wrm-edit"
	if err := root.WriteFile(tmp, text, 0o600); err != nil {
		return err
	}
	if err := root.Rename(tmp, rel); err != nil {
		root.Remove(tmp)
		return err
	}
	return nil
}

type mdConfigEdit struct {
	Name     string  `json:"name"`
	Text     *string `json:"text"` // nil with Delete
	BaseHash string  `json:"base_hash"`
	Delete   bool    `json:"delete"`
	Check    bool    `json:"check_only"`
}

// mdAPIConfigFiles serves GET/POST /config/files and GET /config/file.
func mdAPIConfigFiles(w http.ResponseWriter, r *http.Request, uid int, action string) {
	dir := mdConfigDir(uid)
	root, err := os.OpenRoot(dir)
	if err != nil {
		if action == "/config/files" && r.Method == "GET" {
			jsonOK(w, map[string]interface{}{"files": []mdConfigFile{}, "valid": false, "can_edit": mdNativeAllowed(uid), "error": "Import a deploy-config ZIP or set the configuration repository first."})
			return
		}
		jsonError(w, "there is no deploy-config yet", 404)
		return
	}
	defer root.Close()
	switch {
	case action == "/config/files" && r.Method == "GET":
		_, err := mdLoadConfig(dir)
		out := map[string]interface{}{"files": mdConfigFiles(root), "valid": err == nil, "can_edit": mdNativeAllowed(uid)}
		if err != nil {
			out["error"] = err.Error()
		}
		jsonOK(w, out)
	case action == "/config/file" && r.Method == "GET":
		name := r.URL.Query().Get("name")
		if !mdConfigNameRe.MatchString(name) {
			jsonError(w, "invalid file name", 400)
			return
		}
		b, err := mdConfigRead(root, name)
		if err != nil || b == nil {
			jsonError(w, "not found", 404)
			return
		}
		jsonOK(w, map[string]interface{}{"name": name, "text": string(b), "hash": mdHashBytes(b), "crlf": bytes.Contains(b, []byte("\r\n"))})
	case action == "/config/files" && r.Method == "POST":
		if !mdNativeAllowed(uid) {
			jsonError(w, "editing deploy-config in WRM is disabled by policy (medium_native)", 403)
			return
		}
		var in mdConfigEdit
		if json.NewDecoder(io.LimitReader(r.Body, mdConfigMax+64<<10)).Decode(&in) != nil || !mdConfigNameRe.MatchString(in.Name) {
			jsonError(w, "invalid request", 400)
			return
		}
		if in.Delete && !strings.HasPrefix(in.Name, mdProjectsDir+"/") {
			jsonError(w, "only project files can be deleted", 400)
			return
		}
		if !in.Delete && (in.Text == nil || len(*in.Text) > mdConfigMax) {
			jsonError(w, "the file is missing or larger than 1 MiB", 400)
			return
		}
		mdConfigEditMu.Lock()
		defer mdConfigEditMu.Unlock()
		cur, err := mdConfigRead(root, in.Name)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		curHash := ""
		if cur != nil {
			curHash = mdHashBytes(cur)
		}
		if in.BaseHash != curHash {
			msg := "the file changed since you opened it; reload it and apply your edit again"
			if in.BaseHash == "" {
				msg = "the file already exists; open it instead"
			}
			jsonError(w, msg, 409)
			return
		}
		var text []byte
		if !in.Delete {
			text = []byte(*in.Text)
			// a browser text area sends LF; a file that used CRLF keeps it
			if bytes.Contains(cur, []byte("\r\n")) {
				text = bytes.ReplaceAll(bytes.ReplaceAll(text, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))
			}
			if bytes.IndexByte(text, 0) >= 0 || !utf8.Valid(text) {
				jsonError(w, "the file must be UTF-8 text", 400)
				return
			}
		}
		cfg, err := mdCheckConfigEdit(root, in.Name, text)
		if err != nil {
			jsonOK(w, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		summary := map[string]int{"servers": len(cfg.Servers), "environments": len(cfg.Envs), "projects": len(cfg.Projects)}
		if in.Check {
			jsonOK(w, map[string]interface{}{"ok": true, "checked": true, "summary": summary})
			return
		}
		if in.Delete {
			err = root.Remove(in.Name)
		} else {
			err = mdWriteConfigFile(root, in.Name, text)
		}
		if err != nil {
			jsonError(w, "cannot save: "+err.Error(), 500)
			return
		}
		act := "saved"
		switch {
		case in.Delete:
			act = "deleted"
		case cur == nil:
			act = "created"
		}
		auditLog(r, uid, "git.medium_config_edit", in.Name, map[string]interface{}{"action": act, "bytes": len(text)})
		mediumRecord(uid, "config_edit", "ok", 0, in.Name+" "+act, "", "")
		jsonOK(w, map[string]interface{}{"ok": true, "hash": mdHashBytes(text), "summary": summary, "action": act})
	default:
		jsonError(w, "not found", 404)
	}
}
