package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntake_TooLargeDecomposed_CreatesChildrenAndBlocksParent(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-decomp-parent",
		Title:      "Parent bead to decompose",
		Acceptance: "1. do the thing\n2. run tests",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	var execCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			t.Error("executor must not run after too_large_decomposed intake")
			return ExecuteBeadReport{}, nil
		}),
	}

	decomp := &PreClaimDecomposition{
		Rationale: "bead too large, splitting into two subtasks",
		Children: []PreClaimDecompositionChild{
			{Title: "Child A", Description: "Part A desc", Acceptance: "1. do part A"},
			{Title: "Child B", Description: "Part B desc", Acceptance: "1. do part B"},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. do the thing", Coverage: "covered by Child A and Child B"},
			{ParentAC: "2. run tests", Coverage: "covered by Child A and Child B"},
		},
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: candidate.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome:       PreClaimIntakeTooLargeDecomposed,
				Detail:        "bead is too large for a single implementation attempt",
				Decomposition: decomp,
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 0, result.Attempts, "too_large_decomposed intake must not count as an attempt")
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls), "executor must not run")

	// Parent must remain open (not proposed) after a successful decomposition.
	got, err := store.Get(context.Background(), "ddx-decomp-parent")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "parent must remain open after successful decomposition")
	assert.Equal(t, false, got.Extra[bead.ExtraExecutionElig], "parent must be marked execution-ineligible after successful decomposition")

	// Two children must exist with Parent == candidate.ID.
	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	var children []bead.Bead
	for _, b := range all {
		if b.Parent == "ddx-decomp-parent" {
			children = append(children, b)
		}
	}
	assert.Len(t, children, 2, "two children must be created")

	// Parent must not gain dependency edges to its children.
	got, err = store.Get(context.Background(), "ddx-decomp-parent")
	require.NoError(t, err)
	assert.Empty(t, got.DepIDs(), "parent must not depend on its decomposed children")

	// triage-decomposed event must be appended to parent.
	events, err := store.Events("ddx-decomp-parent")
	require.NoError(t, err)
	var foundDecomp bool
	for _, ev := range events {
		if ev.Kind == "triage-decomposed" {
			foundDecomp = true
		}
	}
	assert.True(t, foundDecomp, "triage-decomposed event must be appended to parent")
}

func TestDecomposeDoesNotSelfDep(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-decomp-no-self-dep",
		Title:      "Parent bead for self-dep regression",
		Acceptance: "1. decompose into children\n2. keep parent out of deps",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Error("executor must not run after too_large_decomposed intake")
			return ExecuteBeadReport{}, nil
		}),
	}

	decomp := &PreClaimDecomposition{
		Rationale: "split without wiring parent as a dependency",
		Children: []PreClaimDecompositionChild{
			{Title: "Child 1", Description: "first child", Acceptance: "1. first child"},
			{Title: "Child 2", Description: "second child", Acceptance: "1. second child"},
			{Title: "Child 3", Description: "third child", Acceptance: "1. third child"},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. decompose into children", Coverage: "covered by Child 1, Child 2, and Child 3"},
			{ParentAC: "2. keep parent out of deps", Coverage: "covered by all children remaining independent of the parent"},
		},
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: candidate.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome:       PreClaimIntakeTooLargeDecomposed,
				Detail:        "bead is too large for a single implementation attempt",
				Decomposition: decomp,
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Equal(t, false, got.Extra[bead.ExtraExecutionElig], "parent must be parked as execution-ineligible")
	assert.Empty(t, got.DepIDs(), "parent must not depend on its children")

	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	var children []bead.Bead
	for _, b := range all {
		if b.Parent == candidate.ID {
			children = append(children, b)
		}
	}
	require.Len(t, children, 3, "three children must be created")
	for _, child := range children {
		assert.NotContains(t, child.DepIDs(), candidate.ID, "child %s must not depend on the parent", child.ID)
	}
}

