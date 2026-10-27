# Secure Software Development — SSDF Mapping

This document maps the project's secure-development practices to the NIST Secure Software Development Framework (SSDF) SP 800-218 practices. It is a summary; the authoritative practice lives in the referenced files and CI configuration.

| SSDF practice | Where it is done in this project |
|---------------|----------------------------------|
| Define security requirements (PO.1) | `docs/THREAT-MODEL.md`, `docs/PSIRT.md` |
| Identify and select secure components (PW.1) | Dependency review, SBOM generation in `docs/RELEASE.md` |
| Analyze components for vulnerabilities (PW.2) | `govulncheck` and dependency scanning in CI |
| Use compiler/toolchain hardening (PW.6) | Standard Go build hardening flags in CI |
| Review code for vulnerabilities (PW.7) | Mandatory maintainer code review; `gosec` static analysis in CI |
| Reuse existing secure software (PW.4) | Prefer established, maintained libraries over custom crypto |
| Build the code securely (PW.8) | Reproducible build in CI; signed release artifacts per `docs/KEYS.md` |
| Run static and dynamic analysis (PW.7, PW.8) | `gosec`, `govulncheck`, and lint in CI |
| Verify third-party compliance (PW.4) | Licence and SPDX checks in `docs/RELEASE.md` |
| Protect all forms of code from tampering (PO.3) | Signed tags and signed release artifacts |
| Manage vulnerabilities (RV.1) | `docs/PSIRT.md` process for triage and remediation |
| Fix vulnerabilities (RV.2) | Patch releases following `docs/PSIRT.md` response commitments |
| Prepare and publish advisories (RV.3) | `docs/PSIRT.md` and `docs/advisories/` |
| Respond to incidents (RV.1, RV.3) | `docs/INCIDENT.md` runbook |
| Document the SDLC (PO.1) | This file, `GOVERNANCE.md`, and `docs/rfcs/` |

CI gates: every pull request runs the test suite, `gosec`, `govulncheck`, lint, and a licence/SPDX check before merge.
