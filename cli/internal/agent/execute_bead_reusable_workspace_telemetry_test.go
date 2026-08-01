package agent

import (
	"encoding/json"
	"testing"
	"time"

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

func TestAttemptWorkspaceReuseTelemetryScopesByProjectAndWorkerSlot(t *testing.T) {
	app := &stubBeadEventAppender{}
	body := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ProjectRoot: "/repo/project-a",
			AttemptID:   "20260801T010203-reuse",
			ReusableSlot: &AttemptWorkspaceSlot{
				Key: AttemptWorkspaceSlotKey{
					ProjectRoot: "/repo/project-a",
					Backend:     AttemptBackendLocalClone,
					WorkerSlot:  "worker-a",
				},
				Pooled:                  true,
				SlotHitCount:            1,
				SlotMissCount:           0,
				ConservativeTimeSavedMS: 8400,
				ConservativeBytesSaved:  512 << 20,
			},
		},
		nil,
	)

	require.NotNil(t, body)
	appendReusableWorkspaceTelemetry(app, "ddx-int-0001", *body)

	require.Len(t, app.events, 1)
	evt := app.events[0].Event
	require.Equal(t, "reusable-workspace", evt.Kind)
	require.Contains(t, evt.Summary, "project_root=/repo/project-a")
	require.Contains(t, evt.Summary, "worker_slot=worker-a")
	require.Contains(t, evt.Summary, "resolved_backend_policy=local-clone")
	require.Contains(t, evt.Summary, "attempt_id=20260801T010203-reuse")

	var parsed ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
	require.Equal(t, "/repo/project-a", parsed.ProjectRoot)
	require.Equal(t, "worker-a", parsed.WorkerSlot)
	require.Equal(t, AttemptBackendLocalClone, parsed.ResolvedBackendPolicy)
	require.Equal(t, "20260801T010203-reuse", parsed.AttemptID)
	require.Equal(t, 1, parsed.SlotHitCount)
	require.Equal(t, 0, parsed.SlotMissCount)
	require.Equal(t, int64(8400), parsed.TimeSavedMS)
	require.Equal(t, int64(512<<20), parsed.BytesSaved)
}

func TestAttemptWorkspaceReuseTelemetryScopesColdStarts(t *testing.T) {
	app := &stubBeadEventAppender{}
	body := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ProjectRoot: "/repo/project-b",
			AttemptID:   "20260801T010204-cold",
			ReusableSlot: &AttemptWorkspaceSlot{
				Key: AttemptWorkspaceSlotKey{
					ProjectRoot: "/repo/project-b",
					Backend:     AttemptBackendLocalClone,
					WorkerSlot:  "worker-b",
				},
				Pooled:        false,
				SlotHitCount:  0,
				SlotMissCount: 1,
			},
		},
		nil,
	)

	require.NotNil(t, body)
	appendReusableWorkspaceTelemetry(app, "ddx-int-0002", *body)

	require.Len(t, app.events, 1)
	evt := app.events[0].Event
	require.Equal(t, "reusable-workspace", evt.Kind)
	require.Contains(t, evt.Summary, "project_root=/repo/project-b")
	require.Contains(t, evt.Summary, "worker_slot=worker-b")
	require.Contains(t, evt.Summary, "resolved_backend_policy=local-clone")
	require.Contains(t, evt.Summary, "attempt_id=20260801T010204-cold")
	require.Contains(t, evt.Summary, "slot_hit_count=0")
	require.Contains(t, evt.Summary, "slot_miss_count=1")

	var parsed ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
	require.Equal(t, "/repo/project-b", parsed.ProjectRoot)
	require.Equal(t, "worker-b", parsed.WorkerSlot)
	require.Equal(t, AttemptBackendLocalClone, parsed.ResolvedBackendPolicy)
	require.Equal(t, "20260801T010204-cold", parsed.AttemptID)
	require.Equal(t, 0, parsed.SlotHitCount)
	require.Equal(t, 1, parsed.SlotMissCount)
	require.Zero(t, parsed.TimeSavedMS)
	require.Zero(t, parsed.BytesSaved)
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

func assertColdStartCombinedReusableWorkspaceTelemetry(t *testing.T, beadID string, slotMisses int) {
	t.Helper()

	report := ExecuteBeadReport{
		BeadID:                       beadID,
		Status:                       ExecuteBeadStatusNoChanges,
		ReusableWorkspaceSlotMisses:  slotMisses,
		ReusableWorkspaceTimeSavedMS: 0,
		ReusableWorkspaceBytesSaved:  0,
	}

	event := executeBeadLoopEvent(report, "worker", time.Unix(0, 0).UTC())
	require.Equal(t, "execute-bead", event.Kind)
	require.Equal(t, "worker", event.Actor)
	require.Equal(t, "ddx work", event.Source)
	require.Contains(t, event.Body, "reusable_workspace_slot_hits=0")
	require.Contains(t, event.Body, "reusable_workspace_slot_misses=1")
	require.Contains(t, event.Body, "reusable_workspace_time_saved_ms=0")
	require.Contains(t, event.Body, "reusable_workspace_bytes_saved=0")
}

func TestAttemptWorkspaceReuseTelemetryRecordsColdStartMissAndZeroSavings(t *testing.T) {
	assertColdStartCombinedReusableWorkspaceTelemetry(t, "ddx-int-0001", 1)
}

func TestAttemptWorkspaceReuseTelemetryPayloadEmitsZeroSavingsForColdStart(t *testing.T) {
	t.Run("cold_start_payload_emits_explicit_zero_savings", func(t *testing.T) {
		app := &stubBeadEventAppender{}
		appendReusableWorkspaceTelemetry(app, "ddx-int-0001", ReusableWorkspaceTelemetry{
			AttemptID: "20260801T010203-cold",
		})

		require.Len(t, app.events, 1)
		evt := app.events[0].Event
		require.Equal(t, "reusable-workspace", evt.Kind)
		require.Contains(t, evt.Summary, "slot_hit_count=0")
		require.Contains(t, evt.Summary, "slot_miss_count=0")
		require.Contains(t, evt.Summary, "time_saved_ms=0")
		require.Contains(t, evt.Summary, "bytes_saved=0")

		var parsed ReusableWorkspaceTelemetry
		require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
		require.Equal(t, "20260801T010203-cold", parsed.AttemptID)
		require.Zero(t, parsed.SlotHitCount)
		require.Zero(t, parsed.SlotMissCount)
		require.Zero(t, parsed.TimeSavedMS)
		require.Zero(t, parsed.BytesSaved)
	})

	t.Run("no_reuse_allocation_outcome_keeps_miss_counter_and_zero_savings", func(t *testing.T) {
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
			AttemptID:     "20260801T010203-miss",
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
		require.Equal(t, "20260801T010203-miss", parsed.AttemptID)
		require.Zero(t, parsed.SlotHitCount)
		require.Equal(t, 1, parsed.SlotMissCount)
		require.Zero(t, parsed.TimeSavedMS)
		require.Zero(t, parsed.BytesSaved)

		report := ExecuteBeadReport{
			BeadID:                       "ddx-int-0001",
			Status:                       ExecuteBeadStatusNoChanges,
			ReusableWorkspaceSlotMisses:  telemetry.SlotMissCount,
			ReusableWorkspaceTimeSavedMS: telemetry.TimeSavedMS,
			ReusableWorkspaceBytesSaved:  telemetry.BytesSaved,
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
	})
}
