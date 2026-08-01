package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttemptWorkspaceReuseTelemetryPayloadCarriesSavingsFields(t *testing.T) {
	app := &stubBeadEventAppender{}
	body := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ReusableSlot: &AttemptWorkspaceSlot{
				SlotHitCount:            1,
				SlotMissCount:           0,
				ConservativeTimeSavedMS: 8400,
				ConservativeBytesSaved:  512 << 20,
			},
		},
		nil,
	)
	require.NotNil(t, body)
	body.AttemptID = "20260801T010203-reuse"
	appendReusableWorkspaceTelemetry(app, "ddx-int-0001", *body)

	require.Len(t, app.events, 1)
	got := app.events[0]
	require.Equal(t, "ddx-int-0001", got.BeadID)
	require.Equal(t, "reusable-workspace", got.Event.Kind)
	require.Equal(t, "ddx", got.Event.Actor)
	require.Equal(t, "legacy agent execute-bead", got.Event.Source)
	require.Contains(t, got.Event.Summary, "slot_hit_count=1")
	require.Contains(t, got.Event.Summary, "slot_miss_count=0")
	require.Contains(t, got.Event.Summary, "time_saved_ms=8400")
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

func TestAttemptWorkspaceReuseTelemetryCombinedEventKeepsColdStartZeros(t *testing.T) {
	body := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ReusableSlot: &AttemptWorkspaceSlot{
				Pooled: false,
			},
		},
		nil,
	)

	require.NotNil(t, body)
	require.Equal(t, 0, body.SlotHitCount)
	require.Equal(t, 1, body.SlotMissCount)
	require.Zero(t, body.TimeSavedMS)
	require.Zero(t, body.BytesSaved)

	body.AttemptID = "20260801T010203-cold"
	app := &stubBeadEventAppender{}
	appendReusableWorkspaceTelemetry(app, "ddx-int-0001", *body)

	require.Len(t, app.events, 1)
	got := app.events[0]
	require.Equal(t, "ddx-int-0001", got.BeadID)
	require.Equal(t, "reusable-workspace", got.Event.Kind)
	require.Contains(t, got.Event.Summary, "slot_hit_count=0")
	require.Contains(t, got.Event.Summary, "slot_miss_count=1")
	require.Contains(t, got.Event.Summary, "time_saved_ms=0")
	require.Contains(t, got.Event.Summary, "bytes_saved=0")

	var parsed ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(got.Event.Body), &parsed))
	require.Equal(t, "20260801T010203-cold", parsed.AttemptID)
	require.Equal(t, 0, parsed.SlotHitCount)
	require.Equal(t, 1, parsed.SlotMissCount)
	require.Zero(t, parsed.TimeSavedMS)
	require.Zero(t, parsed.BytesSaved)
}
