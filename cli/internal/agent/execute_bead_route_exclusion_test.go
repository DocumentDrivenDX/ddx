package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routeReleaseFailingStore struct {
	ExecuteBeadLoopStore
	err error
}

func (s *routeReleaseFailingStore) Release(_, _, _ string) error { return s.err }

type routeReleaseFlakyStore struct {
	ExecuteBeadLoopStore
	releaser leaseReleaser
	calls    int
	err      error
}

func (s *routeReleaseFlakyStore) Release(id, assignee, status string) error {
	s.calls++
	if s.calls == 1 {
		return s.err
	}
	return s.releaser.Release(id, assignee, status)
}

func TestIsProviderConnectivityFailureReport_Discriminates(t *testing.T) {
	cases := []struct {
		name   string
		report ExecuteBeadReport
		want   bool
	}{
		{"dial_tcp_timeout", ExecuteBeadReport{
			Status: ExecuteBeadStatusExecutionFailed, Provider: "bragi",
			Detail: "dial tcp 100.127.38.115:1234: i/o timeout"}, true},
		{"connection_refused", ExecuteBeadReport{
			Status: ExecuteBeadStatusExecutionFailed, Provider: "bragi",
			Error: "Post \"http://bragi:1234/v1/chat\": dial tcp: connection refused"}, true},
		{"concrete_route_omitted", ExecuteBeadReport{
			Status: ExecuteBeadStatusExecutionFailed,
			Detail: "dial tcp 1.2.3.4:80: i/o timeout"}, true},
		{"no_viable_provider_path_owned_elsewhere", ExecuteBeadReport{
			Status: ExecuteBeadStatusExecutionFailed, Provider: "bragi",
			Detail: "routing: no viable routing candidate: 3 candidates rejected"}, false},
		{"success_status", ExecuteBeadReport{
			Status: ExecuteBeadStatusSuccess, Provider: "bragi",
			Detail: "dial tcp i/o timeout"}, false},
		{"unrelated_error", ExecuteBeadReport{
			Status: ExecuteBeadStatusExecutionFailed, Provider: "bragi",
			Detail: "build failed"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isProviderConnectivityFailureReport(tc.report))
		})
	}
}

func TestProviderConnectivityClassificationIgnoresConcreteRouteIdentity(t *testing.T) {
	base := ExecuteBeadReport{
		Status:        ExecuteBeadStatusExecutionFailed,
		OutcomeReason: "execution_failed",
		ActualPower:   7,
		Error:         "Post http://opaque.invalid/v1: dial tcp: connection refused",
	}

	for _, actual := range []struct {
		name, harness, provider, model string
	}{
		{name: "omitted"},
		{name: "route_a", harness: "harness-a", provider: "provider-a", model: "model-a"},
		{name: "route_b", harness: "harness-b", provider: "provider-b", model: "model-b"},
	} {
		t.Run(actual.name, func(t *testing.T) {
			report := base
			report.Harness = actual.harness
			report.Provider = actual.provider
			report.Model = actual.model
			assert.True(t, isProviderConnectivityFailureReport(report),
				"concrete returned identity must not steer disruption/retry/triage control flow")
		})
	}
}

func TestRouteResolutionTimeoutDefaultIs60s(t *testing.T) {
	require.Equal(t, 60*time.Second, DefaultRouteResolutionTimeout)

	var rt ExecuteBeadLoopRuntime
	assert.Equal(t, 60*time.Second, rt.effectiveRouteResolutionTimeout())

	rt.RouteResolutionTimeout = 5 * time.Second
	assert.Equal(t, 5*time.Second, rt.effectiveRouteResolutionTimeout())
}

func TestRouteStageTimeoutReleasesLeaseAfterOpaqueFizeauDispatch(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	entered := make(chan struct{}, 1)
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			onExecuteStart := onExecuteStartFromContext(ctx)
			if onExecuteStart == nil {
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, assert.AnError
			}
			onExecuteStart()
			select {
			case entered <- struct{}{}:
			default:
			}
			<-ctx.Done()
			cancelRun()
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, ctx.Err()
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-a"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	done := make(chan error, 1)
	go func() {
		_, err := worker.Run(runCtx, rcfg, ExecuteBeadLoopRuntime{
			Once:                   true,
			RouteResolutionTimeout: 25 * time.Millisecond,
		})
		done <- err
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("executor did not start")
	}
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			require.NoError(t, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not return after route-stage timeout")
	}

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var attention *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "operator_attention" && events[i].Summary == FailureModeRouteResolutionTimeout {
			attention = &events[i]
			break
		}
	}
	require.NotNil(t, attention, "route-stage timeout must emit operator_attention")
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(attention.Body), &body))
	assert.Equal(t, first.ID, body["bead_id"])
	assert.NotEmpty(t, body["attempt_id"])
	assert.NotEmpty(t, body["last_activity_at"])
	assert.Contains(t, body["diagnosis"], "before routing_decision")
}

