package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/lockmetrics"
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

// trackerLockPath returns the main-git lock directory for the given project
// root. The historical name remains because tests and older comments refer to
// the tracker lock, but the lock now protects all primary-checkout git writes:
// tracker commits, pre-dispatch checkpoint/ref updates, and landing.
//
// The lock target must be shared across linked worktrees for the same repo, so
// we prefer the primary DDx workspace when one exists instead of the caller's
// checkout-local .ddx directory.
func trackerLockPath(projectRoot string) string {
	return lockmetrics.SharedTrackerLockPath(projectRoot)
}

// withMainGitLock acquires the process-shared main-git lock for the
// given project root, runs fn, and releases the lock. The lock is a
// directory created via os.Mkdir (atomic across processes on POSIX and
// Windows) following the same pattern as cli/internal/bead/lock.go.
//
// section identifies the call site (e.g. "durable_audit", "pre_dispatch",
// "land") and is surfaced in TrackerLockSample.Section and in the lockmetrics
// operation label so slow holds can be attributed without reading source.
//
// Contention is handled per LockRetryPolicy: exponential backoff up to a
// bounded budget. Per TD-031 §8.5 the contention itself does not change
// bead status; on budget exhaustion the caller surfaces an ordinary
// execution_failed and triage decides next steps.
func withMainGitLock(projectRoot, section string, fn func() error) error {
	if projectRoot == "" {
		return fn()
	}
	return withTrackerLockPolicy(projectRoot, section, trackerLockPolicy, fn)
}

// WithMainGitLock exposes the process-shared main-git lock to sibling
// packages that need the same serialized add/commit boundary as execute-bead
// landing and pre-dispatch checkpointing.
func WithMainGitLock(projectRoot, section string, fn func() error) error {
	return withMainGitLock(projectRoot, section, fn)
}

// withTrackerLock is retained for existing tracker/pre-dispatch call sites. It
// uses the same process-shared lock as landing so separate ddx work processes
// cannot interleave commits, ref updates, and checkout sync in the main tree.
func withTrackerLock(projectRoot, section string, fn func() error) error {
	return withMainGitLock(projectRoot, section, fn)
}

// TrackerLockSample carries timing and contention metrics for one
// withTrackerLock / withMainGitLock acquire+release cycle.
type TrackerLockSample struct {
	LockDir string
	Section string        // call-site identifier, e.g. "durable_audit", "pre_dispatch", "land"
	Wait    time.Duration // time from entry to lock acquisition
	Hold    time.Duration // time the callback held the lock
	Retries int           // number of sleep-and-retry iterations (0 = acquired on first try)
}

var (
	trackerLockSinkMu sync.RWMutex
	trackerLockSinkFn func(TrackerLockSample)
)

// TrackerLockTimeoutError marks a tracker-lock acquisition timeout so callers
// can classify it with errors.Is even when the error is wrapped several times.
type TrackerLockTimeoutError struct {
	Why      string
	LockDir  string
	OwnerPID string
}

// TrackerLockTimeoutErr is the sentinel used by errors.Is for transient
// tracker-lock contention.
var TrackerLockTimeoutErr = &TrackerLockTimeoutError{}

func (e *TrackerLockTimeoutError) Error() string {
	if e == nil {
		return "tracker lock timeout"
	}
	why := strings.TrimSpace(e.Why)
	if why == "" {
		why = "unknown"
	}
	lockDir := strings.TrimSpace(e.LockDir)
	if lockDir == "" {
		lockDir = "unknown"
	}
	owner := strings.TrimSpace(e.OwnerPID)
	if owner == "" {
		owner = "missing"
	}
	return fmt.Sprintf("tracker lock timeout (%s, lock: %s, owner pid: %s)", why, lockDir, owner)
}

func (e *TrackerLockTimeoutError) Is(target error) bool {
	_, ok := target.(*TrackerLockTimeoutError)
	return ok
}

// SetTrackerLockMetricsSink atomically installs a new tracker-lock
// metrics sink and returns the previous one. Passing nil disables
// emission. Safe for concurrent use; two workers in the same process
// will not race on the sink variable.
func SetTrackerLockMetricsSink(fn func(TrackerLockSample)) func(TrackerLockSample) {
	trackerLockSinkMu.Lock()
	prev := trackerLockSinkFn
	trackerLockSinkFn = fn
	trackerLockSinkMu.Unlock()
	return prev
}

