package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttemptWorkspaceReuseTelemetryInputCarriesReusedAllocationSavings(t *testing.T) {
	got := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:  1,
			SlotMissCount: 0,
		},
		AttemptWorkspaceReuseSavings{
			TimeSavedMS: 1834,
			BytesSaved:  987654321,
		},
	)

	require.Equal(t, 1, got.SlotHitCount)
	require.Equal(t, 0, got.SlotMissCount)
	require.Equal(t, int64(1834), got.TimeSavedMS)
	require.Equal(t, int64(987654321), got.BytesSaved)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"slot_hit_count":1`)
	require.Contains(t, string(raw), `"slot_miss_count":0`)
	require.Contains(t, string(raw), `"time_saved_ms":1834`)
	require.Contains(t, string(raw), `"bytes_saved":987654321`)
}

func TestAttemptWorkspaceReuseTelemetryReusedAttemptCarriesAllocationSavings(t *testing.T) {
	got := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:  1,
			SlotMissCount: 0,
		},
		AttemptWorkspaceReuseSavings{
			TimeSavedMS: 1834,
			BytesSaved:  987654321,
		},
	)

	require.Equal(t, 1, got.SlotHitCount)
	require.Equal(t, 0, got.SlotMissCount)
	require.Equal(t, int64(1834), got.TimeSavedMS)
	require.Equal(t, int64(987654321), got.BytesSaved)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"slot_hit_count":1`)
	require.Contains(t, string(raw), `"slot_miss_count":0`)
	require.Contains(t, string(raw), `"time_saved_ms":1834`)
	require.Contains(t, string(raw), `"bytes_saved":987654321`)
}

func TestAttemptWorkspaceReuseTelemetryInputPreservesColdStartZeroSavings(t *testing.T) {
	got := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:  0,
			SlotMissCount: 1,
		},
		AttemptWorkspaceReuseSavings{},
	)

	require.Equal(t, 0, got.SlotHitCount)
	require.Equal(t, 1, got.SlotMissCount)
	require.Zero(t, got.TimeSavedMS)
	require.Zero(t, got.BytesSaved)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"slot_hit_count":0`)
	require.Contains(t, string(raw), `"slot_miss_count":1`)
	require.Contains(t, string(raw), `"time_saved_ms":0`)
	require.Contains(t, string(raw), `"bytes_saved":0`)
}
