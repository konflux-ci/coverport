---
name: codecov-setup
description: >-
  Use when bulk-onboarding multiple repos to Codecov from an audit CSV,
  or when user says "bulk codecov", "prepare codecov PRs", "enable codecov",
  "codecov from audit", "onboard repos to codecov", "open prepare MRs",
  "enable coverage for all repos", "prepare local for audit.csv". Not for
  single-repo interactive onboarding (codecov-onboarding) or E2E
  instrumentation (coverport-integration).
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
   platform-specific PR (GitHub via `gh`) and MR (GitLab via `curl`) creation steps

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

All bulk modes (`prepare local`, `prepare`, `enable`, `full`) use the batched multi-subagent
dispatch approach — one subagent per repo, in parallel waves of up to `batch_size` (default:
15). Pass `--batch-size <N>` to override.

**Fast dry run** — if the user says "dry run", "preview", or "what would change": infer
changes from the CSV data alone (no cloning). Shows which repos would be touched and what
flags/jobs would be added based on the declared language and CI system. Fast, zero network.

### How to Invoke This Skill

Recognize the user's intent and map it to the appropriate mode and targeting:

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
| `URL` | Full HTTPS URL to the repository |
| `Onboard` | `TRUE` to include this repo |
| `Language` | Primary language: Go, Python, JavaScript, TypeScript, C, C++ |
| `CI System` | `gitlab-ci` or `github-actions` |
| `Has Codecov` | `TRUE` if Codecov is already configured — skip these rows |

Process only rows where `Onboard=TRUE` AND `Has Codecov` is not `TRUE`.

### Coverage Flag Detection

Find the existing test command in the CI config file and inject coverage flags.
If no test command is found, insert a `# TODO` comment and add the repo to the
manual-attention list.

| Language | Action | Coverage output file |
|---|---|---|
| Go | Append `-coverprofile=coverage.out -covermode=atomic` to `go test` | `coverage.out` |
| Python (pytest) | Append `--cov=<package> --cov-report=xml:coverage.xml` to `pytest` — see Python notes | `coverage.xml` |
| Python (unittest) | Replace `python -m unittest <args>` with three `coverage.py` lines — see Python notes; do **not** migrate to pytest | `coverage.xml` |
| JavaScript | Append `--coverage` to `jest` or `vitest` | `coverage/lcov.info` |
| TypeScript (jest/vitest) | Append `--coverage` to `jest` or `vitest` | `coverage/lcov.info` |
| TypeScript (Angular/Karma) | Append `--code-coverage` to `ng test --no-watch`; lcov at `coverage/<project-name>/lcov.info` — flag for manual attention: "verify headless Chrome configured (`--browsers ChromeHeadless` or `karma.conf.js`) before enabling" | `coverage/<project-name>/lcov.info` |
| C/C++ | Delegate entirely to `c-cpp-coverage` skill | `coverage.info` |
| Other | Insert `# TODO: add coverage flags for <language>` near test step | — |

**General injection rule:** Insert flags immediately before any trailing package/path argument
(`./...`, `./pkg/...`, `tests/`). If no trailing argument exists, append at the end. Never
replace any part of the existing command. If flags are already present, skip and note in summary.

#### Python Notes

**pytest — package detection (for `--cov=<package>`):**
Applies **only when pytest is called directly** (not via tox — see tox section below).
Determine the package name in this order:
1. Existing `--cov=<something>` in the pytest command → reuse it, do not duplicate.
2. Top-level directory containing `__init__.py` → use that name (e.g. `src/mypackage/__init__.py` → `--cov=mypackage`).
3. `setup.py`, `pyproject.toml`, or `setup.cfg` declared package name.
4. None found → use `--cov=. --cov-report=xml:coverage.xml` with `# TODO: replace . with the actual package name` on the same line; add to manual-attention list.

**pytest — `pytest-cov` availability:** Verify before injecting `--cov` flags:
- `pip install -r requirements*.txt` install → check if `pytest-cov` is in the file; if absent, add `- pip install pytest-cov` as a separate line immediately before the pytest command.
- Explicit `pip install` line → append `pytest-cov` to it.
- Tox → add `pytest-cov` to the `deps` list in the relevant `[testenv]` section.

**unittest runner:** `coverage.py` wraps `unittest` directly — no pytest migration needed.
Replace the existing `python -m unittest <args>` line with:
```yaml
- pip install coverage
- coverage run -m unittest <args>   # preserve original args exactly
- coverage xml -o coverage.xml
```

