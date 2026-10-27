// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── DISCOVERY AND COMPARISON OVER SSH ───────────────
//
// One SSH login per server: a find over the configured roots (limited depth) returns the
// fingerprint files, a second script hashes the files of every installation with POSIX
// tools (tr -d '\r' < f | sha256sum), reads VERSION.md and lists the systemd / supervisor
// units that mention the installation directory. Nothing on the server is changed.

const (
	gitDiscoverDepth   = 3
	gitMaxInstallFiles = 5000
	gitScanTimeout     = 3 * time.Minute
)

// unit file locations searched for units of an installation (a variable for tests)
var (
	gitSystemdDirs    = []string{"/etc/systemd/system", "/lib/systemd/system", "/usr/lib/systemd/system"}
	gitSupervisorDirs = []string{"/etc/supervisor/conf.d", "/etc/supervisord.d", "/etc/supervisor", "/etc"}
)

var gitPruneNames = []string{".git", ".deploy-bak", "node_modules", "__pycache__", ".venv", "venv"}

func findPrune() string {
	parts := make([]string, len(gitPruneNames))
	for i, n := range gitPruneNames {
		parts[i] = "-name " + shellQuote(n)
	}
	return `\( ` + strings.Join(parts, " -o ") + ` \) -prune`
}

func findNames(names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = "-name " + shellQuote(n)
	}
	return `\( ` + strings.Join(parts, " -o ") + ` \)`
}

type gitFound struct {
	App  string
	Path string
	Env  string
	Root string
}

// envLabel derives an environment from the path: a directory between the root and the
// installation (/srv/scripts/test/App → test) or a suffix (App_prod → prod).
func envLabel(root, dir string) string {
	rel := strings.Trim(strings.TrimPrefix(dir, root), "/")
	parts := strings.Split(rel, "/")
	if len(parts) >= 2 && parts[0] != "" {
		return strings.ToLower(parts[len(parts)-2])
	}
	base := parts[len(parts)-1]
	if i := strings.LastIndexAny(base, "_"); i > 0 && i < len(base)-1 {
		return strings.ToLower(base[i+1:])
	}
	return ""
}

func relDepth(root, dir string) int {
	rel := strings.Trim(strings.TrimPrefix(dir, root), "/")
	if rel == "" {
		return 0
	}
	return strings.Count(rel, "/") + 1
}

