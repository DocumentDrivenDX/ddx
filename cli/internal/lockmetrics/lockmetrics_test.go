package lockmetrics

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capture installs a capturing sink for the duration of the test and returns a
// snapshot function. The previous sink is restored on cleanup.
func capture(t *testing.T) func() []Event {
	t.Helper()
	var mu sync.Mutex
	var events []Event
	SetSink(func(ev Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	})
	t.Cleanup(func() { SetSink(nil) })
	return func() []Event {
		mu.Lock()
		defer mu.Unlock()
		out := make([]Event, len(events))
		copy(out, events)
		return out
	}
}

// TestLockMetrics_EmitsAcquireAndReleaseEvents asserts that instrumenting a
// held lock window produces one acquire and one release event carrying the
// required fields (lock_name, acquired_at, released_at, duration_ms,
// holder_pid, operation).
func TestLockMetrics_EmitsAcquireAndReleaseEvents(t *testing.T) {
	snapshot := capture(t)

	err := Instrument("tracker.lock", "tracker.commit", func() error { return nil })
	require.NoError(t, err)

	events := snapshot()
	require.Len(t, events, 2, "expected one acquire and one release event")

	acquire, release := events[0], events[1]
	assert.Equal(t, "acquire", acquire.Event)
	assert.Equal(t, "release", release.Event)

	// Both events carry the lock identity, operation, and holder pid.
	for _, ev := range events {
		assert.Equal(t, "tracker.lock", ev.LockName)
		assert.Equal(t, "tracker.commit", ev.Operation)
		assert.Equal(t, os.Getpid(), ev.HolderPID)
		_, perr := time.Parse(time.RFC3339Nano, ev.AcquiredAt)
		assert.NoError(t, perr, "acquired_at must be RFC3339Nano")
	}

	// The release event additionally carries released_at and duration_ms.
	_, perr := time.Parse(time.RFC3339Nano, release.ReleasedAt)
	assert.NoError(t, perr, "released_at must be RFC3339Nano")
	assert.GreaterOrEqual(t, release.DurationMS, int64(0))
}

