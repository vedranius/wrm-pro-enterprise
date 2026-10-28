# Operator / go-live checklist

Everything that must be done by a human before WRM Pro Enterprise can be released and run in
production. Each item says **what**, **where**, and **how to verify**.

---

## 1. Release signing key (`RELEASE_SIGNING_KEY`)

The public key is already built in: `remote-manager/releasesign.go` →
`builtinReleaseKeys = ["wrm-ed25519:dDQxQPC0rcL16tuoDJ35/XC9b0p73Vdy6XB4mcL3+/w="]` (2026-10-17).
CI signs `SHA256SUMS.txt` with the **private** key from the repository secret. The `finalize`
job **fails** the release if the secret is missing (because a built-in key exists but cannot
sign).

### Option A — you still have the matching private key

If you kept the file from when the key was created (a file whose content starts with
`wrm-ed25519-private:`), just add it as a secret. No code change.

```bash
gh secret set RELEASE_SIGNING_KEY \
  --repo vedranius/wrm-pro-enterprise \
  < /path/to/release.key
```

### Option B — generate a new key (if the old private key is lost)

GitHub secrets are write-only: you cannot read the old repo's secret back. If you did not keep
`release.key`, generate a new pair.

```bash
cd remote-manager
go build -o wrm .          # or download a wrm binary from the old repo's releases
./wrm -release-keygen release.key
# prints: "Private key written to release.key ..." and a Public key line: wrm-ed25519:XXXX
gh secret set RELEASE_SIGNING_KEY \
  --repo vedranius/wrm-pro-enterprise \
  < release.key
```

Then add the **printed public key** at the **top** of `builtinReleaseKeys` in
`remote-manager/releasesign.go`, with a comment, e.g.:

```go
var builtinReleaseKeys = []string{
    "wrm-ed25519:NEW_PUBLIC_KEY_HERE", // project release key, 2026-10-28
    "wrm-ed25519:dDQxQPC0rcL16tuoDJ35/XC9b0p73Vdy6XB4mcL3+/w=", // old key, 2026-10-17
}
```

Commit and push that change. Then **keep `release.key` offline** (hardware/air-gapped) or
delete it. Treat it as the project's root of trust.

### Where the secret lives

GitHub → **Settings → Secrets and variables → Actions → New repository secret**:
name `RELEASE_SIGNING_KEY`, value = the exact content of `release.key`.

**Verify:** push a tag (e.g. re-push `v14.0.0`); the workflow `finalize` runs `wrm -release-sign`
and `wrm -release-verify`, and the release gets `SHA256SUMS.txt.sig`.

---

## 2. CI and repository settings

Do these once in the new repo:

- **Settings → Actions → General → Fork pull request workflows:** *Require approval for all
  external contributors* (so a fork PR can never reach the signing job).
- **Settings → Code security and analysis:** enable **Dependency graph** (needed by the
  dependency-review job), **Dependabot alerts** and **Private vulnerability reporting** (the
  "Report a vulnerability" button).
- **Secret `ANTHROPIC_API_KEY`** (optional): enables the `Claude security review` workflow;
  set a monthly spend limit on the key. Without it the job is skipped.
- **GHCR:** nothing to configure — the Docker job pushes to `ghcr.io/vedranius/wrm-pro` with the
  built-in `GITHUB_TOKEN` (`packages: write`). If the package is private and someone needs pull
  access, set it in the package settings.
- **Branch protection on `main`:** already set (1 review + code-owner review + required status
  checks + conversation resolution). Add the exact rendered check names in *Settings → Branches
  → main → Require status checks* once the first CI run has listed them (the API-set names do
  not include the matrix jobs `Build …` / `AppImage …`).
- **2nd maintainer:** invite another person, add them to `.github/CODEOWNERS` and
  `MAINTAINERS.md`, then raise the required reviews from 1 to 2.

---

## 3. External infrastructure

These features are implemented behind RFCs and need a live service to finish and test.

### KMS / HSM (RFC 0004)

1. Provision a key service: AWS KMS, Azure Key Vault, HashiCorp Vault (transit), or a PKCS#11
   HSM.
2. Create a key; note its id/ARN. Put the access credential in a **server-side secret** (env or
   a config file readable only by the WRM account) — never in the browser.
