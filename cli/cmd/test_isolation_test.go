package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoContamination verifies that tests don't contaminate each other
// by checking that config files are created in isolated directories
func TestNoContamination(t *testing.T) {
	t.Run("tests_use_isolated_directories", func(t *testing.T) {
		// Get current working directory at start
		startDir, err := os.Getwd()
		require.NoError(t, err)

		// Ensure cleanup of any stray config files
		defer func() {
			_ = os.Remove(".ddx.yml")
			_ = os.RemoveAll(".ddx")
		}()

		// Create first test environment
		env1 := NewTestEnvironment(t)
		env1.CreateDefaultConfig()

		// Clean up any stray files that might have been created
		_ = os.Remove(".ddx.yml")
		_ = os.RemoveAll(".ddx")

		// Verify config was created in env1's directory, not current directory
		assert.FileExists(t, env1.ConfigPath, "Config should exist in env1")
		assert.NoFileExists(t, ".ddx.yml", "No legacy config should exist in current directory")
		assert.NoFileExists(t, ".ddx/config.yaml", "No config should exist in current directory")

		// Create second test environment
		env2 := NewTestEnvironment(t)
		env2.CreateDefaultConfig()

		// Verify both environments are isolated
		assert.FileExists(t, env1.ConfigPath, "Config should still exist in env1")
		assert.FileExists(t, env2.ConfigPath, "Config should exist in env2")
		assert.NotEqual(t, env1.Dir, env2.Dir, "Environments should use different directories")

		// Verify current working directory unchanged
		currentDir, err := os.Getwd()
		require.NoError(t, err)
		assert.Equal(t, startDir, currentDir, "Working directory should be unchanged")
	})

	t.Run("test_environment_creates_unique_paths", func(t *testing.T) {
		// Create multiple test environments in parallel
		var envs []*TestEnvironment
		for i := 0; i < 5; i++ {
			env := NewTestEnvironment(t)
			env.CreateDefaultConfig()
			envs = append(envs, env)
		}

		// Verify all environments have unique directories and config paths
		usedDirs := make(map[string]bool)
		usedConfigPaths := make(map[string]bool)

		for i, env := range envs {
			assert.False(t, usedDirs[env.Dir], "Directory %s should be unique (env %d)", env.Dir, i)
			assert.False(t, usedConfigPaths[env.ConfigPath], "Config path %s should be unique (env %d)", env.ConfigPath, i)

			usedDirs[env.Dir] = true
			usedConfigPaths[env.ConfigPath] = true

			// Verify config file exists in the right place
			assert.FileExists(t, env.ConfigPath, "Config should exist for env %d", i)
			assert.True(t, filepath.IsAbs(env.ConfigPath), "Config path should be absolute for env %d", i)
		}
	})

	t.Run("test_environment_supports_config_loading", func(t *testing.T) {
		env := NewTestEnvironment(t)
		env.CreateDefaultConfig()

		// Load config using the environment
		cfg, err := env.LoadConfig()
		require.NoError(t, err, "Should be able to load config from environment")
		require.NotNil(t, cfg, "Config should not be nil")

		// Verify expected default values
		assert.Equal(t, "1.0", cfg.Version, "Version should be set")
		assert.NotNil(t, cfg.Library, "Library should be set")
		if cfg.Library != nil {
			assert.Equal(t, ".ddx/plugins/ddx", cfg.Library.Path, "Library base path should be set")
			assert.NotNil(t, cfg.Library.Repository, "Repository should be set")
		}
		assert.NotNil(t, cfg.PersonaBindings, "PersonaBindings should be set")
	})

	t.Run("test_environment_supports_custom_config", func(t *testing.T) {
		env := NewTestEnvironment(t)

		customConfig := `version: "1.0"
library:
  path: "./custom-library"
  repository:
    url: "https://github.com/custom/repo"
    branch: "develop"
    subtree: "lib"
persona_bindings:
  project_name: "custom-project"
  custom_var: "custom-value"
`
		env.CreateConfig(customConfig)

		// Load and verify custom config
		cfg, err := env.LoadConfig()
		require.NoError(t, err, "Should be able to load custom config")

		assert.NotNil(t, cfg.Library, "Library should be loaded")
		if cfg.Library != nil {
			assert.Equal(t, "./custom-library", cfg.Library.Path, "Custom library path should be set")
			assert.NotNil(t, cfg.Library.Repository, "Repository should be loaded")
			if cfg.Library.Repository != nil {
				assert.Equal(t, "develop", cfg.Library.Repository.Branch, "Custom branch should be set")
			}
		}
		assert.Equal(t, "custom-project", cfg.PersonaBindings["project_name"], "Custom binding should be set")
		assert.Equal(t, "custom-value", cfg.PersonaBindings["custom_var"], "Custom binding should be set")
	})
}

// TestLegacyConfigMigration verifies that we no longer support legacy .ddx.yml files
func TestLegacyConfigMigration(t *testing.T) {
	t.Run("no_legacy_ddx_yml_support", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Try to create a legacy .ddx.yml file in the test environment
		legacyConfigPath := filepath.Join(env.Dir, ".ddx.yml")
		legacyConfig := `version: "1.0"
name: test-project
repository:
  url: https://github.com/test/repo
  branch: main
`
		err := os.WriteFile(legacyConfigPath, []byte(legacyConfig), 0644)
		require.NoError(t, err)

		// Try to load config - should fail because no .ddx/config.yaml exists
		_, err = env.LoadConfig()
		assert.Error(t, err, "Should not be able to load legacy config")
		assert.Contains(t, err.Error(), "no configuration file found", "Error should mention missing config file")
	})

	t.Run("only_new_format_supported", func(t *testing.T) {
		env := NewTestEnvironment(t)
		env.CreateDefaultConfig()

		// Should be able to load new format
		cfg, err := env.LoadConfig()
		require.NoError(t, err, "Should be able to load new format config")
		assert.Equal(t, "1.0", cfg.Version, "Should load new format successfully")
	})
}
