package agent

import (
	"encoding/json"
	"testing"
	"time"

	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDrainServiceEvents_CapturesFinalText covers the ddx-7bc0c8d5
// normalizer stream: the upstream v0.8.0 final event carries a FinalText
// field (agent-32e8ff5e) containing the harness's cleaned final response.
// DDx must read it — NOT re-parse the raw stream per harness — so the
// reviewer verdict extractor and other consumers see normalized text.
//
// Pre-migration DDx defined serviceFinalData without FinalText and the
// field was silently dropped; result.Output was always empty and callers
// fell back to per-harness parsers (extractOutputCodex, extractOutputClaude,
// extractOutputPiGemini). Each of those had its own drift risk when
// upstream stream shapes changed, which is exactly the maintenance burden
// agent-32e8ff5e eliminates.
func TestDrainServiceEvents_CapturesFinalText(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 2)
	finalPayload, err := json.Marshal(map[string]any{
		"status":      "success",
		"exit_code":   0,
		"final_text":  "### Verdict: APPROVE\n\nClean run.",
		"duration_ms": 1234,
	})
	require.NoError(t, err)
	events <- agentlib.ServiceEvent{
		Type: "final",
		Time: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
		Data: finalPayload,
	}
	close(events)

	final, _, _, _ := drainServiceEvents(events)
	require.NotNil(t, final, "final event must drain even when only FinalText is populated")
	assert.Equal(t, "### Verdict: APPROVE\n\nClean run.", final.FinalText,
		"FinalText must round-trip verbatim — the reviewer verdict extractor depends on this being the harness's normalized output, not a raw stream frame")
}

// TestDrainServiceEvents_MissingFinalTextIsNotAnError covers the mixed
// rollout period: not every harness/upstream version emits FinalText. An
// absent FinalText must not crash the drain — it just yields empty Output
// and callers handle empty gracefully (reviewer surfaces it as a parse
// error, which feeds the retryable-failure path from ddx-f7ae036f).
func TestDrainServiceEvents_MissingFinalTextIsNotAnError(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 2)
	finalPayload, err := json.Marshal(map[string]any{
		"status":      "success",
		"exit_code":   0,
		"duration_ms": 500,
	})
	require.NoError(t, err)
	events <- agentlib.ServiceEvent{
		Type: "final",
		Time: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
		Data: finalPayload,
	}
	close(events)

	final, _, _, _ := drainServiceEvents(events)
	require.NotNil(t, final)
	assert.Empty(t, final.FinalText,
		"missing FinalText must remain empty rather than synthesize placeholder — the reviewer-error path is the right response, not a fabricated string")
	assert.Equal(t, "success", final.Status)
}
