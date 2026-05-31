// Package failclass implements the FEAT-004 failure-classification routing
// policy applied by ddx work between bead-execution attempts (bead
// ddx-d27a1895).
//
// It provides four things the queue drain composes:
//
//   - Classify maps a single attempt Outcome to exactly one routing Action,
//     applying the normative failure-classification table.
//   - Escalate owns the single one-hop middle->max tier escalation, which only
//     a typed genuine failure triggers.
//   - Pool enforces a per-pool in-flight max-attempt cap that prevents a
//     multi-worker stampede when many workers fail concurrently.
//   - Pool also carries the global (per-pool, not per-bead) cooldown applied on
//     quota exhaustion.
//
// The classification table is normative; it is inlined here from the bead body
// because the upstream FEAT-004 routing addendum lives in the fizeau repo and
// is not reachable from a cold ddx agent.
package failclass

import (
	"sync"
	"time"
)

// Outcome is the typed result of a single ddx-work attempt. Every Outcome maps
// to exactly one Action via Classify.
type Outcome string

const (
	// OutcomeSuccess: the attempt satisfied the bead's acceptance criteria.
	OutcomeSuccess Outcome = "success"
	// OutcomeAlreadySatisfied: the bead was already satisfied before work.
	OutcomeAlreadySatisfied Outcome = "already_satisfied"
	// OutcomeNoChanges: the agent produced nothing and wrote a rationale.
	OutcomeNoChanges Outcome = "no_changes"
	// OutcomeGenuineFailure: tests fail after real work; acceptance unmet.
	// This is the only outcome that can trigger a tier escalation.
	OutcomeGenuineFailure Outcome = "genuine_failure"
	// OutcomeDirtyWorktree: dirty worktree or partial write.
	OutcomeDirtyWorktree Outcome = "dirty_worktree"
	// OutcomeTransport: provider transport failure (i/o timeout, conn refused,
	// 5xx).
	OutcomeTransport Outcome = "transport"
	// OutcomeQuota: quota exhausted (retry_after).
	OutcomeQuota Outcome = "quota"
	// OutcomeAuth: auth/config missing.
	OutcomeAuth Outcome = "auth"
	// OutcomeSetup: toolchain/setup failure.
	OutcomeSetup Outcome = "setup"
	// OutcomeTimeout: timeout / no-progress watchdog.
	OutcomeTimeout Outcome = "timeout"
	// OutcomeMergeConflict: merge/land conflict.
	OutcomeMergeConflict Outcome = "merge_conflict"
)

// Action is the single routing action selected for an Outcome.
type Action string

const (
	// ActionCloseBead: close the bead (success path).
	ActionCloseBead Action = "close_bead"
	// ActionCloseOrUnclaim: close or unclaim per the agent's rationale; never
	// escalate the tier.
	ActionCloseOrUnclaim Action = "close_or_unclaim"
	// ActionEscalateTier: escalate the tier exactly once (middle->max).
	ActionEscalateTier Action = "escalate_tier"
	// ActionResetRetrySameTier: clean/reset the worktree and retry at the same
	// tier (not an escalation).
	ActionResetRetrySameTier Action = "reset_retry_same_tier"
	// ActionRerouteSameTier: reroute to another dispatchable candidate at the
	// same tier; no tier change.
	ActionRerouteSameTier Action = "reroute_same_tier"
	// ActionGlobalCooldown: put the whole pool to sleep until quota reset; not
	// a per-bead mutation, not an escalation.
	ActionGlobalCooldown Action = "global_cooldown"
	// ActionOperatorAttention: explicit bead state transition to the operator
	// lane (not a notification or an indefinite wait).
	ActionOperatorAttention Action = "operator_attention"
	// ActionUnclaimRetryLater: unclaim and retry later; not an escalation.
	ActionUnclaimRetryLater Action = "unclaim_retry_later"
)

// Escalates reports whether the Action mutates the tier. Only
// ActionEscalateTier escalates; this is the single guard the escalation owner
// (ddx work) checks before calling Escalate.
func (a Action) Escalates() bool {
	return a == ActionEscalateTier
}

// ClassifyContext carries the routing state needed to resolve the tier- and
// attempt-conditional rows of the table (genuine failure and timeout).
type ClassifyContext struct {
	// AtMaxTier reports whether the attempt already ran at the maximum tier. A
	// genuine failure at the max tier goes to the operator lane instead of
	// escalating.
	AtMaxTier bool
	// SameTierRetried reports whether a same-tier retry was already consumed. A
	// second timeout (after one same-tier retry) goes to the operator lane.
	SameTierRetried bool
}

