package bead

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeedsHumanMetadataRoundTrip(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "needs human bead"}
	require.NoError(t, s.Create(b))

	require.NoError(t, s.Update(b.ID, func(b *Bead) {
		b.Labels = append(b.Labels, LabelNeedsHuman)
		SetNeedsHumanMeta(b, NeedsHumanMeta{
			Reason:          "review-block",
			Since:           "2026-05-07T00:00:00Z",
			Source:          "execute-loop",
			SuggestedAction: "split or retry",
			Summary:         "Agent failed 3 times",
		})
	}))

	got, err := s.Get(b.ID)
	require.NoError(t, err)

	assert.Contains(t, got.Labels, LabelNeedsHuman)

	meta := GetNeedsHumanMeta(*got)
	assert.Equal(t, "review-block", meta.Reason)
	assert.Equal(t, "2026-05-07T00:00:00Z", meta.Since)
	assert.Equal(t, "execute-loop", meta.Source)
	assert.Equal(t, "split or retry", meta.SuggestedAction)
	assert.Equal(t, "Agent failed 3 times", meta.Summary)

	// Verify raw Extra keys round-trip
	assert.Equal(t, "review-block", got.Extra[ExtraNeedsHumanReason])
	assert.Equal(t, "2026-05-07T00:00:00Z", got.Extra[ExtraNeedsHumanSince])
	assert.Equal(t, "execute-loop", got.Extra[ExtraNeedsHumanSource])
	assert.Equal(t, "split or retry", got.Extra[ExtraNeedsHumanSuggestedAction])
	assert.Equal(t, "Agent failed 3 times", got.Extra[ExtraNeedsHumanSummary])

	// SetNeedsHumanMeta with partial values clears omitted keys
	require.NoError(t, s.Update(b.ID, func(b *Bead) {
		SetNeedsHumanMeta(b, NeedsHumanMeta{Reason: "new-reason"})
	}))
	got2, err := s.Get(b.ID)
	require.NoError(t, err)
	meta2 := GetNeedsHumanMeta(*got2)
	assert.Equal(t, "new-reason", meta2.Reason)
	assert.Empty(t, meta2.Since)
	assert.Empty(t, meta2.Source)
	assert.Empty(t, meta2.SuggestedAction)
	assert.Empty(t, meta2.Summary)
	assert.NotContains(t, got2.Extra, ExtraNeedsHumanSince)
	assert.NotContains(t, got2.Extra, ExtraNeedsHumanSource)
}

func TestStoreNeedsHumanListsOpenNeedsHumanSorted(t *testing.T) {
	s := newTestStore(t)

	p1 := &Bead{Title: "P1 needs human", Priority: 1}
	p0 := &Bead{Title: "P0 needs human", Priority: 0}
	ordinary := &Bead{Title: "ordinary bead"}
	toClose := &Bead{Title: "closed needs human", Priority: 0}

	require.NoError(t, s.Create(p1))
	require.NoError(t, s.Create(p0))
	require.NoError(t, s.Create(ordinary))
	require.NoError(t, s.Create(toClose))

	require.NoError(t, s.Update(p1.ID, func(b *Bead) {
		b.Labels = append(b.Labels, LabelNeedsHuman)
	}))
	require.NoError(t, s.Update(p0.ID, func(b *Bead) {
		b.Labels = append(b.Labels, LabelNeedsHuman)
	}))
	require.NoError(t, s.Update(toClose.ID, func(b *Bead) {
		b.Labels = append(b.Labels, LabelNeedsHuman)
	}))
	require.NoError(t, s.Close(toClose.ID))

	result, err := s.NeedsHuman()
	require.NoError(t, err)

	ids := make([]string, len(result))
	for i, b := range result {
		ids[i] = b.ID
	}
	assert.Contains(t, ids, p0.ID)
	assert.Contains(t, ids, p1.ID)
	assert.NotContains(t, ids, ordinary.ID, "ordinary bead without needs_human label excluded")
	assert.NotContains(t, ids, toClose.ID, "closed bead excluded")

	require.Len(t, result, 2)
	assert.Equal(t, p0.ID, result[0].ID, "P0 sorts before P1")
	assert.Equal(t, p1.ID, result[1].ID)
}

func TestReadyExecutionSkipsNeedsHumanButReadyCanIncludeWhenRequested(t *testing.T) {
	s := newTestStore(t)

	dep := &Bead{Title: "dep"}
	nh := &Bead{Title: "needs human bead"}
	ordinary := &Bead{Title: "ordinary ready bead"}

	require.NoError(t, s.Create(dep))
	require.NoError(t, s.Create(nh))
	require.NoError(t, s.Create(ordinary))

	require.NoError(t, s.Close(dep.ID))

	require.NoError(t, s.Update(nh.ID, func(b *Bead) {
		b.Labels = append(b.Labels, LabelNeedsHuman)
	}))

	// Ready() uses dependency-ready semantics — includes needs_human beads
	ready, err := s.Ready()
	require.NoError(t, err)
	readyIDs := make(map[string]bool, len(ready))
	for _, b := range ready {
		readyIDs[b.ID] = true
	}
	assert.True(t, readyIDs[nh.ID], "Ready should include dep-satisfied needs_human beads")
	assert.True(t, readyIDs[ordinary.ID], "Ready should include ordinary dep-satisfied beads")

	// ReadyExecution() uses worker-drain semantics — skips needs_human beads
	execReady, err := s.ReadyExecution()
	require.NoError(t, err)
	execIDs := make(map[string]bool, len(execReady))
	for _, b := range execReady {
		execIDs[b.ID] = true
	}
	assert.False(t, execIDs[nh.ID], "ReadyExecution should skip needs_human beads")
	assert.True(t, execIDs[ordinary.ID], "ReadyExecution should include ordinary dep-satisfied beads")
}

func TestStatusCountsIncludeNeedsHumanAndWorkerReady(t *testing.T) {
	s := newTestStore(t)

	dep := &Bead{Title: "dep"}
	nh := &Bead{Title: "needs human"}
	worker := &Bead{Title: "worker ready"}
	depBlocked := &Bead{Title: "dep blocked"}

	require.NoError(t, s.Create(dep))
	require.NoError(t, s.Create(nh))
	require.NoError(t, s.Create(worker))
	require.NoError(t, s.Create(depBlocked))

	// depBlocked depends on dep (which is open → depBlocked is not ready)
	require.NoError(t, s.DepAdd(depBlocked.ID, dep.ID))

	require.NoError(t, s.Update(nh.ID, func(b *Bead) {
		b.Labels = append(b.Labels, LabelNeedsHuman)
	}))

	counts, err := s.Status()
	require.NoError(t, err)

	assert.Equal(t, 4, counts.Total)
	assert.Equal(t, 4, counts.Open)
	assert.Equal(t, 0, counts.Closed)
	// dep, nh, worker are dependency-ready; depBlocked is not
	assert.Equal(t, 3, counts.Ready)
	assert.Equal(t, 1, counts.Blocked)
	// Only nh has needs_human label
	assert.Equal(t, 1, counts.NeedsHuman)
	// dep and worker are worker-ready; nh is excluded by needs_human label
	assert.Equal(t, 2, counts.WorkerReady)
}
