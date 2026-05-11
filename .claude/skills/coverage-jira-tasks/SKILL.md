---
name: coverage-jira-tasks
description: Generate and create Jira tasks for code coverage onboarding from an audit CSV. Two-phase workflow — dry-run generates task files locally for review, then create-tasks pushes them to Jira under a target epic. Use when user wants to create Jira tasks from an audit spreadsheet, convert coverage audit results into actionable work items, or batch-create Jira issues for coverage gaps across an org. Also use when user mentions "jira tasks from audit", "coverage onboarding tasks", "create tasks for repos without coverage", or wants to track coverage work in Jira.
---

# Coverage Jira Tasks Skill

Read audit CSV → classify repos → generate one task per repo with subtasks per test type + DevLake follow-up tasks → push to Jira epic.

## When to Use

- User has a coverage audit CSV (from `coverage-audit` skill or manual)
- User wants to create Jira tasks for repos that need coverage onboarding
- User wants to preview tasks before creating them in Jira
- User wants to filter tasks by type (e.g., only repos ready for onboarding)

## Important: You Execute Everything

The user should NOT run scripts manually. YOU (the AI agent) execute all
scripts on behalf of the user. The user only provides input (org name,
epic key, etc.) and reviews/confirms results.

## Important: Ask Before Acting

**Never assume. Always confirm.** If you are unsure about anything —
a parameter, a classification, whether to proceed — stop and ask the
user. This skill creates real Jira issues, so accuracy matters more
than speed.

Rules:
- Do NOT create Jira tasks without explicit user confirmation
- Do NOT guess CSV file paths, epic keys, or project keys — ask
- Do NOT skip the dry-run phase — always show summary first
- If a repo classification looks wrong, flag it and ask
- If Jira API returns errors, stop and explain — do not retry blindly
- After dry-run, wait for user to say "go ahead" or similar before Phase 2

## Two-Phase Workflow

### Phase 1: Dry Run

YOU run `scripts/dry-run.py` — reads CSV, classifies repos, generates an output directory with:
- One subdirectory per repo containing `task.md` (parent) + `subtask-<type>.md` files
- Two `_devlake-*.md` files at the root level (DevLake follow-up tasks)

Show summary to user for review.

### Phase 2: Create Tasks

After user confirms, YOU run `scripts/create-tasks.py` — reads the output directory, creates:
1. One parent **Task** per repo under the epic
2. **Subtasks** under each parent task (one per test type)
3. Two **DevLake follow-up Tasks** directly under the epic

## Prerequisites

The user typically has nothing set up locally — just a Google Sheets URL with the audit data and a Jira epic they want tasks under. The skill should handle the full onboarding: downloading the CSV, setting up credentials, and creating tasks. Don't assume anything is ready — check and guide.

## Instructions

### Step 1: Gather Input

Many of these will already be in the user's message — extract what you can and only ask for what's missing:

1. **Audit data source** — one of:
   - **Google Sheets URL** (most common): The sheet is almost always in a company Google
     account and **cannot be exported via API** (requires auth). Ask the user to download
     it as CSV manually:
     1. Open the Google Sheets link
     2. File → Download → Comma Separated Values (.csv)
     3. Provide the downloaded file path (typically `~/Downloads/<filename>.csv`)
     
     Only attempt `curl` export if the user explicitly says the sheet is publicly shared.
   - **Local CSV path**: Use directly
   - **No source mentioned**: Look for `*-audit.csv` files in the working directory. If none found, ask.

2. **Epic key** — Jira epic to link tasks to (always ask — no default). User may provide a Jira URL like `https://redhat.atlassian.net/browse/PROJ-123` — extract the key (`PROJ-123`).
3. **Jira project** — project key for new tasks (default: derived from epic key)
4. **GitHub/GitLab org name** — for constructing Codecov URLs. Can often be inferred from repo URLs the user provides, or from the CSV content (repository URLs).
5. **Output directory** — where to write dry-run task files (default: `./jira-tasks-draft/`)
6. **Task type filter** (optional) — if user only wants certain task types (e.g., `onboard-unit,fix-codecov`)
7. **Repo filter** (optional) — if user only wants specific repos. User may provide:
   - Repo names: `quay-operator, mirror-registry`
   - GitHub URLs: `https://github.com/quay/quay-operator`
   - Extract the repo name from URLs (last path segment) and pass to `--repos`

