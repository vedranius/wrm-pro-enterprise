// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── GIT ENVIRONMENTS: destinations, deploy plans, locks and server-side state ───
//
// An environment of a service (catalog: apps[].environments) is an ordered list of
// destinations (a WRM SSH connection by name and an installation path) with its own rules:
// a typed confirmation, allowed refs, extra ignore patterns, an optional post_deploy command
// and the number of backups to keep. Catalogs without environments work as before.
//
// A deploy to an environment is an update (or upgrade) run whose items are the destinations
// in order (git_env_run.go):
//  1. compare: a plan per destination (files to add / change / delete, excluded and ignored
//     files, CRLF/LF-only differences, type changes) with a fingerprint of what was compared;
//     a plan is kept in memory and applied once;
//  2. per destination: an atomic mkdir lock in the state directory with an owner file, the
//     plan computed again under the lock (another fingerprint stops the deploy), the files as
//     one tar stream into a staging directory, verified, a backup of what is replaced or
//     deleted, renames, checks, VERSION.md, state.json and a line in history.jsonl; a deploy
//     releases only its own lock.
//
// The state directory defaults to <app>/.deploy-bak (the backup directory of the file-based
// deploy tool, never deployed into or compared). Layout:
//
//	<state>/<deploy id>/                the backup of a deploy (the existing backup format)
//	<state>/deploys/<deploy id>.json    its manifest (added, changed, deleted files, removed directories)
//	<state>/state.json                  the current and the previous state
//	<state>/history.jsonl               the append-only history (DEPLOY and ROLLBACK entries)
//	<state>/backups/rollback-<id>/      what a rollback moved aside
//	<state>/.lock/owner                 the lock (deploy id, user, WRM host, server time)

const (
	gitEnvMaxDests     = 20
	gitEnvPlanTTL      = 2 * time.Hour
	gitEnvKeepDefault  = 5
	gitEnvKeepRollback = 5
	gitEnvHistoryList  = 500 // file names per kind in a history entry
	gitPostDeployTime  = 10 * time.Minute
)

var (
	// how long a transfer may take (at least an hour; a variable for tests)
	gitTransferTimeout = time.Hour
	// added to the receive script after unpacking (tests simulate an interrupted transfer)
	gitEnvReceiveHook = ""
)

// gitEnvStaleAfter is the age from which a lock counts as stale: max(30 min, the transfer timeout).
func gitEnvStaleAfter() time.Duration {
	if gitTransferTimeout > 30*time.Minute {
		return gitTransferTimeout
	}
	return 30 * time.Minute
}

// ─── catalog: environments ───────────────────────────

type gitEnvDest struct {
	Server string `json:"server"` // the name of an SSH connection
	Path   string `json:"path"`
}

type gitEnvironment struct {
	Name         string       `json:"name"`
	Destinations []gitEnvDest `json:"destinations"`
	Confirm      bool         `json:"confirm,omitempty"`      // type the environment name to deploy
	AllowedRefs  []string     `json:"allowed_refs,omitempty"` // refs/heads/main, refs/tags/v*
	Ignore       []string     `json:"ignore,omitempty"`       // extra ignore patterns
	PostDeploy   string       `json:"post_deploy,omitempty"`  // runs in the app directory when chosen
	KeepBackups  int          `json:"keep_backups,omitempty"` // deploy backups kept per destination
	StateDir     string       `json:"state_dir,omitempty"`    // "" = <path>/.deploy-bak
	Auto         bool         `json:"auto,omitempty"`         // derived from the detected installations, not stored (git_env_auto.go)
}

var prodEnvNames = map[string]bool{"prod": true, "production": true, "prd": true, "live": true}

// needsConfirm: the environment asks for it, or its name says production.
func (e gitEnvironment) needsConfirm() bool {
	return e.Confirm || prodEnvNames[strings.ToLower(e.Name)]
}

func (e gitEnvironment) keep() int {
	if e.KeepBackups <= 0 {
		return gitEnvKeepDefault
	}
	return e.KeepBackups
}

func (e *gitEnvironment) normalize(app string) error {
	e.Name = strings.TrimSpace(e.Name)
	if !gitNameRe.MatchString(e.Name) {
		return fmt.Errorf("%s: invalid environment name %q (letters, digits, . _ + -)", app, e.Name)
	}
	if len(e.Destinations) == 0 || len(e.Destinations) > gitEnvMaxDests {
		return fmt.Errorf("%s / %s: 1 to %d destinations", app, e.Name, gitEnvMaxDests)
	}
	seen := map[string]bool{}
	for i := range e.Destinations {
		d := &e.Destinations[i]
		d.Server = strings.TrimSpace(d.Server)
		if d.Server == "" || len(d.Server) > 200 || strings.ContainsAny(d.Server, "\n\r\t") {
			return fmt.Errorf("%s / %s: enter the server (an SSH connection name) of every destination", app, e.Name)
		}
		p, err := validInstallDir(d.Path)
		if err != nil {
			return fmt.Errorf("%s / %s: %v", app, e.Name, err)
		}
		d.Path = p
		k := strings.ToLower(d.Server) + ":" + p
		if seen[k] {
			return fmt.Errorf("%s / %s: %s:%s is listed twice", app, e.Name, d.Server, p)
		}
		seen[k] = true
	}
	e.AllowedRefs = cleanList(e.AllowedRefs, 30)
	e.Ignore = cleanList(e.Ignore, 100)
	e.PostDeploy = strings.TrimSpace(e.PostDeploy)
	if len(e.PostDeploy) > 500 || strings.ContainsAny(e.PostDeploy, "\n\r") {
		return fmt.Errorf("%s / %s: post_deploy must be one line of at most 500 characters", app, e.Name)
	}
	if e.KeepBackups < 0 || e.KeepBackups > 50 {
		return fmt.Errorf("%s / %s: keep 1 to 50 backups", app, e.Name)
	}
	if strings.TrimSpace(e.StateDir) != "" {
		p, err := validInstallDir(e.StateDir)
		if err != nil {
			return fmt.Errorf("%s / %s: state directory: %v", app, e.Name, err)
		}
		e.StateDir = p
	} else {
		e.StateDir = ""
	}
	return nil
}

func normalizeEnvs(app string, envs []gitEnvironment) ([]gitEnvironment, error) {
	if len(envs) == 0 {
		return nil, nil
	}
	if len(envs) > 20 {
		return nil, fmt.Errorf("%s: at most 20 environments", app)
	}
	seen := map[string]bool{}
	for i := range envs {
		if err := envs[i].normalize(app); err != nil {
			return nil, err
		}
		k := strings.ToLower(envs[i].Name)
		if seen[k] {
			return nil, fmt.Errorf("%s: environment %s is listed twice", app, envs[i].Name)
		}
		seen[k] = true
	}
	return envs, nil
}

// envStateDir is the state directory of one destination.
func envStateDir(e gitEnvironment, app, p string) string {
	if e.StateDir == "" {
		return p + "/" + gitBackupDir
	}
	flat := strings.ReplaceAll(strings.Trim(p, "/"), "/", "_")
	return e.StateDir + "/" + app + "/" + flat
}

// within tells whether p is dir or lies below it.
func within(p, dir string) bool { return p == dir || strings.HasPrefix(p, dir+"/") }

// checkEnvPaths rejects destinations on one server where one app path lies inside another
// (unless the outer service's excludes cover the inner one) and state directories inside
// an app path (the default <app>/.deploy-bak is never deployed into).
func checkEnvPaths(apps []gitCatalogApp) error {
	type dest struct {
		server, path, app, env, state string
		excl                          []string
		custom                        bool
	}
	var all []dest
	for _, a := range apps {
		for _, e := range a.Environments {
			for _, d := range e.Destinations {
				all = append(all, dest{strings.ToLower(d.Server), d.Path, a.Name, e.Name, envStateDir(e, a.Name, d.Path),
					append(append([]string{}, a.Exclude...), e.Ignore...), e.StateDir != ""})
			}
		}
	}
	for _, x := range all {
		for _, y := range all {
			if x.server != y.server {
				continue
			}
			if x.path == y.path && x.app != y.app {
				return fmt.Errorf("%s (%s) and %s (%s) use the same path %s on %s", x.app, x.env, y.app, y.env, x.path, x.server)
			}
			if x.path != y.path && within(y.path, x.path) {
				rel := strings.TrimPrefix(y.path, x.path+"/")
				if !globHitDir(rel, x.excl) {
					return fmt.Errorf("%s (%s) at %s lies inside %s (%s) at %s: exclude %s in %s first", y.app, y.env, y.path, x.app, x.env, x.path, "/"+globEscape(rel), x.app)
				}
			}
			if y.custom && within(y.state, x.path) {
				return fmt.Errorf("%s (%s): the state directory %s lies inside the app path %s on %s", y.app, y.env, y.state, x.path, x.server)
			}
		}
	}
	return nil
}

