# RFC 0001 — WebAuthn / FIDO2 passkeys (second factor + step-up)

- **Status:** in-review
- **Author:** vedranius
- **Date:** 2026-10-28

## Problem

The only available second factor is TOTP (RFC 6238). TOTP codes are shared secrets that are
phishable — a convincing login page can collect a code and replay it. Enterprise and
security-conscious users increasingly require hardware-backed authenticators (security keys) or
platform passkeys. Faza 6 lists this as the first production-maturity gap (L-50).

## Options considered

1. **WebAuthn as second factor only** — keep the password, add a passkey challenge at login and
   step-up, exactly where TOTP sits today. Smallest change, lowest risk.
2. **Full passwordless** (discoverable/resident credentials) — replace the password entirely.
   Bigger UX and account-recovery redesign; depends on backup/recovery policy.
3. **Leave TOTP** — does not close the phishing gap.

**Decision:** option 1. WebAuthn is a second factor for sign-in and step-up, side by side with
TOTP. Passwordless stays out of scope of this RFC (a follow-up once backup/recovery and Faza 6's
other items land).

## Proposal

- Add a `webauthn_credentials` table: `user_id`, `credential_id`, `public_key`, `sign_count`,
  `name`, `transports`, `aaguid`, `created_at`.
- Use the [`go-webauthn/webauthn`](https://github.com/go-webauthn/webauthn) library for
  registration and assertion; the RP ID is the request host (overridable), the origin is the
  request origin.
- **Registration** requires a fresh step-up (password + existing 2FA), mirroring how TOTP
  enrollment works. At most two credentials per user (the L-50 acceptance criterion is "login +
  step-up with 2 authenticators"); a user may register a second only after the first.
- **Sign-in:** after a correct password, if the user has passkeys, the MFA challenge offers
  WebAuthn; the browser performs the assertion and the server verifies it, enforcing the
  credential's sign count for replay protection.
- **Step-up:** `POST /api/auth/step-up` accepts a WebAuthn assertion as an alternative to a TOTP
  code.
- Every registration and assertion is audited (`auth.webauthn_*`).

## Security impact

- The challenge is a short-lived signed nonce bound to the user and the operation; it is never
  reusable and never returned as a decision.
- Assertions are verified against the stored public key; a reused or cloned credential is
  detected via sign count.
- Registration is gated behind step-up, so a stolen session cannot silently add a passkey.
- A passkey is bound to its user at registration and is only usable for that user.

## Compatibility

- Additive: a new table and new columns; no existing schema or API is changed.
- TOTP remains available; users keep at least one recovery path (recovery codes) as today.

## Migration

- `webauthn_credentials` is created idempotently on startup alongside the existing migrations.
- No operator action; the feature is off until a user registers a passkey.

## Alternatives rejected

- Passwordless (resident credentials) — deferred; needs the recovery/backup redesign first.
- In-house WebAuthn crypto — rejected; use the maintained library rather than hand-rolling the
  CBOR/attestation parsing.
