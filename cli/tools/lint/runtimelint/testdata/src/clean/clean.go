// Package clean is the analyzer-test stub for cli/internal/agent
// after Stage 4 cleanup completes: only *Runtime structs with allowed
// fields exist; no legacy *Options types remain. Running runtimelint
// against this package must produce zero diagnostics. Mirrors the AC
// "Running runtimelint against the post-cleanup tree exits 0".
package clean

// AgentRunRuntime: shape of the post-cleanup *Runtime per TD-024.
// All fields are runtime intent, plumbing, or ephemeral state — no
// durable knobs.
type AgentRunRuntime struct {
	Prompt                string
	PromptFile            string
	PromptSource          string
	Correlation           map[string]string
	WorkDir               string
	EstimatedPromptTokens int
	RequiresTools         bool
	SessionLogDirOverride string
	NoReview              bool
	PollInterval          int
}

// Result is a non-Runtime status type — fields with names from the
// forbidden list are legitimate observable outputs and must not be
// flagged because the type name does not end in "Runtime".
type Result struct {
	Harness  string
	Model    string
	Provider string
	ExitCode int
}
