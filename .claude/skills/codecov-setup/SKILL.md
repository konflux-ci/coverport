---
name: codecov-setup
description: Non-interactive, data-driven Codecov onboarding for one or many repos. Use
  when you need to open prepare or enable PRs/MRs for Codecov integration across multiple
  GitLab or GitHub repositories using an audit CSV, or for a single repo without interactive
  Q&A. Handles coverage generation flags, disabled upload job (prepare mode), and activation
  (enable mode). Use instead of codecov-onboarding when you have an audit CSV or need bulk
  processing. Triggers on: "setup codecov for all repos", "bulk codecov", "prepare codecov
  PRs", "enable codecov", "codecov setup from audit", "onboard repos to codecov".
---

# codecov-setup

Non-interactive, data-driven version of `codecov-onboarding`. Uses audit CSV data to
pre-answer every question `codecov-onboarding` would ask interactively, producing a
complete, ready-to-merge PR per repo.

## When to Use

| Situation | Command |
|---|---|
| Many repos, Codecov instance not yet available | `codecov-setup --csv audit.csv --mode prepare` |
| Many repos, instance already running | `codecov-setup --csv audit.csv` |
| Activate all previously prepared repos | `codecov-setup --csv audit.csv --mode enable` |
| Single repo, instance not yet available | `codecov-setup --target <repo-url> --mode prepare` |
| Single repo, wants interactive Q&A guidance | Use `codecov-onboarding` instead |
| E2e container instrumentation | Use `coverport-integration` instead |

## Prerequisites — Read These First

Before executing any steps in this skill, read these files in order:

1. `codecov-config/CONFIG.md` — platform detection and Codecov instance URL routing
2. `codecov-onboarding/SKILL.md` — GitLab CI job template (Option C) and GitHub Actions
   job template (Option A); read these templates at runtime, do not copy them here
3. `add-codecov-yml/skill.md` — `codecov.yml` template, compliance rules, and PR/MR
   creation steps for both GitLab and GitHub

These paths are relative to the coverport repo root. Locate the coverport repo from context
or ask the user if the path is unclear.

## Interface

**Modes:**
- `prepare` — adds disabled upload job + coverage flags + `codecov.yml`; no instance URL needed
- `enable` — removes disable guard, fills real instance URL from `codecov-config/CONFIG.md`
- `full` (default) — fully enabled job + `codecov.yml`; instance must be live

**Targeting:**
- `--target <repo-url>` — single-repo mode; execute steps directly in this session
- `--csv <path>` — bulk mode; read CSV, dispatch one subagent per repo in parallel

**Dry run:** If the user says "dry run" or "preview", print what would change per repo
without cloning or opening any PRs/MRs.

## How to Invoke This Skill

This is a natural language skill — users describe what they want, not CLI commands.
Recognize the user's intent from their message and map it to the appropriate mode and
targeting. Examples of real user prompts and how to interpret them:

| User says | Mode | Targeting |
|---|---|---|
| "Open prepare PRs for all repos in ~/Downloads/audit.csv" | prepare | CSV |
| "We ran the coverage audit — can you set up all the Onboard=TRUE repos in disabled state?" | prepare | CSV (ask for path if not given) |
| "The Codecov instance is live, go through audit-q2.csv and enable everything" | enable | CSV |
| "Set up Codecov for https://gitlab.cee.redhat.com/myteam/myservice — instance isn't ready yet" | prepare | single repo |
| "Add Codecov to this repo, instance is already running" | full | single repo |
| "Show me what would change for these repos without actually opening any PRs" | any (dry run) | CSV or single |

If the user doesn't mention a CSV path, ask for it before proceeding. If mode is
ambiguous, ask whether the Codecov instance is available yet (prepare vs full).

## CSV Format

Produced by `coverage-audit`. Required columns:

| Column | Description |
|---|---|
| `Repo URL` | Full HTTPS URL to the repository |
| `Onboard` | `TRUE` to include this repo |
| `Language` | Primary language: Go, Python, JavaScript, TypeScript, C, C++ |
| `CI System` | `gitlab-ci` or `github-actions` |
| `Has Codecov` | `TRUE` if Codecov is already configured — skip these rows |

Process only rows where `Onboard=TRUE` AND `Has Codecov` is not `TRUE`.

## Coverage Flag Detection

Find the existing test command in the CI config file and inject coverage flags.
If no test command is found for the repo's language, insert a `# TODO` comment and
add the repo to the manual-attention list in the session summary.

| Language | Flags to append to existing test command | Coverage output file |
|---|---|---|
| Go | `-coverprofile=coverage.out -covermode=atomic` appended to `go test` | `coverage.out` |
| Python | `--cov --cov-report=xml:coverage.xml` appended to `pytest` | `coverage.xml` |
| JavaScript | `--coverage` appended to `jest` or `vitest` | `coverage/lcov.info` |
| TypeScript | `--coverage` appended to `jest` or `vitest` | `coverage/lcov.info` |
| C/C++ | Delegate entirely to `c-cpp-coverage` skill for the lcov pipeline | `coverage.info` |
| Other | Insert `# TODO: add coverage flags for <language>` comment near test step | — |

