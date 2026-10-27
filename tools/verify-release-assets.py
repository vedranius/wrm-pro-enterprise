#!/usr/bin/env python3
"""Fail before publishing unless all supported packages match SHA256SUMS.txt."""
import hashlib
import pathlib
import sys

version, directory = sys.argv[1:]
root = pathlib.Path(directory)
platforms = [
    'linux-amd64', 'linux-arm64', 'linux-armv7', 'linux-armv6', 'linux-386',
    'windows-amd64.exe', 'windows-arm64.exe', 'darwin-amd64', 'darwin-arm64',
    'darwin-universal', 'android-arm64', 'freebsd-amd64', 'freebsd-arm64',
    'openbsd-amd64', 'linux-amd64.AppImage', 'linux-arm64.AppImage',
]
expected = {f'wrm-pro-{version}-{p}' for p in platforms}
actual = {p.name for p in root.glob('wrm-pro-*')}
if actual != expected:
    sys.exit(f'Incorrect asset set: missing={expected-actual}, unexpected={actual-expected}')
sums = {}
for line in (root / 'SHA256SUMS.txt').read_text().splitlines():
    digest, name = line.split(maxsplit=1)
    name = name.lstrip('*')
    if name in sums:
        sys.exit(f'Duplicate checksum: {name}')
    sums[name] = digest
if set(sums) != expected:
    sys.exit('Checksum manifest must cover exactly the 16 release packages.')
for name in sorted(expected):
    path = root / name
    if path.stat().st_size == 0 or hashlib.sha256(path.read_bytes()).hexdigest() != sums[name]:
        sys.exit(f'Invalid or empty asset: {name}')
print('Verified all 16 packages and SHA-256 checksums.')
