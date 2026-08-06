package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// sleepyRunner is an AgentRunner stand-in whose Run simply blocks for a fixed
// duration before succeeding — long enough for the running-phase guard to
// tick at least once mid-attempt, without spawning any real OS process.
type sleepyRunner struct {
	sleep time.Duration
}

func (r sleepyRunner) Run(RunArgs) (*Result, error) {
	time.Sleep(r.sleep)
	return &Result{ExitCode: 0}, nil
}

// TestDDXNeverScansOrSignalsFizeauOwnedProcesses proves the execute-bead
// path no longer performs provider-child census/signaling during an attempt.
// The scanner and terminator are stubbed to count invocations; after the
// change both counts must stay at zero even while a long-lived attempt runs.
func TestDDXNeverScansOrSignalsFizeauOwnedProcesses(t *testing.T) {
	const beadID = "ddx-provider-harness-fixture"
	projectRoot, gitOps := setupProcessCleanupAttempt(t, beadID)

	restoreScanner := providerChildScanner
	restoreTerminate := terminateProviderChild
	t.Cleanup(func() {
		providerChildScanner = restoreScanner
		terminateProviderChild = restoreTerminate
	})

	var scans atomic.Int32
	var terminations atomic.Int32
	providerChildScanner = func(context.Context, int, time.Time) ([]providerChildProcess, error) {
		scans.Add(1)
		return []providerChildProcess{{
			PID:       919192,
			Provider:  "claude",
			Command:   "/usr/local/bin/claude --print",
			StartedAt: time.Now().UTC(),
		}}, nil
	}
	terminateProviderChild = func(pid int) {
		terminations.Add(1)
	}

	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Harness: "codex"}).
		Resolve(config.TestBeadOverrides(config.TestBeadConfigOpts{Harness: "codex"}))

	runner := sleepyRunner{sleep: runningProviderGuardInterval + 500*time.Millisecond}

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, cfg, ExecuteBeadRuntime{
		AgentRunner: runner,
	}, gitOps)
	if err != nil {
		t.Fatalf("ExecuteBeadWithConfig: %v", err)
	}
	if res == nil {
		t.Fatal("ExecuteBeadWithConfig returned a nil result")
	}
	if got := scans.Load(); got != 0 {
		t.Fatalf("provider child scanner was called %d times; want 0", got)
	}
	if got := terminations.Load(); got != 0 {
		t.Fatalf("provider child terminator was called %d times; want 0", got)
	}
}
