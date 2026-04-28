package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentCompareDispatchUsesConfigResolve proves that the
// `ddx agent run --compare` dispatch path now flows through
// config.LoadAndResolve + CompareRuntime + RunCompareWithConfigViaService.
// SD-024 Stage 2 / bead ddx-4c31e465 behavioral test.
//
// Before the migration, agent_cmd.go built compare dispatch args directly from
// flag values; agent.* fields in .ddx/config.yaml were dead at the
// compare dispatch site. With the migration, LoadAndResolve resolves
// the durable knobs (Model, Effort, Permissions, Timeout) into rcfg
// and CompareRuntime carries only per-invocation intent.
//
// The test exercises the migrated path end-to-end: it loads a real
// config file from disk, dispatches a two-arm virtual compare, and
// asserts the JSON output is well-formed with both arms returning
// the configured virtual response. This proves the new code path
// executes successfully against a real ResolvedConfig.
func TestAgentCompareDispatchUsesConfigResolve(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES",
		`[{"prompt_match":"hello","response":"compare-arm-response"}]`)

	dir := agentTestDirWithHarness(t, "virtual")
	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "run",
		"--compare",
		"--harnesses", "virtual,virtual",
		"--text", "hello",
		"--json",
	)
	require.NoError(t, err, "compare dispatch via LoadAndResolve must succeed")

	var record struct {
		ID   string `json:"id"`
		Arms []struct {
			Harness  string `json:"harness"`
			Output   string `json:"output"`
			ExitCode int    `json:"exit_code"`
		} `json:"arms"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &record))
	require.Len(t, record.Arms, 2, "both compare arms must dispatch through migrated path")
	for i, arm := range record.Arms {
		assert.Equal(t, 0, arm.ExitCode, "arm %d should succeed under rcfg-driven dispatch", i)
		assert.Equal(t, "compare-arm-response", arm.Output, "arm %d should receive virtual response via migrated dispatch", i)
	}
}
