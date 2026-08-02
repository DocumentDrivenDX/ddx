package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
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

func TestAttemptWorkspaceReuseCombinedTelemetryReusedEventName(t *testing.T) {
	projectRoot := setupArtifactTestProjectRoot(t)
	const beadID = "ddx-int-0001"
	baseRev := "bbbb000000000001"
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
		AttemptBackend: AttemptBackendLocalClone,
	})

	app := &stubBeadEventAppender{}
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		WorkerID:   "worker-slot-a",
		BeadEvents: app,
		AgentRunner: &artifactTestAgentRunner{
			result: &Result{
				ExitCode: 1,
				Error:    "simulated reused attempt",
			},
		},
		ReusableWorkspaceTelemetry: &ReusableWorkspaceTelemetry{
			SlotHitCount:  1,
			SlotMissCount: 0,
			TimeSavedMS:   8400,
			BytesSaved:    512 << 20,
		},
		AttemptBackend: &reusableWorkspaceTelemetryPrepBackend{
			inner: WorktreeAttemptBackend{},
			slot: &AttemptWorkspaceSlot{
				Pooled:                  true,
				SlotHitCount:            1,
				SlotMissCount:           0,
				ConservativeTimeSavedMS: 8400,
				ConservativeBytesSaved:  512 << 20,
			},
			telemetry: &AttemptWorkspaceReuseTelemetryInput{
				SlotHitCount:  1,
				SlotMissCount: 0,
				TimeSavedMS:   8400,
				BytesSaved:    512 << 20,
			},
		},
		FromRev: baseRev,
	}, &artifactTestGitOps{
		projectRoot: projectRoot,
		baseRev:     baseRev,
		resultRev:   baseRev,
		wtSetupFn: func(wtPath string) {
			setupArtifactTestWorktree(t, wtPath, beadID, "", false, 0)
		},
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	var evt bead.BeadEvent
	found := false
	for _, recorded := range app.events {
		if recorded.Event.Kind != "reusable-workspace" {
			continue
		}
		evt = recorded.Event
		found = true
		break
	}
	require.True(t, found, "expected a reusable-workspace event in the execute-bead telemetry stream")
	require.Equal(t, "reusable-workspace", evt.Kind)
	require.Contains(t, evt.Summary, "slot_hit_count=1")
	require.Contains(t, evt.Summary, "slot_miss_count=0")
	require.Contains(t, evt.Summary, "time_saved_ms=8400")
	require.Contains(t, evt.Summary, "bytes_saved=536870912")
	require.NotContains(t, evt.Summary, "outcome=")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
	require.ElementsMatch(t, []string{
		"bytes_saved",
		"slot_hit_count",
		"slot_miss_count",
		"time_saved_ms",
	}, telemetryBodyKeys(t, parsed))
	require.Equal(t, float64(1), parsed["slot_hit_count"])
	require.Equal(t, float64(0), parsed["slot_miss_count"])
	require.Equal(t, float64(8400), parsed["time_saved_ms"])
	require.Equal(t, float64(512<<20), parsed["bytes_saved"])
	require.NotContains(t, parsed, "outcome")
	require.NotContains(t, parsed, "project_root")
	require.NotContains(t, parsed, "worker_slot")
	require.NotContains(t, parsed, "resolved_backend_policy")
	require.NotContains(t, parsed, "attempt_id")
}

