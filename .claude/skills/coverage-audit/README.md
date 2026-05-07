# Coverage Audit Skill

Scan a GitHub/GitLab organization and produce a CSV spreadsheet showing code coverage status for every repository.

## Overview

The AI agent writes and runs an audit script on your behalf. You provide the org name, the agent does the rest — no manual scripting needed.

## What It Produces

A CSV file (`<org>-audit.csv`) with one row per repo and these columns:

| Column | Description |
|--------|-------------|
| Repository | Repo name |
| Onboard | TRUE/FALSE — pre-selected for Codecov onboarding |
| URL | GitHub/GitLab link |
| Language | Primary language |
| Stars | Star count |
| Fork | yes/no |
| Archived | yes/no |
| Category | application, fork, archived, documentation, container-image, sample/test, infrastructure, catalog |
| Description | Repo description |
| Has Unit Tests | yes, likely, unknown, n/a |
| Has E2E Tests | yes, unknown, n/a |
| Has Codecov | yes, config-only, no, n/a |
| CI System | GitHub Actions, GitLab CI, Tekton, tox, etc. |
| Codecov Details | OIDC/token, flags, codecov.yml presence |
| Test Details | How tests were detected |
| Contributor 1-3 | Top 3 non-bot contributors |

The `Onboard` column (column B) is a pre-selection flag. Repos classified as good onboarding
candidates (application, not fork/archived, has language, has tests or Codecov) get `TRUE`.
When uploaded to Google Sheets, this renders as a checkbox the user can toggle before
downloading as CSV for the `coverage-jira-tasks` skill.

If Codecov API token is provided, two additional columns appear:

| Column | Description |
|--------|-------------|
| Last Coverage Upload | Date of most recent coverage data on Codecov |
| Coverage % | Current coverage percentage from Codecov |

## Usage

### With Cursor or Claude Code

Just ask:

> "Audit the ansible GitHub org for code coverage status"

The agent will:
1. Ask you to confirm the org name and scope (public/private)
2. Help you set up `GITHUB_TOKEN` if needed
3. Write and execute the audit script
4. Show summary stats
5. Save the CSV

### Prerequisites

- **Python 3** (standard library only, no pip install needed)
- **GitHub token** for API access:
  ```
  export GITHUB_TOKEN=$(gh auth token)
  ```
  Or create one at https://github.com/settings/tokens
- **GitLab token** (if auditing GitLab):
  ```
  export GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxxx
  ```
- **Codecov API token** (optional, enriches results with actual coverage data):
  ```
  export CODECOV_TOKEN=your-codecov-api-token
  ```

### Supported Platforms

- GitHub.com
- GitHub Enterprise (provide instance URL)
- GitLab.com
- Self-hosted GitLab (provide instance URL)

### Output Example

```
Total repos:       301
Application repos: 126
Forks:             40
Archived:          130
Has unit tests:    74
Has Codecov:       16
Gap (tests, no Codecov): 58
```

The **gap** number is the primary onboarding opportunity — repos with tests but no Codecov.

## What Happens Next

1. Upload the CSV to Google Sheets — the `Onboard` column renders as checkboxes
2. Review and toggle checkboxes for repos you want to onboard
3. Download as CSV (File → Download → CSV)
4. Use the **coverage-jira-tasks** skill to generate Jira tasks — it reads the `Onboard` column automatically

## Related Skills

| Skill | Purpose |
|-------|---------|
| **coverage-jira-tasks** | Generate Jira tasks from the audit CSV |
| **codecov-onboarding** | Engineers use this to implement the actual onboarding |
| **coverport-integration** | E2e coverage for containerized Go/Python/Node.js apps |
