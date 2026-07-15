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
- `prepare` — clone each repo fresh, apply disabled changes (coverage flags + disabled upload
  job + `.codecov.yml`), commit locally, print diff. **Always stops here — no push.**
  Records `committed_locally` in the progress file.
- `enable` — clone each repo fresh (independent of any prior prepare run), remove disable
  guard, commit locally, print diff. **Always stops here — no push.** Records
  `committed_locally` in progress file.
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
| "enable codecov" / "enable for audit.csv" / "activate the upload jobs" / "the instance is live, enable it" | enable | CSV (ask for path if not given) |
| "open the MRs" / "push those changes" / "apply" | apply | reads progress file |
| "set up Codecov for myrepo — instance isn't ready yet" | prepare | single repo |
| "add Codecov to this repo, instance is already running" | full | single repo |
| "show me what would change without opening any PRs" | fast dry run | CSV or single |
| "prepare local for audit.csv" / "show me the diffs first" | prepare | CSV |

If the user doesn't mention a CSV path, ask for it before proceeding. If operation is
genuinely ambiguous (e.g. "set up codecov" with no other context), ask: "Have the prepare
MRs already been merged? If yes → enable; if no → prepare or full."

**If the user explicitly names an operation, run it — do not ask for confirmation.**

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
| HCL/Terraform, Helm, YAML-only, pure config | **No coverage possible** — skip this repo entirely. Do not add an upload job. Add to manual-attention: "No coverage-generating tests — <language> has no standard coverage tooling" | — |
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
> directly, apply changes to `tox.ini` instead of the CI config file.
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

#### GitLab CI Modifier

Read the upload job template from `codecov-onboarding` Option C. Apply fields per operation:

| Field | Prepare | Enable |
|---|---|---|
| `variables:` block | Do not add — `CODECOV_URL` (group) and `CODECOV_TOKEN` (project) are set as CI/CD variables outside YAML | same |
| `tags:` | Add `[itup-alm-x86]` | Verify present; add if missing |
| `rules:` | Add `when: never` block (disables job) | Remove `when: never`; add trigger rules (see below) |

**Prepare `rules:` block:**
```yaml
  rules:
    - when: never   # DISABLED — remove this block when Codecov instance is ready
```

**Enable `rules:` block:**
```yaml
  rules:
    - if: '$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH'
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
  allow_failure: true
```

#### GitHub Actions Modifier

Read the upload step template from `codecov-onboarding` Option A. Add to the **workflow
file that contains the test command** (identified in step 5) — do not create a new file.
`CODECOV_URL` and `CODECOV_TOKEN` are repository secrets; do not hardcode the URL.

| Field | Prepare | Enable |
|---|---|---|
| `if:` on upload step | Add `if: false` (disables step) | Remove `if: false` |
| `url:` | Verify `${{ secrets.CODECOV_URL }}` present; add if missing | same |

**Step template (prepare — with `if: false`):**
```yaml
    - name: Upload coverage to Codecov
      if: false  # DISABLED — remove this line when Codecov instance is ready
      uses: codecov/codecov-action@v6
      with:
        url: ${{ secrets.CODECOV_URL }}
        token: ${{ secrets.CODECOV_TOKEN }}
        flags: unit-tests
        files: <coverage-file-path>
        fail_ci_if_error: false
```

**Step template (enable — `if: false` removed):**
```yaml
    - name: Upload coverage to Codecov
      uses: codecov/codecov-action@v6
      with:
        url: ${{ secrets.CODECOV_URL }}
        token: ${{ secrets.CODECOV_TOKEN }}
        flags: unit-tests
        files: <coverage-file-path>
        fail_ci_if_error: false
```


### Prepare / Enable Workflow

Both operations clone fresh, inject coverage flags, write `.codecov.yml`, apply the
appropriate CI modifier, commit, and stop — the only differences are:

| | `prepare` | `enable` |
|---|---|---|
| Pre-check | — | Confirm instance URL is set in `codecov-config/CONFIG.md` |
| CI modifier | Prepare modifier (`when: never` / `if: false`) | Enable modifier (real URL, no guard) |
| Branch | `add-codecov-config` | `enable-codecov-coverage` |
| Commit message | `chore: add codecov setup (disabled, pending internal instance)` | `chore: enable codecov upload to <instance-url>` |
| Mode recorded | `prepare` | `enable` |

**[enable only — pre-check]** Read instance URL from `codecov-config/CONFIG.md`. If still
`PLACEHOLDER`, stop: "Instance URL is not set in codecov-config/CONFIG.md — cannot run enable."

