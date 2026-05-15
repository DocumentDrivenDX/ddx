package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteBeadWithConfig_RecordsCandidateCycleMetadata(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"append-line output.txt candidate cycle integration",
		"commit chore: candidate cycle integration",
	})

	runner := NewRunner(Config{})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).Resolve(config.CLIOverrides{Harness: "script"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: runner,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, ExecuteBeadStatusSuccess, res.Status)
	require.NotEmpty(t, res.CandidateRef, "successful worker results must carry a pinned candidate ref")
	assert.Equal(t, 0, res.CycleIndex)
	require.Len(t, res.CycleTrace, 1, "the worker candidate cycle must record one initial implementation cycle")

	candidateRev := res.ImplementationRev
	if candidateRev == "" {
		candidateRev = res.ResultRev
	}
	got, err := gitRevParse(t, projectRoot, res.CandidateRef)
	require.NoError(t, err)
	assert.Equal(t, candidateRev, got, "candidate ref must remain reachable from the project root after the worktree is removed")
	assert.Equal(t, candidateRev, res.CycleTrace[0].ResultRev)
	assert.Equal(t, ExecuteBeadStatusSuccess, res.CycleTrace[0].FinalDecision)
}
