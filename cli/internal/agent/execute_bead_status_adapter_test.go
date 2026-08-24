package agent

import (
	"testing"

	agentlib "github.com/easel/fizeau"

	"github.com/DocumentDrivenDX/ddx/internal/agent/failclass"
)

// TestDDXAttemptPolicyBuildsFromTypedLifecycleEvidence verifies
// BuildAttemptPolicyInput constructs the failclass adapter input from
// Fizeau's public lifecycle fields preserved on ExecuteBeadResult
// (FizeauOutcome/FizeauCause/FizeauStage) plus explicit DDx-owned evidence,
// and that the assembled input drives DecideAttemptPolicy to the expected
// decision.
func TestDDXAttemptPolicyBuildsFromTypedLifecycleEvidence(t *testing.T) {
	cases := []struct {
		name       string
		result     ExecuteBeadResult
		evidence   failclass.AttemptPolicyEvidence
		audit      failclass.AttemptPolicyAudit
		wantFinal  *agentlib.ServiceFinalData
		wantAction failclass.AttemptPolicyAction
	}{
		{
			name: "completed_lands_when_land_ready",
			result: ExecuteBeadResult{
				FizeauOutcome: string(agentlib.SessionOutcomeSuccess),
				FizeauCause:   string(agentlib.TerminalCauseCompleted),
				FizeauStage:   string(agentlib.SessionStageHarness),
			},
			evidence: failclass.AttemptPolicyEvidence{LandReady: true},
			audit:    failclass.AttemptPolicyAudit{Harness: "claude", Provider: "anthropic", Model: "claude-sonnet-4-5"},
			wantFinal: &agentlib.ServiceFinalData{
				Outcome: agentlib.SessionOutcomeSuccess,
				Cause:   agentlib.TerminalCauseCompleted,
				Stage:   agentlib.SessionStageHarness,
			},
			wantAction: failclass.AttemptPolicyActionLand,
		},
		{
			name: "provider_failed_retries_new_attempt",
			result: ExecuteBeadResult{
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseProviderFailed),
				FizeauStage:   string(agentlib.SessionStageProvider),
			},
			evidence: failclass.AttemptPolicyEvidence{NewAttemptRetryAllowed: true},
			wantFinal: &agentlib.ServiceFinalData{
				Outcome: agentlib.SessionOutcomeFailed,
				Cause:   agentlib.TerminalCauseProviderFailed,
				Stage:   agentlib.SessionStageProvider,
			},
			wantAction: failclass.AttemptPolicyActionNewAttemptRetry,
		},
		{
			name:       "no_lifecycle_tuple_recorded_yields_nil_final",
			result:     ExecuteBeadResult{},
			wantFinal:  nil,
			wantAction: failclass.AttemptPolicyActionPark,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildAttemptPolicyInput(&tc.result, tc.evidence, tc.audit)

			if tc.wantFinal == nil {
				if got.Final != nil {
					t.Fatalf("BuildAttemptPolicyInput(%s).Final = %#v, want nil", tc.name, got.Final)
				}
			} else {
				if got.Final == nil {
					t.Fatalf("BuildAttemptPolicyInput(%s).Final = nil, want %#v", tc.name, tc.wantFinal)
				}
				if got.Final.Outcome != tc.wantFinal.Outcome || got.Final.Cause != tc.wantFinal.Cause || got.Final.Stage != tc.wantFinal.Stage {
					t.Fatalf("BuildAttemptPolicyInput(%s).Final = %#v, want %#v", tc.name, got.Final, tc.wantFinal)
				}
			}
			if got.Evidence != tc.evidence {
				t.Fatalf("BuildAttemptPolicyInput(%s).Evidence = %#v, want %#v", tc.name, got.Evidence, tc.evidence)
			}
			if got.Audit != tc.audit {
				t.Fatalf("BuildAttemptPolicyInput(%s).Audit = %#v, want %#v", tc.name, got.Audit, tc.audit)
			}

			decision := failclass.DecideAttemptPolicy(got)
			if decision.Action != tc.wantAction {
				t.Fatalf("DecideAttemptPolicy(BuildAttemptPolicyInput(%s)).Action = %q, want %q", tc.name, decision.Action, tc.wantAction)
			}
		})
	}
}

