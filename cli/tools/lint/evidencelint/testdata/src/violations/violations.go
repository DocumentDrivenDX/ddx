// Package violations holds one unannotated instance of patterns 1 and 3
// (the patterns whose target types are exported / visible from non-server
// packages). Patterns 2 and 4 live in the testdata server package
// because they require unexported type access / server-package scope.
package violations

import (
	"os"

	"agent"
)

// Pattern 1: assignment to RunOptions.Prompt with non-constant RHS.
func pattern1(opts *agent.RunOptions, dynamic string) {
	opts.Prompt = dynamic // want `evidencelint: assignment to RunOptions.Prompt`
}

// Pattern 1b: same sink reached through an embedded RunOptions field.
func pattern1Embedded(opts *agent.QuorumOptions, dynamic string) {
	opts.Prompt = dynamic // want `evidencelint: assignment to RunOptions.Prompt`
}

// Pattern 3: os.ReadFile result flowing into a *prompt* variable.
func pattern3(path string) ([]byte, error) {
	promptBytes, err := os.ReadFile(path) // want `evidencelint: os.ReadFile.*prompt-named variable`
	return promptBytes, err
}

// Constant-string assignment to RunOptions.Prompt is exempt (static
// fragments compiled into the binary). Must NOT trigger pattern 1.
func pattern1ConstantOK(opts *agent.RunOptions) {
	opts.Prompt = "static literal prompt fragment"
}
