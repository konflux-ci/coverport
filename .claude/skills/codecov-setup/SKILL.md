---
name: codecov-setup
description: >-
  Automated, non-interactive Codecov onboarding driven by an audit CSV. Reads
  pre-collected audit data and opens PRs/MRs with zero Q&A — across one repo or
  hundreds in parallel. Supports a two-phase rollout: prepare (disabled CI job,
  safe to merge now) and enable (activates once instance is live). Use when you
  have an audit CSV or need bulk processing. Triggers on: "setup codecov for all
  repos", "bulk codecov", "prepare codecov PRs", "enable codecov", "codecov from
  audit", "onboard repos to codecov", "open prepare MRs", "enable coverage".
---

# Codecov Setup Skill

Non-interactive, CSV-driven Codecov onboarding. Reads an audit CSV (from
`coverage-audit`) and opens one PR/MR per repo automatically — no Q&A required.
Two-phase rollout: **prepare** (disabled CI job + coverage flags + `.codecov.yml`,
zero CI impact) then **enable** (removes disable guard, sets real instance URL).

Do not use for interactive single-repo guidance (`codecov-onboarding`) or E2E
container instrumentation (`coverport-integration`).

## Prerequisites

Before executing any steps, read these files in order:

1. `codecov-config/CONFIG.md` — platform detection and Codecov instance URL routing
2. `codecov-onboarding/SKILL.md` — GitLab CI job template (Option C) and GitHub Actions
   step template (Option A); read at runtime, do not copy here
3. `add-codecov-yml/skill.md` — `.codecov.yml` template, compliance rules, and
   platform-specific PR (GitHub via `gh`) and MR (GitLab via `glab`) creation steps

These paths are relative to the skills directory (`.claude/skills/` or `.cursor/skills/` — both resolve to the same location).

## Instructions

### Modes and Targeting

**Modes:**
- `prepare local` — clone each repo, apply all changes (coverage flags + disabled upload
  job + `.codecov.yml`), commit locally, print a `git show HEAD` diff per repo — then stop.
  No push, no MR. Use to review the exact changes before opening any PRs.
- `prepare` — full prepare workflow: same as `prepare local` plus push and open MR/PR.
- `enable` — removes disable guard, fills real instance URL from `codecov-config/CONFIG.md`
- `full` (default) — fully enabled job + `.codecov.yml`; instance must be live

**Targeting:**
- `--target <repo-url>` — single-repo mode; execute steps directly in this session
- `--csv <path>` — bulk mode; read CSV, dispatch one subagent per repo in parallel

Both `prepare local` and `prepare` use the multi-subagent dispatch approach in bulk mode —
one subagent per repo, all in parallel.

**Fast dry run** — if the user says "dry run", "preview", or "what would change": infer
changes from the CSV data alone (no cloning). Shows which repos would be touched and what
flags/jobs would be added based on the declared language and CI system. Fast, zero network.
No cloning.

### How to Invoke This Skill

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
| "Show me what would change for these repos without opening any PRs" | fast dry run | CSV or single |
| "Commit changes locally but don't push yet — I want to review the diffs first" | prepare local | CSV or single |
| "prepare local for audit.csv" or "show me the exact diff before opening MRs" | prepare local | CSV or single |

If the user doesn't mention a CSV path, ask for it before proceeding. If mode is
ambiguous, ask whether the Codecov instance is available yet (prepare vs full).

### CSV Format

Produced by `coverage-audit`. Required columns:

| Column | Description |
|---|---|
| `Repo URL` | Full HTTPS URL to the repository |
| `Onboard` | `TRUE` to include this repo |
| `Language` | Primary language: Go, Python, JavaScript, TypeScript, C, C++ |
| `CI System` | `gitlab-ci` or `github-actions` |
| `Has Codecov` | `TRUE` if Codecov is already configured — skip these rows |

Process only rows where `Onboard=TRUE` AND `Has Codecov` is not `TRUE`.

### Coverage Flag Detection

Find the existing test command in the CI config file and inject coverage flags.
If no test command is found for the repo's language, insert a `# TODO` comment and
add the repo to the manual-attention list in the session summary.

