---
name: coverage-audit
description: Audit a GitHub/GitLab organization's repositories for code coverage status. Generates CSV spreadsheet showing which repos have tests, Codecov integration, and what's missing. Use when user asks to audit an org, create a coverage spreadsheet, or assess coverage gaps across repositories. Also works with pre-existing repo lists (JSON, CSV, text files) spanning multiple orgs/groups.
---

# Coverage Audit Skill

Scan GitHub/GitLab org (or a pre-existing repo list) → produce CSV with coverage status per repo.

## When to Use

- User asks to audit a GitHub/GitLab org for coverage
- User wants a spreadsheet of repos with test/Codecov status
- User needs to identify coverage gaps across an organization
- User has a file (JSON, CSV, text) listing repos to audit instead of a whole org
- User wants to audit repos from a self-hosted GitLab instance

## Important: You Execute Everything

The user should NOT write or run scripts manually. YOU (the AI agent)
write the audit script, execute it, and present results. The user only
provides input (org name, scope) and reviews output.

## Important: Ask Before Acting

**Never assume. Always confirm.** If you are unsure about anything —
org name spelling, scope (public vs private), platform (GitHub vs GitLab)
— stop and ask the user before proceeding.

Rules:
- Do NOT start the audit without confirming org name and scope
- If the org has many repos (100+), warn user it may take several minutes
- If GitHub/GitLab API rate limit is low, warn user and ask how to proceed
- If results look unexpected (0 repos, all archived), flag it and ask
- Show summary stats after audit completes, ask if user wants to re-run with different parameters

## Prerequisites

### GitHub

- Token with `repo` scope (private repos) or `public_repo` (public only)
- For org-level metadata: `read:org` scope
- Help the user set `GITHUB_TOKEN` if needed:
  ```
  export GITHUB_TOKEN=$(gh auth token)
  ```
  Or if they don't have `gh` CLI, walk them through creating a token at
  https://github.com/settings/tokens
- **GitHub Enterprise**: If user provides a GHE URL, use `https://<host>/api/v3/` as the API base instead of `https://api.github.com/`

### GitLab

- Token with `read_api` scope
- Help the user set `GITLAB_TOKEN`:
  ```
  export GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxxx
  ```
  Create at `https://gitlab.com/-/user_settings/personal_access_tokens`
  (or equivalent for self-hosted GitLab)
- **Self-hosted GitLab**: Use `https://<host>/api/v4/` as the API base
- **Token discovery — parent agent only.** Run discovery once before dispatching any
  subagents. Once resolved, include the literal token value in each subagent's Task
  instructions as: `"Set GITLAB_TOKEN=<resolved-value> before running any shell commands."`
  Subagents cannot inherit environment variables from the parent — the value must be
  embedded in their instructions explicitly. Never log or print the token value.

- **Token discovery order** (try each in order, use first found):
  1. `GITLAB_TOKEN` or `GITLAB_PERSONAL_ACCESS_TOKEN` env var
  2. MCP server config in `~/.claude/settings.json` — look for a GitLab MCP server entry:
     ```python
     import json, os
     with open(os.path.expanduser("~/.claude/settings.json")) as f:
         cfg = json.load(f)
     for name, server in cfg.get("mcpServers", {}).items():
         env = server.get("env", {})
         token = env.get("GITLAB_PERSONAL_ACCESS_TOKEN") or env.get("GITLAB_TOKEN")
         gitlab_url = env.get("GITLAB_URL")  # e.g., "https://gitlab.cee.redhat.com"
         if token:
             break
     ```
  3. `git credential fill` — assign to a variable only, never print or echo the value:
     ```python
     import subprocess
     result = subprocess.run(
         ['git', 'credential', 'fill'],
         input=f'protocol=https\nhost=<gitlab-hostname>\n'.encode(),
         capture_output=True
     )
     token = None
     for line in result.stdout.decode().splitlines():
         if line.startswith('password='):
             token = line[len('password='):]  # split on first = only, preserves = in value
             break
     # token is now available as a Python string within this script only.
     # Do NOT use os.environ — environment changes don't persist across Shell tool calls
     # or to subagents. Instead, embed the resolved value directly in each subagent's
     # instructions so they can: export GITLAB_TOKEN=<value>
     ```
     ⚠️ The credential helper may return a **login password** rather than a PAT,
     depending on how it was configured. Verify the value is a GitLab PAT (typically
     starts with `glpat-`) before using it as `GITLAB_TOKEN`. If unsure, fall through
     to step 4 and ask the user.
  4. Ask the user directly as last resort — request they set `GITLAB_TOKEN` in their
     environment or provide it in chat for this session only. Do **not** suggest printing
     or echoing credential values to discover them.