func TestAttemptWorkspaceReuseTelemetryRecordsHitsMisses(t *testing.T) {
	const beadID = "ddx-int-0001"
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})

	testCases := []struct {
		name      string
		slot      *AttemptWorkspaceSlot
		telemetry *AttemptWorkspaceReuseTelemetryInput
		want      string
	}{
		{
			name: "hit",
			slot: &AttemptWorkspaceSlot{
				Pooled:        true,
				SlotHitCount:  1,
				SlotMissCount: 0,
			},
			telemetry: &AttemptWorkspaceReuseTelemetryInput{
				SlotHitCount:  1,
				SlotMissCount: 0,
			},
			want: AttemptWorkspaceReuseOutcomeHit,
		},
		{
			name: "miss",
			slot: &AttemptWorkspaceSlot{
				Pooled:        true,
				SlotHitCount:  0,
				SlotMissCount: 1,
			},
			telemetry: &AttemptWorkspaceReuseTelemetryInput{
				SlotHitCount:  0,
				SlotMissCount: 1,
			},
			want: AttemptWorkspaceReuseOutcomeMiss,
		},
		{
			name: "cold_start",
			telemetry: &AttemptWorkspaceReuseTelemetryInput{
				SlotHitCount:  0,
				SlotMissCount: 1,
			},
			want: AttemptWorkspaceReuseOutcomeColdStart,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			projectRoot, baseRev := newScriptHarnessRepo(t, 1)
			app := &stubBeadEventAppender{}
			backend := &reusableWorkspaceTelemetryCleanupCountingBackend{
				reusableWorkspaceTelemetryPrepBackend: reusableWorkspaceTelemetryPrepBackend{
					slot:      tc.slot,
					telemetry: tc.telemetry,
				},
			}

			res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
				WorkerID:       "worker-slot-a",
				BeadEvents:     app,
				AgentRunner:    scriptHarnessAgentRunner{},
				FromRev:        baseRev,
				AttemptBackend: backend,
			}, &RealGitOps{})
			require.NoError(t, err)
			require.NotNil(t, res)

			var reuseEvents []bead.BeadEvent
			for _, recorded := range app.events {
				if recorded.Event.Kind == "reusable-workspace" {
					reuseEvents = append(reuseEvents, recorded.Event)
				}
			}
			require.Len(t, reuseEvents, 1, "expected exactly one reusable-workspace record for %s", tc.name)

			evt := reuseEvents[0]
			require.Contains(t, evt.Summary, "slot_hit_count=")
			require.Contains(t, evt.Summary, "slot_miss_count=")
			require.Contains(t, evt.Summary, "time_saved_ms=")
			require.Contains(t, evt.Summary, "bytes_saved=")
			require.NotContains(t, evt.Summary, "outcome=")

			var parsed map[string]any
			require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
			require.ElementsMatch(t, []string{
				"bytes_saved",
				"slot_hit_count",
				"slot_miss_count",
				"time_saved_ms",
			}, telemetryBodyKeys(t, parsed))
			require.Equal(t, float64(tc.telemetry.SlotHitCount), parsed["slot_hit_count"])
			require.Equal(t, float64(tc.telemetry.SlotMissCount), parsed["slot_miss_count"])
			require.Equal(t, float64(tc.telemetry.TimeSavedMS), parsed["time_saved_ms"])
			require.Equal(t, float64(tc.telemetry.BytesSaved), parsed["bytes_saved"])
			require.NotContains(t, parsed, "outcome")
			require.NotContains(t, parsed, "project_root")
			require.NotContains(t, parsed, "worker_slot")
			require.NotContains(t, parsed, "resolved_backend_policy")
			require.NotContains(t, parsed, "attempt_id")
		})
	}
}

type reusableWorkspaceTelemetryPrepBackend struct {
	inner     AttemptBackend
	slot      *AttemptWorkspaceSlot
	telemetry *AttemptWorkspaceReuseTelemetryInput
}

func (b *reusableWorkspaceTelemetryPrepBackend) Name() string { return AttemptBackendLocalClone }

func (b *reusableWorkspaceTelemetryPrepBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	inner := b.inner
	if inner == nil {
		inner = LocalCloneAttemptBackend{}
	}
	ws, err := inner.Prepare(ctx, req)
	if err != nil {
		return nil, err
	}
	if b.slot != nil {
		slot := *b.slot
		if slot.Path == "" {
			slot.Path = ws.WorkDir
		}
		ws.ReusableSlot = &slot
	}
	if b.telemetry != nil {
		ws.ReusableTelemetry = b.telemetry
	}
	return ws, nil
}

func (b *reusableWorkspaceTelemetryPrepBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	inner := b.inner
	if inner == nil {
		inner = LocalCloneAttemptBackend{}
	}
	return inner.Run(ctx, req)
}

func (b *reusableWorkspaceTelemetryPrepBackend) ImportCandidate(context.Context, *AttemptWorkspace, *ExecuteBeadResult) error {
	return nil
}

func (b *reusableWorkspaceTelemetryPrepBackend) ReleaseCandidateImport(context.Context, *AttemptWorkspace) error {
	return nil
}

