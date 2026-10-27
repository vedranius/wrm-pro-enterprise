# WRM PRO v12.4.3 implementation plan

The owner explicitly prioritized these four requests over the earlier accepted backlog. Implementation: [8f9e07a](https://github.com/vedranius/web-browser-RDM-public/commit/8f9e07a). Release: [v12.4.3](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.4.3).

## Accepted scope

1. Add Medium alongside Simple/Advanced using the supplied original deploy-tool (3) exactly, compatible with the supplied deploy-config archive.
2. Upload reference config/INI/Python files and inspect settings across catalog servers/installations/operators in Functionality.
3. Sign in to Bitwarden and use native SSH keys by reference in SSH/SFTP connections.
4. Reorder folders freely, including moving up/down and drag/drop in the existing hierarchy.

## Implementation and acceptance

| Request | Implementation | Acceptance evidence |
|---|---|---|
| Original Medium | Import approved private source ZIP; unchanged original ASGI app behind authenticated URL bridge; private operator config, SSH profile and data; encrypted original runtime preferences | Original 1,581-test suite; actual archive/config imports; native comparison, plan, two deployments, file-content checks, history and real rollback in Chromium; [parity record](MEDIUM_PARITY_v12.4.3.md) |
| Config profiles | Mask/encrypt parsed reference; owner-scoped bounded remote checks; searchable reference/actual matrix | Parser/path/symlink/ownership regressions; real browser profile upload and operator/hostname/value matrix |
| Bitwarden | Official CLI profiles and memory-only unlock; native item ID in connections; server-side key retrieval; sealed CLI cache | Mock account/MFA/unlock/sync/lock/isolation tests; actual SSH public-key handshake; official CLI installation/status smoke |
| Folder ordering | Additive sort column; transactional sibling reorder/reparent; menu/drag controls; exports | Ownership/cycle/nesting/order/export tests; browser persistence after reload |

The normal full Go race/vet/format, Python wrapper, UI translations, Node regressions, cross-build and real-browser checks are required before push. Release publication requires green exact-head CI, merge, annotated version tag, complete packages/checksums and Docker publication.

## Boundaries

The original tool remains private; no supplied source/config or real infrastructure identifiers enter public Git. Its native engine retains original plans, locks, backups and history. Medium executes editable native commands as the WRM OS user and is intended for trusted operators; per-user directories are not an OS sandbox. The runtime needs direct reachability to its Git/SSH destinations. Config profiles inspect WRM catalog installations, not an automatically imported Medium catalog. Bitwarden cloud account login is not claimed as live-tested without real credentials. Earlier roadmap requests remain pending.
