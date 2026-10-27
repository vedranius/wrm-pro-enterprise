// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"
)

// ─── GIT ENVIRONMENTS: deploy and rollback runs ──────
//
// Environment deploys and rollbacks are runs of kind update / upgrade / rollback with an
// "env" part: the same history, live progress, cancel, restart choice, audit and
// notifications as the other runs. A dry run uses the exact options of the real one, takes
// no lock, writes nothing and needs no typed confirmation; it only logs what would happen.

// gitEnvRun is the environment part of a run request.
type gitEnvRun struct {
	App           string         `json:"app"`
	Env           string         `json:"env"`
	PlanID        string         `json:"plan_id,omitempty"`
	Exclude       []string       `json:"exclude,omitempty"`        // deploy: files left out of this run
	DeleteRemoved bool           `json:"delete_removed,omitempty"` // deploy: delete files removed from the repository
	PostDeploy    bool           `json:"post_deploy,omitempty"`    // run the environment's post_deploy command
	PostCommand   string         `json:"post_command,omitempty"`   // pinned when the run is created
	DryRun        bool           `json:"dry_run,omitempty"`
	Confirm       string         `json:"confirm,omitempty"`       // the typed environment name (not stored)
	DeployID      string         `json:"deploy_id,omitempty"`     // deploy: assigned; rollback: the deploy to undo
	Dests         []int          `json:"dests,omitempty"`         // deploy/rollback: destinations (omitted = all; empty is refused)
	ConnBindings  map[int]string `json:"conn_bindings,omitempty"` // pinned by the server when creating a deploy
	Keep          int            `json:"keep,omitempty"`
}

// createEnvRun validates an environment deploy or rollback and starts it (never scheduled).
func createEnvRun(r *http.Request, userID int, p gitRunParams) (int, int, error) {
	e := p.Env
	e.App, e.Env = strings.TrimSpace(e.App), strings.TrimSpace(e.Env)
	app, env, err := findEnv(envCatalog(userID, e.App, e.Env), e.App, e.Env)
	if err != nil {
		return 0, 404, err
	}
	if p.ScheduleAt != "" {
		return 0, 400, fmt.Errorf("environment deploys run now (use a dry run to preview)")
	}
	if e.Dests != nil && len(e.Dests) == 0 {
		return 0, 400, fmt.Errorf("select at least one destination")
	}
	switch p.Restart.Mode {
	case "", "none":
		p.Restart = gitRestartChoice{Mode: "none"}
	case "now":
		if !gitActionAllowed(userID, "git_restart") {
			return 0, 403, fmt.Errorf("restarts are not allowed for your account")
		}
	default:
		return 0, 400, fmt.Errorf("environment deploys restart now or not at all")
	}
	checks := map[string]string{}
	if c := strings.TrimSpace(p.Checks[app.Name]); c != "" {
		if len(c) > 500 || strings.ContainsAny(c, "\n\r") {
			return 0, 400, fmt.Errorf("the check command must be one line of at most 500 characters")
		}
		checks[app.Name] = c
	}
	p.Checks = checks
	if env.needsConfirm() && !e.DryRun && !strings.EqualFold(strings.TrimSpace(e.Confirm), env.Name) {
		return 0, 400, fmt.Errorf("type the environment name %q to confirm", env.Name)
	}
	e.Confirm = ""
	e.ConnBindings = nil
	e.PostCommand = ""
	if e.PostDeploy {
		if env.PostDeploy == "" {
			return 0, 400, fmt.Errorf("%s has no post_deploy command", env.Name)
		}
		e.PostCommand = env.PostDeploy
	}
	e.Keep = env.keep()
	now := time.Now()
	label := gitRunLabel(now)
	var plan *gitEnvPlan
	p.Items = nil
	switch p.Kind {
	case "update", "upgrade":
		if plan, err = takeEnvPlan(userID, e.PlanID, false); err != nil {
			return 0, 409, err
		}
		if plan.App != app.Name || plan.Env != env.Name {
			return 0, 400, fmt.Errorf("the plan belongs to another environment")
		}
		p.Kind = "update"
		if plan.Ref != "" {
			p.Kind = "upgrade"
		}
		if !gitActionAllowed(userID, gitKindPolicy[p.Kind]) {
			return 0, 403, fmt.Errorf("this action is not allowed for your account")
		}
		if len(plan.Errors) > 0 {
			return 0, 400, errors.New(strings.Join(plan.Errors, "; "))
		}
		if !refAllowed(plan.target, env.AllowedRefs) {
			return 0, 400, fmt.Errorf("%s is not allowed in %s", refName(plan.target), env.Name)
		}
		excl := map[string]bool{}
		var list []string
		for _, x := range e.Exclude {
			if x = cleanRel(x); x != "" && !excl[x] {
				excl[x] = true
				list = append(list, x)
			}
		}
		sort.Strings(list)
		e.Exclude = list
		if len(plan.Dests) != len(env.Destinations) {
			return 0, 409, fmt.Errorf("the environment changed since the plan: compare again")
		}
		byIndex := make(map[int]*gitEnvDestPlan, len(plan.Dests))
		for _, dp := range plan.Dests {
			if dp.Index < 0 || dp.Index >= len(env.Destinations) || byIndex[dp.Index] != nil {
				return 0, 409, fmt.Errorf("the environment changed since the plan: compare again")
			}
			if d := env.Destinations[dp.Index]; d.Path != dp.Path || !strings.EqualFold(d.Server, dp.Server) {
				return 0, 409, fmt.Errorf("the environment changed since the plan: compare again")
			}
			byIndex[dp.Index] = dp
		}
		selected := append([]int(nil), e.Dests...)
		if len(selected) == 0 {
			for i := range env.Destinations {
				selected = append(selected, i)
			}
		}
		sort.Ints(selected) // retain the configured destination order
		seen := make(map[int]bool, len(selected))
		e.ConnBindings = make(map[int]string, len(selected))
		for _, i := range selected {
			if i < 0 || i >= len(env.Destinations) || seen[i] {
				return 0, 400, fmt.Errorf("unknown or duplicate destination")
			}
			seen[i] = true
			dp := byIndex[i]
			if dp == nil {
				return 0, 409, fmt.Errorf("the environment changed since the plan: compare again")
			}
			if len(dp.Errors) > 0 {
				return 0, 400, fmt.Errorf("%s: %s", dp.Server, strings.Join(dp.Errors, "; "))
			}
			conn, err := envConn(userID, dp.Server)
			if err != nil || conn.ID != dp.ConnID || gitEnvConnBinding(conn) != dp.connBinding {
				return 0, 409, fmt.Errorf("the connection changed since the plan: compare again")
			}
			e.ConnBindings[i] = dp.connBinding
			if _, _, err := envSelection(dp, excl, e.DeleteRemoved); err != nil {
				return 0, 400, err
			}
			p.Items = append(p.Items, envItem(userID, app.Name, env.Name, dp.Index, dp.ConnID, dp.Server, dp.Path, dp.StateDir, dp.Fingerprint))
		}
		e.Dests = selected
		p.Targets = map[string]gitTarget{app.Name: plan.target}
		p.Ref = plan.Ref
		e.DeployID = gitBackupName(bundleOf(plan.target, label)+"-"+randomID(2), now) // unique when two deploys start in one second
	case "rollback":
		if !gitActionAllowed(userID, "git_rollback") {
			return 0, 403, fmt.Errorf("rollbacks are not allowed for your account")
		}
		e.DeployID = gitBackupNameRe.Replace(strings.TrimSpace(e.DeployID))
		if e.DeployID == "" || strings.HasPrefix(e.DeployID, ".") || len(e.DeployID) > 200 {
			return 0, 400, fmt.Errorf("choose the deploy to roll back")
		}
		dests := e.Dests
		if len(dests) == 0 {
			for i := range env.Destinations {
				dests = append(dests, i)
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(dests))) // reverse destination order
		seen := map[int]bool{}
		for _, i := range dests {
			if i < 0 || i >= len(env.Destinations) || seen[i] {
				return 0, 400, fmt.Errorf("unknown destination")
			}
			seen[i] = true
			d := env.Destinations[i]
			conn, err := envConn(userID, d.Server)
			if err != nil {
				return 0, 400, err
			}
			p.Items = append(p.Items, envItem(userID, app.Name, env.Name, i, conn.ID, d.Server, d.Path, envStateDir(*env, app.Name, d.Path), ""))
		}
		e.Dests = dests
		e.PlanID, e.Exclude, e.DeleteRemoved = "", nil, false
	default:
		return 0, 400, fmt.Errorf("unknown kind")
	}
	gitLive.Lock()
	busy := gitLive.busy[userID] != 0
	gitLive.Unlock()
	if busy {
		return 0, 409, fmt.Errorf("another run is working: wait until it ends")
	}
	if plan != nil && !e.DryRun {
		if _, err := takeEnvPlan(userID, e.PlanID, true); err != nil { // one deploy per reviewed plan
			return 0, 409, err
		}
		saveEnvExclusions(userID, app.Name, env.Name, e.Exclude)
	}
	p.Confirm, p.ScheduleAt, p.Provision = nil, "", nil
	res, err := db.Exec(`INSERT INTO git_runs (user_id, kind, state, label, parent_id, params, result, message, created_at, scheduled_at, started_at, ended_at, notified_at)
		VALUES (?,?,'starting',?,0,?,'','',?,'','','','')`, userID, p.Kind, label, string(jsonMarshal(p)), nowStamp())
	if err != nil {
		return 0, 500, err
	}
	id64, _ := res.LastInsertId()
	id := int(id64)
	pruneGitRuns(userID)
	if err := startGitRun(userID, id, r); err != nil {
		db.Exec(`UPDATE git_runs SET state='failed', message=?, ended_at=? WHERE id=?`, err.Error(), nowStamp(), id)
		return 0, 409, err
	}
	return id, 200, nil
}