- **SSL certificate issues**: Self-hosted GitLab instances often use internal CAs. If `urllib` raises SSL errors, create a permissive SSL context (`ssl.CERT_NONE`). Check MCP config for `NODE_TLS_REJECT_UNAUTHORIZED=0` as a signal. Warn the user when disabling SSL verification.
- **MCP tool fallback**: Do NOT rely on GitLab MCP tools being connected — they may be unavailable to the session or to subagents. Always use direct API calls via `urllib.request` as the primary approach. MCP tools are a convenience, not a dependency.

## Instructions

### Step 1: Gather Info

Ask user:
1. **Org name** — GitHub org URL or name (e.g., `ansible`, `konflux-ci`), or GitLab group path (e.g., `gitlab-org/ci-cd`)
2. **Scope** — public repos only, or include private?
3. **Platform** — GitHub (default), GitHub Enterprise, GitLab.com, or self-hosted GitLab?
4. **Codecov API** — do they have a Codecov API token? (optional, enriches results with actual coverage data)

If platform is GitHub Enterprise or self-hosted GitLab, also ask for the instance URL.

### Step 1b: Alternative Repo Sources (Repo List Mode)

If the user already has a list of repos (instead of a whole org to scan), use **repo list mode** — skip API pagination and scan from the list directly.

**Supported formats:**

1. **Team JSON config files** (e.g., `containers/dashboard/teams/*.json`):
   ```json
   {
     "sources": [{
       "type": "gitrepos",
       "repos": [
         {"id": "repo-id", "type": "gitlab", "owner": "group/subgroup", "name": "project-name"},
         {"id": "repo-id", "type": "github", "owner": "org-name", "name": "repo-name"}
       ]
     }]
   }
   ```
   - Filter by `type` field: `"gitlab"` or `"github"`
   - Full path for GitLab = `owner + "/" + name`
   - URL = `https://{gitlab_host}/{owner}/{name}`
   - Deduplicate by `(type, owner, name)` across multiple files

2. **CSV file** — expect columns like `repo_url` or `owner,name` or `full_path`
3. **Plain text** — one repo URL or `owner/name` per line

**Pattern**: Read list → look up each project via `GET /projects/{url_encoded_full_path}` → scan each. The per-repo scan logic (tree fetch, test detection, Codecov detection) is identical to org-scan mode.

URL-encode the full path: `urllib.parse.quote("group/subgroup/project", safe="")` → `group%2Fsubgroup%2Fproject`.

**Output naming**: Derive from input source (e.g., `sre-audit.csv` from `teams/sre.json`), or ask user.

### Step 2: Write and Run Audit Script

Create a Python script that:

1. **Fetches all repos** via platform API with pagination
   - **GitHub**: `GET /orgs/{org}/repos?per_page=100&page=N` — follow `Link` header or increment page until empty response
   - **GitLab**: `GET /groups/{group}/projects?per_page=100&page=N&include_subgroups=true` — follow `X-Next-Page` header
   - **GitLab (single project)**: `GET /projects/{url_encoded_path}` — use `urllib.parse.quote("group/project", safe="")` for path encoding
