// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// ─── ADDON SERVER CODE (WebAssembly) ─────────────────
//
// Server code runs in a WebAssembly sandbox through wazero: no CGO, no WASI
// (no files, no network, no environment, no clock), a 64 MiB memory ceiling and
// a five second limit per call. The only import an addon gets is wrm.log.
//
// ABI, kept small on purpose so an addon can be written by hand or by any
// compiler that targets wasm32:
//
//	wrm_alloc(size i32) -> ptr i32   the host writes the request JSON here
//	<export>(ptr i32, len i32) -> i32  a pointer to a NUL-terminated result
//
// The module must export its memory. Anything that does not fit this shape is
// refused at call time with a readable error; nothing executes at install time.

const (
	addonCallTimeout = 5 * time.Second
	addonMemoryPages = 1024 // 64 MiB, the wazero memory limit in 64 KiB pages
	addonInputLimit  = 64 << 10
	addonOutputLimit = 1 << 20
	addonLogLimit    = 4 << 10
)

var (
	addonRuntimeOnce sync.Once
	addonRuntimeInst wazero.Runtime
	addonRuntimeErr  error
)

// addonWASMRuntime is one runtime for the whole process. Its host module
// exposes only wrm.log, so a module that imports anything else (WASI included)
// fails to instantiate instead of reaching the host.
func addonWASMRuntime(ctx context.Context) (wazero.Runtime, error) {
	addonRuntimeOnce.Do(func() {
		cfg := wazero.NewRuntimeConfig().
			WithCloseOnContextDone(true).
			WithMemoryLimitPages(addonMemoryPages)
		rt := wazero.NewRuntimeWithConfig(ctx, cfg)
		if _, err := rt.NewHostModuleBuilder("wrm").
			NewFunctionBuilder().WithFunc(addonWASMLog).Export("log").
			Instantiate(ctx); err != nil {
			rt.Close(ctx)
			addonRuntimeErr = err
			return
		}
		addonRuntimeInst = rt
	})
	return addonRuntimeInst, addonRuntimeErr
}

// addonWASMLog is the one thing an addon may tell the host: a line for the
// server log, truncated so it cannot flood it.
func addonWASMLog(_ context.Context, m api.Module, ptr, n uint32) {
	if n > addonLogLimit {
		n = addonLogLimit
	}
	b, ok := m.Memory().Read(ptr, n)
	if !ok {
		return
	}
	log.Printf("addon: %s", strings.TrimSpace(string(b)))
}

// addonCallWASM runs one declared export of an enabled addon in a fresh module
// instance. The input is JSON written by the administrator; the result is
// whatever NUL-terminated UTF-8 string the export points at.
func addonCallWASM(ctx context.Context, rec addonRecord, export string, input []byte) (string, error) {
	if rec.Server == nil {
		return "", fmt.Errorf("this addon has no server code")
	}
	declared := false
	for _, e := range rec.Server.Exports {
		if e == export {
			declared = true
			break
		}
	}
	if !declared {
		return "", fmt.Errorf("the addon does not declare the export %q", export)
	}
	if len(input) > addonInputLimit {
		return "", fmt.Errorf("the input is larger than 64 KB")
	}
	if !addonSafeRel(rec.Server.Module) || strings.Contains(rec.Server.Module, "..") {
		return "", fmt.Errorf("the server module path is not valid")
	}
	file := filepath.Join(addonDir(rec.ID), filepath.FromSlash(rec.Server.Module))
	st, err := os.Stat(file)
	if err != nil || !st.Mode().IsRegular() {
		return "", fmt.Errorf("the server module is missing; install the addon again")
	}
	if st.Size() > addonMaxFile {
		return "", fmt.Errorf("the server module is larger than 8 MB")
	}
	bin, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("the server module cannot be read")
	}
	rt, err := addonWASMRuntime(ctx)
	if err != nil {
		return "", fmt.Errorf("the WebAssembly runtime is unavailable")
	}

	runCtx, cancel := context.WithTimeout(ctx, addonCallTimeout)
	defer cancel()
	code, err := rt.CompileModule(runCtx, bin)
	if err != nil {
		return "", fmt.Errorf("the server module is not valid WebAssembly: %v", err)
	}
	defer code.Close(context.Background())
	mod, err := rt.InstantiateModule(runCtx, code, wazero.NewModuleConfig())
	if err != nil {
		return "", fmt.Errorf("the server module cannot start: %v", err)
	}
	defer mod.Close(context.Background())
	return addonInvoke(runCtx, mod, export, input)
}

