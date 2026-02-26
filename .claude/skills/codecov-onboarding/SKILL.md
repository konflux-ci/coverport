---
name: codecov-onboarding
description: Onboard repositories to Codecov for test coverage tracking. Use this skill when users want to add Codecov integration, configure coverage uploads with flags, or set up CI pipelines to report coverage. Works with GitHub Actions, OpenShift CI (Prow), and GitLab CI.
---

# Codecov Onboarding Skill

This skill helps developers onboard their repositories to Codecov for test coverage tracking. It guides users through analyzing their test setup, verifying coverage generation works, configuring CI pipelines, and uploading coverage with proper flags.

## When to Use This Skill

Use this skill when the user:
- Asks to onboard their repository to Codecov
- Wants to add test coverage reporting
- Needs to configure coverage uploads with flags
- Mentions integrating Codecov with GitHub Actions, OpenShift CI, or GitLab CI
- Wants to add coverage tracking to their existing test pipeline

## Prerequisites

Before using this skill, verify:
- User has an existing repository with tests
- User has access to Codecov (https://app.codecov.io)
- User knows which CI system runs their tests

## Instructions

### Step 0: Pre-Onboarding Assessment

Before starting, ask the user these questions and wait for responses:

```
I'll help you onboard your repository to Codecov. Let me start with a few questions:

1. **Codecov Account**: Have you signed up at https://app.codecov.io using your GitHub/GitLab account?
   - If NO: Please sign up first, then come back.

2. **Upstream vs Fork**: Are you onboarding the upstream (main) repository or a personal fork?
   - ⚠️ **Important:** You should onboard the **upstream repository**, not your fork.
   - Forks inherit coverage data from the upstream repo automatically.

3. **Repository Added**: Is the upstream repository already added to Codecov?
   - If NO: Go to Codecov → Your Org → Click "Configure" on the repo to add it.

4. **Upload Token**: Do you have your Codecov upload token ready?
   - Repository token: Found in Codecov UI → Your Repo → Configure → "Step 3: add token"
   - Global token: Organization Settings → Global Upload Token (requires org admin)

5. **CI System**: Which CI system runs your tests?
   - GitHub Actions
   - OpenShift CI (Prow)
   - GitLab CI
   - Other (please specify)

6. **Existing Coverage**: Do you already have any Codecov setup (even partial)?
```

Wait for user responses before proceeding.

### Step 1: Analyze the Repository

Scan the repository to understand the current setup:

1. **Detect programming language(s):**
   ```bash
   ls -la go.mod package.json requirements.txt Cargo.toml setup.py pyproject.toml 2>/dev/null
   ```

2. **Find test configuration:**
   ```bash
   grep -E "^test|^unit|^coverage" Makefile 2>/dev/null
   grep -A20 '"scripts"' package.json 2>/dev/null | grep -i test
   ```

3. **Discover test types and how they're separated:**
   - **Python:** Look for pytest markers (`grep -r "@pytest.mark" tests/`), check `pytest.ini`/`pyproject.toml` for marker definitions, look for separate Makefile targets (`test-unit`, `test-integration`)
   - **Go:** Check for separate test directories (`test/unit/`, `test/integration/`, `test/e2e/`), build tags, or Makefile targets
   - **JS/TS:** Check `package.json` scripts for separate test commands
   - **General:** Check Makefile for targets like `test-unit`, `test-integration`, `test-e2e`

4. **Check for existing Codecov setup:**
   ```bash
   ls -la codecov.yml .codecov.yml 2>/dev/null
   grep -r "codecov" .github/workflows/ --include="*.yml" --include="*.yaml" 2>/dev/null
   grep -r "codecov\|CODECOV" .tekton/ --include="*.yaml" 2>/dev/null
   ```

5. **Find CI configuration:**
   ```bash
   ls .github/workflows/*.yml .github/workflows/*.yaml 2>/dev/null
   ls .gitlab-ci.yml 2>/dev/null
   ```

Present findings including discovered test types:

```
I've analyzed your repository. Here's what I found:

**Language(s):** [detected languages]
**Test types discovered:**
  - [type 1, e.g., "unit tests (pytest -m 'not integration')"]
  - [type 2, e.g., "integration tests (pytest -m integration)"]
**Test command(s):** [if found in Makefile/CI/etc.]
**CI System:** [detected CI files]
**Existing Codecov setup:** [None / Partial - describe what exists]

Is this correct? Please confirm or let me know if I missed anything.
```

Wait for user confirmation.

### Step 2: Verify Tests Run with Coverage Locally

Before configuring CI, verify that tests actually pass and produce coverage.

1. **Install dependencies** -- check for requirements files, lock files, setup.py/pyproject.toml:
   ```bash
   # For Python: install the package in dev mode + test dependencies
   pip install -e .
   pip install -r requirements-test.txt  # or tests/requirements-test.txt

   # For Go: no special install needed
   # For JS/TS: npm install or yarn install
   ```

2. **Run tests with coverage** for each discovered test type:

   Provide language-specific commands -- see [reference.md](reference.md) for full list.

   **Python example:**
   ```bash
   # All tests
   pytest --cov=<package> --cov-report=xml:coverage.xml tests/
   # Or by marker
   pytest -m "not integration" --cov=<package> --cov-report=xml:coverage-unit.xml tests/
   pytest -m integration --cov=<package> --cov-report=xml:coverage-integration.xml tests/
   ```

   **Go example:**
   ```bash
   go test -v -coverprofile=coverage.out ./pkg/... ./internal/... ./cmd/...
   ```

3. **Verify coverage file was generated:**
   ```bash
   ls -la coverage*.xml coverage.out  # depending on language
   ```

4. **Fix any issues** before proceeding. Common problems:
   - Missing test dependencies (add to requirements/package.json)
   - Package not installed in dev mode (`pip install -e .`)
   - Coverage package not installed (`pip install pytest-cov`, etc.)

If tests fail or coverage isn't generated, fix the issues first. Do NOT proceed to CI configuration until tests pass locally with coverage.

### Step 3: Decide on Test Type Coverage Strategy

Based on the test types discovered in Step 1, decide how to configure coverage:

**If only one test type exists:** Use a single coverage upload with `--flag unit-tests`.

**If multiple test types exist (e.g., unit + integration):** Evaluate each:
- **Same infrastructure** (same language, same tooling, just different markers/dirs): configure a separate coverage upload for each with its own flag (e.g., `unit-tests`, `integration-tests`). Each gets its own CI step or job.
- **Different infrastructure** (needs real databases, containers, special services): note it for future work, configure what's feasible now.

Do NOT create separate CI jobs for test types unless they have genuinely different infrastructure requirements (e.g., one needs Docker, the other doesn't). If both run with the same tooling, a single job with multiple test+upload steps is cleaner.

Present the proposed strategy to the user and confirm before proceeding.

### Step 4: Handle Partial Codecov Setup

If the repository already has some Codecov configuration:

**Scenario A: Codecov action exists but no flags**
- Recommend adding flags to properly categorize coverage
- Show current vs proposed config

**Scenario B: Codecov configured only on PR, not on push to main**
- Codecov needs main branch coverage for accurate PR diffs
- Recommend adding coverage upload on push to main as well

**Scenario C: Already fully configured**
- Ask if there's something specific to change or improve

### Step 5: Recommend Local Upload First

**Always recommend testing the upload locally before CI configuration:**

```
Before configuring CI, I recommend uploading coverage locally to establish a baseline.

1. Switch to main branch: git checkout main
2. Generate coverage (using commands from Step 2)
3. Install and run Codecov CLI:
   pip install codecov-cli
   codecovcli upload-process \
     --token YOUR_CODECOV_TOKEN \
     --flag unit-tests \
     --file [coverage-file-path] \
     --branch main
4. Verify in Codecov UI:
   - Coverage appears for your repo
   - Click "Flags" tab → "Enable flag analytics"
   - The "unit-tests" flag should appear

Have you completed the local upload successfully? (yes/no)
```

If configuring multiple test types, upload each with its own flag.

### Step 6: Configure CI Pipeline

Based on the CI system and the test types to cover, configure the pipeline.

**Key principle:** Each test type needs a step/job that (1) runs tests with coverage flags and (2) uploads the coverage to Codecov with the appropriate flag.

#### Option A: GitHub Actions

**For a single test type** -- add coverage + upload to the existing test job:

```yaml
- name: Run tests with coverage
  run: |
    pip install -e .
    pip install pytest-cov
    pytest --cov=<package> --cov-report=xml:coverage.xml tests/
- name: Upload coverage to Codecov
  uses: codecov/codecov-action@v5
  with:
    token: ${{ secrets.CODECOV_TOKEN }}
    flags: unit-tests
    files: coverage.xml
    fail_ci_if_error: false
```

**For multiple test types** -- create a dedicated coverage job:

```yaml
coverage:
  name: Test coverage
  runs-on: ubuntu-latest
  steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-python@v5
    with:
      python-version: '3.12'
  - name: Install dependencies
    run: |
      pip install -e .
      pip install -r tests/requirements-test.txt
  - name: Run unit tests with coverage
    run: |
      python3 -m pytest -m "not integration" -v \
        --cov=<package> --cov-report=xml:coverage-unit.xml tests/
  - name: Upload unit test coverage
    uses: codecov/codecov-action@v5
    with:
      token: ${{ secrets.CODECOV_TOKEN }}
      flags: unit-tests
      files: coverage-unit.xml
      fail_ci_if_error: false
  - name: Run integration tests with coverage
    run: |
      python3 -m pytest -m integration -v \
        --cov=<package> --cov-report=xml:coverage-integration.xml tests/
  - name: Upload integration test coverage
    uses: codecov/codecov-action@v5
    with:
      token: ${{ secrets.CODECOV_TOKEN }}
      flags: integration-tests
      files: coverage-integration.xml
      fail_ci_if_error: false
```

Adapt the test commands to match what was verified in Step 2.

**Important:**
- Ensure the workflow triggers on BOTH `push` to main AND `pull_request`
- Add `CODECOV_TOKEN` as a repository secret in GitHub Settings → Secrets

#### Option B: OpenShift CI (Prow)

1. Create a coverage upload script (`hack/codecov.sh`):
```bash
#!/bin/bash
set -euo pipefail
[detected-test-command-with-coverage]
curl -Os https://cli.codecov.io/latest/linux/codecov
chmod +x codecov
./codecov upload-process \
  --token "${CODECOV_TOKEN}" \
  --flag unit-tests \
  --file [coverage-file-path]
```

2. Add Makefile target and ci-operator jobs (presubmit + postsubmit).
   See [reference.md](reference.md) for full OpenShift CI configuration details.

**Secret setup:** Add Codecov token to openshift-ci vault per https://docs.ci.openshift.org/docs/how-tos/adding-a-new-secret-to-ci/

#### Option C: GitLab CI

Add coverage generation + Codecov upload to your test job. See [reference.md](reference.md) for GitLab CI configuration details.

**Secret setup:** Add `CODECOV_TOKEN` as a masked CI/CD variable in GitLab Settings.

### Step 7: Verification Checklist

```
Here's your verification checklist:

**Immediate:**
□ Review the changes to ensure they look correct
□ Test coverage generation locally one more time

**After pushing:**
□ Open a PR with the changes
□ Check CI logs for "Codecov upload successful" or similar
□ Verify in Codecov UI:
  - Coverage data appears for your commit
  - Flags are visible in the Flags tab
  - Coverage diff shown on PR (if main baseline exists)

**After merging to main:**
□ Verify push job uploads coverage
□ Future PRs should show accurate coverage comparisons
```

### Step 8: Optional codecov.yml Configuration

```yaml
codecov:
  require_ci_to_pass: yes

coverage:
  precision: 2
  round: down
  range: "50...100"

flags:
  unit-tests:
    carryforward: true
  # Add flags for each test type configured:
  # integration-tests:
  #   carryforward: true

comment:
  layout: "reach,diff,flags,files"
  behavior: default
```

Update the flags section to include all test types that were configured.

## Best Practices

1. **Verify locally first** - Always confirm tests pass with coverage before touching CI
2. **Establish main baseline** - Upload from main branch first for accurate PR comparisons
3. **Use meaningful flags** - Separate unit-tests, integration-tests, e2e-tests
4. **Don't over-split** - Only separate test types in CI when they have different infrastructure needs
5. **Propose, don't assume** - Show proposed changes and wait for confirmation
6. **Keep existing logic** - Don't break existing test configurations
7. **Document manual steps** - Clearly explain steps the user must do outside the repo (secrets, Codecov UI)

## Reference

For language-specific coverage commands, CI configuration details (OpenShift CI, GitLab CI), and troubleshooting, see [reference.md](reference.md).

## Reference Documentation

- Codecov Quick Start: https://docs.codecov.com/docs/quick-start
- Codecov Flags: https://docs.codecov.com/docs/flags
- Codecov YAML Reference: https://docs.codecov.com/docs/codecov-yaml
- Codecov CLI: https://github.com/codecov/codecov-cli
- OpenShift CI Secrets: https://docs.ci.openshift.org/docs/how-tos/adding-a-new-secret-to-ci/
