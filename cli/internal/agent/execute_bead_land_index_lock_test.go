package agent

// execute_bead_land_index_lock_test.go — recovery of stale .git/index.lock left
// behind when the trailing evidence commit is capped/cancelled mid-flight
// (ddx-9b012b6c). The land evidence commit runs under the index.lock cap; a
// hold past the cap force-releases the lock and can leave an interrupted
// commit's stale lock behind, which used to block the next tracker-only commit
// until an operator removed it manually.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/lockmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLandEvidenceCommit_IndexLockCapDoesNotLeaveStaleLock proves that when the
// evidence commit subprocess stalls past the index.lock cap and is cancelled,
// the worktree's stale .git/index.lock is recovered so nothing is left behind.
func TestLandEvidenceCommit_IndexLockCapDoesNotLeaveStaleLock(t *testing.T) {
	r := newLandTestRepo(t)
	lockPath := worktreeIndexLockPath(r.dir)

	t.Setenv("DDX_LOCK_CAP_INDEX_MS", "25")
	lockmetrics.SetCapEnforcement(r.dir, t.TempDir())
	t.Cleanup(func() { lockmetrics.SetCapEnforcement("", "") })
	lockmetrics.SetSink(nil)
	t.Cleanup(func() { lockmetrics.SetSink(nil) })

	origRunner := landCommitGitRunner
	t.Cleanup(func() { landCommitGitRunner = origRunner })

	var sawDeadline bool
	landCommitGitRunner = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		// Simulate git taking the index lock then stalling past the cap.
		require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))
		<-ctx.Done()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			sawDeadline = true
		}
		return nil, ctx.Err()
	}

	start := time.Now()
	_, err := commitStagedWithIndexLockRecovery(r.dir, "commit", "-m", "evidence")
	require.Error(t, err)
	assert.True(t, sawDeadline, "evidence commit must receive the cap-bounded deadline")
	assert.Less(t, time.Since(start), 500*time.Millisecond,
		"capped commit must not outlive the index lock cap budget")

	_, statErr := os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr),
		"capped/cancelled evidence commit must not leave a stale .git/index.lock")
}

// TestExecuteBeadLoop_IndexCommitLockViolationReleasesAndContinues proves the
// worker records the index.lock cap violation, recovers the stale lock, and a
// subsequent tracker-only commit proceeds without manual lock removal.
func TestExecuteBeadLoop_IndexCommitLockViolationReleasesAndContinues(t *testing.T) {
	r := newLandTestRepo(t)
	lockPath := worktreeIndexLockPath(r.dir)

	t.Setenv("DDX_LOCK_CAP_INDEX_MS", "25")
	lockmetrics.SetCapEnforcement(r.dir, t.TempDir())
	t.Cleanup(func() { lockmetrics.SetCapEnforcement("", "") })

	var eventsMu sync.Mutex
	var events []lockmetrics.Event
	lockmetrics.SetSink(func(ev lockmetrics.Event) {
		eventsMu.Lock()
		events = append(events, ev)
		eventsMu.Unlock()
	})
	t.Cleanup(func() { lockmetrics.SetSink(nil) })

	origRunner := landCommitGitRunner
	t.Cleanup(func() { landCommitGitRunner = origRunner })

	landCommitGitRunner = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		// Hold the index lock well past the cap so the watchdog records a
		// violation, then leave a stale lock behind as an interrupted commit
		// would after the watchdog's first force-release.
		require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))
		time.Sleep(120 * time.Millisecond)
		require.NoError(t, os.WriteFile(lockPath, []byte("recreated"), 0o644))
		return nil, ctx.Err()
	}

	_, err := commitStagedWithIndexLockRecovery(r.dir, "commit", "-m", "evidence")
	require.Error(t, err)

	// Restore the real runner so the follow-up commit reflects normal operation.
	landCommitGitRunner = origRunner

	eventsMu.Lock()
	var sawViolation bool
	for _, ev := range events {
		if ev.Event == "violation" && ev.LockName == "index.lock" {
			sawViolation = true
			assert.Equal(t, "error", ev.Severity, "index.lock violation must be error severity")
		}
	}
	eventsMu.Unlock()
	assert.True(t, sawViolation, "over-cap evidence commit must record an index.lock violation")

	_, statErr := os.Stat(lockPath)
	require.True(t, os.IsNotExist(statErr),
		"stale index.lock must be recovered after the capped commit")

	// A subsequent tracker-only commit must proceed without manual lock removal.
	r.writeFile("follow-up.txt", "after recovery\n")
	r.runGit("add", "-A")
	r.runGit("-c", "user.name=Test", "-c", "user.email=test@test.local", "commit", "-m", "follow up")
}

// TestLandEvidenceCommit_StaleLockRecoveryLeavesLiveOwnerLock proves the cancel
// recovery distinguishes a verified-stale lock (no live owner → removed) from a
// lock held by a live process (left in place).
func TestLandEvidenceCommit_StaleLockRecoveryLeavesLiveOwnerLock(t *testing.T) {
	r := newLandTestRepo(t)
	lockPath := worktreeIndexLockPath(r.dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(lockPath), 0o755))

	origLookup := indexLockOwnerLookup
	t.Cleanup(func() { indexLockOwnerLookup = origLookup })

	// Live owner: a running process holds the lock → it must NOT be removed.
	require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))
	indexLockOwnerLookup = func(string) (int, error) { return os.Getpid(), nil }
	recoverStaleIndexLockAfterCommitCancel(r.dir)
	_, statErr := os.Stat(lockPath)
	assert.NoError(t, statErr, "a lock held by a live owner must not be removed")

	// Verified-stale: no live owner → the lock is removed.
	indexLockOwnerLookup = func(string) (int, error) { return 0, nil }
	recoverStaleIndexLockAfterCommitCancel(r.dir)
	_, statErr = os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr), "a lock with no live owner must be removed")
}
