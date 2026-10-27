# Third-party components

WRM Pro Enterprise is distributed under the Apache License 2.0. Third-party components keep
their own licences. This file summarises them; the authoritative, machine-readable list for
each release is the CycloneDX SBOM (`sbom-vX.Y.Z.cdx.json`) published with that release,
together with the files under `remote-manager/static/vendor/`.

## Components installed at runtime (not distributed)

These are installed by the operator, not bundled in the WRM image or binaries:

| Component | Licence | Notes |
|---|---|---|
| Bitwarden CLI (`@bitwarden/cli`) | GPL-3.0 | Optional vault integration. WRM finds it via `WRM_BITWARDEN_CLI` or `bw` on `PATH`; the feature is disabled when it is absent. It is deliberately **not** in the Docker image. |
| OpenCode | MIT | Optional AI coding agent, installed by an administrator at runtime. |
| guacd (Guacamole proxy daemon) | Apache-2.0 | Remote desktop (RDP/VNC/Telnet). A separate container (`guacamole/guacd`) in `docker-compose.yml`, not part of the WRM image. |

## Go modules

`remote-manager/go.mod` pins the Go dependencies. All direct dependencies are permissive
(MIT, BSD, ISC or Apache-2.0); there are no copyleft modules in the distributed binary. To
regenerate the report, run from `remote-manager/`:

```bash
go-licenses report ./... > licenses.csv
```

## Vendored front-end assets

`remote-manager/static/vendor/` ships third-party front-end code with its own notices; see
`remote-manager/static/vendor/THIRD-PARTY-LICENSES.txt`. The main components are xterm.js
(MIT) and Guacamole Common JS (Apache-2.0).

## Policy

- GPL- and AGPL-licensed code is **not** linked into or bundled with WRM; such tools are
  installed by the operator at runtime as separate processes.
- Each release must ship a CycloneDX SBOM (see `docs/RELEASE.md`).
- If you add or upgrade a dependency, update this file and regenerate the SBOM.
