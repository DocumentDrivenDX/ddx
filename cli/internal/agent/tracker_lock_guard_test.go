package agent

import (
	"context"
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
