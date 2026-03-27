---
name: codecov-onboarding
description: Onboard repositories to Codecov for unit test coverage tracking. Use this skill when users want to add Codecov integration, configure coverage uploads with flags, or set up CI pipelines to report coverage. Works with GitHub Actions, OpenShift CI (Prow), and GitLab CI. Supports public Codecov (app.codecov.io) and self-hosted Codecov instances.
---

# Codecov Onboarding Skill

This skill helps developers onboard their repositories to Codecov for unit test coverage tracking. It guides users through the entire process: analyzing their repository, configuring coverage uploads with proper flags, and setting up CI integration.

Supports multiple Codecov instances — see `codecov-config/CONFIG.md`
for the instance routing table that maps repository locations to the
correct Codecov instance.

## What is Codecov?

Codecov is a code coverage reporting service that:
1. Collects coverage reports from your CI pipelines
2. Provides visibility into test coverage trends
3. Comments on PRs with coverage changes
4. Supports multiple coverage flags to separate unit tests, integration tests, and e2e tests

## When to Use This Skill

Use this skill when the user:
- Asks to onboard their repository to Codecov
- Wants to add unit test coverage reporting
- Needs to configure coverage uploads with flags
- Mentions integrating Codecov with GitHub Actions, OpenShift CI, or GitLab CI
- Wants to add coverage tracking to their existing test pipeline

**For C/C++ projects:** This skill handles the overall onboarding
workflow. For C/C++ specific coverage generation (gcov, lcov, build
system configuration, workarounds), delegate to the
`c-cpp-coverage` skill which has detailed guidance.

**For e2e coverage** of containerized applications (via Tekton pipelines
or GitHub Actions with coverport CLI), use the `coverport-integration`
skill instead.

## Prerequisites

Before using this skill, verify:
- User has an existing repository with unit tests already running
- User has access to a Codecov instance (see instance routing below)
- User knows which CI system runs their unit tests

## Instructions

### Step 0: Pre-Onboarding Assessment

Before starting, determine the correct Codecov instance and ask the
user these questions. Read `codecov-config/CONFIG.md` for the full
instance routing table.

**Instance routing summary:**
- Public GitHub repos → https://app.codecov.io (OIDC auth)
- Public GitLab.com repos → https://app.codecov.io (token auth)
- Private GitHub repos → https://codecov-codecov.apps.rosa.konflux-qe.zmr9.p3.openshiftapps.com (token auth)
- Internal GitLab (gitlab.cee.redhat.com) → https://codecov-codecov.apps.rosa.kflux-c-stg-i01.qfla.p3.openshiftapps.com (token auth)

Ask the user these questions and wait for responses:

```
I'll help you onboard your repository to Codecov. Let me start with a few questions:

1. **Repository location**: Where is your repository hosted?
   - GitHub (github.com) — public or private?
   - GitLab.com (public)
   - Internal GitLab (gitlab.cee.redhat.com)
   → This determines which Codecov instance to use.

2. **Codecov Account**: Have you signed up at [CODECOV_INSTANCE_URL]?
   - If NO: Please sign up first, then come back.

3. **Upstream vs Fork**: Are you onboarding the upstream (main) repository or a personal fork?
   - ⚠️ **Important:** You should onboard the **upstream repository**, not your fork.
   - Coverage should be tracked on the main project where PRs are merged.
   - Forks inherit coverage data from the upstream repo automatically.
   - If you only have a fork, coordinate with the upstream maintainers to onboard the main repo.

4. **Repository Added**: Is the upstream repository already added to Codecov?
   - If NO: Go to Codecov → Your Org → Click "Configure" on the repo to add it.

5. **CI System**: Which CI system runs your unit tests?
   - GitHub Actions
   - OpenShift CI (Prow)
   - GitLab CI
   - Other (please specify)

6. **Authentication** — choose based on your CI system and Codecov instance:
   - **GitHub Actions + public Codecov (app.codecov.io) → Use OIDC (preferred, no token needed).**
     The workflow just needs `permissions: id-token: write` and `use_oidc: true`.
     Do NOT combine `use_oidc` with `token` — they are mutually exclusive.
   - **GitHub Actions + self-hosted Codecov → Use a token** (OIDC is only available with app.codecov.io).
     Also requires `url:` parameter in the codecov-action.
   - **OpenShift CI / GitLab CI / local uploads → Use a token:**
     - Repository token: Found in Codecov UI → Your Repo → Configure → "Step 3: add token"
     - Global token: Organization Settings → Global Upload Token (requires org admin)

7. **Existing Coverage**: Do you already have any Codecov setup (even partial)?
   - If YES: What's currently configured? (helps me understand what needs updating)
```