// trackerLockContendedAttemptHook is a test-only hook fired after each
// failed acquire attempt that has been classified as real lock contention
// (a directory exists at lockDir and the stale-owner path did not reclaim
// it). It is invoked just before the retry sleep with the zero-based
// attempt index. The zero value (nil) is a no-op.
//
// Tests use this hook to deterministically synchronise with the retry
// loop — e.g. release the held lock from the hook so the next attempt
// observes a non-zero retry count without relying on time.Sleep races.
var trackerLockContendedAttemptHook func(attempt int)

// trackerLockOwnerTokenFile is optional lock metadata. Ordinary acquisition
// does not require it; tests may install it on a fresh replacement to prove
// a delayed stale observer cannot alter winner-owned bytes.
const trackerLockOwnerTokenFile = "owner_token"

type trackerStaleLockGuardStage string

const (
	trackerStaleGuardStageAttempted    trackerStaleLockGuardStage = "attempted"
	trackerStaleGuardStageContended    trackerStaleLockGuardStage = "contended"
	trackerStaleGuardStageAcquired     trackerStaleLockGuardStage = "acquired"
	trackerStaleGuardStageBeforeRename trackerStaleLockGuardStage = "before-rename"
)

type trackerStaleLockTransitionGuard struct {
	file  *os.File
	mutex *sync.Mutex
}

var (
	trackerStaleLockGuardMutexes sync.Map
	trackerStaleLockTombstoneSeq atomic.Uint64
)

// withTrackerLockPolicy is the policy-parameterised form of
// withTrackerLock; exposed at package scope so tests can pin a specific
// curve. section is the call-site identifier forwarded to TrackerLockSample
// and the lockmetrics operation label.
func withTrackerLockPolicy(projectRoot, section string, policy LockRetryPolicy, fn func() error) error {
	lockDir := trackerLockPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(lockDir), 0o755); err != nil {
		return fmt.Errorf("tracker lock dir: %w", err)
	}

	start := time.Now()
	retries := 0
	for attempt := 0; ; attempt++ {
		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			// Metadata publication is outside the stale-break guard: ordinary
			// acquisition never holds the transition sidecar.
			_ = os.WriteFile(filepath.Join(lockDir, "pid"),
				[]byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
			_ = os.WriteFile(filepath.Join(lockDir, "acquired_at"),
				[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
			break
		}

		// Classify what exists at lockDir before deciding to sleep or fail.
		// Per TD-031 §8.5: only a confirmed lock directory enters retry/backoff;
		// malformed paths are operator diagnostics, not lock contention.
		info, statErr := os.Lstat(lockDir)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				// Path disappeared between Mkdir and Lstat (race). Retry immediately.
				continue
			}
			return fmt.Errorf("tracker lock stat: %w", statErr)
		}

		// Directory and malformed regular-file stale objects share the same
		// single-winner guarded disposal. Observation made here is not
		// authoritative; breakStaleTrackerLock reclassifies under the guard.
		if info.Mode().IsDir() || info.Mode().IsRegular() {
			broke, breakErr := breakStaleTrackerLock(lockDir)
			if breakErr != nil {
				return fmt.Errorf("tracker lock: break stale: %w", breakErr)
			}
			if broke {
				// Guard is released inside break; ordinary Mkdir can retry.
				continue
			}
		}

		// Re-stat after a failed break so guard contention and races resolve
		// to the current object before the fail-fast/contention decision.
		info, statErr = os.Lstat(lockDir)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return fmt.Errorf("tracker lock stat: %w", statErr)
		}

		switch {
		case info.Mode().IsDir():
			if attempt >= policy.MaxRetries {
				return lockTimeoutError(lockDir, "max retries")
			}
			if policy.MaxElapsed > 0 && time.Since(start) >= policy.MaxElapsed {
				return lockTimeoutError(lockDir, "max elapsed")
			}
			retries++
			if hook := trackerLockContendedAttemptHook; hook != nil {
				hook(attempt)
			}
			// Retry sleep is outside the transition guard.
			time.Sleep(policy.step(attempt))

		case info.Mode().IsRegular():
			// Malformed: lock path must be a directory, not a regular file.
			// Over-age files that failed to break (guard contended) stay in the
			// LockRetryPolicy budget rather than failing fast.
			if time.Since(info.ModTime()) > trackerLockStaleAge {
				if attempt >= policy.MaxRetries {
					return lockTimeoutError(lockDir, "max retries")
				}
				if policy.MaxElapsed > 0 && time.Since(start) >= policy.MaxElapsed {
					return lockTimeoutError(lockDir, "max elapsed")
				}
				retries++
				if hook := trackerLockContendedAttemptHook; hook != nil {
					hook(attempt)
				}
				time.Sleep(policy.step(attempt))
				continue
			}
			return fmt.Errorf("tracker lock: malformed lock path %s is a regular file; a lock directory was expected — remove it manually to recover", lockDir)

		default:
			// Symlink, socket, device, or other special filesystem object. Do not remove.
			return fmt.Errorf("tracker lock: malformed lock path %s has type %v; a lock directory was expected — inspect and remove it manually to recover", lockDir, info.Mode().Type())
		}
	}

	waitDur := time.Since(start)
	holdStart := time.Now()
	// Ordinary owner release is the holder's private cleanup of the directory
	// it just created. Stale-break paths never RemoveAll the canonical path.
	defer os.RemoveAll(lockDir)
	op := section
	if op == "" {
		op = "tracker.commit"
	}
	// Callbacks and lockmetrics instrumentation run only after ordinary
	// acquisition and never while holding the stale-break transition guard.
	fnErr := lockmetrics.InstrumentCapped("tracker.lock", op, lockmetrics.CapConfigFor("tracker.lock"), fn)
	trackerLockSinkMu.RLock()
	sink := trackerLockSinkFn
	trackerLockSinkMu.RUnlock()
	if sink != nil {
		sink(TrackerLockSample{LockDir: lockDir, Section: section, Wait: waitDur, Hold: time.Since(holdStart), Retries: retries})
	}
	return fnErr
}

