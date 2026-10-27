# Writing WRM PRO addons (apiVersion 2)

An addon is a ZIP with `addon.json` at its root (or one folder down) and the files it declares. Nothing runs at install time: WRM checks the manifest, the paths and the declared files, pins the SHA-256 of every file, and installs the addon **disabled**. Enabling it shows a permission sheet that WRM generates from `addon.json`.

A manifest without `apiVersion` is a v1 addon and keeps working as before. This page describes **apiVersion 2**, available since v13.9.0.

## Who installs what

- **Organisation addons** are installed by an administrator for everybody (*Settings → Addons*, *Install for: Everybody*). They may carry WebAssembly server code (`server.wasm`).
- **Personal addons** can be installed by any user the policy `addon_personal` allows (default: everybody). Only the user who installed one sees it. They run in the same sandbox and cannot carry server code. Administrators see every user's personal addons and can turn one off or remove it.

## addon.json

```json
{
  "apiVersion": 2,
  "id": "demo.counter",
  "name": "Demo counter",
  "version": "1.0.0",
  "description": "Counts clicks and keeps the count on the server.",
  "author": "Your name",
  "license": "MIT",
  "homepage": "https://example.com/addon",
  "capabilities": ["ui.panel", "ui.theme", "store", "notify", "http"],
  "ui": {
    "panel": "panel.html",
    "title": "Counter",
    "defaultSize": { "w": 420, "h": 260 },
    "minSize": { "w": 300, "h": 180 },
    "themes": [
      { "id": "ocean", "name": "Ocean", "name_hr": "Ocean", "mode": "dark",
        "tokens": { "--bg": "#0b1220", "--accent": "#38bdf8" }, "css": "ocean.css" }
    ]
  },
  "store": { "quotaKb": 16, "shared": false },
  "http": {
    "origins": ["https://grafana.example.com"],
    "paths": ["/api/health", "/api/dashboards/*"],
    "methods": ["GET"],
    "secrets": ["grafana_token"]
  }
}
```

Rules:

- **Unknown fields are rejected** (`E_MANIFEST_FIELD`), including nested ones, so a misspelt field never silently grants nothing.
- `id`: 2-64 characters, lowercase letters, digits, `.`, `-`, `_`. `name` 1-80, `version` 1-32 characters. Versions compare as dotted numbers (`1.10.0` is newer than `1.9.3`). A lower version than the installed one needs an explicit confirmation (`E_DOWNGRADE`).
- `capabilities` is the grant list. A block without its capability, or a capability this WRM does not know, refuses the install.

