package agent

import "time"

// Harness defines a known agent harness.
type Harness struct {
	Name            string              // e.g. "codex", "claude", "gemini"
	Binary          string              // binary name to exec
	Args            []string            // deprecated: use BaseArgs; kept for compatibility
	BaseArgs        []string            // args always included regardless of permission level
	PermissionArgs  map[string][]string // extra args keyed by permission level: "safe", "supervised", "unrestricted"
	PromptMode      string              // "arg" (final arg), "stdin" (pipe)
	DefaultModel    string              // built-in model choice when no config override exists
	Models          []string            // known valid models for this harness
	ReasoningLevels []string            // supported reasoning levels in preference order
	ModelFlag       string              // flag for model override (e.g. "-m", "--model"), empty if unsupported
	WorkDirFlag     string              // flag for working directory (e.g. "-C", "--cwd"), empty if unsupported
	EffortFlag      string              // flag for effort/reasoning control, empty if unsupported
	EffortFormat    string              // format string for effort value (e.g. "reasoning.effort=%s"), empty = use value directly
	TokenPattern    string              // regex to extract token count from output, must have one capture group
	Surface         string              // catalog surface identifier: "codex", "claude", "embedded-openai", "embedded-anthropic"
	CostClass       string              // local, cheap, medium, expensive
	IsLocal         bool                // true for embedded/local harnesses (no cloud cost)
	ExactPinSupport bool                // true if harness can accept an exact concrete model pin
	QuotaCommand    string              // CLI args for non-interactive quota introspection (e.g. "usage", "status"); empty = skip probe. Must NOT be an interactive slash command.
}

// Config holds agent service configuration.
type Config struct {
	Profile         string              `yaml:"profile"`          // default routing intent: cheap, fast, smart
	Harness         string              `yaml:"harness"`          // optional forced harness override
	Model           string              `yaml:"model"`            // optional default model ref or exact pin
	Models          map[string]string   `yaml:"models"`           // per-harness model overrides
	ReasoningLevels map[string][]string `yaml:"reasoning_levels"` // per-harness reasoning-level options
	TimeoutMS       int                 `yaml:"timeout_ms"`       // default timeout in ms
	SessionLogDir   string              `yaml:"session_log_dir"`  // log directory
	Permissions     string              `yaml:"permissions"`      // permission level: safe, supervised, unrestricted
}

// RouteFlags holds raw CLI flag values before normalization into a RouteRequest.
// These come directly from parsed command-line arguments.
type RouteFlags struct {
	Profile     string // --profile: cheap, fast, smart
	Model       string // --model: logical ref or exact pin
	Harness     string // --harness: forced harness override
	Effort      string // --effort: low, medium, high
	Permissions string // --permissions: safe, supervised, unrestricted
}

// RunOptions holds options for a single agent invocation.
type RunOptions struct {
	Harness      string
	Prompt       string // prompt text (or path to file)
	PromptFile   string // explicit file path
	PromptSource string
	Correlation  map[string]string
	Model        string
	Effort       string
	Timeout      time.Duration
	WorkDir      string
	Permissions  string // permission level override: safe, supervised, unrestricted
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
	Harness         string          `json:"harness"`
	Model           string          `json:"model,omitempty"`
	ExitCode        int             `json:"exit_code"`
	Output          string          `json:"output"`
	CondensedOutput string          `json:"condensed_output,omitempty"`
	Stderr          string          `json:"stderr,omitempty"`
	Tokens          int             `json:"tokens,omitempty"`
	InputTokens     int             `json:"input_tokens,omitempty"`
	OutputTokens    int             `json:"output_tokens,omitempty"`
	CostUSD         float64         `json:"cost_usd,omitempty"`
	DurationMS      int             `json:"duration_ms"`
	Error           string          `json:"error,omitempty"`
	ToolCalls       []ToolCallEntry `json:"tool_calls,omitempty"`       // populated by agent, nil for subprocess
	AgentSessionID  string          `json:"agent_session_id,omitempty"` // agent session ID for event log cross-reference
}

// SessionEntry is written to the session log.
type SessionEntry struct {
	ID              string            `json:"id"`
	Timestamp       time.Time         `json:"timestamp"`
	Harness         string            `json:"harness"`
	Surface         string            `json:"surface,omitempty"`
	CanonicalTarget string            `json:"canonical_target,omitempty"`
	Model           string            `json:"model,omitempty"`
	PromptLen       int               `json:"prompt_len"`
	Prompt          string            `json:"prompt,omitempty"`
	PromptSource    string            `json:"prompt_source,omitempty"`
	Response        string            `json:"response,omitempty"`
	Correlation     map[string]string `json:"correlation,omitempty"`
	NativeSessionID string            `json:"native_session_id,omitempty"`
	NativeLogRef    string            `json:"native_log_ref,omitempty"`
	TraceID         string            `json:"trace_id,omitempty"`
	SpanID          string            `json:"span_id,omitempty"`
	Stderr          string            `json:"stderr,omitempty"`
	Tokens          int               `json:"tokens,omitempty"`
	InputTokens     int               `json:"input_tokens,omitempty"`
	OutputTokens    int               `json:"output_tokens,omitempty"`
	CostUSD         float64           `json:"cost_usd,omitempty"`
	Duration        int               `json:"duration_ms"`
	ExitCode        int               `json:"exit_code"`
	Error           string            `json:"error,omitempty"`
	TotalTokens     int               `json:"total_tokens,omitempty"` // input + output; populated on every run
	BaseRev         string            `json:"base_rev,omitempty"`     // git SHA the execution started from (execute-bead only)
	ResultRev       string            `json:"result_rev,omitempty"`   // git SHA of landed/preserved iteration (execute-bead only)
}

