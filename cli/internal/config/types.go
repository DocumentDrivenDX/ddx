package config

import (
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/triage"
)

// NewConfig represents the simplified DDx configuration structure
// This aligns with the schema defined in ADR-005 and SD-003
type NewConfig struct {
	Version         string              `yaml:"version" json:"version"`
	Library         *LibraryConfig      `yaml:"library" json:"library"`
	Bead            *BeadConfig         `yaml:"bead,omitempty" json:"bead,omitempty"`
	System          *SystemConfig       `yaml:"system,omitempty" json:"system,omitempty"`
	PersonaBindings map[string]string   `yaml:"persona_bindings,omitempty" json:"persona_bindings,omitempty"`
	UpdateCheck     *UpdateCheckConfig  `yaml:"update_check,omitempty" json:"update_check,omitempty"`
	Agent           *AgentConfig        `yaml:"agent,omitempty" json:"agent,omitempty"`
	Git             *GitConfig          `yaml:"git,omitempty" json:"git,omitempty"`
	Server          *ServerConfig       `yaml:"server,omitempty" json:"server,omitempty"`
	Executions      *ExecutionsConfig   `yaml:"executions,omitempty" json:"executions,omitempty"`
	Cost            *CostConfig         `yaml:"cost,omitempty" json:"cost,omitempty"`
	Workers         *WorkersConfig      `yaml:"workers,omitempty" json:"workers,omitempty"`
	EvidenceCaps    *EvidenceCapsConfig `yaml:"evidence_caps,omitempty" json:"evidence_caps,omitempty"`
	// ReviewMaxRetries caps reviewer attempts per committed result_rev before
	// the execute-loop emits a terminal `review-manual-required` event and
	// parks the bead (FEAT-022 §14). Zero or unset uses the binary default
	// (3). Negative values are treated as unset.
	ReviewMaxRetries *int `yaml:"review_max_retries,omitempty" json:"review_max_retries,omitempty"`
	// Triage configures the post-attempt triage decision module
	// (cli/internal/triage). Optional; when unset the binary default policy
	// is used. Distinct from agent.triage, which controls the pre-claim
	// complexity gate.
	Triage *TriagePolicyConfig `yaml:"triage,omitempty" json:"triage,omitempty"`
}

// TriagePolicyConfig is the project-config form of triage.TriagePolicy.
// Each entry of `policies` maps a failure mode name to an ordered ladder
// of action names; missing modes inherit the binary default ladder.
type TriagePolicyConfig struct {
	Policies map[string][]string `yaml:"policies,omitempty" json:"policies,omitempty"`
}

// ResolveTriagePolicy returns the effective triage policy for this config,
// layering any `triage.policies` entries on top of the binary default
// ladders. Unknown failure-mode names and unknown action names are
// silently dropped (schema validation surfaces typos at load time).
func (c *NewConfig) ResolveTriagePolicy() triage.TriagePolicy {
	policy := triage.DefaultPolicy()
	if c == nil || c.Triage == nil || len(c.Triage.Policies) == 0 {
		return policy
	}
	for modeStr, actionStrs := range c.Triage.Policies {
		mode, err := triage.ParseFailureMode(modeStr)
		if err != nil {
			continue
		}
		ladder := make([]triage.Action, 0, len(actionStrs))
		for _, a := range actionStrs {
			act, err := triage.ParseAction(a)
			if err != nil {
				continue
			}
			ladder = append(ladder, act)
		}
		if len(ladder) == 0 {
			continue
		}
		policy.Ladders[mode] = ladder
	}
	return policy
}

// ResolveReviewMaxRetries returns the effective reviewer retry cap for this
// config. Defaults to 3 when unset (matching agent.DefaultReviewMaxRetries).
func (c *NewConfig) ResolveReviewMaxRetries() int {
	if c == nil || c.ReviewMaxRetries == nil || *c.ReviewMaxRetries <= 0 {
		return 3
	}
	return *c.ReviewMaxRetries
}

