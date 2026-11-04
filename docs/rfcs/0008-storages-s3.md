# RFC 0008 - Object storage (S3) as a separate "Storages" section

- **Status:** draft
- **Author:** vedranius
- **Date:** 2026-10-30

## Problem

WRM reaches servers over SSH, RDP, VNC and FTP/SFTP, but it cannot reach the object storage
that most of those servers write to: backups, uploads, media, logs and application data live
in S3-compatible buckets (Hetzner Object Storage, DigitalOcean Spaces, AWS S3, MinIO,
Backblaze B2). An operator who has to check a backup, fix a config file or remove a broken
upload leaves WRM, opens a provider console or a CLI, and comes back.

Doing nothing keeps a daily round trip outside the product and keeps object storage outside
WRM's audit log, its permission model and its AI/MCP layer.

## Options considered

1. **A new `Connection` protocol (`S3`) in the existing connections list.** Cheapest to
   write, but the `Connection` type feeds the terminal, jump hosts, tunnels, the SFTP pool,
   `tar` deploy, status monitoring, key deployment and AI sessions. Every one of those would
   need an `if isS3` branch, and users would see buckets mixed with servers.
2. **A separate top-level section "Storages" with its own table, routes, permissions, audit
   vocabulary and window.** More code, but nothing existing changes, and the permission
   model can be stricter than "you own the connection" (read / write / delete are separate).
3. **An addon that talks to S3 through the addon sandbox.** No core changes, but addons get a
   restricted sandbox without long-lived network credentials, cannot stream large objects,
   and cannot take part in audit, shares or AI approvals. Not viable for a first-class
   feature.

**Chosen: option 2.** Isolation is the point: an object store is not a server, and mixing the
two would weaken both.

## Proposal

### Scope of v1

Add one S3-compatible endpoint per storage, browse it by prefix, inspect and **edit small text
objects** (JSON, configs, subtitles, scripts), play a video object in the browser, download an
object, and delete objects only when the storage explicitly allows it.

WRM is not a transfer tool for this feature: it moves kilobytes, not gigabytes. The typical
jobs are fixing a JSON config, correcting a subtitle line, deleting a stale file and checking
that an uploaded MP4 is the right one.

**Non-goals for v1:** bulk upload, drag-and-drop of large files, multipart upload, transcoding
or HLS packaging, bucket administration (create/delete bucket, policies, CORS), object
versioning UI, lifecycle rules, cross-region replication, presigned browser URLs, Azure Blob /
Google Cloud Storage, STS or instance-role credentials, S3 through jump hosts or proxies.

### Data model (additive)

```sql
CREATE TABLE storages (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL,
  name            TEXT    NOT NULL,
  kind            TEXT    NOT NULL DEFAULT 's3',
  endpoint        TEXT    NOT NULL,              -- https://fsn1.your-objectstorage.com
  region          TEXT    NOT NULL DEFAULT '',   -- fsn1, nyc3, us-east-1 ...
  bucket          TEXT    NOT NULL,
  prefix          TEXT    NOT NULL DEFAULT '',   -- mandatory confinement, normalised
  path_style      INTEGER NOT NULL DEFAULT 1,    -- 1 for Hetzner, DO and MinIO
  access_key      TEXT    NOT NULL,
  secret_enc      TEXT    NOT NULL,              -- encryptValue(), never returned
  ca_fingerprint  TEXT    NOT NULL DEFAULT '',   -- pin a self-signed endpoint (SHA-256)
  sse             TEXT    NOT NULL DEFAULT '',   -- '' | AES256 | aws:kms
  allow_write     INTEGER NOT NULL DEFAULT 1,
  allow_delete    INTEGER NOT NULL DEFAULT 0,
  allow_upload    INTEGER NOT NULL DEFAULT 0,    -- small files only; off by default
  max_object_mb   INTEGER NOT NULL DEFAULT 16,   -- edit/save ceiling, not a transfer budget
  created_at      TEXT    NOT NULL,
  updated_at      TEXT    NOT NULL,
  UNIQUE(user_id, name)
);

CREATE TABLE storage_shares (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  storage_id  INTEGER NOT NULL,
  token_hash  TEXT    NOT NULL UNIQUE,
  prefix      TEXT    NOT NULL DEFAULT '',
  can_write   INTEGER NOT NULL DEFAULT 0,
  can_delete  INTEGER NOT NULL DEFAULT 0,
  expires_at  TEXT    NOT NULL DEFAULT '',
  created_by  INTEGER NOT NULL,
  created_at  TEXT    NOT NULL
);
```