// globEscape escapes the glob characters of a path (fnmatch: [*] [?] [[]).
func globEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '*', '?', '[':
			b.WriteString("[" + string(r) + "]")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func findEnv(cat *gitCatalog, app, env string) (*gitCatalogApp, *gitEnvironment, error) {
	a, ok := cat.app(app)
	if !ok {
		return nil, nil, fmt.Errorf("%s is not in the catalog", app)
	}
	for i := range a.Environments {
		if a.Environments[i].Name == env {
			return a, &a.Environments[i], nil
		}
	}
	return a, nil, fmt.Errorf("%s has no environment %s", app, env)
}

// refName is the full ref of a target: refs/tags/<tag> or refs/heads/<branch>.
func refName(t gitTarget) string {
	if t.RefKind == "tag" {
		return "refs/tags/" + t.Version
	}
	return "refs/heads/" + t.Branch
}

// refAllowed matches the target's ref with allowed_refs (empty = any); a pattern without
// refs/ matches the short branch or tag name.
func refAllowed(t gitTarget, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	full := refName(t)
	short := strings.TrimPrefix(strings.TrimPrefix(full, "refs/tags/"), "refs/heads/")
	for _, p := range allowed {
		if fnmatch(full, p) || (!strings.HasPrefix(p, "refs/") && fnmatch(short, p)) {
			return true
		}
	}
	return false
}

// envConn resolves the SSH connection of a destination by name (one connection of the user).
func envConn(userID int, server string) (Connection, error) {
	rows, err := db.Query(`SELECT id FROM connections WHERE user_id=? AND name=? COLLATE NOCASE`, userID, server)
	if err != nil {
		return Connection{}, err
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	var found []Connection
	for _, id := range ids {
		if c, err := loadConnection(id); err == nil && isSSHProtocol(c) {
			found = append(found, c)
		}
	}
	switch len(found) {
	case 0:
		return Connection{}, fmt.Errorf("no SSH connection named %q", server)
	case 1:
		return found[0], nil
	}
	return Connection{}, fmt.Errorf("%d SSH connections are named %q: rename one", len(found), server)
}

// ─── server-side state ───────────────────────────────

// gitEnvRecord is a deployed state (state.json current / previous).
type gitEnvRecord struct {
	DeployID string `json:"deploy_id"`
	App      string `json:"app"`
	Env      string `json:"env"`
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Branch   string `json:"branch"`
	Ref      string `json:"ref"`
	By       string `json:"by"`
	At       string `json:"at"`
	Host     string `json:"host,omitempty"`
	Source   string `json:"source,omitempty"`
}

type gitEnvState struct {
	Current    *gitEnvRecord `json:"current,omitempty"`
	Previous   *gitEnvRecord `json:"previous,omitempty"`
	RolledBack string        `json:"rolled_back,omitempty"` // the deploy a rollback undid
}

// gitEnvLock is a lock found on a server.
type gitEnvLock struct {
	ID    string `json:"id"`
	User  string `json:"user"`
	From  string `json:"from"`
	Since string `json:"since,omitempty"` // server time
	Age   int64  `json:"age_seconds"`
	Stale bool   `json:"stale"`
}

// envLockRead prints the lock of state directory $SD (K lines) and the server time.
const envLockRead = `L="$SD/.lock"
if [ -d "$L" ]; then printf 'K\t1\n'; [ -f "$L/owner" ] && sed 's/^/KO\t/' "$L/owner"
  printf 'KM\t%s\n' "$(stat -c %Y "$L" 2>/dev/null || stat -f %m "$L" 2>/dev/null)"; fi
printf 'NOW\t%s\n' "$(date +%s)"
`

func parseLock(out string) *gitEnvLock {
	var l *gitEnvLock
	var epoch, mtime, now int64
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "K":
			l = &gitEnvLock{}
		case "KO":
			if l == nil {
				continue
			}
			k, v, _ := strings.Cut(parts[1], "=")
			switch k {
			case "id":
				l.ID = v
			case "user":
				l.User = v
			case "from":
				l.From = v
			case "epoch":
				epoch, _ = strconv.ParseInt(v, 10, 64)
			}
		case "KM":
			mtime, _ = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		case "NOW":
			now, _ = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		}
	}
	if l == nil {
		return nil
	}
	if epoch == 0 {
		epoch = mtime
	}
	if epoch > 0 && now > 0 {
		l.Age = now - epoch
		l.Since = time.Unix(epoch, 0).UTC().Format(time.RFC3339)
	}
	l.Stale = epoch > 0 && l.Age >= int64(gitEnvStaleAfter()/time.Second)
	return l
}

func cleanOwnerValue(s string) string {
	return truncateStr(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ", "=", "-").Replace(s), 200)
}

var wrmHostName = func() string {
	h, _ := os.Hostname()
	if h == "" {
		h = "wrm"
	}
	return h
}

// envLock takes the lock of a state directory: mkdir is atomic, the owner file says who
// holds it (deploy id, WRM user, WRM host, the server's time). It returns the holder when
// the lock is taken by someone else.
func envLock(cl *ssh.Client, stateDir, id, user string) (*gitEnvLock, error) {
	script := fmt.Sprintf(`SD=%s; L="$SD/.lock"
mkdir -p "$SD" 2>/dev/null || { printf 'E\tcannot create the state directory %%s\n' "$SD"; exit 0; }
if mkdir "$L" 2>/dev/null; then
  if printf 'id=%%s\nuser=%%s\nfrom=%%s\nepoch=%%s\n' %s %s %s "$(date +%%s)" > "$L/owner"; then echo WRM_LOCKED; exit 0; fi
  rm -rf "$L"; printf 'E\tcannot write the lock owner file\n'; exit 0
fi
echo WRM_BUSY
%s`, shellQuote(stateDir), shellQuote(cleanOwnerValue(id)), shellQuote(cleanOwnerValue(user)), shellQuote(cleanOwnerValue(wrmHostName())), envLockRead)
	out, err := runRemoteScript(cl, script, "", gitStepTimeout)
	if err != nil {
		return nil, err
	}
	if e := scriptError(out); e != nil {
		return nil, e
	}
	if strings.Contains(out, "WRM_LOCKED") {
		return nil, nil
	}
	if !strings.Contains(out, "WRM_BUSY") {
		return nil, fmt.Errorf("the lock did not answer")
	}
	l := parseLock(out)
	if l == nil {
		l = &gitEnvLock{}
	}
	return l, nil
}

// envUnlock releases a lock only when its owner file names this deploy.
func envUnlock(cl *ssh.Client, stateDir, id string) (bool, error) {
	script := fmt.Sprintf(`L=%s/.lock
if [ -f "$L/owner" ] && grep -qx %s "$L/owner"; then rm -rf "$L" && echo WRM_RELEASED; else echo WRM_NOT_MINE; fi
`, shellQuote(stateDir), shellQuote("id="+cleanOwnerValue(id)))
	out, err := runRemoteScript(cl, script, "", gitStepTimeout)
	if err != nil {
		return false, err
	}
	return strings.Contains(out, "WRM_RELEASED"), nil
}

// envBreakStale removes a stale lock in one command that checks again on the server that
// it is still the same lock (owner id) and still stale.
func envBreakStale(cl *ssh.Client, stateDir, id string) (bool, error) {
	script := fmt.Sprintf(`L=%s/.lock
[ -d "$L" ] || { echo WRM_GONE; exit 0; }
i=$(sed -n 's/^id=//p' "$L/owner" 2>/dev/null | head -n 1)
e=$(sed -n 's/^epoch=//p' "$L/owner" 2>/dev/null | head -n 1)
[ -n "$e" ] || e=$(stat -c %%Y "$L" 2>/dev/null || stat -f %%m "$L" 2>/dev/null)
n=$(date +%%s)
if [ "$i" = %s ] && [ -n "$e" ] && [ $((n - e)) -ge %d ]; then rm -rf "$L" && echo WRM_REMOVED; else echo WRM_CHANGED; fi
`, shellQuote(stateDir), shellQuote(cleanOwnerValue(id)), int64(gitEnvStaleAfter()/time.Second))
	out, err := runRemoteScript(cl, script, "", gitStepTimeout)
	if err != nil {
		return false, err
	}
	if strings.Contains(out, "WRM_GONE") {
		return true, nil
	}
	return strings.Contains(out, "WRM_REMOVED"), nil
}

func scriptError(out string) error {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "E\t") {
			return fmt.Errorf("%s", line[2:])
		}
	}
	return nil
}

