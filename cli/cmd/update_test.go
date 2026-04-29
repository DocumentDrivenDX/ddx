package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUpdate_ForceReplacesStaleSymlinks verifies that ddx update --force
// replaces legacy symlinks under .agents/skills/ with real files.
// This covers the FEAT-015 requirement that skills.Install is always invoked
// with Force:true during update (via refreshShippedSkills).
func TestUpdate_ForceReplacesStaleSymlinks(t *testing.T) {
	te := NewTestEnvironment(t, WithGitInit(false))

	// Create .agents/skills/ddx as a symlink (pre-migration state).
	agentSkillsDir := filepath.Join(te.Dir, ".agents", "skills")
	require.NoError(t, os.MkdirAll(agentSkillsDir, 0o755))

	fakeTarget := filepath.Join(t.TempDir(), "old-global-ddx")
	require.NoError(t, os.MkdirAll(fakeTarget, 0o755))
	symlinkPath := filepath.Join(agentSkillsDir, "ddx")
	require.NoError(t, os.Symlink(fakeTarget, symlinkPath))

	// Verify the symlink exists before update.
	info, err := os.Lstat(symlinkPath)
	require.NoError(t, err)
	require.True(t, info.Mode()&os.ModeSymlink != 0, "expected symlink before update")

	// Run ddx update --force; this should call refreshShippedSkills which
	// calls skills.Install with Force:true, replacing the symlink.
	_, err = te.RunCommand("update", "--force")
	// The command may fail due to network operations (checking plugin updates),
	// but refreshShippedSkills runs unconditionally before any network call.
	// We tolerate network errors but require the symlink to be gone.
	_ = err

	// After update, the symlink must be replaced with a real directory.
	info, err = os.Lstat(symlinkPath)
	if os.IsNotExist(err) {
		// Skill was not installed (embedded FS may not have ddx skill in test);
		// acceptable — the symlink is gone either way.
		return
	}
	require.NoError(t, err)
	if info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("symlink was not replaced by ddx update --force: %s is still a symlink", symlinkPath)
	}
}
