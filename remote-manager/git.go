// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ─── GIT WORKSPACE: storage, settings and API ────────
//
// Everything is per user: Git sources (API tokens encrypted at rest, bundles), the catalog
// (catalog JSON), the computed targets, the installations found on the user's servers and
// the workspace settings. Policies: git_enabled hides the workspace, git_checks says who may
// run checks (discovery / comparison over SSH).

func initGitSchema() {
	// columns added after a table was first released are added in place before ensureTable
	// checks the table (it would otherwise set the table aside and start an empty one)
	for _, m := range []string{
		// v11.3.0
		`ALTER TABLE git_sources ADD COLUMN write_token TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE git_sources ADD COLUMN hook_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE git_sources ADD COLUMN hook_secret TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE git_sources ADD COLUMN hook_at TEXT NOT NULL DEFAULT ''`,
		// v11.4.0: the last good result of a check
		`ALTER TABLE git_installs ADD COLUMN last_ok_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE git_installs ADD COLUMN last_ok_state TEXT NOT NULL DEFAULT ''`,
	} {
		db.Exec(m) // fails harmlessly when the table does not exist yet or has the column
	}
	ensureTable("git_sources", `CREATE TABLE IF NOT EXISTS git_sources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		kind TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		url TEXT NOT NULL DEFAULT '',
		token TEXT NOT NULL DEFAULT '',
		info TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '',
		write_token TEXT NOT NULL DEFAULT '',
		hook_id TEXT NOT NULL DEFAULT '',
		hook_secret TEXT NOT NULL DEFAULT '',
		hook_at TEXT NOT NULL DEFAULT '')`,
		"id", "user_id", "kind", "name", "url", "token", "info", "created_at", "last_error", "write_token", "hook_id", "hook_secret", "hook_at")
	restoreSetAsideGitSources()
	ensureTable("git_blobs", `CREATE TABLE IF NOT EXISTS git_blobs (
		source_id INTEGER NOT NULL,
		blob TEXT NOT NULL,
		hash TEXT NOT NULL DEFAULT '',
		hash2 TEXT NOT NULL DEFAULT '',
		size INTEGER NOT NULL DEFAULT 0,
		content BLOB,
		fetched_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (source_id, blob))`,
		"source_id", "blob", "hash", "hash2", "size", "content", "fetched_at")
	ensureTable("git_trees", `CREATE TABLE IF NOT EXISTS git_trees (
		source_id INTEGER NOT NULL,
		project TEXT NOT NULL,
		commit_id TEXT NOT NULL,
		data TEXT NOT NULL DEFAULT '',
		fetched_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (source_id, project, commit_id))`,
		"source_id", "project", "commit_id", "data", "fetched_at")
	ensureTable("git_workspace", `CREATE TABLE IF NOT EXISTS git_workspace (
		user_id INTEGER PRIMARY KEY,
		catalog TEXT NOT NULL DEFAULT '',
		settings TEXT NOT NULL DEFAULT '',
		last_check_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`,
		"user_id", "catalog", "settings", "last_check_at", "updated_at")
	ensureTable("git_targets", `CREATE TABLE IF NOT EXISTS git_targets (
		user_id INTEGER NOT NULL,
		app TEXT NOT NULL,
		data TEXT NOT NULL DEFAULT '',
		computed_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (user_id, app))`,
		"user_id", "app", "data", "computed_at")
	ensureTable("git_installs", `CREATE TABLE IF NOT EXISTS git_installs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		conn_id INTEGER NOT NULL,
		app TEXT NOT NULL DEFAULT '',
		path TEXT NOT NULL DEFAULT '',
		env TEXT NOT NULL DEFAULT '',
		manual INTEGER NOT NULL DEFAULT 0,
		state TEXT NOT NULL DEFAULT '',
		data TEXT NOT NULL DEFAULT '',
		checked_at TEXT NOT NULL DEFAULT '',
		discovered_at TEXT NOT NULL DEFAULT '',
		last_ok_at TEXT NOT NULL DEFAULT '',
		last_ok_state TEXT NOT NULL DEFAULT '')`,
		"id", "user_id", "conn_id", "app", "path", "env", "manual", "state", "data", "checked_at", "discovered_at", "last_ok_at", "last_ok_state")
	mustExec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_git_installs_path ON git_installs(user_id, conn_id, path)`)
	initGitRelay()
	initGitReview()
	initGitFunctionalitySchema()
	initGitRunsSchema()
	initMediumNativeSchema()
	initMediumDeploySchema()
	initMediumActivitySchema()
	mediumActivityRunningFix()
	initGitCISchema()
	initGitEnvSchema()
}

// restoreSetAsideGitSources brings back the Git sources of a table v11.3.0 set aside
// (git_sources_old_<time>) when it upgraded an earlier database, while the new table is still
// empty. The old table is renamed afterwards so this happens once.
func restoreSetAsideGitSources() {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM git_sources`).Scan(&n)
	var old string
	if n > 0 || db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'git_sources_old_%' ORDER BY name DESC LIMIT 1`).Scan(&old) != nil {
		return
	}
	q := `"` + strings.ReplaceAll(old, `"`, `""`) + `"`
	res, err := db.Exec(`INSERT INTO git_sources (id, user_id, kind, name, url, token, info, created_at, last_error)
		SELECT id, user_id, kind, name, url, token, info, created_at, last_error FROM ` + q)
	if err != nil {
		log.Printf("Git sources in %s could not be restored: %v", old, err)
		return
	}
	k, _ := res.RowsAffected()
	db.Exec(`ALTER TABLE ` + q + ` RENAME TO "` + strings.Replace(old, "_old_", "_restored_", 1) + `"`)
	log.Printf("Restored %d Git source(s) from %s", k, old)
}

