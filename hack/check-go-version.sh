#!/usr/bin/env bash
#
# Verify the Go version is consistent across the repo's build artifacts.
#
# cli/go.mod is the source of truth. When it is bumped (by a developer or by
# Renovate) the CI matrix and the Dockerfile must be bumped with it, or the
# build breaks on merge. This script catches that drift.
#
# Checks:
#   1. cli/Dockerfile downloads the exact go.mod version (go<major.minor.patch>)
#   2. .github/workflows/test.yml pins the go.mod major.minor for the CLI job
#   3. .github/workflows/test.yml pins the go.mod major.minor for the lint job
#
# Usage: hack/check-go-version.sh   (run from anywhere in the repo)
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
gomod="$repo_root/cli/go.mod"
dockerfile="$repo_root/cli/Dockerfile"
workflow="$repo_root/.github/workflows/test.yml"

# Full version from the go directive, e.g. "1.24.0"
full="$(sed -n 's/^go \([0-9][0-9.]*\)$/\1/p' "$gomod")"
if [ -z "$full" ]; then
  echo "ERROR: could not read the go directive from $gomod" >&2
  exit 1
fi
# Major.minor, e.g. "1.24"
mm="$(printf '%s' "$full" | cut -d. -f1-2)"

echo "cli/go.mod declares Go $full (major.minor $mm)"

fail=0
report() { echo "MISMATCH: $1" >&2; fail=1; }

# 1. Dockerfile downloads the exact patch version.
if ! grep -q "go${full}\." "$dockerfile"; then
  report "cli/Dockerfile does not download go${full} (expected 'go${full}.<os>-<arch>.tar.gz')"
fi

# 2. CLI test job matrix pins the major.minor.
if ! grep -q "go-version: \['${mm}'\]" "$workflow"; then
  report ".github/workflows/test.yml CLI matrix is not go-version: ['${mm}']"
fi

# 3. Lint job pins the major.minor.
if ! grep -q "go-version: '${mm}'" "$workflow"; then
  report ".github/workflows/test.yml lint job is not go-version: '${mm}'"
fi

if [ "$fail" -ne 0 ]; then
  echo "" >&2
  echo "Go version drift detected. Update the files above to match cli/go.mod (${full})." >&2
  exit 1
fi

echo "OK: Go version is consistent across go.mod, Dockerfile and CI workflow."
