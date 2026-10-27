// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
)

func testVersion(commit, tag, date string) gitTargetVersion {
	return gitTargetVersion{Commit: commit, CommitFull: commit + "0000000000", Date: date, Tag: tag}
}

func testHistory(version int, hash, tag string) gitHistEntry {
	return gitHistEntry{Commit: tag, CommitFull: tag + "0000000000", Version: version, Date: "2026-01-0" + string(rune('1'+version)), Hash: hash, Tag: tag}
}

func testTarget(files map[string]gitTargetFile) gitTarget {
	return gitTarget{Version: "v3", Commit: "ccc", CommitFull: "ccc0000000000", CommitDate: "2026-01-03",
		Versions: []gitTargetVersion{
			testVersion("ccc", "v3", "2026-01-03"),
			testVersion("bbb", "v2", "2026-01-02"),
			testVersion("aaa", "v1", "2026-01-01"),
		},
		Files: files,
	}
}

func TestDetectInstallVersion(t *testing.T) {
	files := map[string]gitTargetFile{
		"a.py": {Hash: "a3", Oldest: 2, History: []gitHistEntry{
			testHistory(0, "a3", "v3"), testHistory(1, "a2", "v2"), testHistory(2, "a1", "v1"),
		}},
		"b.py": {Hash: "b3", Oldest: 2, History: []gitHistEntry{
			testHistory(0, "b3", "v3"),
		}},
		"config.ini": {Hash: "cfg", Oldest: 2, History: []gitHistEntry{testHistory(0, "cfg", "v3")}},
	}
	tg := testTarget(files)
	tg.Protected = []string{"config*"}
	server := map[string]string{"a.py": "a2", "b.py": "b3", "config.ini": "different"}

	got := detectInstallVersion(tg, server, nil)
	if got.State != "exact" || got.Version != "v2" || got.Commit != "bbb0000000000" || got.Date != "2026-01-02" || got.Approx {
		t.Fatalf("v2: %+v", got)
	}

	server["a.py"] = "a3"
	if got := detectInstallVersion(tg, server, nil); got.State != "exact" || got.Version != "v3" {
		t.Fatalf("target: %+v", got)
	}

	server["a.py"] = "a1"
	if got := detectInstallVersion(tg, server, nil); got.State != "exact" || got.Version != "v1" || got.Commit != "aaa0000000000" {
		t.Fatalf("v1: %+v", got)
	}

	// one file from v2 and one from v1: no single version
	server["a.py"], server["b.py"] = "a2", "b1"
	if got := detectInstallVersion(tg, server, []gitFileState{
		{Path: "a.py", State: "old", Tag: "v2"},
		{Path: "b.py", State: "modified"},
	}); got.State != "modified" {
		t.Fatalf("modified: %+v", got)
	}
	if got := detectInstallVersion(tg, server, []gitFileState{
		{Path: "a.py", State: "old", Tag: "v2"},
		{Path: "b.py", State: "old", Tag: "v1"},
	}); got.State != "mixed" {
		t.Fatalf("mixed: %+v", got)
	}

	// a missing file never matches a version
	delete(server, "b.py")
	if got := detectInstallVersion(tg, server, []gitFileState{{Path: "a.py", State: "old", Tag: "v2"}, {Path: "b.py", State: "missing"}}); got.State != "missing" {
		t.Fatalf("missing: %+v", got)
	}

	// a file added after a version makes that version impossible
	files["new.py"] = gitTargetFile{Hash: "n3", Oldest: 0, History: []gitHistEntry{testHistory(0, "n3", "v3")}}
	tg = testTarget(files)
	server = map[string]string{"a.py": "a1", "b.py": "b3", "new.py": "n3"}
	if got := detectInstallVersion(tg, server, nil); got.State == "exact" && got.Version == "v1" {
		t.Fatalf("new file matched an old version: %+v", got)
	}
}

func TestDetectInstallVersionFallback(t *testing.T) {
	// old targets and bundles without version trees: all files at the target
	tg := gitTarget{Version: "v2", Commit: "c2", CommitFull: "c20000000000", CommitDate: "2026-02-02",
		Files: map[string]gitTargetFile{"a.py": {Hash: "a2", History: []gitHistEntry{{Commit: "c2", Hash: "a2", Tag: "v2"}}}}}
	got := detectInstallVersion(tg, map[string]string{"a.py": "a2"}, []gitFileState{{Path: "a.py", State: "ok"}})
	if got.State != "exact" || got.Version != "v2" || got.Commit != "c20000000000" || got.Approx {
		t.Fatalf("fallback target: %+v", got)
	}

	// one changed tag on all old files: the best-effort older version
	tg.Files["a.py"] = gitTargetFile{Hash: "a2", History: []gitHistEntry{
		{Commit: "c2", Hash: "a2", Tag: "v2"},
		{Commit: "c1", CommitFull: "c10000000000", Hash: "a1", Tag: "v1", Date: "2026-02-01"},
	}}
	got = detectInstallVersion(tg, map[string]string{"a.py": "a1"}, []gitFileState{{Path: "a.py", State: "old", Behind: 1, Tag: "v1"}})
	if got.State != "exact" || got.Version != "v1" || got.Commit != "c10000000000" || !got.Approx {
		t.Fatalf("fallback old: %+v", got)
	}

	// more than one old tag is mixed, not a guess
	got = detectInstallVersion(tg, map[string]string{"a.py": "a1"}, []gitFileState{
		{Path: "a.py", State: "old", Tag: "v1"}, {Path: "b.py", State: "old", Tag: "v0"},
	})
	if got.State != "mixed" {
		t.Fatalf("fallback mixed: %+v", got)
	}

	// only protected files: nothing to conclude
	tg = gitTarget{Version: "v2", Files: map[string]gitTargetFile{"config.ini": {Hash: "x"}}, Protected: []string{"config*"}}
	if got := detectInstallVersion(tg, map[string]string{"config.ini": "y"}, []gitFileState{{Path: "config.ini", State: "protected"}}); got.State != "unknown" {
		t.Fatalf("protected only: %+v", got)
	}
}

func TestFileHashAtVersion(t *testing.T) {
	f := gitTargetFile{Hash: "a3", Oldest: 2, History: []gitHistEntry{
		testHistory(0, "a3", "v3"), testHistory(2, "a1", "v1"),
	}}
	for i, want := range map[int]string{0: "a3", 1: "a3", 2: "a1"} {
		got, _, ok := fileHashAtVersion(f, i)
		if !ok || got != want {
			t.Fatalf("version %d: %q %v, want %q", i, got, ok, want)
		}
	}
	if _, _, ok := fileHashAtVersion(f, 3); ok {
		t.Fatal("a file that did not exist in the oldest version must not match")
	}
}

func TestTrackedStateUsesVersionIndex(t *testing.T) {
	f := gitTargetFile{Hash: "a3", History: []gitHistEntry{
		testHistory(0, "a3", "v3"), testHistory(3, "a1", "v0"),
	}}
	state, behind, tag, _ := trackedState(f, "a1")
	if state != "old" || behind != 3 || tag != "v0" {
		t.Fatalf("state=%s behind=%d tag=%s", state, behind, tag)
	}
}