**Injection rule:** Append flags to the end of the existing command. Never replace the command.

Example (Go):
```
Before: go test ./...
After:  go test -coverprofile=coverage.out -covermode=atomic ./...
```

If coverage flags are already present in the command, skip this step and note it in the summary.

## CI Job Modifiers

These modifiers are applied on top of the upload job templates read from `codecov-onboarding`.

### GitLab CI — Prepare Modifier

Read the upload job template from `codecov-onboarding` Option C. Then apply:

1. In the job's `variables:` block, add:
   ```yaml
   CODECOV_URL: "PLACEHOLDER"
   ```
2. Append this `rules:` block to the job definition (it overrides all other rules, making the job inert):
   ```yaml
     rules:
       - when: never   # DISABLED — remove this block when Codecov instance is ready
   ```

### GitLab CI — Enable Modifier

1. Read the real Codecov instance URL from `codecov-config/CONFIG.md`.
2. Replace `CODECOV_URL: "PLACEHOLDER"` with the real URL:
   ```yaml
   CODECOV_URL: "<instance-url>"
   ```
3. Remove the entire `rules: - when: never` block.
4. Add proper trigger rules and failure tolerance:
   ```yaml
     rules:
       - if: '$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH'
       - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
     allow_failure: true
   ```

### GitHub Actions — Prepare Modifier

Read the upload job template from `codecov-onboarding` Option A. Then apply:

1. In the Codecov upload step's `with:` block, add:
   ```yaml
         url: PLACEHOLDER
   ```
2. Add `if: false` as the first key under the job name:
   ```yaml
   jobs:
     upload-coverage:
       if: false  # DISABLED — remove this line when Codecov instance is ready
   ```

### GitHub Actions — Enable Modifier

1. Read the real Codecov instance URL from `codecov-config/CONFIG.md`.
2. Replace `url: PLACEHOLDER` with `url: <real-instance-url>`.
3. Remove the `if: false` line from the job definition.

## Prepare Mode Workflow

Execute these steps for each target repo (directly in single-repo mode; each subagent
runs this workflow independently in bulk mode):

1. **Idempotency check:** Search for an open MR/PR with branch name `add-codecov-config`.
   If one exists, skip this repo and add it to the "already prepared" list in the summary.
2. **Clone** the repo:
   ```bash
   git clone <repo-url> /tmp/codecov-setup/<repo-name>
   cd /tmp/codecov-setup/<repo-name>
   ```
3. **Create branch:**
   ```bash
   git checkout -b add-codecov-config
   ```
4. **Identify CI file** from the audit CSV (`CI System` column):
   - `gitlab-ci` → `.gitlab-ci.yml`
   - `github-actions` → `.github/workflows/` (find the primary test workflow)
5. **Find test command** in the CI file by searching for the language's test runner
   (e.g., `go test`, `pytest`, `jest`, `vitest`).
6. **Inject coverage flags** per the Coverage Flag Detection table. If no command is found,
   insert the `# TODO` comment and add the repo to the manual-attention list.
7. **Read upload job template** from `codecov-onboarding` SKILL.md:
   - GitLab → Option C
   - GitHub → Option A
8. **Apply the GitLab or GitHub prepare modifier** (see CI Job Modifiers above).
9. **Write the modified job** to the CI config:
   - GitLab: append the job block to `.gitlab-ci.yml`
   - GitHub: create `.github/workflows/codecov-upload.yml` as a new standalone file
10. **Handle `codecov.yml`** using the template from `add-codecov-yml/skill.md`:
    - File absent → generate from template, write to repo root
    - File present and compliant → skip
    - File present but non-compliant → fix in-place and include in this PR
11. **Commit:**
    ```bash
    git add -A
    git commit -m "chore: add codecov setup (disabled, pending internal instance)"
    ```
12. **Push and open MR/PR** using the steps from `add-codecov-yml/skill.md` for the platform:
    - MR/PR title: `chore: add Codecov coverage config (disabled — pending internal instance)`
    - MR/PR body: see PR Description Template section below
13. **Record** the MR/PR URL in the session summary.

## Enable Mode Workflow

1. **Read instance URL** from `codecov-config/CONFIG.md`. If the URL is still `PLACEHOLDER`,
   stop and report: "Instance URL is not set in codecov-config/CONFIG.md — cannot run enable mode."
2. **Idempotency check:** Search for an open MR/PR with branch `enable-codecov-coverage`.
   If found, skip and add to the "already enabled" list.
3. **Clone** the repo to `/tmp/codecov-setup/<repo-name>`.
4. **Verify** the disabled upload job is present in the default branch:
   - GitLab: look for a job block containing `when: never`
   - GitHub: look for `if: false` in a job named `upload-coverage`
   If not found, warn "prepare job not found in default branch — was the prepare PR merged?"
   and skip.
5. **Create branch:**
   ```bash
   git checkout -b enable-codecov-coverage
   ```