// discoverOn finds the installations of the catalog's services below the roots.
func discoverOn(cl *ssh.Client, cat *gitCatalog, apps []gitCatalogApp) ([]gitFound, error) {
	if len(cat.ServerRoots) == 0 {
		return nil, fmt.Errorf("no server roots configured (Settings)")
	}
	names, hints := map[string]bool{}, map[string]bool{}
	maxFp := 0
	for _, a := range apps {
		for _, f := range a.Fingerprint {
			names[path.Base(f)] = true
			if d := strings.Count(f, "/") + 1; d > maxFp {
				maxFp = d
			}
		}
		if len(a.Fingerprint) == 0 {
			for _, h := range a.InstallHint {
				hints[h] = true
			}
		}
	}
	if len(names) == 0 && len(hints) == 0 {
		return nil, fmt.Errorf("the catalog has no fingerprint files or install hints")
	}
	var sb strings.Builder
	for _, r := range cat.ServerRoots {
		q := shellQuote(r)
		fmt.Fprintf(&sb, "if [ -d %s ]; then\n", q)
		if len(names) > 0 {
			fmt.Fprintf(&sb, "find %s -maxdepth %d %s -o -type f %s -print 2>/dev/null | sed 's/^/F\t/'\n", q, gitDiscoverDepth+maxFp, findPrune(), findNames(keys(names)))
		}
		if len(hints) > 0 {
			fmt.Fprintf(&sb, "find %s -maxdepth %d %s -o -type d %s -print 2>/dev/null | sed 's/^/D\t/'\n", q, gitDiscoverDepth, findPrune(), findNames(keys(hints)))
		}
		sb.WriteString("fi\n")
	}
	sb.WriteString("echo WRM_DONE\n")
	out, err := runRemoteScript(cl, sb.String(), "", gitScanTimeout)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(out, "WRM_DONE") {
		return nil, fmt.Errorf("discovery did not finish")
	}
	files := map[string]bool{}
	var dirs []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "F\t") {
			files[line[2:]] = true
		} else if strings.HasPrefix(line, "D\t") {
			dirs = append(dirs, line[2:])
		}
	}
	words := backupWords()
	rootOf := func(dir string) string {
		best := ""
		for _, r := range cat.ServerRoots {
			if (dir == r || strings.HasPrefix(dir, strings.TrimSuffix(r, "/")+"/")) && len(r) > len(best) {
				best = r
			}
		}
		return best
	}
	usable := func(dir string) (string, bool) {
		root := rootOf(dir)
		if root == "" || relDepth(root, dir) > gitDiscoverDepth {
			return "", false
		}
		if ignoredPath(strings.Trim(strings.TrimPrefix(dir, root), "/"), cat.IgnoreDirs, words) {
			return "", false
		}
		return root, true
	}
	type cand struct {
		app   string
		score int
	}
	best := map[string]cand{} // dir → app
	consider := func(dir, app string, score int, hintMatch bool) {
		if hintMatch {
			score += 1
		}
		if c, ok := best[dir]; !ok || score > c.score || (score == c.score && app < c.app) {
			best[dir] = cand{app, score}
		}
	}
	for _, a := range apps {
		hintSet := map[string]bool{}
		for _, h := range a.InstallHint {
			hintSet[strings.ToLower(h)] = true
		}
		if len(a.Fingerprint) > 0 {
			counts := map[string]int{}
			for f := range files {
				for _, fp := range a.Fingerprint {
					if strings.HasSuffix(f, "/"+fp) {
						counts[strings.TrimSuffix(f, "/"+fp)]++
					}
				}
			}
			for dir, n := range counts {
				if n < len(a.Fingerprint) {
					continue
				}
				if _, ok := usable(dir); ok {
					consider(dir, a.Name, 2*n, hintSet[strings.ToLower(path.Base(dir))])
				}
			}
			continue
		}
		for _, d := range dirs {
			if hintSet[strings.ToLower(path.Base(d))] {
				if _, ok := usable(d); ok {
					consider(d, a.Name, 1, false)
				}
			}
		}
	}
	var out2 []gitFound
	ds := keysOf(best)
	sort.Strings(ds)
	taken := map[string]string{} // dir → app, to skip copies nested in an installation of the same app
	for _, d := range ds {
		c := best[d]
		nested := false
		for pd, app := range taken {
			if app == c.app && strings.HasPrefix(d, pd+"/") {
				nested = true
				break
			}
		}
		if nested {
			continue
		}
		taken[d] = c.app
		root := rootOf(d)
		out2 = append(out2, gitFound{App: c.app, Path: d, Env: envLabel(root, d), Root: root})
	}
	return out2, nil
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ─── scan ────────────────────────────────────────────

type gitUnit struct {
	Kind string `json:"kind"` // systemd | supervisor
	Name string `json:"name"`
}

type gitScanResult struct {
	Path    string
	Missing bool
	Files   map[string]string // rel → hash ("-" = unreadable)
	Sizes   map[string]int64  // rel → size in bytes
	Version []string          // first lines of VERSION.md
	Units   []gitUnit
	Capped  bool
}

const gitHashPrelude = `if command -v sha256sum >/dev/null 2>&1; then S="sha256sum"
elif command -v shasum >/dev/null 2>&1; then S="shasum -a 256"
elif command -v openssl >/dev/null 2>&1; then S="openssl dgst -sha256 -r"
else echo "E	sha256sum, shasum or openssl is needed on the server"; exit 0; fi
`

// scanOn hashes the files of installations on one server.
func scanOn(cl *ssh.Client, dirs []string) (map[string]*gitScanResult, error) {
	return scanOnMode(cl, dirs, true)
}

// scanFilesOn hashes files and reads VERSION.md, without looking for units (used by the
// environment overview, where only the deployed version is needed).
func scanFilesOn(cl *ssh.Client, dirs []string) (map[string]*gitScanResult, error) {
	return scanOnMode(cl, dirs, false)
}

