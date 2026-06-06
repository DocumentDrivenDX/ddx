package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// isolatedWorktreeGitOps is a GitOps stub for isolated worktree lifecycle
// tests. WorktreeAdd creates real directories (with a bead store) so the
// execution path can read bead context. WorktreeRemove deletes those
// directories, simulating the real git-worktree cleanup. Both operations are
// recorded so tests can assert on the call sequence.
type isolatedWorktreeGitOps struct {
	mu           sync.Mutex
	baseRev      string
	beadID       string
	addedPaths   []string
	addedRevs    []string
	removedPaths []string
}

func (g *isolatedWorktreeGitOps) HeadRev(dir string) (string, error) {
	return g.baseRev, nil
}

func (g *isolatedWorktreeGitOps) ResolveRev(dir, rev string) (string, error) {
	return g.baseRev, nil
}

func (g *isolatedWorktreeGitOps) WorktreeAdd(dir, wtPath, rev string) error {
	g.mu.Lock()
	g.addedPaths = append(g.addedPaths, wtPath)
	g.addedRevs = append(g.addedRevs, rev)
	g.mu.Unlock()

	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		return err
	}
	ddxDir := filepath.Join(wtPath, ddxroot.DirName)
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		return err
	}
	store := bead.NewStore(ddxDir)
	if err := store.Init(context.Background()); err != nil {
		return err
	}
	return store.Create(context.Background(), &bead.Bead{ID: g.beadID, Title: "Isolated worktree test"})
}

func (g *isolatedWorktreeGitOps) WorktreeRemove(dir, wtPath string) error {
	g.mu.Lock()
	g.removedPaths = append(g.removedPaths, wtPath)
	g.mu.Unlock()
	return os.RemoveAll(wtPath)
}

func (g *isolatedWorktreeGitOps) WorktreeList(dir string) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	removed := make(map[string]bool, len(g.removedPaths))
	for _, p := range g.removedPaths {
		removed[p] = true
	}
	var active []string
	for _, p := range g.addedPaths {
		if !removed[p] {
			active = append(active, p)
		}
	}
	return active, nil
}

func (g *isolatedWorktreeGitOps) WorktreePrune(dir string) error                 { return nil }
func (g *isolatedWorktreeGitOps) IsDirty(dir string) (bool, error)               { return false, nil }
func (g *isolatedWorktreeGitOps) SynthesizeCommit(dir, msg string) (bool, error) { return false, nil }
func (g *isolatedWorktreeGitOps) UpdateRef(dir, ref, sha string) error           { return nil }
func (g *isolatedWorktreeGitOps) DeleteRef(dir, ref string) error                { return nil }

// fixedExitCodeRunner is an AgentRunner stub that returns a fixed exit code
// without touching the filesystem.
type fixedExitCodeRunner struct {
	exitCode int
}

func (r fixedExitCodeRunner) Run(opts RunArgs) (*Result, error) {
	return &Result{ExitCode: r.exitCode}, nil
}

const (
	isolatedWtBeadID  = "ddx-isolated-wt-test"
	isolatedWtBaseRev = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func newIsolatedWorktreeGitOps() *isolatedWorktreeGitOps {
	return &isolatedWorktreeGitOps{
		baseRev: isolatedWtBaseRev,
		beadID:  isolatedWtBeadID,
	}
}

func runIsolatedWtAttempt(t *testing.T, gitOps *isolatedWorktreeGitOps, projectRoot string, exitCode int) (*ExecuteBeadResult, error) {
	t.Helper()
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{Harness: "test-harness"})
	return ExecuteBeadWithConfig(context.Background(), projectRoot, isolatedWtBeadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: fixedExitCodeRunner{exitCode: exitCode},
	}, gitOps)
}

// TestIsolatedWorktree_FailedAttemptLeavesCanonicalClean verifies that when an
// attempt fails (agent exits non-zero), the isolated worktree is removed so the
// canonical project root is left in a clean state with no dangling worktree
// directories.
func TestIsolatedWorktree_FailedAttemptLeavesCanonicalClean(t *testing.T) {
	execRoot := setExecutionWorktreeRootForTest(t)
	projectRoot := setupArtifactTestProjectRoot(t)
	gitOps := newIsolatedWorktreeGitOps()

	res, err := runIsolatedWtAttempt(t, gitOps, projectRoot, 1)
	if err != nil {
		t.Fatalf("unexpected error from failed attempt: %v", err)
	}
	if res == nil {
		t.Fatal("nil result from failed attempt")
	}
	if res.Outcome != ExecuteBeadOutcomeTaskFailed {
		t.Errorf("outcome = %q, want %q", res.Outcome, ExecuteBeadOutcomeTaskFailed)
	}

	// Exactly one worktree was created for the attempt.
	if len(gitOps.addedPaths) != 1 {
		t.Fatalf("WorktreeAdd calls = %d, want 1", len(gitOps.addedPaths))
	}
	wtPath := gitOps.addedPaths[0]

	// WorktreeRemove was called (deferred cleanup ran).
	if len(gitOps.removedPaths) != 1 || gitOps.removedPaths[0] != wtPath {
		t.Errorf("WorktreeRemove calls = %v, want [%s]", gitOps.removedPaths, wtPath)
	}

	// The worktree directory must not exist on disk after cleanup.
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("worktree %s still present on disk after failed attempt", wtPath)
	}

	// No worktree dirs remain in the execution temp root.
	entries, readErr := os.ReadDir(execRoot)
	if readErr != nil {
		t.Fatalf("reading execution root: %v", readErr)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ExecuteBeadWtPrefix) {
			t.Errorf("execution root has leftover worktree dir after failed attempt: %s", e.Name())
		}
	}
}