// ProviderStatus tracks provider connectivity and credit status.
type ProviderStatus struct {
	Reachable bool   `json:"reachable"`
	CreditsOK bool   `json:"credits_ok,omitempty"` // false if out of credits/quota
	Error     string `json:"error,omitempty"`
}

// HarnessStatus reports availability of a harness.
type HarnessStatus struct {
	Name      string          `json:"name"`
	Available bool            `json:"available"`
	Binary    string          `json:"binary"`
	Path      string          `json:"path,omitempty"` // resolved binary path
	Error     string          `json:"error,omitempty"`
	Provider  *ProviderStatus `json:"provider,omitempty"` // provider connectivity status
}

// HarnessCapabilities describes the effective capabilities for a harness.
type HarnessCapabilities struct {
	Harness             string            `json:"harness"`
	Available           bool              `json:"available"`
	Binary              string            `json:"binary"`
	Path                string            `json:"path,omitempty"`
	Model               string            `json:"model,omitempty"`
	Models              []string          `json:"models,omitempty"`
	ReasoningLevels     []string          `json:"reasoning_levels,omitempty"`
	Surface             string            `json:"surface,omitempty"`          // catalog surface identifier
	CostClass           string            `json:"cost_class,omitempty"`       // local, cheap, medium, expensive
	IsLocal             bool              `json:"is_local"`                   // true if embedded/local (no cloud cost)
	ExactPinSupport     bool              `json:"exact_pin_support"`          // true if harness accepts exact model pin
	ProfileMappings     map[string]string `json:"profile_mappings,omitempty"` // effective profile → model for this harness
	SupportsEffort      bool              `json:"supports_effort"`            // true if harness has effort/reasoning flag
	SupportsPermissions bool              `json:"supports_permissions"`       // true if harness has permission-level flags
}

// CompareOptions configures a comparison dispatch.
type CompareOptions struct {
	RunOptions
	Harnesses   []string       // harnesses to compare (may include duplicates with different models)
	ArmModels   map[int]string // per-arm model overrides keyed by arm index
	ArmLabels   map[int]string // per-arm display labels (e.g. "claude-fast")
	Sandbox     bool           // run each arm in an isolated worktree
	KeepSandbox bool           // preserve worktrees after comparison
	PostRun     string         // command to run in each worktree after the agent completes
}

