package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAttemptWorkspaceReuseSavingsEstimateContractPreservesExplicitValues(t *testing.T) {
	outcome := AttemptWorkspaceReuseAllocationOutcome{
		SlotHitCount:            1,
		SlotMissCount:           0,
		ConservativeTimeSavedMS: 1834,
		ConservativeBytesSaved:  987654321,
	}
	telemetry := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(outcome)

	require.Equal(t, 1, telemetry.SlotHitCount)
	require.Equal(t, 0, telemetry.SlotMissCount)
	require.Equal(t, int64(1834), telemetry.TimeSavedMS)
	require.Equal(t, int64(987654321), telemetry.BytesSaved)

	raw, err := json.Marshal(telemetry)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"slot_hit_count":1`)
	require.Contains(t, string(raw), `"slot_miss_count":0`)
	require.Contains(t, string(raw), `"time_saved_ms":1834`)
	require.Contains(t, string(raw), `"bytes_saved":987654321`)

	combined := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ReusableSlot: &AttemptWorkspaceSlot{
				SlotHitCount:            1,
				SlotMissCount:           0,
				ConservativeTimeSavedMS: telemetry.TimeSavedMS,
				ConservativeBytesSaved:  telemetry.BytesSaved,
			},
		},
		&ReusableWorkspaceTelemetry{
			TimeSavedMS: telemetry.TimeSavedMS,
			BytesSaved:  telemetry.BytesSaved,
		},
	)

	require.NotNil(t, combined)
	require.Equal(t, 1, combined.SlotHitCount)
	require.Equal(t, 0, combined.SlotMissCount)
	require.Equal(t, int64(1834), combined.TimeSavedMS)
	require.Equal(t, int64(987654321), combined.BytesSaved)

	res := &ExecuteBeadResult{
		BeadID:    "ddx-int-0001",
		Status:    ExecuteBeadStatusNoChanges,
		BaseRev:   "base-rev",
		ResultRev: "result-rev",
	}
	applyReusableWorkspaceTelemetry(res, combined)
	report := ReportFromExecuteBeadResult(res, "")
	event := executeBeadLoopEvent(report, "worker", time.Unix(0, 0).UTC())
	require.Equal(t, "execute-bead", event.Kind)
	require.Contains(t, event.Body, "reusable_workspace_slot_hits=1")
	require.Contains(t, event.Body, "reusable_workspace_slot_misses=0")
	require.Contains(t, event.Body, "reusable_workspace_time_saved_ms=1834")
	require.Contains(t, event.Body, "reusable_workspace_bytes_saved=987654321")

	raw, err = json.Marshal(combined)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"slot_hit_count":1`)
	require.Contains(t, string(raw), `"slot_miss_count":0`)
	require.Contains(t, string(raw), `"time_saved_ms":1834`)
	require.Contains(t, string(raw), `"bytes_saved":987654321`)
}

func TestAttemptWorkspaceReuseSavingsEstimateZeroForMissOutcome(t *testing.T) {
	coldStart := AttemptWorkspaceReuseAllocationOutcome{
		SlotMissCount: 1,
	}
	require.Zero(t, coldStart.ConservativeTimeSavedMS)
	require.Zero(t, coldStart.ConservativeBytesSaved)
	got := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(coldStart)

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

	telemetry := got

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
