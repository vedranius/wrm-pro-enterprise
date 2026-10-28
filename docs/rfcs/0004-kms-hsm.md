# RFC 0004 — KMS/HSM key wrapping and rekey

- **Status:** draft
- **Author:** vedranius
- **Date:** 2026-10-28

## Problem

The key that encrypts stored secrets (the credentials vault, provider keys) lives in a file
next to the database. Whoever reads the disk reads both. A key management service (AWS KMS,
Azure Key Vault, HashiCorp Vault, an HSM via PKCS#11) would let the encryption key itself be
wrapped by a key the operator does not hold directly, and enable rekey without re-encrypting
every record. Faza 6 lists this as the largest risk (L-52).

## Options considered

1. **Envelope encryption behind a KMS interface** — a data key encrypts secrets; the data key
   is wrapped by a KMS-held key. Providers: file (today), AWS KMS, Azure Key Vault, Vault
   transit, PKCS#11.
2. **Full HSM for all crypto** — slow, expensive, unnecessary for a self-hosted tool.

**Decision:** option 1. Keep the current file-based key as the default; add a `KMS` interface
and the providers later. The deliverable now is the interface, the file provider, and a
documented rekey + recovery procedure.

## Proposal

- A `KeyProvider` interface: `Wrap(plaintext) (ciphertext, error)` and
  `Unwrap(ciphertext) (plaintext, error)`.
- The file provider is the current behaviour, unchanged.
- Settings: `kms_provider` (`file`, `aws`, `azure`, `vault`), plus provider-specific config.
- `rekey` command/endpoint that unwraps with the old key and rewraps with the new key, with a
  full backup first, audited.

## Security impact

- The data key is no longer the only secret; the KMS key becomes the root of trust.
- Recovery must be documented and tested: losing the KMS key without a wrapped backup = data
  loss.

## Compatibility

- Additive; the file provider is the default, so nothing changes for existing installs.

## Status

The interface and file provider are the near-term deliverable; the AWS/Azure/Vault providers
and the rekey test need the respective external service.

## Rekey and recovery procedure (documented, to be tested with a real provider)

1. **Before any rekey**, write a full backup (`wrm -backup`) and verify it (`PRAGMA
   integrity_check`). Keep the old key available until the rekey is confirmed.
2. **Rekey** unwraps every row encrypted with the current data key and rewraps it with the new
   one, in a single transaction (or resumable batches with a progress marker), then swaps the
   active provider. A row that fails to unwrap stops the job; nothing is half-rekeyed.
3. **Recovery** from a failed rekey = restore the backup + the matching key. The backup and its
   key must be stored separately (see `docs/rfcs/0006-backup-restore.md`).
4. **KMS provider**: the operator supplies the KMS key id/ARN via a secret; the data key is
   wrapped by the KMS and never leaves it in plaintext. Losing the KMS key without a wrapped
   backup means the vault cannot be recovered — this must be stated in the operator's runbook.

Because the actual unwrap/rewrap needs a live KMS or HSM, this procedure is verified as part of
the provider implementation, not in CI.
