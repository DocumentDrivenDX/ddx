package agent

import (
	"os"
	"path/filepath"
	"strings"
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

// TestCaptureLifecycleProjectStatusPreservesGitStderr verifies that git status
// failures still surface the underlying git stderr instead of a bare wrapper
// error.
func TestCaptureLifecycleProjectStatusPreservesGitStderr(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	runGitInteg(t, projectRoot, "config", "core.bare", "true")

	_, err := captureLifecycleProjectStatus(projectRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fatal: this operation must be run in a work tree")
	assert.True(t, strings.Contains(err.Error(), "snapshot project root dirtiness"))
}
