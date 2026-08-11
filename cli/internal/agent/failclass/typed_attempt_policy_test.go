package failclass

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
)

func TestAttemptPolicyConsumesTypedFizeauResult(t *testing.T) {
	inputType := reflect.TypeOf(AttemptPolicyInput{})
	if got, want := inputType.NumField(), 4; got != want {
		t.Fatalf("AttemptPolicyInput has %d fields, want %d", got, want)
	}
	for _, fieldName := range []string{"Final", "ImmediateErr", "Evidence", "Audit"} {
		if _, ok := inputType.FieldByName(fieldName); !ok {
			t.Fatalf("AttemptPolicyInput missing typed field %q", fieldName)
		}
	}
	for _, fieldName := range []string{"ProviderText", "ErrorText", "Stderr", "RouteText"} {
		if _, ok := inputType.FieldByName(fieldName); ok {
			t.Fatalf("AttemptPolicyInput unexpectedly exposes text field %q", fieldName)
		}
	}
	decisionType := reflect.TypeOf(AttemptPolicyDecision{})
	if got, want := decisionType.NumField(), 3; got != want {
		t.Fatalf("AttemptPolicyDecision has %d fields, want %d", got, want)
	}
	for _, fieldName := range []string{"Action", "Reason", "Audit"} {
		if _, ok := decisionType.FieldByName(fieldName); !ok {
			t.Fatalf("AttemptPolicyDecision missing typed field %q", fieldName)
		}
	}

	cases := []struct {
		name       string
		input      AttemptPolicyInput
		wantAct    AttemptPolicyAction
		wantReason string
	}{
		{
			name: "completed_land",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "success",
					Outcome: agentlib.SessionOutcomeSuccess,
					Cause:   agentlib.TerminalCauseCompleted,
					Stage:   agentlib.SessionStageHarness,
				},
				Evidence: AttemptPolicyEvidence{LandReady: true},
				Audit: AttemptPolicyAudit{
					Harness:  "codex",
					Provider: "openai",
					Model:    "gpt-5",
					Route:    "route-a",
				},
			},
			wantAct:    AttemptPolicyActionLand,
			wantReason: "fizeau_outcome_success",
		},
		{
			name: "retryable_current_attempt_repair",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeFailed,
					Cause:   agentlib.TerminalCauseProviderFailed,
					Stage:   agentlib.SessionStageProvider,
				},
				Evidence: AttemptPolicyEvidence{CurrentAttemptRepairable: true},
				Audit: AttemptPolicyAudit{
					Harness:  "codex",
					Provider: "openai",
					Model:    "gpt-5",
					Route:    "route-b",
				},
			},
			wantAct:    AttemptPolicyActionCurrentAttemptRepair,
			wantReason: "fizeau_terminal_retryable",
		},
		{
			name: "unavailable_now_parks",
			input: AttemptPolicyInput{
				ImmediateErr: fmt.Errorf("provider text that should be ignored: %w", &agentlib.NoViableProviderForNow{
					RetryAfter: time.Unix(1_700_000_000, 0),
				}),
			},
			wantAct:    AttemptPolicyActionPark,
			wantReason: "fizeau_immediate_unavailable_now",
		},
		{
			name: "cancelled_parks",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeCancelled,
					Cause:   agentlib.TerminalCauseContextCancelled,
					Stage:   agentlib.SessionStageCleanup,
				},
			},
			wantAct:    AttemptPolicyActionPark,
			wantReason: "fizeau_outcome_cancelled",
		},
		{
			name: "permanent_failure_escalates",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeFailed,
					Cause:   agentlib.TerminalCauseInternalError,
					Stage:   agentlib.SessionStageCleanup,
				},
				Evidence: AttemptPolicyEvidence{RequestMinimumStrength: true},
				Audit: AttemptPolicyAudit{
					Harness:  "claude",
					Provider: "anthropic",
					Model:    "claude-sonnet-4-5",
					Route:    "route-c",
				},
			},
			wantAct:    AttemptPolicyActionMinimumStrengthEscalation,
			wantReason: "fizeau_terminal_permanent_failure",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DecideAttemptPolicy(tc.input)
			if got.Action != tc.wantAct {
				t.Fatalf("DecideAttemptPolicy(%s).Action = %q, want %q", tc.name, got.Action, tc.wantAct)
			}
			if got.Reason != tc.wantReason {
				t.Fatalf("DecideAttemptPolicy(%s).Reason = %q, want %q", tc.name, got.Reason, tc.wantReason)
			}
			if got.Audit != tc.input.Audit {
				t.Fatalf("DecideAttemptPolicy(%s).Audit = %#v, want %#v", tc.name, got.Audit, tc.input.Audit)
			}
		})
	}
}

