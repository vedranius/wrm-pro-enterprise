#!/usr/bin/env bash
# SPDX header gate. Fails when a project-owned source file is missing the Apache-2.0
# SPDX identifier. Vendored code (remote-manager/static/vendor/) and test fixtures
# (remote-manager/testdata/) keep their own headers and are not checked.
set -euo pipefail

missing=0
while IFS= read -r -d '' f; do
  if ! grep -q "SPDX-License-Identifier: Apache-2.0" "$f"; then
    echo "MISSING SPDX header: $f"
    missing=1
  fi
done < <(
  find remote-manager tools -type f \( \
      -name '*.go' -o -name '*.js' -o -name '*.css' -o -name '*.html' \) \
    -not -path 'remote-manager/testdata/*' \
    -not -path 'remote-manager/static/vendor/*' \
    -not -path 'remote-manager/static/brand/*' \
    -print0
)

if [ "$missing" -ne 0 ]; then
  echo "ERROR: some source files are missing the Apache-2.0 SPDX identifier." >&2
  echo "Add the header shown in CONTRIBUTING.md." >&2
  exit 1
fi

echo "SPDX headers OK"
