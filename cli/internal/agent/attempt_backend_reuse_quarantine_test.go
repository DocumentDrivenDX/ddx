package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type delegatingReusableSlotBackend struct {
	inner reusableAttemptBackend

	releaseCalls    int
	quarantineCalls int
}

func (b *delegatingReusableSlotBackend) Name() string { return b.inner.Name() }

func (b *delegatingReusableSlotBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	return b.inner.Prepare(ctx, req)
}

func (b *delegatingReusableSlotBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	return b.inner.Run(ctx, req)
}

func (b *delegatingReusableSlotBackend) ImportCandidate(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	return b.inner.ImportCandidate(ctx, ws, res)
}

func (b *delegatingReusableSlotBackend) ReleaseCandidateImport(ctx context.Context, ws *AttemptWorkspace) error {
	return b.inner.ReleaseCandidateImport(ctx, ws)
}

func (b *delegatingReusableSlotBackend) PublishResult(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	return b.inner.PublishResult(ctx, ws, res)
}

func (b *delegatingReusableSlotBackend) Cleanup(ctx context.Context, ws *AttemptWorkspace) error {
	return b.inner.Cleanup(ctx, ws)
}

func (b *delegatingReusableSlotBackend) Release(ctx context.Context, ws *AttemptWorkspace) error {
	b.releaseCalls++
	return b.inner.Release(ctx, ws)
}

func (b *delegatingReusableSlotBackend) Quarantine(ctx context.Context, ws *AttemptWorkspace) error {
	b.quarantineCalls++
	return b.inner.Quarantine(ctx, ws)
}

func TestReusableAttemptWorkspaceIntegrityDiagnosticsIncludeQuarantineReason(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)

	backend := LocalCloneAttemptBackend{}
	ws, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-int-0001",
		AttemptID:   "20260801T010101-corrupt",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = backend.Cleanup(context.Background(), ws) })

	ws.ReusableSlot = &AttemptWorkspaceSlot{
		Key: AttemptWorkspaceSlotKey{
			ProjectRoot: projectRoot,
			Backend:     AttemptBackendLocalClone,
			WorkerSlot:  "worker-a",
		},
		Path:   ws.WorkDir,
		Pooled: true,
	}

	require.NoError(t, os.RemoveAll(filepath.Join(ws.WorkDir, ".git")))

	err = backend.Release(context.Background(), ws)
	require.Error(t, err)
	require.Contains(t, err.Error(), "slot="+ws.WorkDir)
	require.Contains(t, err.Error(), "backend="+AttemptBackendLocalClone)
	require.Contains(t, err.Error(), "project="+projectRoot)
	require.Contains(t, err.Error(), "reason=")
	require.Contains(t, err.Error(), "scrub failed")
}

func TestReusableAttemptWorkspaceQuarantinesFailedIntegrityCheck(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)

	backend := &delegatingReusableSlotBackend{
		inner: LocalCloneAttemptBackend{},
	}

	ws, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-int-0002",
		AttemptID:   "20260801T010102-quarantine",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = backend.Cleanup(context.Background(), ws) })

	ws.ReusableSlot = &AttemptWorkspaceSlot{
		Key: AttemptWorkspaceSlotKey{
			ProjectRoot: projectRoot,
			Backend:     AttemptBackendLocalClone,
			WorkerSlot:  "worker-a",
		},
		Path:   ws.WorkDir,
		Pooled: true,
	}

	require.NoError(t, os.RemoveAll(filepath.Join(ws.WorkDir, ".git")))

	ok := cleanupReusableAttemptWorkspace(context.Background(), backend, ws, &ExecuteBeadResult{
		Outcome: ExecuteBeadOutcomeTaskSucceeded,
	})
	require.True(t, ok)
	require.Equal(t, 1, backend.releaseCalls)
	require.Equal(t, 1, backend.quarantineCalls)
	_, statErr := os.Stat(ws.WorkDir)
	require.True(t, os.IsNotExist(statErr))
}

func TestReusableAttemptWorkspaceReturnsOnlyHealthySlotsToPool(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)

	backend := &delegatingReusableSlotBackend{
		inner: LocalCloneAttemptBackend{},
	}

	ws, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-int-0003",
		AttemptID:   "20260801T010103-healthy",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = backend.Cleanup(context.Background(), ws) })

	ws.ReusableSlot = &AttemptWorkspaceSlot{
		Key: AttemptWorkspaceSlotKey{
			ProjectRoot: projectRoot,
			Backend:     AttemptBackendLocalClone,
			WorkerSlot:  "worker-a",
		},
		Path:   ws.WorkDir,
		Pooled: true,
	}

	ok := cleanupReusableAttemptWorkspace(context.Background(), backend, ws, &ExecuteBeadResult{
		Outcome: ExecuteBeadOutcomeTaskSucceeded,
	})
	require.True(t, ok)
	require.Equal(t, 1, backend.releaseCalls)
	require.Zero(t, backend.quarantineCalls)

	_, statErr := os.Stat(ws.WorkDir)
	require.NoError(t, statErr)

	status, err := runGitIntegOutput(ws.WorkDir, "status", "--porcelain", "--untracked-files=all")
	require.NoError(t, err)
	require.Empty(t, reusableAttemptWorkspaceDirtyPaths(strings.TrimSpace(status)))
	require.FileExists(t, filepath.Join(ws.WorkDir, slotStampFileName))
}
