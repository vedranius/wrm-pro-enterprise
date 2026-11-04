# Handover - Storages (S3)

For whoever continues this work (Claude / another agent). Everything is built, tested and waiting
to be merged; nothing is half-finished. Read this top to bottom before touching the branch.

## 1. State in one paragraph

The Storages feature (S3-compatible object storage as a section of its own) is complete against
`docs/ROADMAP-STORAGES.md` (STG-01 … STG-12, all `done`). It lives on branch `feat/storages` in
both repositories, with one PR per repository, both **ready for review**, `MERGEABLE`, and CI
green except the long race suite which also passed on the last full run. It must not be merged
until the parallel work on `main` (Claude / ChatGPT) settles; then it ships as **v14.1.0**
(enterprise) and **v13.15.0** (public).

| Repo | Branch | PR | Base at handover |
|---|---|---|---|
| `vedranius/wrm-pro-enterprise` | `feat/storages` | **#20** | `main` @ `b1ef81f` |
| `vedranius/web-browser-RDM-public` | `feat/storages` | **#159** | `main` @ `d762c80` |

The public branch is a mirror: the same feature, one commit on top of `main` (plus the docs).
It was rebased onto current `main` and no longer carries anyone else's commits.

## 2. What was delivered

**Backend (`remote-manager/`)**

| File | Contents |
|---|---|
| `storages.go` | `Storage` model, tables, CRUD (`/api/storages`), validation, prefix normalisation, capability checks (`requireStorage`), audit, status map + background loop, export/import helpers |
| `storage_s3.go` | SigV4 client without an SDK: list, get (Range), head, put (If-Match), copy, delete; retries; TLS pinning (`VerifyConnection`, session tickets off) |
| `storages_http.go` | object routes: `/objects`, `/object` (GET/HEAD/PUT/DELETE), `/dir`, `/move`, `/text` (editor), unsafe-content handling, `s3ErrorStatus` |
| `storage_shares.go` | read-only share links: create/list/revoke, `/s3/{token}` server-rendered page, `/s3/{token}/object`, expiry cleanup |
| `mcp_storages.go` | MCP tools `storage_list/read/write/delete`, scope and mode checks, read-deny |
| `v14100_*` / `v14101_*` / `v14102_*` tests | S3 stub with independent SigV4 check, CRUD, traversal, capabilities, delete gate, step-up, BOM/CRLF, 409, Range/206, status transitions, export/import, shares, MCP |

**Wiring:** `main.go` (routes `/api/storages`, `/api/storages/`, `/s3/`; `ensureTable` for both tables;
export/import; `go storageStatusLoop()`), `settings.go` (`storages_enabled`, `storage_edit_kb`,
`ai_mcp_scopes` default → `mcpDefaultScopes`), `notify.go` (`storage.down`/`storage.up`),
`mcp_tokens.go` (three new scopes; `mcpDefaultScopes`), `mcp_server.go` (dispatch + tool list).

**Frontend (`remote-manager/static/`)**

- `storages.js` (new, lazy-loaded): section, list with status dot, add/edit dialog with *Test
  connection* and provider presets, prefix browser, download, small upload, new folder, rename,
  delete with confirmation, share dialog, text editor (JSON format/validate, subtitle timing
  check/shift/renumber), video player over Range with a sibling SRT/VTT/ASS track converted to VTT
  in the browser, image preview.
- `index.html`: top-bar button, `openStorages()` loader, `stg_title` in EN/HR, `ne_storage_down/up`.
- `mcp.js`: scope labels (EN/HR) and the scope hint.

**API surface** (all under `repo root`, JSON unless stated):

```
GET/POST            /api/storages
GET/PUT/DELETE      /api/storages/{id}
POST                /api/storages/{id}/test
GET                 /api/storages/{id}/objects?prefix=&cursor=
GET/HEAD/PUT/DELETE /api/storages/{id}/object?key=      (Range, ?inline=1)
GET/PUT             /api/storages/{id}/text?key=        (editor, JSON {content, etag, bom, crlf})
POST                /api/storages/{id}/dir?prefix=
POST                /api/storages/{id}/move             {from,to,overwrite}
GET/POST            /api/storages/{id}/shares
DELETE              /api/storages/{id}/shares/{shareId}
GET                 /s3/{token}                         (public page, no script)
GET/HEAD            /s3/{token}/object?key=
```

**Data:** tables `storages`, `storage_shares` (additive; created by `ensureTable`).
**Settings:** `storages_enabled` (default `1`, kill switch), `storage_edit_kb` (default `2`).
**AI scopes:** `storage_read`, `storage_write`, `storage_delete` (opt-in via `ai_mcp_scopes`).

**Documents**

- `docs/rfcs/0008-storages-s3.md` - design, permissions, API, security, non-goals.
- `docs/ROADMAP-STORAGES.md` - STG-01 … STG-12 with status and commit SHAs.
- `docs/security-reviews/0001-storages-s3.md` - 11 checks, findings and residual risks.
- `docs/STORAGES.md` - operator guide (providers, switches, editor, shares, MCP, troubleshooting).
- `docs/THREAT-MODEL.md` - eight new rows for the storage surface.

## 3. Verify quickly

```bash
cd remote-manager
go build ./... && go vet ./...
go test -count=1 -run 'TestStorage|TestS3Client' -timeout 400s .
# UI syntax, translations and key parity:
cd .. && node tools/check-ui.js remote-manager/static/index.html && node --check remote-manager/static/storages.js
```

The full suite (`go test -race ./...`) takes ~35-40 minutes and is what the `Vet & lint` CI job
runs. `Coverage` runs the package tests with race as well.

## 4. Merge and release runbook

