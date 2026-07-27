package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreDispatchDirtyGuard_IgnoresOtherLiveAttemptOutputs is the regression
// for ddx-8a936f36: when several attempts share one project worktree, a
// sibling's in-flight out-<beadID> file must not be treated as operator dirt
// and must not release the current bead's claim.
func TestPreDispatchDirtyGuard_IgnoresOtherLiveAttemptOutputs(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const currentBead = "ddx-int-0001"
	const otherBead = "ddxfixture-b70d1d65"

	// Sibling attempt output in the shared project worktree (fixture naming).
	outRel := "out-" + otherBead + ".txt"
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, outRel), []byte("sibling done\n"), 0o644))

	// Running-attempt registry entry for the sibling.
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       otherBead,
		AttemptID:    "sibling-attempt-1",
		StartedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
		WorktreePath: projectRoot,
		PID:          os.Getpid(),
	}))

	// Fresh claim lease for the sibling (second ownership source).
	leasePath := filepath.Join(bead.ClaimLivenessRoot(ddxroot.JoinProject(projectRoot)), otherBead+".json")
	require.NoError(t, os.MkdirAll(filepath.Dir(leasePath), 0o755))
	leaseData, err := json.Marshal(bead.ClaimLeaseRecord{
		BeadID:    otherBead,
		Owner:     "sibling-worker",
		UpdatedAt: time.Now().UTC(),
		StartedAt: time.Now().UTC(),
		PID:       os.Getpid(),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(leasePath, append(leaseData, '\n'), 0o644))

	// Production helper: sibling-owned path must not remain in blocked dirt.
	blocked := filterOtherLiveAttemptOwnedPaths(projectRoot, currentBead, []string{outRel, "cli/unrelated.go"})
	assert.Equal(t, []string{"cli/unrelated.go"}, blocked,
		"only unowned paths remain after filtering sibling live-attempt outputs")

	// Production checkpoint path used by ExecuteBead must not refuse.
	_, err = checkpointPreDispatchDirt(projectRoot, "attempt-current", currentBead)
	require.NoError(t, err, "checkpoint must ignore sibling live-attempt output")

	// Full ExecuteBead entrypoint (AC #3): pre-dispatch must not fail.
	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).
		Resolve(config.CLIOverrides{Harness: "script", Model: dirFile})
	res, execErr := ExecuteBeadWithConfig(context.Background(), projectRoot, currentBead, rcfg, ExecuteBeadRuntime{
		AgentRunner: scriptHarnessAgentRunner{},
	}, &RealGitOps{})
	require.NoError(t, execErr, "ExecuteBead must not fail pre-dispatch over sibling output")
	require.NotNil(t, res)
	assert.NotContains(t, res.Error, preDispatchCheckpointDirtyRefusalPrefix)
	assert.NotContains(t, res.Error, outRel)

	// Work-loop claim path: guard must not release the current bead.
	// Re-seed sibling dirt (ExecuteBead may have advanced the tree).
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, outRel), []byte("sibling still live\n"), 0o644))
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       otherBead,
		AttemptID:    "sibling-attempt-2",
		StartedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
		WorktreePath: projectRoot,
		PID:          os.Getpid(),
	}))

	// Fresh current bead for a clean claim (first may have been closed).
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	const loopBead = "ddx-int-loop-0002"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:        loopBead,
		Title:     "loop claim guard bead",
		IssueType: "task",
		Priority:  0,
	}))

	counting := &releaseCountingStore{claimCountingStore: &claimCountingStore{Store: store}}
	worker := &ExecuteBeadWorker{
		Store: counting,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			// Same production function ExecuteBead invokes at pre-dispatch.
			if _, err := checkpointPreDispatchDirt(projectRoot, "loop-attempt", beadID); err != nil {
				return ExecuteBeadReport{}, fmt.Errorf("pre-execute-bead checkpoint: %w", err)
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-sibling-ok",
				ResultRev: "rev-sibling-ok",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	loopCfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, runErr := worker.Run(context.Background(), loopCfg, ExecuteBeadLoopRuntime{
		Mode:        executeloop.ModeDrain,
		ProjectRoot: projectRoot,
		SessionID:   "sess-guard-ignore",
		WorkerID:    "worker-guard-ignore",
	})
	require.NoError(t, runErr)
	require.NotNil(t, result)
	assert.Nil(t, result.OperatorAttention,
		"sibling live-attempt output must not raise operator attention")
	assert.NotEqual(t, "OperatorAttention", result.StopCondition,
		"must not release the current bead's claim over a sibling attempt's output file")
	assert.GreaterOrEqual(t, result.Successes, 1)

	got, getErr := store.Get(context.Background(), loopBead)
	require.NoError(t, getErr)
	assert.Equal(t, bead.StatusClosed, got.Status,
		"current bead must complete successfully rather than being claim-released for sibling dirt")
}

// TestPreDispatchDirtyGuard_StillReleasesOnOperatorEdit proves the ownership
// filter narrowed the guard without disabling it: an unowned uncommitted
// implementation change still raises operator attention and releases the claim.
func TestPreDispatchDirtyGuard_StillReleasesOnOperatorEdit(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 2)
	const currentBead = "ddx-int-0001"

	implRel := filepath.ToSlash(filepath.Join("cli", "internal", "agent", "operator_edit.go"))
	implPath := filepath.Join(projectRoot, filepath.FromSlash(implRel))
	require.NoError(t, os.MkdirAll(filepath.Dir(implPath), 0o755))
	require.NoError(t, os.WriteFile(implPath, []byte("package agent\n"), 0o644))

	// No live attempt / claim owns this path — filter must leave it blocked.
	blocked := filterOtherLiveAttemptOwnedPaths(projectRoot, currentBead, []string{implRel})
	assert.Equal(t, []string{implRel}, blocked)

	// Production checkpoint refuses unowned implementation dirt.
	_, err := checkpointPreDispatchDirt(projectRoot, "attempt-operator", currentBead)
	require.Error(t, err)
	assert.Contains(t, err.Error(), preDispatchCheckpointDirtyRefusalPrefix)
	assert.Contains(t, err.Error(), implRel)

	// ExecuteBead production entrypoint surfaces the same refusal (AC #3).
	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).
		Resolve(config.CLIOverrides{Harness: "script", Model: dirFile})
	_, execErr := ExecuteBeadWithConfig(context.Background(), projectRoot, currentBead, rcfg, ExecuteBeadRuntime{
		AgentRunner: scriptHarnessAgentRunner{},
	}, &RealGitOps{})
	require.Error(t, execErr)
	assert.Contains(t, execErr.Error(), preExecuteCheckpointDirtyMarker)
	assert.Contains(t, execErr.Error(), implRel)

	// Work loop: unowned dirt still releases the claim and raises attention.
	inner := bead.NewStore(ddxroot.JoinProject(projectRoot))
	store := &releaseCountingStore{
		claimCountingStore: &claimCountingStore{Store: inner},
	}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			if _, err := checkpointPreDispatchDirt(projectRoot, "loop-attempt", beadID); err != nil {
				return ExecuteBeadReport{}, fmt.Errorf("pre-execute-bead checkpoint: %w", err)
			}
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusSuccess,
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	loopCfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, runErr := worker.Run(context.Background(), loopCfg, ExecuteBeadLoopRuntime{
		Mode:        executeloop.ModeDrain,
		ProjectRoot: projectRoot,
		SessionID:   "sess-guard-operator",
		WorkerID:    "worker-guard-operator",
	})
	require.NoError(t, runErr)
	require.NotNil(t, result)
	assert.Equal(t, "OperatorAttention", result.StopCondition)
	assert.Equal(t, "operator_attention", result.ExitReason)
	require.NotNil(t, result.OperatorAttention)
	assert.Equal(t, "checkpoint_dirty", result.OperatorAttention.Reason)
	assert.Contains(t, result.OperatorAttention.DirtyPaths, implRel)
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls),
		"queue must stop before claiming further beads while operator dirt remains")

	gotFirst, err := inner.Get(context.Background(), currentBead)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status,
		"unowned operator edit must release the claim back to open")
	assert.Empty(t, gotFirst.Owner, "released claim must clear owner")
}
