# coverport - Universal Coverage Tool for Konflux Pipelines

`coverport` is a comprehensive CLI tool for collecting, processing, and uploading coverage data from instrumented applications. It supports both Kubernetes-based collection (ideal for CI/CD pipelines) and direct HTTP collection (perfect for local development). Specifically designed for Konflux/Tekton integration pipelines.

## Features

- **🌐 Direct URL Collection**: Collect from localhost or any HTTP endpoint (no Kubernetes required)
- **🔍 Image-based Pod Discovery**: Automatically find pods running specific container images
- **📦 Konflux Snapshot Support**: Parse Tekton/Konflux snapshots to discover all components
- **🎯 Multi-Component Collection**: Collect coverage from multiple services in one command
- **📊 Automatic Processing**: Generate, filter, and create HTML reports automatically
- **🗂️ Organized Output**: Coverage data organized by component for easy analysis
- **🚀 OCI Registry Push**: Push coverage artifacts directly to container registries
- **🔧 Flexible Discovery**: Support for label selectors, image refs, and explicit pod names
- **🔐 Git Metadata Extraction**: Extract repository information from container images using cosign
- **📤 Codecov Integration**: Direct upload to Codecov with proper commit mapping
- **🌍 Multi-Language Support**: Go and Python supported, Node.js (NYC) coming soon

## Recent Improvements

- **Manifest-Based Workflow**: `collect` now generates `metadata.json` for simplified batch processing
- **Automatic PR Detection**: Extracts PR numbers from image metadata or branch names for Codecov
- **Intelligent Path Remapping**: Automatically converts container paths (`/app/`) to repository-relative paths
- **HTML Generation in Process**: HTML reports now generated during `process` phase with source code access
- **Workspace Safety**: Added checks to prevent accidental data deletion during processing
- **Git Service Auto-Detection**: Automatically detects GitHub/GitLab from repository URLs for Codecov

## Installation

### From Source

```bash
cd coverport-cli
go build -o coverport main.go
```

### Using Go Install

```bash
go install github.com/konflux-ci/coverport/cli@latest
```

## Quick Start

### Collect Coverage from Localhost (Local Development)

Perfect for local development and testing:

```bash
# Start your app with coverage instrumentation on port 8080
# (with coverage server on port 53700)

# Collect coverage directly
coverport collect \
  --url http://localhost:53700 \
  --test-name="local-e2e-test" \
  --output=./coverage-output

# View results
go tool covdata textfmt -i=./coverage-output/local-e2e-test -o=coverage.out
go tool cover -func=coverage.out
```

### Collect Coverage in Konflux Pipeline

The primary use case is in a Konflux/Tekton integration test pipeline:

```bash
# Using the SNAPSHOT parameter from Konflux
coverport collect \
  --snapshot="$SNAPSHOT" \
  --test-name="e2e-tests" \
  --output=/workspace/coverage-output \
  --push \
  --registry=quay.io \
  --repository=myorg/coverage-artifacts \
  --tag="coverage-$(date +%Y%m%d-%H%M%S)"
```

### Collect from Specific Images

```bash
coverport collect \
  --images=quay.io/user/app1@sha256:abc123,quay.io/user/app2@sha256:def456 \
  --namespace=testing \
  --output=./coverage-output
```

### Discover Pods (Dry Run)

```bash
# See which pods will be targeted without collecting coverage
coverport discover --snapshot="$SNAPSHOT"
```

## Commands

### `coverport collect`

Collect raw coverage data from HTTP endpoints or Kubernetes pods.

**Discovery Methods** (choose one):

- `--url` - Direct HTTP URL to coverage server (e.g., `http://localhost:53700`) - **New!**
- `--snapshot` - Konflux/Tekton snapshot JSON (recommended for CI/CD)
- `--snapshot-file` - Path to snapshot JSON file
- `--images` - Comma-separated list of container images
- `--label-selector` - Label selector to find pods
- `--pods` - Comma-separated list of explicit pod names

