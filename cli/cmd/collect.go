package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/konflux-ci/coverport/cli/internal/discovery"
	"github.com/konflux-ci/coverport/cli/internal/manifest"
	"github.com/konflux-ci/coverport/cli/internal/snapshot"
	coverageclient "github.com/konflux-ci/coverport/cli/pkg/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var collectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Collect coverage from Kubernetes pods or HTTP endpoints",
	Long: `Collect coverage data from instrumented Go applications running in Kubernetes or from local HTTP endpoints.

This command can discover pods in multiple ways:
  1. By direct HTTP URL (for local development or non-Kubernetes deployments)
  2. By Konflux/Tekton SNAPSHOT (recommended for CI/CD)
  3. By explicit list of container images
  4. By label selector
  5. By explicit pod names

Coverage data is organized by component and can be automatically pushed to an OCI registry.`,
	Example: `  # Collect from localhost (for local development)
  coverport collect --url http://localhost:9095 --test-name my-local-test

  # Collect using Konflux snapshot
  coverport collect --snapshot='{"components":[{"name":"app","containerImage":"quay.io/user/app@sha256:abc"}]}'

  # Collect from specific images
  coverport collect --images=quay.io/user/app1:latest,quay.io/user/app2:latest

  # Collect from pods with label selector
  coverport collect --namespace=default --label-selector=app=myapp

  # Collect and push to OCI registry
  coverport collect --snapshot="$SNAPSHOT" --push \
    --registry=quay.io --repository=user/coverage-artifacts`,
	Run: runCollect,
}

var (
	// Discovery options
	coverageURL   string
	snapshotJSON  string
	snapshotFile  string
	images        []string
	namespace     string
	labelSelector string
	podNames      []string

	// Coverage options
	coveragePort int
	outputDir    string
	testName     string
	sourceDir    string
	enableRemap  bool
	filters      []string

	// Processing options
	autoProcess  bool
	skipGenerate bool
	skipFilter   bool

	// OCI push options
	push          bool
	registry      string
	repository    string
	tag           string
	expiresAfter  string
	artifactTitle string

	// Advanced options
	timeout int
)

func init() {
	rootCmd.AddCommand(collectCmd)

	// Discovery options
	collectCmd.Flags().StringVar(&coverageURL, "url", "", "Direct HTTP URL to coverage server (e.g., http://localhost:9095)")
	collectCmd.Flags().StringVar(&snapshotJSON, "snapshot", "", "Konflux/Tekton snapshot JSON")
	collectCmd.Flags().StringVar(&snapshotFile, "snapshot-file", "", "Path to snapshot JSON file")
	collectCmd.Flags().StringSliceVar(&images, "images", nil, "Comma-separated list of container images")
	collectCmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Kubernetes namespace (empty = search all non-system namespaces)")
	collectCmd.Flags().StringVarP(&labelSelector, "label-selector", "l", "", "Label selector to find pods")
	collectCmd.Flags().StringSliceVar(&podNames, "pods", nil, "Comma-separated list of pod names (requires --namespace)")

	// Coverage options
	collectCmd.Flags().IntVar(&coveragePort, "port", 9095, "Coverage server port")
	collectCmd.Flags().StringVarP(&outputDir, "output", "o", "./coverage-output", "Output directory for coverage data")
	collectCmd.Flags().StringVar(&testName, "test-name", "", "Test name (default: auto-generated)")
	collectCmd.Flags().StringVar(&sourceDir, "source-dir", ".", "Source directory for path remapping")
	collectCmd.Flags().BoolVar(&enableRemap, "remap-paths", true, "Enable automatic path remapping")
	collectCmd.Flags().StringSliceVar(&filters, "filters", []string{"coverage_server.go"}, "File patterns to filter from coverage")

	// Processing options
	collectCmd.Flags().BoolVar(&autoProcess, "auto-process", true, "Automatically process coverage reports")
	collectCmd.Flags().BoolVar(&skipGenerate, "skip-generate", false, "Skip generating text reports")
	collectCmd.Flags().BoolVar(&skipFilter, "skip-filter", false, "Skip filtering reports")
	// Note: HTML generation moved to 'process' command as it requires source code access

	// OCI push options
	collectCmd.Flags().BoolVar(&push, "push", false, "Push coverage artifact to OCI registry")
	collectCmd.Flags().StringVar(&registry, "registry", "quay.io", "OCI registry URL")
	collectCmd.Flags().StringVar(&repository, "repository", "", "OCI repository (e.g., 'user/coverage-artifacts')")
	collectCmd.Flags().StringVar(&tag, "tag", "", "OCI artifact tag (default: auto-generated)")
	collectCmd.Flags().StringVar(&expiresAfter, "expires-after", "30d", "Artifact expiration (e.g., '30d', '1y')")
	collectCmd.Flags().StringVar(&artifactTitle, "artifact-title", "", "Artifact title")

	// Advanced options
	collectCmd.Flags().IntVar(&timeout, "timeout", 120, "Timeout in seconds for operations")
}

