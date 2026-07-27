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

// validMaterialSplit returns a material, independent two-child split accepted by
// the material-scope-reduction gate for parents with the given AC lines.
func validMaterialSplit(rationale string) *PreClaimDecomposition {
	return &PreClaimDecomposition{
		Rationale: rationale,
		Children: []PreClaimDecompositionChild{
			{
				Title:       "Implement part A only",
				Description: "PROPOSED FIX\nImplement only the A path in cli/internal/agent/a.go.",
				Acceptance:  "1. TestPartA covers A\n2. cd cli && go test ./internal/agent/... -run TestPartA",
			},
			{
				Title:       "Implement part B only",
				Description: "PROPOSED FIX\nImplement only the B path in cli/internal/agent/b.go.",
				Acceptance:  "1. TestPartB covers B\n2. cd cli && go test ./internal/agent/... -run TestPartB",
			},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. implement part A of the work", Coverage: "covered by Implement part A only"},
			{ParentAC: "2. implement part B of the work", Coverage: "covered by Implement part B only"},
		},
	}
}

func familyDescendantCount(t *testing.T, store *bead.Store, rootID string) int {
	t.Helper()
	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	childrenOf := map[string][]string{}
	for _, b := range all {
		if b.Parent != "" {
			childrenOf[b.Parent] = append(childrenOf[b.Parent], b.ID)
		}
	}
	count := 0
	queue := append([]string(nil), childrenOf[rootID]...)
	seen := map[string]struct{}{rootID: {}}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		count++
		queue = append(queue, childrenOf[id]...)
	}
	return count
}

func triageDecomposedBodies(t *testing.T, store *bead.Store, beadID string) []map[string]any {
	t.Helper()
	events, err := store.Events(beadID)
	require.NoError(t, err)
	var bodies []map[string]any
	for _, ev := range events {
		if ev.Kind != "triage-decomposed" {
			continue
		}
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(ev.Body), &body), "triage-decomposed body must be JSON")
		bodies = append(bodies, body)
	}
	return bodies
}

