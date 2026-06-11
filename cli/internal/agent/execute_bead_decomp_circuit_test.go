package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecompositionDepDirection verifies that instrStep0SizeCheck no longer
// instructs the agent to wire the parent into the dependency graph. The
// decomposition flow may still add legitimate child-to-child or sibling /
// replacement edges, but the parent itself must not be a dependency target.
func TestDecompositionDepDirection(t *testing.T) {
	correct := "legitimate child-to-child or sibling/replacement edges"
	wrong := "ddx bead dep add <parent-id> <child-id>"

	assert.True(t,
		strings.Contains(instrStep0SizeCheck, correct),
		"instrStep0SizeCheck must describe legitimate child-specific dependency edges", correct,
	)
	assert.False(t,
		strings.Contains(instrStep0SizeCheck, wrong),
		"instrStep0SizeCheck must not contain %q (parent-as-dependency back-edge)", wrong,
	)
}

// TestMixedCommitCooldown verifies that the circuit-breaker parks a bead to
// proposed after 2 mixed_commit_and_no_changes_rationale outcomes within 24h.
func TestMixedCommitCooldown(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-mixed-cb", Title: "Mixed commit circuit breaker test"}
	require.NoError(t, store.Create(context.

		// Append a prior mixed_commit execute-bead event (the 1st occurrence).
		Background(), b))

	require.NoError(t, store.AppendEvent("ddx-mixed-cb", bead.BeadEvent{
		Kind:      "execute-bead",
		Summary:   ExecuteBeadStatusExecutionFailed,
		Body:      mixedCommitAndNoChangesRationaleReason,
		Actor:     "test-worker",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
	}))

	var callCount int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&callCount, 1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    mixedCommitAndNoChangesRationaleReason,
				BaseRev:   "abc1111",
				ResultRev: "def2222",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "test-worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-mixed-cb",
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&callCount), "executor must run exactly once")

	got, err := store.Get(context.Background(), "ddx-mixed-cb")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status,
		"bead must be parked to proposed after 2nd mixed_commit within 24h")
}

func TestExecuteBeadLoop_NiflheimEvidence_DecomposedParentIsExecutionIneligible(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	parent := &bead.Bead{ID: "ddx-niflheim-parent", Title: "Niflheim decomposed parent"}
	require.NoError(t, store.Create(context.Background(), parent))

	decomp := &PreClaimDecomposition{
		Rationale: "split the parent after mixed-commit decomposition",
		Children: []PreClaimDecompositionChild{
			{Title: "Child A", Description: "child a", Acceptance: "1. do child a"},
			{Title: "Child B", Description: "child b", Acceptance: "1. do child b"},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. split the parent", Coverage: "covered by Child A"},
			{ParentAC: "2. keep the parent out of the ready queue", Coverage: "covered by Child B"},
		},
	}

	var execCalls int32
	var escalationCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusExecutionFailed,
				Detail:             mixedCommitAndNoChangesRationaleReason,
				BaseRev:            "base-niflheim",
				ResultRev:          "result-niflheim",
				NoChangesRationale: "status: open\norchestrator_action: decompose\nreason: split the parent",
			}, nil
		}),
		EscalationNextFloor: func(actualPower int) (int, error) {
			atomic.AddInt32(&escalationCalls, 1)
			return actualPower + 1, nil
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", MaxDecompositionDepth: 3}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	runtime := ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: parent.ID,
		PostAttemptTriageHook: func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
			return TriageResult{
				Classification:    "decomposed",
				RecommendedAction: "close_decomposed_or_mark_execution_ineligible",
				Rationale:         "parent split already exists",
			}, nil
		},
		PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
			return decomp, nil
		},
	}

	result, err := worker.Run(context.Background(), rcfg, runtime)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int32(1), atomic.LoadInt32(&execCalls), "executor must run exactly once")
	assert.Zero(t, atomic.LoadInt32(&escalationCalls), "decomposed parents must not schedule min-power escalation")
	require.Len(t, result.Results, 1)
	assert.Equal(t, "decomposed", result.Results[0].OutcomeReason)
	assert.Equal(t, "execution_ineligible", result.Results[0].ExecutionDecision)
	assert.Len(t, result.Results[0].DecomposedChildIDs, 2)

	got, err := store.Get(context.Background(), parent.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "decomposed parent must stay open")
	assert.Equal(t, false, got.Extra[bead.ExtraExecutionElig], "decomposed parent must be execution-ineligible")

	events, err := store.Events(parent.ID)
	require.NoError(t, err)
	foundDecomposition := false
	for _, ev := range events {
		if ev.Kind != "triage-decomposed" {
			continue
		}
		foundDecomposition = true
		assert.Contains(t, ev.Body, `"child_ids"`, "decomposition event must record child IDs")
	}
	assert.True(t, foundDecomposition, "post-attempt decomposition must record a triage-decomposed event")

	children, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	childCount := 0
	for i := range children {
		if children[i].Parent != parent.ID {
			continue
		}
		childCount++
		assert.Equal(t, parent.ID, children[i].Parent, "child must reference the decomposed parent")
	}
	assert.Equal(t, 2, childCount, "post-attempt decomposition must create both child beads")

	_, err = worker.Run(context.Background(), rcfg, runtime)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&execCalls), "parent must not be re-claimed after parking")
}

