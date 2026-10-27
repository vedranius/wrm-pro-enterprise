// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path"
	"sort"
	"strings"
	"unicode"
)

// Log grouping (v13.6): every discovered log file gets the main folder it lives
// under, whether that folder is a log folder or an application (script)
// folder, and an operator guessed from words that repeat across service and
// folder names. Two folders with the same name, one under /var/log and one
// with the scripts, are then told apart in the list, and "nova" and "novatv"
// end up in one operator group.

// logEnvWords are environment and copy markers, never operators.
var logEnvWords = map[string]bool{}

// logGenericWords describe what a folder holds rather than whose it is.
var logGenericWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`qa test tests testing dev devel develop development stage staging stg prod production prd uat preprod int integration
		bkp bak backup backups old new live demo tmp temp copy local beta alpha main master current release sandbox canary`) {
		logEnvWords[w] = true
	}
	for _, w := range strings.Fields(`log logs app apps api service services server servers worker workers web www data files file home var srv opt run bin
		script scripts deploy info error errors warn out err access history tools cron jobs job daemon tmp cache config conf lib share www-data
		nginx apache httpd docker containers supervisor systemd journal`) {
		logGenericWords[w] = true
	}
}

// logNameTokens splits a service or folder name into lower-case words. A
// trailing number is dropped (nova2 -> nova) and short or numeric words are
// left out.
func logNameTokens(name string) []string {
	out := []string{}
	for _, w := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if t := strings.TrimRightFunc(w, unicode.IsDigit); len(t) >= 3 {
			w = t
		}
		if len(w) < 3 || strings.TrimFunc(w, unicode.IsDigit) == "" {
			continue
		}
		out = append(out, w)
	}
	return out
}

// logMainFolder returns the longest main folder that holds dir.
func logMainFolder(dir string, mains []string) string {
	best := ""
	for _, m := range mains {
		if (dir == m || strings.HasPrefix(dir, strings.TrimSuffix(m, "/")+"/")) && len(m) > len(best) {
			best = m
		}
	}
	if best == "" {
		// Folders the service metadata named outside every main folder
		// (/var/lib/docker/containers/<id>) are grouped under their parent.
		best = path.Dir(dir)
		if best == "/" {
			best = dir
		}
	}
	return best
}

// logPlace says whether a file sits in a log folder or in an application or
// script folder.
func logPlace(dir, main string) string {
	if dir == "/var/log" || strings.HasPrefix(dir, "/var/log/") || strings.HasPrefix(dir, "/var/lib/docker/") {
		return "logs"
	}
	for _, s := range strings.Split(strings.Trim(strings.TrimPrefix(dir, main), "/"), "/") {
		if monitorLogDirName(s) && s != "var" {
			return "logs"
		}
	}
	if monitorLogDirName(path.Base(main)) && path.Base(main) != "var" {
		return "logs"
	}
	return "app"
}

// logFileNames lists the names that describe a file: its service label and
// the folders between its main folder and the file, deepest first. Hidden
// folders (.deploy) and log folder names (logs) say nothing about the owner.
func logFileNames(f monitorLogFile) []string {
	names := []string{}
	if f.Service != "" {
		names = append(names, f.Service)
	}
	rel := strings.Trim(strings.TrimPrefix(path.Dir(f.Path), f.Main), "/")
	if rel == "" {
		return names
	}
	segs := strings.Split(rel, "/")
	for i := len(segs) - 1; i >= 0; i-- {
		s := segs[i]
		if strings.HasPrefix(s, ".") || monitorLogDirName(s) || strings.EqualFold(s, f.Service) {
			continue
		}
		names = append(names, s)
	}
	return names
}

