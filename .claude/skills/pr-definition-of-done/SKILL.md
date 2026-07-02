---
name: pr-definition-of-done
description: >-
  Checklist for merge-ready PRs in coverport. Use when preparing a PR for review
  or verifying a PR meets all requirements before merging.
---

# PR Definition of Done

## Pre-Submit Checklist

- [ ] Run `pre-commit run --all-files` (formatting, trailing whitespace, YAML validation)
- [ ] All existing tests pass: `cd cli && make test`
- [ ] New code has unit tests (table-driven, follow existing patterns)
- [ ] `golangci-lint run ./...` passes for modified Go modules (same linters as CI)
- [ ] Code compiles cleanly: `cd cli && go build ./...`
- [ ] If CLI flags changed: update `cli/README.md` command reference
- [ ] If new dependency added: justify in PR description (instrumentation must stay zero-dep)
- [ ] Coverage does not decrease (Codecov patch check is informational, not blocking)

## CI Checks That Must Pass

| Check | Trigger | Required |
|-------|---------|----------|
| CLI Tests (Go 1.24) | All PRs | Yes |
| Instrumentation Tests (Go matrix) | All PRs | Yes |
| Lint (golangci-lint + go build) | All PRs | Yes |
| AGENTS.md line limit | All PRs | Yes |
| Konflux build (coverport-cli) | PRs touching `cli/**` | Yes |
| Codecov coverage | All PRs | Informational |

## CI Quirks

- **Konflux Tekton build** only triggers when files under `cli/` change (path filter in
  `.tekton/coverport-cli-pull-request.yaml`). Changes outside `cli/` won't get a container build.
- **Codecov** uses OIDC — no token needed, but `id-token: write` permission must be present.
- **golangci-lint** runs in CI via `golangci-lint-action@v9` with v2.12, using per-module `.golangci.yml` configs.
- **PR images expire after 5 days** — built with tag `on-pr-{revision}` in RedHat user workloads.

## PR Description Guidelines

- Explain **why** the change is needed, not just what changed
- Reference any related Jira tickets or GitHub issues
- For new CLI commands: include example usage in description
- For instrumentation changes: note which languages are affected

## Merge Strategy

- Squash merge preferred for feature branches
- Rebase merge for multi-commit PRs where history matters
- Target branch: `main`