`prefix` is not a convenience: every key is resolved inside it, so a storage configured as
`backups/2026/` can never address the rest of the bucket. Keys are validated (`..`, leading
`/`, control characters, length) before they reach the client.

### Permission matrix

| Action | Owner | Admin | AI (MCP scope) | Share token |
|---|---|---|---|---|
| list, read, preview, play, download | yes | yes | `storage_read` | yes (inside share prefix) |
| edit, save, rename | `allow_write` | `allow_write` | `storage_write` + approval | `can_write` |
| upload a small new file | `allow_upload` | `allow_upload` | `storage_write` + approval | `can_write` |
| delete | `allow_delete` + confirmation | same | `storage_delete` (off by default) | `can_delete` |
| add / edit / remove storage | yes | yes | no | no |

Capabilities are checked server-side in a `requireStorage(w, r, id, capability)` middleware,
never only in the UI.

### API (namespace of its own)

```
GET    /api/storages                            list (secrets masked)
POST   /api/storages                            create; runs a bucket preflight first
GET    /api/storages/{id}                       one storage (no secret)
PUT    /api/storages/{id}                       update; secret optional
DELETE /api/storages/{id}                       remove; revokes its shares
POST   /api/storages/{id}/test                  re-check credentials and bucket

GET    /api/storages/{id}/objects?prefix=&cursor=
GET    /api/storages/{id}/object?key=           GET, HEAD with Range; ?inline=1 for media
PUT    /api/storages/{id}/object?key=           save ≤ max_object_mb, If-Match on edit
POST   /api/storages/{id}/dir?prefix=           create an empty "folder"
POST   /api/storages/{id}/move                  {from, to, overwrite}
DELETE /api/storages/{id}/object?key=           needs allow_delete + X-WRM-Confirm
GET    /api/storages/{id}/preview?key=          text / image / PDF, as the file manager
```

`GET /object` streams with `Range` support and `Accept-Ranges: bytes`, which is what the
browser needs to play an MP4 and to jump inside it. `?inline=1` sets
`Content-Disposition: inline` for playable types; everything else is an attachment. No
transcoding and no HLS: the browser plays what the bucket stores, so a codec it does not
support (typically HEVC) is reported as such instead of being converted on the server.

`GET /objects` uses `ListObjectsV2` with `delimiter=/`: common prefixes become folders,
`NextContinuationToken` becomes the cursor. `PUT` accepts only bodies up to `max_object_mb`
(16 MB by default): this feature moves kilobytes, not backups, so there is no multipart and
nothing is buffered in memory. `DELETE` of a prefix is not offered in v1: one object per call,
`DeleteObjects` only for a multi-select the user confirmed.

### Editor

The editor is the centre of the feature, not an afterthought: what people change in a bucket
is a JSON config, a subtitle file or a small script.

- **Formats with a purpose-built mode:** JSON (pretty-print, validation with line/column of the
  error), YAML, XML, INI/conf, shell/Go/Python/JS (plain syntax), SRT, WebVTT, ASS/SSA.
  Everything else opens as plain text if it is valid UTF-8, and stays read-only otherwise.
- **Subtitle checks:** line numbering, timestamp sanity (monotonic, `-->`, milliseconds), a
  duplicate-index warning for SRT, and "renumber" / "shift timestamps by…" actions that run
  on the text before saving.
- **Encoding and newlines are preserved.** UTF-8 with or without BOM, CRLF vs LF: a save
  writes back the same shape it read, otherwise every diff in the project turns into noise.
  A file that is not valid UTF-8 opens read-only with an explicit message instead of being
  mangled.
- **Optimistic concurrency.** Saving sends `If-Match: <etag>`; a changed object answers 409
  and the UI offers "reload" or "overwrite anyway" (an explicit second action).
- **Limits.** Editing is capped (default 2 MB, `max_edit_kb`), with a clear message instead of
  a frozen tab. Formatting and validation run in the browser; the server stores bytes.
- Nothing is executed, rendered or interpreted server-side: the editor is text in, text out.

### Media and subtitles

- **Playback** uses the browser's `<video>` against `GET /object?inline=1`, which streams with
  `Range` support, so seeking works and the server never loads the file. `media-src 'self'` in
  the CSP already allows it.
- **Codecs are the provider's and the browser's business.** H.264/AAC plays; HEVC usually does
  not, and the UI says so with a download link rather than pretending. No transcoding, no HLS,
  no thumbnails in v1 (an optional `ffprobe`-based metadata line is a later nicety).
