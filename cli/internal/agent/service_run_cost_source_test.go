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

func TestServiceResultPreservesFizeauCostPresence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		finalPayload []byte
		wantCostUSD  float64
		wantPresence *bool
	}{
		{
			name:         "absent",
			finalPayload: []byte(`{"status":"success","exit_code":0,"final_text":"ok"}`),
			wantCostUSD:  0,
			wantPresence: nil,
		},
		{
			name: "explicit_zero",
			finalPayload: mustJSON(t, agentlib.ServiceFinalData{
				Status:     "success",
				ExitCode:   0,
				FinalText:  "free",
				CostUSD:    floatPtr(0),
				CostSource: agentlib.CostSourceReported,
			}),
			wantCostUSD:  0,
			wantPresence: boolPtr(false),
		},
		{
			name: "positive",
			finalPayload: mustJSON(t, agentlib.ServiceFinalData{
				Status:     "success",
				ExitCode:   0,
				FinalText:  "paid",
				CostUSD:    floatPtr(1.5),
				CostSource: agentlib.CostSourceReported,
			}),
			wantCostUSD:  1.5,
			wantPresence: boolPtr(true),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			svc := &passthroughTestService{
				executeEvents: []agentlib.ServiceEvent{
					{
						Type: "final",
						Data: tc.finalPayload,
					},
				},
			}
			rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-3-7-sonnet", 0, 0)
			result, err := executeOnService(context.Background(), svc, workDir, rcfg, AgentRunRuntime{
				Prompt: "hello",
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tc.wantCostUSD, result.CostUSD)
			assertBoolPtrEqual(t, tc.wantPresence, result.FizeauCostPresent)

			entries, err := ReadSessionIndex(SessionLogDirForWorkDir(workDir), SessionIndexQuery{})
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assertBoolPtrEqual(t, tc.wantPresence, entries[0].FizeauCostPresent)
		})
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return out
}

func boolPtr(v bool) *bool {
	return &v
}

func assertBoolPtrEqual(t *testing.T, want, got *bool) {
	t.Helper()
	switch {
	case want == nil && got == nil:
		return
	case want == nil || got == nil:
		t.Fatalf("presence = %v, want %v", got, want)
	case *want != *got:
		t.Fatalf("presence = %v, want %v", *got, *want)
	}
}
