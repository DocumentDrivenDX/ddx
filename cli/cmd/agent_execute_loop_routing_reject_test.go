package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteLoopLocalRejectsProfileLadders verifies AC #4 of bead ddx-3bd7396a:
// a project config carrying agent.routing.profile_ladders causes ddx work
// --once --local to fail with a hard error naming the removed field.
func TestExecuteLoopLocalRejectsProfileLadders(t *testing.T) {
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
agent:
  routing:
    profile_ladders:
      default: [cheap, standard, smart]
`), 0o644))

	factory := NewCommandFactory(dir)
	root := factory.NewRootCommand()
	_, err := executeCommand(root, "agent", "execute-loop", "--local", "--json", "--once")
	require.Error(t, err, "execute-loop must fail when config has profile_ladders")
	// routinglint:legacy-rejection reason="asserts the rejection error names the retired field"
	assert.Contains(t, err.Error(), "profile_ladders")
}

// TestExecuteLoopLocalRejectsModelOverrides verifies AC #4 of bead ddx-3bd7396a:
// a project config carrying agent.routing.model_overrides causes ddx work
// --once --local to fail with a hard error naming the removed field.
func TestExecuteLoopLocalRejectsModelOverrides(t *testing.T) {
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
agent:
  routing:
    model_overrides:
      cheap: qwen/qwen3-27b
`), 0o644))

	factory := NewCommandFactory(dir)
	root := factory.NewRootCommand()
	_, err := executeCommand(root, "agent", "execute-loop", "--local", "--json", "--once")
	require.Error(t, err, "execute-loop must fail when config has model_overrides")
	// routinglint:legacy-rejection reason="asserts the rejection error names the retired field"
	assert.True(t, strings.Contains(err.Error(), "model_overrides"), "error must name the field: %v", err)
}
