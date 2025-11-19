package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/konflux-ci/coverport/cli/internal/git"
	"github.com/konflux-ci/coverport/cli/internal/manifest"
	"github.com/konflux-ci/coverport/cli/internal/metadata"
	"github.com/konflux-ci/coverport/cli/internal/processor"
	"github.com/konflux-ci/coverport/cli/internal/upload"
)

var processCmd = &cobra.Command{
	Use:   "process",
	Short: "Process coverage data and upload to coverage services",
	Long: `Process coverage data by:
  1. Extracting coverage artifact from OCI registry (or using local directory)
  2. Extracting git metadata from container image using cosign
  3. Cloning the source repository at the specific commit
  4. Converting and processing coverage data with proper path mapping
  5. Uploading to Codecov (and optionally SonarQube)

This command replaces the complex bash scripts in Tekton pipelines with a single,
maintainable CLI command.`,
	Example: `  # Process all components from collected coverage (reads metadata.json)
  coverport process \
    --coverage-dir=./coverage-output \
    --codecov-token=$CODECOV_TOKEN

  # Process with custom flags
  coverport process \
    --coverage-dir=./coverage-output \
    --codecov-token=$CODECOV_TOKEN \
    --codecov-flags=e2e-tests,integration

  # Legacy: Process single component without manifest
  coverport process \
    --coverage-dir=./coverage-output/comp1/test1 \
    --image=quay.io/org/app@sha256:abc123 \
    --codecov-token=$CODECOV_TOKEN`,
	Run: runProcess,
}

var (
	// Input options
	artifactRef string
	coverageDir string
	imageRef    string

	// Workspace options
	workspaceDir  string
	keepWorkspace bool

	// Coverage processing options
	coverageFormat  string
	coverageFilters []string

	// Upload options
	uploadCoverage bool
	codecovToken   string
	codecovFlags   []string
	codecovName    string

	// Git options
	repoURL    string
	commitSHA  string
	skipClone  bool
	cloneDepth int
)

func init() {
	rootCmd.AddCommand(processCmd)

	// Input options
	processCmd.Flags().StringVar(&artifactRef, "artifact-ref", "", "OCI artifact reference containing coverage data")
	processCmd.Flags().StringVar(&coverageDir, "coverage-dir", "", "Local directory containing coverage data (alternative to --artifact-ref)")
	processCmd.Flags().StringVar(&imageRef, "image", "", "Container image reference to extract git metadata from")

	// Workspace options
	processCmd.Flags().StringVar(&workspaceDir, "workspace", "", "Workspace directory (default: temp directory)")
	processCmd.Flags().BoolVar(&keepWorkspace, "keep-workspace", false, "Keep workspace directory after processing")

	// Coverage processing options
	processCmd.Flags().StringVar(&coverageFormat, "format", "auto", "Coverage format: go, python, nyc, auto")
	processCmd.Flags().StringSliceVar(&coverageFilters, "filters", []string{"coverage_server.go", "*_test.go"}, "File patterns to exclude from coverage")

	// Upload options
	processCmd.Flags().BoolVar(&uploadCoverage, "upload", true, "Upload coverage to services (codecov, sonarqube)")
	processCmd.Flags().StringVar(&codecovToken, "codecov-token", "", "Codecov upload token (can also use CODECOV_TOKEN env var)")
	processCmd.Flags().StringSliceVar(&codecovFlags, "codecov-flags", []string{"e2e-tests"}, "Codecov flags")
	processCmd.Flags().StringVar(&codecovName, "codecov-name", "", "Codecov upload name")

	// Git options
	processCmd.Flags().StringVar(&repoURL, "repo-url", "", "Git repository URL (optional, extracted from image if not provided)")
	processCmd.Flags().StringVar(&commitSHA, "commit-sha", "", "Git commit SHA (optional, extracted from image if not provided)")
	processCmd.Flags().BoolVar(&skipClone, "skip-clone", false, "Skip cloning the repository (use existing workspace)")
	processCmd.Flags().IntVar(&cloneDepth, "clone-depth", 1, "Git clone depth (0 for full clone)")
}