// TestAttemptPolicyInputCarriesDDXOwnedEvidence verifies the DDx-owned
// evidence and audit passed into BuildAttemptPolicyInput survive the handoff
// exactly once, unchanged, and are never re-derived from res.Status or
// res.Error text. Each case sets a misleading Status/Error on the result —
// text that would classify to a different failure mode/evidence under
// ClassifyFailureMode — to prove the adapter input construction does not
// scan it.
func TestAttemptPolicyInputCarriesDDXOwnedEvidence(t *testing.T) {
	cases := []struct {
		name     string
		result   ExecuteBeadResult
		evidence failclass.AttemptPolicyEvidence
		audit    failclass.AttemptPolicyAudit
	}{
		{
			name: "land_ready_survives_misleading_error_text",
			result: ExecuteBeadResult{
				FizeauOutcome: string(agentlib.SessionOutcomeSuccess),
				FizeauCause:   string(agentlib.TerminalCauseCompleted),
				FizeauStage:   string(agentlib.SessionStageHarness),
				Status:        ExecuteBeadStatusExecutionFailed,
				Error:         "test failed: --- FAIL: TestSomething (context deadline exceeded)",
			},
			evidence: failclass.AttemptPolicyEvidence{LandReady: true},
			audit:    failclass.AttemptPolicyAudit{Harness: "claude", Provider: "anthropic", Model: "claude-sonnet-4-5", Route: "route-a"},
		},
		{
			name: "current_attempt_repairable_survives_misleading_status",
			result: ExecuteBeadResult{
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseProviderFailed),
				FizeauStage:   string(agentlib.SessionStageProvider),
				Status:        ExecuteBeadStatusSuccess,
				Error:         "",
			},
			evidence: failclass.AttemptPolicyEvidence{CurrentAttemptRepairable: true},
			audit:    failclass.AttemptPolicyAudit{Harness: "codex", Provider: "openai", Model: "gpt-5"},
		},
		{
			name: "request_minimum_strength_survives_empty_evidence_fields_untouched",
			result: ExecuteBeadResult{
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseInternalError),
				FizeauStage:   string(agentlib.SessionStageCleanup),
				Status:        ExecuteBeadStatusNoChanges,
				Error:         "agent power unsatisfied: no model meets min_power",
			},
			evidence: failclass.AttemptPolicyEvidence{RequestMinimumStrength: true},
			audit:    failclass.AttemptPolicyAudit{Harness: "claude", Provider: "anthropic", Model: "claude-sonnet-4-5"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.evidence

			got := BuildAttemptPolicyInput(&tc.result, tc.evidence, tc.audit)

			if got.Evidence != before {
				t.Fatalf("BuildAttemptPolicyInput(%s).Evidence = %#v, want unchanged %#v", tc.name, got.Evidence, before)
			}
			if got.Audit != tc.audit {
				t.Fatalf("BuildAttemptPolicyInput(%s).Audit = %#v, want unchanged %#v", tc.name, got.Audit, tc.audit)
			}

			// Re-deriving evidence from res.Status/res.Error via the legacy
			// ClassifyFailureMode path must not have been consulted: changing
			// Status/Error while holding the same explicit evidence produces
			// an identical adapter Evidence/Audit.
			mutated := tc.result
			mutated.Status = ExecuteBeadStatusResourceExhausted
			mutated.Error = "unrelated diagnostic text that must not affect evidence"
			again := BuildAttemptPolicyInput(&mutated, tc.evidence, tc.audit)
			if again.Evidence != got.Evidence {
				t.Fatalf("BuildAttemptPolicyInput(%s) evidence changed with status/error text: got %#v, want %#v", tc.name, again.Evidence, got.Evidence)
			}
			if again.Audit != got.Audit {
				t.Fatalf("BuildAttemptPolicyInput(%s) audit changed with status/error text: got %#v, want %#v", tc.name, again.Audit, got.Audit)
			}
		})
	}
}
