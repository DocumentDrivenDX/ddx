package agent

import (
	"fmt"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// previewQueueTestStore wraps a *bead.Store to satisfy PreviewQueueStore, but
// allows tests to also call store.Create and store.Init directly.
type previewStoreAdapter struct {
	s *bead.Store
}

func (a *previewStoreAdapter) ReadyExecution() ([]bead.Bead, error) {
	return a.s.ReadyExecution()
}

func newPreviewTestStore(t *testing.T) (*bead.Store, PreviewQueueStore) {
	t.Helper()
	s := bead.NewStore(t.TempDir())
	require.NoError(t, s.Init())
	return s, &previewStoreAdapter{s: s}
}

// TestPreviewQueue_PrioritySort verifies that beads are returned in priority
// order (0 = highest priority first), with FIFO (created_at asc) within the
// same priority.
func TestPreviewQueue_PrioritySort(t *testing.T) {
	store, qs := newPreviewTestStore(t)

	now := time.Now().UTC()

	b2 := &bead.Bead{ID: "ddx-p2", Title: "P2 bead", Priority: 2, CreatedAt: now}
	b0 := &bead.Bead{ID: "ddx-p0", Title: "P0 bead", Priority: 0, CreatedAt: now.Add(time.Second)}
	b1 := &bead.Bead{ID: "ddx-p1", Title: "P1 bead", Priority: 1, CreatedAt: now.Add(2 * time.Second)}
	for _, b := range []*bead.Bead{b2, b0, b1} {
		require.NoError(t, store.Create(b))
	}

	entries, err := PreviewQueue(qs, PickerFilters{}, 0)
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, "ddx-p0", entries[0].BeadID, "P0 must be first")
	assert.Equal(t, FilterDecisionNext, entries[0].FilterDecision)
	assert.Equal(t, "ddx-p1", entries[1].BeadID, "P1 must be second")
	assert.Equal(t, FilterDecisionEligible, entries[1].FilterDecision)
	assert.Equal(t, "ddx-p2", entries[2].BeadID, "P2 must be third")
	assert.Equal(t, FilterDecisionEligible, entries[2].FilterDecision)
}

// TestPreviewQueue_LabelFilter verifies that label filtering narrows the queue
// identically to the worker's label intersection logic.
func TestPreviewQueue_LabelFilter(t *testing.T) {
	store, qs := newPreviewTestStore(t)

	bMatch := &bead.Bead{ID: "ddx-match", Title: "Matching bead", Priority: 0, Labels: []string{"area:agent", "phase:2"}}
	bNoMatch := &bead.Bead{ID: "ddx-nomatch", Title: "Non-matching bead", Priority: 0, Labels: []string{"area:cli"}}
	require.NoError(t, store.Create(bMatch))
	require.NoError(t, store.Create(bNoMatch))

	entries, err := PreviewQueue(qs, PickerFilters{LabelFilter: "area:agent"}, 0)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	// The matching bead must come first (same priority; ID sort picks ddx-match < ddx-nomatch).
	// However the non-matching bead should be skipped.
	var matchEntry, noMatchEntry *QueueEntry
	for i := range entries {
		if entries[i].BeadID == "ddx-match" {
			matchEntry = &entries[i]
		}
		if entries[i].BeadID == "ddx-nomatch" {
			noMatchEntry = &entries[i]
		}
	}
	require.NotNil(t, matchEntry)
	require.NotNil(t, noMatchEntry)
	assert.Equal(t, FilterDecisionNext, matchEntry.FilterDecision)
	assert.Equal(t, FilterDecisionSkipped, noMatchEntry.FilterDecision)
	assert.Contains(t, noMatchEntry.Why, "label_filter mismatch")
}

