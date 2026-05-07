package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentRunDispatchUsesFlagHarness proves that harness selection remains a
// per-invocation passthrough. There is no durable agent.harness config field.
func TestAgentRunDispatchUsesFlagHarness(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES",
		`[{"prompt_match":"hello","response":"config-driven virtual response"}]`)

	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "run",
		"--harness", "virtual",
		"--text", "hello",
	)
	require.NoError(t, err, "flag-resolved harness must drive the run path")
	assert.Equal(t, "config-driven virtual response", strings.TrimSpace(output),
		"--harness must drive the run")
}
