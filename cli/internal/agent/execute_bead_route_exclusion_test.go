package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
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

// TestRouteRequest_PopulatedFromFailedRoutes asserts that failed-route entries
// whose At timestamp is within RouteExclusionWindow appear in the Fizeau
// RouteRequest.ExcludedRoutes payload built by buildExcludedRoutes.
func TestRouteRequest_PopulatedFromFailedRoutes(t *testing.T) {
	frozen := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	failed := []FailedRouteEntry{
		{Provider: "bragi", Model: "qwen3.5-27b", ActualPower: 50, Reason: FailureModeProviderConnectivity, At: frozen.Add(-10 * time.Minute).Format(time.RFC3339)},
		{Provider: "local", Model: "llama3", ActualPower: 30, Reason: FailureModeProviderConnectivity, At: frozen.Add(-5 * time.Minute).Format(time.RFC3339)},
	}

	excluded := buildExcludedRoutes(failed, frozen, RouteExclusionWindow)

	require.Len(t, excluded, 2)
	assert.Equal(t, "bragi", excluded[0].Provider)
	assert.Equal(t, "qwen3.5-27b", excluded[0].Model)
	assert.Equal(t, "local", excluded[1].Provider)
	assert.Equal(t, "llama3", excluded[1].Model)
}

// TestRouteRequest_ExpiredFailedRoutesDropped asserts that entries older than
// RouteExclusionWindow are omitted from the RouteRequest payload but remain
// in the bead Extra audit list (the input slice is not modified).
func TestRouteRequest_ExpiredFailedRoutesDropped(t *testing.T) {
	frozen := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	failed := []FailedRouteEntry{
		{Provider: "bragi", Model: "qwen3.5-27b", At: frozen.Add(-2 * time.Hour).Format(time.RFC3339)}, // expired
		{Provider: "local", Model: "llama3", At: frozen.Add(-30 * time.Minute).Format(time.RFC3339)},   // active
	}

	excluded := buildExcludedRoutes(failed, frozen, RouteExclusionWindow)

	// expired entry must be omitted; active entry must be kept
	require.Len(t, excluded, 1, "only active (non-expired) entries must appear in ExcludedRoutes")
	assert.Equal(t, "local", excluded[0].Provider)
	assert.Equal(t, "llama3", excluded[0].Model)

	// audit list must be untouched
	assert.Len(t, failed, 2, "buildExcludedRoutes must not modify the input slice")
}

// TestRouteRequest_AllRoutesExcludedTriggersEscalation asserts that when
// every candidate at the requested power class is excluded (resolveRoute
// returns a no-viable-candidate error), CheckAndApplyRouteExclusions escalates
// TriagePowerHintKey via the ddx-8a7a6843 ladder-exhaustion path.
func TestRouteRequest_AllRoutesExcludedTriggersEscalation(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	frozen := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)

	_ = store.Update(first.ID, func(b *bead.Bead) {
		appendFailedRoute(b, FailedRouteEntry{
			Provider: "bragi", Model: "qwen3.5-27b", ActualPower: 50,
			Reason: FailureModeProviderConnectivity,
			At:     frozen.Add(-10 * time.Minute).Format(time.RFC3339),
		})
	})
	b, err := store.Get(first.ID)
	require.NoError(t, err)

	noViableRoute := func(_ context.Context, req agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
		require.Len(t, req.ExcludedRoutes, 1, "resolveRoute must receive the excluded routes payload")
		assert.Equal(t, "bragi", req.ExcludedRoutes[0].Provider)
		assert.Equal(t, "qwen3.5-27b", req.ExcludedRoutes[0].Model)
		return nil, fmt.Errorf("ResolveRoute: no viable routing candidate: 1 candidates rejected")
	}

	report, skip := CheckAndApplyRouteExclusions(
		context.Background(), store, first.ID, "actor", b.Extra, frozen, 50,
		noViableRoute,
		func(p int) (int, error) { return p + 20, nil },
	)

	require.True(t, skip, "dispatch must be skipped when all routes at the requested power class are excluded")
	assert.Equal(t, FailureModeNoViableProvider, report.OutcomeReason)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, report.Status)
	assert.Equal(t, first.ID, report.BeadID)

	updated, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, float64(70), updated.Extra[TriagePowerHintKey],
		"TriagePowerHintKey must escalate to actualPower+ladder-step (50+20=70)")
}

