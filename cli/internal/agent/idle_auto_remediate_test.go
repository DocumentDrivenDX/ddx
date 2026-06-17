package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func idleTestConfig(t *testing.T) config.ResolvedConfig {
	t.Helper()
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	return config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
}

// newGenuineEpic returns an epic bead that satisfies every AutoDecomposeEpic
// precondition so the idle-path decomposer will dispatch for it. It is marked
// execution-ineligible so it lands in the NotEligible (non-ready) bucket — the
// state in which a genuinely-undecomposed epic reaches the idle path — while
// bead.Diagnose still reports genuinely_needs_decomposition (0 children).
func newGenuineEpic(id, title string) *bead.Bead {
	return &bead.Bead{
		ID:          id,
		Title:       title,
		IssueType:   "epic",
		Description: "PROBLEM\nThis epic has no children and needs decomposition.\n\nROOT CAUSE\nThe epic was created but not broken down.\n\nPROPOSED FIX\nSplit into child tasks.\n\nNON-SCOPE\nNone.",
		Acceptance:  "1. Children are created with proper structure\n2. cd cli && go test ./internal/agent/... green\n",
		Labels:      []string{"spec:FEAT-010", "phase:2"},
		Extra:       map[string]any{bead.ExtraExecutionElig: false},
	}
}

// decomposeRunnerReturningChildren is a fake AgentRunner that returns a
// two-child decomposition, mirroring TestAutoDecomposeEpic_HappyPath.
func decomposeRunnerReturningChildren(called *int) reframeRunnerFunc {
	return reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		*called++
		children := []map[string]interface{}{
			{
				"title":       "child one",
				"description": "PROBLEM\nChild one.\n\nROOT CAUSE\nSplit from parent.\n",
				"acceptance":  "1. Implements feature\n2. cd cli && go test ./internal/agent/... green\n",
				"labels":      []string{"spec:FEAT-010"},
			},
			{
				"title":       "child two",
				"description": "PROBLEM\nChild two.\n\nROOT CAUSE\nSplit from parent.\n",
				"acceptance":  "1. Implements feature\n2. cd cli && go test ./internal/agent/... green\n",
				"labels":      []string{"spec:FEAT-010"},
			},
		}
		out, _ := json.Marshal(map[string]interface{}{
			"children": children,
			"ac_map":   []interface{}{},
		})
		return &Result{ExitCode: 0, Output: string(out), CostUSD: 0.0}, nil
	})
}

// TestWorkIdle_AutoRemediatesAndRescansWithoutSleep covers AC #3: a fixture
// with one supersession leftover plus one dead-intermediate (a closure
// candidate the epic cascade misses because it is not an epic) is fully
// resolved in a single idle iteration, emitting loop.auto_remediated with
// counts, then drains without sleeping.
func TestWorkIdle_AutoRemediatesAndRescansWithoutSleep(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	// Supersession leftover: X superseded by closed Y.
	superseder := &bead.Bead{ID: "ddx-superseder", Title: "superseder"}
	require.NoError(t, store.Create(ctx, superseder))
	require.NoError(t, store.Close(ctx, superseder.ID))
	leftover := &bead.Bead{
		ID:    "ddx-leftover",
		Title: "superseded leftover",
		Extra: map[string]any{"superseded-by": superseder.ID},
	}
	require.NoError(t, store.Create(ctx, leftover))

	// Dead-intermediate: D is execution-ineligible with all children closed.
	dead := &bead.Bead{
		ID:    "ddx-dead",
		Title: "closure candidate dead-intermediate",
		Extra: map[string]any{bead.ExtraExecutionElig: false},
	}
	require.NoError(t, store.Create(ctx, dead))
	deadChild := &bead.Bead{ID: "ddx-dead-child", Title: "dead child", Parent: dead.ID}
	require.NoError(t, store.Create(ctx, deadChild))
	// Close the child without invoking Store.Close so the dead-intermediate
	// walk-up does not pre-close D during fixture setup.
	require.NoError(t, store.SetLifecycleStatus(deadChild.ID, bead.StatusClosed, bead.LifecycleTransitionOptions{
		ManualClose: true, Reason: "test fixture",
	}))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run: no bead is execution-ready (%s)", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	sink := &bytes.Buffer{}
	rcfg := idleTestConfig(t)
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: sink,
		IdleAutoRemediation: IdleAutoRemediationConfig{
			AutoSupersedeClose:    true,
			AutoClosureReclassify: true,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.NoReadyWork, "loop must drain after remediation")

	// Both beads resolved.
	gotLeftover, err := store.Get(ctx, leftover.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, gotLeftover.Status, "superseded leftover must be cascade-closed")
	gotDead, err := store.Get(ctx, dead.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, gotDead.Status, "dead-intermediate must be closed")

	events := parseLoopEvents(t, sink.String())
	remediated := loopEventDataByType(events, "loop.auto_remediated")
	require.Len(t, remediated, 1, "exactly one loop.auto_remediated event in the single resolving iteration")
	assert.EqualValues(t, 2, remediated[0]["total_succeeded"])
	counts, ok := remediated[0]["remediation_counts"].(map[string]any)
	require.True(t, ok, "remediation_counts present")
	assert.EqualValues(t, 1, counts[string(bead.ReasonSupersededPendingClose)])
	assert.EqualValues(t, 1, counts[string(bead.ReasonDeadIntermediateAllChildrenClosed)])
}