func TestWorkRetryEscalation_NiflheimEvidence_DecomposedParentDoesNotRaiseMinPower(t *testing.T) {
	cases := []struct {
		name   string
		status string
		detail string
	}{
		{
			name:   "mixed commit plus decompose rationale",
			status: ExecuteBeadStatusExecutionFailed,
			detail: mixedCommitAndNoChangesRationaleReason,
		},
		{
			name:   "no changes decompose rationale",
			status: ExecuteBeadStatusNoChanges,
			detail: "agent decomposed the parent into executable children",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := bead.NewStore(t.TempDir())
			require.NoError(t, store.Init(context.Background()))

			parent := &bead.Bead{ID: "ddx-niflheim-retry-parent", Title: "Niflheim decomposed parent retry"}
			require.NoError(t, store.Create(context.Background(), parent))

			var execCalls int32
			var escalationCalls int32
			worker := &ExecuteBeadWorker{
				Store: store,
				Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
					atomic.AddInt32(&execCalls, 1)
					return ExecuteBeadReport{
						BeadID:             beadID,
						Status:             tc.status,
						Detail:             tc.detail,
						BaseRev:            "base-retry",
						ResultRev:          "result-retry",
						ActualPower:        50,
						RequestedProfile:   "standard",
						NoChangesRationale: "status: open\norchestrator_action: decompose\nreason: parent already split into child beads",
					}, nil
				}),
				EscalationNextFloor: func(actualPower int) (int, error) {
					atomic.AddInt32(&escalationCalls, 1)
					return actualPower + 10, nil
				},
			}

			cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", MaxDecompositionDepth: 3}
			rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
			runtime := ExecuteBeadLoopRuntime{
				Once:         true,
				TargetBeadID: parent.ID,
				PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
					return niflheimDecomposedParentFixture(), nil
				},
			}

			result, err := worker.Run(context.Background(), rcfg, runtime)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, int32(1), atomic.LoadInt32(&execCalls), "first pass should make one implementation attempt")
			assert.Zero(t, atomic.LoadInt32(&escalationCalls), "decomposed parent must not consult min-power escalation")
			require.Len(t, result.Results, 1)
			assert.Equal(t, "decomposed", result.Results[0].OutcomeReason)
			assert.Equal(t, "operator_attention", result.Results[0].CycleTrace[0].RetryAction)

			got, err := store.Get(context.Background(), parent.ID)
			require.NoError(t, err)
			assert.Equal(t, false, got.Extra[bead.ExtraExecutionElig], "parent must be execution-ineligible")
			assert.NotContains(t, got.Extra, executeLoopNoChangesNextMinPowerKey, "decompose must not request a stronger retry")

			_, err = worker.Run(context.Background(), rcfg, runtime)
			require.NoError(t, err)
			assert.Equal(t, int32(1), atomic.LoadInt32(&execCalls), "execution-ineligible parent must not be attempted again")
			assert.Zero(t, atomic.LoadInt32(&escalationCalls), "second pass must not raise min power either")
		})
	}
}

