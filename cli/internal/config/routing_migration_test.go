package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadConfigHardErrorsOnDefaultHarness covers AC #1 of bead
// ddx-87fb72c2: a config carrying agent.routing.default_harness must
// fail to load with a migration message. The CLI surfaces the error to
// the user; the loader's exit code is non-zero (the *cobra.Command path
// returns the error to its caller).
func TestLoadConfigHardErrorsOnDefaultHarness(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  routing:
    default_harness: claude
`), 0644))

	_, err := LoadWithWorkingDir(tempDir)
	require.Error(t, err, "loading must fail when agent.routing.default_harness is present")

	migErr, ok := err.(*RoutingMigrationError)
	require.True(t, ok, "expected RoutingMigrationError, got %T: %v", err, err)
	assert.Equal(t, "agent.routing.default_harness", migErr.Field)

	msg := err.Error()
	assert.Contains(t, msg, "agent.routing.default_harness")
	assert.Contains(t, msg, "removed")
	assert.Contains(t, msg, "docs/migrations/routing-config.md")
}

// TestLoadConfigHardErrorsOnProfileLadders covers bead ddx-3bd7396a:
// a config carrying agent.routing.profile_ladders must fail to load.
func TestLoadConfigHardErrorsOnProfileLadders(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  routing:
    profile_ladders:
      default: [cheap, standard, smart]
`), 0644))

	_, err := LoadWithWorkingDir(tempDir)
	require.Error(t, err, "loading must fail when agent.routing.profile_ladders is present")

	migErr, ok := err.(*RoutingMigrationError)
	require.True(t, ok, "expected RoutingMigrationError, got %T: %v", err, err)
	assert.Equal(t, "agent.routing.profile_ladders", migErr.Field)

	msg := err.Error()
	assert.Contains(t, msg, "agent.routing.profile_ladders")
	assert.Contains(t, msg, "removed")
	assert.Contains(t, msg, "docs/migrations/routing-config.md")
}

// TestLoadConfigHardErrorsOnModelOverrides covers bead ddx-3bd7396a:
// a config carrying agent.routing.model_overrides must fail to load.
func TestLoadConfigHardErrorsOnModelOverrides(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  routing:
    model_overrides:
      cheap: qwen/qwen3.6
`), 0644))

	_, err := LoadWithWorkingDir(tempDir)
	require.Error(t, err, "loading must fail when agent.routing.model_overrides is present")

	migErr, ok := err.(*RoutingMigrationError)
	require.True(t, ok, "expected RoutingMigrationError, got %T: %v", err, err)
	assert.Equal(t, "agent.routing.model_overrides", migErr.Field)

	msg := err.Error()
	assert.Contains(t, msg, "agent.routing.model_overrides")
	assert.Contains(t, msg, "removed")
	assert.Contains(t, msg, "docs/migrations/routing-config.md")
}
