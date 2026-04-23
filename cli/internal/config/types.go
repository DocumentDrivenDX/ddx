package config

// NewConfig represents the simplified DDx configuration structure
// This aligns with the schema defined in ADR-005 and SD-003
type NewConfig struct {
	Version         string             `yaml:"version" json:"version"`
	Library         *LibraryConfig     `yaml:"library" json:"library"`
	Bead            *BeadConfig        `yaml:"bead,omitempty" json:"bead,omitempty"`
	System          *SystemConfig      `yaml:"system,omitempty" json:"system,omitempty"`
	PersonaBindings map[string]string  `yaml:"persona_bindings,omitempty" json:"persona_bindings,omitempty"`
	UpdateCheck     *UpdateCheckConfig `yaml:"update_check,omitempty" json:"update_check,omitempty"`
	Agent           *AgentConfig       `yaml:"agent,omitempty" json:"agent,omitempty"`
	Git             *GitConfig         `yaml:"git,omitempty" json:"git,omitempty"`
	Server          *ServerConfig      `yaml:"server,omitempty" json:"server,omitempty"`
	Executions      *ExecutionsConfig  `yaml:"executions,omitempty" json:"executions,omitempty"`
	Cost            *CostConfig        `yaml:"cost,omitempty" json:"cost,omitempty"`
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
	AutoCommit       string `yaml:"auto_commit,omitempty" json:"auto_commit,omitempty"`
	CommitPrefix     string `yaml:"commit_prefix,omitempty" json:"commit_prefix,omitempty"`
	CheckpointPrefix string `yaml:"checkpoint_prefix,omitempty" json:"checkpoint_prefix,omitempty"`
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
}

// RoutingConfig is the agent routing policy block. See FEAT-006 Profile
// Routing and epic ddx-bbb65768. ProfilePriority is the legacy flat list
// and is deprecated in favour of ProfileLadders (per-profile tier lists).
// When both are present, ProfileLadders wins.
type RoutingConfig struct {
	// ProfilePriority is the deprecated flat-list form. New configs should
	// use ProfileLadders.
	ProfilePriority []string `yaml:"profile_priority,omitempty" json:"profile_priority,omitempty"`
	// ProfileLadders maps a profile name to the ordered tier list that a
	// dispatch should try. Example:
	//   default: [cheap, standard, smart]
	//   cheap:   [cheap]
	//   fast:    [fast, smart]
	//   smart:   [smart]
	ProfileLadders map[string][]string `yaml:"profile_ladders,omitempty" json:"profile_ladders,omitempty"`
	// DefaultHarness is the fallback harness when no profile match succeeds.
	DefaultHarness string `yaml:"default_harness,omitempty" json:"default_harness,omitempty"`
	// ModelOverrides maps a profile name to a concrete model reference.
	// e.g. { "cheap": "qwen/qwen3.6", "smart": "claude-opus-4-6" }.
	ModelOverrides map[string]string `yaml:"model_overrides,omitempty" json:"model_overrides,omitempty"`
}

var defaultProfileLadders = map[string][]string{
	"default": {"cheap", "standard", "smart"},
	"cheap":   {"cheap"},
	"fast":    {"fast", "smart"},
	"smart":   {"smart"},
}

// DefaultProfileLadders returns the built-in profile escalation policy.
func DefaultProfileLadders() map[string][]string {
	out := make(map[string][]string, len(defaultProfileLadders))
	for profile, ladder := range defaultProfileLadders {
		out[profile] = append([]string(nil), ladder...)
	}
	return out
}

// ResolvedLadder returns the escalation ladder for the named profile. If
// ProfileLadders contains an entry for profile, that wins. Otherwise falls
// back to the deprecated ProfilePriority for the default profile only. If
// neither is set, returns the shipped FEAT-006 profile ladder.
func (r *RoutingConfig) ResolvedLadder(profile string) []string {
	if profile == "" {
		profile = "default"
	}
	if r == nil {
		if ladder, ok := defaultProfileLadders[profile]; ok {
			return append([]string(nil), ladder...)
		}
		return append([]string(nil), defaultProfileLadders["default"]...)
	}
	if r.ProfileLadders != nil {
		if ladder, ok := r.ProfileLadders[profile]; ok && len(ladder) > 0 {
			return append([]string(nil), ladder...)
		}
	}
	if profile == "default" && len(r.ProfilePriority) > 0 {
		return append([]string(nil), r.ProfilePriority...)
	}
	if ladder, ok := defaultProfileLadders[profile]; ok {
		return append([]string(nil), ladder...)
	}
	return append([]string(nil), defaultProfileLadders["default"]...)
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