func deleteGitUserData(userID int) {
	for _, s := range loadGitSources(userID) {
		if s.Owned {
			deleteGitSource(s.ID)
		}
	}
	db.Exec(`DELETE FROM git_server_health WHERE user_id=?`, userID)
	db.Exec(`DELETE FROM git_workspace WHERE user_id=?`, userID)
	db.Exec(`DELETE FROM git_targets WHERE user_id=?`, userID)
	db.Exec(`DELETE FROM git_installs WHERE user_id=?`, userID)
	db.Exec(`DELETE FROM git_runs WHERE user_id=?`, userID)
	db.Exec(`DELETE FROM git_pending_restarts WHERE user_id=?`, userID)
	db.Exec(`DELETE FROM git_feeds WHERE user_id=?`, userID)
	db.Exec(`DELETE FROM git_pipelines WHERE user_id=?`, userID)
	db.Exec(`DELETE FROM git_env_exclusions WHERE user_id=?`, userID)
	db.Exec(`DELETE FROM medium_build_settings WHERE user_id=?`, userID)
}

func gitAllowed() bool { return settingBool("git_enabled") }

func gitChecksAllowed(userID int) bool {
	return gitAllowed() && policyWhoAllows(userID, "git_checks")
}

func jsonUnmarshalBytes(b []byte, v interface{}) { json.Unmarshal(b, v) }

// ─── settings and catalog ────────────────────────────

type gitSettings struct {
	SourceID     int               `json:"source_id"` // API source used for live targets (0 = the first one)
	Mode         string            `json:"mode"`      // auto | live | bundle
	BundleID     int               `json:"bundle_id"` // bundle source (0 = the newest with the service)
	RefMode      string            `json:"ref_mode"`  // catalog | default | branch
	RefBranch    string            `json:"ref_branch"`
	RefOverrides map[string]string `json:"ref_overrides"`
	HistoryDepth int               `json:"history_depth"`
	ConnIDs      []int             `json:"conn_ids"` // servers to check
	Monitor      bool              `json:"monitor"`  // periodic checks
	// custom check command per service, remembered from the last run
	CheckCommands map[string]string `json:"check_commands"`
}