// processFromManifest processes all components listed in the collection manifest
func processFromManifest(ctx context.Context, cmd *cobra.Command, verbose bool) {
	// Load the manifest
	collectionManifest, err := manifest.Load(coverageDir)
	if err != nil {
		exitWithError("Failed to load collection manifest: %v", err)
	}

	fmt.Println("🚀 coverport - Coverage Processing Tool (Batch Mode)")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Collection:    %s\n", collectionManifest.TestName)
	fmt.Printf("Components:    %d\n", len(collectionManifest.Components))
	fmt.Printf("Coverage Dir:  %s\n", coverageDir)
	fmt.Println(strings.Repeat("=", 60))

	if len(collectionManifest.Components) == 0 {
		exitWithError("No components found in manifest")
	}

	// Process each component
	successCount := 0
	failedComponents := []string{}

	for i, component := range collectionManifest.Components {
		fmt.Printf("\n[%d/%d] Processing component: %s\n", i+1, len(collectionManifest.Components), component.Name)
		fmt.Printf("  Image: %s\n", truncateImage(component.Image))
		fmt.Printf("  Coverage: %s\n", component.CoverageDir)

		// Setup workspace for this component
		componentWorkspace := filepath.Join(coverageDir, component.Name+"-workspace")
		if workspaceDir != "" {
			componentWorkspace = filepath.Join(workspaceDir, component.Name)
		}
		if err := os.MkdirAll(componentWorkspace, 0755); err != nil {
			printWarning("Failed to create workspace for %s: %v", component.Name, err)
			failedComponents = append(failedComponents, component.Name)
			continue
		}

		// Process this component
		componentCoverageDir := filepath.Join(coverageDir, component.CoverageDir)
		if err := processComponent(ctx, component, componentCoverageDir, componentWorkspace, verbose); err != nil {
			printWarning("Failed to process %s: %v", component.Name, err)
			failedComponents = append(failedComponents, component.Name)
		} else {
			successCount++
		}

		// Cleanup component workspace unless keeping
		if !keepWorkspace {
			os.RemoveAll(componentWorkspace)
		}
	}

	// Print summary
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("📊 Processing Summary")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total components:     %d\n", len(collectionManifest.Components))
	fmt.Printf("Successfully processed: %d\n", successCount)
	fmt.Printf("Failed:               %d\n", len(failedComponents))

	if len(failedComponents) > 0 {
		fmt.Printf("\nFailed components:\n")
		for _, name := range failedComponents {
			fmt.Printf("  - %s\n", name)
		}
	}

	if successCount == 0 {
		exitWithError("Failed to process any components")
	}

	fmt.Println("\n✅ Coverage processing complete!")
}

// processComponent processes a single component
func processComponent(ctx context.Context, component manifest.ComponentInfo, coverageDir, workspace string, verbose bool) error {
	// Extract git metadata from image
	gitMeta, err := extractGitMetadata(ctx, component.Image, verbose)
	if err != nil {
		return fmt.Errorf("extract git metadata: %w", err)
	}

	// Clone repository
	repoDir := filepath.Join(workspace, "repo")
	if !skipClone {
		if err := cloneRepository(ctx, gitMeta, repoDir, verbose); err != nil {
			return fmt.Errorf("clone repository: %w", err)
		}
	}

	// Process coverage
	coverageFile := filepath.Join(workspace, "coverage.out")
	if err := processCoverage(ctx, coverageDir, coverageFile, repoDir, verbose); err != nil {
		return fmt.Errorf("process coverage: %w", err)
	}

	// Upload to services
	if uploadCoverage {
		token := codecovToken
		if token == "" {
			token = os.Getenv("CODECOV_TOKEN")
		}

		if token != "" {
			if err := uploadToCodecov(ctx, token, coverageFile, gitMeta, verbose); err != nil {
				printWarning("Failed to upload to Codecov: %v", err)
			}
		} else if verbose {
			printInfo("Skipping Codecov upload (no token provided)")
		}
	}

	return nil
}

