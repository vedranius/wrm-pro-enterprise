// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// ─── OPT-IN TELEMETRY (RFC 0007) ─────────────────────────────────────
// Strictly opt-in and off by default. The payload is small and non-identifying: no
// hostnames, IPs, usernames, paths or secrets. See docs/rfcs/0007-telemetry.md.

var processStart = time.Now()

// telemetryReport builds the anonymous report. Keep this function small and reviewable:
// every field it sends is listed here.
func telemetryReport() map[string]interface{} {
	var users, connections, sessions int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE disabled=0`).Scan(&users)
	db.QueryRow(`SELECT COUNT(*) FROM connections`).Scan(&connections)
	db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions)
	return map[string]interface{}{
		"version":     AppVersion,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"go":          runtime.Version(),
		"uptime_h":    int(time.Since(processStart).Hours()),
		"users":       users,
		"connections": connections,
		"sessions":    sessions,
	}
}

// sendTelemetry posts one report to the configured endpoint (empty = disabled).
func sendTelemetry(endpoint string) error {
	if endpoint == "" {
		return nil
	}
	body, err := json.Marshal(telemetryReport())
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "wrm-telemetry/"+AppVersion)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// telemetryLoop sends one report at start-up and then daily, only when enabled.
func telemetryLoop() {
	for {
		if getSetting("telemetry") == "basic" {
			sendTelemetry(strings.TrimSpace(getSetting("telemetry_endpoint")))
		}
		time.Sleep(24 * time.Hour)
	}
}
