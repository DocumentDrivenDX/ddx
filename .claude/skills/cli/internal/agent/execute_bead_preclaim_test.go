package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// FetchOriginAncestryCheck tests — real git in temp dirs, no mocks.
// ---------------------------------------------------------------------------

// setupBareOrigin creates a bare repo at originDir and clones it into workDir.
// Returns (workDir, originDir, initialSHA).
func setupBareOrigin(t *testing.T) (workDir, originDir, initialSHA string) {
	t.Helper()

	originDir = t.TempDir()
	workDir = t.TempDir()

	// Init bare origin.
	runGitInteg(t, originDir, "init", "--bare", "-b", "main")

	// Clone into workDir.
	runGitInteg(t, workDir, "clone", originDir, ".")
	runGitInteg(t, workDir, "config", "user.email", "test@ddx.test")
	runGitInteg(t, workDir, "config", "user.name", "DDx Test")

	// Make an initial commit in workDir and push.
	seedFile := filepath.Join(workDir, "seed.txt")
	require.NoError(t, os.WriteFile(seedFile, []byte("seed\n"), 0644))
	runGitInteg(t, workDir, "add", "seed.txt")
	runGitInteg(t, workDir, "commit", "-m", "chore: initial seed")
	runGitInteg(t, workDir, "push", "-u", "origin", "main")

	initialSHA = runGitInteg(t, workDir, "rev-parse", "HEAD")
	return workDir, originDir, initialSHA
}

// TestPreClaimNoOriginSkip verifies that FetchOriginAncestryCheck returns
// Action=="no-origin" when the repo has no remote configured.
func TestPreClaimNoOriginSkip(t *testing.T) {
	root := t.TempDir()
	runGitInteg(t, root, "init", "-b", "main")
	runGitInteg(t, root, "config", "user.email", "test@ddx.test")
	runGitInteg(t, root, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(root, "f.txt"), []byte("x\n"), 0644))
	runGitInteg(t, root, "add", ".")
	runGitInteg(t, root, "commit", "-m", "init")

	ops := RealLandingGitOps{}
	res, err := ops.FetchOriginAncestryCheck(root, "main")
	require.NoError(t, err)
	assert.Equal(t, "no-origin", res.Action)
}

// TestPreClaimOriginEqualNoOp verifies that when local == origin the result is
// Action=="unchanged" and no update-ref is performed.
func TestPreClaimOriginEqualNoOp(t *testing.T) {
	workDir, _, initialSHA := setupBareOrigin(t)

	ops := RealLandingGitOps{}
	res, err := ops.FetchOriginAncestryCheck(workDir, "main")
	require.NoError(t, err)
	assert.Equal(t, "unchanged", res.Action)
	assert.Equal(t, initialSHA, res.LocalSHA)
	assert.Equal(t, initialSHA, res.OriginSHA)
}

// TestPreClaimOriginAheadFastForward verifies that when origin has advanced the
// local branch is fast-forwarded to match and Action=="fast-forwarded".
func TestPreClaimOriginAheadFastForward(t *testing.T) {
	workDir, originDir, initialSHA := setupBareOrigin(t)

	// Add a commit directly to the bare origin via a second clone.
	secondDir := t.TempDir()
	runGitInteg(t, secondDir, "clone", originDir, ".")
	runGitInteg(t, secondDir, "config", "user.email", "test@ddx.test")
	runGitInteg(t, secondDir, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(secondDir, "extra.txt"), []byte("new\n"), 0644))
	runGitInteg(t, secondDir, "add", "extra.txt")
	runGitInteg(t, secondDir, "commit", "-m", "feat: extra commit")
	runGitInteg(t, secondDir, "push", "origin", "main")
	newSHA := runGitInteg(t, secondDir, "rev-parse", "HEAD")

	// workDir still points at initialSHA — origin is ahead.
	localBefore := runGitInteg(t, workDir, "rev-parse", "refs/heads/main")
	assert.Equal(t, initialSHA, localBefore)

	ops := RealLandingGitOps{}
	res, err := ops.FetchOriginAncestryCheck(workDir, "main")
	require.NoError(t, err)
	assert.Equal(t, "fast-forwarded", res.Action)
	assert.Equal(t, initialSHA, res.LocalSHA, "LocalSHA should be old tip before ff")
	assert.Equal(t, newSHA, res.OriginSHA)

	// Local branch should now be at the origin tip.
	localAfter := runGitInteg(t, workDir, "rev-parse", "refs/heads/main")
	assert.Equal(t, newSHA, localAfter, "local branch should be fast-forwarded")
}