func runProcess(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	verbose, _ := cmd.Flags().GetBool("verbose")

	// Validate inputs
	if artifactRef == "" && coverageDir == "" {
		exitWithError("Either --artifact-ref or --coverage-dir must be specified")
	}
	if artifactRef != "" && coverageDir != "" {
		exitWithError("Cannot specify both --artifact-ref and --coverage-dir")
	}

	// Check if coverage directory has a manifest (new workflow)
	if coverageDir != "" && manifest.Exists(coverageDir) {
		// New workflow: Process all components from manifest
		processFromManifest(ctx, cmd, verbose)
		return
	}

	// Legacy workflow: Single component processing
	if imageRef == "" && (repoURL == "" || commitSHA == "") {
		exitWithError("Legacy mode requires --image, or both --repo-url and --commit-sha. For batch processing, ensure metadata.json exists in coverage directory.")
	}

	// Setup workspace
	workspace, err := setupWorkspace(workspaceDir, keepWorkspace)
	if err != nil {
		exitWithError("Failed to setup workspace: %v", err)
	}

	// Step 1: Get coverage data location first to validate workspace
	var rawCoverageDir string
	if artifactRef != "" {
		// Will be pulled into workspace, no conflict
		rawCoverageDir = ""
	} else {
		rawCoverageDir = coverageDir
	}

	// Safety check: prevent workspace from being deleted if it contains coverage data
	if rawCoverageDir != "" && !keepWorkspace {
		absWorkspace, _ := filepath.Abs(workspace)
		absCoverageDir, _ := filepath.Abs(rawCoverageDir)

		// Check if coverage dir is inside workspace or workspace is inside coverage dir
		if strings.HasPrefix(absCoverageDir, absWorkspace+string(filepath.Separator)) {
			exitWithError("Workspace '%s' contains coverage data '%s'. Use --keep-workspace or specify a different workspace directory.", workspace, rawCoverageDir)
		}
		if strings.HasPrefix(absWorkspace, absCoverageDir+string(filepath.Separator)) {
			exitWithError("Coverage directory '%s' contains workspace '%s'. Use --keep-workspace or specify a different workspace directory.", rawCoverageDir, workspace)
		}
	}

	if !keepWorkspace {
		defer cleanupWorkspace(workspace)
	}

	fmt.Println("🚀 coverport - Coverage Processing Tool")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Workspace:     %s\n", workspace)
	fmt.Println(strings.Repeat("=", 60))

	// Step 1: Get coverage data
	if artifactRef != "" {
		rawCoverageDir, err = pullCoverageArtifact(ctx, artifactRef, workspace, verbose)
		if err != nil {
			exitWithError("Failed to pull coverage artifact: %v", err)
		}
	} else {
		printInfo("Using local coverage directory: %s", rawCoverageDir)
	}

	// Step 2: Extract git metadata
	var gitMeta *metadata.GitMetadata
	if repoURL != "" && commitSHA != "" {
		// Use provided git information
		gitMeta = &metadata.GitMetadata{
			RepoURL:   repoURL,
			CommitSHA: commitSHA,
		}
		printInfo("Using provided git metadata")
	} else {
		// Extract from image
		gitMeta, err = extractGitMetadata(ctx, imageRef, verbose)
		if err != nil {
			exitWithError("Failed to extract git metadata: %v", err)
		}
	}

	// Step 3: Clone repository
	repoDir := filepath.Join(workspace, "repo")
	if !skipClone {
		if err := cloneRepository(ctx, gitMeta, repoDir, verbose); err != nil {
			exitWithError("Failed to clone repository: %v", err)
		}
	} else {
		printInfo("Skipping repository clone (using existing)")
		if _, err := os.Stat(repoDir); err != nil {
			exitWithError("Repository directory not found: %s", repoDir)
		}
	}

	// Step 4: Process coverage
	coverageFile := filepath.Join(workspace, "coverage.out")
	if err := processCoverage(ctx, rawCoverageDir, coverageFile, repoDir, verbose); err != nil {
		exitWithError("Failed to process coverage: %v", err)
	}

	// Step 5: Upload to services
	if uploadCoverage {
		// Get codecov token from flag or environment
		token := codecovToken
		if token == "" {
			token = os.Getenv("CODECOV_TOKEN")
		}

		if token == "" {
			printWarning("CODECOV_TOKEN not provided, skipping upload")
			printInfo("Set --codecov-token or CODECOV_TOKEN environment variable to enable upload")
		} else {
			if err := uploadToCodecov(ctx, token, coverageFile, gitMeta, verbose); err != nil {
				printWarning("Failed to upload to Codecov: %v", err)
			}
		}

		// TODO: Add SonarQube upload support
	}

	fmt.Println("\n✅ Coverage processing complete!")
	if keepWorkspace {
		fmt.Printf("📁 Workspace saved at: %s\n", workspace)
	}
}

