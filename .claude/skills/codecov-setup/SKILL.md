---
name: codecov-setup
description: >-
  Use when bulk-onboarding multiple repos to Codecov from an audit CSV,
  or when user says "bulk codecov", "prepare codecov", "enable codecov",
  "codecov from audit", "onboard repos to codecov", "apply codecov changes",
  "open the MRs", "push those changes", "enable coverage for all repos",
  "prepare local for audit.csv". Not for single-repo interactive onboarding
  (codecov-onboarding) or E2E instrumentation (coverport-integration).
---

# Codecov Setup Skill

Non-interactive, CSV-driven Codecov onboarding. Reads an audit CSV (from
`coverage-audit`) and prepares or enables Codecov across repos automatically.
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

These paths are relative to the skills directory (`.claude/skills/` or `.cursor/skills/`).

## Instructions

### Operations and Targeting

**Operations:**
- `prepare` — clone each repo, apply disabled changes (coverage flags + disabled upload
  job + `.codecov.yml`), commit locally, print diff. **Always stops here — no push.**
  Records `committed_locally` in the progress file.
- `enable` — clone each repo, remove disable guard, set real instance URL, commit locally,
  print diff. **Always stops here — no push.** Records `committed_locally` in progress file.
- `apply` — push + open MR/PR for all repos with `committed_locally` status in the progress
  file. Reads progress file to find what's ready, confirms local clone still exists, then
  pushes in batch waves. Works for both prepare and enable changes.
- `full` — convenience shorthand: runs prepare + apply back-to-back with no diff review
  stop. Fully enabled job (no disable guard); Codecov instance must be live.

**Targeting:**
- `--target <repo-url>` — single-repo mode; execute steps directly in this session
- `--csv <path>` — bulk mode; dispatch one subagent per repo in parallel waves

All operations use batched multi-subagent dispatch in bulk mode — one subagent per repo,
in parallel waves of up to `batch_size` (default: 15). Pass `--batch-size <N>` to override.

**Fast dry run** — if the user says "dry run", "preview", or "what would change": infer
changes from the CSV data alone (no cloning). Shows which repos would be touched and what
flags/jobs would be added based on declared language and CI system. No network, no cloning.

### How to Invoke This Skill

Recognize the user's intent and map it to the appropriate operation:

| User says | Operation | Targeting |
|---|---|---|
| "prepare for ~/Downloads/audit.csv" | prepare | CSV |
| "set up all the Onboard=TRUE repos in disabled state" | prepare | CSV (ask for path if not given) |
| "enable for audit-q2.csv — instance is live" | enable | CSV |
| "open the MRs" / "push those changes" / "apply" | apply | reads progress file |
| "set up Codecov for myrepo — instance isn't ready yet" | prepare | single repo |
| "add Codecov to this repo, instance is already running" | full | single repo |
| "show me what would change without opening any PRs" | fast dry run | CSV or single |
| "prepare local for audit.csv" / "show me the diffs first" | prepare | CSV |

If the user doesn't mention a CSV path, ask for it before proceeding. If operation is
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
| TypeScript (Angular/Karma) | Append `--code-coverage` to `ng test --no-watch`; lcov at `coverage/<project-name>/lcov.info` — **always add to manual-attention:** "verify headless Chrome configured (`--browsers ChromeHeadless` or `karma.conf.js`) before enabling" | `coverage/<project-name>/lcov.info` |
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

1. **Do not add a `variables:` block** for `CODECOV_URL` or `CODECOV_TOKEN`. The job
   references both as CI/CD variables — set at different scopes:
   - `CODECOV_URL` — set at the **group level** (shared across all repos in the group);
     changing it once (e.g. staging → production) updates every repo with no YAML changes
   - `CODECOV_TOKEN` — set at the **project level** only; it is a repo-specific upload
     token from the Codecov UI and must not be set at group level
2. Add the required runner tag:
   ```yaml
   tags:
     - itup-alm-x86
   ```