func lockTimeoutError(lockDir, why string) error {
	owner := "missing"
	if pidData, perr := os.ReadFile(filepath.Join(lockDir, "pid")); perr == nil && len(pidData) > 0 {
		owner = strings.TrimSpace(string(pidData))
	}
	return &TrackerLockTimeoutError{
		Why:      why,
		LockDir:  lockDir,
		OwnerPID: owner,
	}
}

// breakStaleTrackerLock serializes stale inspection and canonical-path mutation
// across processes. The stable advisory guard is intentionally separate from
// the canonical lock directory: ordinary acquisition, release, retry sleeps,
// git operations, and private tombstone removal never hold it.
//
// Returns true when this contender successfully renamed a freshly reclassified
// stale object to a contender-unique tombstone and removed that tombstone.
func breakStaleTrackerLock(lockDir string) (bool, error) {
	return breakStaleTrackerLockObserved(lockDir, nil)
}

func breakStaleTrackerLockObserved(lockDir string, observer func(trackerStaleLockGuardStage)) (bool, error) {
	guard, acquired, err := tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir, observer)
	if err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}

	// No observation made before taking the guard can authorize this rename.
	// Re-read and reclassify the current canonical path under the guard, then
	// rename only that freshly inspected stale object to a unique tombstone.
	tombstone, broke, renameErr := renameFreshlyStaleTrackerLockObserved(lockDir, observer)
	guardErr := releaseTrackerStaleLockBreakGuard(guard)
	if renameErr != nil {
		return false, errors.Join(renameErr, guardErr)
	}
	if guardErr != nil {
		return broke, guardErr
	}
	if !broke {
		return false, nil
	}
	// Remove only the exact tombstone this contender produced. Never remove
	// the canonical pathname from a stale-break path.
	if err := os.RemoveAll(tombstone); err != nil {
		return true, fmt.Errorf("remove stale tracker lock tombstone %s: %w", tombstone, err)
	}
	return true, nil
}

// renameFreshlyStaleTrackerLockObserved must only be called while the caller
// owns the stable stale-break guard for lockDir. It returns the unique
// tombstone owned by this transition; callers must never remove the canonical
// lock path.
func renameFreshlyStaleTrackerLockObserved(lockDir string, observer func(trackerStaleLockGuardStage)) (string, bool, error) {
	inspected, stale := freshlyInspectStaleTrackerLock(lockDir)
	if !stale {
		return "", false, nil
	}
	if observer != nil {
		observer(trackerStaleGuardStageBeforeRename)
	}

	// Revalidate after the pre-rename stage. Tests pause here to install a
	// fresh canonical owner; the only safe response is to decline the rename
	// when the identity changed after stale classification.
	current, err := os.Lstat(lockDir)
	if err != nil || !os.SameFile(inspected, current) {
		return "", false, nil
	}
	// Type must still match the inspected object (directory or regular file).
	if inspected.IsDir() != current.IsDir() ||
		inspected.Mode().IsRegular() != current.Mode().IsRegular() {
		return "", false, nil
	}

	tombstone := trackerStaleLockTombstonePath(lockDir)
	if tombstone == "" {
		return "", false, nil
	}
	if err := os.Rename(lockDir, tombstone); err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return tombstone, true, nil
}