Wait for user responses before proceeding.

Once you know the repository location, set these variables for the rest
of the onboarding:
- `CODECOV_INSTANCE_URL` — the Codecov instance URL from the routing table
- `USE_OIDC` — `true` only for public GitHub repos with app.codecov.io
- `NEEDS_CODECOV_URL_FLAG` — `true` for self-hosted instances (CLI needs `--codecov-url`)

### Step 1: Analyze the Repository

Scan the repository to understand the current setup:

1. **Detect programming language(s):**
   ```bash
   # Check for language indicators
   ls -la go.mod package.json requirements.txt Cargo.toml setup.py pyproject.toml 2>/dev/null
   # Check for C/C++ projects (autotools, CMake, Meson)
   ls -la configure.ac CMakeLists.txt meson.build Makefile.am 2>/dev/null
   ```

   **If C/C++ is detected** (configure.ac, CMakeLists.txt, or meson.build
   present), inform the user and read the `c-cpp-coverage` skill for
   detailed coverage generation guidance. That skill covers compiler
   flags, lcov pipeline, build system integration, and workarounds.

2. **Find test configuration:**
   ```bash
   # Check Makefile for test targets
   grep -E "^test|^unit|^coverage" Makefile 2>/dev/null
   # Check package.json scripts
   grep -A20 '"scripts"' package.json 2>/dev/null | grep -i test
   ```

3. **Check for existing Codecov setup:**
   ```bash
   # Look for codecov configuration
   ls -la codecov.yml .codecov.yml 2>/dev/null
   grep -r "codecov" .github/workflows/ --include="*.yml" --include="*.yaml" 2>/dev/null
   grep -r "codecov\|CODECOV" .tekton/ --include="*.yaml" 2>/dev/null
   grep -r "codecov" .gitlab-ci.yml 2>/dev/null
   ```

4. **Find CI configuration:**
   ```bash
   # GitHub Actions
   ls .github/workflows/*.yml .github/workflows/*.yaml 2>/dev/null
   # OpenShift CI - ask user for prow config link
   # GitLab CI
   ls .gitlab-ci.yml 2>/dev/null
   ```

Present findings to the user:

```
I've analyzed your repository. Here's what I found:

**Language(s):** [detected languages]
**Test command:** [if found in Makefile/package.json/etc.]
**CI System:** [detected CI files]
**Existing Codecov setup:** [None / Partial - describe what exists]

Is this correct? Please confirm or let me know if I missed anything.
```

Wait for user confirmation.

### Step 2: Handle Partial Codecov Setup

If the repository already has some Codecov configuration:

**Scenario A: Codecov action exists but no flags**
```
I found an existing Codecov upload step, but it's not using flags.

Current setup:
[show current codecov action/config]

I recommend adding the `unit-tests` flag to properly categorize your coverage.

Proposed change:
[show updated config with flags: unit-tests]

This allows you to:
- Separate unit test coverage from other test types
- Add integration/e2e coverage later with different flags
- Get clearer coverage reports per test category

Should I apply this change? (yes/no)
```

**Scenario B: Codecov configured only on PR, not on push to main**
```
I found Codecov configured for PRs, but not for pushes to main/master.

⚠️ Important: Codecov needs coverage from your main branch to calculate coverage changes on PRs.
Without main branch coverage, PR coverage comments won't show accurate diff coverage.

I recommend adding coverage upload on push to main as well.

Should I update the workflow to also upload coverage on push to main? (yes/no)
```

**Scenario C: Already fully configured**
```
Your repository already has Codecov configured with:
- ✅ Upload on PRs
- ✅ Upload on push to main
- ✅ Using flags: [list flags]

Is there something specific you'd like to change or improve?
```

### Step 3: Recommend Local Upload First

**Always recommend testing locally before CI configuration:**

