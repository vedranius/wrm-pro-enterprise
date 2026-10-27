# Contributing to Web Remote Manager PRO

Thanks for helping to make WRM better! Bug reports, ideas, documentation, translations and code
are all welcome.

## Ways to contribute

- **Report a bug** — open an issue using the *Bug report* template. Include your WRM version
  (`/api/version` or the top bar), OS, browser and steps to reproduce.
- **Report a security problem** — privately, as described in [SECURITY.md](SECURITY.md). Please
  do not open a public issue for it.
- **Suggest an idea / feature** — open an issue using the *Feature request* template and describe
  the problem it solves.
- **Improve translations** — the UI strings live in the `LANGS` object in
  `remote-manager/static/index.html` (English and Croatian today; new languages are welcome).
- **Send a pull request** — for anything bigger than a small fix, please open an issue first so we
  can agree on the approach.

## Licence and contributions

WRM is licensed under the **Apache License 2.0** from v14.0.0 (see [LICENSE](LICENSE), [NOTICE](NOTICE) and [LICENSE-CHANGE.md](LICENSE-CHANGE.md)).

Before we can accept your contribution, you must:

- sign the contributor licence agreement in [CLA.md](CLA.md) — it grants the project a licence to your contribution, it does **not** transfer your copyright; and
- certify every commit with a `Signed-off-by` line (Developer Certificate of Origin): use `git commit -s`, or add `Signed-off-by: Your Name <you@example.com>` to the commit message.

By contributing you confirm you wrote the code (or have the right to submit it) and that you license it under Apache-2.0. Do not submit third-party code unless its licence permits Apache-2.0 redistribution and you state that licence.

**AI-assisted contributions.** You may use AI tools, but you are responsible for what you submit: disclose when a contribution is substantially AI-generated, review its output, and never paste code whose origin or licence you cannot state. AI output still needs the CLA and `Signed-off-by`. See [docs/AI-CONTRIBUTIONS.md](docs/AI-CONTRIBUTIONS.md).

**SPDX headers.** New source files must start with the copyright line and SPDX identifier, e.g. for Go:

```go
// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0
```

Use the matching comment style for other languages (see `remote-manager/static/*.js` for the JS form). Do not add a header to files whose origin or licence you cannot state.

## Development

Requirements: Go (version from `remote-manager/go.mod`), Node.js is only needed for the JS syntax
check.

```bash
cd remote-manager
gofmt -l .                      # must print nothing
go vet ./...
go test -race -count=1 -timeout 45m ./...             # unit + integration tests (security, roles, 2FA, encryption, recording, audit…)
go build -o wrm-server .
HTTPS_SELF_SIGNED=1 PORT=8080 ./wrm-server   # open https://localhost:8080
```

