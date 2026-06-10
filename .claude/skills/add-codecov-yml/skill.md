---
name: add-codecov-yml
description: Use when asked to add, create, or generate a .codecov.yml file for a repository and open a PR. Triggers on requests like "add codecov config", "create codecov yml", "set up codecov for repo X", "add coverage config to repo".
---

# Add .codecov.yml to a Repository

## Overview

Generates a `.codecov.yml` configuration file for a GitHub repository and opens a PR. Uses the Konflux standard coverage config as the baseline with customizable thresholds.

## Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `repo` | **yes** | — | GitHub repo in `org/name` format (e.g. `konflux-ci/build-service`) |
| `project_target` | no | `auto` | Project coverage target (`auto` tracks current, or a percentage like `80%`) |
| `project_threshold` | no | `1%` | Allowed drop below project target |
| `patch_target` | no | `80%` | Minimum coverage for changed lines |
| `patch_threshold` | no | — | Not used; patch status is `informational: true` (won't fail CI) |

## Step-by-Step

### 1. Clone the repo

```bash
REPO="org/repo-name"  # from user input
WORKDIR=$(mktemp -d)
gh repo clone "$REPO" "$WORKDIR/repo" -- --depth 1
cd "$WORKDIR/repo"
```

### 2. Check for existing config

```bash
EXISTING=$(find . -maxdepth 1 -name "codecov.y*ml" -o -name ".codecov.y*ml" | head -1)
```

If a config file exists, **do not create a new one**. Instead, validate it against these compliance rules:

**Compliance checks (both are required):**

| Rule | Check | Pass | Fail |
|------|-------|------|------|
| CI gate | `codecov.require_ci_to_pass` | `false` | missing or `true` |
| Patch threshold | `coverage.status.patch.default.target` | `>= 80%` | missing or below `80%` |

Parse the existing file and run both checks. Report results to the user:
- **Both pass**: report "Codecov config is compliant" and stop — no PR needed.
- **Any fail**: show what's non-compliant, fix the values in-place, and open a PR with the fix (see step 4). Use commit message `Fix .codecov.yml compliance` and PR title `Fix .codecov.yml: disable CI gate and enforce minimum coverage threshold`.

### 3. Generate `.codecov.yml`

Write the file using the parameter values (or defaults). The template:

```yaml
codecov:
  require_ci_to_pass: false

coverage:
  status:
    project:
      default:
        target: <project_target>
        threshold: <project_threshold>
        informational: true
    patch:
      default:
        target: <patch_target>
        informational: true

comment:
  layout: "reach,diff,flags,files,footer"
  require_changes: true
```

Replace `<project_target>`, `<project_threshold>`, `<patch_target>`, `<patch_threshold>` with the actual values.

### 4. Create branch, commit, and open PR

```bash
BRANCH="add-codecov-yml"
git checkout -b "$BRANCH"
git add .codecov.yml
git commit -m "Add .codecov.yml configuration

Adds Codecov coverage configuration with:
- Project target: <project_target> (threshold: <project_threshold>)
- Patch target: <patch_target> (threshold: <patch_threshold>)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"

git push -u origin "$BRANCH"

gh pr create \
  --title "Add .codecov.yml configuration" \
  --body "$(cat <<'EOF'
## Summary

- Adds `.codecov.yml` with standard Konflux coverage settings
- Project coverage target: <project_target> (threshold: <project_threshold>)
- Patch coverage target: <patch_target> (threshold: <patch_threshold>)

## Reference

Based on [konflux-ui/.codecov.yml](https://github.com/konflux-ci/konflux-ui/blob/main/.codecov.yml)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

### 5. Report back

Print the PR URL so the user can review it.

### 6. Cleanup

```bash
rm -rf "$WORKDIR"
```

## Common Issues

- **Fork required**: If the user doesn't have push access, fork first with `gh repo fork "$REPO" --clone`
- **Branch already exists**: Append a timestamp — `add-codecov-yml-$(date +%s)`
- **Go repos**: The default `ignore` pattern targets TypeScript test data (`**/*__data__*/*.ts`). For Go repos, ask the user if they want to adjust ignore patterns (e.g. `**/testdata/**`, `**/*_test.go`)