func (b *reusableWorkspaceTelemetryPrepBackend) PublishResult(context.Context, *AttemptWorkspace, *ExecuteBeadResult) error {
	return nil
}

func (b *reusableWorkspaceTelemetryPrepBackend) Cleanup(ctx context.Context, ws *AttemptWorkspace) error {
	inner := b.inner
	if inner == nil {
		inner = LocalCloneAttemptBackend{}
	}
	return inner.Cleanup(ctx, ws)
}

type reusableWorkspaceTelemetryCleanupCountingBackend struct {
	reusableWorkspaceTelemetryPrepBackend
	releaseCalls    int
	quarantineCalls int
	releaseErr      error
	quarantineErr   error
}

func (b *reusableWorkspaceTelemetryCleanupCountingBackend) Release(ctx context.Context, ws *AttemptWorkspace) error {
	b.releaseCalls++
	if b.releaseErr != nil {
		return b.releaseErr
	}
	return finalizeReusableAttemptWorkspace(ctx, AttemptBackendLocalClone, ws, true)
}

func (b *reusableWorkspaceTelemetryCleanupCountingBackend) Quarantine(ctx context.Context, ws *AttemptWorkspace) error {
	b.quarantineCalls++
	if b.quarantineErr != nil {
		return b.quarantineErr
	}
	return finalizeReusableAttemptWorkspace(ctx, AttemptBackendLocalClone, ws, false)
}

// TestAttemptWorkspaceReuseTelemetryRecordsReuseHitAndNonZeroSavings proves a
// reused-attempt combined telemetry event carries hit allocation counts and
// non-zero savings when the savings estimate provides proven preserved state.
func TestAttemptWorkspaceReuseTelemetryRecordsReuseHitAndNonZeroSavings(t *testing.T) {
	const beadID = "ddx-int-0001"

	telemetry := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:                     1,
			ConservativeTimeSavedMS:          8400,
			ConservativeBytesSaved:           512 << 20,
			ProvenPreservedProjectLocalState: true,
		},
	)
	require.Equal(t, 1, telemetry.SlotHitCount)
	require.Zero(t, telemetry.SlotMissCount)
	require.Equal(t, int64(8400), telemetry.TimeSavedMS)
	require.Equal(t, int64(512<<20), telemetry.BytesSaved)

	app := &stubBeadEventAppender{}
	contract := reusableWorkspaceTelemetryEventContract(
		&AttemptWorkspace{
			ReusableSlot: &AttemptWorkspaceSlot{
				Pooled:                  true,
				SlotHitCount:            telemetry.SlotHitCount,
				SlotMissCount:           telemetry.SlotMissCount,
				ConservativeTimeSavedMS: telemetry.TimeSavedMS,
				ConservativeBytesSaved:  telemetry.BytesSaved,
			},
		},
		&ReusableWorkspaceTelemetry{
			SlotHitCount:  telemetry.SlotHitCount,
			SlotMissCount: telemetry.SlotMissCount,
			TimeSavedMS:   telemetry.TimeSavedMS,
			BytesSaved:    telemetry.BytesSaved,
		},
		"/tmp/project-root",
		"worker-slot-a",
		AttemptBackendLocalClone,
		"20260801T010203-reuse",
	)
	appendAttemptWorkspaceReuseTelemetry(app, beadID, contract)

	require.Len(t, app.events, 1)
	evt := app.events[0].Event
	require.Equal(t, "reusable-workspace", evt.Kind)
	require.Contains(t, evt.Summary, "outcome=hit")
	require.Contains(t, evt.Summary, "slot_hit_count=1")
	require.Contains(t, evt.Summary, "slot_miss_count=0")
	require.Contains(t, evt.Summary, "time_saved_ms=8400")
	require.Contains(t, evt.Summary, "bytes_saved=536870912")

	var parsed AttemptWorkspaceReuseTelemetryEventContract
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
	require.Equal(t, AttemptWorkspaceReuseOutcomeHit, parsed.Outcome)
	require.Equal(t, "/tmp/project-root", parsed.ProjectRoot)
	require.Equal(t, "worker-slot-a", parsed.WorkerSlot)
	require.Equal(t, AttemptBackendLocalClone, parsed.ResolvedBackendPolicy)
	require.Equal(t, "20260801T010203-reuse", parsed.AttemptID)
	require.Equal(t, 1, parsed.SlotHitCount)
	require.Zero(t, parsed.SlotMissCount)
	require.Equal(t, int64(8400), parsed.TimeSavedMS)
	require.Equal(t, int64(512<<20), parsed.BytesSaved)
}

