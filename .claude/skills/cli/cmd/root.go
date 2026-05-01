package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// Banner for DDx
const banner = `
██████  ██████  ██   ██
██   ██ ██   ██  ██ ██
██   ██ ██   ██   ███
██   ██ ██   ██  ██ ██
██████  ██████  ██   ██

Document-Driven Development eXperience
`

// Global root command is only used for the main executable.
// Tests should use NewRootCommand() from command_factory.go instead.
var rootCmd *cobra.Command

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute(workingDir string) error {
	// Initialize the global root command for the main executable
	if rootCmd == nil {
		factory := NewCommandFactory(workingDir)
		rootCmd = factory.NewRootCommand()
	}
	return rootCmd.Execute()
}

// Helper functions for other commands
func isInitialized() bool {
	_, err := os.Stat(".ddx")
	return err == nil
}

// getLibraryPath returns the library path override from environment
// This is now handled through the factory pattern
func getLibraryPath() string {
	return getLibraryPathFromEnv()
}