// TestWorkIdle_AutoDecomposeUndecomposedEpic covers AC #4.
func TestWorkIdle_AutoDecomposeUndecomposedEpic(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	epic := newGenuineEpic("ddx-epic-decompose", "epic: undecomposed")
	require.NoError(t, store.Create(ctx, epic))

	decomposeCalls := 0
	runner := decomposeRunnerReturningChildren(&decomposeCalls)

	var executed []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-child",
				ResultRev: "feedbeef",
			}, nil
		}),
	}

	sink := &bytes.Buffer{}
	rcfg := idleTestConfig(t)
	idleAutoDecomposeCalls := 0
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		EventSink: sink,
		IdleAutoRemediation: IdleAutoRemediationConfig{
			AutoEpicDecompose:  true,
			MaxRecoveryCostUSD: 2.0,
			Decompose: func(ctx context.Context, beadID string) (DecomposeResult, error) {
				idleAutoDecomposeCalls++
				return AutoDecomposeEpic(ctx, store, runner, rcfg, t.TempDir(), beadID)
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, idleAutoDecomposeCalls, "exactly one AutoDecomposeEpic dispatch")
	assert.Equal(t, 1, decomposeCalls, "decomposer runner dispatched exactly once")

	all, err := store.ReadAll(ctx)
	require.NoError(t, err)
	var children []bead.Bead
	for _, b := range all {
		if b.Parent == epic.ID {
			children = append(children, b)
		}
	}
	assert.Len(t, children, 2, "child beads are filed")

	parent, err := store.Get(ctx, epic.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, parent.Status, "parent epic must be closed after auto-decompose")
	assert.Empty(t, parent.Owner, "closed decomposed parent must not keep owner metadata")

	require.NotEmpty(t, executed, "loop re-scans and picks at least one child")
	childIDs := map[string]bool{}
	for _, c := range children {
		childIDs[c.ID] = true
	}
	for _, id := range executed {
		assert.True(t, childIDs[id], "only child beads are executed, got %s", id)
	}

	events := parseLoopEvents(t, sink.String())
	remediated := loopEventDataByType(events, "loop.auto_remediated")
	require.Len(t, remediated, 1)
	counts, _ := remediated[0]["remediation_counts"].(map[string]any)
	assert.EqualValues(t, 1, counts[string(bead.ReasonGenuinelyNeedsDecomposition)])
}

