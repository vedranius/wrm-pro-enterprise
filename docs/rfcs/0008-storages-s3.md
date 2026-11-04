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

Add one S3-compatible endpoint per storage, browse it, preview and download objects, upload,
edit text objects, and delete only when the storage explicitly allows it.

**Non-goals for v1:** bucket administration (create/delete bucket, bucket policies, CORS),
object versioning UI, lifecycle rules, cross-region replication, presigned browser uploads,
Azure Blob / Google Cloud Storage, STS or instance-role credentials, S3 through jump hosts or
proxies.

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
  max_object_mb   INTEGER NOT NULL DEFAULT 512,
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
| list, read, preview, download | yes | yes | `storage_read` | yes (inside share prefix) |
| upload, edit, rename | `allow_write` | `allow_write` | `storage_write` + approval | `can_write` |
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
GET    /api/storages/{id}/object?key=           GET, HEAD with Range support
PUT    /api/storages/{id}/object?key=           raw body or multipart, If-Match on edit
POST   /api/storages/{id}/dir?prefix=           create an empty "folder"
POST   /api/storages/{id}/move                  {from, to, overwrite}
DELETE /api/storages/{id}/object?key=           needs allow_delete + X-WRM-Confirm
GET    /api/storages/{id}/preview?key=          text / image / PDF, as the file manager
```

`GET /objects` uses `ListObjectsV2` with `delimiter=/`: common prefixes become folders,
`NextContinuationToken` becomes the cursor. `PUT` streams to the client (multipart above
16 MiB, 8 MiB parts) and never buffers the whole object in memory. `DELETE` of a prefix is not
offered in v1: one object per call, `DeleteObjects` only for a multi-select the user
confirmed.

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
6. **Resource limits.** Per-object cap (`max_object_mb`), request body limits, a per-storage
   rate limit (60 operations/minute/user), 30 s for metadata calls, 10 min for transfers,
   multipart abort on failure.
7. **No indirect fetch.** No "import from URL", no presigned URLs in v1: the browser always
   goes through WRM, so the object ACL never has to be public.
8. **Shares.** Tokens are hashed at rest, prefix-scoped, read-only by default, with an
   expiry; a share never reveals the storage credentials.
9. **AI/MCP.** Separate scopes (`storage_read`, `storage_write`, `storage_delete`) so an AI
   connection approved for servers does not silently gain buckets; writes ask for approval,
   deletes are refused unless the scope is granted.

### UI

- A new left-nav section **Storages**, under Connections, with its own empty state.
- Add/edit dialog: name, endpoint (presets for Hetzner, DigitalOcean, AWS, MinIO), region,
  bucket, prefix, path-style (auto for non-AWS), access key, secret, **Test connection**,
  and the two capability switches. Delete is presented as what it is: permanent.
- A storage window reuses the file-manager shell: prefix breadcrumb, list (size, modified),
  preview, download, drag-and-drop upload, new folder, rename, editor with `If-Match` save
  ("the file changed in the meantime" instead of a silent overwrite).
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
