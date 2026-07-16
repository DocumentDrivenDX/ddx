package queue_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	beadqueue "github.com/DocumentDrivenDX/ddx/internal/bead/ops/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) *bead.Store {
	t.Helper()
	s := bead.NewStore(filepath.Join(t.TempDir(), ".ddx"))
	require.NoError(t, s.Init(context.Background()))
	return s
}

func createBead(t *testing.T, s *bead.Store, id, title string, priority int) *bead.Bead {
	t.Helper()
	b := &bead.Bead{
		ID:        id,
		Title:     title,
		Status:    bead.StatusOpen,
		Priority:  priority,
		IssueType: "task",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, s.Create(context.Background(), b))
	got, err := s.Get(context.Background(), b.ID)
	require.NoError(t, err)
	return got
}

type recordingLoader struct {
	beads   []bead.Bead
	applied []string
}

func (l *recordingLoader) ReadAll(context.Context) ([]bead.Bead, error) {
	return append([]bead.Bead(nil), l.beads...), nil
}

func (l *recordingLoader) Apply(id string, op bead.Operation) error {
	l.applied = append(l.applied, id)
	for i := range l.beads {
		if l.beads[i].ID == id {
			return op.Apply(&l.beads[i])
		}
	}
	return nil
}

func queueRank(t *testing.T, b bead.Bead) int {
	t.Helper()
	rank, ok := bead.QueueRank(b.Extra)
	require.True(t, ok, "%s must have an explicit queue rank", b.ID)
	return rank
}

// TestOpsQueue_Top_MovesBeadToFront verifies that Top sets an explicit rank that
// places the target before unranked beads in the same priority bucket.
func TestOpsQueue_Top_MovesBeadToFront(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	now := time.Now().UTC()

	b1 := &bead.Bead{ID: "a", Title: "A", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now, UpdatedAt: now}
	b2 := &bead.Bead{ID: "b", Title: "B", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)}
	require.NoError(t, s.Create(context.Background(), b1))
	require.NoError(t, s.Create(context.Background(), b2))

	// b2 was created after b1, so it normally sorts second. Top should move it first.
	require.NoError(t, beadqueue.Top(context.Background(), s, "b"))

	got, err := s.Get(context.Background(), "b")
	require.NoError(t, err)
	require.NotNil(t, got.Extra, "Top must set a queue-rank")
	_, hasRank := got.Extra["queue-rank"]
	assert.True(t, hasRank, "Top must assign queue-rank")

	// b should now sort before a in the ready queue
	ready, err := s.Ready()
	require.NoError(t, err)
	require.Len(t, ready, 2)
	assert.Equal(t, "b", ready[0].ID, "b should be first after Top")
}

// TestOpsQueue_TopRepeatedCallsStrictlyReorder proves Top never ties a newly
// topped bead with an existing rank-zero entry. Each call must place its target
// uniquely first while preserving the prior top order behind it.
func TestOpsQueue_TopRepeatedCallsStrictlyReorder(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	for i, id := range []string{"a", "b", "c"} {
		b := &bead.Bead{
			ID:        id,
			Title:     id,
			Status:    bead.StatusOpen,
			Priority:  0,
			IssueType: "task",
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
			UpdatedAt: now.Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, s.Create(context.Background(), b))
	}

	wantAfterTop := map[string][]string{
		"a": {"a", "b", "c"},
		"b": {"b", "a", "c"},
		"c": {"c", "b", "a"},
	}
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, beadqueue.Top(context.Background(), s, id))
		ready, err := s.ReadyExecution()
		require.NoError(t, err)
		gotIDs := make([]string, 0, len(ready))
		for _, row := range ready {
			gotIDs = append(gotIDs, row.ID)
		}
		assert.Equal(t, wantAfterTop[id], gotIDs)
	}

	ranks := make(map[any]string, 3)
	for _, id := range []string{"a", "b", "c"} {
		got, err := s.Get(context.Background(), id)
		require.NoError(t, err)
		rank, ok := got.Extra["queue-rank"]
		require.True(t, ok, "%s must have an explicit rank", id)
		if prior, exists := ranks[rank]; exists {
			t.Fatalf("%s and %s share queue-rank %v", prior, id, rank)
		}
		ranks[rank] = id
	}
}

func TestOpsQueue_TopRankZeroWritesOnlyTarget(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := &recordingLoader{beads: []bead.Bead{
		{ID: "old", Priority: 0, CreatedAt: now, Extra: map[string]any{"queue-rank": 0}},
		{ID: "new", Priority: 0, CreatedAt: now.Add(time.Minute)},
	}}

	require.NoError(t, beadqueue.Top(context.Background(), l, "new"))
	require.Equal(t, []string{"new"}, l.applied)
	assert.Equal(t, -1, queueRank(t, l.beads[1]))
	assert.Equal(t, 0, queueRank(t, l.beads[0]))
}

