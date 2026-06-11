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
	require.NoError(t, s.Create(testCtx(), b))

	meta := NeedsHumanMeta{
		Reason:          "agent loop exhausted retries",
		Since:           time.Now().UTC().Format(time.RFC3339),
		Source:          "work",
		SuggestedAction: "review no_changes rationale",
		Summary:         "bead returned no_changes 3 times",
	}
	// Add label and metadata via the mutate function.
	require.NoError(t, s.Update(testCtx(), b.ID, func(bead *Bead) {
		bead.Labels = append(bead.Labels, LabelNeedsHuman)
		SetNeedsHumanMeta(bead, meta)
	}))

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)

	gotMeta := GetNeedsHumanMeta(*got)
	assert.Equal(t, meta.Reason, gotMeta.Reason)
	assert.Equal(t, meta.Since, gotMeta.Since)
	assert.Equal(t, meta.Source, gotMeta.Source)
	assert.Equal(t, meta.SuggestedAction, gotMeta.SuggestedAction)
	assert.Equal(t, meta.Summary, gotMeta.Summary)
}

func TestStoreNeedsHumanListsProposedOperatorAttentionSorted(t *testing.T) {
	s := newTestStore(t)

	// Create a blocker so we can verify dep-waiting proposed beads are included.
	blocker := &Bead{Title: "blocker", Priority: 0}
	require.NoError(t, s.Create(testCtx(), blocker))

	// Proposed beads at different priorities.
	nh1 := &Bead{Title: "operator attention P1", Priority: 1, Status: StatusProposed}
	nh2 := &Bead{Title: "operator attention P0", Priority: 0, Status: StatusProposed}
	// Dep-waiting proposed bead should still appear in the operator lane.
	nhDep := &Bead{Title: "operator attention dep-waiting P0", Priority: 0, Status: StatusProposed}
	// Open bead with legacy needs_human metadata must be excluded.
	plain := &Bead{Title: "plain open"}
	legacy := &Bead{Title: "legacy label only", Labels: []string{LabelNeedsHuman}}
	// Closed needs_human bead must be excluded.
	closed := &Bead{Title: "closed needs-human", Labels: []string{LabelNeedsHuman}}

	require.NoError(t, s.Create(testCtx(), nh1))
	require.NoError(t, s.Create(testCtx(), nh2))
	require.NoError(t, s.Create(testCtx(), nhDep))
	require.NoError(t, s.Create(testCtx(), plain))
	require.NoError(t, s.Create(testCtx(), legacy))
	require.NoError(t, s.Create(testCtx(), closed))
	require.NoError(t, s.DepAdd(testCtx(), nhDep.ID, blocker.ID))
	require.NoError(t, s.Close(testCtx(), closed.ID))

	result, err := s.NeedsHuman()
	require.NoError(t, err)

	ids := make([]string, len(result))
	for i, b := range result {
		ids[i] = b.ID
	}

	// Must include all proposed beads regardless of dep status.
	assert.Contains(t, ids, nh1.ID)
	assert.Contains(t, ids, nh2.ID)
	assert.Contains(t, ids, nhDep.ID)

	// Must exclude non-proposed, legacy-label-only, and closed beads.
	assert.NotContains(t, ids, plain.ID)
	assert.NotContains(t, ids, legacy.ID)
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

func TestReadyExecutionSkipsProposedButIgnoresLegacyNeedsHumanLabel(t *testing.T) {
	s := newTestStore(t)

	legacy := &Bead{Title: "legacy needs-human open", Labels: []string{LabelNeedsHuman}}
	proposed := &Bead{Title: "proposed attention", Status: StatusProposed}
	require.NoError(t, s.Create(testCtx(), legacy))
	require.NoError(t, s.Create(testCtx(), proposed))

	// Worker drain ignores legacy lifecycle labels but skips proposed status.
	executionReady, err := s.ReadyExecution()
	require.NoError(t, err)
	foundLegacy := false
	for _, b := range executionReady {
		if b.ID == legacy.ID {
			foundLegacy = true
		}
		assert.NotEqual(t, proposed.ID, b.ID, "ReadyExecution must not include proposed beads")
	}
	assert.True(t, foundLegacy, "ReadyExecution must not filter legacy needs_human labels")

	// Ready query has the same status-owned semantics.
	depReady, err := s.Ready()
	require.NoError(t, err)
	found := false
	for _, b := range depReady {
		if b.ID == legacy.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Ready must include dep-satisfied legacy needs_human labels")
}

func TestStatusCountsIncludeOperatorAttentionAndWorkerReady(t *testing.T) {
	s := newTestStore(t)

	// Two plain open beads with no deps → both dep-ready and worker-ready.
	a := &Bead{Title: "worker-ready A"}
	b := &Bead{Title: "worker-ready B"}
	require.NoError(t, s.Create(testCtx(), a))
	require.NoError(t, s.Create(testCtx(), b))

	// One proposed bead with satisfied deps → operator-attention, not ready.
	nh := &Bead{Title: "operator attention", Status: StatusProposed, Labels: []string{LabelNeedsHuman}}
	require.NoError(t, s.Create(testCtx(), nh))

	// One closed bead.
	c := &Bead{Title: "closed"}
	require.NoError(t, s.Create(testCtx(), c))
	require.NoError(t, s.Close(testCtx(), c.ID))

	counts, err := s.Status()
	require.NoError(t, err)

	assert.Equal(t, 4, counts.Total)
	assert.Equal(t, 2, counts.Open)        // a, b
	assert.Equal(t, 1, counts.Proposed)    // nh
	assert.Equal(t, 1, counts.Closed)      // c
	assert.Equal(t, 2, counts.Ready)       // a, b
	assert.Equal(t, 2, counts.WorkerReady) // a, b
	assert.Equal(t, 1, counts.NeedsHuman)  // compatibility alias for operator attention
	assert.Equal(t, 1, counts.OperatorAttention)
}
