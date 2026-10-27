//go:build android

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
)

// The built-in TURN relay depends on a network library that cannot be linked for Android
// (Termux) with current Go versions. Voice calls still work peer-to-peer, and external
// STUN/TURN servers can be configured in Admin → Voice (ice_servers).

func startTURN()   {}
func stopTURN()    {}
func restartTURN() {}

func turnStatus() map[string]interface{} {
	return map[string]interface{}{"enabled": false, "running": false, "error": "the built-in TURN relay is not available on Android — configure an external TURN server"}
}

func iceServersFor(r *http.Request, pid string) []map[string]interface{} {
	out := []map[string]interface{}{}
	var configured []map[string]interface{}
	if json.Unmarshal([]byte(getSetting("ice_servers")), &configured) == nil {
		out = append(out, configured...)
	}
	return out
}
