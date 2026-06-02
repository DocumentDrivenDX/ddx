package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
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
		"harness":  "fiz",
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

	_, routing, _ := drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), nil, nil)
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
		"subject":         "cli/internal/file.go",
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

	_, _, progress := drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), nil, nil)
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

func TestDrainServiceEventsWithWriter_LabelsRoutesByPhase(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 2)
	events <- agentlib.ServiceEvent{
		Type: "routing_decision",
		Time: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Data: json.RawMessage(`{"harness":"fiz","provider":"openrouter","model":"gpt-5.4-mini","reason":"profile"}`),
	}
	events <- agentlib.ServiceEvent{
		Type: "progress",
		Time: time.Date(2026, 4, 30, 12, 0, 1, 0, time.UTC),
		Data: json.RawMessage(`{"phase":"tool","state":"complete","task_id":"ddx-live","turn_index":2,"action":"run tests","subject":"cli/internal/bead","output_bytes":42,"output_lines":3}`),
	}
	close(events)

	var out bytes.Buffer
	_, routing, progress := drainServiceEventsWithRenderer(events, &out, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), nil, nil)
	require.NotNil(t, routing)
	require.Len(t, progress, 1)
	assert.Contains(t, out.String(), "12:00:00 do route fiz/gpt-5.4-mini provider=openrouter reason=profile")
	assert.Contains(t, out.String(), "12:00:01 do ok ddx-live 2 run tests to cli/internal/bead < out=42B 3 lines")
	assert.NotContains(t, out.String(), "route: harness=agent")
}

// TestFizeauPassthrough_RawModelUnchanged proves DDx passes the operator-supplied
// model string directly into ServiceExecuteRequest.Model without normalizing,
// fuzzy-matching, or aliasing it. Model resolution is Fizeau-owned per CONTRACT-003.
func TestFizeauPassthrough_RawModelUnchanged(t *testing.T) {
	stub := &passthroughTestService{}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Model: "qwen36",
	}).Resolve(config.CLIOverrides{Harness: "agent"})

	_, err := RunWithConfigViaService(context.Background(), t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "test",
	})
	require.NoError(t, err)
	require.True(t, stub.executeCalled)
	assert.Equal(t, "qwen36", stub.lastReq.Model,
		"DDx must not normalize or transform the raw model string before passing to Fizeau")
}

// TestFizeauPassthrough_RecordsActualModelFromRouting proves DDx records the
// canonical model/provider/harness that Fizeau returns via routing events, not
// the raw requested model string. This validates Fizeau owns resolution.
func TestFizeauPassthrough_RecordsActualModelFromRouting(t *testing.T) {
	routingPayload, err := json.Marshal(map[string]any{
		"harness":  "fiz",
		"provider": "openai-compat",
		"model":    "Qwen-3.6-27b-MLX-8bit",
		"candidates": []map[string]any{
			{"model": "Qwen-3.6-27b-MLX-8bit", "eligible": true, "components": map[string]any{"power": 45}},
		},
	})
	require.NoError(t, err)
	finalPayload, err := json.Marshal(map[string]any{
		"status":     "success",
		"exit_code":  0,
		"final_text": "done",
		"routing_actual": map[string]any{
			"harness":  "fiz",
			"provider": "openai-compat",
			"model":    "Qwen-3.6-27b-MLX-8bit",
			"power":    45,
		},
	})
	require.NoError(t, err)

	stub := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{
			{Type: "routing_decision", Data: routingPayload},
			{Type: "final", Data: finalPayload},
		},
	}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Model: "qwen36",
	}).Resolve(config.CLIOverrides{Harness: "agent"})

	result, err := RunWithConfigViaService(context.Background(), t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "test",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Qwen-3.6-27b-MLX-8bit", result.Model,
		"DDx must record the canonical model returned by Fizeau routing, not the raw requested string")
	assert.Equal(t, "openai-compat", result.Provider)
	assert.Equal(t, "fiz", result.Harness)
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
				Data: json.RawMessage(`{"task_id":"ddx-1234","turn_index":1,"tool_name":"Bash","action":"run","subject":"cli/internal/file.go","output_excerpt":"ok","output_bytes":2,"output_lines":1,"duration_ms":3}`),
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
		Env: map[string]string{
			DDXModeEnvKey: DDXModeBeadExecution,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, stub.executeCalled, "RunWithConfigViaService must use the Fizeau service adapter")
	assert.Equal(t, "fiz", stub.lastReq.Harness)
	assert.Equal(t, "hello", stub.lastReq.Prompt)
	assert.Equal(t, DDXModeBeadExecution, stub.lastReq.Metadata[DDXModeEnvKey])
	assert.Equal(t, "done", result.Output)
	assert.Equal(t, 0, result.ExitCode)
}

func TestRunWithConfigViaService_CapturesCacheReadTokens(t *testing.T) {
	stub := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{
			{
				Type: "final",
				Time: time.Date(2026, 4, 30, 12, 0, 1, 0, time.UTC),
				Data: json.RawMessage(`{"status":"success","exit_code":0,"final_text":"done","cost_usd":0.003,"usage":{"input_tokens":200,"output_tokens":500,"cache_read_tokens":800,"total_tokens":1500}}`),
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
	assert.Equal(t, 200, result.InputTokens)
	assert.Equal(t, 800, result.CachedTokens)
	assert.Equal(t, 500, result.OutputTokens)
	assert.Equal(t, 1500, result.Tokens)
	assert.Equal(t, 0.003, result.CostUSD)
}
