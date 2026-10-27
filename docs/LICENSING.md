# Licensing

## Licence

WRM Pro Enterprise is licensed under the **Apache License 2.0** from **v14.0.0**. See
[LICENSE](../LICENSE), [NOTICE](../NOTICE) and [LICENSE-CHANGE.md](../LICENSE-CHANGE.md).

Versions up to and including v13.x were published under the **PolyForm Noncommercial 1.0.0**
/ **PolyForm Internal Use 1.0.0** terms. Those tags and release assets are unchanged, and
live in the predecessor repository
[`web-browser-RDM-public`](https://github.com/vedranius/web-browser-RDM-public).

## Authors and contributors

The source was written by **vedranius**, with substantial AI assistance (commits by
`claude`). There are no third-party human contributors whose permission is needed for the
licence change. Copyright is held by `vedranius and WRM contributors`.

To list all commit authors in a checkout:

```bash
git log --format='%an <%ae>' | sort -u
```

## SPDX

Every project-owned source file carries a copyright line and an
`SPDX-License-Identifier: Apache-2.0` header. Generated files and third-party vendored code
keep their own headers. See [CONTRIBUTING.md](../CONTRIBUTING.md) for the format.

## Dependency licence inventory

- **Go modules:** `go-licenses report ./...` from `remote-manager/` (all permissive; no
  copyleft in the distributed binary). See [THIRD-PARTY.md](THIRD-PARTY.md).
- **Container image:** `syft dir:. -o cyclonedx-json` and
  `trivy image --scanners license` (each component must have a licence).
- **SBOM:** each release ships a CycloneDX SBOM (`sbom-vX.Y.Z.cdx.json`).

## Third-party components

See [THIRD-PARTY.md](THIRD-PARTY.md) for the runtime-installed components (Bitwarden CLI,
OpenCode, guacd) and the vendored front-end assets.
