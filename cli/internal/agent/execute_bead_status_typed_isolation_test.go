package agent

import (
	"reflect"
	"testing"

	agentlib "github.com/easel/fizeau"

	"github.com/DocumentDrivenDX/ddx/internal/agent/failclass"
)

// TestFinalFizeauOutcomeDoesNotParseStatusText verifies that
// AttemptPolicyDecisionForResult — the single production entry point for the
// typed DDx attempt-policy adapter — derives its Action/Reason solely from
// the typed Fizeau lifecycle tuple (FizeauOutcome/FizeauCause/FizeauStage).
// Varying or blanking res.Status and res.Error must never change the typed
// decision, proving the legacy text-driven compatibility path
// (ClassifyFailureMode) is not consulted on this route.
func TestFinalFizeauOutcomeDoesNotParseStatusText(t *testing.T) {
	cases := []struct {
		name       string
		fizeau     ExecuteBeadResult
		wantAction failclass.AttemptPolicyAction
		wantReason string
	}{
		{
			name: "completed_close",
			fizeau: ExecuteBeadResult{
				FizeauOutcome: string(agentlib.SessionOutcomeSuccess),
				FizeauCause:   string(agentlib.TerminalCauseCompleted),
				FizeauStage:   string(agentlib.SessionStageHarness),
			},
			wantAction: failclass.AttemptPolicyActionClose,
			wantReason: "fizeau_outcome_success",
		},
		{
			name: "provider_failed_parks",
			fizeau: ExecuteBeadResult{
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseProviderFailed),
				FizeauStage:   string(agentlib.SessionStageProvider),
			},
			wantAction: failclass.AttemptPolicyActionPark,
			wantReason: "fizeau_terminal_retryable",
		},
		{
			name: "internal_error_parks",
			fizeau: ExecuteBeadResult{
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseInternalError),
				FizeauStage:   string(agentlib.SessionStageCleanup),
			},
			wantAction: failclass.AttemptPolicyActionPark,
			wantReason: "fizeau_terminal_permanent_failure",
		},
	}

	// Status/Error variants that would classify to different (or no)
	// failure modes under ClassifyFailureMode if the typed adapter path
	// ever fell back to scanning them.
	statusErrorVariants := []struct {
		name   string
		status string
		errMsg string
	}{
		{name: "empty", status: "", errMsg: ""},
		{name: "success_status_with_no_error", status: ExecuteBeadStatusSuccess, errMsg: ""},
		{name: "timeout_looking_text", status: ExecuteBeadStatusExecutionFailed, errMsg: "context deadline exceeded"},
		{name: "merge_conflict_looking_text", status: ExecuteBeadStatusLandConflict, errMsg: "merge conflict"},
		{name: "auth_error_looking_text", status: ExecuteBeadStatusExecutionFailed, errMsg: "401 unauthorized: invalid api key"},
		{name: "no_changes_status", status: ExecuteBeadStatusNoChanges, errMsg: "unrelated diagnostic"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var baseline *failclass.AttemptPolicyDecision
			for _, variant := range statusErrorVariants {
				t.Run(variant.name, func(t *testing.T) {
					res := tc.fizeau
					res.Status = variant.status
					res.Error = variant.errMsg

					got := AttemptPolicyDecisionForResult(&res)
					if got.Action != tc.wantAction {
						t.Fatalf("AttemptPolicyDecisionForResult(%s/%s).Action = %q, want %q", tc.name, variant.name, got.Action, tc.wantAction)
					}
					if got.Reason != tc.wantReason {
						t.Fatalf("AttemptPolicyDecisionForResult(%s/%s).Reason = %q, want %q", tc.name, variant.name, got.Reason, tc.wantReason)
					}
					if baseline == nil {
						baseline = &failclass.AttemptPolicyDecision{Action: got.Action, Reason: got.Reason}
						return
					}
					if got.Action != baseline.Action || got.Reason != baseline.Reason {
						t.Fatalf("AttemptPolicyDecisionForResult(%s/%s) diverged from baseline: got %+v, want %+v", tc.name, variant.name, got, baseline)
					}
				})
			}
		})
	}
}

