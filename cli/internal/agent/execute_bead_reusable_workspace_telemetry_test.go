package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAttemptWorkspaceReuseTelemetryRecordsReuseSavingsFromAllocationOutcome(t *testing.T) {
	app := &stubBeadEventAppender{}
	body := ReusableWorkspaceTelemetry{
		AttemptID:     "20260801T010203-reuse",
		SlotHitCount:  1,
		SlotMissCount: 0,
		TimeSavedMS:   8400,
		BytesSaved:    512 << 20,
	}
	appendReusableWorkspaceTelemetry(app, "ddx-int-0001", body)

	require.Len(t, app.events, 1)
	got := app.events[0]
	require.Equal(t, "ddx-int-0001", got.BeadID)
	require.Equal(t, "reusable-workspace", got.Event.Kind)
	require.Equal(t, "ddx", got.Event.Actor)
	require.Equal(t, "legacy agent execute-bead", got.Event.Source)
	require.Contains(t, got.Event.Summary, "slot_hit_count=1")
	require.Contains(t, got.Event.Summary, "slot_miss_count=0")
	require.Contains(t, got.Event.Summary, "time_saved=8400")
	require.Contains(t, got.Event.Summary, "bytes_saved=536870912")

	var parsed ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(got.Event.Body), &parsed))
	require.Equal(t, "20260801T010203-reuse", parsed.AttemptID)
	require.Equal(t, int64(8400), parsed.TimeSavedMS)
	require.Equal(t, int64(512<<20), parsed.BytesSaved)
	require.Equal(t, 1, parsed.SlotHitCount)
	require.Equal(t, 0, parsed.SlotMissCount)
}

func TestAttemptWorkspaceReuseTelemetryDoesNotRecomputeReuseSavings(t *testing.T) {
	got := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ReusableSlot: &AttemptWorkspaceSlot{
				SlotHitCount:            7,
				SlotMissCount:           3,
				ConservativeTimeSavedMS: 1234,
				ConservativeBytesSaved:  5678,
			},
		},
		&ReusableWorkspaceTelemetry{
			AttemptID:     "fallback",
			SlotHitCount:  1,
			SlotMissCount: 1,
			TimeSavedMS:   9999,
			BytesSaved:    8888,
		},
	)

	require.NotNil(t, got)
	require.Equal(t, 7, got.SlotHitCount)
	require.Equal(t, 3, got.SlotMissCount)
	require.Equal(t, int64(1234), got.TimeSavedMS)
	require.Equal(t, int64(5678), got.BytesSaved)
}

func TestAttemptWorkspaceReuseTelemetryPayloadEmitsZeroSavingsForColdStart(t *testing.T) {
	app := &stubBeadEventAppender{}
	appendReusableWorkspaceTelemetry(app, "ddx-int-0001", ReusableWorkspaceTelemetry{
		AttemptID: "20260801T010203-cold",
	})

	require.Len(t, app.events, 1)
	evt := app.events[0].Event
	require.Equal(t, "reusable-workspace", evt.Kind)
	require.Contains(t, evt.Summary, "slot_hit_count=0")
	require.Contains(t, evt.Summary, "slot_miss_count=0")
	require.Contains(t, evt.Summary, "time_saved=0")
	require.Contains(t, evt.Summary, "bytes_saved=0")

	var parsed ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
	require.Equal(t, "20260801T010203-cold", parsed.AttemptID)
	require.Zero(t, parsed.SlotHitCount)
	require.Zero(t, parsed.SlotMissCount)
	require.Zero(t, parsed.TimeSavedMS)
	require.Zero(t, parsed.BytesSaved)

	report := ExecuteBeadReport{
		BeadID:                       "ddx-int-0001",
		Status:                       ExecuteBeadStatusNoChanges,
		ReusableWorkspaceSlotMisses:  1,
		ReusableWorkspaceTimeSavedMS: 0,
		ReusableWorkspaceBytesSaved:  0,
	}
	reportJSON, err := json.Marshal(report)
	require.NoError(t, err)
	require.Contains(t, string(reportJSON), `"reusable_workspace_slot_misses":1`)
	require.Contains(t, string(reportJSON), `"reusable_workspace_time_saved_ms":0`)
	require.Contains(t, string(reportJSON), `"reusable_workspace_bytes_saved":0`)

	body := executeBeadLoopEvent(report, "worker", time.Unix(0, 0).UTC())
	require.Equal(t, "execute-bead", body.Kind)
	require.Contains(t, body.Body, "reusable_workspace_slot_misses=1")
	require.Contains(t, body.Body, "reusable_workspace_time_saved_ms=0")
	require.Contains(t, body.Body, "reusable_workspace_bytes_saved=0")
}
