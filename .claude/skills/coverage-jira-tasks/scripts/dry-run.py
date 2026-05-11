#!/usr/bin/env python3
"""Dry-run: read audit CSV, classify repos, generate Jira task files.

Output structure:
  <output-dir>/
    <repo-name>/
      task.md              (parent task for the repo)
      subtask-<type>.md    (one per test type)
    _devlake-setup.md      (DevLake project setup task)
    _devlake-dashboard.md  (Metrics dashboard task)
"""

import argparse
import csv
import os
import re
import sys


def has_ci(row):
    ci = row.get("CI System", "").strip().lower()
    return ci and ci != "none detected"


def has_language(row):
    lang = row.get("Language", "").strip()
    return bool(lang) and lang.lower() not in ("makefile", "dockerfile")


def star_count(row):
    try:
        return int(row.get("Stars", "0").strip())
    except ValueError:
        return 0


def is_false_positive_tests(row):
    """Detect repos where Has Unit Tests=yes but tests are actually placeholder/broken."""
    details = row.get("Test Details", "").strip().lower()
    return 'echo "error: no test specified"' in details or "no test specified" in details


def priority_from_stars(stars, base_priority):
    """Boost priority for high-star repos."""
    if stars >= 1000:
        return "Critical"
    if stars >= 100:
        return max_priority(base_priority, "Major")
    if stars >= 30:
        return max_priority(base_priority, "Normal")
    return base_priority


PRIORITY_ORDER = ["Minor", "Normal", "Major", "Critical"]


def max_priority(a, b):
    return PRIORITY_ORDER[max(PRIORITY_ORDER.index(a), PRIORITY_ORDER.index(b))]


def classify_repo(row):
    """Return list of (task_type, priority) or None to skip."""
    category = row.get("Category", "").strip()
    fork = row.get("Fork", "").strip().lower()
    archived = row.get("Archived", "").strip().lower()
    has_tests = row.get("Has Unit Tests", "").strip().lower()
    has_e2e = row.get("Has E2E Tests", "").strip().lower()
    has_codecov = row.get("Has Codecov", "").strip().lower()
    codecov_details = row.get("Codecov Details", "").strip().lower()
    stars = star_count(row)

    if category != "application":
        return None, "skip-non-app"
    if fork == "yes":
        return None, "skip-fork"
    if archived == "yes":
        return None, "skip-archived"

    if not has_language(row) and not has_ci(row):
        return None, "skip-no-code"

    tasks = []

    # Codecov already present
    if has_codecov == "yes":
        if "flags" not in codecov_details:
            p = priority_from_stars(stars, "Critical")
            tasks.append(("fix-codecov", p))
        else:
            p = priority_from_stars(stars, "Major")
            tasks.append(("verify-codecov", p))
    elif has_codecov == "config-only":
        p = priority_from_stars(stars, "Critical")
        tasks.append(("fix-codecov", p))

    # Unit tests present but no Codecov
    elif has_tests in ("yes", "likely"):
        if is_false_positive_tests(row):
            p = priority_from_stars(stars, "Minor")
            tasks.append(("needs-tests", p))
        elif not has_ci(row):
            p = priority_from_stars(stars, "Normal")
            tasks.append(("needs-ci", p))
        else:
            p = priority_from_stars(stars, "Normal")
            tasks.append(("onboard-unit", p))

    # Unknown tests
    elif has_tests == "unknown":
        if has_ci(row) and has_language(row):
            p = priority_from_stars(stars, "Minor")
            tasks.append(("needs-tests", p))
        elif has_language(row):
            p = priority_from_stars(stars, "Minor")
            tasks.append(("investigate", p))

    # E2E tests present (can be in addition to unit test task)
    if has_e2e == "yes":
        lang = row.get("Language", "").strip()
        if lang in ("Go", "Python", "TypeScript", "JavaScript"):
            p = priority_from_stars(stars, "Minor")
            tasks.append(("onboard-e2e", p))

    if tasks:
        return tasks, None
    return None, "skip-no-action"


