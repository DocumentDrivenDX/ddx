package bead

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeedsHumanMetadataRoundTrip(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "needs human attention"}
	require.NoError(t, s.Create(b))

	meta := NeedsHumanMeta{
		Reason:          "agent loop exhausted retries",
		Since:           time.Now().UTC().Format(time.RFC3339),
		Source:          "execute-loop",
		SuggestedAction: "review no_changes rationale",
		Summary:         "bead returned no_changes 3 times",
	}
	// Add label and metadata via the mutate function.
	require.NoError(t, s.Update(b.ID, func(bead *Bead) {
		bead.Labels = append(bead.Labels, LabelNeedsHuman)
		SetNeedsHumanMeta(bead, meta)
	}))

	got, err := s.Get(b.ID)
	require.NoError(t, err)

	gotMeta := GetNeedsHumanMeta(*got)
	assert.Equal(t, meta.Reason, gotMeta.Reason)
	assert.Equal(t, meta.Since, gotMeta.Since)
	assert.Equal(t, meta.Source, gotMeta.Source)
	assert.Equal(t, meta.SuggestedAction, gotMeta.SuggestedAction)
	assert.Equal(t, meta.Summary, gotMeta.Summary)
}

func TestStoreNeedsHumanListsOpenNeedsHumanSorted(t *testing.T) {
	s := newTestStore(t)

	// Create a blocker so we can verify dep-blocked needs_human beads are included.
	blocker := &Bead{Title: "blocker", Priority: 0}
	require.NoError(t, s.Create(blocker))

	// needs_human beads at different priorities.
	nh1 := &Bead{Title: "needs-human P1", Priority: 1, Labels: []string{LabelNeedsHuman}}
	nh2 := &Bead{Title: "needs-human P0", Priority: 0, Labels: []string{LabelNeedsHuman}}
	// Dep-blocked needs_human bead — should still appear.
	nhDep := &Bead{Title: "needs-human dep-blocked P0", Priority: 0, Labels: []string{LabelNeedsHuman}}
	// Open bead without needs_human — must be excluded.
	plain := &Bead{Title: "plain open"}
	// Closed needs_human bead — must be excluded.
	closed := &Bead{Title: "closed needs-human", Labels: []string{LabelNeedsHuman}}

	require.NoError(t, s.Create(nh1))
	require.NoError(t, s.Create(nh2))
	require.NoError(t, s.Create(nhDep))
	require.NoError(t, s.Create(plain))
	require.NoError(t, s.Create(closed))
	require.NoError(t, s.DepAdd(nhDep.ID, blocker.ID))
	require.NoError(t, s.Close(closed.ID))

	result, err := s.NeedsHuman()
	require.NoError(t, err)

	ids := make([]string, len(result))
	for i, b := range result {
		ids[i] = b.ID
	}

	// Must include all open needs_human beads regardless of dep status.
	assert.Contains(t, ids, nh1.ID)
	assert.Contains(t, ids, nh2.ID)
	assert.Contains(t, ids, nhDep.ID)

	// Must exclude non-needs_human and closed beads.
	assert.NotContains(t, ids, plain.ID)
	assert.NotContains(t, ids, closed.ID)

	// Must be sorted by queue order: P0 before P1.
	p0idx, p1idx := -1, -1
	for i, b := range result {
		if b.ID == nh2.ID {
			p0idx = i
		}
		if b.ID == nh1.ID {
			p1idx = i
		}
	}
	assert.Less(t, p0idx, p1idx, "P0 needs_human bead should sort before P1")
}

func TestReadyExecutionSkipsNeedsHumanButReadyCanIncludeWhenRequested(t *testing.T) {
	s := newTestStore(t)

	// Open bead with needs_human label and no unsatisfied deps.
	nh := &Bead{Title: "needs-human open", Labels: []string{LabelNeedsHuman}}
	require.NoError(t, s.Create(nh))

	// Worker drain must skip it.
	executionReady, err := s.ReadyExecution()
	require.NoError(t, err)
	for _, b := range executionReady {
		assert.NotEqual(t, nh.ID, b.ID, "ReadyExecution must not include needs_human beads")
	}

	// Dep-ready query must include it.
	depReady, err := s.Ready()
	require.NoError(t, err)
	found := false
	for _, b := range depReady {
		if b.ID == nh.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Ready must include dep-satisfied needs_human beads when not filtering for execution")
}

func TestStatusCountsIncludeNeedsHumanAndWorkerReady(t *testing.T) {
	s := newTestStore(t)

	// Two plain open beads with no deps → both dep-ready and worker-ready.
	a := &Bead{Title: "worker-ready A"}
	b := &Bead{Title: "worker-ready B"}
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Create(b))

	// One open needs_human bead with satisfied deps → dep-ready but NOT worker-ready.
	nh := &Bead{Title: "needs-human", Labels: []string{LabelNeedsHuman}}
	require.NoError(t, s.Create(nh))

	// One closed bead.
	c := &Bead{Title: "closed"}
	require.NoError(t, s.Create(c))
	require.NoError(t, s.Close(c.ID))

	counts, err := s.Status()
	require.NoError(t, err)

	assert.Equal(t, 4, counts.Total)
	assert.Equal(t, 3, counts.Open)        // a, b, nh
	assert.Equal(t, 1, counts.Closed)      // c
	assert.Equal(t, 3, counts.Ready)       // a, b, nh (all dep-satisfied open beads)
	assert.Equal(t, 2, counts.WorkerReady) // a, b (needs_human excluded)
	assert.Equal(t, 1, counts.NeedsHuman)  // nh
}