func (s *gitSettings) normalize(userID int) error {
	switch s.Mode {
	case "", "auto":
		s.Mode = "auto"
	case "live", "bundle":
	default:
		return fmt.Errorf("unknown source mode")
	}
	switch s.RefMode {
	case "":
		s.RefMode = "catalog"
	case "catalog", "default":
	case "branch":
		if strings.TrimSpace(s.RefBranch) == "" {
			return fmt.Errorf("enter the branch for all services")
		}
	default:
		return fmt.Errorf("unknown ref choice")
	}
	s.RefBranch = strings.TrimSpace(s.RefBranch)
	if s.HistoryDepth <= 0 {
		s.HistoryDepth = gitDefaultDepth
	}
	if s.HistoryDepth > gitMaxDepth {
		s.HistoryDepth = gitMaxDepth
	}
	ov := map[string]string{}
	for k, v := range s.RefOverrides {
		if v = strings.TrimSpace(v); v != "" && len(ov) < 500 {
			ov[k] = v
		}
	}
	s.RefOverrides = ov
	cc := map[string]string{}
	for k, v := range s.CheckCommands {
		if v = strings.TrimSpace(v); v != "" && len(v) <= 500 && len(cc) < 500 {
			cc[k] = v
		}
	}
	s.CheckCommands = cc
	ids := []int{}
	seen := map[int]bool{}
	for _, id := range s.ConnIDs {
		if !seen[id] && userOwnsConnection(id, userID) {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	s.ConnIDs = ids
	return nil
}

func loadGitWorkspace(userID int) (*gitCatalog, gitSettings) {
	var c, s string
	db.QueryRow(`SELECT catalog, settings FROM git_workspace WHERE user_id=?`, userID).Scan(&c, &s)
	cat := &gitCatalog{}
	jsonUnmarshalString(c, cat)
	if c == "" {
		cat.ServerRoots = []string{"/opt", "/srv"}
		cat.FallbackBranches = []string{"main", "master"}
	}
	cat.normalize()
	var st gitSettings
	jsonUnmarshalString(s, &st)
	st.normalize(userID)
	return cat, st
}

func loadGitCatalog(userID int) *gitCatalog {
	c, _ := loadGitWorkspace(userID)
	return c
}

func saveGitWorkspace(userID int, cat *gitCatalog, st *gitSettings) {
	db.Exec(`INSERT OR IGNORE INTO git_workspace (user_id) VALUES (?)`, userID)
	if cat != nil {
		db.Exec(`UPDATE git_workspace SET catalog=?, updated_at=? WHERE user_id=?`, string(jsonMarshal(cat)), nowStamp(), userID)
	}
	if st != nil {
		db.Exec(`UPDATE git_workspace SET settings=?, updated_at=? WHERE user_id=?`, string(jsonMarshal(st)), nowStamp(), userID)
	}
}

// effectiveApps is the catalog plus the services of imported bundles that the catalog does
// not have (so a bundle alone is enough to find and compare installations).
func effectiveApps(userID int, cat *gitCatalog) []gitCatalogApp {
	apps := append([]gitCatalogApp(nil), cat.Apps...)
	have := map[string]bool{}
	for _, a := range apps {
		have[a.Name] = true
	}
	for _, s := range loadGitSources(userID) {
		if s.Kind != "bundle" {
			continue
		}
		m, err := loadBundleManifest(userID, s.ID)
		if err != nil {
			continue
		}
		for _, name := range s.Apps {
			if have[name] {
				continue
			}
			b := m.Apps[name]
			a := gitCatalogApp{Name: name, Project: b.Project, Ref: "tag:latest", Branch: b.Branch, Subdir: b.Subdir, Kind: b.Kind,
				Protected: b.Protected, Fingerprint: b.Fingerprint, InstallHint: b.InstallHint, Exclude: b.Exclude}
			if b.RefKind != "tag" {
				a.Ref = b.Branch
			}
			if a.normalize() == nil {
				have[name] = true
				apps = append(apps, a)
			}
		}
	}
	return apps
}

// ─── targets ─────────────────────────────────────────

type gitTargetBrief struct {
	App        string   `json:"app"`
	Source     string   `json:"source"`
	SourceID   int      `json:"source_id"`
	Project    string   `json:"project"`
	Ref        string   `json:"ref"`
	Branch     string   `json:"branch"`
	RefKind    string   `json:"ref_kind"`
	Version    string   `json:"version"`
	LatestTag  string   `json:"latest_tag"`
	Commit     string   `json:"commit"`
	CommitDate string   `json:"commit_date"`
	Files      int      `json:"files"`
	Warnings   []string `json:"warnings"`
	Error      string   `json:"error,omitempty"`
	ComputedAt string   `json:"computed_at"`
	APICalls   int      `json:"api_calls,omitempty"`
}

func (t gitTarget) brief() gitTargetBrief {
	return gitTargetBrief{App: t.App, Source: t.Source, SourceID: t.SourceID, Project: t.Project, Ref: t.Ref, Branch: t.Branch, RefKind: t.RefKind,
		Version: t.Version, LatestTag: t.LatestTag, Commit: t.Commit, CommitDate: t.CommitDate, Files: len(t.Files), Warnings: nonNil(t.Warnings),
		Error: t.Error, ComputedAt: t.ComputedAt, APICalls: t.APICalls}
}

func loadGitTarget(userID int, app string) (gitTarget, bool) {
	var data string
	if db.QueryRow(`SELECT data FROM git_targets WHERE user_id=? AND app=?`, userID, app).Scan(&data) != nil {
		return gitTarget{}, false
	}
	var t gitTarget
	if json.Unmarshal([]byte(data), &t) != nil {
		return gitTarget{}, false
	}
	return t, true
}

func loadGitTargets(userID int) []gitTargetBrief {
	out := []gitTargetBrief{}
	rows, err := db.Query(`SELECT data FROM git_targets WHERE user_id=? ORDER BY app`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var data string
		rows.Scan(&data)
		var t gitTarget
		if json.Unmarshal([]byte(data), &t) == nil {
			out = append(out, t.brief())
		}
	}
	return out
}

// pickSources returns the API source and the bundles used for targets.
func pickSources(userID int, st gitSettings) (*gitSource, []gitSource) {
	var api *gitSource
	var bundles []gitSource
	for _, s := range loadGitSources(userID) {
		s := s
		if s.Kind == "bundle" {
			bundles = append(bundles, s)
		} else if api == nil || s.ID == st.SourceID {
			api = &s
		}
	}
	// newest bundle first
	sort.SliceStable(bundles, func(i, j int) bool {
		if bundles[i].Created != bundles[j].Created {
			return bundles[i].Created > bundles[j].Created
		}
		return bundles[i].ID > bundles[j].ID
	})
	return api, bundles
}

type gitTargetChange struct {
	App, Old, New string
}

// refreshTargets computes the targets of the services (all when apps is empty) and stores
// them. It returns the services whose target version changed.
func refreshTargets(userID int, apps []string) ([]gitTarget, []gitTargetChange) {
	cat, st := loadGitWorkspace(userID)
	api, bundles := pickSources(userID, st)
	want := map[string]bool{}
	for _, a := range apps {
		want[a] = true
	}
	all := effectiveApps(userID, cat)
	useBundle := st.Mode == "bundle" || (st.Mode == "auto" && api == nil)
	var p *gitProvider
	var perr error
	if !useBundle {
		if api == nil {
			perr = fmt.Errorf("no Git source configured (Settings)")
		} else {
			p, perr = newGitProvider(*api)
		}
	}
	out := make([]gitTarget, 0, len(all))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 2)
	for _, a := range all {
		if len(want) > 0 && !want[a.Name] {
			continue
		}
		wg.Add(1)
		go func(a gitCatalogApp) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var t gitTarget
			switch {
			case useBundle:
				t = gitTarget{App: a.Name, Error: "no imported bundle contains this service", Files: map[string]gitTargetFile{}, ComputedAt: nowStamp()}
				for _, b := range bundles {
					if st.BundleID != 0 && b.ID != st.BundleID {
						continue
					}
					m, err := loadBundleManifest(userID, b.ID)
					if err != nil {
						continue
					}
					if bt, ok := bundleTarget(b, m, a.Name); ok {
						t = bt
						break
					}
				}
			case perr != nil:
				t = gitTarget{App: a.Name, Error: perr.Error(), Files: map[string]gitTargetFile{}, ComputedAt: nowStamp()}
			default:
				t = computeTarget(p, *api, cat, a, st)
			}
			mu.Lock()
			out = append(out, t)
			mu.Unlock()
		}(a)
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].App < out[j].App })
	var changes []gitTargetChange
	for _, t := range out {
		if old, ok := loadGitTarget(userID, t.App); ok && t.Error == "" && old.Error == "" && old.Version != t.Version && old.Version != "" {
			changes = append(changes, gitTargetChange{App: t.App, Old: old.Version, New: t.Version})
		}
		if t.Error != "" {
			// keep the last good target for comparisons, but record the error
			if old, ok := loadGitTarget(userID, t.App); ok && old.Error == "" {
				old.Error = ""
				old.Warnings = append([]string{"last refresh failed: " + t.Error}, nonNil(old.Warnings)...)
				if len(old.Warnings) > 5 {
					old.Warnings = old.Warnings[:5]
				}
				db.Exec(`UPDATE git_targets SET data=? WHERE user_id=? AND app=?`, string(jsonMarshal(old)), userID, t.App)
				continue
			}
		}
		db.Exec(`INSERT OR REPLACE INTO git_targets (user_id, app, data, computed_at) VALUES (?,?,?,?)`, userID, t.App, string(jsonMarshal(t)), t.ComputedAt)
	}
	if api != nil && p != nil {
		msg := ""
		if perr != nil {
			msg = perr.Error()
		}
		for _, t := range out {
			if t.Error != "" && msg == "" {
				msg = t.Error
			}
		}
		db.Exec(`UPDATE git_sources SET last_error=? WHERE id=?`, truncateStr(msg, 300), api.ID)
	}
	// forget targets of services that are gone
	names := map[string]bool{}
	for _, a := range all {
		names[a.Name] = true
	}
	for _, t := range loadGitTargets(userID) {
		if !names[t.App] {
			db.Exec(`DELETE FROM git_targets WHERE user_id=? AND app=?`, userID, t.App)
		}
	}
	return out, changes
}

