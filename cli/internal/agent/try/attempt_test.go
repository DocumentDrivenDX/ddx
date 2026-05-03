package try

import (
	"context"
	"errors"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// TestAttempt_WrapsLegacyExecutor verifies that Attempt delegates to the
// injected ExecuteBeadExecutor and converts its ExecuteBeadReport to a
// try.Outcome via the C1 ToOutcome adapter.
func TestAttempt_WrapsLegacyExecutor(t *testing.T) {
	t.Run("merged report becomes DispositionMerged", func(t *testing.T) {
		var gotBead string
		stub := agent.ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			gotBead = beadID
			return agent.ExecuteBeadReport{
				BeadID:     "ddx-aaaaaaaa",
				AttemptID:  "att-1",
				BaseRev:    "deadbeef",
				ResultRev:  "cafebabe",
				SessionID:  "sess-xyz",
				Status:     agent.ExecuteBeadStatusSuccess,
				CostUSD:    0.42,
				DurationMS: 1234,
			}, nil
		})
		out, err := Attempt(context.Background(), nil, "ddx-aaaaaaaa", AttemptOpts{Executor: stub})
		if err != nil {
			t.Fatalf("Attempt err = %v, want nil", err)
		}
		if gotBead != "ddx-aaaaaaaa" {
			t.Fatalf("stub got beadID = %q, want %q", gotBead, "ddx-aaaaaaaa")
		}
		if out.Disposition != DispositionMerged {
			t.Errorf("Disposition = %v, want %v", out.Disposition, DispositionMerged)
		}
		if out.BeadID != "ddx-aaaaaaaa" || out.AttemptID != "att-1" {
			t.Errorf("Outcome ids = (%q,%q), want (ddx-aaaaaaaa, att-1)", out.BeadID, out.AttemptID)
		}
		if out.BaseRev != "deadbeef" || out.ResultRev != "cafebabe" {
			t.Errorf("Outcome revs = (%q,%q), want (deadbeef, cafebabe)", out.BaseRev, out.ResultRev)
		}
		if out.SessionID != "sess-xyz" || out.CostUSD != 0.42 || out.DurationMS != 1234 {
			t.Errorf("Outcome telemetry = (%q,%v,%v), want (sess-xyz, 0.42, 1234)", out.SessionID, out.CostUSD, out.DurationMS)
		}
	})

	t.Run("preserved-needs-review report becomes DispositionPark/needs_review", func(t *testing.T) {
		stub := agent.ExecuteBeadExecutorFunc(func(_ context.Context, _ string) (agent.ExecuteBeadReport, error) {
			return agent.ExecuteBeadReport{
				BeadID: "ddx-bbbbbbbb",
				Status: agent.ExecuteBeadStatusPreservedNeedsReview,
			}, nil
		})
		out, err := Attempt(context.Background(), nil, "ddx-bbbbbbbb", AttemptOpts{Executor: stub})
		if err != nil {
			t.Fatalf("Attempt err = %v, want nil", err)
		}
		if out.Disposition != DispositionPark {
			t.Errorf("Disposition = %v, want %v", out.Disposition, DispositionPark)
		}
		if out.ParkReason != ParkReasonNeedsReview {
			t.Errorf("ParkReason = %q, want %q", out.ParkReason, ParkReasonNeedsReview)
		}
	})

	t.Run("executor error is propagated; outcome carries beadID", func(t *testing.T) {
		wantErr := errors.New("boom")
		stub := agent.ExecuteBeadExecutorFunc(func(_ context.Context, _ string) (agent.ExecuteBeadReport, error) {
			return agent.ExecuteBeadReport{}, wantErr
		})
		out, err := Attempt(context.Background(), nil, "ddx-cccccccc", AttemptOpts{Executor: stub})
		if !errors.Is(err, wantErr) {
			t.Fatalf("Attempt err = %v, want %v", err, wantErr)
		}
		if out.BeadID != "ddx-cccccccc" {
			t.Errorf("Outcome.BeadID = %q, want %q", out.BeadID, "ddx-cccccccc")
		}
	})
}