// TestLegacyCompatibilityPathStaysOutsideTypedAdapter verifies two things
// required by phase1 WB-1: (1) non-Fizeau results (no typed lifecycle tuple
// recorded) still classify through the legacy text-driven compatibility path
// (ClassifyFailureMode), and (2) the typed adapter path
// (failclass.AttemptPolicyDecision) carries no bead-state, review-policy,
// landing, or closure fields/side effects — it is a pure advisory decision,
// not an owner of those concerns.
func TestLegacyCompatibilityPathStaysOutsideTypedAdapter(t *testing.T) {
	t.Run("non_fizeau_result_uses_compatibility_path", func(t *testing.T) {
		cases := []struct {
			name    string
			outcome string
			exit    int
			errMsg  string
			want    string
		}{
			{
				name:    "legacy_test_failure_text",
				outcome: ExecuteBeadOutcomeTaskFailed,
				exit:    1,
				errMsg:  "--- FAIL: TestSomething",
				want:    FailureModeTestFailure,
			},
			{
				name:    "legacy_timeout_text",
				outcome: ExecuteBeadOutcomeTaskFailed,
				exit:    1,
				errMsg:  "context deadline exceeded",
				want:    FailureModeTimeout,
			},
			{
				name:    "legacy_no_changes_outcome",
				outcome: ExecuteBeadOutcomeTaskNoChanges,
				exit:    0,
				errMsg:  "",
				want:    FailureModeNoChanges,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				res := &ExecuteBeadResult{
					Outcome:  tc.outcome,
					ExitCode: tc.exit,
					Error:    tc.errMsg,
				}
				if res.FizeauOutcome != "" || res.FizeauCause != "" || res.FizeauStage != "" {
					t.Fatalf("test setup error: result unexpectedly carries a Fizeau lifecycle tuple")
				}
				got := ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)
				if got != tc.want {
					t.Fatalf("ClassifyFailureMode(%s) = %q, want %q", tc.name, got, tc.want)
				}
			})
		}
	})

	t.Run("typed_decision_carries_no_bead_state_review_landing_or_closure_fields", func(t *testing.T) {
		decisionType := reflect.TypeOf(failclass.AttemptPolicyDecision{})
		allowed := map[string]bool{"Action": true, "Reason": true, "Audit": true}
		for i := 0; i < decisionType.NumField(); i++ {
			name := decisionType.Field(i).Name
			if !allowed[name] {
				t.Fatalf("failclass.AttemptPolicyDecision has unexpected field %q; the typed adapter must stay a pure advisory decision, not an owner of bead state/review/landing/closure", name)
			}
		}
		for _, forbidden := range []string{
			"BeadID", "BeadStatus", "ReviewVerdict", "Landed", "LandingOutcome", "Closed", "Merged",
		} {
			if _, ok := decisionType.FieldByName(forbidden); ok {
				t.Fatalf("failclass.AttemptPolicyDecision unexpectedly exposes %q", forbidden)
			}
		}
	})

	t.Run("typed_adapter_call_does_not_mutate_the_result", func(t *testing.T) {
		res := &ExecuteBeadResult{
			FizeauOutcome: string(agentlib.SessionOutcomeSuccess),
			FizeauCause:   string(agentlib.TerminalCauseCompleted),
			FizeauStage:   string(agentlib.SessionStageHarness),
			Status:        ExecuteBeadStatusExecutionFailed,
			Error:         "misleading pre-existing error text",
			Outcome:       ExecuteBeadOutcomeTaskFailed,
		}
		wantStatus, wantError, wantOutcome := res.Status, res.Error, res.Outcome
		wantFizeauOutcome, wantFizeauCause, wantFizeauStage := res.FizeauOutcome, res.FizeauCause, res.FizeauStage

		_ = AttemptPolicyDecisionForResult(res)

		if res.Status != wantStatus || res.Error != wantError || res.Outcome != wantOutcome ||
			res.FizeauOutcome != wantFizeauOutcome || res.FizeauCause != wantFizeauCause || res.FizeauStage != wantFizeauStage {
			t.Fatalf("AttemptPolicyDecisionForResult mutated the result: got %+v", res)
		}
	})
}