No push in either case. In bulk mode, dispatch one subagent per repo using Bulk Dispatch.

1. **Idempotency check:** Check `.codecov-setup-progress.json` for this repo under the
   current mode. If status is `committed_locally` or `opened`, skip.
2. **Clone the repo fresh:**
   - **GitLab:** `git clone <repo-https-url> /tmp/codecov-setup/<repo-name>`
   - **GitHub:** prefer `gh repo clone <org/repo> /tmp/codecov-setup/<repo-name>` — uses
     `gh`'s stored auth and avoids interactive prompts for private repos. Fall back to
     `git clone` only if `gh` is not available and non-interactive credentials (SSH or
     token-based HTTPS) are already configured.
   ```bash
   cd /tmp/codecov-setup/<repo-name>
   ```
3. **Create branch** (name from table above).
4. **Identify CI file** from the audit CSV (`CI System` column):
   - `gitlab-ci` → `.gitlab-ci.yml`
   - `github-actions` → `.github/workflows/` (identify in step 5 — the file containing the test command)

   **If the expected CI file is absent:** skip this repo — do not commit or open an MR.
   Add to **Needs Manual Attention**:
   - `gitlab-ci` declared but no `.gitlab-ci.yml` → "No GitLab CI config found — repo may be a GitHub mirror or CI not yet configured"
   - `github-actions` declared but no `.github/workflows/` → "No GitHub Actions workflows found — CI not yet configured"

5. **Find test command** in the CI file by searching for the language's test runner.
6. **Inject coverage flags** per Coverage Flag Detection.
   - If the repo's language has no coverage tooling (HCL/Terraform, Helm, pure config, etc.),
     **stop here for this repo** — do not add an upload job or commit anything. Add to
     manual-attention: "No coverage-generating tests — <language> has no standard coverage tooling."
   - If no test command is found in the CI file, insert a `# TODO` comment and add to
     manual-attention list.
7. **[GitLab CI only] Expose coverage file as artifact** on the same test job:
   Add or update the `artifacts:` block in the test job to include the coverage output
   file (path from Coverage Flag Detection table). If an `artifacts:` block already exists,
   add the coverage file to its `paths:` list; do not replace existing entries:
   ```yaml
     artifacts:
       paths:
         - <coverage-output-file>   # e.g. coverage.xml, coverage.out
       expire_in: 1 day
   ```
   Without this, the upload job cannot locate the coverage file.
8. **Read upload template** from `codecov-onboarding` SKILL.md (GitLab → Option C; GitHub → Option A).
9. **Check for rules mismatch [GitLab CI only]:** Compare the test job's `rules:` block
   with the upload template's `rules:` block. The upload job must only fire on pipelines
   where the test job also ran — otherwise it will fail with no coverage file.
   (GitHub Actions: not applicable — the upload step is in the same job as the test step
   and always runs together; no separate job-level rules to reconcile.)
   - If the upload job's rules are **broader** than the test job's (e.g. upload fires on
     `$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH` but the test job only fires on
     `merge_request_event`): narrow the upload job's rules to match the test job's rules
     exactly. Note this in the commit message and add to manual-attention:
     "Upload job rules narrowed to match test job — coverage only uploads on MR events,
     not main branch pushes. Consider expanding test job rules if main-branch coverage is needed."
   - If the test job has `changes:` file filters on its rules: the upload job does not
     need to mirror these (Codecov uploads are cheap); keeping the upload job trigger
     broader than the file filter is acceptable as long as the test job ran first via `needs:`.
10. **Apply the CI modifier** for the current mode (see CI Job Modifiers above).
11. **Write the change** to the CI config:
    - GitLab: append the modified job block to `.gitlab-ci.yml`
    - GitHub: add the modified upload step to the workflow file that contains the test
      command from step 5 — do not create a new workflow file
12. **Handle `.codecov.yml`** using the template from `add-codecov-yml/skill.md`:
    - Absent → generate from template, write to repo root
    - Present and compliant → skip
    - Present but non-compliant → fix in-place
13. **Commit** (message from table above):
    ```bash
    git add -A
    git commit -m "<commit message from table>"
    ```
14. **Print diff:** `git show HEAD --stat --patch`
15. **Return result** — output a JSON object for the parent to collect:
    ```json
    {"repo": "<url>", "status": "committed_locally", "mode": "<prepare|enable>",
     "branch": "<branch from table>", "local_path": "/tmp/codecov-setup/<repo-name>"}
    ```
    **In bulk mode (dispatched as a subagent):** do NOT write the progress file — the
    parent collects all subagent results and writes the file once after the wave completes.
    **In single-repo mode (`--target`):** write this entry directly to the progress file.
