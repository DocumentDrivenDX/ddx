//go:build linux

package agent

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
)

type reusableWorkspaceTelemetryRecordingBackend struct {
	reusableWorkspaceTelemetryCleanupCountingBackend
	workspace    *AttemptWorkspace
	cleanupCalls int
}

func (b *reusableWorkspaceTelemetryRecordingBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	ws, err := b.reusableWorkspaceTelemetryCleanupCountingBackend.Prepare(ctx, req)
	if err != nil {
		return nil, err
	}
	b.workspace = ws
	return ws, nil
}

func (b *reusableWorkspaceTelemetryRecordingBackend) Cleanup(ctx context.Context, ws *AttemptWorkspace) error {
	b.cleanupCalls++
	return b.reusableWorkspaceTelemetryCleanupCountingBackend.Cleanup(ctx, ws)
}

type reusableWorkspaceTelemetryProcessRunner struct {
	started chan int
}

func newReusableWorkspaceTelemetryProcessRunner() *reusableWorkspaceTelemetryProcessRunner {
	return &reusableWorkspaceTelemetryProcessRunner{started: make(chan int, 1)}
}

func (r *reusableWorkspaceTelemetryProcessRunner) Run(opts RunArgs) (*Result, error) {
	cmd := exec.Command("sleep", "60")
	cmd.Dir = opts.WorkDir
	if err := cmd.Start(); err != nil {
		return &Result{Harness: "script", ExitCode: 1, Error: err.Error()}, nil
	}
	go func() {
		_, _ = cmd.Process.Wait()
	}()
	r.started <- cmd.Process.Pid
	return &Result{Harness: "script", ExitCode: 0}, nil
}

func TestAttemptWorkspaceReuseTelemetryHelpersDoNotLeakProcesses(t *testing.T) {
	const beadID = "ddx-int-0001"

	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	backend := &reusableWorkspaceTelemetryRecordingBackend{
		reusableWorkspaceTelemetryCleanupCountingBackend: reusableWorkspaceTelemetryCleanupCountingBackend{
			reusableWorkspaceTelemetryPrepBackend: reusableWorkspaceTelemetryPrepBackend{
				slot: &AttemptWorkspaceSlot{
					Pooled: false,
				},
			},
		},
	}
	runner := newReusableWorkspaceTelemetryProcessRunner()
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
		AttemptBackend: AttemptBackendLocalClone,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := ExecuteBeadWithConfig(ctx, projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		WorkerID:       "worker-slot-a",
		BeadEvents:     &stubBeadEventAppender{},
		AgentRunner:    runner,
		FromRev:        baseRev,
		AttemptBackend: backend,
		NoReview:       true,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)

	pid := <-runner.started
	require.Eventually(t, func() bool {
		return processDeadOrZombie(pid)
	}, 5*time.Second, 25*time.Millisecond, "attempt process pid %d was not reaped", pid)

	report := readProcessCleanupReport(t, projectRoot)
	require.NotEmpty(t, report.Killed, "cleanup should record the spawned process as killed")
	found := false
	for _, proc := range report.Killed {
		if proc.PID == pid {
			found = true
			break
		}
	}
	require.True(t, found, "cleanup report should include the spawned pid %d", pid)

	require.Zero(t, backend.releaseCalls)
	require.Zero(t, backend.quarantineCalls)
	require.Equal(t, 1, backend.cleanupCalls)
	require.NotNil(t, backend.workspace)

	status, err := runGitIntegOutput(backend.workspace.WorkDir, "status", "--porcelain", "--untracked-files=all")
	require.Error(t, err)
	require.Empty(t, status)
}
