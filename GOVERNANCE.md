# WRM Governance

This document describes how decisions are made in the WRM project. WRM is a community-governed project with a single steward, vedranius, who holds final authority and is the primary copyright holder.

## Roles

- **Steward** — vedranius. Holds final decision-making authority, owns the trademark and release signing key, and is the copyright holder for the project.
- **Maintainers** — trusted contributors who review and merge changes and set technical direction. Listed in `MAINTAINERS.md`.
- **Contributors** — anyone who submits patches, documentation, or other work.

See `MAINTAINERS.md` for the full ladder and advancement criteria.

## Decision-making model

- **Minor changes** (typos, small fixes, documentation) use lazy consensus: a maintainer may merge if no one objects within a reasonable review window.
- **Major changes** (new features, breaking changes, architectural decisions) require maintainer consensus. A proposal is approved when maintainers reach consensus in the discussion and no maintainer objects with a technical justification.
- **Tie-breaking** — if maintainers cannot reach consensus, the steward decides.

## RFC process

Significant changes — new subsystems, breaking API changes, changes to the threat model, or anything that benefits from up-front design review — go through the RFC process described in `docs/rfcs/README.md`. RFCs are discussed in GitHub Discussions or issues and merged into `docs/rfcs/` once accepted.

## Licence changes

- Changing the licence of *future* releases requires maintainer consensus plus steward sign-off.
- Past releases are never retroactively re-licensed. A release remains under the licence it was published with. Versions up to v13.x remain under their original PolyForm terms; v14.0.0 and later are Apache-2.0.

## Code of Conduct

All participants are expected to follow the `CODE_OF_CONDUCT.md`. The steward and maintainers are responsible for enforcing it.
