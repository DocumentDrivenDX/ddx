package agent

import (
	"context"
	"os"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
)

type recordingReusableSlotBackend struct {
	inner       AttemptBackend
	runResult   *Result
	runErr      error
	prepareHook func(*AttemptWorkspace)

	workspace       *AttemptWorkspace
	releaseCalls    int
	quarantineCalls int
	cleanupCalls    int
}

func (b *recordingReusableSlotBackend) Name() string { return b.inner.Name() }

func (b *recordingReusableSlotBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	ws, err := b.inner.Prepare(ctx, req)
	if err != nil {
		return nil, err
	}
	if b.prepareHook != nil {
		b.prepareHook(ws)
	}
	b.workspace = ws
	return ws, nil
}

func (b *recordingReusableSlotBackend) Run(context.Context, AttemptBackendRunRequest) (*Result, error) {
	if b.runErr != nil {
		return nil, b.runErr
	}
	if b.runResult != nil {
		res := *b.runResult
		return &res, nil
	}
	return &Result{
		Harness:  "script",
		ExitCode: 1,
		Error:    "simulated reusable-slot failure",
		Output:   "",
	}, nil
}

func (*recordingReusableSlotBackend) ImportCandidate(context.Context, *AttemptWorkspace, *ExecuteBeadResult) error {
	return nil
}

func (*recordingReusableSlotBackend) ReleaseCandidateImport(context.Context, *AttemptWorkspace) error {
	return nil
}

func (*recordingReusableSlotBackend) PublishResult(context.Context, *AttemptWorkspace, *ExecuteBeadResult) error {
	return nil
}

func (b *recordingReusableSlotBackend) Cleanup(ctx context.Context, ws *AttemptWorkspace) error {
	b.cleanupCalls++
	return b.inner.Cleanup(ctx, ws)
}

func (b *recordingReusableSlotBackend) Release(ctx context.Context, ws *AttemptWorkspace) error {
	b.releaseCalls++
	return nil
}

func (b *recordingReusableSlotBackend) Quarantine(ctx context.Context, ws *AttemptWorkspace) error {
	b.quarantineCalls++
	return b.inner.Cleanup(ctx, ws)
}

func TestExecuteBeadQuarantinesUnhealthyReusableSlot(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	backend := &recordingReusableSlotBackend{inner: LocalCloneAttemptBackend{}}

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
		AttemptBackend: AttemptBackendLocalClone,
	})

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AttemptBackend: backend,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, ExecuteBeadStatusExecutionFailed, res.Status)

	require.Equal(t, 0, backend.releaseCalls, "unhealthy reusable slots must not be returned to the pool")
	require.Equal(t, 1, backend.quarantineCalls, "unhealthy reusable slots must be quarantined")
	require.Equal(t, 0, backend.cleanupCalls, "execute-bead should use the quarantine path instead of generic cleanup")
	require.NotNil(t, backend.workspace)
	_, statErr := os.Stat(backend.workspace.WorkDir)
	require.True(t, os.IsNotExist(statErr), "quarantine should delete the unhealthy workspace")
}

func TestExecuteBeadDisabledWorkspaceReuseKeepsPerAttemptWorkspaces(t *testing.T) {
	t.Run("success_uses_destructive_cleanup", func(t *testing.T) {
		projectRoot, _ := newScriptHarnessRepo(t, 1)
		backend := &recordingReusableSlotBackend{
			inner: LocalCloneAttemptBackend{},
			runResult: &Result{
				Harness:  "script",
				ExitCode: 0,
			},
		}

		disabled := false
		cfgSrc := config.NewTestConfigForBead(config.TestBeadConfigOpts{})
		cfgSrc.Executions.ReusableWorkspace = &config.ReusableWorkspaceConfig{Enabled: &disabled}
		rcfg := cfgSrc.Resolve(config.CLIOverrides{AttemptBackend: AttemptBackendLocalClone})

		res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
			AttemptBackend: backend,
		}, &RealGitOps{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, 0, res.ExitCode)
		require.Equal(t, 0, backend.releaseCalls, "disabled reuse must not return a workspace to the slot pool")
		require.Equal(t, 0, backend.quarantineCalls, "disabled reuse must not quarantine through reusable-slot cleanup")
		require.Equal(t, 1, backend.cleanupCalls, "disabled reuse must keep the destructive cleanup path")
		require.NotNil(t, backend.workspace)
		_, statErr := os.Stat(backend.workspace.WorkDir)
		require.True(t, os.IsNotExist(statErr), "destructive cleanup should delete the workspace")
	})

	t.Run("preserve_attempt_worktree_still_skips_cleanup", func(t *testing.T) {
		projectRoot, _ := newScriptHarnessRepo(t, 1)
		backend := &recordingReusableSlotBackend{
			inner: WorktreeAttemptBackend{},
			runResult: &Result{
				Harness:  "script",
				ExitCode: 1,
				Error:    "simulated reusable-slot failure",
			},
		}

		disabled := false
		cfgSrc := config.NewTestConfigForBead(config.TestBeadConfigOpts{})
		cfgSrc.Executions.ReusableWorkspace = &config.ReusableWorkspaceConfig{Enabled: &disabled}
		rcfg := cfgSrc.Resolve(config.CLIOverrides{AttemptBackend: AttemptBackendWorktree})

		res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
			AttemptBackend:          backend,
			PreserveAttemptWorktree: true,
		}, &RealGitOps{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, 1, res.ExitCode)
		require.Equal(t, 0, backend.releaseCalls, "preserved worktrees must not be returned to the slot pool")
		require.Equal(t, 0, backend.quarantineCalls, "preserved worktrees must not be quarantined")
		require.Equal(t, 0, backend.cleanupCalls, "preserved worktrees must bypass generic cleanup")
		require.NotNil(t, backend.workspace)
		_, statErr := os.Stat(backend.workspace.WorkDir)
		require.NoError(t, statErr, "preserved worktree should remain on disk")
	})

	t.Run("keep_on_error_still_uses_backend_cleanup", func(t *testing.T) {
		projectRoot, _ := newScriptHarnessRepo(t, 1)
		backend := &recordingReusableSlotBackend{
			inner: LocalCloneAttemptBackend{},
			runResult: &Result{
				Harness:  "script",
				ExitCode: 1,
				Error:    "simulated reusable-slot failure",
			},
			prepareHook: func(ws *AttemptWorkspace) {
				ws.KeepOnError = true
			},
		}

		disabled := false
		cfgSrc := config.NewTestConfigForBead(config.TestBeadConfigOpts{})
		cfgSrc.Executions.ReusableWorkspace = &config.ReusableWorkspaceConfig{Enabled: &disabled}
		rcfg := cfgSrc.Resolve(config.CLIOverrides{AttemptBackend: AttemptBackendLocalClone})

		res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
			AttemptBackend: backend,
		}, &RealGitOps{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, 1, res.ExitCode)
		require.Equal(t, 0, backend.releaseCalls, "disabled reuse must not return keep-on-error workspaces to the pool")
		require.Equal(t, 0, backend.quarantineCalls, "disabled reuse must not quarantine via reusable-slot cleanup")
		require.Equal(t, 1, backend.cleanupCalls, "keep-on-error should still route through backend cleanup")
		require.NotNil(t, backend.workspace)
		_, statErr := os.Stat(backend.workspace.WorkDir)
		require.NoError(t, statErr, "keep-on-error workspace should remain on disk")
	})
}
