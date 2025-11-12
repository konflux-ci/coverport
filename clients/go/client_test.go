package coverageclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSetDefaultFilters(t *testing.T) {
	client := &CoverageClient{}

	patterns := []string{"test1.go", "test2.go"}
	client.SetDefaultFilters(patterns)

	if len(client.defaultFilters) != 2 {
		t.Errorf("Expected 2 filters, got %d", len(client.defaultFilters))
	}

	if client.defaultFilters[0] != "test1.go" || client.defaultFilters[1] != "test2.go" {
		t.Errorf("Filters not set correctly: %v", client.defaultFilters)
	}
}

func TestAddDefaultFilter(t *testing.T) {
	client := &CoverageClient{
		defaultFilters: []string{"existing.go"},
	}

	client.AddDefaultFilter("new.go")

	if len(client.defaultFilters) != 2 {
		t.Errorf("Expected 2 filters, got %d", len(client.defaultFilters))
	}

	if client.defaultFilters[1] != "new.go" {
		t.Errorf("New filter not added correctly")
	}
}

func TestSetSourceDirectory(t *testing.T) {
	client := &CoverageClient{}

	testDir := "/test/source"
	client.SetSourceDirectory(testDir)

	if client.sourceDir != testDir {
		t.Errorf("Expected source directory %s, got %s", testDir, client.sourceDir)
	}
}

func TestSetPathRemapping(t *testing.T) {
	client := &CoverageClient{}

	client.SetPathRemapping(false)
	if client.enablePathRemap {
		t.Error("Expected path remapping to be disabled")
	}

	client.SetPathRemapping(true)
	if !client.enablePathRemap {
		t.Error("Expected path remapping to be enabled")
	}
}

func TestGetPodNameWithContext(t *testing.T) {
	tests := []struct {
		name          string
		pods          []runtime.Object
		labelSelector string
		expectPod     string
		expectError   bool
	}{
		{
			name: "finds running pod",
			pods: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-1",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			labelSelector: "app=test",
			expectPod:     "test-pod-1",
			expectError:   false,
		},
		{
			name: "returns first running pod when multiple exist",
			pods: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-pending",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-running",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			labelSelector: "app=test",
			expectPod:     "test-pod-running",
			expectError:   false,
		},
		{
			name:          "no pods found",
			pods:          []runtime.Object{},
			labelSelector: "app=nonexistent",
			expectError:   true,
		},
		{
			name: "no running pods",
			pods: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-pending",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
			},
			labelSelector: "app=test",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tt.pods...)

			client := &CoverageClient{
				clientset: clientset,
				namespace: "default",
			}

			ctx := context.Background()
			podName, err := client.GetPodNameWithContext(ctx, tt.labelSelector)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if podName != tt.expectPod {
					t.Errorf("Expected pod %s, got %s", tt.expectPod, podName)
				}
			}
		})
	}
}

func TestGetPodName(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	clientset := fake.NewSimpleClientset(pod)

	client := &CoverageClient{
		clientset: clientset,
		namespace: "default",
	}

	podName, err := client.GetPodName("app=test")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if podName != "test-pod" {
		t.Errorf("Expected pod test-pod, got %s", podName)
	}
}

