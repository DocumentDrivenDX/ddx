package agent

import (
	"encoding/json"
	"testing"
	"time"

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
	allocationOutcome := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:  1,
			SlotMissCount: 0,
		},
		AttemptWorkspaceReuseSavings{
			TimeSavedMS: 1834,
			BytesSaved:  987654321,
		},
	)

	got := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ReusableSlot: &AttemptWorkspaceSlot{
				SlotHitCount:            1,
				SlotMissCount:           0,
				ConservativeTimeSavedMS: 0,
				ConservativeBytesSaved:  0,
			},
		},
		&ReusableWorkspaceTelemetry{
			TimeSavedMS: allocationOutcome.TimeSavedMS,
			BytesSaved:  allocationOutcome.BytesSaved,
		},
	)

	require.NotNil(t, got)
	require.Equal(t, 1, got.SlotHitCount)
	require.Equal(t, 0, got.SlotMissCount)
	require.Equal(t, int64(1834), got.TimeSavedMS)
	require.Equal(t, int64(987654321), got.BytesSaved)

	res := &ExecuteBeadResult{
		BeadID:    "ddx-int-0001",
		Status:    ExecuteBeadStatusNoChanges,
		BaseRev:   "base-rev",
		ResultRev: "result-rev",
	}
	applyReusableWorkspaceTelemetry(res, got)
	report := ReportFromExecuteBeadResult(res, "")
	event := executeBeadLoopEvent(report, "worker", time.Unix(0, 0).UTC())
	require.Equal(t, "execute-bead", event.Kind)
	require.Contains(t, event.Body, "reusable_workspace_slot_hits=1")
	require.Contains(t, event.Body, "reusable_workspace_slot_misses=0")
	require.Contains(t, event.Body, "reusable_workspace_time_saved_ms=1834")
	require.Contains(t, event.Body, "reusable_workspace_bytes_saved=987654321")

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
