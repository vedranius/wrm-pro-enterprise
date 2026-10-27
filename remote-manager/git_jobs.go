// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── CHECK JOBS: partial, streamed and cancellable checks ─
//
// A check job compares a chosen set of installations (selected ones, the visible ones,
// whole servers, stale ones or everything) in the background. Every installation is
// scanned on its own (one SSH session on the server's login) and saved as soon as it is
// done, so results stream into the browser, which polls the job. Concurrency is bounded per
// server (git_check_per_server) and overall (git_check_parallel). Installations outside the
// job keep their results; a failed check keeps the last good files and counts. A job holds
// the user's check lock (one check at a time) and can be cancelled: queued installations
// are skipped and running scans are aborted without saving anything.

type gitJobItem struct {
	InstallID int    `json:"install_id"`
	ConnID    int    `json:"conn_id"`
	State     string `json:"state"` // queued | running | done | error | cancelled
	Error     string `json:"error,omitempty"`
	Seq       int    `json:"seq"`
}

type gitJobServer struct {
	plan     bool  // discover on this server first
	installs []int // nil: every installation of the server (after discovery)
}

type gitCheckJob struct {
	mu        sync.Mutex
	ID        int
	UserID    int
	State     string // running | done | cancelled
	Phase     string // targets | checking | ""
	Started   string
	Ended     string
	Discover  bool
	Refresh   bool
	items     map[int]*gitJobItem
	order     []int
	servers   map[int]*gitServerResult
	seq       int
	cancelled bool
	clients   map[*ssh.Client]bool
	active    int
	MaxActive int
	plan      map[int]*gitJobServer
}

type gitJobView struct {
	ID        int               `json:"id"`
	State     string            `json:"state"`
	Phase     string            `json:"phase,omitempty"`
	Started   string            `json:"started"`
	Ended     string            `json:"ended,omitempty"`
	Total     int               `json:"total"`
	Done      int               `json:"done"`
	Seq       int               `json:"seq"`
	MaxActive int               `json:"max_active"`
	Items     []gitJobItem      `json:"items"`
	Servers   []gitServerResult `json:"servers"`
	Installs  []gitInstall      `json:"installs,omitempty"` // installations finished after "since"
}

var gitJobs = struct {
	sync.Mutex
	last   map[int]*gitCheckJob // user → newest job
	nextID int
}{last: map[int]*gitCheckJob{}}

// gitJobTestHook runs inside a scan slot (tests: slow scans to observe the limits).
var gitJobTestHook func(installID int)

func (j *gitCheckJob) view(since int) gitJobView {
	j.mu.Lock()
	v := gitJobView{ID: j.ID, State: j.State, Phase: j.Phase, Started: j.Started, Ended: j.Ended, Seq: j.seq, MaxActive: j.MaxActive,
		Items: []gitJobItem{}, Servers: []gitServerResult{}}
	var ids []interface{}
	for _, id := range j.order {
		it := j.items[id]
		v.Items = append(v.Items, *it)
		v.Total++
		if it.State == "done" || it.State == "error" || it.State == "cancelled" {
			v.Done++
		}
		if it.Seq > since && (it.State == "done" || it.State == "error") {
			ids = append(ids, it.InstallID)
		}
	}
	for _, s := range j.servers {
		if s.Name != "" || s.Error != "" {
			v.Servers = append(v.Servers, *s)
		}
	}
	j.mu.Unlock()
	sort.Slice(v.Servers, func(a, b int) bool { return v.Servers[a].Name < v.Servers[b].Name })
	if len(ids) > 0 {
		v.Installs = loadGitInstalls(j.UserID, false, "i.id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+")", ids...)
	}
	return v
}

func (j *gitCheckJob) set(id int, state, msg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	it := j.items[id]
	if it == nil {
		return
	}
	it.State, it.Error = state, truncateStr(msg, 300)
	if state != "running" {
		j.seq++
		it.Seq = j.seq
	}
}

func (j *gitCheckJob) add(in gitInstall) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.items[in.ID]; ok {
		return
	}
	j.items[in.ID] = &gitJobItem{InstallID: in.ID, ConnID: in.ConnID, State: "queued"}
	j.order = append(j.order, in.ID)
}

func (j *gitCheckJob) isCancelled() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.cancelled
}

