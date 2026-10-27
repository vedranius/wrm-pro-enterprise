# RFC Process

## What an RFC is

An RFC (Request for Comments) is a design document that proposes a significant change and invites community review before implementation. It captures the motivation, design, trade-offs, and alternatives.

## When to write one

Write an RFC for changes that are significant, cross-cutting, or hard to reverse, including:

- New subsystems or major features.
- Breaking API or protocol changes.
- Changes to the threat model or security posture.
- Architectural decisions with broad impact.
- Changes to the database schema that go beyond additive migrations.

A bug fix or small, self-contained change does not need an RFC.

## Process

1. Discuss the idea in GitHub Discussions or an issue to confirm it warrants an RFC.
2. Write the RFC using the template and open a pull request adding it to `docs/rfcs/`.
3. The community reviews and comments. The author updates the RFC in response.
4. Decisions follow `GOVERNANCE.md`: maintainer consensus for major changes, steward sign-off for licence-related changes.
5. On acceptance, the RFC is merged. Rejected RFCs are not merged; accepted RFCs are archived.

## Template

Use the template at the top of the `docs/rfcs/` directory. The template covers: summary, motivation, design, trade-offs, alternatives, and impact on security, migrations, and compatibility.

## Archive convention

Accepted RFCs are stored as numbered files in `docs/rfcs/` using the convention `NNNN-title.md`, where `NNNN` is a zero-padded sequence number and `title` is a short slug. The RFC stays in the archive as the historical record of the decision, even if later implementation details change.