// ─── locks: one check run per user ───────────────────

var gitRunning = struct {
	sync.Mutex
	m map[int]bool
}{m: map[int]bool{}}

func gitLock(userID int) bool {
	gitRunning.Lock()
	defer gitRunning.Unlock()
	if gitRunning.m[userID] {
		return false
	}
	gitRunning.m[userID] = true
	return true
}

func gitUnlock(userID int) {
	gitRunning.Lock()
	delete(gitRunning.m, userID)
	gitRunning.Unlock()
}

// ─── API ─────────────────────────────────────────────

func gitState(userID int) map[string]interface{} {
	cat, st := loadGitWorkspace(userID)
	var last string
	db.QueryRow(`SELECT last_check_at FROM git_workspace WHERE user_id=?`, userID).Scan(&last)
	apps := effectiveApps(userID, cat)
	fromBundle := []string{}
	for _, a := range apps[len(cat.Apps):] {
		fromBundle = append(fromBundle, a.Name)
	}
	return map[string]interface{}{
		"catalog": cat, "settings": st, "sources": loadGitSources(userID), "targets": loadGitTargets(userID),
		"server_health": loadGitServerHealth(userID), "installs": loadGitInstalls(userID, false, ""), "bundle_apps": fromBundle, "last_check_at": last,
		"can_check": gitChecksAllowed(userID), "interval_minutes": settingInt("git_check_interval_minutes"),
		"notifications": settingBool("notifications_enabled"), "deploy": gitDeployPolicies(userID),
		"grace_minutes": settingInt("git_schedule_grace_minutes"), "feeds": loadGitFeeds(userID), "pipelines": loadGitPipelines(userID), "hosts": hostLabels(userID),
	}
}

func decodeGitJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		jsonError(w, "Invalid request: "+err.Error(), 400)
		return false
	}
	return true
}

