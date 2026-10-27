// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── GIT RUNS: update, upgrade, rollback, stamp and restart jobs ───
//
// A run is a list of installations handled one after another (rolling: the first failure
// stops it). Runs start right away or at a chosen time (a maintenance window); scheduled
// runs and restarts are stored in git_runs, survive a WRM restart, can be cancelled and are
// skipped when WRM was down past git_schedule_grace_minutes. Progress is kept in memory
// while a run works (the UI polls it, as for credential rotations) and stored when it ends.
// Policies: git_update (also stamp), git_upgrade, git_rollback, git_restart, git_install and
// git_transfer (install and transfer runs: git_provision.go).

const (
	gitMaxRunItems     = 100
	gitKeepRuns        = 500
	gitUpcomingNotice  = 15 * time.Minute
	gitSchedulerPeriod = 20 * time.Second
	gitPlanCacheTTL    = 30 * time.Minute
)

func initGitRunsSchema() {
	ensureTable("git_runs", `CREATE TABLE IF NOT EXISTS git_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		kind TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT '',
		label TEXT NOT NULL DEFAULT '',
		parent_id INTEGER NOT NULL DEFAULT 0,
		params TEXT NOT NULL DEFAULT '',
		result TEXT NOT NULL DEFAULT '',
		message TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		scheduled_at TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL DEFAULT '',
		ended_at TEXT NOT NULL DEFAULT '',
		notified_at TEXT NOT NULL DEFAULT '')`,
		"id", "user_id", "kind", "state", "label", "parent_id", "params", "result", "message", "created_at", "scheduled_at", "started_at", "ended_at", "notified_at")
	mustExec(`CREATE INDEX IF NOT EXISTS idx_git_runs_user ON git_runs(user_id, id)`)
	mustExec(`CREATE INDEX IF NOT EXISTS idx_git_runs_state ON git_runs(state)`)
	ensureTable("git_pending_restarts", `CREATE TABLE IF NOT EXISTS git_pending_restarts (
		install_id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		since TEXT NOT NULL DEFAULT '',
		run_id INTEGER NOT NULL DEFAULT 0)`,
		"install_id", "user_id", "since", "run_id")
}

// ─── policies ────────────────────────────────────────

var gitKindPolicy = map[string]string{"update": "git_update", "stamp": "git_update", "upgrade": "git_upgrade", "rollback": "git_rollback", "restart": "git_restart",
	"install": "git_install", "transfer": "git_transfer"}

func gitActionAllowed(userID int, key string) bool {
	if !gitAllowed() {
		return false
	}
	return policyWhoAllows(userID, key)
}

func gitDeployPolicies(userID int) map[string]bool {
	return map[string]bool{"update": gitActionAllowed(userID, "git_update"), "upgrade": gitActionAllowed(userID, "git_upgrade"),
		"rollback": gitActionAllowed(userID, "git_rollback"), "restart": gitActionAllowed(userID, "git_restart"),
		"install": gitActionAllowed(userID, "git_install"), "transfer": gitActionAllowed(userID, "git_transfer"),
		"gitignore_mr": gitActionAllowed(userID, "git_gitignore_mr")}
}

// isProdInstall: tagged env:prod / prod (or production, prd, live) or found in such an
// environment directory.
func isProdInstall(in gitInstall) bool {
	prod := map[string]bool{"prod": true, "production": true, "prd": true, "live": true}
	for _, tg := range in.Tags {
		tg = strings.ToLower(tg)
		if prod[tg] || prod[strings.TrimPrefix(tg, "env:")] && strings.HasPrefix(tg, "env:") {
			return true
		}
	}
	return prod[strings.ToLower(in.Env)]
}

// ─── runs ────────────────────────────────────────────

type gitStepLog struct {
	Step   string `json:"step"`
	Status string `json:"status"` // ok | fail | skip | info
	Msg    string `json:"msg,omitempty"`
	At     string `json:"at"`
}

// gitRunItem is one installation of a run: the request (install_id, files, modified_ok,
// backup) and its result.
type gitRunItem struct {
	ReviewID    string       `json:"review_id,omitempty"`
	InstallID   int          `json:"install_id"`
	Files       []string     `json:"files,omitempty"`
	ModifiedOK  []string     `json:"modified_ok,omitempty"`
	Backup      string       `json:"backup,omitempty"`
	ConnID      int          `json:"conn_id"`
	ConnName    string       `json:"conn_name"`
	App         string       `json:"app"`
	Path        string       `json:"path"`
	Env         string       `json:"env,omitempty"`
	Prod        bool         `json:"prod,omitempty"`
	Units       []gitUnit    `json:"units,omitempty"`
	State       string       `json:"state"` // pending | running | ok | failed | rolled_back | skipped | cancelled
	Step        string       `json:"step,omitempty"`
	Log         []gitStepLog `json:"log"`
	Written     []string     `json:"written,omitempty"`
	Deleted     []string     `json:"deleted,omitempty"`
	MadeBackup  string       `json:"made_backup,omitempty"`
	FromVersion string       `json:"from_version,omitempty"`
	ToVersion   string       `json:"to_version,omitempty"`
	Dangling    []string     `json:"dangling,omitempty"`
	Health      []gitHealth  `json:"health,omitempty"`
	UpdateLog   string       `json:"update_log,omitempty"`
	Error       string       `json:"error,omitempty"`
	// transfer: where the installation comes from
	SourceConnName string `json:"source_conn_name,omitempty"`
	SourcePath     string `json:"source_path,omitempty"`
	// deploy method "CI pipeline": the triggered job or pipeline
	Via      string `json:"via,omitempty"` // ci
	CIKind   string `json:"ci_kind,omitempty"`
	CIURL    string `json:"ci_url,omitempty"`
	CIStatus string `json:"ci_status,omitempty"`
	// environment deploys and rollbacks (git_env_run.go)
	Dest        int      `json:"dest,omitempty"` // index of the destination in the environment
	StateDir    string   `json:"state_dir,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"` // of the reviewed plan
	DeployID    string   `json:"deploy_id,omitempty"`
	Partial     bool     `json:"partial,omitempty"` // the transfer was interrupted
	Moved       []string `json:"moved,omitempty"`   // what had arrived before
	MovedCount  int      `json:"moved_count,omitempty"`
	Skipped     []string `json:"skipped,omitempty"` // rollback: symlinks left alone
	PostDeploy  string   `json:"post_deploy,omitempty"`
	Note        string   `json:"note,omitempty"`
}

type gitRestartChoice struct {
	Mode         string `json:"mode"` // none | now | at | delay
	At           string `json:"at,omitempty"`
	DelayMinutes int    `json:"delay_minutes,omitempty"`
}

// gitRunParams is the request of a run and what is stored with it.
type gitRunParams struct {
	Kind           string               `json:"kind"` // update | upgrade | rollback | stamp | restart
	Ref            string               `json:"ref,omitempty"`
	Items          []*gitRunItem        `json:"items"`
	Restart        gitRestartChoice     `json:"restart"`
	Checks         map[string]string    `json:"checks,omitempty"` // service → custom check command
	IgnoreImports  bool                 `json:"ignore_imports,omitempty"`
	Force          bool                 `json:"force,omitempty"`         // stamp: overwrite VERSION.md
	StampPartial   bool                 `json:"stamp_partial,omitempty"` // stamp: also installations that are not up to date
	ScheduleAt     string               `json:"schedule_at,omitempty"`
	Confirm        map[string]string    `json:"confirm,omitempty"` // install id → typed host name
	SetRefOverride bool                 `json:"set_ref_override,omitempty"`
	Targets        map[string]gitTarget `json:"targets,omitempty"`   // pinned when the run is created
	Provision      *gitProvision        `json:"provision,omitempty"` // install / transfer (stored without typed values)
	Env            *gitEnvRun           `json:"env,omitempty"`       // environment deploy or rollback
}

func (run *gitRun) dryRun() bool { return run.P.Env != nil && run.P.Env.DryRun }

// gitRun is a run being worked on.
type gitRun struct {
	mu       sync.Mutex
	ID       int
	UserID   int
	Kind     string
	State    string
	Label    string
	ParentID int
	P        gitRunParams
	Message  string
	Started  string
	cancel   bool
	r        *http.Request
}

func (run *gitRun) set(f func()) {
	run.mu.Lock()
	f()
	run.mu.Unlock()
}

var gitLive = struct {
	sync.Mutex
	runs map[int]*gitRun // run id → running run
	busy map[int]int     // user id → running run id
}{runs: map[int]*gitRun{}, busy: map[int]int{}}

