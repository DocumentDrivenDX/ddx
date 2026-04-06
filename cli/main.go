package main

import (
	"fmt"
	"os"

	"github.com/DocumentDrivenDX/ddx/cmd"
	"github.com/DocumentDrivenDX/ddx/internal/git"
)

var version = "dev"
var commit = "unknown"
var date = "unknown"

func main() {
	// Set version info for cobra
	cmd.Version = version
	cmd.Commit = commit
	cmd.Date = date

	// Get working directory once at startup, then resolve to the git
	// repository root so that ddx always operates from the project root
	// regardless of which subdirectory the user invokes it from.
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting working directory: %v\n", err)
		os.Exit(1)
	}
	workingDir := git.FindProjectRoot(cwd)

	if err := cmd.Execute(workingDir); err != nil {
		// Check if it's an ExitError with a specific exit code
		if exitErr, ok := err.(*cmd.ExitError); ok {
			// Print error message only if it's not empty
			if exitErr.Message != "" {
				fmt.Fprintf(os.Stderr, "Error: %v\n", exitErr.Message)
			}
			os.Exit(exitErr.Code)
		}
		// For other errors, print them
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
