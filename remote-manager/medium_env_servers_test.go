// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// environments.yaml in the original deploy-tool also allows servers: [name, ...] per
// environment. The environment then limits the destinations a project may deploy to, and
// the project cannot lift that limit. This mirrors the Python tool's rule.
func TestMediumEnvironmentServers(t *testing.T) {
	envs := `environments:
  dev:
    servers: [app-01, app-02]
  prod: {}
`
	cfg, err := mdLoadConfig(mdValidConfig(t, map[string]string{"environments.yaml": envs}))
	if err != nil {
		t.Fatalf("valid environment server list rejected: %v", err)
	}
	if got := strings.Join(cfg.env("dev").Servers, ","); got != "app-01,app-02" {
		t.Fatalf("environment servers = %q, want app-01,app-02", got)
	}
	if cfg.env("prod").Servers != nil {
		t.Fatalf("prod without servers should allow every server, got %v", cfg.env("prod").Servers)
	}
}

func TestMediumEnvironmentServersErrors(t *testing.T) {
	cases := []struct {
		name string
		envs string
		want string
	}{
		{
			"empty list",
			"environments:\n  dev: {servers: []}\n  prod: {}\n",
			"empty list would forbid every server",
		},
		{
			"unknown server",
			"environments:\n  dev: {servers: [app-09]}\n  prod: {}\n",
			"is not defined in servers.yaml",
		},
		{
			"duplicate server",
			"environments:\n  dev: {servers: [app-01, app-01]}\n  prod: {}\n",
			"listed more than once",
		},
		{
			"destination outside restriction",
			"environments:\n  dev: {servers: [app-01]}\n  prod: {}\n",
			"is not allowed in environment",
		},
		{
			"wrong type",
			"environments:\n  dev: {servers: app-01}\n  prod: {}\n",
			"must be a list",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mdLoadConfig(mdValidConfig(t, map[string]string{"environments.yaml": tc.envs}))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}
