# E2E Test Fixture Images

Minimal apps used as targets for e2e testing of the `coverport` CLI and
onboarding patterns.

## Container fixtures (Kind / HTTP collection)

| Language | Image | Coverage mechanism |
|----------|-------|--------------------|
| Go | `quay.io/konflux-ci/konflux-devprod/coverport-testapp-go` | `go build -cover` + `instrumentation/go/coverage_server.go` |
| Rust | `quay.io/konflux-ci/konflux-devprod/coverport-testapp-rust` | LLVM profraw + `instrumentation/rust/` crate |
| Node.js | `quay.io/konflux-ci/konflux-devprod/coverport-testapp-nodejs` | V8 inspector + Istanbul JSON via `instrumentation/nodejs/coverage_server.js` |
| Python | `quay.io/konflux-ci/konflux-devprod/coverport-testapp-python` | Gunicorn + `instrumentation/python/` (`coverage_server.py`, sitecustomize, `.coveragerc`) |

Each image exposes:
- Port **8080** — app endpoint (`/hello?name=...`)
- Port **53700** — coverage server (`/coverage`, `/health`)

Rust `process` also needs the instrumented binary from the image (`/testapp`).
E2E extracts it with `docker/podman create` + `cp` and sets `COVERAGE_BINARY`.

## Node.js (HTTP collection and Pattern C)

Node.js supports HTTP collection through `coverport collect`, which writes the
decoded Istanbul payload as `coverage-final.json`. Pattern C remains supported:
feed existing Istanbul/NYC JSON to `coverport process --format=nyc`.

The Kind image `coverport-testapp-nodejs` is used by `TestCollectNodejs` for the
HTTP path and by `TestProcessNodejsFilesystem` for Pattern C.

## Python (Kind/HTTP and Pattern D)

Python has two e2e paths:

1. **Kind/HTTP container** — Gunicorn Flask app (`wsgi.py`) with
   `instrumentation/python/`. Covered by `TestCollectPython` /
   `TestProcessPython` (`coverport collect` → `coverport process --format python`).
2. **Pattern D (pytest-cov)** — run `pytest` with `--cov` against source and
   upload the XML report. No Kind pod and no coverport `collect`/`process`.
   Covered by `TestPythonPytestCov`.

```
test/fixtures/python/
├── app.py              # shared greet() used by pytest and the Flask app
├── wsgi.py             # Flask WSGI entry for the container fixture
├── test_app.py         # Pattern D pytest
├── requirements.txt    # pytest / pytest-cov (Pattern D only)
└── Dockerfile          # Gunicorn + instrumentation/python/
```

## Building and pushing (container fixtures)

All builds must run from the **repo root** since Dockerfiles reference both
`test/fixtures/` and `instrumentation/`.

```bash
cd /path/to/coverport

# Go
podman build -f test/fixtures/go/Dockerfile -t quay.io/konflux-ci/konflux-devprod/coverport-testapp-go:latest .
podman push quay.io/konflux-ci/konflux-devprod/coverport-testapp-go:latest

# Node.js
podman build -f test/fixtures/nodejs/Dockerfile -t quay.io/konflux-ci/konflux-devprod/coverport-testapp-nodejs:latest .
podman push quay.io/konflux-ci/konflux-devprod/coverport-testapp-nodejs:latest

# Rust
podman build -f test/fixtures/rust/Dockerfile -t quay.io/konflux-ci/konflux-devprod/coverport-testapp-rust:latest .
podman push quay.io/konflux-ci/konflux-devprod/coverport-testapp-rust:latest

# Python
podman build -f test/fixtures/python/Dockerfile -t quay.io/konflux-ci/konflux-devprod/coverport-testapp-python:latest .
podman push quay.io/konflux-ci/konflux-devprod/coverport-testapp-python:latest
```

If Quay credentials are unavailable, build and load locally instead:

```bash
podman build -f test/fixtures/python/Dockerfile -t quay.io/konflux-ci/konflux-devprod/coverport-testapp-python:latest .
kind load docker-image quay.io/konflux-ci/konflux-devprod/coverport-testapp-python:latest
# or: kind load image-archive <(podman save ...)
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

# Python container fixture
podman run --rm -p 8080:8080 -p 53700:53700 quay.io/konflux-ci/konflux-devprod/coverport-testapp-python:latest
curl http://localhost:8080/hello?name=test
curl http://localhost:53700/health
curl http://localhost:53700/coverage/save
curl http://localhost:53700/coverage

# Python Pattern D (no container)
cd test/fixtures/python
pip install -r requirements.txt
pytest --cov=app --cov-report=xml --cov-report=term --cov-branch
```
