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

// TestAutoDecomposeEpic_HappyPath validates that a genuinely undecomposed epic
// with a valid description and labels is dispatched to the preclaim decomposer.
func TestAutoDecomposeEpic_HappyPath(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:          "ddx-auto-decompose-happy",
		Title:       "epic: auto-decompose happy path",
		IssueType:   "epic",
		Description: "PROBLEM\nThis epic has no children and needs decomposition.\n\nROOT CAUSE\nThe epic was created but not broken down.\n\nPROPOSED FIX\nSplit into child tasks.\n\nNON-SCOPE\nNone.",
		Acceptance:  "1. Children are created with proper structure\n2. cd cli && go test ./internal/agent/... green\n",
		Labels:      []string{"spec:FEAT-010", "phase:2"},
	}
	require.NoError(t, store.Create(context.Background(), b))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	decomposerCalled := false
	decomposeRunner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		decomposerCalled = true
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
		return &Result{
			ExitCode: 0,
			Output:   string(out),
			CostUSD:  0.05,
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := AutoDecomposeEpic(ctx, store, decomposeRunner, rcfg, t.TempDir(), b.ID)

	require.NoError(t, err)
	assert.False(t, result.Failed)
	assert.Equal(t, 2, len(result.ChildIDs))
	assert.True(t, decomposerCalled, "decomposer must be dispatched")
	// Note: cost is not returned from runPreClaimDecomposer, so it defaults to 0.
	// The actual cost tracking would come from the dispatchLifecycleRun result,
	// but that's handled separately in the dispatch flow.

	// Verify parent execution-eligible is set to false
	parent, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, false, parent.Extra[bead.ExtraExecutionElig])

	// Verify auto_decompose_attempted event was emitted
	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var foundEvent bool
	for _, ev := range events {
		if ev.Kind == autoDecomposeAttemptedEventKind {
			foundEvent = true
			var body decomposerEventBody
			err := json.Unmarshal([]byte(ev.Body), &body)
			require.NoError(t, err)
			assert.Equal(t, 2, len(body.ChildIDs))
			// Cost defaults to 0 since runPreClaimDecomposer doesn't return it
		}
	}
	assert.True(t, foundEvent, "auto_decompose_attempted event must be emitted")

	// Verify child beads were created
	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	var children []bead.Bead
	for _, bb := range all {
		if bb.Parent == b.ID {
			children = append(children, bb)
		}
	}
	assert.Len(t, children, 2)
}

