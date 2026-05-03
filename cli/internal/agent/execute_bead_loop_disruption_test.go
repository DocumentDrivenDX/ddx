package agent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// disruptionEventCapture is an in-memory capture of structured loop events
// emitted via writeLoopEvent. Tests use this to assert the
// `disruption_detected` event surfaces with the expected reason kind.
type disruptionEventCapture struct {
	mu     atomic.Value
	lines  []string
	wrote  int32
	wrErr  error
	closed atomic.Bool
}

func (c *disruptionEventCapture) Write(p []byte) (int, error) {
	if c.wrErr != nil {
		return 0, c.wrErr
	}
	atomic.AddInt32(&c.wrote, int32(len(p)))
	cur, _ := c.mu.Load().([]string)
	c.mu.Store(append(cur, string(p)))
	return len(p), nil
}

func (c *disruptionEventCapture) all() []string {
	cur, _ := c.mu.Load().([]string)
	return cur
}

// TestLoop_DisruptedExecution_NoCooldown asserts ddx-5b3e57f4 AC #1, #3, #7:
// when the executor returns a context.Canceled error mid-execution, the loop
// classifies the failure as Disrupted, does NOT call SetExecutionCooldown,
// and leaves the bead immediately re-claimable.
func TestLoop_DisruptedExecution_NoCooldown(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	cancelCtx, cancel := context.WithCancel(context.Background())
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			cancel()
			// Simulate the worker being killed during execution: BaseRev
			// snapshot was taken but no commit was made, so BaseRev ==
			// ResultRev. Without the Disrupted classification the loop
			// would mistake this for a genuine no_changes outcome and
			// park the bead under noProgressCooldown.
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "context canceled",
				BaseRev:   "abc1234",
				ResultRev: "abc1234",
			}, context.Canceled
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(cancelCtx, rcfg, ExecuteBeadLoopRuntime{Once: true})
	// ctx was cancelled mid-run; the loop returns the cancel error after
	// the iteration completes. We assert on the result regardless.
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	require.NotNil(t, result)
	require.GreaterOrEqual(t, len(result.Results), 1)
	report := result.Results[0]
	assert.True(t, report.Disrupted, "report must be marked Disrupted on context.Canceled")
	assert.Equal(t, "context_canceled", report.DisruptionReason)
	assert.Empty(t, report.RetryAfter,
		"Disrupted report must NOT carry a retry_after — no cooldown applied")

	got, err := store.Get(candidate.ID)
	require.NoError(t, err)
	if got.Extra != nil {
		_, hasRetry := got.Extra["execute-loop-retry-after"]
		assert.False(t, hasRetry,
			"Disrupted bead must not have execute-loop-retry-after persisted")
	}
}

// TestLoop_GenuineNoProgress_StillCooldowns asserts ddx-5b3e57f4 AC #4: a
// model that returns clean (no error) with BaseRev == ResultRev and no
// Disrupted marker still hits the noProgressCooldown branch. This proves the
// disruption fix is targeted to disrupted attempts and does NOT defang the
// genuine no-progress path.
func TestLoop_GenuineNoProgress_StillCooldowns(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusNoChanges,
				SessionID: "sess-noprog",
				BaseRev:   "feedface00112233",
				ResultRev: "feedface00112233",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	report := result.Results[0]
	assert.False(t, report.Disrupted,
		"clean no_changes return is not disrupted — must keep current cooldown semantics")
	require.NotEmpty(t, report.RetryAfter,
		"genuine no-progress must still be parked under noProgressCooldown")

	got, err := store.Get(candidate.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Extra)
	_, hasRetry := got.Extra["execute-loop-retry-after"]
	assert.True(t, hasRetry,
		"non-disrupted no_changes bead must have execute-loop-retry-after persisted")
}

