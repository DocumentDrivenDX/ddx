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

func TestExecuteBeadResult_NiflheimEvidence_PreClaimDecisionAudit(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		detail string
	}{
		{
			name:   "timeout warning family",
			reason: "timeout",
			detail: "readiness check timed out after 45s (.ddx/harness-sessions/session-timeout.json)",
		},
		{
			name:   "system_unready warning family",
			reason: "system_unready",
			detail: "generated DDx artifact dirt at .ddx/harness-sessions/session-system-unready.json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, first, second := newExecuteLoopTestStore(t)
			state := &preClaimWarnRepeatState{}
			var emitted map[string]any
			emit := func(_ string, payload map[string]any) {
				emitted = payload
			}

			at := time.Unix(10, 0).UTC()
			threshold := 2
			assert.False(t, appendPreClaimIntakeWarning(store, emit, state, threshold, first.ID, "worker", tc.reason, tc.detail, at))
			assert.True(t, appendPreClaimIntakeWarning(store, emit, state, threshold, second.ID, "worker", tc.reason, tc.detail, at.Add(time.Second)))

			require.NotNil(t, emitted)
			assert.Equal(t, "preclaim_warn_repeated", emitted["reason"])
			assert.Equal(t, "preclaim_warn_repeated", emitted["failure_class"])
			assert.Equal(t, "operator_attention", emitted["retry_action"])
			assert.Equal(t, float64(threshold), numericAny(emitted["escalation_count"]))
			assert.NotEmpty(t, emitted["fingerprint"])
			assert.Equal(t, float64(threshold), numericAny(emitted["count"]))
			assert.Equal(t, tc.detail, emitted["example_detail"])

			distinctBeadIDs, ok := emitted["distinct_bead_ids"].([]string)
			require.True(t, ok, "distinct_bead_ids must remain a Go string slice before JSON encoding")
			require.Len(t, distinctBeadIDs, threshold)
			assert.Equal(t, first.ID, distinctBeadIDs[0])
			assert.Equal(t, second.ID, distinctBeadIDs[1])

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
			assert.Equal(t, "preclaim_warn_repeated", durable["reason"])
			assert.Equal(t, "preclaim_warn_repeated", durable["failure_class"])
			assert.Equal(t, "operator_attention", durable["retry_action"])
			assert.Equal(t, float64(threshold), durable["count"])
			assert.Equal(t, tc.detail, durable["example_detail"])

			report := ExecuteBeadReport{
				BeadID:              second.ID,
				AttemptID:           "attempt-" + tc.reason,
				Status:              ExecuteBeadStatusLandOperatorAttention,
				Detail:              tc.detail,
				ResultRev:           "result-" + tc.reason,
				Harness:             "codex",
				Provider:            "openai",
				Model:               "gpt-5",
				ActualPower:         72,
				RequestedProfile:    "smart",
				RoutingIntentSource: "readiness",
				EstimatedDifficulty: "hard",
				InferredPowerClass:  "smart",
				EscalationCount:     threshold,
				OutcomeReason:       "preclaim_warn_repeated",
			}
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
			assert.Equal(t, "preclaim_warn_repeated", cycle["failure_class"])
			assert.Equal(t, "operator_attention", cycle["retry_action"])
			assert.Equal(t, float64(threshold), cycle["escalation_count"])
			assert.Equal(t, "smart", nestedString(t, cycle, "requested_route", "profile"))
			assert.Equal(t, "readiness", nestedString(t, cycle, "requested_route", "routing_intent_source"))
			assert.Equal(t, "openai", nestedString(t, cycle, "actual_route", "provider"))
			assert.Equal(t, "gpt-5", nestedString(t, cycle, "actual_route", "model"))

			event := executeBeadLoopEvent(report, "worker", time.Unix(0, 0))
			audit := decisionAuditFromEventBody(t, event.Body)
			assert.Equal(t, "preclaim_warn_repeated", audit["failure_class"])
			assert.Equal(t, "operator_attention", audit["retry_action"])
			assert.Equal(t, float64(threshold), audit["escalation_count"])
			assert.Equal(t, "smart", nestedString(t, audit, "requested_route", "profile"))
			assert.Equal(t, "readiness", nestedString(t, audit, "requested_route", "routing_intent_source"))
			assert.Equal(t, "openai", nestedString(t, audit, "actual_route", "provider"))
			assert.Equal(t, "gpt-5", nestedString(t, audit, "actual_route", "model"))
			assert.Contains(t, event.Body, "outcome_reason=preclaim_warn_repeated")
			assert.Contains(t, event.Body, tc.detail)
		})
	}
}