2. **Detect default branch** per repo — use `default_branch` field from API response (do NOT hardcode `main` or `master`)
3. **For each repo**, collects:
   - Basic metadata: name, URL, language, stars, fork/archived status, description
   - **Category** classification (see below)
   - **Test detection**: check for test dirs, test files, test commands in CI
   - **Codecov detection**: check workflows for `codecov` references, check for `codecov.yml`
   - **CI system**: GitHub Actions, GitLab CI, Tekton, tox, etc.
   - **Top 3 contributors** (excluding bots)

   **API efficiency:** Fetch the recursive tree ONCE per repo. Extract workflow paths, test file paths, CI indicators, and language hints all from the same tree response. Cache workflow file contents when fetched for test detection and reuse for Codecov detection — do NOT fetch the same file twice.

   **Language inference:** When GitHub API returns `null` for language, infer from file extensions in the tree (e.g., `*.go` → Go, `*.py` → Python, `*.ts` → TypeScript). Use the most common extension. This prevents repos with code from being misclassified as no-language.

   **GitLab API endpoints** (all under `https://{host}/api/v4`):
   - **Repository tree**: `GET /projects/{id}/repository/tree?recursive=true&ref={default_branch}&per_page=100` — returns file paths for test/CI detection. Use project numeric `id` (from project lookup) or URL-encoded path.
   - **File content**: `GET /projects/{id}/repository/files/{url_encoded_filepath}/raw?ref={default_branch}` — encode filepath with `urllib.parse.quote(filepath, safe="")`.
   - **Languages**: `GET /projects/{id}/languages` — returns `{"Go": 65.3, "Python": 34.7}`. More reliable than tree-based inference.
   - **Contributors**: `GET /projects/{id}/repository/contributors?order_by=commits&sort=desc&per_page=3` — returns `[{"name": "...", "email": "...", "commits": N}]`.
   - **URL encoding**: Always encode project paths with `urllib.parse.quote(path, safe="")`. Slashes MUST be encoded as `%2F`.
   - **Auth header**: Use `Private-Token: {token}` (NOT `Authorization: Bearer`).
   - **Pagination**: `X-Next-Page` header (empty string = no more pages), `X-Total-Pages` for progress display.

4. **Writes CSV** sorted by repo name
5. **Saves progress** to `<org>-audit-progress.json` every 10 repos (enables resume on interruption)

#### Category Classification

Classify each repo into one of (check in this order — first match wins):
- `archived` — archived/deprecated (check BEFORE fork — archived forks are just archived)
- `fork` — fork of upstream project
- `documentation` — docs-only repos (reStructuredText, HTML, MDX, no app code). Language = MDX, HTML, CSS, or reStructuredText triggers this
- `sample/test` — example, sample, demo, or test repos. Match "sample", "demo", "example", "quickstart", "test" ANYWHERE in name (not just prefix), but exclude repos with substantial source code
- `ci-tooling` — reusable CI actions/workflows. Detect by: `action.yml` or `action.yaml` at repo root, or name contains "actions" and repo has no application source code
- `configuration` — config-only repos (renovate config, shared settings). Detect by: name contains "config", "renovate-config", "settings", AND repo has no programming language or only YAML/JSON
- `container-image` — Dockerfile-only, no application logic
- `infrastructure` — deployment manifests, Helm charts, GitOps config
- `catalog` — pipeline/task catalogs (Tekton, CI definitions)
- `application` — has source code, potential for tests (default/fallback)

#### Test Detection

Check in this order:

**GitHub repos:**
1. **CI workflows** — scan `.github/workflows/*.yml` for test commands:
   `go test`, `pytest`, `npm test`, `make test`, `tox`, `coverage run`, `jest`, `vitest`, `unittest`, `nox`
2. **Test directories** — check for `tests/`, `test/`, `tests/unit/`, `tests/e2e/`, `integration-tests/`
3. **Language-specific** — `package.json` scripts for JS/TS, `*_test.go` for Go

**GitLab repos:**
1. **CI config** — scan `.gitlab-ci.yml` for test stages/jobs:
   Look for stage names containing `test`, job scripts containing `go test`, `pytest`, `npm test`, `make test`, `tox`, `coverage run`
2. **Test directories** — same as GitHub
3. **Language-specific** — same as GitHub
4. **`include:` directives** — note if `.gitlab-ci.yml` uses `include:` (may pull test jobs from templates — mark as `likely` if includes exist but no explicit test commands found)

Values: `yes`, `likely`, `unknown`, `n/a` (for skipped categories)

#### Codecov Detection

**From CI config** — scan workflow files for:
- `codecov/codecov-action` or `codecov` CLI usage
- `use_oidc: true` vs `CODECOV_TOKEN`
- `flags:` configuration
- `codecov.yml` or `.codecov.yml` config file

**From Codecov API** (optional, if user has Codecov access):
- `GET https://app.codecov.io/api/v2/github/{owner}/repos/{repo}/` — check if repo exists on Codecov
- `GET https://app.codecov.io/api/v2/github/{owner}/repos/{repo}/commits/` — check for recent coverage uploads
- Requires `CODECOV_TOKEN` (API token from codecov.io account settings)
- If Codecov API available, add `Last Coverage Upload` and `Coverage %` columns

