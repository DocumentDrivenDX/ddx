package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/require"
)

// runBlockedHeartbeatLoopWithRuntime starts ExecuteBeadWorker.Run with a
// caller-supplied runtime so tests can wire ProjectRoot / SessionID and
// EventSink, then block the attempt on a release channel. Returns a
// release closure (call to let the attempt finish) and a done channel
// carrying the Run() error.
func runBlockedHeartbeatLoopWithRuntime(t *testing.T, store *bead.Store, heartbeatInterval time.Duration, runtime ExecuteBeadLoopRuntime) (func(), <-chan error) {
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

	runtime.Once = true
	go func() {
		_, err := worker.Run(context.Background(), rcfg, runtime)
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

// TestWorkerHeartbeat_MirrorsPeriodicLivenessWithoutTrackerHeartbeatSpam
// verifies AC #1: a long-running ExecuteBeadWorker.Run with a short
// heartbeat interval refreshes worker-side liveness on every tick while
// the bead tracker bytes stay byte-identical between claim and terminal
// outcome. The sidecar's last_activity_at must advance across ticks so an
// operator-facing view can detect a live worker even when the tracker
// claim timestamp has not changed.
func TestWorkerHeartbeat_MirrorsPeriodicLivenessWithoutTrackerHeartbeatSpam(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	projectRoot := store.Dir
	sessionID := "agent-loop-test-liveness"

	runtime := ExecuteBeadLoopRuntime{
		ProjectRoot: projectRoot,
		SessionID:   sessionID,
	}
	release, done := runBlockedHeartbeatLoopWithRuntime(t, store, 5*time.Millisecond, runtime)

	trackerBefore, err := os.ReadFile(store.File)
	require.NoError(t, err)

	// Wait through enough ticks to advance last_activity_at at least twice.
	require.Eventually(t, func() bool {
		rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
		if err != nil {
			return false
		}
		return rec.CurrentBead == first.ID && !rec.LastActivityAt.IsZero()
	}, time.Second, 5*time.Millisecond, "sidecar must reflect the claimed bead")

	firstSnap, err := workerstatus.ReadLiveness(projectRoot, sessionID)
	require.NoError(t, err)
	require.Equal(t, first.ID, firstSnap.CurrentBead, "sidecar must record the claimed bead id")
	require.NotEmpty(t, firstSnap.AttemptID, "sidecar must record a provisional attempt id")

	require.Eventually(t, func() bool {
		rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
		if err != nil {
			return false
		}
		return rec.LastActivityAt.After(firstSnap.LastActivityAt)
	}, time.Second, 5*time.Millisecond, "last_activity_at must advance on each heartbeat tick")

	trackerAfter, err := os.ReadFile(store.File)
	require.NoError(t, err)
	require.Equal(t, trackerBefore, trackerAfter,
		"sidecar liveness updates must not rewrite beads.jsonl between claim and terminal outcome")

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