func scanOnMode(cl *ssh.Client, dirs []string, withUnits bool) (map[string]*gitScanResult, error) {
	var sb strings.Builder
	sb.WriteString(gitHashPrelude)
	sysd := make([]string, len(gitSystemdDirs))
	for i, d := range gitSystemdDirs {
		sysd[i] = shellQuote(d)
	}
	sup := make([]string, len(gitSupervisorDirs))
	for i, d := range gitSupervisorDirs {
		sup[i] = shellQuote(d)
	}
	for _, d := range dirs {
		q := shellQuote(d)
		re := shellQuote(regexp.QuoteMeta(d) + `([/"' :;=]|$)`)
		fmt.Fprintf(&sb, "printf 'D\\t%%s\\n' %s\n", q)
		fmt.Fprintf(&sb, "if [ ! -d %s ]; then echo X; else\n", q)
		fmt.Fprintf(&sb, "find %s %s -o -type f -print 2>/dev/null | head -n %d | while IFS= read -r f; do\n", q, findPrune(), gitMaxInstallFiles+1)
		fmt.Fprintf(&sb, "  rel=${f#%s/}\n", q)
		sb.WriteString(`  if [ -r "$f" ]; then h=$(tr -d '\r' < "$f" | $S | cut -c1-64); z=$(wc -c < "$f"); z=${z##* }; else h=-; z=0; fi` + "\n")
		sb.WriteString(`  printf 'F\t%s\t%s\t%s\n' "$h" "$z" "$rel"` + "\n")
		sb.WriteString("done\n")
		fmt.Fprintf(&sb, "if [ -f %s/VERSION.md ]; then head -n 40 %s/VERSION.md | sed 's/^/V\\t/'; fi\n", q, q)
		if withUnits {
			fmt.Fprintf(&sb, "for u in %s; do [ -d \"$u\" ] && grep -lE -- %s \"$u\"/*.service 2>/dev/null; done | sort -u | while IFS= read -r f; do printf 'U\\tsystemd\\t%%s\\n' \"${f##*/}\"; done\n", strings.Join(sysd, " "), re)
			fmt.Fprintf(&sb, `for u in %s; do [ -d "$u" ] && grep -lE -- %s "$u"/*.conf "$u"/*.ini 2>/dev/null; done | sort -u | while IFS= read -r f; do
awk -v dir=%s 'function emit() {if (name != "" && hit) printf "U\tsupervisor\t%%s\n", name}
/^[[:space:]]*\[/ {emit(); name=""; hit=0; if ($0 ~ /^[[:space:]]*\[program:/) {name=$0; sub(/^[[:space:]]*\[program:/,"",name); sub(/\].*$/,"",name)}}
{p=index($0,dir); c=substr($0,p+length(dir),1); if(p && (c=="" || c ~ /[\/ ":;=]/ || c==sprintf("%%c",39))) hit=1}
END {emit()}' "$f"; done
`, strings.Join(sup, " "), re, shellQuote(d))
		}
		sb.WriteString("fi\n")
	}
	sb.WriteString("echo WRM_DONE\n")
	out, err := runRemoteScript(cl, sb.String(), "", gitScanTimeout)
	if err != nil {
		return nil, err
	}
	res := map[string]*gitScanResult{}
	var cur *gitScanResult
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "E\t"):
			return nil, fmt.Errorf("%s", line[2:])
		case strings.HasPrefix(line, "D\t"):
			cur = &gitScanResult{Path: line[2:], Files: map[string]string{}, Sizes: map[string]int64{}}
			res[cur.Path] = cur
		case cur == nil:
		case line == "X":
			cur.Missing = true
		case strings.HasPrefix(line, "F\t"):
			parts := strings.SplitN(line[2:], "\t", 3)
			if len(parts) == 3 {
				if len(cur.Files) >= gitMaxInstallFiles {
					cur.Capped = true
					continue
				}
				cur.Files[parts[2]] = parts[0]
				cur.Sizes[parts[2]], _ = strconv.ParseInt(parts[1], 10, 64)
			}
		case strings.HasPrefix(line, "V\t"):
			cur.Version = append(cur.Version, line[2:])
		case strings.HasPrefix(line, "U\t"):
			parts := strings.SplitN(line[2:], "\t", 2)
			if len(parts) == 2 && parts[1] != "" {
				dup := false
				for _, u := range cur.Units {
					dup = dup || (u.Kind == parts[0] && u.Name == parts[1])
				}
				if !dup {
					cur.Units = append(cur.Units, gitUnit{Kind: parts[0], Name: parts[1]})
				}
			}
		}
	}
	if !strings.Contains(out, "WRM_DONE") {
		return nil, fmt.Errorf("the scan did not finish")
	}
	return res, nil
}

// ─── VERSION.md ──────────────────────────────────────

type gitVersionInfo struct {
	App     string `json:"app,omitempty"`
	Version string `json:"version,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Commit  string `json:"commit,omitempty"`
	Updated string `json:"updated,omitempty"`
	Host    string `json:"host,omitempty"`
	User    string `json:"user,omitempty"`
	Bundle  string `json:"bundle,omitempty"`
}

var (
	versionFieldRe = regexp.MustCompile(`^\s*[-*]\s*\*\*([^*]+)\*\*\s*:\s*(.*?)\s*$`)
	versionTitleRe = regexp.MustCompile(`^#\s+(.+?)\s+-\s+(.+?)\s*$`)
)

