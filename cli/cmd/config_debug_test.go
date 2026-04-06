package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigResourcesDebug tests if config loading works correctly
func TestConfigResourcesDebug(t *testing.T) {
	// Use TestEnvironment for isolated testing
	env := NewTestEnvironment(t)

	// Create simple config (without resources since we don't support them in new format)
	configContent := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/DocumentDrivenDX/ddx-library
    branch: main
persona_bindings:
  project_name: "test-project"
`
	env.CreateConfig(configContent)

	// Load config and verify it works
	cfg, err := env.LoadConfig()
	require.NoError(t, err, "Config should load successfully")
	require.NotNil(t, cfg, "Config should not be nil")

	t.Logf("Config version: %s", cfg.Version)
	if cfg.Library != nil {
		t.Logf("Config library base path: %s", cfg.Library.Path)
	}
	t.Logf("Config persona bindings: %+v", cfg.PersonaBindings)

	// Verify basic config is loaded correctly
	assert.Equal(t, "1.0", cfg.Version, "Version should be loaded")
	assert.NotNil(t, cfg.Library, "Library should be loaded")
	if cfg.Library != nil {
		assert.Equal(t, ".ddx/plugins/ddx", cfg.Library.Path, "Library base path should be loaded")
	}
	assert.NotNil(t, cfg.PersonaBindings, "PersonaBindings should be loaded")
}

// TestBasicConfigRepository tests repository configuration
func TestBasicConfigRepository(t *testing.T) {
	// Use TestEnvironment for isolated testing
	env := NewTestEnvironment(t)

	// Create config with repository settings
	configContent := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/DocumentDrivenDX/ddx-library
    branch: main
persona_bindings:
  project_name: "ddx-test"
`
	env.CreateConfig(configContent)

	// Load config and verify repository settings
	cfg, err := env.LoadConfig()
	require.NoError(t, err, "Config should load successfully")
	require.NotNil(t, cfg, "Config should not be nil")
	require.NotNil(t, cfg.Library, "Library should be loaded")
	require.NotNil(t, cfg.Library.Repository, "Repository should be loaded")

	// Verify repository configuration
	assert.Equal(t, "https://github.com/DocumentDrivenDX/ddx-library", cfg.Library.Repository.URL, "Repository URL should be loaded")
	assert.Equal(t, "main", cfg.Library.Repository.Branch, "Repository branch should be loaded")
}
