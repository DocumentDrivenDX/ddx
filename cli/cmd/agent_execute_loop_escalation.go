package cmd

import (
	"context"
	"errors"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	tierescalation "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	policyescalation "github.com/DocumentDrivenDX/ddx/internal/escalation"
)

type escalationFloorFinder interface {
	Next(actualPower int) (int, error)
}

func isBudgetExhaustedFailure(report agent.ExecuteBeadReport) bool {
	return strings.Contains(report.Detail, agent.RateLimitBudgetExhaustedReason)
}

func nextEscalationFloor(l escalationFloorFinder, actualPower int) (int, error) {
	if l == nil {
		return 0, tierescalation.ErrLadderExhausted
	}
	floor := actualPower
	for {
		next, err := l.Next(floor)
		if err == nil {
			return next, nil
		}
		var nvp *tierescalation.NoViableProviderError
		if errors.As(err, &nvp) {
			floor = nvp.Floor
			continue
		}
		return 0, err
	}
}

// computeReviewFixableGapRepairMinPower returns the MinPower floor for a
// bounded repair attempt after a review_fixable_gap classification. It advances
// one rung above implActualPower on the escalation ladder, skipping
// no-viable-provider rungs. If the ladder is nil or all rungs above
// implActualPower are exhausted, the fallback is implActualPower+1 so a repair
// always requests strictly higher capability than the original attempt.
//
// The returned value is a pure MinPower floor — callers must preserve all other
// routing pins (harness, model, provider, profile) unchanged and must not
// introduce a MaxPower cap unless the operator has explicitly configured one.
func computeReviewFixableGapRepairMinPower(l escalationFloorFinder, implActualPower int) int {
	next, err := nextEscalationFloor(l, implActualPower)
	if err == nil {
		return next
	}
	return implActualPower + 1
}

func runEscalatingSingleTierAttempts(
	ctx context.Context,
	initialMinPower int,
	ladder escalationFloorFinder,
	attempt func(context.Context, int) (agent.ExecuteBeadReport, error),
	recordAttempt func(agent.ExecuteBeadReport),
) (agent.ExecuteBeadReport, error) {
	minPower := initialMinPower
	for {
		report, err := attempt(ctx, minPower)
		if recordAttempt != nil && report.BeadID != "" {
			recordAttempt(report)
		}
		if err != nil {
			return report, err
		}
		if report.Disrupted || isBudgetExhaustedFailure(report) || !policyescalation.ShouldEscalate(report.Status) || policyescalation.IsInfrastructureFailure(report.Status, report.Detail) {
			return report, nil
		}
		basis := minPower
		if report.ActualPower > 0 {
			basis = report.ActualPower
		}
		nextPower, nextErr := nextEscalationFloor(ladder, basis)
		if nextErr != nil {
			return report, nil
		}
		minPower = nextPower
	}
}
