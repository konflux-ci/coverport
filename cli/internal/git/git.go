package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// RepositoryCloner handles cloning git repositories
type RepositoryCloner struct {
	gitPath string
}

// NewRepositoryCloner creates a new repository cloner
func NewRepositoryCloner() (*RepositoryCloner, error) {
	// Check if git is available
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git not found in PATH: %w", err)
	}

	return &RepositoryCloner{
		gitPath: gitPath,
	}, nil
}

// CloneOptions contains options for cloning a repository
type CloneOptions struct {
	RepoURL   string
	CommitSHA string
	Branch    string
	TargetDir string
	Depth     int // Shallow clone depth (0 = full clone)
}

// Clone clones a git repository at a specific commit
func (c *RepositoryCloner) Clone(ctx context.Context, opts CloneOptions) error {
	fmt.Printf("📦 Cloning repository: %s\n", opts.RepoURL)
	fmt.Printf("   Commit: %s\n", opts.CommitSHA)
	fmt.Printf("   Target: %s\n", opts.TargetDir)

	// Create target directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(opts.TargetDir), 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Clone arguments
	args := []string{"clone"}
	
	// Add shallow clone if depth is specified
	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}
	
	// Add branch if specified
	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}
	
	args = append(args, opts.RepoURL, opts.TargetDir)

	// Execute clone
	cmd := exec.CommandContext(ctx, c.gitPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// Checkout specific commit if different from HEAD
	if opts.CommitSHA != "" {
		fmt.Printf("📍 Checking out commit: %s\n", opts.CommitSHA)
		
		// First, we might need to fetch if this is a shallow clone
		if opts.Depth > 0 {
			fmt.Println("   Fetching commit (shallow clone)...")
			fetchCmd := exec.CommandContext(ctx, c.gitPath, "-C", opts.TargetDir, "fetch", "--depth=1", "origin", opts.CommitSHA)
			fetchCmd.Stdout = os.Stdout
			fetchCmd.Stderr = os.Stderr
			if err := fetchCmd.Run(); err != nil {
				// If fetch fails, try without depth
				fmt.Println("   Retrying fetch without depth limit...")
				fetchCmd = exec.CommandContext(ctx, c.gitPath, "-C", opts.TargetDir, "fetch", "origin", opts.CommitSHA)
				fetchCmd.Stdout = os.Stdout
				fetchCmd.Stderr = os.Stderr
				if err := fetchCmd.Run(); err != nil {
					return fmt.Errorf("failed to fetch commit: %w", err)
				}
			}
		}
		
		checkoutCmd := exec.CommandContext(ctx, c.gitPath, "-C", opts.TargetDir, "checkout", opts.CommitSHA)
		checkoutCmd.Stdout = os.Stdout
		checkoutCmd.Stderr = os.Stderr
		
		if err := checkoutCmd.Run(); err != nil {
			return fmt.Errorf("failed to checkout commit: %w", err)
		}
	}

	fmt.Println("✅ Repository cloned successfully")
	
	// Show some info about the cloned repo
	if err := c.ShowInfo(ctx, opts.TargetDir); err != nil {
		fmt.Printf("⚠️  Failed to show repo info: %v\n", err)
	}

	return nil
}

// ShowInfo displays information about the cloned repository
func (c *RepositoryCloner) ShowInfo(ctx context.Context, repoDir string) error {
	// Get current commit
	cmd := exec.CommandContext(ctx, c.gitPath, "-C", repoDir, "log", "-1", "--oneline")
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	
	fmt.Printf("   Current commit: %s", string(output))
	
	// Check for go.mod
	goModPath := filepath.Join(repoDir, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		fmt.Println("   Language: Go (go.mod found)")
	}
	
	// Check for package.json
	packageJSONPath := filepath.Join(repoDir, "package.json")
	if _, err := os.Stat(packageJSONPath); err == nil {
		fmt.Println("   Language: Node.js (package.json found)")
	}
	
	// Check for requirements.txt or setup.py
	requirementsPath := filepath.Join(repoDir, "requirements.txt")
	setupPyPath := filepath.Join(repoDir, "setup.py")
	if _, err := os.Stat(requirementsPath); err == nil {
		fmt.Println("   Language: Python (requirements.txt found)")
	} else if _, err := os.Stat(setupPyPath); err == nil {
		fmt.Println("   Language: Python (setup.py found)")
	}
	
	return nil
}

