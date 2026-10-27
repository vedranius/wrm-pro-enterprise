// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ─── MEDIUM: server state in the original tool's format ───
//
// The native engine writes current.json, history.jsonl, lock/owner.json and the backup
// manifests exactly as the original tool does, so either tool can read the state the other
// wrote, deploy after it and roll back its deployments. The records are compact JSON with
// the keys in the original's order and every field present (null when unknown), because
// the original matches some of them as text (a lock is ours when owner.json contains
// "deploy_id":"<id>").

const (
	mdKeepBackups     = 5   // backups of the last 5 applied deployments are kept
	mdHistoryWindow   = 100 // history entries read for the rollback choice
	mdHistoryPaths    = 500 // paths per kind in one history entry
	mdStaleLockFloor  = 30  // minutes; a lock is released by hand only, and never younger
	mdRollbackPrefix  = "rollback-"
	mdLockedExit      = 75
	mdIDExistsExit    = 76
	mdStaleGone       = 3
	mdStaleOtherOwner = 4
	mdStaleYoung      = 5
	mdRestoreRefused  = 9
	// mdPreTransferNote is how the original tool's notes begin when a deployment failed
	// before anything changed. It is a format token of the original's history (its own
	// language), read only for its old entries without "counts"; WRM never writes it.
	mdPreTransferNote = "deploy nije uspio"
	mdTimeFormat      = "2006-01-02T15:04:05Z"
)

var (
	mdDeployIDRe = regexp.MustCompile(`^\d{8}-\d{4}(\d{2})?-[0-9a-f]{7}$`)
	mdSHARe      = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
)

// mdMakeDeployID is YYYYMMDD-HHMMSS-<7 characters of the commit> in UTC.
func mdMakeDeployID(t time.Time, sha string) (string, error) {
	short := strings.ToLower(strings.TrimSpace(sha))
	if !mdSHARe.MatchString(short) {
		return "", fmt.Errorf("invalid commit %q", sha)
	}
	return t.UTC().Format("20060102-150405") + "-" + short[:7], nil
}

func mdNowISO() string { return time.Now().UTC().Format(mdTimeFormat) }

// mdRecord is current.json; the field order is the original's.
type mdRecord struct {
	Project     string   `json:"project"`
	Environment string   `json:"environment"`
	Version     string   `json:"version"`
	Commit      string   `json:"commit"`
	Branch      string   `json:"branch"`
	RefKind     *string  `json:"ref_kind"`
	DeployID    string   `json:"deploy_id"`
	DeployedAt  string   `json:"deployed_at"`
	DeployedBy  string   `json:"deployed_by"`
	Excluded    []string `json:"excluded"`
	Path        *string  `json:"path"`
}

// mdHistoryEntry is one line of history.jsonl: the record's fields, then the entry's.
type mdHistoryEntry struct {
	mdRecord
	Action     string         `json:"action"`
	Server     *string        `json:"server"`
	Note       *string        `json:"note"`
	RolledBack *string        `json:"rolled_back"`
	Current    *bool          `json:"current"`
	Previous   *mdRecord      `json:"previous"`
	New        []string       `json:"new"`
	Changed    []string       `json:"changed"`
	Deleted    []string       `json:"deleted"`
	Counts     map[string]int `json:"counts"`

	hasPrevious bool // the line has a "previous" key (older entries do not)
}

// UnmarshalJSON reads an entry and remembers whether "previous" was present.
func (e *mdHistoryEntry) UnmarshalJSON(b []byte) error {
	type plain mdHistoryEntry
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return err
	}
	_, v.hasPrevious = keys["previous"]
	if v.Action == "" {
		v.Action = "DEPLOY"
	}
	*e = mdHistoryEntry(v)
	return nil
}

