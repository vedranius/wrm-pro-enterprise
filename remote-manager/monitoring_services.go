// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── MONITORED SERVICES ──────────────────────────────
//
// A monitored service is a systemd unit or a Docker container whose state is
// read on every SSH poll, in the same round trip as the metrics. The command is
// one POSIX loop over a quoted heredoc, so a name cannot become shell syntax:
// the names are also validated against a strict pattern before they are stored.

var (
	monitorUnitPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@:\-]{0,199}$`)
	monitorContainerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]{0,63}$`)
)

// monitorServiceScript prints one "kind:name=state" line per service. A missing
// container is "missing", a failed unit is what systemctl reports.
func monitorServiceScript(list []monitorService) string {
	var b strings.Builder
	b.WriteString(`while IFS= read -r s; do `)
	b.WriteString(`case "$s" in `)
	b.WriteString(`systemd:*) n=${s#systemd:}; st=$(systemctl is-active -- "$n" 2>/dev/null) ;; `)
	b.WriteString(`docker:*) n=${s#docker:}; st=$(docker inspect -f '{{.State.Status}}' -- "$n" 2>/dev/null) || st=missing ;; `)
	b.WriteString(`*) st=unknown ;; esac; `)
	b.WriteString(`printf '%s=%s\n' "$s" "${st:-unknown}"; done <<'WRM_SERVICES'` + "\n")
	for _, x := range list {
		b.WriteString(x.Kind + ":" + x.Name + "\n")
	}
	b.WriteString("WRM_SERVICES\n")
	return b.String()
}

func parseMonitorServiceStates(out string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || !strings.Contains(k, ":") {
			continue
		}
		m[truncateStr(k, 260)] = truncateStr(strings.TrimSpace(v), 40)
	}
	return m
}

// monitorServiceProblem names the services that are not active/running.
func monitorServiceProblem(states map[string]string) string {
	bad := []string{}
	for k, v := range states {
		if v != "active" && v != "running" {
			bad = append(bad, k+"="+v)
		}
	}
	if len(bad) == 0 {
		return ""
	}
	sort.Strings(bad)
	if len(bad) > 5 {
		bad = append(bad[:5], "…")
	}
	return "service problem: " + strings.Join(bad, ", ")
}

// collectMonitorServices fills s.Services and escalates a healthy sample when a
// watched service is down, so the existing notification path reports it too.
func collectMonitorServices(ctx context.Context, cl *ssh.Client, list []monitorService, s *monitorSample) {
	runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := monitorSSHCommand(runCtx, cl, monitorServiceScript(list))
	if err != nil {
		s.Services = map[string]string{}
		for _, x := range list {
			s.Services[x.Kind+":"+x.Name] = "unknown"
		}
		s.Error = strings.TrimSpace(s.Error + "; service check failed")
		if s.State == "healthy" {
			s.State = "problem"
		}
		return
	}
	s.Services = parseMonitorServiceStates(out)
	for _, x := range list {
		k := x.Kind + ":" + x.Name
		if _, ok := s.Services[k]; !ok {
			s.Services[k] = "unknown"
		}
	}
	if msg := monitorServiceProblem(s.Services); msg != "" {
		if s.State == "healthy" {
			s.State = "problem"
		}
		if s.Error != "" {
			s.Error += "; "
		}
		s.Error += msg
	}
}
