package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
)

func runBlockedHeartbeatLoop(t *testing.T, store *bead.Store, heartbeatInterval time.Duration) (func(), <-chan error) {
	t.Helper()

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			select {
			case <-started:
			default:
				close(started)
			}
			<-release
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-heartbeat",
				ResultRev: "deadbeef",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:          "worker",
		HeartbeatInterval: heartbeatInterval,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	go func() {
		_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
		done <- err
	}()

	require.Eventually(t, func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	}, time.Second, 5*time.Millisecond, "worker never started")

	return func() {
		close(release)
	}, done
}

func TestWorkLoop_ClaimLivenessDoesNotMutateBeadsJSONL(t *testing.T) {
	store, _, _ := newExecuteLoopTestStore(t)

	release, done := runBlockedHeartbeatLoop(t, store, 5*time.Millisecond)

	before, err := os.ReadFile(store.File)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	after, err := os.ReadFile(store.File)
	require.NoError(t, err)

	require.Equal(t, before, after, "heartbeat refresh must not rewrite beads.jsonl")
	release()
	require.NoError(t, <-done)
}

func TestWorkLoop_LivenessRecordedOutsideTracker(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	release, done := runBlockedHeartbeatLoop(t, store, 5*time.Millisecond)

	trackerBefore, err := os.ReadFile(store.File)
	require.NoError(t, err)
	require.NoError(t, store.TouchClaimHeartbeat(first.ID))
	fresh, _, err := store.ClaimHeartbeatFresh(first.ID)
	require.NoError(t, err)
	require.True(t, fresh, "store must read the external claim lease as fresh")
	time.Sleep(50 * time.Millisecond)
	trackerAfter, err := os.ReadFile(store.File)
	require.NoError(t, err)
	require.Equal(t, trackerBefore, trackerAfter, "lease refresh must stay outside beads.jsonl")

	release()
	require.NoError(t, <-done)
}