// cancel skips the queued installations and aborts the running scans.
func (j *gitCheckJob) cancel() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.State != "running" {
		return false
	}
	j.cancelled = true
	for _, it := range j.items {
		if it.State == "queued" {
			it.State = "cancelled"
			j.seq++
			it.Seq = j.seq
		}
	}
	for cl := range j.clients {
		cl.Close()
	}
	return true
}

type gitJobRequest struct {
	InstallIDs   []int `json:"install_ids"`
	ConnIDs      []int `json:"conn_ids"`
	All          bool  `json:"all"`           // every server of the settings
	Discover     bool  `json:"discover"`      // look for new installations on whole servers
	Refresh      bool  `json:"refresh"`       // refresh the targets first
	StaleMinutes int   `json:"stale_minutes"` // only installations checked longer ago (0: no limit)
	OnlyNever    bool  `json:"only_never"`    // only installations never checked
}

// planGitJob resolves a request to servers and installations.
func planGitJob(userID int, in gitJobRequest) (map[int]*gitJobServer, []gitInstall, error) {
	plan := map[int]*gitJobServer{}
	var list []gitInstall
	stale := in.StaleMinutes > 0 || in.OnlyNever
	keep := func(i gitInstall) bool {
		if in.OnlyNever {
			return i.CheckedAt == ""
		}
		if in.StaleMinutes > 0 && i.CheckedAt != "" {
			t, err := time.Parse(time.RFC3339, i.CheckedAt)
			return err != nil || time.Since(t) >= time.Duration(in.StaleMinutes)*time.Minute
		}
		return true
	}
	conns := in.ConnIDs
	if in.All {
		_, st := loadGitWorkspace(userID)
		conns = append(append([]int(nil), conns...), st.ConnIDs...)
		if len(st.ConnIDs) == 0 && len(in.ConnIDs) == 0 {
			return nil, nil, fmt.Errorf("Choose the servers to check (Settings)")
		}
	}
	for _, id := range conns {
		if !userOwnsConnection(id, userID) {
			return nil, nil, fmt.Errorf("Connection not found")
		}
		if stale {
			// only the stale installations of these servers, no discovery
			for _, i := range loadGitInstalls(userID, false, "i.conn_id=?", id) {
				if keep(i) {
					list = append(list, i)
				}
			}
			continue
		}
		plan[id] = &gitJobServer{plan: in.Discover}
	}
	if len(in.InstallIDs) > 0 {
		all := map[int]gitInstall{}
		for _, i := range loadGitInstalls(userID, false, "") {
			all[i.ID] = i
		}
		for _, id := range in.InstallIDs {
			i, ok := all[id]
			if !ok {
				return nil, nil, fmt.Errorf("Installation not found")
			}
			if keep(i) {
				list = append(list, i)
			}
		}
	} else if stale && len(conns) == 0 {
		for _, i := range loadGitInstalls(userID, false, "") {
			if keep(i) {
				list = append(list, i)
			}
		}
	}
	for _, i := range list {
		s := plan[i.ConnID]
		if s == nil {
			s = &gitJobServer{installs: []int{}}
			plan[i.ConnID] = s
		}
		if s.installs != nil {
			s.installs = append(s.installs, i.ID)
		}
	}
	if len(plan) == 0 {
		return nil, nil, fmt.Errorf("Nothing to check")
	}
	return plan, list, nil
}

// startGitJob starts a check job; the caller holds the user's check lock, which the job
// releases when it ends.
func startGitJob(userID int, in gitJobRequest) (*gitCheckJob, error) {
	plan, list, err := planGitJob(userID, in)
	if err != nil {
		return nil, err
	}
	gitJobs.Lock()
	gitJobs.nextID++
	j := &gitCheckJob{ID: gitJobs.nextID, UserID: userID, State: "running", Started: nowStamp(), Discover: in.Discover, Refresh: in.Refresh,
		items: map[int]*gitJobItem{}, servers: map[int]*gitServerResult{}, clients: map[*ssh.Client]bool{}, plan: plan}
	gitJobs.last[userID] = j
	gitJobs.Unlock()
	for _, i := range list {
		j.add(i)
	}
	// whole servers: their known installations are queued now (discovery may add more)
	for id, s := range plan {
		if s.installs == nil {
			for _, i := range loadGitInstalls(userID, false, "i.conn_id=?", id) {
				j.add(i)
			}
		}
	}
	go j.run()
	return j, nil
}