Pull requests also run the [security checks](SECURITY.md#continuous-security-checks) (govulncheck, CodeQL, gosec against its baseline, staticcheck, gitleaks, workflow checks, dependency review). To run the main ones locally, see `.claude/skills/wrm-security-review/SKILL.md`. Do not add gosec findings to the baseline, or entries to `.gitleaks.toml`, without explaining why in the pull request; never commit real secrets, also not in tests. A new parser for uploaded or pasted files gets a fuzz target in `remote-manager/fuzz_test.go` (and a line in `.github/workflows/nightly.yml`); run one locally with `go test -run '^$' -fuzz '^FuzzName$' -fuzztime 60s .`.

Use a separate `DB_PATH` for development so you do not touch your real database. Voice calls need
HTTPS (or `localhost`) and two browser profiles or devices to test.

Code layout:

| File | What it does |
|---|---|
| `main.go` | Startup, routes, database schema & migrations, connections API, server-to-server transfer |
| `security.go` | Encryption of secrets, signed values, security headers, CSRF, HTTPS |
| `users.go`, `totp.go` | Accounts, sessions, sign-in, 2FA, admin user management |
| `settings.go`, `audit.go`, `admin.go` | Policies (feature flags), audit log (hash chain, append-only), admin panel API, terminal registry |
| `recording.go` | Terminal sessions, asciicast session recorder, file transfer log, recordings & transfers API |
| `shares.go`, `collab.go` | Shares, roles & permissions, collaboration rooms (chat, terminal sharing, voice signalling) |
| `turn.go` | Built-in TURN relay for voice calls |
| `ssh_ws.go`, `files.go`, `sftp_pool.go`, `search.go`, `conntest.go`, `hostkeys.go` | Terminal, file manager, SFTP connection pool, search, connection test, host key verification |
| `jump.go`, `tunnels.go` | Jump hosts (dialing through chains of SSH connections), SSH tunnels (local / remote / SOCKS5), tunnels API, web interface connections |
| `importers.go` | Import from mRemoteNG `confCons.xml` and OpenSSH config |
| `putty.go` | Import of PuTTY sessions (`.reg`) with their proxies; links mRemoteNG `PuttySession` |
| `snippets.go`, `status.go` | Snippets and run on connect; live up/down status monitor |
| `bookmarks.go` | Folder bookmarks: scopes, path variables, shell-quoted `cd`, start directory, WinSCP.ini import |
| `guac.go` | Remote desktop (RDP / VNC / Telnet) through guacd: handshake, relay, recording |
| `bmc.go`, `console.go`, `tty_linux.go` | Out-of-band management (Redfish, IPMI), terminal backends (BMC console, Serial-over-LAN, serial ports), pseudo terminals and termios |
| `inventory.go` | Tags, CSV / XLSX inventory import with column mapping, NetBox import and sync |
| `quick.go`, `nettools.go`, `pwa.go` | Quick connect (temporary connections) and notes, network tools (port check, ping, traceroute, DNS, HTTP/TLS), web app manifest and service worker |
| `keys.go`, `credentials.go` | SSH key store (deploy, revoke, who has access) and credentials vault (sharing, host lists, password rotation) |
| `static/index.html` | The whole web UI (embedded into the binary); `static/vendor/` holds xterm.js, guacamole-common-js and fonts, `static/brand/` the logo and icons |
| `static/git.js` | The Git workspace, loaded when it is opened; its translations are `Object.assign(LANGS.en / LANGS.hr, …)` blocks at the top and `tools/check-ui.js` checks them with the page |
| `ai_*.go` | AI assistant: permission engine (`ai_engine.go`, `ai_policy.go`), read-only classifier (`ai_classify.go`, `ai_shell.go`), destructive patterns (`ai_destructive.go` — every rule needs a test case in `ai_policy_test.go`), tools, redaction, providers (`ai_llm.go`, `ai_cloud.go`, `ai_provider.go`) and the API |
| `static/ai.js` | The AI assistant panel and the admin AI tab, loaded on demand; translations like `static/git.js` |
| `mcp_bundle.go`, `mcpb/server/index.js` | The Claude Desktop extension (`wrm-pro.mcpb`) and its dependency-free Node.js bridge; keep the bridge in step with `mcp_stdio.go` and check a generated bundle with `npx @anthropic-ai/mcpb validate` |
| `.claude-plugin/`, `packaging/claude-plugin/wrm-pro/` | The Claude plugin marketplace and plugin; check with `claude plugin validate .` and `claude plugin validate packaging/claude-plugin/wrm-pro`. After changing `mcpb/server/index.js` or the plugin bundle's manifest, rebuild `server/wrm.mcpb` with `WRM_WRITE_PLUGIN_MCPB=1 go test -run TestV13130PluginBundle` and raise `pluginMCPBVersion` together with the plugin's `version`. The plugin must not declare `userConfig` (the Claude app cannot ask for it) or have a top-level `bin/` |
| `static/workboard.js`, `workboard.go` | The Workboard (addon windows on the shared canvas, addon catalog, addons docked as dynamic layout panels, canvas menu with grid, arrange, saved layouts, export/import), in-app notifications `/api/me/events` and server metrics `/api/workboard/metrics`; a new addon is an entry in `A` with `render` (and `load`), its texts in both languages |
| `static/layout.js`, `user_ui.go` | The panel layout (dock zones, tab groups, floating windows, lock) and the per-user interface store `/api/me/ui/{key}`; its translations are checked with the page by `tools/check-ui.js` |
| `static/mcp.js` | AI connections (MCP): Settings → AI connections, the AI connected indicator, approval cards, the OAuth consent dialog and the admin tab, loaded on demand; translations like `static/git.js` |
| `*_test.go` | `security_test.go` and `upgrade_test.go` (unit), `integration_test.go` (in-process SSH/SFTP server: connect → audit → recording, transfers), `tunnels_test.go` (jump hosts, tunnels, web interfaces), `importers_test.go` (mRemoteNG, OpenSSH config), `import_limits_test.go` (import size limits), `snippets_test.go`, `bookmarks_test.go`, `status_test.go`, `guac_test.go`, `keys_test.go` (fake SSH server with `sh` and a simulated `passwd`), `inventory_test.go` (CSV/XLSX, fake NetBox), `bmc_test.go` (fake Redfish, fake ipmitool, PTYs), `quick_test.go` (quick connect, notes, network tools, PWA), `git_test.go` (fake GitLab / GitHub, bundles, catalog, discovery and comparison on the fake SSH host), `git_deploy_test.go` (update, rollback, stamp, checks, dangling imports, scheduled restarts on the fake SSH host; a fake `systemctl` on `PATH`), `git_provision_test.go` (templates, transfer exclusions, units; install from the fake GitLab and from a bundle, transfer between two fake SSH hosts, overwrite refusal, policies), `git_gitignore_test.go` (.gitignore helper, merge / pull request on the fake GitLab / GitHub write API), `git_ci_test.go` (webhooks, artifact feeds, fake Jenkins and GitLab pipelines), `git_env_test.go` (environment plans, dry runs, locks, exclusions, partial transfers, `post_deploy`, rollback on the fake SSH host), `git_activity_test.go` (Git activity against fake GitLab / GitHub / Gitea: paging, rate limits, filters, summaries, CSV), `git_where_test.go` (deploy history from several destinations and WRM runs, commits behind the branch head, drift, CSV), `ai_policy_test.go` (classifier, destructive patterns), `ai_engine_test.go` (AI sessions against fake Anthropic / OpenAI servers and the fake SSH host), `ai_provider_test.go` (SigV4, Bedrock, Vertex, Azure) |

See [ARCHITECTURE.md](ARCHITECTURE.md) for how the parts fit together and where planned features belong.

Before sending a PR please make sure that:

- `gofmt -l .` prints nothing, and `go vet ./...` and `go test -race ./...` pass,
- the page has no JavaScript errors (the CI extracts the inline script and runs `node --check`),
- new UI text has both `en` and `hr` entries in `LANGS`,
- every new API endpoint or WebSocket message checks **who** may use it (signed-in user, share role
  / permission, admin) on the server. The UI hiding a button is not access control,
- secrets (passwords, keys, tokens) are never returned to the browser or written to the log,
- security-relevant actions write an audit entry (`auditLog…`), and nothing updates or deletes audit data outside the retention job,
- new features that change behaviour sit behind a setting (feature flag), and the existing flow works with the flag off,
- you tested the change in a browser (terminal, file manager, sessions, collaboration and voice if
  you touched them).

The v12.4.0 regressions include `folders_test.go` (hierarchy, ownership, imports), `git_functionality_test.go` (workers, redaction and config boundaries), `preview_test.go` (SFTP/FTP ranges, active content and share permissions) and `ai_availability_test.go` (SFTP providers and policy events). Run `node tools/check-ui.js` and `node --test tools/test-git-simple.js tools/test-ai-events.js tools/test-appimage.js tools/test-workspace-context.js` from the repository root. UI layout/media checks require a real browser; retain a concise report that distinguishes compiled platforms from platforms actually run.

CI (GitHub Actions) builds every platform for each pull request.

## Contributor terms

By submitting a contribution (code, documentation, translations or other material) to this
repository you agree that:

1. You wrote it yourself, or otherwise have the right to submit it under these terms.
2. Your contribution is licensed under the **Apache License 2.0** ([LICENSE](LICENSE)), so
   everyone can use it on the same terms as the rest of the project.
3. You have signed the contributor licence agreement in [CLA.md](CLA.md) and certify each commit
   with a `Signed-off-by` line. The CLA grants the project a licence to your contribution; it does
   not transfer your copyright.
4. You keep the copyright of your contribution and may use your own contribution however you like.

If you do not agree with these terms, please open an issue to discuss instead of sending code.

## Code of conduct

Be respectful and constructive. Harassment or abusive behaviour is not tolerated.

## v12.4.3 integration regressions

Run `python3 tools/test-medium-wrapper.py` from the root in addition to the full Go race/vet/format/UI/Node checks. `v1243_test.go` covers Medium ZIP/proxy/origin boundaries, config profile ownership/masking/path rules, folder ordering/imports and mocked Bitwarden account isolation plus actual SSH authentication. The native SSH/rsync test requires those binaries; it skips when absent. Original-tool parity uses the privately supplied archive and its pinned suite; do not commit private source, configs, keys or screenshots containing real hostnames. Distinguish source-suite coverage from actions exercised through the bridge and mock Bitwarden login from a live cloud account.

The optional original runtime is not a sandbox and must be described as trusted executable code. Keep its native modules/templates unchanged when verifying 1:1 behavior; bridge changes belong in WRM or the embedded wrapper. Runtime artifacts belong outside source control. Test responsive dialogs and folder-order persistence in a real browser.

For v12.4.3b, `git_medium_runtime_test.go` covers backend selection, mount/argv/readiness boundaries, missing-runtime source retention and letter-hotfix version parsing. Wrapper tests cover aliased environment and private SSH-home resolution. Exercise the pinned-image import and native deploy/rollback against local fixtures; test stop/start persistence and builder access denial. Windows cross-builds do not substitute for a live Windows Desktop/CLI-context test: state that limitation. Keep runtime source/profiles/volumes private and distinguish the accepted v12.4.4 plan from completed hotfix work.

## v12.4.4 regression checks

Include `v1244_test.go`, `v1244_extra_test.go` and `git_relay_test.go` in the full race suite. Exercise a delayed response header, total/idle cancellation, mutable grant policy and pending/active invalidation, SFTP inventory/path denial, root/symlink/log rotation/timezone/cursor bounds, validated visual labels and immutable reviewed deployment targets. Relay fixtures must use canonical Git blob hashes and demonstrate client-only tokens, isolated membership, failed/out-of-order publication and disconnect/restart persistence.

Use isolated generic browser/agent fixtures for Monitoring, simultaneous live panels, Functionality hostname filters, real XML/registry uploads, MCP controls/install scripts, terminal startup/context and replay at phone/desktop widths. Never add private attachments, actual hostnames, exported credentials or downloaded fixture databases to the repository. Distinguish actual Zabbix/PSK/provider requests from protocol mocks and cross-builds from native OS runs. See [v12.4.4 validation](docs/RELEASE_SUMMARY_v12.4.4.md).

`node --test tools/test-git-client.js` verifies client relay response bytes and cancellation at the 8 MiB budget, including responses without a declared Content-Length. The release workflow runs it with the existing Node suites.

For Medium/Monitoring changes, run `node --test tools/test-monitoring.js` and the v12.4.5 Go regressions. Test actual progressive discovery, date-only and keyword historical searches, tab changes during pending requests, timezone/event details, combined environment filters and all sticky control rows at phone/desktop widths. Use generic service/config fixtures only. Windows Docker first-install paths need a real Windows session for runtime verification; Linux Docker tests/cross-builds do not establish that result.
