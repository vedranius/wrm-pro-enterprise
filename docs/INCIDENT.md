# Incident Response Runbook

Runbook for responding to a security incident affecting a WRM deployment or the project's own infrastructure. Follow the phases in order; do not skip containment.

## Roles

- **Incident lead** — steward or a designated maintainer. Owns decisions and communication.
- **Responders** — maintainers and any operators of the affected deployment.
- **Scribe** — records a timeline of actions and evidence.

## 1. Detect

- [ ] Confirm a potential incident from an alert, report, or observation.
- [ ] Note the time, source, and scope (which host, which component).
- [ ] Start a shared timeline; log every subsequent action with timestamps.
- [ ] Declare severity (Critical/High/Medium/Low) using `docs/PSIRT.md`.

## 2. Triage

- [ ] Reproduce or verify the indicator if possible.
- [ ] Identify affected components (web UI, API, MCP layer, transport, vault, audit, database).
- [ ] Determine blast radius: data accessed, credentials touched, hosts affected.
- [ ] Preserve evidence: logs, recordings, database snapshots, network captures.
- [ ] Decide whether to escalate to external parties (users, legal) — consult the steward.

## 3. Contain

- [ ] Isolate affected hosts or revoke the compromised credentials.
- [ ] Rotate all credentials that may have been exposed, including the database encryption key if indicated.
- [ ] Block the attacker's access path (revoke keys, rotate secrets, close the vector).
- [ ] Take affected service offline if containment otherwise fails.
- [ ] Keep evidence intact while stopping further damage.

## 4. Eradicate

- [ ] Identify and remove the root cause (patch, misconfiguration, compromised account).
- [ ] Verify no persistence remains (backdoors, cron jobs, injected sessions).
- [ ] Rebuild or re-provision compromised systems from trusted state.
- [ ] Confirm the fix closes the vector on all affected instances.

## 5. Recover

- [ ] Restore service in a controlled manner.
- [ ] Restore data from clean, verified backups.
- [ ] Re-issue and distribute new credentials to legitimate users.
- [ ] Monitor for signs of re-entry during and after recovery.

## 6. Post-incident review

- [ ] Schedule a blameless review with all responders.
- [ ] Reconstruct the full timeline from the scribe's log.
- [ ] Identify what failed and what worked.
- [ ] Write action items with owners, tracked as GitHub issues.
- [ ] Publish an advisory if warranted (see `docs/PSIRT.md`) and update `docs/THREAT-MODEL.md`.
