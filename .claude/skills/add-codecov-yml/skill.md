---
name: add-codecov-yml
description: Use when asked to add, create, or generate a .codecov.yml file for a repository and open a PR or MR. Supports GitHub and GitLab repositories. Triggers on requests like "add codecov config", "create codecov yml", "set up codecov for repo X", "add coverage config to repo".
---

# Add .codecov.yml to a Repository

## Overview

Generates a `.codecov.yml` configuration file for a GitHub or GitLab repository and opens a PR (GitHub) or MR (GitLab). Uses the Konflux standard coverage config as the baseline with customizable thresholds.

Detect the platform from the repo URL or the `platform` parameter:
- URL contains `github.com` → GitHub; use `gh` CLI
- URL contains a GitLab hostname (e.g. `gitlab.com`, `gitlab.cee.redhat.com`) → GitLab; use `glab` CLI

## Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `repo` | **yes** | — | GitHub repo in `org/name` format **or** full GitLab HTTPS URL |
| `platform` | no | auto-detect from URL | `github` or `gitlab`; overrides URL-based detection |
| `project_target` | no | `auto` | Project coverage target (`auto` tracks current, or a percentage like `80%`) |
| `project_threshold` | no | `1%` | Allowed drop below project target |
| `patch_target` | no | `80%` | Minimum coverage for changed lines |
| `patch_threshold` | no | — | Not used; patch status is `informational: true` (won't fail CI) |

## Step-by-Step

### 1. Clone the repo

**GitHub:**
```bash
REPO="org/repo-name"  # from user input (org/name format)
WORKDIR=$(mktemp -d)
gh repo clone "$REPO" "$WORKDIR/repo" -- --depth 1
cd "$WORKDIR/repo"
```

**GitLab:**
```bash
REPO_URL="https://gitlab.cee.redhat.com/group/repo"  # full HTTPS URL
WORKDIR=$(mktemp -d)
git clone --depth 1 "$REPO_URL" "$WORKDIR/repo"
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
| CI gate | `codecov.require_ci_to_pass` | `false` or `true` (any explicit value) | missing |
| Patch threshold | `coverage.status.patch.default.target` | `>= 80%` | missing or below `80%` |

**Note on `require_ci_to_pass`:** If a team has explicitly set `require_ci_to_pass: true`, **leave it as `true`**. Only set it to `false` when the field is missing entirely. Teams that chose `true` did so intentionally.

Parse the existing file and run both checks. Report results to the user:
- **Both pass**: report "Codecov config is compliant" and stop — no PR needed.
- **Any fail**: show what's non-compliant, fix the values in-place, and open a PR with the fix (see step 4). Use commit message `Fix .codecov.yml compliance` and PR title `Fix .codecov.yml: disable CI gate and enforce minimum coverage threshold`.

**IMPORTANT — Preserve existing sections.** When rewriting the config, carry forward these blocks verbatim from the original file if they exist:
- **`codecov.require_ci_to_pass`** — if set to `true`, keep it `true`
- **`ignore:`** — excluded paths (generated files, mocks, vendor dirs, test data)
- **`flags:`** — flag definitions with `carryforward: true` and `paths` filters
- **YAML comments** — inline comments (`# generated file`), section comments (`# Allows coverage to drop...`), and documentation links (`# Documentation: https://...`)
- **File-level comments** — license headers (e.g. ASF/Apache headers) at the top of the file

Teams set these intentionally. Do not remove or modify them.

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

### 4. Create branch, commit, and open PR/MR

The branch creation and commit steps are the same on both platforms:

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
```

Then open the PR or MR using the platform-specific steps below.

**GitHub — open PR with `gh`:**
```bash
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

**GitLab — open MR with `curl`:**

Requires `GITLAB_TOKEN` (a personal access token with `api` scope) and the GitLab host URL.

```bash
GITLAB_HOST="https://gitlab.cee.redhat.com"   # adjust to your instance
PROJECT_PATH=$(git remote get-url origin \
  | sed 's|.*'"$GITLAB_HOST"'/||;s|\.git$||' \
  | python3 -c "import sys,urllib.parse; print(urllib.parse.quote(sys.stdin.read().strip(), safe=''))")
DEFAULT_BRANCH=$(git symbolic-ref refs/remotes/origin/HEAD | sed 's|refs/remotes/origin/||')

curl -s -X POST \
  --header "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  --header "Content-Type: application/json" \
  --data "{
    \"source_branch\": \"$BRANCH\",
    \"target_branch\": \"$DEFAULT_BRANCH\",
    \"title\": \"Add .codecov.yml configuration\",
    \"description\": \"## Summary\n\n- Adds \`.codecov.yml\` with standard Konflux coverage settings\n- Project coverage target: <project_target> (threshold: <project_threshold>)\n- Patch coverage target: <patch_target>\n\n## Reference\n\nBased on [konflux-ui/.codecov.yml](https://github.com/konflux-ci/konflux-ui/blob/main/.codecov.yml)\",
    \"remove_source_branch\": true
  }" \
  "$GITLAB_HOST/api/v4/projects/$PROJECT_PATH/merge_requests" \
  | python3 -c "import json,sys; r=json.load(sys.stdin); print(r.get('web_url', r))"
```

The last line prints the MR URL. If `GITLAB_TOKEN` is not set, open the MR manually: in the GitLab web UI, navigate to the repository → Merge Requests → New merge request → select branch `add-codecov-yml` and submit with the title and description above.

### 5. Report back

Print the PR/MR URL so the user can review it.

### 6. Cleanup

```bash
rm -rf "$WORKDIR"
```

## Common Issues

- **GitHub — fork required**: If the user doesn't have push access, fork first with `gh repo fork "$REPO" --clone`
- **GitLab — fork required**: If the user doesn't have push access, fork via the API:
  ```bash
  curl -s -X POST \
    --header "PRIVATE-TOKEN: $GITLAB_TOKEN" \
    "$GITLAB_HOST/api/v4/projects/<url-encoded-path>/fork" \
    | python3 -c "import json,sys; r=json.load(sys.stdin); print(r.get('http_url_to_repo',''))"
  ```
  Clone the fork URL from the output, add the original as `upstream`, then open the MR targeting the upstream project ID using the `merge_requests` API endpoint above.
- **Branch already exists**: Append a timestamp — `add-codecov-yml-$(date +%s)`
- **Go repos**: The default `ignore` pattern targets TypeScript test data (`**/*__data__*/*.ts`). For Go repos, ask the user if they want to adjust ignore patterns (e.g. `**/testdata/**`, `**/*_test.go`)
