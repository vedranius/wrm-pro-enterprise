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
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── MEDIUM: native deployment ───
//
// A deployment applies a reviewed plan (a native preview run) to its destinations, one
// after another, through the WRM connections linked to the servers. Per destination:
//
//  1. the lock (lock/ created with mkdir, owner.json written in the same command);
//  2. the plan again, under the lock: it must equal the reviewed one, else nothing changes;
//  3. backups/<id>.added (what the deployment adds) and backups/<id>.dirs (directories it
//     deletes, with their modes and owners), refused when the id already exists;
//  4. the transfer: items that are deleted or replaced move into backups/<id>/, new
//     directories are created, new and changed files are uploaded next to their target,
//     checked by SHA-256 and moved into place; changed files go to the backup first;
//  5. post_deploy in the application directory (the login shell, as ssh runs it);
//  6. current.json and a line in history.jsonl; old backups are pruned; the lock released.
//
// A dry run does step 2 only, takes no lock and writes nothing. What the server ends up
// with matches rsync -rlc --delete --backup without -p and -t (checked in the tests):
// changed files keep their permissions, new ones get the source's (0664, 0775 executable)
// minus the server's umask, and nothing is converted (line endings stay as in Git).

const (
	mdUploadParallel = 4
	mdJobLogMax      = 20000
	mdJobKeep        = time.Hour
)

// ─── plan of one destination ───

type mdServerView struct {
	changes     []mdChange // raw paths (dirs with "/")
	src, dst    mdTree     // filtered by the ignore patterns and the plan's exclusions
	keep        map[string]bool
	exists      bool
	excludedFor []string // excluded paths that hide a change on this server
}

// mdPlanOn computes what a deployment changes below root. excluded are paths the operator
// left out of this deployment: each one and everything below it, taken literally.
func mdPlanOn(r mdRemote, root string, ignore, excluded []string, src mdTree) (*mdServerView, error) {
	rules := mdCompileRules(ignore)
	fsrc := mdFilterTree(src, rules)
	dst, keep, exists, err := mdWalk(r, root, rules, fsrc)
	if err != nil {
		return nil, err
	}
	if err := mdFillHashes(r, root, fsrc, dst); err != nil {
		return nil, err
	}
	full, conflicts := mdDiff(fsrc, dst, keep)
	v := &mdServerView{exists: exists, changes: full, excludedFor: []string{}}
	if len(excluded) > 0 {
		exact := make([]*mdRule, 0, len(excluded))
		for _, e := range excluded {
			exact = append(exact, mdExactRule(e))
		}
		fsrc = mdFilterTree(fsrc, exact)
		kept := map[string]bool{}
		for p := range keep {
			kept[p] = true
		}
		fdst := mdTree{}
		for p, e := range dst {
			if mdExcluded(exact, p, e.Type == "dir") {
				for d := mdParent(p); d != ""; d = mdParent(d) {
					kept[d] = true
				}
				continue
			}
			fdst[p] = e
		}
		dst, keep = fdst, kept
		v.changes, conflicts = mdDiff(fsrc, dst, keep)
		for _, e := range excluded {
			for _, c := range full {
				if cp := strings.TrimSuffix(c.Path, "/"); cp == e || strings.HasPrefix(cp, e+"/") {
					v.excludedFor = append(v.excludedFor, e)
					break
				}
			}
		}
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("the server has a directory where the source has a file or symlink (%s), and something in it is excluded, so it cannot be replaced; exclude the whole path or nothing in it", mdEscapePath(strings.Join(conflicts, ", ")))
	}
	v.src, v.dst, v.keep = fsrc, dst, keep
	return v, nil
}