// Classify maps a single attempt Outcome to exactly one Action, applying the
// normative failure-classification table.
//
// Only OutcomeGenuineFailure can produce ActionEscalateTier, and only when the
// attempt was not already at the max tier. The no_changes, dirty, transport,
// quota, auth, setup, timeout, and merge-conflict outcomes never escalate.
func Classify(outcome Outcome, ctx ClassifyContext) Action {
	switch outcome {
	case OutcomeSuccess, OutcomeAlreadySatisfied:
		return ActionCloseBead
	case OutcomeNoChanges:
		return ActionCloseOrUnclaim
	case OutcomeGenuineFailure:
		// Single one-hop middle->max escalation; a max-tier genuine failure
		// stops escalating and goes to the operator lane.
		if ctx.AtMaxTier {
			return ActionOperatorAttention
		}
		return ActionEscalateTier
	case OutcomeDirtyWorktree:
		return ActionResetRetrySameTier
	case OutcomeTransport:
		return ActionRerouteSameTier
	case OutcomeQuota:
		return ActionGlobalCooldown
	case OutcomeAuth, OutcomeSetup:
		return ActionOperatorAttention
	case OutcomeTimeout:
		// Clean + same-tier retry once, then operator-attention.
		if ctx.SameTierRetried {
			return ActionOperatorAttention
		}
		return ActionResetRetrySameTier
	case OutcomeMergeConflict:
		return ActionUnclaimRetryLater
	default:
		// Unknown outcomes go to the operator lane rather than silently
		// escalating or retrying.
		return ActionOperatorAttention
	}
}

// Tier is a routing capability tier. Escalation is a single one-hop edge from
// TierMiddle to TierMax; no other edge escalates.
type Tier int

const (
	// TierLow is the cheapest tier and is never an escalation target.
	TierLow Tier = iota
	// TierMiddle is the default starting tier for dispatch.
	TierMiddle
	// TierMax is the strongest tier and the only escalation target.
	TierMax
)

// Escalate performs the single one-hop middle->max escalation and reports
// whether it changed the tier. Escalation is idempotent: only TierMiddle
// escalates (to TierMax); every other tier — including a tier already at
// TierMax — is returned unchanged with escalated=false. This enforces "exactly
// one middle->max" per dispatch chain.
func Escalate(t Tier) (next Tier, escalated bool) {
	if t == TierMiddle {
		return TierMax, true
	}
	return t, false
}

// Pool tracks per-pool in-flight attempts and the global cooldown for a single
// dispatch pool. It is safe for concurrent use by multiple workers.
//
// The in-flight cap prevents a multi-worker stampede: when many workers fail
// concurrently against the same pool, at most maxInFlight attempts may hold a
// slot at once. The cooldown is global to the pool (applied on quota
// exhaustion), not a per-bead mutation.
type Pool struct {
	mu            sync.Mutex
	name          string
	maxInFlight   int
	inFlight      int
	cooldownUntil time.Time
	now           func() time.Time
}

// NewPool returns a Pool with the given name and in-flight cap. A non-positive
// maxInFlight is clamped to 1. The clock defaults to time.Now and can be
// overridden for tests via SetClock.
func NewPool(name string, maxInFlight int) *Pool {
	if maxInFlight < 1 {
		maxInFlight = 1
	}
	return &Pool{
		name:        name,
		maxInFlight: maxInFlight,
		now:         time.Now,
	}
}

// SetClock overrides the pool's time source. It exists so tests can advance
// time without sleeping; the bead requires bounded, non-hanging tests.
func (p *Pool) SetClock(now func() time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.now = now
}

// Name returns the pool's name.
func (p *Pool) Name() string {
	return p.name
}

// Acquire attempts to take one in-flight slot. It returns false when the pool
// is in cooldown or the in-flight cap is already reached; otherwise it takes a
// slot and returns true. Every successful Acquire must be paired with a
// Release.
func (p *Pool) Acquire() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.now().Before(p.cooldownUntil) {
		return false
	}
	if p.inFlight >= p.maxInFlight {
		return false
	}
	p.inFlight++
	return true
}

// Release returns one in-flight slot. It never drops below zero.
func (p *Pool) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inFlight > 0 {
		p.inFlight--
	}
}

// InFlight returns the current number of held in-flight slots.
func (p *Pool) InFlight() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inFlight
}

// EnterCooldown applies a global pool cooldown of duration d (typically the
// quota retry_after). It is global to the pool, not a per-bead mutation:
// subsequent Acquire calls fail until the cooldown elapses. A later, longer
// cooldown extends the window; a shorter one never shortens an active window.
func (p *Pool) EnterCooldown(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	until := p.now().Add(d)
	if until.After(p.cooldownUntil) {
		p.cooldownUntil = until
	}
}

// InCooldown reports whether the pool is currently in a global cooldown.
func (p *Pool) InCooldown() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.now().Before(p.cooldownUntil)
}
