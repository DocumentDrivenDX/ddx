package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type noChangesCountStore struct {
	count int
}

func (s *noChangesCountStore) AppendEvent(string, bead.BeadEvent) error       { return nil }
func (s *noChangesCountStore) CloseWithEvidence(string, string, string) error { return nil }
func (s *noChangesCountStore) Unclaim(string) error                           { return nil }
func (s *noChangesCountStore) SetExecutionCooldown(string, time.Time, string, string, string) error {
	return nil
}
func (s *noChangesCountStore) UpdateWithLifecycleStatus(string, string, bead.LifecycleTransitionOptions, func(*bead.Bead) error) error {
	return nil
}
func (s *noChangesCountStore) IncrNoChangesCount(string) (int, error) {
	s.count++
	return s.count, nil
}

func noChangesAttempt(t *testing.T, rationale string) (agenttry.Outcome, error) {
	t.Helper()
	store := &noChangesCountStore{}
	return agenttry.Attempt(context.Background(), store, "ddx-nochanges", agenttry.AttemptOpts{
		Executor: agenttry.ExecutorFunc(func(context.Context, string) (agenttry.Report, error) {
			return agenttry.Report{
				BeadID:             "ddx-nochanges",
				Status:             agenttry.StatusNoChanges,
				ActualPower:        70,
				BaseRev:            "base",
				ResultRev:          "base",
				NoChangesRationale: rationale,
			}, nil
		}),
	})
}

func TestNoChangesBadAttempt_EscalatesNextAttemptPower(t *testing.T) {
	out, err := noChangesAttempt(t, "the work seems done")
	require.NoError(t, err)
	require.NotNil(t, out.NoChanges)
	assert.Equal(t, 1, out.Report.EscalationCount, "IncrNoChangesCount must feed the attempt report")
	assert.Equal(t, agenttry.NoChangesActionBadAttemptNoCooldown, out.NoChanges.Action)

	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-bead-1", Title: "bad-attempt escalation"}))

	require.NoError(t, applyNoChangesBadAttemptEscalation(store, "ddx-bead-1", "worker", out.NoChanges, out.Report.ActualPower, func(actualPower int) (int, error) {
		return actualPower + 1, nil
	}))

	updated, err := store.Get("ddx-bead-1")
	require.NoError(t, err)
	require.Equal(t, bead.StatusOpen, updated.Status)
	require.Equal(t, 71, noChangesMinPowerOverride(updated, 70))
	assert.EqualValues(t, 71, updated.Extra[executeLoopNoChangesNextMinPowerKey])
}

func TestNoEvidenceProduced_EscalatesImmediately(t *testing.T) {
	out, err := noChangesAttempt(t, "")
	require.NoError(t, err)
	require.NotNil(t, out.NoChanges)
	assert.Equal(t, 1, out.Report.EscalationCount, "IncrNoChangesCount must increment on the first no-evidence attempt")
	assert.Equal(t, agenttry.NoChangesActionBadAttemptNoCooldown, out.NoChanges.Action)

	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-bead-2", Title: "no-evidence escalation"}))

	require.NoError(t, applyNoChangesBadAttemptEscalation(store, "ddx-bead-2", "worker", out.NoChanges, out.Report.ActualPower, func(actualPower int) (int, error) {
		return actualPower + 2, nil
	}))

	updated, err := store.Get("ddx-bead-2")
	require.NoError(t, err)
	require.Equal(t, bead.StatusOpen, updated.Status)
	require.Equal(t, 72, noChangesMinPowerOverride(updated, 70))
	assert.EqualValues(t, 72, updated.Extra[executeLoopNoChangesNextMinPowerKey])
}

func TestNoChangesEscalation_BoundedByLadderCap(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-bead-3", Title: "ladder cap escalation"}))

	noChanges := &agenttry.NoChangesOutcome{
		Action:          agenttry.NoChangesActionBadAttemptNoCooldown,
		EventKind:       agenttry.NoChangesEventUnjustified,
		Reason:          "stronger model needed but no higher rung remains",
		SuggestedAction: "park for operator review",
	}

	require.NoError(t, applyNoChangesBadAttemptEscalation(store, "ddx-bead-3", "worker", noChanges, 90, func(int) (int, error) {
		return 0, fmt.Errorf("ladder exhausted")
	}))

	updated, err := store.Get("ddx-bead-3")
	require.NoError(t, err)
	require.Equal(t, bead.StatusProposed, updated.Status)
	assert.Empty(t, updated.Owner)
	_, hasHint := updated.Extra[executeLoopNoChangesNextMinPowerKey]
	assert.False(t, hasHint, "top-of-ladder parking must clear the next-min-power hint")
}