Only when `main` is quiet. Work repository by repository; enterprise first.

```bash
# 0. fresh base (the public clone's fetch refspec is not the default one!)
git fetch origin +refs/heads/main:refs/remotes/origin/main
git checkout feat/storages
git rebase origin/main          # expect no conflicts; resolve docs/* and static/* if any
go build ./... && go vet ./... && go test -count=1 -run 'TestStorage|TestS3Client' .
git push --force-with-lease origin feat/storages

# 1. merge (auto-merge is disabled on enterprise; wait for Coverage + staticcheck first)
gh pr merge <20|159> --repo <repo> --merge --admin

# 2. version bump (one commit, "release: v14.1.0 - storages"):
#    remote-manager/main.go        var AppVersion = "v14.1.0"
#    server.json                   "version": "14.1.0"
#    README.md                     **Current version: v14.1.0**
#    .github/workflows/build.yml   APP_VERSION: v14.1.0
#    CHANGELOG.md                  new section at the top
#    RELEASE_NOTES.md              new section at the top
#    docs/ROADMAP-STORAGES.md      header: shipped

# 3. push and tag (the tag pipeline publishes the release in ~45 min)
git push origin HEAD:main && git tag v14.1.0 && git push origin v14.1.0
```

Public: identical with `v13.15.0` / `"13.15.0"` and `Refs #155`.

**Changelog bullets (use as-is, adjust the version):**

```markdown
## [14.1.0] - <date>

### Added

- **Storages: S3-compatible object storage as a section of its own.** Hetzner, DigitalOcean, AWS,
  MinIO and Backblaze B2; browse by prefix, preview, download, edit small text objects (JSON,
  config, scripts, SRT/VTT/ASS subtitles), play video over Range with a sibling subtitle track.
- **Explicit permissions per storage:** read always, editing, small uploads and deleting each
  behind a switch; deleting also asks for a confirmation and the step-up password/2FA where the
  installation enforces it.
- **Read-only share links** to a prefix: hashed token, expiry, revoke, a page without any script.
- **Status per storage** (`HeadBucket`) with `storage.down`/`storage.up` notifications.
- **AI/MCP scopes** `storage_read`/`storage_write`/`storage_delete` with the storage tools; the
  AI read-deny list applies to buckets too.
- Config export/import includes the storages (secrets only in a secrets export).
```

## 5. If `main` moves again

- Enterprise: `git fetch origin +refs/heads/main:refs/remotes/origin/main && git rebase origin/main`.
- Public: **same explicit refspec**; the clone's `remote.origin.fetch` is a tag-only refspec, so a
  plain `git fetch origin main` leaves `origin/main` stale. This bit us once and the branch ended
  up carrying someone else's commits - check `git log --oneline origin/main..HEAD` afterwards: it
  must show only the storage commits.
- Conflicts so far only in `settings.go` and `static/index.html` (LANGS / top bar). Keep both sides:
  the storage keys plus whatever the other branch added.

## 6. Known caveats

| Caveat | Note |
|---|---|
| `script-src-attr 'unsafe-inline'` remains | Inline `onclick` handlers are still allowed; removing them is UX-05, a separate sweep. Everything else in the CSP is strict (hash for the inline script). |
| Share links are read-only | The table carries `can_write`/`can_delete` for a future write share; v1 exposes none of it. |
| No multipart upload | Deliberate: 16 MB per object, 2 MB in the editor. Large media is streamed, never uploaded through WRM. |
| `egress_policy` defaults to `allow_private` | On-prem MinIO must keep working; `public_only` plus `egress_allow_hosts` is the strict mode. |
| gosec G704 (two places in `storage_s3.go`) | Annotated with `#nosec` and justified: the endpoint is administrator-entered and checked by `guardEgressURL`. Do not remove the annotations, or the security job fails on "new findings". |
| Two findings from the review | Both fixed: TLS pin on a resumed session (gosec G123) and the AI read-deny for storage keys. The tests cover them. |
| `storages_enabled` default `1` | The RFC originally said off; the code ships it on as a kill switch (the section is empty until someone adds a storage). The RFC is updated to match. |

## 7. Backlog for the next iteration (not started)

- **STG-13** - write shares (uploads through a token), with the same capability switches.
- **STG-14** - object versioning / "undelete" where the provider supports it, or a `_trash/` copy.
- **STG-15** - optional `ffprobe` metadata (duration, resolution) when ffmpeg is installed.
- **STG-16** - presigned direct upload, only if bandwidth through WRM becomes a problem.
- **UX-05** - remove the inline handlers so `script-src-attr` can go; the editor and the storage
  window were written without any inline handlers, use them as the pattern.
- **Tests** - a browser-level smoke test (Playwright) for the storage window and the editor; today
  the coverage is Go-level plus `check-ui`.

## 8. Conventions used here

- One item = one commit = one test; commit `feat(storages): STG-xx ...` + `Refs #19` (public `#155`).
- New tests: `v<version><n>_<what>_test.go` (here `v14100_*`, `v13152_*`).
- Every UI change runs `node tools/check-ui.js` (key parity between EN and HR is enforced).
- Roadmap status cells carry `done (<sha>)`; keep them updated with every merged item.
- Security-relevant code carries a comment saying *why*, not what; the audit log must never see a
  secret.

## 9. Where to look first

1. `docs/ROADMAP-STORAGES.md` - what exists and what it cost.
2. `docs/rfcs/0008-storages-s3.md` - why it looks like this.
3. `docs/security-reviews/0001-storages-s3.md` - what was checked and accepted.
4. `docs/STORAGES.md` - what an operator sees.
5. PR #20 / #159 - the diffs and the release plan comment.