func (j *gitCheckJob) run() {
	targets := map[string]gitTarget{}
	if j.Refresh && !j.isCancelled() {
		j.mu.Lock()
		j.Phase = "targets"
		j.mu.Unlock()
		apps := map[string]bool{}
		for _, i := range loadGitInstalls(j.UserID, false, "") {
			j.mu.Lock()
			_, ok := j.items[i.ID]
			j.mu.Unlock()
			if ok {
				apps[i.App] = true
			}
		}
		var names []string
		if !j.Discover {
			names = keys(apps)
		}
		if j.Discover || len(names) > 0 {
			ts, _ := refreshTargets(j.UserID, names)
			for _, t := range ts {
				if t.Error == "" {
					targets[t.App] = t
				}
			}
		}
	}
	j.mu.Lock()
	j.Phase = "checking"
	j.mu.Unlock()
	cat := loadGitCatalog(j.UserID)
	apps := effectiveApps(j.UserID, cat)
	overall := make(chan struct{}, settingInt("git_check_parallel"))
	var wg sync.WaitGroup
	for connID, s := range j.plan {
		wg.Add(1)
		go func(connID int, s *gitJobServer) {
			defer wg.Done()
			j.server(connID, s, cat, apps, targets, overall)
		}(connID, s)
	}
	wg.Wait()
	j.mu.Lock()
	checked := 0
	for _, it := range j.items {
		if it.State == "queued" || it.State == "running" {
			it.State = "cancelled"
			j.seq++
			it.Seq = j.seq
		}
		if it.State == "done" {
			checked++
		}
	}
	j.mu.Unlock()
	if checked > 0 {
		db.Exec(`INSERT OR IGNORE INTO git_workspace (user_id) VALUES (?)`, j.UserID)
		db.Exec(`UPDATE git_workspace SET last_check_at=? WHERE user_id=?`, nowStamp(), j.UserID)
	}
	// free the user's check slot before the job reads as ended, so a client that sees the
	// end can start the next check at once instead of getting "already running"
	gitUnlock(j.UserID)
	j.mu.Lock()
	j.State, j.Phase, j.Ended = "done", "", nowStamp()
	if j.cancelled {
		j.State = "cancelled"
	}
	j.mu.Unlock()
}

// server checks the job's installations on one server.
func (j *gitCheckJob) server(connID int, s *gitJobServer, cat *gitCatalog, apps []gitCatalogApp, targets map[string]gitTarget, overall chan struct{}) {
	sr := &gitServerResult{ConnID: connID}
	j.mu.Lock()
	j.servers[connID] = sr
	j.mu.Unlock()
	failAll := func(msg string) {
		saveGitServerHealthError(j.UserID, connID, msg)
		j.mu.Lock()
		sr.Error = msg
		var ids []int
		for _, it := range j.items {
			if it.ConnID == connID && it.State == "queued" {
				ids = append(ids, it.InstallID)
			}
		}
		j.mu.Unlock()
		for _, id := range ids {
			if list := loadGitInstalls(j.UserID, false, "i.id=?", id); len(list) > 0 {
				saveInstallError(list[0], msg)
			}
			j.set(id, "error", msg)
		}
	}
	c, err := loadConnection(connID)
	if err != nil || c.UserID != j.UserID || !isSSHProtocol(c) {
		failAll("not an SSH connection of yours")
		return
	}
	j.mu.Lock()
	sr.Name = c.Name
	j.mu.Unlock()
	if j.isCancelled() {
		return
	}
	overall <- struct{}{}
	cl, err := dialSSH(c, nil)
	<-overall
	if err != nil {
		failAll(err.Error())
		return
	}
	j.mu.Lock()
	if j.cancelled {
		j.mu.Unlock()
		cl.Close()
		return
	}
	j.clients[cl] = true
	j.mu.Unlock()
	defer func() {
		j.mu.Lock()
		delete(j.clients, cl)
		j.mu.Unlock()
		cl.Close()
	}()
	installs := s.installs
	if s.installs == nil {
		if s.plan {
			n, err := discoverAndRegister(j.UserID, connID, cl, cat, apps, nowStamp())
			j.mu.Lock()
			if err != nil {
				sr.Error = err.Error()
			} else {
				sr.Found = n
			}
			j.mu.Unlock()
			// installations no longer found were forgotten: drop them from the job
			j.mu.Lock()
			for id, it := range j.items {
				if it.ConnID == connID && it.State == "queued" {
					var n int
					if db.QueryRow(`SELECT COUNT(*) FROM git_installs WHERE id=?`, id).Scan(&n); n == 0 {
						it.State = "cancelled"
						j.seq++
						it.Seq = j.seq
					}
				}
			}
			j.mu.Unlock()
		}
		for _, i := range loadGitInstalls(j.UserID, false, "i.conn_id=?", connID) {
			j.add(i)
			installs = append(installs, i.ID)
		}
	}
	checkGitServerHealth(j.UserID, connID, cl)
	per := make(chan struct{}, settingInt("git_check_per_server"))
	var wg sync.WaitGroup
	for _, id := range installs {
		list := loadGitInstalls(j.UserID, true, "i.id=?", id)
		if len(list) == 0 {
			j.set(id, "cancelled", "")
			continue
		}
		in := list[0]
		wg.Add(1)
		go func() {
			defer wg.Done()
			per <- struct{}{}
			defer func() { <-per }()
			overall <- struct{}{}
			defer func() { <-overall }()
			if j.isCancelled() {
				j.set(in.ID, "cancelled", "")
				return
			}
			j.mu.Lock()
			j.active++
			if j.active > j.MaxActive {
				j.MaxActive = j.active
			}
			j.mu.Unlock()
			defer func() {
				j.mu.Lock()
				j.active--
				j.mu.Unlock()
			}()
			j.set(in.ID, "running", "")
			if gitJobTestHook != nil {
				gitJobTestHook(in.ID)
			}
			scans, err := scanOn(cl, []string{in.Path})
			if j.isCancelled() {
				j.set(in.ID, "cancelled", "")
				return
			}
			if err != nil {
				saveInstallError(in, err.Error())
				j.set(in.ID, "error", err.Error())
				return
			}
			t, ok := targets[in.App]
			if !ok {
				t, ok = loadGitTarget(j.UserID, in.App)
			}
			state, d := evalInstall(scans[in.Path], t, ok, cat.IgnoreDirs)
			app, _ := cat.app(in.App)
			checkGitFunctionality(cl, in, app, state, &d)
			if j.isCancelled() {
				j.set(in.ID, "cancelled", "")
				return
			}
			saveInstallResult(in.ID, state, d)
			j.set(in.ID, "done", "")
		}()
	}
	wg.Wait()
}