// logOperatorWords finds the words that name operators. A word counts when it
// shows up in at least two different names and, in at least one of them, it
// is not the first word (the first word of VideoX_nova, VideoX_dinamo is the
// product, the varying rest is whose it is). Words that start two or more
// names are products, never operators. Operators already set by patterns or
// fixed roots always count.
func logOperatorWords(files []monitorLogFile) map[string]bool {
	seen := map[string]bool{}
	names := []string{}
	for _, f := range files {
		for _, n := range logFileNames(f) {
			k := strings.ToLower(n)
			if !seen[k] {
				seen[k] = true
				names = append(names, k)
			}
		}
	}
	inNames := map[string]int{}
	notFirst := map[string]bool{}
	firstOfMany := map[string]int{}
	for _, n := range names {
		toks := logNameTokens(n)
		once := map[string]bool{}
		for i, t := range toks {
			if !once[t] {
				once[t] = true
				inNames[t]++
			}
			if i > 0 {
				notFirst[t] = true
			}
		}
		if len(toks) > 1 {
			firstOfMany[toks[0]]++
		}
	}
	ops := map[string]bool{}
	for t, n := range inNames {
		if n >= 2 && notFirst[t] && firstOfMany[t] < 2 && !logEnvWords[t] && !logGenericWords[t] {
			ops[t] = true
		}
	}
	for _, f := range files {
		if f.Operator != "" && !f.OpGuess {
			if k := logOperatorKey(f.Operator); k != "" {
				ops[k] = true
			}
		}
	}
	// A word that extends a known operator belongs to it even when it shows
	// up once (novatv next to nova).
	for t := range inNames {
		if ops[t] || firstOfMany[t] >= 2 || logEnvWords[t] || logGenericWords[t] {
			continue
		}
		for g := range ops {
			if len(g) >= 4 && len(t) > len(g) && strings.HasPrefix(t, g) {
				ops[t] = true
				break
			}
		}
	}
	return ops
}

// logOperatorKey is the operator's comparable form: lower case, without a
// trailing number.
func logOperatorKey(op string) string {
	k := strings.ToLower(strings.TrimSpace(op))
	if t := strings.TrimRightFunc(k, unicode.IsDigit); len(t) >= 3 {
		k = t
	}
	return k
}

// logOperatorGroups links operators where one name starts with another of at
// least four letters (nova, novatv, nova2 -> nova).
func logOperatorGroups(ops map[string]bool) map[string]string {
	keys := make([]string, 0, len(ops))
	for k := range ops {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) < len(keys[j]) || len(keys[i]) == len(keys[j]) && keys[i] < keys[j]
	})
	group := map[string]string{}
	for _, k := range keys {
		group[k] = k
		for _, g := range keys {
			if len(g) >= 4 && len(g) < len(k) && strings.HasPrefix(k, g) && group[g] == g {
				group[k] = g
				break
			}
		}
	}
	return group
}

// logDecorate fills Main, Place, OpGroup and, when guess is on, an operator
// guessed from repeated words for files that have none.
func logDecorate(files []monitorLogFile, mains []string, guess bool) {
	for i := range files {
		f := &files[i]
		if f.Kind == "journal" {
			f.Main, f.Place = "journal", "journal"
			continue
		}
		d := path.Dir(f.Path)
		f.Main = logMainFolder(d, mains)
		f.Place = logPlace(d, f.Main)
	}
	ops := map[string]bool{}
	if guess {
		ops = logOperatorWords(files)
	} else {
		for _, f := range files {
			if f.Operator != "" && !f.OpGuess {
				ops[logOperatorKey(f.Operator)] = true
			}
		}
	}
	group := logOperatorGroups(ops)
	for i := range files {
		f := &files[i]
		if f.Operator == "" && guess && f.Kind != "journal" {
		names:
			for _, n := range logFileNames(*f) {
				toks := logNameTokens(n)
				// The last word of a name is the likeliest owner (VideoX_nova_bkp
				// -> nova after skipping bkp).
				for j := len(toks) - 1; j >= 0; j-- {
					if ops[toks[j]] {
						f.Operator, f.OpGuess = toks[j], true
						break names
					}
				}
			}
		}
		if f.Operator != "" {
			k := logOperatorKey(f.Operator)
			if g, ok := group[k]; ok {
				f.OpGroup = g
			} else {
				f.OpGroup = k
			}
		}
	}
}

// logMainFolders are the folders the file tree starts from: the configured
// main folders, the scan paths and the Git workspace's server roots.
func logMainFolders(c Connection, s logSettings) []string {
	cat, _ := loadGitWorkspace(c.UserID)
	out := []string{}
	seen := map[string]bool{}
	for _, l := range [][]string{s.MainFolders, s.ScanPaths, cat.ServerRoots} {
		for _, p := range l {
			p = path.Clean(p)
			if logAbsDir(p) && !seen[p] && !strings.ContainsAny(p, "*?[") {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}
