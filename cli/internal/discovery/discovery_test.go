package discovery

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNormalizeImageRef(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "image with tag",
			input:    "quay.io/user/app:latest",
			expected: "quay.io/user/app",
		},
		{
			name:     "image with digest",
			input:    "quay.io/user/app@sha256:abc123",
			expected: "quay.io/user/app",
		},
		{
			name:     "image without tag or digest",
			input:    "quay.io/user/app",
			expected: "quay.io/user/app",
		},
		{
			name:     "image with port and tag",
			input:    "registry.example.com:5000/app:v1.0",
			expected: "registry.example.com:5000/app",
		},
		{
			name:     "docker hub short name with tag",
			input:    "nginx:alpine",
			expected: "nginx",
		},
		{
			name:     "image with version tag",
			input:    "quay.io/org/service:v2.3.1",
			expected: "quay.io/org/service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeImageRef(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeImageRef(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractComponentName(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		image    string
		expected string
	}{
		{
			name: "has app.kubernetes.io/name label",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": "my-service",
					},
				},
			},
			image:    "quay.io/org/my-service:latest",
			expected: "my-service",
		},
		{
			name: "has app label",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "backend",
					},
				},
			},
			image:    "quay.io/org/backend:latest",
			expected: "backend",
		},
		{
			name: "has app.kubernetes.io/component label",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": "api-gateway",
					},
				},
			},
			image:    "quay.io/org/gateway:latest",
			expected: "api-gateway",
		},
		{
			name: "prefers app.kubernetes.io/name over app",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": "preferred",
						"app":                    "fallback",
					},
				},
			},
			image:    "quay.io/org/something:latest",
			expected: "preferred",
		},
		{
			name: "falls back to image name with tag",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			image:    "quay.io/org/my-app:v1.0",
			expected: "my-app",
		},
		{
			name: "falls back to image name with digest",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			image:    "quay.io/org/my-app@sha256:abc123",
			expected: "my-app",
		},
		{
			name: "no labels and empty image returns empty string",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			image:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentName(tt.pod, tt.image)
			if result != tt.expected {
				t.Errorf("extractComponentName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestIsSystemNamespace(t *testing.T) {
	tests := []struct {
		name     string
		ns       string
		expected bool
	}{
		{"kube-system", "kube-system", true},
		{"kube-public", "kube-public", true},
		{"kube-node-lease", "kube-node-lease", true},
		{"openshift", "openshift", true},
		{"openshift-monitoring", "openshift-monitoring", true},
		{"openshift-console", "openshift-console", true},
		{"default", "default", true},
		{"user namespace", "my-namespace", false},
		{"custom namespace", "test-env", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSystemNamespace(tt.ns)
			if result != tt.expected {
				t.Errorf("isSystemNamespace(%q) = %v, want %v", tt.ns, result, tt.expected)
			}
		})
	}
}

func TestDiscoverPodsByImages(t *testing.T) {
	tests := []struct {
		name      string
		pods      []runtime.Object
		images    []string
		namespace string
		wantCount int
		wantError bool
	}{
		{
			name: "finds pod by exact image match",
			pods: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app-pod",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "myapp"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "main", Image: "quay.io/org/app:latest"},
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
			},
			images:    []string{"quay.io/org/app:latest"},
			namespace: "test-ns",
			wantCount: 1,
		},
		{
			name: "skips non-running pods",
			pods: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pending-pod",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "myapp"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "main", Image: "quay.io/org/app:latest"},
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodPending},
				},
			},
			images:    []string{"quay.io/org/app:latest"},
			namespace: "test-ns",
			wantCount: 0,
		},
		{
			name: "no matching images",
			pods: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-pod",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "other"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "main", Image: "quay.io/org/other:latest"},
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
			},
			images:    []string{"quay.io/org/app:latest"},
			namespace: "test-ns",
			wantCount: 0,
		},
		{
			name: "finds multiple pods across namespaces",
			pods: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2"}},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "ns1",
						Labels:    map[string]string{"app": "myapp"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "main", Image: "quay.io/org/app:v1"},
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "ns2",
						Labels:    map[string]string{"app": "myapp"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "main", Image: "quay.io/org/app:v1"},
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
			},
			images:    []string{"quay.io/org/app:v1"},
			namespace: "", // search all
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tt.pods...)
			disco := NewImageDiscovery(clientset)

			result, err := disco.DiscoverPodsByImages(context.Background(), tt.images, tt.namespace)
			if tt.wantError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(result) != tt.wantCount {
				t.Errorf("got %d pods, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestDiscoverPodsByLabelSelector(t *testing.T) {
	pods := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "running-pod",
				Namespace: "test-ns",
				Labels:    map[string]string{"app": "myapp", "env": "test"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "quay.io/org/app:latest"},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pending-pod",
				Namespace: "test-ns",
				Labels:    map[string]string{"app": "myapp", "env": "test"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "quay.io/org/app:latest"},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	}

	clientset := fake.NewSimpleClientset(pods...)
	disco := NewImageDiscovery(clientset)

	result, err := disco.DiscoverPodsByLabelSelector(context.Background(), "test-ns", "app=myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("got %d pods, want 1 (only running)", len(result))
	}

	if len(result) > 0 {
		if result[0].Name != "running-pod" {
			t.Errorf("got pod %q, want %q", result[0].Name, "running-pod")
		}
		if result[0].ContainerName != "main" {
			t.Errorf("got container %q, want %q", result[0].ContainerName, "main")
		}
	}
}
