package processor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindProfrawFiles(t *testing.T) {
	tests := []struct {
		name          string
		files         map[string]string
		expectedCount int
	}{
		{
			name: "single profraw file",
			files: map[string]string{
				"default_12345.profraw": "data",
			},
			expectedCount: 1,
		},
		{
			name: "multiple profraw files",
			files: map[string]string{
				"default_12345.profraw": "data1",
				"default_67890.profraw": "data2",
				"another.profraw":       "data3",
			},
			expectedCount: 3,
		},
		{
			name: "mixed files only returns profraw",
			files: map[string]string{
				"default_12345.profraw": "profraw",
				"coverage.profdata":     "profdata",
				"binary":               "elf",
				"report.lcov":          "lcov",
			},
			expectedCount: 1,
		},
		{
			name:          "empty directory",
			files:         map[string]string{},
			expectedCount: 0,
		},
		{
			name: "no profraw files",
			files: map[string]string{
				"coverage.profdata": "profdata",
				"report.lcov":      "lcov",
			},
			expectedCount: 0,
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

			files, err := findProfrawFiles(tmpDir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(files) != tt.expectedCount {
				t.Errorf("got %d files, want %d", len(files), tt.expectedCount)
			}

			for _, f := range files {
				if !strings.HasSuffix(f, ".profraw") {
					t.Errorf("returned file %q is not a .profraw file", f)
				}
				if !filepath.IsAbs(f) {
					t.Errorf("returned file %q is not an absolute path", f)
				}
			}
		})
	}
}

