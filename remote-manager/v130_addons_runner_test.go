// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
)

// The sandbox tests use hand-written modules so no WebAssembly toolchain is
// needed to run them: a small builder writes the binary format directly.

func wasmLeb(n uint32) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n == 0 {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

func wasmSection(id byte, body []byte) []byte {
	out := append([]byte{id}, wasmLeb(uint32(len(body)))...)
	return append(out, body...)
}

func wasmExport(name string, kind byte, idx uint32) []byte {
	out := append(wasmLeb(uint32(len(name))), []byte(name)...)
	out = append(out, kind)
	return append(out, wasmLeb(idx)...)
}

// wasmConstModule is a module whose "status" returns the NUL-terminated result
// and whose wrm_alloc hands back a fixed input buffer at offset 32.
func wasmConstModule(result string) []byte {
	data := append([]byte(result), 0)
	b := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	types := []byte{0x02}
	types = append(types, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f) // (i32,i32)->i32
	types = append(types, 0x60, 0x01, 0x7f, 0x01, 0x7f)       // (i32)->i32
	b = append(b, wasmSection(1, types)...)
	b = append(b, wasmSection(3, []byte{0x02, 0x01, 0x00})...) // alloc(type 1), status(type 0)
	b = append(b, wasmSection(5, []byte{0x01, 0x00, 0x01})...) // one 64 KiB page
	ex := []byte{0x03}
	ex = append(ex, wasmExport("memory", 0x02, 0)...)
	ex = append(ex, wasmExport("wrm_alloc", 0x00, 0)...)
	ex = append(ex, wasmExport("status", 0x00, 1)...)
	b = append(b, wasmSection(7, ex)...)
	code := []byte{0x02}
	code = append(code, 0x04, 0x00, 0x41, 0x20, 0x0b) // return 64
	code = append(code, 0x04, 0x00, 0x41, 0x08, 0x0b) // return 8
	b = append(b, wasmSection(10, code)...)
	seg := []byte{0x01, 0x00, 0x41, 0x08, 0x0b}
	seg = append(seg, wasmLeb(uint32(len(data)))...)
	seg = append(seg, data...)
	b = append(b, wasmSection(11, seg)...)
	return b
}

// wasmSpinModule loops forever: the runtime must cut it off.
func wasmSpinModule() []byte {
	b := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	types := []byte{0x02}
	types = append(types, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f)
	types = append(types, 0x60, 0x01, 0x7f, 0x01, 0x7f)
	b = append(b, wasmSection(1, types)...)
	b = append(b, wasmSection(3, []byte{0x02, 0x01, 0x00})...)
	b = append(b, wasmSection(5, []byte{0x01, 0x00, 0x01})...)
	ex := []byte{0x03}
	ex = append(ex, wasmExport("memory", 0x02, 0)...)
	ex = append(ex, wasmExport("wrm_alloc", 0x00, 0)...)
	ex = append(ex, wasmExport("spin", 0x00, 1)...)
	b = append(b, wasmSection(7, ex)...)
	code := []byte{0x02}
	code = append(code, 0x04, 0x00, 0x41, 0x20, 0x0b)
	code = append(code, 0x08, 0x00, 0x03, 0x40, 0x0c, 0x00, 0x0b, 0x00, 0x0b) // loop { br 0 } unreachable
	b = append(b, wasmSection(10, code)...)
	return b
}

func wasmServerManifest(id, module string, exports string) string {
	return `{"id":"` + id + `","name":"WASM demo","version":"1.0.0","capabilities":["server.wasm"],"server":{"module":"` + module + `","exports":[` + exports + `]}}`
}

func TestV130AddonCallsWasmExport(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "wasm-admin", true)
	user := newTestUser(t, srv, "wasm-user", false)
	body := addonZip(t, map[string]string{
		"addon.json":  wasmServerManifest("demo.wasm", "plugin.wasm", `"status"`),
		"plugin.wasm": string(wasmConstModule("hello from wasm")),
	})
	resp := admin.do("POST", "/api/addons/install", bytes.NewReader(body), "application/zip")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("install: %d", resp.StatusCode)
	}
	call := func(c *testClient, export, input string) (int, map[string]interface{}) {
		r := c.do("POST", "/api/addons/demo.wasm/call/"+export, strings.NewReader(input), "application/json")
		defer r.Body.Close()
		raw, _ := io.ReadAll(r.Body)
		var out map[string]interface{}
		json.Unmarshal(raw, &out)
		return r.StatusCode, out
	}
	if code, _ := call(admin, "status", `{}`); code != 409 {
		t.Fatalf("a disabled addon ran its code: %d", code)
	}
	if code := admin.jsonDo("POST", "/api/addons/demo.wasm/enable", nil, nil); code != 200 {
		t.Fatalf("enable: %d", code)
	}
	if code, _ := call(user, "status", `{}`); code != 403 {
		t.Fatalf("a normal user ran server code: %d", code)
	}
	code, out := call(admin, "status", `{"q":"x"}`)
	if code != 200 || out["output"] != "hello from wasm" {
		t.Fatalf("call: %d %+v", code, out)
	}
	if code, out := call(admin, "other", `{}`); code != 400 || !strings.Contains(toString(out["error"]), "does not declare") {
		t.Fatalf("an undeclared export ran: %d %+v", code, out)
	}
}

