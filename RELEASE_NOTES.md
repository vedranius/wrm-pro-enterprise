## Web Remote Manager PRO v14.0.3 — egress, tunnels and host keys

**The next review batch.**

- **Server-side fetches cannot reach cloud metadata or link-local addresses** (Git, NetBox, AI providers, OIDC).
- **A loopback tunnel is private to its owner:** connections, SFTP and FTP refuse a port held by another user's tunnel.
- **Host-key auto-trust is scoped to the connection owner** in tofu mode (administrators and BMC owners included).
- **Terminal clipboard history expires after an hour.**
- **Daily database snapshots** (`backup_daily`, `backup_keep`) with retention, next to the database.

### Upgrade

Replace the executable or container and keep the database and encryption key. No schema change.

[Download v14.0.3](https://github.com/vedranius/wrm-pro-enterprise/releases/tag/v14.0.3) · [Full changelog](CHANGELOG.md)

---

## Web Remote Manager PRO v14.0.2 — egress, links and scans

**A small follow-up to v14.0.1** with the next review batch.

- **`curl`/`wget` are no longer read-only:** fetching a URL is egress and needs approval. More secret files (`.env`, `.p12`, `.npmrc`, `wp-config.php`, `terraform.tfvars`, …) are refused, also when the assistant wants to create them.
- **Guest links stay closed** when the administrator disables them; they are not widened to every signed-in account.
- **HSTS is sent behind a trusted reverse proxy.**
- **Whole-server Git scans** get a 32 MB output budget and report truncation; the deploy history no longer falls back to `/tmp`.
- **Release:** the enterprise image moved to `ghcr.io/<owner>/wrm-pro-enterprise`, and a registry failure no longer blocks publishing a release.

### Upgrade

Replace the executable or container and keep the database and encryption key. No schema change.

[Download v14.0.2](https://github.com/vedranius/wrm-pro-enterprise/releases/tag/v14.0.2) · [Full changelog](CHANGELOG.md)

---

## Web Remote Manager PRO v14.0.1 — security and reliability fixes

**The first patch release after v14.0.0.** It closes the highest-priority findings of an external review and the issues the review found in the automatic-mode and deployment paths.

- **2FA and password confirmations** now lock the account after repeated wrong codes or passwords, instead of being guessable in a loop.
- **Destructive AI commands** can no longer hide behind `sudo -u`, `..` or a here-string; automatic-mode budgets survive a new session.
- **Bundle manifests and addon archives** are validated and bounded before they reach a server or the disk.
- **SQLite foreign keys** are enforced on every pooled connection; `ensureTable` no longer sets addon instances aside on every start.
- **Host builds** are administrator-only and no longer see WRM's encryption key.
- **Faster UI:** gzip + ETag, deferred vendor scripts, and honest error messages when a delete fails.

### Upgrade

Replace the executable or container and keep the database and encryption key. No schema change is required; `mcp_tokens.auto_limits` already exists. Instances set aside by v13.9.0–v13.14.0 are restored on the first start.

[Download v14.0.1](https://github.com/vedranius/wrm-pro-enterprise/releases/tag/v14.0.1) · [Full changelog](CHANGELOG.md)

---

## WRM Pro Enterprise v14.0.0 — Apache License 2.0

**WRM is now free software under the Apache License 2.0.** This release changes the project's licence and governance, and is the first release of the **WRM Pro Enterprise** distribution.

- **Licence:** [Apache License 2.0](LICENSE) (see [LICENSE-CHANGE.md](LICENSE-CHANGE.md)). Versions up to v13.x remain under their original PolyForm terms; their tags and assets are unchanged.
- **No GPL-licensed tools in the image:** the Bitwarden CLI (GPL-3.0) is no longer bundled; operators install it at runtime ([docs/THIRD-PARTY.md](docs/THIRD-PARTY.md)).
- **Governance:** [TRADEMARK.md](TRADEMARK.md), [GOVERNANCE.md](GOVERNANCE.md), [MAINTAINERS.md](MAINTAINERS.md), [CLA.md](CLA.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) added; contributions require a CLA and a `Signed-off-by` line.
- **SPDX identifiers** and the copyright line are on every source file; releases ship a CycloneDX SBOM.
- **Predecessor:** the pre-v14.0.0 history and tags live in [`web-browser-RDM-public`](https://github.com/vedranius/web-browser-RDM-public).

[Full changelog](CHANGELOG.md)

---

## Web Remote Manager PRO v13.14.0 - Git checks that stick to Git, sorted folders and a record of every file edit

### Added

- **Sort folders A–Z** ([#134](https://github.com/vedranius/web-browser-RDM-public/issues/134)): right-click a folder to sort its subfolders by name or keep them sorted, also for all folders at once. Numbers sort naturally (`srv2` before `srv10`).
- **File edit history** ([#129](https://github.com/vedranius/web-browser-RDM-public/issues/129)): *Admin → File edits* lists every file saved in the editor or changed by the AI, with the diff, filterable by user, server, session and AI, with CSV export. Diffs are stored encrypted with secrets masked.
- **SSH certificates and opkssh** ([#119](https://github.com/vedranius/web-browser-RDM-public/issues/119)): keys log in with their OpenSSH user certificate, from a `-cert.pub` file next to the key (renewed by `opkssh login`), saved with a key in the key store, or pasted after the private key.
- **Folder filters in the Git workspace**: filter the overview, functionality, environments, *What is where* and the files of an installation by folder on the servers, as a tree with counts.
- **Logs** in the .gitignore helper's standard patterns.

### Changed

- **Git checks compare only what Git tracks.** Logs and other files outside the check are listed as *outside the check* and never compared; log files are never deployed unless the include list names them.
- Servers in the Git overview are named by hostname or connection name, never by an IP address.

### Fixed

- The Git overview mixed hostnames and IP addresses.
- The Git checker compared log files with Git.

### Upgrade

Replace the executable or container and keep the database and encryption key. Additive migration (table `file_edits`, columns `folders.auto_sort` and `ssh_keys.certificate`); a previous version still starts on the database.

[Download v13.14.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.14.0) · [Full changelog](CHANGELOG.md)

## Included from v13.13.1 - Adding the WRM PRO marketplace in the Claude app

#### Changed

- In the Claude app, add the marketplace with the repository name in lowercase: *Customize → Plugins → Add marketplace* → `vedranius/web-browser-rdm-public` → WRM PRO. The app names the marketplace after the repository, and GitHub accepts the name in any case. WRM's texts, README and the plugin's README now show it this way.
- Plugin 1.2.1 (icon path only).

#### Fixed

- *Failed to add marketplace* in the Claude app: besides the lowercase name, README (Troubleshooting) says what else to check and where the app's log keeps the real reason. *Upload plugin* with `claude-plugin-wrm-pro.zip` from the release still installs the same plugin without a marketplace.

#### Upgrade

Replace the executable or container and keep the database and encryption key. No database change; a previous version still starts on the database.

[Download v13.13.1](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.13.1) · [Full changelog](CHANGELOG.md)

## Included from v13.13.0 - The Claude plugin with one pick, and every AI app with one script

#### Added

- **WRM PRO plugin without settings**: in the Claude app (*Customize → Plugins → Add marketplace* → `vedranius/web-browser-RDM-public` → WRM PRO) or in Claude Code (`/plugin install wrm-pro --marketplace vedranius/web-browser-RDM-public`). Type `/wrm-pro:wrm`: the first time Claude asks for your WRM address and WRM opens in the browser, where you sign in and approve. The token goes straight to your computer, never into the chat.
- **`/wrm-pro:wrm`** lists your servers and everything WRM can do (logs, services, files, commands, edits, transfers), and starts the work you ask for.
- **All AI apps at once** (*Settings → AI connections*): one script for Windows or Linux/macOS signs in once and adds WRM to Claude Code, the Claude app, Codex, Gemini CLI, VS Code, Cursor, Windsurf and OpenCode, with a backup of every file it changes. When the access expires, signing in again in any app renews it for all.
- **`claude-plugin-wrm-pro.zip`** in the release, for *Customize → Plugins → Upload plugin* when adding the marketplace is not possible.

#### Changed

- Plugin 1.2.0 has no address and token settings any more (the Claude app cannot ask for them). After updating, run `/wrm-pro:wrm` once and approve the sign-in.
- The Claude Desktop extension keeps working when its token expires: Claude offers to sign in again.

#### Fixed

- The plugin could not connect from the Claude app: it depended on settings the app cannot ask for, and its web connector would have connected from the cloud, which cannot reach a WRM on your own network.
- The AI app approval dialog showed a raw key for the addon permission.

#### Upgrade

Replace the executable or container and keep the database and encryption key. No database change; a previous version still starts on the database.


## Included from v13.12.0 - Tools for writing addons, also with an AI

#### Added

- **Check an addon before installing it**: `wrm -addon-lint ./my-addon` (or *Settings → Addons → 🛠 Developer tools → Check ZIP*) lists everything the sandbox would refuse, every finding with a code, file, line and hint. A failed install opens this report by itself, and *Copy report* hands it to an AI to fix.
- **Templates**: `wrm -addon-new grafana-panel ./my-addon` (blank, fetch-and-render, grafana-panel, wasm-hook), also as ZIPs in *Templates*. They come with `wrm-bridge.js`, a small helper for talking to WRM.
- **A dev server**: `wrm -addon-dev ./my-addon` opens the panel on your computer with the real sandbox and CSP, a stand-in for WRM (settings, web requests, secrets from environment variables) and a reload on every change.
- **For AI assistants**: WRM serves the manifest's JSON Schema and an `llms.txt` guide at `/api/addons/dev/`.

#### Changed

- The addon ids `dev`, `install`, `drafts`, `community` and `users` are reserved.

#### Upgrade

Replace the executable or container and keep the database and encryption key. No database change; a previous version still starts on the database.

[Download v13.12.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.12.0) · [Full changelog](CHANGELOG.md)

## Included from v13.11.0 - Addons that reach web sites, safely

#### Added

- **Addons can show data from web sites** (for example a Grafana dashboard): the addon lists the sites, paths and methods it needs in `addon.json`, and WRM makes the request for it. The panel itself still has no network.
- **An administrator approves the sites** before anything is sent. Enabling an organisation addon approves them; a personal addon shows *sites wait for an administrator* with *Approve*, *Change* and *Withdraw* for administrators.
- **Private network only when allowed**: local, private, link-local (cloud metadata) and CGNAT addresses are refused unless the administrator ticked *private network* for that site. WRM checks every address it connects to, also after redirects.
- **Secrets** (🔑 on an addon instance): API keys and passwords for those sites, stored encrypted, per instance, with your own value in a shared instance if you need one. WRM adds them to the request; the panel never sees them, and setting or clearing one is audited without the value.
- **Recent web requests** in the window's ⚠ diagnostics (method, site, path, status, size, time, error code).
- New policy `addon_http` (`off`, `admins`, `all`) for who may use addons with web access.

#### Upgrade

Replace the executable or container and keep the database and encryption key. Additive migration (tables `addon_http_grants`, `addon_secrets`, `addon_http_log`); a previous version still starts on the database. Existing addons are unchanged.

[Download v13.11.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.11.0) · [Full changelog](CHANGELOG.md)

## Included from v13.10.0 - Saved logins, hostname names and a calmer top bar

#### Added

- **Show saved login** in *Edit connection*: after you confirm with your WRM password (a domain account with its domain password, an SSO-only account through its identity provider), WRM shows what the connection logs in with: password, private key, your vault credential, key store key, Bitwarden key or the key file path. Every reveal is audited and can raise an alert; the administrator's *Allow export of passwords & keys* policy applies.
- **Right-click a connection**: *Copy username*, *Copy password* (same confirmation, never shown in the notification) and *Copy IP address*.
- **Use the server's hostname as the name** when adding a connection, and *Rename to the server's hostname* in the menu: WRM reads the hostname over SSH, or the DNS name for other protocols.

#### Changed

- Tabs and window titles show only the connection's name; the folder path is in the tooltip.
- The vault's *Reveal* and the key store's *Export* accept a domain account's password.

#### Fixed

- The version next to the WRM logo is visible again; the empty session *— none —* no longer stands in its place.
- The user name in the top bar no longer jumps every 5 seconds when MCP is enabled.
- The Git workspace server filter no longer shows blank entries for deleted connections.

#### Upgrade

Replace the executable or container and keep the database and encryption key. No database change: installations of deleted connections are no longer listed, and deleting a connection now removes them. New API endpoints `POST /api/connections/{id}/secret` and `POST /api/connections/{id}/name-from-hostname`.

[Download v13.10.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.10.0) · [Full changelog](CHANGELOG.md)

## Included from v13.9.0 - Addon platform v2: personal addons, instances and a stricter sandbox

#### Added

- **Personal addons.** Every user can install addons that only they see (*Settings → Addons*). Administrators decide who may (`addon_personal`) and can turn off or remove any user's personal addon.
- **Themes from addons** appear in *Settings → Appearance*; each user picks one for themselves.
- **Instances**: open one addon several times (for example two dashboards), each with its own settings and window. Shared instances are set up by an administrator for everybody.
- **Settings on the server**: an addon keeps its settings in WRM (per instance), with export and import, so nothing is retyped after a reload.
- **Windows come back**: where you put an addon window, its size and whether it was open are remembered on the server, also on another device.
- **A watchdog**: a panel that does not start or stops answering shows a banner with *Reload*, *Diagnostics*, *Disable addon* and *Close*; errors the panel reports are listed under ⚠ in its title bar.
- **A permission sheet** before an addon is enabled, written by WRM from `addon.json`. An update that asks for more stays off until you approve it again.
- **Addon developer guide** ([docs/ADDONS.md](docs/ADDONS.md)) and an updated *AI addon* prompt.

#### Changed

- apiVersion 2 panels run without any network of their own and only with the scripts that came in their ZIP; every addon file is checked against the SHA-256 recorded at install.

#### Upgrade

Replace the executable or container and keep the database and encryption key. Additive migration only: new tables `addons_personal`, `addon_instances`, `addon_store` and `addon_windows`, new columns `files`, `approved` and `pending` in `addons`. Installed addons keep working; their files are pinned the next time they are installed. New policy `addon_personal` (`all`).


## Included from v13.8.0 - AI sessions, your own AI settings and approvals that do what they say

#### Added

- **AI sessions** (🧠 panel next to *Sessions*): every AI session you had in one list, whatever started it: the assistant in a terminal, the log and file assistants, OpenCode agents and AI apps such as Claude or ChatGPT over MCP. Search (titles, servers, questions and commands), filter by kind, ⭐ saved or running, open the transcript, jump to the live chat or the live view, rename, export as Markdown or JSON, and **continue** any ended session in the WRM assistant. An AI app's session continues with what the app did on the server as its earlier context. Every user sees only their own sessions.
- **Saved sessions**: ☆ keeps a session beyond the transcript retention (`ai_keep_saved_sessions`, on by default).
- **My AI settings** (*Settings → AI assistant*): your own default engine, provider, model and permission mode for new sessions, whether to show the assistant's thinking, your own API keys (when the administrator allows personal keys) and a shortcut to connect your own Claude, ChatGPT or Cursor app over MCP. These settings are per user.

#### Changed

- **An approval given after the AI stopped waiting runs at once.** When an AI app gives up waiting (its own timeout, or nobody answered), the request stays open in WRM. Approving it now runs exactly that call right away, shows the output in the live view, and hands the output to the AI when it repeats the call; nothing runs twice. Administrators can keep the old behaviour (*Admin → AI connections → Approval after the AI stopped waiting: Allow the AI to repeat it once*).
- **Deny in the AI app is final.** Choosing *Deny* in Claude's or ChatGPT's own approval question now refuses the call like Deny in WRM; only closing the question without an answer keeps it open in WRM.
- When you decide in WRM, the approval question still open in the AI app is closed.

#### Fixed

- **"The approval is no longer pending"** is replaced by what really happened: who approved or denied it and where (WRM or the AI app), or that the AI stopped waiting and the request was closed.
- An approval card in the 🤖 menu now changes when the request is kept for later (other text, no editing, which a later approval would ignore), and the approval pop-up closes once the request is decided.
- **The AI work log no longer loses its text while it fills.** The assistant's thinking used to show only its last 600 characters, so its beginning kept disappearing; the whole thought now stays and scrolls in its own line. Empty *Work log* lines above your questions are gone.

#### Upgrade

Replace the executable or container and keep the database and encryption key. Additive migration only: new column `saved` in `ai_sessions`. New settings `ai_mcp_later_approval` (`run`) and `ai_keep_saved_sessions` (on). Personal AI settings are stored in `user_ui` under the key `ai`.


## Included from v13.7.0 - Themes, accessibility and a real screen lock

#### Added

- **Settings → Appearance**: eleven themes (dark, OLED, Fjord, Solar, Forest, Ember, light Daylight, Paper, Solar light, high contrast dark and light), *follow the system light/dark*, a colour picker for every colour including the terminal, your own saved themes with export and import, fonts, interface size, spacing, corners and custom CSS. Your choice follows you to every browser.
- **Accessibility**: palettes for deuteranopia, protanopia, tritanopia and achromatopsia (also in terminals), signs and shapes so nothing relies on colour alone, more contrast, terminal minimum contrast (WCAG AA/AAA), reduced motion and transparency, a strong focus outline.
- **Themes from addons** are offered in Appearance and applied only for the users who choose them; administrators can make any theme the default.
- **Settings → Security → Screen lock**: **Lock now**, idle time with a warning, lock when WRM is hidden, when the computer locks or sleeps, on every start, a keyboard shortcut, how the lock screen looks, and notifications held until the unlock. Everything keeps running in the background.
- **Unlock PIN** and an optional sign-out after wrong attempts.

#### Changed

- **The lock is kept by the server**: a reload or a new tab stays locked, and a locked browser session cannot reach WRM data until it is unlocked (`screen_lock_mode`, default `server`).
- Addon themes are no longer applied to everyone; the accent colour is part of the theme.

#### Fixed

- Directory (LDAP) accounts can unlock the screen.
- The lock screen no longer breaks after its first use.

#### Upgrade

Replace the executable or container and keep the database and encryption key. Additive migration only: new columns `locked_at` and `lock_failures` in `auth_sessions` and `lock_pin_hash` in `users`. New settings `screen_lock_mode`, `screen_lock_max_minutes`, `screen_lock_pin`, `screen_lock_max_failures`, `appearance_default_theme` and `appearance_custom_css`. Users who used an addon theme pick it again in *Settings → Appearance*.

[Download v13.7.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.7.0) · [Full changelog](CHANGELOG.md)

## Included from v13.6.0 - Log folders, a folder tree and smarter operators

#### Added

- **Folder tree** in *Monitoring → Logs* (*View: Folder tree*): logs grouped under their main folder (`/var/log`, `/home/scripts` …) and subfolders, each folder can be opened and ticked as a whole. A **Folder** filter narrows either view to one folder and everything below it.
- **Main folders** in *⚙ Log settings*: everything below one is grouped under it (scan paths and Git workspace server roots count too).
- **Operators from repeated words**: `nova`, `NOVA2` and `novatv` in folder and service names are recognised and linked into one operator group; the filter offers the group and each operator. Guesses are marked `≈` and can be turned off in *Log settings*.
- Every log shows whether it sits in a **log folder** or an **app folder**.

#### Fixed

- Logs in `/var/log/<service>` looked missing when a script folder had the same name in a different case: the service filter now treats them as one service and shows both folders, clearly marked.

#### Upgrade

Replace the executable or container and keep the database and encryption key. No migration: the new log settings `main_folders` and `guess_operators` (on) live in the existing `log_settings` JSON.

[Download v13.6.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.6.0) · [Full changelog](CHANGELOG.md)

## Included from v13.5.0 - Log viewer, smart editor and AI that shows its work

#### Added

- **A professional log viewer** in *Monitoring → Logs*: sortable and filterable file list (operator, service, newest first), log tabs with line numbers, level colours, smart highlighting (times, IPs, URLs, HTTP codes, UUIDs, key=value), find in the log, level filters, **Follow**, older lines and go to line.
- **Search across all logs by time**: the search runs on the server with grep, takes a From/To range in the EU format (`dd.mm.yyyy hh:mm`) or a quick range, sorts hits by time and draws a timeline. A click opens the file at the hit.
- **Log settings of their own** (⚙ *Log settings*, not in the Zabbix dialog any more): scan paths, exclusions, operator/service path patterns, limits, time budget, journal, systemd, Supervisor and Docker metadata, and the discovery method. Discovery runs as one read-only SSH command and never lists scripts such as `.py` as logs.
- **AI log assistant** (🤖 in the log viewer): searches every discovered log with the new `search_logs` tool and explains what failed, since when and where. Read-only.
- **Smart file editor with an AI assistant**: highlighting, find and replace, go to line, JSON check and format, a diff of your changes before saving, a warning when the file changed on the server, and an AI panel that reviews, explains and edits the open file (every edit is a diff you approve).
- **The AI work log shows thinking, narration and tool results**, chats get a short title with date and time, and an ended chat can be **continued** with its history.
- **Date and time format**: EU by default, ISO, US or the browser's locale in *Settings*.
- **Plugin and skills**: the skills use `search_logs`; `server.json` lets the owner publish WRM to the MCP Registry.

#### Fixed

- A week-wide log search no longer ends with "time budget reached" or "results are bounded or incomplete".
- Operators under `/home/<project>/<operator>/<service>` are found again.
- AI sessions no longer hang in "stopped": an ended session says so and offers **Continue chat**.
- The version at the top left is shown again after reloading a signed-in page.

#### Upgrade

Replace the executable or container and keep the database and encryption key. Additive migration only: new table `log_settings` and new columns `title`, `purpose`, `resumed_from` and `file_path` in `ai_sessions`. Log folders configured in the Zabbix dialog are read into the new log settings. New settings `ai_show_thinking` and `ai_auto_titles` (both on).

[Download v13.5.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.5.0) · [Full changelog](CHANGELOG.md)

## Web Remote Manager PRO v13.4.4 - Claude Desktop or Claude Code, clearly

### Fixed

- **No more `/plugin` in PowerShell.** *Settings → AI connections* now says plainly:
  - **Claude Desktop app (chat)** uses the `.mcpb` extension, not `/plugin`.
  - **Claude Code (terminal or Code tab)** uses `/plugin …` **inside Claude**, or the `claude plugin install …` / `claude mcp add …` command in PowerShell (no leading slash).
  - After adding the connector, restart Claude Code or run `/mcp` to reconnect.

The quick card and the wizard carry the same note, so the “/plugin is not recognized” dead end is explained where it happens.

### Upgrade

Replace the executable or container and keep the database and encryption key. No migration.

[Download v13.4.4](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.4.4) · [Full changelog](CHANGELOG.md)

## Web Remote Manager PRO v13.4.3 - Claude in one command

### Added

- **The Claude command is now at the top of Settings → AI connections.** A quick card shows the exact command, a **Copy** button, and one-click shortcuts into the step-by-step wizard for **Claude Code**, **Claude Desktop** and **claude.ai / ChatGPT**. The admin AI connections view links to the same wizard, and the README has the quick start.

```sh
claude mcp add --transport http --scope user wrm "https://<your-wrm>/mcp" \
  --header "Authorization: Bearer <token>"
```

- The wizard still creates the token, fills it into the command, and tests the connection.

### Upgrade

Replace the executable or container and keep the database and encryption key. No migration.

[Download v13.4.3](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.4.3) · [Full changelog](CHANGELOG.md)

## Web Remote Manager PRO v13.4.2 - The version and the lock button tell the truth

### Fixed

- **The version label matches the running binary.** The title bar and the sign-in card no longer show a hard-coded `v12.8.1`; they are filled from the server (`/api/auth/config`, `/api/version`), so an updated server shows its real version everywhere.
- **The lock button works.** It is now a fixed top-bar button with its own handler (🔒 **Lock**), not one inserted next to Sign Out at runtime. Clicking it opens the password lock screen; sessions and AI runs keep going.

### Upgrade

Replace the executable or container and keep the database and encryption key. No migration.

[Download v13.4.2](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.4.2) · [Full changelog](CHANGELOG.md)

## Web Remote Manager PRO v13.4.1 - OpenCode Go on existing installations

### Fixed

- If `ai_provider_kinds` was saved with the old "all provider kinds" list, WRM now adds `opencode_go` automatically, so **OpenCode Go (subscription key)** appears in the provider form without editing the policy. A deliberately narrowed list is not changed.

### Upgrade

Replace the executable or container and keep the database and encryption key. No migration.

[Download v13.4.1](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.4.1) · [Full changelog](CHANGELOG.md)

## Web Remote Manager PRO v13.4.0 - OpenCode Go keys work

### Added

- **OpenCode Go provider.** The Go subscription is separate from OpenCode Zen: a Go key is only accepted by Go's endpoints, and Zen answers *insufficient funds* when it receives one. Choose **OpenCode Go (subscription key)** as the provider, paste the key from `opencode.ai/console/go`, and WRM sends requests to `https://opencode.ai/inference/go/...` with the right API per model (Chat Completions or Anthropic Messages), a stable `x-opencode-session` and a `WRM-PRO` user agent. The Go model list is loaded from Go (with the documented list as fallback); models WRM cannot speak (OpenAI Responses, Google Gemini) are left out. The OpenCode agent engine can use the same provider as `opencode-go/<model>`.

### Fixed

- A Go subscription key is no longer used against the Zen endpoint, so it no longer produces *insufficient funds*; a missing key is reported clearly.

### Upgrade

Replace the executable or container and keep the database and encryption key. No migration.

[Download v13.4.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.4.0) · [Full changelog](CHANGELOG.md)

## Web Remote Manager PRO v13.3.0 - The AI writes addons, you review them

### Added

- **AI addon loop.** The assistant drafts an addon in its own workspace (`<DB>.addons/.drafts/<id>/`) instead of the live addon folder. New tools — `addon_list`, `addon_files`, `addon_create`, `addon_write`, `addon_enable`, `addon_disable`, `addon_delete`, `addon_run` and `addon_install_community` — are covered by the new **`addons.manage`** scope and are available to administrators only. A draft is validated exactly like a ZIP, including the WebAssembly compile check, and installing a new draft always leaves it disabled.
- **Drafts in Settings → Addons.** Review a draft with **Diff** (a unified diff against the installed version), then **Install draft** or **Discard**. Nothing in the workspace is served or executed until you install it.
- **Approvals you cannot miss.** A pending approval is recorded in your notifications (`ai.approval`, also for email/Telegram/…), pops up as a fixed card even when the AI panel is closed, and plays a short sound. The assistant chat keeps a sticky **Approval needed** bar with **Approve / Deny**, and every reply plays a short sound; the speaker button turns sound off. The global check runs every five seconds and covers the built-in assistant and OpenCode, not only MCP apps.
- **`file_path` works as `path`.** A model that used the name from other tools no longer gets its edit refused before you see an approval.

### Security

- Addon changes always ask for your approval, including in automatic mode and with full access; a draft never enables itself.
- A non-administrator cannot use the addon tools, cannot create an AI connection with `addons.manage`, and is not offered the scope. A broken draft write leaves the previous draft untouched.

### Upgrade

Replace the executable or container and keep the database and encryption key. No migration.

[Download v13.3.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.3.0) · [Full changelog](CHANGELOG.md)

## Web Remote Manager PRO v13.2.0 - Screen lock

### Added

- **Screen lock (#115).** After 2, 5, 10, 30 or 60 minutes without activity (or off) the interface is covered by a lock screen that asks for the signed-in account's password; the **🔒** button next to Sign out locks it immediately. Terminals, tunnels and AI runs keep going, every unlock attempt is audited (`auth.unlocked` / `auth.unlock_failed`, five per minute), and the note on the screen says plainly that this is a presence check, not a new sign-in.

### Upgrade

Replace the executable or container and keep the database and encryption key. No migration.

[Download v13.2.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.2.0) · [Full changelog](CHANGELOG.md)

## Web Remote Manager PRO v13.1.2 - Three-level service discovery and a clean history

### Changed

- **Service labels under `/home`.** Discovery descends one container level under `/home`, so `/home/<project>/<operator>/<service>` is recognised and labelled operator/service without naming any project. Those labels are defaults: a root line in **Monitoring settings** (`path | service | operator`) overrides them.
- Documentation, tests, release notes and the community repository now use neutral placeholders only; the public branch history was rewritten so no private deployment name remains.

### Upgrade

Replace the executable or container and keep the database and encryption key. No migration.

[Download v13.1.2](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.1.2) · [Full changelog](CHANGELOG.md)



## Web Remote Manager PRO v13.1.1 - Claude Code in one command, updater assets fixed

### Added

- **One-command Claude Code setup.** The plugin ships `/wrm-pro:connect` and `scripts/connect-claude.sh` / `.ps1`, which run `claude mcp add --transport http --scope user wrm <url> --header "Authorization: Bearer <token>"`. In the chat: `/plugin marketplace add vedranius/web-browser-RDM-public` and `/plugin install wrm-pro@wrm-pro`.
- **Release assets match the updater:** `wrm-pro-v<version>-<os>-<arch>[.exe]` and `SHA256SUMS.txt` for Windows, Linux (amd64, arm64, 386) and macOS (amd64, arm64). The previous upload without the `wrm-pro-` prefix made the in-app update report that the release has no file for the platform.

### Changed

- Log discovery walks `/home` with the same bounded operator/service depth instead of one hardcoded home directory; docs use `/home/<operator>`.
- The generated version file lists deployments under `## Services`.

### Upgrade

Replace the executable or container and keep the database and encryption key. No migration.

[Download v13.1.1](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.1.1) · [Full changelog](CHANGELOG.md)
## Web Remote Manager PRO v13.1.0 - Logs the assistant can read, addons you can install, monitoring you can watch

### Logs and the assistant

- **Find logs is an explicit action.** Monitoring Ôćĺ Logs never scans a server by itself. Select a server, click **Find logs**, and the result is cached for an hour.
- **A service inventory, not a folder list.** Discovery reports systemd units, Docker containers, Supervisor programs, Git installs and plain folders, including services whose output only goes to journald, and application folders under a broad root such as `/var/log/media-api` with `error.log` and `info`.
- **The assistant reads the same inventory.** `find_logs` and `read_log` go through the existing approval, audit, redaction and size-limit path, over the session's own SSH connection.

### Addons

- **Install from a ZIP in Settings Ôćĺ Addons.** The archive is checked before anything is written: manifest, safe entry names, declared files, capabilities. Panels open in a sandboxed `iframe` with no same-origin; themes are a stylesheet that disappears when the addon is disabled.
- **Server code runs as WebAssembly** (`wazero`, no CGO, no WASI): no files, no network, no environment, 64 MiB and five seconds per call. Only an administrator may call it, and the installer compiles the module and checks its exports and imports before install.
- **Community library.** The index of *WRM Pro - community public addons* is listed with trust levels (`official`, `reviewed`, `community`, `unverified`). Downloads must come from a GitHub host over HTTPS and match the SHA-256 in the index, then pass the same checks as an uploaded ZIP.
- **AI addon.** The pane builds a complete prompt from the capabilities this build supports and hands it to your AI client or the built-in assistant.

### Monitoring and replay

- **Watch services, not only metrics.** A server can watch up to 20 systemd units or Docker containers on the same SSH poll; a service that is not `active`/`running` turns the sample into `problem` and uses the existing notifications. **Poll automatically** is a switch on the server card.
- **Replay shows wall-clock time.** The player puts the exact time at the playhead next to the offset and the header gains **Ended**; old recordings show absolute times too.

### Accounts and the assistant

- **Recording level per account.** When creating a user, choose *As the server settings*, *Off for this user*, *Session output only* or *Output and keystrokes*. The server setting stays the master switch.
- **Work log in the assistant chat.** Each request gets a collapsible work log that names every tool step while the model works and collapses to a step count when the answer arrives.

### Fixed

- **Ollama:** a turn that only called tools failed with `HTTP 400: invalid message content type: <nil>`; the request builder wrote `"content": null` for an assistant message without text. It now always sends a string.

### Security

- Journal access and log discovery use the existing SSH account and file-read permission; discovery is owner-only and audited (`monitor.logs_discovered`). The addon surface is a capability allowlist, and a capability this build does not implement refuses the whole install. Panel HTML is served with `Content-Security-Policy: sandbox allow-scripts` and same-origin framing only.

### Upgrade

Replace the executable or container and keep the database and encryption key. The database is upgraded in place (`users.recording_level` is added, empty means "follow the server settings"). No manual step is required.

[Download v13.1.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v13.1.0) ┬Ě [Full changelog](CHANGELOG.md)

## Web Remote Manager PRO v12.8.1 - Security update: Go 1.27.3 and a reviewed gosec finding

### Security

- **Go 1.27.3 fixes GO-2026-6809 / CVE-2026-111943.** In Go 1.27.2, the `crypto/x509` certificate verifier could still trust a chain built on a known compromised intermediate certificate in environments whose system trust store had not removed it. Go 1.27.3 (and 1.26.10) adds a denylist and makes `Certificate.Verify` return an error for those intermediates. The advisory was published by the Go security team on 2026-10-21; it is not a WRM-specific vulnerability. WRM reaches the API in `nettools.go` when it verifies an HTTP check's certificate chain, so this release moves the build toolchain to `go1.27.3`.
- **gosec G702 in the OpenCode sandbox is a reviewed false positive.** gosec follows the operator-set `WRM_MEDIUM_DOCKER` value from `mediumDockerCLI()` as tainted input. The flagged `docker inspect` call passes the internally generated `wrm-opencode-<session>-<random>` container name as a separate argument, without a shell. The call arrived with the OpenCode Docker sandbox in v12.6.19 (commit `9c102ae`, merged in PR #82); the taint finding was not in the reviewed baseline. It is now accepted there with the reason, alongside the existing G204 entry for the same line.

### Upgrade

Replace the executable or container and keep the database and encryption key. No database migration is required. This release only updates the Go runtime and the reviewed security baseline.

[Download v12.8.1](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.8.1) · [Full changelog](CHANGELOG.md)

## Web Remote Manager PRO v12.8.0 - Folder management, permissions, upload queue and the reported UI fixes

### Added

- **Folders delete recursively.** Deleting a folder now deletes its subfolders and every connection inside it, with the tunnels, snippets, bookmarks and shares that belong to them. Nothing is moved to the root. The confirmation shows the exact folder and connection counts before you confirm.
- **Folders and connections move where you put them.** Use **Move to folder…** in the context menu, or drag a folder or one/more selected connections. The new position is saved, and invalid moves (a folder into itself or its descendant) are refused.
- **File permissions from the file manager.** Change a file or folder to a three- or four-digit octal mode. For folders you can apply it recursively. If the SSH account is not allowed, WRM asks for an optional sudo password and retries. FTP connections get a clear explanation instead of a silent failure.
- **Upload queue.** Drop more files while an upload is running; they are queued and start automatically. Each batch remembers the folder it was dropped into.
- **Tunnel search and proxy shortcuts.** Filter tunnels by port, description, host or route, and switch to active tunnels only. Proxy settings are now reachable from **Tools** and from the Tunnels panel.
- **Edit and Tunnels in the terminal title bar.** Owned SSH and SFTP windows can open connection editing or their tunnels without going back to the sidebar.
- **Scroll arrows for overflowing tabs.** Settings, Admin and folder tab rows get left/right arrows when they do not fit. The Settings window now uses the Admin window size.

### Fixed

- Nested shared and collaboration folders keep their hierarchy instead of being flattened into the root.
- Connection names sort case-insensitively. Folder names compare case-insensitively in dropdowns and paths, and folder order stays manual.
- **Save Connection** and **Save Folder** show progress and cannot be submitted twice by a double click.
- **+ Connection** on a folder is pre-filled with that folder and inherits its jump-host and proxy defaults.
- The folder delete confirmation now describes recursive deletion instead of promising that connections move to the root.

### Upgrade

Replace the executable or container and keep the database and encryption key. No database migration is required. Recursive deletion is intentional and permanent: review the counts in the confirmation before deleting a folder.

[Download v12.8.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.8.0) · [Full changelog](CHANGELOG.md)

## Web Remote Manager PRO v12.7.4 - Remote desktop without the manual guacd setup, previews for every text file

### Added

- **Windows and macOS get guacd automatically.** If guacd does not answer and `guacd_address` is local (`127.0.0.1:4822`), WRM starts the official `guacamole/guacd:1.5.5` container in Docker Desktop. The container is named `wrm-guacd`, listens on `127.0.0.1:4822` only, and restarts with Docker (`unless-stopped`). The first RDP/VNC/Telnet connection starts it; `-setup-guacd` does the same from the command line.
- Policy **Start guacd automatically** (`guacd_auto`, default on) and a **Start guacd** button in *Admin → Overview* with the current state. A custom guacd on another host is never touched.
- Clear messages in the desktop window when Docker Desktop is missing, stopped or set to Windows containers (it must use Linux containers).
- **Every textual file opens in Preview, recognized by content.** Files are no longer limited to a short extension allowlist: scripts, configuration files, logs and files without an extension open as text when their bytes are textual. UTF-8, UTF-16 (with a BOM or a detected byte order) and single-byte text are decoded; binary files still get a clear “download it to open it” answer. A bundled PrismJS 1.29.0 adds offline syntax highlighting for XML/SMIL/SVG, JSON, YAML, TOML, INI, shell, Python, JavaScript/TypeScript, Go, SQL, CSS/SCSS/LESS, Markdown, PowerShell, batch, Dockerfile, Makefile, C/C++/C#/Java/Rust/Ruby/PHP and similar files. The 256 KiB limit, truncation notice and escaped-text safety policy are unchanged.

### Fixed

- **RDP on Windows no longer stops at “Remote desktop needs guacd … but it is not reachable”.** There is no manual `docker run` or Linux guacd machine to prepare.

### Upgrade

Replace the executable/container and keep the database and encryption key. No database change. On Windows/macOS install or start Docker Desktop; the first desktop connection (or *Admin → Overview → Start guacd*) prepares guacd. Linux keeps using the system guacd or the `docker-compose.yml` service. The syntax highlighter is bundled in the binary; no CDN or network access is needed.

[Download v12.7.4](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.7.4) · [Remote desktop in the README](https://github.com/vedranius/web-browser-RDM-public/blob/v12.7.4/README.md#remote-desktop-rdp-vnc-telnet)

## Web Remote Manager PRO v12.7.3 - Medium build locations and complete service logs

### Added

- **Choose where a Medium build runs.** A project with a `build` step can now deploy natively. For each project choose the **WRM host**, a **Linux build server over SSH**, or **no build**. The choice is saved per user and project and audited.
- The whole commit is prepared, the `build` command runs in the repository root, and `source_dir` is read from the result. A build server receives only the prepared source in a temporary directory (no Git metadata and no SSH keys); the build runs through the account's login shell and the directory is removed afterwards.
- The plan shows the build location, its platform and architecture, and the last build output. A destination with a different platform or architecture is refused with the reason. WRM-host builds need a POSIX host with the toolchain; on Windows choose a Linux build server or no build.

### Fixed

- **Monitoring → Logs now discovers every service under `/home/<operator>/<operator>/<service>`.** Service and Operator labels are correct, virtual environments and build/archive directories no longer hide services, and service `log`/`logs` folders include their log files, including FTP `.txt` logs, while compressed archives are skipped. Larger time budgets keep large hosts complete.

## Web Remote Manager PRO v12.7.2 — Version detected from the code

### Added

- **The deployed version is read from the code.** The Git workspace compares every tracked file on the server with the repository tree at the target and at the earlier tags instead of trusting `VERSION.md`. All files matching one version show that version; a consistent older version is named exactly from its commit tree; a mix of versions or a hand-edited file shows **Mixed / modified** with the per-file details.
- **Optional `VERSION.md` check.** A checkbox next to *Check now* (off by default) also reads `VERSION.md` and warns when it disagrees with the code. With it on, the commits list follows `VERSION.md`; with it off, the list follows the detected commit.

### Changed

- The installs table, the details dialog and the Simple view show the detected version and commit. The per-environment behind/drift comparison uses the detected commit and falls back to the deploy record only when the code cannot be identified.
- Deployments and updates still write `VERSION.md` for compatibility with the original tool. Older stored targets and offline bundles without version trees use the per-file history and are marked *approx.* when no full tree is available.

### Upgrade

Replace the executable/container and keep the database and encryption key. No database change. The first check after the upgrade may need a target refresh to load the older version trees; until then the per-file history is used.

[Download v12.7.2](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.7.2) · [Git workspace: every file and partial checks](https://github.com/vedranius/web-browser-RDM-public/blob/v12.7.2/README.md#git-workspace-every-file-and-partial-checks)

### Included from v12.7.1 — Medium environment server lists

#### Fixed

- **Medium: per-environment `servers` lists.** `environments.yaml` now accepts the original deploy tool's `servers: [name, ...]` key. A listed server must exist in `servers.yaml` exactly once; an empty list is refused; and a project target in that environment may only deploy to a listed server. Environments without the key keep every server, as before.

#### Upgrade

Replace the executable/container and keep the database and encryption key. No database change. Existing `deploy-config` files keep working; add `servers` only where an environment should be limited.

[Download v12.7.1](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.7.1) · [Medium native engine](https://github.com/vedranius/web-browser-RDM-public/blob/v12.7.1/README.md#medium-native-engine)

### Included from v12.7.0 — Medium deploys natively

#### Added

- **WRM deploys Medium projects itself:** switch a project to the *WRM native* engine and WRM deploys it from your WRM Git source to every server of the environment through your WRM connections. No Docker and no original tool are needed for it. Each server gets a lock, a backup of everything that changes, a checksum check of every uploaded file, `post_deploy` and a history entry.
- **Same server state as the original tool:** the lock, `current.json`, `history.jsonl` and the backups are written exactly as the original tool writes them, so you can switch a project between the two engines at any time and each continues after the other, rollbacks included.
- **Dry run:** takes the lock, compares again and stops before changing anything.
- **Leave changes out:** tick changes in the plan to keep them as they are on the server for this deployment.
- **Safe by design:** WRM compares again under the lock and stops if anything changed since you reviewed the plan; production environments still ask you to type their name; deploying, rolling back and releasing a lock ask for step-up confirmation.
- **History and rollback:** see what is on each server, who deployed what and when, and roll back the last deployment (or try a dry rollback first). A lock someone left behind can be released once it is old enough.
- **Live progress:** a log with every step and command output, the result per server, and *Cancel* before the next server starts changing.

#### Changed

- Projects with a build step are still deployed by the original tool until build locations arrive in v12.7.3.

#### Upgrade

Replace the executable/container and keep the database and encryption key. The database gets one new table (`medium_project_engine`); a previous version still starts on it. Every project stays on the original tool until someone allowed by the new policy `medium_native` (default: administrators) switches it.

[Download v12.7.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.7.0) · [Medium native engine](https://github.com/vedranius/web-browser-RDM-public/blob/v12.7.0/README.md#medium-native-engine)

### Included from v12.6.26 — SIEM export, alerts and lockdown

#### Added

- **Send the audit log to your SIEM:** syslog over TLS (RFC 5424), a JSON lines file, Splunk HEC, Elastic, Microsoft Sentinel or any HTTPS collector. Every event carries the user, role, directory identity, source address and the audit hash chain. Nothing is lost while a destination is down: WRM keeps its place and catches up, and *Admin panel → Security* shows for each destination whether it delivers, how much is waiting, the last error and the next try.
- **Security alerts:** repeated failed sign-ins, a break-glass sign-in, a new administrator, a revealed or exported secret, an AI command approved under full access, and audit or SIEM export turned off. Administrators get them in the inbox and by notification, and every alert goes to the SIEM. Each rule can be changed or turned off in *Policies → Security alerts*.
- **Stop user:** one click disables an account and ends everything it has open (sessions, terminals, shared sessions, tunnels, AI sessions and AI connections), then shows what was ended.
- **Lockdown:** in an incident only break-glass administrators can sign in; everyone else is signed out at once, AI is switched off and shares are paused until the lockdown ends. Turn it on in *Admin panel → Security*, on the WRM host with `wrm -lockdown on`, or with the `WRM_LOCKDOWN=1` environment variable. The sign-in page shows the lockdown and its reason.

#### Changed

- Security alerts and lockdown events are always recorded, even with the audit log switched off.

#### Upgrade

Replace the executable/container and keep the database and encryption key. The database gets one new table (`siem_destinations`); a previous version still starts on it (it does not enforce a lockdown). Nothing is exported until an administrator adds a SIEM destination. The alert rules are on by default and only notify administrators.

[Download v12.6.26](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.26) · [SIEM export, alerts and lockdown](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.26/README.md#siem-export-alerts-and-lockdown)

### Included from v12.6.25 — Active Directory, LDAP and SCIM

#### Added

- **Sign in with Active Directory or LDAP:** people use their directory name and password on the normal sign-in page (`name`, `DOMAIN\name` or `name@domain`). WRM checks the password with the directory and never stores it. The connection is always encrypted (LDAPS or StartTLS). Nested AD groups, OpenLDAP, 389 Directory Server and FreeIPA are supported.
- **Who gets in, decided by directory groups:** rules turn groups (nested ones included) or attributes into a WRM role and teams. Accounts are created on the first sign-in only for people a rule matches; disabled, locked and expired accounts are refused with a clear message.
- **Access ends when the directory says so:** WRM checks every linked account regularly (every 15 minutes by default, or with *Check now*). Someone deleted, disabled, locked or removed from the WRM groups loses access at once.
- **SCIM provisioning:** Microsoft Entra ID, Okta and other identity providers create, change, disable and delete WRM accounts and groups immediately. Groups become WRM teams, and each provisioning client has its own token that can be rotated or revoked.
- **Stopping someone stops everything:** disabling an account in WRM, in the directory or through SCIM ends their sign-in sessions, terminals, tunnels, collaboration rooms, AI sessions and AI connections.
- **Set up step by step:** *Test connection* checks the address, encryption, certificate, service account, base DN, users and groups and says what to fix. *Test sign-in* shows what WRM would do for a person, without creating anything.
- **Secrets stay with their server:** changing a directory's address or a provider's issuer asks for the password or client secret again, so a stored secret is never sent to another server.

#### Fixed

- **Automatic releases:** the release job never started after a merge, so v12.6.24 was not published on its own. This release includes everything from v12.6.24 and is the first signed, automatically published release.

#### Upgrade

Replace the executable/container and keep the database and encryption key. The database gets six new tables (`ldap_directories`, `ldap_identities`, `scim_clients`, `scim_users`, `scim_groups`, `scim_group_members`); a previous version still starts on it. Nothing changes until an administrator adds a directory or a provisioning client.

[LDAP / Active Directory and SCIM](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.26/README.md#ldap--active-directory-and-scim)

### Included from v12.6.24 — Sign in with your organisation's account

#### Added

- **Directory sign-in:** people sign in with Microsoft Entra ID, ADFS, Okta, Keycloak, Google Workspace or any OpenID Connect provider. *Admin panel → Directory sign-in* adds the provider with setup hints for each kind and checks the connection step by step, saying what to fix when a step fails.
- **Who gets in, decided in the directory:** rules turn Entra app roles, group IDs or other claims into a WRM role and teams. Accounts are created on the first sign-in only for people a rule matches; everyone else is refused with a clear message. People are recognised by their immutable directory ID, never by name or e-mail.
- **Access ends when the directory says so:** WRM checks regularly that each signed-in person may still sign in. Someone disabled, deleted or removed from the WRM group loses their sessions and terminals, and role or team changes in the directory apply at the next sign-in.
- **MFA from the directory:** a provider can be required to prove a second factor in its token (for Entra through Conditional Access authentication contexts).
- **Test sign-in:** see exactly what the provider sends and what WRM would do with it, without creating anything.
- **Password sign-in for emergencies only:** optionally only break-glass administrators keep password sign-in (required by the Enterprise level once a directory sign-in is on).
- **Confirm with the directory:** accounts without a password confirm risky actions (step-up) by signing in at the provider again. Existing accounts can link their directory account in *Settings → Security*.
- **Signed, automatic releases:** releases are now published automatically when a new version is merged, and signed with the project's key; the one-click update refuses a release without a valid signature.

#### Upgrade

Replace the executable/container and keep the database and encryption key. The database gets two new tables (`oidc_providers`, `user_identities`) and three columns; a previous version still starts on it. Nothing changes until an administrator adds an identity provider.

v12.6.24 was not released on its own (see *Fixed* above); it ships in v12.6.25. · [Directory sign-in (OIDC)](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.26/README.md#directory-sign-in-oidc)

### Included from v12.6.23 — Roles, teams and step-up

#### Added

- **Roles:** *Administrator*, *Security auditor*, *Operator* and *User*. A security auditor reads the audit log, terminal sessions, recordings, file transfers and the security posture and changes nothing; auditors cannot open connections or use the AI assistant. Operators pass policies set to *operators*.
- **Teams:** *Admin panel → Teams* with members and team leads. Add a whole team to a share with one role; people who join the team later get it too, and leaving the team closes what they opened through the share.
- **Policies by role or team:** every *who may use it* policy (tunnels, serial ports, network tools, proxies, Git actions, Medium, Bitwarden, AI assistant, OpenCode, AI connections and more) now accepts *operators* and chosen teams besides *off / admins / all*.
- **Confirm risky actions (step-up):** with *Step-up confirmation* on (required from the Business level), changing policies, roles or the security level, deleting or disabling users, resetting passwords or 2FA and revealing stored secrets ask for your password (and 2FA code) again. One confirmation lasts 10 minutes by default; signing in counts as one.
- **Break-glass administrator:** an emergency administrator account that must use 2FA, can be limited to chosen networks, and whose every sign-in is audited and sent to the other administrators.
- **Handover when a user leaves:** deleting a user can hand their connections, folders, credentials, keys, shares and proxies to their team lead or another account, so the team keeps its access.

#### Upgrade

Replace the executable/container and keep the database and encryption key. The database gets three new tables (`teams`, `team_members`, `share_teams`) and three columns; a previous version still starts on it. Existing administrators stay administrators and everyone else becomes a *User*; nothing changes until an administrator assigns roles or teams. With step-up on, scripts that change settings or users need `POST /api/auth/step-up` first.

[Download v12.6.23](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.23) · [Roles, teams, step-up and break-glass](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.23/README.md#roles-teams-step-up-and-break-glass)

### Included from v12.6.22 — Security levels

#### Added

- **Choose how strict WRM is:** *Home* (as before), *Business* or *Enterprise*. The level sets minimums for HTTPS, two-factor authentication, password length, session lifetime, failed sign-ins, registration, host keys, secret export, guest links, AI full access, audit and session recording. A higher level raises weaker settings and never loosens stronger ones.
- **At install or later:** the first administrator picks the level on the first-run screen; `wrm -security-level business` (also with `-install-service`) or `WRM_INITIAL_SECURITY_LEVEL` sets it at install, `WRM_SECURITY_LEVEL` forces it. Later it changes in **Admin panel → Security**.
- **See before you switch:** the preview lists every setting that changes, what blocks the change (for example HTTPS while you use plain HTTP), and whom it affects: users who must set up 2FA, sessions that end, guest links that become too long. You can keep a setting as a **deviation** with a written reason, and sign out every other session at once.
- **Deviations are on record:** loosening a policy below the level asks why. Who recorded each deviation, when and why is listed and audited, and it disappears once the setting meets the level again.
- **Security posture page:** a checklist of this installation against the level (HTTPS, every level rule, administrators and users without 2FA, where the encryption key lives, open origins, guest links without expiry, signed updates, the current version, the audit hash chain), each with the explanation and the exact fix. Export it as JSON or print it to PDF for an auditor.
- **New policies:** *HTTPS required for sign-in* (`require_https`, localhost always allowed) and *Longest guest link* (`link_share_max_days`).

#### Upgrade

Replace the executable/container and keep the database and encryption key. The database gets one new table (`security_deviations`); a previous version still starts on it. Existing installations stay at *Home* with all their settings unchanged until an administrator picks another level.

[Download v12.6.22](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.22) · [Security levels](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.22/SECURITY.md#security-levels)

### Included from v12.6.21 — Signed releases

#### Added

- **Signed releases:** every release carries `SHA256SUMS.txt.sig`, an ed25519 signature of the checksums file, plus a software bill of materials (CycloneDX) and GitHub build provenance. You can check where a file comes from and that nobody changed it.
- **Updates check the signature:** once the project's public key is built in, the one-click update refuses a release that is unsigned or wrongly signed. The update dialog says whether the signature is checked. Administrators of their own fork add its key in *Release signing keys* (`update_signing_keys`).
- **Verify a download yourself:** put `SHA256SUMS.txt` and `SHA256SUMS.txt.sig` next to the file and run `wrm-pro-… -release-verify SHA256SUMS.txt`.
- **Nightly security tests:** fuzz tests feed random input to the import parsers (mRemoteNG, SSH config, CSV/XLSX, PuTTY, WinSCP, configuration files, deploy-config YAML), and OWASP ZAP scans a running WRM signed out and as an administrator.

#### Fixed

- A damaged PuTTY `.reg` file could make the import fail with an internal error instead of a clear message. The fuzz tests found it.

#### Upgrade

Replace the executable/container and keep the database and encryption key. No database change. This version has no built-in release key yet, so updates still check the SHA-256 only; the next version with the key turns the signature check on.

[Download v12.6.21](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.21) · [Signed releases](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.21/SECURITY.md#signed-releases)

### Included from v12.6.20 — Security checked on every change

#### Added

- **Automatic security checks on every change** to WRM, before it reaches a release:
  - known vulnerabilities in Go and in the libraries WRM uses, limited to code WRM actually calls (govulncheck);
  - GitHub **CodeQL** code scanning of the server and the web UI;
  - Go security patterns (gosec) and bug checks (staticcheck); every existing gosec finding was reviewed, and a new one stops the change until it is fixed or explained;
  - secrets such as tokens and private keys in commits (gitleaks);
  - the build workflows themselves (actionlint, zizmor), a review of new dependencies, a vulnerability scan of the Docker image (Trivy) and the OpenSSF Scorecard.
- **Claude security review** of pull requests with WRM's own checklist (who could attack, what they could reach), and the same review in every development session before a push.
- **Dependabot** proposes updates of Go libraries, GitHub Actions and the Docker base images every week.

#### Changed

- WRM is built with **Go 1.27** (Go 1.23, used until now, no longer receives security fixes), and all libraries are updated, among them `golang.org/x/crypto`, `golang.org/x/sys`, the SQLite driver, SFTP and FTP.
- The build workflows run with read-only tokens unless a job publishes release files, every action is pinned to an exact commit, and checkouts no longer keep credentials.
- The Docker image no longer contains npm, corepack and yarn, which are only needed while it is built.
- The Medium proxy uses Go's safer `Rewrite` hook, so a request can no longer drop the headers WRM adds by naming them in its `Connection` header.

#### Upgrade

Replace the executable/container and keep the database and encryption key. No database or setting change. Building from source needs Go 1.26 or newer. What comes next for security (signed releases, security levels for home, business and enterprise, roles and teams, sign-in with Microsoft Entra ID / Active Directory, LDAP and SCIM, SIEM export) is in the [roadmap](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.20/ROADMAP.md).

[Download v12.6.20](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.20) · [Continuous security checks](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.20/SECURITY.md#continuous-security-checks)

### Included from v12.6.19 — OpenCode with its own tools, in a sandbox

#### Added

- **Both ways to run OpenCode:** next to the contained OpenCode agent (its own tools off, everything through WRM), an administrator can now allow **🐳 OpenCode's own tools, in a Docker sandbox** (`ai_opencode_sandbox`: off by default, administrators only, or everyone). The agent then gets its own shell, file and web tools, but only inside a container on the WRM host. Your servers are still reached only through WRM.
- **Every use asks you:** each shell command, file change and web fetch in the sandbox appears as an approval card with the exact command, diff or address. You approve it once or refuse it, with a note if you like; it cannot be edited and is never approved automatically, not even with full access.
- **A locked-down container:** no capabilities, no privilege escalation, read-only system, memory, CPU and process limits, only the session's own folder mounted, the port open on `127.0.0.1` only. It is removed when the session ends or WRM stops. The image is your choice (`ai_opencode_sandbox_image`, default `debian:stable-slim`).
- **Set up with the same one click:** *Download and install* also prepares the sandbox (the Linux build of OpenCode for Docker, the image, a test run). The admin page shows whether the sandbox is ready and what is missing.

#### Upgrade

Replace the executable/container and keep the database and encryption key. No database change. Nothing changes until an administrator sets `ai_opencode_sandbox` and presses *Download and install* again under *Admin panel → AI assistant → OpenCode agents*. The sandbox needs Docker on the WRM host (Medium's step 2 can install it) and WRM itself not running in a container.

[Download v12.6.19](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.19) · [OpenCode in WRM](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.19/README.md#opencode-zen-agents-and-helper-models)

### Included from v12.6.11 — OpenCode inside WRM

#### Added

- **OpenCode Zen, free models without a key:** add the **OpenCode Zen** provider (a preset in the wizard) and use OpenCode's free models in the AI assistant without an account. With an OpenCode account, paste its Zen API key (org-wide by an administrator, or for yourself under *My API keys*) and every model of the account is available. Free models are marked in the model lists.
- **OpenCode agents in the assistant:** choose **OpenCode agent** as the engine and its **build** or **plan** agent. WRM downloads OpenCode itself (an administrator presses *Download and install* once; the file is checked against the registry's sha512) and runs it on the WRM host, one private process per session. OpenCode's own shell, file and web tools are switched off: it works on your server **only through WRM**, so every command and edit shows up in the chat and waits for your approval exactly like the WRM assistant (unless full access is on).
- **Helper models, picked automatically:** the assistant can hand a self-contained text job (summarise a long log, explain an error, draft a snippet) to a cheaper or free helper model. WRM chooses the model by the kind of job, free ones first, and tries the next one if a model fails. Each delegation is a card in the chat with the model, size, tokens, cost and how that model treats your data.
- **Clear data-use labels:** 🛡 zero retention, ⚠ may use prompts for training, ❔ unknown terms, 💳 paid. Models that may train on prompts are **off by default**; each user can switch them all on for themselves in the assistant (administrators can also allow them for everyone or never).
- **OpenCode on your computer** can connect to WRM too: the MCP wizard has an OpenCode entry with the `opencode.json` snippet or the OAuth login.

#### Fixed

- Stopping WRM also stops every running OpenCode agent; after a crash WRM cleans up what was left on its next start.

#### Upgrade

Replace the executable/container and keep the database and encryption key. The database gets one new column (`users.ai_delegate_training`); a previous version still starts on it. Nothing changes until an administrator adds an OpenCode Zen provider, turns on delegation (`ai_delegate`) or installs OpenCode (*Admin panel → AI assistant → OpenCode agents*). New settings: `ai_delegate*`, `ai_opencode*`. New endpoints: `/api/ai/delegate`, `/api/admin/opencode`.

[Download v12.6.11](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.11) · [OpenCode in WRM](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.11/README.md#opencode-zen-agents-and-helper-models)

### Included from v12.6.10 — One canvas, arranged your way

#### Added

- **One canvas:** the Workboard is no longer a page over your windows. Everything in the window area shares one canvas: terminals, file managers, desktops, Workboard addons and panels you place on it. Put a server-load addon next to the terminal of that server, a clock in a corner, notes under a file manager, and it stays like that.
- **Addons are windows:** move them, resize them, snap them to halves and quarters, minimize, maximize, switch with the tabs, or dock one as a panel at the left, right or bottom. They come back exactly where you left them, in every browser.
- **Panels anywhere:** drag a panel's tab anywhere on the canvas (or pick *On the canvas (anywhere)* in its ⋯ menu) and it stays there between your windows, not only in the docks at the edges.
- **The ▤ Workboard menu** holds the canvas tools: add addons, **snap to a grid** (10, 20 or 40 px, with dots or lines on the canvas), **tile** or **cascade** all windows, minimize and show them, show or hide the addons, **lock** the whole layout, **saved layouts** (for example *Morning check* and *Deploy day*, optionally with the open terminals), and **export / import** a layout as a file to share it or move it to another WRM.
- **Pin and always on top:** right-click any window's title bar or tab to pin it in place or keep it above the others.
- **New addons:** *Links* to your dashboards and wikis, *Saved sessions* to open one with a click, and a *Timer* (countdown with a notification, or stopwatch). Every addon can get a colour and a text size.
- **Close from the reconnect banner:** when a terminal or desktop connection closes or fails, the banner has a red **✕ Close window** button next to **↻ Reconnect** (#70).

#### Changed

- **Top bar:** *Broadcast* is in the **🧰 Tools** menu with the network tools (the button turns orange while a broadcast is on); *Shares* is only its icon; **Settings, Panels and Workboard** sit together right of *Keys*.
- **Lock the layout** now also keeps windows from being moved or resized.
- Folder bookmarks in the file manager (#17) have been in WRM since v10.10: ★ next to the path bar, per connection, folder, tag or for all connections, with WinSCP import.

#### Upgrade

Replace the executable/container and keep the database and encryption key. No database change. Your v12.6.9 Workboard tiles appear as addon windows beside your terminals the first time; panels you pulled out stay where they are. The per-user interface store accepts up to 256 KiB.

[Download v12.6.10](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.10) · [The canvas](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.10/README.md#workboard-the-shared-canvas-and-addons)

### Included from v12.6.9 — Workboard: what matters, at a glance

#### Added

- **Workboard** (▤ in the top bar): a page of tiles over your windows, empty until you add something. Pick addons from the catalog and arrange them: drag a tile by its title to move it, make it wide or narrow, give it your own title, remove it again. Your Workboard follows you to every browser.
- **Addons to start with:**
  - **Server status:** which servers are down, slow or up, with latency and since when; problems first, or only problems. Click a server to open it.
  - **Server load:** CPU, RAM, load, uptime and disk usage of the servers you choose, as bars that turn yellow at 75 % and red at 90 %. Values come from Monitoring when the server has it, otherwise WRM reads them over SSH (Linux) once a minute while the tile is visible. Each server says where its values come from and how old they are; errors are shown as they are.
  - **Notifications:** everything WRM told you (servers down and back, expiring keys, new versions, Git jobs) in WRM itself, also when no e-mail or chat channel is set up. Unread ones are marked and counted on the ▤ button.
  - **Clock:** your local time and the time zones you work with.
  - **Recent commits:** the last 10 (or 5, 20) commits and pushes of a service from your Git workspace (GitLab, GitHub or Gitea), with author, branch and a link.
  - **Running tunnels**, **Quick connect** buttons for your most used servers, and **Notes**.
- **Pull a tile out:** **⤢** turns a tile into a panel you can dock left, right or at the bottom, stack with *Sessions*, or let float, like every other panel. It stays there after a reload; *Back to the Workboard* returns it. A locked layout keeps tiles in place too.
- **Administrators** decide whether server load may be read over SSH: *Workboard: read server load over SSH* in *Admin panel → Policies → Live status* (on by default).

#### Changed

- A tab bar that is too narrow for all its tabs scrolls to the open one.

#### Upgrade

Replace the executable/container and keep the database and encryption key. The database gets one new table (`user_events`); a previous version still starts on it. Nothing new appears until you open the Workboard and add an addon. New endpoints: `/api/me/events`, `/api/workboard/metrics`.

[Download v12.6.9](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.9) · [Workboard](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.9/README.md#workboard-and-addons)

### Included from v12.6.8 — Your panels, where you want them

#### Added

- **Arrange the page yourself.** *Connections*, *Sessions*, *Snippets*, *Clipboard* and the AI live view are now panels with a tab. Drag a panel by its tab:
  - onto another panel's tab bar, and they share it as tabs;
  - onto the top or bottom part of a panel, and the column is split between them;
  - onto the *Left*, *Right* or *Bottom* strip that appears at the edges while you drag, and it docks there (the bottom zone sits under your windows);
  - anywhere else, and it becomes a floating window you move by its tab bar and resize by its corner.
  Drag the line between two panels, or a zone's edge, to resize. Every panel's **⋯** menu also moves it, hides it, locks the layout or resets it.
- **Panels** (▦ in the top bar): every panel with where it is; show or hide it, lock the layout, or go back to the start layout.
- **Lock the layout** so nothing moves by accident: no dragging, splitting or resizing while locked (the button shows 🔒). Tabs, collapsing the side panels, showing and hiding still work.
- **Your layout follows you:** it is stored for your account on the WRM server, so every browser and computer you sign in on shows the same arrangement.

#### Changed

- The AI live view is a panel like the others: dock it left, right or at the bottom, stack it with *Sessions*, let it float or hide it. Its title bar buttons and the 🤖 menu still work.
- On a phone the page keeps its fixed layout with the two drawers.

#### Upgrade

Replace the executable/container and keep the database and encryption key. The database gets one new table (`user_ui`); a previous version still starts on it. Your first layout is built from what this browser remembered in v12.6.7 (the right panel's tab and width, where the AI live view was).

[Download v12.6.8](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.8) · [Panel layout](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.8/README.md#panel-layout-dock-stack-float-and-lock)

### Included from v12.6.7 — A top bar that fits, an AI panel that docks

#### Added

- **The AI live view docks where you want it.** What an AI app runs on your servers (commands, files, WRM's decision, approvals and output) is no longer a box over your windows. Choose with the buttons in its title bar, in the 🤖 menu, or by dragging the title bar onto the right or bottom edge:
  - **Right** (default): a **🤖 AI** tab next to *Sessions* and *Snippets*, with its own width; the tab counts the open AI sessions and turns yellow while one waits for you.
  - **Bottom**: a dock under the windows, resizable by its top edge; several AI sessions sit side by side.
  - **Floating**: a window you move by its title bar and resize by its corner.
  - **Hidden**: a small *Show the AI panel* chip at the bottom left, which pulses while an approval waits.
- WRM remembers the layout, the floating position and size and the dock height for each user in this browser.
- Approval cards still always pop up, whatever the layout, so no request is missed. On a phone the live view is a sheet at the bottom of the screen.
- **Medium works behind proxies and antivirus that inspect HTTPS.** The original deploy tool's Docker runtime now trusts the certificates your WRM computer trusts (the Windows certificate store, the macOS keychains or the Linux CA bundle), so pip no longer fails with *self-signed certificate in certificate chain*. New policy *Medium trusts this computer's certificates* (on by default) and *Extra CA file for Medium* for a proxy root that is not in the computer's store, in *Administration → Policies → Git*.

#### Changed

- **The top bar fits at any width and browser zoom.** It measures itself and first drops the labels of less used buttons; buttons that still do not fit move into a **⋯** menu (with their badges), so no button covers another at 1100–1920 px or at 125–150 % zoom.
- The right panel remembers its width, separately for the AI tab.

#### Fixed

- **You can select and copy every text on the page again**, for example Medium's command output, error messages and hints. Only buttons, tabs, title bars and drag handles stay unselectable.
- The *Snippets* tab no longer looks selected while *Sessions* is open.
- When pip fails because of an untrusted certificate, Medium says so and what to do.

#### Upgrade

Replace the executable/container and keep the database and encryption key. No database or API change. If Medium's runtime preparation failed with a certificate error, press *Prepare runtime* again after the upgrade.

[Download v12.6.7](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.7) · [AI live view layout](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.7/README.md#ai-desktop-apps-mcp)

### Included from v12.6.6 — Docker everywhere, installed by WRM

#### Added

- **Medium installs Docker itself.** When Docker is missing on the WRM host, step 2 of the Medium guide says which Docker WRM will install and how. An administrator presses **Install Docker** once; on Windows and macOS that also accepts Docker Desktop's licence (shown with its terms). From then on importing the tool ZIP or pressing *Prepare runtime* installs Docker, starts it, waits for it and prepares the runtime, with no manual steps.
  - **Linux:** Docker Engine from your distribution's packages, its service enabled and started.
  - **macOS:** Docker Desktop from docker.com, signature checked, installed for the user signed in on the Mac.
  - **Windows:** Docker Desktop from docker.com, signature checked, installed quietly for the signed-in user. The first time Windows may ask once to enable WSL and to restart.
- **Every step is visible:** installing, downloading, checking the signature, starting and waiting for Docker, each with its time, the command output and what to do when it fails, also in the activity log.
- **One command when WRM has no rights:** if WRM runs as a normal Linux account, step 2 shows `sudo wrm -setup-docker -service-user <account>`. It installs Docker, lets WRM use it and restarts WRM.
- **New policy** *Install Docker for Medium* (`medium_docker_install`, on by default) and *Medium command output* in *Administration → Policies → Git*.

#### Changed

- The original deploy tool now runs in the same Docker runtime on Linux, macOS and Windows. A host runtime you already prepared on Linux or macOS keeps working; `WRM_MEDIUM_RUNTIME=docker` moves it to Docker.

#### Upgrade

Replace the executable/container and keep the database and encryption key. No database change. Nothing is installed until an administrator presses **Install Docker**. Docker Desktop is free for personal use, education, non-commercial open source and small businesses; larger organisations need a paid Docker subscription.

[Download v12.6.6](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.6) · [Docker for Medium](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.6/README.md#docker-for-medium-installed-by-wrm)

### Included from v12.6.5 — Medium tells you what it is doing

#### Added

- **Step by step:** the original deploy tool's card in Medium is a numbered guide: 1 import the deploy-tool ZIP, 2 prepare its runtime, 3 your deploy-config, 4 your SSH profile (optional), 5 start/stop. Each step shows whether it is done, running, waiting or failed, why a button is disabled and what to do when something fails.
- **Signals:** a light for the original tool (off, working, running, needs attention) and one for the read-only WRM preview, which is now a separate panel.
- **What runs, who and since when:** while an operation runs, everyone sees it, and its starter sees every step with its time and the live output of Docker, pip and the original tool. A second click is answered with the operation, the user and the start time instead of "busy".
- **Activity log:** every Medium action with time, user, result and duration; click a row for its steps and output. Secrets in the output are redacted. Administrators decide who sees output with `medium_log_output` (everyone, administrators, nobody).
- WRM's console logs each Medium operation and step.

#### Changed

- Your deploy-config ZIP is checked by WRM immediately and, while the runtime is still being prepared, kept and applied at your next start, so you no longer wait to upload it.
- Start and the tool import run in the background and the page follows them; a failure stays shown with the failed step and the last lines of the output.

#### Fixed

- **Approve later after the AI app closed its session:** approving such a request in WRM failed with "AI session not found". It now works for the request's hour, and the AI runs the same command once when it repeats it in a new session.
- **Search patterns are not files:** a `grep` regular expression, a `sed` or `awk` script and `find -name` patterns no longer count as file names, so a pattern such as `'^[[:space:]]*command=.*'` is not taken for a PuTTY key and does not ask for approval.

#### Upgrade

Replace the executable/container and keep the database and encryption key. One additive table (`medium_activity`); a previous binary still starts on the database. API clients: `POST /api/git/medium/start` now answers `202` (poll `GET /api/git/medium`).

[Download v12.6.5](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.5) · [Medium step by step](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.5/README.md#medium-step-by-step-status-and-activity-log)

### Included from v12.6.4 — Your blocked commands, your choice

#### Added

- **Blocked commands with full access** (`ai_full_access_blocked`, *Admin panel → AI connections*): the administrator decides what happens to their own list of blocked commands (`ai_blocked_commands`) when full access is on.
  - **Ask for approval** (default): as in v12.6.3, such a command comes to you as an approval marked *FULL ACCESS*.
  - **Stay blocked**: such a command is refused even with full access, and the AI is told that an approval cannot help and what you can change instead.

The built-in list of destructive commands, deny lists, shutdown and secret files keep asking under full access either way.

#### Upgrade

Replace the executable/container and keep the database and encryption key. No database change. Nothing changes until an administrator sets *Blocked commands with full access* to *Stay blocked*.

[Download v12.6.4](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.4) · [MCP setup](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.4/README.md#ai-desktop-apps-mcp)

### Included from v12.6.3 — Full access with one switch

#### Added

- **The administrator chooses how full access applies** (`ai_full_access_mode`, *Admin panel → AI connections*):
  - **Every AI session** (default): the **Full access for the AI** switch alone opens full access for every AI session, the WRM AI panel and every AI connection, without ticking a scope anywhere.
  - **AI connections with the Full access scope**: as in v12.6.2, a user ticks **⚠ Full access** on an AI connection.
  - **Users who turn it on themselves**: each user gets a **Full access for my AI sessions** switch in *Settings → AI connections*; it applies to all of that user's AI sessions.
- *Settings → AI connections* shows whether full access is on and how it applies.

Full access still never runs anything WRM would refuse on its own: such a command or file comes to you as an approval marked *FULL ACCESS*, every time, and read-only sessions stay read-only.

#### Upgrade

Replace the executable/container and keep the database and encryption key. One additive column (`users.ai_full_access`); a previous binary still starts on the database. **If you had already turned on `ai_full_access` in v12.6.2,** it now applies to every AI session (mode *Every AI session*); set `ai_full_access_mode` to *AI connections with the Full access scope* to keep the v12.6.2 behaviour.

[Download v12.6.3](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.3) · [MCP setup](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.3/README.md#ai-desktop-apps-mcp)

### Included from v12.6.2 — Full access, but Claude still asks

#### Added

- **Full access with approval:** an administrator turns on **Full access for AI connections** (`ai_full_access`, *Admin panel → AI connections*). After that, an AI connection can get the **⚠ Full access** scope. With it, WRM refuses nothing outright: destructive commands, blocked commands, your deny lists, shutdown and reboot, protected paths and secret files all come to you as an approval marked *FULL ACCESS*, with the full command and the rule WRM would otherwise apply. They are asked every time and never run automatically, even with "Allow everything". Ordinary commands keep working as the connection is set up.

#### Changed

A refused command now tells the AI that full access exists and that you or your administrator turn it on, so it can explain the choice to you.

#### Upgrade

Replace the executable/container and keep the database and encryption key. No database change. Nothing changes until an administrator turns on `ai_full_access` and you give a connection the *Full access* scope (edit the AI connection; open sessions end and are reopened). A read-only connection stays read-only even with full access.

[Download v12.6.2](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.2) · [MCP setup](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.2/README.md#ai-desktop-apps-mcp)

### Included from v12.6.1 — "Allow everything" means everything, and nothing ends in a dead refusal

#### Added

- **Approve later:** when the AI app declines WRM's question, stops waiting, or nobody answers in time, the request is no longer lost. It stays in WRM for an hour as a card with the full command and **Approve** / **Deny** (also in the live panel). Approve it, tell the AI to continue, and the same command runs once without asking again.
- **Files that may hold secrets:** reading a configuration or credentials file WRM treats as sensitive now asks you instead of failing; after you approve, the AI gets the file with passwords, keys and tokens replaced by `[REDACTED]`. Administrators can keep such files blocked with `ai_sensitive_reads` = deny.

- **Allow everything:** when you create or edit an AI connection in automatic mode, **Allow everything** fills in `*` for commands and `/**` for files, and **No time or action limit** keeps the connection automatic for as long as it is valid (up to the lifetime you chose). The AI then runs commands on the chosen servers without asking, also backups such as `cp -a a a.bak && cp -a b b.bak && ls -la`. Administrators can switch this option off with `ai_auto_until_grant`.

#### Changed

- `*` in the allow list now covers chains, variables, groups and redirections. Blocked commands (`ai_blocked_commands` and the built-in destructive list), your deny lists, sensitive files and commands WRM cannot read still ask or are refused, and the AI connection still reaches only the servers you chose, until it expires.
- Commands that stay blocked (the destructive list, your deny lists, read-only sessions) now say so: the AI is told that an approval cannot help and what you can do instead.
- When WRM does ask and the answer is no (or nobody answers), the AI is told which rule asked, for example "the automatic mode time limit is over" or "not in the session's allow list", what you can change in *Settings → AI connections* so it is not asked again, and that it must tell you this and ask before trying again.

#### Fixed

A long automatic session switched back to asking once its time limit (at most 60 minutes by default) was over, and commands with variables asked even with `*` allowed. When the AI app declined that question, the AI only saw "The user denied this action" although you had allowed everything, and there was nowhere to approve it afterwards.

#### Upgrade

Replace the executable/container and keep the database and encryption key. No database change. Reading files that may hold secrets now asks instead of being refused; set `ai_sensitive_reads` to `deny` in *Admin panel → AI assistant* to keep the old behaviour. Existing AI connections keep their limits: to allow everything without limits, edit the connection, press **Allow everything**, confirm and save (open sessions end; the AI opens them again with the new rules).

[Download v12.6.1](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.1) · [MCP setup](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.1/README.md#ai-desktop-apps-mcp)

### Included from v12.6.0 — Medium's native engine, side by side with the original tool

#### Added

- **Native engine preview (read-only):** open **Medium** in the Git workspace and expand *Native engine (preview, read-only)*. WRM reads your `deploy-config` as it is, with the same validation as the original tool, and lists every server of `servers.yaml` with the WRM connection it will use. Connections are matched by address and user; pick another one where needed.
- **Plan:** choose a project, an environment and a branch or tag, then **Preview natively**. WRM prepares the source from your WRM Git source and compares it with each destination over SSH, the way `rsync -rlc --delete` with the project's exclusions would, and shows new, changed and deleted files, the exclusions, what is on the server (version, ref, lock, recent deployments) and the `post_deploy` command. Nothing is written to any server.
- **Compare with the original tool:** with Medium running, **Compare with the original tool** runs the original comparison and the native one on the same source and says per server whether they give the same result, listing any change found by only one of them. Projects with a build step use the original tool's built source until the native engine arrives.

#### Changed

The Medium bridge has one more authenticated endpoint for the comparison; the original tool itself is not modified.

#### Upgrade

Replace the executable/AppImage/container and keep the database, encryption key and Medium directories. One additive table (`medium_server_links`); a previous binary still starts on the database. The preview needs a WRM Git source (GitLab, GitHub or Gitea API) for the repositories named in `projects/*.yaml` and saved SSH connections for the servers. Deploying is still done by the original tool; the native deploy, build location and rollback follow in v12.7.0 ([roadmap](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.0/ROADMAP.md)).

[Download v12.6.0](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.6.0) · [Medium native engine](https://github.com/vedranius/web-browser-RDM-public/blob/v12.6.0/README.md#medium-native-engine-preview)

### Included from v12.5.1 — Claude Desktop and Claude Code in one step

#### Added

- **Claude Desktop extension:** in *Settings → AI connections* pick **Claude Desktop**, choose the servers and how long the token is valid, then **Download Claude Desktop extension**. Open `wrm-pro.mcpb` and press **Install**: the WRM address and token are already filled in. The extension runs on your computer with Claude Desktop's built-in Node.js, so it reaches a WRM on your own network and over plain HTTP; a self-signed HTTPS certificate can be trusted with a CA file in the extension's settings. Approvals still appear in WRM and, where supported, in Claude.
- **Claude Code plugin:** `claude plugin install wrm-pro --marketplace vedranius/web-browser-RDM-public --config wrm_url=… --config wrm_token=…` (Claude Code 2.1.275 or later). The wizard prints the command with your values; the token is kept in Claude Code's secure storage.
- **See what the AI does:** when an AI app works on a server through WRM, a live panel shows each command, WRM's decision, the approval buttons, the exit code and the output. *Watch live* in the 🤖 menu reopens it; the automatic opening can be turned off there.
- **Lifetime in hours or days:** the wizard, the advanced form and the OAuth approval dialog take the token lifetime in hours or days, so apps need not be approved again every day.

#### Changed

When you deny a command, or nobody answers in time, the AI is told where it was refused and to ask you whether to continue, change the command or stop, rather than trying again on its own. Default AI connection lifetime is 7 days (`ai_mcp_token_hours` 168), at most 30 days (`ai_mcp_max_token_hours` 720).

#### Fixed

Claude Desktop could not be connected to a WRM on a private network: the wizard offered only the claude.ai custom connector, which connects from Anthropic's cloud and needs a public HTTPS address, and `claude_desktop_config.json` does not accept HTTP entries. The wizard now names that option *Claude.ai (cloud connector, public HTTPS)* and offers the extension for Claude Desktop. README troubleshooting explains an empty `mcpServers` block.

#### Upgrade

Replace the executable/container and keep the database and encryption key. No database change. MCP stays off by default (`ai_mcp_enabled`). Token lifetimes default to **7 days, at most 30 days** (were 8 hours, at most 24); values an administrator saved explicitly stay in force and can be changed in *Admin panel → AI connections*. The extension contains the token: delete the downloaded file after installing. Windows packages are cross-compiled; the extension was validated with the official MCP bundle tool and its bridge was run against WRM, but not inside a native Windows Claude Desktop in this release's validation.

[Download v12.5.1](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.5.1) · [MCP setup](https://github.com/vedranius/web-browser-RDM-public/blob/v12.5.1/README.md#ai-desktop-apps-mcp)

### Included from v12.5.0 — Stamping as it is, Back in dialogs and reviewed fixes

#### Added

- **Stamp as it is:** the Stamp dialog lists each installation's differing files and says why it cannot be stamped instead of "No files are selected". **Also stamp installations that are not up to date** writes VERSION.md with each file's actual version (older files their matching version, hand-changed files `(local)`) and a `Napomena` note that the installation is partial.
- **Back in dialogs:** a Git dialog opened from another one (Details → Update, Upgrade → file selection → confirmation, Rollback …) has **← Back** (Alt+←), which returns to the previous dialog with its ticks and typed values. Settings → Import inventory, Settings → Admin panel and Tunnels → Edit connection return to where they were opened.
- **Medium forced stop:** when the original tool cannot report its state, Stop can be confirmed a second time and stops it anyway.

#### Fixed

- **Production confirmation** in the Git workspace compares the typed connection name (it compared a hostname, so production updates could not start).
- **Medium:** one user's start or setup no longer blocks other users' status, proxy and Stop; empty settings keep the original defaults and WRM's SSH options; a successful start or import clears an earlier setup error and older pending imports; an imported config replaces a config repository; replaced SSH profiles (and their keys) are deleted; ZIP names that differ only in case are rejected; status polling continues after a network error; stage texts are translated.
- **Logs:** a non-UTF-8 byte no longer stalls a search; search segments end at a complete line; keyword highlighting is literal (`foo(`, `c++`, `[ERROR]`); empty files complete; Previous segment no longer skips data; logs are discovered again after Monitoring settings change; a closed tab during Check now no longer records a failed sample or a problem notification; a late check does not repaint another tab.
- **Client GitLab cache:** a newer snapshot published by another member is not deleted; WRM relay errors are no longer reported as GitLab CORS problems; one failed status poll no longer blocks later refreshes.
- Following a log in a terminal keeps the shell after Ctrl+C; moving or deleting a folder rechecks folder shares, so shared terminals outside the shared subtree end.

#### Upgrade

Replace the executable/container and keep the database, encryption key and private runtime directories/volumes. No schema migration. Earlier Medium versions kept replaced SSH profiles as `.previous` directories; they are removed at the next start.

The next versions replace Medium's Docker runtime with WRM's native engine (same original interface and deploy-config, WRM connections and keys, chosen build location); see [ROADMAP.md](https://github.com/vedranius/web-browser-RDM-public/blob/v12.5.0/ROADMAP.md). [Downloads](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.5.0).

### Included from v12.4.5 — One-click Medium and usable log search

#### Added

- **Medium:** add the original trusted tool/config ZIPs and optional SSH ZIP, then **Prepare and run**. WRM starts Docker, prepares the original dependencies, validates configuration, synchronizes profiles and opens the unchanged tool. Approved imports survive setup failure and can be resumed without re-uploading.
- **Logs:** automatic discovery from manual roots, service log folders and Supervisor metadata; progressive server results, visible errors, operator/service filters and historical cross-file keyword/date search. Tiles show modification/event times, highlighted excerpts and open/follow actions. Search can be canceled or continued when a bound is reached.
- **Environments:** combined text/service/environment/server filters with sticky controls.

#### Fixed

The whole Overview/Installations filter/check/action area stays at the top during scroll. Monitoring tab responses are isolated. Date-only searches and continuation-event timestamps are handled. Discovery avoids repeated per-file metadata round trips; host metric versus log prerequisites are explained.

#### Upgrade

Retain the database/key and private runtime directories/volumes, then replace the executable/container. No schema migration. Windows setup may display the official Docker installer and vendor/OS prompts; required license choices, WSL/virtualization setup and restarts remain user actions. A Windows service needs a preinstalled Docker account; other Windows containers are never stopped or switched automatically. Linux without installed Docker/native prerequisites needs its administrator's initial package setup. Windows builds are cross-compiled, without a native Windows VM run.

Search is bounded per file/round and excerpts; incomplete results show Continue. Compressed/binary logs remain explicit unsupported text inputs. See [guide](https://github.com/vedranius/web-browser-RDM-public/blob/v12.4.5/docs/V12.4.5_GUIDE.md), [validation](https://github.com/vedranius/web-browser-RDM-public/blob/v12.4.5/docs/RELEASE_SUMMARY_v12.4.5.md) and [downloads](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.4.5).

### Included from v12.4.4 — Monitoring, logs and AI workflows

#### Added

- **Monitoring** brings Zabbix server metrics, history, agent detection and service Functionality into one workspace. Use passive agent/agent 2 over SSH or your configured network route, verified certificate TLS, PSK through the existing remote `zabbix_get`, or an existing HTTPS Zabbix server API for active-only agents. Unavailable metrics are unknown and diagnostics remain visible.
- **Logs** supports multiple roots/operators/services, historical segmented search and date/timezone/keyword filters, highlighted results, copying/downloads, terminal actions and up to 16 movable/resizable live windows with tiling/cascade/pause/rotation handling.
- **Client GitLab tokens and cache** let a computer with GitLab access supply bounded repository snapshots to WRM. The GitLab token stays in browser memory or the companion's local token file. The latest complete successful server cache survives client disconnect and WRM restart; refresh is atomic, sharing is explicit and reviewed deployment content stays pinned.
- **MCP onboarding and permissions:** CLI setup scripts, OAuth/cloud guidance, Cursor install link, `wrm` prompt and authorized server picker, native assistant `/wrm`, safe structured configuration inventory, visible Automatic within limits and editable existing grants. Grant edits retain bearer/expiry and terminate old sessions/approvals.
- **Terminal and navigation:** previewed/redacted selection-to-AI context, Open terminal here for POSIX/PowerShell, Copy/Paste/native-menu fallback, hostname groups/combined Functionality filters and folder/connection colors/icons.

#### Fixed

General import recognizes mRemoteNG XML/PuTTY `.reg` ([issue #5](https://github.com/vedranius/web-browser-RDM-public/issues/5)); synthetic 4 MiB XML and UTF-16 registry exports were exercised in the browser. Provider header/request/idle timeouts are configurable for longer local AI requests. Wide replay fits phone/desktop widths. Search/filter bars remain at the top of supported scroll areas, and MCP wizard controls align correctly on narrow screens.

#### Upgrade and operation

Replace the executable/container and retain the database, encryption key and private runtime files. Migrations are additive. Functionality is now **Monitoring → Functionality**. Configure each server's installed Zabbix agent and log roots in Monitoring settings; automatic polling is opt-in. MCP stays disabled by default and existing explicit administrator policies remain in force.

Browser GitLab access needs the GitLab CORS/origin/local-network permissions; use the WRM companion when the browser cannot reach that API. The relay is for WRM repository API reads: native Medium Git clone/build/config Git operations and GitLab writes still need a direct route. Native Medium's Windows Linux-container prerequisite from v12.4.3b remains.

[Download v12.4.4](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.4.4) · [setup/limits](https://github.com/vedranius/web-browser-RDM-public/blob/v12.4.4/docs/V12.4.4_GUIDE.md) · [validation](https://github.com/vedranius/web-browser-RDM-public/blob/v12.4.4/docs/RELEASE_SUMMARY_v12.4.4.md). Windows packages are cross-compiled; no native Windows VM execution is claimed.

### Included from v12.4.3b — Windows Medium runtime fix

#### Fixed

Windows `.exe` installations run the original Medium deploy tool inside a managed Linux Docker runtime. Python, Git, OpenSSH and rsync are supplied by its pinned image; separate Windows installations are unnecessary. ZIP imports survive setup failure and can be prepared again without re-uploading. Private Linux volumes preserve deploy history, rollback state, permissions and config edits across restarts. Native Linux mode remains available.

The original tool remains unchanged. Runtime source/venv are read-only, SSH/config are private and builds use an unprivileged user. Runtime environment forwarding preserves Docker CLI context and keeps secret values out of command arguments. Only owned containers are stopped.

#### Windows upgrade

**Download the new Windows executable manually:** v12.4.3 cannot parse a `b` suffix; v12.4.3b supports it for later updates. Retain the DB/key. Start Docker Desktop with **Linux containers** on Windows 10/11, then use **Git → Medium → Prepare runtime / retry** for an existing tool import, or import your trusted tool ZIP first. Import deploy-config/SSH profile and start Medium. A browser client needs no deployment dependencies.

Docker Desktop is not supported on Windows Server; use a supported Linux WRM host/VM/container for Medium. The Docker engine must be local and accessible to the WRM account. Back up private `.medium` files and the managed Linux runtime/user Docker volumes.

#### Validation and packages

Real pinned-image setup, original comparison/build, two actual OpenSSH/rsync deployments, rollback and stop/start history persistence were exercised against a local fixture. The build user could not read private SSH/config. Browser layouts at 390/1100/1440/1920 px had no JavaScript errors. Windows is cross-compiled; no native Windows VM run is claimed. Full validation: [hotfix summary](https://github.com/vedranius/web-browser-RDM-public/blob/v12.4.3b/docs/RELEASE_SUMMARY_v12.4.3b.md).

[Download v12.4.3b](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.4.3b): 16 platform packages plus checksums; Docker `ghcr.io/vedranius/wrm-pro:v12.4.3b` and `latest`. The remaining accepted work is planned for **v12.4.4** in ROADMAP, not included in this hotfix.

### Included from v12.4.3 — Original Medium deployment, config profiles and Bitwarden

#### Added

- **Medium** uses the original supplied deploy tool and deploy-config format unchanged: original screens, comparisons, file selection/exclusions, builds, native plans/confirmations, deployment progress, backups, history, rollback and config Git controls. Import the private tool/config ZIPs in Git → Medium. WRM handles authenticated access, private directories, encrypted runtime preferences and setup status. [Original-tool parity record](https://github.com/vedranius/web-browser-RDM-public/blob/v12.4.3/docs/MEDIUM_PARITY_v12.4.3.md).
- **Functionality config profiles** inspect uploaded reference settings across servers/operators and show searchable reference/actual values, differences, missing settings and masked secrets. INI/config/Python parsing is static; incomplete checks remain explicit.
- **Bitwarden** account sign-in/unlock and direct native SSH item references for SSH/SFTP. Private key retrieval happens on the server during authentication. Master passwords and unlock sessions are not persisted; per-user CLI profiles are encrypted between commands.
- **Folder order** persists after drag/drop, Move up/down and export/import, including nested folders.

#### Fixed and changed

- Git dialogs now stay visible above mobile drawers and keep wide tables within their scroll area.
- The bridge supports original same-origin no-referrer forms and rejects cross-site submissions; stop refuses active original-tool operations.
- Docker includes Python, git, OpenSSH, rsync and the official Bitwarden CLI. Standalone installations need these optional dependencies on the WRM host. Medium defaults to administrators; grant it only to trusted operators because its native build/config commands run as the WRM OS account.

#### Validation and packages

Original tool: **1,581 tests passed, 9 skipped**. Real Chromium exercised private archive imports and native comparison, deploy, history and rollback against test Git/SSH destinations. Config profiles, folder persistence and layouts at 390/1100/1440/1920 px were checked without JavaScript errors. Bitwarden mock-account flows and direct real SSH authentication were tested; a real cloud account was not used. Full WRM race/vet/format/UI checks and platform builds are detailed in the [release summary](https://github.com/vedranius/web-browser-RDM-public/blob/v12.4.3/docs/RELEASE_SUMMARY_v12.4.3.md).

Release packages: 13 standalone targets, macOS universal and Linux amd64/arm64 AppImages, plus `SHA256SUMS.txt`. Docker: `ghcr.io/vedranius/wrm-pro:v12.4.3` and `latest`. The pipeline publishes only after the complete package set and Docker succeed.

#### Upgrading from v12.4.0

Replace the binary/AppImage or update Docker. Startup adds the config-profile table and folder order column. Preserve your database, encryption key and private adjacent `.medium`/`.bitwarden` directories. Tool and config archives are private imports, not public release assets. Git/SSH targets must be reachable from the runtime. The earlier client GitLab relay/replay, AI/navigation and icon/color requests remain accepted backlog, and were not included in this prioritized release.

**Downloads:** [v12.4.3 packages and checksums](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.4.3). [Changelog](https://github.com/vedranius/web-browser-RDM-public/blob/v12.4.3/CHANGELOG.md).

### Included from v12.4.0 — Service functionality, nested folders and remote previews

Inspect service functionality and configuration on your servers, distinguish identically named connections by client/site, and preview remote files directly in WRM.

#### Added

- **Git → Functionality** checks every Supervisor worker and failed systemd service on selected servers, as well as each catalog service's runtime units. It shows installation problems, disk/memory warnings, timestamps and bounded error logs. Optional HTTP/TCP health probes run from the selected server. Unavailable checks remain unverified; problem/recovery notifications are available.
- **Configuration** shows section, variable/function, value and description for each service's config/INI/environment files and JSON. Secrets are masked. Paths stay inside the installation; contents are read on demand with explicit limits and parse-gap warnings.
- **Nested folders** ([#57](https://github.com/vedranius/web-browser-RDM-public/issues/57)): recursive tree, parent selection, complete folder paths in connection pickers/windows/tabs, folder tab filtering and hierarchy-preserving imports. Folder shares include descendants; route defaults remain local to the immediate folder.
- **Remote previews** ([#55](https://github.com/vedranius/web-browser-RDM-public/issues/55)): video/audio streaming with seekable byte ranges over SFTP/FTP, image/PDF viewing and bounded text viewing. Select a path in a terminal and right-click to preview/download it; the file manager offers Preview too. Browser codec support determines playable media formats.

#### Fixed

- **AI entry points and diagnostics** ([discussion #56](https://github.com/vedranius/web-browser-RDM-public/discussions/56)): AI is visible in SSH/SFTP windows; unavailable permissions lead to an explanation. Settings → AI assistant and the top AI button provide setup/status access. Policy changes refresh other open browsers. A successful provider test is distinguished from account permission, and the terminal assistant requires no MCP setup.
- Missing installations stay visible with a warning. Supervisor programs in the same configuration file are associated with the correct service stanza.
- Docker Compose uses the current version consistently.

#### Packages and validation

Linux, Windows, macOS (including universal), Android/Termux, FreeBSD and OpenBSD standalone binaries; Linux amd64/arm64 AppImages; Docker `ghcr.io/vedranius/wrm-pro:v12.4.0` and `latest`. The release pipeline verifies all 16 downloadable packages and `SHA256SUMS.txt` before publication.

Validation details: [v12.4.0 release summary](https://github.com/vedranius/web-browser-RDM-public/blob/v12.4.0/docs/RELEASE_SUMMARY_v12.4.0.md).

#### Upgrading from v12.3.6

Replace the binary/AppImage or update the container. Startup adds `folders.parent_id` and a service-health cache table without removing existing data. Previous binaries ignore the new fields; run the previous binary to downgrade. Existing catalogs need no new fields. Git runtime checks use the SSH account's read permissions and do not restart services.

**Downloads:** [v12.4.0 packages and checksums](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.4.0). See [CHANGELOG.md](https://github.com/vedranius/web-browser-RDM-public/blob/v12.4.0/CHANGELOG.md) for the complete history.

### Included from v12.3.6

This release adds deployment review and server selection to Git Simple, gives AI tools the server's actual date, and improves Linux systemd service installation and failure diagnostics.

#### 🤖 AI assistant

- Ollama setup releases its action lock before reporting completion, so the next setup step can start immediately. This fixes the intermittent HTTP 409 exposed by the release CI run.

- `system_info` now reports the remote server's actual date and time instead of leaving the model to infer it from the kernel build string.
- The assistant is explicitly instructed not to invent tool calls, tool output, logs or internal `<tool_response>` wrappers.
- Enabling or disabling the assistant refreshes its buttons on already-open terminals without reconnecting. Shared connection roles remain enforced; legacy lowercase SSH protocols are accepted.
- The processing indicator now has its English and Croatian translation.
- The indicator remains visible until the model finishes. Recognized tool JSON is removed from the finalized chat message, and incomplete JSON is no longer repaired into executable calls or silently removed from answers.

#### ⎇ Git Simple

- Git Simple now has combined service, hostname, environment, status, branch and version/commit filters. Its deploy review shows changed files and their state per destination, lets you select successfully compared servers and exclude files, and shows when server state was compared. Deploys keep using WRM's fingerprinted plan, backup, lock and audit flow.
- Empty server selections are refused; confirmation is recomputed whenever the selection changes. Canceled or superseded comparisons cannot reopen a review. Rollback shows each selected server's own previous version. Simple mode keeps removed files; Advanced mode offers deletion after backup.
- Connection changes after comparison invalidate the plan, so a reviewed deployment cannot silently follow a changed SSH destination or login.

#### 🐧 Linux service

- Installation checks read/write access to SQLite and its WAL/SHM files, plus write/search access to its directory, before changing service settings. It points to `DB_PATH` when permissions prevent startup.
- If systemd cannot start WRM, installation reports the unit status and recent journal entries; read-only database errors include a specific permissions hint.
- Generated units limit rapid restart loops after a persistent failure. The install summary shows the database path the service will use.
- Service definitions handle spaces, literal dollars and percent characters in paths correctly. Reinstalling clears the unit's old failures and restarts the newly configured binary.

#### 📦 Packages and validation

- **Linux**: amd64, arm64, ARMv7, ARMv6, 386; **AppImage** for amd64 and arm64.
- **Windows**: amd64 and arm64 `.exe` files.
- **macOS**: Intel, Apple Silicon and universal binaries.
- **Android / Termux**: arm64; **FreeBSD**: amd64 and arm64; **OpenBSD**: amd64.
- Docker: `ghcr.io/vedranius/wrm-pro:v12.3.6` and `latest` (Linux amd64).
- `SHA256SUMS.txt` covers all 16 downloadable packages. GitHub also provides source archives.

AppImage quick start: `chmod +x wrm-pro-v12.3.6-linux-amd64.AppImage`, then run it and open **http://localhost:8080**. On ARM64 choose the arm64 AppImage. Without FUSE, add `--appimage-extract-and-run`. Data stays in `${XDG_DATA_HOME:-$HOME/.local/share}/wrm-pro/` unless `DB_PATH` is set. Replace the AppImage to upgrade; use the standalone binary for boot services.

The pre-push review covered race tests, UI regressions, all 13 cross-compilation targets, native Linux/Docker health checks, Git Simple browser deployment and rollback, and a real local Ollama tool call. Other OS binaries were compiled; they were not run on their respective OSes. See [the review report](https://github.com/vedranius/web-browser-RDM-public/blob/v12.3.6/docs/PRE_PUSH_REVIEW_v12.3.5.md).

#### ⬆️ Upgrading from v12.3.4

Replace the binary or use the in-app updater. No database migration is required.

For an existing Linux service, re-run `-install-service` with the new binary to regenerate its unit and apply the path and restart-limit fixes. This restarts that service.

**Downloads:** [all v12.3.6 files and `SHA256SUMS.txt`](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.3.6)

See [the release summary](https://github.com/vedranius/web-browser-RDM-public/blob/v12.3.6/docs/RELEASE_SUMMARY_v12.3.6.md) for package and validation details.

See [CHANGELOG.md](https://github.com/vedranius/web-browser-RDM-public/blob/v12.3.6/CHANGELOG.md) for the complete change history.
