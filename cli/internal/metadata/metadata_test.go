package metadata

import (
	"testing"
)

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"123", true},
		{"0", true},
		{"9999999", true},
		{"", false},
		{"abc", false},
		{"12a3", false},
		{"12.3", false},
		{"-1", false},
		{" 123", false},
		{"123 ", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isNumeric(tt.input)
			if result != tt.expected {
				t.Errorf("isNumeric(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractPRNumber(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]interface{}
		branch      string
		expected    string
	}{
		{
			name: "from pipelinesascode annotation",
			annotations: map[string]interface{}{
				"pipelinesascode.tekton.dev/pull-request": "42",
			},
			branch:   "",
			expected: "42",
		},
		{
			name: "from appstudio annotation",
			annotations: map[string]interface{}{
				"build.appstudio.redhat.com/pull_request_number": "99",
			},
			branch:   "",
			expected: "99",
		},
		{
			name:        "from GitHub branch pull/123/head",
			annotations: map[string]interface{}{},
			branch:      "pull/123/head",
			expected:    "123",
		},
		{
			name:        "from refs/pull/456/head",
			annotations: map[string]interface{}{},
			branch:      "refs/pull/456/head",
			expected:    "456",
		},
		{
			name:        "from pr-789 branch",
			annotations: map[string]interface{}{},
			branch:      "pr-789",
			expected:    "789",
		},
		{
			name:        "from pr/321 branch",
			annotations: map[string]interface{}{},
			branch:      "pr/321",
			expected:    "321",
		},
		{
			name:        "non-PR branch",
			annotations: map[string]interface{}{},
			branch:      "main",
			expected:    "",
		},
		{
			name:        "feature branch",
			annotations: map[string]interface{}{},
			branch:      "feature/add-tests",
			expected:    "",
		},
		{
			name:        "empty annotations and branch",
			annotations: map[string]interface{}{},
			branch:      "",
			expected:    "",
		},
		{
			name: "annotation takes precedence over branch",
			annotations: map[string]interface{}{
				"pipelinesascode.tekton.dev/pull-request": "100",
			},
			branch:   "pull/200/head",
			expected: "100",
		},
		{
			name:        "pr- with non-numeric suffix",
			annotations: map[string]interface{}{},
			branch:      "pr-abc",
			expected:    "",
		},
		{
			name:        "pull/ with non-numeric part",
			annotations: map[string]interface{}{},
			branch:      "pull/abc/head",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPRNumber(tt.annotations, tt.branch)
			if result != tt.expected {
				t.Errorf("extractPRNumber() = %q, want %q", result, tt.expected)
			}
		})
	}
}
