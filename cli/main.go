package main

import (
	"os"

	"github.com/konflux-ci/coverport/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