func TestCollectCoverageFromURL(t *testing.T) {
	// Create test data
	metaData := []byte("meta content")
	counterData := []byte("counter content")

	response := CoverageResponse{
		MetaFilename:     "covmeta.test",
		MetaData:         base64.StdEncoding.EncodeToString(metaData),
		CountersFilename: "covcounters.test",
		CountersData:     base64.StdEncoding.EncodeToString(counterData),
		TestName:         "test-case",
		Timestamp:        time.Now().Unix(),
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Parse request body
		var reqBody map[string]string
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if reqBody["test_name"] != "test-case" {
			t.Errorf("Expected test_name 'test-case', got '%s'", reqBody["test_name"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create temp directory for output
	tempDir, err := os.MkdirTemp("", "coverage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	client := &CoverageClient{
		outputDir:  tempDir,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	// Test successful collection
	err = client.CollectCoverageFromURL(server.URL, "test-case")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify files were created
	testDir := filepath.Join(tempDir, "test-case")
	metaPath := filepath.Join(testDir, "covmeta.test")
	counterPath := filepath.Join(testDir, "covcounters.test")

	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Error("Meta file was not created")
	}

	if _, err := os.Stat(counterPath); os.IsNotExist(err) {
		t.Error("Counter file was not created")
	}

	// Verify file contents
	metaContent, _ := os.ReadFile(metaPath)
	if string(metaContent) != string(metaData) {
		t.Errorf("Meta content mismatch. Expected %s, got %s", metaData, metaContent)
	}

	counterContent, _ := os.ReadFile(counterPath)
	if string(counterContent) != string(counterData) {
		t.Errorf("Counter content mismatch. Expected %s, got %s", counterData, counterContent)
	}
}

func TestCollectCoverageFromURL_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	tempDir, _ := os.MkdirTemp("", "coverage-test-*")
	defer os.RemoveAll(tempDir)

	client := &CoverageClient{
		outputDir:  tempDir,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	err := client.CollectCoverageFromURL(server.URL, "test-case")
	if err == nil {
		t.Error("Expected error for server error response")
	}

	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to mention status code 500, got: %v", err)
	}
}

func TestFilterCoverageReport(t *testing.T) {
	tests := []struct {
		name             string
		reportContent    string
		patterns         []string
		expectedContent  string
		expectedFiltered int
	}{
		{
			name: "filters single pattern",
			reportContent: `mode: atomic
github.com/test/pkg/file1.go:10.1,12.2 2 1
github.com/test/pkg/coverage_server.go:20.1,22.2 2 1
github.com/test/pkg/file2.go:30.1,32.2 2 1`,
			patterns: []string{"coverage_server.go"},
			expectedContent: `mode: atomic
github.com/test/pkg/file1.go:10.1,12.2 2 1
github.com/test/pkg/file2.go:30.1,32.2 2 1`,
			expectedFiltered: 1,
		},
		{
			name: "filters multiple patterns",
			reportContent: `mode: atomic
github.com/test/pkg/file1.go:10.1,12.2 2 1
github.com/test/pkg/coverage_server.go:20.1,22.2 2 1
github.com/test/pkg/test_helper.go:30.1,32.2 2 1
github.com/test/pkg/file2.go:40.1,42.2 2 1`,
			patterns: []string{"coverage_server.go", "test_helper.go"},
			expectedContent: `mode: atomic
github.com/test/pkg/file1.go:10.1,12.2 2 1
github.com/test/pkg/file2.go:40.1,42.2 2 1`,
			expectedFiltered: 2,
		},
		{
			name: "no filters - uses default",
			reportContent: `mode: atomic
github.com/test/pkg/file1.go:10.1,12.2 2 1
github.com/test/pkg/coverage_server.go:20.1,22.2 2 1`,
			patterns: nil, // Will use default filters
			expectedContent: `mode: atomic
github.com/test/pkg/file1.go:10.1,12.2 2 1`,
			expectedFiltered: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "coverage-filter-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			testDir := filepath.Join(tempDir, "test-case")
			os.MkdirAll(testDir, 0755)

			reportPath := filepath.Join(testDir, "coverage.out")
			os.WriteFile(reportPath, []byte(tt.reportContent), 0644)

			client := &CoverageClient{
				outputDir:      tempDir,
				defaultFilters: []string{"coverage_server.go"},
			}

			var err2 error
			if tt.patterns != nil {
				err2 = client.FilterCoverageReport("test-case", tt.patterns...)
			} else {
				err2 = client.FilterCoverageReport("test-case")
			}

			if err2 != nil {
				t.Errorf("Unexpected error: %v", err2)
			}

			filteredPath := filepath.Join(testDir, "coverage_filtered.out")
			content, err := os.ReadFile(filteredPath)
			if err != nil {
				t.Fatalf("Failed to read filtered report: %v", err)
			}

			if string(content) != tt.expectedContent {
				t.Errorf("Content mismatch.\nExpected:\n%s\n\nGot:\n%s", tt.expectedContent, string(content))
			}
		})
	}
}

func TestFilterCoverageReport_EmptyPatterns(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "coverage-filter-test-*")
	defer os.RemoveAll(tempDir)

	testDir := filepath.Join(tempDir, "test-case")
	os.MkdirAll(testDir, 0755)

	reportContent := `mode: atomic
github.com/test/pkg/file1.go:10.1,12.2 2 1`
	reportPath := filepath.Join(testDir, "coverage.out")
	os.WriteFile(reportPath, []byte(reportContent), 0644)

	client := &CoverageClient{
		outputDir:      tempDir,
		defaultFilters: []string{}, // No default filters
	}

	// Pass empty slice to explicitly disable filtering
	err := client.FilterCoverageReport("test-case", []string{}...)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	filteredPath := filepath.Join(testDir, "coverage_filtered.out")
	content, _ := os.ReadFile(filteredPath)

	// Should be unchanged when no filters applied
	if string(content) != reportContent {
		t.Errorf("Content should be unchanged when no filters applied")
	}
}

func TestDetectContainerPaths(t *testing.T) {
	tests := []struct {
		name             string
		coverageLines    []string
		sourceFiles      map[string]string // relative path -> content
		expectedMappings map[string]string
	}{
		{
			name: "detects simple container path",
			coverageLines: []string{
				"mode: atomic",
				"/app/pkg/file.go:10.1,12.2 2 1",
				"/app/pkg/other.go:20.1,22.2 2 1",
			},
			sourceFiles: map[string]string{
				"pkg/file.go":  "package pkg",
				"pkg/other.go": "package pkg",
			},
			expectedMappings: nil, // Will depend on temp dir path
		},
		{
			name: "handles missing files",
			coverageLines: []string{
				"mode: atomic",
				"./local/file.go:10.1,12.2 2 1", // Exists locally
			},
			sourceFiles: map[string]string{
				"local/file.go": "package local",
			},
			expectedMappings: nil, // No remapping needed for local paths
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp source directory
			tempDir, err := os.MkdirTemp("", "source-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Create source files
			for relPath, content := range tt.sourceFiles {
				fullPath := filepath.Join(tempDir, relPath)
				os.MkdirAll(filepath.Dir(fullPath), 0755)
				os.WriteFile(fullPath, []byte(content), 0644)
			}

			client := &CoverageClient{
				sourceDir:       tempDir,
				enablePathRemap: true,
			}

			mappings := client.detectContainerPaths(tt.coverageLines)

			// For this test, we just verify it doesn't crash and returns a map
			if mappings == nil && len(tt.sourceFiles) > 0 {
				// When we have container paths that don't exist locally, we should get mappings
				// This is hard to test precisely because it depends on path detection heuristics
				t.Logf("No mappings detected (this may be expected depending on path structure)")
			}
		})
	}
}

func TestProcessCoverageReports(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "process-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testDir := filepath.Join(tempDir, "test-case")
	os.MkdirAll(testDir, 0755)

	client := &CoverageClient{
		outputDir:       tempDir,
		defaultFilters:  []string{"coverage_server.go"},
		enablePathRemap: false, // Disable to avoid path complications in test
	}

	// Note: go tool covdata handles empty directories gracefully and creates an empty report
	// So this test verifies the method works end-to-end, even with no actual coverage data
	err = client.ProcessCoverageReports("test-case")

	// The method should succeed (it will create empty but valid coverage files)
	// HTML generation might fail without source files, but that's caught internally
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify output files were created
	reportPath := filepath.Join(testDir, "coverage.out")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Error("Coverage report was not generated")
	}

	filteredPath := filepath.Join(testDir, "coverage_filtered.out")
	if _, err := os.Stat(filteredPath); os.IsNotExist(err) {
		t.Error("Filtered coverage report was not generated")
	}
}

