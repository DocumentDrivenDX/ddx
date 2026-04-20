package agent

import (
	"context"
	"path/filepath"
	"time"
)

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
	TUIQuotaCommand string              // Slash command to send as a prompt when native quota signal is unavailable (e.g. "/usage", "/status"). Invoked with the binary's non-interactive print mode.
	IsHTTPProvider  bool                // true for API-only providers (openrouter, lmstudio) that have no CLI binary.
	IsSubscription  bool                // true for fixed-subscription harnesses (codex, claude) — preferred over pay-per-token when within quota.
	TestOnly        bool                // true for sentinel/test harnesses (script, virtual) that must never be selected by production tier routing. Explicit --harness override still works.
}

// Config holds agent service configuration.
type Config struct {
	Profile         string              `yaml:"profile"`          // default routing intent: cheap, standard, smart
	Harness         string              `yaml:"harness"`          // optional forced harness override
	Model           string              `yaml:"model"`            // optional default model ref or exact pin
	Models          map[string]string   `yaml:"models"`           // per-harness model overrides
	ReasoningLevels map[string][]string `yaml:"reasoning_levels"` // per-harness reasoning-level options
	TimeoutMS       int                 `yaml:"timeout_ms"`       // idle (inactivity) timeout in ms — resets on every stream/event
	WallClockMS     int                 `yaml:"wall_clock_ms"`    // absolute wall-clock cap in ms — fires regardless of activity
	SessionLogDir   string              `yaml:"session_log_dir"`  // log directory
	Permissions     string              `yaml:"permissions"`      // permission level: safe, supervised, unrestricted
}

// RouteFlags holds raw CLI flag values before normalization into a RouteRequest.
// These come directly from parsed command-line arguments.
type RouteFlags struct {
	Profile     string // --profile: cheap, standard, smart
	Model       string // --model: logical ref or exact pin
	Provider    string // --provider: explicit provider name
	ModelRef    string // --model-ref: catalog model reference
	Harness     string // --harness: forced harness override
	Effort      string // --effort: low, medium, high
	Permissions string // --permissions: safe, supervised, unrestricted
}

// RunOptions holds options for a single agent invocation.
type RunOptions struct {
	// Context is the caller's context. When non-nil, the Runner derives its
	// internal cancel context from this so upstream cancellation (e.g.
	// server.WorkerManager.Stop) propagates into the provider HTTP call.
	// Nil defaults to context.Background().
	Context       context.Context
	Harness       string
	Prompt        string // prompt text (or path to file)
	PromptFile    string // explicit file path
	PromptSource  string
	Correlation   map[string]string
	Model         string
	Provider      string // explicit provider name (e.g. "vidar", "openrouter"); bypasses default provider selection
	ModelRef      string // catalog model-ref (e.g. "code-medium"); resolved via the model catalog
	Effort        string
	Timeout       time.Duration // idle (inactivity) timeout; nonzero overrides Config.TimeoutMS
	WallClock     time.Duration // absolute wall-clock cap; nonzero overrides Config.WallClockMS
	WorkDir       string
	Permissions   string // permission level override: safe, supervised, unrestricted
	SessionLogDir string // per-run override for session log dir; used by execute-bead to redirect embedded-agent runtime state out of the worktree root
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
	Harness         string `json:"harness"`
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ExitCode        int    `json:"exit_code"`
	Output          string `json:"output"`
	CondensedOutput string `json:"condensed_output,omitempty"`
	Stderr          string `json:"stderr,omitempty"`
	// Routing evidence populated by the embedded agent harness (RunAgent) or
	// the script harness (RunScript). Used by ExecuteBead to record kind:routing
	// evidence on the bead.
	RouteReason     string          `json:"route_reason,omitempty"`
	ResolvedBaseURL string          `json:"resolved_base_url,omitempty"`
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
	Installed     bool                   `json:"installed"`
	Reachable     bool                   `json:"reachable"`
	Authenticated bool                   `json:"authenticated"`
	QuotaOK       bool                   `json:"quota_ok"`
	QuotaState    string                 `json:"quota_state,omitempty"` // ok, blocked, unknown
	Degraded      bool                   `json:"degraded"`
	PolicyOK      bool                   `json:"policy_ok"`
	LastChecked   time.Time              `json:"last_checked,omitempty"`
	Error         string                 `json:"error,omitempty"`
	Quota         *QuotaInfo             `json:"quota,omitempty"`
	RoutingSignal *RoutingSignalSnapshot `json:"routing_signal,omitempty"`
}

// QuotaInfo holds parsed quota data from CLI introspection.
type QuotaInfo struct {
	PercentUsed int    `json:"percent_used"`
	LimitWindow string `json:"limit_window,omitempty"` // e.g. "5h", "7 day"
	ResetDate   string `json:"reset_date,omitempty"`   // e.g. "April 12"
}

// AccountInfo captures provider account metadata from local auth files.
type AccountInfo struct {
	Email    string `json:"email,omitempty"`
	PlanType string `json:"plan_type,omitempty"`
	OrgName  string `json:"org_name,omitempty"`
}

// QuotaWindow captures one quota window (e.g. 5h, weekly, model-specific).
type QuotaWindow struct {
	Name          string  `json:"name"`               // e.g. "5h", "7d", "spark"
	LimitID       string  `json:"limit_id,omitempty"` // provider limit_id
	WindowMinutes int     `json:"window_minutes"`
	UsedPercent   float64 `json:"used_percent"`
	ResetsAt      string  `json:"resets_at,omitempty"`      // human-readable
	ResetsAtUnix  int64   `json:"resets_at_unix,omitempty"` // unix timestamp
	State         string  `json:"state"`
}

