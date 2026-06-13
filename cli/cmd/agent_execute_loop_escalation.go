package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	powerladder "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	policyescalation "github.com/DocumentDrivenDX/ddx/internal/escalation"
)

type escalationFloorFinder interface {
	Next(actualPower int) (int, error)
}

type tryLoopFloorFinder struct {
	ladder escalationFloorFinder
}

func (f tryLoopFloorFinder) Next(actualPower int) (int, error) {
	return nextEscalationFloor(f.ladder, actualPower)
}

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
		Profile:  report.RequestedProfile,
	}, resolved, pf, decision)
	report.ProviderFailureEvidence = &evidence
	if pin.Any() {
		agent.MarkHardPinExhausted(report, pin, pf)
	}
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
		if next <= floor {
			if highest > 0 {
				return highest, nil
			}
			return 0, powerladder.ErrLadderExhausted
		}
		highest = next
		floor = next
	}
}

func investigationRetryInitialMinPower(b *bead.Bead, baseMinPower, maxPower int, ladder escalationFloorFinder) (int, agent.ExecuteBeadReport, bool) {
	return baseMinPower, agent.ExecuteBeadReport{}, false
}

func recentProviderConnectivityMinPower(store *bead.Store, now time.Time, baseMinPower, maxPower int, ladder escalationFloorFinder) (int, agent.ExecuteBeadReport, bool) {
	if store == nil || ladder == nil {
		return baseMinPower, agent.ExecuteBeadReport{}, false
	}
	beads, err := store.List("", "", nil)
	if err != nil {
		return baseMinPower, agent.ExecuteBeadReport{}, false
	}
	floor := baseMinPower
	for _, b := range beads {
		events, err := store.Events(b.ID)
		if err != nil {
			continue
		}
		if next, ok := recentProviderConnectivityMinPowerFromEvents(events, now, floor, maxPower, ladder); ok {
			if next >= maxPower && maxPower > 0 {
				return baseMinPower, routeUnavailableReport(&b, next, maxPower, nil), true
			}
			if next > floor {
				floor = next
			}
		}
	}
	if floor > baseMinPower {
		return floor, agent.ExecuteBeadReport{}, true
	}
	return baseMinPower, agent.ExecuteBeadReport{}, false
}

func recentProviderConnectivityMinPowerFromEvents(events []bead.BeadEvent, now time.Time, baseMinPower, maxPower int, ladder escalationFloorFinder) (int, bool) {
	if len(events) == 0 || ladder == nil {
		return baseMinPower, false
	}
	floor := baseMinPower
	var last recentRouteEvent
	for _, ev := range events {
		if ev.CreatedAt.IsZero() || now.Sub(ev.CreatedAt) > agent.ProviderUnavailableCooldown || ev.CreatedAt.After(now.Add(time.Minute)) {
			continue
		}
		if route, ok := routeEventFromBeadEvent(ev); ok {
			last = route
			continue
		}
		if ev.Kind == "route-failure" {
			route, ok := routeFailureEventFromBeadEvent(ev)
			if !ok {
				continue
			}
			if next, ok := nextFloorForRecentConnectivity(route.ActualPower, maxPower, ladder); ok && next > floor {
				floor = next
			}
			continue
		}
		if ev.Kind == "execute-bead" && strings.EqualFold(strings.TrimSpace(ev.Summary), "execution_failed") && isProviderConnectivityEventBody(ev.Body) && last.Provider != "" {
			if next, ok := nextFloorForRecentConnectivity(last.ActualPower, maxPower, ladder); ok && next > floor {
				floor = next
			}
		}
	}
	return floor, floor > baseMinPower
}

type recentRouteEvent struct {
	Provider    string
	Model       string
	ActualPower int
}

func routeEventFromBeadEvent(ev bead.BeadEvent) (recentRouteEvent, bool) {
	if ev.Kind != "routing" && ev.Kind != "execution-routing-intent" {
		return recentRouteEvent{}, false
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(ev.Body), &body); err != nil {
		return recentRouteEvent{}, false
	}
	route := recentRouteEvent{
		Provider:    firstNonEmptyString(body["resolved_provider"], body["actual_provider"], body["provider"]),
		Model:       firstNonEmptyString(body["resolved_model"], body["actual_model"], body["model"]),
		ActualPower: intFromAny(body["actual_power"]),
	}
	if route.Provider == "" {
		return recentRouteEvent{}, false
	}
	return route, true
}

