package coverageclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
)

// CoverageClient handles coverage collection from Kubernetes pods
type CoverageClient struct {
	clientset       kubernetes.Interface
	restConfig      *rest.Config
	namespace       string
	outputDir       string
	httpClient      *http.Client
	defaultFilters  []string // Default file patterns to filter out from coverage
	sourceDir       string   // Local source directory for path remapping
	enablePathRemap bool     // Whether to automatically remap container paths
}

// CoverageResponse matches the server's response format
type CoverageResponse struct {
	MetaFilename     string `json:"meta_filename"`
	MetaData         string `json:"meta_data"`
	CountersFilename string `json:"counters_filename"`
	CountersData     string `json:"counters_data"`
	TestName         string `json:"test_name"`
	Timestamp        int64  `json:"timestamp"`
}

// PodMetadata contains information about the pod from which coverage was collected
type PodMetadata struct {
	PodName      string            `json:"pod_name"`
	Namespace    string            `json:"namespace"`
	Container    ContainerMetadata `json:"container"`
	CollectedAt  string            `json:"collected_at"`
	TestName     string            `json:"test_name"`
	CoveragePort int               `json:"coverage_port"`
}

// ContainerMetadata contains information about a container in the pod
type ContainerMetadata struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// NewClient creates a new coverage client for the given namespace
func NewClient(namespace, outputDir string) (*CoverageClient, error) {
	// Load kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	// Build config from kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		// Try in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("build kubernetes config: %w", err)
		}
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	// Get current working directory as default source directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	return &CoverageClient{
		clientset:       clientset,
		restConfig:      config,
		namespace:       namespace,
		outputDir:       outputDir,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		defaultFilters:  []string{"coverage_server.go"}, // Default: filter out the coverage server itself
		sourceDir:       cwd,
		enablePathRemap: true, // Default: enable automatic path remapping
	}, nil
}

// SetDefaultFilters configures which files to automatically filter from coverage reports
func (c *CoverageClient) SetDefaultFilters(patterns []string) {
	c.defaultFilters = patterns
}

// AddDefaultFilter adds a file pattern to the default filter list
func (c *CoverageClient) AddDefaultFilter(pattern string) {
	c.defaultFilters = append(c.defaultFilters, pattern)
}

// SetSourceDirectory sets the local source directory for path remapping
func (c *CoverageClient) SetSourceDirectory(dir string) {
	c.sourceDir = dir
}

// SetPathRemapping enables or disables automatic path remapping
func (c *CoverageClient) SetPathRemapping(enabled bool) {
	c.enablePathRemap = enabled
}

// GetPodName discovers a pod name dynamically based on label selector
// Example: client.GetPodName("app=coverage-demo")
func (c *CoverageClient) GetPodName(labelSelector string) (string, error) {
	return c.GetPodNameWithContext(context.Background(), labelSelector)
}

// GetPodNameWithContext discovers a pod name with context support
func (c *CoverageClient) GetPodNameWithContext(ctx context.Context, labelSelector string) (string, error) {
	fmt.Printf("🔍 Discovering pod with label selector: %s\n", labelSelector)

	// List pods with the label selector
	pods, err := c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return "", fmt.Errorf("list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods found with label selector '%s' in namespace '%s'", labelSelector, c.namespace)
	}

	// Find the first running pod
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			fmt.Printf("✅ Found running pod: %s\n", pod.Name)
			return pod.Name, nil
		}
	}

	// If no running pod found, return first pod with its status
	firstPod := pods.Items[0]
	return "", fmt.Errorf("no running pod found (first pod '%s' is in phase '%s')", firstPod.Name, firstPod.Status.Phase)
}

// CollectCoverageFromPod collects coverage data from a pod via port-forwarding
func (c *CoverageClient) CollectCoverageFromPod(ctx context.Context, podName, testName string, targetPort int) error {
	return c.CollectCoverageFromPodWithContainer(ctx, podName, "", testName, targetPort)
}

