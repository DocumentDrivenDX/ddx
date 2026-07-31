package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttemptWorkspaceReuseTelemetryCombinedPayloadIncludesHitsMissesAndSavings(t *testing.T) {
	payload := AttemptWorkspaceReuseTelemetryPayload{
		SlotHitCount:  3,
		SlotMissCount: 1,
		AttemptWorkspaceReuseSavingsContract: AttemptWorkspaceReuseSavingsContract{
			TimeSavedMS: 4200,
			BytesSaved:  1048576,
		},
	}

	evt := AttemptWorkspaceReuseTelemetryEvent(payload)
	require.Equal(t, "attempt-workspace-reuse", evt.Kind)
	require.Equal(t, "hits=3 misses=1 time_saved_ms=4200 bytes_saved=1048576", evt.Summary)
	require.NotEmpty(t, evt.Body)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &got))
	assert.Equal(t, float64(3), got["slot_hit_count"])
	assert.Equal(t, float64(1), got["slot_miss_count"])
	assert.Equal(t, float64(4200), got["time_saved_ms"])
	assert.Equal(t, float64(1048576), got["bytes_saved"])
}

func TestAttemptWorkspaceReuseTelemetryCombinedPayloadUsesSavingsContract(t *testing.T) {
	savings := AttemptWorkspaceReuseSavingsContract{
		TimeSavedMS: 9876,
		BytesSaved:  7654321,
	}

	evt := AttemptWorkspaceReuseTelemetryEvent(AttemptWorkspaceReuseTelemetryPayload{
		SlotHitCount:                         9,
		SlotMissCount:                        2,
		AttemptWorkspaceReuseSavingsContract: savings,
	})

	var got AttemptWorkspaceReuseTelemetryPayload
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &got))
	assert.Equal(t, int64(9), got.SlotHitCount)
	assert.Equal(t, int64(2), got.SlotMissCount)
	assert.Equal(t, savings.TimeSavedMS, got.TimeSavedMS)
	assert.Equal(t, savings.BytesSaved, got.BytesSaved)
}
