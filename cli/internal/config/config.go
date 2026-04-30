package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RoutingMigrationError signals that the loaded config carries a removed
// agent.routing field. The field is gone for good; configs must be migrated
// before DDx will load them.
type RoutingMigrationError struct {
	Field string
	Path  string
}

func (e *RoutingMigrationError) Error() string {
	return fmt.Sprintf(
		"%s: %s has been removed. "+
			"Migration: delete the field from your .ddx/config.yaml. "+
			"See docs/migrations/routing-config.md.",
		e.Path, e.Field)
}

// checkRoutingMigration parses the raw config bytes and hard-errors on any
// removed agent.routing fields.
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
	// routinglint:legacy-rejection reason="hard-error rejection of retired routing keys per ddx-3bd7396a"
	for _, field := range []string{"default_harness", "profile_ladders", "model_overrides"} {
		if _, ok := routing[field]; ok {
			return &RoutingMigrationError{
				Field: "agent.routing." + field,
				Path:  path,
			}
		}
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
	// removed agent.routing fields before the schema validator sees the file.
	// This happens before LoadConfig so the migration message is what the
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
		fmt.Fprintln(os.Stderr, "warning: agent.routing.profile_priority is deprecated and will be removed in a future version")
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
