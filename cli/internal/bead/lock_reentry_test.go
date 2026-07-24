package bead

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestWriteAllLocked_WorksUnderWithLock is the positive contract for
// ddx-2a319f04: corpus rewrites already holding the collection lock must
// use WriteAllLocked, not WriteAll. WriteAll re-enters the non-reentrant
// directory lock and deadlocks until LockWait expires.
func TestWriteAllLocked_WorksUnderWithLock(t *testing.T) {
	s := newTestStore(t)
	s.LockWait = 500 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- s.WithLock(func() error {
			return s.WriteAllLocked([]Bead{{
				ID:     "b1",
				Title:  "under lock",
				Status: StatusOpen,
			}})
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WriteAllLocked under WithLock: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WriteAllLocked under WithLock hung — lock re-entry regression")
	}

	got, err := s.ReadAll(context.Background())
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 1 || got[0].ID != "b1" {
		t.Fatalf("got %#v, want one bead b1", got)
	}
}

// TestWriteAll_NestedWithLockTimesOut pins the failure mode that broke CI:
// Store.WriteAll acquires the collection lock, so calling it from inside
// WithLock deadlocks on the same process-owned lock dir until LockWait.
// If this test starts passing without a design change, re-check whether
// WithLock became re-entrant and update callers accordingly.
func TestWriteAll_NestedWithLockTimesOut(t *testing.T) {
	s := newTestStore(t)
	s.LockWait = 200 * time.Millisecond

	start := time.Now()
	err := s.WithLock(func() error {
		return s.WriteAll([]Bead{{
			ID:     "b1",
			Title:  "nested write",
			Status: StatusOpen,
		}})
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected nested WriteAll to fail with lock timeout; got nil")
	}
	if !strings.Contains(err.Error(), "lock timeout") {
		t.Fatalf("expected lock timeout error, got: %v", err)
	}
	if elapsed < 150*time.Millisecond {
		t.Fatalf("nested WriteAll returned too quickly (%v); expected wait near LockWait", elapsed)
	}
	// A subsequent uncontended WriteAll must succeed (outer lock released).
	if err := s.WriteAll([]Bead{{ID: "b2", Title: "after", Status: StatusOpen}}); err != nil {
		t.Fatalf("WriteAll after nested timeout: %v", err)
	}
}