func TestAttemptWorkspaceReuseCombinedTelemetryColdStartValues(t *testing.T) {
	app := &stubBeadEventAppender{}
	body := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ReusableSlot: &AttemptWorkspaceSlot{
				Pooled: false,
			},
		},
		nil,
	)
	require.NotNil(t, body)
	body.AttemptID = "20260801T010203-cold"

	appendReusableWorkspaceTelemetry(app, "ddx-int-0001", *body)

	require.Len(t, app.events, 1)
	evt := app.events[0].Event
	require.Equal(t, "reusable-workspace", evt.Kind)
	require.Equal(t, "slot_hit_count=0 slot_miss_count=1 time_saved_ms=0 bytes_saved=0", evt.Summary)

	var parsed ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
	require.Equal(t, "20260801T010203-cold", parsed.AttemptID)
	require.Zero(t, parsed.SlotHitCount)
	require.Equal(t, 1, parsed.SlotMissCount)
	require.Zero(t, parsed.TimeSavedMS)
	require.Zero(t, parsed.BytesSaved)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &got))
	require.Len(t, got, 5)
	for _, key := range []string{"attempt_id", "slot_hit_count", "slot_miss_count", "time_saved_ms", "bytes_saved"} {
		require.Contains(t, got, key)
	}
}

func TestAttemptWorkspaceReuseCombinedTelemetryReusedValues(t *testing.T) {
	telemetry := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:                     1,
			ConservativeTimeSavedMS:          8400,
			ConservativeBytesSaved:           512 << 20,
			ProvenPreservedProjectLocalState: true,
		},
	)
	combined := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ReusableSlot: &AttemptWorkspaceSlot{
				Pooled:                  true,
				SlotHitCount:            telemetry.SlotHitCount,
				ConservativeTimeSavedMS: telemetry.TimeSavedMS,
				ConservativeBytesSaved:  telemetry.BytesSaved,
			},
			ReusableTelemetry: &telemetry,
		},
		nil,
	)

	require.NotNil(t, combined)
	require.Equal(t, 1, combined.SlotHitCount)
	require.Zero(t, combined.SlotMissCount)
	require.Equal(t, int64(8400), combined.TimeSavedMS)
	require.Equal(t, int64(512<<20), combined.BytesSaved)
	combined.AttemptID = "ddx-int-0001"

	res := &ExecuteBeadResult{
		BeadID:  "ddx-int-0001",
		Status:  ExecuteBeadStatusNoChanges,
		BaseRev: "base-rev",
	}
	applyReusableWorkspaceTelemetry(res, combined)
	report := ReportFromExecuteBeadResult(res, "")
	event := executeBeadLoopEvent(report, "worker", time.Unix(0, 0).UTC())
	require.Contains(t, event.Body, "reusable_workspace_slot_hits=1")
	require.Contains(t, event.Body, "reusable_workspace_slot_misses=0")
	require.Contains(t, event.Body, "reusable_workspace_time_saved_ms=8400")
	require.Contains(t, event.Body, "reusable_workspace_bytes_saved=536870912")

	app := &stubBeadEventAppender{}
	appendReusableWorkspaceTelemetry(app, "ddx-int-0001", *combined)
	require.Len(t, app.events, 1)
	evt := app.events[0].Event
	require.Equal(t, "reusable-workspace", evt.Kind)
	require.Contains(t, evt.Summary, "slot_hit_count=1")
	require.Contains(t, evt.Summary, "slot_miss_count=0")
	require.Contains(t, evt.Summary, "time_saved_ms=8400")
	require.Contains(t, evt.Summary, "bytes_saved=536870912")

	var parsed ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
	require.Equal(t, "ddx-int-0001", parsed.AttemptID)
	require.Equal(t, 1, parsed.SlotHitCount)
	require.Zero(t, parsed.SlotMissCount)
	require.Equal(t, int64(8400), parsed.TimeSavedMS)
	require.Equal(t, int64(512<<20), parsed.BytesSaved)
}