```
Before configuring CI, I strongly recommend uploading coverage locally first.

This helps:
✅ Verify coverage generation works correctly
✅ Establish a baseline on your main/master branch (required for PR comparisons!)
✅ Confirm the "unit-tests" flag appears in Codecov UI

Here's how:

1. **Switch to main branch:**
   git checkout main

2. **Generate coverage for your unit tests:**
   [provide language-specific command based on analysis]

3. **Install and run Codecov CLI:**
   # Install
   pip install codecov-cli
   # Or download directly:
   curl -Os https://cli.codecov.io/latest/linux/codecov && chmod +x codecov

   # Upload with unit-tests flag
   # For app.codecov.io:
   codecovcli upload-process \
     --token YOUR_CODECOV_TOKEN \
     --flag unit-tests \
     --file [coverage-file-path] \
     --branch main

   # For self-hosted Codecov, add --codecov-url:
   codecovcli upload-process \
     --codecov-url [CODECOV_INSTANCE_URL] \
     --token YOUR_CODECOV_TOKEN \
     --flag unit-tests \
     --file [coverage-file-path] \
     --branch main

4. **Verify in Codecov UI:**
   - Go to [CODECOV_INSTANCE_URL] (the instance from the routing table)
   - Check that coverage appears for your repo
   - Click on "Flags" tab in your repository
   - Click "Enable flag analytics" button to enable flag tracking
   - The "unit-tests" flag should now appear

Have you completed the local upload successfully? (yes/no)
```

Wait for confirmation. If they encounter issues, help troubleshoot (see Troubleshooting section).

### Step 4: Handle Multiple Test Types

If the repository has integration tests that generate coverage without instrumentation:

```
I noticed you may have integration tests in addition to unit tests.

If your integration tests generate coverage data (e.g., they run with coverage flags
and don't require special instrumentation like coverport), you can upload that coverage
too with a different flag.

Options:
1. **Unit tests only (this ticket):** Upload with `--flag unit-tests`
2. **Integration tests too:** Upload with `--flag integration-tests`

For each test type, you'll add a separate upload step in your CI.

Would you like me to configure both unit and integration test coverage uploads? (yes/no)

Note: For e2e tests that require instrumentation (running containerized apps),
see the coverport-integration skill instead.
```

### Step 5: Configure CI Pipeline

Based on the CI system the user specified:

#### Option A: GitHub Actions

Analyze existing workflow files and propose changes.

```
I found your test workflow at: .github/workflows/[filename].yml

I'll propose adding Codecov upload after your test step.

**Changes to .github/workflows/[filename].yml:**
```

**For public repos using app.codecov.io (OIDC auth):**

The job must have `id-token: write` permission for OIDC to work:

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    # Required for OIDC: https://github.com/codecov/codecov-action?tab=readme-ov-file#using-oidc
    permissions:
      id-token: write

    steps:
      # ... checkout, setup, test steps ...

      # Add after your test step
      - name: Upload coverage to Codecov
        uses: codecov/codecov-action@v5
        with:
          use_oidc: true
          flags: unit-tests
          files: [coverage-file-path]  # e.g., coverage.out, coverage.xml
          fail_ci_if_error: false
```

**For private repos using self-hosted Codecov (token auth):**

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ... checkout, setup, test steps ...

      - name: Upload coverage to Codecov
        uses: codecov/codecov-action@v5
        with:
          url: [CODECOV_INSTANCE_URL]  # Self-hosted instance URL
          token: ${{ secrets.CODECOV_TOKEN }}
          flags: unit-tests
          files: [coverage-file-path]
          fail_ci_if_error: false
```

```
**Important configuration:**
- Ensure this runs on BOTH pull requests AND push to main/master
- The workflow trigger should include:
  on:
    push:
      branches: [main, master]
    pull_request:
      branches: [main, master]

**OIDC setup (public repos with app.codecov.io only — no secrets required):**
- OIDC is the preferred method for GitHub Actions — use it for public repos
- OIDC is enabled by default for repos added to Codecov after mid-2024
- Verify in Codecov UI → Your Repo → Settings → General → "GitHub OIDC" is enabled
- The `id-token: write` permission in the workflow is required for GitHub to issue the OIDC token
- **Do NOT use `token` and `use_oidc` together** — they are mutually exclusive
- **OIDC is NOT available for self-hosted Codecov instances** — use token auth with `url:` instead

**Note:** OIDC is not available for pull requests from forks (GitHub does not grant `id-token: write` to fork PRs for security reasons). Fork PR coverage uploads will be skipped, which is acceptable since the baseline comes from main branch pushes.

Should I apply this change to your workflow file? (yes/no)
```