- **Subtitles next to the video.** If an object `x.mp4` has a sibling `x.srt`, `x.vtt` or
  `x.ass`, the player offers it as a track. WebVTT is native; SRT and ASS/SSA are converted to
  VTT **in the browser** for display only, so the file on the bucket is never touched. Editing
  the subtitle opens the editor on the original format.
- Sizes: a 2 GB MP4 plays fine because it is streamed; only text objects are ever read
  whole.

### Security impact

New outbound network egress and new long-lived secrets. Controls:

1. **Secrets.** Encrypted at rest with the installation key, write-only in the UI, redacted
   in audit and in the config export, never logged. Same path as connection passwords.
2. **TLS.** System roots by default. An endpoint with a self-signed certificate is accepted
   only when its SHA-256 fingerprint is pinned for that storage; there is no global
   "ignore certificate" switch.
3. **Egress.** The endpoint goes through `guardEgressURL()`. A MinIO on a LAN address needs
   `egress_allow_hosts` (or `egress_policy=allow_private`, the default).
4. **Confinement.** Prefix check plus key validation, in one place, before any request.
5. **Delete gate.** `allow_delete` is off by default; a delete request must also carry
   `X-WRM-Confirm: delete`, is audited with key and size, and can require a step-up
   confirmation (2FA) when the installation uses one.
6. **Resource limits.** Writes are capped (`max_object_mb`, default 16 MB) and edits tighter
   still (`max_edit_kb`, default 2 MB); request bodies are limited, a per-storage rate limit
   (60 operations/minute/user) applies, metadata calls time out after 30 s and a stream after
   10 minutes.
7. **Dangerous content is never rendered in the WRM origin.** HTML, SVG, XML, JS and anything
   the browser could execute are served as `text/plain` or as an attachment, never inline: an
   SVG or HTML preview would run with the signed-in user's session. The existing preview rules
   for remote files are reused, not re-invented.
8. **The editor does not execute anything.** It reads text and writes text; validation and
   formatting run in the browser, the server only checks size and etag.
9. **No indirect fetch.** No "import from URL", no presigned URLs in v1: the browser always
   goes through WRM, so the object ACL never has to be public.
10. **Shares.** Tokens are hashed at rest, prefix-scoped, read-only by default, with an
    expiry; a share never reveals the storage credentials.
11. **AI/MCP.** Separate scopes (`storage_read`, `storage_write`, `storage_delete`) so an AI
    connection approved for servers does not silently gain buckets; writes ask for approval,
    deletes are refused unless the scope is granted.

### UI

- A new left-nav section **Storages**, under Connections, with its own empty state.
- Add/edit dialog: name, endpoint (presets for Hetzner, DigitalOcean, AWS, MinIO), region,
  bucket, prefix, path-style (auto for non-AWS), access key, secret, **Test connection**,
  and the capability switches (edit, small upload, delete). Delete is presented as what it is:
  permanent.
- A storage window reuses the file-manager shell: prefix breadcrumb, list (size, modified),
  preview, **player for video objects**, download, new folder, rename, delete (when allowed)
  and the **editor** with `If-Match` save ("the file changed in the meantime" instead of a
  silent overwrite).
- Upload is a small "add a file" action (default off), not a transfer manager: a progress
  bar for a 3 MB JSON is enough; there is no queue, no resume and no multipart.
- Actions the storage does not allow are absent, not merely disabled.

### Compatibility

Additive only: two new tables, new routes, a new setting `storages_enabled` (default off)
and new audit actions. No existing table, route or export format changes. An installation
that never enables the feature behaves exactly as before.

### Migration

`ensureTable` creates `storages` and `storage_shares` on start, like the other feature
tables. Rollback: disable `storages_enabled`, then drop both tables; nothing else references
them. Import/export of the WRM configuration gains an optional `storages` array with secrets
removed.

### Alternatives rejected

- **S3 as a connection protocol** (option 1): leaks into terminal, tunnels, deploy and
  monitoring, and makes permissions coarser.
- **Addon** (option 3): no streaming, no audit, no approvals.
- **Mount with rclone/s3fs on the server:** adds a system dependency, hides errors from the
  user and gives the whole server the credentials.

## Open questions

1. Should an administrator see (and be able to open) every user's storage, as with
   connections, or only their own? Proposal: same rule as connections (owner plus admin).
2. Do we need object versioning ("undelete") in v2, or is a provider-side versioning hint in
   the UI enough?
3. Should uploads above a threshold go straight to the provider with a presigned URL to save
   WRM bandwidth? Proposal: no in v1, revisit after measuring.