// CollectCoverageFromPodWithContainer collects coverage data from a specific container in a pod via port-forwarding
// If containerName is empty, it will try to detect the correct container automatically
func (c *CoverageClient) CollectCoverageFromPodWithContainer(ctx context.Context, podName, containerName, testName string, targetPort int) error {
	fmt.Printf("📊 Collecting coverage from pod %s for test: %s\n", podName, testName)

	// Setup port forwarding
	localPort, stopChan, err := c.setupPortForward(podName, targetPort)
	if err != nil {
		return fmt.Errorf("setup port forward: %w", err)
	}
	defer close(stopChan)

	// Wait a bit for port forward to be ready
	time.Sleep(2 * time.Second)

	// Collect coverage via HTTP
	coverageURL := fmt.Sprintf("http://localhost:%d/coverage", localPort)
	if err := c.collectCoverageFromURL(coverageURL, testName); err != nil {
		return fmt.Errorf("collect coverage: %w", err)
	}

	// Get pod metadata and save it
	if err := c.savePodMetadata(ctx, podName, containerName, testName, targetPort); err != nil {
		// Log warning but don't fail the coverage collection
		fmt.Printf("⚠️  Failed to save pod metadata: %v\n", err)
	}

	fmt.Printf("✅ Coverage collected successfully for test: %s\n", testName)
	return nil
}

// CollectCoverageFromURL collects coverage data from a direct URL (no port-forwarding)
func (c *CoverageClient) CollectCoverageFromURL(coverageURL, testName string) error {
	return c.collectCoverageFromURL(coverageURL, testName)
}

// savePodMetadata retrieves pod information and saves it to metadata.json
func (c *CoverageClient) savePodMetadata(ctx context.Context, podName, containerName, testName string, targetPort int) error {
	// Get pod details
	pod, err := c.clientset.CoreV1().Pods(c.namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get pod details: %w", err)
	}

	var coverageContainer *ContainerMetadata

	// If container name is explicitly provided, use it
	if containerName != "" {
		for _, container := range pod.Spec.Containers {
			if container.Name == containerName {
				coverageContainer = &ContainerMetadata{
					Name:  container.Name,
					Image: container.Image,
				}
				fmt.Printf("  🔍 Using specified container: %s (image: %s)\n", container.Name, container.Image)
				break
			}
		}
		if coverageContainer == nil {
			return fmt.Errorf("specified container '%s' not found in pod", containerName)
		}
	} else {
		// Try to detect the container that exposes the target port
		for _, container := range pod.Spec.Containers {
			for _, port := range container.Ports {
				if int(port.ContainerPort) == targetPort {
					coverageContainer = &ContainerMetadata{
						Name:  container.Name,
						Image: container.Image,
					}
					fmt.Printf("  🔍 Detected coverage container: %s (image: %s)\n", container.Name, container.Image)
					break
				}
			}
			if coverageContainer != nil {
				break
			}
		}

		// If no container explicitly exposes the port, try to detect by checking which one is listening
		if coverageContainer == nil {
			fmt.Printf("  🔍 Port %d not in container specs, detecting by checking listeners...\n", targetPort)
			detectedContainer := c.detectContainerByPort(ctx, podName, pod.Spec.Containers, targetPort)
			if detectedContainer != "" {
				for _, container := range pod.Spec.Containers {
					if container.Name == detectedContainer {
						coverageContainer = &ContainerMetadata{
							Name:  container.Name,
							Image: container.Image,
						}
						fmt.Printf("  🔍 Detected container listening on port %d: %s (image: %s)\n", targetPort, container.Name, container.Image)
						break
					}
				}
			}
		}

		// Final fallback: use first container
		if coverageContainer == nil {
			if len(pod.Spec.Containers) > 0 {
				fmt.Printf("  ⚠️  Could not detect coverage container, using first container\n")
				coverageContainer = &ContainerMetadata{
					Name:  pod.Spec.Containers[0].Name,
					Image: pod.Spec.Containers[0].Image,
				}
			} else {
				return fmt.Errorf("no containers found in pod")
			}
		}
	}

	// Create metadata structure
	metadata := PodMetadata{
		PodName:      podName,
		Namespace:    c.namespace,
		Container:    *coverageContainer,
		CollectedAt:  time.Now().Format(time.RFC3339),
		TestName:     testName,
		CoveragePort: targetPort,
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata to JSON: %w", err)
	}

	// Save to file in the test directory
	testDir := filepath.Join(c.outputDir, testName)
	metadataPath := filepath.Join(testDir, "metadata.json")

	if err := os.WriteFile(metadataPath, jsonData, 0644); err != nil {
		return fmt.Errorf("write metadata file: %w", err)
	}

	fmt.Printf("  📁 Saved: %s\n", metadataPath)
	return nil
}