> **⚠ Tox repos — STOP AND READ:** If the CI job runs `tox -e <env>` rather than pytest
> directly, apply changes to `tox.ini` instead of `.gitlab-ci.yml`.
>
> **Allowed — add ONLY if XML report format is absent:**
> - `--cov-report=xml:{toxinidir}/coverage.xml` appended to the existing pytest `addopts`
>
> **Forbidden — never touch these, regardless of current value:**
> - Any existing `--cov=<anything>` — leave exactly as written, even if it uses a
>   substitution like `{toxinidir}` or `{[vars]MODULE}` that looks "wrong"
> - Any tox substitution variable (`{toxinidir}`, `{[vars]MODULE}`, etc.)
> - Any other `[testenv]` key not explicitly listed as "Allowed" above
>
> **Correct vs incorrect:**
> ```
> # WRONG — subagent replaced maintainer's substitution with a hardcoded name:
> Before: addopts = --cov={toxinidir} --cov-report=term
> After:  addopts = --cov=mypackage --cov-report=term --cov-report=xml:coverage.xml
>
> # RIGHT — only the missing XML report appended; --cov= untouched:
> Before: addopts = --cov={toxinidir} --cov-report=term
> After:  addopts = --cov={toxinidir} --cov-report=term --cov-report=xml:{toxinidir}/coverage.xml
> ```

### CI Job Modifiers

Applied on top of the upload job templates read from `codecov-onboarding`.

#### GitLab CI — Prepare Modifier

Read the upload job template from `codecov-onboarding` Option C. Then apply:

1. In the job's `variables:` block, add:
   ```yaml
   CODECOV_URL: "PLACEHOLDER"
   ```
2. Append this `rules:` block (overrides all other rules, making the job inert):
   ```yaml
     rules:
       - when: never   # DISABLED — remove this block when Codecov instance is ready
   ```

#### GitLab CI — Enable Modifier

1. Read the real Codecov instance URL from `codecov-config/CONFIG.md`.
2. Replace `CODECOV_URL: "PLACEHOLDER"` with the real URL.
3. Remove the entire `rules: - when: never` block.
4. Add proper trigger rules:
   ```yaml
     rules:
       - if: '$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH'
       - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
     allow_failure: true
   ```

#### GitHub Actions — Prepare Modifier

Read the upload step template from `codecov-onboarding` Option A (self-hosted/token auth
variant). Add to the **existing primary test workflow file** — do not create a new file.

Apply `if: false` and `url: PLACEHOLDER` to the upload step:
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

#### GitHub Actions — Enable Modifier

1. Read the real Codecov instance URL from `codecov-config/CONFIG.md`.
2. Replace `url: PLACEHOLDER` with `url: <real-instance-url>`.
3. Remove the `if: false` line from the upload step.

### Prepare Local Workflow

Clones every target repo, applies all prepare-mode changes, commits locally, prints a
diff — then stops. No push, no MR. In bulk mode, use the Bulk Dispatch section.

1. **Parse CSV / identify target** (filter `Onboard=TRUE`, `Has Codecov ≠ TRUE`).
2. **Announce:** "Prepare local — found N repos. Dispatching N subagents in parallel. No MRs will be opened."
3. **For each repo:** run the full Prepare Mode Workflow through the **commit step**, skipping
   the idempotency check and the push/MR step.
4. After committing, print the per-repo diff:
   ```bash
   git show HEAD --stat --patch
   ```
5. Print the session summary with a `PREPARE LOCAL` header; replace "Opened MRs/PRs"
   with "Committed locally (not pushed)".
6. Local clones remain in `/tmp/codecov-setup/<repo-name>/`. Run `prepare` when ready
   to push (it re-clones fresh).

### Prepare Mode Workflow

Execute for each target repo (directly in single-repo mode; subagents run independently in bulk):

1. **Idempotency check:** Search for an open MR/PR with branch name `add-codecov-config`.
   If one exists, skip this repo and add it to the "already prepared" list.
2. **Clone the repo:**
   ```bash
   git clone <repo-url> /tmp/codecov-setup/<repo-name>
   cd /tmp/codecov-setup/<repo-name>
   ```
   No token needed to clone. `GITLAB_TOKEN` is only required at the push/MR step.
3. **Create branch:** `git checkout -b add-codecov-config`
4. **Identify CI file** from the audit CSV (`CI System` column):
   - `gitlab-ci` → `.gitlab-ci.yml`
   - `github-actions` → `.github/workflows/` (find the primary test workflow)
5. **Find test command** in the CI file by searching for the language's test runner.
6. **Inject coverage flags** per the Coverage Flag Detection table. If no command found,
   insert the `# TODO` comment and add the repo to the manual-attention list.
