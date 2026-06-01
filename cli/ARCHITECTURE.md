# coverport CLI Architecture

## Overview

`coverport` is a command-line interface for collecting code coverage from Go and Python applications running in Kubernetes, designed specifically for Konflux/Tekton integration pipelines and CI/CD automation.

## Recent Changes

**v2 Architecture Updates**:
- Added `process` command for post-collection processing (git metadata extraction, cloning, coverage mapping, Codecov upload)
- Introduced manifest-based workflow: `collect` creates `metadata.json` for batch processing
- Added direct URL collection via `NewClientForURL()` for localhost/HTTP endpoints
- Automatic PR detection from image metadata (Konflux annotations, branch patterns)
- Intelligent path remapping with `./` prefix for Go tooling compatibility
- HTML generation moved to `process` phase for proper source code access

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                     coverport CLI                            │
│                                                              │
│  ┌────────────┐  ┌────────────┐  ┌──────────────────────┐  │
│  │   collect  │  │  discover  │  │  (future commands)   │  │
│  └─────┬──────┘  └─────┬──────┘  └──────────────────────┘  │
│        │               │                                     │
│  ┌─────▼───────────────▼────────────────────────────────┐  │
│  │             Command Layer (cobra)                     │  │
│  └─────┬────────────────────────────────────────────────┘  │
│        │                                                     │
│  ┌─────▼────────────────────────────────────────────────┐  │
│  │           Internal Packages                           │  │
│  │  ┌──────────────────┐  ┌────────────────────────┐    │  │
│  │  │  discovery/      │  │  snapshot/             │    │  │
│  │  │  - Image-based   │  │  - Snapshot parsing    │    │  │
│  │  │    pod discovery │  │  - Component extraction│    │  │
│  │  └──────────────────┘  └────────────────────────┘    │  │
│  └─────┬────────────────────────────────────────────────┘  │
└────────┼──────────────────────────────────────────────────┘
         │
         │ Uses
         ▼
┌─────────────────────────────────────────────────────────────┐
│            go-coverage-http/client Library                   │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  • CoverageClient                                     │  │
│  │  • Pod port-forwarding                                │  │
│  │  • HTTP coverage collection                           │  │
│  │  • Report generation & filtering                      │  │
│  │  • Path remapping                                     │  │
│  │  • OCI artifact push                                  │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
         │
         │ Uses
         ▼
┌─────────────────────────────────────────────────────────────┐
│                 Kubernetes & Go Tools                        │
│  • k8s.io/client-go  • oras-go  • go tool covdata          │
└─────────────────────────────────────────────────────────────┘
```

### Language-Specific Collection Flows

**Go applications:**
1. POST `/coverage` → binary coverage data (covmeta + covcounters)
2. `go tool covdata` converts to text/HTML reports locally

**Python applications:**
1. GET `/health` → auto-detects Python coverage server
2. POST `/coverage/save` → triggers SIGHUP to Gunicorn (workers save coverage to `/dev/shm`)
3. GET `/coverage` → base64-encoded combined coverage data
4. `kubectl exec` runs `coverage xml` inside the pod to generate Cobertura XML
5. XML is fetched from the pod and saved locally

The Python flow generates XML inside the target pod because the CoverPort CLI container does not include Python. This is by design — it avoids adding Python dependencies to the CLI image while leveraging the Python runtime already present in the instrumented pod.

## Components

### 1. Command Layer (`cmd/`)

#### `root.go`
- Main CLI entry point
- Global flags and configuration
- Version information
- Help system

#### `collect.go`
- Primary command for coverage collection
- Handles multiple discovery methods:
  - Konflux/Tekton snapshots
  - Container image references
  - Label selectors
  - Explicit pod names
- Orchestrates multi-component collection
- Manages OCI artifact push

#### `discover.go`
- Pod discovery without collection
- Useful for debugging and validation
- Shows what pods will be targeted

### 2. Internal Packages

#### `internal/discovery/`
**Purpose**: Intelligent pod discovery based on various criteria

**Key Features**:
- **Image-based discovery**: Normalizes and matches container images
- **Cross-namespace search**: Searches all non-system namespaces
- **Label selector support**: Standard Kubernetes label matching
- **Component extraction**: Identifies component names from labels or images

**Main Types**:
```go
type PodInfo struct {
    Name          string
    Namespace     string
    ComponentName string
    Image         string
    ContainerName string
}

