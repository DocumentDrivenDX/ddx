package agent

// durable_audit_index_lock_test.go — recovery of stale .git/index.lock left
// behind when the durable-audit git commit is capped/cancelled mid-flight
// (ddx-355bb190). The durable-audit commit runs under the index.lock cap; a
// hold past the cap force-releases the lock and can leave an interrupted
// commit's stale lock behind, which used to block follow-up commits until an
// operator removed it manually.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/lockmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommitDurableAuditOutputs_IndexLockCapDoesNotLeaveStaleLock proves that
// when the durable-audit commit subprocess stalls past the index.lock cap and
// is cancelled, any stale .git/index.lock it left behind is recovered so
// nothing blocks the next commit.
func TestCommitDurableAuditOutputs_IndexLockCapDoesNotLeaveStaleLock(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755))
	lockPath := filepath.Join(projectRoot, ".git", "index.lock")

	t.Setenv("DDX_LOCK_CAP_INDEX_MS", "25")
	lockmetrics.SetCapEnforcement(projectRoot, t.TempDir())
	t.Cleanup(func() { lockmetrics.SetCapEnforcement("", "") })
	lockmetrics.SetSink(nil)
	t.Cleanup(func() { lockmetrics.SetSink(nil) })

	origRunner := durableAuditGitRunner
	t.Cleanup(func() { durableAuditGitRunner = origRunner })

	var sawDeadline bool
	durableAuditGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
		// Simulate git taking the index lock, the cap watchdog fires and removes it,
		// then the subprocess re-creates the lock as an interrupted commit would.
		require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))
		time.Sleep(120 * time.Millisecond)
		require.NoError(t, os.WriteFile(lockPath, []byte("recreated"), 0o644))
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			sawDeadline = true
		}
		return nil, ctx.Err()
	}

	start := time.Now()
	_, err := runDurableAuditGitWithIndexLockRecovery(projectRoot, "commit", "--no-verify")
	require.Error(t, err)
	assert.True(t, sawDeadline, "durable-audit commit must receive the cap-bounded deadline")
	assert.Less(t, time.Since(start), 500*time.Millisecond,
		"capped durable-audit commit must not outlive the index lock cap budget by the old 30s")

	_, statErr := os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr),
		"capped/cancelled durable-audit commit must not leave a stale .git/index.lock")
}

// TestCommitDurableAuditOutputs_StaleLockRecoveryLeavesLiveOwnerLock proves
// the cancel recovery distinguishes a verified-stale lock (no live owner →
// removed) from a lock held by a live process (left in place).
func TestCommitDurableAuditOutputs_StaleLockRecoveryLeavesLiveOwnerLock(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755))
	lockPath := filepath.Join(projectRoot, ".git", "index.lock")

	origLookup := durableAuditIndexLockOwnerLookup
	t.Cleanup(func() { durableAuditIndexLockOwnerLookup = origLookup })

	// Live owner: a running process holds the lock — it must NOT be removed.
	require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))
	durableAuditIndexLockOwnerLookup = func(string) (int, error) { return os.Getpid(), nil }
	recoverStaleIndexLockAfterDurableAuditCancel(projectRoot)
	_, statErr := os.Stat(lockPath)
	assert.NoError(t, statErr, "a lock held by a live owner must not be removed")

	// Verified-stale: no live owner → the lock is removed.
	durableAuditIndexLockOwnerLookup = func(string) (int, error) { return 0, nil }
	recoverStaleIndexLockAfterDurableAuditCancel(projectRoot)
	_, statErr = os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr), "a lock with no live owner must be removed")
}

