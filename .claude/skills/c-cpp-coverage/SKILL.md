---
name: c-cpp-coverage
description: Generate code coverage for C/C++ projects using gcov and lcov, and upload to Codecov. Use this skill when users need to add coverage to C or C++ projects, set up lcov pipelines, or troubleshoot gcov/lcov issues. Covers autotools, CMake, and Meson build systems.
---

# C/C++ Coverage Generation Skill

This skill provides detailed guidance for generating code coverage from
C/C++ projects using gcov/lcov and uploading it to Codecov. It encodes
hard-won knowledge from real-world onboarding of C/C++ projects where
the standard "just add --coverage" advice is insufficient.

## When to Use This Skill

Use this skill when:
- The `codecov-onboarding` skill detects a C/C++ project
- A user asks about gcov, lcov, or C/C++ code coverage
- Coverage generation fails with gcov/lcov errors
- A user needs to add a coverage CI job to a C/C++ project
- A user encounters `_FORTIFY_SOURCE`, negative count, or format errors

## Why C/C++ Coverage Needs Special Handling

Unlike Go (`go test -coverprofile`) or Python (`pytest-cov`), C/C++
coverage requires a multi-step pipeline:

1. **Compile** with `--coverage` flags (produces `.gcno` files)
2. **Run tests** (produces `.gcda` counter files)
3. **Capture** with `lcov` (reads `.gcda` → produces `.info` report)
4. **Filter** system headers and test code from the report
5. **Upload** the `.info` file to Codecov

Each step has gotchas. This skill documents them all.

## Prerequisites

- GCC or Clang (for `--coverage` / gcov support)
- `lcov` (often not pre-installed in CI containers)
- A build system: autotools, CMake, or Meson
- An existing test suite (e.g., `make check`, `ctest`, `meson test`)

## Instructions

### Step 1: Detect the Build System

```bash
# Autotools (configure.ac / Makefile.am)
ls configure.ac Makefile.am 2>/dev/null

# CMake
ls CMakeLists.txt 2>/dev/null

# Meson
ls meson.build 2>/dev/null

# Plain Makefile
ls Makefile GNUmakefile 2>/dev/null
```

Also check how tests are run:
```bash
grep -E "^check|^test" Makefile 2>/dev/null
grep "add_test\|enable_testing" CMakeLists.txt 2>/dev/null
grep "test(" meson.build 2>/dev/null
```

### Step 2: Configure Coverage Compiler Flags

The core flags are the same regardless of build system:

```
CFLAGS="-g -O0 --coverage"
CXXFLAGS="-g -O0 --coverage"
LDFLAGS="--coverage"
```

- `-g` — debug symbols (needed for source mapping)
- `-O0` — no optimization (accurate line-level coverage)
- `--coverage` — equivalent to `-fprofile-arcs -ftest-coverage`

#### Workaround: _FORTIFY_SOURCE conflict

On Fedora, RHEL, and CentOS, the system's default CFLAGS include
`_FORTIFY_SOURCE=2`, which requires at least `-O1`. Since coverage
needs `-O0`, the compiler will error:

```
error: _FORTIFY_SOURCE requires compiling with optimization (-O1 or higher)
```

**Fix:** Explicitly undefine and redefine `_FORTIFY_SOURCE` to 0:

```
CFLAGS="-g -O0 --coverage -Wp,-U_FORTIFY_SOURCE,-D_FORTIFY_SOURCE=0"
CXXFLAGS="-g -O0 --coverage -Wp,-U_FORTIFY_SOURCE,-D_FORTIFY_SOURCE=0"
```

The `-Wp,-U,-D` syntax passes preprocessor directives through the
compiler driver.

#### Workaround: -Werror with -O0

Many projects use `--enable-gcc-warnings` or `-Werror` in their default
build. At `-O0` some warnings that are suppressed at higher optimization
levels become visible, causing the build to fail.

**Fix:** Pass `--disable-gcc-warnings` (autotools) or remove `-Werror`
from the coverage build configuration.

### Step 3: Apply Flags to the Build System

#### Autotools

