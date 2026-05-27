package lockmetrics

import (
	"os"
	"sync"
	"testing"
	"time"

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
