# Pattern B Fixture Gap

Pattern B (`collect --url` followed by `process --format=python`) collects
coverage from a Python application running locally via `podman run` or
`docker compose`, rather than from a Kubernetes pod. The CLI connects to
the coverage HTTP endpoint, saves serialized `CoverageData` bytes as
`.coverage`, and then `process --format=python` remaps container paths and
writes Cobertura XML.

## Current state

Pattern B has **no in-repo fixture image** and is **not covered by
`test/e2e`**. The other Python patterns are covered:

| Pattern | Coverage | Fixture |
|---------|----------|---------|
| D (pytest-cov) | `TestPythonPytestCov` | `test/fixtures/python/` (app.py, test_app.py) |
| A (Kind HTTP) | Container e2e tests | `instrumentation/python/` vendored into Kind pod |
| B (local `--url`) | **None** | **None** |

Pattern B was validated manually against an external application (not
checked into this repo). That external validation confirmed the CLI path
works, but it leaves a gap: there is no automated regression test for
the `collect --url` + `process --format=python` flow.

## Gaps in external-only validation

1. **No regression coverage** -- a CLI change that breaks the Python
   `--url` path would not be caught until manual re-validation.
2. **No `.coverage` serialization round-trip test** -- `collect --url`
   produces serialized `CoverageData.dumps()` bytes (not SQLite), and
   `process --format=python` must deserialize them correctly. This path
   is only tested end-to-end with the external app.
3. **No container-path remapping test** -- `process --format=python`
   rewrites `/app/`, `/src/`, `/code/`, and `/workspace/` prefixes to
   the local repo root. This logic is not exercised by any e2e test.

## What an in-repo fixture would need

A Pattern B fixture would extend the existing `test/fixtures/python/`
directory (or add a sibling) with:

1. **An instrumented Dockerfile** with a `test` stage that installs the
   four `instrumentation/python/` files (`coverage_server.py`,
   `sitecustomize.py`, `.coveragerc`, `gunicorn_coverage.py`) and runs
   the app behind Gunicorn with the coverage wrapper. See
   `instrumentation/python/README.md` for the Dockerfile template.

2. **A simple Flask/WSGI app** (the existing `app.py` could be reused)
   that exposes at least one route to exercise during collection.

3. **An e2e test** (in `test/e2e/`) that:
   - Builds the instrumented image (`podman build --target test`)
   - Starts the container with ports 8080 and 53700 mapped
   - Hits the app endpoint to generate coverage
   - Runs `coverport collect --url http://localhost:53700 --test-name=...`
   - Runs `coverport process --format=python --coverage-dir=...`
   - Asserts the output contains valid Cobertura XML with remapped paths

4. **`python3` with `coverage` importable on the test runner** --
   `process --format=python` shells out to `python3`, so the e2e
   environment (Kind node or CI runner) must have Python and the
   `coverage` package installed. This is the main reason Pattern B
   is harder to test in the existing Kind-based e2e suite: the
   coverport binary runs on the host (or in the CLI container, which
   has no Python).

## Related references

- `instrumentation/python/README.md` -- Dockerfile template and local
  validation steps
- `CLAUDE.md` -- Pattern B CLI support description
- `.claude/skills/coverport-integration/SKILL.md` -- Pattern B (Python)
  onboarding workflow (Steps 3-5 Python, Pattern B section)