func envItem(userID int, app, env string, idx, connID int, server, p, stateDir, fp string) *gitRunItem {
	it := &gitRunItem{ConnID: connID, ConnName: server, App: app, Path: p, Env: env, Dest: idx, StateDir: stateDir, Fingerprint: fp, State: "pending", Log: []gitStepLog{}}
	if list := loadGitInstalls(userID, false, "i.conn_id=? AND i.path=?", connID, p); len(list) == 1 {
		it.InstallID, it.Units, it.Prod = list[0].ID, list[0].Units, isProdInstall(list[0])
	}
	return it
}

func lockMsg(l *gitEnvLock) string {
	msg := fmt.Sprintf("locked by %s from %s (deploy %s", l.User, l.From, l.ID)
	if l.Since != "" {
		msg += ", since " + l.Since + " server time"
	}
	msg += ")"
	if l.Stale {
		msg += ": the lock is stale; remove it in the plan after checking that no deploy is running"
	}
	return msg
}

// envProbe reads the host, the SSH user, the owner of the app directory (or its nearest
// existing parent) and the tools for the checks.
func envProbe(s *deploySession) error {
	script := fmt.Sprintf(`D=%s
echo "H	$(hostname 2>/dev/null || uname -n)"
echo "U	$(id -un 2>/dev/null || whoami)"
P=$D; while [ ! -d "$P" ]; do P=${P%%/*}; [ -n "$P" ] || P=/; done
echo "O	$(ls -ldn "$P" | awk '{print $3":"$4}')"
for x in python3 node bash; do command -v $x >/dev/null 2>&1 && echo "T	$x"; done
echo WRM_DONE
`, shellQuote(s.in.Path))
	out, err := s.run(script, "", gitStepTimeout)
	if err != nil {
		return err
	}
	s.tools, s.crlf = map[string]bool{}, map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		switch k {
		case "H":
			s.host = strings.TrimSpace(v)
		case "U":
			s.user = strings.TrimSpace(v)
		case "O":
			s.owner = strings.TrimSpace(v)
		case "T":
			s.tools[v] = true
		}
	}
	if !strings.Contains(out, "WRM_DONE") {
		return fmt.Errorf("the server did not answer completely")
	}
	if s.host == "" {
		s.host = s.in.ConnName
	}
	return nil
}

// envBackup copies the files a deploy replaces or deletes (a list on stdin) into a new
// backup directory; symlinks are kept as symlinks.
func envBackup(s *deploySession, backup string, rels []string) error {
	script := fmt.Sprintf(`B=%s; D=%s
mkdir -p -- "${B%%/*}" || exit 3
mkdir "$B" 2>/dev/null || { printf 'E\tthe backup %%s exists already\n' "$B"; exit 0; }
cd "$D" || exit 3
while IFS= read -r rel; do
  [ -n "$rel" ] || continue
  d=${rel%%/*}; [ "$d" = "$rel" ] || mkdir -p -- "$B/$d" || { printf 'F\t%%s\n' "$rel"; exit 3; }
  if [ -L "$rel" ]; then ln -s -- "$(readlink -- "$rel")" "$B/$rel" || { printf 'F\t%%s\n' "$rel"; exit 3; }
  elif [ -f "$rel" ]; then cp -p -- "$rel" "$B/$rel" || { printf 'F\t%%s\n' "$rel"; exit 3; }; fi
done
echo WRM_OK
`, shellQuote(backup), shellQuote(s.in.Path))
	out, err := s.run(script, strings.Join(rels, "\n")+"\n", gitTransferTimeout)
	if e := scriptError(out); e != nil {
		return e
	}
	if err == nil && !strings.Contains(out, "WRM_OK") {
		err = fmt.Errorf("the backup failed")
	}
	if err != nil {
		s.run("rm -rf "+shellQuote(backup), "", time.Minute)
	}
	return err
}

