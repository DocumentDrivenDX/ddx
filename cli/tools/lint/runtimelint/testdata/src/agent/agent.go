// Package agent is the analyzer-test stub for cli/internal/agent.
// It declares the legacy *Options types (so they exist as named
// types with package path matching the agentPkgSuffixes scoping
// rule) and a non-Runtime struct that must NOT be scanned.
package agent

// Legacy options types — pattern 2 (composite literal anywhere) and
// pattern 3 (param in agent pkg) reference these.

type RunOptions struct {
	Prompt string
}

type ExecuteBeadLoopOptions struct {
	WorkerID string
}

type ExecuteBeadOptions struct {
	FromRev string
}

type CompareOptions struct {
	RunOptions
}

type QuorumOptions struct {
	RunOptions
}

// Result is a non-Runtime struct in the agent pkg. Its fields use
// names that are on the forbidden list (Harness, Model, Provider).
// The analyzer must NOT flag it because the type name does not end
// in "Runtime". This locks the §Lint-rule scope-discipline guard.
type Result struct {
	Harness  string
	Model    string
	Provider string
	ExitCode int
}

// AllowedRuntime is a *Runtime struct whose fields are all on the
// allow list (NoReview, PollInterval, SessionLogDirOverride). Must
// not produce diagnostics.
type AllowedRuntime struct {
	NoReview              bool
	PollInterval          int
	SessionLogDirOverride string
	WorkerID              string // not on the closed list — allowed
}