// detectContainerByPort tries to detect which container is listening on the specified port
func (c *CoverageClient) detectContainerByPort(ctx context.Context, podName string, containers []corev1.Container, targetPort int) string {
	for _, container := range containers {
		// Try to check if the port is listening in this container
		// We'll use netstat or ss to check for listening ports
		cmd := []string{"sh", "-c", fmt.Sprintf("netstat -tln 2>/dev/null | grep ':%d ' || ss -tln 2>/dev/null | grep ':%d '", targetPort, targetPort)}

		req := c.clientset.CoreV1().RESTClient().
			Post().
			Resource("pods").
			Name(podName).
			Namespace(c.namespace).
			SubResource("exec").
			Param("container", container.Name).
			Param("command", cmd[0]).
			Param("command", cmd[1]).
			Param("command", cmd[2]).
			Param("stdout", "true").
			Param("stderr", "true")

		exec, err := c.createExecutor(req)
		if err != nil {
			continue
		}

		var stdout, stderr bytes.Buffer
		err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdout: &stdout,
			Stderr: &stderr,
		})

		// If command succeeded and found the port, this is our container
		if err == nil && stdout.Len() > 0 {
			return container.Name
		}
	}

	return ""
}

// createExecutor creates a remote command executor
func (c *CoverageClient) createExecutor(req *rest.Request) (remotecommand.Executor, error) {
	exec, err := remotecommand.NewSPDYExecutor(c.restConfig, "POST", req.URL())
	if err != nil {
		return nil, err
	}
	return exec, nil
}

// setupPortForward sets up port forwarding to the pod
func (c *CoverageClient) setupPortForward(podName string, targetPort int) (int, chan struct{}, error) {
	// Use a local port (let the system choose)
	localPort := 0 // 0 means let the system choose

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", c.namespace, podName)
	hostIP := strings.TrimPrefix(c.restConfig.Host, "https://")
	serverURL, err := url.Parse(fmt.Sprintf("https://%s%s", hostIP, path))
	if err != nil {
		return 0, nil, fmt.Errorf("parse server URL: %w", err)
	}

	transport, upgrader, err := spdy.RoundTripperFor(c.restConfig)
	if err != nil {
		return 0, nil, fmt.Errorf("create round tripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", serverURL)

	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{})

	// Create port forward
	ports := []string{fmt.Sprintf("%d:%d", localPort, targetPort)}

	out := io.Discard
	errOut := io.Discard

	forwarder, err := portforward.New(dialer, ports, stopChan, readyChan, out, errOut)
	if err != nil {
		return 0, nil, fmt.Errorf("create port forwarder: %w", err)
	}

	// Start port forwarding in background
	go func() {
		if err := forwarder.ForwardPorts(); err != nil {
			fmt.Printf("⚠️  Port forward error: %v\n", err)
		}
	}()

	// Wait for ready signal
	select {
	case <-readyChan:
		// Get the actual local port that was assigned
		forwardedPorts, err := forwarder.GetPorts()
		if err != nil || len(forwardedPorts) == 0 {
			close(stopChan)
			return 0, nil, fmt.Errorf("get forwarded ports: %w", err)
		}
		actualLocalPort := int(forwardedPorts[0].Local)
		fmt.Printf("✅ Port forward ready: localhost:%d -> pod:%d\n", actualLocalPort, targetPort)
		return actualLocalPort, stopChan, nil
	case <-time.After(30 * time.Second):
		close(stopChan)
		return 0, nil, fmt.Errorf("timeout waiting for port forward")
	}
}