func runCollect(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	verbose, _ := cmd.Flags().GetBool("verbose")

	// Validate inputs
	discoveryMethods := 0
	if coverageURL != "" {
		discoveryMethods++
	}
	if snapshotJSON != "" {
		discoveryMethods++
	}
	if snapshotFile != "" {
		discoveryMethods++
	}
	if len(images) > 0 {
		discoveryMethods++
	}
	if labelSelector != "" {
		discoveryMethods++
	}
	if len(podNames) > 0 {
		discoveryMethods++
	}

	if discoveryMethods == 0 {
		exitWithError("No discovery method specified. Use --url, --snapshot, --images, --label-selector, or --pods")
	}
	if discoveryMethods > 1 {
		exitWithError("Multiple discovery methods specified. Use only one of: --url, --snapshot, --images, --label-selector, or --pods")
	}

	if push && repository == "" {
		exitWithError("--repository is required when --push is enabled")
	}

	if len(podNames) > 0 && namespace == "" {
		exitWithError("--namespace is required when using --pods")
	}

	// Generate test name if not provided
	if testName == "" {
		testName = fmt.Sprintf("coverage-%s", time.Now().Format("20060102-150405"))
	}

	// Generate tag if not provided
	if tag == "" && push {
		tag = fmt.Sprintf("%s-%s", testName, time.Now().Format("20060102-150405"))
	}

	fmt.Println("🚀 coverport - Coverage Collection Tool")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Test Name:     %s\n", testName)
	fmt.Printf("Output Dir:    %s\n", outputDir)
	if coverageURL != "" {
		fmt.Printf("Coverage URL:  %s\n", coverageURL)
	} else {
		fmt.Printf("Coverage Port: %d\n", coveragePort)
	}
	fmt.Println(strings.Repeat("=", 60))

	// Handle direct URL collection (bypass Kubernetes)
	if coverageURL != "" {
		collectFromURL(ctx, verbose)
		return
	}

	// Setup Kubernetes client
	clientset, restConfig := setupKubeClient()

	// Discover pods
	var podsToCollect []discovery.PodInfo
	var err error

	if snapshotJSON != "" || snapshotFile != "" {
		podsToCollect, err = discoverPodsFromSnapshot(ctx, clientset, verbose)
	} else if len(images) > 0 {
		podsToCollect, err = discoverPodsFromImages(ctx, clientset, images, verbose)
	} else if labelSelector != "" {
		podsToCollect, err = discoverPodsFromLabelSelector(ctx, clientset, verbose)
	} else if len(podNames) > 0 {
		podsToCollect, err = discoverPodsFromNames(ctx, clientset, verbose)
	}

	if err != nil {
		exitWithError("Pod discovery failed: %v", err)
	}

	if len(podsToCollect) == 0 {
		exitWithError("No running pods found matching the criteria")
	}

	fmt.Printf("\n📍 Discovered %d pod(s) for coverage collection:\n", len(podsToCollect))
	for i, pod := range podsToCollect {
		fmt.Printf("  %d. %s/%s (component: %s, image: %s)\n",
			i+1, pod.Namespace, pod.Name, pod.ComponentName, truncateImage(pod.Image))
	}
	fmt.Println()

	// Create collection manifest
	collectionManifest := manifest.NewCollectionManifest(testName, manifest.CollectionParameters{
		CoveragePort: coveragePort,
		Filters:      filters,
		Format:       "go", // TODO: support auto-detection
		Namespace:    namespace,
	})

	// Collect coverage from each pod
	successCount := 0
	for _, podInfo := range podsToCollect {
		componentInfo, err := collectFromPod(ctx, restConfig, podInfo, verbose)
		if err != nil {
			printWarning("Failed to collect from %s/%s: %v", podInfo.Namespace, podInfo.Name, err)
		} else {
			successCount++
			// Add successful collection to manifest
			if componentInfo != nil {
				collectionManifest.AddComponent(*componentInfo)
			}
		}
	}

	if successCount == 0 {
		exitWithError("Failed to collect coverage from any pods")
	}

	printSuccess("Collected coverage from %d/%d pod(s)", successCount, len(podsToCollect))

	// Save collection manifest
	if err := collectionManifest.Save(outputDir); err != nil {
		printWarning("Failed to save collection manifest: %v", err)
	}

	// Push to OCI registry if requested
	if push {
		if err := pushCoverageArtifact(ctx, podsToCollect); err != nil {
			printWarning("Failed to push coverage artifact: %v", err)
			fmt.Println("   (Coverage data is still available locally)")
		}
	}

	fmt.Println("\n✅ Coverage collection complete!")
	fmt.Printf("📁 Coverage data saved to: %s\n", outputDir)
}