// TestAutoDecomposeEpic_PreconditionGates validates that each precondition gate
// correctly rejects invalid epics without dispatching the decomposer.
func TestAutoDecomposeEpic_PreconditionGates(t *testing.T) {
	tests := []struct {
		name          string
		makeBead      func() *bead.Bead
		wantErrReason string
	}{
		{
			name: "not_epic",
			makeBead: func() *bead.Bead {
				return &bead.Bead{
					ID:          "ddx-gate-not-epic",
					Title:       "regular task",
					IssueType:   "task",
					Description: "PROBLEM\nTest.\n\nROOT CAUSE\nTest.\n\nPROPOSED FIX\nTest.\n",
					Acceptance:  "1. Test passes\n",
					Labels:      []string{"spec:FEAT-010"},
				}
			},
			wantErrReason: "not_epic",
		},
		{
			name: "has_children",
			makeBead: func() *bead.Bead {
				return &bead.Bead{
					ID:          "ddx-gate-has-children",
					Title:       "epic: with children",
					IssueType:   "epic",
					Description: "PROBLEM\nTest.\n\nROOT CAUSE\nTest.\n\nPROPOSED FIX\nTest.\n",
					Acceptance:  "1. Test passes\n",
					Labels:      []string{"spec:FEAT-010"},
				}
			},
			wantErrReason: "has_children",
		},
		{
			name: "manual_hold_label",
			makeBead: func() *bead.Bead {
				return &bead.Bead{
					ID:          "ddx-gate-manual-hold",
					Title:       "epic: manual hold",
					IssueType:   "epic",
					Description: "PROBLEM\nTest.\n\nROOT CAUSE\nTest.\n\nPROPOSED FIX\nTest.\n",
					Acceptance:  "1. Test passes\n",
					Labels:      []string{"spec:FEAT-010", "manual-hold"},
				}
			},
			wantErrReason: "override_label",
		},
		{
			name: "no_auto_decompose_label",
			makeBead: func() *bead.Bead {
				return &bead.Bead{
					ID:          "ddx-gate-no-auto-decompose",
					Title:       "epic: no-auto-decompose",
					IssueType:   "epic",
					Description: "PROBLEM\nTest.\n\nROOT CAUSE\nTest.\n\nPROPOSED FIX\nTest.\n",
					Acceptance:  "1. Test passes\n",
					Labels:      []string{"spec:FEAT-010", "no-auto-decompose"},
				}
			},
			wantErrReason: "override_label",
		},
		{
			name: "container_label",
			makeBead: func() *bead.Bead {
				return &bead.Bead{
					ID:          "ddx-gate-container",
					Title:       "epic: container",
					IssueType:   "epic",
					Description: "PROBLEM\nTest.\n\nROOT CAUSE\nTest.\n\nPROPOSED FIX\nTest.\n",
					Acceptance:  "1. Test passes\n",
					Labels:      []string{"spec:FEAT-010", "container"},
				}
			},
			wantErrReason: "override_label",
		},
		{
			name: "missing_problem_section",
			makeBead: func() *bead.Bead {
				return &bead.Bead{
					ID:          "ddx-gate-missing-problem",
					Title:       "epic: missing problem",
					IssueType:   "epic",
					Description: "ROOT CAUSE\nTest.\n\nPROPOSED FIX\nTest.\n",
					Acceptance:  "1. Test passes\n",
					Labels:      []string{"spec:FEAT-010"},
				}
			},
			wantErrReason: "malformed_description",
		},
		{
			name: "missing_proposed_fix_section",
			makeBead: func() *bead.Bead {
				return &bead.Bead{
					ID:          "ddx-gate-missing-proposed",
					Title:       "epic: missing proposed",
					IssueType:   "epic",
					Description: "PROBLEM\nTest.\n\nROOT CAUSE\nTest.\n",
					Acceptance:  "1. Test passes\n",
					Labels:      []string{"spec:FEAT-010"},
				}
			},
			wantErrReason: "malformed_description",
		},
		{
			name: "missing_acceptance_criteria",
			makeBead: func() *bead.Bead {
				return &bead.Bead{
					ID:          "ddx-gate-missing-ac",
					Title:       "epic: missing ac",
					IssueType:   "epic",
					Description: "PROBLEM\nTest.\n\nROOT CAUSE\nTest.\n\nPROPOSED FIX\nTest.\n",
					Acceptance:  "no numbered criteria here",
					Labels:      []string{"spec:FEAT-010"},
				}
			},
			wantErrReason: "malformed_description",
		},
		{
			name: "missing_spec_or_area_label",
			makeBead: func() *bead.Bead {
				return &bead.Bead{
					ID:          "ddx-gate-missing-label",
					Title:       "epic: missing label",
					IssueType:   "epic",
					Description: "PROBLEM\nTest.\n\nROOT CAUSE\nTest.\n\nPROPOSED FIX\nTest.\n",
					Acceptance:  "1. Test passes\n",
					Labels:      []string{"phase:2"},
				}
			},
			wantErrReason: "malformed_description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := bead.NewStore(t.TempDir())
			require.NoError(t, store.Init(context.Background()))

			b := tt.makeBead()
			require.NoError(t, store.Create(context.Background(

			// If this is the has_children test, create a child
			), b))

			if tt.name == "has_children" {
				child := &bead.Bead{
					ID:     "ddx-gate-child",
					Parent: b.ID,
					Title:  "child",
				}
				require.NoError(t, store.Create(context.Background(), child))
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			decomposerCalled := false
			decomposeRunner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
				decomposerCalled = true
				return &Result{ExitCode: 0}, nil
			})

			cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
			rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

			_, err := AutoDecomposeEpic(ctx, store, decomposeRunner, rcfg, t.TempDir(), b.ID)

			assert.Error(t, err)
			assert.False(t, decomposerCalled, "decomposer must NOT be dispatched for precondition failure")

			var typeErr *AutoDecomposeEpicError
			require.ErrorAs(t, err, &typeErr)
			assert.Equal(t, tt.wantErrReason, typeErr.Reason)
		})
	}
}

