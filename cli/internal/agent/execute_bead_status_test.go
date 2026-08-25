package agent

import (
	"testing"

	agentlib "github.com/easel/fizeau"
)

// TestAttemptPolicyStageEvidenceDoesNotUseStderr proves
// ClassifyTypedAttemptFailure populates stage (FizeauStage, preserved
// verbatim) and owner (DDxOwnerStage) evidence from the typed Fizeau
// lifecycle tuple and DDx Status alone. Varying Stderr/Error/Detail across
// misleading provider-diagnostic text must never change the result: the
// function does not parse those fields.
func TestAttemptPolicyStageEvidenceDoesNotUseStderr(t *testing.T) {
	cases := []struct {
		name   string
		result ExecuteBeadResult
	}{
		{
			name: "provider_failed_retryable",
			result: ExecuteBeadResult{
				Status:        ExecuteBeadStatusExecutionFailed,
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseProviderFailed),
				FizeauStage:   string(agentlib.SessionStageProvider),
			},
		},
		{
			name: "harness_failed",
			result: ExecuteBeadResult{
				Status:        ExecuteBeadStatusExecutionFailed,
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseHarnessFailed),
				FizeauStage:   string(agentlib.SessionStageHarness),
			},
		},
		{
			name: "completed_success",
			result: ExecuteBeadResult{
				Status:        ExecuteBeadStatusSuccess,
				FizeauOutcome: string(agentlib.SessionOutcomeSuccess),
				FizeauCause:   string(agentlib.TerminalCauseCompleted),
				FizeauStage:   string(agentlib.SessionStageHarness),
			},
		},
	}

	// Stderr/Error/Detail variants that would classify to a different (or no)
	// failure mode under the legacy ClassifyFailureMode text scan if
	// ClassifyTypedAttemptFailure ever fell back to scanning them.
	textVariants := []struct {
		name   string
		stderr string
		errMsg string
		detail string
	}{
		{name: "empty"},
		{name: "timeout_looking_text", stderr: "context deadline exceeded", errMsg: "timed out"},
		{name: "merge_conflict_looking_text", stderr: "merge conflict", errMsg: "automatic merge failed"},
		{name: "auth_error_looking_text", stderr: "401 unauthorized: invalid api key", errMsg: "quota exceeded"},
		{name: "harness_unavailable_looking_text", stderr: "no assistant final event", errMsg: "transcript could not be read"},
		{name: "success_looking_text", detail: "merged cleanly", errMsg: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var baseline *TypedAttemptFailureEvidence
			for _, variant := range textVariants {
				t.Run(variant.name, func(t *testing.T) {
					res := tc.result
					res.Stderr = variant.stderr
					res.Error = variant.errMsg
					res.Detail = variant.detail

					got, ok := ClassifyTypedAttemptFailure(&res)
					if !ok {
						t.Fatalf("ClassifyTypedAttemptFailure(%s/%s) ok = false, want true", tc.name, variant.name)
					}

					wantOwnerStage := ddxOwnerStageForStatus(tc.result.Status)
					if got.OwnerStage != wantOwnerStage {
						t.Fatalf("ClassifyTypedAttemptFailure(%s/%s).OwnerStage = %q, want %q", tc.name, variant.name, got.OwnerStage, wantOwnerStage)
					}
					if res.FizeauStage != tc.result.FizeauStage {
						t.Fatalf("FizeauStage mutated by classification: got %q, want %q", res.FizeauStage, tc.result.FizeauStage)
					}

					if baseline == nil {
						baseline = &TypedAttemptFailureEvidence{
							FailureMode:   got.FailureMode,
							Retryable:     got.Retryable,
							OutcomeReason: got.OutcomeReason,
							OwnerStage:    got.OwnerStage,
						}
						return
					}
					if got != *baseline {
						t.Fatalf("ClassifyTypedAttemptFailure(%s/%s) diverged from baseline with varied stderr/error/detail text: got %+v, want %+v", tc.name, variant.name, got, *baseline)
					}
				})
			}
		})
	}
}

