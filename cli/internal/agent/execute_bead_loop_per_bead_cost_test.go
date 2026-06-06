package agent

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPerBeadCostTracker_ChainsIntoAutoRecovery asserts that
// incrementConsecutiveLadderExhaustions increments the counter stored in
// Extra["consecutive_ladder_exhaustions"] each time it is called. The counter
// drives the auto-recovery hook (sister bead ddx-63155d5c): once the counter
// reaches the threshold the hook fires.
func TestPerBeadCostTracker_ChainsIntoAutoRecovery(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-chain-test", Title: "chain test bead"}
	require.NoError(t, store.Create(context.

		// Counter starts at zero (key absent).
		Background(), b))

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Extra[consecutiveLadderExhaustionsKey], "counter should be absent before any exhaustion")

	// First per-bead budget exhaustion: counter becomes 1.
	require.NoError(t, incrementConsecutiveLadderExhaustions(store, b.ID))
	got, err = store.Get(b.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, got.Extra[consecutiveLadderExhaustionsKey],
		"counter must be 1 after first per-bead budget exhaustion")

	// Second per-bead budget exhaustion: counter becomes 2 (auto-recovery threshold).
	require.NoError(t, incrementConsecutiveLadderExhaustions(store, b.ID))
	got, err = store.Get(b.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, got.Extra[consecutiveLadderExhaustionsKey],
		"counter must be 2 after second per-bead budget exhaustion (auto-recovery threshold)")
}
