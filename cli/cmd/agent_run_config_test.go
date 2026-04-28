package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentRunDispatchUsesConfigHarness proves that the `ddx agent run`
// dispatch path now flows through config.LoadAndResolve: with no
// --harness flag, the agent.harness value from .ddx/config.yaml drives
// the run. SD-024 Stage 2 / Bead ddx-c0ff5d3e behavioral test.
//
// Before the migration, agent_cmd.go built RunArgs directly from
// flags and the harness defaulted to "" (service-routed) when the user
// omitted --harness — config's agent.harness was dead. With the
// migration, LoadAndResolve resolves agent.harness into rcfg.Harness()
// and AgentRunRuntime carries only per-invocation intent, so the
// configured harness reaches RunViaService.
func TestAgentRunDispatchUsesConfigHarness(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES",
		`[{"prompt_match":"hello","response":"config-driven virtual response"}]`)

	// Config sets agent.harness: virtual; the test invokes `ddx agent run`
	// with NO --harness flag. The only way the virtual harness can be
	// selected is via config.LoadAndResolve threading agent.harness into
	// rcfg.Harness().
	dir := agentTestDirWithHarness(t, "virtual")
	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "run",
		"--text", "hello",
	)
	require.NoError(t, err, "config-resolved harness must drive the run path")
	assert.Equal(t, "config-driven virtual response", strings.TrimSpace(output),
		"agent.harness from .ddx/config.yaml must drive the run when --harness is omitted")
}