func TestAttemptPolicyKeepsFizeauAndDDxStagesSeparate(t *testing.T) {
	final := &agentlib.ServiceFinalData{
		Status:  "failed",
		Outcome: agentlib.SessionOutcomeFailed,
		Cause:   agentlib.TerminalCauseProviderFailed,
		Stage:   agentlib.SessionStageProvider,
	}
	input := AttemptPolicyInput{
		Final: final,
		Evidence: AttemptPolicyEvidence{
			CurrentAttemptRepairable: true,
		},
		Audit: AttemptPolicyAudit{
			Harness:  "codex",
			Provider: "openai",
			Model:    "gpt-5",
			Route:    "route-separate",
		},
	}

	got := DecideAttemptPolicy(input)
	if got.Action != AttemptPolicyActionCurrentAttemptRepair {
		t.Fatalf("DecideAttemptPolicy(input).Action = %q, want %q", got.Action, AttemptPolicyActionCurrentAttemptRepair)
	}
	if got.Reason != "fizeau_terminal_retryable" {
		t.Fatalf("DecideAttemptPolicy(input).Reason = %q, want %q", got.Reason, "fizeau_terminal_retryable")
	}
	if got.Audit != input.Audit {
		t.Fatalf("DecideAttemptPolicy(input).Audit = %#v, want %#v", got.Audit, input.Audit)
	}
	if final.Cause != agentlib.TerminalCauseProviderFailed {
		t.Fatalf("final cause mutated: got %q, want %q", final.Cause, agentlib.TerminalCauseProviderFailed)
	}
	if final.Stage != agentlib.SessionStageProvider {
		t.Fatalf("final stage mutated: got %q, want %q", final.Stage, agentlib.SessionStageProvider)
	}
	if !input.Evidence.CurrentAttemptRepairable {
		t.Fatalf("DDx evidence lost current-attempt repairability")
	}
}