// ToolCallEntry records one tool execution during an agent run.
// Mirrors the agent library's ToolCallLog without importing it in types.
type ToolCallEntry struct {
	Tool     string `json:"tool"`
	Input    string `json:"input"`
	Output   string `json:"output,omitempty"`
	Duration int    `json:"duration_ms,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ComparisonArm holds the result of one harness arm in a comparison.
type ComparisonArm struct {
	Harness      string          `json:"harness"`
	Model        string          `json:"model,omitempty"`
	Output       string          `json:"output"`
	Diff         string          `json:"diff,omitempty"`         // git diff of side effects
	ToolCalls    []ToolCallEntry `json:"tool_calls,omitempty"`   // agent tool call log (nil for subprocess)
	PostRunOut   string          `json:"post_run_out,omitempty"` // post-run command output
	PostRunOK    *bool           `json:"post_run_ok,omitempty"`  // post-run pass/fail
	Tokens       int             `json:"tokens,omitempty"`
	InputTokens  int             `json:"input_tokens,omitempty"`
	OutputTokens int             `json:"output_tokens,omitempty"`
	CostUSD      float64         `json:"cost_usd,omitempty"`
	DurationMS   int             `json:"duration_ms"`
	ExitCode     int             `json:"exit_code"`
	Error        string          `json:"error,omitempty"`
}

// ComparisonGrade holds the evaluation of one arm by a grading harness.
type ComparisonGrade struct {
	Arm       string `json:"arm"`
	Score     int    `json:"score"`
	MaxScore  int    `json:"max_score"`
	Pass      bool   `json:"pass"`
	Rationale string `json:"rationale"`
}

// ComparisonRecord is the complete record of a comparison run.
type ComparisonRecord struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Prompt    string            `json:"prompt"`
	Arms      []ComparisonArm   `json:"arms"`
	Grades    []ComparisonGrade `json:"grades,omitempty"`
}

// HarnessState captures the runtime routing-relevant state of a harness.
type HarnessState struct {
	Installed     bool       `json:"installed"`
	Reachable     bool       `json:"reachable"`
	Authenticated bool       `json:"authenticated"`
	QuotaOK       bool       `json:"quota_ok"`
	Degraded      bool       `json:"degraded"`
	PolicyOK      bool       `json:"policy_ok"`
	LastChecked   time.Time  `json:"last_checked,omitempty"`
	Error         string     `json:"error,omitempty"`
	Quota         *QuotaInfo `json:"quota,omitempty"`
}

// QuotaInfo holds parsed quota data from CLI introspection.
type QuotaInfo struct {
	PercentUsed int    `json:"percent_used"`
	LimitWindow string `json:"limit_window,omitempty"` // e.g. "5h", "7 day"
	ResetDate   string `json:"reset_date,omitempty"`   // e.g. "April 12"
}

// RoutingOutcome is one bounded sample of DDx-observed routing performance.
type RoutingOutcome struct {
	Harness         string    `json:"harness"`
	Surface         string    `json:"surface,omitempty"`
	CanonicalTarget string    `json:"canonical_target,omitempty"`
	Model           string    `json:"model,omitempty"`
	ObservedAt      time.Time `json:"observed_at"`
	Success         bool      `json:"success"`
	LatencyMS       int       `json:"latency_ms"`
	InputTokens     int       `json:"input_tokens,omitempty"`
	OutputTokens    int       `json:"output_tokens,omitempty"`
	CostUSD         float64   `json:"cost_usd,omitempty"`
	NativeSessionID string    `json:"native_session_id,omitempty"`
	NativeLogRef    string    `json:"native_log_ref,omitempty"`
	TraceID         string    `json:"trace_id,omitempty"`
	SpanID          string    `json:"span_id,omitempty"`
}

// QuotaSnapshot captures one quota/headroom sample for routing.
type QuotaSnapshot struct {
	Harness         string    `json:"harness"`
	Surface         string    `json:"surface,omitempty"`
	CanonicalTarget string    `json:"canonical_target,omitempty"`
	Source          string    `json:"source,omitempty"`
	ObservedAt      time.Time `json:"observed_at"`
	QuotaState      string    `json:"quota_state"`
	UsedPercent     int       `json:"used_percent,omitempty"`
	WindowMinutes   int       `json:"window_minutes,omitempty"`
	ResetsAt        string    `json:"resets_at,omitempty"`
	SampleKind      string    `json:"sample_kind"`
}

// BurnSummary is a derived relative subscription-pressure estimate.
type BurnSummary struct {
	Harness         string    `json:"harness"`
	Surface         string    `json:"surface,omitempty"`
	CanonicalTarget string    `json:"canonical_target,omitempty"`
	ObservedAt      time.Time `json:"observed_at"`
	BurnIndex       float64   `json:"burn_index"`
	Trend           string    `json:"trend,omitempty"`
	Confidence      float64   `json:"confidence,omitempty"`
	Basis           string    `json:"basis,omitempty"`
}

// RouteRequest is the normalized routing ask built from CLI flags and config.
type RouteRequest struct {
	Profile         string // cheap, fast, smart
	ModelRef        string // logical catalog ref or alias
	ModelPin        string // exact concrete model string (bypasses catalog policy)
	Effort          string // low, medium, high, etc.
	Permissions     string // safe, supervised, unrestricted
	HarnessOverride string // forces routing to one harness only
}

// CandidatePlan is a routing evaluation result for one harness.
type CandidatePlan struct {
	Harness             string       `json:"harness"`
	Surface             string       `json:"surface,omitempty"`          // catalog surface: embedded-openai, embedded-anthropic, codex, claude
	RequestedRef        string       `json:"requested_ref,omitempty"`    // profile or model ref from the request
	CanonicalTarget     string       `json:"canonical_target,omitempty"` // resolved catalog canonical target
	ConcreteModel       string       `json:"concrete_model,omitempty"`   // concrete model string to pass to harness
	SupportsEffort      bool         `json:"supports_effort"`
	SupportsPermissions bool         `json:"supports_permissions"`
	State               HarnessState `json:"state"`
	CostClass           string       `json:"cost_class,omitempty"`          // local, cheap, medium, expensive
	EstimatedCostUSD    float64      `json:"estimated_cost_usd,omitempty"`  // -1 = unknown
	RejectReason        string       `json:"reject_reason,omitempty"`       // non-empty means rejected
	DeprecationWarning  string       `json:"deprecation_warning,omitempty"` // non-empty when requested ref is deprecated
	Score               float64      `json:"score,omitempty"`
	Viable              bool         `json:"viable"`
}

// Default configuration values.
const (
	DefaultHarness   = "codex"
	DefaultTimeoutMS = 300000 // 5 minutes
	DefaultLogDir    = ".ddx/agent-logs"
)