// gitEnvDir is a directory a deploy removed (re-created by a rollback with its mode and owner).
type gitEnvDir struct {
	Path  string `json:"path"`
	Mode  string `json:"mode,omitempty"`
	Owner string `json:"owner,omitempty"`
}

// envCommit deletes, then moves the staged files into place (keeping mode and owner of the
// files they replace). Lines on stdin: X<TAB>rel (delete), W<TAB>rel (write). It returns
// the directories the deletes left empty and removed.
func envCommit(s *deploySession, staging string, writes, deletes []string) ([]gitEnvDir, error) {
	script := fmt.Sprintf(`D=%s; T=%s
cd "$D" || exit 3
mo() { m=$(stat -c %%a -- "$1" 2>/dev/null || stat -f %%Lp -- "$1" 2>/dev/null); o=$(stat -c %%u:%%g -- "$1" 2>/dev/null || stat -f %%u:%%g -- "$1" 2>/dev/null); printf '%%s\t%%s' "$m" "$o"; }
keep() { { [ -f "$1" ] && [ ! -L "$1" ]; } || return 0; m=$(stat -c %%a -- "$1" 2>/dev/null || stat -f %%Lp -- "$1" 2>/dev/null); o=$(stat -c %%u:%%g -- "$1" 2>/dev/null || stat -f %%u:%%g -- "$1" 2>/dev/null)
  [ -z "$m" ] || chmod -- "$m" "$2"; [ -z "$o" ] || chown -- "$o" "$2" 2>/dev/null; return 0; }
while IFS= read -r line; do
  k=${line%%%%	*}; rel=${line#*	}
  case $k in
  X) if [ -L "$rel" ] || [ -f "$rel" ]; then rm -f -- "$rel" || { printf 'F\t%%s\n' "$rel"; exit 3; }; fi
     d=$rel; while :; do case $d in */*) d=${d%%/*};; *) break;; esac; [ -L "$d" ] && break; x=$(mo "$d"); rmdir -- "$d" 2>/dev/null || break; printf 'R\t%%s\t%%s\n' "$d" "$x"; done ;;
  W) if [ -d "$rel" ] && [ ! -L "$rel" ]; then printf 'F\t%%s\tis a directory\n' "$rel"; exit 3; fi
     d=${rel%%/*}; if [ "$d" != "$rel" ] && [ ! -d "$d" ]; then m=$d; while :; do case $m in */*) [ -d "${m%%/*}" ] && break; m=${m%%/*};; *) break;; esac; done
       mkdir -p -- "$d" || { printf 'F\t%%s\n' "$rel"; exit 3; }; printf 'M\t%%s\n' "$D/$m"; fi
     keep "$rel" "$T/$rel"; mv -f -- "$T/$rel" "$rel" || { printf 'F\t%%s\n' "$rel"; exit 3; } ;;
  esac
done
echo WRM_OK
`, shellQuote(s.in.Path), shellQuote(staging))
	var in strings.Builder
	for _, d := range deletes {
		in.WriteString("X\t" + d + "\n")
	}
	for _, w := range writes {
		in.WriteString("W\t" + w + "\n")
	}
	out, err := s.run(script, in.String(), gitTransferTimeout)
	var removed []gitEnvDir
	var failed []string
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "\t")
		switch {
		case parts[0] == "M" && len(parts) >= 2:
			s.created = append(s.created, strings.TrimPrefix(line, "M\t"))
		case parts[0] == "R" && len(parts) == 4:
			removed = append(removed, gitEnvDir{Path: parts[1], Mode: parts[2], Owner: parts[3]})
		case parts[0] == "F" && len(parts) >= 2:
			failed = append(failed, strings.Join(parts[1:], ": "))
		}
	}
	if len(failed) > 0 {
		return removed, fmt.Errorf("could not write %s", strings.Join(failed, ", "))
	}
	if err == nil && !strings.Contains(out, "WRM_OK") {
		err = fmt.Errorf("the renames did not finish")
	}
	return removed, err
}

// envRestore undoes a failed deploy: files that existed come back from the backup, new
// files go, directories the deploy created are removed when empty. With drop the backup
// is removed after a complete restore.
func envRestore(s *deploySession, staging string, existed, added []string, drop bool) error {
	script := fmt.Sprintf(`B=%s; D=%s; bad=0
cd "$D" || exit 3
while IFS= read -r line; do
  k=${line%%%%	*}; rel=${line#*	}
  case $k in
  E) d=${rel%%/*}; [ "$d" = "$rel" ] || mkdir -p -- "$d"
     if [ -L "$B/$rel" ]; then { ln -s -- "$(readlink -- "$B/$rel")" "$rel.wrm-back" && mv -f -- "$rel.wrm-back" "$rel"; } || { printf 'F\t%%s\n' "$rel"; bad=1; }
     elif [ -f "$B/$rel" ]; then { cp -p -- "$B/$rel" "$rel.wrm-back" && mv -f -- "$rel.wrm-back" "$rel"; } || { printf 'F\t%%s\n' "$rel"; bad=1; }
     else printf 'F\t%%s\n' "$rel"; bad=1; fi ;;
  N) rm -f -- "$rel" ;;
  C) find "$rel" -depth -type d -exec rmdir {} \; 2>/dev/null ;;
  esac
done
rm -rf %s
`, shellQuote(s.backup), shellQuote(s.in.Path), shellQuote(staging))
	if drop {
		script += "[ $bad = 0 ] && rm -rf \"$B\"\n"
	}
	script += "echo WRM_OK\n"
	var in strings.Builder
	for _, rel := range existed {
		in.WriteString("E\t" + rel + "\n")
	}
	for _, rel := range added {
		in.WriteString("N\t" + rel + "\n")
	}
	for i := len(s.created) - 1; i >= 0; i-- {
		in.WriteString("C\t" + s.created[i] + "\n")
	}
	out, err := s.run(script, in.String(), gitTransferTimeout)
	if err != nil {
		return err
	}
	var failed []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "F\t") {
			failed = append(failed, line[2:])
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("could not restore %s (the backup stays in %s)", strings.Join(capList(failed, 10), ", "), s.backup)
	}
	return nil
}

// envChecks runs the language checks of the written files in chunks (short scripts) and
// the custom command with the last chunk.
func envChecks(s *deploySession, rels []string, custom string) ([]string, error) {
	var check []string
	for _, rel := range rels {
		switch strings.ToLower(path.Ext(rel)) {
		case ".py", ".js", ".mjs", ".cjs", ".sh":
			check = append(check, rel)
		}
	}
	var passed []string
	for i := 0; i == 0 || i < len(check); i += 200 {
		end := min(i+200, len(check))
		cmd := ""
		if end >= len(check) {
			cmd = custom
		}
		p, err := s.checks(check[i:end], cmd)
		passed = append(passed, p...)
		if err != nil {
			return passed, err
		}
	}
	return passed, nil
}

