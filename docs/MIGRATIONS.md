# Database Migrations

WRM stores state in a single database. Migration policy protects users from destructive or irreversible upgrades.

## Rules

- **Additive-only.** Migrations may add tables, columns, and indexes. They may not rename, remove, or change the type of existing columns in a minor or patch release.
- **No destructive changes in a minor or patch.** Destructive schema changes may only appear in a major release, and must be preceded by a deprecation period and an automated data-migration path.
- **One-way forward.** Each migration moves the schema forward by one version. The migration version is recorded in the database.
- **Tested.** Every migration runs in CI against a fresh and an upgraded database, including upgrades from the previous release.

## Upgrading

1. Stop the running binary.
2. **Back up** the database before upgrading.
3. Replace the binary with the new release.
4. Start the new binary. It applies pending migrations automatically on startup.

Keep both the database file and the encryption key. The key is required to decrypt the credentials vault; losing it makes vault data unrecoverable. Back up the key separately from the database and never store them together.

## Rolling back

Migrations are applied forward only and are not reversed by a newer release. To roll back an upgrade:

1. Stop the new binary.
2. Restore the database from the backup taken before the upgrade.
3. Run the **previous** binary against the restored database.

Because the previous binary's migrations match the restored schema, it will start normally. Do not run the previous binary against a database that a newer binary has already migrated, as that schema is ahead of what the older binary expects.

## Encrypted data

Migrations never touch vault ciphertext. The encryption key is independent of schema version, so an upgrade or rollback does not require re-encrypting the vault.