// setupWorkspace creates or uses the specified workspace directory
func setupWorkspace(dir string, keep bool) (string, error) {
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
		return dir, nil
	}

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "coverport-process-*")
	if err != nil {
		return "", err
	}

	return tempDir, nil
}

// cleanupWorkspace removes the workspace directory
func cleanupWorkspace(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		printWarning("Failed to cleanup workspace: %v", err)
	}
}

// pullCoverageArtifact pulls coverage artifact from OCI registry using oras
func pullCoverageArtifact(ctx context.Context, artifactRef, workspace string, verbose bool) (string, error) {
	fmt.Printf("📦 Pulling coverage artifact: %s\n", artifactRef)

	// Check if oras is available
	orasPath, err := exec.LookPath("oras")
	if err != nil {
		return "", fmt.Errorf("oras not found in PATH (required for pulling OCI artifacts): %w", err)
	}

	// Create coverage directory
	coverageDir := filepath.Join(workspace, "coverage-raw")
	if err := os.MkdirAll(coverageDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create coverage directory: %w", err)
	}

	// Pull artifact
	cmd := exec.CommandContext(ctx, orasPath, "pull", artifactRef)
	cmd.Dir = coverageDir
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("oras pull failed: %w", err)
	}

	// Verify metadata.json exists
	metadataPath := filepath.Join(coverageDir, "metadata.json")
	if _, err := os.Stat(metadataPath); err != nil {
		return "", fmt.Errorf("metadata.json not found in artifact")
	}

	if verbose {
		fmt.Println("📄 Artifact contents:")
		entries, _ := os.ReadDir(coverageDir)
		for _, entry := range entries {
			fmt.Printf("   - %s\n", entry.Name())
		}
	}

	printSuccess("Coverage artifact pulled successfully")
	return coverageDir, nil
}

// extractGitMetadata extracts git metadata from container image
func extractGitMetadata(ctx context.Context, image string, verbose bool) (*metadata.GitMetadata, error) {
	extractor, err := metadata.NewImageMetadataExtractor()
	if err != nil {
		return nil, err
	}

	return extractor.ExtractGitMetadata(ctx, image)
}

// cloneRepository clones the git repository
func cloneRepository(ctx context.Context, gitMeta *metadata.GitMetadata, targetDir string, verbose bool) error {
	cloner, err := git.NewRepositoryCloner()
	if err != nil {
		return err
	}

	opts := git.CloneOptions{
		RepoURL:   gitMeta.RepoURL,
		CommitSHA: gitMeta.CommitSHA,
		Branch:    gitMeta.Branch,
		TargetDir: targetDir,
		Depth:     cloneDepth,
	}

	return cloner.Clone(ctx, opts)
}

// processCoverage processes the coverage data
func processCoverage(ctx context.Context, inputDir, outputFile, repoRoot string, verbose bool) error {
	// Detect or use specified format
	var format processor.CoverageFormat
	switch coverageFormat {
	case "go":
		format = processor.FormatGo
	case "python":
		format = processor.FormatPython
	case "nyc":
		format = processor.FormatNYC
	case "auto":
		format = processor.FormatAuto
	default:
		return fmt.Errorf("unsupported coverage format: %s", coverageFormat)
	}

	proc := processor.NewCoverageProcessor(format)

	opts := processor.ProcessOptions{
		Format:     format,
		InputDir:   inputDir,
		OutputFile: outputFile,
		RepoRoot:   repoRoot,
		Filters:    coverageFilters,
	}

	return proc.Process(ctx, opts)
}