func addonInvoke(ctx context.Context, mod api.Module, export string, input []byte) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = "", fmt.Errorf("the addon failed")
		}
	}()
	mem := mod.Memory()
	if mem == nil {
		return "", fmt.Errorf("the server module must export its memory")
	}
	alloc := mod.ExportedFunction("wrm_alloc")
	fn := mod.ExportedFunction(export)
	if alloc == nil || fn == nil {
		return "", fmt.Errorf("the server module must export wrm_alloc and %s", export)
	}
	res, err := alloc.Call(ctx, uint64(len(input)))
	if err != nil {
		return "", addonCallError(ctx, err)
	}
	ptr := uint32(res[0])
	if len(input) > 0 && !mem.Write(ptr, input) {
		return "", fmt.Errorf("wrm_alloc returned a buffer outside memory")
	}
	res, err = fn.Call(ctx, uint64(ptr), uint64(len(input)))
	if err != nil {
		return "", addonCallError(ctx, err)
	}
	rp := uint32(res[0])
	size := mem.Size()
	if rp >= size {
		return "", fmt.Errorf("%s returned a pointer outside memory", export)
	}
	n := size - rp
	if n > addonOutputLimit {
		n = addonOutputLimit
	}
	b, ok := mem.Read(rp, n)
	if !ok {
		return "", fmt.Errorf("%s returned an unreadable result", export)
	}
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b), nil
}

// addonCallError keeps the one message that matters: a hook that loops is cut
// off by the context, not by the caller's patience.
func addonCallError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("the addon did not finish within %s", addonCallTimeout)
	}
	return fmt.Errorf("the addon failed: %v", err)
}

// addonCheckWASM compiles a server module without running it: it must export
// wrm_alloc, its memory and every function the manifest names, and the only
// import it may have is wrm.log. Compilation is linear in the module size and
// the size is already capped by addonMaxFile, so this is safe at install time.
func addonCheckWASM(ctx context.Context, m *addonManifest, dir string) error {
	if !addonSafeRel(m.Server.Module) || strings.Contains(m.Server.Module, "..") {
		return fmt.Errorf("the server module path is not valid")
	}
	file := filepath.Join(dir, filepath.FromSlash(m.Server.Module))
	st, err := os.Stat(file)
	if err != nil || !st.Mode().IsRegular() {
		return fmt.Errorf("%s is declared in addon.json but missing from the archive", m.Server.Module)
	}
	bin, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("%s cannot be read", m.Server.Module)
	}
	rt, err := addonWASMRuntime(ctx)
	if err != nil {
		return fmt.Errorf("the WebAssembly runtime is unavailable")
	}
	code, err := rt.CompileModule(ctx, bin)
	if err != nil {
		return fmt.Errorf("%s is not valid WebAssembly: %v", m.Server.Module, err)
	}
	defer code.Close(context.Background())
	for _, f := range code.ImportedFunctions() {
		if f.ModuleName() != "wrm" || f.Name() != "log" {
			return fmt.Errorf("the module imports %q; only wrm.log is available", f.ModuleName()+"."+f.Name())
		}
	}
	if len(code.ExportedMemories()) == 0 {
		return fmt.Errorf("the module must export its memory")
	}
	have := map[string]bool{}
	for name := range code.ExportedFunctions() {
		have[name] = true
	}
	if !have["wrm_alloc"] {
		return fmt.Errorf("the module must export wrm_alloc")
	}
	for _, e := range m.Server.Exports {
		if !have[e] {
			return fmt.Errorf("the module does not export %q", e)
		}
	}
	return nil
}

// addonCallHandler serves POST /api/addons/{id}/call/{export}. Administrator
// only: server code is not something an ordinary share should trigger.
func addonCallHandler(w http.ResponseWriter, r *http.Request, adminID int, id, export string) {
	rec, ok := addonGet(id)
	if !ok {
		jsonError(w, "Addon not found", 404)
		return
	}
	if !rec.Enabled {
		jsonError(w, "Enable the addon before running its code", 409)
		return
	}
	input, err := io.ReadAll(io.LimitReader(r.Body, addonInputLimit+1))
	if err != nil {
		jsonError(w, "The input cannot be read", 400)
		return
	}
	if len(input) > addonInputLimit {
		jsonError(w, "The input is larger than 64 KB", 413)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), addonCallTimeout+time.Second)
	defer cancel()
	out, err := addonCallWASM(ctx, rec, export, input)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	auditLog(r, adminID, "addon.call", id+"/"+export, map[string]interface{}{
		"version": rec.Version, "bytes_in": len(input), "bytes_out": len(out),
	})
	jsonOK(w, map[string]interface{}{"export": export, "output": out})
}