func toString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func TestV130AddonWasmRejectsBadManifests(t *testing.T) {
	cases := map[string]addonManifest{
		"no capability": {ID: "x.y", Name: "x", Version: "1", Capabilities: []string{"ui.theme"},
			Server: &addonServer{Module: "p.wasm", Exports: []string{"status"}}},
		"no exports": {ID: "x.y", Name: "x", Version: "1", Capabilities: []string{"server.wasm"},
			Server: &addonServer{Module: "p.wasm"}},
		"not wasm": {ID: "x.y", Name: "x", Version: "1", Capabilities: []string{"server.wasm"},
			Server: &addonServer{Module: "p.bin", Exports: []string{"status"}}},
		"escaping module": {ID: "x.y", Name: "x", Version: "1", Capabilities: []string{"server.wasm"},
			Server: &addonServer{Module: "../p.wasm", Exports: []string{"status"}}},
		"bad export name": {ID: "x.y", Name: "x", Version: "1", Capabilities: []string{"server.wasm"},
			Server: &addonServer{Module: "p.wasm", Exports: []string{"run()"}}},
		"capability without server": {ID: "x.y", Name: "x", Version: "1",
			Capabilities: []string{"server.wasm"}},
	}
	for name, m := range cases {
		mm := m
		if err := mm.validate(); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	ok := addonManifest{ID: "x.y", Name: "x", Version: "1", Capabilities: []string{"server.wasm"},
		Server: &addonServer{Module: "p.wasm", Exports: []string{"status"}}}
	if err := ok.validate(); err != nil {
		t.Fatalf("a valid server addon was refused: %v", err)
	}
}

func TestV130AddonInstallChecksWasmModule(t *testing.T) {
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "wasm-check", true)
	install := func(body []byte) (int, string) {
		resp := admin.do("POST", "/api/addons/install", bytes.NewReader(body), "application/zip")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	missing := addonZip(t, map[string]string{
		"addon.json":  wasmServerManifest("demo.badwasm", "plugin.wasm", `"missing"`),
		"plugin.wasm": string(wasmConstModule("ok")),
	})
	if code, body := install(missing); code != 400 || !strings.Contains(body, "does not export") {
		t.Fatalf("a module without the declared export was accepted: %d %s", code, body)
	}
	broken := addonZip(t, map[string]string{
		"addon.json":  wasmServerManifest("demo.notwasm", "plugin.wasm", `"status"`),
		"plugin.wasm": "this is not wasm",
	})
	if code, body := install(broken); code != 400 || !strings.Contains(body, "not valid WebAssembly") {
		t.Fatalf("a broken module was accepted: %d %s", code, body)
	}
	if addonByID(addonRecords(false), "demo.badwasm") != nil {
		t.Fatal("a refused module left an addon record")
	}
	if _, err := os.Stat(addonDir("demo.notwasm")); err == nil {
		t.Fatal("a refused module left its folder")
	}
}

func TestV130AddonWasmTimeout(t *testing.T) {
	ctx := context.Background()
	rt, err := addonWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	code, err := rt.CompileModule(ctx, wasmSpinModule())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer code.Close(ctx)
	mod, err := rt.InstantiateModule(ctx, code, wazero.NewModuleConfig())
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	defer mod.Close(ctx)
	runCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := addonInvoke(runCtx, mod, "spin", nil); err == nil {
		t.Fatal("an endless hook returned")
	} else if !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("the timeout took %s", time.Since(start))
	}
}