3. Implement the `KeyProvider` interface (RFC 0004) and wire `encryptValue`/`decryptValue` to
   wrap the data key with the KMS key.
4. Run the **rekey** on a copy, then **recovery** from a backup; document the result.
5. Add the provider test to CI only if it can run against an ephemeral/mocked KMS.

### HA / PostgreSQL (RFC 0002)

1. Provision PostgreSQL (primary + standby; or a managed HA Postgres).
2. Implement the Postgres backend behind the same `database/sql` interface; replace the
   idempotent SQLite DDL with a versioned migration list.
3. Run the migration on a **copy** first; verify a rollback (restore backup + previous binary).
4. Failover test: stop the primary, confirm the standby takes over and WRM reconnects.

### Windows password rotation (RFC 0003)

1. Provision a Windows host with a local account to rotate.
2. Build the agent (Go) and deploy it; register it with WRM over an authenticated, scoped token.
3. Rotate on a schedule and after use; confirm the new password lands encrypted in the vault and
   the matching connection is updated.
4. Audit `credential.rotated` events and a failure alert.

---

## 4. Real end-to-end tests

### WebAuthn / passkeys (L-50)

1. Run WRM over **HTTPS** or on **`localhost`** (passkeys require a secure context).
2. Sign in, open **Settings → Account**, press **Add passkey**, complete the browser prompt
   (security key or platform passkey). Repeat to register a **second** authenticator.
3. Sign out and sign back in: choose **Use a passkey / security key** on the 2FA step.
4. Trigger a dangerous action (e.g. change a policy) and confirm **step-up** with the passkey.
5. If you use a hostname other than the request `Host`, set **Admin → Settings →
   `webauthn_rpid`** to that hostname.

### SAML 2.0 (L-51)

1. In the IdP (Okta, Entra ID, Google Workspace, or a free self-hosted **Keycloak**), create a
   SAML app.
2. Set the IdP metadata URL/XML in **Admin → Settings → `saml_idp_metadata`**, enable
   `saml_enabled`, and set `saml_entity_id` if needed.
3. Give the IdP the SP metadata from `https://<your-wrm>/saml/metadata` (entity id, ACS URL).
4. Map the username attribute (`saml_attr_username`, default `uid`).
5. Sign in through the IdP; confirm the account is created/linked and a session starts.
6. Repeat with a **second** IdP (the L-51 acceptance criterion).

---

## 5. Institutional (Faza 5 and 7)

### Budget (L-45)

Use the table in `FUNDING.md`, adapt the FTE and EUR figures, and get it approved by whoever
owns the money. Gate G5: one source covers ≥ 50 % of maintenance.

### Sponsorship (L-47)

- GitHub Sponsors: `https://github.com/sponsors` → set up for the account/org; add the tier
  description (money funds maintenance and security work, never priority).
- Optionally Open Collective for transparent spending.

### Foundation applications (L-48)

- **OpenSSF Alpha-Omega** — application via the OpenSSF site; focus on security work.
- **Sovereign Tech Fund** (Germany) — open calls; security-critical open source.
- **NLnet / NGI** — periodic open calls.
Keep one application in flight; the first one is the gate.

### Independent security board (L-60)

Recruit **two external members** who review critical fixes. Record them in `MAINTAINERS.md`
(or a new `SECURITY-BOARD.md`) and give them access to the private advisory process.

### Annual audit (L-61)

Engage an external firm for a yearly review; publish the report and postmortems.

### Legal review (Faza 0, L-08)

Have a lawyer confirm: the CLA, the trademark policy (`TRADEMARK.md`), and the export
classification. Update the documents with anything they require.

---

## 6. Final release of v14.0.0

1. Set `RELEASE_SIGNING_KEY` (section 1).
2. Confirm `main` is green.
3. Push the tag `v14.0.0` (or re-push after the fix): the `build`/`appimage`/`docker` jobs build
   and attach assets, `finalize` signs the checksums and publishes the release.
4. Verify on the release page: binaries, `SHA256SUMS.txt`, `SHA256SUMS.txt.sig`, SBOM
   (`sbom-*.cdx.json`), provenance, and the Claude plugin zip.
5. Announce (README link, Discussions, social), and link `LICENSE-CHANGE.md` in the release
   notes.
