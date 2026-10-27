// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── EVERY FILE OF AN INSTALLATION ───────────────────
//
// The comparison of a check keeps only the files the catalog tracks (plus extra files the
// include / exclude lists allow). The file list of an installation shows everything: every
// file in the server folder (one find over the folder, ignore_dirs honoured) and every file
// of the repository at the target ref below the service's subdirectory, each with the
// reason for its place (tracked, excluded by a pattern, not in the include list, protected,
// a log file, …). Only tracked and protected files are "in the check" and get a state from
// a comparison; every other file is "outside" the check: it is listed with where it is (in
// Git, on the server or both) but never compared, because it never changes the
// installation's state. Its difference can still be opened on request. Everything here is
// read only on the server; paths are quoted and symlinks are never followed out of the
// installation.

const (
	gitListCap     = 10000    // entries of one folder listing
	gitListHashMax = 64 << 20 // larger files are listed without a hash
	gitMaxViewFile = gitMaxDiffFile - 64<<10
)

type gitListEntry struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"` // file | symlink | dir
	Size     int64  `json:"size,omitempty"`
	MTime    string `json:"mtime,omitempty"`
	Link     string `json:"link,omitempty"`    // symlink target as stored in the link
	Outside  bool   `json:"outside,omitempty"` // the symlink points out of the installation
	OnServer bool   `json:"on_server"`
	InGit    bool   `json:"in_git"`
	Tracked  bool   `json:"tracked"`
	// in the check: ok | old | modified | missing | unknown (tracked) · protected
	// outside the check: outside · symlink · unreadable
	State   string `json:"state"`
	Reason  string `json:"reason"` // tracked | excluded | not_included | protected | extra | tool | compiled | logs | not_tracked | no_target
	Pattern string `json:"pattern,omitempty"`
	Behind  int    `json:"behind,omitempty"`
	Tag     string `json:"tag,omitempty"`
	// what can be done with the file
	Diffable bool `json:"diffable"`
	Viewable bool `json:"viewable"`
}

type gitListing struct {
	InstallID   int            `json:"install_id"`
	Path        string         `json:"path"`
	Target      string         `json:"target"`
	TargetError string         `json:"target_error,omitempty"`
	GitPartial  bool           `json:"git_partial"` // only the catalog's files of the repository are known
	Truncated   bool           `json:"truncated"`
	ListedAt    string         `json:"listed_at"`
	Counts      map[string]int `json:"counts"`
	Files       []gitListEntry `json:"files"`
}

// ─── the repository at the target ref ────────────────

// gitRepoView is the repository side of an installation: every file below the service's
// subdirectory at the target commit (relative path → blob ID).
type gitRepoView struct {
	userID  int
	t       gitTarget
	files   map[string]string
	partial bool
	src     gitSource
	p       *gitProvider
}

func openRepoView(userID int, t gitTarget) *gitRepoView {
	v := &gitRepoView{userID: userID, t: t, files: map[string]string{}, partial: true}
	for rel, f := range t.Files {
		v.files[rel] = f.Blob
	}
	src, found := loadGitSource(userID, t.SourceID)
	if !found || src.Kind == "bundle" {
		return v
	}
	v.src = src
	p, err := newGitProvider(src)
	if err != nil {
		return v
	}
	v.p = p
	if t.CommitFull == "" {
		return v
	}
	tree, err := p.tree(src.ID, t.Project, t.CommitFull)
	if err != nil {
		return v
	}
	prefix := ""
	if t.Subdir != "" {
		prefix = t.Subdir + "/"
	}
	for p, blob := range tree {
		if !strings.HasPrefix(p, prefix) || len(p) == len(prefix) {
			continue
		}
		if rel := p[len(prefix):]; v.files[rel] == "" {
			v.files[rel] = blob
		}
	}
	v.partial = false
	return v
}