// TestCommitDurableAuditOutputs_CapTimeoutRecoversStaleIndexLock proves that a
// cap-cancelled durable-audit commit records the over-long hold, recovers the
// stale index.lock, and allows the next git operation to proceed without manual
// cleanup.
func TestCommitDurableAuditOutputs_CapTimeoutRecoversStaleIndexLock(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	lockPath := filepath.Join(projectRoot, ".git", "index.lock")

	t.Setenv("DDX_LOCK_CAP_INDEX_MS", "25")
	lockmetrics.SetCapEnforcement(projectRoot, t.TempDir())
	t.Cleanup(func() { lockmetrics.SetCapEnforcement("", "") })

	var eventsMu sync.Mutex
	var events []lockmetrics.Event
	lockmetrics.SetSink(func(ev lockmetrics.Event) {
		eventsMu.Lock()
		events = append(events, ev)
		eventsMu.Unlock()
	})
	t.Cleanup(func() { lockmetrics.SetSink(nil) })

	origRunner := durableAuditGitRunner
	t.Cleanup(func() { durableAuditGitRunner = origRunner })

	durableAuditGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
		// Hold the index lock well past the cap so the watchdog records a
		// violation, then leave a stale lock behind as an interrupted commit
		// would after the watchdog's first force-release.
		_ = os.WriteFile(lockPath, []byte("held"), 0o644)
		time.Sleep(120 * time.Millisecond)
		_ = os.WriteFile(lockPath, []byte("recreated"), 0o644)
		return nil, ctx.Err()
	}

	_, err := runDurableAuditGitWithIndexLockRecovery(projectRoot, "commit", "--no-verify")
	require.Error(t, err)

	// Restore the real runner so the follow-up commit reflects normal operation.
	durableAuditGitRunner = origRunner

	eventsMu.Lock()
	var sawViolation bool
	for _, ev := range events {
		if ev.Event == "violation" && ev.LockName == "index.lock" {
			sawViolation = true
			assert.Equal(t, "error", ev.Severity, "index.lock violation must be error severity")
		}
	}
	eventsMu.Unlock()
	assert.True(t, sawViolation, "over-cap durable-audit commit must record an index.lock violation")

	_, statErr := os.Stat(lockPath)
	require.True(t, os.IsNotExist(statErr),
		"stale index.lock must be recovered after the capped durable-audit commit")

	// A subsequent tracker-only or durable-audit commit must proceed without
	// manual rm .git/index.lock — run a real git commit to prove it.
	runGitInteg(t, projectRoot, "-c", "user.name=Test", "-c", "user.email=test@test.local",
		"commit", "--allow-empty", "-m", "follow-up after durable-audit lock recovery")
}

// TestCommitDurableAuditOutputs_IndexCommitStaysBelowCap proves that the
// durable-audit commit path stays comfortably below the active index.lock cap
// even when it has a large set of managed dirty outputs to stage and commit.
func TestCommitDurableAuditOutputs_IndexCommitStaysBelowCap(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	evidenceDir := t.TempDir()
	t.Setenv("DDX_LOCK_CAP_INDEX_MS", "")
	lockmetrics.SetCapEnforcement(projectRoot, evidenceDir)
	t.Cleanup(func() { lockmetrics.SetCapEnforcement("", "") })

	var eventsMu sync.Mutex
	var events []lockmetrics.Event
	lockmetrics.SetSink(func(ev lockmetrics.Event) {
		eventsMu.Lock()
		events = append(events, ev)
		eventsMu.Unlock()
	})
	t.Cleanup(func() { lockmetrics.SetSink(nil) })

	origRunner := durableAuditGitRunner
	t.Cleanup(func() { durableAuditGitRunner = origRunner })

	dirtyPaths := []string{
		".ddx/beads.jsonl",
		".ddx/beads-archive.jsonl",
		".ddx/metrics/attempts.jsonl",
	}
	for i := 0; i < 48; i++ {
		dirtyPaths = append(dirtyPaths, fmt.Sprintf(
			".ddx/attachments/ddx-cap-test-%02d/events-%02d.jsonl",
			i, i,
		))
	}

	var addArgs []string
	var commitBudget time.Duration
	durableAuditGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
		require.NotEmpty(t, args)
		switch args[0] {
		case "rev-parse":
			return []byte("true\n"), nil
		case "status":
			var status strings.Builder
			for _, path := range dirtyPaths {
				if strings.Contains(path, "/attachments/") {
					status.WriteString("?? ")
				} else {
					status.WriteString(" M ")
				}
				status.WriteString(path)
				status.WriteByte('\n')
			}
			return []byte(status.String()), nil
		case "add":
			addArgs = append([]string(nil), args...)
			return nil, nil
		case "diff":
			return []byte("diff --git a/.ddx/beads.jsonl b/.ddx/beads.jsonl\n"), nil
		case "commit":
			deadline, ok := ctx.Deadline()
			require.True(t, ok, "durable-audit index commit must receive a deadline")
			commitBudget = time.Until(deadline)
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected git command: %v", args)
		}
	}

	require.NoError(t, CommitDurableAuditOutputs(projectRoot, "20260616T220110-cap-safe"))
	require.Len(t, addArgs, 4+len(dirtyPaths))
	assert.Equal(t, "add", addArgs[0])
	assert.Equal(t, "-f", addArgs[1])
	assert.Equal(t, "-A", addArgs[2])
	assert.Equal(t, "--", addArgs[3])
	assert.Equal(t, dirtyPaths, addArgs[4:])
	assert.Greater(t, commitBudget, time.Duration(0))
	assert.Less(t, commitBudget, lockmetrics.DefaultIndexLockCap-250*time.Millisecond,
		"durable-audit commit budget must stay below the active cap with headroom")

	eventsMu.Lock()
	defer eventsMu.Unlock()
	for _, ev := range events {
		assert.NotEqual(t, "violation", ev.Event,
			"ordinary durable-audit commits must not emit index.lock violations")
	}
}
