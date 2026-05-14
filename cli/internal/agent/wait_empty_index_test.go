package agent

import (
	"strings"
	"testing"
	"time"
)

// TestWaitForEmptyGitIndex_RecoversFromStaleIndex simulates the
// queue-blocking state observed in the field: a prior land's
// post-merge SyncWorkTreeToHead failed silently (its error was swallowed
// by syncWorkTreeToHeadGuarded), leaving the index showing the reverse
// of the merge as staged changes. Without recovery, every subsequent
// pre-claim hook bounces with "landing worktree has staged changes",
// blocking the entire queue.
//
// waitForEmptyGitIndex must self-heal by running read-tree HEAD before
// reporting the failure.
func TestWaitForEmptyGitIndex_RecoversFromStaleIndex(t *testing.T) {
	r := newLandTestRepo(t)

	// Build the stuck state: HEAD has a commit that adds a file; the
	// index is left at the pre-commit state, so `git diff --cached`
	// sees a deletion staged against HEAD.
	r.writeFile("delta.txt", "added\n")
	r.runGit("add", "delta.txt")
	r.runGit("commit", "-m", "add delta")
	// Now HEAD contains delta.txt. Force the index back to the original
	// tree so `git diff --cached` reports delta.txt as deleted in the
	// index relative to HEAD — exactly the post-failed-sync residue.
	r.runGit("read-tree", r.baseSHA)

	// Sanity-check: the stuck state actually reproduces what we expect.
	staged := r.runGit("diff", "--cached", "--name-status")
	if !strings.Contains(staged, "delta.txt") {
		t.Fatalf("setup did not reproduce stuck state; staged=%q", staged)
	}

	// 100ms is well under any plausible operator-contention window, so
	// the test exercises the recovery path rather than waiting it out.
	if err := waitForEmptyGitIndex(r.dir, 100*time.Millisecond); err != nil {
		t.Fatalf("waitForEmptyGitIndex did not recover: %v", err)
	}

	// After recovery the index should match HEAD.
	if got := r.runGit("diff", "--cached", "--name-status"); got != "" {
		t.Fatalf("index still dirty after recovery: %q", got)
	}
}

// TestWaitForEmptyGitIndex_NoChangesReturnsImmediately is the
// happy-path check — a clean worktree resolves on the first poll.
func TestWaitForEmptyGitIndex_NoChangesReturnsImmediately(t *testing.T) {
	r := newLandTestRepo(t)
	start := time.Now()
	if err := waitForEmptyGitIndex(r.dir, 2*time.Second); err != nil {
		t.Fatalf("clean worktree should not error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("clean worktree should return promptly, took %s", elapsed)
	}
}

// TestWaitForEmptyGitIndex_PreservesOperatorWork verifies that real
// operator-staged work (a novel tree, not matching any ancestor) is
// surfaced as an error rather than silently overwritten by the
// recovery path.
func TestWaitForEmptyGitIndex_PreservesOperatorWork(t *testing.T) {
	r := newLandTestRepo(t)
	r.writeFile("operator.txt", "operator's local work\n")
	r.runGit("add", "operator.txt")

	err := waitForEmptyGitIndex(r.dir, 100*time.Millisecond)
	if err == nil {
		t.Fatalf("expected error for operator-staged work; got nil")
	}
	if !strings.Contains(err.Error(), "staged changes") {
		t.Fatalf("error = %v, want staged changes error", err)
	}

	// Operator's staged file must remain in the index.
	staged := r.runGit("diff", "--cached", "--name-only")
	if !strings.Contains(staged, "operator.txt") {
		t.Fatalf("operator work was lost; staged=%q", staged)
	}
}