func TestAttemptPolicyStageEvidenceSurvivesDecisionSelection(t *testing.T) {
	cases := []struct {
		name       string
		input      AttemptPolicyInput
		wantAct    AttemptPolicyAction
		wantReason string
	}{
		{
			name: "completed",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "success",
					Outcome: agentlib.SessionOutcomeSuccess,
					Cause:   agentlib.TerminalCauseCompleted,
					Stage:   agentlib.SessionStageHarness,
				},
				Evidence: AttemptPolicyEvidence{
					LandReady: true,
				},
				Audit: AttemptPolicyAudit{
					Harness:  "codex",
					Provider: "openai",
					Model:    "gpt-5",
					Route:    "route-completed",
				},
			},
			wantAct:    AttemptPolicyActionLand,
			wantReason: "fizeau_outcome_success",
		},
		{
			name: "retryable",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeFailed,
					Cause:   agentlib.TerminalCauseProviderFailed,
					Stage:   agentlib.SessionStageProvider,
				},
				Evidence: AttemptPolicyEvidence{
					CurrentAttemptRepairable: true,
				},
				Audit: AttemptPolicyAudit{
					Harness:  "codex",
					Provider: "openai",
					Model:    "gpt-5",
					Route:    "route-retryable",
				},
			},
			wantAct:    AttemptPolicyActionCurrentAttemptRepair,
			wantReason: "fizeau_terminal_retryable",
		},
		{
			name: "unavailable-now",
			input: AttemptPolicyInput{
				ImmediateErr: &agentlib.NoViableProviderForNow{},
				Audit: AttemptPolicyAudit{
					Harness:  "claude",
					Provider: "anthropic",
					Model:    "claude-sonnet-4-5",
					Route:    "route-unavailable",
				},
			},
			wantAct:    AttemptPolicyActionPark,
			wantReason: "fizeau_immediate_unavailable_now",
		},
		{
			name: "cancelled",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeCancelled,
					Cause:   agentlib.TerminalCauseContextCancelled,
					Stage:   agentlib.SessionStageCleanup,
				},
				Audit: AttemptPolicyAudit{
					Harness:  "codex",
					Provider: "openai",
					Model:    "gpt-5",
					Route:    "route-cancelled",
				},
			},
			wantAct:    AttemptPolicyActionPark,
			wantReason: "fizeau_outcome_cancelled",
		},
		{
			name: "permanent_failure",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeFailed,
					Cause:   agentlib.TerminalCauseInternalError,
					Stage:   agentlib.SessionStageCleanup,
				},
				Evidence: AttemptPolicyEvidence{
					RequestMinimumStrength: true,
				},
				Audit: AttemptPolicyAudit{
					Harness:  "claude",
					Provider: "anthropic",
					Model:    "claude-sonnet-4-5",
					Route:    "route-permanent",
				},
			},
			wantAct:    AttemptPolicyActionMinimumStrengthEscalation,
			wantReason: "fizeau_terminal_permanent_failure",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var finalSnapshot *agentlib.ServiceFinalData
			if tc.input.Final != nil {
				snapshot := *tc.input.Final
				finalSnapshot = &snapshot
			}
			evidence := tc.input.Evidence

			got := DecideAttemptPolicy(tc.input)

			if got.Action != tc.wantAct {
				t.Fatalf("DecideAttemptPolicy(%s).Action = %q, want %q", tc.name, got.Action, tc.wantAct)
			}
			if got.Reason != tc.wantReason {
				t.Fatalf("DecideAttemptPolicy(%s).Reason = %q, want %q", tc.name, got.Reason, tc.wantReason)
			}
			if got.Audit != tc.input.Audit {
				t.Fatalf("DecideAttemptPolicy(%s).Audit = %#v, want %#v", tc.name, got.Audit, tc.input.Audit)
			}
			if finalSnapshot != nil {
				if tc.input.Final == nil {
					t.Fatalf("DecideAttemptPolicy(%s) cleared Final", tc.name)
				}
				if tc.input.Final.Status != finalSnapshot.Status || tc.input.Final.Outcome != finalSnapshot.Outcome || tc.input.Final.Cause != finalSnapshot.Cause || tc.input.Final.Stage != finalSnapshot.Stage {
					t.Fatalf("DecideAttemptPolicy(%s) mutated Final: got %#v, want %#v", tc.name, tc.input.Final, finalSnapshot)
				}
			}
			if tc.input.Evidence != evidence {
				t.Fatalf("DecideAttemptPolicy(%s) mutated Evidence: got %#v, want %#v", tc.name, tc.input.Evidence, evidence)
			}
		})
	}
}

func TestAttemptPolicyMET003AttributionUsesTypedEvidence(t *testing.T) {
	baseFinal := &agentlib.ServiceFinalData{
		Status:  "failed",
		Outcome: agentlib.SessionOutcomeFailed,
		Cause:   agentlib.TerminalCauseProviderFailed,
		Stage:   agentlib.SessionStageProvider,
	}

	first := AttemptPolicyInput{
		Final: baseFinal,
		Evidence: AttemptPolicyEvidence{
			CurrentAttemptRepairable: true,
			NewAttemptRetryAllowed:   true,
		},
		Audit: AttemptPolicyAudit{
			Harness:  "codex",
			Provider: "openai",
			Model:    "gpt-5",
			Route:    "route-a",
		},
	}
	second := AttemptPolicyInput{
		Final: baseFinal,
		Evidence: AttemptPolicyEvidence{
			CurrentAttemptRepairable: true,
			NewAttemptRetryAllowed:   true,
		},
		Audit: AttemptPolicyAudit{
			Harness:  "claude",
			Provider: "anthropic",
			Model:    "claude-sonnet-4-5",
			Route:    "route-b",
		},
	}

	gotFirst := DecideAttemptPolicy(first)
	gotSecond := DecideAttemptPolicy(second)

	if gotFirst.Action != AttemptPolicyActionCurrentAttemptRepair {
		t.Fatalf("DecideAttemptPolicy(first).Action = %q, want %q", gotFirst.Action, AttemptPolicyActionCurrentAttemptRepair)
	}
	if gotSecond.Action != AttemptPolicyActionCurrentAttemptRepair {
		t.Fatalf("DecideAttemptPolicy(second).Action = %q, want %q", gotSecond.Action, AttemptPolicyActionCurrentAttemptRepair)
	}
	if gotFirst.Reason != gotSecond.Reason {
		t.Fatalf("reasons differ: first=%q second=%q", gotFirst.Reason, gotSecond.Reason)
	}
	if gotFirst.Reason != "fizeau_terminal_retryable" {
		t.Fatalf("DecideAttemptPolicy(first).Reason = %q, want %q", gotFirst.Reason, "fizeau_terminal_retryable")
	}
	if gotFirst.Audit != first.Audit {
		t.Fatalf("DecideAttemptPolicy(first).Audit = %#v, want %#v", gotFirst.Audit, first.Audit)
	}
	if gotSecond.Audit != second.Audit {
		t.Fatalf("DecideAttemptPolicy(second).Audit = %#v, want %#v", gotSecond.Audit, second.Audit)
	}
	if baseFinal.Cause != agentlib.TerminalCauseProviderFailed {
		t.Fatalf("baseFinal cause mutated: got %q, want %q", baseFinal.Cause, agentlib.TerminalCauseProviderFailed)
	}
	if baseFinal.Stage != agentlib.SessionStageProvider {
		t.Fatalf("baseFinal stage mutated: got %q, want %q", baseFinal.Stage, agentlib.SessionStageProvider)
	}
}