3. Append this `rules:` block (overrides all other rules, making the job inert):
   ```yaml
     rules:
       - when: never   # DISABLED — remove this block when Codecov instance is ready
   ```

#### GitLab CI — Enable Modifier

1. Verify `tags: [itup-alm-x86]` is present; add if missing.
2. Remove the entire `rules: - when: never` block.
3. Add proper trigger rules:
   ```yaml
     rules:
       - if: '$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH'
       - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
     allow_failure: true
   ```

No URL substitution needed — `CODECOV_URL` is a CI/CD variable, not hardcoded in the YAML.

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

### Prepare Workflow

For each target repo, clones, applies disabled changes, commits, shows diff — then stops.
No push. In bulk mode, dispatch one subagent per repo using Bulk Dispatch.

1. **Idempotency check:** Check `.codecov-setup-progress.json` for this repo under mode
   `prepare`. If status is `committed_locally` or `opened`, skip.
2. **Clone the repo:**
   ```bash
   git clone <repo-url> /tmp/codecov-setup/<repo-name>
   cd /tmp/codecov-setup/<repo-name>
   ```
   No token needed to clone.
3. **Create branch:** `git checkout -b add-codecov-config`
4. **Identify CI file** from the audit CSV (`CI System` column):
   - `gitlab-ci` → `.gitlab-ci.yml`
   - `github-actions` → `.github/workflows/` (find the primary test workflow)

   **If the expected CI file is absent:** skip this repo — do not commit or open an MR.
   Add to **Needs Manual Attention**:
   - `gitlab-ci` declared but no `.gitlab-ci.yml` → "No GitLab CI config found — repo may be a GitHub mirror or CI not yet configured"
   - `github-actions` declared but no `.github/workflows/` → "No GitHub Actions workflows found — CI not yet configured"

5. **Find test command** in the CI file by searching for the language's test runner.
6. **Inject coverage flags** per Coverage Flag Detection. If no command found, insert
   `# TODO` comment and add to manual-attention list.
7. **Read upload template** from `codecov-onboarding` SKILL.md (GitLab → Option C; GitHub → Option A).
8. **Apply the GitLab or GitHub prepare modifier** (see CI Job Modifiers above).
9. **Write the change** to the CI config:
   - GitLab: append the modified job block to `.gitlab-ci.yml`
   - GitHub: add the modified upload step to `.github/workflows/<test-workflow>.yml` — do not create a new workflow file
10. **Handle `.codecov.yml`** using the template from `add-codecov-yml/skill.md`:
    - Absent → generate from template, write to repo root
    - Present and compliant → skip
    - Present but non-compliant → fix in-place
11. **Commit:**
    ```bash
    git add -A
    git commit -m "chore: add codecov setup (disabled, pending internal instance)"
    ```
12. **Print diff:** `git show HEAD --stat --patch`
13. **Record in progress file:**
    ```json
    {"repo": "<url>", "status": "committed_locally", "mode": "prepare",
     "branch": "add-codecov-config", "local_path": "/tmp/codecov-setup/<repo-name>"}
    ```
14. **Stop.** Run `apply` when ready to push.

### Enable Workflow

For each target repo, clones, removes the disable guard, commits, shows diff — then stops.
No push. In bulk mode, dispatch one subagent per repo using Bulk Dispatch.

1. **Read instance URL** from `codecov-config/CONFIG.md`. If still `PLACEHOLDER`, stop:
   "Instance URL is not set in codecov-config/CONFIG.md — cannot run enable."
2. **Idempotency check:** Check progress file for this repo under mode `enable`. If
   `committed_locally` or `opened`, skip.
3. **Clone** the repo (no token needed):
   ```bash
   git clone <repo-url> /tmp/codecov-setup/<repo-name>
   cd /tmp/codecov-setup/<repo-name>
   ```
4. **Verify** the disabled upload job/step is present in the default branch:
   - GitLab: look for a job block containing `when: never`
   - GitHub: look for `if: false` on a step named `Upload coverage to Codecov`

   If not found, skip and add to **Needs Manual Attention**: "prepare change not found in
   default branch — was the prepare MR merged?"
