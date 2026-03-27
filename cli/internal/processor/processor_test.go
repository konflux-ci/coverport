package processor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected CoverageFormat
		wantErr  bool
	}{
		{
			name: "Go coverage files",
			files: map[string]string{
				"covmeta.abc123":                "meta",
				"covcounters.abc123.1234.56789": "counters",
			},
			expected: FormatGo,
		},
		{
			name: "Python .coverage file",
			files: map[string]string{
				".coverage": "python coverage data",
			},
			expected: FormatPython,
		},
		{
			name: "Python coverage.xml file",
			files: map[string]string{
				"coverage.xml": "<coverage>...</coverage>",
			},
			expected: FormatPython,
		},
		{
			name: "NYC coverage-final.json",
			files: map[string]string{
				"coverage-final.json": "{}",
			},
			expected: FormatNYC,
		},
		{
			name:    "empty directory",
			files:   map[string]string{},
			wantErr: true,
		},
		{
			name: "unrecognized files",
			files: map[string]string{
				"README.md":  "readme",
				"config.yml": "config",
			},
			wantErr: true,
		},
		{
			name: "Go takes precedence over Python",
			files: map[string]string{
				"covmeta.abc123": "meta",
				".coverage":      "python",
			},
			expected: FormatGo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			format, err := DetectFormat(tmpDir)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if format != tt.expected {
				t.Errorf("got format %q, want %q", format, tt.expected)
			}
		})
	}
}