// TestLoop_PreflightRejection_NoCooldown asserts ddx-5b3e57f4 AC #1: a
// routing preflight rejection is classified as Disrupted (operator config
// issue, not model failure). The bead must not be parked under any cooldown.
func TestLoop_PreflightRejection_NoCooldown(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run on preflight rejection")
			return ExecuteBeadReport{}, nil
		}),
	}
	rejected := agentlib.ErrHarnessModelIncompatible{
		Harness: "claude", Model: "gpt-5", SupportedModels: []string{"claude-opus-4-7"},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", Harness: "claude", Model: "gpt-5"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		RoutePreflight: func(ctx context.Context, harness, model string) error {
			return rejected
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)

	report := result.Results[0]
	assert.True(t, report.Disrupted, "preflight rejection must be Disrupted")
	assert.Equal(t, "preflight_rejected", report.DisruptionReason)
	assert.Empty(t, report.RetryAfter,
		"preflight-rejected bead must not carry retry_after")

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	if got.Extra != nil {
		_, hasRetry := got.Extra["execute-loop-retry-after"]
		assert.False(t, hasRetry,
			"preflight-rejected bead must not have execute-loop-retry-after persisted")
	}
}

// TestLoop_DisruptionEventEmitted asserts ddx-5b3e57f4 AC #5: a
// `disruption_detected` event is appended to the bead and to the loop event
// sink when a Disrupted classification fires.
func TestLoop_DisruptionEventEmitted(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	transportErr := errors.New("dial tcp 127.0.0.1:443: connection refused")
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    transportErr.Error(),
				BaseRev:   "deadbeef",
				ResultRev: "deadbeef",
			}, transportErr
		}),
	}

	sink := &disruptionEventCapture{}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: sink,
		SessionID: "sess-disrupt",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	report := result.Results[0]
	assert.True(t, report.Disrupted, "transport error must be classified Disrupted")
	assert.Equal(t, "transport_error", report.DisruptionReason)
	assert.Empty(t, report.RetryAfter, "transport-error Disrupted bead must skip cooldown")

	// Sink event surface
	var found bool
	for _, line := range sink.all() {
		if strings.Contains(line, `"type":"disruption_detected"`) &&
			strings.Contains(line, `"reason":"transport_error"`) {
			found = true
			break
		}
	}
	assert.True(t, found, "disruption_detected event with reason=transport_error must be emitted to event sink; got: %v", sink.all())

	// Bead event surface
	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	var beadEv *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "disruption_detected" {
			beadEv = &events[i]
			break
		}
	}
	require.NotNil(t, beadEv, "disruption_detected event must be appended to the bead")
	assert.Equal(t, "transport_error", beadEv.Summary)
}

// TestClassifyDisruption_Markers asserts the transport-error marker set
// recognizes a representative sample of disruption-class strings, and that
// non-transport errors are not misclassified.
func TestClassifyDisruption_Markers(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		want   bool
		reason string
	}{
		{"connection_refused", errors.New("dial tcp: connection refused"), true, "transport_error"},
		{"connection_reset", errors.New("read: connection reset by peer"), true, "transport_error"},
		{"deadline_exceeded", errors.New("Post: context deadline exceeded"), true, "transport_error"},
		{"bad_gateway", errors.New("502 bad gateway from upstream"), true, "transport_error"},
		{"plain_error", errors.New("model declined to commit"), false, ""},
		{"nil_err", nil, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, ok := classifyDisruption(context.Background(), tc.err)
			assert.Equal(t, tc.want, ok)
			assert.Equal(t, tc.reason, reason)
		})
	}
}

// TestClassifyDisruption_ContextErrors covers the ctx.Err() branch of
// classifyDisruption: cancelled and deadline-exceeded contexts must classify
// as Disrupted regardless of executorErr.
func TestClassifyDisruption_ContextErrors(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		reason, ok := classifyDisruption(ctx, nil)
		assert.True(t, ok)
		assert.Equal(t, "context_canceled", reason)
	})
	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		reason, ok := classifyDisruption(ctx, nil)
		assert.True(t, ok)
		assert.Equal(t, "context_deadline", reason)
	})
}
