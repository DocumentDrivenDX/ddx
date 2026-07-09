package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTrackerSyncRepo(t *testing.T) (workDir, originDir string, store *bead.Store) {
	t.Helper()

	workDir, originDir, _ = setupBareOrigin(t)
	store = bead.NewStore(filepath.Join(workDir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	return workDir, originDir, store
}

func seedTrackerBeads(t *testing.T, store *bead.Store, beads ...*bead.Bead) {
	t.Helper()
	for _, b := range beads {
		require.NoError(t, store.Create(context.Background(), b))
	}
}

func commitAndPushTracker(t *testing.T, workDir, message string) {
	t.Helper()
	runGitInteg(t, workDir, "add", ".ddx/beads.jsonl")
	runGitInteg(t, workDir, "commit", "-m", message)
	runGitInteg(t, workDir, "push", "origin", "main")
}

func cloneBeadsJSONL(t *testing.T, originDir string) string {
	t.Helper()
	cloneDir := t.TempDir()
	runGitInteg(t, cloneDir, "clone", originDir, ".")
	raw, err := os.ReadFile(filepath.Join(cloneDir, ddxroot.DirName, "beads.jsonl"))
	require.NoError(t, err)
	return string(raw)
}

func readBeadStatus(t *testing.T, workDir, id string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(workDir, ddxroot.DirName, "beads.jsonl"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for _, line := range lines {
		var b bead.Bead
		require.NoError(t, json.Unmarshal([]byte(line), &b))
		if b.ID == id {
			return b.Status
		}
	}
	t.Fatalf("bead %s not found in tracker", id)
	return ""
}

func captureTrackerSyncEvents() (func(string, map[string]any), *bytes.Buffer, *[]map[string]any) {
	var log bytes.Buffer
	events := []map[string]any{}
	emit := func(kind string, payload map[string]any) {
		entry := map[string]any{"type": kind}
		for k, v := range payload {
			entry[k] = v
		}
		events = append(events, entry)
	}
	return emit, &log, &events
}

func TestExecuteLoop_PreClaimSyncFetchesBeforeCandidateSelection(t *testing.T) {
	workDir, originDir, store := setupTrackerSyncRepo(t)

	seedTrackerBeads(t, store,
		&bead.Bead{ID: "ddx-x", Title: "closed bead", Priority: 0},
		&bead.Bead{ID: "ddx-y", Title: "next bead", Priority: 1},
	)
	commitAndPushTracker(t, workDir, "chore: seed open tracker")

	secondDir := t.TempDir()
	runGitInteg(t, secondDir, "clone", originDir, ".")
	secondStore := bead.NewStore(filepath.Join(secondDir, ddxroot.DirName))
	require.NoError(t, secondStore.Close(context.Background(), "ddx-x"))
	runGitInteg(t, secondDir, "add", ".ddx/beads.jsonl")
	runGitInteg(t, secondDir, "commit", "-m", "feat: close x remotely")
	runGitInteg(t, secondDir, "push", "origin", "main")

	var syncLog bytes.Buffer
	syncTrackerBeforeClaim(context.Background(), workDir, &syncLog, nil)

	ready, err := store.ReadyExecution()
	require.NoError(t, err)
	require.NotEmpty(t, ready)
	assert.Equal(t, "ddx-y", ready[0].ID, "pre-claim sync must refresh origin/main before candidate selection")
	assert.NotContains(t, syncLog.String(), "tracker sync (pre_claim): fetch_failed")

	require.NoError(t, store.Claim(ready[0].ID, "worker"))
	assert.Equal(t, bead.StatusInProgress, readBeadStatus(t, workDir, "ddx-y"))
	assert.Equal(t, bead.StatusClosed, readBeadStatus(t, workDir, "ddx-x"))
}

func TestExecuteLoop_PostClaimPushPublishesClaim(t *testing.T) {
	workDir, originDir, store := setupTrackerSyncRepo(t)

	seedTrackerBeads(t, store, &bead.Bead{ID: "ddx-claim", Title: "claim bead", Priority: 0})
	commitAndPushTracker(t, workDir, "chore: seed claim tracker")

	require.NoError(t, store.Claim("ddx-claim", "worker"))
	var emitted []map[string]any
	emit := func(kind string, payload map[string]any) {
		entry := map[string]any{"type": kind}
		for k, v := range payload {
			entry[k] = v
		}
		emitted = append(emitted, entry)
	}
	syncTrackerAfterClaim(context.Background(), workDir, "ddx-claim", &bytes.Buffer{}, emit)

	status := cloneBeadsJSONL(t, originDir)
	assert.Contains(t, status, `"id":"ddx-claim"`)
	assert.Contains(t, status, `"status":"in_progress"`)
	assert.Empty(t, emitted, "successful post-claim push should not emit operator attention")
}

func TestExecuteLoop_PostCloseSyncPushesClose(t *testing.T) {
	workDir, originDir, store := setupTrackerSyncRepo(t)

	seedTrackerBeads(t, store, &bead.Bead{ID: "ddx-close", Title: "close bead", Priority: 0})
	commitAndPushTracker(t, workDir, "chore: seed close tracker")

	require.NoError(t, store.Claim("ddx-close", "worker"))
	require.NoError(t, store.Close(context.Background(), "ddx-close"))
	syncTrackerAfterClose(context.Background(), workDir, "ddx-close", &bytes.Buffer{}, nil)

	status := cloneBeadsJSONL(t, originDir)
	assert.Contains(t, status, `"id":"ddx-close"`)
	assert.Contains(t, status, `"status":"closed"`)
}

func TestExecuteLoop_PreClaimSyncDegradesWhenOriginUnreachable(t *testing.T) {
	workDir, _, store := setupTrackerSyncRepo(t)

	seedTrackerBeads(t, store, &bead.Bead{ID: "ddx-local", Title: "local bead", Priority: 0})
	commitAndPushTracker(t, workDir, "chore: seed local tracker")

	prevRunner := trackerSyncGitRunner
	t.Cleanup(func() { trackerSyncGitRunner = prevRunner })
	trackerSyncGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "fetch" {
			return nil, context.DeadlineExceeded
		}
		return internalgit.Command(ctx, gitDir, args...).CombinedOutput()
	}

	var log bytes.Buffer
	syncTrackerBeforeClaim(context.Background(), workDir, &log, nil)

	ready, err := store.ReadyExecution()
	require.NoError(t, err)
	require.NotEmpty(t, ready)
	assert.Equal(t, "ddx-local", ready[0].ID, "worker must continue from local state when origin is unreachable")
	assert.Contains(t, log.String(), "continuing with local state")
}

func TestExecuteLoop_TrackerSyncFailureEmitsOperatorAttention(t *testing.T) {
	workDir, _, store := setupTrackerSyncRepo(t)

	seedTrackerBeads(t, store, &bead.Bead{ID: "ddx-push", Title: "push bead", Priority: 0})
	commitAndPushTracker(t, workDir, "chore: seed push tracker")

	require.NoError(t, store.Claim("ddx-push", "worker"))

	prevRunner := trackerSyncGitRunner
	t.Cleanup(func() { trackerSyncGitRunner = prevRunner })
	trackerSyncGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "push" {
			return []byte("non-fast-forward"), assert.AnError
		}
		return internalgit.Command(ctx, gitDir, args...).CombinedOutput()
	}

	emit, logBuf, events := captureTrackerSyncEvents()
	syncTrackerAfterClaim(context.Background(), workDir, "ddx-push", logBuf, emit)

	require.NotEmpty(t, *events)
	var sawAttention bool
	for _, event := range *events {
		if event["type"] == "loop.operator_attention" && event["reason"] == "tracker_sync_push_failed" {
			sawAttention = true
			assert.Equal(t, "claim", event["stage"])
			assert.Equal(t, "ddx-push", event["bead_id"])
		}
	}
	assert.True(t, sawAttention, "persistent tracker push failure must emit loop.operator_attention")
	assert.Contains(t, logBuf.String(), "continuing with local state")
}

// TestTrackerSync_UsesRemoteHeadDefaultBranchWhenOriginMainMissing proves that
// on a master-only remote (no origin/main ref exists at all), pre-claim sync
// resolves the default branch via refs/remotes/origin/HEAD and merges
// origin/master instead of failing to reference a nonexistent origin/main.
func TestTrackerSync_UsesRemoteHeadDefaultBranchWhenOriginMainMissing(t *testing.T) {
	originDir := t.TempDir()
	runGitInteg(t, originDir, "init", "--bare", "-b", "master")

	// Seed the remote with an initial commit before workDir clones, so the
	// fresh clone below observes refs/remotes/origin/HEAD -> origin/master.
	seedDir := t.TempDir()
	runGitInteg(t, seedDir, "clone", originDir, ".")
	runGitInteg(t, seedDir, "config", "user.email", "test@ddx.test")
	runGitInteg(t, seedDir, "config", "user.name", "DDx Test")
	seedStore := bead.NewStore(filepath.Join(seedDir, ddxroot.DirName))
	require.NoError(t, seedStore.Init(context.Background()))
	seedTrackerBeads(t, seedStore,
		&bead.Bead{ID: "ddx-x", Title: "closed bead", Priority: 0},
		&bead.Bead{ID: "ddx-y", Title: "next bead", Priority: 1},
	)
	runGitInteg(t, seedDir, "add", ".")
	runGitInteg(t, seedDir, "commit", "-m", "chore: seed open tracker")
	runGitInteg(t, seedDir, "push", "-u", "origin", "master")

	workDir := t.TempDir()
	runGitInteg(t, workDir, "clone", originDir, ".")
	runGitInteg(t, workDir, "config", "user.email", "test@ddx.test")
	runGitInteg(t, workDir, "config", "user.name", "DDx Test")

	// Isolate the origin/HEAD resolution path: drop upstream tracking so
	// resolveTrackerSyncBranch cannot resolve via the current branch's @{u}.
	runGitInteg(t, workDir, "branch", "--unset-upstream")
	require.Equal(t, "refs/remotes/origin/master", strings.TrimSpace(runGitInteg(t, workDir, "symbolic-ref", "refs/remotes/origin/HEAD")))

	store := bead.NewStore(filepath.Join(workDir, ddxroot.DirName))

	// Simulate a remote-side close on master, pushed by a second clone.
	secondDir := t.TempDir()
	runGitInteg(t, secondDir, "clone", originDir, ".")
	secondStore := bead.NewStore(filepath.Join(secondDir, ddxroot.DirName))
	require.NoError(t, secondStore.Close(context.Background(), "ddx-x"))
	runGitInteg(t, secondDir, "add", ".ddx/beads.jsonl")
	runGitInteg(t, secondDir, "commit", "-m", "feat: close x remotely")
	runGitInteg(t, secondDir, "push", "origin", "master")

	var syncLog bytes.Buffer
	syncTrackerBeforeClaim(context.Background(), workDir, &syncLog, nil)

	assert.NotContains(t, syncLog.String(), "origin/main", "sync must not reference a nonexistent origin/main")

	ready, err := store.ReadyExecution()
	require.NoError(t, err)
	require.NotEmpty(t, ready)
	assert.Equal(t, "ddx-y", ready[0].ID, "pre-claim sync must merge origin/master via the origin/HEAD fallback")
}

// TestTrackerSync_PushesToResolvedDefaultBranch proves publish pushes to
// HEAD:master on a master-only remote and continues to push HEAD:main on a
// main-only remote.
func TestTrackerSync_PushesToResolvedDefaultBranch(t *testing.T) {
	assertPushRefspec := func(t *testing.T, branch, wantRefspec string) {
		t.Helper()

		originDir := t.TempDir()
		runGitInteg(t, originDir, "init", "--bare", "-b", branch)

		workDir := t.TempDir()
		runGitInteg(t, workDir, "clone", originDir, ".")
		runGitInteg(t, workDir, "config", "user.email", "test@ddx.test")
		runGitInteg(t, workDir, "config", "user.name", "DDx Test")
		seedFile := filepath.Join(workDir, "seed.txt")
		require.NoError(t, os.WriteFile(seedFile, []byte("seed\n"), 0644))
		runGitInteg(t, workDir, "add", "seed.txt")
		runGitInteg(t, workDir, "commit", "-m", "chore: initial seed")
		runGitInteg(t, workDir, "push", "-u", "origin", branch)

		store := bead.NewStore(filepath.Join(workDir, ddxroot.DirName))
		require.NoError(t, store.Init(context.Background()))
		seedTrackerBeads(t, store, &bead.Bead{ID: "ddx-push", Title: "push bead", Priority: 0})
		runGitInteg(t, workDir, "add", ".")
		runGitInteg(t, workDir, "commit", "-m", "chore: seed tracker")
		runGitInteg(t, workDir, "push", "origin", branch)

		require.NoError(t, store.Claim("ddx-push", "worker"))

		var recordedRefspec string
		prevRunner := trackerSyncGitRunner
		t.Cleanup(func() { trackerSyncGitRunner = prevRunner })
		trackerSyncGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
			if len(args) >= 3 && args[0] == "push" {
				recordedRefspec = args[2]
			}
			return internalgit.Command(ctx, gitDir, args...).CombinedOutput()
		}

		syncTrackerAfterClaim(context.Background(), workDir, "ddx-push", &bytes.Buffer{}, nil)

		assert.Equal(t, wantRefspec, recordedRefspec)
	}

	t.Run("master-only remote", func(t *testing.T) {
		assertPushRefspec(t, "master", "HEAD:master")
	})
	t.Run("main remote", func(t *testing.T) {
		assertPushRefspec(t, "main", "HEAD:main")
	})
}