| Language | Flags to append to existing test command | Coverage output file |
|---|---|---|
| Go | `-coverprofile=coverage.out -covermode=atomic` appended to `go test` | `coverage.out` |
| Python | `--cov=<package> --cov-report=xml:coverage.xml` appended to `pytest` (see package detection below) | `coverage.xml` |
| JavaScript | `--coverage` appended to `jest` or `vitest` | `coverage/lcov.info` |
| TypeScript | `--coverage` appended to `jest` or `vitest` | `coverage/lcov.info` |
| C/C++ | Delegate entirely to `c-cpp-coverage` skill for the lcov pipeline | `coverage.info` |
| Other | Insert `# TODO: add coverage flags for <language>` comment near test step | — |

**Injection rule:** Insert coverage flags immediately before any trailing package or path
argument (e.g., `./...`, `./pkg/...`, a directory like `tests/`). For test runners where
no such trailing argument exists, append at the end. Never replace any part of the command.

> **Tox repos:** If the CI job runs `tox -e <env>` rather than pytest directly, apply
> changes to `tox.ini` instead of `.gitlab-ci.yml`. Only add
> `--cov-report=xml:coverage.xml` if XML output is absent — **do not change any existing
> `--cov=` target or tox substitution** (`{toxinidir}`, `{[vars]MODULE}`, etc.). Those
> were set intentionally by the maintainer and must be left untouched.

**Python package detection (for `--cov=<package>`):** Determine the package name in this order:
1. If the existing `pytest` command already contains `--cov=<something>`, extract that value and reuse it (do not duplicate).
2. Look for a top-level directory containing an `__init__.py` file — that is the package name (e.g. `src/mypackage/__init__.py` → `--cov=mypackage`).
3. Check `setup.py`, `pyproject.toml`, or `setup.cfg` for the declared package name.
4. If none of the above yields a clear answer, use `--cov=. --cov-report=xml:coverage.xml` and insert a `# TODO: replace . with the actual package name` comment on the same line; add the repo to the manual-attention list.

Example (Go — flags inserted before trailing `./...`):
```
Before: go test ./...
After:  go test -coverprofile=coverage.out -covermode=atomic ./...
```

Example (Python — flags inserted before trailing `tests/`):
```
Before: pytest tests/
After:  pytest --cov=myservice --cov-report=xml:coverage.xml tests/
```

If coverage flags are already present in the command, skip this step and note it in the summary.

### CI Job Modifiers

These modifiers are applied on top of the upload job templates read from `codecov-onboarding`.

#### GitLab CI — Prepare Modifier

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

#### GitLab CI — Enable Modifier

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

#### GitHub Actions — Prepare Modifier

Read the upload step template from `codecov-onboarding` Option A (self-hosted/token auth
variant). This is a step that gets added to the **existing primary test workflow file** —
do not create a new workflow file. Then apply:

1. Add `if: false` directly on the upload step, and set `url: PLACEHOLDER`. Use the
   auth method from the template read from `codecov-onboarding` Option A — do not
   add or change auth fields:
   ```yaml
       - name: Upload coverage to Codecov
         if: false  # DISABLED — remove this line when Codecov instance is ready
         uses: codecov/codecov-action@v5
         with:
           url: PLACEHOLDER
           # auth fields carried over from Option A template unchanged
           flags: unit-tests
           files: <coverage-file-path>
           fail_ci_if_error: false
   ```

The step-level `if: false` makes only this step inert — the rest of the workflow is unchanged.

#### GitHub Actions — Enable Modifier

1. Read the real Codecov instance URL from `codecov-config/CONFIG.md`.
2. Replace `url: PLACEHOLDER` with `url: <real-instance-url>`.
3. Remove the `if: false` line from the upload step.

### Prepare Local Workflow

Clones every target repo, applies all prepare-mode changes, commits locally, prints a
diff — then stops. No push, no MR.

In bulk mode, dispatch one subagent per repo simultaneously (same approach as `prepare`).

1. **Parse CSV / identify target** (filter `Onboard=TRUE`, `Has Codecov ≠ TRUE`).
2. **Announce:** "Prepare local — found N repos. Dispatching N subagents in parallel. No MRs will be opened."
3. Execute **Prepare Mode Workflow steps 1–11** for each repo. Step 2 is a plain
   `git clone` — no `GITLAB_TOKEN` needed. Stop after commit; skip step 12 (push/MR).
4. After committing, print the per-repo diff:
   ```bash
   git show HEAD --stat --patch
   ```
