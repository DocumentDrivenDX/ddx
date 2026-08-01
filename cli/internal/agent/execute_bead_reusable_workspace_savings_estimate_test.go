package agent

import (
	"context"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
)

type reusableWorkspaceSavingsEstimateBackend struct {
	inner     AttemptBackend
	telemetry *AttemptWorkspaceReuseTelemetryInput
	pooled    bool
}

func (b *reusableWorkspaceSavingsEstimateBackend) Name() string { return AttemptBackendLocalClone }

func (b *reusableWorkspaceSavingsEstimateBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	inner := b.inner
	if inner == nil {
		inner = LocalCloneAttemptBackend{}
	}
	ws, err := inner.Prepare(ctx, req)
	if err != nil {
		return nil, err
	}
	ws.ReusableTelemetry = b.telemetry
	if b.pooled {
		ws.ReusableSlot = &AttemptWorkspaceSlot{Pooled: true, Path: ws.WorkDir}
	} else if b.telemetry == nil {
		ws.ReusableSlot = &AttemptWorkspaceSlot{Pooled: false, Path: ws.WorkDir}
	}
	return ws, nil
}

func (b *reusableWorkspaceSavingsEstimateBackend) Run(context.Context, AttemptBackendRunRequest) (*Result, error) {
	return &Result{
		Harness:  "stub",
		ExitCode: 1,
		Error:    "simulated reusable workspace attempt failure",
	}, nil
}

func (b *reusableWorkspaceSavingsEstimateBackend) ImportCandidate(context.Context, *AttemptWorkspace, *ExecuteBeadResult) error {
	return nil
}

func (b *reusableWorkspaceSavingsEstimateBackend) ReleaseCandidateImport(context.Context, *AttemptWorkspace) error {
	return nil
}

func (b *reusableWorkspaceSavingsEstimateBackend) PublishResult(context.Context, *AttemptWorkspace, *ExecuteBeadResult) error {
	return nil
}

func (b *reusableWorkspaceSavingsEstimateBackend) Cleanup(ctx context.Context, ws *AttemptWorkspace) error {
	inner := b.inner
	if inner == nil {
		inner = LocalCloneAttemptBackend{}
	}
	return inner.Cleanup(ctx, ws)
}

func executeBeadReusableWorkspaceSavingsEstimateTestConfig(t *testing.T) config.ResolvedConfig {
	t.Helper()
	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})
	return cfg
}

func TestAttemptWorkspaceReuseSavingsEstimateFromReusableSlotOutcome(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	telemetry := AttemptWorkspaceReuseSavingsEstimateFromReusableSlotOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:                     1,
			ConservativeTimeSavedMS:          8400,
			ConservativeBytesSaved:           512 << 20,
			ProvenPreservedProjectLocalState: true,
		},
	)
	backend := &reusableWorkspaceSavingsEstimateBackend{
		telemetry: &telemetry,
		pooled:    true,
	}

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, executeBeadReusableWorkspaceSavingsEstimateTestConfig(t), ExecuteBeadRuntime{
		AttemptBackend: backend,
		AgentRunner:    scriptHarnessAgentRunner{},
		FromRev:        baseRev,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 1, res.ReusableWorkspaceSlotHits)
	require.Zero(t, res.ReusableWorkspaceSlotMisses)
	require.Equal(t, int64(8400), res.ReusableWorkspaceTimeSavedMS)
	require.Equal(t, int64(512<<20), res.ReusableWorkspaceBytesSaved)

	report := ReportFromExecuteBeadResult(res, "")
	event := executeBeadLoopEvent(report, "worker", time.Unix(0, 0).UTC())
	require.Contains(t, event.Body, "reusable_workspace_slot_hits=1")
	require.Contains(t, event.Body, "reusable_workspace_slot_misses=0")
	require.Contains(t, event.Body, "reusable_workspace_time_saved_ms=8400")
	require.Contains(t, event.Body, "reusable_workspace_bytes_saved=536870912")
}