// gitRunView is what the API returns for a run.
type gitRunView struct {
	ID          int               `json:"id"`
	Kind        string            `json:"kind"`
	State       string            `json:"state"`
	Label       string            `json:"label"`
	ParentID    int               `json:"parent_id,omitempty"`
	Ref         string            `json:"ref,omitempty"`
	Restart     gitRestartChoice  `json:"restart"`
	Message     string            `json:"message,omitempty"`
	CreatedAt   string            `json:"created_at"`
	ScheduledAt string            `json:"scheduled_at,omitempty"`
	StartedAt   string            `json:"started_at,omitempty"`
	EndedAt     string            `json:"ended_at,omitempty"`
	Running     bool              `json:"running"`
	Versions    map[string]string `json:"versions,omitempty"` // service → target version
	Env         *gitEnvRun        `json:"env,omitempty"`
	Items       []*gitRunItem     `json:"items"`
}

func loadGitRunView(userID, id int) (*gitRunView, *gitRunParams, bool) {
	var v gitRunView
	var params, result string
	err := db.QueryRow(`SELECT id, kind, state, label, parent_id, params, result, message, created_at, scheduled_at, started_at, ended_at FROM git_runs WHERE id=? AND user_id=?`,
		id, userID).Scan(&v.ID, &v.Kind, &v.State, &v.Label, &v.ParentID, &params, &result, &v.Message, &v.CreatedAt, &v.ScheduledAt, &v.StartedAt, &v.EndedAt)
	if err != nil {
		return nil, nil, false
	}
	var p gitRunParams
	jsonUnmarshalString(params, &p)
	v.Ref, v.Restart, v.Env = p.Ref, p.Restart, p.Env
	v.Items = p.Items
	if result != "" {
		var items []*gitRunItem
		jsonUnmarshalString(result, &items)
		v.Items = items
	}
	gitLive.Lock()
	live := gitLive.runs[id]
	gitLive.Unlock()
	if live != nil {
		live.mu.Lock()
		b, _ := json.Marshal(live.P.Items)
		v.State, v.Message, v.StartedAt, v.Running = live.State, live.Message, live.Started, true
		live.mu.Unlock()
		var items []*gitRunItem
		json.Unmarshal(b, &items)
		v.Items = items
	}
	if v.Items == nil {
		v.Items = []*gitRunItem{}
	}
	if len(p.Targets) > 0 {
		v.Versions = map[string]string{}
		for app, t := range p.Targets {
			v.Versions[app] = t.Version
		}
	}
	return &v, &p, true
}

