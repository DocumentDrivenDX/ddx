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