// collectCoverageFromURL collects coverage from the given URL
func (c *CoverageClient) collectCoverageFromURL(coverageURL, testName string) error {
	// Prepare request body
	reqBody, err := json.Marshal(map[string]string{
		"test_name": testName,
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Send POST request to coverage endpoint
	resp, err := c.httpClient.Post(coverageURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("send coverage request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coverage endpoint returned %d: %s", resp.StatusCode, body)
	}

	// Parse response
	var covResp CoverageResponse
	if err := json.NewDecoder(resp.Body).Decode(&covResp); err != nil {
		return fmt.Errorf("decode coverage response: %w", err)
	}

	// Decode and save metadata
	metaData, err := base64.StdEncoding.DecodeString(covResp.MetaData)
	if err != nil {
		return fmt.Errorf("decode metadata: %w", err)
	}

	// Decode and save counters
	counterData, err := base64.StdEncoding.DecodeString(covResp.CountersData)
	if err != nil {
		return fmt.Errorf("decode counters: %w", err)
	}

	// Create test-specific subdirectory
	testDir := filepath.Join(c.outputDir, testName)
	if err := os.MkdirAll(testDir, 0755); err != nil {
		return fmt.Errorf("create test directory: %w", err)
	}

	// Save files with proper names
	metaPath := filepath.Join(testDir, covResp.MetaFilename)
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return fmt.Errorf("write metadata file: %w", err)
	}

	counterPath := filepath.Join(testDir, covResp.CountersFilename)
	if err := os.WriteFile(counterPath, counterData, 0644); err != nil {
		return fmt.Errorf("write counters file: %w", err)
	}

	fmt.Printf("  📁 Saved: %s\n", metaPath)
	fmt.Printf("  📁 Saved: %s\n", counterPath)

	return nil
}

// GenerateCoverageReport generates a text coverage report from collected data
func (c *CoverageClient) GenerateCoverageReport(testName string) error {
	testDir := filepath.Join(c.outputDir, testName)
	reportPath := filepath.Join(testDir, "coverage.out")

	fmt.Printf("📊 Generating coverage report for test: %s\n", testName)

	// Run go tool covdata to convert binary format to text
	cmd := exec.Command("go", "tool", "covdata", "textfmt",
		"-i="+testDir,
		"-o="+reportPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("generate coverage report: %w\nOutput: %s", err, output)
	}

	fmt.Printf("✅ Coverage report generated: %s\n", reportPath)

	// Apply path remapping if enabled
	if c.enablePathRemap {
		if err := c.remapCoveragePaths(reportPath); err != nil {
			fmt.Printf("⚠️  Path remapping failed: %v (continuing with original paths)\n", err)
		}
	}

	return nil
}

// FilterCoverageReport filters out specified files from the coverage report.
// If no patterns are provided, uses the client's default filters.
// Pass an empty slice []string{} to disable all filtering.
func (c *CoverageClient) FilterCoverageReport(testName string, patterns ...string) error {
	testDir := filepath.Join(c.outputDir, testName)
	reportPath := filepath.Join(testDir, "coverage.out")
	filteredPath := filepath.Join(testDir, "coverage_filtered.out")

	data, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("read coverage report: %w", err)
	}

	// Use default filters if no patterns provided
	filterPatterns := patterns
	if len(patterns) == 0 {
		filterPatterns = c.defaultFilters
	}

	// If no filters at all, just copy the file
	if len(filterPatterns) == 0 {
		if err := os.WriteFile(filteredPath, data, 0644); err != nil {
			return fmt.Errorf("write filtered report: %w", err)
		}
		fmt.Printf("✅ Coverage report (no filters applied): %s\n", filteredPath)
		return nil
	}

	lines := strings.Split(string(data), "\n")
	var filtered []string
	filteredCount := 0

	for _, line := range lines {
		shouldFilter := false
		for _, pattern := range filterPatterns {
			if pattern != "" && strings.Contains(line, pattern) {
				shouldFilter = true
				filteredCount++
				break
			}
		}
		if !shouldFilter {
			filtered = append(filtered, line)
		}
	}

	filteredData := strings.Join(filtered, "\n")
	if err := os.WriteFile(filteredPath, []byte(filteredData), 0644); err != nil {
		return fmt.Errorf("write filtered report: %w", err)
	}

	fmt.Printf("✅ Filtered coverage report: %s (removed %d lines matching: %v)\n",
		filteredPath, filteredCount, filterPatterns)
	return nil
}

