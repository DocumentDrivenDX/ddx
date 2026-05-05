package agent

import (
	"encoding/json"
	"testing"
	"time"

	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDrainServiceEventsIgnoresCompactionTelemetry(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 400)
	start := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	for elapsed := time.Duration(0); elapsed <= 15*time.Minute; elapsed += 3 * time.Second {
		events <- compactionTelemetryServiceEvent(start.Add(elapsed))
	}
	events <- agentlib.ServiceEvent{
		Type: "tool_call",
		Time: start.Add(15 * time.Minute),
		Data: json.RawMessage(`{"id":"call-1","name":"read","input":{"path":"README.md"}}`),
	}
	events <- agentlib.ServiceEvent{
		Type: "tool_result",
		Time: start.Add(15*time.Minute + time.Second),
		Data: json.RawMessage(`{"id":"call-1","output":"ok","duration_ms":1}`),
	}
	events <- agentlib.ServiceEvent{
		Type: "progress",
		Time: start.Add(15*time.Minute + 2*time.Second),
		Data: json.RawMessage(`{"phase":"thinking","state":"update","message":"still running"}`),
	}
	events <- agentlib.ServiceEvent{
		Type: "final",
		Time: start.Add(15*time.Minute + 3*time.Second),
		Data: json.RawMessage(`{"status":"success","exit_code":0,"duration_ms":1,"final_text":"done"}`),
	}
	close(events)

	final, _, _, _ := drainServiceEvents(events)
	require.NotNil(t, final)
	assert.Equal(t, "success", final.Status)
	assert.Empty(t, final.Error)
	assert.Equal(t, "done", final.FinalText)
}

func TestDrainServiceEventsKeepsFinalAfterCompactionTelemetry(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 700)
	start := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	for elapsed := time.Duration(0); elapsed < 15*time.Minute; elapsed += 3 * time.Second {
		events <- compactionTelemetryServiceEvent(start.Add(elapsed))
	}
	events <- compactionTelemetryServiceEvent(start.Add(15*time.Minute + time.Second))
	events <- agentlib.ServiceEvent{
		Type: "final",
		Time: start.Add(15*time.Minute + 2*time.Second),
		Data: json.RawMessage(`{"status":"success","exit_code":0,"duration_ms":1,"final_text":"still done"}`),
	}
	close(events)

	final, _, _, _ := drainServiceEvents(events)
	require.NotNil(t, final)
	assert.Equal(t, "success", final.Status)
	assert.Empty(t, final.Error)
	assert.Equal(t, "still done", final.FinalText)
}

// TestDrainServiceEvents_CapturesRouteEconomics covers the routing_decision
// event's winning candidate (eligible=true, model matches payload.model)
// carrying power, speed, and cost telemetry. DDx must surface those fields
// from the selected eligible candidate without touching the final event.
func TestDrainServiceEvents_CapturesRouteEconomics(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 4)

	routingPayload, err := json.Marshal(map[string]any{
		"harness":  "agent",
		"provider": "anthropic",
		"model":    "claude-3-5-sonnet",
		"candidates": []map[string]any{
			{
				"model":      "claude-3-haiku",
				"eligible":   false,
				"components": map[string]any{"power": 20},
			},
			{
				"model":                  "claude-3-5-sonnet",
				"eligible":               true,
				"cost_usd_per_1k_tokens": 0.0125,
				"cost_source":            "catalog",
				"components": map[string]any{
					"power":     65,
					"speed_tps": 42.5,
				},
			},
		},
	})
	require.NoError(t, err)
	events <- agentlib.ServiceEvent{
		Type: "routing_decision",
		Time: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Data: routingPayload,
	}

	finalPayload, err := json.Marshal(map[string]any{
		"status":     "success",
		"exit_code":  0,
		"final_text": "done",
	})
	require.NoError(t, err)
	events <- agentlib.ServiceEvent{
		Type: "final",
		Time: time.Date(2026, 4, 30, 12, 0, 1, 0, time.UTC),
		Data: finalPayload,
	}
	close(events)

	_, _, routing, actualPower := drainServiceEvents(events)
	assert.Equal(t, 65, actualPower,
		"power must come from the eligible winning candidate in routing_decision.candidates")
	require.NotNil(t, routing)
	assert.Equal(t, 65, routing.PredictedPower)
	assert.Equal(t, 42.5, routing.PredictedSpeedTPS)
	assert.Equal(t, 0.0125, routing.PredictedCostUSDPer1kTokens)
	assert.Equal(t, "catalog", routing.PredictedCostSource)
}

func compactionTelemetryServiceEvent(ts time.Time) agentlib.ServiceEvent {
	return agentlib.ServiceEvent{
		Type: "compaction",
		Time: ts,
		Data: json.RawMessage(`{"no_compaction":true,"messages_before":42,"messages_after":42}`),
	}
}
