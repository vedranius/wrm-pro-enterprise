# Maintainers

## Maintainer ladder

### Contributor

Anyone who submits a pull request, issue, documentation change, or other contribution.

Responsibilities:

- Follow the `CODE_OF_CONDUCT.md`.
- Sign contributions with DCO `Signed-off-by` and comply with the `CLA.md`.
- Respond to review feedback.

### Reviewer

A contributor whose reviews are consistently useful and accurate.

Responsibilities:

- Review pull requests for correctness, security, and style.
- Triage issues and help new contributors.
- Ask for help when unsure rather than guessing.

How to advance: sustained, high-quality reviews over several months, nominated by a maintainer and approved by the steward.

### Maintainer

A trusted, long-term contributor with merge rights.

Responsibilities:

- Merge pull requests that meet the quality bar.
- Enforce the governance model in `GOVERNANCE.md`.
- Lead a component or subsystem.
- Cut releases following `docs/RELEASE.md`.
- Respond to security reports per `docs/PSIRT.md`.

How to advance: sustained contribution plus review record, sponsorship by an existing maintainer, and approval by the steward.

### Security maintainer

A maintainer granted merge rights on the security-critical paths listed in `.github/CODEOWNERS`
(auth, crypto, sessions, MCP, update, workflows).

How to advance: appointment by the steward with two signatures (the steward and one existing
security maintainer). Security-maintainer rights are the highest trust level short of the
steward.

### Steward

The final authority. Owns the trademark, the release signing key, and the copyright.

Responsibilities:

- Set project direction and resolve deadlocks.
- Sign releases and manage the release signing key per `docs/KEYS.md`.
- Approve maintainer promotions and licence changes.

## Current maintainers

| Name | GitHub | Role |
|------|--------|------|
| vedranius | [@vedranius](https://github.com/vedranius) | Steward |

## Triage rotation

Issues and security reports are triaged on a weekly rotation. Each week one maintainer is on
call and answers new issues and reports within **5 business days** (see `docs/PSIRT.md` for the
security response targets). Add maintainers here as they join; until then the steward is on
call every week.
