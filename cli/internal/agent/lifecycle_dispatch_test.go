package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewLifecycleScratchDirSeedsFromHEAD verifies the lifecycle readiness
// scratch worktree is seeded with the project's HEAD source instead of an empty
// repo. An empty scratch made the readiness-check classifier block every bead
// with "target file not found in working directory" (ddx-efadca32).
func TestNewLifecycleScratchDirSeedsFromHEAD(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)

	dir, err := newLifecycleScratchDir(projectRoot)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	// The file committed at HEAD must be present in the scratch worktree —
	// the bug left only an empty .git here.
	seed := filepath.Join(dir, "seed.txt")
	require.FileExists(t, seed)
	content, err := os.ReadFile(seed)
	require.NoError(t, err)
	assert.Equal(t, "seed\n", string(content))
}

func TestParseLifecycleProjectStatusIgnoresDDxRuntimePaths(t *testing.T) {
	raw := "" +
		" M .ddx/beads.jsonl\n" +
		" M .ddx/beads-archive.jsonl\n" +
		" M .ddx/metrics/attempts.jsonl\n" +
		" M .ddx/metrics/locks.jsonl\n" +
		"?? .ddx/attachments/ddx-example/events.jsonl\n" +
		"?? .ddx/dirty-root-guard.json\n" +
		"?? .ddx/run-state.json\n" +
		"?? .ddx/run-state/attempt.json\n" +
		"?? .ddx/workers/worker-1/status.json\n" +
		" M cli/internal/agent/lifecycle_dispatch.go\n"

	got := parseLifecycleProjectStatus(raw)

	assert.Equal(t, map[string]string{
		"cli/internal/agent/lifecycle_dispatch.go": " M",
	}, got)
}

func TestLifecycleProjectStatusDiffIgnoresOnlyDDxRuntimePaths(t *testing.T) {
	before := parseLifecycleProjectStatus(" M cli/internal/agent/foo.go\n")
	after := parseLifecycleProjectStatus("" +
		" M .ddx/beads.jsonl\n" +
		" M .ddx/dirty-root-guard.json\n" +
		" M cli/internal/agent/foo.go\n")

	newPaths, changedPaths := diffLifecycleProjectStatus(before, after)

	assert.Empty(t, newPaths)
	assert.Empty(t, changedPaths)
}

func TestLifecycleProjectStatusDiffStillRejectsSourceMutations(t *testing.T) {
	before := parseLifecycleProjectStatus("")
	after := parseLifecycleProjectStatus(" M cli/internal/agent/foo.go\n")

	newPaths, changedPaths := diffLifecycleProjectStatus(before, after)

	assert.Equal(t, []string{"cli/internal/agent/foo.go"}, newPaths)
	assert.Empty(t, changedPaths)
}