func TestAttemptPolicyDecisionIsExhaustive(t *testing.T) {
	testAttemptPolicyDecisionIsExhaustive(t)
}

func testAttemptPolicyDecisionIsExhaustive(t *testing.T) {
	cases := []struct {
		name string
		in   AttemptPolicyInput
		want AttemptPolicyAction
	}{
		{
			name: "close",
			in: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "success",
					Outcome: agentlib.SessionOutcomeSuccess,
					Cause:   agentlib.TerminalCauseCompleted,
					Stage:   agentlib.SessionStageHarness,
				},
			},
			want: AttemptPolicyActionClose,
		},
		{
			name: "land",
			in: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "success",
					Outcome: agentlib.SessionOutcomeSuccess,
					Cause:   agentlib.TerminalCauseCompleted,
					Stage:   agentlib.SessionStageHarness,
				},
				Evidence: AttemptPolicyEvidence{LandReady: true},
			},
			want: AttemptPolicyActionLand,
		},
		{
			name: "current_attempt_repair",
			in: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeFailed,
					Cause:   agentlib.TerminalCauseProviderFailed,
					Stage:   agentlib.SessionStageProvider,
				},
				Evidence: AttemptPolicyEvidence{CurrentAttemptRepairable: true},
			},
			want: AttemptPolicyActionCurrentAttemptRepair,
		},
		{
			name: "new_attempt_retry",
			in: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeFailed,
					Cause:   agentlib.TerminalCauseProviderFailed,
					Stage:   agentlib.SessionStageProvider,
				},
				Evidence: AttemptPolicyEvidence{NewAttemptRetryAllowed: true},
			},
			want: AttemptPolicyActionNewAttemptRetry,
		},
		{
			name: "park",
			in: AttemptPolicyInput{
				ImmediateErr: &agentlib.NoViableProviderForNow{
					RetryAfter: time.Unix(1_700_000_000, 0),
				},
			},
			want: AttemptPolicyActionPark,
		},
		{
			name: "escalate",
			in: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeFailed,
					Cause:   agentlib.TerminalCauseIterationLimit,
					Stage:   agentlib.SessionStageCleanup,
				},
				Evidence: AttemptPolicyEvidence{RequestMinimumStrength: true},
			},
			want: AttemptPolicyActionMinimumStrengthEscalation,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DecideAttemptPolicy(tc.in)
			if got.Action != tc.want {
				t.Fatalf("DecideAttemptPolicy(%s).Action = %q, want %q", tc.name, got.Action, tc.want)
			}
			if got.Audit != tc.in.Audit {
				t.Fatalf("DecideAttemptPolicy(%s).Audit = %#v, want %#v", tc.name, got.Audit, tc.in.Audit)
			}
		})
	}
}

