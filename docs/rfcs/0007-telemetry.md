# RFC 0007 — Opt-in telemetry

- **Status:** draft
- **Author:** vedranius
- **Date:** 2026-10-28

## Problem

WRM learns nothing from the field: there is no signal about crashes, failed upgrades, or how
the product is actually used. Faza 6 wants anonymous, opt-in health/error reporting (L-57).

## Options considered

1. **Opt-in, anonymous telemetry** — a single periodic report of version, platform, and
   non-identifying counters (no hostnames, IPs, usernames, paths, or secrets).
2. **No telemetry** — no field signal.

**Decision:** option 1, strictly opt-in and off by default.

## Proposal

- Setting `telemetry` (`off` default; `basic`). When `basic`:
  - once a day, send `{version, platform, go, uptime, counters: {users, connections, sessions}}`
    over HTTPS to a configurable endpoint (`telemetry_endpoint`, default empty = off).
  - on a crash, offer to include the redacted stack.
- Nothing identifiable is ever included; the code that builds the report is a single,
  reviewable function so the exact payload is auditable.

## Security impact

- Off by default and GDPR-compliant by construction (no personal data). The endpoint is
  configurable, so a self-hosted collector is possible.

## Compatibility

- Additive.

## Status

Small and self-contained; implementable in-repo.
