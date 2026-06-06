package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reframeRunnerFunc implements AgentRunner via a function for testing.
type reframeRunnerFunc func(opts RunArgs) (*Result, error)

func (f reframeRunnerFunc) Run(opts RunArgs) (*Result, error) { return f(opts) }

// TestPostLadderExhaustion_TriggersReframe verifies that a SpecGap failure class
// routes to runReframer, and on a successful rewrite the bead returns to
// status=open with a reframe-applied event emitted.
func TestPostLadderExhaustion_TriggersReframe(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	const origDesc = "PROBLEM\nOriginal description with ADR-024 reference.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42 has a bug.\n"
	const origAcc = "1. TestPostLadderExhaustion_TriggersReframe passes\n2. cd cli && go test ./internal/agent/... green\n"

	b := &bead.Bead{
		ID:          "ddx-reframe-specgap",
		Title:       "reframe spec-gap test",
		Description: origDesc,
		Acceptance:  origAcc,
	}
	require.NoError(t, store.Create(context.

		// Pre-seed counter to 1 so the next budget exhaustion hits the threshold.
		Background(), b))

	require.NoError(t, incrementConsecutiveLadderExhaustions(store, b.ID))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const newDesc = "PROBLEM\nReframed description with ADR-024 reference.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42 needs a targeted fix.\n"

	var reframeDispatched bool
	reframeRunner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		reframeDispatched = true
		cancel() // stop the loop after the reframer fires
		out, _ := json.Marshal(map[string]interface{}{
			"description": newDesc,
			"acceptance":  nil,
		})
		return &Result{
			ExitCode: 0,
			Output:   string(out),
			CostUSD:  0.0042,
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	hook := NewReframePostLadderExhaustionHook(store, reframeRunner, rcfg, t.TempDir())

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed + "_" + ReviewTerminalClassSpecGap,
				Detail:    escalation.PerBeadBudgetExhaustedReason + ": $1.00 billed >= $0.50 per-bead budget",
				SessionID: "sess-reframe-specgap",
			}, nil
		}),
	}

	_, _ = worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:                     executeloop.ModeDrain,
		PostLadderExhaustionHook: hook,
		ProjectRoot:              t.TempDir(),
		SessionID:                "sess-reframe-specgap",
		WorkerID:                 "worker-reframe-specgap",
	})

	assert.True(t, reframeDispatched, "reframer must be dispatched for SpecGap class")

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead must be open after successful reframe")
	assert.Equal(t, strings.TrimSpace(newDesc), strings.TrimSpace(got.Description), "description must be updated")
	assert.EqualValues(t, 0, consecutiveLadderExhaustionsValue(got.Extra[consecutiveLadderExhaustionsKey]),
		"consecutive_ladder_exhaustions must be cleared after reframe")

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var reframeApplied bool
	for _, ev := range events {
		if ev.Kind == "reframe-applied" {
			reframeApplied = true
			assert.Contains(t, ev.Body, "cost_usd", "reframe-applied event body must contain cost_usd field")
		}
	}
	assert.True(t, reframeApplied, "reframe-applied event must be emitted")
}

// TestReframerTimeout_CountsAsFailure verifies that a stub agent blocking past
// the context deadline returns ReframeResult{Failed:true, Reason:"timeout"}.
func TestReframerTimeout_CountsAsFailure(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:          "ddx-reframe-timeout",
		Title:       "reframe timeout test",
		Description: "PROBLEM\nTimeout test.",
		Acceptance:  "1. TestReframerTimeout_CountsAsFailure\n",
	}
	require.NoError(t, store.Create(context.Background(), b))

	blockingRunner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		<-opts.Context.Done()
		return nil, opts.Context.Err()
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Short-lived ctx so the reframer's internal timeout fires quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := runReframer(ctx, store, blockingRunner, rcfg, t.TempDir(), b.ID)
	assert.True(t, result.Failed, "must be marked as failed on timeout")
	assert.Equal(t, "timeout", result.Reason)
}

// TestReframerNoOpEdits_CountsAsFailure verifies that a stub agent returning
// unchanged description and acceptance yields ReframeResult{Failed:true, Reason:"noop_edits"}.
func TestReframerNoOpEdits_CountsAsFailure(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	const desc = "PROBLEM\nOriginal description."
	const acc = "1. TestReframerNoOpEdits_CountsAsFailure\n"

	b := &bead.Bead{
		ID:          "ddx-reframe-noop",
		Title:       "reframe no-op test",
		Description: desc,
		Acceptance:  acc,
	}
	require.NoError(t, store.Create(context.

		// Runner returns the same text as the current bead — no actual change.
		Background(), b))

	noopRunner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		out, _ := json.Marshal(map[string]interface{}{
			"description": desc,
			"acceptance":  acc,
		})
		return &Result{
			ExitCode: 0,
			Output:   string(out),
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result := runReframer(context.Background(), store, noopRunner, rcfg, t.TempDir(), b.ID)
	assert.True(t, result.Failed, "must be marked as failed for no-op edits")
	assert.Equal(t, "noop_edits", result.Reason)
}
