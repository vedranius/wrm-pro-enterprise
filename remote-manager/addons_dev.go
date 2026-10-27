// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── ADDON DEV SERVER ────────────────────────────────
//
// wrm -addon-dev <dir> serves one addon folder on 127.0.0.1 the way WRM runs
// it: the panel in a sandboxed frame with the real CSP, and a host page that
// stands in for WRM (context, store in memory, notifications, web requests,
// watchdog). Web requests follow the manifest exactly like WRM, with every
// listed origin treated as approved and private addresses refused unless
// -addon-dev-private is given; secrets come from WRM_ADDON_SECRET_<NAME>. The
// page reloads the panel when a file changes and shows the lint report.
//
// The dev server accepts requests only for its own host name and only with
// the per-run token the host page carries, so another web site open in the
// same browser cannot use it to send requests or read the store.

type addonDevServer struct {
	dir          string
	port         int
	allowPrivate bool
	token        string

	mu      sync.Mutex
	version string
	report  addonLintReport
	store   map[string]json.RawMessage
}

func addonDevToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// treeVersion hashes names, sizes and modification times of the folder.
func (d *addonDevServer) treeVersion() string {
	h := sha256.New()
	var lines []string
	_ = filepath.WalkDir(d.dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p != d.dir && strings.HasPrefix(e.Name(), ".") {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.IsDir() {
			return nil
		}
		if info, err := e.Info(); err == nil {
			lines = append(lines, p+"|"+strconv.FormatInt(info.Size(), 10)+"|"+strconv.FormatInt(info.ModTime().UnixNano(), 10))
		}
		return nil
	})
	sort.Strings(lines)
	for _, l := range lines {
		io.WriteString(h, l+"\n")
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// refresh re-lints when the folder changed; it returns the current state.
func (d *addonDevServer) refresh(ctx context.Context) (string, addonLintReport) {
	v := d.treeVersion()
	d.mu.Lock()
	if v == d.version {
		rep := d.report
		d.mu.Unlock()
		return v, rep
	}
	d.mu.Unlock()
	rep := addonLintPath(ctx, d.dir)
	d.mu.Lock()
	d.version, d.report = v, rep
	d.mu.Unlock()
	log.Printf("addon-dev: %s changed; %d error(s), %d warning(s)", d.dir, rep.Errors, rep.Warnings)
	return v, rep
}

func (d *addonDevServer) hostAllowed(r *http.Request) bool {
	host := strings.ToLower(r.Host)
	p := strconv.Itoa(d.port)
	return host == "127.0.0.1:"+p || host == "localhost:"+p || host == "[::1]:"+p
}

func (d *addonDevServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !d.hostAllowed(r) {
		http.Error(w, "unknown host", http.StatusMisdirectedRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	switch {
	case r.URL.Path == "/":
		d.serveHost(w)
	case strings.HasPrefix(r.URL.Path, "/addon/"):
		d.serveAsset(w, r, strings.TrimPrefix(r.URL.Path, "/addon/"))
	case strings.HasPrefix(r.URL.Path, "/__dev/"):
		if r.Header.Get("X-WRM-Dev") != d.token {
			http.Error(w, "missing dev token", http.StatusForbidden)
			return
		}
		d.serveAPI(w, r, strings.TrimPrefix(r.URL.Path, "/__dev/"))
	default:
		http.NotFound(w, r)
	}
}

func (d *addonDevServer) serveAsset(w http.ResponseWriter, r *http.Request, rel string) {
	ctype, ok := addonAssetType(rel)
	if !ok || !addonSafeRel(rel) || strings.HasPrefix(path.Base(rel), ".") || strings.Contains("/"+rel, "/.") {
		http.NotFound(w, r)
		return
	}
	root, err := os.OpenRoot(d.dir)
	if err != nil {
		http.Error(w, "the folder cannot be opened", 500)
		return
	}
	defer root.Close()
	f, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() || st.Size() > addonMaxFile {
		http.NotFound(w, r)
		return
	}
	b, err := io.ReadAll(io.LimitReader(f, addonMaxFile))
	if err != nil {
		http.Error(w, "read error", 500)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(ctype, "text/html") {
		w.Header().Set("Content-Security-Policy", addonPanelCSP(b))
	}
	w.Write(b)
}

func (d *addonDevServer) serveAPI(w http.ResponseWriter, r *http.Request, what string) {
	switch {
	case what == "state" && r.Method == http.MethodGet:
		v, rep := d.refresh(r.Context())
		out := map[string]interface{}{"version": v, "report": rep, "dir": d.dir, "private": d.allowPrivate}
		if m := rep.manifest; m != nil {
			out["manifest"] = m
			secrets := map[string]bool{}
			if m.HTTP != nil {
				for _, sp := range m.HTTP.secretSpecs() {
					secrets[sp.Name] = addonDevSecret(sp.Name) != ""
				}
			}
			out["secrets"] = secrets
		}
		jsonOK(w, out)
	case what == "store" && r.Method == http.MethodGet:
		d.mu.Lock()
		vals := map[string]json.RawMessage{}
		for k, v := range d.store {
			vals[k] = v
		}
		d.mu.Unlock()
		jsonOK(w, map[string]interface{}{"values": vals})
	case strings.HasPrefix(what, "store/") && (r.Method == http.MethodPut || r.Method == http.MethodDelete):
		key := strings.TrimPrefix(what, "store/")
		var raw json.RawMessage
		if r.Method == http.MethodPut {
			b, err := io.ReadAll(io.LimitReader(r.Body, addonStoreMaxWrite+1))
			if err != nil || len(b) > addonStoreMaxWrite {
				addonFail(w, addonErrf("E_LIMIT_SIZE", "A value is larger than 32 KB"), 413)
				return
			}
			raw = b
		}
		if err := addonStoreCheck(key, raw); r.Method == http.MethodPut && err != nil {
			addonFail(w, err, 400)
			return
		}
		d.mu.Lock()
		if r.Method == http.MethodDelete {
			delete(d.store, key)
		} else {
			d.store[key] = raw
		}
		d.mu.Unlock()
		jsonOK(w, map[string]interface{}{"ok": true})
	case what == "http" && r.Method == http.MethodPost:
		d.serveHTTPProxy(w, r)
	default:
		http.NotFound(w, r)
	}
}

func addonDevSecret(name string) string {
	return os.Getenv("WRM_ADDON_SECRET_" + strings.ToUpper(name))
}

// serveHTTPProxy sends a panel's request with the checks of the real proxy.
func (d *addonDevServer) serveHTTPProxy(w http.ResponseWriter, r *http.Request) {
	_, rep := d.refresh(r.Context())
	m := rep.manifest
	if m == nil || m.HTTP == nil {
		addonFail(w, addonErrf("E_CAPABILITY", "The addon does not declare the http capability"), 403)
		return
	}
	rec := addonRecord{ID: m.ID, Name: m.Name, Version: m.Version, Capabilities: m.Capabilities, HTTP: m.HTTP, Key: m.ID, Enabled: true}
	grants := map[string]addonHTTPGrant{}
	for _, o := range m.HTTP.Origins {
		grants[o] = addonHTTPGrant{Origin: o, Private: d.allowPrivate, ApprovedBy: "addon-dev"}
	}
	var req addonHTTPRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, addonHTTPMaxBody*2+16<<10)).Decode(&req); err != nil {
		addonFail(w, addonErrf("E_HTTP_REQUEST", "The request is not valid JSON"), 400)
		return
	}
	prep, status, err := addonHTTPPrepare(rec, grants, &req, addonDevSecret)
	if err != nil {
		log.Printf("addon-dev: %s %s refused: %s", addonDevLogText(req.Method), addonDevLogText(req.URL), addonDevLogText(err.Error()))
		addonFail(w, err, status)
		return
	}
	start := time.Now()
	res, err := addonHTTPDo(r.Context(), rec, grants, req, prep.u, prep.origin, prep.body, prep.auth)
	if err != nil {
		var ae *addonError
		code := "E_HTTP_UPSTREAM"
		if errors.As(err, &ae) {
			code = ae.Code
		}
		log.Printf("addon-dev: %s %s %s (%v)", addonDevLogText(req.Method), addonDevLogText(prep.origin+prep.logPath), code, time.Since(start).Round(time.Millisecond))
		addonFail(w, err, 502)
		return
	}
	log.Printf("addon-dev: %s %s %d %d bytes (%v)", addonDevLogText(req.Method), addonDevLogText(prep.origin+prep.logPath), res.Status, len(res.Body), time.Since(start).Round(time.Millisecond))
	jsonOK(w, res)
}

// addonDevLogText keeps panel-supplied text on one console line.
func addonDevLogText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

func (d *addonDevServer) serveHost(w http.ResponseWriter) {
	nonce := addonDevToken()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'nonce-"+nonce+"'; style-src 'unsafe-inline'; img-src 'self' data:; frame-src 'self'; connect-src 'self'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'")
	page := strings.NewReplacer("{{NONCE}}", nonce, "{{TOKEN}}", d.token, "{{VERSION}}", AppVersion).Replace(addonDevHostPage)
	io.WriteString(w, page)
}

// addonDevRun is wrm -addon-dev.
func addonDevRun(dir string, port int, allowPrivate bool) error {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("%s is not a folder", dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	d := &addonDevServer{dir: abs, port: port, allowPrivate: allowPrivate, token: addonDevToken(), store: map[string]json.RawMessage{}}
	_, rep := d.refresh(context.Background())
	fmt.Print(addonLintText(rep))
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	if port == 0 {
		d.port = ln.Addr().(*net.TCPAddr).Port
	}
	fmt.Printf("\nAddon dev server: http://127.0.0.1:%d/  (Ctrl+C stops it)\n", d.port)
	if allowPrivate {
		fmt.Println("Private and local addresses are allowed for the listed origins (-addon-dev-private).")
	}
	srv := &http.Server{Handler: d, ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(ln)
}

const addonDevHostPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>WRM addon dev</title>
<style>
:root { color-scheme: dark; --bg:#0f172a; --bg2:#111c33; --border:#22314f; --text:#e2e8f0; --text3:#94a3b8; --accent:#3b82f6; --red:#ef4444; --yellow:#f59e0b; --green:#22c55e; }
* { box-sizing: border-box; }
body { margin: 0; font: 13px system-ui, sans-serif; background: var(--bg); color: var(--text); display: grid; grid-template-columns: 1fr minmax(280px, 34%); grid-template-rows: auto 1fr; height: 100vh; }
@media (max-width: 800px) { body { grid-template-columns: 1fr; grid-template-rows: auto 1fr; height: auto; } }
header { grid-column: 1 / -1; display: flex; flex-wrap: wrap; gap: 8px; align-items: center; padding: 8px 12px; border-bottom: 1px solid var(--border); }
header b { margin-right: auto; }
button, select { background: var(--bg2); color: var(--text); border: 1px solid var(--border); border-radius: 6px; padding: 4px 10px; font: inherit; cursor: pointer; }
#stage { padding: 12px; overflow: auto; }
#win { border: 1px solid var(--border); border-radius: 8px; overflow: hidden; resize: both; background: var(--bg2); max-width: 100%; }
#win .bar { padding: 4px 8px; border-bottom: 1px solid var(--border); color: var(--text3); display: flex; gap: 8px; }
#win iframe { border: 0; width: 100%; height: calc(100% - 26px); display: block; background: transparent; }
#side { border-left: 1px solid var(--border); overflow: auto; padding: 8px 12px; }
h2 { font-size: 13px; margin: 12px 0 6px; color: var(--text3); text-transform: uppercase; letter-spacing: .04em; }
.d { padding: 4px 0; border-bottom: 1px solid var(--border); white-space: pre-wrap; word-break: break-word; }
.error { color: var(--red); } .warning { color: var(--yellow); } .info { color: var(--text3); } .ok { color: var(--green); }
#banner { display: none; margin: 0 0 8px; padding: 6px 10px; border: 1px solid var(--yellow); border-radius: 6px; color: var(--yellow); }
#toasts { position: fixed; right: 12px; bottom: 12px; display: flex; flex-direction: column; gap: 6px; }
#toasts div { background: var(--bg2); border: 1px solid var(--border); border-radius: 6px; padding: 6px 10px; max-width: 320px; }
</style>
</head>
<body>
<header>
  <b id="title">WRM addon dev</b>
  <select id="lang" aria-label="Language"><option value="en">English</option><option value="hr">Hrvatski</option></select>
  <select id="theme" aria-label="Theme"><option value="dark">Dark</option><option value="light">Light</option></select>
  <button id="vis">Hide window</button>
  <button id="reload">Reload</button>
</header>
<div id="stage">
  <p id="banner"></p>
  <div id="win"><div class="bar"><span id="wtitle"></span><span id="beat"></span></div><iframe id="frame" sandbox="allow-scripts" title="Addon panel"></iframe></div>
  <p class="info" id="nopanel" hidden>This addon has no panel; see the lint report.</p>
</div>
<aside id="side">
  <h2>Lint</h2><div id="lint"></div>
  <h2>Bridge</h2><div id="log"></div>
</aside>
<div id="toasts"></div>
<script nonce="{{NONCE}}">
(function () {
  'use strict';
  const TOKEN = '{{TOKEN}}';
  const $ = (id) => document.getElementById(id);
  const THEMES = {
    dark: {'--bg': '#0f172a', '--bg2': '#111c33', '--bg3': '#16223d', '--border': '#22314f', '--text': '#e2e8f0', '--text2': '#cbd5e1', '--text3': '#94a3b8', '--accent': '#3b82f6', '--green': '#22c55e', '--red': '#ef4444', '--yellow': '#f59e0b', '--purple': '#a855f7', '--font-ui': 'system-ui, sans-serif', '--font-mono': 'ui-monospace, monospace', '--radius': '6px'},
    light: {'--bg': '#ffffff', '--bg2': '#f1f5f9', '--bg3': '#e2e8f0', '--border': '#cbd5e1', '--text': '#0f172a', '--text2': '#334155', '--text3': '#64748b', '--accent': '#2563eb', '--green': '#16a34a', '--red': '#dc2626', '--yellow': '#d97706', '--purple': '#7c3aed', '--font-ui': 'system-ui, sans-serif', '--font-mono': 'ui-monospace, monospace', '--radius': '6px'}
  };
  let state = null, version = '', visible = true, ready = false, beat = 0, started = 0;
  const frame = $('frame');

  function api(method, p, body) {
    return fetch('/__dev/' + p, {method, headers: {'X-WRM-Dev': TOKEN, 'Content-Type': 'application/json'}, body}).then(async (r) => {
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw Object.assign(new Error(j.error || ('HTTP ' + r.status)), {code: j.code || ''});
      return j;
    });
  }
  function line(box, cls, text) {
    const d = document.createElement('div');
    d.className = 'd ' + cls;
    d.textContent = text;
    box.prepend(d);
    while (box.children.length > 200) box.lastChild.remove();
  }
  const log = (cls, text) => line($('log'), cls, new Date().toLocaleTimeString() + '  ' + text);
  function post(msg) { if (frame.contentWindow) frame.contentWindow.postMessage(Object.assign({v: 2}, msg), '*'); }
  function toast(text) {
    const d = document.createElement('div');
    d.textContent = text;
    $('toasts').appendChild(d);
    setTimeout(() => d.remove(), 6000);
  }
  function banner(text) { $('banner').textContent = text || ''; $('banner').style.display = text ? 'block' : 'none'; }

  function renderLint(rep) {
    const box = $('lint');
    box.textContent = '';
    (rep.diagnostics || []).slice().reverse().forEach((d) => line(box, d.level, d.level + '  ' + (d.file ? d.file + (d.line ? ':' + d.line : '') + '  ' : '') + d.message + ' [' + d.code + ']' + (d.hint && d.level !== 'info' ? '\n→ ' + d.hint : '')));
    line(box, rep.ok ? 'ok' : 'error', rep.ok ? 'OK, ' + rep.warnings + ' warning(s)' : rep.errors + ' error(s), ' + rep.warnings + ' warning(s)');
  }

  async function storeValues() { return (await api('GET', 'store')).values || {}; }

  async function context() {
    const m = state.manifest;
    const secretsReady = {};
    Object.keys(state.secrets || {}).forEach((k) => { secretsReady[k] = !!state.secrets[k]; });
    post({type: 'wrm:context', user: {id: 1, name: 'Developer'}, lang: $('lang').value, theme: THEMES[$('theme').value],
      addon: {id: m.id, key: m.id, version: m.version, instance: 1, instanceName: 'Dev'}, config: await storeValues(),
      features: {store: has('store'), notify: has('notify'), http: has('http'), jobs: false, history: false, visibility: true},
      http: m.http ? {origins: m.http.origins || [], approved: true, ready: secretsReady} : null,
      size: {w: frame.clientWidth, h: frame.clientHeight}, visible});
  }
  const has = (c) => !!(state && state.manifest && (state.manifest.capabilities || []).includes(c));

  function load() {
    const m = state && state.manifest;
    if (!m || !m.ui || !m.ui.panel) { $('win').hidden = true; $('nopanel').hidden = false; return; }
    $('win').hidden = false; $('nopanel').hidden = true;
    $('title').textContent = m.name + ' ' + m.version;
    $('wtitle').textContent = m.ui.title || m.name;
    const size = m.ui.defaultSize || {w: 480, h: 320};
    if (!$('win').dataset.sized) { $('win').style.width = size.w + 'px'; $('win').style.height = (size.h + 26) + 'px'; $('win').dataset.sized = '1'; }
    ready = false; beat = 0; started = Date.now(); banner('');
    frame.src = '/addon/' + m.ui.panel + '?instance=1&v=' + version;
  }

  window.addEventListener('message', async (e) => {
    if (e.source !== frame.contentWindow) return;
    const d = e.data;
    if (!d || d.v !== 2 || typeof d.type !== 'string') { log('warning', 'dropped: a message without v: 2'); return; }
    const fail = (type, err, extra) => post(Object.assign({type, id: d.id, error: err.message, code: err.code || ''}, extra || {}));
    switch (d.type) {
      case 'wrm:ready': ready = true; beat = Date.now(); banner(''); log('ok', 'wrm:ready after ' + (Date.now() - started) + ' ms'); return context();
      case 'wrm:heartbeat': beat = Date.now(); $('beat').textContent = '♥ ' + new Date().toLocaleTimeString(); return;
      case 'wrm:context': return context();
      case 'wrm:store.get':
      case 'wrm:store.set':
        if (!has('store')) { log('error', d.type + ' without the store capability'); return fail('wrm:store', {message: 'The addon does not declare the store capability', code: 'E_CAPABILITY'}, {key: d.key}); }
        try {
          if (d.type === 'wrm:store.get') {
            const v = (await storeValues())[d.key];
            post({type: 'wrm:store', id: d.id, key: d.key, value: v === undefined ? null : v});
          } else {
            await api('PUT', 'store/' + encodeURIComponent(d.key), JSON.stringify(d.value === undefined ? null : d.value));
            post({type: 'wrm:store', id: d.id, key: d.key, value: d.value});
          }
          log('info', d.type + ' ' + d.key);
        } catch (err) { log('error', d.type + ' ' + d.key + ': ' + err.code + ' ' + err.message); fail('wrm:store', err, {key: d.key}); }
        return;
      case 'wrm:notify':
        if (!has('notify')) return log('error', 'wrm:notify without the notify capability');
        toast(String(d.text || '').slice(0, 200)); log('info', 'notify: ' + d.text); return;
      case 'wrm:http':
        if (!has('http')) { log('error', 'wrm:http without the http capability'); return fail('wrm:http', {message: 'The addon does not declare the http capability', code: 'E_CAPABILITY'}, {ok: false}); }
        try {
          const res = await api('POST', 'http', JSON.stringify({method: d.method, url: d.url, headers: d.headers, body: typeof d.body === 'string' ? d.body : (d.body == null ? undefined : JSON.stringify(d.body)), encoding: d.encoding, secret: d.secret}));
          log(res.status < 400 ? 'ok' : 'warning', 'wrm:http ' + (d.method || 'GET') + ' ' + d.url + ' → ' + res.status + (res.code ? ' ' + res.code : ''));
          post(Object.assign({type: 'wrm:http', id: d.id}, res));
        } catch (err) { log('error', 'wrm:http ' + d.url + ': ' + err.code + ' ' + err.message); fail('wrm:http', err, {ok: false, status: 0}); }
        return;
      case 'wrm:resize': log('info', 'resize ' + d.w + '×' + d.h); $('win').style.width = Math.max(160, Number(d.w) || 0) + 'px'; $('win').style.height = Math.max(120, Number(d.h) || 0) + 26 + 'px'; return;
      case 'wrm:error': return log('error', 'panel error: ' + String(d.message || '') + (d.source ? ' (' + d.source + ':' + (d.line || 0) + ')' : ''));
      default: log('warning', 'unknown message dropped: ' + String(d.type).slice(0, 40));
    }
  });

  setInterval(() => {
    if (!state || !state.manifest || !state.manifest.ui || !state.manifest.ui.panel || !visible) return;
    if (!ready && Date.now() - started > 5000) banner('The panel did not send wrm:ready within 5 seconds (WRM would show "did not start").');
    else if (ready && Date.now() - beat > 45000) banner('No wrm:heartbeat for 45 seconds (WRM would show "not responding").');
  }, 1000);

  async function poll() {
    try {
      const s = await api('GET', 'state');
      if (s.version !== version) {
        version = s.version; state = s;
        renderLint(s.report);
        log('info', 'files changed: reloading');
        load();
      }
    } catch (err) { banner('The dev server does not answer: ' + err.message); }
    setTimeout(poll, 1000);
  }

  $('lang').addEventListener('change', context);
  $('theme').addEventListener('change', () => post({type: 'wrm:theme', theme: THEMES[$('theme').value]}));
  $('vis').addEventListener('click', () => {
    visible = !visible;
    $('vis').textContent = visible ? 'Hide window' : 'Show window';
    frame.style.visibility = visible ? 'visible' : 'hidden';
    post({type: 'wrm:visibility', visible});
    log('info', 'wrm:visibility ' + visible);
  });
  $('reload').addEventListener('click', load);
  poll();
})();
</script>
</body>
</html>
`

// runAddonLint is wrm -addon-lint.
func runAddonLint(p string, asJSON bool, out io.Writer) int {
	rep := addonLintPath(context.Background(), p)
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(rep)
	} else {
		io.WriteString(out, addonLintText(rep))
	}
	if !rep.OK {
		return 1
	}
	return 0
}

// runAddonNew is wrm -addon-new.
func runAddonNew(template, id, dir string, out io.Writer) int {
	if dir == "" {
		dir = template
	}
	files, err := addonTemplateWrite(template, id, dir)
	if err != nil {
		fmt.Fprintln(out, "-addon-new:", err)
		return 2
	}
	fmt.Fprintf(out, "Created %s from the %s template:\n", dir, template)
	for _, f := range files {
		fmt.Fprintln(out, "  "+f)
	}
	fmt.Fprintf(out, "Next: edit addon.json, then wrm -addon-lint %s and wrm -addon-dev %s\n", dir, dir)
	return 0
}

// runAddonDev is wrm -addon-dev.
func runAddonDev(dir string, port int, allowPrivate bool) int {
	if err := addonDevRun(dir, port, allowPrivate); err != nil {
		fmt.Fprintln(os.Stderr, "-addon-dev:", err)
		return 1
	}
	return 0
}

// addonDevAPIHandler serves /api/addons/dev/…: the manifest schema, llms.txt,
// the templates and lint for a ZIP. Lint unpacks and compiles but never runs
// anything, and is offered to whoever may install addons.
func addonDevAPIHandler(w http.ResponseWriter, r *http.Request, userID int, admin bool, parts []string) {
	what := strings.Join(parts, "/")
	switch {
	case what == "schema.json" && r.Method == http.MethodGet:
		b, _ := addonTemplatesFS.ReadFile("addon_templates/_docs/addon.schema.json")
		w.Header().Set("Content-Type", "application/schema+json")
		w.Write(b)
	case what == "llms.txt" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, addonLLMsText())
	case what == "templates" && r.Method == http.MethodGet:
		jsonOK(w, map[string]interface{}{"templates": addonTemplateList()})
	case strings.HasPrefix(what, "templates/") && r.Method == http.MethodGet:
		name, ok := addonTemplateName(strings.TrimPrefix(what, "templates/"))
		if !ok {
			jsonError(w, "Template not found", 404)
			return
		}
		id := r.URL.Query().Get("id")
		if id != "" && !addonIDPattern.MatchString(id) {
			addonFail(w, addonErrf("E_MANIFEST", "The addon id must be 2-64 characters: lowercase letters, digits, dot, dash or underscore"), 400)
			return
		}
		// A no-op for a valid id; it tells the XSS analysis that the id written
		// into the archive cannot carry markup.
		id = html.EscapeString(id)
		b, err := addonTemplateZip(name, id)
		if err != nil {
			addonFail(w, addonErrf("E_MANIFEST", "%v", err), 400)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", `attachment; filename="wrm-addon-`+name+`.zip"`)
		w.Write(b)
	case what == "lint" && r.Method == http.MethodPost:
		if !admin && !addonPersonalAllowed(userID) {
			addonFail(w, addonErrf("E_PERSONAL_OFF", "Personal addons are turned off by the administrator (addon_personal)"), 403)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, addonMaxArchive+1))
		if err != nil {
			jsonError(w, "Upload failed", 400)
			return
		}
		jsonOK(w, addonLintArchive(r.Context(), body))
	default:
		jsonError(w, "Not found", 404)
	}
}

func addonLLMsText() string {
	b, _ := addonTemplatesFS.ReadFile("addon_templates/_docs/llms.txt")
	return strings.ReplaceAll(string(b), "{{VERSION}}", AppVersion)
}
