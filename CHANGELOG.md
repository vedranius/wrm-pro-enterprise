## [14.0.2] - 2026-10-29

### Security

- **`curl` is network egress, not a read-only command.** A model that has just read a file can put its contents into a URL, so URL fetches now need approval (or an automatic-mode pattern).
- **More files are never read (or created) by the assistant:** `.env`/`.env.*`, `*.p12`/`*.pfx`, `.npmrc`, `.pypirc`, `wp-config.php`, `terraform.tfvars`, `credentials.json`, `config/database.yml`.
- **Server-controlled system info is sanitized** (line breaks, control characters and angle brackets) before it enters the system prompt.
- **Guest links stay closed.** With `allow_link_shares` off, an existing link is refused (`403 link_shares_disabled`) instead of becoming available to every signed-in account.
- **HSTS behind a trusted proxy.** The header is now sent whenever `isHTTPS()` is true, including `WRM_TRUST_PROXY=1` setups.

### Fixed

- **Whole-server Git scans are no longer cut at 2 MB.** Scan output has a 32 MB cap, and any truncation is reported as such instead of "the scan did not finish".
- **No `/tmp/deploytool-updates.jsonl`.** The deploy history never uses the predictable `/tmp` path and skips a symlink at its target.
- **Certificate reads stay read-only.** `openssl x509 -in cert.pem -noout` works again; private key bundles remain denied.
- **The enterprise container image is `ghcr.io/<owner>/wrm-pro-enterprise`** (the shared `wrm-pro` package belongs to the public repository). A registry failure no longer blocks a release.

## [14.0.1] - 2026-10-28

### Security

- **2FA lockout.** A correct password no longer resets the per-IP counter before the second factor, and a wrong TOTP/recovery code counts against the account (`login_max_failures`, 15-minute lock).
- **Password confirmations are rate-limited.** Password change, config export with secrets, credential reveal, key export, connection secrets and 2FA disable/recovery share the sign-in limiter and account lockout.
- **Destructive AI commands cannot hide.** `sudo -u root …`/`doas …`, `rm -rf ../..` and `bash <<< '…'` are classified and refused like their direct forms.
- **Bundle manifests are validated.** Absolute paths, `..`, dot/empty segments, backslashes and control characters in `bundle.json` or payload names refuse the import.
- **Addon archives are bounded.** The sum of uncompressed entries is capped (64 MB) and an entry over 8 MB fails the install instead of being truncated; `%2e%2e` no longer bypasses `http.paths`; window/secrets/http are refused while the addon is disabled.
- **MCP edits need a fresh confirmation** when step-up is on, and passkeys require user verification (biometric/PIN) at registration, sign-in and step-up.
- **Host builds are administrator-only** and no longer inherit `ENCRYPTION_KEY`, `DB_PATH` or `WRM_*` variables.
- **SAML dependency.** goxmldsig 1.6.0 fixes GO-2026-4753 (signature verification).

### Fixed

- **SQLite foreign keys on every connection** (`_pragma=foreign_keys(1)`) with a bounded pool; `ON DELETE CASCADE` now works.
- **Automatic-mode budget survives a new session**, and persisting it no longer closes the session as a permission change.
- **`ensureTable` keeps columns declared in the CREATE statement**, so addon instances are no longer set aside on every start; instances set aside by 13.9–13.14 are restored.
- **Deleting a connection removes its monitoring samples, profile, log settings and server health.**
- **Embedded UI:** gzip + ETag (304 on reload), deferred vendor scripts; failed deletes/revokes show the server error instead of success; toasts are a live region and the page language follows the chosen language.

## [14.0.0] - 2026-10-27

### Licence

- **Apache License 2.0.** From v14.0.0 the project is licensed under Apache-2.0 (see `LICENSE`, `NOTICE` and `LICENSE-CHANGE.md`). Releases up to v13.x remain under their original PolyForm terms; their tags and assets are unchanged.
- **No GPL-licensed tools in the distributed image.** The Bitwarden CLI (GPL-3.0) is no longer bundled; operators install it at runtime if they use the vault integration (`docs/THIRD-PARTY.md`).
- **Governance and contributions.** Added `TRADEMARK.md`, `GOVERNANCE.md`, `MAINTAINERS.md`, `CLA.md` and `CODE_OF_CONDUCT.md`; contributions now require a CLA and a `Signed-off-by` line (`CONTRIBUTING.md`).
- **SPDX identifiers** and the copyright line are now on every source file; releases ship a CycloneDX SBOM.

