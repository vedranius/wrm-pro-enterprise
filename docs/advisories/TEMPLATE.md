# Security Advisory Template

Copy this file to `docs/advisories/YYYY-NNNN.md` when publishing an advisory, where `YYYY` is the year and `NNNN` is the CVE or a sequence number. Replace each field.

---

# Security Advisory: [Title]

## Metadata

- **CVE ID:** `CVE-____-_____`
- **Severity:** Critical / High / Medium / Low
- **CVSS 4.0:** `X.X` (score) — vector `CVSS:4.0/AV:.../...` (use CVSS 4.0; record the vector string)
- **Attack conditions / prerequisites:** (no account / guest link / signed-in user / share member / administrator; any specific configuration required)
- **Affected versions:** `< vX.Y.Z` (list the affected range)
- **Fixed version:** `vX.Y.Z`
- **Component:** (web UI / HTTP API / MCP layer / transport / vault / audit / database)
- **Reporter:** (name or handle, with thanks; omit if anonymous)
- **Publication date:** (release date of the fix)

## Summary

One paragraph describing the vulnerability and its impact.

## Impact

What an attacker can achieve, the conditions required, and the affected components.

## Affected versions

List the exact affected releases and note that later releases are not affected.

## Workaround

Any mitigation users can apply before upgrading, or a statement that there is no workaround.

## Fix

Describe the fix and the release that contains it.

## Credits

Thanks to the reporter and any other parties involved.

## Timeline

- **Reported:** date
- **Acknowledged:** date
- **Triaged:** date
- **Fix merged:** date
- **Advisory published:** date
