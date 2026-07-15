package cmd

import (
	"context"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	policyescalation "github.com/DocumentDrivenDX/ddx/internal/escalation"
)

func isBudgetExhaustedFailure(report agent.ExecuteBeadReport) bool {
	return strings.Contains(report.Detail, agent.RateLimitBudgetExhaustedReason)
}

// normalizeProviderFailureReport classifies a failed attempt's free-text error
// into the typed provider-failure taxonomy (ddx-3b721804) and stamps the report
// with the typed outcome_reason plus worker-disruption markers. It covers both
// pre-dispatch Execute errors and failed final events, since by the time the
// report reaches the escalation loop both paths have populated Detail/Error.
//
// It only acts on execution_failed reports and only overrides an empty or
// coarse/provider-ish outcome_reason, so a semantic classification (test_failure,
// review_block, ...) is never clobbered. Returns the typed ProviderFailure and
// ok=true when it classified the report.
func normalizeProviderFailureReport(report *agent.ExecuteBeadReport) (agent.ProviderFailure, bool) {
	if report == nil || report.Status != agent.ExecuteBeadStatusExecutionFailed {
		return agent.ProviderFailure{}, false
	}
	if !isReplaceableProviderOutcomeReason(report.OutcomeReason) {
		return agent.ProviderFailure{}, false
	}
	combined := strings.Join([]string{report.Detail, report.Error, report.Stderr}, "\n")
	pf, ok := agent.ClassifyProviderFailure(combined)
	if !ok {
		return agent.ProviderFailure{}, false
	}
	agent.ApplyProviderFailureToReport(report, pf)
	return pf, true
}

// isReplaceableProviderOutcomeReason reports whether an existing outcome_reason
// is safe to refine into the typed provider taxonomy. Empty, the coarse
// pre-typed buckets, and the typed provider reasons themselves are replaceable;
// semantic acceptance/review reasons are not.
func isReplaceableProviderOutcomeReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "",
		"timeout",
		"transport",
		"quota",
		"routing",
		agent.FailureModeAuthError,
		agent.FailureModeServerUnavailable,
		agent.FailureModeHarnessNotInstalled,
		agent.FailureModeUnknown:
		return true
	default:
		return agent.IsProviderFailureReason(reason)
	}
}

// applyProviderFallbackEvidence records the durable route-health evidence for a
// typed provider failure and, for a pinned worker, reports hard-pin-exhausted
// without widening the pin (ddx-3b721804). For an unpinned worker the fallback
// decision drives whether the escalation loop continues; either way the
// evidence names the requested constraints, resolved route, typed failure,
// retryability, and the fallback decision.
func applyProviderFallbackEvidence(report *agent.ExecuteBeadReport, pf agent.ProviderFailure, pin agent.ProviderPin) {
	if report == nil {
		return
	}
	decision := agent.DecideProviderFallback(pf, pin.Any())
	var resolved *agent.ResolvedRoute
	if strings.TrimSpace(report.Provider) != "" || strings.TrimSpace(report.Model) != "" || strings.TrimSpace(report.Harness) != "" {
		resolved = &agent.ResolvedRoute{
			Harness:  report.Harness,
			Provider: report.Provider,
			Model:    report.Model,
		}
	}
	evidence := agent.BuildProviderFailureEvidence(agent.ProviderFailureRequest{
		Harness:  pin.Harness,
		Provider: pin.Provider,
		Model:    pin.Model,
		Profile:  firstNonEmpty(report.RequestedPolicy, report.RequestedProfile),
	}, resolved, pf, decision)
	report.ProviderFailureEvidence = &evidence
	if pin.Any() {
		agent.MarkHardPinExhausted(report, pin, pf)
	}
}

func runEscalatingPowerAttempts(
	ctx context.Context,
	initialMinPower int,
	maxPower int,
	attempt func(context.Context, int) (agent.ExecuteBeadReport, error),
	recordAttempt func(agent.ExecuteBeadReport),
	perBeadTracker *policyescalation.PerBeadCostTracker,
	allowInfrastructureRetry bool,
	pin agent.ProviderPin,
) (agent.ExecuteBeadReport, error) {
	minPower := initialMinPower
	for {
		report, err := attempt(ctx, minPower)
		if err == nil {
			// Normalize provider failures and attach route-health evidence
			// before recording/deciding, so recorders and the fallback decision
			// see the typed outcome (ddx-3b721804).
			if pf, classified := normalizeProviderFailureReport(&report); classified {
				applyProviderFallbackEvidence(&report, pf, pin)
			}
		}
		if recordAttempt != nil && report.BeadID != "" {
			recordAttempt(report)
		}
		if perBeadTracker != nil {
			perBeadTracker.Add(report.Harness, report.CostUSD)
		}
		if err != nil {
			return report, err
		}
		if perBeadTracker != nil {
			if detail, tripped := perBeadTracker.Tripped(); tripped {
				report.Status = agent.ExecuteBeadStatusExecutionFailed
				report.Detail = detail
				report.CostUSD = perBeadTracker.Spent()
				return report, nil
			}
		}
		transition := executeloop.DecideAttemptTransition(executeloop.AttemptTransitionInput{
			Status:                   report.Status,
			Detail:                   report.Detail,
			CurrentMinPower:          minPower,
			MaxPower:                 maxPower,
			ActualPower:              report.ActualPower,
			OutcomeReason:            report.OutcomeReason,
			Disrupted:                report.Disrupted,
			BudgetExhausted:          isBudgetExhaustedFailure(report),
			AllowInfrastructureRetry: allowInfrastructureRetry,
		})
		if transition.Action != executeloop.TryLoopActionRetryPower {
			return report, nil
		}
		minPower = transition.NextMinPower
	}
}
