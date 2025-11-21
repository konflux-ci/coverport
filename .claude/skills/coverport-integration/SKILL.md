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

### Step 1: Analyze the Repository

First, analyze the repository structure to understand what needs to be modified:

1. **Find the Dockerfile** - Look for the main Dockerfile
2. **Identify binaries being built** - Check what Go binaries are compiled in the Dockerfile
3. **Find Tekton push pipeline** - Look in `.tekton/` for `*-push.yaml`
4. **Find E2E test pipeline** - Look in `integration-tests/pipelines/` for `*e2e*.yaml`
5. **Find GitHub Actions** - Look in `.github/workflows/` for `pr.yaml` or `codecov.yml`
6. **Check for existing coverage integration** - Search for `ENABLE_COVERAGE`, `instrumented`, `coverport`

### Step 2: Ask Clarifying Questions

Before making changes, ask the user:

1. **Which binaries to instrument?** - If the Dockerfile builds multiple binaries, ask which ones run during e2e tests
2. **Secret name** - Confirm they want to use `coverport-secrets` or specify a different name
3. **OCI storage** - Confirm where coverage data should be stored

### Step 3: Modify the Dockerfile

Add coverage instrumentation support:

**Add build arguments** (near the top after FROM):
```dockerfile
# Build arguments
ARG ENABLE_COVERAGE=false
ARG COVERAGE_SERVER_URL=https://raw.githubusercontent.com/konflux-ci/coverport/v0.0.1/instrumentation/go/coverage_server.go
```

**Modify the build command** to conditionally build with coverage:
```dockerfile
RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        echo "📥 Downloading coverage server from: $COVERAGE_SERVER_URL"; \
        wget -q "$COVERAGE_SERVER_URL" -O coverage_server.go; \
        echo "✅ Coverage server downloaded"; \
        echo "🧪 Building with coverage instrumentation..."; \
        CGO_ENABLED=0 go build -cover -covermode=atomic -o <binary-name> <source-files> coverage_server.go; \
    else \
        echo "🚀 Building production binary..."; \
        CGO_ENABLED=0 go build -a -o <binary-name> <source-files>; \
    fi
```

**Important**:
- Replace `<binary-name>` and `<source-files>` with actual values
- Only instrument binaries that run during e2e tests
- Keep other binaries without instrumentation

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
      value: buildah-remote-oci-ta  # Or buildah-oci-ta depending on the pipeline
    - name: bundle
      value: <same-bundle-as-regular-build>
    - name: kind
      value: task
    resolver: bundles
  when:
  - input: $(tasks.init.results.build)
    operator: in
    values:
    - "true"
```

**Key points**:
- Image tagged with `.instrumented` suffix
- `HERMETIC: "false"` is required for downloading coverage server
- Use the same buildah task bundle as the regular build
- `BUILD_ARGS` includes `ENABLE_COVERAGE=true`

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

**B. Update image references** to use instrumented versions:
Find parameters that reference container images and change:
- `container-repo` → `instrumented-container-repo`
- `container-tag` → `instrumented-container-tag`
- `container-image` → `instrumented-container-image`

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

### Step 6: Update GitHub Actions

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

### Step 7: Document Manual Steps

After making all changes, inform the user they need to create a Kubernetes secret:

**Option A - Using kubectl:**
```bash
# First, create the dockerconfig JSON file
cat > /tmp/dockerconfig.json <<EOF
{"auths":{"quay.io":{"auth":"<base64-encoded-quay-user:token>","email":""}}}
EOF

# Create the secret with both keys
kubectl create secret generic coverport-secrets \
  --from-literal=codecov-token=<your-codecov-token> \
  --from-file=oci-storage-dockerconfigjson=/tmp/dockerconfig.json \
  -n <namespace>

# Clean up
rm /tmp/dockerconfig.json
```

**Option B - Using YAML:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: coverport-secrets
  namespace: <namespace>
type: Opaque
stringData:
  codecov-token: <your-codecov-token>
  oci-storage-dockerconfigjson: '{"auths":{"quay.io":{"auth":"<base64-encoded-quay-user:token>","email":""}}}'
```

**Required secret keys:**
- `codecov-token` - Codecov upload token
- `oci-storage-dockerconfigjson` - Docker config JSON for pushing coverage data to OCI storage (quay.io)
  - The `auth` value should be base64-encoded `username:token`
  - To encode: `echo -n "quay-username:quay-token" | base64`

## Validation

After integration, provide verification steps:

1. **Check instrumented image build:**
   - Push a commit to main branch
   - Verify the push pipeline creates an image with `.instrumented` tag
   - Check build logs for "Building with coverage instrumentation..." message

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

**Instrumented build fails:**
- Verify `COVERAGE_SERVER_URL` is accessible
- Check that `wget` is available in the builder image
- Ensure hermetic mode is disabled for instrumented builds

**Coverage data not uploaded:**
- Verify `coverport-secrets` exists in the correct namespace
- Check `codecov-token` key exists in the secret
- Check `oci-storage-dockerconfigjson` key exists and is valid
- Review `collect-and-upload-coverage` task logs for errors

**Coverage data incomplete:**
- Verify e2e tests are using the instrumented image
- Ensure coverage server is properly included in the build
- Check that the correct binaries are instrumented

## Best Practices

1. **Be adaptive** - Repository structures vary, adapt the integration to the specific repository
2. **Ask questions** - If unsure about something, ask the user for clarification
3. **Show diffs** - When modifying files, explain what's changing
4. **Preserve existing logic** - Don't break existing functionality
5. **Handle edge cases** - Check for existing build args, multiple Dockerfiles, etc.
6. **Provide context** - Explain why each change is needed

## Reference Implementation

The reference implementation can be found in the `release-service` repository, commits `1b2208f..dbf965d`.

Key files modified:
- `Dockerfile` - Added coverage instrumentation
- `.tekton/release-service-push.yaml` - Added instrumented image build
- `integration-tests/pipelines/konflux-e2e-tests-pipeline.yaml` - Updated to use test-metadata v0.4 and added coverage collection
- `.github/workflows/codecov.yml` and `.github/workflows/pr.yml` - Added codecov flags

## Examples

### Example 1: Single Binary Repository

For a repository that builds one binary (`manager`):

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

### Example 2: Multiple Binaries

For a repository that builds `manager` and `snapshotgc`, where only `manager` needs coverage:

```dockerfile
ARG ENABLE_COVERAGE=false
ARG COVERAGE_SERVER_URL=https://raw.githubusercontent.com/konflux-ci/coverport/v0.0.1/instrumentation/go/coverage_server.go

RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        wget -q "$COVERAGE_SERVER_URL" -O coverage_server.go; \
        CGO_ENABLED=0 go build -cover -covermode=atomic -a -o manager cmd/main.go coverage_server.go; \
    else \
        CGO_ENABLED=0 go build -a -o manager cmd/main.go; \
    fi \
 && CGO_ENABLED=0 go build -a -o snapshotgc cmd/snapshotgc/snapshotgc.go
```

## Summary

This skill automates coverport integration by:
1. Analyzing the repository structure
2. Asking clarifying questions
3. Modifying the Dockerfile to support coverage builds
4. Adding instrumented image build to Tekton push pipeline
5. Updating e2e pipeline to use test-metadata v0.4 and instrumented images
6. Adding coverage collection task to e2e pipeline
7. Updating GitHub Actions to add codecov flags
8. Providing documentation for manual secret creation

The integration enables automatic e2e test coverage collection and upload to Codecov with proper flag separation from unit tests.
