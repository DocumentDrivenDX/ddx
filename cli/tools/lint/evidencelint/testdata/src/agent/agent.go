// Package agent provides a stub RunOptions type for analyzer tests.
package agent

type RunOptions struct {
	Prompt     string
	PromptFile string
}

type QuorumOptions struct {
	RunOptions
	Harnesses []string
}