5. **Create branch:** `git checkout -b enable-codecov-coverage`
6. **Apply the GitLab or GitHub enable modifier** (see CI Job Modifiers above).
7. **Commit:**
   ```bash
   git add -A
   git commit -m "chore: enable codecov upload to <instance-url>"
   ```
8. **Print diff:** `git show HEAD --stat --patch`
9. **Record in progress file:**
   ```json
   {"repo": "<url>", "status": "committed_locally", "mode": "enable",
    "branch": "enable-codecov-coverage", "local_path": "/tmp/codecov-setup/<repo-name>"}
   ```
10. **Stop.** Run `apply` when ready to push.

### Apply Workflow

Pushes and opens MRs/PRs for all repos that were prepared or enabled locally.

1. **Read progress file** (`.codecov-setup-progress.json`). Collect all entries where
   `status = "committed_locally"`. If none found, report: "No locally committed repos
   found in progress file — run `prepare` or `enable` first."
2. **Confirm local clones:** For each entry, verify `local_path` exists and has an
   unpushed commit on the recorded `branch`:
   ```bash
   git -C <local_path> log --branches --not --remotes --oneline | head -1
   ```
   If the path is missing or the branch has no unpushed commit, add to **Needs Manual
   Attention**: "local clone missing or already pushed — re-run prepare/enable to regenerate."
