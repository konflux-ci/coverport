# Coverage Processing for container images built by Konflux

> **Status**: ✅ POC Successfully Implemented and Tested End-to-End

Event-driven coverage processing system for Konflux-built applications. Automatically processes coverage artifacts from Quay.io, extracts Git metadata from SLSA attestations, and uploads remapped coverage to SonarCloud.

## 🚀 What It Does

1. **Listens** for Quay.io webhook events when coverage artifacts are pushed
2. **Extracts** coverage data and metadata from OCI artifacts using ORAS
3. **Resolves** Git repository and commit from Konflux image attestations using cosign
4. **Clones** the source repository at the specific commit
5. **Remaps** coverage paths from container to source code using `go tool covdata`
6. **Uploads** coverage to SonarCloud for analysis

## 📋 Prerequisites

- OpenShift/Kubernetes cluster with Tekton Pipelines installed
- Quay.io repository for coverage artifacts
- SonarCloud account and project
- Smee.io channel for webhook forwarding (or direct webhook access)

## 🛠️ Quick Start

### 1. Get Prerequisites

Before deployment, gather these required values:

1. **Smee.io Channel** - Free webhook relay:
   - Visit https://smee.io/new
   - Copy your channel URL (e.g., `https://smee.io/abc123xyz`)

2. **SonarCloud Token** - Authentication token:
   - Visit https://sonarcloud.io/account/security
   - Generate a new token
   - Copy the token value

### 2. Deploy Coverage Processor

The deployment script will interactively prompt for configuration:

```bash
./deploy.sh
```

**What it asks:**
- Smee.io channel URL (required)
- SonarCloud token (optional, can configure later)

Configuration is auto-saved to `config.env` for future deployments.

**Alternative: Use config file**

```bash
# Create from template
cp config.env.example config.env

# Edit with your values
vi config.env

# Deploy (reads config.env automatically)
./deploy.sh
```

**Expected output:**
- ✓ Namespace `coverage-processor` created
- ✓ RBAC configured
- ✓ Secret `sonar-token` created
- ✓ gosmee forwarder running with your Smee.io channel
- ✓ EventListener `el-coverage-listener` running

### 3. Verify Deployment

```bash
# Check all resources
oc get all -n coverage-processor

# Verify gosmee is forwarding webhooks
oc logs -n coverage-processor -l component=gosmee-forwarder -f
# Should show: "Forwarding https://smee.io/YOUR_CHANNEL to http://el-coverage-listener..."
```

### 4. Test Manually

```bash
# Edit with your coverage artifact reference
vi examples/manual-taskrun.yaml
# Replace __COVERAGE_ARTIFACT_REF__ with: quay.io/your-org/coverage-artifacts:tag

# Run the task
oc apply -f examples/manual-taskrun.yaml

# Watch the logs
tkn tr logs -f -n coverage-processor coverage-manual-test
```

**Expected result:** Coverage uploaded to SonarCloud in ~20 seconds

### 5. Configure Quay.io Webhook

Set up automatic processing when you push coverage artifacts:

1. Go to your Quay.io repository settings → Webhooks
2. Click "Create Webhook"
3. **Webhook URL**: Your Smee.io channel (from step 1)
4. **Event**: Select "Repository Push"
5. Save and test

**Test end-to-end:**
```bash
# Push a coverage artifact
oras push quay.io/yourorg/coverage-artifacts:test \
  ./covcounters.* \
  ./covmeta.* \
  ./metadata.json

# Watch for automatic TaskRun creation
tkn tr list -n coverage-processor -w
```

## 📁 Repository Structure

```
coverage-processor/
├── k8s/                          # Kubernetes manifests
│   ├── namespace.yaml            # coverage-processor namespace
│   ├── rbac.yaml                 # ServiceAccount and RBAC permissions
│   ├── config.yaml               # SonarCloud secret template
│   └── gosmee-deployment.yaml    # Webhook forwarder (Smee.io client)
├── tekton/                       # Tekton resources
│   ├── eventlistener.yaml        # Receives and processes webhooks
│   ├── trigger-binding.yaml      # Extracts coverage-artifact-ref parameter
│   ├── trigger-template.yaml     # Creates TaskRuns from webhook events
│   └── tasks/
│       └── coverage-task.yaml   # Main processing task (6 steps)
├── examples/                     # Example configurations
│   ├── manual-taskrun.yaml       # Manual test TaskRun
│   └── test-secret.yaml          # SonarCloud secret template
├── deploy.sh                     # One-command deployment script
└── README.md                     # This file
```

