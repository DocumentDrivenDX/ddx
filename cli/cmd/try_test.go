package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTry_RegisteredAtRoot verifies the `ddx try` command is wired into the
// cobra root and exposes a help screen — covers AC #7 (wired-in registration).
func TestTry_RegisteredAtRoot(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	tryCmd, _, err := root.Find([]string{"try"})
	require.NoError(t, err, "ddx try must exist")
	require.NotNil(t, tryCmd)
	assert.Equal(t, "try <bead-id>", tryCmd.Use)
}

// TestTry_BeadNotFound covers AC #2: an unknown bead id exits non-zero with
// `bead not found: <id>` on stderr.
func TestTry_BeadNotFound(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ddx"), 0o755))
	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init())

	root := NewCommandFactory(dir).NewRootCommand()
	out, err := executeCommand(root, "try", "unknown-bead-id")
	require.Error(t, err)
	assert.Contains(t, out, "bead not found: unknown-bead-id")

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, ExitCodeGeneralError, exitErr.Code)
}

// TestTry_BeadClosed covers AC #3: a closed bead exits non-zero with
// `bead is not claimable (status=closed)`.
func TestTry_BeadClosed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ddx"), 0o755))
	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:     "ddx-closed-1",
		Title:  "Closed bead",
		Status: bead.StatusClosed,
	}))

	root := NewCommandFactory(dir).NewRootCommand()
	out, err := executeCommand(root, "try", "ddx-closed-1")
	require.Error(t, err)
	assert.Contains(t, out, "bead is not claimable (status=closed)")
}

// TestTry_BeadCancelled covers AC #3 (status=cancelled variant): a cancelled
// bead exits non-zero with `bead is not claimable (status=cancelled)`.
func TestTry_BeadCancelled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ddx"), 0o755))
	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init())
	// Cancelled requires a transition path; create open then update to cancelled.
	require.NoError(t, store.Create(&bead.Bead{
		ID:     "ddx-cancel-1",
		Title:  "Cancellable bead",
		Status: bead.StatusOpen,
	}))
	require.NoError(t, store.Update("ddx-cancel-1", func(b *bead.Bead) {
		b.Status = bead.StatusCancelled
	}))

	root := NewCommandFactory(dir).NewRootCommand()
	out, err := executeCommand(root, "try", "ddx-cancel-1")
	require.Error(t, err)
	assert.Contains(t, out, "bead is not claimable (status=cancelled)")
}

// TestTry_BeadUnmetDeps covers AC #4: a bead with an open dependency exits
// non-zero with `bead has unmet dependencies: <list>`.
func TestTry_BeadUnmetDeps(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ddx"), 0o755))
	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init())

	require.NoError(t, store.Create(&bead.Bead{
		ID:     "ddx-dep-blocker",
		Title:  "Open dependency",
		Status: bead.StatusOpen,
	}))
	require.NoError(t, store.Create(&bead.Bead{
		ID:     "ddx-dep-target",
		Title:  "Target with unmet dep",
		Status: bead.StatusOpen,
		Dependencies: []bead.Dependency{{
			IssueID:     "ddx-dep-target",
			DependsOnID: "ddx-dep-blocker",
			Type:        "blocks",
		}},
	}))

	root := NewCommandFactory(dir).NewRootCommand()
	out, err := executeCommand(root, "try", "ddx-dep-target")
	require.Error(t, err)
	assert.Contains(t, out, "bead has unmet dependencies: ddx-dep-blocker")
}

// TestTry_FlagsPlumbThrough covers AC #6: the cobra command exposes the
// passthrough routing surface (--harness, --model, --profile, --provider,
// --effort, --opaque-passthrough, --no-review, --review-harness,
// --review-model). Each flag must be wired so the runtime config receives it.
func TestTry_FlagsPlumbThrough(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	tryCmd, _, err := root.Find([]string{"try"})
	require.NoError(t, err)

	for _, name := range []string{
		"harness", "model", "profile", "provider", "effort",
		"opaque-passthrough", "no-review", "review-harness", "review-model",
		"min-power", "max-power", "from", "json",
	} {
		assert.NotNil(t, tryCmd.Flags().Lookup(name),
			"ddx try must expose --%s flag", name)
	}
}

