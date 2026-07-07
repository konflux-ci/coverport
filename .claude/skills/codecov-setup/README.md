# codecov-setup — Automated Bulk Codecov Onboarding

AI skill for non-interactive, data-driven Codecov onboarding across one or many
repositories. Reads an audit CSV and opens one PR/MR per repo automatically —
no interactive Q&A required.

**Skills location:** https://github.com/konflux-ci/coverport/blob/main/.claude/skills/codecov-setup

## What This Skill Does

| Operation | What happens |
|---|---|
| **prepare** | Clones each repo fresh, adds a disabled CI upload job + coverage flags + `.codecov.yml`, commits locally, prints diff. **No push.** Zero CI impact. |
| **enable** | Clones each repo fresh, removes the disable guard and sets the real Codecov URL, commits locally, prints diff. **No push.** |
| **apply** | Pushes locally committed changes and opens one MR/PR per repo. Works for both prepare and enable changes. |
| **full** | prepare + apply in one shot (use when the Codecov instance is already live). |

In bulk mode, repos are processed in parallel waves of up to 15 at a time — one subagent per repo per wave.

## When to Use This Skill vs codecov-onboarding

| Situation | Use |
|---|---|
| Many repos, audit CSV available | `codecov-setup` |
| Single repo, no Q&A needed | `codecov-setup` |
| Single repo, want interactive step-by-step guidance | `codecov-onboarding` |
| E2e containerized app coverage | `coverport-integration` |

## Prerequisites

### Required: audit CSV

Produced by the `coverage-audit` skill. Must contain these columns:

| Column | Description |
|---|---|
| `URL` | Full HTTPS URL to the repository |
| `Onboard` | `TRUE` to include this repo |
| `Language` | Go, Python, JavaScript, TypeScript, C, C++ |
| `CI System` | `gitlab-ci` or `github-actions` |
| `Has Codecov` | `TRUE` if already configured (these rows are skipped) |

### Required: CLI tools

| Tool | Used for | Install |
|---|---|---|
| `gh` | GitHub repos — access check, clone, fork, open PR | `dnf install gh` / [brew](https://cli.github.com) |
| `curl` | GitLab repos — access check, fork, open MR via REST API | Pre-installed on most systems |
| `git` | All platforms | Pre-installed on most systems |

GitLab operations use `curl` against the GitLab REST API directly — no `glab` needed.
Set `GITLAB_TOKEN` in your environment:
```bash
export GITLAB_TOKEN=<your-personal-access-token>
```
Create a token at `https://<your-gitlab-host>/-/user_settings/personal_access_tokens`
with `api` scope.

### Required: access to dependent skills

The following skills must be installed alongside `codecov-setup`:

- `codecov-config/CONFIG.md` — Codecov instance URL routing table
- `codecov-onboarding/SKILL.md` — CI job templates (Option A and Option C)
- `add-codecov-yml/skill.md` — `.codecov.yml` template and PR/MR creation steps

---

## Installation

### Claude Code

```bash
mkdir -p ~/.claude/skills/codecov-setup

curl -o ~/.claude/skills/codecov-setup/SKILL.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-setup/SKILL.md

# Also install dependent skills if not already present:
mkdir -p ~/.claude/skills/{codecov-config,codecov-onboarding,add-codecov-yml}

curl -o ~/.claude/skills/codecov-config/CONFIG.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-config/CONFIG.md

curl -o ~/.claude/skills/codecov-onboarding/SKILL.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-onboarding/SKILL.md

curl -o ~/.claude/skills/add-codecov-yml/skill.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/add-codecov-yml/skill.md
```

### Cursor

```bash
mkdir -p ~/.cursor/skills-cursor/codecov-setup

curl -o ~/.cursor/skills-cursor/codecov-setup/SKILL.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-setup/SKILL.md

# Also install dependent skills if not already present:
mkdir -p ~/.cursor/skills-cursor/{codecov-config,codecov-onboarding,add-codecov-yml}

curl -o ~/.cursor/skills-cursor/codecov-config/CONFIG.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-config/CONFIG.md

curl -o ~/.cursor/skills-cursor/codecov-onboarding/SKILL.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-onboarding/SKILL.md

curl -o ~/.cursor/skills-cursor/add-codecov-yml/skill.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/add-codecov-yml/skill.md
```

---

## Usage

The skill is invoked with natural language — no CLI flags needed.

### Prepare mode (instance not ready yet)

```
We ran the coverage audit and exported it to ~/Downloads/audit-2026-q2.csv.
The internal Codecov instance isn't up yet. Can you open prepare PRs for all
the Onboard=TRUE repos so we're ready to flip the switch when it is?
```

### Enable mode (instance is live)

```
The internal Codecov instance just went live. CONFIG.md has been updated.
Enable coverage for all the repos in audit-2026-q2.csv.
```

Enable is standalone — it clones fresh and does not require prepare MRs to be merged first.

### Apply (push locally committed changes and open MRs)

```
Open the MRs for all the repos we prepared.
```

Reads the progress file for all `committed_locally` entries and pushes them in waves.

### Single-repo prepare

```
Set up Codecov for https://gitlab.cee.redhat.com/myteam/myservice — the
instance isn't ready yet. It's a Go service using .gitlab-ci.yml.
```

### Fast dry run (CSV-based preview, no cloning)

```
Show me what would change for the repos in ~/audit.csv without actually
opening any PRs.
```

Infers changes from CSV columns alone — instant, zero network.

---

## Two-Phase Rollout Overview

```
Step 1 — prepare (local)            Step 2 — apply (open MRs)
────────────────────────────────    ──────────────────────────
+ coverage flags in CI test cmd     Push branch, open one MR/PR per repo.
+ codecov-upload job (disabled)     MR title: chore: add Codecov coverage
+ .codecov.yml config                 config (disabled)
Commits locally. No push.

Step 3 — enable (local)             Step 4 — apply (open MRs)
────────────────────────────────    ──────────────────────────
- remove 'when: never'/'if: false'  Push branch, open one MR/PR per repo.
+ set real CODECOV_URL              MR title: chore: enable Codecov upload
Commits locally. No push.            to <instance-url>
```

`full` = steps 1+2 in one shot (use when the Codecov instance is already live).
Prepare MRs have zero CI impact — the upload job never runs until the enable MR is merged.

---

## Updating

Re-run the `curl` install commands to get the latest version. Skills are plain
markdown files — the new version overwrites the old one.

## Questions?

Reach out to the Code Coverage Workgroup.