func TestRouteStageTimeoutReleaseFailureDoesNotClaimLeaseReleased(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	releaseErr := errors.New("tracker lock unavailable")
	failing := &routeReleaseFailingStore{ExecuteBeadLoopStore: store, err: releaseErr}
	require.NoError(t, store.Claim(first.ID, "worker-a"))

	report, err := routeResolutionTimeoutReport(
		failing,
		first.ID,
		"worker-a",
		"attempt-route",
		time.Now().UTC(),
		time.Second,
		time.Now().UTC(),
	)
	require.ErrorIs(t, err, releaseErr)
	assert.Empty(t, report.OutcomeReason, "failed release must not produce a handled timeout report")

	after, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusInProgress, after.Status)
	assert.Equal(t, "worker-a", after.Owner)
	assert.Zero(t, readWedgeMarker(after.Extra).Count, "failed release must not advance the consecutive-wedge guard")
	events, err := store.Events(first.ID)
	require.NoError(t, err)
	for _, event := range events {
		if event.Kind == "operator_attention" && event.Summary == FailureModeRouteResolutionTimeout {
			t.Fatalf("failed release was reported as successful: %+v", event)
		}
	}
}

func TestRouteStageTimeoutReleaseFailureFallsBackToNormalAttemptCleanup(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	flaky := &routeReleaseFlakyStore{
		ExecuteBeadLoopStore: store,
		releaser:             store,
		err:                  errors.New("transient tracker lock contention"),
	}
	recordConsecutiveWedge(store, first.ID, FailureModeProgressWatchdog, time.Now().UTC())
	worker := &ExecuteBeadWorker{
		Store: flaky,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			onExecuteStartFromContext(ctx)()
			<-ctx.Done()
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, ctx.Err()
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-a"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:                   true,
		RouteResolutionTimeout: 25 * time.Millisecond,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, flaky.calls, 2, "failed guard release must not suppress ordinary claim cleanup")
	after, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, after.Status)
	assert.Empty(t, after.Owner)
	assert.Equal(t, 1, readWedgeMarker(after.Extra).Count,
		"failed timeout release must not erase prior durable wedge evidence")

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	for _, event := range events {
		if event.Kind == "operator_attention" && event.Summary == FailureModeRouteResolutionTimeout {
			t.Fatalf("guard release failure was recorded as a handled timeout: %+v", event)
		}
	}
}

func TestWorkerDoesNotImmediatelyReclaimRouteTimedOutBead(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	var firstAttempts, secondAttempts int
	secondEntered := make(chan struct{}, 1)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			switch beadID {
			case first.ID:
				firstAttempts++
				onExecuteStart := onExecuteStartFromContext(ctx)
				if onExecuteStart == nil {
					return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, assert.AnError
				}
				onExecuteStart()
				<-ctx.Done()
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, ctx.Err()
			case second.ID:
				secondAttempts++
				secondEntered <- struct{}{}
				cancelRun()
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, context.Canceled
			default:
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, assert.AnError
			}
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-a"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	done := make(chan error, 1)
	go func() {
		_, err := worker.Run(runCtx, rcfg, ExecuteBeadLoopRuntime{
			Mode:                   executeloop.ModeDrain,
			RouteResolutionTimeout: 25 * time.Millisecond,
		})
		done <- err
	}()

	select {
	case <-secondEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not continue to the second ready bead after route timeout")
	}
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			require.NoError(t, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after test cancellation")
	}

	assert.Equal(t, 1, firstAttempts, "route-timed-out bead must be skipped for the rest of the drain pass")
	assert.Equal(t, 1, secondAttempts, "worker must continue draining a sibling bead")
	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	assert.Equal(t, 1, readWedgeMarker(got.Extra).Count, "durable wedge evidence must remain for a later Run invocation")
}

func TestRouteStageTimeoutDisarmsAfterRoutingDecision(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			onExecuteStart := onExecuteStartFromContext(ctx)
			if onExecuteStart == nil {
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, assert.AnError
			}
			onExecuteStart()
			onRouteResolved := onRouteResolvedFromContext(ctx)
			if onRouteResolved == nil {
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, assert.AnError
			}
			onRouteResolved("opaque-harness", "opaque-provider", "opaque-model")
			time.Sleep(75 * time.Millisecond)
			return ExecuteBeadReport{
				BeadID:        beadID,
				Status:        ExecuteBeadStatusExecutionFailed,
				OutcomeReason: "tests_red",
				Detail:        "semantic failure after provider execution",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-a"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:                   true,
		RouteResolutionTimeout: 20 * time.Millisecond,
	})
	require.NoError(t, err)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	for _, event := range events {
		if event.Kind == "operator_attention" && event.Summary == FailureModeRouteResolutionTimeout {
			t.Fatalf("route timeout fired after routing_decision: %+v", event)
		}
	}
}

