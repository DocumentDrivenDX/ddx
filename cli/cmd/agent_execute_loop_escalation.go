package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	powerladder "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
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
		return 0, powerladder.ErrLadderExhausted
	}
	floor := actualPower
	for {
		next, err := l.Next(floor)
		if err == nil {
			return next, nil
		}
		var nvp *powerladder.NoViableProviderError
		if errors.As(err, &nvp) {
			floor = nvp.Floor
			continue
		}
		return 0, err
	}
}

func highestViableEscalationFloor(l escalationFloorFinder) (int, error) {
	if l == nil {
		return 0, powerladder.ErrLadderExhausted
	}
	floor := 0
	highest := 0
	for {
		next, err := nextEscalationFloor(l, floor)
		if err != nil {
			if errors.Is(err, powerladder.ErrLadderExhausted) && highest > 0 {
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
	if floor, ok := numericPowerFloorHint(b); ok {
		if baseMinPower > floor {
			return baseMinPower, smartRouteUnavailableReport(b, baseMinPower, maxPower, nil), true
		}
		if maxPower > 0 && floor >= maxPower {
			return baseMinPower, smartRouteUnavailableReport(b, floor, maxPower, nil), true
		}
		return floor, agent.ExecuteBeadReport{}, false
	}
	if powerClass, ok := triagePowerHint(b); ok && powerClass == policyescalation.PowerSmart {
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
		if powerClass, ok := policyescalation.ParseBeadPowerHintLabel(b.Labels); ok {
			labelFloor, hasFloor := resolvePowerFloor(powerClass, ladder)
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
		} else if policyescalation.HasBeadPowerHintLabel(b.Labels) {
			fmt.Fprintf(os.Stderr, "ddx: bead %s has unrecognized power:hint label; using default MinPower\n", b.ID)
		}
	}
	if inferZeroConfig {
		return zeroConfigInferredMinPower(b, baseMinPower, maxPower, ladder)
	}
	return baseMinPower, agent.ExecuteBeadReport{}, false
}

func resolvePowerFloor(powerClass policyescalation.PowerClass, ladder escalationFloorFinder) (int, bool) {
	switch powerClass {
	case policyescalation.PowerSmart:
		floor, err := highestViableEscalationFloor(ladder)
		if err != nil {
			return 0, false
		}
		return floor, true
	case policyescalation.PowerStandard:
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
	powerClass := policyescalation.InferPowerClass(b)
	if powerClass == "" {
		return baseMinPower, agent.ExecuteBeadReport{}, false
	}
	powerFloor, hasPowerFloor := resolvePowerFloor(powerClass, ladder)
	if !hasPowerFloor && powerClass != policyescalation.PowerCheap {
		return baseMinPower, smartRouteUnavailableReport(b, baseMinPower, maxPower, fmt.Errorf("no viable routing floor for inferred powerClass %s", powerClass)), true
	}
	if powerFloor > baseMinPower {
		if maxPower > 0 && powerFloor >= maxPower {
			return baseMinPower, smartRouteUnavailableReport(b, powerFloor, maxPower, nil), true
		}
		return powerFloor, agent.ExecuteBeadReport{}, false
	}
	return baseMinPower, agent.ExecuteBeadReport{}, false
}

func numericPowerFloorHint(b *bead.Bead) (int, bool) {
	if b == nil || b.Extra == nil {
		return 0, false
	}
	raw, ok := b.Extra[agent.TriagePowerHintKey]
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

func triagePowerHint(b *bead.Bead) (policyescalation.PowerClass, bool) {
	if b == nil || b.Extra == nil {
		return "", false
	}
	raw, ok := b.Extra[agent.TriagePowerHintKey]
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(raw))) {
	case string(policyescalation.PowerSmart):
		return policyescalation.PowerSmart, true
	case string(policyescalation.PowerStandard):
		return policyescalation.PowerStandard, true
	case string(policyescalation.PowerCheap):
		return policyescalation.PowerCheap, true
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

func runEscalatingPowerAttempts(
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
		var nextPower int
		if report.ActualPower > 0 {
			// Evidence-driven retry: bump MinPower one above the actual power
			// the previous route ran at. Profile reselection in the caller's
			// attempt callback moves to a stronger policy band only when the
			// new floor exceeds the current band's MaxPower; otherwise the
			// same policy intent is preserved so DDx does not duplicate
			// Fizeau policy bounds as initial dispatch floors.
			nextPower = report.ActualPower + 1
		} else {
			next, nextErr := nextEscalationFloor(ladder, minPower)
			if nextErr != nil {
				return report, nil
			}
			nextPower = next
		}
		minPower = nextPower
	}
}
