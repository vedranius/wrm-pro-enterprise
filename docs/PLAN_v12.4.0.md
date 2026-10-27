# v12.4.0 implementation plan

Accepted and implemented on 2026-10-14. Target release: **v12.4.0**.

| Request | Implementation | Acceptance criteria |
|---|---|---|
| Server and service functionality | A Functionality section using the existing Git service catalog (including GitLab sources) and server installations. Read-only runtime checks during manual and periodic Git checks. | Installation problems, missing files, failed/inactive systemd services and every Supervisor worker are visible per server. Unknown or inaccessible checks are never reported as healthy. Bounded error logs accompany failures. Transitions generate notifications. |
| Configuration visibility | On-demand, bounded inspection of configuration/INI/environment files inside an installation; structured section/key/value rows and descriptions from comments. | Values and enabled/disabled flags are readable. Secrets are masked before the API response. Missing, unreadable, unsupported or truncated files are explicit. Symlinks cannot escape the installation. Configuration contents are not cached in the database. |
| [Issue #57](https://github.com/vedranius/web-browser-RDM-public/issues/57) | Add an optional folder parent with owner/cycle/depth validation; recursive sidebar; full folder paths in pickers, titles and tabs; folder grouping row for open windows. Preserve hierarchy in config export/import. | Client-A/Site-1/Backup and Client-B/Site-1/Backup remain distinguishable. Reparenting is possible. Deleting a folder retains its child folders at its former parent. Route defaults remain scoped to the connection's immediate folder. |
| [Issue #55](https://github.com/vedranius/web-browser-RDM-public/issues/55) | Safe inline MIME allowlist, remote SFTP/FTP byte-range streaming, bounded text preview, embedded media/image/PDF viewer; terminal selection context actions and file-manager preview. | Large video/audio files seek without downloading the whole file. File access permissions and share roles apply to every request. HTML/SVG cannot execute in WRM's origin. Selected relative paths resolve against terminal CWD. Download remains available. |
| [Discussion #56](https://github.com/vedranius/web-browser-RDM-public/discussions/56) | Refresh effective AI policy for open windows, support SSH-backed SFTP connections, provide an AI setup entry point and availability diagnostics after provider testing. | A tested provider does not misleadingly imply the assistant is enabled. Users can see the precise policy/setup blocker. Enabling the assistant reveals its terminal button without signing out. OpenAI-compatible providers work independently of MCP settings. |

## Validation and delivery

- Add focused regression tests for runtime parsing, configuration redaction/path safety, folder hierarchy, byte ranges and permission enforcement, and AI UI availability.
- Run formatting, vet, the full Go race suite and UI checks; verify real browser layouts in Croatian at 390 px and desktop widths.
- Build supported OS binaries through CI and retain the complete release asset/checksum verification.
- Reference #55, #57 and discussion #56 in relevant commits, changelog, release notes and PR. Assign issues to the v12.4.0 milestone and acknowledge planned work publicly. Close issues through the merged PR; add final release links to the issues and discussion.