TASK_TYPE_LABELS = {
    "fix-codecov": "Fix Codecov configuration",
    "verify-codecov": "Verify Codecov setup",
    "onboard-unit": "Onboard unit test coverage to Codecov",
    "investigate": "Investigate tests and onboard to Codecov",
    "needs-tests": "Add tests then onboard to Codecov",
    "needs-ci": "Set up CI then onboard to Codecov",
    "onboard-e2e": "Onboard e2e test coverage via Coverport",
}

TASK_TYPE_JIRA_LABELS = {
    "fix-codecov": "fix-codecov",
    "verify-codecov": "verify-codecov",
    "onboard-unit": "unit-tests",
    "investigate": "investigate",
    "needs-tests": "needs-tests",
    "needs-ci": "needs-ci",
    "onboard-e2e": "e2e-tests",
}

TASK_TYPE_SUMMARIES = {
    "fix-codecov": "Fix Codecov configuration",
    "verify-codecov": "Verify Codecov setup",
    "onboard-unit": "Onboard unit test coverage to Codecov",
    "investigate": "Investigate and onboard to Codecov",
    "needs-tests": "Add unit tests and onboard to Codecov",
    "needs-ci": "Set up CI pipeline and onboard to Codecov",
    "onboard-e2e": "Onboard e2e test coverage via Coverport",
}


def generate_steps(row, task_type):
    lang = row.get("Language", "").strip()
    ci = row.get("CI System", "").strip()
    repo = row.get("Repository", "").strip()
    org = row.get("_org", "")
    codecov_url = f"https://app.codecov.io/gh/{org}/{repo}"

    is_gha = "GitHub Actions" in ci

    if task_type == "fix-codecov":
        if is_gha:
            auth_step = "4. Switch to OIDC authentication (recommended for GitHub Actions) — set `use_oidc: true` in codecov-action and remove `CODECOV_TOKEN` secret"
        else:
            auth_step = "4. Verify `CODECOV_TOKEN` secret is set in CI"
        return f"""1. Review current Codecov configuration in CI workflows
2. Ensure coverage upload uses `flags: unit-tests`
3. Ensure workflow runs on push to `main` (not just PRs)
{auth_step}
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab → click "Enable flag analytics"
"""

    if task_type == "verify-codecov":
        return f"""1. Check [{codecov_url}]({codecov_url}) shows coverage data on main page
2. Verify `unit-tests` flag appears under Flags tab
3. Confirm coverage percentage is non-zero
4. If anything missing, follow fix steps above
"""

    if task_type == "onboard-e2e":
        auth_note = ""
        if is_gha:
            auth_note = "\n6. Use OIDC authentication (recommended for GitHub Actions) — set `use_oidc: true` in codecov-action instead of `CODECOV_TOKEN`"
        return f"""1. Follow the [E2E Code Coverage Guide](https://konflux.pages.redhat.com/docs/users/testing/e2e-code-coverage.html)
2. Add Coverport instrumentation to the application
3. Modify Dockerfile for instrumented builds
4. Update CI pipeline for coverage collection
5. Upload to Codecov with `e2e-tests` flag{auth_note}
"""

    if task_type == "investigate":
        return f"""1. Check if repository has unit tests (look for `_test.go`, `*_test.py`, `*.test.ts`, `*.spec.ts` files)
2. Check if a CI pipeline exists but was not detected by the audit
3. If tests exist, follow onboarding steps for the repo's language
4. If no tests exist but the repo has meaningful code, consider adding basic unit tests
5. If the repo is config-only or has no testable code, close this task as won't-do
"""

    if task_type == "needs-tests":
        lang_hint = ""
        if lang == "Go":
            lang_hint = "\n   - Go: create `*_test.go` files, use `go test ./...`"
        elif lang == "Python":
            lang_hint = "\n   - Python: create `tests/` directory, use `pytest`"
        elif lang in ("TypeScript", "JavaScript"):
            lang_hint = "\n   - JS/TS: add `jest` or `vitest`, create `*.test.ts` files"
        auth_step = "4. Add Codecov upload step with `flags: unit-tests` using OIDC (`use_oidc: true`)" if is_gha else "4. Add Codecov upload step with `flags: unit-tests` using `CODECOV_TOKEN`"
        return f"""1. Identify key packages/modules that should have test coverage
2. Add unit test framework and initial test files:{lang_hint}
3. Add coverage generation to CI pipeline
{auth_step}
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""

    if task_type == "needs-ci":
        return f"""1. Set up CI pipeline (GitHub Actions recommended):
   - Create `.github/workflows/ci.yml`
   - Add test step with coverage generation
