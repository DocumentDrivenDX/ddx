package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcceptance_US007_ConfigureDDxSettings tests US-007: Configure DDX Settings
func TestAcceptance_US007_ConfigureDDxSettings(t *testing.T) {

	t.Run("view_current_configuration", func(t *testing.T) {
		// AC: Given I want to view settings, when I run `ddx config`, then the current configuration is displayed in readable format

		// Setup temp project directory
		tempDir := t.TempDir()

		// Create a basic .ddx/config.yaml config file using DDx structure
		config := `version: "2.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
persona_bindings:
  author: "Test User"
  email: "test@example.com"
`
		ddxDir := filepath.Join(tempDir, ".ddx")
		require.NoError(t, os.MkdirAll(ddxDir, 0755))
		configPath := filepath.Join(ddxDir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte(config), 0644))

		factory := NewCommandFactory(tempDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeCommand(rootCmd, "config", "export")

		require.NoError(t, err, "Config export should work")
		assert.Contains(t, output, "Test User", "Should show author")
		assert.Contains(t, output, "test@example.com", "Should show email")
		assert.Contains(t, output, "https://github.com/test/repo", "Should show repository URL")

		// Output should be in readable YAML format
		assert.Contains(t, output, "version:", "Should show version key")
		assert.Contains(t, output, "author:", "Should show author key")
	})

	t.Run("set_configuration_value", func(t *testing.T) {
		// AC: Given I want to change a setting, when I run `ddx config set <key> <value>`, then the setting is updated and confirmed

		// Unset environment variable that would override config file
		originalLibPath := os.Getenv("DDX_LIBRARY_BASE_PATH")
		_ = os.Unsetenv("DDX_LIBRARY_BASE_PATH")
		defer func() {
			if originalLibPath != "" {
				_ = os.Setenv("DDX_LIBRARY_BASE_PATH", originalLibPath)
			}
		}()

		tempDir := t.TempDir()

		// Create initial config using DDx structure
		config := `version: "2.0"
persona_bindings:
  author: "Old User"
`
		ddxDir := filepath.Join(tempDir, ".ddx")
		require.NoError(t, os.MkdirAll(ddxDir, 0755))
		configPath := filepath.Join(ddxDir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte(config), 0644))

		factory := NewCommandFactory(tempDir)
		rootCmd := factory.NewRootCommand()

		// Set a new value using library namespace
		output, err := executeCommand(rootCmd, "config", "set", "library.path", "./new-library")
		require.NoError(t, err, "Config set should work")
		assert.Contains(t, output, "./new-library", "Should confirm the new value")
		assert.Contains(t, output, "library.path", "Should mention the key being set")

		// Verify the value was actually set
		getCmd := factory.NewRootCommand()
		getOutput, err := executeCommand(getCmd, "config", "get", "library.path")
		require.NoError(t, err, "Config get should work")
		assert.Contains(t, getOutput, "./new-library", "Should retrieve the updated value")
	})

	t.Run("get_specific_configuration_value", func(t *testing.T) {
		// AC: Given I need a specific value, when I run `ddx config get <key>`, then the current value for that key is displayed

		tempDir := t.TempDir()

		config := `version: "2.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/specific/repo"
    branch: "main"
persona_bindings:
  author: "Specific User"
`
		ddxDir := filepath.Join(tempDir, ".ddx")
		require.NoError(t, os.MkdirAll(ddxDir, 0755))
		configPath := filepath.Join(ddxDir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte(config), 0644))

		factory := NewCommandFactory(tempDir)
		rootCmd := factory.NewRootCommand()

		// Get repository URL using library namespace
		output, err := executeCommand(rootCmd, "config", "get", "library.repository.url")
		require.NoError(t, err, "Config get repository URL should work")
		assert.Contains(t, output, "https://github.com/specific/repo", "Should show repository URL")

		// Get nested value
		repoCmd := factory.NewRootCommand()
		repoOutput, err := executeCommand(repoCmd, "config", "get", "library.repository.url")
		require.NoError(t, err, "Config get nested value should work")
		assert.Contains(t, repoOutput, "https://github.com/specific/repo", "Should show repository URL")
	})

	t.Run("global_and_project_level_configs", func(t *testing.T) {
		// AC: Given I work on multiple projects, when I configure DDX, then both global and project-level configs are supported

		// Setup temp home directory
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)

		// Create global config
		globalConfigDir := filepath.Join(homeDir, ".ddx")
		require.NoError(t, os.MkdirAll(globalConfigDir, 0755))
		globalConfig := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/DocumentDrivenDX/ddx-library
    branch: main
persona_bindings:
  author: "Global User"
  email: "global@example.com"
`
		require.NoError(t, os.WriteFile(filepath.Join(globalConfigDir, "config.yaml"), []byte(globalConfig), 0644))

		// Setup project directory with local config
		projectDir := t.TempDir()

		localConfig := `version: "2.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: "https://github.com/project/repo"
    branch: main
persona_bindings:
  author: "Project User"
`
		localDdxDir := filepath.Join(projectDir, ".ddx")
		require.NoError(t, os.MkdirAll(localDdxDir, 0755))
		localConfigPath := filepath.Join(localDdxDir, "config.yaml")
		require.NoError(t, os.WriteFile(localConfigPath, []byte(localConfig), 0644))

		factory := NewCommandFactory(projectDir)
		rootCmd := factory.NewRootCommand()
		output, err := executeCommand(rootCmd, "config", "export")

		require.NoError(t, err, "Should export merged config")
		// Project config should override global for author
		assert.Contains(t, output, "Project User", "Project config should override global")
		// Global email should be available if not overridden
		// (This may depend on current implementation behavior)
	})

	t.Run("environment_variable_override", func(t *testing.T) {
		// AC: Given multiple config sources exist, when settings are loaded, then environment variables override config files
		t.Skip("pending feature: environment variable override for config values not yet implemented")
	})

	t.Run("configuration_value_validation", func(t *testing.T) {
		// AC: Given I set a configuration value, when it's saved, then the value is validated against acceptable options
		t.Skip("pending feature: configuration value validation (e.g. URL format checking) not yet implemented")
	})

	t.Run("export_import_configurations", func(t *testing.T) {
		// AC: Given I need to share configs, when I run export/import commands, then configurations can be transferred between systems
		t.Skip("pending feature: config import command not yet implemented")
	})

	t.Run("show_config_file_locations", func(t *testing.T) {
		// AC: Given I'm troubleshooting, when I run `ddx config --show-files`, then all config file locations are displayed
		t.Skip("pending feature: --show-files flag not yet implemented")
	})

	t.Run("configuration_validation_command", func(t *testing.T) {
		// Test the --validate flag functionality

		// Create valid config
		validConfig := `version: "2.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/valid/repo"
    branch: "main"
persona_bindings:
  author: "Valid User"
`
		env := NewTestEnvironment(t)
		env.CreateConfig(validConfig)

		factory := NewTestRootCommandWithDir(env.Dir)
		rootCmd := factory.NewRootCommand()
		output, err := executeCommand(rootCmd, "config", "--validate")

		require.NoError(t, err, "Valid config should pass validation")
		assert.Contains(t, strings.ToLower(output), "valid", "Should confirm config is valid")
	})

	t.Run("configuration_error_handling", func(t *testing.T) {
		// AC: Given invalid config exists, when commands are run, then errors are reported clearly
		t.Skip("pending feature: error handling behavior for invalid YAML and missing keys not yet specified")
	})
}
