# Code Coverage AI Skills — Quick Start Guide

AI skills that help you onboard your repository to Codecov and generate
code coverage data. Once installed, your AI coding assistant will
automatically recognize coverage-related requests and guide you
through the entire process.

**Skills location:** https://github.com/konflux-ci/coverport/blob/main/.claude/skills

## What the Skills Do

| Skill | What it does |
|---|---|
| **codecov-onboarding** | Onboards any repo to Codecov — analyzes your project, configures CI, uploads coverage with proper flags |
| **codecov-setup** | Automated bulk onboarding from an audit CSV — opens prepare/enable PRs across many repos with no Q&A |
| **add-codecov-yml** | Quick-path: just adds a `.codecov.yml` config file to a repo and opens a PR (no CI setup) |
| **codecov-config** | Routes you to the correct Codecov instance based on where your repo is hosted |
| **c-cpp-coverage** | Generates coverage for C/C++ projects (gcov/lcov pipeline, workarounds for autotools/CMake/Meson) |
| **coverport-integration** | Integrates e2e test coverage via coverport for containerized Go/Python/Node.js apps (Tekton pipelines or GitHub Actions via podman) |
| **refresh-codecov-sheet** | Tracks org-wide Codecov rollout progress — refreshes PR statuses in a Google Sheet tracker |

**You don't need all of them** — install what's relevant:
- **Everyone** needs `codecov-onboarding` + `codecov-config`
- **Bulk / automated rollout** — use `codecov-setup` to onboard many repos at once from an audit CSV
- **Quick config-only rollout** — use `add-codecov-yml` to add `.codecov.yml` and open a PR without full CI onboarding
- **C/C++ projects** also need `c-cpp-coverage`
- **E2E coverage** (containerized apps with Tekton or GitHub Actions) also needs `coverport-integration`
- **Rollout tracking** — use `refresh-codecov-sheet` to bulk-check PR statuses across an org

## What the Skills Handle

- Detecting your programming language and build system
- Determining which Codecov instance to use (public or self-hosted)
- Choosing the right auth method (OIDC for GitHub Actions, tokens for GitLab CI/Tekton)
- Configuring your CI pipeline (GitHub Actions, GitLab CI, OpenShift CI/Prow)
- Generating coverage reports (language-specific)
- Uploading with proper flags (`unit-tests`, `integration-tests`, `e2e-tests`)
- Establishing a main branch baseline for PR coverage diffs
- Troubleshooting common issues

## Prerequisites

Before starting, make sure you have:
- Tests already in your repository (unit, integration, or both)
- Access to a Codecov instance (see the instance table below)
- For GitLab CI / Tekton: your Codecov upload token ready

### Codecov Instances

| Your repo is on... | Use this Codecov instance |
|---|---|
| Public GitHub (github.com) | https://app.codecov.io |
| Public GitLab (gitlab.com) | https://app.codecov.io |
| Private GitHub (github.com) | https://codecov-codecov-public-production.apps.rosa.appsrep11ue1.tgem.p3.openshiftapps.com |
| Internal GitLab (gitlab.cee.redhat.com) | https://codecov-codecov-production.apps.rosa.appsrep09ue1.03r5.p3.openshiftapps.com |

---

## Installation

### Option A: Claude Code

**Requirements:**
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) installed

#### Setup (one-time)

Run this to install all skills:

```bash
# Core skills (everyone needs these)
mkdir -p ~/.claude/skills/{codecov-onboarding,codecov-config}

curl -o ~/.claude/skills/codecov-onboarding/SKILL.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-onboarding/SKILL.md

curl -o ~/.claude/skills/codecov-config/CONFIG.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-config/CONFIG.md

# C/C++ projects — also install this:
mkdir -p ~/.claude/skills/c-cpp-coverage
curl -o ~/.claude/skills/c-cpp-coverage/SKILL.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/c-cpp-coverage/SKILL.md

# E2E coverage (coverport) — also install this:
mkdir -p ~/.claude/skills/coverport-integration
curl -o ~/.claude/skills/coverport-integration/SKILL.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/coverport-integration/SKILL.md

# Quick config-only rollout — adds .codecov.yml and opens a PR:
mkdir -p ~/.claude/skills/add-codecov-yml
curl -o ~/.claude/skills/add-codecov-yml/skill.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/add-codecov-yml/skill.md

# Rollout tracking — bulk-refresh PR statuses in a Google Sheet:
mkdir -p ~/.claude/skills/refresh-codecov-sheet
curl -o ~/.claude/skills/refresh-codecov-sheet/skill.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/refresh-codecov-sheet/skill.md
```

#### Verify installation

In Claude Code, type `/skills` to see installed skills:

```
 User skills (~/.claude/skills)
 codecov-onboarding · Onboard repositories to Codecov...
 c-cpp-coverage     · Generate code coverage for C/C++ projects...
```

