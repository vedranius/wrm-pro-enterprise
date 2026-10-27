// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

// v13.1: a replay shows the wall-clock time of what it plays, not only the
// offset from the start. The cast header already carries the Unix start time,
// so both the asciicast player and the Guacamole player add the playhead to it.
func TestV130ReplayShowsWallClock(t *testing.T) {
	page, _ := os.ReadFile("static/index.html")
	rec, _ := os.ReadFile("recording.go")
	p, r := string(page), string(rec)
	for _, want := range []string{
		`id="rp-wall"`, "function replayWall(", "replay.startMs = ", "rp_ended",
		"replayWall(replay.pos)", "replayWall(pos / 1000)",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	// The absolute start must exist in the recording itself, otherwise the
	// player has nothing to add the playhead to.
	if !strings.Contains(r, `json:"timestamp"`) || !strings.Contains(r, "now.Unix()") {
		t.Error("the asciicast header no longer records the absolute start time")
	}
}
