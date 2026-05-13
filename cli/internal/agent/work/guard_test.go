package work

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type stubCooldownStore struct {
	calls int
}

func (s *stubCooldownStore) SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error {
	s.calls++
	return nil
}

func TestGuard_PreClaim_TwoStrikesSkips(t *testing.T) {
	store := &stubCooldownStore{}
	guard := NewPreClaimGuard(func(ctx context.Context) error {
		return errors.New("hook failed")
	}, store, nil, func() time.Time { return time.Unix(0, 0) }, 30*time.Second, 30*time.Second)

	allowed1, reason1 := guard.Allow(context.Background(), "ddx-1")
	if allowed1 {
		t.Fatalf("first failure should skip the bead")
	}
	if reason1 != "hook failed" {
		t.Fatalf("unexpected reason on first failure: %q", reason1)
	}

	allowed2, reason2 := guard.Allow(context.Background(), "ddx-1")
	if allowed2 {
		t.Fatalf("second failure should skip the bead")
	}
	if reason2 != "hook failed" {
		t.Fatalf("unexpected reason on second failure: %q", reason2)
	}
	if store.calls != 1 {
		t.Fatalf("cooldown should be written once on the second failure, got %d", store.calls)
	}
}

func TestGuard_PreClaimTimeoutReturnsPromptly(t *testing.T) {
	store := &stubCooldownStore{}
	started := make(chan struct{}, 1)
	guard := NewPreClaimGuard(func(ctx context.Context) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return ctx.Err()
	}, store, nil, func() time.Time { return time.Unix(0, 0) }, 30*time.Second, 20*time.Millisecond)

	start := time.Now()
	allowed, reason := guard.Allow(context.Background(), "ddx-1")
	elapsed := time.Since(start)

	if allowed {
		t.Fatalf("timed-out hook must not allow the bead")
	}
	if !strings.Contains(reason, "timed out") {
		t.Fatalf("unexpected timeout reason: %q", reason)
	}
	if elapsed >= 200*time.Millisecond {
		t.Fatalf("timed-out hook should return promptly, elapsed=%s", elapsed)
	}
	select {
	case <-started:
	default:
		t.Fatalf("pre-claim hook never started")
	}
}

func TestGuard_Complexity_NilGateAllowsSilently(t *testing.T) {
	var buf bytes.Buffer
	guard := NewComplexityGuard(nil, &buf)

	allowed1, reason1 := guard.Allow(context.Background(), "ddx-1")
	if !allowed1 || reason1 != "" {
		t.Fatalf("nil complexity gate should allow the bead")
	}
	allowed2, reason2 := guard.Allow(context.Background(), "ddx-2")
	if !allowed2 || reason2 != "" {
		t.Fatalf("nil complexity gate should continue to allow beads")
	}

	if got := strings.Count(buf.String(), "complexity gate missing"); got != 0 {
		t.Fatalf("expected no legacy warning, got %d in %q", got, buf.String())
	}
}
