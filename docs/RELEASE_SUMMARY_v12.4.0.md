# WRM PRO v12.4.0 release summary

Release date: 2026-10-14. Accepted scope and acceptance criteria: [implementation plan](PLAN_v12.4.0.md).

## Requests and implementation

| Request | Delivered behavior | Tracking |
|---|---|---|
| Service functionality and configuration | Git catalog/GitLab installations, all Supervisor workers, failed systemd units, runtime probes, installation/resource warnings, bounded redacted logs, masked config settings and notifications | [Changelog](../CHANGELOG.md), [architecture](../ARCHITECTURE.md), `git_functionality.go` |
| Nested folders and connection context | Parent selection, recursive tree, client/site paths, folder tabs that focus/filter windows, hierarchy-preserving config/mRemoteNG imports and owner-scoped folder shares | [Issue #57](https://github.com/vedranius/web-browser-RDM-public/issues/57), initial implementation [commit 90dca10](https://github.com/vedranius/web-browser-RDM-public/commit/90dca10) |
| Remote preview/streaming | SFTP/FTP byte ranges, video/audio/image/PDF/text viewers, terminal-selected filename actions and enforced share permissions | [Issue #55](https://github.com/vedranius/web-browser-RDM-public/issues/55), implementation [commit 1d3a980](https://github.com/vedranius/web-browser-RDM-public/commit/1d3a980) |
| AI availability | SSH-backed SFTP support, permanent setup/status entry points, provider-test availability diagnostic, cross-browser effective policy refresh | [Discussion #56](https://github.com/vedranius/web-browser-RDM-public/discussions/56), implementation [commit 1d3a980](https://github.com/vedranius/web-browser-RDM-public/commit/1d3a980) |

Issues #55 and #57 were acknowledged and assigned to the **v12.4.0** milestone before implementation. The discussion also received a versioned acknowledgement. The release PR closes both issues and links all three requests to the changelog/release notes.

## Local validation

- `gofmt -l .`: empty; `go vet ./...`: passed.
- Complete `go test -race -count=1 -timeout 20m ./...`: passed in **515.927 seconds**. An earlier full run exposed two assertions that counted read-only `systemctl is-active` calls as restarts; they now assert actual mutation ordering, and focused scheduled-restart/install-transfer race regressions passed.
- Focused race coverage for new folder ownership/import/share behavior and live window titles, Supervisor groups/log redaction (including complete quoted secrets), config boundaries/private-key blocks and secret continuations (including escaped quotes and explicit entry limits), SFTP/FTP ranges, preview/share permissions and AI availability: passed. Separate regressions verify that capped unit lists and unsupported runtime/worker names are unverified rather than healthy, and HTTP/TCP probes run through SSH, accept 2xx, reject redirects/untrusted TLS and report failures.
- UI syntax/translation checks: passed. **23 Node regressions** passed across Git Simple, AI events, AppImage launchers and workspace context.
- Real Chromium browser against the WRM test server, fake GitLab and SSH/SFTP hosts: passed with **no JavaScript page errors**. Checked Croatian UI, identically named connections under Client-A/Client-B and Site-1, folder focus/filtering, account diagnostics and cross-browser policy changes, failed worker visibility, masked config values and escaped HTML, terminal-relative preview, bounded text and actual MP4 playback/seeking with HTTP 206. A separate browser check confirmed M4A playback and visually verified the native PDF viewer.
- Browser screenshots inspected at **390 px**, **1100 px**, **1440 px** and **1920 px**. Configuration is readable on mobile, and the desktop top bar fits its viewport. Screenshots remain local; no real machine hostnames are published in repository artifacts.
- Standalone Linux binary starts on a scratch database, reports `v12.4.0`, answers `/healthz` and passes `-healthcheck`.
- Local compile checks: Linux amd64, Windows amd64, macOS arm64, FreeBSD amd64 and Linux ARMv7. Other operating systems are compiled by CI; they are not claimed to have been run on their native OSes.

## Boundaries

Runtime inspection uses the SSH account's permissions and never restarts services or executes deployment check commands. HTTP probes accept 2xx, follow no redirects and verify TLS. Unavailable tools/permissions, caps and absent configured checks remain explicit. Config/source parsing does not evaluate code; unsupported lines are counted. Logs use conservative known-pattern redaction, so services should avoid logging secrets in unstructured prose. Text previews are capped at 256 KiB and playable media codecs depend on the browser. Folder shares include descendants; route defaults stay local to the immediate folder.

## Release packages

The release workflow builds 13 standalone OS/architecture targets, macOS universal and two Linux AppImages, publishes the Docker tag, then verifies the complete **16-package** set and `SHA256SUMS.txt` before publishing the draft release. GitHub also provides source archives. Final publication and CI state are recorded in the release PR and linked request updates.