16. **Stop.** Run `apply` when ready to push.

### Apply Workflow

For each repo with `committed_locally` status, pushes and opens MR/PR.
In bulk mode, dispatch one subagent per repo using Bulk Dispatch.

1. **Resolve credentials** using the Token Discovery logic from `coverage-audit/SKILL.md`
   for the platform of the repo being pushed. Never run `git credential fill` or
   `glab auth` as shell commands — use the Python subprocess approach.
2. **Push and open MR/PR** following `add-codecov-yml/skill.md § 4`:
   ```bash
   git -C <local_path> push -u origin <branch>
   ```
   MR/PR title and body: see PR Description Templates below (based on `mode` field).
3. **Return result** — output a JSON object for the parent to collect:
   ```json
   {"repo": "<url>", "status": "opened", "pr_url": "<mr-or-pr-url>"}
   ```
   **In bulk mode:** do NOT write the progress file — the parent collects all subagent
   results and updates the file once after the wave completes.
   **In single-repo mode (`--target`):** write the entry directly to the progress file.
4. **[Parent, after all waves]** Update progress file — change status from
   `committed_locally` → `opened`, add `pr_url`. Print session summary.

### Full Mode Workflow

Convenience: runs prepare + apply in one shot with no diff review stop.
In bulk mode, dispatch one subagent per repo using Bulk Dispatch.

1. **Idempotency check:** Check progress file for this repo under mode `full`. If
   `committed_locally` or `opened`, skip.
2. **Read instance URL** from `codecov-config/CONFIG.md`. If still `PLACEHOLDER`, stop.
3. **Run the Prepare / Enable Workflow** for this repo in **enable mode**, using branch
   `add-codecov-coverage`. Run through the **Return result** step. Do not stop.
4. **Run the Apply Workflow** for this repo. Use the enable mode PR description template.

### Bulk Dispatch

Used by `prepare`, `enable`, and `full` when a `--csv` is provided. `apply` uses the same
wave pattern but sources repos from the progress file instead of a CSV.

**Batch size:** default `15`. Pass `--batch-size <N>` to override.

1. **Source repos:**
   - `prepare` / `enable` / `full`: parse CSV, filter `Onboard=TRUE` AND `Has Codecov ≠ TRUE`
   - `apply`: read `.codecov-setup-progress.json`, collect entries where `status = "committed_locally"`.
     If none, report: "No locally committed repos found — run `prepare` or `enable` first."
     For each entry, verify `local_path` exists and the recorded `branch` has an unpushed commit:
     ```bash
     git -C <local_path> rev-list --count <branch> --not --remotes
     ```
     A result of `0` or missing path → **Needs Manual Attention**: "local clone missing or branch
     already pushed — re-run prepare/enable to regenerate." Exclude these from dispatch.
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
      receives: repo URL, language, CI system, instance URL (if needed), and instructions
      to execute the workflow for this operation:
      - `prepare` → run **Prepare / Enable Workflow** for this repo (prepare mode)
      - `enable` → run **Prepare / Enable Workflow** for this repo (enable mode)
      - `full` → run **Full Mode Workflow** for this repo
      - `apply` → run **Apply Workflow** for this repo

      GitLab repos: each subagent discovers `GITLAB_TOKEN` independently per
      `coverage-audit/SKILL.md`. Do not pass token values in instructions.
   b. Wait for all subagents in this wave to complete.
   c. **Parent appends results to `.codecov-setup-progress.json`** (sole writer — subagents
      return results as output, they do not write the file themselves):
      ```json
      {
        "mode": "prepare", "batch_size": 15, "timestamp": "<ISO-8601>",
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

**Before merging the enable PR**, set these repository secrets in Settings → Secrets and variables → Actions:
- `CODECOV_TOKEN` — repo-specific upload token from the Codecov UI (repository level)
- `CODECOV_URL` — instance URL from `codecov-config/CONFIG.md` (can be set at org level to share across repos)

**Next:** a follow-up PR will remove `if: false`.
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
**Prerequisite:** `CODECOV_TOKEN` (repository level) and `CODECOV_URL` (org or repository level)
must be set as repository secrets before merging.
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
| Added a placeholder upload job to a repo with no coverage-generating tests (e.g. pure HCL/Terraform) | Skip entirely — no upload job, no commit; flag as manual-attention |
| Upload job fires on pipelines where the test job doesn't run (rules mismatch) | Narrow upload job rules to match the test job's trigger conditions; note in manual-attention |

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
