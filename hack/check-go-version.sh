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
#   4. .github/workflows/e2e.yml pins the go.mod major.minor
#
# Not checked, by design: instrumentation/go/go.mod. The instrumentation library
# is built by consumers against their own toolchain, so it pins the oldest Go it
# supports (1.21) and is tested against a matrix rather than tracking the CLI.
#
# Usage: hack/check-go-version.sh   (run from anywhere in the repo)
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
gomod="$repo_root/cli/go.mod"
dockerfile="$repo_root/cli/Dockerfile"
test_workflow="$repo_root/.github/workflows/test.yml"
e2e_workflow="$repo_root/.github/workflows/e2e.yml"

# Full version from the go directive, e.g. "1.24.0"
full="$(sed -n 's/^go \([0-9][0-9.]*\)$/\1/p' "$gomod")"
if [ -z "$full" ]; then
  echo "ERROR: could not read the go directive from $gomod" >&2
  exit 1
fi
# The go directive may omit the patch level ("go 1.24"). The Dockerfile always
# downloads a full x.y.z tarball, so normalise to three parts before matching,
# otherwise "go1.24." would match go1.24.7 and pass on a stale Dockerfile.
case "$full" in
  *.*.*) ;;
  *) full="$full.0" ;;
esac
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
if ! grep -q "go-version: \['${mm}'\]" "$test_workflow"; then
  report ".github/workflows/test.yml CLI matrix is not go-version: ['${mm}']"
fi

# 3. Lint job pins the major.minor.
if ! grep -q "go-version: '${mm}'" "$test_workflow"; then
  report ".github/workflows/test.yml lint job is not go-version: '${mm}'"
fi

# 4. E2E workflow pins the major.minor. It builds the CLI from source, so it
#    drifts the same way the test workflow does.
if ! grep -q "go-version: '${mm}'" "$e2e_workflow"; then
  report ".github/workflows/e2e.yml is not go-version: '${mm}'"
fi

if [ "$fail" -ne 0 ]; then
  echo "" >&2
  echo "Go version drift detected. Update the files above to match cli/go.mod (${full})." >&2
  exit 1
fi

echo "OK: Go version is consistent across go.mod, Dockerfile and CI workflows."