2. Add Codecov upload step with `flags: unit-tests` using OIDC (`use_oidc: true` — recommended for GitHub Actions, no token secret needed)
3. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""

    # onboard-unit — language-specific
    if is_gha:
        if lang == "Go":
            return f"""1. Update test step to generate coverage:
   ```
   go test ./... -coverprofile=coverage.out -covermode=atomic
   ```
2. Add Codecov upload step using OIDC (recommended for GitHub Actions — no token secret needed):
   ```yaml
   - uses: codecov/codecov-action@v5
     with:
       use_oidc: true
       flags: unit-tests
       files: ./coverage.out
       fail_ci_if_error: false
   ```
3. Ensure workflow runs on push to `main`
4. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""
        elif lang == "Python":
            return f"""1. Update test step to generate coverage:
   ```
   pip install coverage
   coverage run -m pytest
   coverage xml -o coverage.xml
   ```
2. Add Codecov upload step using OIDC (recommended for GitHub Actions — no token secret needed):
   ```yaml
   - uses: codecov/codecov-action@v5
     with:
       use_oidc: true
       flags: unit-tests
       files: ./coverage.xml
       fail_ci_if_error: false
   ```
3. Ensure workflow runs on push to `main`
4. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""
        elif lang in ("TypeScript", "JavaScript"):
            return f"""1. Update test step to generate coverage:
   ```
   npx jest --coverage   # or: npx vitest --coverage
   ```
2. Add Codecov upload step using OIDC (recommended for GitHub Actions — no token secret needed):
   ```yaml
   - uses: codecov/codecov-action@v5
     with:
       use_oidc: true
       flags: unit-tests
       files: ./coverage/lcov.info
       fail_ci_if_error: false
   ```
3. Ensure workflow runs on push to `main`
4. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""
        else:
            return f"""1. Add coverage generation to test step (language-specific)
2. Add Codecov upload step using OIDC (recommended for GitHub Actions — no token secret needed):
   ```yaml
   - uses: codecov/codecov-action@v5
     with:
       use_oidc: true
       flags: unit-tests
   ```
3. Ensure workflow runs on push to `main`
4. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""
    elif "GitLab CI" in ci:
        return f"""1. Add `CODECOV_TOKEN` CI variable in GitLab
2. Add coverage generation to test job
3. Add upload step using Codecov CLI:
   ```
   curl -Os https://cli.codecov.io/latest/linux/codecov
   chmod +x codecov
   ./codecov upload-process --token $CODECOV_TOKEN --flag unit-tests --file <coverage-file>
   ```
4. Ensure pipeline runs on push to `main`
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""
    else:
        return f"""1. Add `CODECOV_TOKEN` to CI secrets
2. Add coverage generation to test step
3. Upload to Codecov with `--flag unit-tests`
4. Ensure tests run on push to `main`
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""


def _common_task_fields(row, org):
    """Extract common fields used by both parent and flat tasks."""
    repo = row.get("Repository", "").strip()
    lang = row.get("Language", "") or "unknown"
    has_tests = row.get("Has Unit Tests", "")
    has_e2e = row.get("Has E2E Tests", "")
    has_codecov = row.get("Has Codecov", "")
    ci = row.get("CI System", "") or "unknown"
    description = row.get("Description", "").strip()
    stars = star_count(row)
    contributors = []
    for i in range(1, 4):
        c = row.get(f"Contributor {i}", "").strip()
        if c:
            contributors.append(c)

    codecov_status = has_codecov
    if has_codecov == "yes":
        codecov_status = f"yes ({row.get('Codecov Details', '')})"

    desc_line = f"\n> {description}\n" if description else ""
    stars_line = f" ({stars} ⭐)" if stars > 0 else ""
    contributors_line = ""
    if contributors:
        contributors_line = f"\n**Key contacts:** {', '.join(contributors)}\n"

    return {
        "repo": repo, "lang": lang, "has_tests": has_tests, "has_e2e": has_e2e,
        "has_codecov": has_codecov, "ci": ci, "codecov_status": codecov_status,
        "desc_line": desc_line, "stars_line": stars_line, "contributors_line": contributors_line,
        "test_details": row.get("Test Details", "").strip(),
    }


