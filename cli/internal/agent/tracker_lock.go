package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LockRetryPolicy describes the retry/backoff curve used when attempting to
// acquire a process-shared lock that guards git index operations on the
// primary .git (the tracker-commit lock). The policy is owned by this
// package per ddx-da11a34a AC#4: callers in cli/internal/triage and
// elsewhere consult ActionRetryWithBackoff but do not duplicate the curve
// implementation here.
//
// The curve is exponential with a cap and a bounded max-retries budget so
// that lock contention is absorbed silently up to a point, after which it
// surfaces as an ordinary error to the caller. Per TD-031 §8.5
// (LockContentionContract) lock contention does NOT change persisted bead
// status; the bead remains claimable and the worker remains `draining`.
type LockRetryPolicy struct {
	// InitialBackoff is the wait before the second attempt.
	InitialBackoff time.Duration
	// MaxBackoff caps the per-step backoff after exponential growth.
	MaxBackoff time.Duration
	// Multiplier is the per-step growth factor (>= 1.0).
	Multiplier float64
	// MaxRetries bounds the number of acquisition attempts after the
	// initial try. Total attempts = MaxRetries + 1.
	MaxRetries int
	// MaxElapsed bounds the total wall-clock time spent retrying. If both
	// MaxRetries and MaxElapsed are set, the first to trip wins.
	MaxElapsed time.Duration
}

// DefaultLockRetryPolicy returns the policy used by withTrackerLock. The
// curve is intentionally tight (sub-second) for in-process tracker-commit
// contention; the larger 30s/90s/300s curve referenced by the auto-triage
// child of ddx-3c154349 lives one layer up, in cli/internal/triage —
// it consults ActionRetryWithBackoff and re-queues across drain passes,
// while this policy resolves contention within a single attempt.
func DefaultLockRetryPolicy() LockRetryPolicy {
	return LockRetryPolicy{
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
		MaxRetries:     200,
		MaxElapsed:     30 * time.Second,
	}
}

// step returns the backoff duration for retry index n (0-based: n=0 is the
// wait BEFORE the second acquisition attempt).
func (p LockRetryPolicy) step(n int) time.Duration {
	if p.InitialBackoff <= 0 {
		return 0
	}
	d := float64(p.InitialBackoff)
	m := p.Multiplier
	if m < 1 {
		m = 1
	}
	for i := 0; i < n; i++ {
		d *= m
		if p.MaxBackoff > 0 && d >= float64(p.MaxBackoff) {
			d = float64(p.MaxBackoff)
			break
		}
	}
	out := time.Duration(d)
	if p.MaxBackoff > 0 && out > p.MaxBackoff {
		out = p.MaxBackoff
	}
	return out
}

// trackerLockStaleAge is the age after which a held tracker lock is
// considered stale (owner crashed mid-commit) and may be forcibly broken.
var trackerLockStaleAge = 10 * time.Minute

// trackerLockPolicy is the active retry policy for withTrackerLock.
// Tests may swap this out via the exported helpers below.
var trackerLockPolicy = DefaultLockRetryPolicy()

// trackerLockPath returns the tracker-commit lock directory for the given
// project root. The lock guards git index operations on the primary .git
// (staging and committing .ddx/beads.jsonl) so that multiple concurrent
// workers (e.g. several `ddx work` processes) do not race on
// .git/index.lock.
func trackerLockPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".ddx", ".git-tracker.lock")
}

// withTrackerLock acquires the process-shared tracker-commit lock for the
// given project root, runs fn, and releases the lock. The lock is a
// directory created via os.Mkdir (atomic across processes on POSIX and
// Windows) following the same pattern as cli/internal/bead/lock.go.
//
// Contention is handled per LockRetryPolicy: exponential backoff up to a
// bounded budget. Per TD-031 §8.5 the contention itself does not change
// bead status; on budget exhaustion the caller surfaces an ordinary
// execution_failed and triage decides next steps.
func withTrackerLock(projectRoot string, fn func() error) error {
	return withTrackerLockPolicy(projectRoot, trackerLockPolicy, fn)
}

// withTrackerLockPolicy is the policy-parameterised form of
// withTrackerLock; exposed at package scope so tests can pin a specific
// curve.
func withTrackerLockPolicy(projectRoot string, policy LockRetryPolicy, fn func() error) error {
	lockDir := trackerLockPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(lockDir), 0o755); err != nil {
		return fmt.Errorf("tracker lock dir: %w", err)
	}

	start := time.Now()
	for attempt := 0; ; attempt++ {
		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			_ = os.WriteFile(filepath.Join(lockDir, "pid"),
				[]byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
			_ = os.WriteFile(filepath.Join(lockDir, "acquired_at"),
				[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
			break
		}

		if breakStaleTrackerLock(lockDir) {
			continue
		}

		if attempt >= policy.MaxRetries {
			return lockTimeoutError(lockDir, "max retries")
		}
		if policy.MaxElapsed > 0 && time.Since(start) >= policy.MaxElapsed {
			return lockTimeoutError(lockDir, "max elapsed")
		}
		time.Sleep(policy.step(attempt))
	}

	defer os.RemoveAll(lockDir)
	return fn()
}

func lockTimeoutError(lockDir, why string) error {
	owner := "unknown"
	if pidData, perr := os.ReadFile(filepath.Join(lockDir, "pid")); perr == nil && len(pidData) > 0 {
		owner = strings.TrimSpace(string(pidData))
	}
	return fmt.Errorf("tracker lock timeout (%s, owner pid: %s)", why, owner)
}

// breakStaleTrackerLock removes lockDir if its owner process is dead or the
// lock is older than trackerLockStaleAge. Returns true if the lock was broken.
func breakStaleTrackerLock(lockDir string) bool {
	pidData, err := os.ReadFile(filepath.Join(lockDir, "pid"))
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if err == nil && pid > 0 && pid != os.Getpid() {
			if !trackerProcessAlive(pid) {
				os.RemoveAll(lockDir)
				return true
			}
		}
	}

	acquiredData, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
	if err == nil {
		acquired, err := time.Parse(time.RFC3339, strings.TrimSpace(string(acquiredData)))
		if err == nil && time.Since(acquired) > trackerLockStaleAge {
			os.RemoveAll(lockDir)
			return true
		}
	}

	return false
}
