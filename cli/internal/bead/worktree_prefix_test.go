package bead

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

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
func TestGenID_WorktreePrefix(t *testing.T) {
	// Build a project directory named "ddx" — the canonical project name.
	tmpRoot := t.TempDir()
	projectDir := filepath.Join(tmpRoot, "ddx")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	// NewStore receives an absolute path rooted at the real project, not the
	// worktree.  workingDir inside NewStore will be projectDir.
	ddxDir := filepath.Join(projectDir, ".ddx")
	s := NewStore(ddxDir)
	require.NoError(t, s.Init())

	id, err := s.GenID()
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

	prefix := detectPrefix(projectDir)
	assert.Equal(t, "ddx", prefix,
		"detectPrefix should return the project dir name, not the worktree cwd")
}
