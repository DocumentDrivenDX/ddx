package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAttemptWorkspaceReuseTelemetryRecordsReuseSavingsFromAllocationOutcome(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	beadID := "ddx-int-0001"

	backend := &recordingReusableSlotBackend{
		inner: LocalCloneAttemptBackend{},
		prepareHook: func(ws *AttemptWorkspace) {
			ws.ReusableSlot = &AttemptWorkspaceSlot{
				Pooled:                  true,
				SlotHitCount:            1,
				SlotMissCount:           0,
				ConservativeTimeSavedMS: 8400,
				ConservativeBytesSaved:  512 << 20,
			}
		},
		runFunc: func(context.Context, AttemptBackendRunRequest) (*Result, error) {
			return &Result{Harness: "script", ExitCode: 0}, nil
		},
	}
	app := &stubBeadEventAppender{}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
		AttemptBackend: AttemptBackendLocalClone,
	})

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AttemptBackend: backend,
		BeadEvents:     app,
		NoReview:       true,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)

	var got *ReusableWorkspaceTelemetry
	for i := range app.events {
		if app.events[i].Event.Kind != "reusable-workspace" {
			continue
		}
		require.Nil(t, got, "reuse telemetry should be emitted once")
		body := app.events[i].Event.Body
		var parsed ReusableWorkspaceTelemetry
		require.NoError(t, json.Unmarshal([]byte(body), &parsed))
		got = &parsed
		require.Equal(t, "ddx", app.events[i].Event.Actor)
		require.Equal(t, "legacy agent execute-bead", app.events[i].Event.Source)
		require.Contains(t, app.events[i].Event.Summary, "slot_hit_count=1")
		require.Contains(t, app.events[i].Event.Summary, "slot_miss_count=0")
		require.Contains(t, app.events[i].Event.Summary, "time_saved=8400")
		require.Contains(t, app.events[i].Event.Summary, "bytes_saved=536870912")
	}

	require.NotNil(t, got, "expected reusable-workspace telemetry event")
	require.Equal(t, int64(8400), got.TimeSavedMS)
	require.Equal(t, int64(512<<20), got.BytesSaved)
	require.Equal(t, 1, got.SlotHitCount)
	require.Equal(t, 0, got.SlotMissCount)
	require.NotEmpty(t, got.AttemptID)
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