> **Note**: The `--url` flag enables local development workflows without requiring Kubernetes. Perfect for testing coverage collection locally before deploying to CI/CD. See [URL_COLLECTION.md](URL_COLLECTION.md) for details.

**Coverage Options:**

- `--port` - Coverage server port (default: 53700, falls back to 9095 when not set)
- `--output`, `-o` - Output directory (default: ./coverage-output)
- `--test-name` - Test name for identification (auto-generated if not specified)
- `--source-dir` - Source directory for path remapping (default: .)
- `--remap-paths` - Enable automatic path remapping (default: true)
- `--filters` - File patterns to filter from coverage (default: coverage_server.go)

**Processing Options:**

- `--auto-process` - Automatically process reports (default: true)
- `--skip-generate` - Skip generating text reports
- `--skip-filter` - Skip filtering reports
- `--skip-html` - Skip generating HTML reports

**OCI Push Options:**

- `--push` - Push coverage artifact to OCI registry
- `--registry` - OCI registry URL (default: quay.io)
- `--repository` - OCI repository (e.g., 'user/coverage-artifacts')
- `--tag` - OCI artifact tag (auto-generated if not specified)
- `--expires-after` - Artifact expiration (default: 30d, examples: 7d, 1y)
- `--artifact-title` - Custom artifact title

**Advanced Options:**

- `--timeout` - Timeout in seconds (default: 120)
- `--namespace`, `-n` - Kubernetes namespace (empty = search all)
- `--verbose` - Enable verbose output

### `coverport process`

Process coverage data and upload to coverage services. This command:
1. Extracts coverage artifact from OCI registry (or uses local directory)
2. Extracts git metadata from container image using cosign
3. Clones the source repository at the specific commit
4. Converts and processes coverage data with proper path mapping
5. Uploads to Codecov (and optionally SonarQube)

This single command replaces 5+ complex bash script steps in Tekton pipelines!

**Input Options:**

- `--artifact-ref` - OCI artifact reference containing coverage data
- `--coverage-dir` - Local directory containing coverage data (alternative to --artifact-ref)
- `--image` - Container image reference to extract git metadata from

**Workspace Options:**

- `--workspace` - Workspace directory (default: temp directory)
- `--keep-workspace` - Keep workspace directory after processing

**Coverage Processing Options:**

- `--format` - Coverage format: go, python, nyc, auto (default: auto)
- `--filters` - File patterns to exclude from coverage

**Upload Options:**

- `--upload` - Upload coverage to services (default: true)
- `--codecov-token` - Codecov upload token (or use CODECOV_TOKEN env var)
- `--codecov-flags` - Codecov flags (default: e2e-tests)
- `--codecov-name` - Codecov upload name

**Git Options:**

- `--repo-url` - Git repository URL (optional, extracted from image if not provided)
- `--commit-sha` - Git commit SHA (optional, extracted from image if not provided)
- `--skip-clone` - Skip cloning the repository
- `--clone-depth` - Git clone depth (default: 1, 0 for full clone)

**Examples:**

```bash
# Process coverage from OCI artifact
coverport process \
  --artifact-ref=quay.io/org/coverage:tag \
  --image=quay.io/org/app@sha256:abc123 \
  --codecov-token=$CODECOV_TOKEN

# Process from local directory with custom options
coverport process \
  --coverage-dir=./coverage-output/myapp/test-123 \
  --image=quay.io/org/app@sha256:abc123 \
  --codecov-token=$CODECOV_TOKEN \
  --codecov-flags=e2e,integration \
  --workspace=/workspace/process \
  --keep-workspace

# Process with manual git metadata (no cosign needed)
coverport process \
  --artifact-ref=quay.io/org/coverage:tag \
  --repo-url=https://github.com/org/repo \
  --commit-sha=abc123def456 \
  --codecov-token=$CODECOV_TOKEN
```

