package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	trackerGuardHelperEnv   = "DDX_TRACKER_STALE_GUARD_HELPER"
	trackerGuardModeEnv     = "DDX_TRACKER_STALE_GUARD_MODE"
	trackerGuardLockDirEnv  = "DDX_TRACKER_STALE_GUARD_LOCK_DIR"
	trackerGuardCoordDirEnv = "DDX_TRACKER_STALE_GUARD_COORD_DIR"
)

// TestTrackerStaleLockGuardCrashReleasesWithoutReplacingSidecar spawns a child
// that Acquires the advisory guard, signals after actual lock acquisition, is
// killed and waited on; a successor Acquire in the parent then succeeds, and
// os.SameFile proves the sidecar inode/path was never replaced.
func TestTrackerStaleLockGuardCrashReleasesWithoutReplacingSidecar(t *testing.T) {
	if runTrackerGuardHelper(t) {
		return
	}

	lockDir := filepath.Join(t.TempDir(), ".ddx", ".git-tracker.lock")
	require.NoError(t, os.MkdirAll(filepath.Dir(lockDir), 0o755))
	coordDir := filepath.Join(t.TempDir(), "guard-crash")
	require.NoError(t, os.MkdirAll(coordDir, 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	holder := spawnTrackerGuardHelper(ctx, "TestTrackerStaleLockGuardCrashReleasesWithoutReplacingSidecar", "crash-holder", lockDir, coordDir)
	require.NoError(t, holder.Start())
	require.NoError(t, waitForTrackerGuardFile(filepath.Join(coordDir, "guard-held"), 5*time.Second))

	guardPath := trackerStaleLockBreakGuardPath(lockDir)
	require.NotEmpty(t, guardPath, "sidecar path must be derived")
	require.True(t, strings.HasSuffix(guardPath, ".lock"))
	before, err := os.Stat(guardPath)
	require.NoError(t, err)

	require.NoError(t, holder.Process.Kill())
	require.Error(t, holder.Wait(), "killed guard holder must exit abnormally")

	require.Eventually(t, func() bool {
		guard, acquired, acquireErr := tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir, nil)
		if acquireErr != nil || !acquired {
			return false
		}
		require.NoError(t, releaseTrackerStaleLockBreakGuard(guard))
		return true
	}, 5*time.Second, 10*time.Millisecond, "advisory guard must release when its holder crashes")

	after, err := os.Stat(guardPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after), "crash + successor acquire must not replace the sidecar inode")

	// Ordinary acquire/release cycle must also leave the sidecar path intact.
	guard, acquired, err := tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir, nil)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, releaseTrackerStaleLockBreakGuard(guard))
	finalInfo, err := os.Stat(guardPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, finalInfo), "ordinary acquire/release must not replace the sidecar inode")
}

// TestTrackerStaleLockGuardSerializesSameProcess proves the path-keyed process
// mutex serializes two goroutine/local acquire attempts on one sidecar.
func TestTrackerStaleLockGuardSerializesSameProcess(t *testing.T) {
	lockDir := filepath.Join(t.TempDir(), ".ddx", ".git-tracker.lock")
	require.NoError(t, os.MkdirAll(filepath.Dir(lockDir), 0o755))

	first, acquired, err := tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir, nil)
	require.NoError(t, err)
	require.True(t, acquired)

	second, secondAcquired, err := tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir, nil)
	require.NoError(t, err)
	assert.False(t, secondAcquired, "keyed process mutex must serialize same-process guard attempts")
	assert.Nil(t, second)

	require.NoError(t, releaseTrackerStaleLockBreakGuard(first))
	third, thirdAcquired, err := tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir, nil)
	require.NoError(t, err)
	require.True(t, thirdAcquired)
	require.NoError(t, releaseTrackerStaleLockBreakGuard(third))
}

// TestTrackerStaleLockGuardOpenErrorFailsSafe proves a non-contention open
// error returns an error and never reports the guard as held.
func TestTrackerStaleLockGuardOpenErrorFailsSafe(t *testing.T) {
	lockDir := filepath.Join(t.TempDir(), ".ddx", ".git-tracker.lock")
	guardPath := trackerStaleLockBreakGuardPath(lockDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(guardPath), 0o755))
	// Place a directory at the sidecar path so O_RDWR open fails (EISDIR).
	require.NoError(t, os.Mkdir(guardPath, 0o755))

	guard, acquired, err := tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir, nil)
	assert.Error(t, err, "non-contention open failure must surface as an error")
	assert.False(t, acquired, "failed open must not report the guard as held")
	assert.Nil(t, guard)
}

