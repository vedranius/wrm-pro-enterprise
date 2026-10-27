# WRM PRO v12.4.3 release summary

Release date: 2026-10-14. Scope: [implementation plan](PLAN_v12.4.3.md). Code: [8f9e07a](https://github.com/vedranius/web-browser-RDM-public/commit/8f9e07a). Downloads: [v12.4.3](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.4.3).

## Delivered requests

- **Original Medium:** unchanged private deploy-tool runtime, native deploy-config compatibility, original screens/options/deploy/rollback/history/config Git, authenticated bridge and private operator data. [Parity and verification](MEDIUM_PARITY_v12.4.3.md).
- **Functionality profiles:** reference upload, hostname/service/environment/operator settings matrix, search and explicit difference/missing/parse states. Masked/encrypted references; live values read on demand.
- **Bitwarden:** official CLI sign-in/MFA/API login, unlock/sync/lock/logout, native SSH item references and server-side key retrieval. Memory-only unlock, inactivity expiry and encrypted CLI profiles between commands.
- **Folder ordering:** persistent drag/drop and Move up/down, safe reparenting, export/import and additive migration.

## Validation

- Complete WRM `go test -race -count=1 -timeout 20m ./...`: **passed in 516.404 seconds**. `go vet`, gofmt (empty) and diff checks: passed.
- Original private tool: **1,581 passed, 9 skipped**, using its pinned development dependencies. No private source or configuration was added to public Git.
- Python ASGI wrapper tests: **3 passed**. UI syntax/translation checks: passed, including English/Croatian controls. PR CI also passed the full race suite in **853.454 seconds**, all 13 cross-builds, both AppImages and Docker health smoke. Existing Node regressions: **23 passed**.
- New Go regressions cover import/path boundaries, authenticated proxy/CSRF, config profiles, folder ownership/order/import, per-user Bitwarden lock/cache isolation and real SSH authentication. Native OpenSSH/rsync interoperability is exercised against the test SSH host.
- Chromium imported the actual supplied archives, then exercised a generic test fixture's native comparison, plan, two deployments, file-content checks, history, dry rollback and real rollback. Config-repo clone and original Git controls were verified. A subsequent final-binary Chromium check also completed native config Git commit/push and fetch/pull against a local bare test repository, verified origin/local commits and reported no JavaScript errors.
- Chromium profile upload/matrix/masking, folder order persistence and layouts at **390, 1100, 1440, 1920 px**: passed with no JavaScript errors; screenshots inspected and kept private.
- Final embedded v12.4.3 Linux UI/health and Docker health smoke: passed. Official CLI **2026.9.1** status through the final WRM binary and Docker dependency/version checks: passed. Bitwarden authentication paths use a mock CLI plus a real SSH handshake; no real Bitwarden cloud credentials were used.
- Local builds: Linux amd64, Windows amd64, macOS arm64, FreeBSD amd64 and Linux ARMv7. Native OS runtime checks are Linux/Docker only; other platform binaries are compile checks.

## Upgrade and operating boundaries

Migrations only add `folders.sort_order` and `git_config_profiles`. Existing folders keep alphabetical order until manually reordered. Back up the DB, encryption key and private `.medium` / `.bitwarden` directories. Medium's native plans/history are separate from Simple/Advanced. Private runtime source/config/SSH files use restrictive filesystem permissions; preferences and Bitwarden profiles are encrypted with the WRM key. Native commands execute as the WRM account and require trusted operators; directory separation is not an OS sandbox. Runtime Git/SSH/cloud reachability remains required. Earlier roadmap items were deferred by the owner's explicit v12.4.3 priority.

The release workflow builds 13 standalone targets, a macOS universal binary and two AppImages, publishes Docker, and verifies **16 packages plus SHA256SUMS.txt** before publishing the draft. Final CI/publication state is linked from the release PR.
