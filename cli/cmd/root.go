package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "coverport",
	Short: "Coverage collection tool for Konflux integration pipelines",
	Long: `coverport is a CLI tool for collecting Go coverage data from Kubernetes pods
running instrumented applications. It's designed for CI/CD integration, especially
Konflux pipelines.

Key features:
  • Image-based pod discovery: Find pods by container image reference
  • Multi-component support: Collect coverage from multiple components in one run
  • Organized output: Saves coverage data per component
  • OCI artifact support: Push coverage data to container registries`,
	Version: fmt.Sprintf("%s (commit: %s)", version, commit),
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().Bool("verbose", false, "Enable verbose output")
}

// exitWithError prints an error message and exits with status code 1
func exitWithError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "❌ Error: "+format+"\n", args...)
	os.Exit(1)
}

// printInfo prints an info message
func printInfo(format string, args ...interface{}) {
	fmt.Printf("ℹ️  "+format+"\n", args...)
}

// printSuccess prints a success message
func printSuccess(format string, args ...interface{}) {
	fmt.Printf("✅ "+format+"\n", args...)
}

// printWarning prints a warning message
func printWarning(format string, args ...interface{}) {
	fmt.Printf("⚠️  "+format+"\n", args...)
}

