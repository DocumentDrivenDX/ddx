package bead

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWithLock_RecordsWaitDuration asserts that LockMetricsSink is called with
// a measurable wait duration when WithLock must spin waiting for a contended lock.
func TestWithLock_RecordsWaitDuration(t *testing.T) {
	var mu sync.Mutex
	var captured []LockSample
	prev := LockMetricsSink
	LockMetricsSink = func(s LockSample) {
		mu.Lock()
		captured = append(captured, s)
		mu.Unlock()
	}
	defer func() { LockMetricsSink = prev }()

	s := newTestStore(t)

	// Pre-acquire the lock with the current PID so breakStaleLockDir cannot reclaim it.
	require.NoError(t, os.MkdirAll(s.LockDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(s.LockDir, "pid"),
		[]byte(fmt.Sprintf("%d", os.Getpid())), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(s.LockDir, "acquired_at"),
		[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644))

	// Release the lock after 20ms so WithLock experiences measurable contention.
	go func() {
		time.Sleep(20 * time.Millisecond)
		os.RemoveAll(s.LockDir)
	}()

	err := s.WithLock(func() error { return nil })
	require.NoError(t, err)

	mu.Lock()
	got := append([]LockSample(nil), captured...)
	mu.Unlock()

	if len(got) == 0 {
		t.Fatal("LockMetricsSink was not called")
	}
	if got[0].LockDir != s.LockDir {
		t.Fatalf("LockSample.LockDir = %q, want %q", got[0].LockDir, s.LockDir)
	}
	if got[0].Wait < 5*time.Millisecond {
		t.Fatalf("expected wait >= 5ms for contended lock, got %v", got[0].Wait)
	}
}