// parseVersionMD reads the header of a VERSION.md written by the deploy tool (or WRM).
func parseVersionMD(lines []string) gitVersionInfo {
	var v gitVersionInfo
	title := ""
	for _, l := range lines {
		if m := versionFieldRe.FindStringSubmatch(l); m != nil {
			val := m[2]
			switch strings.ToLower(strings.TrimSpace(m[1])) {
			case "servis":
				v.App = val
			case "verzija":
				v.Version = val
			case "grana/ref":
				v.Ref = val
			case "commit":
				v.Commit = val
			case "azurirano", "ažurirano":
				v.Updated = val
			case "host":
				v.Host = val
			case "korisnik":
				v.User = val
			case "bundle":
				v.Bundle = val
			}
		} else if m := versionTitleRe.FindStringSubmatch(l); m != nil && title == "" {
			title = m[2]
			if v.App == "" {
				v.App = m[1]
			}
		}
	}
	if v.Version == "" {
		v.Version = title
	}
	return v
}

// ─── comparison ──────────────────────────────────────

type gitFileState struct {
	Path       string `json:"path"`
	State      string `json:"state"` // ok | old | modified | missing | extra | protected
	Behind     int    `json:"behind,omitempty"`
	Tag        string `json:"tag,omitempty"`
	Date       string `json:"date,omitempty"`
	ServerHash string `json:"server_hash,omitempty"`
	TargetHash string `json:"target_hash,omitempty"`
	Size       int64  `json:"size,omitempty"` // extra files: size on the server
	Note       string `json:"note,omitempty"`
	Diffable   bool   `json:"diffable,omitempty"`
}

// toolFiles are written by the deploy tool (and later by WRM) in an installation.
var toolFiles = map[string]bool{"VERSION.md": true, "updates.jsonl": true}

func matchHash(h string, hashes ...string) bool {
	for _, x := range hashes {
		if x != "" && x == h {
			return true
		}
	}
	return false
}

// trackedState compares the server hash of a tracked file with the target and its history:
// ok, old (with how many versions behind and the tag) or modified.
func trackedState(f gitTargetFile, h string) (string, int, string, string) {
	if matchHash(h, f.Hash, f.Hash2) {
		return "ok", 0, "", ""
	}
	for i, e := range f.History {
		if i == 0 {
			continue
		}
		if matchHash(h, e.Hash, e.Hash2) {
			tag := e.Tag
			if tag == "" {
				tag = e.Commit
			}
			behind := e.Version
			if behind == 0 {
				behind = i
			}
			return "old", behind, tag, e.Date
		}
	}
	return "modified", 0, "", ""
}

// gitDetected is the version concluded from the code on the server, not from VERSION.md.
type gitDetected struct {
	State   string `json:"state"` // exact | mixed | modified | missing | unknown
	Version string `json:"version,omitempty"`
	Commit  string `json:"commit,omitempty"`
	Date    string `json:"date,omitempty"`
	Approx  bool   `json:"approx,omitempty"` // from per-file history, without full version trees
}

func versionCommit(t gitTarget, v gitTargetVersion, i int) string {
	if v.CommitFull != "" {
		return v.CommitFull
	}
	if v.Commit != "" {
		return v.Commit
	}
	if i == 0 {
		if t.CommitFull != "" {
			return t.CommitFull
		}
		return t.Commit
	}
	return ""
}

func versionName(t gitTarget, v gitTargetVersion, i int) string {
	if i == 0 && t.Version != "" {
		return t.Version
	}
	if v.Tag != "" {
		return v.Tag
	}
	if v.Commit != "" {
		return v.Commit
	}
	return t.Version
}

// fileHashAtVersion returns the hash of a target file at one known version.
func fileHashAtVersion(f gitTargetFile, i int) (string, string, bool) {
	if i > f.Oldest {
		return "", "", false
	}
	hash, hash2 := f.Hash, f.Hash2
	for _, e := range f.History {
		if e.Version > i {
			break
		}
		hash, hash2 = e.Hash, e.Hash2
	}
	return hash, hash2, true
}

// historyAtVersion finds the history entry of a file at one version index. Older stored
// targets have no Version values, so it falls back to the position in the list.
func historyAtVersion(f gitTargetFile, v int) (gitHistEntry, bool) {
	for _, e := range f.History {
		if e.Version == v {
			return e, true
		}
	}
	if v < len(f.History) {
		return f.History[v], true
	}
	return gitHistEntry{}, false
}