// envPrune keeps the newest keep deploy backups (by the time in the deploy id) and the
// newest rollback backups; the current deploy is never removed.
func envPrune(s *deploySession, stateDir, current string, keep int) []string {
	out, err := s.run(fmt.Sprintf("ls -1 %s/deploys 2>/dev/null\necho WRM_DONE\n", shellQuote(stateDir)), "", time.Minute)
	if err != nil {
		return nil
	}
	var ids []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasSuffix(l, ".json") && !strings.HasPrefix(l, ".") {
			ids = append(ids, strings.TrimSuffix(l, ".json"))
		}
	}
	stamp := func(id string) string {
		if m := backupTimeRe.FindStringSubmatch(id); m != nil {
			return m[1] + m[2]
		}
		return ""
	}
	sort.SliceStable(ids, func(i, j int) bool {
		if a, b := stamp(ids[i]), stamp(ids[j]); a != b {
			return a > b
		}
		return ids[i] > ids[j]
	})
	var drop []string
	kept := 0
	if current != "" {
		kept = 1 // the current deploy counts first
	}
	for _, id := range ids {
		if id == current {
			continue
		}
		if kept < keep {
			kept++
			continue
		}
		drop = append(drop, id)
	}
	script := fmt.Sprintf(`SD=%s
while IFS= read -r id; do [ -n "$id" ] && rm -rf "$SD/$id" "$SD/deploys/$id.json"; done
if [ -d "$SD/backups" ]; then cd "$SD/backups" && ls -1td rollback-* 2>/dev/null | tail -n +%d | while IFS= read -r d; do rm -rf "$d"; done; fi
echo WRM_OK
`, shellQuote(stateDir), gitEnvKeepRollback+1)
	s.run(script, strings.Join(drop, "\n")+"\n", gitStepTimeout)
	return drop
}

// envRegister adds the destination to the installations (when it is not known yet),
// compares it and reads its units.
func (c itemCtx) envRegister(s *deploySession) {
	if c.it.InstallID == 0 {
		db.Exec(`INSERT INTO git_installs (user_id, conn_id, app, path, env, manual, state, data, checked_at, discovered_at) VALUES (?,?,?,?,?,1,'unknown','','',?)
			ON CONFLICT(user_id, conn_id, path) DO UPDATE SET app=excluded.app, env=excluded.env, manual=1`,
			c.run.UserID, s.in.ConnID, s.in.App, s.in.Path, truncateStr(s.in.Env, 40), nowStamp())
		db.QueryRow(`SELECT id FROM git_installs WHERE user_id=? AND conn_id=? AND path=?`, c.run.UserID, s.in.ConnID, s.in.Path).Scan(&s.in.ID)
		c.run.set(func() { c.it.InstallID = s.in.ID })
	}
	refreshInstallState(c.run.UserID, s.in, s.cl)
	if list := loadGitInstalls(c.run.UserID, false, "i.id=?", s.in.ID); len(list) == 1 {
		s.in.Units = list[0].Units
		c.run.set(func() { c.it.Units = list[0].Units })
	}
	if len(s.in.Units) > 0 {
		db.Exec(`INSERT INTO git_pending_restarts (install_id, user_id, since, run_id) VALUES (?,?,?,?)
			ON CONFLICT(install_id) DO UPDATE SET since=excluded.since, run_id=excluded.run_id`, s.in.ID, c.run.UserID, nowStamp(), c.run.ID)
	}
}

// postDeploy runs the environment's post_deploy command in the app directory.
func (c itemCtx) postDeploy(s *deploySession, cmd string) error {
	c.step("post_deploy")
	out, err := s.run(fmt.Sprintf("cd %s || exit 3\nsh -c %s 2>&1\n", shellQuote(s.in.Path), shellQuote(cmd)), "", gitPostDeployTime)
	tail := strings.TrimSpace(out)
	if len(tail) > 1500 {
		tail = "…" + tail[len(tail)-1500:]
	}
	if err != nil {
		c.log("post_deploy", "fail", cmd+"\n"+err.Error())
		return err
	}
	c.log("post_deploy", "ok", strings.TrimSpace(cmd+"\n"+tail))
	return nil
}

// ─── deploy ──────────────────────────────────────────

