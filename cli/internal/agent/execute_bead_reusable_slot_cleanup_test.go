package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordingReusableSlotBackend struct {
	inner AttemptBackend

	workspace       *AttemptWorkspace
	releaseCalls    int
	quarantineCalls int
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

func (b *recordingReusableSlotBackend) Release(ctx context.Context, ws *AttemptWorkspace) error {
	b.releaseCalls++
	if reusable, ok := b.inner.(interface {
		Release(context.Context, *AttemptWorkspace) error
	}); ok {
		return reusable.Release(ctx, ws)
	}
	return nil
}

func (b *recordingReusableSlotBackend) Quarantine(ctx context.Context, ws *AttemptWorkspace) error {
	b.quarantineCalls++
	if reusable, ok := b.inner.(interface {
		Quarantine(context.Context, *AttemptWorkspace) error
	}); ok {
		return reusable.Quarantine(ctx, ws)
	}
	return nil
}

func (b *recordingReusableSlotBackend) Cleanup(ctx context.Context, ws *AttemptWorkspace) error {
	return b.inner.Cleanup(ctx, ws)
}

func readQuarantineRecord(t *testing.T, ws *AttemptWorkspace) reusableAttemptWorkspaceQuarantineRecord {
	t.Helper()
	path := reusableAttemptWorkspaceQuarantineMarkerPath(ws.WorkDir)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var record reusableAttemptWorkspaceQuarantineRecord
	require.NoError(t, json.Unmarshal(raw, &record))
	return record
}

func TestReusableAttemptWorkspaceQuarantinesDirtyOrLiveSlot(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	cases := []struct {
		name            string
		backend         AttemptBackend
		result          *ExecuteBeadResult
		cleanupSignal   *attemptProcessCleanupSignal
		wantQuarantine  int
		wantRelease     int
		wantReasonMatch string
	}{
		{
			name:    "local-clone dirty outcome quarantines",
			backend: LocalCloneAttemptBackend{},
			result: &ExecuteBeadResult{
				Outcome: ExecuteBeadOutcomeTaskFailed,
				Error:   "simulated cleanup failure",
				Reason:  "simulated cleanup failure",
			},
			wantQuarantine:  1,
			wantRelease:     0,
			wantReasonMatch: "attempt outcome task_failed",
		},
		{
			name:    "linked-worktree live descendants quarantine",
			backend: WorktreeAttemptBackend{},
			result: &ExecuteBeadResult{
				Outcome: ExecuteBeadOutcomeTaskSucceeded,
				Reason:  "result landed cleanly",
			},
			cleanupSignal:   &attemptProcessCleanupSignal{LiveDescendants: 1},
			wantQuarantine:  1,
			wantRelease:     0,
			wantReasonMatch: "surviving descendant process",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backend := &recordingReusableSlotBackend{inner: tc.backend}
			ws, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
				ProjectRoot: projectRoot,
				BeadID:      "ddx-reusable-slot-test",
				AttemptID:   "20260731T000001-test",
				BaseRev:     baseRev,
			})
			require.NoError(t, err)
			ws.ReusableSlot = true
			t.Cleanup(func() { _ = backend.inner.Cleanup(context.Background(), ws) })

			require.True(t, cleanupReusableAttemptWorkspace(context.Background(), backend, ws, tc.result, tc.cleanupSignal))
			require.Equal(t, tc.wantRelease, backend.releaseCalls)
			require.Equal(t, tc.wantQuarantine, backend.quarantineCalls)

			record := readQuarantineRecord(t, ws)
			require.Equal(t, filepath.Base(ws.WorkDir), record.Slot)
			require.Equal(t, ws.Backend, record.Backend)
			require.Equal(t, projectRoot, record.ProjectRoot)
			require.Contains(t, record.Reason, tc.wantReasonMatch)
		})
	}
}

func TestReusableAttemptWorkspaceDiagnosticsIncludeQuarantineReason(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	backend := &recordingReusableSlotBackend{inner: LocalCloneAttemptBackend{}}

	ws, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-reusable-slot-diagnostics",
		AttemptID:   "20260731T000002-test",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	ws.ReusableSlot = true
	t.Cleanup(func() { _ = backend.inner.Cleanup(context.Background(), ws) })

	ws.QuarantineReason = "cleanup failed: git reset --hard returned exit status 1"
	require.NoError(t, backend.Quarantine(context.Background(), ws))

	record := readQuarantineRecord(t, ws)
	require.Equal(t, filepath.Base(ws.WorkDir), record.Slot)
	require.Equal(t, ws.Backend, record.Backend)
	require.Equal(t, projectRoot, record.ProjectRoot)
	require.Equal(t, ws.QuarantineReason, record.Reason)
	require.Equal(t, ws.WorkDir, record.WorkDir)
}

func TestReusableAttemptWorkspaceDoesNotRetainLiveProcessesBetweenBeads(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	backend := &recordingReusableSlotBackend{inner: LocalCloneAttemptBackend{}}

	ws, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-reusable-slot-live",
		AttemptID:   "20260731T000003-test",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	ws.ReusableSlot = true
	t.Cleanup(func() { _ = backend.inner.Cleanup(context.Background(), ws) })

	result := &ExecuteBeadResult{
		Outcome: ExecuteBeadOutcomeTaskSucceeded,
		Reason:  "task succeeded, but descendants survived cleanup",
	}
	signal := &attemptProcessCleanupSignal{LiveDescendants: 2}

	require.True(t, cleanupReusableAttemptWorkspace(context.Background(), backend, ws, result, signal))
	require.Equal(t, 0, backend.releaseCalls)
	require.Equal(t, 1, backend.quarantineCalls)

	record := readQuarantineRecord(t, ws)
	require.Contains(t, record.Reason, "surviving descendant process")
	require.Equal(t, ws.WorkDir, record.WorkDir)
	require.Equal(t, ws.Backend, record.Backend)
}
