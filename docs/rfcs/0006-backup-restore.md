# RFC 0006 — Automated backup and restore

- **Status:** draft
- **Author:** vedranius
- **Date:** 2026-10-28

## Problem

Backup is currently a manual step (copy the database file and the encryption key, kept
separately). Faza 6 requires an in-product backup/restore with a recovery-time objective
(RTO) under one hour and a recovery-point objective (RPO) under 15 minutes (L-56).

## Options considered

1. **Built-in backup/restore** — WRM writes a consistent backup (database + encrypted key
   material) to a configurable location or a downloadable archive, and can restore it.
2. **Documentation only** — does not meet the RTO/RPO goal.

**Decision:** option 1.

## Proposal

- `POST /api/admin/backup` (or a `wrm -backup` command): a consistent snapshot using SQLite's
  online backup, plus the key material, into one encrypted archive (or to `backup_path`).
- `POST /api/admin/restore` / `wrm -restore`: stop, replace, verify, restart; restore from a
  validated archive.
- Scheduled backups with retention, and a restore test (restore to a temp location and verify)
  so a backup that cannot be restored is reported, not discovered during an incident.

## Security impact

- The backup contains secrets; it must be encrypted with a key separate from the one being
  backed up, and stored separately.

## Compatibility

- Additive.

## Status

Implementable in-repo. The RTO/RPO numbers are operational targets verified on a real install.
