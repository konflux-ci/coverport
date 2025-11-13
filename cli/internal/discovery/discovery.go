package discovery

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// PodInfo contains information about a discovered pod
type PodInfo struct {
	Name          string
	Namespace     string
	ComponentName string // Derived from labels or image
	Image         string
	ContainerName string // Which container has the matching image
}

// ImageDiscovery handles pod discovery based on container images
type ImageDiscovery struct {
	clientset kubernetes.Interface
}

// NewImageDiscovery creates a new ImageDiscovery instance
func NewImageDiscovery(clientset kubernetes.Interface) *ImageDiscovery {
	return &ImageDiscovery{
		clientset: clientset,
	}
}

// DiscoverPodsByImages finds all pods running the specified container images
// It searches across all namespaces unless a specific namespace is provided
func (d *ImageDiscovery) DiscoverPodsByImages(ctx context.Context, images []string, namespace string) ([]PodInfo, error) {
	var pods []PodInfo

	// Normalize images (remove tag/digest variations for comparison)
	imageSet := make(map[string]string) // normalized -> original
	for _, img := range images {
		imageSet[normalizeImageRef(img)] = img
	}

	// List options
	listOpts := metav1.ListOptions{}

	// Get namespaces to search
	var namespaces []string
	if namespace != "" {
		namespaces = []string{namespace}
	} else {
		// Search all namespaces
		nsList, err := d.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list namespaces: %w", err)
		}
		for _, ns := range nsList.Items {
			// Skip system namespaces
			if !isSystemNamespace(ns.Name) {
				namespaces = append(namespaces, ns.Name)
			}
		}
	}

	// Search for pods in each namespace
	for _, ns := range namespaces {
		podList, err := d.clientset.CoreV1().Pods(ns).List(ctx, listOpts)
		if err != nil {
			fmt.Printf("Warning: failed to list pods in namespace %s: %v\n", ns, err)
			continue
		}

		for _, pod := range podList.Items {
			// Only consider running pods
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}

			// Check each container in the pod
			for _, container := range pod.Spec.Containers {
				normalizedContainerImage := normalizeImageRef(container.Image)

				// Check if this container matches any of our target images
				for normalizedTarget, originalTarget := range imageSet {
					if matchesImage(normalizedContainerImage, normalizedTarget) {
						// Extract component name from labels or image
						componentName := extractComponentName(&pod, originalTarget)

						pods = append(pods, PodInfo{
							Name:          pod.Name,
							Namespace:     pod.Namespace,
							ComponentName: componentName,
							Image:         originalTarget,
							ContainerName: container.Name,
						})
						break // Found match for this container
					}
				}
			}
		}
	}

	return pods, nil
}

// DiscoverPodsByLabelSelector finds pods matching a label selector
func (d *ImageDiscovery) DiscoverPodsByLabelSelector(ctx context.Context, namespace, labelSelector string) ([]PodInfo, error) {
	pods, err := d.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	var result []PodInfo
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Use first container
		var containerName, image string
		if len(pod.Spec.Containers) > 0 {
			containerName = pod.Spec.Containers[0].Name
			image = pod.Spec.Containers[0].Image
		}

		componentName := extractComponentName(&pod, image)

		result = append(result, PodInfo{
			Name:          pod.Name,
			Namespace:     pod.Namespace,
			ComponentName: componentName,
			Image:         image,
			ContainerName: containerName,
		})
	}

	return result, nil
}

// normalizeImageRef normalizes an image reference for comparison
// Example: quay.io/user/app:tag -> quay.io/user/app
// Example: quay.io/user/app@sha256:abc -> quay.io/user/app
func normalizeImageRef(image string) string {
	// Remove tag
	if idx := strings.LastIndex(image, ":"); idx != -1 {
		// Check if this is a digest (contains @sha256:)
		if !strings.Contains(image[idx:], "@") {
			image = image[:idx]
		}
	}

	// Remove digest
	if idx := strings.Index(image, "@"); idx != -1 {
		image = image[:idx]
	}

	return image
}

// matchesImage checks if two normalized image references match
func matchesImage(image1, image2 string) bool {
	return image1 == image2
}

// extractComponentName tries to extract a meaningful component name
func extractComponentName(pod *corev1.Pod, image string) string {
	// Try common labels
	if name, ok := pod.Labels["app.kubernetes.io/name"]; ok {
		return name
	}
	if name, ok := pod.Labels["app"]; ok {
		return name
	}
	if name, ok := pod.Labels["app.kubernetes.io/component"]; ok {
		return name
	}

	// Fallback: extract from image name
	parts := strings.Split(image, "/")
	if len(parts) > 0 {
		imageName := parts[len(parts)-1]
		// Remove tag/digest
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

// isSystemNamespace checks if a namespace is a system namespace
func isSystemNamespace(ns string) bool {
	systemNamespaces := []string{
		"kube-system",
		"kube-public",
		"kube-node-lease",
		"openshift",
		"openshift-.*",
		"default",
	}

	for _, sysNs := range systemNamespaces {
		if strings.HasPrefix(ns, strings.TrimSuffix(sysNs, ".*")) {
			return true
		}
	}

	return false
}