func TestDDXAttemptPolicyConsumesTypedFizeauResult(t *testing.T) {
	cases := []struct {
		name       string
		input      AttemptPolicyInput
		wantAction AttemptPolicyAction
		wantReason string
	}{
		{
			name: "completed_close",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "success",
					Outcome: agentlib.SessionOutcomeSuccess,
					Cause:   agentlib.TerminalCauseCompleted,
					Stage:   agentlib.SessionStageHarness,
				},
			},
			wantAction: AttemptPolicyActionClose,
			wantReason: "fizeau_outcome_success",
		},
		{
			name: "completed_land",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "success",
					Outcome: agentlib.SessionOutcomeSuccess,
					Cause:   agentlib.TerminalCauseCompleted,
					Stage:   agentlib.SessionStageHarness,
				},
				Evidence: AttemptPolicyEvidence{LandReady: true},
			},
			wantAction: AttemptPolicyActionLand,
			wantReason: "fizeau_outcome_success",
		},
		{
			name: "retryable_current_attempt_repair",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeFailed,
					Cause:   agentlib.TerminalCauseProviderFailed,
					Stage:   agentlib.SessionStageProvider,
				},
				Evidence: AttemptPolicyEvidence{CurrentAttemptRepairable: true},
			},
			wantAction: AttemptPolicyActionCurrentAttemptRepair,
			wantReason: "fizeau_terminal_retryable",
		},
		{
			name: "retryable_new_attempt_retry",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeFailed,
					Cause:   agentlib.TerminalCauseProviderFailed,
					Stage:   agentlib.SessionStageProvider,
				},
				Evidence: AttemptPolicyEvidence{NewAttemptRetryAllowed: true},
			},
			wantAction: AttemptPolicyActionNewAttemptRetry,
			wantReason: "fizeau_terminal_retryable",
		},
		{
			name: "unavailable_now_parks",
			input: AttemptPolicyInput{
				ImmediateErr: &agentlib.NoViableProviderForNow{
					RetryAfter: time.Unix(1_700_000_000, 0),
				},
			},
			wantAction: AttemptPolicyActionPark,
			wantReason: "fizeau_immediate_unavailable_now",
		},
		{
			name: "cancelled_parks",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeCancelled,
					Cause:   agentlib.TerminalCauseContextCancelled,
					Stage:   agentlib.SessionStageCleanup,
				},
			},
			wantAction: AttemptPolicyActionPark,
			wantReason: "fizeau_outcome_cancelled",
		},
		{
			name: "permanent_failure_requests_minimum_strength",
			input: AttemptPolicyInput{
				Final: &agentlib.ServiceFinalData{
					Status:  "failed",
					Outcome: agentlib.SessionOutcomeFailed,
					Cause:   agentlib.TerminalCauseInternalError,
					Stage:   agentlib.SessionStageCleanup,
				},
				Evidence: AttemptPolicyEvidence{RequestMinimumStrength: true},
			},
			wantAction: AttemptPolicyActionMinimumStrengthEscalation,
			wantReason: "fizeau_terminal_permanent_failure",
		},
	}

	seenActions := map[AttemptPolicyAction]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DecideAttemptPolicy(tc.input)
			if got.Action != tc.wantAction {
				t.Fatalf("DecideAttemptPolicy(%s).Action = %q, want %q", tc.name, got.Action, tc.wantAction)
			}
			if got.Reason != tc.wantReason {
				t.Fatalf("DecideAttemptPolicy(%s).Reason = %q, want %q", tc.name, got.Reason, tc.wantReason)
			}
			if got.Audit != tc.input.Audit {
				t.Fatalf("DecideAttemptPolicy(%s).Audit = %#v, want %#v", tc.name, got.Audit, tc.input.Audit)
			}
			seenActions[got.Action] = true
		})
	}

	wantActions := []AttemptPolicyAction{
		AttemptPolicyActionCurrentAttemptRepair,
		AttemptPolicyActionNewAttemptRetry,
		AttemptPolicyActionMinimumStrengthEscalation,
		AttemptPolicyActionPark,
		AttemptPolicyActionLand,
		AttemptPolicyActionClose,
	}
	for _, want := range wantActions {
		if !seenActions[want] {
			t.Fatalf("exhaustiveness gap: decision %q was not observed", want)
		}
	}
}

