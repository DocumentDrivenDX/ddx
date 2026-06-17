package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/lockmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreDispatchCommits_IndexCommitStaysBelowCap(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	beadID := "ddx-int-0001"

	metricsRel := filepath.Join(ddxroot.DirName, "metrics", "attempts.jsonl")
	metricsPath := filepath.Join(projectRoot, metricsRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(metricsPath), 0o755))
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Update(context.Background(), beadID, func(b *bead.Bead) {
		b.Notes = "tracker seed"
	}))
	require.NoError(t, os.WriteFile(metricsPath, []byte(`{"seed":"checkpoint"}`+"\n"), 0o644))

	rec := &lockSampleRecorder{}
	prevSink := SetTrackerLockMetricsSink(rec.record)
	t.Cleanup(func() { SetTrackerLockMetricsSink(prevSink) })

	var eventsMu sync.Mutex
	var events []lockmetrics.Event
	lockmetrics.SetSink(func(ev lockmetrics.Event) {
		eventsMu.Lock()
		events = append(events, ev)
		eventsMu.Unlock()
	})
	t.Cleanup(func() { lockmetrics.SetSink(nil) })

	lockmetrics.SetCapEnforcement(projectRoot, t.TempDir())
	t.Cleanup(func() { lockmetrics.SetCapEnforcement("", "") })
	t.Setenv("DDX_LOCK_CAP_INDEX_MS", "")

	res := runNoopExecuteBeadForCheckpoint(t, projectRoot, beadID)
	require.NotNil(t, res)
	require.NotEmpty(t, res.BaseRev)

	log := runGitInteg(t, projectRoot, "log", "--format=%s", "-2")
	assert.Contains(t, log, "chore: update tracker (execute-bead ")
	assert.Contains(t, log, "chore: checkpoint pre-execute-bead ")
	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:"+filepath.ToSlash(metricsRel)), "checkpoint")

	trackerDurations := trackerLockDurations(rec.snapshot(), "pre_dispatch_commits")
	require.NotEmpty(t, trackerDurations, "tracker commit samples were not recorded")
	assert.Less(t, percentileDuration(trackerDurations, 1.0), lockmetrics.DefaultIndexLockCap,
		"tracker commit hold time must stay below the default index cap")

	checkpointDurations := trackerLockDurations(rec.snapshot(), "pre_dispatch_checkpoint")
	require.NotEmpty(t, checkpointDurations, "checkpoint lock samples were not recorded")
	assert.Less(t, percentileDuration(checkpointDurations, 1.0), lockmetrics.DefaultIndexLockCap,
		"checkpoint hold time must stay below the default index cap")

	eventsMu.Lock()
	defer eventsMu.Unlock()
	for _, ev := range events {
		assert.NotEqual(t, "violation", ev.Event, "pre-dispatch commits must not trip the index cap watchdog")
	}
}

