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

**Local `--url` (Pattern B):** `collect` saves `coverage-output/<test-name>/.coverage` only.
The file contains **serialized** `CoverageData.dumps()` bytes from the HTTP response — not a
SQLite database. Generate Cobertura XML on the host (coverport-cli image has no Python).

> **URL format:** `--url` must point at the `/coverage` endpoint. The CLI appends
> `?name=<test-name>` — it does not add `/coverage` for you.
> Use `http://localhost:<port>/coverage`, not `http://localhost:<port>`.

> **Save before collect:** Unlike the K8s collect path, `--url` does **not** call
> `/coverage/save` automatically. Flush worker data first if `/health` shows zero coverage files.

```bash
COVERAGE_PORT=53700

curl -sf "http://localhost:${COVERAGE_PORT}/coverage/save"

coverport collect \
  --url "http://localhost:${COVERAGE_PORT}/coverage" \
  --test-name=e2e-tests \
  --output=./coverage-output

pip install coverage
python3 <<'PY'
import os
import coverage

repo = os.path.abspath(".")
# Must match container WORKDIR and .coveragerc `source` (default /app/)
container_prefix = "/app/"
raw_path = "coverage-output/e2e-tests/.coverage"
xml_path = "coverage-output/e2e-tests/coverage.xml"
sqlite_path = "coverage-output/e2e-tests/.coverage.local"

raw = open(raw_path, "rb").read()
data = coverage.CoverageData(no_disk=True)
data.loads(raw)

remapped = coverage.CoverageData(no_disk=True)
for fn in data.measured_files():
    local_fn = fn.replace(container_prefix, repo + "/")
    lines = data.lines(fn)
    if lines:
        remapped.add_lines({local_fn: lines})
    arcs = data.arcs(fn)
    if arcs:
        remapped.add_arcs({local_fn: arcs})

db = coverage.CoverageData(basename=sqlite_path)
db.update(remapped)
db.write()

cov = coverage.Coverage(data_file=sqlite_path)
cov.load()
cov.xml_report(outfile=xml_path)
print(f"Wrote {xml_path}")
PY
```

`[paths]` in `.coveragerc` alone does **not** remap container paths for host-side XML
generation — set `container_prefix` in the conversion script to match WORKDIR (default
`/app/`), or use K8s collect (Pattern A).

`coverport process --format=python` currently expects a SQLite `.coverage` file and will fail
on `--url` output until the CLI handles serialized data (see follow-up ticket).

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
