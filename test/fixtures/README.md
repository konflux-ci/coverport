# E2E Test Fixture Images

Minimal apps used as targets for e2e testing of the `coverport` CLI and
onboarding patterns.

## Container fixtures (Kind / HTTP collection)

| Language | Image | Coverage mechanism |
|----------|-------|--------------------|
| Go | `quay.io/konflux-ci/konflux-devprod/coverport-testapp-go` | `go build -cover` + `instrumentation/go/coverage_server.go` |
| Rust | `quay.io/konflux-ci/konflux-devprod/coverport-testapp-rust` | LLVM profraw + `instrumentation/rust/` crate |

Each image exposes:
- Port **8080** — app endpoint (`/hello?name=...`)
- Port **53700** — coverage server (`/coverage`, `/health`)

Rust `process` also needs the instrumented binary from the image (`/testapp`).
E2E extracts it with `docker/podman create` + `cp` and sets `COVERAGE_BINARY`.

## Node.js (Pattern C — NYC filesystem process)

Node.js follows the coverport-integration skill's Pattern C: feed Istanbul/NYC
JSON to `coverport process --format=nyc`. There is no supported
`coverport collect` path for Node HTTP responses (format field collides with
Python).

The Kind image `coverport-testapp-nodejs` is only used by
`TestProcessNodejsFilesystem` to obtain Istanbul JSON, write
`coverage-final.json`, and run `process`.

## Python (Pattern D — pytest-cov, no container)

Python follows Pattern D: run `pytest` with `--cov` against source and upload
the XML report. No instrumented container and no coverport `collect`/`process`.

```
test/fixtures/python/
├── app.py
├── test_app.py
└── requirements.txt
```

Covered by `TestPythonPytestCov` in `test/e2e`.

## Building and pushing (container fixtures)

All builds must run from the **repo root** since Dockerfiles reference both
`test/fixtures/` and `instrumentation/`.

```bash
cd /path/to/coverport

# Go
podman build -f test/fixtures/go/Dockerfile -t quay.io/konflux-ci/konflux-devprod/coverport-testapp-go:latest .
podman push quay.io/konflux-ci/konflux-devprod/coverport-testapp-go:latest

# Node.js (Pattern C process test only)
podman build -f test/fixtures/nodejs/Dockerfile -t quay.io/konflux-ci/konflux-devprod/coverport-testapp-nodejs:latest .
podman push quay.io/konflux-ci/konflux-devprod/coverport-testapp-nodejs:latest

# Rust
podman build -f test/fixtures/rust/Dockerfile -t quay.io/konflux-ci/konflux-devprod/coverport-testapp-rust:latest .
podman push quay.io/konflux-ci/konflux-devprod/coverport-testapp-rust:latest
```

## When to rebuild

Rebuild and push updated images when:
- The fixture app code changes (`test/fixtures/<lang>/`)
- The instrumentation server code changes (`instrumentation/<lang>/`)

The images are pinned to `:latest` — there is no automated build pipeline for these.
Manual rebuild and push is intentional to keep things simple.

## Running locally

```bash
# Example: run the Go fixture locally
podman run --rm -p 8080:8080 -p 53700:53700 quay.io/konflux-ci/konflux-devprod/coverport-testapp-go:latest

# Hit the app to generate coverage
curl http://localhost:8080/hello?name=test

# Collect coverage
curl http://localhost:53700/coverage

# Python Pattern D (no container)
cd test/fixtures/python
pip install -r requirements.txt
pytest --cov=app --cov-report=xml --cov-report=term --cov-branch
```
