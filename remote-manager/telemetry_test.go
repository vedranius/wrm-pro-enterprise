// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestTelemetryReportIsNonIdentifying(t *testing.T) {
	r := telemetryReport()
	if r["version"] != AppVersion {
		t.Fatalf("version = %v, want %s", r["version"], AppVersion)
	}
	// The report must carry only non-identifying fields.
	for k := range r {
		switch k {
		case "version", "os", "arch", "go", "uptime_h", "users", "connections", "sessions":
		default:
			t.Fatalf("unexpected (potentially identifying) field %q", k)
		}
	}
	if _, ok := r["os"]; !ok {
		t.Fatal("missing os")
	}
	if _, ok := r["arch"]; !ok {
		t.Fatal("missing arch")
	}
}

func TestSendTelemetryDisabled(t *testing.T) {
	if err := sendTelemetry(""); err != nil {
		t.Fatalf("empty endpoint should be a no-op, got %v", err)
	}
}