// content returns a repository file's content (blob cache, else the Git API).
func (v *gitRepoView) content(rel string) ([]byte, error) {
	if loadGitRelayMeta(v.t.SourceID).OwnerID > 0 && !relaySourceAllowed(v.userID, v.t.SourceID) {
		return nil, fmt.Errorf("shared Git source access has been removed")
	}
	blob := v.files[rel]
	if blob == "" {
		return nil, fmt.Errorf("the content of this file is not available")
	}
	if data, have := blobContent(v.t.SourceID, blob); have {
		return data, nil
	}
	if v.p == nil {
		return nil, fmt.Errorf("the content of this file is not available (a bundle, or a large file)")
	}
	data, err := v.p.blob(v.t.Project, blob)
	if err != nil {
		return nil, err
	}
	storeBlob(v.src.ID, blob, data)
	return data, nil
}

// ─── reasons ─────────────────────────────────────────

func firstGlob(rel string, globs []string, dirs bool) string {
	for _, g := range globs {
		if (dirs && globHitDir(rel, []string{g})) || (!dirs && globHit(rel, []string{g})) {
			return g
		}
	}
	return ""
}

// catalogReason says why a file is (not) compared by a check, with the matching pattern.
func catalogReason(rel string, t gitTarget, haveTarget, inGit bool) (string, string) {
	if !haveTarget {
		return "no_target", ""
	}
	if g := firstGlob(rel, t.Protected, false); g != "" {
		return "protected", g
	}
	if _, ok := t.Files[rel]; ok {
		return "tracked", ""
	}
	if toolFiles[rel] {
		return "tool", ""
	}
	if strings.HasSuffix(rel, ".pyc") {
		return "compiled", ""
	}
	if isLogPath(rel) {
		return "logs", ""
	}
	if len(t.Include) > 0 && !globHitDir(rel, t.Include) {
		return "not_included", ""
	}
	if g := firstGlob(rel, t.Exclude, true); g != "" {
		return "excluded", g
	}
	if !inGit {
		return "extra", ""
	}
	return "not_tracked", ""
}

// isLogPath tells whether a path is a log file: named *.log, *.log.1, *.log.gz, …, or below
// a folder named log or logs (except source and configuration files there, so a code package
// called log stays code). Log files are never part of a check or a target unless the
// catalog's include list names them.
func isLogPath(rel string) bool {
	parts := strings.Split(strings.ToLower(rel), "/")
	base := parts[len(parts)-1]
	if strings.HasSuffix(base, ".log") || strings.Contains(base, ".log.") {
		return true
	}
	if logSourceExts[path.Ext(base)] {
		return false
	}
	for _, d := range parts[:len(parts)-1] {
		if d == "log" || d == "logs" {
			return true
		}
	}
	return false
}

var logSourceExts = map[string]bool{".py": true, ".go": true, ".js": true, ".mjs": true, ".ts": true, ".sh": true, ".java": true, ".rb": true,
	".php": true, ".pl": true, ".c": true, ".h": true, ".cpp": true, ".rs": true, ".ini": true, ".conf": true, ".cfg": true, ".yml": true,
	".yaml": true, ".json": true, ".toml": true, ".xml": true, ".md": true}

// ─── listing on the server ───────────────────────────

const gitStatPrelude = `if stat -c '%s %Y' / >/dev/null 2>&1; then wst() { stat -c '%s %Y' "$1" 2>/dev/null; }
elif stat -f '%z %m' / >/dev/null 2>&1; then wst() { stat -f '%z %m' "$1" 2>/dev/null; }
else wst() { z=$(wc -c < "$1" 2>/dev/null); printf '%s 0\n' "${z##* }"; }; fi
`

type gitServerEntry struct {
	kind       string // f | r (unreadable) | l | d (unreadable directory)
	hash       string
	size       int64
	mtime      int64
	link       string
	unreadable bool
}