// fileStateKey is a stable signature of the compared code, used to spot drift between
// servers even when no single version matches.
func fileStateKey(files []gitFileState) string {
	parts := make([]string, 0, len(files))
	for _, f := range files {
		if f.State == "extra" || f.State == "protected" {
			continue
		}
		parts = append(parts, f.Path+"\x00"+f.State+"\x00"+f.Tag+"\x00"+f.ServerHash)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\x1f")
}

// installMatchesVersion reports whether every comparable tracked file on the server is
// exactly as it was in one known version.
func installMatchesVersion(t gitTarget, server map[string]string, i int) bool {
	comparable := 0
	for rel, f := range t.Files {
		if globHit(rel, t.Protected) {
			continue
		}
		comparable++
		h, ok := server[rel]
		if !ok || h == "-" {
			return false
		}
		hash, hash2, ok := fileHashAtVersion(f, i)
		if !ok || !matchHash(h, hash, hash2) {
			return false
		}
	}
	return comparable > 0
}

// detectInstallVersion determines the deployed version from the code itself. The target's
// version trees are complete (computeTarget fetched them), so no extra API calls are needed.
func detectInstallVersion(t gitTarget, server map[string]string, files []gitFileState) gitDetected {
	if len(t.Files) == 0 {
		return gitDetected{State: "unknown"}
	}
	if len(t.Versions) > 0 {
		for i, v := range t.Versions {
			if installMatchesVersion(t, server, i) {
				return gitDetected{State: "exact", Version: versionName(t, v, i), Commit: versionCommit(t, v, i), Date: v.Date}
			}
		}
	}
	// No single version matches (or the target has no version metadata yet): use the
	// per-file comparison for a clear state and a best-effort older version.
	var allOK, modified, missing bool = true, false, false
	tags := map[string]bool{}
	tracked := 0
	for _, f := range files {
		switch f.State {
		case "ok":
			tracked++
		case "old":
			tracked++
			allOK = false
			if f.Tag != "" {
				tags[f.Tag] = true
			}
		case "modified":
			tracked++
			allOK = false
			modified = true
		case "missing":
			tracked++
			allOK = false
			missing = true
		}
	}
	switch {
	case tracked == 0:
		return gitDetected{State: "unknown"}
	case modified:
		return gitDetected{State: "modified"}
	case missing:
		return gitDetected{State: "missing"}
	case allOK:
		commit := t.CommitFull
		if commit == "" {
			commit = t.Commit
		}
		return gitDetected{State: "exact", Version: t.Version, Commit: commit, Date: t.CommitDate}
	case len(tags) == 1:
		// Old targets and bundles without version trees: all changed files point to one
		// tag, so that is the most likely deployed version.
		tag := ""
		for x := range tags {
			tag = x
		}
		commit, date := "", ""
		for _, f := range files {
			if f.State != "old" || f.Tag != tag {
				continue
			}
			for _, e := range t.Files[f.Path].History {
				if e.Tag == tag || e.Commit == tag {
					commit, date = e.CommitFull, e.Date
					if commit == "" {
						commit = e.Commit
					}
					break
				}
			}
			if commit != "" {
				break
			}
		}
		return gitDetected{State: "exact", Version: tag, Commit: commit, Date: date, Approx: true}
	default:
		return gitDetected{State: "mixed"}
	}
}

// compareInstall compares the files on a server with a target.
func compareInstall(t gitTarget, server map[string]string, ignore []string) ([]gitFileState, map[string]int, string) {
	words := backupWords()
	counts := map[string]int{}
	var out []gitFileState
	for rel, f := range t.Files {
		fs := gitFileState{Path: rel, TargetHash: f.Hash, Diffable: f.Blob != ""}
		h, ok := server[rel]
		fs.ServerHash = h
		switch {
		case globHit(rel, t.Protected):
			fs.State = "protected"
			fs.Diffable = false
			if !ok {
				fs.Note = "missing"
			}
		case !ok:
			fs.State = "missing"
		case h == "-":
			fs.State = "modified"
			fs.Note = "unreadable"
		default:
			fs.State, fs.Behind, fs.Tag, fs.Date = trackedState(f, h)
		}
		counts[fs.State]++
		out = append(out, fs)
	}
	for rel, h := range server {
		if _, known := t.Files[rel]; known || toolFiles[rel] || strings.HasSuffix(rel, ".pyc") || isLogPath(rel) || ignoredPath(path.Dir(rel), ignore, words) {
			continue
		}
		if len(t.Include) > 0 && !globHitDir(rel, t.Include) {
			continue
		}
		if globHitDir(rel, t.Exclude) {
			continue
		}
		fs := gitFileState{Path: rel, State: "extra", ServerHash: h}
		if globHit(rel, t.Protected) {
			fs.State = "protected"
		}
		counts[fs.State]++
		out = append(out, fs)
	}
	order := map[string]int{"modified": 0, "missing": 1, "old": 2, "extra": 3, "protected": 4, "ok": 5}
	sort.Slice(out, func(i, j int) bool {
		if order[out[i].State] != order[out[j].State] {
			return order[out[i].State] < order[out[j].State]
		}
		return out[i].Path < out[j].Path
	})
	state := "ok"
	switch {
	case counts["modified"] > 0:
		state = "review"
	case counts["old"] > 0 || counts["missing"] > 0:
		state = "update"
	}
	return out, counts, state
}

// ─── installations ───────────────────────────────────

type gitInstall struct {
	Functionality *gitFunctionality `json:"functionality,omitempty"`
	ID            int               `json:"id"`
	ConnID        int               `json:"conn_id"`
	ConnName      string            `json:"conn_name"`
	Host          string            `json:"host,omitempty"`
	Hostname      string            `json:"hostname,omitempty"` // the server's real hostname (cached), "" = not read yet
	FolderID      *int              `json:"folder_id,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	App           string            `json:"app"`
	Path          string            `json:"path"`
	Env           string            `json:"env"`
	Manual        bool              `json:"manual"`
	State         string            `json:"state"` // ok | update | review | error | gone | unknown
	Counts        map[string]int    `json:"counts"`
	Behind        int               `json:"behind"`
	Target        string            `json:"target"`
	VersionMD     gitVersionInfo    `json:"version_md"`
	Detected      gitDetected       `json:"detected"`
	Units         []gitUnit         `json:"units"`
	Error         string            `json:"error,omitempty"`
	Capped        bool              `json:"capped,omitempty"`
	CheckedAt     string            `json:"checked_at"`
	DiscoveredAt  string            `json:"discovered_at"`
	LastOKAt      string            `json:"last_ok_at,omitempty"`    // last check that did not fail
	LastOKState   string            `json:"last_ok_state,omitempty"` // and its state
	Files         []gitFileState    `json:"files,omitempty"`         // details only
	// time of the last update whose units were not restarted yet
	RestartPending string `json:"restart_pending,omitempty"`
}

type gitInstallData struct {
	Functionality *gitFunctionality `json:"functionality,omitempty"`
	Counts        map[string]int    `json:"counts"`
	Behind        int               `json:"behind"`
	Target        string            `json:"target"`
	VersionMD     gitVersionInfo    `json:"version_md"`
	Detected      gitDetected       `json:"detected"`
	Units         []gitUnit         `json:"units"`
	Error         string            `json:"error,omitempty"`
	Capped        bool              `json:"capped,omitempty"`
	Files         []gitFileState    `json:"files"`
}

// deleteGitInstallsOf forgets the installations found on a deleted connection. Installations
// whose connection is gone are not listed either (loadGitInstalls), so older leftovers cannot
// show up as nameless servers.
func deleteGitInstallsOf(userID, connID int) {
	db.Exec(`DELETE FROM git_installs WHERE user_id=? AND conn_id=?`, userID, connID)
	db.Exec(`DELETE FROM git_hostnames WHERE conn_id=?`, connID)
}

func loadGitInstalls(userID int, withFiles bool, where string, args ...interface{}) []gitInstall {
	out := []gitInstall{}
	q := `SELECT i.id, i.conn_id, COALESCE(c.name,''), COALESCE(c.host,''), COALESCE(h.hostname,''), c.folder_id, COALESCE(c.tags,''), i.app, i.path, i.env, i.manual, i.state, i.data, i.checked_at, i.discovered_at,
		COALESCE(p.since,''), i.last_ok_at, i.last_ok_state FROM git_installs i LEFT JOIN connections c ON c.id=i.conn_id LEFT JOIN git_hostnames h ON h.conn_id=i.conn_id LEFT JOIN git_pending_restarts p ON p.install_id=i.id WHERE i.user_id=? AND c.id IS NOT NULL`
	if where != "" {
		q += " AND " + where
	}
	rows, err := db.Query(q+" ORDER BY c.name, i.path", append([]interface{}{userID}, args...)...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var in gitInstall
		var tags, data string
		var manual int
		rows.Scan(&in.ID, &in.ConnID, &in.ConnName, &in.Host, &in.Hostname, &in.FolderID, &tags, &in.App, &in.Path, &in.Env, &manual, &in.State, &data, &in.CheckedAt, &in.DiscoveredAt, &in.RestartPending,
			&in.LastOKAt, &in.LastOKState)
		in.Manual = manual == 1
		in.Tags = parseTags(tags)
		var d gitInstallData
		jsonUnmarshalString(data, &d)
		in.Counts, in.Behind, in.Target, in.VersionMD, in.Detected, in.Units, in.Error, in.Capped = d.Counts, d.Behind, d.Target, d.VersionMD, d.Detected, d.Units, d.Error, d.Capped
		in.Functionality = d.Functionality
		if in.Counts == nil {
			in.Counts = map[string]int{}
		}
		if in.Units == nil {
			in.Units = []gitUnit{}
		}
		if withFiles {
			in.Files = d.Files
		}
		out = append(out, in)
	}
	return out
}

func jsonUnmarshalString(s string, v interface{}) {
	if s != "" {
		jsonUnmarshalBytes([]byte(s), v)
	}
}

// ─── a whole check: discover + scan + compare ────────

type gitServerResult struct {
	ConnID int    `json:"conn_id"`
	Name   string `json:"name"`
	Found  int    `json:"found"`
	Error  string `json:"error,omitempty"`
}

type gitRunResult struct {
	Servers  []gitServerResult `json:"servers"`
	Installs []gitInstall      `json:"installs"`
	Targets  []gitTargetBrief  `json:"targets"`
	Changes  []gitChange       `json:"-"`
}

type gitChange struct {
	Kind  string // drift | unreachable
	Name  string
	App   string
	Path  string
	Error string
}

// runGitCheck discovers (optional) and compares the installations on the given servers.
func runGitCheck(userID int, connIDs []int, discover bool, targets map[string]gitTarget) gitRunResult {
	cat := loadGitCatalog(userID)
	apps := effectiveApps(userID, cat)
	res := gitRunResult{Servers: []gitServerResult{}}
	var mu sync.Mutex
	sem := make(chan struct{}, settingInt("git_check_parallel"))
	var wg sync.WaitGroup
	for _, id := range connIDs {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sr, changes := checkServer(userID, id, cat, apps, discover, targets)
			mu.Lock()
			res.Servers = append(res.Servers, sr)
			res.Changes = append(res.Changes, changes...)
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	sort.Slice(res.Servers, func(i, j int) bool { return res.Servers[i].Name < res.Servers[j].Name })
	res.Installs = loadGitInstalls(userID, false, "")
	return res
}

func checkServer(userID, connID int, cat *gitCatalog, apps []gitCatalogApp, discover bool, targets map[string]gitTarget) (gitServerResult, []gitChange) {
	sr := gitServerResult{ConnID: connID}
	var changes []gitChange
	c, err := loadConnection(connID)
	if err != nil || c.UserID != userID || !isSSHProtocol(c) {
		sr.Error = "not an SSH connection of yours"
		return sr, nil
	}
	sr.Name = c.Name
	now := nowStamp()
	markUnreachable := func(msg string) {
		sr.Error = msg
		saveGitServerHealthError(userID, connID, msg)
		for _, in := range loadGitInstalls(userID, true, "i.conn_id=?", connID) {
			saveInstallError(in, msg)
		}
		wasErr := getNotifyState(fmt.Sprintf("git:unreach:%d:%d", userID, connID)) != ""
		if !wasErr {
			changes = append(changes, gitChange{Kind: "unreachable", Name: c.Name, Error: msg})
			setNotifyState(fmt.Sprintf("git:unreach:%d:%d", userID, connID), now)
		}
	}
	cl, err := dialSSH(c, nil)
	if err != nil {
		markUnreachable(err.Error())
		return sr, changes
	}
	defer cl.Close()
	checkGitServerHealth(userID, connID, cl)
	cacheServerHostname(connID, cl)
	setNotifyState(fmt.Sprintf("git:unreach:%d:%d", userID, connID), "")
	if discover {
		n, err := discoverAndRegister(userID, connID, cl, cat, apps, now)
		if err != nil {
			sr.Error = err.Error()
		} else {
			sr.Found = n
		}
	}
	installs := loadGitInstalls(userID, true, "i.conn_id=?", connID)
	if len(installs) == 0 {
		return sr, changes
	}
	dirs := make([]string, len(installs))
	for i, in := range installs {
		dirs[i] = in.Path
	}
	scans, err := scanOn(cl, dirs)
	if err != nil {
		sr.Error = err.Error()
		for _, in := range installs {
			saveInstallError(in, err.Error())
		}
		return sr, changes
	}
	for _, in := range installs {
		t, ok := targets[in.App]
		if !ok {
			t, ok = loadGitTarget(userID, in.App)
		}
		state, d := evalInstall(scans[in.Path], t, ok, cat.IgnoreDirs)
		app, _ := cat.app(in.App)
		checkGitFunctionality(cl, in, app, state, &d)
		if state == "review" && in.State != "review" && in.State != "unknown" {
			changes = append(changes, gitChange{Kind: "drift", Name: c.Name, App: in.App, Path: in.Path})
		}
		saveInstallResult(in.ID, state, d)
	}
	return sr, changes
}

// discoverAndRegister finds the installations on one server and stores new ones; the ones that
// are no longer found remain registered so missing installations can be reported.
func discoverAndRegister(userID, connID int, cl *ssh.Client, cat *gitCatalog, apps []gitCatalogApp, now string) (int, error) {
	found, err := discoverOn(cl, cat, apps)
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	for _, f := range found {
		seen[f.Path] = true
		var id int
		if db.QueryRow(`SELECT id FROM git_installs WHERE user_id=? AND conn_id=? AND path=?`, userID, connID, f.Path).Scan(&id) == nil {
			db.Exec(`UPDATE git_installs SET app=?, env=? WHERE id=? AND manual=0`, f.App, f.Env, id)
		} else {
			db.Exec(`INSERT INTO git_installs (user_id, conn_id, app, path, env, manual, state, data, checked_at, discovered_at) VALUES (?,?,?,?,?,0,'unknown','','',?)`,
				userID, connID, f.App, f.Path, f.Env, now)
		}
	}
	return len(found), nil
}

// evalInstall derives the state and the stored data of an installation from its scan.
func evalInstall(s *gitScanResult, t gitTarget, ok bool, ignore []string) (string, gitInstallData) {
	d := gitInstallData{Units: []gitUnit{}}
	state := "unknown"
	switch {
	case s == nil:
		d.Error = "no answer for this directory"
		state = "error"
	case s.Missing:
		d.Error = "the directory does not exist any more"
		state = "gone"
	default:
		d.VersionMD = parseVersionMD(s.Version)
		if s.Units != nil {
			d.Units = s.Units
		}
		d.Capped = s.Capped
		if !ok || t.Error != "" || len(t.Files) == 0 {
			d.Error = "no target for this service yet (refresh the targets)"
			if ok && t.Error != "" {
				d.Error = t.Error
			}
			for rel, h := range s.Files {
				d.Files = append(d.Files, gitFileState{Path: rel, State: "extra", ServerHash: h, Size: s.Sizes[rel]})
			}
			d.Detected = gitDetected{State: "unknown"}
		} else {
			d.Target = t.Version
			d.Files, d.Counts, state = compareInstall(t, s.Files, ignore)
			for i := range d.Files {
				if d.Files[i].State == "extra" {
					d.Files[i].Size = s.Sizes[d.Files[i].Path]
				}
			}
			for _, f := range d.Files {
				if f.State == "old" && f.Behind > d.Behind {
					d.Behind = f.Behind
				}
			}
			d.Detected = detectInstallVersion(t, s.Files, d.Files)
			if s.Capped {
				d.Detected = gitDetected{State: "unknown"}
			}
		}
	}
	return state, d
}

// refreshInstallState compares one installation again (after a change on the server).
func refreshInstallState(userID int, in gitInstall, cl *ssh.Client) {
	scans, err := scanOn(cl, []string{in.Path})
	if err != nil {
		return
	}
	t, ok := loadGitTarget(userID, in.App)
	cat := loadGitCatalog(userID)
	state, d := evalInstall(scans[in.Path], t, ok, cat.IgnoreDirs)
	app, _ := cat.app(in.App)
	checkGitFunctionality(cl, in, app, state, &d)
	saveInstallResult(in.ID, state, d)
}

func saveInstallResult(id int, state string, d gitInstallData) {
	if d.Files == nil {
		d.Files = []gitFileState{}
	}
	now := nowStamp()
	notifyGitFunctionality(id, d.Functionality)
	if state == "error" || state == "gone" {
		db.Exec(`UPDATE git_installs SET state=?, data=?, checked_at=? WHERE id=?`, state, string(jsonMarshal(d)), now, id)
		return
	}
	db.Exec(`UPDATE git_installs SET state=?, data=?, checked_at=?, last_ok_at=?, last_ok_state=? WHERE id=?`, state, string(jsonMarshal(d)), now, now, state, id)
}

// saveInstallError records a failed check of one installation and keeps the files and
// counts of the last good result (the last good state and time stay in their columns).
func saveInstallError(in gitInstall, msg string) {
	var data string
	db.QueryRow(`SELECT data FROM git_installs WHERE id=?`, in.ID).Scan(&data)
	var d gitInstallData
	jsonUnmarshalString(data, &d)
	d.Error = redactText(msg)
	d.Functionality = &gitFunctionality{State: "failed", CheckedAt: nowStamp(), Checks: []gitRuntimeCheck{}, Warnings: []string{d.Error}}
	saveInstallResult(in.ID, "error", d)
}