// TestTrackerSync_CurrentBranchUpstreamWinsOverFallback proves an explicit
// upstream branch on the current branch is honored even when it differs from
// the remote's origin/HEAD-derived default.
func TestTrackerSync_CurrentBranchUpstreamWinsOverFallback(t *testing.T) {
	originDir := t.TempDir()
	runGitInteg(t, originDir, "init", "--bare", "-b", "master")

	seedDir := t.TempDir()
	runGitInteg(t, seedDir, "clone", originDir, ".")
	runGitInteg(t, seedDir, "config", "user.email", "test@ddx.test")
	runGitInteg(t, seedDir, "config", "user.name", "DDx Test")
	seedFile := filepath.Join(seedDir, "seed.txt")
	require.NoError(t, os.WriteFile(seedFile, []byte("seed\n"), 0644))
	runGitInteg(t, seedDir, "add", "seed.txt")
	runGitInteg(t, seedDir, "commit", "-m", "chore: initial seed")
	runGitInteg(t, seedDir, "push", "-u", "origin", "master")
	runGitInteg(t, seedDir, "checkout", "-b", "release")
	runGitInteg(t, seedDir, "push", "-u", "origin", "release")

	workDir := t.TempDir()
	runGitInteg(t, workDir, "clone", originDir, ".")
	runGitInteg(t, workDir, "config", "user.email", "test@ddx.test")
	runGitInteg(t, workDir, "config", "user.name", "DDx Test")
	require.Equal(t, "refs/remotes/origin/master", strings.TrimSpace(runGitInteg(t, workDir, "symbolic-ref", "refs/remotes/origin/HEAD")),
		"remote default branch must resolve to master")

	runGitInteg(t, workDir, "checkout", "release")
	require.Equal(t, "origin/release", strings.TrimSpace(runGitInteg(t, workDir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")),
		"current branch must track origin/release")

	branch := resolveTrackerSyncBranch(context.Background(), workDir)
	assert.Equal(t, "release", branch, "current branch upstream must win over the origin/HEAD-derived default")
}