// GenerateHTMLReport generates an HTML coverage report
func (c *CoverageClient) GenerateHTMLReport(testName string) error {
	testDir := filepath.Join(c.outputDir, testName)
	reportPath := filepath.Join(testDir, "coverage_filtered.out")
	htmlPath := filepath.Join(testDir, "coverage.html")

	// Check if filtered report exists, fallback to regular report
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		reportPath = filepath.Join(testDir, "coverage.out")
	}

	fmt.Printf("📊 Generating HTML coverage report for test: %s\n", testName)

	cmd := exec.Command("go", "tool", "cover",
		"-html="+reportPath,
		"-o="+htmlPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("generate HTML report: %w\nOutput: %s", err, output)
	}

	fmt.Printf("✅ HTML report generated: %s\n", htmlPath)
	return nil
}

// PrintCoverageSummary prints a summary of the coverage data
func (c *CoverageClient) PrintCoverageSummary(testName string) error {
	testDir := filepath.Join(c.outputDir, testName)
	reportPath := filepath.Join(testDir, "coverage_filtered.out")

	// Check if filtered report exists, fallback to regular report
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		reportPath = filepath.Join(testDir, "coverage.out")
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("read coverage report: %w", err)
	}

	fmt.Printf("\n📊 Coverage Summary for test: %s\n", testName)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println(string(data))
	fmt.Println(strings.Repeat("=", 60))

	return nil
}

// ProcessCoverageReports is a convenience method that generates, filters, and creates HTML reports
// all in one call. It automatically uses the client's default filters.
func (c *CoverageClient) ProcessCoverageReports(testName string) error {
	// Generate text report from binary coverage data
	if err := c.GenerateCoverageReport(testName); err != nil {
		return fmt.Errorf("generate report: %w", err)
	}

	// Filter the report (uses default filters)
	if err := c.FilterCoverageReport(testName); err != nil {
		return fmt.Errorf("filter report: %w", err)
	}

	// Generate HTML report
	if err := c.GenerateHTMLReport(testName); err != nil {
		// HTML generation might fail if source files aren't available, log but don't fail
		fmt.Printf("⚠️  HTML report generation failed (source files may not be available): %v\n", err)
	}

	return nil
}

// PushCoverageArtifactOptions contains options for pushing coverage artifacts to OCI registry
type PushCoverageArtifactOptions struct {
	Registry     string            // Registry URL (e.g., "quay.io")
	Repository   string            // Repository name (e.g., "psturc/oci-artifacts")
	Tag          string            // Tag for the artifact (e.g., "test-coverage-v1")
	ExpiresAfter string            // Expiration time (e.g., "1y", "30d")
	Title        string            // Artifact title
	Annotations  map[string]string // Additional annotations
}

