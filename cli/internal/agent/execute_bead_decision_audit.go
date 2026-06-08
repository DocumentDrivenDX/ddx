package agent

import (
	"encoding/json"
	"strings"
)

type executionDecisionAudit struct {
	FailureClass     string                            `json:"failure_class"`
	RetryAction      string                            `json:"retry_action"`
	EscalationCount  int                               `json:"escalation_count"`
	RequestedRoute   ExecutionCycleRequestedRouteFacts `json:"requested_route"`
	ActualRoute      ExecutionCycleRouteFacts          `json:"actual_route"`
	ReviewStatus     string                            `json:"review_status"`
	ReviewVerdict    string                            `json:"review_verdict,omitempty"`
	ReviewSkipReason string                            `json:"review_skip_reason"`
	LandStatus       string                            `json:"land_status"`
	ReconcileStatus  string                            `json:"reconcile_status"`
}

func executionDecisionAuditForReport(report ExecuteBeadReport, finalDecision string, reviewPresent bool, reviewVerdict string) executionDecisionAudit {
	if finalDecision == "" {
		finalDecision = report.Status
	}
	reviewVerdict = strings.TrimSpace(firstNonEmpty(reviewVerdict, report.ReviewVerdict, report.FirstReviewVerdictFromTrace()))
	reviewStatus, reviewSkipReason := reviewAuditStatus(report, reviewPresent, reviewVerdict)
	return executionDecisionAudit{
		FailureClass:     failureClassForAudit(report, finalDecision),
		RetryAction:      retryActionForAudit(report, finalDecision),
		EscalationCount:  report.EscalationCount,
		RequestedRoute:   requestedRouteFactsForAudit(report),
		ActualRoute:      actualRouteFactsForAudit(report),
		ReviewStatus:     reviewStatus,
		ReviewVerdict:    reviewVerdict,
		ReviewSkipReason: reviewSkipReason,
		LandStatus:       landStatusForAudit(report, finalDecision),
		ReconcileStatus:  reconcileStatusForAudit(report),
	}
}

func applyDecisionAuditToTrace(entry *ExecutionCycleTrace, audit executionDecisionAudit) {
	if entry == nil {
		return
	}
	entry.FailureClass = audit.FailureClass
	entry.RetryAction = audit.RetryAction
	entry.EscalationCount = audit.EscalationCount
	entry.RequestedRoute = audit.RequestedRoute
	entry.ActualRoute = audit.ActualRoute
	entry.ReviewStatus = audit.ReviewStatus
	entry.ReviewSkipReason = audit.ReviewSkipReason
	entry.LandStatus = audit.LandStatus
	entry.ReconcileStatus = audit.ReconcileStatus
}

func requestedRouteFactsForAudit(report ExecuteBeadReport) ExecutionCycleRequestedRouteFacts {
	return ExecutionCycleRequestedRouteFacts{
		Harness:             report.Harness,
		Provider:            report.Provider,
		Model:               report.Model,
		Profile:             report.RequestedProfile,
		RoutingIntentSource: report.RoutingIntentSource,
		EstimatedDifficulty: report.EstimatedDifficulty,
		InferredPowerClass:  report.InferredPowerClass,
		RequestedPowerClass: firstNonEmpty(report.InferredPowerClass, report.PowerClass),
	}
}

func actualRouteFactsForAudit(report ExecuteBeadReport) ExecutionCycleRouteFacts {
	return ExecutionCycleRouteFacts{
		Harness:     report.Harness,
		Provider:    report.Provider,
		Model:       report.Model,
		ActualPower: report.ActualPower,
		RouteReason: report.RoutingIntentSource,
	}
}

func failureClassForAudit(report ExecuteBeadReport, finalDecision string) string {
	for _, candidate := range []string{report.OutcomeReason, report.DisruptionReason, report.Status, finalDecision} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == ExecuteBeadStatusSuccess || candidate == ExecuteBeadStatusAlreadySatisfied {
			continue
		}
		return candidate
	}
	return "none"
}

