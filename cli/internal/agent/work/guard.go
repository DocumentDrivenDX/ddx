package work

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// Guard decides whether a bead may proceed. Callers use the returned reason
// for skip telemetry and logs.
type Guard interface {
	Allow(ctx context.Context, beadID string) (bool, string)
}

// cooldownStore is the minimal store surface PreClaimGuard needs.
type cooldownStore interface {
	SetExecutionCooldown(id string, until time.Time, status, detail string) error
}

// PreClaimHook matches the execute-loop pre-claim hook signature.
type PreClaimHook func(ctx context.Context) error

// PreClaimGuard implements the two-strikes pre-claim policy. The first hook
// failure skips the candidate and retries it on the next presentation; the
// second failure parks the bead on execution cooldown.
type PreClaimGuard struct {
	hook     PreClaimHook
	store    cooldownStore
	log      io.Writer
	now      func() time.Time
	cooldown time.Duration

	mu         sync.Mutex
	failCounts map[string]int
}

// NewPreClaimGuard constructs a Guard that owns the pre-claim hook retry
// state for one worker run.
func NewPreClaimGuard(hook PreClaimHook, store cooldownStore, log io.Writer, now func() time.Time, cooldown time.Duration) *PreClaimGuard {
	if now == nil {
		now = time.Now
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &PreClaimGuard{
		hook:       hook,
		store:      store,
		log:        log,
		now:        now,
		cooldown:   cooldown,
		failCounts: make(map[string]int),
	}
}

// Allow runs the pre-claim hook and applies the two-strikes retry policy.
func (g *PreClaimGuard) Allow(ctx context.Context, beadID string) (bool, string) {
	if g == nil || g.hook == nil {
		return true, ""
	}
	if err := g.hook(ctx); err != nil {
		g.mu.Lock()
		defer g.mu.Unlock()

		g.failCounts[beadID]++
		if g.failCounts[beadID] >= 2 {
			if g.store != nil {
				until := g.now().UTC().Add(g.cooldown)
				_ = g.store.SetExecutionCooldown(beadID, until, "preclaim-hook-failed", err.Error())
			}
		}
		if g.log != nil {
			_, _ = fmt.Fprintf(g.log, "pre-claim hook: %v (skipping %s)\n", err, beadID)
		}
		return false, err.Error()
	}

	g.mu.Lock()
	delete(g.failCounts, beadID)
	g.mu.Unlock()
	return true, ""
}

// ComplexityGuard is a compatibility wrapper that delegates to a configured
// bead readiness gate. When no gate is configured it allows the bead to
// proceed. The model-backed readiness hook is now the canonical pre-claim
// gate; this wrapper remains only for callers that still inject a Guard directly.
type ComplexityGuard struct {
	Gate Guard
	Log  io.Writer

	once sync.Once
}

// NewComplexityGuard wraps the configured gate.
func NewComplexityGuard(gate Guard, log io.Writer) *ComplexityGuard {
	return &ComplexityGuard{Gate: gate, Log: log}
}

// Allow either delegates to the configured gate or fail-opens silently when the
// gate is absent.
func (g *ComplexityGuard) Allow(ctx context.Context, beadID string) (bool, string) {
	if g == nil || g.Gate == nil {
		return true, ""
	}
	return g.Gate.Allow(ctx, beadID)
}
