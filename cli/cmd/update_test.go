package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commitTestFiles stages and commits the listed project-relative paths.
func commitTestFiles(t *testing.T, dir string, relPaths ...string) {
	t.Helper()
	cleanEnv := gitpkg.CleanEnv()
	for _, rel := range relPaths {
		add := exec.Command("git", "add", rel)
		add.Dir = dir
		add.Env = cleanEnv
		require.NoError(t, add.Run(), "git add %s", rel)
	}
	commit := exec.Command("git", "commit", "--allow-empty", "-m", "test: commit update target files")
	commit.Dir = dir
	commit.Env = cleanEnv
	require.NoError(t, commit.Run(), "git commit")
}

// TestUpdate_ForceReplacesStaleSymlinksWithCacheBackedAdapter verifies that
// ddx update --force replaces legacy links under .agents/skills/ with the
// cache-backed built-in DDx adapter.
func TestUpdate_ForceReplacesStaleSymlinksWithCacheBackedAdapter(t *testing.T) {
	homeDir := t.TempDir()
	xdgDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDir)

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

	// Run ddx update --force; this should recreate the baked-in package cache
	// and replace the stale symlink with the generated adapter shim.
	_, err = te.RunCommand("update", "--force")
	// The command may fail due to network operations (checking plugin updates),
	// but refreshGeneratedAdapters runs unconditionally before any network call.
	// We tolerate network errors but require the symlink to be gone.
	_ = err

	builtin, err := registry.BuiltinRegistry().Find("ddx")
	require.NoError(t, err)
	cacheSkillDir := filepath.Join(registry.PluginCacheDir("ddx", builtin.Version), "skills", "ddx")
	assert.FileExists(t, filepath.Join(cacheSkillDir, "SKILL.md"))
	assertLocalSymlink(t, symlinkPath, cacheSkillDir)
}

func TestUpdate_DoesNotAttemptBinaryUpgrade(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	te := NewTestEnvironment(t, WithGitInit(false))

	output, err := te.RunCommand("update")
	require.NoError(t, err)

	assert.NotContains(t, output, "Checking for DDx updates")
	assert.NotContains(t, output, "Upgrading DDx")
	assert.Contains(t, output, "Generated adapters refreshed")
}

// TestUpdate_RefusesDirtyGeneratedAdapterFile asserts ddx update --force exits
// non-zero when a legacy real-file generated adapter has uncommitted changes.
// The error output must mention --force --discard-local [AC1].
func TestUpdate_RefusesDirtyGeneratedAdapterFile(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	te := NewTestEnvironment(t) // git-initialised by default

	installedFile := filepath.Join(".agents", "skills", "ddx", "SKILL.md")

	te.CreateFile(installedFile, "original content\n")
	commitTestFiles(t, te.Dir, installedFile)

	// Modify without staging → dirty worktree
	te.CreateFile(installedFile, "modified content — not yet committed\n")

	output, err := te.RunCommand("update", "--force")

	require.Error(t, err, "update --force should fail when an update-target file is dirty")
	assert.Contains(t, output, "--force --discard-local",
		"error output should mention --force --discard-local; got: %q", output)
}

// TestUpdate_AllowsCleanGeneratedAdapterFile asserts ddx update --force proceeds
// without a dirty-file error when a legacy real-file generated adapter has no
// uncommitted changes [AC2].
func TestUpdate_AllowsCleanGeneratedAdapterFile(t *testing.T) {
	homeDir := t.TempDir()
	xdgDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDir)

	te := NewTestEnvironment(t) // git-initialised by default

	installedFile := filepath.Join(".agents", "skills", "ddx", "SKILL.md")

	te.CreateFile(installedFile, "original content\n")
	commitTestFiles(t, te.Dir, installedFile)

	// File is clean — no modification after commit.

	output, err := te.RunCommand("update", "--force")

	require.NoError(t, err, output)
	assert.NotContains(t, output, "--force --discard-local",
		"clean file should not trigger the dirty-file error; got: %q", output)
}