func (run *gitRun) envDeployItem(it *gitRunItem) error {
	c := itemCtx{run, it}
	e := run.P.Env
	t, ok := run.P.Targets[it.App]
	if !ok {
		return fmt.Errorf("no target for %s", it.App)
	}
	cat := envCatalog(run.UserID, e.App, e.Env)
	app, env, err := findEnv(cat, e.App, e.Env)
	if err != nil {
		return err
	}
	if it.Dest >= len(env.Destinations) || env.Destinations[it.Dest].Path != it.Path || envStateDir(*env, app.Name, it.Path) != it.StateDir {
		return fmt.Errorf("the environment changed since the plan: compare again")
	}
	conn, cl, err := c.dial(it.ConnID, "connect")
	if err != nil {
		return err
	}
	defer cl.Close()
	if binding := e.ConnBindings[it.Dest]; binding != "" && gitEnvConnBinding(conn) != binding {
		return fmt.Errorf("the connection changed since the plan: compare again")
	}
	run.set(func() { it.DeployID = e.DeployID })
	if e.DryRun {
		c.log("lock", "skip", "dry run: no lock is taken and nothing is written")
	} else {
		c.step("lock")
		holder, err := envLock(cl, it.StateDir, e.DeployID, usernameOf(run.UserID))
		if err != nil {
			c.log("lock", "fail", err.Error())
			return err
		}
		if holder != nil {
			c.log("lock", "fail", lockMsg(holder))
			return errors.New(lockMsg(holder))
		}
		c.log("lock", "ok", it.StateDir+"/.lock")
		defer func() {
			switch released, err := envUnlock(cl, it.StateDir, e.DeployID); {
			case err != nil:
				c.log("unlock", "fail", err.Error())
			case released:
				c.log("unlock", "ok", "")
			default:
				c.log("unlock", "info", "the lock belongs to another deploy now: left alone")
			}
		}()
	}
	c.step("plan")
	x := newEnvPlanCtx(run.UserID, cat, app, env, t)
	dp, err := x.destPlan(cl, it.Dest, env.Destinations[it.Dest], conn)
	if err == nil && len(dp.Errors) > 0 {
		err = errors.New(strings.Join(dp.Errors, "; "))
	}
	if err != nil {
		c.log("plan", "fail", err.Error())
		return err
	}
	if dp.Fingerprint != it.Fingerprint {
		msg := "the server or the repository changed since the plan was reviewed: compare again"
		c.log("plan", "fail", msg)
		return errors.New(msg)
	}
	exclude := map[string]bool{}
	for _, x := range e.Exclude {
		exclude[x] = true
	}
	writes, deletes, err := envSelection(dp, exclude, e.DeleteRemoved)
	if err != nil {
		c.log("plan", "fail", err.Error())
		return err
	}
	byPath := map[string]gitEnvFile{}
	var excluded []string
	for _, f := range dp.Files {
		byPath[f.Path] = f
		if exclude[f.Path] && f.State != "same" && f.State != "extra" && f.State != "protected" && f.State != "ignored" {
			excluded = append(excluded, f.Path)
		}
	}
	var added, changed []string
	eol := 0
	for _, w := range writes {
		f := byPath[w]
		if f.State == "new" {
			added = append(added, w)
		} else {
			changed = append(changed, w)
		}
		if f.State == "eol" {
			eol++
		}
	}
	from := dp.FromVersion
	if from == "" {
		from = "-"
	}
	run.set(func() { it.FromVersion, it.ToVersion = from, t.Version })
	c.log("plan", "ok", fmt.Sprintf("the reviewed plan still holds: %d new, %d changed (%d CRLF/LF only), %d to delete, %d excluded", len(added), len(changed), eol, len(deletes), len(excluded)))
	backup := it.StateDir + "/" + e.DeployID
	if e.DryRun {
		dry := func(what string, list []string) {
			if len(list) > 0 {
				c.log("dry_run", "info", fmt.Sprintf("%s (%d): %s", what, len(list), strings.Join(capList(list, 50), ", ")))
			}
		}
		dry("would add", added)
		dry("would change", changed)
		dry("would delete", deletes)
		dry("excluded", excluded)
		c.log("dry_run", "info", "would back up the replaced files to "+backup+" and write VERSION.md, state.json and history.jsonl")
		if e.PostCommand != "" {
			c.log("dry_run", "info", "would run post_deploy: "+e.PostCommand)
		} else {
			c.log("dry_run", "info", "post_deploy: not chosen")
		}
		if run.P.Restart.Mode == "now" && len(it.Units) > 0 {
			c.log("dry_run", "info", "would restart: "+unitNames(it.Units))
		}
		run.set(func() { it.State, it.Step = "ok", "" })
		return nil
	}
	s := &deploySession{cl: cl, in: gitInstall{ID: it.InstallID, ConnID: it.ConnID, ConnName: it.ConnName, App: it.App, Path: it.Path, Env: env.Name, Units: it.Units}}
	c.step("prepare")
	if err := envProbe(s); err != nil {
		c.log("prepare", "fail", err.Error())
		return err
	}
	now := time.Now()
	// VERSION.md travels with the files (staged, verified, backed up and restored like them)
	rows := envVersionPlan(dp, exclude)
	written := map[string]bool{}
	for _, w := range writes {
		written[w] = true
	}
	user := usernameOf(run.UserID)
	vmd := versionMD(t, versionRows(t, rows, written), now, s.host, s.user, bundleOf(t, run.Label), dp.vmdCR)
	c.step("fetch")
	files := map[string][]byte{"VERSION.md": vmd}
	modes := map[string]int64{"VERSION.md": 0o644}
	want := map[string]string{"VERSION.md": trHash(vmd)}
	for _, rel := range writes {
		data, err := gitFileContent(run.UserID, t, rel, x.providers)
		if err != nil {
			c.log("fetch", "fail", err.Error())
			return err
		}
		files[rel], modes[rel], want[rel] = data, fileMode(data), trHash(data)
	}
	allWrites := append(append([]string{}, writes...), "VERSION.md")
	tarb, err := tarOf(files, modes)
	if err != nil {
		return err
	}
	staging := it.Path + "/.wrm-incoming-" + e.DeployID
	c.step("transfer")
	c.log("transfer", "info", fmt.Sprintf("%d files (%d KB) as one stream into %s", len(files), len(tarb)/1024+1, staging))
	script := strings.Replace(receiveScript(it.Path, staging, s.owner), "cd \"$T\" || exit 3\n", gitEnvReceiveHook+"cd \"$T\" || exit 3\n", 1)
	out, terr := runRemoteScript(cl, script, string(tarb), gitTransferTimeout)
	got, err := s.parseReceive(out, terr)
	if err == nil {
		err = verifyStaged(got, want)
	}
	if err != nil {
		return c.envPartial(s, it, e, staging, len(files), err)
	}
	c.log("transfer", "ok", fmt.Sprintf("%d files arrived and match", len(got)))
	c.step("backup")
	var existed []string
	for _, w := range allWrites {
		if _, on := byPath[w]; on && byPath[w].State != "new" {
			existed = append(existed, w)
		}
	}
	if dp.vmdExists {
		existed = append(existed, "VERSION.md")
	}
	existed = append(existed, deletes...)
	s.backup = backup
	if err := envBackup(s, backup, existed); err != nil {
		c.log("backup", "fail", err.Error())
		s.dropStaging(staging)
		return err
	}
	run.set(func() { it.MadeBackup = backup })
	c.log("backup", "ok", fmt.Sprintf("%s (%d files)", backup, len(existed)))
	var newFiles []string
	for _, w := range allWrites {
		if w == "VERSION.md" && dp.vmdExists {
			continue
		}
		if f, on := byPath[w]; !on || f.State == "new" {
			newFiles = append(newFiles, w)
		}
	}
	rollback := func(cause error) error {
		c.step("rollback")
		if rerr := envRestore(s, staging, existed, newFiles, true); rerr != nil {
			c.log("rollback", "fail", rerr.Error())
			return fmt.Errorf("%v; THE AUTOMATIC ROLLBACK FAILED: %v", cause, rerr)
		}
		c.log("rollback", "ok", "the previous files are back")
		run.set(func() { it.State, it.MadeBackup = "rolled_back", "" })
		return fmt.Errorf("%v (rolled back)", cause)
	}
	c.step("write")
	removedDirs, err := envCommit(s, staging, allWrites, deletes)
	if err != nil {
		c.log("write", "fail", err.Error())
		return rollback(err)
	}
	s.run("rm -rf "+shellQuote(staging), "", time.Minute)
	c.log("write", "ok", fmt.Sprintf("%d written, %d deleted", len(writes), len(deletes)))
	c.step("checks")
	passed, err := envChecks(s, writes, run.P.Checks[app.Name])
	if err != nil {
		c.log("checks", "fail", err.Error())
		return rollback(fmt.Errorf("check failed: %v", err))
	}
	if len(passed) > 0 {
		c.log("checks", "ok", fmt.Sprintf("%d passed", len(passed)))
	} else {
		c.log("checks", "skip", "no checks for these files")
	}
	c.step("state")
	rec := &gitEnvRecord{DeployID: e.DeployID, App: app.Name, Env: env.Name, Version: t.Version, Commit: t.CommitFull, Branch: t.Branch, Ref: refName(t),
		By: user, At: now.UTC().Format(time.RFC3339), Host: s.host, Source: t.Source}
	if rec.Commit == "" {
		rec.Commit = t.Commit
	}
	manifest := map[string]interface{}{"id": e.DeployID, "app": app.Name, "env": env.Name, "backup": backup, "at": rec.At, "by": user,
		"added": nonNil(added), "changed": nonNil(changed), "deleted": nonNil(deletes), "removed_dirs": removedDirs, "version_md_existed": dp.vmdExists, "previous": dp.Current}
	if removedDirs == nil {
		manifest["removed_dirs"] = []gitEnvDir{}
	}
	err = envWriteFile(cl, it.StateDir+"/deploys/"+e.DeployID+".json", jsonMarshal(manifest))
	if err == nil {
		err = envWriteFile(cl, it.StateDir+"/state.json", jsonMarshal(gitEnvState{Current: rec, Previous: dp.Current}))
	}
	if err != nil {
		c.log("state", "fail", err.Error())
		s.run("rm -f "+shellQuote(it.StateDir+"/deploys/"+e.DeployID+".json"), "", time.Minute)
		return rollback(fmt.Errorf("state: %v", err))
	}
	c.log("state", "ok", it.StateDir+"/state.json")
	run.set(func() { it.Written, it.Deleted = writes, deletes })
	c.updateLog(s, map[string]interface{}{"app": app.Name, "backup": backup, "branch": t.Branch, "bundle": bundleOf(t, run.Label), "env": env.Name,
		"files": append(append([]string{}, writes...), deletes...), "from_version": from, "host": s.host, "install": it.Path, "services": serviceNames(it.Units),
		"time": now.Format("2006-01-02T15:04:05"), "to_version": fmt.Sprintf("%s (%s)", short10(t.Commit), t.CommitDate), "user": s.user})
	var postErr error
	post := ""
	if e.PostCommand != "" {
		if postErr = c.postDeploy(s, e.PostCommand); postErr != nil {
			post = "failed: " + truncateStr(postErr.Error(), 300)
		} else {
			post = "ok"
		}
	}
	c.step("history")
	entry := map[string]interface{}{"action": "DEPLOY", "id": e.DeployID, "app": app.Name, "env": env.Name, "version": t.Version, "commit": rec.Commit,
		"branch": t.Branch, "ref": rec.Ref, "by": user, "at": rec.At, "host": s.host, "server": it.ConnName, "path": it.Path, "previous": dp.Current,
		"new": listCap(added), "changed": listCap(changed), "deleted": listCap(deletes), "excluded": listCap(excluded),
		"counts": map[string]int{"new": len(added), "changed": len(changed), "deleted": len(deletes), "excluded": len(excluded), "eol": eol},
		"backup": backup, "result": "ok", "run": run.Label}
	if post != "" {
		entry["post_deploy"] = post
	}
	if postErr != nil {
		entry["result"] = "post_deploy_failed"
		entry["note"] = "post_deploy failed; the files are already on the server"
	}
	if err := envAppendHistory(cl, it.StateDir, entry); err != nil {
		c.log("history", "fail", err.Error())
	} else {
		c.log("history", "ok", it.StateDir+"/history.jsonl")
	}
	if dropped := envPrune(s, it.StateDir, e.DeployID, e.Keep); len(dropped) > 0 {
		c.log("history", "info", fmt.Sprintf("older backups removed (keeping %d): %s", e.Keep, strings.Join(dropped, ", ")))
	}
	c.step("register")
	c.envRegister(s)
	run.set(func() { it.PostDeploy = post })
	if postErr != nil {
		note := fmt.Sprintf("post_deploy failed: the files are already on the server (deploy %s); the remaining destinations were not started", e.DeployID)
		run.set(func() { it.Note, it.State, it.Step = note, "failed", "" })
		return fmt.Errorf("post_deploy: %v (%s)", postErr, note)
	}
	run.set(func() { it.State, it.Step = "ok", "" })
	return nil
}

