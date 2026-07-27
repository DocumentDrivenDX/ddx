package agent

import (
	"context"
	"encoding/json"
	"testing"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServiceRun_ProjectsCostSource asserts a "reported" final yields
// CostSource "reported" and a source-less final yields the unknown value.
func TestServiceRun_ProjectsCostSource(t *testing.T) {
	t.Parallel()

	t.Run("reported", func(t *testing.T) {
		t.Parallel()
		svc := &passthroughTestService{
			executeEvents: []agentlib.ServiceEvent{
				{
					Type: "final",
					Data: []byte(`{"status":"success","exit_code":0,"final_text":"ok","cost_usd":0.25,"cost_source":"reported"}`),
				},
			},
		}
		rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 0, 0)
		result, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
			Prompt: "hello",
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, CostSourceReported, result.CostSource)
		assert.Equal(t, 0.25, result.CostUSD)
	})

	t.Run("source_less_final_is_unknown", func(t *testing.T) {
		t.Parallel()
		// Legacy source-less final: fizeau refuses to promote cost_usd without
		// cost_source, so CostUSD is nil and CostSource is unknown.
		svc := &passthroughTestService{
			executeEvents: []agentlib.ServiceEvent{
				{
					Type: "final",
					Data: []byte(`{"status":"success","exit_code":0,"final_text":"ok","cost_usd":1.50}`),
				},
			},
		}
		rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 0, 0)
		result, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
			Prompt: "hello",
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, CostSourceUnknown, result.CostSource)
		assert.Equal(t, 0.0, result.CostUSD, "source-less cost must not be promoted to Result.CostUSD")
	})
}

// TestServiceRun_ZeroCostWithUnknownSourceIsDistinguishable asserts a zero
// cost with unknown provenance is distinguishable from a genuine zero cost
// with reported provenance.
func TestServiceRun_ZeroCostWithUnknownSourceIsDistinguishable(t *testing.T) {
	t.Parallel()

	// Genuine free call: explicit zero + reported provenance.
	reportedPayload, err := json.Marshal(agentlib.ServiceFinalData{
		Status:     "success",
		ExitCode:   0,
		FinalText:  "free",
		CostUSD:    floatPtr(0.0),
		CostSource: agentlib.CostSourceReported,
	})
	require.NoError(t, err)

	// Unknown provenance: no cost_source (and optionally a discarded cost_usd).
	unknownPayload := []byte(`{"status":"success","exit_code":0,"final_text":"unknown"}`)

	reportedSvc := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{{Type: "final", Data: reportedPayload}},
	}
	unknownSvc := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{{Type: "final", Data: unknownPayload}},
	}
	rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 0, 0)

	reported, err := executeOnService(context.Background(), reportedSvc, t.TempDir(), rcfg, AgentRunRuntime{Prompt: "a"})
	require.NoError(t, err)
	require.NotNil(t, reported)

	unknown, err := executeOnService(context.Background(), unknownSvc, t.TempDir(), rcfg, AgentRunRuntime{Prompt: "b"})
	require.NoError(t, err)
	require.NotNil(t, unknown)

	// Both have zero CostUSD; only CostSource distinguishes them.
	assert.Equal(t, 0.0, reported.CostUSD)
	assert.Equal(t, 0.0, unknown.CostUSD)
	assert.Equal(t, CostSourceReported, reported.CostSource)
	assert.Equal(t, CostSourceUnknown, unknown.CostSource)
	assert.NotEqual(t, reported.CostSource, unknown.CostSource,
		"zero cost with unknown provenance must be distinguishable from genuine free/reported zero")
	assert.True(t, CostSourceAuthoritative(reported.CostSource))
	assert.False(t, CostSourceAuthoritative(unknown.CostSource))
}

func floatPtr(v float64) *float64 { return &v }

// TestReviewCostDeferredEventBody_RecordsCostSource proves cost-cap
// deferred evidence consumes CostSource rather than only storing it on Result.
func TestReviewCostDeferredEventBody_RecordsCostSource(t *testing.T) {
	t.Parallel()

	body := ReviewCostDeferredEventBody("rev-abc", 0.0, 1.0, 2.0, CostSourceUnknown)
	assert.Contains(t, body, "cost_source=unknown")
	assert.Contains(t, body, "review_cost_usd=0.0000")

	reported := ReviewCostDeferredEventBody("rev-abc", 0.0, 1.0, 2.0, CostSourceReported)
	assert.Contains(t, reported, "cost_source=reported")
	assert.NotEqual(t, body, reported, "unknown vs reported zero must differ in deferred evidence")

	empty := ReviewCostDeferredEventBody("rev-abc", 0.0, 1.0, 2.0, "")
	assert.Contains(t, empty, "cost_source=unknown", "empty provenance normalizes to unknown")
}
