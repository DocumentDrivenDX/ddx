package work

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// DefaultPreClaimTimeout bounds pre-claim readiness hooks when callers do not
// provide an explicit timeout.
const DefaultPreClaimTimeout = 30 * time.Second

// Guard decides whether a bead may proceed. Callers use the returned reason
// for skip telemetry and logs.
type Guard interface {
	Allow(ctx context.Context, beadID string) (bool, string)
}

// cooldownStore is the minimal store surface PreClaimGuard needs.
type cooldownStore interface {
	SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error
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
	timeout  time.Duration

	mu         sync.Mutex
	failCounts map[string]int
}

// NewPreClaimGuard constructs a Guard that owns the pre-claim hook retry
// state for one worker run.
func NewPreClaimGuard(hook PreClaimHook, store cooldownStore, log io.Writer, now func() time.Time, cooldown, timeout time.Duration) *PreClaimGuard {
	if now == nil {
		now = time.Now
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	if timeout <= 0 {
		timeout = DefaultPreClaimTimeout
	}
	return &PreClaimGuard{
		hook:       hook,
		store:      store,
		log:        log,
		now:        now,
		cooldown:   cooldown,
		timeout:    timeout,
		failCounts: make(map[string]int),
	}
}

// Allow runs the pre-claim hook and applies the two-strikes retry policy.
func (g *PreClaimGuard) Allow(ctx context.Context, beadID string) (bool, string) {
	if g == nil || g.hook == nil {
		return true, ""
	}
	err, timedOut := callPreClaimHookWithTimeout(ctx, g.hook, g.timeout)
	if err != nil || timedOut {
		g.mu.Lock()
		defer g.mu.Unlock()

		g.failCounts[beadID]++
		if g.failCounts[beadID] >= 2 {
			if g.store != nil {
				until := g.now().UTC().Add(g.cooldown)
				detail := "pre-claim hook failed"
				if err != nil {
					detail = err.Error()
				} else if timedOut {
					detail = fmt.Sprintf("pre-claim hook timed out after %s", g.timeout)
				}
				_ = g.store.SetExecutionCooldown(beadID, until, "preclaim-hook-failed", detail, "")
			}
		}
		if g.log != nil {
			switch {
			case timedOut:
				_, _ = fmt.Fprintf(g.log, "pre-claim hook timed out after %s (skipping %s)\n", g.timeout, beadID)
			case err != nil:
				_, _ = fmt.Fprintf(g.log, "pre-claim hook: %v (skipping %s)\n", err, beadID)
			}
		}
		if timedOut {
			return false, fmt.Sprintf("pre-claim hook timed out after %s", g.timeout)
		}
		return false, err.Error()
	}

	g.mu.Lock()
	delete(g.failCounts, beadID)
	g.mu.Unlock()
	return true, ""
}

type preClaimHookCallResult struct {
	err error
}

func callPreClaimHookWithTimeout(ctx context.Context, hook PreClaimHook, timeout time.Duration) (error, bool) {
	if hook == nil {
		return nil, false
	}
	if timeout <= 0 {
		timeout = DefaultPreClaimTimeout
	}
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan preClaimHookCallResult, 1)
	go func() {
		resultCh <- preClaimHookCallResult{err: hook(hookCtx)}
	}()

	select {
	case result := <-resultCh:
		if errors.Is(hookCtx.Err(), context.DeadlineExceeded) || errors.Is(result.err, context.DeadlineExceeded) {
			return context.DeadlineExceeded, true
		}
		return result.err, false
	case <-hookCtx.Done():
		if errors.Is(hookCtx.Err(), context.DeadlineExceeded) {
			return context.DeadlineExceeded, true
		}
		return hookCtx.Err(), false
	}
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
