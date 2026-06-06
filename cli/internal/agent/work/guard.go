package work

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/trackerpaths"
)

// DefaultPreClaimTimeout bounds pre-claim readiness hooks when callers do not
// provide an explicit timeout.
const DefaultPreClaimTimeout = 5 * time.Minute

const SystemicPreClaimReasonPrefix = "systemic_preclaim:"

// TrackerContentionPreClaimReasonPrefix marks a pre-claim skip caused by
// transient multi-worker contention on DDx-managed tracker/metadata files
// (.ddx/beads.jsonl, .ddx/metrics/attempts.jsonl, …) rather than a systemic
// repo-config problem. Like systemic skips it idles without cooldowning the
// bead, but it carries a distinct reason code so operators can tell transient
// contention apart from a configuration wedge (ddx-df77e668).
const TrackerContentionPreClaimReasonPrefix = "tracker_contention_preclaim:"

// preClaimClass categorizes a pre-claim hook error so the guard can choose
// between idling without cooldown (systemic / tracker-contention) and the
// per-bead two-strikes cooldown policy (bead-specific failures).
type preClaimClass int

const (
	preClaimClassBeadSpecific preClaimClass = iota
	preClaimClassSystemic
	preClaimClassTrackerContention
)

// Guard decides whether a bead may proceed. Callers use the returned reason
// for skip telemetry and logs.
type Guard interface {
	Allow(ctx context.Context, beadID string) (bool, string)
}

// cooldownStore is the minimal store surface PreClaimGuard needs.
type cooldownStore interface {
	SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error
}

// PreClaimHook matches the work pre-claim hook signature.
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

	mu             sync.Mutex
	failCounts     map[string]int
	loggedSystemic map[string]bool
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
		hook:           hook,
		store:          store,
		log:            log,
		now:            now,
		cooldown:       cooldown,
		timeout:        timeout,
		failCounts:     make(map[string]int),
		loggedSystemic: make(map[string]bool),
	}
}

// Allow runs the pre-claim hook and applies the two-strikes retry policy.
func (g *PreClaimGuard) Allow(ctx context.Context, beadID string) (bool, string) {
	if g == nil || g.hook == nil {
		return true, ""
	}
	err, timedOut := callPreClaimHookWithTimeout(ctx, g.hook, g.timeout)
	if err != nil || timedOut {
		switch classifyPreClaimError(err) {
		case preClaimClassTrackerContention:
			reason := err.Error()
			g.mu.Lock()
			shouldLog := !g.loggedSystemic[reason]
			g.loggedSystemic[reason] = true
			g.mu.Unlock()
			if shouldLog && g.log != nil {
				_, _ = fmt.Fprintf(g.log, "pre-claim hook: %v (transient tracker-file contention; leaving beads untouched)\n", err)
			}
			return false, TrackerContentionPreClaimReasonPrefix + reason
		case preClaimClassSystemic:
			reason := err.Error()
			g.mu.Lock()
			shouldLog := !g.loggedSystemic[reason]
			g.loggedSystemic[reason] = true
			g.mu.Unlock()
			if shouldLog && g.log != nil {
				_, _ = fmt.Fprintf(g.log, "pre-claim hook: %v (systemic; leaving beads untouched)\n", err)
			}
			return false, SystemicPreClaimReasonPrefix + reason
		}

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

func IsSystemicPreClaimSkipReason(reason string) bool {
	return strings.HasPrefix(reason, SystemicPreClaimReasonPrefix)
}

func SystemicPreClaimDetail(reason string) string {
	return strings.TrimPrefix(reason, SystemicPreClaimReasonPrefix)
}

func IsTrackerContentionPreClaimSkipReason(reason string) bool {
	return strings.HasPrefix(reason, TrackerContentionPreClaimReasonPrefix)
}

// IsManagedTrackerPath reports whether path is one of the DDx-managed tracker
// or durable-audit files that workers rewrite continuously.
func IsManagedTrackerPath(path string) bool {
	return trackerpaths.IsManagedTrackerPath(path)
}

func TrackerContentionPreClaimDetail(reason string) string {
	return strings.TrimPrefix(reason, TrackerContentionPreClaimReasonPrefix)
}

// classifyPreClaimError categorizes a pre-claim hook error. Systemic and
// tracker-contention classes both idle the worker without cooldowning the bead
// (no bead is at fault); they differ only in the reason code surfaced to
// operators. Anything else is treated as bead-specific and runs through the
// two-strikes cooldown policy.
func classifyPreClaimError(err error) preClaimClass {
	if err == nil {
		return preClaimClassBeadSpecific
	}
	msg := err.Error()
	// Staged tracker-file contention: a landing-worktree "staged changes" error
	// whose staged paths are exclusively DDx-managed tracker/metadata files.
	// Another worker's tracker commit is partially staged; the index settles
	// within milliseconds. Treating this as bead-specific would wrongly cooldown
	// beads during a transient multi-worker commit race (ddx-df77e668).
	if isStagedTrackerContentionError(msg) {
		return preClaimClassTrackerContention
	}
	// Branch divergence: local tracker has diverged from origin.
	if strings.Contains(msg, "local branch ") && strings.Contains(msg, " has diverged from origin ") {
		return preClaimClassSystemic
	}
	// Staged changes that include code/doc/test files: real work was never
	// committed. This is a systemic repo state, not a transient contention.
	if strings.Contains(msg, "staged changes") {
		return preClaimClassSystemic
	}
	// Git index.lock: an external git process holds the index lock. This is
	// transient (sub-second for normal operations); no bead is at fault.
	if strings.Contains(msg, "index.lock") {
		return preClaimClassSystemic
	}
	// Corrupt landing indexes are repo-health failures, not bead-specific
	// failures. Surface once as systemic so the worker can auto-repair or ask
	// for operator intervention without cooldowning beads.
	if strings.Contains(msg, "index file smaller than expected") ||
		strings.Contains(msg, "repairing landing worktree index") {
		return preClaimClassSystemic
	}
	return preClaimClassBeadSpecific
}

// isStagedTrackerContentionError reports whether a "landing worktree has staged
// changes" error lists only DDx-managed tracker/metadata files. A mixed or
// code-only staged set is NOT tracker contention (it is systemic).
func isStagedTrackerContentionError(msg string) bool {
	if !strings.Contains(msg, "staged changes after waiting") {
		return false
	}
	paths := stagedPathsFromError(msg)
	if len(paths) == 0 {
		return false
	}
	for _, p := range paths {
		if !isTrackerMetadataPath(p) {
			return false
		}
	}
	return true
}

// stagedPathsFromError extracts the repo-relative paths from the
// "landing worktree has staged changes after waiting <d>:\n<status>\t<path>"
// error body. Each path line is "<status><TAB><path>" (git diff --name-status);
// the leading summary line (before the first newline) is skipped.
func stagedPathsFromError(msg string) []string {
	lines := strings.Split(msg, "\n")
	if len(lines) <= 1 {
		return nil
	}
	var paths []string
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if tab := strings.IndexByte(line, '\t'); tab >= 0 {
			line = line[tab+1:]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

// isTrackerMetadataPath reports whether a repo-relative path is one of the
// DDx-managed tracker/metadata files.
func isTrackerMetadataPath(path string) bool {
	return trackerpaths.IsManagedTrackerPath(path)
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
