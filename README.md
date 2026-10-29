<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/svg/wrm-lockup-on-dark.svg">
    <img src="docs/brand/svg/wrm-lockup-on-light.svg" alt="WRM PRO — Web Remote Manager" width="420">
  </picture>
</p>

# Web Remote Manager PRO (WRM)

**A remote server manager that runs in any web browser, with an AI assistant and guided setup for AI agents.** Add your own AI provider or a local model: WRM offers model suggestions, an Ollama setup wizard and connection presets for MCP agents to help you start practical work on your servers quickly.

**Optional automatic Ollama installation:** from WRM, an administrator can install the Ollama server on the **WRM host**, start it as a background service and download the chosen model. Enable `ai_ollama_manage` in Admin → AI, then confirm each action in the provider wizard. Choose the suggested local model or another supported model; installation and setup actions are recorded and audited. Connect **Codex CLI, Claude Code, Gemini CLI, OpenCode, Cursor / VS Code, Claude or ChatGPT** through the MCP wizard, which supplies the connection commands or configuration and lets you choose the servers and permissions.

Manage **SSH terminals, SFTP / FTP / FTPS files and RDP / VNC desktops**, Git service comparison and deployment, jump hosts, proxies, tunnels, BMC and serial consoles, keys and credentials, inventory imports, server-to-server transfers, notifications and collaboration with voice. AI tools can inspect logs and services, diagnose problems, run permitted commands and propose file changes with read-only, approval or limited automatic modes. WRM also provides sharing roles, 2FA, audit logs and session recordings. Run WRM as one self-hosted binary, a Docker container or a Linux AppImage.

**Current version: v14.0.4** · [Download](https://github.com/vedranius/wrm-pro-enterprise/releases/latest) · [Release notes](RELEASE_NOTES.md) · [Changelog](CHANGELOG.md) · [Security](SECURITY.md) · [Architecture](ARCHITECTURE.md) · [Licence](LICENSE)

## What's new in v13.14.0

**The Git workspace checks only what Git tracks, folders sort themselves, and every file edit is on record.** The Git overview names servers by their hostname or connection name instead of an IP. A check compares only the files the catalog tracks: logs and other files outside the check are listed as *outside the check* and never compared, and the .gitignore helper has a **Logs** group. Every Git filter can narrow to a **folder on the servers** (a tree with counts, like the log viewer's folder filter), and the connection-folder filter includes subfolders. Right-click a folder to **sort its subfolders A–Z** or **keep them sorted** ([#134](https://github.com/vedranius/web-browser-RDM-public/issues/134)). *Admin → File edits* keeps **every file edit with its diff**, from the editor and from the AI, filterable by user, server, session and AI ([#129](https://github.com/vedranius/web-browser-RDM-public/issues/129)). SSH keys can log in with an **OpenSSH user certificate**, also the short-lived ones from **opkssh** ([#119](https://github.com/vedranius/web-browser-RDM-public/issues/119)). See [Connections & folders](#connections--folders), [SSH user certificates and opkssh](#ssh-user-certificates-and-opkssh), [File edit history](#file-edit-history) and [Git workspace: every file and partial checks](#git-workspace-every-file-and-partial-checks).

## What's new in v13.13.1

**Adding the WRM PRO marketplace in the Claude app.** Enter the repository in lowercase, `vedranius/wrm-pro-enterprise`: the Claude app names the marketplace after the repository, and GitHub accepts the name in any case. If the app still says only *Failed to add marketplace*, [Troubleshooting](#troubleshooting) says what to check and where its log keeps the reason; *Upload plugin* with `claude-plugin-wrm-pro.zip` from the release installs the same plugin.

## What's new in v13.13.0

**The WRM PRO plugin installs from the Claude app with one pick, and one script connects every AI app.** The plugin needs no settings any more: it carries the WRM bridge, and the first `/wrm-pro:wrm` asks for the WRM address and opens WRM in the browser, where you sign in and approve; the token goes straight to your computer, never into the chat. `/wrm-pro:wrm` then lists your servers and everything WRM can do. Before, the plugin depended on settings the Claude app cannot ask for, and its HTTP connector would have connected from the cloud, which cannot reach a WRM on your own network. *Settings → AI connections → 🧰 All AI apps at once* downloads one script (Windows or Linux/macOS) that signs in once and adds WRM to Claude Code, the Claude app, Codex, Gemini CLI, VS Code, Cursor, Windsurf and OpenCode. Releases carry `claude-plugin-wrm-pro.zip` for *Customize → Plugins → Upload plugin*. See [AI desktop apps (MCP)](#ai-desktop-apps-mcp).

## What's new in v13.12.0

**Tools for writing addons, also with an AI.** `wrm -addon-lint ./my-addon` checks an addon before you install it: the manifest against its schema, every file, and the code for what the sandbox would refuse (remote scripts, inline handlers, `eval`, `fetch`, `localStorage`, tokens in the ZIP, sites missing from `http.origins`, a panel that never says *ready*). Every finding has a code, the file, the line and a hint, so an AI can fix its own addon; `-addon-lint-json` gives the same as JSON, and a failed install in *Settings → Addons* opens the report by itself. `wrm -addon-new grafana-panel ./my-addon` starts from one of four templates (blank, fetch-and-render, grafana-panel, wasm-hook) that come with `wrm-bridge.js`, a small helper for the bridge. `wrm -addon-dev ./my-addon` opens the panel on your computer with the real sandbox and CSP, a stand-in for WRM (settings, web requests, secrets from environment variables) and a reload whenever a file changes. WRM also serves the manifest schema and an `llms.txt` guide for AI assistants. See [Addons](#addons) and the [addon developer guide](docs/ADDONS.md#tools-for-writing-addons).

## What's new in v13.11.0

**Addons can reach web sites, through WRM and only where an administrator said yes.** An addon now lists the sites it needs in `addon.json` (for example `https://grafana.example.com`, with the methods and paths it uses). WRM makes the request on the panel's behalf, so the panel itself still has no network: anything not listed is refused, every name is resolved by WRM and private or local addresses are blocked unless the administrator ticked *private network* for that site, and redirects are checked again. API keys and passwords for those sites are **secrets** that you type into WRM (🔑 *Secrets* on the instance): per instance, with your own value in a shared instance if you need one. They are stored encrypted, added to the request by WRM and never shown to the panel. A personal addon's sites wait for an administrator's approval. Recent requests are listed in the window's diagnostics. See [Addons](#addons) and the [addon developer guide](docs/ADDONS.md).

## What's new in v13.10.0

**Saved logins on demand, hostname names and a calmer top bar.** *Edit connection* has **👁 Show saved login**: after you confirm with your WRM password (or your domain password), WRM shows what the connection logs in with. Right-click a connection for **Copy username**, **Copy password** (same confirmation) and **Copy IP address**. A new connection can take its **name from the server's hostname**. Tabs show just the connection's name, the version next to the logo stays visible, the user name no longer jumps every 5 seconds, and the Git workspace server filter has no blank entries any more. See [Connections & folders](#connections--folders).

## What's new in v13.9.0

**Addon platform v2: your own addons, several windows each, and settings that stay.** Every user can now install **personal addons** that only they see (an administrator decides who may with `addon_personal` and can turn any of them off). An addon with a panel opens as **instances**, each with its own settings and window, for example two dashboards side by side; administrators can set up shared instances for everybody. Settings are kept **on the WRM server** with export and import, and windows come back where you left them after a reload or on another device. Addon **themes** appear in *Settings → Appearance* for whoever picks them. Before an addon is enabled WRM shows a **permission sheet** generated from `addon.json`; an update that asks for more waits for approval again. apiVersion 2 panels run without any network of their own, every file is pinned by SHA-256, and a watchdog offers *Reload* when a panel hangs. See [Addons](#addons) and the [addon developer guide](docs/ADDONS.md).

## What's new in v13.8.0

**AI sessions, your own AI settings and approvals that do what they say.** The new **🧠 AI sessions** panel lists every AI session you had, from the assistant, the log and file assistants, OpenCode agents and AI apps over MCP, with search (also in the conversation), filters, a transcript viewer, **Continue** (also an MCP session, whose steps become the earlier context), **Save** (kept beyond the transcript retention) and export to Markdown or JSON. Everybody sees only their own sessions. *⚙ My AI settings* in the assistant keep your own default provider (your own key, e.g. your OpenAI or Anthropic account), model, mode, engine and whether you see the model's thinking. Over MCP an approval given in WRM after the AI app stopped waiting now **runs the call at once** (`ai_mcp_later_approval`), *Deny* in the AI app is final, an app's open question is closed when you decide in WRM, and deciding a request that is no longer waiting says who decided it and where. The work log keeps the whole thinking text instead of its last lines. See [AI sessions](#ai-sessions) and [AI desktop apps (MCP)](#ai-desktop-apps-mcp).

## What's new in v13.7.0

**Themes, accessibility and a screen lock you can rely on.** *Settings → Appearance* offers eleven themes (dark, OLED black, Fjord, Solar, Forest, Ember, light Daylight, Paper, Solar light and two high-contrast themes), follows the system's light/dark setting if you want, and lets you change every colour, the fonts, the interface size, corners and spacing, save your own themes, export and import them, and add your own CSS. Themes from addons are picked there per user instead of being forced on everyone. For colour vision deficiency (deuteranopia, protanopia, tritanopia, achromatopsia) WRM switches status colours, diffs and optionally the terminal palette to colours you can tell apart, can add ✓ ✕ ! signs and shapes so nothing relies on colour alone, and offers more contrast, a terminal minimum contrast, reduced motion and transparency and a strong focus outline. *Settings → Security → Screen lock* sets when WRM locks (inactivity with a warning, a hidden window, the computer locking or sleeping, on every start, a keyboard shortcut) and how the lock screen looks; **Lock now** locks at once while terminals, transfers, tunnels, AI and MCP keep running. The lock is now kept by the server: a reload or a new tab stays locked, directory (LDAP) accounts can unlock, and a personal PIN can unlock instead of the password. See [Appearance, themes and accessibility](#appearance-themes-and-accessibility) and [Screen lock](#screen-lock).

## What's new in v13.6.0

**Log folders, a folder tree and smarter operators.** *Monitoring → Logs* can show the logs as a **folder tree** under their main folders (`/var/log`, `/home/scripts` …) and has a **Folder** filter for one folder and everything below it; *Log settings* take your own **main folders**. Operators are recognised from words that repeat in folder and service names (`nova`, `NOVA2`, `novatv` → one operator group), and every log says whether it sits in a log folder or an app folder. A log folder and a script folder with the same name (different case) no longer hide each other's logs. See [Logs: viewer, search and log settings](#logs-viewer-search-and-log-settings).

## What's new in v13.5.0

**A professional log viewer, a smart editor and an AI that shows its work.** *Monitoring → Logs* lists every log of a server in about a second (one read-only SSH command), sortable and filterable by operator and service, and never lists scripts such as `.py`. Logs open in tabs with level colours, smart highlighting, find, level filters and **Follow**; the **Search** tab greps all selected logs on the server for a From/To range in the EU date format and sorts the hits on a timeline. *Log settings* are now separate from Zabbix. The file manager's editor highlights syntax, finds and replaces, shows your changes before saving, warns when the file changed on the server and has an **AI assistant** that reviews and edits the open file with your approval. The log viewer has an AI log assistant (`search_logs`). The AI work log shows the assistant's thinking and each step, chats get a short title with date and time, and an ended chat can be **continued**. See [Logs: viewer, search and log settings](#logs-viewer-search-and-log-settings), [Text editor](#text-editor) and [AI assistant](#ai-assistant).

v13.0.0 to v13.4.4 (log engine, addons, screen lock, OpenCode Go, Claude quick start, Claude Desktop and Claude Code setup) are described in the [changelog](CHANGELOG.md).

## What's new in v12.9.1

**Medium runs without the original tool.** The WRM engine is now the default for projects nobody chose an engine for, its card comes first on the Medium page, and the original deploy tool with its Docker runtime is folded away as optional. Upgraded installations that already use the original tool keep it as their default until an administrator switches **Medium: default engine** to `native`. The deploy-config editor can now pull the configuration from its Git repository, show your local changes and **Commit and push** them through the WRM Git source, after checking that the whole configuration loads. WRM-host builds can run as an unprivileged account (`medium_build_user`) with a clean environment.

## What's new in v12.9.0

**Medium shows what changes in each file and lets you edit deploy-config.** In the WRM engine's plan, **Details** opens the server's version of a changed file against the branch, line by line. Files that differ only in CRLF/LF are marked *line endings only* and counted separately, as in the original tool. *Edit deploy-config* edits `servers.yaml`, `environments.yaml` and the project files in WRM: a file is saved only when the whole configuration still loads, and comments, layout and line endings stay as you wrote them.

## What's new in v12.8.1

**Security update without a database change.** The build toolchain is now Go 1.27.3, which fixes **GO-2026-6809 / CVE-2026-111943**: `crypto/x509` now rejects known compromised intermediate certificates that an older system trust store may still contain. The gosec G702 finding in the OpenCode Docker sandbox was reviewed and accepted as a false positive: the Docker path is operator-configured and the container name is generated by WRM, both passed as separate arguments without a shell. See the [release notes](RELEASE_NOTES.md) for the provenance of both findings.

## What's new in v12.8.0

**Folders now behave like a proper remote manager.** Deleting a folder removes its subfolders and every connection inside in one step, without moving anything to the root first. Folders and connections can be moved from the context menu or dragged, including several selected connections at once, and the new position is saved. Shared and collaboration trees keep their nested folder structure; connection names sort case-insensitively, while folders keep their manual order.

**Every reported detail is covered.** The Settings window opens at the size of the Admin window and wide tab rows have scroll arrows. Terminal windows have **Edit** and **Tunnels** buttons in the title bar. The file manager can change permissions (including recursively and with an optional sudo password), and the transfer destination can create a folder. Additional uploads can be queued while one is running. Tunnels can be filtered by port, description or host, with an active-only switch, and proxy settings are reachable from **Tools** and the Tunnels panel. Save buttons show progress and reject a second click, and **+ Connection** on a folder is pre-filled with that folder and its defaults.

## What's new in v12.7.4

**RDP works on Windows without preparing guacd by hand.** When guacd does not answer and its address is local (`127.0.0.1:4822`), WRM starts the official `guacamole/guacd:1.5.5` container in Docker Desktop, publishes it on loopback only, and keeps it running with Docker (`wrm-guacd`, `unless-stopped`). The first RDP/VNC/Telnet connection can start it; later starts happen with WRM. *Admin → Overview* shows the state and has **Start guacd**, `-setup-guacd` does the same from the command line, and the desktop window explains when Docker Desktop is missing or stopped. A custom guacd on another machine is never touched. Linux keeps using its system guacd or the `docker-compose.yml` service.

**Every text file previews with its formatting.** Preview now detects text from the file content, so shell/Python scripts, XML/SMIL, JSON, YAML, configuration files and extensionless files open as text instead of being refused. A bundled offline PrismJS highlighter colors the formats it knows; binary files still get the clear answer to download them. The 256 KiB preview limit and escaped-text safety policy are unchanged.

## What's new in v12.7.3

**Medium builds where you choose.** Projects with a build step can now deploy with the **WRM native** engine. For each project, choose where the build runs: on the WRM host, on a chosen Linux build server over SSH, or not at all. The plan shows the place and the build's platform and architecture before anything runs; a destination with a different platform or architecture is refused with the reason. A build server receives only the prepared source in a temporary directory (no Git metadata and no SSH keys), and the directory is removed afterwards.

**Monitoring → Logs finds every service.** Service logs are now discovered under `/home/<operator>/<operator>/<service>/log` and `/logs`, with the correct Service and Operator labels. Non-compressed service logs, including FTP `.txt` files, are listed; virtual environments, build/archive directories and old backups are skipped. Discovery and listing have larger time budgets, so large hosts no longer show incomplete results.

## What's new in v12.7.2

**The version is read from the code.** The Git workspace no longer trusts `VERSION.md` to say what is deployed. It compares every tracked file on the server with the repository tree at the target and at the earlier tags: all files matching one version show that version, a consistent older version is named exactly from its commit tree, and a mix or a hand-edited file shows **Mixed / modified** with the per-file details. A new checkbox next to *Check now* (off by default) also checks `VERSION.md` and warns when it disagrees. See [Git workspace: every file and partial checks](#git-workspace-every-file-and-partial-checks).

## What's new in v12.7.1

**Medium: per-environment server lists.** `environments.yaml` now accepts the original deploy tool's `servers: [name, ...]` key, so an environment can be limited to chosen servers. A project target in that environment cannot deploy to any other server; an unknown, duplicate or empty list is refused with the file and key named. Existing configs without the key keep every server. See [Medium native engine](#medium-native-engine).

## What's new in v12.7.0

**Medium deploys natively.** Switch a Medium project to the **WRM native** engine and WRM deploys it itself, from your WRM Git source to every server of the environment through your saved WRM connections, without Docker and without the original tool. Each server gets a lock, a backup of everything that changes, a SHA-256 check of every uploaded file, `post_deploy` and a history entry, all in the original tool's format, so both engines can take turns on the same servers and roll back each other's deployments. A **dry run** compares again under the lock and changes nothing; changes can be **left out** of a deployment; **History and rollback** shows what runs where, rolls back the last deployment and releases a lock someone left behind. Projects with a build step gained native builds in v12.7.3. See [Medium native engine](#medium-native-engine).

## What's new in v12.6.26

**SIEM export, security alerts and lockdown.** WRM sends every audit entry to your **SIEM**: syslog over TLS (RFC 5424), a JSON lines file, Splunk HEC, Elastic, Microsoft Sentinel or any HTTPS collector, with user, role, directory identity, source address and the audit hash chain, and catches up after an outage. The admin sees per destination whether it delivers and what is waiting. **Security alerts** tell administrators about repeated failed sign-ins, break-glass sign-ins, new administrators, revealed secrets, AI full-access approvals and the audit being turned off. **Stop user** ends everything an account has open in one click, and **lockdown** keeps everyone except break-glass administrators out, switches AI off and pauses shares, from the UI, `wrm -lockdown` or `WRM_LOCKDOWN=1`. See [SIEM export, alerts and lockdown](#siem-export-alerts-and-lockdown).

## What's new in v12.6.25

**Active Directory, LDAP and SCIM.** People sign in with their **Active Directory or LDAP** name and password on the normal sign-in page; WRM checks the password with the directory over LDAPS or StartTLS and never stores it. Rules turn directory groups (nested AD groups included) into WRM roles and teams, and WRM **checks every linked account regularly**: someone deleted, disabled, locked or removed from the WRM groups loses access at once. **SCIM 2.0** lets Microsoft Entra ID or Okta create, change and disable WRM accounts and groups immediately. Whichever way an account is stopped, WRM ends its sessions, terminals, tunnels, rooms, AI sessions and AI connections. See [LDAP / Active Directory and SCIM](#ldap--active-directory-and-scim).

## What's new in v12.6.24

**Sign in with your organisation's account.** WRM now signs people in through **Microsoft Entra ID, ADFS, Okta, Keycloak, Google Workspace or any OpenID Connect provider** (authorization code with PKCE). In *Admin panel → Directory sign-in* you add the provider, test the connection step by step, and write rules: an Entra **app role** or a **group ID** gives a WRM role and teams. Accounts are created on the first sign-in only for people a rule matches; everyone else is refused with a message that says why. People are recognised by their immutable directory ID, never by their name, so a renamed or re-used name cannot take over an account. A provider can require **MFA proven by the token**. WRM **checks regularly** with the provider that each signed-in user may still sign in: someone disabled or removed from the WRM group loses their sessions and terminals. Password sign-in can be limited to the **break-glass** administrators (the Enterprise level requires it once a directory sign-in is on). **Test sign-in** shows what the provider sends and what WRM would do, without creating anything. See [Directory sign-in (OIDC)](#directory-sign-in-oidc).

## What's new in v12.6.23

**Roles, teams and a second confirmation for risky actions.** Every account now has a role: *Administrator*, *Security auditor* (reads the audit log, terminal sessions, recordings, file transfers and the security posture, and changes nothing), *Operator* or *User*. Administrators create **teams** in *Admin panel → Teams*; a share can include a whole team with one role, and policies that say who may use a feature accept `operators` and `teams:<names>` besides `off / admins / all`. With **step-up** on (required from the Business level), changing policies, roles, the security level, deleting or disabling users, resetting passwords or 2FA and revealing stored secrets ask for your password (and 2FA code) again. A **break-glass** administrator account must use 2FA, can be limited to chosen networks, and every sign-in is audited and sent to the other administrators. When you delete a user, their connections, folders, credentials, keys, shares and proxies can **pass to their team lead or another account** instead of being deleted. See [Roles, teams, step-up and break-glass](#roles-teams-step-up-and-break-glass).

## What's new in v12.6.22

**Security levels: Home, Business, Enterprise.** Pick how strict WRM is on the first-run screen, at install (`-security-level`, `WRM_INITIAL_SECURITY_LEVEL`, forced with `WRM_SECURITY_LEVEL`) or later in **Admin panel → Security**. A level sets minimums for HTTPS, two-factor authentication, passwords, sessions, registration, host keys, secret export, guest links, AI full access, audit and session recording. Before you switch, a preview shows every change, what blocks it and whom it affects; a setting you keep below the level is a **deviation** with a written reason, on record and audited. The **security posture** page grades the installation item by item with the exact fix, and exports a report for auditors (JSON or PDF). See [Security levels and posture](#security-levels-and-posture).

## What's new in v12.6.21

**Signed releases.** Every release from now on carries an ed25519 signature of its `SHA256SUMS.txt`, a software bill of materials (CycloneDX) and GitHub build provenance. Once the project's public key is built in, the one-click update refuses a release whose signature is missing or wrong, so a changed file on the release page cannot be installed. `wrm -release-verify SHA256SUMS.txt` checks a download by hand. Every night, fuzz tests feed random input to the import parsers (mRemoteNG, SSH config, CSV/XLSX, PuTTY, WinSCP, configuration files, deploy-config YAML) and OWASP ZAP scans a running WRM, signed out and as an administrator. The fuzz tests already found and fixed a crash in the PuTTY import.

## What's new in v12.6.20

**Security checked on every change.** WRM is now built with a supported Go release (1.27) and up-to-date libraries, and every push and pull request runs automatic security checks: known vulnerabilities in Go and the libraries WRM uses (govulncheck), GitHub CodeQL for the server and the web UI, gosec and staticcheck, secret scanning (gitleaks), checks of the build workflows themselves, a dependency review, a vulnerability scan of the Docker image and the OpenSSF Scorecard. Dependabot proposes library updates every week, and Claude can review pull requests for security problems. The Medium proxy uses Go's safer `Rewrite` hook. No change for users; see [Continuous security checks](SECURITY.md#continuous-security-checks) and the [security roadmap](ROADMAP.md) (signed releases, security levels for home, business and enterprise, roles and teams, sign-in with Microsoft Entra ID / AD, LDAP and SCIM, SIEM export).

## What's new in v12.6.11

**OpenCode inside WRM.** Add the **OpenCode Zen** provider and use OpenCode's free models in the AI assistant without an account, or paste an OpenCode account's Zen key for all its models. A **Go subscription** has its own **OpenCode Go** provider, so a Go key is sent to Go's endpoints instead of Zen. Choose **OpenCode agent** as the assistant's engine (agents *build* and *plan*): WRM downloads OpenCode itself after one click by an administrator (sha512 checked) and runs it on the WRM host with its own shell, file and web tools switched off, so it works on your servers only through WRM and every command still waits for your approval. The assistant can also hand text jobs to **helper models**, picked automatically by the kind of job (free first), each labelled with how it treats your data; models that may train on prompts stay off until you switch them on. See [OpenCode Zen, Go, agents and helper models](#opencode-zen-go-agents-and-helper-models).

## What's new in v12.6.10

**One canvas, arranged your way.** The window area is one canvas: terminals, file managers, desktops, Workboard addons and panels share it. Addons are windows you move, resize, snap, pin or dock; panels can be placed anywhere on the canvas, not only at the edges. The **▤ Workboard** menu adds addons and holds the canvas tools: grid, tile and cascade, lock, saved layouts (optionally with your terminals) and export / import. New addons: links, saved sessions and a timer. The top bar is tidier: *Broadcast* is in **🧰 Tools**, *Shares* is an icon, and **Settings · Panels · Workboard** sit together. A closed terminal's banner has a **✕ Close window** button. See [Workboard: the shared canvas and addons](#workboard-the-shared-canvas-and-addons).

## What's new in v12.6.9

**Workboard: what matters, at a glance.** **▤ Workboard** in the top bar opens a page of tiles you put together yourself from an addon catalog, so nothing appears until you add it: server status, CPU / RAM / disk bars of chosen servers (from Monitoring, or read over SSH), WRM's notifications inside WRM, clocks for your time zones, the last commits of a Git service, running tunnels, quick connect buttons and notes. Any tile can be pulled out into a panel and docked, stacked or floated like the other panels. See [Workboard and addons](#workboard-the-shared-canvas-and-addons).

## What's new in v12.6.8

**Your panels, where you want them.** *Connections*, *Sessions*, *Snippets*, *Clipboard* and the AI live view are panels with a tab you can drag: onto another panel's tab bar to stack them as tabs, onto the top or bottom part of a panel to split the column, onto an edge strip to dock it left, right or under the windows, or anywhere else to let it float. **Panels** (▦) in the top bar shows and hides them, and **Lock the layout** keeps everything in place. The layout is stored for your account on the server, so it follows you to every browser. See [Panel layout](#panel-layout-dock-stack-float-and-lock).

## What's new in v12.6.7

**A top bar that fits and an AI panel that docks.** The top bar now fits at any width and browser zoom: labels go first, then less used buttons move into a **⋯** menu, so nothing overlaps. The AI live view (what an AI app runs on your servers, with approvals and output) no longer covers your windows: it is a **🤖 AI** tab next to *Sessions* and *Snippets*, a dock under the windows, a floating window, or hidden, chosen with its title bar buttons or by dragging it to an edge, and remembered per user. Approval cards still always pop up. See [AI desktop apps (MCP)](#ai-desktop-apps-mcp). Every text on the page can be selected and copied again, and Medium's Docker runtime trusts your computer's certificates, so it works behind proxies and antivirus that inspect HTTPS (see [Docker for Medium](#docker-for-medium-installed-by-wrm)).

## What's new in v12.6.6

**Docker everywhere, installed by WRM.** The original deploy tool now runs in WRM's managed Docker runtime on Linux, macOS and Windows alike, and WRM installs Docker itself when it is missing: an administrator presses **Install Docker** in Medium step 2 once (accepting Docker Desktop's licence on Windows and macOS), and WRM installs Docker Engine from the Linux distribution's packages or Docker Desktop from docker.com (signature checked), starts it, waits for it and prepares the runtime, every step with its output in the activity log. Afterwards a tool ZIP import does all of it by itself. Where WRM has no administrator rights it shows the one command to run, `sudo wrm -setup-docker`. See [Docker for Medium](#docker-for-medium-installed-by-wrm).

## What's new in v12.6.5

**Medium tells you what it is doing.** The Medium view now has a numbered **step-by-step guide** for the original deploy tool (1 import the tool ZIP, 2 prepare its runtime, 3 your deploy-config, 4 your SSH profile, 5 start/stop), each with a light, what it waits for and what to do when it fails. While something runs you see **what runs, who started it, since when and every step with its time**, with the output of Docker, pip and the original tool (secrets redacted). A deploy-config ZIP is checked by WRM at once and kept for the next start while the runtime is still being prepared, so you no longer wait to upload it. A new **activity log** lists every Medium action with its result, duration, steps and output, and WRM's console logs each step. The read-only WRM preview is a separate, clearly labelled panel. See [Medium step by step](#medium-step-by-step-status-and-activity-log).

## What's new in v12.6.4

**Your blocked commands, your choice.** With full access on, the administrator now decides whether their own list of blocked commands (`ai_blocked_commands`) asks for approval like everything else (default) or stays blocked even with full access (`ai_full_access_blocked`, *Admin panel → AI connections*). See [AI desktop apps (MCP)](#ai-desktop-apps-mcp).

## What's new in v12.6.3

**Full access with one switch.** The administrator now chooses how full access applies (`ai_full_access_mode`): by default the **Full access for the AI** switch alone opens it for every AI session; it can instead apply only to AI connections with the **⚠ Full access** scope, or to users who turn it on for themselves in *Settings → AI connections*. Claude still asks before anything WRM would otherwise refuse. See [AI desktop apps (MCP)](#ai-desktop-apps-mcp).

## What's new in v12.6.2

**Full access, but Claude still asks.** When an administrator allows it (`ai_full_access`), an AI connection can get the **⚠ Full access** scope. WRM then refuses nothing outright: destructive and blocked commands, deny lists, shutdown and secret files come to you as an approval marked *FULL ACCESS*, every time and never automatically. Read-only connections stay read-only. See [AI desktop apps (MCP)](#ai-desktop-apps-mcp).

## What's new in v12.6.1

**"Allow everything" now means everything.** In an AI connection's automatic mode, **Allow everything** fills in `*` for commands and `/**` for files, and **No time or action limit** keeps the connection automatic for as long as it is valid. Chains with `&&`, variables and redirections no longer stop at an approval when `*` is allowed; blocked commands, your deny lists and sensitive files still do. When WRM does ask and nobody answers in time (or the app declines the question), the request stays in WRM to **approve later**, and the AI repeats it once you have. Files that may hold secrets are read after your approval, with secrets redacted. See [AI desktop apps (MCP)](#ai-desktop-apps-mcp).

## What's new in v12.6.0

**Medium's native engine, first step.** The Medium view has a read-only *Native engine* panel: WRM reads your `deploy-config` as it is, matches its servers to your saved WRM connections, prepares the chosen branch or tag from your WRM Git source and shows what a deployment would change on each server, without Docker and without writing anything. **Compare with the original tool** runs the original comparison next to it and shows whether both give the same result. Deploying natively and rollback followed in v12.7.0. See [Medium native engine](#medium-native-engine).

## What's new in v12.5.1

**Claude Desktop and Claude Code in one step.** *Settings → AI connections → Claude Desktop* now downloads a **WRM PRO extension** (`wrm-pro.mcpb`) with your WRM address and token already filled in: open it, press **Install**, done. It works with a WRM on your own network and over plain HTTP, because Claude Desktop talks to WRM from your computer (a claude.ai custom connector connects from Anthropic's cloud and needs a public HTTPS address). For Claude Code there is a **WRM PRO plugin**: `claude plugin install wrm-pro --marketplace vedranius/wrm-pro-enterprise --config wrm_url=… --config wrm_token=…`. Token lifetimes are entered in hours or days and default to 7 days (at most 30), so you are not asked to approve again every day. While an AI app works on a server, a **live panel** in WRM shows each command, the approval buttons and the output; after a denial the AI asks you how to go on. See [AI desktop apps (MCP)](#ai-desktop-apps-mcp).

## What's new in v12.5.0

**Stamp as it is:** the Stamp dialog shows which files differ and why an installation cannot be stamped. Tick **Also stamp installations that are not up to date** to write VERSION.md with each file's actual version and a note that the installation is partial.

**Back in dialogs:** dialogs opened from another dialog (for example Details → Update → confirmation) have **← Back** (Alt+←) and keep what you ticked or typed. A code review of the v12.4.x features fixed production confirmations, Medium start/stop/settings and imports, log search with non-UTF-8 bytes and special characters, and the client GitLab cache. The next versions run Medium without Docker on WRM's own engine ([roadmap](ROADMAP.md)).

## What's new in v12.4.5

**Medium:** select the trusted tool/config ZIPs and click **Prepare and run**. WRM retains the imports, starts Docker, prepares the original dependencies, validates config, synchronizes the private profile and opens the unchanged original tool. First Windows setup can invoke the signed official Docker installer; OS/vendor prompts and required restarts remain visible. Existing custom configuration Git settings are replaced by the explicitly uploaded local config ZIP.

**Monitoring → Logs:** discovery starts automatically and streams results by server. Service log folders and Supervisor paths supplement manual roots. Filter by server/operator/service and event time, then search a keyword across all matching files from their beginning. Results show file modification time, matching event range, highlighted excerpts and open/follow actions. Progress, permission errors and partial searches remain visible; continue a bounded scan when necessary. Host metrics, service checks and SSH logs have separate setup requirements.

Overview/Installations keep filters, check controls and actions together while scrolling. Environments adds combined search/service/environment/server filters.

[Operation and limits](docs/V12.4.5_GUIDE.md) · [Validation](docs/RELEASE_SUMMARY_v12.4.5.md).

## What's new in v12.4.4

Monitoring with Zabbix metrics/history and service Functionality, multi-window logs, client GitLab token access with persistent snapshots, easier MCP setup and editable grants, structured configuration inventory, terminal selection-to-AI/Open terminal here, visual folder/connection labels, longer provider timeouts, sticky filters and replay/import fixes.

[Operation and limits](docs/V12.4.4_GUIDE.md) · [Release validation](docs/RELEASE_SUMMARY_v12.4.4.md) · [Original Medium tool parity](docs/MEDIUM_PARITY_v12.4.3.md). General configuration import now recognizes mRemoteNG XML and PuTTY registry files ([#5](https://github.com/vedranius/web-browser-RDM-public/issues/5)).

## 📜 License: Apache License 2.0

WRM is **free software** under the **Apache License 2.0**, starting with **v14.0.0**. You may use, modify and redistribute it, including commercially, as long as you keep the [LICENSE](LICENSE) and [NOTICE](NOTICE) files and state your changes. The licence includes an express patent grant. It does **not** grant trademark rights — see [TRADEMARK.md](TRADEMARK.md).

| | |
|---|---|
| ✅ **Use in any project** | Personal, commercial and SaaS use, modification and redistribution, under the terms of Apache-2.0. |
| ✅ **Companies** | Companies of any size may run WRM on their own servers and use it for their customers' work. |
| 🔄 **Earlier versions** | Releases up to **v13.x** were published under the **PolyForm Noncommercial 1.0.0** / **PolyForm Internal Use 1.0.0** terms. Copies obtained under those terms keep the rights those terms granted. The earlier code, history and tags live in the predecessor repository [`web-browser-RDM-public`](https://github.com/vedranius/web-browser-RDM-public). See [LICENSE-CHANGE.md](LICENSE-CHANGE.md). |
| 🤝 **Contributions welcome** | Ideas, bug reports, translations and pull requests. Contributions need a CLA and a `Signed-off-by` line. See [CONTRIBUTING.md](CONTRIBUTING.md). |

Copyright © 2026 vedranius and WRM contributors. Third-party libraries keep their own licences; each release ships a CycloneDX SBOM.

## ☕ Support the project

WRM is built in my spare time. If it saves you time, you can buy me a coffee:

[![Support me on Ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/vedranius)

**https://ko-fi.com/vedranius**. Thank you! ❤️

---

## Table of contents

1. [What WRM is](#what-wrm-is)
2. [Quick start](#quick-start)
3. [Upgrading from v13.13.1](#upgrading-from-v13131) · [from v13.13.0](#upgrading-from-v13130) · [from v13.12.0](#upgrading-from-v13120) · [from v13.11.0](#upgrading-from-v13110) · [from v13.10.0](#upgrading-from-v13100) · [from v13.9.0](#upgrading-from-v1390) · [from v13.8.0](#upgrading-from-v1380) · [from v13.7.0](#upgrading-from-v1370) · [from v13.6.0](#upgrading-from-v1360) · [from v13.5.0](#upgrading-from-v1350) · [from v13.4.4](#upgrading-from-v1344) · [from v12.9.0](#upgrading-from-v1290) · [from v12.8.1](#upgrading-from-v1281) · [from v12.7.1](#upgrading-from-v1271) · [from v12.7.0](#upgrading-from-v1270) · [from v12.6.26](#upgrading-from-v12626) · [from v12.6.25](#upgrading-from-v12625) · [from v12.6.24](#upgrading-from-v12624) · [from v12.6.23](#upgrading-from-v12623) · [from v12.6.22](#upgrading-from-v12622) · [from v12.6.21](#upgrading-from-v12621) · [from v12.6.20](#upgrading-from-v12620) · [from v12.6.19](#upgrading-from-v12619) · [from v12.6.11](#upgrading-from-v12611) · [from v12.6.10](#upgrading-from-v12610) · [from v12.6.9](#upgrading-from-v1269) · [from v12.6.8](#upgrading-from-v1268) · [from v12.6.7](#upgrading-from-v1267) · [from v12.6.6](#upgrading-from-v1266) · [from v12.6.5](#upgrading-from-v1265) · [from v12.6.4](#upgrading-from-v1264) · [from v12.6.3](#upgrading-from-v1263) · [from v12.6.2](#upgrading-from-v1262) · [from v12.6.1](#upgrading-from-v1261) · [from v12.6.0](#upgrading-from-v1260) · [from v12.5.1](#upgrading-from-v1251) · [from v12.5.0](#upgrading-from-v1250) · [from v12.4.5](#upgrading-from-v1245) · [from v12.4.4](#upgrading-from-v1244) · [from v12.4.3b](#upgrading-from-v1243b) · [from v12.4.3](#upgrading-from-v1243) · [from v12.4.0](#upgrading-from-v1240) · [from v12.3.6](#upgrading-from-v1236) · [from v12.2.8](#upgrading-from-v1228) · [from v12.2.7](#upgrading-from-v1227) · [from v12.2.6](#upgrading-from-v1226) · [from v12.2.5](#upgrading-from-v1225) · [from v12.2.4](#upgrading-from-v1224) · [from v12.2.3](#upgrading-from-v1223) · [from v12.2.2](#upgrading-from-v1222) · [from v12.2.1](#upgrading-from-v1221) · [from v12.2.0](#upgrading-from-v1220) · [from v12.1.0](#upgrading-from-v1210) · [from v12.0.0](#upgrading-from-v1200) · [from v11.7.0](#upgrading-from-v1170) · [from v11.6.1](#upgrading-from-v1161) · [from v11.6.0](#upgrading-from-v1160) · [from v11.5.0](#upgrading-from-v1150) · [from v11.4.1](#upgrading-from-v1141) · [from v11.4.0](#upgrading-from-v1140) · [from v11.3.0](#upgrading-from-v1130) · [from v11.2.0](#upgrading-from-v1120) · [from v11.1.0](#upgrading-from-v1110) · [from v11.0.0](#upgrading-from-v1100) · [from v10.10.0](#upgrading-from-v10100) · [from v10.9.1](#upgrading-from-v1091) · [from v10.9.0](#upgrading-from-v1090) · [from v10.8.1](#upgrading-from-v1081) · [from v10.8.0](#upgrading-from-v1080) · [from v10.7](#upgrading-from-v107) · [from v10.6](#upgrading-from-v106) · [from v10.5](#upgrading-from-v105) · [from v10.4](#upgrading-from-v104) · [from v10.3](#upgrading-from-v103) · [from v10.2](#upgrading-from-v102) · [from v10.1](#upgrading-from-v101) · [from v10.0](#upgrading-from-v100) · [from v9](#upgrading-from-v9)
4. [How it works](#how-it-works)
5. [Features in detail](#features-in-detail)
   - [Accounts, sign-in & two-factor authentication](#accounts-sign-in--two-factor-authentication)
   - [Connections & folders](#connections--folders)
   - [Quick connect & connection notes](#quick-connect--connection-notes)
   - [SSH keys & credentials vault](#ssh-keys--credentials-vault)
   - [SSH user certificates and opkssh](#ssh-user-certificates-and-opkssh)
   - [Jump hosts (bastions)](#jump-hosts-bastions)
   - [Proxies (SOCKS / HTTP)](#proxies-socks--http)
   - [SSH tunnels (port forwarding)](#ssh-tunnels-port-forwarding)
   - [Web interfaces (HTTP / HTTPS connections)](#web-interfaces-http--https-connections)
   - [Tags & inventory import (CSV, Excel, NetBox)](#tags--inventory-import-csv-excel-netbox)
   - [Import from mRemoteNG, PuTTY and OpenSSH](#import-from-mremoteng-putty-and-openssh)
   - [Windows, tabs & snapping](#windows-tabs--snapping)
   - [Panel layout: dock, stack, float and lock](#panel-layout-dock-stack-float-and-lock)
   - [Appearance, themes and accessibility](#appearance-themes-and-accessibility)
   - [Screen lock](#screen-lock)
   - [Workboard: the shared canvas and addons](#workboard-the-shared-canvas-and-addons)
   - [Addons](#addons)
   - [SSH terminal](#ssh-terminal)
   - [Remote desktop: RDP, VNC, Telnet](#remote-desktop-rdp-vnc-telnet)
   - [Out-of-band management, consoles & serial ports](#out-of-band-management-consoles--serial-ports)
   - [Snippets & run on connect](#snippets--run-on-connect)
   - [Folder bookmarks](#folder-bookmarks)
   - [Broadcast input](#broadcast-input)
   - [Live up/down status](#live-updown-status)
   - [Logs: viewer, search and log settings](#logs-viewer-search-and-log-settings)
   - [Notifications (e-mail, chat, push, webhook)](#notifications-e-mail-chat-push-webhook)
   - [Git workspace: service functionality and configuration](#git-workspace-service-functionality-and-configuration)
   - [Git workspace: compare services](#git-workspace-compare-services)
   - [Git workspace: update, upgrade, rollback & restarts](#git-workspace-update-upgrade-rollback--restarts)
   - [Git workspace: tab explanations](#git-workspace-tab-explanations)
   - [Git workspace: Simple mode](#git-workspace-simple-mode)
   - [Git workspace: environments, deploy plans & safe deploys](#git-workspace-environments-deploy-plans--safe-deploys)
   - [Git workspace: activity, deploy history & what is where](#git-workspace-activity-deploy-history--what-is-where)
   - [Git workspace: new servers (install & transfer)](#git-workspace-new-servers-install--transfer)
   - [Git workspace: .gitignore helper](#git-workspace-gitignore-helper)
   - [Git workspace: CI integration (optional)](#git-workspace-ci-integration-optional)
   - [Git workspace: every file and partial checks](#git-workspace-every-file-and-partial-checks)
   - [AI assistant](#ai-assistant)
   - [AI desktop apps (MCP)](#ai-desktop-apps-mcp)
   - [Network tools](#network-tools)
   - [File manager (SFTP / FTP / FTPS)](#file-manager-sftp--ftp--ftps)
   - [Search in files](#search-in-files)
   - [Text editor](#text-editor)
   - [Server-to-server transfer](#server-to-server-transfer)
   - [Workspace sessions](#workspace-sessions)
   - [Clipboard panel](#clipboard-panel)
   - [Sharing: members, roles & links](#sharing-members-roles--links)
   - [Real-time collaboration](#real-time-collaboration)
   - [Voice calls](#voice-calls)
   - [Settings](#settings)
   - [Audit log, session recording & file transfers](#audit-log-session-recording--file-transfers)
   - [File edit history](#file-edit-history)
   - [Admin panel](#admin-panel)
   - [Security levels and posture](#security-levels-and-posture)
   - [Roles, teams, step-up and break-glass](#roles-teams-step-up-and-break-glass)
   - [Directory sign-in (OIDC)](#directory-sign-in-oidc)
   - [LDAP / Active Directory and SCIM](#ldap--active-directory-and-scim)
   - [SIEM export, alerts and lockdown](#siem-export-alerts-and-lockdown)
   - [Updates & self-update](#updates--self-update)
   - [Mobile, responsive UI & installable app](#mobile-responsive-ui--installable-app)
6. [Keyboard shortcuts](#keyboard-shortcuts)
7. [Configuration](#configuration)
8. [Docker, service, reverse proxy & firewall](#docker-service-reverse-proxy--firewall)
9. [Security model](#security-model)
10. [Limitations](#limitations)
11. [API reference](#api-reference)
12. [Building from source & releases](#building-from-source--releases)
13. [Troubleshooting](#troubleshooting)
14. [Contributing](#contributing)

---

## What WRM is

WRM is a **single executable** with a built-in web server and a built-in web app. You start it on any machine: your PC, a Raspberry Pi, a VPS or a company server. Then you open it in a browser and manage your remote servers from there:

- **SSH terminals** in the browser (xterm.js, 256 colors, full-screen apps like `nano`, `vim`, `htop`, `mc`), with **snippets** (saved commands with variables, one click or Ctrl+Shift+Space), **run on connect** (`sudo -i`, `cd /srv/app`…) and **broadcast input** (type into many terminals at once, with a safety check for dangerous commands).
- **Live up/down status** of every saved connection in the sidebar, with latency, server banner and a notification when a server goes down.
- **Quick connect** — type `root@10.0.0.5:22` (or `rdp://…`, `vnc://…`) and connect without saving; **notes & runbooks** per connection (procedures, contacts, links); **network tools** — port check, ping, traceroute, DNS and HTTP/TLS check from the WRM server or **from any of your servers** ("can app-01 reach the database on 5432?").
- **Out-of-band management** — power on/off/restart, health and inventory through the server's **BMC** (iDRAC, iLO, XClarity, Supermicro, OpenBMC via **Redfish**, or **IPMI**), and its **serial console** (BMC SSH or IPMI Serial-over-LAN) in a terminal window; **serial ports** of the WRM machine for switch and router consoles.
- **Remote desktops** — **RDP** (Windows), **VNC** and **Telnet** in a browser window, also behind jump hosts, with clipboard, Ctrl+Alt+Del, recording and replay (through guacd, the Apache Guacamole proxy).
- **File manager** for **SFTP** (over SSH), **FTP** and **FTPS**: browse, upload, download, rename, delete, edit, search.
- **SSH keys & credentials vault**: generate SSH keys, put them on many servers at once (like `ssh-copy-id`), see **who has access** to a server; one shared login such as `root@dc1` for many connections, shared with colleagues without revealing it, with **password rotation** on all its servers.
- **Jump hosts** (like `ssh -J`): reach servers behind a bastion, also in chains — for terminals, files, transfers and tunnels. **Proxies** (SOCKS5/4, HTTP CONNECT) as a connection setting, like in PuTTY, also combined with jump hosts; **folder defaults** for the jump host and proxy of everything in a folder.
- **Bookmarks** for deep directories (like WinSCP): per server, per folder or tag, or global, with variables such as `{name}` — one click opens the directory in the file manager or `cd`s there in the terminal.
- **Notifications** when a server goes down or comes back, a password is due for rotation or a new service version is out: in WRM, in the browser, by e-mail, Telegram, Slack / Mattermost, Microsoft Teams, Discord, ntfy, Gotify, Pushover or a signed webhook.
- **SSH tunnels** (like `ssh -L / -R / -D`, PuTTY, mRemoteNG): reach the web interface of a switch, an iDRAC/iLO or a database behind a server; a SOCKS proxy into a whole management network. **Web interface connections** open such pages with one double-click.
- **Import** your server inventory from **CSV / Excel** files and **NetBox** (with tags for environment, site and rack, and sync), and your connections from **mRemoteNG** (with passwords, folders and SSH tunnels), **PuTTY** (with proxies) and **OpenSSH** `~/.ssh/config`. **Tags** filter the sidebar and mark production servers.
- **Git workspace**: which version of which service runs where — compare installations on your servers **file by file** with **GitLab / GitHub** or an **offline bundle**, see what is *old*, *missing* or *changed by hand*, with diffs and notifications — and **update, upgrade or roll back** them over SSH (backups, atomic writes, checks, automatic rollback, opt-in restarts, maintenance windows), deploy to **environments** of ordered servers with a reviewed one-shot plan, a dry run, a server-side lock, history and rollback per deploy, or **install them on a new server** and **transfer** them between servers with their per-host configuration; a **.gitignore helper**, and optional **CI integration** (GitLab / GitHub / Jenkins webhooks, bundles from CI artifacts, deploy through a pipeline).
- **AI desktop apps (MCP):** connect Claude (desktop or claude.ai), ChatGPT or any other MCP client to WRM — with OAuth or a personal access token, or through the small stdio bridge in the WRM binary. The app gets time-limited access to the servers, mode and scopes you choose; every call goes through the same permission engine, changes wait for your approval (in WRM or in the app), an **AI connected** indicator shows the session and revokes it with one click.
- **AI assistant** next to the terminal and the file manager: ask about a server and the assistant reads logs, services, packages and files through WRM's own tools. **Read-only**, **ask before every change** (the exact command or a diff, approved, edited or denied by you) or **automatic within limits** (an allow list, a time and an action limit you opt into). Destructive commands are always refused; administrators decide which modes, providers and models are allowed per connection, folder or tag, and every request, tool call and approval is audited and recorded. Works with the Anthropic API, OpenAI, Azure OpenAI, AWS Bedrock, Google Vertex AI and OpenAI-compatible local models, with the organisation's key or your own.
- **Server-to-server copy** between two SSH servers, without downloading to your computer first.
- **Workspaces**: many terminal and file windows side by side, tabs, snapping, saved sessions. **Installable as an app** (PWA) on desktops, tablets and phones.
- **Sharing with roles**: give colleagues or guests access to some connections — as *Observer*, *Viewer*, *Operator* or *Moderator* — without ever revealing the passwords.
- **Live collaboration**: chat with history, file exchange, **voice calls** (like Discord/Jitsi), terminal sharing with snapshots, remote keyboard control on request, raised hands and moderation.
- **Enterprise security**: two-factor authentication, closed registration, admin-managed accounts, password & session policies, account lockout, audit log (with CSV export and SIEM-friendly log lines), SSH host-key verification, encrypted secrets, CSRF protection and strict security headers.

Everything is stored in one local **SQLite** file. There is no external database, no Docker requirement, no CDN and no cloud account: the binary also serves all scripts and fonts, so WRM works in closed networks.

**Supported platforms (prebuilt binaries):** Linux (x86-64, arm64, ARMv7/Raspberry Pi, ARMv6, 32-bit), Windows (x86-64, arm64), macOS (Intel, Apple Silicon, Universal), Android arm64 (Termux), FreeBSD (x86-64, arm64), OpenBSD (x86-64).

---

## Quick start

1. Download the binary for your system from **[Releases](https://github.com/vedranius/wrm-pro-enterprise/releases/latest)** and optionally verify it: put `SHA256SUMS.txt` and `SHA256SUMS.txt.sig` next to it and run `wrm-pro-… -release-verify SHA256SUMS.txt` (signature and SHA-256), or compare the SHA-256 yourself. `gh attestation verify <file> --repo vedranius/wrm-pro-enterprise` shows which workflow and commit built it.
2. Run it:

   **Linux / macOS / FreeBSD / OpenBSD**
   ```bash
   chmod +x wrm-pro-v14.0.0-linux-amd64
   ./wrm-pro-v14.0.0-linux-amd64
   ```
   On macOS, if Gatekeeper blocks the file: `xattr -d com.apple.quarantine wrm-pro-*-darwin-*`.

   **Windows** (PowerShell), or just double-click the `.exe`:
   ```powershell
   .\wrm-pro-v14.0.0-windows-amd64.exe
   ```

   **Linux AppImage (x86_64 / ARM64)**
   ```bash
   chmod +x wrm-pro-v14.0.0-linux-amd64.AppImage
   ./wrm-pro-v14.0.0-linux-amd64.AppImage
   # On ARM64 use wrm-pro-v14.0.0-linux-arm64.AppImage instead.
   ```
   Open **http://localhost:8080**. The AppImage runs in a terminal and binds to localhost by default; use `PORT` or `LISTEN_ADDR` to change that. Stop it with Ctrl+C. If FUSE is unavailable, run with `--appimage-extract-and-run`.

   Data is stored in `${XDG_DATA_HOME:-$HOME/.local/share}/wrm-pro/`, or at your explicit `DB_PATH`. Replace the AppImage to upgrade; the data and encryption key stay in place. For a boot service use the standalone Linux binary with `-install-service`.

   **Android (Termux)**
   ```bash
   pkg install wget
   wget https://github.com/vedranius/wrm-pro-enterprise/releases/download/v14.0.0/wrm-pro-v14.0.0-android-arm64
   chmod +x wrm-pro-v14.0.0-android-arm64 && ./wrm-pro-v14.0.0-android-arm64
   ```

   **Docker**
   ```bash
   git clone https://github.com/vedranius/wrm-pro-enterprise.git && cd wrm-pro-enterprise
   docker compose up -d          # https://<host>:8080 (self-signed certificate)
   ```
   See [Docker](#docker) for details.
3. Open **http://localhost:8080** (or `http://<server-ip>:8080`). For voice calls from other computers use HTTPS — the quickest way is `HTTPS_SELF_SIGNED=1` (see [Configuration](#configuration)).
4. **Create the administrator account** (the first account). After that, self-registration is **closed**: the administrator creates accounts in *Admin panel → Users* (or opens registration in *Security policies*).
5. Click **+ Connection**, enter host, user and password or key, then double-click the connection to open it.

On the first start WRM creates, next to the database:

| File | What it is |
|---|---|
| `remote_manager.db` | SQLite database (users, connections, shares, audit log). Mode `0600`. |
| `remote_manager.db.key` | Random 256-bit key that encrypts stored passwords, private keys and 2FA secrets. Mode `0600`. **Back it up separately** — without it stored secrets cannot be decrypted. Or set your own key with `ENCRYPTION_KEY` / `ENCRYPTION_KEY_FILE`. |
| `recordings/` | Terminal session recordings (`YYYY/MM/<id>.cast.gz`, mode `0600`), created when the first session is recorded. Location: `WRM_RECORDINGS_DIR`. |

Locked out? `./wrm-pro-… -reset-password admin` prints a new temporary password (add `-reset-2fa` to also turn off two-factor authentication).

---

## Upgrading from v13.13.1

From v13.13.0, also read the next section. Replace the executable/AppImage/container and keep the database and encryption key. Additive migration: a new table `file_edits` (append-only like the audit log, kept for `audit_retention_days`) and new columns `folders.auto_sort` and `ssh_keys.certificate`; a previous version still starts on the database. New settings `file_edit_history` (`1`) and `file_edit_max_kb` (`1024`). New endpoints `GET / POST /api/folders/sort` and `GET /api/admin/file-edits[/{id}]`; `PUT /api/keys/{id}` also takes `certificate`. Git checks no longer compare files the catalog does not track: their state is *outside the check* instead of *same* / *differs*, and log files (`*.log`, `*.log.*`, files below a `log` or `logs` folder) are never compared nor deployed unless the catalog's include list names them. A key file on the WRM server with a `<key>-cert.pub` next to it now logs in with that certificate first.

## Upgrading from v13.13.0

Replace the executable/AppImage/container and keep the database and encryption key. No database change; a previous version still starts on the database. The WRM PRO Claude plugin is now 1.2.1 (icon path only). If adding the marketplace failed in the Claude app, add it again with the lowercase repository name `vedranius/wrm-pro-enterprise`.

## Upgrading from v13.12.0

Replace the executable/AppImage/container and keep the database and encryption key. No database change; a previous version still starts on the database. New endpoint `POST /api/mcp/setup-all` (audited as `mcp.setup_script`). The WRM PRO Claude plugin 1.2.0 no longer has the settings `wrm_url` and `wrm_token`: after updating the plugin, run `/wrm-pro:wrm` (or `/wrm-pro:connect`) once and approve the sign-in in the browser; drop `--config` from scripts that install it. A Claude Desktop extension downloaded from this version keeps running when its token is refused and offers `wrm_connect`.

## Upgrading from v13.11.0

Replace the executable/AppImage/container and keep the database and encryption key. No database change; a previous version still starts on the database. New command-line options `-addon-lint`, `-addon-lint-json`, `-addon-new`, `-addon-id`, `-addon-dev`, `-addon-dev-port` and `-addon-dev-private`, and new endpoints under `/api/addons/dev/`. The addon ids `dev`, `install`, `drafts`, `community` and `users` are now reserved; an installed addon with one of those ids keeps working but cannot be installed again under that id.

## Upgrading from v13.10.0

Replace the executable/AppImage/container and keep the database and encryption key. Additive migration: new tables `addon_http_grants`, `addon_secrets` and `addon_http_log`; a previous version still starts on the database (it ignores them, and addons with an `http` block were not installable there). New policy `addon_http` (`all`): who may use addons that reach web sites (`off`, `admins`, `all`). Existing addons are unchanged; an addon gets network access only after an update that declares `http` and a new approval.

## Upgrading from v13.9.0

Replace the executable/AppImage/container and keep the database and encryption key. No database change; a previous version still starts on the database. Git workspace installations whose connection was deleted are no longer listed, and deleting a connection now removes its installations. *Show saved login* and *Copy password* follow the existing *Allow export of passwords & keys* policy (`allow_secret_export`, on by default; administrators always can) and the step-up policy.

## Upgrading from v13.8.0

Replace the executable/AppImage/container and keep the database and encryption key. Additive migration: new tables `addons_personal`, `addon_instances`, `addon_store` and `addon_windows` and new columns `files`, `approved` and `pending` in `addons`; a previous version still starts on the database (it ignores personal addons, instances and stored values). Installed addons keep working; their files are pinned by SHA-256 the next time they are installed. New policy `addon_personal` (`all`): set it to `admins` or `off` if users must not install their own addons.

## Upgrading from v13.7.0

Replace the executable/AppImage/container and keep the database and encryption key. Additive migration: a new column `saved` in `ai_sessions` and an index; a previous version still starts on the database. New settings `ai_mcp_later_approval` (`run`: an approval given in WRM after the AI app stopped waiting runs the call at once; `repeat` keeps the earlier behaviour, where the AI must repeat the call) and `ai_keep_saved_sessions` (`1`). *Deny* in an AI app's approval question is now a final denial; closing the question without an answer still keeps the request in WRM. New endpoints: `GET /api/ai/sessions?view=all`, `POST /api/ai/sessions/{id}/save`, `GET /api/ai/sessions/{id}/export`, and the key `ai` of `/api/me/ui/{key}`. A decision on a request that is no longer waiting answers `409` with `decided`.

## Upgrading from v13.6.0

Replace the executable/AppImage/container and keep the database and encryption key. Additive migration: new columns `locked_at` and `lock_failures` in `auth_sessions` and `lock_pin_hash` in `users`; a previous version still starts on the database. New settings `screen_lock_mode` (`server`), `screen_lock_max_minutes` (`0`), `screen_lock_pin` (`1`), `screen_lock_max_failures` (`0`), `appearance_default_theme` (`graphite`) and `appearance_custom_css` (`1`); the Business and Enterprise security levels require `screen_lock_mode=server` and an idle lock of at most 30 / 15 minutes, and Enterprise turns the PIN off. **Two behaviour changes:** a locked browser session can no longer reach the API until it is unlocked (set `screen_lock_mode` to `remember` or `browser` for the looser behaviour), and a colour theme from an addon is no longer applied to everyone: each user picks it in *Settings → Appearance*, or the administrator makes it the default (`appearance_default_theme=addon:<addon>:theme`). The accent colour chosen in *Settings → General* before v13.7 is carried over into the theme once.

## Upgrading from v13.5.0

Replace the executable/AppImage/container and keep the database and encryption key. No migration: the log settings gain `main_folders` (empty) and `guess_operators` (on) inside the existing per-server JSON, and a previous version ignores them. Labels of logs under a configured main folder are taken from it before the default patterns.

## Upgrading from v13.4.4

Replace the executable/AppImage/container and keep the database and encryption key. Additive migration: a new table `log_settings` (per connection, encrypted) and new columns `title`, `purpose`, `resumed_from` and `file_path` in `ai_sessions`; a previous version still starts on the database. Log folders and journal units configured in the Zabbix dialog are read into the new *Log settings* until you save them there. New settings `ai_show_thinking` and `ai_auto_titles` (both `1`). Dates are shown in the EU format by default; pick another in *Settings → Date and time format* (per browser).

## Upgrading from v12.9.0

Replace the executable/AppImage/container and keep the database and encryption key. No database change. New policies: `medium_default_engine` (`native`) and `medium_build_user` (empty). If the original deploy tool was already imported, WRM sets `medium_default_engine=original` once at the first start and logs it, so projects keep their engine; set it to `native` in *Admin → Policies* to deploy with WRM by default. A project with an engine chosen explicitly keeps it. Pulling deploy-config from Git needs `DEPLOY_CONFIG_REPO` in the Medium settings and a WRM Git source for that repository (with a write token for commit and push).

## Upgrading from v12.8.1

Replace the executable/AppImage/container and keep the database and encryption key. No database change. Editing deploy-config in WRM uses the existing `medium_native` policy (default `admins`); users without it see the files read-only.

## Upgrading from v12.7.1

Replace the executable/AppImage/container and keep the database and encryption key. No database change. The Git check reads the version from the code by default; `VERSION.md` is only compared when the new checkbox is on. Deployments and updates still write `VERSION.md`. The first check after the upgrade may need a target refresh to load the older version trees; until then the per-file history is used.

## Upgrading from v12.7.0

Replace the executable/AppImage/container and keep the database and encryption key. No database change. Existing `deploy-config` files keep working; an environment may now add `servers: [name, ...]` to limit its projects to those servers. Importing a config ZIP still replaces the config directory and removes its Git history, so use Git for changes once the directory is a clone.

## Upgrading from v12.6.26

Replace the executable/AppImage/container and keep the database and encryption key. The database gets the table `medium_project_engine`; a previous version still starts on it (projects switched to the native engine are then deployed by the original tool again, which reads the same server state). Every project stays on the Docker runtime until someone allowed by the new policy `medium_native` (default `admins`) switches it to *WRM native*. New settings `medium_command_timeout` (300) and `medium_transfer_timeout` (3600). With step-up on, native deployments, rollbacks and lock releases ask for a confirmation (`medium.deploy`, `medium.rollback`, `medium.lock_release`).

## Upgrading from v12.6.25

Replace the executable/AppImage/container and keep the database and encryption key. The database gets the table `siem_destinations`; a previous version still starts on it (it does not enforce a lockdown, so end a lockdown before going back). Nothing is exported until an administrator adds a SIEM destination. New settings `alert_failed_signins` (10), `alert_failed_signins_minutes` (10), `alert_break_glass`, `alert_new_admin`, `alert_secret_reveal`, `alert_ai_full_access` and `alert_audit_off` (all on); new notification event `security.alert`; new command line flags `-lockdown` / `-lockdown-reason` and environment variable `WRM_LOCKDOWN`. Security events (`security.*`) are now recorded even with `audit_enabled` off.

## Upgrading from v12.6.24

Replace the executable/AppImage/container and keep the database and encryption key. The database gets the tables `ldap_directories`, `ldap_identities`, `scim_clients`, `scim_users`, `scim_groups` and `scim_group_members`; a previous version still starts on it (accounts that sign in with a directory password then cannot sign in, since WRM keeps no password for them). Nothing changes until an administrator adds an LDAP directory or a SCIM client. New setting `ldap_sync_minutes` (15). Changing an OIDC provider's issuer now asks for its client secret again.

## Upgrading from v12.6.23

Replace the executable/AppImage/container and keep the database and encryption key. The database gets the tables `oidc_providers` and `user_identities` and the columns `users.sso_only`, `auth_sessions.identity_id` and `team_members.source`; a previous version still starts on it (directory accounts then cannot sign in, since they have no password). Nothing changes until an administrator adds an identity provider. New settings `local_login` (`all`), `oidc_revalidate_minutes` (60) and `oidc_unverified_hours` (12). The Enterprise level now requires `local_login` = `break_glass`; it takes effect only while a directory sign-in is enabled, so nobody is locked out of an installation without one. Releases from this version on are signed and published automatically when a version is merged.

## Upgrading from v12.6.22

Replace the executable/AppImage/container and keep the database and encryption key. The database gets the tables `teams`, `team_members` and `share_teams` and the columns `users.role`, `users.break_glass` and `auth_sessions.step_up_at`; a previous version still starts on it (it sees administrators as before, and auditors and operators as ordinary users). Existing accounts keep their rights: administrators stay administrators, everyone else is a *User*. New settings `require_step_up` (off; the Business and Enterprise levels require it, so an installation at one of those levels shows it in the security posture until it is turned on), `step_up_minutes` and `break_glass_ips`. Scripts that call `PUT /api/admin/settings`, change roles or delete users with step-up on get `403 {"step_up": true}` unless the session confirmed with `POST /api/auth/step-up` (signing in counts as a confirmation for `step_up_minutes`). `DELETE /api/admin/users/{id}` still deletes everything the user owns unless `?transfer_to=<user id>` is given.

## Upgrading from v12.6.21

Replace the executable/AppImage/container and keep the database and encryption key. The database gets one new table, `security_deviations`; a previous version still starts on it. Existing installations stay at the **Home** level and no setting changes until an administrator picks another level in *Admin panel → Security* (or sets `WRM_INITIAL_SECURITY_LEVEL` / `WRM_SECURITY_LEVEL`). New settings `security_level`, `require_https` and `link_share_max_days`; new endpoints `GET /api/admin/security` and `POST /api/admin/security/level`. Loosening a policy below the chosen level now needs a reason (`_deviation_notes` in `PUT /api/admin/settings`); scripts that set settings at the Home level are not affected.

## Upgrading from v12.6.20

Replace the executable/AppImage/container and keep the database and encryption key. No database change. New setting `update_signing_keys` (empty: only the built-in key). This version has no built-in release key yet, so its self-update still verifies the SHA-256 only and the update dialog says *Signature: not checked*; the version that adds the key turns the signature check on. New command-line options `-release-keygen`, `-release-sign`, `-release-verify`.

## Upgrading from v12.6.19

Replace the executable/AppImage/container and keep the database and encryption key. No database or setting change. Building from source now needs **Go 1.26 or newer** (`go.mod` asks for the Go 1.27.2 toolchain, which `go` downloads by itself when `GOTOOLCHAIN` allows it).

## Upgrading from v12.6.11

Replace the executable/AppImage/container and keep the database and encryption key. No database change. Nothing changes until an administrator sets `ai_opencode_sandbox` (`admins` or `all`) and presses **Download and install** again in *Admin panel → AI assistant → OpenCode agents*, which prepares the Docker sandbox. New settings `ai_opencode_sandbox` and `ai_opencode_sandbox_image`; `POST /api/ai/sessions` takes `sandbox: true`.

## Upgrading from v12.6.10

Replace the executable/AppImage/container and keep the database and encryption key. The database gets one new column, `users.ai_delegate_training` (a user's own choice to allow helper models that may train on prompts); a previous version still starts on it. Nothing changes until an administrator adds an **OpenCode Zen** provider, turns on delegation (`ai_delegate`) or installs OpenCode in *Admin panel → AI assistant → OpenCode agents*. `ai_provider_kinds` now includes `opencode`; if you set it yourself, add `opencode` to allow the new provider. New settings `ai_delegate*` and `ai_opencode*`; new endpoints `/api/ai/delegate` and `/api/admin/opencode`. OpenCode itself is stored next to the database in `<DB>.opencode/`.

## Upgrading from v12.6.9

Replace the executable/AppImage/container and keep the database and encryption key. No database change. The Workboard is now the canvas itself: your v12.6.9 tiles open as addon windows beside your terminals the first time (from the top right), and tiles you pulled out into panels stay where they are. *Broadcast* moved from the top bar into **🧰 Tools**. `/api/me/ui/{key}` now accepts values up to 256 KiB.

## Upgrading from v12.6.8

Replace the executable/AppImage/container and keep the database and encryption key. The database gets one new table, `user_events` (each user's in-app notifications, newest 200); a previous version still starts on it and ignores it. Nothing changes on the page until you open **▤ Workboard** and add an addon. New policy `workboard_ssh_metrics` (on): turn it off if the *Server load* addon must not log in to servers over SSH; servers with Monitoring still show their values. New endpoints `/api/me/events` and `/api/workboard/metrics`.

## Upgrading from v12.6.7

Replace the executable/AppImage/container and keep the database and encryption key. The database gets one new table, `user_ui` (your panel layout); a previous version still starts on it and ignores it. Nothing moves: your first layout is built from what the browser remembered in v12.6.7 (the right panel's tab and width, where the AI live view was docked, floating or hidden). New endpoint `/api/me/ui/{key}`.

## Upgrading from v12.6.6

Replace the executable/AppImage/container and keep the database and encryption key. No database or API change. The AI live view now opens as a **🤖 AI** tab in the right panel instead of a box at the bottom left; pick *Floating* in its title bar (or the 🤖 menu) for the old behaviour. Medium's Docker runtime now trusts the WRM host's root certificates (policy `medium_host_ca`, default on); if runtime preparation failed with *certificate verify failed*, press *Prepare runtime* again.

## Upgrading from v12.6.5

Replace the executable/AppImage/container and keep the database and encryption key. No database change. Medium now uses the managed Docker runtime by default on Linux and macOS too. An installation whose host runtime (Python virtual environment) is already prepared keeps it; set `WRM_MEDIUM_RUNTIME=docker` to move it to Docker, or `native` to keep the host runtime for good. Nothing is installed until an administrator presses **Install Docker** in Medium step 2; the new policy `medium_docker_install` (default on) switches that button and the automatic installation off.

## Upgrading from v12.6.4

Replace the executable/AppImage/container and keep the database and encryption key. One additive table (`medium_activity`); a previous binary still starts on the database. `POST /api/git/medium/start` now answers `202` and starts in the background (poll `GET /api/git/medium`), and a deploy-config or SSH ZIP uploaded while Medium is busy or not prepared yet is kept and applied at the next start (`202`, `pending`). Command output is shown in the Medium activity log to the user who ran it and, for the shared runtime, to every Medium user; set `medium_log_output` to `admins` or `none` to limit that.

## Upgrading from v12.6.3

Replace the executable/AppImage/container and keep the database and encryption key. No database change. Blocked commands keep asking under full access until an administrator sets `ai_full_access_blocked` to `deny` (*Admin panel → AI connections → Blocked commands with full access*).

## Upgrading from v12.6.2

Replace the executable/AppImage/container and keep the database and encryption key. One additive column (`users.ai_full_access`); a previous binary still starts on the database. If `ai_full_access` was already on, it now applies to every AI session (`ai_full_access_mode` = `all`, the default); set the mode to *AI connections with the Full access scope* (`connection`) in *Admin panel → AI connections* to keep the v12.6.2 behaviour, or to *Users who turn it on themselves* (`user`).

## Upgrading from v12.6.1

Replace the executable/AppImage/container and keep the database and encryption key. No database change. Full access stays off until an administrator turns on `ai_full_access` (*Admin panel → AI connections*); then edit an AI connection and tick **⚠ Full access**. Open sessions of that connection end and the app reopens them with the new rules.

## Upgrading from v12.6.0

Replace the executable/AppImage/container and keep the database and encryption key. No database change. Existing AI connections keep their limits. To let an app work without limits, edit its AI connection in *Settings → AI connections*, press **Allow everything** (it also ticks **No time or action limit**), confirm and save; open sessions end and the app opens them again with the new rules. Administrators can forbid connections without limits with `ai_auto_until_grant` = off.

## Upgrading from v12.5.1

Replace the executable/AppImage/container and keep the database, encryption key and Medium directories. One additive table (`medium_server_links`) is created at start; a previous binary still starts on the database. The native preview uses your existing WRM Git sources and SSH connections and writes nothing to servers. Medium deployments keep running through the original tool.

## Upgrading from v12.5.0

Replace the executable/AppImage/container and keep the database and encryption key. No database change. AI connections (MCP) stay off by default; once an administrator turns them on (*Admin panel → AI connections*), **Claude Desktop** users pick *Claude Desktop* in *Settings → AI connections*, download the extension and install it. An old `claude_desktop_config.json` entry with the stdio bridge keeps working. Token lifetimes can now be chosen in **hours or days**. The defaults change from 8 hours (at most 24) to **7 days (at most 30 days)**: `ai_mcp_token_hours` 168, `ai_mcp_max_token_hours` 720. A value an administrator saved explicitly stays in force; lower them again in *Admin panel → AI connections* if your policy needs short lifetimes.

## Upgrading from v12.4.5

Replace the executable/AppImage/container and keep the database, encryption key and private Medium directories/volumes. No database migration. Medium settings saved empty now mean "original default" and are no longer passed to the tool; earlier `.previous` copies of replaced SSH profiles are deleted at the next Medium start. Stamp no longer refuses silently: choose **Also stamp installations that are not up to date** to stamp an installation as it is.

## Upgrading from v12.4.4

Replace the executable/AppImage/container, preserving your database, encryption key and private runtime directories/volumes. No database migration is needed. In Medium use the combined ZIP fields and **Prepare and run**; empty fields reuse existing imports. Setup failure retains approved archives for retry. Existing original-tool deployment history and behavior are retained.

In Monitoring select **Logs** and a server. Discovery uses the same SSH account without sudo; it finds known service `log`/`logs` directories, bounded configured service roots and Supervisor log directories. Manual roots remain editable from the log toolbar. Date/timezone filters and keyword search read historical segments across matching logs, rather than only their recent tail. Progress and bounds remain visible. Zabbix is needed for host metrics, not SSH logs.

Windows first setup can require Docker/WSL license, OS permissions, virtualization setup or a restart on the WRM host. A Windows service cannot display a desktop installer: install Docker once for its account, then resume. Docker in Windows-container mode is not switched automatically because other containers may be using it. See the [guide](docs/V12.4.5_GUIDE.md).

## Upgrading from v12.4.3b

Replace the executable/container with v12.4.4 and retain the DB, encryption key and private runtime directories/volumes. Migrations are additive. Existing explicit policy values remain in force; MCP is still off by default. Functionality moves to **Monitoring → Functionality**. Configure each server's agent transport/log roots; automatic metrics polling is opt-in.

Client GitLab cache requires a client token and browser CORS permission or the companion on the computer that can access GitLab. Native Medium's Git/build subprocesses still need their direct route. [v12.4.4 setup, limits and troubleshooting](docs/V12.4.4_GUIDE.md).

## Upgrading from v12.4.3

Download the v12.4.3b Windows executable, stop WRM and replace the previous executable while retaining its database and encryption key. **The v12.4.3 updater cannot parse the `b` suffix, so this first hotfix needs a manual download.** v12.4.3b adds letter-hotfix support for subsequent checks/updates.

For Windows 10/11, install/start [Docker Desktop with Linux containers](https://docs.docker.com/desktop/setup/install/windows-install/) under the account running WRM. In **Git → Medium**, import the original tool ZIP or, if it is already imported, click **Prepare runtime / retry**. WRM pulls the pinned Linux image and installs the original lockfile in a Linux volume. Import your deploy-config and SSH profile and start Medium. Standard `~/.ssh/key` references work inside its private Linux home. No Windows Python/rsync installation is needed. Browser clients need no deploy tools.

Docker Desktop is not supported on Windows Server: run WRM/Medium on a supported Linux host or Linux VM/container and access it from the Windows browser. A remote Docker daemon is not supported by this bind-mount/localhost bridge. Docker engine access must belong to the account running WRM; installing Docker for a different desktop account does not grant a Windows service access automatically.

Back up the database/key and private `.medium` source/config/preferences **plus** Docker volumes named `wrm-medium-<instance>-runtime` and `wrm-medium-<instance>-u<user>`. Stop Medium before backing up user volumes. Runtime source/venv are read-only; user volumes retain native edits/history across restarts. Re-importing config/SSH replaces that profile explicitly without deleting deploy history. Existing native Linux runtime data is copied into a new user volume on its first Docker start. Source and SSH keys remain private.

Linux keeps its native runtime when dependencies exist; use `WRM_MEDIUM_RUNTIME=docker` to select the managed runtime or `native` to select native explicitly. Windows always selects Docker. Custom Docker paths/images use `WRM_MEDIUM_DOCKER` and `WRM_MEDIUM_CONTAINER_IMAGE`. The Windows CLI is also discovered at standard Docker Desktop installation paths without requiring a PATH refresh. Additional build toolchains/users may require a trusted custom Linux image; the pinned image uses its unprivileged `wrm` account for builds. Unix host-agent forwarding remains a native-runtime option; managed Docker uses the private SSH profile.

## Upgrading from v12.4.0

Replace the binary/AppImage or update the container. Additive migrations create `git_config_profiles` and add `folders.sort_order`. Existing folders retain alphabetical order until reordered. Back up the database, matching encryption key and adjacent `.medium` / `.bitwarden` directories; they contain private runtime/configuration data. Older binaries ignore the new fields, but cannot use the new features.

Medium is enabled for administrators by default. Import the original deploy-tool ZIP once as an administrator, then each operator imports their deploy-config and optional SSH profile. The Docker image includes Python 3.12+, git, OpenSSH, rsync and the official Bitwarden CLI. Standalone binaries keep these dependencies optional: install them on the WRM host before enabling the corresponding feature. Medium uses the original engine's own plans, locks, backups and history; it does not import those into Simple/Advanced runs.

The earlier client GitLab relay/replay and AI/navigation/color/icon requests are delivered in v12.4.4; see the [operation guide](docs/V12.4.4_GUIDE.md).

## Upgrading from v12.3.6

Replace the binary or use the in-app updater; update the AppImage/container as appropriate. Startup adds a nullable folder parent and the `git_server_health` cache table. Existing folders, catalogs and route defaults keep their settings; previous binaries ignore the new fields.

The new **Functionality** view is available from Git Simple and Advanced. Run a Git check to populate it, and enable periodic monitoring/notification subscriptions if desired. Runtime commands read service state with the SSH login's permissions; no services are started or restarted. For AI availability, open **Settings → AI assistant**; provider connectivity and effective account permission are shown separately.

## Upgrading from v12.2.8

Replace the binary. One additive table (`git_hostnames`) is created automatically; v12.2.8 still starts on the same database. No configuration changes.

- **Git Simple mode is automatic.** Services and environments the checker found on your servers appear as ready cards without defining environments per service; **Save detected environments** stores them in the catalog (audited). Advanced is unchanged and your own definitions keep priority.
- **Servers are shown by hostname** (read once over the existing SSH login and cached, falling back to the connection host) instead of the connection name.

## Upgrading from v12.2.7

Replace the binary. No database or configuration changes; v12.2.7 still starts on the same database.

- The **version label in the top bar** ("WRM v12.2.8") now opens the **Updates** dialog, so an administrator can always press **Check now**, even when no update is known and the green badge is hidden. Non-administrators see the versions only.

## Upgrading from v12.2.6

Replace the binary. One additive table (`ai_ollama_runs`) is created automatically; v12.2.6 still starts on the same database. No configuration changes.

- The **AI provider wizard** has presets for xAI, DeepSeek, Together AI, Fireworks AI, Cerebras, Cohere and Perplexity, refreshed model suggestions and a **Load the model list** button (the provider's `/models`). Models can always be typed freely.
- **Ollama, step by step:** detection, status, installed models, per-OS install / background-service / download commands and, optionally, **running them through WRM**. The last is off by default: an administrator enables the policy `ai_ollama_manage` (`WRM_AI_OLLAMA_MANAGE=1`). Each run needs a confirmation, is permission-checked on the server and written to the audit log (`admin.ai_ollama_*`). See the configuration table and the API reference below.

## Upgrading from v12.2.5

Replace the binary. No database or configuration changes; v12.2.5 still starts on the same database.

- **Settings → AI connections** now has a step-by-step **wizard** for connecting Codex CLI, Claude Code, Gemini CLI, Cursor / VS Code, Claude and ChatGPT: it creates the token, shows (and lets you download) the ready command or config file and tests the connection. The AI assistant has a matching **provider wizard**. See *Connect an AI app* below.
- The MCP server explains what "wrm" is to its clients, and permission denials now say which scope is missing and how to fix it.

## Upgrading from v12.2.4

Replace the binary. No database or configuration changes; v12.2.4 still starts on the same database.

- The **AI assistant window** inside a terminal window can now be docked left / right / top / bottom, floated, moved, resized and expanded over the whole terminal window. See *AI assistant* below.

## Upgrading from v12.2.3

Replace the binary. No database or configuration changes; v12.2.3 still starts on the same database.

- The **session recording replay** window can now be resized (drag its bottom-right corner) and expanded to **fullscreen** (⛶ button, **Esc** leaves). The terminal is scaled to the window width, so there is no sideways scrolling. See *Session recording* below.

## Upgrading from v12.2.2

Replace the binary. No database or configuration changes; v12.2.2 still starts on the same database.

- Every tab of the Advanced Git workspace, of Settings and of the Admin panel now has a short explanation of what it is for. See [Git workspace: tab explanations](#git-workspace-tab-explanations).

## Upgrading from v12.2.1

Replace the binary. No database or configuration changes; v12.2.1 still starts on the same database.

- The Git workspace has a **Simple / Advanced** switch in its header. **Simple** shows one guided card per service environment (what runs on the server vs. the latest version on the branch, a clear status) with **Install / Update / Deploy again** and **Roll back to the previous version**. **Advanced** is the unchanged Git workspace and stays the default. See [Git workspace: Simple mode](#git-workspace-simple-mode).

## Upgrading from v12.2.0

Replace the binary. No database or configuration changes.

- New terminal windows open **maximized** with the cursor in the terminal; windows of a saved session keep their layout.
- Selecting more text than fits on the screen now copies the **whole selection** (it is copied when you release the mouse).
- A click or double-click on the title bar of a maximized window no longer shrinks it; only dragging restores it.

## Upgrading from v12.1.0
Replace the binary. There are no database changes; v12.1.0 still starts on the same database.

- **Gemini and other OpenAI-compatible providers** work with tools: the provider's `extra_content` (Gemini's `thought_signature`) of each tool call is stored in the session and sent back unchanged, which fixes *HTTP 400: Function call is missing a thought_signature*.
- **Temporary provider errors** no longer end the turn at once: HTTP 408 / 429 / 5xx (for example 503 *high demand*) are retried up to three times with a back-off, and a request that is too large for the provider (HTTP 413, or a token limit such as Groq's) shortens the long command output of the conversation and is retried. The assistant shows a short notice for both.

## Upgrading from v12.0.0
Replace the binary. The database gets new tables (`mcp_tokens`, `mcp_clients`) and the column `ai_sessions.mcp_token_id`; v12.0.0 still starts on it and ignores them.

- **AI desktop apps (MCP)** are **off by default**. An administrator turns them on in *Admin panel → AI connections* (`ai_mcp_enabled`); by default only administrators may create AI connections (`ai_mcp` = `admins`), with the modes `read_only,ask,auto` (`ai_mcp_modes`), a default lifetime of 8 hours (`ai_mcp_token_hours`) and at most 24 hours (`ai_mcp_max_token_hours`; since v12.5.1 the defaults are 7 and 30 days). Users then find *Settings → AI connections*. See [AI desktop apps (MCP)](#ai-desktop-apps-mcp).
- MCP clients reach WRM at `https://<your WRM>/mcp`. Hosted connectors (claude.ai, ChatGPT) connect **from the provider's cloud**: WRM must be reachable from the internet over HTTPS for them. Behind a reverse proxy set `ai_mcp_public_url` (the address the apps use) and pass `/mcp`, `/oauth/` and `/.well-known/` through. Desktop apps with the stdio bridge only need to reach WRM from your computer.
- The kill switch of the AI assistant (`ai_kill_switch`) also stops every AI connection. Turning off the built-in assistant (`ai_assistant`) does not turn off MCP, and the other way round.
- The top bar shows **🤖 AI connected** while an AI app is connected; at narrower widths the top-bar labels hide a little earlier than before so nothing overlaps.

## Upgrading from v11.7.0

Replace the binary. The database gets new tables (`ai_providers`, `ai_sessions`, `ai_messages`) and the column `users.ai_blocked`; v11.7.0 still starts on it and ignores them.

- **AI assistant** (off by default). An administrator turns it on in *Admin panel → AI assistant* (`ai_assistant` = `admins` or `all`) and adds the organisation's provider (Anthropic, OpenAI, Azure OpenAI, AWS Bedrock, Google Vertex AI or an OpenAI-compatible endpoint). Users then get an **AI** button in the title bar of SSH terminals and file managers. See [AI assistant](#ai-assistant).
- New modes are restrictive by default: `ai_modes` = `read_only,ask` (the automatic mode must be allowed explicitly), personal API keys are off (`ai_personal_keys`), output is redacted before it is sent to the provider (`ai_redact_output`).
- **Consumer chat subscriptions (Claude.ai, ChatGPT) cannot be used** by WRM or any other third-party application: use an API key, an enterprise gateway, or the MCP integration of v12.1.0 ([AI desktop apps](#ai-desktop-apps-mcp)).
- AI sessions appear in *Sessions & recordings* with an **✦ AI** mark (the transcript is recorded like a terminal) and in the audit log as `ai.*` events.
- The top bar no longer overlaps the *New window* button at about 1100 px.

## Upgrading from v11.6.1

Replace the binary (or use the new one-click update from the next release on); the database does not change.

- **Run as a service:** on an interactive start WRM asks once whether to install itself as a service that starts at boot (systemd, launchd, rc.d or a Windows service; *Don't ask again* is remembered in `<DB_PATH>.service.json`). Scripts use `-install-service`, `-uninstall-service` and `-service-status`; `-no-service-prompt` skips the question. See [Service](#service). `sudo wrm -setup-docker [-service-user ACCOUNT]` (Linux) installs Docker Engine for Medium, lets the WRM account use it and restarts the WRM service.
- **Updates:** WRM checks the project's GitHub releases once a day (no user data is sent). The version in the top bar shows **⬆ Update vX.Y.Z** with the release notes; administrators update with one click (SHA-256 checked against `SHA256SUMS.txt`, previous binary kept for a rollback). Air-gapped installations: `WRM_UPDATE_CHECK=off`. Docker never replaces itself. See [Updates & self-update](#updates--self-update).
- **📂 in an SSH terminal** is now a menu: *Open here*, *Open /*, *Open home*. "Here" works without configuring OSC 7. See [Windows, tabs & snapping](#windows-tabs--snapping).
- A systemd unit that runs WRM with `ProtectSystem=strict` needs the binary's folder in `ReadWritePaths` for the one-click update (units written by `-install-service` have it).

## Upgrading from v11.6.0

Replace the binary; the database does not change.

- **📂 in the title bar of SSH terminals** opens the file manager of the same server, in the terminal's current directory when the shell reports it (OSC 7) ([#24](https://github.com/vedranius/web-browser-RDM-public/issues/24)). See [Windows, tabs & snapping](#windows-tabs--snapping).

## Upgrading from v11.5.0

Replace the binary; the database does not change.

- **Activity tab** in the Git workspace with three views — *Git activity* (pushes and commits per branch and author from the Git server), *Deploy history* (the `history.jsonl` of every environment destination merged with the WRM runs of installations without environments) and *What is where* (branch, commit, version, who and when per environment and server, commits behind the branch head, drift). Each view has a **CSV export** of what its filters show. See [Git workspace: activity, deploy history & what is where](#git-workspace-activity-deploy-history--what-is-where).
- **Gitea sources:** a Git source can now be a Gitea server (API v1, `Authorization: token …`). Targets, comparisons, deploys and the activity work as with GitHub; a .gitignore pull request is not available for Gitea yet (download the file).
- The activity reads the provider API with the source's **read-only token**; for GitHub repository activity (pushes, force pushes) the token needs read access to the repository's metadata, otherwise only commits are shown. The deploy history and the matrix read the destinations over SSH like the *Environments* tab and need the `git_checks` policy; without it only WRM's own records are shown.
- No new settings or policies.

## Upgrading from v11.4.1

Replace the binary. The database gets one new table (`git_env_exclusions`); the previous binary still starts on it and ignores it. Catalogs and installations without environments work exactly as before.

- **Environments (optional):** a service of the catalog can get named environments — ordered destinations `server:/path` (the server is the name of an SSH connection) with rules: typed confirmation, `allowed_refs`, extra ignore patterns, an optional `post_deploy` command and the number of backups to keep. Add them on the new *Environments* tab (✎) or in the catalog JSON (`apps[].environments`). See [Git workspace: environments, deploy plans & safe deploys](#git-workspace-environments-deploy-plans--safe-deploys).
- **Server-side state:** an environment deploy keeps its state next to the installation, by default in `<path>/.deploy-bak` (the existing backup directory): `state.json`, `history.jsonl`, `deploys/<id>.json`, `backups/rollback-<id>/` and the lock `.lock/`. Backups keep the existing `<id>-<YYYYMMDD-HHMMSS>` layout, and `VERSION.md` / `updates.jsonl` are written as before, so file-based tools keep working.
- **Anchored patterns:** an exclude or ignore pattern that starts with `/` now matches the whole relative path only (`/lib/a.py` does not match `x/lib/a.py`). *Exclude permanently* writes such patterns; existing patterns are unchanged.
- The same policies apply: `git_update` (deploy), `git_upgrade` (deploy of another ref), `git_rollback`, `git_restart`. Environment deploys run right away (no scheduling); use a dry run to preview.

## Upgrading from v11.4.0

Replace the binary; the database does not change.

- **Clipboard history per user:** the *Clipboard* panel now keeps its history per signed-in user, and signing out removes it from the browser ([#30](https://github.com/vedranius/web-browser-RDM-public/issues/30)). The browser-wide history of earlier versions is deleted on the first sign-in, so the panel starts empty once.

## Upgrading from v11.3.0

Replace the binary. The database gets two new columns of `git_installs` (`last_ok_at`, `last_ok_state`); the previous binary still starts on it and ignores them.

- **Git sources lost by the v11.3.0 upgrade come back:** upgrading a database from v11.2.0 or earlier to v11.3.0 set the `git_sources` table aside as `git_sources_old_<time>`. On the first start, v11.4.0 restores those sources (with their encrypted tokens) when no source exists yet; the old table is renamed to `git_sources_restored_<time>`.
- **Every file of an installation:** the details list every file of the server folder and of the repository, with the reason for each state, and diff any repository file (also the ones the catalog excludes). **Refresh the targets once** so the targets keep their full commit and the repository files outside the catalog are listed too. See [Git workspace: every file and partial checks](#git-workspace-every-file-and-partial-checks).
- **Partial checks:** select installations, servers or services and check only those (or only stale ones); results stream in and can be cancelled. Two new settings: `git_check_parallel` (8) and `git_check_per_server` (2).

## Upgrading from v11.2.0

Replace the binary. The database gets two new tables (`git_feeds`, `git_pipelines`) and four new columns of `git_sources` (`write_token`, `hook_id`, `hook_secret`, `hook_at`); the previous binary still starts on it and ignores them.

- **.gitignore helper:** a new *.gitignore* tab in the Git workspace. The output is a download; a merge / pull request needs an optional **write token** on the Git source and the new policy `git_gitignore_mr` (**administrators only** by default). See [Git workspace: .gitignore helper](#git-workspace-gitignore-helper).
- **Optional CI integration:** a webhook per Git source, bundles fetched from CI artifacts, and the deploy method *CI pipeline* per service. All of it is off until you set it up; WRM works without any CI. See [Git workspace: CI integration (optional)](#git-workspace-ci-integration-optional).
- Webhooks arrive at `/api/hooks/git/<id>`: if WRM is behind a reverse proxy that only forwards some paths, forward this one too.
- Checks now also record the size of files found only on servers; installations show it after their next check.

## Upgrading from v11.1.0

Replace the binary. The database is unchanged: installs and transfers are stored as runs in `git_runs`; the previous binary still starts on it.

- **New servers from the Git workspace:** *Install…* on the *Installations* tab (a wizard: server, services with their version, target directory, pre-checks, per-host files from templates / copies / typed, optional systemd or supervisor unit, optional start) and *Transfer…* for an installation (copy it to another server with its per-host configuration). See [Git workspace: new servers (install & transfer)](#git-workspace-new-servers-install--transfer).
- New policies `git_install` and `git_transfer` — **administrators only** by default; change them in *Admin panel → Policies*.
- Installs and transfers send the existing run notifications (*Git: … started / succeeded / failed*) and are audited as `git.install` / `git.transfer`.

## Upgrading from v11.0.0

Replace the binary. The database gets two new tables (`git_runs`, `git_pending_restarts`); the previous binary still starts on it and ignores them.

- **Deploying from the Git workspace:** *Update*, *Upgrade…*, *Rollback…*, *Restart…* and *Stamp* on the *Installations* tab and in the details of an installation, a new *Runs* tab, and scheduled runs. See [Git workspace: update, upgrade, rollback & restarts](#git-workspace-update-upgrade-rollback--restarts).
- New policies `git_update`, `git_upgrade`, `git_rollback`, `git_restart` — **administrators only** by default; change them in *Admin panel → Policies* — and `git_schedule_grace_minutes` (30).
- New notification events for runs and restarts (*Git: an update, upgrade or rollback started / succeeded / failed*, *a service restart is scheduled / done*, *a scheduled run starts soon*) in *Settings → Notifications*.
- WRM writes on servers only when you start or schedule a run; checks stay read-only.

## Upgrading from v10.10.0

Replace the binary. The database gets new tables (`git_sources`, `git_trees`, `git_blobs`, `git_workspace`, `git_targets`, `git_installs`); the previous binary still starts on it and ignores them.

- **Git workspace:** the new **⎇ Git** button in the top bar compares the services installed on your servers with GitLab / GitHub or an offline bundle, file by file and read-only. See [Git workspace](#git-workspace-compare-services).
- It is on for everyone. Administrators can hide it (`git_enabled` off, or `WRM_GIT_ENABLED=0`) or limit checks to administrators (`git_checks`).
- New notification events *Git: a new version is available*, *Git: files changed by hand on a server* and *Git: a server is unreachable for checks* in *Settings → Notifications*.

## Upgrading from v10.9.1

Replace the binary. The database gets a new table `bookmarks`; the previous binary still starts on it and ignores it.

- **Folder bookmarks:** ★ next to the path bar of the file manager and in a terminal's title bar. *Bookmark this directory…* saves the current folder for one connection, a folder, a tag or all connections; paths may use `~`, `$USER`, `{host}` and `{name}`. A bookmark can be the **start directory** of a connection. See [Folder bookmarks](#folder-bookmarks).
- **WinSCP bookmarks:** *★ → Manage bookmarks… → Import WinSCP.ini* adds the remote directory bookmarks of a WinSCP configuration as global bookmarks.
- Bookmarks are part of *Export* / *Import*.

## Upgrading from v10.9.0

Replace the binary. The database gets a new table `putty_sessions` and a column `connections.putty_session`; the previous binary still starts on it and ignores them.

- **Import from PuTTY:** *Settings → Data → Import from PuTTY* reads a `.reg` export of your saved PuTTY sessions, with their proxies. See [Import from mRemoteNG, PuTTY and OpenSSH](#import-from-mremoteng-putty-and-openssh).
- mRemoteNG connections imported **before** v10.9.1 do not remember their *PuttySession*, and importing the file again skips them as existing: give them the proxy by hand (or through their folder's default proxy).

## Upgrading from v10.8.1

Replace the binary. The database gets new tables (`proxies`, `proxy_grants`, `notify_channels`, `notify_subscriptions`, `notify_prefs`, `notify_pending`, `notify_state`) and columns (`connections.proxy_id`, `folders.jump_conn_id`, `folders.proxy_id`); the previous binary still starts on it and ignores them (connections then go without their proxy and folder defaults).

- **Proxies:** define SOCKS5, SOCKS4/4a or HTTP CONNECT proxies (or a SOCKS tunnel of a connection) under *🔑 SSH keys & credentials → 🌐 Proxies* and choose one in a connection (*Proxy*, next to *Jump host*). See [Proxies](#proxies-socks--http).
- **Folder defaults:** right-click a folder → *Folder settings…* to give it a default jump host and proxy. Connections in the folder that have no jump host of their own then use the folder's (existing connections without a jump host included). Choose *no jump host (not the folder's)* where a connection must stay direct.
- **Notifications:** an administrator adds channels in *Admin panel → Notifications* (e-mail, Telegram, Slack / Mattermost / Rocket.Chat, Teams, Discord, ntfy, Gotify, Pushover, webhook); users subscribe in *Settings → Notifications*. Nothing is sent until both happened. See [Notifications](#notifications-e-mail-chat-push-webhook).
- **Docker:** `docker-compose.yml` now maps `host.docker.internal` to the Docker host (`extra_hosts`), for proxies that run on the host.
- Deleting a jump host now makes the connections that used it fall back to their folder's default jump host (or go direct).

## Upgrading from v10.8.0

Replace the binary. Nothing changes in the database.

- **Larger imports work:** mRemoteNG, OpenSSH config and inventory (CSV / Excel) files up to **20 MB** import again (from about 1 MB on they failed with *Bad JSON*). A larger file gets a clear message with its size and the limit, already in the browser.
- **Install as an app** (*Settings → General*) now shows the steps for your browser, or the reason why installing is not possible here (plain `http://` to an address other than `localhost`, an untrusted certificate, Firefox on the desktop). The *Install* button appears only when the browser can install right away. To get HTTPS quickly, see [HTTPS with Caddy](#https-with-caddy).
- A reverse proxy in front of WRM must allow request bodies of at least 32 MB for imports (`client_max_body_size 0;` in the nginx example does).

## Upgrading from v10.7

Replace the binary. The database gets two new columns (`connections.notes`, `connections.temp_until`); the previous binary still starts on it (it shows quick connections as normal ones).

- **⚡ Quick connect** field at the top of the sidebar: `user@host:port` + Enter.
- **📝 Notes** in the connection dialog and in the right-click menu; a 📝 next to a connection opens them.
- **🧰 Tools** in the top bar, and *Network tools from this server…* / *Check ports of this host* in the right-click menu (policy `network_tools`, default: all users).
- **Install as an app**: *Settings → General*, or the install icon of the browser's address bar (needs HTTPS with a trusted certificate, or `localhost`).

## Upgrading from v10.6

Replace the binary. The database gets one new column (`connections.bmc`); the previous binary still starts on it.

- Connections can have a **BMC** (*Edit → Out-of-band management*); right-click → *Power & status (BMC)…* and *Serial console*. For IPMI install `ipmitool` on the WRM server or on the jump host (`apt install ipmitool`).
- New protocol **Serial port** for console cables on the WRM machine (administrators by default, policy `serial_ports`).
- Saved workspace sessions now really keep remote desktop windows (and console windows).

## Upgrading from v10.5

Replace the binary. The database gets three new columns (`connections.tags`, `ext_id`, `ext_tags`) and a table `inventory_sources`; the previous binary still starts on it (it ignores the tags).

- Connections have **tags**; the sidebar shows a tag filter and an environment badge (`env:prod` …). See [Tags & inventory import](#tags--inventory-import-csv-excel-netbox).
- **Settings → Data → Import inventory**: CSV / Excel files and NetBox.

## Upgrading from v10.4

Replace the binary. The database gets new tables (`ssh_keys`, `ssh_key_deployments`, `credentials`, `credential_grants`) and two new columns (`connections.key_id`, `connections.credential_id`); the previous binary still starts on it (connections that use the new login types then cannot log in).

- New **🔑 Keys** button: your SSH key store and the credentials vault. See [SSH keys & credentials vault](#ssh-keys--credentials-vault).
- Connections can log in with an **SSH key from the key store** or a **credential from the vault**; remote desktops and FTP can use vault credentials too.
- Deleting an account now also deletes its snippets, tunnels, keys and credentials.

## Upgrading from v10.3

Replace the binary. The database gets one new column (`connections.options`); the previous binary still starts on it.

- **Remote desktops need guacd** next to WRM: `apt install guacd` (Debian/Ubuntu), `docker run -d --network host guacamole/guacd`, or the `guacd` service that `docker-compose.yml` now contains. Without guacd everything else keeps working; opening an RDP/VNC/Telnet connection then explains what is missing. See [Remote desktop](#remote-desktop-rdp-vnc-telnet).
- Importing from **mRemoteNG** now also brings RDP, VNC and Telnet connections (with domain, console, colors and RD Gateway).
- SQLite now waits up to 10 s for a busy database on every connection (fixes rare *database is locked* errors under load).

## Upgrading from v10.2

Replace the binary. The database gets a new table (`snippets`) and one new column (`connections.monitor`); the previous binary still starts on it.

- **File names no longer carry a suffix:** releases are `v10.3.0`, binaries `wrm-pro-v10.3.0-linux-amd64` etc. Update scripts and systemd units that use the file name.
- **Live status is on by default:** WRM opens a TCP connection to every saved host:port once a minute (SSH and FTP: it reads the greeting and closes politely, like Nagios `check_ssh`). Servers see it in their logs. Change the interval or turn it off in *Admin panel → Security policies → Live status*, or untick *Monitor up/down status* in a connection. See [Live up/down status](#live-updown-status) about fail2ban.
- **Broadcast input** is available to everybody; turn it off with `broadcast_enabled` if you do not want it. Every start and stop is in the audit log.

## Upgrading from v10.1

Replace the binary. The database gets a new table (`connection_tunnels`) and two new columns on `connections` (`jump_conn_id`, `web_path`); nothing else changes. To go back, run the v10.1 binary on the same database; it does not know these features (connections behind a jump host then try to connect directly, web interface connections cannot be opened, tunnels do not run).

- New: [jump hosts](#jump-hosts-bastions), [SSH tunnels](#ssh-tunnels-port-forwarding), [web interface connections](#web-interfaces-http--https-connections) and [import from mRemoteNG / OpenSSH](#import-from-mremoteng-putty-and-openssh).
- **Tunnels are on by default for all users**, listening on `127.0.0.1` of the WRM machine only. Remote forwards (`-R`) and listening on network addresses are limited to administrators. Review *Admin panel → Security policies → SSH tunnels* — e.g. turn tunnels off (`WRM_TUNNELS_ENABLED=0`) or limit them to administrators if WRM runs on a shared server.
- Jump hosts need `AllowTcpForwarding yes` on the bastion (the OpenSSH default), exactly like `ssh -J`.

## Upgrading from v10.0

Replace the binary. The database is extended automatically (new tables, columns and triggers only — nothing is changed or removed; the previous binary still runs on the upgraded database).

- **Terminal sessions are now recorded by default** (`session_recording`). Users see *“This session is recorded”* and a **● REC** badge on the terminal. Inform your users (in many countries this is required), or turn it off in *Admin panel → Security policies → Session recording*. Keystrokes are **not** recorded unless you turn on *Record keystrokes*.
- Recordings use disk space next to the database (`recordings/`, typically a few KB to a few MB per session; limit per session `recording_max_mb`, kept `recording_retention_days` = 90 days). Back up or exclude that folder as you need.
- The audit log is now **append-only and hash-chained**. *Admin panel → Audit log → Verify integrity* checks it. Entries written before the upgrade have no hash and are skipped by the check.
- Every upload, download (also files opened in the editor and files in ZIP downloads) and server-to-server copy is logged in *Admin panel → File transfers* with size and SHA-256.
- New: `/healthz` for load balancers and monitoring, `wrm -healthcheck` for container health checks, a `Dockerfile` and `docker-compose.yml`.

## Upgrading from v9

Replace the binary and start it with the same database and the same `ENCRYPTION_KEY` (if you set one). Everything is migrated automatically. Things to know:

- **Stored secrets are re-encrypted.** Without `ENCRYPTION_KEY`, v9 used a public default key. v10 generates a random key file (`remote_manager.db.key`) and re-encrypts all secrets with it on the first start. Back that file up. (If you did set `ENCRYPTION_KEY`, nothing changes.)
- **Self-registration is closed by default.** Existing accounts keep working. Open it again in *Admin panel → Security policies* if you want it.
- **Passwords and keys are no longer sent to the browser.** The *Edit connection* dialog shows “saved and encrypted — leave empty to keep it”. *Export* is without secrets; *Export with passwords & keys* asks for your password.
- **SSH host keys are verified** (trust on first use). The first connection to each server remembers its key; a changed key is refused until you accept it.
- **Key file / Auto (~/.ssh)** authentication (keys stored on the WRM server) is limited to administrators unless the policy *Allow server key files for all users* is on.
- **Existing shares keep their behaviour** (anyone with the link, role *Operator*). Edit them to use the new access modes and roles.
- **Behind a reverse proxy:** state-changing API calls now require the `Origin` to match the `Host` (like WebSockets already did). Pass the `Host` header, or set `WRM_ALLOWED_ORIGINS`. Set `WRM_TRUST_PROXY=1` so rate limits and the audit log see real client IPs.
- The per-IP sign-in limit is now 20 failures / 10 minutes; accounts additionally lock for 15 minutes after 10 wrong passwords (policy).
- If the database comes from an earlier build that already had tables named like the new v10 tables (`audit_log`, `collab_messages`, …) but with a different layout, WRM keeps them as `<table>_old_<time>` and creates new ones (logged as *Upgraded table …*). Nothing is deleted.
- The app is served at `/`. Old bookmarks to `/static/` are forwarded there.

---

## How it works

```
 Browser (any device)                      WRM server (one binary)                     Your servers
┌──────────────────────┐   HTTPS/HTTP   ┌──────────────────────────────┐   SSH / SFTP   ┌───────────┐
│ Web UI (embedded     │ ─────────────▶ │ REST API  (/api/...)         │ ─────────────▶ │ Linux/BSD │
│ single-page app,     │                │ WebSockets:                  │   FTP / FTPS   │ servers,  │
│ xterm.js terminal)   │ ◀───────────── │  /ws/ssh     terminal        │ ─────────────▶ │ NAS, ...  │
│                      │   WebSockets   │  /ws/share/  collaboration   │                └───────────┘
└──────────┬───────────┘                │  /ws/events  live updates    │
           │  voice (WebRTC, DTLS-SRTP) │ TURN relay  UDP/TCP 3478     │
           └────────────────────────────┤ SQLite: users, connections,  │
             peer-to-peer or via relay  │ shares, audit log, settings  │
                                        └──────────────────────────────┘
```

- **The WRM server makes the connections.** Your browser talks only to WRM, and WRM talks to your servers with SSH (`golang.org/x/crypto/ssh`), SFTP (`pkg/sftp`) and FTP (`jlaffaye/ftp`). Your servers only need to be reachable **from the WRM machine**.
- **Terminal:** every terminal window is a WebSocket (`/ws/ssh`) bound to one SSH session with a PTY. Keystrokes and output go as **binary frames**, so large outputs and multi-byte UTF-8 are never corrupted. The PTY size follows the window.
- **Flow control:** when a command prints faster than the browser can draw, the browser asks the server to pause and resume. Nothing is lost.
- **File operations** use a small **pool of SFTP connections** per saved connection (reused for 5 minutes and health-checked). Uploads are **streamed**; downloads and ZIP archives are streamed to the browser.
- **Secrets at rest** (connection passwords, private keys, 2FA secrets) are **AES-256-GCM encrypted**. Session tokens are stored only as SHA-256 hashes.
- **Collaboration:** `/ws/share/{token}` is the room of a share. The server decides identities and roles and relays chat, files, terminal streams (only to people watching them), remote-control keystrokes (only to the person who granted control) and voice signalling.
- **Jump hosts and tunnels:** WRM logs in to the jump host, opens an SSH `direct-tcpip` channel through it and runs a second, separately encrypted SSH session inside it (like `ssh -J`). Tunnels listen on the **WRM machine** (local / SOCKS) or on the SSH server (remote) and forward every connection through SSH. Tunnel ports are opened by WRM, not by the browser.
- **Voice:** audio flows **directly between browsers** (WebRTC, end-to-end encrypted with DTLS-SRTP). When browsers cannot reach each other (NAT, firewalls), the **built-in TURN relay** carries the still-encrypted audio.

---

## Features in detail

### Accounts, sign-in & two-factor authentication

- The **first account becomes administrator**. Afterwards **self-registration is closed** (policy); administrators create accounts, optionally with a generated temporary password that must be changed at the first sign-in.
- Passwords are stored as **bcrypt** hashes (cost 12). Minimum length is a policy (default 8).
- **Two-factor authentication (TOTP)** with any authenticator app (Google/Microsoft Authenticator, 1Password, Bitwarden, FreeOTP…): *Settings → Security → Two-factor authentication*. A QR code is shown; **10 single-use recovery codes** are generated. Codes cannot be replayed. Administrators can **require 2FA** for administrators or for everyone — users without it must set it up at their next sign-in.
- **Brute-force protection:** 20 failed sign-ins from one IP within 10 minutes lock that IP for 5 minutes; an **account** locks for 15 minutes after 10 wrong passwords (policy). Failed sign-ins take the same time whether the user exists or not.
- **Sessions:** HttpOnly, SameSite cookie (`Secure` on HTTPS); the database only stores a hash of the token. Sessions end after inactivity (default 7 days) and after a maximum age (default 30 days) — both policies. *Settings → Security* lists your **signed-in devices** and signs out one or all others. Changing your password signs out every other device.
- If your login expires while terminals or transfers are running, WRM shows a message instead of reloading the page.

### Connections & folders

Folders can have a **Parent folder** (up to 32 levels); right-click a folder to create a subfolder or edit its parent. The sidebar renders the hierarchy and connection pickers show the complete folder path; tabs and window titles show only the connection's name, with the folder path in their tooltip. The row above connection tabs filters open tabs/windows by folder and focuses its most recently used window. Config export/import and new mRemoteNG imports preserve nesting. Deleting a folder moves its child folders to its former parent and keeps their connections. Sharing a folder includes descendants; jump/proxy defaults apply to the immediate folder only.

**Sorting folders (v13.14.0).** Folders keep the order you give them by drag and drop. Right-click a folder for **Sort subfolders A–Z (all levels)** or **Keep subfolders sorted A–Z**, and the top level offers **Sort all folders A–Z** and **Keep all folders sorted A–Z**. A kept-sorted folder puts new, renamed and moved subfolders in their place by name at every level below it. Names compare without case and with numbers in their natural order (`srv2` before `srv10`). Connections inside a folder are always listed by name. Sorting is audited as `folder.sorted`.

Left sidebar ("PRO MANAGER"):

- **+ Connection** creates a connection:
  - **Protocol:** `SSH` (terminal, plus files over SFTP), `SFTP` (opens the file manager by default), **🖥 RDP**, **🖥 VNC**, **Telnet** (see [Remote desktop](#remote-desktop-rdp-vnc-telnet)), **🔌 Serial port** of the WRM server (see [Serial ports](#out-of-band-management-consoles--serial-ports)), `FTP`, `FTPS` (FTP with explicit TLS; file manager only), **🌐 Web interface** `HTTPS` / `HTTP` (see [Web interfaces](#web-interfaces-http--https-connections)).
  - **Host : Port** (default port 22 for SSH/SFTP, 21 for FTP/FTPS, 443/80 for web interfaces), **Username**, **Folder**, **Tags** (comma separated, e.g. `env:prod, site:dc1, rack:a12, web`; the most used tags are offered below the field).
  - **Jump host (connect via):** another saved SSH connection to go through (see [Jump hosts](#jump-hosts-bastions)).
  - **Proxy:** a saved SOCKS / HTTP proxy for everything the connection does over TCP (see [Proxies](#proxies-socks--http)). With a jump host the proxy is reached through it; the dialog shows the route, e.g. `⤳ Route: bastion → socks5://10.1.1.1:1080 → app-01`.
  - Both show *— as the folder: … —* when the folder has a default (see below); *no jump host / no proxy (not the folder's)* overrides it.
  - **🔀 SSH tunnels:** port forwards of this connection (see [SSH tunnels](#ssh-tunnels-port-forwarding)).
  - **⚡ Out-of-band management:** the BMC of the server (see [Out-of-band management](#out-of-band-management-consoles--serial-ports)).
  - **📝 Notes & runbook:** procedures, contacts, links (see [Notes](#quick-connect--connection-notes)).
  - **Authentication:** *Password*, *Private key (paste)*, *Key file (path on the WRM server)*, *Auto* (tries `~/.ssh/id_rsa`, `id_ed25519`, `id_ecdsa`, `id_dsa` of the WRM server user), **🔑 SSH key from the key store** or **🗝 Credential from the vault** (see [SSH keys & credentials vault](#ssh-keys--credentials-vault)). *Key file* and *Auto* use keys of the **WRM server itself** and are therefore limited to administrators (policy). Password authentication also answers keyboard-interactive prompts. RDP, VNC, Telnet and FTP log in with a password or a vault credential.
  - **Use the server's hostname as the name:** leave the name empty; after saving, WRM logs in once and reads the hostname (SSH, SFTP), or looks the address up in DNS (other protocols, or the first label of a typed host name), and names the connection after it. Until then the address is the name. A host key that is not known yet is shown for approval first. *Rename to the server's hostname* in the connection menu does the same later.
  - **🔌 Test connection** logs in once and shows the result, latency and — for a new server — its host key fingerprint.
- **Secrets stay on the server unless the owner asks for them.** Passwords and private keys are write-only in the form: the edit dialog shows “saved and encrypted — leave empty to keep it”, with an option to remove the saved secret. *Duplicate* copies the connection on the server.
- **👁 Show saved login** (in *Edit connection*, or the 👁 in an empty password field) shows what the connection logs in with: its password or private key, the vault credential's user name and password (only for credentials you own), the key store key or the Bitwarden SSH key, or the path of a key file on the WRM server. You confirm with your WRM password; a domain (LDAP / Active Directory) account confirms with its directory password, and an account that only signs in through an identity provider confirms there (step-up). With *Confirm dangerous actions (step-up)* on, the 2FA code is asked as well. Every reveal is in the audit log (`connection.secret_revealed`, failed attempts `connection.secret_denied`), raises the *secret revealed* alert when it is on, counts towards the sign-in rate limit, and follows the *Allow export of passwords & keys* policy (`allow_secret_export`; administrators always can).
- **Host key verification:** the first connection to a server remembers its SSH host key (and, for FTPS, the certificate if it is not signed by a public CA). A **changed key is refused** with a clear warning and both fingerprints; the owner of the connection (or an administrator) can accept the new key after checking it. Policy: *trust on first use* (default), *strict* (new hosts must be approved) or *off*.
- **Double-click** a connection to open it: SSH opens a terminal; SFTP/FTP/FTPS open the file manager. On touch devices a single tap opens it.
- **Right-click** a connection: *Open as Terminal / File Manager* (in a new or an existing window), *Tunnels*, *Start / Stop all tunnels*, *Add tunnel…*, *Network tools from this server…*, *Who has access (authorized_keys)…*, *Deploy SSH key…*, *Notes & runbook*, *Check ports of this host*, *Edit*, *Duplicate*, *Rename to the server's hostname*, *Share*, **Copy username**, **Copy password** (after the same confirmation as *Show saved login*; the password is never shown in the notification), **Copy IP address** (or host name), *Delete*. Web interface connections: *Open web interface*.
- **📁 Folder** creates folders. **Drag** connections onto a folder to move them, or onto *"↓ Drop here"* to move them back to the root. Right-click a folder to *Share* or *Delete* it (its connections move to the root), or **Folder settings…** to rename it and give it a **default jump host** and a **default proxy**: its connections — also new ones and the ones moved into it — use them unless they choose their own (⤳ after the folder name). Export and import keep the defaults.
- **Search box** filters by name, host, username, protocol or tag; `#tag` searches tags only, several words must all match (`#env:prod web`). The **🏷 tag bar** above the list filters by one or more tags (click again to remove). **Multi-select** with **Ctrl/Cmd + click**, then use **🤝 Share**, **🔑 Key** (deploy an SSH key to all of them), **🏷 Tags** (add tags, or remove them with `-tag`) or **Delete**.
- The **dot in front of a connection** is its [live status](#live-updown-status): 🟢 up, 🟡 up but slow (> 300 ms), 🔴 down, a ring = behind a jump host (colored by the jump host's state). A **blue dot after the name** means one of its terminals is connected. **⤳** means it goes through a jump host (hover shows the route), **🔀** that it has tunnels (colored while one runs; click opens them), **WEB** marks web interfaces.
- **Shared with me** lists shares you are a member of (and shares published for all users), with your **role**; ↗ opens the collaboration room.

Folders also support manual ordering: use the folder menu's **Move up / Move down**, or drag into the top/bottom third of another folder to place before/after it. Drop in the center to make it a child, or on the root area to move it to the root. Ownership, hierarchy depth and cycle checks apply. Export/import includes the saved order.

### Quick connect & connection notes

**⚡ Quick connect** — the field at the top of the sidebar (*⚡ Quick connect: user@host:port*) — connects without filling in the connection dialog:

- Type an address and press **Enter**: `root@10.0.0.5`, `admin@sw1:2222`, `10.0.0.7`, or with a scheme: `ssh://`, `sftp://`, `rdp://admin@10.0.0.20`, `vnc://10.0.0.30:5901`, `telnet://sw1:23`, `https://10.0.0.11/ui`. Without a scheme it is SSH.
- A small dialog asks for the **login** — a password, a **vault credential** or a **key from the key store** — and optionally a **jump host**. Enter connects: a terminal, file manager, remote desktop or web interface opens like for a saved connection (host key check, recording and audit included; audit action `connection.quick`).
- The connection is listed under **⚡ Quick connections** in the sidebar: 💾 (or right-click → *Save as a connection*) opens the connection dialog to give it a name, folder and tags and keep it; ✕ removes it. Quick connections are kept **24 hours after their last use** (an open terminal keeps them), at most 20 per user, are not monitored and not exported.

**📝 Notes & runbook** — every connection has notes: what runs there, how to restart it, who to call, links to the wiki or the ticket:

- *Edit → 📝 Notes & runbook*, or right-click → *Notes & runbook*. A **📝** after the name in the sidebar and in the window bar of its terminals opens them.
- A small markdown: `# headings`, `- lists`, `**bold**`, `` `code` ``, ```` ``` ```` code blocks and links (`https://…`). Everything else is shown as text (no HTML).
- Notes belong to the owner of the connection: share members don't see them. They are exported and imported with the connection and copied by *Duplicate*. Up to 20 000 characters.

### SSH keys & credentials vault

The **🔑 Keys** button in the top bar opens two lists: **SSH keys** and **Credentials**.

**SSH keys (key store)**

- **Generate key:** ED25519 (recommended), RSA 2048/3072/4096 or ECDSA 256/384/521, with a name and a comment. The key is created on the WRM server and stored **encrypted** like all other secrets.
- **Import key:** paste (or choose the file of) an OpenSSH or PEM private key — keys with a passphrase are asked for it once and then stored encrypted with WRM's key. A **public key alone** (e.g. a colleague's `id_ed25519.pub`) can be imported too: it can be deployed to servers, but WRM cannot log in with it. PuTTY `.ppk` keys: export them with PuTTYgen → *Conversions → Export OpenSSH key* first.
- **Use it:** in a connection choose *Authentication → SSH key from the key store*. The key list shows which connections log in with a key, where it is deployed and when it was last used.
- **🚀 Deploy:** puts the public key on the selected SSH connections, like `ssh-copy-id`: WRM logs in with each connection's current login (password, other key or vault credential — through jump hosts too), creates `~/.ssh` if needed and appends the key to `~/.ssh/authorized_keys` once (a key that is already there is not added again; other lines stay untouched; SELinux contexts are restored). With **"Log in with this key from now on"** WRM then logs in with the key to test it and switches the connections to it. Deploy from the key, from a connection (right-click → *Deploy SSH key…*) or for many selected connections (**🔑 Key** in the sidebar).
- **✂ Revoke:** removes the key from `authorized_keys` on the selected servers. A connection that logs in with that key cannot be selected — WRM would lock itself out.
- **📋 Copy public key**, **⬇ .pub** download, **rename**, **delete** (refused while connections or credentials log in with the key — servers keep a deleted key until you revoke it).
- **🔓 Export private key:** after entering your WRM password again, optionally protected with a new passphrase; recorded in the audit log. Allowed when *allow_secret_export* is on (always for administrators).

**Who has access** (right-click an SSH connection → *Who has access (authorized_keys)…*): WRM reads `~/.ssh/authorized_keys` of the user it logs in as and lists every key with type, size, comment, fingerprint and options (`from=…`, `command=…`). Keys of your key store are named, **the key WRM itself logs in with** is marked (and cannot be removed), and every other key can be **removed** with one click. *Add a key…* deploys one there. Keys allowed elsewhere (`AuthorizedKeysFile`, `AuthorizedKeysCommand`, other accounts) are not shown.

**Credentials (vault)**

A credential is a login that many connections share: a user name with a **password**, an **SSH key** of the key store, or both. Typical: `root@dc1`, `Administrator` of the Windows servers, the iDRAC/iLO admin.

- **New credential:** name, user name (optional — without one each connection uses its own user name), password (🎲 generates a strong one), SSH key, description, a **rotation reminder** in days and **Only for hosts**: host names with `*` and `?` or CIDR ranges (`*.dc1.example.com, 10.1.0.0/16`). With a host list the credential works only for matching hosts.
- **Use it:** in a connection choose *Authentication → Credential from the vault*. SSH, SFTP, RDP, VNC, Telnet and FTP connections can use it. Change the credential once — every connection logs in with the new secret.
- **Share with users:** add colleagues (or, administrators, *all users*). They see the credential in their list and can use it in **their own** connections, but they never see or change the secret. Removing a user stops their connections from logging in with it at once. Because a password is sent to the server when logging in, a shared credential should have a host list: then it only works for those hosts — and, for the people it is shared with, only through jump hosts that match the list too. WRM asks before saving a shared credential without a host list.
- **👁 Show** reveals the stored password to its owner after entering the WRM password again (audited; policy *allow_secret_export*). **🔗 Used by** lists the connections that use it (all users' for the owner).

**Password rotation** (🔄 *Rotate…* on a credential, owner only) changes the password on every SSH server that uses the credential:

1. **Check logins** — WRM logs in to every server with the stored password (through jump hosts, in parallel) and changes nothing. Connections of other users that use the credential are included; the same user@host:port is changed only once.
2. **Rotate now** — the same check first: if one server does not accept the stored password, **nothing is changed**. Then, one server after the other, WRM runs `passwd` over SSH (answering its prompts in a terminal), logs in with the new password to verify it, and finally stores it in the vault. The new password is generated (24 characters: upper and lower case letters, digits and symbols) or entered by you.
3. **All or nothing:** when a server fails (password rules, `passwd` not allowed…), the servers already changed get the old password back and the vault keeps it. If even that fails (e.g. a password history rule refuses the old password), the vault keeps the old password and remembers the new one as **pending**: the credential is marked *rotation incomplete*, *Show* reveals both, and the result lists which servers have which password. Set the right password with *Edit* once fixed.

The progress of every server is shown live. Rotation needs SSH connections to servers where the login user may change its own password with `passwd` (Linux, BSD, macOS). Connections of other types (RDP, VNC, FTP…) block rotation — change those passwords yourself and save the new one with *Edit* (*Edit* with a new password counts as rotated). When the rotation reminder is due, the 🔑 button shows a badge and *Admin panel → Overview* warns.

Everything is in the audit log: `ssh_key.created`, `ssh_key.deployed`, `ssh_key.revoked`, `ssh_key.exported`, `ssh_key.deleted`, `credential.created`, `credential.updated`, `credential.granted`, `credential.grant_revoked`, `credential.revealed`, `credential.checked`, `credential.rotated`, `credential.rotation_failed`, `credential.rotation_incomplete` (never the secrets).

### SSH user certificates and opkssh

A key can log in with an **OpenSSH user certificate** issued for it, by an SSH certificate authority or by [opkssh](https://github.com/openpubkey/opkssh), which turns an OpenID Connect sign-in into a short-lived certificate (`opkssh login` writes `~/.ssh/id_ecdsa` and `id_ecdsa-cert.pub`). WRM offers the certificate first and the plain key after it.

- **Key files on the WRM server** (*Key file* or *Automatic*): WRM reads `<key>-cert.pub` next to the key at every login, so a renewed certificate is used at once; a path to the `-cert.pub` file works too. For opkssh, run `opkssh login` as the user WRM runs as. *Automatic* prefers a key with a valid certificate.
- **Keys in the key store**: *SSH keys → 📜 Certificate* saves the certificate line with the key. The key shows a pill with the expiry (*opkssh* for opkssh certificates) and the details (key ID, principals, validity, CA fingerprint) in its tooltip.
- **A key pasted into a connection**: paste the certificate line after the private key (what `cat id_ecdsa id_ecdsa-cert.pub` prints).

A certificate must be a user certificate for the same public key, or it is refused. An expired certificate is skipped (the plain key is still offered), and a refused login says that the certificate expired and how to renew it. The server side needs `TrustedUserCAKeys` (or opkssh's `AuthorizedKeysCommand`); WRM does not change it.

### Bitwarden SSH keys

Open **Keys & credentials → Bitwarden**, choose the cloud/EU/self-hosted HTTPS server, and sign in with email, master password and supported two-step code. Personal API credentials plus a master password are also supported when email login requires a bot challenge or another MFA flow. Unlock the vault, sync if needed, then choose **Bitwarden SSH key** in an SSH/SFTP connection and select a native SSH key item. Secure-note attachments and arbitrary text fields are not SSH key items.

Install the [official Bitwarden CLI](https://bitwarden.com/help/cli/) on the WRM server (`bw` on PATH or `WRM_BITWARDEN_CLI`); Docker includes CLI 2026.9.1. Each WRM account has its own CLI profile. The connection stores its item ID; private keys are fetched server-side for authentication and are not returned to the browser or copied into the connection database. Master passwords and unlock session keys are not persisted. CLI `data.json` is sealed with the WRM encryption key between commands and recovered at startup after interrupted calls.

The default unlock inactivity is 15 minutes; a WRM restart, WRM logout, Lock or Sign out also ends unlock access. Lock prevents new key retrieval; existing SSH sessions continue. Refreshing keys and manual vault synchronization extend interactive inactivity; background SSH operations do not. Cloud login requires reachability from the WRM host. Errors distinguish missing CLI/locked vault from login failures without exposing CLI output.

### Jump hosts (bastions)

Servers in data centers and customer networks are often reachable only through a bastion ("jump host"). WRM goes through it like `ssh -J` / `ProxyJump` and like the *SSH tunnel* setting of mRemoteNG:

1. Create the bastion as a normal SSH connection, e.g. `bastion-dc1` → `203.0.113.10:22`.
2. Create the internal server, e.g. `app-01` → `10.0.0.21:22`, and choose **Jump host (connect via) → bastion-dc1**.
3. Double-click `app-01`. The terminal shows `Connecting to 10.0.0.21:22 via bastion-dc1…`.

```
 WRM ──SSH──► bastion-dc1 ──SSH inside the first connection──► app-01 (10.0.0.21)
```

- **Everything works through it:** terminal, file manager (SFTP), FTP/FTPS (the FTP connection is made from the jump host), search, editor, server-to-server transfer, *Test connection*, tunnels and web interfaces.
- **Chains:** a jump host can have its own jump host — up to 5 hops, e.g. `vpn-gw → bastion-dc1 → app-01`. Loops are refused when you save.
- **Every hop logs in with its own credentials** (password, key, key file) and **its host key is verified** (trust on first use / strict), like a direct connection. WRM opens no port on the jump host; it uses a `direct-tcpip` channel, so the bastion only needs `AllowTcpForwarding yes` (the OpenSSH default).
- The route is shown in the sidebar (**⤳**), in *Test connection* (`… via bastion-dc1`), in the terminal, in *Sessions & recordings* and in the audit log.
- Jump hosts are your own saved SSH connections. **Shares** of a connection behind a jump host work too — members never see the credentials of either.
- Deleting a jump host makes the connections that used it direct again (or use their folder's default jump host).
- A folder can give its connections a **default jump host** (*Folder settings…*); a jump host that is itself in the folder does not go through itself.

### Proxies (SOCKS / HTTP)

Some servers are reached through a SOCKS or HTTP proxy instead of (or after) a bastion, like PuTTY's *Connection → Proxy*. A proxy is defined once and chosen per connection, like a vault credential:

- *🔑 SSH keys & credentials → 🌐 Proxies → New proxy* (or *＋ New proxy…* in the connection dialog's **Proxy** field). Types:
  - **SOCKS5**, with an optional user name and password (RFC 1929) — encrypted, or a **vault credential**;
  - **SOCKS4 / SOCKS4a** (a user id, no password);
  - **HTTP CONNECT** with Basic authentication;
  - **WRM SOCKS tunnel of a connection:** a dynamic tunnel (`-D`) of one of your SSH connections. WRM starts it when a connection needs it and stops it after 30 minutes without traffic.
- **DNS at the proxy** (PuTTY: *Do DNS name lookup at proxy end*, default on): the proxy resolves host names (SOCKS5 domain names, SOCKS4a). Off: WRM resolves them and sends the address.
- **Everything a connection does over TCP goes through it:** SSH terminal, SFTP, search, transfers, tunnels, key deploy and password rotation, FTP/FTPS (also the data connections), RDP / VNC / Telnet (guacd connects to a local relay on `desktop_tunnel_bind`), web interfaces (opened through a local relay on `127.0.0.1`), *Test connection*, live status, network tools from the server, Redfish (when *Through the jump host* is ticked). **IPMI and Serial-over-LAN use UDP** and cannot go through a proxy: WRM refuses that combination.
- **Proxy and jump host together:** the proxy is reached through the jump hosts, then the target:

```
 WRM ──SSH──► bastion ──direct-tcpip──► proxy 10.1.1.1:1080 ──SOCKS CONNECT──► app-01:22
```

  The route is shown in the sidebar, the dialog, the terminal, *Sessions & recordings* and the audit log: `bastion → socks5://10.1.1.1:1080 → app-01`. Jump hosts can have proxies of their own.
- **Clear errors** that tell the proxy and the target apart:
  - `proxy socks5://10.1.1.1:1080: not reachable: connection refused` — the proxy itself;
  - `proxy socks5://…: authentication required` / `authentication failed: wrong user name or password` (HTTP: `407`);
  - `proxy socks5://… cannot reach 10.0.0.5:22: connection refused (SOCKS5 reply 5)` — the proxy works, the target does not answer (HTTP: `502` / `504`).
- **Live status** checks through a *WRM SOCKS tunnel* proxy only while that tunnel runs (it is never started or kept alive by a check); otherwise the state is *unknown*.
- A proxy address on the WRM machine that is a running SSH tunnel of **another user** is refused (*this address is an SSH tunnel of another user on the WRM server*): tunnel listeners have no login of their own.
- **Sharing:** share a proxy with users (or, administrators, with everybody): they can use it but never see its login. A proxy shared **with a password** is not reached through the grantee's own jump hosts (they could read the plain-text proxy login there). WRM tunnel proxies are not shared.
- **Folder default:** *Folder settings…* → *Default proxy*.
- **Policy `proxies`** (`all` / `admins` / `off`): who may define proxies. Everybody can use proxies shared with them.
- Export and import keep proxies by name (the export lists your proxies, passwords only in an export with secrets; an import creates the missing ones); *Duplicate* keeps the proxy.
- **Docker:** inside a container `127.0.0.1` is the container itself. `docker-compose.yml` maps `host.docker.internal` to the Docker host (`extra_hosts: ["host.docker.internal:host-gateway"]`): for `ssh -D 1080` or a proxy on the host use `host.docker.internal:1080`, and let it listen on an address the container reaches (not only `127.0.0.1` of the host). WRM warns when a proxy at `127.0.0.1` is saved while it runs in a container.

### SSH tunnels (port forwarding)

Like `ssh -L / -R / -D`, the tunnels of PuTTY and the SSH tunnel feature of mRemoteNG. You configure tunnels in a connection (**✏ Edit → 🔀 SSH tunnels**, or right-click → *Add tunnel…*) and start and stop them in the **🔀 Tunnels** panel in the top bar.

| Type | What it does | Example |
|---|---|---|
| **Local (-L)** | A port on the **WRM machine** leads, through the SSH connection, to a host:port **as seen from that server**. | Listen `127.0.0.1:2443` → target `10.0.0.5:443`: open `https://127.0.0.1:2443` to reach the web UI of a switch behind the server |
| **SOCKS proxy (-D)** | A SOCKS5 proxy on the WRM machine. Every destination is reached through the server. | `127.0.0.1:1080`: set it in the browser (e.g. FoxyProxy, *SOCKS v5, proxy DNS*) and browse the whole management network, or `curl --socks5-hostname 127.0.0.1:1080 http://10.0.0.5` |
| **Remote (-R)** | A port **on the SSH server** leads back to a host:port reachable **from WRM**. | `127.0.0.1:9000` on the server → `10.10.0.2:80`: the server downloads from a mirror on your side |

**Fields of a tunnel**

- **Listen:** address and port. `127.0.0.1` (default) means only programs on the WRM machine can use the tunnel. Leave the port empty for a free port chosen automatically (shown while running). Local and SOCKS ports must be 1024–65535. A network address (`0.0.0.0`, a LAN IP) makes the tunnel reachable by **anyone who can reach that port, without signing in to WRM** — administrators only, unless the policy allows it.
- **Target:** host:port, as seen from the server (local) or from WRM (remote). Not used by SOCKS.
- **Open as** `http`/`https` + path: adds a **🌐 Open** button that opens the tunnel in a new browser tab.
- **Start:** *Manual* (▶ in the Tunnels panel); *With terminal* (starts with the first terminal of the connection and stops about 15 s after the last one closes); *Always* (starts with WRM and keeps running, also after a restart).
- **Templates:** web interface (HTTPS 443 / HTTP 80), SSH, RDP and VNC to an internal host, PostgreSQL, MySQL/MariaDB, iDRAC / iLO / IPMI web, SOCKS proxy.

The hint under each tunnel says in plain words what it will do, e.g. *Connections to 127.0.0.1:2443 on the WRM machine go through Site1 to 10.0.0.5:443.*

**The 🔀 Tunnels panel** lists your tunnels by connection: state (🟢 running, 🟡 reconnecting, 🔴 error), listen → target, open / total connections, traffic ↑↓, since when and who started it, with **🌐 Open**, **📋 Copy address** (for SOCKS as `socks5h://…`), **▶ Start** and **■ Stop**. The number on the button counts running tunnels. Administrators can show **all users'** tunnels and stop them (also in *Admin panel → Overview*).

**Reliability:** every running tunnel has its own SSH connection (through the jump hosts), checks it every 30 seconds and **reconnects automatically** (after 2, 5, 10, 30, 60 s…) while its port stays open. Changing a connection restarts its running tunnels; deleting it stops them. Each start, stop (with the bytes transferred) and error is in the **audit log**.

**Where is the tunnel port?** On the computer where **WRM runs**:

- **WRM on your own PC** (the usual case): open `http://127.0.0.1:<port>` in your browser, or point any program (DBeaver, RDP client, `psql`, FileZilla…) at `127.0.0.1:<port>`.
- **WRM on a server you reach over the network:** a `127.0.0.1` tunnel is usable only on that server. An administrator can use listen address `0.0.0.0` (or the server's LAN address) and allow the port in the firewall — then everyone who can reach that port can use the tunnel, so prefer a restricted network. Or forward the port to your PC with plain SSH (`ssh -L 2443:127.0.0.1:2443 wrm-server`).

**Example — the web interface of a server behind a site gateway** (PC → WRM → `Site1` → `Site1-Webserver:443`):

1. Connection `Site1` (SSH to the site gateway).
2. *Edit `Site1` → 🔀 SSH tunnels → + Local (-L)*: listen `127.0.0.1:2443`, target `10.1.0.20:443` (the web server as seen from `Site1`), open as `https`.
3. *🔀 Tunnels → ▶ Start → 🌐 Open* → `https://127.0.0.1:2443`.

Even simpler: save it as a [web interface connection](#web-interfaces-http--https-connections) with jump host `Site1`.

### Web interfaces (HTTP / HTTPS connections)

Save the web interfaces of your devices — iDRAC / iLO / IPMI, switches, firewalls, NAS, hypervisors, monitoring — as connections with protocol **🌐 Web interface (HTTPS)** or **(HTTP)**: host:port, an optional **path** (e.g. `/login.html`) and an optional **jump host**.

- **Double-click** (or right-click → *Open web interface*) opens it in a new browser tab.
- **Without a jump host** the browser opens the address directly (`https://10.0.0.200/login.html`).
- **With a jump host** WRM opens a **temporary tunnel** on the WRM machine (`127.0.0.1:<free port>` → host:port as seen from the last jump host) and opens that. The tunnel is reused while you work and stops after 30 minutes without traffic; the Tunnels panel shows it as *temporary*.
- *Test connection* checks that the port answers (through the jump hosts).
- TLS goes end to end from your browser to the device; for self-signed device certificates the browser shows its usual warning.
- As with all tunnels, the temporary tunnel is on the WRM machine: when WRM runs on another computer, WRM shows the address instead of opening it (see *Where is the tunnel port?* above).

### Tags & inventory import (CSV, Excel, NetBox)

**Tags** describe connections: free words (`web`, `db`) and tags with a key — `env:` (environment), `site:` (location), `rack:`, `role:`, `tenant:`, `platform:`, `cluster:`. Tags are lower case, without spaces (`Rack A 12` becomes `rack:a-12`), at most 30 per connection.

- Set them in the connection dialog or for many connections at once (**🏷 Tags** after a multi-select; `-tag` removes one).
- The **🏷 tag bar** above the connections lists the most used tags with their counts; click tags to show only connections that have all of them (with *+N* for the rest). The search box finds tags too (`#rack:a12`).
- **Environments stand out:** `env:prod` (also `production`, `prd`, `live`) gets a red **PROD** badge and a red edge, staging/UAT a yellow, test/QA a blue and dev/lab a green badge — so a production server is never mistaken for a test one.
- Tags are kept by *Export* / *Import* and *Duplicate*.

**Import inventory** (*Settings → Data → Import inventory*):

*CSV / Excel*

- Choose a **CSV** file (comma, semicolon, tab or `|` separated — detected automatically; UTF-8, UTF-16 or **Windows-1250** as Excel saves it in Croatian, so `č ć š ž đ` come out right) or an **Excel workbook (.xlsx)** with a sheet selector. Old `.xls` files: save them as `.xlsx` or CSV first.
- WRM reads the first row as column names and **guesses the mapping** (English and Croatian names: *Name/Naziv/Hostname, IP/IP adresa/Address/URL, Port, Protocol/Protokol, User/Korisnik, Password/Lozinka, Folder/Mapa/Grupa, Tags/Oznake, Environment/Okruženje, Site/Location/Lokacija, Rack, Role/Uloga, Platform/OS, Jump host/Bastion, Path*). The preview shows the first rows under a field selector per column — change any column or set it to *ignore*.
- The host column understands `10.0.0.5`, `10.0.0.5:2222`, `root@10.0.0.5`, `10.0.0.5/24`, IPv6 and URLs (`https://idrac-01/` becomes a web interface connection, `ssh://admin@sw1:2222` an SSH connection). A *Platform* containing *Windows* makes an RDP connection. *Environment*, *Site*, *Rack*, *Role* and *Platform* become tags; a *Jump host* column links connections to a jump host by name.

*NetBox*

- Enter the NetBox address and an **API token** (read-only is enough), optionally filters (sites, roles, tenants, a NetBox tag, status), whether to use the **primary IP** or the **name (DNS)** as address, and the name of a custom field that holds the environment. *Load from NetBox* reads **devices** and/or **virtual machines** (all pages) and shows them; objects without an address are marked.
- Sites, racks, roles, tenants, platforms, clusters, the environment and NetBox tags become tags; Windows platforms become RDP connections.
- **Sync:** imported objects remember their NetBox id. Loading again marks them as *imported*; with **Update the ones imported before** their address, NetBox tags and folder follow NetBox (your own tags, the name and the login stay). Objects that disappeared from NetBox are listed — WRM never deletes connections on its own.
- *Remember address and token* stores the address, filters and the token (encrypted) for your account; the token is never shown again.

*For both:* choose the **login** of the imported connections — none (set it later), a **vault credential** (e.g. `root@dc1`) or a **stored SSH key** — a default user name and protocol, **folders** (from a column, one per site / role / tenant, all in one, or none) and extra tags for all. Rows with their own password keep it. Existing connections (same name, host and protocol) are skipped, so importing again is safe. The result lists what was imported, updated and skipped (with reasons).

### Import from mRemoteNG, PuTTY and OpenSSH

*Settings → Data*:

- **Import from mRemoteNG** — the `confCons.xml` file (`%APPDATA%\mRemoteNG\confCons.xml`, or *File → Export* in mRemoteNG):
  - containers become folders (`DC1 / Rack A`); SSH1/SSH2 connections become SSH connections, RDP, VNC and Telnet connections remote desktops (with their options), HTTP/HTTPS connections web interface connections (with the path of the URL);
  - **passwords are decrypted** (current AES-GCM format, older AES-CBC format, *FullFileEncryption*) and stored encrypted in WRM. If the file is protected with a **master password**, WRM asks for it;
  - inherited settings (*Inherit* user name, password, port, domain, SSH tunnel) are resolved from the parent folders;
  - the **SSH tunnel** setting (`SSHTunnelConnectionName`) becomes the **jump host**;
  - ICA, rlogin and other protocols WRM does not open are listed as *skipped*; connections that already exist (same name, host and protocol) are skipped too, so importing again is safe.
- **Import OpenSSH config** — `~/.ssh/config` (folder *SSH config*):
  - `Host` entries with `HostName`, `User`, `Port`; defaults from `Host *` and from wildcard patterns;
  - **`ProxyJump`** (also `user@host:port`; a jump host that is not its own `Host` entry is created) and `ProxyCommand ssh -W …` become the **jump host**;
  - **`LocalForward`, `RemoteForward`, `DynamicForward`** become tunnels that start with the terminal;
  - `IdentityFile` becomes *Key file* authentication (only if key files on the WRM server are allowed for you; otherwise a note asks you to add the key or a password);
  - `Match` blocks and wildcard-only hosts are skipped.
- **Import from PuTTY** — a registry export of the saved sessions (folder *PuTTY*). In `regedit` right-click `HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions` → *Export*, or run `reg export HKCU\Software\SimonTatham\PuTTY\Sessions putty.reg`. Both UTF-16 (regedit's default) and UTF-8 files are read:
  - session names are decoded (`My%20Server` → *My Server*); `HostName` (also `user@host`), `PortNumber`, `UserName` and `Protocol` are imported. *Default Settings* is not imported as a connection, but its values are the defaults of the other sessions;
  - **SSH** and **Telnet** sessions become connections; raw, rlogin and serial sessions (a COM port of the Windows PC) are listed as *skipped*;
  - `PublicKeyFile` gives a note: convert the `.ppk` with PuTTYgen (*Conversions → Export OpenSSH key*) and add it under *🔑 SSH keys*;
  - **proxies:** SOCKS4, SOCKS5 and HTTP proxies (`ProxyMethod`, `ProxyHost`, `ProxyPort`, `ProxyUsername`, `ProxyPassword`, `ProxyDNS`) become [saved proxies](#proxies-socks--http) named e.g. *PuTTY socks5 proxy.example.com:1080*, linked to the connections. Sessions with the same proxy share one; a proxy you already have (same type, address, login and DNS setting) is reused, so importing again creates nothing new. PuTTY's *SSH proxy* becomes the **jump host** (by name); Telnet and local-command proxies get a note. Needs the policy `proxies` to allow you to define proxies;
  - **with mRemoteNG:** connections whose *PuttySession* names an imported PuTTY session get that session's proxy (unless they have a proxy already). The order does not matter: the second import links connections that the first one created.

After the import a summary lists the imported connections, folders, jump host links and tunnels, what was skipped and what to check (e.g. connections without a user name or password).

**Size limit:** every import file (WRM export, mRemoteNG, PuTTY, OpenSSH config, CSV / Excel inventory) may be up to **20 MB**. The browser checks the size before sending; a larger file gets *The file is too large (… MB): the import limit is 20.0 MB* (HTTP 413 from the API).

### Windows, tabs & snapping

Every opened connection is a **window** inside the workspace. There is also a **tab** for it in the tab bar.

- **+ New Window** (or **Alt+N**) opens an empty window with quick buttons for your connections and **saved sessions**.
- **Title bar:** drag to move, **double-click** to maximize or restore. Buttons: *snap layouts*, **↻ Reconnect** (SSH windows), **📂 Files** (SSH terminals), 📡 *share terminal* (in a collaboration room), minimize, maximize, close.
- **📂▾ Files** in an SSH terminal's title bar is a menu that opens the **file manager of the same server** ([#24](https://github.com/vedranius/web-browser-RDM-public/issues/24)); a file manager of that connection that is already open is brought to the front and moves to the chosen directory:
  - **Open here:** the terminal's current directory. When the remote shell reports it with OSC 7, WRM uses it right away (bash: `PROMPT_COMMAND='printf "\033]7;file://%s%s\033\\" "$HOSTNAME" "$PWD"'` in `~/.bashrc`). Otherwise WRM asks the server over a separate SSH channel of the same connection: it finds the terminal's shell among the server's processes (`/proc` or `ps`, Linux and BSD), prefers the terminal's foreground program, and reads its working directory (`/proc/<pid>/cwd`, `pwdx`, `procstat` or `lsof`). Nothing has to be configured. When the directory cannot be found (no `/proc` / `ps`, another user's process after `sudo -i`, a non-OpenSSH server), WRM says why and offers home or `/`.
  - **Open /** and **Open home**.
- **Snapping:** left, right, **top and bottom halves**, the four **quarters**, and **maximize**. Use the buttons in the title bar, the compact *snap layouts* menu in narrow windows, or the tab's right-click menu.
- **Drag-to-edge snapping:** drag a window with the pointer to an edge (half) or a corner (quarter) of the workspace. A blue preview shows where it will go.
- **Windows can never leave the workspace:** moving and resizing stop at all four edges. When the browser is resized or zoomed, or a panel changes size, windows are moved and shrunk to stay visible. Snapped and maximized windows keep their slot.
- **Resize** from any edge or corner. Dragging a snapped window restores its previous size.
- **Tabs:** click to focus or restore, **drag to reorder**, **middle-click** to close, **right-click** for *Reconnect*, *Duplicate window*, snap layouts and *Close*. SSH tabs show the connection state (🟡 connecting, 🟢 connected, 🔴 disconnected).
- **Close confirmation** can be *Always*, *Never* or *Only when connected* (Settings).
- The browser warns you before leaving the page while terminals are connected or a transfer is running.

### Appearance, themes and accessibility

*Settings → Appearance* (`static/appearance.js`) changes how WRM looks for you; the choice is stored on the server (`/api/me/ui/appearance`) and follows you to every browser.

- **Themes:** Graphite (default), Midnight (OLED black), Fjord, Solar dark, Forest, Ember, Daylight, Paper, Solar light, High contrast dark and High contrast light. *Follow the system light / dark setting* switches between a dark and a light theme of your choice. Every theme also sets the terminal colours.
- **Your own colours:** every colour token (backgrounds, text, borders, accent, status colours and their text variants, terminal background, foreground and ANSI colours) has a colour picker and a CSS field; your changes apply on top of the theme and can be reset one by one or all at once. **Save as my theme** keeps the result (up to 30), **Export** / **Import** move a theme as JSON (`{"wrm_theme": 1, "name", "mode", "tokens", "term"}`).
- **Fonts and size:** interface font (including readable choices such as Atkinson Hyperlegible, Lexend and OpenDyslexic when installed), monospace font for terminals, editor and logs, interface size 70–160 %, spacing (compact, normal, roomy with larger click targets), rounded corners, line and letter spacing, heavier text.
- **Colour vision:** deuteranopia, protanopia, tritanopia and achromatopsia switch OK / warning / error / info colours and diffs to an Okabe–Ito based palette, optionally also in terminals. *Do not rely on colour alone* adds ✓ ✕ ! signs to status badges and toasts, shapes to connection status dots (● up, ◆ slow, ■ down) and underlines links.
- **Other options:** contrast (normal, more, maximum), minimum contrast in terminals (WCAG 4.5:1 or 7:1), animations (system, reduce, full), reduce transparency and blur, strong keyboard focus outline, underline links.
- **Custom CSS** for your own interface only (`appearance_custom_css`, default on).
- **Themes from addons:** an enabled addon offers its themes here (`ui.themes` in `addon.json` with `id`, `name`, `name_hr`, `mode`, `tokens` and/or `css`; the older single `ui.theme` stylesheet appears as one theme). Nothing is applied to users who did not pick it. An administrator can make any theme the default with **Default for everyone** or `appearance_default_theme`.

### Screen lock

The lock covers WRM and asks for the account password; terminals, file transfers, tunnels, AI and MCP sessions keep running. *Settings → Security → Screen lock* (per user, `/api/me/ui/lock`):

- **Lock now** (also 🔒 in the top bar and a keyboard shortcut, Ctrl+Alt+L by default).
- **When:** after inactivity (off, 1 minute to 4 hours or a custom time, with an optional 10/30/60 s warning; activity in any WRM tab counts, mouse movement optionally), when WRM is not visible (immediately or after 1/5/15 minutes, unless another WRM tab is visible), when the computer locks (browser idle detection, Chrome/Edge, asks for permission once), after the computer sleeps, and on every start or reload.
- **Lock screen:** blurred, dimmed or solid background, clock, your name, how many windows keep running, a message of your own, and a neutral tab title. Notifications are held while locked and shown after the unlock; desktop notifications do not show their text.
- **Unlocking:** the account password (local or directory/LDAP), or a personal 4–12 digit **PIN** (`screen_lock_pin`). Accounts that only sign in through an identity provider can set a PIN right after signing in, or sign out from the lock screen. Every lock, unlock and failed attempt is audited; five attempts per minute at most, and `screen_lock_max_failures` signs the browser session out after that many wrong attempts.
- **How strict** (`screen_lock_mode`): `server` (default) remembers the lock per sign-in session and refuses that session's requests with `423 Locked` until the unlock, so a reload, a new tab or the browser's developer tools do not get past it; `remember` keeps the lock over a reload but lets the page refresh underneath; `browser` is the page-only lock of earlier versions. All WRM tabs of the browser lock and unlock together.
- **Administrator limit:** `screen_lock_max_minutes` is the longest idle time a user may choose (0 = no limit); users cannot turn the idle lock off below it.

### Panel layout: dock, stack, float and lock

The side panels are yours to arrange (v12.6.8). *Connections*, *Sessions*, *Snippets*, *Clipboard* and the AI live view (while an AI app works) are **panels**, each with a tab. Panels share a tab bar in a **group**; groups sit in a **dock zone** (left, right, or at the bottom under the windows) or **on the canvas**, anywhere between your windows (v12.6.10).

- **Drag a panel by its tab** (a blue frame shows where it lands):
  - onto another tab bar: the panels share it as tabs (dropped between two tabs, it goes there);
  - onto the top or bottom 30 % of a panel: that column is split, the dragged panel above or below (left or right part in the bottom zone);
  - onto the **Left**, **Right** or **Bottom** strip at the edges of the workspace: it docks there as a new group;
  - anywhere else on the canvas: it is placed right there, between your windows. It shares their stacking order (click it to bring it to the front), stays inside the canvas when the browser or the side panels change size, and lands on the grid when *Snap to the grid* is on.
- **Resize:** the line between two groups, the inner edge of the left and right zones and the top edge of the bottom zone. A panel on the canvas moves by the empty part of its tab bar and resizes by its bottom right corner.
- **⋯ on every group:** *Dock on the left*, *Dock on the right*, *Dock at the bottom* (as a tab of the zone's first group), *On the canvas (anywhere)*, *Hide*, *Lock the layout*, *Panels…*, *Reset the layout*.
- **Panels** (▦ in the top bar, or in its ⋯ menu when the bar is narrow) lists every panel and where it is (left, right, bottom, on the canvas, hidden). Tick a panel to show it again where it was, untick to hide it. The AI live view is listed but appears only while an AI app works on a server.
- **Lock the layout** (in *Panels*, the ▤ Workboard menu or any ⋯ menu): no panel or window can be dragged, split, resized or moved (the snap and maximize buttons still work), and the ▦ button shows 🔒. Switching tabs, collapsing the side panels and showing or hiding panels still work. Dragging a tab while locked says how to unlock.
- **Reset the layout** puts every panel back: *Connections* left; *Sessions*, *Snippets* and the AI live view as tabs on the right with *Clipboard* under them.
- **Saved per user on the server** (`/api/me/ui/layout`), so the layout follows you to every browser, with a copy in the browser for an instant start. A zone without panels disappears. While the AI live view is shown in a side zone, that zone keeps its own width (up to half the window).
- **Phones** (below 760 px) keep the fixed layout: the connection list and the Sessions/Snippets/Clipboard drawer; the AI live view is a sheet at the bottom.
- **Workboard addons** docked as a panel are panels too; *Panels* lists them under *Workboard addons*, and hiding one puts it back onto the canvas as a window (see [the canvas](#workboard-the-shared-canvas-and-addons)).

### Workboard: the shared canvas and addons

The window area is **one canvas** (v12.6.10): terminals, file managers, desktops, **Workboard addons** and the panels you place on it share it, and you arrange it as you like. **▤ Workboard** (in the *Settings · Panels · Workboard* group of the top bar, or its ⋯ menu when the bar is narrow) opens the canvas menu. Addons start **empty**: you add only what you want from the catalog (**＋ Add addon**, also on the empty canvas), up to 24, the same addon several times if you like (for example two *Server load* addons for two groups of servers).

- **Addons are windows:** move by the title bar, resize from any edge or corner, snap by dragging to an edge or with the snap buttons, double-click to maximize, minimize, switch with the tabs. Title bar buttons: **↻** refresh (addons that fetch data; hover the title to see how old the data is), **⚙** settings, **⇥** dock as a panel, and the usual minimize, maximize and **✕ remove** (it asks first when settings or a note would be lost). Addons stay visible with any connection folder tab chosen. A new addon finds a free spot beside your windows.
- **Window menu** (right-click a title bar or a tab), for every window: snap layouts, **📌 Pin in place** (no moving or resizing until unpinned; 📌 shows in the title bar) and **Always on top**; for addons also *Settings*, *Refresh now* and *Dock on the right / left / at the bottom*.
- **Docked as a panel**, an addon is a panel of the [panel layout](#panel-layout-dock-stack-float-and-lock): stack it with *Sessions*, split a column, move it to another edge or anywhere on the canvas. Its bar keeps ↻, ⚙ and **⧉ Back onto the canvas as a window**, which returns it to its last place.
- **The ▤ Workboard menu:**
  - *Add addon…*
  - **Canvas:** *Snap to the grid* (windows and panels land on grid lines when moved, and moved edges when resized), *Grid size* 10 / 20 / 40 px, *Canvas background* (*only with the grid* shows dots while snapping is on; or always dots, lines, plain).
  - *Tile the windows* (a grid of all shown, unpinned windows), *Cascade the windows*, *Minimize all windows*, *Show all minimized windows*, *Minimize / Show the addons*, *Lock the layout*.
  - **Saved layouts:** *Save this layout…* stores the addons with their places and settings, the panel layout and the canvas settings under a name (up to 10); tick *Also the open terminals, file managers and desktops* to keep those windows too (like a [workspace session](#workspace-sessions)). Choosing a saved layout replaces your addons and panel layout; with windows in it, WRM asks whether to replace or add to your open windows. *Manage saved layouts…* applies or deletes them.
  - *Export the layout (JSON)* downloads the addons, canvas settings, panel layout and saved layouts as `wrm-workboard-<date>.json`; *Import a layout…* reads such a file (up to 512 KiB, everything checked as when loading), replaces your addons and panel layout and adds its saved layouts to yours. Exported files contain addon settings and notes, but no secrets.
  - *Panels…* and a short help text.
- **Every addon's settings:** your own title, *Colour* (a coloured edge on the title bar), *Text size* (small to extra large), the addon's own options, and *Remove*.
- **Saved per user on the server** (`/api/me/ui/workboard`, with a copy in the browser), so the canvas follows you to every browser. Places are kept as fractions of the canvas, so they fit other screen sizes.
- Addons load and refresh **only while visible**: not minimized and not completely covered by other windows (a countdown still ends on time).
- **Phones** (below 760 px): every window, addons included, is full screen and switched with the tabs; addons are not docked as panels there.

| Addon | Shows | Settings |
|---|---|---|
| 🟢 **Server status** | Down, slow (over 300 ms), unknown and up connections from the [live status](#live-updown-status), problems first, with latency or since when; a click opens the connection | connections (none chosen: all with a status), *only problems* |
| 📈 **Server load** | Per server: CPU, RAM (with total), load per CPU, uptime and disks (fullest first; more in a tooltip). Bars turn yellow at 75 % and red at 90 %. Each server names its source, *Monitoring (Zabbix agent)* or *read over SSH*, and the age of the values; errors (SSH refused, not Linux …) are shown as they are | up to 8 SSH/SFTP connections |
| 🔔 **Notifications** | Everything WRM notified you about, newest first, unread marked; *Mark all read*, *Clear*; the ▤ Workboard button counts unread ones | — |
| 🕒 **Clock** | Local time and date, and other time zones | IANA time zones (`UTC`, `Europe/London` …), seconds |
| ⎇ **Recent commits** | The last commits and pushes of a Git service (author, branch, short SHA, age, link to the commit on the Git server), refreshed every 5 minutes | service of the Git catalog, branch (empty: the branches the service tracks), 5 / 10 / 20 |
| 🔀 **Running tunnels** | Running tunnels with their local address; *Tunnels…* opens the tunnel panel | — |
| ⚡ **Quick connect** | Buttons with a status light that open the connection | up to 12 connections |
| 📝 **Notes** | A note to yourself (up to 2000 characters), saved with the addon | — |
| 🗂 **Saved sessions** | Your saved sessions with their number of windows; a click opens one | — |
| 🔗 **Links** | Your links to web pages (dashboards, wikis, tickets), opened in a new tab | up to 30 lines `Name \| https://address` (only http and https) |
| ⏱ **Timer** | A countdown with a progress bar that blinks and notifies (toast and, when allowed, a browser notification) when time is up, or a stopwatch; *Start* / *Pause* / *Reset*; keeps running across reloads | countdown or stopwatch, minutes (1–1440) |

**Where the values of *Server load* come from:** when the server has Monitoring (📊 in the top bar, Zabbix agent) on and its last sample is younger than 5 minutes, WRM uses that sample and does not log in. Otherwise WRM logs in over SSH with the connection's own credentials and runs one read-only command (`uname -s`, `/proc/loadavg`, `/proc/stat` twice one second apart, `MemTotal`/`MemAvailable` from `/proc/meminfo`, `df -P -k` without tmpfs/overlay mounts, `/proc/uptime`, `nproc`): at most once a minute per server while the addon is visible, one login per server at a time, results cached for 25 s. Reading over SSH works on Linux and can be turned off by an administrator (`workboard_ssh_metrics`); then only servers with Monitoring show values. Only your own SSH/SFTP connections can be chosen.

**Notifications in WRM:** every notification WRM sends you is also kept in your in-app list (newest 200), whatever channels or events you chose in *Settings → Notifications*, and even when an administrator turned sending off (`notifications_enabled`). Repeated reminders (expiring keys and credentials, a new version, upcoming Git jobs) are kept once a day.

### Remote desktop: RDP, VNC, Telnet

Windows servers (RDP), Linux desktops and KVM/IPMI consoles (VNC) and old network gear (Telnet) open in a WRM window next to your SSH terminals — no RDP client, no plug-in.

```
 browser ──WebSocket── WRM ──guacd protocol── guacd ──RDP / VNC / Telnet── server
```

**Setup (once):** WRM uses **guacd**, the proxy daemon of [Apache Guacamole](https://guacamole.apache.org/), which implements the protocols. Run it on the same machine as WRM:

| Where WRM runs | Install guacd |
|---|---|
| Debian / Ubuntu | `sudo apt install guacd` (listens on 127.0.0.1:4822) |
| Docker | `docker compose up -d` — `docker-compose.yml` contains a `guacd` service that shares WRM's network |
| Any Linux with Docker | `docker run -d --name guacd --network host --restart unless-stopped guacamole/guacd` |
| Windows / macOS | **nothing by hand:** WRM starts the official guacd container in Docker Desktop when `guacd_address` is local (`127.0.0.1:4822`). Docker Desktop must use **Linux containers**. Install/start it; otherwise run guacd on a Linux machine and set `guacd_address`. |

*Admin panel → Overview* shows whether guacd answers (and its version); *Test connection* on an RDP/VNC connection checks the server's port **and** guacd.

**Connections:** choose protocol **RDP**, **VNC** or **Telnet**, host (default ports 3389, 5900, 23), user, password and optionally a jump host. **🖥 Remote desktop options**:

- **RDP:** domain, security (automatic, NLA, TLS, RDP, Hyper-V console), keyboard layout of the server, what happens when the window is resized (adapt the desktop size, reconnect, or keep and scale), colors, start program, accept the server certificate, console (admin) session, sound, clipboard, wallpaper, **RD Gateway** (host, port, user, domain).
- **VNC:** colors, local or remote mouse pointer, view only, clipboard.
- **Telnet:** font size, colors, login and password prompt patterns (for automatic login).

**Working in a desktop window:**

- **Double-click** the connection (or right-click → *Open remote desktop*). The window shows the remote screen, scaled to fit; **⤢ Fit** switches to 1:1 with scroll bars, **⛶** to full screen. With RDP the remote desktop adapts its size to the window.
- **Keyboard and mouse** go to the remote computer while the window has the focus (click it). **Ctrl+Alt+Del** has its own button.
- **Clipboard:** text copied on the remote computer appears in WRM's clipboard panel and in your clipboard; **📋 Clipboard** sends text to the remote clipboard. **⌨ Type** types text as keystrokes — for login screens, BIOS/iDRAC consoles and other places without a clipboard.
- **Sound** from RDP plays in the browser.
- If the server asks for credentials that are not stored (e.g. an empty password), WRM asks for them.
- **↻** reconnects; the tab and the window show the connection state.
- **Behind a jump host**, WRM opens a temporary tunnel through the jump hosts for the session (listed as *Remote desktop (temporary)* in the Tunnels panel) and closes it with the session.
- Saved workspace sessions restore desktop windows too.

**Audit and recording:** every desktop session appears in *Sessions & recordings* and the audit log (`desktop.open`, `desktop.close`, `desktop.error`). With session recording on, WRM records the screen stream (no passwords: they are sent to guacd during the handshake and never appear on screen) and **▶ Replay** plays it in the browser with play/pause and seek; *⬇ .guac* downloads it (playable with Guacamole's tools).

**Security:** the browser never picks the target: WRM performs the handshake with the stored host, user, password and options, and afterwards forwards only display data and input instructions from an allow-list. Access follows the same rules as terminals (owner; share members with the *Operator* role or higher). Policies: `desktop_enabled` (`REMOTE_DESKTOP_ENABLED`), `guacd_address` (`GUACD_ADDRESS`, default `127.0.0.1:4822`), `guacd_auto` (Windows/macOS: start the local guacd container in Docker Desktop; default on), `desktop_tunnel_bind` (address of jump-host tunnels for guacd, default `127.0.0.1`).

### Out-of-band management, consoles & serial ports

**The BMC of a server** (iDRAC, iLO, XClarity, Supermicro, OpenBMC, any IPMI controller) is set in the connection: *Edit → ⚡ Out-of-band management*:

- **Type:** **Redfish** (the HTTPS API of current BMCs) or **IPMI** (IPMI over LAN with `ipmitool`).
- **BMC address** (`10.0.100.11`, or `host:port`), **login**: a user name and password (stored encrypted), or a **vault credential** (e.g. one `idrac-root` for all servers).
- **Through the jump host:** when the management network is only reachable from the bastion, WRM reaches the BMC through the connection's jump hosts — Redfish over an SSH channel of the last hop, `ipmitool` runs **on** the last hop (IPMI uses UDP).
- **Serial console command** for the BMC's SSH (`console com2` for iDRAC, `vsp` for iLO, `console 1` for XClarity; suggested from the BMC's maker) and its SSH port.

**⚡ Power & status** (right-click → *Power & status (BMC)…*):

- Power state, health (with fault messages over IPMI), maker, model, serial number, BIOS, CPUs and memory, BMC model and firmware, power draw and inlet temperature (as far as the BMC tells).
- **Power on**, **Shut down** (graceful), **Restart** (graceful), **Force off**, **Force restart**, **Power cycle**, **NMI** — only what the BMC offers. Everything except *Power on* asks first and explains what happens; every action is in the audit log (`bmc.power`, `bmc.power_failed`).
- Redfish certificates are mostly self-signed: WRM pins the certificate on first use like an SSH host key and refuses a changed one (accept it after checking, like a host key).

**📟 Serial console** (right-click → *Serial console*, or the button in *Power & status*) opens a terminal window with the console of the machine — BIOS/UEFI setup, boot loader, kernel messages, a login prompt when the network is down:

- **Redfish BMCs:** WRM logs in to the BMC's SSH with the BMC login and types the console command (iDRAC `console com2`, iLO `vsp` …).
- **IPMI:** Serial-over-LAN (`ipmitool sol activate`, an old session is deactivated first), on the jump host or on the WRM server (Linux). `~.` ends it.
- Consoles are terminal sessions like any other: audited, recorded (`protocol` *console* / *sol*), visible to administrators — and only the connection's owner opens them (not share members). Saved workspace sessions restore console windows.

**🔌 Serial ports of the WRM machine** (USB console cables, `ttyS0`): protocol *Serial port*, the device name (`ttyUSB0`, `ttyACM0`, `ttyS0`) as host and the line settings — speed (default 9600; 115200 is common too), data bits, parity, stop bits, flow control (none, RTS/CTS, XON/XOFF). *Test connection* opens the port; a double-click opens it in a terminal (raw, recorded). Serial ports belong to the WRM server like its key files: by default only administrators may add them (policy `serial_ports`: off / admins / all); WRM must run on Linux, as a user that may open the device (group `dialout`).

Policies: `bmc_enabled` (`BMC_ENABLED`), `serial_ports`.

### Snippets & run on connect

Snippets are saved commands. Open them in the right panel (**⚡ Snippets** tab), or press **Ctrl+Shift+Space** in a terminal (or ⚡ in its title bar) for a quick picker.

- **Click** a snippet: it is typed into the focused terminal and run. **Shift+click** (or ⎘) types it without Enter, so you can edit it first. **Right-click:** run, insert, copy, edit, duplicate, delete.
- **Quick picker:** type to filter, ↑ ↓ to choose, **Enter** runs, **Shift+Enter** inserts, **Esc** closes, **＋ New snippet** saves the selected terminal text as a snippet.
- **Groups** (Diagnostics, Services, …), a description, and the scope: **all connections**, the connections of **one folder**, or **one connection**. The picker only offers snippets that apply to the terminal's connection.
- **Multi-line snippets:** every line is one command.
- **Variables** are filled in for each terminal: `{{host}}`, `{{port}}`, `{{user}}`, `{{name}}` (connection), `{{folder}}`, `{{wrm_user}}`, `{{date}}`, `{{time}}`. **Prompts** are asked before the snippet runs, with a live preview: `{{?Service}}` or with a default `{{?Service=nginx}}`. Example: `journalctl -u {{?Service=nginx}} --since "{{?Since=1 hour ago}}" --no-pager`.
- **Run on connect:** tick *Run on connect* and WRM types the snippet into the shell as soon as a terminal of a matching connection is connected (also after a reconnect) — `sudo -i`, `cd /srv/app`, `tmux attach || tmux new`, `export KUBECONFIG=…`. Right-click a connection or a folder → **⚡ Run on connect…** creates one. Order: all-connection snippets, then folder, then connection snippets. A message lists what ran; the audit log records it (`terminal.auto_run`).
- **Shared snippets:** administrators can share a snippet with all users (read-only for them, all connections, never auto-run) — a team runbook in the terminal.
- **✨ Add example snippets** creates a starter set (disk, memory, top processes, failed services, journal errors, listening ports, big files, service status/restart…).
- Snippets are included in *Export* / *Import*, copied with *Duplicate*, and removed with their connection or folder.
- Dangerous-looking commands (see below) ask for confirmation before they run.

### Folder bookmarks

Bookmarks are named remote directories, like WinSCP's: the same deep folders (`/opt/app/servers/<server>/downloads`) are one click away on every server.

- **★ in the file manager** (next to the path bar): **☆ Bookmark this directory…**, the bookmarks of this connection, and **⚙ Manage bookmarks…**. Picking a bookmark opens that directory. If it does not exist on this server, a message says so and the current folder stays open.
- **★ in a terminal's title bar** and in the **Ctrl+Shift+Space** picker (after the snippets): picking a bookmark types `cd -- '<path>'` into the terminal (shell-quoted; a leading `~/` stays outside the quotes so the shell expands it). In a [broadcast](#broadcast-input), **📣** next to a bookmark sends the `cd` to every terminal of the group, each with its own connection's variables.
- **Scope:** **one connection**, the connections of **a folder**, the connections with **a tag**, or **all SSH / SFTP / FTP connections** (global).
- **Variables**, filled in by the server for each connection: `~` (home directory), `$USER` / `${USER}` / `{user}` (the connection's user name), `{host}` (without the port), `{name}` (connection name). Example: `/opt/app/servers/{name}/downloads`.
- **Start directory:** tick *Start directory* and the file manager of a matching connection opens there; a terminal types the `cd` when it connects (also after a reconnect), before the [run-on-connect](#snippets--run-on-connect) snippets. The most specific start directory wins: connection, then tag, then folder, then global. Right-click a connection or a folder → **★ Start directory…** creates one.
- **Name, colour and note**; **Manage bookmarks** renames, reorders (↑ ↓), moves bookmarks between scopes and deletes them.
- **Sharing:** a global bookmark marked *Share with share members* is offered (read-only) to the people who use your connections through a [share](#sharing-members-roles--links).
- **Import from WinSCP:** *Manage bookmarks → Import WinSCP.ini* reads the remote directory bookmarks (`[Configuration\Bookmarks\Remote\…]`) of a WinSCP configuration file and adds them as global bookmarks, with the WinSCP site in the note. Bookmarks that already exist (same name and path) are skipped.
- Bookmarks are included in *Export* / *Import*, copied with *Duplicate*, and removed with their connection or folder.

### Broadcast input

Type the same command into many servers at once — like *MultiSSH*, *cssh* or mRemoteNG's multi-SSH.

1. Open the terminals (e.g. all web servers of a cluster).
2. Top bar → **🧰 Tools** → **📣 Broadcast…** → select the terminals → **Start broadcast** (asks once).
3. Everything you type in one of them now goes to all of them. An orange bar shows how many terminals receive it; included windows get an orange frame. **📣** in a window's title bar adds or removes that terminal; **■ Stop** ends it.

Safety:

- **Dangerous commands ask first:** before Enter is sent for a line that looks dangerous — `rm -r/-f`, `shutdown`/`reboot`/`halt`, `init 0/6`, `systemctl stop/restart/disable…`, `mkfs`, `dd … of=`, `fdisk`/`parted`, `wipefs`, writing to `/dev/sd*` or `/etc/…`, `kill -9`, `pkill`, `chmod 777`, `chown -R`, `iptables -F`, `ufw disable`, `DROP/TRUNCATE TABLE`, `DELETE FROM`, `crontab -r`, package removal, `docker rm`, `kubectl delete`, `terraform destroy`, a fork bomb … — WRM shows the command and the number of terminals; **Cancel is the default button**.
- **Pasting several lines** into a broadcast asks for confirmation too.
- **Snippets** run on every terminal of the broadcast, each with its own `{{host}}`/`{{user}}`, after one confirmation.
- The check works on what you typed in this broadcast (not on commands recalled from the shell history with ↑).
- Each terminal records when it joined and left a broadcast (`terminal.broadcast`, with the number of terminals) and, as always, the session recording shows what happened on each server. Administrators can turn the feature off (`broadcast_enabled`).

### Live up/down status

WRM checks every saved connection in the background and shows the result in the sidebar:

- 🟢 **up** (hover: latency, since when, SSH server version), 🟡 **slow** (more than 300 ms), 🔴 **down** (pulsing; hover: since when and why — *connection refused*, *timeout*, *host name not found* …), a **ring** for connections behind a jump host.
- A folder shows how many of its connections are down (**2↓**). When something is down, a red bar above the tree shows **only the connections that are down** with one click.
- **Notifications:** a message when a server goes down or comes back, and a desktop notification while WRM is in the background (*Settings → General → Notify when a server goes down*). Outside the browser: e-mail, chat or push through [Notifications](#notifications-e-mail-chat-push-webhook).
- **Through a proxy:** connections with a proxy are checked through it (and through their jump hosts when *Check through jump hosts* is on).
- **🔄 Check status now** (right-click a connection or a folder) checks immediately — also **through the jump hosts** — and shows the result per connection.
- **How it checks:** a TCP connection to the connection's port (SSH 22, FTP 21, web 80/443 or the port you set). SSH checks read the server banner and answer with `SSH-2.0-WRM_status_check` before closing, FTP checks read the 220 greeting and send `QUIT` — the same as Nagios `check_ssh`/`check_ftp`. Nothing logs in. A failed check is repeated once before a server counts as down. Each distinct host:port is checked once per round, however many connections and users point to it.
- **Behind a jump host** a connection shows the state of its jump host (a red ring when the jump host is down). With the policy *Check through jump hosts*, WRM logs in to the jump host once per round and checks the ports behind it through that SSH connection.
- Untick **Monitor up/down status** in a connection to leave it out (e.g. servers that are often off on purpose).
- Policies: `status_enabled` (on), `status_interval_seconds` (60, 15–3600), `status_jump_checks` (off).

> **fail2ban / IDS:** the check is an ordinary connection without a login. Default fail2ban `sshd` filters do not count it, but the *aggressive* / *ddos* modes may. Add the WRM server to `ignoreip`, raise the interval, or untick monitoring for such hosts.

### Logs: viewer, search and log settings

*Monitoring → Logs*, choose a server.

- **Discovery** runs over the connection's SSH account without sudo: one read-only command reads systemd units, Supervisor programs and Docker containers, a second one runs `find` over the scan paths with a time budget. Accounts without a shell (SFTP only) fall back to an SFTP walk. Scripts and code (`.py`, `.sh`, `.js` …) are never listed as logs, even in a log folder.
- **File list:** sort by newest, oldest, name, size, service, operator or path; filter by operator, service, folder and name; tick files to search only those. Each file shows its operator and service labels, whether it sits in a **log folder** (`/var/log`, `log`, `logs` …) or an **app folder** (scripts, application), its folder, size and modification time. Services are compared without case, so a log folder `/var/log/VideoX_NOVA` and a script folder `/home/scripts/Videox_NOVA` are one service with both folders.
- **Folder tree** (*View: Folder tree*): logs grouped under their **main folder** (the scan paths, the Git workspace server roots and the main folders from *Log settings*) and its subfolders. Open or close a folder with a click; its checkbox ticks every shown log in it. The **Folder** filter lists the same tree and keeps one folder and everything below it, in either view.
- **Operators:** labels from the path patterns and fixed folders come first. With *Guess operators* on (default), a word that repeats across service and folder names is an operator unless it is the product (the first word of several names) or an environment word (`qa`, `test`, `dev`, `prod`, `bkp` …): `VideoX_NOVA`, `VideoX_NOVA2`, `novatv` and `INGEST_SFTP_novatv` give operators `nova` and `novatv`. Operators where one name starts with another (at least four letters) form a group; the operator filter offers the group (*nova (5) · nova, novatv*) and each member. Guessed operators are marked `≈`.
- **Log tabs:** line numbers, level colours (a stack trace keeps the level of its line), highlighting of times, IPs, URLs, HTTP status codes, UUIDs and key=value pairs, **find** in the log (regex, case, ↑ ↓, *only matching lines*), level chips (errors, warnings, info, debug, other), **Follow** (new lines every 2.5 s; for the journal by cursor), *Load older lines*, *Go to line*, wrap, line numbers, copy, download and *open a terminal here*. Keys: `/` find, `n` / `N` next and previous hit, `g` go to line, `f` follow, `w` wrap.
- **Search tab:** the search runs **on the server** (grep), so large and old logs are searched quickly: literal or regex, case, whole word, context lines, *From* / *To* in your date format with quick ranges (15 minutes to 30 days), the time zone of the logs, the selected or the shown files. Hits are sorted by time (either order) with level chips, a per-file filter and a timeline you can zoom. A click opens the file at that line.
- **⚙ Log settings** (per server, only its owner can change them): scan paths, **main folders** for the folder tree, *Guess operators*, excluded names, **path patterns** that give operator and service labels (default `/home/*/{operator}/{service}/{logdir}/**`, `/srv/{operator}/{service}/{logdir}/**` …), extra log name patterns, depth, maximum files and age, time budget, rotated and compressed files, empty files, the systemd journal, systemd/Supervisor/Docker metadata, explicit folders and journal units, and the method (`auto`, `ssh`, `sftp`). Zabbix settings stay in the Zabbix dialog.
- **🤖 AI log assistant:** a chat panel next to the logs that searches every discovered log with `search_logs`, reads around the hits and explains what failed, since when, how often and which services are affected. It is always read-only. Its chats are listed separately from terminal chats, with title, date and time.
- **Date and time format:** EU (`dd.mm.yyyy hh:mm`) by default; ISO, US or the browser's locale in *Settings → Date and time format* (saved per browser).

### Notifications (e-mail, chat, push, webhook)

WRM tells you about events outside the browser too. **Administrators** set up the channels (*Admin panel → Notifications*), **users** choose what they receive and where (*Settings → Notifications*). The in-app messages and browser notifications stay as they are.

- **Channels:**

| Type | What it needs |
|---|---|
| E-mail (SMTP) | server, port, security (STARTTLS / TLS / none), optional login, from, to. A login is only sent over TLS or to a server on the WRM machine |
| Telegram | bot token (@BotFather), chat ID; optionally the address of a local Bot API server |
| Slack / Mattermost / Rocket.Chat | incoming webhook URL (Slack format) |
| Microsoft Teams | a *Workflows* webhook URL (*Post to a channel when a webhook request is received*); sent as an Adaptive Card |
| Discord | webhook URL |
| ntfy | server (ntfy.sh or your own), topic, optional access token |
| Gotify | server, application token |
| Pushover | application token, user or group key |
| Webhook | URL; WRM POSTs JSON (`title`, `text`, `events`). With a signing secret the header `X-WRM-Signature: sha256=<HMAC-SHA256 of the body>` lets the receiver check it |

- Secrets (passwords, tokens, webhook URLs) are **encrypted** and never shown again; leave a field empty to keep it. **📨 Send test** sends a test message at once and shows the service's answer (*HTTP 400 chat not found*, *SMTP login: …*). The last delivery and the last error are listed per channel.
- *Users may enter their own recipient* (e-mail, Telegram, ntfy, Pushover): each user can enter their own address, chat ID, topic or user key (e-mail: plain addresses only); otherwise the channel's is used. A channel without a default recipient needs the user's own. Slack-format and Discord messages cannot mention `@channel` / `@everyone` through connection names.
- **Events:** a server went down, a server is up again (only the owner's own monitored connections; connections with monitoring off are left out), credential rotation is due (again every 7 days while it stays due), credential rotation is incomplete (again every day).
- **One message per round:** what happens in one status round (or within a few seconds) goes out as **one digest** per user and channel, and a channel sends to the same recipient at most once per `notify_min_interval_seconds` (60); what comes in meanwhile goes into the next message.
- **Quiet hours** (per user, in the browser's time zone): messages wait and are sent together when the quiet hours end.
- Messages are in the user's language (English / Hrvatski). Waiting messages survive a restart; a failing channel is retried with a growing delay and gives up after 6 attempts.
- Policies `notifications_enabled` (on) and `notify_min_interval_seconds`. Channel changes and tests are audited (`admin.notify_channel_*`), subscriptions too (`notify.settings_changed`).

### Git workspace: compare services

**⎇ Git** in the top bar opens a full-screen workspace (its code, `static/git.js`, loads only then). It answers *which version of which service runs where, and what was changed by hand* — for any GitLab or GitHub (cloud or self-hosted) that you connect. WRM ships no services or organisation-specific configuration. Servers need no git, Python or internet access: WRM reads the Git provider's API itself and works on servers over its existing SSH connections, so jump hosts, proxies and the vault apply. Checks only read; servers are changed only by the runs described in the next section.

- **Sources** (*Settings*): GitLab (API v4), GitHub (REST; `github.com` or Enterprise `<url>/api/v3`) or Gitea (API v1, `<url>/api/v1`) with a **read-only token** (encrypted, never sent back to the browser). *Test* lists the projects of the catalog's groups / organisations (with subgroups). Self-signed Git servers are pinned on first use.
- **Few API calls:** one recursive tree listing per version (cached forever, a commit never changes) gives the blob ID of every file; a file's content is downloaded only for a blob WRM has not seen yet and cached by blob ID — the same file in 20 versions is fetched once.
- **Offline bundles** (*Settings → Offline bundles*): when WRM cannot reach the Git server, import a `.tar.gz` built elsewhere — one top directory with `bundle.json` (per service the target ref and the hash and history of every file) and `payload/<service>/<path>`; `deploytool.py` and `README.txt` are ignored, protected files of the payload are never used. The list shows the bundle's age, author and source. *Services → Export bundle* writes the same format for servers that only a file-based deploy tool can reach. *Targets from*: automatic (the Git API, otherwise the newest bundle), the Git API or a bundle.
- **Service catalog** (*Services*): name, project, ref (`tag:latest`, `tag:v2` = the newest `v2.x`, or a branch), branch for tags, tag filter (regex), **subdirectory** of the repository that is the installation root, include / exclude globs, **protected** per-host files (`config*`, `*.ini`, `.env`; matched like Python's `fnmatch` on the path and the file name) that are never overwritten and only shown, **fingerprint** files that recognise an installation, install hints (directory names) and kind (app, library, tool). *Suggest from repository* fills an entry from the repository tree. *Import catalog* (merge or replace) and *Export catalog* use catalog JSON (`gitlab.url` / `groups`, `fallback_branches`, `server_roots`, `apps`, `ignore_dirs`). Services of imported bundles work without a catalog entry.
- **Refs:** only tags whose commit is on the chosen branch count (*branch-aware*); versions sort numerically (`v1.10` after `v1.9`); `tag:latest` without a matching tag uses the branch head, and a missing branch falls back to `fallback_branches` or the default branch — both with a warning. *Ref* in the overview: as in the catalog, one branch for all, or each project's default branch.
- **Discovery** (*Installations → Discover*, and with every check): one SSH call per server walks the server roots (`/opt`, `/srv`, …) to depth 3 and finds installations by their fingerprint files. Paths that look like copies or backups are skipped (segments with `bkp`, `backup`, `bak`, `old`, `orig`, `prev`, `copy`, `kopija`, `stari`, `recyclebin`, `trash`, `snapshot`, policy `git_backup_words`, plus the catalog's `ignore_dirs`). The environment comes from the path: `/srv/scripts/test/App` → *test*, `/opt/App_prod` → *prod*. Installations can also be added by hand.
- **Comparison:** the server hashes its files with `tr -d '\r' < f | sha256sum` (or `shasum` / `openssl`); WRM compares them with the target (SHA-256 with CRLF → LF) and with the earlier versions of every file (the earlier tags of the branch, *History depth* in *Settings*). Files are **ok**, **old** (*n behind*, with the tag it matches), **modified**, **missing**, **extra** (only on the server) or **protected**; an installation is **review** when a file is modified, **needs update** when a file is old or missing, otherwise **up to date**. `VERSION.md` and `updates.jsonl` of deploy tools, `.deploy-bak`, `.git`, `__pycache__` are left out.
- **Version from the code:** the deployed version is concluded from the compared files, not from `VERSION.md`. When every tracked file matches one known version, that version is shown exactly; an older consistent version is looked up in its Git commit tree; a mix or a hand-edited file shows **Mixed / modified** with the per-file states and is never guessed as one version. The checkbox next to *Check now* (off by default) also compares `VERSION.md` and warns when it disagrees; deployments and updates still write it for compatibility.
- **Details:** the detected version, commit and date, the optional `VERSION.md` version (when the checkbox is on), the systemd / supervisor units whose unit file mentions the directory, the file states and a coloured **diff** of a file (server → target).
- **Overview:** the target of every service (version, latest tag, source, warnings) and a **servers × services matrix** with the state, environment, detected version and *n behind* of each installation, filtered by state, environment, folder, tag and text.
- **Periodic checks** (*Settings → Periodic checks*): while WRM runs, the chosen servers are checked every `git_check_interval_minutes` (60). Checks never change anything. They notify (one digest per round) about **a new version** of a service, **drift** (an installation that turns to *review*) and **unreachable servers** — subscribe to the *Git* events in *Settings → Notifications*.
- Everything is per user (sources, catalog, installations). Policies `git_enabled` (hides the workspace), `git_checks` (who may run checks: everyone), `git_check_interval_minutes`, `git_backup_words`. Checks, bundle imports / exports, source and catalog changes and viewed diffs are audited (`git.*`).

### Git workspace: service functionality and configuration

Open **Monitoring → Functionality**, or the **Functionality** button in Git Simple mode. It uses the existing service catalog and installations, including GitLab sources. Manual and periodic Git checks inspect all Supervisor workers on each selected server, failed systemd services, installation disk usage and Linux memory usage (90% warns). Each installation checks discovered/configured units and reports healthy, warning, failed or unverified state with timestamps and bounded error logs. A running Supervisor worker never hides a failed sibling. Missing installations remain visible. Read access failures, absent tools, no runtime check and incomplete scans cannot be reported as healthy.

Edit a service to set optional runtime units, an HTTP health URL or a TCP `host:port`. HTTP accepts 2xx, follows no redirects and verifies TLS; probes originate through SSH from the selected server. Unit commands use the SSH account, without automatic sudo or restart. Commands are bounded by a 30-second runtime step (individual commands use a six-second timeout where available); endpoints use six seconds. The server inventory caps workers at 256 and failed systemd units at 64, with an explicit warning if incomplete. Logs are capped at 16 KiB per check and redacted before storage.

**Configuration** reads configuration files on demand and displays section, variable/function, configured value and comment description. Explicit relative `config_files` paths take precedence; otherwise WRM discovers `.ini`, `.conf`, `.cfg`, `.env*` and config/settings files. JSON is flattened into setting paths; INI and assignment-style settings are parsed without executing source code. Unrecognized lines are counted. Secret-name values, credential URLs and known token formats are masked; configuration contents are not stored in the database. Up to 20 files of 256 KiB each and 2,000 entries per file are inspected. Symlink escapes, unreadable/missing files and limits are reported.

Optional catalog fields (existing catalogs remain valid):

```json
"functionality": {
  "units": [{"kind": "systemd", "name": "demo-api.service"}, {"kind": "supervisor", "name": "demo-workers"}],
  "http": "http://127.0.0.1:8080/health",
  "tcp": "127.0.0.1:8080",
  "config_files": ["config.ini", ".env"]
}
```

Subscribe in **Settings → Notifications** to `git.functionality_problem`, `git.functionality_recovered`, `git.server_problem` and `git.server_recovered`. The server events include workers with no catalog association. Notifications report transitions/changed warnings and do not repeat an identical result on each check.

### Functionality configuration profiles

In **Monitoring → Functionality → Config profiles**, upload a reference such as `storage_backend.ini` or `config.py`, choose the catalog service, relative path inside each installation and optional operator label. Run a profile check across that service's installations (or selected installation IDs through the API). The matrix shows hostname, connection, service, environment, operator, path, timestamp, section/key/function and reference/actual values. Search matches server/operator/settings. Values are marked matching, different, additional, missing or masked.

Profiles use the existing static parser: Python expressions are displayed as source text, never evaluated. Parse gaps, missing files, unsafe symlinks and incomplete inspections are explicit. Up to 50 profiles per account, 256 KiB reference files, 50 installations per check and four concurrent reads are supported. Structured reference values are masked and encrypted in SQLite; live remote values are read on demand. These profiles inspect WRM catalog installations; Medium uses its own native config and does not automatically register its private projects in that catalog.

### Git workspace: Medium original deploy tool

Select **Medium** next to Simple and Advanced. Use the combined ZIP fields and **Prepare and run trusted ZIPs** for retained dependency setup and automatic launch; [setup guide](docs/V12.4.5_GUIDE.md). Advanced individual import controls remain available. The original tool is required as a private ZIP and is not bundled in this public repository. An administrator imports the approved tool ZIP; WRM creates its Python virtual environment and installs the archive's pinned `requirements.lock`. Setup may take several minutes and reports status. Each user imports their deploy-config ZIP (`servers.yaml`, `environments.yaml`, `projects/*.yaml`) and optional SSH ZIP (`config`, keys and `known_hosts`). Confirm executable-code/config imports in the UI. Stop the runtime before replacing its config/profile/settings; stopping is refused while a comparison or background deployment is active.

The original tool runs unchanged behind WRM's authenticated `/medium/` bridge, embedded in the workspace or opened in a full window. Its own project editor, server/tag/branch selection, comparison, exclusions, local build, dry run, confirmation, deploy, per-server progress, backups, rollback, history and config Git controls remain available. See the [parity record](docs/MEDIUM_PARITY_v12.4.3.md). Closing the Git window leaves the process running; restarting WRM stops it. Runtime data remains on disk.

Use **Runtime settings** for the original `DEPLOY_CONFIG_REPO`, pull/check cadence, user email/hostname, SSH options, build user and timeout. Preferences are encrypted; stored command/credential values are masked on read. A config Git clone lives separately from the imported flat config. Git identity, keys, known-host verification and repository write access must be configured for native Git push. Uploaded SSH files use the user's private runtime home; a trusted administrator may provide `WRM_MEDIUM_SSH_AUTH_SOCK` for users allowed server-side keys.

Medium runs the original tool in the managed Docker runtime on Linux, macOS and Windows (v12.6.6); Python, git, SSH and rsync come with its Linux image, and WRM installs Docker itself (see below). The host runtime (`WRM_MEDIUM_RUNTIME=native`, and the default on systems without Docker such as FreeBSD) needs Python 3.12+, git, OpenSSH and rsync on the WRM host (`WRM_MEDIUM_PYTHON` can select Python). Builds need their original toolchain and OS user permissions. Git and target hosts must be reachable from the runtime. **Grant Medium only to trusted operators:** editable configs/builds/deploy commands execute as the WRM OS account. Per-user directories/processes are not an OS sandbox. WRM checks authentication, `git_medium`, Git checks and action policies and records the action boundary; the original tool supplies its native review/locks/history. Simple and Advanced continue using WRM's existing engine and catalog.

### Docker for Medium, installed by WRM

Medium uses the same Docker runtime on every operating system (v12.6.6). When Docker is missing, step 2 says which Docker WRM will install and how, and an administrator presses **Install Docker** once. That consent is stored with the administrator's name and time and audited (`git.medium_docker_consent`); from then on a tool ZIP import or **Prepare runtime / retry** installs, starts and prepares everything without further clicks. Each step (*Installing Docker*, *Downloading Docker Desktop*, *Checking the installer's signature*, *Starting Docker*, *Waiting for the Docker engine*) is shown with its time, its output and what to do when it fails, and stays in the activity log.

| WRM host | What WRM does |
|---|---|
| Linux, WRM runs as root | Installs Docker Engine from the distribution's packages (`apt-get install docker.io`, Fedora `moby-engine`, Amazon Linux `docker`, Docker's `docker-ce` repository on RHEL/CentOS/Rocky/Alma, `zypper`/`pacman`/`apk` `docker`) and starts its service (`systemctl enable --now docker`, OpenRC or `service`). From WRM's own sandboxed systemd service (`ProtectSystem=strict`) the commands run through `systemd-run` as a separate unit. WRM's proxy variables (`HTTPS_PROXY`, `NO_PROXY`, …) are passed on. The installation has its own 45-minute budget, so a package manager is never stopped halfway. |
| Linux, WRM runs as a normal account | With passwordless `sudo`, the same through `sudo -n`, then adds WRM's account to the `docker` group (restart WRM once). Otherwise step 2 shows one command to run as root: `sudo wrm -setup-docker -service-user <account>`. It installs and starts Docker Engine, adds the account to the `docker` group and restarts the WRM service. |
| Linux, WRM in a container | Nothing: mount the host's Docker socket and CLI into WRM's container. |
| macOS | Downloads `Docker.dmg` from docker.com, checks that it is signed by Docker Inc. (team `9BNSXJN65R`) and installs it with the accepted licence for the user signed in on the Mac (as root or with passwordless `sudo`; Docker Desktop then runs in that user's session, so someone must be signed in); without administrator rights Docker Desktop is copied to `~/Applications` and started, and its first-start window needs one confirmation on the Mac. |
| Windows | Downloads the Docker Desktop installer from docker.com, checks its Authenticode signature (Docker Inc.) and installs it quietly for the signed-in user (`install --user --quiet --accept-license --backend=wsl-2`). The first time Windows may ask once to enable WSL (an administrator prompt) and to restart. As a Windows service WRM cannot run Docker Desktop: install it for the signed-in user or run WRM as that user. |

Docker Desktop (Windows, macOS) is free for personal use, education, non-commercial open source and small businesses (fewer than 250 employees and less than USD 10 million annual revenue); larger organisations need a paid subscription. WRM shows this and the [Docker Subscription Service Agreement](https://www.docker.com/legal/docker-subscription-service-agreement/) and installs Docker Desktop only after the administrator accepts it. Docker Engine on Linux is open source and needs no licence acceptance. The policy `medium_docker_install` switches automatic installation off; the administrator's consent can be withdrawn with `POST /api/git/medium/docker` `{"accept": false}`. A running Docker that refuses WRM's account (*permission denied*) is reported with the same `-setup-docker` command.

**Proxies and antivirus that inspect HTTPS.** Medium's Docker runtime trusts the root certificates the WRM host trusts, in addition to the image's own: the Windows certificate stores (user, machine, group policy, enterprise), the macOS keychains or the Linux/BSD CA bundle. WRM writes them to `<database>.medium/host-ca.pem` (public certificates only) and mounts it read-only into each Medium container, where pip, Python, git and curl use the combined bundle. Without this, pip inside Docker fails with *self-signed certificate in certificate chain* on networks whose proxy or antivirus inspects HTTPS. Turn it off with the policy `medium_host_ca`; when the proxy's root is not in the host's store, export it as PEM and set its path in `medium_ca_file`.

### Medium step by step, status and activity log

The Medium view (v12.6.5) separates its two parts: the **original deploy tool** (it deploys and rolls back) and the **WRM preview** (read-only, see below). The original tool's card has a light (*off*, *working*, *running*, *needs attention*) and a numbered guide:

1. **Deploy-tool ZIP** (administrator, once): imports the approved tool and starts step 2 by itself.
2. **Prepare the runtime** (administrator): Docker (installed by WRM after the administrator's one-time consent, started if needed), the Docker image and Python environment, then the tool's Python packages, or Python, a virtual environment and packages on the WRM host. The first time this downloads an image and packages and can take several minutes; **Prepare the runtime / try again** repeats it.
3. **Your deploy-config ZIP**: WRM checks it at once with its own reader (the same rules as the original tool) and names the file and key of an error. If the runtime is not ready yet or another Medium operation runs, the ZIP is kept and checked by the original tool and applied at your next start; the WRM preview already uses it. A deploy-config the original tool rejects is discarded and the previous one stays in use; when the check cannot run (for example Docker is not running), the ZIP stays kept.
4. **Your SSH profile ZIP** (optional): kept the same way.
5. **Start / Stop**: start runs the original tool's config check, copies your files into the runtime, starts the process and waits until it answers; only then is it shown as running and its tabs open below. Stop ends only your process, also while it is starting.

A step that cannot run yet says why (for example *the runtime is still being prepared*) and its button is disabled with the reason as tooltip. While an operation runs, everyone sees what it is, who started it and since when; its starter sees every step with its start time and duration and can show the live output. When an operation fails, the failed step, the error and **what to do** stay shown, with the last lines of the output. Another click while an operation runs is refused with `409` naming the operation, the user and the start time. *Or all at once* keeps the combined *Prepare and run* form.

The **activity log** below lists every Medium action (imports, preparation, starts, stops, settings, WRM preview runs, requests to the original tool) with time, user, result and duration; click a row for its steps and output. Users see their own rows and the shared runtime's; administrators see all. Command output (Docker, pip, the config check, the original tool's process) is kept for each operation (at most 200 lines), with secrets redacted and encrypted in the database (`medium_activity`, the newest 1000 rows per user and for the shared runtime). `medium_log_output` decides who sees output: `all` (default: the user who ran it, administrators, and everyone for the shared runtime), `admins` or `none`. WRM's console logs each operation and step as `Medium (user): …`.

### Medium native engine

The Medium view has a **WRM engine** panel. It compares a branch or tag with every server of an environment and, for a project on the native engine, deploys and rolls it back without Docker and without the original tool ([roadmap](ROADMAP.md)). With the native engine as the default (`medium_default_engine=native`) the panel comes first and opens, and the original tool's card is folded away as optional; a deploy-config ZIP can also be imported from the WRM engine panel.

- **Configuration:** the imported `deploy-config` (`servers.yaml`, `environments.yaml`, `projects/*.yaml`) is read as it is by a Go reader with the original tool's validation: unknown and duplicate keys, YAML 1.1 booleans and `<<` merges, names, hosts, ports, `allowed_refs`, per-environment `servers` lists, ignore rules, absolute paths, `source_dir`, state locations (`.deploy/<app>` or `state_dir`) and destinations that overlap or nest. Errors name the file and the key.
- **Environment server limits:** an environment's `servers: [name, ...]` restricts every project target in that environment to those servers, as in the original tool. A listed server must exist in `servers.yaml`; unknown, duplicate and empty lists are refused. Leave the key out to allow every server.
- **Servers and WRM connections:** each server is matched to a saved SSH connection with the same address and user (*Automatic*), or linked to a chosen one. The link is stored per user and audited (`git.medium_server_linked`). Keys stay in WRM.
- **Source:** the repository of a project is served by the WRM Git source (GitLab, GitHub or Gitea API) on the same host. Branches, tags, full refs and commits are resolved there; files below `source_dir` are fetched through the blob cache, symlinks keep their targets, and a path that is absolute or has `.` or `..` components stops the comparison.
- **Plan:** for each destination of the environment, WRM walks the application directory over SFTP and computes what `rsync -rlc --delete` with the project's exclusions would do: new, changed (SHA-256, computed on the server with `sha256sum` where available), deleted, type changes, kept directories that hold excluded items and the "cannot make way" error. It shows the server state in the original format (`current.json`, recent `history.jsonl` entries, the lock owner), `allowed_refs` and confirmation rules, and the `post_deploy` command. Tick changes and *Compare again without the ticked ones* to leave them out of this deployment.
- **Compare with the original tool:** with Medium running, the original comparison (fetch, export, build, `rsync --dry-run`) runs through the authenticated bridge and the native plan is computed on the same prepared source. Each server shows *Same result as the original tool* or the changes found by only one engine. The WRM Git source is also checked against the original export.
- **Engine per project:** *Original tool (Docker)* or *WRM native*. A project nobody chose an engine for uses `medium_default_engine` (`native` for users allowed by `medium_native`, else the original tool). Switching needs the `medium_native` policy (default `admins`) and is audited (`git.medium_engine_set`). The server state is the same for both, so a project can be switched back at any time.
- **Dry run and Deploy:** a deployment uses the comparison you reviewed (its revision) and is refused when the comparison changed, the environment's `allowed_refs` does not allow the ref, or the environment has `confirm` and its name was not typed. Per destination, in config order: lock (`lock/` and `lock/owner.json`), compare again under the lock (a different result writes nothing), backup manifests, removals into `backups/<deploy_id>/`, new directories, uploads to temporary files next to their targets with a SHA-256 check, replacement (the old version goes to the backup first), `post_deploy` in the application directory through the account's login shell (`medium_command_timeout`), a final check, `current.json`, `history.jsonl`, pruning of old backups (the current deployment and the last five applied ones are kept) and the lock released. A dry run stops after the second comparison. A destination that fails after it started changing gets a partial history entry, can be rolled back, and the next destinations are skipped.
- **History and rollback:** per server the current version, the lock (owner and age), the recent history with counts, and which deployment a rollback undoes and what it restores (chosen from the history as the original tool chooses). *Dry rollback* shows what would happen; *Roll back* restores files, links and directories with their modes from the backup and records a `ROLLBACK` entry (`rollback-<id>/` keeps what it replaced). A lock older than the longest a deployment may take (from `medium_command_timeout` and `medium_transfer_timeout`, at least 30 minutes) can be released with *Release lock* if its owner did not change.
- **Progress:** one deployment or rollback per user runs in the background with a live log (commands and their output), the result per destination and *Cancel*, which stops before the next destination starts changing. Each run is a row in the Medium activity log. Stopping a user or a lockdown cancels their runs.
- Deploying, rolling back and releasing a lock need the `medium_native` policy and, with step-up on, a fresh confirmation; all are audited (`git.medium_native_deploy`, `git.medium_native_rollback`, `git.medium_native_lock_released`, `git.medium_native_cancelled`, and `git.medium_native_<kind>_done` with the outcome per destination). Comparisons and dry runs need `git_medium` like the rest of Medium and are audited (`git.medium_native_preview`).
- **File details:** every changed file of a plan has **Details**: the server's version against the branch, line by line, as in the Git workspace (up to 512 KiB per side; new and deleted files, links, directories and binary files are explained instead). Opening one is audited (`git.medium_native_diff`).
- **Line endings:** a changed file whose content without `\r` equals the branch's is marked *line endings only* (`eol_only`) and counted separately; the hash is computed on the server with `tr -d '\r' | sha256sum` (over SFTP when `sha256sum` is missing), for files up to 8 MiB and at most 2000 files per server. The plan does not change: the deployment sends those files, and the history entry's `counts.eol` records how many there were.
- **deploy-config editor:** *Edit deploy-config* opens `servers.yaml`, `environments.yaml` and `projects/*.yaml` as text. **Check** loads the whole configuration with your edit; **Save** writes the file only when it loads and the file did not change since you opened it (otherwise *reload*). Comments, layout and CRLF line endings are kept; project files can be created and deleted (never `servers.yaml` or `environments.yaml`). Needs the `medium_native` policy; audited (`git.medium_config_edit`) and listed in the Medium activity log.
- **Build locations:** for a project with a build step, choose where the build runs before previewing: on the WRM host, on a chosen Linux build server over SSH, or not at all (`none`). The choice is stored per user and project and audited (`git.medium_build_set`). The whole commit is prepared, the configured `build` command runs in the repository root, and `source_dir` is read from the result. A build server gets a temporary directory with only the source (no Git metadata or SSH keys) and the directory is removed afterwards; the build runs through the account's login shell. The plan shows the build location, its platform and architecture, and the last build output; a destination whose platform or architecture differs is refused with the reason. WRM-host builds need a POSIX host with the toolchain, so on Windows use a Linux build server or `none`.
- **Build user:** with `medium_build_user` set (for example `wrm-build`), WRM-host builds run as that account: WRM must run as root on Linux or another Unix system; root, accounts in the root group and accounts that can read the WRM database, its key or `<DB>.medium` are refused with the reason. The build directory is handed to the account, the build gets only `PATH`, `HOME`, `USER`, `LOGNAME`, `LANG` and `TMPDIR` (no WRM variables), and the plan shows *as wrm-build*. Afterwards WRM takes the directory back before reading the result and refuses hard links to files the account does not own.
- **deploy-config from Git:** with `DEPLOY_CONFIG_REPO` in the Medium settings and a WRM Git source serving it, the editor's **Git** section shows the repository, branch, pulled commit, whether Git has newer commits, and the local changes. *Pull from Git and use it* makes WRM's copy (`<DB>.medium/users/<id>/native-config`) the configuration in use; the imported deploy-config stays untouched. *Pull* replaces the copy with the branch head only when there are no local changes (or *Discard changes and pull*), and never applies a head whose configuration does not load. *Commit and push* validates the whole configuration, refuses when a changed file moved on Git since the pull, writes one commit through the source's write token (GitLab commits API, GitHub Git data API with a non-forced ref update, Gitea contents API; author from `DEPLOY_USER_EMAIL` when set) and pulls the result back. *Stop using the Git copy* deletes it and goes back to the imported deploy-config. Needs `medium_native`; audited (`git.medium_config_pull`, `git.medium_config_push`, `git.medium_config_git_stop`) and listed in the Medium activity log.

### Git workspace: every file and partial checks

**Every file of an installation.** The details of an installation (click it in the overview, or *Details*) read the server folder once and list **every file** — not only the ones the catalog tracks — together with every file of the repository at the target ref below the service's subdirectory:

- **Columns:** path, size, modification time, state and **why**: *tracked*, *excluded by the catalog* (with the pattern, e.g. `setup.py` or `tests`), *not in the include list*, *protected* (with the pattern), *not in Git* (only on the server), *written by the deploy tool* (`VERSION.md`, `updates.jsonl`), *compiled file*, *in Git, not tracked*.
- **States:** tracked files as in checks (*ok*, *old*, *modified*, *missing*); files the catalog does not track are *outside the check* and are not compared (since v13.14.0); *extra* (only on the server), *missing* (only in Git), *symlink* (the target is shown, a symlink is **never followed**, and one that points out of the installation is marked), *unreadable* (permission denied, also for directories).
- The catalog's `ignore_dirs`, backup-looking paths and `.git`, `.deploy-bak`, `node_modules`, `__pycache__`, virtual environments are left out; at most 10 000 entries are listed (a notice says when the folder has more).
- **Filters:** *In the check* (the default), *Differing*, *Outside the check*, *All*, a **folder** filter (a tree of the folders with counts; a folder includes everything below it) and a search. The *Why* column says whether a file is in Git, on the server or both.
- **Log files** (`*.log`, `*.log.*`, files below a `log` or `logs` folder, except source and configuration files) are never compared, never offered for a diff and never deployed, unless the catalog's include list names them.
- **Open** shows any file (view only, up to about 1.9 MB; binary files show their size and hash), from the server or from Git. **Diff** compares server ↔ Git for **any file in the repository** — also files the catalog excludes, labelled *informational, not updated* — unified or side by side; line endings are ignored like in the hashing. Binary and large files are compared by hash.
- Files the catalog does not track are **informational**: they never change the state of an installation (*review* / *needs update* / *up to date* still come from the tracked files only).
- **Commits behind:** the details use the detected commit; with the `VERSION.md` checkbox on they use the commit (or version) in `VERSION.md` instead. They show how many commits the target is ahead and list them (GitLab / GitHub compare API). When the code is mixed or modified, no single commit is known, so the list is not shown.
- Server files are read over SSH with quoted paths and POSIX tools only, never cached by WRM. Opening a file is audited (`git.file_viewed`). The other repository files come from the cached tree of the target commit; their contents are fetched and cached like target files (at most 200 unknown files per listing).

**Names and folders in the overview.** Servers are named by the hostname read during checks or by the connection's name, never by a bare IP address (a connection named by its IP still shows it: rename it, or use *Name from hostname*). Besides the connection-folder filter (which includes subfolders), the overview, *Functionality*, *Environments*, *What is where* and Simple mode have a **server folder** filter: a tree of the installation folders with counts, like the log viewer's folder filter.

**Partial and incremental checks.** In the overview and on the *Installations* tab:

- **Filters** by service, server, state (including *not checked*), environment, folder, tag and a search that also matches the host.
- **Selection:** a checkbox on every installation, every server row and every service column (and *all visible*). *Check selected*, *Check visible* (what the filters show), *↻ Check this cell*, *Check only stale* (never checked, or older than 1 hour / 24 hours / 7 days). *Check now* in the header still checks everything (discovery on the servers of *Settings* and fresh targets).
- **Streaming:** a check runs as a job in the background; each installation is saved as soon as it is compared and shows a spinner while it is queued or being checked. *Cancel* skips what is queued and drops the scans that are running.
- **Limits:** `git_check_per_server` (2) installations of one server and `git_check_parallel` (8) installations overall at the same time.
- Installations that are not part of a check keep their result and show *checked X ago*. A failed check keeps the files and counts of the **last good result** and shows it next to the error.

### Git workspace: update, upgrade, rollback & restarts

Runs change servers only when you start or schedule them. Select installations on the *Installations* tab (or open one) and choose an action; a run works on the installations **one after another** and **stops at the first failure** (*rolling*). Everything goes over SSH with POSIX tools (`sh`, `cp`, `mv`, `find`, `awk`, `tr`, `sha256sum`).

- **Update** — the newest version of the ref the installation follows. **Upgrade…** — another branch or tag (`main`, `tag:v2`, `tag:latest`), with a warning and a summary per installation: files to change, new files, files changed locally, and files that the new version no longer has (they stay on the server). *Follow this ref from now on* sets the services' ref override, so later comparisons use it. **Rollback…** — restore a backup from `.deploy-bak/`. **Stamp** — write `VERSION.md` where an installation is up to date and has none (*Overwrite an existing VERSION.md* forces it). The dialog lists the files that differ; *Also stamp installations that are not up to date* writes it anyway with each file's actual version (older files their matching version, hand-changed files `(local)`) and a `Napomena` line saying the installation is partial. Dialogs opened from another one have **← Back** (Alt+←).
- **Choosing files:** *old* and *missing* files are ticked. A file **changed by hand** is written only when you tick it, with a warning (its content stays in the backup). **Protected** files are never written. *Show all files* lists the current ones too.
- **Confirmation:** a summary, and for **production** installations — tag `env:prod` (or `prod`, `production`, `prd`, `live`) or an environment directory with that name — you type the server name. The server checks it again.
- **Steps per installation**, shown live in the run's details:
  1. **Compare again** (a fresh scan): a file changed since the last check stops the run unless it was ticked as changed.
  2. **Dangling Python imports:** when `.py` files are written, a `from mod import name` in another file of the installation whose `name` the new `mod.py` no longer defines (a scan of its top-level `def`, `class`, assignments and imports; relative imports and packages are resolved) stops the run before anything is written. *Skip the check for dangling Python imports* turns it off for one run.
  3. **Backup** of the files that are replaced (and `VERSION.md`) to `<install>/.deploy-bak/<id>-<YYYYMMDD-HHMMSS>/<path>`; `<id>` is the bundle ID for targets from a bundle, else the run ID `wrm-<YYYYMMDD-HHMM>`.
  4. **Atomic writes:** each file is written to a temporary file in the same directory (copied from the old file first, so **mode and owner stay**) and verified by hash, then all are renamed into place. **Line endings stay**: a CRLF file gets CRLF; a new file gets what most files of the installation use; new files get mode `644` (`755` with a `#!` line) and the installation's owner.
  5. **Checks:** `python3 -m py_compile` for `.py` (when `python3` exists; its `.pyc` files go to a temporary directory), `node --check` for `.js`, `sh -n` (`bash -n` for bash scripts) for `.sh`, and an optional **custom command** per service (run in the installation directory, exit code 0 = OK; remembered for the next run).
  6. **Automatic rollback** on any failure: the replaced files come back from the backup (atomically), new files and new directories are removed and the backup is dropped. A run that fails before writing changes nothing.
  7. **`VERSION.md`** and a line in **`updates.jsonl`**, then the installation is compared again.
  8. Optionally a **restart** (below).
- **Compatibility with file-based deploy tools** — WRM writes their formats, so both can be used side by side:
  - `VERSION.md` in the installation root, with the installation's line endings (never part of the comparison):

    ```
    # demo-api - v1.10

    - **Servis**:      demo-api
    - **Verzija**:     v1.10
    - **Grana/ref**:   main (tag)
    - **Commit**:      0123456789 (2026-09-10)
    - **Azurirano**:   2026-10-09 21:05:07
    - **Host**:        app-01
    - **Korisnik**:    deploy
    - **Bundle**:      wrm-20261009-2105

    ## Services (3)

    | Datoteka    | Verzija    | Datum      |
    | ----------- | ---------- | ---------- |
    | config.ini  | (per-host) | -          |
    | lib/util.py | v1.10      | 2026-09-10 |
    | run.py      | v1.10      | 2026-09-10 |
    ```

    Rows are sorted by path; columns are padded to the longest value. *Verzija* is the tag of the file's newest history entry, else its commit; older files that were not updated show the version they match, files changed by hand `(local)`.
  - `updates.jsonl`: one line per installation and run (sorted keys) in the first writable of `/var/log/deploytool/updates.jsonl`, `<install>/.deploy-bak/updates.jsonl` and `/tmp/deploytool-updates.jsonl`:

    ```
    {"app": "demo-api", "backup": "/opt/demo-api/.deploy-bak/wrm-20261009-2105-20261009-210507", "branch": "main", "bundle": "wrm-20261009-2105", "env": "prod", "files": ["lib/util.py", "run.py"], "from_version": "v1.9", "host": "app-01", "install": "/opt/demo-api", "services": ["systemctl demo-api"], "time": "2026-10-09T21:05:07", "to_version": "0123456789 (2026-09-10)", "user": "deploy"}
    ```
  - **Rollback** lists the backups of the installation with their time, `VERSION.md` and log line. The files of a backup come from WRM's run history or the `updates.jsonl` line; a file that did not exist before is deleted on restore. For a backup without such a record only the files in it are restored. The current files are backed up first, so a rollback can be rolled back.
- **Service restart (opt-in, off by default).** The units are the systemd / supervisor units found by the check (their unit file mentions the installation directory). Per run: **do not restart** (the installation shows *↻ restart pending*), **right after a successful update** (in the rolling order), **at a chosen time** or **after a delay**. A warning names the units and servers. Every restart is followed by a **health check** (`systemctl is-active` must say `active`, `supervisorctl status` must show `RUNNING`); an unhealthy unit stops the run. `sudo -n` is used when the SSH user is not root and may use it without a password. *Restart…* also restarts installations without an update.
- **Scheduling (maintenance windows):** *When → At a chosen time*. Scheduled runs and restarts are stored in the database, **survive a WRM restart** and can be **cancelled** (*Runs*). A restart that waits for an update is skipped if the update failed. A run is **skipped when WRM was not running at its time** for longer than `git_schedule_grace_minutes` (30). Notifications: 15 minutes before (*a scheduled run starts soon*), at the start and at the end.
- **Runs tab:** history (the last 500 runs per user) and scheduled runs with installations, versions, states and every step; *Stop after the current server* for a run that works.
- **Policies:** `git_update` (update and stamp), `git_upgrade`, `git_rollback`, `git_restart` — `admins` by default, `all` or `off`. Runs are audited per installation (`git.update`, `git.upgrade`, `git.rollback`, `git.restart`, `git.stamp` with the result, files, backup and versions), and so are scheduling, cancelling and skipped runs.

### Git workspace: tab explanations

Every tab of the **Advanced** Git workspace (Overview, Services, Installations, Environments, Runs, Activity, .gitignore, Settings) shows a short plain-language description under the header and the same text as a tooltip on the tab button. The Settings window and the Admin panel show one under their tab bar too (Settings: General, Terminal, Security, Notifications, Voice & audio, Session history, AI connections, Data; Admin: Overview … File transfers). The texts are in English and Croatian (`gtd_*`, `stin_*`, `adin_*` keys). Behaviour and layout are otherwise unchanged; Simple mode keeps its own intro.

### Git workspace: Simple mode

The header of the Git workspace has a **Simple | Advanced** switch. The choice is remembered per user in this browser (`localStorage`); **Advanced** is the default and is exactly the workspace described in the other Git sections.

**Simple** is a single view for people who only want to know what is deployed and to update it. For every environment of every service it shows one card. **No manual setup is needed:** once a Git source is connected and the checker has found your services on the servers, the environments appear automatically (see *Automatic environments* below); environments defined in Advanced → *Environments* are shown as before and win over detected ones:

- **On the server**: the detected version and commit, the optional `VERSION.md` (with the checkbox next to *Check now*) and who deployed it, per destination server (or *Not installed here yet*).
- **Latest on the branch**: version, branch, commit and date of the service target.
- A status: **Up to date**, **Update available**, **Not installed**, **Mixed / modified**, **Cannot compare** or **Problem**.
- Combined filters for service, hostname, environment and status, plus searches for branch, version or commit; each card shows when its server state was compared.
- Big actions: **Install** (not installed yet), **Update** (a newer version exists), **Deploy again** (already current) and **Roll back to the previous version**.

Install, update and deploy open a plain-language review: the version change, how many files are added or changed per server, and an expandable file list with each file's status on each destination. You can choose the successfully compared servers and exclude files before confirmation. Empty selections are refused. Simple mode keeps files removed from the repository; Advanced mode lets you delete them after backup. Optional *post_deploy* and *restart* checkboxes (when allowed), an optional *different version* (tag or branch), and, for protected environments, typing the environment name are also available. Rollback lets you choose which servers to restore and shows the version now on each server and the one it returns to.

**Automatic environments.** Simple mode asks the overview for detected environments (`?auto=1`). For every service of the catalog, the installations the checker found (`git_installs`) are grouped by environment label (the tag or folder-name hint, `default` if none) into virtual environments; a production label asks for the typed environment name, as before. They are only *derived*: nothing is stored and nothing is deployed until you start an install, update or rollback, which runs through the same plan, locks, backups, permission checks and audit. A stored environment with the same name, or one that already lists the same server and folder, takes priority; conflicting or invalid derivations are left out. A notice shows how many were detected and, for users allowed `git_update`, **Save detected environments** stores them in the catalog (`POST /api/git/env/adopt`, audited as `git.env_adopted`; existing definitions are untouched). An empty Simple mode offers **Find services on my servers** (the normal check). The Advanced tabs show stored environments only.

**Servers are shown by hostname.** Wherever the Git workspace lists a server (Simple cards and dialogs, Environments, Activity, Installations, Runs), the label is the server's real hostname instead of the connection name you chose. WRM reads `hostname` over the existing SSH login during checks and deploy plans, caches it (`git_hostnames`, 6 h) and falls back to the connection's host address until it is known. Filters and requests still use the connection name.

Simple mode adds one endpoint (`POST /api/git/env/adopt`) and otherwise uses the environment overview, cell, plan, history and run endpoints of the Advanced deploy, so permissions (`deploy` update / rollback / restart), locks, backups, remembered exclusions, the audit trail and run history are the same. Settings, sources, catalog, activity, `.gitignore`, CI, schedules and other advanced controls stay in Advanced.

### Git workspace: environments, deploy plans & safe deploys

Environments are **optional**. A service without environments keeps using *Update*, *Upgrade* and *Rollback* per installation as before. An environment is a named, **ordered** list of destinations of one service, with its own rules; a deploy is reviewed as a plan, applied once, locked on the server and recorded there.

**Defining environments** (*Environments* tab → ✎, or the catalog JSON):

```json
"environments": [{
  "name": "prod",
  "destinations": [{"server": "app-01", "path": "/opt/demo-api"}, {"server": "app-02", "path": "/opt/demo-api"}],
  "confirm": true,
  "allowed_refs": ["refs/tags/v*"],
  "ignore": ["/data", "*.local"],
  "post_deploy": "./bin/migrate --yes",
  "keep_backups": 5,
  "state_dir": ""
}]
```

- `server` is the name of an SSH connection (jump hosts, vault credentials and proxies work as everywhere); paths are absolute service directories, not system directories.
- `confirm` asks to type the environment name before a deploy or rollback (always for an environment named `prod`, `production`, `prd` or `live`). `allowed_refs` limits what may be deployed (`refs/heads/main`, `refs/tags/v*`; a pattern without `refs/` matches the short branch or tag name). `ignore` adds patterns to the catalog's excludes for this environment. `post_deploy` runs in the app directory after the files — **only when it is ticked** for a deploy or rollback. `keep_backups` (1–50, default 5) deploy backups are kept per destination.
- `state_dir` (optional) moves the server-side state to `<state_dir>/<service>/<path with / as _>`; by default it is `<path>/.deploy-bak`.
- **Validation:** two destinations on the same server where one app path lies inside the other are rejected unless the outer service's excludes (or environment ignores) cover the inner path; a `state_dir` inside any app path is rejected.

**The overview** (*Environments* tab) is a service × environment table. Every cell is read **lazily, one request per cell**: per destination the deployed version and commit, who deployed it and when (from `state.json`, else `VERSION.md`), how many commits it is behind the service's target, and a lock if there is one. A cell warns when **the servers of one environment differ**.

**Deploy:** *▶ Deploy* compares every destination with the target (or another ref: `tag:v2`, a branch) and shows the **plan** per server:

- **States:** *new*, *changed* (with *changed by hand* when it matches no version of the repository, or how many versions behind), **same (CRLF/LF)** — only the line endings differ: counted separately and **sent anyway**, so the deploy normalises the line endings to the repository's —, *removed from Git* (in the repository at the deployed commit but not any more), *only on the server*, *protected*, *ignored* (with the pattern), *same* (*Show all files*).
- **Excluding files:** untick a file to leave it out of **this** run; the choice is **remembered** for the next compare with who and when (*Clear remembered exclusions*). **⊘ service** / **⊘ environment** exclude a file **permanently**: an anchored pattern with glob characters escaped (`/lib/a[[]1].py`) is written to the service's excludes or to the environment's ignore list, unless an existing pattern already covers the file; the plan is compared again.
- **Options:** delete files removed from the repository (opt-in), run `post_deploy` (opt-in, the command is shown), restart the units after each destination (opt-in), the custom check command.
- **Plan errors and warnings:** excluding (or not deleting) something the branch turns from a directory into a file or back is an error; replacing a symlink on the server, files changed by hand, a missing path (**the first deploy creates the path and its parents**) and a lock are warnings.
- Every destination plan has a **fingerprint** of what was compared (the server files with their hashes, the current deploy, the target commit). A plan can be applied **once**; a new deploy needs a new compare.
- **Dry run** uses exactly the options of the real deploy, takes **no lock**, writes **nothing** and needs no typed confirmation; the run log lists what would be added, changed, deleted and run. *Back to the plan* returns to the same plan.

**Per destination, in order** (the run's log refreshes about every second):

1. **Lock:** `mkdir <state>/.lock` (atomic) with an owner file — deploy id, WRM user, WRM host and the **server's** time. A lock held by someone else stops the deploy; a deploy releases **only its own** lock (the owner file must name its deploy id).
2. **The plan again under the lock:** another fingerprint stops the deploy before anything is written (*compare again*).
3. **Transfer:** the files (and `VERSION.md`) as **one tar stream** into `<path>/.wrm-incoming-<id>` (timeout at least 1 hour), verified by hash. An **interrupted transfer** is recorded as **partial** with what had really arrived (removed again); nothing in the installation changes and no new current state is written.
4. **Backup** of what is replaced or deleted to `<state>/<id>/` (the existing backup layout), then deletes and renames (mode and owner of replaced files stay; directories left empty by deletes are removed and remembered with their mode and owner), checks (`py_compile`, `node --check`, `sh -n`, the custom command) and an **automatic restore** on failure.
5. **State:** `deploys/<id>.json` (the manifest: added, changed and deleted files, removed directories, the previous state), `state.json` (`current` and `previous`), the `updates.jsonl` line, `post_deploy` when chosen, then a **DEPLOY** entry in `history.jsonl` with the previous state and the lists of new, changed and deleted files (at most 500 names per kind, plus the real counts). The oldest backups beyond `keep_backups` are removed.
6. A failure stops the run: the remaining destinations are not started. A **failed `post_deploy`** is recorded with a note — the files are already on the server and the new state is current.

**History** (*🕘 History*) merges `history.jsonl` of every destination (one row per destination) with the file lists in an expandable column. **Rollback** works per deploy id on the chosen destinations (default: every destination where it is the latest), **in reverse destination order**:

- Under the lock it checks that this deploy is **still the latest** on the destination and that its backup exists.
- Files the deploy **added** are **moved** to `<state>/backups/rollback-<id>/` (never deleted); the **current versions** of changed files are kept there too, then the backed-up versions come back atomically; deleted files come back; **removed directories are re-created with their mode and owner**.
- **Nothing is done through a symlink** on the server: such items are skipped and the rollback is marked **incomplete** (the state does not change; fix the server and roll back again).
- Then `post_deploy` (when ticked), the stored previous state becomes current again, and a **ROLLBACK** entry with `rolled_back=<id>` is written. The last 5 rollback backups are kept. A dry run shows what a rollback would do.

**Stale locks:** a lock older than max(30 minutes, the transfer timeout), measured by the server's clock, is *stale*. The plan offers *Remove the stale lock* after a confirmation; one server command checks again that it is the same lock (owner id) and still stale before it removes it (audited as `git.lock_removed`).

**🩺 Doctor** checks every destination: SSH login, a clean non-interactive shell (no banner or rc output before the commands), the SSH user (a warning for `root`), the app directory (writable; a missing one is OK when its nearest existing parent is writable), the state directory, unreadable subdirectories, signs of another deploy method (`.git`, `.svn`, `.hg`, `releases` / `current`, a symlinked app path), a path that looks like a public web root, `sha256sum`, `rsync` and `tar` with their versions, and the lock.

Environment deploys and rollbacks are ordinary runs: they appear on the *Runs* tab, can be stopped after the current destination, send the run notifications (not for dry runs) and are audited per destination (`git.update` / `git.upgrade` / `git.rollback` with `env`, `deploy_id`, `dry_run`, `partial`, `post_deploy`).

### Git workspace: activity, deploy history & what is where

The *Activity* tab answers *who changed what on which branch, who deployed what where, and what runs where now*. It has three views, each with its own filters and a **⬇ CSV** export of exactly what the filters show (UTF-8 with a byte order mark for spreadsheets; values that a spreadsheet would run as a formula are prefixed with `'`).

**Git activity** (per service, from the Git source of its target):

- **GitLab:** project events with `action=pushed` (every branch and tag: who pushed, how many commits, from → to) and the commits of the listed branches. **GitHub:** the commits of the listed branches and, where the token allows it, the repository activity (pushes, force pushes, branch creation and deletion, merges). **Gitea:** the commits of the listed branches.
- **Branches:** without a chosen branch, the commits of the service's branch, the target's branch, the default branch, the catalog's `fallback_branches` and the branches of the environments' `allowed_refs` are listed (at most 6). Pick any branch of the project in the filter. A commit on several listed branches appears once, with its branches.
- **Filters:** branch, author (part of the name, user name or e-mail) and dates (default: the last 30 days, at most 366). **Per author** (commits, pushes, branches, last activity) and **per branch** (commits, pushes, authors, last activity, a link to the branch's commits) summaries follow the filters; click a row to filter by it. Each commit links to its commit page, each push to the compare page of its range.
- **API limits:** listings follow the provider's next links up to 10 pages each and stop early when the provider reports that 5 or fewer calls are left; a cut listing says so (with the time calls are possible again when the provider tells it). A rate-limited request (HTTP 429, or 403 with no calls left) becomes a note instead of an error. Results are cached per user, service, branch and dates for 3 minutes (*↻ Refresh* reads again); the author filter works on the cached listing.

**Deploy history:** the `history.jsonl` of every destination of every environment (deploys and rollbacks, one row per destination, read over SSH like the environment cells, **one request per service and environment, three at a time**) merged with the WRM runs (update, upgrade, rollback, install, transfer) of installations without environments — environment runs are left out there because their destinations keep the history. Filters: service, environment, server, action (deploy / rollback / update / upgrade / install / transfer), who, branch, version or commit, dates. Every row shows the result, the version before, the commit (linked) and the per-file lists (new, changed, deleted, excluded, restored, moved aside …) in an expandable column.

**What is where:** per service, environment and server the branch, commit (linked) and version that runs there, who deployed it and when (from `state.json`, else `VERSION.md`), and **how many commits it is behind the head of its branch**, linked to the compare page of those commits. A tag deploy or a `VERSION.md` without a branch counts against the service's branch. Servers of one environment whose commit and version differ from the others are **highlighted** (*differs*). Installations without environments are listed from their last check (no SSH). Cells load lazily like the *Environments* tab; head and compare answers are cached for 2 minutes.

Policies: the activity needs only the Git workspace (`git_enabled`); the deploy history from the servers and the matrix need `git_checks` (WRM's own records are shown without it). Tokens never reach the browser.

### Git workspace: new servers (install & transfer)

Two wizards set up a service where it does not run yet. Both are runs (they appear on the *Runs* tab with every step), start right away and work over SSH with POSIX tools plus `tar`.

- **Install…** (*Installations* tab):
  1. **Target server:** any SSH connection.
  2. **Services**, each at a version: its target (as in the catalog), another branch or tag (`main`, `tag:v2`; needs a GitLab / GitHub source) or an imported **offline bundle** that contains it.
  3. **Target directory** per service, suggested from the server roots (`/opt/<service>`), and an environment label. System directories (`/etc`, `/usr`, `/opt` itself, …) are refused.
  4. **Pre-checks** on the target: free disk space (the files plus 10 %), a writable directory (or the nearest existing parent), `tar`, the `python3` / `node` versions when the service has `.py` / `.js` files (a missing tool is a warning: its checks are skipped), and an **existing installation** (files in the directory) — refused unless you tick *Replace it*; the replaced files and `VERSION.md` are then backed up to `.deploy-bak/` first.
  5. **Per-host files.** WRM writes the files of the ref **without the protected ones** and offers every protected file to fill:
     - **From a template** in the repository: `<name>.example`, `.sample`, `.template`, `.tmpl`, `.tpl`, `.dist`, `.default` (also `settings.example.py` → `settings.py`, `.env.example` → `.env`), or the protected file itself when the repository has it. Its **placeholders** (`{{ name }}`, `${NAME}` / `${NAME:-default}`, `__NAME__`, `@NAME@`, `<NAME>`) and **`key = value` / `key: value` lines** (INI keys as `section.key`, quotes kept) become form fields with the template's defaults; names that look secret are password fields. A field can take a **credential of the vault** (its password or user name; only credentials whose host list allows the target server). Placeholders left empty stay in the file (the run notes them).
     - **Copied** from another installation of the same service (any server), or **typed** by hand, or **left out**.
     - Filled files get mode `640`. Typed values and contents are **never stored** with the run; vault values are read on the server side only.
  6. Optionally a **custom check** command (as for updates).
  7. Optionally a **service unit** (opt-in, with a warning): a **systemd** unit (`/etc/systemd/system/<name>.service`: `User=`, `WorkingDirectory=`, `ExecStart=`, `Restart=on-failure`, `daemon-reload` and optionally `enable`) or a **supervisor** program (`/etc/supervisor/conf.d/<name>.conf` or `/etc/supervisord.d/<name>.ini`, then `supervisorctl reread`). It is written as root or with `sudo -n`, never over an existing file. WRM does not test the command.
  8. Optionally **start** it after the install (policy `git_restart`): the units that mention the directory are restarted and health-checked, as after an update. Otherwise the installation shows *↻ restart pending*.
- **What a run does:** the contents (Git API or bundle, and the filled per-host files) travel as **one tar stream** into a staging directory inside the target (`.wrm-incoming-<run>`), are **verified by hash**, then moved into place by rename; the language checks and the custom check run; any failure **rolls back** (replaced files from the backup, new files and directories removed). Then `VERSION.md` (as an update writes it) and the `updates.jsonl` line, the installation is **registered** (as added by hand, so discovery keeps it) and compared, and the unit is created.
- **Transfer…** (an installation, *Installations* tab or its details): copies it to another server or directory **with its per-host configuration**, but **without** logs (`log/`, `logs/`, `*.log`, `*.log.N`, `updates.jsonl`), `.deploy-bak`, caches (`__pycache__`, `*.pyc`, `.pytest_cache`), the catalog's `ignore_dirs`, backup-looking paths (a backup word as a directory or after the first part of a file name: `data_BKP/`, `run.py.bak`, `config_old.ini`) and the usual prunes (`.git`, `node_modules`, `venv`, `.venv` — recreate them on the target). The wizard lists what stays behind and the source's units (they are not copied; create one in the wizard). The files stream from server A through WRM straight into server B (`tar -c` → `tar -x`, nothing is kept on WRM's disk), with the same pre-checks, overwrite confirmation, backup, verification, checks and rollback; `VERSION.md` gets the new host and user.
- **Confirmation:** a summary; production targets (tag `env:prod`, or a production environment label) and overwrites need the **server name typed**.
- **Policies:** `git_install` and `git_transfer` — `admins` by default, `all` or `off`. Audit `git.install` / `git.transfer` per installation (result, files, backup, versions, source, the new installation). Notifications as for other runs (*started*, *succeeded*, *failed*).

### Git workspace: .gitignore helper

The **.gitignore** tab of the Git workspace builds the `.gitignore` of a service's repository from what the checks found. Pick the service; WRM reads the head of its branch through the API (trees and files are cached like everything else) and shows:

- **Files only on servers** — the files in state *extra* from the latest checks of every installation of the service, with their **size** and the **servers** they were seen on, and a suggested pattern: a known directory (`logs/`, `tmp/`, `cache/`, `venv/`, …), a known extension (`*.log`, `*.pyc`, `*.bak`, `*.sqlite`, …) or the file itself (`/local.txt`).
- **Standard patterns** per stack — Python, Node, Go, Java, Docker, IDE files and **Logs** (`log/`, `logs/`, `*.log`, `*.log.*`; pre-selected when the repository holds log files, which also get a warning). Stacks found in the repository tree (`requirements.txt`, `*.py`, `package.json`, `go.mod`, `pom.xml`, `Dockerfile`, …) are pre-selected.
- The service's **protected globs** (per-host files), pre-selected, and a box for more patterns.
- **Warnings** about committed files that look like secrets (`.env`, `*.pem`, `*.key`, `*credentials*`, `*secret*`, `id_rsa*`, …) or match the protected globs (per-host configuration such as `config.ini`), with the advice to commit a **template** instead (`config.example.ini`, `.env.example` — the install wizard fills those), add the file to `.gitignore` and remove it with `git rm --cached`.
- The service's **exclude / protected lists** of the catalog, editable on the same screen.
- The **current `.gitignore`** (in the service's subdirectory, e.g. `linux/.gitignore`).

**Preview** shows the result as a diff against the current file: only patterns it does not have yet are appended, under a dated comment, with the file's line endings. The output is a **download** by default. **Create merge request…** (GitLab) / **Create pull request…** (GitHub) is a separate, explicit choice: it needs a **write token** on the Git source (optional, encrypted, used for nothing else — the read token stays read-only) and the policy `git_gitignore_mr` (`admins` by default). The confirmation names the repository, and its name must be typed. WRM then creates a branch `wrm/gitignore-<service>-<time>` from the head it read, commits the file and opens the request against the service's branch. Audit `git.gitignore_mr` (repository, branch, number of patterns, the link or the error). WRM never writes to a Git server on its own.

### Git workspace: CI integration (optional)

WRM does not need Jenkins or GitLab CI; each of these is opt-in.

- **Incoming webhook per Git source** (*Settings → Git sources → Webhook…*): turning it on shows the URL (`https://<wrm>/api/hooks/git/<id>`) and a secret **once** (stored encrypted; *New secret* replaces it). A delivery starts an **immediate check** of the affected services — their targets are refreshed and their installations compared — instead of waiting for the interval, with the usual notifications.
  - **GitLab** push, tag push and release events: the secret as *Secret token* (`X-Gitlab-Token`).
  - **GitHub** push and release events: the secret signs the body (`X-Hub-Signature-256`, HMAC-SHA256); `ping` is answered.
  - **Generic POST** (Jenkins, any CI): the header `X-WRM-Token: <secret>`, or a signature `X-WRM-Signature: sha256=<HMAC of "<timestamp>.<body>">` with `X-WRM-Timestamp` (unix seconds, at most 5 minutes off). The body may name `{"project": "group/name"}` or `{"services": ["name"]}`; without one, all services are checked.
  - The services are those whose catalog project is the event's project. Deliveries are **rate-limited** (30 per minute per hook and per client address), **audited** (`git.webhook`, `git.webhook_rejected` with the reason) and **replays are refused** where the provider sends a delivery ID (`X-GitHub-Delivery`, `X-Gitlab-Event-UUID`, `X-WRM-Delivery`, or the generic signature). Deliveries that arrive during a check are merged into the next one. The owner of the source needs the `git_checks` policy.
- **Bundles from CI artifacts** (*Settings → Bundles from CI artifacts*): a URL of a bundle `.tar.gz` — the GitLab job artifacts API (`…/api/v4/projects/<id>/jobs/artifacts/<ref>/raw/bundle.tar.gz?job=<job>` with `PRIVATE-TOKEN`), a Jenkins artifact URL (`…/lastSuccessfulBuild/artifact/bundle.tar.gz` with `Authorization: Basic user:token`, encoded by WRM) or any HTTPS URL — with an optional **auth header** (encrypted, never shown again), fetched **by hand** (*Fetch now*) or **every N minutes**. A changed file is imported as an offline bundle (the feed keeps only its newest one); an unchanged one is skipped. Audit `git.bundle_fetched` / `git.bundle_fetch_failed`.
- **Deploy method "CI pipeline" per service** (the service's *Edit* dialog, policy `git_update`): for services that are built and deployed elsewhere.
  - **Jenkins job:** the job URL, a user and API token (or the job's *trigger builds remotely* token without a user). *Update* / *Upgrade* calls `buildWithParameters` with `server` (the connection's host), `install_path` and `version` (the target version), then follows the queue item and the build until it ends (`SUCCESS` succeeds; `FAILURE`, `UNSTABLE`, `ABORTED` fail the run).
  - **GitLab pipeline:** the GitLab address (default: the Git source), the project (default: the service's project), the branch (default: the target's branch) and a **pipeline trigger token**; the variables are `server`, `install_path` and `version`. The status is read with the read token of the GitLab source on the same host.
  - The run goes through the **same plan, confirmation (production: the server name typed), policies and audit** (`git.update` / `git.upgrade` with `via: ci`, the link and the result) as a normal update. No files are written; the run history shows the CI status and a link. Afterwards WRM compares the installation again when it can log in. *Stop* stops waiting (the job keeps running in CI).

### AI assistant

Use the top **AI** button or **Settings → AI assistant** to inspect effective account permission, provider availability and setup blockers. SSH and SSH-backed SFTP windows have an AI button; if access is disabled it opens the diagnostic view. Enabling/disabling policy refreshes open browsers, including after event-bus reconnect. A successful provider test confirms connectivity, and the setup result separately shows whether the account can use the assistant. Terminal AI does not depend on MCP settings.

An AI assistant in a panel **next to the terminal** (also from the file manager). Click **AI** in the title bar of an SSH terminal or file manager window, choose the provider, model and mode, and ask in plain language: *why is nginx failing?*, *free some disk space*, *raise the worker count*. The assistant explains every step and works only through WRM:

**Position and size (desktop):** the panel can be **docked** left, right, top or bottom with the ◧ ⬒ ⬓ ◨ buttons, shown as a **floating window** with ❐, **expanded** over the whole terminal window with ⛶ (or a double-click on the title bar; press again to restore) and **resized** by dragging its inner edge (the corner when floating). Drag the title bar to move it; a docked panel then starts floating. The terminal shrinks beside a docked panel and is refitted. Dock, size and floating position are remembered per user in the browser (`localStorage` key `wrm_ai_win_<user id>`). On compact windows (up to 760 px wide, e.g. phones) the panel keeps its full-window layout and these buttons are hidden.

- **The model never gets a shell.** It can only call WRM's tools: `system_info`, `read_file` (size-limited), `list_directory`, `tail_log`, `find_logs`, `read_log`, `search_logs` (grep over every discovered log), `journal` (journalctl), `service_status`, `list_packages`, `processes`, `disk_usage`, `network_status`, `run_command`, `write_file` and `edit_file`. Every call goes through the **permission engine on the server**, which decides; WRM runs the result over its own SSH connection to that server (jump hosts, proxies and vault logins as for the terminal).
- **Three modes per session** (the panel shows the current one; WRM's side of the policy decides):

| Mode | What happens |
|---|---|
| **Read-only** | The read tools and commands the **read-only classifier** accepts (`systemctl status`, `journalctl -u …`, `df`, `ss -tulpn`, `dpkg -l`, `grep … /var/log/…`, pipes of such commands …). Anything that is not clearly read-only — unknown commands, redirections, `$( … )`, `sed -i`, `find -delete`, `systemctl restart` — is refused. File edits are not even offered to the model. |
| **Ask before every change** | Reads run directly. Every command that changes something and every file edit becomes a **pending approval** in the panel: the exact command (you can **edit** it before approving) or a **unified diff** (you can edit the new content). Approve or deny with an optional note; an approval that is not answered in time (`ai_approval_timeout_seconds`) is **denied**. A file is written only if it did not change since the diff was made. |
| **Automatic within limits** | Only after an explicit opt-in for this session: an **allow list** and a **deny list** of command patterns (`systemctl restart nginx`, `apt-get install -y *`) and file paths (`/etc/nginx/**`), a **time limit** and a **maximum number of actions** (both capped by policy). Matching changes run without asking; the deny list refuses; everything else, and everything after the limits, asks again. When the time is over the session falls back to *ask*. An allow list of `*` allows every command, also chains, variables and redirections (still not the blocked or denied ones). AI connections (MCP) can choose **no time or action limit** instead: automatic until the connection expires. |

- **Always blocked, in every mode, also after an approval:** `rm -rf /` and of system directories, `mkfs` / `wipefs` / partitioning, `dd` to disks, fork bombs, stopping or killing the SSH server, flushing the firewall, killing PID 1, stopping the audit, `crontab -r`, deleting or locking root, writing to `/etc/shadow`, `/etc/passwd`, sudoers, `/boot`, raw disks or `authorized_keys`, `curl … | sh`. `shutdown` / `reboot` are blocked unless the `ai_allow_power` policy allows them **with an approval** (never automatically). Administrators add their own patterns (`ai_blocked_commands`); the built-in list is shown in *Admin panel → AI assistant*.
- **Sensitive files are not read without you:** password hashes, SSH and TLS private keys, cloud and tool credentials (`~/.aws/credentials`, `.kube/config`, `.netrc` …), process environments — also through wildcards or symlinks — plus your own patterns (`ai_read_deny_paths`). Since v12.6.1 reading one with `read_file` / `tail_log` asks you each time and the AI gets the content with secret-looking values redacted; with `ai_sensitive_reads` = `deny` they are never read. Commands that name such a file always need an approval.
- **What the AI sees** (👁 in the panel): the tool results of this session, the OS of the server and, only if you tick it, your notes for the connection. Never the passwords and keys stored in WRM, your WRM sign-in, other connections or your terminal screen. **Secrets in output are redacted** before they are sent to the provider (`ai_redact_output`): private keys, passwords and tokens in `key=value` form, `Authorization` headers, passwords in URLs, AWS / GitHub / GitLab / Slack / OpenAI / Anthropic / Google keys, JWTs, password hashes. Output is limited per call (`ai_output_max_kb`).
- **Server output is untrusted data.** Tool results are framed as untrusted data and the model is told never to follow instructions found in them. A log line that says *switch to automatic mode* changes nothing: only you can change the mode, in the WRM interface, and every change goes through the same checks.
- **Work log, titles and continuing a chat:** the work log shows the assistant's thinking (💭; Anthropic adaptive thinking on the models that support it, `reasoning_content` of OpenAI-compatible servers), its narration before each step and every tool result (`ai_show_thinking`). After the first answer the chat gets a short title (`ai_auto_titles`; click it to rename). A chat that ended (WRM restart, idle time, kill switch) shows **Continue chat**, which starts a new session with the earlier conversation as context; the history lists title, date, time and *Continue*.
- **My AI settings** (⚙ in the panel, stored per user like the panel layout): your default provider (an organisation provider or your own key, when `ai_personal_keys` allows), model, mode and engine for new chats, and whether you see the model's thinking (only while `ai_show_thinking` is on). The work log keeps the whole thinking text in a scrolling box.
- **Stop and kill:** ■ *Stop* cancels the running request and command; ⏻ ends the session. Administrators end one session, all sessions of a user, or every session (*Stop every AI session now*), turn the assistant off for a user, or turn on the **kill switch** (`ai_kill_switch`, also `WRM_AI_KILL_SWITCH=1`), which ends everything and blocks new sessions. An **✦ AI** badge on the window shows that a session is active.
- **Providers:** the **Anthropic API** (the latest Claude models, e.g. `claude-opus-5-5`, `claude-sonnet-5-5`, `claude-haiku-5-5`; an optional gateway base URL), the **OpenAI API**, **Azure OpenAI** (deployment + api-version, or the v1 API), **AWS Bedrock** (the Converse API with SigV4 access keys or a Bedrock API key, or the Messages API endpoint with an API key; model IDs such as `anthropic.claude-opus-5-5`), **Google Vertex AI** (Claude through `rawPredict`, or the OpenAI-compatible endpoint, with a service account key) **OpenCode Zen** (pay-as-you-go: free models without a key, every model with an OpenCode account's key), **OpenCode Go** (the Go subscription key; see [OpenCode Zen, Go, agents and helper models](#opencode-zen-go-agents-and-helper-models)) and any **OpenAI-compatible** base URL (Ollama, vLLM, LM Studio …). Responses are streamed; tokens and the estimated cost are counted per request (built-in list prices for Anthropic models, your own prices for others); requests time out and are cancelled with *Stop*.
- **Keys:** the organisation's providers are managed by administrators (*Admin panel → AI assistant → Organisation providers*); personal keys (*🔑 My API keys* in the panel) only when `ai_personal_keys` allows them. Keys are encrypted at rest and **never sent to the browser**; *Test* sends one tiny request.
- **Consumer subscriptions do not work here:** Claude.ai and ChatGPT chat subscriptions cannot be used by third-party applications as an API. Use an API key (or your company's gateway) for this panel — or let the chat app itself connect to WRM over MCP: see [AI desktop apps (MCP)](#ai-desktop-apps-mcp).
- **Shared connections:** the assistant can be used on a shared connection by signed-in users whose role allows terminals and file changes (*Operator* and up); their notes are not shared.

**Admin policies** (*Admin panel → AI assistant*): who may use it (`ai_assistant`), the allowed modes (`ai_modes`) and **mode rules per tag, folder, connection or host** — the **most restrictive** matching rule wins, e.g. `[{"match":"tag","value":"prod","modes":["read_only"]}, {"match":"folder","value":"Staging","modes":["read_only","ask"]}]` — the default mode, providers and models (`ai_provider_kinds`, `ai_models`), personal keys, redaction, sharing notes, the approval timeout, the automatic mode's maximum time and actions, power commands, extra blocked commands and unreadable files, the command time limit, the output limit, the steps per request, the idle timeout and the **transcript retention** (`ai_transcript_retention_days`). A running session follows a policy change at its next tool call.

**Audit and transcripts:** every AI request (provider, model, tokens, cost), every tool call with its inputs and its (redacted) output, every approval and denial with who and when, mode changes, stops and kills are audit events (`ai.*`) linked to the connection. The session transcript (redacted) is kept for `ai_transcript_retention_days` and shown to its user (🕘 in the panel) and to administrators (*Admin panel → AI assistant → Transcript*); it is also **recorded like a terminal session** (*Sessions & recordings*, marked ✦ AI, with replay).

### OpenCode Zen, Go, agents and helper models

[OpenCode](https://opencode.ai) is an open-source coding agent (MIT licence) with two model gateways, **OpenCode Zen** (pay-as-you-go) and **OpenCode Go** (a monthly subscription). WRM uses both, in independent parts. Everything stays visible: what runs, which model got what, tokens and cost.

**1. OpenCode Zen as a provider.** Add it in *Admin panel → AI assistant → Organisation providers* (for everyone), or as a personal key under *🔑 My API keys* when `ai_personal_keys` allows it; the provider wizard has an **OpenCode Zen** preset.
- **Without a key** WRM offers only the **free models** (it sends OpenCode's public key, as OpenCode's own client does). Nothing else is needed.
- **With a key** from your OpenCode account (*opencode.ai/auth* → API keys) every model of the account is available, billed by OpenCode.
- The model list comes from OpenCode's public catalog (`models.dev`): names, prices, context size and which API each model speaks. Models that only work over OpenAI's Responses API or Google's API are refused with a clear message; choose another one.

**OpenCode Go.** Go is a subscription with included monthly usage, not a Console balance. A Go key is only accepted by Go's endpoints, so using it as an OpenCode Zen provider makes Console answer *insufficient funds*. Add the separate **OpenCode Go (subscription key)** provider with the key from *opencode.ai/console/go*:
- WRM sends the key to `https://opencode.ai/inference/go/...`, with Chat Completions or Anthropic Messages chosen per model (Qwen and MiniMax M3 use Messages on Go even where the Zen catalog says chat), and a stable `x-opencode-session` per conversation.
- **Load models** asks Go for its current list; if the list cannot be reached, the documented models are offered. Models that only work over OpenAI Responses or Google Gemini are left out.
- The **OpenCode agent** engine accepts the same provider and runs `opencode-go/<model>`; no Zen balance is involved.

**2. Helper models (delegation).** With `ai_delegate` on, the assistant may hand a self-contained text job to a helper model: summarise a long log, classify, explain an error, draft a snippet or some code. The helper gets only the text the assistant passes, with secrets redacted and at most `ai_delegate_max_kb`; it has **no tools** and cannot touch a server.
- **Automatic choice:** with `ai_delegate_models` empty WRM uses the whole catalog of the provider in `ai_delegate_provider` and picks per job: free models first, then coding and reasoning models for code and drafts, reasoning models for explanations, large-context and fast models for summaries. If a model fails, WRM tries the next one (at most three). A list of models narrows the choice.
- **Data-use labels** on every model, in the admin list, the assistant setup and each delegation card: 🛡 *zero retention*, ⚠ *may use prompts for training*, ❔ *unknown terms* (treated like ⚠), 💳 *paid*, 🔑 *your own provider*. The labels follow OpenCode's published terms for its free models.
- **Training is off by default.** Models marked ⚠ or ❔ are only used when allowed: `ai_delegate_training` = `user` (default) lets each user switch them all on for themselves in the assistant setup (*Helper models* → *Also use free models that may use my prompts for training*), `all` allows them for everyone, `off` never.
- Each delegation is a card in the chat (task, model, label, size, tokens, cost), part of the transcript and audited (`ai.delegated`, `ai.delegate_result`, `ai.delegate_denied`; a user's training choice `ai.delegate_training`).

**3. OpenCode agents.** In the assistant setup choose **Engine → OpenCode agent** and an agent: **build** does the work, **plan** looks and plans. The WRM mode (read-only, ask, automatic) applies as for the WRM assistant and is fixed for the session.
- **Installed by WRM, once.** *Admin panel → AI assistant → OpenCode agents* shows the state. **Download and install** asks for confirmation, then downloads the official package for the WRM host from the npm registry (`ai_opencode_registry`, an HTTPS mirror also works), checks its **sha512** against the registry, unpacks only the `opencode` binary into `<DB>.opencode/bin/<version>/` and runs `--version`. The log is shown live; who installed which version is recorded and audited (`opencode.install_started`, `opencode.installed`). Or point `ai_opencode_path` at an OpenCode binary you installed yourself.
- **How it runs:** one `opencode serve` process per session on `127.0.0.1`, in its own folder under `<DB>.opencode/run/`, with a configuration WRM writes: one provider (OpenCode Zen, Anthropic, OpenAI or OpenAI-compatible providers of WRM; the key is passed in the process environment, never written to disk), **all of OpenCode's own tools switched off** (shell, file read and edit, web fetch and search, LSP, questions) and **WRM as its only tool server** (MCP over a loopback-only listener). The process gets a minimal environment without WRM's variables.
- **It works through WRM.** The agent uses WRM's server tools in a WRM session opened for it, so every command and file edit goes through WRM's permission engine and appears in the chat with the same **approval cards**; nothing changes on a server without your approval unless full access is on. OpenCode's own steps are shown too. If OpenCode asks for a permission or a question, WRM refuses it and audits `ai.opencode_refused`.
- **Option: OpenCode's own tools in a Docker sandbox** (`ai_opencode_sandbox` = `admins` or `all`; `off` by default). The setup then offers **🐳 OpenCode's own tools, in a Docker sandbox**. The agent runs in a container on the WRM host and may use its shell, file and web tools there, for scratch work such as scripts, parsing output it got through WRM, or downloads; servers are still reached only through WRM's tools.
  - **Every use asks:** OpenCode's `bash`, file changes and `webfetch` are set to *ask*, and each request is a WRM approval card with the exact command, the diff (secrets redacted) or the URL. It cannot be edited: approve it once or refuse it, with an optional note. No mode, full access included, approves it automatically. Reading files inside the sandbox folder needs no approval; any other permission is refused.
  - **The container:** image `ai_opencode_sandbox_image` (default `debian:stable-slim`; any image with a shell), WRM's own user ID, no capabilities, `no-new-privileges`, read-only root file system, `/tmp` as tmpfs, 2 GB memory, 2 CPUs, 512 processes. Only the session's folder is mounted (`/work`, working folder `/work/work`) plus the OpenCode binary read-only; OpenCode's port is published on `127.0.0.1` only. Containers are labelled per WRM installation and removed when the session ends, when WRM stops and after a crash on the next start.
  - **Setup:** Docker on the WRM host (Linux engine or Docker Desktop with Linux containers; Medium's step 2 can install it) and WRM not running in a container itself. *Download and install* then also fetches the Linux build of OpenCode for the Docker server (sha512-checked), pulls the image and runs OpenCode once in it; the admin page shows *Docker sandbox ready* or what is missing.
  - **Network:** the container has normal outbound network access (web fetches still ask). It reaches WRM's MCP listener through `host.docker.internal` (on Linux WRM listens on the Docker bridge gateway for this, token-protected); an AI provider on the WRM host's loopback address is forwarded to that one container. Tested with Docker Engine on Linux; Docker Desktop on Windows and macOS uses its own `host.docker.internal`.
- **Lifetime:** the session's AI connection (kind `opencode`) works only from `127.0.0.1` and only while the agent runs; ending the session, a kill, the kill switch, an idle timeout or stopping WRM stops the process and removes its folder. At most `ai_opencode_max_sessions` agents run at once; `ai_opencode` decides who may use them (`off`, `admins`, `all`).

**OpenCode on your own computer** can also use WRM as an MCP server: *Settings → AI connections → Connect an AI app → OpenCode* gives the `opencode.json` snippet (token in `WRM_MCP_TOKEN`) or the OAuth login (`opencode mcp auth wrm`).

### AI sessions

**🧠 AI sessions** (a panel; *Panels* menu) lists all of **your own** AI sessions, newest first, whatever started them: the assistant in a terminal, the log and file assistants, OpenCode agents and AI apps over MCP. Nobody else's sessions are listed, also not for administrators (they have *Admin panel → AI assistant → Transcript*).

- **Search and filters:** the search looks in the title, server, model, provider and the conversation itself; filters for kind (assistant, logs, file, OpenCode, AI app), *Live* and *Saved*.
- **Open:** the transcript (redacted, as stored) with thinking, steps, approvals and results; a live MCP session opens its live view, a live chat goes to its window.
- **Continue:** starts a new assistant chat on the same server with the earlier session as context. An MCP or OpenCode session can be continued too: its tool calls and results become the earlier context.
- **Save:** ☆ keeps a session beyond `ai_transcript_retention_days` while `ai_keep_saved_sessions` is on. **Rename** and **Export** (Markdown or JSON; audited as `ai.session_export`).

### AI desktop apps (MCP)

WRM is an **MCP server** (Model Context Protocol). AI apps that speak MCP — **Claude** (the desktop app and claude.ai), **ChatGPT** (connectors / developer mode), Claude Code and other MCP clients — can open a WRM session to a server you chose and work on it, under the same rules as the built-in assistant. The model runs in the app, on your account there; WRM never sees your chat and never needs a provider key.

**Quick start: all AI apps at once (v13.13.0).** *Settings → AI connections → 🧰 All AI apps at once* downloads one script for the computer where your AI apps run (`wrm-setup-all.ps1` for Windows, `wrm-setup-all.sh` for Linux and macOS). Run it there:

```sh
# Windows (in the folder with the file)
powershell -ExecutionPolicy Bypass -File .\wrm-setup-all.ps1
# Linux / macOS
sh wrm-setup-all.sh
```

It opens WRM in the browser, where you sign in and choose the servers, the mode, the scopes and the lifetime (the normal OAuth consent dialog), saves the connection once in `~/.wrm/mcp.json` and adds WRM to every AI app it finds: **Claude Code** (as the WRM PRO plugin), the **Claude app** (`claude_desktop_config.json`, chat and Cowork), **Codex CLI**, **Gemini CLI**, **VS Code**, **Cursor**, **Windsurf** and **OpenCode**. Files it changes are copied to `<file>.wrm-backup-<time>` first; apps it does not find are listed as skipped. All of them start the same small bridge (`~/.wrm/wrm-mcp.js`) with the saved connection, so when the access expires, `wrm_connect` in any one of them signs in again for all. The script holds this WRM's address and the bridge, never a token; with `WRM_MCP_TOKEN` set it uses that token instead of the browser sign-in. It needs Node.js 18 or later. `POST /api/mcp/setup-all` builds it; downloads are audited (`mcp.setup_script`).

**Quick start for the Claude app and Claude Code: the WRM PRO plugin.** This repository is a plugin marketplace. The plugin needs **no settings**:

- **Claude app** (desktop or claude.ai): *Customize → Plugins → Add → Add marketplace* → `vedranius/wrm-pro-enterprise` (in lowercase) → **WRM PRO** → *Install*. If adding the marketplace fails, *Add → Upload plugin* with `claude-plugin-wrm-pro.zip` from the [latest release](https://github.com/vedranius/wrm-pro-enterprise/releases/latest) installs the same plugin. If it says only *Failed to add marketplace*, see [Troubleshooting](#troubleshooting).
- **Claude Code**: `claude plugin install wrm-pro --marketplace vedranius/wrm-pro-enterprise`, or inside Claude Code `/plugin marketplace add vedranius/wrm-pro-enterprise` and `/plugin install wrm-pro@wrm-pro`.

Then type **`/wrm-pro:wrm`** (typing `/wrm` finds it). The first time, Claude asks for the WRM address (the one you open in the browser, for example `http://192.168.1.10:8780`) and calls `wrm_connect`, which opens WRM in the browser for sign-in and approval; the token goes straight to `~/.wrm/mcp.json` on your computer, never into the chat. After that `/wrm-pro:wrm` lists your servers and everything WRM can do. The plugin's MCP server is the same bridge as the Claude Desktop extension, packed as an MCP bundle (`packaging/claude-plugin/wrm-pro/server/wrm.mcpb`) that the app runs on your computer with its built-in Node.js, so it reaches a WRM on your own network and over plain HTTP. In the Claude app the bundled server runs in Cowork; chat in the desktop app uses the extension or the setup script above (chat ignores local servers of plugins).

Plain MCP without the plugin (any Claude Code version; a token from the wizard):

```sh
claude mcp add --transport http --scope user wrm "https://wrm.example.com/mcp" \
  --header "Authorization: Bearer wrm_pat_…"
```

A local WRM (`http://localhost:8080/mcp`) works with this command **only with the token header**: Claude Code opens a sign-in page only over HTTPS. *Settings → AI connections* shows the command with your address and token after the wizard creates it, and has a **Copy** button.

> **Using a Claude or ChatGPT subscription with WRM:** consumer chat subscriptions cannot be used by other applications as an API (so the built-in assistant needs an API key). The supported way to use your Claude or ChatGPT account with WRM is the other direction: the chat app connects to WRM over MCP, as described here.

**Connect an AI app (wizard)**

*Settings → AI connections* opens with a step-by-step wizard: **1.** pick the app (Codex CLI, **Claude Desktop**, Claude Code, Gemini CLI, Cursor / VS Code / other, or claude.ai / ChatGPT through sign-in); **2.** pick the servers, how long the token is valid and what the app may do (*look only*, or *look and change with your approval* when the policy allows `ask`); **3.** copy or download the ready command or config file; **4.** press *Test*, which runs `initialize` and `tools/list` against the MCP address with the new token. The token is created by the audited `POST /api/mcp/grants` and shown once; the old form is under *Advanced*. For Codex CLI: `codex mcp add wrm --url <address> --bearer-token-env-var WRM_MCP_TOKEN`; for Claude Code: `claude mcp add --transport http wrm <address> --header "Authorization: Bearer …"`; for Gemini CLI: `gemini mcp add --transport http wrm <address> -H "Authorization: Bearer …"`.

The wizard also offers an `AGENTS.md` / `CLAUDE.md` / `GEMINI.md` text that explains to the agent that **"wrm" is the WRM PRO MCP server**; the server sends the same explanation in `instructions` during `initialize`, so clients no longer ask what *wrm* is.

**Permission messages.** If a tool is refused, WRM answers `Denied by WRM: this AI connection's scopes (…) do not allow <tool>; it needs one of: …`. This is a limit of the **AI connection** (scopes or mode), not of the server or the server account: create a new AI connection with that scope (Settings → AI connections) and reconnect. the `open_session` result lists `not_allowed` and `how_to_allow`.

**How it works**

- An **AI connection** binds **you**, the **servers** you pick (your own SSH connections), one **mode** (read-only, ask before every change, or automatic within limits), the **scopes** and a **lifetime** in hours or days (default 7 days, at most what the policy allows, by default 30 days). It is a **personal access token** you create in *Settings → AI connections*, or an **OAuth approval** you give when an app connects. Tokens are shown once and stored only as a hash; they can be revoked at any time.
- The app sees these tools: `list_connections` (the servers of the AI connection and the mode WRM applies there), `open_session` / `close_session`, and the WRM tools the scopes allow, each with the `session_id` of an open session:

| Scope | Tools | What WRM does |
|---|---|---|
| `read_logs` | `system_info`, `read_file`, `list_directory`, `tail_log`, `journal`, `service_status`, `list_packages`, `processes`, `disk_usage`, `network_status` | Reads; sensitive files (password hashes, private keys, credentials) are refused |
| `run_readonly` | `run_command` | Only commands the **read-only classifier** accepts |
| `run_with_approval` | `run_command` | Also commands that change something: each waits for **your approval** (or the automatic-mode allow list) |
| `edit_file_with_approval` | `write_file`, `edit_file` | File changes as a **unified diff** that you approve |
| `transfer` | `transfer_file` | Copies one file from one server of the AI connection to another over SFTP, after your approval; recorded in *File transfers* with size and SHA-256 |

- **Every call goes through the v12.0.0 permission engine**: the read-only classifier, the always-blocked destructive patterns, the approvals, the automatic-mode limits (per session), the mode rules per tag / folder / connection / host (the most restrictive wins — a production server can be read-only for apps even when the AI connection allows *ask*), redaction of secrets in output, and the audit.
- **Approvals** appear in WRM as a card with **Approve**, **Deny** and **Edit** (the exact command, or the diff), on every open WRM tab of yours — and, when the app supports **MCP elicitation**, also as a question in the app (policy `ai_mcp_elicitation`). Whichever answers first decides; an unanswered approval is denied after `ai_approval_timeout_seconds`.
- **Server output is untrusted.** Tool results are framed as untrusted data; nothing in them, and nothing the app asks for, can change the mode or the scopes: there is no such tool, and only you change them, in WRM (by creating a new AI connection).
- **Live view:** when an app opens a session, WRM opens a panel that follows it: every tool call with the command or file and WRM's decision (*runs*, *needs your approval*, *refused by WRM*), **Approve** / **Deny** for pending approvals, the outcome, the exit code and the output. *Watch live* in the 🤖 menu reopens it; the checkbox there turns the automatic opening off for this browser. Closing a session's box does not stop the app.
- **Where the live view sits:** it is a panel of the [panel layout](#panel-layout-dock-stack-float-and-lock) (v12.6.8): drag its **🤖 AI** tab anywhere, or use the buttons in its title bar (▕ right, ▁ bottom, ⧉ floating, – hide) or *Layout* in the 🤖 menu.
  - *Right* (default): a **🤖 AI** tab next to *Sessions* and *Snippets*. The tab shows how many AI sessions are open and turns yellow while one waits for your approval; the zone keeps its own width while it shows the live view (drag its left edge).
  - *Bottom*: under the windows; drag its top edge to resize it. Several sessions sit side by side.
  - *Floating*: move it by the empty part of its tab bar, resize it by its corner.
  - *Hidden*: a *Show the AI panel* chip at the bottom left brings it back; it pulses while an approval waits.
  - The layout is stored for your account. Approval cards always pop up at the bottom right, whatever the layout. On a phone the live view is a sheet at the bottom of the screen.
- **After a refusal** (denied in WRM or in the app, or no answer within `ai_approval_timeout_seconds`) the tool result tells the AI where it was refused, **which rule made WRM ask** (the time or action limit, a command outside the allow list, ask mode, a power command) and what you can change in WRM, and to ask you whether to continue, change the command or stop, instead of retrying a variant on its own.
- **Full access (v12.6.2, modes v12.6.3):** the administrator's `ai_full_access` switch applies, depending on `ai_full_access_mode`, to every AI session (`all`, the default: the WRM AI panel and every AI connection), only to AI connections with the scope `full_access` (`connection`; the scope is offered only in this mode), or to users who turn on **Full access for my AI sessions** in *Settings → AI connections* (`user`). Then nothing is refused outright: commands on the destructive list or `ai_blocked_commands`, the connection's deny lists, power commands, protected paths and secret files (also with `ai_sensitive_reads` = `deny`) become approvals marked *FULL ACCESS*, asked every time and never automatic, even with "Allow everything". A read-only connection stays read-only and the other scopes still apply. With `ai_full_access_blocked` = `deny` (v12.6.4) the administrator's `ai_blocked_commands` stay refused even under full access. Turning the policy off restores the refusals at the next call.
- **Approve later:** an approval nobody answers within `ai_approval_timeout_seconds`, that the app declines on its own, or that the app stops waiting for does not disappear: it stays in WRM for an hour (card, live panel, 🤖 counter). Since v13.8.0 approving it there **runs the call at once** (`ai_mcp_later_approval` = `run`, the default): the output appears in the live view, and when the AI repeats the same call it gets that output (marked as run after your approval) instead of running it again. With `repeat` the approval only lets the identical call run once without asking when the AI repeats it. This also works after the AI app closed its session (v12.6.5): the approval covers the same call once in your next session on that connection. A denial in WRM stays a denial, and so does *Deny* in the app's question (v13.8.0); only closing the question without an answer keeps the request for later. When you decide in WRM, the app's open question is closed. Deciding a request that is no longer waiting tells you who decided it and where (in WRM or in the AI app), or that the AI stopped waiting. Refusals that an approval cannot change (destructive patterns, deny lists, read-only sessions, scopes) say so.
- **Allow everything:** in automatic mode an AI connection can allow every command (`*`) and file (`/**`) with **no time or action limit**: it then stays automatic until the AI connection expires or you revoke it. With `*`, chains, variables, groups and redirections run too. Still refused or asked: the built-in destructive patterns and `ai_blocked_commands`, your deny lists, sensitive files, commands WRM cannot parse, power commands, and servers the connection does not include. The policy `ai_auto_until_grant` (default on) decides whether users may choose no limits; the browser's built-in assistant always keeps its limits.
- **🤖 AI connected** in the top bar shows while an app has an open session (with the number of pending approvals); click it to see the apps and **revoke** one or all with one click. Revoking ends the sessions at once; so do expiry, the kill switch and the policies.
- Each session is an AI session with transport *mcp*: in the audit log (`mcp.*` for connecting, tokens and OAuth; `ai.*` for every tool call, approval and denial), in the transcripts and in **Sessions & recordings** (✦ AI), like the built-in assistant.

**Set up your app** — *Settings → AI connections* shows the address (`https://<your WRM>/mcp`) and, after you create a token, ready-to-copy snippets:

1. **Claude Desktop — one-click extension (recommended for Claude Desktop).** In the wizard pick *Claude Desktop*, choose the servers and press *Create token*, then **Download Claude Desktop extension**. Open the downloaded `wrm-pro.mcpb` (or *Claude → Settings → Extensions → Advanced settings → Install extension…*) and press **Install**: the WRM address and the token are already filled in. The extension is an [MCP bundle](https://github.com/modelcontextprotocol/mcpb) with a small Node.js bridge (`remote-manager/mcpb/server/index.js`, no dependencies) that Claude Desktop runs with its own built-in Node.js; it passes MCP messages, including approval questions, between Claude and WRM's `/mcp`. It runs on your computer, so it reaches a WRM on your own network and over plain HTTP; for a self-signed HTTPS certificate choose the CA file in the extension's settings. Claude Desktop keeps the token in the operating system's secure storage; delete the downloaded file after installing, since it contains the token. When the token expires or is revoked, the extension keeps running and offers `wrm_connect` with the reason (since v13.13.0): ask Claude to connect to WRM again and approve the sign-in in the browser, or download the extension again with a new token. `POST /api/mcp/claude-desktop` builds the bundle; downloads are audited (`mcp.desktop_extension`).
2. **Claude app and Claude Code — WRM PRO plugin** (no settings, see the quick start above). The plugin (`packaging/claude-plugin/wrm-pro`, version 1.2.1) carries the bridge as an MCP bundle, the skills `wrm` (`/wrm-pro:wrm`), `wrm-safe-operator` and `linux-incident-triage`, and the command `/wrm-pro:connect`. Without a saved connection the bridge starts anyway and offers one tool, `wrm_connect`; a wrong, expired or revoked token or an unreachable WRM also leaves it running with that tool and the reason. `wrm_connect` registers itself with WRM (dynamic client registration with a `http://127.0.0.1:<port>/callback` redirect, which the default `ai_mcp_redirect_uris` allows), opens WRM's consent dialog in the browser, exchanges the code with PKCE and saves the token in `~/.wrm/mcp.json` (`WRM_MCP_CONFIG` to move it; mode 0600). The app is told that the tool list changed, and the WRM tools appear. `wrm_connect` with another address switches to another WRM. It needs AI connections with OAuth (`ai_mcp_oauth`, on by default). Needs Claude Code 2.1.275 or later for the one-line install.
3. **claude.ai — custom connector with OAuth.** *Settings → Connectors → Add custom connector*, enter `https://<your WRM>/mcp`. Claude registers itself (dynamic client registration), opens WRM in the browser, you sign in (with 2FA) and choose the servers, the mode, the scopes and the lifetime; Claude then gets a token for exactly that. claude.ai connects from Anthropic's cloud, so WRM must be reachable from the internet over HTTPS (put it behind a reverse proxy with a real certificate, and keep the other hardening of [SECURITY.md](SECURITY.md)).
4. **ChatGPT — connector.** *Settings → Apps & Connectors → Advanced settings → Developer mode*, then *Create* (*Add custom connector*): the MCP server URL `https://<your WRM>/mcp`, authentication **OAuth**. ChatGPT opens WRM for your approval, as above. ChatGPT also connects from its cloud: WRM must be reachable over HTTPS.
5. **Any stdio client — the bridge in the WRM binary** (no inbound access from the internet needed; Claude Desktop users prefer the extension above). Copy the WRM binary for your computer's system (the same release file as the server) to your computer, create a token, and add to `claude_desktop_config.json` (*Claude → Settings → Developer → Edit config*):
   ```json
   {
     "mcpServers": {
       "wrm": {
         "command": "/path/to/wrm-pro",
         "args": ["-mcp-stdio", "-url", "https://wrm.example.com/mcp"],
         "env": { "WRM_MCP_TOKEN": "wrm_pat_…" }
       }
     }
   }
   ```
   The bridge (`wrm -mcp-stdio -url … -token …`, or the token from `WRM_MCP_TOKEN` / `-token-file`) passes MCP messages between stdin/stdout and WRM's `/mcp`, including approval questions; `-ca-file` trusts a private CA or a self-signed WRM certificate. Use it for any client that only starts local (stdio) servers.
6. **Other MCP clients** (Claude Code, IDE agents, your own scripts) with streamable HTTP and a header:
   ```json
   { "mcpServers": { "wrm": { "type": "http", "url": "https://wrm.example.com/mcp", "headers": { "Authorization": "Bearer wrm_pat_…" } } } }
   ```
   e.g. `claude mcp add --transport http wrm https://wrm.example.com/mcp --header "Authorization: Bearer wrm_pat_…"`. Clients that support OAuth discovery (the `WWW-Authenticate` answer with `resource_metadata`) can use OAuth instead of a token.

**Admin** (*Admin panel → AI connections*): every active AI connection (user, app, servers, mode, scopes, expiry, last use and address) with **Revoke** and **Revoke all**, the registered OAuth apps (remove one to revoke its connections), and the policies: MCP on/off (`ai_mcp_enabled`, default off), who may use AI connections (`ai_mcp`: off / admins / all, default admins), allowed scopes (`ai_mcp_scopes`) and modes (`ai_mcp_modes`), default and maximum lifetime (`ai_mcp_token_hours`, `ai_mcp_max_token_hours`), OAuth on/off (`ai_mcp_oauth`), the **redirect URI allowlist** (`ai_mcp_redirect_uris`: by default the callbacks of claude.ai / claude.com and ChatGPT, plus `http://localhost:*` for desktop clients), browser origins allowed to call `/mcp` (`ai_mcp_allowed_origins`, CORS), the public address (`ai_mcp_public_url`), approvals in the app (`ai_mcp_elicitation`) and the transfer size limit (`ai_mcp_transfer_max_mb`). The AI assistant's kill switch, mode rules, blocked commands, unreadable paths, approval timeout, redaction and limits apply to MCP sessions too.

### Network tools

**🧰 Tools** in the top bar (or right-click a connection → *Network tools from this server…* / *Check ports of this host*) — the usual first checks when something does not answer, without opening a terminal:

| Tool | What it does |
|---|---|
| 🔍 **Port check** | TCP connection to up to 100 ports (`22, 80, 443`, ranges like `8000-8010`): **open** (with time), **closed** (refused) or **no answer** (filtered by a firewall) |
| 📶 **Ping** | 4 pings, the output of `ping` |
| 🛤 **Traceroute** | `traceroute` (or `tracepath`; `tracert` on Windows), up to 20 hops |
| 🌍 **DNS** | Addresses, CNAME, MX, TXT, and the name of an IP address (PTR) |
| 🔒 **HTTP / TLS** | One request to the URL (redirects are shown, not followed): status, time, server; for HTTPS the TLS version, certificate name, issuer, validity (**days left**), names and whether it is **trusted** |

**From** chooses where the check runs: the **WRM server**, or **one of your SSH connections** — through its jump hosts. That answers "can *app-01* reach the database on 5432?" or "does the firewall between *dmz-web* and the API let 443 through?". From a server, port and HTTP checks go through SSH channels (nothing to install there); ping, traceroute and DNS run the commands of the server (`ping`, `traceroute`/`tracepath`, `getent`/`dig`/`nslookup`).

Only your own connections can be used as a source; one check per user at a time; targets are host names or addresses only (no options or shell characters); every check is audited (`nettool.run`). Policy `network_tools`: all users (default), administrators only, or off.

### Addons

An addon extends WRM from inside itself: a panel (a window), colour themes and, for organisation addons, WebAssembly server code. It is a ZIP with `addon.json`, installed in **Settings → Addons**. Nothing runs at install time, every file is pinned by SHA-256, and a new addon is installed **off**.

- **Who installs:** an administrator installs for everybody (*Install for: Everybody*) or only for themselves (*Only me*). Every other user installs **personal addons** that only they see, if the policy `addon_personal` allows it (default: everybody). Personal addons cannot carry server code. Administrators find all users' personal addons under *Personal addons of other users* with **Disable** and **Remove**.
- **Enabling** shows the **permission sheet**: what the addon may do, written by WRM from `addon.json` (a window without network, *n* themes, *x* KB of settings per instance, notifications, server code). An update that asks for more stays off and shows *waits for approval* with the new lines until you enable it again; a smaller update keeps it on. Installing an older version asks first.
- **Instances:** *Open panel* opens your instance (the first one is created as *Default*). *Instances* in the addon's card creates more (administrators may share one with everybody), opens, renames, **exports** and **imports** their settings, and removes them. Each instance has its own settings and window.
- **Windows** are ordinary WRM windows (move, resize, snap, minimize, tabs). Their place and size are remembered on the server per user, so open addon windows come back after a reload or on another device. **↻** reloads the panel; **⚠ n** lists the errors it reported. A panel that does not start within 5 seconds or stops answering for 45 seconds shows a banner with *Reload*, *Diagnostics*, *Disable addon* and *Close*.
- **Themes** of enabled addons are offered in *Settings → Appearance*; nobody gets one without picking it.
- **Web sites (v13.11):** an addon with the `http` capability lists its sites (`http.origins`), methods, paths and secrets in `addon.json`. The panel asks WRM, and WRM sends the request: only to a listed site, path and method, after an administrator approved the site (enabling an organisation addon approves its sites; a personal addon shows *sites wait for an administrator* with **Approve** for administrators), with names resolved by WRM and loopback, private, link-local and CGNAT addresses refused unless the administrator ticked **private network** for that site (plain `http://` needs that tick too). Redirects (3 at most) are checked again and the secret is not sent to another site. Limits: 256 KB request, 8 MB response, 10 s, 120 requests a minute and 20 at once per instance; identical GETs within 2 s share one answer. **Withdraw** removes the approval. The window's **⚠** diagnostics list the recent requests.
- **Secrets (v13.11):** **🔑 Secrets** on an instance stores the API key or password of a site, encrypted on the server, as the administrator's value for a shared instance (or the owner's for a private one) and optionally **your own** value that replaces it for you. WRM adds it to the request (`Authorization: Bearer`, `Basic` or a named header); the panel only learns whether it is set. Setting and clearing a secret is audited without the value.
- **What a panel can do:** it runs in a sandboxed frame with no access to WRM, its cookies or other windows and (apiVersion 2) **no network of its own**. It receives the display name, the language, the theme, its instance and its stored settings, and may store settings, show short notifications (5 a minute) and ask for a window size. Addons work while their window is open: WRM runs no background jobs for them, so 24/7 alerting stays in the monitored system.
- **Writing one:** see the [addon developer guide](docs/ADDONS.md) (manifest, bridge, store, themes, error codes) or *AI addon* in the same pane for a ready prompt. The WRM assistant can write a draft that you review (**Diff**) and install.
- **Developer tools (v13.12):** *🛠 Developer tools* in the same pane (administrators, and users who may install personal addons) has **Check ZIP**, which lints an addon without installing it, and **Templates** with the four starters as ZIPs, the manifest schema and `llms.txt`. A failed install opens the lint report on its own; **Copy report** copies it for an AI. On the command line:
  - `wrm -addon-new TEMPLATE [-addon-id ID] DIR` writes a template (`blank`, `fetch-and-render`, `grafana-panel`, `wasm-hook`) into an empty folder; the panel templates include `wrm-bridge.js`, the WebAssembly one a TinyGo source and a placeholder module.
  - `wrm -addon-lint DIR|ZIP` (`-addon-lint-json` for JSON) prints errors, warnings and notes as `level file:line: message [code]` with a hint, then the permission summary; it exits with 1 when there is an error. Nothing in the addon runs.
  - `wrm -addon-dev DIR` serves the panel at `http://127.0.0.1:8790` (`-addon-dev-port`) with the real CSP, a language, theme and visibility switch, an in-memory store, web requests to the listed sites (secrets from `WRM_ADDON_SECRET_<NAME>`; private and plain-http sites only with `-addon-dev-private`), the lint result and the bridge log, and reloads it when a file changes. Options go before the folder.

### SSH terminal

- Full **xterm-256color** terminal (xterm.js 5, served by WRM itself — no CDN) with the JetBrains Mono font. Works with `nano`, `vim`, `less`, `htop`, `mc`, `tmux`, colors, mouse reporting and UTF-8 (č, ć, ž, š, đ, emoji…).
- **Correct size everywhere:** the PTY size follows the window on connect, maximize, snap, resize, sidebar toggle, font change and browser zoom.
- **Large output is safe:** binary WebSocket frames plus flow control, so there are no disconnects on big `cat`/`tail`/`journalctl` output.
- **Reconnect:**
  - **↻** button in the title bar (also while connected, e.g. when a session hangs), **Ctrl+Shift+R**, or the tab menu.
  - When a terminal is closed, a banner offers **↻ Reconnect** (or press **Enter**) and a red **✕ Close window**; **–** hides the banner and stops the automatic reconnect.
  - **Automatic reconnect** when the SSH connection or the connection to the web server drops (backoff 1 s → 30 s, up to 10 attempts), and immediately when the network comes back. It can be turned off in Settings. If the shell simply exits (`exit`), it does not reconnect automatically.
- **Keepalive:** WRM pings the browser every 25 s and the SSH server every 30 s and detects dead connections.
- **Copy on select:** selecting text copies it to the clipboard and adds it to the [Clipboard panel](#clipboard-panel). **Ctrl+Shift+C** copies too, and **Ctrl+Shift+V** / normal paste pastes.
- **Search in the scrollback:** **Ctrl+Shift+F** (Enter = next, Shift+Enter = previous).
- **Clickable URLs** in the output.
- **Browser shortcuts are blocked while a terminal is focused:** **Ctrl+W** is sent to the terminal (nano "Where Is", bash "delete word") instead of closing the browser tab. Ctrl+T, Ctrl+N and Ctrl+Q are blocked too.
- Configurable **font size** (11–18 px) and **scrollback** (5k–50k lines).
- **"Open terminal here"** from the file manager opens a new terminal and `cd`s into that folder.

### File manager (SFTP / FTP / FTPS)

**Preview** in the file context menu opens video/audio, images, PDFs or text inside WRM. Large video/audio files stream with byte-range seeking over SFTP/FTP; browser codec support still applies. Text is recognized from the content, not only from the extension: XML/SMIL/SVG, JSON, YAML, TOML, INI, shell, Python, JavaScript/TypeScript, Go, SQL, CSS/SCSS/LESS, Markdown, PowerShell, batch, Dockerfile, Makefile, C/C++/C#/Java/Rust/Ruby/PHP and similar files are syntax-highlighted by the bundled PrismJS; other textual files open as plain text. UTF-8, UTF-16 (with a BOM or a detected byte order) and single-byte text are decoded; binary files are refused with **Download**. Text previews show at most 256 KiB, with a truncation notice and Download. HTML/SVG/source files render as escaped text. Closing the viewer stops playback. Every metadata/range/content request checks the same file-read/share permissions as downloads; FTP requires SIZE and REST support for seekable playback.

In a terminal, select a single file path with the pointer and right-click for **Preview selected path** or **Download selected path**. Relative paths use the terminal's current directory; `~/` resolves the SSH file account's home. Paths are treated as filenames and never executed as shell commands.

- **Breadcrumb path bar:** click any part to jump there, or click empty space to type a path (Enter). **↑ Up** and **⟳ Refresh** (F5).
- **Columns:** name (with icons by file type, symlink marker), **Modified**, size (B … TB), type. Click a column header to sort (folders always first, natural sort).
- **Selection:**
  - click to select, **Ctrl/Cmd + click** to toggle, **Shift + click** for a range, checkbox, *select all*, **Ctrl+A**
  - keyboard: arrows, Home/End, **Enter** (open), **Backspace** (up), **Delete**, **F2** (rename), **F5**, **Ctrl+F** (search), Esc
- **Double-click:** folder opens; text file opens in the [editor](#text-editor); other files download.
- **Upload:**
  - **⬆ Upload** (many files at once), **🗂 Folder** (a whole folder with subfolders), or **drag & drop** files or folders from your computer onto the list
  - progress bar with speed, cancel button
  - existing files are detected, and you are asked whether to **overwrite** them
  - optional per-file size limit (Settings; administrators can enforce a limit on the server)
- **Download:** single files, or **folders as ZIP** (SFTP). **⬇ Download** downloads all selected items.
- **📁 New Folder**, **✏️ Rename** (F2), **🗑 Delete** (selected files and folders; folders recursively, with confirmation).
- **⚡ Transfer** sends the selection to another server (see [Server-to-server transfer](#server-to-server-transfer)).
- **⌨ Terminal** opens a terminal in the current folder (SSH/SFTP only).
- **Right-click menu:** Open, Edit, Download / Download as ZIP, Transfer to server, Open terminal here, Copy path, Rename, Delete. It works on multiple selected items.
- **Status bar:** number of folders and files, selection count and size, connection name.
- In narrow windows the toolbar switches to icons only.

### Search in files

The search box at the top of every file manager:

- **Typing filters the current folder instantly.** Wildcards work: `*.log`, `nginx*.conf`, `?`.
- **Enter** (or ⌕) searches **all subfolders** of the current folder on the server. Results stream in live with a progress indicator (folders scanned, current folder) and can be **stopped** at any time.
- Search options (▾):
  - **File names** (default) walks the tree over SFTP/FTP. Works on every server, including Windows SFTP and FTP.
  - **File contents** runs `grep` on the server over SSH: case-insensitive, fixed string, text files only. It needs a POSIX shell with `grep` (Linux/BSD/macOS servers). Not available for FTP.
  - include/skip **hidden** files; result **limit** (200 / 500 / 2000 / 5000).
- When searching from `/`, the virtual filesystems `/proc`, `/sys`, `/dev`, `/run` and `/snap` are skipped. A search stops after 3 minutes or 1,000,000 scanned entries, and tells you if it was truncated.
- Results show the file, its folder, size and date. **Double-click** a result to open its folder with the file highlighted. Results support selection, the context menu, download and delete.

### Text editor

- Opens text and config files up to **5 MB** in the browser (double-click, or right-click → *Edit*). Binary files show a warning first. Read-only shares open read-only.
- **Syntax highlighting** for about 40 languages (shell, YAML, JSON, INI and systemd units, nginx, Apache, Dockerfile, Python, Go, JavaScript, SQL …), chosen from the file name and changeable in the toolbar; it is off above 300 KB. Line numbers, the current line and column, wrap.
- **Find and replace** (`Ctrl+F`, `Ctrl+H`): literal or regex, case, all hits marked, *Replace* and *Replace all*. **Go to line** (`Ctrl+G`). `Tab` / `Shift+Tab` indent and outdent the selected lines, `Enter` keeps the indentation.
- **JSON:** *Check JSON* (with line and column of an error) and *Format JSON*; saving invalid JSON asks first.
- **Saving** (`Ctrl+S`): *Review before saving* (on by default) shows a diff of your changes first. Before writing, WRM reads the file again: if someone else, a service or the AI assistant changed it since you opened it, you choose to overwrite or to see the differences. **± Changes** shows your changes at any time and **⟲ Reload** reads the file again. Line endings (LF or CRLF) and UTF-8 are kept.
- **🤖 AI assistant:** a panel next to the file that reviews it (bugs, security problems, wrong configuration), explains it and proposes changes. Every change is a diff that waits for your approval; after it the editor reloads the file, or warns if you have unsaved changes. **Ask AI about the selection** sends the selected lines. Its chats are kept per file, with title, date and time, and can be continued.

### Server-to-server transfer

Copy files and folders **directly from one SSH/SFTP server to another**. The data flows through the WRM server, not through your browser.

- Open with **⚡ Transfer** in a file manager or from the context menu. The dual-panel dialog shows the source (with your selection) on the left and the destination connection and folder on the right.
- **If a file exists:** *Ask each time* (checks for conflicts first and asks once for all), *Overwrite all*, *Skip existing*, *Overwrite if newer*.
- Folders are copied recursively with their structure. File modification times are preserved. Up to **4 files are copied in parallel**.
- Live progress: per-file progress, total bytes, speed (sliding window), **ETA**, results list with errors.
- **Minimize** the dialog to a small floating progress bar (draggable, always kept on screen) and keep working. **Cancel** stops the transfer on the server too.
- A sound plays when the transfer finishes (success or error).
- FTP connections are not supported as source or destination.

### Workspace sessions

The **Sessions** panel (right by default; see [Panel layout](#panel-layout-dock-stack-float-and-lock)):

- **+ Save Session** saves **all open windows**: connection, terminal or file manager, current folder, snap slot or position (stored relative to the workspace size, so it fits other screens), minimized and focused state. The suggested name is built from the connections.
- **Click a saved session to open it.** If windows are already open, choose **Replace** (close them) or **Add** (open next to them). Saved sessions are also offered in every empty *New Window*.
- **Right-click** a session: *Open (replace)*, *Open (add)*, **Save current windows here** (update), *Rename*, *Lock/Unlock* (a locked session cannot be overwritten, and deleting it needs confirmation), *Delete*.
- Sessions belong to your account and are stored on the server, so they are available in every browser.

### Clipboard panel

The **Clipboard** panel (right by default): the last 20 texts you copied from terminals (and copied paths).

- Click an entry to copy it again. **Paste** sends it to the focused terminal.
- The history is kept per workspace session and **per user** in your browser (local storage), not on the server. Another account signing in on the same browser does not see it, and **signing out removes it**. Use **Clear** to empty it.

### Sharing: members, roles & links

Share connections (or whole folders) with other WRM users or with people **without an account** — the recipients use them **with your stored credentials without ever seeing them**.

Right-click a connection → **🤝 Share**, a folder → **Share Folder**, select several connections and use **🤝 Share**, or *Shares → + New share*. The share dialog has:

- **Connections in this share:** tick connections and/or whole folders (a folder also includes connections added to it later). A share without connections is still a room for chat, voice and terminal sharing.
- **Who can open the link:**

  | Mode | Who |
  |---|---|
  | 👥 **Members only** (default) | Only the users you add as members. They sign in with their account. |
  | 🏢 **Everyone signed in** | Any user of this WRM server who has the link. |
  | 🌐 **Anyone with the link** | Also people without an account (guests). Administrators can disable this mode. |

- **Members** with an individual role each; **role for everyone else** (non-members in the other two modes); optional **password** (members never need it); optional listing under “Shared with me” for all users; **expiry** (1 h … 30 days or a custom date).
- **Roles:**

  | | Observer | Viewer | Operator | Moderator | Owner |
  |---|:-:|:-:|:-:|:-:|:-:|
  | Chat & voice call | ✔ | ✔ | ✔ | ✔ | ✔ |
  | Watch shared terminals | ✔ | ✔ | ✔ | ✔ | ✔ |
  | Browse & download files | — | ✔ | ✔ | ✔ | ✔ |
  | Open terminals | — | — | ✔ | ✔ | ✔ |
  | Upload, edit, rename, delete, transfer | — | — | ✔ | ✔ | ✔ |
  | Share own terminal, receive keyboard control | — | — | ✔ | ✔ | ✔ |
  | Change roles, mute, remove, ban people | — | — | — | ✔ | ✔ |
  | Share settings & members | — | — | — | — | ✔ |

  Roles are **enforced by the server** on every request; the UI only hides what would be refused anyway.
- **🤝 Shares** (top bar) lists your shares with mode, members, expiry and **how many people are online**. Actions: **Open room**, **Copy link**, **Edit** (everything above), **People** (everyone who ever opened the share, with last seen and IP — change a person's role or **ban** them), **New link** (the old link stops working immediately), **Pause/Resume**, **Delete**.
- **Revocation is immediate:** deleting, pausing or rotating a share, removing a member, lowering a role, banning, or disabling a user closes the affected terminals and room connections at once.

### Real-time collaboration

Everyone who opens the **same share link** (including you as the owner) joins its live room. The **collaboration bar** shows the share, your role, avatars of the people online (a green ring shows who is speaking) and the controls.

- **Identities are decided by the server:** signed-in users appear with their account name; guests choose a name on first visit and are marked **guest** (they cannot pose as a registered user).
- **👥 People** panel: who is in the voice call and who is online, with roles, mute/deafen state, shared terminals, raised hands, connection quality (P2P/relay, latency) and a per-person volume slider. Moderators get a **⋯** menu: *change role*, *mute for everyone*, *stop their terminal sharing*, *lower hand*, *remove from room*, *ban from share*.
- **💬 Chat** with **history** (last messages are kept, also for people who join later), timestamps, clickable links, unread badge and notifications. **📎 File exchange** up to the policy limit (images are previewed; other files are always downloaded, never opened in the page).
- **✋ Raise hand.**
- **📡 Terminal sharing:** press 📡 in the title bar of an SSH window. Several terminals can be shared at once. Others click **👁 Watch**: the viewer gets a **snapshot of the current screen** (with colors) and then the live output, in the sharer's exact size. Terminal data is only sent to people who watch it.
- **🎮 Remote keyboard control:** a viewer clicks **Request control**; the sharer sees *Allow / Deny*. The sharer can also give control directly from the People panel and revoke it any time. Keystrokes go only to the terminal that was granted and only while it is shared. Only roles with *Operator* or higher can receive control.
- The room reconnects automatically after network interruptions and restores watching, sharing and the voice call.

### Voice calls

Join the call with **🎙 Join voice** in the collaboration bar. It works like Discord or Jitsi:

- **Mute** (🎤, **Ctrl+Shift+M**), **deafen** (🎧, **Ctrl+Shift+D**), **leave** (📞). Moderators can mute someone for everyone.
- **Push-to-talk** with a key of your choice (e.g. F8), or voice activity.
- **Microphone and speaker selection**, live **input level meter**, test sound, **noise suppression**, **echo cancellation** and **automatic gain control** (*Settings → Voice & audio*, or ⚙ in the call).
- **Speaking indicators**, **per-person volume**, join/leave sounds, connection quality per person (good/fair/poor, P2P or relay, round-trip time).
- **Listen-only:** without a microphone (or permission) you can still join and listen.
- **Robust networking:** peer-to-peer WebRTC with automatic ICE restarts when the network changes, automatic rejoin after reconnects, and the **built-in TURN relay** (UDP and TCP) for networks where direct connections are blocked. External STUN/TURN servers (e.g. coturn on port 443/TLS for very strict networks) can be added in *Admin → Voice & network*.
- **Private:** audio is end-to-end encrypted (DTLS-SRTP); the relay cannot decrypt it.
- Browsers allow the microphone only on **HTTPS** pages (or `localhost`). Up to about 12 people per call work well (mesh); the limit is a policy.

### Audit log, session recording & file transfers

WRM keeps a complete, tamper-evident record of who did what, where and when:

- **Audit log** (*Admin panel → Audit log*): sign-ins (also failed), account and policy changes, connections, shares, room joins and moderation, terminal sessions, file transfers and changes, host keys, exports, viewing of recordings. Entries are linked to the **connection** and the **terminal session** (click **▶ #id** to open the session). Filter by text, event type, user and date range; **CSV export**; **Verify integrity**.
- **Append-only and tamper-evident:** the database refuses to change audit entries, file transfer records and recordings, and to delete anything younger than 7 days. Old entries are removed only by the retention job (`audit_retention_days`, `recording_retention_days`). Every entry contains the SHA-256 of the previous one (**hash chain**), so a changed or removed entry is detected by *Verify integrity*. Every entry is also written as an `AUDIT …` line to the server log (journald/syslog → SIEM).
- **Terminal sessions** (*Admin panel → Sessions & recordings*; every user sees their own in *Settings → Session history*): who, from which IP, which server and remote user, when, how long, how it ended (closed, connection lost, ended by an administrator, interrupted by a server restart).
- **Session recording:** the output of every SSH terminal is recorded in the open **asciinema** format (asciicast v2), compressed, with its SHA-256 stored in the database. **▶ Replay** in the browser — play/pause, seek, speed 0.5–16×, *skip idle time*; the replay window can be **resized** (drag the bottom-right corner) or shown in **fullscreen** (⛶, **Esc** leaves), and the terminal is scaled to the width so no sideways scrolling is needed — or **⬇ download** the `.cast` file and play it with `asciinema play`. The user sees *“This session is recorded”* and a **● REC** badge.
  - **No passwords in recordings:** servers do not echo passwords, so they never appear in the output. Keystrokes are recorded only when an administrator turns on *Record keystrokes*, and typing at password, passphrase and PIN prompts is masked.
  - Recording never slows the terminal (written in the background); a size limit per session stops recording (not the terminal).
  - **Who can see recordings:** administrators all; users their own sessions and sessions of other people on **their** connections (e.g. guests of a share). Viewing or downloading a recording is itself recorded in the audit log.
- **File transfers** (*Admin panel → File transfers*): every upload, download (including files opened in the editor and every file of a ZIP download) and server-to-server copy with source, destination, size, **SHA-256** and status; CSV export.

### File edit history

*Admin → File edits* (administrators and auditors) lists every file changed through WRM: saves from the text editor (also in shares) and changes the AI assistant made after approval. Each entry has the time, user, connection and server, path, source (*editor* or *AI*), session (the AI session or the share) and the size before and after; **View** shows the unified diff. Filters: search, user, server, connection, source, session and a time range; **CSV** exports the list (audited as `file_edit.exported`), and opening a diff is audited as `file_edit.viewed`. The audit entry of the save names the `file_edits` id.

The diff is stored **encrypted**, with secrets masked (`redactText`) before it is stored. Files larger than `file_edit_max_kb` (1024 KB) and binary files are recorded without a diff, with a note. The table is append-only like the audit log and kept for `audit_retention_days`. `file_edit_history` = `0` turns recording off.

### Settings

**⚙ Settings** in the top bar has tabs:

| Tab | What it contains |
|---|---|
| General | Language (English / Hrvatski), **install as an app**, accent color, close-window confirmation, **notifications when a server goes down**, browser-side upload limit |
| Notifications | Events × channels to receive outside the browser, your own recipient where the channel allows it, quiet hours, *Send test* |
| Terminal | Font size, scrollback, auto-reconnect |
| Security | Change password, two-factor authentication (enable/disable, new recovery codes), signed-in devices |
| Voice & audio | Microphone, speaker, level meter, noise suppression, echo cancellation, gain control, input mode / push-to-talk key, call sounds |
| Session history | Your terminal and remote desktop sessions and sessions on your connections, with replay and download |
| AI connections | The MCP address, your AI connections (revoke), a new token with servers, mode, scopes and lifetime, and setup snippets for Claude Desktop, Claude and ChatGPT connectors and other MCP clients (when MCP is on) |
| Data | Export connections (without secrets), export **with** passwords & keys (asks for your password; policy), import (also jump hosts and tunnels), **import from mRemoteNG**, **PuTTY** and **OpenSSH config** |

Personal preferences are stored per browser; everything security-related is stored on the server.

### Admin panel

*Settings → 🛡 Admin panel* (administrators only):

- **Overview:** version, uptime, users (admins, 2FA), HTTPS, voice relay status, encryption key source, **security warnings** (no HTTPS, open registration, admins without 2FA, host keys off…), **open terminals** (who, which server, from which IP — with *End*), **active SSH tunnels** (owner, connection and route, listen → target, traffic — with *Stop*), **live rooms** (participants, voice, shared terminals).
- **Users:** create users with a role (temporary password generated if you leave it empty), display name, change the role (administrator, security auditor, operator, user), **break-glass** on/off for administrators, **reset password**, **reset 2FA**, **sign out everywhere**, unlock, **disable/enable**, delete (with everything the user owns, or handed over to another account). The last active administrator cannot be removed.
- **Teams:** create teams, add members and mark team leads; see which policies and shares use a team.
- **Shares:** every share on the server — pause/resume or delete.
- **Security:** the security level (Home / Business / Enterprise), its preview, deviations and the security posture report (see [Security levels and posture](#security-levels-and-posture)).
- **Security policies:** registration, required 2FA, password length, lockout, session idle/maximum time, host-key policy, server key files, server-side upload limit, secret export, guest links, chat file size, audit log on/off and retention, session recording (on/off, keystrokes, retention, size limit), **broadcast input**, **live status** (on/off, interval, checks through jump hosts), **SSH tunnels** (on/off, who may use them, listening on network addresses, remote forwarding, idle stop), **proxies** (who may define them).
- **Notifications:** notification channels (e-mail, Telegram, Slack / Mattermost / Rocket.Chat, Teams, Discord, ntfy, Gotify, Pushover, webhook) with *Send test*, last delivery and error; notifications on/off and the minimum interval.
- **Voice & network:** voice on/off, participants per call, built-in TURN relay (port, public IP, host name, relay ports, private networks), additional STUN/TURN servers.
- **Host keys:** remembered SSH host keys and FTPS certificates; forget an entry after a server was reinstalled.
- **Audit log:** searchable and filterable (event type, user, date range), linked to sessions, **CSV export**, **Verify integrity** (hash chain).
- **AI assistant:** organisation providers, AI sessions with transcripts, replay and *Kill*, usage per user (turn the assistant off for a user), the kill switch and the AI policies.
- **AI connections:** AI apps connected over MCP (revoke one or all), registered OAuth apps and the MCP policies.
- **Sessions & recordings:** every terminal session, with replay and `.cast` download.
- **File transfers:** every transferred file with size and SHA-256, CSV export.

Any policy can also be **forced by an environment variable** (`WRM_<KEY>`), e.g. for configuration management; it is then shown locked in the admin panel.

### Security levels and posture

A **security level** sets minimums for the settings that matter. Pick it on the first-run screen (the first administrator), at install with `wrm -security-level home|business|enterprise` (kept by `-install-service`) or `WRM_INITIAL_SECURITY_LEVEL` (applied once, while no level was chosen), force it with `WRM_SECURITY_LEVEL`, or change it later in *Admin panel → Security*.

| Setting | Home | Business | Enterprise |
|---|---|---|---|
| `require_https` (sign-in only over HTTPS or from localhost) | – | on | on |
| `require_2fa` | (recommended: admins) | all | all |
| `password_min_length` | – | 12 | 14 |
| `session_idle_hours` / `session_max_days` | – | 12 / 7 | 8 / 1 |
| `login_max_failures` | – | 5 | 5 |
| `registration` | (recommended: closed) | closed | closed |
| `host_key_policy` | (recommended: tofu) | tofu | strict |
| `allow_server_keys`, `allow_secret_export`, `ai_full_access` | – | off | off |
| `allow_link_shares` | – | – | off |
| `link_share_max_days` | – | 30 | 7 |
| `audit_enabled` / `audit_retention_days` | (recommended: on) | on / 365 | on / 730 |
| `session_recording` / `recording_retention_days` | – | on / – | on / 365 |

- **Home** is WRM's previous behaviour; existing installations start there with nothing changed.
- A higher level **raises** weaker settings and leaves stronger ones as they are. Before applying, *Preview* lists every change, settings an environment variable keeps, what blocks the change (HTTPS while this browser uses plain HTTP) and whom it affects: users who must set up 2FA at their next sign-in, sessions that end, guest links that are too long or blocked, passwords checked at the next change. *Apply* can also sign out every other session.
- **Deviations:** a setting may stay below the level only with a written reason (in the preview, or when you change a policy). The *Security* tab lists each deviation with who recorded it and when; it is audited (`security.deviation`, `security.deviation_removed`) and removed once the setting meets the level again. Choosing a level over plain HTTP on the first-run screen records the HTTPS deviation by itself, so you can turn HTTPS on afterwards.
- **Security posture:** the same tab grades the installation item by item (✅ / ⚠ / ⛔): HTTPS for this browser, every rule of the level, administrators and users without 2FA, where the encryption key lives (next to the database or separate), `WRM_ALLOW_ANY_ORIGIN`, guest links without expiry, signed updates, the current version and the audit hash chain. Every item says why it matters and how to fix it. *Export JSON* and *Print / PDF* produce a report for auditors.

### Roles, teams, step-up and break-glass

**Roles.** Every account has one role, set when it is created or later in *Admin panel → Users → ⋯*:

| Role | What it can do |
|---|---|
| **Administrator** | Everything, as before. |
| **Security auditor** | Reads the audit log, terminal sessions and recordings, file transfers and the security posture in the admin panel. Cannot change anything, open connections, use the AI assistant or AI connections. Changing someone to auditor ends their terminals and AI sessions. |
| **Operator** | A user who also passes policies set to `operators` (for example tunnels or Git updates for the operations staff only). |
| **User** | As before. |

**Teams.** *Admin panel → Teams* creates teams with members and optional **leads** (★). Teams are used in three places:
- **Shares:** in the share dialog (*Members only* mode) add a team with a role; every member gets that role, and people added to the team later get it too. Removing someone from the team closes what they opened through the share at once. A person in several teams gets the highest role; a direct member role that is higher wins.
- **Policies:** every *who may use it* policy (tunnels, remote forwarding, serial ports, network tools, proxies, Git checks/updates/upgrades/rollbacks/restarts/installs/Medium/transfers/merge requests, Bitwarden, AI assistant, personal AI keys, OpenCode, the OpenCode sandbox and AI connections) accepts `operators` (administrators and operators) and `teams:<name>,<name>` (administrators and members of those teams) besides `off`, `admins` and `all`. In *Policies* choose *Choose teams…*. A team named in a policy cannot be deleted or renamed until the policy changes.
- **Handover:** the lead of a leaving user's team is the suggested new owner (see below).

**Step-up confirmation.** With `require_step_up` on (required by the Business and Enterprise levels), these actions ask for your password, and your 2FA or a recovery code when the account has 2FA: changing policies, changing the security level, creating administrators or auditors, changing a role, disabling, deleting or turning break-glass on for a user, resetting a password or 2FA, revealing a stored credential password, exporting a private key and exporting connections with secrets. The confirmation lasts `step_up_minutes` (default 10) in that browser session; signing in counts as one. Wrong passwords count toward the sign-in rate limit, and every confirmation is audited (`auth.step_up`, `auth.step_up_failed`).

**Break-glass administrator.** Mark a local administrator account as break-glass (*Admin panel → Users → ⋯*) for emergencies, for example when the directory sign-in of a later version is down. It must use 2FA (its sessions end until 2FA is set up), it can be limited to `break_glass_ips` (IP addresses or networks such as `10.0.0.0/24`), and every sign-in is audited (`auth.break_glass_login`) and sent to all other administrators as a notification.

**Handover when deleting a user.** *Delete* shows what the user owns (connections, folders, credentials, SSH keys, shares, proxies) and asks what happens to it: delete it with the account, or hand it to another active account (the team lead is suggested, administrators are listed). The receiver becomes the owner; shares keep their members and teams, so people keep their access. A folder whose name the receiver already uses gets the old owner's name added. The handover is audited (`admin.user_handover`) with the counts.

### Directory sign-in (OIDC)

Companies sign in with their directory instead of WRM passwords. *Admin panel → Directory sign-in*:

1. **Add a provider:** choose the kind (Microsoft Entra ID, ADFS, Okta, Keycloak, Google Workspace, other OpenID Connect); the editor shows where to create the client at that provider. Register the **redirect address** shown (`https://<WRM address>/api/auth/oidc/callback`; behind a reverse proxy set it in the provider, or set `ai_mcp_public_url`). Enter the **issuer** (Entra: `https://login.microsoftonline.com/<tenant ID>/v2.0`, your own tenant, not `common`), the **client ID** and the **client secret** (stored encrypted, never shown again).
2. **Test connection** checks step by step: the issuer address, the discovery document, the endpoints, the signing keys, PKCE and whether WRM can check users later (`offline_access`). A failed step says what to fix.
3. **Rules** decide who may sign in: each rule names a claim (`roles` for Entra app roles, `groups` for group IDs, `hd` for a Google Workspace domain, or any other claim), a value, a WRM role and optional teams. The strongest matched role wins (*Administrator* > *Security auditor* > *Operator* > *User*); teams of every matched rule are added. Values are compared without case. Without rules, only WRM accounts that were linked by hand may sign in.
4. **Create accounts on first sign-in** (on by default) makes an account for a person a rule matches. The username comes from `preferred_username` (or the claim you choose); a name already taken gets a number added. Such an account has no password (*signs in only through the directory*).
5. **Test sign-in** opens the provider in a new window and shows what it sent (claims), which account would be used or created, the role and teams, whether MFA was proven and whether a refresh token came back. Nothing is created.
6. **Enable** the provider: the sign-in page shows *Sign in with <name>*.

How it behaves:
- **Identity:** a person is the directory's immutable ID: Entra's tenant and object ID (`tid` + `oid`), otherwise issuer + subject (`iss` + `sub`). E-mail addresses and names are never used to find an account, so renaming someone or re-using a name in the directory does not give anyone else's account.
- **Every sign-in follows the directory:** the role and the teams added by rules (`team_members.source = 'oidc'`) are updated; teams an administrator added by hand stay. A person no rule matches any more is refused and their open sessions end. The last active administrator is never demoted by a sign-in (audited `oidc.role_kept`). Break-glass accounts stay local and cannot be linked.
- **MFA:** with *Require MFA proven by the provider* the ID token must show a second factor (`amr` such as `mfa`, `otp`, `hwk`) or one of the listed values in `acr` or Entra's `acrs` (authentication context, requested through the `claims` parameter; for other providers through `acr_values`). Microsoft Entra v2 tokens carry no `amr`: enforce MFA there with Conditional Access and an authentication context (for example `c1`). A provider that proves MFA satisfies `require_2fa`; otherwise an account that uses WRM's own 2FA enters its code after the directory sign-in, and `require_2fa` asks other accounts to set it up.
- **Regular check:** every `oidc_revalidate_minutes` WRM uses the refresh token to ask the provider whether each signed-in person may still sign in. A refused refresh (account disabled or deleted, password reset, consent revoked) or a refreshed token that no longer matches a rule ends the person's sessions, terminals, tunnels, rooms and AI sessions (audited `oidc.access_ended`). Without a refresh token, or while the provider cannot be reached, directory sessions end after `oidc_unverified_hours` without a successful check. The list of linked accounts shows the last check and its error.
- **Step-up:** accounts without a password confirm dangerous actions through the provider (it is asked to sign the person in again); accounts with a password can use either.
- **Linking existing accounts:** in *Settings → Security → Organisation accounts* a signed-in user connects their directory account (the provider window must confirm the same person); afterwards they can sign in either way. An administrator can unlink accounts in the list; unlinking ends the directory sessions.
- **Password sign-in only for emergencies:** `local_login` = `break_glass` leaves the password form only to break-glass administrators (folded behind *Emergency sign-in with a local account*) and closes self-registration. It takes effect only while a provider is enabled. The Enterprise level requires it.
- Audited: `oidc.provider_saved`, `oidc.provider_deleted`, `oidc.account_created`, `oidc.identity_linked`, `oidc.identity_unlinked`, `oidc.role_synced`, `oidc.access_ended`, `oidc.test_sign_in`, `auth.oidc_refused` (with the reason) and `auth.login` with `method: oidc`.

### LDAP / Active Directory and SCIM

**LDAP / Active Directory sign-in.** *Admin panel → Directory sign-in → LDAP / Active Directory → Add directory*:

1. **Kind:** *Active Directory* or *other LDAP* (OpenLDAP, 389 Directory Server, FreeIPA and similar). The kind fills in the user filter, the sign-in attribute (`sAMAccountName` / `uid`) and the ID attribute (`objectGUID` / `entryUUID`); change them under *Advanced* if your directory differs.
2. **Server:** `ldaps://dc1.example.com` (port 636) or `ldap://dc1.example.com` (port 389, StartTLS is then mandatory). Plain LDAP without TLS is not possible. When the directory's certificate comes from an internal CA that the WRM host does not trust, paste that CA certificate (PEM).
3. **Service account:** the DN (for example `CN=svc-wrm,OU=Service,DC=example,DC=com`) and password of an account that may read users and groups. The password is stored encrypted and never shown again; changing the server address asks for it again.
4. **Base DN** where users are searched, for example `DC=example,DC=com`.
5. **Test connection** checks step by step: the address, the connection, TLS and the certificate, the service account, the base DN, whether users are found, and for a test name the account, its immutable ID and its groups. A failed step says what to fix.
6. **Rules** decide who may sign in: `group` matches a group name or DN (nested groups included), any other claim matches a user attribute (for example `department`). The strongest matched role wins; teams of every matched rule are added.
7. **Create accounts on first sign-in** makes an account for a person a rule matches, named after the directory's sign-in name. **Test sign-in** checks a name and password and shows the account, groups, attributes and what WRM would do, without creating anything.
8. **Enable** the directory. It can only be enabled while the service account works.

How it behaves:
- **Signing in:** on the normal sign-in page with `name`, `DOMAIN\name` or `name@domain` and the directory password. WRM finds the person with the service account and checks the password by binding as them; the password is never stored, and an empty one is refused (servers would treat it as an anonymous bind). Failed directory sign-ins count towards the same lockout as local ones.
- **Identity:** the person is the directory's immutable ID (`objectGUID`, or the chosen ID attribute), never the name.
- **Refused accounts:** disabled (`userAccountControl`), locked, expired (`accountExpires`) and, on other LDAP servers, `nsAccountLock`, `loginDisabled` or `pwdAccountLockedTime` accounts cannot sign in.
- **Regular check:** every `ldap_sync_minutes` (default 15) WRM looks up every linked account with the service account. A deleted, disabled, locked or expired account, or one no rule matches any more, loses its sessions, terminals, tunnels, rooms, AI sessions and AI connections at once (audited `ldap.access_ended`); role and team changes are applied (`team_members.source = 'ldap'`). *Check now* runs it on demand. The directory card shows the last check and a ⚠ when it failed.
- **Linking existing accounts:** in *Settings → Security → Organisation accounts* a signed-in user links their directory account with its name and password (WRM asks for a fresh confirmation first). From then on the account signs in with its directory password; WRM no longer keeps a local password for it. An administrator can unlink accounts in the list; the user then needs a password reset to sign in locally.
- **Directory passwords in WRM:** such accounts cannot change their password in WRM; step-up and 2FA changes are confirmed with the directory password.
- `local_login` = `break_glass` also applies while a directory is enabled: only break-glass administrators keep a local password.
- Audited: `ldap.directory_created`, `ldap.directory_updated`, `ldap.directory_deleted`, `ldap.account_created`, `ldap.identity_linked`, `ldap.identity_unlinked`, `ldap.synced`, `ldap.access_ended`, `ldap.test_sign_in`, and `auth.login` / `auth.login_failed` with `method: ldap` (refusals with the reason).

**SCIM provisioning.** With SCIM 2.0 an identity provider (Microsoft Entra ID, Okta, …) creates, changes, disables and deletes WRM accounts and groups at once. *Admin panel → Directory sign-in → SCIM provisioning → Add provisioning client*:

1. Name the client and optionally add rules that turn group names into roles (`group` = `WRM Admins` → Administrator).
2. Copy the **token** (`wrm_scim_…`); it is shown only once and stored only as a hash. *New token* replaces it, *Revoke* stops the client.
3. In the identity provider set the **tenant URL** to the endpoint shown (`https://<WRM address>/scim/v2`) and the **secret token** to the copied token. In Entra ID: *Enterprise application → Provisioning → Automatic*. Map `externalId` to `objectId` so the person's first sign-in through [directory sign-in (OIDC)](#directory-sign-in-oidc) uses the provisioned account.

How it behaves:
- A client sees only the accounts and groups it created; local accounts and other clients' accounts are invisible to it, and names never link accounts.
- Provisioned accounts have no password; they sign in through the directory. The first OIDC sign-in links the account when its `externalId` equals the token's `oid` (Entra) or `sub`, and the account has no directory link yet.
- `active: false` or deleting the user disables the account and ends its sessions, terminals, tunnels, rooms, AI sessions and AI connections; WRM keeps the account so an administrator can hand its work over. The last active administrator is never disabled or demoted (`scim.disable_refused`).
- Groups become WRM teams named `<group> (SCIM)`; membership follows the group. A team made by hand is never taken over.
- The identity provider is the source of truth for the accounts it provisions: `active: true` re-enables such an account even when an administrator disabled it in WRM. To keep someone out, disable or unassign them in the identity provider.
- Supported: Users and Groups (create, read, replace, PATCH, delete), `filter=userName eq "…"`, `externalId eq "…"` or `id eq "…"` (groups: `displayName`, `externalId`, `id`), `startIndex` / `count`, `ServiceProviderConfig`, `ResourceTypes`, `Schemas`. Limits: 600 requests a minute per address, 1 MB per request.
- Audited: `scim.client_created`, `scim.client_updated`, `scim.token_rotated`, `scim.client_revoked`, `scim.user_created`, `scim.user_updated`, `scim.user_disabled`, `scim.user_deleted`, `scim.group_created`, `scim.group_renamed`, `scim.group_deleted`, `scim.team_created`.

**Ending access.** Every route that stops an account (an administrator disabling it, OIDC revalidation, the LDAP check, SCIM) ends the same things: sign-in sessions, terminals, tunnels, collaboration rooms, AI sessions and AI connections (MCP tokens).

### SIEM export, alerts and lockdown

Since v12.6.26 (`remote-manager/siem.go`, `alerts.go`, `lockdown.go`), in *Admin panel → Security*.

**SIEM export.** *SIEM export → Add destination*, choose the kind, fill in its fields and save (step-up). *Test* sends one `siem.test` event and shows the answer.

| Kind | Address | Secret | Notes |
|---|---|---|---|
| Syslog (RFC 5424 over TLS) | `host:port` (usually 6514) | private key of the client certificate (optional) | RFC 5425 octet counting; structured data `wrm@32473` with the main fields, the whole event as JSON in the message; facility `authpriv` (10) unless set; severity notice, warning for failures, critical for `security.alert` |
| JSON lines file | absolute path on the WRM host ending in `.jsonl`, `.json` or `.log` | – | one event per line, mode 0600, rotated at 100 MB keeping `.1`–`.3`; the folder must exist and the file must not be a symbolic link |
| Splunk HEC | the HEC address (`https://splunk.example.com:8088`) or the full `…/services/collector/event` | HEC token | `sourcetype` (default `wrm:audit`) and `index` optional |
| Elastic | the Elasticsearch / OpenSearch address | API key (optional) | `_bulk` into the index (default `wrm-audit`); item errors count as a failed delivery |
| Microsoft Sentinel | the data collection endpoint (`https://<dce>.<region>.ingest.monitor.azure.com`) | client secret of the Entra application | tenant ID, application (client) ID, the data collection rule's immutable ID and the stream (default `Custom-WRMAudit_CL`); the application needs *Monitoring Metrics Publisher* on the rule |
| Generic HTTPS | any HTTPS URL | token (optional) | POST of a JSON array of events; with a token WRM sends `Authorization: Bearer <token>` and `X-WRM-Signature: sha256=<HMAC-SHA256 of the body with the token>` |

HTTPS addresses must use HTTPS (plain HTTP only on loopback); an extra CA certificate (PEM) can be added for internal collectors. Redirects are not followed.

How it behaves:
- **Every event, in order, at least once.** Each destination keeps its own position in the audit log and sends up to 200 events at a time. A failed delivery is retried after 5 s, 10 s, 20 s … up to 5 minutes; nothing is lost while WRM keeps the audit log (`audit_retention_days`). A new destination starts with new events or, when chosen, with the whole kept audit log. *Send only these actions* limits a destination to action prefixes (for example `auth., admin., security.`).
- **Event fields:** `@timestamp`, `event` (`id`, `action`, `category`, `outcome`, `kind`), `observer` (product, version, host name), `hash` (`prev`, `this`: the audit chain, so the SIEM can prove nothing was removed), `user` (`id`, `name`, `roles`, `email`, `directory` with the provider and immutable ID for directory accounts), `source.ip`, `target`, `details`, `connection_id`, `session_id`.
- **Status:** each destination shows *Delivering*, *Behind* (more than 1000 events waiting), *Failing* (with the last error, its time and the next try) or *Off*, with the sent and waiting counts and the last delivery.
- **Secrets** are encrypted at rest and never sent to the browser. Changing a destination's address or kind asks for the secret again, so a stored secret never goes to another server.
- Audited: `siem.destination_created`, `siem.destination_updated`, `siem.destination_deleted`, `siem.tested`, `siem.test_failed`.

**Security alerts.** Rules watch the audit log. A matching rule writes a `security.alert` entry (rule, summary, the action, user and address that caused it), which reaches the SIEM, and tells every active administrator in the Workboard inbox and through the `security.alert` notification. *Admin panel → Security* lists the recent alerts; *Policies → Security alerts* holds the rules:

| Rule | Setting | Raised when |
|---|---|---|
| Failed sign-ins | `alert_failed_signins` (10, 0 = off), `alert_failed_signins_minutes` (10) | that many failed sign-ins (password, 2FA, step-up, AI tokens) from one address or for one name within the window; once per window and key |
| Break-glass sign-in | `alert_break_glass` | a break-glass administrator signed in (SIEM only: administrators are already notified with `security.break_glass_login`) |
| New administrator | `alert_new_admin` | an account was created as, changed to, or synced from a directory as an administrator |
| Secret revealed | `alert_secret_reveal` | a stored password was shown, a private SSH key or connections with secrets were exported |
| AI full access | `alert_ai_full_access` | someone approved an AI command that WRM refuses without full access |
| Audit off | `alert_audit_off` | `audit_enabled` was turned off, or a SIEM destination was turned off or deleted |

Security events (`security.*`) are recorded even with `audit_enabled` off.

**Stop user.** In *Admin panel → Users* → ⋯ → *Stop user* (step-up): the account is disabled and its sign-in sessions, terminals, shared sessions, tunnels, AI sessions and AI connections end at once; WRM then shows how many of each were ended. Enable the account again when it is safe. Not possible for yourself or the last administrator. Audited `admin.user_stopped` with what was ended.

**Lockdown.** For an incident: only break-glass administrators can sign in or keep a session.
- **Turning it on:** *Admin panel → Security → Lockdown → Lock down WRM* with a reason everyone sees (a fresh confirmation is always asked). It is refused while there is no break-glass administrator, because nobody could sign in. On the WRM host: `wrm -lockdown on -lockdown-reason "incident 42"` (also `off` and `status`); a running WRM applies it within five seconds. `WRM_LOCKDOWN=1` forces it at start and cannot be ended in the UI.
- **What happens:** everyone except break-glass administrators is signed out, their terminals, shared sessions and tunnels end; all AI sessions and AI connections end and AI is refused as with the AI kill switch; shares are refused. The response lists what was ended. Administrators who are not break-glass are signed out too.
- **During the lockdown** the sign-in page shows it with the reason, and only break-glass administrators can sign in (password and 2FA; directory sign-in is refused).
- **Ending it:** a break-glass administrator presses *End lockdown*, or `wrm -lockdown off` on the host. Accounts, shares and AI settings are as before; people sign in again.
- Audited: `security.lockdown_on`, `security.lockdown_off`, `security.lockdown_enforced` (applied at start or from the command line), and refused sign-ins with the reason `lockdown`.

### Updates & self-update

- **Update check:** once a day (and with *Check now*) WRM asks the GitHub releases API of `update_repo` (default this project) for the latest release. Only the repository path and a fixed `User-Agent` are sent: no user data, no installation ID. A proxy comes from `update_proxy` or `HTTPS_PROXY` / `HTTP_PROXY`. Air-gapped installations turn it off with `update_check` (`WRM_UPDATE_CHECK=off`).
- **Version badge:** when a newer release exists, the top bar shows **⬆ Update vX.Y.Z** (only ⬆ on narrow screens) and the sign-in page shows *update available vX.Y.Z*. The badge opens the release notes and the release page.
- **One-click update** (administrators, policy `self_update`):
  1. downloads `wrm-pro-vX.Y.Z-<os>-<arch>[.exe]` of the release next to the binary (with progress);
  2. verifies the **signature `SHA256SUMS.txt.sig`** (ed25519) of the release's `SHA256SUMS.txt` with the release key built into WRM (plus `update_signing_keys`, for example the key of your own fork), then the file's **SHA-256 against `SHA256SUMS.txt`**. It refuses the update when the signature is missing or wrong (once a key is known), or when the checksums file, its line or the hash does not match. Without any known key the dialog shows *Signature: not checked* and only the SHA-256 is verified;
  3. runs the new file with `-version`, which must answer the release's version;
  4. replaces the binary **atomically**: a rename over the old file (Linux, macOS, BSD), or on Windows a rename of the running `.exe` to `.exe.old` and the new file in its place (the `.old` file becomes the rollback copy on the next start);
  5. restarts: a service through its service manager (exit code 75: systemd, launchd, FreeBSD `daemon(8)`, Windows recovery), otherwise WRM starts itself again in place. The page waits and reconnects by itself.
  - The previous binary is kept as `<binary>.previous`: **↩ Roll back to vX.Y.Z** swaps it back and restarts.
  - Every step is audited: `system.update_check`, `system.update_started`, `system.update_downloaded`, `system.update_verified`, `system.update_installed`, `system.update_failed`, `system.update_restart`, `system.update_rollback`.
  - The database is never touched by the update itself; the new version runs its usual additive migrations, so the previous binary still starts on it.
  - The binary's folder must be writable by WRM; otherwise the dialog says so.
- **Newest binary in the folder:** copy a newer `wrm-pro-v<semver>-<os>-<arch>[.exe]` next to the running binary. A service switches to the newest valid one when it starts (it must answer `-version` with the version in its name; other platforms, older versions and files that do not answer are ignored). The update dialog offers **↻ Restart to vX.Y.Z** for it.
- **Docker:** a container never replaces itself. The badge and the release notes are shown; the dialog lists `docker pull ghcr.io/<owner>/wrm-pro:vX.Y.Z` and the compose commands instead.

### Mobile, responsive UI & installable app

- The top bar and the collaboration bar adapt to the available width (labels collapse into icons, step by step).
- **Installable app (PWA):** *Settings → General → Install as an app* (or the install icon in the address bar; on iPhone/iPad: *Share → Add to Home Screen*) opens WRM in its own window, from the dock, start menu or home screen. The app caches only its own scripts and fonts (per version); connections and data always come live from the server, and without the server it shows an offline page. Browsers allow this only over **HTTPS with a trusted certificate** (or on `localhost`). The *Install as an app* row shows the steps for the current browser, or why installing is not possible (plain `http://`, an untrusted certificate, Firefox on the desktop); the *Install* button appears only when the browser can install right away.
- On phones (< 760 px): the connection list and the Sessions/Snippets/Clipboard panel become **slide-in drawers** (in the fixed start layout, whatever you arranged on a larger screen) (☰ and 🗂 buttons), windows open **full screen**, and a single tap opens a connection. Chat and People open as full-width panels.

---

## Keyboard shortcuts

| Where | Shortcut | Action |
|---|---|---|
| Anywhere | **Alt+N** | New window |
| Anywhere | **Esc** | Close menus / dialogs / panels |
| Voice call | **Ctrl+Shift+M** / **Ctrl+Shift+D** | Mute / deafen (also while a terminal has focus) |
| Voice call | your push-to-talk key (e.g. **F8**) | Hold to talk |
| Terminal | **Ctrl+Shift+F** | Search in terminal output |
| Terminal | **Ctrl+Shift+Space** | Snippet picker (Enter run, Shift+Enter insert) |
| Snippets panel | **Click** / **Shift+click** | Run in the focused terminal / insert without Enter |
| Terminal | **Ctrl+Shift+C** / select text | Copy |
| Terminal | **Ctrl+Shift+R** | Reconnect SSH |
| Terminal | **Enter** (when closed) | Reconnect |
| Terminal | **Ctrl+W** | Sent to the terminal (browser tab is not closed) |
| File manager | **Enter / Backspace** | Open / go up |
| File manager | **↑ ↓ Home End**, **Shift** | Move / extend selection |
| File manager | **Ctrl+A**, **Delete**, **F2**, **F5** | Select all, delete, rename, refresh |
| File manager | **Ctrl+F**, **Enter** in search | Filter, search subfolders |
| Editor | **Ctrl+S**, **Tab** | Save, insert tab |
| Window title | **Double-click** | Maximize / restore |
| Tab | **Middle-click**, **right-click** | Close, tab menu |

---

## Configuration

Medium runtime settings (introduced in v12.4.3b):

| Setting / environment override | Default | Purpose |
|---|---|---|
| `git_medium` / `WRM_GIT_MEDIUM` | `admins` | `off`, `admins`, `all`; trusted original-tool operators. Tool code imports always require administrator access. |
| `medium_host_ca` / `WRM_MEDIUM_HOST_CA` | `1` | Medium's Docker runtime also trusts the root certificates the WRM host trusts (needed behind proxies or antivirus that inspect HTTPS). |
| `medium_ca_file` / `WRM_MEDIUM_CA_FILE` | – | Path to a PEM file with extra root certificates for Medium's Docker runtime, e.g. a proxy's root CA. |
| `medium_docker_install` / `WRM_MEDIUM_DOCKER_INSTALL` | `1` | WRM may install and start Docker for Medium on its own host after an administrator allows it once in Medium step 2 (accepting Docker Desktop's licence on Windows and macOS). `0` hides **Install Docker** and stops automatic installation. |
| `medium_native` / `WRM_MEDIUM_NATIVE` | `admins` | Who may switch Medium projects to the WRM native engine and deploy, roll back and release locks with it (`off`, `admins`, `operators`, `teams:<names>`, `all`). Comparisons and dry runs only need `git_medium`. |
| `medium_default_engine` / `WRM_MEDIUM_DEFAULT_ENGINE` | `native` | Engine of Medium projects nobody chose one for: `native` (WRM deploys them for users allowed by `medium_native`; the original tool is optional) or `original`. Upgraded installations with the original tool imported start with `original`. |
| `medium_build_user` / `WRM_MEDIUM_BUILD_USER` | – | Unprivileged OS account that runs Medium builds on the WRM host (WRM must run as root on Linux/Unix). Empty: builds run with the WRM account. |
| `medium_command_timeout` / `WRM_MEDIUM_COMMAND_TIMEOUT` | `300` | Seconds one command of a native deployment may run on a server, such as `post_deploy` (30–86400). |
| `medium_transfer_timeout` / `WRM_MEDIUM_TRANSFER_TIMEOUT` | `3600` | Seconds the file transfer of one destination may take (600–86400). With the command timeout it decides when a lock left behind may be released by hand. |
| `medium_log_output` / `WRM_MEDIUM_LOG_OUTPUT` | `all` | Who sees command output in the Medium activity log: `all` (the user who ran it, administrators, everyone for the shared runtime), `admins`, `none`. Steps, results and errors are always shown. |
| `bitwarden` / `WRM_BITWARDEN` | `all` | `off`, `admins`, `all`; access to each user's own vault. |
| `bitwarden_idle_minutes` / `WRM_BITWARDEN_IDLE_MINUTES` | `15` | Unlock inactivity, 1–240 minutes. |
| `WRM_BITWARDEN_CLI` | `bw` on PATH | Official CLI executable override. |
| `WRM_MEDIUM_RUNTIME` | `auto` | `auto` uses Docker on Linux, macOS and Windows (a host runtime prepared earlier stays until the Docker runtime is prepared) and the host runtime elsewhere; `docker` or `native` choose explicitly. Windows always uses Docker. |
| `WRM_MEDIUM_DOCKER` | Docker on PATH/standard Desktop paths | CLI override, without shell arguments. |
| `WRM_MEDIUM_CONTAINER_IMAGE` | Pinned published WRM Linux image | Trusted custom runtime image; source is still imported privately. |
| `WRM_MEDIUM_PYTHON` | Python on PATH | Python 3.12+ executable for the private venv. |
| `WRM_MEDIUM_SSH_AUTH_SOCK` | unset | Optional SSH agent socket, restricted to users allowed server-side keys. |


**Environment variables**

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Listening port |
| `LISTEN_ADDR` | `:PORT` | Full listen address, e.g. `127.0.0.1:8080` (overrides `PORT`) |
| `DB_PATH` | `./remote_manager.db` | SQLite database file |
| `ENCRYPTION_KEY` | – | Your own key for stored secrets (first 32 bytes are used; same derivation as v9). |
| `ENCRYPTION_KEY_FILE` | – | Read the key from a file (64 hex characters = raw 256-bit key, anything else is hashed). |
| *(neither set)* | `<DB_PATH>.key` | A random key is generated there on first start (mode `0600`). |
| `HTTPS_CERT_FILE` + `HTTPS_KEY_FILE` | – | Serve HTTPS with this certificate and key (HSTS is sent). |
| `HTTPS_SELF_SIGNED` | – | `1` = create and use a self-signed certificate (`wrm-selfsigned.crt/.key` next to the database). Browsers warn once; good for LANs and voice calls without a CA. |
| `HTTPS_SELF_SIGNED_HOSTS` | – | Extra host names/IPs for the self-signed certificate, comma-separated. |
| `WRM_RECORDINGS_DIR` | `<DB dir>/recordings` | Where session recordings are stored |
| `WRM_TRUST_PROXY` | – | `1` = trust `X-Real-IP` / `X-Forwarded-For` / `X-Forwarded-Proto` / `X-Forwarded-Host` from your reverse proxy (client IPs in rate limits and audit, secure cookies). Only when WRM is reachable **only** through that proxy. |
| `WRM_ALLOWED_ORIGINS` | – | Extra hostnames allowed as `Origin` (comma-separated), e.g. when a proxy rewrites the `Host` header |
| `WRM_ALLOW_ANY_ORIGIN` | – | `1` disables the origin check (not recommended) |
| `WRM_LOCKDOWN` | – | `1` forces a lockdown at start: only break-glass administrators can sign in, AI off, shares paused; it cannot be ended in the UI |
| `WRM_INITIAL_SECURITY_LEVEL` | – | `home` / `business` / `enterprise`: applied once at start while no security level was chosen (for installers); `WRM_SECURITY_LEVEL` forces and locks it |
| `WRM_<POLICY>` | – | Forces a policy from the admin panel, e.g. `WRM_REGISTRATION=open`, `WRM_REQUIRE_2FA=all`, `WRM_TURN_PUBLIC_IP=203.0.113.10`, `WRM_TURN_ENABLED=0`, `WRM_HOST_KEY_POLICY=strict`. |

**Policies** (*Admin panel*, or `WRM_<KEY>` in upper case)

| Key | Default | Meaning |
|---|---|---|
| `security_level` | `home` | `home` / `business` / `enterprise`, see [Security levels and posture](#security-levels-and-posture); changed in *Admin panel → Security*, not here |
| `require_https` | `0` | sign-in, registration and 2FA only over HTTPS (or from localhost) |
| `require_step_up` | `0` | dangerous actions need a fresh password (and 2FA) confirmation, see [Roles, teams, step-up and break-glass](#roles-teams-step-up-and-break-glass); required by the Business and Enterprise levels |
| `step_up_minutes` | `10` | how long a step-up confirmation lasts (1–240) |
| `break_glass_ips` | – | IP addresses or networks (comma-separated) a break-glass administrator may sign in from; empty = anywhere |
| `local_login` | `all` | `all` / `break_glass`: who may still sign in with a password while a directory sign-in is enabled, see [Directory sign-in (OIDC)](#directory-sign-in-oidc); required `break_glass` by the Enterprise level |
| `oidc_revalidate_minutes` | `60` | how often WRM checks with the identity provider that signed-in directory users may still sign in (5–1440) |
| `oidc_unverified_hours` | `12` | directory sessions end after this long without a successful check (1–720) |
| `alert_failed_signins` | `10` | failed sign-ins from one address or for one name that raise a security alert (0 = off), see [SIEM export, alerts and lockdown](#siem-export-alerts-and-lockdown) |
| `alert_failed_signins_minutes` | `10` | the window in which failed sign-ins are counted (1–1440) |
| `alert_break_glass` / `alert_new_admin` / `alert_secret_reveal` / `alert_ai_full_access` / `alert_audit_off` | `1` | the other security alert rules |
| `ldap_sync_minutes` | `15` | how often WRM checks every account linked to an LDAP / Active Directory directory (5–1440), see [LDAP / Active Directory and SCIM](#ldap--active-directory-and-scim) |
| `registration` | `closed` | `closed` / `open` self-registration |
| `require_2fa` | `off` | `off` / `admins` / `all` |
| `password_min_length` | `8` | 6–128 |
| `login_max_failures` | `10` | account lock (15 min) after this many wrong passwords |
| `session_idle_hours` | `168` | sign out after inactivity |
| `session_max_days` | `30` | maximum session age |
| `host_key_policy` | `tofu` | `tofu` / `strict` / `off` |
| `allow_server_keys` | `0` | allow *Key file* / *Auto* for non-admins |
| `max_upload_mb` | `0` | server-side upload limit per file (0 = unlimited) |
| `allow_secret_export` | `1` | allow users to export their secrets, export private keys of the key store and show vault passwords (after re-entering the password) |
| `allow_link_shares` | `1` | allow “anyone with the link” shares for guests |
| `link_share_max_days` | `0` | longest lifetime of a guest link in days (0 = no limit); a link without expiry is refused when set |
| `chat_file_max_mb` | `5` | file size in collaboration chat (0 = off) |
| `voice_enabled` | `1` | voice calls on/off |
| `voice_max_participants` | `12` | people per call |
| `turn_enabled` | `1` | built-in TURN relay |
| `turn_port` | `3478` | relay port (UDP + TCP) |
| `turn_public_ip` | auto | public IPv4 of the server when behind NAT |
| `turn_host` | WRM host | host name browsers use to reach the relay |
| `turn_relay_ports` | `49152-65535` | UDP port range for relayed audio |
| `turn_allow_private` | `0` | allow relaying to private/loopback networks |
| `ice_servers` | Google + Cloudflare STUN | additional STUN/TURN servers (JSON) |
| `audit_enabled` | `1` | record security events (administrator actions are always recorded). Also `AUDIT_ENABLED` |
| `audit_retention_days` | `365` | how long audit log, file transfers and chat history are kept (minimum 7) |
| `file_edit_history` | `1` | keep every file edit with its diff (*Admin → File edits*) |
| `file_edit_max_kb` | `1024` | larger files are recorded without a diff (16–10240) |
| `session_recording` | `1` | record terminal sessions. Also `SESSION_RECORDING_ENABLED` |
| `session_recording_input` | `0` | also record keystrokes (masked at password prompts) |
| `recording_retention_days` | `90` | how long recordings are kept (minimum 7) |
| `recording_max_mb` | `100` | maximum recording size per session (0 = unlimited) |
| `tunnels_enabled` | `1` | SSH tunnels and web interfaces behind jump hosts; off stops running tunnels. Also `TUNNELS_ENABLED` |
| `tunnel_users` | `all` | `all` / `admins`: who may use tunnels (every *who may* policy also accepts `operators` and `teams:<names>`) |
| `tunnel_bind_any` | `0` | allow non-administrators to listen on network addresses (`0.0.0.0`, LAN IP) instead of `127.0.0.1` |
| `tunnel_remote_forward` | `admins` | `off` / `admins` / `all`: remote port forwarding (`-R`) |
| `tunnel_idle_minutes` | `0` | stop tunnels started by hand after this many minutes without traffic (0 = never) |
| `broadcast_enabled` | `1` | broadcast input (typing into several terminals at once). Also `BROADCAST_ENABLED` |
| `status_enabled` | `1` | live up/down status of connections. Also `STATUS_ENABLED` |
| `status_interval_seconds` | `60` | seconds between status rounds (15–3600) |
| `status_jump_checks` | `0` | check connections behind jump hosts by logging in to the jump host |
| `addon_personal` | `all` | who may install personal addons that only they see (`off`, `admins`, `all`); personal addons cannot carry server code |
| `addon_http` | `all` | who may use addons that reach web sites through WRM (`off`, `admins`, `all`); the sites still need an administrator's approval |
| `workboard_ssh_metrics` | `1` | the Workboard's *Server load* addon may read CPU, RAM, load and disks over SSH (Linux) for servers without Monitoring |
| `desktop_enabled` | `1` | RDP / VNC / Telnet in the browser. Also `REMOTE_DESKTOP_ENABLED` |
| `guacd_address` | `127.0.0.1:4822` | host:port of guacd. Also `GUACD_ADDRESS` |
| `bmc_enabled` | `1` | out-of-band management (BMC power, status, consoles). Also `BMC_ENABLED` |
| `serial_ports` | `admins` | who may use serial ports of the WRM server: `off`, `admins`, `all` |
| `network_tools` | `all` | who may use the network tools (port check, ping, traceroute, DNS, HTTP/TLS): `off`, `admins`, `all` |
| `proxies` | `all` | who may define saved proxies: `off`, `admins`, `all` (everybody may use proxies shared with them) |
| `notifications_enabled` | `1` | send notifications through the channels of *Admin panel → Notifications*. Also `NOTIFICATIONS_ENABLED` |
| `notify_min_interval_seconds` | `60` | at most one message per user and channel in this time (0–86400); the rest goes into the next digest |
| `git_enabled` | `1` | the Git workspace (⎇ Git); off hides it completely. Also `GIT_ENABLED` |
| `git_checks` | `all` | who may run Git checks (discovery and comparison over SSH, read-only): `off`, `admins`, `all` |
| `git_check_interval_minutes` | `60` | interval of periodic Git checks for users who turned them on (0 = none, up to 10080) |
| `git_backup_words` | `bkp,backup,bak,old,…` | path segments with one of these words are treated as copies / backups by Git discovery and comparison |
| `git_check_parallel` | `8` | how many installations a Git check compares at the same time, over all servers (1–64) |
| `git_check_per_server` | `2` | how many installations of one server a Git check compares at the same time (SSH sessions on one login, 1–8) |
| `git_update` / `git_upgrade` / `git_rollback` / `git_restart` | `admins` | who may update (and stamp), upgrade, roll back installations and restart their units: `off`, `admins`, `all` |
| `git_install` / `git_transfer` | `admins` | who may install services on a new server and transfer installations between servers: `off`, `admins`, `all` |
| `git_gitignore_mr` | `admins` | who may open a .gitignore merge request (GitLab) / pull request (GitHub) from the helper: `off`, `admins`, `all` |
| `git_schedule_grace_minutes` | `30` | a scheduled Git run or restart is skipped when WRM was not running at its time for longer than this (1–1440) |
| `update_check` | `1` | check the GitHub releases of `update_repo` once a day (no user data is sent); `WRM_UPDATE_CHECK=off` for air-gapped installations |
| `update_repo` | `vedranius/wrm-pro-enterprise` | GitHub repository (`owner/name`) whose releases are checked and installed |
| `update_signing_keys` | – | extra release public keys (`wrm-ed25519:…`, comma-separated) whose signature of `SHA256SUMS.txt` an update must carry, e.g. for your own fork in `update_repo`; this project's key is built in |
| `update_proxy` | – | HTTP proxy for the update check and download (e.g. `http://proxy.example.com:3128`); empty = `HTTPS_PROXY` / `HTTP_PROXY` |
| `self_update` | `admins` | who may update, roll back and restart WRM from the version badge: `off`, `admins` (never in Docker) |
| `ai_assistant` | `off` | who may use the AI assistant: `off`, `admins`, `all` |
| `ai_kill_switch` | `0` | `1` ends every AI session and blocks new ones |
| `ai_modes` | `read_only,ask` | allowed modes (`read_only`, `ask`, `auto`); mode rules can only narrow them |
| `ai_default_mode` | `read_only` | mode of a new session when allowed: `read_only`, `ask` |
| `ai_mode_rules` | `[]` | JSON rules `{"match": "tag"\|"folder"\|"connection"\|"host", "value", "modes"}`; the most restrictive matching rule wins |
| `ai_personal_keys` | `off` | who may add their own provider keys: `off`, `admins`, `all` |
| `ai_provider_kinds` | all | allowed provider types: `anthropic,openai,azure,bedrock,vertex,openai_compatible,opencode` |
| `ai_models` | – | allowed models, comma-separated patterns (`claude-*`); empty = every model of the providers |
| `ai_ollama_manage` | `0` | lets administrators install / start / pull Ollama through WRM from the provider wizard (`WRM_AI_OLLAMA_MANAGE`); off = the wizard only shows the commands |
| `ai_redact_output` | `1` | redact secrets before output is sent to the provider (transcripts and the audit are always redacted) |
| `screen_lock_mode` | `server` | how strict the screen lock is: `server` (a locked session gets `423` until unlocked), `remember` (survives a reload), `browser` (page only) |
| `screen_lock_max_minutes` | `0` | longest idle time before the screen lock users may choose; `0` = the user decides |
| `screen_lock_pin` | `1` | users may unlock the screen with a personal 4–12 digit PIN |
| `screen_lock_max_failures` | `0` | sign the browser session out after this many wrong unlock attempts; `0` = never |
| `appearance_default_theme` | `graphite` | theme for users who have not chosen one: a built-in id or `addon:<addon>:<theme>` |
| `appearance_custom_css` | `1` | users may add CSS to their own interface |
| `ai_show_thinking` | `1` | show the assistant's thinking (reasoning summaries) in the work log and keep it in the transcript |
| `ai_auto_titles` | `1` | give each AI chat a short title after its first answer (one small extra request without tools) |
| `ai_share_notes` | `1` | users may opt in to send their connection notes |
| `ai_transcript_retention_days` | `90` | AI transcripts are deleted after this time (1–3650) |
| `ai_approval_timeout_seconds` | `300` | an unanswered approval is denied (30–3600) |
| `ai_mcp_later_approval` | `run` | MCP: an approval given in WRM after the AI app stopped waiting runs the call at once (`run`) or only lets the AI repeat it (`repeat`) |
| `ai_keep_saved_sessions` | `1` | saved AI sessions (☆ in *AI sessions*) are kept beyond `ai_transcript_retention_days` |
| `ai_auto_max_minutes` / `ai_auto_max_actions` | `60` / `50` | the largest time limit and action count a user may opt into |
| `ai_auto_until_grant` | `1` | AI connections (MCP) may use automatic mode with no time or action limit, for as long as the connection is valid |
| `ai_allow_power` | `off` | `shutdown` / `reboot`: `off` (blocked) or `ask` (each needs an approval, never automatic) |
| `ai_blocked_commands` | – | extra command patterns refused in every mode |
| `ai_read_deny_paths` | – | extra path patterns treated as sensitive (a pattern without `/` matches the file name) |
| `ai_delegate` | `0` | the assistant may hand text jobs to helper models |
| `ai_delegate_provider` | `0` | id of the organisation provider for helper models |
| `ai_delegate_models` | – | allowed helper models, comma-separated; empty = automatic choice from the provider's catalog |
| `ai_delegate_max_kb` | `64` | the most text one delegation may send (1–1024 KiB) |
| `ai_delegate_training` | `user` | free models that may use prompts for training: `off` never, `user` each user decides (default off), `all` for everyone |
| `ai_opencode` | `all` | who may use OpenCode agents in the assistant: `off`, `admins`, `all` (the assistant permission still applies) |
| `ai_opencode_version` | `1.19.3` | the OpenCode version WRM installs |
| `ai_opencode_registry` | `https://registry.npmjs.org` | npm registry (HTTPS) WRM downloads OpenCode from |
| `ai_opencode_path` | – | absolute path of your own OpenCode binary instead of the one WRM installs |
| `ai_opencode_max_sessions` | `4` | OpenCode agents running at the same time (1–50) |
| `ai_opencode_sandbox` | `off` | OpenCode's own tools in a Docker sandbox on the WRM host, every use approved: `off`, `admins`, `all` |
| `ai_opencode_sandbox_image` | `debian:stable-slim` | Docker image of the sandbox (any image with a shell) |
| `ai_full_access` | `0` | Full access for the AI: everything WRM would refuse is asked instead (each time, never automatically); read-only stays read-only |
| `ai_full_access_mode` | `all` | Where full access applies: `all` (every AI session), `connection` (AI connections with the `full_access` scope), `user` (users who turn it on themselves) |
| `ai_full_access_blocked` | `ask` | Under full access, commands on `ai_blocked_commands` ask for approval (`ask`) or stay refused (`deny`); the built-in destructive list always asks |
| `ai_sensitive_reads` | `ask` | sensitive files: `ask` reads them after the user approves each read (secret-looking values redacted), `deny` never reads them |
| `ai_command_timeout_seconds` | `60` | a tool call is stopped after this time |
| `ai_output_max_kb` | `64` | output per tool call and the read limit for files |
| `ai_max_steps` | `25` | model requests per user message |
| `ai_idle_minutes` | `60` | an idle AI session ends |
| `ai_mcp_enabled` | `0` | serve the MCP endpoint `/mcp` and the OAuth endpoints for AI apps |
| `ai_mcp` | `admins` | who may create and use AI connections: `off`, `admins`, `all` |
| `ai_mcp_scopes` | all five | allowed scopes: `read_logs`, `run_readonly`, `run_with_approval`, `edit_file_with_approval`, `transfer` |
| `ai_provider_header_seconds` | `900` | provider response-header wait, 30–7200 seconds |
| `ai_provider_request_seconds` | `3600` | total provider request, 60–14400 seconds |
| `ai_provider_stream_idle_seconds` | `600` | provider stream inactivity, 30–7200 seconds |
| `ai_mcp_modes` | `read_only,ask,auto` | allowed modes of AI connections (the mode rules narrow them per connection) |
| `ai_mcp_token_hours` / `ai_mcp_max_token_hours` | `168` / `720` | default and maximum lifetime of a token or OAuth approval |
| `ai_mcp_oauth` | `1` | OAuth 2.1 (PKCE, dynamic client registration) for AI apps; `0` = tokens only |
| `ai_mcp_redirect_uris` | claude.ai, claude.com, ChatGPT, `http://localhost:*` | OAuth redirect URI allowlist (`*` = any text; https, or http on loopback) |
| `ai_mcp_allowed_origins` | – | browser origins (besides WRM) allowed to call `/mcp` (CORS) |
| `ai_mcp_public_url` | – | the public address of WRM for AI apps (reverse proxy) |
| `ai_mcp_elicitation` | `1` | also ask approvals in the AI app (MCP elicitation) |
| `ai_mcp_transfer_max_mb` | `100` | the largest file of the `transfer` scope |
| `desktop_tunnel_bind` | `127.0.0.1` | address of the temporary jump-host tunnels guacd connects to (keep 127.0.0.1 when guacd runs on the WRM machine) |

**Command line**

```
wrm -version                       print the version
wrm -reset-password USER           set a new temporary password (must be changed at sign-in)
wrm -reset-password USER -reset-2fa  … and turn off two-factor authentication
wrm -lockdown on|off|status        lockdown: only break-glass administrators can sign in (a running WRM applies it within 5 s)
    -lockdown-reason TEXT          the reason shown on the sign-in page (with -lockdown on)
wrm -healthcheck                   check /healthz of the local server (exit code 0 = healthy; for containers)
wrm -install-service               install WRM as a service that starts at boot, start it (see Service)
    -security-level LEVEL          home, business or enterprise on first start (kept in the service environment)
wrm -uninstall-service             stop and remove the service (the data stays)
wrm -service-status                is the service installed and running?
    -service-name NAME             service name (default wrm)
    -service-scope system|user     macOS: LaunchDaemon (boot) or LaunchAgent (login)
    -service-user ACCOUNT          Unix account the service runs as (default: you, or SUDO_USER)
wrm -no-service-prompt             do not ask on the console whether to install the service
wrm -env-file FILE                 read KEY=value lines before starting (variables already set win)
wrm -workdir DIR                   change to DIR before starting
wrm -mcp-stdio -url URL -token T   stdio bridge for MCP clients to the WRM server at URL (see AI desktop apps)
    -token-file FILE               read the token from FILE (or WRM_MCP_TOKEN; the URL also from WRM_MCP_URL)
    -ca-file FILE                  trust this CA / self-signed certificate (PEM) for the WRM server
wrm -addon-new TEMPLATE DIR        write an addon template (blank, fetch-and-render, grafana-panel, wasm-hook) into an empty DIR
    -addon-id ID                   use ID as the addon id
wrm -addon-lint DIR|ZIP            check an addon (schema, files, sandbox rules, secrets, permissions); exit 1 on an error
    -addon-lint-json               print the report as JSON
wrm -addon-dev DIR                 serve the addon's panel on 127.0.0.1 with the real CSP and a WRM stand-in, reload on change
    -addon-dev-port PORT           port of the dev server (default 8790)
    -addon-dev-private             allow private-network and plain-http sites in the dev server
wrm -release-verify SHA256SUMS.txt check SHA256SUMS.txt.sig with the built-in release key, and the SHA-256 of the listed files next to it
    -release-public-key KEY        also trust this public key (wrm-ed25519:…)
wrm -release-keygen FILE           (maintainers) new release key pair: private key to FILE (0600), public key printed
wrm -release-sign FILE             (maintainers) write FILE.sig; key from WRM_RELEASE_SIGNING_KEY or -release-key-file FILE
```

`-service=…` is set by the service definitions WRM writes; it is not for interactive use.

`GET /healthz` (no sign-in) returns `{"status":"ok","version":…}` while the server and its database work, otherwise HTTP 503. Use it for load balancers and monitoring.

---

## Docker, service, reverse proxy & firewall

### Docker

The repository contains a `Dockerfile` (static binary on Alpine, runs as an unprivileged user, data in the volume `/data`, built-in health check) and a `docker-compose.yml`:

```bash
docker compose up -d                       # build and start
WRM_TURN_PUBLIC_IP=203.0.113.10 docker compose up -d   # with voice relay: the IPv4 browsers reach this host at
```

Images of releases are also published to the GitHub Container Registry: `docker run -d -p 8080:8080 -v wrm-data:/data ghcr.io/vedranius/wrm-pro:latest`.

- `/data` holds the database, the encryption key (`remote_manager.db.key`), recordings and the self-signed certificate. Back up the volume and keep a **separate** copy of the key — or provide your own key as a Docker secret (`ENCRYPTION_KEY_FILE=/run/secrets/…`, see the comments in `docker-compose.yml`).
- The compose file enables `HTTPS_SELF_SIGNED=1`. Behind a TLS reverse proxy remove it and set `WRM_TRUST_PROXY=1`.
- Voice relay from a container needs `WRM_TURN_PUBLIC_IP` and the published relay ports (`49160-49200/udp`, matching `WRM_TURN_RELAY_PORTS`).
- `127.0.0.1` inside the container is the container. The compose file adds `extra_hosts: ["host.docker.internal:host-gateway"]`, so a [proxy](#proxies-socks--http) on the Docker host is `host.docker.internal:<port>` (it must listen on an address the container reaches, e.g. the Docker bridge). With `docker run` add `--add-host=host.docker.internal:host-gateway`.

### Service

**The easy way:** start WRM once in a terminal. It asks:

```
WRM can run as a service that starts at boot (a systemd unit).
Install it as a service now? [y]es / [n]o / [d]on't ask again (default: no):
```

*Yes* installs and starts the service and ends the console process; *No* asks again next time; *Don't ask again* is remembered in `<DB_PATH>.service.json`. The question is not asked when stdin is not a terminal, in a container, as a service, with `-no-service-prompt` or `WRM_NO_SERVICE_PROMPT=1`, or when the service already exists. Scripts and administrators use the flags directly:

```bash
sudo ./wrm-pro-v12.6.6-linux-amd64 -install-service          # systemd unit, enabled and started
./wrm-pro-v12.6.6-linux-amd64 -service-status
sudo ./wrm-pro-v12.6.6-linux-amd64 -uninstall-service        # the data stays
```

The service uses the **same data directory and settings** as the interactive start: the working directory, `DB_PATH` (made absolute), `PORT` / `LISTEN_ADDR`, key and certificate files, proxies and every `WRM_*` variable are written to `<data dir>/wrm.env` (mode `0600`). The installer checks that the service account can write the database and its directory, and prints the database path it selected. Edit `DB_PATH` in that file and restart the service to point it at another WRM installation's database.

| OS | What `-install-service` does |
|---|---|
| **Linux** | `/etc/systemd/system/wrm.service` with `EnvironmentFile=`, `WorkingDirectory=`, `User=` (you, or `SUDO_USER`; `-service-user`), `Restart=on-failure` with a restart-rate limit, and hardening (`NoNewPrivileges`, `ProtectSystem=strict` with `ReadWritePaths=` for the data and binary folders, `ProtectHome=read-only`, `PrivateTmp`, kernel and cgroup protection, `UMask=0077`; `CAP_NET_BIND_SERVICE` for ports below 1024), then enables the unit, clears previous startup failures and restarts it to apply the new binary. If startup fails, the installer prints the unit status and recent journal entries. Needs root: when you answer *Yes* as a normal user, WRM runs the installer with `sudo` (or `doas`). Without systemd it says so and prints the command for your init system. |
| **macOS** | A launchd plist `local.wrm`: a **LaunchDaemon** in `/Library/LaunchDaemons` (starts at boot, needs `sudo`; runs as you) or a **LaunchAgent** in `~/Library/LaunchAgents` (starts at login): you choose in the prompt or with `-service-scope system|user`. `KeepAlive` restarts it after a failure; the log is `<data dir>/wrm-service.log`. |
| **Windows** | A real Windows service (Service Control Manager) *Web Remote Manager PRO*, automatic (delayed) start, recovery *restart after 3 s* (also when WRM asks for a restart), log in `<data dir>\wrm-service.log`. Needs administrator rights: the prompt asks Windows for them (UAC); otherwise run `.\wrm-pro-….exe -install-service` in PowerShell opened with *Run as administrator*. |
| **FreeBSD** | `/usr/local/etc/rc.d/wrm` running WRM under `daemon(8) -r` (restart on exit) as your user, `sysrc wrm_enable=YES`, `service wrm start`. |
| **OpenBSD** | `/etc/rc.d/wrm` with `rc.subr` (`daemon_user`, output to syslog), `rcctl enable wrm`, `rcctl start wrm`. |
| Android (Termux), others | Not supported: WRM says so; use your init system or `tmux` / `screen`. |

`-service-name NAME` installs several instances side by side (each with its own data directory).

**By hand (systemd):** if you prefer to write the unit yourself, e.g. `/etc/systemd/system/wrm.service`:

```ini
[Unit]
Description=Web Remote Manager PRO
After=network-online.target
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
User=wrm
WorkingDirectory=/opt/wrm
Environment=LISTEN_ADDR=127.0.0.1:8080
Environment=WRM_TRUST_PROXY=1
Environment=ENCRYPTION_KEY_FILE=/etc/wrm/encryption.key
ExecStart=/opt/wrm/wrm-pro-v12.6.6-linux-amd64 -service=systemd
Restart=on-failure
RestartForceExitStatus=75
SuccessExitStatus=75
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/opt/wrm
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload && sudo systemctl enable --now wrm
```

`-service=systemd` and exit code 75 let the one-click update restart WRM through systemd; `ReadWritePaths` must include the binary's folder for it.

**nginx** in front of WRM (TLS, WebSockets, streaming and big uploads):

```nginx
server {
    listen 443 ssl http2;
    server_name wrm.example.com;
    # ssl_certificate ...; ssl_certificate_key ...;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;                # required: origin check
        proxy_set_header X-Real-IP $remote_addr;    # with WRM_TRUST_PROXY=1
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 3600s;                   # long-lived terminals
        proxy_buffering off;                        # live transfer/search progress
        client_max_body_size 0;                     # large uploads
    }
}
```

**Firewall for voice calls:** allow **UDP and TCP 3478** (TURN) and the **UDP relay range** (`49152-65535`, or narrow it with `turn_relay_ports`) to the WRM server. Behind NAT (cloud VM, home router) forward those ports and set `turn_public_ip`. The TURN port is not proxied through nginx. For networks that only allow HTTPS, run a TURN server with TLS on port 443 (e.g. coturn) and add it in *Admin → Voice & network*.

### HTTPS with Caddy

[Caddy](https://caddyserver.com/) gets and renews a certificate by itself and proxies WebSockets without extra settings — the shortest way to HTTPS, which browsers need to **install WRM as an app**, for the microphone and for secure cookies. WRM listens on `127.0.0.1:8080` with `WRM_TRUST_PROXY=1` (as in the service above). `/etc/caddy/Caddyfile`:

```caddyfile
# A public DNS name pointing to this server (ports 80 and 443 open): a Let's Encrypt certificate.
wrm.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Only an internal name or an IP address? Let Caddy be its own certificate authority and trust it once on every computer that opens WRM:

```caddyfile
wrm.lan.example.com, 10.0.0.10 {
    tls internal
    reverse_proxy 127.0.0.1:8080
}
```

```bash
sudo systemctl reload caddy
# root certificate of Caddy's local CA, to import into the browsers' / systems' trusted roots:
sudo cat /var/lib/caddy/.local/share/caddy/pki/authorities/local/root.crt
```

Caddy has no request body limit by default, so large imports and uploads pass. With WRM in Docker, point `reverse_proxy` at the published port and remove `HTTPS_SELF_SIGNED=1` from the compose file.

---

## Security model

See also **[SECURITY.md](SECURITY.md)** (how to report vulnerabilities, hardening checklist).

- **Accounts:** bcrypt hashes, optional/required TOTP 2FA with single-use recovery codes, closed self-registration by default, account lockout and per-IP rate limiting, constant-time responses for unknown users, forced password change after an admin reset, disable/delete accounts with immediate sign-out everywhere.
- **Sessions:** 256-bit random tokens in HttpOnly/SameSite cookies (`Secure` over HTTPS), stored hashed; idle and absolute expiry; device list and remote sign-out.
- **Secrets at rest:** AES-256-GCM with a random per-installation key (or your own) — connection passwords and keys, the SSH key store and the credentials vault. Secrets are **never sent to the browser** — not to share recipients, not to users a credential is shared with — except when their owner explicitly exports or shows them after entering the account password again (policy `allow_secret_export`, audited). The database and key files are created with mode `0600`.
- **Web security:** CSRF protection (custom request header + Origin check on every state-changing API call), WebSocket origin check, strict **Content-Security-Policy** (no external scripts, `frame-ancestors 'none'`), `X-Frame-Options`, `nosniff`, `Referrer-Policy: no-referrer` (share tokens never leak via Referer), `Permissions-Policy`, HSTS on HTTPS, request-size limits, no directory listings, all assets served locally (no CDN / supply-chain dependency at runtime).
- **SSH & FTPS:** host keys verified (TOFU/strict); FTPS certificates validated against public CAs or pinned. Server-side key files are admin-only by default.
- **Sharing:** four roles enforced server-side on every file operation, terminal and room action; members-only / signed-in / guest modes; signed password cookies (invalidated when the password changes); expiry, pause and link rotation; bans; immediate revocation of open terminals and room connections.
- **Collaboration:** server-assigned identities, per-client send queues (a slow client cannot stall a room), message rate limits and size limits, terminal data only to watchers, keystrokes only to the granting sharer, file downloads never rendered inline.
- **Voice:** WebRTC with DTLS-SRTP; the built-in TURN relay accepts only short-lived HMAC credentials issued to room participants, limits allocations, and refuses private/loopback/link-local peers by default (no pivoting into internal networks).
- **Audit:** sign-ins (also failed), account and policy changes, connections, shares, room joins and moderation, terminal sessions, file transfers (with SHA-256) and changes, host keys, exports, viewing of recordings — stored **append-only** and **hash-chained** in the database (verifiable) and written as `AUDIT …` lines to the server log (journald/syslog → SIEM). Secrets in audit details are redacted.
- **Session recording:** terminal output in asciicast v2, gzip, mode `0600`, SHA-256 in the database; no keystrokes unless enabled (then masked at password prompts); visible to administrators, the session's user and the connection's owner; every view is audited.
- **Folder bookmarks:** per user; the server fills in the variables and builds the shell-quoted `cd`, so a path cannot inject commands. Share participants see only the owner's bookmarks marked as shared (global, read-only), and only for connections of the share.
- **Snippets & broadcast:** snippets are per user (shared snippets only by administrators, read-only for others, never auto-run); run-on-connect snippets come from the connection's owner and are recorded in the audit log; broadcast input is client-side typing into the user's own terminals, with confirmation of dangerous commands and an audit entry for every start and stop per terminal.
- **Out-of-band management:** BMC passwords are encrypted (or come from the vault, with its host rules); Redfish certificates are pinned on first use, ipmitool gets the password in its environment (`-E`) — on a jump host through stdin, never on a command line; power actions and consoles are audited and limited to the connection's owner; serial ports are an administrator feature by default.
- **Inventory import:** files are parsed in memory (CSV, or `.xlsx` read with size limits) and never stored. NetBox is called from the WRM server over HTTP(S) with your token; redirects and pagination links to another server are refused, responses are size-limited, and a remembered token is stored encrypted and never returned to the browser.
- **SSH keys & vault:** private keys and vault passwords are encrypted at rest and used only on the server. Keys go to servers on stdin of a POSIX `sh` script (never on a command line); `authorized_keys` is changed in place, keeping other lines, permissions and SELinux contexts; WRM refuses to remove the key it logs in with. Shared credentials are usable but never readable by the people they are shared with; a host list limits where they can be sent (target and, for them, jump hosts). Rotation is all-or-nothing with a pre-flight check, verification of every new login and rollback; passwords are scrubbed from `passwd` output and never logged.
- **Remote desktop:** WRM does the guacd handshake with the stored credentials (never sent to the browser) and forwards only display and allow-listed input instructions; jump-host tunnels for guacd listen on loopback and live only as long as the session; sessions are audited and recorded.
- **Live status:** checks are plain TCP connections from the WRM server (no credentials, except optional checks through jump hosts with the jump host's own saved login); users only see the states of their own connections.
- **Tunnels & jump hosts:** every hop is authenticated and host-key-verified; tunnels belong to the owner of the connection (only the owner starts them; administrators can stop any); listen on `127.0.0.1` by default; ports below 1024 refused; network addresses and remote forwards only for administrators (policies); a tunnel can be turned off globally or limited to administrators; every start/stop/error is audited with the traffic. A tunnel port on a network address is **not** protected by WRM sign-in — treat it like an open port of that machine.
- **Proxies:** proxy passwords are encrypted, never sent to the browser and not shown to grantees; a proxy shared with a password is never reached through the grantee's own jump hosts; WRM tunnel proxies cannot be shared; IPMI / SOL never bypass a proxy silently (refused). Creation, changes, grants and tests are audited (`proxy.*`) and every session records its route.
- **Notifications:** channel secrets are encrypted and write-only; users only see enabled channels by name and type; messages go only to the owner of a connection or credential; configuration changes are audited as `admin.*` (always recorded).
- **Quick connect, notes & network tools:** quick connections are ordinary connections of their owner (same checks, encryption, host keys, audit) that expire; notes are only shown to the owner and rendered as escaped text; network tools only take host names or addresses (never options or shell characters), run from the WRM server or the user's own SSH connections, one at a time per user, and are audited (`nettool.run`) — limit them with `network_tools`.
- **AI assistant:** the model acts only through WRM's tools; the server-side permission engine (read-only classifier, approvals, automatic-mode limits, the always-blocked destructive list, unreadable sensitive files) decides every call, and only the user changes the mode. Server output is passed to the model as untrusted data and redacted (policy); provider keys are encrypted and never sent to the browser; every request, tool call and approval is audited and recorded. See [SECURITY.md](SECURITY.md#ai-assistant-threat-model).
- **AI desktop apps (MCP):** `/mcp` accepts only bearer tokens of AI connections (never the WRM cookie); tokens are random, stored as SHA-256, bound to a user, chosen connections, a mode, scopes and an expiry, and revocable; OAuth needs PKCE (S256), exact redirect URIs from an allowlist and the user's consent after a normal sign-in; requests from foreign browser origins are refused and CORS answers only allowed origins; every tool call goes through the same permission engine and audit as the built-in assistant. See the MCP threat model in [SECURITY.md](SECURITY.md).
- **Transport:** use HTTPS (certificate, reverse proxy, or `HTTPS_SELF_SIGNED=1`). Without it passwords and terminal traffic between browser and WRM are not encrypted and browsers block the microphone.

---

## Limitations

- Ping and traceroute use the commands of the WRM server or of the source server (`ping`, `traceroute`/`tracepath`); without them the tool says so. The app (PWA) installs only over HTTPS with a trusted certificate or on `localhost`.
- IPMI needs `ipmitool` (on the WRM server or the jump host); Serial-over-LAN on the WRM server and serial ports need WRM on Linux. Power actions go to one server at a time.
- Inventory import reads `.xlsx` and CSV, not old `.xls`; NetBox sync updates and reports, but never deletes connections.
- Password rotation works for SSH logins that may run `passwd` (Linux, BSD, macOS) — not for Windows/RDP, network devices or IPMI. *Who has access* shows `~/.ssh/authorized_keys` of the login user only.
- RDP, VNC and Telnet need **guacd** next to WRM (see [Remote desktop](#remote-desktop-rdp-vnc-telnet)); file transfer and printer redirection of RDP are not offered.
- The AI assistant works with SSH and SSH-backed SFTP connections (not RDP / VNC / Telnet or BMC consoles), runs non-interactive commands only (no editors, pagers or password prompts), and its read-only classifier is deliberately strict: a harmless command it does not know needs an approval. Consumer chat subscriptions (Claude.ai, ChatGPT) cannot be used; desktop AI apps over MCP come with v12.1.0.
- AI connections (MCP) work with the user's **own** SSH connections (not connections shared with them); OAuth access is not refreshed — when it expires the user approves the app again. `/mcp` offers no standalone event stream (GET) and no resumption of an interrupted stream; a call interrupted while it waits for an approval is cancelled.
- No **SSO/LDAP/SAML** yet (local accounts with 2FA).
- Voice calls are a **mesh**: fine up to about 12 people; larger meetings would need an SFU.
- No SSH **agent forwarding**. Jump hosts must be SSH servers (no HTTP/SOCKS proxies as jump hosts).
- Tunnel ports open on the **WRM machine**: when WRM runs on a server, a tunnel listening on `127.0.0.1` is not reachable from your PC (see [Where is the tunnel port?](#ssh-tunnels-port-forwarding)).
- Server-to-server transfer and ZIP download work with SSH/SFTP servers only (not FTP). *File contents* search needs `grep` on the server.
- The built-in TURN relay is IPv4 and not available in the Android build (use an external TURN server there).

Ideas and pull requests for any of these are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

---

## API reference

### v13.14.0 endpoints

| Endpoint | Behavior |
|---|---|
| `GET /api/folders/sort` | `{keep_all}`: whether all your folders are kept sorted. |
| `POST /api/folders/sort` | `{folder_id, recursive, keep}`: sort the subfolders of a folder (`0` = top level) by name, at every level with `recursive`; `keep` switches automatic sorting for that folder (for `0`: all folders). Returns `{folders, keep_all, moved}`; audited as `folder.sorted`. |
| `GET /api/admin/file-edits` | File edit history (administrators, auditors): filters `q`, `user`, `source` (`editor`, `ai`), `session`, `server`, `conn`, `from`, `to`, `before_id`; `format=csv` exports. |
| `GET /api/admin/file-edits/{id}` | One edit with its diff; audited as `file_edit.viewed`. |
| `PUT /api/keys/{id}` | Also `{"certificate": "…-cert-v01@openssh.com …"}` (empty removes it); the certificate must be a user certificate for this key. `GET /api/keys` adds `cert` (type, key ID, principals, validity, CA fingerprint, `opkssh`). |
| `POST /api/remote/upload?…&edit=1` | An editor save: WRM records the change in the file edit history. |

### v13.7.0 endpoints

| Endpoint | Behavior |
|---|---|
| `GET / POST /api/auth/lock` | Lock state of this sign-in session / lock it (`{"reason": "manual"\|"idle"\|"hidden"\|"os_locked"\|"sleep"\|"shortcut"}`), audited as `auth.locked`. |
| `POST /api/auth/lock/verify` | `{"password"}` or `{"pin"}`: unlock (`auth.unlocked` / `auth.unlock_failed`); `429` after five attempts a minute, `401 {"signed_out": true}` at `screen_lock_max_failures`. |
| `PUT / DELETE /api/auth/lock/pin` | Set (`{"pin", "password"}`) or remove the unlock PIN. |
| `GET / PUT /api/me/ui/appearance`, `/api/me/ui/lock` | The user's appearance and screen lock preferences (JSON object, 256 KiB). |

While a session is locked in `server` mode every other authenticated endpoint answers `423 Locked`; `/api/auth/me` reports `screen_locked` and the `screen_lock` and `appearance` policy.

### v12.4.4–v12.4.5 endpoints

All endpoints require a WRM session and effective owner/policy access. Mutating `/api/` calls require WRM's normal CSRF header/origin.

| Endpoint | Behavior |
|---|---|
| `POST /api/git/medium/setup?confirm=1` | Combined multipart `tool`, `config`, optional `ssh` ZIPs (20 MiB each); retained background setup and automatic launch. Empty fields resume/reuse imports. |
| `GET /api/monitoring/logs?id=…` | Bounded automatic/manual root discovery; progressive UI aggregates files/roots/warnings per server. |
| `GET /api/monitoring/log?id=…&root_path=…&path=…` | Authorized historical/live chunk, optional `scan=1`, byte `offset`, date/timezone/keyword; modification/event-range/record-count metadata. |
| `GET / PUT /api/monitoring/logs/settings?id=…` | The server's log settings (scan paths, exclusions, path patterns, limits, journal, metadata sources, method) and the defaults; only the owner may save. |
| `POST /api/monitoring/logs/discover?id=…` | Discover logs now with the log settings (SSH, or SFTP as a fallback). |
| `POST /api/monitoring/logs/grep?id=…` | `{files, q, regex, case, word, context, max, from, to, timezone}`: grep discovered files on the server; hits with path, line, text and parsed time. |
| `GET /api/monitoring/logs/lines?id=…&path=…&mode=tail\|around\|since` | Lines of one discovered file: `n`, or `line`/`before`/`after`, or `bytes` (files) / `cursor` (journal) for following. |
| `GET /api/git/medium` | Runtime/import/dependency status and tool SHA-256; the running `operation` (op, user, start), the caller's `steps` with times, `pending_config` / `pending_ssh`, `running_since`, `last_exit` and `output_policy`. |
| `GET /api/git/medium/activity?limit=&output=1` | Medium activity log (newest first): time, user, operation, result, duration, detail, error, steps and, per `medium_log_output`, redacted output. Users see their own and shared rows. |
| `GET /api/git/medium/log` | Live output (redacted) of the caller's running operation and original-tool process, per `medium_log_output`. |
| `POST /api/git/medium/{tool,config,ssh}?confirm=1` | Multipart ZIP in field `file`; tool is administrator-only; 20 MiB expanded limit. Tool import returns `202` and prepares the runtime in the background. Config is checked at once (`400` with the reason); config/SSH return `202` with `pending` when kept for the next start. `409` names the running operation. |
| `GET, POST /api/git/medium/settings` | Read masked / save encrypted original runtime preferences. |
| `POST /api/git/medium/docker` | Administrator: `{"accept": true, "accept_license": true}` records the consent (the licence is required for Docker Desktop) and installs, starts and waits for Docker in the background (`202`), then prepares the runtime if the tool is imported; `{"accept": false}` withdraws the consent. `GET /api/git/medium` reports `docker` (`installed`, `product`, `method`, `auto`, `allowed`, `needs_license`, `consent`, `why`, `command`, `note`) while Docker is missing. |
| `POST /api/git/medium/{start,stop,prepare}` | Start original app in the background (`202`; poll the status) / stop only when idle (`stop?force=1` stops a tool whose state cannot be read or that reports work, after a second confirmation in the UI) / administrator retry of retained tool setup. |
| `GET /api/git/medium/native/config` | Native preview: validated deploy-config summary (projects, environments, servers with their WRM connection links) or the validation errors. |
| `POST /api/git/medium/native/link` | Link a `servers.yaml` server to a saved SSH connection (`{"server","conn_id"}`, `conn_id` 0 = automatic). |
| `GET /api/git/medium/native/refs?project=` | Branches and tags of the project's WRM Git source. |
| `POST /api/git/medium/native/plan` | Start a read-only native preview (`project`, `environment`, `ref`, optional `servers`, `compare`, `excluded`). Returns the run id. |
| `GET /api/git/medium/native/plan/<id>` | Stage, per-server plans, comparison with the original tool and source check of a preview; `deployable` and `revision` for a native deployment. |
| `GET /api/git/medium/native/plan/<id>/diff?server=&path=` | One changed file of a finished plan: the server's and the branch's side (`text`, `missing`, `binary`, `too_large`, `symlink`, `dir`, CRLF), `eol_only` and the line diff. |
| `GET /api/git/medium/native/config/files` | deploy-config files (name, size, hash), whether the configuration loads, and whether the user may edit it. |
| `GET /api/git/medium/native/config/file?name=` | One deploy-config file's text and hash. |
| `POST /api/git/medium/native/config/files` | Check, save, create or delete a file (`name`, `text`, `base_hash`, `check_only`, `delete`). Saved only when the whole configuration loads; `409` when the file changed since `base_hash`. Policy `medium_native`. |
| `GET /api/git/medium/native/config/git` | deploy-config Git state: repository and source, branch, pulled commit, branch head, `behind`, local `changes`, `can_push`. |
| `POST /api/git/medium/native/config/git/pull` | Make the branch head WRM's copy of deploy-config (`{"discard":true}` drops local changes; `409` with local changes otherwise). Policy `medium_native`. |
| `POST /api/git/medium/native/config/git/push` | Validate, commit and push the local changes (`message`, optional) through the Git source's write token; `409` when a changed file moved on Git. Policy `medium_native`. |
| `POST /api/git/medium/native/config/git/stop` | Delete WRM's Git copy and use the imported deploy-config again. Policy `medium_native`. |
| `POST /api/git/medium/native/engine` | Switch a project between the Docker runtime and the native engine (`{"project","engine":"docker"\|"native"}`, policy `medium_native`). |
| `POST /api/git/medium/native/deploy` | Deploy or dry-run a finished comparison (`run_id`, `revision`, `confirm`, `dry_run`). Returns `job_id` and `deploy_id`. Step-up `medium.deploy`. |
| `GET /api/git/medium/native/history?project=&environment=` | Per destination: `current.json`, the lock, recent history entries and the rollback choice; `stale_lock_minutes`. |
| `POST /api/git/medium/native/rollback` | Roll back or dry-run a rollback (`project`, `environment`, `deploy_id`, `servers`, `confirm`, `dry_run`). Step-up `medium.rollback`. |
| `POST /api/git/medium/native/lock/release` | Release a stale lock (`project`, `environment`, `server`, `deploy_id` of the lock owner). Step-up `medium.lock_release`. |
| `GET /api/git/medium/native/jobs` · `GET /jobs/<id>?from=` · `POST /jobs/<id>/cancel` | The caller's native deployments and rollbacks: phase, outcome per destination and log lines from `from`; cancel before the next destination. |
| `/medium/*` | Original UI and native actions behind WRM authentication, policy and origin checks. |
| `GET, POST /api/git/config-profiles` | List / add reference `{name,app,path,operator,content}`. |
| `DELETE /api/git/config-profiles/{id}` | Remove owned profile. |
| `POST /api/git/config-profiles/{id}/check` | Inspect matching installs; optional `{ids:[...]}` selection. |
| `GET /api/bitwarden`, `GET /api/bitwarden/keys` | CLI/account status / native SSH item metadata only. |
| `POST /api/bitwarden/{login,unlock,sync,lock,logout}` | Account and memory-only unlock operations. Login accepts `server,email,master_password,client_id,client_secret,method,code` as appropriate. |
| `POST /api/folders/reorder` | Move `{id,parent_id,before_id}` or `{id,parent_id,after_id}` with owner/cycle/depth guards. |


All endpoints (except sign-in, `/api/auth/config`, version and share pages) need the session cookie. JSON in, JSON out, unless noted. **Every unsafe `/api` request (POST/PUT/PATCH/DELETE) must send `X-WRM-Request: 1`** (CSRF protection) and, if present, an `Origin` of this server.

**Accounts**

| Method | Endpoint | Description |
|---|---|---|
| GET | `/api/auth/config` | `{first_run, registration_open, version}`; on first run also `security_level`, `security_level_fixed`, `secure_transport`; `https_required` when sign-in needs HTTPS; with an available update also `update`, `update_url` |
| POST | `/api/auth/register` | Create account (first user = admin; afterwards only if registration is open); the first user may send `security_level` |
| POST | `/api/auth/login` | `{username, password}` → user, or `{mfa_required, mfa_token}` |
| POST | `/api/auth/mfa` | `{mfa_token, code}` (TOTP or recovery code) |
| POST | `/api/auth/logout` | Sign out |
| GET | `/api/auth/me` | Current user, restrictions and policies; also `role`, `is_auditor`, `break_glass`, `teams` and `step_up: {required, minutes, totp}` |
| GET | `/api/auth/oidc/start?provider=<id>` | Directory sign-in: redirects to the identity provider (authorization code + PKCE) |
| POST | `/api/auth/oidc/start` | `{provider, mode}` with `link`, `stepup` or `test` (administrators) for a signed-in user: `{url, state}` to open in a new window |
| GET | `/api/auth/oidc/callback` | The provider returns here (register this address at the provider) |
| GET | `/api/auth/oidc/result?state=` | Outcome of a link / step-up / test window (`done`, `ok`, `error`, test details) |
| GET / POST | `/api/me/ldap` | Your linked directory accounts and the enabled directories / link one `{directory, username, password}` (fresh confirmation required) |
| DELETE | `/api/me/ldap/{id}` | Unlink your directory account (accounts that still have a local password) |
| * | `/scim/v2/…` | SCIM 2.0 for identity providers (`Authorization: Bearer wrm_scim_…`): `Users`, `Groups`, `ServiceProviderConfig`, `ResourceTypes`, `Schemas` |
| GET / DELETE | `/api/me/identities`, `/api/me/identities/{id}` | Your linked directory accounts and the providers you can connect / unlink one (accounts with a password only) |
| POST | `/api/auth/step-up` | `{password, code, action}` confirms this session for `step_up_minutes`; a 403 with `step_up: true` from another call means this is needed |
| GET | `/api/teams` | Teams for the share dialog `{teams: [{id, name, member_count}], mine}` |
| GET / POST / DELETE | `/api/me/events` · `/api/me/events/read` | Your in-app notifications: `GET ?limit=` (default 50, at most 200) → `{events: [{id, event, title, body, at, read}], unread}`; `POST /api/me/events/read` `{ids: […]}` or `{all: true}`; `DELETE /api/me/events` clears the list |
| GET | `/api/addons` | installed addons you can use (administrators: all organisation addons; `?personal=all` adds `personal_others`); each has `key` (`id`, or `~id` for your personal addon), `asset_base`, `apiVersion`, `permissions`, `needs_approval`, `permissions_added`; also `capabilities` and `personal_allowed` |
| POST | `/api/addons/install` | install a ZIP (body). Administrators install for the organisation unless `scope=personal`; other users install a personal addon. `allow_downgrade=1` installs an older version. Refusals carry `{error, code, hint}` |
| POST | `/api/addons/{key}/enable` · `/disable` · DELETE `/api/addons/{key}` | manage an addon: administrators for organisation addons, the owner for a personal one; enabling records the approved permission sheet |
| POST · DELETE | `/api/addons/users/{user id}/{id}/disable` · `/api/addons/users/{user id}/{id}` | administrators: turn off or remove another user's personal addon |
| GET · POST | `/api/addons/{key}/instances` | instances of an apiVersion 2 panel you can see; create `{name, shared}` (shared: administrators, organisation addons) |
| PATCH · DELETE | `/api/addons/{key}/instances/{iid}` | rename `{name}` or remove an instance with its values and windows |
| GET · PUT · DELETE | `/api/addons/{key}/instances/{iid}/store[/{k}]` | stored values: all (`{values, used, quota, writable}`), one key, write a JSON body (32 KB, quota per instance), delete |
| GET · POST | `/api/addons/{key}/instances/{iid}/export` · `/import` | the instance's values as `{"format": "wrm-addon-config", …}` and back |
| PUT · DELETE | `/api/addons/{key}/instances/{iid}/window` | your window state of the instance (JSON object, 4 KB) |
| POST | `/api/addons/{key}/instances/{iid}/http` | send a request for the panel `{method, url, headers, body, encoding, secret}` → `{ok, status, headers, body, encoding, truncated, code}`; refusals carry `E_HTTP_*` codes |
| GET | `/api/addons/{key}/instances/{iid}/http/log` | the last 50 requests of the instance (your own; administrators: all) |
| GET · PUT · DELETE | `/api/addons/{key}/instances/{iid}/secrets[/{name}]` | secret states (never values); set `{value, scope}` (`instance` or `own`); clear with `?scope=` |
| POST · DELETE | `/api/addons/{key}/http` · `/api/addons/users/{user id}/{id}/http` | administrators: approve the addon's sites `{private: [origins]}` or withdraw the approval |
| GET | `/api/addons/dev/schema.json` · `/api/addons/dev/llms.txt` | the JSON Schema of `addon.json` and the addon guide for AI assistants (signed-in users) |
| GET | `/api/addons/dev/templates` · `/api/addons/dev/templates/{name}.zip?id=` | the starter templates; one as a ZIP, with the manifest id replaced when `id` is given |
| POST | `/api/addons/dev/lint` | lint a ZIP (body) without installing it → `{ok, errors, warnings, diagnostics: [{level, code, message, file, line, hint}], permissions, summary, csp}`; administrators and users allowed personal addons (`E_PERSONAL_OFF`) |
| GET | `/api/addons/{key}/asset/{file}` | a file of an enabled addon; HTML of apiVersion 2 panels carries the full CSP; a file that changed since install answers 409 `E_INTEGRITY` |
| GET | `/api/workboard/metrics?id=` | CPU, RAM, load, uptime and disks of one of your SSH/SFTP connections: `{conn_id, at, source: zabbix\|ssh, cpu_percent, memory_percent, memory_total, load, cpu_count, uptime, disks: [{mount, percent, size_kb}], error}`; `refresh=1` skips the 25 s cache; 403 when reading over SSH is off and there is no fresh Monitoring sample |
| GET / PUT / DELETE | `/api/me/ui/{key}` | Your interface state, `key` = `layout` (panel layout), `workboard` or `ai` (my AI settings: `provider_id, model, mode, engine, thinking`): `GET` → `{value, updated_at}` (`value` null when nothing is stored), `PUT {value: {…}}` (a JSON object, at most 64 KiB), `DELETE` resets |
| POST | `/api/auth/password` | `{current_password, new_password}` |
| POST | `/api/auth/2fa/setup` · `enable` · `disable` · `recovery` | Two-factor management |
| GET / DELETE | `/api/auth/sessions[/{id}]` | Signed-in devices / sign out one or all others |
| GET | `/api/users?q=` | User directory (for adding members) |

**Connections, folders, sessions**

| Method | Endpoint | Description |
|---|---|---|
| GET / POST | `/api/connections` | List (without secrets) / create |
| GET / PUT / DELETE | `/api/connections/{id}` | Read / update (empty secret = keep; `clear_password`, `clear_private_key`) / delete |
| POST | `/api/connections/{id}/duplicate` | Copy on the server |
| POST | `/api/connections/{id}/secret` | `{password, purpose: "show"\|"copy"}` → `{username, password, private_key, key_path, source: connection\|vault\|key_store\|bitwarden\|server_key, source_name}` (owner; the WRM or directory password, or a fresh step-up for SSO-only accounts; `allow_secret_export`; audited) |
| POST | `/api/connections/{id}/name-from-hostname` | Reads the server's hostname (SSH) or DNS name and renames the connection → `{name, hostname}`; an unknown host key answers `409` with `hostkey` |
| POST | `/api/connections/bulk` | `{action: "move"\|"delete", ids, folder_id}` |
| POST | `/api/connections/test` | Test a connection (reports host key problems) |
| POST | `/api/hostkeys/accept` | `{host, fingerprint}` accept a new/changed host key |
| GET / POST | `/api/folders` · PUT / DELETE `/api/folders/{id}` | Folders: `{name, parent_id, jump_id, proxy_id}` (owner-scoped, no cycles, max 32 levels; views add full `path`) (defaults of its connections; views add `jump_name`, `proxy_name`) |
| GET / POST | `/api/sessions` · PUT / DELETE `/api/sessions/{id}` | Workspace sessions |
| GET | `/api/config/export` | Export without secrets (with jump hosts and tunnels) |
| POST | `/api/config/export` | `{password}` export with secrets |
| POST | `/api/config/import` | Import → `{imported, skipped, tunnels}` |
| POST | `/api/config/import/mremoteng` | `{xml, password}` import an mRemoteNG `confCons.xml` → `{imported, folders, jump_hosts, tunnels, skipped, notes}`; HTTP 400 with `need_password: true` when a master password is needed |
| POST | `/api/config/import/sshconfig` | `{text, folder}` import an OpenSSH config |
| POST | `/api/config/import/putty` | `{data, folder}` import a PuTTY `.reg` export (`data` = the file, base64; UTF-16 or UTF-8) → also `{proxies, proxy_links}` |

Import files may be up to 20 MB (request bodies of `/api/config/import*` and `/api/inventory/*` up to 32 MB); larger ones get HTTP 413 with the size and the limit.

Connections have `jump_id` (jump host connection id or `null`) and `web_path`; lists also return `route` (e.g. `"vpn-gw → bastion-dc1"`) and `tunnels` (number of configured tunnels).

**Remote desktop**

| Endpoint | Description |
|---|---|
| `/ws/desktop?id&width&height&dpi&tz[&share_token]` | WebSocket (subprotocol `guacamole`) for an RDP/VNC/Telnet connection. WRM does the guacd handshake; the browser then exchanges Guacamole instructions (display ← / input →). First message: the tunnel id; then `wrm-session` (session id, recorded, route) |
| `GET /api/recordings/{id}/guac[?download=1]` | Recording of a remote desktop session (Guacamole protocol stream) |

Connections of type RDP/VNC/TELNET have `options` (see [Remote desktop](#remote-desktop-rdp-vnc-telnet)); sessions in `/api/recordings` have `protocol` and `recording.format` (`asciicast-v2+gzip` or `guacamole+gzip`).

**Quick connect, notes & network tools**

| Endpoint | Description |
|---|---|
| `POST /api/connections/quick` | `{target: "user@host:port" or "rdp://…", protocol?, password?, credential_id?, key_id?, jump_id?}` → the new temporary connection (`temporary: true`) |
| `PUT /api/connections/{id}` | `{…, notes, temporary: false}` — `notes` sets the notes, `temporary: false` keeps a quick connection |
| `GET /api/connections/{id}` | Includes `notes` (lists only have `has_notes`) |
| `POST /api/nettools` | `{tool: ports\|ping\|traceroute\|dns\|http, target, ports?, source_id?}` → `{source, target, ms, ports: [{port, state: open\|closed\|filtered\|error, ms, info}] \| output \| dns: {addresses, cname, mx, txt, ptr} \| http: {status, tls, subject, issuer, not_after, days_left, names, trusted, trust_error}}` |
| `GET /manifest.webmanifest`, `GET /sw.js` | Web app manifest and service worker (installable app) |

**Out-of-band management**

| Endpoint | Description |
|---|---|
| `GET /api/connections/{id}/bmc` | `{type, power: on\|off\|unknown, health, manufacturer, model, serial, bios, cpus, cpu_model, memory_gib, bmc_model, bmc_firmware, power_watts, inlet_c, faults, actions, console}`. A changed certificate answers `502` with `hostkey` |
| `POST /api/connections/{id}/bmc` | `{action: on\|shutdown\|restart\|off\|reset\|cycle\|nmi}` |
| `/ws/ssh?id&…&console=bmc\|sol` | The serial console of the connection's server (same protocol as terminals) |

Connections have `bmc: {type: redfish|ipmi, host, username, password (write-only), credential_id, via_jump, console, ssh_port}` (`{type: ""}` removes it; views add `has_password`, `credential_name`). Protocol `SERIAL` connections have the device name as `host` and `options: {baud, data_bits, parity, stop_bits, flow}`.

**Tags & inventory**

| Endpoint | Description |
|---|---|
| `POST /api/connections/bulk` | `{action: "tag", ids, add: [tags], remove: [tags]}` → `{changed}` |
| `POST /api/inventory/parse` | `{file_name, data (base64), sheet, header}` → `{sheets, sheet, encoding, delimiter, headers, rows (preview), total, mapping}` |
| `POST /api/inventory/import` | `{file_name, data, sheet, header, mapping: {field: column}, options}` → import result. Fields: `name host port protocol username password folder tags env site rack role platform jump web_path` |
| `GET /api/inventory/netbox` | Saved NetBox address and filters (`has_token`, never the token) |
| `POST /api/inventory/netbox/preview` | `{query: {url, token, devices, vms, site, role, tenant, tag, status, host_from, env_field, insecure, remember}}` → `{rows, missing}` |
| `POST /api/inventory/netbox/import` | `{query, rows, options}` → import result (`imported`, `updated`, `skipped`, `notes`) |

`options`: `{default_protocol, username, auth: ""|credential|key, credential_id, key_id, folder_mode: column|site|role|tenant|fixed|none, folder, tags, update}`. Connections have `tags` (array; on PUT, leaving it out keeps them) and, when imported from NetBox, `source: "netbox"`.

**SSH keys & credentials vault**

| Endpoint | Description |
|---|---|
| `GET /api/keys` | Your key store (public keys, fingerprints, `used_by`, `deployed`; never private keys) |
| `POST /api/keys` | `{name, comment, generate: {type: ed25519\|rsa\|ecdsa, bits}}` or `{name, key, passphrase}` (private or public key). `400 {need_passphrase: true}` when the key is encrypted |
| `PUT / DELETE /api/keys/{id}` | Rename `{name}` / delete (`409` while in use) |
| `GET /api/keys/{id}/public` | Public key file (`.pub`) |
| `POST /api/keys/{id}/export` | `{password, passphrase}` → `{private_key}` (re-authentication, policy `allow_secret_export`) |
| `POST /api/keys/{id}/deploy` | `{conn_ids, use_key}` → `{results: [{conn_id, name, ok, already, switched, error}], ok, failed}` |
| `POST /api/keys/{id}/revoke` | `{conn_ids}` → `{results: [{conn_id, name, ok, removed, error}], ok, failed}` |
| `GET /api/connections/{id}/authorized-keys` | `{user, path, entries: [{line, key_type, bits, fingerprint, comment, options, key_id, key_name, current}], login_key}` |
| `DELETE /api/connections/{id}/authorized-keys?fingerprint=SHA256:…` | Remove that key from the server (`409` for WRM's own login key) |
| `GET /api/credentials` | Credentials you own, are granted or are shared with everybody (no secrets) |
| `POST /api/credentials` · `PUT / DELETE /api/credentials/{id}` | `{name, username, password, key_id, description, hosts, rotate_days, grants: [user ids], shared_all}`; empty `password` on PUT keeps it (owner only) |
| `GET /api/credentials/{id}/usage` | Connections that use it |
| `POST /api/credentials/{id}/reveal` | `{password}` → `{password, pending_password}` (owner, re-authentication) |
| `POST /api/credentials/{id}/rotate` | `{mode: check\|rotate, new_password}` → starts a background job |
| `GET /api/credentials/{id}/rotation` | `{running, stage: preflight\|change\|rollback\|done, ok, message, hosts: [{key, connections, state, error}]}` |

**Proxies**

| Endpoint | Description |
|---|---|
| `GET /api/proxies` | `{proxies: [...], can_define, container}`: proxies you own, are granted or are shared with everybody (no secrets; `url`, `warning: docker_loopback`) |
| `POST /api/proxies` · `PUT / DELETE /api/proxies/{id}` | `{name, kind: socks5\|socks4\|http\|wrm_tunnel, host, port, username, password, credential_id, remote_dns, tunnel_id, description, grants, shared_all}`; empty `password` on PUT keeps it (owner only; policy `proxies`); DELETE is refused (409) while connections or folders use it |
| `POST /api/proxies/{id}/test` | `{target?: "host:port"}` → `{ok, message, latency_ms}`: connects to the proxy (and through it to the target) |

Connections have `proxy_id` (and `jump_id`): `null` = as the folder, `-1` = none, an id otherwise. Views return the effective `jump_id` / `proxy_id`, the stored `jump_choice` / `proxy_choice`, `jump_inherited` / `proxy_inherited`, `proxy_name` and the `route`. Exports name proxies (`proxy_ref`, and a `proxies` list).

**Notifications**

| Endpoint | Description |
|---|---|
| `GET /api/notify` | `{enabled, events, channels: [{id, name, kind, user_address, address_field}], subscriptions: [{event, channel_id, address}], prefs: {quiet_enabled, quiet_start, quiet_end, tz, lang}, pending}` |
| `PUT /api/notify` | `{subscriptions: [...], prefs: {...}}` replaces your subscriptions and quiet hours |
| `POST /api/notify/test` | `{channel_id, address}` a test message to you (once every 10 s) |
| `GET / POST /api/admin/notify/channels` | channels (secret fields empty, `secrets_set`), channel types and their fields · create `{name, kind, enabled, user_address, config: {…}}` |
| `PUT / DELETE /api/admin/notify/channels/{id}` | change (empty secret fields keep the stored value; `clear: [field]` removes one) · delete |
| `POST /api/admin/notify/channels/{id}/test` | `{address?}` send a test message now (HTTP 502 with the service's error) |

Events: `status.down`, `status.up`, `credential.rotation_due`, `credential.rotation_incomplete`.

Connections with `auth_method` `KEY_REF` have `key_id`, with `CREDENTIAL` a `credential_id` (views add `key_name`, `credential_name`, `credential_user`). Exports name keys and credentials (`key_ref`, `credential_ref`) instead of containing them; an import uses your key or credential with that name.

**Snippets & status**

| Method | Endpoint | Description |
|---|---|---|
| GET / POST | `/api/snippets` | My snippets and shared snippets / create `{name, command, description, group, scope: "all"\|"folder"\|"connection", scope_id, auto_run, shared}` |
| PUT / DELETE | `/api/snippets/{id}` | Update / delete (owner; administrators also shared snippets) |
| GET / POST | `/api/bookmarks` | My folder bookmarks / create `{name, path, scope: "global"\|"folder"\|"tag"\|"connection", scope_id, tag, color, note, start_dir, shared}` |
| GET | `/api/bookmarks?conn={id}` | Bookmarks for one connection with `resolved` path and `cd` command, plus `start` (also with `share_token`: the owner's shared ones) |
| POST | `/api/bookmarks/resolve` | `{id, conns: [...]}` → `{conn_id: {path, cd}}` (a broadcast group) |
| POST | `/api/bookmarks/reorder` | `{ids: [...]}` |
| POST | `/api/bookmarks/import-winscp` | Body: a `WinSCP.ini` → `{found, imported, skipped}` |
| PUT / DELETE | `/api/bookmarks/{id}` | Update (rename, move between scopes) / delete |
| GET | `/api/status` | `{enabled, interval, jump_checks, connections: {id: {state: "up"\|"down"\|"unknown", latency_ms, since, checked_at, error, banner, via, via_state}}}` |
| POST | `/api/status/check` | `{ids: [...]}` check now, also through jump hosts (once every 3 s per user) |

Connections also have `monitor` (live status on/off). The terminal WebSocket accepts `{"type":"broadcast","on":true\|false,"peers":n}` (audit) and sends `{"type":"autorun","snippets":[…]}` after run-on-connect snippets ran.

**Git workspace**

| Method | Endpoint | Description |
|---|---|---|
| GET | `/api/git` | Workspace state: `catalog`, `settings`, `sources` (no tokens), `targets`, `installs`, `bundle_apps`, `last_check_at`, `can_check` |
| PUT | `/api/git/settings` | `{mode: "auto"\|"live"\|"bundle", source_id, bundle_id, ref_mode: "catalog"\|"default"\|"branch", ref_branch, ref_overrides: {app: ref}, history_depth, conn_ids, monitor}` |
| PUT | `/api/git/catalog` | The whole catalog (catalog JSON) |
| POST | `/api/git/catalog/import?mode=replace\|merge` | Import catalog JSON |
| GET | `/api/git/catalog/export` | Catalog JSON (download) |
| PUT / DELETE | `/api/git/catalog/apps/{name}` | Add or change / delete one service |
| POST | `/api/git/sources` | `{kind: "gitlab"\|"github"\|"gitea", name, url, token}`; `PUT` / `DELETE /api/git/sources/{id}` (an omitted `token` keeps the stored one) |
| POST | `/api/git/sources/{id}/test` | `{projects}` of the catalog's groups |
| POST | `/api/git/suggest` | `{project, ref}` → `{app, files}` (catalog entry from the repository tree) |
| POST | `/api/git/bundles` | Body: a bundle `.tar.gz` → `{result: {source, files, payload, warnings}, state}` |
| POST | `/api/git/bundles/export` | `{apps: [...]}` → bundle `.tar.gz` |
| POST | `/api/git/targets/refresh` | `{apps}` (empty = all): resolve refs and build targets |
| GET | `/api/git/targets/{app}` | Target with files and per-file history |
| POST | `/api/git/check` | `{conn_ids (empty = the chosen servers), discover, refresh}` → `{result: {servers, installs, targets}, state}` |
| POST | `/api/git/installs` | `{conn_id, path, app, env}` (added by hand) |
| GET / DELETE | `/api/git/installs/{id}` | Details with file states / forget |
| GET | `/api/git/installs/{id}/diff?path=` | `{lines: [{op: " "\|"-"\|"+"\|"@", a, b, s}], state, tracked, informational, reason, pattern, same, binary, too_big, missing_on_server, truncated}` (server → target; any file in the repository except protected ones) |
| GET | `/api/git/installs/{id}/configuration` | On-demand masked configuration/settings, owner and `git_checks` enforced |
| GET | `/api/git/installs/{id}/files` | Every file of the folder and of the repository: `{files: [{path, kind, size, mtime, link, outside, on_server, in_git, tracked, state, reason, pattern, diffable, viewable}], truncated, git_partial, not_compared, target}` |
| GET | `/api/git/installs/{id}/file?path=&side=server\|git` | One file for viewing: `{side, size, hash, text \| binary \| too_big}` |
| GET | `/api/git/installs/{id}/commits` | `{from, to, count, commits: [{sha, title, author, date}]}` between `VERSION.md`'s commit and the target |
| POST | `/api/git/check/jobs` | `{install_ids, conn_ids, all, discover, refresh, stale_minutes, only_never}` → a check job (one at a time) |
| GET | `/api/git/check/jobs/current?since=N` | `{job: {id, state, phase, total, done, seq, items: [{install_id, state, error}], installs}}` (installations finished after `N`) |
| POST | `/api/git/check/jobs/current/cancel` | Cancel the running check job |
| POST | `/api/git/plan` | `{kind: "update"\|"upgrade"\|"stamp", ref, install_ids}` → `{items: [{install_id, prod, units, from_version, target, current, files: [{path, state, selected, local}], removed, counts, eligible}]}` |
| GET | `/api/git/installs/{id}/backups` | Backups in `.deploy-bak` with their files, `VERSION.md` and `updates.jsonl` line (policy `git_rollback`) |
| POST | `/api/git/runs` | Start or schedule a run: `{kind: "update"\|"upgrade"\|"rollback"\|"stamp"\|"restart", ref, items: [{install_id, files, modified_ok, backup}], restart: {mode: "none"\|"now"\|"at"\|"delay", at, delay_minutes}, schedule_at, checks: {service: command}, ignore_imports, force, stamp_partial, set_ref_override, confirm: {install_id: server name}}`. Install / transfer (right away): `{kind: "install"\|"transfer", provision: {conn_id, services: [{app, ref: ""\|"bundle:<id>"\|branch\|"tag:…", path, env, files: [{path, mode: "template"\|"copy"\|"manual"\|"skip", template, values: {field id: {value} \| {cred_id, part}}, from_install, content}], unit: {kind: "systemd"\|"supervisor", name, user, command, work_dir, enable}}], source_install, path, env, unit, overwrite, confirm: server name}, restart: {mode: "none"\|"now"}, checks}` |
| POST | `/api/git/provision/prepare` | `{kind: "install", conn_id, services: [{app, ref, path}], overwrite}` → `{roots, prod, services: [{target, files, bytes, check: {exists, existing, files, version, parent, writable, free_kb, need_kb, tools, needs, errors, warnings}, slots: [{path, protected, in_repo, templates: [{path, available, fields: [{id, name, kind, default, secret}]}], copies}]}]}`; `{kind: "transfer", conn_id, source_install, path, overwrite}` → `{source: {files, bytes, excluded, excluded_list, units, version}, check}` |
| GET | `/api/git/runs` | History and scheduled runs |
| GET | `/api/git/runs/{id}` | A run with live progress: items with `state`, `step`, `log`, `written`, `deleted`, `made_backup`, `dangling`, `health` |
| GET | `/api/git/gitignore/{app}` | .gitignore helper: `{repo, project, branch, file, current, has_current, note, extra: [{path, size, servers, pattern}], stacks: [{name, detected, reason, patterns}], protected, exclude, warnings: [{path, reason, template}], can_mr, mr_note, source_kind}` |
| POST | `/api/git/gitignore/{app}/preview` | `{patterns}` → `{content, added, lines, has_current, file}` |
| POST | `/api/git/gitignore/{app}/download` | `{patterns}` → the new `.gitignore` (download) |
| POST | `/api/git/gitignore/{app}/mr` | `{patterns, confirm: project}` → `{url, branch, kind: "merge_request"\|"pull_request"}` (policy `git_gitignore_mr`, write token) |
| POST / DELETE | `/api/git/sources/{id}/hook` | Turn the webhook of a source on (→ `{hook_id, path, secret}`, the secret only now) or off |
| POST | `/api/hooks/git/{hook_id}` | Incoming webhook (no session; GitLab `X-Gitlab-Token`, GitHub `X-Hub-Signature-256`, generic `X-WRM-Token` or `X-WRM-Signature` + `X-WRM-Timestamp`) → `202 {queued, services}` |
| POST | `/api/git/feeds` | `{name, url, header_name, header_value, interval_minutes}`; `PUT` / `DELETE /api/git/feeds/{id}` |
| POST | `/api/git/feeds/{id}/fetch` | Fetch and import now → `{status: "imported"\|"unchanged", result, state}` |
| PUT / DELETE | `/api/git/pipelines/{app}` | Deploy method *CI pipeline*: `{kind: "jenkins"\|"gitlab", url, username, project, ref, token}`; `DELETE` → files over SSH again |
| POST | `/api/git/env/adopt` | Save the detected (derived) environments of one service or all to the catalog (`{app?}`, policy `git_update`, audit `git.env_adopted`): `{added, environments}` |
| GET | `/api/git/env/overview` | Services with environments (`?auto=1` also merges the environments derived from detected installs, flagged `auto`, plus `auto` count and `hosts`: connection name → hostname): `{services: [{app, envs: [{name, destinations, confirm, allowed_refs, post_deploy}]}], envs, stale_minutes}` |
| GET | `/api/git/env/cell?app=&env=` | One overview cell: `{dests: [{server, path, exists, current, version_md, commit_md, ref_md, user_md, updated_md, behind, lock, error}], drift, target}` |
| POST | `/api/git/env/plan` | `{app, env, ref}` → `{id, ref_name, target, confirm, post_deploy, excluded: {path: {by, at}}, errors, dests: [{index, server, path, state_dir, exists, current, from_version, files: [{path, state: "new"\|"changed"\|"eol"\|"same"\|"removed"\|"extra"\|"protected"\|"ignored", local, link, behind, tag, pattern}], counts, conflicts: [{path, kind, server}], fingerprint, lock, errors, warnings}]}` (kept in memory for 2 hours, applied once) |
| POST | `/api/git/runs` (environment) | Deploy: `{kind: "update", env: {app, env, plan_id, exclude: [path], delete_removed, post_deploy, dry_run, confirm: environment name}, restart: {mode: "none"\|"now"}, checks}`. Rollback: `{kind: "rollback", env: {app, env, deploy_id, dests: [index], post_deploy, dry_run, confirm}, restart}` |
| GET | `/api/git/env/history?app=&env=` | `{dests: [{server, path, current, error}], entries: [{action: "DEPLOY"\|"ROLLBACK", id, rolled_back, by, at, version, commit, branch, previous, new, changed, deleted, excluded, restored, moved_aside, skipped, counts, result, post_deploy, server, path, dest}]}` |
| POST | `/api/git/env/doctor` | `{app, env}` → `{dests: [{server, path, checks: [{name, status: "ok"\|"warn"\|"fail"\|"info", msg}]}]}` |
| POST | `/api/git/env/unlock` | `{app, env, dest, lock_id}`: remove a stale lock (checked again on the server; `409` when it changed) |
| POST | `/api/git/env/exclude` | `{app, env, path, scope: "service"\|"env"}` → `{pattern, added}` (anchored, escaped; not added when a pattern covers the file) |
| DELETE | `/api/git/env/exclusions?app=&env=[&path=]` | Clear the remembered per-run exclusions |
| GET | `/api/git/activity?app=&branch=&author=&since=&until=&refresh=1` | Pushes and commits of a service: `{app, project, provider, since, until, branches, tracked, items: [{kind: "commit"\|"push", at, author, login, branch, branches, ref_type, ref, sha, title, action, count, before, after, url}], authors: [{key, commits, pushes, branches, last}], by_branch: [{key, commits, pushes, authors, last, url}], truncated, rate_limited, rate_reset, notes, fetched_at, cached, calls}` (dates `YYYY-MM-DD`, default the last 30 days) |
| GET | `/api/git/activity/export.csv?…` | The same items as CSV |
| GET | `/api/git/history/runs?<filters>` | WRM runs of installations without environments: `{rows: [{at, action, source: "run", app, env, server, path, by, branch, version, commit, id, run_id, result, previous, counts, files, error, note, commit_url}]}` |
| GET | `/api/git/history/cell?app=&env=&<filters>&refresh=1` | `history.jsonl` of every destination of an environment (policy `git_checks`): `{app, env, rows, errors, at}` |
| GET | `/api/git/history/export.csv?<filters>` | Everything as CSV (filters: `app`, `env`, `server`, `who`, `branch`, `version`, `action` = `deploy`\|`rollback`\|`update`\|`upgrade`\|`install`\|`transfer`, `since`, `until`) |
| GET | `/api/git/where/cell?app=&env=&refresh=1` | The destinations of an environment (policy `git_checks`): `{rows: [{app, env, server, path, kind: "env", exists, branch, commit, version, by, at, deploy_id, head, behind, commit_url, compare_url, branch_url, drift, locked, error}], drift, note}` |
| GET | `/api/git/where/installs?app=&refresh=1` | Installations of a service without environment, from their last check: `{rows: [{… kind: "install", state}]}` |
| GET | `/api/git/where/export.csv?app=&env=&server=` | The matrix as CSV |
| POST | `/api/git/runs/{id}/cancel` | Cancel a scheduled run (with its waiting restart) or stop a running one after the current installation |

**Tunnels & web interfaces**

| Method | Endpoint | Description |
|---|---|---|
| GET / PUT | `/api/connections/{id}/tunnels` | Tunnels of a connection / replace them: `[{id?, name, kind: "local"\|"remote"\|"dynamic", bind_host, bind_port, target_host, target_port, open_scheme, open_path, start_mode: "manual"\|"connect"\|"always"}]` |
| GET | `/api/tunnels[?all=1]` | `{tunnels: [...], policy}` my tunnels with state (`running`, `state`, `listen`, `active`, `total`, `bytes_up`, `bytes_down`, `started_by`, …); administrators: `all=1` → every running tunnel |
| POST | `/api/tunnels/{key}/start` · `…/stop` | Start (owner) / stop (owner or administrator) |
| POST | `/api/connections/{id}/open-web` | Web interface connection → `{url, direct, route}` (opens a temporary tunnel when it has a jump host) |

**Remote files** (`id` = connection id; for shared connections add `share_token`; the role must allow the action)

| Method | Endpoint | Description |
|---|---|---|
| GET | `/api/remote/list?id&path` | List a folder |
| GET | `/api/remote/list-recursive?id&path` | All files below a path |
| GET | `/api/remote/search?id&path&q&mode=name\|content&max&hidden` | Search (NDJSON stream) |
| GET / HEAD | `/api/remote/preview?id&path` | Inline media/PDF with byte ranges; `metadata=1` returns kind/MIME/size/language, `text=1` returns bounded, content-detected text with a Prism language id |
| GET | `/api/remote/download?id&path` | Download a file |
| GET | `/api/remote/download-dir?id&path` | Download a folder as ZIP (SFTP) |
| POST | `/api/remote/upload?id&path&overwrite` | Multipart upload, streamed |
| POST | `/api/remote/mkdir?id&path` | Create folder |
| POST | `/api/remote/rename?id&old&new` | Rename / move |
| DELETE | `/api/remote/delete?id&path&dir=0\|1` | Delete file or folder |
| POST | `/api/remote/transfer` | Server-to-server transfer (NDJSON progress) |

**Sharing**

| Method | Endpoint | Description |
|---|---|---|
| GET / POST | `/api/shares` | My shares / create `{name, connection_ids, folder_ids, access_mode, guest_role, members:[{username, role}], password, public, expires_at}` |
| GET / PUT / DELETE | `/api/shares/{id}` | Details / update (also `active`, `clear_password`) / delete |
| POST | `/api/shares/{id}/rotate` | New link |
| GET / POST | `/api/shares/{id}/members` · PUT / DELETE `…/members/{userID}` | Members and roles |
| GET | `/api/shares/{id}/participants` · PUT `…/participants/{pid}` | People history; `{role, banned}` |
| GET | `/api/share/{token}` | Share info, my role and permissions, connections |
| POST | `/api/share/{token}/unlock` · `…/profile` | Password unlock · guest name |
| GET | `/api/shared` | “Shared with me” |

**Administration** (administrators)

| Method | Endpoint | Description |
|---|---|---|
| GET / POST | `/api/admin/users` | Users (with `role`, `break_glass`, `teams`) / create `{username, password, role, …}` |
| PUT / DELETE | `/api/admin/users/{id}` | `{role, is_admin, break_glass, disabled, display_name}` / delete; `?transfer_to=<id>` hands what the user owns to that account first |
| GET | `/api/admin/users/{id}/handover` | What the user owns `{owned, suggested_user_id, suggested_username, suggested_reason, teams}` |
| GET / POST | `/api/admin/oidc` | Identity providers (never the client secret), linked accounts and the redirect address / create a provider `{name, issuer, client_id, client_secret, scopes, redirect_uri, username_claim, require_mfa, mfa_values, jit, rules: [{claim, value, role, teams}], enabled}` (step-up) |
| PUT / DELETE | `/api/admin/oidc/{id}` | Change a provider (fields optional; an empty `client_secret` keeps the stored one, `clear_secret` removes it) / remove it with its links (step-up) |
| POST | `/api/admin/oidc/check` | `{issuer}` step-by-step connection test |
| POST | `/api/admin/oidc/{id}/preview` | `{claims, rules?}` the role and teams WRM would give |
| DELETE | `/api/admin/oidc/identities/{id}` | Unlink a directory account (step-up) |
| GET / POST | `/api/admin/ldap` | LDAP directories (never the service account password), linked accounts and `sync_minutes` / create a directory `{name, kind: ad\|ldap, url, ca_pem, bind_dn, bind_password, base_dn, user_filter, login_attr, id_attr, group_base, nested, jit, rules, enabled}` (step-up) |
| PUT / DELETE | `/api/admin/ldap/{id}` | Change a directory (fields optional; an empty `bind_password` keeps the stored one unless `url` changes, `clear_bind_password` removes it) / remove it with its links (step-up) |
| POST | `/api/admin/ldap/check` | `{…directory fields, directory?, username?}` step-by-step connection test (`directory` uses that directory's stored password) |
| POST | `/api/admin/ldap/{id}/test` | `{username, password}` test sign-in: account, groups, attributes and the mapping, nothing is created |
| POST | `/api/admin/ldap/{id}/sync` | Check the directory's linked accounts now: `{checked, ended, updated, error}` |
| DELETE | `/api/admin/ldap/identities/{id}` | Unlink a directory account (step-up) |
| GET / POST | `/api/admin/scim` | SCIM clients, provisioned accounts and the endpoint / create a client `{name, rules}`: `{id, token, endpoint}`, the token is shown once (step-up) |
| PUT / DELETE | `/api/admin/scim/{id}` | Change a client's name or rules / revoke it (step-up) |
| POST | `/api/admin/scim/{id}/rotate` | New token for a client (step-up) |
| GET / POST | `/api/admin/teams` | Teams with members and the policies that name them / create `{name, description, members: [{user_id, lead}]}` |
| PUT / DELETE | `/api/admin/teams/{id}` | Change (same fields, each optional) / delete; 409 while a policy names the team |
| POST | `/api/admin/users/{id}/reset-password` · `reset-2fa` · `revoke-sessions` · `unlock` | Account actions |
| POST | `/api/admin/users/{id}/stop` | Stop user: disable and end everything (step-up): `{ok, user, ended: {sessions_ended, terminals_ended, rooms_left, tunnels_stopped, ai_sessions_ended, ai_connections_revoked, disabled}}` |
| GET / PUT | `/api/admin/settings` | Policies; `GET` adds each setting's `required` value and `deviation`; `PUT` takes `_deviation_notes: {key: reason}`, without a reason a value below the level → 400 `{error, deviation: {key, value, required, level}}` |
| GET / POST | `/api/admin/lockdown` | Lockdown state `{on, reason, by, at, source, env, break_glass_admins}` / `{on, reason}` turn it on (reason required, fresh step-up; adds `ended` and `you_signed_out`) or off (break-glass administrators only) |
| GET / POST | `/api/admin/siem` | SIEM destinations with status (`state`, `sent`, `failed`, `waiting`, `last_ok_at`, `last_error`, `retry_at`; never the secret), `kinds`, recent `alerts` and `hostname` / create `{name, kind, url, config: {index, sourcetype, tenant_id, client_id, dcr_id, stream, facility, ca_pem, client_cert, actions}, secret, enabled, start_from: now\|all}` (step-up) |
| PUT / DELETE | `/api/admin/siem/{id}` | Change a destination (an empty `secret` keeps the stored one unless the address or kind changes, `clear_secret` removes it) / delete it (step-up) |
| POST | `/api/admin/siem/{id}/test` | Send a test event `{ok, error}` |
| GET | `/api/admin/security` | Security posture: `{level, level_locked, level_env, levels, rules, deviations, checks: [{id, kind, status: ok\|warn\|bad\|info, value, required, recommended, env, count, deviation}], summary, version, host, generated_at}` |
| POST | `/api/admin/security/level` | `{level, preview, keep: {key: reason}, apply_now}` → the plan `{from, to, changes, forced, kept, blockers, removed, affected}`; 409 when the level is forced or a blocker is not kept |
| GET | `/api/admin/status` | Status, open terminals, rooms, relay |
| POST | `/api/admin/terminals/{id}/kill` | End a terminal |
| GET | `/api/admin/shares` | All shares |
| GET / DELETE | `/api/admin/known-hosts[/{id}]` | Host keys |
| GET | `/api/admin/audit?q&action&user&conn&session&from&to&before_id&limit[&format=csv]` | Audit log |
| GET | `/api/admin/audit/verify` | Verify the hash chain `{ok, checked, first_id, last_id, unsigned, broken_at, reason}` |
| GET | `/api/admin/transfers?q&user&conn&direction&from&to&before_id&limit[&format=csv]` | File transfers |
| GET | `/api/version` | Server version `{version}`, with `update` when a newer release is known |
| GET | `/healthz` | Health check (no sign-in) |

**Updates** (status for everyone; actions for administrators, policy `self_update`; never in a container)

| Method | Endpoint | Description |
|---|---|---|
| GET | `/api/update` | `{current, latest, available, name, notes, url, published_at, checked_at, check_enabled, docker}`; administrators also `{repo, error, service, platform, policy, can_update, blocked: ""\|"docker"\|"not_writable", docker_image, signature_keys (trusted release keys; 0 = signature not checked), local: {file, version}, rollback, progress: {state: "idle"\|"downloading"\|"verifying"\|"installing"\|"restarting"\|"failed", version, done, total, error}}` |
| POST | `/api/update/check` | Check the GitHub releases now → the status |
| POST | `/api/update/apply` | `{version}`: download, verify (signature `SHA256SUMS.txt.sig` once a release key is known, SHA-256 against `SHA256SUMS.txt`, `-version`), replace and restart in the background; follow `progress` |
| POST | `/api/update/rollback` | Swap back to `<binary>.previous` and restart |
| POST | `/api/update/restart` | `{local: true}`: restart with the newest valid binary in the folder (or the same one without `local`) |

**AI assistant** (signed-in users the policy allows; the connection must be their own or shared with a role that allows terminals and file changes — add `?share_token=` for shared ones)

| Method | Endpoint | Description |
|---|---|---|
| GET | `/api/ai/config` | `{enabled, reason, providers: [{id, name, kind, scope, models, default_model}], modes, default_mode, personal_keys, redact_output, kill_switch, active: [sessions]…}` — never keys |
| GET | `/api/ai/connections/{id}` | `{allowed_modes, default_mode, rules, has_notes, sees}` for a connection (most restrictive rule wins) |
| GET / POST / PUT / DELETE | `/api/ai/providers[/{id}]` | Personal providers (policy `ai_personal_keys`); secrets are write-only (`api_key`, `access_key_id`, `secret_access_key`, `session_token`, `service_account`; omitted = unchanged, `""` = removed), responses carry `has_api_key` / `has_access_key` / `has_service_account` |
| POST | `/api/ai/providers/{id}/test` | One tiny request `{model}` → `{ok, reply \| error}` |
| GET / POST | `/api/ai/sessions` | Own sessions (`?purpose=terminal\|logs\|file&conn_id=`) / start `{conn_id, provider_id, model, mode, purpose?: "logs"\|"file", file_path?, resume_from?, auto: {allow, deny, path_allow, path_deny, minutes, max_actions}, confirm_auto, share_notes, engine?: "opencode", sandbox?: true, agent?: "build"\|"plan"}` |
| GET | `/api/ai/sessions?view=all` | The AI sessions view: own sessions of every kind `?kind=assistant\|logs\|file\|opencode\|mcp\|saved\|live&q=&conn_id=&before=&limit=` → `{sessions (with saved, preview, kind, can_continue), more, keep_saved, retention_days}` |
| GET | `/api/ai/sessions/{id}` | Session and its (redacted) transcript |
| POST | `/api/ai/sessions/{id}/save` | `{saved}`: keep an own session beyond the retention |
| GET | `/api/ai/sessions/{id}/export?format=md\|json` | Download an own session's transcript |
| GET | `/api/ai/sessions/{id}/events?since=N` | Server-sent events: `session`, `user`, `assistant_start`, `text`, `assistant_done`, `tool_call`, `approval_required`, `approval_resolved`, `tool_result`, `mode`, `usage`, `busy`, `notice`, `error`, `thinking`, `title`, `history`, `ended` (an ended session answers with `ended` and `resumable`) |
| POST | `/api/ai/sessions/{id}/prompt` | `{text}` |
| POST | `/api/ai/sessions/{id}/title` | `{title}`: rename a chat |
| POST | `/api/ai/sessions/{id}/mode` | `{mode, auto, confirm}` |
| POST | `/api/ai/sessions/{id}/approvals/{approval}` | `{decision: "approve"\|"deny", command?, content?, note?}` (only the session's user); a request that no longer waits answers `409 {error, decided: {outcome, by, via, at}}` |
| POST | `/api/ai/sessions/{id}/stop` · `/kill` | Cancel the running request / end the session |
| GET / PUT | `/api/ai/delegate` | Helper models for the user (provider, models with data-use labels, allowed or not) / `{training: bool}` (only with `ai_delegate_training` = `user`) |
| GET | `/api/admin/opencode` | OpenCode state: installed version, binary, package for this host, running agents, install log (administrators) |
| POST | `/api/admin/opencode/install` | `{confirm: true}`: download, verify (sha512) and install OpenCode in the background (administrators) |
| POST | `/api/ai/kill-all` | End all own sessions |

**AI connections (MCP)** — the MCP endpoint takes a bearer token, the OAuth endpoints are public, the rest needs a WRM sign-in

| Method | Endpoint | Description |
|---|---|---|
| POST / DELETE | `/mcp` | MCP streamable HTTP (protocol versions `2025-11-25`, `2025-06-18`, `2025-03-26`): JSON-RPC `initialize` (answers `Mcp-Session-Id`), `ping`, `tools/list`, `tools/call` (JSON, or an event stream that can carry `elicitation/create` and `notifications/progress`), `notifications/cancelled`; DELETE ends the MCP session. `Authorization: Bearer wrm_pat_…` / `wrm_oat_…`; without one: 401 with `WWW-Authenticate: Bearer resource_metadata="…"`. Requests with a foreign `Origin` are refused (403) |
| GET | `/.well-known/oauth-protected-resource[/mcp]` | RFC 9728 metadata: resource, authorization server, scopes |
| GET | `/.well-known/oauth-authorization-server` | RFC 8414 metadata |
| POST | `/oauth/register` | RFC 7591 dynamic client registration `{client_name, redirect_uris, token_endpoint_auth_method}` (redirect URIs from `ai_mcp_redirect_uris` only) |
| GET | `/oauth/authorize` | `response_type=code`, `client_id`, `redirect_uri`, `code_challenge` + `code_challenge_method=S256` (required), `state`, `scope`, `resource` → WRM's consent dialog |
| POST | `/oauth/token` | `grant_type=authorization_code`, `code`, `redirect_uri`, `client_id`, `code_verifier` → `{access_token, token_type: "Bearer", expires_in, scope}` (no refresh tokens) |
| POST | `/oauth/revoke` | RFC 7009 `token` |
| GET | `/api/mcp/config` | What the user may choose: `{enabled, reason, connections, modes, scopes, default_hours, max_hours, url, oauth, elicitation, instructions, description, version}` |
| GET / POST | `/api/mcp/grants` | Own AI connections / create a token `{name, conn_ids, mode, scopes, hours, auto, confirm_auto}` (`auto.until_grant`: no time or action limit, since v12.6.1) → `{grant, token, url}` (the token only here) |
| DELETE | `/api/mcp/grants/{id}` | Revoke (ends its sessions) |
| POST | `/api/mcp/revoke-all` | Revoke every own AI connection |
| GET | `/api/mcp/active` | `{live: [AI connections with open sessions], pending: [approvals]}` for the indicator; decide with `/api/ai/sessions/{id}/approvals/{approval}` |
| POST | `/api/mcp/claude-desktop` | The Claude Desktop extension `wrm-pro.mcpb` for this WRM `{token}` (optional; only a `wrm_pat_…` token is accepted) → `application/zip`. Audited as `mcp.desktop_extension` |
| POST | `/api/mcp/full-access` | The user's own full access `{on}`: turning it on needs `ai_full_access` on and `ai_full_access_mode` = `user` (403 otherwise), turning it off is always allowed. Audited as `mcp.full_access` |
| GET / POST | `/api/mcp/authorize/{id}` | The consent dialog: the request (`client_name`, `redirect_host`, `scopes`) / `{decision: "approve"\|"deny", conn_ids, mode, scopes, hours}` → `{redirect}` |
| GET / DELETE | `/api/admin/mcp/grants[/{id}]`, POST `/api/admin/mcp/revoke-all` | Every AI connection; revoke one or all (administrators) |
| GET / DELETE | `/api/admin/mcp/clients[/{id}]` | Registered OAuth apps; removing one revokes its AI connections |
| GET / POST / PUT / DELETE | `/api/admin/ai/providers[/{id}]`, POST `…/{id}/test` | Organisation providers (administrators) |
| GET | `/api/admin/ai/providers/{id}/models` | The provider's own model list (`GET {base}/models`) → `{ok, models \| error}`; audited |
| GET | `/api/ai/ollama/status[?url=]` | Ollama detection `{running, version, models, binary, os, steps, can_manage, openai_base}` (`url` is admin only) |
| POST | `/api/ai/ollama/action` | `{action: install\|start\|pull, model, url, confirm: true}` → `{run_id}`; administrator + policy `ai_ollama_manage`, fixed commands, one run at a time |
| GET | `/api/ai/ollama/runs[/{id}]` | Last runs with output (administrators) |
| GET | `/api/admin/ai/sessions?user&conn&status&limit`, `/api/admin/ai/sessions/{id}` | Every AI session; a transcript (viewing is audited) |
| POST | `/api/admin/ai/sessions/{id}/kill`, `/api/admin/ai/users/{id}/kill`, `/api/admin/ai/kill-all` | Kill switch per session, per user, for everyone |
| PUT | `/api/admin/ai/users/{id}` | `{blocked}`: turn the assistant off (and end the sessions) for a user |
| GET | `/api/admin/ai/summary` | Usage per user, active sessions, the destructive rules |

**Terminal sessions & recordings** (administrators: all; users: own sessions and sessions on their connections)

| Method | Endpoint | Description |
|---|---|---|
| GET | `/api/recordings?mine&q&user&conn&host&status&from&to&before_id&limit` | Terminal sessions with recording info |
| GET | `/api/recordings/{id}` | One session and its audit entries |
| GET | `/api/recordings/{id}/cast[?download=1]` | The recording as asciicast v2 (`application/x-asciicast`) |

**WebSockets**

| Endpoint | Description |
|---|---|
| `/ws/ssh?id&cols&rows[&share_token]` | Terminal. Binary frames = terminal data; text frames = JSON control (`resize`, `pause`, `resume`, `ping`, `cwd` (the shell's working directory) → server; `cwd` with `path` or `error`, `status` (with `recording`, `session_id`), `error`, `exit`, `hostkey` ← server). Close codes: 1000 shell exited, 4001 connect failed / not allowed, 4002 SSH connection lost, 4003 access revoked |
| `/ws/events` | Live notifications for the signed-in user (`sessions_changed`, `tunnels_changed`, `snippets_changed`, `bookmarks_changed`, `status_changed`, `ai_sessions_changed`, `mcp_changed`) |
| `/ws/share/{token}` | Collaboration room. Client → server: `chat`, `file`, `set-name`, `screen` (`start/stop/data/resize/snapshot`), `watch`, `unwatch`, `control-request/grant/revoke/release`, `remote-input`, `voice-join/leave/signal/state`, `hand`, `mod` (`set-role/mute/unmute/stop-share/lower-hand/kick/ban`). Server → client: `welcome`, `participants`, `chat`, `file`, `system`, `screen`, `watch-request`, `control`, `control-request`, `remote-input`, `voice-peers/joined/left/signal`, `force-mute`, `role`, `kicked`, `closed`, `error` |

---

## Building from source & releases

Requirements: **Go** (version in `remote-manager/go.mod`). No C compiler is needed (pure-Go SQLite).

```bash
git clone https://github.com/vedranius/wrm-pro-enterprise.git
cd wrm-pro-enterprise/remote-manager
go test -race -count=1 -timeout 45m ./...
go build -o wrm-server .
./wrm-server
```

Cross-compile any platform, e.g. `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o wrm-linux-arm64 .`, or build all 13 targets with `bash remote-manager/build-all.sh`.

**Project structure**

```
remote-manager/
  main.go         HTTP server, routes, database schema & migrations, connections, folders, sessions, transfer
  security.go     server secret, encryption key management, security headers, CSRF, self-signed TLS, validation
  users.go        sign-in, 2FA step, sessions, password change, account administration
  totp.go         TOTP (RFC 6238), QR code, recovery codes
  settings.go     admin policies (database + WRM_<KEY> environment overrides)
  addons.go       addons: install, enable, assets; addons_v2.go: manifest v2, personal addons, instances, store, windows, CSP, integrity, permission sheets; addons_http.go: web requests for addons (allowlist, SSRF guard, secrets, approvals, log); addons_lint.go: addon lint and diagnostics; addons_dev.go: wrm -addon-dev and /api/addons/dev; addons_templates.go + addon_templates/: starter templates, wrm-bridge.js, schema, llms.txt
  audit.go        audit log (hash chain, append-only triggers), CSV export, integrity check, retention
  recording.go    terminal sessions, asciicast recorder, file transfer log, recordings & transfers API
  hostkeys.go     SSH host key verification (TOFU/strict), FTPS certificate pinning
  shares.go       sharing model: access modes, roles, members, participants, authorization
  collab.go       collaboration rooms: presence, chat, terminal sharing, control, voice signalling, moderation
  turn.go         built-in TURN relay for voice (turn_android.go: stub for Android)
  admin.go        open-terminal registry, admin status
  ssh_ws.go       SSH terminal WebSocket (binary frames, flow control, keepalive, reconnect codes)
  termcwd.go      working directory of a terminal's shell over a separate SSH exec channel (📂 Open here)
  service.go      run as a service: prompt, env file, service files, versions, newest binary in the folder, restarts
  service_unix.go / service_windows.go  systemd, launchd, rc.d installers / Windows service (SCM)
  update.go       update check against GitHub releases, verified self-update, rollback, restart
  roles.go        roles, who-policies (operators, teams:), step-up, break-glass, handover
  teams.go        teams, team members, teams in shares, admin team API
  oidc.go         directory sign-in: providers, PKCE sign-in, mapping rules, identities, revalidation, admin API
  oidc_jwt.go     OIDC discovery, signing keys (JWKS) and ID token verification
  ldap.go         LDAP / Active Directory: directories, sign-in, groups and mapping, regular check, admin and self-service API
  ldap_proto.go   LDAPv3 client: BER, LDAPS and StartTLS, bind, paged search, filter compiler
  scim.go         SCIM 2.0 provisioning: Users, Groups, tokens, groups as teams, admin API
  directory.go    endUserAccess: what ends when an account is stopped by any route
  siem.go         SIEM export: destinations, event format, syslog / file / Splunk / Elastic / Sentinel / HTTPS delivery, status, admin API
  alerts.go       security alert rules on the audit log
  lockdown.go     lockdown (UI, wrm -lockdown, WRM_LOCKDOWN) and Stop user
  security_level.go  security levels: rules, preview and apply, deviations, the first-run and start-up level, HTTPS for sign-in
  security_posture.go  the security posture checks for Admin → Security
  releasesign.go  release signatures (ed25519 over SHA256SUMS.txt): verification for the self-update, -release-keygen/-sign/-verify
  ai_engine.go    AI sessions: tool calls through the permission engine, approvals, kill switch, transcript, recording, the agent loop
  ai_policy.go    AI modes, mode rules (most restrictive wins), automatic-mode limits, the decision
  ai_classify.go  read-only command classifier and unreadable sensitive files
  ai_destructive.go  always-blocked destructive patterns
  ai_shell.go     shell command parser and glob matching for the AI checks
  ai_tools.go     the AI tools (fixed read scripts, run_command, file edits with diffs)
  ai_redact.go    redaction of secrets in AI output, transcripts and audit
  ai_llm.go       model calls: Anthropic Messages, OpenAI Chat Completions, Bedrock Converse; streaming, usage, cost
  ai_cloud.go     AWS SigV4, AWS event streams, Google service account tokens
  ai_provider.go  AI providers (organisation / personal), encrypted keys, schema
  ai_api.go       /api/ai and /api/admin/ai
  ai_ollama.go    Ollama detection / install / start / pull (audited), provider model lists
  ai_opencode.go  OpenCode Zen provider: models.dev catalog, free models, data-use labels, API family routing
  ai_delegate.go  delegate_task: helper models chosen by kind of job, fallback, training opt-in, /api/ai/delegate
  opencode_install.go  OpenCode download from the npm registry with sha512 check, /api/admin/opencode
  opencode_agent.go    OpenCode agents: managed opencode serve per session, its config, loopback MCP, event mapping, cleanup
  ai_transfer.go  transfer_file: SFTP copy between two sessions of one AI connection
  mcp_server.go   the MCP server at /mcp: streamable HTTP, JSON-RPC, tools, elicitation, origin and CORS checks
  mcp_tokens.go   AI connections (hashed tokens, scopes, expiry, revoke), policies, /api/mcp and /api/admin/mcp
  mcp_oauth.go    OAuth 2.1 for MCP clients: metadata, dynamic client registration, consent, PKCE, token, revoke
  mcp_stdio.go    the stdio bridge (wrm -mcp-stdio) for clients that only start local servers
  mcp_bundle.go   the Claude Desktop extension (wrm-pro.mcpb), the plugin's bundle (buildPluginMCPB) and the setup script for all AI apps (POST /api/mcp/setup-all): manifest, the Node.js bridge in mcpb/server/index.js, the icon
  files.go        file API: list, download, ZIP, streamed upload, mkdir, rename, delete
  search.go       recursive name / content search (NDJSON stream)
  sftp_pool.go    pooled SFTP connections with liveness checks
  conntest.go     "Test connection" endpoint
  jump.go         jump hosts: chains, dialing through hops (and their proxies), validation
  proxy.go        saved proxies: SOCKS5 / SOCKS4(a) / HTTP CONNECT clients, WRM tunnel proxies, routes, API
  notify.go       notifications: channels (SMTP, chat, push, webhook), subscriptions, digests, quiet hours
  tunnels.go      SSH tunnels (local, remote, SOCKS5), manager, reconnects, tunnels API, web interfaces
  importers.go    import from mRemoteNG confCons.xml and OpenSSH config
  putty.go        import of PuTTY sessions (.reg) with proxies, link to mRemoteNG PuttySession
  snippets.go     snippets (saved commands), variables, run on connect
  bookmarks.go    folder bookmarks: scopes, variables, shell-quoted cd, start directory, WinSCP import
  status.go       live up/down status monitor and API
  git.go          Git workspace: storage, settings, targets, API, diff
  git_source.go   GitLab API v4 / GitHub REST client, tree and blob cache, ref resolution, suggestions
  git_bundle.go   offline bundles: import and export (bundle.json + payload)
  folders.go      nested hierarchy, owner/cycle/depth validation and import ordering
  preview.go      authenticated SFTP/FTP byte-range streaming, content-detected text previews and language hints
  git_functionality.go runtime checks, redacted logs and configuration inspection
  git_scan.go     discovery and file hashing over SSH, VERSION.md, units, comparison
  git_monitor.go  periodic Git checks and their notifications
  git_deploy.go   deploy steps over SSH: backup, atomic writes, checks, rollback, VERSION.md, updates.jsonl, dangling imports, restarts
  git_runs.go     Git runs: plan, rolling execution, scheduling, live progress, policies, notifications, API
  git_provision.go  Git install and transfer: pre-checks, staged tar transfer, units, wizard API
  git_template.go   templates of per-host files: destinations, fields, filling, slots
  git_gitignore.go  .gitignore helper: candidates, warnings, preview, merge / pull request
  git_ci.go         optional CI: webhooks, bundles from CI artifacts, CI pipeline deploy method
  git_files.go      every file of an installation: listing with reasons, view, diff of any repository file, commits behind
  git_jobs.go       check jobs: partial, streamed and cancellable checks with concurrency limits
  git_env.go        environments: validation, plans with fingerprints, locks, server-side state, overview, history, doctor, API
  git_env_run.go    environment deploy and rollback runs: dry run, staged transfer, backup, manifest, history, post_deploy
  git_activity.go   Git activity: pushes and commits per branch and author, paging with rate limits, short cache, web links, CSV
  medium_config.go  Medium native: deploy-config reader and validator (the original tool's rules)
  medium_filter.go  Medium native: rsync exclude rules, allowed_refs matching
  medium_engine.go  Medium native: rsync-compatible diff over SFTP, server state, comparison with the original plan
  medium_native.go  Medium native: WRM Git source, server ↔ connection links, background previews, API
  medium_state.go   Medium native: server state in the original format (current.json, history.jsonl, lock owner), rollback choice
  medium_host.go    Medium native: writing to a destination (SFTP with sh over SSH)
  medium_deploy.go  Medium native: deployment (lock, re-plan, backup, upload, checksum check, post_deploy, history, pruning), jobs
  medium_rollback.go  Medium native: rollback from the backup manifests
  medium_deploy_api.go  Medium native: engine per project, deploy, history, rollback, lock release and job API
  git_medium_docker.go  Medium Docker: per-OS installation (Linux packages, Docker Desktop with signature and licence), consent, -setup-docker
  git_medium_ca.go      Medium certificates: the host's trusted roots (Windows stores, macOS keychains, CA bundle) and medium_ca_file for the Docker runtime
  git_where.go      deploy history (server history + WRM runs) and the "what is where" matrix with commits behind the branch head
  bmc.go          out-of-band management: Redfish and IPMI status and power, certificate pinning
  quick.go        quick connect (temporary connections and their cleanup), connection notes
  nettools.go     network tools: port check, ping, traceroute, DNS, HTTP/TLS check, from WRM or a server
  pwa.go          web app manifest and service worker (installable app)
  console.go      terminal backends: SSH shell, BMC SSH console, IPMI Serial-over-LAN, serial ports
  tty_linux.go    pseudo terminals and serial port settings (Linux); tty_other.go elsewhere
  inventory.go    tags, CSV/XLSX reader with column mapping, NetBox import and sync
  keys.go         SSH key store: generate/import, deploy/revoke (authorized_keys), who has access
  credentials.go  credentials vault: sharing, host lists, password rotation
  guac.go         remote desktop: guacd handshake, relay, recording (RDP / VNC / Telnet)
  helpers.go      origin check, login rate limiting, shared helpers
  security_test.go  unit tests (TOTP, roles, share access, CSRF, encryption migration, settings)
  upgrade_test.go   unit tests (upgrading databases from earlier builds, old URLs)
  integration_test.go  in-process SSH/SFTP server: connect → audit → recording, transfers, append-only audit
  tunnels_test.go      jump host chains, local/remote/SOCKS tunnels, policies, web interfaces
  snippets_test.go     snippets API, sharing, variables, run on connect, broadcast audit
  bookmarks_test.go    folder bookmarks: scopes, ownership, variables, cd quoting, shares, WinSCP.ini, start directory
  status_test.go       live status: up/down, banners, jump hosts, check now
  git_test.go          Git workspace: fake GitLab / GitHub / Gitea, branch-aware tags, blob cache, bundles, catalog, discovery and comparison on a fake SSH host, notifications
  git_deploy_test.go   Git deploy: VERSION.md / updates.jsonl formats, dangling imports; update, CRLF, protected files, stamp, rollback, failed check → rollback, scheduled restart (fake SSH host)
  git_gitignore_test.go  .gitignore helper: candidates from a check, preview, download, merge request only on explicit choice (fake GitLab), pull request (fake GitHub)
  folders_test.go hierarchy, cycle/owner/depth checks and config round trips
  git_functionality_test.go Supervisor group health, log/config redaction and symlink boundaries
  preview_test.go SFTP/FTP ranges, safe text preview and share permissions
  ai_availability_test.go SFTP providers without MCP and cross-window policy events
  git_files_test.go      every file of an installation (reasons, excluded file diff, symlinks, unreadable, quoting, commits) and check jobs (partial, stale, limits, cancel, last good result)
  git_ci_test.go         webhooks (GitLab token, GitHub HMAC, generic token / HMAC, replays, rate limit), artifact feed fetch and schedule, Jenkins job and GitLab pipeline runs
  git_env_test.go        environments: plan with CRLF-only files, dry run, typed confirmation, one-shot plans, fingerprint mismatch, lock contention, stale lock, own lock only, remembered and permanent exclusions, allowed_refs, nested paths, partial transfer, post_deploy failure, rollback (moved aside, symlinks skipped, removed directories, kept backups), doctor, drift
  git_activity_test.go   Git activity against fake GitLab / GitHub / Gitea: paging (X-Next-Page, Link, cursors), tracked branches, filters, author / branch summaries, links, cache, rate limits and 429, CSV escaping
  medium_deploy_test.go  Medium native engine: state format, rollback choice on the original's history, lock, deploy and rollback against real rsync with backups, plan mismatch, exclusions, post_deploy, partial deployment, pruning, deploy over SSH (fake server), API with policy, confirmation and engine checks
  medium_screens_test.go Medium file details (line diff, binary/large/CRLF), eol_only marking locally and over SSH with counts in history, the details API and the deploy-config editor (validation, stale hash, CRLF kept, create/delete, policy)
  medium_config_git_test.go deploy-config from Git with fake GitLab / GitHub / Gitea: pull, local changes, refused pull, push with conflict check and validation, discard, stop
  medium_build_user_test.go build user checks (root, missing account, readable WRM key) and a real build as nobody with a clean environment
  medium_native_test.go  Medium native: deploy-config validation, state paths, exclude rules and the diff against real rsync (fixed cases and random trees), server state, Git sources (fake GitLab / GitHub / Gitea), connection links, comparison run
  git_where_test.go      deploy history merged from two destinations and WRM runs (filters, cache, CSV with file lists, policy), what is where (commits behind the branch head, drift, installations, CSV)
  git_provision_test.go  Git install / transfer: templates, exclusions, units; install from the fake GitLab and from a bundle, vault values, overwrite refusal and backup, transfer A → B, policies
  quick_test.go        quick connect (targets, expiry, open terminals, limit), notes, network tools from WRM and through SSH, PWA endpoints
  bmc_test.go          Redfish (fake BMC, pinning, jump host, credential), IPMI with a fake ipmitool, SOL (local PTY and jump host), BMC SSH console, serial port on a PTY
  inventory_test.go    tags, CSV encodings/separators, XLSX import, NetBox import and sync (fake NetBox)
  keys_test.go         key store, deploy/revoke/who has access, vault sharing and rotation (fake SSH server with sh and passwd)
  guac_test.go         remote desktop relay against a fake guacd: handshake, filtering, recording, jump hosts
  importers_test.go    mRemoteNG (encryption formats, inheritance, master password) and OpenSSH config import
  putty_test.go        PuTTY .reg import (UTF-16 / UTF-8, defaults, proxies and their reuse, SSH proxy → jump host), link with mRemoteNG in both orders
  proxy_test.go        proxies (in-process SOCKS5 / HTTP CONNECT, with and without auth): terminal, SFTP, status, jump host + proxy, web interface, desktop relay, WRM tunnel proxy, errors, sharing, folder defaults
  review_fixes_test.go  FTP with EPSV through a proxy (fake FTP server), another user's tunnel refused as a proxy, folder defaults (own WRM tunnel, loops refused, PUT keeps choices), per-user status keys, notification fixes (recipient injection, mentions, reminders, AUTH LOGIN)
  notify_test.go       notification channels against fake HTTP endpoints and a fake SMTP server, digests, rate limit, quiet hours, status and rotation events
  import_limits_test.go  import size limits through the real middleware: multi-MB confCons.xml and CSV, HTTP 413 above the limit
  service_test.go      service files for every OS (golden files in testdata/service), env file, version comparison, newest binary in the folder
  releasesign_test.go  release signatures: round trip, tampering, wrong key or file, WRM context, key parsing, the command-line tools, self-update refusing unsigned or wrongly signed releases
  v138_ai_sessions_test.go  AI sessions view (own sessions only, filters, search, save, export, retention), personal AI settings, continuing an MCP session; MCP approvals given later run at once (and with repeat) and are checked again before they run, Deny in the app is final
  v12625_test.go       LDAP and SCIM against a fake LDAP server (LDAPS, StartTLS, AD nested groups, paging): protocol pieces and filter escaping, sign-in and mapping, refused accounts, the regular check ending access, linking, stored passwords not sent to a changed address, SCIM users, groups, PATCH and deprovisioning, the SCIM account linked on the first OIDC sign-in, disabling ends everything
  v12626_test.go       lockdown (sessions, sign-in, AI, shares, CLI state, break-glass only), Stop user and what it ends, every alert rule and its settings, SIEM delivery to fake Splunk, Elastic, Sentinel (with a fake Entra token endpoint) and HTTPS collectors over TLS, syslog over TLS (RFC 5424 framing, structured data), the JSON lines file, catching up after an outage, action filters and the stored secret not sent to a changed address
  v12624_test.go       directory sign-in against a fake identity provider: ID token checks (RS256/ES256, nonce, audience, expiry, alg none/HS256, key rotation), accounts from rules, role and team sync, refusals, browser binding and replay of the callback, MFA, local 2FA after a directory sign-in, revalidation (refused refresh, removed from the group, no refresh token), password sign-in limited to break-glass, linking, step-up and test sign-in, admin API
  v12623_test.go       roles and teams: who-policy values, auditor read-only and refused connections, step-up, break-glass networks and notifications, teams in shares, handover on delete
  v12622_test.go       security levels: rule comparison, preview, blockers and deviations, apply and sign-out, env lock, HTTPS for sign-in, level at start and first run, guest link lifetime
  fuzz_test.go         fuzz targets for the import parsers (mRemoteNG, SSH config, CSV/XLSX, PuTTY, WinSCP, configuration files, deploy-config YAML); seeds run with every go test
  update_test.go       self-update against a fake GitHub releases server: success and rollback, checksum mismatch, missing checksums file or line, missing asset, wrong -version, policy, Docker, local binary
  ai_policy_test.go    read-only classifier (many cases), destructive patterns (every rule, false positives), admin patterns, globs
  ai_engine_test.go    AI sessions against a fake Anthropic / OpenAI-compatible server and the fake SSH host: approve, edit, deny, timeout, read-only, destructive in every mode, automatic-mode limits, file edits with diffs, kill switch, most restrictive policy, keys never returned, redaction, audit, transcript and recording, prompt injection, the event stream
  mcp_test.go          an MCP client harness: initialize, tools/list, tools/call, elicitation; OAuth with PKCE failure and code reuse; token scopes and expiry; revoke and kill switch; approval round trip; read-only and scope enforcement; destructive block; prompt injection; audit; transfer; the stdio bridge; origin and CORS checks
  mcp_bundle_test.go   the Claude Desktop extension: download, manifest, token checks; its Node.js bridge end to end with an approval
  v13140_test.go       server names from hostnames, log paths, folder sorting, file edit history (diff, redaction, encryption, filters, CSV), SSH user certificates against a fake SSH host that trusts a CA (expired, mismatched, pasted, key file)
  v13130_plugin_test.go  the Claude plugin: committed bridge bundle is current, no settings, no bin/; the bridge's browser sign-in (wrm_connect) end to end, saved connection, denial; the setup script for all AI apps run with sh against real config files
  v1265_test.go        Medium step by step: tool import steps with output, deploy-config checked at once and kept while preparing, failed start keeps its step, error and redacted output, activity log; 409 names the running operation and its user, visibility and medium_log_output; approve later after the AI app closed its session; grep/sed/find patterns are not taken for files
  v1266_test.go        Docker for Medium: backend choice, Linux packages after consent with steps and output, no rights shows -setup-docker, policy off, Windows and macOS Docker Desktop with download, signature and licence, permission denied, wrm -setup-docker
  ai_opencode_test.go  OpenCode Zen: catalog parsing, free and deprecated models, keyless use only of free models, API family routing, fallback without the catalog; delegation off, automatic choice, training opt-in, redaction, audit
  opencode_agent_test.go  OpenCode configuration (own tools off, WRM as MCP server), install with sha512 check against a fake registry, loopback-only tokens; with WRM_TEST_OPENCODE set, the real OpenCode binary end to end (WRM tools, approval in the chat, cleanup)
  opencode_sandbox_test.go  OpenCode sandbox: configuration (own tools on, shell/edit/web fetch ask, local provider rewritten), locked-down docker arguments without secrets, settings, a permission request becoming a WRM approval that cannot be edited (approve, deny with a note, other permissions refused); with WRM_TEST_OPENCODE and WRM_TEST_OPENCODE_SANDBOX=<image>, a real container end to end
  v12610_test.go       the canvas is wired (addon windows, floating panels on the canvas, Tools menu, top bar group and order, sessions keep addons); /api/me/ui takes 256 KiB
  addons_v2_test.go    addon platform v2: strict manifest and error codes, theme validation, personal addons (visibility, policy, no server code, admin kill switch), instances, store scopes and quota, windows, export/import, CSP with script hashes, integrity, approvals on growing updates, downgrades
  addons_http_test.go  addon web requests: http manifest validation and permission coverage, IP guard (loopback, private, metadata, CGNAT), proxy against a fake site (allowlist, approval, methods, paths, header filtering, secrets per instance and personal override, redirects, size cut, rate limit, coalescing, log), plain http and personal addons waiting for an administrator
  addons_lint_test.go  addon tools: every template lints clean and installs, lint findings with codes and lines, schema covers the manifest, llms.txt lists the codes, /api/addons/dev, the dev server (host check, token, CSP, store, web requests, env secrets, reload)
  v1269_test.go        in-app notifications (recorded without channels, per user, reminders once a day, mark read, clear, newest 200 kept, removed with the user); metrics parser; /api/workboard/metrics (own connections only, SSH error in the result, policy off, fresh and stale Monitoring sample); the page loads static/workboard.js
  v1268_test.go        /api/me/ui: per-user layout store (put, overwrite, reset, objects only, 64 KiB limit, unknown keys, signed out, removed with the user); the page loads static/layout.js and has every panel
  v1267_test.go        Medium certificates: host CA bundle and extra PEM file reach every container (mount, environment, image roots kept), expired and duplicate certificates left out, bad or missing file named
  v1264_test.go        ai_full_access_blocked: deny keeps the administrator's blocked commands refused under full access, the destructive list still asks; ask asks
  v1263_test.go        full access modes: every AI session with the switch alone (default), only connections with the scope, each user's own switch (403 when not allowed, audited)
  v1262_test.go        full access: blocked and destructive commands and secret files ask (also in automatic mode with *), policy off refuses again, read-only stays read-only
  v1261_test.go        "Allow everything": * with chains, variables and redirections, deny lists and blocked commands still apply, no time or action limit until the AI connection expires, panel sessions keep limits; approve later; approved, redacted reads of secret files
  ai_provider_test.go  SigV4 test vector, Bedrock Converse event stream, Vertex token exchange, Azure / OpenAI, Anthropic request shape, refusal, cancel, cost
  termcwd_test.go      the shell's working directory over SSH against the fake SSH host (a real sh on a PTY): start directory, after cd, a foreground job
  static/index.html          the entire web UI (embedded into the binary)
  static/git.js              the Git workspace (loaded when it is opened)
  static/ai.js               the AI assistant panel and the admin AI tab (loaded when used)
  static/mcp.js              AI connections (MCP): settings, indicator, approval cards, OAuth consent, admin tab (loaded when used)
  static/aisessions.js       the AI sessions panel and viewer; shared handling of decisions on requests that no longer wait
  static/layout.js           panel layout: dock zones, tab groups, splitters, floating windows, lock, Panels menu, per-user storage
  static/addons.js           Settings → Addons, addon panels and the v2 bridge (instances, store, watchdog, permission sheet, web requests, secrets)
  static/workboard.js        the Workboard: addon windows on the canvas, addon catalog, settings, docking addons as panels, canvas menu (grid, arrange, saved layouts, export/import)
  static/brand/              logo, favicon and app icons
  static/vendor/             xterm.js + addons and fonts (served locally, see THIRD-PARTY-LICENSES.txt)
  mcpb/server/index.js       the Node.js bridge of the Claude Desktop extension, the Claude plugin and the setup script for all AI apps (embedded; mcp_bundle.go): forwarding, wrm_connect sign-in, --setup
.claude-plugin/marketplace.json  Claude Code plugin marketplace of this repository
packaging/claude-plugin/wrm-pro/  the WRM PRO plugin for the Claude app and Claude Code: server/wrm.mcpb (the bridge, built by buildPluginMCPB), skills (wrm, wrm-safe-operator, linux-incident-triage), /wrm-pro:connect
.github/workflows/build.yml  CI: vet, gofmt, tests (race), JS syntax check, builds all platforms, Docker image (with a vulnerability scan), releases on tags
.github/workflows/security.yml  security checks: govulncheck, staticcheck, gosec (new findings only), gitleaks, actionlint and zizmor, dependency review
.github/workflows/codeql.yml  CodeQL code scanning (Go and JavaScript)
.github/workflows/scorecard.yml  OpenSSF Scorecard
.github/workflows/claude-security-review.yml  Claude security review of pull requests (needs the ANTHROPIC_API_KEY secret)
.github/workflows/nightly.yml  nightly fuzzing of the import parsers and an OWASP ZAP scan (signed out and as an administrator)
.github/security/            gosec baseline and the WRM checklist for the Claude review
.github/dependabot.yml       weekly Go module, GitHub Actions and Docker updates
.gitleaks.toml, remote-manager/staticcheck.conf  scanner settings
tools/gosec-baseline.js      compares a gosec report with the reviewed baseline
.claude/skills/wrm-security-review/  the security review every Claude Code session runs before a push
Dockerfile, docker-compose.yml  container image and one-command deployment
ARCHITECTURE.md              how WRM is built; extension plan status and touch points
docs/brand/                  logo kit (SVG + PNG: mark, lockup, app icons, favicon, social preview)
```

**CI/CD:** every push builds all 13 targets and the Docker image and runs the [security checks](SECURITY.md#continuous-security-checks). Pushing a tag `v*` creates a GitHub Release with all binaries, a macOS universal binary, Linux x86_64 / ARM64 AppImages `SHA256SUMS.txt` with its signature `SHA256SUMS.txt.sig` (repository secret `RELEASE_SIGNING_KEY`), a CycloneDX SBOM and GitHub build provenance, and publishes the image `ghcr.io/vedranius/wrm-pro`.

---

## Troubleshooting

For v12.4.4, **AI missing/unavailable** is diagnosed in Settings → AI assistant; check account policy/kill switch, provider availability and SSH/SFTP context. **Functionality unverified** means the remote account cannot inspect a service manager, no unit/probe is configured, or a check is incomplete; inspect its status message and configure the service's health endpoint if needed. **Media cannot play** can mean missing browser codec support or an FTP server without SIZE/REST; use Download. **Same-named servers** can be distinguished by full folder paths; choose Parent folder to build client/site nesting.

| Problem | Fix |
|---|---|
| Claude app: *Failed to add marketplace* | Type the repository in lowercase, `vedranius/wrm-pro-enterprise`: the Claude app names the marketplace after the repository, and GitHub accepts the name in any case. Remove a marketplace with the same name that is already under *Customize → Plugins*. The real reason is in the app's log (Windows `%APPDATA%\Claude\logs\claude.ai-web.log`, macOS `~/Library/Logs/Claude/claude.ai-web.log`, lines with `MARKETPLACE_ERROR` or `REMOTE_SYNC_FAILED`). *Add → Upload plugin* with `claude-plugin-wrm-pro.zip` from the release installs the same plugin without a marketplace |
| Medium native deploy: *locked by … since …* | Another deployment (WRM or the original tool) holds the destination. Wait for it; a lock left behind by a crash can be released in *History and rollback* once it is older than the time shown (from `medium_command_timeout` and `medium_transfer_timeout`) |
| Medium native deploy: *the plan changed since you reviewed it* / *plan changed* | The server or the deploy-config changed after the comparison. Compare again, review the new plan and deploy that |
| Medium native deploy: *Deploy* is disabled | Switch the project to *WRM native* (Engine), check the `medium_native` policy and `allowed_refs`; for a project with a build step, choose a build location (WRM host, Linux build server or none) before previewing |
| Directory sign-in: *Sign-in refused: your directory account is not assigned to WRM* | No rule matches the token. Use *Test sign-in* with that person's account to see the claims; assign the app role or group in the directory, or add a rule. Entra sends `groups` only when the app registration's token configuration adds them, and replaces them by an overage note for people in many groups: map app roles instead |
| Directory sign-in: the provider says the redirect address does not match | Register exactly the address shown in *Admin panel → Directory sign-in*; behind a reverse proxy set the provider's redirect address (or `ai_mcp_public_url`) to the public address |
| Directory sign-in: *the ID token has expired* / *issued in the future* | The WRM host's clock is wrong; sync it (NTP) |
| LDAP: *Test connection* fails at *TLS* | The certificate is not trusted by the WRM host or does not name the address you entered: paste the directory's CA certificate (PEM), and use the name in the certificate (not the IP address) |
| LDAP: *Test connection* fails at *Connect* on port 389 | The server does not offer StartTLS: enable LDAPS or StartTLS on the directory (Active Directory needs a server certificate), or use `ldaps://` on port 636 |
| LDAP: *Your directory account is not in a group that may use WRM* | No rule matches: use *Test sign-in* with that account to see its groups and attributes, then add the person to a mapped group or add a rule |
| SIEM destination *Failing* with *x509* or *certificate* | The collector's certificate is not trusted by the WRM host: paste its CA certificate (PEM) into the destination |
| Sentinel: *403* or *Forbidden* | The Entra application needs the role *Monitoring Metrics Publisher* on the data collection rule; check the rule's immutable ID and the stream name |
| Everyone is signed out and cannot sign in | WRM is in lockdown (the sign-in page says so): a break-glass administrator ends it in *Admin panel → Security*, or run `wrm -lockdown off` on the host; with `WRM_LOCKDOWN=1` remove the variable and restart |
| SCIM: Entra reports *401* or *Unauthorized* | The secret token is wrong, rotated or revoked: create a new token in *SCIM provisioning* and paste it into the provisioning settings |
| Directory users are signed out every few hours | The provider gives no refresh token: add `offline_access` to the scopes (and allow it for the client), or raise `oidc_unverified_hours` |
| A panel is gone, or tabs cannot be dragged | *Panels* (▦ in the top bar, or in its ⋯ menu) shows and hides every panel; 🔒 on that button means the layout is locked: unlock it there. *Reset the layout* puts every panel back where it started |
| Medium: pip fails with *certificate verify failed: self-signed certificate in certificate chain* | A proxy or antivirus inspects HTTPS. Keep `medium_host_ca` on (default) and press *Prepare runtime* again; if its root certificate is not in the WRM host's store, export it as PEM and set `medium_ca_file` |
| *Request blocked by CSRF protection* / terminals do not connect behind a proxy | Pass the `Host` header (`proxy_set_header Host $host;`) and WebSocket upgrade headers, or set `WRM_ALLOWED_ORIGINS` |
| AI: no **AI** button on a terminal | The assistant is off (`ai_assistant`), not for your account, the kill switch is on, the connection is not SSH, or your share role does not allow terminals and file changes |
| AI: *the policy does not allow the AI assistant on this connection* | A mode rule for its tag, folder, host or id leaves no mode; ask an administrator (*Admin panel → AI assistant → Mode rules*) |
| AI: a harmless command needs an approval in read-only mode | The read-only classifier only accepts commands it knows are read-only (no redirections, substitutions or variables). Use a read tool, approve it in *ask* mode, or add it to the automatic-mode allow list |
| AI: *the provider answered HTTP 401/403/404* | Check the key, the model name (Bedrock: `anthropic.…` IDs or inference profiles; Vertex: the region offers the model) and the base URL; *Test* in the provider list sends one small request |
| AI: *HTTP 503* (*high demand*) or *429* | Temporary provider overload: since v12.2.0 WRM retries up to three times (3, 8 and 15 seconds) before it shows the error; try again later or choose another model |
| AI: *HTTP 413* / *request too large* (Groq and other providers with token-per-minute limits) | Since v12.2.0 WRM shortens the long command output of the conversation and retries; if the provider's limit is still too small for the system prompt and tools, pick a model with a larger limit |
| AI: Gemini answers *missing a thought_signature* | Fixed in v12.2.0: WRM sends the provider's `extra_content` of tool calls back unchanged. Update WRM |
| AI: a Claude.ai / ChatGPT subscription does not work | Consumer chat subscriptions cannot be used by other applications: use an API key or your company's gateway — or connect the chat app to WRM over MCP ([AI desktop apps](#ai-desktop-apps-mcp)) |
| MCP: Claude Desktop shows no WRM tools; `claude_desktop_config.json` has an empty `mcpServers` | Claude Desktop does not read HTTP entries from that file, and a custom connector connects from Anthropic's cloud, which cannot reach a WRM on your network. Install the WRM PRO extension instead (*Settings → AI connections → Claude Desktop*); extensions are listed under *Claude → Settings → Extensions*, not in `mcpServers` |
| Claude Code: *claude.ai connectors are disabled because ANTHROPIC_API_KEY or another auth source is set* after adding WRM | Not caused by WRM: the WRM plugin, `claude mcp add` and the setup scripts only pass the WRM token in the MCP server's own `Authorization` header and never set Claude's sign-in. Something on the computer sets `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN` or an `apiKeyHelper` (`/status` in Claude Code shows the auth source). Remove it from the environment (Windows: `[Environment]::SetEnvironmentVariable('ANTHROPIC_API_KEY', $null, 'User')`) and from the `env` / `apiKeyHelper` of `~/.claude/settings.json`, then restart the terminal and Claude |
| MCP: *The user denied this action* or *Not run* although the AI connection is automatic with `*` | Before v12.6.1 a long session fell back to asking after its time limit, and commands with variables asked even with `*`; when the app declined that question the command was refused. Upgrade, edit the AI connection and press **Allow everything** (no time or action limit). The refusal now names the rule that asked |
| MCP: *Not run yet … WRM keeps the request open* | Nobody approved in time or the app declined the question. Approve the card in WRM (or the live panel), then tell the AI to continue: it repeats the call and it runs once |
| MCP: no *AI connections* tab in Settings | MCP is off (`ai_mcp_enabled`); the tab says why when the policy `ai_mcp` does not include you or the kill switch is on |
| MCP: claude.ai / ChatGPT cannot connect (*couldn't reach the server*, timeout) | Hosted connectors connect from the provider's cloud: WRM must be reachable from the internet over HTTPS with a trusted certificate. Behind a reverse proxy pass `/mcp`, `/oauth/` and `/.well-known/` through, turn off response buffering for `/mcp` (server-sent events) and set `ai_mcp_public_url` |
| MCP: *the redirect URI … is not allowed* | The app's OAuth callback is not in `ai_mcp_redirect_uris`; an administrator adds it (*Admin panel → AI connections*) |
| MCP: HTTP 401 *the token has expired* / *was revoked* | Create a new token, or let the app connect again (OAuth); tokens live `ai_mcp_token_hours` (at most `ai_mcp_max_token_hours`) |
| MCP: HTTP 403 *origin not allowed* | A browser-based client on another origin: add it to `ai_mcp_allowed_origins` |
| MCP: a change is *Denied by WRM … run_readonly* | The AI connection has no `run_with_approval` scope; create one with it (if the policy allows) |
| MCP: the app waits and nothing happens | An approval is pending: answer it on the card in WRM (or in the app); unanswered approvals are denied after `ai_approval_timeout_seconds` |
| MCP: "already approved / denied" when deciding | Somebody (you in the AI app, or in another WRM tab) decided it first; the message says who and where. "The AI stopped waiting": ask the AI to repeat the call |
| MCP stdio bridge: *certificate signed by unknown authority* | Start the bridge with `-ca-file` pointing to the WRM certificate or your CA (PEM) |
| *HOST KEY VERIFICATION FAILED* | The server's SSH key changed. If you know why (reinstall), click *Trust this key* in the terminal (connection owner/admin) or forget the host in *Admin → Host keys* |
| *Microphone requires HTTPS* / no microphone | Use HTTPS (certificate, reverse proxy or `HTTPS_SELF_SIGNED=1`) and allow the microphone in the browser; you can also join as a listener |
| Voice: *Connecting…* forever or *Connection problem* | Open UDP+TCP 3478 and the relay port range on the WRM server; behind NAT set `turn_public_ip` and forward the ports; in very strict networks add a TURN server on 443/TLS (*Admin → Voice & network*) |
| Environment deploy: *the server or the repository changed since the plan was reviewed* | Something changed between the compare and the deploy (a file on the server, another deploy, a new target). Compare again, review the new plan and deploy it |
| Environment deploy: *locked by … from …* | Another deploy holds the lock of that destination. Wait for it; a lock older than max(30 min, the transfer timeout) is *stale* and the plan offers to remove it — only do so when no deploy is running there |
| Environment rollback *incomplete* | A file or directory of the rollback is a symlink on the server, so it was skipped. Fix the server (replace the symlink) and roll back the same deploy again |
| Activity: *the API rate limit was reached* / the list is incomplete | The Git server limits API calls per token. Wait until the time shown, narrow the dates or choose one branch; results are cached for 3 minutes |
| Activity on GitHub shows commits but no pushes | The token cannot read the repository activity (it needs read access to the repository's metadata); commits are listed anyway |
| What is where: *behind* shows — | The branch head or the compare could not be read (no API source, a branch that no longer exists, or the commit is not in the repository) |
| Registration tab missing | Self-registration is closed (default). An administrator creates accounts, or opens registration in *Security policies* |
| The service does not start / *-install-service* fails | `wrm -service-status`; Linux: `journalctl -u wrm`, macOS / Windows / BSD: `<data dir>/wrm-service.log`. Installing needs root (`sudo`) or administrator rights (Windows). Settings of the service are in `<data dir>/wrm.env` |
| *Update refused: … SHA256SUMS.txt …* | The release has no checksums file, no line for this platform's file, or the download does not match it. WRM does not install unverified files; wait for a complete release or replace the binary by hand |
| *WRM cannot write to the folder of its binary* | The one-click update writes next to the binary. Give the service account write access to that folder (systemd units with `ProtectSystem=strict` need it in `ReadWritePaths`) or update by hand |
| No update badge in an air-gapped network | Expected: the check cannot reach GitHub. Turn it off with `WRM_UPDATE_CHECK=off`, or set `update_proxy` |
| 📂 *Open here*: *directory not found* | The shell was not found among the server's processes (a non-OpenSSH server, no `/proc` / `ps`) or its directory cannot be read (e.g. a root shell after `sudo -i`). Let the shell report its directory with OSC 7 (see [Windows, tabs & snapping](#windows-tabs--snapping)) or open home or `/` |
| Locked out as administrator | `wrm -reset-password <user>` (add `-reset-2fa` if needed) |
| *Connection failed: unable to authenticate* | Check user/password/key with **Test connection**; for *Key file*/*Auto* the key must exist on the **WRM server** and the account must be an administrator (or the policy allows it) |
| No *● REC* badge / no recordings | *Admin panel → Security policies → Session recording* is off, or forced off by `WRM_SESSION_RECORDING` / `SESSION_RECORDING_ENABLED`. Check the server log for `recording:` errors (e.g. the recordings folder is not writable) |
| Recordings take too much disk space | Lower `recording_retention_days` or `recording_max_mb`, or move them with `WRM_RECORDINGS_DIR` |
| *Verify integrity* reports a broken chain | An audit entry was changed or removed outside WRM (directly in the database). Restore the database from a backup and investigate who had access to the server |
| Stored passwords stopped working after a restart | The encryption key changed (`ENCRYPTION_KEY`, `ENCRYPTION_KEY_FILE` or the `.key` file next to the database). Restore it, or re-enter the passwords |
| *bastion cannot reach 10.0.0.21:22* (or *jump host "…": …*) | The jump host cannot open a connection to the next hop: check the host:port as seen **from the jump host**, its firewall, and `AllowTcpForwarding yes` in its `sshd_config` |
| Tunnel shows 🔴 *Error* / *administratively prohibited* | The SSH server refuses forwarding (`AllowTcpForwarding`, `PermitOpen`, `GatewayPorts` for remote listeners on other addresses than loopback), or the target is not reachable from the server |
| *address already in use* when starting a tunnel | Another program (or tunnel) uses that port on the WRM machine. Choose another port or leave it empty (automatic) |
| A snippet does not appear in the ⚡ picker | Its scope is a folder or connection other than the terminal's. The panel shows all snippets (others dimmed) |
| Run on connect does not run | It runs only for the owner's snippets that apply to that connection and have no `{{?…}}` prompts; check the audit log for `terminal.auto_run` |
| A bookmark does not appear under ★ | Its scope is another connection, folder or tag. In a share you see only bookmarks the owner shared (global ones) |
| *the directory … does not exist on this server* | The bookmark's path (after `{name}`, `{host}`, `$USER` are filled in) is not there or not readable on that server. Edit the path or use a narrower scope |
| Broadcast did not ask before a dangerous command | The check sees what you typed during the broadcast, not a command recalled from the shell history (↑) |
| All connections stay grey (no status dot) | Live status is off (`status_enabled`), or the connection has *Monitor up/down status* unticked. The first round starts a few seconds after WRM starts |
| A server shows 🔴 but SSH works | The port in the connection differs from the real one, or a firewall allows SSH only from some addresses (not from the WRM server). Behind a jump host use a jump host instead of a direct connection |
| BMC: *refused the login* / *the BMC user may not do this* | Check the BMC user and password (or the vault credential); the user needs the *Operator* or *Administrator* role on the BMC for power actions |
| BMC: *not reachable* | The management network is often only reachable from a bastion: set the jump host on the connection and tick *Through the jump host* |
| IPMI: *ipmitool is not installed* | `apt install ipmitool` on the WRM server, or on the jump host when the BMC is reached through it. IPMI over LAN must be enabled on the BMC |
| Serial console shows nothing | Press Enter. iDRAC: the console needs *Serial communication → Redirection* in the BIOS; IPMI SOL: enable SOL on the BMC and the serial console in the OS (`console=ttyS1,115200`) |
| Serial port: *permission denied* | Run WRM as a user in the `dialout` group (or give it access to the device) |
| No *Install* button / *Install as an app* only shows steps or a reason | The button appears only when the browser offers to install right away; otherwise the row shows the steps for your browser (Chrome/Edge/Brave: the icon in the address bar or the menu; Safari on macOS: *File → Add to Dock*; iPhone/iPad: *Share → Add to Home Screen*; Android: menu → *Install app*). *Not possible here* means: plain `http://` to an address other than `localhost` (use HTTPS with a **trusted** certificate, e.g. [Caddy](#https-with-caddy)), an untrusted certificate (`HTTPS_SELF_SIGNED=1`: trust it first, or use Caddy's `tls internal` CA), or Firefox on the desktop (use Chrome, Edge, Brave or Safari) |
| Import: *The file is too large (… MB): the import limit is 20.0 MB* | Import files may be up to 20 MB. Split the file (e.g. export mRemoteNG folders separately, or split the CSV) and import the parts — existing connections are skipped. Behind a reverse proxy allow bodies of at least 32 MB (nginx `client_max_body_size`) |
| `proxy socks5://…: not reachable` | WRM cannot open a TCP connection to the proxy: check its address and port, and with a jump host that the **jump host** can reach it. In Docker `127.0.0.1` is the container: use `host.docker.internal` |
| `proxy …: authentication required` / `authentication failed` / HTTP `407` | Set or correct the proxy's user name and password (or its vault credential) in *🌐 Proxies* |
| `proxy … cannot reach host:port: … (SOCKS5 reply 5)` / HTTP `502` | The proxy works but the target refused or is unreachable from the proxy. Check the host:port as seen **from the proxy**; turn off *DNS at the proxy* if the proxy cannot resolve internal names |
| *IPMI and Serial-over-LAN use UDP: they cannot go through the proxy* | Use Redfish for that BMC, or untick *Through the jump host* (ipmitool then runs on the WRM server) |
| *proxy … is shared with a password: it cannot be reached through your own jump hosts* | Ask the owner for a proxy without a password for this route, or define your own proxy |
| A connection suddenly goes through a jump host / proxy | Its folder has a default (*Folder settings…*, ⤳ after the folder name). Choose *no jump host / no proxy (not the folder's)* in the connection |
| Git: an installation is not found | Check *Settings → Catalog settings → Server roots* (searched to depth 3), the service's fingerprint files (relative to the installation root, all must exist) and whether the path looks like a backup (`git_backup_words`, `ignore_dirs`). Installations can be added by hand |
| Git: every file is *modified* | The *subdirectory* of the service must be the part of the repository that is the installation root (e.g. `linux`). The server needs `sha256sum`, `shasum` or `openssl` |
| Git: a run stops with *dangling imports* | Another file imports a name the new module no longer defines: update that file in the same run (tick it), fix the repository, or tick *Skip the check for dangling Python imports* if the import is not used |
| Git: *… was changed on the server since the check* | Someone edited the file after the last check. Check again, review the diff, and tick the file explicitly if it should be overwritten (it stays in the backup) |
| Git: a webhook does nothing | *Settings → Git sources → Webhook…* shows the last delivery. Check the audit log for `git.webhook_rejected` (bad token or signature, replay, too old timestamp). The event's project must be the catalog project of a service; the owner needs the `git_checks` policy. Behind a reverse proxy, forward `/api/hooks/git/` |
| Git: *Create merge request* is greyed out | The Git source needs a write token (*Edit* the source), and the policy `git_gitignore_mr` must allow it for you. The download always works |
| Git: a CI pipeline run fails right away | Jenkins: the job must have the parameters `server`, `install_path`, `version` and the user / API token must be allowed to build it. GitLab: check the trigger token; without a read token for that GitLab the status cannot be followed (the run then says so) |
| Git: the restart fails or the unit is not *active* | The SSH user must be root or allowed `sudo -n systemctl restart …` / `supervisorctl` without a password. The run shows the output of `systemctl is-active` / `supervisorctl status` |
| Git: an install or transfer stops at *Pre-checks* | The message names the problem: free space (`df`), a directory the SSH user cannot write (use a writable root or a user with rights), `tar` missing, or an existing installation (tick *Replace it* and type the server name) |
| Git: *the unit … exists already* or *cannot write* the unit | WRM never overwrites a unit file: choose another name. Writing units needs root or `sudo -n tee` without a password. The files of the installation stay installed |
| Git: the file list says *only the catalog's files of the repository are known* | The target was computed by an earlier version or comes from a bundle: *Refresh targets* (bundles only contain the catalog's files) |
| Git: *A check is already running* | One check per user runs at a time (also the periodic one and webhook checks). Wait for it or press *Cancel* in the check bar |
| Git: a template field is not offered | Fields are placeholders (`{{ name }}`, `${NAME}`, `__NAME__`, `@NAME@`, `<NAME>`) and simple `key = value` / `key: value` lines; nested YAML / JSON values are not fields — type the file or copy it from another installation instead |
| Git: a scheduled run was *skipped* | WRM was not running at its time for longer than `git_schedule_grace_minutes`, the update it waited for failed, or the action is no longer allowed for the account |
| Git: `HTTP 401` / `403` from the Git server | Check the token (read access to the API and repositories) in *Settings → Git sources*; 403 can also be the API rate limit |
| Notifications: nothing arrives | Check *Admin panel → Notifications* (channel enabled, last error, *Send test*), *Settings → Notifications* (events ticked, quiet hours) and the policy `notifications_enabled`. Messages of one round arrive together after a few seconds, at most one per `notify_min_interval_seconds` |
| Workboard: *Server load* shows an error | *SSH: …*: WRM could not log in with the connection's credentials (check the connection itself). *needs Linux*: set up Monitoring (📊 in the top bar, Zabbix agent) for that server. *turned off by the administrator*: the policy `workboard_ssh_metrics` is off; only servers with Monitoring show values |
| Workboard: *Recent commits* shows an error | The Git workspace needs a GitLab, GitHub or Gitea source for the service (*Git → Sources*); the message is the Git server's answer |
| Notifications: *SMTP login: … unencrypted connection* | Go's SMTP client sends a password only over TLS (or to localhost): choose STARTTLS or TLS |
| Network tools: *ping is not installed* | Install `iputils-ping` / `traceroute` on the WRM server (or on the source server), or use the port check — it needs nothing |
| A quick connection disappeared | Quick connections are deleted 24 hours after their last use. Save the ones you need (💾) |
| CSV import: wrong letters (`Ä�`, `?` instead of `č`) | WRM reads UTF-8, UTF-16 and Windows-1250. For other encodings save the file from Excel as *CSV UTF-8* or as `.xlsx` |
| NetBox: *refused the token* / *API not found* | Use the base address (`https://netbox.example.com`, without `/api`), a token of a user that can view devices and VMs; for a self-signed certificate tick *Accept a self-signed certificate* |
| Deploy SSH key: *cannot write ~/.ssh/authorized_keys* | The login user's home is read-only or its shell is not POSIX (network devices): add the key there by hand. A key that is not accepted afterwards: check `AuthorizedKeysFile` and `PubkeyAcceptedAlgorithms` (old servers may need RSA) in `sshd_config` |
| Rotation: *the stored password does not work on …* | Nothing was changed. Fix the login (or the password with *Edit*) and check again |
| Rotation: *passwd failed* / *rejected by its password rules* | The server's password policy (length, classes, history, minimum age) refused the new password: try your own password, or wait for the minimum age. The other servers got the old password back |
| *rotation incomplete* | Some servers have the new password and could not be changed back: *Show* reveals both; set the servers to one of them, then save it with *Edit* |
| *credential … is shared for hosts matching …: the jump host … does not match* | The credential's owner limited it to some hosts: the jump hosts of your connection must match the list too (ask the owner to add the bastion) |
| Remote desktop: *guacd … is not reachable* | Install and start guacd on the WRM machine (`apt install guacd` or the Docker image) or set `guacd_address`. *Admin panel → Overview* shows its state |
| RDP: *login failed* / black screen and disconnect | Check user, password and **domain**; try *Security: NLA* or *TLS*; older servers need *RDP*. Accept the server certificate (option) for self-signed certificates |
| RDP: wrong characters when typing | Set the **keyboard layout** to the server's layout (or *Unicode*) |
| Remote desktop behind a jump host does not connect | guacd must reach the temporary tunnel on `desktop_tunnel_bind` (127.0.0.1): run guacd on the WRM machine or in the same network namespace (compose file) |
| fail2ban bans the WRM server | Add WRM's IP to `ignoreip`, raise `status_interval_seconds`, or untick monitoring for those hosts |
| Tunnel runs but the page does not open from my PC | The tunnel listens on `127.0.0.1` of the **WRM machine**. See [Where is the tunnel port?](#ssh-tunnels-port-forwarding) |
| No *🔀 Tunnels* button / *not allowed* | Tunnels are off or limited to administrators (*Admin panel → Security policies → SSH tunnels*, or `WRM_TUNNELS_ENABLED` / `WRM_TUNNEL_USERS`) |
| mRemoteNG import: *wrong master password* | Enter the master password the file was protected with in mRemoteNG. Files without a master password are opened automatically |
| *File contents* search fails | The server needs `grep` and a POSIX shell; use *File names* search instead |
| Uploads fail at a certain size | The administrator's upload limit (`max_upload_mb`) or nginx `client_max_body_size` |
| Transfer/search progress appears only at the end | Disable proxy buffering (`proxy_buffering off;`) |

---

## Contributing

Bug reports, ideas, translations and pull requests are welcome! Please read **[CONTRIBUTING.md](CONTRIBUTING.md)**. It explains how to build, test and submit changes, and the contributor terms.

If WRM is useful to you, consider supporting it on **[Ko-fi](https://ko-fi.com/vedranius)** ☕.

---

<sub>Web Remote Manager PRO · © 2026 vedranius and WRM contributors · [Apache License 2.0](LICENSE)</sub>

### Building Linux AppImages locally

On x86_64 Linux with Go, curl and sha256sum installed:

```bash
bash tools/build-appimage.sh v12.4.4 amd64
bash tools/build-appimage.sh v12.4.4 arm64
```

Outputs go to `dist/`. The script verifies pinned upstream AppImage tooling and runtimes, uses the existing WRM icon, and needs no FUSE during the build. GitHub releases remain drafts until all 16 packages and their checksums are verified and the Docker publication succeeds.

## v12.4.4 runtime troubleshooting and source map

- **Medium: Docker missing or not starting:** step 2 says what WRM can do. Press **Install Docker** (administrator, once); if WRM has no administrator rights, run the command step 2 shows (`sudo wrm -setup-docker -service-user <account>`) and press **Prepare runtime / retry**. *Permission denied* means Docker runs but WRM's account is not in the `docker` group: the same command fixes it. On Windows finish the WSL prompt or restart if asked; on a Mac without administrator rights confirm Docker Desktop's first-start window once.
- **Medium setup unavailable:** use the combined Prepare and run button to resume retained imports. Complete any Docker/WSL/license/restart prompt on the WRM host; Windows-container mode requires an explicit switch. Advanced Prepare runtime remains available; on Linux install native tools or select the managed Docker runtime. Dependency installation needs access to the original requirements' package registry. The import status reports failure without exposing arbitrary installer output.
- **Medium SSH/Git fails:** check the private SSH profile, known hosts, Git identity, host reachability, native repository permissions, build dependencies and OS user privileges. WRM's own connection credentials are separate from this original engine's SSH configuration.
- **Bitwarden login fails:** verify official CLI/server compatibility, HTTPS reachability, account credentials and MFA. Use personal API credentials with master password when interactive bot challenges prevent email login. Unlock again after inactivity/restart; no private key is returned by the key-list API.
- **Config profile missing/unparsed:** verify the catalog service and relative path, remote read permissions and supported static syntax. A source expression is not its evaluated runtime value.

Implementation: `remote-manager/git_medium.go`, embedded `medium_wrapper.py`, `git_config_profiles.go`, `bitwarden.go` and folder reorder code in `folders.go`. Regression coverage: `v1243_test.go`, native SSH/rsync helper in `keys_test.go` and `python3 tools/test-medium-wrapper.py`, alongside the full Go race suite, UI/translation check and existing Node tests. Private source/config archives and runtime state are excluded from Git/Docker build context.

## v12.4.4 workspaces, API and regression coverage

See [the v12.4.5 guide](docs/V12.4.5_GUIDE.md) for one-click Medium setup, automatic discovery, cross-log time/keyword search and sticky/filter changes. See [the v12.4.4 operation guide](docs/V12.4.4_GUIDE.md) for Monitoring/Zabbix transports, log roots/search/live panels, client GitLab cache/companion, MCP app setup and commands, editable grant scopes, selected AI context, terminal directory startup, visual labels, import routing and their exact API/bounds. [Validation and platform limits](docs/RELEASE_SUMMARY_v12.4.4.md) distinguish actual agents/browser/local-provider checks from fixtures and cross-compilation.

New backend modules: `monitoring.go`, `monitoring_zabbix.go`, `monitoring_logs.go`, `git_relay.go`, `git_relay_client.go`, `git_review.go`, `mcp_edit.go`, `mcp_prompts.go`, `mcp_picker.go`, `ai_inventory.go`, `ai_context.go`, `ai_idle.go`, `terminal_directory.go`, `visual_labels.go`. On-demand UI modules: `static/monitoring.js` and `static/git_client.js`.

`v1250_test.go` covers v12.5.0 scan UTF-8 handling, Medium SSH ZIP duplicates/replaced profiles and the partial-stamp note; `git_deploy_test.go` covers partial stamps over SSH. `v1244_test.go`, `v1244_extra_test.go` and `git_relay_test.go` cover grant edits/session invalidation, inventories, labels/ownership/import, provider HTTP cancellation/idle/header limits, Zabbix framing/API TLS/items, log roots/symlinks/timestamps/rotation, literal terminal paths, client-cache token/scope/failure/generation isolation and reviewed targets surviving a changed/failed refresh. Run the full race/vet/format/UI/Node/Python checks described in CONTRIBUTING.
