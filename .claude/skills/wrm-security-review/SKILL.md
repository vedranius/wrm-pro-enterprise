---
name: wrm-security-review
description: Security review of a WRM PRO change before pushing or before a release; WRM's threat model, the checklist, and the local scanners to run.
---

# WRM PRO security review

Run this before every push of a version branch (CLAUDE.md, "Before every push") and for the
weekly or per-release deep review.

1. Run the built-in `/security-review` on the branch's changes against `main`.
2. Apply the WRM checklist in `.github/security/claude-review-instructions.txt` to the diff,
   and the exclusions in `.github/security/claude-review-false-positives.txt`.
3. Run the scanners the CI runs (from `remote-manager/`):
   ```
   go install golang.org/x/vuln/cmd/govulncheck@v1.9.0 && govulncheck ./...
   go install honnef.co/go/tools/cmd/staticcheck@v0.8.2 && staticcheck ./...
   # gosec needs Go 1.26 until it supports Go 1.27:
   GOTOOLCHAIN=go1.26.9 go install github.com/securego/gosec/v2/cmd/gosec@v2.29.0
   GOTOOLCHAIN=go1.26.9 gosec -quiet -fmt json -out gosec.json -exclude=G104 ./... ; node ../tools/gosec-baseline.js gosec.json
   ```
   From the repository root: `gitleaks git --config .gitleaks.toml --log-opts="origin/main..HEAD" .`
   and `zizmor --offline .github/workflows`.
4. Fix real findings in the same branch. A new gosec finding that is reviewed and not
   exploitable is added with `--update`, with the reason in the pull request description.
   gosec's taint rules are not deterministic, so run gosec about 20 times (into different
   files) and pass every report: `node tools/gosec-baseline.js g1.json … g20.json --update`.
5. Write the result in the pull request description under "How was it tested?": what was
   reviewed, which scanners ran, and what was fixed or accepted and why.

Never weaken a check to make it pass (no new gosec exclusions, gitleaks allowlist entries for
real secrets, or skipped workflow jobs) without the owner's explicit decision.