```bash
./configure \
  CFLAGS="-g -O0 --coverage -Wp,-U_FORTIFY_SOURCE,-D_FORTIFY_SOURCE=0" \
  CXXFLAGS="-g -O0 --coverage -Wp,-U_FORTIFY_SOURCE,-D_FORTIFY_SOURCE=0" \
  LDFLAGS="--coverage" \
  --disable-gcc-warnings
make
make check
```

**Important:** If the project's `ci/build.sh` or configure wrapper
assembles `./configure` arguments from multiple sources, ensure that
per-job overrides (like `--disable-gcc-warnings`) come **last** on the
command line so they take precedence.

#### CMake

```bash
cmake -B build \
  -DCMAKE_C_FLAGS="-g -O0 --coverage -Wp,-U_FORTIFY_SOURCE,-D_FORTIFY_SOURCE=0" \
  -DCMAKE_CXX_FLAGS="-g -O0 --coverage -Wp,-U_FORTIFY_SOURCE,-D_FORTIFY_SOURCE=0" \
  -DCMAKE_EXE_LINKER_FLAGS="--coverage" \
  -DCMAKE_SHARED_LINKER_FLAGS="--coverage"
cmake --build build
cd build && ctest
```

Some CMake projects support a `CMAKE_BUILD_TYPE=Coverage` or have a
`-DENABLE_COVERAGE=ON` option. Check the project's `CMakeLists.txt`
for existing coverage support before adding flags manually.

#### Meson

```bash
meson setup builddir \
  -Db_coverage=true \
  -Dc_args="-Wp,-U_FORTIFY_SOURCE,-D_FORTIFY_SOURCE=0" \
  -Dcpp_args="-Wp,-U_FORTIFY_SOURCE,-D_FORTIFY_SOURCE=0" \
  -Dwerror=false
cd builddir
ninja
ninja test
```

Meson has built-in coverage support via `-Db_coverage=true` and can
generate reports with `ninja coverage`.

### Step 4: Generate the lcov Report

After tests run, `.gcda` files are scattered throughout the build tree.
Use `lcov` to collect them into a single `.info` report:

```bash
lcov --capture \
  --directory . \
  --output-file coverage.info \
  --ignore-errors format,gcov,unsupported,negative
```

Then filter out system headers and test code:

```bash
lcov --remove coverage.info \
  '/usr/*' \
  '*/tests/*' \
  '*/test/*' \
  --output-file coverage.info \
  --ignore-errors format,negative
```

Verify the report:

```bash
lcov --list coverage.info --ignore-errors negative | tail -5
```

#### Why --ignore-errors is needed

- **`negative`** — When tests run in parallel (`make -j N`), gcov's
  internal counters can race, producing `.gcda` files with negative
  execution counts. lcov >= 2.0 treats these as hard errors by default.
  This is a known gcov limitation with parallel test execution.

- **`format`** — Stale `.gcda` files from a previous build or a
  different compiler version may have incompatible format.

- **`gcov`** — Some source files may fail gcov processing (e.g.,
  generated code or assembly files).

- **`unsupported`** — Some gcov extensions used by newer GCC versions
  may not be recognized by the installed lcov version.

### Step 5: Separate Unit and Integration Test Coverage (Optional)

If you want to upload unit tests and integration tests as separate
Codecov flags, use `lcov --zerocounters` between test phases:

```bash
# Phase 1: Unit tests
lcov --zerocounters --directory .
make check TESTS="$UNIT_TESTS"
lcov --capture --directory . --output-file unit-tests.info \
  --ignore-errors format,gcov,unsupported,negative

# Phase 2: Integration tests
lcov --zerocounters --directory .
make check TESTS="$INTEGRATION_TESTS"
lcov --capture --directory . --output-file integration-tests.info \
  --ignore-errors format,gcov,unsupported,negative
```

For autotools projects, you can classify tests by file type:
- Compiled C binaries (`test-*` without `.sh`) → unit tests
- Shell scripts (`test-*.sh`) → integration tests

### Step 6: Upload to Codecov