func TestIntake_TooLargeWithoutConcreteSplit_InvokesDecompositionHook(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:          "ddx-decomp-hook-parent",
		Title:       "Parent bead requiring hook split",
		Acceptance:  "1. TestHookSplit covers split\n2. cd cli && go test ./internal/agent/... green",
		Description: "PROBLEM\nToo broad.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42 does too much.\n",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run after pre-claim decomposition hook returns children")
			return ExecuteBeadReport{}, nil
		}),
	}
	decomp := &PreClaimDecomposition{
		Rationale: "split broad foundation bead",
		Children: []PreClaimDecompositionChild{
			{Title: "Child from hook", Description: "PROBLEM\nChild scope.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42.\n", Acceptance: "1. TestHookChild\n2. cd cli && go test ./internal/agent/..."},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. TestHookSplit covers split", Coverage: "covered by Child from hook AC 1"},
			{ParentAC: "2. cd cli && go test ./internal/agent/... green", Coverage: "covered by Child from hook AC 2"},
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", MaxDecompositionDepth: 3}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	var hookCalls int32
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: candidate.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeTooLargeDecomposed,
				Detail:  "too large; split required",
			}, nil
		},
		PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
			atomic.AddInt32(&hookCalls, 1)
			return decomp, nil
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(1), atomic.LoadInt32(&hookCalls))
	assert.Equal(t, 0, result.Attempts)

	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	var children []bead.Bead
	for _, b := range all {
		if b.Parent == candidate.ID {
			children = append(children, b)
		}
	}
	require.Len(t, children, 1)
	assert.Equal(t, "Child from hook", children[0].Title)

	parent, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, parent.Status)
	assert.Equal(t, false, parent.Extra[bead.ExtraExecutionElig], "parent must be marked execution-ineligible after hook decomposition")
	assert.Empty(t, parent.DepIDs(), "parent must not depend on its decomposed child")
}

func TestIntake_DecompositionEventIncludesACMap(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-acmap-event",
		Title:      "ACMap event body test",
		Acceptance: "1. do stuff\n2. run tests",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Error("executor must not run")
			return ExecuteBeadReport{}, nil
		}),
	}

	decomp := &PreClaimDecomposition{
		Rationale: "split for parallel execution",
		Children: []PreClaimDecompositionChild{
			{Title: "Part 1", Description: "first part", Acceptance: "1. do stuff"},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. do stuff", Coverage: "fully handled by Part 1"},
			{ParentAC: "2. run tests", Coverage: "non_scope"},
		},
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: candidate.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome:       PreClaimIntakeTooLargeDecomposed,
				Decomposition: decomp,
			}, nil
		},
	})
	require.NoError(t, err)

	events, err := store.Events("ddx-acmap-event")
	require.NoError(t, err)

	var decompEv bead.BeadEvent
	for _, ev := range events {
		if ev.Kind == "triage-decomposed" {
			decompEv = ev
			break
		}
	}
	require.Equal(t, "triage-decomposed", decompEv.Kind, "triage-decomposed event must exist")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(decompEv.Body), &body))
	assert.Contains(t, body, "child_ids", "event body must include child_ids")
	assert.Contains(t, body, "rationale", "event body must include rationale")
	assert.Contains(t, body, "ac_map", "event body must include ac_map")
	assert.Equal(t, "split for parallel execution", body["rationale"])
	childIDs, ok := body["child_ids"].([]any)
	assert.True(t, ok, "child_ids must be a list")
	assert.Len(t, childIDs, 1, "child_ids must have one entry")
}

func TestIntake_DecompositionACMapRejectsDroppedAC(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-lossy-intake",
		Title:      "Lossy decomposition test",
		Acceptance: "1. do the thing\n2. run tests",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Error("executor must not run for lossy decomposition")
			return ExecuteBeadReport{}, nil
		}),
	}

	// ACMap with empty Coverage entry — lossy split.
	lossyDecomp := &PreClaimDecomposition{
		Rationale: "splitting",
		Children: []PreClaimDecompositionChild{
			{Title: "Child", Description: "desc", Acceptance: "1. do the thing"},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. do the thing", Coverage: "covered by Child"},
			{ParentAC: "2. run tests", Coverage: ""}, // empty → lossy
		},
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: candidate.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome:       PreClaimIntakeTooLargeDecomposed,
				Decomposition: lossyDecomp,
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 0, result.Attempts, "lossy split must not count as an attempt")

	// No children must be created.
	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	for _, b := range all {
		assert.NotEqual(t, "ddx-lossy-intake", b.Parent, "no children must be created for a lossy split")
	}

	// Parent must be parked for operator review.
	got, err := store.Get(context.Background(), "ddx-lossy-intake")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status, "lossy split must park parent for operator review")
}

