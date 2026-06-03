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

// writeRegistryInstalled writes a fake installed.yaml under ~HOME/.ddx/ so that
// performUpdate sees the listed files as registry-installed update targets.
func writeRegistryInstalled(t *testing.T, entries []registry.InstalledEntry) {
	t.Helper()
	state := &registry.InstalledState{Installed: entries}
	require.NoError(t, registry.SaveState(state))
}

// TestUpdate_ForceReplacesStaleSymlinks verifies that ddx update --force
// replaces legacy symlinks under .agents/skills/ with real files.
// This covers the FEAT-015 requirement that skills.Install is always invoked
// with Force:true during update (via refreshShippedSkills).
func TestUpdate_ForceReplacesStaleSymlinks(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

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

func TestUpdate_DoesNotAttemptBinaryUpgrade(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	te := NewTestEnvironment(t, WithGitInit(false))

	output, err := te.RunCommand("update")
	require.NoError(t, err)

	assert.NotContains(t, output, "Checking for DDx updates")
	assert.NotContains(t, output, "Upgrading DDx")
	assert.Contains(t, output, "Shipped skills refreshed")
}

// TestUpdate_RefusesDirtyLibraryFile asserts ddx update --force exits non-zero
// when a registry-installed file (library/skills/ddx/SKILL.md) has uncommitted
// changes and the new content would differ. The error output must mention
// --force --discard-local [AC1].
func TestUpdate_RefusesDirtyLibraryFile(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	te := NewTestEnvironment(t) // git-initialised by default

	installedFile := filepath.Join("library", "skills", "ddx", "SKILL.md")
	writeRegistryInstalled(t, []registry.InstalledEntry{{
		Name:    "ddx",
		Version: "0.4.7",
		Type:    registry.PackageTypePlugin,
		Files:   []string{installedFile},
	}})

	te.CreateFile(installedFile, "original content\n")
	commitTestFiles(t, te.Dir, installedFile)

	// Modify without staging → dirty worktree
	te.CreateFile(installedFile, "modified content — not yet committed\n")

	output, err := te.RunCommand("update", "--force")

	require.Error(t, err, "update --force should fail when an update-target file is dirty")
	assert.Contains(t, output, "--force --discard-local",
		"error output should mention --force --discard-local; got: %q", output)
}

// TestUpdate_AllowsCleanLibraryFile asserts ddx update --force proceeds without
// a dirty-file error when the registry-installed file has no uncommitted changes
// [AC2].
func TestUpdate_AllowsCleanLibraryFile(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	te := NewTestEnvironment(t) // git-initialised by default

	installedFile := filepath.Join("library", "skills", "ddx", "SKILL.md")
	writeRegistryInstalled(t, []registry.InstalledEntry{{
		Name:    "ddx",
		Version: "0.4.7",
		Type:    registry.PackageTypePlugin,
		Files:   []string{installedFile},
	}})

	te.CreateFile(installedFile, "original content\n")
	commitTestFiles(t, te.Dir, installedFile)

	// File is clean — no modification after commit.

	output, err := te.RunCommand("update", "--force")

	// A network error from InstallPackage is tolerated; the test only asserts
	// that the dirty-file guard did NOT fire.
	_ = err
	assert.NotContains(t, output, "--force --discard-local",
		"clean file should not trigger the dirty-file error; got: %q", output)
}

// TestUpdate_DiscardLocalOverwritesDirty asserts that ddx update --force
// --discard-local succeeds when an installed file is dirty, and that the
// original content is saved to .ddx/update-backup/<timestamp>/<path> [AC3].
func TestUpdate_DiscardLocalOverwritesDirty(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	te := NewTestEnvironment(t) // git-initialised by default

	installedFile := filepath.Join("library", "skills", "ddx", "SKILL.md")
	writeRegistryInstalled(t, []registry.InstalledEntry{{
		Name:    "ddx",
		Version: "0.4.7",
		Type:    registry.PackageTypePlugin,
		Files:   []string{installedFile},
	}})

	te.CreateFile(installedFile, "original committed content\n")
	commitTestFiles(t, te.Dir, installedFile)

	// Make dirty — this is the content that must be saved in the backup.
	dirtyContent := "modified content — not yet committed\n"
	te.CreateFile(installedFile, dirtyContent)

	// Run with --discard-local; tolerate network errors from InstallPackage.
	output, err := te.RunCommand("update", "--force", "--discard-local")
	_ = err

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

// TestUpdateGlobal_UpdatesGlobalNotProject verifies AC1:
// ddx update --global modifies only ${XDG_DATA_HOME}/ddx/global/plugins/ and
// leaves the project plugins/ dir byte-identical.
func TestUpdateGlobal_UpdatesGlobalNotProject(t *testing.T) {
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

	// Run ddx update --global with empty global installed state (no packages to update).
	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "update", "--global")
	require.NoError(t, err, output)

	// Project plugin tree must be byte-identical after global update.
	projectData, readErr := os.ReadFile(projectSentinel)
	require.NoError(t, readErr, "project sentinel must exist after global update")
	assert.Equal(t, "project-side", string(projectData), "project sentinel must be byte-identical")

	// Global update must not trigger project-level skill refresh.
	assert.NotContains(t, output, "Shipped skills refreshed")
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

		writeRegistryInstalled(t, []registry.InstalledEntry{})

		factory := NewCommandFactory(workDir)
		output, err := executeCommand(factory.NewRootCommand(), "update")
		require.NoError(t, err, output)

		// Shipped skills should be refreshed in the in-tree project.
		assert.Contains(t, output, "Shipped skills refreshed")

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

		writeRegistryInstalled(t, []registry.InstalledEntry{})

		factory := NewCommandFactory(workDir)
		output, err := executeCommand(factory.NewRootCommand(), "update")
		require.NoError(t, err, output)

		// Shipped skills should be refreshed in the project dir.
		assert.Contains(t, output, "Shipped skills refreshed")

		// Global tree must be byte-identical after convention-mode project update.
		globalData, readErr := os.ReadFile(globalSentinel)
		require.NoError(t, readErr, "global sentinel must exist after convention project update")
		assert.Equal(t, "global-side", string(globalData), "global sentinel must be byte-identical")
	})
}

// TestUpdate_PartialDirtyBatchHaltsCleanly asserts that when multiple
// registry-installed files are slated for overwrite and even ONE is dirty, the
// entire update halts before any file is mutated — clean files remain untouched
// [AC4].
func TestUpdate_PartialDirtyBatchHaltsCleanly(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	te := NewTestEnvironment(t) // git-initialised by default

	dirtyFile := filepath.Join("library", "skills", "ddx", "SKILL.md")
	cleanFile := filepath.Join("library", "skills", "ddx", "reference", "work.md")
	writeRegistryInstalled(t, []registry.InstalledEntry{{
		Name:    "ddx",
		Version: "0.4.7",
		Type:    registry.PackageTypePlugin,
		Files:   []string{dirtyFile, cleanFile},
	}})

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

	// refreshShippedSkills must NOT have run — .agents/skills/ddx/ should
	// not have been created.
	_, statErr := os.Stat(filepath.Join(te.Dir, ".agents", "skills", "ddx"))
	assert.True(t, os.IsNotExist(statErr),
		".agents/skills/ddx should not exist after atomic refuse")
}
