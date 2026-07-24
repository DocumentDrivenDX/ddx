package server

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServerShutdownBoundsSupervisorWaitBeforeWorkerShutdown proves that
// when the supervisor reconcile goroutine is stuck after cancellation
// (supervisorDone never closes), Server.Shutdown still reaches worker
// cleanup within a bounded duration instead of hanging until systemd
// SIGKILLs the process group.
func TestServerShutdownBoundsSupervisorWaitBeforeWorkerShutdown(t *testing.T) {
	prevGrace := supervisorShutdownGrace
	supervisorShutdownGrace = 50 * time.Millisecond
	t.Cleanup(func() { supervisorShutdownGrace = prevGrace })

	workDir := setupTestDir(t)
	beadID := "ddx-supervisor-bound-shutdown"
	_ = seedClaimedBead(t, workDir, beadID)

	srv := New(":0", workDir)

	// Simulate a supervisor that has been cancelled but is stuck inside
	// reconcile/git and never closes supervisorDone.
	srv.supervisorCancel = func() {}
	srv.supervisorDone = make(chan struct{}) // intentionally never closed

	workerID := "worker-supervisor-bound-shutdown"
	require.NoError(t, os.MkdirAll(filepath.Join(srv.workers.rootDir, workerID), 0o755))
	startedAt := time.Now().UTC().Add(-time.Minute)
	_, cancelled := newIdleHandle(t, srv.workers, workerID, beadID, startedAt, startedAt)

	// Outer bound must be larger than supervisorShutdownGrace but far
	// smaller than systemd's default stop timeout (~90s). Without a
	// bounded wait, Shutdown would hang forever on the stuck channel.
	const outerBound = 2 * time.Second
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- srv.Shutdown() }()

	select {
	case err := <-done:
		elapsed := time.Since(start)
		require.NoError(t, err)
		if elapsed > outerBound {
			t.Fatalf("Shutdown took %v; expected return within %v when supervisor is stuck", elapsed, outerBound)
		}
		// Must have waited at least the grace window before continuing
		// (otherwise we might not have exercised the timeout path).
		if elapsed < supervisorShutdownGrace {
			t.Fatalf("Shutdown returned in %v before supervisor grace %v; stuck-supervisor path not exercised", elapsed, supervisorShutdownGrace)
		}
	case <-time.After(outerBound):
		t.Fatal("Shutdown blocked on stuck supervisorDone; worker cleanup never ran")
	}

	assert.True(t, cancelled.Load(),
		"Shutdown must invoke worker cleanup (cancel live worker) even when supervisorDone never closes")
	assert.Nil(t, srv.supervisorCancel, "Shutdown must clear supervisorCancel after the bounded wait")
	assert.Nil(t, srv.supervisorDone, "Shutdown must clear supervisorDone after the bounded wait")
}

// spyBeadHub wraps a real lifecycle subscriber and records whether Close was
// called.
type spyBeadHub struct {
	beadHubCloser
	closeCalled bool
}

func (s *spyBeadHub) Close() {
	s.closeCalled = true
	s.beadHubCloser.Close()
}

// TC-SERVER-SHUTDOWN-001: Shutdown stops server-owned workers before closing
// the bead hub and land coordinators.
func TestServerShutdownCallsWorkerManagerShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is covered by Unix implementation")
	}

	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "shutdown-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)
	srv.EnableManagedWorkers()

	// Inject a spy so we can observe the Close() call.
	hub := bead.NewLifecycleSubscriber(func(projectID string) (bead.BeadReader, error) {
		return bead.NewStore(ddxroot.JoinProject(projectID)), nil
	}, 250*time.Millisecond).(beadHubCloser)
	spy := &spyBeadHub{beadHubCloser: hub}
	srv.beadHub = spy

	// Prime one coordinator so StopAll has an entry to clear.
	_ = srv.workers.LandCoordinators.Get(workDir)

	store := seedClaimedBead(t, workDir, "ddx-server-shutdown")
	_, rootPID, claudePID, claudeSleepPID, codexPID, codexSleepPID := startManagedWatchdogTree(t)

	workerID := "worker-server-shutdown"
	require.NoError(t, os.MkdirAll(filepath.Join(srv.workers.rootDir, workerID), 0o755))
	startedAt := time.Now().UTC().Add(-time.Minute)
	handle, cancelled := newManagedIdleHandle(t, srv.workers, workerID, "ddx-server-shutdown", rootPID, startedAt, startedAt)
	handle.record.PID = rootPID
	handle.record.PGID = rootPID
	handle.record.Lifecycle = []WorkerLifecycleEvent{{
		Action:    "start",
		Actor:     "local-operator",
		Timestamp: startedAt,
		Detail:    "kind=work",
	}}

	if err := srv.Shutdown(); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	// Verify beadHub.Close() was invoked.
	if !spy.closeCalled {
		t.Error("Shutdown did not call beadHub.Close()")
	}

	// Verify StopAll was invoked: the registry must be empty after Shutdown.
	all := srv.workers.LandCoordinators.AllMetrics()
	if len(all) != 0 {
		t.Errorf("Shutdown did not call StopAll: coordinator registry has %d entries", len(all))
	}

	waitForProcessGone(t, rootPID)
	waitForProcessGone(t, claudePID)
	waitForProcessGone(t, claudeSleepPID)
	waitForProcessGone(t, codexPID)
	waitForProcessGone(t, codexSleepPID)

	rec, err := srv.workers.readRecord(filepath.Join(srv.workers.rootDir, workerID))
	require.NoError(t, err)
	assert.Equal(t, "stopped", rec.State)
	assert.Equal(t, "stopped", rec.Status)
	assert.False(t, rec.FinishedAt.IsZero())
	require.Len(t, rec.Lifecycle, 2)
	assert.Equal(t, "start", rec.Lifecycle[0].Action)
	assert.Equal(t, "stop", rec.Lifecycle[1].Action)
	assert.Contains(t, rec.Lifecycle[1].Detail, "cleanup=")

	b, err := store.Get(context.Background(), "ddx-server-shutdown")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status)
	assert.True(t, cancelled.Load(), "Shutdown must cancel the worker goroutine")
}
