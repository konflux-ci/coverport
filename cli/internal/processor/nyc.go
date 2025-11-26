package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NYCFileCoverage represents Istanbul/NYC coverage data for a single file
type NYCFileCoverage struct {
	Path         string                       `json:"path"`
	StatementMap map[string]NYCLocation       `json:"statementMap"`
	FnMap        map[string]NYCFunctionInfo   `json:"fnMap"`
	BranchMap    map[string]NYCBranchInfo     `json:"branchMap"`
	S            map[string]int               `json:"s"`            // Statement counts
	F            map[string]int               `json:"f"`            // Function counts
	B            map[string][]int             `json:"b"`            // Branch counts
	InputSourceMap json.RawMessage            `json:"inputSourceMap,omitempty"`
}

// NYCLocation represents a source location in Istanbul format
type NYCLocation struct {
	Start NYCPosition `json:"start"`
	End   NYCPosition `json:"end"`
}

// NYCPosition represents a position (line, column) in source
type NYCPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// NYCFunctionInfo represents function information in Istanbul format
type NYCFunctionInfo struct {
	Name string      `json:"name"`
	Decl NYCLocation `json:"decl"`
	Loc  NYCLocation `json:"loc"`
	Line int         `json:"line"`
}

// NYCBranchInfo represents branch information in Istanbul format
type NYCBranchInfo struct {
	Type      string        `json:"type"`
	Locations []NYCLocation `json:"locations"`
	Line      int           `json:"line"`
}

// NYCCoverageData is a map of file paths to their coverage data
type NYCCoverageData map[string]*NYCFileCoverage

// processNYCCoverage processes NYC (Node.js/Istanbul) coverage data
func (p *CoverageProcessor) processNYCCoverage(ctx context.Context, opts ProcessOptions) error {
	fmt.Println("🔄 Processing NYC/Istanbul coverage data...")
	fmt.Printf("   Input: %s\n", opts.InputDir)
	fmt.Printf("   Output: %s\n", opts.OutputFile)

	// Find the coverage JSON file
	coverageFile, err := findNYCCoverageFile(opts.InputDir)
	if err != nil {
		return fmt.Errorf("failed to find NYC coverage file: %w", err)
	}
	fmt.Printf("   Found coverage file: %s\n", coverageFile)

	// Read the coverage data
	coverageData, err := readNYCCoverage(coverageFile)
	if err != nil {
		return fmt.Errorf("failed to read NYC coverage: %w", err)
	}
	fmt.Printf("   📊 Found coverage for %d files\n", len(coverageData))

	// Remap paths if repo root is provided
	if opts.RepoRoot != "" {
		remappedCount := remapNYCPaths(coverageData, opts.RepoRoot)
		fmt.Printf("   ✅ Remapped %d file paths\n", remappedCount)
	}

	// Create output directory
	if err := os.MkdirAll(filepath.Dir(opts.OutputFile), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write the remapped coverage JSON
	outputJSON := strings.TrimSuffix(opts.OutputFile, filepath.Ext(opts.OutputFile)) + ".json"
	if err := writeNYCCoverage(coverageData, outputJSON); err != nil {
		return fmt.Errorf("failed to write remapped coverage: %w", err)
	}
	fmt.Printf("   📄 Wrote remapped coverage to: %s\n", outputJSON)

	// Generate LCOV format for Codecov compatibility
	lcovFile := strings.TrimSuffix(opts.OutputFile, filepath.Ext(opts.OutputFile)) + ".lcov"
	if err := generateLCOV(coverageData, lcovFile); err != nil {
		fmt.Printf("   ⚠️  Failed to generate LCOV: %v\n", err)
	} else {
		fmt.Printf("   📄 Generated LCOV report: %s\n", lcovFile)
	}

	// Copy to output file location for consistency
	if opts.OutputFile != outputJSON {
		if err := copyFile(outputJSON, opts.OutputFile); err != nil {
			fmt.Printf("   ⚠️  Failed to copy to output file: %v\n", err)
		}
	}

	// Show coverage summary
	showNYCCoverageSummary(coverageData)

	fmt.Println("✅ NYC coverage processed successfully!")
	return nil
}

// findNYCCoverageFile finds the NYC coverage file in the input directory
func findNYCCoverageFile(inputDir string) (string, error) {
	// Check for common NYC coverage file names
	candidates := []string{
		filepath.Join(inputDir, "coverage-final.json"),
		filepath.Join(inputDir, "out.json"),
		filepath.Join(inputDir, ".nyc_output", "out.json"),
		filepath.Join(inputDir, ".nyc_output", "coverage-final.json"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// Look for any .json file in .nyc_output directory
	nycOutputDir := filepath.Join(inputDir, ".nyc_output")
	if entries, err := os.ReadDir(nycOutputDir); err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				return filepath.Join(nycOutputDir, entry.Name()), nil
			}
		}
	}

	// Look for any coverage*.json file in input directory
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return "", fmt.Errorf("failed to read input directory: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".json") && strings.Contains(strings.ToLower(name), "coverage") {
			return filepath.Join(inputDir, name), nil
		}
	}

	return "", fmt.Errorf("no NYC coverage file found in %s", inputDir)
}

