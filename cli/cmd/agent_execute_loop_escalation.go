package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	return investigationRetryInitialMinPowerWithInference(b, baseMinPower, maxPower, ladder, false)
}

func investigationRetryInitialMinPowerWithInference(b *bead.Bead, baseMinPower, maxPower int, ladder escalationFloorFinder, inferZeroConfig bool) (int, agent.ExecuteBeadReport, bool) {
	if floor, ok := numericTierFloorHint(b); ok {
		if baseMinPower > floor {
			return baseMinPower, smartRouteUnavailableReport(b, baseMinPower, maxPower, nil), true
		}
		if maxPower > 0 && floor >= maxPower {
			return baseMinPower, smartRouteUnavailableReport(b, floor, maxPower, nil), true
		}
		return floor, agent.ExecuteBeadReport{}, false
	}
	if tier, ok := triageTierHint(b); ok && tier == policyescalation.TierSmart {
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
	if b != nil {
		if tier, ok := policyescalation.ParseBeadTierHintLabel(b.Labels); ok {
			labelFloor, hasFloor := resolveTierFloor(tier, ladder)
			if !hasFloor {
				labelFloor = baseMinPower
			}
			floor := labelFloor
			if baseMinPower > floor {
				floor = baseMinPower
			}
			if maxPower > 0 && floor >= maxPower {
				return baseMinPower, smartRouteUnavailableReport(b, floor, maxPower, nil), true
			}
			return floor, agent.ExecuteBeadReport{}, false
		} else if policyescalation.HasBeadTierHintLabel(b.Labels) {
			fmt.Fprintf(os.Stderr, "ddx: bead %s has unrecognized tier:hint label; using default MinPower\n", b.ID)
		}
	}
	if inferZeroConfig {
		return zeroConfigInferredMinPower(b, baseMinPower, maxPower, ladder)
	}
	return baseMinPower, agent.ExecuteBeadReport{}, false
}

func resolveTierFloor(tier policyescalation.ModelTier, ladder escalationFloorFinder) (int, bool) {
	switch tier {
	case policyescalation.TierSmart:
		floor, err := highestViableEscalationFloor(ladder)
		if err != nil {
			return 0, false
		}
		return floor, true
	case policyescalation.TierStandard:
		first, err := nextEscalationFloor(ladder, 0)
		if err != nil {
			return 0, false
		}
		second, err := nextEscalationFloor(ladder, first)
		if err != nil {
			return first, true
		}
		return second, true
	default:
		return 0, false
	}
}

func zeroConfigInferredMinPower(b *bead.Bead, baseMinPower, maxPower int, ladder escalationFloorFinder) (int, agent.ExecuteBeadReport, bool) {
	tier := policyescalation.InferTier(b)
	if tier == "" {
		return baseMinPower, agent.ExecuteBeadReport{}, false
	}
	tierFloor, hasTierFloor := resolveTierFloor(tier, ladder)
	if !hasTierFloor && tier != policyescalation.TierCheap {
		return baseMinPower, smartRouteUnavailableReport(b, baseMinPower, maxPower, fmt.Errorf("no viable routing floor for inferred tier %s", tier)), true
	}
	if tierFloor > baseMinPower {
		if maxPower > 0 && tierFloor >= maxPower {
			return baseMinPower, smartRouteUnavailableReport(b, tierFloor, maxPower, nil), true
		}
		return tierFloor, agent.ExecuteBeadReport{}, false
	}
	return baseMinPower, agent.ExecuteBeadReport{}, false
}

func numericTierFloorHint(b *bead.Bead) (int, bool) {
	if b == nil || b.Extra == nil {
		return 0, false
	}
	raw, ok := b.Extra[agent.TriageTierHintKey]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case int:
		return v, v > 0
	case int64:
		return int(v), int(v) > 0
	case float64:
		return int(v), int(v) > 0
	default:
		return 0, false
	}
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
	perBeadTracker *policyescalation.PerBeadCostTracker,
) (agent.ExecuteBeadReport, error) {
	minPower := initialMinPower
	for {
		report, err := attempt(ctx, minPower)
		if recordAttempt != nil && report.BeadID != "" {
			recordAttempt(report)
		}
		if perBeadTracker != nil {
			perBeadTracker.Add(report.Harness, report.CostUSD)
		}
		if err != nil {
			return report, err
		}
		if report.Disrupted || isBudgetExhaustedFailure(report) || !policyescalation.ShouldEscalate(report.Status) || policyescalation.IsInfrastructureFailure(report.Status, report.Detail) {
			return report, nil
		}
		if perBeadTracker != nil {
			if detail, tripped := perBeadTracker.Tripped(); tripped {
				report.Status = agent.ExecuteBeadStatusExecutionFailed
				report.Detail = detail
				report.CostUSD = perBeadTracker.Spent()
				return report, nil
			}
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
