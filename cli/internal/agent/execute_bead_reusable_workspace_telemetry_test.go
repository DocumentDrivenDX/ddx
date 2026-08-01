package agent

import (
	"encoding/json"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
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

func TestAttemptWorkspaceReuseTelemetryRecordsZeroSavingsForColdStarts(t *testing.T) {
	root := t.TempDir()
	disabled := false
	maxSlots := 1
	pool := NewAttemptWorkspaceSlotPool(&config.ReusableWorkspaceConfig{
		Enabled:  &disabled,
		MaxSlots: &maxSlots,
	}).withRoot(root)

	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   "/proj/cold-start",
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "w0",
		TrustBoundary: "default",
	}
	slot, err := pool.Allocate(key)
	require.NoError(t, err)
	require.False(t, slot.Pooled)
	t.Cleanup(func() { _ = pool.Release(slot) })

	telemetry := reusableWorkspaceTelemetryForSlot(slot)
	require.NotNil(t, telemetry)
	require.Zero(t, telemetry.SlotHitCount)
	require.Equal(t, 1, telemetry.SlotMissCount)
	require.Zero(t, telemetry.TimeSavedMS)
	require.Zero(t, telemetry.BytesSaved)

	app := &stubBeadEventAppender{}
	appendReusableWorkspaceTelemetry(app, "ddx-cold", *telemetry)

	require.Len(t, app.events, 1)
	got := app.events[0]
	require.Equal(t, "reusable-workspace", got.Event.Kind)
	require.Equal(t, "slot_hit_count=0 slot_miss_count=1 time_saved=0 bytes_saved=0", got.Event.Summary)

	var body ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(got.Event.Body), &body))
	require.Equal(t, 0, body.SlotHitCount)
	require.Equal(t, 1, body.SlotMissCount)
	require.Equal(t, int64(0), body.TimeSavedMS)
	require.Equal(t, int64(0), body.BytesSaved)
}

func TestAttemptWorkspaceReuseTelemetryRecordsZeroSavingsForMisses(t *testing.T) {
	root := t.TempDir()
	enabled := true
	maxSlots := 1
	pool := NewAttemptWorkspaceSlotPool(&config.ReusableWorkspaceConfig{
		Enabled:  &enabled,
		MaxSlots: &maxSlots,
	}).withRoot(root)

	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   "/proj/reuse-miss",
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "w0",
		TrustBoundary: "default",
	}
	pooled, err := pool.Allocate(key)
	require.NoError(t, err)
	require.True(t, pooled.Pooled)
	t.Cleanup(func() { _ = pool.Release(pooled) })

	miss, err := pool.Allocate(key)
	require.NoError(t, err)
	require.False(t, miss.Pooled)
	t.Cleanup(func() { _ = pool.Release(miss) })

	telemetry := reusableWorkspaceTelemetryForSlot(miss)
	require.NotNil(t, telemetry)
	require.Zero(t, telemetry.SlotHitCount)
	require.Equal(t, 1, telemetry.SlotMissCount)
	require.Zero(t, telemetry.TimeSavedMS)
	require.Zero(t, telemetry.BytesSaved)

	app := &stubBeadEventAppender{}
	appendReusableWorkspaceTelemetry(app, "ddx-miss", *telemetry)

	require.Len(t, app.events, 1)
	got := app.events[0]
	require.Equal(t, "reusable-workspace", got.Event.Kind)
	require.Equal(t, "slot_hit_count=0 slot_miss_count=1 time_saved=0 bytes_saved=0", got.Event.Summary)

	var body ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(got.Event.Body), &body))
	require.Equal(t, 0, body.SlotHitCount)
	require.Equal(t, 1, body.SlotMissCount)
	require.Equal(t, int64(0), body.TimeSavedMS)
	require.Equal(t, int64(0), body.BytesSaved)
}