// EvidenceCapsConfig configures byte-size caps used by the shared
// evidence-assembly primitives (FEAT-022 §1a). Project-level fields
// override the binary defaults; entries in `per_harness` further
// override the project-level values for a specific harness name.
type EvidenceCapsConfig struct {
	MaxPromptBytes       *int                             `yaml:"max_prompt_bytes,omitempty" json:"max_prompt_bytes,omitempty"`
	MaxInlinedFileBytes  *int                             `yaml:"max_inlined_file_bytes,omitempty" json:"max_inlined_file_bytes,omitempty"`
	MaxDiffBytes         *int                             `yaml:"max_diff_bytes,omitempty" json:"max_diff_bytes,omitempty"`
	MaxGoverningDocBytes *int                             `yaml:"max_governing_doc_bytes,omitempty" json:"max_governing_doc_bytes,omitempty"`
	PerHarness           map[string]*EvidenceCapsOverride `yaml:"per_harness,omitempty" json:"per_harness,omitempty"`
	// ContextBudget is the prompt budget label that execute-bead consults
	// when assembling the bead prompt. Empty string means "full budget";
	// "minimal" omits large governing documents (cheap-tier path). See
	// SD-024 §Stage 3 — ExecuteBeadOptions migration.
	ContextBudget string `yaml:"context_budget,omitempty" json:"context_budget,omitempty"`
}

// ResolveContextBudget returns the effective context-budget label for this
// config. Defaults to "" (full budget) when unset.
func (e *EvidenceCapsConfig) ResolveContextBudget() string {
	if e == nil {
		return ""
	}
	return e.ContextBudget
}

// EvidenceCapsOverride is the per-harness override shape inside
// `evidence_caps.per_harness`.
type EvidenceCapsOverride struct {
	MaxPromptBytes       *int `yaml:"max_prompt_bytes,omitempty" json:"max_prompt_bytes,omitempty"`
	MaxInlinedFileBytes  *int `yaml:"max_inlined_file_bytes,omitempty" json:"max_inlined_file_bytes,omitempty"`
	MaxDiffBytes         *int `yaml:"max_diff_bytes,omitempty" json:"max_diff_bytes,omitempty"`
	MaxGoverningDocBytes *int `yaml:"max_governing_doc_bytes,omitempty" json:"max_governing_doc_bytes,omitempty"`
}

// WorkersConfig controls the Add/Remove-worker affordances on the workers
// overview. `default_spec` supplies sane defaults for one-click worker
// dispatch; `max_count` optionally caps concurrent drain workers per project.
type WorkersConfig struct {
	DefaultSpec *WorkerDefaultSpec `yaml:"default_spec,omitempty" json:"default_spec,omitempty"`
	MaxCount    *int               `yaml:"max_count,omitempty" json:"max_count,omitempty"`
	// NoProgressCooldown bounds how long a bead may sit without a phase
	// transition before the execute-loop forces the worker to give up. Parsed
	// via time.ParseDuration (e.g. "6h"). Empty string uses the built-in
	// default (6h). See SD-024 §Config extensions §WorkersConfig.
	NoProgressCooldown string `yaml:"no_progress_cooldown,omitempty" json:"no_progress_cooldown,omitempty"`
	// MaxNoChangesBeforeClose caps the number of consecutive no_changes
	// attempts the execute-loop tolerates before closing the bead as
	// no-progress. Zero or unset uses the built-in default (3). Negative
	// values are treated as unset.
	MaxNoChangesBeforeClose *int `yaml:"max_no_changes_before_close,omitempty" json:"max_no_changes_before_close,omitempty"`
	// HeartbeatInterval is how often a claim owner refreshes its heartbeat
	// while holding a bead. Parsed via time.ParseDuration (e.g. "15s"). Empty
	// string uses the built-in default that matches bead.HeartbeatInterval
	// (30s today).
	HeartbeatInterval string `yaml:"heartbeat_interval,omitempty" json:"heartbeat_interval,omitempty"`
}

// Default values for WorkersConfig resolvers. These mirror the hardcoded
// constants the execute-loop uses today; bead 6/7 of SD-024 wires the
// resolvers into the loop.
const (
	defaultNoProgressCooldown      = 6 * time.Hour
	defaultMaxNoChangesBeforeClose = 3
	defaultHeartbeatInterval       = 30 * time.Second
)

// ResolveNoProgressCooldown returns the effective no-progress cooldown for
// this config. Defaults to 6h when unset or unparseable.
func (w *WorkersConfig) ResolveNoProgressCooldown() time.Duration {
	if w == nil || w.NoProgressCooldown == "" {
		return defaultNoProgressCooldown
	}
	d, err := time.ParseDuration(w.NoProgressCooldown)
	if err != nil || d <= 0 {
		return defaultNoProgressCooldown
	}
	return d
}

// ResolveMaxNoChangesBeforeClose returns the effective max no_changes cap.
// Defaults to 3 when unset or non-positive.
func (w *WorkersConfig) ResolveMaxNoChangesBeforeClose() int {
	if w == nil || w.MaxNoChangesBeforeClose == nil || *w.MaxNoChangesBeforeClose <= 0 {
		return defaultMaxNoChangesBeforeClose
	}
	return *w.MaxNoChangesBeforeClose
}

