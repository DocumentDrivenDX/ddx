package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteBeadWorker_ProviderTimeoutRetriesWithRouteExclusion exercises
// the route-exclusion path: a service attempt fails with a TCP-level
// provider-connectivity timeout against a routed provider, and the loop
// records structured route-failure evidence plus a power-hint bump so a
// subsequent attempt's routing query naturally excludes the failed
// (provider, model) tuple. The retry preserves operator intent: no
// hardcoded provider/policy pins are written.
func TestExecuteBeadWorker_ProviderTimeoutRetriesWithRouteExclusion(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	frozen := time.Date(2026, 5, 14, 8, 8, 30, 0, time.UTC)
	var floorCalls []int
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusExecutionFailed,
				Harness:     "fiz",
				Provider:    "bragi",
				Model:       "qwen3.5-27b",
				ActualPower: 50,
				Detail:      "agent: execute: provider request failed: dial tcp 100.127.38.115:1234: i/o timeout",
				Error:       "dial tcp 100.127.38.115:1234: i/o timeout",
				BaseRev:     "aaaa1111",
				ResultRev:   "aaaa1111",
			}, nil
		}),
		EscalationNextFloor: func(actualPower int) (int, error) {
			floorCalls = append(floorCalls, actualPower)
			return actualPower + 20, nil
		},
		Now: func() time.Time { return frozen },
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	report := result.Results[0]
	assert.Equal(t, FailureModeProviderConnectivity, report.OutcomeReason)
	assert.True(t, report.Disrupted, "provider connectivity failure must be marked disrupted")
	assert.Equal(t, "provider_connectivity", report.DisruptionReason)
	assert.Equal(t, frozen.Add(ProviderUnavailableCooldown).Format(time.RFC3339), report.RetryAfter)

	require.Equal(t, []int{50}, floorCalls, "EscalationNextFloor must be consulted with actualPower")

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner, "bead must be unclaimed for the next attempt")
	require.NotNil(t, got.Extra)

	failed := readFailedRoutes(got.Extra)
	require.Len(t, failed, 1, "failed-route record must be persisted on the bead")
	assert.Equal(t, "bragi", failed[0].Provider)
	assert.Equal(t, "qwen3.5-27b", failed[0].Model)
	assert.Equal(t, 50, failed[0].ActualPower)
	assert.Equal(t, FailureModeProviderConnectivity, failed[0].Reason)

	hint, ok := got.Extra[TriagePowerHintKey]
	require.True(t, ok, "power hint must be set so next attempt routes off the failed tier")
	assert.Equal(t, float64(70), hint, "hint must equal actualPower+ladder-step from EscalationNextFloor")

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var routeFailureBody map[string]any
	for _, ev := range events {
		if ev.Kind == "route-failure" {
			require.NoError(t, json.Unmarshal([]byte(ev.Body), &routeFailureBody))
			break
		}
	}
	require.NotNil(t, routeFailureBody, "route-failure event must be appended")
	assert.Equal(t, "bragi", routeFailureBody["provider"])
	assert.Equal(t, "qwen3.5-27b", routeFailureBody["model"])
	assert.Equal(t, FailureModeProviderConnectivity, routeFailureBody["outcome_reason"])
}

// TestExecuteBeadWorker_ProviderTimeoutPreservesOperatorPin verifies that when
// an operator pinned the route (harness/model/provider in the passthrough
// envelope), the loop records the failure but does NOT bump the power hint.
// Pinned routes are honored exactly as the operator requested; silently
// re-routing them would violate operator intent.
func TestExecuteBeadWorker_ProviderTimeoutPreservesOperatorPin(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	frozen := time.Date(2026, 5, 14, 8, 8, 30, 0, time.UTC)
	var floorCalls int
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusExecutionFailed,
				Harness:     "fiz",
				Provider:    "bragi",
				Model:       "qwen3.5-27b",
				ActualPower: 50,
				Detail:      "agent: execute: dial tcp 100.127.38.115:1234: i/o timeout",
				Error:       "dial tcp 100.127.38.115:1234: i/o timeout",
				BaseRev:     "aaaa1111",
				ResultRev:   "aaaa1111",
			}, nil
		}),
		EscalationNextFloor: func(actualPower int) (int, error) {
			floorCalls++
			return actualPower + 20, nil
		},
		Now: func() time.Time { return frozen },
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", Harness: "fiz", Model: "qwen3.5-27b"}
	pinned := config.TestLoopOverrides(cfgOpts)
	pinned.Provider = "bragi"
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(pinned)
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	report := result.Results[0]
	assert.Equal(t, FailureModeProviderConnectivity, report.OutcomeReason)
	assert.Equal(t, 0, floorCalls, "EscalationNextFloor must NOT be consulted under operator pin")

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Extra)

	failed := readFailedRoutes(got.Extra)
	require.Len(t, failed, 1, "failed-route record must still be persisted for visibility")
	assert.Equal(t, "bragi", failed[0].Provider)

	_, hasHint := got.Extra[TriagePowerHintKey]
	assert.False(t, hasHint, "pinned routing must NOT be silently rerouted via power-hint bump")
}

// TestIsProviderConnectivityFailureReport_Discriminates pins down the
// classifier boundaries: it fires only on transport-level errors against an
// identified route, and defers to the existing no_viable_provider /
// routing-infrastructure paths when their patterns apply.
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
		{"no_provider_field", ExecuteBeadReport{
			Status: ExecuteBeadStatusExecutionFailed,
			Detail: "dial tcp 1.2.3.4:80: i/o timeout"}, false},
		{"no_viable_provider_path_owned_elsewhere", ExecuteBeadReport{
			Status: ExecuteBeadStatusExecutionFailed, Provider: "bragi",
			Detail: "ResolveRoute: no viable routing candidate: 3 candidates rejected"}, false},
		{"success_status", ExecuteBeadReport{
			Status: ExecuteBeadStatusSuccess, Provider: "bragi",
			Detail: "dial tcp i/o timeout"}, false},
		{"unrelated_error", ExecuteBeadReport{
			Status: ExecuteBeadStatusExecutionFailed, Provider: "bragi",
			Detail: "build failed"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isProviderConnectivityFailureReport(tc.report)
			if got != tc.want {
				t.Fatalf("isProviderConnectivityFailureReport(%+v) = %t, want %t", tc.report, got, tc.want)
			}
		})
	}
}