3. **Resolve `GITLAB_TOKEN`** for GitLab repos using the discovery logic in
   `coverage-audit/SKILL.md` (env var → `~/.claude/settings.json` → `git credential fill`
   for the repo's host → ask user). Pass the resolved token to each subagent.
4. **Announce:**
   ```
   Found N repos ready to push. Dispatching in <num_waves> wave(s) (batch: <batch_size>).
   ```
5. **Dispatch push+MR subagents** in batch waves (see Bulk Dispatch). Each subagent:
   a. `cd <local_path>`
   b. Push and open MR/PR following `add-codecov-yml/skill.md § 4`:
      ```bash
      git push -u origin <branch>
      ```
      - MR/PR title and body: see PR Description Templates below (based on `mode` field)
   c. Return the MR/PR URL
6. **Update progress file:** change each repo's status from `committed_locally` → `opened`,
   add `pr_url`.
7. **Print session summary.**

### Full Mode Workflow

Convenience: runs prepare + apply in one shot with no diff review stop.

1. **Idempotency check:** Check progress file for this repo under mode `full`. If
   `committed_locally` or `opened`, skip.
2. **Read instance URL** from `codecov-config/CONFIG.md`. If still `PLACEHOLDER`, stop.
3. **Clone → branch → inject coverage flags → read template → apply enable modifier →
   handle `.codecov.yml` → commit** (same as Prepare Workflow steps 2–11), with:
   - Branch name: `add-codecov-coverage`
   - Apply the **enable modifier** (not prepare modifier) using the real URL
4. **Immediately push and open MR/PR** following `add-codecov-yml/skill.md § 4` (no stop).
   - Title: `feat: add Codecov coverage reporting`
   - Body: "Adds fully enabled Codecov coverage upload. Coverage uploads begin on next pipeline run after merge."
5. **Record** the MR/PR URL in the session summary.

### Bulk Dispatch

Used by `prepare`, `enable`, and `full` when a `--csv` is provided. `apply` uses the same
wave pattern but sources repos from the progress file instead of a CSV.

**Batch size:** default `15`. Pass `--batch-size <N>` to override.

1. **Source repos:**
   - `prepare` / `enable` / `full`: parse CSV, filter `Onboard=TRUE` AND `Has Codecov ≠ TRUE`
   - `apply`: read progress file, collect `committed_locally` entries (confirmed by local clone check)
2. **Check progress file:** skip repos already recorded as `committed_locally` or `opened`
   under the same mode.
3. **Split into waves** of `batch_size`.
4. **Announce:**
   ```
   Found N repos. Batch size: <batch_size>. Dispatching in <num_waves> wave(s).
   Wave 1/<num_waves>: repos 1–<batch_size> — dispatching <batch_size> subagents.
   ```
5. **For each wave (sequentially):**
   a. Dispatch all repos simultaneously (all Task calls in a single turn). Each subagent
      receives: repo URL, mode, instance URL (if needed), GITLAB_TOKEN (if resolved),
      and instructions to run the single-repo workflow for that repo only.
   b. Wait for all subagents in this wave to complete.
   c. Append results to `.codecov-setup-progress.json`:
      ```json
      {
        "mode": "prepare",
        "batch_size": 15,
        "timestamp": "<ISO-8601>",
        "results": [
          {"repo": "<url>", "status": "committed_locally", "mode": "prepare",
           "branch": "add-codecov-config", "local_path": "/tmp/codecov-setup/<repo-name>"},
          {"repo": "<url>", "status": "skipped", "reason": "already prepared"}
        ]
      }
      ```
   d. Announce: `Wave 1 complete (15/47). Wave 2/<num_waves>: repos 16–30 — dispatching 15 subagents.`
6. **Print the session summary** after all waves complete.

### Idempotency

| Situation | Action |
|---|---|
| Repo has `committed_locally` or `opened` in progress file (same mode) | Skip before dispatching |
| Open MR/PR on branch `add-codecov-config` already exists | Skip prepare; add to "already prepared" list |
| Open MR/PR on branch `enable-codecov-coverage` already exists | Skip enable; add to "already enabled" list |
| Open MR/PR on branch `add-codecov-coverage` already exists | Skip full; add to "already set up" list |

Never open a duplicate MR/PR. Always report skips in the summary.

### PR Description Templates

#### Prepare Mode

**GitLab MR body:**
```markdown
## Codecov Setup (Disabled — Pending Internal Instance)

The upload job is **disabled** (`when: never`) — zero CI impact until the enable MR is merged.

**Added:** coverage flags in test command · `codecov-upload` job (disabled) · `.codecov.yml`

**Before merging the enable MR**, set these masked CI/CD variables
(token auth required — OIDC unavailable for GitLab CI):
- `CODECOV_TOKEN` — repo-specific upload token from the Codecov UI; set at **project level**
  (Settings → CI/CD → Variables on this repo)
- `CODECOV_URL` — instance URL from `codecov-config/CONFIG.md`; set at **group level** once
  to share across all repos in the group

**Next:** a follow-up MR will remove the disable guard.
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
**Prerequisite:** before merging, verify:
- `CODECOV_TOKEN` is set at **project level** (repo-specific upload token)
- `CODECOV_URL` is set at **group level** (instance URL — shared across repos)
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
| Opened an MR when no CI config file was found for the declared CI system | Skip and add to manual-attention; don't open MRs for repos with no CI |

### Session Summary Format

**Prepare / Enable (local only):**
```
## codecov-setup Summary

Operation: <prepare|enable> | Repos processed: N | Committed locally: X | Skipped: Y | Failed: Z

### Committed Locally (not pushed — run `apply` to open MRs)
| Repo | Local path | Branch |
|------|------------|--------|
| <url> | /tmp/codecov-setup/<name> | add-codecov-config |

### Skipped
| Repo | Reason |
|------|--------|
| <url> | already prepared |

### Needs Manual Attention
| Repo | Issue |
|------|-------|
| <url> | no CI config found |
```

**Apply / Full:**
```
## codecov-setup Summary

Operation: <apply|full> | Repos processed: N | Opened: X | Skipped: Y | Failed: Z

### Opened MRs/PRs
| Repo | MR/PR URL |
|------|-----------|
| <url> | <mr-url> |

### Skipped
| Repo | Reason |
|------|--------|
| <url> | already opened |

### Needs Manual Attention
| Repo | Issue |
|------|-------|
| <url> | local clone missing — re-run prepare |
```
