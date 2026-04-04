package agent

import "time"

// Harness defines a known agent harness.
type Harness struct {
	Name         string   // e.g. "codex", "claude", "gemini"
	Binary       string   // binary name to exec
	Args         []string // base arguments for exec mode
	PromptMode   string   // "arg" (final arg), "stdin" (pipe)
	ModelFlag    string   // flag for model override (e.g. "-m", "--model"), empty if unsupported
	WorkDirFlag  string   // flag for working directory (e.g. "-C", "--cwd"), empty if unsupported
	EffortFlag   string   // flag for effort/reasoning control, empty if unsupported
	EffortFormat string   // format string for effort value (e.g. "reasoning.effort=%s"), empty = use value directly
	TokenPattern string   // regex to extract token count from output, must have one capture group
}

// Config holds agent service configuration.
type Config struct {
	Harness       string            `yaml:"harness"`        // default harness name
	Model         string            `yaml:"model"`          // global model override
	Models        map[string]string `yaml:"models"`         // per-harness model overrides
	TimeoutMS     int               `yaml:"timeout_ms"`     // default timeout in ms
	SessionLogDir string            `yaml:"session_log_dir"` // log directory
}

// RunOptions holds options for a single agent invocation.
type RunOptions struct {
	Harness    string
	Prompt     string // prompt text (or path to file)
	PromptFile string // explicit file path
	Model   string
	Effort  string
	Timeout time.Duration
	WorkDir    string
}

// QuorumOptions extends RunOptions for multi-agent consensus.
type QuorumOptions struct {
	RunOptions
	Harnesses []string // multiple harnesses to invoke
	Strategy  string   // any, majority, unanimous, or numeric
	Threshold int      // numeric threshold (when Strategy is "")
}

// Result holds the output of an agent invocation.
type Result struct {
	Harness  string        `json:"harness"`
	Model    string        `json:"model,omitempty"`
	ExitCode int           `json:"exit_code"`
	Output   string        `json:"output"`
	Tokens   int           `json:"tokens,omitempty"`
	DurationMS int `json:"duration_ms"`
	Error    string        `json:"error,omitempty"`
}

// SessionEntry is written to the session log.
type SessionEntry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Harness   string    `json:"harness"`
	Model     string    `json:"model,omitempty"`
	PromptLen int       `json:"prompt_len"`
	Tokens    int       `json:"tokens,omitempty"`
	Duration  int       `json:"duration_ms"`
	ExitCode  int       `json:"exit_code"`
	Error     string    `json:"error,omitempty"`
}

// HarnessStatus reports availability of a harness.
type HarnessStatus struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Binary    string `json:"binary"`
	Path      string `json:"path,omitempty"` // resolved binary path
	Error     string `json:"error,omitempty"`
}

// Default configuration values.
const (
	DefaultHarness   = "codex"
	DefaultTimeoutMS = 300000 // 5 minutes
	DefaultLogDir    = ".ddx/agent-logs"
)