Read the Codecov instance configuration from
`codecov-config/CONFIG.md` to determine the correct instance URL.

**In GitHub Actions**, prefer the `codecov/codecov-action@v5` with
`use_oidc: true` (see Step 9 template). OIDC eliminates the need for
a token secret.

**In GitLab CI, Tekton, or any CLI-based upload**, a token is always
required — the CLI does not support OIDC:

```bash
curl -Os https://cli.codecov.io/latest/linux/codecov
chmod +x codecov

# For app.codecov.io:
./codecov upload-process \
  --token "${CODECOV_TOKEN}" \
  --flag unit-tests \
  --file coverage.info

# For self-hosted instances, add --codecov-url:
./codecov upload-process \
  --codecov-url "${CODECOV_URL}" \
  --token "${CODECOV_TOKEN}" \
  --flag unit-tests \
  --file coverage.info
```

### Step 7: Handle Heavy Test Suites

Some C/C++ projects have test suites that become impractical under
coverage instrumentation. Coverage overhead amplifies I/O, CPU, and
timing in ways that can cause tests to hang or time out.

#### Common problem categories

| Category | Symptom | Example |
|---|---|---|
| VM-based tests | Extremely slow boot/I/O | Tests spawning QEMU/guestfish |
| Timing-sensitive | Assertions on wall-clock time fail | Timeout or rate-limit tests |
| Exponential backoff | Delays compound with overhead | Retry tests |
| FFI/plugin tests | Missing gcov symbols at load time | OCaml/Rust plugins linked without libgcov |
| Shell-heavy tests | Hang from repeated process spawning | Eval/shell plugin tests |

#### Solution: Skip problematic tests