type ImageDiscovery struct {
    clientset kubernetes.Interface
}
```

#### `internal/snapshot/`
**Purpose**: Parse and process Konflux/Tekton snapshots

**Key Features**:
- JSON snapshot parsing
- Component extraction
- Image list generation
- File and string input support

**Main Types**:
```go
type Snapshot struct {
    Components []Component `json:"components"`
}

type Component struct {
    Name           string `json:"name"`
    ContainerImage string `json:"containerImage"`
    Source         Source `json:"source,omitempty"`
}
```

## Design Decisions

### 1. CLI vs Library

**When to use the CLI:**
- CI/CD pipelines (Tekton, GitHub Actions, etc.)
- One-off manual coverage collection
- Multi-component applications
- Need for snapshot parsing
- OCI registry push automation

**When to use the library directly:**
- Custom Go test code (like e2e_test.go)
- Need programmatic control
- Custom workflows
- Integration into existing Go applications

### 2. Discovery Strategy

The CLI implements a layered discovery approach:

1. **Snapshot-first**: Konflux snapshots are the primary method
2. **Image-based**: Direct image reference matching
3. **Label-based**: Kubernetes label selectors
4. **Explicit**: Manual pod specification

This prioritization reflects the most common CI/CD use cases.

### 3. Multi-Component Support

Key design for multi-component handling:

```
coverage-output/
├── component-1/
│   └── test-name-component-1/
│       └── (coverage files)
├── component-2/
│   └── test-name-component-2/
│       └── (coverage files)
└── component-3/
    └── test-name-component-3/
        └── (coverage files)
```

**Benefits**:
- Clear separation of concerns
- Parallel processing friendly
- Easy to identify failures
- Simple to merge or analyze separately

### 4. Automatic Processing

By default, the CLI auto-processes coverage:
1. Collect binary data
2. Generate text report
3. Remap paths
4. Filter unwanted files
5. Create HTML visualization

**Rationale**: Most CI/CD use cases want complete reports without manual intervention.

### 5. OCI Artifact Support

Built-in OCI push for:
- Persistent storage
- Integration with registry workflows
- Artifact metadata and annotations
- Automatic expiration

## Workflow Examples

### Standard CI/CD Flow

```
┌─────────────────┐
│  Deploy Apps    │
│  (Tekton Task)  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Run Tests     │
│  (Tekton Task)  │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│   coverport collect                     │
│   --snapshot="$SNAPSHOT"                │
│   ↓                                     │
│   1. Parse snapshot                     │
│   2. Discover pods (by image)           │
│   3. Collect from each pod              │
│   4. Process reports                    │
│   5. Push to OCI registry               │
└────────┬────────────────────────────────┘
         │
         ▼
┌─────────────────┐
│  Analyze        │
│  (SonarQube)    │
└─────────────────┘
```

### Local Development Flow

```
┌─────────────────┐
│  Deploy to      │
│  Local Cluster  │
│  (kind/minikube)│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Manual Testing │
└────────┬────────┘
         │
         ▼
┌──────────────────────────────────────┐
│   coverport collect                  │
│   --label-selector="app=myapp"       │
│   ↓                                  │
│   1. Discover pods (by label)        │
│   2. Collect coverage                │
│   3. Generate reports                │
│   4. Open HTML (no push)             │
└────────┬─────────────────────────────┘
         │
         ▼
