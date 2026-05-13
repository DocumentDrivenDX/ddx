package bead

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReviewAccuracy_OperatorOverride_EmitsEvent verifies that Store.Close and
// Store.Reopen emit "review-accuracy-override" events when the operator's
// action contradicts the reviewer's prior verdict.
func TestReviewAccuracy_OperatorOverride_EmitsEvent(t *testing.T) {
	t.Run("Close after BLOCK emits override event", func(t *testing.T) {
		s := newTestStore(t)

		b := &Bead{Title: "test bead"}
		require.NoError(t, s.Create(testCtx(), b))

		// Simulate a BLOCK reviewer verdict event.
		require.NoError(t, s.AppendEvent(b.ID, BeadEvent{
			Kind:    "review",
			Summary: "BLOCK",
			Body:    "AC3 not implemented",
		}))

		// Operator manually closes despite BLOCK verdict.
		require.NoError(t, s.Close(testCtx(), b.ID))

		// The override event must be present in the sidecar attachment.
		events, err := s.Events(b.ID)
		require.NoError(t, err)

		found := false
		for _, ev := range events {
			if ev.Kind == "review-accuracy-override" {
				found = true
				if !strings.Contains(ev.Body, "verdict_was=BLOCK") {
					t.Errorf("expected verdict_was=BLOCK in body, got: %s", ev.Body)
				}
				if !strings.Contains(ev.Body, "operator_action=close") {
					t.Errorf("expected operator_action=close in body, got: %s", ev.Body)
				}
				break
			}
		}
		if !found {
			t.Errorf("expected review-accuracy-override event after Close on BLOCK-reviewed bead; events: %+v", events)
		}
	})

	t.Run("Close after APPROVE does not emit override event", func(t *testing.T) {
		s := newTestStore(t)

		b := &Bead{Title: "approved bead"}
		require.NoError(t, s.Create(testCtx(), b))

		require.NoError(t, s.AppendEvent(b.ID, BeadEvent{
			Kind:    "review",
			Summary: "APPROVE",
			Body:    "all ACs pass",
		}))

		require.NoError(t, s.Close(testCtx(), b.ID))

		events, err := s.Events(b.ID)
		require.NoError(t, err)

		for _, ev := range events {
			if ev.Kind == "review-accuracy-override" {
				t.Errorf("unexpected review-accuracy-override event after Close on APPROVE-reviewed bead")
			}
		}
	})

	t.Run("Reopen after APPROVE emits override event", func(t *testing.T) {
		s := newTestStore(t)

		b := &Bead{Title: "reopen bead"}
		require.NoError(t, s.Create(testCtx(), b))

		// Simulate an APPROVE verdict followed by a close (using Store.Close
		// to bypass ClosureGate for simplicity).
		require.NoError(t, s.AppendEvent(b.ID, BeadEvent{
			Kind:    "review",
			Summary: "APPROVE",
			Body:    "all ACs pass",
		}))
		require.NoError(t, s.Close(testCtx(), b.ID))

		// Operator reopens after suspecting fake work.
		require.NoError(t, s.Reopen(b.ID, "fake-work", "reopened for investigation"))

		events, err := s.Events(b.ID)
		require.NoError(t, err)

		found := false
		for _, ev := range events {
			if ev.Kind == "review-accuracy-override" {
				found = true
				if !strings.Contains(ev.Body, "verdict_was=APPROVE") {
					t.Errorf("expected verdict_was=APPROVE in body, got: %s", ev.Body)
				}
				if !strings.Contains(ev.Body, "operator_action=reopen") {
					t.Errorf("expected operator_action=reopen in body, got: %s", ev.Body)
				}
				break
			}
		}
		if !found {
			t.Errorf("expected review-accuracy-override event after Reopen on APPROVE-reviewed bead; events: %+v", events)
		}
	})

	t.Run("Reopen after BLOCK does not emit override event", func(t *testing.T) {
		s := newTestStore(t)

		b := &Bead{Title: "block reopen bead"}
		require.NoError(t, s.Create(testCtx(), b))

		require.NoError(t, s.AppendEvent(b.ID, BeadEvent{
			Kind:    "review",
			Summary: "BLOCK",
			Body:    "AC3 not implemented",
		}))
		require.NoError(t, s.Close(testCtx(), b.ID))

		// When reopening after BLOCK (operator agrees with reviewer), no override.
		require.NoError(t, s.Reopen(b.ID, "rework", ""))

		events, err := s.Events(b.ID)
		require.NoError(t, err)

		overrideCount := 0
		for _, ev := range events {
			if ev.Kind == "review-accuracy-override" {
				// The Close already emitted one for the BLOCK override; Reopen should not add another.
				overrideCount++
			}
		}
		// Only the Close-on-BLOCK override should be present; Reopen-after-BLOCK should not add one.
		if overrideCount > 1 {
			t.Errorf("expected at most 1 review-accuracy-override event (from Close on BLOCK), got %d", overrideCount)
		}
	})
}
