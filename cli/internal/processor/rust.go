package processor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// processRustCoverage processes Rust/LLVM profraw coverage data using llvm-profdata and llvm-cov
func (p *CoverageProcessor) processRustCoverage(ctx context.Context, opts ProcessOptions) error {
	fmt.Println("Processing Rust coverage data...")
	fmt.Printf("   Input: %s\n", opts.InputDir)
	fmt.Printf("   Output: %s\n", opts.OutputFile)

	// Find profraw files
	profrawFiles, err := findProfrawFiles(opts.InputDir)
	if err != nil {
		return fmt.Errorf("find profraw files: %w", err)
	}
	if len(profrawFiles) == 0 {
		return fmt.Errorf("no .profraw files found in %s", opts.InputDir)
	}
	fmt.Printf("   Found %d profraw file(s)\n", len(profrawFiles))

	// Find llvm-profdata tool
	llvmProfdata, err := findLLVMTool("llvm-profdata")
	if err != nil {
		return err
	}
	fmt.Printf("   Using: %s\n", llvmProfdata)

	// Find llvm-cov tool
	llvmCov, err := findLLVMTool("llvm-cov")
	if err != nil {
		return err
	}
	fmt.Printf("   Using: %s\n", llvmCov)

	// Create output directory
	if err := os.MkdirAll(filepath.Dir(opts.OutputFile), 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Step 1: Merge profraw files into profdata
	profdataPath := filepath.Join(opts.InputDir, "coverage.profdata")
	fmt.Println("   Merging profraw files...")
	mergeArgs := []string{"merge", "--sparse"}
	mergeArgs = append(mergeArgs, profrawFiles...)
	mergeArgs = append(mergeArgs, "-o", profdataPath)

	cmd := exec.CommandContext(ctx, llvmProfdata, mergeArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("llvm-profdata merge failed: %w\nOutput: %s", err, string(output))
	}
	fmt.Printf("   Merged into: %s\n", profdataPath)

	// Step 2: Find the instrumented binary
	binaryPath := findInstrumentedBinary(opts)
	if binaryPath == "" {
		return fmt.Errorf("no instrumented binary found; cannot generate LCOV report. Set COVERAGE_BINARY env var or place the binary in %s", opts.InputDir)
	}
	fmt.Printf("   Binary: %s\n", binaryPath)

	// Step 3: Export as LCOV (the standard format for Codecov/SonarCloud)
	fmt.Println("   Generating LCOV report...")
	exportArgs := []string{"export", "--format=lcov", "--instr-profile=" + profdataPath, binaryPath}

	// Apply ignore filters
	for _, filter := range opts.Filters {
		exportArgs = append(exportArgs, "--ignore-filename-regex="+filter)
	}

	cmd = exec.CommandContext(ctx, llvmCov, exportArgs...)
	lcovOutput, err := cmd.Output()
	if err != nil {
		fmt.Printf("   Warning: llvm-cov export with filters failed (%v), retrying without filters\n", err)
		cmd = exec.CommandContext(ctx, llvmCov, "export", "--format=lcov",
			"--instr-profile="+profdataPath, binaryPath)
		lcovOutput, err = cmd.Output()
		if err != nil {
			return fmt.Errorf("llvm-cov export failed: %w", err)
		}
	}

	if err := os.WriteFile(opts.OutputFile, lcovOutput, 0644); err != nil {
		return fmt.Errorf("write LCOV file: %w", err)
	}
	fmt.Printf("   LCOV report: %s (%d bytes)\n", opts.OutputFile, len(lcovOutput))

	// Step 4: Remap paths if repo root is available
	if opts.RepoRoot != "" {
		if err := p.remapRustPaths(opts.OutputFile, opts.RepoRoot); err != nil {
			fmt.Printf("   Warning: Path remapping failed: %v\n", err)
		}
	}

	// Step 5: Generate text summary
	fmt.Println("   Generating coverage summary...")
	reportArgs := []string{"report", "--use-color=false", "--instr-profile=" + profdataPath, binaryPath}
	for _, filter := range opts.Filters {
		reportArgs = append(reportArgs, "--ignore-filename-regex="+filter)
	}

	cmd = exec.CommandContext(ctx, llvmCov, reportArgs...)
	reportOutput, err := cmd.Output()
	if err == nil {
		// Save text report
		textPath := strings.TrimSuffix(opts.OutputFile, filepath.Ext(opts.OutputFile)) + ".txt"
		if writeErr := os.WriteFile(textPath, reportOutput, 0644); writeErr == nil {
			fmt.Printf("   Text report: %s\n", textPath)
		}

		// Print summary (last few lines typically contain totals)
		lines := strings.Split(string(reportOutput), "\n")
		for _, line := range lines {
			if strings.Contains(line, "TOTAL") {
				fmt.Printf("   %s\n", strings.TrimSpace(line))
				break
			}
		}
	}

	// Step 6: Generate HTML report if requested
	if opts.GenerateHTML {
		if err := p.generateRustHTMLReport(ctx, llvmCov, profdataPath, binaryPath, opts); err != nil {
			fmt.Printf("   Warning: HTML report generation failed: %v\n", err)
		}
	}

	fmt.Println("Rust coverage processed successfully!")
	return nil
}

// findProfrawFiles finds all .profraw files in the given directory
func findProfrawFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".profraw") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}