func setupKubeClient() (kubernetes.Interface, *rest.Config) {
	// Load kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	// Build config
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		// Try in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			exitWithError("Failed to build Kubernetes config: %v", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		exitWithError("Failed to create Kubernetes client: %v", err)
	}

	return clientset, config
}

func discoverPodsFromSnapshot(ctx context.Context, clientset kubernetes.Interface, verbose bool) ([]discovery.PodInfo, error) {
	var snap *snapshot.Snapshot
	var err error

	if snapshotFile != "" {
		if verbose {
			fmt.Printf("📄 Reading snapshot from file: %s\n", snapshotFile)
		}
		snap, err = snapshot.ParseSnapshotFromFile(snapshotFile)
	} else {
		if verbose {
			fmt.Printf("📄 Parsing snapshot from JSON\n")
		}
		snap, err = snapshot.ParseSnapshot(snapshotJSON)
	}

	if err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}

	fmt.Printf("📦 Snapshot contains %d component(s):\n", len(snap.Components))
	for i, comp := range snap.Components {
		fmt.Printf("  %d. %s: %s\n", i+1, comp.Name, truncateImage(comp.ContainerImage))
	}

	images := snap.GetImages()
	return discoverPodsFromImages(ctx, clientset, images, verbose)
}

func discoverPodsFromImages(ctx context.Context, clientset kubernetes.Interface, images []string, verbose bool) ([]discovery.PodInfo, error) {
	if verbose {
		fmt.Printf("🔍 Searching for pods with images:\n")
		for _, img := range images {
			fmt.Printf("  - %s\n", img)
		}
	}

	disco := discovery.NewImageDiscovery(clientset)
	return disco.DiscoverPodsByImages(ctx, images, namespace)
}

func discoverPodsFromLabelSelector(ctx context.Context, clientset kubernetes.Interface, verbose bool) ([]discovery.PodInfo, error) {
	if namespace == "" {
		return nil, fmt.Errorf("--namespace is required when using --label-selector")
	}

	if verbose {
		fmt.Printf("🔍 Searching for pods with label selector: %s (namespace: %s)\n", labelSelector, namespace)
	}

	disco := discovery.NewImageDiscovery(clientset)
	return disco.DiscoverPodsByLabelSelector(ctx, namespace, labelSelector)
}

