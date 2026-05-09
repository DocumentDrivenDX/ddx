package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	tierescalation "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
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

func highestViableEscalationFloor(l escalationFloorFinder) (int, error) {
	if l == nil {
		return 0, tierescalation.ErrLadderExhausted
	}
	floor := 0
	highest := 0
	for {
		next, err := nextEscalationFloor(l, floor)
		if err != nil {
			if errors.Is(err, tierescalation.ErrLadderExhausted) && highest > 0 {
				return highest, nil
			}
			return 0, err
		}
		highest = next
		floor = next
	}
}

func investigationRetryInitialMinPower(b *bead.Bead, baseMinPower, maxPower int, ladder escalationFloorFinder) (int, agent.ExecuteBeadReport, bool) {
	if tier, ok := triageTierHint(b); !ok || tier != policyescalation.TierSmart {
		return baseMinPower, agent.ExecuteBeadReport{}, false
	}
	floor, err := highestViableEscalationFloor(ladder)
	if err != nil {
		return baseMinPower, smartRouteUnavailableReport(b, baseMinPower, maxPower, err), true
	}
	if baseMinPower > floor {
		return baseMinPower, smartRouteUnavailableReport(b, baseMinPower, maxPower, nil), true
	}
	if maxPower > 0 && floor >= maxPower {
		return baseMinPower, smartRouteUnavailableReport(b, floor, maxPower, nil), true
	}
	return floor, agent.ExecuteBeadReport{}, false
}

func triageTierHint(b *bead.Bead) (policyescalation.ModelTier, bool) {
	if b == nil || b.Extra == nil {
		return "", false
	}
	raw, ok := b.Extra[agent.TriageTierHintKey]
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(raw))) {
	case string(policyescalation.TierSmart):
		return policyescalation.TierSmart, true
	case string(policyescalation.TierStandard):
		return policyescalation.TierStandard, true
	case string(policyescalation.TierCheap):
		return policyescalation.TierCheap, true
	default:
		return "", false
	}
}

func smartRouteUnavailableReport(b *bead.Bead, minPower, maxPower int, cause error) agent.ExecuteBeadReport {
	beadID := ""
	if b != nil {
		beadID = b.ID
	}
	detail := fmt.Sprintf("smart retry route unavailable: no viable routing candidate satisfies requested MinPower %d", minPower)
	if maxPower > 0 {
		detail += fmt.Sprintf(" and MaxPower %d", maxPower)
	}
	if cause != nil {
		detail += ": " + cause.Error()
	}
	return agent.ExecuteBeadReport{
		BeadID:        beadID,
		Status:        agent.ExecuteBeadStatusExecutionFailed,
		Detail:        detail,
		OutcomeReason: agent.FailureModeNoViableProvider,
	}
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
