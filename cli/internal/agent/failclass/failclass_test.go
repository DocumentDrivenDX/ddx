package failclass

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestClassifyTable is the normative failure-classification table test (AC1).
// Every attempt Outcome maps to exactly one Action.
func TestClassifyTable(t *testing.T) {
	cases := []struct {
		name    string
		outcome Outcome
		ctx     ClassifyContext
		want    Action
	}{
		{"success", OutcomeSuccess, ClassifyContext{}, ActionCloseBead},
		{"already_satisfied", OutcomeAlreadySatisfied, ClassifyContext{}, ActionCloseBead},
		{"no_changes", OutcomeNoChanges, ClassifyContext{}, ActionCloseOrUnclaim},
		{"genuine_failure_middle", OutcomeGenuineFailure, ClassifyContext{AtMaxTier: false}, ActionEscalateTier},
		{"genuine_failure_max", OutcomeGenuineFailure, ClassifyContext{AtMaxTier: true}, ActionOperatorAttention},
		{"dirty_worktree", OutcomeDirtyWorktree, ClassifyContext{}, ActionResetRetrySameTier},
		{"transport", OutcomeTransport, ClassifyContext{}, ActionRerouteSameTier},
		{"quota", OutcomeQuota, ClassifyContext{}, ActionGlobalCooldown},
		{"auth", OutcomeAuth, ClassifyContext{}, ActionOperatorAttention},
		{"setup", OutcomeSetup, ClassifyContext{}, ActionOperatorAttention},
		{"timeout_first", OutcomeTimeout, ClassifyContext{SameTierRetried: false}, ActionResetRetrySameTier},
		{"timeout_second", OutcomeTimeout, ClassifyContext{SameTierRetried: true}, ActionOperatorAttention},
		{"merge_conflict", OutcomeMergeConflict, ClassifyContext{}, ActionUnclaimRetryLater},
		{"unknown", Outcome("something_new"), ClassifyContext{}, ActionOperatorAttention},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.outcome, tc.ctx); got != tc.want {
				t.Fatalf("Classify(%q, %+v) = %q, want %q", tc.outcome, tc.ctx, got, tc.want)
			}
		})
	}
}

// TestOnlyGenuineFailureEscalates verifies that exactly one outcome can
// escalate — a genuine failure below the max tier — and that no_changes, dirty,
// transport, quota, and auth never escalate (AC2).
func TestOnlyGenuineFailureEscalates(t *testing.T) {
	// The only escalating row.
	if act := Classify(OutcomeGenuineFailure, ClassifyContext{AtMaxTier: false}); !act.Escalates() {
		t.Fatalf("genuine failure below max tier should escalate, got %q", act)
	}

	nonEscalating := []Outcome{
		OutcomeNoChanges,
		OutcomeDirtyWorktree,
		OutcomeTransport,
		OutcomeQuota,
		OutcomeAuth,
	}
	for _, outcome := range nonEscalating {
		for _, atMax := range []bool{false, true} {
			act := Classify(outcome, ClassifyContext{AtMaxTier: atMax})
			if act.Escalates() {
				t.Fatalf("outcome %q (atMaxTier=%v) must not escalate, got %q", outcome, atMax, act)
			}
		}
	}

	// A genuine failure at the max tier must not escalate either.
	if act := Classify(OutcomeGenuineFailure, ClassifyContext{AtMaxTier: true}); act.Escalates() {
		t.Fatalf("max-tier genuine failure must not escalate, got %q", act)
	}
}

// TestEscalateSingleOneHop verifies the single one-hop middle->max escalation
// and that it is idempotent: a second escalation is a no-op (AC2).
func TestEscalateSingleOneHop(t *testing.T) {
	next, escalated := Escalate(TierMiddle)
	if !escalated || next != TierMax {
		t.Fatalf("Escalate(TierMiddle) = (%v, %v), want (TierMax, true)", next, escalated)
	}

	// Already at max: no further escalation (exactly one middle->max).
	if again, ok := Escalate(next); ok || again != TierMax {
		t.Fatalf("Escalate(TierMax) = (%v, %v), want (TierMax, false)", again, ok)
	}

	// Low never escalates (escalation is the single middle->max edge only).
	if low, ok := Escalate(TierLow); ok || low != TierLow {
		t.Fatalf("Escalate(TierLow) = (%v, %v), want (TierLow, false)", low, ok)
	}
}

// TestPoolMaxInFlightCapConcurrent verifies the per-pool in-flight max-attempt
// cap holds under concurrent failures, preventing a multi-worker stampede
// (AC3). The test is bounded: a fixed number of goroutines, a WaitGroup
// barrier, and no sleeps.
func TestPoolMaxInFlightCapConcurrent(t *testing.T) {
	const (
		capLimit = 3
		workers  = 50
	)
	p := NewPool("default", capLimit)

	var acquired int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			// Acquire and hold the slot (do not Release) so the cap is the
			// hard ceiling on simultaneous in-flight attempts.
			if p.Acquire() {
				atomic.AddInt64(&acquired, 1)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&acquired); got != capLimit {
		t.Fatalf("concurrent Acquire granted %d slots, want cap %d", got, capLimit)
	}
	if got := p.InFlight(); got != capLimit {
		t.Fatalf("InFlight() = %d, want %d", got, capLimit)
	}

	// Releasing one slot frees exactly one acquisition.
	p.Release()
	if !p.Acquire() {
		t.Fatal("Acquire after Release should succeed")
	}
	if got := p.InFlight(); got != capLimit {
		t.Fatalf("InFlight() after release+acquire = %d, want %d", got, capLimit)
	}
}

// TestPoolQuotaGlobalCooldown verifies that quota exhaustion drives a global
// per-pool cooldown (not a per-bead mutation): while in cooldown no worker can
// acquire a slot, and acquisition resumes once the cooldown elapses (AC4). A
// fake clock keeps the test bounded with no real sleeps.
func TestPoolQuotaGlobalCooldown(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	current := base
	p := NewPool("default", 5)
	p.SetClock(func() time.Time { return current })

	// A quota outcome routes to a global cooldown.
	if act := Classify(OutcomeQuota, ClassifyContext{}); act != ActionGlobalCooldown {
		t.Fatalf("quota outcome = %q, want %q", act, ActionGlobalCooldown)
	}

	// Apply the cooldown; the whole pool is now unavailable to every worker.
	p.EnterCooldown(30 * time.Second)
	if !p.InCooldown() {
		t.Fatal("pool should be in cooldown after EnterCooldown")
	}
	if p.Acquire() {
		t.Fatal("Acquire must fail while the pool is in global cooldown")
	}
	if got := p.InFlight(); got != 0 {
		t.Fatalf("InFlight() during cooldown = %d, want 0 (no slot taken)", got)
	}

	// Before the window elapses, still blocked.
	current = base.Add(29 * time.Second)
	if p.Acquire() {
		t.Fatal("Acquire must fail before the cooldown window elapses")
	}

	// After the window elapses, acquisition resumes.
	current = base.Add(31 * time.Second)
	if p.InCooldown() {
		t.Fatal("pool should not be in cooldown after the window elapses")
	}
	if !p.Acquire() {
		t.Fatal("Acquire should succeed after the cooldown elapses")
	}
}
