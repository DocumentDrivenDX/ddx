package bead

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventsByKind(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "test bead"}
	require.NoError(t, s.Create(b))

	// Append a mix of event kinds.
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "routing", Summary: "provider=claude model=opus reason=config"}))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "summary", Summary: "completed"}))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "routing", Summary: "provider=claude model=sonnet reason=fallback"}))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "error", Summary: "transient failure"}))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "routing", Summary: "provider=local model=qwen reason=cost"}))

	routing, err := s.EventsByKind(b.ID, "routing")
	require.NoError(t, err)
	require.Len(t, routing, 3)
	assert.Equal(t, "provider=claude model=opus reason=config", routing[0].Summary)
	assert.Equal(t, "provider=claude model=sonnet reason=fallback", routing[1].Summary)
	assert.Equal(t, "provider=local model=qwen reason=cost", routing[2].Summary)

	summaries, err := s.EventsByKind(b.ID, "summary")
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "completed", summaries[0].Summary)

	missing, err := s.EventsByKind(b.ID, "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, missing)
}

func TestEventsByKindEmptyBead(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "no events bead"}
	require.NoError(t, s.Create(b))

	events, err := s.EventsByKind(b.ID, "routing")
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestEventsByKindUnknownBead(t *testing.T) {
	s := newTestStore(t)

	_, err := s.EventsByKind("ddx-notexist", "routing")
	assert.Error(t, err)
}