// envVersionPlan describes the target's files for VERSION.md after the deploy.
func envVersionPlan(dp *gitEnvDestPlan, exclude map[string]bool) []planFile {
	var out []planFile
	for _, f := range dp.Files {
		p := planFile{Path: f.Path, State: "ok"}
		switch {
		case f.State == "protected":
			p.State = "protected"
		case f.State == "same" || f.State == "eol" || !exclude[f.Path]:
		case f.State == "new":
			p.State = "missing"
		case f.Behind > 0:
			p.State, p.Behind = "old", f.Behind
		default:
			p.State = "modified"
		}
		out = append(out, p)
	}
	return out
}

// envPartial records an interrupted transfer: what had arrived in the staging directory
// (removed again; nothing in the installation changed). No new current state is written.
func (c itemCtx) envPartial(s *deploySession, it *gitRunItem, e *gitEnvRun, staging string, total int, cause error) error {
	var moved []string
	out, err := s.run(fmt.Sprintf("T=%s; if [ -d \"$T\" ]; then (cd \"$T\" && find . -type f | sed 's|^\\./|A\t|'); fi\necho WRM_DONE\n", shellQuote(staging)), "", time.Minute)
	if err == nil {
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "A\t") {
				moved = append(moved, l[2:])
			}
		}
	}
	sort.Strings(moved)
	s.dropStaging(staging)
	c.run.set(func() { it.Partial, it.Moved, it.MovedCount = true, listCap(moved), len(moved) })
	msg := fmt.Sprintf("the transfer was interrupted: %d of %d files had arrived in the staging directory (removed again); nothing in the installation was changed", len(moved), total)
	if err != nil {
		msg = fmt.Sprintf("the transfer was interrupted (what had arrived could not be read: %v); nothing in the installation was changed", err)
	}
	c.log("transfer", "fail", msg+": "+cause.Error())
	entry := map[string]interface{}{"action": "DEPLOY", "id": e.DeployID, "app": it.App, "env": it.Env, "by": usernameOf(c.run.UserID), "at": nowStamp(),
		"server": it.ConnName, "path": it.Path, "result": "partial", "moved": listCap(moved), "counts": map[string]int{"moved": len(moved), "total": total},
		"error": truncateStr(cause.Error(), 300), "run": c.run.Label}
	if herr := envAppendHistory(s.cl, it.StateDir, entry); herr != nil {
		c.log("history", "fail", herr.Error())
	}
	return fmt.Errorf("%s: %v", msg, cause)
}

// ─── rollback ────────────────────────────────────────

type gitEnvManifest struct {
	ID          string        `json:"id"`
	Backup      string        `json:"backup"`
	Added       []string      `json:"added"`
	Changed     []string      `json:"changed"`
	Deleted     []string      `json:"deleted"`
	RemovedDirs []gitEnvDir   `json:"removed_dirs"`
	VersionMD   bool          `json:"version_md_existed"`
	Previous    *gitEnvRecord `json:"previous"`
}