// TestTrackerStaleLockGuardContentionRespectsPolicyDeadline holds the advisory
// transition guard in a child process while a breakable stale canonical lock
// exists, then proves parent acquisition returns *TrackerLockTimeoutError
// within the LockRetryPolicy MaxElapsed budget. The protected callback must
// not run, and the canonical lock object must not be removed, renamed, or
// recreated. Guard contention is absorbed by the same MaxRetries/MaxElapsed
// curve that governs ordinary lock-directory contention; the transition guard
// itself is never held across ordinary Mkdir, metadata publication, fn, or
// retry sleep (those paths only run after breakStale releases the guard).
func TestTrackerStaleLockGuardContentionRespectsPolicyDeadline(t *testing.T) {
	if runTrackerGuardHelper(t) {
		return
	}

	root := initTrackerRepo(t)
	lockDir := trackerLockPath(root)
	// Over-age acquired_at is an independent stale criterion (works on Windows
	// where trackerProcessAlive is conservative). With a free transition guard
	// this lock would be renamed away; under contention it must survive.
	writeStaleTrackerLockDir(t, lockDir, 0, time.Now().Add(-3*trackerLockStaleAge))

	beforeInfo, err := os.Lstat(lockDir)
	require.NoError(t, err)
	wantPID, err := os.ReadFile(filepath.Join(lockDir, "pid"))
	require.NoError(t, err)
	wantAcquired, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
	require.NoError(t, err)

	coordDir := filepath.Join(t.TempDir(), "guard-contention")
	require.NoError(t, os.MkdirAll(coordDir, 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Reuse crash-holder: acquires the real advisory guard, signals, and holds
	// until the parent kills the process (OS releases the lock on death).
	holder := spawnTrackerGuardHelper(ctx, "TestTrackerStaleLockGuardContentionRespectsPolicyDeadline", "crash-holder", lockDir, coordDir)
	require.NoError(t, holder.Start())
	defer killTrackerGuardHelper(holder)
	require.NoError(t, waitForTrackerGuardFile(filepath.Join(coordDir, "guard-held"), 5*time.Second))

	// Direct break must observe contention and leave the canonical path alone.
	broke, breakErr := breakStaleTrackerLock(lockDir)
	require.NoError(t, breakErr)
	require.False(t, broke, "contended transition guard must refuse the stale break")

	policy := LockRetryPolicy{
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
		// High MaxRetries so MaxElapsed is the budget that trips.
		MaxRetries: 10_000,
		MaxElapsed: 80 * time.Millisecond,
	}

	var contendedAttempts int
	prevHook := trackerLockContendedAttemptHook
	trackerLockContendedAttemptHook = func(int) { contendedAttempts++ }
	defer func() { trackerLockContendedAttemptHook = prevHook }()

	fnCalled := false
	start := time.Now()
	err = withTrackerLockPolicy(root, "guard_contention_deadline", policy, func() error {
		fnCalled = true
		return nil
	})
	elapsed := time.Since(start)

	require.Error(t, err)
	var timeoutErr *TrackerLockTimeoutError
	require.True(t, errors.As(err, &timeoutErr), "want *TrackerLockTimeoutError, got %T: %v", err, err)
	assert.False(t, fnCalled, "fn must not run when guard contention exhausts the policy budget")
	assert.Greater(t, contendedAttempts, 0, "guard contention must consume LockRetryPolicy MaxRetries/MaxElapsed budget")
	// Bounded deadline: MaxElapsed plus a couple of backoff steps and schedule slack.
	tolerance := policy.MaxElapsed + 2*policy.MaxBackoff + 250*time.Millisecond
	assert.LessOrEqual(t, elapsed, tolerance,
		"timeout must surface within MaxElapsed tolerance; elapsed=%v max=%v", elapsed, policy.MaxElapsed)
	assert.GreaterOrEqual(t, elapsed, policy.MaxElapsed/2,
		"must actually wait against the policy budget rather than failing instantly")

	// Canonical lock must be byte- and inode-identical (no remove/rename/recreate).
	afterInfo, err := os.Lstat(lockDir)
	require.NoError(t, err, "canonical lock must still exist under guard contention")
	assert.True(t, os.SameFile(beforeInfo, afterInfo), "canonical lock inode must not change under guard contention")
	assertTrackerFileBytesEqual(t, filepath.Join(lockDir, "pid"), wantPID)
	assertTrackerFileBytesEqual(t, filepath.Join(lockDir, "acquired_at"), wantAcquired)

	// Release the child so the advisory guard drops; the same stale object must
	// then break, proving the earlier timeout was specifically guard contention.
	killTrackerGuardHelper(holder)
	require.Eventually(t, func() bool {
		return mustBreakStaleTrackerLock(t, lockDir)
	}, 5*time.Second, 10*time.Millisecond, "after the guard holder exits, the stale lock must break")
	assert.NoDirExists(t, lockDir)
}

func killTrackerGuardHelper(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if cmd.ProcessState != nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

func runTrackerGuardHelper(t *testing.T) bool {
	t.Helper()
	if os.Getenv(trackerGuardHelperEnv) != "1" {
		return false
	}

	switch os.Getenv(trackerGuardModeEnv) {
	case "crash-holder":
		runTrackerGuardCrashHolderHelper(t)
	default:
		t.Fatalf("unknown tracker guard helper mode %q", os.Getenv(trackerGuardModeEnv))
	}
	return true
}

func runTrackerGuardCrashHolderHelper(t *testing.T) {
	t.Helper()
	lockDir := os.Getenv(trackerGuardLockDirEnv)
	coordDir := os.Getenv(trackerGuardCoordDirEnv)
	require.NotEmpty(t, lockDir)
	require.NotEmpty(t, coordDir)

	var signaled bool
	guard, acquired, err := tryAcquireTrackerStaleLockTransitionGuardObserved(lockDir, func(stage trackerStaleLockGuardStage) {
		if stage == trackerStaleGuardStageAcquired {
			require.NoError(t, os.WriteFile(filepath.Join(coordDir, "guard-held"), []byte("held"), 0o644))
			signaled = true
		}
	})
	require.NoError(t, err)
	require.True(t, acquired, "crash-holder must acquire the real advisory guard")
	require.True(t, signaled, "signal must follow actual advisory-lock acquisition")
	// Hold until the parent kills this process. Do not release on exit paths
	// that tests control — crash release is the contract under test.
	_ = guard
	for {
		time.Sleep(time.Hour)
	}
}

func spawnTrackerGuardHelper(ctx context.Context, testName, mode, lockDir, coordDir string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+testName+"$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		trackerGuardHelperEnv+"=1",
		trackerGuardModeEnv+"="+mode,
		trackerGuardLockDirEnv+"="+lockDir,
		trackerGuardCoordDirEnv+"="+coordDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func waitForTrackerGuardFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