#### Option B: OpenShift CI (Prow)

```
For OpenShift CI, I need to see your Prow job configuration.

Please provide the link to your ci-operator config file in the openshift/release repository.

Example: https://github.com/openshift/release/blob/master/ci-operator/config/[org]/[repo]/[filename].yaml

Once you provide the link, I'll analyze how your tests run and suggest the appropriate changes.
```

After user provides the link, analyze and propose:

```
I've reviewed your Prow configuration. Here's what I suggest:

**1. Create a coverage upload script (hack/codecov.sh):**
```

```bash
#!/bin/bash
set -euo pipefail

# Generate coverage report
# Adjust this command for your project's test setup
[detected-or-suggested-test-command-with-coverage]

# Download and run Codecov CLI
curl -Os https://cli.codecov.io/latest/linux/codecov
chmod +x codecov
./codecov upload-process \
  --token "${CODECOV_TOKEN}" \
  --flag unit-tests \
  --file [coverage-file-path]
```

```
**2. Add Makefile target:**
```

```makefile
.PHONY: coverage
coverage:
	hack/codecov.sh
```

```
**3. Add coverage job to your ci-operator config (presubmit - runs on PRs):**
```

```yaml
- as: coverage
  commands: |
    export CODECOV_TOKEN=$(cat /tmp/secret/CODECOV_TOKEN)
    make coverage
  container:
    from: src
  secret:
    mount_path: /tmp/secret
    name: [repo]-codecov-token
```

```
**4. Add postsubmit job (runs on push to main - IMPORTANT for baseline!):**
```

```yaml
- as: publish-coverage
  commands: |
    export CODECOV_TOKEN=$(cat /tmp/secret/CODECOV_TOKEN)
    make coverage
  container:
    from: src
  postsubmit: true
  secret:
    mount_path: /tmp/secret
    name: [repo]-codecov-token
```

```
**5. Secret setup required:**
You'll need to add your Codecov token to the openshift-ci vault.

See: https://docs.ci.openshift.org/docs/how-tos/adding-a-new-secret-to-ci/

The secret should be named: [repo]-codecov-token
With key: CODECOV_TOKEN

Should I create the hack/codecov.sh script and Makefile target? (yes/no)

Note: The ci-operator config changes need to be made in the openshift/release repository.
I can show you the exact YAML to add, but you'll need to submit a PR there.
```

#### Option C: GitLab CI

Analyze `.gitlab-ci.yml` and propose changes.

**For C/C++ projects:** Read the `c-cpp-coverage` skill first — it has
a complete GitLab CI job template with all necessary workarounds
(compiler flags, lcov pipeline, test timeout, etc.).

**Choose the appropriate GitLab CI configuration based on the GitLab instance:**

##### GitLab.com (Public GitLab) → app.codecov.io

For public GitLab.com repositories using app.codecov.io:

```
I found your GitLab CI configuration at: .gitlab-ci.yml

I'll propose adding Codecov upload to your test job.

**Add to your existing test job or create a new job:**
```

```yaml
coverage-upload:
  stage: test
  script:
    # Your existing test command with coverage
    - [test-command-with-coverage]
    # Download Codecov CLI binary
    - curl -Os https://cli.codecov.io/latest/linux/codecov
    - chmod +x codecov
    # Upload to app.codecov.io
    - ./codecov upload-process --token $CODECOV_TOKEN --flag unit-tests --file [coverage-file-path]
  rules:
    # Run on main branch (for baseline)
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
    # Run on merge requests
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
```

```
**Setup required:**
1. Add CODECOV_TOKEN as a CI/CD variable:
   - Go to GitLab → Repo → Settings → CI/CD → Variables
   - Add variable: CODECOV_TOKEN
   - Mark as "Masked" and "Protected"
```

##### Internal GitLab (gitlab.cee.redhat.com) → Self-Hosted Codecov

For internal GitLab repositories using the self-hosted Codecov instance,
use the Python-based codecov-cli installed via pip. This is the recommended
approach for enterprise GitLab environments.

```
I found your GitLab CI configuration at: .gitlab-ci.yml

Since you're using internal GitLab (gitlab.cee.redhat.com), I'll configure
uploads to the self-hosted Codecov instance using the Python codecov-cli.

**Recommended pipeline structure (separate upload job):**
```