**For GitLab**: check `.gitlab-ci.yml` for `coverage:` keyword regex, `artifacts: reports: coverage_report:` and codecov uploader usage.

Values: `yes`, `config-only`, `no`, `n/a`

#### Skip Deep Scan

For `archived`, `fork`, `documentation`, `container-image`, `sample/test`, `ci-tooling`, `configuration` categories — skip workflow/test analysis, set fields to `n/a`.

#### Rate Limiting & Error Handling

**GitHub:**
- Check `api.github.com/rate_limit` before and after
- Add `time.sleep(0.15-0.3)` between API calls
- Warn if <500 calls remaining
- On 403 with `X-RateLimit-Remaining: 0`, calculate wait from `X-RateLimit-Reset` and ask user

**GitLab:**
- Check `RateLimit-Remaining` response header
- Default 10 req/sec for authenticated, 5 for unauthenticated
- Respect `Retry-After` header on 429

**Retry logic:**
- On 5xx or network timeout: retry up to 3 times with exponential backoff (1s, 2s, 4s)
- On 404 for specific repo content (workflow files, codecov.yml): treat as "not found", do NOT retry
- On 403 (not rate limit): log warning, skip repo, continue audit

**Progress & Resume:**
- Save completed repos to `<org>-audit-progress.json` every 10 repos
- On startup, check for progress file — if found, ask user: resume or start fresh?
- Progress file stores: org name, timestamp, list of completed repo names + their data
- Print progress: `[42/301] Scanning repo-name...` to show advancement

### Step 3: CSV Output Format

Columns (in order):

```
Repository, Onboard, URL, Language, Stars, Fork, Archived, Category, Description,
Has Unit Tests, Has E2E Tests, Has Codecov, CI System, Codecov Details,
Test Details, Contributor 1, Contributor 2, Contributor 3
```

File name: `<org>-audit.csv`

#### Onboard Column

The last column `Onboard` is a pre-selection flag for the `coverage-jira-tasks` skill.
Set it to `TRUE` for repos the audit thinks are worth onboarding to Codecov, `FALSE` otherwise.

**Selection logic** — set `TRUE` when ALL of these hold:
- Category = `application`
- Not a fork, not archived
- Has a programming language (not just Makefile/Dockerfile)
- Has unit tests (`yes` or `likely`) OR has Codecov already (`yes` or `config-only`)

This gives the user a pre-filtered list they can review in Google Sheets
(checkboxes render as clickable toggles). The `coverage-jira-tasks` skill
reads this column to know which repos to create tasks for — no manual
repo selection needed.

**Important:** This is a suggestion, not a decision. The user curates the
checkboxes in the spreadsheet before downloading as CSV.

### Step 3.5: Verify CSV Output

After writing CSV, verify it by reading first 5 rows with Python's csv module:
```python
python3 -c "import csv; r=csv.DictReader(open('org-audit.csv')); [print(dict(row)) for row in list(r)[:5]]"
```
Do NOT use `column -s',' -t` — it breaks on quoted fields containing commas (e.g., "GitHub Actions, Tekton, Make").

Also sanitize description fields before writing: strip newlines, replace commas in descriptions with semicolons, and truncate to 200 chars.

### Step 4: Print Summary Stats

After CSV generation, print:

```
Total repos:              N
Application repos:        N
Forks:                    N
Archived:                 N
Has unit tests:           N
Has Codecov:              N
Gap (tests, no Codecov):  N
```

The **gap** number = repos with tests but no Codecov. This is the primary onboarding opportunity.

## Bot Exclusion List

Filter these from contributor lists:
`dependabot[bot]`, `dependabot-preview[bot]`, `github-actions[bot]`,
`renovate[bot]`, `mergify[bot]`, `codecov[bot]`, `pre-commit-ci[bot]`,
`snyk-bot`, `mend-bolt-for-github[bot]`, `ansibullbot`, `ansibot`,
`patchback[bot]`, and any login ending in `[bot]`.

## Example

User: "Audit the ansible GitHub org for coverage status"

Result: `ansible-audit.csv` with 301 rows + summary showing 74 repos with tests, 16 with Codecov, 58 gaps.
