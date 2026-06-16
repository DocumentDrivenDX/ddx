package bead

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenID_WorktreePrefix verifies that when NewStore is given an absolute
// .ddx path rooted in the real project (e.g. "/path/to/ddx/.ddx"), the
// generated bead IDs use the real project directory name as a prefix — even
// when the process working directory is an execute-bead linked worktree whose
// name would otherwise contaminate the prefix.
//
// Regression test for: bead-id resolver uses cwd inside worktree
// (ddx-7eab13a6).
//
// To exercise the bug, this test chdirs into a worktree-shaped non-git
// directory before calling NewStore. Pre-fix, detectPrefix() ran git from
// cwd and (on git failure) fell back to filepath.Base(cwd), producing a
// ".execute-bead-wt-…" prefix. Post-fix, detectPrefix runs git from the
// supplied workingDir and falls back to filepath.Base(workingDir).
func TestGenID_WorktreePrefix(t *testing.T) {
	// Real project dir (the path NewStore is told about).
	tmpRoot := t.TempDir()
	projectDir := filepath.Join(tmpRoot, "ddx")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	// Worktree-shaped cwd, distinct from projectDir, NOT inside any git repo.
	// This mirrors the real failure: the agent's cwd is the linked worktree
	// directory while it invokes `ddx bead create`.
	cwdRoot := t.TempDir()
	worktreeCwd := filepath.Join(cwdRoot, ".execute-bead-wt-ddx-7eab13a6-20260430T005604-c425377f")
	require.NoError(t, os.MkdirAll(worktreeCwd, 0o755))
	t.Chdir(worktreeCwd)

	// NewStore receives an absolute path rooted at the real project, not the
	// worktree.  workingDir inside NewStore will be projectDir.
	ddxDir := filepath.Join(projectDir, ddxroot.DirName)
	s := NewStore(ddxDir)
	require.NoError(t, s.Init(testCtx()))

	id, err := s.GenID(testCtx())
	require.NoError(t, err)

	// Must be "ddx-<8 hex digits>", not ".execute-bead-wt-…-<hex>".
	assert.Regexp(t, regexp.MustCompile(`^ddx-[0-9a-f]{8}$`), id,
		"bead ID must use real project dir name, not worktree path component")
}

// TestDetectPrefix_WorktreeDir verifies that detectPrefix, when given an
// absolute workingDir that is NOT a linked worktree, returns the base name of
// that directory even when the process cwd is a worktree-shaped path.
func TestDetectPrefix_WorktreeDir(t *testing.T) {
	tmpRoot := t.TempDir()
	projectDir := filepath.Join(tmpRoot, "ddx")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	// Chdir into a worktree-shaped non-git directory so that — pre-fix —
	// detectPrefix would have fallen back to filepath.Base(cwd) and produced
	// the worktree path component.
	cwdRoot := t.TempDir()
	worktreeCwd := filepath.Join(cwdRoot, ".execute-bead-wt-ddx-7eab13a6-20260430T005604-c425377f")
	require.NoError(t, os.MkdirAll(worktreeCwd, 0o755))
	t.Chdir(worktreeCwd)

	prefix := detectPrefix(projectDir)
	assert.Equal(t, "ddx", prefix,
		"detectPrefix should return the project dir name, not the worktree cwd")
}

func TestDetectPrefixRejectsInvalidWorktreePrefix(t *testing.T) {
	worktreeDir := filepath.Join(t.TempDir(), ".execute-bead-wt-123")
	require.NoError(t, os.MkdirAll(worktreeDir, 0o755))

	prefix := detectPrefix(worktreeDir)
	require.Equal(t, DefaultPrefix, prefix)
}

func TestDetectPrefixReturnsOnlyValidateIDCompatiblePrefix(t *testing.T) {
	t.Run("accepts validate-id-compatible basename", func(t *testing.T) {
		projectDir := filepath.Join(t.TempDir(), "ddx")
		require.NoError(t, os.MkdirAll(projectDir, 0o755))

		prefix := detectPrefix(projectDir)
		require.Equal(t, "ddx", prefix)
	})

	t.Run("rejects leading dot", func(t *testing.T) {
		worktreeDir := filepath.Join(t.TempDir(), ".execute-bead-wt-123")
		require.NoError(t, os.MkdirAll(worktreeDir, 0o755))

		prefix := detectPrefix(worktreeDir)
		require.Equal(t, DefaultPrefix, prefix)
	})

	t.Run("rejects slash and overlong prefixes", func(t *testing.T) {
		require.Empty(t, validateIDPrefix("foo/bar"))
		require.Empty(t, validateIDPrefix(strings.Repeat("a", 57)))
	})
}
