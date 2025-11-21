---
name: coverport-integration
description: Integrate coverport into Go repositories with Tekton pipelines to enable e2e test coverage collection and upload to Codecov. Use this skill when users ask to integrate coverport, add e2e coverage tracking, or set up coverage instrumentation for Go projects.
---

# Coverport Integration Skill

This skill automates the integration of coverport into Go repositories for e2e test coverage collection and upload to Codecov.

## What is Coverport?

Coverport is a tool that enables e2e test coverage collection by:
1. Building instrumented container images with Go's `-cover` flag
2. Collecting coverage data from running containers during e2e tests
3. Uploading the coverage data to Codecov with appropriate flags

## When to Use This Skill

Use this skill when the user:
- Asks to integrate coverport into their repository
- Wants to add e2e test coverage tracking
- Needs to set up coverage instrumentation for Go projects
- Mentions integrating coverage collection for Tekton/Konflux pipelines

## Prerequisites

Before using this skill, verify the repository has:
- Go codebase with a Dockerfile
- Tekton pipelines for CI/CD (typically in `.tekton/` directory)
- E2E test pipeline (typically in `integration-tests/pipelines/`)
- GitHub Actions workflows (optional but common)
- Codecov account

## Instructions

### Step 0: Pre-Integration Repository Scan

Before starting, run these checks to understand the repository structure:

1. **Find main.go location:**
   ```bash
   find . -name "main.go" -not -path "*/vendor/*" -not -path "*/test/*"
   ```

2. **Check current Dockerfile build command:**
   ```bash
   grep -A5 "go build" Dockerfile
   ```

3. **List Tekton pipelines:**
   ```bash
   ls .tekton/*.yaml
   ls integration-tests/pipelines/*.yaml 2>/dev/null || echo "No integration-tests/pipelines found"
   ```

4. **Check for existing coverage setup:**
   ```bash
   grep -r "ENABLE_COVERAGE\|instrumented\|coverport" . --exclude-dir=vendor --exclude-dir=.git
   ```

This helps identify potential conflicts or existing coverage infrastructure before making changes.

### Step 1: Analyze the Repository

Analyze the repository structure to understand what needs to be modified:

1. **Find the Dockerfile** - Look for the main Dockerfile
2. **Identify binaries being built** - Check what Go binaries are compiled in the Dockerfile and note if main.go is in root or subdirectory
3. **Find Tekton push pipeline** - Look in `.tekton/` for `*-push.yaml`
4. **Find E2E test pipeline** - Look in `integration-tests/pipelines/` for `*e2e*.yaml`
5. **Find Tekton PR pipeline** - Look in `.tekton/` for `*-pull-request.yaml`
6. **Find GitHub Actions** - Look in `.github/workflows/` for `pr.yaml`, `pr.yml`, `codecov.yaml`, or `codecov.yml`
7. **Check for existing coverage integration** - Search for `ENABLE_COVERAGE`, `instrumented`, `coverport`

### Step 2: Ask Clarifying Questions

Before making changes, ask the user:

1. **Which binaries to instrument?** - If the Dockerfile builds multiple binaries, ask which ones run during e2e tests
2. **Tenant namespace** - Confirm the namespace where their build and integration pipelines run (check `.tekton/*-push.yaml` for the `namespace` field)
3. **Secret name** - Confirm they want to use `coverport-secrets` or specify a different name
4. **OCI storage** - Confirm where coverage data should be stored (the quay.io repository for test artifacts)

### Step 3: Modify the Dockerfile

Add coverage instrumentation support:

**Add build arguments** (near the top after FROM):
```dockerfile
# Build arguments
ARG ENABLE_COVERAGE=false
ARG COVERAGE_SERVER_URL=https://raw.githubusercontent.com/konflux-ci/coverport/v0.0.1/instrumentation/go/coverage_server.go
```

**Modify the build command** to conditionally build with coverage:

**IMPORTANT**: Go's `go build` requires all source files to be in the same directory when building with individual files. You have two approaches:

**Approach A: Package-based build (RECOMMENDED)**
```dockerfile
RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        echo "📥 Downloading coverage server from: $COVERAGE_SERVER_URL"; \
        wget -q "$COVERAGE_SERVER_URL" -O <source-dir>/coverage_server.go; \
        echo "✅ Coverage server downloaded"; \
        echo "🧪 Building with coverage instrumentation..."; \
        CGO_ENABLED=0 go build -cover -covermode=atomic -a -o <binary-name> ./<source-dir>; \
    else \
        echo "🚀 Building production binary..."; \
        CGO_ENABLED=0 go build -a -o <binary-name> ./<source-dir>; \
    fi
```