func currentGitJob(userID int) *gitCheckJob {
	gitJobs.Lock()
	defer gitJobs.Unlock()
	return gitJobs.last[userID]
}

// apiGitJobs serves the check jobs:
//
//	POST /api/git/check/jobs           {install_ids, conn_ids, all, discover, refresh, stale_minutes, only_never} → job
//	GET  /api/git/check/jobs/current?since=N → the newest job, with the installations finished after N
//	POST /api/git/check/jobs/current/cancel
func apiGitJobs(w http.ResponseWriter, r *http.Request, userID int, rest string) bool {
	if !strings.HasPrefix(rest, "check/jobs") {
		return false
	}
	if !gitChecksAllowed(userID) {
		jsonError(w, "Checks are not allowed for your account", 403)
		return true
	}
	switch {
	case rest == "check/jobs" && r.Method == http.MethodPost:
		var in gitJobRequest
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		if !gitLock(userID) {
			jsonError(w, "A check is already running", 409)
			return true
		}
		j, err := startGitJob(userID, in)
		if err != nil {
			gitUnlock(userID)
			code := 400
			if strings.Contains(err.Error(), "not found") {
				code = 404
			}
			jsonError(w, err.Error(), code)
			return true
		}
		auditLog(r, userID, "git.checked", "", map[string]interface{}{"job": j.ID, "installs": len(j.order), "servers": len(j.plan), "discover": in.Discover,
			"partial": !in.All, "stale_minutes": in.StaleMinutes, "only_never": in.OnlyNever})
		jsonOK(w, j.view(0))
	case rest == "check/jobs/current" && r.Method == http.MethodGet:
		j := currentGitJob(userID)
		if j == nil {
			jsonOK(w, map[string]interface{}{"job": nil})
			return true
		}
		since := atoiDefault(r.URL.Query().Get("since"), 0)
		jsonOK(w, map[string]interface{}{"job": j.view(since)})
	case rest == "check/jobs/current/cancel" && r.Method == http.MethodPost:
		j := currentGitJob(userID)
		if j == nil || !j.cancel() {
			jsonError(w, "No check is running", 400)
			return true
		}
		auditLog(r, userID, "git.check_cancelled", "", map[string]interface{}{"job": j.ID})
		jsonOK(w, map[string]interface{}{"job": j.view(0)})
	default:
		jsonError(w, "Not found", 404)
	}
	return true
}