Create a script that replaces problematic test files with `exit 77`
stubs (automake's SKIP status) before running `make check`:

```bash
#!/bin/bash
# ci/skip-coverage-tests.sh
set -euo pipefail

TESTS_TO_SKIP=(
    tests/test-heavy-io.sh
    tests/test-timing-sensitive.sh
    # ... add more as needed
)

for test_file in "${TESTS_TO_SKIP[@]}"; do
    if [ -f "$test_file" ]; then
        echo "  SKIP: $test_file"
        cat > "$test_file" <<'SKIP'
#!/bin/bash
echo "Skipped: too slow under coverage instrumentation"
exit 77
SKIP
        chmod +x "$test_file"
    fi
done
```

**Important:** Use `count=$((count + 1))` instead of `((count++))`
in bash scripts with `set -e`, because `((0++))` returns exit code 1
when the value is 0, causing the script to abort.

#### Solution: Timeout for make check

Add a timeout to prevent any single test from blocking the CI job:

```bash
# Use -s KILL (not --signal=KILL) for BusyBox compatibility (Alpine)
timeout -s KILL 30m make check || true
```

The `|| true` ensures the CI job continues to the coverage upload
step even if some tests fail — partial coverage is better than none.

### Step 8: GitLab CI Job Template

Here is a complete GitLab CI job for C/C++ coverage with all
workarounds applied. Adapt to the specific project:

```yaml
coverage:
  stage: builds
  timeout: 2h
  variables:
    CFLAGS: "-g -O0 --coverage -Wp,-U_FORTIFY_SOURCE,-D_FORTIFY_SOURCE=0"
    CXXFLAGS: "-g -O0 --coverage -Wp,-U_FORTIFY_SOURCE,-D_FORTIFY_SOURCE=0"
    LDFLAGS: "--coverage"
    CONFIGURE_OPTS: "--disable-gcc-warnings"
    BUILD_ONLY: "true"
  script:
    - ci/skip-coverage-tests.sh      # Skip problematic tests
    - ci/build.sh                     # Configure + compile (no tests)
    - ci/codecov.sh                   # Run tests + lcov + upload
  artifacts:
    paths:
      - "config.log"
      - "**/test-suite.log"
      - "unit-tests.info"
      - "integration-tests.info"
    when: always
    expire_in: 1 week
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event" && $CI_MERGE_REQUEST_TARGET_BRANCH_NAME == "master"'
    - if: '$CI_PIPELINE_SOURCE == "push" && $CI_COMMIT_BRANCH == "master"'
    - when: never
```

Key design decisions:
- **`BUILD_ONLY: "true"`** — The build script compiles without running
  tests. The codecov script handles test execution so it can separate
  unit and integration test phases with `lcov --zerocounters` between them.
- **`timeout: 2h`** — Coverage-instrumented builds and tests are
  significantly slower than normal.
- **`|| true` on test execution** (inside codecov.sh) — Ensures
  coverage data is always uploaded, even if some tests fail.
- **Artifacts include `.info` files** — For debugging and manual
  inspection of coverage data.

### Step 9: GitHub Actions Job Template

For C/C++ projects on GitHub:

```yaml
coverage:
  runs-on: ubuntu-latest
  timeout-minutes: 120
  permissions:
    id-token: write  # Required for OIDC authentication with Codecov
  steps:
    - uses: actions/checkout@v4

    - name: Install lcov
      run: sudo apt-get install -y lcov

    - name: Build with coverage
      run: |
        ./configure \
          CFLAGS="-g -O0 --coverage" \
          CXXFLAGS="-g -O0 --coverage" \
          LDFLAGS="--coverage" \
          --disable-gcc-warnings
        make -j$(nproc)

    - name: Run tests
      run: |
        timeout 1800 make check || true

    - name: Generate coverage report
      run: |
        lcov --capture --directory . --output-file coverage.info \
          --ignore-errors format,gcov,unsupported,negative
        lcov --remove coverage.info '/usr/*' '*/tests/*' \
          --output-file coverage.info \
          --ignore-errors format,negative

    - name: Upload to Codecov
      uses: codecov/codecov-action@v5
      with:
        use_oidc: true  # Preferred for public repos with app.codecov.io
        flags: unit-tests
        files: coverage.info
        fail_ci_if_error: false
        # For self-hosted Codecov, replace use_oidc with:
        # url: <CODECOV_INSTANCE_URL>
        # token: ${{ secrets.CODECOV_TOKEN }}
```

### Step 10: ci/codecov.sh Template Script

A reusable script for generating lcov reports and uploading to Codecov.
Includes all known workarounds:

```bash
#!/bin/bash
# ci/codecov.sh - Run tests, generate lcov reports, upload to Codecov.
#
# Expects the project to be already compiled with --coverage flags.
# Handles two test phases (unit + integration) with separate flags.

set -euo pipefail

MAKE="${MAKE:-make -j $(getconf _NPROCESSORS_ONLN)}"
LCOV_IGNORE="--ignore-errors format,gcov,unsupported,negative"
LCOV_IGNORE_FILTER="--ignore-errors format,negative"

# Install lcov if not present
if ! command -v lcov &>/dev/null; then
    echo "== Installing lcov =="
    if command -v dnf &>/dev/null; then
        dnf install -y lcov
    elif command -v apt-get &>/dev/null; then
        apt-get update && apt-get install -y lcov
    else
        echo "ERROR: Cannot install lcov — unknown package manager"
        exit 1
    fi
fi

capture_coverage() {
    local name="$1"
    lcov --capture --directory . --output-file "${name}.info" $LCOV_IGNORE
    lcov --remove "${name}.info" '/usr/*' '*/tests/*' \
         --output-file "${name}.info" $LCOV_IGNORE_FILTER
    echo "== ${name} coverage summary =="
    lcov --list "${name}.info" --ignore-errors negative | tail -3
}

# --- Run tests and capture coverage ---
# Adjust the test commands below to match your project.

lcov --zerocounters --directory .
echo "== Running tests =="
timeout -s KILL 30m $MAKE check || true
capture_coverage "coverage"

# --- Upload to Codecov ---
# Choose ONE of the following options based on your GitLab instance:

# Option A: Public GitLab (gitlab.com) → app.codecov.io (binary download)
curl -Os --connect-timeout 10 --max-time 120 \
  https://cli.codecov.io/latest/linux/codecov
chmod +x codecov
./codecov upload-process \
  --token "${CODECOV_TOKEN}" \
  --flag unit-tests \
  --file coverage.info

# Option B: Internal GitLab (gitlab.cee.redhat.com) → self-hosted Codecov (pip)
# pip install codecov-cli
# codecovcli --enterprise-url "${CODECOV_URL}" upload-process \
#   --token "${CODECOV_TOKEN}" \
#   --slug "${CI_PROJECT_PATH}" \
#   --flag unit-tests \
#   --file coverage.info \
#   --disable-search \
#   --git-service gitlab_enterprise
```

**Note for internal GitLab:** See `codecov-config/CONFIG.md` for the pip-based
codecov-cli approach which is recommended for internal GitLab environments.

## Troubleshooting

### "error: _FORTIFY_SOURCE requires compiling with optimization"

**Cause:** Fedora/RHEL default CFLAGS set `_FORTIFY_SOURCE=2` which
needs `-O1`, but coverage requires `-O0`.

**Fix:** Add `-Wp,-U_FORTIFY_SOURCE,-D_FORTIFY_SOURCE=0` to CFLAGS.

### "geninfo: ERROR: Unexpected negative count '-NNN'"

**Cause:** Parallel test execution (`make -j`) races gcov counters.

**Fix:** Add `--ignore-errors negative` to all lcov commands.

### "lcov: command not found"

**Cause:** lcov is not pre-installed in the CI container.

**Fix:** Install it: `dnf install -y lcov` (Fedora/RHEL) or
`apt-get install -y lcov` (Debian/Ubuntu).

### "timeout: unrecognized option: signal=KILL"

**Cause:** Alpine Linux uses BusyBox which has different `timeout`
syntax. BusyBox does not support the GNU `--signal=KILL` long option.

**Fix:** Use `-s KILL` instead of `--signal=KILL`. The short option
works on both GNU coreutils and BusyBox.

### "undefined symbol: __gcov_merge_add" when loading plugins

**Cause:** A dynamically-loaded plugin (.so) was compiled without
`--coverage`, or was compiled with a language toolchain (OCaml, Rust)
that doesn't link against libgcov.

**Fix:** Skip tests that load such plugins by adding them to the
skip list in `ci/skip-coverage-tests.sh`.

### Tests hang indefinitely under coverage

**Cause:** Coverage instrumentation slows down I/O-intensive and
timing-sensitive tests enough to cause hangs or infinite loops.

**Fix:**
1. Add a `timeout -s KILL 30m` to the `make check` command
2. Skip the heaviest tests via `ci/skip-coverage-tests.sh`
3. Use `|| true` so hangs don't prevent coverage upload

### Build fails with `-Werror` at `-O0`

**Cause:** Some warnings only appear at `-O0` and are treated as
errors when `-Werror` is active.

**Fix:** Pass `--disable-gcc-warnings` (autotools) or remove
`-Werror` from the coverage build configuration.

### if [ "$failed" ] is always true

**Cause:** In shell, `[ "$failed" ]` tests if the string is non-empty.
The string `"0"` is non-empty, so the test is always true.

**Fix:** Use `[ "$failed" != "0" ]` for numeric comparison.

## Reference

- gcov documentation: https://gcc.gnu.org/onlinedocs/gcc/Gcov.html
- lcov documentation: https://github.com/linux-test-project/lcov
- Codecov lcov support: https://docs.codecov.com/docs/supported-coverage-report-formats
- Codecov CLI: https://github.com/codecov/codecov-cli
- Codecov instance routing: see `codecov-config/CONFIG.md`
- General Codecov onboarding: see `codecov-onboarding/SKILL.md`

## Summary

This skill covers the full C/C++ coverage pipeline:
1. Detecting the build system (autotools, CMake, Meson)
2. Configuring the right compiler/linker flags with workarounds
3. Running tests with timeout protection
4. Generating lcov reports with error tolerance
5. Optionally separating unit vs integration test coverage
6. Skipping problematic tests under coverage instrumentation
7. Uploading to the correct Codecov instance
8. Providing reusable CI job templates and scripts
