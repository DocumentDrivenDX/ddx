package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentRunInvalidTimeoutReturnsError guards the exit-code semantic for
// invalid --timeout values. main.go turns any non-nil RunE error into exit 1,
// so asserting a non-nil error containing "invalid timeout" at the command
// layer is sufficient to catch the regression reported externally.
func TestAgentRunInvalidTimeoutReturnsError(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := agentTestDir(t)
	rootCmd := NewCommandFactory(dir).NewRootCommand()

	output, err := executeCommand(rootCmd, "agent", "run",
		"--timeout", "60ss",
		"--harness", "codex",
		"--text", "hello",
	)
	require.Error(t, err, "invalid --timeout must surface a non-nil error")
	assert.Contains(t, err.Error(), "invalid timeout")

	// SilenceUsage: true should suppress the Usage/Flags block on flag-parse-style errors.
	assert.NotContains(t, output, "Usage:", "SilenceUsage should suppress usage block")
	assert.NotContains(t, output, "Flags:", "SilenceUsage should suppress flags block")
}
