# Release Signing Keys

WRM releases are cryptographically signed so users can verify authenticity and integrity.

## Key details

- **Algorithm** — Ed25519.
- **Key type** — release signing key, used only to sign release tags and release artifacts (binaries and checksums). It is not used for routine commits.
- **Custody** — the key has **two holders** (the steward and one maintainer), and an **offline backup copy** kept separate from any CI system. See *Key custody* below.
- **CI** — the private key lives only in the repository secret `RELEASE_SIGNING_KEY`, which is **never** exposed to pull-request workflows from forks. See *CI and fork safety* below.

## Where the public key lives

The public key is published in the repository at the top level alongside the release artifacts. Trust is established out of band; verify the key fingerprint against a second, independent source before relying on it.

## Verifying a signature

Each release publishes:

- The release artifacts.
- A checksums file (SHA-256) for the artifacts.
- A detached Ed25519 signature over the checksums file.

To verify:

1. Download the release artifacts, the checksums file, and the signature file.
2. Verify the checksums file against the public key.
3. Verify each artifact's SHA-256 hash against the checksums file.

The release tag itself is signed with the same key; verify the tag signature before checking out the source.

## Key compromise and rotation

If the release signing key is compromised or suspected compromised:

1. Stop signing with the key immediately.
2. Generate a new Ed25519 key.
3. Publish the new public key and announce the rotation in GitHub Discussions.
4. Re-sign the current and, where practical, prior release artifacts with the new key.
5. Revoke the old key and clearly state the fingerprint of the compromised key so users can identify artifacts signed before the compromise.
6. Review whether the compromise affects release authenticity and issue an advisory per `docs/PSIRT.md` if any release was tampered with.

## Key custody

- **Two holders** — the steward and one other maintainer, so a release can still be cut if either is unavailable.
- The private key exists in two places: an **offline copy** (hardware or air-gapped medium) and the CI secret `RELEASE_SIGNING_KEY` (encrypted at rest by GitHub). The offline copy is the recovery source of truth.
- Access is logged, and every rotation is documented and announced (see below).

## CI and fork safety

- The private key is provided to CI only as the repository secret `RELEASE_SIGNING_KEY` (*Settings → Secrets and variables → Actions*). It is never hard-coded or placed in a variable reachable by untrusted code.
- Fork PRs run with read-only tokens and cannot read repository secrets. Keep *Settings → Actions → Fork pull request workflows* at *Require approval for all external contributors*, so a fork PR can never trigger the signing job.
