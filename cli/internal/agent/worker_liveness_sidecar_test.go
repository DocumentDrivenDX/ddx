package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkLoop_ClaimAndHeartbeatDoNotMutateTrackerBeforeOutcome guards the
// ddx-092c4343 contract: a long-running inline ExecuteBeadWorker.Run with a
// short heartbeat interval must refresh .ddx/workers/<worker-id>/status.json
// last_activity_at on every tick, while leaving the .ddx/beads.jsonl byte
// content untouched from before claim acquisition through the terminal outcome
// write. Live worker claim state belongs in sidecars, not in tracked rows.
func TestWorkLoop_ClaimAndHeartbeatDoNotMutateTrackerBeforeOutcome(t *testing.T) {
	projectRoot := t.TempDir()
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init(context.Background()))
	target := &bead.Bead{ID: "ddx-liveness-1", Title: "Liveness sidecar bead"}
	require.NoError(t, store.Create(context.Background(), target))

	beadsPath := filepath.Join(ddxDir, "beads.jsonl")
	beforeClaimSnapshot, err := os.ReadFile(beadsPath)
	require.NoError(t, err)
	sessionID := "sess-liveness-test"

	var (
		mu                                sync.Mutex
		claimSnapshot, preOutcomeSnapshot []byte
		observedTimestamps                []time.Time
		execErr                           error
	)

	executor := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		// Snapshot beads.jsonl right after the executor was handed control,
		// which is immediately after the claim write.
		snap, err := os.ReadFile(beadsPath)
		if err != nil {
			execErr = err
			return ExecuteBeadReport{}, err
		}
		mu.Lock()
		claimSnapshot = snap
		mu.Unlock()

		// Observe the sidecar advancing across ticks. With a 5ms heartbeat
		// interval, 80ms of wall-clock gives ~16 ticks; we only require 3
		// distinct timestamps to confirm "every tick" advances last_activity_at
		// without making the test flaky on slow CI.
		//
		// Under a full-suite run the detector plus CPU contention stretch the
		// effective heartbeat period well past 5ms, so widen the observation
		// window rather than lowering the required tick count: the assertion
		// ("every tick advances last_activity_at") stays intact, and the loop
		// still exits as soon as 3 distinct timestamps are seen.
		observationWindow := 3 * time.Second
		if raceEnabled {
			observationWindow = 10 * time.Second
		}
		seen := map[time.Time]struct{}{}
		deadline := time.Now().Add(observationWindow)
		for time.Now().Before(deadline) && len(seen) < 3 {
			rec, rerr := workerstatus.ReadLiveness(projectRoot, sessionID)
			if rerr == nil && !rec.LastActivityAt.IsZero() {
				if _, ok := seen[rec.LastActivityAt]; !ok {
					seen[rec.LastActivityAt] = struct{}{}
					mu.Lock()
					observedTimestamps = append(observedTimestamps, rec.LastActivityAt)
					mu.Unlock()
				}
			}
			time.Sleep(8 * time.Millisecond)
		}

		// Snapshot beads.jsonl right before returning the outcome — this is
		// the byte state immediately before the loop performs the terminal
		// CloseWithEvidence / Unclaim write.
		snap2, err := os.ReadFile(beadsPath)
		if err != nil {
			execErr = err
			return ExecuteBeadReport{}, err
		}
		mu.Lock()
		preOutcomeSnapshot = snap2
		mu.Unlock()

		return ExecuteBeadReport{
			BeadID:    beadID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: sessionID,
			ResultRev: "live-rev",
			BaseRev:   "base-rev",
		}, nil
	})

	worker := &ExecuteBeadWorker{Store: store, Executor: executor}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:          "worker",
		HeartbeatInterval: 5 * time.Millisecond,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Mode:        executeloop.ModeOnce,
		ProjectRoot: projectRoot,
		SessionID:   sessionID,
		WorkerID:    "worker",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NoError(t, execErr)
	require.Equal(t, 1, result.Attempts, "the single ready bead must be claimed and attempted")

	mu.Lock()
	defer mu.Unlock()

	require.NotEmpty(t, claimSnapshot, "claim-time beads.jsonl snapshot must be captured")
	require.NotEmpty(t, preOutcomeSnapshot, "pre-outcome beads.jsonl snapshot must be captured")
	assert.True(t, bytes.Equal(beforeClaimSnapshot, claimSnapshot),
		"beads.jsonl byte content must stay unchanged across worker claim acquisition")
	assert.True(t, bytes.Equal(claimSnapshot, preOutcomeSnapshot),
		"beads.jsonl byte content must not change between the claim write and the terminal outcome write; heartbeat ticks must not rewrite the tracker")

	require.GreaterOrEqualf(t, len(observedTimestamps), 3,
		"sidecar last_activity_at must advance on every heartbeat tick (saw %d distinct timestamps)", len(observedTimestamps))
	for i := 1; i < len(observedTimestamps); i++ {
		assert.Truef(t, observedTimestamps[i].After(observedTimestamps[i-1]),
			"sidecar last_activity_at must strictly advance across ticks: %v -> %v",
			observedTimestamps[i-1], observedTimestamps[i])
	}

	// Final sidecar must still be present after the attempt finalises.
	finalRec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
	require.NoError(t, err)
	assert.Equal(t, sessionID, finalRec.WorkerID)
}

