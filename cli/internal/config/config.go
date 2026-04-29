package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// routingDeprecationWarnOnce ensures the deprecated profile_priority +
// opt-in (profile_ladders / model_overrides) warnings fire only once per
// process even when LoadWithWorkingDir is called many times. Each key
// gets its own sync.Once.
var (
	routingDeprecationWarnOnce sync.Map // key string -> *sync.Once
)

func warnRoutingOnce(key, msg string) {
	v, _ := routingDeprecationWarnOnce.LoadOrStore(key, &sync.Once{})
	v.(*sync.Once).Do(func() {
		fmt.Fprintln(os.Stderr, msg)
	})
}

// ResetRoutingDeprecationWarnings clears the one-time warning latches.
// Tests call this so each test case can observe the warning fresh.
func ResetRoutingDeprecationWarnings() {
	routingDeprecationWarnOnce = sync.Map{}
}

// RoutingMigrationError signals that the loaded config carries the
// removed agent.routing.default_harness field (bead ddx-87fb72c2). The
// field is gone for good; configs must be migrated before DDx will
// load them.
type RoutingMigrationError struct {
	Field string
	Path  string
}

func (e *RoutingMigrationError) Error() string {
	return fmt.Sprintf(
		"%s: %s has been removed. "+
			"Migration: delete the field. The top-level agent.harness is a "+
			"tie-break preference, not a default override. "+
			"See docs/migrations/routing-config.md.",
		e.Path, e.Field)
}

// checkRoutingMigration parses the raw config bytes and reports the
// breaking change cases for agent.routing. Returns a hard error when
// agent.routing.default_harness is present; emits a one-time process
// warning when profile_ladders or model_overrides are set (because
// those fields are now opt-in via --escalate / --override-model).
func checkRoutingMigration(path string, data []byte) error {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil // YAML errors surface from the validator below
	}
	agent, _ := raw["agent"].(map[string]any)
	if agent == nil {
		return nil
	}
	routing, _ := agent["routing"].(map[string]any)
	if routing == nil {
		return nil
	}
	if _, ok := routing["default_harness"]; ok {
		return &RoutingMigrationError{
			Field: "agent.routing.default_harness",
			Path:  path,
		}
	}
	if _, ok := routing["profile_ladders"]; ok {
		warnRoutingOnce("profile_ladders",
			"warning: agent.routing.profile_ladders is opt-in. "+
				"It is consulted only when --escalate is passed; "+
				"the default execute path ignores it. "+
				"See docs/migrations/routing-config.md.")
	}
	if _, ok := routing["model_overrides"]; ok {
		warnRoutingOnce("model_overrides",
			"warning: agent.routing.model_overrides is opt-in. "+
				"It is consulted only when --override-model is passed; "+
				"the default execute path ignores it. "+
				"See docs/migrations/routing-config.md.")
	}
	return nil
}

// Type aliases for smooth transition
type Config = NewConfig
type Repository = RepositoryConfig

// ConfigError represents a single configuration error
type ConfigError struct {
	Field      string
	Value      string
	Message    string
	Suggestion string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationError represents multiple configuration errors
type ValidationError struct {
	Errors []*ConfigError
}

func (e *ValidationError) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	return fmt.Sprintf("%d configuration errors found", len(e.Errors))
}

// Load loads configuration using the new simplified approach
func Load() (*Config, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	return LoadWithWorkingDir(workingDir)
}

// LoadWithWorkingDir loads configuration from a specific working directory
func LoadWithWorkingDir(workingDir string) (*Config, error) {
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Use the new ConfigLoader to load from .ddx/config.yaml only
	loader, err := NewConfigLoaderWithWorkingDir(workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create config loader: %w", err)
	}

	// Pre-validation migration check: surface a friendly hard error for
	// removed-field cases (agent.routing.default_harness) and emit
	// one-time warnings for opt-in fields (profile_ladders,
	// model_overrides) before the schema validator sees the file. This
	// happens before LoadConfig so the migration message is what the
	// operator sees.
	configPath := filepath.Join(workingDir, ".ddx", "config.yaml")
	if data, readErr := os.ReadFile(configPath); readErr == nil {
		if migErr := checkRoutingMigration(configPath, data); migErr != nil {
			return nil, migErr
		}
	}

	config, err := loader.LoadConfig()
	if err != nil {
		// If no config file exists, return default config
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no configuration file found") {
			config = DefaultNewConfig()
		} else {
			return nil, err
		}
	}

	// Apply defaults to ensure complete configuration
	config.ApplyDefaults()
	if config.Agent != nil && config.Agent.Routing != nil &&
		len(config.Agent.Routing.ProfilePriority) > 0 {
		fmt.Fprintln(os.Stderr, "warning: agent.routing.profile_priority is deprecated; use agent.routing.profile_ladders.default instead")
	}

	// Override library path with environment variable if set
	if envLibraryPath := os.Getenv("DDX_LIBRARY_BASE_PATH"); envLibraryPath != "" {
		config.Library.Path = envLibraryPath
	}

	// Validate the final configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// Validate validates the configuration structure and values (simplified)