### Step 1b: Check Jira Credentials

Before running anything, verify credentials are set:
```
echo "USERNAME: ${JIRA_USERNAME:-(not set)}" && echo "TOKEN: ${JIRA_API_TOKEN:+set (hidden)}"
```

If either is missing, walk the user through setup:

1. **JIRA_USERNAME** — their email for the Atlassian account (e.g., `user@redhat.com`)
2. **JIRA_API_TOKEN** — generated at https://id.atlassian.com/manage-profile/security/api-tokens
   - Click "Create API token"
   - Give it a label (e.g., "coverage-tasks")
   - Copy the token

Tell the user to run these in the terminal (suggest `! export ...` in the prompt):
```
export JIRA_USERNAME="user@redhat.com"
export JIRA_API_TOKEN="the-token-they-copied"
```

**Important:** If the user pastes their API token in the chat, warn them to rotate it afterward — tokens in conversation history are a security risk.

Do NOT block the dry-run on credentials — they're only needed for Phase 2 (create-tasks). But check early so the user isn't surprised later.

### Step 2: Preview Classification

#### If CSV has an `Onboard` column (from `coverage-audit` skill)

The user already curated which repos to onboard in their spreadsheet. The `dry-run.py`
script automatically filters to rows where `Onboard=TRUE`.

1. Read the CSV and count how many repos have `Onboard=TRUE`
2. Show a summary: "CSV has Onboard column — N repos selected for onboarding"
3. Show task type breakdown for those repos
4. Ask user to confirm before proceeding to dry-run

The `--repos` flag overrides the Onboard column if the user wants to narrow further.

#### If CSV has NO `Onboard` column (manual CSV or older audit)

Fall back to the interactive selection flow:

1. Read the CSV with Python or pandas
2. Apply the same filtering logic as dry-run.py (skip forks, archived, non-apps, no-code repos)
3. Present a summary table grouped by task type, showing repo names, languages, and proposed priorities
4. Explicitly ask: **"Which of these repos should I generate tasks for?"**

The user might respond with:
- Specific repos: "just quay-operator and mirror-registry"
- By task type: "only the onboard-unit ones"
- By wave: "just Critical and Major priority"
- Exclusions: "all of them except the investigate ones"
- All: "go ahead with all of them"

Map their answer to `--repos` and/or `--types` flags for the dry-run script.

**Do NOT run dry-run on the full CSV without user confirmation of which repos to include.** The whole point is to avoid creating 40+ Jira tasks that nobody asked for.

### Step 3: Run Dry Run

Execute the dry-run script yourself:
```
python <skill-dir>/scripts/dry-run.py \
  --csv <csv-path> \
  --org <org-name> \
  --output-dir <output-dir>
```

To filter by task type:
```
python <skill-dir>/scripts/dry-run.py \
  --csv <csv-path> \
  --org <org-name> \
  --output-dir <output-dir> \
  --types onboard-unit,fix-codecov
```

To filter by specific repos:
```
python <skill-dir>/scripts/dry-run.py \
  --csv <csv-path> \
  --org <org-name> \
  --output-dir <output-dir> \
  --repos quay-operator,mirror-registry
```

Valid `--types` values: `fix-codecov`, `verify-codecov`, `onboard-unit`, `investigate`, `needs-tests`, `needs-ci`, `onboard-e2e`

Both `--types` and `--repos` can be combined.

Then show the user:
- Total parent tasks, subtasks, and DevLake tasks that would be created
- Breakdown by subtask type
- Execution wave summary (Critical → High → Medium → Low)
- Any repos that seem misclassified (flag high-star repos with low priority, etc.)

