// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func draftViewByID(list []addonDraftView, id string) *addonDraftView {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

func draftFiles(id string) map[string]string {
	return map[string]string{
		"addon.json": addonDemoManifest(id),
		"panel.html": `<!doctype html><body><p>draft panel</p></body>`,
		"theme.css":  ":root{--accent:#654321}",
	}
}

func TestV133AddonDraftLifecycle(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "draft-admin", true)
	user := newTestUser(t, srv, "draft-user", false)
	id := "draft.panel"
	if _, err := addonDraftSave(context.Background(), id, draftFiles(id), false); err != nil {
		t.Fatalf("save draft: %v", err)
	}
	var list struct {
		Drafts []addonDraftView `json:"drafts"`
	}
	if code := admin.jsonDo("GET", "/api/addons/drafts", nil, &list); code != 200 {
		t.Fatalf("list drafts: %d", code)
	}
	v := draftViewByID(list.Drafts, id)
	if v == nil || !v.Valid || v.Installed || len(v.Files) != 3 {
		t.Fatalf("unexpected draft: %+v", v)
	}
	if code := user.jsonDo("GET", "/api/addons/drafts", nil, nil); code != 403 {
		t.Fatalf("a normal user listed drafts: %d", code)
	}
	var diff struct {
		Diff string `json:"diff"`
	}
	if code := admin.jsonDo("GET", "/api/addons/drafts/"+id+"/diff", nil, &diff); code != 200 {
		t.Fatalf("diff: %d", code)
	}
	if !strings.Contains(diff.Diff, "panel.html") || !strings.Contains(diff.Diff, "draft panel") {
		t.Fatalf("diff does not show the new panel: %q", diff.Diff)
	}
	// Installing a draft goes through the same checks as a ZIP, and a new addon
	// is never enabled by the install.
	var inst struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if code := admin.jsonDo("POST", "/api/addons/drafts/"+id+"/install", nil, &inst); code != 200 {
		t.Fatalf("install draft: %d", code)
	}
	rec, ok := addonGet(id)
	if !ok || rec.Enabled || inst.Enabled {
		t.Fatalf("a draft was enabled by install: %+v %+v", rec, inst)
	}
	resp := admin.do("GET", "/api/addons/"+id+"/asset/panel.html", nil, "")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("a disabled draft served its panel: %d", resp.StatusCode)
	}
	if code := admin.jsonDo("POST", "/api/addons/"+id+"/enable", nil, nil); code != 200 {
		t.Fatalf("enable: %d", code)
	}
	resp = admin.do("GET", "/api/addons/"+id+"/asset/panel.html", nil, "")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "draft panel") {
		t.Fatalf("installed draft panel: %d %q", resp.StatusCode, body)
	}
	// A second draft install keeps the addon disabled: nothing here enables it.
	if code := admin.jsonDo("POST", "/api/addons/"+id+"/disable", nil, nil); code != 200 {
		t.Fatalf("disable: %d", code)
	}
	files := draftFiles(id)
	files["panel.html"] = `<p>draft panel v2</p>`
	if _, err := addonDraftSave(context.Background(), id, files, true); err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if code := admin.jsonDo("POST", "/api/addons/drafts/"+id+"/install", nil, &inst); code != 200 || inst.Enabled {
		t.Fatalf("reinstall draft: %d %+v", code, inst)
	}
	if rec, _ := addonGet(id); rec.Enabled {
		t.Fatal("a draft reinstall enabled a disabled addon")
	}
	// Discarding only removes the workspace copy; the installed addon stays.
	if code := admin.jsonDo("DELETE", "/api/addons/drafts/"+id, nil, nil); code != 200 {
		t.Fatalf("discard: %d", code)
	}
	if _, ok := addonGet(id); !ok {
		t.Fatal("discarding a draft removed the installed addon")
	}
}