func loadGitRuns(userID, limit int) []*gitRunView {
	out := []*gitRunView{}
	rows, err := db.Query(`SELECT id FROM git_runs WHERE user_id=? ORDER BY CASE WHEN state='scheduled' THEN 0 ELSE 1 END, id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return out
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if v, _, ok := loadGitRunView(userID, id); ok {
			for _, it := range v.Items {
				it.Log = nil // the list shows states only
			}
			out = append(out, v)
		}
	}
	return out
}

// ─── creating a run ──────────────────────────────────

var gitBackupNameRe = strings.NewReplacer("/", "", "\\", "", "\n", "", "\r", "", "\t", "")

func parseRunTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return t, fmt.Errorf("invalid time %q (RFC 3339 expected)", s)
	}
	return t, nil
}

// createGitRun validates a request, pins the targets and starts or schedules the run.
func createGitRun(r *http.Request, userID int, p gitRunParams) (int, int, error) {
	p.Targets = nil
	if p.Kind == "install" || p.Kind == "transfer" {
		return createProvisionRun(r, userID, p)
	}
	p.Provision = nil
	if p.Env != nil {
		if p.Kind != "update" && p.Kind != "upgrade" && p.Kind != "rollback" {
			return 0, 400, fmt.Errorf("environments support update, upgrade and rollback")
		}
		return createEnvRun(r, userID, p)
	}
	key, known := gitKindPolicy[p.Kind]
	if !known {
		return 0, 400, fmt.Errorf("unknown kind")
	}
	if !gitActionAllowed(userID, key) {
		return 0, 403, fmt.Errorf("this action is not allowed for your account")
	}
	if len(p.Items) == 0 || len(p.Items) > gitMaxRunItems {
		return 0, 400, fmt.Errorf("choose 1 to %d installations", gitMaxRunItems)
	}
	if p.Restart.Mode == "" {
		p.Restart.Mode = "none"
	}
	now := time.Now()
	var schedAt time.Time
	if strings.TrimSpace(p.ScheduleAt) != "" {
		t, err := parseRunTime(p.ScheduleAt)
		if err != nil {
			return 0, 400, err
		}
		if t.Before(now.Add(-time.Minute)) || t.After(now.Add(366*24*time.Hour)) {
			return 0, 400, fmt.Errorf("the scheduled time must be in the future (within a year)")
		}
		schedAt = t
		p.ScheduleAt = t.UTC().Format(time.RFC3339)
	} else {
		p.ScheduleAt = ""
	}
	switch p.Restart.Mode {
	case "none":
	case "now", "at", "delay":
		if p.Kind != "restart" && !gitActionAllowed(userID, "git_restart") {
			return 0, 403, fmt.Errorf("restarts are not allowed for your account")
		}
		if p.Restart.Mode == "at" {
			t, err := parseRunTime(p.Restart.At)
			if err != nil {
				return 0, 400, err
			}
			if t.Before(now) || (!schedAt.IsZero() && t.Before(schedAt)) {
				return 0, 400, fmt.Errorf("the restart time must be after the update")
			}
			p.Restart.At = t.UTC().Format(time.RFC3339)
		}
		if p.Restart.Mode == "delay" && (p.Restart.DelayMinutes < 1 || p.Restart.DelayMinutes > 7*24*60) {
			return 0, 400, fmt.Errorf("the delay must be 1 minute to 7 days")
		}
	default:
		return 0, 400, fmt.Errorf("unknown restart choice")
	}
	if p.Kind == "restart" && p.Restart.Mode == "none" {
		p.Restart.Mode = "now"
	}
	if p.Kind == "restart" && p.ScheduleAt != "" {
		return 0, 400, fmt.Errorf("choose the restart time with the restart choice")
	}
	checks := map[string]string{}
	for app, c := range p.Checks {
		if c = strings.TrimSpace(c); c != "" {
			if len(c) > 500 || strings.ContainsAny(c, "\n\r") {
				return 0, 400, fmt.Errorf("the check command must be one line of at most 500 characters")
			}
			checks[app] = c
		}
	}
	p.Checks = checks
	cat, st := loadGitWorkspace(userID)
	seen := map[int]bool{}
	apps := map[string]bool{}
	for _, it := range p.Items {
		if seen[it.InstallID] {
			return 0, 400, fmt.Errorf("an installation is listed twice")
		}
		seen[it.InstallID] = true
		list := loadGitInstalls(userID, false, "i.id=?", it.InstallID)
		if len(list) == 0 {
			return 0, 404, fmt.Errorf("installation not found")
		}
		in := list[0]
		if !userOwnsConnection(in.ConnID, userID) {
			return 0, 404, fmt.Errorf("connection not found")
		}
		it.ConnID, it.ConnName, it.App, it.Path, it.Env, it.Prod = in.ConnID, in.ConnName, in.App, in.Path, in.Env, isProdInstall(in)
		it.Units = in.Units
		it.State, it.Log, it.Step = "pending", []gitStepLog{}, ""
		it.Written, it.Deleted, it.MadeBackup, it.Error, it.Health, it.Dangling, it.UpdateLog = nil, nil, "", "", nil, nil, ""
		it.Via, it.CIKind, it.CIURL, it.CIStatus = "", "", "", ""
		if p.Kind == "update" || p.Kind == "upgrade" {
			if _, ci := loadGitPipeline(userID, in.App); ci {
				it.Via = "ci" // the service is deployed by its CI pipeline
			}
		}
		files := []string{}
		for _, f := range it.Files {
			if f = cleanRel(f); f != "" && !strings.HasPrefix(f, "..") {
				files = append(files, f)
			}
		}
		it.Files = files
		if it.Prod && !strings.EqualFold(strings.TrimSpace(p.Confirm[strconv.Itoa(it.InstallID)]), strings.TrimSpace(in.ConnName)) {
			return 0, 400, fmt.Errorf("%s is a production installation: type the server name %q to confirm", in.Path, in.ConnName)
		}
		switch p.Kind {
		case "rollback":
			it.Backup = gitBackupNameRe.Replace(strings.TrimSpace(it.Backup))
			if it.Backup == "" || it.Backup == "." || it.Backup == ".." {
				return 0, 400, fmt.Errorf("choose the backup to restore")
			}
		case "restart":
			if len(in.Units) == 0 {
				return 0, 400, fmt.Errorf("%s: no systemd or supervisor unit is known (check the server first)", in.Path)
			}
		}
		apps[in.App] = true
	}
	// pin the targets
	if p.Kind == "update" || p.Kind == "stamp" || p.Kind == "upgrade" {
		p.Targets = map[string]gitTarget{}
		for app := range apps {
			var t gitTarget
			var ok bool

			var reviewed *gitTarget
			for _, item := range p.Items {
				if item.App != app {
					continue
				}
				if item.ReviewID == "" {
					current, _ := loadGitTarget(userID, app)
					if loadGitRelayMeta(current.SourceID).Enabled || loadGitRelayMeta(st.SourceID).Enabled {
						return 0, 409, fmt.Errorf("client-cache deployment requires a reviewed plan; review again")
					}
					continue
				}
				original, err := loadGitReview(userID, item.InstallID, p.Kind, p.Ref, item.ReviewID)
				if err != nil {
					return 0, 409, err
				}
				if reviewed != nil && reviewedContentHash(*reviewed) != reviewedContentHash(original) {
					return 0, 409, fmt.Errorf("installations were reviewed against different targets; review together again")
				}
				reviewed = &original
			}
			if reviewed != nil {
				t = *reviewed
				ok = true
			} else {
				if p.Kind == "upgrade" {
					var err error
					if t, err = upgradeTarget(userID, cat, st, app, p.Ref); err != nil {
						return 0, 400, err
					}
					ok = true
				} else {
					t, ok = loadGitTarget(userID, app)
				}
			}
			if !ok || t.Error != "" || len(t.Files) == 0 {
				return 0, 400, fmt.Errorf("%s: no target yet (refresh the targets)", app)
			}
			p.Targets[app] = t
		}
	}
	if p.Kind == "upgrade" {
		p.Ref = strings.TrimSpace(p.Ref)
	}
	// remember the custom checks per service
	if len(p.Checks) > 0 {
		if st.CheckCommands == nil {
			st.CheckCommands = map[string]string{}
		}
		for app, c := range p.Checks {
			st.CheckCommands[app] = c
		}
		saveGitWorkspace(userID, nil, &st)
	}
	p.Confirm = nil
	label := gitRunLabel(now)
	state, sched := "running", ""
	if p.Kind == "restart" {
		switch p.Restart.Mode {
		case "at":
			state, sched = "scheduled", p.Restart.At
		case "delay":
			state, sched = "scheduled", now.Add(time.Duration(p.Restart.DelayMinutes)*time.Minute).UTC().Format(time.RFC3339)
		}
	} else if p.ScheduleAt != "" {
		state, sched = "scheduled", p.ScheduleAt
	}
	if state == "running" {
		gitLive.Lock()
		busy := gitLive.busy[userID] != 0
		gitLive.Unlock()
		if busy {
			return 0, 409, fmt.Errorf("another run is working: wait until it ends")
		}
	}
	res, err := db.Exec(`INSERT INTO git_runs (user_id, kind, state, label, parent_id, params, result, message, created_at, scheduled_at, started_at, ended_at, notified_at)
		VALUES (?,?,?,?,0,?,'','',?,?,'','','')`, userID, p.Kind, map[bool]string{true: "starting", false: "scheduled"}[state == "running"], label, string(jsonMarshal(p)), nowStamp(), sched)
	if err != nil {
		return 0, 500, err
	}
	id64, _ := res.LastInsertId()
	id := int(id64)
	pruneGitRuns(userID)
	// a restart at a time or after a delay is a run of its own, waiting for this one
	if p.Kind != "restart" && (p.Restart.Mode == "at" || p.Restart.Mode == "delay") {
		child := gitRunParams{Kind: "restart", Restart: p.Restart}
		for _, it := range p.Items {
			if len(it.Units) > 0 {
				child.Items = append(child.Items, &gitRunItem{InstallID: it.InstallID, ConnID: it.ConnID, ConnName: it.ConnName, App: it.App, Path: it.Path,
					Env: it.Env, Prod: it.Prod, Units: it.Units, State: "pending", Log: []gitStepLog{}})
			}
		}
		if len(child.Items) > 0 {
			csched := ""
			if p.Restart.Mode == "at" {
				csched = p.Restart.At
			}
			db.Exec(`INSERT INTO git_runs (user_id, kind, state, label, parent_id, params, result, message, created_at, scheduled_at, started_at, ended_at, notified_at)
				VALUES (?,'restart','scheduled',?,?,?,'','',?,?,'','','')`, userID, label, id, string(jsonMarshal(child)), nowStamp(), csched)
		}
	}
	details := map[string]interface{}{"run_id": id, "kind": p.Kind, "installs": len(p.Items), "restart": p.Restart.Mode}
	if p.Ref != "" {
		details["ref"] = p.Ref
	}
	if sched != "" {
		details["scheduled_at"] = sched
		auditLog(r, userID, "git.run_scheduled", label, details)
		if p.Kind == "restart" {
			notifyRestartScheduled(userID, id, label, p.Items, sched)
		}
	}
	if state == "running" {
		if err := startGitRun(userID, id, r); err != nil {
			db.Exec(`UPDATE git_runs SET state='failed', message=?, ended_at=? WHERE id=?`, err.Error(), nowStamp(), id)
			db.Exec(`UPDATE git_runs SET state='skipped', message='skipped: the update did not start', ended_at=? WHERE parent_id=? AND state='scheduled'`, nowStamp(), id)
			return 0, 409, err
		}
	}
	return id, 200, nil
}

func pruneGitRuns(userID int) {
	db.Exec(`DELETE FROM git_runs WHERE user_id=? AND state NOT IN ('scheduled','running','starting') AND id NOT IN
		(SELECT id FROM git_runs WHERE user_id=? ORDER BY id DESC LIMIT ?)`, userID, userID, gitKeepRuns)
}

// ─── upgrade targets (cached for the plan → run step) ─

var gitPlanCache = struct {
	sync.Mutex
	m map[string]gitPlanEntry
}{m: map[string]gitPlanEntry{}}

type gitPlanEntry struct {
	t  gitTarget
	at time.Time
}

// upgradeTarget computes the target of a service at another ref (Git API only).
func upgradeTarget(userID int, cat *gitCatalog, st gitSettings, app, ref string) (gitTarget, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || len(ref) > 200 || strings.ContainsAny(ref, " \n\r\t") {
		return gitTarget{}, fmt.Errorf("enter the branch or tag (tag:v2) to upgrade to")
	}
	key := fmt.Sprintf("%d|%s|%s", userID, app, ref)
	gitPlanCache.Lock()
	e, ok := gitPlanCache.m[key]
	gitPlanCache.Unlock()
	if ok && time.Since(e.at) < gitPlanCacheTTL {
		return e.t, nil
	}
	var a *gitCatalogApp
	for _, x := range effectiveApps(userID, cat) {
		if x.Name == app {
			x := x
			a = &x
		}
	}
	if a == nil {
		return gitTarget{}, fmt.Errorf("%s is not in the catalog", app)
	}
	api, _ := pickSources(userID, st)
	if api == nil {
		return gitTarget{}, fmt.Errorf("an upgrade needs a GitLab or GitHub source (Settings)")
	}
	p, err := newGitProvider(*api)
	if err != nil {
		return gitTarget{}, err
	}
	st2 := st
	st2.RefOverrides = map[string]string{app: ref}
	t := computeTarget(p, *api, cat, *a, st2)
	if t.Error != "" {
		return t, fmt.Errorf("%s at %s: %s", app, ref, t.Error)
	}
	gitPlanCache.Lock()
	for k, x := range gitPlanCache.m {
		if time.Since(x.at) > gitPlanCacheTTL {
			delete(gitPlanCache.m, k)
		}
	}
	gitPlanCache.m[key] = gitPlanEntry{t: t, at: time.Now()}
	gitPlanCache.Unlock()
	return t, nil
}

// ─── plan ────────────────────────────────────────────

type gitPlanItem struct {
	ReviewID       string          `json:"review_id,omitempty"`
	InstallID      int             `json:"install_id"`
	ConnName       string          `json:"conn_name"`
	App            string          `json:"app"`
	Path           string          `json:"path"`
	Env            string          `json:"env,omitempty"`
	Prod           bool            `json:"prod"`
	State          string          `json:"state"`
	Units          []gitUnit       `json:"units"`
	FromVersion    string          `json:"from_version"`
	Target         *gitTargetBrief `json:"target,omitempty"`
	Current        *gitTargetBrief `json:"current,omitempty"`
	Files          []planFile      `json:"files"`
	Removed        []string        `json:"removed,omitempty"` // upgrade: files of the current target the new one does not have
	Counts         map[string]int  `json:"counts"`
	Eligible       bool            `json:"eligible"`           // stamp: up to date
	Pipeline       *gitPipeline    `json:"pipeline,omitempty"` // deployed by CI: the run triggers it instead of writing files
	HasVersionMD   bool            `json:"has_version_md"`
	RestartPending string          `json:"restart_pending,omitempty"`
	Error          string          `json:"error,omitempty"`
}

func gitPlan(userID int, kind, ref string, ids []int) ([]gitPlanItem, error) {
	cat, st := loadGitWorkspace(userID)
	out := []gitPlanItem{}
	if len(ids) == 0 || len(ids) > gitMaxRunItems {
		return nil, fmt.Errorf("choose 1 to %d installations", gitMaxRunItems)
	}
	for _, id := range ids {
		list := loadGitInstalls(userID, true, "i.id=?", id)
		if len(list) == 0 {
			return nil, fmt.Errorf("installation not found")
		}
		in := list[0]
		pi := gitPlanItem{InstallID: in.ID, ConnName: in.ConnName, App: in.App, Path: in.Path, Env: in.Env, Prod: isProdInstall(in), State: in.State,
			Units: in.Units, FromVersion: in.VersionMD.Version, Files: []planFile{}, Counts: map[string]int{}, RestartPending: in.RestartPending,
			HasVersionMD: in.VersionMD.Version != "" || in.VersionMD.App != ""}
		server := map[string]string{}
		for _, f := range in.Files {
			if f.ServerHash != "" {
				server[f.Path] = f.ServerHash
			}
		}
		cur, haveCur := loadGitTarget(userID, in.App)
		if haveCur && cur.Error == "" {
			b := cur.brief()
			pi.Current = &b
		}
		var t gitTarget
		switch kind {
		case "update", "stamp":
			if !haveCur || cur.Error != "" || len(cur.Files) == 0 {
				pi.Error = "no target for this service yet (refresh the targets)"
				out = append(out, pi)
				continue
			}
			t = cur
		case "upgrade":
			var err error
			if t, err = upgradeTarget(userID, cat, st, in.App, ref); err != nil {
				pi.Error = err.Error()
				out = append(out, pi)
				continue
			}
			if haveCur {
				for rel := range cur.Files {
					if _, still := t.Files[rel]; !still && server[rel] != "" {
						pi.Removed = append(pi.Removed, rel)
					}
				}
				sort.Strings(pi.Removed)
			}
		default:
			return nil, fmt.Errorf("unknown kind")
		}
		b := t.brief()
		pi.Target = &b
		review, reviewErr := storeGitReview(userID, in.ID, kind, ref, t)
		if reviewErr != nil {
			return nil, reviewErr
		}
		pi.ReviewID = review
		if kind != "stamp" {
			if pl, ci := loadGitPipeline(userID, in.App); ci {
				pi.Pipeline = &pl
			}
		}
		var curp *gitTarget
		if haveCur {
			curp = &cur
		}
		if len(in.Files) == 0 && in.State != "ok" {
			pi.Error = "check this installation first"
		}
		pi.Files = planFiles(kind, t, server, curp, cat.IgnoreDirs)
		pi.Eligible = true
		for _, f := range pi.Files {
			pi.Counts[f.State]++
			if f.State != "ok" && f.State != "protected" {
				pi.Eligible = false
			}
		}
		out = append(out, pi)
	}
	return out, nil
}

// ─── running ─────────────────────────────────────────

// startGitRun marks a run as running and works on it in the background. full is the
// complete request when the stored one is redacted (install: typed values).
func startGitRun(userID, id int, r *http.Request, full ...*gitRunParams) error {
	var params, label, kind string
	var parent int
	if db.QueryRow(`SELECT params, label, kind, parent_id FROM git_runs WHERE id=? AND user_id=?`, id, userID).Scan(&params, &label, &kind, &parent) != nil {
		return fmt.Errorf("run not found")
	}
	run := &gitRun{ID: id, UserID: userID, Kind: kind, State: "running", Label: label, ParentID: parent, Started: nowStamp(), r: r}
	jsonUnmarshalString(params, &run.P)
	if len(full) == 1 && full[0] != nil {
		run.P = *full[0]
	}
	gitLive.Lock()
	if gitLive.busy[userID] != 0 {
		gitLive.Unlock()
		return fmt.Errorf("another run is working: wait until it ends")
	}
	gitLive.busy[userID] = id
	gitLive.runs[id] = run
	gitLive.Unlock()
	db.Exec(`UPDATE git_runs SET state='running', started_at=? WHERE id=?`, run.Started, id)
	go executeGitRun(run)
	return nil
}

func (run *gitRun) save() {
	run.mu.Lock()
	b := jsonMarshal(run.P.Items)
	run.mu.Unlock()
	db.Exec(`UPDATE git_runs SET result=? WHERE id=?`, string(b), run.ID)
}

func executeGitRun(run *gitRun) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("git run %d: %v", run.ID, rec)
			run.set(func() { run.State, run.Message = "failed", fmt.Sprint("internal error: ", rec) })
			finishGitRun(run)
		}
	}()
	if run.Kind != "restart" && !run.dryRun() {
		notifyGitRun(run, "git.deploy_started")
	}
	failed := false
	for i, it := range run.P.Items {
		run.mu.Lock()
		stop := run.cancel
		run.mu.Unlock()
		if stop || failed {
			st := "skipped"
			if stop {
				st = "cancelled"
			}
			run.set(func() { it.State = st })
			continue
		}
		run.set(func() { it.State = "running" })
		var err error
		switch run.Kind {
		case "update", "upgrade":
			if run.P.Env != nil {
				err = run.envDeployItem(it)
			} else if it.Via == "ci" {
				err = run.pipelineItem(it)
			} else {
				err = run.deployFiles(it)
			}
		case "rollback":
			if run.P.Env != nil {
				err = run.envRollbackItem(it)
			} else {
				err = run.rollbackItem(it)
			}
		case "stamp":
			err = run.stampItem(it)
		case "restart":
			err = run.restartItem(it)
		case "install":
			err = run.installItem(i, it)
		case "transfer":
			err = run.transferItem(it)
		}
		if err == nil && run.Kind != "restart" && run.Kind != "stamp" && run.P.Restart.Mode == "now" && it.State == "ok" && len(it.Units) > 0 && (len(it.Written) > 0 || len(it.Deleted) > 0) {
			err = run.restartItem(it)
		}
		if err != nil {
			run.set(func() {
				if it.State == "running" || it.State == "ok" {
					it.State = "failed"
				}
				it.Error = err.Error()
				it.Step = ""
				run.Message = fmt.Sprintf("%s on %s: %s", it.Path, it.ConnName, err.Error())
			})
			failed = true
		}
		auditGitItem(run, it)
		run.save()
	}
	run.set(func() {
		switch {
		case failed:
			run.State = "failed"
		case run.cancel:
			run.State = "cancelled"
			run.Message = "cancelled"
		default:
			run.State = "done"
		}
	})
	if run.Kind == "upgrade" && run.State == "done" && run.P.SetRefOverride {
		_, st := loadGitWorkspace(run.UserID)
		if st.RefOverrides == nil {
			st.RefOverrides = map[string]string{}
		}
		for app := range run.P.Targets {
			st.RefOverrides[app] = run.P.Ref
		}
		saveGitWorkspace(run.UserID, nil, &st)
	}
	finishGitRun(run)
}

func finishGitRun(run *gitRun) {
	run.save()
	run.mu.Lock()
	state, msg := run.State, run.Message
	run.mu.Unlock()
	db.Exec(`UPDATE git_runs SET state=?, message=?, ended_at=? WHERE id=?`, state, truncateStr(msg, 1000), nowStamp(), run.ID)
	gitLive.Lock()
	delete(gitLive.runs, run.ID)
	if gitLive.busy[run.UserID] == run.ID {
		delete(gitLive.busy, run.UserID)
	}
	gitLive.Unlock()
	if run.dryRun() {
		// a dry run changes nothing: no notification
	} else if run.Kind == "restart" {
		notifyGitRun(run, "git.restart_done")
	} else if state == "done" {
		notifyGitRun(run, "git.deploy_done")
	} else {
		notifyGitRun(run, "git.deploy_failed")
	}
	releaseChildRestarts(run)
	hub.sendTo(run.UserID, []byte(`{"type":"git_run_changed"}`))
}

// releaseChildRestarts schedules (or skips) the restart that waits for this run.
func releaseChildRestarts(run *gitRun) {
	rows, err := db.Query(`SELECT id, params, scheduled_at FROM git_runs WHERE parent_id=? AND user_id=? AND state='scheduled'`, run.ID, run.UserID)
	if err != nil {
		return
	}
	type child struct {
		id     int
		params string
		sched  string
	}
	var list []child
	for rows.Next() {
		var c child
		rows.Scan(&c.id, &c.params, &c.sched)
		list = append(list, c)
	}
	rows.Close()
	for _, c := range list {
		var p gitRunParams
		jsonUnmarshalString(c.params, &p)
		if run.State != "done" {
			db.Exec(`UPDATE git_runs SET state='skipped', message=?, ended_at=? WHERE id=?`, "skipped: the update did not succeed", nowStamp(), c.id)
			continue
		}
		changed := map[int]bool{}
		for _, it := range run.P.Items {
			if it.State == "ok" && (len(it.Written) > 0 || len(it.Deleted) > 0) {
				changed[it.InstallID] = true
			}
		}
		var keep []*gitRunItem
		for _, it := range p.Items {
			if changed[it.InstallID] {
				keep = append(keep, it)
			}
		}
		if len(keep) == 0 {
			db.Exec(`UPDATE git_runs SET state='skipped', message=?, ended_at=? WHERE id=?`, "skipped: nothing was changed", nowStamp(), c.id)
			continue
		}
		p.Items = keep
		sched := c.sched
		if p.Restart.Mode == "delay" {
			sched = time.Now().Add(time.Duration(p.Restart.DelayMinutes) * time.Minute).UTC().Format(time.RFC3339)
		}
		db.Exec(`UPDATE git_runs SET params=?, scheduled_at=? WHERE id=?`, string(jsonMarshal(p)), sched, c.id)
		notifyRestartScheduled(run.UserID, c.id, run.Label, keep, sched)
	}
}

// ─── one installation ────────────────────────────────

type itemCtx struct {
	run *gitRun
	it  *gitRunItem
}

func (c itemCtx) step(name string) { c.run.set(func() { c.it.Step = name }) }

func (c itemCtx) log(step, status, msg string) {
	c.run.set(func() {
		c.it.Log = append(c.it.Log, gitStepLog{Step: step, Status: status, Msg: truncateStr(msg, 2000), At: nowStamp()})
	})
}

// openInstall loads the installation and logs in to its server.
func (c itemCtx) openInstall() (gitInstall, *deploySession, error) {
	list := loadGitInstalls(c.run.UserID, true, "i.id=?", c.it.InstallID)
	if len(list) == 0 {
		return gitInstall{}, nil, fmt.Errorf("the installation is not known any more")
	}
	in := list[0]
	conn, err := loadConnection(in.ConnID)
	if err != nil || conn.UserID != c.run.UserID || !isSSHProtocol(conn) {
		return in, nil, fmt.Errorf("connection not found")
	}
	c.step("connect")
	cl, err := dialSSH(conn, nil)
	if err != nil {
		c.log("connect", "fail", err.Error())
		return in, nil, fmt.Errorf("cannot log in to %s: %v", conn.Name, err)
	}
	c.log("connect", "ok", conn.Name)
	return in, &deploySession{cl: cl, in: in}, nil
}

// scanOne hashes the files of the installation now.
func (c itemCtx) scanOne(s *deploySession) (*gitScanResult, error) {
	c.step("scan")
	scans, err := scanOn(s.cl, []string{s.in.Path})
	if err != nil {
		return nil, err
	}
	sr := scans[s.in.Path]
	if sr == nil || sr.Missing {
		return nil, fmt.Errorf("the directory %s does not exist", s.in.Path)
	}
	if sr.Capped {
		return nil, fmt.Errorf("the installation has more than %d files", gitMaxInstallFiles)
	}
	return sr, nil
}

// apply backs up, writes atomically, checks and rolls back on failure. It returns whether
// files were changed and an error (after the automatic rollback).
func (c itemCtx) apply(s *deploySession, ops []*deployOp, bundle string, now time.Time, check []string, custom string) error {
	var rels []string
	for _, op := range ops {
		if op.Existed {
			rels = append(rels, op.Rel)
		}
	}
	c.step("backup")
	if err := s.makeBackup(bundle, now, rels, true); err != nil {
		c.log("backup", "fail", err.Error())
		return err
	}
	c.run.set(func() { c.it.MadeBackup = s.backup })
	c.log("backup", "ok", fmt.Sprintf("%s (%d files)", s.backup, len(rels)))
	c.step("write")
	if err := s.stage(ops); err != nil {
		c.log("write", "fail", err.Error())
		if rerr := s.restore(ops, true); rerr != nil {
			c.log("rollback", "fail", rerr.Error())
			return fmt.Errorf("%v; cleaning up failed: %v", err, rerr)
		}
		c.run.set(func() { c.it.MadeBackup = "" })
		return err
	}
	rollback := func(cause error) error {
		c.step("rollback")
		if rerr := s.restore(ops, true); rerr != nil {
			c.log("rollback", "fail", rerr.Error())
			return fmt.Errorf("%v; THE AUTOMATIC ROLLBACK FAILED: %v", cause, rerr)
		}
		c.log("rollback", "ok", "the previous files are back")
		c.run.set(func() { c.it.State, c.it.MadeBackup = "rolled_back", "" })
		return fmt.Errorf("%v (rolled back)", cause)
	}
	if err := s.commit(ops); err != nil {
		c.log("write", "fail", err.Error())
		return rollback(err)
	}
	n := 0
	for _, op := range ops {
		if !op.Delete {
			n++
		}
	}
	c.log("write", "ok", fmt.Sprintf("%d written, %d deleted", n, len(ops)-n))
	c.step("checks")
	passed, err := s.checks(check, custom)
	if err != nil {
		c.log("checks", "fail", err.Error())
		return rollback(fmt.Errorf("check failed: %v", err))
	}
	if len(passed) > 0 {
		c.log("checks", "ok", strings.Join(passed, ", "))
	} else {
		c.log("checks", "skip", "no checks for these files")
	}
	return nil
}

// writeVersion writes VERSION.md atomically (rolled back with the files on failure).
func (c itemCtx) writeVersion(s *deploySession, ops []*deployOp, content []byte, existed bool) error {
	c.step("version")
	vop := &deployOp{Rel: "VERSION.md", Content: content, Existed: existed, Mode: "644"}
	err := s.stage([]*deployOp{vop})
	if err == nil {
		err = s.commit([]*deployOp{vop})
	}
	if err != nil {
		c.log("version", "fail", err.Error())
		if rerr := s.restore(append(ops, vop), true); rerr != nil {
			return fmt.Errorf("VERSION.md: %v; THE AUTOMATIC ROLLBACK FAILED: %v", err, rerr)
		}
		c.run.set(func() { c.it.State, c.it.MadeBackup = "rolled_back", "" })
		return fmt.Errorf("VERSION.md: %v (rolled back)", err)
	}
	c.log("version", "ok", "VERSION.md")
	return nil
}

func (c itemCtx) updateLog(s *deploySession, rec map[string]interface{}) {
	c.step("log")
	where, err := s.appendUpdateLog(pyJSONLine(rec))
	if err != nil {
		c.log("log", "fail", "updates.jsonl: "+err.Error())
		return
	}
	c.run.set(func() { c.it.UpdateLog = where })
	c.log("log", "ok", where)
}

// finishItem refreshes the stored state of the installation and the pending restart.
func (c itemCtx) finishItem(s *deploySession, changed bool) {
	refreshInstallState(c.run.UserID, s.in, s.cl)
	if changed && len(s.in.Units) > 0 {
		db.Exec(`INSERT INTO git_pending_restarts (install_id, user_id, since, run_id) VALUES (?,?,?,?)
			ON CONFLICT(install_id) DO UPDATE SET since=excluded.since, run_id=excluded.run_id`, s.in.ID, c.run.UserID, nowStamp(), c.run.ID)
	}
	c.run.set(func() {
		c.it.State = "ok"
		c.it.Step = ""
	})
}

func (run *gitRun) deployFiles(it *gitRunItem) error {
	c := itemCtx{run, it}
	t, ok := run.P.Targets[it.App]
	if !ok {
		return fmt.Errorf("no target for %s", it.App)
	}
	in, s, err := c.openInstall()
	if err != nil {
		return err
	}
	defer s.cl.Close()
	sr, err := c.scanOne(s)
	if err != nil {
		c.log("scan", "fail", err.Error())
		return err
	}
	cat := loadGitCatalog(run.UserID)
	var cur *gitTarget
	if run.Kind == "upgrade" {
		if x, have := loadGitTarget(run.UserID, in.App); have && x.Error == "" {
			cur = &x
		}
	}
	plan := planFiles(run.Kind, t, sr.Files, cur, cat.IgnoreDirs)
	sel, err := selectRunFiles(plan, it)
	from := parseVersionMD(sr.Version).Version
	if from == "" {
		from = "-"
	}
	run.set(func() { it.FromVersion, it.ToVersion = from, t.Version })
	if err != nil {
		c.log("scan", "fail", err.Error())
		return err
	}
	if len(sel) == 0 {
		c.log("scan", "ok", "nothing to change: the selected files are up to date")
		run.set(func() { it.State, it.Step = "ok", "" })
		return nil
	}
	sort.Slice(sel, func(i, j int) bool { return sel[i].Path < sel[j].Path })
	if len(sel) > gitMaxDeployFiles {
		return fmt.Errorf("at most %d files per installation", gitMaxDeployFiles)
	}
	c.log("scan", "ok", fmt.Sprintf("%d files to change", len(sel)))
	providers := map[int]*gitProvider{}
	contents := map[string][]byte{}
	for _, p := range sel {
		data, err := gitFileContent(run.UserID, t, p.Path, providers)
		if err != nil {
			return err
		}
		contents[p.Path] = data
	}
	var existing []string
	for rel := range sr.Files {
		if _, inTarget := t.Files[rel]; inTarget || rel == "VERSION.md" {
			existing = append(existing, rel)
		}
	}
	sort.Strings(existing)
	wantPy := false
	for rel := range contents {
		wantPy = wantPy || strings.HasSuffix(rel, ".py")
	}
	c.step("prepare")
	if err := s.prepare(existing, wantPy && !run.P.IgnoreImports); err != nil {
		c.log("prepare", "fail", err.Error())
		return err
	}
	if wantPy {
		if run.P.IgnoreImports {
			c.log("imports", "skip", "the import check was turned off for this run")
		} else {
			c.step("imports")
			var imports []pyImport
			for _, im := range s.imports {
				if _, replaced := contents[im.File]; !replaced {
					imports = append(imports, im)
				}
			}
			files := map[string]bool{}
			for rel := range sr.Files {
				files[rel] = true
			}
			for rel, data := range contents {
				files[rel] = true
				if strings.HasSuffix(rel, ".py") {
					imports = append(imports, pyImportsOf(rel, data)...)
				}
			}
			if d := danglingImports(contents, imports, files); len(d) > 0 {
				run.set(func() { it.Dangling = d })
				c.log("imports", "fail", strings.Join(d, "\n"))
				return fmt.Errorf("dangling imports: %s", strings.Join(d, "; "))
			}
			c.log("imports", "ok", "")
		}
	}
	var ops []*deployOp
	var rels []string
	for _, p := range sel {
		_, existed := sr.Files[p.Path]
		data := withLineEndings(contents[p.Path], s.crlfFor(p.Path))
		mode := "644"
		if strings.HasPrefix(string(data), "#!") {
			mode = "755"
		}
		ops = append(ops, &deployOp{Rel: p.Path, Content: data, Existed: existed, Mode: mode})
		rels = append(rels, p.Path)
	}
	now := time.Now()
	bundle := bundleOf(t, run.Label)
	if err := c.apply(s, ops, bundle, now, rels, run.P.Checks[in.App]); err != nil {
		return err
	}
	written := map[string]bool{}
	for _, r := range rels {
		written[r] = true
	}
	_, vmdExists := sr.Files["VERSION.md"]
	vcrlf := s.mostlyCRLF()
	if c, known := s.crlf["VERSION.md"]; known {
		vcrlf = c
	}
	content := versionMD(t, versionRows(t, plan, written), now, s.host, s.user, bundle, vcrlf)
	if err := c.writeVersion(s, ops, content, vmdExists); err != nil {
		return err
	}
	run.set(func() { it.Written = rels })
	c.updateLog(s, map[string]interface{}{"app": in.App, "backup": s.backup, "branch": t.Branch, "bundle": bundle, "env": in.Env,
		"files": rels, "from_version": from, "host": s.host, "install": in.Path, "services": serviceNames(in.Units),
		"time": now.Format("2006-01-02T15:04:05"), "to_version": fmt.Sprintf("%s (%s)", short10(t.Commit), t.CommitDate), "user": s.user})
	c.finishItem(s, true)
	return nil
}

// rollbackFiles finds the files a backup belongs to: the run that made it, else the
// updates.jsonl line, else the files in the backup (then new files cannot be removed).
func rollbackFiles(userID int, in gitInstall, b gitBackup) ([]string, string) {
	full := in.Path + "/" + gitBackupDir + "/" + b.Name
	rows, err := db.Query(`SELECT result FROM git_runs WHERE user_id=? AND result LIKE ? ORDER BY id DESC LIMIT 20`, userID, "%"+full+"%")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var res string
			rows.Scan(&res)
			var items []*gitRunItem
			jsonUnmarshalString(res, &items)
			for _, it := range items {
				if it.InstallID == in.ID && it.MadeBackup == full {
					files := append(append([]string{}, it.Written...), it.Deleted...)
					if len(files) > 0 {
						return files, "run"
					}
				}
			}
		}
	}
	if b.LogFiles != nil {
		return b.LogFiles, "updates.jsonl"
	}
	return b.Files, "backup"
}

func (run *gitRun) rollbackItem(it *gitRunItem) error {
	c := itemCtx{run, it}
	in, s, err := c.openInstall()
	if err != nil {
		return err
	}
	defer s.cl.Close()
	c.step("backups")
	backups, err := listBackups(s.cl, in.Path)
	if err != nil {
		return err
	}
	var b *gitBackup
	for i := range backups {
		if backups[i].Name == it.Backup {
			b = &backups[i]
		}
	}
	if b == nil {
		return fmt.Errorf("backup %s not found in %s/%s", it.Backup, in.Path, gitBackupDir)
	}
	files, from := rollbackFiles(run.UserID, in, *b)
	if from == "backup" {
		c.log("backups", "info", "no record of this backup: only the files in it are restored (files added by that update stay)")
	}
	sr, err := c.scanOne(s)
	if err != nil {
		return err
	}
	inBackup := map[string]bool{}
	for _, f := range b.Files {
		inBackup[f] = true
	}
	src := in.Path + "/" + gitBackupDir + "/" + b.Name + "/"
	var ops []*deployOp
	var restored, deleted, check []string
	for _, rel := range files {
		rel = cleanRel(rel)
		if rel == "" || strings.HasPrefix(rel, "..") || toolFiles[rel] {
			continue
		}
		_, exists := sr.Files[rel]
		switch {
		case inBackup[rel]:
			ops = append(ops, &deployOp{Rel: rel, From: src + rel, Existed: exists})
			restored = append(restored, rel)
			check = append(check, rel)
		case exists:
			ops = append(ops, &deployOp{Rel: rel, Delete: true, Existed: true})
			deleted = append(deleted, rel)
		}
	}
	cur := parseVersionMD(sr.Version)
	fromV := cur.Version
	if fromV == "" {
		fromV = "-"
	}
	toV := b.Version
	if toV == "" {
		toV = "-"
	}
	run.set(func() { it.FromVersion, it.ToVersion = fromV, toV })
	if len(ops) == 0 {
		return fmt.Errorf("the backup has nothing to restore")
	}
	c.log("backups", "ok", fmt.Sprintf("%s: %d to restore, %d to delete", b.Name, len(restored), len(deleted)))
	var existing []string
	for rel := range sr.Files {
		existing = append(existing, rel)
	}
	c.step("prepare")
	if err := s.prepare(existing, false); err != nil {
		return err
	}
	now := time.Now()
	if err := c.apply(s, ops, run.Label, now, check, run.P.Checks[in.App]); err != nil {
		return err
	}
	_, vmdExists := sr.Files["VERSION.md"]
	var vop *deployOp
	switch {
	case b.Version != "" || b.Commit != "":
		vop = &deployOp{Rel: "VERSION.md", From: src + "VERSION.md", Existed: vmdExists}
	case from != "backup" && vmdExists:
		// the update that made the backup wrote VERSION.md where there was none
		vop = &deployOp{Rel: "VERSION.md", Delete: true, Existed: true}
	}
	if vop != nil {
		c.step("version")
		err := s.stage([]*deployOp{vop})
		if err == nil {
			err = s.commit([]*deployOp{vop})
		}
		if err != nil {
			if rerr := s.restore(append(ops, vop), true); rerr != nil {
				return fmt.Errorf("VERSION.md: %v; THE AUTOMATIC ROLLBACK FAILED: %v", err, rerr)
			}
			run.set(func() { it.State, it.MadeBackup = "rolled_back", "" })
			return fmt.Errorf("VERSION.md: %v (rolled back)", err)
		}
		c.log("version", "ok", "VERSION.md as in the backup")
	}
	run.set(func() { it.Written, it.Deleted = restored, deleted })
	toCommit := b.Commit
	if toCommit == "" {
		toCommit = "-"
	}
	c.updateLog(s, map[string]interface{}{"app": in.App, "backup": s.backup, "branch": b.Branch, "bundle": run.Label, "env": in.Env,
		"files": append(append([]string{}, restored...), deleted...), "from_version": fromV, "host": s.host, "install": in.Path,
		"services": serviceNames(in.Units), "time": now.Format("2006-01-02T15:04:05"), "to_version": toCommit, "user": s.user})
	c.finishItem(s, true)
	return nil
}

// stampItem writes VERSION.md where the installation is up to date (and the file is
// missing, unless forced).
func (run *gitRun) stampItem(it *gitRunItem) error {
	c := itemCtx{run, it}
	t, ok := run.P.Targets[it.App]
	if !ok {
		return fmt.Errorf("no target for %s", it.App)
	}
	_, s, err := c.openInstall()
	if err != nil {
		return err
	}
	defer s.cl.Close()
	sr, err := c.scanOne(s)
	if err != nil {
		return err
	}
	cat := loadGitCatalog(run.UserID)
	plan := planFiles("update", t, sr.Files, nil, cat.IgnoreDirs)
	run.set(func() { it.FromVersion, it.ToVersion = parseVersionMD(sr.Version).Version, t.Version })
	differ := 0
	for _, p := range plan {
		if p.State != "ok" && p.State != "protected" {
			if !run.P.StampPartial {
				c.log("scan", "skip", "not up to date: "+p.Path+" is "+p.State)
				run.set(func() {
					it.State, it.Step, it.Error = "skipped", "", "not up to date (update it first, or stamp it as it is)"
				})
				return nil
			}
			differ++
		}
	}
	note := ""
	if differ > 0 {
		// VERSION.md then records each file's actual version; the note says the header
		// version is not complete on this server.
		note = fmt.Sprintf("djelomicno: %d datoteka se razlikuje od verzije %s", differ, t.Version)
		c.log("scan", "ok", fmt.Sprintf("stamping as it is: %d file(s) differ from the target", differ))
	}
	_, vmdExists := sr.Files["VERSION.md"]
	if vmdExists && !run.P.Force {
		c.log("scan", "skip", "VERSION.md exists")
		run.set(func() { it.State, it.Step, it.Error = "skipped", "", "VERSION.md exists (use force to overwrite it)" })
		return nil
	}
	var existing []string
	for rel := range sr.Files {
		existing = append(existing, rel)
	}
	c.step("prepare")
	if err := s.prepare(existing, false); err != nil {
		return err
	}
	vcrlf := s.mostlyCRLF()
	if c, known := s.crlf["VERSION.md"]; known {
		vcrlf = c
	}
	content := versionMDNote(t, versionRows(t, plan, nil), time.Now(), s.host, s.user, bundleOf(t, run.Label), note, vcrlf)
	c.step("version")
	vop := &deployOp{Rel: "VERSION.md", Content: content, Existed: vmdExists, Mode: "644"}
	if err := s.stage([]*deployOp{vop}); err != nil {
		return err
	}
	if err := s.commit([]*deployOp{vop}); err != nil {
		return err
	}
	c.log("version", "ok", "VERSION.md")
	c.finishItem(s, false)
	return nil
}

func (run *gitRun) restartItem(it *gitRunItem) error {
	c := itemCtx{run, it}
	if !gitActionAllowed(run.UserID, "git_restart") {
		return fmt.Errorf("restarts are not allowed for your account")
	}
	list := loadGitInstalls(run.UserID, false, "i.id=?", it.InstallID)
	if len(list) == 0 {
		return fmt.Errorf("the installation is not known any more")
	}
	conn, err := loadConnection(list[0].ConnID)
	if err != nil || conn.UserID != run.UserID || !isSSHProtocol(conn) {
		return fmt.Errorf("connection not found")
	}
	units := it.Units
	if len(units) == 0 {
		return fmt.Errorf("no units to restart")
	}
	c.step("restart")
	cl, err := dialSSH(conn, nil)
	if err != nil {
		return fmt.Errorf("cannot log in to %s: %v", conn.Name, err)
	}
	defer cl.Close()
	health, err := restartUnits(cl, units)
	run.set(func() { it.Health = health })
	if err != nil {
		c.log("restart", "fail", err.Error())
		return err
	}
	c.log("restart", "ok", "restarted and healthy: "+unitNames(units))
	db.Exec(`DELETE FROM git_pending_restarts WHERE install_id=?`, it.InstallID)
	run.set(func() { it.State, it.Step = "ok", "" })
	return nil
}

// ─── audit and notifications ─────────────────────────

func auditGitItem(run *gitRun, it *gitRunItem) {
	run.mu.Lock()
	details := map[string]interface{}{"run_id": run.ID, "run": run.Label, "app": it.App, "result": it.State, "from": it.FromVersion, "to": it.ToVersion}
	if len(it.Written) > 0 {
		details["files"] = len(it.Written)
	}
	if len(it.Deleted) > 0 {
		details["deleted"] = len(it.Deleted)
	}
	if it.MadeBackup != "" {
		details["backup"] = it.MadeBackup
	}
	if it.Error != "" {
		details["error"] = truncateStr(it.Error, 500)
	}
	if len(it.Health) > 0 {
		details["units"] = unitNames(it.Units)
	}
	if run.P.Ref != "" {
		details["ref"] = run.P.Ref
	}
	if it.SourcePath != "" {
		details["source"] = it.SourceConnName + ":" + it.SourcePath
	}
	if it.Via == "ci" {
		details["via"], details["ci_kind"], details["ci_url"], details["ci_status"] = "ci", it.CIKind, it.CIURL, it.CIStatus
	}
	if e := run.P.Env; e != nil {
		details["env"], details["deploy_id"] = e.Env, e.DeployID
		if e.DryRun {
			details["dry_run"] = true
		}
		if it.Partial {
			details["partial"], details["moved"] = true, it.MovedCount
		}
		if it.PostDeploy != "" {
			details["post_deploy"] = it.PostDeploy
		}
		if len(it.Skipped) > 0 {
			details["skipped"] = len(it.Skipped)
		}
	}
	if run.Kind == "install" || run.Kind == "transfer" {
		details["to_conn_id"] = it.ConnID
		if it.InstallID != 0 {
			details["install_id"] = it.InstallID
		}
	}
	action := "git." + run.Kind
	if run.Kind != "restart" && len(it.Health) > 0 {
		auditLogRef(run.r, run.UserID, usernameOf(run.UserID), "git.restart", it.Path, map[string]interface{}{"run_id": run.ID, "units": unitNames(it.Units), "result": it.State}, auditRef{ConnID: it.ConnID})
	}
	run.mu.Unlock()
	auditLogRef(run.r, run.UserID, usernameOf(run.UserID), action, it.Path, details, auditRef{ConnID: it.ConnID})
}

var gitKindText = map[string]map[string]string{
	"en": {"update": "update", "upgrade": "upgrade", "rollback": "rollback", "stamp": "stamp", "restart": "restart", "install": "install", "transfer": "transfer"},
	"hr": {"update": "ažuriranje", "upgrade": "nadogradnja", "rollback": "vraćanje", "stamp": "označavanje", "restart": "ponovno pokretanje", "install": "instalacija", "transfer": "prijenos"},
}

func gitItemsSummary(items []*gitRunItem) string {
	var parts []string
	seen := map[string]bool{}
	for _, it := range items {
		s := it.App + " @ " + it.ConnName
		if !seen[s] {
			seen[s] = true
			parts = append(parts, s)
		}
	}
	if len(parts) > 5 {
		parts = append(parts[:5], fmt.Sprintf("+%d", len(parts)-5))
	}
	return strings.Join(parts, ", ")
}

func gitUnitsSummary(items []*gitRunItem) string {
	var parts []string
	for _, it := range items {
		if len(it.Units) > 0 {
			parts = append(parts, it.ConnName+": "+unitNames(it.Units))
		}
	}
	return strings.Join(parts, "; ")
}

func localTime(rfc string) string {
	if t, err := time.Parse(time.RFC3339, rfc); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return rfc
}

func notifyGitRun(run *gitRun, event string) {
	run.mu.Lock()
	vars := map[string]string{"label": run.Label, "summary": gitItemsSummary(run.P.Items), "message": run.Message, "units": gitUnitsSummary(run.P.Items)}
	var health []string
	ok := true
	for _, it := range run.P.Items {
		for _, h := range it.Health {
			health = append(health, fmt.Sprintf("%s %s: %s", it.ConnName, h.Name, h.Status))
		}
		if it.State != "ok" {
			ok = false
		}
	}
	vars["health"] = strings.Join(health, "; ")
	if vars["health"] == "" {
		vars["health"] = "-"
	}
	kind := run.Kind
	run.mu.Unlock()
	notifyUser(run.UserID, event, func(lang string) (string, string) {
		v := map[string]string{"kind": gitKindText[lang][kind]}
		if v["kind"] == "" {
			v["kind"] = gitKindText["en"][kind]
		}
		for k, x := range vars {
			v[k] = x
		}
		v["result"] = notifyText(lang, map[bool]string{true: "git.result.ok", false: "git.result.failed"}[ok], nil)
		return notifyText(lang, event, v), notifyText(lang, event+".body", v)
	})
}

func notifyRestartScheduled(userID, runID int, label string, items []*gitRunItem, sched string) {
	vars := map[string]string{"label": label, "summary": gitItemsSummary(items), "units": gitUnitsSummary(items), "time": localTime(sched)}
	notifyUser(userID, "git.restart_scheduled", func(lang string) (string, string) {
		return notifyText(lang, "git.restart_scheduled", vars), notifyText(lang, "git.restart_scheduled.body", vars)
	})
}

// ─── scheduler ───────────────────────────────────────

func runGitScheduler() {
	db.Exec(`UPDATE git_runs SET state='interrupted', message='WRM stopped during the run', ended_at=? WHERE state IN ('running','starting')`, nowStamp())
	for {
		time.Sleep(gitSchedulerPeriod)
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("git scheduler: %v", r)
				}
			}()
			gitSchedulerTick(time.Now())
			gitFeedsTick(time.Now())
		}()
	}
}

// gitSchedulerTick starts due runs, skips runs that are too late (WRM was down) and sends
// the notice before a scheduled run.
func gitSchedulerTick(now time.Time) {
	grace := time.Duration(settingInt("git_schedule_grace_minutes")) * time.Minute
	rows, err := db.Query(`SELECT id, user_id, kind, label, params, scheduled_at, notified_at FROM git_runs WHERE state='scheduled' AND scheduled_at<>'' ORDER BY scheduled_at, id`)
	if err != nil {
		return
	}
	type due struct {
		id, user                     int
		kind, label, params, s, note string
	}
	var list []due
	for rows.Next() {
		var d due
		rows.Scan(&d.id, &d.user, &d.kind, &d.label, &d.params, &d.s, &d.note)
		list = append(list, d)
	}
	rows.Close()
	for _, d := range list {
		at, err := time.Parse(time.RFC3339, d.s)
		if err != nil {
			continue
		}
		var p gitRunParams
		jsonUnmarshalString(d.params, &p)
		run := &gitRun{ID: d.id, UserID: d.user, Kind: d.kind, Label: d.label, P: p}
		switch {
		case now.After(at.Add(grace)):
			msg := fmt.Sprintf("skipped: WRM was not running at the scheduled time (%s, more than %d minutes ago)", localTime(d.s), int(grace/time.Minute))
			db.Exec(`UPDATE git_runs SET state='skipped', message=?, ended_at=? WHERE id=?`, msg, nowStamp(), d.id)
			db.Exec(`UPDATE git_runs SET state='skipped', message=?, ended_at=? WHERE parent_id=? AND state='scheduled'`, "skipped: the update was skipped", nowStamp(), d.id)
			run.State, run.Message = "skipped", msg
			for _, it := range run.P.Items {
				it.State = "skipped"
			}
			if d.kind == "restart" {
				notifyGitRun(run, "git.restart_done")
			} else {
				notifyGitRun(run, "git.deploy_failed")
			}
			auditLogRef(nil, d.user, usernameOf(d.user), "git.run_skipped", d.label, map[string]interface{}{"run_id": d.id, "kind": d.kind, "reason": "late"}, auditRef{})
		case !now.Before(at):
			key := gitKindPolicy[d.kind]
			if !gitActionAllowed(d.user, key) {
				db.Exec(`UPDATE git_runs SET state='skipped', message=?, ended_at=? WHERE id=?`, "skipped: the action is no longer allowed for this account", nowStamp(), d.id)
				continue
			}
			startGitRun(d.user, d.id, nil) // busy: try again on the next tick
		case at.Sub(now) <= gitUpcomingNotice && d.note == "":
			db.Exec(`UPDATE git_runs SET notified_at=? WHERE id=?`, nowStamp(), d.id)
			vars := map[string]string{"label": d.label, "summary": gitItemsSummary(p.Items), "time": localTime(d.s)}
			kind := d.kind
			notifyUser(d.user, "git.job_upcoming", func(lang string) (string, string) {
				v := map[string]string{"kind": gitKindText[lang][kind]}
				for k, x := range vars {
					v[k] = x
				}
				return notifyText(lang, "git.job_upcoming", v), notifyText(lang, "git.job_upcoming.body", v)
			})
		}
	}
}

// cancelGitRun cancels a scheduled run (and the restart waiting for it) or asks a running
// run to stop after the current installation.
func cancelGitRun(r *http.Request, userID, id int) error {
	var state, label string
	if db.QueryRow(`SELECT state, label FROM git_runs WHERE id=? AND user_id=?`, id, userID).Scan(&state, &label) != nil {
		return fmt.Errorf("run not found")
	}
	switch state {
	case "scheduled":
		db.Exec(`UPDATE git_runs SET state='cancelled', message='cancelled', ended_at=? WHERE id=?`, nowStamp(), id)
		db.Exec(`UPDATE git_runs SET state='cancelled', message='cancelled with the update', ended_at=? WHERE parent_id=? AND state='scheduled'`, nowStamp(), id)
	case "running", "starting":
		gitLive.Lock()
		run := gitLive.runs[id]
		gitLive.Unlock()
		if run == nil {
			return fmt.Errorf("the run is not working any more")
		}
		run.set(func() { run.cancel = true })
	default:
		return fmt.Errorf("only scheduled or running runs can be cancelled")
	}
	auditLog(r, userID, "git.run_cancelled", label, map[string]interface{}{"run_id": id, "state": state})
	return nil
}

// ─── API ─────────────────────────────────────────────

// apiGitDeploy handles the deploy routes of /api/git; it returns false for other routes.
//
//	POST /api/git/plan                {kind, ref, install_ids} → files and defaults per installation
//	GET  /api/git/installs/{id}/backups                      → backups on the server
//	GET  /api/git/runs                                       → history and scheduled runs
//	POST /api/git/runs                {kind, items, …}       → start or schedule a run
//	GET  /api/git/runs/{id}                                  → live progress / result
//	POST /api/git/runs/{id}/cancel
//	POST /api/git/provision/prepare  {kind, conn_id, services | source_install, path} → targets, templates, pre-checks
func apiGitDeploy(w http.ResponseWriter, r *http.Request, userID int, rest string, parts []string) bool {
	switch {
	case rest == "plan" && r.Method == http.MethodPost:
		var in struct {
			Kind       string `json:"kind"`
			Ref        string `json:"ref"`
			InstallIDs []int  `json:"install_ids"`
		}
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		if key, ok := gitKindPolicy[in.Kind]; !ok || !gitActionAllowed(userID, key) {
			jsonError(w, "This action is not allowed for your account", 403)
			return true
		}
		items, err := gitPlan(userID, in.Kind, in.Ref, in.InstallIDs)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return true
		}
		jsonOK(w, map[string]interface{}{"kind": in.Kind, "ref": in.Ref, "items": items})

	case len(parts) == 3 && parts[0] == "installs" && parts[2] == "backups" && r.Method == http.MethodGet:
		if !gitActionAllowed(userID, "git_rollback") {
			jsonError(w, "Rollbacks are not allowed for your account", 403)
			return true
		}
		id, _ := strconv.Atoi(parts[1])
		list := loadGitInstalls(userID, false, "i.id=?", id)
		if len(list) == 0 {
			jsonError(w, "Installation not found", 404)
			return true
		}
		conn, err := loadConnection(list[0].ConnID)
		if err != nil || conn.UserID != userID {
			jsonError(w, "Connection not found", 404)
			return true
		}
		cl, err := dialSSH(conn, nil)
		if err != nil {
			jsonError(w, fmt.Sprintf("cannot log in to %s: %v", conn.Name, err), 502)
			return true
		}
		defer cl.Close()
		backups, err := listBackups(cl, list[0].Path)
		if err != nil {
			jsonError(w, err.Error(), 502)
			return true
		}
		for i := range backups {
			files, from := rollbackFiles(userID, list[0], backups[i])
			backups[i].LogFiles = files
			if from == "backup" {
				backups[i].LogFiles = nil
			}
		}
		jsonOK(w, map[string]interface{}{"backups": backups, "dir": path.Join(list[0].Path, gitBackupDir)})

	case rest == "provision/prepare" && r.Method == http.MethodPost:
		var in gitPrepareReq
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		res, code, err := gitPrepare(userID, in)
		if err != nil {
			jsonError(w, err.Error(), code)
			return true
		}
		jsonOK(w, res)

	case rest == "runs" && r.Method == http.MethodGet:
		jsonOK(w, map[string]interface{}{"runs": loadGitRuns(userID, 100)})

	case rest == "runs" && r.Method == http.MethodPost:
		var p gitRunParams
		if !decodeGitJSON(w, r, &p) {
			return true
		}
		id, code, err := createGitRun(r, userID, p)
		if err != nil {
			jsonError(w, err.Error(), code)
			return true
		}
		v, _, _ := loadGitRunView(userID, id)
		jsonOK(w, v)

	case len(parts) == 2 && parts[0] == "runs" && r.Method == http.MethodGet:
		id, _ := strconv.Atoi(parts[1])
		v, _, ok := loadGitRunView(userID, id)
		if !ok {
			jsonError(w, "Run not found", 404)
			return true
		}
		jsonOK(w, v)

	case len(parts) == 3 && parts[0] == "runs" && parts[2] == "cancel" && r.Method == http.MethodPost:
		id, _ := strconv.Atoi(parts[1])
		if err := cancelGitRun(r, userID, id); err != nil {
			jsonError(w, err.Error(), 400)
			return true
		}
		v, _, _ := loadGitRunView(userID, id)
		jsonOK(w, v)

	default:
		return false
	}
	return true
}
