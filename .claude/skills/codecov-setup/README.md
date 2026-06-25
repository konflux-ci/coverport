# codecov-setup — Automated Bulk Codecov Onboarding

AI skill for non-interactive, data-driven Codecov onboarding across one or many
repositories. Reads an audit CSV and opens one PR/MR per repo automatically —
no interactive Q&A required.

**Skills location:** https://github.com/konflux-ci/coverport/blob/main/.claude/skills/codecov-setup

## What This Skill Does

| Phase | What happens |
|---|---|
| **Prepare** | Adds a disabled CI upload job + coverage flags + `.codecov.yml` to every repo. Zero CI impact — the job is inert until the enable phase. |
| **Enable** | Removes the disable guard and fills in the real Codecov instance URL. One PR per repo activates coverage uploads. |
| **Full** | Prepare + enable in a single PR (use when the Codecov instance is already live). |

In bulk mode, all repos are processed in parallel — one subagent per repo.

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
| `Repo URL` | Full HTTPS URL to the repository |
| `Onboard` | `TRUE` to include this repo |
| `Language` | Go, Python, JavaScript, TypeScript, C, C++ |
| `CI System` | `gitlab-ci` or `github-actions` |
| `Has Codecov` | `TRUE` if already configured (these rows are skipped) |

### Required: CLI tools

| Tool | Used for | Install |
|---|---|---|
| `gh` | GitHub repos — access check, clone, fork, open PR | `dnf install gh` / [brew](https://cli.github.com) |
| `glab` | GitLab repos — access check, fork, open MR | `dnf install glab` / [binary](https://gitlab.com/gitlab-org/cli) |
| `git` | All platforms | Pre-installed on most systems |

**No `glab`?** The skill falls back to direct `curl` calls against the GitLab REST API.
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
Can you go through audit-2026-q2.csv and open enable PRs for all the repos
that had their prepare PRs merged?
```

### Single-repo prepare

```
Set up Codecov for https://gitlab.cee.redhat.com/myteam/myservice — the
instance isn't ready yet. It's a Go service using .gitlab-ci.yml.
```

### Dry run (preview only)

```
Show me what would change for the repos in ~/audit.csv without actually
opening any PRs.
```

---

## Two-Phase Rollout Overview

```
Phase 1 — Prepare PR (now)          Phase 2 — Enable PR (when instance is live)
─────────────────────────────────    ────────────────────────────────────────────
+ coverage flags in CI test cmd      - remove 'when: never' / 'if: false'
+ codecov-upload job (disabled)      + set real CODECOV_URL
+ .codecov.yml config                PR title: feat: enable Codecov coverage reporting
PR title: chore: add Codecov
  coverage config (disabled)
```

Phase 1 PRs have zero CI impact — the upload job never runs until Phase 2.

---

## Updating

Re-run the `curl` install commands to get the latest version. Skills are plain
markdown files — the new version overwrites the old one.

## Questions?

Reach out to the Code Coverage Workgroup.
