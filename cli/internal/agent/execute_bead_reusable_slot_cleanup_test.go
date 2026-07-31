package agent

import (
	"context"
	"os"
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
}

func (b *recordingReusableSlotBackend) Name() string { return b.inner.Name() }

func (b *recordingReusableSlotBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	ws, err := b.inner.Prepare(ctx, req)
	if err != nil {
		return nil, err
	}
	b.workspace = ws
	return ws, nil
}

func (b *recordingReusableSlotBackend) Run(context.Context, AttemptBackendRunRequest) (*Result, error) {
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
	if backend, ok := b.inner.(interface {
		Quarantine(context.Context, *AttemptWorkspace) error
	}); ok {
		return backend.Quarantine(ctx, ws)
	}
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
