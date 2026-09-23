package agent

import (
	"context"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteBeadWorkerNoChangesVerifiedLongCommandCloses(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-longverify", Title: "Verification gate needs time"}
	require.NoError(t, store.Create(context.Background(), b))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				NoChangesRationale: "verification_command: sh -lc 'sleep 0.05'",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                     "worker",
		NoChangesVerificationTimeout: time.Second,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var sawVerified, sawTerminal bool
	for _, ev := range events {
		if ev.Kind == NoChangesEventVerified {
			sawVerified = true
			assert.Contains(t, ev.Body, "exit_code=0")
			assert.Contains(t, ev.Body, "verification_command=sh -lc 'sleep 0.05'")
		}
		if ev.Summary == ExecuteBeadStatusAlreadySatisfied {
			sawTerminal = true
		}
	}
	assert.True(t, sawVerified, "no_changes_verified event must be emitted")
	assert.True(t, sawTerminal, "already_satisfied close must be recorded")
}

// TestExecuteBeadWorkerNoChangesVerificationTimeoutKeepsOpenAndReaps lives in
// execute_bead_no_changes_verification_timeout_linux_test.go: it asserts on
// /proc-derived zombie/dead process state via processDeadOrZombieStatus,
// which is Linux-only (see process_dead_or_zombie_linux_test.go).