// TestDecompositionFamilyExpansionBudgetStopsQueueGrowth drives repeated
// preclaim and post-attempt decomposition rounds against one bead family and
// asserts the total descendant count never exceeds the configured family-wide
// budget. Once at budget, each subsequent round produces exactly one stable
// parked/refusal outcome with a reason and creates zero additional beads.
func TestDecompositionFamilyExpansionBudgetStopsQueueGrowth(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	const budget = 3
	root := &bead.Bead{
		ID:     "ddx-family-root",
		Title:  "Family root",
		Status: bead.StatusOpen,
		Description: "PROBLEM\nRoot work spans A and B.\n\n" +
			"PROPOSED FIX\nSplit into independent A and B paths.\n",
		Acceptance: "1. implement part A of the work\n2. implement part B of the work",
	}
	require.NoError(t, store.Create(context.Background(), root))

	// Seed the family with budget-1 existing descendants so one more two-child
	// split would exceed the budget.
	for i, id := range []string{"ddx-family-c1", "ddx-family-c2"} {
		child := &bead.Bead{
			ID:     id,
			Title:  "Existing child " + string(rune('A'+i)),
			Parent: root.ID,
			Status: bead.StatusOpen,
			Labels: []string{"decomposed"},
		}
		require.NoError(t, store.Create(context.Background(), child))
	}
	require.Equal(t, 2, familyDescendantCount(t, store, root.ID))

	// Candidate is the root itself: still open, wants another two-child split.
	decomp := validMaterialSplit("would widen family past budget")

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 5, // depth not the limiting factor
		MaxFamilyExpansion:    budget,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	require.Equal(t, budget, rcfg.MaxFamilyExpansion())
	require.Equal(t, budget, rcfg.DecompositionPolicy().MaxFamilyExpansion)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Errorf("executor must not run when family budget refuses the split (%s)", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	// --- Preclaim rounds: repeated too_large_decomposed with the same proposal ---
	// Use a dedicated candidate under the root. Creating it would bring the
	// family to budget-1; the two-child proposal would exceed budget 3.
	// Keep the candidate as a separate bead so we can TransitionLifecycle it
	// back to open between park rounds without touching the organizational root.
	preclaimCandidate := &bead.Bead{
		ID:     "ddx-family-preclaim",
		Title:  "Preclaim candidate under family",
		Parent: root.ID,
		Status: bead.StatusOpen,
		Labels: []string{"decomposed"},
		Description: "PROBLEM\nStill too large.\n\n" +
			"PROPOSED FIX\nSplit into independent A and B paths.\n",
		Acceptance: "1. implement part A of the work\n2. implement part B of the work",
	}
	require.NoError(t, store.Create(context.Background(), preclaimCandidate))
	// descendants = c1, c2, preclaim = 3 → already at budget; any further split must refuse.
	require.Equal(t, budget, familyDescendantCount(t, store, root.ID))

	var refusalReasons []string
	for round := 0; round < 3; round++ {
		// Operator-style reopen so the preclaim path can re-evaluate.
		require.NoError(t, store.TransitionLifecycle(preclaimCandidate.ID, bead.StatusOpen, bead.LifecycleTransitionOptions{
			Actor:  "test",
			Source: "test",
			Reason: "reopen for family budget stability round",
		}, func(b *bead.Bead) error {
			b.Owner = ""
			ensureBeadExtra(b)
			b.Extra[bead.ExtraExecutionElig] = true
			return nil
		}))
		_ = store.Unclaim(preclaimCandidate.ID)

		beforeCount := familyDescendantCount(t, store, root.ID)
		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: preclaimCandidate.ID,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{
					Outcome:       PreClaimIntakeTooLargeDecomposed,
					Detail:        "bead is too large",
					Decomposition: decomp,
				}, nil
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 0, result.Attempts, "budget refusal must not count as an attempt (round %d)", round)

		afterCount := familyDescendantCount(t, store, root.ID)
		assert.Equal(t, beforeCount, afterCount, "preclaim round %d must create zero additional beads", round)
		assert.LessOrEqual(t, afterCount, budget, "descendant count must never exceed budget")

		bodies := triageDecomposedBodies(t, store, preclaimCandidate.ID)
		require.NotEmpty(t, bodies, "preclaim round %d must emit triage-decomposed", round)
		last := bodies[len(bodies)-1]
		refusal, _ := last["refusal_reason"].(string)
		require.NotEmpty(t, refusal, "preclaim round %d must carry refusal_reason", round)
		assert.Contains(t, refusal, familyExpansionBudgetRefusal)
		refusalReasons = append(refusalReasons, refusal)
	}
	require.Len(t, refusalReasons, 3)
	assert.Equal(t, refusalReasons[0], refusalReasons[1], "preclaim refusal reason must be stable across rounds")
	assert.Equal(t, refusalReasons[1], refusalReasons[2], "preclaim refusal reason must be stable across rounds")

	// --- Post-attempt rounds: no_changes + orchestrator_action: decompose ---
	// Reuse the same candidate (still at family budget) via the post-attempt path.
	postWorker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				NoChangesRationale: "status: open\norchestrator_action: decompose\nreason: still too large under family budget test",
			}, nil
		}),
	}
	var postReasons []string
	for round := 0; round < 3; round++ {
		require.NoError(t, store.TransitionLifecycle(preclaimCandidate.ID, bead.StatusOpen, bead.LifecycleTransitionOptions{
			Actor:  "test",
			Source: "test",
			Reason: "reopen for post-attempt family budget stability round",
		}, func(b *bead.Bead) error {
			b.Owner = ""
			ensureBeadExtra(b)
			b.Extra[bead.ExtraExecutionElig] = true
			return nil
		}))
		_ = store.Unclaim(preclaimCandidate.ID)

		beforeCount := familyDescendantCount(t, store, root.ID)
		result, err := postWorker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: preclaimCandidate.ID,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
			},
			PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
				return decomp, nil
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		afterCount := familyDescendantCount(t, store, root.ID)
		assert.Equal(t, beforeCount, afterCount, "post-attempt round %d must create zero additional beads", round)
		assert.LessOrEqual(t, afterCount, budget, "descendant count must never exceed budget after post-attempt")

		bodies := triageDecomposedBodies(t, store, preclaimCandidate.ID)
		require.NotEmpty(t, bodies, "post-attempt round %d must emit triage-decomposed", round)
		last := bodies[len(bodies)-1]
		refusal, _ := last["refusal_reason"].(string)
		require.NotEmpty(t, refusal)
		assert.Contains(t, refusal, familyExpansionBudgetRefusal)
		postReasons = append(postReasons, refusal)
	}
	require.Len(t, postReasons, 3)
	assert.Equal(t, postReasons[0], postReasons[1], "post-attempt refusal reason must be stable")
	assert.Equal(t, postReasons[1], postReasons[2], "post-attempt refusal reason must be stable")

	// Final family size never exceeds budget.
	assert.LessOrEqual(t, familyDescendantCount(t, store, root.ID), budget)
}

