# RFC 0003 — Managed password rotation (Windows/RDP)

- **Status:** draft
- **Author:** vedranius
- **Date:** 2026-10-28

## Problem

WRM stores and uses credentials, but does not rotate them. Privileged-access-management (PAM)
expectation is that service/RDP passwords rotate on a schedule and after use. Faza 6 lists
this as a gap (L-55: "password rotation — at least Windows via an agent").

## Options considered

1. **Windows agent** — a small agent on each managed Windows host that rotates the local
   account password under a schedule and reports the new value back to WRM (encrypted).
2. **Agentless via WinRM/SMB** — rotate without an agent. Requires exposing WinRM and
   carrying credentials with the necessary rights; brittle across environments.
3. **Do nothing** — does not close the gap.

**Decision:** option 1. A minimal Windows agent is the most reliable and least-privileged
path. It is the only component that needs to run on the managed host; everything else stays in
WRM.

## Proposal

- A small, auditable Windows agent (Go, same codebase) that:
  - registers with WRM over an authenticated channel (a scoped AI/API token),
  - rotates a named local account password on a schedule and after use,
  - reports the new password back encrypted with the WRM server's key (never in plaintext).
- WRM stores the rotated password in the existing encrypted credentials vault and updates the
  matching connection.
- Rotation is audited (`credential.rotated`) and failures raise an alert.

## Security impact

- The agent holds no long-lived secrets: it receives a one-time scope and reports results only.
- The new password travels encrypted; it is never written to logs.

## Compatibility

- Additive; Windows-only. Linux/macOS rotation can follow the same agent pattern later.

## Migration

- Operators install the agent on the hosts they want rotated; nothing changes for other hosts.

## Alternatives rejected

- Agentless WinRM: too much surface area and privilege.

## Status

Not yet implemented. Requires a Windows build/test environment and a real Windows host to
verify rotation. This RFC captures the design.