func TestExecuteBeadResult_NiflheimEvidence_DecomposedParentDecisionAudit(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	parent := &bead.Bead{ID: "ddx-niflheim-audit-parent", Title: "Niflheim decomposed parent audit"}
	require.NoError(t, store.Create(context.Background(), parent))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:              beadID,
				AttemptID:           "attempt-niflheim-decompose",
				Status:              ExecuteBeadStatusExecutionFailed,
				Detail:              mixedCommitAndNoChangesRationaleReason,
				BaseRev:             "base-audit",
				ResultRev:           "result-audit",
				Harness:             "codex",
				Provider:            "openai",
				Model:               "gpt-5",
				ActualPower:         72,
				RequestedProfile:    "standard",
				RoutingIntentSource: "readiness",
				EstimatedDifficulty: "hard",
				InferredPowerClass:  "smart",
				EscalationCount:     0,
				NoChangesRationale:  "status: open\norchestrator_action: decompose\nreason: parent already split into child beads",
			}, nil
		}),
		EscalationNextFloor: func(actualPower int) (int, error) {
			t.Fatalf("decomposed parent must not ask for min-power escalation")
			return 0, nil
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", MaxDecompositionDepth: 3}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: parent.ID,
		PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
			return niflheimDecomposedParentFixture(), nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	raw, err := json.Marshal(result)
	require.NoError(t, err)
	var resultJSON map[string]any
	require.NoError(t, json.Unmarshal(raw, &resultJSON))
	results, ok := resultJSON["results"].([]any)
	require.True(t, ok)
	require.Len(t, results, 1)
	resultReport, ok := results[0].(map[string]any)
	require.True(t, ok)
	cycle := firstCycleTrace(t, resultReport)
	assert.Equal(t, "decomposed", cycle["failure_class"])
	assert.Equal(t, "operator_attention", cycle["retry_action"])
	assert.Equal(t, float64(0), cycle["escalation_count"])
	assert.Equal(t, "standard", nestedString(t, cycle, "requested_route", "profile"))
	assert.Equal(t, "readiness", nestedString(t, cycle, "requested_route", "routing_intent_source"))
	assert.Equal(t, "gpt-5", nestedString(t, cycle, "actual_route", "model"))
	assert.Equal(t, "execution_ineligible", cycle["execution_decision"])
	childIDs, ok := cycle["decomposed_child_ids"].([]any)
	require.True(t, ok)
	assert.Len(t, childIDs, 2)

	events, err := store.Events(parent.ID)
	require.NoError(t, err)
	var audit map[string]any
	for _, ev := range events {
		if ev.Kind == "execute-bead" {
			audit = decisionAuditFromEventBody(t, ev.Body)
			break
		}
	}
	require.NotNil(t, audit)
	assert.Equal(t, "decomposed", audit["failure_class"])
	assert.Equal(t, "operator_attention", audit["retry_action"])
	assert.Equal(t, float64(0), audit["escalation_count"])
	assert.Equal(t, "standard", nestedString(t, audit, "requested_route", "profile"))
	assert.Equal(t, "openai", nestedString(t, audit, "actual_route", "provider"))
	assert.Equal(t, "execution_ineligible", audit["execution_decision"])
	eventChildIDs, ok := audit["decomposed_child_ids"].([]any)
	require.True(t, ok)
	assert.Len(t, eventChildIDs, 2)
}

func niflheimDecomposedParentFixture() *PreClaimDecomposition {
	return &PreClaimDecomposition{
		Rationale: "split the Niflheim parent into executable child beads",
		Children: []PreClaimDecompositionChild{
			{Title: "Niflheim child A", Description: "child a", Acceptance: "1. do child a"},
			{Title: "Niflheim child B", Description: "child b", Acceptance: "1. do child b"},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. split the parent", Coverage: "covered by Niflheim child A"},
			{ParentAC: "2. keep the parent out of the ready queue", Coverage: "covered by Niflheim child B"},
		},
	}
}

// TestMixedCommitCooldown_FirstOccurrenceDoesNotPark verifies that a single
// mixed_commit outcome (no prior events) does NOT park the bead.
func TestMixedCommitCooldown_FirstOccurrenceDoesNotPark(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-mixed-first", Title: "First mixed commit; must not park"}
	require.NoError(t, store.Create(context.

		// No prior events — this is the first occurrence.
		Background(), b))

	var callCount int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&callCount, 1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    mixedCommitAndNoChangesRationaleReason,
				BaseRev:   "abc1111",
				ResultRev: "def2222",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "test-worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-mixed-first",
	})
	require.NoError(t, err)

	got, err := store.Get(context.Background(), "ddx-mixed-first")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status,
		"bead must remain open after first mixed_commit (circuit-breaker needs 2 occurrences)")
}

// TestMixedCommitCooldown_OutsideWindowDoesNotPark verifies that a prior
// mixed_commit event older than 24h does not trigger the circuit-breaker.
func TestMixedCommitCooldown_OutsideWindowDoesNotPark(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-mixed-old", Title: "Old mixed commit; must not park"}
	require.NoError(t, store.Create(context.

		// Prior event is older than the 24h window.
		Background(), b))

	require.NoError(t, store.AppendEvent("ddx-mixed-old", bead.BeadEvent{
		Kind:      "execute-bead",
		Summary:   ExecuteBeadStatusExecutionFailed,
		Body:      mixedCommitAndNoChangesRationaleReason,
		Actor:     "test-worker",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC().Add(-25 * time.Hour),
	}))

	var callCount int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&callCount, 1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    mixedCommitAndNoChangesRationaleReason,
				BaseRev:   "abc1111",
				ResultRev: "def2222",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "test-worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-mixed-old",
	})
	require.NoError(t, err)

	got, err := store.Get(context.Background(), "ddx-mixed-old")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status,
		"bead must remain open when prior mixed_commit event is outside the 24h window")
}