func TestIntake_DepthCapOverflow_BlocksOperator(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	// Create a hierarchy with two consecutive decomposed child layers. Ordinary
	// epic ancestry does not count toward the cap, but decomposed descendants do.
	root := &bead.Bead{ID: "ddx-dc-root", Title: "Root bead", Status: bead.StatusClosed}
	require.NoError(t, store.Create(context.Background(), root))

	child := &bead.Bead{ID: "ddx-dc-child", Title: "Child bead", Parent: "ddx-dc-root", Status: bead.StatusClosed, Labels: []string{"decomposed"}}
	require.NoError(t, store.Create(context.Background(), child))

	grandchild := &bead.Bead{
		ID:     "ddx-dc-grand",
		Title:  "Grandchild bead at depth 2",
		Parent: "ddx-dc-child",
		Labels: []string{"decomposed"},
	}
	require.NoError(t, store.Create(context.Background(), grandchild))

	var intakeCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Error("executor must not run when depth cap triggers")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 2,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: grandchild.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			atomic.AddInt32(&intakeCalls, 1)
			t.Errorf("intake hook must not be called when depth cap triggers (called for %s)", beadID)
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(0), atomic.LoadInt32(&intakeCalls), "intake hook must not run when depth cap triggers")
	assert.Equal(t, 0, result.Attempts, "depth cap must not count as an attempt")

	// Grandchild must be parked as proposed.
	got, err := store.Get(context.Background(), "ddx-dc-grand")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status, "depth-capped bead must be parked as proposed")

	// triage-overflow event must be appended to the grandchild.
	events, err := store.Events("ddx-dc-grand")
	require.NoError(t, err)
	var foundOverflow bool
	for _, ev := range events {
		if ev.Kind == "triage-overflow" {
			foundOverflow = true
		}
	}
	assert.True(t, foundOverflow, "triage-overflow event must be appended when depth cap fires")

	// needs-human-decomposition label must be present.
	assert.Contains(t, got.Labels, "needs-human-decomposition",
		"needs-human-decomposition label must be added when depth cap fires")
}

// TestPreClaimDecompositionAlreadyDecomposedAtomicChildExecutes proves that an
// already-decomposed child sitting at the configured depth cap with numbered
// acceptance criteria and a concrete implementation outcome is dispatched to
// the executor once — without invoking the decomposition hook or creating
// descendants.
func TestPreClaimDecompositionAlreadyDecomposedAtomicChildExecutes(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	root := &bead.Bead{ID: "ddx-atcap-root", Title: "Root", Status: bead.StatusClosed}
	require.NoError(t, store.Create(context.Background(), root))

	parent := &bead.Bead{
		ID:     "ddx-atcap-parent",
		Title:  "Parent (depth 1)",
		Parent: "ddx-atcap-root",
		Status: bead.StatusClosed,
		Labels: []string{"decomposed"},
	}
	require.NoError(t, store.Create(context.Background(), parent))

	// Candidate at depth 2 with max_decomposition_depth=2: at the cap, but
	// actionable (numbered AC + PROPOSED FIX implementation outcome).
	candidate := &bead.Bead{
		ID:     "ddx-atcap-atomic",
		Title:  "Atomic child at decomposition cap",
		Parent: "ddx-atcap-parent",
		Labels: []string{"decomposed"},
		Description: "PROBLEM\nAt-cap bead was parked instead of executed.\n\n" +
			"ROOT CAUSE\ncli/internal/agent/execute_bead_loop.go:2860 parks all at-cap beads.\n\n" +
			"PROPOSED FIX\nDispatch when numbered AC and PROPOSED FIX are present.\n\n" +
			"NON-SCOPE\nDepth policy unification.",
		Acceptance: "1. TestPreClaimDecompositionAlreadyDecomposedAtomicChildExecutes\n" +
			"2. cd cli && go test ./internal/agent/... -run TestPreClaimDecompositionAlreadyDecomposedAtomicChildExecutes -count=1",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	var execCalls int32
	var decompHookCalls int32
	var intakeCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			assert.Equal(t, candidate.ID, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-atcap-atomic",
				ResultRev: "abc123",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 2,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: candidate.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			atomic.AddInt32(&intakeCalls, 1)
			t.Errorf("preclaim intake must not run for actionable at-cap bead (called for %s)", beadID)
			return PreClaimIntakeResult{Outcome: PreClaimIntakeTooLargeDecomposed}, nil
		},
		PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
			atomic.AddInt32(&decompHookCalls, 1)
			t.Errorf("decomposition hook must not run for actionable at-cap bead (called for %s)", beadID)
			return &PreClaimDecomposition{
				Children: []PreClaimDecompositionChild{
					{Title: "should-not-exist", Description: "x", Acceptance: "1. no"},
				},
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&execCalls),
		"executor must be invoked exactly once for the actionable at-cap bead")
	assert.Equal(t, int32(0), atomic.LoadInt32(&decompHookCalls),
		"decomposition hook must not be called at the cap")
	assert.Equal(t, int32(0), atomic.LoadInt32(&intakeCalls),
		"preclaim intake must be skipped when at-cap and actionable")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)

	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	for _, b := range all {
		assert.NotEqual(t, candidate.ID, b.Parent,
			"no descendants must be created for an actionable bead at the cap")
	}

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.NotContains(t, got.Labels, "needs-human-decomposition",
		"actionable at-cap bead must not be labeled needs-human-decomposition")
}

