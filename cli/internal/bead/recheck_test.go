package bead

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRecheckRepoStore(t *testing.T, root string) *Store {
	t.Helper()
	store := NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	return store
}

func seedClosedBeadInRepo(t *testing.T, store *Store, id string) {
	t.Helper()
	require.NoError(t, store.Create(testCtx(), &Bead{
		ID:    id,
		Title: fmt.Sprintf("target %s", id),
	}))
	require.NoError(t, store.Close(testCtx(), id))
}

func seedBlockedBeadWithRef(t *testing.T, store *Store, id, repoAlias, targetID, reason string) {
	t.Helper()
	ref, err := NewCrossRepoBlockerRef(repoAlias, targetID)
	require.NoError(t, err)
	require.NoError(t, store.Create(testCtx(), &Bead{
		ID:     id,
		Title:  fmt.Sprintf("blocked %s", id),
		Status: StatusBlocked,
		Extra: map[string]any{
			ExtraLifecycleExternalBlockerReason: reason,
			ExtraLifecycleCrossRepoBlockerRef:   ref,
		},
	}))
}

func recheckKnownRepos(path string) map[string]config.KnownRepoConfig {
	return map[string]config.KnownRepoConfig{
		"upstream": {Path: path},
	}
}

func TestRecheckBlockers_LocalRepo_ClosesReopens(t *testing.T) {
	root := t.TempDir()
	blockedRoot := filepath.Join(root, "blocked")
	upstreamRoot := filepath.Join(root, "upstream")
	require.NoError(t, os.MkdirAll(blockedRoot, 0o755))
	require.NoError(t, os.MkdirAll(upstreamRoot, 0o755))

	upstreamStore := newRecheckRepoStore(t, upstreamRoot)
	closedID := "upstream-closed"
	seedClosedBeadInRepo(t, upstreamStore, closedID)

	blockedStore := newRecheckRepoStore(t, blockedRoot)
	blockedID := "blocked-cross-repo"
	seedBlockedBeadWithRef(t, blockedStore, blockedID, "upstream", closedID, "waiting on upstream")

	results, err := RecheckBlockers(testCtx(), blockedStore, recheckKnownRepos("../upstream"), "")
	require.NoError(t, err)
	require.Len(t, results, 1)

	row := results[0]
	assert.Equal(t, blockedID, row.BeadID)
	assert.Equal(t, RecheckBlockerOutcomeReopened, row.Outcome)
	assert.Equal(t, StatusOpen, row.Status)
	assert.Equal(t, "upstream", row.Repo)
	assert.Equal(t, closedID, row.TargetBead)
	assert.Equal(t, StatusClosed, row.ObservedStatus)

	got, err := blockedStore.Get(testCtx(), blockedID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
	assert.NotContains(t, got.Extra, ExtraLifecycleExternalBlockerReason)
	assert.NotContains(t, got.Extra, ExtraLifecycleCrossRepoBlockerRef)

	events, err := blockedStore.Events(blockedID)
	require.NoError(t, err)
	found := false
	for _, ev := range events {
		if ev.Kind != "cross_repo_blocker_recheck" {
			continue
		}
		found = true
		assert.Equal(t, "system:cross-repo-recheck", ev.Actor)
		assert.Equal(t, "upstream", ev.Source)
		assert.Contains(t, ev.Body, closedID)
		break
	}
	assert.True(t, found, "cross-repo reopen must append a recheck event")
}

func TestRecheckBlockers_LocalRepo_TargetStillOpen(t *testing.T) {
	root := t.TempDir()
	blockedRoot := filepath.Join(root, "blocked")
	upstreamRoot := filepath.Join(root, "upstream")
	require.NoError(t, os.MkdirAll(blockedRoot, 0o755))
	require.NoError(t, os.MkdirAll(upstreamRoot, 0o755))

	upstreamStore := newRecheckRepoStore(t, upstreamRoot)
	openID := "upstream-open"
	require.NoError(t, upstreamStore.Create(testCtx(), &Bead{
		ID:    openID,
		Title: "still open",
	}))

	blockedStore := newRecheckRepoStore(t, blockedRoot)
	blockedID := "blocked-still-waiting"
	seedBlockedBeadWithRef(t, blockedStore, blockedID, "upstream", openID, "waiting on upstream")

	results, err := RecheckBlockers(testCtx(), blockedStore, recheckKnownRepos("../upstream"), "")
	require.NoError(t, err)
	require.Len(t, results, 1)

	row := results[0]
	assert.Equal(t, RecheckBlockerOutcomeBlocked, row.Outcome)
	assert.Equal(t, StatusBlocked, row.Status)
	assert.Equal(t, openID, row.TargetBead)
	assert.Equal(t, StatusOpen, row.ObservedStatus)
	assert.Contains(t, row.Reason, "status=open")

	got, err := blockedStore.Get(testCtx(), blockedID)
	require.NoError(t, err)
	assert.Equal(t, StatusBlocked, got.Status)
	assert.Contains(t, got.Extra, ExtraLifecycleExternalBlockerReason)
	assert.Contains(t, got.Extra, ExtraLifecycleCrossRepoBlockerRef)
}

func TestRecheckBlockers_LocalRepo_UnresolvableRepo(t *testing.T) {
	root := t.TempDir()
	blockedRoot := filepath.Join(root, "blocked")
	upstreamRoot := filepath.Join(root, "upstream")
	require.NoError(t, os.MkdirAll(blockedRoot, 0o755))
	require.NoError(t, os.MkdirAll(upstreamRoot, 0o755))

	upstreamStore := newRecheckRepoStore(t, upstreamRoot)
	seedClosedBeadInRepo(t, upstreamStore, "upstream-closed")

	blockedStore := newRecheckRepoStore(t, blockedRoot)
	blockedID := "blocked-unknown-repo"
	seedBlockedBeadWithRef(t, blockedStore, blockedID, "missing", "upstream-closed", "waiting on upstream")

	results, err := RecheckBlockers(testCtx(), blockedStore, recheckKnownRepos("../upstream"), "")
	require.NoError(t, err)
	require.Len(t, results, 1)

	row := results[0]
	assert.Equal(t, RecheckBlockerOutcomeUnresolvable, row.Outcome)
	assert.Equal(t, StatusBlocked, row.Status)
	assert.Contains(t, row.Reason, `unknown known-repo "missing"`)

	got, err := blockedStore.Get(testCtx(), blockedID)
	require.NoError(t, err)
	assert.Equal(t, StatusBlocked, got.Status)
	assert.Contains(t, got.Extra, ExtraLifecycleExternalBlockerReason)
	assert.Contains(t, got.Extra, ExtraLifecycleCrossRepoBlockerRef)
}

func TestRecheckBlockers_LocalRepo_UnresolvableBeadID(t *testing.T) {
	root := t.TempDir()
	blockedRoot := filepath.Join(root, "blocked")
	upstreamRoot := filepath.Join(root, "upstream")
	require.NoError(t, os.MkdirAll(blockedRoot, 0o755))
	require.NoError(t, os.MkdirAll(upstreamRoot, 0o755))

	newRecheckRepoStore(t, upstreamRoot)

	blockedStore := newRecheckRepoStore(t, blockedRoot)
	blockedID := "blocked-missing-target"
	seedBlockedBeadWithRef(t, blockedStore, blockedID, "upstream", "missing-target", "waiting on upstream")

	results, err := RecheckBlockers(testCtx(), blockedStore, recheckKnownRepos("../upstream"), "")
	require.NoError(t, err)
	require.Len(t, results, 1)

	row := results[0]
	assert.Equal(t, RecheckBlockerOutcomeUnresolvable, row.Outcome)
	assert.Equal(t, StatusBlocked, row.Status)
	assert.Contains(t, row.Reason, `target bead "missing-target" not found in repo "upstream"`)

	got, err := blockedStore.Get(testCtx(), blockedID)
	require.NoError(t, err)
	assert.Equal(t, StatusBlocked, got.Status)
	assert.Contains(t, got.Extra, ExtraLifecycleExternalBlockerReason)
	assert.Contains(t, got.Extra, ExtraLifecycleCrossRepoBlockerRef)
}
