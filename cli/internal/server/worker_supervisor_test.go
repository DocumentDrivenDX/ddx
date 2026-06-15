package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWorkerController is an in-memory workerController for testing reconcile
// decisions without spawning real worker goroutines. It tracks each worker's
// record and whether the supervisor would observe a live in-memory handle.
type fakeWorkerController struct {
	seq     int
	now     func() time.Time
	records map[string]*WorkerRecord
	live    map[string]bool
}

func newFakeWorkerController(now func() time.Time) *fakeWorkerController {
	return &fakeWorkerController{
		now:     now,
		records: map[string]*WorkerRecord{},
		live:    map[string]bool{},
	}
}

func (f *fakeWorkerController) StartExecuteLoop(spec ExecuteLoopWorkerSpec) (WorkerRecord, error) {
	f.seq++
	id := fmt.Sprintf("worker-%03d", f.seq)
	// Distinct, monotonically increasing StartedAt so newest-excess selection
	// is deterministic.
	started := f.now().Add(time.Duration(f.seq) * time.Second)
	rec := WorkerRecord{
		ID:          id,
		Kind:        "work",
		State:       workerStateRunning,
		Status:      workerStateRunning,
		ProjectRoot: spec.ProjectRoot,
		StartedAt:   started,
	}
	f.records[id] = &rec
	f.live[id] = true
	return rec, nil
}

func (f *fakeWorkerController) Stop(id string) error {
	rec, ok := f.records[id]
	if !ok {
		return fmt.Errorf("worker not running")
	}
	rec.State = workerStateStopped
	rec.Status = workerStateStopped
	f.live[id] = false
	return nil
}

func (f *fakeWorkerController) List() ([]WorkerRecord, error) {
	out := make([]WorkerRecord, 0, len(f.records))
	for _, r := range f.records {
		out = append(out, *r)
	}
	return out, nil
}

func (f *fakeWorkerController) HasLiveWorker(id string) bool {
	return f.live[id]
}

// crash simulates an unexpected worker exit: the record flips terminal and the
// in-memory handle disappears.
func (f *fakeWorkerController) crash(id string) {
	if rec, ok := f.records[id]; ok {
		rec.State = "exited"
		rec.Status = "exited"
	}
	f.live[id] = false
}

// seedRunning injects a disk-only running record (no live handle), simulating
// a status.json left behind by a previous server run.
func (f *fakeWorkerController) seedRunning(id string) {
	f.records[id] = &WorkerRecord{
		ID:     id,
		Kind:   "work",
		State:  workerStateRunning,
		Status: workerStateRunning,
	}
	f.live[id] = false
}

// TestWorkerSupervisorReconcileStartsAndStopsToDesiredCount proves reconcile
// starts missing server-managed workers and stops the newest excess workers to
// match desired_count (AC2).
func TestWorkerSupervisorReconcileStartsAndStopsToDesiredCount(t *testing.T) {
	root := t.TempDir()
	clock := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	fake := newFakeWorkerController(func() time.Time { return clock })
	sup := NewWorkerSupervisor(root, fake)
	sup.clock = func() time.Time { return clock }

	require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{
		DesiredCount: 3,
		DefaultSpec:  WorkerDefaultSpec{Mode: "watch", IdleInterval: "30s"},
	}))

	// First reconcile: zero running, desired 3 -> start 3.
	res, err := sup.Reconcile()
	require.NoError(t, err)
	assert.Len(t, res.Started, 3, "must start workers up to desired_count")
	assert.Empty(t, res.Stopped)

	// Idempotent: a second reconcile at desired_count does nothing.
	res, err = sup.Reconcile()
	require.NoError(t, err)
	assert.Empty(t, res.Started)
	assert.Empty(t, res.Stopped)

	// Lower desired_count to 1 -> stop the 2 newest workers.
	require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{
		DesiredCount: 1,
		DefaultSpec:  WorkerDefaultSpec{Mode: "watch", IdleInterval: "30s"},
	}))
	res, err = sup.Reconcile()
	require.NoError(t, err)
	require.Len(t, res.Stopped, 2, "must stop excess workers down to desired_count")
	// Newest-first: worker-003 and worker-002 stop; worker-001 (oldest) stays.
	assert.ElementsMatch(t, []string{"worker-003", "worker-002"}, res.Stopped)
	assert.Equal(t, workerStateStopped, fake.records["worker-003"].State)
	assert.Equal(t, workerStateStopped, fake.records["worker-002"].State)
	assert.Equal(t, workerStateRunning, fake.records["worker-001"].State)

	// Now stable at 1.
	res, err = sup.Reconcile()
	require.NoError(t, err)
	assert.Empty(t, res.Started)
	assert.Empty(t, res.Stopped)
}