### Step 4: User Review

Ask user to confirm. Tell them they can:
- Ask you to remove specific repos/tasks
- Ask you to change priorities
- Ask you to edit descriptions
- Ask you to filter to specific task types only

Apply any requested changes by deleting or editing files in the output directory.

**Handling large task lists:** If the dry run generates more than 30 tasks, proactively suggest the user review by priority wave. Recommend starting with Critical and High priority tasks, and deciding whether Low priority tasks (especially "investigate" and "needs-tests") are worth creating as Jira issues or should be tracked differently.

### Step 5: Create Tasks

Only after user explicitly confirms, execute:
```
python <skill-dir>/scripts/create-tasks.py \
  --input-dir <output-dir> \
  --epic <epic-key> \
  --project <project-key>
```

For non-default Jira instances:
```
python <skill-dir>/scripts/create-tasks.py \
  --input-dir <output-dir> \
  --epic <epic-key> \
  --project <project-key> \
  --jira-url https://your-instance.atlassian.net
```

The script auto-detects the correct subtask issue type (`Subtask` vs `Sub-task`) by querying the project's issue types before creating. Override with `--subtask-type "Sub-task"` if auto-detection fails.

Show user the created issue keys and URLs when done. The output clearly distinguishes parent tasks, subtasks, and DevLake tasks.

## Task Structure

Each repo that passes filtering gets **one parent Task** with **one or more Subtasks** (one per test type). A repo can have both `onboard-unit` and `onboard-e2e` subtasks if it has both unit tests and e2e tests.

Additionally, **two DevLake follow-up Tasks** are created directly under the epic (not per repo):
1. "Set up DevLake project for code coverage tracking" — create DevLake project with GitHub + Codecov connections
2. "Add team to metrics dashboard (metrics.dprod.io)" — submit MR to add team to the unified dashboard

## Subtask Type Classification

The script uses a decision tree to classify repos into subtask types. The order matters — repos are filtered first, then classified by their coverage readiness.

### Filtering (repos skipped entirely)

| Condition | Skip Reason |
|-----------|-------------|
| Category != `application` | Non-application repo (infra, sample, etc.) |
| Fork = `yes` | Fork — coverage is upstream's responsibility |
| Archived = `yes` | Archived — no longer maintained |
| No language AND no CI detected | Config/docs repo — no testable code |

### Classification (repos that get subtasks)

| Condition | Subtask Type | Base Priority | Label |
|-----------|-----------|---------------|-------|
| Has Codecov = `yes`, flags NOT configured | Fix Codecov config | Critical | `fix-codecov` |
| Has Codecov = `yes`, flags configured | Verify Codecov setup | Major | `verify-codecov` |
| Has Codecov = `config-only` | Fix Codecov config | Critical | `fix-codecov` |
| Has tests = `yes`/`likely`, has CI | Onboard unit tests → Codecov | Normal | `onboard-unit` |
| Has tests = `yes`/`likely`, NO CI | Set up CI first | Normal | `needs-ci` |
| Has tests = `yes` but tests are placeholder | Add real tests | Minor | `needs-tests` |
| Tests = `unknown`, has CI + language | Add tests then onboard | Minor | `needs-tests` |
| Tests = `unknown`, has language, no CI | Investigate | Minor | `investigate` |
| Has E2E = `yes`, language in Go/Python/JS/TS | Onboard e2e → Coverport | Minor | `e2e-tests` |

### Priority Boost by Star Count

High-star repos get priority bumped regardless of task type:
- **1000+ stars** → Critical
- **100+ stars** → at least Major
- **30+ stars** → at least Normal

This ensures important projects like `clair` (10k+ stars) get attention even when test detection is imperfect.

All tasks also get the `codecov-onboarding` label.

### False Positive Detection

