package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

// TestWaitForEmptyGitIndex_IgnoresStagedTrackerFiles verifies ddx-df77e668
// AC #1: DDx-managed tracker files staged in the landing worktree must not
// block pre-claim. waitForEmptyGitIndex must return promptly even while
// .ddx/beads.jsonl and .ddx/metrics/attempts.jsonl are staged, because those
// append-mostly metadata files are continuously rewritten by other workers.
func TestWaitForEmptyGitIndex_IgnoresStagedTrackerFiles(t *testing.T) {
	r := newLandTestRepo(t)

	for _, rel := range []string{".ddx/beads.jsonl", ".ddx/metrics/attempts.jsonl", ".ddx/beads-archive.jsonl"} {
		r.writeFile(rel, "{\"id\":\"ddx-1\"}\n")
		r.runGit("add", rel)
	}

	// Sanity-check: the tracker files really are staged.
	staged := r.runGit("diff", "--cached", "--name-only")
	if !strings.Contains(staged, ".ddx/beads.jsonl") {
		t.Fatalf("setup did not stage tracker files; staged=%q", staged)
	}

	start := time.Now()
	if err := waitForEmptyGitIndex(r.dir, 2*time.Second); err != nil {
		t.Fatalf("staged tracker files must not block pre-claim: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("tracker-only staged state should resolve promptly, took %s", elapsed)
	}

	// The exemption must not silently clean the index — the staged tracker
	// files remain staged for the next durable-audit commit.
	if got := r.runGit("diff", "--cached", "--name-only"); !strings.Contains(got, ".ddx/beads.jsonl") {
		t.Fatalf("tracker files must remain staged, got %q", got)
	}
}

// TestWaitForEmptyGitIndex_StagedCodeStillBlocksAlongsideTracker verifies that
// the tracker exemption does not mask a genuine code change: when a code file
// is staged alongside tracker files, pre-claim must still surface the staged
// error (ddx-df77e668 — only tracker-only states are exempt).
func TestWaitForEmptyGitIndex_StagedCodeStillBlocksAlongsideTracker(t *testing.T) {
	r := newLandTestRepo(t)

	r.writeFile(".ddx/beads.jsonl", "{\"id\":\"ddx-1\"}\n")
	r.runGit("add", ".ddx/beads.jsonl")
	r.writeFile("main.go", "package main\n")
	r.runGit("add", "main.go")

	err := waitForEmptyGitIndex(r.dir, 100*time.Millisecond)
	if err == nil {
		t.Fatalf("staged code file must block pre-claim even with tracker files staged")
	}
	if !strings.Contains(err.Error(), "staged changes") {
		t.Fatalf("error = %v, want staged changes error", err)
	}
}

// TestWaitForEmptyGitIndex_RecoversFromCorruptIndex verifies that a truncated
// landing index is rebuilt instead of being misreported as staged work.
func TestWaitForEmptyGitIndex_RecoversFromCorruptIndex(t *testing.T) {
	r := newLandTestRepo(t)
	indexPath := filepath.Join(r.dir, ".git", "index")
	require.NoError(t, os.WriteFile(indexPath, []byte("bad"), 0o644))

	if err := waitForEmptyGitIndex(r.dir, 100*time.Millisecond); err != nil {
		t.Fatalf("waitForEmptyGitIndex did not recover corrupt index: %v", err)
	}

	if got := r.runGit("diff", "--cached", "--name-status"); got != "" {
		t.Fatalf("index should be clean after recovery, got %q", got)
	}
}
