package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAlreadySatisfiedRequiresAcStructuralCheck covers the four AC items for
// the gate-tightening fix: when a bead's AC names a structural property
// (test function, deleted file, removed struct field), already_satisfied
// must be refused unless that property actually holds.
func TestAlreadySatisfiedRequiresAcStructuralCheck(t *testing.T) {
	t.Run("ac_names_test_not_exercised_is_not_already_satisfied", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init())

		b := &bead.Bead{
			ID:         "ddx-act01",
			Title:      "Bead naming a test in AC",
			Acceptance: "go test ./internal/agent/... -run TestStructuralPropertyHolds passes",
		}
		require.NoError(t, store.Create(b))

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
				// Rationale cites a commit SHA (specific) but does NOT cite
				// the AC-named test, so the structural gate must refuse.
				return ExecuteBeadReport{
					BeadID:             id,
					Status:             ExecuteBeadStatusNoChanges,
					NoChangesRationale: "work already present in commit 1da6495",
				}, nil
			}),
		}

		rcfg := loopConfigForACTest(t)
		runtime := ExecuteBeadLoopRuntime{Once: true, ProjectRoot: t.TempDir()}

		result, err := worker.Run(context.Background(), rcfg, runtime)
		require.NoError(t, err)
		require.Len(t, result.Results, 1)
		assert.NotEqual(t, ExecuteBeadStatusAlreadySatisfied, result.Results[0].Status,
			"AC names TestStructuralPropertyHolds but rationale does not cite it; must not close as already_satisfied")
		assert.Equal(t, ExecuteBeadStatusNoChanges, result.Results[0].Status)

		got, err := store.Get(b.ID)
		require.NoError(t, err)
		assert.Equal(t, "open", got.Status, "bead must remain open when AC structural check fails")
	})

	t.Run("ac_names_deleted_file_still_exists_is_not_already_satisfied", func(t *testing.T) {
		projectRoot := t.TempDir()
		// Create the file the AC asserts has been deleted.
		stalePath := filepath.Join(projectRoot, "cmd", "agent", "routing_provider.go")
		require.NoError(t, os.MkdirAll(filepath.Dir(stalePath), 0o755))
		require.NoError(t, os.WriteFile(stalePath, []byte("package agent\n"), 0o644))

		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init())

		b := &bead.Bead{
			ID:         "ddx-act02",
			Title:      "Bead asserting file deletion",
			Acceptance: "cmd/agent/routing_provider.go is deleted",
		}
		require.NoError(t, store.Create(b))

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{
					BeadID:             id,
					Status:             ExecuteBeadStatusNoChanges,
					NoChangesRationale: "work already present in commit 1da6495",
				}, nil
			}),
		}

		rcfg := loopConfigForACTest(t)
		runtime := ExecuteBeadLoopRuntime{Once: true, ProjectRoot: projectRoot}

		result, err := worker.Run(context.Background(), rcfg, runtime)
		require.NoError(t, err)
		require.Len(t, result.Results, 1)
		assert.NotEqual(t, ExecuteBeadStatusAlreadySatisfied, result.Results[0].Status,
			"AC asserts file deleted but it still exists; must not close as already_satisfied")

		got, err := store.Get(b.ID)
		require.NoError(t, err)
		assert.Equal(t, "open", got.Status)
	})

	t.Run("ac_asserts_field_removed_field_still_exists_is_not_already_satisfied", func(t *testing.T) {
		projectRoot := t.TempDir()
		// Create a Go file declaring the field the AC says is removed.
		src := "package x\n\ntype ProviderEntry struct {\n\tName          string\n\tSupportsTools bool\n}\n"
		require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "p.go"), []byte(src), 0o644))

		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init())

		b := &bead.Bead{
			ID:         "ddx-act03",
			Title:      "Bead asserting struct field removed",
			Acceptance: "ProviderEntry no longer has field SupportsTools",
		}
		require.NoError(t, store.Create(b))

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{
					BeadID:             id,
					Status:             ExecuteBeadStatusNoChanges,
					NoChangesRationale: "work already present in commit 1da6495",
				}, nil
			}),
		}

		rcfg := loopConfigForACTest(t)
		runtime := ExecuteBeadLoopRuntime{Once: true, ProjectRoot: projectRoot}

		result, err := worker.Run(context.Background(), rcfg, runtime)
		require.NoError(t, err)
		require.Len(t, result.Results, 1)
		assert.NotEqual(t, ExecuteBeadStatusAlreadySatisfied, result.Results[0].Status,
			"AC says field removed but it is still declared; must not close as already_satisfied")

		got, err := store.Get(b.ID)
		require.NoError(t, err)
		assert.Equal(t, "open", got.Status)
	})

	t.Run("positive_test_cited_in_rationale_and_property_holds_is_already_satisfied", func(t *testing.T) {
		projectRoot := t.TempDir() // empty worktree — no stale file, no offending field
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init())

		b := &bead.Bead{
			ID:         "ddx-act04",
			Title:      "Bead with test cited in rationale",
			Acceptance: "go test ./internal/agent/... -run TestStructuralPropertyHolds passes",
		}
		require.NoError(t, store.Create(b))

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{
					BeadID:             id,
					Status:             ExecuteBeadStatusNoChanges,
					NoChangesRationale: "verification_command: go test ./internal/agent/... -run TestStructuralPropertyHolds\noutput: PASS",
				}, nil
			}),
			VerificationRunner: func(ctx context.Context, projectRoot, command string) (int, string, error) {
				return 0, "PASS", nil
			},
		}

		rcfg := loopConfigForACTest(t)
		runtime := ExecuteBeadLoopRuntime{Once: true, ProjectRoot: projectRoot}

		result, err := worker.Run(context.Background(), rcfg, runtime)
		require.NoError(t, err)
		require.Len(t, result.Results, 1)
		assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, result.Results[0].Status,
			"positive case: verification_command exits 0 → already_satisfied")

		got, err := store.Get(b.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusClosed, got.Status)
	})
}

// TestParseACClaims spot-checks the AC-claim parser for the three claim
// kinds the verifier supports.
func TestParseACClaims(t *testing.T) {
	cases := []struct {
		name   string
		ac     string
		expect []ACClaim
	}{
		{
			name: "test name extracted",
			ac:   "go test ./... -run TestFoo passes",
			expect: []ACClaim{
				{Kind: ACClaimTestName, Value: "TestFoo"},
			},
		},
		{
			name: "deleted file path extracted",
			ac:   "cmd/agent/routing_provider.go is deleted",
			expect: []ACClaim{
				{Kind: ACClaimFileDeleted, Value: "cmd/agent/routing_provider.go"},
			},
		},
		{
			name: "removed field extracted",
			ac:   "ProviderEntry no longer has field SupportsTools",
			expect: []ACClaim{
				{Kind: ACClaimFieldRemoved, Value: "SupportsTools"},
			},
		},
		{
			name:   "empty acceptance",
			ac:     "",
			expect: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseACClaims(tc.ac)
			assert.Equal(t, tc.expect, got)
		})
	}
}

func loopConfigForACTest(t *testing.T) config.ResolvedConfig {
	t.Helper()
	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                "ac-worker",
		MaxNoChangesBeforeClose: 3,
	}
	return config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
}