// findLLVMTool searches for an LLVM tool by trying multiple naming conventions
func findLLVMTool(name string) (string, error) {
	// Try exact name first
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}

	// Try versioned names (newest first)
	for version := 20; version >= 14; version-- {
		versioned := fmt.Sprintf("%s-%d", name, version)
		if path, err := exec.LookPath(versioned); err == nil {
			return path, nil
		}
	}

	// Try cargo-binutils style (rust-profdata, rust-cov)
	rustName := strings.Replace(name, "llvm-", "rust-", 1)
	if path, err := exec.LookPath(rustName); err == nil {
		return path, nil
	}

	// Try rustup sysroot (llvm-tools-preview component)
	if rustcPath, err := exec.LookPath("rustc"); err == nil {
		out, err := exec.Command(rustcPath, "--print", "sysroot").Output()
		if err == nil {
			sysroot := strings.TrimSpace(string(out))
			candidates := []string{
				filepath.Join(sysroot, "lib", "rustlib", runtimeTarget(), "bin", name),
				filepath.Join(sysroot, "bin", name),
			}
			for _, candidate := range candidates {
				if _, err := os.Stat(candidate); err == nil {
					return candidate, nil
				}
			}
		}
	}

	return "", fmt.Errorf("%s not found. Install LLVM tools or run: rustup component add llvm-tools-preview && cargo install cargo-binutils", name)
}

func runtimeTarget() string {
	return fmt.Sprintf("%s-%s", runtimeArch(), runtimeOs())
}

func runtimeArch() string {
	switch a := runtime.GOARCH; a {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return a
	}
}

func runtimeOs() string {
	switch runtime.GOOS {
	case "linux":
		return "unknown-linux-gnu"
	case "darwin":
		return "apple-darwin"
	case "windows":
		return "pc-windows-msvc"
	default:
		return "unknown-" + runtime.GOOS
	}
}