// TestUpdate_DiscardLocalOverwritesDirty asserts that ddx update --force
// --discard-local succeeds when a generated adapter file is dirty, and that the
// original content is saved to .ddx/update-backup/<timestamp>/<path> [AC3].
func TestUpdate_DiscardLocalOverwritesDirty(t *testing.T) {
	homeDir := t.TempDir()
	xdgDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDir)

	te := NewTestEnvironment(t) // git-initialised by default

	installedFile := filepath.Join(".agents", "skills", "ddx", "SKILL.md")

	te.CreateFile(installedFile, "original committed content\n")
	commitTestFiles(t, te.Dir, installedFile)

	// Make dirty — this is the content that must be saved in the backup.
	dirtyContent := "modified content — not yet committed\n"
	te.CreateFile(installedFile, dirtyContent)

	output, err := te.RunCommand("update", "--force", "--discard-local")
	require.NoError(t, err, output)

	// Must NOT produce the dirty-file error.
	assert.NotContains(t, output, "--force --discard-local", /*as error msg*/
		"--discard-local should suppress the dirty-file block; got: %q", output)

	// Backup directory must exist under .ddx/update-backup/<timestamp>/
	backupRoot := filepath.Join(te.Dir, ddxroot.DirName, "update-backup")
	entries, readErr := os.ReadDir(backupRoot)
	require.NoError(t, readErr, "update-backup directory should be created")
	require.NotEmpty(t, entries, "at least one timestamped backup dir should exist")

	// The backup file must contain the dirty (pre-overwrite) content so the
	// operator can recover their uncommitted changes.
	backupFile := filepath.Join(backupRoot, entries[0].Name(), installedFile)
	data, readErr := os.ReadFile(backupFile)
	require.NoError(t, readErr, "backup file should be readable: %s", backupFile)
	assert.Equal(t, dirtyContent, string(data),
		"backup should contain the dirty (pre-overwrite) content")
}