## 🔧 How It Works

### Architecture Overview

```
┌─────────────┐
│  Quay.io    │  Push coverage artifact
│  Registry   │
└──────┬──────┘
       │ webhook
       ↓
┌─────────────┐
│  Smee.io    │  Public webhook relay
│  Channel    │
└──────┬──────┘
       │ forward
       ↓
┌─────────────────────┐
│  gosmee Forwarder   │  In-cluster forwarder
│  (Deployment)       │
└──────┬──────────────┘
       │ HTTP POST
       ↓
┌─────────────────────┐
│  EventListener      │  Tekton Triggers
│  (el-coverage)     │
└──────┬──────────────┘
       │ create
       ↓
┌─────────────────────┐
│  TaskRun            │  Single pod, 6 steps
│  (coverage-task)   │
└─────────────────────┘
       │
       ├─→ 1. Extract coverage artifact (ORAS)
       ├─→ 2. Extract Git metadata (cosign)
       ├─→ 3. Clone repository (git)
       ├─→ 4. Fetch SonarCloud token (oc)
       ├─→ 5. Process coverage (go tool covdata)
       └─→ 6. Upload to SonarCloud (sonar-scanner)
```

### Processing Steps

The `coverage-task` executes these steps sequentially in a single pod with shared `/workspace` emptyDir volume:

#### 1. Extract Coverage Artifact
```bash
# Uses ORAS CLI to pull from Quay.io
oras pull quay.io/repo/coverage-artifacts:tag
# Extracts: covcounters.*, covmeta.*, metadata.json
```

**Reads** app image reference from `metadata.json`:
```json
{
  "container": {
    "image": "quay.io/org/app:sha-abc123"
  }
}
```

#### 2. Extract Git Metadata
```bash
# Downloads SLSA provenance attestation
cosign download attestation quay.io/org/app:sha-abc123

# Extracts Konflux-specific annotations
jq '.predicate.buildConfig.tasks[0].invocation.environment.annotations'
```

**Extracts** from attestation:
- `pipelinesascode.tekton.dev/repo-url` → Git repository URL
- `build.appstudio.redhat.com/commit_sha` → Git commit SHA

#### 3. Clone Repository
```bash
# Clones at specific commit from attestation
git clone <repo-url> /workspace/repo
cd /workspace/repo
git checkout <commit-sha>
```

#### 4. Fetch SonarCloud Token
```bash
# Retrieves credentials from Kubernetes secret
oc get secret sonar-token -n coverage-processor \
  -o jsonpath="{.data.SONAR_TOKEN}" | base64 -d
```

#### 5. Process Coverage
```bash
# Remaps container paths to source paths
cd /workspace/repo
export GOCOVERDIR=/workspace/coverage-raw
go tool covdata textfmt -i=$GOCOVERDIR -o=/workspace/coverage.out
```

**Why remapping?** Coverage was collected in the container at paths like `/app/`, but SonarCloud needs paths relative to the source repository.

#### 6. Upload to SonarCloud
```bash
# Downloads and runs SonarScanner CLI
sonar-scanner \
  -Dsonar.host.url="https://sonarcloud.io" \
  -Dsonar.token="$SONAR_TOKEN" \
  -Dsonar.projectKey="org_project" \
  -Dsonar.go.coverage.reportPaths=coverage.out \
  -Dsonar.scm.revision="$COMMIT_SHA"
```

Result: Coverage uploaded with commit context for accurate tracking.

### Coverage Artifact Format

Your coverage artifacts must be OCI images pushed to Quay.io containing:

**Required files:**
- `covcounters.*` - Go binary coverage counter files
- `covmeta.*` - Go binary coverage metadata
- `metadata.json` - Container metadata with app image reference

**Example `metadata.json`:**
```json
{
  "pod_name": "my-app-pod",
  "namespace": "my-app-namespace",
  "container": {
    "name": "app",
    "image": "quay.io/org/my-app:sha-abc123"
  },
  "collected_at": "2025-11-10T08:30:15+01:00",
  "test_name": "e2e-tests"
}
```

**How to create coverage artifacts:**

