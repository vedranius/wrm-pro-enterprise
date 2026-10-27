# Licence change: Apache License 2.0 from v14.0.0

## What changed

From **v14.0.0** (the first release under the new terms), WRM is licensed under the
**Apache License, Version 2.0**. The full text is in [LICENSE](LICENSE). The project
was previously source-available under the **PolyForm Noncommercial License 1.0.0**
or the **PolyForm Internal Use License 1.0.0**, whichever applied to you.

## Which versions

| Versions | Licence |
|---|---|
| **v14.0.0 and later** | Apache License 2.0 |
| v13.x and earlier | PolyForm Noncommercial 1.0.0 / PolyForm Internal Use 1.0.0, as published |

Tags and release assets of earlier versions are unchanged. Copies you obtained under
the PolyForm terms keep the rights those terms granted, including the right to keep
using that copy. The copyright holder additionally makes the earlier versions
available under Apache-2.0 for anyone who prefers those terms.

## Why

1. **Contributions.** PolyForm Internal Use does not allow redistribution, so a
   contributor could not share a fix with anyone else. That blocks the way open
   projects actually grow.
2. **Adoption.** Apache-2.0 is OSI-approved and is on the default allow-list of most
   corporate legal and procurement processes; source-available licences are not.
3. **Predictability.** The rules are the same for every release. There is no
   per-version relicensing, no conversion date and no "open core" surprise.
4. **Patent grant.** Apache-2.0 includes an express patent licence from contributors,
   which matters for a security product used by companies.
5. **Security work.** A project that expects other organisations to run it in
   production must let them read, patch and redistribute it under clear terms.

## What Apache-2.0 does not give you

- **No trademark rights.** The name, logo and project identity are governed by
  [TRADEMARK.md](TRADEMARK.md). Forks must use a different name.
- **No warranty.** The software is provided "AS IS", as the licence states.
- **No guarantee of support.** Support, SLAs and commercial terms are separate.

## Contributions

Contributions require a signed CLA (see [CONTRIBUTING.md](CONTRIBUTING.md)) and a
`Signed-off-by` line. The CLA lets the project keep one coherent licence for
everyone; it does not transfer your copyright.

## Third-party components

WRM uses third-party software under its own licences. Each release ships a CycloneDX
SBOM listing them. Components that cannot be distributed under Apache-2.0 terms
(for example GPL-3.0 tools) are installed at runtime by the operator and are not
part of the distributed artifact; see [docs/THIRD-PARTY.md](docs/THIRD-PARTY.md).