// PushCoverageArtifact pushes the coverage output directory as an OCI artifact to a registry
func (c *CoverageClient) PushCoverageArtifact(ctx context.Context, testName string, opts PushCoverageArtifactOptions) error {
	testDir := filepath.Join(c.outputDir, testName)

	fmt.Printf("📦 Pushing coverage artifact for test: %s\n", testName)
	fmt.Printf("   Registry: %s/%s:%s\n", opts.Registry, opts.Repository, opts.Tag)
	fmt.Printf("   Source directory: %s\n", testDir)

	// Verify directory exists and has files
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		return fmt.Errorf("test directory does not exist: %s", testDir)
	}

	// Create a file store for the test directory
	fmt.Printf("   Creating file store...\n")
	fs, err := file.New(testDir)
	if err != nil {
		return fmt.Errorf("create file store: %w", err)
	}
	defer fs.Close()
	fmt.Printf("   ✓ File store created\n")

	// Add all files from the test directory
	mediaType := "application/vnd.acme.rocket.docs.layer.v1+tar"
	fileDescriptors := []ocispec.Descriptor{}

	files, err := os.ReadDir(testDir)
	if err != nil {
		return fmt.Errorf("read test directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(testDir, file.Name())
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		// Add file to the store (file store is based at testDir, so we only need the filename)
		desc, err := fs.Add(ctx, file.Name(), mediaType, file.Name())
		if err != nil {
			return fmt.Errorf("add file %s to store: %w", file.Name(), err)
		}
		fileDescriptors = append(fileDescriptors, desc)
		fmt.Printf("   📄 Added: %s (%d bytes)\n", file.Name(), fileInfo.Size())
	}

	// Pack the files and tag the packed manifest
	fmt.Printf("   Packing manifest with %d files...\n", len(fileDescriptors))
	artifactType := "application/vnd.acme.rocket.config"

	// Initialize annotations if not already set
	if opts.Annotations == nil {
		opts.Annotations = make(map[string]string)
	}

	if opts.ExpiresAfter != "" {
		opts.Annotations["quay.expires-after"] = opts.ExpiresAfter
	}
	if opts.Title != "" {
		opts.Annotations[ocispec.AnnotationTitle] = opts.Title
	}

	packOpts := oras.PackManifestOptions{
		Layers:              fileDescriptors,
		ManifestAnnotations: opts.Annotations,
	}

	manifestDesc, err := oras.PackManifest(ctx, fs, oras.PackManifestVersion1_1_RC4, artifactType, packOpts)
	if err != nil {
		return fmt.Errorf("pack manifest: %w", err)
	}
	fmt.Printf("   ✓ Manifest packed\n")

	if err = fs.Tag(ctx, manifestDesc, opts.Tag); err != nil {
		return fmt.Errorf("tag manifest: %w", err)
	}
	fmt.Printf("   ✓ Manifest tagged: %s\n", opts.Tag)

	// Setup remote repository
	fmt.Printf("   Connecting to registry %s/%s...\n", opts.Registry, opts.Repository)
	repo, err := remote.NewRepository(fmt.Sprintf("%s/%s", opts.Registry, opts.Repository))
	if err != nil {
		return fmt.Errorf("create remote repository: %w", err)
	}

	// Setup authentication using Docker credentials
	fmt.Printf("   Setting up authentication...\n")
	storeOpts := credentials.StoreOptions{}
	credStore, err := credentials.NewStoreFromDocker(storeOpts)
	if err != nil {
		return fmt.Errorf("create credential store: %w", err)
	}

	repo.Client = &auth.Client{
		Client:     http.DefaultClient,
		Cache:      auth.NewCache(),
		Credential: credentials.Credential(credStore),
	}
	fmt.Printf("   ✓ Authentication configured\n")

	// Copy from file store to remote repository
	fmt.Printf("   Pushing to registry...\n")
	_, err = oras.Copy(ctx, fs, opts.Tag, repo, opts.Tag, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("push artifact: %w", err)
	}

	fmt.Printf("✅ Coverage artifact pushed successfully\n")
	fmt.Printf("   Location: %s/%s:%s\n", opts.Registry, opts.Repository, opts.Tag)

	return nil
}