```yaml
variables:
  # Self-hosted Codecov instance for internal GitLab
  CODECOV_URL: "https://codecov-codecov.apps.rosa.kflux-c-stg-i01.qfla.p3.openshiftapps.com"

stages:
  - test
  - analysis

# Your existing test job(s) that generate coverage
unit-tests:
  stage: test
  image: registry.access.redhat.com/ubi9/go-toolset:1.22
  script:
    - go test -v -coverprofile=coverage.out -covermode=atomic ./...
    - go tool cover -func=coverage.out
  artifacts:
    paths:
      - coverage.out
    expire_in: 7 days
  rules:
    - when: always

# Codecov upload job (runs after test jobs)
codecov-upload:
  stage: analysis
  image: registry.access.redhat.com/ubi9/python-311:latest
  needs:
    - unit-tests
  script:
    # Install codecov-cli via pip
    - pip install codecov-cli

    # Upload to self-hosted Codecov
    # IMPORTANT: --slug must use equals-sign syntax (--slug="value") to prevent
    # shell word-splitting in GitLab CI when the slug contains slashes.
    # Also: use the upstream repo slug, NOT ${CI_PROJECT_PATH}, because
    # CI_PROJECT_PATH reflects the fork's path in MR pipelines from forks.
    - codecovcli --enterprise-url ${CODECOV_URL} upload-process --token ${CODECOV_TOKEN} --slug="${CODECOV_SLUG}" --flag unit-tests --file coverage.out --disable-search --git-service gitlab_enterprise
  variables:
    # Hardcode the upstream repo slug — do NOT use ${CI_PROJECT_PATH} as it
    # will be the fork's path when running MR pipelines from forks.
    CODECOV_SLUG: "[upstream-org]/[upstream-repo]"
  allow_failure: true
  rules:
    - if: $CODECOV_TOKEN
```

```
**Key differences for internal GitLab:**
- Uses `pip install codecov-cli` instead of downloading the binary
- Uses `codecovcli` command (not `./codecov`)
- Uses `--enterprise-url` to specify the self-hosted Codecov instance
- Uses `--git-service gitlab_enterprise` for enterprise GitLab integration
- Uses `--slug="${CODECOV_SLUG}"` with equals-sign syntax to identify the project
  (plain `--slug ${VAR}` causes "unexpected extra argument" errors due to shell splitting)
- Uses a hardcoded `CODECOV_SLUG` variable with the upstream repo path — do NOT use
  `${CI_PROJECT_PATH}` because it reflects the fork's path in MR pipelines from forks
- Uses `--disable-search` to upload only the specified file

**Setup required:**
1. Add CODECOV_TOKEN as a CI/CD variable:
   - Go to GitLab → Repo → Settings → CI/CD → Variables
   - Add variable: CODECOV_TOKEN (get from Codecov UI → Your Repo → Configure)
   - Mark as "Masked" and "Protected"

Should I apply this change to your .gitlab-ci.yml? (yes/no)
```

### Step 6: Verification Checklist

After making changes, provide verification steps:

```
I've made the proposed changes locally. Here's your verification checklist:

**Immediate verification:**
□ Review the changes I made to ensure they look correct
□ Test coverage generation locally one more time

**After pushing changes:**
□ Commit and push to a branch, open a PR
□ Check CI logs for "Codecov upload successful" or similar message
□ Verify in Codecov UI ([CODECOV_INSTANCE_URL]):
  - Coverage data appears for your commit
  - "unit-tests" flag is visible in the Flags tab
  - Coverage diff is shown on PR (if main branch baseline exists)

**After merging to main:**
□ Verify postsubmit/push job runs and uploads coverage
□ Future PRs should now show accurate coverage comparisons

If you encounter any issues, let me know and I'll help troubleshoot!
```

### Step 7: Optional codecov.yml Configuration

```
**Optional:** You can add a codecov.yml file to customize Codecov behavior.

**Benefits:**
- Exclude files/directories that won't be covered by tests (prevents misleading low coverage)
- Set coverage thresholds and status checks
- Configure PR comment format and layout
- Enable carryforward flags (useful when not all tests run on every commit)
- Configure how partial line coverage is counted
- Prepare for additional flags (e.g., integration-tests, e2e-tests in future)
```

#### Why Coverage Percentages May Differ Between Tools

If users report that Codecov shows different coverage than another dashboard
(e.g., Konflux coverage dashboard), explain these common causes:

1. **Different file scope:** Codecov analyzes ALL files in the repository by
   default, while other tools may only count files that appear in coverage
   reports. If your tests cover 12 files but the repo has 20 Go files,
   Codecov will show lower overall coverage.

2. **Partial line coverage:** Codecov counts "partial" coverage (e.g., a line
   with a branch where only one path is covered) differently than some tools.
   By default, partials count as uncovered. You can change this behavior.

3. **Different exclude patterns:** Each tool may have different default
   exclusions for vendor, test, or generated files.

**Solution:** Create a `codecov.yml` with `ignore:` patterns and optionally
configure partial coverage behavior.

```
**Recommended codecov.yml with ignore patterns:**
```

```yaml
codecov:
  require_ci_to_pass: yes

coverage:
  precision: 2
  round: down
  range: "50...100"

  status:
    project:
      default:
        # Only measure coverage on files that your tests actually target
        # Partials (partially covered lines) count as hits, not misses
        # This aligns Codecov's calculation with tools that don't penalize partials
        partials_as_hits: true
    patch:
      default:
        partials_as_hits: true

# Ignore files/directories that tests won't cover
# Adjust these patterns based on your project structure
ignore:
  - "vendor/**"           # Vendored dependencies
  - "**/mock_*.go"        # Generated mocks
  - "**/*_mock.go"        # Generated mocks (alternate naming)
  - "**/zz_generated*.go" # Kubernetes generated files
  - "**/fake_*.go"        # Fake implementations for testing
  - "test/**"             # Test utilities and fixtures
  - "hack/**"             # Build/CI scripts
  - "docs/**"             # Documentation
  - "examples/**"         # Example code
  # Add project-specific patterns:
  # - "cmd/tools/**"      # CLI tools not covered by unit tests
  # - "internal/legacy/**" # Legacy code being phased out

flags:
  unit-tests:
    carryforward: true
  # Uncomment when you add integration tests:
  # integration-tests:
  #   carryforward: true

comment:
  layout: "reach,diff,flags,files"
  behavior: default
```

```
**When to recommend ignore patterns:**

If the user reports coverage discrepancies, help them identify files to ignore:

1. Ask: "Are there files in your repository that your tests don't cover
   (e.g., generated code, legacy code, CLI tools, examples)?"

2. Check for common patterns:
   - `zz_generated*.go` (Kubernetes controller-gen output)
   - `mock_*.go` or `*_mock.go` (mockgen output)
   - `vendor/` directory
   - `fake_*.go` (testing fakes)
   - CLI tools in `cmd/` that aren't unit tested
   - `examples/` or `docs/` directories

3. Suggest adding these to the `ignore:` section in codecov.yml.

**Note on partials_as_hits:**
- When `partials_as_hits: true`, a line with partial branch coverage
  (e.g., an `if` where only the true branch is tested) counts as covered.
- This aligns with tools that only count "hit vs not hit" per line.
- Set to `false` (default) if you want stricter branch coverage reporting.

Would you like me to create a codecov.yml file for your repository? (yes/no)
```

## Coverage Generation by Language

Provide language-specific guidance based on detected language:

### Go

```bash
# Standard Go tests
go test -v -coverprofile=coverage.out ./...

# With Ginkgo
ginkgo -v --cover --coverprofile=coverage.out ./...
# Or using go test (also works with Ginkgo)
go test -v -coverprofile=coverage.out ./...
```

**Important:** Target unit test directories, not e2e tests:
```bash
# Good - unit tests
go test -v -coverprofile=coverage.out ./pkg/... ./internal/... ./cmd/...

# Avoid - e2e tests (use coverport for these)
go test -v -coverprofile=coverage.out ./test/e2e/...
```

### Python

```bash
pip install pytest-cov
pytest --cov=<package> --cov-report=xml:coverage.xml
```

### TypeScript/JavaScript

```bash
# Jest
jest --coverage --coverageReporters=lcov

# Vitest
vitest run --coverage
```

### Rust

```bash
cargo install cargo-tarpaulin
cargo tarpaulin --out Xml
```

### C/C++

C/C++ coverage is significantly more complex than other languages.
**Read the `c-cpp-coverage` skill** for comprehensive guidance including:
- Build system integration (autotools, CMake, Meson)
- Compiler flag workarounds (`_FORTIFY_SOURCE`, `-Werror`)
- The full lcov pipeline with error handling
- Test suite management under coverage overhead
- Reusable CI job templates and scripts

