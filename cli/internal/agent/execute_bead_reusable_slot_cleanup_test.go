package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
)

type recordingReusableSlotBackend struct {
	inner AttemptBackend

	workspace       *AttemptWorkspace
	releaseCalls    int
	quarantineCalls int
	cleanupCalls    int
	prepareHook     func(*AttemptWorkspace)
	runFunc         func(context.Context, AttemptBackendRunRequest) (*Result, error)
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

func (b *recordingReusableSlotBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	if b.runFunc != nil {
		return b.runFunc(ctx, req)
	}
	return &Result{
		Harness:  "script",
		ExitCode: 1,
		Error:    "simulated reusable-slot failure",
		Output:   "",
	}, nil
}

func (b *recordingReusableSlotBackend) ImportCandidate(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	return b.inner.ImportCandidate(ctx, ws, res)
}

func (b *recordingReusableSlotBackend) ReleaseCandidateImport(ctx context.Context, ws *AttemptWorkspace) error {
	return b.inner.ReleaseCandidateImport(ctx, ws)
}

func (b *recordingReusableSlotBackend) PublishResult(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	return b.inner.PublishResult(ctx, ws, res)
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
	backend := &recordingReusableSlotBackend{
		inner: LocalCloneAttemptBackend{},
		prepareHook: func(ws *AttemptWorkspace) {
			ws.ReusableSlot = &AttemptWorkspaceSlot{Pooled: true, Path: ws.WorkDir}
		},
	}

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
	projectRoot, _ := newScriptHarnessRepo(t, 1)

	t.Run("success_uses_destructive_cleanup_not_pool_release", func(t *testing.T) {
		backend := &recordingReusableSlotBackend{
			inner: LocalCloneAttemptBackend{},
			prepareHook: func(ws *AttemptWorkspace) {
				ws.ReusableSlot = &AttemptWorkspaceSlot{Pooled: false, Path: ws.WorkDir}
			},
			runFunc: func(_ context.Context, req AttemptBackendRunRequest) (*Result, error) {
				require.NoError(t, os.WriteFile(filepath.Join(req.Workspace.WorkDir, "disabled-reuse.txt"), []byte("done\n"), 0o644))
				runGitInteg(t, req.Workspace.WorkDir, "add", "disabled-reuse.txt")
				runGitInteg(t, req.Workspace.WorkDir, "commit", "-m", "chore: disabled reuse attempt output")
				return &Result{Harness: "script", ExitCode: 0}, nil
			},
		}

		rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
			AttemptBackend: AttemptBackendLocalClone,
		})
		res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
			AttemptBackend: backend,
			NoReview:       true,
		}, &RealGitOps{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, ExecuteBeadStatusSuccess, res.Status)
		require.Equal(t, 0, backend.releaseCalls, "disabled reuse must not return a workspace to the reusable slot pool")
		require.Equal(t, 0, backend.quarantineCalls, "disabled reuse must not quarantine through the reusable slot pool")
		require.Equal(t, 1, backend.cleanupCalls, "disabled reuse must keep destructive per-attempt cleanup")
		require.NotNil(t, backend.workspace)
		_, statErr := os.Stat(backend.workspace.WorkDir)
		require.True(t, os.IsNotExist(statErr), "disabled reuse success should delete the per-attempt workspace")
	})

	t.Run("keep_on_error_preserves_existing_cleanup_branch", func(t *testing.T) {
		backend := &recordingReusableSlotBackend{
			inner: LocalCloneAttemptBackend{},
			prepareHook: func(ws *AttemptWorkspace) {
				ws.ReusableSlot = &AttemptWorkspaceSlot{Pooled: false, Path: ws.WorkDir}
				ws.KeepOnError = true
			},
		}

		rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
			AttemptBackend: AttemptBackendLocalClone,
		})
		res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
			AttemptBackend: backend,
		}, &RealGitOps{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, ExecuteBeadStatusExecutionFailed, res.Status)
		require.Equal(t, 0, backend.releaseCalls, "disabled reuse error path must not return a workspace to the reusable slot pool")
		require.Equal(t, 0, backend.quarantineCalls, "disabled reuse error path must not quarantine through the reusable slot pool")
		require.Equal(t, 1, backend.cleanupCalls, "disabled reuse must still call the existing cleanup branch")
		require.NotNil(t, backend.workspace)
		_, statErr := os.Stat(backend.workspace.WorkDir)
		require.NoError(t, statErr, "KeepOnError cleanup branch should preserve the workspace for inspection")
	})
}
