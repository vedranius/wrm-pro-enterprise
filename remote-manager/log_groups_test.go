// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLogNameTokens(t *testing.T) {
	for in, want := range map[string]string{
		"VideoX_NOVA2":       "videox nova",
		"INGEST_SFTP_novatv": "ingest sftp novatv",
		"demo-api.v2":        "demo api",
		"s3":                 "",
		"2026-10-22":         "",
	} {
		got := fmt.Sprint(logNameTokens(in))
		if got != "["+want+"]" {
			t.Errorf("logNameTokens(%q) = %s, want [%s]", in, got, want)
		}
	}
}

// A server where the same product runs for several operators: script folders
// under /home/scripts and log folders under /var/log share names that differ
// only in case, and operators show up as nova, NOVA2, novatv.
func TestLogDecorateOperatorsAndPlaces(t *testing.T) {
	mains := []string{"/var/log", "/home/scripts", "/home"}
	files := []monitorLogFile{
		{Path: "/var/log/VideoX_NOVA/info.txt", Service: "VideoX_NOVA"},
		{Path: "/var/log/VideoX_NOVA2/workers.out.log", Service: "VideoX_NOVA2"},
		{Path: "/var/log/VideoX_test/error.txt", Service: "VideoX_test"},
		{Path: "/var/log/VideoX/info.txt", Service: "VideoX"},
		{Path: "/home/scripts/Videox_NOVA/logs/run.log", Service: "Videox_NOVA"},
		{Path: "/home/scripts/Videox_DINAMO/logs/run.log", Service: "Videox_DINAMO"},
		{Path: "/home/scripts/dinamo/sync.log", Service: "dinamo"},
		{Path: "/home/scripts/novatv/logs/feed.log", Service: "novatv"},
		{Path: "/home/scripts/INGEST_SFTP_novatv/ingest.log", Service: "INGEST_SFTP_novatv"},
		{Path: "/home/scripts/.deploy/Videox_QA/history.jsonl", Service: "Videox_QA"},
		{Path: "/home/scripts/demo-api/logs/app.log", Service: "demo-api", Operator: "ops-team"},
		{Path: "journal:nginx", Kind: "journal", Unit: "nginx", Service: "nginx"},
	}
	logDecorate(files, mains, true)
	want := map[string][4]string{ // main, place, operator, group
		"/var/log/VideoX_NOVA/info.txt":                 {"/var/log", "logs", "nova", "nova"},
		"/var/log/VideoX_NOVA2/workers.out.log":         {"/var/log", "logs", "nova", "nova"},
		"/var/log/VideoX_test/error.txt":                {"/var/log", "logs", "", ""},
		"/var/log/VideoX/info.txt":                      {"/var/log", "logs", "", ""},
		"/home/scripts/Videox_NOVA/logs/run.log":        {"/home/scripts", "logs", "nova", "nova"},
		"/home/scripts/Videox_DINAMO/logs/run.log":      {"/home/scripts", "logs", "dinamo", "dinamo"},
		"/home/scripts/dinamo/sync.log":                 {"/home/scripts", "app", "dinamo", "dinamo"},
		"/home/scripts/novatv/logs/feed.log":            {"/home/scripts", "logs", "novatv", "nova"},
		"/home/scripts/INGEST_SFTP_novatv/ingest.log":   {"/home/scripts", "app", "novatv", "nova"},
		"/home/scripts/.deploy/Videox_QA/history.jsonl": {"/home/scripts", "app", "", ""},
		"/home/scripts/demo-api/logs/app.log":           {"/home/scripts", "logs", "ops-team", "ops-team"},
		"journal:nginx":                                 {"journal", "journal", "", ""},
	}
	for _, f := range files {
		w := want[f.Path]
		if f.Main != w[0] || f.Place != w[1] || f.Operator != w[2] || f.OpGroup != w[3] {
			t.Errorf("%s: main=%q place=%q op=%q group=%q, want %v", f.Path, f.Main, f.Place, f.Operator, f.OpGroup, w)
		}
		if f.Operator == "ops-team" && f.OpGuess {
			t.Error("an explicit operator was marked as guessed")
		}
	}
	// With guessing off only explicit operators stay.
	files2 := []monitorLogFile{{Path: "/var/log/VideoX_NOVA/info.txt", Service: "VideoX_NOVA"}, {Path: "/var/log/VideoX_NOVA2/a.log", Service: "VideoX_NOVA2"}, {Path: "/var/log/x/a.log", Service: "x", Operator: "Nova"}}
	logDecorate(files2, mains, false)
	if files2[0].Operator != "" || files2[2].OpGroup != "nova" {
		t.Errorf("guess off: %+v", files2)
	}
}

