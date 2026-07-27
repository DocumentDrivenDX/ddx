package bead

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MutateFunc adapts an ad-hoc bead mutation into an Operation. Defined in
// the test build so the implementation does not show up in production
// reachability analysis (no production caller constructs MutateFunc).
type MutateFunc func(*Bead) error

// Apply executes the wrapped mutation.
func (m MutateFunc) Apply(b *Bead) error {
	return m(b)
}

func TestOperation_QueueSetTop_AssignsTopRank(t *testing.T) {
	t.Parallel()

	b := &Bead{Title: "queue-top test", Priority: 2}
	require.NoError(t, QueueSetTop{}.Apply(b))

	rank, ok := parseQueueRank(b.Extra["queue-rank"])
	require.True(t, ok, "QueueSetTop must assign a numeric queue-rank")
	assert.Equal(t, 0, rank, "QueueSetTop must assign rank 0 (top)")
	assert.Equal(t, 2, b.Priority, "QueueSetTop must not change priority")
}

func TestOperation_QueueSetPosition_RespectsBucket(t *testing.T) {
	t.Parallel()

	b := &Bead{Title: "queue-position test", Priority: 3}
	require.NoError(t, QueueSetPosition{Position: 42}.Apply(b))

	rank, ok := parseQueueRank(b.Extra["queue-rank"])
	require.True(t, ok, "QueueSetPosition must assign a numeric queue-rank")
	assert.Equal(t, 42, rank, "QueueSetPosition must assign the requested rank")
	assert.Equal(t, 3, b.Priority, "QueueSetPosition must not change priority (respects bucket)")
}

// TestOperation_QueueSetPositionCanonicalizesLegacyAlias proves QueueSetPosition
// replaces a legacy queue_rank with one numeric queue-rank and leaves no alias.
func TestOperation_QueueSetPositionCanonicalizesLegacyAlias(t *testing.T) {
	t.Parallel()

	b := &Bead{
		Title:    "legacy alias position",
		Priority: 1,
		Extra:    map[string]any{"queue_rank": 7, "other": "keep"},
	}
	require.NoError(t, QueueSetPosition{Position: 3}.Apply(b))

	rank, ok := parseQueueRank(b.Extra[queueRankKey])
	require.True(t, ok, "QueueSetPosition must write canonical queue-rank")
	assert.Equal(t, 3, rank)
	_, hasAlias := b.Extra[queueRankAliasKey]
	assert.False(t, hasAlias, "legacy queue_rank alias must not survive QueueSetPosition")
	assert.Equal(t, "keep", b.Extra["other"], "unrelated Extra keys must be preserved")
	assert.Len(t, b.Extra, 2, "only queue-rank and unrelated keys should remain")
}

func TestOperation_QueueClearOp_RemovesRank(t *testing.T) {
	t.Parallel()

	b := &Bead{
		Title:    "queue-clear test",
		Priority: 1,
		Extra:    map[string]any{"queue-rank": 99},
	}
	require.NoError(t, QueueClearOp{}.Apply(b))

	if b.Extra != nil {
		_, hasRank := b.Extra["queue-rank"]
		assert.False(t, hasRank, "QueueClearOp must remove queue-rank")
	}
}

func TestOperation_MutateFuncAdapter_RoundTrip(t *testing.T) {
	t.Parallel()

	var op Operation = MutateFunc(func(b *Bead) error {
		b.Title = "updated title"
		b.Notes = "mutated through Operation"
		return nil
	})

	b := &Bead{Title: "original title"}

	require.NoError(t, op.Apply(b))
	require.Equal(t, "updated title", b.Title)
	require.Equal(t, "mutated through Operation", b.Notes)
}