func (c *Config) Validate() error {
	var errors []*ConfigError

	// Validate version
	if c.Version == "" {
		errors = append(errors, &ConfigError{
			Field:      "version",
			Message:    "version is required",
			Suggestion: "add 'version: \"1.0\"' to your config",
		})
	}

	// Validate library configuration
	if c.Library != nil && c.Library.Repository != nil {
		if c.Library.Repository.URL == "" {
			errors = append(errors, &ConfigError{
				Field:      "library.repository.url",
				Message:    "library repository URL is required",
				Suggestion: "add a valid Git repository URL",
			})
		}

		if c.Library.Repository.Branch == "" {
			errors = append(errors, &ConfigError{
				Field:      "library.repository.branch",
				Message:    "library repository branch is required",
				Suggestion: "add 'branch: \"main\"' or another valid branch name",
			})
		}
	}

	if len(errors) > 0 {
		return &ValidationError{Errors: errors}
	}

	return nil
}

// Merge combines this config with another, with the other taking precedence
func (c *Config) Merge(other *Config) *Config {
	result := &Config{
		Version: c.Version,
	}

	// Copy library configuration from base
	if c.Library != nil {
		result.Library = &LibraryConfig{
			Path: c.Library.Path,
		}
		if c.Library.Repository != nil {
			result.Library.Repository = &RepositoryConfig{
				URL:    c.Library.Repository.URL,
				Branch: c.Library.Repository.Branch,
			}
		}
	}

	// Override with other's values
	if other.Version != "" {
		result.Version = other.Version
	}
	if other.Library != nil {
		if result.Library == nil {
			result.Library = &LibraryConfig{}
		}
		if other.Library.Path != "" {
			result.Library.Path = other.Library.Path
		}
		if other.Library.Repository != nil {
			if result.Library.Repository == nil {
				result.Library.Repository = &RepositoryConfig{}
			}
			if other.Library.Repository.URL != "" {
				result.Library.Repository.URL = other.Library.Repository.URL
			}
			if other.Library.Repository.Branch != "" {
				result.Library.Repository.Branch = other.Library.Repository.Branch
			}
		}
	}

	return result
}

// ResolveLibraryResource resolves a library resource path
// NOTE: This function is now legacy. New code should load config and use cfg.Library.Path directly.
func ResolveLibraryResource(resourcePath, configPath, workingDir string) (string, error) {
	// Load config to get the authoritative library path (which includes env var override)
	cfg, err := LoadWithWorkingDir(workingDir)
	if err == nil && cfg.Library != nil && cfg.Library.Path != "" {
		// Check if it's an absolute path
		if filepath.IsAbs(resourcePath) {
			return resourcePath, nil
		}

		// Try relative to library path from config
		configPath := filepath.Join(cfg.Library.Path, resourcePath)
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		// Return the config path even if it doesn't exist (for consistency)
		return configPath, nil
	}

	// Fall back to original logic if config loading fails
	if workingDir == "" {
		workingDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	// Check if it's an absolute path
	if filepath.IsAbs(resourcePath) {
		return resourcePath, nil
	}

	// Try relative to working directory first
	fullPath := filepath.Join(workingDir, resourcePath)
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath, nil
	}

	// Try relative to library directory
	libraryPath := filepath.Join(workingDir, "library", resourcePath)
	if _, err := os.Stat(libraryPath); err == nil {
		return libraryPath, nil
	}

	// Return the original path even if it doesn't exist
	return fullPath, nil
}

// LoadFromFile loads configuration from a specific file path
func LoadFromFile(configPath string) (*Config, error) {
	// Use ConfigLoader to load the file
	loader, err := NewConfigLoaderWithWorkingDir(filepath.Dir(configPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create config loader: %w", err)
	}
	return loader.LoadConfigFromPath(configPath)
}