func TestLogLabelerMainFolderBeforeDefaults(t *testing.T) {
	s := defaultLogSettings()
	s.MainFolders = []string{"/home/scripts"}
	svc, op := newLogLabeler(s, s.ScanPaths...).label("/home/scripts/Videox_NOVA/logs")
	if svc != "Videox_NOVA" || op != "" {
		t.Errorf("main folder taken as operator: %q/%q", svc, op)
	}
	// Without the main folder the default /home/{operator}/{service} applies.
	if svc, op := newLogLabeler(defaultLogSettings()).label("/home/scripts/Videox_NOVA/logs"); svc != "Videox_NOVA" || op != "scripts" {
		t.Errorf("default pattern changed: %q/%q", svc, op)
	}
	bad := defaultLogSettings()
	bad.MainFolders = []string{"relative"}
	if bad.normalize() == nil {
		t.Error("relative main folder accepted")
	}
}

// The layout from a real report: a log folder with dated .txt copies, .err/.out
// worker logs and no "log" in most names is found completely, and a script
// folder with the same name is listed apart from it.
func TestLogDiscoveryLogFolderNextToScriptFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell")
	}
	h := startFakeHost(t, testSSHUser, testSSHPass)
	srv := newTestServer(t)
	u := newTestUser(t, srv, "log-groups", false)
	id := u.addConnection("demo-groups", h.addr)
	logs := filepath.Join(h.home, "varlog", "VideoX_test")
	scripts := filepath.Join(h.home, "scripts", "Videox_TEST")
	for _, d := range []string{logs, filepath.Join(scripts, "logs"), filepath.Join(h.home, "scripts", "novatv", "logs"), filepath.Join(h.home, "varlog", "VideoX_NOVA"), filepath.Join(h.home, "scripts", "INGEST_nova", "logs")} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	names := []string{"info.txt", "error.txt", "error.txt.1", "info.txt.2026-10-21", "info.txt.2026-10-22", "deleter_error.txt", "deleter_info.txt", "workers.err.log", "workers.out.log"}
	for _, n := range names {
		writeFile(t, filepath.Join(logs, n), "2026-10-23 14:40:00 INFO line\n")
	}
	writeFile(t, filepath.Join(scripts, "main.py"), "print('x')\n")
	writeFile(t, filepath.Join(scripts, "logs", "run.log"), "2026-10-23 14:40:00 INFO run\n")
	writeFile(t, filepath.Join(h.home, "scripts", "novatv", "logs", "feed.log"), "2026-10-23 14:40:00 INFO feed\n")
	writeFile(t, filepath.Join(h.home, "scripts", "INGEST_nova", "logs", "in.log"), "2026-10-23 14:40:00 INFO in\n")
	writeFile(t, filepath.Join(h.home, "varlog", "VideoX_NOVA", "info.txt"), "2026-10-23 14:40:00 INFO nova\n")
	writeFile(t, filepath.Join(h.home, "varlog", "VideoX_NOVA", "workers.out.log"), "2026-10-23 14:40:00 INFO nova\n")
	set := defaultLogSettings()
	set.ScanPaths = []string{filepath.Join(h.home, "varlog"), filepath.Join(h.home, "scripts")}
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/monitoring/logs/settings?id=%d", id), set, nil); code != 200 {
		t.Fatalf("settings: %d", code)
	}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/monitoring/logs/discover?id=%d", id), nil, nil); code != 200 {
		t.Fatalf("discover: %d", code)
	}
	var list struct {
		Files []monitorLogFile `json:"files"`
		Mains []string         `json:"main_folders"`
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/monitoring/logs?id=%d", id), nil, &list); code != 200 {
		t.Fatalf("list: %d", code)
	}
	got := map[string]monitorLogFile{}
	for _, f := range list.Files {
		got[f.Path] = f
	}
	for _, n := range names {
		f, ok := got[filepath.Join(logs, n)]
		if !ok {
			t.Errorf("%s not found", n)
			continue
		}
		if f.Service != "VideoX_test" || f.Main != filepath.Join(h.home, "varlog") {
			t.Errorf("%s: %+v", n, f)
		}
	}
	if f := got[filepath.Join(scripts, "logs", "run.log")]; f.Main != filepath.Join(h.home, "scripts") || f.Service != "Videox_TEST" {
		t.Errorf("script folder log: %+v", f)
	}
	if _, ok := got[filepath.Join(scripts, "main.py")]; ok {
		t.Error("script listed as a log")
	}
	if f := got[filepath.Join(h.home, "varlog", "VideoX_NOVA", "info.txt")]; f.Operator != "nova" || !f.OpGuess || f.OpGroup != "nova" {
		t.Errorf("operator not guessed: %+v", f)
	}
	if f := got[filepath.Join(h.home, "scripts", "novatv", "logs", "feed.log")]; f.OpGroup != "nova" {
		t.Errorf("novatv not grouped with nova: %+v", f)
	}
	if len(list.Mains) < 2 {
		t.Errorf("main folders missing: %v", list.Mains)
	}
}
