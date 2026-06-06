package agent

import (
	"bytes"
	"context"
	"encoding/json"
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
	got, err := store.Get("ddx-decomp-parent")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "parent must remain open after successful decomposition")

	// Two children must exist with Parent == candidate.ID.
	all, err := store.ReadAll()
	require.NoError(t, err)
	var children []bead.Bead
	for _, b := range all {
		if b.Parent == "ddx-decomp-parent" {
			children = append(children, b)
		}
	}
	assert.Len(t, children, 2, "two children must be created")

	// Parent must have dep edges wired to both children.
	got, err = store.Get("ddx-decomp-parent")
	require.NoError(t, err)
	assert.Len(t, got.DepIDs(), 2, "parent must have dep edges to both children")

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

	all, err := store.ReadAll()
	require.NoError(t, err)
	var children []bead.Bead
	for _, b := range all {
		if b.Parent == candidate.ID {
			children = append(children, b)
		}
	}
	require.Len(t, children, 1)
	assert.Equal(t, "Child from hook", children[0].Title)

	parent, err := store.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, parent.Status)
	assert.Equal(t, []string{children[0].ID}, parent.DepIDs())
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
	all, err := store.ReadAll()
	require.NoError(t, err)
	for _, b := range all {
		assert.NotEqual(t, "ddx-lossy-intake", b.Parent, "no children must be created for a lossy split")
	}

	// Parent must be parked for operator review.
	got, err := store.Get("ddx-lossy-intake")
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
	got, err := store.Get("ddx-dc-grand")
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
	all, err := store.ReadAll()
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
		Acceptance: "1. do the work",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	var hookCalls int32
	decomp := &PreClaimDecomposition{
		Rationale: "orchestrator split at depth 1",
		Children: []PreClaimDecompositionChild{
			{Title: "Subtask", Description: "subtask desc", Acceptance: "1. do the work"},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. do the work", Coverage: "fully covered by Subtask"},
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

	// Child must be created.
	all, err := store.ReadAll()
	require.NoError(t, err)
	var children []bead.Bead
	for _, b := range all {
		if b.Parent == "ddx-qdepth-child" {
			children = append(children, b)
		}
	}
	assert.Len(t, children, 1, "child must be created when queue depth < max_decomposition_depth")
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
	all, err := store.ReadAll()
	require.NoError(t, err)
	for _, b := range all {
		assert.NotEqual(t, "ddx-lossy-postdecomp", b.Parent,
			"no children must be created for a lossy post-attempt split")
	}

	// Parent must be parked for operator review.
	got, err := store.Get("ddx-lossy-postdecomp")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status,
		"lossy post-attempt split must park parent for operator review")

	// decomposition.blocked event must be emitted.
	assert.Contains(t, eventSink.String(), "post_attempt_decomposition.blocked",
		"blocked event must be emitted for lossy split")
}