// TestPreClaimLocalAheadNoOp verifies that when local is ahead of origin the
// result is Action=="local-ahead" and no modifications are made.
func TestPreClaimLocalAheadNoOp(t *testing.T) {
	workDir, _, _ := setupBareOrigin(t)

	// Add a local commit that hasn't been pushed.
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "local.txt"), []byte("local\n"), 0644))
	runGitInteg(t, workDir, "add", "local.txt")
	runGitInteg(t, workDir, "commit", "-m", "feat: local only")
	localSHA := runGitInteg(t, workDir, "rev-parse", "HEAD")

	ops := RealLandingGitOps{}
	res, err := ops.FetchOriginAncestryCheck(workDir, "main")
	require.NoError(t, err)
	assert.Equal(t, "local-ahead", res.Action)
	assert.Equal(t, localSHA, res.LocalSHA)

	// Local branch must be unchanged.
	assert.Equal(t, localSHA, runGitInteg(t, workDir, "rev-parse", "refs/heads/main"))
}

// TestPreClaimDivergedError verifies that when local and origin have diverged
// FetchOriginAncestryCheck returns Action=="diverged" and no modifications.
func TestPreClaimDivergedError(t *testing.T) {
	workDir, originDir, _ := setupBareOrigin(t)

	// Add a commit to origin via second clone.
	secondDir := t.TempDir()
	runGitInteg(t, secondDir, "clone", originDir, ".")
	runGitInteg(t, secondDir, "config", "user.email", "test@ddx.test")
	runGitInteg(t, secondDir, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(secondDir, "origin-change.txt"), []byte("origin\n"), 0644))
	runGitInteg(t, secondDir, "add", "origin-change.txt")
	runGitInteg(t, secondDir, "commit", "-m", "feat: origin commit")
	runGitInteg(t, secondDir, "push", "origin", "main")
	originNewSHA := runGitInteg(t, secondDir, "rev-parse", "HEAD")

	// Add a different local commit to workDir (diverge from the initial shared commit).
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "local-change.txt"), []byte("local\n"), 0644))
	runGitInteg(t, workDir, "add", "local-change.txt")
	runGitInteg(t, workDir, "commit", "-m", "feat: local diverging commit")
	localSHA := runGitInteg(t, workDir, "rev-parse", "HEAD")

	ops := RealLandingGitOps{}
	res, err := ops.FetchOriginAncestryCheck(workDir, "main")
	require.NoError(t, err)
	assert.Equal(t, "diverged", res.Action)
	assert.Equal(t, localSHA, res.LocalSHA)
	assert.Equal(t, originNewSHA, res.OriginSHA)

	// Local branch must be unchanged.
	assert.Equal(t, localSHA, runGitInteg(t, workDir, "rev-parse", "refs/heads/main"))
}

// ---------------------------------------------------------------------------
// PreClaimHook wiring in ExecuteBeadWorker.Run
// ---------------------------------------------------------------------------

// TestPreClaimHookDivergedSkipsBead verifies that when PreClaimHook returns an
// error the bead is not claimed and the loop moves on (NoReadyWork path).
func TestPreClaimHookDivergedSkipsBead(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	executed := []string{}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, ResultRev: "abc"}, nil
		}),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{
		Assignee: "worker",
		Once:     true,
		PreClaimHook: func(ctx context.Context) error {
			return fmt.Errorf("diverged: local=aaa origin=bbb")
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Executor must not have been called.
	assert.Empty(t, executed, "executor should not run when pre-claim hook returns error")
	assert.Equal(t, 0, result.Attempts)

	// Bead must still be open (not claimed).
	got, err := store.Get(candidate.ID)
	require.NoError(t, err)
	assert.NotEqual(t, "claimed", got.Status, "bead should not have been claimed")
}