**Approach B: File-based build (only if main.go is in root)**
```dockerfile
RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        echo "📥 Downloading coverage server from: $COVERAGE_SERVER_URL"; \
        wget -q "$COVERAGE_SERVER_URL" -O coverage_server.go; \
        echo "✅ Coverage server downloaded"; \
        echo "🧪 Building with coverage instrumentation..."; \
        CGO_ENABLED=0 go build -cover -covermode=atomic -a -o <binary-name> main.go coverage_server.go; \
    else \
        echo "🚀 Building production binary..."; \
        CGO_ENABLED=0 go build -a -o <binary-name> main.go; \
    fi
```

**Important**:
- If main.go is in a subdirectory (e.g., `cmd/`), use Approach A and download coverage_server.go to that same directory
- If main.go is in the root directory, you can use either approach
- Replace `<binary-name>` with the actual binary name
- Replace `<source-dir>` with the directory containing main.go (e.g., `cmd`, `./`, etc.)
- Only instrument binaries that run during e2e tests
- Keep other binaries without instrumentation

### Step 3.5: Validate Dockerfile Changes Locally

**IMPORTANT**: Before proceeding to pipeline changes, validate the Dockerfile modifications work correctly using podman or docker:

```bash
# Build instrumented image
podman build --build-arg ENABLE_COVERAGE=true -t test-instrumented -f Dockerfile .

# Build production image (without coverage)
podman build -t test-production -f Dockerfile .

# Verify both images built successfully
podman images | grep test-
```

**Expected output in instrumented build:**
- "📥 Downloading coverage server from: ..."
- "✅ Coverage server downloaded"
- "🧪 Building with coverage instrumentation..."

**Expected output in production build:**
- "🚀 Building production binary..."

**If builds fail:**
- Stop and fix the Dockerfile before proceeding
- See Troubleshooting section for common issues
- Ensure wget is available in the builder image
- Verify the coverage server URL is accessible
- Check that file paths match the directory structure

**Why this validation matters:**
- Catches Dockerfile syntax errors immediately
- Verifies coverage server download works
- Confirms both production and instrumented builds succeed
- Prevents wasting CI/CD pipeline time on broken builds
- Validates the conditional build logic works correctly

### Step 4: Update Tekton Push Pipeline

Add a task to build an instrumented image in the push pipeline (e.g., `.tekton/*-push.yaml`):

Find the location after `prefetch-dependencies` task and add:

```yaml
- name: build-instrumented-image
  params:
  - name: IMAGE
    value: $(params.output-image).instrumented
  - name: DOCKERFILE
    value: $(params.dockerfile)
  - name: CONTEXT
    value: $(params.path-context)
  - name: HERMETIC
    value: "false"
  - name: PREFETCH_INPUT
    value: ""
  - name: IMAGE_EXPIRES_AFTER
    value: $(params.image-expires-after)
  - name: COMMIT_SHA
    value: $(tasks.clone-repository.results.commit)
  - name: BUILD_ARGS
    value:
    - $(params.build-args[*])
    - ENABLE_COVERAGE=true
  - name: BUILD_ARGS_FILE
    value: $(params.build-args-file)
  - name: SOURCE_ARTIFACT
    value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
  - name: CACHI2_ARTIFACT
    value: $(tasks.prefetch-dependencies.results.CACHI2_ARTIFACT)
  runAfter:
  - prefetch-dependencies
  taskRef:
    params:
    - name: name
      value: buildah-oci-ta
    - name: bundle
      value: quay.io/konflux-ci/tekton-catalog/task-buildah-oci-ta:0.5@sha256:fb3b36f1f800960dd3eb6291c9f8802b7305608a61b27310a541f53a716844a3
    - name: kind
      value: task
    resolver: bundles
  when:
  - input: $(tasks.init.results.build)
    operator: in
    values:
    - "true"
```