// listOn lists every file below an installation directory (relative path → entry).
func listOn(cl *ssh.Client, dir string, ignore []string) (map[string]*gitServerEntry, bool, bool, error) {
	names := append([]string(nil), gitPruneNames...)
	for _, g := range ignore {
		if !strings.Contains(g, "/") {
			names = append(names, g)
		}
	}
	var sb strings.Builder
	sb.WriteString(gitHashPrelude)
	sb.WriteString(gitStatPrelude)
	fmt.Fprintf(&sb, "cd %s 2>/dev/null || { echo X; echo WRM_DONE; exit 0; }\n", shellQuote(dir))
	fmt.Fprintf(&sb, "find . -type d %s -prune -o -print 2>/dev/null | head -n %d | { n=0; while IFS= read -r f; do\n", findNames(names), gitListCap+1)
	sb.WriteString(`  [ "$f" = . ] && continue
  n=$((n+1)); rel=${f#./}
  if [ -L "$f" ]; then printf 'L\t%s\n' "$rel"; t=$(readlink "$f" 2>/dev/null || ls -ld "$f" | sed 's/^.* -> //'); printf 'T\t%s\n' "$t"
  elif [ -d "$f" ]; then { [ -r "$f" ] && [ -x "$f" ]; } || printf 'D\t%s\n' "$rel"
  elif [ -f "$f" ]; then set -- $(wst "$f"); z=${1:-0}; m=${2:-0}
    if [ ! -r "$f" ]; then printf 'R\t%s\t%s\t%s\n' "$z" "$m" "$rel"
    elif [ "$z" -gt ` + strconv.Itoa(gitListHashMax) + ` ]; then printf 'F\tbig\t%s\t%s\t%s\n' "$z" "$m" "$rel"
    else h=$(tr -d '\r' < "$f" | $S | cut -c1-64); printf 'F\t%s\t%s\t%s\t%s\n' "$h" "$z" "$m" "$rel"; fi
  fi
done; printf 'N\t%s\n' "$n"; }
echo WRM_DONE
`)
	out, err := runRemoteScript(cl, sb.String(), "", gitScanTimeout)
	if err != nil {
		return nil, false, false, err
	}
	if !strings.Contains(out, "WRM_DONE") {
		return nil, false, false, fmt.Errorf("the listing did not finish (too many files?)")
	}
	res := map[string]*gitServerEntry{}
	truncated, missing := false, false
	var last *gitServerEntry
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "\t")
		switch {
		case strings.HasPrefix(line, "E\t"):
			return nil, false, false, fmt.Errorf("%s", line[2:])
		case line == "X":
			missing = true
		case parts[0] == "F" && len(parts) >= 5:
			z, _ := strconv.ParseInt(parts[2], 10, 64)
			m, _ := strconv.ParseInt(parts[3], 10, 64)
			res[strings.Join(parts[4:], "\t")] = &gitServerEntry{kind: "f", hash: parts[1], size: z, mtime: m}
		case parts[0] == "R" && len(parts) >= 4:
			z, _ := strconv.ParseInt(parts[1], 10, 64)
			m, _ := strconv.ParseInt(parts[2], 10, 64)
			res[strings.Join(parts[3:], "\t")] = &gitServerEntry{kind: "f", size: z, mtime: m, unreadable: true}
		case parts[0] == "L" && len(parts) >= 2:
			last = &gitServerEntry{kind: "l"}
			res[strings.Join(parts[1:], "\t")] = last
		case parts[0] == "T" && last != nil:
			last.link = strings.TrimPrefix(line, "T\t")
			last = nil
		case parts[0] == "D" && len(parts) >= 2:
			res[strings.Join(parts[1:], "\t")] = &gitServerEntry{kind: "d", unreadable: true}
		case parts[0] == "N" && len(parts) == 2:
			n, _ := strconv.Atoi(parts[1])
			truncated = n > gitListCap
		}
	}
	return res, truncated, missing, nil
}

// linkOutside tells whether a symlink at rel (target as stored) points out of the root.
func linkOutside(rel, target string) bool {
	if target == "" || strings.HasPrefix(target, "/") {
		return true
	}
	p := path.Clean(path.Join(path.Dir(rel), target))
	return p == ".." || strings.HasPrefix(p, "../")
}

