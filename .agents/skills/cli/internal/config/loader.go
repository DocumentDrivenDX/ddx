package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigLoader handles loading configuration files with validation
type ConfigLoader struct {
	validator  Validator
	workingDir string
}

// NewConfigLoader creates a new configuration loader with validation
func NewConfigLoader() (*ConfigLoader, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	return NewConfigLoaderWithWorkingDir(workingDir)
}

// NewConfigLoaderWithWorkingDir creates a new configuration loader with a specific working directory
func NewConfigLoaderWithWorkingDir(workingDir string) (*ConfigLoader, error) {
	validator, err := NewValidator()
	if err != nil {
		return nil, fmt.Errorf("failed to create validator: %w", err)
	}

	return &ConfigLoader{
		validator:  validator,
		workingDir: workingDir,
	}, nil
}

// LoadConfig loads configuration from .ddx/config.yaml only
func (cl *ConfigLoader) LoadConfig() (*NewConfig, error) {
	// Only support new format: .ddx/config.yaml
	configPath := filepath.Join(cl.workingDir, ".ddx", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		return nil, fmt.Errorf("no configuration file found at %s", configPath)
	}

	return cl.loadNewFormat(configPath)
}

// LoadConfigFromPath loads configuration from a specific path (new format only)
func (cl *ConfigLoader) LoadConfigFromPath(path string) (*NewConfig, error) {
	// Convert relative path to absolute based on working directory
	if !filepath.IsAbs(path) {
		path = filepath.Join(cl.workingDir, path)
	}

	return cl.loadNewFormat(path)
}

// loadNewFormat loads and validates the new configuration format
func (cl *ConfigLoader) loadNewFormat(path string) (*NewConfig, error) {
	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// Validate using two-phase validation
	if err := cl.validator.Validate(data); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Parse YAML into new config structure
	var config NewConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML from %s: %w", path, err)
	}

	// Apply defaults to missing fields
	config.ApplyDefaults()

	return &config, nil
}

// SaveConfig saves configuration in the new format
func (cl *ConfigLoader) SaveConfig(config *NewConfig, path string) error {
	// Convert relative path to absolute based on working directory
	if !filepath.IsAbs(path) {
		path = filepath.Join(cl.workingDir, path)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Validate before saving
	if err := cl.validator.Validate(data); err != nil {
		return fmt.Errorf("config validation failed before save: %w", err)
	}

	// Write file with secure permissions
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", path, err)
	}

	return nil
}

// DetectConfigFormat determines if .ddx/config.yaml exists in working directory
func (cl *ConfigLoader) DetectConfigFormat() (string, string, error) {
	configPath := filepath.Join(cl.workingDir, ".ddx", "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return "new", configPath, nil
	}

	return "none", "", fmt.Errorf("no configuration file found at %s", configPath)
}
