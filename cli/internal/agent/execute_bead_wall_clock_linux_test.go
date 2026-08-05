//go:build linux

package agent

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/require"
)

type wallClockReparentingStart struct {
	PID      int
	WorkDir  string
	ChildPID string
}

type wallClockReparentingRunner struct {
	started chan wallClockReparentingStart
}

func newWallClockReparentingRunner() *wallClockReparentingRunner {
	return &wallClockReparentingRunner{started: make(chan wallClockReparentingStart, 1)}
}

func (r *wallClockReparentingRunner) Run(opts RunArgs) (*Result, error) {
	setsidPath, err := exec.LookPath("setsid")
	if err != nil {
		return &Result{ExitCode: 1, Error: err.Error()}, nil
	}
	if opts.WorkDir == "" {
		return &Result{ExitCode: 1, Error: "missing workdir"}, nil
	}

	childPIDFile := filepath.Join(opts.WorkDir, "attempt-child.pid")
	script := fmt.Sprintf(
		`echo $$ > "$1"; ( %s sh -c 'echo $$ > "$1"; trap "" TERM; sleep 600' sh "$2" & exit 0 ); sleep 600`,
		setsidPath,
	)
	cmd := exec.Command("sh", "-c", script, "sh", filepath.Join(opts.WorkDir, "attempt-parent.pid"), childPIDFile)
	cmd.Dir = opts.WorkDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return &Result{ExitCode: 1, Error: err.Error()}, nil
	}
	if fn := onExecuteStartFromContext(opts.Context); fn != nil {
		fn()
	}
	if fn := onRouteResolvedFromContext(opts.Context); fn != nil {
		fn("test-harness", "test-provider", "test-model")
	}
	r.started <- wallClockReparentingStart{
		PID:      cmd.Process.Pid,
		WorkDir:  opts.WorkDir,
		ChildPID: childPIDFile,
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	<-ctx.Done()
	return &Result{ExitCode: 1, Error: ctx.Err().Error()}, nil
}

func TestWorkAttemptWallClock_ReapsSessionChangingDescendants(t *testing.T) {
	projectRoot := setupArtifactTestProjectRoot(t)
	beadID := "ddx-wallclock-reparent"
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    beadID,
		Title: "wall clock reparent bead",
	}))
	gitOps := &artifactTestGitOps{
		projectRoot: projectRoot,
		baseRev:     "wallclock-base",
		resultRev:   "wallclock-base",
		wtSetupFn: func(wtPath string) {
			setupArtifactTestWorktree(t, wtPath, beadID, "", false, 0)
		},
	}
	runner := newWallClockReparentingRunner()

	rcfg := config.NewTestConfigForLoop(config.TestLoopConfigOpts{
		Assignee:          "worker-wallclock",
		HeartbeatInterval: 5 * time.Millisecond,
	}).Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{
		Assignee:          "worker-wallclock",
		HeartbeatInterval: 5 * time.Millisecond,
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan struct {
		result *ExecuteBeadLoopResult
		err    error
	}, 1)
	go func() {
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				res, err := ExecuteBeadWithConfig(ctx, projectRoot, beadID, processCleanupConfig(), ExecuteBeadRuntime{
					AgentRunner: runner,
					NoReview:    true,
					WorkerID:    "worker-wallclock-reparent",
				}, gitOps)
				if err != nil {
					return ExecuteBeadReport{}, err
				}
				return ReportFromExecuteBeadResult(res, ""), nil
			}),
		}
		result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
			Mode:                executeloop.ModeOnce,
			NoReview:            true,
			AttemptWallClock:    100 * time.Millisecond,
			AttemptWallClockSet: true,
			ProjectRoot:         projectRoot,
			SessionID:           "sess-wallclock-reparent",
			WorkerID:            "worker-wallclock-reparent",
		})
		done <- struct {
			result *ExecuteBeadLoopResult
			err    error
		}{result: result, err: err}
	}()

	attempt := <-runner.started
	childPID := waitForCompletePIDFile(t, attempt.ChildPID, 5*time.Second)
	require.True(t, signalProcessAlive(attempt.PID), "attempt leader must still be alive before wall-clock expiry")
	require.True(t, signalProcessAlive(childPID), "reparented descendant must still be alive before wall-clock expiry")

	select {
	case outcome := <-done:
		if outcome.err != nil && outcome.result == nil {
			t.Fatalf("ExecuteBeadWithConfig returned no result: %v", outcome.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ExecuteBeadWorker.Run did not return after wall-clock expiry")
	}

	require.Eventually(t, func() bool {
		return processDeadOrZombie(attempt.PID) && processDeadOrZombie(childPID)
	}, 5*time.Second, 25*time.Millisecond, "wall-clock expiry must reap both the leader and its reparented descendant")
}