// mdCleanExcluded checks the paths of a plan's exclusions (as the change list shows them).
func mdCleanExcluded(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range in {
		p := strings.TrimSuffix(mdUnescapePath(raw), "/")
		if p == "" || strings.HasPrefix(p, "/") || mdHasDotDot(p) || path.Clean(p) != p {
			return nil, fmt.Errorf("invalid excluded path %q", raw)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	// a path below another excluded one adds nothing
	var kept []string
	for _, p := range out {
		covered := false
		for _, q := range kept {
			covered = covered || strings.HasPrefix(p, q+"/")
		}
		if !covered {
			kept = append(kept, p)
		}
	}
	if kept == nil {
		kept = []string{}
	}
	if len(kept) > 5000 {
		return nil, fmt.Errorf("too many excluded paths")
	}
	return kept, nil
}

// mdPlanRevision is a fingerprint of everything a deployment of the plan would do.
func mdPlanRevision(res *mdPlanResult) string {
	type srv struct {
		Server, Path string
		Excludes     []string
		PostDeploy   *string
		Changes      []mdChange
	}
	v := struct {
		Commit        string
		Excluded      []string
		BuildLocation string
		BuildServer   string
		BuildPlatform string
		BuildArch     string
		Servers       []srv
	}{
		Commit: res.Commit, Excluded: res.Excluded,
		BuildLocation: res.BuildLocation, BuildServer: res.BuildServer,
		BuildPlatform: res.BuildPlatform, BuildArch: res.BuildArch,
	}
	for _, s := range res.Servers {
		v.Servers = append(v.Servers, srv{s.Server, s.Path, s.Excludes, s.PostDeploy, s.Changes})
	}
	sum := sha256.Sum256(jsonMarshal(v))
	return hex.EncodeToString(sum[:8])
}

// ─── jobs ───

type mdLogLine struct {
	At    string `json:"at"`
	Level string `json:"level"` // info | ok | warn | error | cmd | out
	Text  string `json:"text"`
}

type mdOutcome struct {
	Server    string       `json:"server"`
	Label     string       `json:"label"`
	Status    string       `json:"status"` // ok | dry-run | failed | mismatch | locked | skipped
	Message   string       `json:"message"`
	Sent      int          `json:"sent"`
	Deleted   int          `json:"deleted"`
	LockOwner *mdLockOwner `json:"lock_owner,omitempty"`
}

type mdJobView struct {
	ID          string      `json:"id"`
	Kind        string      `json:"kind"` // deploy | rollback
	DryRun      bool        `json:"dry_run"`
	Project     string      `json:"project"`
	Environment string      `json:"environment"`
	Ref         string      `json:"ref,omitempty"`
	Commit      string      `json:"commit,omitempty"`
	Version     string      `json:"version,omitempty"`
	DeployID    string      `json:"deploy_id"`
	RolledBack  string      `json:"rolled_back,omitempty"`
	RunID       string      `json:"run_id,omitempty"`
	Revision    string      `json:"revision,omitempty"`
	Confirm     bool        `json:"confirm"`
	Servers     []string    `json:"servers"`
	User        string      `json:"user"`
	Started     string      `json:"started"`
	Ended       string      `json:"ended,omitempty"`
	Done        bool        `json:"done"`
	Phase       string      `json:"phase"`
	Outcomes    []mdOutcome `json:"outcomes"`
	Lines       []mdLogLine `json:"lines,omitempty"`
	Dropped     int         `json:"dropped"`
}

type mdJob struct {
	mdJobView
	uid       int
	at        time.Time
	activity  int64
	cancelled string // why the job was asked to stop (before any file changed)
	changing  bool   // a destination is being changed: cancelling waits for it
	mu        sync.Mutex
}

var mdJobs = struct {
	sync.Mutex
	m      map[string]*mdJob
	active map[int]*mdJob
	dests  map[string]bool // destinations being changed, by server address and state path
}{m: map[string]*mdJob{}, active: map[int]*mdJob{}, dests: map[string]bool{}}

func mdRandomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (j *mdJob) log(level, format string, a ...interface{}) {
	text := format
	if len(a) > 0 {
		text = fmt.Sprintf(format, a...)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if len(j.Lines) >= mdJobLogMax {
			j.Dropped++
			continue
		}
		j.Lines = append(j.Lines, mdLogLine{At: time.Now().UTC().Format("15:04:05"), Level: level, Text: strings.ToValidUTF8(line, "�")})
	}
}

func (j *mdJob) phase(p string) {
	j.mu.Lock()
	j.Phase = p
	j.mu.Unlock()
}

// stop: has the job been cancelled (checked only before a destination starts changing)?
func (j *mdJob) stop() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.cancelled
}

func (j *mdJob) setChanging(on bool) {
	j.mu.Lock()
	j.changing = on
	j.mu.Unlock()
}

// view copies the job for the API, with the log lines from line from on.
func (j *mdJob) view(from int) mdJobView {
	j.mu.Lock()
	defer j.mu.Unlock()
	v := mdJobView{ID: j.ID, Kind: j.Kind, DryRun: j.DryRun, Project: j.Project, Environment: j.Environment, Ref: j.Ref, Commit: j.Commit, Version: j.Version,
		DeployID: j.DeployID, RolledBack: j.RolledBack, RunID: j.RunID, Revision: j.Revision, Confirm: j.Confirm, Servers: j.Servers, User: j.User,
		Started: j.Started, Ended: j.Ended, Done: j.Done, Phase: j.Phase, Outcomes: append([]mdOutcome{}, j.Outcomes...), Dropped: j.Dropped}
	if from < 0 {
		from = 0
	}
	if from < len(j.Lines) {
		v.Lines = append([]mdLogLine{}, j.Lines[from:]...)
	}
	return v
}

// mdStartJob registers a job (one per user) and runs fn in the background.
func mdStartJob(uid int, j *mdJob, fn func(j *mdJob)) error {
	mdJobs.Lock()
	if cur := mdJobs.active[uid]; cur != nil {
		mdJobs.Unlock()
		return fmt.Errorf("a native %s of %s is already running (started %s); wait for it to finish", cur.Kind, cur.Project, cur.Started)
	}
	for id, old := range mdJobs.m {
		if old.Done && time.Since(old.at) > mdJobKeep {
			delete(mdJobs.m, id)
		}
	}
	j.ID, j.uid, j.at, j.Started, j.User, j.Phase = mdRandomHex(8), uid, time.Now(), nowStamp(), usernameOf(uid), "starting"
	j.Outcomes = []mdOutcome{}
	mdJobs.m[j.ID] = j
	mdJobs.active[uid] = j
	mdJobs.Unlock()
	op := "native_" + j.Kind
	if j.DryRun {
		op += "_dry_run"
	}
	if res, err := db.Exec(`INSERT INTO medium_activity (at, user_id, op, status, detail, shared) VALUES (?,?,?,?,?,0)`, j.Started, uid, op, "running", truncateStr(mdJobSummary(j), 500)); err == nil {
		j.activity, _ = res.LastInsertId()
	}
	go func() {
		defer func() {
			if p := recover(); p != nil {
				j.log("error", "internal error: %v", p)
			}
			mdFinishJob(j)
		}()
		fn(j)
	}()
	return nil
}

func mdJobSummary(j *mdJob) string {
	s := j.Project + " / " + j.Environment
	if j.Kind == "rollback" {
		return s + ": rollback of " + j.RolledBack
	}
	return s + " @ " + j.Ref + " (" + j.DeployID + ")"
}

func mdFinishJob(j *mdJob) {
	// the user may start the next job as soon as this one shows as done
	mdJobs.Lock()
	if mdJobs.active[j.uid] == j {
		delete(mdJobs.active, j.uid)
	}
	mdJobs.Unlock()
	j.mu.Lock()
	j.Done, j.Ended, j.Phase = true, nowStamp(), "done"
	status, errText := "ok", ""
	for _, o := range j.Outcomes {
		if o.Status != "ok" && o.Status != "dry-run" {
			status = "error"
			if errText == "" {
				errText = o.Label + ": " + o.Message
			}
		}
	}
	var out strings.Builder
	for _, l := range j.Lines {
		out.WriteString(l.At + " " + l.Text + "\n")
	}
	summary := mdJobSummary(j)
	for _, o := range j.Outcomes {
		summary += fmt.Sprintf("; %s: %s", o.Server, o.Status)
	}
	j.mu.Unlock()
	text := out.String()
	if len(text) > 256<<10 {
		text = text[len(text)-256<<10:]
	}
	db.Exec(`UPDATE medium_activity SET status=?, ms=?, detail=?, error=?, output=? WHERE id=?`, status, time.Since(j.at).Milliseconds(), truncateStr(summary, 500),
		truncateStr(redactText(errText), 2000), encryptValue(redactText(text)), j.activity)
	mediumPruneActivity()
	auditLog(nil, j.uid, "git.medium_native_"+j.Kind+"_done", j.Project, map[string]interface{}{"environment": j.Environment, "deploy_id": j.DeployID, "dry_run": j.DryRun, "outcomes": j.Outcomes})
}

func mdGetJob(uid int, id string) *mdJob {
	mdJobs.Lock()
	defer mdJobs.Unlock()
	j := mdJobs.m[id]
	if j == nil || j.uid != uid && !isAdminUser(uid) {
		return nil
	}
	return j
}

// mdCancelUserJobs asks a user's running job to stop before it changes the next
// destination (Stop user, lockdown). A destination already being changed is finished, so
// it is never left half deployed.
func mdCancelUserJobs(uid int, why string) int {
	mdJobs.Lock()
	var list []*mdJob
	for u, j := range mdJobs.active {
		if uid == 0 || u == uid {
			list = append(list, j)
		}
	}
	mdJobs.Unlock()
	for _, j := range list {
		j.mu.Lock()
		j.cancelled = why
		j.mu.Unlock()
		j.log("warn", "Stop requested: %s. The job stops before the next destination.", why)
	}
	return len(list)
}

// mdClaimDestination keeps two WRM jobs off one destination (the lock on the server is the
// authority; this only avoids racing for it from one WRM).
func mdClaimDestination(key string) bool {
	mdJobs.Lock()
	defer mdJobs.Unlock()
	if mdJobs.dests[key] {
		return false
	}
	mdJobs.dests[key] = true
	return true
}

func mdReleaseDestination(key string) {
	mdJobs.Lock()
	delete(mdJobs.dests, key)
	mdJobs.Unlock()
}

// ─── remote state helpers ───

func mdStateFile(d *mdDestination, parts ...string) (string, error) {
	dir := d.statePath()
	if !strings.HasPrefix(dir, "/") || mdHasDotDot(dir) || strings.ContainsAny(dir, "\x00\n\r") {
		return "", fmt.Errorf("invalid state directory %q", dir)
	}
	return path.Join(append([]string{dir}, parts...)...), nil
}

// mdReadCurrent reads current.json (nil when there is none). A record of another
// application sharing the state directory is an error.
func mdReadCurrent(h mdRemote, d *mdDestination) (*mdRecord, error) {
	f, err := mdStateFile(d, "current.json")
	if err != nil {
		return nil, err
	}
	b, err := h.ReadFile(f, mdStateMax)
	if err != nil {
		return nil, fmt.Errorf("current.json: %v", err)
	}
	if b == nil {
		return nil, nil
	}
	rec, err := mdParseRecord(b)
	if err != nil {
		return nil, nil // as the original tool: an unreadable current.json counts as none
	}
	if rec.Path != nil && *rec.Path != "" && mdNorm(*rec.Path) != d.appDir() {
		return nil, fmt.Errorf("the state directory %s on %s belongs to %s, not %s: two applications share their state", d.statePath(), d.Server, *rec.Path, d.appDir())
	}
	return rec, nil
}

// mdReadHistory reads the last entries of history.jsonl (newest first), this
// application's only.
func mdReadHistory(h mdRemote, d *mdDestination, limit int) ([]mdHistoryEntry, error) {
	f, err := mdStateFile(d, "history.jsonl")
	if err != nil {
		return nil, err
	}
	tail, err := h.ReadTail(f, 8<<20)
	if err != nil {
		return nil, fmt.Errorf("history.jsonl: %v", err)
	}
	text := string(tail)
	if len(tail) >= 8<<20 {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	var out []mdHistoryEntry
	for _, e := range mdParseHistory(text) {
		if e.Path != nil && *e.Path != "" && mdNorm(*e.Path) != d.appDir() {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// mdReadLock: the lock's owner, or nil without a lock. A lock without a readable
// owner.json has the owner "unknown" and the lock directory's time.
func mdReadLock(h mdHost, d *mdDestination) (*mdLockOwner, error) {
	dir, err := mdStateFile(d, "lock")
	if err != nil {
		return nil, err
	}
	fi, err := h.Lstat(dir)
	if err != nil {
		if mdIsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !fi.IsDir() {
		return nil, nil
	}
	if b, _ := h.ReadFile(dir+"/owner.json", 64<<10); b != nil {
		var o mdLockOwner
		if mdUnmarshal(b, &o) == nil && o.User != "" {
			return &o, nil
		}
	}
	return &mdLockOwner{User: "unknown", Host: "unknown", Since: fi.ModTime().UTC().Format(mdTimeFormat)}, nil
}

type mdLockedError struct{ Owner *mdLockOwner }

func (e *mdLockedError) Error() string {
	if e.Owner == nil {
		return "the destination is locked"
	}
	return fmt.Sprintf("locked by %s (%s) since %s", e.Owner.User, e.Owner.Host, e.Owner.Since)
}

func mdAcquireLock(h mdHost, d *mdDestination, owner mdLockOwner) error {
	dir, err := mdStateFile(d, "lock")
	if err != nil {
		return err
	}
	state := d.statePath()
	owner.Path = strPtr(d.appDir())
	script := "mkdir -p -- " + shellQuote(state) + " && if mkdir -- " + shellQuote(dir) + " 2>/dev/null; then cat > " + shellQuote(dir+"/owner.json") +
		" || { rm -rf -- " + shellQuote(dir) + "; exit 1; }; else exit " + strconv.Itoa(mdLockedExit) + "; fi"
	ctx, cancel := mdCtx(settingInt("medium_command_timeout"))
	defer cancel()
	code, _, stderr, err := h.Sh(ctx, script, mdMarshalLine(&owner))
	if err != nil {
		return err
	}
	switch code {
	case 0:
		return nil
	case mdLockedExit:
		o, _ := mdReadLock(h, d)
		return &mdLockedError{Owner: o}
	}
	return fmt.Errorf("the lock was not set (exit code %d): %s; the state directory %s must be writable", code, strings.TrimSpace(stderr), state)
}

// mdOwnerTest is a shell test: the lock's owner.json is deployment id's.
func mdOwnerTest(dir, id string) string {
	return "grep -qF -- " + shellQuote(`"deploy_id":"`+id+`"`) + " " + shellQuote(dir+"/owner.json") + " 2>/dev/null"
}

// mdReleaseLock removes the lock only while it is still this deployment's.
func mdReleaseLock(h mdHost, d *mdDestination, id string) error {
	dir, err := mdStateFile(d, "lock")
	if err != nil {
		return err
	}
	if !mdDeployIDRe.MatchString(id) {
		return fmt.Errorf("invalid deployment id %q", id)
	}
	_, err = mdShCheck(h, "if "+mdOwnerTest(dir, id)+"; then rm -rf -- "+shellQuote(dir)+"; fi", nil, "release the lock")
	return err
}

// mdReleaseStaleLock removes a lock by hand: only while it exists, still has the owner the
// operator saw (id; empty when the owner is unknown) and is older, by the server's clock,
// than mdStaleLockMinutes.
func mdReleaseStaleLock(h mdHost, d *mdDestination, id string) error {
	dir, err := mdStateFile(d, "lock")
	if err != nil {
		return err
	}
	minutes := mdStaleLockMinutes()
	q := shellQuote(dir)
	script := "[ -d " + q + " ] || exit " + strconv.Itoa(mdStaleGone) + "; "
	if id != "" {
		if !mdDeployIDRe.MatchString(id) {
			return fmt.Errorf("invalid deployment id %q", id)
		}
		script += mdOwnerTest(dir, id) + " || exit " + strconv.Itoa(mdStaleOtherOwner) + "; "
	}
	script += "now=$(date -u +%s); t=$(stat -c %Y -- " + q + " 2>/dev/null || date -u -r " + q + " +%s); age=$(( (now - t) / 60 )); " +
		"if [ \"$age\" -lt " + strconv.Itoa(minutes) + " ]; then echo \"$age\"; exit " + strconv.Itoa(mdStaleYoung) + "; fi; rm -rf -- " + q
	ctx, cancel := mdCtx(settingInt("medium_command_timeout"))
	defer cancel()
	code, out, stderr, err := h.Sh(ctx, script, nil)
	if err != nil {
		return err
	}
	switch code {
	case 0:
		return nil
	case mdStaleGone:
		return fmt.Errorf("%s is no longer locked (the deployment finished)", d.label())
	case mdStaleOtherOwner:
		return fmt.Errorf("another deployment has locked %s since; the lock stays", d.label())
	case mdStaleYoung:
		return fmt.Errorf("the lock on %s is %s minutes old by the server's clock; it can be released once it is older than %d minutes, because a deployment may still be running", d.label(), strings.TrimSpace(string(out)), minutes)
	}
	return fmt.Errorf("the lock on %s was not released (exit code %d): %s", d.label(), code, strings.TrimSpace(stderr))
}

// mdWriteState writes current.json (atomically, when current) and appends the entry.
func mdWriteState(h mdHost, d *mdDestination, e *mdHistoryEntry, current bool) error {
	state := d.statePath()
	if current {
		f, err := mdStateFile(d, "current.json")
		if err != nil {
			return err
		}
		rec := e.mdRecord
		if _, err := mdShCheck(h, "mkdir -p -- "+shellQuote(state)+" && cat > "+shellQuote(f+".tmp")+" && mv -f -- "+shellQuote(f+".tmp")+" "+shellQuote(f), mdMarshalLine(&rec), "write current.json"); err != nil {
			return err
		}
	}
	f, err := mdStateFile(d, "history.jsonl")
	if err != nil {
		return err
	}
	_, err = mdShCheck(h, "mkdir -p -- "+shellQuote(state)+" && cat >> "+shellQuote(f), mdMarshalLine(e), "write history.jsonl")
	return err
}

// mdWriteManifest writes backups/<id>.added and backups/<id>.dirs before the first change.
func mdWriteManifest(h mdHost, d *mdDestination, id string, added []string, dirs []string) error {
	backups, err := mdStateFile(d, "backups")
	if err != nil {
		return err
	}
	base := path.Join(backups, id)
	var content bytes.Buffer
	for _, p := range added {
		content.WriteString(mdEscapePathControls(p) + "\n")
	}
	ctx, cancel := mdCtx(settingInt("medium_command_timeout"))
	defer cancel()
	script := "mkdir -p -- " + shellQuote(backups) + " && if [ -e " + shellQuote(base) + " ] || [ -e " + shellQuote(base+".added") + " ] || [ -e " + shellQuote(base+".dirs") +
		" ]; then exit " + strconv.Itoa(mdIDExistsExit) + "; fi && cat > " + shellQuote(base+".added")
	code, _, stderr, err := h.Sh(ctx, script, content.Bytes())
	if err != nil {
		return err
	}
	if code == mdIDExistsExit {
		return fmt.Errorf("a backup %s already exists on %s; start the deployment again", id, d.label())
	}
	if code != 0 {
		return fmt.Errorf("the list of added items (%s.added) was not written on %s: %s", id, d.label(), strings.TrimSpace(stderr))
	}
	modes := mdDirModes(h, d.appDir(), dirs)
	content.Reset()
	for i, p := range dirs {
		if i < len(modes) && modes[i] != "" {
			content.WriteString(modes[i] + "\t")
		}
		content.WriteString(mdEscapePathControls(p) + "\n")
	}
	_, err = mdShCheck(h, "cat > "+shellQuote(base+".dirs"), content.Bytes(), "write "+id+".dirs")
	return err
}

// mdDirModes reads "mode\tuid\tgid" of directories before they are deleted ("" when not
// known), so a rollback recreates them as they were.
func mdDirModes(h mdHost, root string, dirs []string) []string {
	if len(dirs) == 0 {
		return nil
	}
	var in bytes.Buffer
	for _, p := range dirs {
		in.WriteString(strings.TrimSuffix(p, "/"))
		in.WriteByte(0)
	}
	script := "cd -- " + shellQuote(root) + " 2>/dev/null || exit 0; xargs -0 sh -c 'for p; do printf \"%s\\0\" \"$(stat -c \"%a %u %g\" -- \"$p\" 2>/dev/null)\"; done' sh"
	ctx, cancel := mdCtx(settingInt("medium_command_timeout"))
	defer cancel()
	code, out, _, err := h.Sh(ctx, script, in.Bytes())
	if err != nil || code != 0 {
		return nil
	}
	values := strings.Split(string(out), "\x00")
	if len(values) < len(dirs) {
		return nil
	}
	res := make([]string, len(dirs))
	for i := range dirs {
		f := strings.Fields(values[i])
		ok := len(f) == 3
		for _, x := range f {
			_, e := strconv.Atoi(x)
			ok = ok && e == nil
		}
		if ok {
			res[i] = strings.Join(f, "\t")
		}
	}
	return res
}

// mdPruneBackups removes the backups a rollback can no longer use: everything but the
// current deployment's and the last mdKeepBackups applied ones; rollback backups keep
// their newest mdKeepBackups. Other names are never touched.
func mdPruneBackups(h mdHost, d *mdDestination, current string, applied []string) ([]string, error) {
	backups, err := mdStateFile(d, "backups")
	if err != nil {
		return nil, err
	}
	list, err := h.ReadDir(backups)
	if err != nil {
		if mdIsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	keep := map[string]bool{current: true}
	if len(applied) > mdKeepBackups {
		applied = applied[len(applied)-mdKeepBackups:]
	}
	for _, id := range applied {
		keep[id] = true
	}
	deploys := map[string]bool{}
	type rb struct {
		name string
		t    time.Time
	}
	var rollbacks []rb
	for _, fi := range list {
		name := fi.Name()
		if strings.HasPrefix(name, mdRollbackPrefix) && mdDeployIDRe.MatchString(strings.TrimPrefix(name, mdRollbackPrefix)) {
			rollbacks = append(rollbacks, rb{name, fi.ModTime()})
			continue
		}
		id := strings.TrimSuffix(strings.TrimSuffix(name, ".added"), ".dirs")
		if mdDeployIDRe.MatchString(id) {
			deploys[id] = true
		}
	}
	var removed, targets []string
	ids := sortedKeys(deploys)
	for _, id := range ids {
		if !keep[id] {
			removed = append(removed, id)
			for _, s := range []string{"", ".added", ".dirs"} {
				targets = append(targets, path.Join(backups, id+s))
			}
		}
	}
	sort.Slice(rollbacks, func(i, j int) bool {
		return rollbacks[i].t.After(rollbacks[j].t) || rollbacks[i].t.Equal(rollbacks[j].t) && rollbacks[i].name > rollbacks[j].name
	})
	if len(rollbacks) > mdKeepBackups {
		for _, r := range rollbacks[mdKeepBackups:] {
			removed = append(removed, r.name)
			targets = append(targets, path.Join(backups, r.name))
		}
	}
	if len(targets) == 0 {
		return nil, nil
	}
	q := make([]string, len(targets))
	for i, t := range targets {
		q[i] = shellQuote(t)
	}
	_, err = mdShCheck(h, "rm -rf -- "+strings.Join(q, " "), nil, "remove old backups")
	return removed, err
}

func mdUnmarshal(b []byte, v interface{}) error {
	return json.Unmarshal(bytes.TrimPrefix(bytes.TrimSpace(b), []byte("\xef\xbb\xbf")), v)
}

// ─── one destination ───

type mdDeployTarget struct {
	d          *mdDestination
	plan       *mdServerPlan // the reviewed plan of this destination
	postDeploy *string
}

// mdDeployContext is what every destination of one deployment shares.
type mdDeployContext struct {
	uid      int
	cfg      *mdConfig
	proj     *mdProject
	target   *mdTarget
	src      mdTree
	excluded []string
	entry    mdHistoryEntry // template: project, environment, version, commit, ref, id, time, user
	dryRun   bool
	job      *mdJob
}

// mdChangeSet is a plan turned into operations, with the paths as on the server.
type mdChangeSet struct {
	removals []mdChange // deleted items and server items replaced by another type, deepest first
	mkdirs   []string   // new directories, shallowest first
	files    []mdChange // new and changed files and symlinks
	added    []string   // backups/<id>.added
	delDirs  []string   // backups/<id>.dirs
}

func mdDepth(p string) int { return strings.Count(strings.TrimSuffix(p, "/"), "/") }

func mdMakeChangeSet(changes []mdChange) mdChangeSet {
	var cs mdChangeSet
	for _, c := range changes {
		rel := strings.TrimSuffix(c.Path, "/")
		switch c.Action {
		case "deleted":
			cs.removals = append(cs.removals, c)
			if c.Kind == "dir" {
				cs.delDirs = append(cs.delDirs, c.Path)
			}
		case "new":
			cs.added = append(cs.added, c.Path)
			if c.ServerKind != "" {
				cs.removals = append(cs.removals, mdChange{Path: rel, Action: "replaced", Kind: c.ServerKind, ServerKind: c.ServerKind})
				if c.ServerKind == "dir" {
					cs.delDirs = append(cs.delDirs, rel+"/")
				}
			}
			if c.Kind == "dir" {
				cs.mkdirs = append(cs.mkdirs, rel)
			} else {
				cs.files = append(cs.files, c)
			}
		case "changed":
			cs.files = append(cs.files, c)
		}
	}
	sort.SliceStable(cs.removals, func(i, j int) bool {
		a, b := cs.removals[i].Path, cs.removals[j].Path
		if mdDepth(a) != mdDepth(b) {
			return mdDepth(a) > mdDepth(b)
		}
		return a > b
	})
	sort.SliceStable(cs.mkdirs, func(i, j int) bool {
		if mdDepth(cs.mkdirs[i]) != mdDepth(cs.mkdirs[j]) {
			return mdDepth(cs.mkdirs[i]) < mdDepth(cs.mkdirs[j])
		}
		return cs.mkdirs[i] < cs.mkdirs[j]
	})
	return cs
}

// mdChangeKeys: the reviewed plan's changes as comparable keys.
func mdChangeKeys(list []mdChange) map[string]bool {
	m := map[string]bool{}
	for _, c := range list {
		m[c.Action+" "+c.Kind+" "+c.ServerKind+" "+c.Path+" "+c.Link] = true
	}
	return m
}

// mdPlanDifference describes how the server now differs from the reviewed plan.
func mdPlanDifference(planned, now []mdChange) []string {
	escaped := make([]mdChange, len(now))
	for i, c := range now {
		c.Path = mdEscapePath(c.Path)
		escaped[i] = c
	}
	a, b := mdChangeKeys(planned), mdChangeKeys(escaped)
	var out []string
	for k := range b {
		if !a[k] {
			out = append(out, "not in the plan: "+k)
		}
	}
	for k := range a {
		if !b[k] {
			out = append(out, "planned but no longer so: "+k)
		}
	}
	sort.Strings(out)
	return out
}

func mdShown(list []string, n int) string {
	if len(list) <= n {
		return strings.Join(list, "; ")
	}
	return strings.Join(list[:n], "; ") + fmt.Sprintf("; and %d more", len(list)-n)
}

// mdHistoryFiles fills the entry's file lists and counts.
func mdHistoryFiles(e *mdHistoryEntry, news, changed, deleted []string) {
	limit := func(l []string) []string {
		out := make([]string, 0, len(l))
		for i, p := range l {
			if i >= mdHistoryPaths {
				break
			}
			out = append(out, mdEscapePath(p))
		}
		return out
	}
	e.New, e.Changed, e.Deleted = limit(news), limit(changed), limit(deleted)
	e.Counts = map[string]int{"new": len(news), "changed": len(changed), "deleted": len(deleted)}
}

// mdDeployOne deploys (or dry-runs) one destination.
func mdDeployOne(dc *mdDeployContext, h mdHost, t mdDeployTarget) mdOutcome {
	j, d, id := dc.job, t.d, dc.entry.DeployID
	out := mdOutcome{Server: d.Server, Label: d.label(), Status: "ok"}
	if dc.dryRun {
		j.log("info", "── %s (dry run: nothing is changed)", d.label())
	} else {
		j.log("info", "── %s", d.label())
	}
	rec := dc.entry
	rec.Path, rec.Server, rec.Excluded = strPtr(d.appDir()), strPtr(d.Server), []string{}
	fail := func(status, msg string) mdOutcome {
		out.Status, out.Message = status, msg
		j.log("error", "%s: %s", d.label(), msg)
		return out
	}
	if !dc.dryRun {
		owner := mdLockOwner{User: rec.DeployedBy, Host: mdHostName(), Since: mdNowISO(), Project: rec.Project, Environment: rec.Environment, DeployID: strPtr(id)}
		if err := mdAcquireLock(h, d, owner); err != nil {
			if le, ok := err.(*mdLockedError); ok {
				out.LockOwner = le.Owner
				return fail("locked", err.Error())
			}
			return fail("failed", err.Error())
		}
		j.log("ok", "Lock set: %s/lock", d.statePath())
		defer func() {
			if err := mdReleaseLock(h, d, id); err != nil {
				j.log("warn", "The lock was not released (%v); release it under History once it is stale.", err)
			} else {
				j.log("info", "Lock released.")
			}
		}()
	}
	// whatever happens after the first change, history says what the server has
	partial := func(err error, done mdApplied) mdOutcome {
		first := strings.SplitN(err.Error(), "\n", 2)[0]
		failed := rec
		failed.Note, failed.Current = strPtr("deploy interrupted during transfer (partial): "+first), boolPtr(false)
		mdHistoryFiles(&failed, done.news, done.changed, done.deleted)
		if e := mdWriteState(h, d, &failed, false); e != nil {
			j.log("warn", "History was not written: %v", e)
		}
		out.Sent, out.Deleted = len(done.news)+len(done.changed), len(done.deleted)
		return fail("failed", err.Error()+" (partly deployed; the history says what changed, and a rollback can undo it)")
	}
	if !dc.dryRun {
		prev, err := mdReadCurrent(h, d)
		if err != nil {
			return fail("failed", err.Error())
		}
		rec.Previous, rec.hasPrevious = prev, true
	}
	ignore := dc.proj.ignoreFor(dc.target.Name, d.Server)
	view, err := mdPlanOn(h, d.appDir(), ignore, dc.excluded, dc.src)
	if err != nil {
		return fail("failed", err.Error())
	}
	rec.Excluded = view.excludedFor
	cs := mdMakeChangeSet(view.changes)
	j.log("info", "Plan%s: + %d new, ~ %d changed, − %d to delete", map[bool]string{true: "", false: " under the lock"}[dc.dryRun],
		mdCount(view.changes, "new"), mdCount(view.changes, "changed"), mdCount(view.changes, "deleted"))
	for i, c := range view.changes {
		if i >= 500 {
			j.log("out", "… and %d more", len(view.changes)-500)
			break
		}
		j.log("out", "%s %s", map[string]string{"new": "+", "changed": "~", "deleted": "−"}[c.Action], mdEscapePath(c.Path))
	}
	if diff := mdPlanDifference(t.plan.Changes, view.changes); len(diff) > 0 {
		out.Status = "mismatch"
		if dc.dryRun {
			out.Message = "the server differs from the reviewed plan (" + mdShown(diff, 5) + "); a real deployment would stop here, compare again"
			j.log("warn", "%s", out.Message)
		} else {
			out.Message = "the server differs from the reviewed plan (" + mdShown(diff, 5) + "); nothing was changed, compare again and review the plan"
			j.log("error", "%s", out.Message)
		}
		return out
	}
	if dc.dryRun {
		out.Status = "dry-run"
		out.Sent, out.Deleted = len(cs.files), mdCount(view.changes, "deleted")
		out.Message = "dry run: matches the plan, nothing was changed"
		if t.postDeploy != nil {
			j.log("info", "After the deployment this command would run (post_deploy): %s", *t.postDeploy)
		} else {
			j.log("info", "No command (post_deploy) runs after the deployment on this destination.")
		}
		j.log("ok", "%s: %s", d.label(), out.Message)
		return out
	}
	if why := j.stop(); why != "" {
		return fail("skipped", "stopped before any change: "+why)
	}
	if err := mdWriteManifest(h, d, id, cs.added, cs.delDirs); err != nil {
		return fail("failed", err.Error())
	}
	j.setChanging(true)
	defer j.setChanging(false)
	done, err := mdApply(dc, h, d, id, view, cs)
	if err != nil {
		return partial(err, done)
	}
	out.Sent, out.Deleted = len(done.news)+len(done.changed), len(done.deleted)
	mdHistoryFiles(&rec, done.news, done.changed, done.deleted)
	mdHistoryEOL(&rec, t.plan, done.changed)
	var note *string
	if t.postDeploy != nil && strings.TrimSpace(*t.postDeploy) != "" {
		if n := mdRunPostDeploy(h, d, *t.postDeploy, j); n != "" {
			note = &n
			out.Status, out.Message = "failed", n+"; the files are deployed"
			j.log("error", "%s", out.Message)
		}
	}
	if problems := mdVerify(h, d.appDir(), view, cs); len(problems) > 0 {
		n := fmt.Sprintf("verify: %d paths differ (%s)", len(problems), mdShown(problems, 5))
		if note != nil {
			n = *note + "; " + n
		}
		note = &n
		out.Status, out.Message = "failed", n+"; the files are deployed"
		j.log("error", "%s", out.Message)
	}
	rec.Note, rec.Current = note, boolPtr(true)
	if err := mdWriteState(h, d, &rec, true); err != nil {
		return fail("failed", err.Error()+" (the files are deployed, but the state was not written)")
	}
	j.log("ok", "Written: %s/current.json and history.jsonl", d.statePath())
	if out.Status == "ok" {
		out.Message = fmt.Sprintf("deployed: %d sent, %d deleted", out.Sent, out.Deleted)
		j.log("ok", "%s: %s", d.label(), out.Message)
	}
	if entries, err := mdReadHistory(h, d, mdHistoryWindow); err == nil {
		var applied []string
		for _, e := range mdAppliedDeploys(entries) {
			applied = append(applied, e.DeployID)
		}
		if removed, err := mdPruneBackups(h, d, id, applied); err != nil {
			j.log("warn", "Old backups were not removed: %v", err)
		} else if len(removed) > 0 {
			j.log("info", "Old backups removed: %s", strings.Join(removed, ", "))
		}
	}
	return out
}

func mdCount(list []mdChange, action string) int {
	n := 0
	for _, c := range list {
		if c.Action == action {
			n++
		}
	}
	return n
}

func mdHostName() string {
	h, _ := os.Hostname()
	if h == "" {
		h = "host"
	}
	return "WRM " + h
}

// mdApplied is what a transfer actually did (raw paths, dirs with "/").
type mdApplied struct {
	news, changed, deleted []string
}

// mdApply changes the destination: removals into the backup, new directories, uploads
// next to the targets, a hash check of the uploads, then every upload moved into place.
func mdApply(dc *mdDeployContext, h mdHost, d *mdDestination, id string, view *mdServerView, cs mdChangeSet) (mdApplied, error) {
	j, root := dc.job, d.appDir()
	var done mdApplied
	backup := path.Join(d.statePath(), "backups", id)
	toBackup := func(rel string) error {
		if err := h.MkdirAll(path.Dir(path.Join(backup, rel))); err != nil {
			return fmt.Errorf("backup directory for %s: %v", mdEscapePath(rel), err)
		}
		if err := h.Rename(path.Join(root, rel), path.Join(backup, rel)); err != nil {
			return fmt.Errorf("move %s to the backup: %v", mdEscapePath(rel), err)
		}
		return nil
	}
	if !view.exists {
		if err := h.MkdirAll(root); err != nil {
			return done, fmt.Errorf("create %s: %v", root, err)
		}
		j.log("info", "Created %s", root)
	}
	j.phase("transfer")
	// 1. deleted items and items replaced by another type, deepest first
	for _, c := range cs.removals {
		rel := strings.TrimSuffix(c.Path, "/")
		if c.Kind == "dir" {
			if err := h.RemoveDir(path.Join(root, rel)); err != nil {
				return done, fmt.Errorf("remove the directory %s: %v", mdEscapePath(rel), err)
			}
		} else if err := toBackup(rel); err != nil {
			return done, err
		}
		if c.Action == "deleted" {
			done.deleted = append(done.deleted, c.Path)
		}
	}
	// 2. new directories (the server's umask applies; a setgid parent passes its group on)
	umask := mdRemoteUmask(h)
	for _, rel := range cs.mkdirs {
		p := path.Join(root, rel)
		if err := h.Mkdir(p); err != nil {
			return done, fmt.Errorf("create the directory %s: %v", mdEscapePath(rel), err)
		}
		// rsync creates directories as 0775 minus the umask: never writable for others
		if fi, err := h.Lstat(p); err == nil && fi.Mode().Perm()&0o002 != 0 {
			h.Chmod(p, fi.Mode()&^0o002)
		}
		done.news = append(done.news, rel+"/")
	}
	// 3. uploads next to their targets
	type upload struct {
		c        mdChange
		rel, tmp string
		mode     os.FileMode
	}
	var ups []upload
	for _, c := range cs.files {
		rel := c.Path
		tmp := path.Join(path.Dir(rel), "."+path.Base(rel)+".wrm"+mdRandomHex(3))
		if path.Dir(rel) == "." {
			tmp = "." + rel + ".wrm" + mdRandomHex(3)
		}
		mode := os.FileMode(0o664)
		if s := view.src[rel]; s != nil && s.exec {
			mode = 0o775
		}
		mode &^= umask
		if c.Action == "changed" {
			if old := view.dst[rel]; old != nil && old.perm != 0 {
				mode = old.perm & os.ModePerm
			}
		}
		ups = append(ups, upload{c, rel, tmp, mode})
	}
	cleanup := func(list []upload) {
		for _, u := range list {
			h.Remove(path.Join(root, u.tmp))
		}
	}
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	sem := make(chan struct{}, mdUploadParallel)
	written := make([]bool, len(ups))
	for i := range ups {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			u := ups[i]
			mu.Lock()
			stop := firstErr != nil
			mu.Unlock()
			if stop {
				return
			}
			var err error
			created := false
			s := view.src[u.rel]
			switch {
			case s == nil:
				err = fmt.Errorf("%s is not in the source", mdEscapePath(u.rel))
			case s.Type == "symlink":
				err = h.Symlink(s.Link, path.Join(root, u.tmp))
				created = err == nil
			case s.open == nil:
				err = fmt.Errorf("the content of %s is not available", mdEscapePath(u.rel))
			default:
				var b []byte
				if b, err = s.open(); err == nil {
					err = h.WriteNew(path.Join(root, u.tmp), bytes.NewReader(b))
					created = err == nil
				}
				if err == nil {
					err = h.Chmod(path.Join(root, u.tmp), u.mode)
				}
			}
			mu.Lock()
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("upload %s: %v", mdEscapePath(u.rel), err)
			}
			written[i] = created
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if firstErr != nil {
		var list []upload
		for i, u := range ups {
			if written[i] {
				list = append(list, u)
			}
		}
		cleanup(list)
		return done, firstErr
	}
	// 4. the uploads are checked on the server before anything is replaced
	var tmps []string
	for _, u := range ups {
		if s := view.src[u.rel]; s != nil && s.Type == "file" {
			tmps = append(tmps, u.tmp)
		}
	}
	for i := 0; i < len(tmps); i += mdHashBatch {
		end := i + mdHashBatch
		if end > len(tmps) {
			end = len(tmps)
		}
		got, err := h.Hashes(root, tmps[i:end])
		if err != nil {
			cleanup(ups)
			return done, fmt.Errorf("check the uploaded files: %v", err)
		}
		for _, u := range ups {
			if s := view.src[u.rel]; s != nil && s.Type == "file" {
				if g, ok := got[u.tmp]; ok && g != s.Hash {
					cleanup(ups)
					return done, fmt.Errorf("%s arrived different from the source (SHA-256); nothing was replaced with it", mdEscapePath(u.rel))
				}
			}
		}
	}
	j.log("info", "Uploaded and checked %d files and links", len(ups))
	// 5. into place
	for i, u := range ups {
		if u.c.Action == "changed" {
			if err := toBackup(u.rel); err != nil {
				cleanup(ups[i:])
				return done, err
			}
		}
		if err := h.Rename(path.Join(root, u.tmp), path.Join(root, u.rel)); err != nil {
			cleanup(ups[i:])
			return done, fmt.Errorf("move %s into place: %v", mdEscapePath(u.rel), err)
		}
		if u.c.Action == "changed" {
			done.changed = append(done.changed, u.rel)
		} else {
			done.news = append(done.news, u.rel)
		}
	}
	sort.Strings(done.news)
	sort.Strings(done.changed)
	sort.Strings(done.deleted)
	return done, nil
}

// mdRemoteUmask: the account's umask on the server (022 when it cannot be read).
func mdRemoteUmask(h mdHost) os.FileMode {
	ctx, cancel := mdCtx(30)
	defer cancel()
	code, out, _, err := h.Sh(ctx, "umask", nil)
	if err != nil || code != 0 {
		return 0o022
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(out)), 8, 32)
	if err != nil {
		return 0o022
	}
	return os.FileMode(v) & 0o777
}

// mdVerify reads the touched paths again and reports any that differ from the source.
func mdVerify(h mdHost, root string, view *mdServerView, cs mdChangeSet) []string {
	var problems, files []string
	for _, c := range cs.files {
		s := view.src[c.Path]
		fi, err := h.Lstat(path.Join(root, c.Path))
		switch {
		case s == nil:
		case err != nil:
			problems = append(problems, mdEscapePath(c.Path)+" is missing")
		case mdTypeOf(fi.Mode()) != s.Type:
			problems = append(problems, mdEscapePath(c.Path)+" is a "+mdTypeOf(fi.Mode()))
		case s.Type == "symlink":
			if l, err := h.ReadLink(path.Join(root, c.Path)); err != nil || l != s.Link {
				problems = append(problems, mdEscapePath(c.Path)+" points elsewhere")
			}
		default:
			files = append(files, c.Path)
		}
	}
	for _, rel := range cs.mkdirs {
		if fi, err := h.Lstat(path.Join(root, rel)); err != nil || !fi.IsDir() {
			problems = append(problems, mdEscapePath(rel)+"/ is missing")
		}
	}
	for _, c := range cs.removals {
		if c.Action != "deleted" {
			continue
		}
		if _, err := h.Lstat(path.Join(root, strings.TrimSuffix(c.Path, "/"))); err == nil {
			problems = append(problems, mdEscapePath(c.Path)+" is still there")
		}
	}
	for i := 0; i < len(files); i += mdHashBatch {
		end := i + mdHashBatch
		if end > len(files) {
			end = len(files)
		}
		got, err := h.Hashes(root, files[i:end])
		if err != nil {
			problems = append(problems, "checksums: "+err.Error())
			break
		}
		for _, p := range files[i:end] {
			if got[p] != view.src[p].Hash {
				problems = append(problems, mdEscapePath(p)+" has other content")
			}
		}
	}
	sort.Strings(problems)
	return problems
}

// mdRunPostDeploy runs post_deploy in the application directory; a note for the history
// when it fails (the files are deployed by then).
func mdRunPostDeploy(h mdHost, d *mdDestination, command string, j *mdJob) string {
	j.log("cmd", "post_deploy: %s", command)
	ctx, cancel := mdCtx(settingInt("medium_command_timeout"))
	defer cancel()
	var out capBuffer
	out.max = 4 << 20
	code, err := h.Login(ctx, "cd -- "+shellQuote(d.appDir())+" || exit 1\n"+command, &out)
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) > 500 {
		lines = lines[len(lines)-500:]
	}
	for _, l := range lines {
		if l != "" {
			j.log("out", "%s", l)
		}
	}
	if err != nil {
		return "post_deploy did not finish (" + strings.SplitN(err.Error(), "\n", 2)[0] + ")"
	}
	if code != 0 {
		return fmt.Sprintf("post_deploy failed (exit code %d)", code)
	}
	return ""
}

// mdRefNames: the short name and kind of a full ref.
func mdRefNames(ref, commit string) (string, *string) {
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		return strings.TrimPrefix(ref, "refs/heads/"), strPtr("branch")
	case strings.HasPrefix(ref, "refs/tags/"):
		return strings.TrimPrefix(ref, "refs/tags/"), strPtr("tag")
	}
	return commit, nil
}

// mdDeployDestinations runs a deployment over its destinations in config order; after a
// failure (not in a dry run) the rest are skipped.
func mdDeployDestinations(dc *mdDeployContext, targets []mdDeployTarget) {
	j := dc.job
	for i, t := range targets {
		if why := j.stop(); why != "" {
			for _, rest := range targets[i:] {
				j.mu.Lock()
				j.Outcomes = append(j.Outcomes, mdOutcome{Server: rest.d.Server, Label: rest.d.label(), Status: "skipped", Message: "not started: " + why})
				j.mu.Unlock()
			}
			return
		}
		j.phase(t.d.label())
		key := mdDestinationKey(dc.cfg, t.d)
		var out mdOutcome
		if !mdClaimDestination(key) {
			out = mdOutcome{Server: t.d.Server, Label: t.d.label(), Status: "locked", Message: "another WRM job is working on this destination"}
			j.log("error", "%s: %s", t.d.label(), out.Message)
		} else {
			err := mdOpenHost(dc.uid, dc.cfg, t.d.Server, func(h mdHost, connName string) error {
				j.log("info", "Connection: %s", connName)
				out = mdDeployOne(dc, h, t)
				return nil
			})
			mdReleaseDestination(key)
			if err != nil {
				out = mdOutcome{Server: t.d.Server, Label: t.d.label(), Status: "failed", Message: err.Error()}
				j.log("error", "%s: %s", t.d.label(), err.Error())
			}
		}
		j.mu.Lock()
		j.Outcomes = append(j.Outcomes, out)
		j.mu.Unlock()
		if !dc.dryRun && out.Status != "ok" {
			for _, rest := range targets[i+1:] {
				j.mu.Lock()
				j.Outcomes = append(j.Outcomes, mdOutcome{Server: rest.d.Server, Label: rest.d.label(), Status: "skipped", Message: "not started because of the error on the previous destination"})
				j.mu.Unlock()
			}
			return
		}
	}
}

func mdDestinationKey(cfg *mdConfig, d *mdDestination) string {
	host := d.Server
	if s := cfg.server(d.Server); s != nil {
		host = fmt.Sprintf("%s@%s:%d", s.User, s.Host, s.Port)
	}
	return host + "|" + d.statePath()
}

// mdOpenHost connects to the server's linked WRM connection and runs fn on it.
var mdOpenHost = func(uid int, cfg *mdConfig, server string, fn func(mdHost, string) error) error {
	conns := mdUserConnections(uid)
	link, ok := mdLinks(uid, cfg, conns)[server]
	if !ok {
		s := cfg.server(server)
		return fmt.Errorf("no WRM connection is linked to %s (%s@%s:%d); add or choose one under Servers", server, s.User, s.Host, s.Port)
	}
	conn, err := loadConnection(link.ConnID)
	if err != nil || conn.UserID != uid || !isSSHProtocol(conn) {
		return fmt.Errorf("the connection linked to %s is not available", server)
	}
	p, err := acquireSFTP(conn)
	if err != nil {
		return fmt.Errorf("cannot log in to %s: %v", conn.Name, err)
	}
	err = fn(mdSFTP{cl: p.sftp, ssh: p.ssh}, conn.Name)
	releaseSFTP(p, err)
	return err
}