7. **Read upload template** from `codecov-onboarding` SKILL.md:
   - GitLab → Option C (new job block appended to `.gitlab-ci.yml`)
   - GitHub → Option A (new step added to the existing primary test workflow file)
8. **Apply the GitLab or GitHub prepare modifier** (see CI Job Modifiers above).
9. **Write the change** to the CI config:
   - GitLab: append the modified job block to `.gitlab-ci.yml`
   - GitHub: add the modified upload step to `.github/workflows/<test-workflow>.yml` — do not create a new workflow file
10. **Handle `.codecov.yml`** using the template from `add-codecov-yml/skill.md`:
    - Absent → generate from template, write to repo root
    - Present and compliant → skip
    - Present but non-compliant → fix in-place and include in this PR
11. **Commit:**
    ```bash
    git add -A
    git commit -m "chore: add codecov setup (disabled, pending internal instance)"
    ```
12. **Push and open MR/PR** following `add-codecov-yml/skill.md § 4` (handles fork check,
    push access, and platform-specific MR/PR creation). Requires `GITLAB_TOKEN` for GitLab,
    `gh` auth for GitHub. If running as a subagent, the token must have been passed
    explicitly in the subagent's instructions — subagents do not inherit the parent shell's
    environment variables.
    - Title: `chore: add Codecov coverage config (disabled — pending internal instance)`
    - Body: see PR Description Templates below
13. **Record** the MR/PR URL in the session summary.

### Enable Mode Workflow

1. **Read instance URL** from `codecov-config/CONFIG.md`. If still `PLACEHOLDER`, stop:
   "Instance URL is not set in codecov-config/CONFIG.md — cannot run enable mode."
2. **Idempotency check:** Search for an open MR/PR with branch `enable-codecov-coverage`.
   If found, skip and add to the "already enabled" list.
3. **Clone** the repo (no token needed):
   ```bash
   git clone <repo-url> /tmp/codecov-setup/<repo-name>
   cd /tmp/codecov-setup/<repo-name>
   ```
4. **Verify** the disabled upload job/step is present in the default branch:
   - GitLab: look for a job block containing `when: never`
   - GitHub: look for `if: false` on a step named `Upload coverage to Codecov`

   If not found, skip and add to **Needs Manual Attention**: "prepare change not found in
   default branch — was the prepare MR/PR merged?"
5. **Create branch:** `git checkout -b enable-codecov-coverage`
6. **Apply the GitLab or GitHub enable modifier** (see CI Job Modifiers above).
7. **Commit:**
   ```bash
   git add -A
   git commit -m "chore: enable codecov upload to <instance-url>"
   ```
8. **Push and open MR/PR** following `add-codecov-yml/skill.md § 4`.
   - Title: `feat: enable Codecov coverage reporting`
   - Body: see PR Description Templates below
9. **Record** the MR/PR URL in the session summary.

### Full Mode Workflow

1. **Idempotency check:** Search for an open MR/PR with branch `add-codecov-coverage`.
   If found, skip and add to the "already set up" list.
2. **Read instance URL** from `codecov-config/CONFIG.md`. If still `PLACEHOLDER`, stop.
3. **Clone → branch → inject coverage flags → read template → apply enable modifier →
   handle `.codecov.yml` → commit** (same as Prepare Mode, steps 2–11), with:
   - Branch name: `add-codecov-coverage`
   - Apply the **enable modifier** (not prepare modifier) using the real URL
4. **Push and open MR/PR** following `add-codecov-yml/skill.md § 4`.
   - Title: `feat: add Codecov coverage reporting`
   - Body: "Adds fully enabled Codecov coverage upload. Coverage uploads begin on next pipeline run after merge."
5. **Record** the MR/PR URL in the session summary.

### Bulk Dispatch (CSV Mode)

Used by all modes when a CSV is provided. Repos are processed in **parallel waves**.

**Batch size:** default `15`. Pass `--batch-size <N>` to override.

**`GITLAB_TOKEN` resolution:** The agent shell is a separate process from the user's
terminal — `export GITLAB_TOKEN=...` in the user's terminal is not visible to the agent.
Before dispatching subagents for GitLab repos in `prepare` or `enable` mode, resolve the
token using this discovery order (same as `coverage-audit`):

```python
import json, os

# 1. Environment variable
token = os.environ.get("GITLAB_TOKEN") or os.environ.get("GITLAB_PERSONAL_ACCESS_TOKEN")

# 2. MCP server config in ~/.claude/settings.json
if not token:
    try:
        with open(os.path.expanduser("~/.claude/settings.json")) as f:
            cfg = json.load(f)
        for server in cfg.get("mcpServers", {}).values():
            env = server.get("env", {})
            token = env.get("GITLAB_PERSONAL_ACCESS_TOKEN") or env.get("GITLAB_TOKEN")
            if token:
                break
    except Exception:
        pass
```