// findInstrumentedBinary tries to locate the instrumented binary for llvm-cov
func findInstrumentedBinary(opts ProcessOptions) string {
	// Check for COVERAGE_BINARY env var
	if binary := os.Getenv("COVERAGE_BINARY"); binary != "" {
		if _, err := os.Stat(binary); err == nil {
			return binary
		}
	}

	// Check for binary in the input directory (might have been copied alongside profraw)
	entries, _ := os.ReadDir(opts.InputDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(opts.InputDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		// Look for executable files that aren't profraw/profdata
		if info.Mode()&0111 != 0 &&
			!strings.HasSuffix(entry.Name(), ".profraw") &&
			!strings.HasSuffix(entry.Name(), ".profdata") &&
			!strings.HasSuffix(entry.Name(), ".json") {
			return path
		}
	}

	// Check in repo root target directories (common Rust build outputs)
	if opts.RepoRoot != "" {
		for _, profile := range []string{"release", "debug"} {
			targetDir := filepath.Join(opts.RepoRoot, "target", profile)
			if entries, err := os.ReadDir(targetDir); err == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						continue
					}
					info, err := entry.Info()
					if err != nil {
						continue
					}
					// Executable, not a .d file or other metadata
					if info.Mode()&0111 != 0 &&
						!strings.HasSuffix(entry.Name(), ".d") &&
						!strings.HasPrefix(entry.Name(), ".") {
						return filepath.Join(targetDir, entry.Name())
					}
				}
			}
		}
	}

	return ""
}

// remapRustPaths remaps absolute container paths in LCOV data to relative paths
func (p *CoverageProcessor) remapRustPaths(lcovFile, repoRoot string) error {
	data, err := os.ReadFile(lcovFile)
	if err != nil {
		return err
	}

	content := string(data)

	// Common container source prefixes to strip
	prefixes := []string{"/app/", "/build/", "/workspace/", "/src/"}

	// Also try to detect the prefix from the LCOV content (SF: lines contain source paths)
	lines := strings.Split(content, "\n")
	detectedPrefix := detectLCOVSourcePrefix(lines)
	if detectedPrefix != "" && detectedPrefix != "/" {
		prefixes = append([]string{detectedPrefix}, prefixes...)
	}

	remapped := false
	for _, prefix := range prefixes {
		if strings.Contains(content, prefix) {
			content = strings.ReplaceAll(content, "SF:"+prefix, "SF:")
			remapped = true
			fmt.Printf("   Remapped paths: removed prefix %q\n", prefix)
			break
		}
	}

	if !remapped {
		// Try to make paths relative to repo root
		absRoot, err := filepath.Abs(repoRoot)
		if err == nil && strings.Contains(content, absRoot) {
			content = strings.ReplaceAll(content, "SF:"+absRoot+"/", "SF:")
			fmt.Printf("   Remapped paths: removed repo root prefix\n")
		}
	}

	return os.WriteFile(lcovFile, []byte(content), 0644)
}

// detectLCOVSourcePrefix finds the common prefix of all SF: lines in LCOV data
func detectLCOVSourcePrefix(lines []string) string {
	var paths []string
	for _, line := range lines {
		if strings.HasPrefix(line, "SF:") {
			path := strings.TrimPrefix(line, "SF:")
			if filepath.IsAbs(path) {
				paths = append(paths, path)
			}
		}
	}

	if len(paths) == 0 {
		return ""
	}

	// Find common directory prefix
	prefix := filepath.Dir(paths[0])
	for _, path := range paths[1:] {
		dir := filepath.Dir(path)
		for !strings.HasPrefix(dir, prefix) && prefix != "/" && prefix != "." {
			prefix = filepath.Dir(prefix)
		}
	}

	if prefix != "" && prefix != "/" && prefix != "." {
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		return prefix
	}

	return ""
}

// generateRustHTMLReport generates an HTML coverage report for Rust
func (p *CoverageProcessor) generateRustHTMLReport(ctx context.Context, llvmCov, profdataPath, binaryPath string, opts ProcessOptions) error {
	fmt.Println("   Generating HTML coverage report...")

	htmlDir := filepath.Join(opts.InputDir, "html")

	args := []string{"show", "--format=html",
		"--instr-profile=" + profdataPath,
		"--output-dir=" + htmlDir,
		binaryPath}

	for _, filter := range opts.Filters {
		args = append(args, "--ignore-filename-regex="+filter)
	}

	cmd := exec.CommandContext(ctx, llvmCov, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("llvm-cov show --format=html failed: %w\nOutput: %s", err, string(output))
	}

	fmt.Printf("   HTML report: %s/index.html\n", htmlDir)
	return nil
}