// listInstallFiles builds the full file list of an installation.
func listInstallFiles(userID int, in gitInstall) (*gitListing, error) {
	c, err := loadConnection(in.ConnID)
	if err != nil || c.UserID != userID {
		return nil, fmt.Errorf("connection not found")
	}
	cat := loadGitCatalog(userID)
	t, haveTarget := loadGitTarget(userID, in.App)
	if haveTarget && (t.Error != "" || len(t.Files) == 0) {
		haveTarget = false
	}
	cl, err := dialSSH(c, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot log in to %s: %v", c.Name, err)
	}
	server, truncated, missing, err := listOn(cl, in.Path, cat.IgnoreDirs)
	cl.Close()
	if err != nil {
		return nil, err
	}
	if missing {
		return nil, fmt.Errorf("the directory does not exist on the server")
	}
	out := &gitListing{InstallID: in.ID, Path: in.Path, Truncated: truncated, ListedAt: nowStamp(), Counts: map[string]int{}, Files: []gitListEntry{}}
	repo := &gitRepoView{files: map[string]string{}, partial: true}
	if haveTarget {
		out.Target = t.Version
		repo = openRepoView(userID, t)
	} else if t.Error != "" {
		out.TargetError = t.Error
	}
	out.GitPartial = repo.partial
	words := backupWords()
	ignored := func(rel string) bool { return ignoredPath(path.Dir(rel), cat.IgnoreDirs, words) }
	add := func(e gitListEntry) {
		out.Counts[e.State]++
		out.Files = append(out.Files, e)
	}
	for rel, se := range server {
		if ignored(rel) {
			continue
		}
		blob, inGit := repo.files[rel]
		f, tracked := t.Files[rel]
		e := gitListEntry{Path: rel, Kind: "file", Size: se.size, OnServer: true, InGit: inGit, Tracked: tracked}
		if se.mtime > 0 {
			e.MTime = time.Unix(se.mtime, 0).UTC().Format(time.RFC3339)
		}
		e.Reason, e.Pattern = catalogReason(rel, t, haveTarget, inGit)
		switch {
		case se.kind == "l":
			e.Kind, e.State, e.Link, e.Outside = "symlink", "symlink", se.link, linkOutside(rel, se.link)
		case se.kind == "d":
			e.Kind, e.State = "dir", "unreadable"
		case se.unreadable:
			e.State = "unreadable"
		case e.Reason == "protected":
			e.State = "protected"
			e.Viewable = true
		case tracked:
			e.State = "unknown"
			if se.hash != "big" {
				e.State, e.Behind, e.Tag, _ = trackedState(f, se.hash)
			}
			e.Viewable, e.Diffable = true, f.Blob != ""
		case !haveTarget:
			e.State = "unknown"
			e.Viewable = true
		default:
			// outside the check: listed, never compared (log files not even on request)
			e.State = "outside"
			e.Viewable, e.Diffable = true, inGit && blob != "" && e.Reason != "logs"
		}
		add(e)
	}
	for rel, blob := range repo.files {
		if _, on := server[rel]; on || ignored(rel) {
			continue
		}
		_, tracked := t.Files[rel]
		e := gitListEntry{Path: rel, Kind: "file", InGit: true, Tracked: tracked, State: "missing"}
		if f, ok := t.Files[rel]; ok {
			e.Size = f.Size
		}
		e.Reason, e.Pattern = catalogReason(rel, t, haveTarget, true)
		if !tracked && e.Reason != "protected" {
			e.State = "outside"
		}
		e.Viewable = blob != "" && e.Reason != "protected"
		e.Diffable = e.Viewable && e.Reason != "logs"
		add(e)
	}
	order := map[string]int{"modified": 0, "old": 1, "missing": 2, "unreadable": 3, "unknown": 4, "symlink": 5, "protected": 6, "ok": 7, "outside": 8}
	sort.Slice(out.Files, func(i, j int) bool {
		a, b := out.Files[i], out.Files[j]
		if a.Tracked != b.Tracked {
			return a.Tracked
		}
		if order[a.State] != order[b.State] {
			return order[a.State] < order[b.State]
		}
		return a.Path < b.Path
	})
	return out, nil
}

// ─── one file on the server ──────────────────────────

type gitServerFile struct {
	Exists  bool
	Link    bool
	NoRead  bool
	NotFile bool
	Outside bool
	TooBig  bool
	Size    int64
	Hash    string // tr -d '\r' | sha256
	Data    []byte
}

