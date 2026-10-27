// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─── DEPLOY HISTORY AND "WHAT IS WHERE" ──────────────
//
// The deploy history merges the server-side history.jsonl of every destination of every
// environment (read over SSH, one request per service and environment, like the overview
// cells) with the WRM runs of installations without environments (git_runs; environment
// runs are left out there, their destinations have the history). Filters: service,
// environment, server, who, branch, version, action and dates.
//
// The matrix lists per service, environment and server the branch, commit and version that
// runs there, who deployed it and when, and how many commits it is behind the head of its
// branch (the provider's compare API, one call per distinct commit and branch head),
// highlighting servers of an environment that differ from the others. Installations
// without environments come from the stored checks (no SSH).
//
// Cells are cached for gitWhereTTL so that changing a filter or exporting CSV does not log
// in again; refresh=1 reads them again.

const (
	gitWhereTTL     = 2 * time.Minute
	gitHistoryRuns  = 500 // WRM runs read for the history
	gitHistoryFiles = 2000
)

// ─── deploy history ──────────────────────────────────

type gitHistoryRow struct {
	At        string              `json:"at"`
	Action    string              `json:"action"` // deploy | rollback | update | upgrade | install | transfer
	Source    string              `json:"source"` // env (history.jsonl on the server) | run (WRM run)
	App       string              `json:"app"`
	Env       string              `json:"env,omitempty"`
	Server    string              `json:"server"`
	Path      string              `json:"path"`
	By        string              `json:"by"`
	Branch    string              `json:"branch,omitempty"`
	Version   string              `json:"version,omitempty"`
	Commit    string              `json:"commit,omitempty"`
	Ref       string              `json:"ref,omitempty"`
	ID        string              `json:"id,omitempty"` // deploy id or run label
	RunID     int                 `json:"run_id,omitempty"`
	Result    string              `json:"result"` // ok | failed | partial | post_deploy_failed | rolled_back
	Previous  string              `json:"previous,omitempty"`
	Counts    map[string]int      `json:"counts,omitempty"`
	Files     map[string][]string `json:"files,omitempty"`
	Error     string              `json:"error,omitempty"`
	Note      string              `json:"note,omitempty"`
	CommitURL string              `json:"commit_url,omitempty"`
	at        time.Time
}

type gitHistoryFilter struct {
	App, Env, Server, Who, Branch, Version, Action string
	Since, Until                                   time.Time // zero = open
}

var gitHistoryActions = map[string]bool{"deploy": true, "rollback": true, "update": true, "upgrade": true, "install": true, "transfer": true}

func parseHistoryFilter(q url.Values) (gitHistoryFilter, error) {
	f := gitHistoryFilter{App: q.Get("app"), Env: q.Get("env"), Server: q.Get("server"), Who: strings.TrimSpace(q.Get("who")),
		Branch: strings.TrimSpace(q.Get("branch")), Version: strings.TrimSpace(q.Get("version")), Action: q.Get("action")}
	if f.Action != "" && !gitHistoryActions[f.Action] {
		return f, fmt.Errorf("unknown action %q", f.Action)
	}
	var err error
	f.Since, f.Until, err = parseRange(q, 0)
	if q.Get("until") == "" {
		f.Until = time.Time{}
	}
	return f, err
}

