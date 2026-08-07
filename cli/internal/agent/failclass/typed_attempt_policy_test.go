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
			if got.Audit != tc.in.Audit {
				t.Fatalf("DecideAttemptPolicy(%s).Audit = %#v, want %#v", tc.name, got.Audit, tc.in.Audit)
			}
		})
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

func TestAttemptPolicyDoesNotReadProviderText(t *testing.T) {
	first := AttemptPolicyInput{
		ImmediateErr: fmt.Errorf("provider detail that should be ignored: %w", &agentlib.NoViableProviderForNow{
			RetryAfter: time.Unix(1_700_000_000, 0),
		}),
		Audit: AttemptPolicyAudit{
			Harness:  "codex",
			Provider: "openai",
			Model:    "gpt-5",
			Route:    "route-a",
		},
	}
	second := AttemptPolicyInput{
		ImmediateErr: fmt.Errorf("stderr/detail text changed but the typed error is the same: %w", &agentlib.NoViableProviderForNow{
			RetryAfter: time.Unix(1_700_000_000, 0),
		}),
		Audit: AttemptPolicyAudit{
			Harness:  "codex",
			Provider: "openai",
			Model:    "gpt-5",
			Route:    "route-a",
		},
	}

	gotFirst := DecideAttemptPolicy(first)
	gotSecond := DecideAttemptPolicy(second)

	if gotFirst.Action != AttemptPolicyActionPark {
		t.Fatalf("DecideAttemptPolicy(first).Action = %q, want %q", gotFirst.Action, AttemptPolicyActionPark)
	}
	if gotSecond.Action != AttemptPolicyActionPark {
		t.Fatalf("DecideAttemptPolicy(second).Action = %q, want %q", gotSecond.Action, AttemptPolicyActionPark)
	}
	if gotFirst.Action != gotSecond.Action {
		t.Fatalf("actions differ: first=%q second=%q", gotFirst.Action, gotSecond.Action)
	}
	if gotFirst.Reason != gotSecond.Reason {
		t.Fatalf("reasons differ: first=%q second=%q", gotFirst.Reason, gotSecond.Reason)
	}
	if gotFirst.Reason != "fizeau_immediate_unavailable_now" {
		t.Fatalf("DecideAttemptPolicy(first).Reason = %q, want %q", gotFirst.Reason, "fizeau_immediate_unavailable_now")
	}
	if gotFirst.Audit != first.Audit {
		t.Fatalf("DecideAttemptPolicy(first).Audit = %#v, want %#v", gotFirst.Audit, first.Audit)
	}
	if gotSecond.Audit != second.Audit {
		t.Fatalf("DecideAttemptPolicy(second).Audit = %#v, want %#v", gotSecond.Audit, second.Audit)
	}
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