def generate_flat_task(row, task_type, priority, org):
    """Generate a flat task (no subtasks) for repos with a single test type."""
    f = _common_task_fields(row, org)
    summary_label = TASK_TYPE_SUMMARIES.get(task_type, task_type)
    type_label = TASK_TYPE_JIRA_LABELS.get(task_type, task_type)
    labels = f"codecov-onboarding, {type_label}"

    row["_org"] = org
    steps = generate_steps(row, task_type)
    codecov_url = f"https://app.codecov.io/gh/{org}/{f['repo']}"
    is_gha = "GitHub Actions" in f["ci"]

    test_details_line = ""
    if f["test_details"]:
        test_details_line = f"| Test details | {f['test_details']} |\n"

    if is_gha:
        manual_steps = """### Manual Steps Required

- Enable flag analytics in Codecov UI
- No token secret needed — OIDC handles authentication"""
    else:
        manual_steps = """### Manual Steps Required

- Get upload token from Codecov settings
- Add `CODECOV_TOKEN` as CI secret
- Enable flag analytics in Codecov UI"""

    content = f"""---
summary: "{f['repo']}: {summary_label}"
priority: "{priority}"
type: "Task"
labels: "{labels}"
---

### Objective

{summary_label} for [{f['repo']}](https://github.com/{org}/{f['repo']}){f['stars_line']}.
{f['desc_line']}
### Current State

| Item | Status |
|------|--------|
| Unit tests | {f['has_tests']} |
| E2E tests | {f['has_e2e']} |
| Codecov | {f['codecov_status']} |
| CI System | {f['ci']} |
| Language | {f['lang']} |
{test_details_line}{f['contributors_line']}
### Steps

{steps}

{manual_steps}

### Verification

- [ ] CI workflow uploads coverage successfully
- [ ] Codecov dashboard shows coverage data
- [ ] Coverage flag visible in Flags tab
- [ ] Coverage percentage is non-zero

### AI-Assisted Implementation

Most code changes can be automated using the **codecov-onboarding** AI skill:

**Quick Start:** https://github.com/konflux-ci/coverport/blob/main/.claude/skills/codecov-onboarding/README.md
"""
    return content


def generate_parent_task(row, subtasks, org):
    """Generate parent task content for a repo with multiple subtask types."""
    f = _common_task_fields(row, org)

    parent_priority = subtasks[0][1]
    for _, p in subtasks:
        parent_priority = max_priority(parent_priority, p)

    subtask_list = "\n".join(
        f"- {TASK_TYPE_SUMMARIES.get(tt, tt)} (Priority: {p})"
        for tt, p in subtasks
    )

    content = f"""---
summary: "{f['repo']}: Code coverage onboarding"
priority: "{parent_priority}"
type: "Task"
labels: "codecov-onboarding"
---

### Objective

Onboard [{f['repo']}](https://github.com/{org}/{f['repo']}){f['stars_line']} to code coverage tracking.
{f['desc_line']}
### Current State

| Item | Status |
|------|--------|
| Unit tests | {f['has_tests']} |
| E2E tests | {f['has_e2e']} |
| Codecov | {f['codecov_status']} |
| CI System | {f['ci']} |
| Language | {f['lang']} |
{f['contributors_line']}
### Subtasks

{subtask_list}

### AI-Assisted Implementation

Most code changes can be automated using the **codecov-onboarding** AI skill:

**Quick Start:** https://github.com/konflux-ci/coverport/blob/main/.claude/skills/codecov-onboarding/README.md
"""
    return parent_priority, content


