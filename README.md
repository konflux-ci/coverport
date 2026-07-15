# CoverPort

Multi-language coverage collection system for containerized applications running in Kubernetes. Collect code coverage from running containers via HTTP with no volume mounts or deployment modifications.

## 📦 What's Inside

- **`cli/`** - **coverport CLI** - Kubernetes-native tool for collecting coverage from running pods via port-forwarding. Supports Konflux snapshot integration, multi-component collection, and OCI artifact publishing.
- **`instrumentation/`** - Coverage HTTP servers (Go, Python, Node.js, Rust) that embed into your applications to expose coverage data via HTTP endpoint (default port 53700).

## Quick Start

```bash
go install github.com/konflux-ci/coverport/cli@latest
coverport collect --url http://localhost:53700 --test-name=local --output=./coverage-output
```

See [cli/QUICKSTART.md](cli/QUICKSTART.md) for full details.

## Development

```bash
cd cli && make build && make test
```

Requires Go 1.24+. See [cli/README.md](cli/README.md) for all commands and [cli/ARCHITECTURE.md](cli/ARCHITECTURE.md) for design details.

## Contributing

PRs welcome — target `main`, squash merge preferred. Run `pre-commit run --all-files` before pushing.