func discoverPodsFromNames(ctx context.Context, clientset kubernetes.Interface, verbose bool) ([]discovery.PodInfo, error) {
	if verbose {
		fmt.Printf("🔍 Using explicitly specified pods: %v\n", podNames)
	}

	var pods []discovery.PodInfo
	for _, podName := range podNames {
		// Get pod details to extract image info
		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get pod %s: %w", podName, err)
		}

		if pod.Status.Phase != corev1.PodRunning {
			printWarning("Pod %s is not running (phase: %s)", podName, pod.Status.Phase)
			continue
		}

		var containerName, image string
		if len(pod.Spec.Containers) > 0 {
			containerName = pod.Spec.Containers[0].Name
			image = pod.Spec.Containers[0].Image
		}

		componentName := extractComponentNameFromLabels(pod.Labels, image)

		pods = append(pods, discovery.PodInfo{
			Name:          podName,
			Namespace:     namespace,
			ComponentName: componentName,
			Image:         image,
			ContainerName: containerName,
		})
	}

	return pods, nil
}

func collectFromPod(ctx context.Context, restConfig *rest.Config, podInfo discovery.PodInfo, verbose bool) (*manifest.ComponentInfo, error) {
	fmt.Printf("\n📊 Collecting from: %s/%s (component: %s)\n", podInfo.Namespace, podInfo.Name, podInfo.ComponentName)

	// Create component-specific output directory
	componentDir := filepath.Join(outputDir, podInfo.ComponentName)
	if err := os.MkdirAll(componentDir, 0755); err != nil {
		return nil, fmt.Errorf("create component directory: %w", err)
	}

	// Create coverage client for this pod's namespace
	client, err := coverageclient.NewClient(podInfo.Namespace, componentDir)
	if err != nil {
		return nil, fmt.Errorf("create coverage client: %w", err)
	}

	// Configure client
	client.SetSourceDirectory(sourceDir)
	client.SetPathRemapping(enableRemap)
	if len(filters) > 0 {
		client.SetDefaultFilters(filters)
	}

	// Collect coverage
	componentTestName := fmt.Sprintf("%s-%s", testName, podInfo.ComponentName)
	if err := client.CollectCoverageFromPodWithContainer(ctx, podInfo.Name, podInfo.ContainerName, componentTestName, coveragePort); err != nil {
		return nil, fmt.Errorf("collect coverage: %w", err)
	}

	// Note: Component metadata is now stored in the top-level manifest, not as separate files

	// Process reports if enabled
	if autoProcess && !skipGenerate {
		if verbose {
			fmt.Printf("  📝 Processing coverage reports...\n")
		}

		if err := client.GenerateCoverageReport(componentTestName); err != nil {
			printWarning("Failed to generate report: %v", err)
		} else if !skipFilter {
			if err := client.FilterCoverageReport(componentTestName); err != nil {
				printWarning("Failed to filter report: %v", err)
			}
		}
		// Note: HTML generation moved to 'process' command as it requires source code access
	}

	// Return component info for manifest
	return &manifest.ComponentInfo{
		Name:          podInfo.ComponentName,
		Image:         podInfo.Image,
		CoverageDir:   filepath.Join(podInfo.ComponentName, componentTestName),
		Namespace:     podInfo.Namespace,
		PodName:       podInfo.Name,
		ContainerName: podInfo.ContainerName,
		CollectedAt:   time.Now().Format(time.RFC3339),
	}, nil
}

