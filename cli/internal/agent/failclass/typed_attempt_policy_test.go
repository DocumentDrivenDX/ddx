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
	if got, want := inputType.NumField(), 3; got != want {
		t.Fatalf("AttemptPolicyInput has %d fields, want %d", got, want)
	}
	for _, fieldName := range []string{"Final", "ImmediateErr", "Evidence"} {
		if _, ok := inputType.FieldByName(fieldName); !ok {
			t.Fatalf("AttemptPolicyInput missing typed field %q", fieldName)
		}
	}
	for _, fieldName := range []string{"ProviderText", "ErrorText", "Stderr", "RouteText"} {
		if _, ok := inputType.FieldByName(fieldName); ok {
			t.Fatalf("AttemptPolicyInput unexpectedly exposes text field %q", fieldName)
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
		})
	}
}

func TestDDXAttemptPolicyConsumesTypedFizeauResult(t *testing.T) {
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
		})
	}
}
