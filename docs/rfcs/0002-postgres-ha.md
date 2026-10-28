# RFC 0002 — PostgreSQL / high-availability backend

- **Status:** draft
- **Author:** vedranius
- **Date:** 2026-10-28

## Problem

WRM stores everything in a single SQLite database. SQLite is a single point of failure: one
file, one writer, no replication. A production deployment that needs 99.9 % availability or
several WRM nodes behind a load balancer cannot use it. Faza 6 lists this as a blocking gap
(L-53: "HA / Postgres — failover test").

## Options considered

1. **PostgreSQL backend** — replace the SQLite driver with Postgres. Real replication,
   failover, and backups, at the cost of migrating the SQL and the dialect-specific queries.
2. **SQLite + Litestream/replication** — replicate the SQLite file. Simpler, but still a
   single writer and weaker failover.
3. **Keep SQLite** — does not close the gap.

**Decision:** option 1. Add a PostgreSQL backend behind the same `database/sql` interface,
selected by `DB_DRIVER`/`DB_PATH` (or `DATABASE_URL`). SQLite remains the default for small
deployments; Postgres is for HA.

## Proposal

- Introduce a thin repository/DAO layer over `database/sql` so the same code runs on both
  drivers, or audit every query for SQLite-specific syntax (`?` placeholders, `PRAGMA`,
  `INSERT ... ON CONFLICT`, `ALTER TABLE ... ADD COLUMN`, `strftime`).
- Migrations become an ordered, versioned list applied on startup (instead of the current
  idempotent `CREATE TABLE IF NOT EXISTS` / `ALTER TABLE ... ADD COLUMN`), because Postgres
  `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` differs and idempotent DDL is harder to express.
- Document the failover topology (primary + standby, `pg_auto_failover` or managed Postgres)
  and the failover test.

## Security impact

- None new. The connection string holds credentials and must be a secret, not in the binary.
- The migration to a versioned schema is a one-time, carefully-tested change; it must not
  silently drop or retype columns.

## Compatibility

- The SQLite path must keep working unchanged; Postgres is additive.

## Migration

- Operators stay on SQLite by default. Switching to Postgres is a documented migration: dump
  and reload, or a one-time import.

## Alternatives rejected

- Litestream: real, but still a single writer; weaker failover story than Postgres.
- A bespoke replication layer: not worth it when Postgres exists.

## Status

Not yet implemented. This RFC captures the design; the implementation is a dedicated
engineering effort (audit of every query + a versioned migration system) and requires a real
Postgres for the failover test.
