package upload

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// CodecovUploader handles uploading coverage to Codecov
type CodecovUploader struct {
	token        string
	codecovPath  string
	downloadedCLI bool
}

// CodecovOptions contains options for uploading to Codecov
type CodecovOptions struct {
	Token       string
	CommitSHA   string
	Branch      string
	RepoRoot    string
	RepoSlug    string // Repository slug (e.g., "owner/repo")
	GitService  string // Git service: github, gitlab, bitbucket, etc.
	CoverageFile string
	Flags       []string
	Name        string
	Verbose     bool
}

// NewCodecovUploader creates a new Codecov uploader
func NewCodecovUploader(token string) (*CodecovUploader, error) {
	if token == "" {
		return nil, fmt.Errorf("codecov token is required")
	}

	// Check if codecov CLI is already available
	codecovPath, err := exec.LookPath("codecov")
	if err != nil {
		// Not found, we'll need to download it
		return &CodecovUploader{
			token:         token,
			downloadedCLI: false,
		}, nil
	}

	return &CodecovUploader{
		token:       token,
		codecovPath: codecovPath,
	}, nil
}

// ensureCodecovCLI ensures the codecov CLI is available
func (u *CodecovUploader) ensureCodecovCLI(ctx context.Context) error {
	if u.codecovPath != "" {
		return nil // Already have it
	}

	fmt.Println("📥 Downloading Codecov CLI...")

	// Determine download URL based on OS
	var downloadURL string
	switch runtime.GOOS {
	case "linux":
		downloadURL = "https://cli.codecov.io/latest/linux/codecov"
	case "darwin":
		downloadURL = "https://cli.codecov.io/latest/macos/codecov"
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	// Download to temp directory
	tempDir := os.TempDir()
	codecovPath := filepath.Join(tempDir, "codecov")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download codecov CLI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download codecov CLI: status %d", resp.StatusCode)
	}

	// Write to file
	out, err := os.OpenFile(codecovPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create codecov file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to save codecov CLI: %w", err)
	}

	u.codecovPath = codecovPath
	u.downloadedCLI = true
	
	fmt.Printf("✅ Codecov CLI downloaded to: %s\n", codecovPath)
	return nil
}

// Upload uploads coverage data to Codecov
func (u *CodecovUploader) Upload(ctx context.Context, opts CodecovOptions) error {
	// Ensure codecov CLI is available
	if err := u.ensureCodecovCLI(ctx); err != nil {
		return err
	}

	fmt.Println("📤 Uploading coverage to Codecov...")
	fmt.Printf("   File: %s\n", opts.CoverageFile)
	fmt.Printf("   Commit: %s\n", opts.CommitSHA)
	if opts.Branch != "" {
		fmt.Printf("   Branch: %s\n", opts.Branch)
	}

	// Convert coverage file to absolute path (needed when running from repo root)
	absCoverageFile, err := filepath.Abs(opts.CoverageFile)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for coverage file: %w", err)
	}

	// Verify coverage file exists
	if _, err := os.Stat(absCoverageFile); err != nil {
		return fmt.Errorf("coverage file not found: %w", err)
	}

	// Build codecov command
	args := []string{
		"upload-coverage",
		"-t", opts.Token,
		"-f", absCoverageFile,
		"--sha", opts.CommitSHA,
		"--disable-search", // Don't search for other coverage files
	}

	// Add repository slug if provided
	if opts.RepoSlug != "" {
		args = append(args, "--slug", opts.RepoSlug)
	}

	// Add git service if provided
	if opts.GitService != "" {
		args = append(args, "--git-service", opts.GitService)
	}

	// Add optional parameters
	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}

	for _, flag := range opts.Flags {
		args = append(args, "--flag", flag)
	}

	if opts.Name != "" {
		args = append(args, "--name", opts.Name)
	}

	// Note: Codecov CLI doesn't support --verbose flag
	// if opts.Verbose {
	// 	args = append(args, "--verbose")
	// }

	// Execute upload
	cmd := exec.CommandContext(ctx, u.codecovPath, args...)
	cmd.Dir = opts.RepoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codecov upload failed: %w", err)
	}

	fmt.Println("✅ Coverage uploaded to Codecov successfully!")
	return nil
}

// Cleanup removes the downloaded codecov CLI if it was downloaded
func (u *CodecovUploader) Cleanup() {
	if u.downloadedCLI && u.codecovPath != "" {
		os.Remove(u.codecovPath)
	}
}