func TestAttemptWorkspaceReuseTelemetryDoesNotDoubleCountCleanup(t *testing.T) {
	const beadID = "ddx-int-0001"
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
		AttemptBackend: AttemptBackendLocalClone,
	})

	testCases := []struct {
		name        string
		slot        *AttemptWorkspaceSlot
		telemetry   *AttemptWorkspaceReuseTelemetryInput
		wantOutcome string
	}{
		{
			name: "hit",
			slot: &AttemptWorkspaceSlot{
				Pooled:        true,
				SlotHitCount:  1,
				SlotMissCount: 0,
			},
			telemetry: &AttemptWorkspaceReuseTelemetryInput{
				SlotHitCount:  1,
				SlotMissCount: 0,
			},
			wantOutcome: AttemptWorkspaceReuseOutcomeHit,
		},
		{
			name: "miss",
			slot: &AttemptWorkspaceSlot{
				Pooled:        true,
				SlotHitCount:  0,
				SlotMissCount: 1,
			},
			telemetry: &AttemptWorkspaceReuseTelemetryInput{
				SlotHitCount:  0,
				SlotMissCount: 1,
			},
			wantOutcome: AttemptWorkspaceReuseOutcomeMiss,
		},
		{
			name: "cold_start",
			slot: &AttemptWorkspaceSlot{
				Pooled:        false,
				SlotHitCount:  0,
				SlotMissCount: 0,
			},
			wantOutcome: AttemptWorkspaceReuseOutcomeColdStart,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			projectRoot, baseRev := newScriptHarnessRepo(t, 1)
			app := &stubBeadEventAppender{}
			backend := &reusableWorkspaceTelemetryCleanupCountingBackend{
				reusableWorkspaceTelemetryPrepBackend: reusableWorkspaceTelemetryPrepBackend{
					slot:      tc.slot,
					telemetry: tc.telemetry,
				},
			}

			res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
				WorkerID:       "worker-slot-a",
				BeadEvents:     app,
				AgentRunner:    scriptHarnessAgentRunner{},
				FromRev:        baseRev,
				AttemptBackend: backend,
			}, &RealGitOps{})
			require.NoError(t, err)
			require.NotNil(t, res)

			var reuseEvents []bead.BeadEvent
			for _, recorded := range app.events {
				if recorded.Event.Kind == "reusable-workspace" {
					reuseEvents = append(reuseEvents, recorded.Event)
				}
			}
			require.Len(t, reuseEvents, 1, "cleanup should not append a second reusable-workspace event for %s", tc.name)

			evt := reuseEvents[0]
			require.Contains(t, evt.Summary, "slot_hit_count=")
			require.Contains(t, evt.Summary, "slot_miss_count=")
			require.Contains(t, evt.Summary, "time_saved_ms=")
			require.Contains(t, evt.Summary, "bytes_saved=")
			require.NotContains(t, evt.Summary, "outcome=")

			var parsed map[string]any
			require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
			require.ElementsMatch(t, []string{
				"bytes_saved",
				"slot_hit_count",
				"slot_miss_count",
				"time_saved_ms",
			}, telemetryBodyKeys(t, parsed))
			switch tc.wantOutcome {
			case AttemptWorkspaceReuseOutcomeHit:
				require.Equal(t, float64(1), parsed["slot_hit_count"])
				require.Equal(t, float64(0), parsed["slot_miss_count"])
			case AttemptWorkspaceReuseOutcomeMiss:
				require.Equal(t, float64(0), parsed["slot_hit_count"])
				require.Equal(t, float64(1), parsed["slot_miss_count"])
			case AttemptWorkspaceReuseOutcomeColdStart:
				require.Equal(t, float64(0), parsed["slot_hit_count"])
				require.Equal(t, float64(1), parsed["slot_miss_count"])
			default:
				t.Fatalf("unexpected outcome %q", tc.wantOutcome)
			}
			require.Equal(t, float64(0), parsed["time_saved_ms"])
			require.Equal(t, float64(0), parsed["bytes_saved"])
		})
	}
}

