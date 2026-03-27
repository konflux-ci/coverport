package cmd

import (
	"os"
	"testing"
)

func TestTruncateImage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short image unchanged",
			input:    "quay.io/user/app:latest",
			expected: "quay.io/user/app:latest",
		},
		{
			name:     "exactly 60 chars unchanged",
			input:    "quay.io/very-long-organization-name/somewhat-long-app-name:v", // 60 chars
			expected: "quay.io/very-long-organization-name/somewhat-long-app-name:v",
		},
		{
			name:     "long image truncated",
			input:    "quay.io/very-long-organization/very-long-repository-name@sha256:abcdef1234567890abcdef1234567890",
			expected: "quay.io/very-long-organization/very-long-repository-name@...",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateImage(tt.input)
			if result != tt.expected {
				t.Errorf("truncateImage(%q) = %q, want %q", tt.input, result, tt.expected)
			}
			if len(result) > 60 {
				t.Errorf("truncated result should be <= 60 chars, got %d", len(result))
			}
		})
	}
}

func TestExtractComponentNameFromLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		image    string
		expected string
	}{
		{
			name:     "app.kubernetes.io/name label",
			labels:   map[string]string{"app.kubernetes.io/name": "my-service"},
			image:    "quay.io/org/app:v1",
			expected: "my-service",
		},
		{
			name:     "app label",
			labels:   map[string]string{"app": "backend"},
			image:    "quay.io/org/app:v1",
			expected: "backend",
		},
		{
			name:     "app.kubernetes.io/component label",
			labels:   map[string]string{"app.kubernetes.io/component": "api"},
			image:    "quay.io/org/app:v1",
			expected: "api",
		},
		{
			name:     "label precedence: name > app > component",
			labels:   map[string]string{"app.kubernetes.io/name": "winner", "app": "loser", "app.kubernetes.io/component": "also-loser"},
			image:    "quay.io/org/app:v1",
			expected: "winner",
		},
		{
			name:     "fallback to image name with tag",
			labels:   map[string]string{},
			image:    "quay.io/org/my-cool-app:v2.0",
			expected: "my-cool-app",
		},
		{
			name:     "fallback to image name with digest",
			labels:   map[string]string{},
			image:    "quay.io/org/my-app@sha256:abc123",
			expected: "my-app",
		},
		{
			name:     "empty labels and empty image returns empty string",
			labels:   map[string]string{},
			image:    "",
			expected: "",
		},
		{
			name:     "nil labels",
			labels:   nil,
			image:    "quay.io/org/app:latest",
			expected: "app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentNameFromLabels(tt.labels, tt.image)
			if result != tt.expected {
				t.Errorf("extractComponentNameFromLabels() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractRepoSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "HTTPS GitHub URL",
			input:    "https://github.com/owner/repo",
			expected: "owner/repo",
		},
		{
			name:     "HTTPS with .git suffix",
			input:    "https://github.com/owner/repo.git",
			expected: "owner/repo",
		},
		{
			name:     "SSH GitHub URL",
			input:    "git@github.com:owner/repo.git",
			expected: "owner/repo",
		},
		{
			name:     "SSH with ssh:// prefix",
			input:    "ssh://git@github.com/owner/repo",
			expected: "owner/repo",
		},
		{
			name:     "GitLab URL",
			input:    "https://gitlab.com/group/project",
			expected: "group/project",
		},
		{
			name:     "HTTP URL",
			input:    "http://github.com/owner/repo",
			expected: "owner/repo",
		},
		{
			name:     "nested group GitLab",
			input:    "https://gitlab.com/group/subgroup/project",
			expected: "subgroup/project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRepoSlug(tt.input)
			if result != tt.expected {
				t.Errorf("extractRepoSlug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractGitService(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"GitHub", "https://github.com/owner/repo", "github"},
		{"GitLab", "https://gitlab.com/group/project", "gitlab"},
		{"Bitbucket", "https://bitbucket.org/owner/repo", "bitbucket"},
		{"GitHub Enterprise", "https://github.example.com/owner/repo", "github_enterprise"},
		{"GitLab Enterprise", "https://gitlab.internal.com/group/project", "gitlab_enterprise"},
		{"Bitbucket Server", "https://bitbucket.internal.com/owner/repo", "bitbucket_server"},
		{"Unknown defaults to GitHub", "https://unknown-git.example.com/owner/repo", "github"},
		{"case insensitive", "https://GitHub.com/owner/repo", "github"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractGitService(tt.input)
			if result != tt.expected {
				t.Errorf("extractGitService(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsHTTPURL(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"http://localhost:9095", true},
		{"https://example.com", true},
		{"http://localhost:9095/coverage", true},
		{"quay.io/user/app:v1", false},
		{"ssh://git@github.com/org/repo", false},
		{"", false},
		{"ftp://files.example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isHTTPURL(tt.input)
			if result != tt.expected {
				t.Errorf("isHTTPURL(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSetupWorkspace(t *testing.T) {
	t.Run("with explicit directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		dir := tmpDir + "/workspace"

		result, err := setupWorkspace(dir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != dir {
			t.Errorf("got %q, want %q", result, dir)
		}

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Error("workspace directory should be created")
		}
	})

	t.Run("with empty dir creates temp", func(t *testing.T) {
		result, err := setupWorkspace("", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == "" {
			t.Error("expected non-empty temp directory path")
		}
		defer os.RemoveAll(result)

		if _, err := os.Stat(result); os.IsNotExist(err) {
			t.Error("temp directory should exist")
		}
	})
}

func TestCleanupWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := tmpDir + "/workspace"
	os.MkdirAll(wsDir, 0755)
	os.WriteFile(wsDir+"/testfile", []byte("hello"), 0644)

	cleanupWorkspace(wsDir)

	if _, err := os.Stat(wsDir); !os.IsNotExist(err) {
		t.Error("workspace should be removed after cleanup")
	}
}

func TestTruncateForDisplay(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"empty string", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateForDisplay(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateForDisplay(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestReadArtifactMetadata(t *testing.T) {
	t.Run("valid metadata", func(t *testing.T) {
		tmpDir := t.TempDir()
		content := `{"pod_name": "test-pod", "namespace": "test-ns"}`
		os.WriteFile(tmpDir+"/metadata.json", []byte(content), 0644)

		meta, err := readArtifactMetadata(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if meta["pod_name"] != "test-pod" {
			t.Errorf("expected pod_name 'test-pod', got %v", meta["pod_name"])
		}
	})

	t.Run("missing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := readArtifactMetadata(tmpDir)
		if err == nil {
			t.Error("expected error for missing metadata.json")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(tmpDir+"/metadata.json", []byte("{bad"), 0644)
		_, err := readArtifactMetadata(tmpDir)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestCopyCoverageToRepo(t *testing.T) {
	tmpDir := t.TempDir()
	src := tmpDir + "/coverage.out"
	dst := tmpDir + "/repo/coverage.out"

	os.MkdirAll(tmpDir+"/repo", 0755)
	os.WriteFile(src, []byte("mode: atomic\nfile.go:1.1,2.2 1 1"), 0644)

	if err := copyCoverageToRepo(src, dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "mode: atomic\nfile.go:1.1,2.2 1 1" {
		t.Error("file content mismatch")
	}
}
