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
