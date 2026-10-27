# WRM PRO v12.4.4 validation

Implementation: [ea43e49](https://github.com/vedranius/web-browser-RDM-public/commit/ea43e49158f882347323d4b00eb71c1bfcb4950f). [Release](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.4.4). Final scope/terminal guards: [8f86027](https://github.com/vedranius/web-browser-RDM-public/commit/8f86027). Scope and operation: [guide](V12.4.4_GUIDE.md), [changelog](../CHANGELOG.md).

## Delivered scope

Monitoring/Zabbix/history and migrated hostname-grouped Functionality; bounded historical/live multi-panel logs; client GitLab token relay/companion and durable atomic cache; editable MCP grants/automatic mode/onboarding/prompts/server picker/structured inventories; selected AI context and terminal directory startup; visual folder/connection labels; configurable AI provider deadlines; general XML/PuTTY import routing; sticky searches and replay scaling. Original Medium behavior remains delivered by v12.4.3/v12.4.3b.

## Validation record

| Area | Exercised |
|---|---|
| Backend | Full `go test -race -count=1 -timeout 20m ./...`, `go vet ./...`, empty gofmt output; existing suites plus new v12.4.4 regressions. |
| UI/source | `tools/check-ui.js`, the four existing Node suites plus streamed client response-limit regressions, four Python Medium-wrapper regressions; en/hr syntax/translation coverage. |
| Browser | Real Chromium at 390/1100/1440/1920: Monitoring cards/filter, duplicate-hostname Functionality groups, combined filters/query persistence and real sticky scrolling with 160 generic installations; two simultaneous live logs/highlights/history/layouts, MCP automatic-mode opt-in/CLI downloads/endpoint test and a 500-column xterm replay fitted without horizontal scrolling. Screenshots inspected; no application JavaScript errors. |
| Import | Actual general import path with synthetic 4,194,304-byte mRemoteNG XML and UTF-16 PuTTY registry session. Existing encrypted/nested/proxy import tests remain in the full suite. The reporter subsequently confirmed imports work; their actual tunneled proxy endpoints/export were not supplied or tested. |
| Zabbix | Official agent and agent 2 Linux 7.4.15 containers, real passive metrics; real agent 2 PSK via SSH-executed `zabbix_get`, plus wrong-identity rejection. API tests verify trusted/untrusted TLS CA, bearer rejection, supported active-template items and network rates. JSON/legacy framing and metric derivation have fixtures. Initial agent warm-up can produce unknown/warning metrics; it is not reported as zero. |
| GitLab | Browser direct token/CORS relay against a generic GitLab fixture; Chromium stopped a 32 MiB unknown-length response at the 8 MiB budget and retained the earlier complete snapshot; canceled contributor retained usable cache; stopped the GitLab fixture and refreshed targets from cache; restarted an actual WRM binary on a copied DB/key and refreshed again. CLI companion, ownership/member isolation, token non-persistence, failed refresh and late older-generation publication have regression tests. |
| AI provider | Real HTTP delayed-header timeout, total cancellation and blocked-stream idle cancellation; an actual local provider received a long generic prompt through WRM's request path. No real-model identity is included in this public record. |
| MCP | Bearer/expiry preserved on edit, old sessions denied, foreign edits denied, authorized text picker/prompt and permitted `.ini` inventory. Browser setup scripts are token-free; generated Bash syntax and sandboxed fake-CLI add/list/client launch plus configuration backup were checked. |
| Paths/labels | Root ownership, symlink denial, timestamps/continuations, rotations; literal POSIX directory execution with spaces/quotes/command-looking text and PowerShell encoded startup; actual selected-log SSH terminal follow; label validation/owner/export/import regression coverage. |
| Database | Previous v12.4.3b executable started successfully on a copy of the v12.4.4 database/key and returned its connections; migrations remain additive. |
| Platforms | Cross-builds: Windows amd64, Darwin arm64, FreeBSD amd64 and Linux ARMv7. Native Windows/PowerShell runtime, production agents/private exports and actual external cloud OAuth accounts were not exercised. |

## Practical bounds

See the guide for exact snapshot/log/history/request limits. The GitLab relay handles WRM API reads; native Medium Git clone/build/config Git and provider writes need their direct route. Browser CORS/private-network rules may require the client companion. Cloud AI apps require reachable HTTPS/OAuth. Zabbix active-only transport uses an existing Zabbix API; PSK needs the existing remote helper. Live logs are a bounded recent view, not a lossless archive. Installation commands, firewall changes and external client sign-in are not silently performed.

## Issue tracking

[Issue #5](https://github.com/vedranius/web-browser-RDM-public/issues/5) reports mRemoteNG XML/PuTTY import. v12.4.4 fixes generic format routing, while retaining existing specialized encryption/proxy mapping. The latest reporter comment confirms import and leaves their private tunneled-link verification outstanding; this release does not claim those private endpoints were exercised. Earlier issues [#55](https://github.com/vedranius/web-browser-RDM-public/issues/55), [#57](https://github.com/vedranius/web-browser-RDM-public/issues/57) and [discussion #56](https://github.com/vedranius/web-browser-RDM-public/discussions/56) remain linked to their original v12.4.0 fixes and the current navigation/context enhancements.
