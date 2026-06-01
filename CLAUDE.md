# CoverPort

CoverPort is a Kubernetes-native coverage collection tool designed for Konflux CI/CD pipelines.
It instruments running containers to expose coverage data via HTTP, then collects, processes,
and uploads that data to Codecov or SonarCloud. The primary artifact is the `coverport` CLI,
which discovers pods by image reference, port-forwards to collect coverage, and publishes
results as OCI artifacts.

## Stack

- **CLI**: Go 1.24, Cobra, client-go (k8s), oras-go (OCI)
- **Instrumentation**: Go 1.21+ (stdlib), Python 3 (coverage.py), Node.js (V8 inspector)
- **CI**: GitHub Actions (tests/lint), Konflux Tekton (container builds)
- **Container base**: UBI9 minimal + Go 1.24, oras 1.2.0, cosign 2.4.1
- **Coverage**: Codecov (OIDC upload), SonarCloud (via coverage-processor)

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
└── nodejs/           coverage_server.js — V8 inspector + v8-to-istanbul

coverage-processor/
├── tekton/           EventListener, TriggerBinding, coverage task
├── k8s/              Namespace, RBAC, gosmee client config
└── deploy.sh         One-shot deployment script
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

# Run locally
./coverport collect --url http://localhost:53700 --test-name=local --output=./coverage-output
./coverport discover --namespace=my-ns --image=quay.io/org/app:latest
./coverport process --input=./coverage-output --codecov-token=$TOKEN

# Container build
cd cli && make docker-build

```

## Design Choices

- **Separate Go modules**: `cli/` and `instrumentation/go/` are independent modules to allow
  instrumentation to stay on older Go versions (1.21+) while CLI tracks latest.
- **Zero-dep instrumentation**: Instrumentation servers must remain copy-paste embeddable into
  any project; no external dependencies allowed.
- **Port 53700**: Chosen as a high, unlikely-to-conflict port; hardcoded across all languages.
- **OCI artifacts for coverage**: Coverage data is pushed to container registries (not stored in
  git or ephemeral CI storage) so it persists and is addressable.
- **Konflux PaC**: Tekton pipelines in `.tekton/` are managed by Konflux Pipeline-as-Code;
  changes trigger automated rebuilds via push/PR events.
- **OIDC for Codecov**: No tokens stored in repo; CI uses OpenID Connect for upload auth.

## Pitfalls

- `golangci-lint` is in the Makefile but NOT in CI — CI only runs `go vet`. Don't assume
  lint passes locally means CI will pass.
- `QUICKSTART.md` references `URL_COLLECTION.md` and `MANIFEST_WORKFLOW.md` which don't exist
  in the repo — these are aspirational docs.
- Python and Node.js instrumentation have NO tests and NO dependency manifests in-repo;
  they're designed to be copied into consumer projects.
- The `coverage-processor/deploy.sh` requires an active OpenShift session and Smee.io channel;
  it will fail silently if prerequisites aren't met.
- Tekton PipelineRuns reference specific Konflux catalog tasks that may change versions
  upstream without notice.
- No root `.gitignore` — only `cli/.gitignore` and `coverage-processor/.gitignore` exist.