func TestV133AddonDraftValidation(t *testing.T) {
	if _, err := addonDraftSave(context.Background(), "draft.bad", map[string]string{
		"addon.json": addonDemoManifest("draft.bad"),
		"panel.html": "<p>x</p>",
		"theme.css":  "",
		"../evil":    "x",
	}, false); err == nil {
		t.Fatal("a draft accepted an escaping path")
	}
	if _, err := addonDraftSave(context.Background(), "Draft-Bad", draftFiles("draft.bad"), false); err == nil {
		t.Fatal("a draft accepted an invalid id")
	}
	id := "draft.valid"
	if _, err := addonDraftSave(context.Background(), id, draftFiles(id), false); err != nil {
		t.Fatal(err)
	}
	// A broken write is refused and leaves the previous draft untouched.
	if _, err := addonDraftSave(context.Background(), id, map[string]string{
		"addon.json": `{"id":"draft.valid","name":"","version":"","capabilities":[]}`,
	}, true); err == nil {
		t.Fatal("a broken manifest was accepted")
	}
	v := addonDraftViewOf(id)
	if !v.Valid || v.Version != "1.0.0" {
		t.Fatalf("a refused write damaged the draft: %+v", v)
	}
	if _, err := addonDraftSave(context.Background(), id, map[string]string{"panel.html": "<p>v2</p>"}, true); err != nil {
		t.Fatalf("merge write: %v", err)
	}
	if v := addonDraftViewOf(id); len(v.Files) != 3 {
		t.Fatalf("merge write lost files: %+v", v.Files)
	}
	if err := addonDraftRemove(id); err != nil {
		t.Fatal(err)
	}
}

func TestV133AddonToolScopeAndAdmin(t *testing.T) {
	for _, name := range []string{"addon_list", "addon_files", "addon_create", "addon_write", "addon_enable", "addon_disable", "addon_delete", "addon_run", "addon_install_community"} {
		spec, ok := aiToolSpecFor(name)
		if !ok || !spec.Host {
			t.Fatalf("%s is not a host addon tool", name)
		}
		if !aiToolInScopes(name, []string{"addons.manage"}) {
			t.Fatalf("%s does not accept addons.manage", name)
		}
		if aiToolInScopes(name, []string{"read_logs", "run_with_approval", "edit_file_with_approval"}) {
			t.Fatalf("%s is allowed by a normal scope", name)
		}
	}
	if !strings.Contains(mcpAllScopes, "addons.manage") {
		t.Fatal("addons.manage is not an MCP scope")
	}
	adminDefs := map[string]bool{}
	for _, d := range aiToolDefsForUser(aiModeAsk, true) {
		adminDefs[d.Name] = true
	}
	userDefs := map[string]bool{}
	for _, d := range aiToolDefsForUser(aiModeAsk, false) {
		userDefs[d.Name] = true
	}
	if !adminDefs["addon_create"] || userDefs["addon_create"] {
		t.Fatalf("addon tools are offered to the wrong users: admin=%v user=%v", adminDefs["addon_create"], userDefs["addon_create"])
	}
	e := newMCPEnv(t)
	q := mcpGrantRequest{ConnIDs: []int{e.connID}, Mode: aiModeReadOnly, Scopes: []string{"read_logs", "addons.manage"}}
	if err := q.validate(e.user.userID); err == nil || !strings.Contains(err.Error(), "administrators") {
		t.Fatalf("a non-administrator got addons.manage: %v", err)
	}
	adminConn := e.admin.addConnection("draft-scope-admin", e.host.addr)
	q = mcpGrantRequest{ConnIDs: []int{adminConn}, Mode: aiModeReadOnly, Scopes: []string{"read_logs", "addons.manage"}}
	if err := q.validate(e.admin.userID); err != nil {
		t.Fatalf("an administrator could not choose addons.manage: %v", err)
	}
}