// TestWorkIdle_AutoDecomposeRespectsCostBudget covers AC #5.
func TestWorkIdle_AutoDecomposeRespectsCostBudget(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	epic := newGenuineEpic("ddx-epic-budget", "epic: budget gated")
	require.NoError(t, store.Create(ctx, epic))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run (%s)", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	sink := &bytes.Buffer{}
	decomposeCalled := false
	result, err := worker.Run(ctx, idleTestConfig(t), ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: sink,
		IdleAutoRemediation: IdleAutoRemediationConfig{
			AutoEpicDecompose:  true,
			MaxRecoveryCostUSD: 0, // no recovery budget
			Decompose: func(ctx context.Context, beadID string) (DecomposeResult, error) {
				decomposeCalled = true
				return DecomposeResult{}, nil
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, decomposeCalled, "decompose must not dispatch with zero recovery budget")

	events := parseLoopEvents(t, sink.String())
	gated := loopEventDataByType(events, "loop.auto_remediation_gated")
	require.Len(t, gated, 1)
	list, _ := gated[0]["gated"].([]any)
	require.Len(t, list, 1)
	entry, _ := list[0].(map[string]any)
	assert.Equal(t, epic.ID, entry["bead_id"])
	assert.Equal(t, string(bead.ReasonGatedByBudgetOrCooldown), entry["reason"])
}

// TestWorkIdle_AtMostOneDecomposePerIteration covers AC #6.
func TestWorkIdle_AtMostOneDecomposePerIteration(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	for _, id := range []string{"ddx-epic-a", "ddx-epic-b", "ddx-epic-c"} {
		require.NoError(t, store.Create(ctx, newGenuineEpic(id, "epic: "+id)))
	}

	var decomposed []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run (%s)", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	sink := &bytes.Buffer{}
	result, err := worker.Run(ctx, idleTestConfig(t), ExecuteBeadLoopRuntime{
		EventSink: sink,
		IdleAutoRemediation: IdleAutoRemediationConfig{
			AutoEpicDecompose:  true,
			MaxRecoveryCostUSD: 5.0,
			Decompose: func(ctx context.Context, beadID string) (DecomposeResult, error) {
				decomposed = append(decomposed, beadID)
				// Mark the epic so it is no longer genuinely_needs_decomposition
				// without creating any execution-ready children (keeps the loop
				// idle so it decomposes the remaining epics one per iteration).
				require.NoError(t, store.Update(ctx, beadID, func(b *bead.Bead) {
					b.Labels = append(b.Labels, "no-auto-decompose")
				}))
				return DecomposeResult{ChildIDs: []string{"synthetic"}}, nil
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, decomposed, 3, "all three epics decomposed across iterations")

	events := parseLoopEvents(t, sink.String())
	remediated := loopEventDataByType(events, "loop.auto_remediated")
	require.Len(t, remediated, 3, "one decompose dispatch per loop iteration → three iterations")
	for _, ev := range remediated {
		counts, _ := ev["remediation_counts"].(map[string]any)
		assert.EqualValues(t, 1, counts[string(bead.ReasonGenuinelyNeedsDecomposition)],
			"each iteration dispatches exactly one decompose")
	}
}

// TestWorkIdle_OverrideFlagSuppressesDecompose covers AC #7.
func TestWorkIdle_OverrideFlagSuppressesDecompose(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	epic := newGenuineEpic("ddx-epic-flagoff", "epic: flag suppressed")
	require.NoError(t, store.Create(ctx, epic))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run (%s)", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	sink := &bytes.Buffer{}
	decomposeCalled := false
	result, err := worker.Run(ctx, idleTestConfig(t), ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: sink,
		IdleAutoRemediation: IdleAutoRemediationConfig{
			AutoSupersedeClose:    true,
			AutoClosureReclassify: true,
			AutoEpicDecompose:     false, // --no-auto-epic-decompose
			MaxRecoveryCostUSD:    2.0,
			Decompose: func(ctx context.Context, beadID string) (DecomposeResult, error) {
				decomposeCalled = true
				return DecomposeResult{}, nil
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, decomposeCalled, "decompose must not dispatch when the override flag is set")

	events := parseLoopEvents(t, sink.String())
	gated := loopEventDataByType(events, "loop.auto_remediation_gated")
	require.Len(t, gated, 1)
	list, _ := gated[0]["gated"].([]any)
	require.Len(t, list, 1)
	entry, _ := list[0].(map[string]any)
	assert.Equal(t, epic.ID, entry["bead_id"])
	assert.Equal(t, string(bead.ReasonGatedByBudgetOrCooldown), entry["reason"])
	assert.Equal(t, "no-auto-epic-decompose", entry["detail"])
}

// TestWorkIdle_NoSuccessFallsThroughToSleep covers AC #8: with only
// non-remediable diagnoses present, the loop falls through to the existing
// idle path, emits loop.idle, and emits neither auto-remediation event.
func TestWorkIdle_NoSuccessFallsThroughToSleep(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	// A proposed bead is non-ready and non-idle-remediable (Diagnose returns
	// no_diagnosis). No execution-ready work exists, so the loop must fall
	// through to the existing idle path.
	prop := &bead.Bead{ID: "ddx-proposed", Title: "needs operator"}
	require.NoError(t, store.Create(ctx, prop))
	require.NoError(t, store.ParkToProposed(prop.ID, bead.ParkIntakeRejection, nil))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run (%s)", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	ctxCancel, cancel := context.WithCancel(ctx)
	defer cancel()
	sink := &cancelOnMatchWriter{match: `"type":"loop.idle"`, cancel: cancel}
	_, err := worker.Run(ctxCancel, idleTestConfig(t), ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: time.Millisecond,
		EventSink:    sink,
		IdleAutoRemediation: IdleAutoRemediationConfig{
			AutoSupersedeClose:    true,
			AutoEpicDecompose:     true,
			AutoClosureReclassify: true,
			MaxRecoveryCostUSD:    2.0,
		},
	})
	if err != nil {
		require.ErrorIs(t, err, context.Canceled)
	}

	events := parseLoopEvents(t, sink.String())
	assert.NotEmpty(t, loopEventDataByType(events, "loop.idle"), "must fall through to the existing idle path")
	assert.Empty(t, loopEventDataByType(events, "loop.auto_remediated"), "no remediation succeeded")
	assert.Empty(t, loopEventDataByType(events, "loop.auto_remediation_gated"), "no bead was gated")
}
