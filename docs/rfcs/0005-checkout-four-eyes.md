# RFC 0005 — Checkout / four-eyes for stored secrets

- **Status:** draft
- **Author:** vedranius
- **Date:** 2026-10-28

## Problem

Revealing a stored credential (a vault entry, a saved password, a private key) is a single
user action. Auditors increasingly require "four-eyes": a second, independent person must
approve before a highly privileged secret is used. Faza 6 lists this (L-54).

## Options considered

1. **Checkout with approval** — a user requests a secret; an approver (a second, distinct
   user) approves or denies; the secret is then revealed to the requester only, once.
2. **Dual control everywhere** — impractical for day-to-day use.

**Decision:** option 1, scoped to secrets the administrator marks as "dual control".

## Proposal

- A per-secret flag `dual_control`. When set, revealing it requires a checkout:
  - requester POSTs a checkout request (with MFA/step-up),
  - an approver (a different user with the right) approves or denies,
  - on approval the requester may read the secret once (it is never shown to the approver),
  - the checkout is audited end-to-end (`credential.checkout_requested/approved/denied/used`).
- Checkout requests expire; approval of your own request is refused.

## Security impact

- A single compromised account can no longer exfiltrate a protected secret alone.
- The approver sees only that a request exists, never the secret itself.

## Compatibility

- Additive; secrets default to single-control, so nothing changes until enabled.

## Status

Implementable in-repo without external services. The reveal flow is in `remote-manager/credentials.go` and `keys.go`.
