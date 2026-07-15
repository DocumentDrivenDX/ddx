package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAgentConfigParsesEndpointBlocks(t *testing.T) {
	raw := `
endpoints:
  - type: lmstudio
    host: vidar
    port: 1234
    api_key: lmstudio
  - type: omlx
    base_url: http://vidar:1235/v1
`
	var cfg AgentConfig
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))

	require.Len(t, cfg.Endpoints, 2)
	assert.Equal(t, "lmstudio", cfg.Endpoints[0].Type)
	assert.Equal(t, "vidar", cfg.Endpoints[0].Host)
	assert.Equal(t, 1234, cfg.Endpoints[0].Port)
	assert.Equal(t, "lmstudio", cfg.Endpoints[0].APIKey)
	assert.Equal(t, "omlx", cfg.Endpoints[1].Type)
	assert.Equal(t, "http://vidar:1235/v1", cfg.Endpoints[1].BaseURL)
}

func TestLegacyProfilePriorityRejected(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  routing:
    # routinglint:legacy-rejection reason="migration fixture for retired routing field"
    profile_priority: [cheap, standard]
`), 0644))

	configPath := filepath.Join(ddxDir, "config.yaml")
	loader, err := NewConfigLoaderWithWorkingDir(tempDir)
	require.NoError(t, err)
	loaders := map[string]func() error{
		"LoadWithWorkingDir": func() error {
			_, err := LoadWithWorkingDir(tempDir)
			return err
		},
		"LoadConfig": func() error {
			_, err := loader.LoadConfig()
			return err
		},
		"LoadConfigFromPath": func() error {
			_, err := loader.LoadConfigFromPath(configPath)
			return err
		},
		"LoadFromFile": func() error {
			_, err := LoadFromFile(configPath)
			return err
		},
	}
	for name, load := range loaders {
		t.Run(name, func(t *testing.T) {
			loadErr := load()
			require.Error(t, loadErr)
			migErr, ok := loadErr.(*RoutingMigrationError)
			require.True(t, ok, "expected RoutingMigrationError, got %T: %v", loadErr, loadErr)
			// routinglint:legacy-rejection reason="asserts the migration error names the retired routing key"
			assert.Equal(t, "agent.routing.profile_priority", migErr.Field)
			assert.Equal(t, configPath, migErr.Path)
			assert.Contains(t, loadErr.Error(), "docs/migrations/routing-config.md")
		})
	}
}