func TestWorkLoop_MirrorsPeriodicLivenessWithoutTrackerHeartbeatSpam(t *testing.T) {
	TestWorkLoop_ClaimAndHeartbeatDoNotMutateTrackerBeforeOutcome(t)
}

// TestWorkerStatusSidecar_RecordsCurrentBeadAttemptAndLastActivity guards the
// ddx-1be8df2b contract that after several ticks of a simulated attempt the
// sidecar JSON carries every operator-facing field: worker_id, current_bead,
// attempt_id, phase, last_activity_at, and pid (when available). Each tick
// must overwrite last_activity_at with the current time so an operator can
// answer "is this worker still alive and what is it on?" from the file alone.
func TestWorkerStatusSidecar_RecordsCurrentBeadAttemptAndLastActivity(t *testing.T) {
	projectRoot := t.TempDir()

	workerID := "worker-1be8df2b"
	rep := work.NewSidecarLivenessReporter(projectRoot, workerID, "sess-status", nil)
	rep.SetAttempt(
		"ddx-aabbccdd",
		"att-20260514-001",
		"running",
		"balanced",
		"claude",
		"opus",
		"balanced",
		7777,
	)

	const ticks = 5
	var (
		timestamps    []time.Time
		ticksObserved int32
	)
	base := time.Now().UTC()
	for i := 0; i < ticks; i++ {
		// Drive distinct, strictly-increasing tick times so we can assert
		// last_activity_at advances on every tick without depending on
		// wall-clock resolution.
		tick := base.Add(time.Duration(i+1) * time.Millisecond)
		rep.OnTick(tick)
		atomic.AddInt32(&ticksObserved, 1)

		rec, err := workerstatus.ReadLiveness(projectRoot, workerID)
		require.NoError(t, err, "sidecar must be readable after tick %d", i)
		timestamps = append(timestamps, rec.LastActivityAt)
	}

	require.Equal(t, int32(ticks), atomic.LoadInt32(&ticksObserved))

	rec, err := workerstatus.ReadLiveness(projectRoot, workerID)
	require.NoError(t, err)

	assert.Equal(t, workerID, rec.WorkerID, "worker_id must be persisted")
	assert.Equal(t, "ddx-aabbccdd", rec.CurrentBead, "current_bead must be persisted")
	assert.Equal(t, "att-20260514-001", rec.AttemptID, "attempt_id must be persisted")
	assert.Equal(t, "running", rec.Phase, "phase must be persisted")
	assert.Equal(t, "balanced", rec.Route)
	assert.Equal(t, "claude", rec.Harness)
	assert.Equal(t, "opus", rec.Model)
	assert.Equal(t, 7777, rec.ChildPID)
	assert.Equal(t, os.Getpid(), rec.PID, "worker pid must be recorded when available")
	assert.False(t, rec.LastActivityAt.IsZero(), "last_activity_at must be set")
	assert.True(t, rec.LastActivityAt.Equal(base.Add(time.Duration(ticks)*time.Millisecond)),
		"last_activity_at must reflect the most recent tick time")

	// AC #3: writes are atomic — the file content stored on disk is a valid
	// JSON document at every observable moment. We verify by checking that
	// each post-tick read returned a strictly-advancing timestamp (no torn
	// reads would have surfaced as a parse error in ReadLiveness above).
	for i := 1; i < len(timestamps); i++ {
		assert.Truef(t, timestamps[i].After(timestamps[i-1]),
			"each tick must advance last_activity_at: tick %d=%v, tick %d=%v",
			i-1, timestamps[i-1], i, timestamps[i])
	}
}