// readServerFile reads a file below an installation directory. Symlinks are not followed
// (neither the file nor a directory on the way that leads out of the installation).
func readServerFile(cl *ssh.Client, root, rel string, max int64) (gitServerFile, error) {
	var f gitServerFile
	rel = cleanRel(rel)
	if rel == "" || rel == ".." || strings.HasPrefix(rel, "../") || strings.ContainsAny(rel, "\n\r\x00") {
		return f, fmt.Errorf("invalid path")
	}
	script := gitHashPrelude + fmt.Sprintf(`cd %s 2>/dev/null || { echo WRM_NOFILE; exit 0; }
r=$(pwd -P)
d=$(cd %s 2>/dev/null && pwd -P) || { echo WRM_NOFILE; exit 0; }
if [ "$r" != / ]; then case "$d/" in "$r"/*) ;; *) echo WRM_OUTSIDE; exit 0;; esac; fi
f="$d"/%s
if [ -L "$f" ]; then echo WRM_LINK; exit 0; fi
[ -e "$f" ] || { echo WRM_NOFILE; exit 0; }
[ -f "$f" ] || { echo WRM_NOTFILE; exit 0; }
[ -r "$f" ] || { echo WRM_NOREAD; exit 0; }
s=$(wc -c < "$f"); s=${s##* }
h=$(tr -d '\r' < "$f" | $S | cut -c1-64)
printf 'WRM_META\t%%s\t%%s\n' "$s" "$h"
[ "$s" -le %d ] || { echo WRM_TOOBIG; exit 0; }
echo WRM_FILE
cat "$f"
`, shellQuote(root), shellQuote("./"+path.Dir(rel)), shellQuote(path.Base(rel)), max)
	out, err := runRemoteScript(cl, script, "", time.Minute)
	if err != nil {
		return f, err
	}
	head, rest, _ := strings.Cut(out, "\n")
	switch {
	case strings.HasPrefix(head, "E\t"):
		return f, fmt.Errorf("%s", head[2:])
	case head == "WRM_NOFILE":
		return f, nil
	case head == "WRM_OUTSIDE":
		f.Outside = true
		return f, nil
	case head == "WRM_LINK":
		f.Exists, f.Link = true, true
		return f, nil
	case head == "WRM_NOTFILE":
		f.Exists, f.NotFile = true, true
		return f, nil
	case head == "WRM_NOREAD":
		f.Exists, f.NoRead = true, true
		return f, nil
	case strings.HasPrefix(head, "WRM_META\t"):
	default:
		return f, fmt.Errorf("unexpected answer from the server")
	}
	parts := strings.Split(head, "\t")
	f.Exists = true
	if len(parts) == 3 {
		f.Size, _ = strconv.ParseInt(parts[1], 10, 64)
		f.Hash = parts[2]
	}
	if strings.HasPrefix(rest, "WRM_TOOBIG") {
		f.TooBig = true
		return f, nil
	}
	if !strings.HasPrefix(rest, "WRM_FILE\n") {
		return f, fmt.Errorf("unexpected answer from the server")
	}
	f.Data = []byte(strings.TrimPrefix(rest, "WRM_FILE\n"))
	return f, nil
}

func (f gitServerFile) problem() error {
	switch {
	case f.Outside:
		return fmt.Errorf("the path leads out of the installation (symlink)")
	case f.Link:
		return fmt.Errorf("this is a symlink; symlinks are not followed")
	case f.NotFile:
		return fmt.Errorf("not a regular file")
	case f.NoRead:
		return fmt.Errorf("permission denied")
	}
	return nil
}

func isBinary(b []byte) bool {
	if len(b) > 8000 {
		b = b[:8000]
	}
	return bytes.IndexByte(b, 0) >= 0
}

func dialInstall(userID int, in gitInstall) (*ssh.Client, error) {
	c, err := loadConnection(in.ConnID)
	if err != nil || c.UserID != userID {
		return nil, fmt.Errorf("connection not found")
	}
	cl, err := dialSSH(c, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot log in to %s: %v", c.Name, err)
	}
	return cl, nil
}

