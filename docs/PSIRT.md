# PSIRT — Product Security Incident Response Team

This document defines how security vulnerabilities in WRM are reported, triaged, and disclosed.

## Roles

- **Security lead** — the steward (vedranius), or a maintainer designated by the steward. Owns the process, assigns severity, and manages disclosure.
- **Maintainers** — assist with reproduction, patching, and advisory authoring. See `MAINTAINERS.md`.

## Receiving reports

All reports come through GitHub **private vulnerability reporting** on the repository: Security → Report a vulnerability, at `https://github.com/vedranius/wrm-pro-enterprise`.

- Do not report vulnerabilities in public issues.
- The reporter receives a private thread where the whole process takes place.
- Only the steward and designated maintainers have access to the report.

Additional channels, configured by the steward and recorded here once live:

- **`security@`** — an email alias forwarded to the steward and at least one maintainer, used when GitHub private reporting is not available or for encrypted follow-up.
- **PGP** — a published fingerprint for encrypted communication. The key and fingerprint are owned by the steward and published here (and in `docs/KEYS.md`) when available.

## Severity classification

| Severity | Definition |
|----------|------------|
| Critical | Remote unauthenticated code execution or full compromise without user interaction; mass secret exposure |
| High | Authenticated code execution, privilege escalation, or significant secret exposure |
| Medium | Limited exposure or exploitation requiring specific conditions or privileges |
| Low | Minor issues with limited security impact; defense-in-depth gaps |

Severity follows a CVSS-style assessment but is ultimately decided by the security lead, considering WRM's self-hosted deployment context.

## Response commitments

Response targets by severity:

| Severity | Acknowledge | Triage | Fix | Publish |
|---|---|---|---|---|
| Critical | 24 h | 48 h | 72 h | immediately after release |
| High | 2 days | 5 days | 14 days | with the release |
| Medium | 5 days | 10 days | 30 days | with the release |
| Low | 10 days | 30 days | next release | as needed |

These are targets, not guarantees. If a fix is delayed, the reporter is kept informed.

## Disclosure window

Public disclosure follows the fix release. The default window is **90 days** from acknowledgement,
or a date agreed with the reporter. Advisories are published as GitHub Security Advisories at the
same time the fixed release goes out.

## Embargo

The report is confidential until disclosure. Reporters must not disclose the vulnerability publicly until the fix is released and the advisory is published. The project will keep the reporter informed and agree on a disclosure timeline.

## Coordinated disclosure

1. Reporter submits via private vulnerability reporting.
2. Security lead acknowledges and triages.
3. Maintainers develop and privately test a fix.
4. A patched release is prepared per `docs/RELEASE.md`.
5. The advisory is published and the fix is released together.

## Advisory publication

Advisories are published as GitHub Security Advisories on the repository and recorded in `docs/advisories/`. Use the template in `docs/advisories/TEMPLATE.md`. Credit is given to the reporter unless they request otherwise.
