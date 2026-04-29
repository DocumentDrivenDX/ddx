package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
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

// TestLoadConfigWarnsOnceForOptInRoutingFields covers AC #2: configs
// containing profile_ladders + model_overrides load successfully but
// emit a one-time process warning, AND the default execute path does
// not consult those fields. Consultation only happens when --escalate /
// --override-model are explicitly passed.
func TestLoadConfigWarnsOnceForOptInRoutingFields(t *testing.T) {
	ResetRoutingDeprecationWarnings()
	t.Cleanup(ResetRoutingDeprecationWarnings)

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
    model_overrides:
      cheap: qwen/qwen3.6
`), 0644))

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	cfg, loadErr := LoadWithWorkingDir(tempDir)
	require.NoError(t, loadErr)
	require.NotNil(t, cfg)

	// Calling load a second time must NOT re-emit the same warning.
	_, _ = LoadWithWorkingDir(tempDir)

	require.NoError(t, w.Close())
	os.Stderr = oldStderr
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	stderrStr := string(out)

	assert.Contains(t, stderrStr, "agent.routing.profile_ladders is opt-in")
	assert.Contains(t, stderrStr, "agent.routing.model_overrides is opt-in")
	assert.Contains(t, stderrStr, "--escalate")
	assert.Contains(t, stderrStr, "--override-model")
	// Second load does not duplicate the warning lines.
	assert.Equal(t, 1, strings.Count(stderrStr, "agent.routing.profile_ladders is opt-in"),
		"profile_ladders warning must fire once per process")
	assert.Equal(t, 1, strings.Count(stderrStr, "agent.routing.model_overrides is opt-in"),
		"model_overrides warning must fire once per process")

	// Config still parses these fields so opt-in flags can consult them.
	require.NotNil(t, cfg.Agent)
	require.NotNil(t, cfg.Agent.Routing)
	require.Contains(t, cfg.Agent.Routing.ProfileLadders, "default")
	assert.Equal(t, "qwen/qwen3.6", cfg.Agent.Routing.ModelOverrides["cheap"])
}