### `coverport discover`

Discover pods without collecting coverage (useful for debugging).

```bash
coverport discover --snapshot="$SNAPSHOT"
coverport discover --images=quay.io/user/app:latest
coverport discover --namespace=default --label-selector=app=myapp
```

## Usage Examples

### Example 1: Complete Konflux Pipeline Workflow

The typical workflow in a Konflux pipeline consists of two steps:

**Step 1: Collect Coverage**
```bash
# After running tests, collect coverage from deployed pods
coverport collect \
  --snapshot="$SNAPSHOT" \
  --namespace="$TEST_NAMESPACE" \
  --test-name="e2e-$(date +%Y%m%d-%H%M%S)" \
  --output=/workspace/coverage \
  --push \
  --registry=quay.io \
  --repository=myorg/coverage-artifacts
```

**Step 2: Process and Upload**
```bash
# Process the coverage artifact and upload to Codecov
# This replaces 5+ bash script steps with one command!
coverport process \
  --artifact-ref="$COVERAGE_ARTIFACT_REF" \
  --image="$COMPONENT_IMAGE" \
  --codecov-token="$CODECOV_TOKEN" \
  --codecov-flags=e2e-tests \
  --workspace=/workspace/process
```

See `examples/simplified-pipeline.yaml` for a complete pipeline example.

### Example 2: Traditional Pipeline (collect only)

Add this task to your Tekton pipeline after running tests:

```yaml
- name: collect-coverage
  runAfter:
    - run-e2e-tests
  taskSpec:
    params:
      - name: SNAPSHOT
        value: $(params.SNAPSHOT)
    steps:
      - name: collect
        image: quay.io/myorg/coverport:latest
        env:
          - name: SNAPSHOT
            value: $(params.SNAPSHOT)
          - name: KUBECONFIG
            value: /workspace/.kube/config
        script: |
          #!/bin/sh
          set -eux

          coverport collect \
            --snapshot="$SNAPSHOT" \
            --test-name="$(context.taskRun.name)" \
            --output=/workspace/coverage-output \
            --push \
            --registry=quay.io \
            --repository=myorg/coverage-artifacts \
            --tag="coverage-$(date +%Y%m%d-%H%M%S)"

          echo "Coverage collection complete!"
```

### Example 3: Multi-Component Collection

When your snapshot contains multiple components:

```json
{
  "components": [
    {
      "name": "frontend",
      "containerImage": "quay.io/user/frontend@sha256:abc123"
    },
    {
      "name": "backend",
      "containerImage": "quay.io/user/backend@sha256:def456"
    },
    {
      "name": "worker",
      "containerImage": "quay.io/user/worker@sha256:ghi789"
    }
  ]
}
```

Running `coverport collect --snapshot="..."` will:
1. Discover all 3 pods running these images
2. Collect coverage from each
3. Organize output by component:
   ```
   coverage-output/
   ├── frontend/
   │   └── coverage-e2e-tests-frontend/
   │       ├── covmeta.*
   │       ├── covcounters.*
   │       ├── coverage.out
   │       ├── coverage_filtered.out
   │       ├── coverage.html
   │       ├── metadata.json
   │       └── component-metadata.json
   ├── backend/
   │   └── coverage-e2e-tests-backend/
   │       └── ...
   └── worker/
       └── coverage-e2e-tests-worker/
           └── ...
   ```

### Example 3: Label Selector

Collect from pods matching a label:

```bash
coverport collect \
  --namespace=testing \
  --label-selector="app=myapp,version=v2" \
  --test-name="integration-tests"
```

### Example 4: Explicit Pod Names

When you know the exact pod names:

```bash
coverport collect \
  --namespace=testing \
  --pods=myapp-pod-1,myapp-pod-2 \
  --test-name="specific-test"
```

### Example 5: No OCI Push (Local Only)

Collect coverage but keep it local (useful for local development):