func retryActionForAudit(report ExecuteBeadReport, finalDecision string) string {
	status := firstNonEmpty(strings.TrimSpace(finalDecision), report.Status)
	if strings.Contains(strings.ToLower(report.Detail), "land coordination reconciled") {
		return "reconcile"
	}
	switch status {
	case ExecuteBeadStatusSuccess, ExecuteBeadStatusAlreadySatisfied:
		return "close"
	case ExecuteBeadStatusLandRetry:
		return "retry_land"
	case ExecuteBeadStatusReviewRequestChanges, ExecuteBeadStatusReviewMalfunction,
		ExecuteBeadStatusRepairCycleExhausted, ExecuteBeadStatusReviewFixableGap:
		return "retry"
	case ExecuteBeadStatusLandOperatorAttention, ExecuteBeadStatusLandConflictOperatorRequired,
		ExecuteBeadStatusReviewBlock, ExecuteBeadStatusReviewRequestClarification,
		ExecuteBeadStatusDeclinedNeedsDecomposition, ExecuteBeadStatusPreservedNeedsReview:
		return "operator_attention"
	case ExecuteBeadStatusNoChanges, ExecuteBeadStatusNoEvidenceProduced,
		ExecuteBeadStatusExecutionFailed, ExecuteBeadStatusPostRunCheckFailed,
		ExecuteBeadStatusLandConflict, ExecuteBeadStatusLandConflictUnresolvable:
		return "retry"
	default:
		if report.Disrupted || report.RetryAfter != "" {
			return "retry"
		}
		if strings.TrimSpace(status) == "" {
			return "unknown"
		}
		return "retry"
	}
}

func reviewAuditStatus(report ExecuteBeadReport, reviewPresent bool, reviewVerdict string) (string, string) {
	if reviewPresent || len(report.ReviewVerdictsFromTrace()) > 0 {
		if strings.TrimSpace(reviewVerdict) == "" {
			return "malformed", ""
		}
		return "completed", ""
	}
	if strings.TrimSpace(report.ReviewVerdict) != "" {
		return "completed", ""
	}
	if report.Status == ExecuteBeadStatusReviewMalfunction {
		return "malformed", ""
	}
	return "skipped", "review_not_configured"
}

func landStatusForAudit(report ExecuteBeadReport, finalDecision string) string {
	status := firstNonEmpty(strings.TrimSpace(finalDecision), report.Status)
	if strings.Contains(strings.ToLower(report.Detail), "land coordination reconciled") {
		return "reconciled"
	}
	switch status {
	case ExecuteBeadStatusSuccess, ExecuteBeadStatusAlreadySatisfied:
		if report.ResultRev == "" && report.LandedRev == "" {
			return "not_applicable"
		}
		return "landed"
	case ExecuteBeadStatusLandRetry:
		return "retry"
	case ExecuteBeadStatusLandOperatorAttention:
		return "operator_attention"
	case ExecuteBeadStatusLandConflict, ExecuteBeadStatusLandConflictUnresolvable, ExecuteBeadStatusLandConflictOperatorRequired:
		return "conflict"
	default:
		return "not_landed"
	}
}

func reconcileStatusForAudit(report ExecuteBeadReport) string {
	detail := strings.ToLower(report.Detail + "\n" + report.OutcomeReason)
	switch {
	case strings.Contains(detail, "land coordination reconciled"):
		return "reconciled"
	case report.Status == ExecuteBeadStatusLandRetry:
		return "pending"
	default:
		return "not_applicable"
	}
}

func preClaimDecisionAudit(reason string, escalationCount int) map[string]any {
	return map[string]any{
		"failure_class":      reason,
		"retry_action":       retryActionForSkipAudit(reason),
		"escalation_count":   escalationCount,
		"requested_route":    map[string]any{},
		"actual_route":       map[string]any{},
		"review_status":      "skipped",
		"review_skip_reason": "pre_claim",
		"land_status":        "not_started",
		"reconcile_status":   "not_applicable",
	}
}

func decisionAuditEventBodyLine(report ExecuteBeadReport) string {
	auditJSON, err := json.Marshal(executionDecisionAuditForReport(report, report.Status, false, ""))
	if err != nil {
		return ""
	}
	return "decision_audit=" + string(auditJSON)
}

func retryActionForSkipAudit(reason string) string {
	switch reason {
	case "closed":
		return "close"
	case "dependency_waiting":
		return "wait"
	case "retry_cooldown":
		return "retry"
	case "decomposed", "execution_ineligible", "operator_attention", "external_blocked",
		"preclaim_idle_escalation", "preclaim_warn_repeated":
		return "operator_attention"
	default:
		return "operator_attention"
	}
}

func (r ExecuteBeadReport) ReviewVerdictsFromTrace() []string {
	for _, trace := range r.CycleTrace {
		if len(trace.ReviewVerdicts) > 0 {
			return trace.ReviewVerdicts
		}
		if strings.TrimSpace(trace.ReviewResult.Verdict) != "" {
			return []string{strings.TrimSpace(trace.ReviewResult.Verdict)}
		}
	}
	return nil
}

func (r ExecuteBeadReport) FirstReviewVerdictFromTrace() string {
	for _, trace := range r.CycleTrace {
		if strings.TrimSpace(trace.ReviewResult.Verdict) != "" {
			return strings.TrimSpace(trace.ReviewResult.Verdict)
		}
	}
	return ""
}