// TestAutoDecomposeEpic_FallbackDecompositionWorks validates that when the
// decomposer output is invalid, the fallback decomposition succeeds and
// creates child beads. Note: the fallback always succeeds for valid epics,
// so we can't easily test decomposer failures without invalid preconditions.
func TestAutoDecomposeEpic_FallbackDecompositionWorks(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:          "ddx-fallback-test",
		Title:       "epic: fallback decomposition",
		IssueType:   "epic",
		Description: "PROBLEM\nTest.\n\nROOT CAUSE\nTest.\n\nPROPOSED FIX\nTest.\n",
		Acceptance:  "1. First AC\n2. Second AC\n3. Third AC\n",
		Labels:      []string{"spec:FEAT-010"},
	}
	require.NoError(t, store.Create(context.Background(), b))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	decomposeRunner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		// Return invalid output, which will trigger fallback decomposition
		return &Result{
			ExitCode: 0,
			Output:   "invalid json {{{",
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := AutoDecomposeEpic(ctx, store, decomposeRunner, rcfg, t.TempDir(), b.ID)

	require.NoError(t, err)
	assert.False(t, result.Failed, "fallback decomposition should succeed")
	assert.True(t, len(result.ChildIDs) > 0, "fallback should create child beads")

	// Verify parent execution-eligible is set to false
	parent, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, false, parent.Extra[bead.ExtraExecutionElig])

	// Verify auto_decompose_attempted event was emitted
	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var foundEvent bool
	for _, ev := range events {
		if ev.Kind == autoDecomposeAttemptedEventKind {
			foundEvent = true
		}
	}
	assert.True(t, foundEvent, "auto_decompose_attempted event must be emitted")
}

// TestAutoDecomposeEpic_AttemptCapGate validates that the attempt cap is enforced.
func TestAutoDecomposeEpic_AttemptCapGate(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:          "ddx-attempt-cap",
		Title:       "epic: attempt cap",
		IssueType:   "epic",
		Description: "PROBLEM\nTest.\n\nROOT CAUSE\nTest.\n\nPROPOSED FIX\nTest.\n",
		Acceptance:  "1. Test passes\n",
		Labels:      []string{"spec:FEAT-010"},
	}
	require.NoError(t, store.Create(context.

		// Seed 3 prior attempts (at the cap)
		Background(), b))

	for i := 0; i < 3; i++ {
		_ = store.AppendEvent(b.ID, bead.BeadEvent{
			Kind:      "auto_decompose_attempt_gate_failed",
			Summary:   "prior attempt",
			CreatedAt: time.Now().UTC(),
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	decomposeRunner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		t.Fatalf("decomposer should not be called when attempt cap is reached")
		return nil, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := AutoDecomposeEpic(ctx, store, decomposeRunner, rcfg, t.TempDir(), b.ID)

	assert.Error(t, err)
	var typeErr *AutoDecomposeEpicError
	require.ErrorAs(t, err, &typeErr)
	assert.Equal(t, "attempt_cap_reached", typeErr.Reason)
}

// TestAutoDecomposeEpic_ActiveCooldownGate validates that an active cooldown blocks
// the decomposer from being dispatched.
func TestAutoDecomposeEpic_ActiveCooldownGate(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:          "ddx-active-cooldown",
		Title:       "epic: active cooldown",
		IssueType:   "epic",
		Description: "PROBLEM\nTest.\n\nROOT CAUSE\nTest.\n\nPROPOSED FIX\nTest.\n",
		Acceptance:  "1. Test passes\n",
		Labels:      []string{"spec:FEAT-010"},
	}
	require.NoError(t, store.Create(context.

		// Set an active cooldown
		Background(), b))

	_ = store.SetExecutionCooldown(
		b.ID,
		time.Now().UTC().Add(5*time.Minute),
		"test-cooldown",
		"test reason",
		"",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	decomposeRunner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		t.Fatalf("decomposer should not be called when cooldown is active")
		return nil, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := AutoDecomposeEpic(ctx, store, decomposeRunner, rcfg, t.TempDir(), b.ID)

	assert.Error(t, err)
	var typeErr *AutoDecomposeEpicError
	require.ErrorAs(t, err, &typeErr)
	assert.Equal(t, "active_cooldown", typeErr.Reason)
}