// TestPreClaimDecompositionAtCapNonActionableParksOnce asserts that a bead at
// the configured depth cap with empty or non-actionable acceptance is parked
// exactly once with the needs-human-decomposition label and an explicit reason,
// and is never dispatched.
func TestPreClaimDecompositionAtCapNonActionableParksOnce(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	root := &bead.Bead{ID: "ddx-park-root", Title: "Root", Status: bead.StatusClosed}
	require.NoError(t, store.Create(context.Background(), root))

	parent := &bead.Bead{
		ID:     "ddx-park-parent",
		Title:  "Parent",
		Parent: "ddx-park-root",
		Status: bead.StatusClosed,
		Labels: []string{"decomposed"},
	}
	require.NoError(t, store.Create(context.Background(), parent))

	// At depth 2 with max=2, empty acceptance → non-actionable at cap.
	candidate := &bead.Bead{
		ID:         "ddx-park-nonactionable",
		Title:      "Non-actionable bead at decomposition cap",
		Parent:     "ddx-park-parent",
		Labels:     []string{"decomposed"},
		Acceptance: "", // empty / non-actionable
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	var execCalls int32
	var intakeCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			t.Error("executor must not run for non-actionable at-cap bead")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 2,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: candidate.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			atomic.AddInt32(&intakeCalls, 1)
			t.Errorf("intake hook must not run when non-actionable depth cap parks (called for %s)", beadID)
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls), "executor must not be dispatched")
	assert.Equal(t, int32(0), atomic.LoadInt32(&intakeCalls), "intake must not run when depth-cap park fires")
	assert.Equal(t, 0, result.Attempts)

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status, "non-actionable at-cap bead must be parked once as proposed")
	assert.Contains(t, got.Labels, "needs-human-decomposition",
		"needs-human-decomposition label must be present when non-actionable depth cap fires")

	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	var overflowCount int
	var foundExplicitReason bool
	for _, ev := range events {
		if ev.Kind == "triage-overflow" {
			overflowCount++
			if strings.Contains(ev.Body, "max") || strings.Contains(ev.Summary, "depth cap") {
				foundExplicitReason = true
			}
		}
		// Intake park events also carry the explicit reason.
		if strings.Contains(ev.Kind, "intake") || ev.Kind == "triage-overflow" {
			if strings.Contains(ev.Body, "max_decomposition_depth") ||
				strings.Contains(ev.Body, `"max"`) ||
				strings.Contains(ev.Summary, "depth") {
				foundExplicitReason = true
			}
		}
	}
	assert.Equal(t, 1, overflowCount, "triage-overflow must be recorded exactly once")
	assert.True(t, foundExplicitReason, "park must carry an explicit depth-cap reason")

	// Re-run once more: parked proposed bead must not be re-dispatched or re-parked into a loop.
	result2, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: candidate.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			atomic.AddInt32(&intakeCalls, 1)
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result2)
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls), "second pass must not dispatch either")

	events2, err := store.Events(candidate.ID)
	require.NoError(t, err)
	overflowCount2 := 0
	for _, ev := range events2 {
		if ev.Kind == "triage-overflow" {
			overflowCount2++
		}
	}
	// Parks once: either no new overflow, or at most the original one remains dominant.
	assert.LessOrEqual(t, overflowCount2, 1,
		"non-actionable at-cap bead must be parked once, not re-overflowed on every drain")
}

