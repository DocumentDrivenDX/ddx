package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttemptWorkspaceReuseTelemetryReusedAttemptCarriesAllocationSavings(t *testing.T) {
	app := &stubBeadEventAppender{}
	appendReusableWorkspaceTelemetry(app, "ddx-reuse", ReusableWorkspaceTelemetry{
		AttemptID:     "20260728T010203-reuse",
		SlotHitCount:  1,
		SlotMissCount: 0,
		TimeSavedMS:   8400,
		BytesSaved:    512 << 20,
	})

	require.Len(t, app.events, 1)
	got := app.events[0]
	require.Equal(t, "ddx-reuse", got.BeadID)
	require.Equal(t, "reusable-workspace", got.Event.Kind)
	require.Equal(t, "ddx", got.Event.Actor)
	require.Equal(t, "legacy agent execute-bead", got.Event.Source)
	require.Contains(t, got.Event.Summary, "slot_hit_count=1")
	require.Contains(t, got.Event.Summary, "slot_miss_count=0")
	require.Contains(t, got.Event.Summary, "time_saved=8400")
	require.Contains(t, got.Event.Summary, "bytes_saved=536870912")

	var body ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(got.Event.Body), &body))
	require.Equal(t, "20260728T010203-reuse", body.AttemptID)
	require.Equal(t, 1, body.SlotHitCount)
	require.Equal(t, 0, body.SlotMissCount)
	require.Equal(t, int64(8400), body.TimeSavedMS)
	require.Equal(t, int64(512<<20), body.BytesSaved)
}
