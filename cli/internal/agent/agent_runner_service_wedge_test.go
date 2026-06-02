package agent

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeToolCallEvent constructs a tool_call ServiceEvent with the given id, name, and input JSON.
func makeToolCallEvent(id, name, inputJSON string) agentlib.ServiceEvent {
	data, _ := json.Marshal(map[string]any{
		"id":    id,
		"name":  name,
		"input": json.RawMessage(inputJSON),
	})
	return agentlib.ServiceEvent{
		Type: agentlib.ServiceEventTypeToolCall,
		Time: time.Now(),
		Data: data,
	}
}

// makeToolResultEvent constructs a tool_result ServiceEvent.
func makeToolResultEvent(id, output, errStr string) agentlib.ServiceEvent {
	data, _ := json.Marshal(map[string]any{
		"id":     id,
		"output": output,
		"error":  errStr,
	})
	return agentlib.ServiceEvent{
		Type: agentlib.ServiceEventTypeToolResult,
		Time: time.Now(),
		Data: data,
	}
}

// makeProgressEvent constructs a progress ServiceEvent (non-meaningful for idle reset).
func makeProgressEvent(phase, state string) agentlib.ServiceEvent {
	data, _ := json.Marshal(map[string]any{
		"phase": phase,
		"state": state,
	})
	return agentlib.ServiceEvent{
		Type: agentlib.ServiceEventTypeProgress,
		Time: time.Now(),
		Data: data,
	}
}

// makeTextDeltaEvent constructs a text_delta ServiceEvent (non-meaningful for idle reset).
func makeTextDeltaEvent(text string) agentlib.ServiceEvent {
	data, _ := json.Marshal(map[string]any{"text": text})
	return agentlib.ServiceEvent{
		Type: agentlib.ServiceEventTypeTextDelta,
		Time: time.Now(),
		Data: data,
	}
}

// makeFinalEvent constructs a final ServiceEvent.
func makeFinalEvent(status string) agentlib.ServiceEvent {
	data, _ := json.Marshal(map[string]any{
		"status":    status,
		"exit_code": 0,
	})
	return agentlib.ServiceEvent{
		Type: agentlib.ServiceEventTypeFinal,
		Time: time.Now(),
		Data: data,
	}
}

// TestIdleTimer_TrueSilenceFiresTimeout asserts that the idle timer fires when
// NO events arrive for the idle window — complete silence is the only trigger.
func TestIdleTimer_TrueSilenceFiresTimeout(t *testing.T) {
	var cancelCalled atomic.Bool
	cancel := func() { cancelCalled.Store(true) }

	events := make(chan agentlib.ServiceEvent, 8)
	// No events are sent — pure silence.

	wd := &drainWatchdog{
		cancel:      cancel,
		idleTimeout: 60 * time.Millisecond,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), wd, nil)
	}()

	require.Eventually(t, func() bool {
		return cancelCalled.Load()
	}, 500*time.Millisecond, 5*time.Millisecond, "idle timer must fire after complete silence")

	close(events)
	<-done
}

// TestIdleTimer_ProgressEventsResetTimer asserts that tool_call, tool_result, and
// progress events each reset the idle timer. A stream of these events running for
// longer than the idle window must NOT trigger cancellation; only complete silence
// (no events of any kind) does.
func TestIdleTimer_ProgressEventsResetTimer(t *testing.T) {
	var cancelCalled atomic.Bool
	cancel := func() { cancelCalled.Store(true) }

	const idleTimeout = 80 * time.Millisecond
	events := make(chan agentlib.ServiceEvent, 64)

	wd := &drainWatchdog{
		cancel:          cancel,
		idleTimeout:     idleTimeout,
		toolCallTimeout: 5 * time.Second, // generous; will not fire during test
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), wd, nil)
	}()

	// Send tool_call/tool_result/progress events for 3× the idle window with
	// spacing well within the idle window. No "final" event is sent during this
	// period. If any of these event types fail to reset the idle timer, cancel
	// would be called within ~80 ms.
	spacing := idleTimeout / 3
	for i := 0; i < 9; i++ {
		time.Sleep(spacing)
		switch i % 3 {
		case 0:
			events <- makeToolCallEvent(fmt.Sprintf("tc%d", i), "Bash", `{"command":"echo ok"}`)
		case 1:
			events <- makeToolResultEvent(fmt.Sprintf("tc%d", i), "ok", "")
		default:
			events <- makeProgressEvent("work", fmt.Sprintf("step %d", i))
		}
	}

	assert.False(t, cancelCalled.Load(), "tool_call/tool_result/progress events must reset the idle timer — no cancellation while events keep arriving")

	events <- makeFinalEvent("success")
	close(events)
	<-done
}

