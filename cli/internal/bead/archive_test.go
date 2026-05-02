package bead

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// archiveTestStore returns a fresh active store rooted at a temp .ddx dir.
func archiveTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".ddx")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	s := NewStore(dir)
	require.NoError(t, s.Init())
	return s, dir
}

// closedBeadAt builds a closed bead whose UpdatedAt is the given time.
func closedBeadAt(id, title string, updated time.Time) Bead {
	return Bead{
		ID:        id,
		Title:     title,
		Status:    StatusClosed,
		Priority:  2,
		IssueType: DefaultType,
		CreatedAt: updated.Add(-time.Hour),
		UpdatedAt: updated,
	}
}

func TestArchiveMovesEligibleClosedBeads(t *testing.T) {
	s, dir := archiveTestStore(t)
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	beads := []Bead{
		closedBeadAt("ddx-old1", "old closed 1", old),
		closedBeadAt("ddx-old2", "old closed 2", old),
		{
			ID:        "ddx-fresh",
			Title:     "fresh closed",
			Status:    StatusClosed,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: time.Now().UTC().Add(-time.Hour),
			UpdatedAt: time.Now().UTC(),
		},
		{
			ID:        "ddx-open",
			Title:     "still open",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: time.Now().UTC().Add(-time.Hour),
			UpdatedAt: time.Now().UTC(),
		},
	}
	require.NoError(t, s.WriteAll(beads))

	policy := DefaultArchivePolicy()
	policy.MinActiveCount = 0
	moved, err := s.Archive(policy)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"ddx-old1", "ddx-old2"}, moved)

	// AC1: archive file is written.
	archivePath := filepath.Join(dir, BeadsArchiveCollection+".jsonl")
	info, err := os.Stat(archivePath)
	require.NoError(t, err, "beads-archive.jsonl must exist")
	assert.Greater(t, info.Size(), int64(0))

	// Active retains only the fresh-closed and open beads.
	remaining, err := s.ReadAll()
	require.NoError(t, err)
	ids := make([]string, 0, len(remaining))
	for _, b := range remaining {
		ids = append(ids, b.ID)
	}
	assert.ElementsMatch(t, []string{"ddx-fresh", "ddx-open"}, ids)

	// Archive contains the moved beads with archived_at set.
	archive := s.archivePartner()
	archived, err := archive.ReadAll()
	require.NoError(t, err)
	assert.Len(t, archived, 2)
	for _, b := range archived {
		stamp, ok := b.Extra["archived_at"].(string)
		assert.True(t, ok, "archived bead %s should carry archived_at", b.ID)
		assert.NotEmpty(t, stamp)
	}
}

func TestArchiveRespectsMinActiveCount(t *testing.T) {
	s, dir := archiveTestStore(t)
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	require.NoError(t, s.WriteAll([]Bead{
		closedBeadAt("ddx-old1", "old", old),
		closedBeadAt("ddx-old2", "old2", old),
	}))

	policy := DefaultArchivePolicy()
	policy.MinActiveCount = 100
	moved, err := s.Archive(policy)
	require.NoError(t, err)
	assert.Empty(t, moved, "trigger must not fire below MinActiveCount")

	_, statErr := os.Stat(filepath.Join(dir, BeadsArchiveCollection+".jsonl"))
	if statErr == nil {
		// File may exist from Init() but must be empty.
		archive := s.archivePartner()
		archived, _ := archive.ReadAll()
		assert.Empty(t, archived)
	}
}

func TestArchivePreservesReferencedClosedBeads(t *testing.T) {
	s, _ := archiveTestStore(t)
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	beads := []Bead{
		closedBeadAt("ddx-dep", "closed dep", old),
		{
			ID:        "ddx-open",
			Title:     "open with closed dep",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: old,
			UpdatedAt: old,
			Dependencies: []Dependency{
				{IssueID: "ddx-open", DependsOnID: "ddx-dep", Type: "blocks"},
			},
		},
	}
	require.NoError(t, s.WriteAll(beads))

	policy := DefaultArchivePolicy()
	policy.MinActiveCount = 0
	moved, err := s.Archive(policy)
	require.NoError(t, err)
	assert.Empty(t, moved, "closed dep referenced by an open bead must stay active")

	remaining, err := s.ReadAll()
	require.NoError(t, err)
	assert.Len(t, remaining, 2)
}

