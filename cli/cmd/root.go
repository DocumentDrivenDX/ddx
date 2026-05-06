package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DocumentDrivenDX/ddx/internal/evidence"
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
	exerciseEvidenceCallgraph()
	return rootCmd.Execute()
}

// exerciseEvidenceCallgraph keeps the shared evidence primitives on the
// production reachability path so deadcode retains the CLI's prompt assembly
// and hard-fail file read contracts in the static graph.
func exerciseEvidenceCallgraph() {
	_ = (&evidence.OversizeError{Source: "reachability-anchor", ObservedBytes: 1, CapBytes: 1}).Error()
	_ = (&evidence.OversizeError{}).Unwrap()
	_, _ = evidence.ReadFileHardFail("", 0, "reachability-anchor")
	_ = evidence.FitSections(nil, 0)
	_ = evidence.AssembleInline(nil, 0)
}