┌─────────────────┐
│  Review in      │
│  Browser        │
└─────────────────┘
```

## Extension Points

### Adding New Commands

To add new commands (e.g., `coverport merge`, `coverport report`):

1. Create `cmd/newcommand.go`
2. Implement cobra command
3. Add to `rootCmd` in `init()`
4. Use existing internal packages

Example:
```go
// cmd/merge.go
var mergeCmd = &cobra.Command{
    Use:   "merge",
    Short: "Merge coverage from multiple sources",
    Run:   runMerge,
}

func init() {
    rootCmd.AddCommand(mergeCmd)
}
```

### Adding New Discovery Methods

To add new discovery methods:

1. Add method to `internal/discovery/discovery.go`
2. Add flags to `cmd/collect.go`
3. Add to discovery validation logic

### Custom Processing

To add custom report processing:

1. Use the library's `CoverageClient` directly
2. Implement custom logic in `cmd/collect.go`
3. Add flags for configuration

## Comparison: CLI vs Library Usage

### Using the CLI

```bash
coverport collect \
  --snapshot="$SNAPSHOT" \
  --test-name=e2e-test \
  --push \
  --repository=org/artifacts
```

**Pros**:
- No code required
- Built-in snapshot parsing
- Multi-component support
- OCI push included
- CI/CD ready

**Cons**:
- Less programmatic control
- Fixed workflow
- External process

### Using the Library

```go
client, _ := coverageclient.NewClient(namespace, outputDir)
client.SetSourceDirectory(projectRoot)

// For each pod
client.CollectCoverageFromPod(ctx, podName, testName, port)
client.GenerateCoverageReport(testName)
client.FilterCoverageReport(testName)
client.GenerateHTMLReport(testName)
```

**Pros**:
- Full programmatic control
- Custom workflows
- Direct Go integration
- Test framework integration

**Cons**:
- More code to write
- Manual multi-component handling
- Need to handle errors explicitly

## Testing Strategy

### Unit Tests
- Test discovery logic with fake Kubernetes client
- Test snapshot parsing
- Test path normalization

### Integration Tests
- Test against real Kubernetes cluster (kind)
- Test with actual coverage servers
- Verify OCI push (with test registry)

### E2E Tests
- Full pipeline simulation
- Multi-component scenarios
- Error handling validation

## Performance Considerations

### Parallel Collection
Currently sequential, but could be parallelized:
```go
// Future enhancement
var wg sync.WaitGroup
for _, pod := range pods {
    wg.Add(1)
    go func(p PodInfo) {
        defer wg.Done()
        collectFromPod(ctx, config, p)
    }(pod)
}
wg.Wait()
```

### Discovery Optimization
- Cache namespace lists
- Reuse Kubernetes client
- Batch pod queries

### Memory Management
- Stream large coverage files
- Clean up temp data
- Limit concurrent operations

## Security Considerations

1. **Credentials**: Uses kubeconfig or in-cluster auth
2. **Registry**: Leverages Docker credentials
3. **Network**: Port-forward is temporary and scoped
4. **Permissions**: Requires pod read and port-forward permissions

## Future Enhancements

### Planned Features
- [ ] `coverport merge` - Merge multiple coverage reports
- [ ] `coverport pull` - Pull and extract OCI artifacts
- [ ] `coverport report` - Generate reports from existing data
- [ ] `coverport diff` - Compare coverage between runs
- [ ] Parallel collection
- [ ] Progress bars for long operations
- [ ] JSON output mode
- [ ] Watch mode for continuous collection

### Potential Integrations
- GitHub Actions
- GitLab CI
- SonarQube direct upload
- Slack notifications
- Prometheus metrics

## Contributing

See main project CONTRIBUTING.md for guidelines.

## References

- Main library: `../client/client.go`
- Example test: `../test/e2e_test.go`
- [go-coverage-http](https://github.com/psturc/go-coverage-http) - Go coverage HTTP server
- [py-coverage-http](https://github.com/psturc/py-coverage-http) - Python coverage HTTP server
- Cobra documentation: https://cobra.dev/
- Kubernetes client-go: https://github.com/kubernetes/client-go
