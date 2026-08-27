# Python container instrumentation

HTTP-based coverage collection for Python web apps (Flask, Django, FastAPI) running behind
Gunicorn in a container. Exposes combined multiprocess coverage on port **53700** for the
coverport CLI.

## Files

| File | Purpose |
|------|---------|
| `coverage_server.py` | Wrapper: starts HTTP server, runs app via Gunicorn |
| `sitecustomize.py` | Auto-starts `coverage.process_startup()` in every process |
| `.coveragerc` | Multiprocess config; writes to `/dev/shm` |
| `gunicorn_coverage.py` | Gunicorn hooks — `worker_exit` saves worker coverage |

Copy all four into your application repo (e.g. `server/`) and reference them from your
Dockerfile. See [coverport-integration skill](../../.claude/skills/coverport-integration/SKILL.md)
Steps 3–5 (Python) for full CI integration.

## Dockerfile (instrumented `test` stage)

```dockerfile
FROM python:3.12-slim AS base
WORKDIR /app
RUN pip install --no-cache-dir flask gunicorn coverage
COPY app.py .

FROM base AS production
CMD ["gunicorn", "-b", "0.0.0.0:8080", "-w", "2", "app:app"]

FROM base AS test
COPY server/sitecustomize.py /tmp/sitecustomize.py
RUN SITE_PACKAGES=$(python -c "import site; print(site.getsitepackages()[0])") && \
    cp /tmp/sitecustomize.py "$SITE_PACKAGES/sitecustomize.py"
COPY server/.coveragerc /app/.coveragerc
COPY server/gunicorn_coverage.py /opt/gunicorn_coverage.py
COPY server/coverage_server.py /opt/coverage_server.py
ENV COVERAGE_PROCESS_START=/app/.coveragerc
ENV COVERAGE_DATA_DIR=/dev/shm
ENV TMPDIR=/dev/shm
EXPOSE 8080 53700
CMD ["python", "/opt/coverage_server.py", "-m", "gunicorn", \
     "-c", "/opt/gunicorn_coverage.py", "-b", "0.0.0.0:8080", "-w", "1", "app:app"]
```

Build: `podman build --target test -t myapp:instrumented .`

Tekton (buildah-oci-ta): set `TARGET_STAGE=test` on the instrumented image build task.

## Local validation (podman)

```bash
podman build --target test -t myapp:instrumented .
podman run --rm -d --name py-cov -p 8080:8080 -p 53700:53700 myapp:instrumented

curl -s http://localhost:53700/health    # coverage_enabled: true
curl -s http://localhost:8080/             # exercise app routes
curl -s http://localhost:53700/coverage/save
curl -s http://localhost:53700/coverage    # non-empty coverage_data

podman stop py-cov
```

## Collect with coverport CLI

**Kubernetes (recommended):** `collect` port-forwards to the pod, triggers save, fetches
coverage, and generates `coverage.xml` inside the pod.

```bash
coverport collect \
  --namespace=<ns> \
  --label-selector=app=myapp \
  --test-name=e2e-tests \
  --output=./coverage-output
# → coverage-output/e2e-tests/coverage.xml
```

**Local `--url`:** `collect` saves `coverage-output/<test-name>/.coverage` only. Generate XML
on the host (coverport-cli image has no Python):

```bash
coverport collect --url http://localhost:53700 --test-name=e2e-tests --output=./coverage-output
pip install coverage
coverage xml \
  --rcfile=<paths-rcfile-with-/app-mapping> \
  --data-file=coverage-output/e2e-tests/.coverage \
  -o coverage-output/e2e-tests/coverage.xml
```

## Configuration notes

- Set `source = /app` in `.coveragerc` to match container `WORKDIR`
- `COVERAGE_DATA_DIR=/dev/shm` and `TMPDIR=/dev/shm` are required for `readOnlyRootFilesystem`
- Install `sitecustomize.py` into **site-packages**, not only on `PYTHONPATH`
- No application code changes — instrumentation is file-based only

## HTTP endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Health check; `coverage_enabled: true` when active |
| `GET /coverage/save` | Flush Gunicorn worker coverage to `/dev/shm` |
| `GET /coverage` | Combined base64-encoded coverage.py data (JSON wrapper) |

Default port: **53700** (`COVERAGE_PORT` env var overrides).
