package agent

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func indexIsClean(t *testing.T, dir string) bool {
	t.Helper()
	return exec.Command("git", "-C", dir, "diff", "--cached", "--quiet").Run() == nil
}

// TestLandingIndex_UnstagesOrphanedExecutionEvidence guards ddx-2ab14458: a
// landing worktree whose only staged paths are .ddx/executions/* evidence left
// by a dead attempt is recovered by unstaging that evidence (not committing
// it), so it cannot jam the pre-claim / landing-index guard.
func TestLandingIndex_UnstagesOrphanedExecutionEvidence(t *testing.T) {
	r := newLandTestRepo(t)
	r.writeFile(".ddx/executions/20260706T000000-abc/result.json", `{"status":"ok"}`)
	r.writeFile(".ddx/executions/20260706T000000-abc/manifest.json", `{}`)
	// Force-stage: executions are gitignored, so an orphaned staged state only
	// arises via a --force add.
	r.runGit("add", "-f", ".ddx/executions")

	require.False(t, indexIsClean(t, r.dir), "precondition: evidence should be staged")
	require.True(t, unstageOrphanedExecutionEvidence(r.dir), "should unstage orphaned execution evidence")
	require.True(t, indexIsClean(t, r.dir), "index should be clean after unstaging evidence")
}

// TestLandingIndex_RefusesStagedCode guards ddx-2ab14458: real staged work (code,
// or a mixed evidence+code set) must never be silently discarded.
func TestLandingIndex_RefusesStagedCode(t *testing.T) {
	t.Run("code only", func(t *testing.T) {
		r := newLandTestRepo(t)
		r.writeFile("main.go", "package main\n")
		r.runGit("add", "main.go")
		require.False(t, unstageOrphanedExecutionEvidence(r.dir))
		require.False(t, indexIsClean(t, r.dir), "staged code must remain staged")
	})

	t.Run("mixed evidence and code", func(t *testing.T) {
		r := newLandTestRepo(t)
		r.writeFile(".ddx/executions/run/result.json", "{}")
		r.writeFile("code.go", "package main\n")
		r.runGit("add", "-f", ".ddx/executions", "code.go")
		require.False(t, unstageOrphanedExecutionEvidence(r.dir))
		require.False(t, indexIsClean(t, r.dir), "mixed set must remain staged")
	})
}