func pushCoverageArtifact(ctx context.Context, pods []discovery.PodInfo) error {
	fmt.Println("\n📦 Pushing coverage artifact to OCI registry...")

	// For multi-component, we push the entire output directory
	// Create a temporary coverage client just for pushing
	client, err := coverageclient.NewClient("default", outputDir)
	if err != nil {
		return fmt.Errorf("create coverage client: %w", err)
	}

	// Build artifact title
	title := artifactTitle
	if title == "" {
		components := make([]string, 0, len(pods))
		seen := make(map[string]bool)
		for _, pod := range pods {
			if !seen[pod.ComponentName] {
				components = append(components, pod.ComponentName)
				seen[pod.ComponentName] = true
			}
		}
		title = fmt.Sprintf("Coverage data for: %s", strings.Join(components, ", "))
	}

	opts := coverageclient.PushCoverageArtifactOptions{
		Registry:     registry,
		Repository:   repository,
		Tag:          tag,
		ExpiresAfter: expiresAfter,
		Title:        title,
		Annotations: map[string]string{
			"org.opencontainers.image.created":     time.Now().Format(time.RFC3339),
			"org.opencontainers.image.title":       title,
			"org.opencontainers.image.description": fmt.Sprintf("Coverage data from test: %s", testName),
		},
	}

	// Push each component's coverage
	for _, pod := range pods {
		componentTestName := fmt.Sprintf("%s-%s", testName, pod.ComponentName)
		if err := client.PushCoverageArtifact(ctx, componentTestName, opts); err != nil {
			return fmt.Errorf("push component %s: %w", pod.ComponentName, err)
		}
	}

	artifactRef := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	printSuccess("Coverage artifact pushed to: %s", artifactRef)

	// Write artifact ref to file if specified
	if artifactRefFile := os.Getenv("COVERAGE_ARTIFACT_REF_FILE"); artifactRefFile != "" {
		if err := os.WriteFile(artifactRefFile, []byte(artifactRef), 0644); err != nil {
			printWarning("Failed to write artifact ref to %s: %v", artifactRefFile, err)
		} else {
			fmt.Printf("📝 Artifact reference saved to: %s\n", artifactRefFile)
		}
	}

	return nil
}

func collectFromURL(ctx context.Context, verbose bool) {
	fmt.Printf("\n📡 Collecting coverage from URL: %s\n", coverageURL)

	// Create coverage client (without Kubernetes)
	client, err := coverageclient.NewClientForURL(outputDir)
	if err != nil {
		exitWithError("Failed to create coverage client: %v", err)
	}

	// Set filters on the client
	client.SetDefaultFilters(filters)

	// Collect coverage from URL
	fmt.Printf("  🔄 Sending coverage collection request...\n")
	if err := client.CollectCoverageFromURL(coverageURL, testName); err != nil {
		exitWithError("Failed to collect coverage from URL: %v", err)
	}

	printSuccess("Coverage collected from URL")
	fmt.Printf("  📂 Output: %s/%s\n", outputDir, testName)

	// Create a simple manifest for URL-based collection
	componentName := "direct-url"
	collectionManifest := manifest.NewCollectionManifest(testName, manifest.CollectionParameters{
		CoveragePort: 0, // Not applicable for URL collection
		Filters:      filters,
		Format:       "go",
		Namespace:    "",
	})

	// Add component info for the URL collection
	collectionManifest.AddComponent(manifest.ComponentInfo{
		Name:        componentName,
		Image:       coverageURL, // Store the URL in the image field for reference
		CoverageDir: testName,    // Store relative path, will be joined with coverageDir during processing
		Namespace:   "",
		PodName:     "",
		CollectedAt: time.Now().Format(time.RFC3339),
	})

	// Save manifest
	if err := collectionManifest.Save(outputDir); err != nil {
		printWarning("Failed to save manifest: %v", err)
	} else {
		fmt.Printf("  📋 Manifest saved: %s/metadata.json\n", outputDir)
	}

	fmt.Println("\n✅ Coverage collection complete!")
	fmt.Printf("📊 Coverage output: %s\n", outputDir)
	fmt.Printf("🔍 To process and upload coverage, run:\n")
	fmt.Printf("   coverport process --coverage-dir=%s\n", outputDir)
}

func truncateImage(image string) string {
	if len(image) <= 60 {
		return image
	}
	return image[:57] + "..."
}

func extractComponentNameFromLabels(labels map[string]string, image string) string {
	if name, ok := labels["app.kubernetes.io/name"]; ok {
		return name
	}
	if name, ok := labels["app"]; ok {
		return name
	}
	if name, ok := labels["app.kubernetes.io/component"]; ok {
		return name
	}

	// Fallback to image name
	parts := strings.Split(image, "/")
	if len(parts) > 0 {
		imageName := parts[len(parts)-1]
		if idx := strings.Index(imageName, ":"); idx != -1 {
			imageName = imageName[:idx]
		}
		if idx := strings.Index(imageName, "@"); idx != -1 {
			imageName = imageName[:idx]
		}
		return imageName
	}

	return "unknown"
}
