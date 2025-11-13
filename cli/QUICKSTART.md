# coverport - Quick Start Guide

Get started with `coverport` in 5 minutes!

## Prerequisites

1. **Instrumented Go Application**: Your app must be built with coverage:
   ```bash
   go build -cover -o app
   ```

2. **Coverage Server Running**: Your app must run the coverage HTTP server (port 9095)

3. **Kubernetes Access**: Valid kubeconfig with access to the cluster

4. **Application Deployed**: Your instrumented app running in Kubernetes

## Step 1: Install coverport

```bash
# From source
cd coverport-cli
make build
sudo mv coverport /usr/local/bin/

# Or using go install
go install github.com/konflux-ci/coverport/cli@latest
```

Verify installation:
```bash
coverport --version
```

## Step 2: Discover Your Pods

Before collecting coverage, discover which pods will be targeted:

```bash
# By label selector
coverport discover \
  --namespace=default \
  --label-selector=app=myapp

# By container image
coverport discover \
  --images=quay.io/myorg/myapp:v1.0.0
```

Expected output:
```
🔍 coverport - Pod Discovery
─────────────────────────────
✅ Discovered 2 pod(s):

📦 Component: myapp
   • Pod: default/myapp-pod-1
     Container: app
     Image: quay.io/myorg/myapp:v1.0.0
   • Pod: default/myapp-pod-2
     Container: app
     Image: quay.io/myorg/myapp:v1.0.0
```

## Step 3: Collect Coverage

### Option A: Using Label Selector

```bash
coverport collect \
  --namespace=default \
  --label-selector=app=myapp \
  --test-name=my-test \
  --output=./coverage-output
```

### Option B: Using Container Images

```bash
coverport collect \
  --images=quay.io/myorg/myapp:v1.0.0 \
  --test-name=my-test \
  --output=./coverage-output
```

### Option C: Using Konflux Snapshot (CI/CD)

```bash
export SNAPSHOT='{"components":[{"name":"myapp","containerImage":"quay.io/myorg/myapp@sha256:abc123"}]}'

coverport collect \
  --snapshot="$SNAPSHOT" \
  --test-name=e2e-test \
  --output=./coverage-output
```

Expected output:
```
🚀 coverport - Coverage Collection Tool
============================================================
Test Name:     my-test
Output Dir:    ./coverage-output
Coverage Port: 9095
============================================================

📍 Discovered 1 pod(s) for coverage collection:
  1. default/myapp-pod-1 (component: myapp, image: quay.io/myorg/myapp:v1.0.0)

📊 Collecting from: default/myapp-pod-1 (component: myapp)
✅ Port forward ready: localhost:54321 -> pod:9095
📁 Saved: ./coverage-output/myapp/my-test-myapp/covmeta.xxx
📁 Saved: ./coverage-output/myapp/my-test-myapp/covcounters.xxx
📁 Saved: ./coverage-output/myapp/my-test-myapp/metadata.json
📊 Generating coverage report...
✅ Coverage report generated: ./coverage-output/myapp/my-test-myapp/coverage.out
✅ Filtered coverage report: ./coverage-output/myapp/my-test-myapp/coverage_filtered.out
✅ HTML report generated: ./coverage-output/myapp/my-test-myapp/coverage.html

✅ Coverage collection complete!
📁 Coverage data saved to: ./coverage-output
```

## Step 4: View Coverage Report

### Text Report

```bash
cat ./coverage-output/myapp/my-test-myapp/coverage_filtered.out
```

### HTML Report

```bash
open ./coverage-output/myapp/my-test-myapp/coverage.html
```

## Step 5: Push to OCI Registry (Optional)

Push coverage artifact to a container registry:

```bash
# Login to registry first
docker login quay.io

# Collect and push
coverport collect \
  --snapshot="$SNAPSHOT" \
  --test-name=e2e-test \
  --output=./coverage-output \
  --push \
  --registry=quay.io \
  --repository=myorg/coverage-artifacts \
  --tag=e2e-coverage-$(date +%Y%m%d-%H%M%S) \
  --expires-after=30d
```

## Common Use Cases

### Local Development Testing

```bash
# Quick coverage check during development
coverport collect \
  --namespace=dev \
  --label-selector=app=myapp,version=dev \
  --test-name=dev-test \
  --output=./coverage
```

### CI/CD Pipeline (Konflux/Tekton)

```yaml
# In your Tekton task
- name: collect-coverage
  image: quay.io/myorg/coverport:latest
  env:
    - name: SNAPSHOT
      value: $(params.SNAPSHOT)
  script: |
    #!/bin/sh
    coverport collect \
      --snapshot="$SNAPSHOT" \
      --test-name="$(context.pipelineRun.name)" \
      --push \
      --registry=quay.io \
      --repository=myorg/coverage-artifacts
```

### Multiple Components

When testing a microservices application:

```bash
coverport collect \
  --images=quay.io/myorg/frontend:latest,quay.io/myorg/backend:latest,quay.io/myorg/worker:latest \
  --test-name=integration-test \
  --output=./coverage-output
```

Output structure:
```
coverage-output/
├── frontend/
│   └── integration-test-frontend/
│       └── ... (coverage files)
├── backend/
│   └── integration-test-backend/
│       └── ... (coverage files)
└── worker/
    └── integration-test-worker/
        └── ... (coverage files)
```

## Troubleshooting

### No Pods Found

**Problem**: `No running pods found matching the criteria`

**Solution**:
1. Check your label selector or image reference
2. Verify pods are in `Running` state
3. Try `coverport discover` first to debug

### Coverage Collection Fails

**Problem**: `Failed to collect from pod`

**Solution**:
1. Verify coverage server is running in the pod:
   ```bash
   kubectl port-forward pod/myapp-pod-1 9095:9095
   curl http://localhost:9095/coverage
   ```
2. Check `GOCOVERDIR` is set in container
3. Verify app was built with `-cover` flag

### Permission Denied

**Problem**: `Failed to setup port forward`

**Solution**:
1. Check kubeconfig is valid: `kubectl get pods`
2. Verify you have port-forward permissions
3. Check if running in-cluster vs local

## Next Steps

- Read the full [README.md](./README.md) for detailed documentation
- Check [examples/](./examples/) for more usage patterns
- Set up automated coverage collection in your CI/CD pipeline
- Integrate with SonarQube or other coverage analysis tools

## Getting Help

- **Documentation**: See [README.md](./README.md)
- **Examples**: Check [examples/](./examples/) directory
- **Issues**: Open an issue in the GitHub repository

## Quick Reference

```bash
# Discover pods
coverport discover --namespace=NS --label-selector=SELECTOR

# Collect coverage
coverport collect --snapshot="$SNAPSHOT" --test-name=NAME

# Collect and push
coverport collect --snapshot="$SNAPSHOT" --push --repository=REPO

# Help
coverport --help
coverport collect --help
```