func TestOpsQueue_TopPrecedesLegacyAliasRank(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newStore(t)
	require.NoError(t, s.Create(context.Background(), &bead.Bead{
		ID: "store-old", Title: "old", Status: bead.StatusOpen, Priority: 0, IssueType: "task",
		CreatedAt: now, UpdatedAt: now, Extra: map[string]any{"queue_rank": -7},
	}))
	require.NoError(t, s.Create(context.Background(), &bead.Bead{
		ID: "store-new", Title: "new", Status: bead.StatusOpen, Priority: 0, IssueType: "task",
		CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
	}))
	require.NoError(t, beadqueue.Top(context.Background(), s, "store-new"))
	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, ready, 2)
	assert.Equal(t, []string{"store-new", "store-old"}, []string{ready[0].ID, ready[1].ID})

	// The sparse path must still mutate only the target bead.
	l := &recordingLoader{beads: []bead.Bead{
		{ID: "old", Priority: 0, CreatedAt: now, Extra: map[string]any{"queue_rank": -7}},
		{ID: "new", Priority: 0, CreatedAt: now.Add(time.Minute)},
	}}

	require.NoError(t, beadqueue.Top(context.Background(), l, "new"))
	require.Equal(t, []string{"new"}, l.applied, "a sparse Top must write only its target")
	assert.Equal(t, -8, queueRank(t, l.beads[1]))
	assert.Equal(t, -7, queueRank(t, l.beads[0]))
}

func TestOpsQueue_TopAtMinimumRankRenormalizesWithoutOverflow(t *testing.T) {
	t.Parallel()
	maxRank := int(^uint(0) >> 1)
	minRank := -maxRank - 1
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := &recordingLoader{beads: []bead.Bead{
		{ID: "old", Priority: 0, CreatedAt: now, Extra: map[string]any{"queue-rank": minRank}},
		{ID: "new", Priority: 0, CreatedAt: now.Add(time.Minute)},
	}}

	require.NoError(t, beadqueue.Top(context.Background(), l, "new"))
	require.Equal(t, []string{"new", "old"}, l.applied)
	assert.Equal(t, 0, queueRank(t, l.beads[1]))
	assert.Equal(t, 10, queueRank(t, l.beads[0]))
}

func TestOpsQueue_MoveRenormalizesAtMaxIntTail(t *testing.T) {
	t.Parallel()
	maxRank := int(^uint(0) >> 1)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := &recordingLoader{beads: []bead.Bead{
		{ID: "old", Priority: 0, CreatedAt: now, Extra: map[string]any{"queue-rank": maxRank}},
		{ID: "new", Priority: 0, CreatedAt: now.Add(time.Minute)},
	}}

	require.NoError(t, beadqueue.Move(context.Background(), l, "new", 1))
	require.Equal(t, []string{"old", "new"}, l.applied)
	assert.Equal(t, 0, queueRank(t, l.beads[0]))
	assert.Equal(t, 10, queueRank(t, l.beads[1]))
}

func TestOpsQueue_MoveBetweenExtremeRanksDoesNotOverflow(t *testing.T) {
	t.Parallel()
	maxRank := int(^uint(0) >> 1)
	minRank := -maxRank - 1
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := &recordingLoader{beads: []bead.Bead{
		{ID: "first", Priority: 0, CreatedAt: now, Extra: map[string]any{"queue-rank": minRank}},
		{ID: "last", Priority: 0, CreatedAt: now.Add(time.Minute), Extra: map[string]any{"queue-rank": maxRank}},
		{ID: "middle", Priority: 0, CreatedAt: now.Add(2 * time.Minute)},
	}}

	require.NoError(t, beadqueue.Move(context.Background(), l, "middle", 1))
	require.Equal(t, []string{"middle"}, l.applied)
	assert.Equal(t, minRank+1, queueRank(t, l.beads[2]))
}

// TestOpsQueue_Move_PreservesPriorityBucket verifies that Move sets queue-rank
// without altering the bead's priority field.
func TestOpsQueue_Move_PreservesPriorityBucket(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	now := time.Now().UTC()

	b := &bead.Bead{ID: "x", Title: "X", Status: bead.StatusOpen, Priority: 2, IssueType: "task", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, s.Create(context.Background(), b))

	require.NoError(t, beadqueue.Move(context.Background(), s, "x", 0))

	got, err := s.Get(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, 2, got.Priority, "Move must not change priority")
	_, hasRank := got.Extra["queue-rank"]
	assert.True(t, hasRank, "Move must assign a queue-rank")
}

// TestOpsQueue_Clear_RemovesOperatorOverride verifies that Clear removes the
// queue-rank key, restoring natural ordering.
func TestOpsQueue_Clear_RemovesOperatorOverride(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	now := time.Now().UTC()

	b := &bead.Bead{ID: "y", Title: "Y", Status: bead.StatusOpen, Priority: 1, IssueType: "task", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, s.Create(context.Background(), b))

	require.NoError(t, beadqueue.Top(context.Background(), s, "y"))
	got, err := s.Get(context.Background(), "y")
	require.NoError(t, err)
	_, hasRank := got.Extra["queue-rank"]
	require.True(t, hasRank, "queue-rank should be set before Clear")

	require.NoError(t, beadqueue.Clear(context.Background(), s, "y"))

	cleared, err := s.Get(context.Background(), "y")
	require.NoError(t, err)
	if cleared.Extra != nil {
		_, stillHasRank := cleared.Extra["queue-rank"]
		assert.False(t, stillHasRank, "Clear must remove queue-rank")
	}
}
