package agent

import (
	"encoding/json"
	"reflect"
	"testing"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAttemptPolicyStoresSeparateLifecycleAndPolicyEvidence proves that
// ExecuteBeadReport keeps Fizeau lifecycle evidence (FizeauOutcome/
// FizeauCause/FizeauStage) and DDx-owned policy evidence (DDxOwnerStage) on
// distinct typed fields that vary independently, per WB-1's MET-003
// attribution requirement:
// docs/helix/06-iterate/phase1-lower-the-altitude-plan-2026-07-13.md:128-143.
func TestAttemptPolicyStoresSeparateLifecycleAndPolicyEvidence(t *testing.T) {
	reportType := reflect.TypeOf(ExecuteBeadReport{})
	for _, fieldName := range []string{"FizeauOutcome", "FizeauCause", "FizeauStage", "DDxOwnerStage"} {
		if _, ok := reportType.FieldByName(fieldName); !ok {
			t.Fatalf("ExecuteBeadReport missing typed field %q", fieldName)
		}
	}

	lifecycle := struct {
		outcome, cause, stage string
	}{
		outcome: string(agentlib.SessionOutcomeSuccess),
		cause:   string(agentlib.TerminalCauseCompleted),
		stage:   string(agentlib.SessionStageHarness),
	}

	// Same Fizeau lifecycle tuple, different DDx-owned status: the owner
	// stage must diverge while the lifecycle evidence stays identical.
	succeeded := &ExecuteBeadResult{
		BeadID:        "ddx-lifecycle-verify",
		Outcome:       ExecuteBeadOutcomeTaskSucceeded,
		FizeauOutcome: lifecycle.outcome,
		FizeauCause:   lifecycle.cause,
		FizeauStage:   lifecycle.stage,
	}
	populateWorkerStatus(succeeded)
	reportVerify := ReportFromExecuteBeadResult(succeeded, "standard")

	failed := &ExecuteBeadResult{
		BeadID:        "ddx-lifecycle-attempt",
		Outcome:       ExecuteBeadOutcomeTaskFailed,
		FizeauOutcome: lifecycle.outcome,
		FizeauCause:   lifecycle.cause,
		FizeauStage:   lifecycle.stage,
	}
	populateWorkerStatus(failed)
	reportAttempt := ReportFromExecuteBeadResult(failed, "standard")

	assert.Equal(t, reportVerify.FizeauOutcome, reportAttempt.FizeauOutcome, "lifecycle outcome must not be coupled to DDx status")
	assert.Equal(t, reportVerify.FizeauCause, reportAttempt.FizeauCause, "lifecycle cause must not be coupled to DDx status")
	assert.Equal(t, reportVerify.FizeauStage, reportAttempt.FizeauStage, "lifecycle stage must not be coupled to DDx status")
	assert.NotEqual(t, reportVerify.DDxOwnerStage, reportAttempt.DDxOwnerStage, "DDx owner stage must track DDx status independently of lifecycle evidence")
	assert.Equal(t, "verify", reportVerify.DDxOwnerStage)
	assert.Equal(t, "attempt", reportAttempt.DDxOwnerStage)

	// Same DDx-owned status, different Fizeau lifecycle tuple: the owner
	// stage must stay identical while the lifecycle evidence diverges.
	succeededOtherCause := &ExecuteBeadResult{
		BeadID:        "ddx-lifecycle-verify-2",
		Outcome:       ExecuteBeadOutcomeTaskSucceeded,
		FizeauOutcome: string(agentlib.SessionOutcomeFailed),
		FizeauCause:   string(agentlib.TerminalCauseProviderFailed),
		FizeauStage:   string(agentlib.SessionStageProvider),
	}
	populateWorkerStatus(succeededOtherCause)
	reportVerify2 := ReportFromExecuteBeadResult(succeededOtherCause, "standard")

	assert.Equal(t, reportVerify.DDxOwnerStage, reportVerify2.DDxOwnerStage, "DDx owner stage must not be derived from lifecycle evidence")
	assert.NotEqual(t, reportVerify.FizeauCause, reportVerify2.FizeauCause)
	assert.NotEqual(t, reportVerify.FizeauStage, reportVerify2.FizeauStage)
}

// TestAttemptPolicySchemaPreservesTypedEvidenceRoundTrip proves the
// persisted JSON schema for ExecuteBeadReport (the shape written to
// .ddx/attachments/*/events.jsonl and result.json) keeps Fizeau lifecycle
// evidence, the DDx owner-stage classification, and the legacy text-derived
// outcome reason on distinct keys that all survive a marshal/unmarshal round
// trip without collapsing into one another.
func TestAttemptPolicySchemaPreservesTypedEvidenceRoundTrip(t *testing.T) {
	report := ExecuteBeadReport{
		BeadID:        "ddx-schema-roundtrip",
		Status:        ExecuteBeadStatusLandRetry,
		FizeauOutcome: string(agentlib.SessionOutcomeFailed),
		FizeauCause:   string(agentlib.TerminalCauseProviderFailed),
		FizeauStage:   string(agentlib.SessionStageProvider),
		DDxOwnerStage: "land",
		OutcomeReason: FailureModeLandRetry,
	}

	body, err := json.Marshal(report)
	require.NoError(t, err)

	var asMap map[string]any
	require.NoError(t, json.Unmarshal(body, &asMap))

	wantKeys := map[string]string{
		"fizeau_outcome":  report.FizeauOutcome,
		"fizeau_cause":    report.FizeauCause,
		"fizeau_stage":    report.FizeauStage,
		"ddx_owner_stage": report.DDxOwnerStage,
		"outcome_reason":  report.OutcomeReason,
	}
	for key, want := range wantKeys {
		got, ok := asMap[key]
		if !ok {
			t.Fatalf("marshaled report missing schema key %q", key)
		}
		if got != want {
			t.Fatalf("marshaled report[%q] = %v, want %v", key, got, want)
		}
	}

	// Every distinct evidence source must decode back onto its own field,
	// none of them merged or aliased onto another.
	distinct := map[string]bool{}
	for _, v := range wantKeys {
		distinct[v] = true
	}
	if len(distinct) != len(wantKeys) {
		t.Fatalf("test fixture values are not distinct, cannot prove separation: %#v", wantKeys)
	}

	var decoded ExecuteBeadReport
	require.NoError(t, json.Unmarshal(body, &decoded))

	assert.Equal(t, report.FizeauOutcome, decoded.FizeauOutcome)
	assert.Equal(t, report.FizeauCause, decoded.FizeauCause)
	assert.Equal(t, report.FizeauStage, decoded.FizeauStage)
	assert.Equal(t, report.DDxOwnerStage, decoded.DDxOwnerStage)
	assert.Equal(t, report.OutcomeReason, decoded.OutcomeReason)
}
