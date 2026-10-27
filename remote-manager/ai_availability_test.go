// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAISFTPCompatibleProviderWithoutMCP(t *testing.T) {
	e := newAIEnv(t, func(n int, body map[string]interface{}) fakeTurn {
		if n == 0 {
			return fakeTurn{Calls: []fakeCall{{"system_info", `{}`}}}
		}
		return fakeTurn{Text: "Server checked."}
	})
	t.Setenv("WRM_AI_MCP_ENABLED", "0")
	db.Exec(`UPDATE connections SET protocol='SFTP' WHERE id=?`, e.connID)
	code, v := e.user.aiDo("GET", fmt.Sprintf("/api/ai/connections/%d", e.connID), nil)
	if code != 200 || asMap(v)["ssh"] != true {
		t.Fatalf("SFTP excluded: %d %v", code, v)
	}
	code, v = e.admin.aiDo("POST", "/api/admin/ai/providers", map[string]interface{}{"name": "Demo local provider", "kind": "openai_compatible", "base_url": e.llm.srv.URL + "/v1", "models": "local-model"})
	if code != 200 {
		t.Fatalf("provider: %d %v", code, v)
	}
	e.provID = int(asMap(v)["id"].(float64))
	s := e.start("read_only", nil)
	e.prompt(s, "check server")
	e.waitIdle(s)
	if e.llm.requests() != 2 {
		t.Fatalf("tool loop not run: %d", e.llm.requests())
	}
	ch := hub.subscribe(e.user.userID)
	defer hub.unsubscribe(ch)
	code, v = e.admin.aiDo("PUT", fmt.Sprintf("/api/admin/ai/users/%d", e.user.userID), map[string]interface{}{"blocked": true})
	if code != 200 {
		t.Fatalf("block: %d %v", code, v)
	}
	found := false
	for len(ch) > 0 {
		var event map[string]interface{}
		json.Unmarshal(<-ch, &event)
		if event["type"] == "ai_policy_changed" {
			found = true
		}
	}
	if !found {
		t.Fatal("other window did not receive policy refresh event")
	}
	code, v = e.user.aiDo("GET", "/api/ai/config", nil)
	if code != 200 || asMap(v)["enabled"] != false || !strings.Contains(fmt.Sprint(asMap(v)["reason"]), "account") {
		t.Fatalf("no account blocker diagnostic: %v", v)
	}
}

func TestAIPolicyChangeBroadcastAndConfigDiagnostic(t *testing.T) {
	t.Setenv("WRM_AI_ASSISTANT", "")
	t.Setenv("WRM_AI_KILL_SWITCH", "0")
	saved := getSetting("ai_assistant")
	defer setSetting("ai_assistant", saved)
	setSetting("ai_assistant", "off")
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	admin := newTestUser(t, srv, "availability-admin", true)
	user := newTestUser(t, srv, "availability-user", false)
	code, v := user.aiDo("GET", "/api/ai/config", nil)
	if code != 200 || asMap(v)["enabled"] != false || asMap(v)["reason"] == "" {
		t.Fatalf("disabled diagnostic: %v", v)
	}
	ch := hub.subscribe(user.userID)
	defer hub.unsubscribe(ch)
	if code := admin.jsonDo("PUT", "/api/admin/settings", map[string]string{"ai_assistant": "all"}, nil); code != 200 {
		t.Fatal(code)
	}
	select {
	case event := <-ch:
		if !strings.Contains(string(event), "ai_policy_changed") {
			t.Fatalf("event: %s", event)
		}
	default:
		t.Fatal("no cross-window policy update")
	}
	var me map[string]interface{}
	user.jsonDo("GET", "/api/auth/me", nil, &me)
	if asMap(me["policy"])["ai"] != true {
		t.Fatalf("effective policy stale: %v", me)
	}
}