func TestRouteStageTimeoutDoesNotArmDuringLocalPreDispatchWork(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			// This delay represents DDx-local worktree/prompt preparation. It is
			// deliberately longer than the route budget and occurs before the
			// explicit Fizeau Execute boundary signal.
			time.Sleep(75 * time.Millisecond)
			onExecuteStart := onExecuteStartFromContext(ctx)
			onRouteResolved := onRouteResolvedFromContext(ctx)
			if onExecuteStart == nil || onRouteResolved == nil {
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, assert.AnError
			}
			onExecuteStart()
			onRouteResolved("opaque-harness", "opaque-provider", "opaque-model")
			return ExecuteBeadReport{
				BeadID:        beadID,
				Status:        ExecuteBeadStatusExecutionFailed,
				OutcomeReason: "tests_red",
				Detail:        "semantic failure after resolved route",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-a"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:                   true,
		RouteResolutionTimeout: 20 * time.Millisecond,
	})
	require.NoError(t, err)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	for _, event := range events {
		if event.Kind == "operator_attention" && event.Summary == FailureModeRouteResolutionTimeout {
			t.Fatalf("route timeout armed during DDx-local preparation: %+v", event)
		}
	}
}

func TestRouteStageTimeoutUsesExecuteDeadlineWhenGuardStartIsDelayed(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	guardStart := make(chan struct{})
	executeStarted := make(chan struct{}, 1)
	attemptCancelled := make(chan struct{}, 1)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			onExecuteStart := onExecuteStartFromContext(ctx)
			if onExecuteStart == nil {
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, assert.AnError
			}
			onExecuteStart()
			executeStarted <- struct{}{}
			<-ctx.Done()
			attemptCancelled <- struct{}{}
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed}, ctx.Err()
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-a"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	done := make(chan error, 1)
	go func() {
		_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:                   true,
			RouteResolutionTimeout: 300 * time.Millisecond,
			routeGuardStartGate:    guardStart,
		})
		done <- err
	}()

	select {
	case <-executeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("executor did not reach the Fizeau Execute boundary")
	}
	// Hold the guard longer than the route budget. A guard-relative timer would
	// incorrectly grant a fresh 300ms after this gate opens.
	time.Sleep(450 * time.Millisecond)
	releasedAt := time.Now()
	close(guardStart)
	select {
	case <-attemptCancelled:
	case <-time.After(150 * time.Millisecond):
		t.Fatal("expired Execute-relative deadline was restarted when the delayed guard woke")
	}
	assert.Less(t, time.Since(releasedAt), 150*time.Millisecond)
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			require.NoError(t, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not finish timeout bookkeeping")
	}

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	for _, event := range events {
		if event.Kind == "operator_attention" && event.Summary == FailureModeRouteResolutionTimeout {
			return
		}
	}
	t.Fatal("delayed guard did not emit route-resolution operator attention")
}

func TestParseProviderConnectivityFacts(t *testing.T) {
	cases := []struct {
		name             string
		report           ExecuteBeadReport
		wantEndpoint     string
		wantTimeoutClass string
	}{
		{
			name: "vidar_io_timeout",
			report: ExecuteBeadReport{
				Error: "openai POST http://vidar:1235/v1/chat/completions dial tcp 100.108.162.80:1235 i/o timeout",
			},
			wantEndpoint:     "http://vidar:1235/v1/chat/completions",
			wantTimeoutClass: "i/o timeout",
		},
		{
			name: "quoted_go_url_error",
			report: ExecuteBeadReport{
				Error: "Post \"http://bragi:1234/v1/chat\": dial tcp: connection refused",
			},
			wantEndpoint:     "http://bragi:1234/v1/chat",
			wantTimeoutClass: "connection refused",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, timeoutClass := parseProviderConnectivityFacts(tc.report)
			assert.Equal(t, tc.wantEndpoint, endpoint)
			assert.Equal(t, tc.wantTimeoutClass, timeoutClass)
		})
	}
}

func TestEmitRouteFailureEvent_IncludesEndpointAndTimeoutClass(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	report := ExecuteBeadReport{
		BeadID:   first.ID,
		Status:   ExecuteBeadStatusExecutionFailed,
		Provider: "vidar",
		Model:    "Qwen3.6-27B-MLX-8bit",
		Error:    "openai POST http://vidar:1235/v1/chat/completions dial tcp 100.108.162.80:1235 i/o timeout",
	}

	emitRouteFailureEvent(store, first.ID, "actor", report, time.Now().UTC())

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var body map[string]any
	for _, event := range events {
		if event.Kind == "route-failure" {
			require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
			break
		}
	}
	require.NotNil(t, body)
	assert.Equal(t, "http://vidar:1235/v1/chat/completions", body["endpoint"])
	assert.Equal(t, "i/o timeout", body["timeout_class"])
}