#### Usage

1. Navigate to your project repository
2. Start Claude Code and ask:

```
I want to onboard my repository to Codecov for code coverage
```

3. Follow the prompts — Claude will ask about your repo, analyze it,
   and make the changes.

---

### Option B: Cursor

**Requirements:** [Cursor](https://cursor.sh) installed

#### Setup (one-time)

Choose global (all projects) or per-project installation:

**Global installation (recommended):**

```bash
# Core skills
mkdir -p ~/.cursor/skills-cursor/{codecov-onboarding,codecov-config}

curl -o ~/.cursor/skills-cursor/codecov-onboarding/SKILL.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-onboarding/SKILL.md

curl -o ~/.cursor/skills-cursor/codecov-config/CONFIG.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-config/CONFIG.md

# C/C++ projects — also install this:
mkdir -p ~/.cursor/skills-cursor/c-cpp-coverage
curl -o ~/.cursor/skills-cursor/c-cpp-coverage/SKILL.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/c-cpp-coverage/SKILL.md

# E2E coverage (coverport) — also install this:
mkdir -p ~/.cursor/skills-cursor/coverport-integration
curl -o ~/.cursor/skills-cursor/coverport-integration/SKILL.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/coverport-integration/SKILL.md

# Quick config-only rollout:
mkdir -p ~/.cursor/skills-cursor/add-codecov-yml
curl -o ~/.cursor/skills-cursor/add-codecov-yml/skill.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/add-codecov-yml/skill.md

# Rollout tracking:
mkdir -p ~/.cursor/skills-cursor/refresh-codecov-sheet
curl -o ~/.cursor/skills-cursor/refresh-codecov-sheet/skill.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/refresh-codecov-sheet/skill.md
```

**Per-project installation:**

```bash
cd /path/to/your/project

mkdir -p .cursor/rules
curl -o .cursor/rules/codecov-onboarding.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-onboarding/SKILL.md
curl -o .cursor/rules/codecov-config.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-config/CONFIG.md

# Add c-cpp-coverage.md and/or coverport-integration.md as needed
```

#### Usage

1. Open your project in Cursor
2. Ask:

```
I want to onboard my repository to Codecov for code coverage
```

3. Follow the prompts.

---

## Examples

### Example 1: Go project on GitHub with GitHub Actions

```
You: I want to add Codecov to my Go project.

AI: I'll help you onboard to Codecov. A few questions first:
    1. Where is your repo hosted? (GitHub public/private, GitLab...)
    2. Which CI system runs your tests?
    ...

You: Public GitHub, using GitHub Actions.

AI: Great — I'll use app.codecov.io with OIDC authentication (no token
    needed). Let me analyze your repo...
    [Detects Go, finds go.mod, checks workflows]
    I'll add a codecov upload step with use_oidc: true after your
    test step. Here's the proposed change...
```

### Example 2: C/C++ project on GitLab

```
You: I need to add code coverage to my C project on gitlab.com.

AI: I see this is a C project using autotools (configure.ac found).
    I'll use the c-cpp-coverage skill for this.
    Your Codecov instance will be app.codecov.io.

    I'll need to:
    1. Add a coverage CI job with gcov/lcov
    2. Handle the _FORTIFY_SOURCE conflict with -O0
    3. Skip heavy tests that hang under coverage
    4. Create a ci/codecov.sh script for lcov report generation
    ...
```

### Example 3: Private GitHub repo

```
You: I want coverage for my private repo on GitHub.

AI: Since this is a private GitHub repo, I'll use the self-hosted
    Codecov instance at codecov-codecov-public-production.apps.rosa.appsrep11ue1...
    You'll need a CODECOV_TOKEN secret (OIDC is not available for
    self-hosted instances).
    ...
```

### Example 4: E2E coverage in GitHub Actions

```
You: I want to collect e2e coverage from my Go app in GitHub Actions.

AI: I'll use the coverport-integration skill. Since your e2e tests
    run in GitHub Actions (not Tekton), I'll set up the coverport
    CLI container via podman.

    How does your e2e test work?
    a) App deployed to a Kubernetes cluster
    b) App started locally via podman/docker in the same job
    c) Frontend app tested with Cypress

You: The app runs locally via docker compose.

AI: I'll use the --url pattern. The workflow will:
    1. Start your instrumented app (port 9095 exposed)
    2. Run your e2e tests
    3. Use `podman run coverport-cli collect --url http://localhost:9095`
    4. Use `podman run coverport-cli process` to upload to Codecov
    ...
```

## Updating Skills

To update to the latest version, re-run the `curl` commands from the
setup section. The skills are plain markdown files — new versions
overwrite the old ones.

## Questions?

If you have questions or run into issues, reach out to the Code Coverage
Workgroup.