func TestProviderIdentityDoesNotAffectPolicy(t *testing.T) {
	baseFinal := &agentlib.ServiceFinalData{
		Status:  "failed",
		Outcome: agentlib.SessionOutcomeFailed,
		Cause:   agentlib.TerminalCauseProviderFailed,
		Stage:   agentlib.SessionStageProvider,
	}

	first := AttemptPolicyInput{
		Final: baseFinal,
		Evidence: AttemptPolicyEvidence{
			NewAttemptRetryAllowed: true,
		},
		Audit: AttemptPolicyAudit{
			Harness:  "codex",
			Provider: "openai",
			Model:    "gpt-5",
			Route:    "route-a",
		},
	}
	second := AttemptPolicyInput{
		Final: baseFinal,
		Evidence: AttemptPolicyEvidence{
			NewAttemptRetryAllowed: true,
		},
		Audit: AttemptPolicyAudit{
			Harness:  "claude",
			Provider: "anthropic",
			Model:    "claude-sonnet-4-5",
			Route:    "route-b",
		},
	}

	gotFirst := DecideAttemptPolicy(first)
	gotSecond := DecideAttemptPolicy(second)

	if gotFirst.Action != gotSecond.Action {
		t.Fatalf("actions differ: first=%q second=%q", gotFirst.Action, gotSecond.Action)
	}
	if gotFirst.Reason != gotSecond.Reason {
		t.Fatalf("reasons differ: first=%q second=%q", gotFirst.Reason, gotSecond.Reason)
	}
	if gotFirst.Action != AttemptPolicyActionNewAttemptRetry {
		t.Fatalf("unexpected action %q", gotFirst.Action)
	}
	if gotFirst.Audit != first.Audit {
		t.Fatalf("first audit = %#v, want %#v", gotFirst.Audit, first.Audit)
	}
	if gotSecond.Audit != second.Audit {
		t.Fatalf("second audit = %#v, want %#v", gotSecond.Audit, second.Audit)
	}
}

func TestAttemptPolicyKeepsProviderIdentityAsAuditEvidence(t *testing.T) {
	input := AttemptPolicyInput{
		Final: &agentlib.ServiceFinalData{
			Status:  "success",
			Outcome: agentlib.SessionOutcomeSuccess,
			Cause:   agentlib.TerminalCauseCompleted,
			Stage:   agentlib.SessionStageHarness,
		},
		Evidence: AttemptPolicyEvidence{
			LandReady: true,
		},
		Audit: AttemptPolicyAudit{
			Harness:  "codex",
			Provider: "openai",
			Model:    "gpt-5",
			Route:    "route-a",
		},
	}

	got := DecideAttemptPolicy(input)
	if got.Action != AttemptPolicyActionLand {
		t.Fatalf("DecideAttemptPolicy(input).Action = %q, want %q", got.Action, AttemptPolicyActionLand)
	}
	if got.Audit != input.Audit {
		t.Fatalf("DecideAttemptPolicy(input).Audit = %#v, want %#v", got.Audit, input.Audit)
	}
}

func TestAttemptPolicyIgnoresRouteAuditFieldsForDecision(t *testing.T) {
	baseFinal := &agentlib.ServiceFinalData{
		Status:  "failed",
		Outcome: agentlib.SessionOutcomeFailed,
		Cause:   agentlib.TerminalCauseProviderFailed,
		Stage:   agentlib.SessionStageProvider,
	}

	first := AttemptPolicyInput{
		Final: baseFinal,
		Evidence: AttemptPolicyEvidence{
			NewAttemptRetryAllowed: true,
		},
		Audit: AttemptPolicyAudit{
			Harness:  "codex",
			Provider: "openai",
			Model:    "gpt-5",
			Route:    "route-a",
		},
	}
	second := AttemptPolicyInput{
		Final: baseFinal,
		Evidence: AttemptPolicyEvidence{
			NewAttemptRetryAllowed: true,
		},
		Audit: AttemptPolicyAudit{
			Harness:  "codex",
			Provider: "openai",
			Model:    "gpt-5",
			Route:    "route-b",
		},
	}

	gotFirst := DecideAttemptPolicy(first)
	gotSecond := DecideAttemptPolicy(second)

	if gotFirst.Action != gotSecond.Action {
		t.Fatalf("actions differ: first=%q second=%q", gotFirst.Action, gotSecond.Action)
	}
	if gotFirst.Reason != gotSecond.Reason {
		t.Fatalf("reasons differ: first=%q second=%q", gotFirst.Reason, gotSecond.Reason)
	}
	if gotFirst.Action != AttemptPolicyActionNewAttemptRetry {
		t.Fatalf("unexpected action %q", gotFirst.Action)
	}
	if gotFirst.Audit != first.Audit {
		t.Fatalf("first audit = %#v, want %#v", gotFirst.Audit, first.Audit)
	}
	if gotSecond.Audit != second.Audit {
		t.Fatalf("second audit = %#v, want %#v", gotSecond.Audit, second.Audit)
	}
}