The script checks `Test Details` for known false positives like `echo "Error: no test specified"`. Repos where tests are detected but actually broken get `needs-tests` instead of `onboard-unit`.

## Output Structure

```
<output-dir>/
  <repo-slug>/
    task.md              (parent task — overview, subtask list, AI skill reference)
    subtask-<type>.md    (one per test type — detailed steps, verification)
  _devlake-setup.md      (DevLake project setup — directly under epic)
  _devlake-dashboard.md  (Metrics dashboard onboarding — directly under epic)
```

### Parent task file (`task.md`)
- YAML frontmatter: summary, priority, type=Task, labels
- Objective with repo link and star count
- Current State table (tests, codecov, CI, language)
- Key contacts (top contributors from CSV)
- Subtask list summary
- AI-assisted implementation reference

### Subtask file (`subtask-<type>.md`)
- YAML frontmatter: summary, priority, type=Subtask, labels (type-specific)
- Objective
- Current State table
- Language and CI-specific implementation steps
- Manual steps required
- Verification checklist

### DevLake files (`_devlake-*.md`)
- YAML frontmatter: summary, priority, type=Task, labels
- Detailed step-by-step instructions (from COVERPORT-111 / COVERPORT-112 templates)
- Prerequisites, troubleshooting, verification checklist

## Script Reference

### scripts/dry-run.py

```
Usage: python scripts/dry-run.py \
  --csv <audit.csv> \
  --org <github-org-name> \
  --output-dir <./jira-tasks-draft/> \
  [--types <comma-separated-task-types>] \
  [--repos <comma-separated-repo-names>] \
  [--no-devlake]

Output: directory structure with task.md + subtask-*.md per repo,
        _devlake-*.md at root, + summary to stdout
```

### scripts/create-tasks.py

```
Usage: python scripts/create-tasks.py \
  --input-dir <./jira-tasks-draft/> \
  --epic <EPIC-KEY> \
  --project <PROJECT-KEY> \
  [--jira-url <https://your-instance.atlassian.net>] \
  [--subtask-type <Subtask>]

Env vars required: JIRA_USERNAME, JIRA_API_TOKEN
Optional env var: JIRA_URL (alternative to --jira-url flag)
Output: created issue keys + URLs to stdout (parent tasks, subtasks, DevLake tasks)
```

## Important Notes

### Jira Formatting

Task files are written in markdown for human readability. The `create-tasks.py` script automatically converts markdown to Jira wiki markup before sending to the API. The task `.md` files in the output directory remain in markdown.

### Subtask Issue Type

Different Jira projects use different names for the subtask issue type: `Subtask`, `Sub-task`, or custom names. The script auto-detects the correct name by querying `GET /rest/api/2/project/<KEY>/statuses` before creating issues. Override with `--subtask-type` if needed.

### Idempotent Retries

The script saves a `.created-issues.json` log in the input directory, tracking which tasks have been created (keyed by `task:<repo>`, `subtask:<repo>:<file>`, `devlake:<file>`). On re-run, already-created issues are skipped — only failed items are retried. This prevents duplicates when a run partially fails (e.g., subtask type wrong but parent tasks succeed). Delete `.created-issues.json` to force a full re-creation.

### Priority Compatibility

The script uses Jira priorities: Critical, Major, Normal, Minor. "Low" is intentionally avoided because some Jira project schemes don't include it. If a project uses a different priority scheme, edit the task files' frontmatter before running `create-tasks.py`.

### DevLake Tasks

The two DevLake tasks are generated from templates based on [COVERPORT-111](https://redhat.atlassian.net/browse/COVERPORT-111) and [COVERPORT-112](https://redhat.atlassian.net/browse/COVERPORT-112). They are follow-up tasks meant to be done after Codecov onboarding is complete. Use `--no-devlake` flag in dry-run if these tasks already exist or are not needed.

### SSL Certificates

On macOS, Python may fail with SSL certificate errors. The script uses the `certifi` package if available. If you get SSL errors, install it: `pip install certifi`.
