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

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// claudeDeathRunner is a real-process AgentRunner stand-in for the harness
// dispatch: it spawns a genuine OS process named "claude" (so the
// running-phase guard's ps scan classifies and route-scopes it exactly as it
// would the real subprocess), reports its pid, and then blocks purely on
// ctx.Done() — it does NOT itself notice the spawned process dying, mirroring
// a dispatch layer that only exits via cancellation. This is deliberate: the
// test proves recovery comes from the harness-liveness watchdog detecting
// the death via the process tree, not from the AgentRunner noticing on its
// own.
type claudeDeathRunner struct {
	bin     string
	started chan int
}

func newClaudeDeathRunner(t *testing.T, dir string) *claudeDeathRunner {
	t.Helper()
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	bin := filepath.Join(dir, "claude")
	if err := os.Symlink(sleepPath, bin); err != nil {
		t.Fatalf("symlink fake claude: %v", err)
	}
	return &claudeDeathRunner{bin: bin, started: make(chan int, 1)}
}

func (r *claudeDeathRunner) Run(opts RunArgs) (*Result, error) {
	cmd := exec.Command(r.bin, "120")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return &Result{ExitCode: 1, Error: err.Error()}, nil
	}
	pid := cmd.Process.Pid
	go func() { _, _ = cmd.Process.Wait() }()
	r.started <- pid
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	<-ctx.Done()
	return &Result{ExitCode: 1, Error: ctx.Err().Error()}, nil
}

// TestWork_ClaudeSubprocessDeathEndToEndRecoversWithin60s (ddx-f2b7cf89 AC3):
// a scripted end-to-end reproduction of the reported incident — a real
// "claude" subprocess is killed out from under a running attempt, without
// the test ever cancelling the attempt's context itself. Before this bead's
// fix, nothing re-checked a route-owned process's liveness once the route
// was resolved (the generic progress watchdog disables itself once a route
// is populated), so the dispatch call — and therefore ddx work --watch —
// would wedge indefinitely with phase=running. ExecuteBeadWithConfig must
// now exit the attempt on its own within 60s, because the running-phase
// guard's harness-liveness watchdog detects the disappearance and cancels
// the attempt itself.
func TestWork_ClaudeSubprocessDeathEndToEndRecoversWithin60s(t *testing.T) {
	const beadID = "ddx-claude-subprocess-death"
	projectRoot, gitOps := setupProcessCleanupAttempt(t, beadID)
	dir := t.TempDir()
	runner := newClaudeDeathRunner(t, dir)
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{Harness: "claude"})

	type outcome struct {
		res *ExecuteBeadResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
			AgentRunner: runner,
		}, gitOps)
		done <- outcome{res: res, err: err}
	}()

	pid := <-runner.started
	waitForProviderChildren(t, os.Getpid(), pid)

	// Give the running-phase guard time to actually tick and record the
	// route-owned process as seen at least once before it disappears — the
	// watchdog deliberately never fires on a process it never observed
	// alive, so a kill before the first tick would be indistinguishable
	// from "still starting up" and prove nothing about recovery.
	time.Sleep(runningProviderGuardInterval + 500*time.Millisecond)

	// Simulate the claude harness subprocess crashing mid-attempt: kill it
	// directly. Deliberately do NOT cancel the attempt ourselves — recovery
	// must come from the worker's own harness-liveness watchdog, not from an
	// external actor.
	// ESRCH means the subprocess already died on its own (possible on a
	// loaded host) — which is exactly the condition the watchdog must
	// recover from, so proceed to observe recovery either way.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		t.Fatalf("kill fake claude subprocess: %v", err)
	}

	// Race-detector overhead plus CPU contention from concurrent agent
	// attempts can push recovery well past the idle-host budget; keep a
	// hard ceiling so genuine non-recovery still fails.
	recoveryDeadline := 60 * time.Second
	if raceEnabled {
		recoveryDeadline = 180 * time.Second
	}
	select {
	case out := <-done:
		if out.res == nil && out.err == nil {
			t.Fatal("expected either a result or an error describing the terminal outcome")
		}
		if out.res != nil && out.res.Status == ExecuteBeadStatusSuccess {
			t.Fatalf("a dead claude subprocess must not be reported as success: %+v", out.res)
		}
	case <-time.After(recoveryDeadline):
		t.Fatalf("ExecuteBeadWithConfig did not exit the attempt within %s of the claude subprocess dying", recoveryDeadline)
	}
}