func TestUpdateGlobal_IsRetiredAndDoesNotTouchProjectOrGlobalTrees(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	// Seed a sentinel in the project plugin tree to verify it is not modified.
	projectPluginDir := filepath.Join(workDir, ddxroot.DirName, "plugins", "helix")
	require.NoError(t, os.MkdirAll(projectPluginDir, 0o755))
	projectSentinel := filepath.Join(projectPluginDir, "sentinel.txt")
	require.NoError(t, os.WriteFile(projectSentinel, []byte("project-side"), 0o644))

	// Seed a sentinel in the global plugin tree.
	globalPluginDir := filepath.Join(xdgDataHome, "ddx", "global", "plugins", "helix")
	require.NoError(t, os.MkdirAll(globalPluginDir, 0o755))
	globalSentinel := filepath.Join(globalPluginDir, "sentinel.txt")
	require.NoError(t, os.WriteFile(globalSentinel, []byte("global-side"), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "update", "--global")
	require.Error(t, err, output)
	assert.Contains(t, err.Error(), "global plugin installs are retired")

	projectData, readErr := os.ReadFile(projectSentinel)
	require.NoError(t, readErr, "project sentinel must remain after retired global update")
	assert.Equal(t, "project-side", string(projectData), "project sentinel must be byte-identical")

	globalData, readErr := os.ReadFile(globalSentinel)
	require.NoError(t, readErr, "global sentinel must remain after retired global update")
	assert.Equal(t, "global-side", string(globalData), "global sentinel must be byte-identical")
}

// TestUpdate_RespectsConventionVsInTreeMode verifies AC2:
// ddx update (no --global) updates the project install in the active mode and
// never touches the global plugin tree.
func TestUpdate_RespectsConventionVsInTreeMode(t *testing.T) {
	t.Run("in-tree", func(t *testing.T) {
		workDir := t.TempDir()
		homeDir := t.TempDir()
		xdgDataHome := t.TempDir()
		t.Setenv("HOME", homeDir)
		t.Setenv("XDG_DATA_HOME", xdgDataHome)

		// Force in-tree mode.
		require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

		// Seed a sentinel in the global plugin tree to verify it is not touched.
		globalPluginDir := filepath.Join(xdgDataHome, "ddx", "global", "plugins", "helix")
		require.NoError(t, os.MkdirAll(globalPluginDir, 0o755))
		globalSentinel := filepath.Join(globalPluginDir, "sentinel.txt")
		require.NoError(t, os.WriteFile(globalSentinel, []byte("global-side"), 0o644))

		factory := NewCommandFactory(workDir)
		output, err := executeCommand(factory.NewRootCommand(), "update")
		require.NoError(t, err, output)

		// Generated adapters should be refreshed in the in-tree project.
		assert.Contains(t, output, "Generated adapters refreshed")

		// Global tree must be byte-identical after project update.
		globalData, readErr := os.ReadFile(globalSentinel)
		require.NoError(t, readErr, "global sentinel must exist after in-tree project update")
		assert.Equal(t, "global-side", string(globalData), "global sentinel must be byte-identical")
	})

	t.Run("convention", func(t *testing.T) {
		workDir := t.TempDir()
		homeDir := t.TempDir()
		xdgDataHome := t.TempDir()
		t.Setenv("HOME", homeDir)
		t.Setenv("XDG_DATA_HOME", xdgDataHome)

		// No .ddx/ in workDir — convention mode.

		// Seed a sentinel in the global plugin tree.
		globalPluginDir := filepath.Join(xdgDataHome, "ddx", "global", "plugins", "helix")
		require.NoError(t, os.MkdirAll(globalPluginDir, 0o755))
		globalSentinel := filepath.Join(globalPluginDir, "sentinel.txt")
		require.NoError(t, os.WriteFile(globalSentinel, []byte("global-side"), 0o644))

		factory := NewCommandFactory(workDir)
		output, err := executeCommand(factory.NewRootCommand(), "update")
		require.NoError(t, err, output)

		// Generated adapters should be refreshed in the project dir.
		assert.Contains(t, output, "Generated adapters refreshed")

		// Global tree must be byte-identical after convention-mode project update.
		globalData, readErr := os.ReadFile(globalSentinel)
		require.NoError(t, readErr, "global sentinel must exist after convention project update")
		assert.Equal(t, "global-side", string(globalData), "global sentinel must be byte-identical")
	})
}

// TestUpdate_PartialDirtyBatchHaltsCleanly asserts that when multiple generated
// adapter files are slated for overwrite and even ONE is dirty, the entire
// update halts before any file is mutated — clean files remain untouched [AC4].
func TestUpdate_PartialDirtyBatchHaltsCleanly(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	te := NewTestEnvironment(t) // git-initialised by default

	dirtyFile := filepath.Join(".agents", "skills", "ddx", "SKILL.md")
	cleanFile := filepath.Join(".claude", "skills", "ddx", "SKILL.md")

	te.CreateFile(dirtyFile, "dirty file content\n")
	te.CreateFile(cleanFile, "clean file content\n")
	commitTestFiles(t, te.Dir, dirtyFile, cleanFile)

	// Only modify the dirty file
	te.CreateFile(dirtyFile, "modified and uncommitted\n")

	_, err := te.RunCommand("update", "--force")
	require.Error(t, err, "update should fail when any target file is dirty")

	// The clean file must NOT have been mutated by the aborted update.
	data, readErr := os.ReadFile(filepath.Join(te.Dir, cleanFile))
	require.NoError(t, readErr)
	assert.Equal(t, "clean file content\n", string(data),
		"clean file must be unchanged after atomic refuse")

	dirtyData, readErr := os.ReadFile(filepath.Join(te.Dir, dirtyFile))
	require.NoError(t, readErr)
	assert.Equal(t, "modified and uncommitted\n", string(dirtyData),
		"dirty file must be unchanged after atomic refuse")
}

func TestUpdatePluginTargetPointsToPluginUpgrade(t *testing.T) {
	te := NewTestEnvironment(t, WithGitInit(false))

	output, err := te.RunCommand("update", "helix")

	require.Error(t, err, output)
	assert.Contains(t, output, "ddx plugin upgrade helix")
}