// remapCoveragePaths remaps container paths in the coverage report to local paths
func (c *CoverageClient) remapCoveragePaths(reportPath string) error {
	// Read the coverage report
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("read coverage report: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	// Detect container path mappings
	pathMappings := c.detectContainerPaths(lines)

	if len(pathMappings) == 0 {
		fmt.Println("📍 No container paths detected, using paths as-is")
		return nil
	}

	fmt.Printf("📍 Auto-detected path mappings:\n")
	for containerPath, localPath := range pathMappings {
		fmt.Printf("  [PATH] %s -> %s\n", containerPath, localPath)
	}

	// Remap paths in the coverage data
	var remappedLines []string
	remappedCount := 0

	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "mode:") {
			remappedLines = append(remappedLines, line)
			continue
		}

		// Coverage line format: path/to/file.go:line.col,line.col num count
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			remappedLines = append(remappedLines, line)
			continue
		}

		filePath := parts[0]
		rest := parts[1]

		// Try to remap the path
		newPath := filePath
		for containerPrefix, localPrefix := range pathMappings {
			if strings.HasPrefix(filePath, containerPrefix) {
				newPath = strings.Replace(filePath, containerPrefix, localPrefix, 1)
				remappedCount++
				break
			}
		}

		remappedLines = append(remappedLines, newPath+":"+rest)
	}

	// Write the remapped coverage report back
	remappedData := strings.Join(remappedLines, "\n")
	if err := os.WriteFile(reportPath, []byte(remappedData), 0644); err != nil {
		return fmt.Errorf("write remapped report: %w", err)
	}

	fmt.Printf("✅ Path remapping complete (%d lines remapped)\n", remappedCount)
	return nil
}

