# CoverPort

CoverPort is a Kubernetes-native coverage collection tool designed for Konflux CI/CD pipelines.
It instruments running containers to expose coverage data via HTTP, then collects, processes,
and uploads that data to Codecov or SonarCloud. The primary artifact is the `coverport` CLI,
which discovers pods by image reference, port-forwards to collect coverage, and publishes
results as OCI artifacts.

## Stack

- **CLI**: Go 1.24, Cobra, client-go (k8s), oras-go (OCI)
- **Instrumentation**: Go 1.21+ (stdlib), Python 3 (coverage.py), Node.js (V8 inspector)
- **CI**: GitHub Actions (unit tests/lint + Kind e2e), Konflux Tekton (container builds)
- **Container base**: UBI9 minimal + Go 1.24, oras 1.2.0, cosign 2.4.1
- **Coverage**: Codecov (OIDC upload)

## Code Layout

```
cli/
├── cmd/              Cobra commands (collect, discover, process, root)
├── internal/
│   ├── discovery/    Pod discovery by image reference
│   ├── snapshot/     Konflux snapshot parsing
│   ├── manifest/     Coverage manifest handling
│   ├── metadata/     Git/OCI metadata extraction
│   ├── processor/    Coverage processing and remapping
│   ├── upload/       Codecov upload logic
│   └── git/          Git operations
├── pkg/client/       Reusable HTTP + K8s coverage client
├── examples/         Tekton tasks, pipeline YAML, usage scripts
├── Makefile          Build, test, lint, docker targets
└── Dockerfile        Multi-stage UBI9 build

instrumentation/
├── go/               coverage_server.go — stdlib HTTP server, zero deps
├── python/           coverage_server.py — coverage.py wrapper + Gunicorn
├── nodejs/           coverage_server.js — V8 inspector + v8-to-istanbul
└── rust/             coverage-server crate — axum HTTP server, LLVM profraw via FFI

test/
├── e2e/              Kind-based CLI e2e suite (collect/discover/process + failure paths)
└── fixtures/         Per-language test apps / images; see test/fixtures/README.md
```

## Build / Test / Run

```bash
# Daily dev
cd cli
make build                    # produces ./coverport binary
make test                     # go test -v ./...
make lint                     # golangci-lint (install separately)
make dev-build                # build with -race

# CI-equivalent (what GitHub Actions runs)
cd cli && go test ./... -v -count=1 -race -coverprofile=coverage.out -covermode=atomic
cd instrumentation/go && go test ./... -v -count=1 -cover -coverprofile=coverage.out

# E2E (Kind) — also .github/workflows/e2e.yml
# Needs: kind/kubectl, docker or podman, fixture images loaded into the cluster,
# Rust 1.80 + llvm-tools-preview (for Rust process), pip install -r test/fixtures/python/requirements.txt
cd cli && go build -cover -o ./coverport-cover .
cd test/e2e
COVERPORT_BIN=$(pwd)/../../cli/coverport-cover go test -v -timeout 25m ./...

# Run locally
./coverport collect --url http://localhost:53700 --test-name=local --output=./coverage-output
./coverport discover --namespace=my-ns --images=quay.io/org/app:latest
./coverport process --input=./coverage-output --codecov-token=$TOKEN

# Container build
cd cli && make docker-build

```

## E2E coverage

- **Go / Rust**: Kind pods + HTTP `collect`/`process`. Images:
  `quay.io/konflux-ci/konflux-devprod/coverport-testapp-{go,rust}`.
  Rust `process` extracts `/testapp` from the image and sets `COVERAGE_BINARY`.
- **Node.js**: Pattern C only — `TestProcessNodejsFilesystem` (`process --format=nyc`);
  no HTTP `collect` (format collides with Python). Uses `coverport-testapp-nodejs`.
- **Python**: Pattern D only — `TestPythonPytestCov` (`pytest --cov` on
  `test/fixtures/python/`); no Kind image / coverport CLI path.
- Fixture rebuild/push instructions: `test/fixtures/README.md`.

## Design Choices

- **Separate Go modules**: `cli/` and `instrumentation/go/` are independent modules to allow
  instrumentation to stay on older Go versions (1.21+) while CLI tracks latest.
- **Zero-dep instrumentation**: Instrumentation servers must remain copy-paste embeddable into
  any project; no external dependencies allowed. Exception: Rust requires axum/tokio since
  the stdlib has no HTTP server.
- **Port 53700**: Chosen as a high, unlikely-to-conflict port; hardcoded across all languages.
- **OCI artifacts for coverage**: Coverage data is pushed to container registries (not stored in
  git or ephemeral CI storage) so it persists and is addressable.
- **Konflux PaC**: Tekton pipelines in `.tekton/` are managed by Konflux Pipeline-as-Code;
  changes trigger automated rebuilds via push/PR events.
- **OIDC for Codecov**: No tokens stored in repo; CI uses OpenID Connect for upload auth.

## Pitfalls

- `golangci-lint` runs in CI via `golangci-lint-action@v9` (v2.12). Both `cli/` and
  `instrumentation/go/` have `.golangci.yml` configs.
- `QUICKSTART.md` references `URL_COLLECTION.md` and `MANIFEST_WORKFLOW.md` which don't exist
  in the repo — these are aspirational docs.
- Python, Node.js, and Rust instrumentation packages have no unit tests in-repo;
  they're copy-paste/embed targets. CLI behavior for those languages is covered by
  `test/e2e` (Kind + pattern-specific fixtures), not by tests under `instrumentation/`.
- E2E fixture images are `:latest` and rebuilt manually — if instrumentation or
  fixture app code changes, rebuild/push per `test/fixtures/README.md` or Kind
  will still run against stale Quay images.
- Tekton PipelineRuns reference specific Konflux catalog tasks that may change versions
  upstream without notice.
- Root `.gitignore` covers Go, Rust, Python, Node, and IDE artifacts.
