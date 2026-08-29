package agent

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureHeadlessFizeauRoutingRestoresEnvironmentExactly(t *testing.T) {
	original, originallySet := os.LookupEnv(FizeauDisableClaudeTUIDefaultEnv)
	t.Cleanup(func() {
		if originallySet {
			require.NoError(t, os.Setenv(FizeauDisableClaudeTUIDefaultEnv, original))
			return
		}
		require.NoError(t, os.Unsetenv(FizeauDisableClaudeTUIDefaultEnv))
	})

	t.Run("unset", func(t *testing.T) {
		require.NoError(t, os.Unsetenv(FizeauDisableClaudeTUIDefaultEnv))
		restore, err := ConfigureHeadlessFizeauRouting(false)
		require.NoError(t, err)
		assert.Equal(t, "1", os.Getenv(FizeauDisableClaudeTUIDefaultEnv))
		restore()
		_, set := os.LookupEnv(FizeauDisableClaudeTUIDefaultEnv)
		assert.False(t, set)
	})

	t.Run("prior value", func(t *testing.T) {
		require.NoError(t, os.Setenv(FizeauDisableClaudeTUIDefaultEnv, "operator-value"))
		restore, err := ConfigureHeadlessFizeauRouting(false)
		require.NoError(t, err)
		assert.Equal(t, "1", os.Getenv(FizeauDisableClaudeTUIDefaultEnv))
		restore()
		value, set := os.LookupEnv(FizeauDisableClaudeTUIDefaultEnv)
		assert.True(t, set)
		assert.Equal(t, "operator-value", value)
	})
}

func TestExecuteOnServiceHeadlessHarnessGuardAtDispatchBoundary(t *testing.T) {
	previousDetector := interactiveTerminalDetector
	interactiveTerminalDetector = func() bool { return false }
	t.Cleanup(func() { interactiveTerminalDetector = previousDetector })

	t.Run("claude tui rejected before dispatch", func(t *testing.T) {
		svc := &passthroughTestService{}
		rcfg := resolvedWithPassthrough("claude-tui", "", "", 0, 0)

		_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
			Prompt: "do the work",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a PTY-capable interactive terminal")
		assert.Contains(t, err.Error(), "no controlling terminal is attached")
		assert.False(t, svc.executeCalled, "headless claude-tui must fail before Fizeau Execute")
	})

	t.Run("non tui pin still dispatches", func(t *testing.T) {
		svc := &passthroughTestService{}
		rcfg := resolvedWithPassthrough("codex", "", "", 0, 0)

		_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
			Prompt: "do the work",
		})

		require.NoError(t, err)
		assert.True(t, svc.executeCalled)
		assert.Equal(t, "codex", svc.lastReq.Harness)
	})
}