This is the first release of the **WRM Pro Enterprise** distribution. The pre-v14.0.0 history and tags live in the predecessor repository [`web-browser-RDM-public`](https://github.com/vedranius/web-browser-RDM-public).

## [13.14.0] - 2026-10-26

### Added
- **Folder sorting** ([#134](https://github.com/vedranius/web-browser-RDM-public/issues/134), `folder_sort.go`): right-click a folder for *Sort subfolders A–Z (all levels)* and *Keep subfolders sorted A–Z*, and for the whole tree *Sort all folders A–Z* and *Keep all folders sorted A–Z*. Names compare without case and with numbers in their natural order. A kept-sorted folder (`folders.auto_sort`) or the whole tree (user_ui key `folders_sort`) is sorted again after a folder is created, renamed or moved. `GET / POST /api/folders/sort`, audited as `folder.sorted`.
- **File edit history** ([#129](https://github.com/vedranius/web-browser-RDM-public/issues/129), `file_edits.go`): every save from the text editor (also in shares, `POST /api/remote/upload?…&edit=1`) and every file change the AI assistant makes after approval is stored in `file_edits` with user, connection, server, path, source, session, sizes and hashes, and a unified diff computed on the WRM server, with secrets masked and encrypted at rest. Files over `file_edit_max_kb` (1024) and binary files are recorded without a diff. The table is append-only and kept for `audit_retention_days`. *Admin → File edits* (administrators and auditors) filters by search, user, server, connection, source, session and time, shows the diff and exports CSV (`GET /api/admin/file-edits[/{id}]`, audited as `file_edit.viewed` / `file_edit.exported`). Setting `file_edit_history` (default on).
- **SSH user certificates and opkssh** ([#119](https://github.com/vedranius/web-browser-RDM-public/issues/119), `ssh_cert.go`): a key logs in with an OpenSSH user certificate first and the plain key after it. Key files on the WRM server use `<key>-cert.pub` (read at every login, so `opkssh login` renewals work at once; a path to the `-cert.pub` file works too; *Auto* prefers a key with a valid certificate), key-store keys keep a certificate (`ssh_keys.certificate`, `PUT /api/keys/{id} {"certificate"}`, audited as `ssh_key.certificate_set` / `ssh_key.certificate_removed`), and a pasted private key can carry its certificate line. Certificates must be user certificates for the same key; expired ones are skipped and a refused login names them. The key store shows the validity, principals, CA fingerprint and opkssh certificates.
- **Folder filters in the Git workspace**: a server-folder filter (a tree with counts, a folder includes everything below it) in the overview, *Functionality*, *Environments*, *What is where*, Simple mode and the files of an installation. The connection-folder filter shows the folder tree and includes subfolders.
- **Logs** group in the .gitignore helper's standard patterns (`log/`, `logs/`, `*.log`, `*.log.*`), pre-selected when the repository has log files, with a warning about committed logs.

### Changed
- **Git checks compare only what the catalog tracks.** Files the catalog does not track are listed as *outside the check* and are no longer fetched from Git or compared; the files view opens on *In the check* and its *Why* column says where a file is. Log files (`*.log`, `*.log.*`, files below `log` / `logs` folders except source and configuration files) are never compared, never diffable, not reported as extra and not deployed unless the include list names them.
- The Git overview names servers by hostname or connection name; an IP address is no longer used as the server name. Hostnames are read with a marker line, so a login banner or MOTD cannot become the name.

### Fixed
- The Git overview mixed hostnames and IP addresses for servers: a connection whose hostname was not known yet fell back to its IP address.
- The Git checker compared log files and other untracked files with Git, which filled the files view with differences that the deploy never touches.

## [13.13.1] - 2026-10-26

### Changed
- The Claude app instructions (*Settings → AI connections*, the setup script's summary, README and the plugin's README) give the repository in lowercase, `vedranius/web-browser-rdm-public`. The Claude app names a marketplace after the repository name; GitHub accepts the name in any case.
- Plugin **1.2.1**: the icon path starts with `./`, as the manifest reference asks.

### Fixed
- **Adding the marketplace in the Claude app showed only *Failed to add marketplace***: the instructions now use the lowercase repository name, and README (Troubleshooting) and the plugin's README say what else to check: a marketplace with the same name already added, and where the app's log keeps the real reason (`MARKETPLACE_ERROR`, `REMOTE_SYNC_FAILED`).

## [13.13.0] - 2026-10-24

### Added
- **The WRM PRO Claude plugin works without settings** in the Claude app (chat, Cowork) and in Claude Code. Its MCP server is now the WRM bridge packed as an MCP bundle (`packaging/claude-plugin/wrm-pro/server/wrm.mcpb`, built reproducibly by `buildPluginMCPB` with `pluginMCPBVersion`; `TestV13130PluginBundle` fails when the committed copy is stale, `WRM_WRITE_PLUGIN_MCPB=1` rewrites it). The Claude app runs it on the user's computer with its built-in Node.js, so a WRM on a private network and over plain HTTP is reachable.
- **`wrm_connect`, sign-in from the bridge** (`mcpb/server/index.js`): without `WRM_MCP_URL`/`WRM_MCP_TOKEN` the bridge reads `~/.wrm/mcp.json` (`WRM_MCP_CONFIG`); without a connection, or when WRM refuses the token or cannot be reached, it answers `initialize` itself and offers `wrm_connect` with the reason. `wrm_connect {address}` accepts what a user types (`192.168.1.10:8780`, with or without scheme or `/mcp`), registers with WRM (dynamic client registration, loopback redirect `http://127.0.0.1:<port>/callback`), opens WRM's consent dialog in the browser, exchanges the code with PKCE S256, saves the token (0600, atomically), opens the WRM session with the app's own `initialize` and sends `notifications/tools/list_changed`. It waits up to 90 seconds and otherwise returns the link; the sign-in stays open for 15 minutes. Connected, the bridge appends `wrm_connect` to `tools/list` for signing in again or switching WRM. The token never passes through the chat.
- **Skill `wrm`** (`/wrm-pro:wrm`): connect when needed, list the servers with their modes and everything WRM offers, grouped by area, and how to work through it.
- **All AI apps at once**: *Settings → AI connections → 🧰 All AI apps at once* and `POST /api/mcp/setup-all {os}` (signed in, `mcpUserAllowed`, audited as `mcp.setup_script`) download `wrm-setup-all.ps1` (UTF-8 with BOM) or `wrm-setup-all.sh` with this WRM's address and the bridge, never a token. The script writes `~/.wrm/wrm-mcp.js` and runs `node wrm-mcp.js --setup --url …`: browser sign-in (or `WRM_MCP_TOKEN`), a connection check, `~/.wrm/mcp.json`, then the bridge as MCP server `wrm` in Claude Code (the WRM PRO plugin, else `claude mcp add`), the Claude app (`claude_desktop_config.json`, also the Microsoft Store location), Codex CLI, Gemini CLI, VS Code (user `mcp.json`), Cursor, Windsurf and OpenCode, each edited file copied to `<file>.wrm-backup-<time>` first (JSON with comments and trailing commas is read). All apps share the one connection.
- **`claude-plugin-wrm-pro.zip`** in every release, for *Customize → Plugins → Upload plugin* (outside `SHA256SUMS.txt`, covered by the build provenance).

### Changed
- Plugin **1.2.0**: no `userConfig` (`wrm_url`, `wrm_token`), `mcpServers` is the bundle, an icon and a privacy link; `/wrm-pro:connect` uses `wrm_connect` instead of `claude mcp add`. The wizard's plugin command has no `--config` any more.
- The Claude Desktop extension (`POST /api/mcp/claude-desktop`) uses the same bridge: with a wrong, expired or revoked token it keeps running and offers `wrm_connect` instead of failing every call.
- *Settings → AI connections*: **All AI apps at once** is the first quick-start button; the Claude Desktop and Claude Code texts mention the plugin in the Claude app.

### Fixed
- **Installing the plugin from the Claude app could not work**: its MCP server needed `${user_config.*}` values, which the Claude app cannot ask for (chat ignores such a server, Cowork does not prompt), and an HTTP connector there connects from Anthropic's cloud, which cannot reach a WRM on a private network.
- The OAuth consent dialog showed the key `mcp_sc_addons.manage` instead of a description.

## [13.12.0] - 2026-10-24

### Added
- **Addon platform v2, step 3: tools for AI-made addons** ([docs/ADDONS.md](docs/ADDONS.md#tools-for-writing-addons)).
- **`wrm -addon-lint DIR|ZIP`** (`addons_lint.go`; `-addon-lint-json` for JSON, exit code 1 on an error): the same archive checks as an install (size, paths, file count, manifest decoding and validation, WebAssembly imports and exports), then the files: remote scripts, stylesheets, fonts and images, inline event handlers, `javascript:` URLs, `eval` / `new Function`, `fetch` / XHR / WebSocket / EventSource, `localStorage` / IndexedDB / cookies, token shapes and secret assignments in the ZIP, sites missing from `http.origins`, `wrm:ready` and `wrm:heartbeat` in apiVersion 2 panels, bridge calls without the capability, files never served, apiVersion 1 manifests, hidden files. Every finding is `{level, code, message, file, line, hint}`; the report adds the permission summary and the panel's CSP. Nothing in the addon runs.
- **`POST /api/addons/dev/lint`** and *Settings → Addons → 🛠 Developer tools → Check ZIP*; a failed install opens the lint report by itself, with *Copy report* for an AI.
- **Starter templates** (`addons_templates.go`, `addon_templates/`): `blank`, `fetch-and-render`, `grafana-panel` (alerts from Grafana's Alertmanager API with a secret and notifications) and `wasm-hook` (TinyGo source plus a placeholder module so it installs at once). `wrm -addon-new TEMPLATE [-addon-id ID] DIR`, `GET /api/addons/dev/templates[/{name}.zip?id=]` and *Templates* in the developer tools. The panel templates carry **`wrm-bridge.js`**: `wrm.ready`, `wrm.on`, `wrm.storeGet`, `wrm.storeSet`, `wrm.notify`, `wrm.http`, `wrm.t`, the heartbeat and error reporting.
- **`wrm -addon-dev DIR`** (`addons_dev.go`): a dev server on `127.0.0.1` (`-addon-dev-port`, default 8790) that serves the panel with the real CSP in the real sandbox, a host page with language, theme and visibility switches, an in-memory store, web requests to the listed sites through the same checks as WRM (secrets from `WRM_ADDON_SECRET_<NAME>`, private and plain-http sites only with `-addon-dev-private`), the lint report, the bridge log, the watchdog, and a reload on every file change.
- **JSON Schema and `llms.txt`**: `GET /api/addons/dev/schema.json` (draft 2020-12, the apiVersion 2 manifest) and `GET /api/addons/dev/llms.txt` (the addon contract for AI assistants with every error code). The *AI addon* prompt names the tools.
- Hints for the web request error codes (`E_HTTP_*`, `E_SECRET_MISSING`, `E_LIMIT_RATE`).

### Changed
- Addon ids `dev`, `install`, `drafts`, `community` and `users` are reserved (they collide with API paths).
- Unpacking an addon archive writes through `os.Root`, so no archive entry can reach outside the addon folder even if a path check were missed.

## [13.11.0] - 2026-10-24

### Added
- **Addon platform v2, step 2: web sites through WRM** (`addons_http.go`, [docs/ADDONS.md](docs/ADDONS.md)). A new capability `http` with a manifest block `http: {origins, paths, methods, secrets}` (8 origins, exact `scheme://host[:port]`; paths exact or prefix with a trailing `*`; methods `GET HEAD POST PUT PATCH DELETE`, default `GET`). The panel sends `wrm:http` and WRM performs the request (`POST /api/addons/{key}/instances/{iid}/http`): only listed origins, paths and methods, only `Accept`, `Content-Type` and `Range` from the panel, its own `User-Agent`, no cookies, 256 KB request, 8 MB response (cut with `truncated`), 10 s, at most 3 redirects, each checked again. Text answers come back as text, others as base64; a whitelist of response headers is passed on.
- **SSRF guard**: WRM resolves the host itself and connects to the address it checked, so DNS rebinding does not get around it. Loopback, private, link-local, CGNAT, multicast, broadcast and reserved addresses are refused (`E_HTTP_BLOCKED`, audited as `addon.http_refused`) unless an administrator approved the origin as **private network**; plain `http://` needs that approval too (`E_HTTP_SCHEME`).
- **Administrator approval of origins** (`addon_http_grants`): enabling an organisation addon as an administrator approves its origins (with the *private network* ticks on the permission sheet); personal addons show *sites wait for an administrator*, and administrators approve, change or withdraw them (`POST/DELETE /api/addons/{key}/http`, `/api/addons/users/{user id}/{id}/http`), audited as `addon.http_approved` / `addon.http_revoked`. The permission sheet lists every site, the methods, the paths and the secrets; an update that adds any of them waits for approval again.
- **Secrets** (`addon_secrets`, encrypted): `http.secrets` entries `name` (Bearer), `basic:name` or `header:X-Name:name`. *🔑 Secrets* on an instance sets the value for the instance (administrators for shared instances, the owner for private ones) and, in a shared instance, each user's own value that replaces it for them (`GET/PUT/DELETE /api/addons/{key}/instances/{iid}/secrets[/{name}]`). The panel only learns whether a value is set; WRM adds it to requests for the same origin and strips it on a redirect elsewhere. Audited as `addon.http_auth_set` / `addon.http_auth_cleared` without the value.
- **Rate limits and coalescing** per instance: 120 requests a minute and 20 at once (`E_LIMIT_RATE`); identical `GET`/`HEAD` requests within 2 seconds share one answer.
- **Request log** (`addon_http_log`, 20000 rows kept): method, origin, path, status, size, time and code, never the secret or the body; `GET /api/addons/{key}/instances/{iid}/http/log` and *Recent web requests* in the window's diagnostics.
- Policy `addon_http` (`off`, `admins`, `all`; default `all`): who may use addons with web access (`E_HTTP_OFF`).
- `wrm:context` carries `features.http` and `http: {origins, approved, ready}` (`ready` says which secrets have a value); the *AI addon* prompt describes the http capability and its error codes.

## [13.10.0] - 2026-10-23

### Added
- **Show saved login** in *Edit connection* (and the 👁 in an empty password field): the owner sees what the connection logs in with: its password or private key, the user name and password of an own vault credential, the key store key, the Bitwarden SSH key, or the path of a key file on the WRM server. Confirmed with the WRM password; a domain (LDAP / Active Directory) account confirms with its directory password and an SSO-only account through its identity provider (step-up). `POST /api/connections/{id}/secret`, audited as `connection.secret_revealed` / `connection.secret_denied`, raises the *secret revealed* alert, follows `allow_secret_export` and the step-up policy, and counts wrong passwords towards the sign-in rate limit. A credential shared by another user is never shown to the grantee.
- **Copy username, Copy password and Copy IP address** in the connection's right-click menu. *Copy password* asks for the same confirmation; the password goes to the clipboard and is not shown in the notification (over plain HTTP, where the browser may refuse, a **Copy** button tries again).
- **Use the server's hostname as the name** when adding a connection: the name can stay empty; after saving, WRM logs in once and reads the hostname (SSH, SFTP) or looks the address up in DNS (other protocols) and renames the connection. An unknown host key is shown for approval first. *Rename to the server's hostname* in the connection menu does the same for existing connections (`POST /api/connections/{id}/name-from-hostname`).

### Changed
- **Tabs and window titles show only the connection's name**, not *folder / subfolder / name*; the folder path is in the tooltip. Pickers and search still show the path.
- The vault's *Reveal* and the key store's *Export* accept the directory password of a domain account (they only knew local passwords).

### Fixed
- **The version next to the logo is always visible.** When the top bar shortened its texts it hid the version, and the empty session name *— none —* (whose *SESSION:* label had been hidden) looked like a missing version. Now only the *WRM* prefix gives way, and an empty session is hidden together with its label.
- **The user name in the top bar no longer jumps every 5 seconds.** With MCP enabled the background poll showed the *AI connected* button and the indicator hid it again a moment later; the button now has one owner and is shown only while an AI app is connected or an approval waits, and the poll changes nothing when nothing changed.
- **Git workspace: no more blank servers in the server filter.** Installations found on connections that were later deleted stayed in the database with an empty name; they are no longer listed, and deleting a connection removes its installations and cached hostname.

## [13.9.0] - 2026-10-23

### Added
- **Addon platform v2, step 1** (`addons_v2.go`, `static/addons.js`, [docs/ADDONS.md](docs/ADDONS.md)). A manifest with `"apiVersion": 2` is decoded strictly: unknown fields, also nested ones, refuse the install with `E_MANIFEST_FIELD`. A manifest without `apiVersion` keeps the v1 behaviour.
- **Personal addons.** Any user the new policy `addon_personal` allows (`off`, `admins`, `all`; default `all`) installs addons that only they see (table `addons_personal`, folder `<DB>.addons/.users/<user id>/<id>`, URL key `~<id>`). They run in the same sandbox and cannot carry server code (`E_PERSONAL_SERVER`). Administrators see every user's personal addons in *Settings → Addons* and can turn one off or remove it (`/api/addons/users/{user id}/{id}`).
- **Themes from addons** (`ui.themes`): up to 16 themes per addon with `id`, `name`, `name_hr`, `mode` and CSS variable `tokens` and/or a `css` file, validated on install (token names `--name`, values without `;`, `{`, `}`, `<`, `url(`, `@import`). They are offered in *Settings → Appearance* and used only by the people who pick them.
- **Instances**: an apiVersion 2 panel opens as one or more instances, each with its own stored values and window (`addon_instances`). Administrators create shared instances of organisation addons; everybody else creates their own. *Settings → Addons → Instances* creates, opens, renames, exports, imports and removes them.
- **Server-side store** (`store` capability, `addon_store`): JSON values per instance, 32 KB per value, `store.quotaKb` (default 64, at most 1024) per instance, per user in a shared instance unless `store.shared` makes one set for everybody written by administrators. Export and import of an instance's values as a file.
- **Windows remembered on the server** (`addon_windows`): place, size, minimized and snapped state per user and instance; open windows come back after a reload, a restart or on another device. `ui.defaultSize` and `ui.minSize` set the first and smallest size.
- **Bridge v2** (`v: 2` on every message): `wrm:ready` within 5 s and `wrm:heartbeat` every 15 s with a watchdog banner (*Reload*, *Diagnostics*, *Disable addon*, *Close*), `wrm:context` with user id and display name, language, theme tokens, instance and stored values, `wrm:theme` on a theme switch, `wrm:visibility` on minimize and restore, `wrm:store.get` / `wrm:store.set`, `wrm:notify` (`notify` capability, 200 characters, 5 a minute, repeats within 5 minutes dropped), `wrm:resize`, `wrm:error` into the window's diagnostics (⚠ in the title bar), `wrm:fatal` before WRM closes a panel. Unknown messages are dropped and counted.
- **Permission sheet**: enabling an addon shows what it may do, generated by WRM from `addon.json`, and records it as approved. An update that asks for more (a new capability, a larger or shared store, more server exports) is installed but stays off until approved again; the new lines are marked. A lower version needs `allow_downgrade=1` (`E_DOWNGRADE`; the pane asks).
- **Error codes with hints** on addon API refusals (`{error, code, hint}`: `E_MANIFEST`, `E_MISSING_FILE`, `E_LIMIT_QUOTA`, `E_STORE_READONLY`, `E_INTEGRITY`, …) so an AI that wrote the addon can repair it.
- The **AI addon** prompt describes apiVersion 2: manifest, CSP, bridge, store, notify, themes and error codes.

### Changed
- **apiVersion 2 panels have no network of their own**: their HTML is served with a full CSP (`default-src 'none'`, `connect-src 'none'`, `form-action 'none'`, `base-uri 'none'`, inline scripts allowed only by their SHA-256). v1 panels keep `sandbox allow-scripts`.
- **Every addon file is pinned by SHA-256 at install**; a file changed or added on disk afterwards is refused with `E_INTEGRITY` until the addon is installed again.
- An addon's install, enable, disable and removal are audited with its permissions and, for personal addons, the owner; instances, imports and the kill switch are audited too.

## [13.8.0] - 2026-10-23

### Added
- **AI sessions view** (🧠 *AI sessions* panel next to *Sessions*, `static/aisessions.js`, `ai_sessions_view.go`): every AI session of the signed-in user, from the terminal assistant, the log and file assistants, OpenCode and AI apps over MCP. Search (title, server, host, model or app, and the text of questions, answers and tool calls), filter (`kind`: assistant, logs, file, opencode, mcp, saved, live), paging, transcript, *Go to the chat* for a running assistant, *Open live view* for a running AI app, rename, export and *Continue*. `GET /api/ai/sessions?view=all&kind=&q=&conn_id=&before=&limit=`.
- **Continue any ended session in the WRM assistant**, including OpenCode and MCP sessions: an AI app's tool calls and results become the earlier context. Without a provider the personal default (or the first one offered) is used.
- **Saved AI sessions**: `POST /api/ai/sessions/{id}/save {saved}` (audited `ai.session_saved`); saved sessions are kept beyond `ai_transcript_retention_days` while the new setting `ai_keep_saved_sessions` is on (default).
- **Export**: `GET /api/ai/sessions/{id}/export?format=md|json` (own sessions only, audited `ai.session_export`; transcripts are stored redacted).
- **Personal AI settings** in *Settings → AI assistant*: default engine, provider, model and mode, show thinking, own API keys and the AI connections shortcut. Stored per user in `/api/me/ui/ai`; `GET /api/ai/config` returns them as `prefs` with `show_thinking`. The thinking choice is applied by the server.
- **`ai_mcp_later_approval`** (`run` default, `repeat`): approving a request the AI no longer waits for runs it at once (audited `ai.later_run`), after checking it again against the current mode and policies; the repeated call gets that output without running again.

### Changed
- An explicit *Deny* in the AI app's approval question (MCP elicitation `accept` with `decision: deny`) is final; only `decline` keeps the request for later.
- When a request is decided in WRM, WRM sends `notifications/cancelled` for its open elicitation in the AI app.
- Approvals carry `run_now` when they are kept for later; the live view and the 🤖 menu explain what an approval will do.

### Fixed
- A decision on a request that no longer waits answers `409` with `decided` (`outcome`, `by`, `via`, `at`) and the UI says who decided it and where, instead of "the approval is no longer pending".
- 🤖 menu approval cards are rebuilt when a request becomes "kept for later" (the edit button is gone, as edits are ignored then); approval toasts close once the request is decided.
- AI work log: the thinking line kept only its last 600 characters, so its beginning vanished while it filled; it now keeps the whole text in a scrolling box. An empty work log is removed instead of left above the question, and chat items no longer shrink in the scrolling column.

## [13.7.0] - 2026-10-23

### Added
- **Settings → Appearance** (`static/appearance.js`): eleven built-in themes (Graphite, Midnight, Fjord, Solar dark, Forest, Ember, Daylight, Paper, Solar light, High contrast dark and light), following the system's light/dark setting, a colour picker for every token including the terminal palette, saved, exported and imported themes, interface and monospace fonts, interface size, spacing, corners, line and letter spacing, heavier text and custom CSS (`appearance_custom_css`). Stored per user (`/api/me/ui/appearance`). Administrators set the default theme (`appearance_default_theme`, or **Default for everyone**).
- **Accessibility:** colour-vision palettes for deuteranopia, protanopia, tritanopia and achromatopsia (also for the terminal's ANSI colours), *Do not rely on colour alone* (signs on status badges and toasts, shapes on connection status dots, underlined links), more or maximum contrast, a minimum contrast for terminals (xterm `minimumContrastRatio`), reduced motion and transparency, a strong focus outline.
- **Themes from addons** are listed in Appearance (`ui.themes` with tokens and/or a stylesheet; the older `ui.theme` appears as one theme) and applied only for users who pick them.
- **Settings → Security → Screen lock**: Lock now, idle time with a warning, lock when WRM is hidden, when the computer locks (idle detection) or sleeps, on every start, a keyboard shortcut (Ctrl+Alt+L), the lock screen's background, clock, name, running-window count, message and tab title, and holding notifications while locked. Stored per user (`/api/me/ui/lock`). All tabs of a browser lock and unlock together.
- **Unlock PIN** (`PUT/DELETE /api/auth/lock/pin`, `screen_lock_pin`) and sign-out after wrong attempts (`screen_lock_max_failures`).
- Admin settings `screen_lock_mode` (`server`, `remember`, `browser`) and `screen_lock_max_minutes`; the Business and Enterprise security levels require the server mode and an idle lock of at most 30 / 15 minutes, Enterprise turns the PIN off.

### Changed
- **The screen lock is kept by the server** (`auth_sessions.locked_at`, `POST /api/auth/lock`): a reload or a new tab stays locked, and in the default `server` mode the locked session gets `423 Locked` from the API until it is unlocked. Running terminals, tunnels, AI and MCP sessions are not affected.
- Every colour in the interface is a theme token (`--bg0`, `--*-rgb`, `--red-t` …, `--font-ui`, `--font-mono`); primary buttons follow the accent. The accent colour moved from *Settings → General* to the theme (an earlier choice is carried over once).
- An addon's colour theme is no longer applied globally.

### Fixed
- **Directory (LDAP) accounts could not unlock the screen**: the unlock now checks the password the account signs in with.
- **The lock screen broke after its first use** (a missing helper made the lock screen throw), and reloading the page removed the lock.
- Light themes no longer show a dark top bar, dark menus, toasts and sign-in card.

## [13.6.0] - 2026-10-23

### Added
- **Folder tree in the log viewer.** *View: Folder tree* groups the logs under their **main folder** (`/var/log`, `/home/scripts` …) and its subfolders; every folder can be opened, closed and ticked to search all its logs. A new **Folder** filter (in both views) lists the main folders and their subfolders as a tree, so you can narrow the list to one folder and everything below it.
- **Main folders** in *⚙ Log settings* (`main_folders`): everything under a main folder is grouped under it. The scan paths and the Git workspace server roots count as main folders too, and a main folder is also used for labels before the default patterns, so `/home/scripts/<service>/logs` is no longer read as operator `scripts`.
- **Operators guessed from repeated words** (`guess_operators`, on by default). A word that repeats across service and folder names and is not the product name (the first word shared by several names) or an environment word (`qa`, `test`, `bkp` …) is an operator: `VideoX_NOVA`, `VideoX_NOVA2` and `INGEST_SFTP_novatv` give operators `nova` and `novatv`. Similar operators form one group (`nova` + `novatv`); the operator filter offers the group and each member. Guessed operators are marked with `≈` and a dashed border.
- Each file shows whether it lives in a **log folder** (`/var/log`, `log`, `logs` …) or an **app folder** (scripts and application folders), and the API returns `main`, `place`, `op_group`, `op_guess` per file and `main_folders`.

### Fixed
- **Logs in `/var/log/<service>` seemed missing when a script folder had the same name.** Discovery did find them (`info.txt`, dated `info.txt.2026-10-22`, `workers.out.log` …), but the service filter listed `VideoXTractor_NOVA` (the log folder) and `VideoXtractor_NOVA` (the script folder) as two services that differ only in case, so picking the script folder showed no logs. Services are now compared without case: one entry shows the logs of both folders, each marked log folder or app folder, and the folder tree keeps them apart.

## [13.5.0] - 2026-10-23

### Added
- **Professional log viewer** (*Monitoring → Logs*, `static/logviewer.js`). The file list can be sorted by newest, oldest, name, size, service, operator or path and filtered by operator and service. A log opens in a tab with line numbers, level colours (a stack trace keeps the level of its line), highlighting of times, IPs, URLs, HTTP status codes, UUIDs and key=value pairs, find in the log (regex, case, previous/next, only matching lines), level filters, **Follow** (new lines every 2.5 s, journal by cursor), loading of older lines, go to line, wrap, copy, download and a terminal shortcut. Keys: `/` find, `n`/`N`, `g`, `f`, `w`.
- **Search across logs by time.** The Search tab greps the selected or shown files on the server (`POST /api/monitoring/logs/grep`): literal or regex, case, whole word, context lines, a From/To range in the chosen date format with quick ranges (15 min … 30 days), the log time zone, hits sorted by time in either order, level chips, per-file filter and a timeline with zoom. Clicking a hit opens the file at that line.
- **Log settings, separate from Zabbix** (`⚙ Log settings`, `GET/PUT /api/monitoring/logs/settings`, table `log_settings`): scan paths, excluded names, path patterns that label operator and service (`/home/*/{operator}/{service}/{logdir}/**`), extra log name patterns, depth, file and age limits, time budget, rotated and compressed files, empty files, systemd journal, systemd/Supervisor/Docker metadata and the discovery method (`auto`, `ssh`, `sftp`). Explicit log folders and journal units moved here from the Zabbix configuration; existing ones are read from it.
- **Fast discovery over SSH.** One read-only command collects systemd, Supervisor and Docker metadata and a second runs `find` over the scan paths, so a large server is listed in about a second; servers without a shell fall back to the SFTP walk.
- **`GET /api/monitoring/logs/lines`**: `mode=tail` (`n`), `around` (`line`, `before`, `after`) and `since` (`bytes`, or `cursor` for the journal; only whole lines, `reset` when the file shrank).
- **AI log assistant.** 🤖 in the log viewer opens a chat panel that searches every discovered log on the server with the new **`search_logs`** tool (also offered over MCP under `read_logs`), reads around the hits and explains what happened. It is always read-only.
- **Smart file editor with an AI assistant** (`static/fileeditor.js`, replaces the plain editor of the file manager): syntax highlighting for about 40 languages (off above 300 KB), line numbers, find and replace with regex and case (`Ctrl+F`, `Ctrl+H`), go to line (`Ctrl+G`), auto-indent and block indent, JSON check and format, **review of the changes before saving** (a diff, can be turned off), a check that nobody wrote the file on the server since it was opened (overwrite, or show the differences first), reload from the server, and a **🤖 AI assistant** panel that reviews, explains and edits the open file. Every change of the assistant is a diff that waits for approval; the editor reloads the file afterwards, or warns when you have unsaved changes. **Ask AI about the selection** sends the selected lines.
- **The AI work log shows what the assistant thinks and does**: its reasoning (💭, Anthropic adaptive thinking on models that support it, `reasoning_content` from OpenAI-compatible servers), its narration before tool calls and each tool result. Setting `ai_show_thinking` (default on).
- **AI chats have titles and can be continued.** A short title is generated after the first answer (`ai_auto_titles`, default on; click the title to rename, `POST /api/ai/sessions/{id}/title`). An ended or interrupted session shows **Continue chat**: a new session with the earlier conversation as context (`resume_from`). History lists title, date and time, and log and file chats are kept apart from terminal chats (`GET /api/ai/sessions?purpose=terminal|logs|file`).
- **EU date and time by default** (`dd.mm.yyyy hh:mm`), with ISO, US and the browser's locale under *Settings → Date and time format*.
- **Plugin and skills**: the skills name `search_logs`, the plugin README explains what is in it and where the data goes, and `server.json` describes WRM for the MCP Registry (`io.github.vedranius/wrm-pro`). Tests check the manifests and that every tool a skill names exists.

### Changed
- **Addon panels get the display name, not the sign-in name.** The `user` field of the `wrm:context` message is now the display name (empty when none is set). Log search and the log viewer pass the search text and file paths to the server on standard input instead of on the command line.
- Discovery never lists scripts and code (`.py`, `.sh`, `.js`, …) as logs, finds logs with or without a service label and labels operator and service from the path patterns.

### Fixed
- **A week-wide search no longer stops with "Automatic folder discovery time budget reached" or "Results are bounded or incomplete"**: discovery runs on the server in one command with its own time budget, and searches grep on the server instead of downloading files.
- **Operators were not found** for logs under `/home/<project>/<operator>/<service>`; the path patterns and the operator filter now cover them.
- **AI sessions stuck in "stopped".** A session that ended (WRM restart, idle time, kill switch) kept the browser reconnecting to a dead stream; the stream now answers with an `ended` event and the chat offers **Continue chat** with its history.
- The log viewer's status no longer shows `01.01.1` before the first discovery.
- **The version at the top left is shown again after a reload.** A browser that was already signed in skipped `/api/auth/config`, so the title bar said only *WRM*; it now reads `/api/version`, and the version stays visible until the top bar also hides button labels.

## [13.4.4] - 2026-10-23

### Fixed
- **Claude Desktop and Claude Code are clearly separate in AI connections.** The wizard labels them *Claude Desktop app (chat)* and *Claude Code (terminal or Code tab)*, and says plainly that Claude Desktop uses the `.mcpb` extension while `/plugin` is a slash command inside Claude Code, not a PowerShell command. Running `/plugin` in PowerShell is the error most people hit; the panel now tells you to run `claude` first (or use the `claude plugin install …` / `claude mcp add …` line) and to run `/mcp` or restart Claude Code after adding the connector.

## [13.4.3] - 2026-10-23

### Added
- **Claude in one command, at the top of AI connections.** *Settings → AI connections* now opens with a card that shows the exact `claude mcp add --transport http --scope user wrm … --header "Authorization: Bearer …"` command, a **Copy** button and one-click shortcuts into the step-by-step wizard for **Claude Code**, **Claude Desktop** and **claude.ai / ChatGPT**. The admin view of AI connections has a matching **Open the step-by-step wizard** button. The README has the same quick start, including the WRM PRO plugin command.
- The token wizard already creates the token, fills the address and token into the command and tests the connection; the quick card makes that path discoverable without opening the advanced form.

## [13.4.2] - 2026-10-23

### Fixed
- **The version in the title bar and on the sign-in card is now the running version.** `index.html` still carried a hard-coded `v12.8.1` label, so the Update dialog said v13.4.x while the header said v12.8.1. The title, the header and the sign-in card are filled from `/api/auth/config` / `/api/version`; nothing is hard-coded any more.
- **The screen-lock button is always there and always wired.** It is a fixed top-bar button with its own `wrmLock.lock()` handler instead of one inserted at runtime next to Sign Out; `lock.js` only translates its title and label. A click now opens the lock screen even if the panel was re-rendered.

## [13.4.1] - 2026-10-22

### Fixed
- **OpenCode Go appears on existing installations.** If `ai_provider_kinds` was saved with the old "all kinds" list (before `opencode_go` existed), the setting is upgraded in memory to include it, so the new provider can be added without editing the policy; a deliberately narrowed list is left unchanged.

## [13.4.0] - 2026-10-22

### Added
- **OpenCode Go provider.** OpenCode Go is the subscription next to OpenCode Zen (the pay-as-you-go Console). A Go key is only accepted by the Go endpoints, so sending it to Zen made Console answer *insufficient funds*. The new provider kind `opencode_go` sends the key to `https://opencode.ai/inference/go/...` (chat completions or Anthropic Messages, chosen per model), uses the Go model list (`/v1/models`, with the documented list as fallback), sends a stable `x-opencode-session` per conversation and a `WRM-PRO` user agent. Qwen and MiniMax M3 use the Messages API on Go even where the Zen catalog says chat. The OpenCode agent engine also accepts the provider and runs `opencode-go/<model>`.

### Fixed
- An OpenCode Go subscription key is no longer charged to a Zen balance, and a missing key gives a clear message instead of an *insufficient funds* answer from the wrong endpoint. Models the WRM engine cannot speak (OpenAI Responses, Google Gemini) are left out of the Go list instead of failing at the first prompt.

## [13.3.0] - 2026-10-22

### Added
- **AI addon loop.** The assistant can now write an addon into a draft workspace, `<DB>.addons/.drafts/<id>/`, instead of the live folder. New tools (`addon_list`, `addon_files`, `addon_create`, `addon_write`, `addon_enable`, `addon_disable`, `addon_delete`, `addon_run`, `addon_install_community`) are covered by the new `addons.manage` scope and are available to administrators only. `addon_create`/`addon_write` validate the draft exactly like a ZIP (manifest, layout, declared files, WebAssembly compile check) and can install it in the same call; installing a new draft always leaves it disabled.
- **Drafts in Settings → Addons.** Every draft is listed with its files, version, validity and whether the addon is installed, with **Diff** (a unified diff against the installed version), **Install draft** and **Discard**. Nothing in the workspace is served or executed until it is installed.
- **Approval in every mode.** Addon writes and `addon_run` always wait for the user's approval, including automatic mode and full access; a draft never enables itself.
- **Approvals are impossible to miss.** Every pending approval now also appears in the user's notifications (`ai.approval`, subscribable for email/Telegram/…), pops up as a fixed approval card even when the AI panel or the live view is closed, and plays a short sound. The AI panel keeps a sticky **Approval needed** bar with **Approve / Deny** above the chat, and every assistant reply plays a short sound; the speaker button in the panel header turns sound off for that browser. The global poll runs every five seconds instead of once a minute, and it includes built-in and OpenCode sessions, not only MCP.
- **`file_path` is accepted as an alias for `path`.** Some models send `file_path` (the name other tools use); WRM treated that as an unknown argument and refused the call before any approval was shown. The tool input now accepts both, with `path` winning when both are present.

### Fixed
- **A refused tool call can no longer look like a pending approval.** The assistant is told to claim a change only after a successful tool result; when WRM refuses the arguments, the tool card shows the error and no approval card is created.

### Security
- The addon tools act on WRM itself and are refused for non-administrators even when a token carries `addons.manage`; the scope is also refused when a non-administrator creates an AI connection, and it is not offered in their scope list.
- A draft write is validated in a temporary folder first, so a broken manifest or an unsafe path leaves the previous draft untouched. Draft ids and paths use the same strict checks as an installed addon, and installing goes through `addonInstallArchive`, the function behind an uploaded ZIP and the community library.

## [13.2.0] - 2026-10-22

### Added
- **Screen lock (#115).** After a chosen period without activity (2, 5, 10, 30 or 60 minutes, or off) the interface is covered by a lock screen that asks for the signed-in account's password; a **🔒** button next to Sign out locks it at once. Terminals, tunnels and AI runs keep working, and every unlock attempt is checked and audited (`auth.unlocked` / `auth.unlock_failed`, five attempts per minute). The choice is stored per browser; the lock is a presence check, not a second sign-in, and the session stays valid.

## [13.1.2] - 2026-10-22

### Changed
- **Service labels under `/home`.** Discovery descends one container level under `/home`, so `/home/<project>/<operator>/<service>` is recognised and labelled operator/service without naming any project; labels found this way are defaults and a root line in Monitoring settings (`path | service | operator`) overrides them.
- Documentation, tests, release notes and the community repository use neutral placeholders only; the history of the public branch was rewritten so no private deployment name remains in it.

## [13.1.1] - 2026-10-22

### Added
- **One-command Claude Code setup.** The plugin ships `/wrm-pro:connect` and `scripts/connect-claude.sh` / `.ps1`, which run `claude mcp add --transport http --scope user wrm <url> --header "Authorization: Bearer <token>"`. The README documents the two-line marketplace install and the values it asks for, including a local instance over `http://localhost:8080` (Claude Code opens a sign-in page only over HTTPS, so a local address needs the token header).
- **Release assets match the updater again**: `wrm-pro-v<version>-<os>-<arch>[.exe]` plus `SHA256SUMS.txt` for Windows, Linux (amd64, arm64, 386) and macOS (amd64, arm64). The in-app update failed with "the release has no file for this platform" because the assets were published without the `wrm-pro-` prefix.

### Changed
- Log discovery walks `/home` with the same bounded operator/service depth instead of one hardcoded home directory; documentation uses `/home/<operator>`.
- The generated version file lists deployments under `## Services`.

### Security
- Documentation, tests, release notes and the community repository no longer name private deployments or paths; examples use generic placeholders.

## [13.1.0] - 2026-10-22

### Added
- **Monitored services.** A monitored server can watch up to 20 services next to its metrics: systemd units (`systemd:nginx`) and Docker containers (`docker:web`). They are read on every SSH poll, in the same round trip as the metrics, and each sample carries the state (`active`, `running`, `failed`, `missing`, …). A watched service that is not `active`/`running` turns the sample into `problem` with a `service problem: …` message, so the existing notification and history path reports it without a second mechanism. The watch list lives in **Monitoring settings**, and **Poll automatically** can be switched directly on the server card.
- **Safer service checks.** Names are validated before they are stored (`systemd` units and Docker container names only) and the remote command is one fixed POSIX loop that receives the names on stdin through a quoted heredoc, so a name is data and never shell syntax.
- **Community library.** **Settings → Addons** lists the index of the public repository "WRM Pro - community public addons" with each entry's trust badge (**official**, **reviewed**, **community**, **unverified**), license and declared capabilities. Only an administrator installs; the download must be HTTPS from a GitHub host and must match the SHA-256 in the index, and the archive then passes the same manifest, layout, capability and WebAssembly checks as an uploaded ZIP. The source is stored as `community:<id>@<trust>` and every install is audited as `addon.installed`.
- **Recording level per account.** The new-user dialog gained **Session recording** — as the server settings, off for this account, session output only, or output and keystrokes. It is stored in `users.recording_level`, validated on create and on the user update endpoint, and resolved by `userRecordingLevel` for terminals, desktops and AI sessions alike. The server setting remains the master switch: turning recording off there still records nothing, and an account without a choice follows `session_recording_input`. The `terminal.open` and `desktop.open` audit entries now carry the level that was used.
- **Assistant work log.** While the assistant works, the chat shows one collapsible **Work log** per request: it opens with the request, names every tool step as it starts, and collapses to a single line with the step count when the answer arrives. The tool cards below stay the detailed record; the log is the overview you can ignore.
- **Replay shows wall-clock time.** The player puts the exact time at the playhead next to the offset (`0:12 / 3:40 · 22.10.2026. 12:01:05`) and the recording header gains **Ended**; the terminal cast always stored its Unix start in the asciicast header and the desktop recording has `started_at`, so both players show absolute time without re-recording anything and old recordings work too. The session events list already had absolute timestamps.

### Fixed
- **Ollama rejected a turn that only called tools** with `HTTP 400: invalid message content type: <nil>`. The request builder wrote `"content": null` for an assistant message without text; Ollama's OpenAI-compatible endpoint accepts only a string there, and every provider accepts an empty one, so the content field is now always a string.

### Security
- Service state is read with the SSH account and no extra privilege: `systemctl is-active` and `docker inspect` only. A missing container is reported as `missing`, not as a failed container, and a failed check marks the sample `problem` instead of hiding the previous state.
- The community index cannot point WRM anywhere: entries are accepted only from HTTPS GitHub hosts (HTTP is allowed for loopback so the tests can run), downloads are capped at 20 MB, redirects stop after five hops, and an archive whose SHA-256 differs from the index is refused before it reaches the installer. An entry that fails validation is dropped instead of hiding the rest of the list.

### Upgrade
Replace the executable or container and keep the database and encryption key. The database is upgraded in place: `users.recording_level` is added with an empty default (follow the server settings), and existing addons, recordings, monitor profiles and audits are untouched. 13.0.0 was never published on its own; its log engine, AI log tools and addon core are part of this release.

## [13.0.0] - 2026-10-22

### Added
- **Find logs is an explicit action.** Opening Monitoring → Logs no longer scans a server. Select a server and click **Find logs**; the result is cached for an hour per connection and configuration, and the file list is served from that cache. The tab states plainly that nothing was scanned yet.
- **Service inventory.** Discovery reports services, not just folders: systemd units, Docker containers, Supervisor programs, Git installs and plain folders. A service whose output goes to journald is listed as `journal` instead of disappearing. Services with no readable file log are shown with that reason.
- **systemd journal sources.** A log root can be `kind: journal` with a unit name. It is read with `journalctl --output=short-iso`, filtered by time range and keyword, and followed live by timestamp. Unit names are validated against a strict pattern and every other argument is generated server-side.
- **Broader log folder detection.** `log`, `logs`, `log-*`, `logs_*`, `*-log`, `*_logs`, and `WorkingDirectory/{log,logs,var/log,var/logs}` are recognised; project folders such as `catalog`, `dialog` or `blog` are not.
- **Application folders under a broad root are services.** A folder inside `/var/log` (or any configured root without a service label) that contains a `*.log` file is listed as a service, and every file in it is listed under that service name — including extensionless names such as `info` or `warn` that a logger writes next to `error.log`.
- **The assistant reads logs through that inventory.** `find_logs` lists the discovered services with their folders or journal unit and `read_log` opens one service's newest log, or its journal unit, with optional literal filtering. Both are ordinary read tools: allowed in read-only sessions, covered by the `read_logs` MCP scope, redacted, size-limited and audited. The cache of the Logs tab is reused; only the connection owner can trigger a scan and it runs over the session's existing SSH connection.
- **Addons, in the spirit of Omarchy.** An addon is a folder under `<DB_PATH>.addons/<id>/` with `addon.json`; it is installed from a ZIP in **Settings → Addons** and discovered at runtime, never compiled into the binary. This version implements the core: the manifest, a capability allowlist (`ui.panel`, `ui.theme`), install/enable/disable/remove with an audit entry for each, an archive checksum kept in the database, strict entry-name checks (absolute paths, `..`, backslashes, duplicates, oversize and missing declared files are refused), and a served asset endpoint limited to known file types.
- **Panels and themes.** A panel opens as a normal WRM window whose content is a sandboxed `iframe` (`sandbox="allow-scripts"`, no same-origin): it cannot read the page, cookies or other panels, and it receives only a small context through `postMessage` (user name, language, theme tokens, connection names and hosts) and can send a toast back. A theme is a stylesheet link applied while the addon is enabled and removed when it is disabled. Only an administrator installs, enables or removes; other users see the enabled addons.
- **Server code runs in a WebAssembly sandbox.** A manifest may declare `"server": {"module": "plugin.wasm", "exports": ["status"]}` with the `server.wasm` capability. The module is executed by `wazero` (pure Go, no CGO) in a fresh instance per call, with no WASI: no files, no network, no environment and no clock; the only import is `wrm.log`, the memory ceiling is 64 MiB and a call is stopped after five seconds. The ABI is two functions — `wrm_alloc(size) -> ptr` for the request buffer and `<export>(ptr, len) -> ptr` returning a NUL-terminated UTF-8 result. Administrators run a declared export from **Settings → Addons** (`POST /api/addons/{id}/call/{export}`, 64 KB input, 1 MB output), and every call is audited as `addon.call`. The installer compiles the module without running it and refuses an archive whose module is invalid, lacks `wrm_alloc` or its memory, misses a declared export, or imports anything but `wrm.log`.
- **AI addon action.** **Settings → Addons** generates a prompt from the capabilities this server actually reports — the manifest, the panel/theme rules, the WebAssembly ABI and its limits — with a **Copy prompt** button and a shortcut to the built-in assistant. WRM does not start another desktop app and does not trust what the AI writes: the result still arrives as a ZIP and goes through the same install checks.

### Changed
- Log discovery no longer runs inside the log-list request, so opening the tab cannot start an unbounded scan.
- Discovery keeps its depth and file caps; the root budget rises from 100 to 200 roots and the folder scan visits up to 400 directories.
- The addon installer accepts an `addon.json` written with a UTF-8 byte order mark, which editors on Windows add by default.

### Security
- Journal access uses the existing SSH account and the file-read permission, exactly like a file read; the SSH account still needs journal read access on the host.
- Discovery is audited as `monitor.logs_discovered` and stays owner-only; shares can read a journal source but cannot trigger a scan.
- Docker container names and log paths come from bounded reads of `config.v2.json`; no Docker socket or daemon command is used.
- Addons execute nothing at install time: the manifest is validated and the archive unpacked into its own folder, and a capability that is not implemented refuses the whole install instead of being ignored. Panel HTML is served with `Content-Security-Policy: sandbox allow-scripts` and same-origin framing only; other documents stay unframeable. Server code is WebAssembly only (no native binaries, no shell), runs without WASI or host filesystem access, is limited to 64 MiB and five seconds, and only an administrator can call it. The addon settings tab carries the community and no-warranty notice.

## [12.8.1] - 2026-10-21

### Security
- **Go 1.27.3 fixes GO-2026-6809 / CVE-2026-111943.** The standard library's `crypto/x509` verifier now rejects known compromised intermediate certificates instead of trusting them through an older system trust store. The module toolchain is `go1.27.3`, and the security workflow's `govulncheck` run is clean. WRM reaches the affected API in `nettools.go` when it verifies an HTTP check's certificate chain.
- **gosec G702 at `opencode_sandbox.go:492` is a reviewed false positive, not a command injection.** gosec follows the operator-set `WRM_MEDIUM_DOCKER` value from `mediumDockerCLI()` as tainted input. The flagged `docker inspect` passes the internally generated `wrm-opencode-<session>-<random>` container name as a separate argument, never through a shell. The call is now in the reviewed baseline with that reason; the existing G204 entry for the same line is unchanged.

### Upgrade
Replace the executable or container and keep the database and encryption key. No database migration is required. This release only updates the Go runtime and the reviewed security baseline.

## [12.8.0] - 2026-10-21

### Added
- **Recursive folder deletion.** Deleting a folder now deletes every subfolder and every connection inside it, together with its tunnels, SSH-key deployments, snippets, bookmarks and share entries, in one database transaction. Nothing is moved to the root first. The confirmation shows how many folders and connections will be removed, and the API returns both counts.
- **Reliable folder and connection moves.** Folders and connections have a **Move to folder…** action. Connection drag-and-drop moves all selected connections, and folder/connection positions persist after reload. A folder cannot be moved into itself or one of its descendants.
- **File-manager permissions.** The context menu can change permissions with a three- or four-digit octal mode. Folders can apply it recursively; if SSH reports a permission error, WRM offers an optional sudo password and retries. FTP limitations are reported clearly.
- **Upload queue.** Additional uploads dropped into a file-manager window are queued and start automatically when the current upload finishes. Each queued job keeps the destination folder it was dropped into.
- **Tunnel filters and proxy shortcuts.** The Tunnels panel can filter by name/description, port, host or route, and can show active tunnels only. Proxy settings are available from **Tools** and from the Tunnels panel.
- **Terminal title-bar actions.** Owned SSH and SFTP windows have **Edit** and **Tunnels** buttons in the title bar.
- **Tab scroll arrows.** Settings and Admin tab rows, and the folder tab bar, get left/right arrows when they overflow. The Settings window now uses the same size as the Admin window.

### Fixed
- **Shared and collaboration folders keep their hierarchy.** Nested shared folders are returned by the API and rendered as a tree instead of flattened into the root, including the share room.
- **Case-insensitive name ordering.** Connections and shared connections sort by name case-insensitively, and folder dropdowns and paths compare names case-insensitively. Folder sibling order stays manual: a new folder is appended and can then be moved.
- **No duplicate saves.** **Save Connection** and **Save Folder** disable during the request and show a saving state, so a double click cannot submit twice.
- **Folder `+ Connection` is pre-filled.** Creating a connection from a folder context menu selects that folder and inherits its jump-host and proxy defaults.
- **Delete confirmation describes the real result.** The old text promised that connections would be moved to the root; it now reports the recursive folder and connection counts.

### Upgrade
Replace the executable/container and keep the database and encryption key. No database migration is needed. Recursive deletion is intentional: deleting a folder removes everything inside it.

## [12.7.4] - 2026-10-20

### Added
- **Remote desktop works out of the box on Windows and macOS.** When guacd is not reachable and `guacd_address` is local, WRM starts the official `guacamole/guacd:1.5.5` container in Docker Desktop (name `wrm-guacd`, published on `127.0.0.1:4822` only, restart policy `unless-stopped`) and waits for it. The first RDP/VNC/Telnet click can start it; later starts happen when WRM starts. A custom (non-local) guacd is never touched.
- The new policy **Start guacd automatically** (`guacd_auto`, default on) controls this. Admin → Overview shows the state and has a **Start guacd** button; `-setup-guacd` sets the container up once from the command line.
- If Docker Desktop is missing, stopped or set to Windows containers, the desktop window says exactly what to do (including switching Docker Desktop to Linux containers) instead of only showing the old Linux instructions.
- **Every textual file opens in Preview, with the right formatting.** Text is now detected from the content instead of a short extension allowlist, so scripts, configuration files and files without an extension open as text too. UTF-8, UTF-16 (with a BOM or a detected byte order) and single-byte text are decoded; binary files still get the clear 415 answer. A bundled PrismJS 1.29.0 highlights XML/SMIL/SVG, JSON, YAML, TOML, INI, shell, Python, JavaScript/TypeScript, Go, SQL, CSS/SCSS/LESS, Markdown, PowerShell, batch, Dockerfile, Makefile, C/C++/C#/Java/Rust/Ruby/PHP and similar files, offline and without a CDN. The 256 KiB text limit and the escaped-text safety policy are unchanged.

### Fixed
- **RDP on Windows no longer fails with “Remote desktop needs guacd … but it is not reachable”.** WRM manages guacd itself instead of requiring a manual `docker run` or a Linux machine.

### Upgrade
Replace the executable/container and keep the database and encryption key. No database change. On Windows/macOS install or start Docker Desktop; then the first desktop connection (or Admin → Overview → Start guacd) sets guacd up. Linux keeps using the system guacd or docker-compose.

## [12.7.3] - 2026-10-20

### Added
- **Medium build locations.** A project with a `build` step can now deploy with the **WRM native** engine. For each project, choose where the build runs: on the WRM host, on a chosen Linux build server over SSH, or not at all (`none`). The choice is stored per user and project and audited (`git.medium_build_set`).
- The whole commit is prepared, the `build` command runs in the repository root, and `source_dir` is read from the result. A build server receives a temporary directory containing only the prepared source (no Git metadata and no SSH keys); the build runs through the account's login shell, and the temporary directory is removed afterwards.
- The plan shows the build location, the build's platform and architecture, and the last build output. A destination whose platform or architecture differs from the build is refused with the reason. WRM-host builds need a POSIX host with the toolchain; on Windows, choose a Linux build server or no build.

### Fixed
- **Monitoring → Logs discovers every service under `/home/<operator>`.** The first level is now read as the operator and the second as the service, so the Service and Operator dropdowns show the correct values. Virtual environments, builds, distributions, archives, temporary and private-key directories are skipped instead of consuming the visit limit.
- Service `log` and `logs` folders list their log files, including FTP `.txt` logs, and skip compressed archives; `/var/log` and broad roots keep the name filter. Archive, backup and old subfolders are pruned. Discovery and listing have larger time budgets (20 s and 60 s), so large hosts no longer return an incomplete list.

## [12.7.2] - 2026-10-19

### Added
- **The deployed version is read from the code, not from `VERSION.md`.** The Git workspace compares every tracked file on the server with the repository tree at the target and at the earlier tags. All files matching one version show that version; a consistent older version is named exactly from its commit tree; a mix of versions or a hand-edited file shows **Mixed / modified** with the per-file details. Deployments and updates still write `VERSION.md` for compatibility with the original tool.
- **Optional `VERSION.md` check.** A checkbox next to *Check now* (off by default) also reads `VERSION.md` and warns when it disagrees with the code. With it on, the commits list follows `VERSION.md`; with it off, the list follows the detected commit.

### Changed
- The installs table, the details dialog and the Simple view show the detected version and commit. The per-environment behind/drift comparison uses the detected commit and falls back to the deploy record only when the code cannot be identified.
- Older stored targets and offline bundles without version trees keep working through the per-file history fallback; they are marked *approx.* when no full tree is available.

## [12.7.1] - 2026-10-19

### Fixed
- **Medium deploy-config: per-environment server lists** (`medium_config.go`): `environments.yaml` now accepts the original deploy tool's `servers: [name, ...]` key. A listed server must exist in `servers.yaml` exactly once; an empty list is refused because it would forbid every deploy; and every project target in that environment may only use a listed server. Environments without the key keep every server. Errors name the file and the key, and `medium_env_servers_test.go` covers valid, unknown, duplicate, empty and out-of-list cases.

## [12.7.0] - 2026-10-18

### Added
- **Medium native engine: deployments and rollbacks** (`medium_deploy.go`, `medium_rollback.go`, `medium_state.go`, `medium_host.go`, `medium_deploy_api.go`): WRM deploys a project from its WRM Git source to every destination of an environment through the user's WRM connections (SFTP plus `sh` over SSH), without Docker and without the original tool. The server state is the original tool's, byte for byte: `lock/` (created with `mkdir`) and `lock/owner.json`, `current.json`, `history.jsonl` (compact JSON, the original's key order, every field present), `backups/<deploy_id>/` with `<deploy_id>.added` and `<deploy_id>.dirs`, and `rollback-<id>/` for rollbacks. Either tool reads, continues after and rolls back what the other wrote.
- **Deployment sequence per destination:** lock; compare again under the lock and stop when the result differs from the reviewed plan (nothing written); manifests; removals deepest first into the backup; new directories; uploads to temporary files next to their targets (four at a time, new files 0664/0775 minus the server's umask, changed files keep their permissions); SHA-256 check of every upload; replacement (the old file goes to the backup first); `post_deploy` in the application directory through the login shell; a final check; `current.json` and history; pruning (the current deployment and the last five applied ones keep their backups, rollbacks their newest five); lock released. A failure after the first change writes a partial history entry (`current: false`) so it can be rolled back.
- **Dry run:** takes the lock, compares again under it and releases it, changing nothing.
- **Rollback:** WRM chooses the deployment to undo and what it restores from the history as the original tool does (applied deployments that changed files, rollbacks that completed, `previous` where recorded), restores files, symbolic links and directories with their modes from the backup, removes what the deployment added, and records a `ROLLBACK` entry. A dry rollback shows what would happen. The chosen deployment must still be the last one when the rollback starts.
- **Locks:** a deployment by someone else (WRM or the original tool) is reported with its owner and age. A lock older than the longest a deployment may take (`medium_command_timeout` and `medium_transfer_timeout`, at least 30 minutes) can be released by hand, only if its owner did not change meanwhile (`POST /api/git/medium/native/lock/release`, step-up, audited).
- **Per-project engine** (table `medium_project_engine`, `POST /api/git/medium/native/engine`): every project starts on the Docker runtime (the original tool deploys it); a user allowed by the new policy `medium_native` (default `admins`) switches it to the WRM native engine and back at any time, since the server state is the same.
- **Leave changes out:** in the plan, tick changes to leave out of this deployment and compare again; the plan keeps them as they are on each server and `current.json` records them in `excluded`.
- **WRM engine panel** (Medium): the engine switch per project, exclusions, the typed environment confirmation where `confirm` is set, *Dry run* and *Deploy*, a live log with the outcome per destination and *Cancel* (it stops before the next destination starts changing), and *History and rollback* with the current version, the lock with its owner and *Release lock*, the recent entries with their changes, and *Dry rollback* / *Roll back* grouped by deployment.
- **Jobs** (`GET /api/git/medium/native/jobs`, `/jobs/{id}?from=`, `POST /jobs/{id}/cancel`): one running deployment or rollback per user, kept for an hour, each with a row in the Medium activity log (`native_deploy`, `native_deploy_dry_run`, `native_rollback`, `native_rollback_dry_run`). Stopping a user or a lockdown cancels their jobs before the next destination.
- New settings `medium_command_timeout` (300 s) and `medium_transfer_timeout` (3600 s).
- Audit: `git.medium_engine_set`, `git.medium_native_deploy`, `git.medium_native_rollback`, `git.medium_native_<kind>_done`, `git.medium_native_lock_released`, `git.medium_native_cancelled`. Deploying, rolling back and releasing a lock need step-up (`medium.deploy`, `medium.rollback`, `medium.lock_release`).

### Changed
- A comparison's result carries a `revision` (over the commit, the exclusions and every server's plan) and `deployable`; a deployment names the revision it was reviewed at and is refused when the plan changed. A plan is deployed at most once.
- Projects with a build step are still deployed by the original tool; build locations (WRM host, a Linux build server over SSH, none) come in v12.7.3.
- A repository path that is absolute or has empty, `.` or `..` components stops the comparison.
- Symbolic links created over SFTP are checked and made with `ln -s` when the SFTP server changed their target.

## [12.6.26] - 2026-10-18

### Added
- **SIEM export** (`siem.go`, table `siem_destinations`): every audit entry goes to up to 20 destinations, in order and at least once. Kinds: *syslog* (RFC 5424 over TLS with RFC 5425 octet counting, structured data `wrm@32473`, facility `authpriv` by default, optional client certificate), *JSON lines* file on the WRM host (absolute path ending in `.jsonl`, `.json` or `.log`, no symbolic links, mode 0600, rotated at 100 MB keeping three files), *Splunk HEC*, *Elastic* (`_bulk` into `wrm-audit` or a chosen index, optional API key), *Microsoft Sentinel* (Logs ingestion API with an Entra application's client credentials, data collection rule and stream) and *generic HTTPS* (a JSON array, optional bearer token). Each event carries `@timestamp`, `event.action` / `category` / `outcome`, the user with role, e-mail and directory identity, `source.ip`, target, details, connection and session IDs, and the audit hash chain (`hash.prev`, `hash.this`). A destination keeps its own position in the audit log (`cursor`), so nothing is lost while it is unreachable; it is retried with growing pauses (5 s up to 5 min). A new destination starts with new events or with the whole kept audit log. An action filter (prefixes such as `auth.`, `security.`) limits what is sent. Secrets (HEC token, API key, client secret, private key) are encrypted at rest, never sent to the browser, and never sent to a changed address.
- **Admin panel → Security → SIEM export:** destinations with a status light (*Delivering*, *Behind*, *Failing*, *Off*), sent and waiting counts, the last delivery, the last error with its time, the next try, *Test* (sends a `siem.test` event) and an editor with the fields of each kind. Creating, changing and deleting a destination needs step-up and is audited (`siem.destination_created`, `siem.destination_updated`, `siem.destination_deleted`, `siem.tested`, `siem.test_failed`).
- **Security alerts** (`alerts.go`): rules on the audit log raise a `security.alert` entry (it reaches the SIEM like everything else), a Workboard inbox entry and the new `security.alert` notification for every active administrator. Rules and settings: repeated failed sign-ins from one address or for one name (`alert_failed_signins`, default 10, within `alert_failed_signins_minutes`, default 10; password, 2FA, step-up and AI token sign-ins), a break-glass sign-in (`alert_break_glass`, SIEM only, administrators are already told), a new administrator by any route (`alert_new_admin`), a revealed or exported secret (`alert_secret_reveal`), an AI command approved under full access that WRM otherwise refuses (`alert_ai_full_access`), and the audit log or a SIEM export turned off or deleted (`alert_audit_off`). *Admin panel → Security* lists the recent alerts; *Policies → Security alerts* holds the rules.
- **Stop user** (`POST /api/admin/users/{id}/stop`, user menu): one click disables the account and ends its sign-in sessions, terminals, collaboration rooms, tunnels, AI sessions and AI connections, then shows how many of each were ended (audited `admin.user_stopped`, step-up). Not for yourself or the last administrator.
- **Lockdown** (`lockdown.go`): in an incident only break-glass administrators can sign in or keep a session; everyone else is signed out at once (sessions, terminals, rooms, tunnels), AI and AI connections are switched off as with the AI kill switch, and shares are refused (`lockdown`). Turned on from *Admin panel → Security* with a reason everyone sees (fresh step-up, refused while there is no break-glass administrator), on the WRM host with `wrm -lockdown on -lockdown-reason "…"` / `off` / `status`, or forced with the `WRM_LOCKDOWN=1` environment variable. A lockdown set from the command line or by another instance on the same database applies within five seconds. Only a break-glass administrator ends it in the UI. The sign-in page shows it with the reason (`/api/auth/config` returns `lockdown`). Audited `security.lockdown_on`, `security.lockdown_off`, `security.lockdown_enforced`.

### Changed
- Security events (`security.*`: alerts, lockdown) are always recorded, like administrator actions, even with `audit_enabled` off.
- AI approvals record the policy rule that asked for the approval (`rule`, for example `full_access`).

## [12.6.25] - 2026-10-18

### Added
- **LDAP / Active Directory sign-in** (`ldap.go`, `ldap_proto.go`, tables `ldap_directories`, `ldap_identities`): people sign in on the normal sign-in page with their directory name (`name`, `DOMAIN\name` or `name@domain`) and password. WRM looks the person up with a service account, then checks the password by binding as that person; the password is never stored. The connection is always encrypted: `ldaps://` or `ldap://` with StartTLS (TLS 1.2 or newer, certificate checked against the system store or a CA certificate pasted per directory). An empty password is refused before it reaches the server, because servers treat it as an anonymous bind. The LDAP client is WRM's own (BER, simple bind, paged search, StartTLS, an RFC 4515 filter compiler that escapes every value).
- **Directory kinds:** *Active Directory* (sAMAccountName or userPrincipalName, `objectGUID`, nested groups through `LDAP_MATCHING_RULE_IN_CHAIN`) and *other LDAP* (OpenLDAP, 389 Directory Server, FreeIPA and similar: `uid` or `mail`, `entryUUID`, groups through `memberOf` or `member` / `uniqueMember` / `memberUid`, nested up to ten levels). The user filter, sign-in attribute, ID attribute and group base can be changed under *Advanced*.
- **Immutable identities:** a directory account is keyed by its `objectGUID` (`guid:<hex>`) or the chosen ID attribute, never by name, so a renamed or re-used name does not reach someone else's account.
- **Mapping rules** per directory (the same rules as directory sign-in): a group name or DN (nested groups included) or any user attribute gives a WRM role (the strongest wins) and teams. Accounts are created on the first sign-in only for people a rule matches; everyone else is refused with a message that says why. Disabled, locked and expired accounts are refused (`userAccountControl`, `msDS-User-Account-Control-Computed`, `accountExpires`, `nsAccountLock`, `loginDisabled`, `pwdAccountLockedTime`).
- **Directory sync** (`ldap_sync_minutes`, default 15): WRM checks every linked account with the service account. A deleted, disabled, locked or expired account, or one that left every mapped group, loses access at once (`ldap.access_ended`); role and team changes are applied (`team_members.source = 'ldap'`). *Check now* runs it on demand and shows how many accounts were checked and ended.
- **SCIM 2.0 provisioning** (`scim.go`, tables `scim_clients`, `scim_users`, `scim_groups`, `scim_group_members`; endpoint `/scim/v2`): Microsoft Entra ID, Okta and other identity providers create, change, disable and delete WRM accounts and groups at once. Each provisioning client has its own bearer token (`wrm_scim_…`, shown once, stored only as SHA-256, rotatable and revocable) and sees only the accounts and groups it created. Users, Groups, `ServiceProviderConfig`, `ResourceTypes` and `Schemas`; `filter=attr eq "value"`, paging, PATCH in the forms Entra and Okta send. `active=false` or DELETE disables the account and ends all its access; WRM keeps the account so its work can be handed over. Groups become WRM teams named `<group> (SCIM)`, a team made by hand is never taken over, and mapping rules on the client turn groups into roles. The last active administrator is never disabled or demoted (`scim.disable_refused`). 600 requests a minute per address, 1 MB per request.
- **SCIM and directory sign-in together:** when a SCIM account first signs in through OIDC, WRM links it if its `externalId` equals the token's `oid` (Entra) or `sub`, and only if the account has no directory link yet (`oidc.scim_account_linked`).
- **Admin panel → Directory sign-in:** an *LDAP / Active Directory* section (directories with kind presets, *Test connection* step by step: address, connection, TLS and certificate, service account, base DN, users, the test user and their groups, each with what to fix; *Test sign-in* that shows the account, groups, attributes and what WRM would do without creating anything; *Check now*; linked accounts with their last check and unlinking) and a *SCIM provisioning* section (the endpoint address to paste into the identity provider, clients with token rotation and revocation, provisioned accounts). Saving, deleting, unlinking and token changes need step-up.
- **Settings → Security → Organisation accounts:** link a directory account to an existing WRM account with the directory name and password (after a fresh confirmation, even with step-up off). The account then signs in with its directory password.
- **One path that ends access** (`endUserAccess`, `directory.go`): an administrator disabling an account, the directory refusing it (OIDC revalidation, LDAP sync) and SCIM deprovisioning all end the user's sign-in sessions, terminals, tunnels, collaboration rooms, AI sessions and AI connections (MCP tokens).

### Changed
- A stored secret is never sent to a changed server address: changing an LDAP directory's address asks for the service account password again, and changing an OIDC provider's issuer asks for the client secret again.
- Password sign-in limited to break-glass (`local_login = break_glass`) also applies while an LDAP directory is enabled; directory users sign in with their directory password on the same form.
- `/api/auth/config` returns `ldap` (the directories' names); `/api/auth/me` returns `directory_password` (the account confirms with its directory password). Accounts that sign in with a directory password cannot change it in WRM, and confirm step-up and 2FA changes with it.
- LDAP sign-in and sync errors are logged and audited by kind only (for example *connection refused*, *LDAP error: invalid credentials*), never with the server's message or the typed name.

### Fixed
- Automatic releases: the `Tag new version` job was always skipped on `main`, because the build jobs it waits for depend on the release job that only runs on tags. v12.6.24 was therefore not released on its own; this release includes it.

## [12.6.24] - 2026-10-18

### Added
- **Directory sign-in (OIDC)** (`oidc.go`, `oidc_jwt.go`, tables `oidc_providers`, `user_identities`): sign-in through Microsoft Entra ID, ADFS, Okta, Keycloak, Google Workspace or any OpenID Connect provider with the authorization code flow and PKCE (S256), `state` bound to the browser by a cookie and used once, and a `nonce`. ID tokens are verified with the standard library (RS256/384/512, PS256/384/512, ES256/384/512 from the provider's JWKS, keys refetched on rotation; `none` and HS* refused; issuer, audience, `azp`, expiry, `iat`/`nbf` with two minutes of clock skew). Provider addresses must use HTTPS (plain HTTP only on loopback).
- **Immutable identities:** a directory user is keyed by Entra's `tid` + `oid`, otherwise `iss` + `sub`, never by name or e-mail.
- **Mapping rules** per provider (`claim`, `value`, `role`, `teams`): app roles, group IDs or any claim give a WRM role (the strongest wins) and teams. Accounts are created on first sign-in only for matched users (`jit`), without a password (`users.sso_only`); unmatched users are refused with a message that says why (and a hint for Entra group overage). Every sign-in syncs the role and the SSO team memberships (`team_members.source = 'oidc'`); hand-added teams stay, the last active administrator is never demoted, break-glass accounts cannot be linked.
- **MFA proven by the token** (`require_mfa`, `mfa_values`): `amr`, or accepted `acr` / Entra `acrs` values requested through `claims` (Entra) or `acr_values`. A provider that proves MFA satisfies `require_2fa`; otherwise accounts with WRM's own 2FA enter their code after the directory sign-in (the signed MFA token carries the identity).
- **Revalidation** (`oidc_revalidate_minutes`, default 60; `oidc_unverified_hours`, default 12): WRM refreshes each signed-in directory identity with its refresh token (encrypted at rest). A refused refresh, a refreshed token no rule matches, or no successful check within `oidc_unverified_hours` ends the user's sessions, terminals, tunnels, rooms and AI sessions (`oidc.access_ended`).
- **`local_login`** (`all` / `break_glass`): with `break_glass`, password sign-in and self-registration are limited to break-glass administrators while a provider is enabled. The Enterprise security level requires it.
- **Admin panel → Directory sign-in:** providers with presets and setup hints, step-by-step *Test connection* (`POST /api/admin/oidc/check`), rule editor with a claims preview (`POST /api/admin/oidc/{id}/preview`), *Test sign-in* in a new window that shows the claims and what WRM would do without creating anything, the linked accounts with their last directory check, and unlinking. Saving, deleting and unlinking need step-up.
- **Sign-in page:** *Sign in with <provider>* buttons; with `local_login = break_glass` the password form is folded behind *Emergency sign-in with a local account*.
- **Step-up through the provider** (`prompt=login`, `max_age=0`, the same directory identity) for accounts without a password; 2FA changes of such accounts use that confirmation instead of a password.
- **Settings → Security → Organisation accounts:** link a directory account to an existing WRM account (the provider must confirm it, and WRM asks for a fresh confirmation first, even with step-up off; an existing link is never replaced silently) and unlink it.
- The project's release public key is built in (`builtinReleaseKeys`): self-update refuses a release whose `SHA256SUMS.txt.sig` is missing or does not verify, and the release workflow signs with the `RELEASE_SIGNING_KEY` secret. A test checks that every built-in key parses.
- **Automatic releases:** the `Tag new version` job in `build.yml` tags a push to `main` whose version is new (`APP_VERSION`, `AppVersion` and the top CHANGELOG section must agree) once every build passed, and starts the release on that tag.

### Changed
- A failed AI connection sign-in (`mcp.auth_failed`) records the first 12 hex characters of the presented token's SHA-256 instead of its last four characters.
- `/api/auth/config` returns `sso` (the sign-in buttons) and `local_login_limited`; `/api/auth/me` returns `sso_only` and `sso`; the admin user list shows which accounts are linked to a directory; a step-up answer carries `password` and `sso`.
- The sign-in page is shown in the chosen language (it stayed partly English before signing in).

## [12.6.23] - 2026-10-17

### Added
- **Roles** (`roles.go`): every account is an *Administrator*, *Security auditor*, *Operator* or *User* (`users.role`; administrators stay `is_admin=1`, so a previous binary sees the same administrators). Auditors may `GET` the audit log, terminal sessions and recordings, file transfers and the security posture (`requireAuditRead`), and are refused connections, the AI assistant and AI connections. Roles are set on create (`role`) and with `PUT /api/admin/users/{id}` (`role`); switching to auditor ends the user's terminals and AI sessions.
- **Teams** (`teams.go`, tables `teams`, `team_members`, `share_teams`): *Admin panel → Teams* with members and leads (`GET/POST /api/admin/teams`, `PUT/DELETE /api/admin/teams/{id}`, audited `admin.team_*`), `GET /api/teams` for the share dialog. Shares in *Members only* mode take teams with a role (`teams: [{team_id, role}]`); members get the best of their team and direct roles, and leaving a team revalidates open share connections.
- **Who-policies accept `operators` and `teams:<names>`** besides `off`, `admins` and `all` (`whoPolicies`, `policyAllows`): tunnels, remote forwarding, serial ports, network tools, proxies, the Git actions, Medium, Bitwarden, the AI assistant, personal AI keys, OpenCode, the OpenCode sandbox and AI connections. Team names are validated against existing teams; a team named in a policy cannot be deleted or renamed.
- **Step-up re-authentication** (`require_step_up`, `step_up_minutes`, `POST /api/auth/step-up`): changing policies or the security level, creating administrators or auditors, changing roles, disabling or deleting users, break-glass changes, password and 2FA resets, revealing a credential password, exporting a private key and exporting connections with secrets answer `403 {step_up: true}` until the session confirms with the password (and TOTP or a recovery code). Signing in counts as a confirmation. The UI asks and repeats the request by itself. Required by the Business and Enterprise security levels. Audited `auth.step_up`, `auth.step_up_failed`.
- **Break-glass administrator** (`users.break_glass`, `break_glass_ips`): must use 2FA, may only sign in from the allowed networks, and every sign-in is audited (`auth.break_glass_login`) and notified to the other administrators (event `security.break_glass_login`).
- **Handover on delete:** `GET /api/admin/users/{id}/handover` lists what a user owns and suggests their team lead; `DELETE /api/admin/users/{id}?transfer_to=<id>` moves connections, folders, credentials, SSH keys, shares and proxies to that account first (audited `admin.user_handover`). The delete dialog offers it.

### Changed
- `/api/auth/me` returns `role`, `is_auditor`, `break_glass`, `teams` and `step_up`; the admin user list returns `role`, `break_glass` and `teams`. Disabling a user also ends their AI sessions and AI connections.
- The share dialog shows a hint text when no members were added (the keys were missing).

### Fixed
- Release workflow: the check whether the build has a built-in release key always answered yes (with `pipefail`, the failing probe piped into `grep` failed the condition), so every release without `RELEASE_SIGNING_KEY` stopped at *Sign SHA256SUMS.txt* before the SBOM and provenance.

## [12.6.22] - 2026-10-17

### Added
- **Security levels** (`security_level.go`): the new setting `security_level` is *Home* (default, today's behaviour), *Business* or *Enterprise*. Each level sets minimums for the settings that matter (`levelRules`): HTTPS, two-factor authentication, password length, session lifetime and idle time, failed sign-ins, registration, host-key policy, server-side keys, secret export, guest links and their lifetime, AI full access, audit and its retention, session recording and its retention. A higher level raises weaker settings; stronger settings stay as they are.
- **Choosing the level:** on the first-run screen (the first administrator picks it; over plain HTTP the HTTPS requirement is recorded as a deviation to fix later), with `wrm -security-level home|business|enterprise` (also kept by `-install-service`), with `WRM_INITIAL_SECURITY_LEVEL` (applied once, while no level was chosen) or `WRM_SECURITY_LEVEL` (forced and locked), and later in **Admin → Security**.
- **Change preview:** `POST /api/admin/security/level` with `preview` returns what changes, what an environment variable keeps, what blocks the change (for example HTTPS while this browser uses HTTP) and whom it affects (users who must set up 2FA, sessions that end, guest links that become too long or are blocked, passwords checked on the next change). Applying can keep a setting as a **deviation** with a written reason, and can sign out all other sessions at once.
- **Deviations:** a policy weaker than the level asks for a reason (`_deviation_notes` in `PUT /api/admin/settings`); without one the API answers 400 with the required value. Deviations are listed with who recorded them and when, audited (`security.deviation`, `security.deviation_removed`) and removed when the setting meets the level again.
- **Security posture** (`security_posture.go`, `GET /api/admin/security`): Admin → Security grades the installation against the level: transport, every level rule, administrators and users without 2FA, where the encryption key lives, `WRM_ALLOW_ANY_ORIGIN`, guest links without expiry, signed updates, the current version and the audit hash chain. Each item shows the value, the requirement, the explanation and the fix. The report exports as JSON or prints to PDF for auditors.
- New settings `require_https` (sign-in, registration and 2FA only over HTTPS or from localhost) and `link_share_max_days` (longest lifetime of a guest link; 0 means no limit).
- Audit events `security.level_changed`, `security.level_enforced`.

### Changed
- `PUT /api/admin/settings` validates every value before it saves any, and `GET` returns the level's requirement and any deviation per setting; Policies show both.
- `GET /api/auth/config` returns `security_level` on first run and `https_required` when sign-in needs HTTPS; the admin status includes `security_level`.

## [12.6.21] - 2026-10-17

### Added
- **Signed releases** (`releasesign.go`): the release workflow signs `SHA256SUMS.txt` with ed25519 (`SHA256SUMS.txt.sig`; repository secret `RELEASE_SIGNING_KEY`). The signed message is the fixed context `WRM release signature v1`, the file name and the file's bytes. Public keys look like `wrm-ed25519:<base64>`; built-in keys live in `builtinReleaseKeys` (empty in this version), extra keys in the new setting `update_signing_keys` (validated and normalized).
- **Self-update checks the signature:** with a known release key a missing or invalid `SHA256SUMS.txt.sig` refuses the update (`system.update_failed`, step `signature`); `system.update_verified` records the signing key. Without a key the update verifies the SHA-256 only. `GET /api/update` returns `signature_keys` for administrators, and the update dialog shows *Signature: required* or *not checked*.
- **Command line:** `-release-keygen FILE` (new key pair; private key 0600, never overwritten; public key printed), `-release-sign FILE` (key from `WRM_RELEASE_SIGNING_KEY` or `-release-key-file`), `-release-verify FILE` (signature plus the SHA-256 of the listed files next to it; `-release-public-key` adds a key).
- **SBOM and build provenance** in the release workflow: a CycloneDX SBOM (`sbom-vX.Y.Z.cdx.json`, cyclonedx-gomod v1.12.0) and `actions/attest-build-provenance` for the binaries, `SHA256SUMS.txt` and the SBOM (`id-token: write`, `attestations: write` for that job only). With a built-in key and no `RELEASE_SIGNING_KEY` the release fails instead of publishing an unsigned release.
- **Nightly security tests** `.github/workflows/nightly.yml` (daily, on demand, and briefly on pull requests that change them): Go fuzzing of seven parsers (`fuzz_test.go`: mRemoteNG XML, SSH config, CSV/XLSX, PuTTY `.reg`, WinSCP bookmarks, Git configuration files, deploy-config YAML; a crashing input is kept as an artifact) and an OWASP ZAP baseline scan of a running WRM with HTTPS, signed out and with an administrator's session (reports as an artifact).

### Fixed
- PuTTY import: a `.reg` key with invalid UTF-8 before the `Sessions` path made the parser panic (found by the fuzz test; the input is kept in `testdata/fuzz`). The same byte-offset mistake is fixed in the Git web link of a source URL.

## [12.6.20] - 2026-10-17

### Added
- **Security workflow** `.github/workflows/security.yml` (every push and pull request to `main`, weekly on Monday, manual): **govulncheck** v1.9.0 (fails on a reachable vulnerability), **staticcheck** v0.8.2 (`remote-manager/staticcheck.conf`: default checks without ST1005, ST1013 and ST1018), **gosec** v2.29.0 without G104, compared with the reviewed baseline `.github/security/gosec-baseline.json` by `tools/gosec-baseline.js` (fingerprint: rule, file and the flagged line's text, so moved code is not new; fails on new findings and when gosec reports load errors; runs with Go 1.26.9 because gosec v2.29 cannot read Go 1.27 export data; the baseline is the union of about 20 runs, because the taint rules are not deterministic), **gitleaks** v8.30.1 (the commits of the push or pull request, the whole history on the weekly run; `.gitleaks.toml`: default rules, vendor files and Go test files allowed), **actionlint** v1.7.12 and **zizmor** 1.31.1 over the workflows, and **dependency review** on pull requests (fails from moderate).
- **CodeQL** `.github/workflows/codeql.yml`: Go (manual build) and JavaScript, `security-extended` queries, vendor files ignored; push, pull request, weekly.
- **OpenSSF Scorecard** `.github/workflows/scorecard.yml`: push to `main` and weekly, results published and uploaded to code scanning.
- **Claude security review** `.github/workflows/claude-security-review.yml` (`anthropics/claude-code-security-review`): comments security findings on pull requests from branches of this repository (never forks), with WRM's checklist `.github/security/claude-review-instructions.txt` and exclusions `.github/security/claude-review-false-positives.txt`; skipped with a notice while the `ANTHROPIC_API_KEY` secret is not set.
- **Trivy** image scan in the Docker job of `build.yml`: HIGH and CRITICAL shown, fails on a fixable CRITICAL vulnerability.
- **Dependabot** `.github/dependabot.yml`: Go modules (`/remote-manager`), GitHub Actions and Docker, weekly, minor and patch grouped.
- Repository skill `.claude/skills/wrm-security-review/` and a *Security review* step in `CLAUDE.md` → *Before every push*.
- ROADMAP: the accepted security and enterprise identity plan (v12.6.21 signed releases, v12.6.22 security levels Home / Business / Enterprise and the security posture page, v12.6.23 roles and teams, v12.6.24 OIDC, v12.6.25 LDAP / AD and SCIM, v12.6.26 SIEM export and containment).

### Changed
- **Go 1.27.2:** `go.mod` declares `go 1.26.0` and `toolchain go1.27.2` (was `go 1.23.0`, a release without security fixes, which CI used for every binary); CI checks that the toolchain in use matches `go.mod`. Docker build stage `golang:1.27-alpine`.
- **Modules updated:** `golang.org/x/crypto` v0.58.0, `golang.org/x/sys` v0.49.0, `modernc.org/sqlite` v1.60.1, `github.com/pkg/sftp` v1.13.11, `github.com/jlaffaye/ftp` v0.2.4 and their dependencies; `gopkg.in/yaml.v3` is a direct dependency.
- **Workflows:** `permissions: contents: read` by default, `contents: write` only for the jobs that upload release assets; every action pinned to a full commit SHA; `actions/checkout` with `persist-credentials: false`.
- **Medium proxy** (`git_medium.go`): `httputil.ReverseProxy` with `Rewrite` instead of the deprecated `Director`; headers set by WRM (`X-WRM-Medium-Key`, `Origin`, `Sec-Fetch-Site`) and removed (`Cookie`, `Authorization`) can no longer be undone by a client's `Connection` header. `X-Forwarded-*` headers are not added.
- Bundle import accepts regular tar entries without the deprecated `tar.TypeRegA` constant (same behaviour).
- **Docker image:** npm, corepack and yarn are removed after the Bitwarden CLI is installed; only Node stays to run `bw` (npm's bundled `tar` had a fixable critical vulnerability, CVE-2026-59873).

### Removed
- Unused code found by staticcheck (`mediumStart`, `mediumDockerUser`, `aiDelegateModel.mayTrain`, two unused struct fields, an unused test type).

### Fixed
- Git checks: a finished check job frees the user's check slot before it reads as ended, so a check started right after the previous one ended no longer gets *A check is already running* (409).

## [12.6.19] - 2026-10-17

### Added
- **OpenCode's own tools in a Docker sandbox** (`opencode_sandbox.go`): an option next to the contained OpenCode agent. With the policy `ai_opencode_sandbox` (`off` by default, `admins`, `all`) the assistant setup shows **🐳 OpenCode's own tools, in a Docker sandbox**. The agent then runs `opencode serve` in a container on the WRM host and may use its own shell, file and web tools there; the servers are still reached only through WRM's tools.
- **Every sandbox use is approved in WRM:** OpenCode's `bash`, file changes (`edit`, `write`, `patch`) and `webfetch` are set to *ask*. Each request becomes a normal WRM approval card showing the exact command, the diff (secrets redacted) or the URL. It **cannot be edited**, only approved once or refused (with an optional note to the agent). It is never approved automatically, also not with automatic mode or full access. Any other OpenCode permission is refused (`ai.opencode_refused`). Reading files inside the sandbox folder needs no approval.
- **Locked-down container:** image `ai_opencode_sandbox_image` (default `debian:stable-slim`, any image with a shell), run as WRM's own user ID with `--cap-drop ALL`, `no-new-privileges`, a read-only root file system, `/tmp` as tmpfs, 2 GB memory, 2 CPUs and 512 processes. The only mount is the session's own folder (`/work`) plus the OpenCode binary read-only; the API port is published on `127.0.0.1` only; keys and tokens reach the container through the environment, never the command line. Containers carry a label per WRM installation and are removed when the session ends, when WRM stops and on the next start after a crash.
- **Installer:** with the sandbox allowed, *Download and install* also fetches the Linux build of OpenCode for the Docker server's architecture (sha512-checked like the host build) when the host's own build cannot run in the container, pulls the image and runs `opencode --version` inside it. The admin page shows the sandbox state (Docker, image, ready or what is missing). Docker Desktop's Linux containers work too; WRM running inside a container cannot use the sandbox.
- **Reaching WRM and local providers from the container:** on Linux WRM's MCP listener for the sandbox binds to the Docker bridge gateway (token-protected as before); an AI provider on the WRM host's loopback address (for example Ollama) is forwarded to that container only.

### Changed
- An OpenCode agent's step cards in the sandbox say *· sandbox* and that they run on the WRM host, not on the server; `ai.opencode_started` records `sandbox`, `container` and `image`.

## [12.6.11] - 2026-10-17

### Added
- **OpenCode Zen provider** (kind `opencode`, `ai_opencode.go`): OpenCode's model gateway at `https://opencode.ai/zen/v1`. Without an API key only the **free models** are offered and used (WRM sends the public key OpenCode's own client uses); with a Zen API key (an OpenCode account, *opencode.ai/auth*) every model of the account. The model list comes from the public catalog `models.dev/api.json` (cached 6 h, a failure retried after 5 min), with names, prices, context size and the API family of each model: Anthropic Messages, or chat completions. Models only reachable over OpenAI's Responses API or Google's API are refused with a clear message. Prices come from the catalog. The provider wizard has an **OpenCode Zen** preset (no key needed) and marks free models.
- **Data-use labels** for helper models: *zero retention* (🛡), *may use prompts for training* (⚠), *unknown terms* (❔), *paid* (💳) and *your own provider* (🔑). Free Zen models that OpenCode documents as possibly training on prompts (Big Pickle, Exo, MiMo, Ling, Muse Spark, Nemotron) and free models with no published terms are treated as **may train**.
- **Delegation to helper models** (`ai_delegate.go`, tool `delegate_task`, assistant panel only): the assistant can hand a self-contained text subtask (`summarize`, `classify`, `explain`, `draft`, `code`, `other`) to a cheaper or free model. WRM picks the model **automatically by the kind of task** (free models first; then coding and reasoning models for code and drafts, reasoning models for explanations, large-context and fast models for summaries and classification) and falls back to the next one on an error (at most three). The helper gets only the text the assistant passes, with secrets redacted, limited to `ai_delegate_max_kb` (64 KiB); it has no tools. Every delegation is a card in the chat (model, data-use label, size, tokens, cost), part of the transcript and audited (`ai.delegated`, `ai.delegate_result`, `ai.delegate_denied`). Settings: `ai_delegate` (off), `ai_delegate_provider`, `ai_delegate_models` (empty = automatic from the whole catalog). **Training models are off by default:** `ai_delegate_training` = `user` lets each user switch them on for themselves in the assistant (*Helper models* → *Also use free models that may use my prompts for training*), `all` for everyone, `off` never; endpoint `GET/PUT /api/ai/delegate`, audit `ai.delegate_training`; column `users.ai_delegate_training`.
- **OpenCode agents in the assistant** (`opencode_agent.go`, `opencode_install.go`): the setup has an **Engine** choice, *WRM assistant* or *OpenCode agent*, with OpenCode's agents **build** and **plan**. WRM runs `opencode serve` on `127.0.0.1` (a free port, one process per session) in its own folder, with a configuration WRM writes: one provider (OpenCode Zen, Anthropic, OpenAI or an OpenAI-compatible provider of WRM, key passed in the environment), **all of OpenCode's own tools off** (shell, files, web, LSP, questions) and **WRM as its only MCP server** over a loopback-only listener. The agent works through a WRM server session opened for it, so every command and file edit goes through WRM's permission engine and appears in the chat with the same approval cards; nothing runs without approval unless full access is on. Text, OpenCode's own tool steps, tokens and costs stream into the chat; OpenCode's permission and question requests are refused and audited (`ai.opencode_refused`). The session's AI connection (kind `opencode`, valid only from `127.0.0.1` and only while the agent runs) is revoked when the session ends; the folder is removed. Audit `ai.opencode_started`, `ai.request`.
- **OpenCode installed by WRM:** *Admin panel → AI assistant → OpenCode agents* shows the state and a **Download and install** button. After the administrator confirms, WRM downloads the official platform package (`opencode-<os>-<arch>[-baseline][-musl]`) from the npm registry, **checks its sha512 against the registry**, unpacks only the binary into `<DB>.opencode/bin/<version>/`, runs `--version` and records who installed it (`installed.json`, audit `opencode.install_started`, `opencode.installed`); the log is shown live. Settings: `ai_opencode` (`off` / `admins` / `all`, default `all`), `ai_opencode_version` (1.19.3), `ai_opencode_registry` (an HTTPS mirror), `ai_opencode_path` (your own binary instead), `ai_opencode_max_sessions` (4). Endpoints `GET /api/admin/opencode`, `POST /api/admin/opencode/install {confirm}`. `POST /api/ai/sessions` takes `engine: "opencode"` and `agent`.
- **OpenCode in the MCP wizard:** OpenCode (the CLI on your computer) as an AI app, with an `opencode.json` snippet (bearer token from `WRM_MCP_TOKEN`) or the OAuth variant (`opencode mcp auth wrm`), and an `AGENTS.md` instruction file.

### Changed
- `ai_provider_kinds` includes `opencode`.
- An approval card of an OpenCode agent is answered in the chat as usual; WRM routes it to the agent's server session.

### Fixed
- WRM's shutdown waits until every OpenCode agent has stopped (the process group is killed); a start after a crash stops what a previous run left behind (Linux) and removes its folders.

## [12.6.10] - 2026-10-16

### Added
- **One canvas for everything:** the window area is a shared canvas. Workboard addons are **windows on it** next to terminals, file managers and desktops (`mode: 'addon'`, created through `addWindow({mode: 'addon'})`): they move by the title bar, resize from every edge, snap to halves and quarters, minimize, maximize, appear as tabs, stack in the window z-order and can be docked as a panel (⇥ in the title bar or the title bar menu: right, left or bottom). Hiding that panel (⧉ in its bar, or ⋯ → *Back onto the canvas as a window*) brings the window back where it was. ✕ removes an addon (it asks first when the addon has settings or a note that would be lost). An addon's place is kept as fractions of the canvas (or its snap), minimized, pinned and on-top state, per user on the server. Addons of v12.6.9 are placed beside the terminals the first time.
- **Panels anywhere on the canvas:** a floating panel (*On the canvas (anywhere)*, or dropped anywhere on the canvas while dragging its tab) is part of the canvas: it lives in `#win-container`, shares the window z-order, stays inside the canvas when the window or the side panels change size, follows the grid, and comes to the front when chosen in *Panels*.
- **▤ Workboard menu** (the canvas tools): *Add addon…*; *Snap to the grid* with grid size 10 / 20 / 40 px and canvas background (only with the grid, dots, lines, plain); *Tile the windows*, *Cascade the windows*, *Minimize all windows*, *Show all minimized windows*, *Minimize / Show the addons*; *Lock the layout*; **saved layouts** (name, addons, panel layout, canvas settings and, when ticked, the open terminal, file and desktop windows; up to 10; apply from the menu, manage in *Manage saved layouts…*); *Export the layout (JSON)* and *Import a layout…*; *Panels…*.
- **Window menu** (right-click a title bar or a tab): *Pin in place* (📌 in the title bar; a pinned window does not move or resize) and *Always on top*, for every window; for addons also *Settings*, *Refresh now* and *Dock on the right / left / at the bottom*.
- **Grid:** with *Snap to the grid* windows and panels land on grid lines when moved, and the moved edges when resized.
- **New addons:** *Links* (up to 30 `Name | https://…` bookmarks to web pages), *Saved sessions* (open a saved session with one click) and *Timer* (countdown with progress bar and a notification when time is up, or stopwatch; keeps running across reloads).
- **Addon settings** gain *Colour* (a coloured title bar edge) and *Text size* (small, normal, large, extra large), and a *Remove* button.
- The empty canvas offers **▤ Add an addon** next to the connection hint.
- **Reconnect banner** of a closed or failed terminal (and desktop) has a red **✕ Close window** button next to **↻ Reconnect**; the small – button hides the banner and stops the automatic reconnect (#70).

### Changed
- **Top bar:** *Broadcast* moved into a **🧰 Tools** menu with the network tools (the Tools button turns orange with 📣 while a broadcast is on, and the menu can stop it); *Shares* shows only its icon (name as tooltip); **⚙ Settings, ▦ Panels and ▤ Workboard** are one joined group right of *Keys*. Tools is hidden only when both network tools and broadcast are off for the user.
- Locking the layout also locks windows on the canvas (moving and resizing; the snap and maximize buttons still work). Trying to drag explains how to unlock.
- *Back to the Workboard* is now *Back onto the canvas as a window*; *Floating window* is *On the canvas (anywhere)*.
- Opening a saved session never closes addon windows; `restoreWindowLayout` is shared by sessions and saved canvas layouts.
- `/api/me/ui/{key}` values may be up to **256 KiB** (was 64 KiB) for saved layouts.
- Addon windows are minimum 180 × 110 px; terminals stay at 280 × 180.

### Fixed
- Windows on the canvas no longer share a stacking level with page menus and dialogs (`#win-container` is isolated), so a window raised many times can never cover a menu.

## [12.6.9] - 2026-10-16

### Added
- **Workboard** (`static/workboard.js`, **▤ Workboard** in the top bar): a page of tiles over the window area. It starts empty; tiles are added from the **addon catalog** (*＋ Add addon*), at most 24. Every tile has: drag its title bar to reorder, **⇥ / ⇤** wide or narrow, **⤢** pull out into a panel, **⚙** settings (own title, width and the addon's options), **✕** remove. The tiles are stored per user (`PUT /api/me/ui/workboard`, copy in `localStorage` `wrm_workboard_<user id>`; the newer wins), so they follow the user to every browser. Tiles load and refresh only while they are visible.
- **Addons:**
  - *Server status*: up, down, slow and unknown connections from the live status with latency and since when, problems first; all connections with a status or a chosen list; option *only problems*; a click opens the connection.
  - *Server load*: CPU, RAM (with total), load per CPU, uptime and disk usage (fullest first; 3 disks narrow, 6 wide, the rest in a tooltip) of up to 8 chosen SSH/SFTP servers, bars yellow from 75 % and red from 90 %, with the source (*Monitoring (Zabbix agent)* or *read over SSH*) and age; refreshed every minute and with ↻.
  - *Notifications*: the user's WRM events (newest 50) with unread marked; *Mark all read*, *Clear*; a click marks one read. The ▤ button shows the unread count while the tile exists.
  - *Clock*: local time and date (seconds optional) and up to 8 IANA time zones (checked when saved).
  - *Recent commits*: the last 5, 10 or 20 commits and pushes of a service of the Git catalog (optionally one branch) from `/api/git/activity`, with author, branch, short SHA, age and a link to the commit; refreshed every 5 minutes and with ↻ (fresh from the Git server).
  - *Running tunnels*: running tunnels with their local address, and *Tunnels…*.
  - *Quick connect*: buttons with a status light for up to 12 chosen connections.
  - *Notes*: a note of up to 2000 characters, saved with the tiles.
- **Pull a tile out into a panel:** a pulled-out tile becomes a panel of the panel layout (id `wb:<tile id>`): it floats first and can be docked, stacked, split or floated like *Sessions* or *Snippets*, keeps its ↻ and ⚙, and stays where it is after a reload. *Panels* lists it under *Pulled out of the Workboard*; hiding it (⋯ → *Back to the Workboard*, or *Bring it back* on its Workboard placeholder) puts it back on the Workboard. With the layout locked, tiles cannot be pulled out, reordered or resized. Phones show every tile on the Workboard.
- **In-app notifications:** table `user_events` (newest 200 per user, removed with the user). Every notification WRM sends a user (`notifyUser`: status changes, monitoring problems, expiring keys and credentials, new versions, Git jobs …) is also kept there, whatever the user's channels, subscriptions and `notifications_enabled`; repeated reminders (`credential.*`, `git.new_version`, `git.job_upcoming`) at most once a day. The page gets `events_changed`. API: `GET /api/me/events?limit=` → `{events, unread}`, `POST /api/me/events/read` `{ids}` or `{all: true}`, `DELETE /api/me/events`.
- **Server metrics endpoint** `GET /api/workboard/metrics?id=` (own SSH/SFTP connections only): a Monitoring (Zabbix) sample younger than 5 minutes when the server has Monitoring on; otherwise one read-only SSH command (`uname -s`, `/proc/loadavg`, `/proc/stat` twice a second apart, `MemTotal`/`MemAvailable` of `/proc/meminfo`, `df -P -k` without tmpfs/devtmpfs/squashfs/overlay, `/proc/uptime`, `nproc`; Linux only), one collection per server at a time, cached 25 s (`refresh=1` reads again), 25 s timeout; errors are returned in the result.
- Policy **`workboard_ssh_metrics`** (on; *Admin panel → Policies → Live status*): allow reading server load over SSH. Off: only servers with Monitoring show values (403 with a hint otherwise).

### Changed
- `static/layout.js` supports dynamic panels (`wrmLayout.register`, `remove`, `placed`); a stored layout keeps their place until the Workboard registers them. The active tab is scrolled into view when a tab bar is too narrow for all tabs.
- The top bar has the **▤ Workboard** button (into the ⋯ menu when the bar is narrow).

## [12.6.8] - 2026-10-16

### Added
- **Panel layout** (`static/layout.js`): *Connections*, *Sessions*, *Snippets*, *Clipboard* and the AI live view are tabs in groups that sit in a dock zone (left, right, or at the bottom under the windows) or in a floating window. Drag a tab:
  - onto another tab bar to stack it there (dropped between tabs, it reorders them);
  - onto the top or bottom 30 % of a panel to split that column (left or right part in the bottom zone);
  - onto the *Left*, *Right* or *Bottom* strip that appears at the edges of the workspace while dragging, to dock it there;
  - anywhere else to let it float.
  Splitters between groups and the zone edges resize; a floating window moves by the empty part of its tab bar and resizes by its corner. Each group has a **⋯** menu: dock left, right or at the bottom, floating window, hide the panel, lock the layout, *Panels…* and reset.
- **Panels** (▦ in the top bar, 🔒 while locked; in the ⋯ overflow menu when the bar is narrow): shows every panel with where it is, shows or hides it, locks or unlocks the layout and resets it.
- **Lock the layout:** panels can no longer be dragged, split, resized or moved; switching tabs, collapsing the side panels, showing and hiding still work. A drag attempt says how to unlock.
- **Layout per user on the server:** `GET` / `PUT` / `DELETE /api/me/ui/{key}` (keys `layout` and `workboard`, a JSON object of at most 64 KiB), table `user_ui` (removed with the user). The browser keeps a copy (`localStorage` `wrm_layout_<user id>`) for an instant first paint; the newer of the two wins, so the layout follows the user to every browser.

### Changed
- The AI live view is a panel of the layout: its title bar buttons and the 🤖 menu dock it right or at the bottom, let it float or hide it, and its tab can be dragged like any other. The v12.6.7 settings (`wrm_ai_dock_<user id>`, `wrm_right_tab`, `wrm_rp_widths`) become the first layout. The title bar drag zones and `#ai-dock-bottom` are replaced by the panel drag and the bottom zone `#dz-bottom`.
- The side zones remember their width per user; while the AI live view is shown in a zone it has its own width (default 400 px, up to half the window). A zone without panels disappears.
- Phones (below 760 px) keep the fixed layout with the two drawers; the AI live view stays a sheet at the bottom.

## [12.6.7] - 2026-10-16

### Added
- **Dockable AI live view** (`static/mcp.js`): the live view of MCP sessions is a **🤖 AI** tab in the right panel next to Sessions and Snippets (default), a dock under the windows (`#ai-dock-bottom`, resizable by its top edge, sessions side by side), a floating window (moved by its title bar, resized by its corner) or hidden behind a chip that pulses while an approval waits. The layout is chosen with the title bar buttons, in the 🤖 menu (*Layout*) or by dragging the title bar onto the right or bottom drop zone, and is remembered per user in this browser (`localStorage` `wrm_ai_dock_<user id>`: mode, hidden, floating position and size, dock height). The AI tab counts the open sessions and turns yellow while one waits for approval. On phones the live view is a sheet at the bottom. Approval cards still always pop up.
- **⋯ menu in the top bar** for buttons that do not fit, with their badges.
- **Medium's Docker runtime trusts the WRM host's certificates** (`git_medium_ca.go`): the host's trusted roots (Windows certificate stores of the user, the machine, group policy and the enterprise; macOS keychains; the Linux/BSD CA bundle) and an optional PEM file go to `<DB>.medium/host-ca.pem`, mounted read-only into every Medium container and added to the image's own roots at start; `PIP_CERT`, `SSL_CERT_FILE`, `REQUESTS_CA_BUNDLE`, `GIT_SSL_CAINFO` and `CURL_CA_BUNDLE` point at the combined bundle. Behind a proxy or antivirus that inspects HTTPS, pip failed with *self-signed certificate in certificate chain*. New policy `medium_host_ca` (default on) and setting `medium_ca_file`.

### Changed
- The top bar measures itself (`ResizeObserver`) and steps through compact levels (labels hidden, smaller gaps) before moving the less used buttons into the ⋯ menu, so buttons no longer overlap at 1100–1920 px or at browser zoom.
- The right panel remembers its width (`wrm_rp_widths`), separately for the AI tab (default 400 px, up to half the window).
- CI and the documented test command allow the full race test suite 45 minutes (it now takes about 20).

### Fixed
- Text on the page could not be selected or copied (the page set `user-select: none` everywhere); now every text can, and only buttons, tabs, title bars, tree rows and resize handles stay unselectable.
- The Snippets tab of the right panel looked selected (styled as a primary button) while Sessions was open.
- A pip failure caused by an untrusted certificate now says so and what to do, instead of the generic "check access to the package registry".

## [12.6.6] - 2026-10-16

### Added
- **Docker for Medium, installed by WRM** on Linux, macOS and Windows. When Docker is missing, Medium step 2 says which Docker WRM installs and how; an administrator presses **Install Docker** once (`POST /api/git/medium/docker`), accepting Docker Desktop's licence on Windows and macOS. The consent is stored and audited (`git.medium_docker_consent`); afterwards a tool ZIP import or *Prepare runtime* installs, starts and prepares everything by itself.
  - Linux: Docker Engine from the distribution's packages (apt `docker.io`, Fedora `moby-engine`, Docker's `docker-ce` repository on RHEL-like systems, zypper/pacman/apk `docker`) and its service, as root, through `systemd-run` from WRM's sandboxed service, or with passwordless `sudo` (then WRM's account joins the `docker` group).
  - macOS: `Docker.dmg` from docker.com, Docker Inc.'s code signature checked, installed with the accepted licence for the signed-in user; without administrator rights copied to `~/Applications` and started.
  - Windows: the Docker Desktop installer from docker.com, its Authenticode signature checked, installed quietly with the accepted licence (`install --user --quiet --accept-license --backend=wsl-2`).
- New steps *Installing Docker*, *Downloading Docker Desktop*, *Checking the installer's signature*, *Starting Docker* and *Waiting for the Docker engine*, each with output and what to do when it fails.
- **`wrm -setup-docker [-service-user ACCOUNT]`** (Linux, as root): installs and starts Docker Engine, adds the WRM account to the `docker` group and restarts the WRM service. Medium shows this command when WRM has no administrator rights or Docker refuses WRM's account (*permission denied*).
- Policy **`medium_docker_install`** (default on) and `medium_log_output` in *Administration → Policies → Git*.

### Changed
- Medium uses the managed Docker runtime by default on Linux and macOS too (`WRM_MEDIUM_RUNTIME=auto`); a host runtime prepared earlier stays until the Docker runtime is prepared (`docker` moves it, `native` keeps it), and systems without Docker (FreeBSD, OpenBSD) use the host runtime.
- Medium preparation and tool import may run for 60 minutes (was 10), the combined setup too (was 30); a Docker installation has its own 45-minute budget.
- The Docker CLI is also found at Docker Desktop's standard macOS locations, and Docker commands get its directory on `PATH` (Docker Desktop's credential helper).
- Clearer Docker errors: what failed, why (the output) and what to run, for each operating system.

## [12.6.5] - 2026-10-16

### Added
- **Medium step-by-step guide** for the original deploy tool (tool ZIP, runtime preparation, deploy-config, SSH profile, start/stop) with a state, a reason when a step cannot run yet (also as the disabled button's tooltip) and what to do when it fails. The original tool and the read-only WRM preview are separate, labelled panels, each with a status light.
- **Running operation and steps:** `GET /api/git/medium` names the running operation, its user and start time, and lists the caller's steps with start time, duration and error. A busy Medium answers `409` naming them instead of a bare "busy".
- **Medium activity log** (`medium_activity`, `GET /api/git/medium/activity`): imports, preparation, starts, stops, unexpected exits, settings, WRM preview runs and requests to the original tool with time, user, result, duration, steps and output. Users see their own and the shared runtime's rows, administrators all.
- **Command output** of Docker, pip, the config check and the original tool's process (`GET /api/git/medium/log` live, kept per operation, at most 200 lines), redacted and stored encrypted. New setting `medium_log_output` (`all` default, `admins`, `none`).
- WRM's console logs each Medium operation and step (`Medium (user): …`).

### Changed
- A deploy-config ZIP is checked by WRM's own reader at once (`400` with file and key). Uploaded while the runtime is being prepared, before it exists or while another operation runs, a deploy-config or SSH ZIP is kept (`202`, `pending`) and checked by the original tool and applied at the next start instead of being refused.
- `POST /api/git/medium/start` and the tool import run in the background (`202`); the UI follows them every 2 seconds and shows the tool as running only when it answers (`ready`). An error stays shown until the next operation, with the failed step.
- A deploy-config the original tool rejects is discarded (the previous one stays in use); a check that could not run (Docker not running, timeout) keeps the ZIP and says so. The WRM preview uses a kept deploy-config. Actions in the original tool are logged with its answer.

### Fixed
- **AI connections:** a request kept to approve later could not be approved once the AI app had closed its session (404 "AI session not found or ended"). Such requests now stay listed and approvable for their hour, and the approval covers the same call once in the user's next session on that connection.
- **AI read checks:** a `grep` pattern (also `-e` values), a `sed`/`awk` script and `find -name`/`-path` patterns were checked as if they were files, so a regular expression such as `'^[[:space:]]*command=.*'` was reported as a PuTTY private key and asked for approval. Only file operands are checked now.
- An administrator's runtime preparation blocked every other Medium action for several minutes with "Medium is busy" and no way to see what ran or how far it was.

## [12.6.4] - 2026-10-16

### Added
- **`ai_full_access_blocked`** (`ask` default, `deny`): with `deny`, commands matching the administrator's `ai_blocked_commands` (rule `admin_pattern`) stay refused under full access, also after the user edits a command in the approval. The refusal tells the AI that an approval cannot help and that the administrator can change the list or the setting. The built-in destructive list, deny lists, power commands and secret files still become approvals under full access.

## [12.6.3] - 2026-10-15

### Added
- **Full access modes** (`ai_full_access_mode`, default `all`): the administrator decides how `ai_full_access` applies. `all` gives every AI session (the WRM AI panel and every AI connection) full access with the switch alone; `connection` keeps the v12.6.2 behaviour (only AI connections with the `full_access` scope, which is offered only in this mode); `user` lets each user turn it on for their own AI sessions in *Settings → AI connections* (`POST /api/mcp/full-access`, column `users.ai_full_access`, audited as `mcp.full_access`).
- *Settings → AI connections* shows whether full access is on and how it applies.

### Changed
- `GET /api/mcp/config` returns `full_access` (`on`, `mode`, `mine`). The refusal hint for blocked commands names where the administrator turns on full access.
- An installation that had `ai_full_access` on now applies it to every AI session; set `ai_full_access_mode` = `connection` for the previous behaviour.

## [12.6.2] - 2026-10-15

### Added
- **Full access with approval** for AI connections: the new scope `full_access` (offered only while the administrator turns on `ai_full_access`, default off) means nothing is refused outright any more. Commands on the destructive list or `ai_blocked_commands`, the connection's deny lists, power commands, protected file paths and secret files (also with `ai_sensitive_reads` = deny) ask the user instead, every time and never automatically, even with "Allow everything". The approval is marked *FULL ACCESS* and names the rule WRM would have applied. A read-only connection stays read-only, other scopes still apply, and the decision is re-checked at every call, so turning the policy off restores the refusals at once.

### Changed
- A refusal of a blocked command tells the AI that full access exists and who can turn it on.

## [12.6.1] - 2026-10-15

### Added
- **Approve later:** over MCP, an approval nobody answered in time, that the AI app declined or stopped waiting for, stays in WRM for an hour (live panel, 🤖 menu and approval card with the full command). Once approved there, the same call runs once when the AI repeats it; the AI is told to say so and repeat the call instead of giving up. Audited as `ai.approved` / `ai.denied` with `later`.
- **Reading files that may hold secrets with an approval:** keys, credentials and `ai_read_deny_paths` files are no longer refused outright; each read asks the user and secret-looking values are redacted for the AI. The new policy `ai_sensitive_reads` (`ask` default, `deny`) restores the old behaviour.
- **Allow everything** for AI connections in automatic mode: one button fills the allow lists with `*` and `/**`, and **No time or action limit** keeps automatic mode on for as long as the AI connection is valid (`until_grant`). Administrators can turn the option off with the new policy `ai_auto_until_grant` (default on).

### Changed
- `*` in the automatic allow list now allows every command, including chains with `&&`, variables, groups, command substitution and redirections. Blocked commands, the deny lists, sensitive files and commands WRM cannot parse still ask or are refused; the grant still covers only the chosen servers and its lifetime.
- When WRM asks for an approval that is then denied or not answered, the AI is told which rule asked (for example the session's time limit or a command outside the allow list), what the user can change in WRM so it is not asked again, and to tell the user why WRM asked before trying again.
- MCP sessions with no limit report `until_grant` and an `auto_note` in `open_session`.
- Hard refusals (destructive or blocked commands, deny lists, read-only sessions, scopes) tell the AI that an approval cannot help and what the user can do instead. The approval question in the app says that an unanswered request stays open in WRM.

### Fixed
- A long MCP session in automatic mode silently fell back to asking after the session time limit (at most `ai_auto_max_minutes`), and a command with a variable such as `$venv` asked even with `*` allowed. Both ended as "The user denied this action" when the app declined the question, although the user had allowed everything.
- An MCP tool call whose approval the app stopped waiting for was reported to the AI as "the session was stopped".

## [12.6.0] - 2026-10-15

### Added
- **Medium native engine preview (read-only):** a *Native engine* panel in the Medium view reads the user's imported `deploy-config` as it is (`servers.yaml`, `environments.yaml`, `projects/*.yaml`) with a Go reader and validator that follows the original tool's rules (unknown keys, duplicate keys, YAML 1.1 booleans, `<<` merges, names, ports, `allowed_refs`, ignore rules, absolute paths, `source_dir`, state locations and overlapping destinations).
- **Native plan:** for a project, environment and branch or tag, WRM prepares the source from the matching WRM Git source (GitLab, GitHub or Gitea API, `source_dir`, symlinks, SHA-256) and compares it with each destination over SFTP through a saved WRM connection. The diff follows `rsync -rlc --delete` with the project's exclusions, including directory-only, anchored, `**` and `***` patterns, kept directories that hold excluded items and the "cannot make way" error. Server state (`current.json`, the history tail, the lock owner) is read in the original format.
- **Comparison with the original tool:** with Medium running, *Compare with the original tool* runs the original comparison (fetch, export, build, `rsync --dry-run`) through the authenticated bridge and shows, per server, whether the native plan is the same, with changes only in one engine or of a different type. The original tool's prepared source is used for projects with a build, and the WRM Git source is checked against the original export.
- **Servers ↔ WRM connections:** each `servers.yaml` server is matched to a saved SSH connection by address and user, or linked explicitly (`medium_server_links`, audited as `git.medium_server_linked`). Previews are audited as `git.medium_native_preview`.
- API: `GET /api/git/medium/native/config`, `POST /api/git/medium/native/link`, `GET /api/git/medium/native/refs`, `POST /api/git/medium/native/plan`, `GET /api/git/medium/native/plan/<id>`.

### Changed
- `medium_wrapper.py` exposes `POST /__wrm_plan` (behind the bridge key) that returns the original comparison and a manifest of its prepared source; the original tool is still unchanged.
- The ROADMAP keeps v12.7.0 (native engine and build location) and v12.8.0 (switch-over).

## [12.5.1] - 2026-10-15

### Added
- **Claude Desktop extension (`wrm-pro.mcpb`):** *Settings → AI connections → Claude Desktop* downloads an MCP bundle with the WRM address and the new token filled in; Claude Desktop installs it with one click. Its dependency-free Node.js bridge (`remote-manager/mcpb/server/index.js`) runs on the user's computer, so a WRM on a private network or over plain HTTP works; optional CA file for self-signed HTTPS. Endpoint `POST /api/mcp/claude-desktop`, audited as `mcp.desktop_extension`.
- **Claude Code plugin `wrm-pro`:** the repository is a Claude Code plugin marketplace (`.claude-plugin/marketplace.json`, plugin in `packaging/claude-plugin/wrm-pro`); one `claude plugin install … --config` command connects Claude Code to WRM over HTTP with the token in secure storage. The wizard shows the command.
- **Live view of AI sessions:** when an AI app starts working on a server over MCP, WRM opens a live panel with every command or file action, WRM's decision, the approval with **Approve** / **Deny**, the exit code and the output. Reopen it with *Watch live* in the 🤖 menu; the automatic opening can be turned off there (per browser).
- The token lifetime of AI connections can be entered in hours or days (setup wizard, advanced form, OAuth approval dialog).

### Changed
- After a denied or unanswered approval the AI is told where it was refused (in WRM or in the app) and to ask the user whether to continue, change the command or stop, instead of only "The user denied this action".
- Default AI connection lifetime is 7 days (`ai_mcp_token_hours` 168, was 8 hours), at most 30 days (`ai_mcp_max_token_hours` 720, was 24 hours). Explicitly saved values are kept.
- The wizard's cloud connector option is named *Claude.ai (cloud connector, public HTTPS)* so it is no longer mistaken for the Claude Desktop setup.

### Fixed
- Claude Desktop could not be connected to a WRM that is not reachable from the internet over HTTPS: the only offered route was the cloud connector, and `claude_desktop_config.json` ignores HTTP server entries.

## [12.5.0] - 2026-10-15

### Added
- **Stamp as it is:** the Stamp dialog lists differing files per installation and explains why nothing can be stamped (instead of "No files are selected"). An option stamps installations that are not up to date: VERSION.md records each file's actual version and a `Napomena` note.
- **Back navigation in dialogs:** Git dialogs opened from another dialog return to it with **← Back** (Alt+←), keeping ticks and typed values. Settings → Inventory/Admin panel and Tunnels → Edit connection return to their opener.
- **Medium forced stop** after a second confirmation when the original tool's state cannot be read.

### Fixed
- Production confirmation in Git workspace runs compared the typed text with a hostname instead of the connection name.
- Medium: starts no longer hold the global process lock while waiting for Docker/readiness; Stop no longer waits for another user's setup; empty settings no longer override WRM's SSH options and original defaults; successful starts/imports clear stale setup errors and pending imports; Advanced config imports replace a config repository; replaced SSH profiles are deleted; case-only duplicate ZIP entries are rejected; status polling survives errors; stage texts are translated.
- Logs: invalid UTF-8 in scanned segments (quadratic trimming and a stuck Continue); segments split mid-line; regex errors and wrong highlights for keywords with special characters; empty files reported as incomplete; Previous segment skipping 192 KiB; discovered roots rejected after settings changes; a closed tab during Check now recording a failed sample and notification; late checks repainting another tab; untranslated Zabbix link.
- Client GitLab cache: cleanup could delete a newer published generation; relay POST errors were shown as CORS problems; a failed status poll blocked later refreshes; untranslated pagination error.
- Following a log kept no shell after Ctrl+C; folder moves/deletes did not recheck folder shares.

## [12.4.5] - 2026-10-15

### Added
- **One-click Medium setup:** combined trusted tool/config/optional SSH ZIP upload, retained imports, background dependency preparation, config validation/profile synchronization and automatic launch of the unchanged original deploy tool. Installed Docker is started automatically; first interactive Windows setup can download and verify the official signed Docker installer. Vendor/OS prompts and restart requirements remain explicit.
- **Cross-log search:** combined server/operator/service/date/timezone/keyword filtering, historical scanning across matching files, incremental per-file results, highlighted excerpts, counts, event ranges, open/follow actions, cancellation and continuation of bounded scans.
- **Environment filters:** combined text/service/environment/server filters, preserved values and a sticky control area.

### Fixed
- Logs start discovery automatically, update as each server finishes and show persistent progress/errors/empty states. Known installation log folders, bounded service/operator folders and Supervisor log paths supplement configured roots; non-log files and symlinks are excluded. Metadata discovery avoids repeated remote per-file realpath round trips.
- Log cards show file modification times; excerpts show matching event times and last-read time in the selected timezone, including timestamp inheritance for continuation matches. Time-only searches count matching records correctly.
- Late discovery/Functionality responses cannot paint into a different Monitoring tab. Host-metric versus service/log prerequisites are explained in the UI.
- Overview and Installations pin the complete filter/check/action stack while scrolling. Environment cell requests are deduplicated during filter changes.

[Operation guide](docs/V12.4.5_GUIDE.md) · [Validation](docs/RELEASE_SUMMARY_v12.4.5.md).

## [12.4.4] - 2026-10-15

### Added
- **Monitoring workspace:** server overview with Zabbix agent/agent 2 metrics, freshness, CPU/RAM/load/uptime, filesystem usage, network rates, history charts and problem/recovery notifications. Supports passive checks through SSH or a network route, verified TLS certificates, existing remote `zabbix_get` for PSK, and existing HTTPS Zabbix server API items for active-only installations. Includes OS/installed-agent detection and setup guidance.
- **Logs:** multiple per-server roots with service/operator labels, bounded discovery and segmented historical search, server/service/operator/time/timezone/keyword filters, highlighted matches and navigation, copy/download excerpts and terminal actions. Up to 16 movable/resizable live panels support pause, recent-line counts, autoscroll, vertical/horizontal/cascade layout, rotation/truncation detection, reconnect diagnostics and request cancellation.
- **Client GitLab access:** a browser token or WRM companion on the computer with GitLab access supplies repository API data. Tokens stay on that computer. Complete bounded snapshots are encrypted and atomically published on WRM; the latest successful generation survives disconnect/restart and failed/out-of-order refreshes. Explicit cache members, contributor/freshness/progress and live uncached reads are supported. Reviewed deployment manifests preserve the reviewed target across refreshes.
- **MCP workflows:** downloadable CLI setup scripts, Codex OAuth instructions, Cursor install link and cloud HTTPS guidance; `wrm` MCP prompt, `select_server` picker where supported, native assistant `/wrm` / `/wrm server`, and structured SFTP `inventory_files` for permitted `.ini`/`.py` searches. Existing MCP grants can change servers/modes/limits without replacing their bearer token or expiry; old sessions and approvals are canceled immediately.
- **Terminal context:** Copy/Paste and native Shift + right-click fallback; preview/redact selected text or file context before queuing an AI question. Open terminal here starts in the safely quoted directory on the same authorized connection, with POSIX/PowerShell startup handling.
- **Visual connection labels:** validated colors and 18 curated IT/streaming icons for folders and connections, defaults/reset/preview, persistence, duplication and configuration export/import.

### Changed
- **Functionality moves into Monitoring**, preserving Git catalog/config associations. Hostname groups show connection context, counts and collapse controls; search and combined server/service/environment/runtime/health/freshness/problem filters preserve state. Search/filter bars stay visible while scrolling Git overview, installations, activity, Simple, Functionality, Monitoring and existing search dialogs/admin lists.
- **MCP Automatic within limits** is visible alongside read-only and approval modes, with explicit opt-in, command/path allowlists, time/action budgets and existing policy/approval checks. Enabling this mode never enables MCP globally.
- **AI provider timeouts:** configurable response-header wait (900 s default), total request (3600 s) and stream inactivity (600 s), with cancellation and distinct diagnostics. A slow local provider is not retried automatically, avoiding duplicate tool execution.

### Fixed
- **Issue [#5](https://github.com/vedranius/web-browser-RDM-public/issues/5):** the general configuration import recognizes mRemoteNG XML and PuTTY registry exports rather than reporting invalid JSON. Real-browser synthetic 4 MiB XML and UTF-16 PuTTY imports pass; existing encrypted/nested/proxy import regressions remain supported. The reporter's original file was not supplied.
- **Replay fit:** very wide terminal recordings scale to the available width without horizontal scrolling; graphical replay accounts for padding and existing resize/fullscreen/font handling remains active.
- **MCP wizard checkbox/radio alignment:** controls retain their natural width beside wrapped labels at phone and desktop sizes.

Full operation limits, setup and validation: [v12.4.4 guide](docs/V12.4.4_GUIDE.md) and [release validation](docs/RELEASE_SUMMARY_v12.4.4.md). The original private Medium tool and deploy-config integration remain as delivered in v12.4.3/v12.4.3b.

## [12.4.3b] - 2026-10-14

### Fixed
- **Windows Medium dependencies:** automatic managed Linux Docker runtime; host Windows no longer needs Python, Git, SSH or rsync. Native Linux remains available, with optional automatic Docker fallback. CLI detection includes standard Docker Desktop paths and an explicit override; daemon/Linux-engine diagnostics explain what to start.
- **Import recovery:** trusted tool archives are retained when setup fails. Administrator retry prepares the existing archive; failed/pending setup is never reported ready.
- **Linux filesystem behavior on Windows:** per-user persistent Docker volumes preserve native permissions, symlinks, Git/deploy state and history. Source/venv are read-only; profiles synchronize only after explicit import. Standard `~/.ssh` references resolve to the private profile. Native builds use an unprivileged account; its access to SSH keys/config is rejected.
- **Runtime cleanup/context:** only owned containers are stopped; runtime environment aliases preserve the host Docker CLI's HOME/context and keep secret values out of argv. Docker's inherited WRM healthcheck is replaced by the authenticated wrapper readiness/busy probes.
- **Letter hotfix versions:** v12.4.3b is parsed/ordered after v12.4.3 and before v12.4.4, with executable/version identity checks. Earlier updater binaries require a manual download for this first letter-suffixed release.

### Changed
- Existing deferred requests and new monitoring/MCP/log/import/terminal requirements are consolidated into the accepted **v12.4.4** roadmap. They are not delivered in this urgent Medium hotfix.

## [12.4.3] - 2026-10-14

### Added
- **Medium original deployment tool** beside Simple and Advanced. Administrators import the supplied private tool ZIP; operators import their native deploy-config/SSH profile. The original Python app, templates and deployment engine run unchanged behind WRM authentication in private per-user directories. All original UI/config options remain native, including comparisons, exclusions, builds, plans, confirmation, deployment progress, backup/history/rollback and config Git controls. ZIP SHA-256, setup/dependency status, encrypted runtime preferences and idle-only stop are exposed. See the [parity record](docs/MEDIUM_PARITY_v12.4.3.md).
- **Functionality configuration profiles**: upload a reference INI/config/Python file and inspect matching catalog installations by hostname, connection, environment, operator, section and setting. Searchable reference/actual matrix with missing/different/additional/masked states, parse warnings and bounded parallel remote reads. Stored structured references are masked and encrypted; source expressions are not executed.
- **Bitwarden native SSH keys**: official CLI account login/MFA or personal API login, unlock/sync/lock/logout, cloud/EU/self-hosted HTTPS servers and per-user profiles. SSH/SFTP connections store a native item reference and fetch the key only for authentication; private keys never reach the browser. Unlock sessions stay in memory and expire after interactive inactivity (15 minutes by default), WRM logout or restart. CLI profiles are encrypted between commands with startup recovery.
- **Manual folder ordering**: drag before/after siblings, nest/reparent or move up/down from the menu. Additive `sort_order`, owner/cycle/depth validation, persistent sibling order and export/import support.

### Changed
- Docker includes Python 3.12+, git, OpenSSH, rsync, Node and pinned official Bitwarden CLI 2026.9.1. Standalone packages retain optional runtime dependencies. Medium access defaults to administrators and is intended for trusted operators: native build/config commands run as the WRM OS account; per-user directories are not an OS sandbox.
- Additive `git_config_profiles` table; new Medium/config-profile/Bitwarden/folder-reorder APIs and English/Croatian controls. Earlier client GitLab relay/replay, AI timeout/navigation and color/icon requests remain documented backlog; the owner prioritized v12.4.3.

### Fixed
- Git dialogs remain above mobile drawers and fit narrow widths; wide profile matrices scroll inside the dialog.
- Native same-origin forms with `Origin: null` under the original tool's no-referrer policy work through the authenticated bridge; cross-site/null-origin submissions remain rejected.
- Connection tests display provider-specific errors for locked/unavailable Bitwarden authentication.

## [12.4.0] - 2026-10-14

### Added
- **Service Functionality** in the Git workspace, also reachable from Simple mode: per-server Supervisor worker inventory, failed systemd units, disk/memory checks, per-installation runtime checks and bounded error logs. Manual and periodic checks use the existing Git catalog and GitLab/GitHub/Gitea/bundle installations. Services may specify additional runtime units, HTTP health URLs and TCP endpoints. Missing/inaccessible checks remain unverified.
- **Configuration visibility** per installation: section/key/value/function tables, comments as descriptions, INI/config/environment files and JSON settings, automatic discovery or explicit relative catalog paths. Secrets, private-key blocks and secret continuation lines are masked before the API response; content is read on demand, not stored with comparison results. File/entry limits, parse gaps, missing/unreadable files and symlink escapes are explicit.
- **Functionality notifications** for service/server problems and recovery, including workers not associated with a catalog service. Quoted secrets with spaces are redacted completely. Identical states do not repeatedly notify.
- **Nested folders and connection context** ([#57](https://github.com/vedranius/web-browser-RDM-public/issues/57)): parent selection, recursive sidebar, full folder paths in pickers/titles/tabs, and a folder row that filters open connection tabs. Hierarchy survives config export/import and new mRemoteNG imports. Sharing a folder includes its descendants; route defaults stay scoped to the immediate folder. Owner, cycle and 32-level depth checks protect moves/imports; deleting a folder reparents its children without deleting their connections.
- **Remote previews and streaming** ([#55](https://github.com/vedranius/web-browser-RDM-public/issues/55)): embedded video/audio playback with SFTP/FTP byte-range seeking, image/PDF previews and text previews capped at 256 KiB. Preview/download actions work on a path selected in a terminal; relative paths use its CWD. The file manager has a Preview action. Every request enforces connection/share permissions; active document formats render as text, and closing the viewer stops media playback.

### Fixed
- **AI assistant availability** ([discussion #56](https://github.com/vedranius/web-browser-RDM-public/discussions/56)): SSH-backed SFTP connections can use the assistant in both frontend and backend. A permanent AI entry point and Settings → AI assistant explain effective account/policy blockers. Provider setup shows availability separately from connectivity. Open connection titles refresh after folder changes, and shared tabs resolve the correct share. Policy changes broadcast to other open browsers, and reconnecting the event bus refreshes effective policy; shared connection resolution uses the matching share. The terminal assistant works independently of MCP settings.
- **Missing installations** remain registered with a failed/gone warning instead of disappearing from discovery. Supervisor discovery associates only the program stanza that references the installation directory.
- **Release version consistency**: Docker Compose now matches the application and release version.

### Changed
- Additive schema: `folders.parent_id` and `git_server_health`; existing catalogs, databases and route defaults remain compatible. New API: `GET|HEAD /api/remote/preview`, `GET /api/git/installs/{id}/configuration`; Git state includes server health and installation functionality.
- Regression coverage for nested-folder ownership/imports, shared subtree access, Supervisor group failures/log redaction, configuration boundaries, SFTP/FTP ranges, preview permissions and cross-window AI policy refresh. UI checks include workspace-context regressions.

## [12.3.6] - 2026-10-13

### Added
- **Linux AppImage**: portable x86_64 and ARM64 packages with persistent user data, localhost defaults, desktop icon and FUSE-free extraction fallback. Updates preserve data; boot services use standalone binaries.
- **Complete release publication**: draft releases are published only after all 16 packages, macOS universal binary, SHA-256 checksums and Docker publication succeed.

### Fixed
- **Ollama action completion**: command execution and its audit now finish and release the action lock before the completed status is published, preventing a transient HTTP 409 when the next setup step starts. CI exposed the timing window; the regression checks lock availability at completion.
- **AI server date and tool output reliability**: `system_info` now reports the remote server's actual date and time. The assistant is told not to fabricate tool calls, logs, dates or internal tool-result wrappers.
- **AI button after policy changes**: enabling or disabling the assistant refreshes the current user's policy and AI buttons on already-open terminals without reconnecting. Legacy lowercase SSH protocol values are also recognized; shared connection roles remain enforced.
- **AI processing and tool JSON**: the processing indicator follows the complete model turn; finalized messages remove recognized tool JSON while retaining ordinary text. Corrected the missing English/Croatian label and expanded translation checks to include double-quoted and formatted keys.
- **Local model text fallback**: accepts only complete JSON calls to offered tools with object arguments. Truncated responses are preserved rather than repaired into commands or silently discarded; ordinary JSON answers remain visible. Regression coverage includes multiple streamed calls and refusal to run truncated text even in automatic mode.
- **Git Simple review before deploy**: added combined filters for service, hostname, environment, status, branch and version/commit; cards show when their server state was compared, and the deploy plan lists each changed file with its state on each destination. Users can choose successful destinations and exclude files before confirmation; deploys still use the existing fingerprinted plan, backup, lock and audit flow.
- **Deployment connection binding**: a reviewed plan is also bound to the connection's identity, host, login and effective jump/proxy selection. Changing those settings requires a new comparison; a late change is checked again before taking the destination lock.
- **Linux service database diagnostics**: installation checks whether the service account can write the configured SQLite database and data directory; failed starts include systemd status and recent journal output, with a targeted hint for read-only database errors.
- **Systemd restart storms**: generated units cap repeated restarts to avoid a tight loop after a persistent startup failure.
- **Systemd paths and reinstall**: fixed single-path directives for directories containing spaces, preserved literal dollars in path lists and escaped percent specifiers. Reinstalling clears previous startup failures and restarts the new binary instead of leaving the old process running.
- **Service configuration visibility**: successful installation output names the database path used by the service.
- **Pre-push review fixes**: refusing an explicit empty server selection in both deploy and rollback; recomputing confirmation after server changes; ignoring canceled or superseded comparisons; keeping per-server rollback versions visible; preserving service settings on database preflight failure. Regression checks cover the new paths and run in CI; Ollama setup tests no longer depend on a locally installed binary.

## [12.3.4] - 2026-10-12

### Fixed
- **Multi-Tool Interceptor for Local Models**: Pametno izvlačenje višestrukih JSON poziva alata iz običnog tekstualnog odgovora modela (npr. kada model pošalje uvodni tekst i više alata odjednom).

## [12.3.3] - 2026-10-12

### Fixed
- **Integration Test Race Condition**: Riješeno asinkrono kašnjenje upisa `terminal.close` pod `-race` detektorom u `TestTerminalSessionAuditAndRecording`.

## [12.3.2] - 2026-10-12

### Fixed
- **Ollama Textual Tool Call Interceptor**: Dodan fallback u `ai_llm.go` koji automatski presreće tekstualne JSON pozive alata kada lokalni modeli (Ollama/Qwen) vrate naredbu kroz tekst umjesto API strukture, te ih izvršava na poslužitelju.

## [12.3.1] - 2026-10-12

### Added
- **Prefilled Safe Auto-Mode**: Automatski ponuđene sigurne naredbe (*), blokirane destruktivne i automatski označena kvačica potvrde.
- **UI Thinking Feedback**: Vizualni indikator (loader) i privremeno zaključavanje sučelja dok agent analizira i izvršava upite na serveru.

### Fixed
- **Ollama Context Sanitizer**: Sprječavanje loopa sirovog JSON-a kod ponovnog učitavanja poruka iz baze.

## [12.3.0] - 2026-10-12

### Added
- **Smart Safe Auto-Mode**: Automatsko odobravanje sigurnih dijagnostičkih naredbi (`df`, `free`, `top`, `ps`, `docker ps`, `ls`) u auto modu.
- **AI UI Thinking Status**: Prikaz stanja obrade i izvođenja naredbi na serveru u stvarnom vremenu.
- **Ollama Context Sanitizer**: Sprječavanje slanja tekstualnog JSON-a u model nakon učitavanja povijesti iz baze.

### Fixed
- Ispravljen loop odbijanja alata kod Qwen modela na lokalnoj Ollami.
- Session 0 poboljšanja stabilnosti servisa na Windows poslužiteljima.


# Changelog

All notable changes to Web Remote Manager PRO. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Release notes with downloads are on the
[Releases page](https://github.com/vedranius/web-browser-RDM-public/releases).

## [12.2.9] — 2026-10-12 — Git Simple mode is automatic, servers shown by hostname

Part of #43.

### Added
- **Automatic environments in Git Simple mode.** Services and environments the checker already found on the servers (`git_installs`) are *derived* into virtual environments and shown as ready cards, so the administrator no longer defines environments per service. `GET /api/git/env/overview?auto=1` (Simple mode only) merges them; plan, cell, history, doctor, unlock and run creation resolve them too, so locks, backups, permission checks and audit are unchanged and nothing is deployed without an explicit run. Stored environments win (same name, or the same server and folder); invalid or conflicting derivations are dropped.
- **Save detected environments** (`POST /api/git/env/adopt`): stores the derived environments in the catalog. Requires the `git_update` policy, audited as `git.env_adopted` (neutral keys `services`, `environments`).
- **Find services on my servers** in an empty Simple mode (the normal check).
- **Cached hostnames:** the table `git_hostnames` (additive) caches each connection's `hostname`, read with a fixed command over the existing SSH login during checks and deploy plans (6 h).

### Changed
- **Servers are shown by hostname** instead of the connection name in Git Simple mode and wherever the Git workspace lists servers (Environments, Activity, Installations, Runs, filters), falling back to the connection's host address. Filters and requests still use the connection name.
- English and Croatian texts for the new notice, buttons and messages.
## [12.2.8] — 2026-10-12 — Updates dialog always reachable

Part of #43. Frontend only; no database or backend change.

### Fixed
- **The Updates dialog can be opened without a known update.** It used to open only from the green update badge, which is hidden until an update is known, so an administrator could not force a check. The version label in the top bar ("WRM vX.Y.Z") is now a button (tooltip in English and Croatian, keyboard: Tab, Enter, Space) that opens the dialog; **Check now** is there for administrators, other users see the versions only. On narrow windows, where the text is hidden, the logo next to it stays as the button. The badge and the sign-in page are unchanged.
## [12.2.7] — 2026-10-12 — More AI providers and guided Ollama

Last step of issue #43. One additive table (`ai_ollama_runs`).

### Added
- **Provider presets** in the AI provider wizard for xAI, DeepSeek, Together AI, Fireworks AI, Cerebras, Cohere and Perplexity (all OpenAI-compatible), with refreshed multi-model suggestion lists for the existing presets. The model name stays free text and the lists stay editable.
- **Model list loading:** `GET /api/admin/ai/providers/{id}/models` asks the provider for its `/models` list (OpenAI-compatible and Anthropic kinds) using the stored key; the wizard shows the result as one-click chips. Audited as `ai.provider_models_listed`.
- **Guided Ollama setup** (wizard step "Ollama on this server"): detects Ollama (`/api/version`, `/api/tags`, default `http://127.0.0.1:11434`), shows status and installed models, per-OS install / background-service / pull commands with copy buttons, and adds Ollama as an OpenAI-compatible provider (`/v1`, no key).
- **Optional "do it with WRM"**: `POST /api/ai/ollama/action` (`install`, `start`, `pull`) runs fixed commands as a background job. Server-side checks: administrator, policy `ai_ollama_manage` (default off, `WRM_AI_OLLAMA_MANAGE`), explicit `confirm`, supported OS, validated URL and model name, one run at a time. Runs are stored in `ai_ollama_runs` (`GET /api/ai/ollama/runs`, admin only) and audited as `admin.ai_ollama_install|start|pull|finished|denied` with neutral detail keys.
- Tests: `ai_ollama_test.go` (fake Ollama server, permissions, audit, model list).

### Changed
- The wizard test step and the last step offer "Load the model list". All new texts are in English and Croatian.

## [12.2.6] — 2026-10-12 — Guided AI and MCP setup

Seventh step of issue #43. No database changes; no new endpoints.

### Added
- **MCP connection wizard** (Settings → AI connections): pick the app (Codex CLI, Claude Code, Gemini CLI, Cursor / VS Code / other, Claude and ChatGPT through OAuth) → pick servers and access level (look only / look and change with approval, limited by the admin policy) → ready-made commands and config files (`codex mcp add`, `config.toml`, `claude mcp add`, `gemini mcp add`, `settings.json`, `mcp.json`) with copy and download buttons → in-browser **connection test** that runs `initialize` and `tools/list` with the new token. The token is created through the existing audited `POST /api/mcp/grants` and shown once. The old token form is kept under "Advanced".
- **AI provider wizard**: provider presets (Anthropic, OpenAI, Gemini, Groq, OpenRouter, Mistral, Ollama, custom OpenAI-compatible) → key and model → test → done, using the existing provider endpoints. The old form is kept as the advanced option.
- The MCP server now **introduces itself**: `serverInfo.title` / `description` and rewritten `instructions` explain that "wrm" is WRM PRO and list what it can do and which scopes exist. `GET /api/mcp/config` also returns `instructions`, `description` and `version`, and the wizard offers a ready `AGENTS.md` / `CLAUDE.md` / `GEMINI.md` text, so clients such as Codex no longer ask what *wrm* refers to.
- The `open_session` result lists `not_allowed` and `how_to_allow` when the connection's scopes or mode limit it.
- Tests: `mcp_wizard_test.go`.

### Changed
- The scope-denial message ("Denied by WRM …") now states which scopes the connection has, which one the tool needs, that this is a limit of the connection and not of the server, and tells the user to create an AI connection with that scope (Settings → AI connections). This replaces the vague "insufficient permissions" reaction of AI clients.
- All new texts are in English and Croatian.

## [12.2.5] — 2026-10-12 — Movable, resizable AI assistant window

Sixth step of issue #43. No server or database changes.

### Added
- The **AI assistant panel** inside a terminal window can now be **docked left, right, top or bottom**, shown as a **floating window**, **resized** (drag its inner edge, or the corner when floating) and **expanded** over the whole terminal window with a new ⛶ button (a double-click on the title bar does the same; press it again to restore). Drag the title bar to move it; moving a docked panel makes it float. The terminal and file area shrink beside a docked panel and are refitted, so the full command and output stay visible.
- Dock, size and floating position are **remembered per user** in the browser (`localStorage`, key `wrm_ai_win_<user id>`). The expanded state is not remembered.
- All new texts are in English and Croatian.

### Changed
- On compact windows (up to 760 px wide, e.g. phones) the layout is unchanged: the panel covers the terminal window below the title bar, and the new controls are hidden.

## [12.2.4] — 2026-10-12 — Resizable, fullscreen session replay

Fifth step of issue #43. No server or database changes.

### Added
- **Session recording replay** (terminal recordings) can be **resized** by dragging its bottom-right corner and expanded to **fullscreen** with a new ⛶ button in the header (**Esc** or the button leaves fullscreen). The size resets each time a recording is opened. Remote desktop (RDP/VNC) replays use the same window and also refit to it.
- All new texts are in English and Croatian.

### Changed
- The replay terminal keeps the recorded size (columns × rows) but is now **scaled to fit the available width** (and, in fullscreen, the height), re-fitted whenever the window is resized, so there is no sideways scrolling, also on phones (checked at 390 px). Playback controls and behaviour are unchanged.

## [12.2.3] — 2026-10-12 — Explanations for tabs and settings

Fourth step of issue #43. No server or database changes.

### Added
- **A short description under every tab** of the Advanced Git workspace (Overview, Services, Installations, Environments, Runs, Activity, .gitignore, Settings), also shown as a tooltip on the tab button.
- The same for the **Settings** window (General, Terminal, Security, Notifications, Voice & audio, Session history, AI connections, Data) and every tab of the **Admin panel**.
- All texts are in English and Croatian and wrap on small screens (checked at 390 px).

## [12.2.2] — 2026-10-12 — Git Simple Mode

Third step of issue #43. No server or database changes.

### Added
- **Simple mode for the Git workspace.** A **Simple | Advanced** switch in the header (remembered per user in the browser; Advanced stays the default and is unchanged). Simple shows one guided card per service environment: the version on the server next to the latest version on the branch, a clear status (up to date / update available / not installed / cannot compare / problem), and big actions **Install**, **Update** / **Deploy again** and **Roll back to the previous version**, each with a plain-language confirmation (version change, files per server, optional post-deploy and restart, optional other version, typing the environment name for protected environments).
- It reuses the existing environment endpoints (overview, cell, plan, history, runs), so permissions, locks, backups, audit and run history are exactly those of the Advanced deploy; no new privileged paths.
## [12.2.1] — 2026-10-12 — Terminal fixes

Second step of issue #43 (terminal items). No server or database changes.

### Fixed
- **Copying a long selection from the terminal:** the selection was copied on every change while the mouse was still dragging, which moved the focus out of the terminal and cut a selection that scrolls (more than one screen) short, so people had to zoom out. The copy now happens once the selection has settled (mouse released, 200 ms without change), and the whole selection, scrollback included, is copied.
- **Windows that suddenly shrink or grow:** pressing the mouse on the title bar of a maximized or snapped window restored its size at once, so a click (or the first half of a double-click) shrank it and the second half expanded it. The window is now restored only when the pointer actually moves (5 px), as when dragging.

### Changed
- **New terminal windows open maximized** and the cursor is placed in the terminal at once. Windows restored from a saved session keep their saved layout and focus; small screens already used full-size windows.

## [12.2.0] — 2026-10-12 — AI provider fixes

First step of issue #43. The other items of the issue stay open and are planned for the next versions.

### Fixed
- **Gemini (and other OpenAI-compatible providers) with tools:** the provider's `extra_content` of a tool call (Gemini's `thought_signature`) is now kept in the session history and sent back with the call, so the next request no longer fails with *HTTP 400: Function call is missing a thought_signature*.
- **Temporary provider errors** (HTTP 408 / 429 / 500 / 502 / 503 / 504 / 529, for example *This model is currently experiencing high demand*) are retried up to three times (3, 8 and 15 seconds) instead of ending the turn.
- **Requests that are too large** (HTTP 413, or a 400 / 429 that names a token or context limit, as Groq does) shorten the long tool output of the conversation (4000, 1200, then 300 bytes per result, keeping start and end) and retry, so the assistant's answer is shown instead of only the error.

### Added
- Assistant notices *The provider is busy; trying again shortly…* and *The request was too large for the provider; long command output was shortened…* (English and Croatian).
- Tests: `ai_recovery_test.go` (the `thought_signature` round trip against a fake streaming provider, error classification, history shortening).

## [12.1.0] — 2026-10-10 — AI desktop apps (MCP)

### Added
- **MCP server** at `/mcp` (`mcp_server.go`): Model Context Protocol over streamable HTTP (protocol versions `2025-11-25`, `2025-06-18`, `2025-03-26`), JSON-RPC `initialize` / `ping` / `tools/list` / `tools/call` / `notifications/cancelled`, `Mcp-Session-Id` sessions bound to the token, DELETE to end a session. `tools/call` answers with an event stream that carries **elicitation** requests for approvals (when the client declares the capability), progress notifications and keep-alives. Tools: `list_connections`, `open_session`, `close_session` and the AI tools the scopes allow, each with a `session_id`.
- **AI connections** (`mcp_tokens.go`, tables `mcp_tokens`, `mcp_clients`, column `ai_sessions.mcp_token_id`): bound to a user, chosen own SSH connections, a permission mode (with automatic-mode limits), **scopes** (`read_logs`, `run_readonly`, `run_with_approval`, `edit_file_with_approval`, `transfer`) and an expiry (default `ai_mcp_token_hours` 8, at most `ai_mcp_max_token_hours` 24); bearer tokens are 256-bit random, shown once, stored as SHA-256 and revocable. Personal access tokens (`wrm_pat_…`) and OAuth tokens (`wrm_oat_…`).
- **OAuth 2.1** (`mcp_oauth.go`): protected resource metadata (RFC 9728) and authorization server metadata (RFC 8414), dynamic client registration (RFC 7591) against a redirect URI allowlist (`ai_mcp_redirect_uris`: https, or http on loopback; claude.ai, claude.com and ChatGPT callbacks by default), authorization code with **PKCE S256 only**, consent in WRM after the normal sign-in where the user picks connections, mode, scopes and lifetime, single-use codes bound to client, redirect URI and challenge (a reused code revokes its token), `iss` in the response, token revocation (RFC 7009), rate limits.
- **stdio bridge** `wrm -mcp-stdio -url … -token …` (`mcp_stdio.go`; also `-token-file`, `-ca-file`, `WRM_MCP_URL`, `WRM_MCP_TOKEN`) for clients that only start local servers: newline-delimited JSON-RPC on stdin/stdout, concurrent requests, event streams copied to stdout.
- **Scopes in the permission engine:** sessions carry their scopes; tools outside them are refused (`ai.tool_denied`, rule `scope`); `run_command` without `run_with_approval` runs only read-only commands. New MCP-only tool **`transfer_file`** (`ai_transfer.go`): copies one file between two servers of the same AI connection over SFTP, evaluated like a write of the destination (approval, read-only refusal, protected paths, automatic-mode path lists), sensitive sources refused, temporary file, size and SHA-256 checked, recorded in *File transfers*.
- **Origin checks and CORS** for `/mcp` and the OAuth endpoints: only WRM itself and `ai_mcp_allowed_origins`; CORS without credentials; the WRM session cookie never authenticates MCP.
- **UI** (`static/mcp.js`, loaded on demand): *Settings → AI connections* (the MCP address, AI connections with revoke, a new token with servers, mode, scopes, lifetime and automatic-mode limits, setup snippets for Claude Desktop with the stdio bridge, Claude and ChatGPT connectors, other MCP clients and a shell test); the **🤖 AI connected** top-bar indicator with pending approvals and one-click revoke (one or all); **approval cards** for MCP sessions with Approve, Deny and Edit and a countdown; the OAuth **consent dialog**; *Admin panel → AI connections* (active connections, revoke all, registered OAuth apps, policies). English and Croatian.
- **Policies** in `settingSpecs` (`WRM_*`): `ai_mcp_enabled` (off), `ai_mcp` (who may use AI connections: off / admins / all, default admins), `ai_mcp_scopes`, `ai_mcp_modes` (`read_only,ask`), `ai_mcp_token_hours`, `ai_mcp_max_token_hours`, `ai_mcp_oauth`, `ai_mcp_redirect_uris`, `ai_mcp_allowed_origins`, `ai_mcp_public_url`, `ai_mcp_elicitation`, `ai_mcp_transfer_max_mb`. `mePayload` gains `policy.mcp` and `policy.mcp_enabled`.
- **Audit:** `mcp.grant_created`, `mcp.grant_refused`, `mcp.grant_revoked`, `mcp.revoke_all`, `mcp.client_registered`, `mcp.client_refused`, `mcp.client_removed`, `mcp.authorized`, `mcp.authorize_denied`, `mcp.authorize_refused`, `mcp.pkce_failed`, `mcp.connected`, `mcp.disconnected`, `mcp.auth_failed`, `mcp.origin_refused`, `mcp.tool_refused`, `ai.transfer`; approvals answered in the app carry `via: mcp`. MCP sessions are AI sessions (transport `mcp`) with transcripts and recordings.
- API `/api/mcp/…` and `/api/admin/mcp/…`; `/ws/events` sends `mcp_changed`.
- Tests: `mcp_test.go` — an MCP client harness (initialize, tools/list, tools/call, elicitation), the OAuth flow (discovery, registration and the redirect allowlist, PKCE required, PKCE failure, consent, code reuse, revocation), token scopes, policies and expiry, revoke, revoke-all and the kill switch, an approval round trip through the WRM API with an edit, read-only and scope enforcement, mode rules, the destructive block, prompt injection in output not escalating, audit, transcript and recording, `transfer_file`, the stdio bridge against the HTTP server, origin and CORS checks. The fake SSH host of the tests also serves SFTP.

### Changed
- The AI engine starts sessions without a provider for the MCP transport (`aiStart`), picks the allowed modes per transport (`aiModesFor`: `ai_modes` for the panel, `ai_mcp_modes` for MCP, both narrowed by the mode rules) and checks the user per transport; turning off the built-in assistant no longer ends MCP sessions. MCP sessions cannot be prompted or re-moded through the panel API.
- `logFileTransferAs` records transfers without an HTTP request.
- Top-bar labels hide a little earlier (1260 / 1040 / 700 px of the centre area) so the new indicator never overlaps between 1100 and 1920 px.
- `tools/check-ui.js` also checks `static/mcp.js`.

## [12.0.0] — 2026-10-10 — AI assistant: permission engine and the built-in assistant

### Added
- **AI permission engine** (server-side, transport-independent: `ai_engine.go`, `ai_policy.go`, `ai_classify.go`, `ai_destructive.go`, `ai_shell.go`, `ai_tools.go`):
  - Three modes per AI session: **read-only** (fixed read tools and commands accepted by a strict read-only classifier), **ask before every change** (every change is a pending approval with the exact command or a unified diff; approve, deny, or edit first; unanswered approvals time out as denied), **automatic within limits** (explicit per-session opt-in with command and path allow / deny lists, a time limit and an action limit; everything else asks).
  - **Read-only classifier:** a shell parser (segments, quotes, redirections, substitutions, here-documents, groups, background jobs, variables) and an allowlist of about 150 commands with per-command argument checks (`systemctl status` not `restart`, `sed` without `-i` / `w` / `e`, `find` without `-delete` / `-exec`, `tail` without `-f`, `curl` without uploads, files or cloud metadata …); `sudo -n`, `timeout`, `nice` and `xargs` are unwrapped.
  - **Always-blocked destructive patterns** in every mode, also after an approval or an edit: `rm -r` of `/` and system directories, `mv` of them, recursive `chmod` / `chown` of `/`, `mkfs` / `wipefs` / `blkdiscard`, partitioning, `dd` to devices, `shred` of devices, fork bombs, cutting off SSH (stopping / masking / killing sshd, flushing the firewall, a DROP input policy), killing PID 1 or every process, stopping the audit, `crontab -r`, deleting or locking root, writes to password and group files, sudoers, `/boot`, raw disks, `/dev/mem`, sysrq, `authorized_keys` and login records, piping into a shell or interpreter; power commands only with an approval when `ai_allow_power=ask`. Checked on the parsed command and on the raw text (commands nested in `sh -c` / `eval`). Administrators add patterns (`ai_blocked_commands`).
  - **Sensitive files are never read** (password hashes, SSH / TLS private keys, cloud and tool credentials, process environments; also through wildcards and symlinks), plus `ai_read_deny_paths`.
  - **Tools:** `system_info`, `read_file`, `list_directory`, `tail_log`, `journal`, `service_status`, `list_packages`, `processes`, `disk_usage`, `network_status`, `run_command`, `write_file`, `edit_file`. Read tools run fixed scripts with shell-quoted arguments; file edits are written only if the file did not change since the diff.
  - **Prompt injection:** tool output is framed as untrusted data; only the user can change the mode (API), unknown tools are refused, every call is evaluated by the engine.
  - **Kill switch** per session, per user and global (`ai_kill_switch`, *Stop every AI session now*); a user can be turned off (`users.ai_blocked`); idle sessions end (`ai_idle_minutes`); running sessions follow policy changes.
- **Providers** (`ai_provider.go`, `ai_llm.go`, `ai_cloud.go`): the Anthropic Messages API (latest Claude models, prompt caching, eager tool input streaming and refusal fallbacks on Anthropic's own API, thinking blocks passed back unchanged), OpenAI Chat Completions, Azure OpenAI (deployment + api-version or the v1 API), AWS Bedrock (Converse with SigV4 or a Bedrock API key — own SigV4 and event-stream decoder — or the Messages endpoint), Google Vertex AI (Claude `streamRawPredict` or the OpenAI-compatible endpoint, service account JWT token exchange) and OpenAI-compatible base URLs. Streaming, per-request token and cost accounting (built-in Anthropic list prices or prices per provider), time limits and cancellation. Organisation providers (administrators) and personal keys (policy); keys encrypted at rest, never returned, scrubbed from provider errors; *Test* per provider.
- **Admin policies** in `settingSpecs`, all forceable with `WRM_*`: `ai_assistant`, `ai_kill_switch`, `ai_modes`, `ai_default_mode`, `ai_mode_rules` (per tag, folder, connection or host; the most restrictive wins), `ai_personal_keys`, `ai_provider_kinds`, `ai_models`, `ai_redact_output`, `ai_share_notes`, `ai_transcript_retention_days`, `ai_approval_timeout_seconds`, `ai_auto_max_minutes`, `ai_auto_max_actions`, `ai_allow_power`, `ai_blocked_commands`, `ai_read_deny_paths`, `ai_command_timeout_seconds`, `ai_output_max_kb`, `ai_max_steps`, `ai_idle_minutes`. `mePayload` gains `policy.ai` and `policy.ai_personal_keys`.
- **Redaction** (`ai_redact.go`) of output sent to the provider (policy) and always of transcripts and audit details: the audit's secret key names plus private keys, `Authorization` headers, URL passwords, AWS / GitHub / GitLab / Slack / OpenAI / Anthropic / Google / Stripe / npm / Vault tokens, JWTs, Azure keys, password hashes.
- **Audit and recording:** `ai.session_start`, `ai.context`, `ai.prompt`, `ai.request` (provider, model, tokens, cost), `ai.tool_call`, `ai.tool_result` (redacted output), `ai.tool_denied`, `ai.auto_allowed`, `ai.approval_requested`, `ai.approved`, `ai.denied`, `ai.approval_timeout`, `ai.approval_cancelled`, `ai.mode_changed`, `ai.stopped`, `ai.session_end`, `ai.session_killed`, `ai.kill_all`, `ai.user_blocked`, `ai.provider_*`, `ai.transcript_view`, `ai.session_refused`. Transcripts in `ai_messages` (retention), sessions in `ai_sessions`; every AI session is also a `terminal_sessions` row (protocol `ai`) with an asciicast recording of the transcript.
- **Built-in assistant panel** (`static/ai.js`, loaded on demand): an **AI** button on SSH terminal and file manager windows; a panel next to the window (full width at phone size) with provider, model and mode choice (limited by policy), the automatic-mode opt-in form, optional sharing of the connection notes, a *what can the AI see* summary, streamed answers, tool cards (command, decision, output, redactions), approval cards with an editable command or a diff and editable content, a countdown, stop and end, token and cost counters, the session history and transcripts, and an **✦ AI** badge on windows with an active session. Personal API keys (*🔑 My API keys*). English and Croatian.
- **Admin panel → AI assistant:** kill switch, organisation providers (per kind: keys, endpoints, regions, projects, API mode, models, prices), AI sessions with transcript, replay and kill, usage per user with *turn off*, the AI policies and the destructive rule list.
- API `/api/ai/…` and `/api/admin/ai/…` (server-sent events for the panel).
- Tests: `ai_policy_test.go` (classifier with ~190 cases, every destructive rule, false positives, admin patterns, globs), `ai_engine_test.go` (fake Anthropic and OpenAI-compatible servers with streamed tool calls and the fake SSH host: approve with edit, deny, timeout, read-only, destructive in every mode, automatic-mode limits, file edits, kill switch, most restrictive policy, keys never returned, redaction, audit entries, transcript and recording, prompt injection, the event stream), `ai_provider_test.go` (SigV4 test vector, Bedrock event stream, Vertex token exchange, Azure / OpenAI, Anthropic request shape, refusal, cancel, cost).

### Changed
- `tools/check-ui.js` also checks `static/ai.js`.
- *Sessions & recordings* marks AI sessions with **✦ AI**.

### Fixed
- The top bar: at about 1100 px the sign-out button overlapped *New window*.

## [11.7.0] — 2026-10-10 — Run as a service, self-update, file manager "here" or "/"

### Added
- **Run as a service on every OS** (`service.go`, `service_unix.go`, `service_windows.go`):
  - On an interactive start (stdin is a terminal, not a service, not in a container) WRM asks whether to install itself as a service that starts at boot: *Yes*, *No* (ask again next time), *Don't ask again*. The answer is remembered in `<DB_PATH>.service.json`; `-no-service-prompt` or `WRM_NO_SERVICE_PROMPT=1` skips the question.
  - Flags `-install-service`, `-uninstall-service`, `-service-status`, with `-service-name`, `-service-scope system|user` (macOS) and `-service-user`; general flags `-env-file FILE` and `-workdir DIR`.
  - **Linux:** a systemd unit with `EnvironmentFile`, `WorkingDirectory`, `User`, `Restart=on-failure` and hardening (`NoNewPrivileges`, `ProtectSystem=strict` with `ReadWritePaths`, `ProtectHome=read-only`, `PrivateTmp`, kernel / cgroup protection, `UMask=0077`, `CAP_NET_BIND_SERVICE` below port 1024); a clear message with the start command when systemd is missing. Installing as a normal user runs the installer through `sudo` / `doas`.
  - **macOS:** launchd, as a LaunchDaemon (boot) or a LaunchAgent (login).
  - **Windows:** a real Windows service (Service Control Manager), automatic (delayed) start, recovery *restart on failure*, log file; elevation through UAC or a clear "administrator rights needed" message.
  - **FreeBSD:** an rc.d script with `daemon(8) -r`; **OpenBSD:** an rc.d script with `rc.subr`.
  - The service keeps the data directory and the settings of the interactive start: `<data dir>/<name>.env` (mode `0600`) with `PORT` / `LISTEN_ADDR`, the absolute `DB_PATH`, key and certificate files, proxies and every `WRM_*` variable.
  - **Newest binary in the folder:** a service start switches to the newest valid `wrm-pro-v<semver>-<os>-<arch>[.exe]` next to the binary (it must answer `-version` with the version in its name); the update dialog offers **Restart to v…** for it.
- **Self-update from GitHub releases** (`update.go`):
  - Daily and on-demand check of the releases of `update_repo` (default this project; `WRM_UPDATE_REPO`). New settings `update_check` (on; `WRM_UPDATE_CHECK=off` for air-gapped installations), `update_repo`, `update_proxy` (else `HTTPS_PROXY` / `HTTP_PROXY`). Only the repository path and a fixed User-Agent are sent.
  - **Version badge** *⬆ Update vX.Y.Z* in the top bar and on the sign-in page, with the release notes and a link to the release.
  - **One-click update** for administrators (new policy `self_update`, `off` / `admins`, default `admins`): download of the release file for this OS / architecture with progress, **SHA-256 verification against `SHA256SUMS.txt`** (refused when the file, its line or the hash is missing or different), a `-version` sanity check, an atomic replacement of the binary (Windows: the running `.exe` is renamed to `.old` and kept as the rollback copy on the next start), a restart through the service manager or in place, and a page that reconnects by itself.
  - **One-click rollback** to the previous binary (`<binary>.previous`).
  - Docker never replaces itself: the badge stays and the dialog shows the `docker pull` / `docker compose` commands.
  - Audit entries `system.update_check`, `system.update_started`, `system.update_downloaded`, `system.update_verified`, `system.update_installed`, `system.update_failed`, `system.update_restart`, `system.update_rollback`.
  - API `GET /api/update`, `POST /api/update/check`, `/apply`, `/rollback`, `/restart`; `/api/version` and `/api/auth/config` name an available update.
- **File manager from the terminal: here, / or home** (`termcwd.go`): the 📂 button of an SSH terminal is a menu with *Open here*, *Open /* and *Open home*. "Here" works without configuring OSC 7: WRM asks the server for the shell's working directory over a separate exec channel of the terminal's SSH connection (process table from `/proc` or `ps` on Linux and BSD, the terminal's foreground process first, directory from `/proc/<pid>/cwd`, `pwdx`, `procstat` or `lsof`). OSC 7 stays the fast path; when the directory cannot be found WRM says why and offers home or `/`. WebSocket control message `cwd`.
- Tests: `service_test.go` (golden service files for every OS in `testdata/service`, env file, version comparison, newest binary in the folder), `update_test.go` (fake GitHub releases server: success, rollback, checksum mismatch, missing checksums file or line, missing asset, wrong `-version`, policy, Docker detection, local binary), `termcwd_test.go` (the cwd lookup against the fake SSH host with a real shell on a PTY).

### Changed
- `main.go`: the server runs in `serve(stop)`, so the Windows service manager, signals and restarts share one shutdown path; the shutdown now waits for the HTTP server before closing the database.
- The README's systemd example passes `-service=systemd` and allows exit code 75 (restart requested by WRM).

### Fixed
- Tests: `TestGitDeployScheduledRestart` read the notifications as soon as the run showed *done*, but the run sends them a moment later, so it failed intermittently under load. It now waits for them.
- CI: the unit tests get a 20-minute timeout (the default 10 minutes was nearly reached on the shared runners).

## [11.6.1] — 2026-10-09 — File manager from the terminal's title bar

### Added
- **📂 in the title bar of SSH terminals** ([#24](https://github.com/vedranius/web-browser-RDM-public/issues/24)) opens the file manager of the same server, so you no longer have to find the connection in a long sidebar (and cannot pick the wrong server). If a file manager of that connection is already open, it is brought to the front instead of opening another one. Shown only for SSH connections (not for Telnet, serial or BMC consoles) and only with the *Browse & download files* right on shared connections.
- **The terminal's current directory:** when the remote shell reports its directory with the standard OSC 7 escape sequence (`file://host/path`), the file manager opens there, and an already open one moves there. Shells that do not report it open the home directory. Bash example for `~/.bashrc`: `PROMPT_COMMAND='printf "]7;file://%s%s\" "$HOSTNAME" "$PWD"'`.

### Fixed
- Tests: a test SSH server could get a port an earlier test server had used, and its new host key was then refused as a changed key (`TestStatusMonitor` failed intermittently). The test servers now forget the key remembered for their address when they start.
- CI: the Docker image job pulls the Dockerfile's base images from Google's Docker Hub mirror (`mirror.gcr.io`) first, with retries and Docker Hub as the fallback, because anonymous Docker Hub pulls from shared runners were refused with `429 Too Many Requests`.

## [11.6.0] — 2026-10-09 — Git: activity and deploy history per user and branch

### Added
- **Activity tab** in the Git workspace with three views (`static/git.js`): *Git activity*, *Deploy history* and *What is where*; every view has filters and a **CSV export** of what the filters show. English and Croatian; works at phone width (tables scroll inside their cards).
- **Git activity per service** (`git_activity.go`, `GET /api/git/activity`):
  - GitLab: project events with `action=pushed` (branch and tag pushes with the commit count and the range) and the commits of the listed branches; GitHub: the commits of the listed branches and, where the token allows it, the repository activity (`/activity`: pushes, force pushes, branch creation and deletion, merges); Gitea: the commits of the listed branches.
  - Branches: the chosen branch, or the service's, the target's, the default branch, `fallback_branches` and the branches of the environments' `allowed_refs` (at most 6). A commit on several listed branches is one item with its branches; a GitHub push by a login is attributed to the name seen on that login's commits.
  - Filters: branch, author (name, user name or e-mail, case-insensitive), dates (`since` / `until`, default the last 30 days, at most 366). Summaries **per author** (commits, pushes, branches, last activity) and **per branch** (commits, pushes, authors, last activity, link to the branch's commits). Links to the commit page of every commit and the compare page of every push range (GitLab `/-/commit/`, `/-/compare/`; GitHub and Gitea `/commit/`, `/compare/`).
  - **Paging and rate limits:** listings follow the provider's next links (`Link: rel="next"`, GitLab `X-Next-Page`, GitHub's activity cursors) up to 10 pages per listing, stop early when `RateLimit-Remaining` / `X-RateLimit-Remaining` reports 5 or fewer calls left, and stop at the first commit older than the range (servers that ignore `since`). HTTP 429 and 403 with no calls left become a note with the reset time (`Retry-After`, `RateLimit-Reset`, `X-RateLimit-Reset`); the listing is marked `truncated` / `rate_limited`.
  - Cached per user, source, service, branch and dates for 3 minutes (`refresh=1` reads again); the author filter and the summaries are computed from the cached listing.
  - `GET /api/git/activity/export.csv`.
- **Deploy history** (`git_where.go`): `GET /api/git/history/cell` reads `history.jsonl` of every destination of an environment (one request per service and environment, loaded lazily three at a time, policy `git_checks`), `GET /api/git/history/runs` lists the WRM runs (update, upgrade, rollback, install, transfer) of installations without environments (environment runs are left out: their destinations keep the history). Rows carry the action, result, who, branch, version, commit (linked), the version before, counts and the per-file lists. Filters: service, environment, server, who, branch, version or commit, action (`deploy`, `rollback`, `update`, `upgrade`, `install`, `transfer`), dates. Cells are cached for 2 minutes. `GET /api/git/history/export.csv` merges everything (the server parts only with `git_checks`).
- **What is where** (`git_where.go`): `GET /api/git/where/cell` per environment — per destination the branch (`state.json`, else `VERSION.md`'s *Grana/ref*; a tag falls back to the service's branch), commit, version, who and when, the deploy id, the lock, **commits behind the branch head** (`branchHead` + the compare API, one call per branch head and per distinct commit, cached for 2 minutes) with links to the commit, the branch and the compare page, and **drift**: destinations whose commit and version differ from the most common state of the environment are marked. `GET /api/git/where/installs` lists installations without environment from their last check (no SSH). `GET /api/git/where/export.csv`.
- **Gitea as a Git source** (`kind: "gitea"`, API base `<url>/api/v1`, `Authorization: token …`, paging with `limit`): projects, branches, tags, commits, the paginated recursive tree, blobs and compare work as with GitHub, so targets, comparisons, deploys and the activity work too. A .gitignore pull request is not available for Gitea (the helper says so; the download works).
- `GET /api/git/env/cell` also returns `ref_md`, `user_md` and `updated_md` from `VERSION.md`.
- Tests: `git_activity_test.go` (fake GitLab, GitHub and Gitea: paging over `X-Next-Page`, `Link` headers and cursors, tracked branches, deduplicated commits, branch / author / date filters, servers that ignore `since`, author and branch summaries, links, cache and refresh, few calls left, HTTP 429, CSV header, byte order mark, escaping and formula neutralisation) and `git_where_test.go` (history merged from two destinations on two connections with WRM runs, filters, cache and refresh, CSV with file lists, the `git_checks` policy; commits behind the branch head, drift, installations without environment, CSV); `TestGitTargets` also runs against a fake Gitea.

## [11.5.0] — 2026-10-09 — Git: environments, deploy plans and safe deploys

### Added
- **Environments per service** (catalog `apps[].environments`, `git_env.go`; optional — catalogs and installations without environments work as before):
  - Ordered **destinations** `{server, path}` (`server` = the name of an SSH connection; paths checked like install directories) and per-environment rules: **`confirm`** (type the environment name; always for `prod`, `production`, `prd`, `live`), **`allowed_refs`** (`refs/heads/main`, `refs/tags/v*`; a pattern without `refs/` matches the short name), extra **`ignore`** patterns, an optional **`post_deploy`** command (runs in the app directory only when chosen for a run), **`keep_backups`** (1–50, default 5) and an optional **`state_dir`**.
  - Validation: two destinations on one server where one app path lies inside the other are rejected unless the outer service's excludes / environment ignores cover the inner path; a state directory inside any app path is rejected; one path cannot belong to two services.
- **Deploy plan** (`POST /api/git/env/plan`): per destination one scan over SSH (hash with CR removed, number of CRs, symlinks, unreadable files, `VERSION.md`, `state.json`, the lock) compared with the target or another ref: `new`, `changed` (`local` when it matches no version of the repository, else how many versions behind), **`eol`** (*same (CRLF/LF)*: only the line endings differ — counted separately and sent, which normalises them), `same`, `removed` (in the repository at the deployed commit, not any more; deleted only on request), `extra`, `protected`, `ignored` (with the pattern). Type changes (a directory that becomes a file or back) are listed with the server files that must be deleted; a symlink that would be replaced, files changed by hand, a missing path (the first deploy creates it and its parents) and a lock are warnings. Each destination plan has a **fingerprint**; plans live in memory for 2 hours and are **applied once**.
- **Environment deploy runs** (`git_env_run.go`; `POST /api/git/runs` with `env`): kind `update` (or `upgrade` for another ref), the destinations in order, stopping at the first failure. Per destination: a **server-side mkdir lock** with an owner file (deploy id, WRM user, WRM host, server time), the **plan computed again under the lock** (another fingerprint stops the deploy before anything is written), the selection (minus per-run exclusions; deletes only when chosen), one **tar stream** into `<path>/.wrm-incoming-<id>` verified by hash, a **backup** of replaced and deleted files to `<state>/<deploy id>/`, deletes and renames (mode and owner of replaced files kept; directories emptied by deletes removed and recorded with mode and owner), checks, automatic restore on failure, the **manifest** `deploys/<id>.json`, **`state.json`** (`current`, `previous`), the `updates.jsonl` line, `post_deploy` when chosen, a **DEPLOY entry in `history.jsonl`** (the previous state and the new / changed / deleted / excluded files, at most 500 per kind, with the real counts) and pruning of older backups. Only the deploy's **own lock** is released (its id must be in the owner file).
- **Dry run** for deploys and rollbacks: the exact options of the real run, no lock, nothing written, no typed confirmation; the run log lists what would happen. No notifications for dry runs.
- **Failure handling:** the transfer timeout is at least 1 hour; an **interrupted transfer** is recorded as partial (in the run and in `history.jsonl`) with the files that had arrived (removed again) and writes no new state; a **failed `post_deploy`** is recorded with a note (the files are on the server, the new state is current) and the remaining destinations are not started.
- **Rollback per deploy id** (`kind: rollback` with `env.deploy_id`, chosen destinations, default all, **in reverse destination order**): under the lock it checks that the deploy is still the latest there and that its backup and manifest exist; files the deploy added are **moved** to `<state>/backups/rollback-<id>/`, the current versions of changed files are kept there too, backed-up and deleted files come back atomically, removed directories are re-created with their mode and owner. **Nothing is done through a symlink**: such items are skipped and the rollback is incomplete (state unchanged; it can be repeated). Then `post_deploy` when chosen, the stored `previous` becomes current, and a **ROLLBACK** entry with `rolled_back=<id>` is written. The last 5 rollback backups are kept.
- **Stale locks:** older than max(30 minutes, the transfer timeout) by the server's clock; removed only after a confirmation in the UI (`POST /api/git/env/unlock`) by one server command that checks the owner id and the age again. Audit `git.lock_removed`.
- **Per-run exclusions** are remembered per environment with who and when (table `git_env_exclusions`) and pre-unticked on the next compare; `DELETE /api/git/env/exclusions` clears them. **Permanent exclude** (`POST /api/git/env/exclude`, scope `service` = the catalog's excludes, `env` = the environment's ignore list) writes an anchored pattern with glob characters escaped, unless an existing pattern already covers the file. Audit `git.exclude_added`.
- **Overview by environment** (`GET /api/git/env/overview`, `GET /api/git/env/cell` — one request per cell, loaded lazily): per destination version, commit, who / when, commits behind the target, the lock; a drift warning when the servers of one environment differ.
- **History** (`GET /api/git/env/history`): `history.jsonl` of every destination, one row per destination, newest first, with the current deploy of each destination.
- **Access doctor** (`POST /api/git/env/doctor`): SSH, a clean non-interactive shell, the SSH user (a warning for root), the app directory (OK when a missing path's nearest existing parent is writable), the state directory, unreadable subdirectories, signs of another deploy method (`.git`, `.svn`, `.hg`, `releases` / `current`, a symlinked path), a public web root, `sha256sum` / `rsync` / `tar` with versions, the lock.
- UI: a new **Environments** tab in the Git workspace (lazy cells, ✎ editor, ▶ Deploy with the plan, per-file checkboxes, *⊘ service* / *⊘ environment*, options, dry run, typed confirmation, *Back to the plan*; 🕘 History with expandable file lists and *Roll back…*; 🩺 Doctor). Run details show the deploy id, dry-run badge, partial transfers, skipped symlinks, `post_deploy` and notes; English and Croatian.
- Tests: `git_env_test.go` (plans with CRLF-only files, dry run, typed confirmation, one-shot plans, fingerprint mismatch, lock contention, stale lock, releasing only one's own lock, remembered and permanent exclusions with escaping, `allowed_refs`, nested-path and state-directory validation, type changes, partial transfer, `post_deploy` failure, rollback with moved-aside files, skipped symlinks, re-created directories and kept backups, doctor, drift) on the fake SSH host and the fake GitLab.

### Changed
- Exclude / ignore / protected patterns that start with `/` are **anchored**: they match the whole relative path only.
- The backup list of the per-installation *Rollback…* skips the `backups` and `deploys` directories of the environment state.
- `PUT /api/git/catalog/apps/{name}` keeps the environments of a service when the request does not send `environments`.

## [11.4.1] — 2026-10-09 — Clipboard history per user

### Fixed
- **The clipboard history leaked between accounts on the same browser** ([#30](https://github.com/vedranius/web-browser-RDM-public/issues/30)). The *Clipboard* panel stored its history in one browser-wide `localStorage` key, so after signing out and signing in as another user, the previous user's copied texts were still listed. The history is now stored per user (`wrm_clipboard_v9:u<user id>`), signing out removes the signed-in user's history from the browser, and the old browser-wide key is deleted on the next sign-in (it cannot be attributed to a user).

## [11.4.0] — 2026-10-09 — Git: every file of an installation, partial and incremental checks

### Added
- **Every file of an installation** (`git_files.go`, the details of an installation):
  - `GET /api/git/installs/{id}/files` lists every file of the server folder (one `find` over the folder with POSIX tools, `stat -c` / `stat -f` for size and time, hashes as in checks; `.git`, `.deploy-bak`, `node_modules`, `__pycache__`, virtual environments and the catalog's `ignore_dirs` pruned; backup-looking paths skipped; at most 10 000 entries, then *truncated*) and every file of the repository at the target ref below the service's subdirectory (the cached tree of the target commit; targets now keep the full commit, `commit_full`).
  - Per file: kind, size, modification time, on server / in Git / tracked, a **state** (`ok` / `old` / `modified` for tracked files; `same` / `differs` / `unknown` for repository files the catalog does not track; `extra`, `missing`, `protected`, `symlink`, `unreadable`) and a **reason** with the matching pattern (`tracked`, `excluded`, `not_included`, `protected`, `extra`, `tool`, `compiled`, `not_tracked`, `no_target`). Untracked repository files are compared through the blob cache, fetching at most 200 unknown blobs per listing.
  - Symlinks are listed with their target and never followed (an *outside* flag when they point out of the installation); unreadable files and directories are shown as such.
  - `GET /api/git/installs/{id}/file?path=&side=server|git` opens one file (view only, up to ~1.9 MB, binary detected by NUL bytes, otherwise the size and hash). The server side never follows a symlink and refuses paths that lead out of the installation (`pwd -P` check); paths are always shell-quoted. Audit `git.file_viewed`.
  - `GET /api/git/installs/{id}/diff` works for **any file in the repository at the target**, including files the catalog excludes or does not include (`informational: true`, with the reason and pattern), and files missing on the server; binary or large files are compared by hash (`same`). Protected files are still not compared.
  - `GET /api/git/installs/{id}/commits`: the commits between the commit in `VERSION.md` (or its version) and the target, newest first, with the count (GitLab `repository/compare`, GitHub `compare/a...b`).
  - UI: filters *All / Differing / Tracked only / Not tracked*, search, *Open* (server or Git side) and *Diff* per file, unified and side-by-side diff, the informational label, *Commits behind* with the list.
- **Check jobs** (`git_jobs.go`): `POST /api/git/check/jobs` with `install_ids`, `conn_ids`, `all`, `discover`, `refresh`, `stale_minutes` or `only_never`; `GET /api/git/check/jobs/current?since=N` returns the job with every installation's state (queued / running / done / error / cancelled) and the installations finished after `N`; `POST /api/git/check/jobs/current/cancel` skips the queued installations and aborts running scans without saving them. Each installation is scanned and saved on its own; concurrency is bounded per server and overall by the new settings **`git_check_per_server`** (2) and **`git_check_parallel`** (8; also used by periodic checks). One check at a time per user, as before. Audit `git.checked` (with the job) and `git.check_cancelled`.
- UI: overview filters by **service** and **server** (the search also matches host and environment); checkboxes per installation, server row and service column; *Check selected*, *Check visible*, *Check this cell*, *Check only stale* (never checked, older than 1 hour / 24 hours / 7 days) and *Cancel*; a spinner per queued or running cell; "checked X ago" and the last good result on every cell; the same selection and buttons on the *Installations* tab. *Check now* runs as a job too.
- Tests: `git_files_test.go` (the file list with reasons and patterns, an excluded file that is diffable, CRLF, a file only in Git, symlinks inside and outside, an unreadable file, binary files, quoting, commits, other users — against the fake GitLab and GitHub and the fake SSH host; check jobs: streaming, a partial check that keeps other results, stale and never-checked selections, the per-server and overall limits, cancel, an unreachable server that keeps the last good result, policies; the restore of set-aside Git sources).

### Changed
- A failed check of an installation keeps the files and counts of the last good result; the new columns `git_installs.last_ok_at` / `last_ok_state` remember it.
- Installations carry the connection's host (`host`).

### Fixed
- Upgrading a database from v11.2.0 or earlier to v11.3.0 set the `git_sources` table aside (as `git_sources_old_<time>`) instead of adding its new columns, so Git sources were lost. Columns are now added in place before the table check, and the sources of a set-aside table are restored once when `git_sources` is still empty.

## [11.3.0] — 2026-10-09 — Git: .gitignore helper and optional CI integration (phases 4–5)

### Added
- **.gitignore helper** (`git_gitignore.go`, a new *.gitignore* tab in the Git workspace) per service:
  - **Candidates:** files in state *extra* from the latest checks with their **size** and the **servers** they were seen on, and a suggested pattern (known directories such as `logs/`, known extensions such as `*.log`, or the anchored file); **standard patterns** per stack (Python, Node, Go, Java, Docker, IDE), pre-selected when the repository tree shows the stack; the catalog's **protected globs**; more patterns by hand.
  - The repository's **current `.gitignore`** (in the service's subdirectory) read through the API at the head of the service's branch.
  - **Warnings** about committed secret-looking files (`.env`, `*.pem`, `*.key`, `*credentials*`, `*secret*`, `id_rsa*`, …) and files matching the protected globs, with the advice to commit a `.example` template instead (`config.example.ini`, `.env.example`).
  - The service's catalog **exclude / protected lists** edited on the same screen.
  - **Preview** as a diff against the current file (only new patterns, under a dated comment, line endings kept) and a **download** by default.
  - **Merge request (GitLab) / pull request (GitHub)** on a new branch `wrm/gitignore-<service>-<time>` **only when chosen explicitly**, with a confirmation that names the repository (its name is typed and checked by the server). It needs an optional **write token per Git source** (encrypted, used only for this). Policy **`git_gitignore_mr`** (administrators by default, `WRM_GIT_GITIGNORE_MR`, *Admin → Policies*); `mePayload` flag `git_gitignore_mr`. Audit **`git.gitignore_mr`**.
- **Optional CI integration** (`git_ci.go`; all opt-in, WRM works without any CI):
  - **Incoming webhook per Git source** (`POST /api/hooks/git/<id>`, no session, outside the CSRF check): GitLab push / tag / release events (`X-Gitlab-Token`), GitHub push / release events (HMAC `X-Hub-Signature-256`, `ping` answered), and a generic POST (`X-WRM-Token`, or `X-WRM-Signature` over `<timestamp>.<body>` with `X-WRM-Timestamp` within 5 minutes). It queues an **immediate check** of the affected services (targets refreshed, installations compared, notifications as for periodic checks); deliveries during a check are merged. **Rate-limited** (30 per minute per hook and per client address), **replay protection** by delivery ID (`X-GitHub-Delivery`, `X-Gitlab-Event-UUID` / `Idempotency-Key`, `X-WRM-Delivery`, the generic signature), audit `git.webhook`, `git.webhook_rejected`, `git.webhook_enabled`, `git.webhook_disabled`. The secret is shown once and stored encrypted.
  - **Bundles from CI artifacts** (table `git_feeds`): a URL (GitLab job artifacts API, Jenkins artifact URL, any HTTPS URL) with an optional auth header (encrypted; `Basic user:token` is encoded), fetched by hand or every N minutes by the Git scheduler, imported as an offline bundle when it changed (the feed keeps its newest bundle). Audit `git.bundle_fetched`, `git.bundle_fetch_failed`, `git.feed_added`, `git.feed_changed`, `git.feed_deleted`.
  - **Deploy method "CI pipeline" per service** (table `git_pipelines`, the service's *Edit* dialog, policy `git_update`): *Update* / *Upgrade* trigger a **Jenkins** job (`buildWithParameters` with user + API token or the job's trigger token; queue item and build followed) or a **GitLab pipeline** (trigger token; status read with the source's read token) with `server`, `install_path` and `version`. Same plan, confirmation, policies and audit as a normal update (`git.update` / `git.upgrade` with `via: ci`, link, result); the run history shows the CI status and a link. Audit `git.pipeline_saved`, `git.pipeline_deleted`.
- API: `GET /api/git/gitignore/{app}`, `POST …/preview`, `…/download`, `…/mr`; `POST|DELETE /api/git/sources/{id}/hook`; `POST /api/git/feeds`, `PUT|DELETE /api/git/feeds/{id}`, `POST /api/git/feeds/{id}/fetch`; `PUT|DELETE /api/git/pipelines/{app}`; `POST /api/hooks/git/{hook_id}`. Git sources take `write_token`; the workspace state lists `feeds` and `pipelines` (no secrets).
- Tests: `git_gitignore_test.go` (candidates from a check on the fake SSH host, warnings, preview, download, merge request only with a write token, the explicit request and the typed repository — fake GitLab — and a pull request on the fake GitHub, policy), `git_ci_test.go` (webhook tokens, signatures, rejections, replays and rate limit; artifact feed import, unchanged, error and schedule; Jenkins job and GitLab pipeline runs, failure, policy).

### Changed
- Checks record the **size** of files found only on servers (`wc -c` in the hashing script).
- Write calls to the Git API share the error handling of read calls (`gitProvider.do`).
- The roadmap is done: `ROADMAP.md` keeps a short note for new ideas.

## [11.2.0] — 2026-10-09 — Git: new servers, install and transfer (phase 3)

### Added
- **Install wizard** in the Git workspace (*Installations → Install…*, `git_provision.go`, `git_template.go`): install services on **any SSH connection** into a chosen directory.
  - **Services and versions:** each service at its target (as in the catalog), another branch or tag (Git API) or from an imported **offline bundle** (`ref: "bundle:<source id>"`). **Target directory** per service, suggested from the server roots; system directories are refused.
  - **Pre-checks** on the target (`precheckDirs`): free disk space, a writable directory (or nearest existing parent), `tar`, the `python3` / `node` versions when the service has such files, and an **existing installation** — refused unless the overwrite is confirmed (typed server name), then the replaced files are backed up to `.deploy-bak/`.
  - **Per-host files:** protected files are never taken from the repository. Each is filled **from a template** of the repository (`<name>.example`, `.sample`, `.template`, `.tmpl`, `.tpl`, `.dist`, `.default`, `settings.example.py` → `settings.py`, or the protected file itself; placeholders `{{ name }}`, `${NAME}` / `${NAME:-default}`, `__NAME__`, `@NAME@`, `<NAME>` and `key = value` / `key: value` lines become form fields, INI keys as `section.key`), **copied** from another installation of the same service, **typed**, or left out. Field values can come from the **vault** (password or user name, only for servers the credential's host list allows). Filled files get mode `640`. Typed values and contents are **not stored** with the run.
  - **Writing:** the files travel as one tar stream into a staging directory inside the target, are verified by hash and renamed into place; language checks and the custom check run; any failure rolls back. Then `VERSION.md` and the `updates.jsonl` line, as an update writes them, and the installation is registered (as added by hand) and compared.
  - **Optional unit** (opt-in, with a warning): a systemd unit (`User=`, `WorkingDirectory=`, `ExecStart=`, `daemon-reload`, optional `enable`) or a supervisor program (`supervisorctl reread`), written as root or with `sudo -n`, never over an existing file. **Optional start** afterwards (policy `git_restart`) with the health check of v11.1.
- **Transfer wizard** (*Transfer…* for an installation): copies an installation from server A to server B (or another directory) **with its per-host configuration**, but without logs, `.deploy-bak`, caches, the catalog's `ignore_dirs`, backup-looking paths and the usual prunes (`.git`, `node_modules`, virtual environments). The wizard lists what stays behind. The files stream from A to B through WRM (`tar -c` → `tar -x`, nothing on WRM's disk) with the same pre-checks, overwrite confirmation, backup, verification, checks and rollback; `VERSION.md` gets the new host and user.
- **Policies** `git_install` and `git_transfer` (administrators by default, changeable in *Admin → Policies*, `WRM_GIT_INSTALL` / `WRM_GIT_TRANSFER`); `mePayload` flags `git_install`, `git_transfer`; the workspace state's `deploy` map has `install` and `transfer`.
- **Audit** `git.install` and `git.transfer` per installation (result, files, backup, versions, source, the new installation). Installs and transfers send the run notifications (`git.deploy_started`, `git.deploy_done`, `git.deploy_failed`) with the kind *install* / *transfer*.
- API: `POST /api/git/provision/prepare` (targets, per-host slots with template fields, server roots, pre-checks; or the source files and exclusions of a transfer); `POST /api/git/runs` takes `kind: "install" | "transfer"` with `provision`.
- Tests: `git_provision_test.go` (templates, exclusions, directories, units; install from the fake GitLab with a vault value, a typed file, a systemd unit and a start; refused and confirmed overwrite with backup; install from a bundle with a copied per-host file; transfer between two fake SSH hosts with exclusions and a supervisor program; policies).

### Changed
- Runs carry the source of a transfer (`source_conn_name`, `source_path`) and the installation an install created (`install_id`).

## [11.1.0] — 2026-10-09 — Git: update, upgrade, rollback, restarts (phase 2)

### Added
- **Deploying from the Git workspace** (`git_deploy.go`, `git_runs.go`, new tables `git_runs` and `git_pending_restarts`): *Update*, *Upgrade…*, *Rollback…*, *Restart…* and *Stamp* on the *Installations* tab (for the ticked rows) and in the details of an installation. Everything runs over WRM's SSH connections (jump hosts, vault credentials, proxies) with POSIX tools.
  - **Update** brings an installation to the newest version of the ref it follows; **Upgrade** switches it to another branch or tag (`main`, `tag:v2`) with an extra warning and a summary of the changes (files to change, new, changed locally, no longer in the target); an option makes the new ref the services' ref override. **Rollback** restores a chosen `.deploy-bak` backup. **Stamp** writes `VERSION.md` where an installation is up to date and the file is missing (or overwrites it when forced).
  - **Plan first** (`POST /api/git/plan`): the files of each installation with their state. Default selection: *old* + *missing*. Files changed by hand are written only when ticked explicitly (with a warning); protected files never.
  - **Steps per installation:** a fresh scan (a file changed since the check stops the run), a **dangling-import check** for Python (a `from mod import name` in another file of the installation whose name the new `mod.py` no longer defines; top-level `def` / `class` / assignments / imports, star imports and module `__getattr__` disable it; can be skipped per run), a **backup** to `<install>/.deploy-bak/<bundle id or wrm-YYYYMMDD-HHMM>-<YYYYMMDD-HHMMSS>/<rel>`, **atomic writes** (a temporary file in the same directory copied from the old file, so mode and owner stay, then a rename; the existing line endings are kept, new files follow most files of the installation; the written content is verified by hash), **checks** (`python3 -m py_compile` with the `.pyc` files kept out of the installation, `node --check`, `sh -n` / `bash -n`, or a custom command per service that is remembered), an **automatic rollback** on any failure (files restored, new files and directories removed, the backup dropped), then **`VERSION.md`** and a line in **`updates.jsonl`**.
  - **Compatibility with file-based deploy tools:** `VERSION.md` (Croatian labels, padded per-file table sorted by path, `(per-host)` for protected files) and `updates.jsonl` (one line, sorted keys, Python `json.dumps` style) in the first writable of `/var/log/deploytool/updates.jsonl`, `<install>/.deploy-bak/updates.jsonl` and `/tmp/deploytool-updates.jsonl`. Rollback reads which files a backup holds from WRM's run history or the `updates.jsonl` line; a file that did not exist before is deleted.
  - **Rolling** over several installations, stopping at the first failure; **live progress** per installation and step; *Stop after the current server*.
  - **Double confirmation:** a summary, and for production installations (tag `env:prod` / `prod` or a *prod* environment directory) the server name must be typed — checked again by the server.
  - **Scheduling into maintenance windows:** updates, upgrades and rollbacks run now or at a chosen time. Scheduled runs are stored, survive a WRM restart, can be cancelled, and are skipped when WRM was down past `git_schedule_grace_minutes` (30). A run interrupted by a WRM stop is marked *interrupted*.
  - **Service restart (opt-in, off by default):** the systemd / supervisor units found by the check. Per run: do not restart (the installation shows *restart pending*), restart right after a successful update, at a chosen time, or after a delay. A warning names the units and servers. Scheduled restarts are stored runs too (skipped if the update failed) and are followed by a **health check** (`systemctl is-active`, `supervisorctl status`). `sudo -n` is used when the SSH user is not root and may use it.
  - **Runs tab:** history and scheduled runs with their installations, versions and states; details with every step.
  - **Notifications** (new events): `git.deploy_started`, `git.deploy_done`, `git.deploy_failed` (failed or rolled back), `git.restart_scheduled`, `git.restart_done` (with the health check) and `git.job_upcoming` (15 minutes before a scheduled run).
  - **Policies** (administrators by default, changeable in *Admin → Policies*): `git_update` (also *Stamp*), `git_upgrade`, `git_rollback`, `git_restart`; `git_schedule_grace_minutes`. `mePayload` flags `git_update`, `git_upgrade`, `git_rollback`, `git_restart`.
  - **Audit:** `git.update`, `git.upgrade`, `git.rollback`, `git.restart`, `git.stamp` (one entry per installation with the result, files, backup and versions), `git.run_scheduled`, `git.run_cancelled`, `git.run_skipped`.
  - API: `POST /api/git/plan`, `GET /api/git/installs/{id}/backups`, `GET|POST /api/git/runs`, `GET /api/git/runs/{id}`, `POST /api/git/runs/{id}/cancel`; live update event `git_run_changed`.
- Tests: `git_deploy_test.go` (formats of `VERSION.md` and `updates.jsonl`, dangling imports; against the fake SSH host: update with CRLF, mode and protected files kept, production confirmation, stamp, rollback, failed check → automatic rollback, dangling import, ticked modified files, policies; scheduled restart with notices, health check, cancel and grace period).

### Changed
- The comparison of one installation is shared by checks and runs (`evalInstall`); installations carry `restart_pending`.

## [11.0.0] — 2026-10-09 — Git workspace: compare services (phase 1)

### Added
- **Git workspace** (`git.go`, `git_source.go`, `git_bundle.go`, `git_scan.go`, `git_monitor.go`, `/api/git`): a full-screen workspace opened from the **⎇ Git** button in the top bar. Its code (`static/git.js`) loads only when it is opened. Tabs *Overview*, *Services*, *Installations* and *Settings*. **Read-only on servers** in this version: nothing is updated, installed or restarted.
  - **Sources:** GitLab API v4 and GitHub REST (cloud or self-hosted, groups / organisations with subgroups) with a read-only token, encrypted at rest and never sent to the browser. Self-signed Git servers are pinned on first use. One recursive tree listing per commit (cached forever) gives every file's blob ID; file contents are fetched only for blobs not seen before and cached by blob ID (new tables `git_sources`, `git_trees`, `git_blobs`).
  - **Offline bundles:** import a `.tar.gz` (`<name>/bundle.json` + `payload/<app>/<rel>`, the format of file-based deploy tools; `deploytool.py` and `README.txt` are ignored) when WRM cannot reach the Git server, with its age and source; protected files of the payload are never used. **Export** writes the same format (`bundle.json` with indent 1 and sorted keys, payload without protected files) for servers that only a file-based tool can reach.
  - **Service catalog:** project, ref (`tag:latest`, `tag:vX` or a branch), branch, tag filter, subdirectory = installation root, include / exclude globs, **protected** per-host files, fingerprint files, install hints and kind (app, library, tool). **Suggestion from the repository tree** (subdirectory, fingerprint, protected and exclude globs, kind). Import (replace or merge) and export as catalog JSON. Services of imported bundles are used even when the catalog does not list them.
  - **Refs:** branch-aware tags (only tags whose commit is on the branch count; `tag:latest` falls back to the branch head with a warning), numeric version sort (`v1.10` > `v1.9`), fallback branches, and a ref choice per run: as in the catalog, one branch for all, each project's default branch, or per service.
  - **Discovery over SSH:** one `find` per server over the configured roots (depth 3) recognises installations by their fingerprint files (or install hints), skips copies and backups (`*_BKP`, `backup`, `.deploy-bak`, `kopija`, … as path segments; configurable) and derives an environment label from the path (`/srv/scripts/test/App` → `test`, `App_prod` → `prod`). Installations can also be added by hand.
  - **File-level comparison:** the server hashes its files with POSIX tools (`tr -d '
' < f | sha256sum`, no Python); WRM compares them with the target and with the **history of every file** (the earlier tags of the branch). File states *ok*, *old* (*n behind*), *modified*, *missing*, *extra*, *protected*; installation states *needs update*, *review*, *up to date*. Details show the version from `VERSION.md`, the systemd / supervisor units that mention the directory, the files and a coloured **diff** (server → target).
  - **Overview:** targets per service (version, latest tag, source, warnings) and a servers × services matrix with filters (state, environment, folder, tag, text).
  - **Periodic checks** while WRM runs (per user, opt-in, every `git_check_interval_minutes`): they never change anything and notify through the notifications module — new events **`git.new_version`**, **`git.drift`** (files changed by hand) and **`git.unreachable`**, one digest per round.
  - Policies `git_enabled` (hides the workspace completely), `git_checks` (who may run checks: everyone by default), `git_check_interval_minutes` (60, 0 = off) and `git_backup_words`; `mePayload` flags `git` and `git_checks`.
  - Audit events `git.source_added`, `git.source_changed`, `git.source_deleted`, `git.catalog_saved`, `git.service_saved`, `git.settings_changed`, `git.bundle_imported`, `git.bundle_exported`, `git.checked`, `git.install_added`, `git.diff_viewed`.
- `tools/check-ui.js` also checks `static/git.js` (syntax and translation keys).
- Tests: `git_test.go` (fake GitLab and GitHub servers: targets, branch-aware tags, fallbacks, blob cache, suggestion, tokens; bundle export / import round trip and unsafe archives; catalog import / export; discovery, comparison, `VERSION.md`, units, diff and notifications against a fake SSH host; policies and ownership).

## [10.10.0] — 2026-10-08 — folder bookmarks

### Added
- **Folder bookmarks** (`bookmarks.go`, `/api/bookmarks`, new table `bookmarks`): named remote directories with an optional colour and note, for **one connection**, the connections of **a folder**, the connections with **a tag**, or **all SSH / SFTP / FTP connections** (global).
  - **File manager:** a ★ button next to the path bar with *Bookmark this directory…*, the bookmarks of the connection and *Manage bookmarks…*. A bookmark whose directory is missing on the server gives a clear message and keeps the current folder open.
  - **Terminal:** ★ in the window's title bar and the bookmarks in the Ctrl+Shift+Space picker type `cd -- '<path>'` (shell-quoted on the server; a leading `~/` stays unquoted so the shell expands it). In a broadcast group, 📣 next to a bookmark sends the `cd` to every terminal, each resolved for its own connection (`POST /api/bookmarks/resolve`).
  - **Path variables** filled in by the server per connection: `~`, `$USER` / `${USER}` / `{user}`, `{host}`, `{name}` — e.g. `/opt/app/servers/{name}/downloads`.
  - **Start directory:** a bookmark can be the start directory of its connections. The file manager opens there; a terminal types the `cd` on connect (also after a reconnect), before the run-on-connect snippets. The most specific one wins (connection, tag, folder, global). *★ Start directory…* in the context menu of a connection and a folder.
  - **Management:** rename, reorder, move between scopes, delete. Global bookmarks can be **shared with share members** (read-only, `GET /api/bookmarks?conn=…&share_token=…`).
  - **Import of WinSCP bookmarks:** *Manage bookmarks → Import WinSCP.ini* (`POST /api/bookmarks/import-winscp`) reads the `[Configuration\Bookmarks\Remote\…]` sections as global bookmarks; existing ones are skipped.
  - Bookmarks are part of the configuration export and import (folders and connections are mapped), copied with *Duplicate*, and removed with their connection, folder or user.
  - Audit events `bookmark.created`, `bookmark.updated`, `bookmark.deleted`, `bookmark.imported`; live update event `bookmarks_changed`.
- The file manager opens `~/…` paths below the home directory (SFTP) or the login directory (FTP).
- Tests: `bookmarks_test.go` (scopes, ownership, validation, variables, `cd` quoting, reorder, duplicate / delete, export / import, share access, WinSCP.ini, start directory on connect).

## [10.9.1] — 2026-10-08 — PuTTY session import

### Added
- **Import from PuTTY** (*Settings → Data*, `POST /api/config/import/putty`, `putty.go`): a registry export (`.reg`, UTF-16 LE with a BOM as written by regedit, or UTF-8) of `HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions`.
  - URL-encoded session names are decoded (`My%20Server`); `HostName` (also `user@host`), `PortNumber`, `UserName` and `Protocol` are imported into the folder *PuTTY*. *Default Settings* is not imported but supplies the defaults of the other sessions.
  - SSH and Telnet sessions become connections; raw, rlogin and serial sessions are listed as skipped. `PublicKeyFile` gives a note (convert the `.ppk` with PuTTYgen and add the key).
  - **Proxies:** SOCKS4 / SOCKS5 / HTTP settings (`ProxyMethod`, `ProxyHost`, `ProxyPort`, `ProxyUsername`, `ProxyPassword`, `ProxyDNS`) become saved proxies linked to the connections. One proxy per distinct setting; an identical proxy the user already owns is reused, so importing again creates none. PuTTY's *SSH proxy* becomes the jump host (by name); Telnet and local-command proxies get a note. The policy `proxies` applies.
  - **With mRemoteNG:** connections whose `PuttySession` (also inherited) names an imported PuTTY session get its proxy, whichever file is imported first (new table `putty_sessions`, new column `connections.putty_session`).
  - The import result and the audit event `config.imported` also count `proxies` and `proxy_links`.
- Tests: `putty_test.go` (UTF-16 and UTF-8 files, defaults, proxies and their reuse, SSH proxy → jump host, mRemoteNG linking in both orders).

## [10.9.0] — 2026-10-06 — proxies, folder defaults, notifications

### Added
- **Saved proxies** (`proxy.go`, *🔑 SSH keys & credentials → 🌐 Proxies*, `/api/proxies`): SOCKS5 with an optional user name and password (RFC 1929) and *DNS at the proxy*, SOCKS4 / SOCKS4a, HTTP CONNECT with Basic authentication, and the type **WRM SOCKS tunnel of a connection** (a dynamic tunnel that WRM starts when needed and stops after 30 minutes without traffic).
  - The login is encrypted or a vault credential and never sent to the browser; proxies are shared like credentials (grants, everybody) without revealing it.
  - A **Proxy** field in the connection dialog next to *Connect via (jump host)*, with *＋ New proxy…* and the route of the connection.
  - **Every TCP path goes through it:** the SSH hops in `dialSSH` (terminal, SFTP, search, transfers, tunnels, key deploy, rotation, network tools from a server), FTP/FTPS (control and data connections), the local relay of RDP / VNC / Telnet, web interfaces, live status and *Check now*, Redfish, *Test connection*.
  - **Proxy + jump host:** the proxy is reached through the jump hosts; jump hosts can have proxies themselves. The route is shown and audited, e.g. `bastion → socks5://10.1.1.1:1080 → app-01` (sidebar, dialog, terminal, *Sessions & recordings*, audit details).
  - Errors tell proxy and target apart: proxy unreachable, authentication required / failed, SOCKS5 and SOCKS4 reply codes, HTTP status (`407`, `502`, …).
  - IPMI and Serial-over-LAN (UDP) through a proxy are refused with a clear message.
  - Policy `proxies` (`all` / `admins` / `off`): who may define proxies. Audit events `proxy.created`, `proxy.updated`, `proxy.deleted`, `proxy.granted`, `proxy.grant_revoked`, `proxy.tested`.
  - Export lists the user's proxies (passwords only with secrets) and names them in connections (`proxy_ref`); import creates missing ones and links by name. *Duplicate* keeps the proxy.
  - **Docker:** `docker-compose.yml` maps `host.docker.internal` to the Docker host (`extra_hosts`); WRM warns when a proxy at 127.0.0.1 is saved while it runs in a container (`/.dockerenv`).
- **Folder defaults:** a folder can carry a default jump host and a default proxy (*Folder settings…* in the folder's context menu, `PUT /api/folders/{id}`). Its connections, also new ones, inherit them unless they choose their own (*no jump host / no proxy (not the folder's)*). The connection dialog shows the inherited value (*— as the folder: bastion —*), the tree shows ⤳ after the folder name. Export and import keep them. Audit event `folder.updated`.
- **Notifications module** (`notify.go`):
  - channels configured by administrators (*Admin panel → Notifications*): SMTP e-mail (STARTTLS / TLS), Telegram, Slack / Mattermost / Rocket.Chat (incoming webhooks), Microsoft Teams (Workflows webhook, Adaptive Card), Discord, ntfy, Gotify, Pushover and a generic JSON webhook signed with `X-WRM-Signature: sha256=HMAC-SHA256`; secrets encrypted and never returned; **Send test** per channel; last delivery and last error;
  - users subscribe per event and channel (*Settings → Notifications*), optionally with their own recipient (e-mail, Telegram chat, ntfy topic, Pushover key), and set **quiet hours** in their time zone;
  - one **digest** per user and channel for everything of one check round, at most one message per `notify_min_interval_seconds`; waiting messages are stored and retried with backoff; texts in English or Croatian;
  - first events: a server went down / is up again (live status; owner only, monitoring opt-out respected) and credential rotation due / incomplete;
  - policies `notifications_enabled` and `notify_min_interval_seconds`; audit events `admin.notify_channel_created` / `_updated` / `_deleted` / `_tested`, `notify.settings_changed`, `notify.tested`.
- Tests: `proxy_test.go` (in-process SOCKS5 and HTTP CONNECT proxies with and without authentication for terminal, SFTP, status, jump host + proxy, web interface, desktop relay, WRM tunnel proxy; errors, policy, sharing, Docker warning; folder defaults) and `notify_test.go` (fake HTTP endpoints and a fake SMTP server; digests, rate limit, quiet hours, retries, events).

### Fixed
- **FTP / FTPS behind a jump host** (and now through a proxy): EPSV data connections went to the address the library took from the control connection (the jump host channel or the proxy) instead of the FTP server, and FTPS data connections were not encrypted after `PROT P`. Both are fixed (`routedFTPDial`).

### Changed
- E-mail notifications log in with AUTH PLAIN or, when the server offers only that (e.g. Exchange), AUTH LOGIN.
- `connections.jump_conn_id` NULL now means "as the folder" (still direct without a folder default); deleting a jump host makes its connections fall back to the folder default.
- The admin overview shows proxies and notification channels; the audit filter has *Proxies*, *Folders* and *Notifications*.

## [10.8.1] — 2026-10-05 — fixes: larger imports, install hints

### Fixed
- **mRemoteNG, OpenSSH config and inventory imports of larger files.** The security middleware limited every API body to 1 MB except `/api/config/import`, so `/api/config/import/mremoteng`, `/api/config/import/sshconfig` and `/api/inventory/*` failed with *Bad JSON* from about 1 MB on.
  - One limit for all imports: files up to 20 MB (`maxImportFile`), request bodies up to 32 MB (`maxImportBody`, room for JSON escaping and base64) for `/api/config/import`, `/api/config/import/*` and `/api/inventory/*` (`bodyLimitFor`). The separate inventory limits (10 MB file, 30 MB body) are gone.
  - A file or body above the limit gets **HTTP 413** with its size and the limit (*The file is too large (21.0 MB): the import limit is 20.0 MB*) instead of *Bad JSON* (`decodeImportJSON`).
  - The browser checks the size before sending (mRemoteNG / OpenSSH config import, WRM configuration import, inventory file).

### Changed
- **Install as an app** (*Settings → General*) explains what to do in the current browser:
  - the steps for Chrome / Edge / Brave (the install icon in the address bar or the menu), Safari on macOS (*File → Add to Dock*), iPhone / iPad (*Share → Add to Home Screen*) and Android;
  - the real reason when installing is not possible: plain `http://` to an address other than `localhost` (HTTPS with a trusted certificate is needed), an untrusted certificate, Firefox on the desktop;
  - the *Install* button is shown only when the browser can install right away.
- README: *HTTPS with Caddy* in the reverse proxy section (public name with Let's Encrypt, or `tls internal` for internal names and IP addresses), the import size limit, troubleshooting rows for installing and for too large files.

### Added
- Tests (`import_limits_test.go`), through the real middleware chain: a multi-MB `confCons.xml` and a large CSV import; oversized bodies (with and without `Content-Length`) and oversized files get the clear 413; the browser's limit matches the server's.

## [10.8.0] — 2026-10-02 — quick connect, notes, network tools & installable app

### Added
- **Quick connect** (`quick.go`): a field at the top of the sidebar takes `user@host:port` or a URL-like target (`ssh://`, `sftp://`, `rdp://`, `vnc://`, `telnet://`, `https://`) and connects without the connection dialog:
  - a small dialog for the login (password, vault credential or stored key) and an optional jump host; `POST /api/connections/quick`;
  - the connection is a **temporary** connection of its owner (`connections.temp_until`) — same checks, host keys, recording and audit (`connection.quick`) — listed under *Quick connections*, kept 24 hours after its last use (open terminals keep it alive), at most 20 per user, not monitored, not exported;
  - 💾 / *Save as a connection* turns it into a normal connection (`PUT … {temporary: false}`).
- **Notes & runbook** per connection (`connections.notes`, up to 20 000 characters): a small markdown (headings, lists, bold, code, code blocks, links) rendered as escaped text; edited in the connection dialog or a notes window; 📝 in the sidebar, the terminal window bar and the context menu; owner only (not share members); kept by export / import and *Duplicate*; lists only carry `has_notes`.
- **Network tools** (`nettools.go`, 🧰 *Tools*, `POST /api/nettools`): port check (up to 100 ports and ranges; open / closed / no answer), ping, traceroute (`traceroute`, `tracepath` or `tracert`), DNS (addresses, CNAME, MX, TXT, PTR) and an HTTP/TLS check (status, server, redirect target, TLS version, certificate subject, issuer, expiry and days left, names, trust) — **from the WRM server or from one of the user's SSH connections through its jump hosts** (port and HTTP checks over SSH channels, the other tools with the server's commands); targets are validated as host names or addresses, one run per user at a time, audit event `nettool.run`, policy `network_tools` (off / admins / all, default all); context menu entries *Network tools from this server…* and *Check ports of this host*.
- **Installable app (PWA)** (`pwa.go`): `/manifest.webmanifest` and a service worker (`/sw.js`) that caches the app's own static files per version and shows an offline page; *Settings → General → Install as an app*.
- Tests: quick connect (targets, temporary connections, expiry with and without open terminals, limit, export), notes (save, limit, list flag, duplicate), network tools (ports and HTTP/TLS from WRM and through SSH, DNS, ping and traceroute on a server, validation, ownership, policy) and the PWA endpoints.

### Fixed
- The top bar overlapped its buttons at medium widths (and the version label never collapsed); it now gives way step by step.

## [10.7.0] — 2026-10-02 — out-of-band management, consoles & serial ports

### Added
- **Out-of-band management** (`bmc.go`): a BMC per connection (`connections.bmc`, JSON with the password encrypted, or a vault credential):
  - **Redfish** (iDRAC, iLO, XClarity, Supermicro, OpenBMC): power state, health, maker, model, serial number, BIOS, CPUs, memory, BMC firmware, power draw, inlet temperature; certificates pinned on first use like SSH host keys (shared with FTPS: `pinnedTLSConfig`);
  - **IPMI** with `ipmitool` (`chassis status`, `mc info`, `fru print`), the password in the environment (`-E`);
  - power actions on, graceful shutdown/restart, force off/restart, power cycle and NMI — only those the BMC offers — with confirmation and audit events `bmc.power` / `bmc.power_failed`;
  - through the connection's jump hosts: Redfish over an SSH channel of the last hop, `ipmitool` on the last hop (password over stdin).
- **Serial consoles** in terminal windows (`console.go`): over the BMC's SSH with the vendor's console command (iDRAC `console com2`, iLO `vsp`, …, suggested from the BMC maker), or IPMI **Serial-over-LAN** on the jump host or on the WRM server in a pseudo terminal; owner only, audited and recorded (`protocol` console / sol); saved in workspace sessions.
- **Serial ports** of the WRM server: protocol `SERIAL` (device name, speed, data bits, parity, stop bits, flow control), raw terminal, *Test connection*; policy `serial_ports` (off / admins / all, default admins), Linux.
- Policies `bmc_enabled` (`BMC_ENABLED`) and `serial_ports`; ipmitool in the admin overview; audit filters for `bmc.` and `inventory.`.
- Tests: Redfish against a fake BMC (status, actions, pinning, changed certificate, wrong password, jump host, credential), IPMI with a fake ipmitool (locally and on a jump host), Serial-over-LAN (PTY and jump host), the BMC SSH console and a serial port on a pseudo terminal.

### Changed
- The terminal WebSocket runs on terminal backends (SSH shell, BMC console, SOL, serial port) instead of an SSH session only.

### Fixed
- Saved workspace sessions did not include remote desktop windows.

## [10.6.0] — 2026-10-02 — tags & inventory import (CSV, Excel, NetBox)

### Added
- **Tags** on connections (`connections.tags`): free tags and tags with a key (`env:`, `site:`, `rack:`, `role:`, `tenant:`, `platform:`, `cluster:`), normalised to lower case, at most 30:
  - a tags field with suggestions in the connection dialog, bulk tagging (`POST /api/connections/bulk` action `tag`, add / remove);
  - a tag filter bar in the sidebar (several tags = all must match), tag search (`#tag`, several words);
  - environment badges (PROD / staging / test / dev colours) and a red edge for production connections;
  - kept by export / import and *Duplicate*.
- **Inventory import** (`inventory.go`, *Settings → Data → Import inventory*):
  - **CSV** with automatic separator (`,` `;` tab `|`, Excel's `sep=` line) and encoding detection (UTF-8, UTF-16, Windows-1250), and **Excel .xlsx** (read with archive/zip + encoding/xml, sheet selection, shared and inline strings);
  - a column mapping guessed from English and Croatian header names, checked and changed in a preview;
  - host fields with ports, users, CIDR suffixes, IPv6 and URLs; Windows platforms become RDP; environment / site / rack / role / platform columns become tags; jump hosts by name;
  - **NetBox** devices and virtual machines through the REST API (filters, all pages, primary IP or DNS name, environment custom field), with **sync**: `connections.ext_id` remembers the NetBox object, a later import updates address, NetBox tags and folder (`ext_tags`) and lists objects that disappeared; address and token can be remembered per user (token encrypted, table `inventory_sources`);
  - for both: the login of the imported connections (none, a **vault credential** or a **stored SSH key**), default user and protocol, folders by column / site / role / tenant / one folder, extra tags.
- Audit events `inventory.imported`, `inventory.netbox_imported`, `connection.tagged`.
- Tests: tags (normalisation, bulk, export/import), CSV encodings and separators, an XLSX import, NetBox import and sync against a fake NetBox with pagination.

### Fixed
- Texts that still said mRemoteNG RDP/VNC/Telnet connections are skipped (they are imported since 10.4).

## [10.5.0] — 2026-10-02 — SSH keys & credentials vault

### Added
- **SSH key store** (🔑 *Keys* in the top bar, `keys.go`):
  - generate ED25519, RSA 2048/3072/4096 or ECDSA 256/384/521 keys, or import OpenSSH/PEM private keys (with passphrase) and public keys; private keys are encrypted at rest;
  - new auth method **SSH key from the key store** (`KEY_REF`, `connections.key_id`); last use is recorded;
  - **deploy** a key to many SSH connections at once like `ssh-copy-id` (through jump hosts, idempotent, keeps other lines, restores SELinux contexts) and optionally switch them to the key after a test login;
  - **revoke** a key from many servers (never the key WRM logs in with);
  - **who has access**: read a connection's `~/.ssh/authorized_keys`, match the keys of the key store, mark WRM's login key, remove single entries;
  - public key copy/download, rename, delete (refused while in use), export of the private key with re-authentication and an optional new passphrase.
- **Credentials vault** (`credentials.go`):
  - a credential holds a user name with a password and/or a stored key; new auth method **Credential from the vault** (`CREDENTIAL`, `connections.credential_id`) for SSH, SFTP, RDP, VNC, Telnet and FTP;
  - sharing with selected users (administrators: everybody) without revealing the secret; host lists (glob patterns, CIDR) limit where it can be used — for grantees also the jump hosts;
  - **password rotation** on all SSH servers that use it: pre-flight login check, `passwd` over a PTY, verification of the new login, all-or-nothing with rollback, a *pending* password if a rollback fails, live progress per server; a check-only mode;
  - rotation reminders (badge, admin overview warning), owner-only *Show* with re-authentication, usage list.
- Audit events `ssh_key.*` and `credential.*`; audit filter entries for both; admin overview card with key, deployment and credential counts.
- Bulk action **🔑 Key** for selected connections; context menu entries *Who has access…* and *Deploy SSH key…*.
- Export/import reference keys and credentials by name (`key_ref`, `credential_ref`) instead of copying them.
- Tests with a fake SSH server that runs scripts in `sh` and simulates `passwd`: key store, deploy/revoke/who has access, vault sharing, host and jump-host restrictions, rotation with success, rollback and incomplete rollback.

### Changed
- Deleting an account also deletes its snippets, tunnel definitions, keys and credentials (and its grants).
- RDP, VNC, Telnet and FTP connections offer only password and vault logins in the connection dialog.

## [10.4.0] — 2026-10-02 — RDP, VNC & Telnet in the browser

### Added
- **Remote desktop** connections — **RDP**, **VNC** and **Telnet** — in WRM windows, through guacd (Apache Guacamole proxy daemon):
  - WRM performs the guacd handshake with the stored credentials and options and relays the Guacamole protocol over a WebSocket (`/ws/desktop`, subprotocol `guacamole`);
  - browser → guacd input is limited to an allow-list of instructions; internal pings are answered by WRM;
  - guacamole-common-js 1.5.0 (Apache 2.0) is bundled and loaded on demand.
- **Desktop windows:**
  - scale-to-fit or 1:1 with scroll bars, full screen, dynamic resolution for RDP (display update);
  - keyboard and mouse (also touch), **Ctrl+Alt+Del**, **send to the remote clipboard**, **type text as keystrokes**;
  - remote clipboard into WRM's clipboard panel, sound from RDP, prompts for credentials the server requires, reconnect;
  - tabs with connection state; restored by saved workspace sessions.
- **Connection options:**
  - RDP: domain, security (NLA/TLS/RDP/Hyper-V), keyboard layout, resize behaviour, colors, start program, certificate, console session, sound, clipboard, wallpaper, **RD Gateway**;
  - VNC: colors, pointer, view only, clipboard;
  - Telnet: font size, colors, login/password prompt patterns.
- **Behind jump hosts:** a temporary tunnel through the jump hosts is opened for guacd for the duration of the session.
- **Audit and recording:**
  - desktop sessions in *Sessions & recordings* (`protocol` rdp/vnc/telnet) and audit events `desktop.open`, `desktop.close`, `desktop.error`;
  - the screen stream is recorded (gzip, SHA-256) and replayed in the browser with play/pause/seek, or downloaded as `.guac`.
- **Admin overview** shows whether guacd answers and its version; *Test connection* checks the port and guacd.
- **Policies** `desktop_enabled` (`REMOTE_DESKTOP_ENABLED`), `guacd_address` (`GUACD_ADDRESS`), `desktop_tunnel_bind`.
- `docker-compose.yml` contains a `guacd` service sharing WRM's network.
- **mRemoteNG import** brings RDP (domain, console, colors, RD Gateway, sound), VNC (view only) and Telnet connections instead of skipping them.
- **Live status** checks RDP/VNC/Telnet ports too.
- **Tests:**
  - Guacamole codec;
  - desktop relay against a fake guacd (handshake parameters, allow-list, ping, wrong password, policy, recording and download);
  - VNC through a jump host.

### Fixed
- SQLite `busy_timeout` is now set on every pooled database connection (it reached only one before), avoiding rare "database is locked" errors under concurrent load.

## [10.3.0] — 2026-10-02 — snippets, broadcast input & live status

Daily terminal work for data center admins (extension plan phases 4–6, first part). Release names no longer carry a suffix: this release is `v10.3.0`.

### Added
- **Snippets** — saved commands:
  - in the right panel (⚡ tab, grouped, searchable) and in a quick picker in every terminal (**Ctrl+Shift+Space** or ⚡ in the title bar);
  - click runs in the focused terminal, Shift+click inserts without Enter;
  - scope: all connections, one folder, or one connection; multi-line commands;
  - variables `{{host}} {{port}} {{user}} {{name}} {{folder}} {{wrm_user}} {{date}} {{time}}` and prompts `{{?Label}}` / `{{?Label=default}}` with a preview;
  - shared snippets for all users (administrators); example set; export/import; copied with *Duplicate*.
- **Run on connect:** snippets typed into the shell by the server once a terminal of a matching connection is connected (also after reconnects), e.g. `sudo -i`, `cd /srv/app`, `tmux attach`. Created from the connection or folder context menu; audited as `terminal.auto_run`.
- **Broadcast input:**
  - type into several terminals at once (📣 in the top bar, per-window toggle, orange frame and status bar);
  - dangerous commands (rm -rf, shutdown, mkfs, dd, iptables -F, DROP TABLE, kubectl delete, …) and multi-line pastes ask first, with Cancel as the default;
  - snippets run on every terminal of the broadcast with their own variables;
  - every start and stop is audited per terminal (`terminal.broadcast`); policy `broadcast_enabled` (`BROADCAST_ENABLED`).
- **Live up/down status:**
  - a background monitor checks every saved connection (TCP connect; SSH and FTP read the greeting and close politely, like Nagios check_ssh) once per host:port and round, retrying before reporting down;
  - status dots in the sidebar with latency, banner and "since"; down counter per folder; "show only down" filter;
  - notifications (also desktop notifications in the background) when a server goes down or comes back;
  - *Check status now* for a connection or a whole folder, also through jump hosts;
  - connections behind jump hosts show their jump host's state, or are checked through it (`status_jump_checks`);
  - per-connection opt-out (*Monitor up/down status*); policies `status_enabled` (`STATUS_ENABLED`), `status_interval_seconds`, `status_jump_checks`; API `GET /api/status`, `POST /api/status/check`; event `status_changed`.
- **Tests** for snippets (API, sharing, validation, variables, run on connect, export/import), broadcast audit and the status monitor (up/down, banners, jump hosts, check now, FTP greeting).

### Changed
- Release tags, binaries and the Docker image no longer have a suffix after the version number (`v10.3.0`, `wrm-pro-v10.3.0-linux-amd64`). CI names the release files after the tag.
- Confirmation dialogs for dangerous actions focus *Cancel*, so Enter cannot confirm them by accident.
- The "terminal connected" marker in the sidebar is blue (green is now the live status).

### Fixed
- Deleting several connections at once also stops their tunnels, removes their tunnel definitions and detaches connections that used them as jump host (single deletes already did).

## [10.2.0] — 2026-10-02 — jump hosts, SSH tunnels & mRemoteNG import

Phase 5 (fleet operations) of the extension plan, first part: what mRemoteNG, PuTTY and `ssh -J` users need to manage servers behind bastions (see [ARCHITECTURE.md](ARCHITECTURE.md)).

### Added
- **Jump hosts** (ProxyJump):
  - any connection can go through another saved SSH connection, in chains of up to 5 hops; loops, foreign and non-SSH jump hosts are refused;
  - used by terminals, the file manager (SFTP and FTP/FTPS), search, editor, server-to-server transfer, *Test connection*, tunnels and web interfaces;
  - every hop authenticates with its own credentials and its host key is verified;
  - the route is shown in the sidebar (⤳), connection test, terminal, session history (`jump_path`) and audit log.
- **SSH tunnels** (port forwarding) per connection:
  - local (`-L`), remote (`-R`) and dynamic SOCKS5 (`-D`) tunnels, through the jump hosts too;
  - listen address and port (0 = automatic), target, *Open as* http/https + path, start mode *manual*, *with terminal* or *always* (also after a restart);
  - templates: web interface (HTTPS/HTTP), SSH, RDP, VNC, PostgreSQL, MySQL/MariaDB, iDRAC/iLO/IPMI, SOCKS;
  - a hint in plain words for every tunnel;
  - each tunnel keeps its own SSH connection with a 30 s keepalive and automatic reconnect (backoff 2–60 s) while its port stays open;
  - statistics: open and total connections, bytes up/down;
  - **🔀 Tunnels** panel (top bar, with a counter of running tunnels): state, listen → target, traffic, *Open*, *Copy address*, *Start/Stop*; administrators can show and stop the tunnels of all users;
  - connection context menu: *Tunnels*, *Start / Stop all tunnels*, *Add tunnel…*;
  - live updates through `/ws/events` (`tunnels_changed`).
- **Web interface connections** (protocols `HTTPS` / `HTTP`, with a path): a double-click opens the page in a new tab, directly or, behind a jump host, through an automatic temporary tunnel (30 minutes idle limit). *Test connection* checks that the port answers.
- **Import from mRemoteNG** (`confCons.xml`):
  - AES-GCM and legacy AES-CBC passwords, master password (checked with `Protected`, asked for when needed), full-file encryption;
  - inheritance of user name, password, port, domain and SSH tunnel;
  - containers become folders; `SSHTunnelConnectionName` becomes the jump host;
  - unsupported protocols and duplicates are reported as skipped.
- **Import from OpenSSH config:** `Host` blocks, `Host *` and wildcard defaults, `HostName`/`User`/`Port`, `ProxyJump` (implicit jump hosts are created), `ProxyCommand ssh -W`, `LocalForward`/`RemoteForward`/`DynamicForward` (tunnels that start with the terminal), `IdentityFile` (when server key files are allowed).
- **Policies:** `tunnels_enabled` (`TUNNELS_ENABLED`), `tunnel_users`, `tunnel_bind_any`, `tunnel_remote_forward`, `tunnel_idle_minutes`. Running tunnels that a policy change no longer allows are stopped.
- **Audit events:** `tunnel.configured`, `tunnel.start`, `tunnel.stop` (with traffic), `tunnel.error`, `tunnel.reconnected`, `web.open`; connection changes record the jump host.
- **Admin panel → Overview:** active SSH tunnels with *Stop*. **Admin status API** includes the tunnels.
- **API:** `GET/PUT /api/connections/{id}/tunnels`, `GET /api/tunnels[?all=1]`, `POST /api/tunnels/{key}/start|stop`, `POST /api/connections/{id}/open-web`, `POST /api/config/import/mremoteng`, `POST /api/config/import/sshconfig`. Connections have `jump_id`, `web_path`, `route` and `tunnels`.
- **Tests:**
  - the in-process SSH test server also handles `direct-tcpip` and `tcpip-forward`;
  - new tests for two-hop jump chains (terminal, SFTP, connection test, validation), local/SOCKS/remote tunnels (traffic, ownership, policies, audit, start modes) and web interfaces;
  - new tests for mRemoteNG import (both encryption formats, master password, inheritance) and OpenSSH config import.

### Changed
- Export and import of WRM's own format include jump hosts and tunnels (IDs are remapped).
- *Duplicate* copies the jump host, path and tunnels of a connection. Deleting a connection stops its tunnels and makes connections that used it as jump host direct.
- Editing a connection restarts its running tunnels with the new settings.
- The connection dialog is wider and fully translated (labels were partly English in Croatian).

### Fixed
- The connection dialog no longer moves the focus back to *Name* when you already started typing in another field.

## [10.1.0] — 2026-10-01 — audit trail & session recording

First phase of the extension plan: WRM as a lightweight PAM for teams (see [ARCHITECTURE.md](ARCHITECTURE.md)).

### Added
- **Session recording** of every SSH terminal in asciinema format (asciicast v2, gzip, SHA-256 in the database). Recording is written by a background goroutine through a bounded queue, so the terminal is never slowed down. There is a size limit per session (`recording_max_mb`) and a retention period (`recording_retention_days`).
- **Replay in the browser** (play/pause, seek, 0.5–16×, skip idle time) and `.cast` download. Viewing and downloading are audited.
- **Optional keystroke recording** (`session_recording_input`, off by default). Typing at password, passphrase and PIN prompts is masked.
- **● REC** badge and a *“This session is recorded”* notice in the terminal.
- **Terminal sessions log** (`terminal_sessions`): user, IP, server, remote user, start/end, duration, status (closed, failed, connection lost, ended by an administrator, interrupted by a restart), exit code, bytes. Sessions left open by a crash are closed at the next start, and their partial recordings are kept.
- **File transfer log** (`file_transfers`): every upload, download (also editor opens and each file of a ZIP download) and server-to-server copy, with source, destination, size, SHA-256 and status.
- **Admin panel:** *Sessions & recordings* and *File transfers* tabs with filters and CSV export. The *Audit log* tab gained date range filters, recording events, links to sessions and **Verify integrity**.
- **Settings → Session history:** your own sessions and sessions on your connections.
- **Audit integrity:**
  - audit entries now reference the connection and the terminal session;
  - every entry carries a SHA-256 hash chain, checked by `GET /api/admin/audit/verify`;
  - database triggers make `audit_log`, `file_transfers` and `session_recordings` append-only (no UPDATE; no DELETE of entries younger than 7 days);
  - secret-looking keys in audit details are redacted.
- **Feature flags:** `audit_enabled` (`AUDIT_ENABLED`), `session_recording` (`SESSION_RECORDING_ENABLED`), `session_recording_input`, `recording_retention_days`, `recording_max_mb`. Environment aliases are supported for the plan's flag names.
- **API:** `GET /api/recordings`, `/api/recordings/{id}`, `/api/recordings/{id}/cast`, `GET /api/admin/transfers`, `GET /api/admin/audit/verify`, `GET /healthz`.
- **Operations:**
  - `Dockerfile` (unprivileged user, `/data` volume, health check) and `docker-compose.yml`;
  - CI builds and smoke-tests the image and publishes release images to `ghcr.io/vedranius/wrm-pro`;
  - `wrm -healthcheck`;
  - `WRM_RECORDINGS_DIR`.
- **Brand:** new WRM PRO logo for the favicon, app icons, sign-in screen, top bar, sidebar, README, release notes and GitHub social preview. A *WRM Orange* accent color. The logo kit is in `docs/brand/`.
- **Documentation:**
  - `ARCHITECTURE.md` (stack, auth, SSH layer, credentials, collaboration, migrations, extension plan status and touch points);
  - this changelog;
  - README, SECURITY and CONTRIBUTING updated.
- **Tests:**
  - an integration test with an in-process SSH + SFTP server (connect → audit entries → replayable recording without passwords; transfer checksums; access to recordings);
  - unit tests for the recorder (UTF-8 split across chunks, masking, resize, size limit) and for the append-only, hash-chained audit log.

### Changed
- Downloads by connection owners are now audited too (before, only downloads through shares were).
- Server-to-server transfer reports a read error as a failed file instead of a partial "ok".
- The audit log CSV export has new columns: connection, session and hash.

## [10.0.1] — 2026-09-28

### Fixed
- "404 page not found" at `/static/`, the v9 address of the app. It now redirects to `/`.
- Upgrading a database from an earlier build with an incompatible `audit_log` / `collab_messages` table printed `Index warning: no such column: ts / share_id`, and the audit log and chat history could not be written. Such tables are now kept as `<table>_old_<time>` and created again.

### Added
- The startup log shows the address to open in the browser.

## [10.0.0] — 2026-09-28

### Added
- **Voice calls** in collaboration rooms (WebRTC):
  - mute, deafen, push-to-talk, device selection, noise suppression, echo cancellation and gain control;
  - speaking indicators, per-person volume, connection quality;
  - a built-in TURN relay (UDP/TCP 3478) with short-lived credentials.
- **Users and roles:**
  - admin user management (create, disable, reset password/2FA, sign out everywhere, unlock, delete);
  - share access modes (members only / signed-in users / anyone with the link);
  - five roles enforced by the server (observer, viewer, operator, moderator, owner);
  - members, people history, bans, expiry, link rotation, immediate revocation.
- **Collaboration:** collaboration bar, people panel with moderation, chat history, file exchange, raise hand, several shared terminals with screen snapshot, remote keyboard control.
- **Security:**
  - TOTP 2FA with recovery codes and a 2FA policy; closed self-registration; account lockout; hashed session tokens with idle/maximum lifetime; device list;
  - a random AES-256-GCM key per installation (migrated from the v9 default key); secrets never sent to the browser; secret export only after re-entering the password;
  - SSH host key verification (trust on first use, or strict) and FTPS certificate pinning;
  - CSRF protection, CSP and security headers, request limits, local assets (no CDN), self-signed HTTPS option, trusted proxy support;
  - an audit log with CSV export.
- **Admin panel:** overview with warnings, users, shares, policies, voice & network, host keys, audit log. Policies can be forced with `WRM_<KEY>` environment variables.
- `wrm -version`, `wrm -reset-password USER [-reset-2fa]`; unit tests in CI with the race detector; `SECURITY.md`.

## [9.10.2]

### Security
- In a collaboration session any participant could give themselves keyboard control of another participant's shared terminal. Now only the sharer can grant control, keystrokes go only to that sharer, and control ends when sharing stops.

### Fixed
- The *Control* button appears next to participants while you share a terminal; late joiners see *Watch* at once; duplicate names get a suffix.

## [9.10.1]

### Added
- Saved sessions restore all windows (connection, mode, folder, position, snap, minimized, focus).
- Reconnect button on SSH windows (Ctrl+Shift+R).
- Top/bottom half snap targets, drag-to-edge snapping, snap layouts menu.

### Fixed
- Windows stay inside the screen when the zoom, resolution or panel sizes change.

## [9.10.0]

### Added
- Terminal: reconnect and automatic reconnect, connection state, flow control, SSH keepalive, search in output, clickable URLs, configurable scrollback.
- File manager: instant filter and recursive name/content search, breadcrumb path, multi-select, keyboard shortcuts, folder and drag & drop uploads with progress, built-in text editor, pooled SFTP connections.
- Connections: test connection, duplicate, FTPS. Modern dark responsive UI with an accent color; mobile drawers.
- Security: login rate limiting, WebSocket origin check, optional HTTPS.

### Fixed
- Disconnects on large outputs with multi-byte characters (binary frames).
- Full-screen programs drawn at the wrong size (the PTY size follows the window).
- Folder transfer when WRM runs on Windows; several smaller issues.

[10.7.0]: https://github.com/vedranius/web-browser-RDM-public/releases/tag/v10.7.0
[10.6.0]: https://github.com/vedranius/web-browser-RDM-public/releases/tag/v10.6.0
[10.5.0]: https://github.com/vedranius/web-browser-RDM-public/releases/tag/v10.5.0
[10.4.0]: https://github.com/vedranius/web-browser-RDM-public/releases/tag/v10.4.0
[10.3.0]: https://github.com/vedranius/web-browser-RDM-public/releases/tag/v10.3.0
[10.2.0]: https://github.com/vedranius/web-browser-RDM-public/releases?q=v10.2.0&expanded=true
[10.1.0]: https://github.com/vedranius/web-browser-RDM-public/releases?q=v10.1.0&expanded=true
[10.0.1]: https://github.com/vedranius/web-browser-RDM-public/releases?q=v10.0.1&expanded=true
[10.0.0]: https://github.com/vedranius/web-browser-RDM-public/releases?q=v10.0.0&expanded=true
[9.10.2]: https://github.com/vedranius/web-browser-RDM-public/releases?q=v9.10.2&expanded=true