// TestTry_HappyPath_ClaimsAndExecutes covers AC #5 at the runtime layer: with
// TargetBeadID set on ExecuteBeadLoopRuntime, the worker claims and executes
// only the named bead — even when other higher-priority ready beads exist —
// and reports the outcome through ExecuteBeadLoopResult.Results. This is the
// load-bearing assertion for the picker-skipping behavior `ddx try` relies on.
func TestTry_HappyPath_ClaimsAndExecutes(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	// Create two ready beads. Without TargetBeadID, the picker would claim the
	// higher-priority one first; with TargetBeadID set we must claim the named
	// (lower-priority) bead instead.
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-other", Title: "other", Priority: 4}))
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-target", Title: "target", Priority: 0}))

	executed := []string{}
	worker := &agent.ExecuteBeadWorker{
		Store: store,
		Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return agent.ExecuteBeadReport{
				BeadID:    beadID,
				Status:    agent.ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-try",
				ResultRev: "deadbeef",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "tryworker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, agent.ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-target",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Assert: only the named bead was attempted and the outcome was reported.
	assert.Equal(t, []string{"ddx-target"}, executed)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	require.Len(t, result.Results, 1)
	assert.Equal(t, "ddx-target", result.Results[0].BeadID)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, result.Results[0].Status)

	// Assert: the targeted bead was actually claimed (closed by the success
	// path) and the non-targeted bead remains open and untouched.
	target, err := store.Get("ddx-target")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, target.Status)

	other, err := store.Get("ddx-other")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, other.Status)
}

// TestTry_TargetBeadNotInReadySet asserts that when TargetBeadID is set but
// the named bead is filtered out of ReadyExecution (e.g. because it's
// unrecognised, on cooldown, or transitioned), the loop reports
// no_ready_work rather than picking some other bead.
func TestTry_TargetBeadNotInReadySet(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-decoy", Title: "decoy", Priority: 4}))

	worker := &agent.ExecuteBeadWorker{
		Store: store,
		Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			t.Fatalf("executor must not be called when TargetBeadID is filtered out; got %s", beadID)
			return agent.ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "tryworker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, agent.ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-not-in-ready",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.NoReadyWork)
	assert.Equal(t, 0, result.Attempts)

	// And the decoy bead must remain untouched — no picker spillover.
	decoy, err := store.Get("ddx-decoy")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, decoy.Status)
}

// TestPreflightTryBead_Direct exercises the preflight gate function directly.
// Stable stderr-message contract is part of the public surface scripts and
// operators pattern-match against; this test pins the messages.
func TestPreflightTryBead_Direct(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ddx"), 0o755))
	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init())

	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-blocker-a", Status: bead.StatusOpen, Title: "a"}))
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-blocker-b", Status: bead.StatusOpen, Title: "b"}))
	require.NoError(t, store.Create(&bead.Bead{
		ID:     "ddx-multi-dep",
		Title:  "multi-dep",
		Status: bead.StatusOpen,
		Dependencies: []bead.Dependency{
			{IssueID: "ddx-multi-dep", DependsOnID: "ddx-blocker-a", Type: "blocks"},
			{IssueID: "ddx-multi-dep", DependsOnID: "ddx-blocker-b", Type: "blocks"},
		},
	}))
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-ready", Title: "ready", Status: bead.StatusOpen}))

	t.Run("not_found", func(t *testing.T) {
		msg := preflightTryBead(store, "ddx-missing")
		assert.Equal(t, "bead not found: ddx-missing", msg)
	})

	t.Run("multiple_unmet_deps_listed_sorted", func(t *testing.T) {
		msg := preflightTryBead(store, "ddx-multi-dep")
		assert.True(t, strings.HasPrefix(msg, "bead has unmet dependencies: "), "unexpected msg: %s", msg)
		assert.Contains(t, msg, "ddx-blocker-a")
		assert.Contains(t, msg, "ddx-blocker-b")
		// Stable sort: a < b lexicographically.
		assert.True(t, strings.Index(msg, "ddx-blocker-a") < strings.Index(msg, "ddx-blocker-b"),
			"unmet deps must be sorted; got %s", msg)
	})

	t.Run("ready_passes", func(t *testing.T) {
		assert.Empty(t, preflightTryBead(store, "ddx-ready"))
	})
}