// envWriteFile writes a file of the state directory atomically (content on stdin).
func envWriteFile(cl *ssh.Client, file string, content []byte) error {
	script := fmt.Sprintf(`F=%s; mkdir -p "${F%%/*}" || exit 3
cat > "$F.wrm-tmp" && mv -f "$F.wrm-tmp" "$F" && echo WRM_OK
`, shellQuote(file))
	out, err := runRemoteScript(cl, script, string(content), gitStepTimeout)
	if err == nil && !strings.Contains(out, "WRM_OK") {
		err = fmt.Errorf("%s could not be written", file)
	}
	return err
}

// envAppendHistory appends one JSON line to history.jsonl.
func envAppendHistory(cl *ssh.Client, stateDir string, entry map[string]interface{}) error {
	line := string(jsonMarshal(entry))
	script := fmt.Sprintf(`F=%s/history.jsonl; mkdir -p "${F%%/*}" || exit 3
cat >> "$F" && echo WRM_OK
`, shellQuote(stateDir))
	out, err := runRemoteScript(cl, script, line+"\n", gitStepTimeout)
	if err == nil && !strings.Contains(out, "WRM_OK") {
		err = fmt.Errorf("history.jsonl could not be written")
	}
	return err
}

// listCap keeps at most gitEnvHistoryList names of a list.
func listCap(l []string) []string {
	if l == nil {
		return []string{}
	}
	if len(l) > gitEnvHistoryList {
		return l[:gitEnvHistoryList]
	}
	return l
}

// ─── the plan ────────────────────────────────────────