**IMPORTANT - Key points**:
- Use `buildah-oci-ta` (NOT `buildah-remote-oci-ta`) - this is a regular local build for amd64 testing clusters
- This should be a single task, NOT a matrix build (no PLATFORM parameter, no IMAGE_APPEND_PLATFORM)
- Image tagged with `.instrumented` suffix
- `HERMETIC: "false"` is required for downloading coverage server
- `PREFETCH_INPUT: ""` (empty) to skip dependency prefetching for instrumented build
- `BUILD_ARGS` includes `ENABLE_COVERAGE=true`
- Do NOT add a `build-instrumented-image-index` task - the instrumented image is single-platform only

### Step 5: Update E2E Test Pipeline

Make three changes to the e2e test pipeline:

**A. Update test-metadata task** from v0.3 to v0.4:
```yaml
- name: test-metadata
  taskRef:
    resolver: git
    params:
      - name: url
        value: https://github.com/konflux-ci/tekton-integration-catalog.git
      - name: revision
        value: main
      - name: pathInRepo
        value: tasks/test-metadata/0.4/test-metadata.yaml
```

**B. Update image references (if applicable):**

**NOTE:** Only modify this if your e2e tests actually **run the containerized application**.

- If your tests **build the manager from source** (e.g., using `make build` or `go run main.go`), you may need to modify the build/run commands to use coverage flags instead, or deploy the instrumented container image
- If your tests **deploy and run containers**, proceed with updating image references

For tests that deploy/run container images, find parameters that reference images and change:
- `container-repo` → `instrumented-container-repo`
- `container-tag` → `instrumented-container-tag`
- `container-image` → `instrumented-container-image`

**Example scenarios:**
- **Scenario 1** (uses container): Tests deploy the app to a cluster using the container image → Update image references
- **Scenario 2** (builds from source): Tests run `make build && ./manager` inside the pipeline → May not need image reference changes, but need to ensure the running process is instrumented
- **Scenario 3** (hybrid): Tests build from source but coverage collection expects instrumented container → Coordinate with user on approach

**C. Add coverage collection task** after e2e tests:
```yaml
- name: collect-and-upload-coverage
  runAfter:
    - <e2e-test-task-name>  # Replace with actual task name
  params:
    - name: instrumented-images
      value: "$(tasks.test-metadata.results.instrumented-container-repo):$(tasks.test-metadata.results.instrumented-container-tag)"
    - name: cluster-access-secret-name
      value: kfg-$(context.pipelineRun.name)  # Adjust if different
    - name: test-name
      value: e2e-tests
    - name: oci-container
      value: "$(params.oci-container-repo):$(context.pipelineRun.name)"
    - name: codecov-flags
      value: e2e-tests
    - name: credentials-secret-name
      value: "coverport-secrets"  # Or user-specified name
  taskRef:
    resolver: git
    params:
      - name: url
        value: https://github.com/konflux-ci/tekton-integration-catalog.git
      - name: revision
        value: main
      - name: pathInRepo
        value: tasks/coverport-coverage/0.1/coverport-coverage.yaml
```

### Step 6: Update Tekton PR Pipeline (Pull Request Pipeline)

Update the PR pipeline (e.g., `.tekton/*-pull-request.yaml`) to build with coverage instrumentation:

**A. Remove hermetic build and prefetch parameters from spec.params:**

Find and remove these parameters from the `spec.params` section:
```yaml
# REMOVE these lines from spec.params:
  - name: hermetic
    value: "true"  # or true
  - name: prefetch-input
    value:
      - type: gomod
        path: "."
```

This allows the parameters to use their default values from `pipelineSpec.params` (hermetic: "false", prefetch-input: ""), which is required for coverage builds to download the coverage server.

**B. Add ENABLE_COVERAGE=true to BUILD_ARGS:**

Find the `build-images` task (or equivalent) and add `ENABLE_COVERAGE=true` to its `BUILD_ARGS`:

```yaml
- name: build-images
  # ... other params ...
  params:
  # ... other params ...
  - name: BUILD_ARGS
    value:
    - $(params.build-args[*])
    - ENABLE_COVERAGE=true  # Add this line
  # ... rest of the task ...
```

**Key points**:
- Remove `hermetic` and `prefetch-input` from spec.params to disable hermetic build and prefetching
- Add `ENABLE_COVERAGE=true` to the regular build task in PR pipeline
- This enables coverage collection for PR builds which can be used for PR-level testing
- No need to create a separate instrumented image task in PR pipeline - just modify the existing build task

### Step 7: Update GitHub Actions

Add codecov flags to distinguish unit tests from e2e tests.

In `.github/workflows/pr.yaml` (or similar), update the codecov upload step:

```yaml
- name: Upload coverage to Codecov
  uses: codecov/codecov-action@v5
  with:
    flags: unit-tests
```

If there's a separate `codecov.yml` workflow, add the same flags there with the token:
```yaml
- name: Codecov
  uses: codecov/codecov-action@v5
  with:
    token: ${{ secrets.CODECOV_TOKEN }}
    flags: unit-tests
```

### Step 8: Document Manual Steps

After making all changes, inform the user they need to create a Kubernetes secret.

**IMPORTANT**: The secret must be created in the namespace where your build and integration pipelines run. This is typically your tenant namespace (e.g., `my-tenant`, not a specific repository namespace like `rhtap-release-2-tenant`). You can identify the correct namespace by checking the `namespace` field in your `.tekton/*-push.yaml` file.

**Option A - Using kubectl:**
```bash
# First, create the dockerconfig JSON file
cat > /tmp/dockerconfig.json <<EOF
{"auths":{"quay.io":{"auth":"<base64-encoded-quay-user:token>","email":""}}}
EOF

# Create the secret with both keys in YOUR tenant namespace
kubectl create secret generic coverport-secrets \
  --from-literal=codecov-token=<your-codecov-token> \
  --from-file=oci-storage-dockerconfigjson=/tmp/dockerconfig.json \
  -n <your-tenant-namespace>

# Clean up
rm /tmp/dockerconfig.json
```

**Option B - Using YAML:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: coverport-secrets
  namespace: <your-tenant-namespace>  # Replace with your tenant namespace
type: Opaque
stringData:
  codecov-token: <your-codecov-token>
  oci-storage-dockerconfigjson: '{"auths":{"quay.io":{"auth":"<base64-encoded-quay-user:token>","email":""}}}'