// readNYCCoverage reads NYC/Istanbul coverage JSON
func readNYCCoverage(filePath string) (NYCCoverageData, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var coverageData NYCCoverageData
	if err := json.Unmarshal(data, &coverageData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return coverageData, nil
}

// remapNYCPaths remaps absolute container paths to relative paths
func remapNYCPaths(coverageData NYCCoverageData, repoRoot string) int {
	// Detect the common source prefix from coverage paths
	sourcePrefix := detectNYCSourcePrefix(coverageData)

	absRepoRoot, _ := filepath.Abs(repoRoot)
	remappedCount := 0

	// Build new map with remapped paths
	remappedData := make(NYCCoverageData)

	for oldPath, fileCoverage := range coverageData {
		newPath := oldPath

		// Try to remap using detected source prefix
		if sourcePrefix != "" && strings.HasPrefix(oldPath, sourcePrefix) {
			relativePath := strings.TrimPrefix(oldPath, sourcePrefix)
			newPath = filepath.Join(absRepoRoot, relativePath)
			remappedCount++
		} else if filepath.IsAbs(oldPath) {
			// Try common container prefixes
			containerPrefixes := []string{
				"/opt/app-root/src/",
				"/workspace/",
				"/app/",
				"/src/",
				"/home/node/app/",
			}

			for _, prefix := range containerPrefixes {
				if strings.HasPrefix(oldPath, prefix) {
					relativePath := strings.TrimPrefix(oldPath, prefix)
					newPath = filepath.Join(absRepoRoot, relativePath)
					remappedCount++
					break
				}
			}
		} else if !filepath.IsAbs(oldPath) {
			// Already relative, just prepend repo root
			newPath = filepath.Join(absRepoRoot, oldPath)
			remappedCount++
		}

		// Update the path field in the coverage data
		fileCoverage.Path = newPath
		remappedData[newPath] = fileCoverage
	}

	// Replace original data
	for k := range coverageData {
		delete(coverageData, k)
	}
	for k, v := range remappedData {
		coverageData[k] = v
	}

	return remappedCount
}

// detectNYCSourcePrefix detects the common source path prefix from coverage data
func detectNYCSourcePrefix(coverageData NYCCoverageData) string {
	var paths []string
	for path := range coverageData {
		if filepath.IsAbs(path) {
			paths = append(paths, path)
		}
	}

	if len(paths) == 0 {
		return ""
	}

	// Find common prefix
	commonPrefix := filepath.Dir(paths[0])
	for _, path := range paths[1:] {
		dir := filepath.Dir(path)
		for !strings.HasPrefix(dir, commonPrefix) && commonPrefix != "/" && commonPrefix != "." {
			commonPrefix = filepath.Dir(commonPrefix)
		}
	}

	// Ensure it ends with separator
	if commonPrefix != "" && commonPrefix != "/" && commonPrefix != "." {
		if !strings.HasSuffix(commonPrefix, string(filepath.Separator)) {
			commonPrefix += string(filepath.Separator)
		}
		return commonPrefix
	}

	return ""
}

// writeNYCCoverage writes the coverage data to a JSON file
func writeNYCCoverage(coverageData NYCCoverageData, outputPath string) error {
	data, err := json.MarshalIndent(coverageData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// generateLCOV generates LCOV format from NYC coverage data
func generateLCOV(coverageData NYCCoverageData, outputPath string) error {
	var lcov strings.Builder

	for filePath, fileCoverage := range coverageData {
		lcov.WriteString("TN:\n")
		lcov.WriteString(fmt.Sprintf("SF:%s\n", filePath))

		// Function coverage
		for fnID, fnInfo := range fileCoverage.FnMap {
			lcov.WriteString(fmt.Sprintf("FN:%d,%s\n", fnInfo.Line, fnInfo.Name))
			if count, ok := fileCoverage.F[fnID]; ok {
				lcov.WriteString(fmt.Sprintf("FNDA:%d,%s\n", count, fnInfo.Name))
			}
		}
		lcov.WriteString(fmt.Sprintf("FNF:%d\n", len(fileCoverage.FnMap)))

		fnHit := 0
		for _, count := range fileCoverage.F {
			if count > 0 {
				fnHit++
			}
		}
		lcov.WriteString(fmt.Sprintf("FNH:%d\n", fnHit))

		// Line coverage (derived from statements)
		lineHits := make(map[int]int)
		for stmtID, loc := range fileCoverage.StatementMap {
			if count, ok := fileCoverage.S[stmtID]; ok {
				line := loc.Start.Line
				if existing, exists := lineHits[line]; exists {
					if count > existing {
						lineHits[line] = count
					}
				} else {
					lineHits[line] = count
				}
			}
		}

		for line, count := range lineHits {
			lcov.WriteString(fmt.Sprintf("DA:%d,%d\n", line, count))
		}

		lcov.WriteString(fmt.Sprintf("LF:%d\n", len(lineHits)))

		lh := 0
		for _, count := range lineHits {
			if count > 0 {
				lh++
			}
		}
		lcov.WriteString(fmt.Sprintf("LH:%d\n", lh))

		// Branch coverage
		branchID := 0
		for _, branchInfo := range fileCoverage.BranchMap {
			for i := range branchInfo.Locations {
				count := 0
				if branchCounts, ok := fileCoverage.B[fmt.Sprintf("%d", branchID)]; ok && i < len(branchCounts) {
					count = branchCounts[i]
				}
				lcov.WriteString(fmt.Sprintf("BRDA:%d,%d,%d,%d\n", branchInfo.Line, branchID, i, count))
			}
			branchID++
		}

		totalBranches := 0
		hitBranches := 0
		for _, counts := range fileCoverage.B {
			for _, count := range counts {
				totalBranches++
				if count > 0 {
					hitBranches++
				}
			}
		}
		lcov.WriteString(fmt.Sprintf("BRF:%d\n", totalBranches))
		lcov.WriteString(fmt.Sprintf("BRH:%d\n", hitBranches))

		lcov.WriteString("end_of_record\n")
	}

	return os.WriteFile(outputPath, []byte(lcov.String()), 0644)
}

// showNYCCoverageSummary displays a summary of the NYC coverage
func showNYCCoverageSummary(coverageData NYCCoverageData) {
	totalStatements := 0
	coveredStatements := 0
	totalFunctions := 0
	coveredFunctions := 0
	totalBranches := 0
	coveredBranches := 0

	for _, fileCoverage := range coverageData {
		// Statements
		for stmtID := range fileCoverage.StatementMap {
			totalStatements++
			if count, ok := fileCoverage.S[stmtID]; ok && count > 0 {
				coveredStatements++
			}
		}

		// Functions
		for fnID := range fileCoverage.FnMap {
			totalFunctions++
			if count, ok := fileCoverage.F[fnID]; ok && count > 0 {
				coveredFunctions++
			}
		}

		// Branches
		for branchID, branchInfo := range fileCoverage.BranchMap {
			for i := range branchInfo.Locations {
				totalBranches++
				if counts, ok := fileCoverage.B[branchID]; ok && i < len(counts) && counts[i] > 0 {
					coveredBranches++
				}
			}
		}
	}

	stmtPct := float64(0)
	if totalStatements > 0 {
		stmtPct = float64(coveredStatements) / float64(totalStatements) * 100
	}

	fnPct := float64(0)
	if totalFunctions > 0 {
		fnPct = float64(coveredFunctions) / float64(totalFunctions) * 100
	}

	branchPct := float64(0)
	if totalBranches > 0 {
		branchPct = float64(coveredBranches) / float64(totalBranches) * 100
	}

	fmt.Println("\n   📊 Coverage Summary:")
	fmt.Printf("      Statements: %.2f%% (%d/%d)\n", stmtPct, coveredStatements, totalStatements)
	fmt.Printf("      Functions:  %.2f%% (%d/%d)\n", fnPct, coveredFunctions, totalFunctions)
	fmt.Printf("      Branches:   %.2f%% (%d/%d)\n", branchPct, coveredBranches, totalBranches)
	fmt.Printf("      Files:      %d\n", len(coverageData))
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