func TestAttemptWorkspaceReuseTelemetryDoesNotDoubleCountFailedCleanup(t *testing.T) {
	const beadID = "ddx-int-0001"
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
		AttemptBackend: AttemptBackendLocalClone,
	})

	testCases := []struct {
		name           string
		result         *Result
		backend        *reusableWorkspaceTelemetryCleanupCountingBackend
		wantRelease    int
		wantQuarantine int
	}{
		{
			name: "failed_attempt_quarantines_once",
			result: &Result{
				ExitCode: 1,
				Error:    "simulated attempt failure",
			},
			backend: &reusableWorkspaceTelemetryCleanupCountingBackend{
				reusableWorkspaceTelemetryPrepBackend: reusableWorkspaceTelemetryPrepBackend{
					slot: &AttemptWorkspaceSlot{
						Pooled:       true,
						SlotHitCount: 1,
					},
					telemetry: &AttemptWorkspaceReuseTelemetryInput{
						SlotHitCount: 1,
					},
				},
			},
			wantRelease:    0,
			wantQuarantine: 1,
		},
		{
			name: "cleanup_error_does_not_duplicate_event",
			result: &Result{
				ExitCode: 1,
				Error:    "simulated attempt failure with cleanup error",
			},
			backend: &reusableWorkspaceTelemetryCleanupCountingBackend{
				reusableWorkspaceTelemetryPrepBackend: reusableWorkspaceTelemetryPrepBackend{
					slot: &AttemptWorkspaceSlot{
						Pooled:       true,
						SlotHitCount: 1,
					},
					telemetry: &AttemptWorkspaceReuseTelemetryInput{
						SlotHitCount: 1,
					},
				},
				quarantineErr: errors.New("simulated quarantine failure"),
			},
			wantRelease:    0,
			wantQuarantine: 1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			projectRoot, baseRev := newScriptHarnessRepo(t, 1)
			app := &stubBeadEventAppender{}

			res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
				WorkerID:       "worker-slot-a",
				BeadEvents:     app,
				AgentRunner:    &artifactTestAgentRunner{result: tc.result},
				FromRev:        baseRev,
				AttemptBackend: tc.backend,
			}, &RealGitOps{})
			require.NoError(t, err)
			require.NotNil(t, res)
			require.Equal(t, tc.wantRelease, tc.backend.releaseCalls)
			require.Equal(t, tc.wantQuarantine, tc.backend.quarantineCalls)

			var reuseEvents []bead.BeadEvent
			for _, recorded := range app.events {
				if recorded.Event.Kind == "reusable-workspace" {
					reuseEvents = append(reuseEvents, recorded.Event)
				}
			}
			require.Len(t, reuseEvents, 1, "cleanup must not append a second reusable-workspace event for %s", tc.name)

			evt := reuseEvents[0]
			require.Contains(t, evt.Summary, "slot_hit_count=1")
			require.Contains(t, evt.Summary, "slot_miss_count=0")
			require.Contains(t, evt.Summary, "time_saved_ms=")
			require.Contains(t, evt.Summary, "bytes_saved=")
			require.NotContains(t, evt.Summary, "outcome=")

			var parsed map[string]any
			require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
			require.Equal(t, float64(1), parsed["slot_hit_count"])
			require.Equal(t, float64(0), parsed["slot_miss_count"])
			require.Equal(t, float64(0), parsed["time_saved_ms"])
			require.Equal(t, float64(0), parsed["bytes_saved"])
		})
	}
}