// TestIsolatedWorktree_RetryStartsFromCleanWorktree verifies that after a
// failed attempt, a retry creates a fresh isolated worktree at the same base
// revision rather than inheriting any state from the failed attempt.
func TestIsolatedWorktree_RetryStartsFromCleanWorktree(t *testing.T) {
	setExecutionWorktreeRootForTest(t)
	projectRoot := setupArtifactTestProjectRoot(t)
	gitOps := newIsolatedWorktreeGitOps()

	// First attempt fails.
	res1, err1 := runIsolatedWtAttempt(t, gitOps, projectRoot, 1)
	if err1 != nil || res1 == nil || res1.Outcome != ExecuteBeadOutcomeTaskFailed {
		t.Fatalf("first attempt: want task_failed with nil error, got result=%v err=%v", res1, err1)
	}
	if len(gitOps.addedPaths) != 1 {
		t.Fatalf("after first attempt: WorktreeAdd calls = %d, want 1", len(gitOps.addedPaths))
	}
	firstWtPath := gitOps.addedPaths[0]

	// The first worktree is cleaned up before the retry.
	if _, err := os.Stat(firstWtPath); !os.IsNotExist(err) {
		t.Errorf("first worktree %s not cleaned up before retry", firstWtPath)
	}

	// Second attempt (retry).
	res2, err2 := runIsolatedWtAttempt(t, gitOps, projectRoot, 1)
	if err2 != nil || res2 == nil || res2.Outcome != ExecuteBeadOutcomeTaskFailed {
		t.Fatalf("second attempt: want task_failed with nil error, got result=%v err=%v", res2, err2)
	}

	// Two distinct worktree paths — one per attempt.
	if len(gitOps.addedPaths) != 2 {
		t.Fatalf("WorktreeAdd calls = %d, want 2", len(gitOps.addedPaths))
	}
	secondWtPath := gitOps.addedPaths[1]
	if firstWtPath == secondWtPath {
		t.Errorf("retry reused worktree path %q; want a fresh isolated worktree", firstWtPath)
	}

	// Each worktree was created at the same clean base revision.
	for i, rev := range gitOps.addedRevs {
		if rev != isolatedWtBaseRev {
			t.Errorf("worktree[%d] created at rev %q, want %q", i, rev, isolatedWtBaseRev)
		}
	}

	// Both worktrees were cleaned up after their respective attempts.
	if len(gitOps.removedPaths) != 2 {
		t.Fatalf("WorktreeRemove calls = %d, want 2; remaining=%v", len(gitOps.removedPaths), gitOps.removedPaths)
	}
	if _, err := os.Stat(firstWtPath); !os.IsNotExist(err) {
		t.Errorf("first worktree %s still exists after retry", firstWtPath)
	}
	if _, err := os.Stat(secondWtPath); !os.IsNotExist(err) {
		t.Errorf("second worktree %s still exists after retry", secondWtPath)
	}
}

// TestIsolatedWorktree_PreserveAttemptWorktree keeps the attempt worktree on
// disk when the caller explicitly opts in to inspection preservation. This
// mirrors the `ddx try --no-merge` preserve path.
func TestIsolatedWorktree_PreserveAttemptWorktree(t *testing.T) {
	setExecutionWorktreeRootForTest(t)
	projectRoot := setupArtifactTestProjectRoot(t)
	gitOps := newIsolatedWorktreeGitOps()

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{Harness: "test-harness"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, isolatedWtBeadID, rcfg, ExecuteBeadRuntime{
		AgentRunner:             fixedExitCodeRunner{exitCode: 1},
		PreserveAttemptWorktree: true,
	}, gitOps)
	if err != nil {
		t.Fatalf("unexpected error from preserved attempt: %v", err)
	}
	if res == nil {
		t.Fatal("nil result from preserved attempt")
	}
	if res.Outcome != ExecuteBeadOutcomeTaskFailed {
		t.Fatalf("outcome = %q, want %q", res.Outcome, ExecuteBeadOutcomeTaskFailed)
	}
	if len(gitOps.addedPaths) != 1 {
		t.Fatalf("WorktreeAdd calls = %d, want 1", len(gitOps.addedPaths))
	}
	if len(gitOps.removedPaths) != 0 {
		t.Fatalf("WorktreeRemove calls = %v, want none for preserved attempt", gitOps.removedPaths)
	}
	if _, statErr := os.Stat(gitOps.addedPaths[0]); statErr != nil {
		t.Fatalf("preserved worktree %s should remain on disk: %v", gitOps.addedPaths[0], statErr)
	}
}