// SignalSourceMetadata captures where a routing signal came from and how
// fresh it is. Populated from upstream HarnessInfo/ProviderInfo.
type SignalSourceMetadata struct {
	Provider   string    `json:"provider"`
	Kind       string    `json:"kind"`
	Path       string    `json:"path,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
	Freshness  string    `json:"freshness"`
	AgeSeconds int64     `json:"age_seconds,omitempty"`
	Basis      string    `json:"basis,omitempty"`
	Notes      string    `json:"notes,omitempty"`
}

// QuotaSignal captures current quota/headroom for a harness.
type QuotaSignal struct {
	Source        SignalSourceMetadata `json:"source"`
	State         string               `json:"state"`
	UsedPercent   int                  `json:"used_percent,omitempty"`
	WindowMinutes int                  `json:"window_minutes,omitempty"`
	ResetsAt      string               `json:"resets_at,omitempty"`
}

// UsageSignal captures token/session totals for a harness.
type UsageSignal struct {
	Source            SignalSourceMetadata `json:"source"`
	InputTokens       int                  `json:"input_tokens,omitempty"`
	CachedInputTokens int                  `json:"cached_input_tokens,omitempty"`
	OutputTokens      int                  `json:"output_tokens,omitempty"`
	TotalTokens       int                  `json:"total_tokens,omitempty"`
	SessionCount      int                  `json:"session_count,omitempty"`
}

// RoutingSignalSnapshot captures routing signal metadata for a harness.
// Populated from upstream HarnessInfo/ProviderInfo data — no provider-native
// file parsing.
type RoutingSignalSnapshot struct {
	Provider        string               `json:"provider"`
	Source          SignalSourceMetadata `json:"source"`
	CurrentQuota    QuotaSignal          `json:"current_quota,omitempty"`
	HistoricalUsage UsageSignal          `json:"historical_usage,omitempty"`
	Account         *AccountInfo         `json:"account,omitempty"`
	QuotaWindows    []QuotaWindow        `json:"quota_windows,omitempty"`
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
	Profile         string // cheap, standard, smart
	ModelRef        string // logical catalog ref or alias
	ModelPin        string // exact concrete model string (bypasses catalog policy)
	Effort          string // low, medium, high, etc.
	Permissions     string // safe, supervised, unrestricted
	HarnessOverride string // forces routing to one harness only
}

// CandidatePlan is a routing evaluation result for one harness.
type CandidatePlan struct {
	Harness               string       `json:"harness"`
	Surface               string       `json:"surface,omitempty"`          // catalog surface: embedded-openai, embedded-anthropic, codex, claude
	RequestedRef          string       `json:"requested_ref,omitempty"`    // profile or model ref from the request
	CanonicalTarget       string       `json:"canonical_target,omitempty"` // resolved catalog canonical target
	ConcreteModel         string       `json:"concrete_model,omitempty"`   // concrete model string to pass to harness
	SupportsEffort        bool         `json:"supports_effort"`
	SupportsPermissions   bool         `json:"supports_permissions"`
	State                 HarnessState `json:"state"`
	Provider              string       `json:"provider,omitempty"`            // discovered provider endpoint name (e.g. vidar, bragi)
	CostClass             string       `json:"cost_class,omitempty"`          // local, cheap, medium, expensive
	IsSubscription        bool         `json:"is_subscription,omitempty"`     // fixed-subscription harness; preferred over pay-per-token within quota
	EstimatedCostUSD      float64      `json:"estimated_cost_usd,omitempty"`  // -1 = unknown
	HistoricalSuccessRate float64      `json:"historical_success_rate"`       // observed success rate from routing metrics; -1 when insufficient data (< 3 samples)
	RejectReason          string       `json:"reject_reason,omitempty"`       // non-empty means rejected
	DeprecationWarning    string       `json:"deprecation_warning,omitempty"` // non-empty when requested ref is deprecated
	Score                 float64      `json:"score,omitempty"`
	Viable                bool         `json:"viable"`
}

// Default configuration values.
const (
	DefaultHarness = "codex"
	// DefaultTimeoutMS is the default idle (inactivity) timeout — resets on
	// every stdout/stderr byte or agent event. 2 hours is long enough for any
	// task where the provider streams progress; a stuck provider still needs
	// DefaultWallClockMS to guarantee termination.
	DefaultTimeoutMS = 7200000 // 2 hours
	// DefaultWallClockMS is the absolute wall-clock cap applied in addition to
	// the idle timeout. It fires regardless of stream/event activity, so a
	// provider that emits heartbeats forever cannot pin a worker indefinitely.
	// Sized at 3h — roughly 1.5x the idle timeout — so normal long tasks still
	// complete while genuinely hung workers free themselves. See
	// RC2 of ddx-0a651925 for the incident that motivated this bound.
	DefaultWallClockMS = 10800000 // 3 hours
	DefaultLogDir      = ".ddx/agent-logs"
)

// ResolveLogDir returns an absolute session-log directory path anchored at
// projectRoot. Callers that construct an agent.Runner must use this to avoid
// the Runner's relative DefaultLogDir resolving against process CWD — which
// historically wrote logs to stray locations like cli/internal/server/.ddx/.
func ResolveLogDir(projectRoot, configured string) string {
	if configured == "" {
		configured = DefaultLogDir
	}
	if filepath.IsAbs(configured) {
		return configured
	}
	if projectRoot == "" {
		return configured
	}
	return filepath.Join(projectRoot, configured)
}