def generate_subtask_file(row, task_type, priority, org):
    """Generate subtask content for a specific test type."""
    repo = row.get("Repository", "").strip()
    lang = row.get("Language", "") or "unknown"
    has_tests = row.get("Has Unit Tests", "")
    has_e2e = row.get("Has E2E Tests", "")
    has_codecov = row.get("Has Codecov", "")
    ci = row.get("CI System", "") or "unknown"
    test_details = row.get("Test Details", "").strip()
    is_gha = "GitHub Actions" in ci

    summary_label = TASK_TYPE_SUMMARIES.get(task_type, task_type)
    type_label = TASK_TYPE_JIRA_LABELS.get(task_type, task_type)
    labels = f"codecov-onboarding, {type_label}"

    row["_org"] = org
    steps = generate_steps(row, task_type)

    codecov_url = f"https://app.codecov.io/gh/{org}/{repo}"
    test_details_line = ""
    if test_details:
        test_details_line = f"| Test details | {test_details} |\n"

    if is_gha:
        manual_steps = """### Manual Steps Required

- Enable flag analytics in Codecov UI
- No token secret needed — OIDC handles authentication"""
    else:
        manual_steps = """### Manual Steps Required

- Get upload token from Codecov settings
- Add `CODECOV_TOKEN` as CI secret
- Enable flag analytics in Codecov UI"""

    content = f"""---
summary: "{repo}: {summary_label}"
priority: "{priority}"
type: "Subtask"
labels: "{labels}"
---

### Objective

{summary_label} for [{repo}](https://github.com/{org}/{repo}).

### Current State

| Item | Status |
|------|--------|
| Unit tests | {has_tests} |
| E2E tests | {has_e2e} |
| Codecov | {has_codecov} |
| CI System | {ci} |
| Language | {lang} |
{test_details_line}
### Steps

{steps}

{manual_steps}

### Verification

- [ ] CI workflow uploads coverage successfully
- [ ] Codecov dashboard shows coverage data
- [ ] Coverage flag visible in Flags tab
- [ ] Coverage percentage is non-zero
"""
    return content


def generate_devlake_setup_task(org):
    """Generate DevLake project setup task."""
    content = """---
summary: "Set up DevLake project for code coverage tracking"
priority: "Normal"
type: "Task"
labels: "codecov-onboarding, devlake"
---

### Objective

Create a DevLake project with GitHub and Codecov connections so that code coverage data is collected and available for dashboards and custom queries.

### Prerequisites

- Access to [DevLake UI](https://konflux-devlake-ui-konflux-devlake.apps.rosa.kflux-c-prd-i01.7hyu.p3.openshiftapps.com/)
- A **GitHub Personal Access Token (PAT)** with appropriate scopes (click "Learn how to create a personal access token" in the DevLake GitHub connection form for required permissions)
- A **Codecov API Token** — generate from: `https://app.codecov.io/account/github/<your-org>/access`
- Membership in the target GitHub organization and corresponding Codecov organization access
- Your repos already upload coverage to [Codecov](https://app.codecov.io)

### DevLake Project Structure

Each team gets **one DevLake project** with **one blueprint**.

**Naming convention:** `<Product> - <Team>` (e.g., `Ansible - UI Team`, `OpenShift - Service Mesh`)

Each repo belongs to one team project (the team that owns/maintains it). Keep team projects focused — only repos the team actively works on.

### Steps

1. **Create a DevLake project**
   - Go to [DevLake UI](https://konflux-devlake-ui-konflux-devlake.apps.rosa.kflux-c-prd-i01.7hyu.p3.openshiftapps.com/) → Projects → + New Project
   - Name it following the convention: `<Product> - <Team>`

2. **Configure GitHub connection**
   - Add a Connection → Add New Connection → select GitHub
   - Select GitHub Cloud
   - Input your Personal Access Token
   - Click Test Connection — if successful, click Save Connection
   - Click Add Data Scope and select the repositories you want to track

3. **Configure Codecov connection**
   - Add a Connection → select Codecov
   - Enter the GitHub Organization name
   - Input your Codecov API Token
   - Save the connection and confirm the data scope — select the same repos as the GitHub connection

4. **Run initial data collection**
   - In the project view, go to the Status tab
   - Click Collect Data and monitor the pipeline for completion
   - If the pipeline fails, report it in [#wg-code-coverage](https://redhat.enterprise.slack.com/archives/C09MYT9LQCB)

5. **Verify in DevLake dashboards**
   - Once the pipeline succeeds, click Dashboards
   - Log in via your Google account
   - Navigate to Dashboards → Coverport and select your project/repository to verify data

### Troubleshooting

- **Pipeline fails**: Report in [#wg-code-coverage](https://redhat.enterprise.slack.com/archives/C09MYT9LQCB) with your project name and error
- **No coverage data**: Verify your repos are uploading to Codecov and the Codecov connection scope matches the GitHub connection scope

### Verification

- [ ] DevLake project created with correct naming convention
- [ ] Blueprint has GitHub + Codecov connections scoped to team repos
- [ ] Initial data collection pipeline completed successfully
- [ ] Coverage data visible in DevLake Coverport dashboard
"""
    return content