5. Print the session summary with a `PREPARE LOCAL` header; replace "Opened MRs/PRs"
   with "Committed locally (not pushed)".
6. Local clones remain in `/tmp/codecov-setup/<repo-name>/`. Run `prepare` when ready
   to push (it re-clones fresh).

Manual-attention repos are listed in the summary as in a real `prepare` run.

### Prepare Mode Workflow

Execute these steps for each target repo (directly in single-repo mode; each subagent
runs this workflow independently in bulk mode):

1. **Idempotency check:** Search for an open MR/PR with branch name `add-codecov-config`.
   If one exists, skip this repo and add it to the "already prepared" list in the summary.
2. **Clone the repo:**
   ```bash
   git clone <repo-url> /tmp/codecov-setup/<repo-name>
   cd /tmp/codecov-setup/<repo-name>
   ```
   No token needed to clone. `GITLAB_TOKEN` is only required at step 12 (push/MR).
   `prepare local` stops at step 11 and never needs a token.
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
7. **Read upload template** from `codecov-onboarding` SKILL.md:
   - GitLab → Option C (a new job block appended to `.gitlab-ci.yml`)
   - GitHub → Option A (a new step added to the existing primary test workflow file)
8. **Apply the GitLab or GitHub prepare modifier** (see CI Job Modifiers above).
9. **Write the change** to the CI config:
   - GitLab: append the modified job block to `.gitlab-ci.yml`
   - GitHub: add the modified upload step to the existing primary test workflow file
     (`.github/workflows/<test-workflow-name>.yml`) — do not create a new workflow file
10. **Handle `.codecov.yml`** using the template from `add-codecov-yml/skill.md`:
    - File absent → generate from template, write to repo root
    - File present and compliant → skip
    - File present but non-compliant → fix in-place and include in this PR
11. **Commit:**
    ```bash
    git add -A
    git commit -m "chore: add codecov setup (disabled, pending internal instance)"
    ```
12. **Check push access, then push and open MR/PR.**
    This is the first step that requires credentials (`GITLAB_TOKEN` for GitLab,
    `gh` auth for GitHub).

    **Push access check (do this before `git push`):**
    - **GitHub:** `gh api repos/<org>/<repo>` — check `"permissions": {"push": true}`.
      If `false`, fork first:
      ```bash
      gh repo fork <org>/<repo> --clone --remote-name upstream
      # origin = your fork, upstream = original; PR will target upstream default branch
      ```
    - **GitLab:** Check access level via the REST API:
      ```bash
      curl -s --header "PRIVATE-TOKEN: $GITLAB_TOKEN" \
        "https://<gitlab-host>/api/v4/projects/<org>%2F<repo>" \
        | python3 -c "import json,sys; d=json.load(sys.stdin); \
            print(d.get('permissions',{}).get('project_access',{}).get('access_level', 0))"
      ```
      If below 30 (Developer), fork via API then add upstream remote:
      ```bash
      FORK_URL=$(curl -s -X POST --header "PRIVATE-TOKEN: $GITLAB_TOKEN" \
        "https://<gitlab-host>/api/v4/projects/<org>%2F<repo>/fork" \
        | python3 -c "import json,sys; print(json.load(sys.stdin).get('http_url_to_repo',''))")
      git remote set-url origin "$FORK_URL"
      git remote add upstream <original-repo-url>
      ```
    - **If `$GITLAB_TOKEN` is not set:** attempt `git push` directly; if it fails
      with a permission error, add the repo to the Needs Manual Attention list with
      reason "push access denied — set GITLAB_TOKEN and re-run."

    **Push and open MR/PR** using the platform-specific step from `add-codecov-yml/skill.md § 4`:
    ```bash
    git push -u origin add-codecov-config
    ```
    - GitHub repos: use `gh pr create`
    - GitLab repos: use `curl` to create the MR via the GitLab REST API
    - MR/PR title: `chore: add Codecov coverage config (disabled — pending internal instance)`
    - MR/PR body: see PR Description Template section below
13. **Record** the MR/PR URL in the session summary.

### Enable Mode Workflow

1. **Read instance URL** from `codecov-config/CONFIG.md`. If the URL is still `PLACEHOLDER`,
   stop and report: "Instance URL is not set in codecov-config/CONFIG.md — cannot run enable mode."
