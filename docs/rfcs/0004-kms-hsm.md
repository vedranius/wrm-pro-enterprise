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
