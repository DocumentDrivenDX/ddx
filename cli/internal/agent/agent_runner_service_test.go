package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
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

	_, routing, _ := drainServiceEvents(events)
	require.NotNil(t, routing)
	power, speed, cost, source := selectedRoutingCandidateMetrics(routing)
	assert.Equal(t, 65, power,
		"power must come from the eligible winning candidate in routing_decision.candidates")
	assert.Equal(t, 42.5, speed)
	assert.Equal(t, 0.0125, cost)
	assert.Equal(t, "catalog", source)
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

	_, _, progress := drainServiceEvents(events)
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

func TestDrainServiceEvents_WritesLiveRouteAndProgress(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 2)
	events <- agentlib.ServiceEvent{
		Type: "routing_decision",
		Time: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Data: json.RawMessage(`{"harness":"agent","provider":"openrouter","model":"gpt-5.4-mini","reason":"profile"}`),
	}
	events <- agentlib.ServiceEvent{
		Type: "progress",
		Time: time.Date(2026, 4, 30, 12, 0, 1, 0, time.UTC),
		Data: json.RawMessage(`{"phase":"tool","state":"complete","task_id":"ddx-live","turn_index":2,"action":"run tests","target":"cli/internal/bead","output_bytes":42,"output_lines":3}`),
	}
	close(events)

	var out bytes.Buffer
	_, routing, progress := drainServiceEventsWithWriter(events, &out)
	require.NotNil(t, routing)
	require.Len(t, progress, 1)
	assert.Contains(t, out.String(), "route: harness=agent provider=openrouter model=gpt-5.4-mini reason=profile")
	assert.Contains(t, out.String(), "ok ddx-live 2 run tests to cli/internal/bead < out=42B 3 lines")
}

// TestAgentExecution_UsesFizeauServicePathOnly proves transcript-producing
// agent execution is bound to the Fizeau service adapter, with the service
// factory as the only execution seam used here.
func TestAgentExecution_UsesFizeauServicePathOnly(t *testing.T) {
	stub := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{
			{
				Type: "progress",
				Time: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
				Data: json.RawMessage(`{"task_id":"ddx-1234","turn_index":1,"tool_name":"Bash","action":"run","target":"cli/internal/file.go","output_excerpt":"ok","output_bytes":2,"output_lines":1,"duration_ms":3}`),
			},
			{
				Type: "final",
				Time: time.Date(2026, 4, 30, 12, 0, 1, 0, time.UTC),
				Data: json.RawMessage(`{"status":"success","exit_code":0,"final_text":"done"}`),
			},
		},
	}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() {
		SetServiceRunFactory(nil)
	})

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Model: "claude-sonnet-4-6",
	}).Resolve(config.CLIOverrides{Harness: "agent"})

	result, err := RunWithConfigViaService(context.Background(), t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "hello",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, stub.executeCalled, "RunWithConfigViaService must use the Fizeau service adapter")
	assert.Equal(t, "agent", stub.lastReq.Harness)
	assert.Equal(t, "hello", stub.lastReq.Prompt)
	assert.Equal(t, "done", result.Output)
	assert.Equal(t, 0, result.ExitCode)
}
