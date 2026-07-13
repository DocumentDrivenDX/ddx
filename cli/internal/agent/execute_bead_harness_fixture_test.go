package agent

import (
	"context"
	"sync"
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

// TestExecuteBead_PreCommitProviderFixtureSurvivesRunningGuard exercises the
// full ExecuteBeadWithConfig attempt path with an active Codex route and a
// Claude fixture descendant owned by that route through ancestry (the shape
// of a Codex-run pre-commit/test suite spawning Claude as a fixture,
// ddx-44e89575's reproduction shape).
//
// The provider-child scanner and terminator are stubbed so the ancestry
// scenario is deterministic and reproducible without spawning real OS
// processes literally named "codex"/"claude": this host runs a live ddx
// worker (dogfooding this very repo) whose own, already-deployed
// running-phase guard scans the whole process tree by name and would treat
// a multi-second-lived, non-mocked "codex"/"claude" test process as its own
// non-route leak — exactly the bug this bead fixes, just in a different,
// not-yet-rebuilt process rather than in this test.
//
// It proves the running guard never terminates the fixture while the
// attempt is in flight, and that the attempt-end backstop still reaps it
// exactly once the attempt completes.
func TestExecuteBead_PreCommitProviderFixtureSurvivesRunningGuard(t *testing.T) {
	const beadID = "ddx-provider-harness-fixture"
	projectRoot, gitOps := setupProcessCleanupAttempt(t, beadID)

	const harnessPID = 919191
	const fixturePID = 919192

	restoreScanner := providerChildScanner
	restoreTerminate := terminateProviderChild
	t.Cleanup(func() {
		providerChildScanner = restoreScanner
		terminateProviderChild = restoreTerminate
	})

	providerChildScanner = func(context.Context, int, time.Time) ([]providerChildProcess, error) {
		return []providerChildProcess{{
			PID:              fixturePID,
			PPID:             harnessPID,
			Provider:         "claude",
			Command:          "/usr/local/bin/claude --print",
			StartedAt:        time.Now().UTC(),
			OwnerProviderPID: harnessPID,
			OwnerProvider:    "codex",
		}}, nil
	}

	var mu sync.Mutex
	var terminated []int
	terminateProviderChild = func(pid int) {
		mu.Lock()
		terminated = append(terminated, pid)
		mu.Unlock()
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

	mu.Lock()
	defer mu.Unlock()
	if len(terminated) != 1 || terminated[0] != fixturePID {
		t.Fatalf("expected exactly one attempt-end termination of the harness-owned fixture, got %v", terminated)
	}
}