```

**Required secret keys:**
- `codecov-token` - Your Codecov API token for uploading coverage reports to Codecov
- `oci-storage-dockerconfigjson` - Docker config JSON with Quay.io credentials for pushing coverage test artifacts to an OCI container registry
  - This is used by the `collect-and-upload-coverage` task to store coverage data as OCI artifacts in quay.io
  - The coverage collection process extracts coverage data from instrumented containers and pushes it to the OCI registry before uploading to Codecov
  - The `auth` value should be base64-encoded `username:token`
  - To encode: `echo -n "quay-username:quay-token" | base64`
  - You need push access to the quay.io repository specified in the e2e pipeline's `oci-container-repo` parameter

### Step 9: Post-Integration Validation Checklist

Before committing the changes, verify all modifications are correct:

**Local validation (already completed in Step 3.5):**
- [ ] `podman build` (production) succeeds
- [ ] `podman build --build-arg ENABLE_COVERAGE=true` (instrumented) succeeds
- [ ] Instrumented build logs show "🧪 Building with coverage instrumentation..."
- [ ] Production build logs show "🚀 Building production binary..."

**File modifications checklist:**
- [ ] `Dockerfile` has `ENABLE_COVERAGE` and `COVERAGE_SERVER_URL` build args
- [ ] `Dockerfile` has conditional build logic with coverage flags
- [ ] `.tekton/*-push.yaml` has `build-instrumented-image` task after `prefetch-dependencies`
- [ ] `.tekton/*-push.yaml` instrumented task uses `buildah-oci-ta` (not `buildah-remote-oci-ta`)
- [ ] `.tekton/*-push.yaml` instrumented task has `HERMETIC: "false"` and `PREFETCH_INPUT: ""`
- [ ] `.tekton/*-pull-request.yaml` removed `hermetic` and `prefetch-input` from spec.params
- [ ] `.tekton/*-pull-request.yaml` has `ENABLE_COVERAGE=true` in BUILD_ARGS
- [ ] `integration-tests/pipelines/*e2e*.yaml` uses test-metadata v0.4
- [ ] `integration-tests/pipelines/*e2e*.yaml` has `collect-and-upload-coverage` task
- [ ] `integration-tests/pipelines/*e2e*.yaml` updated image references (if applicable)
- [ ] `.github/workflows/pr.y*ml` has `flags: unit-tests` in codecov action
- [ ] `.github/workflows/codecov.y*ml` has `flags: unit-tests` in codecov action

**Documentation provided to user:**
- [ ] Instructions for creating `coverport-secrets` Kubernetes secret in their tenant namespace
- [ ] Explanation that the namespace should be where their build and integration pipelines run
- [ ] Required secret keys: `codecov-token` and `oci-storage-dockerconfigjson`
- [ ] Explanation of what `oci-storage-dockerconfigjson` is used for (pushing coverage artifacts to quay.io)
- [ ] Instructions for encoding auth credentials
- [ ] Note about needing push access to the quay.io repository

**Summary to provide user:**
List all modified files with brief description of changes:
```
Modified files:
- Dockerfile: Added coverage instrumentation support
- .tekton/<name>-push.yaml: Added instrumented image build task
- .tekton/<name>-pull-request.yaml: Enabled coverage for PR builds
- integration-tests/pipelines/<name>-e2e-pipeline.yaml: Added coverage collection
- .github/workflows/pr.yml: Added unit-tests flag
- .github/workflows/codecov.yml: Added unit-tests flag
```

## Validation

After integration is deployed to CI/CD, provide these verification steps to the user:

1. **Check instrumented image build:**
   - Push a commit to main branch
   - Verify the push pipeline creates an image with `.instrumented` tag
   - Check build logs for "🧪 Building with coverage instrumentation..." message

2. **Check e2e coverage collection:**
   - Run e2e tests
   - Verify `collect-and-upload-coverage` task executes successfully
   - Check Codecov dashboard for coverage data with `e2e-tests` flag

3. **Check unit test coverage:**
   - Create a PR
   - Verify unit tests upload coverage with `unit-tests` flag
   - Check Codecov shows both unit and e2e coverage

## Troubleshooting

Common issues and solutions:

**Build error: "named files must all be in one directory"**
- **Cause**: Go build was called with files from different directories (e.g., `go build cmd/main.go coverage_server.go`)
- **Solution**: Use package-based build instead of file-based build:
  - Download coverage_server.go to the same directory as main.go: `wget ... -O <source-dir>/coverage_server.go`
  - Build using package path: `go build -o manager ./<source-dir>` instead of `go build -o manager <source-dir>/main.go coverage_server.go`
- **Example**: If main.go is in `cmd/`, use `wget ... -O cmd/coverage_server.go` and `go build ./cmd`

**Instrumented build fails:**
- Verify `COVERAGE_SERVER_URL` is accessible
- Check that `wget` is available in the builder image
- Ensure hermetic mode is disabled for instrumented builds (`HERMETIC: "false"`)
- For PR pipeline: Ensure `hermetic` and `prefetch-input` params are removed from spec.params section
- Verify the download path matches the source directory
- Ensure you're using `buildah-oci-ta` for instrumented builds in push pipeline, not `buildah-remote-oci-ta`
- Verify there's no matrix build or PLATFORM parameter for the instrumented image task

**Coverage data not uploaded:**
- Verify `coverport-secrets` exists in your tenant namespace (the namespace where your build and integration pipelines run)
- Check `codecov-token` key exists in the secret
- Check `oci-storage-dockerconfigjson` key exists and is valid (should be a valid Docker config JSON)
- Verify you have push access to the quay.io repository specified in the e2e pipeline's `oci-container-repo` parameter
- Review `collect-and-upload-coverage` task logs for errors related to OCI push or Codecov upload

**Coverage data incomplete:**
- Verify e2e tests are using the instrumented image (check image tag has `.instrumented` suffix)
- Ensure coverage server is properly included in the build
- Check that the correct binaries are instrumented
- Verify coverage_server.go was downloaded to the correct directory

**E2E tests pass but no coverage data collected:**
- Verify the e2e tests are actually **running the instrumented binary/container**
- If tests build from source (e.g., `make build`), the build process must include `-cover` flags
- Check that `GOCOVERDIR` environment variable is set in the running container/process
- Verify the coverage collection task can access the cluster where instrumented app runs
- Review coverage collection task logs for connection or permission errors

**Local podman/docker build fails with wget error:**
- The builder image may not have `wget` installed
- Try using `curl` instead: `curl -sL "$COVERAGE_SERVER_URL" -o coverage_server.go`
- Or install wget in the Dockerfile: `RUN microdnf install -y wget` (for UBI images)

**Build fails with "no such file or directory" for coverage_server.go:**
- Verify the download path matches where you're building from
- Check that the conditional logic properly downloads before building
- Ensure the path in `wget -O <path>` matches the build directory structure

## Best Practices

1. **Validate early and often** - Always run podman/docker builds after Dockerfile changes, before modifying pipelines
2. **Be adaptive** - Repository structures vary, adapt the integration to the specific repository
3. **Ask questions** - If unsure about something, ask the user for clarification
4. **Show diffs** - When modifying files, explain what's changing
5. **Preserve existing logic** - Don't break existing functionality
6. **Handle edge cases** - Check for existing build args, multiple Dockerfiles, etc.
7. **Provide context** - Explain why each change is needed
8. **Use checklists** - Go through the post-integration checklist before completing
9. **Test both paths** - Ensure both production and instrumented builds work

## Reference Implementation

The reference implementation can be found in the `release-service` repository, commits `1b2208f..dbf965d`.

Key files modified:
- `Dockerfile` - Added coverage instrumentation
- `.tekton/release-service-push.yaml` - Added instrumented image build
- `integration-tests/pipelines/konflux-e2e-tests-pipeline.yaml` - Updated to use test-metadata v0.4 and added coverage collection
- `.github/workflows/codecov.yml` and `.github/workflows/pr.yml` - Added codecov flags

**Note**: The `release-service` repository uses the `rhtap-release-2-tenant` namespace, which is specific to that repository. When implementing coverport for other repositories, use the appropriate tenant namespace where that repository's build and integration pipelines run.

## Examples

### Example 1: Single Binary Repository (main.go in root)

For a repository that builds one binary (`manager`) where main.go is in the root directory:

```dockerfile
# Before
RUN CGO_ENABLED=0 go build -a -o manager main.go

# After
ARG ENABLE_COVERAGE=false
ARG COVERAGE_SERVER_URL=https://raw.githubusercontent.com/konflux-ci/coverport/v0.0.1/instrumentation/go/coverage_server.go

RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        wget -q "$COVERAGE_SERVER_URL" -O coverage_server.go; \
        CGO_ENABLED=0 go build -cover -covermode=atomic -a -o manager main.go coverage_server.go; \
    else \
        CGO_ENABLED=0 go build -a -o manager main.go; \
    fi
```

### Example 2: Multiple Binaries (main.go in subdirectory)

For a repository that builds `manager` (from cmd/main.go) and `snapshotgc`, where only `manager` needs coverage:

```dockerfile
# Before
RUN CGO_ENABLED=0 go build -a -o manager cmd/main.go \
 && CGO_ENABLED=0 go build -a -o snapshotgc cmd/snapshotgc/snapshotgc.go

# After
ARG ENABLE_COVERAGE=false
ARG COVERAGE_SERVER_URL=https://raw.githubusercontent.com/konflux-ci/coverport/v0.0.1/instrumentation/go/coverage_server.go

RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        wget -q "$COVERAGE_SERVER_URL" -O cmd/coverage_server.go; \
        CGO_ENABLED=0 go build -cover -covermode=atomic -a -o manager ./cmd; \
    else \
        CGO_ENABLED=0 go build -a -o manager ./cmd; \
    fi \
 && CGO_ENABLED=0 go build -a -o snapshotgc cmd/snapshotgc/snapshotgc.go
```

**Note**:
- coverage_server.go is downloaded to the `cmd/` directory where main.go lives
- Package-based build (`./cmd`) is used instead of file-based build to avoid "named files must all be in one directory" error
- snapshotgc binary is built separately without coverage instrumentation

## Summary

This skill automates coverport integration by:
1. Running pre-integration repository scan to understand structure
2. Analyzing the repository structure in detail
3. Asking clarifying questions about binaries, secrets, and storage
4. Modifying the Dockerfile to support coverage builds
5. **Validating Dockerfile changes locally with podman/docker builds**
6. Adding instrumented image build to Tekton push pipeline
7. Updating e2e pipeline to use test-metadata v0.4 and instrumented images
8. Adding coverage collection task to e2e pipeline
9. Updating PR pipeline to build with coverage instrumentation
10. Updating GitHub Actions to add codecov flags
11. Providing comprehensive post-integration validation checklist
12. Providing documentation for manual secret creation

The integration enables automatic e2e test coverage collection and upload to Codecov with proper flag separation from unit tests.

**Key improvements in this skill:**
- **Early validation**: Podman/docker builds catch issues before CI/CD changes
- **Clear checklists**: Pre and post-integration checklists ensure nothing is missed
- **Better guidance**: Clarifies when e2e image references need updating vs source builds
- **Enhanced troubleshooting**: Covers common scenarios encountered during integration