// TestAttemptPolicyTypedFailurePreservesProviderReason proves that
// provider_connectivity and provider_harness_unavailable, their
// retryability, and the outcome reason stamped by MarkResultExecutionError
// remain the typed adapter values rather than collapsing into the generic
// FailureModeUnknown ("unknown") bucket a misleading/unrecognized error
// message would otherwise produce under the legacy text-driven
// ClassifyFailureMode path.
func TestAttemptPolicyTypedFailurePreservesProviderReason(t *testing.T) {
	cases := []struct {
		name            string
		cause           agentlib.TerminalCause
		wantFailureMode string
		wantRetryable   bool
	}{
		{
			name:            "provider_failed_is_provider_connectivity",
			cause:           agentlib.TerminalCauseProviderFailed,
			wantFailureMode: FailureModeProviderConnectivity,
			wantRetryable:   true,
		},
		{
			name:            "harness_failed_is_provider_harness_unavailable",
			cause:           agentlib.TerminalCauseHarnessFailed,
			wantFailureMode: FailureModeProviderHarnessUnavailable,
			wantRetryable:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := &ExecuteBeadResult{
				BeadID:        "ddx-typed-failure",
				AttemptID:     "attempt-typed-failure",
				Status:        ExecuteBeadStatusExecutionFailed,
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(tc.cause),
				FizeauStage:   string(agentlib.SessionStageProvider),
			}

			evidence, ok := ClassifyTypedAttemptFailure(res)
			if !ok {
				t.Fatalf("ClassifyTypedAttemptFailure(%s) ok = false, want true", tc.name)
			}
			if evidence.FailureMode != tc.wantFailureMode {
				t.Fatalf("ClassifyTypedAttemptFailure(%s).FailureMode = %q, want %q", tc.name, evidence.FailureMode, tc.wantFailureMode)
			}
			if evidence.Retryable != tc.wantRetryable {
				t.Fatalf("ClassifyTypedAttemptFailure(%s).Retryable = %v, want %v", tc.name, evidence.Retryable, tc.wantRetryable)
			}
			if evidence.OutcomeReason != tc.wantFailureMode {
				t.Fatalf("ClassifyTypedAttemptFailure(%s).OutcomeReason = %q, want %q", tc.name, evidence.OutcomeReason, tc.wantFailureMode)
			}

			// MarkResultExecutionError receives an error whose text does not
			// match any legacy ClassifyFailureMode pattern; the legacy path
			// would classify it as FailureModeUnknown ("unknown"). The typed
			// tuple recorded above must still win.
			mutated := *res
			mutated.Error = ""
			MarkResultExecutionError(&mutated, &genericAttemptError{msg: "an unrecognized diagnostic that matches no legacy pattern"})

			if mutated.FailureMode != tc.wantFailureMode {
				t.Fatalf("MarkResultExecutionError(%s).FailureMode = %q, want typed %q (not a generic unknown/execution_failed bucket)", tc.name, mutated.FailureMode, tc.wantFailureMode)
			}
			if mutated.FailureMode == FailureModeUnknown || mutated.FailureMode == FailureModeUnknownProviderFailure {
				t.Fatalf("MarkResultExecutionError(%s).FailureMode = %q, collapsed into a generic bucket", tc.name, mutated.FailureMode)
			}

			report := ReportFromExecuteBeadResult(&mutated, "standard")
			if report.OutcomeReason != tc.wantFailureMode {
				t.Fatalf("ReportFromExecuteBeadResult(%s).OutcomeReason = %q, want typed %q", tc.name, report.OutcomeReason, tc.wantFailureMode)
			}
		})
	}
}

// genericAttemptError is a plain error with no typed wrapping (not a
// *ProviderFailureError), used to prove MarkResultExecutionError prefers
// typed Fizeau lifecycle evidence over its legacy text-scanning fallback.
type genericAttemptError struct{ msg string }

func (e *genericAttemptError) Error() string { return e.msg }