// envRollbackScript restores a deploy from its backup. Lines on stdin (in this order):
// A rel (added: moved aside), Z dir (empty directories of added files), M dir mode owner
// (removed directories: re-created), C rel (changed: the current version is copied aside,
// the backup goes back), D rel (deleted: back from the backup), V 1|0 (VERSION.md). Nothing
// is done through a symlink: such items are skipped (S lines). With DRY=1 it only reports.
func envRollbackScript(dir, backup, aside string, dry bool) string {
	d := "0"
	if dry {
		d = "1"
	}
	return fmt.Sprintf(`D=%s; B=%s; R=%s; DRY=%s
cd "$D" || { printf 'E\tcannot enter %%s\n' "$D"; exit 0; }
act() { [ "$DRY" = 1 ] || "$@"; }
issym() { p=$1; while :; do [ -L "$p" ] && return 0; case $p in */*) p=${p%%/*};; *) return 1;; esac; done; }
keepcur() { [ -e "$1" ] || return 0; [ -e "$R/$1" ] && return 0; d=${1%%/*}; [ "$d" = "$1" ] || act mkdir -p -- "$R/$d" || return 1; act cp -p -- "$1" "$R/$1"; }
fromb() { d=${1%%/*}; [ "$d" = "$1" ] || act mkdir -p -- "$d" || return 1; act cp -p -- "$B/$1" "$1.wrm-back" && act mv -f -- "$1.wrm-back" "$1"; }
[ -d "$B" ] || { printf 'E\tthe backup %%s is missing\n' "$B"; exit 0; }
act mkdir -p -- "$R" || { printf 'E\tcannot create %%s\n' "$R"; exit 0; }
while IFS= read -r line; do
  k=${line%%%%	*}; rest=${line#*	}
  case $k in
  A) if issym "$rest"; then printf 'S\t%%s\n' "$rest"
     elif [ -e "$rest" ]; then d=${rest%%/*}; { [ "$d" = "$rest" ] || act mkdir -p -- "$R/$d"; } && act mv -f -- "$rest" "$R/$rest" && printf 'A\t%%s\n' "$rest" || printf 'F\t%%s\n' "$rest"; fi ;;
  Z) if ! issym "$rest" && [ -d "$rest" ]; then act rmdir -- "$rest" 2>/dev/null; fi ;;
  M) d=${rest%%%%	*}; x=${rest#*	}; m=${x%%%%	*}; o=${x#*	}
     if issym "$d"; then printf 'S\t%%s\n' "$d"
     elif [ ! -d "$d" ]; then if act mkdir -p -- "$d"; then [ -z "$m" ] || act chmod -- "$m" "$d"; [ -z "$o" ] || act chown -- "$o" "$d" 2>/dev/null; printf 'M\t%%s\n' "$d"; else printf 'F\t%%s\n' "$d"; fi; fi ;;
  C|D) if issym "$rest"; then printf 'S\t%%s\n' "$rest"
     elif [ ! -f "$B/$rest" ]; then printf 'F\t%%s\tnot in the backup\n' "$rest"
     elif { [ "$k" = D ] || keepcur "$rest"; } && fromb "$rest"; then printf '%%s\t%%s\n' "$k" "$rest"
     else printf 'F\t%%s\n' "$rest"; fi ;;
  V) if issym VERSION.md; then printf 'S\tVERSION.md\n'
     elif [ "$rest" = 1 ] && [ -f "$B/VERSION.md" ]; then keepcur VERSION.md && fromb VERSION.md && printf 'C\tVERSION.md\n' || printf 'F\tVERSION.md\n'
     elif [ "$rest" = 0 ] && [ -e VERSION.md ]; then act mv -f VERSION.md "$R/VERSION.md" && printf 'A\tVERSION.md\n' || printf 'F\tVERSION.md\n'; fi ;;
  esac
done
echo WRM_DONE
`, shellQuote(dir), shellQuote(backup), shellQuote(aside), d)
}

