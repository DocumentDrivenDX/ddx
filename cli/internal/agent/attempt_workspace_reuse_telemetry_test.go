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

func TestAttemptWorkspaceReuseTelemetryColdStartRecordsSlotMiss(t *testing.T) {
	telemetry := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotMissCount: 1,
		},
		AttemptWorkspaceReuseSavings{},
	)

	require.Zero(t, telemetry.SlotHitCount)
	require.Equal(t, 1, telemetry.SlotMissCount)
	require.Zero(t, telemetry.TimeSavedMS)
	require.Zero(t, telemetry.BytesSaved)

	app := &stubBeadEventAppender{}
	appendReusableWorkspaceTelemetry(app, "ddx-int-0001", ReusableWorkspaceTelemetry{
		AttemptID:     "20260801T010203-cold",
		SlotHitCount:  telemetry.SlotHitCount,
		SlotMissCount: telemetry.SlotMissCount,
		TimeSavedMS:   telemetry.TimeSavedMS,
		BytesSaved:    telemetry.BytesSaved,
	})

	require.Len(t, app.events, 1)
	evt := app.events[0].Event
	require.Equal(t, "reusable-workspace", evt.Kind)
	require.Contains(t, evt.Summary, "slot_hit_count=0")
	require.Contains(t, evt.Summary, "slot_miss_count=1")
	require.Contains(t, evt.Summary, "time_saved_ms=0")
	require.Contains(t, evt.Summary, "bytes_saved=0")

	var parsed ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
	require.Equal(t, "20260801T010203-cold", parsed.AttemptID)
	require.Zero(t, parsed.SlotHitCount)
	require.Equal(t, 1, parsed.SlotMissCount)
	require.Zero(t, parsed.TimeSavedMS)
	require.Zero(t, parsed.BytesSaved)
}
