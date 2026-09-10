# Python container instrumentation

HTTP-based coverage collection for Python web apps (Flask, Django, FastAPI) running behind
Gunicorn in a container. Exposes combined multiprocess coverage on port **53700** by default
for the coverport CLI (`COVERAGE_PORT` env var overrides).

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

Use the same port in `podman run`, health checks, and collect URLs. Default is **53700**;
set `COVERAGE_PORT` in the image if you need a different port (e.g. `9095`).

```bash
COVERAGE_PORT=53700

podman build --target test -t myapp:instrumented .
podman run --rm -d --name py-cov -p 8080:8080 -p ${COVERAGE_PORT}:${COVERAGE_PORT} myapp:instrumented

curl -s "http://localhost:${COVERAGE_PORT}/health"    # coverage_enabled: true
curl -s http://localhost:8080/                         # exercise app routes
curl -s "http://localhost:${COVERAGE_PORT}/coverage/save"
curl -s "http://localhost:${COVERAGE_PORT}/coverage"   # non-empty coverage_data

podman stop py-cov
```

## Collect with coverport CLI

**Kubernetes (recommended):** `collect` port-forwards to the pod, checks `/health`, triggers
`/coverage/save` when needed, fetches `/coverage`, and generates `coverage.xml` inside the pod.

```bash
coverport collect \
  --namespace=<ns> \
  --label-selector=app=myapp \
  --test-name=e2e-tests \
  --output=./coverage-output
# → coverage-output/e2e-tests/coverage.xml
```

**Local `--url` (Pattern B):** `collect` saves `coverage-output/<test-name>/.coverage`.
The file contains **serialized** `CoverageData.dumps()` bytes from the HTTP response — not a
SQLite database. Convert to Cobertura XML on the host with `coverport process --format=python`.

> **URL format:** `--url` accepts `http://localhost:<port>` or `http://localhost:<port>/coverage`.
> The CLI normalizes bare host:port URLs to `/coverage` and appends `?name=<test-name>`.

> **Save before collect:** For Python servers, `collect --url` checks `/health` and calls
> `/coverage/save` automatically when no coverage files exist yet (same as the K8s collect path).

```bash
COVERAGE_PORT=53700

coverport collect \
  --url "http://localhost:${COVERAGE_PORT}" \
  --test-name=e2e-tests \
  --output=./coverage-output

coverport process \
  --format=python \
  --coverage-dir=./coverage-output/e2e-tests \
  --repo-url=https://github.com/org/repo \
  --commit-sha=abc123 \
  --upload=false
# → coverage-output/e2e-tests/coverage.xml
```

`coverport process --format=python` deserializes the `.coverage` file, remaps container paths
(e.g. `/app/`) to your local repo root, and writes Cobertura XML. `[paths]` in `.coveragerc`
does not apply to deserialized data — the CLI handles remapping for standard container prefixes.

For manual conversion without the CLI, deserialize and remap container paths explicitly — see
the [coverport-integration skill](../../.claude/skills/coverport-integration/SKILL.md) Pattern B
(Python) section for a host-side `coverage.py` script.

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
| `GET /coverage` | Combined base64-encoded `CoverageData.dumps()` data (JSON wrapper) |

Default port: **53700** (`COVERAGE_PORT` env var overrides).