func TestFindProfrawFiles_NonexistentDir(t *testing.T) {
	_, err := findProfrawFiles("/nonexistent/directory")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestFindProfrawFiles_IgnoresSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory named with .profraw suffix (unlikely but tests robustness)
	subDir := filepath.Join(tmpDir, "subdir.profraw")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create an actual profraw file
	if err := os.WriteFile(filepath.Join(tmpDir, "real.profraw"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := findProfrawFiles(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("got %d files, want 1 (should ignore directory)", len(files))
	}
}

func TestDetectLCOVSourcePrefix(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected string
	}{
		{
			name: "common prefix from multiple SF lines",
			lines: []string{
				"TN:",
				"SF:/app/src/main.rs",
				"DA:1,5",
				"end_of_record",
				"SF:/app/src/lib.rs",
				"DA:1,3",
				"end_of_record",
			},
			expected: "/app/src/",
		},
		{
			name: "different subdirectories share parent prefix",
			lines: []string{
				"SF:/build/project/src/main.rs",
				"SF:/build/project/tests/test1.rs",
				"SF:/build/project/lib/utils.rs",
			},
			expected: "/build/project/",
		},
		{
			name: "single SF line returns its directory",
			lines: []string{
				"SF:/workspace/src/main.rs",
			},
			expected: "/workspace/src/",
		},
		{
			name:     "no SF lines returns empty",
			lines:    []string{"TN:", "DA:1,5", "end_of_record"},
			expected: "",
		},
		{
			name: "relative paths are ignored",
			lines: []string{
				"SF:src/main.rs",
				"SF:src/lib.rs",
			},
			expected: "",
		},
		{
			name: "mixed absolute and relative only considers absolute",
			lines: []string{
				"SF:/app/src/main.rs",
				"SF:relative/path.rs",
				"SF:/app/src/lib.rs",
			},
			expected: "/app/src/",
		},
		{
			name: "root-level files return empty (prefix would be /)",
			lines: []string{
				"SF:/main.rs",
				"SF:/lib.rs",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectLCOVSourcePrefix(tt.lines)
			if result != tt.expected {
				t.Errorf("detectLCOVSourcePrefix() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRemapRustPaths(t *testing.T) {
	tests := []struct {
		name           string
		lcovContent    string
		repoRoot       string
		expectContains []string
		expectAbsent   []string
	}{
		{
			name: "strips detected common prefix including src dir",
			lcovContent: `TN:
SF:/app/src/main.rs
DA:1,5
DA:2,3
end_of_record
SF:/app/src/lib.rs
DA:1,1
end_of_record`,
			repoRoot:       "/tmp/repo",
			expectContains: []string{"SF:main.rs", "SF:lib.rs"},
			expectAbsent:   []string{"SF:/app/"},
		},
		{
			name: "strips /build/ common prefix",
			lcovContent: `TN:
SF:/build/src/main.rs
DA:1,5
end_of_record`,
			repoRoot:       "/tmp/repo",
			expectContains: []string{"SF:main.rs"},
			expectAbsent:   []string{"SF:/build/"},
		},
		{
			name: "strips /workspace/ common prefix",
			lcovContent: `TN:
SF:/workspace/src/main.rs
DA:1,5
end_of_record`,
			repoRoot:       "/tmp/repo",
			expectContains: []string{"SF:main.rs"},
			expectAbsent:   []string{"SF:/workspace/"},
		},
		{
			name: "strips /app/ when files span multiple subdirs",
			lcovContent: `TN:
SF:/app/src/main.rs
DA:1,5
end_of_record
SF:/app/lib/utils.rs
DA:1,3
end_of_record`,
			repoRoot:       "/tmp/repo",
			expectContains: []string{"SF:src/main.rs", "SF:lib/utils.rs"},
			expectAbsent:   []string{"SF:/app/"},
		},
		{
			name: "strips detected deep common prefix",
			lcovContent: `TN:
SF:/custom/path/to/project/src/main.rs
DA:1,5
end_of_record
SF:/custom/path/to/project/src/lib.rs
DA:1,3
end_of_record`,
			repoRoot:       "/tmp/repo",
			expectContains: []string{"SF:main.rs", "SF:lib.rs"},
			expectAbsent:   []string{"SF:/custom/"},
		},
		{
			name: "preserves non-SF content",
			lcovContent: `TN:
SF:/app/src/main.rs
FN:1,main
FNDA:5,main
DA:1,5
DA:2,3
LH:2
LF:2
end_of_record`,
			repoRoot:       "/tmp/repo",
			expectContains: []string{"TN:", "FN:1,main", "FNDA:5,main", "DA:1,5", "LH:2", "end_of_record"},
			expectAbsent:   []string{"SF:/app/"},
		},
		{
			name: "relative paths are unchanged",
			lcovContent: `TN:
SF:src/main.rs
DA:1,5
end_of_record`,
			repoRoot:       "/tmp/repo",
			expectContains: []string{"SF:src/main.rs"},
			expectAbsent:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			lcovFile := filepath.Join(tmpDir, "coverage.lcov")
			if err := os.WriteFile(lcovFile, []byte(tt.lcovContent), 0644); err != nil {
				t.Fatal(err)
			}

			proc := NewCoverageProcessor(FormatRust)
			err := proc.remapRustPaths(lcovFile, tt.repoRoot)
			if err != nil {
				t.Fatalf("remapRustPaths failed: %v", err)
			}

			data, err := os.ReadFile(lcovFile)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)

			for _, expected := range tt.expectContains {
				if !strings.Contains(content, expected) {
					t.Errorf("expected output to contain %q, got:\n%s", expected, content)
				}
			}
			for _, absent := range tt.expectAbsent {
				if strings.Contains(content, absent) {
					t.Errorf("expected output NOT to contain %q, got:\n%s", absent, content)
				}
			}
		})
	}
}

func TestRemapRustPaths_NonexistentFile(t *testing.T) {
	proc := NewCoverageProcessor(FormatRust)
	err := proc.remapRustPaths("/nonexistent/file.lcov", "/tmp/repo")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestRemapRustPaths_RepoRootFallback(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot := filepath.Join(tmpDir, "myrepo")
	if err := os.MkdirAll(repoRoot, 0755); err != nil {
		t.Fatal(err)
	}

	absRoot, _ := filepath.Abs(repoRoot)

	// Use multiple files spanning different subdirs so the common prefix is the repo root
	lcovContent := "TN:\nSF:" + absRoot + "/src/main.rs\nDA:1,5\nend_of_record\nSF:" + absRoot + "/lib/utils.rs\nDA:1,3\nend_of_record\n"

	lcovFile := filepath.Join(tmpDir, "coverage.lcov")
	if err := os.WriteFile(lcovFile, []byte(lcovContent), 0644); err != nil {
		t.Fatal(err)
	}

	proc := NewCoverageProcessor(FormatRust)
	if err := proc.remapRustPaths(lcovFile, repoRoot); err != nil {
		t.Fatalf("remapRustPaths failed: %v", err)
	}

	data, err := os.ReadFile(lcovFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if strings.Contains(content, absRoot) {
		t.Errorf("expected repo root prefix to be stripped, got:\n%s", content)
	}
	if !strings.Contains(content, "SF:src/main.rs") {
		t.Errorf("expected relative path SF:src/main.rs, got:\n%s", content)
	}
	if !strings.Contains(content, "SF:lib/utils.rs") {
		t.Errorf("expected relative path SF:lib/utils.rs, got:\n%s", content)
	}
}

func TestFindInstrumentedBinary_EnvVar(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "myapp")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("COVERAGE_BINARY", binaryPath)

	opts := ProcessOptions{InputDir: tmpDir}
	result := findInstrumentedBinary(opts)
	if result != binaryPath {
		t.Errorf("got %q, want %q", result, binaryPath)
	}
}

func TestFindInstrumentedBinary_EnvVarNonexistent(t *testing.T) {
	t.Setenv("COVERAGE_BINARY", "/nonexistent/binary")

	tmpDir := t.TempDir()
	opts := ProcessOptions{InputDir: tmpDir}
	result := findInstrumentedBinary(opts)
	// Should fall through to other discovery methods; with empty dir, returns ""
	if result != "" {
		t.Errorf("expected empty string for nonexistent COVERAGE_BINARY, got %q", result)
	}
}

func TestFindInstrumentedBinary_ExecutableInInputDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a profraw file (should be skipped)
	os.WriteFile(filepath.Join(tmpDir, "default.profraw"), []byte("profraw"), 0644)

	// Create a profdata file (should be skipped)
	os.WriteFile(filepath.Join(tmpDir, "coverage.profdata"), []byte("profdata"), 0644)

	// Create a json file (should be skipped)
	os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte("{}"), 0644)

	// Create an executable binary
	binaryPath := filepath.Join(tmpDir, "myapp")
	os.WriteFile(binaryPath, []byte("binary"), 0755)

	t.Setenv("COVERAGE_BINARY", "")

	opts := ProcessOptions{InputDir: tmpDir}
	result := findInstrumentedBinary(opts)
	if result != binaryPath {
		t.Errorf("got %q, want %q", result, binaryPath)
	}
}

func TestFindInstrumentedBinary_RepoRootTarget(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "input")
	os.MkdirAll(inputDir, 0755)

	// Create target/release with an executable
	releaseDir := filepath.Join(tmpDir, "target", "release")
	os.MkdirAll(releaseDir, 0755)
	binaryPath := filepath.Join(releaseDir, "myapp")
	os.WriteFile(binaryPath, []byte("binary"), 0755)

	// Also create a .d file that should be ignored
	os.WriteFile(filepath.Join(releaseDir, "myapp.d"), []byte("dep info"), 0644)

	t.Setenv("COVERAGE_BINARY", "")

	opts := ProcessOptions{InputDir: inputDir, RepoRoot: tmpDir}
	result := findInstrumentedBinary(opts)
	if result != binaryPath {
		t.Errorf("got %q, want %q", result, binaryPath)
	}
}

func TestFindInstrumentedBinary_NoBinaryFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Only non-executable files
	os.WriteFile(filepath.Join(tmpDir, "data.profraw"), []byte("profraw"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("hello"), 0644)

	t.Setenv("COVERAGE_BINARY", "")

	opts := ProcessOptions{InputDir: tmpDir}
	result := findInstrumentedBinary(opts)
	if result != "" {
		t.Errorf("expected empty string when no binary found, got %q", result)
	}
}