// TestFailedRoutes_DeduplicatesOnSameProviderModel asserts that two consecutive
// applyProviderConnectivityRouteExclusion calls with the same (provider, model)
// result in ONE entry whose At timestamp is the second call's and whose count is 2.
func TestFailedRoutes_DeduplicatesOnSameProviderModel(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	t1 := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Minute)
	report := ExecuteBeadReport{
		BeadID:      first.ID,
		Status:      ExecuteBeadStatusExecutionFailed,
		Provider:    "bragi",
		Model:       "qwen3-27b",
		ActualPower: 50,
	}
	noopFloor := func(p int) (int, error) { return p + 10, nil }

	require.NoError(t, applyProviderConnectivityRouteExclusion(store, first.ID, "actor", report, false, noopFloor, t1))
	// second call: same (provider, model) — triggers repeatFailure + ParkToProposed
	_ = applyProviderConnectivityRouteExclusion(store, first.ID, "actor", report, false, noopFloor, t2)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Extra)

	entries := readFailedRoutes(got.Extra)
	require.Len(t, entries, 1, "duplicate (provider, model) must produce exactly one entry")
	assert.Equal(t, "bragi", entries[0].Provider)
	assert.Equal(t, "qwen3-27b", entries[0].Model)
	assert.Equal(t, 2, entries[0].Count, "count must be 2 after two calls")
	assert.Equal(t, t2.UTC().Format(time.RFC3339), entries[0].At, "At must reflect the second call's timestamp")
}

// TestFailedRoutes_CapsAt32Entries asserts that after 33 distinct (provider, model)
// failures, the list contains exactly 32 entries and the oldest has been evicted (FIFO).
func TestFailedRoutes_CapsAt32Entries(t *testing.T) {
	b := &bead.Bead{ID: "test", Extra: make(map[string]any)}
	base := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 33; i++ {
		appendFailedRoute(b, FailedRouteEntry{
			Provider: "provider",
			Model:    fmt.Sprintf("model-%02d", i),
			At:       base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
	}
	entries := readFailedRoutes(b.Extra)
	require.Len(t, entries, 32, "ring must cap at 32 entries")
	for _, e := range entries {
		assert.NotEqual(t, "model-00", e.Model, "oldest entry (model-00) must be evicted")
	}
	assert.Equal(t, "model-32", entries[len(entries)-1].Model, "newest entry must be present")
}

// TestFailedRoutes_ExclusionWindowFiltersOldEntriesFromRouteRequest asserts that
// entries older than RouteExclusionWindow are present in the bead's Extra audit list
// but absent from the Fizeau RouteRequest.ExcludedRoutes payload.
func TestFailedRoutes_ExclusionWindowFiltersOldEntriesFromRouteRequest(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	frozen := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.Update(first.ID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		b.Extra[executeLoopFailedRoutesKey] = []FailedRouteEntry{
			{Provider: "bragi", Model: "qwen3-27b", Count: 1,
				At: frozen.Add(-2 * time.Hour).Format(time.RFC3339)}, // outside window
			{Provider: "local", Model: "llama3", Count: 1,
				At: frozen.Add(-30 * time.Minute).Format(time.RFC3339)}, // inside window
		}
	}))

	got, err := store.Get(first.ID)
	require.NoError(t, err)

	audit := readFailedRoutes(got.Extra)
	require.Len(t, audit, 2, "both entries must be present in the audit list")

	excluded := buildExcludedRoutes(audit, frozen, RouteExclusionWindow)
	require.Len(t, excluded, 1, "only the recent entry must appear in ExcludedRoutes")
	assert.Equal(t, "local", excluded[0].Provider)
	assert.Equal(t, "llama3", excluded[0].Model)
}

// TestFailedRoutes_StoreGetCollapsesLegacyDuplicates asserts that a bead loaded from
// .ddx/beads.jsonl with pre-migration duplicate entries is normalized on read.
func TestFailedRoutes_StoreGetCollapsesLegacyDuplicates(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	t1 := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Minute)

	// Write legacy duplicate entries directly, bypassing appendFailedRoute,
	// simulating a bead that accumulated duplicates before this fix.
	require.NoError(t, store.Update(first.ID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		b.Extra[executeLoopFailedRoutesKey] = []map[string]any{
			{"provider": "bragi", "model": "qwen3-27b", "at": t1.Format(time.RFC3339)},
			{"provider": "bragi", "model": "qwen3-27b", "at": t2.Format(time.RFC3339)},
		}
	}))

	got, err := store.Get(first.ID)
	require.NoError(t, err)

	entries := readFailedRoutes(got.Extra)
	require.Len(t, entries, 1, "legacy duplicates must be collapsed on read")
	assert.Equal(t, "bragi", entries[0].Provider)
	assert.Equal(t, "qwen3-27b", entries[0].Model)
	assert.Equal(t, t2.Format(time.RFC3339), entries[0].At, "newer timestamp must be kept")
	assert.Equal(t, 2, entries[0].Count, "count must reflect number of collapsed duplicates")
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