func waitPendingApproval(t *testing.T, s *aiSession) *aiApproval {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		var found *aiApproval
		for _, a := range s.pending {
			found = a
			break
		}
		s.mu.Unlock()
		if found != nil {
			return found
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no approval was requested")
	return nil
}

func TestV133AddonToolApprovalLoop(t *testing.T) {
	e := newAIEnv(t, nil)
	adminConn := e.admin.addConnection("demo-admin", e.host.addr)
	code, v := e.admin.aiDo("POST", "/api/ai/sessions", map[string]interface{}{"conn_id": adminConn, "provider_id": e.provID, "mode": "ask"})
	if code != 200 {
		t.Fatalf("admin session: %d %v", code, v)
	}
	s := aiGet(int64(asMap(v)["id"].(float64)))
	if s == nil {
		t.Fatal("admin session not registered")
	}
	input, _ := json.Marshal(map[string]interface{}{
		"id": "ai.panel", "install": true, "reason": "test the addon loop",
		"files": map[string]string{
			"addon.json": addonDemoManifest("ai.panel"),
			"panel.html": "<p>from the assistant</p>",
			"theme.css":  ":root{--accent:#112233}",
		},
	})
	done := make(chan aiToolResult, 1)
	go func() {
		done <- s.CallTool(context.Background(), aiToolCall{ID: "c1", Name: "addon_create", Input: input})
	}()
	a := waitPendingApproval(t, s)
	if !strings.Contains(a.Command, "create addon draft ai.panel") || !strings.Contains(a.Diff, "panel.html") {
		t.Fatalf("approval does not show the draft: %+v", a)
	}
	// The approval is visible outside the AI panel too: the top-bar poll sees it
	// and the user's notification list records it.
	var active struct {
		Pending []struct {
			Session  int64  `json:"session"`
			Client   string `json:"client"`
			Approval struct {
				ID string `json:"id"`
			} `json:"approval"`
		} `json:"pending"`
	}
	if code := e.admin.jsonDo("GET", "/api/mcp/active", nil, &active); code != 200 {
		t.Fatalf("active: %d", code)
	}
	found := false
	for _, p := range active.Pending {
		if p.Approval.ID == a.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("a panel approval is not in the global pending list: %+v", active.Pending)
	}
	var events int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		db.QueryRow(`SELECT COUNT(*) FROM user_events WHERE user_id=? AND event='ai.approval'`, e.admin.userID).Scan(&events)
		if events > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if events == 0 {
		t.Fatal("the approval was not recorded in the user's notifications")
	}
	if err := s.Resolve(a.ID, e.admin.userID, true, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	res := <-done
	if res.IsError {
		t.Fatalf("addon_create failed: %s", res.Content)
	}
	rec, ok := addonGet("ai.panel")
	if !ok || rec.Enabled {
		t.Fatalf("the draft was installed enabled or not at all: %+v", rec)
	}
	if _, err := addonDraftManifest(addonDraftDir("ai.panel")); err != nil {
		t.Fatalf("the draft was not written: %v", err)
	}
	// In automatic mode with an allow-everything list, enabling an addon still
	// asks: the addon tools never run unattended.
	code, v = e.admin.aiDo("POST", "/api/ai/sessions", map[string]interface{}{
		"conn_id": adminConn, "provider_id": e.provID, "mode": "auto", "confirm_auto": true,
		"auto": map[string]interface{}{"allow": []string{"*"}, "path_allow": []string{"/**"}, "minutes": 10, "max_actions": 10},
	})
	if code != 200 {
		t.Fatalf("auto session: %d %v", code, v)
	}
	auto := aiGet(int64(asMap(v)["id"].(float64)))
	if auto == nil {
		t.Fatal("auto session not registered")
	}
	done = make(chan aiToolResult, 1)
	go func() {
		done <- auto.CallTool(context.Background(), aiToolCall{ID: "c2", Name: "addon_enable", Input: json.RawMessage(`{"id":"ai.panel","reason":"enable it"}`)})
	}()
	a = waitPendingApproval(t, auto)
	if !strings.Contains(a.Command, "enable addon ai.panel") {
		t.Fatalf("unexpected enable approval: %+v", a)
	}
	if err := auto.Resolve(a.ID, e.admin.userID, true, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if res := <-done; res.IsError {
		t.Fatalf("addon_enable failed: %s", res.Content)
	}
	if rec, _ := addonGet("ai.panel"); !rec.Enabled {
		t.Fatal("the approved enable did not run")
	}
	// A non-administrator cannot call the tool even directly.
	code, v = e.user.aiDo("POST", "/api/ai/sessions", map[string]interface{}{"conn_id": e.connID, "provider_id": e.provID, "mode": "ask"})
	if code != 200 {
		t.Fatalf("user session: %d %v", code, v)
	}
	us := aiGet(int64(asMap(v)["id"].(float64)))
	res = us.CallTool(context.Background(), aiToolCall{ID: "c3", Name: "addon_list", Input: json.RawMessage(`{}`)})
	if !res.IsError || !strings.Contains(res.Content, "administrator") {
		t.Fatalf("a non-administrator used an addon tool: %s", res.Content)
	}
}

func TestV133FilePathAlias(t *testing.T) {
	in, err := decodeToolInput([]byte(`{"file_path":"/tmp/example.conf","old_text":"a","new_text":"b","reason":"test"}`))
	if err != nil || in.Path != "/tmp/example.conf" {
		t.Fatalf("file_path alias: %v %+v", err, in)
	}
	in, err = decodeToolInput([]byte(`{"path":"/tmp/a","file_path":"/tmp/b","content":"x","reason":"test"}`))
	if err != nil || in.Path != "/tmp/a" {
		t.Fatalf("path must win over file_path: %v %+v", err, in)
	}
}

func TestV133ApprovalUX(t *testing.T) {
	ai, _ := os.ReadFile("static/ai.js")
	mcp, _ := os.ReadFile("static/mcp.js")
	page, _ := os.ReadFile("static/index.html")
	for _, want := range []string{`ai-pend-`, `pendingBar`, `wrmAiAlert('approval'`, `wrmSound('reply')`, `ai_sound_on`} {
		if !strings.Contains(string(ai), want) {
			t.Errorf("ai.js lacks %s", want)
		}
	}
	for _, want := range []string{`seenPending`, `wrmAiAlert('approval'`} {
		if !strings.Contains(string(mcp), want) {
			t.Errorf("mcp.js lacks %s", want)
		}
	}
	for _, want := range []string{`function wrmSound`, `function wrmAiAlert`, `wrmAskNotifications`, `setInterval(mcpRefresh, document.hidden ? 15000 : 5000)`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
}

func TestV134MCPQuickStartUI(t *testing.T) {
	js, _ := os.ReadFile("static/mcp.js")
	readme, _ := os.ReadFile("../README.md")
	m := string(js)
	for _, want := range []string{`mcp_quick_title`, `mcp-quick-cmd`, `wizClient('claude_code')`, `claude mcp add --transport http --scope user wrm`, `openSettings('st-mcp')`, `mcpw_cc_shell`, `mcpw_desk_no_plugin`, `mcp_quick_which`} {
		if !strings.Contains(m, want) {
			t.Errorf("mcp.js lacks %s", want)
		}
	}
	for _, want := range []string{`claude plugin install wrm-pro`, `claude mcp add --transport http --scope user wrm`, `/plugin install wrm-pro@wrm-pro`} {
		if !strings.Contains(string(readme), want) {
			t.Errorf("README lacks %s", want)
		}
	}
}

func TestV134VersionAndLockUI(t *testing.T) {
	page, _ := os.ReadFile("static/index.html")
	lock, _ := os.ReadFile("static/lock.js")
	p, l := string(page), string(lock)
	if strings.Contains(p, "v12.8.1") {
		t.Error("index.html still carries the old hard-coded version")
	}
	for _, want := range []string{`id="btn-lock"`, `setAppVersion`, `class="ver-txt"`, `wrmLock.lock()`} {
		if !strings.Contains(p, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	for _, want := range []string{`let b = $('btn-lock')`, `b.title = t('lock_now')`, `b.querySelector('.lbl')`} {
		if !strings.Contains(l, want) {
			t.Errorf("lock.js lacks %s", want)
		}
	}
}

func TestV133AddonDraftUI(t *testing.T) {
	js, _ := os.ReadFile("static/addons.js")
	page, _ := os.ReadFile("static/index.html")
	for _, want := range []string{
		`/api/addons/drafts`, `add_drafts`, `wrmAddons.draftDiff`, `wrmAddons.draftInstall`, `wrmAddons.draftDiscard`,
		`add_draft_install_q`, `add_draft_diff_title`,
	} {
		if !strings.Contains(string(js), want) {
			t.Errorf("addons.js lacks %s", want)
		}
	}
	if !strings.Contains(string(page), `data-tab="st-addons"`) || !strings.Contains(string(page), `id="addons-pane"`) {
		t.Error("the addons settings pane is not wired")
	}
	if strings.Contains(string(js), "allow-same-origin") {
		t.Error("a panel must never get allow-same-origin")
	}
}