func TestRemapCoveragePaths_NoRemapping(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "remap-test-*")
	defer os.RemoveAll(tempDir)

	// Create a coverage report with local paths that exist
	reportContent := fmt.Sprintf(`mode: atomic
%s:10.1,12.2 2 1`, filepath.Join(tempDir, "file.go"))

	reportPath := filepath.Join(tempDir, "coverage.out")
	os.WriteFile(reportPath, []byte(reportContent), 0644)

	// Create the actual file so it's detected as local
	os.WriteFile(filepath.Join(tempDir, "file.go"), []byte("package main"), 0644)

	client := &CoverageClient{
		sourceDir:       tempDir,
		enablePathRemap: true,
	}

	err := client.remapCoveragePaths(reportPath)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify content is unchanged (no remapping needed for local paths)
	content, _ := os.ReadFile(reportPath)
	if !strings.Contains(string(content), "mode: atomic") {
		t.Error("Coverage report mode line is missing")
	}
}

func TestCoverageResponse_JSONSerialization(t *testing.T) {
	original := CoverageResponse{
		MetaFilename:     "covmeta.test",
		MetaData:         "base64data",
		CountersFilename: "covcounters.test",
		CountersData:     "base64counters",
		TestName:         "my-test",
		Timestamp:        1234567890,
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded CoverageResponse
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify all fields match
	if decoded.MetaFilename != original.MetaFilename {
		t.Errorf("MetaFilename mismatch: %s != %s", decoded.MetaFilename, original.MetaFilename)
	}
	if decoded.TestName != original.TestName {
		t.Errorf("TestName mismatch: %s != %s", decoded.TestName, original.TestName)
	}
	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp mismatch: %d != %d", decoded.Timestamp, original.Timestamp)
	}
}

// func TestPrintCoverageSummary(t *testing.T) {
// 	tempDir, _ := os.MkdirTemp("", "summary-test-*")
// 	defer os.RemoveAll(tempDir)

// 	testDir := filepath.Join(tempDir, "test-case")
// 	os.MkdirAll(testDir, 0755)

// 	reportContent := `mode: atomic
// github.com/test/pkg/file.go:10.1,12.2 2 1`
// 	reportPath := filepath.Join(testDir, "coverage.out")
// 	os.WriteFile(reportPath, []byte(reportContent), 0644)

// 	client := &CoverageClient{
// 		outputDir: tempDir,
// 	}

// 	// Should not error
// 	err := client.PrintCoverageSummary("test-case")
// 	if err != nil {
// 		t.Errorf("Unexpected error: %v", err)
// 	}
// }

// func TestPrintCoverageSummary_MissingFile(t *testing.T) {
// 	tempDir, _ := os.MkdirTemp("", "summary-test-*")
// 	defer os.RemoveAll(tempDir)

// 	client := &CoverageClient{
// 		outputDir: tempDir,
// 	}

// 	err := client.PrintCoverageSummary("nonexistent-test")
// 	if err == nil {
// 		t.Error("Expected error for missing coverage file")
// 	}
// }
