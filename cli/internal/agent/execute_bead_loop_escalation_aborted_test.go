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

var errLadderExhaustedTest = fmt.Errorf("ladder exhausted")

func TestAutoReopenRetryableProviderConnectivityProposals(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	retryable := &bead.Bead{
		ID:     "ddx-eabc-retry",
		Title:  "retry provider connectivity proposal",
		Status: bead.StatusProposed,
		Labels: []string{bead.LabelNeedsHuman, "area:agent"},
	}
	require.NoError(t, store.Create(context.Background(), retryable))
	require.NoError(t, store.Update(context.Background(), retryable.ID, func(b *bead.Bead) {
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason:          "provider grendel / model qwen3.5-27b unreachable on 2+ attempts; check ddx config or provider health",
			Source:          "ddx work",
			SuggestedAction: "check provider health or reconfigure endpoints in .ddx/config.yaml",
			Summary:         "provider unreachable on repeated attempts",
		})
	}))

	manual := &bead.Bead{
		ID:     "ddx-eabc-manual",
		Title:  "manual operator proposal",
		Status: bead.StatusProposed,
		Labels: []string{bead.LabelNeedsHuman},
	}
	require.NoError(t, store.Create(context.Background(), manual))
	require.NoError(t, store.Update(context.Background(), manual.ID, func(b *bead.Bead) {
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason:          "scope is ambiguous and requires product judgment",
			Source:          "ddx work",
			SuggestedAction: "clarify acceptance criteria",
			Summary:         "manual operator review required",
		})
	}))

	at := time.Date(2026, 5, 17, 14, 0, 0, 0, time.UTC)
	emitted := make([]string, 0, 1)
	reopened, err := autoReopenRetryableProviderConnectivityProposals(
		context.Background(),
		store,
		"test-actor",
		at,
		func(kind string, _ map[string]any) { emitted = append(emitted, kind) },
	)
	require.NoError(t, err)
	assert.Equal(t, 1, reopened)
	assert.Equal(t, []string{"provider_connectivity.auto_reopen"}, emitted)

	gotRetryable, err := store.Get(context.Background(), retryable.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotRetryable.Status)
	assert.NotContains(t, gotRetryable.Labels, bead.LabelNeedsHuman)
	assert.Empty(t, bead.GetNeedsHumanMeta(*gotRetryable).Reason)

	events, err := store.Events(retryable.ID)
	require.NoError(t, err)
	assert.Condition(t, func() bool {
		for _, ev := range events {
			if ev.Kind == "provider_connectivity.auto_reopen" {
				return true
			}
		}
		return false
	}, "auto-reopen event must be recorded")

	gotManual, err := store.Get(context.Background(), manual.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, gotManual.Status)
	assert.Contains(t, gotManual.Labels, bead.LabelNeedsHuman)
}

// TestApplyNoChangesSmartRetry_LadderExhaustedEmitsEvent asserts that when the
// escalation ladder is exhausted, applyNoChangesSmartRetry emits a
// kind=execution-escalation-aborted event before parking the bead.
func TestApplyNoChangesSmartRetry_LadderExhaustedEmitsEvent(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-eabc04", Title: "no-changes smart retry ladder exhausted event"}
	require.NoError(t, store.Create(context.Background(), b))

	noChanges := &agenttry.NoChangesOutcome{
		Reason:          "agent cannot proceed further",
		SuggestedAction: "operator review needed",
	}
	exhaustedFn := func(int) (int, error) { return 0, errLadderExhaustedTest }

	err := applyNoChangesSmartRetry(store, b.ID, "test-actor", noChanges, 40, exhaustedFn)
	require.NoError(t, err)

	events, err := store.Events(b.ID)
	require.NoError(t, err)

	var aborted *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "execution-escalation-aborted" {
			aborted = &events[i]
			break
		}
	}
	require.NotNil(t, aborted, "execution-escalation-aborted event must be emitted when no-changes ladder is exhausted")
	assert.Contains(t, aborted.Summary, "40", "summary must reference actualPower")
	assert.Equal(t, "ddx work", aborted.Source)
}

// TestApplyRepairCycleExhaustedEscalation_LadderExhaustedEmitsEvent asserts that
// when the escalation ladder is exhausted, applyRepairCycleExhaustedEscalation
// emits a kind=execution-escalation-aborted event before parking the bead.
func TestApplyRepairCycleExhaustedEscalation_LadderExhaustedEmitsEvent(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-eabc05", Title: "repair cycle exhausted event"}
	require.NoError(t, store.Create(context.Background(), b))

	exhaustedFn := func(int) (int, error) { return 0, errLadderExhaustedTest }
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	err := applyRepairCycleExhaustedEscalation(store, b.ID, "test-actor", 60, at, exhaustedFn)
	require.NoError(t, err)

	events, err := store.Events(b.ID)
	require.NoError(t, err)

	var aborted *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "execution-escalation-aborted" {
			aborted = &events[i]
			break
		}
	}
	require.NotNil(t, aborted, "execution-escalation-aborted event must be emitted when repair-cycle ladder is exhausted")
	assert.Contains(t, aborted.Summary, "60", "summary must reference actualPower")
	assert.Equal(t, "ddx work", aborted.Source)
}
