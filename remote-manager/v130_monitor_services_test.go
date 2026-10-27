// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

// v13.0: a monitored server can watch systemd units and Docker containers. The
// names are validated before storage and the remote command is one quoted
// heredoc loop, so a name never becomes shell syntax.

func TestV130MonitorServiceConfig(t *testing.T) {
	c := monitorConfig{Mode: "ssh", Address: "127.0.0.1:10050", Interval: 60, TLS: "none",
		Services: []monitorService{{Name: " nginx "}, {Kind: "docker", Name: "web-1"}}}
	if err := c.normalize(); err != nil {
		t.Fatalf("a valid service list was refused: %v", err)
	}
	if c.Services[0].Kind != "systemd" || c.Services[0].Name != "nginx" {
		t.Fatalf("a bare name did not become a trimmed systemd unit: %+v", c.Services[0])
	}
	if c.Services[1].Kind != "docker" {
		t.Fatalf("the docker kind was not kept: %+v", c.Services[1])
	}
	bad := map[string][]monitorService{
		"shell":     {{Kind: "systemd", Name: "nginx; rm -rf /"}},
		"traversal": {{Kind: "systemd", Name: "../etc/passwd"}},
		"newline":   {{Kind: "docker", Name: "web\nnext"}},
		"kind":      {{Kind: "k8s", Name: "web"}},
		"empty":     {{Kind: "systemd", Name: ""}},
	}
	many := []monitorService{}
	for i := 0; i < 21; i++ {
		many = append(many, monitorService{Kind: "systemd", Name: "unit" + strings.Repeat("x", i%5)})
	}
	bad["too many"] = many
	for name, list := range bad {
		cfg := monitorConfig{Mode: "ssh", Address: "127.0.0.1:10050", Interval: 60, TLS: "none", Services: list}
		if err := cfg.normalize(); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestV130MonitorServiceScriptAndStates(t *testing.T) {
	script := monitorServiceScript([]monitorService{{Kind: "systemd", Name: "nginx"}, {Kind: "docker", Name: "web-1"}})
	for _, want := range []string{
		"<<'WRM_SERVICES'", "systemctl is-active", "docker inspect", "docker:web-1", "WRM_SERVICES\n",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("the service command lacks %q:\n%s", want, script)
		}
	}
	// The names are only data: they appear on their own lines, never in the
	// command that the shell parses.
	if strings.Contains(strings.SplitN(script, "<<'WRM_SERVICES'", 2)[0], "nginx") {
		t.Fatal("a service name leaked into the command line")
	}
	states := parseMonitorServiceStates("systemd:nginx=active\ndocker:web-1=running\n\njunk\n=x\n")
	if len(states) != 2 || states["systemd:nginx"] != "active" || states["docker:web-1"] != "running" {
		t.Fatalf("unexpected states: %+v", states)
	}
	if msg := monitorServiceProblem(states); msg != "" {
		t.Fatalf("healthy services reported a problem: %s", msg)
	}
	states["systemd:nginx"] = "failed"
	msg := monitorServiceProblem(states)
	if !strings.Contains(msg, "systemd:nginx=failed") {
		t.Fatalf("a failed unit was not reported: %s", msg)
	}
}

func TestV130MonitorServicesInUI(t *testing.T) {
	js, _ := os.ReadFile("static/monitoring.js")
	p := string(js)
	for _, want := range []string{
		"mon-c-services", "mon_services_watch", "wrmMonitoring.toggleEnabled", "s.services", "c.services",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("monitoring.js lacks %s", want)
		}
	}
}
