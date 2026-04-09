package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestDefaultConfig validates the default configuration values
func TestDefaultConfig_Basic(t *testing.T) {
	t.Parallel()
	config := DefaultNewConfig()

	assert.Equal(t, "1.0", config.Version)
	assert.Equal(t, ".ddx/plugins/ddx", config.Library.Path)
	assert.Equal(t, "https://github.com/DocumentDrivenDX/ddx-library", config.Library.Repository.URL)
	assert.Equal(t, "main", config.Library.Repository.Branch)
	assert.Empty(t, config.PersonaBindings)
}

// TestLoadConfig_DefaultOnly tests loading when no config files exist
func TestLoadConfig_DefaultOnly_Basic(t *testing.T) {
	// Create temp directory without config files
	tempDir := t.TempDir()

	// Isolate from global config by setting temporary HOME
	t.Setenv("HOME", tempDir)

	config, err := LoadWithWorkingDir(tempDir)

	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, DefaultConfig.Version, config.Version)
	assert.Equal(t, DefaultConfig.Library.Repository.URL, config.Library.Repository.URL)
}

// TestLoadConfig_LocalConfig tests loading with local .ddx.yml
func TestLoadConfig_LocalConfig_Basic(t *testing.T) {
	tempDir := t.TempDir()

	// Create local config
	localConfig := &Config{
		Version: "2.0",
		Library: &LibraryConfig{
			Path: "./custom-library",
			Repository: &RepositoryConfig{
				URL:    "https://github.com/custom/repo",
				Branch: "develop",
			},
		},
		PersonaBindings: map[string]string{
			"test-role": "test-persona",
		},
	}

	configData, err := yaml.Marshal(localConfig)
	require.NoError(t, err)

	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	configPath := filepath.Join(ddxDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, configData, 0644))

	// Load config
	config, err := LoadWithWorkingDir(tempDir)

	require.NoError(t, err)
	assert.Equal(t, "2.0", config.Version)
	assert.Equal(t, "https://github.com/custom/repo", config.Library.Repository.URL)
	assert.Equal(t, "develop", config.Library.Repository.Branch)
	assert.Contains(t, config.PersonaBindings, "test-role")
}

// TestLoadLocal tests LoadLocal function
func TestLoadLocal_Basic(t *testing.T) {
	tempDir := t.TempDir()

	// Create local config
	localConfig := &Config{
		Version: "1.5",
		Library: &LibraryConfig{
			Path: "./library",
			Repository: &RepositoryConfig{
				URL:    "https://github.com/local/repo",
				Branch: "feature",
			},
		},
		PersonaBindings: map[string]string{
			"test_var": "test_value",
		},
	}

	configData, err := yaml.Marshal(localConfig)
	require.NoError(t, err)

	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	configPath := filepath.Join(ddxDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, configData, 0644))

	// Load local config
	config, err := LoadWithWorkingDir(tempDir)

	require.NoError(t, err)
	assert.Equal(t, "1.5", config.Version)
	assert.Equal(t, "https://github.com/local/repo", config.Library.Repository.URL)
	assert.Equal(t, "test_value", config.PersonaBindings["test_var"])
}

// TestSaveLocal tests SaveLocal function
func TestSaveLocal_Basic(t *testing.T) {
	tempDir := t.TempDir()

	config := &Config{
		Version: "1.0",
		Library: &LibraryConfig{
			Repository: &RepositoryConfig{
				URL:    "https://github.com/test/repo",
				Branch: "main",
			},
		},
		PersonaBindings: map[string]string{
			"key1": "value1",
		},
	}

	// Save config locally in new format
	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	configPath := filepath.Join(ddxDir, "config.yaml")
	configData, err := yaml.Marshal(config)
	require.NoError(t, err)
	err = os.WriteFile(configPath, configData, 0644)
	require.NoError(t, err)

	// Verify file was created
	assert.FileExists(t, configPath)

	// Load and verify
	loadedConfig, err := LoadWithWorkingDir(tempDir)
	require.NoError(t, err)

	assert.Equal(t, config.Version, loadedConfig.Version)
	assert.Equal(t, config.Library.Repository.URL, loadedConfig.Library.Repository.URL)
	assert.Equal(t, "value1", loadedConfig.PersonaBindings["key1"])
}

// TestReplaceVariables tests the ReplaceVariables method
func TestReplaceVariables_Basic(t *testing.T) {
	t.Parallel()
	// NOTE: ReplaceVariables method doesn't exist in new config - removing test
	// This functionality may be handled differently in the new system
	t.Skip("ReplaceVariables method not implemented in new config system")
}

// TestLoadConfig_InvalidYAML tests handling of invalid YAML
func TestLoadConfig_InvalidYAML_Basic(t *testing.T) {
	tempDir := t.TempDir()

	// Create invalid YAML file in new format location
	invalidYAML := `
version: 1.0
repository:
  url: https://github.com/test
  branch: [this is invalid
`
	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	configPath := filepath.Join(ddxDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(invalidYAML), 0644))

	// Should return error
	config, err := LoadWithWorkingDir(tempDir)

	assert.Error(t, err)
	assert.Nil(t, config)
}

func TestLoadConfig_AgentCapabilitiesFields(t *testing.T) {
	tempDir := t.TempDir()

	content := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  harness: codex
  model: o3-mini
  models:
    claude: claude-sonnet-4-20250514
  reasoning_levels:
    codex:
      - low
      - medium
      - high
`

	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(content), 0644))

	cfg, err := LoadWithWorkingDir(tempDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Agent)
	assert.Equal(t, "o3-mini", cfg.Agent.Model)
	assert.Equal(t, "claude-sonnet-4-20250514", cfg.Agent.Models["claude"])
	assert.Equal(t, []string{"low", "medium", "high"}, cfg.Agent.ReasoningLevels["codex"])
}

func TestSchemaValidation_AgentVirtualNormalize(t *testing.T) {
	t.Parallel()
	validator, err := NewValidator()
	require.NoError(t, err)

	content := []byte(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  virtual:
    normalize:
      - pattern: "foo.*bar"
        replace: "baz"
      - pattern: "\\d{4}-\\d{2}-\\d{2}"
        replace: "<date>"
`)
	err = validator.Validate(content)
	assert.NoError(t, err, "config with agent.virtual.normalize should pass schema validation")
}

func TestSchemaValidation_ServerSection(t *testing.T) {
	t.Parallel()
	validator, err := NewValidator()
	require.NoError(t, err)

	content := []byte(`version: "1.0"
server:
  addr: ":8080"
  tsnet:
    enabled: true
    hostname: "ddx-server"
    auth_key: "tskey-auth-xxx"
    state_dir: "/var/lib/ddx/tsnet"
`)
	err = validator.Validate(content)
	assert.NoError(t, err, "config with server.addr and server.tsnet fields should pass schema validation")
}

func TestLoadConfig_BeadPrefixField(t *testing.T) {
	tempDir := t.TempDir()

	content := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
bead:
  id_prefix: "nif"
`

	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(content), 0o644))

	cfg, err := LoadWithWorkingDir(tempDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Bead)
	assert.Equal(t, "nif", cfg.Bead.IDPrefix)
}