// detectContainerPaths analyzes coverage report lines to detect container path mappings
func (c *CoverageClient) detectContainerPaths(lines []string) map[string]string {
	// Collect all file paths from the coverage report
	var coverageFiles []string
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}

		// Coverage line format: path/to/file.go:line.col,line.col num count
		parts := strings.SplitN(line, ":", 2)
		if len(parts) >= 1 {
			filePath := parts[0]
			// Only add unique paths
			if len(coverageFiles) == 0 || coverageFiles[len(coverageFiles)-1] != filePath {
				coverageFiles = append(coverageFiles, filePath)
			}
		}
	}

	// Find files that don't exist locally (container paths)
	var containerFiles []string
	for _, filePath := range coverageFiles {
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			containerFiles = append(containerFiles, filePath)
		}
	}

	if len(containerFiles) == 0 {
		// No container paths detected
		return nil
	}

	fmt.Printf("[REMAP] Detected %d container paths to remap\n", len(containerFiles))

	// Get absolute path for source directory
	absSourceDir, err := filepath.Abs(c.sourceDir)
	if err != nil {
		fmt.Printf("[REMAP] Warning: Could not get absolute path for %s: %v\n", c.sourceDir, err)
		absSourceDir = c.sourceDir
	}

	fmt.Printf("[REMAP] Searching for source files in: %s\n", absSourceDir)

	// Build a map of local Go files by their relative path structure
	localFilesByRelPath := make(map[string]string) // key: relative path parts joined, value: full path

	err = filepath.Walk(absSourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			// Skip common directories that won't have source code
			baseName := filepath.Base(path)
			if baseName == ".git" || baseName == "vendor" || baseName == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			// Store the full path indexed by filename and path structure
			relPath, _ := filepath.Rel(absSourceDir, path)
			localFilesByRelPath[relPath] = path
		}
		return nil
	})

	if err != nil {
		fmt.Printf("[REMAP] Warning: Error walking source directory: %v\n", err)
		return nil
	}

	fmt.Printf("[REMAP] Found %d Go source files\n", len(localFilesByRelPath))

	// Try to match container files to local files
	type match struct {
		containerFile string
		localFile     string
		matchScore    int
	}

	var matches []match

	for _, containerFile := range containerFiles {
		containerPath := filepath.Clean(containerFile)
		containerParts := strings.Split(containerPath, string(filepath.Separator))
		fileName := filepath.Base(containerPath)

		// Find best matching local file
		bestMatch := ""
		bestScore := 0

		for relPath, localPath := range localFilesByRelPath {
			localParts := strings.Split(relPath, string(filepath.Separator))

			// Files must have same name
			if filepath.Base(localPath) != fileName {
				continue
			}

			// Count matching suffix parts (from filename backwards)
			matchScore := 0
			maxLen := len(containerParts)
			if len(localParts) < maxLen {
				maxLen = len(localParts)
			}

			for i := 1; i <= maxLen; i++ {
				cIdx := len(containerParts) - i
				lIdx := len(localParts) - i
				if containerParts[cIdx] == localParts[lIdx] {
					matchScore = i
				} else {
					break
				}
			}

			// Prefer longer matches (more specific paths)
			if matchScore > bestScore {
				bestScore = matchScore
				bestMatch = localPath
			}
		}

		if bestMatch != "" && bestScore > 0 {
			matches = append(matches, match{
				containerFile: containerFile,
				localFile:     bestMatch,
				matchScore:    bestScore,
			})
			fmt.Printf("[REMAP] Match: %s -> %s (score: %d)\n", containerFile, bestMatch, bestScore)
		}
	}

	if len(matches) == 0 {
		fmt.Printf("[REMAP] No matching files found between container and local paths\n")
		return nil
	}

	fmt.Printf("[REMAP] Found %d matches between container and local files\n", len(matches))

	// Determine the most common container root prefix
	containerRootCounts := make(map[string]int)

	for _, m := range matches {
		containerParts := strings.Split(filepath.Clean(m.containerFile), string(filepath.Separator))
		// Extract container root (everything except the matched suffix)
		rootPartsCount := len(containerParts) - m.matchScore
		fmt.Printf("[REMAP] Container: %s, parts: %v, score: %d, rootPartsCount: %d\n",
			m.containerFile, containerParts, m.matchScore, rootPartsCount)
		if rootPartsCount > 0 {
			rootParts := containerParts[:rootPartsCount]
			containerRoot := string(filepath.Separator) + filepath.Join(rootParts...)
			if !strings.HasSuffix(containerRoot, string(filepath.Separator)) {
				containerRoot += string(filepath.Separator)
			}
			fmt.Printf("[REMAP] Container root candidate: %s\n", containerRoot)
			containerRootCounts[containerRoot]++
		}
	}

	// Find the most common container root
	var bestContainerRoot string
	maxCount := 0
	for root, count := range containerRootCounts {
		if count > maxCount {
			maxCount = count
			bestContainerRoot = root
		}
	}

	if bestContainerRoot == "" {
		fmt.Printf("[REMAP] Could not determine container root\n")
		return nil
	}

	fmt.Printf("[REMAP] Detected container root: %s\n", bestContainerRoot)

	// Calculate the local root from all matches - find the common ancestor
	// This ensures we get the project root, not a subdirectory
	var localRootCandidates []string
	for _, m := range matches {
		if strings.HasPrefix(m.containerFile, bestContainerRoot) {
			// Get the local root by removing the matching suffix from local path
			localPath := filepath.Clean(m.localFile)
			localParts := strings.Split(localPath, string(filepath.Separator))
			rootPartsCount := len(localParts) - m.matchScore

			if rootPartsCount > 0 {
				rootParts := localParts[:rootPartsCount]
				candidateRoot := string(filepath.Separator) + filepath.Join(rootParts...)
				if !strings.HasSuffix(candidateRoot, string(filepath.Separator)) {
					candidateRoot += string(filepath.Separator)
				}
				localRootCandidates = append(localRootCandidates, candidateRoot)
				fmt.Printf("[REMAP] Root candidate from %s: %s\n", filepath.Base(m.localFile), candidateRoot)
			}
		}
	}

	// Find the shortest (most common) root - the one closest to the actual source root
	var localRoot string
	if len(localRootCandidates) > 0 {
		localRoot = localRootCandidates[0]
		for _, candidate := range localRootCandidates {
			// Shorter path means closer to the root
			if len(candidate) < len(localRoot) {
				localRoot = candidate
			}
		}
	}

	if localRoot == "" {
		fmt.Printf("[REMAP] Could not determine local root\n")
		return nil
	}

	fmt.Printf("[REMAP] Detected local root: %s\n", localRoot)

	// Return the path mapping
	return map[string]string{
		bestContainerRoot: localRoot,
	}
}
