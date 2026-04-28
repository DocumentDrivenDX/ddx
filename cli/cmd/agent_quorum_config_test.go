package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentQuorumDispatchUsesConfigResolve proves that the
// `ddx agent run --quorum` dispatch path now flows through
// config.LoadAndResolve + QuorumRuntime + RunQuorumWithConfigViaService.
// SD-024 Stage 2 / bead ddx-21f9e321 behavioral test.
//
// Before the migration, agent_cmd.go built quorum dispatch args directly from
// flag values; agent.* fields in .ddx/config.yaml were dead at the
// quorum dispatch site. With the migration, LoadAndResolve resolves
// the durable knobs (Model, Effort, Permissions, Timeout) into rcfg
// and QuorumRuntime carries only per-invocation intent.
func TestAgentQuorumDispatchUsesConfigResolve(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_VIRTUAL_RESPONSES",
		`[{"prompt_match":"hello","response":"quorum-arm-response"}]`)

	dir := agentTestDirWithHarness(t, "virtual")
	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "run",
		"--quorum", "unanimous",
		"--harnesses", "virtual,virtual",
		"--text", "hello",
		"--json",
	)
	require.NoError(t, err, "quorum dispatch via LoadAndResolve must succeed")

	var record struct {
		QuorumMet bool   `json:"quorum_met"`
		Strategy  string `json:"strategy"`
		Results   []struct {
			Harness  string `json:"harness"`
			Output   string `json:"output"`
			ExitCode int    `json:"exit_code"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &record))
	assert.True(t, record.QuorumMet, "unanimous quorum must be met under rcfg-driven dispatch")
	assert.Equal(t, "unanimous", record.Strategy)
	require.Len(t, record.Results, 2, "both quorum arms must dispatch through migrated path")
	for i, arm := range record.Results {
		assert.Equal(t, 0, arm.ExitCode, "arm %d should succeed under rcfg-driven dispatch", i)
		assert.Equal(t, "quorum-arm-response", arm.Output, "arm %d should receive virtual response via migrated dispatch", i)
	}
}
