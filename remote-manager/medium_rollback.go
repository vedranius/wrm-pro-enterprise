// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

// ─── MEDIUM: native rollback ───
//
// A rollback undoes a destination's last applied deployment (the original tool's rule:
// backups assume the state after their own deployment, so deployments are rolled back in
// reverse order). Under the lock, per destination:
//
//  1. the deployment must still be the last one, and current.json what it left;
//  2. what it added (backups/<id>.added) moves into backups/rollback-<rollback id>/, and
//     directories it added are removed when empty;
//  3. what it replaced or deleted comes back from backups/<id>/ (the current version moves
//     into the rollback backup first, so nothing is lost);
//  4. directories it deleted are created again with their mode and owner (<id>.dirs);
//  5. post_deploy; 6. current.json becomes the state before that deployment (or is
//     removed) and history gets a ROLLBACK entry.
//
// Nothing is written through a symbolic link on the server inside the application path
// (it may lead elsewhere). An incomplete rollback is recorded and can be repeated: every
// step continues where the previous attempt stopped. A dry rollback changes nothing and
// reports what the real one would do.

type mdRollbackMark struct {
	Mark string // D moved away, d empty dir removed, K dir kept, R restored, m dir recreated, L symlink on the path, M dir in the way, E error
	Path string
}

var mdRollbackVerbs = map[string]string{
	"D": "removed (added by the deployment)", "d": "empty directory removed", "K": "directory kept (it holds other files)",
	"R": "restored from the backup", "m": "directory created again", "E": "ERROR",
	"L": "not touched: a symbolic link is on the path", "M": "cannot be restored: a directory is in its place",
}

