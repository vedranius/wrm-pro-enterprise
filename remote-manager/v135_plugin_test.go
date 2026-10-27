// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The Claude plugin, its skills and the MCP Registry entry live outside the Go
// program; these tests keep them in step with the MCP tools WRM really offers.

func TestV135PluginManifests(t *testing.T) {
	for _, f := range []string{"../.claude-plugin/marketplace.json", "../packaging/claude-plugin/wrm-pro/.claude-plugin/plugin.json", "../server.json"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]interface{}
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	var srv struct {
		Name, Description, Version string
		Remotes                    []struct {
			Type, URL string
			Variables map[string]interface{}
		}
	}
	b, _ := os.ReadFile("../server.json")
	json.Unmarshal(b, &srv)
	if !strings.HasPrefix(srv.Name, "io.github.vedranius/") || len(srv.Description) > 100 || len(srv.Remotes) != 1 {
		t.Fatalf("server.json: %+v", srv)
	}
	if "v"+srv.Version != AppVersion {
		t.Fatalf("server.json version %q, app %q", srv.Version, AppVersion)
	}
	r := srv.Remotes[0]
	for _, m := range regexp.MustCompile(`\{(\w+)\}`).FindAllStringSubmatch(r.URL, -1) {
		if _, ok := r.Variables[m[1]]; !ok {
			t.Fatalf("server.json: URL variable %s is not described", m[1])
		}
	}
}

func TestV135PluginSkills(t *testing.T) {
	tools := map[string]bool{}
	g := &mcpGrant{Mode: aiModeAsk, Scopes: []string{"read_logs", "run_readonly", "run_with_approval", "edit_file_with_approval", "transfer"}}
	for _, tl := range mcpToolList(g) {
		tools[tl["name"].(string)] = true
	}
	notTools := map[string]bool{"session_id": true, "wrm_connect": true} // parameters the skills name; wrm_connect is the bridge's own tool
	if !tools["search_logs"] {
		t.Fatal("search_logs is not offered over MCP")
	}
	dirs, err := filepath.Glob("../packaging/claude-plugin/wrm-pro/skills/*")
	if err != nil || len(dirs) < 2 {
		t.Fatalf("skills: %v %v", dirs, err)
	}
	nameRe := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	toolRe := regexp.MustCompile("`([a-z]+(?:_[a-z]+)+)`")
	for _, d := range dirs {
		b, err := os.ReadFile(filepath.Join(d, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.HasPrefix(s, "---\n") {
			t.Fatalf("%s: no frontmatter", d)
		}
		end := strings.Index(s[4:], "\n---\n")
		if end < 0 {
			t.Fatalf("%s: frontmatter not closed", d)
		}
		meta := map[string]string{}
		for _, line := range strings.Split(s[4:4+end], "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok {
				meta[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		if meta["name"] != filepath.Base(d) || !nameRe.MatchString(meta["name"]) || len(meta["name"]) > 64 {
			t.Fatalf("%s: bad name %q", d, meta["name"])
		}
		if meta["description"] == "" || len(meta["description"]) > 1024 || strings.ContainsAny(meta["description"], "<>") {
			t.Fatalf("%s: bad description", d)
		}
		// every tool a skill names must exist
		for _, m := range toolRe.FindAllStringSubmatch(s, -1) {
			if !tools[m[1]] && !notTools[m[1]] {
				t.Errorf("%s names `%s`, which is not an MCP tool of WRM", d, m[1])
			}
		}
	}
}