func TestPostAttemptTooLargeNoChanges_AutoDecomposes(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-postdecomp-01",
		Title:      "Post-attempt decomp test",
		Acceptance: "1. do stuff\n2. run tests",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	decomp := &PreClaimDecomposition{
		Rationale: "post-attempt split",
		Children: []PreClaimDecompositionChild{
			{Title: "Part A", Description: "part a", Acceptance: "1. do part A"},
			{Title: "Part B", Description: "part b", Acceptance: "1. do part B"},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. do stuff", Coverage: "covered by Part A"},
			{ParentAC: "2. run tests", Coverage: "covered by Part B"},
		},
	}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				NoChangesRationale: "status: open\norchestrator_action: decompose\nreason: bead is too large for implementation-level splitting",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-postdecomp-01",
		PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
			return decomp, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.Attempts, "no_changes attempt must count as an attempt")
	assert.Equal(t, 1, result.Failures, "decomposed no_changes must count as a failure")
	assert.Equal(t, 0, result.Successes)

	// Two children must be created with Parent == candidate.ID.
	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	var children []bead.Bead
	for _, b := range all {
		if b.Parent == "ddx-postdecomp-01" {
			children = append(children, b)
		}
	}
	assert.Len(t, children, 2, "two children must be created after post-attempt decomposition")

	// triage-decomposed event must be appended to parent.
	events, err := store.Events("ddx-postdecomp-01")
	require.NoError(t, err)
	var found bool
	for _, ev := range events {
		if ev.Kind == "triage-decomposed" {
			found = true
		}
	}
	assert.True(t, found, "triage-decomposed event must be appended to parent after post-attempt decomposition")
}

// TestPostAttemptTooLargeNoChanges_UsesQueueDepthNotAttemptPromptDepth verifies
// that the post-attempt orchestrator checks the queue-level max_decomposition_depth
// (from config) and not the implementation-level depth cap (hardcoded 2 in the
// execute-bead prompt). A bead at depth 1 with max_decomposition_depth=3 must
// still be split even if the rationale mentions the implementation depth cap.
func TestPostAttemptTooLargeNoChanges_UsesQueueDepthNotAttemptPromptDepth(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	// Create bead at depth 1: child of a closed root bead.
	root := &bead.Bead{ID: "ddx-qdepth-root", Title: "Root bead", Status: bead.StatusClosed}
	require.NoError(t, store.Create(context.Background(), root))

	candidate := &bead.Bead{
		ID:         "ddx-qdepth-child",
		Title:      "Child bead at depth 1",
		Parent:     "ddx-qdepth-root",
		Acceptance: "1. implement part A of the work\n2. implement part B of the work",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	var hookCalls int32
	// Material scope-reducing split: distinct implementation children so the
	// material-scope gate accepts while the depth assertion still holds.
	decomp := &PreClaimDecomposition{
		Rationale: "orchestrator split at depth 1",
		Children: []PreClaimDecompositionChild{
			{Title: "Subtask A", Description: "implement part A only", Acceptance: "1. implement part A of the work"},
			{Title: "Subtask B", Description: "implement part B only", Acceptance: "1. implement part B of the work"},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. implement part A of the work", Coverage: "covered by Subtask A"},
			{ParentAC: "2. implement part B of the work", Coverage: "covered by Subtask B"},
		},
	}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusNoChanges,
				// Rationale mentions implementation depth cap as a red herring;
				// the orchestrator must use queue-level max_decomposition_depth (3).
				NoChangesRationale: "status: open\norchestrator_action: decompose\nreason: implementation depth cap reached at depth 2",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 3, // depth 1 < 3, so orchestrator may still split
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-qdepth-child",
		PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
			atomic.AddInt32(&hookCalls, 1)
			return decomp, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Hook must have been called: depth 1 < max_decomposition_depth 3.
	assert.Equal(t, int32(1), atomic.LoadInt32(&hookCalls),
		"PostAttemptDecompositionHook must run: queue depth 1 < max_decomposition_depth 3")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Failures)

	// Children must be created when queue depth is under the configured cap.
	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	var children []bead.Bead
	for _, b := range all {
		if b.Parent == "ddx-qdepth-child" {
			children = append(children, b)
		}
	}
	assert.Len(t, children, 2, "children must be created when queue depth < max_decomposition_depth")
}

