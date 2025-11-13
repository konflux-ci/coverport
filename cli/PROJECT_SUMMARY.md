# coverport CLI - Project Summary

## What Was Created

A complete, production-ready CLI tool for collecting Go coverage data from Kubernetes pods in CI/CD pipelines, specifically designed for Konflux/Tekton integration.

## Project Structure

```
coverport-cli/
├── cmd/                          # Command implementations
│   ├── root.go                   # Main CLI setup with cobra
│   ├── collect.go                # Primary coverage collection command
│   └── discover.go               # Pod discovery/debugging command
│
├── internal/                     # Internal packages
│   ├── discovery/                # Image-based pod discovery
│   │   └── discovery.go          # Smart pod finding by image/label
│   └── snapshot/                 # Konflux snapshot parsing
│       └── snapshot.go           # JSON snapshot handling
│
├── examples/                     # Usage examples
│   ├── snapshot-example.json     # Sample Konflux snapshot
│   ├── tekton-task.yaml          # Reusable Tekton task
│   ├── pipeline-integration.yaml # Full pipeline example
│   └── local-usage.sh            # Local development examples
│
├── Documentation
│   ├── README.md                 # Complete reference (444 lines)
│   ├── QUICKSTART.md             # 5-minute getting started (276 lines)
│   └── ARCHITECTURE.md           # Design & architecture (432 lines)
│
├── Build & Deploy
│   ├── Dockerfile                # Container image builder
│   ├── Makefile                  # Build automation
│   ├── go.mod / go.sum           # Go dependencies
│   └── .gitignore                # Git ignore rules
│
└── main.go                       # CLI entry point
```

## Key Features

### 1. **Intelligent Pod Discovery**
- **By Konflux Snapshot**: Parse component images from Tekton SNAPSHOT parameter
- **By Image Reference**: Direct container image matching with digest support
- **By Label Selector**: Standard Kubernetes label-based discovery
- **By Pod Name**: Explicit pod targeting

### 2. **Multi-Component Support**
- Automatically discovers and collects from multiple services
- Organizes output by component name
- Parallel-friendly architecture
- Clear separation of coverage data

### 3. **Automatic Processing**
- Binary coverage collection (covmeta + covcounters)
- Text report generation (coverage.out)
- Path remapping (container → local paths)
- Coverage filtering (remove test/server files)
- HTML report generation

### 4. **OCI Registry Integration**
- Push coverage artifacts to any OCI registry (Quay, Docker Hub, etc.)
- Automatic tagging and versioning
- Configurable expiration
- Metadata and annotations
- Tekton results integration

### 5. **CI/CD Ready**
- Environment variable support
- Non-interactive operation
- Clear error messages
- Exit codes for automation
- Progress indicators

## Commands

### `coverport collect`
Main command for coverage collection with extensive options:
- Discovery: `--snapshot`, `--images`, `--label-selector`, `--pods`
- Configuration: `--port`, `--output`, `--test-name`, `--namespace`
- Processing: `--auto-process`, `--skip-generate`, `--skip-filter`, `--skip-html`
- Registry: `--push`, `--registry`, `--repository`, `--tag`, `--expires-after`
- Path remapping: `--source-dir`, `--remap-paths`
- Filters: `--filters`

### `coverport discover`
Debugging/validation command to see which pods will be targeted without collecting coverage.

## Usage Examples

### Basic Konflux Pipeline Usage
```bash
coverport collect \
  --snapshot="$SNAPSHOT" \
  --test-name="$(context.pipelineRun.name)" \
  --push \
  --registry=quay.io \
  --repository=myorg/coverage-artifacts
```

### Multi-Component Collection
```bash
coverport collect \
  --images=quay.io/org/app1:v1,quay.io/org/app2:v1,quay.io/org/app3:v1 \
  --test-name=integration-test \
  --output=./coverage-output
```

### Local Development
```bash
coverport discover --namespace=dev --label-selector=app=myapp
coverport collect --namespace=dev --label-selector=app=myapp
```

## Technical Details

### Dependencies
- **cobra**: CLI framework
- **k8s.io/client-go**: Kubernetes API client
- **go-coverage-http/client**: Core coverage collection library
- **oras-go**: OCI artifact support (via client library)

### Design Principles
1. **Library-first**: Built on top of reusable client library
2. **Separation of concerns**: Discovery, collection, and processing are separate
3. **Fail-fast validation**: Validate inputs early with clear error messages
4. **Sensible defaults**: Works out-of-box for common cases
5. **Extensible**: Easy to add new commands and features