See the [go-coverage-http](https://github.com/konflux-ci/coverport) project for a complete example of:
- Instrumenting Go applications with `-cover`
- Collecting coverage from running containers
- Packaging coverage as OCI artifacts
- Pushing to Quay.io with webhooks

## 🎯 POC Validation Results

### Test Run: November 10, 2025

**Input:**
- Coverage Artifact: `quay.io/psturc/coverage-artifacts:e2e-coverage-20251110_0959`
- App Image: `quay.io/redhat-user-workloads-stage/psturc-tenant/go-coverage-http:on-pr-b3f832...`
- Repository: `https://github.com/konflux-ci/coverport`
- Commit: `b3f832262d76372540118fc278dff1ae42edd6be`

**Results:**
```
✅ Coverage artifact downloaded (0.5s)
✅ Git metadata extracted from attestation (0.5s)
✅ Repository cloned at commit (2s)
✅ SonarCloud token retrieved (0.2s)
✅ Coverage processed and remapped (0.5s)
✅ Uploaded to SonarCloud (14s)

INFO: ANALYSIS SUCCESSFUL
Total time: 18.584s
View: https://sonarcloud.io/dashboard?id=psturc_go-coverage-http
```

### What Was Validated

✅ **OCI Artifact Extraction** - ORAS CLI successfully pulled from Quay.io  
✅ **Metadata Parsing** - Extracted app image from `metadata.json`  
✅ **Attestation Verification** - cosign retrieved SLSA provenance  
✅ **Git Metadata Extraction** - Parsed Konflux annotations  
✅ **Repository Cloning** - Git clone at specific commit  
✅ **Secret Management** - Fetched token from Kubernetes secret  
✅ **Coverage Remapping** - `go tool covdata` converted paths  
✅ **SonarCloud Integration** - Successful upload with commit context  

## 🎯 Current POC Scope

### Supported ✅
- **Language**: Go projects with `-cover` instrumentation
- **Provider**: SonarCloud (cloud-hosted)
- **Images**: Konflux-built with SLSA attestations
- **Repositories**: Public (HTTPS clone)
- **Webhooks**: Quay.io → Smee.io → EventListener
- **Secret**: Single fixed `sonar-token` secret

### Not Yet Implemented ❌
- **Languages**: Python, JavaScript/TypeScript, Java
- **Providers**: Codecov, SonarQube self-hosted
- **Secrets**: Dynamic resolution via ConfigMap
- **Repositories**: Private (SSH key or PAT required)
- **Pull Requests**: Coverage comments on PRs
- **Quality Gates**: Fail on coverage decrease
- **Notifications**: Slack/email on failures

## 🔍 Debugging

### View TaskRuns

```bash
# List all runs
tkn tr list -n coverage-processor

# View logs of latest run
tkn tr logs -f -n coverage-processor \
  $(oc get tr -n coverage-processor \
     --sort-by=.metadata.creationTimestamp \
     -o name | tail -1 | cut -d/ -f2)

# Describe a specific run
tkn tr describe <taskrun-name> -n coverage-processor

# Check status
oc get taskrun -n coverage-processor
```

### View EventListener Logs

```bash
# Check if webhooks are being received
oc logs -f deployment/el-coverage-listener -n coverage-processor

# Look for these messages:
# - "dev.tekton.event.triggers.started.v1"
# - "dev.tekton.event.triggers.done.v1"
```

### View gosmee Forwarder Logs

```bash
# Check webhook forwarding
oc logs -f deployment/gosmee-forwarder -n coverage-processor

# Look for:
# - "request replayed to ..., status: 202"
```

### Common Issues

**Issue**: TaskRun not created after webhook
- Check EventListener logs for CEL interceptor errors
- Verify `coverage_artifact_ref` is constructed correctly
- Test webhook payload with: `curl -X POST http://el-coverage-listener... -d @payload.json`

**Issue**: "File '/app/coverage_server.go' is not included in the project"
- This is a warning, not an error
- Coverage file references container paths that don't exist in source
- Non-critical: SonarCloud will ignore these paths

**Issue**: "Unable to create user cache: /home/tool-box/.sonar/cache"
- Fixed by setting `SONAR_USER_HOME=/workspace/.sonar`
- Allows SonarScanner to write cache to writable workspace

**Issue**: "Error: Could not extract repo URL"
- Image was not built by Konflux
- Attestation doesn't contain required annotations
- Use manual parameters or different metadata source

## 🚀 Deployment

### Prerequisites Check

```bash
# Verify Tekton Pipelines installed
oc get deployment -n openshift-pipelines tekton-pipelines-controller

# Verify tekton CLI available
tkn version

# Verify oc CLI available
oc version
```

### Deploy Everything

```bash
# One command deployment
./deploy.sh

# Or manually
oc apply -f k8s/
oc apply -f tekton/
oc apply -f tekton/tasks/
```

### Create Secret

```bash
# Create from literal
oc create secret generic sonar-token \
  --from-literal=SONAR_TOKEN=your-token-here \
  --from-literal=SONAR_HOST_URL=https://sonarcloud.io \
  -n coverage-processor

# Or apply from template
cp examples/test-secret.yaml sonar-token.yaml
# Edit sonar-token.yaml with your actual token
oc apply -f sonar-token.yaml
```

### Verify Deployment

```bash
# Check all resources
oc get all -n coverage-processor

# Expected output:
# - deployment.apps/gosmee-forwarder (1/1 Running)
# - deployment.apps/el-coverage-listener (1/1 Running)
# - service/el-coverage-listener (ClusterIP)

# Check RBAC
oc get sa,role,rolebinding -n coverage-processor

# Check task installed
oc get task coverage-task -n coverage-processor
```

## 📦 Technical Details

### Why Single TaskRun Instead of Pipeline?

**Initial approach**: Pipeline with 2 Tasks + PVC workspace  
**Current approach**: Single Task with 6 Steps + emptyDir volume

**Benefits:**
- ⚡ **Faster**: No pod startup/teardown between tasks (~2-3s saved)
- 💰 **Cheaper**: No PVC creation/deletion overhead
- 🐛 **Easier to debug**: Single pod, single log stream
- 🔒 **More reliable**: Direct filesystem sharing, no workspace transfer

### Konflux Attestation Structure

Konflux builds generate SLSA provenance attestations with this structure:

```json
{
  "payload": "<base64-encoded>",
  "payloadType": "application/vnd.in-toto+json",
  "signatures": [...]
}
```

After decoding `payload`:
```json
{
  "predicate": {
    "buildConfig": {
      "tasks": [
        {
          "invocation": {
            "environment": {
              "annotations": {
                "pipelinesascode.tekton.dev/repo-url": "https://github.com/org/repo",
                "build.appstudio.redhat.com/commit_sha": "abc123...",
                "pipelinesascode.tekton.dev/sha": "abc123...",
                "build.appstudio.redhat.com/pull_request_number": "42",
                ...
              }
            }
          }
        }
      ]
    }
  }
}
```

**Key fields used:**
- `pipelinesascode.tekton.dev/repo-url` - Git repository URL
- `build.appstudio.redhat.com/commit_sha` - Git commit SHA

### Tool Versions

The `quay.io/konflux-ci/tekton-integration-catalog/utils:latest` image includes:

- **oras**: OCI Registry As Storage CLI
- **cosign**: Container signing and attestation verification
- **jq**: JSON processor
- **oc**: OpenShift CLI
- **git**: Version control
- **curl**, **unzip**: Utilities for SonarScanner download

## 🤝 Contributing

This is a POC demonstrating the core concept. Future enhancements:

### Priority 1 - Multi-language Support
- Python (coverage.py → coverage.xml)
- JavaScript/TypeScript (istanbul/nyc → lcov.info)
- Java (JaCoCo → jacoco.xml)

### Priority 2 - Multi-provider Support
- Codecov integration
- SonarQube self-hosted
- Coveralls
- Code Climate

### Priority 3 - Enhanced Features
- ConfigMap for repo → secret mapping
- Private repository support (SSH keys)
- Pull request coverage comments
- Coverage quality gates
- Slack/email notifications

## 📄 License

Apache 2.0

## 🙏 Acknowledgments

- **Konflux** - CI/CD build system with SLSA attestations
- **Tekton** - Cloud-native pipeline orchestration
- **ORAS** - OCI Registry As Storage for artifact handling
- **cosign** - Container signing and verification from Sigstore
- **SonarCloud** - Code quality and coverage analysis platform
- **Smee.io** - Webhook payload delivery service

---

**Questions or issues?** Check the debugging section above or review the example configurations in `examples/`.