func TestPostAttemptTooLargeNoChanges_LossySplitBlocksHuman(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-lossy-postdecomp",
		Title:      "Lossy post-attempt decomp",
		Acceptance: "1. do stuff\n2. run tests",
	}
	require.NoError(t, store.Create(context.Background(

	// Lossy ACMap: empty Coverage for AC 2.
	), candidate))

	lossyDecomp := &PreClaimDecomposition{
		Rationale: "incomplete split",
		Children: []PreClaimDecompositionChild{
			{Title: "Part A", Description: "desc", Acceptance: "1. do stuff"},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. do stuff", Coverage: "covered by Part A"},
			{ParentAC: "2. run tests", Coverage: ""}, // empty → lossy
		},
	}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				NoChangesRationale: "status: open\norchestrator_action: decompose\nreason: too large",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	var eventSink bytes.Buffer
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: &eventSink,
		PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
			return lossyDecomp, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Failures)

	// No children must be created.
	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	for _, b := range all {
		assert.NotEqual(t, "ddx-lossy-postdecomp", b.Parent,
			"no children must be created for a lossy post-attempt split")
	}

	// Parent must be parked for operator review.
	got, err := store.Get(context.Background(), "ddx-lossy-postdecomp")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status,
		"lossy post-attempt split must park parent for operator review")

	// decomposition.blocked event must be emitted.
	assert.Contains(t, eventSink.String(), "post_attempt_decomposition.blocked",
		"blocked event must be emitted for lossy split")
}

