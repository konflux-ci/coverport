# Codecov Instance Configuration

Shared reference for all coverage skills. This file maps repository
locations to the correct Codecov instance and provides common
configuration that other skills read instead of hardcoding URLs.

## Instance Routing

| Repository Location | Codecov Instance | Auth Method |
|---|---|---|
| Public GitHub (github.com) | https://app.codecov.io | OIDC (GitHub Actions) or token (other CI) |
| Public GitLab (gitlab.com) | https://app.codecov.io | Token |
| Private GitHub (github.com, private repos) | https://codecov-codecov.apps.rosa.konflux-qe.zmr9.p3.openshiftapps.com | Token |
| Internal GitLab (gitlab.cee.redhat.com) | https://codecov-codecov.apps.rosa.kflux-c-stg-i01.qfla.p3.openshiftapps.com | Token |

## Authentication: OIDC vs Token

**OIDC is only available via `codecov/codecov-action@v5` in GitHub
Actions** with `use_oidc: true`. It is the preferred method for public
GitHub repos using app.codecov.io — no token secret needed.

OIDC is **NOT available** for:
- Self-hosted Codecov instances (use token + `url:` parameter)
- The Codecov CLI binary (`./codecov upload-process`) — always needs `--token`
- GitLab CI (uses CLI directly)
- Tekton/OpenShift CI (uses CLI directly)

In those cases, use a `CODECOV_TOKEN` (repository or global upload token).

## How to Determine the Correct Instance

Ask the user these questions in order:

1. **Where is the repository hosted?**
   - `github.com` → continue to question 2
   - `gitlab.com` → use **app.codecov.io**
   - `gitlab.cee.redhat.com` → use **internal self-hosted** instance

2. **Is the GitHub repository public or private?**
   - Public → use **app.codecov.io**
   - Private → use **public self-hosted** instance

## Codecov CLI Installation Methods

There are two ways to install the Codecov CLI:

### Method 1: Binary Download (GitHub Actions, Tekton, most CI systems)

For **app.codecov.io** (official SaaS):
```bash
curl -Os https://cli.codecov.io/latest/linux/codecov
chmod +x codecov
./codecov upload-process \
  --token "${CODECOV_TOKEN}" \
  --flag <flag-name> \
  --file <coverage-file>
```

For **self-hosted instances**, add `--codecov-url`:
```bash
curl -Os https://cli.codecov.io/latest/linux/codecov
chmod +x codecov
./codecov upload-process \
  --codecov-url <CODECOV_INSTANCE_URL> \
  --token "${CODECOV_TOKEN}" \
  --flag <flag-name> \
  --file <coverage-file>
```

### Method 2: Pip Installation (Recommended for Internal GitLab)

For **internal GitLab (gitlab.cee.redhat.com)** using the self-hosted
Codecov instance, use the Python-based codecov-cli installed via pip.
This is the recommended approach for enterprise GitLab environments:

```bash
pip install codecov-cli

codecovcli --enterprise-url <CODECOV_INSTANCE_URL> upload-process \
  --token "${CODECOV_TOKEN}" \
  --slug "${CI_PROJECT_PATH}" \
  --flag <flag-name> \
  --file <coverage-file> \
  --disable-search \
  --git-service gitlab_enterprise
```

Key differences with the pip-based CLI:
- Command is `codecovcli` (not `./codecov`)
- Uses `--enterprise-url` (not `--codecov-url`)
- Requires `--git-service gitlab_enterprise` for enterprise GitLab
- Requires `--slug` to specify the project path
- Uses `--disable-search` to upload only the specified file

The `--codecov-url` / `--enterprise-url` flag tells the CLI to send
coverage data to the self-hosted instance instead of app.codecov.io.

## GitHub Actions — Codecov Action

**Public repos with app.codecov.io — use OIDC (preferred):**

```yaml
- name: Upload coverage to Codecov
  uses: codecov/codecov-action@v5
  with:
    use_oidc: true
    flags: unit-tests
    files: <coverage-file>
    fail_ci_if_error: false
```

The job must have `permissions: id-token: write`. Do NOT combine
`use_oidc` with `token` — they are mutually exclusive.

**Private repos with self-hosted Codecov — use token:**

```yaml
- name: Upload coverage to Codecov
  uses: codecov/codecov-action@v5
  with:
    url: <CODECOV_INSTANCE_URL>
    token: ${{ secrets.CODECOV_TOKEN }}
    flags: unit-tests
    files: <coverage-file>
    fail_ci_if_error: false
```

## CI Systems by Repository Location

| Repository Location | Typical CI Systems | E2E Coverage (coverport) |
|---|---|---|
| Public GitHub | GitHub Actions, Tekton/Konflux pipelines | Tekton task or GitHub Actions (coverport CLI via podman) |
| Private GitHub | GitHub Actions, Tekton/Konflux pipelines | Tekton task or GitHub Actions (coverport CLI via podman) |
| Public GitLab | GitLab CI | Tekton task |
| Internal GitLab | GitLab CI | Tekton task |

## Coverage Flags Convention

Use consistent flag names across all repositories:

| Flag | Meaning |
|---|---|
| `unit-tests` | Unit test coverage (language-native test runner) |
| `integration-tests` | Integration test coverage (without container instrumentation) |
| `e2e-tests` | E2E test coverage via coverport (instrumented containers) |
