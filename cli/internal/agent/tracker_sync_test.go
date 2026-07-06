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