// TestPreClaimDecompositionRequiresMaterialScopeReduction is table-driven over
// the four rejected split shapes: verification-only children, children
// duplicating the parent outcome, children missing acceptance mapping, and
// children that do not reduce implementation scope. Each shape is rejected by
// validatePreClaimDecomposition, creates zero child beads, and records exactly
// one refusal outcome via the existing lossy-split park path.
func TestPreClaimDecompositionRequiresMaterialScopeReduction(t *testing.T) {
	parentTitle := "Implement dual-path auth token refresh"
	parentDesc := "PROBLEM\nAuth refresh spans two packages.\n\nROOT CAUSE\ncli/internal/auth/refresh.go:40.\n\nPROPOSED FIX\nSplit the work into independent implementation boundaries.\n\nNON-SCOPE\nDo not change login UI."
	parentAcc := "1. implement token refresh in cli/internal/auth/refresh.go\n2. implement retry backoff in cli/internal/auth/retry.go\n3. cd cli && go test ./internal/auth/... -count=1"

	type shapeCase struct {
		name   string
		decomp *PreClaimDecomposition
		// substrings expected in validatePreClaimDecomposition error
		errSubstrings []string
	}

	cases := []shapeCase{
		{
			name: "verification_only_children",
			decomp: &PreClaimDecomposition{
				Rationale: "split for verification only",
				Children: []PreClaimDecompositionChild{
					{
						Title:       "Verify auth tests pass",
						Description: "Run the auth package test suite and review the diff for regressions.",
						Acceptance:  "1. Run go test ./internal/auth/...\n2. Verify tests pass\n3. lefthook run pre-commit passes",
					},
					{
						Title:       "Review the auth PR",
						Description: "Validate the change set and ensure the regression suite is green.",
						Acceptance:  "1. Review the PR\n2. Validate coverage reports",
					},
				},
				ACMap: []ACMapEntry{
					{ParentAC: "1. implement token refresh in cli/internal/auth/refresh.go", Coverage: "covered by Verify auth tests pass"},
					{ParentAC: "2. implement retry backoff in cli/internal/auth/retry.go", Coverage: "covered by Review the auth PR"},
					{ParentAC: "3. cd cli && go test ./internal/auth/... -count=1", Coverage: "covered by Verify auth tests pass"},
				},
			},
			errSubstrings: []string{"verification-only"},
		},
		{
			name: "children_duplicate_parent_outcome",
			decomp: &PreClaimDecomposition{
				Rationale: "cosmetic re-title of the same outcome",
				Children: []PreClaimDecompositionChild{
					{
						Title:       parentTitle,
						Description: parentDesc,
						Acceptance:  parentAcc,
					},
					{
						Title:       "Also implement dual-path auth token refresh",
						Description: "Same full parent outcome restated.",
						Acceptance:  parentAcc,
					},
				},
				ACMap: []ACMapEntry{
					{ParentAC: "1. implement token refresh in cli/internal/auth/refresh.go", Coverage: "covered by both children"},
					{ParentAC: "2. implement retry backoff in cli/internal/auth/retry.go", Coverage: "covered by both children"},
					{ParentAC: "3. cd cli && go test ./internal/auth/... -count=1", Coverage: "covered by both children"},
				},
			},
			errSubstrings: []string{"duplicates the parent outcome"},
		},
		{
			name: "children_missing_acceptance_mapping",
			decomp: &PreClaimDecomposition{
				Rationale: "children without an AC map",
				Children: []PreClaimDecompositionChild{
					{
						Title:       "Implement token refresh path",
						Description: "PROBLEM\nRefresh path.\n\nPROPOSED FIX\nImplement refresh.go only.\n",
						Acceptance:  "1. implement token refresh in cli/internal/auth/refresh.go\n2. TestAuthRefresh",
					},
					{
						Title:       "Implement retry backoff path",
						Description: "PROBLEM\nRetry path.\n\nPROPOSED FIX\nImplement retry.go only.\n",
						Acceptance:  "1. implement retry backoff in cli/internal/auth/retry.go\n2. TestAuthRetry",
					},
				},
				ACMap: nil, // missing mapping
			},
			errSubstrings: []string{"missing acceptance mapping"},
		},
		{
			name: "children_do_not_reduce_implementation_scope",
			// Each child restates every parent AC item in distinct wording so
			// duplicate-parent does not fire, but no scope partition occurs.
			decomp: &PreClaimDecomposition{
				Rationale: "siblings each restate the full parent AC set without partitioning",
				Children: []PreClaimDecompositionChild{
					{
						Title:       "Full-scope framing A",
						Description: "PROBLEM\nOwns the entire dual-path work.\n\nPROPOSED FIX\nImplement refresh, retry, and the go test gate under framing A.\n",
						Acceptance: "1. implement token refresh in cli/internal/auth/refresh.go and implement retry backoff in cli/internal/auth/retry.go\n" +
							"2. also cover cd cli && go test ./internal/auth/... -count=1 under framing A",
					},
					{
						Title:       "Full-scope framing B",
						Description: "PROBLEM\nAlso owns the entire dual-path work.\n\nPROPOSED FIX\nImplement refresh, retry, and the go test gate under framing B.\n",
						Acceptance: "1. implement token refresh in cli/internal/auth/refresh.go together with implement retry backoff in cli/internal/auth/retry.go\n" +
							"2. also cover cd cli && go test ./internal/auth/... -count=1 under framing B",
					},
				},
				ACMap: []ACMapEntry{
					{ParentAC: "1. implement token refresh in cli/internal/auth/refresh.go", Coverage: "covered by framing A and B"},
					{ParentAC: "2. implement retry backoff in cli/internal/auth/retry.go", Coverage: "covered by framing A and B"},
					{ParentAC: "3. cd cli && go test ./internal/auth/... -count=1", Coverage: "covered by framing A and B"},
				},
			},
			errSubstrings: []string{"does not reduce implementation scope"},
		},
	}

	// Ensure the four required shapes are represented.
	required := map[string]bool{
		"verification_only_children":                  false,
		"children_duplicate_parent_outcome":           false,
		"children_missing_acceptance_mapping":         false,
		"children_do_not_reduce_implementation_scope": false,
	}
	for _, tc := range cases {
		if _, ok := required[tc.name]; ok {
			required[tc.name] = true
		}
	}
	for name, present := range required {
		require.True(t, present, "table must include shape %s", name)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			parent := &bead.Bead{
				ID:          "ddx-msr-" + tc.name,
				Title:       parentTitle,
				Description: parentDesc,
				Acceptance:  parentAcc,
			}

			// Gate-level assertion: validatePreClaimDecomposition rejects the shape.
			err := validatePreClaimDecomposition(tc.decomp, parent)
			require.Error(t, err, "validatePreClaimDecomposition must reject shape %s", tc.name)
			for _, sub := range tc.errSubstrings {
				assert.Contains(t, err.Error(), sub, "rejection reason for %s", tc.name)
			}

			// Integration: refused split creates zero children and one refusal outcome.
			store := bead.NewStore(t.TempDir())
			require.NoError(t, store.Init(context.Background()))
			require.NoError(t, store.Create(context.Background(), parent))

			worker := &ExecuteBeadWorker{
				Store: store,
				Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
					t.Error("executor must not run for refused material-scope split")
					return ExecuteBeadReport{}, nil
				}),
			}
			cfgOpts := config.TestLoopConfigOpts{
				Assignee:              "worker",
				MaxDecompositionDepth: 3,
			}
			rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

			result, runErr := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
				Once:         true,
				TargetBeadID: parent.ID,
				PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
					return PreClaimIntakeResult{
						Outcome:       PreClaimIntakeTooLargeDecomposed,
						Detail:        "too large; proposed split",
						Decomposition: tc.decomp,
					}, nil
				},
			})
			require.NoError(t, runErr)
			require.NotNil(t, result)
			assert.Equal(t, 0, result.Attempts, "refused split must not count as an attempt")

			all, readErr := store.ReadAll(context.Background())
			require.NoError(t, readErr)
			for _, b := range all {
				assert.NotEqual(t, parent.ID, b.Parent,
					"no child beads must be created for refused shape %s", tc.name)
			}

			got, getErr := store.Get(context.Background(), parent.ID)
			require.NoError(t, getErr)
			assert.Equal(t, bead.StatusProposed, got.Status,
				"refused split must park parent for operator review")

			events, evErr := store.Events(parent.ID)
			require.NoError(t, evErr)
			refusalCount := 0
			for _, ev := range events {
				if ev.Kind == "intake.blocked" {
					refusalCount++
				}
			}
			assert.Equal(t, 1, refusalCount,
				"exactly one refusal outcome (intake.blocked) must be recorded for shape %s", tc.name)
		})
	}
}

