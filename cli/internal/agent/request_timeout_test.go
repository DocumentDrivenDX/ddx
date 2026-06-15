package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkRequestTimeoutCancelsActiveToolSession proves the absolute
// request-timeout cap cancels a provider session that keeps emitting tool
// events past the window — the cap is NOT reset by activity the way the
// idle-read and per-tool-call timers are (ddx-9febbad2).
func TestWorkRequestTimeoutCancelsActiveToolSession(t *testing.T) {
	var cancelCalled atomic.Bool
	var observedElapsed atomic.Int64
	cancel := func() { cancelCalled.Store(true) }

	const requestTimeout = 200 * time.Millisecond
	events := make(chan agentlib.ServiceEvent)

	wd := &drainWatchdog{
		cancel:          cancel,
		idleTimeout:     30 * time.Second, // generous — must not fire
		toolCallTimeout: 30 * time.Second, // generous — must not fire
		requestTimeout:  requestTimeout,
		onRequestTimeout: func(elapsed time.Duration) {
			observedElapsed.Store(int64(elapsed))
		},
	}

	// Fake provider: emit distinct tool_call/tool_result pairs every 20ms, far
	// past the 200ms request timeout. Distinct payloads keep the loop detector
	// quiet and prompt tool_results keep the tool-call timer quiet, so only the
	// absolute cap can end the drain.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				i++
				select {
				case events <- makeToolCallEvent(fmt.Sprintf("tc%d", i), "Bash", fmt.Sprintf(`{"command":"echo %d"}`, i)):
				case <-stop:
					return
				}
				select {
				case events <- makeToolResultEvent(fmt.Sprintf("tc%d", i), fmt.Sprintf("out %d", i), ""):
				case <-stop:
					return
				}
			}
		}
	}()

	start := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), wd, nil)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("drain never returned; absolute request-timeout cap did not fire")
	}
	elapsed := time.Since(start)

	assert.True(t, cancelCalled.Load(), "request-timeout cap must cancel the active tool session")
	assert.Positive(t, observedElapsed.Load(), "onRequestTimeout must be invoked with the elapsed wall-clock time")
	assert.Less(t, elapsed, 2*time.Second,
		"cap must fire at the absolute timeout despite continuous tool activity, not reset on each event")
}

// TestRequestTimeoutEvidenceIncludesWallClockFields proves the durable
// request-timeout evidence names the configured cap, elapsed wall-clock, phase,
// reaped provider PID, cleanup result, and reason=request_timeout.
func TestRequestTimeoutEvidenceIncludesWallClockFields(t *testing.T) {
	now := time.Now().UTC()
	const providerPID = 515151

	restoreScanner := providerChildScanner
	restoreTerminate := terminateProviderChild
	t.Cleanup(func() {
		providerChildScanner = restoreScanner
		terminateProviderChild = restoreTerminate
	})
	providerChildScanner = func(context.Context, int, time.Time) ([]providerChildProcess, error) {
		return []providerChildProcess{{
			PID:       providerPID,
			Provider:  "claude",
			Command:   "/usr/local/bin/claude",
			StartedAt: now.Add(-30 * time.Second),
		}}, nil
	}
	terminateProviderChild = func(int) {}

	projectRoot := t.TempDir()
	const attemptID = "attempt-rt-evidence"
	const configured = 25 * time.Minute
	const elapsed = 56 * time.Minute

	ev := reapRequestTimeoutAttempt(projectRoot, attemptID, "ddx-bead", "do", "", os.Getpid(), configured, elapsed, now)

	assert.Equal(t, reasonRequestTimeout, ev.Reason)
	assert.Equal(t, "do", ev.Phase)
	assert.Equal(t, providerPID, ev.ProviderPID)
	assert.Equal(t, requestTimeoutCleanupReaped, ev.CleanupResult)

	path := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID, requestTimeoutArtifact)
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "request-timeout evidence must be written under the attempt bundle")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	for _, key := range []string{"configured_timeout", "elapsed", "phase", "provider_pid", "cleanup_result", "reason"} {
		_, ok := decoded[key]
		assert.Truef(t, ok, "evidence must include wall-clock field %q", key)
	}
	assert.Equal(t, reasonRequestTimeout, decoded["reason"])
	assert.Equal(t, "do", decoded["phase"])
	assert.Equal(t, requestTimeoutCleanupReaped, decoded["cleanup_result"])
	assert.Equal(t, float64(providerPID), decoded["provider_pid"])
	assert.Equal(t, configured.String(), decoded["configured_timeout"])
	assert.Equal(t, elapsed.String(), decoded["elapsed"])
}