// TestLockMetrics_DurationMatchesElapsed asserts that the release event's
// duration_ms equals released_at minus acquired_at within a 5ms tolerance.
func TestLockMetrics_DurationMatchesElapsed(t *testing.T) {
	snapshot := capture(t)

	err := Instrument("index.lock", "index.commit", func() error {
		time.Sleep(25 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)

	events := snapshot()
	require.Len(t, events, 2)
	release := events[1]
	require.Equal(t, "release", release.Event)

	acquiredAt, err := time.Parse(time.RFC3339Nano, release.AcquiredAt)
	require.NoError(t, err)
	releasedAt, err := time.Parse(time.RFC3339Nano, release.ReleasedAt)
	require.NoError(t, err)

	elapsedMS := releasedAt.Sub(acquiredAt).Milliseconds()
	diff := release.DurationMS - elapsedMS
	if diff < 0 {
		diff = -diff
	}
	assert.LessOrEqualf(t, diff, int64(5),
		"duration_ms (%d) must match released_at-acquired_at (%d) within 5ms",
		release.DurationMS, elapsedMS)
}

// TestLockMetrics_FileSinkRoundTrip asserts the file sink and Load accessor
// round-trip events through .ddx/metrics/locks.jsonl.
func TestLockMetrics_FileSinkRoundTrip(t *testing.T) {
	root := t.TempDir()
	SetSink(FileSink(root))
	t.Cleanup(func() { SetSink(nil) })

	require.NoError(t, Instrument("tracker.lock", "tracker.commit", func() error { return nil }))

	events, err := Load(root)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "acquire", events[0].Event)
	assert.Equal(t, "release", events[1].Event)
}

// TestLockMetrics_NilSinkIsNoOp asserts that with no sink installed,
// instrumentation neither errors nor records anything.
func TestLockMetrics_NilSinkIsNoOp(t *testing.T) {
	SetSink(nil)
	called := false
	err := Instrument("tracker.lock", "tracker.commit", func() error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called, "critical section must still run with no sink")
}

// TestLockMetricsRotatesBeforeLargeFileThreshold seeds locks.jsonl at
// MaxActiveSizeBytes, emits additional lock events via a FileSink, and proves
// the active file remains below the 5 MiB pre-commit large-file guard.
func TestLockMetricsRotatesBeforeLargeFileThreshold(t *testing.T) {
	root := t.TempDir()
	metricsPath := Path(root)
	require.NoError(t, os.MkdirAll(filepath.Dir(metricsPath), 0o755))

	// Seed the active file at the rotation cap to guarantee rotation fires.
	seed := make([]byte, MaxActiveSizeBytes)
	require.NoError(t, os.WriteFile(metricsPath, seed, 0o644))

	SetSink(FileSink(root))
	t.Cleanup(func() { SetSink(nil) })

	require.NoError(t, Instrument("tracker.lock", "tracker.commit", func() error { return nil }))

	// Active file must be below the 5 MiB pre-commit large-file guard.
	const largeFileGuard = 5 * 1024 * 1024
	fi, err := os.Stat(metricsPath)
	require.NoError(t, err)
	assert.Less(t, fi.Size(), int64(largeFileGuard),
		"active locks.jsonl must stay below the %d-byte large-file guard after rotation", largeFileGuard)
}

// TestLockViolationEvidenceSurvivesMetricsRotation proves that a
// lock-violation.json record written to the execution evidence directory is not
// affected by metrics rotation: the file persists after locks.jsonl is rotated.
func TestLockViolationEvidenceSurvivesMetricsRotation(t *testing.T) {
	root := t.TempDir()
	evidenceDir := ddxroot.JoinProject(root, "executions", "20260613T210703-88070be3")
	require.NoError(t, os.MkdirAll(evidenceDir, 0o755))

	// Write a violation record to the evidence directory.
	violationPath := filepath.Join(evidenceDir, "lock-violation.json")
	require.NoError(t, os.WriteFile(violationPath, []byte(`{"lock_name":"index.lock"}`), 0o644))

	// Seed the metrics file at the rotation cap to trigger rotation on next write.
	metricsPath := Path(root)
	require.NoError(t, os.MkdirAll(filepath.Dir(metricsPath), 0o755))
	seed := make([]byte, MaxActiveSizeBytes)
	require.NoError(t, os.WriteFile(metricsPath, seed, 0o644))

	SetSink(FileSink(root))
	t.Cleanup(func() { SetSink(nil) })
	require.NoError(t, Instrument("index.lock", "index.commit", func() error { return nil }))

	// Violation evidence must survive metrics rotation.
	_, err := os.Stat(violationPath)
	assert.NoError(t, err, "lock-violation.json must survive metrics rotation")
}

// TestWorkerCleanupDoesNotLeaveOversizedLockMetricsDirty simulates the lock
// events generated during a worker cleanup pass (many acquire/release cycles)
// and proves the resulting locks.jsonl stays below the 5 MiB pre-commit
// large-file guard even after the file is pre-seeded at the rotation cap.
func TestWorkerCleanupDoesNotLeaveOversizedLockMetricsDirty(t *testing.T) {
	root := t.TempDir()
	metricsPath := Path(root)
	require.NoError(t, os.MkdirAll(filepath.Dir(metricsPath), 0o755))

	// Pre-fill to simulate accumulated events from prior worker cycles.
	seed := make([]byte, MaxActiveSizeBytes)
	require.NoError(t, os.WriteFile(metricsPath, seed, 0o644))

	SetSink(FileSink(root))
	t.Cleanup(func() { SetSink(nil) })

	// Emit acquire/release pairs as a worker cleanup would.
	for i := 0; i < 20; i++ {
		_ = Instrument("tracker.lock", "worker.cleanup", func() error { return nil })
	}

	const largeFileGuard = 5 * 1024 * 1024
	fi, err := os.Stat(metricsPath)
	require.NoError(t, err)
	assert.Less(t, fi.Size(), int64(largeFileGuard),
		"locks.jsonl must not trip the large-file guard after worker cleanup operations")
}