- **Token found** — pass the resolved value to each subagent's instructions so they can
  `export GITLAB_TOKEN=<value>` at the start of their shell session.
- **Token not found** — stop and ask the user:
  ```
  Could not find GITLAB_TOKEN in the agent environment or ~/.claude/settings.json.
  Please provide it one of two ways:
  a) Paste the token value directly into this chat (used for this run only)
  b) Add export GITLAB_TOKEN=<value> to ~/.bashrc or ~/.zprofile for permanent access
  ```

Never print or log the token value in summaries or output.

1. **Parse CSV.** Filter to `Onboard=TRUE` AND `Has Codecov ≠ TRUE`.
2. **Check progress file:** If `.codecov-setup-progress.json` exists, skip repos already
   recorded under the same mode.
3. **Split into waves** of `batch_size` repos each.
   - Example: 47 repos, batch 15 → wave 1 (15), wave 2 (15), wave 3 (15), wave 4 (2).
4. **Announce:**
   ```
   Found N repos. Batch size: <batch_size>. Dispatching in <num_waves> wave(s).
   Wave 1/<num_waves>: repos 1–<batch_size> — dispatching <batch_size> subagents in parallel.
   ```
5. **For each wave (sequentially):**
   a. Dispatch all repos in this wave simultaneously (all Task calls in a single turn).
      Each subagent receives: repo URL, language, CI system, mode, instance URL (if needed),
      and instructions to run this skill's single-repo workflow. Working dir: `/tmp/codecov-setup/<repo-name>/`
      For GitLab repos in `prepare` or `enable` mode, also pass the resolved value of
      `GITLAB_TOKEN` read from the parent environment before dispatch — subagents run in
      isolated shells that do not inherit the parent session's environment variables.
   b. Wait for all subagents in this wave to complete before proceeding.
   c. Append wave results to `.codecov-setup-progress.json`:
      ```json
      {
        "mode": "prepare",
        "batch_size": 15,
        "timestamp": "<ISO-8601>",
        "results": [
          {"repo": "<url>", "status": "opened", "pr_url": "<url>"},
          {"repo": "<url>", "status": "skipped", "reason": "already prepared"}
        ]
      }
      ```
   d. Announce: `Wave 1 complete (15/47). Wave 2/<num_waves>: repos 16–30 — dispatching 15 subagents.`
6. **Print the session summary** after all waves complete.

### Idempotency

| Situation | Action |
|---|---|
| Open MR/PR on branch `add-codecov-config` already exists | Skip prepare; add to "already prepared" list |
| Open MR/PR on branch `enable-codecov-coverage` already exists | Skip enable; add to "already enabled" list |
| Open MR/PR on branch `add-codecov-coverage` already exists | Skip full; add to "already set up" list |
| Repo recorded in `.codecov-setup-progress.json` under the same mode | Skip before dispatching |

Never open a duplicate MR/PR. Always report skips in the summary.

### PR Description Templates

#### Prepare Mode

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

#### Enable Mode

**GitLab MR body:**
```markdown
## Codecov Enable

Activates the upload job added in the prepare MR. Coverage uploads begin on the
next pipeline run after merge.

**Removed:** `when: never` disable guard  
**Set:** `CODECOV_URL` to the real instance URL  
**Prerequisite:** `CODECOV_TOKEN` must be set as a masked CI/CD variable before merging.
```

**GitHub PR body:**
```markdown
## Codecov Enable

Activates the upload step added in the prepare PR. Coverage uploads begin on the
next workflow run after merge.

**Removed:** `if: false` disable guard  
**Set:** `url:` to the real instance URL
```

### Common Mistakes

Real subagent errors observed in production runs.

| Mistake | Rule |
|---|---|
| Mutated `--cov={toxinidir}` to `--cov=mypackage` in `tox.ini` | Never change an existing `--cov=` target — tox substitutions are intentional |
| Added `--cov-report=xml` without the `:{toxinidir}/coverage.xml` path | Always use the full `xml:{toxinidir}/coverage.xml` form in tox |
| Injected `--cov` flags without verifying `pytest-cov` is installed | Check requirements file or install line; add `pip install pytest-cov` if absent |
| Suggested migrating `unittest` to `pytest` to get coverage | `coverage.py` wraps `unittest` directly — no migration needed |
| Used `--cov=.` without a TODO comment | Always add `# TODO: replace . with the actual package name` on the same line |
| Created a new GitHub Actions workflow file for the upload step | Always add the upload step to the existing primary test workflow file |

### Session Summary Format

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