func TestDetectFormat_NonexistentDir(t *testing.T) {
	_, err := DetectFormat("/nonexistent/directory")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestDetectSourcePrefix(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		repoRoot string
		expected string
	}{
		{
			name: "container paths with common prefix",
			lines: []string{
				"mode: atomic",
				"/app/pkg/file1.go:10.1,12.2 2 1",
				"/app/pkg/file2.go:20.1,22.2 2 1",
				"/app/internal/util.go:30.1,32.2 2 1",
			},
			repoRoot: "/workspace/repo",
			expected: "/app/",
		},
		{
			name: "no absolute paths",
			lines: []string{
				"mode: atomic",
				"pkg/file1.go:10.1,12.2 2 1",
			},
			repoRoot: "/workspace",
			expected: "",
		},
		{
			name: "empty lines",
			lines: []string{
				"mode: atomic",
				"",
			},
			repoRoot: "/workspace",
			expected: "",
		},
		{
			name: "single file path",
			lines: []string{
				"mode: atomic",
				"/workspace/src/main.go:10.1,12.2 2 1",
			},
			repoRoot: "/workspace",
			expected: "/workspace/src/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectSourcePrefix(tt.lines, tt.repoRoot)
			if result != tt.expected {
				t.Errorf("detectSourcePrefix() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestApplyFilters(t *testing.T) {
	tmpDir := t.TempDir()

	coverageContent := `mode: atomic
github.com/test/pkg/main.go:10.1,12.2 2 1
github.com/test/pkg/coverage_server.go:20.1,22.2 2 1
github.com/test/pkg/handler.go:30.1,32.2 2 1
github.com/test/pkg/main_test.go:40.1,42.2 2 1`

	coverageFile := filepath.Join(tmpDir, "coverage.out")
	if err := os.WriteFile(coverageFile, []byte(coverageContent), 0644); err != nil {
		t.Fatal(err)
	}

	proc := NewCoverageProcessor(FormatGo)
	if err := proc.applyFilters(coverageFile, []string{"coverage_server.go", "_test.go"}); err != nil {
		t.Fatalf("applyFilters failed: %v", err)
	}

	filteredFile := filepath.Join(tmpDir, "coverage_filtered.out")
	data, err := os.ReadFile(filteredFile)
	if err != nil {
		t.Fatalf("failed to read filtered file: %v", err)
	}

	content := string(data)
	if strings.Contains(content, "coverage_server.go") {
		t.Error("filtered output should not contain coverage_server.go")
	}
	if strings.Contains(content, "main_test.go") {
		t.Error("filtered output should not contain main_test.go")
	}
	if !strings.Contains(content, "main.go") {
		t.Error("filtered output should contain main.go")
	}
	if !strings.Contains(content, "handler.go") {
		t.Error("filtered output should contain handler.go")
	}
	if !strings.Contains(content, "mode: atomic") {
		t.Error("filtered output should contain mode line")
	}
}

func TestRemapPathsToRelative(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot := filepath.Join(tmpDir, "repo")
	os.MkdirAll(repoRoot, 0755)

	coverageContent := `mode: atomic
/app/pkg/main.go:10.1,12.2 2 1
/app/pkg/handler.go:20.1,22.2 2 1`

	coverageFile := filepath.Join(tmpDir, "coverage.out")
	if err := os.WriteFile(coverageFile, []byte(coverageContent), 0644); err != nil {
		t.Fatal(err)
	}

	proc := NewCoverageProcessor(FormatGo)
	if err := proc.remapPathsToRelative(coverageFile, repoRoot); err != nil {
		t.Fatalf("remapPathsToRelative failed: %v", err)
	}

	data, err := os.ReadFile(coverageFile)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if strings.Contains(content, "/app/") {
		t.Error("remapped output should not contain absolute /app/ prefix")
	}
	if !strings.Contains(content, "mode: atomic") {
		t.Error("mode line should be preserved")
	}
}

// NYC-specific tests

func TestFindNYCCoverageFile(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantErr bool
	}{
		{
			name:    "coverage-final.json",
			files:   map[string]string{"coverage-final.json": "{}"},
			wantErr: false,
		},
		{
			name:    "out.json",
			files:   map[string]string{"out.json": "{}"},
			wantErr: false,
		},
		{
			name:    "no coverage file",
			files:   map[string]string{"readme.md": "hello"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			result, err := findNYCCoverageFile(tmpDir)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == "" {
				t.Error("expected non-empty result")
			}
		})
	}
}

func TestReadNYCCoverage(t *testing.T) {
	tmpDir := t.TempDir()

	coverageData := NYCCoverageData{
		"/app/src/index.js": &NYCFileCoverage{
			Path: "/app/src/index.js",
			StatementMap: map[string]NYCLocation{
				"0": {Start: NYCPosition{Line: 1, Column: 0}, End: NYCPosition{Line: 1, Column: 20}},
			},
			S: map[string]int{"0": 5},
			F: map[string]int{},
			B: map[string][]int{},
		},
	}

	data, err := json.MarshalIndent(coverageData, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(tmpDir, "coverage.json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := readNYCCoverage(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("got %d files, want 1", len(result))
	}

	fileCov, ok := result["/app/src/index.js"]
	if !ok {
		t.Fatal("expected /app/src/index.js in coverage data")
	}
	if fileCov.Path != "/app/src/index.js" {
		t.Errorf("path = %q, want %q", fileCov.Path, "/app/src/index.js")
	}
}

func TestReadNYCCoverage_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "bad.json")
	os.WriteFile(filePath, []byte("{invalid}"), 0644)

	_, err := readNYCCoverage(filePath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWriteNYCCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.json")

	coverageData := NYCCoverageData{
		"src/index.js": &NYCFileCoverage{
			Path: "src/index.js",
			S:    map[string]int{"0": 1},
			F:    map[string]int{},
			B:    map[string][]int{},
		},
	}

	if err := writeNYCCoverage(coverageData, outputPath); err != nil {
		t.Fatalf("writeNYCCoverage failed: %v", err)
	}

	// Read back and verify
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	var loaded NYCCoverageData
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if len(loaded) != 1 {
		t.Errorf("got %d entries, want 1", len(loaded))
	}
}

func TestRemapNYCPaths(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot := filepath.Join(tmpDir, "repo")
	os.MkdirAll(repoRoot, 0755)

	absRepoRoot, _ := filepath.Abs(repoRoot)

	coverageData := NYCCoverageData{
		"/app/src/index.js": &NYCFileCoverage{
			Path: "/app/src/index.js",
			S:    map[string]int{"0": 1},
			F:    map[string]int{},
			B:    map[string][]int{},
		},
		"/app/src/utils.js": &NYCFileCoverage{
			Path: "/app/src/utils.js",
			S:    map[string]int{"0": 2},
			F:    map[string]int{},
			B:    map[string][]int{},
		},
	}

	count := remapNYCPaths(coverageData, repoRoot)
	if count != 2 {
		t.Errorf("remapped %d paths, want 2", count)
	}

	// Verify paths were remapped to include repo root
	for path, cov := range coverageData {
		if !strings.HasPrefix(path, absRepoRoot) {
			t.Errorf("path %q should start with repo root %q", path, absRepoRoot)
		}
		if !strings.HasPrefix(cov.Path, absRepoRoot) {
			t.Errorf("cov.Path %q should start with repo root %q", cov.Path, absRepoRoot)
		}
	}
}

func TestDetectNYCSourcePrefix(t *testing.T) {
	tests := []struct {
		name     string
		data     NYCCoverageData
		expected string
	}{
		{
			name: "common prefix from absolute paths",
			data: NYCCoverageData{
				"/app/src/index.js": &NYCFileCoverage{},
				"/app/src/utils.js": &NYCFileCoverage{},
				"/app/lib/helper.js": &NYCFileCoverage{},
			},
			expected: "/app/",
		},
		{
			name: "relative paths only",
			data: NYCCoverageData{
				"src/index.js": &NYCFileCoverage{},
				"src/utils.js": &NYCFileCoverage{},
			},
			expected: "",
		},
		{
			name:     "empty data",
			data:     NYCCoverageData{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectNYCSourcePrefix(tt.data)
			if result != tt.expected {
				t.Errorf("detectNYCSourcePrefix() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGenerateLCOV(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "coverage.lcov")

	coverageData := NYCCoverageData{
		"src/index.js": &NYCFileCoverage{
			Path: "src/index.js",
			StatementMap: map[string]NYCLocation{
				"0": {Start: NYCPosition{Line: 1, Column: 0}, End: NYCPosition{Line: 1, Column: 20}},
				"1": {Start: NYCPosition{Line: 2, Column: 0}, End: NYCPosition{Line: 2, Column: 30}},
			},
			FnMap: map[string]NYCFunctionInfo{
				"0": {Name: "main", Line: 1, Loc: NYCLocation{Start: NYCPosition{Line: 1, Column: 0}, End: NYCPosition{Line: 5, Column: 1}}},
			},
			BranchMap: map[string]NYCBranchInfo{},
			S:         map[string]int{"0": 3, "1": 0},
			F:         map[string]int{"0": 1},
			B:         map[string][]int{},
		},
	}

	if err := generateLCOV(coverageData, outputPath); err != nil {
		t.Fatalf("generateLCOV failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)

	// Verify LCOV structure
	if !strings.Contains(content, "TN:") {
		t.Error("LCOV should contain TN: line")
	}
	if !strings.Contains(content, "SF:src/index.js") {
		t.Error("LCOV should contain SF: line")
	}
	if !strings.Contains(content, "FN:") {
		t.Error("LCOV should contain FN: line")
	}
	if !strings.Contains(content, "FNDA:") {
		t.Error("LCOV should contain FNDA: line")
	}
	if !strings.Contains(content, "DA:") {
		t.Error("LCOV should contain DA: line")
	}
	if !strings.Contains(content, "end_of_record") {
		t.Error("LCOV should contain end_of_record")
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "dst.txt")

	content := "hello world"
	if err := os.WriteFile(src, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("got %q, want %q", string(data), content)
	}
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyFile(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dst"))
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}
