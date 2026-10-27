# Threat Model

STRIDE analysis of the WRM platform by component. WRM is a self-hosted, browser-based remote operations platform: SSH/SFTP terminals, RDP/VNC desktops, file transfer, a credentials vault, audit with session recording, and an MCP/AI layer. One binary, one database, no agent on managed servers.

| Component | Threat | Mitigation |
|-----------|--------|------------|
| Web UI | XSS via user input or rendered session data | Strict output encoding, CSP, no inline script, sanitize imported data |
| Web UI | CSRF on state-changing actions | CSRF tokens, same-site cookies |
| Web UI | Session hijacking | Secure, httpOnly, SameSite cookies; short-lived sessions; logout everywhere |
| HTTP API | Unauthenticated/unauthorized access | Enforced authentication and per-role authorization on every endpoint |
| HTTP API | Brute-force credential guessing | Rate limiting, lockout, MFA support |
| HTTP API | Information disclosure via errors | Generic error responses; detailed logging server-side only |
| MCP/AI layer | Prompt injection instructing the AI to act maliciously | Approvals enforced server-side; scoped tools; no tool bypasses policy |
| MCP/AI layer | AI exfiltrating secrets to third parties | Credentials never sent to model; least-privilege tool scopes |
| MCP/AI layer | Over-privileged tool access | Tool allowlists and per-connection scopes, re-verified per call |
| SSH/SFTP transport | MITM on connection to managed hosts | Host key verification, known-hosts tracking, key-change warnings |
| SSH/SFTP transport | Credential exposure on the wire | SSH/TLS for all transport; no plaintext fallback |
| RDP/VNC transport | Session hijacking or downgrade | TLS-wrapped connections; reject weak ciphers |
| Vault / secrets | At-rest disclosure | Encrypted at rest with a user-held key; key never stored alongside data |
| Vault / secrets | Unauthorized decryption | Per-user access controls; encryption key required to unlock |
| Vault / secrets | Secrets leaked into logs/recording | Redaction of credentials from logs and session recordings |
| Audit / recording | Tampering with audit trail | Append-only, integrity-protected records |
| Audit / recording | Unauthorized playback | Role-restricted access to recordings |
| Audit / recording | Sensitive data in recordings | Redaction and masking of secrets in captures |
| Database | SQL injection | Parameterized queries only; no string-built SQL |
| Database | Direct filesystem access to DB | Restrictive file permissions; run under least-privilege account |
| Database | Backup exposure | Encrypt backups; store keys separately |

This model is a summary. Findings from security reviews update the table and may spawn RFCs.
