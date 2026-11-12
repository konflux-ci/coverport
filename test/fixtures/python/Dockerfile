# Build context must be the repo root:
#   docker build -f test/fixtures/python/Dockerfile -t coverport-testapp-python .
FROM python:3.12-slim

WORKDIR /app

RUN pip install --no-cache-dir flask gunicorn coverage

COPY test/fixtures/python/app.py test/fixtures/python/wsgi.py ./

COPY instrumentation/python/sitecustomize.py /tmp/sitecustomize.py
RUN SITE_PACKAGES=$(python -c "import site; print(site.getsitepackages()[0])") && \
    cp /tmp/sitecustomize.py "$SITE_PACKAGES/sitecustomize.py"

COPY instrumentation/python/.coveragerc /app/.coveragerc
COPY instrumentation/python/gunicorn_coverage.py /opt/gunicorn_coverage.py
COPY instrumentation/python/coverage_server.py /opt/coverage_server.py

ENV COVERAGE_PROCESS_START=/app/.coveragerc
ENV COVERAGE_DATA_DIR=/dev/shm
ENV TMPDIR=/dev/shm

EXPOSE 8080 53700

CMD ["python", "/opt/coverage_server.py", "-m", "gunicorn", \
     "-c", "/opt/gunicorn_coverage.py", "-b", "0.0.0.0:8080", "-w", "1", "wsgi:app"]
