# Threat Model

STRIDE analysis of the WRM platform. WRM is a self-hosted, browser-based remote operations
platform: SSH/SFTP terminals, RDP/VNC desktops, file transfer, a credentials vault, audit with
session recording, and an MCP/AI layer. One binary, one database, no agent on managed servers.

This model is reviewed quarterly and on every major feature. Findings from security reviews and
penetration tests update it.

## Actors

| Actor | Description |
|---|---|
| Anonymous remote attacker | No account; reaches the HTTP(S) API or MCP endpoint over the network |
| Signed-in user | Holds a WRM account and shares/roles; may also be a share member or auditor |
| Administrator | Holds administrative rights; the most powerful legitimate actor |
| Share member / viewer | Has limited rights on a subset of connections or a shared folder |
| AI model / MCP client | Untrusted, possibly manipulated; acts through WRM's tools and scopes |
| Operator / host | The person running WRM and the OS account it runs under |
| Reporter | Submits security reports through the PSIRT channel |

## Trust boundaries

1. **Browser ↔ WRM** — the web UI and the HTTP API. The browser and its scripts are untrusted;
   the server is the decision point for every authorization and approval.
2. **WRM ↔ managed servers** — SSH/SFTP/RDP/VNC to the servers WRM manages. WRM acts with the
   logins of the connections a user owns; it must not act for anyone else.
3. **WRM ↔ AI providers / MCP clients** — outbound to model providers and inbound MCP clients
   (`/mcp`). Both directions treat the other side as untrusted and possibly manipulated.
4. **WRM ↔ OpenCode / AI sandbox** — the contained coding agent. The container is the boundary,
   not the approval alone; it holds no SSH keys or connection data.
5. **Host filesystem** — the database, encryption key, recordings, and the runtime directories
   (Medium, Bitwarden cache, OpenCode install). Only the WRM OS account may read these.

## STRIDE by component

| Component | Threat | Mitigation |
|---|---|---|
| Web UI | XSS via user input or rendered session data | Strict output encoding, CSP, no inline script, sanitize imported data |
| Web UI | CSRF on state-changing actions | CSRF tokens, same-site cookies |
| Web UI | Session hijacking | Secure, httpOnly, SameSite cookies; short-lived sessions; logout everywhere |
| HTTP API | Unauthenticated/unauthorized access | Enforced authentication and per-role authorization on every endpoint |
| HTTP API | Brute-force credential guessing | Rate limiting, lockout, MFA support |
| HTTP API | Information disclosure via errors | Generic error responses; detailed logging server-side only |
| Authentication | 2FA/OIDC/LDAP bypass, account takeover | Step-up on sensitive actions, verified PKCE/callback binding, lockout |
| Vault / secrets | At-rest disclosure | Encrypted at rest with a user-held key; key never stored alongside data |
| Vault / secrets | Unauthorized decryption / reveal | Per-user access controls; encryption key required to unlock |
| Vault / secrets | Secrets leaked into logs/recording | Redaction of credentials from logs and session recordings |
| SSH/SFTP transport | MITM on connection to managed hosts | Host key verification, known-hosts tracking, key-change warnings |
| SSH/SFTP transport | Credential exposure on the wire | SSH/TLS for all transport; no plaintext fallback |
| RDP/VNC/guacd | Session hijacking or downgrade | TLS-wrapped connections; reject weak ciphers; guacd on loopback only |
| Tunnels / jump hosts | Using another user's tunnel/jump host, or bypassing tunnel policy | Tunnels bound to owner and policy, re-checked per connection and request |
| MCP/AI layer | Prompt injection instructing the AI to act maliciously | Approvals enforced server-side; scoped tools; no tool bypasses policy |
| MCP/AI layer | AI exfiltrating secrets to third parties | Credentials never sent to model; least-privilege tool scopes |
| MCP/AI layer | Over-privileged tool access / token reuse | Tool allowlists and per-connection scopes, re-verified per call; short-lived tokens |
| Addons | SSRF through the `http` capability | Allowlist of origins/paths, private-address blocking, re-checked redirects |
| Addons | Malicious panel code escaping the sandbox | CSP sandbox, opaque frame origin, per-file SHA-256 integrity |
| Update / signing | Tampered or downgraded release | Signed releases (ed25519); SHA-256 verified before install; pinned repo |
| Audit / recording | Tampering with audit trail | Append-only, integrity-protected records |
| Audit / recording | Unauthorized playback | Role-restricted access to recordings |
| Audit / recording | Sensitive data in recordings | Redaction and masking of secrets in captures |
| Database | SQL injection | Parameterized queries only; no string-built SQL |
| Database | Direct filesystem access to DB | Restrictive file permissions; run under least-privilege account |
| Database | Backup exposure | Encrypt backups; store keys separately |

## Assumptions

- The operator runs WRM as its own unprivileged OS account and keeps the host patched.
- HTTPS is terminated either by WRM's self-signed certificate or a reverse proxy the operator trusts.
- The database, encryption key and recordings are not reachable by less-privileged accounts.
- Third-party services (AI providers, Git hosts, NetBox, SMTP/SIEM) are as trustworthy as the
  operator assumes; WRM only protects secrets it manages itself.

## Open risks

- A login that can already read a sensitive file through another means (a custom script, a
  shell) is outside what any read-only classifier can know.
- Redaction is pattern-based: a secret that does not look like one can still appear in a diff
  or recording.
- The OpenCode sandbox has outbound network access; a user-approved command can open connections.
- Plain-HTTP MCP on a LAN transports tokens unencrypted; HTTPS is recommended but not enforced.