Quick reference (see the skill for full details):
```bash
# Compile with coverage
CFLAGS="-g -O0 --coverage" LDFLAGS="--coverage" ./configure && make
# Run tests
make check
# Generate lcov report
lcov --capture --directory . --output-file coverage.info \
  --ignore-errors format,gcov,unsupported,negative
lcov --remove coverage.info '/usr/*' '*/tests/*' \
  --output-file coverage.info --ignore-errors format,negative
```

## Troubleshooting

Common issues and solutions:

### Upload Fails: "Unable to locate build"

**Cause:** Codecov can't match the upload to a commit/PR.

**Solutions:**
- Ensure you're passing the correct branch name: `--branch main`
- For PRs, the branch should be the PR branch, not main
- Check that the commit SHA is correct
- Verify the repository is properly added to Codecov

### Upload Fails: "Token not found" or "Authentication failed"

**Cause:** Missing or incorrect Codecov token (for OpenShift CI / GitLab CI).

**Solutions:**
- Verify the token is correctly set in CI secrets
- Check for extra whitespace or newlines in the token
- Try regenerating the token in Codecov UI
- For GitHub Actions, consider switching to OIDC instead (no token needed)

### Upload Fails: "Unable to get ACTIONS_ID_TOKEN_REQUEST_URL" (GitHub Actions OIDC)

**Cause:** OIDC token is not available in the workflow context.

**Solutions:**
- Ensure the job has `permissions: id-token: write` set
- Check that OIDC is enabled in Codecov UI → Your Repo → Settings → General → "GitHub OIDC"
- OIDC is NOT available for pull requests from forks — this is a GitHub security restriction. Fork PR coverage will be skipped, which is acceptable
- If running on `pull_request_target`, ensure the workflow is properly configured
- Verify you are using `codecov/codecov-action@v5` (OIDC support requires v5)

### Coverage Report Not Found

**Cause:** The coverage file path is incorrect or file wasn't generated.

**Solutions:**
- Verify the coverage file exists after tests run: `ls -la coverage.out`
- Check the file path matches what you pass to Codecov
- Ensure tests actually ran (check for test failures)
- Verify the coverage format is supported by Codecov

### PR Comments Don't Show Coverage Diff

**Cause:** No baseline coverage on main/master branch.

**Solutions:**
- Upload coverage from main branch first (local upload or CI)
- Ensure postsubmit job runs on push to main
- Wait for the main branch upload to complete before checking PRs
- Check Codecov UI to verify main branch has coverage data

### Flags Not Appearing in Codecov UI

**Cause:** Flag analytics must be enabled manually in Codecov UI.

**Solutions:**
- Go to your repository in Codecov UI
- Click on the "Flags" tab
- Click "Enable flag analytics" button
- After enabling, flags from your uploads will appear
- Verify your upload included `--flag unit-tests` in the command
- Check CI logs for the upload command to confirm flags were passed

### OpenShift CI: Secret Not Found

**Cause:** Codecov token secret not configured in openshift-ci vault.

**Solutions:**
- Follow the secret setup guide: https://docs.ci.openshift.org/docs/how-tos/adding-a-new-secret-to-ci/
- Verify secret name matches what's in your ci-operator config
- Check the secret has the correct key name (CODECOV_TOKEN)
- Ensure the secret is available to your repository's jobs

### Partial Coverage Data

**Cause:** Not all tests are generating coverage, or coverage files are overwritten.

**Solutions:**
- If running multiple test suites, merge coverage files before upload
- For Go: `go test -coverprofile=coverage.out ./...` runs all at once
- For separate runs, use coverage merge tools
- Verify all test commands include coverage flags

### Upload Fails: "Got unexpected extra argument" (GitLab CI --slug)

**Cause:** The `--slug` value contains slashes (e.g., `org/group/repo`) and
shell word-splitting in GitLab CI causes the CLI to parse it as a positional
argument instead of the option value.