func mdReadLines(h mdHost, p string) ([]string, bool, error) {
	b, err := h.ReadFile(p, 50<<20)
	if err != nil {
		return nil, false, err
	}
	if b == nil {
		return nil, false, nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out, true, nil
}

func mdCheckedRel(text string) (string, error) {
	rel := strings.TrimSuffix(mdUnescapePath(text), "/")
	if rel == "" || strings.HasPrefix(rel, "/") || mdHasDotDot(rel) {
		return "", fmt.Errorf("invalid path in a backup manifest: %q", text)
	}
	return rel, nil
}

type mdDirRecord struct {
	mode     string
	uid, gid int
	path     string
	owner    bool
}

func mdReadDirsFile(h mdHost, p string) ([]mdDirRecord, error) {
	lines, _, err := mdReadLines(h, p)
	if err != nil {
		return nil, err
	}
	var out []mdDirRecord
	for _, l := range lines {
		parts := strings.Split(l, "\t")
		if len(parts) == 4 {
			u, e1 := strconv.Atoi(parts[1])
			g, e2 := strconv.Atoi(parts[2])
			_, e3 := strconv.ParseUint(parts[0], 8, 32)
			if e1 == nil && e2 == nil && e3 == nil {
				rel, err := mdCheckedRel(parts[3])
				if err != nil {
					return nil, err
				}
				out = append(out, mdDirRecord{mode: parts[0], uid: u, gid: g, path: rel, owner: true})
				continue
			}
		}
		rel, err := mdCheckedRel(l)
		if err != nil {
			return nil, err
		}
		out = append(out, mdDirRecord{path: rel})
	}
	return out, nil
}

// mdModeFromOctal turns "2775" into a FileMode with the special bits.
func mdModeFromOctal(s string) os.FileMode {
	v, _ := strconv.ParseUint(s, 8, 32)
	m := os.FileMode(v & 0o777)
	if v&0o4000 != 0 {
		m |= os.ModeSetuid
	}
	if v&0o2000 != 0 {
		m |= os.ModeSetgid
	}
	if v&0o1000 != 0 {
		m |= os.ModeSticky
	}
	return m
}

// mdBackupFiles lists the files and symlinks of a backup directory (relative paths).
func mdBackupFiles(h mdHost, root string) ([]string, error) {
	var out []string
	var walk func(rel string) error
	walk = func(rel string) error {
		list, err := h.ReadDir(path.Join(root, rel))
		if err != nil {
			return err
		}
		for _, fi := range list {
			p := fi.Name()
			if rel != "" {
				p = rel + "/" + p
			}
			switch {
			case fi.IsDir():
				if err := walk(p); err != nil {
					return err
				}
			case fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink != 0:
				out = append(out, p)
			}
			if len(out) > mdMaxEntries {
				return fmt.Errorf("more than %d files in the backup", mdMaxEntries)
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

type mdRollbackTarget struct {
	d          *mdDestination
	choice     mdRollbackChoice
	postDeploy *string
}

// mdRollbackOne rolls back (or dry-runs the rollback of) one destination.
func mdRollbackOne(dc *mdDeployContext, h mdHost, t mdRollbackTarget) mdOutcome {
	j, d, rid := dc.job, t.d, dc.entry.DeployID
	out := mdOutcome{Server: d.Server, Label: d.label(), Status: "ok"}
	fail := func(status, msg string) mdOutcome {
		out.Status, out.Message = status, msg
		j.log("error", "%s: %s", d.label(), msg)
		return out
	}
	deploy := t.choice.Deploy
	if deploy == nil {
		return fail("failed", t.choice.Reason)
	}
	id := deploy.DeployID
	if !mdDeployIDRe.MatchString(id) {
		return fail("failed", fmt.Sprintf("invalid deployment id in the history: %q", id))
	}
	dry := ""
	if dc.dryRun {
		dry = " (dry run: nothing is changed)"
	}
	j.log("info", "── %s: rollback of %s (%s, %s)%s", d.label(), id, deploy.Version, deploy.DeployedBy, dry)
	if !dc.dryRun {
		owner := mdLockOwner{User: dc.entry.DeployedBy, Host: mdHostName(), Since: mdNowISO(), Project: dc.entry.Project, Environment: dc.entry.Environment, DeployID: strPtr(rid)}
		if err := mdAcquireLock(h, d, owner); err != nil {
			if le, ok := err.(*mdLockedError); ok {
				out.LockOwner = le.Owner
				return fail("locked", err.Error())
			}
			return fail("failed", err.Error())
		}
		j.log("ok", "Lock set: %s/lock", d.statePath())
		defer func() {
			if err := mdReleaseLock(h, d, rid); err != nil {
				j.log("warn", "The lock was not released (%v); release it under History once it is stale.", err)
			} else {
				j.log("info", "Lock released.")
			}
		}()
	}
	// 1. still the last deployment (under the lock nobody deploys or rolls back meanwhile)
	entries, err := mdReadHistory(h, d, mdHistoryWindow)
	if err != nil {
		return fail("failed", err.Error())
	}
	fresh := mdChooseRollback(entries)
	current, err := mdReadCurrent(h, d)
	if err != nil {
		return fail("failed", err.Error())
	}
	restores := t.choice.Restores
	expected := ""
	if mdSetsCurrent(*deploy) {
		expected = id
	} else if restores != nil {
		expected = restores.DeployID
	}
	now := ""
	if current != nil {
		now = current.DeployID
	}
	if fresh.Deploy == nil || fresh.Deploy.DeployID != id || now != expected {
		out.Status = "mismatch"
		out.Message = fmt.Sprintf("%s is no longer the last deployment on %s (another deployment or rollback ran, or current.json changed); refresh History and try again", id, d.label())
		j.log(map[bool]string{true: "warn", false: "error"}[dc.dryRun], "%s", out.Message)
		return out
	}
	backups, err := mdStateFile(d, "backups")
	if err != nil {
		return fail("failed", err.Error())
	}
	addedLines, ok, err := mdReadLines(h, path.Join(backups, id+".added"))
	if err != nil {
		return fail("failed", err.Error())
	}
	if !ok {
		return fail("failed", fmt.Sprintf("the backup of %s on %s is no longer kept (only the last %d deployments are); deploy an older version instead", id, d.label(), mdKeepBackups))
	}
	var added []string
	for _, l := range addedLines {
		rel, err := mdCheckedRel(l)
		if err != nil {
			return fail("failed", err.Error())
		}
		added = append(added, rel)
	}
	dirs, err := mdReadDirsFile(h, path.Join(backups, id+".dirs"))
	if err != nil {
		return fail("failed", err.Error())
	}
	rbDir := path.Join(backups, mdRollbackPrefix+rid)
	if !dc.dryRun {
		if _, err := h.Lstat(rbDir); err == nil {
			return fail("failed", fmt.Sprintf("a rollback backup %s already exists on %s; start the rollback again", rbDir, d.label()))
		}
	}
	root := d.appDir()
	var started bool
	var removed, restored, recreated []mdRollbackMark
	incomplete := func(err error) mdOutcome {
		if started {
			first := strings.SplitN(err.Error(), "\n", 2)[0]
			note := fmt.Sprintf("rollback incomplete (%s); it can be repeated, and the replaced versions are in backups/%s%s", first, mdRollbackPrefix, rid)
			e := mdRollbackEntry(dc, d, rid, deploy, restores, removed, restored, recreated)
			e.Note, e.Current = &note, boolPtr(false)
			if werr := mdWriteState(h, d, &e, false); werr != nil {
				j.log("warn", "History was not written: %v", werr)
			}
		}
		return fail("failed", err.Error())
	}
	if why := j.stop(); why != "" && !dc.dryRun {
		return fail("skipped", "stopped before any change: "+why)
	}
	started = !dc.dryRun
	j.setChanging(started)
	defer j.setChanging(false)
	moveAway := func(rel string) error {
		dst := path.Join(rbDir, rel)
		if err := h.MkdirAll(path.Dir(dst)); err != nil {
			return err
		}
		return h.Rename(path.Join(root, rel), dst)
	}
	// 2. what the deployment added, deepest first (a directory after its contents)
	order := append([]string{}, added...)
	sort.Slice(order, func(a, b int) bool {
		if mdDepth(order[a]) != mdDepth(order[b]) {
			return mdDepth(order[a]) > mdDepth(order[b])
		}
		return order[a] > order[b]
	})
	addedSet := map[string]bool{}
	for _, p := range added {
		addedSet[p] = true
	}
	gone := map[string]bool{} // dry run: what the real run would have removed
	for _, rel := range order {
		if mdLinkOnPath(h, root, rel) {
			removed = append(removed, mdRollbackMark{"L", rel})
			continue
		}
		fi, err := h.Lstat(path.Join(root, rel))
		if err != nil {
			continue // already gone
		}
		switch {
		case fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink != 0:
			if dc.dryRun {
				removed, gone[rel] = append(removed, mdRollbackMark{"D", rel}), true
				continue
			}
			if err := moveAway(rel); err != nil {
				removed = append(removed, mdRollbackMark{"E", rel})
				continue
			}
			removed = append(removed, mdRollbackMark{"D", rel})
		case fi.IsDir():
			if dc.dryRun {
				empty := true
				if list, err := h.ReadDir(path.Join(root, rel)); err == nil {
					for _, c := range list {
						cp := rel + "/" + c.Name()
						empty = empty && addedSet[cp] && gone[cp]
					}
				} else {
					empty = false
				}
				if empty {
					removed, gone[rel] = append(removed, mdRollbackMark{"d", rel}), true
				} else {
					removed = append(removed, mdRollbackMark{"K", rel})
				}
				continue
			}
			if err := h.RemoveDir(path.Join(root, rel)); err != nil {
				removed = append(removed, mdRollbackMark{"K", rel})
				continue
			}
			removed = append(removed, mdRollbackMark{"d", rel})
		}
	}
	// 3. back from the deployment's backup
	bkDir := path.Join(backups, id)
	if fi, err := h.Lstat(bkDir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return incomplete(fmt.Errorf("backups/%s is a symbolic link (refused)", id))
	}
	var files []string
	if fi, err := h.Lstat(bkDir); err == nil && fi.IsDir() {
		if files, err = mdBackupFiles(h, bkDir); err != nil {
			return incomplete(fmt.Errorf("read backups/%s: %v", id, err))
		}
	}
	for _, rel := range files {
		dst := path.Join(root, rel)
		if mdLinkOnPath(h, root, rel) {
			restored = append(restored, mdRollbackMark{"L", rel})
			continue
		}
		if fi, err := h.Lstat(dst); err == nil && fi.IsDir() && !(dc.dryRun && gone[rel]) {
			restored = append(restored, mdRollbackMark{"M", rel})
			continue
		}
		if dc.dryRun {
			restored = append(restored, mdRollbackMark{"R", rel})
			continue
		}
		if _, err := h.Lstat(dst); err == nil {
			if err := moveAway(rel); err != nil {
				restored = append(restored, mdRollbackMark{"E", rel})
				continue
			}
		}
		if err := h.MkdirAll(path.Dir(dst)); err != nil {
			restored = append(restored, mdRollbackMark{"E", rel})
			continue
		}
		if err := h.Rename(path.Join(bkDir, rel), dst); err != nil {
			restored = append(restored, mdRollbackMark{"E", rel})
			continue
		}
		restored = append(restored, mdRollbackMark{"R", rel})
	}
	// 4. directories the deployment deleted, shallowest first
	sort.SliceStable(dirs, func(a, b int) bool { return mdDepth(dirs[a].path) < mdDepth(dirs[b].path) })
	for _, dr := range dirs {
		if dc.dryRun {
			recreated = append(recreated, mdRollbackMark{"m", dr.path})
			continue
		}
		p := path.Join(root, dr.path)
		if mdLinkOnPath(h, root, dr.path) {
			recreated = append(recreated, mdRollbackMark{"L", dr.path})
			continue
		}
		if fi, err := h.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			recreated = append(recreated, mdRollbackMark{"L", dr.path})
			continue
		}
		if err := h.MkdirAll(p); err != nil {
			recreated = append(recreated, mdRollbackMark{"E", dr.path})
			continue
		}
		if dr.mode != "" {
			h.Chmod(p, mdModeFromOctal(dr.mode))
		}
		if dr.owner {
			h.Chown(p, dr.uid, dr.gid) // only root (or the owner, for the group) may; the original ignores failures too
		}
		recreated = append(recreated, mdRollbackMark{"m", dr.path})
	}
	var problems []string
	for i, m := range append(append(append([]mdRollbackMark{}, removed...), restored...), recreated...) {
		if i < 500 {
			level := "out"
			if m.Mark == "E" || m.Mark == "L" || m.Mark == "M" {
				level = "error"
			}
			j.log(level, "%s: %s", mdRollbackVerbs[m.Mark], mdEscapePath(m.Path))
		}
		if m.Mark == "E" || m.Mark == "L" || m.Mark == "M" {
			problems = append(problems, mdEscapePath(m.Path)+" ("+mdRollbackVerbs[m.Mark]+")")
		}
	}
	for _, m := range restored {
		if m.Mark == "R" {
			out.Sent++
		}
	}
	var kept []string
	for _, m := range removed {
		if m.Mark == "D" || m.Mark == "d" {
			out.Deleted++
		}
		if m.Mark == "K" {
			kept = append(kept, mdEscapePath(m.Path))
		}
	}
	if len(kept) > 0 {
		j.log("warn", "Directories the deployment added that now hold other files stay: %s", mdShown(kept, 10))
	}
	if dc.dryRun {
		if len(problems) > 0 {
			out.Status = "mismatch"
			out.Message = "dry run: the real rollback would be incomplete for " + mdShown(problems, 5) + "; fix that on the server first"
			j.log("warn", "%s", out.Message)
		} else {
			out.Status = "dry-run"
			out.Message = fmt.Sprintf("dry run: %d would be restored and %d removed; afterwards: %s", out.Sent, out.Deleted, t.choice.After())
			j.log("ok", "%s: %s", d.label(), out.Message)
		}
		if t.postDeploy != nil {
			j.log("info", "After the rollback this command would run (post_deploy): %s", *t.postDeploy)
		}
		return out
	}
	if len(problems) > 0 {
		return incomplete(fmt.Errorf("the rollback is incomplete: %s (fix it on the server and run the same rollback again)", mdShown(problems, 10)))
	}
	var note *string
	if t.postDeploy != nil && strings.TrimSpace(*t.postDeploy) != "" {
		if n := mdRunPostDeploy(h, d, *t.postDeploy, j); n != "" {
			note = &n
			out.Status, out.Message = "failed", n+"; the files are restored"
			j.log("error", "%s", out.Message)
		}
	}
	e := mdRollbackEntry(dc, d, rid, deploy, restores, removed, restored, recreated)
	e.Note, e.Current = note, boolPtr(true)
	cur, err := mdStateFile(d, "current.json")
	if err != nil {
		return fail("failed", err.Error())
	}
	if restores == nil {
		if _, err := mdShCheck(h, "rm -f -- "+shellQuote(cur), nil, "remove current.json"); err != nil {
			return fail("failed", err.Error())
		}
	} else {
		r := *restores
		r.Path = strPtr(root)
		if _, err := mdShCheck(h, "cat > "+shellQuote(cur+".tmp")+" && mv -f -- "+shellQuote(cur+".tmp")+" "+shellQuote(cur), mdMarshalLine(&r), "write current.json"); err != nil {
			return fail("failed", err.Error())
		}
	}
	if err := mdWriteState(h, d, &e, false); err != nil {
		return fail("failed", err.Error())
	}
	if restores != nil {
		j.log("ok", "Written: current.json (%s) and ROLLBACK in history.jsonl", restores.DeployID)
	} else {
		j.log("ok", "Written: current.json removed (the state before the first deployment) and ROLLBACK in history.jsonl")
	}
	if out.Status == "ok" {
		out.Message = fmt.Sprintf("rolled back: %d restored from the backup, %d added items removed", out.Sent, out.Deleted)
		j.log("ok", "%s: %s", d.label(), out.Message)
	}
	return out
}

// mdRollbackEntry is the ROLLBACK history entry: what was actually restored and removed.
func mdRollbackEntry(dc *mdDeployContext, d *mdDestination, rid string, deploy *mdHistoryEntry, restores *mdRecord, removed, restored, recreated []mdRollbackMark) mdHistoryEntry {
	var news, changed, deleted []string
	for _, m := range recreated {
		if m.Mark == "m" {
			news = append(news, m.Path)
		}
	}
	for _, m := range restored {
		if m.Mark == "R" {
			changed = append(changed, m.Path)
		}
	}
	for _, m := range removed {
		if m.Mark == "D" || m.Mark == "d" {
			deleted = append(deleted, m.Path)
		}
	}
	e := mdHistoryEntry{Action: "ROLLBACK"}
	e.mdRecord = mdRecord{Project: dc.entry.Project, Environment: dc.entry.Environment, Version: "the state before the first deployment", Excluded: []string{}}
	if restores != nil {
		e.Version, e.Commit, e.Branch, e.RefKind = restores.Version, restores.Commit, restores.Branch, restores.RefKind
		e.Excluded = append([]string{}, restores.Excluded...)
	}
	e.DeployID, e.DeployedAt, e.DeployedBy, e.Path = rid, mdNowISO(), dc.entry.DeployedBy, strPtr(d.appDir())
	e.Server, e.RolledBack = strPtr(d.Server), strPtr(deploy.DeployID)
	mdHistoryFiles(&e, news, changed, deleted)
	return e
}

func mdRollbackDestinations(dc *mdDeployContext, targets []mdRollbackTarget) {
	j := dc.job
	for i, t := range targets {
		if why := j.stop(); why != "" && !dc.dryRun {
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
				out = mdRollbackOne(dc, h, t)
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