// uploadToCodecov uploads coverage to Codecov
func uploadToCodecov(ctx context.Context, token, coverageFile string, gitMeta *metadata.GitMetadata, verbose bool) error {
	uploader, err := upload.NewCodecovUploader(token)
	if err != nil {
		return err
	}
	defer uploader.Cleanup()

	// Get the workspace directory and repository directory
	workspace := filepath.Dir(coverageFile) // workspace is parent of coverage.out
	repoRoot := filepath.Join(workspace, "repo")

	// Use filtered coverage if it exists, otherwise use the regular coverage file
	sourceFile := coverageFile
	filteredFile := strings.TrimSuffix(coverageFile, ".out") + "_filtered.out"
	if _, err := os.Stat(filteredFile); err == nil {
		sourceFile = filteredFile
		fmt.Printf("   📄 Using filtered coverage file\n")
	}

	// Copy coverage file to repository directory for upload
	// This ensures Codecov only sees files that exist in the repository
	repoCoverageFile := filepath.Join(repoRoot, "coverage.out")
	if err := copyCoverageToRepo(sourceFile, repoCoverageFile); err != nil {
		return fmt.Errorf("failed to copy coverage to repo: %w", err)
	}

	// Extract repository slug and git service from URL
	repoSlug := extractRepoSlug(gitMeta.RepoURL)
	gitService := extractGitService(gitMeta.RepoURL)

	opts := upload.CodecovOptions{
		Token:        token,
		CommitSHA:    gitMeta.CommitSHA,
		Branch:       gitMeta.Branch,
		RepoRoot:     repoRoot,
		RepoSlug:     repoSlug,
		GitService:   gitService,
		CoverageFile: repoCoverageFile,
		Flags:        codecovFlags,
		Name:         codecovName,
		Verbose:      verbose,
	}

	return uploader.Upload(ctx, opts)
}

// copyCoverageToRepo copies the coverage file to the repository directory
func copyCoverageToRepo(srcPath, dstPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read source coverage file: %w", err)
	}

	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		return fmt.Errorf("write coverage to repo: %w", err)
	}

	fmt.Printf("   📄 Copied coverage file to repository: %s\n", dstPath)
	return nil
}

// extractRepoSlug extracts the repository slug (owner/repo) from a Git URL
func extractRepoSlug(gitURL string) string {
	// Remove .git suffix if present
	gitURL = strings.TrimSuffix(gitURL, ".git")

	// Handle different URL formats:
	// - https://github.com/owner/repo
	// - git@github.com:owner/repo
	// - ssh://git@github.com/owner/repo

	// Remove protocol prefixes
	gitURL = strings.TrimPrefix(gitURL, "https://")
	gitURL = strings.TrimPrefix(gitURL, "http://")
	gitURL = strings.TrimPrefix(gitURL, "ssh://")
	gitURL = strings.TrimPrefix(gitURL, "git@")

	// Replace : with / for SSH-style URLs
	gitURL = strings.Replace(gitURL, ":", "/", 1)

	// Split by / and get the last two components
	parts := strings.Split(gitURL, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}

	return ""
}

// extractGitService detects the git service from a Git URL
func extractGitService(gitURL string) string {
	gitURL = strings.ToLower(gitURL)

	if strings.Contains(gitURL, "github.com") {
		return "github"
	} else if strings.Contains(gitURL, "gitlab.com") {
		return "gitlab"
	} else if strings.Contains(gitURL, "bitbucket.org") {
		return "bitbucket"
	} else if strings.Contains(gitURL, "github") && !strings.Contains(gitURL, "github.com") {
		return "github_enterprise"
	} else if strings.Contains(gitURL, "gitlab") && !strings.Contains(gitURL, "gitlab.com") {
		return "gitlab_enterprise"
	} else if strings.Contains(gitURL, "bitbucket") && !strings.Contains(gitURL, "bitbucket.org") {
		return "bitbucket_server"
	}

	// Default to github if we can't determine
	return "github"
}

// Helper to read metadata from artifact
func readArtifactMetadata(artifactDir string) (map[string]interface{}, error) {
	metadataPath := filepath.Join(artifactDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}
