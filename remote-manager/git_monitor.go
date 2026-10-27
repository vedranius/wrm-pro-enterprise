// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// ─── PERIODIC GIT CHECKS ─────────────────────────────
//
// While WRM runs, users who turned monitoring on get their targets refreshed and their
// servers checked every git_check_interval_minutes. Checks never change anything on a
// server; they notify (one digest per round, through the notifications module) about new
// versions, drift (files changed by hand) and unreachable servers.

func runGitMonitor() {
	time.Sleep(2 * time.Minute)
	for {
		gitMonitorRound(time.Now())
		time.Sleep(time.Minute)
	}
}

// gitMonitorRound checks every user whose interval has passed.
func gitMonitorRound(now time.Time) {
	interval := settingInt("git_check_interval_minutes")
	if interval <= 0 || !gitAllowed() {
		return
	}
	rows, err := db.Query(`SELECT user_id, last_check_at FROM git_workspace`)
	if err != nil {
		return
	}
	type due struct {
		id   int
		last string
	}
	var list []due
	for rows.Next() {
		var d due
		rows.Scan(&d.id, &d.last)
		list = append(list, d)
	}
	rows.Close()
	for _, d := range list {
		if t, err := time.Parse(time.RFC3339, d.last); err == nil && now.Sub(t) < time.Duration(interval)*time.Minute {
			continue
		}
		_, st := loadGitWorkspace(d.id)
		if !st.Monitor || len(st.ConnIDs) == 0 || !gitChecksAllowed(d.id) {
			continue
		}
		if !gitLock(d.id) {
			continue
		}
		func() {
			defer gitUnlock(d.id)
			defer func() {
				if r := recover(); r != nil {
					log.Printf("git monitor: %v", r)
				}
			}()
			runGitRound(d.id, st.ConnIDs, true, true, true)
		}()
	}
}

// notifyGitRound sends the Git events of one round (the dispatcher makes one digest of them).
func notifyGitRound(userID int, targets []gitTargetChange, changes []gitChange) {
	for _, c := range targets {
		key := fmt.Sprintf("git:ver:%d:%s", userID, c.App)
		if getNotifyState(key) == c.New {
			continue
		}
		c := c
		if notifyUser(userID, "git.new_version", func(lang string) (string, string) {
			vars := map[string]string{"app": c.App, "old": c.Old, "new": c.New}
			return notifyText(lang, "git.new_version", vars), notifyText(lang, "git.new_version.body", vars)
		}) {
			setNotifyState(key, c.New)
		}
	}
	for _, c := range changes {
		c := c
		ev := "git." + c.Kind
		notifyUser(userID, ev, func(lang string) (string, string) {
			vars := map[string]string{"app": c.App, "server": c.Name, "path": c.Path, "error": strings.TrimSpace(c.Error)}
			return notifyText(lang, ev, vars), notifyText(lang, ev+".body", vars)
		})
	}
}
