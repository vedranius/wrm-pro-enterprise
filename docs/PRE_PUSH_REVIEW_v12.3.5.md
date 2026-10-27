# v12.3.5 pre-push review

Review date: 2026-10-13. Base: `86a7627` (v12.3.4). Scope: the complete local release diff, its AI, service and Git Simple behavior, existing regression tests, build matrix, embedded UI and release documentation.

## Problems found and fixed

| Area | Finding | Result |
| --- | --- | --- |
| Linux service | Quoted `WorkingDirectory` and `EnvironmentFile` values broke paths containing spaces; systemd could reject the unit or ignore its settings. | Single-path directives retain literal paths and escape percent specifiers. Path lists and command arguments use their own escaping rules. An actual systemd parser test covers spaces, dollars and percent characters. |
| Linux service | Database preflight ran after writing the environment file. Failed installation could overwrite existing settings. | Paths are collected first; read/write access to SQLite/WAL/SHM and directory write/search access are checked before settings are written. |
| Linux service | Persistent failures caused restart loops; reinstalling an active service could leave the previous binary running. | Generated units limit restarts. Installation clears old failures and restarts the new unit. Failed starts report status/journal output and database permission hints. |
| AI visibility | Changing assistant policy did not refresh current-user policy or buttons on existing terminal windows. | AI policy changes refresh the buttons without reconnecting. Shared roles still apply; lowercase SSH protocol values are accepted. |
| AI data | `system_info` omitted the current server date, allowing confusion with the kernel build date. | The tool returns the actual remote date, time and timezone. Prompt instructions require actual tool evidence. |
| AI processing | The processing indicator ended when the prompt POST returned, before model processing finished. Its translation was also missing. | The indicator follows backend busy events for the entire turn, with English/Croatian text. |
| AI streaming | Parsed text tool calls could remain as an assistant message when their cleaned final text was empty. | Final completion removes that provisional message; mixed narrative is retained and rendered safely. |
| AI text fallback | Incomplete JSON was repaired by adding braces, potentially creating a command, or discarded with part of the answer. | Only complete JSON calls to offered tools with object arguments are accepted. Ordinary/incomplete JSON remains text. A regression proves that truncated text cannot run an allowlisted command even in automatic mode. |
| Git Simple | Typed confirmation could re-enable deploy/rollback after every server was deselected. Empty arrays previously meant all destinations. | Confirmation is recomputed from the current selection. The API refuses explicit empty arrays; omission retains the existing all-destinations behavior. |
| Git Simple | A failed destination hid deployment to healthy destinations; delayed comparisons could overwrite another review. | Healthy destinations remain selectable. Closed and superseded reviews ignore delayed responses; submissions use the reviewed options and prevent duplicate clicks. |
| Git deploy | Reviewed connection settings could change after comparison. | Connection identity, host, login and effective route selection are bound to the plan, checked at run creation and again before taking the destination lock. |
| Git Simple | Removed-file checkboxes suggested deletion although Simple kept those files. Rollback could show one server's previous version for all servers. | Retained files are labeled explicitly; deletion remains an Advanced option. Rollback shows each destination's own previous version. |
| Checks | The Ollama setup test depended on binaries installed on the test machine. UI checks missed double-quoted/formatted translation keys. | Filesystem discovery is isolated in that test; translation checks cover both quoting styles and formatted keys. Git Simple/AI event regressions run in CI. |

The service directive changes were checked against the [systemd parser](https://github.com/systemd/systemd/blob/main/src/core/load-fragment.c) and the installed `systemd-analyze verify`, including warnings that a directive was ignored.

## Validation

| Check | Result |
| --- | --- |
| Complete Go suite with race detector | PASS: 183 tests and subtests; no race reports. |
| Targeted final Git environment / OpenAI / systemd tests with race detector | PASS: 32 tests and subtests. |
| UI behavior regressions | PASS: 15 Node tests for selection, confirmation, exclusions, asynchronous comparisons, policy refresh and AI event rendering. |
| Embedded JavaScript syntax and translations | PASS: 2,886 keys per language; 1,959 literal/formatted keys checked. |
| Go vet, gofmt, shell syntax and diff whitespace | PASS. |
| Build matrix | PASS: all 13 targets with CGO disabled and version v12.3.5. |
| Native Linux startup | PASS: isolated database, HTTP health endpoint, CLI healthcheck and embedded asset comparison. |
| Docker | PASS: image build, isolated startup, HTTP health endpoint, CLI healthcheck, version and embedded asset comparison. |
| Browser smoke check | PASS: filters, empty-selection blocking, file review, one-destination deployment with an excluded file, run progress and rollback. Files on the unselected destination were unchanged. |
| Real local Ollama | PASS: installed qwen2.5-coder:7b, actual WRM tool loop against a test SSH host, correct server date/time in the final answer; final check completed in 24 seconds. |
| Release metadata / repository state | Versions agree on v12.3.5; fetched origin/main still matches the base; no local v12.3.5 tag exists. |
| Added-content review | No real private keys, provider/GitHub tokens or machine-specific names/paths found in the release additions. |

Build targets: Linux amd64, arm64, armv7, armv6 and 386; Windows amd64 and arm64; macOS amd64 and arm64; Android arm64; FreeBSD amd64 and arm64; OpenBSD amd64.

Reusable checks, from the repository root:

```sh
node tools/check-ui.js
node --test tools/test-git-simple.js tools/test-ai-events.js
bash -n remote-manager/build-all.sh
git diff --check
```

From `remote-manager`:

```sh
go vet ./...
gofmt -l .
go test -race -count=1 -timeout 20m ./...
```

## Practical limits

Native execution was checked on Linux; the other OS targets were compiled. Cloud provider protocols were exercised with test providers. The real Ollama check used the installed 7B model and a short date/time request with a 256-token response limit; it does not establish response quality or performance for every model and workload. One repeat exposed incomplete model output and the fallback parser bug, which was fixed before the final passing check.

Browser deployments used an isolated database and test SSH host with real command execution in temporary folders. The browser fixture provides SSH exec rather than an interactive shell; terminal behavior is covered by the separate Go integration tests. Linux installation was checked using subprocess fixtures and the real systemd parser; the existing installed service was inspected without reinstalling it.

For existing Linux services, `-install-service` with the new binary regenerates the unit and restarts it to apply the unit fixes. Replacing the binary alone does not update an existing unit file.

## Publication state

Changes are local. No commit, push, tag, release or update of the installed WRM service was performed during this review. Temporary browser/provider fixtures and containers were cleaned up; test logs, screenshots and validation binaries are local artifacts outside the repository.