// TestLoopDetector_FiresAfterRepeatedIdenticalCommands asserts that EarlyCancel
// fires after 4 identical (command, result) pairs appear in the last 8 tool calls.
func TestLoopDetector_FiresAfterRepeatedIdenticalCommands(t *testing.T) {
	var cancelCalled atomic.Bool
	cancel := func() { cancelCalled.Store(true) }

	events := make(chan agentlib.ServiceEvent, 64)

	wd := &drainWatchdog{
		cancel:      cancel,
		idleTimeout: 30 * time.Second, // long enough that it won't fire during the test
	}

	// Three distinct pairs first (won't trigger loop detection).
	for i := 0; i < 3; i++ {
		events <- makeToolCallEvent("id1", "Bash", `{"command":"echo hello"}`)
		events <- makeToolResultEvent("id1", "hello", "")
		// Vary the output so these are not all the same key.
		events <- makeToolCallEvent("id2", "Bash", `{"command":"echo world"}`)
		events <- makeToolResultEvent("id2", "world", "")
	}
	// Ensure cancel is still false at this point (we haven't reached 4 identical pairs).

	// Now send 4 identical pairs. The fourth should trigger loop detection.
	for i := 0; i < 4; i++ {
		events <- makeToolCallEvent("loop", "Bash", `{"command":"go test ./..."}`)
		events <- makeToolResultEvent("loop", "FAIL: tests failed", "exit code 1")
	}

	close(events)

	drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), wd, nil)

	assert.True(t, cancelCalled.Load(), "loop detector must call cancel after ≥4 identical (command, result) pairs in the window")
}

// TestToolCallTimeout_KillsLongHungSubprocess asserts that a single tool_call
// without a matching tool_result within ToolCallTimeout triggers cancellation.
func TestToolCallTimeout_KillsLongHungSubprocess(t *testing.T) {
	var cancelCalled atomic.Bool
	cancel := func() { cancelCalled.Store(true) }

	events := make(chan agentlib.ServiceEvent, 8)

	wd := &drainWatchdog{
		cancel:          cancel,
		idleTimeout:     30 * time.Second,      // won't fire during test
		toolCallTimeout: 60 * time.Millisecond, // short for test
	}

	// Send a tool_call. No matching tool_result will follow within the timeout.
	events <- makeToolCallEvent("hung", "Bash", `{"command":"git push"}`)

	done := make(chan struct{})
	go func() {
		defer close(done)
		drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), wd, nil)
	}()

	// The tool-call timeout must fire and cancel within 500 ms.
	require.Eventually(t, func() bool {
		return cancelCalled.Load()
	}, 500*time.Millisecond, 5*time.Millisecond, "tool call timeout must fire when tool_result never arrives")

	// Clean up the goroutine.
	close(events)
	<-done
}

// TestWedgeImmune_HardWorkRunsToCompletion simulates a long stream of
// regularly-progressing (distinct) tool_use → tool_result pairs and asserts
// that none of the wedge-prevention mechanisms fires: the execution must
// complete naturally without being cancelled.
func TestWedgeImmune_HardWorkRunsToCompletion(t *testing.T) {
	var cancelCalled atomic.Bool
	cancel := func() { cancelCalled.Store(true) }

	// Use a short idle timeout so the test runs quickly, but ensure that
	// regularly-spaced distinct tool_call events keep resetting it.
	const idleTimeout = 80 * time.Millisecond

	events := make(chan agentlib.ServiceEvent, 256)

	wd := &drainWatchdog{
		cancel:          cancel,
		idleTimeout:     idleTimeout,
		toolCallTimeout: 5 * time.Second, // generous per-call cap
	}

	// Simulate 20 rounds of distinct tool_call → tool_result with a small
	// sleep between rounds to exercise the idle timer reset path. Each round
	// produces a unique output so the loop detector never fires.
	go func() {
		for i := 0; i < 20; i++ {
			time.Sleep(idleTimeout / 3) // well within the idle window
			events <- makeToolCallEvent("tc", "Bash", `{"command":"go test ./..."}`)
			events <- makeToolResultEvent("tc", fmt.Sprintf("iteration %d ok", i), "")
		}
		events <- makeFinalEvent("success")
		close(events)
	}()

	final, _, _ := drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), wd, nil)

	assert.False(t, cancelCalled.Load(), "hard work with distinct results must NOT trigger any wedge-prevention mechanism")
	require.NotNil(t, final, "final event must be received when execution completes naturally")
	assert.Equal(t, "success", final.Status)
}
