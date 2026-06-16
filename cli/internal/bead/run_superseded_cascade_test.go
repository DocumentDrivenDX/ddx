package bead

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunSupersededCascade_ClosesEligibleLeftover verifies the out-of-band
// cascade closes an open bead X superseded by an already-closed superseder Y,
// returning the closed count. This is the idle-path remediation entry point
// for superseded_pending_close.
func TestRunSupersededCascade_ClosesEligibleLeftover(t *testing.T) {
	s := newTestStore(t)

	y := &Bead{Title: "superseder"}
	require.NoError(t, s.Create(testCtx(), y))
	require.NoError(t, s.Close(testCtx(), y.ID))

	x := &Bead{
		Title:  "superseded leftover",
		Status: StatusOpen,
		Extra:  map[string]any{"superseded-by": y.ID},
	}
	require.NoError(t, s.Create(testCtx(), x))

	closed, err := s.RunSupersededCascade(y.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, closed)

	xGot, err := s.Get(testCtx(), x.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, xGot.Status)

	// Idempotent: re-running closes nothing new.
	closedAgain, err := s.RunSupersededCascade(y.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, closedAgain)
}

// TestRunSupersededCascade_RespectsScopeGuards verifies the cascade leaves an
// operator-noted superseded bead open (same scope guards as Store.Close).
func TestRunSupersededCascade_RespectsScopeGuards(t *testing.T) {
	s := newTestStore(t)

	y := &Bead{Title: "superseder"}
	require.NoError(t, s.Create(testCtx(), y))
	require.NoError(t, s.Close(testCtx(), y.ID))

	x := &Bead{
		Title:  "superseded but operator-noted",
		Status: StatusOpen,
		Notes:  "operator decision to keep this open",
		Extra:  map[string]any{"superseded-by": y.ID},
	}
	require.NoError(t, s.Create(testCtx(), x))

	closed, err := s.RunSupersededCascade(y.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, closed)

	xGot, err := s.Get(testCtx(), x.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, xGot.Status)
}