```bash
coverport collect \
  --images=localhost:5000/myapp:test \
  --namespace=default \
  --output=./coverage-output
```

## How It Works

### 1. Pod Discovery

`coverport` uses intelligent pod discovery based on your input:

**Snapshot-based discovery:**
- Parses Konflux snapshot JSON
- Extracts all component images
- Searches cluster for pods running these images
- Matches by image digest and repository

**Image-based discovery:**
- Normalizes image references (handles tags and digests)
- Searches all namespaces (or specific namespace)
- Skips system namespaces
- Identifies the correct container in multi-container pods

**Label-based discovery:**
- Uses Kubernetes label selectors
- Filters for running pods only

### 2. Coverage Collection

For each discovered pod:
1. **Port-forward**: Establishes port-forward to the coverage server (default: 53700)
2. **Health check**: Checks `/health` to auto-detect language (Go vs Python)
3. **HTTP request**: Sends request to `/coverage` endpoint
4. **Download**: Retrieves coverage data
5. **Metadata**: Collects pod/container information
6. **Save**: Organizes files by component

**Go-specific flow:**
- POST `/coverage` → retrieves binary coverage data (covmeta + covcounters)

**Python-specific flow:**
- Triggers coverage save via `/coverage/save` (sends SIGHUP to Gunicorn workers)
- GET `/coverage` → retrieves base64-encoded coverage data
- Exec into pod: runs `coverage xml` to generate Cobertura XML using Python inside the pod

### 3. Report Processing

**Go** (when `--auto-process` is enabled, default):
1. **Generate**: Converts binary coverage to text format (`coverage.out`)
2. **Remap**: Remaps container paths to local paths
3. **Filter**: Removes unwanted files (e.g., coverage_server.go)
4. **HTML**: Generates HTML visualization

**Python**: Report processing happens automatically during `collect` — Cobertura XML is generated inside the pod where Python and the `coverage` package are already available. No separate `process` step is needed.

### 4. OCI Artifact Push

When `--push` is enabled:
- Packages coverage data as OCI artifact
- Pushes to specified registry/repository
- Applies metadata and annotations
- Sets expiration time
- Writes artifact reference to file (if `COVERAGE_ARTIFACT_REF_FILE` env var is set)

## Configuration

### Environment Variables

- `KUBECONFIG` - Path to kubeconfig file (default: ~/.kube/config)
- `COVERAGE_ARTIFACT_REF_FILE` - File path to write artifact reference (for Tekton results)

### Coverage Server Requirements

#### Go Applications

