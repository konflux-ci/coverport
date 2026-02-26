# Codecov Onboarding - Reference

Detailed reference material for the codecov-onboarding skill. Read this file when you need language-specific coverage commands, full CI configuration examples, or troubleshooting guidance.

## Coverage Generation by Language

### Go

```bash
# Standard Go tests
go test -v -coverprofile=coverage.out ./...

# With Ginkgo
ginkgo -v --cover --coverprofile=coverage.out ./...

# Target unit test directories only
go test -v -coverprofile=coverage.out ./pkg/... ./internal/... ./cmd/...
```

### Python

```bash
pip install pytest-cov

# All tests
pytest --cov=<package> --cov-report=xml:coverage.xml tests/

# By pytest marker
pytest -m "not integration" --cov=<package> --cov-report=xml:coverage-unit.xml tests/
pytest -m integration --cov=<package> --cov-report=xml:coverage-integration.xml tests/

# By directory
pytest --cov=<package> --cov-report=xml:coverage.xml tests/unit/
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

```bash
gcc --coverage -o myprogram myprogram.c
./myprogram
gcov myprogram.c
# Or use lcov for better output
lcov --capture --directory . --output-file coverage.info
```

## OpenShift CI (Prow) - Full Configuration

### 1. Coverage upload script (hack/codecov.sh)

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

### 2. Makefile target

```makefile
.PHONY: coverage
coverage:
	hack/codecov.sh
```

### 3. Presubmit job (runs on PRs)

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

### 4. Postsubmit job (runs on push to main - required for baseline)

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

### 5. Secret setup

Add your Codecov token to the openshift-ci vault:
- Guide: https://docs.ci.openshift.org/docs/how-tos/adding-a-new-secret-to-ci/
- Secret name: `[repo]-codecov-token`
- Key: `CODECOV_TOKEN`

The ci-operator config changes are made in the openshift/release repository via PR.

## GitLab CI - Full Configuration

```yaml
coverage-upload:
  stage: test
  script:
    - [test-command-with-coverage]
    - pip install codecov-cli
    - codecovcli upload-process --token $CODECOV_TOKEN --flag unit-tests --file [coverage-file-path]
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
```

Setup: Add `CODECOV_TOKEN` as a CI/CD variable in GitLab → Settings → CI/CD → Variables (masked and protected).

## Troubleshooting

### Upload Fails: "Unable to locate build"

**Cause:** Codecov can't match the upload to a commit/PR.

**Solutions:**
- Ensure correct branch name: `--branch main`
- Check that the commit SHA is correct
- Verify the repository is properly added to Codecov

### Upload Fails: "Token not found" or "Authentication failed"

**Cause:** Missing or incorrect Codecov token.

**Solutions:**
- Verify token is correctly set in CI secrets
- Check for extra whitespace or newlines
- Try regenerating the token in Codecov UI

### Coverage Report Not Found

**Cause:** Coverage file path incorrect or file wasn't generated.

**Solutions:**
- Verify coverage file exists after tests: `ls -la coverage.*`
- Check file path matches what's passed to Codecov
- Ensure tests ran successfully

### PR Comments Don't Show Coverage Diff

**Cause:** No baseline coverage on main/master branch.

**Solutions:**
- Upload coverage from main branch first (local upload or CI)
- Ensure postsubmit/push job runs on push to main
- Wait for main branch upload before checking PRs

### Flags Not Appearing in Codecov UI

**Cause:** Flag analytics must be enabled manually.

**Solutions:**
- Go to Codecov UI → your repo → "Flags" tab
- Click "Enable flag analytics"
- Verify upload included `--flag` in the command

### OpenShift CI: Secret Not Found

**Solutions:**
- Follow: https://docs.ci.openshift.org/docs/how-tos/adding-a-new-secret-to-ci/
- Verify secret name matches ci-operator config
- Check the key name is `CODECOV_TOKEN`

### Partial Coverage Data

**Solutions:**
- If running multiple test suites, ensure each generates its own coverage file
- Don't overwrite coverage files between test runs (use unique names)
- For Go: `go test -coverprofile=coverage.out ./...` runs all at once