func TestAttemptWorkspaceReuseTelemetryUsesSamePayloadShapeForReuseAndColdStart(t *testing.T) {
	const attemptID = "20260801T010203-shape"
	fallback := &ReusableWorkspaceTelemetry{
		AttemptID:     attemptID,
		SlotHitCount:  1,
		SlotMissCount: 0,
		TimeSavedMS:   4200,
		BytesSaved:    256 << 20,
	}

	reused := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ReusableSlot: &AttemptWorkspaceSlot{
				Pooled:       true,
				SlotHitCount: 1,
			},
		},
		fallback,
	)
	cold := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ReusableSlot: &AttemptWorkspaceSlot{
				Pooled: false,
			},
		},
		fallback,
	)

	require.NotNil(t, reused)
	require.NotNil(t, cold)
	require.Equal(t, attemptID, fallback.AttemptID)

	reused.AttemptID = attemptID
	cold.AttemptID = attemptID

	reusedApp := &stubBeadEventAppender{}
	coldApp := &stubBeadEventAppender{}
	appendReusableWorkspaceTelemetry(reusedApp, "ddx-int-reused", *reused)
	appendReusableWorkspaceTelemetry(coldApp, "ddx-int-cold", *cold)

	require.Len(t, reusedApp.events, 1)
	require.Len(t, coldApp.events, 1)

	var reusedBody, coldBody map[string]any
	require.NoError(t, json.Unmarshal([]byte(reusedApp.events[0].Event.Body), &reusedBody))
	require.NoError(t, json.Unmarshal([]byte(coldApp.events[0].Event.Body), &coldBody))
	require.Len(t, reusedBody, len(coldBody))
	require.ElementsMatch(t, telemetryBodyKeys(t, reusedBody), telemetryBodyKeys(t, coldBody))

	require.Equal(t, float64(1), reusedBody["slot_hit_count"])
	require.Equal(t, float64(0), reusedBody["slot_miss_count"])
	require.Equal(t, float64(4200), reusedBody["time_saved_ms"])
	require.Equal(t, float64(256<<20), reusedBody["bytes_saved"])

	require.Equal(t, float64(0), coldBody["slot_hit_count"])
	require.Equal(t, float64(1), coldBody["slot_miss_count"])
	require.Equal(t, float64(0), coldBody["time_saved_ms"])
	require.Equal(t, float64(0), coldBody["bytes_saved"])
}

func TestAttemptWorkspaceReuseTelemetryEventContract(t *testing.T) {
	app := &stubBeadEventAppender{}

	cases := []AttemptWorkspaceReuseTelemetryEventContract{
		{
			Outcome:               AttemptWorkspaceReuseOutcomeHit,
			ProjectRoot:           "/proj/a",
			WorkerSlot:            "worker-a",
			ResolvedBackendPolicy: AttemptBackendLocalClone,
			AttemptID:             "20260801T010203-hit",
			SlotHitCount:          1,
		},
		{
			Outcome:               AttemptWorkspaceReuseOutcomeMiss,
			ProjectRoot:           "/proj/b",
			WorkerSlot:            "worker-b",
			ResolvedBackendPolicy: AttemptBackendWorktree,
			AttemptID:             "20260801T010203-miss",
			SlotMissCount:         1,
		},
		{
			Outcome:               AttemptWorkspaceReuseOutcomeColdStart,
			ProjectRoot:           "/proj/c",
			WorkerSlot:            "worker-c",
			ResolvedBackendPolicy: AttemptBackendDockerClone,
			AttemptID:             "20260801T010203-cold",
			SlotMissCount:         1,
		},
	}

	for _, contract := range cases {
		appendAttemptWorkspaceReuseTelemetry(app, "ddx-int-0001", contract)
	}

	require.Len(t, app.events, len(cases))
	for i, want := range cases {
		got := app.events[i].Event
		require.Equal(t, "reusable-workspace", got.Kind)
		require.Contains(t, got.Summary, "outcome="+want.Outcome)
		require.Contains(t, got.Summary, "slot_hit_count=")
		require.Contains(t, got.Summary, "slot_miss_count=")
		require.Contains(t, got.Summary, "time_saved_ms=")
		require.Contains(t, got.Summary, "bytes_saved=")

		var parsed AttemptWorkspaceReuseTelemetryEventContract
		require.NoError(t, json.Unmarshal([]byte(got.Body), &parsed))
		require.Equal(t, want.Outcome, parsed.Outcome)
		require.Equal(t, want.ProjectRoot, parsed.ProjectRoot)
		require.Equal(t, want.WorkerSlot, parsed.WorkerSlot)
		require.Equal(t, want.ResolvedBackendPolicy, parsed.ResolvedBackendPolicy)
		require.Equal(t, want.AttemptID, parsed.AttemptID)
		require.Equal(t, want.SlotHitCount, parsed.SlotHitCount)
		require.Equal(t, want.SlotMissCount, parsed.SlotMissCount)
		require.Equal(t, want.TimeSavedMS, parsed.TimeSavedMS)
		require.Equal(t, want.BytesSaved, parsed.BytesSaved)
	}
}

func telemetryBodyKeys(t *testing.T, body map[string]any) []string {
	t.Helper()
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	return keys
}