// TestWorkerSupervisorRestartBackoff proves an unexpected exit restarts only
// when restart is enabled and max_restarts_per_hour/backoff allow it (AC3).
func TestWorkerSupervisorRestartBackoff(t *testing.T) {
	root := t.TempDir()
	clock := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	fake := newFakeWorkerController(func() time.Time { return clock })
	sup := NewWorkerSupervisor(root, fake)
	sup.clock = func() time.Time { return clock }

	require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{
		DesiredCount: 1,
		DefaultSpec:  WorkerDefaultSpec{Mode: "watch", IdleInterval: "30s"},
		Restart: WorkerRestartPolicy{
			Enabled:            true,
			MaxRestartsPerHour: 2,
			Backoff:            "1m",
		},
	}))

	// Initial provisioning starts one worker (not a restart).
	res, err := sup.Reconcile()
	require.NoError(t, err)
	require.Len(t, res.Started, 1)
	require.Empty(t, res.Restarted)
	first := res.Started[0]

	// Worker crashes -> restart allowed (no prior restarts, no backoff).
	fake.crash(first)
	res, err = sup.Reconcile()
	require.NoError(t, err)
	require.Len(t, res.Restarted, 1, "first crash must restart while policy allows")
	require.Empty(t, res.Started, "replacing a crash is a restart, not initial provisioning")
	second := res.Restarted[0]

	// Crash again within the backoff window -> no restart.
	clock = clock.Add(30 * time.Second)
	fake.crash(second)
	res, err = sup.Reconcile()
	require.NoError(t, err)
	assert.Empty(t, res.Restarted, "restart must be suppressed inside the backoff window")
	assert.Empty(t, res.Started, "must not fall through to provisioning while a restart is pending")
	assert.Contains(t, res.RestartSkipped, "within backoff window")

	// Past the backoff window, within the per-hour budget -> restart resumes.
	clock = clock.Add(2 * time.Minute)
	res, err = sup.Reconcile()
	require.NoError(t, err)
	require.Len(t, res.Restarted, 1, "restart must resume after backoff elapses")
	third := res.Restarted[0]

	// Two restarts have now happened within the hour (max=2). A third crash is
	// not restarted, even past backoff.
	clock = clock.Add(2 * time.Minute)
	fake.crash(third)
	res, err = sup.Reconcile()
	require.NoError(t, err)
	assert.Empty(t, res.Restarted, "max_restarts_per_hour must cap restarts")
	assert.Contains(t, res.RestartSkipped, "max restarts per hour reached")
}

// TestWorkerSupervisorRestartDisabledAndPaused proves restart is suppressed
// when the policy is disabled or an external pause blocker is active, and that
// a suppressed crash does not fall through to unconditional provisioning.
func TestWorkerSupervisorRestartDisabledAndPaused(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		root := t.TempDir()
		clock := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
		fake := newFakeWorkerController(func() time.Time { return clock })
		sup := NewWorkerSupervisor(root, fake)
		sup.clock = func() time.Time { return clock }

		require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{
			DesiredCount: 1,
			DefaultSpec:  WorkerDefaultSpec{Mode: "watch", IdleInterval: "30s"},
			Restart:      WorkerRestartPolicy{Enabled: false},
		}))

		res, err := sup.Reconcile()
		require.NoError(t, err)
		require.Len(t, res.Started, 1)

		fake.crash(res.Started[0])
		res, err = sup.Reconcile()
		require.NoError(t, err)
		assert.Empty(t, res.Restarted)
		assert.Empty(t, res.Started)
		assert.Contains(t, res.RestartSkipped, "restart disabled")
	})

	t.Run("paused on blocker", func(t *testing.T) {
		root := t.TempDir()
		clock := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
		fake := newFakeWorkerController(func() time.Time { return clock })
		sup := NewWorkerSupervisor(root, fake)
		sup.clock = func() time.Time { return clock }
		sup.RestartPauseCheck = func() (bool, string) { return true, "dirty project root" }

		require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{
			DesiredCount: 1,
			DefaultSpec:  WorkerDefaultSpec{Mode: "watch", IdleInterval: "30s"},
			Restart:      WorkerRestartPolicy{Enabled: true},
		}))

		res, err := sup.Reconcile()
		require.NoError(t, err)
		require.Len(t, res.Started, 1)

		fake.crash(res.Started[0])
		res, err = sup.Reconcile()
		require.NoError(t, err)
		assert.Empty(t, res.Restarted)
		assert.Empty(t, res.Started)
		assert.Contains(t, res.RestartSkipped, "dirty project root")
	})
}

// TestWorkerSupervisorMarksStaleRunningRecordsStopped proves a disk status
// record that says running but has no in-memory server-owned worker is marked
// stale/stopped without adopting it (AC4).
func TestWorkerSupervisorMarksStaleRunningRecordsStopped(t *testing.T) {
	root := t.TempDir()
	clock := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	fake := newFakeWorkerController(func() time.Time { return clock })
	sup := NewWorkerSupervisor(root, fake)
	sup.clock = func() time.Time { return clock }

	// A status record left "running" by a previous server run, with no live
	// in-memory handle.
	fake.seedRunning("stale-worker")

	require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{
		DesiredCount: 1,
		DefaultSpec:  WorkerDefaultSpec{Mode: "watch", IdleInterval: "30s"},
	}))

	res, err := sup.Reconcile()
	require.NoError(t, err)

	// Stale record is marked stopped...
	assert.Contains(t, res.StaleMarked, "stale-worker")
	assert.Equal(t, workerStateStopped, fake.records["stale-worker"].State)

	// ...and never adopted: it is not tracked as managed, and a fresh worker is
	// started to satisfy desired_count instead of counting the stale one.
	_, adopted := sup.managed["stale-worker"]
	assert.False(t, adopted, "stale record must not be adopted as a managed worker")
	require.Len(t, res.Started, 1, "supervisor must start a fresh worker, not adopt the stale one")
	assert.NotEqual(t, "stale-worker", res.Started[0])
}
