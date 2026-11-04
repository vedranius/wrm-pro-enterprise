# Security review 0001 - Storages (S3)

- **Feature:** S3-compatible object storage as its own section (RFC 0008, roadmap STG-01 … STG-12)
- **Reviewer:** security lead (self-review, no external pentest yet)
- **Date:** 2026-11-04
- **Code:** branch `feat/storages`, PR #20
- **Verdict:** approved for a minor release; residual risks below are accepted and tracked

## Scope

New code: the storage model and CRUD (`storages.go`), the S3 client (`storage_s3.go`), the object
routes (`storages_http.go`), share links (`storage_shares.go`), the MCP tools
(`mcp_storages.go`), the browser module (`static/storages.js`) and the wiring in `main.go`,
`settings.go`, `notify.go`, `mcp_server.go`, `mcp_tokens.go`, `static/index.html` and
`static/mcp.js`.

Threats considered: SSRF from an administrator-entered endpoint, escaping the storage prefix,
credential leakage, silent overwrite, destructive delete, active content (HTML/SVG/JS) rendered in
the WRM origin, AI access through a server approval, certificate impersonation of a self-signed
endpoint, token guessing on a share link.

## Checks and evidence

| # | Check | Result | Evidence |
|---|---|---|---|
| 1 | Secret never leaves the server (API, audit, export, log) | pass | `Secret` is `json:"-"`, storage export strips it, `TestStorageCRUDAndSecretMasking`, `TestStorageExportRedactsSecretsAndImport` |
| 2 | Prefix cannot be escaped by any key | pass | `validStorageKey` + `s3Client.fullKey`, `TestStorageKeysStayInsideThePrefix`, `TestStorageShareToken` (traversal and a valid key outside the share prefix) |
| 3 | Endpoint passes the egress guard | pass | `storageEndpoint` → `guardEgressURL`; `TestStorageValidationRefusesTraversal` includes `169.254.169.254` |
| 4 | TLS pinning cannot be bypassed by session resumption | pass | `VerifyConnection` + `SessionTicketsDisabled` (gosec G123 clean) |
| 5 | Active content is never rendered inline | pass | `unsafeInline` → `text/plain` + attachment; share page has no script at all |
| 6 | Delete needs permission, confirmation and step-up | pass | `allow_delete` + `X-WRM-Confirm: delete` + `requireStepUp`; `TestStorageDeleteNeedsConfirmation`, `TestStorageDeleteRequiresStepUp` |
| 7 | Stale writes cannot overwrite silently | pass | `If-Match` and 409; `TestStorageTextKeepsBOMAndLineEndings` |
| 8 | AI access is scoped and read-deny applies | pass | `storage_read/write/delete` scopes, `aiReadDenied`; `TestStorageMCPTools` |
| 9 | Rate limits and size caps are enforced | pass | `requireStorage` (120/min), `max_object_mb`, `storage_edit_kb`; 413 test |
| 10 | Share tokens are unguessable, scoped, expiring and revocable | pass | 24-byte token, `tokenHash` at rest, prefix-narrowed storage, expiry check, revoke; `TestStorageShareToken` |
| 11 | gosec/staticcheck clean for the new code | pass | CI `staticcheck & gosec` green; the two G704 reports are annotated as the admin-validated endpoint |

## Findings

No open vulnerabilities. Two notes from the review, both fixed before the PR was finished:

1. **Pinned TLS and session resumption** (gosec G123). A `VerifyPeerCertificate` pin is skipped for a
   resumed session. Fixed by moving the check to `VerifyConnection` and disabling session tickets.
2. **AI read path bypassed the read-deny list.** `storage_read` returned any text object, including
   `.env` and keys. Fixed by applying `aiReadDenied` to storage keys and extending the test.

## Residual risks (accepted)

| Risk | Why accepted | Follow-up |
|---|---|---|
| `script-src-attr 'unsafe-inline'` still allows inline handlers | removing ~150 handler attributes is a separate refactor (UX-05) | tracked in `ROADMAP-FIXES.md` |
| A share link has no password, only a token and an expiry | the token is 192 bits, the page is read-only and revocable; a password is a v2 nicety | `ROADMAP-STORAGES.md` |
| No antivirus or content scan on download | WRM never executes the object; the browser downloads it as a file | optional addon later |
| `egress_policy` defaults to `allow_private` | on-premise MinIO is a legitimate target; an administrator can require `public_only` | documented in the RFC |
| Self-review, not an external pentest | consistent with the project's current process | schedule an external review with the first production deployment |

## Sign-off

The feature may ship as a minor release with `storages_enabled` as the kill switch. The threat
model (`docs/THREAT-MODEL.md`) carries the new rows.
