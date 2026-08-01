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
	require.True(t, got.ReuseWin)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"slot_hit_count":1`)
	require.Contains(t, string(raw), `"slot_miss_count":0`)
	require.Contains(t, string(raw), `"reuse_win":true`)
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
	require.False(t, got.ReuseWin)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"slot_hit_count":0`)
	require.Contains(t, string(raw), `"slot_miss_count":1`)
	require.Contains(t, string(raw), `"reuse_win":false`)
	require.Contains(t, string(raw), `"time_saved_ms":0`)
	require.Contains(t, string(raw), `"bytes_saved":0`)
}

func TestAttemptWorkspaceReuseTelemetryDoesNotCountColdSavingsAsReuseWin(t *testing.T) {
	got := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:  0,
			SlotMissCount: 1,
		},
		AttemptWorkspaceReuseSavings{
			TimeSavedMS: 0,
			BytesSaved:  0,
		},
	)

	require.False(t, got.ReuseWin)
	require.Equal(t, 0, got.SlotHitCount)
	require.Equal(t, 1, got.SlotMissCount)

	evt := AttemptWorkspaceReuseTelemetryEvent(AttemptWorkspaceReuseTelemetryPayload{
		SlotHitCount:  int64(got.SlotHitCount),
		SlotMissCount: int64(got.SlotMissCount),
		ReuseWin:      got.ReuseWin,
		AttemptWorkspaceReuseSavingsContract: AttemptWorkspaceReuseSavingsContract{
			TimeSavedMS: got.TimeSavedMS,
			BytesSaved:  got.BytesSaved,
		},
	})
	require.Equal(t, "attempt-workspace-reuse", evt.Kind)
	require.Contains(t, evt.Summary, "reuse_win=false")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &body))
	require.Equal(t, false, body["reuse_win"])
	require.Equal(t, float64(0), body["time_saved_ms"])
	require.Equal(t, float64(0), body["bytes_saved"])

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"reuse_win":false`)
}

func TestAttemptWorkspaceReuseTelemetryCountsOnlyReuseHitsAsReuseWins(t *testing.T) {
	hit := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:  1,
			SlotMissCount: 0,
		},
		AttemptWorkspaceReuseSavings{
			TimeSavedMS: 0,
			BytesSaved:  0,
		},
	)
	require.True(t, hit.ReuseWin)

	miss := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:  0,
			SlotMissCount: 1,
		},
		AttemptWorkspaceReuseSavings{
			TimeSavedMS: 1234,
			BytesSaved:  5678,
		},
	)
	require.False(t, miss.ReuseWin)

	hitRaw, err := json.Marshal(hit)
	require.NoError(t, err)
	require.Contains(t, string(hitRaw), `"reuse_win":true`)

	missRaw, err := json.Marshal(miss)
	require.NoError(t, err)
	require.Contains(t, string(missRaw), `"reuse_win":false`)
}
