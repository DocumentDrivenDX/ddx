package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecompositionDepthPolicyIsUnified proves that preclaim intake, post-attempt
// recovery, the queue-drain path, and executeBeadDynamicStep0Hints all resolve
// the same depth cap from one policy for unset, zero, and explicit
// agent.triage.max_decomposition_depth values — and that under the default
// policy no path creates a third generated child layer.
func TestDecompositionDepthPolicyIsUnified(t *testing.T) {
	t.Parallel()

	// zero: non-positive pointer resolves to the binary default. Schema minimum
	// is 1, so zero is not loadable from YAML; exercise the resolver branch
	// directly, then confirm workDir without the field matches that default.
	zero := 0
	zeroCfg := &config.NewConfig{
		Agent: &config.AgentConfig{
			Triage: &config.TriageConfig{MaxDecompositionDepth: &zero},
		},
	}
	zeroResolved := zeroCfg.Resolve(config.CLIOverrides{})
	assert.Equal(t, config.DefaultMaxDecompositionDepth, zeroResolved.MaxDecompositionDepth(),
		"zero max_decomposition_depth must resolve to DefaultMaxDecompositionDepth")
	assert.Equal(t, zeroResolved.MaxDecompositionDepth(), decompositionDepthCap(zeroResolved),
		"decompositionDepthCap must match MaxDecompositionDepth for zero policy")
	assert.Equal(t, zeroResolved.MaxDecompositionDepth(), zeroResolved.DecompositionPolicy().MaxDepth,
		"DecompositionPolicy.MaxDepth must match MaxDecompositionDepth for zero policy")
	assert.LessOrEqual(t, decompositionDepthCap(zeroResolved), 2,
		"default/zero policy must be at most two generated child levels")

	cases := []struct {
		name            string
		yaml            string // empty means no config file (unset)
		wantExact       int    // when non-zero, resolved depth must equal this
		assertAtMostTwo bool
	}{
		{
			name:            "unset",
			yaml:            "",
			assertAtMostTwo: true,
		},
		{
			// workDir has no explicit field; resolver-side zero is covered above.
			name:            "zero",
			yaml:            "",
			wantExact:       zeroResolved.MaxDecompositionDepth(),
			assertAtMostTwo: true,
		},
		{
			name: "explicit_3",
			yaml: `version: "1.0"
agent:
  triage:
    max_decomposition_depth: 3
`,
			wantExact: 3,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run("resolve_"+tc.name, func(t *testing.T) {
			t.Parallel()

			workDir := t.TempDir()
			if tc.yaml != "" {
				ddxDir := filepath.Join(workDir, ddxroot.DirName)
				require.NoError(t, os.MkdirAll(ddxDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(tc.yaml), 0o644))
			}

			rcfg, err := config.LoadAndResolve(workDir, config.CLIOverrides{})
			if tc.yaml != "" {
				require.NoError(t, err)
			}
			want := rcfg.MaxDecompositionDepth()
			require.Greater(t, want, 0, "resolved max depth must be positive")
			if tc.assertAtMostTwo {
				assert.LessOrEqual(t, want, 2,
					"default/unset policy must be at most two generated child levels")
				assert.Equal(t, config.DefaultMaxDecompositionDepth, want,
					"unset/zero must resolve to DefaultMaxDecompositionDepth")
			}
			if tc.wantExact > 0 {
				assert.Equal(t, tc.wantExact, want,
					"resolved depth must match expected policy value")
			}

			// All accessors and the agent helper must agree for this config.
			assert.Equal(t, want, rcfg.DecompositionPolicy().MaxDepth,
				"DecompositionPolicy.MaxDepth must equal MaxDecompositionDepth")
			assert.Equal(t, want, decompositionDepthCap(rcfg),
				"decompositionDepthCap must equal MaxDecompositionDepth")
			assert.Equal(t, want, resolveMaxDecompositionDepthForPrompt(workDir),
				"prompt helper must equal MaxDecompositionDepth")

			b := &bead.Bead{ID: "ddx-depth-policy-test", Title: "depth policy prompt"}
			hints := executeBeadDynamicStep0Hints(workDir, b)
			require.NotEmpty(t, hints, "dynamic step0 hints must include the depth policy")

			needle := fmt.Sprintf("Auto-decomposition is capped at depth %d", want)
			assert.Contains(t, hints, needle,
				"executeBeadDynamicStep0Hints must render the unified policy depth=%d", want)
			assert.Contains(t, hints, "agent.triage.max_decomposition_depth")

			// Guard against reintroducing an independent hard-coded cap.
			for other := 1; other <= 10; other++ {
				if other == want {
					continue
				}
				bad := fmt.Sprintf("Auto-decomposition is capped at depth %d", other)
				assert.NotContains(t, hints, bad,
					"prompt must not state a different hard-coded depth cap")
			}
		})
	}

	// AC2: under the default unset policy, a bead already at the default cap
	// yields no new descendants from preclaim intake, post-attempt recovery,
	// or the queue-drain path (handlePostAttemptDecomposition).
	t.Run("default_at_cap_no_third_level", func(t *testing.T) {
		t.Parallel()

		// Shared actionable child template for post-attempt / mid paths that
		// must refuse to create another layer at depth == DefaultMaxDecompositionDepth.
		materialChildren := func() *PreClaimDecomposition {
			return &PreClaimDecomposition{
				Rationale: "would create a third generated layer",
				Children: []PreClaimDecompositionChild{
					{Title: "Subtask A", Description: "part A only", Acceptance: "1. implement part A of the work"},
					{Title: "Subtask B", Description: "part B only", Acceptance: "1. implement part B of the work"},
				},
				ACMap: []ACMapEntry{
					{ParentAC: "1. implement part A of the work", Coverage: "covered by Subtask A"},
					{ParentAC: "2. implement part B of the work", Coverage: "covered by Subtask B"},
				},
			}
		}

		// Default / unset policy via zero MaxDecompositionDepth in test opts.
		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		require.Equal(t, config.DefaultMaxDecompositionDepth, decompositionDepthCap(rcfg),
			"test loop default must use DefaultMaxDecompositionDepth")
		require.Equal(t, 2, decompositionDepthCap(rcfg),
			"default policy must be two generated levels")

		// At-cap hierarchy: root -> decomposed child -> decomposed grandchild (depth 2).
		seedAtCap := func(t *testing.T, store *bead.Store, idSuffix string, acceptance, description string) *bead.Bead {
			t.Helper()
			root := &bead.Bead{ID: "ddx-cap-root-" + idSuffix, Title: "Root", Status: bead.StatusClosed}
			require.NoError(t, store.Create(context.Background(), root))
			parent := &bead.Bead{
				ID:     "ddx-cap-parent-" + idSuffix,
				Title:  "Parent depth 1",
				Parent: root.ID,
				Status: bead.StatusClosed,
				Labels: []string{"decomposed"},
			}
			require.NoError(t, store.Create(context.Background(), parent))
			candidate := &bead.Bead{
				ID:          "ddx-cap-bead-" + idSuffix,
				Title:       "At default decomposition cap",
				Parent:      parent.ID,
				Labels:      []string{"decomposed"},
				Description: description,
				Acceptance:  acceptance,
			}
			require.NoError(t, store.Create(context.Background(), candidate))
			require.Equal(t, 2, storeBeadDepth(context.Background(), store, candidate),
				"fixture must sit at default cap depth 2")
			return candidate
		}

		assertNoDescendants := func(t *testing.T, store *bead.Store, parentID string) {
			t.Helper()
			all, err := store.ReadAll(context.Background())
			require.NoError(t, err)
			for _, b := range all {
				assert.NotEqual(t, parentID, b.Parent,
					"no path may create a third generated child under %s", parentID)
			}
		}

		t.Run("preclaim_intake", func(t *testing.T) {
			// Early preclaim depth gate (execute_bead_loop preclaim intake).
			store := bead.NewStore(t.TempDir())
			require.NoError(t, store.Init(context.Background()))
			candidate := seedAtCap(t, store, "preclaim",
				"1. implement part A of the work\n2. implement part B of the work",
				"PROBLEM\nat cap\n\nPROPOSED FIX\ndispatch without split\n\nNON-SCOPE\nnone",
			)

			var intakeCalls int32
			var decompHookCalls int32
			worker := &ExecuteBeadWorker{
				Store: store,
				Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
					return ExecuteBeadReport{
						BeadID:    beadID,
						Status:    ExecuteBeadStatusSuccess,
						SessionID: "sess-atcap-preclaim",
						ResultRev: "abc123",
					}, nil
				}),
			}
			_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
				Once:         true,
				TargetBeadID: candidate.ID,
				PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
					atomic.AddInt32(&intakeCalls, 1)
					// If intake ran, return a split that would create a third layer.
					return PreClaimIntakeResult{
						Outcome:       PreClaimIntakeTooLargeDecomposed,
						Detail:        "should not run at default cap",
						Decomposition: materialChildren(),
					}, nil
				},
				PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
					atomic.AddInt32(&decompHookCalls, 1)
					return materialChildren(), nil
				},
			})
			require.NoError(t, err)
			assert.Equal(t, int32(0), atomic.LoadInt32(&intakeCalls),
				"preclaim intake must not run when already at default depth cap")
			assert.Equal(t, int32(0), atomic.LoadInt32(&decompHookCalls),
				"post-attempt decomp hook must not run for successful at-cap dispatch")
			assertNoDescendants(t, store, candidate.ID)
		})

		t.Run("post_attempt_recovery", func(t *testing.T) {
			// Mid-attempt no_changes with orchestrator_action:decompose — the
			// post-attempt recovery / queue-drain path (handlePostAttemptDecomposition).
			store := bead.NewStore(t.TempDir())
			require.NoError(t, store.Init(context.Background()))
			// Non-actionable acceptance so early preclaim is not required;
			// no PreClaimIntakeHook so the attempt runs and post-attempt fires.
			candidate := seedAtCap(t, store, "post",
				"1. implement part A of the work\n2. implement part B of the work",
				"PROBLEM\nat cap\n\nPROPOSED FIX\nstill too large\n\nNON-SCOPE\nnone",
			)

			var decompHookCalls int32
			worker := &ExecuteBeadWorker{
				Store: store,
				Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
					return ExecuteBeadReport{
						BeadID:             beadID,
						Status:             ExecuteBeadStatusNoChanges,
						NoChangesRationale: "status: open\norchestrator_action: decompose\nreason: still too large at depth cap",
					}, nil
				}),
			}
			result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
				Once:         true,
				TargetBeadID: candidate.ID,
				PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
					atomic.AddInt32(&decompHookCalls, 1)
					return materialChildren(), nil
				},
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			// Hook must not run: depth check refuses before invoking the splitter.
			assert.Equal(t, int32(0), atomic.LoadInt32(&decompHookCalls),
				"post-attempt splitter must not run when bead is already at default depth cap")
			assertNoDescendants(t, store, candidate.ID)
		})

		t.Run("queue_drain_path", func(t *testing.T) {
			// Same handlePostAttemptDecomposition path exercised without a
			// targeted bead ID (queue-drain style once-shot claim of the only ready bead).
			store := bead.NewStore(t.TempDir())
			require.NoError(t, store.Init(context.Background()))
			candidate := seedAtCap(t, store, "drain",
				"1. implement part A of the work\n2. implement part B of the work",
				"PROBLEM\nat cap\n\nPROPOSED FIX\nstill too large\n\nNON-SCOPE\nnone",
			)

			var decompHookCalls int32
			worker := &ExecuteBeadWorker{
				Store: store,
				Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
					return ExecuteBeadReport{
						BeadID:             beadID,
						Status:             ExecuteBeadStatusNoChanges,
						NoChangesRationale: "status: open\norchestrator_action: decompose\nreason: queue drain at depth cap",
					}, nil
				}),
			}
			result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
				Once: true,
				// No TargetBeadID: drain-style pick of the ready candidate.
				PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
					atomic.AddInt32(&decompHookCalls, 1)
					return materialChildren(), nil
				},
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, int32(0), atomic.LoadInt32(&decompHookCalls),
				"queue drain path must not invoke splitter when already at default depth cap")
			assertNoDescendants(t, store, candidate.ID)
		})
	})

	// AC4: explicit max_decomposition_depth=3 is honored consistently (not
	// treated as an error, not silently clamped) by prompt text and all
	// orchestrator accessors.
	t.Run("explicit_3_consistent_not_clamped", func(t *testing.T) {
		t.Parallel()

		workDir := t.TempDir()
		ddxDir := filepath.Join(workDir, ddxroot.DirName)
		require.NoError(t, os.MkdirAll(ddxDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
agent:
  triage:
    max_decomposition_depth: 3
`), 0o644))

		rcfg, err := config.LoadAndResolve(workDir, config.CLIOverrides{})
		require.NoError(t, err)
		assert.Equal(t, 3, rcfg.MaxDecompositionDepth())
		assert.Equal(t, 3, rcfg.DecompositionPolicy().MaxDepth)
		assert.Equal(t, 3, decompositionDepthCap(rcfg))
		assert.Equal(t, 3, resolveMaxDecompositionDepthForPrompt(workDir))

		hints := executeBeadDynamicStep0Hints(workDir, &bead.Bead{ID: "ddx-explicit3", Title: "explicit 3"})
		assert.Contains(t, hints, "Auto-decomposition is capped at depth 3")
		assert.NotContains(t, hints, "Auto-decomposition is capped at depth 2")
		assert.False(t, strings.Contains(hints, "error") && strings.Contains(hints, "max_decomposition_depth"),
			"explicit 3 must not be reported as a configuration error")

		// Loop test constructor with MaxDecompositionDepth: 3 must also land on 3.
		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", MaxDecompositionDepth: 3}
		loopRcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		assert.Equal(t, 3, decompositionDepthCap(loopRcfg),
			"explicit 3 via test loop opts must not be clamped")
	})
}

// TestDecompositionDepthPolicyPromptUsesResolvedConfig remains as a focused
// prompt-only regression; unified multi-path coverage lives in
// TestDecompositionDepthPolicyIsUnified.
func TestDecompositionDepthPolicyPromptUsesResolvedConfig(t *testing.T) {
	t.Parallel()

	zero := 0
	zeroCfg := &config.NewConfig{
		Agent: &config.AgentConfig{
			Triage: &config.TriageConfig{MaxDecompositionDepth: &zero},
		},
	}
	zeroResolved := zeroCfg.Resolve(config.CLIOverrides{}).MaxDecompositionDepth()
	assert.Equal(t, config.DefaultMaxDecompositionDepth, zeroResolved,
		"zero max_decomposition_depth must resolve to DefaultMaxDecompositionDepth")
	assert.LessOrEqual(t, zeroResolved, 2,
		"default/zero policy must be at most two generated child levels")

	cases := []struct {
		name            string
		yaml            string
		wantExact       int
		assertAtMostTwo bool
	}{
		{name: "unset", yaml: "", assertAtMostTwo: true},
		{name: "zero", yaml: "", wantExact: zeroResolved, assertAtMostTwo: true},
		{
			name: "explicit",
			yaml: `version: "1.0"
agent:
  triage:
    max_decomposition_depth: 5
`,
			wantExact: 5,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workDir := t.TempDir()
			if tc.yaml != "" {
				ddxDir := filepath.Join(workDir, ddxroot.DirName)
				require.NoError(t, os.MkdirAll(ddxDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(tc.yaml), 0o644))
			}

			rcfg, err := config.LoadAndResolve(workDir, config.CLIOverrides{})
			if tc.yaml != "" {
				require.NoError(t, err)
			}
			want := rcfg.MaxDecompositionDepth()
			require.Greater(t, want, 0, "resolved max depth must be positive")
			if tc.assertAtMostTwo {
				assert.LessOrEqual(t, want, 2,
					"default/unset policy must be at most two generated child levels")
				assert.Equal(t, config.DefaultMaxDecompositionDepth, want,
					"unset/zero must resolve to DefaultMaxDecompositionDepth")
			}
			if tc.wantExact > 0 {
				assert.Equal(t, tc.wantExact, want,
					"resolved depth must match expected policy value")
			}

			b := &bead.Bead{ID: "ddx-depth-policy-test", Title: "depth policy prompt"}
			hints := executeBeadDynamicStep0Hints(workDir, b)
			require.NotEmpty(t, hints, "dynamic step0 hints must include the depth policy")

			needle := fmt.Sprintf("Auto-decomposition is capped at depth %d", want)
			assert.Contains(t, hints, needle,
				"executeBeadDynamicStep0Hints must render ResolvedConfig.MaxDecompositionDepth()=%d", want)
			assert.Contains(t, hints, "agent.triage.max_decomposition_depth")

			for other := 1; other <= 10; other++ {
				if other == want {
					continue
				}
				bad := fmt.Sprintf("Auto-decomposition is capped at depth %d", other)
				assert.NotContains(t, hints, bad,
					"prompt must not state a different hard-coded depth cap")
			}

			assert.Equal(t, want, resolveMaxDecompositionDepthForPrompt(workDir))
			assert.Equal(t, want, decompositionDepthCap(rcfg))
		})
	}
}
