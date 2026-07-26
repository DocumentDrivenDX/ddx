package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePersistedManagedWorkerSpec(t *testing.T, projectRoot, workerID string, spec serverpkg.ExecuteLoopWorkerSpec) string {
	t.Helper()
	if strings.TrimSpace(spec.ProjectRoot) == "" {
		spec.ProjectRoot = projectRoot
	}
	spec.ApplyDefaults()
	dir := filepath.Join(ddxroot.JoinProject(projectRoot, "workers"), workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := json.MarshalIndent(spec, "", "  ")
	require.NoError(t, err)
	path := filepath.Join(dir, "spec.json")
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o644))
	return path
}

// TestManagedWorkerServerManagedFlagRunsWorkLoop proves the hidden
// ddx work --server-managed <worker-id> entry point reads the persisted
// ExecuteLoopWorkerSpec and runs the same execute-loop path used by
// server-managed workers (runAgentExecuteLoopImpl / shared work loop).
func TestManagedWorkerServerManagedFlagRunsWorkLoop(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateDefaultConfig()
	store := bead.NewStore(filepath.Join(env.Dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	// Empty queue: the shared work loop still starts and exits with no-ready-work.

	const workerID = "worker-entry-loop"
	writePersistedManagedWorkerSpec(t, env.Dir, workerID, serverpkg.ExecuteLoopWorkerSpec{
		Mode:     executeloop.ModeOnce,
		NoReview: true,
		// Distinctive non-default values so a defaults-only path would not match.
		Harness: "persisted-harness",
		Model:   "persisted-model",
	})

	factory := NewCommandFactory(env.Dir)
	// Avoid real harness dispatch if a bead somehow becomes ready.
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	root := factory.NewRootCommand()

	// Parent launch path: only worker id (and project). Spec must come from disk.
	out, err := executeCommand(root,
		"work",
		"--server-managed", workerID,
		"--project", env.Dir,
		"--allow-non-default-branch",
	)
	require.NoError(t, err, "server-managed entry must run the work loop; out=%q", out)
	combined := strings.ToLower(out)
	// Shared execute-loop terminal summary for empty queue.
	assert.True(t,
		strings.Contains(combined, "no ready") ||
			strings.Contains(combined, "drained") ||
			strings.Contains(combined, "project:"),
		"expected execute-loop terminal output, got: %q", out)
}

// TestManagedWorkerServerManagedFlagRejectsMissingSpec proves the entry point
// exits with an actionable error when the worker id has no persisted
// ExecuteLoopWorkerSpec.
func TestManagedWorkerServerManagedFlagRejectsMissingSpec(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateDefaultConfig()
	// Ensure .ddx exists but no worker dir / spec.json for this id.
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(env.Dir), 0o755))

	const workerID = "worker-missing-spec"
	factory := NewCommandFactory(env.Dir)
	root := factory.NewRootCommand()

	out, err := executeCommand(root,
		"work",
		"--server-managed", workerID,
		"--project", env.Dir,
		"--allow-non-default-branch",
	)
	require.Error(t, err, "missing persisted spec must fail; out=%q", out)
	msg := err.Error() + "\n" + out
	assert.Contains(t, msg, "no persisted ExecuteLoopWorkerSpec")
	assert.Contains(t, msg, workerID)
	assert.Contains(t, msg, "spec.json")
}

// TestManagedWorkerServerManagedFlagUsesPersistedSpec proves the entry point
// uses the persisted worker spec instead of reconstructing execution parameters
// from CLI defaults.
func TestManagedWorkerServerManagedFlagUsesPersistedSpec(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateDefaultConfig()

	const workerID = "worker-persisted-spec"
	want := serverpkg.ExecuteLoopWorkerSpec{
		Mode:        executeloop.ModeOnce,
		NoReview:    true,
		Harness:     "from-disk-harness",
		Model:       "from-disk-model",
		Provider:    "from-disk-provider",
		Profile:     "from-disk-profile",
		LabelFilter: "area:workers",
		MinPower:    7,
		MinPowerSet: true,
		MaxPower:    11,
		ReviewTier:  executeloop.ReviewTierElevated,
	}
	writePersistedManagedWorkerSpec(t, env.Dir, workerID, want)

	factory := NewCommandFactory(env.Dir)
	root := factory.NewRootCommand()
	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err)
	require.NoError(t, workCmd.Flags().Set("server-managed", workerID))
	require.NoError(t, workCmd.Flags().Set("project", env.Dir))
	// Leave harness/model/etc at flag defaults — persisted spec must win.
	// Intentionally set conflicting CLI values that must be ignored.
	require.NoError(t, workCmd.Flags().Set("harness", "cli-default-harness"))
	require.NoError(t, workCmd.Flags().Set("model", "cli-default-model"))
	require.NoError(t, workCmd.Flags().Set("min-power", "1"))

	spec, _, explicitMin, err := factory.resolveExecuteLoopSpec(workCmd, true)
	require.NoError(t, err)

	assert.Equal(t, want.Harness, spec.Harness, "must use harness from persisted spec, not CLI flags")
	assert.Equal(t, want.Model, spec.Model, "must use model from persisted spec, not CLI flags")
	assert.Equal(t, want.Provider, spec.Provider)
	assert.Equal(t, want.Profile, spec.Profile)
	assert.Equal(t, want.LabelFilter, spec.LabelFilter)
	assert.Equal(t, want.MinPower, spec.MinPower)
	assert.True(t, explicitMin, "MinPowerSet from persisted spec must drive explicit min-power")
	assert.Equal(t, want.MaxPower, spec.MaxPower)
	assert.Equal(t, want.ReviewTier, spec.ReviewTier)
	assert.Equal(t, executeloop.ModeOnce, spec.Mode)
	assert.True(t, spec.NoReview)
	assert.True(t, spec.OpaquePassthrough, "server-managed work keeps opaque passthrough")
	assert.NotEqual(t, "cli-default-harness", spec.Harness)
	assert.NotEqual(t, "cli-default-model", spec.Model)

	// Also exercise LoadManagedWorkerSpec directly (server package contract).
	loaded, err := serverpkg.LoadManagedWorkerSpec(env.Dir, workerID)
	require.NoError(t, err)
	assert.Equal(t, want.Harness, loaded.Harness)
	assert.Equal(t, want.Model, loaded.Model)
}