| Capability | What it allows |
|---|---|
| `ui.panel` | a window inside WRM (`ui.panel`, `ui.title`, `ui.defaultSize`, `ui.minSize`) |
| `ui.theme` | colour themes users pick in *Settings → Appearance* (`ui.themes`, or the v1 `ui.theme` stylesheet) |
| `store` | settings kept on the WRM server per instance (`store.quotaKb` 1-1024, default 64; `store.shared`) |
| `notify` | short notifications from the panel |
| `http` | requests to the web sites in `http`, sent by WRM after an administrator approved them (see [Web sites and secrets](#web-sites-and-secrets)) |
| `server.wasm` | WebAssembly server code, organisation addons only (see the v1 documentation in ARCHITECTURE.md) |

`store`, `notify` and `http` need `apiVersion: 2` and a panel.

### Themes

`ui.themes` holds up to 16 themes. Each has an `id` (lowercase letters, digits, dash, up to 32), `name` and optional `name_hr` (up to 60 characters), `mode` (`light` or `dark`) and `tokens` and/or `css`:

- `tokens` are CSS variables of WRM: `--bg --bg2 --bg3 --bg4 --bg5 --border --border2 --text --text2 --text3 --accent --green --red --yellow --purple --font-ui --font-mono --radius` and the terminal palette. Names must look like `--name`; values are plain values (colours, lengths, font lists) without `;`, `{`, `}`, `<`, `>`, `url(`, `@import` or `expression`.
- `css` is a stylesheet in the ZIP.

A theme from an addon is never applied to everybody: it appears in *Settings → Appearance* and is used by the people who pick it.

## The panel

The panel is an HTML page in an `iframe` with `sandbox="allow-scripts"`, so it has an opaque origin: it cannot read WRM, its cookies, its storage or another panel. apiVersion 2 panels are served with this Content Security Policy:

```
sandbox allow-scripts; default-src 'none'; script-src 'self' 'sha256-<each inline script>';
style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:;
media-src 'self' data: blob:; connect-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'self'
```

What that means for your code:

- **No network of its own**: `fetch`, XHR and WebSocket are blocked. To reach a web site, declare it in `http` and ask WRM with `wrm:http` (see [Web sites and secrets](#web-sites-and-secrets)).
- Scripts are files of the ZIP or inline `<script>` blocks (WRM allows each inline block by its hash). **Inline event handlers** (`onclick="…"`), `eval` and remote scripts or fonts do not run: use `addEventListener`.
- Use a `ResizeObserver` to redraw charts when the window changes size.

Every file is pinned by SHA-256 when the addon is installed. A file that changes on disk afterwards, or a file added later, is refused (`E_INTEGRITY`) until the addon is installed again.

## The bridge (postMessage)

The panel talks only to the WRM page around it: `window.parent.postMessage(message, '*')`. Every message the panel sends carries `v: 2`; anything else is dropped and shown in the window's diagnostics.

Panel → WRM:

| Message | Meaning |
|---|---|
| `{v:2, type:'wrm:ready'}` | send it on load, **within 5 seconds**, or the window shows "did not start" |
| `{v:2, type:'wrm:heartbeat'}` | every 15 seconds; three missed heartbeats show "not responding" |
| `{v:2, type:'wrm:context'}` | ask for the context again |
| `{v:2, type:'wrm:store.get', id, key}` | read a stored value |
| `{v:2, type:'wrm:store.set', id, key, value}` | store any JSON value (32 KB at most) |
| `{v:2, type:'wrm:http', id, method, url, headers, body, encoding, secret}` | a web request sent by WRM (see below) |
| `{v:2, type:'wrm:notify', text, level}` | a notification: plain text up to 200 characters, `level` is `info`, `success`, `warn` or `error`; at most 5 a minute per instance, a repeat within 5 minutes is dropped |
| `{v:2, type:'wrm:resize', w, h}` | ask for a content size; WRM decides (not when snapped, pinned or the layout is locked) |
| `{v:2, type:'wrm:error', message, source, line}` | report an error into the window's diagnostics (⚠ in the title bar) |

WRM → panel:

| Message | Meaning |
|---|---|
| `wrm:context` | `{v, user: {id, name}, lang, theme: {"--bg": …}, addon: {id, key, version, instance, instanceName}, config: {…stored values…}, features: {store, notify, http, jobs: false, history: false, visibility: true}, http: {origins, approved, ready: {secret name: true if a value is set}}, size: {w, h}, visible}` |
| `wrm:theme` | `{theme}` after the user switched the theme |
| `wrm:visibility` | `{visible}`: the window was minimized or restored, or the browser tab hidden. **Stop polling and timers while hidden.** |
| `wrm:store` | the answer to a store request: `{id, key, value}`, or `{id, key, error, code}` |
| `wrm:http` | the answer to a web request: `{id, ok, status, headers, body, encoding: 'text'|'base64', truncated, coalesced}`, or `{id, ok: false, error, code}` |
| `wrm:fatal` | `{code, message}` right before WRM closes the panel (addon disabled or removed) |

`user.name` is the display name, never the sign-in name. WRM runs **no background jobs** for addons: a panel works while its window is open, and 24/7 alerting belongs in the monitored system (for example Grafana alerting).

## Instances, store and windows

An addon with a panel can be opened as several **instances**, each with its own stored values and window (for example two dashboards). Opening the panel the first time creates an instance named *Default*. *Settings → Addons → Instances* creates, renames, exports, imports and removes them.

- An administrator can create **shared** instances of an organisation addon; everybody who can use the addon sees them. Your own instances are visible only to you.
- **Store scope:** a personal instance keeps the owner's values. In a shared instance each user has their own values, unless the manifest says `"store": {"shared": true}`: then there is one set for everybody, written by administrators (`E_STORE_READONLY` for others).
- Keys: 1-64 characters (letters, digits, `.`, `-`, `_`), not starting with `wrm.`. Values: JSON, 32 KB each, `quotaKb` per instance in total (`E_LIMIT_QUOTA`).
- **Export / Import** writes or reads a file `{"format": "wrm-addon-config", "addon", "version", "instance", "values"}`.
- The place and size of each window are remembered per user on the server, so an open window comes back after a reload, a restart or on another device.

## Web sites and secrets

The `http` block lists what the panel may reach:

- `origins` (1-8): exact `scheme://host[:port]`, lowercase, without a path or the default port. `https` is the rule; a plain `http://` origin works only after an administrator approved it as private network.
- `paths` (default `/*`): exact paths, or prefixes ending in `*` (`/api/*`). Paths with `..` are refused.
- `methods` (default `GET`): `GET`, `HEAD`, `POST`, `PUT`, `PATCH`, `DELETE`.
- `secrets` (up to 8): `name` (sent as `Authorization: Bearer <value>`), `basic:name` (the value is `user:password`) or `header:X-Api-Key:name`. Names are 1-32 lowercase letters, digits and `_`.

A request:

```js
const id = 'h' + Date.now();
window.parent.postMessage({v: 2, type: 'wrm:http', id, method: 'GET',
  url: 'https://grafana.example.com/api/health', headers: {Accept: 'application/json'},
  secret: 'grafana_token'}, '*');
window.addEventListener('message', (e) => {
  const d = e.data;
  if (d && d.type === 'wrm:http' && d.id === id && d.ok) render(JSON.parse(d.body));
});
```

How WRM sends it:

- Only to a listed origin, path and method, and only after an **administrator approved the origins**. Enabling an organisation addon as an administrator approves them; a personal addon shows *sites wait for an administrator* until an administrator presses *Approve*. An update that adds an origin, a method, a path or a secret waits for approval again.
- WRM resolves the name itself and refuses loopback, private (RFC 1918, `fc00::/7`), link-local (cloud metadata), CGNAT, multicast and reserved addresses (`E_HTTP_BLOCKED`), unless the administrator ticked **private network** for that origin. The address that was checked is the one connected to, so DNS rebinding does not get around it.
- Only `Accept`, `Content-Type` and `Range` are taken from the panel; WRM sets its own `User-Agent`. Cookies are never sent or kept. The answer carries `Content-Type`, `Content-Length`, `Content-Range`, `ETag`, `Last-Modified`, `Cache-Control`, `Date` and `Retry-After`.
- Redirects (3 at most) are checked like the first URL; the secret is sent only to the same origin.
- Limits: 256 KB request body, 8 MB response (cut, `truncated: true`), 10 s, 120 requests a minute and 20 at once per instance. Identical `GET`/`HEAD` requests within 2 seconds share one answer (`coalesced: true`).
- Text answers (`text/*`, JSON, XML, JavaScript) come back as text, anything else as base64.
- Every request is logged (method, origin, path, status, size, time, code; never the secret or the body); the window's **⚠** diagnostics list the recent ones.

**Secrets** are entered in WRM, never in the addon: *Settings → Addons → Instances → 🔑 Secrets*. The value for a shared instance is set by an administrator, for a private instance by its owner; in a shared instance every user may add their **own** value, which replaces it for them. Values are stored encrypted, up to 4 KB, without line breaks. The panel names a secret in `wrm:http` and only learns from `wrm:context` (`http.ready`) whether a value is set. Never put tokens, passwords or internal host names in the ZIP.

The policy `addon_http` (`off`, `admins`, `all`) decides who may use addons with web access at all.

## Approvals and updates

Enabling an addon approves its permission sheet. An update that asks for **more** (a new capability, a larger store, a shared store, more server exports) is installed but stays **off until the sheet is approved again**; the new lines are marked. An update that asks for the same or less keeps the addon enabled.

## Error codes

Every refusal of the API carries `{error, code, hint}`:

| Code | Meaning |
|---|---|
| `E_MANIFEST` | addon.json breaks a rule (the message says which) |
| `E_MANIFEST_FIELD` | an unknown field in an apiVersion 2 manifest |
| `E_API_VERSION` | `apiVersion` is not 1 or 2 |
| `E_MISSING_FILE` | a declared file is not in the ZIP |
| `E_LIMIT_SIZE` | archive, value, request body or response too large |
| `E_LIMIT_QUOTA` | the store of the instance is full |
| `E_LIMIT_COUNT` | too many instances (50 per user per addon) |
| `E_STORE_KEY`, `E_STORE_VALUE` | invalid key, value is not JSON |
| `E_STORE_READONLY` | shared values are written by administrators |
| `E_CAPABILITY` | the addon does not declare what it tried to use |
| `E_DISABLED` | the addon is disabled |
| `E_INTEGRITY` | a file changed after the install |
| `E_DOWNGRADE` | an older version than the installed one; confirm with `allow_downgrade=1` |
| `E_PERSONAL_OFF`, `E_PERSONAL_SERVER` | personal addons are turned off; personal addons cannot carry server code |
| `E_NOT_FOUND` | the addon or instance does not exist (for you) |
| `E_HTTP_REQUEST` | the request is not valid (URL, JSON, base64 body) |
| `E_HTTP_ORIGIN`, `E_HTTP_PATH`, `E_HTTP_METHOD` | the origin, path or method is not in `http` |
| `E_HTTP_UNAPPROVED` | an administrator has not approved the origin yet |
| `E_HTTP_SCHEME` | plain `http://` without a private-network approval |
| `E_HTTP_BLOCKED` | the name resolves to a local or private address |
| `E_HTTP_REDIRECT` | too many redirects, or a redirect to a site that is not allowed |
| `E_HTTP_SECRET`, `E_SECRET_MISSING` | the secret is not declared; it has no value for this instance |
| `E_HTTP_TIMEOUT`, `E_HTTP_UPSTREAM` | the site did not answer in 10 s; the connection failed |
| `E_HTTP_OFF` | web access for addons is turned off for you (`addon_http`) |
| `E_LIMIT_RATE` | too many requests (120 a minute, 20 at once) |

In the panel, `wrm:store` answers carry the same codes, and *Diagnostics* lists the last 20 bridge errors (`E_BRIDGE`, `E_LIMIT_RATE`, …).

## Tools for writing addons

Since v13.12.0 WRM has tools for writing an addon by hand or with an AI assistant. The usual loop is *template → edit → lint → dev server → install*.

### Templates

`wrm -addon-new TEMPLATE [-addon-id ID] DIR` writes a template into an empty folder (options go before the folder). *Settings → Addons → 🛠 Developer tools → Templates* offers the same as ZIPs (`GET /api/addons/dev/templates/{name}.zip?id=`).

| Template | What it shows |
|---|---|
| `blank` | a panel with the bridge, a stored setting and a notification |
| `fetch-and-render` | JSON from one site (`https://status.example.com/api/*`) with a secret, refreshed only while the window is visible |
| `grafana-panel` | firing alerts from Grafana's Alertmanager API (`/api/alertmanager/grafana/api/v2/alerts`) with a service account token as the secret, and a notification for new alerts |
| `wasm-hook` | a `server.wasm` hook: TinyGo source (`main.go`, build with `tinygo build -o plugin.wasm -target=wasm-unknown -no-debug .`) and a placeholder `plugin.wasm` so it installs at once |

The panel templates include **`wrm-bridge.js`**, a helper that hides the message format: `wrm.ready()` (sends `wrm:ready` and the heartbeat), `wrm.on('context' | 'theme' | 'visibility', fn)`, `wrm.context`, `wrm.visible`, `await wrm.storeGet(key, fallback)`, `await wrm.storeSet(key, value)`, `wrm.notify(text, level)`, `await wrm.http({method, url, secret})`, `wrm.t(en, hr)`, `wrm.error(message)`, `wrm.resize(w, h)`. Uncaught errors are reported as `wrm:error`. Promises reject with an `Error` whose `code` is the WRM error code.

### Lint

`wrm -addon-lint DIR|ZIP` checks an addon without running any of it and prints `level file:line: message [code]` with a hint, then the permission summary. It exits with 1 when there is an error. `-addon-lint-json` prints the report as JSON, the same as `POST /api/addons/dev/lint` (a ZIP as the body) and *Check ZIP* in the developer tools. A failed install opens this report in WRM by itself.

```json
{"ok": false, "errors": 1, "warnings": 1,
 "diagnostics": [{"level": "error", "code": "E_LINT_NETWORK", "message": "fetch( is blocked in the panel (connect-src 'none')",
                  "file": "panel.js", "line": 12, "hint": "Ask WRM: wrm:http (or wrm.http() in wrm-bridge.js) with the http capability"}],
 "permissions": ["..."], "summary": ["..."], "csp": "default-src 'none'; ..."}
```

The lint first runs every check of an install (archive size, paths, file count, strict manifest, validation, WebAssembly imports and exports), so an addon that lints without errors installs. Then it reads the files:

| Code | Level | Meaning |
|---|---|---|
| `E_LINT_REMOTE` | error | a remote script, stylesheet, font or image (`https://…` in `src`, `href`, `url()` or `@import`) |
| `E_LINT_INLINE_HANDLER` | error | `onclick=…` and other inline handlers; use `addEventListener` |
| `E_LINT_JS_URL` | error | a `javascript:` URL |
| `E_LINT_EVAL` | error | `eval`, `new Function` or a string passed to `setTimeout` / `setInterval` |
| `E_LINT_NETWORK` | error | `fetch`, `XMLHttpRequest`, `WebSocket`, `EventSource`, `navigator.sendBeacon` |
| `E_LINT_STORAGE` | error | `localStorage`, `sessionStorage`, `indexedDB`, `document.cookie` |
| `E_LINT_SECRET` | error | something that looks like a token (`glsa_…`, `ghp_…`, `xox…`, `AKIA…`, `sk-…`) or a password or token assigned to a literal |
| `E_LINT_ORIGIN` | warning | a URL whose origin is not in `http.origins` |
| `E_LINT_READY`, `E_LINT_HEARTBEAT` | warning | an apiVersion 2 panel that never sends `wrm:ready` or `wrm:heartbeat` |
| `E_LINT_CAPABILITY` | warning | a bridge message (`wrm:store.*`, `wrm:notify`, `wrm:http`) without its capability |
| `E_LINT_V1` | warning | an apiVersion 1 manifest (no store, notifications or web requests) |
| `E_LINT_SERVER` | info | server code: only administrators install it, personal addons cannot carry it |
| `E_LINT_UNSERVED` | info | a file with a type WRM never serves to the panel |
| `E_LINT_HIDDEN` | info | a hidden file or `node_modules` left out when a folder is checked |
| `E_WASM` | error | the module imports something other than `wrm.log`, or an export is missing |
| `E_MANIFEST*`, `E_LIMIT_*`, … | error | the install checks above, with the field's line in `addon.json` where it can be found |

### Dev server

`wrm -addon-dev DIR` serves the addon at `http://127.0.0.1:8790` (`-addon-dev-port`): the panel in the same sandboxed frame and with the same CSP as in WRM, inside a host page with a language, theme and visibility switch, *Reload*, the lint report, the bridge log and the watchdog. The host answers like WRM: `wrm:context`, an in-memory store with the same limits, notifications as toasts, and `wrm:http` through the same checks (origin, path, method, address guard, redirects, size). Secrets come from environment variables, `WRM_ADDON_SECRET_<NAME>` with the name in upper case (`grafana_token` → `WRM_ADDON_SECRET_GRAFANA_TOKEN`). Private-network and plain `http://` sites are refused unless you start it with `-addon-dev-private`. When a file in the folder changes the server lints again and the page reloads the panel.

The dev server listens on the loopback address only, answers only requests for `127.0.0.1`, `localhost` or `[::1]` with its port (against DNS rebinding), and its own API needs a random token that only its page knows.

### Schema and llms.txt

`GET /api/addons/dev/schema.json` is the JSON Schema (draft 2020-12) of an apiVersion 2 `addon.json`; editors that understand `"$schema"` can use it. `GET /api/addons/dev/llms.txt` is this contract in short for AI assistants: the workflow, the manifest, the sandbox, the bridge, web requests and secrets, and every error code. Both need a signed-in WRM user.

## A minimal panel

```html
<!doctype html><html><body>
<h3 id="who">…</h3><p>Count: <b id="n">0</b></p><button id="inc">+1</button>
<script>
const send = m => parent.postMessage(Object.assign({v: 2}, m), '*');
let n = 0;
addEventListener('message', e => {
  const d = e.data || {};
  if (d.type === 'wrm:context') {
    document.getElementById('who').textContent = d.addon.instanceName;
    n = (d.config && d.config.count) || 0;
    document.getElementById('n').textContent = n;
  }
});
document.getElementById('inc').addEventListener('click', () => {
  n++; document.getElementById('n').textContent = n;
  send({type: 'wrm:store.set', id: 'c' + n, key: 'count', value: n});
});
send({type: 'wrm:ready'});
setInterval(() => send({type: 'wrm:heartbeat'}), 15000);
</script></body></html>
```

*Settings → Addons → AI addon* gives the same rules as a prompt for an AI client; with the addon tools the WRM assistant writes a draft you review and install.