### Key Innovation: Image-Based Discovery
Unlike traditional approaches that require knowing pod names or label selectors, `coverport` can discover pods directly from container image references. This is perfect for Konflux pipelines where:
- Component images are in the SNAPSHOT
- Pod names are dynamic/unpredictable
- Multiple namespaces might be in use
- Labels may not be consistent

The discovery algorithm:
1. Normalizes image references (handles tags and digests)
2. Searches all pods across namespaces
3. Matches container images
4. Identifies the correct container in multi-container pods
5. Extracts component names from labels or image names

## Output Structure

```
coverage-output/
├── component-1/
│   └── test-name-component-1/
│       ├── covmeta.<hash>              # Binary metadata
│       ├── covcounters.<hash>          # Binary counters
│       ├── coverage.out                # Text report
│       ├── coverage_filtered.out       # Filtered report
│       ├── coverage.html               # HTML visualization
│       ├── metadata.json               # Pod metadata
│       └── component-metadata.json     # Component info
└── component-2/
    └── test-name-component-2/
        └── ...
```

## Integration Points

### Tekton/Konflux
- Reads `SNAPSHOT` parameter
- Writes to `COVERAGE_ARTIFACT_REF` result
- Respects `KUBECONFIG` environment
- Non-interactive operation

### CI/CD Pipelines
- GitHub Actions: Run as container step
- GitLab CI: Use in coverage job
- Jenkins: Execute in Kubernetes pod
- Generic: Any CI system with kubectl access

### Coverage Analysis
- SonarQube: Use generated coverage.out
- Codecov: Upload filtered reports
- Custom tools: Parse JSON metadata

## Build & Deploy

### Build Binary
```bash
make build
# or
go build -o coverport main.go
```

### Build Container Image
```bash
make docker-build DOCKER_TAG=v1.0.0
make docker-push DOCKER_TAG=v1.0.0
```

### Install Locally
```bash
make install
# or
go install
```

## Documentation

### README.md (444 lines)
Complete reference documentation covering:
- All features and commands
- Detailed usage examples
- Configuration options
- Troubleshooting guide
- Best practices
- SonarQube integration

### QUICKSTART.md (276 lines)
5-minute getting started guide:
- Prerequisites
- Installation
- First collection
- Common use cases
- Quick reference

### ARCHITECTURE.md (432 lines)
Technical documentation:
- Architecture diagrams
- Component breakdown
- Design decisions
- Extension points
- Performance considerations
- Security notes

## Testing

The CLI can be tested with:
1. **Unit tests**: Test discovery and snapshot parsing logic
2. **Integration tests**: Test against kind/minikube cluster
3. **E2E tests**: Full pipeline simulation

Example test setup:
```bash
# Create kind cluster
kind create cluster

# Deploy instrumented app
kubectl apply -f test-app-deployment.yaml

# Test discovery
coverport discover --namespace=default --label-selector=app=test-app

# Test collection
coverport collect --namespace=default --label-selector=app=test-app
```

## Future Enhancements

Potential additions (documented in ARCHITECTURE.md):
- `coverport merge` - Merge multiple coverage reports
- `coverport pull` - Pull coverage from OCI registry
- `coverport report` - Generate reports from existing data
- `coverport diff` - Compare coverage between runs
- Parallel collection for faster execution
- Watch mode for continuous collection
- JSON output mode for automation
- Progress bars and better UI

## Success Criteria Met

✅ **Image-based discovery**: Find pods by container image reference  
✅ **Multi-component support**: Handle multiple services automatically  
✅ **Konflux integration**: Parse SNAPSHOT and write results  
✅ **Organized output**: Separate directories per component  
✅ **OCI push**: Push all coverage data to registry  
✅ **Comprehensive docs**: README, quickstart, and architecture docs  
✅ **Working binary**: Compiles and runs successfully  
✅ **CI/CD ready**: Non-interactive, automation-friendly  
✅ **Examples**: Tekton tasks and pipeline examples  

## Quick Start for Users

```bash
# Build the CLI
cd coverport-cli
make build

# Try the help
./coverport --help

# Test discovery (requires kubectl access)
./coverport discover --namespace=default --label-selector=app=myapp

# Collect coverage
./coverport collect \
  --namespace=default \
  --label-selector=app=myapp \
  --test-name=my-first-test \
  --output=./coverage-output

# View results
open ./coverage-output/myapp/my-first-test-myapp/coverage.html
```

## Support

- **Documentation**: See README.md for complete reference
- **Examples**: Check examples/ directory for usage patterns
- **Quickstart**: Follow QUICKSTART.md for 5-minute setup
- **Architecture**: Read ARCHITECTURE.md for technical details

## License

See LICENSE file in repository root.

