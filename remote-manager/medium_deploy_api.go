// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── MEDIUM: native deployment API (/api/git/medium/native/…) ───
//
// A project uses the engine an operator chose for it, else medium_default_engine: the
// native engine (WRM deploys and rolls it back itself) or the original tool on the Docker
// runtime. Installations that already ran the original tool keep it as their default. The state on the servers is the same either way, so switching back and forth
// needs no migration, and both tools show each other's deployments.

func initMediumDeploySchema() {
	ensureTable("medium_project_engine", `CREATE TABLE IF NOT EXISTS medium_project_engine (
		user_id INTEGER NOT NULL,
		project TEXT NOT NULL,
		engine TEXT NOT NULL DEFAULT 'docker',
		updated_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (user_id, project))`,
		"user_id", "project", "engine", "updated_at")
	mdBuildSchema()
}

func mdProjectEngine(uid int, project string) string {
	var e string
	if db.QueryRow(`SELECT engine FROM medium_project_engine WHERE user_id=? AND project=?`, uid, project).Scan(&e) == nil && (e == "native" || e == "docker") {
		return e
	}
	return mdDefaultEngine(uid)
}

// mdDefaultEngine is the engine of a project nobody chose one for: the native engine when
// medium_default_engine says so and the user may deploy with it, else the original tool.
func mdDefaultEngine(uid int) string {
	if getSetting("medium_default_engine") == "native" && mdNativeAllowed(uid) {
		return "native"
	}
	return "docker"
}

// mdProjectEngines lists the engines users chose per project; the rest use mdDefaultEngine.
func mdProjectEngines(uid int) map[string]string {
	out := map[string]string{}
	rows, err := db.Query(`SELECT project, engine FROM medium_project_engine WHERE user_id=?`, uid)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var p, e string
		if rows.Scan(&p, &e) == nil && (e == "native" || e == "docker") {
			out[p] = e
		}
	}
	return out
}

// mdMigrateDefaultEngine keeps installations that already use the original deploy tool on
// it after an upgrade: the native default applies to new installations, and the admin can
// switch it in Admin → Policies.
func mdMigrateDefaultEngine() {
	if env, _ := settingEnv("medium_default_engine"); env != "" {
		return
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE key='medium_default_engine'`).Scan(&n)
	if n > 0 {
		return
	}
	if _, err := os.Stat(mediumToolDir()); err != nil {
		return
	}
	if _, err := setSetting("medium_default_engine", "original"); err == nil {
		log.Printf("Medium: the original deploy tool is installed, so projects keep using it by default (medium_default_engine=original); switch to native in Admin → Policies")
	}
}

// mdNativeAllowed: may the user change servers with the native engine?
func mdNativeAllowed(uid int) bool { return gitActionAllowed(uid, "medium_native") }

// mdNativeAPI answers the deployment routes; false when action is not one of them.
func mdNativeAPI(w http.ResponseWriter, r *http.Request, uid int, action string) bool {
	switch {
	case action == "/engine" && r.Method == "POST":
		mdAPIEngine(w, r, uid)
	case action == "/deploy" && r.Method == "POST":
		mdAPIDeploy(w, r, uid)
	case action == "/rollback" && r.Method == "POST":
		mdAPIRollback(w, r, uid)
	case action == "/history" && r.Method == "GET":
		mdAPIHistory(w, r, uid)
	case action == "/lock/release" && r.Method == "POST":
		mdAPILockRelease(w, r, uid)
	case action == "/jobs" && r.Method == "GET":
		mdAPIJobs(w, uid)
	case strings.HasPrefix(action, "/jobs/") && strings.HasSuffix(action, "/cancel") && r.Method == "POST":
		j := mdGetJob(uid, strings.TrimSuffix(strings.TrimPrefix(action, "/jobs/"), "/cancel"))
		if j == nil {
			jsonError(w, "not found", 404)
			return true
		}
		j.mu.Lock()
		busy, done := j.changing, j.Done
		if !done {
			j.cancelled = "cancelled by " + usernameOf(uid)
		}
		j.mu.Unlock()
		if !done {
			j.log("warn", "Cancel requested by %s.", usernameOf(uid))
		}
		auditLog(r, uid, "git.medium_native_cancelled", j.Project, map[string]interface{}{"job": j.ID, "deploy_id": j.DeployID})
		jsonOK(w, map[string]interface{}{"ok": true, "finishing_destination": busy})
	case strings.HasPrefix(action, "/jobs/") && r.Method == "GET":
		j := mdGetJob(uid, strings.TrimPrefix(action, "/jobs/"))
		if j == nil {
			jsonError(w, "not found", 404)
			return true
		}
		from, _ := strconv.Atoi(r.URL.Query().Get("from"))
		jsonOK(w, j.view(from))
	default:
		return false
	}
	return true
}

func mdAPIEngine(w http.ResponseWriter, r *http.Request, uid int) {
	var in struct {
		Project string `json:"project"`
		Engine  string `json:"engine"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in) != nil || !mdNameRe.MatchString(in.Project) || in.Engine != "native" && in.Engine != "docker" {
		jsonError(w, "invalid request", 400)
		return
	}
	if in.Engine == "native" && !mdNativeAllowed(uid) {
		jsonError(w, "Native deployments are not allowed for you by policy (medium_native)", 403)
		return
	}
	cfg, err := mdLoadConfig(mdConfigDir(uid))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if cfg.project(in.Project) == nil {
		jsonError(w, "unknown project", 404)
		return
	}
	db.Exec(`INSERT OR REPLACE INTO medium_project_engine (user_id, project, engine, updated_at) VALUES (?,?,?,?)`, uid, in.Project, in.Engine, nowStamp())
	auditLog(r, uid, "git.medium_engine_set", in.Project, map[string]interface{}{"engine": in.Engine})
	jsonOK(w, map[string]interface{}{"ok": true, "engine": in.Engine})
}