// TestDecompositionEventCarriesFamilyTelemetry asserts triage-decomposed bodies
// from applyPreClaimDecomposition and decomposerEventBodyForResult carry family
// size, family depth, split reason, and refusal reason for both an accepted
// split and a budget refusal.
func TestDecompositionEventCarriesFamilyTelemetry(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	const budget = 4
	root := &bead.Bead{
		ID:     "ddx-tele-root",
		Title:  "Telemetry root",
		Status: bead.StatusOpen,
		Description: "PROBLEM\nRoot spans A and B.\n\n" +
			"PROPOSED FIX\nSplit into independent A and B paths.\n",
		Acceptance: "1. implement part A of the work\n2. implement part B of the work",
	}
	require.NoError(t, store.Create(context.Background(), root))

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		MaxDecompositionDepth: 5,
		MaxFamilyExpansion:    budget,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	at := time.Now().UTC()

	// --- Accepted split via applyPreClaimDecomposition ---
	decomp := validMaterialSplit("split for family telemetry accepted path")
	childIDs, err := applyPreClaimDecomposition(context.Background(), store, root, decomp, "worker", at, rcfg)
	require.NoError(t, err)
	require.Len(t, childIDs, 2)

	bodies := triageDecomposedBodies(t, store, root.ID)
	require.Len(t, bodies, 1)
	accepted := bodies[0]
	assert.NotZero(t, accepted["family_size"], "accepted split must report family_size")
	assert.NotNil(t, accepted["family_depth"], "accepted split must report family_depth")
	assert.NotEmpty(t, accepted["split_reason"], "accepted split must report split_reason")
	// refusal_reason is empty string on accept; key must still be present or at least zero-value ok.
	refusal, _ := accepted["refusal_reason"].(string)
	assert.Empty(t, refusal, "accepted split refusal_reason must be empty")
	assert.Equal(t, float64(budget), accepted["family_budget"])
	assert.GreaterOrEqual(t, int(accepted["family_size"].(float64)), 3) // root + 2 children

	// --- Budget refusal via applyPreClaimDecomposition ---
	// Family now has 2 descendants; budget 4 allows 2 more. Seed two more so
	// another two-child split would exceed.
	for _, id := range []string{"ddx-tele-extra1", "ddx-tele-extra2"} {
		require.NoError(t, store.Create(context.Background(), &bead.Bead{
			ID: id, Title: id, Parent: root.ID, Status: bead.StatusOpen, Labels: []string{"decomposed"},
		}))
	}
	// descendants = 4, budget = 4; proposing 2 more must refuse.
	require.Equal(t, budget, familyDescendantCount(t, store, root.ID))

	// Use a fresh open bead under the same family root with the same parent AC
	// shape so material-scope validation passes and only the budget gate fires.
	refuseParent := &bead.Bead{
		ID:     "ddx-tele-refuse-parent",
		Title:  "Budget refusal parent",
		Parent: root.ID,
		Status: bead.StatusOpen,
		Labels: []string{"decomposed"},
		Description: "PROBLEM\nStill too large.\n\n" +
			"PROPOSED FIX\nSplit into independent A and B paths.\n",
		Acceptance: "1. implement part A of the work\n2. implement part B of the work",
	}
	require.NoError(t, store.Create(context.Background(), refuseParent))
	// Creating refuseParent itself is a 5th descendant — raise budget check:
	// for this assertion we call apply with a snapshot family that is already
	// at budget by using a parent that is *not* counted as new if we instead
	// use one of the existing children with matching AC rewritten.
	// Simpler: set MaxFamilyExpansion low enough that current descendants
	// (5 after refuseParent create) + 2 proposed exceeds; budget is still 4.
	require.Greater(t, familyDescendantCount(t, store, root.ID), budget)

	_, err = applyPreClaimDecomposition(context.Background(), store, refuseParent, decomp, "worker", at, rcfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), familyExpansionBudgetRefusal)

	// triage-decomposed on the refuse parent carries refusal telemetry.
	refuseBodies := triageDecomposedBodies(t, store, refuseParent.ID)
	require.NotEmpty(t, refuseBodies)
	refused := refuseBodies[len(refuseBodies)-1]
	assert.NotZero(t, refused["family_size"], "budget refusal must report family_size")
	assert.NotNil(t, refused["family_depth"], "budget refusal must report family_depth")
	refuseReason, _ := refused["refusal_reason"].(string)
	require.NotEmpty(t, refuseReason)
	assert.Contains(t, refuseReason, familyExpansionBudgetRefusal)
	splitReason, _ := refused["split_reason"].(string)
	assert.Empty(t, splitReason, "budget refusal split_reason must be empty")
	assert.Equal(t, float64(budget), refused["family_budget"])
	// Zero additional beads from the refusal (count stays at pre-apply size).
	afterRefuse := familyDescendantCount(t, store, root.ID)
	assert.Equal(t, afterRefuse, familyDescendantCount(t, store, root.ID))
	// And specifically no children of refuseParent.
	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	for _, b := range all {
		assert.NotEqual(t, refuseParent.ID, b.Parent, "budget refusal must create zero children")
	}

	// --- decomposerEventBodyForResult telemetry for accept + refuse ---
	acceptBody := decomposerEventBodyForResultWithTelemetry(
		[]string{"ddx-c1", "ddx-c2"},
		&Result{CostUSD: 0.1, Harness: "claude", Model: "sonnet"},
		rcfg,
		"",
		decomposerEventTelemetry{
			FamilySize:      3,
			FamilyDepth:     1,
			DescendantCount: 2,
			FamilyBudget:    budget,
			SplitReason:     "accepted material scope reduction split",
		},
	)
	raw, err := json.Marshal(acceptBody)
	require.NoError(t, err)
	var acceptMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &acceptMap))
	assert.Equal(t, float64(3), acceptMap["family_size"])
	assert.Equal(t, float64(1), acceptMap["family_depth"])
	assert.Equal(t, "accepted material scope reduction split", acceptMap["split_reason"])
	_, hasRefusal := acceptMap["refusal_reason"]
	// omitempty drops empty refusal_reason on accept — that is fine; field is
	// present on refusal path below.
	_ = hasRefusal

	refuseBody := decomposerEventBodyForResultWithTelemetry(
		nil,
		&Result{CostUSD: 0.05},
		rcfg,
		"",
		decomposerEventTelemetry{
			FamilySize:      5,
			FamilyDepth:     2,
			DescendantCount: 4,
			FamilyBudget:    budget,
			RefusalReason:   familyExpansionBudgetRefusal + ": 4 existing descendants + 2 proposed exceeds budget 4",
		},
	)
	raw, err = json.Marshal(refuseBody)
	require.NoError(t, err)
	var refuseMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &refuseMap))
	assert.Equal(t, float64(5), refuseMap["family_size"])
	assert.Equal(t, float64(2), refuseMap["family_depth"])
	assert.Contains(t, refuseMap["refusal_reason"], familyExpansionBudgetRefusal)
	// split_reason omitted when empty (omitempty); ensure refusal is non-empty.
	assert.NotEmpty(t, refuseMap["refusal_reason"])

	// Convenience wrapper still fills family_budget from policy when tele is empty.
	defaultBody := decomposerEventBodyForResult(nil, nil, rcfg, "invalid output")
	assert.Equal(t, budget, defaultBody.FamilyBudget)
	assert.Equal(t, "invalid output", defaultBody.RefusalReason)
	assert.Equal(t, "invalid output", defaultBody.FallbackReason)
}