// TestExecuteBeadResult_NiflheimEvidence_LandCoordinationDecisionAudit verifies
// that land coordination failure scenarios — ref-lock after already landed,
// staged generated DDx evidence, and staged operator files — produce correct
// failure_class, retry_action, escalation_count, routing facts, land_status,
// reconcile_status, and review_status in both result JSON cycle traces and
// worker event bodies (ddx-67cdc109).
func TestExecuteBeadResult_NiflheimEvidence_LandCoordinationDecisionAudit(t *testing.T) {
	cases := []struct {
		name             string
		report           ExecuteBeadReport
		wantFailClass    string
		wantRetryAction  string
		wantLandStatus   string
		wantReconcile    string
		wantReviewStatus string
	}{
		{
			name: "cannot_lock_ref_already_landed_reconciles",
			report: ExecuteBeadReport{
				BeadID:              "ddx-lock-ref-reconcile",
				AttemptID:           "attempt-lock-ref",
				Harness:             "codex",
				Provider:            "openai",
				Model:               "gpt-5",
				ActualPower:         80,
				RequestedProfile:    "smart",
				RoutingIntentSource: "profile",
				EstimatedDifficulty: "hard",
				InferredPowerClass:  "smart",
				EscalationCount:     1,
				Status:              ExecuteBeadStatusSuccess,
				Detail:              "land coordination reconciled: result already landed",
				BaseRev:             "base-sha",
				ResultRev:           "result-sha",
			},
			wantFailClass:    "none",
			wantRetryAction:  "reconcile",
			wantLandStatus:   "reconciled",
			wantReconcile:    "reconciled",
			wantReviewStatus: "skipped",
		},
		{
			name: "staged_generated_evidence_land_retry",
			report: ExecuteBeadReport{
				BeadID:              "ddx-staged-evidence-retry",
				AttemptID:           "attempt-staged-evidence",
				Harness:             "codex",
				Provider:            "openai",
				Model:               "gpt-5",
				ActualPower:         75,
				RequestedProfile:    "standard",
				RoutingIntentSource: "profile",
				EstimatedDifficulty: "medium",
				InferredPowerClass:  "standard",
				EscalationCount:     0,
				Status:              ExecuteBeadStatusLandRetry,
				Detail:              "land coordination retry: staged changes after waiting 2s\n\t.ddx/executions/attempt-staged-evidence/result.json",
				OutcomeReason:       FailureModeLandRetry,
				BaseRev:             "base-sha",
				ResultRev:           "result-sha",
			},
			wantFailClass:    FailureModeLandRetry,
			wantRetryAction:  "retry_land",
			wantLandStatus:   "retry",
			wantReconcile:    "pending",
			wantReviewStatus: "skipped",
		},
		{
			name: "staged_operator_files_operator_attention",
			report: ExecuteBeadReport{
				BeadID:              "ddx-staged-operator-attention",
				AttemptID:           "attempt-staged-operator",
				Harness:             "codex",
				Provider:            "openai",
				Model:               "gpt-5",
				ActualPower:         80,
				RequestedProfile:    "smart",
				RoutingIntentSource: "profile",
				EstimatedDifficulty: "hard",
				InferredPowerClass:  "smart",
				EscalationCount:     2,
				Status:              ExecuteBeadStatusLandOperatorAttention,
				Detail:              "land coordination operator attention: staged changes after waiting 2s\n\toperator-notes.md",
				OutcomeReason:       FailureModeLandOperatorAttention,
				BaseRev:             "base-sha",
				ResultRev:           "result-sha",
			},
			wantFailClass:    FailureModeLandOperatorAttention,
			wantRetryAction:  "operator_attention",
			wantLandStatus:   "operator_attention",
			wantReconcile:    "not_applicable",
			wantReviewStatus: "skipped",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trace := executionCycleTraceFor(
				CandidateResult{Report: tc.report, CycleIndex: 0},
				nil,
				tc.report.Status,
			)
			tc.report.CycleTrace = []ExecutionCycleTrace{trace}

			raw, err := json.Marshal(ExecuteBeadResult{
				BeadID:     tc.report.BeadID,
				AttemptID:  tc.report.AttemptID,
				Status:     tc.report.Status,
				ResultRev:  tc.report.ResultRev,
				CycleTrace: tc.report.CycleTrace,
			})
			require.NoError(t, err)
			var resultJSON map[string]any
			require.NoError(t, json.Unmarshal(raw, &resultJSON))
			cycle := firstCycleTrace(t, resultJSON)
			assert.Equal(t, tc.wantFailClass, cycle["failure_class"], "result JSON failure_class")
			assert.Equal(t, tc.wantRetryAction, cycle["retry_action"], "result JSON retry_action")
			assert.Equal(t, float64(tc.report.EscalationCount), cycle["escalation_count"], "result JSON escalation_count")
			assert.Equal(t, tc.wantLandStatus, cycle["land_status"], "result JSON land_status")
			assert.Equal(t, tc.wantReconcile, cycle["reconcile_status"], "result JSON reconcile_status")
			assert.Equal(t, tc.wantReviewStatus, cycle["review_status"], "result JSON review_status")
			assert.Equal(t, tc.report.RequestedProfile, nestedString(t, cycle, "requested_route", "profile"), "result JSON requested_route.profile")
			assert.Equal(t, tc.report.RoutingIntentSource, nestedString(t, cycle, "requested_route", "routing_intent_source"), "result JSON requested_route.routing_intent_source")
			assert.Equal(t, tc.report.Provider, nestedString(t, cycle, "actual_route", "provider"), "result JSON actual_route.provider")
			assert.Equal(t, tc.report.Model, nestedString(t, cycle, "actual_route", "model"), "result JSON actual_route.model")

			eventAudit := decisionAuditFromEventBody(t, executeBeadLoopEvent(tc.report, "worker", time.Unix(0, 0)).Body)
			assert.Equal(t, tc.wantFailClass, eventAudit["failure_class"], "event failure_class")
			assert.Equal(t, tc.wantRetryAction, eventAudit["retry_action"], "event retry_action")
			assert.Equal(t, float64(tc.report.EscalationCount), numericAny(eventAudit["escalation_count"]), "event escalation_count")
			assert.Equal(t, tc.wantLandStatus, eventAudit["land_status"], "event land_status")
			assert.Equal(t, tc.wantReconcile, eventAudit["reconcile_status"], "event reconcile_status")
			assert.Equal(t, tc.wantReviewStatus, eventAudit["review_status"], "event review_status")
			assert.Equal(t, tc.report.RequestedProfile, nestedString(t, eventAudit, "requested_route", "profile"), "event requested_route.profile")
			assert.Equal(t, tc.report.RoutingIntentSource, nestedString(t, eventAudit, "requested_route", "routing_intent_source"), "event requested_route.routing_intent_source")
			assert.Equal(t, tc.report.Provider, nestedString(t, eventAudit, "actual_route", "provider"), "event actual_route.provider")
			assert.Equal(t, tc.report.Model, nestedString(t, eventAudit, "actual_route", "model"), "event actual_route.model")
		})
	}
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