// mdDeployTargets checks a reviewed plan against the current configuration and returns
// its destinations in config order.
func mdDeployTargets(cfg *mdConfig, res *mdPlanResult) (*mdProject, *mdTarget, []mdDeployTarget, error) {
	proj := cfg.project(res.Project)
	if proj == nil {
		return nil, nil, nil, fmt.Errorf("project %s is no longer in deploy-config; compare again", res.Project)
	}
	target := proj.target(res.Environment)
	if target == nil {
		return nil, nil, nil, fmt.Errorf("project %s no longer has the environment %s; compare again", res.Project, res.Environment)
	}
	var out []mdDeployTarget
	for _, d := range target.Destinations {
		for i := range res.Servers {
			sp := &res.Servers[i]
			if sp.Server != d.Server {
				continue
			}
			same := sp.Path == d.appDir() && strings.Join(sp.Excludes, "\x00") == strings.Join(proj.ignoreFor(target.Name, d.Server), "\x00") &&
				(sp.PostDeploy == nil) == (d.PostDeploy == nil) && (sp.PostDeploy == nil || *sp.PostDeploy == *d.PostDeploy)
			if !same {
				return nil, nil, nil, fmt.Errorf("deploy-config changed for %s since the comparison (path, ignore or post_deploy); compare again", d.label())
			}
			out = append(out, mdDeployTarget{d: d, plan: sp, postDeploy: d.PostDeploy})
		}
	}
	if len(out) != len(res.Servers) {
		return nil, nil, nil, fmt.Errorf("a destination of the plan is no longer in deploy-config; compare again")
	}
	return proj, target, out, nil
}