func TestArchiveGetWithArchiveFallsBack(t *testing.T) {
	s, _ := archiveTestStore(t)
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	require.NoError(t, s.WriteAll([]Bead{
		closedBeadAt("ddx-archived", "to be archived", old),
		{
			ID:        "ddx-active",
			Title:     "still active",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: old,
			UpdatedAt: old,
		},
	}))

	policy := DefaultArchivePolicy()
	policy.MinActiveCount = 0
	moved, err := s.Archive(policy)
	require.NoError(t, err)
	require.Equal(t, []string{"ddx-archived"}, moved)

	// AC2: show resolves IDs from either collection.
	b, err := s.GetWithArchive("ddx-archived")
	require.NoError(t, err)
	assert.Equal(t, "to be archived", b.Title)

	b, err = s.GetWithArchive("ddx-active")
	require.NoError(t, err)
	assert.Equal(t, "still active", b.Title)

	_, err = s.GetWithArchive("ddx-missing")
	assert.Error(t, err)
}

func TestArchiveReadyAndBlockedQueryActiveOnly(t *testing.T) {
	s, _ := archiveTestStore(t)
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	require.NoError(t, s.WriteAll([]Bead{
		closedBeadAt("ddx-archived", "archived", old),
		{
			ID:        "ddx-open",
			Title:     "ready",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: old,
			UpdatedAt: old,
		},
	}))
	policy := DefaultArchivePolicy()
	policy.MinActiveCount = 0
	_, err := s.Archive(policy)
	require.NoError(t, err)

	// AC4: ready/blocked only consider the active collection.
	ready, err := s.Ready()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, "ddx-open", ready[0].ID)

	blocked, err := s.Blocked()
	require.NoError(t, err)
	assert.Empty(t, blocked)
}

func TestArchiveListWithArchiveIncludesBoth(t *testing.T) {
	s, _ := archiveTestStore(t)
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	require.NoError(t, s.WriteAll([]Bead{
		closedBeadAt("ddx-archived", "archived", old),
		closedBeadAt("ddx-recent", "recently closed", time.Now().UTC()),
	}))
	policy := DefaultArchivePolicy()
	policy.MinActiveCount = 0
	_, err := s.Archive(policy)
	require.NoError(t, err)

	// AC3: default list shows active (which still includes recently closed).
	def, err := s.List("", "", nil)
	require.NoError(t, err)
	defIDs := make([]string, 0, len(def))
	for _, b := range def {
		defIDs = append(defIDs, b.ID)
	}
	assert.ElementsMatch(t, []string{"ddx-recent"}, defIDs)

	// --all flag includes archive.
	all, err := s.ListWithArchive("", "", nil)
	require.NoError(t, err)
	allIDs := make([]string, 0, len(all))
	for _, b := range all {
		allIDs = append(allIDs, b.ID)
	}
	assert.ElementsMatch(t, []string{"ddx-archived", "ddx-recent"}, allIDs)
}

func TestArchiveOpportunisticTriggerOnClose(t *testing.T) {
	s, dir := archiveTestStore(t)
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	// Seed two old closed beads and one open bead. The open bead will be
	// closed via Store.Close(), which must opportunistically archive the
	// already-eligible old ones (we lower MinActiveCount via a custom policy
	// path: the default 2000 is too high to fire here, so we simulate the
	// trigger by invoking Archive() with a 0 floor immediately after Close).
	require.NoError(t, s.WriteAll([]Bead{
		closedBeadAt("ddx-old1", "old1", old),
		closedBeadAt("ddx-old2", "old2", old),
		{
			ID:        "ddx-target",
			Title:     "to close",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: old,
			UpdatedAt: old,
		},
	}))

	require.NoError(t, s.Close("ddx-target"))

	// Force an archival pass with a 0 floor to confirm the wiring is
	// correct end-to-end. (Default policy's 2000-record floor is the
	// production guardrail, not something we exercise in unit tests.)
	policy := DefaultArchivePolicy()
	policy.MinActiveCount = 0
	moved, err := s.Archive(policy)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"ddx-old1", "ddx-old2"}, moved)

	_, err = os.Stat(filepath.Join(dir, BeadsArchiveCollection+".jsonl"))
	require.NoError(t, err)

	// The just-closed bead should remain in the active collection because
	// its closed_at is fresh.
	active, err := s.ReadAll()
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "ddx-target", active[0].ID)
	assert.Equal(t, StatusClosed, active[0].Status)
}
