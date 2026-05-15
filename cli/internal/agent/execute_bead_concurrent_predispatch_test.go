package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteBead_ConcurrentWorkers_NoHEADRefRace is the regression test for
// concurrent `ddx work` against the same project hitting
// "fatal: cannot lock ref 'HEAD': is at X but expected Y" CAS failures during
// the pre-execute-bead checkpoint.
//
// Before the fix, Run() called CommitTracker (locked) → SynthesizeCommit
// (unlocked) → resolveBase (unlocked) → WorktreeAdd (unlocked). When two
// workers raced, worker B's CommitTracker advanced HEAD between worker A's
// `git add` and `git commit` inside SynthesizeCommit, and A's commit failed
// the HEAD compare-and-swap.
//
// After the fix, the entire pre-dispatch sequence runs inside one
// withTrackerLock acquisition, so concurrent workers serialize through it.
func TestExecuteBead_ConcurrentWorkers_NoHEADRefRace(t *testing.T) {
	const n = 5
	projectRoot, _ := newScriptHarnessRepo(t, n)

	// Each goroutine gets its own directive file (no-op directive — we don't
	// care about the agent's output, only about the parent-repo
	// pre-dispatch sequence racing).
	dirFiles := make([]string, n)
	tmp := t.TempDir()
	for i := 0; i < n; i++ {
		dirFile := filepath.Join(tmp, fmt.Sprintf("directive-%d.txt", i+1))
		writeDirectiveFile(t, dirFile, []string{})
		dirFiles[i] = dirFile
	}

	// Build runners up front so worker goroutines do not share construction-time
	// setup.
	runners := make([]AgentRunner, n)
	rcfgs := make([]config.ResolvedConfig, n)
	for i := 0; i < n; i++ {
		runners[i] = NewRunner(Config{})
		rcfgs[i] = config.NewTestConfigForBead(config.TestBeadConfigOpts{
			Harness: "script",
			Model:   dirFiles[i],
		}).Resolve(config.CLIOverrides{})
	}

	// Seed allowed DDx bookkeeping dirt in the parent so the checkpoint
	// path has real work to do — exercises the `git add` / `git commit`
	// race window inside the locked critical section without tripping the
	// implementation-file rejection path.
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "run-state.json"),
		[]byte(`{"workers":"concurrent"}`+"\n"), 0o644))

	// Run n ExecuteBeadWithConfig calls concurrently against the SAME
	// projectRoot, with NO outer mutex serialising them. Without the
	// pre-dispatch lock-scope fix, this races on HEAD.
	var wg sync.WaitGroup
	results := make([]*ExecuteBeadResult, n)
	errs := make([]error, n)
	gitOps := &RealGitOps{}
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			beadID := fmt.Sprintf("ddx-int-%04d", i+1)
			results[i], errs[i] = ExecuteBeadWithConfig(
				context.Background(),
				projectRoot,
				beadID,
				rcfgs[i],
				ExecuteBeadRuntime{AgentRunner: runners[i]},
				gitOps,
			)
		}()
	}
	wg.Wait()

	// Primary regression assertion: no goroutine surfaced the
	// compare-and-swap error. Any "cannot lock ref" / "but expected"
	// failure means the race re-opened.
	for i, e := range errs {
		if e != nil {
			msg := e.Error()
			if IsGitUpdateRefCompareAndSwapFailure(msg) {
				t.Fatalf("worker %d hit HEAD CAS race (regression): %v", i, e)
			}
			require.NoErrorf(t, e, "worker %d returned unexpected error: %v", i, e)
		}
	}

	// Each goroutine must have produced a result with a baseRev resolved
	// from the locked sequence. The exact run outcome is not asserted here —
	// the script harness with an empty directive yields no_changes, which is
	// a successful pre-dispatch (we got past the worktree creation).
	seenBaseRevs := map[string]bool{}
	for i, r := range results {
		require.NotNilf(t, r, "worker %d nil result", i)
		assert.NotEmptyf(t, r.BaseRev, "worker %d BaseRev empty", i)
		seenBaseRevs[r.BaseRev] = true
	}

	// All n attempts ran serially through the lock — at least one tracker
	// commit and one checkpoint commit advanced main per first-acquirer.
	// Subsequent acquirers either no-op'd (dirt already committed) or
	// committed fresh dirt. Either way, the parent's HEAD is consistent
	// (only one writer at a time) and at least one commit landed beyond
	// the seed.
	commitCount := gitCommitCount(t, projectRoot, "HEAD")
	assert.GreaterOrEqualf(t, commitCount, 3,
		"main must have advanced past initial seed + bead seed (got %d commits)",
		commitCount)
}

// TestRun_LockScopeIncludesSynthesizeCommit pins the contract: the locked
// pre-dispatch critical section must hold the tracker lock through
// SynthesizeCommit. Without this, the original niflheim race re-opens —
// CommitTracker takes and releases the lock, then SynthesizeCommit's
// `git commit` races against a sibling worker's CommitTracker advancing
// HEAD.
//
// The test exercises the contract by spawning N goroutines that each:
//  1. Acquire withTrackerLock
//  2. Call commitTrackerLocked + SynthesizeCommit
//  3. Release the lock
//
// and asserting no compare-and-swap errors surface. This mirrors what
// Run()'s locked block now does.
func TestRun_LockScopeIncludesSynthesizeCommit(t *testing.T) {
	const n = 8
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	gitOps := &RealGitOps{}

	// Pre-seed each goroutine's distinct dirt file so SynthesizeCommit
	// always has something to stage, maximising the race window.
	for i := 0; i < n; i++ {
		path := filepath.Join(projectRoot, fmt.Sprintf("dirt-%d.txt", i))
		require.NoError(t, os.WriteFile(path, []byte("dirt\n"), 0o644))
	}

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = withTrackerLock(projectRoot, func() error {
				if err := commitTrackerLocked(projectRoot); err != nil {
					return err
				}
				_, err := gitOps.SynthesizeCommit(projectRoot,
					fmt.Sprintf("chore: checkpoint pre-execute-bead test-%d", i))
				return err
			})
		}()
	}
	wg.Wait()

	for i, e := range errs {
		if e == nil {
			continue
		}
		if IsGitUpdateRefCompareAndSwapFailure(e.Error()) ||
			strings.Contains(e.Error(), "cannot lock ref") {
			t.Fatalf("goroutine %d hit HEAD CAS race inside locked block: %v", i, e)
		}
		t.Fatalf("goroutine %d unexpected error: %v", i, e)
	}

	// HEAD must have a clean log — no two commits with the same parent
	// (which would indicate a divergence the lock should have prevented).
	out := runGitInteg(t, projectRoot, "log", "--format=%H %P")
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		// Each commit (except root) has exactly one parent; no merges.
		assert.LessOrEqual(t, len(fields), 2,
			"unexpected merge / multi-parent commit: %s", line)
	}
}