func mdAPIDeploy(w http.ResponseWriter, r *http.Request, uid int) {
	var in struct {
		RunID    string `json:"run_id"`
		Revision string `json:"revision"`
		Confirm  string `json:"confirm"`
		DryRun   bool   `json:"dry_run"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&in) != nil || in.RunID == "" {
		jsonError(w, "invalid request", 400)
		return
	}
	mdRuns.Lock()
	run := mdRuns.m[in.RunID]
	mdRuns.Unlock()
	if run == nil || run.uid != uid {
		jsonError(w, "the comparison is no longer available; compare again", 404)
		return
	}
	mdRuns.Lock()
	res, src, done, used := run.Result, run.src, run.Done, run.used
	mdRuns.Unlock()
	switch {
	case !done || res == nil:
		jsonError(w, "the comparison has not finished", 409)
		return
	case !res.Deployable || src == nil:
		jsonError(w, "this comparison cannot be deployed natively (it needs a WRM Git source and every destination compared without errors)", 400)
		return
	case in.Revision != res.Revision:
		jsonError(w, "the plan changed since you reviewed it; compare again", 400)
		return
	case used:
		// A real deployment consumes the plan and removes a build's temporary
		// files. Do not run a dry run on that plan afterwards.
		jsonError(w, "this plan was already deployed; compare again", 409)
		return
	}
	if !in.DryRun {
		switch {
		case !mdNativeAllowed(uid):
			jsonError(w, "Native deployments are not allowed for you by policy (medium_native)", 403)
			return
		case mdProjectEngine(uid, res.Project) != "native":
			jsonError(w, "switch "+res.Project+" to the native engine first (Engine: native); it is deployed by the original tool now", 400)
			return
		case !res.RefAllowed:
			jsonError(w, res.Ref+" is not allowed for "+res.Environment+" (allowed_refs)", 400)
			return
		case res.Confirm && in.Confirm != res.Environment:
			jsonError(w, "type the environment name ("+res.Environment+") to confirm", 400)
			return
		}
		if !requireStepUp(w, r, "medium.deploy") {
			return
		}
	}
	cfg, err := mdLoadConfig(mdConfigDir(uid))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	proj, target, targets, err := mdDeployTargets(cfg, res)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	now := time.Now()
	id, err := mdMakeDeployID(now, res.Commit)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	branch, kind := mdRefNames(res.Ref, res.Commit)
	version := res.Commit[:7]
	if kind != nil && *kind == "tag" {
		version = branch
	}
	entry := mdHistoryEntry{Action: "DEPLOY"}
	entry.mdRecord = mdRecord{Project: res.Project, Environment: res.Environment, Version: version, Commit: res.Commit, Branch: branch, RefKind: kind,
		DeployID: id, DeployedAt: now.UTC().Format(mdTimeFormat), DeployedBy: usernameOf(uid), Excluded: []string{}}
	excluded, _ := mdCleanExcluded(res.Excluded)
	j := &mdJob{}
	j.Kind, j.DryRun, j.Project, j.Environment, j.Ref, j.Commit, j.Version, j.DeployID, j.RunID, j.Revision, j.Confirm =
		"deploy", in.DryRun, res.Project, res.Environment, res.Ref, res.Commit, version, id, run.ID, res.Revision, res.Confirm
	for _, t := range targets {
		j.Servers = append(j.Servers, t.d.Server)
	}
	if !in.DryRun {
		mdRuns.Lock()
		if run.used {
			mdRuns.Unlock()
			jsonError(w, "this plan was already deployed; compare again", 409)
			return
		}
		run.used = true
		mdRuns.Unlock()
	}
	mdRuns.Lock()
	run.deploying = true
	mdRuns.Unlock()
	dc := &mdDeployContext{uid: uid, cfg: cfg, proj: proj, target: target, src: src, excluded: excluded, entry: entry, dryRun: in.DryRun, job: j}
	if err := mdStartJob(uid, j, func(j *mdJob) {
		defer func() {
			mdRuns.Lock()
			run.deploying = false
			var cleanup func()
			if !in.DryRun && run.cleanup != nil {
				cleanup, run.cleanup = run.cleanup, nil
			}
			mdRuns.Unlock()
			if cleanup != nil {
				cleanup()
			}
		}()
		j.log("info", "%s of %s / %s @ %s (%s), deployment %s", map[bool]string{true: "Dry run", false: "Deployment"}[in.DryRun], res.Project, res.Environment, res.Ref, res.Commit[:10], id)
		if len(excluded) > 0 {
			j.log("info", "Excluded from this deployment: %s", mdShown(excluded, 20))
		}
		mdDeployDestinations(dc, targets)
	}); err != nil {
		mdRuns.Lock()
		run.deploying = false
		if !in.DryRun {
			run.used = false
		}
		mdRuns.Unlock()
		jsonError(w, err.Error(), 409)
		return
	}
	auditLog(r, uid, "git.medium_native_deploy", res.Project, map[string]interface{}{"environment": res.Environment, "ref": res.Ref, "commit": res.Commit,
		"servers": j.Servers, "dry_run": in.DryRun, "revision": res.Revision, "deploy_id": id, "excluded": len(excluded)})
	jsonOK(w, map[string]string{"job_id": j.ID, "deploy_id": id})
}

// ─── history and rollback ───

type mdHistoryView struct {
	Server  string           `json:"server"`
	Label   string           `json:"label"`
	Error   string           `json:"error,omitempty"`
	Current *mdRecord        `json:"current"`
	Lock    *mdLockOwner     `json:"lock"`
	Entries []mdHistoryEntry `json:"entries"`
	Choice  *mdRollbackView  `json:"rollback"`
}

type mdRollbackView struct {
	DeployID string `json:"deploy_id,omitempty"`
	Version  string `json:"version,omitempty"`
	By       string `json:"by,omitempty"`
	At       string `json:"at,omitempty"`
	After    string `json:"after,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func mdHistoryOf(h mdHost, d *mdDestination) mdHistoryView {
	v := mdHistoryView{Server: d.Server, Label: d.label(), Entries: []mdHistoryEntry{}}
	cur, err := mdReadCurrent(h, d)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	v.Current = cur
	if v.Lock, err = mdReadLock(h, d); err != nil {
		v.Error = err.Error()
	}
	entries, err := mdReadHistory(h, d, mdHistoryWindow)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	c := mdChooseRollback(entries)
	rv := &mdRollbackView{Reason: c.Reason}
	if c.Deploy != nil && !mdDeployIDRe.MatchString(c.Deploy.DeployID) {
		rv.Reason = "the last deployment in history.jsonl has an invalid deploy_id; it cannot be rolled back"
	} else if c.Deploy != nil {
		rv.DeployID, rv.Version, rv.By, rv.At, rv.After = c.Deploy.DeployID, c.Deploy.Version, c.Deploy.DeployedBy, c.Deploy.DeployedAt, c.After()
	}
	v.Choice = rv
	for i, e := range entries {
		if i >= 30 {
			break
		}
		e.New, e.Changed, e.Deleted = nil, nil, nil // counts say how many; the lists stay on the server
		v.Entries = append(v.Entries, e)
	}
	return v
}

func mdAPIHistory(w http.ResponseWriter, r *http.Request, uid int) {
	cfg, err := mdLoadConfig(mdConfigDir(uid))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	proj := cfg.project(r.URL.Query().Get("project"))
	if proj == nil {
		jsonError(w, "unknown project", 404)
		return
	}
	target := proj.target(r.URL.Query().Get("environment"))
	if target == nil {
		jsonError(w, "unknown environment", 404)
		return
	}
	out := make([]mdHistoryView, len(target.Destinations))
	var wg sync.WaitGroup
	for i, d := range target.Destinations {
		wg.Add(1)
		go func(i int, d *mdDestination) {
			defer wg.Done()
			err := mdOpenHost(uid, cfg, d.Server, func(h mdHost, _ string) error {
				out[i] = mdHistoryOf(h, d)
				return nil
			})
			if err != nil {
				out[i] = mdHistoryView{Server: d.Server, Label: d.label(), Error: err.Error(), Entries: []mdHistoryEntry{}}
			}
		}(i, d)
	}
	wg.Wait()
	jsonOK(w, map[string]interface{}{"project": proj.Name, "environment": target.Name, "confirm": target.Confirm, "engine": mdProjectEngine(uid, proj.Name),
		"can_deploy": mdNativeAllowed(uid), "stale_lock_minutes": mdStaleLockMinutes(), "servers": out})
}

func mdAPIRollback(w http.ResponseWriter, r *http.Request, uid int) {
	var in struct {
		Project     string   `json:"project"`
		Environment string   `json:"environment"`
		DeployID    string   `json:"deploy_id"`
		Servers     []string `json:"servers"`
		Confirm     string   `json:"confirm"`
		DryRun      bool     `json:"dry_run"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&in) != nil || !mdDeployIDRe.MatchString(in.DeployID) || len(in.Servers) == 0 {
		jsonError(w, "choose the deployment and the servers to roll back", 400)
		return
	}
	cfg, err := mdLoadConfig(mdConfigDir(uid))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	proj := cfg.project(in.Project)
	var target *mdTarget
	if proj != nil {
		target = proj.target(in.Environment)
	}
	if target == nil {
		jsonError(w, "unknown project or environment", 404)
		return
	}
	if !in.DryRun {
		switch {
		case !mdNativeAllowed(uid):
			jsonError(w, "Native rollbacks are not allowed for you by policy (medium_native)", 403)
			return
		case mdProjectEngine(uid, proj.Name) != "native":
			jsonError(w, "switch "+proj.Name+" to the native engine first (Engine: native)", 400)
			return
		case target.Confirm && in.Confirm != target.Name:
			jsonError(w, "type the environment name ("+target.Name+") to confirm", 400)
			return
		}
		if !requireStepUp(w, r, "medium.rollback") {
			return
		}
	}
	want := map[string]bool{}
	for _, s := range in.Servers {
		if target.destination(s) == nil {
			jsonError(w, "environment "+target.Name+" has no destination on "+s, 400)
			return
		}
		want[s] = true
	}
	var targets []mdRollbackTarget
	for _, d := range target.Destinations {
		if want[d.Server] {
			targets = append(targets, mdRollbackTarget{d: d, postDeploy: d.PostDeploy})
		}
	}
	commit := "0000000"
	if i := strings.LastIndex(in.DeployID, "-"); i > 0 {
		commit = in.DeployID[i+1:]
	}
	rid, _ := mdMakeDeployID(time.Now(), commit)
	entry := mdHistoryEntry{Action: "ROLLBACK"}
	entry.mdRecord = mdRecord{Project: proj.Name, Environment: target.Name, DeployID: rid, DeployedBy: usernameOf(uid)}
	j := &mdJob{}
	j.Kind, j.DryRun, j.Project, j.Environment, j.DeployID, j.RolledBack, j.Confirm = "rollback", in.DryRun, proj.Name, target.Name, rid, in.DeployID, target.Confirm
	for _, t := range targets {
		j.Servers = append(j.Servers, t.d.Server)
	}
	dc := &mdDeployContext{uid: uid, cfg: cfg, proj: proj, target: target, entry: entry, dryRun: in.DryRun, job: j}
	if err := mdStartJob(uid, j, func(j *mdJob) {
		j.log("info", "%s of %s on %s / %s", map[bool]string{true: "Dry rollback", false: "Rollback"}[in.DryRun], in.DeployID, proj.Name, target.Name)
		// the choice is read per destination, and must still be the deployment chosen
		for i := range targets {
			t := &targets[i]
			err := mdOpenHost(uid, cfg, t.d.Server, func(h mdHost, _ string) error {
				entries, err := mdReadHistory(h, t.d, mdHistoryWindow)
				if err != nil {
					return err
				}
				t.choice = mdChooseRollback(entries)
				return nil
			})
			if err != nil {
				t.choice = mdRollbackChoice{Reason: err.Error()}
			} else if t.choice.Deploy != nil && t.choice.Deploy.DeployID != in.DeployID {
				t.choice = mdRollbackChoice{Reason: fmt.Sprintf("the last deployment on %s is %s, not %s; refresh History", t.d.label(), t.choice.Deploy.DeployID, in.DeployID)}
			}
		}
		mdRollbackDestinations(dc, targets)
	}); err != nil {
		jsonError(w, err.Error(), 409)
		return
	}
	auditLog(r, uid, "git.medium_native_rollback", proj.Name, map[string]interface{}{"environment": target.Name, "deploy_id": in.DeployID, "rollback_id": rid, "servers": in.Servers, "dry_run": in.DryRun})
	jsonOK(w, map[string]string{"job_id": j.ID, "rollback_id": rid})
}

func mdAPILockRelease(w http.ResponseWriter, r *http.Request, uid int) {
	var in struct {
		Project     string `json:"project"`
		Environment string `json:"environment"`
		Server      string `json:"server"`
		DeployID    string `json:"deploy_id"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in) != nil || in.DeployID != "" && !mdDeployIDRe.MatchString(in.DeployID) {
		jsonError(w, "invalid request", 400)
		return
	}
	if !mdNativeAllowed(uid) {
		jsonError(w, "Releasing locks is not allowed for you by policy (medium_native)", 403)
		return
	}
	cfg, err := mdLoadConfig(mdConfigDir(uid))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	var d *mdDestination
	if proj := cfg.project(in.Project); proj != nil {
		if t := proj.target(in.Environment); t != nil {
			d = t.destination(in.Server)
		}
	}
	if d == nil {
		jsonError(w, "unknown destination", 404)
		return
	}
	if !requireStepUp(w, r, "medium.lock_release") {
		return
	}
	var owner *mdLockOwner
	err = mdOpenHost(uid, cfg, d.Server, func(h mdHost, _ string) error {
		owner, _ = mdReadLock(h, d)
		return mdReleaseStaleLock(h, d, in.DeployID)
	})
	status, msg := "ok", ""
	if err != nil {
		status, msg = "error", err.Error()
	}
	mediumRecord(uid, "native_lock_release", status, 0, in.Project+" / "+in.Environment+" on "+d.label(), msg, "")
	if err != nil {
		jsonError(w, err.Error(), 409)
		return
	}
	auditLog(r, uid, "git.medium_native_lock_released", d.label(), map[string]interface{}{"project": in.Project, "environment": in.Environment, "owner": owner})
	jsonOK(w, map[string]bool{"ok": true})
}

func mdAPIJobs(w http.ResponseWriter, uid int) {
	mdJobs.Lock()
	var list []*mdJob
	for _, j := range mdJobs.m {
		if j.uid == uid {
			list = append(list, j)
		}
	}
	mdJobs.Unlock()
	out := []mdJobView{}
	for _, j := range list {
		v := j.view(1 << 30)
		out = append(out, v)
	}
	jsonOK(w, map[string]interface{}{"jobs": out})
}
