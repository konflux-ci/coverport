# Codecov Onboarding Skill - Quick Start Guide

We've added a new AI skill to help you onboard your repository to Codecov for unit test coverage tracking.

**Skill location:** https://github.com/konflux-ci/coverport/blob/main/.claude/skills/codecov-onboarding/SKILL.md

## What It Does

The skill guides you through:
- Analyzing your repository's test setup (discovering test types like unit, integration, etc.)
- Verifying tests actually run with coverage locally before touching CI
- Configuring CI to run tests with coverage and upload to Codecov
- Automatically handling multiple test types with separate Codecov flags
- Integrating with your CI system (GitHub Actions, OpenShift CI/Prow, or GitLab CI)
- Establishing a main branch baseline for accurate PR coverage diffs

## Prerequisites

Before starting, make sure you have:
- ✅ Tests already in your repository (unit, integration, or both)
- ✅ A Codecov account (sign up at https://app.codecov.io)
- ✅ Your Codecov upload token ready

## What You're Installing

This guide helps you install an AI skill - a specialized workflow that teaches the AI how to help you onboard repositories to Codecov. Once installed, it will automatically recognize when you're asking about Codecov and use this skill to guide you.

---

## Option A: Claude Code

**Requirements:**
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) installed
- Minimum version: **2.1.39** (check with `claude --version`)

### Setup (One-Time)

```bash
# Create the skills directory if it doesn't exist
mkdir -p ~/.claude/skills/codecov-onboarding

# Download the skill
curl -o ~/.claude/skills/codecov-onboarding/SKILL.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-onboarding/SKILL.md
```

### Verify Installation

In Claude Code, type `/skills` to verify the skill is available:

```
 User skills (~/.claude/skills)
 codecov-onboarding · ~72 description tokens
```

### Usage

1. **Navigate to your project repository** where you want to add Codecov

2. **Start Claude Code** and simply ask:
   ```
   I want to onboard my repository to Codecov for unit test coverage using the codecov-onboarding skill.
   ```

3. **Follow the prompts** - Claude will:
   - Ask about your Codecov account and CI system
   - Analyze your repository structure
   - Guide you through local testing first
   - Propose CI configuration changes
   - Help troubleshoot any issues

---

## Option B: Cursor

**Requirements:** [Cursor](https://cursor.sh) installed

### Setup (One-Time)

Choose one of these options:

**Option B1: Add to your project (recommended for team sharing)**
```bash
# Navigate to your project directory
cd /path/to/your/project

# Create rules directory and download skill
mkdir -p .cursor/rules
curl -o .cursor/rules/codecov-onboarding.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-onboarding/SKILL.md
```

**Option B2: Add globally (available in all projects)**
```bash
# Check Cursor docs for your OS-specific global rules location
# Typically: ~/.cursor/rules/ or similar
mkdir -p ~/.cursor/rules
curl -o ~/.cursor/rules/codecov-onboarding.md \
  https://raw.githubusercontent.com/konflux-ci/coverport/main/.claude/skills/codecov-onboarding/SKILL.md
```

### Usage

1. **Open your project in Cursor**

2. **Ask Cursor:**
   ```
   I want to onboard my repository to Codecov for unit test coverage using the codecov-onboarding skill
   ```

3. **Follow the prompts** - Cursor will guide you through the onboarding process.

---

## Quick Example

```
You: I want to add Codecov to my Go project that uses OpenShift CI for testing.

AI: I'll help you onboard your repository to Codecov. Let me start with a few questions:

1. Have you signed up at https://app.codecov.io?
2. Do you have your Codecov upload token ready?
3. Do you already have any partial Codecov setup?

[The AI will then analyze your repo and guide you step-by-step]
```

## Key Features

| Feature | Description |
|---------|-------------|
| **Test Type Discovery** | Detects unit, integration, and other test types automatically |
| **Local Verification** | Verifies tests pass with coverage before configuring CI |
| **Multi-Test-Type Support** | Configures separate Codecov flags per test type |
| **Multi-CI Support** | GitHub Actions, OpenShift CI (Prow), GitLab CI |
| **Partial Setup Handling** | Detects and updates existing Codecov config |
| **Troubleshooting** | Built-in solutions for common issues |

## Related Skills

- **coverport-integration** - For e2e test coverage collection (requires instrumentation)
  - Location: https://github.com/konflux-ci/coverport/blob/main/.claude/skills/coverport-integration/SKILL.md

## Questions?

If you have questions or run into issues, reach out to the Code Coverage Workgroup.