func TestPreDispatchCommits_CapTimeoutDoesNotKillManagedWorker(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	beadID := "ddx-int-0001"

	store := makeLoopStore(t, ddxDir)
	require.NoError(t, store.Update(context.Background(), beadID, func(b *bead.Bead) {
		b.Notes = "timeout seed"
	}))

	lockPath := worktreeIndexLockPath(projectRoot)
	prevLookup := preDispatchIndexLockOwnerLookup
	t.Cleanup(func() { preDispatchIndexLockOwnerLookup = prevLookup })
	preDispatchIndexLockOwnerLookup = func(string) (int, error) { return 0, nil }

	prevRunner := runGitWithIndexLockRecovery
	t.Cleanup(func() { runGitWithIndexLockRecovery = prevRunner })
	var calls int32
	runGitWithIndexLockRecovery = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if atomic.AddInt32(&calls, 1) != 1 {
			return []byte{}, nil
		}
		require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))
		<-ctx.Done()
		require.NoError(t, os.WriteFile(lockPath, []byte("recreated"), 0o644))
		return nil, ctx.Err()
	}

	lockmetrics.SetCapEnforcement(projectRoot, t.TempDir())
	t.Cleanup(func() { lockmetrics.SetCapEnforcement("", "") })
	t.Setenv("DDX_LOCK_CAP_INDEX_MS", "25")

	firstErr := withTrackerLock(projectRoot, "pre_dispatch_commits", func() error {
		return commitTrackerLocked(projectRoot)
	})
	require.Error(t, firstErr)
	assert.Contains(t, firstErr.Error(), preDispatchGitTimeoutMarker)

	got, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner, "timed-out pre-dispatch work must release the claim")

	report := ExecuteBeadReport{
		BeadID: beadID,
		Status: ExecuteBeadStatusExecutionFailed,
		Detail: firstErr.Error(),
		Error:  firstErr.Error(),
	}
	timeoutStop, detail, ok := preDispatchGitTimeoutStop(report, firstErr, projectRoot, beadID)
	require.True(t, ok)
	require.NotNil(t, timeoutStop)
	assert.Equal(t, preDispatchGitTimeoutReason, timeoutStop.Reason)
	assert.Equal(t, beadID, timeoutStop.BeadID)
	assert.Equal(t, projectRoot, timeoutStop.ProjectRoot)
	assert.Equal(t, "context deadline exceeded", detail)
	assert.Contains(t, timeoutStop.Message, "parent index lock clears")

	_, statErr := os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr), "timed-out pre-dispatch git must not leave a stale index.lock")

	require.NoError(t, store.Claim(beadID, "worker"))
	require.NoError(t, releaseWorkerClaim(store, beadID, "worker"))
	claimed, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Empty(t, claimed.Owner, "timed-out pre-dispatch work must release the claim")
}

func TestPreDispatchCommits_RecoversStaleIndexLockBeforeNextClaim(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	lockPath := worktreeIndexLockPath(projectRoot)

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Update(context.Background(), "ddx-int-0001", func(b *bead.Bead) {
		b.Notes = "retry seed"
	}))

	prevLookup := preDispatchIndexLockOwnerLookup
	t.Cleanup(func() { preDispatchIndexLockOwnerLookup = prevLookup })
	preDispatchIndexLockOwnerLookup = func(string) (int, error) { return 0, nil }

	lockmetrics.SetCapEnforcement(projectRoot, t.TempDir())
	t.Cleanup(func() { lockmetrics.SetCapEnforcement("", "") })
	t.Setenv("DDX_LOCK_CAP_INDEX_MS", "25")

	prevRunner := runGitWithIndexLockRecovery
	var firstCall atomic.Bool
	runGitWithIndexLockRecovery = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if firstCall.Swap(true) {
			return prevRunner(ctx, dir, args...)
		}
		require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))
		<-ctx.Done()
		require.NoError(t, os.WriteFile(lockPath, []byte("recreated"), 0o644))
		return nil, ctx.Err()
	}
	t.Cleanup(func() { runGitWithIndexLockRecovery = prevRunner })

	firstErr := withTrackerLock(projectRoot, "pre_dispatch_commits", func() error {
		return commitTrackerLocked(projectRoot)
	})
	require.Error(t, firstErr)
	assert.Contains(t, firstErr.Error(), preDispatchGitTimeoutMarker)
	_, statErr := os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr), "cancelled pre-dispatch git must be cleaned up before the next claim")

	secondErr := withTrackerLock(projectRoot, "pre_dispatch_commits", func() error {
		return commitTrackerLocked(projectRoot)
	})
	require.NoError(t, secondErr)

	subject := runGitInteg(t, projectRoot, "log", "-1", "--pretty=%s")
	assert.True(t, strings.HasPrefix(subject, "chore: update tracker (execute-bead "),
		"tracker commit must succeed after stale-lock recovery, got %q", subject)
}