func TestAttemptWorkspaceReuseSavingsEstimateDoesNotInventSavingsForColdOrUnprovenSlots(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	t.Run("cold_allocation", func(t *testing.T) {
		telemetry := AttemptWorkspaceReuseSavingsEstimateFromReusableSlotOutcome(
			AttemptWorkspaceReuseAllocationOutcome{SlotMissCount: 1},
		)
		backend := &reusableWorkspaceSavingsEstimateBackend{
			telemetry: &telemetry,
		}

		res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, executeBeadReusableWorkspaceSavingsEstimateTestConfig(t), ExecuteBeadRuntime{
			AttemptBackend: backend,
			AgentRunner:    scriptHarnessAgentRunner{},
			FromRev:        baseRev,
		}, &RealGitOps{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Zero(t, res.ReusableWorkspaceSlotHits)
		require.Equal(t, 1, res.ReusableWorkspaceSlotMisses)
		require.Zero(t, res.ReusableWorkspaceTimeSavedMS)
		require.Zero(t, res.ReusableWorkspaceBytesSaved)

		report := ReportFromExecuteBeadResult(res, "")
		event := executeBeadLoopEvent(report, "worker", time.Unix(0, 0).UTC())
		require.Contains(t, event.Body, "reusable_workspace_slot_hits=0")
		require.Contains(t, event.Body, "reusable_workspace_slot_misses=1")
		require.Contains(t, event.Body, "reusable_workspace_time_saved_ms=0")
		require.Contains(t, event.Body, "reusable_workspace_bytes_saved=0")
	})

	t.Run("reused_slot_without_preserved_project_local_state", func(t *testing.T) {
		telemetry := AttemptWorkspaceReuseSavingsEstimateFromReusableSlotOutcome(
			AttemptWorkspaceReuseAllocationOutcome{
				SlotHitCount: 1,
			},
		)
		require.Equal(t, 1, telemetry.SlotHitCount)
		require.Zero(t, telemetry.SlotMissCount)
		require.Zero(t, telemetry.TimeSavedMS)
		require.Zero(t, telemetry.BytesSaved)

		backend := &reusableWorkspaceSavingsEstimateBackend{
			telemetry: &telemetry,
			pooled:    true,
		}

		res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, executeBeadReusableWorkspaceSavingsEstimateTestConfig(t), ExecuteBeadRuntime{
			AttemptBackend: backend,
			AgentRunner:    scriptHarnessAgentRunner{},
			FromRev:        baseRev,
		}, &RealGitOps{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, 1, res.ReusableWorkspaceSlotHits)
		require.Zero(t, res.ReusableWorkspaceSlotMisses)
		require.Zero(t, res.ReusableWorkspaceTimeSavedMS)
		require.Zero(t, res.ReusableWorkspaceBytesSaved)

		report := ReportFromExecuteBeadResult(res, "")
		event := executeBeadLoopEvent(report, "worker", time.Unix(0, 0).UTC())
		require.Contains(t, event.Body, "reusable_workspace_slot_hits=1")
		require.Contains(t, event.Body, "reusable_workspace_slot_misses=0")
		require.Contains(t, event.Body, "reusable_workspace_time_saved_ms=0")
		require.Contains(t, event.Body, "reusable_workspace_bytes_saved=0")
	})

	t.Run("cold_start", func(t *testing.T) {
		backend := &reusableWorkspaceSavingsEstimateBackend{
			pooled: false,
		}

		res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, executeBeadReusableWorkspaceSavingsEstimateTestConfig(t), ExecuteBeadRuntime{
			AttemptBackend: backend,
			AgentRunner:    scriptHarnessAgentRunner{},
			FromRev:        baseRev,
		}, &RealGitOps{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Zero(t, res.ReusableWorkspaceSlotHits)
		require.Equal(t, 1, res.ReusableWorkspaceSlotMisses)
		require.Zero(t, res.ReusableWorkspaceTimeSavedMS)
		require.Zero(t, res.ReusableWorkspaceBytesSaved)

		report := ReportFromExecuteBeadResult(res, "")
		event := executeBeadLoopEvent(report, "worker", time.Unix(0, 0).UTC())
		require.Contains(t, event.Body, "reusable_workspace_slot_hits=0")
		require.Contains(t, event.Body, "reusable_workspace_slot_misses=1")
		require.Contains(t, event.Body, "reusable_workspace_time_saved_ms=0")
		require.Contains(t, event.Body, "reusable_workspace_bytes_saved=0")
	})
}
