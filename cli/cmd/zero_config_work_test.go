package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestZeroConfigWork_NoConfigDoesNotEmitUnderSpecified covers ddx-b790449b AC1
// and AC2: a project with .ddx/beads.jsonl but no .ddx/config.yaml must drain
// against the global agent config defaults instead of failing with the
// upstream "routing under-specified" error. The test sets HOME to a temp
// directory holding a minimal agent config and runs `ddx work --once --local`
// programmatically. It asserts:
//   - the command does not return "routing under-specified" (the regression
//     this hot-fix addresses)
//   - no .ddx/config.yaml is created as a side effect of the run
//
// Real provider dispatch is not exercised: the bead's actual execution may
// fail because the temp config points at an unreachable endpoint, but that
// failure is downstream of the routing decision and unrelated to AC1.
func TestZeroConfigWork_NoConfigDoesNotEmitUnderSpecified(t *testing.T) {
	projectDir := t.TempDir()
	ddxDir := filepath.Join(projectDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	// Single trivial open bead so the loop has something to route.
	beads := `{"id":"test-zero-cfg-1","title":"trivial","type":"task","status":"open","priority":3,"created_at":"2026-05-01T00:00:00Z","updated_at":"2026-05-01T00:00:00Z"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(beads), 0o644))

	// Synthesize a HOME with a minimal global agent config so the
	// providersHasProviders precheck passes. The provider points at a
	// localhost port no test will ever bind — actual dispatch will fail
	// but the routing-under-specified gate must not.
	homeDir := t.TempDir()
	agentCfgDir := filepath.Join(homeDir, ".config", "agent")
	require.NoError(t, os.MkdirAll(agentCfgDir, 0o755))
	agentCfg := `providers:
  testprov:
    type: lmstudio
    base_url: http://127.0.0.1:1
    api_key: test
    model: test-model
default_provider: testprov
`
	require.NoError(t, os.WriteFile(filepath.Join(agentCfgDir, "config.yaml"), []byte(agentCfg), 0o644))

	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))

	factory := NewCommandFactory(projectDir)
	root := factory.NewRootCommand()
	out, err := executeCommand(root, "work", "--local", "--once", "--project", projectDir)

	combined := out
	if err != nil {
		combined = combined + " | err=" + err.Error()
	}
	// AC1/AC2: the upstream "routing under-specified" message must not surface
	// for a config-less project when the global config has at least one
	// provider. If it does, the zero-config fall-through is broken.
	assert.NotContainsf(t, strings.ToLower(combined), "routing under-specified",
		"zero-config drain must not emit routing-under-specified; got: %s", combined)

	// AC1: ddx work must not write .ddx/config.yaml as a side effect.
	_, statErr := os.Stat(filepath.Join(ddxDir, "config.yaml"))
	assert.True(t, os.IsNotExist(statErr),
		"zero-config drain must not create .ddx/config.yaml (statErr=%v)", statErr)
}
