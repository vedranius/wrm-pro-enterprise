# Storages (S3) - operator guide

WRM can browse and edit S3-compatible object storage next to your servers. It is a section of its
own: a storage is not a Connection and never reaches the terminal, tunnels, jump hosts or the
deploy code.

Design: `docs/rfcs/0008-storages-s3.md`. Work items and status: `docs/ROADMAP-STORAGES.md`.

## Add a storage

**Storages → Add storage.** What to enter depends on the provider:

| Provider | Endpoint | Region | Path-style |
|---|---|---|---|
| Hetzner Object Storage | `https://<location>.your-objectstorage.com` (e.g. `fsn1`) | `fsn1`, `nbg1`, `hel1` | on |
| DigitalOcean Spaces | `https://<region>.digitaloceanspaces.com` | `nyc3`, `ams3`, `fra1`, `sgp1` | on |
| Amazon S3 | `https://s3.<region>.amazonaws.com` | e.g. `eu-central-1` | off |
| MinIO | `https://minio.example.lan:9000` | what the server uses (`us-east-1` by default) | on |
| Backblaze B2 (S3 API) | `https://s3.<region>.backblazeb2.com` | e.g. `eu-central-003` | off |

- **Prefix** is mandatory in practice: set it to the folder the storage may touch (e.g.
  `team-a/backups`). Every key is resolved inside it, so the rest of the bucket is unreachable,
  even with the API.
- **Access key / secret key:** use a key scoped to that bucket and prefix. WRM encrypts the
  secret with the installation key; it is never returned to the browser, written to the audit log
  or included in a configuration export (unless you explicitly export secrets).
- **Test connection** checks the endpoint, the bucket and the key permissions before saving.
- A self-signed endpoint (MinIO) needs its **SHA-256 certificate fingerprint** in the dialog; there
  is no global "ignore certificate" switch.
- A LAN endpoint must pass the egress policy: it is allowed by default (`egress_policy=allow_private`);
  with `public_only`, add the host to `egress_allow_hosts`.

## Permissions inside a storage

| Switch | Default | What it allows |
|---|---|---|
| (always) | on | list, preview, download, play video |
| Allow editing files | on | save an existing object from the editor |
| Allow adding new files | **off** | upload a new small file |
| Allow deleting files | **off** | delete an object (also asks for a confirmation, and for the step-up password/2FA when the installation enforces it) |

Actions a storage does not allow are hidden in the UI and refused by the API. The switches are
per storage; an administrator can turn the whole section off with `storages_enabled=0`.

## What the browser does

- Lists prefixes like folders. Objects: preview (images), play (video through Range requests),
  download, edit, rename, delete.
- **Editor:** JSON (format, validate), YAML/XML/INI, scripts, and subtitles (SRT, WebVTT, ASS/SSA)
  with a timing check, a shift and a renumber. Saves keep the BOM and the line endings and use
  `If-Match`, so a file changed in the meantime is a conflict, not a silent overwrite.
- **Video:** the browser plays what the bucket stores. H.264/AAC works; HEVC usually does not, and
  WRM says so instead of transcoding. A sibling `x.srt`, `x.vtt` or `x.ass` is offered as a
  subtitle track (SRT/ASS converted to VTT in the browser; the stored file is untouched).
- WRM is not a transfer tool here: uploads are for small files (16 MB per object by default,
  2 MB in the editor). Large media is streamed, never buffered.

## Share links

**Share** in the storage window creates a read-only link to one prefix:

- the token is random, stored hashed, and shown to you once;
- the link expires (default 24 h, at most 30 days) and can be revoked at any time;
- the page is plain server-rendered HTML without a script; visitors can list and download inside
  the shared prefix only.

## AI / MCP

An AI connection reaches a storage only through its own scopes:

| Scope | Tools |
|---|---|
| `storage_read` | `storage_list`, `storage_read` |
| `storage_write` | `storage_write` (with `allow_write` and an optional etag) |
| `storage_delete` | `storage_delete` (with `allow_delete` and `confirm="delete"`) |

The scopes are opt-in: an administrator adds them to `ai_mcp_scopes`, then the user grants them to
an AI connection. The AI read-deny list (`.env`, keys, certificates) applies to storage reads as
well.

## Status and notifications

WRM checks each storage's bucket in the background (`HeadBucket`, every 5 minutes) and shows a
green or red dot in the list. A storage that stops answering raises a `storage.down` notification
for its owner, and `storage.up` when it comes back (Notifications → events).

## Troubleshooting

| Message | Cause |
|---|---|
| "The bucket does not exist (check the name and the region)" | wrong bucket, wrong region, or the endpoint is not the S3 API |
| "Access denied: the key may not allow this operation" | the key policy does not allow list/get/put on that prefix |
| "The secret key or the region does not match the endpoint" | region mismatch (Hetzner uses the location code) |
| "The endpoint is in another region" | the bucket lives in another region than the one entered |
| "The certificate fingerprint does not match" | the endpoint certificate changed; verify it and update the pin |
| "This storage does not allow …" | the capability switch is off for that storage |

## Backup and rollback

The two tables (`storages`, `storage_shares`) are created on start; nothing else changes. A
database backup covers them. To remove the feature, set `storages_enabled=0`; to remove the data
as well, drop the two tables.