**Solutions:**
- Use equals-sign syntax: `--slug="${CODECOV_SLUG}"` instead of `--slug ${CODECOV_SLUG}`
- This applies to both `upload-process` and `do-upload` subcommands
- Note: `--slug` is a **subcommand** option, not a global `codecovcli` option — place it after the subcommand name
- YAML multiline blocks (`|`) with `\` continuations can also cause quoting issues; prefer single-line commands

### Upload Reports to Wrong Repo (Fork vs Upstream)

**Cause:** Using `${CI_PROJECT_PATH}` for `--slug` reports coverage to the
fork instead of the upstream repo when MR pipelines run from forks.

**Solutions:**
- Hardcode the upstream repo slug in a CI variable (e.g., `CODECOV_SLUG: "org/repo"`)
- Do NOT use `${CI_PROJECT_PATH}` — it reflects the fork's path in MR pipelines
- The Codecov repo must be onboarded under the upstream path, not the fork

### GitLab CI: Variable Not Available

**Cause:** CI variable not properly configured or protected.

**Solutions:**
- Check variable is set in Settings → CI/CD → Variables
- If variable is "Protected", ensure it's available on your branch
- For MRs from forks, protected variables may not be available
- Consider using masked but not protected for broader access

## Validation

After integration, provide these verification steps:

1. **Local verification:**
   - Generate coverage locally
   - Upload with Codecov CLI
   - Check Codecov UI for data

2. **PR verification:**
   - Open a PR with the CI changes
   - Verify coverage uploads in CI logs
   - Check Codecov comments on PR (may need main baseline first)

3. **Main branch verification:**
   - Merge PR or push to main
   - Verify postsubmit/push job uploads coverage
   - Confirm main branch shows in Codecov UI

4. **Subsequent PR verification:**
   - Open another PR after main has coverage
   - Verify coverage diff appears in PR comments
   - Check flags are properly displayed

## Related Skills

- **`codecov-config/CONFIG.md`** — Codecov instance routing table (which
  repo location maps to which Codecov instance). Read this first.
- **`c-cpp-coverage`** — Detailed C/C++ coverage generation with gcov/lcov.
  Use when a C/C++ project is detected.
- **`coverport-integration`** — E2E test coverage for containerized apps
  using coverport. Use for Go/Python/Node.js apps with Tekton pipelines
  or GitHub Actions (coverport CLI container via podman).

## Reference Documentation

- Codecov Quick Start: https://docs.codecov.com/docs/quick-start
- Codecov OIDC (GitHub Actions): https://github.com/codecov/codecov-action?tab=readme-ov-file#using-oidc
- Codecov Tokens: https://docs.codecov.com/docs/codecov-tokens
- Codecov Flags: https://docs.codecov.com/docs/flags
- Codecov YAML Reference: https://docs.codecov.com/docs/codecov-yaml
- Codecov CLI: https://github.com/codecov/codecov-cli
- Supported Coverage Formats: https://docs.codecov.com/docs/supported-coverage-report-formats
- OpenShift CI Secrets: https://docs.ci.openshift.org/docs/how-tos/adding-a-new-secret-to-ci/
- CoverPort (for e2e coverage): https://konflux.pages.redhat.com/docs/users/testing/e2e-code-coverage.html

## Best Practices

1. **Test locally first** - Always verify coverage generation and upload locally before configuring CI
2. **Establish main baseline** - Upload from main branch first for accurate PR comparisons
3. **Use meaningful flags** - Separate unit-tests, integration-tests, e2e-tests
4. **Ask when unsure** - If file paths or commands are unclear, ask the user
5. **Propose, don't assume** - Show proposed changes and wait for confirmation
6. **Keep existing logic** - Don't break existing test configurations
7. **Document manual steps** - Clearly explain any steps user must do outside the repo (secrets, Codecov UI)

## Summary

This skill helps onboard repositories to Codecov by:
1. Assessing current Codecov setup (if any)
2. Analyzing repository structure and CI configuration
3. Recommending local upload first to establish baseline
4. Handling multiple test types (unit, integration) with flags
5. Configuring the appropriate CI system (GitHub Actions, OpenShift CI, GitLab CI)
6. For OpenShift CI, ensuring postsubmit jobs run for main branch coverage
7. Providing optional codecov.yml configuration
8. Troubleshooting common issues

The result is automated coverage uploads with proper flag categorization, enabling teams to track unit test coverage trends and get coverage feedback on PRs.

For C/C++ projects, this skill delegates to the `c-cpp-coverage` skill
for the coverage generation details. For e2e coverage of containerized
apps (Tekton or GitHub Actions), see the `coverport-integration` skill.
For Codecov instance routing, see `codecov-config/CONFIG.md`.