// freshlyInspectStaleTrackerLock evaluates the stale criteria from the
// canonical path while its caller owns the stale-break guard.
//
// For lock directories the criteria intentionally remain ORed: either a valid
// dead owner PID or a valid over-age acquired_at timestamp is enough. Missing
// or malformed metadata does not itself authorize removal.
//
// For malformed regular files, over-age mtime is the sole stale criterion.
func freshlyInspectStaleTrackerLock(lockDir string) (os.FileInfo, bool) {
	inspected, err := os.Lstat(lockDir)
	if err != nil {
		return nil, false
	}

	stale := false
	switch {
	case inspected.IsDir():
		pidData, err := os.ReadFile(filepath.Join(lockDir, "pid"))
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
			if err == nil && pid > 0 && pid != os.Getpid() {
				if !trackerProcessAlive(pid) {
					stale = true
				}
			}
		}

		acquiredData, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
		if err == nil {
			acquired, err := time.Parse(time.RFC3339, strings.TrimSpace(string(acquiredData)))
			if err == nil && time.Since(acquired) > trackerLockStaleAge {
				stale = true
			}
		}
	case inspected.Mode().IsRegular():
		if time.Since(inspected.ModTime()) > trackerLockStaleAge {
			stale = true
		}
	default:
		return nil, false
	}

	current, err := os.Lstat(lockDir)
	if err != nil || !os.SameFile(inspected, current) {
		return nil, false
	}
	if inspected.IsDir() != current.IsDir() ||
		inspected.Mode().IsRegular() != current.Mode().IsRegular() {
		return nil, false
	}
	return inspected, stale
}

func trackerStaleLockBreakGuardPath(lockDir string) string {
	if lockDir == "" {
		return ""
	}
	return lockDir + ".stale-break.lock"
}

func trackerStaleLockTombstonePath(lockDir string) string {
	if lockDir == "" {
		return ""
	}
	suffix := fmt.Sprintf("%d-%d-%d", os.Getpid(), time.Now().UnixNano(), trackerStaleLockTombstoneSeq.Add(1))
	return filepath.Join(filepath.Dir(lockDir), filepath.Base(lockDir)+".tombstone."+suffix+".lock")
}

func trackerStaleLockGuardMutex(guardPath string) *sync.Mutex {
	mutex := &sync.Mutex{}
	actual, _ := trackerStaleLockGuardMutexes.LoadOrStore(guardPath, mutex)
	return actual.(*sync.Mutex)
}

func tryAcquireTrackerStaleLockBreakGuard(lockDir string) (*trackerStaleLockTransitionGuard, bool) {
	guard, acquired, _ := tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir, nil)
	return guard, acquired
}

func tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir string, observer func(trackerStaleLockGuardStage)) (*trackerStaleLockTransitionGuard, bool, error) {
	guardPath := trackerStaleLockBreakGuardPath(lockDir)
	if guardPath == "" {
		return nil, false, fmt.Errorf("tracker lock: stale-break guard requires lock directory")
	}
	if observer != nil {
		observer(trackerStaleGuardStageAttempted)
	}
	mutex := trackerStaleLockGuardMutex(guardPath)
	if !mutex.TryLock() {
		if observer != nil {
			observer(trackerStaleGuardStageContended)
		}
		return nil, false, nil
	}
	guard, err := os.OpenFile(guardPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		mutex.Unlock()
		return nil, false, err
	}
	locked, err := tryLockTrackerStaleBreakGuardFile(guard)
	if err != nil {
		_ = guard.Close()
		mutex.Unlock()
		return nil, false, err
	}
	if !locked {
		_ = guard.Close()
		mutex.Unlock()
		if observer != nil {
			observer(trackerStaleGuardStageContended)
		}
		return nil, false, nil
	}
	if observer != nil {
		observer(trackerStaleGuardStageAcquired)
	}
	return &trackerStaleLockTransitionGuard{file: guard, mutex: mutex}, true, nil
}

func releaseTrackerStaleLockBreakGuard(guard *trackerStaleLockTransitionGuard) error {
	if guard == nil {
		return nil
	}
	var err error
	if guard.file != nil {
		err = errors.Join(unlockTrackerStaleBreakGuardFile(guard.file), guard.file.Close())
	}
	if guard.mutex != nil {
		guard.mutex.Unlock()
	}
	return err
}
