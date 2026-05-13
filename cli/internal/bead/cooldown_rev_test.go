package bead

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestActiveRetryCooldown_RevBoundAutoClears verifies that when origin/main
// advances past the rev recorded at cooldown time, the cooldown auto-clears
// even though the wall-clock has not expired.
func TestActiveRetryCooldown_RevBoundAutoClears(t *testing.T) {
	b := Bead{
		Extra: map[string]any{
			ExtraRetryAfter:      time.Now().UTC().Add(6 * time.Hour).Format(time.RFC3339),
			ExtraCooldownBaseRev: "aaaaaa1111111111111111111111111111111111",
		},
	}
	// origin/main is now at a different SHA — the world has moved on.
	originHead := "bbbbbb2222222222222222222222222222222222"
	_, active := activeRetryCooldown(b, time.Now(), originHead)
	assert.False(t, active, "cooldown must auto-clear when origin HEAD has advanced past base-rev")
}

// TestActiveRetryCooldown_SameRevStillActive verifies that when origin/main
// has NOT advanced (same rev), the wall-clock cooldown remains active.
func TestActiveRetryCooldown_SameRevStillActive(t *testing.T) {
	rev := "aaaaaa1111111111111111111111111111111111"
	b := Bead{
		Extra: map[string]any{
			ExtraRetryAfter:      time.Now().UTC().Add(6 * time.Hour).Format(time.RFC3339),
			ExtraCooldownBaseRev: rev,
		},
	}
	_, active := activeRetryCooldown(b, time.Now(), rev)
	assert.True(t, active, "cooldown must remain active when origin HEAD has not advanced")
}

// TestActiveRetryCooldown_NoBaseRevFallsBackToWallClock verifies backward
// compatibility: legacy cooldowns without execute-loop-cooldown-base-rev still
// apply wall-clock logic regardless of the originHead value.
func TestActiveRetryCooldown_NoBaseRevFallsBackToWallClock(t *testing.T) {
	future := time.Now().UTC().Add(6 * time.Hour).Format(time.RFC3339)
	bFuture := Bead{
		Extra: map[string]any{
			ExtraRetryAfter: future,
			// No ExtraCooldownBaseRev — legacy cooldown
		},
	}
	// Even with a different origin HEAD, wall-clock wins when base-rev is absent.
	_, activeWithAdvancedHead := activeRetryCooldown(bFuture, time.Now(), "bbbbbb2222222222222222222222222222222222")
	assert.True(t, activeWithAdvancedHead, "legacy cooldown without base-rev must stay active (wall-clock only)")

	// Also verify: past wall-clock clears correctly.
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	bPast := Bead{
		Extra: map[string]any{
			ExtraRetryAfter: past,
		},
	}
	_, activeExpired := activeRetryCooldown(bPast, time.Now(), "")
	assert.False(t, activeExpired, "expired wall-clock cooldown must not be active")
}

// TestSetExecutionCooldown_WritesBaseRev verifies that SetExecutionCooldown
// writes execute-loop-cooldown-base-rev when a non-empty baseRev is passed.
func TestSetExecutionCooldown_WritesBaseRev(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "test bead"}
	require.NoError(t, s.Create(testCtx(), b))

	baseRev := "deadbeef1111111111111111111111111111111"
	until := time.Now().UTC().Add(time.Hour)
	require.NoError(t, s.SetExecutionCooldown(b.ID, until, "no_changes", "detail", baseRev))

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, baseRev, got.Extra[ExtraCooldownBaseRev], "base-rev must be written to Extra")
	assert.NotEmpty(t, got.Extra[ExtraRetryAfter])
}

// TestSetExecutionCooldown_ClearsBaseRevWhenEmpty verifies that passing an
// empty baseRev removes any previously recorded execute-loop-cooldown-base-rev.
func TestSetExecutionCooldown_ClearsBaseRevWhenEmpty(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{
		Title: "test bead",
		Extra: map[string]any{
			ExtraCooldownBaseRev: "stale-rev",
		},
	}
	require.NoError(t, s.Create(testCtx(), b))

	until := time.Now().UTC().Add(time.Hour)
	require.NoError(t, s.SetExecutionCooldown(b.ID, until, "no_changes", "detail", ""))

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.NotContains(t, got.Extra, ExtraCooldownBaseRev, "empty baseRev must remove stale base-rev from Extra")
}