6. **Apply the GitLab or GitHub enable modifier** (see CI Job Modifiers above).
7. **Commit:**
   ```bash
   git add -A
   git commit -m "chore: enable codecov upload to <instance-url>"
   ```
8. **Push and open MR/PR:**
   - Title: `feat: enable Codecov coverage reporting`
   - Body: "Activates the Codecov upload job added in the prepare PR. Coverage uploads begin
     on next pipeline run after merge."
9. **Record** the MR/PR URL in the session summary.

## Full Mode Workflow

Identical to Prepare Mode with one change: in step 8, apply the **enable modifier** instead
of the prepare modifier, using the real URL from `codecov-config/CONFIG.md`. The job is
active immediately; no second PR is needed.

Branch name: `add-codecov-coverage`
Title: `feat: add Codecov coverage reporting`

## Bulk Dispatch (CSV Mode)

1. **Parse CSV.** Filter to rows where `Onboard=TRUE` AND `Has Codecov` ≠ `TRUE`.
2. **Check progress file:** If `.codecov-setup-progress.json` exists in the current working
   directory, skip any repo already recorded under the same mode.
3. **Announce:** "Found N repos to process. Dispatching N subagents in parallel."
4. **Dispatch one subagent per repo simultaneously** (all in a single turn). Each subagent
   receives:
   - Repo URL, language, CI system (from the CSV row)
   - Mode (prepare / enable / full)
   - Instance URL pre-read from `codecov-config/CONFIG.md` (for enable/full modes)
   - Instructions to execute this skill's single-repo workflow for that one repo only
   - Working directory: `/tmp/codecov-setup/<repo-name>/`
5. **Wait** for all subagents to complete.
6. **Write results** to `.codecov-setup-progress.json` in the current directory:
   ```json
   {
     "mode": "prepare",
     "timestamp": "<ISO-8601>",
     "results": [
       {"repo": "<url>", "status": "opened", "pr_url": "<url>"},
       {"repo": "<url>", "status": "skipped", "reason": "already prepared"}
     ]
   }
   ```
7. **Print the session summary** (see Session Summary Format below).

## Idempotency

| Situation | Action |
|---|---|
| Open MR/PR on branch `add-codecov-config` already exists | Skip prepare; add to "already prepared" list |
| Open MR/PR on branch `enable-codecov-coverage` already exists | Skip enable; add to "already enabled" list |
| Repo recorded in `.codecov-setup-progress.json` for same mode | Skip in bulk mode before dispatching |

Never open a duplicate MR/PR. Always report skips in the summary.

## PR Description Template (Prepare Mode)

Use the platform-appropriate body below. Include only the token step matching the repo's
platform — do not include both.

**GitLab MR body:**

```markdown
## Codecov Setup (Disabled — Pending Internal Instance)

This MR adds Codecov coverage infrastructure. The upload job is **disabled** and will not
affect current CI pipelines until the enable MR is merged.

### What was added
- Coverage generation flags added to the existing test command in CI
- `codecov-upload` job added (disabled via `when: never`)
- `codecov.yml` configuration file

### One manual step required
Set `CODECOV_TOKEN` as a masked CI/CD variable before the enable MR is merged:
**Settings → CI/CD → Variables → add `CODECOV_TOKEN` (masked, protected)**

Obtain the token from the internal Codecov instance once it is available.

### What happens next
A follow-up MR will remove the disable guard and set the instance URL once the internal
Codecov instance is ready. No further changes to this repo will be needed at that point.
```

**GitHub PR body:**

```markdown
## Codecov Setup (Disabled — Pending Internal Instance)

This PR adds Codecov coverage infrastructure. The upload job is **disabled** and will not
affect current CI pipelines until the enable PR is merged.

### What was added
- Coverage generation flags added to the existing test command in CI
- `codecov-upload` job added (disabled via `if: false`)
- `codecov.yml` configuration file

### One manual step required
Set `CODECOV_TOKEN` as a repository secret before the enable PR is merged:
**Settings → Secrets → Actions → add `CODECOV_TOKEN`**

Obtain the token from the internal Codecov instance once it is available.

### What happens next
A follow-up PR will remove the disable guard and set the instance URL once the internal
Codecov instance is ready. No further changes to this repo will be needed at that point.
```

## Session Summary Format

After processing all repos, print this summary:

```
## codecov-setup Summary

Mode: <prepare|enable|full> | Repos processed: N | Opened: X | Skipped: Y | Failed: Z

### Opened MRs/PRs
| Repo | MR/PR URL |
|------|-----------|
| <repo-url> | <mr-url> |

### Skipped
| Repo | Reason |
|------|--------|
| <repo-url> | already prepared — open MR exists |

### Needs Manual Attention
| Repo | Issue |
|------|-------|
| <repo-url> | no test command found — add coverage flags manually |
| <repo-url> | C/C++ — follow c-cpp-coverage skill for lcov pipeline setup |
| <repo-url> | unknown language (<lang>) — add coverage flags manually |
```