// viewInstallFile returns one file for viewing: from the server, or from Git (side "git",
// or when the file is only in the repository).
func viewInstallFile(userID int, in gitInstall, rel, side string) (map[string]interface{}, error) {
	rel = cleanRel(rel)
	if rel == "" || rel == ".." || strings.HasPrefix(rel, "../") || strings.ContainsAny(rel, "\n\r\x00") {
		return nil, fmt.Errorf("invalid path")
	}
	res := map[string]interface{}{"path": rel}
	text := func(data []byte, from string) map[string]interface{} {
		res["side"], res["size"], res["hash"] = from, len(data), trHash(data)
		if isBinary(data) {
			res["binary"] = true
		} else {
			res["text"] = string(data)
			res["crlf"] = bytes.Contains(data, []byte("\r\n"))
		}
		return res
	}
	t, haveTarget := loadGitTarget(userID, in.App)
	fromGit := func() (map[string]interface{}, error) {
		if !haveTarget {
			return nil, fmt.Errorf("no target for this service")
		}
		if globHit(rel, t.Protected) {
			return nil, fmt.Errorf("protected files are not shown from the repository")
		}
		repo := openRepoView(userID, t)
		if _, ok := repo.files[rel]; !ok {
			return nil, fmt.Errorf("this file is not in the repository at the target")
		}
		data, err := repo.content(rel)
		if err != nil {
			return nil, err
		}
		res["target"] = t.Version
		return text(data, "git"), nil
	}
	if side == "git" {
		return fromGit()
	}
	cl, err := dialInstall(userID, in)
	if err != nil {
		return nil, err
	}
	defer cl.Close()
	sf, err := readServerFile(cl, in.Path, rel, gitMaxViewFile)
	if err != nil {
		return nil, err
	}
	if !sf.Exists && !sf.Outside {
		return fromGit()
	}
	if err := sf.problem(); err != nil {
		return nil, err
	}
	if sf.TooBig {
		res["side"], res["size"], res["hash"], res["too_big"] = "server", sf.Size, sf.Hash, true
		return res, nil
	}
	return text(sf.Data, "server"), nil
}

// ─── diff of any file in the repository ──────────────

func installDiff(userID int, in gitInstall, rel string) (map[string]interface{}, error) {
	rel = cleanRel(rel)
	if rel == "" || rel == ".." || strings.HasPrefix(rel, "../") || strings.ContainsAny(rel, "\n\r\x00") {
		return nil, fmt.Errorf("unknown file")
	}
	t, ok := loadGitTarget(userID, in.App)
	if !ok {
		return nil, fmt.Errorf("no target for this service")
	}
	if globHit(rel, t.Protected) {
		return nil, fmt.Errorf("protected files are not compared")
	}
	repo := openRepoView(userID, t)
	if _, inGit := repo.files[rel]; !inGit {
		return nil, fmt.Errorf("this file is not in the repository at the target (open it to view it)")
	}
	target, err := repo.content(rel)
	if err != nil {
		return nil, err
	}
	cl, err := dialInstall(userID, in)
	if err != nil {
		return nil, err
	}
	defer cl.Close()
	sf, err := readServerFile(cl, in.Path, rel, gitMaxViewFile)
	if err != nil {
		return nil, err
	}
	if err := sf.problem(); err != nil {
		return nil, err
	}
	f, tracked := t.Files[rel]
	reason, pattern := catalogReason(rel, t, true, true)
	res := map[string]interface{}{"path": rel, "target": t.Version, "tracked": tracked, "informational": !tracked, "reason": reason, "pattern": pattern,
		"missing_on_server": !sf.Exists}
	same := sf.Exists && sf.Hash == trHash(target)
	switch {
	case !sf.Exists:
		res["state"] = "missing"
	case tracked:
		res["state"], _, _, _ = trackedState(f, sf.Hash)
	case same:
		res["state"] = "same"
	default:
		res["state"] = "differs"
	}
	if sf.TooBig {
		res["too_big"], res["same"], res["lines"] = true, same, []diffLine{}
		return res, nil
	}
	if isBinary(sf.Data) || isBinary(target) {
		res["binary"], res["same"], res["lines"] = true, same, []diffLine{}
		return res, nil
	}
	lines, truncated := unifiedDiff(splitLines(string(sf.Data)), splitLines(string(target)), 3)
	res["lines"], res["truncated"], res["same"] = lines, truncated, same
	res["crlf_server"], res["crlf_target"] = bytes.Contains(sf.Data, []byte("\r\n")), bytes.Contains(target, []byte("\r\n"))
	return res, nil
}

// ─── commits between the server's version and the target ─

