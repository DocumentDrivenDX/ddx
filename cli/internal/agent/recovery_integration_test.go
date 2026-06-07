package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRecoveryTestBead(t *testing.T, store *bead.Store, id string) {
	t.Helper()
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:          id,
		Title:       "auto recovery test",
		Description: "PROBLEM\nNeeds recovery.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42.\n",
		Acceptance:  "1. TestRecovery passes\n2. cd cli && go test ./internal/agent/... green\n",
	}))
}

func TestReframeFailure_FallsBackToDecompose(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	newRecoveryTestBead(t, store, "ddx-recovery-fallback")

	var promptSources []string
	runner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		promptSources = append(promptSources, opts.PromptSource)
		if opts.PromptSource == reframerPromptSource {
			return &Result{ExitCode: 0, Output: `not json`, CostUSD: 0.10}, nil
		}
		children, _ := json.Marshal([]map[string]any{{
			"title":       "child one",
			"description": "PROBLEM\nChild.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42.\n",
			"acceptance":  "1. TestChild passes\n",
		}})
		return &Result{ExitCode: 0, Output: string(children), CostUSD: 0.20}, nil
	})

	hook := NewAutoRecoveryPostLadderExhaustionHook(store, runner, config.NewTestConfigForLoop(config.TestLoopConfigOpts{}).Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{})), t.TempDir(), AutoRecoveryConfig{
		MaxRecoveryCostUSD: 2.0,
		MaxBeadCostUSD:     5.0,
	})
	result, err := hook(context.Background(), "ddx-recovery-fallback", PersistentExecutionFailed)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Succeeded)
	assert.Equal(t, Decompose, result.Path)
	assert.Equal(t, []string{reframerPromptSource, decomposerPromptSource}, promptSources)
}

func TestBothFail_ParkProposed(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	newRecoveryTestBead(t, store, "ddx-recovery-both-fail")

	runner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		return &Result{ExitCode: 0, Output: `not json`, CostUSD: 0.10}, nil
	})
	hook := NewAutoRecoveryPostLadderExhaustionHook(store, runner, config.NewTestConfigForLoop(config.TestLoopConfigOpts{}).Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{})), t.TempDir(), AutoRecoveryConfig{
		MaxRecoveryCostUSD: 2.0,
		MaxBeadCostUSD:     5.0,
	})
	result, err := hook(context.Background(), "ddx-recovery-both-fail", PersistentExecutionFailed)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Succeeded)
	assert.Equal(t, "both_failed", result.OutcomeReason)

	got, err := store.Get(context.Background(), "ddx-recovery-both-fail")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)

	body := autoRecoveryFailedBody(t, store, got.ID)
	assert.Equal(t, "both_failed", body.Reason)
	assert.InDelta(t, 0.20, body.TotalCostUSD, 0.001)
}

func TestRecoveryCircuitBreaker_AbortsAtCostCap(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	newRecoveryTestBead(t, store, "ddx-recovery-cost-cap")

	costs := []float64{1.50, 0.60}
	var calls int
	runner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		cost := costs[calls]
		calls++
		return &Result{ExitCode: 0, Output: `not json`, CostUSD: cost}, nil
	})
	hook := NewAutoRecoveryPostLadderExhaustionHook(store, runner, config.NewTestConfigForLoop(config.TestLoopConfigOpts{}).Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{})), t.TempDir(), AutoRecoveryConfig{
		MaxRecoveryCostUSD: 2.0,
		MaxBeadCostUSD:     5.0,
	})
	result, err := hook(context.Background(), "ddx-recovery-cost-cap", PersistentExecutionFailed)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "circuit-breaker", result.OutcomeReason)
	assert.InDelta(t, 2.10, result.CostUSD, 0.001)

	got, err := store.Get(context.Background(), "ddx-recovery-cost-cap")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)
	body := autoRecoveryFailedBody(t, store, got.ID)
	assert.Equal(t, "circuit-breaker", body.Reason)
}

func TestPerBeadBudgetExhausted_TriggersAutoRecovery(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	newRecoveryTestBead(t, store, "ddx-recovery-per-bead-budget")

	runner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		return &Result{ExitCode: 0, Output: `not json`, CostUSD: 0.60}, nil
	})
	hook := NewAutoRecoveryPostLadderExhaustionHook(store, runner, config.NewTestConfigForLoop(config.TestLoopConfigOpts{}).Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{})), t.TempDir(), AutoRecoveryConfig{
		MaxRecoveryCostUSD: 2.0,
		MaxBeadCostUSD:     0.50,
	})
	result, err := hook(context.Background(), "ddx-recovery-per-bead-budget", PersistentExecutionFailed)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, escalation.PerBeadBudgetExhaustedReason, result.OutcomeReason)

	got, err := store.Get(context.Background(), "ddx-recovery-per-bead-budget")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	events, err := store.Events(got.ID)
	require.NoError(t, err)
	for _, ev := range events {
		assert.NotEqual(t, "auto-recovery-failed", ev.Kind)
	}
}

func autoRecoveryFailedBody(t *testing.T, store *bead.Store, beadID string) autoRecoveryFailedEventBody {
	t.Helper()
	events, err := store.Events(beadID)
	require.NoError(t, err)
	for _, ev := range events {
		if ev.Kind == "auto-recovery-failed" {
			var body autoRecoveryFailedEventBody
			require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
			return body
		}
	}
	t.Fatalf("auto-recovery-failed event not found")
	return autoRecoveryFailedEventBody{}
}
