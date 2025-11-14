# Building and Testing the Coverport Container Image

## Prerequisites

- Docker or Podman
- Access to registry.access.redhat.com (for UBI base images)

## Build the Image

### Using Make (Recommended)

```bash
# Build with default tag (dev)
make docker-build

# Build with specific version
make docker-build VERSION=v1.0.0

# Build and push to registry
make docker-push VERSION=v1.0.0
```

### Using Docker Directly

```bash
# Build
docker build -t quay.io/yourorg/coverport:latest .

# Build with custom registry
docker build -t registry.example.com/yourorg/coverport:v1.0.0 .
```

### Using Podman

```bash
podman build -t quay.io/yourorg/coverport:latest .
```

## Verify the Image

### Check installed tools

```bash
# Run the image and check versions
docker run --rm quay.io/yourorg/coverport:latest --version

# Check individual tools
docker run --rm --entrypoint /bin/sh quay.io/yourorg/coverport:latest -c "
  echo '=== Coverport Version ===' && coverport --version && echo '' &&
  echo '=== Git Version ===' && git --version && echo '' &&
  echo '=== ORAS Version ===' && oras version && echo '' &&
  echo '=== Cosign Version ===' && cosign version && echo '' &&
  echo '=== Go Version ===' && go version
"
```

Expected output:
```
=== Coverport Version ===
coverport version dev (commit: unknown)

=== Git Version ===
git version 2.x.x

=== ORAS Version ===
Version:        1.2.0
Go version:     go1.x.x

=== Cosign Version ===
cosign version 2.4.1

=== Go Version ===
go version go1.24 linux/amd64
```

## Test the Commands

### Test `coverport collect` command

```bash
# Show help
docker run --rm quay.io/yourorg/coverport:latest collect --help

# Test with your kubeconfig (mount it into the container)
docker run --rm \
  -v ~/.kube/config:/tmp/kubeconfig:ro \
  -e KUBECONFIG=/tmp/kubeconfig \
  quay.io/yourorg/coverport:latest \
  discover --namespace=default --label-selector=app=myapp
```

### Test `coverport process` command

```bash
# Show help
docker run --rm quay.io/yourorg/coverport:latest process --help

# Test with local coverage directory
docker run --rm \
  -v $(pwd)/coverage-output:/workspace/coverage:ro \
  -e CODECOV_TOKEN=your-token \
  quay.io/yourorg/coverport:latest \
  process \
    --coverage-dir=/workspace/coverage/myapp/test-123 \
    --repo-url=https://github.com/org/repo \
    --commit-sha=abc123 \
    --codecov-token=$CODECOV_TOKEN \
    --workspace=/workspace/process
```

## Image Size

The UBI9-based image will be larger than the previous distroless image due to the included tools:

- **Previous (distroless)**: ~20MB (binary only)
- **New (UBI9 + tools)**: ~500-600MB (includes git, go, oras, cosign)

This is necessary for the `process` command to function properly.

## Troubleshooting

### Build fails with permission errors

Make sure you're using the correct user in the builder stage:
```dockerfile
COPY --chown=default:root go.mod go.sum ./
```

### Runtime fails to find tools

Verify all tools are in PATH:
```bash
docker run --rm --entrypoint /bin/sh quay.io/yourorg/coverport:latest -c "which git oras cosign go"
```

### Can't access kubeconfig

Mount your kubeconfig and set the environment variable:
```bash
docker run --rm \
  -v ~/.kube/config:/tmp/kubeconfig:ro \
  -e KUBECONFIG=/tmp/kubeconfig \
  quay.io/yourorg/coverport:latest \
  discover --help
```

### Codecov upload fails

Ensure the codecov CLI can be downloaded:
```bash
docker run --rm --entrypoint /bin/sh quay.io/yourorg/coverport:latest -c "curl -I https://cli.codecov.io/latest/linux/codecov"
```

## Multi-Architecture Builds

To build for multiple architectures (amd64, arm64):

```bash
# Using docker buildx
docker buildx create --name coverport-builder --use
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t quay.io/yourorg/coverport:latest \
  --push \
  .
```

## Security Scanning

Scan the image for vulnerabilities:

```bash
# Using Trivy
trivy image quay.io/yourorg/coverport:latest

# Using Grype
grype quay.io/yourorg/coverport:latest

# Using Clair (if available)
clairctl analyze quay.io/yourorg/coverport:latest
```

## Registry Authentication

### Quay.io

```bash
# Login
docker login quay.io

# Push
docker push quay.io/yourorg/coverport:latest
```

### Red Hat Registry (for pulling UBI images)

```bash
# If you need authentication for registry.access.redhat.com
# Usually it's public and doesn't require auth
docker login registry.access.redhat.com
```

## CI/CD Integration

### GitHub Actions Example

```yaml
- name: Build and push coverport image
  uses: docker/build-push-action@v5
  with:
    context: cli/
    push: true
    tags: |
      quay.io/${{ github.repository_owner }}/coverport:latest
      quay.io/${{ github.repository_owner }}/coverport:${{ github.sha }}
```

### Tekton Example

```yaml
- name: build-coverport-image
  taskRef:
    name: buildah
  params:
    - name: IMAGE
      value: quay.io/$(params.IMAGE_ORG)/coverport:$(params.VERSION)
    - name: DOCKERFILE
      value: ./cli/Dockerfile
    - name: CONTEXT
      value: ./cli
```

## Image Optimization Tips

1. **Use build cache**: Docker will cache layers, so unchanged dependencies won't be rebuilt

2. **Multi-stage builds**: Already implemented - builder stage is separate from runtime

3. **Layer ordering**: Dependencies that change less frequently are installed first

4. **Clean up**: The `microdnf clean all` removes package manager cache

## Next Steps

1. Build the image locally and test both commands
2. Push to your registry (quay.io or internal)
3. Update your Tekton pipelines to use the new image
4. Test the full workflow in a pipeline

## Support

For issues with the container image:
- Check the build logs for errors
- Verify all tools are installed with the verification command above
- Test individual tools in an interactive shell: `docker run --rm -it --entrypoint /bin/sh quay.io/yourorg/coverport:latest`

