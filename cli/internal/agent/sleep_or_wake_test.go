package agent

import (
	"context"
	"testing"
	"time"
)

// AC: "Execute-loop wakes on approve event without waiting for next tick".
// sleepOrWake is the unit-level building block — when wakeCh fires before
// the timer it returns nil immediately so the loop re-scans the queue. A
// 60s sleep that returns within 50ms proves the wake path short-circuits
// the timer.
func TestSleepOrWake_WakesEarly(t *testing.T) {
	wakeCh := make(chan struct{}, 1)
	wakeCh <- struct{}{}

	start := time.Now()
	if err := sleepOrWake(context.Background(), 60*time.Second, wakeCh); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("wake must short-circuit the timer; elapsed=%v", elapsed)
	}
}

// AC sanity: with a nil wakeCh the helper falls back to plain
// sleepWithContext semantics and respects the timer / context cancel.
func TestSleepOrWake_NilWakeChFallsBackToTimer(t *testing.T) {
	start := time.Now()
	if err := sleepOrWake(context.Background(), 20*time.Millisecond, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Errorf("nil wake must wait for the timer; elapsed=%v", elapsed)
	}
}

// AC sanity: ctx cancel still wins over the wake channel.
func TestSleepOrWake_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	wakeCh := make(chan struct{})
	cancel()
	if err := sleepOrWake(ctx, 5*time.Second, wakeCh); err == nil {
		t.Error("want context.Canceled, got nil")
	}
}
