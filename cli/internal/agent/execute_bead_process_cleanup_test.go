package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

type cleanupProcessRunner struct {
	started chan int
}

func newCleanupProcessRunner() *cleanupProcessRunner {
	return &cleanupProcessRunner{started: make(chan int, 1)}
}

func (r *cleanupProcessRunner) Run(opts RunArgs) (*Result, error) {
	cmd := exec.Command("sh", "-c", "cd \"$1\" && sh -c 'sleep 60' & wait", "sh", opts.WorkDir)
	cmd.Env = append(os.Environ(), DDXModeEnvKey+"="+DDXModeBeadExecution)
	if err := cmd.Start(); err != nil {
		return &Result{ExitCode: 1, Error: err.Error()}, nil
	}
	go func() { _, _ = cmd.Process.Wait() }()
	r.started <- cmd.Process.Pid
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	<-ctx.Done()
	return &Result{ExitCode: 1, Error: ctx.Err().Error()}, nil
}

func TestWorkRequestTimeoutCancelsProviderProcessTree(t *testing.T) {
	const beadID = "ddx-process-timeout"
	projectRoot, gitOps := setupProcessCleanupAttempt(t, beadID)
	runner := newCleanupProcessRunner()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	res, err := ExecuteBeadWithConfig(ctx, projectRoot, beadID, processCleanupConfig(), ExecuteBeadRuntime{
		AgentRunner: runner,
	}, gitOps)
	if err != nil && res == nil {
		t.Fatalf("ExecuteBeadWithConfig returned no result: %v", err)
	}

	pid := <-runner.started
	assertProcessGone(t, pid)
	report := readProcessCleanupReport(t, projectRoot)
	if len(report.Killed) == 0 {
		t.Fatalf("process cleanup report killed no descendants: %+v", report)
	}
	if report.Trigger == "" {
		t.Fatalf("process cleanup report missing trigger: %+v", report)
	}
}

func TestWorkSignalShutdownReapsAttemptDescendants(t *testing.T) {
	const beadID = "ddx-process-signal"
	projectRoot, gitOps := setupProcessCleanupAttempt(t, beadID)
	runner := newCleanupProcessRunner()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *ExecuteBeadResult, 1)
	errs := make(chan error, 1)
	go func() {
		res, err := ExecuteBeadWithConfig(ctx, projectRoot, beadID, processCleanupConfig(), ExecuteBeadRuntime{
			AgentRunner: runner,
		}, gitOps)
		errs <- err
		done <- res
	}()

	pid := <-runner.started
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ExecuteBeadWithConfig did not return after signal-style cancellation")
	}
	if err := <-errs; err != nil {
		t.Fatalf("ExecuteBeadWithConfig: %v", err)
	}
	assertProcessGone(t, pid)
	report := readProcessCleanupReport(t, projectRoot)
	if len(report.Killed) == 0 {
		t.Fatalf("process cleanup report killed no descendants: %+v", report)
	}
}

func TestWorkCleanupDoesNotKillUnrelatedProviderProcess(t *testing.T) {
	unrelated := exec.Command("sh", "-c", "sleep 60")
	if err := unrelated.Start(); err != nil {
		t.Fatalf("start unrelated provider-shaped process: %v", err)
	}
	t.Cleanup(func() {
		_ = unrelated.Process.Kill()
		_, _ = unrelated.Process.Wait()
	})

	const beadID = "ddx-process-unrelated"
	projectRoot, gitOps := setupProcessCleanupAttempt(t, beadID)
	runner := newCleanupProcessRunner()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	res, err := ExecuteBeadWithConfig(ctx, projectRoot, beadID, processCleanupConfig(), ExecuteBeadRuntime{
		AgentRunner: runner,
	}, gitOps)
	if err != nil && res == nil {
		t.Fatalf("ExecuteBeadWithConfig returned no result: %v", err)
	}

	pid := <-runner.started
	assertProcessGone(t, pid)
	if !signalProcessAlive(unrelated.Process.Pid) {
		t.Fatalf("cleanup killed unrelated process pid %d", unrelated.Process.Pid)
	}
}

func setupProcessCleanupAttempt(t *testing.T, beadID string) (string, *artifactTestGitOps) {
	t.Helper()
	projectRoot := setupArtifactTestProjectRoot(t)
	gitOps := &artifactTestGitOps{
		projectRoot: projectRoot,
		baseRev:     "processcleanupbase",
		resultRev:   "processcleanupbase",
		wtSetupFn: func(wtPath string) {
			setupArtifactTestWorktree(t, wtPath, beadID, "", false, 0)
		},
	}
	return projectRoot, gitOps
}

func processCleanupConfig() config.ResolvedConfig {
	return config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})
}

func assertProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !signalProcessAlive(pid) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("process pid %d still alive after cleanup", pid)
}

func readProcessCleanupReport(t *testing.T, projectRoot string) attemptProcessCleanupReport {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(projectRoot, ddxroot.DirName, "executions", "*", attemptProcessCleanupArtifact))
	if err != nil {
		t.Fatalf("glob cleanup report: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("missing process-cleanup.json evidence")
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read cleanup report: %v", err)
	}
	var report attemptProcessCleanupReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse cleanup report: %v", err)
	}
	return report
}
