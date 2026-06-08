package agent

import (
	"bytes"
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkRepairsCoreBareAndContinuesSameCandidate verifies that the worker
// repairs a corrupted project-root git config before dispatching the first
// bead and completes that bead instead of failing over to the next one.
func TestWorkRepairsCoreBareAndContinuesSameCandidate(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 2)
	runGitInteg(t, projectRoot, "config", "core.bare", "true")
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init(context.Background()))

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{})
	runner := NewRunner(Config{})
	attemptCfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).Resolve(config.CLIOverrides{Harness: "script"})

	var logBuf bytes.Buffer
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", attemptCfg, ExecuteBeadRuntime{
		AgentRunner: runner,
		Output:      &logBuf,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "ddx-int-0001", res.BeadID)

	gotFirst, err := store.Get(context.Background(), "ddx-int-0001")
	require.NoError(t, err)
	gotSecond, err := store.Get(context.Background(), "ddx-int-0002")
	require.NoError(t, err)
	assert.Equal(t, "open", gotSecond.Status)
	assert.Empty(t, gotSecond.Owner)
	assert.NotEmpty(t, res.BaseRev)
	assert.Empty(t, gotFirst.Owner)
	_, bareErr := runGitIntegOutput(projectRoot, "config", "--local", "--get", "core.bare")
	assert.Error(t, bareErr)
}

// TestWorkGitRepairFailureStopsBeforeSecondClaim verifies that an unresolved
// pre-dispatch git repair failure releases the claimed bead and stops the
// drain before the next ready bead is claimed.
func TestWorkGitRepairFailureStopsBeforeSecondClaim(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 2)
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init(context.Background()))
	claimStore := &claimCountingStore{Store: store}

	worker := &ExecuteBeadWorker{
		Store: claimStore,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusExecutionFailed,
				Error:  "pre-dispatch git repair failed: fatal: this operation must be run in a work tree",
			}, nil
		}),
	}

	loopCfg := config.NewTestConfigForLoop(config.TestLoopConfigOpts{Assignee: "worker"}).Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{Assignee: "worker"}))
	result, err := worker.Run(context.Background(), loopCfg, ExecuteBeadLoopRuntime{
		ProjectRoot: projectRoot,
		SessionID:   "sess-core-bare-failure",
		WorkerID:    "worker-core-bare-failure",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.OperatorAttention)
	assert.Equal(t, "pre_dispatch_git_repair_failed", result.OperatorAttention.Reason)
	assert.Equal(t, int32(1), atomic.LoadInt32(&claimStore.claimCalls))

	gotFirst, err := store.Get(context.Background(), "ddx-int-0001")
	require.NoError(t, err)
	gotSecond, err := store.Get(context.Background(), "ddx-int-0002")
	require.NoError(t, err)
	assert.Equal(t, "open", gotFirst.Status)
	assert.Equal(t, "open", gotSecond.Status)
	assert.Empty(t, gotFirst.Owner)
	assert.Empty(t, gotSecond.Owner)
}