def generate_devlake_dashboard_task(org):
    """Generate metrics dashboard onboarding task."""
    content = f"""---
summary: "Add team to metrics dashboard (metrics.dprod.io)"
priority: "Normal"
type: "Task"
labels: "codecov-onboarding, devlake"
---

### Objective

Add your team's code coverage widgets to the [metrics dashboard](https://metrics.dprod.io/) by adding an entry to `teams.json`.

### Prerequisites

- DevLake project already set up with GitHub + Codecov connections
- Data collection pipeline completed — coverage data visible in DevLake dashboards
- Your DevLake **blueprint ID** (found in the project settings URL)

### Steps

1. **Add your team to `teams.json`**

   Submit an MR to [n8n-pulumi-poc](https://gitlab.cee.redhat.com/devtools/n8n-pulumi-poc). See the full [team onboarding guide](https://gitlab.cee.redhat.com/devtools/n8n-pulumi-poc/-/blob/main/containers/dashboard/docs/team-onboarding-guide.md).

   Add an entry like this (or add the `codecoverage` dashboard to your existing team entry):

   ```json
   {{{{
     "id": "<product>-<team>",
     "name": "<Display Name>",
     "sources": [
       {{{{"id": "<repo-name>", "type": "github", "owner": "<org>", "name": "<repo>"}}}}
     ],
     "dashboards": [
       {{{{
         "id": "codecoverage",
         "type": "codecoverage",
         "title": "Code Coverage",
         "blueprintid": "<your-blueprint-id>",
         "defaultsource": "<one-repo-name>",
         "description": "Code coverage metrics powered by Codecov.",
         "diagrams": [
           {{{{"position": 0, "type": "json", "endpoint": "https://n8n-poc.dprod.io/webhook/coveragekeymetrics"}}}},
           {{{{"position": 1, "type": "json", "endpoint": "https://n8n-poc.dprod.io/webhook/coveragetrend"}}}},
           {{{{"position": 2, "type": "json", "endpoint": "https://n8n-poc.dprod.io/webhook/coveragelinebreakdown"}}}}
         ],
         "chaturl": "https://n8n-poc.dprod.io/webhook/aa93f8ee-02d9-48df-8f47-481a4e513a5c/chat"
       }}}}
     ]
   }}}}
   ```

2. **Verify**

   After the MR is merged and deployed, check your dashboard at:
   `https://metrics.dprod.io/?team=<your-team-id>&dashboard=codecoverage`

### Troubleshooting

- **Dashboard shows empty**: Check that `blueprintid` in `teams.json` matches your DevLake project's blueprint and that `sources` list matches the repos in your DevLake project
- **Questions**: Ask in [#wg-code-coverage](https://redhat.enterprise.slack.com/archives/C09MYT9LQCB)

### Verification

- [ ] Team entry added to `teams.json` with correct `blueprintid` and sources
- [ ] MR merged to [n8n-pulumi-poc](https://gitlab.cee.redhat.com/devtools/n8n-pulumi-poc)
- [ ] Coverage widgets visible at `https://metrics.dprod.io/?team=<your-team-id>&dashboard=codecoverage`
"""
    return content


