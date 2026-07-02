package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noisyNeverClosingService is a FizeauService stub whose Execute returns a
// caller-fed channel that the stub never closes on its own. It simulates the
// production failure mode this test guards against: a provider that keeps
// emitting unrelated "activity" (progress noise, retries, a non-route
// fallback spawn loop) after its actual route process has died, which would
// otherwise reset drainServiceEventsWithRenderer's idle timer indefinitely
// and mask the death for the full multi-hour idle timeout.
type noisyNeverClosingService struct {
	*passthroughTestService
	events chan agentlib.ServiceEvent
}

func (s *noisyNeverClosingService) Execute(context.Context, agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	return s.events, nil
}

// TestWork_ClaudeSubprocessDeathSynthesizesTerminalOutcome (ddx-f2b7cf89 AC2):
// proves that once something external (in production, the running-phase
// guard's harness-liveness watchdog) proves the route's own harness process
// died and cancels the attempt context, executeOnService synthesizes a
// terminal, typed outcome promptly — even while the provider keeps emitting
// unrelated events that would otherwise reset the idle-silence timeout
// indefinitely. Before wiring drainWatchdog.ctx to the caller's context, the
// drain loop had no way to notice this cancellation except by the idle timer
// firing, which continuous unrelated noise can suppress forever.
func TestWork_ClaudeSubprocessDeathSynthesizesTerminalOutcome(t *testing.T) {
	events := make(chan agentlib.ServiceEvent)
	svc := &noisyNeverClosingService{passthroughTestService: &passthroughTestService{}, events: events}
	rcfg := resolvedWithPassthrough("claude", "", "", 0, 0)

	ctx, cancel := context.WithCancel(context.Background())

	// Continuous unrelated noise: never a "final" event, never a channel
	// close, spaced well inside any idle window a real idle-timeout config
	// would use. If drainServiceEventsWithRenderer only watched the event
	// stream, this alone would keep the drain alive forever.
	noiseDone := make(chan struct{})
	go func() {
		defer close(noiseDone)
		for i := 0; ; i++ {
			data, _ := json.Marshal(map[string]any{
				"phase":   "tool",
				"state":   "complete",
				"task_id": fmt.Sprintf("noise-%d", i),
			})
			select {
			case <-ctx.Done():
				return
			case events <- agentlib.ServiceEvent{Type: "progress", Time: time.Now(), Data: data}:
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	// Simulate the harness-liveness watchdog: it observed the route process
	// die and, well before any generic idle timeout could plausibly fire,
	// cancels the attempt.
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := executeOnService(ctx, svc, t.TempDir(), rcfg, AgentRunRuntime{Prompt: "do the work"})
	elapsed := time.Since(start)
	<-noiseDone

	require.Error(t, err, "a cancelled attempt with no final event must not report success")
	var pfErr *ProviderFailureError
	require.ErrorAs(t, err, &pfErr, "must be a typed ProviderFailureError, not a generic error")
	assert.Equal(t, FailureModeProviderHarnessUnavailable, pfErr.Failure.Reason)
	assert.True(t, pfErr.Failure.Retryable, "a dead harness process is retryable: another route may be healthy")

	report := ExecuteBeadReport{Status: ExecuteBeadStatusExecutionFailed}
	ApplyProviderFailureToReport(&report, pfErr.Failure)
	assert.Equal(t, FailureModeProviderHarnessUnavailable, report.OutcomeReason)
	assert.True(t, report.Disrupted)

	assert.Less(t, elapsed, 5*time.Second,
		"drain must return promptly once the caller cancels ctx, not wait for the multi-hour idle timeout")
}

// TestRunningProviderGuardFiresHarnessDeadAfterRouteProcessDisappears proves
// the harness-liveness watchdog (SetHarnessDeadWatch) fires onDead exactly
// once, and only after the route-owned process has been observed at least
// once and then stayed missing for >= grace — never on startup (before the
// process has spawned) and never while the process is still present.
func TestRunningProviderGuardFiresHarnessDeadAfterRouteProcessDisappears(t *testing.T) {
	g := newRunningProviderGuard(t.TempDir(), "ddx-test", "attempt-1", 12345)
	fired := make(chan struct{}, 1)
	g.SetHarnessDeadWatch(30*time.Millisecond, func() {
		select {
		case fired <- struct{}{}:
		default:
		}
	})

	now := time.Now().UTC()

	// Route known, but the harness process has never been observed yet
	// (still starting up) — must not fire even after grace elapses.
	g.observeHarnessLiveness("claude/sonnet", "", nil, now)
	g.observeHarnessLiveness("claude/sonnet", "", nil, now.Add(100*time.Millisecond))
	select {
	case <-fired:
		t.Fatal("must not fire before the route-owned process has ever been observed")
	default:
	}

	// The route-owned process is observed: liveness confirmed.
	g.observeHarnessLiveness("claude/sonnet", "", []workerstatus.ProviderChild{
		{PID: 1, Provider: "claude", RouteOwner: "claude/sonnet"},
	}, now.Add(200*time.Millisecond))
	select {
	case <-fired:
		t.Fatal("must not fire while the route-owned process is present")
	default:
	}

	// It disappears. Must not fire immediately (grace not yet elapsed).
	missingSince := now.Add(210 * time.Millisecond)
	g.observeHarnessLiveness("claude/sonnet", "", nil, missingSince)
	select {
	case <-fired:
		t.Fatal("must not fire before grace elapses")
	default:
	}

	// Grace elapses with it still missing: must fire exactly once.
	g.observeHarnessLiveness("claude/sonnet", "", nil, missingSince.Add(40*time.Millisecond))
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("onHarnessDead must fire once the route-owned process has been missing for >= grace")
	}

	// Further ticks after firing must not fire again.
	g.observeHarnessLiveness("claude/sonnet", "", nil, missingSince.Add(80*time.Millisecond))
	select {
	case <-fired:
		t.Fatal("onHarnessDead must fire at most once per guard")
	case <-time.After(50 * time.Millisecond):
	}
}