// ResolveHeartbeatInterval returns the effective heartbeat refresh interval.
// Defaults to 30s (matching bead.HeartbeatInterval today) when unset or
// unparseable.
func (w *WorkersConfig) ResolveHeartbeatInterval() time.Duration {
	if w == nil || w.HeartbeatInterval == "" {
		return defaultHeartbeatInterval
	}
	d, err := time.ParseDuration(w.HeartbeatInterval)
	if err != nil || d <= 0 {
		return defaultHeartbeatInterval
	}
	return d
}

// WorkerDefaultSpec mirrors the knobs a one-click "+ Add worker" dispatch
// honours. Any field left unset falls back to the built-in `ddx work` defaults.
type WorkerDefaultSpec struct {
	Harness string `yaml:"harness,omitempty" json:"harness,omitempty"`
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	Effort  string `yaml:"effort,omitempty" json:"effort,omitempty"`
}

// CostConfig controls optional cost estimates that DDx cannot infer safely.
type CostConfig struct {
	LocalPer1KTokens *float64 `yaml:"local_per_1k_tokens,omitempty" json:"local_per_1k_tokens,omitempty"`
}

// ExecutionsConfig configures the execute-bead bundle archive (mirror).
type ExecutionsConfig struct {
	Mirror     *ExecutionsMirrorConfig `yaml:"mirror,omitempty" json:"mirror,omitempty"`
	RetainDays int                     `yaml:"retain_days,omitempty" json:"retain_days,omitempty"`
}

// ExecutionsMirrorConfig describes the out-of-band archive target for
// .ddx/executions/<attempt>/ bundles. A configured kind plus path is enough
// to enable mirroring; missing entries leave mirroring disabled.
type ExecutionsMirrorConfig struct {
	Kind    string   `yaml:"kind,omitempty" json:"kind,omitempty"`
	Path    string   `yaml:"path,omitempty" json:"path,omitempty"`
	Include []string `yaml:"include,omitempty" json:"include,omitempty"`
	Async   *bool    `yaml:"async,omitempty" json:"async,omitempty"`
}

// ServerConfig represents server configuration settings.
type ServerConfig struct {
	Addr  string       `yaml:"addr,omitempty" json:"addr,omitempty"`
	Tsnet *TsnetConfig `yaml:"tsnet,omitempty" json:"tsnet,omitempty"`
	// WatchdogDeadline bounds total worker lifetime before the autonomous
	// watchdog considers reaping it. Parsed via time.ParseDuration (e.g. "6h").
	// Empty string uses the built-in default (6h).
	WatchdogDeadline string `yaml:"watchdog_deadline,omitempty" json:"watchdog_deadline,omitempty"`
	// StallDeadline is the max time the current attempt may sit without a
	// phase transition before the watchdog considers it stalled. Parsed via
	// time.ParseDuration (e.g. "1h"). Empty string uses the built-in default (1h).
	StallDeadline string `yaml:"stall_deadline,omitempty" json:"stall_deadline,omitempty"`
}

