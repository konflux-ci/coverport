package processor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CoverageFormat represents the type of coverage data
type CoverageFormat string

const (
	FormatGo     CoverageFormat = "go"
	FormatPython CoverageFormat = "python"
	FormatNYC    CoverageFormat = "nyc"
	FormatAuto   CoverageFormat = "auto"
)

// CoverageProcessor handles processing coverage data
type CoverageProcessor struct {
	format CoverageFormat
}

// ProcessOptions contains options for processing coverage
type ProcessOptions struct {
	Format           CoverageFormat
	InputDir         string  // Directory containing binary coverage (Go) or raw coverage
	OutputFile       string  // Output coverage file path
	RepoRoot         string  // Repository root for path mapping
	Filters          []string // File patterns to exclude
}

// NewCoverageProcessor creates a new coverage processor
func NewCoverageProcessor(format CoverageFormat) *CoverageProcessor {
	return &CoverageProcessor{
		format: format,
	}
}

// DetectFormat detects the coverage format from the input directory
func DetectFormat(inputDir string) (CoverageFormat, error) {
	// Check for Go coverage files (covmeta.* and covcounters.*)
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return "", fmt.Errorf("failed to read input directory: %w", err)
	}

	hasGoCoverage := false
	hasPythonCoverage := false
	hasNYCCoverage := false

	for _, entry := range entries {
		name := entry.Name()
		
		// Check for Go coverage
		if strings.HasPrefix(name, "covmeta.") || strings.HasPrefix(name, "covcounters.") {
			hasGoCoverage = true
		}
		
		// Check for Python coverage
		if name == ".coverage" || name == "coverage.xml" {
			hasPythonCoverage = true
		}
		
		// Check for NYC coverage
		if name == "coverage-final.json" || name == ".nyc_output" {
			hasNYCCoverage = true
		}
	}

	if hasGoCoverage {
		return FormatGo, nil
	}
	if hasPythonCoverage {
		return FormatPython, nil
	}
	if hasNYCCoverage {
		return FormatNYC, nil
	}

	return "", fmt.Errorf("unable to detect coverage format from directory: %s", inputDir)
}

// Process processes the coverage data and converts it to a standard format
func (p *CoverageProcessor) Process(ctx context.Context, opts ProcessOptions) error {
	format := p.format
	if format == FormatAuto {
		detectedFormat, err := DetectFormat(opts.InputDir)
		if err != nil {
			return err
		}
		format = detectedFormat
		fmt.Printf("🔍 Detected coverage format: %s\n", format)
	}

	switch format {
	case FormatGo:
		return p.processGoCoverage(ctx, opts)
	case FormatPython:
		return p.processPythonCoverage(ctx, opts)
	case FormatNYC:
		return p.processNYCCoverage(ctx, opts)
	default:
		return fmt.Errorf("unsupported coverage format: %s", format)
	}
}

// processGoCoverage processes Go binary coverage data
func (p *CoverageProcessor) processGoCoverage(ctx context.Context, opts ProcessOptions) error {
	fmt.Println("🔄 Processing Go coverage data...")
	fmt.Printf("   Input: %s\n", opts.InputDir)
	fmt.Printf("   Output: %s\n", opts.OutputFile)

	// Check for Go toolchain
	goPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go toolchain not found (required for processing Go coverage): %w", err)
	}

	// Create output directory
	if err := os.MkdirAll(filepath.Dir(opts.OutputFile), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Convert binary coverage to text format
	fmt.Println("   Converting binary coverage to text format...")
	cmd := exec.CommandContext(ctx, goPath, "tool", "covdata", "textfmt", 
		"-i="+opts.InputDir, 
		"-o="+opts.OutputFile)
	
	if opts.RepoRoot != "" {
		cmd.Dir = opts.RepoRoot
	}
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to convert coverage: %w\nOutput: %s", err, string(output))
	}

	// Verify output file was created
	if _, err := os.Stat(opts.OutputFile); err != nil {
		return fmt.Errorf("coverage file was not created: %w", err)
	}

	// Apply filters if specified
	if len(opts.Filters) > 0 {
		if err := p.applyFilters(opts.OutputFile, opts.Filters); err != nil {
			fmt.Printf("⚠️  Failed to apply filters: %v\n", err)
		} else {
			fmt.Printf("   Applied filters: %v\n", opts.Filters)
		}
	}

	// Show coverage summary
	if err := p.showGoCoverageSummary(ctx, goPath, opts.OutputFile); err != nil {
		fmt.Printf("⚠️  Failed to show coverage summary: %v\n", err)
	}

	fmt.Println("✅ Go coverage processed successfully!")
	return nil
}

// processPythonCoverage processes Python coverage data (future implementation)
func (p *CoverageProcessor) processPythonCoverage(ctx context.Context, opts ProcessOptions) error {
	fmt.Println("🔄 Processing Python coverage data...")
	
	// TODO: Implement Python coverage processing
	// This would use the Python coverage package to convert .coverage to XML/JSON
	
	return fmt.Errorf("Python coverage processing not yet implemented")
}

// processNYCCoverage processes NYC (Node.js) coverage data (future implementation)
func (p *CoverageProcessor) processNYCCoverage(ctx context.Context, opts ProcessOptions) error {
	fmt.Println("🔄 Processing NYC coverage data...")
	
	// TODO: Implement NYC coverage processing
	// This would use nyc or istanbul to convert coverage-final.json to lcov
	
	return fmt.Errorf("NYC coverage processing not yet implemented")
}

// applyFilters removes coverage data for files matching the filter patterns
func (p *CoverageProcessor) applyFilters(coverageFile string, filters []string) error {
	// Read the coverage file
	data, err := os.ReadFile(coverageFile)
	if err != nil {
		return fmt.Errorf("failed to read coverage file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	var filteredLines []string

	for _, line := range lines {
		// Coverage lines start with "mode:" or contain a colon followed by line numbers
		if strings.HasPrefix(line, "mode:") {
			filteredLines = append(filteredLines, line)
			continue
		}

		// Check if line should be filtered
		shouldFilter := false
		for _, filter := range filters {
			if strings.Contains(line, filter) {
				shouldFilter = true
				break
			}
		}

		if !shouldFilter {
			filteredLines = append(filteredLines, line)
		}
	}

	// Write filtered coverage
	filtered := strings.Join(filteredLines, "\n")
	filteredFile := strings.TrimSuffix(coverageFile, ".out") + "_filtered.out"
	
	if err := os.WriteFile(filteredFile, []byte(filtered), 0644); err != nil {
		return fmt.Errorf("failed to write filtered coverage: %w", err)
	}

	fmt.Printf("   Filtered coverage saved to: %s\n", filteredFile)
	return nil
}

// showGoCoverageSummary displays a summary of the coverage
func (p *CoverageProcessor) showGoCoverageSummary(ctx context.Context, goPath, coverageFile string) error {
	cmd := exec.CommandContext(ctx, goPath, "tool", "cover", "-func="+coverageFile)
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	lines := strings.Split(string(output), "\n")
	
	// Find the total line
	for _, line := range lines {
		if strings.HasPrefix(line, "total:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				fmt.Printf("   📊 Total coverage: %s\n", parts[len(parts)-1])
			}
			break
		}
	}

	return nil
}