// gitEnvFile is one file of a destination plan.
type gitEnvFile struct {
	Path    string `json:"path"`
	State   string `json:"state"` // new | changed | eol | same | removed | extra | protected | ignored
	Local   bool   `json:"local,omitempty"`
	Link    bool   `json:"link,omitempty"`
	Behind  int    `json:"behind,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Pattern string `json:"pattern,omitempty"` // ignored: why
	hash    string
	cr      int
}

// gitEnvConflict is a path the branch turns from a directory into a file or back.
type gitEnvConflict struct {
	Path   string   `json:"path"`
	Kind   string   `json:"kind"`   // dir_to_file | file_to_dir
	Server []string `json:"server"` // server files that must be deleted
}

type gitEnvDestPlan struct {
	Index       int              `json:"index"`
	Server      string           `json:"server"`
	ConnID      int              `json:"conn_id"`
	Path        string           `json:"path"`
	StateDir    string           `json:"state_dir"`
	Exists      bool             `json:"exists"`
	Files       []gitEnvFile     `json:"files"`
	Counts      map[string]int   `json:"counts"`
	Conflicts   []gitEnvConflict `json:"conflicts,omitempty"`
	Fingerprint string           `json:"fingerprint"`
	Current     *gitEnvRecord    `json:"current,omitempty"`
	FromVersion string           `json:"from_version,omitempty"` // VERSION.md
	Lock        *gitEnvLock      `json:"lock,omitempty"`
	Errors      []string         `json:"errors"`
	Warnings    []string         `json:"warnings"`
	vmdCR       bool
	connBinding string // safe connection settings captured during comparison
	vmdExists   bool
	state       *gitEnvState
}

type gitEnvExclusion struct {
	By string `json:"by"`
	At string `json:"at"`
}

type gitEnvPlan struct {
	ID         string                     `json:"id"`
	App        string                     `json:"app"`
	Env        string                     `json:"env"`
	Ref        string                     `json:"ref,omitempty"`
	RefName    string                     `json:"ref_name"`
	Target     gitTargetBrief             `json:"target"`
	Confirm    bool                       `json:"confirm"`
	PostDeploy string                     `json:"post_deploy,omitempty"`
	Dests      []*gitEnvDestPlan          `json:"dests"`
	Excluded   map[string]gitEnvExclusion `json:"excluded"` // remembered per-run exclusions
	Errors     []string                   `json:"errors"`
	CreatedAt  string                     `json:"created_at"`
	Used       bool                       `json:"used"`
	userID     int
	created    time.Time
	target     gitTarget
}

var gitEnvPlans = struct {
	sync.Mutex
	m map[string]*gitEnvPlan
}{m: map[string]*gitEnvPlan{}}

func randomID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func storeEnvPlan(p *gitEnvPlan) {
	gitEnvPlans.Lock()
	defer gitEnvPlans.Unlock()
	for k, x := range gitEnvPlans.m {
		if time.Since(x.created) > gitEnvPlanTTL {
			delete(gitEnvPlans.m, k)
		}
	}
	gitEnvPlans.m[p.ID] = p
}

// takeEnvPlan returns a stored plan; with use it marks it applied (one deploy per plan).
func takeEnvPlan(userID int, id string, use bool) (*gitEnvPlan, error) {
	gitEnvPlans.Lock()
	defer gitEnvPlans.Unlock()
	p := gitEnvPlans.m[id]
	if p == nil || p.userID != userID || time.Since(p.created) > gitEnvPlanTTL {
		return nil, fmt.Errorf("the plan is not known any more: compare again")
	}
	if p.Used {
		return nil, fmt.Errorf("this plan was applied already: compare again for a new deploy")
	}
	if use {
		p.Used = true
	}
	return p, nil
}

// envScanScript lists the files of an app directory (hash with CR removed, number of CRs,
// symlinks, unreadable files), VERSION.md, state.json and the lock.
func envScanScript(dir, stateDir string, ignore []string) string {
	names := append([]string(nil), gitPruneNames...)
	names = append(names, ".wrm-incoming-*")
	for _, g := range ignore {
		if !strings.Contains(g, "/") {
			names = append(names, g)
		}
	}
	var sb strings.Builder
	sb.WriteString(gitHashPrelude)
	fmt.Fprintf(&sb, "D=%s; SD=%s\n", shellQuote(dir), shellQuote(stateDir))
	sb.WriteString(`[ -L "$D" ] && echo RS
if [ -e "$D" ] && [ ! -d "$D" ]; then printf 'E\t%s is not a directory\n' "$D"; exit 0; fi
if [ ! -d "$D" ]; then echo X; else
cd "$D" || { printf 'E\tcannot enter %s\n' "$D"; exit 0; }
`)
	fmt.Fprintf(&sb, "find . -type d %s -prune -o \\( -type f -o -type l \\) -print 2>/dev/null | head -n %d | while IFS= read -r f; do\n", findNames(names), gitMaxInstallFiles+1)
	sb.WriteString(`  rel=${f#./}
  if [ -L "$f" ]; then printf 'L\t%s\n' "$rel"
  elif [ -r "$f" ]; then h=$(tr -d '\r' < "$f" | $S | cut -c1-64); c=$(tr -cd '\r' < "$f" | wc -c); printf 'F\t%s\t%s\t%s\n' "$h" "${c##* }" "$rel"
  else printf 'R\t%s\n' "$rel"; fi
done
[ -f VERSION.md ] && head -n 40 VERSION.md | sed 's/^/V\t/'
fi
[ -f "$SD/state.json" ] && { printf 'J\t'; tr -d '\n\r' < "$SD/state.json"; echo; }
`)
	sb.WriteString(envLockRead)
	sb.WriteString("echo WRM_DONE\n")
	return sb.String()
}

type envServerFile struct {
	hash       string
	cr         int
	link       bool
	unreadable bool
}

type envScan struct {
	missing   bool
	rootLink  bool
	files     map[string]envServerFile
	version   []string
	state     *gitEnvState
	lock      *gitEnvLock
	capped    bool
	vmdCR     bool
	vmdExists bool
}

func runEnvScan(cl *ssh.Client, dir, stateDir string, ignore []string) (*envScan, error) {
	out, err := runRemoteScript(cl, envScanScript(dir, stateDir, ignore), "", gitScanTimeout)
	if err != nil {
		return nil, err
	}
	if e := scriptError(out); e != nil {
		return nil, e
	}
	if !strings.Contains(out, "WRM_DONE") {
		return nil, fmt.Errorf("the scan did not finish")
	}
	s := &envScan{files: map[string]envServerFile{}}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 4)
		switch {
		case line == "X":
			s.missing = true
		case line == "RS":
			s.rootLink = true
		case parts[0] == "F" && len(parts) == 4:
			if len(s.files) >= gitMaxInstallFiles {
				s.capped = true
				continue
			}
			c, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
			s.files[parts[3]] = envServerFile{hash: parts[1], cr: c}
		case parts[0] == "L" && len(parts) >= 2:
			s.files[strings.TrimPrefix(line, "L\t")] = envServerFile{link: true, hash: "link"}
		case parts[0] == "R" && len(parts) >= 2:
			s.files[strings.TrimPrefix(line, "R\t")] = envServerFile{unreadable: true, hash: "-"}
		case parts[0] == "V" && len(parts) >= 2:
			s.version = append(s.version, strings.TrimPrefix(line, "V\t"))
		case parts[0] == "J" && len(parts) >= 2:
			var st gitEnvState
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "J\t")), &st) == nil {
				s.state = &st
			}
		}
	}
	if f, ok := s.files["VERSION.md"]; ok {
		s.vmdExists, s.vmdCR = true, f.cr > 0
	}
	s.lock = parseLock(out)
	return s, nil
}

// envPlanCtx carries what the plans of one service share.
type envPlanCtx struct {
	userID    int
	cat       *gitCatalog
	app       *gitCatalogApp
	env       *gitEnvironment
	t         gitTarget
	providers map[int]*gitProvider
	trees     map[string]map[string]bool
	words     []string
}

func newEnvPlanCtx(userID int, cat *gitCatalog, app *gitCatalogApp, env *gitEnvironment, t gitTarget) *envPlanCtx {
	return &envPlanCtx{userID: userID, cat: cat, app: app, env: env, t: t, providers: map[int]*gitProvider{}, trees: map[string]map[string]bool{}, words: backupWords()}
}

// ignoredBy tells why a file is left alone ("" = it is deployed / compared).
func (x *envPlanCtx) ignoredBy(rel string) string {
	switch {
	case toolFiles[rel]:
		return rel
	case strings.HasSuffix(rel, ".pyc"):
		return "*.pyc"
	case path.Dir(rel) != "." && ignoredPath(path.Dir(rel), x.cat.IgnoreDirs, x.words):
		return "ignore_dirs"
	case len(x.app.Include) > 0 && !globHitDir(rel, x.app.Include):
		return "include"
	}
	if g := firstGlob(rel, x.app.Exclude, true); g != "" {
		return g
	}
	if g := firstGlob(rel, x.env.Ignore, true); g != "" {
		return g
	}
	return ""
}

func (x *envPlanCtx) protected(rel string) bool {
	return globHit(rel, x.t.Protected) || globHit(rel, x.app.Protected)
}

// previousTree is the repository at the deployed commit (relative to the subdirectory), so
// files removed from the repository can be told from local files; nil when not known.
func (x *envPlanCtx) previousTree(st *gitEnvState) map[string]bool {
	if st == nil || st.Current == nil || len(st.Current.Commit) < 40 {
		return nil
	}
	commit := st.Current.Commit
	if m, ok := x.trees[commit]; ok {
		return m
	}
	x.trees[commit] = nil
	src, found := loadGitSource(x.userID, x.t.SourceID)
	if !found || src.Kind == "bundle" {
		return nil
	}
	p := x.providers[src.ID]
	if p == nil {
		var err error
		if p, err = newGitProvider(src); err != nil {
			return nil
		}
		x.providers[src.ID] = p
	}
	tree, err := p.tree(src.ID, x.t.Project, commit)
	if err != nil {
		return nil
	}
	prefix := ""
	if x.t.Subdir != "" {
		prefix = x.t.Subdir + "/"
	}
	m := map[string]bool{}
	for p := range tree {
		if strings.HasPrefix(p, prefix) && len(p) > len(prefix) {
			m[p[len(prefix):]] = true
		}
	}
	x.trees[commit] = m
	return m
}

// repoCR tells whether the repository's content of a file has CRs (from the blob cache;
// fetched when the server file has CRs, so a CRLF/LF-only difference is found).
func (x *envPlanCtx) repoCR(rel string, serverCR bool) bool {
	f := x.t.Files[rel]
	if data, ok := blobContent(x.t.SourceID, f.Blob); ok {
		return bytes.IndexByte(data, '\r') >= 0
	}
	if !serverCR {
		return false
	}
	data, err := gitFileContent(x.userID, x.t, rel, x.providers)
	return err == nil && bytes.IndexByte(data, '\r') >= 0
}

// destPlan compares one destination with the target.
func (x *envPlanCtx) destPlan(cl *ssh.Client, idx int, d gitEnvDest, conn Connection) (*gitEnvDestPlan, error) {
	dp := &gitEnvDestPlan{Index: idx, Server: d.Server, ConnID: conn.ID, Path: d.Path, StateDir: envStateDir(*x.env, x.app.Name, d.Path),
		Files: []gitEnvFile{}, Counts: map[string]int{}, Errors: []string{}, Warnings: []string{}}
	dp.connBinding = gitEnvConnBinding(conn)
	sc, err := runEnvScan(cl, d.Path, dp.StateDir, x.cat.IgnoreDirs)
	if err != nil {
		return dp, err
	}
	dp.Exists, dp.Lock, dp.state, dp.vmdCR, dp.vmdExists = !sc.missing, sc.lock, sc.state, sc.vmdCR, sc.vmdExists
	if sc.state != nil {
		dp.Current = sc.state.Current
	}
	dp.FromVersion = parseVersionMD(sc.version).Version
	if sc.capped {
		dp.Errors = append(dp.Errors, fmt.Sprintf("the directory has more than %d files", gitMaxInstallFiles))
	}
	if sc.rootLink {
		dp.Warnings = append(dp.Warnings, d.Path+" is a symlink on the server")
	}
	if sc.missing {
		dp.Warnings = append(dp.Warnings, "first deploy: "+d.Path+" does not exist yet and is created with its parents")
	}
	prev := x.previousTree(sc.state)
	var links, local []string
	for rel, f := range x.t.Files {
		ef := gitEnvFile{Path: rel}
		sf, on := sc.files[rel]
		ef.Link, ef.hash, ef.cr = sf.link, sf.hash, sf.cr
		switch {
		case strings.ContainsAny(rel, "\n\r"):
			dp.Errors = append(dp.Errors, fmt.Sprintf("%q: file names with line breaks cannot be deployed", rel))
			continue
		case x.protected(rel):
			ef.State = "protected"
		case x.ignoredBy(rel) != "":
			ef.State, ef.Pattern = "ignored", x.ignoredBy(rel)
		case !on:
			ef.State = "new"
		case sf.link:
			ef.State = "changed"
			links = append(links, rel)
		case sf.unreadable:
			ef.State, ef.Local = "changed", true
			local = append(local, rel)
		case matchHash(sf.hash, f.Hash, f.Hash2):
			ef.State = "same"
			if (sf.cr > 0) != x.repoCR(rel, sf.cr > 0) {
				ef.State = "eol"
			}
		default:
			ef.State = "changed"
			st, behind, tag, _ := trackedState(f, sf.hash)
			if st == "modified" {
				ef.Local = true
				local = append(local, rel)
			} else {
				ef.Behind, ef.Tag = behind, tag
			}
		}
		dp.Files = append(dp.Files, ef)
	}
	// type changes: a directory of the server becomes a file of the branch or back
	conflictFiles := map[string]bool{}
	for rel := range x.t.Files {
		var under []string
		for srel := range sc.files {
			if strings.HasPrefix(srel, rel+"/") {
				under = append(under, srel)
			}
		}
		if len(under) > 0 {
			sort.Strings(under)
			dp.Conflicts = append(dp.Conflicts, gitEnvConflict{Path: rel, Kind: "dir_to_file", Server: under})
			for _, u := range under {
				conflictFiles[u] = true
			}
		}
		for p := path.Dir(rel); p != "." && p != "/"; p = path.Dir(p) {
			if _, isFile := sc.files[p]; isFile {
				if !conflictFiles[p] {
					dp.Conflicts = append(dp.Conflicts, gitEnvConflict{Path: p, Kind: "file_to_dir", Server: []string{p}})
				}
				conflictFiles[p] = true
			}
		}
	}
	sort.Slice(dp.Conflicts, func(i, j int) bool { return dp.Conflicts[i].Path < dp.Conflicts[j].Path })
	for rel, sf := range sc.files {
		if _, inTarget := x.t.Files[rel]; inTarget {
			continue
		}
		ef := gitEnvFile{Path: rel, Link: sf.link, hash: sf.hash, cr: sf.cr}
		switch {
		case strings.ContainsAny(rel, "\n\r"):
			continue
		case conflictFiles[rel]:
			ef.State = "removed"
		case x.protected(rel):
			ef.State = "protected"
		case x.ignoredBy(rel) != "":
			continue // ignored server-only files are left alone and not listed
		case prev != nil && prev[rel]:
			ef.State = "removed"
		default:
			ef.State = "extra"
		}
		dp.Files = append(dp.Files, ef)
	}
	sort.Slice(dp.Files, func(i, j int) bool { return dp.Files[i].Path < dp.Files[j].Path })
	for _, f := range dp.Files {
		dp.Counts[f.State]++
	}
	if len(links) > 0 {
		sort.Strings(links)
		dp.Warnings = append(dp.Warnings, "replaces a symlink on the server: "+strings.Join(capList(links, 10), ", "))
	}
	if len(local) > 0 {
		sort.Strings(local)
		dp.Warnings = append(dp.Warnings, fmt.Sprintf("%d file(s) were changed on the server by hand: %s", len(local), strings.Join(capList(local, 10), ", ")))
	}
	if dp.Lock != nil {
		msg := fmt.Sprintf("locked by %s from %s (deploy %s, %s ago)", dp.Lock.User, dp.Lock.From, dp.Lock.ID, (time.Duration(dp.Lock.Age) * time.Second).String())
		if dp.Lock.Stale {
			msg += ": stale"
		}
		dp.Warnings = append(dp.Warnings, msg)
	}
	dp.Fingerprint = x.fingerprint(dp)
	return dp, nil
}

func capList(l []string, n int) []string {
	if len(l) <= n {
		return l
	}
	return append(append([]string{}, l[:n]...), fmt.Sprintf("+%d", len(l)-n))
}

// fingerprint covers what the plan compared: the destination, its current deploy, the
// target commit and every listed file with its server hash (not whether the directory
// exists: the lock creates the state directory, and a missing directory has no files).
func (x *envPlanCtx) fingerprint(dp *gitEnvDestPlan) string {
	h := sha256.New()
	cur := ""
	if dp.Current != nil {
		cur = dp.Current.DeployID
	}
	fmt.Fprintf(h, "%s\n%d\n%s\n%s\n%s\n%s\n%s\n", strings.ToLower(dp.Server), dp.ConnID, dp.connBinding, dp.Path, cur, x.t.CommitFull, x.t.Commit)
	for _, f := range dp.Files {
		fmt.Fprintf(h, "%s\t%s\t%s\t%d\t%v\n", f.Path, f.State, f.hash, f.cr, f.Link)
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// Bind reviewed destinations to connection identity, login and effective route without
// including secrets. Renaming/changing a connection requires a new comparison.
func gitEnvConnBinding(c Connection) string {
	settings := struct {
		ID, UserID                     int
		Name, Protocol, Host, Username string
		JumpID, ProxyID                *int
	}{c.ID, c.UserID, c.Name, c.Protocol, c.Host, c.Username, c.JumpID, c.ProxyID}
	b, _ := json.Marshal(settings)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// envSelection is what a deploy does on one destination: the files to write and to
// delete. Excluding a file the branch's type change needs is an error.
func envSelection(dp *gitEnvDestPlan, exclude map[string]bool, deleteRemoved bool) ([]string, []string, error) {
	var writes, deletes []string
	for _, f := range dp.Files {
		if exclude[f.Path] {
			continue
		}
		switch f.State {
		case "new", "changed", "eol":
			writes = append(writes, f.Path)
		case "removed":
			if deleteRemoved {
				deletes = append(deletes, f.Path)
			}
		}
	}
	del := map[string]bool{}
	for _, d := range deletes {
		del[d] = true
	}
	for _, c := range dp.Conflicts {
		for _, s := range c.Server {
			if del[s] {
				continue
			}
			what := "a directory into a file"
			if c.Kind == "file_to_dir" {
				what = "a file into a directory"
			}
			if exclude[s] {
				return nil, nil, fmt.Errorf("%s: the branch turns %s at %s; excluding %s is not possible", dp.Server, what, c.Path, s)
			}
			return nil, nil, fmt.Errorf("%s: the branch turns %s at %s; %s must be deleted (choose to delete removed files)", dp.Server, what, c.Path, s)
		}
	}
	return writes, deletes, nil
}

// ─── exclusions remembered per environment ───────────

func initGitEnvSchema() {
	ensureTable("git_env_exclusions", `CREATE TABLE IF NOT EXISTS git_env_exclusions (
		user_id INTEGER NOT NULL,
		app TEXT NOT NULL,
		env TEXT NOT NULL,
		path TEXT NOT NULL,
		by_user TEXT NOT NULL DEFAULT '',
		at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (user_id, app, env, path))`,
		"user_id", "app", "env", "path", "by_user", "at")
	ensureTable("git_hostnames", `CREATE TABLE IF NOT EXISTS git_hostnames (
		conn_id INTEGER PRIMARY KEY,
		hostname TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`,
		"conn_id", "hostname", "updated_at")
}

func loadEnvExclusions(userID int, app, env string) map[string]gitEnvExclusion {
	out := map[string]gitEnvExclusion{}
	rows, err := db.Query(`SELECT path, by_user, at FROM git_env_exclusions WHERE user_id=? AND app=? AND env=?`, userID, app, env)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		var x gitEnvExclusion
		rows.Scan(&p, &x.By, &x.At)
		out[p] = x
	}
	return out
}

// saveEnvExclusions remembers the exclusions of a deploy (kept with their first who / when).
func saveEnvExclusions(userID int, app, env string, paths []string) {
	old := loadEnvExclusions(userID, app, env)
	db.Exec(`DELETE FROM git_env_exclusions WHERE user_id=? AND app=? AND env=?`, userID, app, env)
	who, now := usernameOf(userID), nowStamp()
	for _, p := range paths {
		x, had := old[p]
		if !had {
			x = gitEnvExclusion{By: who, At: now}
		}
		db.Exec(`INSERT OR REPLACE INTO git_env_exclusions (user_id, app, env, path, by_user, at) VALUES (?,?,?,?,?,?)`, userID, app, env, p, x.By, x.At)
	}
}

// addPermanentExclude writes an anchored, escaped pattern for one file to the service's
// excludes (scope service) or the environment's ignore list (scope env), unless a pattern
// already covers it.
func addPermanentExclude(userID int, app, env, rel, scope string) (string, bool, error) {
	rel = cleanRel(rel)
	if rel == "" || strings.HasPrefix(rel, "..") {
		return "", false, fmt.Errorf("invalid path")
	}
	cat := loadGitCatalog(userID)
	a, e, err := findEnv(cat, app, env)
	if err != nil && (scope == "env" || a == nil) {
		return "", false, err
	}
	pattern := "/" + globEscape(rel)
	switch scope {
	case "service":
		if globHitDir(rel, a.Exclude) {
			return firstGlob(rel, a.Exclude, true), false, nil
		}
		a.Exclude = append(a.Exclude, pattern)
	case "env":
		if globHitDir(rel, a.Exclude) {
			return firstGlob(rel, a.Exclude, true), false, nil
		}
		if globHitDir(rel, e.Ignore) {
			return firstGlob(rel, e.Ignore, true), false, nil
		}
		e.Ignore = append(e.Ignore, pattern)
	default:
		return "", false, fmt.Errorf("unknown scope")
	}
	if err := cat.normalize(); err != nil {
		return "", false, err
	}
	saveGitWorkspace(userID, cat, nil)
	return pattern, true, nil
}

// ─── computing a plan ────────────────────────────────

// envPlanRequest computes the plan of an environment at a ref ("" = the service's target).
func envPlanRequest(userID int, app, env, ref string) (*gitEnvPlan, int, error) {
	cat := envCatalog(userID, app, env)
	a, e, err := findEnv(cat, app, env)
	if err != nil {
		return nil, 404, err
	}
	key := "git_update"
	if strings.TrimSpace(ref) != "" {
		key = "git_upgrade"
	}
	if !gitActionAllowed(userID, key) {
		return nil, 403, fmt.Errorf("this action is not allowed for your account")
	}
	t, err := installTarget(userID, app, ref)
	if err != nil {
		return nil, 400, err
	}
	p := &gitEnvPlan{ID: randomID(12), App: app, Env: env, Ref: strings.TrimSpace(ref), RefName: refName(t), Target: t.brief(), Confirm: e.needsConfirm(),
		PostDeploy: e.PostDeploy, Dests: []*gitEnvDestPlan{}, Errors: []string{}, CreatedAt: nowStamp(), userID: userID, created: time.Now(), target: t,
		Excluded: loadEnvExclusions(userID, app, env)}
	if !refAllowed(t, e.AllowedRefs) {
		p.Errors = append(p.Errors, fmt.Sprintf("%s is not allowed in %s (allowed: %s)", p.RefName, env, strings.Join(e.AllowedRefs, ", ")))
	}
	results := make([]*gitEnvDestPlan, len(e.Destinations))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, d := range e.Destinations {
		wg.Add(1)
		go func(i int, d gitEnvDest) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = envDestPlanOver(userID, newEnvPlanCtx(userID, cat, a, e, t), i, d) // a context per goroutine (caches)
		}(i, d)
	}
	wg.Wait()
	p.Dests = results
	storeEnvPlan(p)
	return p, 200, nil
}

// envDestPlanOver logs in to a destination and computes its plan; errors are kept in it.
func envDestPlanOver(userID int, x *envPlanCtx, i int, d gitEnvDest) *gitEnvDestPlan {
	fail := func(err error) *gitEnvDestPlan {
		return &gitEnvDestPlan{Index: i, Server: d.Server, Path: d.Path, StateDir: envStateDir(*x.env, x.app.Name, d.Path), Files: []gitEnvFile{},
			Counts: map[string]int{}, Errors: []string{err.Error()}, Warnings: []string{}}
	}
	conn, err := envConn(userID, d.Server)
	if err != nil {
		return fail(err)
	}
	cl, err := dialSSH(conn, nil)
	if err != nil {
		return fail(fmt.Errorf("cannot log in to %s: %v", conn.Name, err))
	}
	defer cl.Close()
	cacheServerHostname(conn.ID, cl)
	dp, err := x.destPlan(cl, i, d, conn)
	if err != nil {
		dp.Errors = append(dp.Errors, err.Error())
	}
	return dp
}

// ─── reading destinations (overview, history, doctor) ─

type envDestCtx struct {
	idx   int
	d     gitEnvDest
	state string
	conn  Connection
	cl    *ssh.Client
}

// forEachDest logs in to every destination (a few at a time) and calls fn; a login error is
// passed as err.
func forEachDest(userID int, app *gitCatalogApp, e *gitEnvironment, fn func(dc envDestCtx, err error)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, d := range e.Destinations {
		wg.Add(1)
		go func(i int, d gitEnvDest) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			dc := envDestCtx{idx: i, d: d, state: envStateDir(*e, app.Name, d.Path)}
			conn, err := envConn(userID, d.Server)
			if err != nil {
				fn(dc, err)
				return
			}
			dc.conn = conn
			cl, err := dialSSH(conn, nil)
			if err != nil {
				fn(dc, fmt.Errorf("cannot log in to %s: %v", conn.Name, err))
				return
			}
			defer cl.Close()
			cacheServerHostname(conn.ID, cl)
			dc.cl = cl
			fn(dc, nil)
		}(i, d)
	}
	wg.Wait()
}

// gitEnvCellDest is one destination of an overview cell.
type gitEnvCellDest struct {
	Server   string        `json:"server"`
	Path     string        `json:"path"`
	Exists   bool          `json:"exists"`
	Current  *gitEnvRecord `json:"current,omitempty"`
	VersionM string        `json:"version_md,omitempty"`
	CommitMD string        `json:"commit_md,omitempty"`
	Detected gitDetected   `json:"detected"`
	CodeErr  string        `json:"code_error,omitempty"`
	CodeKey  string        `json:"-"`
	RefMD    string        `json:"ref_md,omitempty"`     // Grana/ref of VERSION.md
	UserMD   string        `json:"user_md,omitempty"`    // who wrote VERSION.md
	UpdateMD string        `json:"updated_md,omitempty"` // and when
	Behind   int           `json:"behind"`               // -1 = not known
	Lock     *gitEnvLock   `json:"lock,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// envCell reads the state of every destination of an environment.
func envCell(userID int, app, env string) (map[string]interface{}, error) {
	cat := envCatalog(userID, app, env)
	a, e, err := findEnv(cat, app, env)
	if err != nil {
		return nil, err
	}
	out := make([]gitEnvCellDest, len(e.Destinations))
	t, haveT := loadGitTarget(userID, app)
	forEachDest(userID, a, e, func(dc envDestCtx, err error) {
		cd := gitEnvCellDest{Server: dc.d.Server, Path: dc.d.Path, Behind: -1}
		if err == nil {
			script := fmt.Sprintf("D=%s; SD=%s\n[ -d \"$D\" ] && echo XD\n[ -f \"$D/VERSION.md\" ] && head -n 12 \"$D/VERSION.md\" | sed 's/^/V\t/'\n[ -f \"$SD/state.json\" ] && { printf 'J\\t'; tr -d '\\n\\r' < \"$SD/state.json\"; echo; }\n%secho WRM_DONE\n",
				shellQuote(dc.d.Path), shellQuote(dc.state), envLockRead)
			var o string
			o, err = runRemoteScript(dc.cl, script, "", gitStepTimeout)
			if err == nil {
				var vl []string
				for _, line := range strings.Split(o, "\n") {
					switch {
					case line == "XD":
						cd.Exists = true
					case strings.HasPrefix(line, "V\t"):
						vl = append(vl, line[2:])
					case strings.HasPrefix(line, "J\t"):
						var st gitEnvState
						if json.Unmarshal([]byte(line[2:]), &st) == nil {
							cd.Current = st.Current
						}
					}
				}
				v := parseVersionMD(vl)
				cd.VersionM, cd.CommitMD = v.Version, strings.TrimSpace(strings.Split(v.Commit, " ")[0])
				cd.RefMD, cd.UserMD, cd.UpdateMD = v.Ref, v.User, v.Updated
				cd.Lock = parseLock(o)
				if cd.Exists && haveT && t.Error == "" && len(t.Files) > 0 {
					if scans, serr := scanFilesOn(dc.cl, []string{dc.d.Path}); serr != nil {
						cd.CodeErr = serr.Error()
						cd.Detected = gitDetected{State: "unknown"}
					} else if s := scans[dc.d.Path]; s != nil && !s.Missing {
						files, _, _ := compareInstall(t, s.Files, cat.IgnoreDirs)
						cd.Detected = detectInstallVersion(t, s.Files, files)
						cd.CodeKey = fileStateKey(files)
						if s.Capped {
							cd.Detected = gitDetected{State: "unknown"}
						}
					} else {
						cd.Detected = gitDetected{State: "unknown"}
					}
				} else if cd.Exists {
					cd.Detected = gitDetected{State: "unknown"}
				}
			}
		}
		if err != nil {
			cd.Error = err.Error()
		}
		out[dc.idx] = cd
	})
	// commits behind the service's target (one compare per distinct commit)
	var p *gitProvider
	if haveT && t.Error == "" && t.CommitFull != "" {
		if src, found := loadGitSource(userID, t.SourceID); found && src.Kind != "bundle" {
			p, _ = newGitProvider(src)
		}
	}
	behind := map[string]int{}
	seen := map[string]bool{}
	drift := false
	for i := range out {
		cd := &out[i]
		commit, version, key := "", "", ""
		switch {
		case cd.Detected.State == "exact":
			commit, version = cd.Detected.Commit, cd.Detected.Version
			key = "exact|" + version + "|" + commit
		case cd.Detected.State != "":
			key = cd.Detected.State + "|" + cd.CodeKey
		default:
			// no code check (no target yet, or an old check): use the deploy record
			version, commit = cd.VersionM, cd.CommitMD
			if cd.Current != nil {
				version, commit = cd.Current.Version, cd.Current.Commit
			}
			key = "meta|" + version + "|" + commit
		}
		if cd.Error == "" && cd.Exists {
			seen[key] = true
		}
		if commit == "" {
			continue
		}
		if commit != "" && ((t.CommitFull != "" && (strings.HasPrefix(t.CommitFull, commit) || strings.HasPrefix(commit, t.CommitFull))) ||
			(t.Commit != "" && (strings.HasPrefix(t.Commit, commit) || strings.HasPrefix(commit, t.Commit)))) {
			cd.Behind = 0
			continue
		}
		if p == nil {
			continue
		}
		n, ok := behind[commit]
		if !ok {
			n = -1
			if c, _, err := p.compareCommits(t.Project, commit, t.CommitFull); err == nil {
				n = c
			}
			behind[commit] = n
		}
		cd.Behind = n
	}
	drift = len(seen) > 1
	res := map[string]interface{}{"app": app, "env": env, "dests": out, "drift": drift, "hosts": hostLabels(userID)}
	if haveT {
		res["target"] = t.brief()
	}
	return res, nil
}

// envHistory merges history.jsonl of every destination (newest first, one row per
// destination) and returns the current deploy of each destination.
func envHistory(userID int, app, env string) (map[string]interface{}, error) {
	cat := envCatalog(userID, app, env)
	a, e, err := findEnv(cat, app, env)
	if err != nil {
		return nil, err
	}
	type destInfo struct {
		Server  string `json:"server"`
		Path    string `json:"path"`
		Current string `json:"current,omitempty"` // deploy id
		Error   string `json:"error,omitempty"`
	}
	dests := make([]destInfo, len(e.Destinations))
	var mu sync.Mutex
	byKey := map[string]map[string]interface{}{}
	forEachDest(userID, a, e, func(dc envDestCtx, err error) {
		di := destInfo{Server: dc.d.Server, Path: dc.d.Path}
		if err == nil {
			script := fmt.Sprintf("SD=%s\n[ -f \"$SD/state.json\" ] && { printf 'J\\t'; tr -d '\\n\\r' < \"$SD/state.json\"; echo; }\n[ -f \"$SD/history.jsonl\" ] && tail -n 300 \"$SD/history.jsonl\" | sed 's/^/H\t/'\necho WRM_DONE\n", shellQuote(dc.state))
			var o string
			o, err = runRemoteScript(dc.cl, script, "", gitStepTimeout)
			if err == nil {
				for _, line := range strings.Split(o, "\n") {
					switch {
					case strings.HasPrefix(line, "J\t"):
						var st gitEnvState
						if json.Unmarshal([]byte(line[2:]), &st) == nil && st.Current != nil {
							di.Current = st.Current.DeployID
						}
					case strings.HasPrefix(line, "H\t"):
						entry := map[string]interface{}{}
						if json.Unmarshal([]byte(line[2:]), &entry) != nil {
							continue
						}
						id, _ := entry["id"].(string)
						action, _ := entry["action"].(string)
						// one row per destination: the file lists differ between servers
						key := fmt.Sprintf("%s|%s|%d|%s", action, id, dc.idx, entry["at"])
						entry["dest"], entry["server"], entry["path"] = dc.idx, dc.d.Server, dc.d.Path
						mu.Lock()
						byKey[key] = entry
						mu.Unlock()
					}
				}
			}
		}
		if err != nil {
			di.Error = err.Error()
		}
		dests[dc.idx] = di
	})
	list := make([]map[string]interface{}, 0, len(byKey))
	for _, v := range byKey {
		list = append(list, v)
	}
	sort.SliceStable(list, func(i, j int) bool {
		ai, _ := list[i]["at"].(string)
		aj, _ := list[j]["at"].(string)
		if ai[:min(len(ai), 16)] != aj[:min(len(aj), 16)] {
			return ai > aj // newest first (to the minute), then in destination order
		}
		di, _ := list[i]["dest"].(int)
		dj, _ := list[j]["dest"].(int)
		if di != dj {
			return di < dj
		}
		return ai > aj
	})
	return map[string]interface{}{"app": app, "env": env, "dests": dests, "entries": list}, nil
}

// ─── access doctor ───────────────────────────────────

type gitDoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | fail | info
	Msg    string `json:"msg"`
}

type gitDoctorDest struct {
	Server string           `json:"server"`
	Path   string           `json:"path"`
	Checks []gitDoctorCheck `json:"checks"`
}

var publicWebRoots = []string{"/var/www", "/srv/www", "/usr/share/nginx", "/usr/share/httpd", "/var/lib/nginx"}
var publicWebSegments = map[string]bool{"htdocs": true, "public_html": true, "wwwroot": true, "html": true, "public": true, "webroot": true}

func looksPublic(p string) bool {
	for _, r := range publicWebRoots {
		if within(p, r) {
			return true
		}
	}
	for _, seg := range strings.Split(p, "/") {
		if publicWebSegments[strings.ToLower(seg)] {
			return true
		}
	}
	return false
}

// doctorScript checks one destination: a clean shell (the first line must be the marker),
// the user, the directory or its nearest existing parent, the state directory, unreadable
// subdirectories, signs of another deploy method and the tools.
func doctorScript(dir, stateDir string) string {
	return fmt.Sprintf(`echo WRM_BEGIN
D=%s; SD=%s
printf 'U\t%%s\t%%s\n' "$(id -u 2>/dev/null)" "$(id -un 2>/dev/null || whoami)"
if [ -d "$D" ]; then printf 'X\t1\n'; [ -w "$D" ] && printf 'W\t1\n' || printf 'W\t0\n'
  [ -L "$D" ] && printf 'SIGN\tsymlink\n'
  for s in .git .svn .hg releases current .deploy-bak; do [ -e "$D/$s" ] && printf 'SIGN\t%%s\n' "$s"; done
  find "$D" -type d 2>&1 >/dev/null | grep -i 'permission denied' | head -n 5 | sed 's/^/UR\t/'
else P=$D; while [ ! -d "$P" ]; do P=${P%%/*}; [ -n "$P" ] || P=/; done
  printf 'X\t0\nP\t%%s\n' "$P"; [ -w "$P" ] && printf 'W\t1\n' || printf 'W\t0\n'; fi
if [ -d "$SD" ]; then [ -w "$SD" ] && printf 'S\tw\n' || printf 'S\tro\n'
else Q=$SD; while [ ! -d "$Q" ]; do Q=${Q%%/*}; [ -n "$Q" ] || Q=/; done; [ -w "$Q" ] && printf 'S\tcan\t%%s\n' "$Q" || printf 'S\tno\t%%s\n' "$Q"; fi
for x in rsync sha256sum tar; do if command -v $x >/dev/null 2>&1; then printf 'T\t%%s\t%%s\n' $x "$($x --version 2>&1 | head -n 1)"; else printf 'T\t%%s\t\n' $x; fi; done
%secho WRM_DONE
`, shellQuote(dir), shellQuote(stateDir), strings.ReplaceAll(envLockRead, "%", "%%"))
}

func envDoctor(userID int, app, env string) ([]gitDoctorDest, error) {
	cat := envCatalog(userID, app, env)
	a, e, err := findEnv(cat, app, env)
	if err != nil {
		return nil, err
	}
	out := make([]gitDoctorDest, len(e.Destinations))
	forEachDest(userID, a, e, func(dc envDestCtx, err error) {
		dd := gitDoctorDest{Server: dc.d.Server, Path: dc.d.Path, Checks: []gitDoctorCheck{}}
		add := func(name, status, msg string) {
			dd.Checks = append(dd.Checks, gitDoctorCheck{Name: name, Status: status, Msg: msg})
		}
		defer func() { out[dc.idx] = dd }()
		if err != nil {
			add("ssh", "fail", err.Error())
			return
		}
		add("ssh", "ok", dc.conn.Name)
		o, err := runRemoteScript(dc.cl, doctorScript(dc.d.Path, dc.state), "", gitStepTimeout)
		if err != nil {
			add("shell", "fail", err.Error())
			return
		}
		if !strings.HasPrefix(o, "WRM_BEGIN\n") {
			before, _, _ := strings.Cut(o, "WRM_BEGIN")
			add("shell", "warn", "the shell prints text before the commands (banner or rc output): "+truncateStr(strings.TrimSpace(before), 200))
		} else {
			add("shell", "ok", "clean non-interactive shell")
		}
		var exists, writable bool
		parent, stateSt, stateAt := "", "", ""
		var signs, unreadable []string
		tools := map[string]string{}
		for _, line := range strings.Split(o, "\n") {
			parts := strings.SplitN(line, "\t", 3)
			switch {
			case parts[0] == "U" && len(parts) == 3:
				if parts[1] == "0" {
					add("user", "warn", "connects as root: a deploy user that owns the app directory is safer")
				} else {
					add("user", "ok", parts[2])
				}
			case parts[0] == "X" && len(parts) >= 2:
				exists = parts[1] == "1"
			case parts[0] == "P" && len(parts) >= 2:
				parent = parts[1]
			case parts[0] == "W" && len(parts) >= 2:
				writable = parts[1] == "1"
			case parts[0] == "SIGN" && len(parts) >= 2:
				signs = append(signs, parts[1])
			case parts[0] == "UR" && len(parts) >= 2:
				unreadable = append(unreadable, strings.TrimPrefix(line, "UR\t"))
			case parts[0] == "S" && len(parts) >= 2:
				stateSt = parts[1]
				if len(parts) == 3 {
					stateAt = parts[2]
				}
			case parts[0] == "T" && len(parts) >= 2:
				v := ""
				if len(parts) == 3 {
					v = strings.TrimSpace(parts[2])
				}
				tools[parts[1]] = v
			}
		}
		switch {
		case exists && writable:
			add("path", "ok", dc.d.Path+" exists and is writable")
		case exists:
			add("path", "fail", dc.d.Path+" is not writable for this user")
		case writable:
			add("path", "ok", fmt.Sprintf("%s does not exist yet; %s (the nearest existing parent) is writable, the first deploy creates it", dc.d.Path, parent))
		default:
			add("path", "fail", fmt.Sprintf("%s does not exist and %s (the nearest existing parent) is not writable", dc.d.Path, parent))
		}
		switch stateSt {
		case "w":
			add("state", "ok", dc.state+" is writable")
		case "ro":
			add("state", "fail", dc.state+" is not writable")
		case "can":
			add("state", "ok", fmt.Sprintf("%s is created on the first deploy (%s is writable)", dc.state, stateAt))
		default:
			add("state", "fail", fmt.Sprintf("%s cannot be created (%s is not writable)", dc.state, stateAt))
		}
		if len(unreadable) > 0 {
			add("unreadable", "warn", "unreadable subdirectories: "+strings.Join(unreadable, "; "))
		} else if exists {
			add("unreadable", "ok", "every subdirectory is readable")
		}
		var other []string
		for _, s := range signs {
			switch s {
			case ".git", ".svn", ".hg":
				other = append(other, s+" checkout")
			case "releases", "current":
				other = append(other, s+" (release directories)")
			case "symlink":
				other = append(other, "the app path is a symlink")
			}
		}
		if len(other) > 0 {
			add("method", "warn", "signs of a different deploy method: "+strings.Join(other, ", "))
		} else {
			add("method", "ok", "no signs of a different deploy method")
		}
		if looksPublic(dc.d.Path) {
			add("webroot", "warn", "the path looks like a public web root: deployed files (and .deploy-bak) may be served")
		}
		for _, x := range []string{"sha256sum", "rsync", "tar"} {
			v, known := tools[x]
			switch {
			case !known:
			case v != "":
				add(x, "ok", v)
			case x == "rsync":
				add(x, "info", "not installed (not needed: WRM sends a tar stream)")
			case x == "sha256sum":
				add(x, "warn", "not installed: shasum or openssl is used when present")
			default:
				add(x, "fail", "not installed")
			}
		}
		if l := parseLock(o); l != nil {
			st := "warn"
			msg := fmt.Sprintf("locked by %s from %s (deploy %s, %ds)", l.User, l.From, l.ID, l.Age)
			if l.Stale {
				msg += ": stale"
			}
			add("lock", st, msg)
		} else {
			add("lock", "ok", "not locked")
		}
	})
	return out, nil
}

// ─── API ─────────────────────────────────────────────

// apiGitEnv handles /api/git/env/…; it returns false for other routes.
//
//	GET    /api/git/env/overview[?auto=1]        services with environments (cells load separately); auto=1 adds the derived ones
//	POST   /api/git/env/adopt {app}              store the derived environments in the catalog (audited)
//	GET    /api/git/env/cell?app=&env=           state of every destination, commits behind, drift
//	POST   /api/git/env/plan {app, env, ref}     → plan with fingerprints (applied once)
//	GET    /api/git/env/history?app=&env=        merged history.jsonl of the destinations
//	POST   /api/git/env/doctor {app, env}        access doctor
//	POST   /api/git/env/unlock {app, env, dest, lock_id}  remove a stale lock (checked again on the server)
//	POST   /api/git/env/exclude {app, env, path, scope}   permanent exclude (service | env)
//	DELETE /api/git/env/exclusions?app=&env=[&path=]      clear remembered per-run exclusions
//	deploys and rollbacks: POST /api/git/runs with "env" (git_env_run.go)
func apiGitEnv(w http.ResponseWriter, r *http.Request, userID int, rest string) bool {
	if !strings.HasPrefix(rest, "env/") {
		return false
	}
	q := r.URL.Query()
	needCheck := func() bool {
		if !gitChecksAllowed(userID) {
			jsonError(w, "Checks are not allowed for your account", 403)
			return false
		}
		return true
	}
	switch {
	case rest == "env/overview" && r.Method == http.MethodGet:
		cat := loadGitCatalog(userID)
		auto := q.Get("auto") == "1" // Simple mode: add the environments derived from the detected installations
		autoN := 0
		if auto {
			for name, envs := range deriveEnvs(userID, cat) {
				if a, ok := cat.app(name); ok {
					a.Environments = append(a.Environments, envs...)
					autoN += len(envs)
				}
			}
		}
		type envBrief struct {
			Name        string       `json:"name"`
			Dests       []gitEnvDest `json:"destinations"`
			Confirm     bool         `json:"confirm"`
			AllowedRefs []string     `json:"allowed_refs"`
			PostDeploy  string       `json:"post_deploy,omitempty"`
			Auto        bool         `json:"auto,omitempty"`
		}
		type svc struct {
			App  string     `json:"app"`
			Envs []envBrief `json:"envs"`
		}
		list := []svc{}
		names := map[string]bool{}
		var order []string
		for _, a := range cat.Apps {
			if len(a.Environments) == 0 {
				continue
			}
			s := svc{App: a.Name}
			for _, e := range a.Environments {
				s.Envs = append(s.Envs, envBrief{e.Name, e.Destinations, e.needsConfirm(), nonNil(e.AllowedRefs), e.PostDeploy, e.Auto})
				if !names[e.Name] {
					names[e.Name] = true
					order = append(order, e.Name)
				}
			}
			list = append(list, s)
		}
		if order == nil {
			order = []string{}
		}
		jsonOK(w, map[string]interface{}{"services": list, "envs": order, "stale_minutes": int(gitEnvStaleAfter() / time.Minute), "auto": autoN, "hosts": hostLabels(userID)})

	case rest == "env/adopt" && r.Method == http.MethodPost:
		if !gitActionAllowed(userID, "git_update") {
			jsonError(w, "This action is not allowed for your account", 403)
			return true
		}
		var in struct {
			App string `json:"app"`
		}
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		added, err := adoptDerivedEnvs(userID, strings.TrimSpace(in.App))
		if err != nil {
			jsonError(w, err.Error(), 400)
			return true
		}
		total := 0
		for _, n := range added {
			total += n
		}
		auditLog(r, userID, "git.env_adopted", strings.TrimSpace(in.App), map[string]interface{}{"services": len(added), "environments": total})
		jsonOK(w, map[string]interface{}{"added": added, "environments": total})

	case rest == "env/cell" && r.Method == http.MethodGet:
		if !needCheck() {
			return true
		}
		res, err := envCell(userID, q.Get("app"), q.Get("env"))
		if err != nil {
			jsonError(w, err.Error(), 404)
			return true
		}
		jsonOK(w, res)

	case rest == "env/plan" && r.Method == http.MethodPost:
		var in struct {
			App string `json:"app"`
			Env string `json:"env"`
			Ref string `json:"ref"`
		}
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		p, code, err := envPlanRequest(userID, in.App, in.Env, in.Ref)
		if err != nil {
			jsonError(w, err.Error(), code)
			return true
		}
		jsonOK(w, p)

	case rest == "env/history" && r.Method == http.MethodGet:
		if !needCheck() {
			return true
		}
		res, err := envHistory(userID, q.Get("app"), q.Get("env"))
		if err != nil {
			jsonError(w, err.Error(), 404)
			return true
		}
		jsonOK(w, res)

	case rest == "env/doctor" && r.Method == http.MethodPost:
		if !needCheck() {
			return true
		}
		var in struct {
			App string `json:"app"`
			Env string `json:"env"`
		}
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		res, err := envDoctor(userID, in.App, in.Env)
		if err != nil {
			jsonError(w, err.Error(), 404)
			return true
		}
		jsonOK(w, map[string]interface{}{"dests": res})

	case rest == "env/unlock" && r.Method == http.MethodPost:
		if !gitActionAllowed(userID, "git_update") && !gitActionAllowed(userID, "git_rollback") {
			jsonError(w, "This action is not allowed for your account", 403)
			return true
		}
		var in struct {
			App    string `json:"app"`
			Env    string `json:"env"`
			Dest   int    `json:"dest"`
			LockID string `json:"lock_id"`
		}
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		cat := envCatalog(userID, in.App, in.Env)
		a, e, err := findEnv(cat, in.App, in.Env)
		if err != nil || in.Dest < 0 || in.Dest >= len(e.Destinations) {
			jsonError(w, "Destination not found", 404)
			return true
		}
		d := e.Destinations[in.Dest]
		conn, err := envConn(userID, d.Server)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return true
		}
		cl, err := dialSSH(conn, nil)
		if err != nil {
			jsonError(w, fmt.Sprintf("cannot log in to %s: %v", conn.Name, err), 502)
			return true
		}
		defer cl.Close()
		sd := envStateDir(*e, a.Name, d.Path)
		ok, err := envBreakStale(cl, sd, in.LockID)
		if err != nil {
			jsonError(w, err.Error(), 502)
			return true
		}
		if !ok {
			jsonError(w, "The lock changed or is not stale: compare again", 409)
			return true
		}
		auditLogRef(r, userID, usernameOf(userID), "git.lock_removed", d.Path, map[string]interface{}{"app": a.Name, "env": e.Name, "lock_id": in.LockID, "state_dir": sd}, auditRef{ConnID: conn.ID})
		jsonOK(w, map[string]interface{}{"ok": true})

	case rest == "env/exclude" && r.Method == http.MethodPost:
		var in struct {
			App   string `json:"app"`
			Env   string `json:"env"`
			Path  string `json:"path"`
			Scope string `json:"scope"`
		}
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		pattern, added, err := addPermanentExclude(userID, in.App, in.Env, in.Path, in.Scope)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return true
		}
		if added {
			auditLog(r, userID, "git.exclude_added", in.App, map[string]interface{}{"env": in.Env, "scope": in.Scope, "pattern": pattern})
		}
		jsonOK(w, map[string]interface{}{"pattern": pattern, "added": added})

	case rest == "env/exclusions" && r.Method == http.MethodDelete:
		if p := q.Get("path"); p != "" {
			db.Exec(`DELETE FROM git_env_exclusions WHERE user_id=? AND app=? AND env=? AND path=?`, userID, q.Get("app"), q.Get("env"), p)
		} else {
			db.Exec(`DELETE FROM git_env_exclusions WHERE user_id=? AND app=? AND env=?`, userID, q.Get("app"), q.Get("env"))
		}
		jsonOK(w, map[string]interface{}{"excluded": loadEnvExclusions(userID, q.Get("app"), q.Get("env"))})

	default:
		jsonError(w, "Not found", 404)
	}
	return true
}