// TsnetConfig represents Tailscale ts-net listener configuration (ADR-006).
type TsnetConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Hostname string `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	AuthKey  string `yaml:"auth_key,omitempty" json:"auth_key,omitempty"`
	StateDir string `yaml:"state_dir,omitempty" json:"state_dir,omitempty"`
}

// GitConfig represents git integration configuration settings.
type GitConfig struct {
	AutoCommit       string   `yaml:"auto_commit,omitempty" json:"auto_commit,omitempty"`
	CommitPrefix     string   `yaml:"commit_prefix,omitempty" json:"commit_prefix,omitempty"`
	CheckpointPrefix string   `yaml:"checkpoint_prefix,omitempty" json:"checkpoint_prefix,omitempty"`
	PostLandCommand  []string `yaml:"post_land_command,omitempty" json:"post_land_command,omitempty"`
	// LargeDeletionLineThreshold controls Land()'s per-file deletion safety
	// gate. Zero or unset uses the binary default.
	LargeDeletionLineThreshold int `yaml:"large_deletion_line_threshold,omitempty" json:"large_deletion_line_threshold,omitempty"`
}

// AgentConfig represents agent service configuration in .ddx/config.yaml
type AgentConfig struct {
	Harness         string              `yaml:"harness,omitempty" json:"harness,omitempty"`
	Model           string              `yaml:"model,omitempty" json:"model,omitempty"`
	Models          map[string]string   `yaml:"models,omitempty" json:"models,omitempty"`
	ReasoningLevels map[string][]string `yaml:"reasoning_levels,omitempty" json:"reasoning_levels,omitempty"`
	TimeoutMS       int                 `yaml:"timeout_ms,omitempty" json:"timeout_ms,omitempty"`
	SessionLogDir   string              `yaml:"session_log_dir,omitempty" json:"session_log_dir,omitempty"`
	Permissions     string              `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Endpoints       []AgentEndpoint     `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
	Routing         *RoutingConfig      `yaml:"routing,omitempty" json:"routing,omitempty"`
	Virtual         *VirtualConfig      `yaml:"virtual,omitempty" json:"virtual,omitempty"`
	Triage          *TriageConfig       `yaml:"triage,omitempty" json:"triage,omitempty"`
}

// TriageConfig controls the pre-Claim complexity gate (ddx-5bf4ee7e).
type TriageConfig struct {
	// MaxDecompositionDepth caps recursive bead splitting. Children re-enter
	// the gate; at cap the parent is blocked with label=needs-human-decomposition.
	// Zero or unset uses the binary default (3). Configurable as
	// agent.triage.max_decomposition_depth in .ddx/config.yaml.
	MaxDecompositionDepth *int `yaml:"max_decomposition_depth,omitempty" json:"max_decomposition_depth,omitempty"`
}

// ResolveMaxDecompositionDepth returns the effective depth cap for the triage
// gate. Defaults to 3 when unset or non-positive.
func (c *NewConfig) ResolveMaxDecompositionDepth() int {
	const defaultDepth = 3
	if c == nil || c.Agent == nil || c.Agent.Triage == nil || c.Agent.Triage.MaxDecompositionDepth == nil {
		return defaultDepth
	}
	if *c.Agent.Triage.MaxDecompositionDepth <= 0 {
		return defaultDepth
	}
	return *c.Agent.Triage.MaxDecompositionDepth
}

// AgentEndpoint describes one endpoint-first native agent provider target.
// Name and model are intentionally absent: routing discovers the live model IDs
// from the endpoint's /v1/models response at dispatch time.
type AgentEndpoint struct {
	Type    string `yaml:"type,omitempty" json:"type,omitempty"`
	Host    string `yaml:"host,omitempty" json:"host,omitempty"`
	Port    int    `yaml:"port,omitempty" json:"port,omitempty"`
	BaseURL string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	APIKey  string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	// RequestTimeoutSeconds is the wall-clock cap for a single Chat/ChatStream
	// call to this endpoint. Zero means "use the model-class default" (15 min
	// for standard models, 60 min for known thinking/reasoning models).
	// Increase this for local thinking models that need >15 min to produce
	// their first body delta (e.g. qwen3.6-35b on lmstudio).
	// Example: request_timeout_seconds: 3600  # 1 hour
	RequestTimeoutSeconds int `yaml:"request_timeout_seconds,omitempty" json:"request_timeout_seconds,omitempty"`
}

// RoutingConfig is the agent routing policy block.
//
// Note: agent.routing.default_harness was REMOVED in the routing-config
// deprecation pass (bead ddx-87fb72c2). agent.routing.profile_ladders and
// agent.routing.model_overrides were removed in bead ddx-3bd7396a. Configs
// that still carry any of these fields fail to load with a migration message.
type RoutingConfig struct {
	// ProfilePriority is the deprecated flat-list form; still parsed so
	// existing configs can emit a deprecation warning.
	ProfilePriority []string `yaml:"profile_priority,omitempty" json:"profile_priority,omitempty"`
}

// AgentRunnerConfig was the embedded DDx Agent harness config block.
// Deprecated: Use native .agent/config.yaml instead. This type is retained for
// schema compatibility so existing configs with agent_runner blocks parse without error,
// but DDx no longer reads or applies these values.
type AgentRunnerConfig struct {
	Provider      string `yaml:"provider,omitempty" json:"provider,omitempty"`
	BaseURL       string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	APIKey        string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Model         string `yaml:"model,omitempty" json:"model,omitempty"`
	Preset        string `yaml:"preset,omitempty" json:"preset,omitempty"`
	MaxIterations int    `yaml:"max_iterations,omitempty" json:"max_iterations,omitempty"`
}

// LLMPresetConfig defines a named LLM configuration with optional multi-endpoint support.
// Deprecated: kept for schema compatibility; no longer read by DDx code.
type LLMPresetConfig struct {
	Model     string   `yaml:"model" json:"model"`
	Provider  string   `yaml:"provider,omitempty" json:"provider,omitempty"`
	Endpoints []string `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
	APIKey    string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Strategy  string   `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}

// VirtualConfig configures the virtual agent harness.
type VirtualConfig struct {
	Normalize []NormalizePattern `yaml:"normalize,omitempty" json:"normalize,omitempty"`
}

// NormalizePattern is a regex→replacement pair applied to prompts before hashing.
type NormalizePattern struct {
	Pattern string `yaml:"pattern" json:"pattern"`
	Replace string `yaml:"replace" json:"replace"`
}

// SystemConfig represents system-level configuration settings
type SystemConfig struct {
	MetaPrompt *string `yaml:"meta_prompt,omitempty" json:"meta_prompt,omitempty"`
}

// LibraryConfig represents library configuration settings
type LibraryConfig struct {
	Path       string            `yaml:"path,omitempty" json:"path,omitempty"`
	Repository *RepositoryConfig `yaml:"repository" json:"repository"`
}

// BeadConfig represents bead tracker configuration settings.
type BeadConfig struct {
	IDPrefix string `yaml:"id_prefix,omitempty" json:"id_prefix,omitempty"`
	// Backend selects the storage backend ("jsonl", "bd", "br", or "axon").
	// Default is "jsonl". Overridden by the DDX_BEAD_BACKEND environment
	// variable when set. The "axon" backend is experimental and requires
	// DDX_AXON_EXPERIMENTAL=1 to be honoured (per ddx-95ec5ed5).
	Backend string `yaml:"backend,omitempty" json:"backend,omitempty"`
}

// RepositoryConfig represents repository settings for the new format
type RepositoryConfig struct {
	URL    string `yaml:"url" json:"url"`
	Branch string `yaml:"branch" json:"branch"`
}

// UpdateCheckConfig represents update checking settings
type UpdateCheckConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Frequency string `yaml:"frequency"` // Duration: "24h", "12h", etc.
}

// DefaultNewConfig returns a new config with default values applied
func DefaultNewConfig() *NewConfig {
	return &NewConfig{
		Version: "1.0",
		Library: &LibraryConfig{
			Path: ".ddx/plugins/ddx",
			Repository: &RepositoryConfig{
				URL:    "https://github.com/DocumentDrivenDX/ddx-library",
				Branch: "main",
			},
		},
		PersonaBindings: make(map[string]string),
		UpdateCheck: &UpdateCheckConfig{
			Enabled:   true,
			Frequency: "24h",
		},
	}
}

// DefaultConfig is an alias for DefaultNewConfig for compatibility
var DefaultConfig = DefaultNewConfig()

// GetMetaPrompt returns the meta-prompt path, defaulting to focused.md if unset
// Returns empty string if explicitly set to null/empty (disabled)
func (c *NewConfig) GetMetaPrompt() string {
	if c.System == nil || c.System.MetaPrompt == nil {
		// Unset: return default
		return "claude/system-prompts/focused.md"
	}
	// Explicitly set (could be empty string to disable)
	return *c.System.MetaPrompt
}

// ApplyDefaults ensures all required fields have default values
func (c *NewConfig) ApplyDefaults() {
	if c.Version == "" {
		c.Version = "1.0"
	}
	if c.Library == nil {
		c.Library = &LibraryConfig{
			Path: ".ddx/plugins/ddx",
			Repository: &RepositoryConfig{
				URL:    "https://github.com/DocumentDrivenDX/ddx-library",
				Branch: "main",
			},
		}
	} else {
		if c.Library.Path == "" {
			c.Library.Path = ".ddx/plugins/ddx"
		}
		if c.Library.Repository == nil {
			c.Library.Repository = &RepositoryConfig{
				URL:    "https://github.com/DocumentDrivenDX/ddx-library",
				Branch: "main",
			}
		} else {
			if c.Library.Repository.URL == "" {
				c.Library.Repository.URL = "https://github.com/DocumentDrivenDX/ddx-library"
			}
			if c.Library.Repository.Branch == "" {
				c.Library.Repository.Branch = "main"
			}
		}
	}
	if c.Bead == nil {
		c.Bead = &BeadConfig{}
	}
	if c.PersonaBindings == nil {
		c.PersonaBindings = make(map[string]string)
	}
	if c.UpdateCheck == nil {
		c.UpdateCheck = &UpdateCheckConfig{
			Enabled:   true,
			Frequency: "24h",
		}
	} else {
		if c.UpdateCheck.Frequency == "" {
			c.UpdateCheck.Frequency = "24h"
		}
	}
}
