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
