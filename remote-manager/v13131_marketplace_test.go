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

// TestV13131MarketplaceNames keeps the marketplace and plugin names in the form the Claude
// app's marketplace sync accepts, and the Claude app instructions on the lowercase repository
// name (the app names the marketplace after the repository).
func TestV13131MarketplaceNames(t *testing.T) {
	kebab := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	var mk struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	data, err := os.ReadFile(filepath.Join("..", ".claude-plugin", "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &mk); err != nil {
		t.Fatal(err)
	}
	if !kebab.MatchString(mk.Name) {
		t.Errorf("marketplace name %q is not kebab-case", mk.Name)
	}
	for _, p := range mk.Plugins {
		if !kebab.MatchString(p.Name) {
			t.Errorf("plugin name %q is not kebab-case", p.Name)
		}
		if !strings.HasPrefix(p.Source, "./") {
			t.Errorf("plugin source %q must start with ./", p.Source)
		}
	}
	var pl struct {
		Icon string `json:"icon"`
	}
	data, err = os.ReadFile(filepath.Join("..", "packaging", "claude-plugin", "wrm-pro", ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &pl); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pl.Icon, "./") {
		t.Errorf("plugin icon %q must start with ./", pl.Icon)
	}
	for _, f := range []string{
		filepath.Join("static", "mcp.js"),
		filepath.Join("mcpb", "server", "index.js"),
		filepath.Join("..", "packaging", "claude-plugin", "wrm-pro", "README.md"),
	} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(data)
		if !strings.Contains(s, "vedranius/wrm-pro-enterprise") {
			t.Errorf("%s: no Claude app instruction with the repository name", f)
		}
	}
}