// TestPreClaimDecompositionAcceptsIndependentImplementationBoundaries asserts
// a split whose children each carry a distinct concrete implementation outcome
// and full AC mapping is accepted and creates the expected children.
func TestPreClaimDecompositionAcceptsIndependentImplementationBoundaries(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	parent := &bead.Bead{
		ID:    "ddx-msr-accept-independent",
		Title: "Implement dual-path auth token refresh",
		Description: "PROBLEM\nAuth refresh spans two packages.\n\nROOT CAUSE\ncli/internal/auth/refresh.go:40.\n\n" +
			"PROPOSED FIX\nSplit into independent implementation boundaries.\n\nNON-SCOPE\nLogin UI.",
		Acceptance: "1. implement token refresh in cli/internal/auth/refresh.go\n" +
			"2. implement retry backoff in cli/internal/auth/retry.go\n" +
			"3. cd cli && go test ./internal/auth/... -count=1",
	}
	require.NoError(t, store.Create(context.Background(), parent))

	decomp := &PreClaimDecomposition{
		Rationale: "independent implementation boundaries for refresh and retry",
		Children: []PreClaimDecompositionChild{
			{
				Title: "Implement auth token refresh",
				Description: "PROBLEM\nRefresh path is incomplete.\n\nROOT CAUSE\ncli/internal/auth/refresh.go:40.\n\n" +
					"PROPOSED FIX\nImplement refresh only.\n\nNON-SCOPE\nRetry backoff.",
				Acceptance: "1. implement token refresh in cli/internal/auth/refresh.go\n" +
					"2. TestAuthTokenRefresh\n3. cd cli && go test ./internal/auth/... -run TestAuthTokenRefresh -count=1",
			},
			{
				Title: "Implement auth retry backoff",
				Description: "PROBLEM\nRetry backoff is incomplete.\n\nROOT CAUSE\ncli/internal/auth/retry.go:12.\n\n" +
					"PROPOSED FIX\nImplement retry only.\n\nNON-SCOPE\nToken refresh.",
				Acceptance: "1. implement retry backoff in cli/internal/auth/retry.go\n" +
					"2. TestAuthRetryBackoff\n3. cd cli && go test ./internal/auth/... -run TestAuthRetryBackoff -count=1",
			},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. implement token refresh in cli/internal/auth/refresh.go", Coverage: "covered by Implement auth token refresh AC 1"},
			{ParentAC: "2. implement retry backoff in cli/internal/auth/retry.go", Coverage: "covered by Implement auth retry backoff AC 1"},
			{ParentAC: "3. cd cli && go test ./internal/auth/... -count=1", Coverage: "covered by both children go test gates"},
		},
	}

	// Gate accepts.
	require.NoError(t, validatePreClaimDecomposition(decomp, parent))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Error("executor must not run after successful decomposition intake")
			return ExecuteBeadReport{}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: parent.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome:       PreClaimIntakeTooLargeDecomposed,
				Detail:        "too large; independent boundaries",
				Decomposition: decomp,
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Attempts)

	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	var children []bead.Bead
	for _, b := range all {
		if b.Parent == parent.ID {
			children = append(children, b)
		}
	}
	require.Len(t, children, 2, "accepted split must create both independent children")
	titles := map[string]bool{}
	for _, c := range children {
		titles[c.Title] = true
	}
	assert.True(t, titles["Implement auth token refresh"])
	assert.True(t, titles["Implement auth retry backoff"])

	got, err := store.Get(context.Background(), parent.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "successful decomp keeps parent open")
	assert.Equal(t, false, got.Extra[bead.ExtraExecutionElig])
}
