package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/konflux-ci/coverport/cli/internal/discovery"
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover pods for coverage collection",
	Long: `Discover pods running instrumented applications without collecting coverage.

This is useful for:
  • Debugging pod discovery logic
  • Verifying which pods will be targeted
  • Testing label selectors and image matching`,
	Example: `  # Discover pods from snapshot
  coverport discover --snapshot='{"components":[...]}'

  # Discover pods by images
  coverport discover --images=quay.io/user/app:latest

  # Discover pods by label selector
  coverport discover --namespace=default --label-selector=app=myapp`,
	Run: runDiscover,
}

var (
	discoverVerbose bool
)

func init() {
	rootCmd.AddCommand(discoverCmd)

	// Reuse the same flags as collect command for discovery
	discoverCmd.Flags().StringVar(&snapshotJSON, "snapshot", "", "Konflux/Tekton snapshot JSON")
	discoverCmd.Flags().StringVar(&snapshotFile, "snapshot-file", "", "Path to snapshot JSON file")
	discoverCmd.Flags().StringSliceVar(&images, "images", nil, "Comma-separated list of container images")
	discoverCmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Kubernetes namespace (empty = search all non-system namespaces)")
	discoverCmd.Flags().StringVarP(&labelSelector, "label-selector", "l", "", "Label selector to find pods")
	discoverCmd.Flags().StringSliceVar(&podNames, "pods", nil, "Comma-separated list of pod names (requires --namespace)")
	discoverCmd.Flags().BoolVar(&discoverVerbose, "verbose", false, "Enable verbose output")
}

func runDiscover(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Validate inputs
	discoveryMethods := 0
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
		exitWithError("No discovery method specified. Use --snapshot, --images, --label-selector, or --pods")
	}
	if discoveryMethods > 1 {
		exitWithError("Multiple discovery methods specified. Use only one of: --snapshot, --images, --label-selector, or --pods")
	}

	fmt.Println("🔍 coverport - Pod Discovery")
	fmt.Println("─────────────────────────────")

	// Setup Kubernetes client
	clientset, _ := setupKubeClient()

	// Discover pods
	var podsToCollect []discovery.PodInfo
	var err error

	if snapshotJSON != "" || snapshotFile != "" {
		podsToCollect, err = discoverPodsFromSnapshot(ctx, clientset, discoverVerbose)
	} else if len(images) > 0 {
		podsToCollect, err = discoverPodsFromImages(ctx, clientset, images, discoverVerbose)
	} else if labelSelector != "" {
		podsToCollect, err = discoverPodsFromLabelSelector(ctx, clientset, discoverVerbose)
	} else if len(podNames) > 0 {
		podsToCollect, err = discoverPodsFromNames(ctx, clientset, discoverVerbose)
	}

	if err != nil {
		exitWithError("Pod discovery failed: %v", err)
	}

	if len(podsToCollect) == 0 {
		fmt.Println("\n⚠️  No running pods found matching the criteria")
		return
	}

	fmt.Printf("\n✅ Discovered %d pod(s):\n\n", len(podsToCollect))

	// Group by component
	componentPods := make(map[string][]discovery.PodInfo)
	for _, pod := range podsToCollect {
		componentPods[pod.ComponentName] = append(componentPods[pod.ComponentName], pod)
	}

	for component, pods := range componentPods {
		fmt.Printf("📦 Component: %s\n", component)
		for _, pod := range pods {
			fmt.Printf("   • Pod: %s/%s\n", pod.Namespace, pod.Name)
			fmt.Printf("     Container: %s\n", pod.ContainerName)
			if discoverVerbose {
				fmt.Printf("     Image: %s\n", pod.Image)
			} else {
				fmt.Printf("     Image: %s\n", truncateImage(pod.Image))
			}
		}
		fmt.Println()
	}

	fmt.Printf("💡 To collect coverage from these pods, run:\n")
	if snapshotJSON != "" {
		fmt.Printf("   coverport collect --snapshot='%s'\n", truncateForDisplay(snapshotJSON, 50))
	} else if snapshotFile != "" {
		fmt.Printf("   coverport collect --snapshot-file=%s\n", snapshotFile)
	} else if len(images) > 0 {
		fmt.Printf("   coverport collect --images=%s\n", images[0])
	} else if labelSelector != "" {
		fmt.Printf("   coverport collect --namespace=%s --label-selector=%s\n", namespace, labelSelector)
	}
}

func truncateForDisplay(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