func (run *gitRun) envRollbackItem(it *gitRunItem) error {
	c := itemCtx{run, it}
	e := run.P.Env
	cat := envCatalog(run.UserID, e.App, e.Env)
	app, env, err := findEnv(cat, e.App, e.Env)
	if err != nil {
		return err
	}
	if it.Dest >= len(env.Destinations) || env.Destinations[it.Dest].Path != it.Path {
		return fmt.Errorf("the environment changed: open the history again")
	}
	_, cl, err := c.dial(it.ConnID, "connect")
	if err != nil {
		return err
	}
	defer cl.Close()
	lockID := fmt.Sprintf("rollback-%d-%s", run.ID, e.DeployID)
	if e.DryRun {
		c.log("lock", "skip", "dry run: no lock is taken and nothing is written")
	} else {
		c.step("lock")
		holder, err := envLock(cl, it.StateDir, lockID, usernameOf(run.UserID))
		if err != nil {
			c.log("lock", "fail", err.Error())
			return err
		}
		if holder != nil {
			c.log("lock", "fail", lockMsg(holder))
			return errors.New(lockMsg(holder))
		}
		c.log("lock", "ok", it.StateDir+"/.lock")
		defer func() {
			if released, err := envUnlock(cl, it.StateDir, lockID); err != nil {
				c.log("unlock", "fail", err.Error())
			} else if released {
				c.log("unlock", "ok", "")
			}
		}()
	}
	c.step("backups")
	sd := shellQuote(it.StateDir)
	out, err := runRemoteScript(cl, fmt.Sprintf("SD=%s\n[ -f \"$SD/state.json\" ] && { printf 'J\\t'; tr -d '\\n\\r' < \"$SD/state.json\"; echo; }\n[ -f \"$SD/deploys/\"%s.json ] && { printf 'M\\t'; tr -d '\\n\\r' < \"$SD/deploys/\"%s.json; echo; }\n[ -d \"$SD/\"%s ] && echo BK\necho WRM_DONE\n",
		sd, shellQuote(e.DeployID), shellQuote(e.DeployID), shellQuote(e.DeployID)), "", gitStepTimeout)
	if err != nil {
		c.log("backups", "fail", err.Error())
		return err
	}
	var st gitEnvState
	var man *gitEnvManifest
	haveBackup := false
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "J\t"):
			jsonUnmarshalString(l[2:], &st)
		case strings.HasPrefix(l, "M\t"):
			var m gitEnvManifest
			if jsonUnmarshalString(l[2:], &m); m.ID != "" {
				man = &m
			}
		case l == "BK":
			haveBackup = true
		}
	}
	cur := "-"
	if st.Current != nil {
		cur = st.Current.DeployID
	}
	switch {
	case st.Current == nil || st.Current.DeployID != e.DeployID:
		err = fmt.Errorf("deploy %s is not the latest on this destination (current: %s)", e.DeployID, cur)
	case man == nil:
		err = fmt.Errorf("the manifest of deploy %s is missing", e.DeployID)
	case !haveBackup:
		err = fmt.Errorf("the backup of deploy %s is missing", e.DeployID)
	}
	if err != nil {
		c.log("backups", "fail", err.Error())
		return err
	}
	toV := "-"
	if man.Previous != nil {
		toV = man.Previous.Version
	}
	run.set(func() { it.FromVersion, it.ToVersion, it.DeployID = st.Current.Version, toV, e.DeployID })
	c.log("backups", "ok", fmt.Sprintf("deploy %s: %d added, %d changed, %d deleted", e.DeployID, len(man.Added), len(man.Changed), len(man.Deleted)))
	// stdin in the order the script expects
	var in strings.Builder
	dirs := map[string]bool{}
	for _, rel := range man.Added {
		in.WriteString("A\t" + rel + "\n")
		for d := path.Dir(rel); d != "." && d != "/"; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	zs := keys(dirs)
	sort.Slice(zs, func(i, j int) bool { return strings.Count(zs[i], "/") > strings.Count(zs[j], "/") })
	for _, d := range zs {
		in.WriteString("Z\t" + d + "\n")
	}
	rd := append([]gitEnvDir{}, man.RemovedDirs...)
	sort.Slice(rd, func(i, j int) bool { return strings.Count(rd[i].Path, "/") < strings.Count(rd[j].Path, "/") })
	for _, d := range rd {
		in.WriteString("M\t" + d.Path + "\t" + d.Mode + "\t" + d.Owner + "\n")
	}
	for _, rel := range man.Changed {
		in.WriteString("C\t" + rel + "\n")
	}
	for _, rel := range man.Deleted {
		in.WriteString("D\t" + rel + "\n")
	}
	if man.VersionMD {
		in.WriteString("V\t1\n")
	} else {
		in.WriteString("V\t0\n")
	}
	aside := it.StateDir + "/backups/rollback-" + e.DeployID
	c.step("restore")
	out, err = runRemoteScript(cl, envRollbackScript(it.Path, it.StateDir+"/"+e.DeployID, aside, e.DryRun), in.String(), gitTransferTimeout)
	if err == nil {
		err = scriptError(out)
	}
	if err == nil && !strings.Contains(out, "WRM_DONE") {
		err = fmt.Errorf("the rollback did not finish")
	}
	if err != nil {
		c.log("restore", "fail", err.Error())
		return err
	}
	var movedAside, restored, recreated, skipped, failed []string
	for _, l := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		switch k {
		case "A":
			movedAside = append(movedAside, v)
		case "C", "D":
			restored = append(restored, v)
		case "M":
			recreated = append(recreated, v)
		case "S":
			skipped = append(skipped, v)
		case "F":
			failed = append(failed, strings.ReplaceAll(v, "\t", ": "))
		}
	}
	summary := fmt.Sprintf("%d restored, %d moved aside to %s, %d directories re-created", len(restored), len(movedAside), aside, len(recreated))
	if e.DryRun {
		c.log("dry_run", "info", "would restore: "+summary)
		if len(skipped) > 0 {
			c.log("dry_run", "info", "would skip (symlinks on the server): "+strings.Join(capList(skipped, 50), ", "))
		}
		if len(failed) > 0 {
			c.log("dry_run", "info", "would fail: "+strings.Join(capList(failed, 50), ", "))
		}
		if e.PostCommand != "" {
			c.log("dry_run", "info", "would run post_deploy: "+e.PostCommand)
		}
		run.set(func() { it.State, it.Step = "ok", "" })
		return nil
	}
	complete := len(skipped) == 0 && len(failed) == 0
	status := "ok"
	if !complete {
		status = "fail"
	}
	c.log("restore", status, summary)
	run.set(func() { it.Written, it.Deleted, it.Skipped, it.MadeBackup = restored, movedAside, skipped, aside })
	var postErr error
	post := ""
	if e.PostCommand != "" && complete {
		if postErr = c.postDeploy(&deploySession{cl: cl, in: gitInstall{Path: it.Path}}, e.PostCommand); postErr != nil {
			post = "failed: " + truncateStr(postErr.Error(), 300)
		} else {
			post = "ok"
		}
	} else if e.PostCommand != "" {
		c.log("post_deploy", "skip", "not run: the rollback is incomplete")
	}
	c.step("state")
	user := usernameOf(run.UserID)
	if complete {
		if err := envWriteFile(cl, it.StateDir+"/state.json", jsonMarshal(gitEnvState{Current: man.Previous, RolledBack: e.DeployID})); err != nil {
			c.log("state", "fail", err.Error())
		} else {
			c.log("state", "ok", "the previous state is current again")
		}
	}
	entry := map[string]interface{}{"action": "ROLLBACK", "id": fmt.Sprintf("rollback-%s-%s", e.DeployID, time.Now().Format("20060102-150405")), "rolled_back": e.DeployID,
		"app": app.Name, "env": env.Name, "by": user, "at": nowStamp(), "server": it.ConnName, "path": it.Path, "from": st.Current, "to": man.Previous,
		"restored": listCap(restored), "moved_aside": listCap(movedAside), "skipped": listCap(skipped), "failed": listCap(failed), "complete": complete, "aside": aside,
		"counts": map[string]int{"restored": len(restored), "moved_aside": len(movedAside), "skipped": len(skipped), "failed": len(failed), "dirs": len(recreated)}, "run": run.Label}
	if post != "" {
		entry["post_deploy"] = post
	}
	if man.Previous != nil {
		entry["version"], entry["commit"], entry["branch"] = man.Previous.Version, man.Previous.Commit, man.Previous.Branch
	}
	if err := envAppendHistory(cl, it.StateDir, entry); err != nil {
		c.log("history", "fail", err.Error())
	}
	s := &deploySession{cl: cl, in: gitInstall{ID: it.InstallID, ConnID: it.ConnID, ConnName: it.ConnName, App: it.App, Path: it.Path, Env: env.Name}}
	envPrune(s, it.StateDir, "", 1<<30) // rollback backups only
	if it.InstallID != 0 {
		c.step("register")
		c.envRegister(s)
	}
	run.set(func() { it.PostDeploy = post })
	if !complete {
		msg := fmt.Sprintf("the rollback is incomplete: %d skipped (symlinks on the server: %s), %d failed; fix the server and roll back again",
			len(skipped), strings.Join(capList(skipped, 5), ", "), len(failed))
		run.set(func() { it.Note = msg })
		return errors.New(msg)
	}
	if postErr != nil {
		note := "post_deploy failed: the files are restored already"
		run.set(func() { it.Note = note })
		return fmt.Errorf("post_deploy: %v (%s)", postErr, note)
	}
	run.set(func() { it.State, it.Step = "ok", "" })
	return nil
}
