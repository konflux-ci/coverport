# CoverPort

Multi-language coverage collection system for containerized applications in Kubernetes.
Collects code coverage from running containers via HTTP (port 53700) without volume mounts or deployment changes.

## Build & Test

```bash
cd cli && make build          # build coverport binary
cd cli && make test           # go test -v ./...
cd instrumentation/go && go test ./... -v -cover
```


## Code Layout

- `cli/` — Go CLI (cobra): `cmd/` commands, `internal/` business logic, `pkg/client/` HTTP/K8s client
- `instrumentation/` — Coverage HTTP servers: `go/`, `python/`, `nodejs/`
- `.claude/skills/` — Agent skills for coverage onboarding workflows

## Conventions

- Go 1.24, modules at `github.com/konflux-ci/coverport/cli`
- Instrumentation servers: zero external deps (stdlib or single lib), expose `/coverage` on port 53700
- CLI subcommands: `collect`, `discover`, `process` — all accept `--verbose` flag
- Container images built via Konflux Tekton pipelines on push to `main`
- Coverage uploaded to Codecov with `unit-tests` flag via OIDC

## Don't

- Don't add external dependencies to instrumentation servers (keep them copy-paste embeddable)
- Don't modify `.tekton/` YAML without understanding Konflux PaC conventions
- Don't change `.golangci.yml` without running `golangci-lint run ./...` locally first
- Don't add `go.work` files (components have separate modules intentionally)

## Pattern References

- Add CLI subcommand: follow `cli/cmd/collect.go` pattern (cobra command + internal package)
- Add instrumentation language: follow `instrumentation/go/coverage_server.go` (HTTP server, `/coverage` endpoint)
- Add Tekton task: follow `cli/examples/tekton-task-coverport.yaml`
- Add agent skill: follow `.claude/skills/codecov-onboarding/SKILL.md` (YAML frontmatter + instructions)
- Add unit test: follow `cli/internal/discovery/discovery_test.go` (table-driven tests)

## Review

This file should be reviewed quarterly. Last reviewed: 2026-06.
