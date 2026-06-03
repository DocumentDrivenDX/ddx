package agent

import (
	"encoding/json"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeRoutingDecisionEvent(harness, provider, model string) agentlib.ServiceEvent {
	data, _ := json.Marshal(map[string]any{
		"harness":  harness,
		"provider": provider,
		"model":    model,
	})
	return agentlib.ServiceEvent{
		Type: "routing_decision",
		Time: time.Now(),
		Data: data,
	}
}

func makeRoutingDecisionEventWithReason(harness, provider, model, reason string) agentlib.ServiceEvent {
	data, _ := json.Marshal(map[string]any{
		"harness":  harness,
		"provider": provider,
		"model":    model,
		"reason":   reason,
	})
	return agentlib.ServiceEvent{
		Type: "routing_decision",
		Time: time.Now(),
		Data: data,
	}
}

// TestDrainServiceEvents_OnRouteResolved_FastPath asserts onRouteResolved fires
// once with the resolved triple in the no-watchdog (fast) drain path.
func TestDrainServiceEvents_OnRouteResolved_FastPath(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 3)
	events <- makeRoutingDecisionEvent("fiz", "anthropic", "sonnet-4.6")
	events <- makeFinalEvent("success")
	close(events)

	var calls [][]string
	onRouteResolved := func(harness, provider, model string) {
		calls = append(calls, []string{harness, provider, model})
	}

	final, _, _ := drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), nil, onRouteResolved)
	require.NotNil(t, final)
	require.Len(t, calls, 1, "onRouteResolved must fire exactly once in the fast path")
	assert.Equal(t, []string{"fiz", "anthropic", "sonnet-4.6"}, calls[0])
}

// TestDrainServiceEvents_OnRouteResolved_WatchdogPath asserts onRouteResolved
// fires once with the resolved triple in the watchdog drain path.
func TestDrainServiceEvents_OnRouteResolved_WatchdogPath(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 3)
	events <- makeRoutingDecisionEvent("fiz", "openrouter", "opus-4.8")
	events <- makeFinalEvent("success")
	close(events)

	var calls [][]string
	onRouteResolved := func(harness, provider, model string) {
		calls = append(calls, []string{harness, provider, model})
	}

	wd := &drainWatchdog{
		cancel:      func() {},
		idleTimeout: 30 * time.Second,
	}
	final, _, _ := drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), wd, onRouteResolved)
	require.NotNil(t, final)
	require.Len(t, calls, 1, "onRouteResolved must fire exactly once in the watchdog path")
	assert.Equal(t, []string{"fiz", "openrouter", "opus-4.8"}, calls[0])
}

// TestResolvedRouteRecordedInMetrics asserts that a RoutingDecision resolving
// harness=claude/provider=anthropic/model=sonnet-4.6 is recorded with those
// exact resolved values (not empty/requested) in routing_metrics.
// The complementary attemptmetrics assertion lives in attemptmetrics_test.go.
func TestResolvedRouteRecordedInMetrics(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 2)
	events <- makeRoutingDecisionEventWithReason("claude", "anthropic", "claude-sonnet-4-6", "policy-match")
	events <- makeFinalEvent("success")
	close(events)

	_, routing, _ := drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), nil, nil)
	require.NotNil(t, routing, "drain must capture the routing decision")

	// Build a result the way executeOnService does.
	result := &Result{
		Harness:     routing.Harness,
		Provider:    routing.Provider,
		Model:       routing.Model,
		RouteReason: routing.Reason,
	}
	assert.Equal(t, "claude", result.Harness, "resolved harness must come from routing decision, not requested config")
	assert.Equal(t, "anthropic", result.Provider, "resolved provider must come from routing decision")
	assert.Equal(t, "claude-sonnet-4-6", result.Model, "resolved model must come from routing decision")
	assert.NotEmpty(t, result.RouteReason, "route_reason must be non-empty for a routed attempt (AC2)")

	// Record to routing_metrics and read back, verifying resolved values land.
	dir := t.TempDir()
	store := NewRoutingMetricsStore(dir)
	require.NoError(t, store.AppendOutcome(RoutingOutcome{
		Harness:     result.Harness,
		Provider:    result.Provider,
		Model:       result.Model,
		RouteReason: result.RouteReason,
		ObservedAt:  time.Now().UTC(),
	}))

	outcomes, err := store.ReadOutcomes()
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, "claude", outcomes[0].Harness, "routing_metrics harness must be resolved value")
	assert.Equal(t, "anthropic", outcomes[0].Provider, "routing_metrics provider must be resolved value")
	assert.Equal(t, "claude-sonnet-4-6", outcomes[0].Model, "routing_metrics model must be resolved value")
	assert.NotEmpty(t, outcomes[0].RouteReason, "routing_metrics route_reason must be non-empty")
}