type gitCommitInfo struct {
	SHA    string `json:"sha"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Date   string `json:"date"`
}

const gitMaxCommitList = 200

// compareCommits lists the commits in to that are not in from (newest first) and their count.
func (p *gitProvider) compareCommits(project, from, to string) (int, []gitCommitInfo, error) {
	var out []gitCommitInfo
	total := 0
	if p.hub() {
		var v struct {
			TotalCommits int `json:"total_commits"`
			Commits      []struct {
				SHA    string `json:"sha"`
				Commit struct {
					Message string `json:"message"`
					Author  struct {
						Name string `json:"name"`
						Date string `json:"date"`
					} `json:"author"`
				} `json:"commit"`
			} `json:"commits"`
		}
		if _, err := p.getJSON(p.gh(project)+"/compare/"+url.PathEscape(from)+"..."+url.PathEscape(to), &v); err != nil {
			return 0, nil, err
		}
		for _, c := range v.Commits {
			title, _, _ := strings.Cut(c.Commit.Message, "\n")
			out = append(out, gitCommitInfo{SHA: c.SHA, Title: title, Author: c.Commit.Author.Name, Date: c.Commit.Author.Date})
		}
		total = v.TotalCommits
	} else {
		var v struct {
			Commits []struct {
				ID            string `json:"id"`
				Title         string `json:"title"`
				AuthorName    string `json:"author_name"`
				CommittedDate string `json:"committed_date"`
			} `json:"commits"`
		}
		if _, err := p.getJSON(p.gl(project)+"/repository/compare?from="+url.QueryEscape(from)+"&to="+url.QueryEscape(to), &v); err != nil {
			return 0, nil, err
		}
		for _, c := range v.Commits {
			out = append(out, gitCommitInfo{SHA: c.ID, Title: c.Title, Author: c.AuthorName, Date: c.CommittedDate})
		}
	}
	if total < len(out) {
		total = len(out)
	}
	// both APIs list the oldest first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if len(out) > gitMaxCommitList {
		out = out[:gitMaxCommitList]
	}
	return total, out, nil
}

// installCommits lists the commits between the server's code version and the target.
func installCommits(userID int, in gitInstall, allowVMD bool) (map[string]interface{}, error) {
	t, ok := loadGitTarget(userID, in.App)
	if !ok || t.Error != "" || t.Commit == "" {
		return nil, fmt.Errorf("no target for this service yet (refresh the targets)")
	}
	to := t.CommitFull
	if to == "" {
		to = t.Commit
	}
	from, fromKind := "", "commit"
	if allowVMD {
		from = strings.TrimSpace(in.VersionMD.Commit)
		fromKind = "commit"
		if from == "" {
			from, fromKind = strings.TrimSpace(in.VersionMD.Version), "version"
		}
	}
	if from == "" && in.Detected.State == "exact" {
		from = strings.TrimSpace(in.Detected.Commit)
		if from == "" {
			from, fromKind = strings.TrimSpace(in.Detected.Version), "version"
		}
	}
	res := map[string]interface{}{"from": from, "from_kind": fromKind, "to": t.Version, "to_commit": t.Commit, "count": 0, "commits": []gitCommitInfo{}}
	if from == "" {
		return nil, fmt.Errorf("the server code does not match a single version (mixed or modified); compare the files")
	}
	if fromKind == "version" && strings.ContainsAny(from, " \t/") {
		return nil, fmt.Errorf("the server's version is unknown")
	}
	if strings.HasPrefix(to, from) || from == t.Version {
		return res, nil
	}
	src, found := loadGitSource(userID, t.SourceID)
	if !found || src.Kind == "bundle" {
		return nil, fmt.Errorf("the commit list needs a GitLab or GitHub source (the target comes from a bundle)")
	}
	p, err := newGitProvider(src)
	if err != nil {
		return nil, err
	}
	n, list, err := p.compareCommits(t.Project, from, to)
	if err != nil {
		return nil, err
	}
	res["count"], res["commits"] = n, nonNilCommits(list)
	return res, nil
}

func nonNilCommits(c []gitCommitInfo) []gitCommitInfo {
	if c == nil {
		return []gitCommitInfo{}
	}
	return c
}
