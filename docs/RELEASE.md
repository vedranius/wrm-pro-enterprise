# Release Checklist

Steps to cut a new WRM release. Run in order.

1. **Tests** — full test suite passes in CI on all supported platforms.
2. **Static analysis** — `gosec` and `govulncheck` pass with no new findings; lint is clean.
3. **Changelog** — update the changelog with all user-facing changes since the last release.
4. **Version** — set the version and confirm it matches the changelog and any SPDX metadata.
5. **Licence/SPDX check** — confirm all files and dependencies carry valid SPDX identifiers and comply with Apache-2.0 for v14.0.0 and later.
6. **SBOM** — generate the software bill of materials and attach it to the release.
7. **Database migration review** — confirm migrations are additive-only per `docs/MIGRATIONS.md` and covered by tests.
8. **Tag** — create an annotated tag, signed with the release signing key per `docs/KEYS.md`.
9. **Artifacts** — build release artifacts, compute checksums, and sign them with the release signing key.
10. **Release notes** — publish GitHub release notes summarizing features, fixes, and upgrade notes.
11. **Announcement** — announce the release in GitHub Discussions.
12. **Security** — if the release contains security fixes, follow `docs/PSIRT.md` for coordinated disclosure and advisory publication.

The signing key and verification procedure are documented in `docs/KEYS.md`.

## Canary and rollback

Before a release reaches everyone, it rolls out in stages:

1. **Staging** — the release runs against a staging or copy database and a representative set of connections.
2. **Canary** — one production installation (or a small subset) upgrades first and is watched for healthcheck failures and error rates.
3. **Full rollout** — the remaining installations upgrade after the canary is stable.

Rollback is automatic when the healthcheck fails: restore the database backup (see `docs/MIGRATIONS.md`) and run the previous binary. Migrations are forward-only, so rollback is always *restore backup + previous binary*, never an in-place downgrade.

The self-updater verifies the release signature and SHA-256 before installing (see `docs/KEYS.md`), so a tampered or incomplete release cannot be applied.

## Coverage target

Critical packages (authentication, authorization, crypto, sessions — `security.go`, `users.go`, `totp.go`, `ai_engine.go`, `ai_policy.go`, `recording.go`) should keep **≥ 80 %** line coverage. The `Coverage` job in `security.yml` reports the current numbers; once the baseline is known, flip that job to fail below the threshold.
