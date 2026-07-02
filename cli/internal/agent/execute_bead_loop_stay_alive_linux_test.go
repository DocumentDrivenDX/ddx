//go:build !windows

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type agentRunnerFunc func(opts RunArgs) (*Result, error)

func (f agentRunnerFunc) Run(opts RunArgs) (*Result, error) {
	return f(opts)
}

func startFakeProviderChildNoWait(t *testing.T, dir, provider string) int {
	t.Helper()
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	bin := filepath.Join(dir, provider)
	if err := os.Symlink(sleepPath, bin); err != nil {
		t.Fatalf("symlink fake %s: %v", provider, err)
	}
	cmd := exec.Command(bin, "120")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake %s: %v", provider, err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
	})
	return pid
}

func writeFakeClaudeHarness(t *testing.T, dir string) string {
	t.Helper()
	codexPath := filepath.Join(dir, "codex")
	codexScript := "#!/bin/sh\nsleep 120\n"
	if err := os.WriteFile(codexPath, []byte(codexScript), 0o755); err != nil {
		t.Fatalf("write fake codex harness: %v", err)
	}
	claudePath := filepath.Join(dir, "claude")
	script := "#!/bin/sh\ncodex \"$@\" >/dev/null 2>&1 &\nsleep 1\nexit 1\n"
	if err := os.WriteFile(claudePath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude harness: %v", err)
	}
	return claudePath
}

// TestWork_NonRouteProviderIsActuallyReaped proves the running-phase guard does
// not just label a non-route provider child as terminated: it actually signals
// and reaps the child, and the process does not reappear after the guard runs.
func TestWork_NonRouteProviderIsActuallyReaped(t *testing.T) {
	dir := t.TempDir()
	claudePID := startFakeProviderChildNoWait(t, dir, "claude")
	codexPID := startFakeProviderChildNoWait(t, dir, "codex")
	waitForProviderChildren(t, os.Getpid(), claudePID, codexPID)

	start := time.Now()
	children, reaped := runningProviderChildGuard(context.Background(), os.Getpid(), "claude/opus", "", "running", time.Now().UTC())
	require.Less(t, time.Since(start), 10*time.Second, "running guard must complete within the bounded reap window")

	assertProcessGone(t, codexPID)
	if !signalProcessAlive(claudePID) {
		t.Fatalf("route-owned claude child %d was reaped by the running guard", claudePID)
	}
	require.Eventually(t, func() bool {
		procs, err := providerChildScanner(context.Background(), os.Getpid(), time.Now().UTC())
		if err != nil {
			return false
		}
		for _, proc := range procs {
			if proc.Provider == "codex" {
				return false
			}
		}
		return true
	}, 3*time.Second, 25*time.Millisecond, "codex child must not respawn after the guard reaps it")

	var sawClaude bool
	for _, child := range children {
		switch child.Provider {
		case "claude":
			sawClaude = true
			assert.False(t, child.NonRoute, "route-owned claude child must stay owned")
			assert.NotEmpty(t, child.RouteOwner, "route-owned claude child must record its owner")
		case "codex":
			assert.True(t, child.NonRoute, "codex must be marked non-route")
			assert.NotEmpty(t, child.Diagnostic, "codex must carry a non-route diagnostic")
		}
	}
	require.True(t, sawClaude, "running guard status must include the route-owned claude child")

	require.Len(t, reaped, 1, "only the non-route codex child should be reaped")
	assert.Equal(t, "codex", reaped[0].Provider)
	assert.Equal(t, reasonRunningPhaseGuard, reaped[0].Reason)
}

// TestWork_ClaudeSubprocessDeathSynthesizesTerminalOutcome proves the worker
// does not stay in phase=running when the claude harness loses its final event
// and the guard has to reap a stray codex child to unblock the attempt.
func TestWork_ClaudeSubprocessDeathSynthesizesTerminalOutcome(t *testing.T) {
	projectRoot := setupArtifactTestProjectRoot(t)
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	first := &bead.Bead{ID: "ddx-0001", Title: "First ready", Priority: 0}
	second := &bead.Bead{ID: "ddx-0002", Title: "Second ready", Priority: 1}
	require.NoError(t, store.Create(context.Background(), first))
	require.NoError(t, store.Create(context.Background(), second))
	_ = second
	binDir := t.TempDir()
	claudePath := writeFakeClaudeHarness(t, binDir)

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Harness: "claude",
		Model:   "opus",
	}).Resolve(config.CLIOverrides{})

	gitOps := &artifactTestGitOps{
		projectRoot: projectRoot,
		baseRev:     "base-rev",
		resultRev:   "base-rev",
		wtSetupFn: func(wtPath string) {
			setupArtifactTestWorktree(t, wtPath, first.ID, "", false, 0)
		},
	}

	runner := agentRunnerFunc(func(opts RunArgs) (*Result, error) {
		ctx := opts.Context
		if ctx == nil {
			ctx = context.Background()
		}
		ctx = withExecutionEnv(ctx, map[string]string{
			"PATH": binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		})
		start := time.Now()
		execResult, execErr := (&OSExecutor{}).ExecuteInDir(ctx, claudePath, nil, "", opts.WorkDir)
		result := &Result{
			Harness:    opts.Harness,
			Model:      opts.Model,
			DurationMS: int(time.Since(start).Milliseconds()),
		}
		if execResult != nil {
			result.ExitCode = execResult.ExitCode
			result.Output = execResult.Stdout
			result.Stderr = execResult.Stderr
		}
		if execErr != nil {
			result.Error = execErr.Error()
		} else if result.ExitCode != 0 {
			result.Error = "agent: provider process exited without emitting a final event"
		}
		return result, nil
	})

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			res, err := ExecuteBeadWithConfig(ctx, projectRoot, beadID, rcfg, ExecuteBeadRuntime{
				WorkerID:    "worker-claude-test",
				AgentRunner: runner,
			}, gitOps)
			if err != nil {
				return ExecuteBeadReport{}, err
			}
			return ReportFromExecuteBeadResult(res, ""), nil
		}),
	}

	loopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", HeartbeatInterval: 25 * time.Millisecond}
	rcfgLoop := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	start := time.Now()
	result, err := worker.Run(loopCtx, rcfgLoop, ExecuteBeadLoopRuntime{
		Mode:        executeloop.ModeOnce,
		ProjectRoot: projectRoot,
		SessionID:   "sess-claude-final",
		WorkerID:    "worker-claude-test",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Less(t, time.Since(start), 30*time.Second, "the worker must exit the attempt instead of hanging in phase=running")
	require.Equal(t, 1, result.Attempts)
	require.Equal(t, 1, result.Failures)
	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.Results[0].Status)
	assert.Contains(t, result.Results[0].Error, "final event")

	liveness, err := workerstatus.ReadLiveness(projectRoot, "sess-claude-final")
	require.NoError(t, err)
	assert.NotEqual(t, "running", liveness.Phase, "liveness sidecar must be terminal after the attempt exits")
	assert.Empty(t, liveness.CurrentBead, "terminal liveness must clear the in-flight bead")
}