type mdLockOwner struct {
	User        string  `json:"user"`
	Host        string  `json:"host"`
	Since       string  `json:"since"`
	Project     string  `json:"project"`
	Environment string  `json:"environment"`
	DeployID    *string `json:"deploy_id"`
	Path        *string `json:"path"`
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

// mdMarshalLine is compact JSON without HTML escaping, plus a newline. Nil lists are
// written as [] (the original's lists are never null).
func mdMarshalLine(v interface{}) []byte {
	switch x := v.(type) {
	case *mdRecord:
		x.fill()
	case *mdHistoryEntry:
		x.mdRecord.fill()
		if x.Previous != nil {
			x.Previous.fill()
		}
		for _, l := range []*[]string{&x.New, &x.Changed, &x.Deleted} {
			if *l == nil {
				*l = []string{}
			}
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return buf.Bytes()
}

func (r *mdRecord) fill() {
	if r.Excluded == nil {
		r.Excluded = []string{}
	}
}

// mdParseRecord reads current.json (a BOM and whitespace are allowed).
func mdParseRecord(b []byte) (*mdRecord, error) {
	var r mdRecord
	if err := json.Unmarshal(bytes.TrimPrefix(bytes.TrimSpace(b), []byte("\xef\xbb\xbf")), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// mdParseHistory reads history.jsonl and returns the entries newest first; lines that are
// not valid entries are skipped.
func mdParseHistory(text string) []mdHistoryEntry {
	var out []mdHistoryEntry
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(strings.TrimPrefix(lines[i], "\ufeff"))
		if line == "" {
			continue
		}
		var e mdHistoryEntry
		if json.Unmarshal([]byte(line), &e) != nil || e.DeployID == "" {
			continue
		}
		out = append(out, e)
	}
	return out
}

// mdEscapePathControls also escapes control characters, so a name with a newline stays one
// line in the backup manifests.
func mdEscapePathControls(p string) string {
	p = mdEscapePath(p)
	if !strings.ContainsAny(p, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f") {
		return p
	}
	var sb strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c < 32 || c == 127 {
			fmt.Fprintf(&sb, `\#%03o`, c)
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// ─── rollback choice (which deployment a rollback undoes, and what it restores) ───

// mdChangedFiles: did the entry's deployment start changing files on the server?
func mdChangedFiles(e mdHistoryEntry) bool {
	if e.Action != "DEPLOY" {
		return false
	}
	if e.Counts != nil {
		return true
	}
	return e.Note == nil || !strings.HasPrefix(*e.Note, mdPreTransferNote)
}

// mdSetsCurrent: did the entry set current.json?
func mdSetsCurrent(e mdHistoryEntry) bool {
	if e.Current != nil {
		return *e.Current
	}
	note := ""
	if e.Note != nil {
		note = *e.Note
	}
	return e.Action == "DEPLOY" && (note == "" || strings.HasPrefix(note, "post_deploy"))
}

// mdAppliedDeploys: the destination's deployments that changed files and were not rolled
// back, oldest first. entries are newest first. A complete rollback removes the deployment
// it names; an incomplete one leaves it, so the same rollback can be repeated.
func mdAppliedDeploys(entries []mdHistoryEntry) []mdHistoryEntry {
	var stack []mdHistoryEntry
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if mdChangedFiles(e) {
			stack = append(stack, e)
			continue
		}
		if e.Action == "ROLLBACK" && e.RolledBack != nil && *e.RolledBack != "" && (e.Current == nil || *e.Current) {
			for j := len(stack) - 1; j >= 0; j-- {
				if stack[j].DeployID == *e.RolledBack {
					stack = append(stack[:j], stack[j+1:]...)
					break
				}
			}
		}
	}
	return stack
}

type mdRollbackChoice struct {
	Deploy   *mdHistoryEntry `json:"deploy"`
	Restores *mdRecord       `json:"restores"`
	Reason   string          `json:"reason,omitempty"`
}

// After is the state after the rollback, for display.
func (c mdRollbackChoice) After() string {
	if c.Restores == nil {
		return "the state before the first deployment"
	}
	return c.Restores.Version + " (" + c.Restores.DeployID + ")"
}

func mdChooseRollback(entries []mdHistoryEntry) mdRollbackChoice {
	stack := mdAppliedDeploys(entries)
	if len(stack) == 0 {
		return mdRollbackChoice{Reason: "no deployment to roll back"}
	}
	target := stack[len(stack)-1]
	c := mdRollbackChoice{Deploy: &target}
	if target.hasPrevious {
		c.Restores = target.Previous
		return c
	}
	for i := len(stack) - 2; i >= 0; i-- {
		if mdSetsCurrent(stack[i]) {
			r := stack[i].mdRecord
			c.Restores = &r
			break
		}
	}
	return c
}

// mdStaleLockMinutes: how old a lock must be before it may be released by hand (the
// longest a deployment can legitimately take, plus 5 minutes).
func mdStaleLockMinutes() int {
	cmd := settingInt("medium_command_timeout")
	transfer := settingInt("medium_transfer_timeout")
	if transfer < cmd {
		transfer = cmd
	}
	if transfer < 3600 {
		transfer = 3600
	}
	m := (2*cmd+transfer+59)/60 + 5
	if m < mdStaleLockFloor {
		m = mdStaleLockFloor
	}
	return m
}