// /api/git…
//
//	GET    /api/git                         → workspace state
//	PUT    /api/git/settings                 settings
//	GET    /api/git/catalog/export           catalog JSON (download)
//	PUT    /api/git/catalog                  whole catalog
//	POST   /api/git/catalog/import?mode=     replace | merge
//	PUT    /api/git/catalog/apps/{name}      add or change one service; DELETE removes it
//	POST   /api/git/sources                  {kind, name, url, token}; PUT/DELETE /api/git/sources/{id}
//	POST   /api/git/sources/{id}/test        → projects
//	POST   /api/git/suggest                  {project, ref} → catalog entry from the tree
//	POST   /api/git/bundles                  .tar.gz body → imported bundle
//	POST   /api/git/bundles/export           {apps} → .tar.gz
//	POST   /api/git/targets/refresh          {apps}
//	GET    /api/git/targets/{app}            target with files
//	POST   /api/git/check                    {conn_ids, discover} → discovery + comparison
//	POST   /api/git/installs                 {conn_id, path, app} (added by hand)
//	GET    /api/git/installs/{id}            details with file states; DELETE forgets it
//	GET    /api/git/installs/{id}/diff?path= coloured diff server ↔ target (any file in the repository)
//	GET    /api/git/installs/{id}/files      every file of the folder and of the repository, with reasons
//	GET    /api/git/installs/{id}/file?path=&side= one file for viewing (server or git)
//	GET    /api/git/installs/{id}/commits    commits between VERSION.md's commit and the target
//	check jobs (partial, streamed, cancellable): see apiGitJobs in git_jobs.go
//	deploy routes (plan, backups, runs): see apiGitDeploy in git_runs.go
//	.gitignore helper: see apiGitIgnore in git_gitignore.go
//	webhooks, artifact feeds, CI pipelines: see apiGitCI in git_ci.go
//	environments (plans, history, doctor, locks, excludes): see apiGitEnv in git_env.go
//	activity per branch and author: see apiGitActivity in git_activity.go
//	deploy history and "what is where": see apiGitWhere in git_where.go
func apiGitHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if !gitAllowed() {
		jsonError(w, "The Git workspace is turned off by the administrator", 403)
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/git"), "/")
	parts := strings.Split(rest, "/")
	needCheck := func() bool {
		if !gitChecksAllowed(userID) {
			jsonError(w, "Checks are not allowed for your account", 403)
			return false
		}
		return true
	}
	if apiGitRelay(w, r, userID, rest) || apiGitConfigProfiles(w, r, userID, rest) || apiGitJobs(w, r, userID, rest) || apiGitEnv(w, r, userID, rest) || apiGitActivity(w, r, userID, rest) || apiGitWhere(w, r, userID, rest) || apiGitDeploy(w, r, userID, rest, parts) || apiGitIgnore(w, r, userID, rest, parts) || apiGitCI(w, r, userID, rest, parts) {
		return
	}
	switch {
	case rest == "" && r.Method == http.MethodGet:
		jsonOK(w, gitState(userID))

	case rest == "settings" && r.Method == http.MethodPut:
		var st gitSettings
		if !decodeGitJSON(w, r, &st) {
			return
		}
		if err := st.normalize(userID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		saveGitWorkspace(userID, nil, &st)
		auditLog(r, userID, "git.settings_changed", "", map[string]interface{}{"mode": st.Mode, "ref_mode": st.RefMode, "servers": len(st.ConnIDs), "monitor": st.Monitor})
		jsonOK(w, gitState(userID))

	case rest == "catalog/export" && r.Method == http.MethodGet:
		cat := loadGitCatalog(userID)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="catalog.json"`)
		w.Write(jsonIndent1(cat))

	case (rest == "catalog" && r.Method == http.MethodPut) || (rest == "catalog/import" && r.Method == http.MethodPost):
		var in gitCatalog
		if !decodeGitJSON(w, r, &in) {
			return
		}
		if rest == "catalog/import" && r.URL.Query().Get("mode") == "merge" {
			cur := loadGitCatalog(userID)
			for _, a := range in.Apps {
				if x, found := cur.app(a.Name); found {
					*x = a
				} else {
					cur.Apps = append(cur.Apps, a)
				}
			}
			if in.GitLab.URL != "" {
				cur.GitLab = in.GitLab
			}
			cur.FallbackBranches = append(cur.FallbackBranches, in.FallbackBranches...)
			cur.ServerRoots = append(cur.ServerRoots, in.ServerRoots...)
			if in.IgnoreDirs != nil {
				cur.IgnoreDirs = append(cur.IgnoreDirs, in.IgnoreDirs...)
			}
			in = *cur
		}
		if err := in.normalize(); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		saveGitWorkspace(userID, &in, nil)
		auditLog(r, userID, "git.catalog_saved", "", map[string]interface{}{"services": len(in.Apps), "import": rest == "catalog/import"})
		jsonOK(w, gitState(userID))

	case len(parts) == 3 && parts[0] == "catalog" && parts[1] == "apps" && (r.Method == http.MethodPut || r.Method == http.MethodDelete):
		cat := loadGitCatalog(userID)
		name := parts[2]
		idx := -1
		for i, a := range cat.Apps {
			if a.Name == name {
				idx = i
			}
		}
		if r.Method == http.MethodDelete {
			if idx < 0 {
				jsonError(w, "Service not found", 404)
				return
			}
			cat.Apps = append(cat.Apps[:idx], cat.Apps[idx+1:]...)
			db.Exec(`DELETE FROM git_targets WHERE user_id=? AND app=?`, userID, name)
		} else {
			var a gitCatalogApp
			if !decodeGitJSON(w, r, &a) {
				return
			}
			if idx >= 0 {
				if a.Environments == nil {
					a.Environments = cat.Apps[idx].Environments // a client that does not know them keeps them
				}
				cat.Apps[idx] = a
			} else {
				cat.Apps = append(cat.Apps, a)
			}
			if a.Name != name && idx >= 0 {
				db.Exec(`DELETE FROM git_targets WHERE user_id=? AND app=?`, userID, name)
				db.Exec(`UPDATE git_installs SET app=? WHERE user_id=? AND app=?`, a.Name, userID, name)
			}
		}
		if err := cat.normalize(); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		saveGitWorkspace(userID, cat, nil)
		auditLog(r, userID, "git.service_saved", name, map[string]interface{}{"deleted": r.Method == http.MethodDelete})
		jsonOK(w, gitState(userID))

	case rest == "sources" && r.Method == http.MethodPost, len(parts) == 2 && parts[0] == "sources" && r.Method == http.MethodPut:
		var in struct {
			Kind       string  `json:"kind"`
			Name       string  `json:"name"`
			URL        string  `json:"url"`
			Token      *string `json:"token"`
			WriteToken *string `json:"write_token"` // optional, only for .gitignore merge requests
		}
		if !decodeGitJSON(w, r, &in) {
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		in.URL = strings.TrimRight(strings.TrimSpace(in.URL), "/")
		if !gitAPIKind(in.Kind) {
			jsonError(w, "Kind must be gitlab, github or gitea", 400)
			return
		}
		if _, err := gitAPIBase(in.Kind, in.URL); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if in.Name == "" {
			in.Name = in.URL
		}
		if rest == "sources" {
			var n int
			db.QueryRow(`SELECT COUNT(*) FROM git_sources WHERE user_id=? AND kind<>'bundle'`, userID).Scan(&n)
			if n >= 20 {
				jsonError(w, "at most 20 Git sources", 400)
				return
			}
			tok, wtok := "", ""
			if in.Token != nil && strings.TrimSpace(*in.Token) != "" {
				tok = encryptValue(strings.TrimSpace(*in.Token))
			}
			if in.WriteToken != nil && strings.TrimSpace(*in.WriteToken) != "" {
				wtok = encryptValue(strings.TrimSpace(*in.WriteToken))
			}
			res, err := db.Exec(`INSERT INTO git_sources (user_id, kind, name, url, token, info, created_at, last_error, write_token) VALUES (?,?,?,?,?,'',?,'',?)`,
				userID, in.Kind, truncateStr(in.Name, 100), in.URL, tok, nowStamp(), wtok)
			if err != nil {
				jsonError(w, "Cannot save", 500)
				return
			}
			id, _ := res.LastInsertId()
			auditLog(r, userID, "git.source_added", in.Name, map[string]interface{}{"kind": in.Kind, "url": in.URL, "source_id": id, "write_access": wtok != ""})
		} else {
			id, _ := strconv.Atoi(parts[1])
			src, found := loadGitSource(userID, id)
			if !found || !src.Owned || src.Kind == "bundle" {
				jsonError(w, "Source not found", 404)
				return
			}
			tx, err := db.Begin()
			if err != nil {
				jsonError(w, "Cannot update source", 500)
				return
			}
			defer tx.Rollback()
			tx.Exec(`UPDATE git_sources SET kind=?, name=?, url=? WHERE id=?`, in.Kind, truncateStr(in.Name, 100), in.URL, id)
			if in.Token != nil {
				tok := ""
				if t := strings.TrimSpace(*in.Token); t != "" {
					tok = encryptValue(t)
				}
				tx.Exec(`UPDATE git_sources SET token=? WHERE id=?`, tok, id)
			}
			if in.WriteToken != nil {
				wtok := ""
				if t := strings.TrimSpace(*in.WriteToken); t != "" {
					wtok = encryptValue(t)
				}
				tx.Exec(`UPDATE git_sources SET write_token=? WHERE id=?`, wtok, id)
			}
			if in.URL != src.URL || in.Kind != src.Kind {
				// a different server: cached trees and blobs belong to the old one
				tx.Exec(`UPDATE git_relay_meta SET enabled=CASE WHEN ?='gitlab' THEN enabled ELSE 0 END,published=0,error='Source address changed; synchronize again' WHERE source_id=?`, in.Kind, id)
				tx.Exec(`DELETE FROM git_relay_cache WHERE source_id=?`, id)
				tx.Exec(`DELETE FROM git_blobs WHERE source_id=?`, id)
				tx.Exec(`DELETE FROM git_trees WHERE source_id=?`, id)
			}
			if err := tx.Commit(); err != nil {
				jsonError(w, "Cannot update source", 500)
				return
			}
			auditLog(r, userID, "git.source_changed", in.Name, map[string]interface{}{"kind": in.Kind, "url": in.URL, "source_id": id, "new_secret": in.Token != nil,
				"new_write_secret": in.WriteToken != nil})
		}
		jsonOK(w, gitState(userID))

	case len(parts) == 2 && parts[0] == "sources" && r.Method == http.MethodDelete:
		id, _ := strconv.Atoi(parts[1])
		src, found := loadGitSource(userID, id)
		if !found || !src.Owned {
			jsonError(w, "Source not found", 404)
			return
		}
		deleteGitSource(id)
		auditLog(r, userID, "git.source_deleted", src.Name, map[string]interface{}{"kind": src.Kind, "source_id": id})
		jsonOK(w, gitState(userID))

	case len(parts) == 3 && parts[0] == "sources" && parts[2] == "test" && r.Method == http.MethodPost:
		id, _ := strconv.Atoi(parts[1])
		src, found := loadGitSource(userID, id)
		if !found || src.Kind == "bundle" {
			jsonError(w, "Source not found", 404)
			return
		}
		p, err := newGitProvider(src)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		cat := loadGitCatalog(userID)
		projects, err := p.projects(cat.GitLab.Groups)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		db.Exec(`UPDATE git_sources SET last_error=? WHERE id=?`, truncateStr(msg, 300), id)
		if err != nil {
			jsonError(w, err.Error(), 502)
			return
		}
		jsonOK(w, map[string]interface{}{"projects": projects, "groups": cat.GitLab.Groups})

	case rest == "suggest" && r.Method == http.MethodPost:
		var in struct {
			SourceID int    `json:"source_id"`
			Project  string `json:"project"`
			Ref      string `json:"ref"`
		}
		if !decodeGitJSON(w, r, &in) {
			return
		}
		_, st := loadGitWorkspace(userID)
		if in.SourceID != 0 {
			st.SourceID = in.SourceID
		}
		api, _ := pickSources(userID, st)
		if api == nil {
			jsonError(w, "Add a GitLab or GitHub source first (Settings)", 400)
			return
		}
		p, err := newGitProvider(*api)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		a, files, err := suggestApp(p, *api, strings.Trim(strings.TrimSpace(in.Project), "/"), strings.TrimSpace(in.Ref))
		if err != nil {
			jsonError(w, err.Error(), 502)
			return
		}
		jsonOK(w, map[string]interface{}{"app": a, "files": files})

	case rest == "bundles" && r.Method == http.MethodPost:
		res, err := importBundle(userID, r.Body)
		if err != nil {
			code := 400
			if strings.Contains(err.Error(), "request body too large") {
				code = 413
			}
			jsonError(w, err.Error(), code)
			return
		}
		auditLog(r, userID, "git.bundle_imported", res.Source.Name, map[string]interface{}{"bundle": res.Source.BundleID, "origin": res.Source.Origin,
			"services": len(res.Source.Apps), "files": res.Files, "source_id": res.Source.ID})
		jsonOK(w, map[string]interface{}{"result": res, "state": gitState(userID)})

	case rest == "bundles/export" && r.Method == http.MethodPost:
		var in struct {
			Apps []string `json:"apps"`
		}
		if !decodeGitJSON(w, r, &in) {
			return
		}
		cat, st := loadGitWorkspace(userID)
		var targets []gitTarget
		for _, name := range in.Apps {
			t, ok := loadGitTarget(userID, name)
			if !ok {
				jsonError(w, name+": refresh the targets first", 400)
				return
			}
			targets = append(targets, t)
		}
		var buf bytes.Buffer
		name, err := writeBundle(&buf, userID, cat, st, targets)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		auditLog(r, userID, "git.bundle_exported", name, map[string]interface{}{"services": in.Apps, "bytes": buf.Len()})
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.Write(buf.Bytes())

	case rest == "targets/refresh" && r.Method == http.MethodPost:
		if !needCheck() {
			return
		}
		var in struct {
			Apps []string `json:"apps"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		if !gitLock(userID) {
			jsonError(w, "A check is already running", 409)
			return
		}
		defer gitUnlock(userID)
		ts, _ := refreshTargets(userID, in.Apps)
		briefs := make([]gitTargetBrief, len(ts))
		for i, t := range ts {
			briefs[i] = t.brief()
		}
		jsonOK(w, map[string]interface{}{"refreshed": briefs, "state": gitState(userID)})

	case len(parts) == 2 && parts[0] == "targets" && r.Method == http.MethodGet:
		t, ok := loadGitTarget(userID, parts[1])
		if !ok {
			jsonError(w, "No target for this service yet", 404)
			return
		}
		jsonOK(w, t)

	case rest == "check" && r.Method == http.MethodPost:
		if !needCheck() {
			return
		}
		var in struct {
			ConnIDs  []int `json:"conn_ids"`
			Discover bool  `json:"discover"`
			Refresh  bool  `json:"refresh"`
		}
		if !decodeGitJSON(w, r, &in) {
			return
		}
		_, st := loadGitWorkspace(userID)
		ids := in.ConnIDs
		if len(ids) == 0 {
			ids = st.ConnIDs
		}
		for _, id := range ids {
			if !userOwnsConnection(id, userID) {
				jsonError(w, "Connection not found", 404)
				return
			}
		}
		if len(ids) == 0 {
			jsonError(w, "Choose the servers to check (Settings)", 400)
			return
		}
		if !gitLock(userID) {
			jsonError(w, "A check is already running", 409)
			return
		}
		defer gitUnlock(userID)
		res := runGitRound(userID, ids, in.Discover, in.Refresh, false)
		auditLog(r, userID, "git.checked", "", map[string]interface{}{"servers": len(ids), "discover": in.Discover, "installs": len(res.Installs)})
		jsonOK(w, map[string]interface{}{"result": res, "state": gitState(userID)})

	case rest == "installs" && r.Method == http.MethodPost:
		var in struct {
			ConnID int    `json:"conn_id"`
			Path   string `json:"path"`
			App    string `json:"app"`
			Env    string `json:"env"`
		}
		if !decodeGitJSON(w, r, &in) {
			return
		}
		in.Path = strings.TrimRight(strings.TrimSpace(in.Path), "/")
		if !userOwnsConnection(in.ConnID, userID) {
			jsonError(w, "Connection not found", 404)
			return
		}
		if !strings.HasPrefix(in.Path, "/") || strings.ContainsAny(in.Path, "\n\r\t") || strings.Contains(in.Path, "/../") {
			jsonError(w, "Enter an absolute directory path", 400)
			return
		}
		found := false
		for _, a := range effectiveApps(userID, loadGitCatalog(userID)) {
			found = found || a.Name == in.App
		}
		if !found {
			jsonError(w, "Unknown service", 400)
			return
		}
		_, err := db.Exec(`INSERT INTO git_installs (user_id, conn_id, app, path, env, manual, state, data, checked_at, discovered_at) VALUES (?,?,?,?,?,1,'unknown','','',?)
			ON CONFLICT(user_id, conn_id, path) DO UPDATE SET app=excluded.app, env=excluded.env, manual=1`,
			userID, in.ConnID, in.App, in.Path, truncateStr(strings.TrimSpace(in.Env), 40), nowStamp())
		if err != nil {
			jsonError(w, "Cannot save", 500)
			return
		}
		auditLogRef(r, userID, usernameOf(userID), "git.install_added", in.Path, map[string]interface{}{"app": in.App}, auditRef{ConnID: in.ConnID})
		jsonOK(w, gitState(userID))

	case len(parts) >= 2 && parts[0] == "installs":
		id, _ := strconv.Atoi(parts[1])
		list := loadGitInstalls(userID, true, "i.id=?", id)
		if len(list) == 0 {
			jsonError(w, "Installation not found", 404)
			return
		}
		in := list[0]
		switch {
		case len(parts) == 2 && r.Method == http.MethodGet:
			jsonOK(w, in)
		case len(parts) == 2 && r.Method == http.MethodDelete:
			db.Exec(`DELETE FROM git_installs WHERE id=? AND user_id=?`, id, userID)
			jsonOK(w, gitState(userID))
		case len(parts) == 3 && parts[2] == "configuration" && r.Method == http.MethodGet:
			if !needCheck() {
				return
			}
			out, err := inspectGitConfigs(userID, in)
			if err != nil {
				jsonError(w, redactText(err.Error()), 502)
				return
			}
			auditLogRef(r, userID, usernameOf(userID), "git.configuration_viewed", in.Path, map[string]interface{}{"app": in.App, "files": len(out)}, auditRef{ConnID: in.ConnID})
			jsonOK(w, map[string]interface{}{"files": out, "read_at": nowStamp()})
		case len(parts) == 3 && parts[2] == "diff" && r.Method == http.MethodGet:
			if !needCheck() {
				return
			}
			out, err := installDiff(userID, in, r.URL.Query().Get("path"))
			if err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
			auditLogRef(r, userID, usernameOf(userID), "git.diff_viewed", in.Path+"/"+r.URL.Query().Get("path"), map[string]interface{}{"app": in.App}, auditRef{ConnID: in.ConnID})
			jsonOK(w, out)
		case len(parts) == 3 && parts[2] == "files" && r.Method == http.MethodGet:
			if !needCheck() {
				return
			}
			out, err := listInstallFiles(userID, in)
			if err != nil {
				jsonError(w, err.Error(), 502)
				return
			}
			jsonOK(w, out)
		case len(parts) == 3 && parts[2] == "file" && r.Method == http.MethodGet:
			if !needCheck() {
				return
			}
			out, err := viewInstallFile(userID, in, r.URL.Query().Get("path"), r.URL.Query().Get("side"))
			if err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
			auditLogRef(r, userID, usernameOf(userID), "git.file_viewed", in.Path+"/"+cleanRel(r.URL.Query().Get("path")), map[string]interface{}{"app": in.App, "side": out["side"]}, auditRef{ConnID: in.ConnID})
			jsonOK(w, out)
		case len(parts) == 3 && parts[2] == "commits" && r.Method == http.MethodGet:
			if !needCheck() {
				return
			}
			out, err := installCommits(userID, in, r.URL.Query().Get("md") == "1")
			if err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
			jsonOK(w, out)
		default:
			jsonError(w, "Not found", 404)
		}

	default:
		jsonError(w, "Not found", 404)
	}
}

// runGitRound refreshes the targets (optional), checks the servers and returns the result;
// with notify it also sends the Git notifications of the round.
func runGitRound(userID int, ids []int, discover, refresh, notify bool) gitRunResult {
	targets := map[string]gitTarget{}
	var tchanges []gitTargetChange
	if refresh {
		var ts []gitTarget
		ts, tchanges = refreshTargets(userID, nil)
		for _, t := range ts {
			if t.Error == "" {
				targets[t.App] = t
			}
		}
	}
	res := runGitCheck(userID, ids, discover, targets)
	res.Targets = loadGitTargets(userID)
	db.Exec(`INSERT OR IGNORE INTO git_workspace (user_id) VALUES (?)`, userID)
	db.Exec(`UPDATE git_workspace SET last_check_at=? WHERE user_id=?`, nowStamp(), userID)
	if notify {
		notifyGitRound(userID, tchanges, res.Changes)
	}
	return res
}

// ─── diff ────────────────────────────────────────────

const gitMaxDiffFile = 2 << 20

type diffLine struct {
	Op string `json:"op"` // " " | "-" | "+" | "@"
	A  int    `json:"a,omitempty"`
	B  int    `json:"b,omitempty"`
	S  string `json:"s"`
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// unifiedDiff compares a (server) with b (target): LCS over the part between the common
// prefix and suffix, with context lines around changes.
func unifiedDiff(a, b []string, ctx int) ([]diffLine, bool) {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	ma, mb := a[pre:len(a)-suf], b[pre:len(b)-suf]
	type op struct {
		k    byte
		i, j int
	}
	var ops []op
	for i := 0; i < pre; i++ {
		ops = append(ops, op{' ', i, i})
	}
	if len(ma) > 4_000_000 || len(mb) > 4_000_000 || len(ma)*len(mb) > 4_000_000 {
		for i := range ma {
			ops = append(ops, op{'-', pre + i, -1})
		}
		for j := range mb {
			ops = append(ops, op{'+', -1, pre + j})
		}
	} else {
		n, m := len(ma), len(mb)
		dp := make([][]int32, n+1)
		for i := range dp {
			dp[i] = make([]int32, m+1)
		}
		for i := n - 1; i >= 0; i-- {
			for j := m - 1; j >= 0; j-- {
				if ma[i] == mb[j] {
					dp[i][j] = dp[i+1][j+1] + 1
				} else if dp[i+1][j] >= dp[i][j+1] {
					dp[i][j] = dp[i+1][j]
				} else {
					dp[i][j] = dp[i][j+1]
				}
			}
		}
		i, j := 0, 0
		for i < n || j < m {
			switch {
			case i < n && j < m && ma[i] == mb[j]:
				ops = append(ops, op{' ', pre + i, pre + j})
				i++
				j++
			case j < m && (i == n || dp[i][j+1] > dp[i+1][j]):
				ops = append(ops, op{'+', -1, pre + j})
				j++
			default:
				ops = append(ops, op{'-', pre + i, -1})
				i++
			}
		}
	}
	for k := 0; k < suf; k++ {
		ops = append(ops, op{' ', len(a) - suf + k, len(b) - suf + k})
	}
	// keep context lines around changes
	keep := make([]bool, len(ops))
	for x, o := range ops {
		if o.k != ' ' {
			for y := x - ctx; y <= x+ctx; y++ {
				if y >= 0 && y < len(ops) {
					keep[y] = true
				}
			}
		}
	}
	out := []diffLine{}
	gap := false
	truncated := false
	for x, o := range ops {
		if !keep[x] {
			gap = true
			continue
		}
		if gap || len(out) == 0 {
			al, bl := o.i, o.j
			if al < 0 {
				al = 0
			}
			if bl < 0 {
				bl = 0
			}
			out = append(out, diffLine{Op: "@", A: al + 1, B: bl + 1})
			gap = false
		}
		var s string
		if o.k == '+' {
			s = b[o.j]
		} else {
			s = a[o.i]
		}
		out = append(out, diffLine{Op: string(o.k), A: o.i + 1, B: o.j + 1, S: truncateStr(s, 2000)})
		if len(out) > 5000 {
			truncated = true
			break
		}
	}
	if len(out) == 1 && out[0].Op == "@" {
		out = out[:0]
	}
	return out, truncated
}