def slugify(text):
    return re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")


def main():
    parser = argparse.ArgumentParser(description="Dry-run: generate Jira task files from audit CSV")
    parser.add_argument("--csv", required=True, help="Path to audit CSV file")
    parser.add_argument("--org", required=True, help="GitHub/GitLab org name")
    parser.add_argument("--output-dir", default="./jira-tasks-draft/", help="Output directory for task files")
    parser.add_argument("--types", default=None,
                        help="Comma-separated task types to include (e.g., 'onboard-unit,fix-codecov'). Default: all")
    parser.add_argument("--repos", default=None,
                        help="Comma-separated repo names to include (e.g., 'quay-operator,mirror-registry'). Default: all")
    parser.add_argument("--no-devlake", action="store_true",
                        help="Skip generating DevLake follow-up tasks")
    args = parser.parse_args()

    allowed_types = None
    if args.types:
        allowed_types = set(t.strip() for t in args.types.split(","))
        valid_types = set(TASK_TYPE_LABELS.keys())
        invalid = allowed_types - valid_types
        if invalid:
            print(f"ERROR: Unknown task types: {', '.join(invalid)}")
            print(f"Valid types: {', '.join(sorted(valid_types))}")
            sys.exit(1)

    if not os.path.exists(args.csv):
        print(f"ERROR: CSV file not found: {args.csv}")
        sys.exit(1)

    os.makedirs(args.output_dir, exist_ok=True)

    with open(args.csv, newline="") as f:
        reader = csv.DictReader(f)
        rows = list(reader)

    has_onboard_col = any("Onboard" in (r.keys() if hasattr(r, 'keys') else []) for r in rows[:1])
    if has_onboard_col and not args.repos:
        before = len(rows)
        rows = [r for r in rows if r.get("Onboard", "").strip().upper() == "TRUE"]
        print(f"Read {before} repos from {args.csv}, filtered to {len(rows)} by Onboard=TRUE column\n")
        if len(rows) == 0:
            print("WARNING: No repos have Onboard=TRUE. Check the CSV or use --repos to override.\n")

    allowed_repos = None
    if args.repos:
        allowed_repos = set(r.strip() for r in args.repos.split(","))

    if allowed_repos:
        before = len(rows)
        rows = [r for r in rows if r.get("Repository", "").strip() in allowed_repos]
        print(f"Read {before} repos from {args.csv}, filtered to {len(rows)} by --repos\n")
        missing = allowed_repos - {r.get("Repository", "").strip() for r in rows}
        if missing:
            print(f"WARNING: repos not found in CSV: {', '.join(sorted(missing))}\n")
    elif not has_onboard_col:
        print(f"Read {len(rows)} repos from {args.csv}\n")

    stats = {"parent_tasks": 0, "subtasks": 0, "tasks_by_type": {}}
    skip_stats = {}
    all_repos = []  # (repo_name, parent_priority, subtask_list)
    wave_buckets = {"critical": [], "high": [], "medium": [], "low": []}

    for row in rows:
        result, skip_reason = classify_repo(row)
        if not result:
            skip_stats[skip_reason] = skip_stats.get(skip_reason, 0) + 1
            continue

        # Filter by allowed types if specified
        filtered_subtasks = []
        for task_type, priority in result:
            if allowed_types and task_type not in allowed_types:
                continue
            filtered_subtasks.append((task_type, priority))

        if not filtered_subtasks:
            continue

        repo = row.get("Repository", "").strip()
        repo_slug = slugify(repo)
        repo_dir = os.path.join(args.output_dir, repo_slug)
        os.makedirs(repo_dir, exist_ok=True)

        row["_org"] = args.org

        if len(filtered_subtasks) == 1:
            # Single test type → flat task (no subtasks)
            task_type, priority = filtered_subtasks[0]
            flat_content = generate_flat_task(row, task_type, priority, args.org)
            task_path = os.path.join(repo_dir, "task.md")
            with open(task_path, "w") as f:
                f.write(flat_content)

            stats["parent_tasks"] += 1
            stats["tasks_by_type"][task_type] = stats["tasks_by_type"].get(task_type, 0) + 1
            all_repos.append((repo, priority, filtered_subtasks))
        else:
            # Multiple test types → parent task + subtasks
            parent_priority, parent_content = generate_parent_task(row, filtered_subtasks, args.org)
            parent_path = os.path.join(repo_dir, "task.md")
            with open(parent_path, "w") as f:
                f.write(parent_content)

            for task_type, priority in filtered_subtasks:
                subtask_content = generate_subtask_file(row, task_type, priority, args.org)
                subtask_path = os.path.join(repo_dir, f"subtask-{task_type}.md")
                with open(subtask_path, "w") as f:
                    f.write(subtask_content)

                stats["subtasks"] += 1
                stats["tasks_by_type"][task_type] = stats["tasks_by_type"].get(task_type, 0) + 1

            stats["parent_tasks"] += 1
            all_repos.append((repo, parent_priority, filtered_subtasks))

        # Bucket for wave summary
        if parent_priority == "Critical":
            wave_buckets["critical"].append(repo)
        elif parent_priority == "Major":
            wave_buckets["high"].append(repo)
        elif parent_priority == "Normal":
            wave_buckets["medium"].append(repo)
        else:
            wave_buckets["low"].append(repo)

    # Generate DevLake follow-up tasks
    devlake_count = 0
    if not args.no_devlake and stats["parent_tasks"] > 0:
        setup_content = generate_devlake_setup_task(args.org)
        setup_path = os.path.join(args.output_dir, "_devlake-setup.md")
        with open(setup_path, "w") as f:
            f.write(setup_content)

        dashboard_content = generate_devlake_dashboard_task(args.org)
        dashboard_path = os.path.join(args.output_dir, "_devlake-dashboard.md")
        with open(dashboard_path, "w") as f:
            f.write(dashboard_content)

        devlake_count = 2

    # Print summary
    total_skipped = sum(skip_stats.values())
    print(f"Generated in {args.output_dir}:")
    print(f"  Parent tasks (one per repo): {stats['parent_tasks']}")
    print(f"  Subtasks (per test type):    {stats['subtasks']}")
    if devlake_count:
        print(f"  DevLake follow-up tasks:     {devlake_count}")
    print(f"  Total Jira issues:           {stats['parent_tasks'] + stats['subtasks'] + devlake_count}")
    print(f"\nSkipped {total_skipped} repos:")
    for reason, count in sorted(skip_stats.items()):
        print(f"  {reason}: {count}")

    print(f"\nSubtask breakdown:")
    for tt, count in sorted(stats["tasks_by_type"].items()):
        label = TASK_TYPE_LABELS.get(tt, tt)
        print(f"  {label}: {count}")

    if allowed_types:
        print(f"\n(Filtered to types: {', '.join(sorted(allowed_types))})")

    print(f"\n{'='*60}")
    print("Recommended execution waves:")
    for wave_name, items in [("Wave 1 - Critical", wave_buckets["critical"]),
                              ("Wave 2 - High priority", wave_buckets["high"]),
                              ("Wave 3 - Medium priority", wave_buckets["medium"]),
                              ("Wave 4 - Low priority", wave_buckets["low"])]:
        if items:
            print(f"\n  {wave_name} ({len(items)} repos):")
            for s in items:
                print(f"    - {s}")

    print(f"\n{'='*60}")
    print("Output structure:")
    for repo, priority, subtasks in all_repos:
        if len(subtasks) == 1:
            tt = subtasks[0][0]
            print(f"  {slugify(repo)}/ [{priority}] → flat task: {tt}")
        else:
            subtask_types = ", ".join(tt for tt, _ in subtasks)
            print(f"  {slugify(repo)}/ [{priority}] → subtasks: {subtask_types}")
    if devlake_count:
        print(f"  _devlake-setup.md")
        print(f"  _devlake-dashboard.md")

    print(f"\nReview files in {args.output_dir}/, then run create-tasks.py to push to Jira.")


if __name__ == "__main__":
    main()
