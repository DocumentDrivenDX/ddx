package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteBeadResult_NiflheimEvidence_ReviewDecisionAudit(t *testing.T) {
	t.Run("empty review result serializes audit in result JSON and execute event", func(t *testing.T) {
		report := ExecuteBeadReport{
			BeadID:              "ddx-empty-review",
			AttemptID:           "attempt-empty-review",
			Harness:             "codex",
			Provider:            "openai",
			Model:               "gpt-5",
			ActualPower:         80,
			Status:              ExecuteBeadStatusReviewMalfunction,
			Detail:              "pre-close review: empty review result before close",
			ResultRev:           "badc0ffe",
			RequestedProfile:    "smart",
			RoutingIntentSource: "profile",
			EstimatedDifficulty: "hard",
			InferredPowerClass:  "smart",
			EscalationCount:     2,
		}
		trace := executionCycleTraceFor(
			CandidateResult{Report: report, CycleIndex: 0},
			&CandidateReviewResult{
				ReviewGroupID:    "rg-empty-review",
				ReviewerIndices:  []int{0},
				ReviewerVerdicts: []string{"APPROVE"},
			},
			report.Status,
		)
		report.CycleTrace = []ExecutionCycleTrace{trace}

		raw, err := json.Marshal(ExecuteBeadResult{
			BeadID:     report.BeadID,
			AttemptID:  report.AttemptID,
			Status:     report.Status,
			ResultRev:  report.ResultRev,
			CycleTrace: report.CycleTrace,
		})
		require.NoError(t, err)
		var resultJSON map[string]any
		require.NoError(t, json.Unmarshal(raw, &resultJSON))
		cycle := firstCycleTrace(t, resultJSON)
		assert.Equal(t, ExecuteBeadStatusReviewMalfunction, cycle["failure_class"])
		assert.Equal(t, "retry", cycle["retry_action"])
		assert.Equal(t, float64(2), cycle["escalation_count"])
		assert.Equal(t, "malformed", cycle["review_status"])
		assert.Equal(t, ReviewFindingClassMalfunction, cycle["review_classification"])
		assert.Equal(t, "not_landed", cycle["land_status"])
		assert.Equal(t, "not_applicable", cycle["reconcile_status"])
		assert.Equal(t, "smart", nestedString(t, cycle, "requested_route", "profile"))
		assert.Equal(t, "gpt-5", nestedString(t, cycle, "actual_route", "model"))

		eventAudit := decisionAuditFromEventBody(t, executeBeadLoopEvent(report, "worker", time.Unix(0, 0)).Body)
		assert.Equal(t, ExecuteBeadStatusReviewMalfunction, eventAudit["failure_class"])
		assert.Equal(t, "retry", eventAudit["retry_action"])
		assert.Equal(t, "malformed", eventAudit["review_status"])
		assert.Equal(t, ReviewFindingClassMalfunction, eventAudit["review_classification"])
		assert.Equal(t, "smart", nestedString(t, eventAudit, "requested_route", "requested_power_class"))
		assert.Equal(t, "openai", nestedString(t, eventAudit, "actual_route", "provider"))
	})

	t.Run("explicit review skip serializes durable reason in result JSON and execute event", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init(t.Context()))
		skipped := &bead.Bead{
			ID:     "ddx-review-skip",
			Title:  "Review skip",
			Labels: []string{"review:skip", "review:skip-reason:test-fixture"},
		}
		require.NoError(t, store.Create(t.Context(), skipped))
		reviewOut := RunPostMergeReview(t.Context(), PostMergeReviewInput{
			Bead: *skipped,
			Report: ExecuteBeadReport{
				BeadID:           "ddx-review-skip",
				AttemptID:        "attempt-review-skip",
				Harness:          "codex",
				Provider:         "openai",
				Model:            "gpt-5",
				ActualPower:      70,
				Status:           ExecuteBeadStatusSuccess,
				ResultRev:        "feedface",
				RequestedProfile: "standard",
				PowerClass:       "standard",
				EscalationCount:  1,
			},
			Store:    store,
			Assignee: "worker",
			Now:      time.Now,
		})
		require.True(t, reviewOut.Approved)
		report := reviewOut.Report
		require.Equal(t, ExecuteBeadReport{
			BeadID:           "ddx-review-skip",
			AttemptID:        "attempt-review-skip",
			Harness:          "codex",
			Provider:         "openai",
			Model:            "gpt-5",
			ActualPower:      70,
			Status:           ExecuteBeadStatusSuccess,
			ResultRev:        "feedface",
			RequestedProfile: "standard",
			PowerClass:       "standard",
			EscalationCount:  1,
			ReviewSkipReason: "review:skip-reason:test-fixture",
		}, report)
		trace := executionCycleTraceFor(CandidateResult{Report: report, CycleIndex: 0}, nil, report.Status)
		report.CycleTrace = []ExecutionCycleTrace{trace}

		raw, err := json.Marshal(ExecuteBeadResult{
			BeadID:     report.BeadID,
			AttemptID:  report.AttemptID,
			Status:     report.Status,
			ResultRev:  report.ResultRev,
			CycleTrace: report.CycleTrace,
		})
		require.NoError(t, err)
		var resultJSON map[string]any
		require.NoError(t, json.Unmarshal(raw, &resultJSON))
		cycle := firstCycleTrace(t, resultJSON)
		assert.Equal(t, "none", cycle["failure_class"])
		assert.Equal(t, "close", cycle["retry_action"])
		assert.Equal(t, float64(1), cycle["escalation_count"])
		assert.Equal(t, "skipped", cycle["review_status"])
		assert.Equal(t, "review:skip-reason:test-fixture", cycle["review_skip_reason"])
		assert.Equal(t, "review_skipped", cycle["review_classification"])
		assert.Equal(t, "standard", nestedString(t, cycle, "requested_route", "profile"))
		assert.Equal(t, "gpt-5", nestedString(t, cycle, "actual_route", "model"))

		eventAudit := decisionAuditFromEventBody(t, executeBeadLoopEvent(report, "worker", time.Unix(0, 0)).Body)
		assert.Equal(t, "none", eventAudit["failure_class"])
		assert.Equal(t, "close", eventAudit["retry_action"])
		assert.Equal(t, "skipped", eventAudit["review_status"])
		assert.Equal(t, "review:skip-reason:test-fixture", eventAudit["review_skip_reason"])
		assert.Equal(t, "review_skipped", eventAudit["review_classification"])
		assert.Equal(t, "standard", nestedString(t, eventAudit, "requested_route", "requested_power_class"))
		assert.Equal(t, "openai", nestedString(t, eventAudit, "actual_route", "provider"))
	})

	t.Run("land failure and reconcile events carry land audit", func(t *testing.T) {
		landRetry := ExecuteBeadReport{
			BeadID:        "ddx-land-retry",
			AttemptID:     "attempt-land-retry",
			Status:        ExecuteBeadStatusLandRetry,
			Detail:        "land coordination retry: staged generated evidence",
			BaseRev:       "base",
			ResultRev:     "result",
			OutcomeReason: FailureModeLandRetry,
		}
		retryAudit := decisionAuditFromEventBody(t, executeBeadLoopEvent(landRetry, "worker", time.Unix(0, 0)).Body)
		assert.Equal(t, FailureModeLandRetry, retryAudit["failure_class"])
		assert.Equal(t, "retry_land", retryAudit["retry_action"])
		assert.Equal(t, "retry", retryAudit["land_status"])
		assert.Equal(t, "pending", retryAudit["reconcile_status"])

		reconciled := ExecuteBeadReport{
			BeadID:    "ddx-reconciled",
			AttemptID: "attempt-reconciled",
			Status:    ExecuteBeadStatusSuccess,
			Detail:    "land coordination reconciled: result already landed",
			BaseRev:   "base",
			ResultRev: "result",
		}
		reconcileAudit := decisionAuditFromEventBody(t, executeBeadLoopEvent(reconciled, "worker", time.Unix(0, 0)).Body)
		assert.Equal(t, "none", reconcileAudit["failure_class"])
		assert.Equal(t, "reconcile", reconcileAudit["retry_action"])
		assert.Equal(t, "reconciled", reconcileAudit["land_status"])
		assert.Equal(t, "reconciled", reconcileAudit["reconcile_status"])
	})

	t.Run("decomposed parent stale skip carries audit", func(t *testing.T) {
		var emitted map[string]any
		emitStaleCandidateSkip(func(_ string, payload map[string]any) {
			emitted = payload
		}, "ddx-niflheim-parent", &candidateSkipDecision{
			Reason: "decomposed",
			Detail: "bead is marked decomposed and execution-ineligible",
		}, &bead.Bead{
			ID:     "ddx-niflheim-parent",
			Labels: []string{"decomposed"},
			Extra:  map[string]any{bead.ExtraExecutionElig: false},
		}, "before_claim")

		require.NotNil(t, emitted)
		assert.Equal(t, "decomposed", emitted["failure_class"])
		assert.Equal(t, "operator_attention", emitted["retry_action"])
		assert.Equal(t, "skipped", emitted["review_status"])
		assert.Equal(t, "pre_claim", emitted["review_skip_reason"])
		assert.Equal(t, "not_started", emitted["land_status"])
		assert.Equal(t, "not_applicable", emitted["reconcile_status"])
	})

	t.Run("pre claim warning escalation carries audit in emitted and durable events", func(t *testing.T) {
		store, first, second := newExecuteLoopTestStore(t)
		state := &preClaimWarnRepeatState{}
		var emitted map[string]any
		emit := func(_ string, payload map[string]any) {
			emitted = payload
		}
		at := time.Unix(10, 0).UTC()
		assert.False(t, appendPreClaimIntakeWarning(store, emit, state, 2, first.ID, "worker", "system_unready", "shared readiness schema mismatch", at))
		assert.True(t, appendPreClaimIntakeWarning(store, emit, state, 2, second.ID, "worker", "system_unready", "shared readiness schema mismatch", at.Add(time.Second)))

		require.NotNil(t, emitted)
		assert.Equal(t, "preclaim_warn_repeated", emitted["failure_class"])
		assert.Equal(t, "operator_attention", emitted["retry_action"])
		assert.Equal(t, float64(2), numericAny(emitted["escalation_count"]))
		assert.Equal(t, "skipped", emitted["review_status"])
		assert.Equal(t, "pre_claim", emitted["review_skip_reason"])
		assert.Equal(t, "not_started", emitted["land_status"])

		events, err := store.Events(second.ID)
		require.NoError(t, err)
		var durable map[string]any
		for _, event := range events {
			if event.Kind != "operator_attention" {
				continue
			}
			require.NoError(t, json.Unmarshal([]byte(event.Body), &durable))
			break
		}
		require.NotNil(t, durable)
		assert.Equal(t, "preclaim_warn_repeated", durable["failure_class"])
		assert.Equal(t, "operator_attention", durable["retry_action"])
		assert.Equal(t, "pre_claim", durable["review_skip_reason"])
	})
}

func firstCycleTrace(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	rawTrace, ok := result["cycle_trace"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, rawTrace)
	cycle, ok := rawTrace[0].(map[string]any)
	require.True(t, ok)
	return cycle
}

func decisionAuditFromEventBody(t *testing.T, body string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		raw, ok := strings.CutPrefix(line, "decision_audit=")
		if !ok {
			continue
		}
		var audit map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &audit))
		return audit
	}
	t.Fatalf("decision_audit line missing from event body:\n%s", body)
	return nil
}

func nestedString(t *testing.T, parent map[string]any, objectKey, valueKey string) string {
	t.Helper()
	obj, ok := parent[objectKey].(map[string]any)
	require.True(t, ok, "%s must be an object", objectKey)
	value, _ := obj[valueKey].(string)
	return value
}

func numericAny(value any) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case float64:
		return v
	default:
		return 0
	}
}