func routeFailureEventFromBeadEvent(ev bead.BeadEvent) (recentRouteEvent, bool) {
	var body map[string]any
	if err := json.Unmarshal([]byte(ev.Body), &body); err != nil {
		return recentRouteEvent{}, false
	}
	if reason := strings.TrimSpace(fmt.Sprint(body["outcome_reason"])); reason != "" && reason != agent.FailureModeProviderConnectivity {
		return recentRouteEvent{}, false
	}
	route := recentRouteEvent{
		Provider:    firstNonEmptyString(body["provider"]),
		Model:       firstNonEmptyString(body["model"]),
		ActualPower: intFromAny(body["actual_power"]),
	}
	if route.Provider == "" {
		return recentRouteEvent{}, false
	}
	return route, true
}

func nextFloorForRecentConnectivity(actualPower, maxPower int, ladder escalationFloorFinder) (int, bool) {
	next, err := nextEscalationFloor(ladder, actualPower)
	if err != nil {
		return 0, false
	}
	if maxPower > 0 && next >= maxPower {
		return next, true
	}
	return next, next > 0
}

func isProviderConnectivityEventBody(body string) bool {
	lower := strings.ToLower(body)
	for _, marker := range []string{"dial tcp", "i/o timeout", "connection refused", "connection reset", "no route to host", "provider error"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s := strings.TrimSpace(fmt.Sprint(value)); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

// resolvePowerFloor maps an inferred power class to a concrete ladder floor.
// When the ladder cannot satisfy the requested class, it degrades through the
// hierarchy (Smart → Standard → Cheap) per ADR-024 P1 ("cheapest first") so
// dispatch hands the bead to Fizeau with a viable bound rather than failing
// outright. Returning (0, false) means "no MinPower pin; let Fizeau pick".
func resolvePowerFloor(powerClass policyescalation.PowerClass, ladder escalationFloorFinder) (int, bool) {
	switch powerClass {
	case policyescalation.PowerSmart:
		if floor, err := highestViableEscalationFloor(ladder); err == nil {
			return floor, true
		}
		return resolvePowerFloor(policyescalation.PowerStandard, ladder)
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

// zeroConfigInitialMinPower selects the initial MinPower for a bead when the
// caller has no explicit routing flags or project routing config. The third
// return value carries a degradation note when the inferred class could not
// be pinned to a ladder floor and was downgraded to Cheap-equivalent
// (no MinPower pin). Callers should propagate the note into RoutingIntentNote
// so the evidence stream records why dispatch ended up unpinned.
func zeroConfigInitialMinPower(b *bead.Bead, powerClass policyescalation.PowerClass, baseMinPower, maxPower int, ladder escalationFloorFinder) (int, agent.ExecuteBeadReport, string, bool) {
	if powerClass == "" {
		return baseMinPower, agent.ExecuteBeadReport{}, "", false
	}
	powerFloor, hasPowerFloor := resolvePowerFloor(powerClass, ladder)
	if !hasPowerFloor {
		note := ""
		if powerClass != policyescalation.PowerCheap {
			note = fmt.Sprintf(
				"no ladder floor for inferred powerClass %s; degraded to cheap-equivalent (no MinPower pin)",
				powerClass,
			)
		}
		return baseMinPower, agent.ExecuteBeadReport{}, note, false
	}
	if powerFloor > baseMinPower {
		if maxPower > 0 && powerFloor >= maxPower {
			return baseMinPower, routeUnavailableReport(b, powerFloor, maxPower, nil), "", true
		}
		return powerFloor, agent.ExecuteBeadReport{}, "", false
	}
	return baseMinPower, agent.ExecuteBeadReport{}, "", false
}

func routeUnavailableReport(b *bead.Bead, minPower, maxPower int, cause error) agent.ExecuteBeadReport {
	beadID := ""
	if b != nil {
		beadID = b.ID
	}
	detail := fmt.Sprintf("route unavailable: no viable routing candidate satisfies requested MinPower %d", minPower)
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
	allowInfrastructureRetry bool,
	pin agent.ProviderPin,
) (agent.ExecuteBeadReport, error) {
	minPower := initialMinPower
	floors := tryLoopFloorFinder{ladder: ladder}
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
			ActualPower:              report.ActualPower,
			OutcomeReason:            report.OutcomeReason,
			Disrupted:                report.Disrupted,
			BudgetExhausted:          isBudgetExhaustedFailure(report),
			AllowInfrastructureRetry: allowInfrastructureRetry,
		}, floors)
		if transition.Action != executeloop.TryLoopActionRetryPower {
			return report, nil
		}
		minPower = transition.NextMinPower
	}
}
