package agent

import (
	"context"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// worktreeIsClean reports whether dir has no uncommitted tracked changes
// (staged or unstaged) and no untracked/ignored dirt reported by porcelain
// status. Used for preconditions/postconditions that must observe the full
// working-tree state, not just the index (indexIsClean checks staged only).
func worktreeIsClean(t *testing.T, dir string) bool {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").CombinedOutput()
	require.NoError(t, err)
	return len(out) == 0
}

// pathIsDirty reports whether path has an uncommitted tracked change in dir,
// scoped to that single pathspec so unrelated untracked cruft elsewhere in
// the worktree (e.g. loop bookkeeping under .ddx/workers/) cannot affect the
// result.
func pathIsDirty(t *testing.T, dir, path string) bool {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--", path).CombinedOutput()
	require.NoError(t, err)
	return len(out) != 0
}

// TestPreClaimGate_RemediatesSafeDirtBeforeClaim guards ddx-2a7cfa3f: a
// worker whose only project-root dirt is DDx-owned tracked state (e.g.
// .ddx/beads.jsonl) must checkpoint-commit it via the unjam classification
// and claim in the same iteration, instead of parking with
// operator_attention.
func TestPreClaimGate_RemediatesSafeDirtBeforeClaim(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	r := newLandTestRepo(t)
	r.writeFile(".ddx/beads.jsonl", `{"id":"ddx-1"}`+"\n")
	r.runGit("add", "-f", ".ddx/beads.jsonl")
	r.runGit("commit", "-m", "seed tracker")
	r.writeFile(".ddx/beads.jsonl", `{"id":"ddx-1","status":"updated"}`+"\n")
	require.True(t, pathIsDirty(t, r.dir, ".ddx/beads.jsonl"), "precondition: tracker file has an uncommitted change")

	var execCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-preclaim-remediate",
				ResultRev: "abc1234",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: r.dir,
		SessionID:   "sess-preclaim-remediate",
		WorkerID:    "worker-preclaim-remediate",
		ProjectRootDirtyCheck: func(_ string) []string {
			return []string{".ddx/beads.jsonl"}
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Nil(t, result.OperatorAttention, "DDx-owned dirt must be remediated, not parked")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "claim must proceed in the same iteration")
	assert.Equal(t, int32(1), atomic.LoadInt32(&execCalls))

	got, err := inner.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, "closed", got.Status)

	assert.False(t, pathIsDirty(t, r.dir, ".ddx/beads.jsonl"), "checkpoint must resolve the tracker dirt")
	log := r.runGit("log", "--oneline", "-1")
	assert.Contains(t, log, "checkpoint ddx-owned state")
}

// TestGates_ParkOnForeignDirt guards ddx-2a7cfa3f: dirt that is neither
// DDx-owned nor backed by a preserved iteration ref (i.e. genuine
// operator-authored work) must still park with the exact path list in the
// operator_attention event body.
func TestGates_ParkOnForeignDirt(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	r := newLandTestRepo(t)
	r.writeFile("src/main.go", "package main\n")
	r.runGit("add", "src/main.go")
	r.runGit("commit", "-m", "seed")
	r.writeFile("src/main.go", "package main\n\nfunc main() {}\n")

	var execCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			t.Fatalf("executor must not run while foreign dirt is unresolved: %s", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        false,
		ProjectRoot: r.dir,
		SessionID:   "sess-foreign-dirt",
		WorkerID:    "worker-foreign-dirt",
		ProjectRootDirtyCheck: func(_ string) []string {
			return []string{"src/main.go"}
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls))
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls))
	require.NotNil(t, result.OperatorAttention)
	assert.Equal(t, "dirty_project_root", result.OperatorAttention.Reason)
	assert.Equal(t, []string{"src/main.go"}, result.OperatorAttention.DirtyPaths)
	assert.True(t, pathIsDirty(t, r.dir, "src/main.go"), "foreign dirt must be left untouched, not remediated away")
}

// TestLandGate_RemediatesSafeDirtBeforeLand guards ddx-2a7cfa3f: a landing
// worktree whose only staged changes are .ddx/executions/* evidence must
// proceed through the unjam-classification-backed recovery chain instead of
// erroring with "landing worktree has staged changes" (land_retry).
func TestLandGate_RemediatesSafeDirtBeforeLand(t *testing.T) {
	r := newLandTestRepo(t)
	r.writeFile(".ddx/executions/20260824T000000-remediate/result.json", `{"status":"ok"}`)
	r.writeFile(".ddx/executions/20260824T000000-remediate/manifest.json", `{}`)
	r.runGit("add", "-f", ".ddx/executions")
	require.False(t, indexIsClean(t, r.dir), "precondition: execution evidence is staged")

	err := waitForEmptyGitIndex(r.dir, 200*time.Millisecond)
	require.NoError(t, err, "staged execution evidence must be remediated instead of land_retry")
	assert.True(t, indexIsClean(t, r.dir))
}

// TestLandGate_RemediatesPreserveRefBackedDirtBeforeLand guards ddx-2a7cfa3f:
// staged content that overlaps a preserved iteration ref
// (refs/ddx/iterations/...) is a leaked checkout fragment, not new local
// work — it must be stashed back under that ref (recoverLandingIndexLocked /
// recoverLandingIndexPreserveRefBackedDirt) rather than silently folded into
// the land's substantive checkpoint commit.
func TestLandGate_RemediatesPreserveRefBackedDirtBeforeLand(t *testing.T) {
	r := newLandTestRepo(t)
	r.writeFile("src/leaked.go", "package src\n\nfunc Leaked() {}\n")
	r.runGit("add", "src/leaked.go")
	r.runGit("commit", "-m", "iteration work")
	preserveSHA := r.resolveRef("HEAD")
	r.runGit("update-ref", "refs/ddx/iterations/ddx-test/1-abcdef", preserveSHA)
	r.runGit("reset", "--soft", "HEAD~1")
	require.False(t, indexIsClean(t, r.dir), "precondition: leaked file is staged")

	err := waitForEmptyGitIndex(r.dir, 200*time.Millisecond)
	require.NoError(t, err, "preserve-ref-backed dirt must be stashed instead of land_retry")
	assert.True(t, indexIsClean(t, r.dir))

	// The content must be recoverable from the stash, not silently folded
	// into a checkpoint commit under HEAD.
	head := r.runGit("log", "--oneline", "-1")
	assert.NotContains(t, head, "checkpoint local tree before land")
	stashList := r.runGit("stash", "list")
	assert.Contains(t, stashList, "refs/ddx/iterations/ddx-test/1-abcdef")
}
