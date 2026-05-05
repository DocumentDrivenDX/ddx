package agent

import (
	"encoding/json"
	"testing"
	"time"

	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	_, routing, actualPower, _ := drainServiceEvents(events)
	assert.Equal(t, 65, actualPower,
		"power must come from the eligible winning candidate in routing_decision.candidates")
	require.NotNil(t, routing)
	assert.Equal(t, 65, routing.PredictedPower)
	assert.Equal(t, 42.5, routing.PredictedSpeedTPS)
	assert.Equal(t, 0.0125, routing.PredictedCostUSDPer1kTokens)
	assert.Equal(t, "catalog", routing.PredictedCostSource)
}

// TestDrainServiceEvents_ForwardsCanonicalProgressPayload proves canonical
// Fizeau progress is surfaced directly from the ServiceEvent stream rather
// than being reconstructed from session-log JSONL.
func TestDrainServiceEvents_ForwardsCanonicalProgressPayload(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 2)
	progressPayload, err := json.Marshal(map[string]any{
		"phase":           "tool",
		"state":           "complete",
		"task_id":         "ddx-1234",
		"turn_index":      7,
		"tool_name":       "apply_patch",
		"action":          "add test implementation",
		"target":          "cli/internal/file.go",
		"output_excerpt":  "Success. Updated the following files:",
		"output_bytes":    312,
		"output_lines":    12,
		"duration_ms":     35,
		"tok_per_sec":     18.4,
		"input_tokens":    10,
		"output_tokens":   20,
		"total_tokens":    30,
		"session_summary": "add test implementation",
	})
	require.NoError(t, err)
	events <- agentlib.ServiceEvent{
		Type: "progress",
		Time: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Data: progressPayload,
	}
	events <- agentlib.ServiceEvent{
		Type: "final",
		Time: time.Date(2026, 4, 30, 12, 0, 1, 0, time.UTC),
		Data: json.RawMessage(`{"status":"success","exit_code":0,"final_text":"done"}`),
	}
	close(events)

	_, _, _, progress := drainServiceEvents(events)
	require.Len(t, progress, 1)
	assert.Equal(t, "ddx-1234", progress[0].TaskID)
	assert.Equal(t, 7, progress[0].TurnIndex)
	assert.Equal(t, "add test implementation", progress[0].Action)
	assert.Equal(t, "cli/internal/file.go", progress[0].Target)
	assert.Equal(t, 312, progress[0].OutputBytes)
	assert.Equal(t, 12, progress[0].OutputLines)
	require.NotNil(t, progress[0].TokPerSec)
	assert.InDelta(t, 18.4, *progress[0].TokPerSec, 0.0001)
	assert.Contains(t, FormatServiceProgressEntries(progress), "ok ddx-1234 7 add test implementation to cli/internal/file.go")
	assert.Contains(t, FormatServiceProgressEntries(progress), "< out=312B 12 lines")
	assert.Contains(t, FormatServiceProgressEntries(progress), "18.4 tok/s")
}
