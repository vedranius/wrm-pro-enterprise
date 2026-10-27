# WRM PRO v12.3.6 — release summary

## Included changes

The initial v12.3.5 CI run exposed a timing window in Ollama action completion. That run was stopped before any packages were published. v12.3.6 includes the completion-order fix and the complete reviewed feature set.

- Git Simple: combined filters, per-file review, destination selection, exclusions, comparison timestamps and per-server rollback versions. Selection, confirmation and canceled comparison guards prevent unintended deployments.
- Deployment plans: connection identity, SSH destination/login and effective jump/proxy settings are pinned to the reviewed plan and checked again before acquiring a lock.
- AI: real remote date/time, immediate policy refresh, processing indicators tied to the model turn, complete allowlisted tool-call extraction and visible incomplete responses.
- Linux services: database access preflight, useful failure diagnostics, restart limits, correct paths and reinstall behavior.
- Linux AppImages: x86_64 and ARM64 packages, persistent user data, localhost defaults, desktop icon, extraction without FUSE, explicit upgrade guidance and protection against installing services from a temporary mount.

## Release packages

| Platform | Packages |
| --- | --- |
| Linux standalone | amd64, arm64, ARMv7, ARMv6, 386 |
| Linux AppImage | amd64 (x86_64), arm64 (aarch64) |
| Windows | amd64, arm64 |
| macOS | amd64, arm64, universal |
| Android / Termux | arm64 |
| FreeBSD | amd64, arm64 |
| OpenBSD | amd64 |

The 16 packages are covered by `SHA256SUMS.txt`. GitHub provides source archives. The Linux amd64 Docker image is published as `ghcr.io/vedranius/wrm-pro:v12.3.6` and `latest`.

## Validation and publication

The [pre-push review](PRE_PUSH_REVIEW_v12.3.5.md) records the original 183 Go tests/subtests with the race detector, 15 JavaScript regressions, all 13 cross-compilation targets, native Linux and Docker health checks, real browser deployment/rollback and a real local Ollama call.

The Ollama action completion regression passed 20 isolated process runs with the race detector (80 sequential action completions). Release preparation adds an AppImage updater regression, two launcher regressions, real x86_64 AppImage startup and restart with persistent database/key, boot-service refusal, and both AppImage architecture checks. The AppImage build pins and verifies upstream tooling and runtime checksums. The workflow also receives Actionlint validation; macOS universal generation and the complete/missing asset publication guard are checked locally.

GitHub Actions runs the Go race suite, UI checks, launcher regressions, all platform builds, AppImage builds and Docker smoke test. Tag runs upload packages to a draft release. Publication waits for successful binary/AppImage builds and Docker publication, constructs the universal binary, verifies the exact package set and every SHA-256 checksum, then publishes the release as latest.

Other operating systems are cross-compiled; native execution on those operating systems is not claimed. ARM64 AppImages are packaged and their ELF architecture checked; native ARM64 desktop execution is not claimed.

## Upgrade

Standalone installations can replace the binary or use the in-app updater. Existing Linux services should run `-install-service` with the new binary to regenerate the unit and restart with the unit fixes. No database migration is required.

For AppImage, stop WRM and replace the AppImage. User data stays under `${XDG_DATA_HOME:-$HOME/.local/share}/wrm-pro/` unless an explicit `DB_PATH` is configured. Use the standalone binary for a boot service.

See [release notes](../RELEASE_NOTES.md) and [changelog](../CHANGELOG.md).