func containsFold(s, sub string) bool {
	return sub == "" || strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func (f gitHistoryFilter) match(r gitHistoryRow) bool {
	if (f.App != "" && r.App != f.App) || (f.Env != "" && r.Env != f.Env) || (f.Server != "" && r.Server != f.Server) || (f.Action != "" && r.Action != f.Action) {
		return false
	}
	if !containsFold(r.By, f.Who) || !containsFold(r.Branch, f.Branch) || !(containsFold(r.Version, f.Version) || containsFold(r.Commit, f.Version)) {
		return false
	}
	if !f.Since.IsZero() && (r.at.IsZero() || r.at.Before(f.Since)) {
		return false
	}
	if !f.Until.IsZero() && (r.at.IsZero() || !r.at.Before(f.Until.AddDate(0, 0, 1))) {
		return false
	}
	return true
}

func filterHistory(rows []gitHistoryRow, f gitHistoryFilter) []gitHistoryRow {
	out := []gitHistoryRow{}
	for _, r := range rows {
		if f.match(r) {
			out = append(out, r)
		}
	}
	sortHistory(out)
	return out
}

// sortHistory orders newest first, then by service, environment and server.
func sortHistory(rows []gitHistoryRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if !a.at.Equal(b.at) {
			return a.at.After(b.at)
		}
		if a.App != b.App {
			return a.App < b.App
		}
		if a.Env != b.Env {
			return a.Env < b.Env
		}
		return a.Server < b.Server
	})
}