1. Build with coverage instrumentation: `go build -cover`
2. Set `GOCOVERDIR` environment variable
3. Run the [go-coverage-http](https://github.com/psturc/go-coverage-http) server (port 53700 by default)
4. Expose the coverage port in the container

```yaml
containers:
- name: app
  image: quay.io/user/myapp:latest
  env:
  - name: GOCOVERDIR
    value: /tmp/coverage
  - name: COVERAGE_SERVER_PORT
    value: "53700"
  ports:
  - containerPort: 8080
  - containerPort: 53700
```

#### Python Applications

1. Add coverage instrumentation files from [py-coverage-http](https://github.com/psturc/py-coverage-http)
2. Build a test Docker image with the coverage wrapper
3. Coverage server runs automatically on port 53700

```yaml
containers:
- name: app
  image: quay.io/user/myapp:test
  env:
  - name: COVERAGE_PROCESS_START
    value: /app/.coveragerc
  - name: COVERAGE_PORT
    value: "53700"
  - name: COVERAGE_DATA_DIR
    value: /dev/shm
  - name: TMPDIR
    value: /dev/shm
  ports:
  - containerPort: 8080
  - containerPort: 53700
  securityContext:
    readOnlyRootFilesystem: true
```

See [py-coverage-http](https://github.com/psturc/py-coverage-http) for full instrumentation setup.

## Output Structure

**Go applications:**
```
coverage-output/
├── component-1/
│   └── coverage-e2e-tests-component-1/
│       ├── covmeta.<hash>              # Binary coverage metadata
│       ├── covcounters.<hash>          # Binary coverage counters
│       ├── coverage.out                # Text coverage report
│       ├── coverage_filtered.out       # Filtered coverage report
│       ├── coverage.html               # HTML visualization
│       ├── metadata.json               # Pod/container metadata
│       └── component-metadata.json     # Component-specific metadata
```

**Python applications:**
```
coverage-output/
├── <test-name>/
│   ├── .coverage                       # Raw coverage data
│   ├── coverage.xml                    # Cobertura XML (for Codecov)
│   └── metadata.json                   # Pod/container metadata
```

## Troubleshooting

### No pods found

**Problem**: "No running pods found matching the criteria"

**Solutions:**
- Check image references match exactly (including registry, repository, tag/digest)
- Verify pods are in `Running` state
- Try `coverport discover` to debug
- Use `--verbose` for more details
- Check namespace restrictions

### Coverage collection fails

**Problem**: "Failed to collect from pod"

**Solutions:**
- Verify coverage server is running in the pod
- Check port is correct (default: 53700)
- Ensure pod has coverage instrumentation
- **Go**: Verify `GOCOVERDIR` is set in the container
- **Python**: Verify `COVERAGE_PROCESS_START` is set and `sitecustomize.py` is installed
- Check network policies allow port-forwarding

### Path remapping issues

**Problem**: HTML report shows container paths

**Solutions:**
- Set `--source-dir` to your project root
- Verify source code is available locally
- Use `--remap-paths=false` to disable (not recommended)

### OCI push fails

**Problem**: "Failed to push coverage artifact"

**Solutions:**
- Verify registry credentials are configured
- Check `docker login` or registry authentication
- Ensure repository exists and you have push permissions
- Verify network connectivity to registry

## Best Practices

### For CI/CD Pipelines

1. **Use snapshots**: Always use `--snapshot` in Konflux pipelines for automatic multi-component support
2. **Set test names**: Use pipeline/task run names for traceability
3. **Enable push**: Always push artifacts to registry for persistence
4. **Set expiration**: Use appropriate `--expires-after` values (7d for PR tests, 90d for releases)
5. **Save artifact ref**: Use `COVERAGE_ARTIFACT_REF_FILE` to pass artifact location to next tasks

### For Local Development

1. **Skip push**: Don't use `--push` for local testing
2. **Use verbose**: Enable `--verbose` for debugging
3. **Discover first**: Run `coverport discover` before `collect`
4. **Check HTML**: Use generated HTML reports for visual inspection

### For Coverage Quality

1. **Filter wisely**: Add test files and generated code to `--filters`
2. **Enable remapping**: Keep `--remap-paths=true` for accurate reports
3. **Set source dir**: Point `--source-dir` to project root
4. **Process reports**: Keep `--auto-process=true` for complete reports

## Integration with SonarQube

The generated `coverage.out` files can be used with SonarQube:

```bash
# Merge all component coverage
go tool covdata textfmt \
  -i=coverage-output/component-1/coverage-*/,coverage-output/component-2/coverage-*/ \
  -o=coverage-merged.out

# Upload to SonarQube
sonar-scanner \
  -Dsonar.go.coverage.reportPaths=coverage-merged.out \
  ...
```

## Contributing

Contributions are welcome! Please submit issues and pull requests to the main repository.

## License

See LICENSE file in the repository root.

## Related Tools

- **[go-coverage-http](https://github.com/psturc/go-coverage-http)**: Coverage HTTP server for Go applications
- **[py-coverage-http](https://github.com/psturc/py-coverage-http)**: Coverage HTTP server for Python applications (Flask, Django, Gunicorn)

## Support

For issues, questions, or feature requests, please open an issue in the GitHub repository.

