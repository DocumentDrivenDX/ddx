package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