2. **Idempotency check:** Search for an open MR/PR with branch `enable-codecov-coverage`.
   If found, skip and add to the "already enabled" list.
3. **Clone** the repo:
   ```bash
   git clone <repo-url> /tmp/codecov-setup/<repo-name>
   cd /tmp/codecov-setup/<repo-name>
   ```
   No token needed to clone. Push access check is deferred to step 8.
4. **Verify** the disabled upload step/job is present in the default branch:
   - GitLab: look for a job block containing `when: never`
   - GitHub: look for `if: false` on a step named `Upload coverage to Codecov` in the primary test workflow
   If not found, skip this repo and add it to the **Needs Manual Attention** list with reason
   "prepare change not found in default branch — was the prepare MR/PR merged?"
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

### Full Mode Workflow

1. **Idempotency check:** Search for an open MR/PR with branch `add-codecov-coverage`.
   If one exists, skip this repo and add it to the "already set up" list in the summary.
2. **Read instance URL** from `codecov-config/CONFIG.md`. If the URL is still `PLACEHOLDER`,
   stop and report: "Instance URL is not set in codecov-config/CONFIG.md — cannot run full mode."
3. **Execute Prepare Mode steps 2–11** (clone → branch → coverage flags → template → enable
   modifier → `.codecov.yml` → commit), with these differences:
   - Branch name: `add-codecov-coverage`
   - In step 8, apply the **enable modifier** (not the prepare modifier) using the real URL —
     the job is active immediately; no second PR is needed.
4. **Push and open MR/PR:**
   - Title: `feat: add Codecov coverage reporting`
   - Body: "Adds fully enabled Codecov coverage upload. Coverage uploads begin on next
     pipeline run after merge."
5. **Record** the MR/PR URL in the session summary.

### Bulk Dispatch (CSV Mode)

Used by `prepare local`, `prepare`, `enable`, and `full` modes when a CSV is provided.
All modes dispatch one subagent per repo in parallel.

1. **Parse CSV.** Filter to rows where `Onboard=TRUE` AND `Has Codecov` ≠ `TRUE`.
2. **Check progress file:** If `.codecov-setup-progress.json` exists in the current working
   directory, skip any repo already recorded under the same mode.
3. **Announce:** "Found N repos to process. Dispatching N subagents in parallel."
4. **Dispatch one subagent per repo simultaneously** (all in a single turn). Each subagent
   receives:
   - Repo URL, language, CI system (from the CSV row)
   - Mode (`prepare local` / `prepare` / `enable` / `full`)
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

### Idempotency

| Situation | Action |
|---|---|
| Open MR/PR on branch `add-codecov-config` already exists | Skip prepare; add to "already prepared" list |
| Open MR/PR on branch `enable-codecov-coverage` already exists | Skip enable; add to "already enabled" list |
| Open MR/PR on branch `add-codecov-coverage` already exists | Skip full; add to "already set up" list |
| Repo recorded in `.codecov-setup-progress.json` under the same mode | Skip in bulk mode before dispatching (applies to prepare, enable, and full) |

Never open a duplicate MR/PR. Always report skips in the summary.

### PR Description Template (Prepare Mode)

Use the platform-appropriate body. Include only the matching section.

**GitLab MR body:**
```markdown
## Codecov Setup (Disabled — Pending Internal Instance)

The upload job is **disabled** (`when: never`) — zero CI impact until the enable MR is merged.

**Added:** coverage flags in test command · `codecov-upload` job (disabled) · `.codecov.yml`

**Before enable MR:** add `CODECOV_TOKEN` as a masked CI/CD variable. Token auth is
required for GitLab CI (OIDC unavailable). See `codecov-config/CONFIG.md` for the value.

**Next:** a follow-up MR will remove the disable guard and set the instance URL.
```

**GitHub PR body:**
```markdown
## Codecov Setup (Disabled — Pending Internal Instance)

The upload step is **disabled** (`if: false`) — zero CI impact until the enable PR is merged.

**Added:** coverage flags in test command · `Upload coverage to Codecov` step (disabled) · `.codecov.yml`

**Auth:** confirm the repo is configured with the Codecov instance per `codecov-config/CONFIG.md`.

**Next:** a follow-up PR will remove `if: false` and set the instance URL.
```

### Session Summary Format

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
