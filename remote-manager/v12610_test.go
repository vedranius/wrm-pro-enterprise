// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// v12.6.10: the window area is one canvas. Addons are windows next to the terminals, panels can
// be placed anywhere on it, and the top bar groups Settings, Panels and Workboard.
func TestCanvasWired(t *testing.T) {
	page, _ := os.ReadFile("static/index.html")
	js, _ := os.ReadFile("static/workboard.js")
	lay, _ := os.ReadFile("static/layout.js")
	p, w, l := string(page), string(js), string(lay)
	for _, want := range []string{
		`id="tb-canvas-grp"`, `onclick="openToolsMenu(this)"`, `id="btn-shares"`, `id="hint-add-addon"`,
		"function winLayoutChanged(", "function canvasGrid(", "function arrangeWindows(", "function restoreWindowLayout(",
		"wrmWorkboard.closeAddon(wo)", "wrmWorkboard.windowGone(wo)", "wrmWorkboard.tabInfo(wo)", "wrmWorkboard.ctxItems(wo)",
		"w.mode !== 'addon' && (how === 'replace' || !w.mode)", // sessions never close addons
	} {
		if !strings.Contains(p, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	if strings.Contains(p, `id="btn-broadcast"`) {
		t.Error("broadcast is in the Tools menu now, not a top bar button")
	}
	// Settings, Panels and Workboard stay together, right of Keys.
	keys, grp := strings.Index(p, `id="btn-vault"`), strings.Index(p, `id="tb-canvas-grp"`)
	set, pan, wb := strings.Index(p, `id="btn-settings"`), strings.Index(p, `id="btn-panels"`), strings.Index(p, `id="btn-workboard"`)
	if keys < 0 || !(keys < grp && grp < set && set < pan && pan < wb) {
		t.Errorf("top bar order: keys %d group %d settings %d panels %d workboard %d", keys, grp, set, pan, wb)
	}
	for _, want := range []string{"addWindow({mode: 'addon'})", "wrmLayout.register", "restoreWindowLayout(", "captureLayout()", "wrm-workboard", "links:", "sessions:", "timer:"} {
		if !strings.Contains(w, want) {
			t.Errorf("workboard.js lacks %s", want)
		}
	}
	if strings.Contains(w, "id = 'workboard'") {
		t.Error("the Workboard overlay is gone: addons live on the canvas")
	}
	for _, want := range []string{"clampFloats, snapshot, apply, menu: openMenu", "(canvas() || document.body).appendChild(w)", ".dk-float { position: absolute;"} {
		if !strings.Contains(l, want) {
			t.Errorf("layout.js lacks %s", want)
		}
	}
}

// Saved canvas layouts need more room than one layout: the per-user UI store takes 256 KiB.
func TestUserUILargerWorkboard(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	a := newTestUser(t, srv, "cv-big", false)
	body := `{"value":{"tiles":[],"saved":[{"name":"x","note":"` + strings.Repeat("a", 150<<10) + `"}]}}`
	resp := a.do("PUT", "/api/me/ui/workboard", strings.NewReader(body), "application/json")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("150 KiB workboard: %d", resp.StatusCode)
	}
	body = `{"value":{"x":"` + strings.Repeat("a", userUIMaxBytes) + `"}}`
	resp = a.do("PUT", "/api/me/ui/workboard", strings.NewReader(body), "application/json")
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("over the limit: %d", resp.StatusCode)
	}
}
