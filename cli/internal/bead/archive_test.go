package bead

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultArchivePolicy mirrors the TD-027 §(b) shipping defaults that the
// archive tests want to exercise. Kept as a test-local helper so the
// production package does not carry a function with no live caller.
func defaultArchivePolicy() ArchivePolicy {
	return ArchivePolicy{
		Statuses:       []string{StatusClosed},
		MinAge:         30 * 24 * time.Hour,
		MinActiveCount: 2000,
		BatchSize:      500,
	}
}

// archiveTestStore returns a fresh active store rooted at a temp .ddx dir.
func archiveTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ddxroot.DirName)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	s := NewStore(dir)
	require.NoError(t, s.Init(testCtx()))
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

	policy := defaultArchivePolicy()
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
	remaining, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	ids := make([]string, 0, len(remaining))
	for _, b := range remaining {
		ids = append(ids, b.ID)
	}
	assert.ElementsMatch(t, []string{"ddx-fresh", "ddx-open"}, ids)

	// Archive contains the moved beads with archived_at set.
	archive := s.archivePartner()
	archived, err := archive.ReadAll(testCtx())
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

	policy := defaultArchivePolicy()
	policy.MinActiveCount = 100
	moved, err := s.Archive(policy)
	require.NoError(t, err)
	assert.Empty(t, moved, "trigger must not fire below MinActiveCount")

	_, statErr := os.Stat(filepath.Join(dir, BeadsArchiveCollection+".jsonl"))
	if statErr == nil {
		// File may exist from Init() but must be empty.
		archive := s.archivePartner()
		archived, _ := archive.ReadAll(testCtx())
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

	policy := defaultArchivePolicy()
	policy.MinActiveCount = 0
	moved, err := s.Archive(policy)
	require.NoError(t, err)
	assert.Empty(t, moved, "closed dep referenced by an open bead must stay active")

	remaining, err := s.ReadAll(testCtx())
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

	policy := defaultArchivePolicy()
	policy.MinActiveCount = 0
	moved, err := s.Archive(policy)
	require.NoError(t, err)
	require.Equal(t, []string{"ddx-archived"}, moved)

	// AC2: show resolves IDs from either collection.
	b, err := s.GetWithArchive(testCtx(), "ddx-archived")
	require.NoError(t, err)
	assert.Equal(t, "to be archived", b.Title)

	b, err = s.GetWithArchive(testCtx(), "ddx-active")
	require.NoError(t, err)
	assert.Equal(t, "still active", b.Title)

	_, err = s.GetWithArchive(testCtx(), "ddx-missing")
	assert.Error(t, err)
}

func testStoreGetWithArchiveHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	s, _ := archiveTestStore(t)
	require.NoError(t, s.WriteAll([]Bead{
		{
			ID:        "ddx-active",
			Title:     "active",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: time.Now().UTC().Add(-time.Hour),
			UpdatedAt: time.Now().UTC(),
		},
		closedBeadAt("ddx-archived", "archived", time.Now().UTC().Add(-60*24*time.Hour)),
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.GetWithArchive(ctx, "ddx-active")
	require.Error(t, err)
}

func testStoreGetWithArchiveForwardsCallerContext(t *testing.T) {
	s, _ := archiveTestStore(t)
	require.NoError(t, s.WriteAll([]Bead{
		closedBeadAt("ddx-archived", "archived", time.Now().UTC().Add(-60*24*time.Hour)),
	}))

	policy := defaultArchivePolicy()
	policy.MinActiveCount = 0
	_, err := s.Archive(policy)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = s.GetWithArchive(ctx, "ddx-archived")
	require.Error(t, err)
}

func TestStoreGetWithArchiveHonorsCanceledContext(t *testing.T) {
	testStoreGetWithArchiveHonorsCanceledContext(t)
}

func TestStoreGetWithArchiveForwardsCallerContext(t *testing.T) {
	testStoreGetWithArchiveForwardsCallerContext(t)
}

func TestStoreGetWithArchive_(t *testing.T) {
	t.Run("HonorsCanceledContext", testStoreGetWithArchiveHonorsCanceledContext)
	t.Run("ForwardsCallerContext", testStoreGetWithArchiveForwardsCallerContext)
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
	policy := defaultArchivePolicy()
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
	policy := defaultArchivePolicy()
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
	padding := strings.Repeat("x", 4096)
	beads := make([]Bead, 0, 1101)
	for i := 0; i < 1100; i++ {
		b := closedBeadAt("ddx-old-"+strconv.Itoa(i), "old", old)
		b.Description = padding
		beads = append(beads, b)
	}
	beads = append(beads, Bead{
		ID:          "ddx-target",
		Title:       "to close",
		Status:      StatusOpen,
		Priority:    2,
		IssueType:   DefaultType,
		CreatedAt:   old,
		UpdatedAt:   old,
		Description: padding,
	})
	require.NoError(t, s.WriteAll(beads))
	info, err := os.Stat(filepath.Join(dir, "beads.jsonl"))
	require.NoError(t, err)
	require.Greater(t, info.Size(), DefaultArchiveSizeThreshold)

	require.NoError(t, s.Close(testCtx(), "ddx-target"))

	_, err = os.Stat(filepath.Join(dir, BeadsArchiveCollection+".jsonl"))
	require.NoError(t, err)

	active, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	assert.Empty(t, active, "close-time maintenance should drain eligible closed rows once the active file crosses the size threshold")

	target, err := s.GetWithArchive(testCtx(), "ddx-target")
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, target.Status)
}