func TestImmediateFizeauFailureDoesNotParseProviderText(t *testing.T) {
	baseInput := AttemptPolicyInput{
		ImmediateErr: &agentlib.NoViableProviderForNow{
			RetryAfter: time.Unix(1_700_000_000, 0),
		},
		Audit: AttemptPolicyAudit{
			Harness:  "codex",
			Provider: "openai",
			Model:    "gpt-5",
			Route:    "route-a",
		},
	}
	want := DecideAttemptPolicy(baseInput)
	if want.Action != AttemptPolicyActionPark {
		t.Fatalf("DecideAttemptPolicy(baseInput).Action = %q, want %q", want.Action, AttemptPolicyActionPark)
	}
	if want.Reason != "fizeau_immediate_unavailable_now" {
		t.Fatalf("DecideAttemptPolicy(baseInput).Reason = %q, want %q", want.Reason, "fizeau_immediate_unavailable_now")
	}

	cases := []struct {
		name                string
		stderrText          string
		providerMessageText string
	}{
		{
			name:                "empty_text",
			stderrText:          "",
			providerMessageText: "",
		},
		{
			name:                "stderr_text_varied",
			stderrText:          "stderr: quota exhausted; retry later",
			providerMessageText: "",
		},
		{
			name:                "provider_message_text_varied",
			stderrText:          "",
			providerMessageText: "provider-message: route timed out after a successful connect",
		},
		{
			name:                "misleading_text",
			stderrText:          "stderr: this looks permanent, but it is not",
			providerMessageText: "provider-message: do not trust this human-readable explanation",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := noViableProviderForNowErr(tc.stderrText, tc.providerMessageText)

			got := DecideAttemptPolicy(AttemptPolicyInput{
				ImmediateErr: err,
				Audit: AttemptPolicyAudit{
					Harness:  "codex",
					Provider: "openai",
					Model:    "gpt-5",
					Route:    "route-a",
				},
			})

			if got.Action != want.Action {
				t.Fatalf("DecideAttemptPolicy(%s).Action = %q, want %q", tc.name, got.Action, want.Action)
			}
			if got.Reason != want.Reason {
				t.Fatalf("DecideAttemptPolicy(%s).Reason = %q, want %q", tc.name, got.Reason, want.Reason)
			}
			if got.Audit != baseInput.Audit {
				t.Fatalf("DecideAttemptPolicy(%s).Audit = %#v, want %#v", tc.name, got.Audit, baseInput.Audit)
			}
		})
	}
}

func noViableProviderForNowErr(stderrText, providerMessageText string) error {
	var err error = &agentlib.NoViableProviderForNow{
		RetryAfter: time.Unix(1_700_000_000, 0),
	}
	if providerMessageText != "" {
		err = fmt.Errorf("provider-message: %s: %w", providerMessageText, err)
	}
	if stderrText != "" {
		err = fmt.Errorf("stderr: %s: %w", stderrText, err)
	}
	return err
}

func TestAttemptPolicyRejectsAmbiguousTypedFizeauResult(t *testing.T) {
	input := AttemptPolicyInput{
		Final: &agentlib.ServiceFinalData{
			Status:  "success",
			Outcome: agentlib.SessionOutcomeSuccess,
			Cause:   agentlib.TerminalCauseCompleted,
			Stage:   agentlib.SessionStageHarness,
		},
		ImmediateErr: &agentlib.NoViableProviderForNow{
			RetryAfter: time.Unix(1_700_000_000, 0),
		},
	}

	got := DecideAttemptPolicy(input)
	if got.Action != AttemptPolicyActionPark {
		t.Fatalf("DecideAttemptPolicy(ambiguous).Action = %q, want %q", got.Action, AttemptPolicyActionPark)
	}
	if got.Reason != "fizeau_lifecycle_ambiguous" {
		t.Fatalf("DecideAttemptPolicy(ambiguous).Reason = %q, want %q", got.Reason, "fizeau_lifecycle_ambiguous")
	}
}