// TestPreviewQueue_CooldownSkip verifies that a bead with an active
// execute-loop-retry-after is excluded by ReadyExecution (and therefore absent
// from the PreviewQueue result rather than listed as skipped). ReadyExecution
// itself filters cooldown beads before they reach PreviewQueue.
func TestPreviewQueue_CooldownSkip(t *testing.T) {
	store, qs := newPreviewTestStore(t)

	// A bead on cooldown should be invisible to ReadyExecution.
	future := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	onCooldown := &bead.Bead{
		ID:    "ddx-cooldown",
		Title: "On cooldown",
		Extra: map[string]any{"execute-loop-retry-after": future},
	}
	eligible := &bead.Bead{ID: "ddx-eligible", Title: "Eligible bead"}
	require.NoError(t, store.Create(onCooldown))
	require.NoError(t, store.Create(eligible))

	entries, err := PreviewQueue(qs, PickerFilters{}, 0)
	require.NoError(t, err)

	// Only the eligible bead should appear; the cooldown bead is filtered by
	// ReadyExecution before PreviewQueue sees it.
	require.Len(t, entries, 1)
	assert.Equal(t, "ddx-eligible", entries[0].BeadID)
	assert.Equal(t, FilterDecisionNext, entries[0].FilterDecision)
}

// TestPreviewQueue_DepsBlocked verifies that a bead with an unclosed
// dependency does not appear in the PreviewQueue result (ReadyExecution
// excludes it before PreviewQueue is called).
func TestPreviewQueue_DepsBlocked(t *testing.T) {
	store, qs := newPreviewTestStore(t)

	blocker := &bead.Bead{ID: "ddx-blocker", Title: "Blocker bead (open)"}
	require.NoError(t, store.Create(blocker))

	dependent := &bead.Bead{ID: "ddx-dependent", Title: "Depends on blocker"}
	dependent.AddDep(blocker.ID, "blocks")
	require.NoError(t, store.Create(dependent))

	independent := &bead.Bead{ID: "ddx-independent", Title: "No deps"}
	require.NoError(t, store.Create(independent))

	entries, err := PreviewQueue(qs, PickerFilters{}, 0)
	require.NoError(t, err)

	// Only beads whose deps are all closed are execution-eligible. blocker
	// and independent are open with no deps; dependent is blocked.
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.BeadID
	}
	assert.NotContains(t, ids, "ddx-dependent", "blocked bead must not appear in preview queue")
	assert.Contains(t, ids, "ddx-independent")
}

// TestPreviewQueue_DeterministicAcrossRuns verifies that calling PreviewQueue
// twice with the same store and filters returns byte-identical results — no
// per-Run state leaks in.
func TestPreviewQueue_DeterministicAcrossRuns(t *testing.T) {
	store, qs := newPreviewTestStore(t)

	for i := 0; i < 5; i++ {
		b := &bead.Bead{
			ID:    fmt.Sprintf("ddx-%04d", i),
			Title: fmt.Sprintf("Bead %d", i),
			// Alternate priorities to exercise sort determinism.
			Priority: i % 3,
		}
		require.NoError(t, store.Create(b))
	}

	first, err := PreviewQueue(qs, PickerFilters{}, 0)
	require.NoError(t, err)
	second, err := PreviewQueue(qs, PickerFilters{}, 0)
	require.NoError(t, err)

	require.Equal(t, len(first), len(second), "result length must be identical across calls")
	for i := range first {
		assert.Equal(t, first[i].BeadID, second[i].BeadID, "position %d bead ID must match", i+1)
		assert.Equal(t, first[i].FilterDecision, second[i].FilterDecision, "position %d decision must match", i+1)
	}
}

// TestPicker_StillWorks_AfterRefactor verifies that ExecuteBeadWorker.nextCandidate
// still returns the first eligible candidate correctly after the PreviewQueue
// refactor — behavior must remain unchanged.
func TestPicker_StillWorks_AfterRefactor(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	// Create two beads; first by priority should win.
	high := &bead.Bead{ID: "ddx-high", Title: "High priority", Priority: 0}
	low := &bead.Bead{ID: "ddx-low", Title: "Low priority", Priority: 2}
	require.NoError(t, store.Create(high))
	require.NoError(t, store.Create(low))

	worker := &ExecuteBeadWorker{Store: store}

	// Empty attempted map — both are eligible; high priority wins.
	got, skips, ok, err := worker.nextCandidate(map[string]struct{}{}, "", "")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "ddx-high", got.ID)
	assert.Empty(t, skips)

	// With high priority in attempted, low priority should be returned.
	attempted := map[string]struct{}{"ddx-high": {}}
	got2, skips2, ok2, err := worker.nextCandidate(attempted, "", "")
	require.NoError(t, err)
	require.True(t, ok2)
	assert.Equal(t, "ddx-low", got2.ID)
	assert.Len(t, skips2, 1)
	assert.Equal(t, "in_attempted", skips2[0].Reason)
}