func str(m map[string]interface{}, k string) string {
	switch v := m[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

var historyFileKinds = []string{"new", "changed", "deleted", "excluded", "restored", "moved_aside", "skipped", "failed", "moved"}

// historyEntryRow turns one history.jsonl entry (as envHistory returns it) into a row.
func historyEntryRow(app, env string, e map[string]interface{}) gitHistoryRow {
	r := gitHistoryRow{At: str(e, "at"), Source: "env", App: app, Env: env, Server: str(e, "server"), Path: str(e, "path"), By: str(e, "by"),
		Branch: str(e, "branch"), Version: str(e, "version"), Commit: str(e, "commit"), Ref: str(e, "ref"), ID: str(e, "id"),
		Error: str(e, "error"), Note: str(e, "note")}
	r.at = parseAPITime(r.At)
	if strings.EqualFold(str(e, "action"), "ROLLBACK") {
		r.Action, r.Result = "rollback", "ok"
		if b, ok := e["complete"].(bool); ok && !b {
			r.Result = "failed"
		}
		if from, ok := e["from"].(map[string]interface{}); ok {
			r.Previous = str(from, "version")
		}
	} else {
		r.Action, r.Result = "deploy", str(e, "result")
		if r.Result == "" {
			r.Result = "ok"
		}
		if prev, ok := e["previous"].(map[string]interface{}); ok {
			r.Previous = str(prev, "version")
		}
	}
	if c, ok := e["counts"].(map[string]interface{}); ok {
		r.Counts = map[string]int{}
		for k, v := range c {
			if n, ok := v.(float64); ok {
				r.Counts[k] = int(n)
			}
		}
	}
	for _, k := range historyFileKinds {
		l, ok := e[k].([]interface{})
		if !ok || len(l) == 0 {
			continue
		}
		if r.Files == nil {
			r.Files = map[string][]string{}
		}
		for _, x := range l {
			if s, ok := x.(string); ok && len(r.Files[k]) < gitHistoryFiles {
				r.Files[k] = append(r.Files[k], s)
			}
		}
	}
	return r
}

type gitHistoryCell struct {
	App    string          `json:"app"`
	Env    string          `json:"env"`
	Rows   []gitHistoryRow `json:"rows"`
	Errors []string        `json:"errors"`
	At     string          `json:"at"`
}

// envHistoryRows reads (or takes from the cache) the history of every destination of an
// environment.
func envHistoryRows(userID int, app, env string, refresh bool) (*gitHistoryCell, error) {
	key := fmt.Sprintf("hist|%d|%s|%s", userID, app, env)
	if !refresh {
		if v, _, ok := shortCacheGet(key, gitWhereTTL); ok {
			return v.(*gitHistoryCell), nil
		}
	}
	h, err := envHistory(userID, app, env)
	if err != nil {
		return nil, err
	}
	cell := &gitHistoryCell{App: app, Env: env, Rows: []gitHistoryRow{}, Errors: []string{}, At: nowStamp()}
	web := serviceWeb(userID, app)
	if entries, ok := h["entries"].([]map[string]interface{}); ok {
		for _, e := range entries {
			r := historyEntryRow(app, env, e)
			r.CommitURL = web.commit(r.Commit)
			cell.Rows = append(cell.Rows, r)
		}
	}
	var dests []struct {
		Server string `json:"server"`
		Error  string `json:"error"`
	}
	jsonUnmarshalBytes(jsonMarshal(h["dests"]), &dests)
	for _, d := range dests {
		if d.Error != "" {
			cell.Errors = append(cell.Errors, d.Server+": "+d.Error)
		}
	}
	shortCachePut(key, cell)
	return cell, nil
}

// serviceWeb returns the web links of a service's project ("" links without an API source).
func serviceWeb(userID int, app string) gitWeb {
	_, a, src, err := serviceSource(userID, app)
	if err != nil {
		return gitWeb{}
	}
	return newGitWeb(src, a.Project)
}

// runHistoryRows lists the WRM runs of installations (environment runs excluded: their
// destinations keep the history).
func runHistoryRows(userID int) []gitHistoryRow {
	out := []gitHistoryRow{}
	rows, err := db.Query(`SELECT id FROM git_runs WHERE user_id=? AND kind IN ('update','upgrade','rollback','install','transfer') AND state NOT IN ('scheduled','starting')
		ORDER BY id DESC LIMIT ?`, userID, gitHistoryRuns)
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
	by := usernameOf(userID)
	webs := map[string]gitWeb{}
	for _, id := range ids {
		v, p, ok := loadGitRunView(userID, id)
		if !ok || p.Env != nil || v.Running {
			continue
		}
		at := v.EndedAt
		if at == "" {
			at = v.StartedAt
		}
		if at == "" {
			at = v.CreatedAt
		}
		for _, it := range v.Items {
			switch it.State {
			case "pending", "skipped", "cancelled", "running", "":
				continue
			}
			r := gitHistoryRow{At: at, Action: v.Kind, Source: "run", App: it.App, Env: it.Env, Server: it.ConnName, Path: it.Path, By: by,
				ID: v.Label, RunID: v.ID, Result: it.State, Previous: it.FromVersion, Error: it.Error, Note: it.Note}
			r.at = parseAPITime(at)
			if t, ok := p.Targets[it.App]; ok && v.Kind != "rollback" {
				r.Branch, r.Version, r.Commit, r.Ref = t.Branch, t.Version, t.CommitFull, t.Ref
				if r.Commit == "" {
					r.Commit = t.Commit
				}
			} else {
				r.Version = it.ToVersion
			}
			if len(it.Written) > 0 || len(it.Deleted) > 0 {
				r.Files = map[string][]string{}
				r.Counts = map[string]int{}
				if len(it.Written) > 0 {
					r.Files["written"], r.Counts["written"] = capList(it.Written, gitHistoryFiles), len(it.Written)
				}
				if len(it.Deleted) > 0 {
					r.Files["deleted"], r.Counts["deleted"] = capList(it.Deleted, gitHistoryFiles), len(it.Deleted)
				}
			}
			if r.Commit != "" {
				w, ok := webs[it.App]
				if !ok {
					w = serviceWeb(userID, it.App)
					webs[it.App] = w
				}
				r.CommitURL = w.commit(r.Commit)
			}
			out = append(out, r)
		}
	}
	return out
}

// envPairs lists the (service, environment) pairs of the catalog that pass a filter.
func envPairs(userID int, app, env string) [][2]string {
	out := [][2]string{}
	for _, a := range loadGitCatalog(userID).Apps {
		if app != "" && a.Name != app {
			continue
		}
		for _, e := range a.Environments {
			if env == "" || e.Name == env {
				out = append(out, [2]string{a.Name, e.Name})
			}
		}
	}
	return out
}

func joinFiles(files map[string][]string) string {
	var b strings.Builder
	keys := append(append([]string{}, historyFileKinds...), "written")
	for _, k := range keys {
		for _, f := range files[k] {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(k + ": " + f)
		}
	}
	return b.String()
}

var historyCSVHeader = []string{"time", "action", "result", "service", "environment", "server", "path", "who", "branch", "version", "commit", "deploy_id", "run_id",
	"previous", "new", "changed", "deleted", "excluded", "restored", "moved_aside", "written", "files", "error", "source", "commit_url"}

func historyCSVRows(rows []gitHistoryRow) [][]string {
	out := [][]string{}
	n := func(r gitHistoryRow, k string) string {
		if v, ok := r.Counts[k]; ok {
			return strconv.Itoa(v)
		}
		if l := r.Files[k]; len(l) > 0 {
			return strconv.Itoa(len(l))
		}
		return ""
	}
	for _, r := range rows {
		run := ""
		if r.RunID > 0 {
			run = strconv.Itoa(r.RunID)
		}
		out = append(out, []string{r.At, r.Action, r.Result, r.App, r.Env, r.Server, r.Path, r.By, r.Branch, r.Version, r.Commit, r.ID, run,
			r.Previous, n(r, "new"), n(r, "changed"), n(r, "deleted"), n(r, "excluded"), n(r, "restored"), n(r, "moved_aside"), n(r, "written"),
			joinFiles(r.Files), r.Error, r.Source, r.CommitURL})
	}
	return out
}

// ─── what is where ───────────────────────────────────

type gitWhereRow struct {
	App        string `json:"app"`
	Env        string `json:"env"`
	Server     string `json:"server"`
	Path       string `json:"path"`
	Kind       string `json:"kind"` // env (a destination) | install (an installation without environment)
	Exists     bool   `json:"exists"`
	Branch     string `json:"branch,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Version    string `json:"version,omitempty"`
	By         string `json:"by,omitempty"`
	At         string `json:"at,omitempty"`
	DeployID   string `json:"deploy_id,omitempty"`
	Head       string `json:"head,omitempty"` // head commit of the branch
	Behind     int    `json:"behind"`         // commits behind the branch head; -1 = not known
	CommitURL  string `json:"commit_url,omitempty"`
	CompareURL string `json:"compare_url,omitempty"` // the commits it is behind
	BranchURL  string `json:"branch_url,omitempty"`
	Drift      bool   `json:"drift,omitempty"` // differs from the other servers of the environment
	Locked     bool   `json:"locked,omitempty"`
	State      string `json:"state,omitempty"` // installations: the state of the last check
	Error      string `json:"error,omitempty"`
}

type gitWhereCell struct {
	App   string        `json:"app"`
	Env   string        `json:"env,omitempty"`
	Rows  []gitWhereRow `json:"rows"`
	Drift bool          `json:"drift"`
	Note  string        `json:"note,omitempty"`
	At    string        `json:"at"`
}

// branchOf normalises the branch of a deployed state: refs/heads/x → x; a tag or nothing
// falls back to the service's branch.
func branchOf(ref, fallback string) string {
	ref = strings.TrimSpace(ref)
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		return strings.TrimPrefix(ref, "refs/heads/")
	case ref == "" || strings.HasPrefix(ref, "tag:") || strings.HasPrefix(ref, "refs/tags/") || strings.HasPrefix(ref, "bundle"):
		return fallback
	}
	if f := strings.Fields(ref); len(f) > 0 {
		return f[0]
	}
	return fallback
}

// branchHead returns the head commit of a branch.
func (p *gitProvider) branchHead(project, branch string) (string, error) {
	var v struct {
		Commit struct {
			ID  string `json:"id"`
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	rel := p.gl(project) + "/repository/branches/" + url.PathEscape(branch)
	if p.hub() {
		rel = p.gh(project) + "/branches/" + url.PathEscape(branch)
	}
	if _, err := p.getJSON(rel, &v); err != nil {
		return "", err
	}
	if v.Commit.ID != "" {
		return v.Commit.ID, nil
	}
	if v.Commit.SHA == "" {
		return "", fmt.Errorf("branch %s has no commit", branch)
	}
	return v.Commit.SHA, nil
}

// behindCounter counts commits behind branch heads, one API call per branch head and per
// distinct commit (cached for gitWhereTTL across requests).
type behindCounter struct {
	userID  int
	app     *gitCatalogApp
	src     gitSource
	p       *gitProvider
	web     gitWeb
	def     string
	refresh bool
	err     error
}

func newBehindCounter(userID int, app string, refresh bool) *behindCounter {
	bc := &behindCounter{userID: userID, refresh: refresh}
	_, a, src, err := serviceSource(userID, app)
	if err != nil {
		bc.err = err
		return bc
	}
	bc.app, bc.src, bc.web = a, src, newGitWeb(src, a.Project)
	bc.p, bc.err = newGitProvider(src)
	if bc.err == nil {
		bc.def = a.Branch
		if bc.def == "" {
			if t, ok := loadGitTarget(userID, app); ok && t.Branch != "" {
				bc.def = t.Branch
			}
		}
	}
	return bc
}

func (bc *behindCounter) cached(key string, fn func() (interface{}, error)) (interface{}, error) {
	key = fmt.Sprintf("bc|%d|%s|%s", bc.src.ID, bc.app.Project, key)
	if !bc.refresh {
		if v, _, ok := shortCacheGet(key, gitWhereTTL); ok {
			if e, isErr := v.(error); isErr {
				return nil, e
			}
			return v, nil
		}
	}
	v, err := fn()
	if err != nil {
		shortCachePut(key, err)
		return nil, err
	}
	shortCachePut(key, v)
	return v, nil
}

func (bc *behindCounter) branchFor(ref string) string {
	if bc.def == "" && bc.p != nil {
		v, err := bc.cached("default", func() (interface{}, error) { return bc.p.defaultBranch(bc.app.Project) })
		if err == nil {
			bc.def = v.(string)
		}
	}
	return branchOf(ref, bc.def)
}

// fill sets head, behind and the links of a row whose Branch and Commit are known.
func (bc *behindCounter) fill(r *gitWhereRow) {
	r.Behind = -1
	if bc.err != nil || bc.p == nil {
		return
	}
	r.CommitURL = bc.web.commit(r.Commit)
	r.BranchURL = bc.web.branch(r.Branch)
	if r.Branch == "" || r.Commit == "" {
		return
	}
	hv, err := bc.cached("head|"+r.Branch, func() (interface{}, error) { return bc.p.branchHead(bc.app.Project, r.Branch) })
	if err != nil {
		return
	}
	head := hv.(string)
	r.Head = head
	if strings.HasPrefix(head, r.Commit) {
		r.Behind = 0
		return
	}
	nv, err := bc.cached("cmp|"+r.Commit+"|"+head, func() (interface{}, error) {
		n, _, err := bc.p.compareCommits(bc.app.Project, r.Commit, head)
		return n, err
	})
	if err != nil {
		return
	}
	r.Behind = nv.(int)
	r.CompareURL = bc.web.compare(r.Commit, head)
}

// markDrift flags the rows of one environment whose commit and version differ from the
// most common state (ties: the first destination's).
func markDrift(rows []gitWhereRow) bool {
	count := map[string]int{}
	var order []string
	key := func(r gitWhereRow) string { return short10(r.Commit) + "|" + r.Version }
	for _, r := range rows {
		if !r.Exists || r.Error != "" {
			continue
		}
		k := key(r)
		if count[k] == 0 {
			order = append(order, k)
		}
		count[k]++
	}
	if len(order) < 2 {
		return false
	}
	best := order[0]
	for _, k := range order {
		if count[k] > count[best] {
			best = k
		}
	}
	for i := range rows {
		if rows[i].Exists && rows[i].Error == "" && key(rows[i]) != best {
			rows[i].Drift = true
		}
	}
	return true
}

// whereEnvCell reads every destination of an environment (envCell) and adds branch heads.
func whereEnvCell(userID int, app, env string, refresh bool) (*gitWhereCell, error) {
	key := fmt.Sprintf("where|%d|%s|%s", userID, app, env)
	if !refresh {
		if v, _, ok := shortCacheGet(key, gitWhereTTL); ok {
			return v.(*gitWhereCell), nil
		}
	}
	res, err := envCell(userID, app, env)
	if err != nil {
		return nil, err
	}
	dests, _ := res["dests"].([]gitEnvCellDest)
	bc := newBehindCounter(userID, app, refresh)
	cell := &gitWhereCell{App: app, Env: env, Rows: []gitWhereRow{}, At: nowStamp()}
	if bc.err != nil {
		cell.Note = bc.err.Error()
	}
	for _, d := range dests {
		r := gitWhereRow{App: app, Env: env, Server: d.Server, Path: d.Path, Kind: "env", Exists: d.Exists, Error: d.Error, Locked: d.Lock != nil, Behind: -1}
		if d.Current != nil {
			r.Commit, r.Version, r.By, r.At, r.DeployID = d.Current.Commit, d.Current.Version, d.Current.By, d.Current.At, d.Current.DeployID
			r.Branch = bc.branchFor(d.Current.Branch)
		} else if d.Exists {
			r.Commit, r.Version, r.By, r.At = d.CommitMD, d.VersionM, d.UserMD, d.UpdateMD
			r.Branch = bc.branchFor(d.RefMD)
		}
		if r.Exists && r.Error == "" {
			bc.fill(&r)
		}
		cell.Rows = append(cell.Rows, r)
	}
	cell.Drift = markDrift(cell.Rows)
	shortCachePut(key, cell)
	return cell, nil
}

// whereInstalls lists the installations of a service that are no environment destination,
// from the stored checks.
func whereInstalls(userID int, app string, refresh bool) *gitWhereCell {
	cat := loadGitCatalog(userID)
	isDest := map[string]bool{}
	for _, a := range cat.Apps {
		for _, e := range a.Environments {
			for _, d := range e.Destinations {
				isDest[strings.ToLower(d.Server)+"\x00"+d.Path] = true
			}
		}
	}
	cell := &gitWhereCell{App: app, Rows: []gitWhereRow{}, At: nowStamp()}
	var bc *behindCounter
	for _, in := range loadGitInstalls(userID, false, "i.app=?", app) {
		if isDest[strings.ToLower(in.ConnName)+"\x00"+in.Path] {
			continue
		}
		if bc == nil {
			bc = newBehindCounter(userID, app, refresh)
			if bc.err != nil {
				cell.Note = bc.err.Error()
			}
		}
		v := in.VersionMD
		r := gitWhereRow{App: app, Env: in.Env, Server: in.ConnName, Path: in.Path, Kind: "install", Exists: in.State != "gone", State: in.State,
			Version: v.Version, Commit: strings.TrimSpace(strings.Split(strings.TrimSpace(v.Commit), " ")[0]), By: v.User, At: v.Updated, Error: in.Error, Behind: -1}
		r.Branch = bc.branchFor(v.Ref)
		if r.Exists {
			bc.fill(&r)
		}
		cell.Rows = append(cell.Rows, r)
	}
	return cell
}

var whereCSVHeader = []string{"service", "environment", "server", "path", "kind", "branch", "commit", "version", "deployed_by", "deployed_at", "deploy_id",
	"behind", "branch_head", "drift", "locked", "state", "error", "compare_url"}

func whereCSVRows(rows []gitWhereRow) [][]string {
	out := [][]string{}
	yes := func(b bool) string {
		if b {
			return "yes"
		}
		return ""
	}
	for _, r := range rows {
		behind := ""
		if r.Behind >= 0 {
			behind = strconv.Itoa(r.Behind)
		}
		out = append(out, []string{r.App, r.Env, r.Server, r.Path, r.Kind, r.Branch, r.Commit, r.Version, r.By, r.At, r.DeployID,
			behind, r.Head, yes(r.Drift), yes(r.Locked), r.State, r.Error, r.CompareURL})
	}
	return out
}

// ─── API ─────────────────────────────────────────────

// apiGitWhere handles the history and matrix routes of /api/git; it returns false for other
// routes. Reading destinations over SSH needs the git_checks policy.
//
//	GET /api/git/history/runs?<filters>              WRM runs of installations without environments
//	GET /api/git/history/cell?app=&env=&<filters>    history.jsonl of every destination of an environment
//	GET /api/git/history/export.csv?<filters>        everything as CSV
//	GET /api/git/where/cell?app=&env=                the destinations of an environment
//	GET /api/git/where/installs?app=                 installations without environment
//	GET /api/git/where/export.csv?app=&env=&server=  everything as CSV
//
// filters: app, env, server, who, branch, version, action, since, until (YYYY-MM-DD); refresh=1
func apiGitWhere(w http.ResponseWriter, r *http.Request, userID int, rest string) bool {
	if !strings.HasPrefix(rest, "history/") && !strings.HasPrefix(rest, "where/") {
		return false
	}
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", 405)
		return true
	}
	q := r.URL.Query()
	refresh := q.Get("refresh") == "1"
	checks := gitChecksAllowed(userID)
	needCheck := func() bool {
		if !checks {
			jsonError(w, "Checks are not allowed for your account", 403)
		}
		return checks
	}
	switch rest {
	case "history/runs", "history/cell", "history/export.csv":
		f, err := parseHistoryFilter(q)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return true
		}
		switch rest {
		case "history/runs":
			jsonOK(w, map[string]interface{}{"rows": filterHistory(runHistoryRows(userID), f)})
		case "history/cell":
			if !needCheck() {
				return true
			}
			cell, err := envHistoryRows(userID, q.Get("app"), q.Get("env"), refresh)
			if err != nil {
				jsonError(w, err.Error(), 404)
				return true
			}
			out := *cell
			out.Rows = filterHistory(cell.Rows, f)
			jsonOK(w, out)
		default:
			all := runHistoryRows(userID)
			if checks {
				for _, pr := range envPairs(userID, f.App, f.Env) {
					if cell, err := envHistoryRows(userID, pr[0], pr[1], refresh); err == nil {
						all = append(all, cell.Rows...)
					}
				}
			}
			writeCSV(w, csvName("deploy-history", f.App, f.Env), historyCSVHeader, historyCSVRows(filterHistory(all, f)))
		}

	case "where/cell":
		if !needCheck() {
			return true
		}
		cell, err := whereEnvCell(userID, q.Get("app"), q.Get("env"), refresh)
		if err != nil {
			jsonError(w, err.Error(), 404)
			return true
		}
		jsonOK(w, cell)

	case "where/installs":
		cat := loadGitCatalog(userID)
		if _, ok := cat.app(q.Get("app")); !ok {
			jsonError(w, "Service not found", 404)
			return true
		}
		jsonOK(w, whereInstalls(userID, q.Get("app"), refresh))

	case "where/export.csv":
		app, env, server := q.Get("app"), q.Get("env"), q.Get("server")
		var rows []gitWhereRow
		for _, a := range loadGitCatalog(userID).Apps {
			if app != "" && a.Name != app {
				continue
			}
			if checks {
				for _, pr := range envPairs(userID, a.Name, env) {
					if cell, err := whereEnvCell(userID, pr[0], pr[1], refresh); err == nil {
						rows = append(rows, cell.Rows...)
					}
				}
			}
			for _, x := range whereInstalls(userID, a.Name, refresh).Rows {
				if env == "" || x.Env == env {
					rows = append(rows, x)
				}
			}
		}
		filtered := []gitWhereRow{}
		for _, x := range rows {
			if server == "" || x.Server == server {
				filtered = append(filtered, x)
			}
		}
		writeCSV(w, csvName("what-is-where", app, env), whereCSVHeader, whereCSVRows(filtered))

	default:
		jsonError(w, "Not found", 404)
	}
	return true
}
