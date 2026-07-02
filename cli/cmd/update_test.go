package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	updatepkg "github.com/DocumentDrivenDX/ddx/internal/update"
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

type updateInstallCall struct {
	installRoot  string
	rootTarget   string
	skillTargets []string
}

func makeUpdateInstaller(t *testing.T, call *updateInstallCall) func(*registry.Package, string) (registry.InstalledEntry, error) {
	t.Helper()
	return func(pkg *registry.Package, installRoot string) (registry.InstalledEntry, error) {
		t.Helper()

		if call != nil {
			call.installRoot = installRoot
			if pkg.Install.Root != nil {
				call.rootTarget = pkg.Install.Root.Target
			}
			call.skillTargets = call.skillTargets[:0]
			for _, mapping := range pkg.Install.Skills {
				call.skillTargets = append(call.skillTargets, mapping.Target)
			}
		}

		rootPath := filepath.Join(installRoot, filepath.FromSlash(pkg.Install.Root.Target))
		require.NoError(t, os.MkdirAll(rootPath, 0o755))
		rootMarker := filepath.Join(rootPath, "root-sentinel.txt")
		require.NoError(t, os.WriteFile(rootMarker, []byte(pkg.Name+" "+pkg.Version), 0o644))

		files := []string{filepath.Join(pkg.Install.Root.Target, "root-sentinel.txt")}
		for _, mapping := range pkg.Install.Skills {
			targetDir := registry.ExpandHome(mapping.Target)
			skillDir := filepath.Join(targetDir, pkg.Name+"-skill")
			require.NoError(t, os.MkdirAll(skillDir, 0o755))
			marker := filepath.Join(skillDir, "SKILL.md")
			require.NoError(t, os.WriteFile(marker, []byte(pkg.Name+" "+pkg.Version), 0o644))
			files = append(files, marker)
		}

		return registry.InstalledEntry{
			Name:        pkg.Name,
			Version:     pkg.Version,
			Type:        pkg.Type,
			Source:      pkg.Source,
			InstalledAt: time.Now(),
			Files:       files,
		}, nil
	}
}

func setGitOrigin(t *testing.T, dir, remote string) {
	t.Helper()
	cleanEnv := gitpkg.CleanEnv()
	cmd := exec.Command("git", "remote", "add", "origin", remote)
	cmd.Dir = dir
	cmd.Env = cleanEnv
	require.NoError(t, cmd.Run(), "git remote add origin")
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

	installedFile := filepath.Join("plugins", "ddx", "SKILL.md")
	writeRegistryInstalled(t, []registry.InstalledEntry{{
		Name:    "ddx",
		Version: "0.4.7",
		Type:    registry.PackageTypePlugin,
		Files:   []string{installedFile},
	}})

	te.CreateFile(filepath.Join(ddxroot.DirName, installedFile), "original content\n")
	commitTestFiles(t, te.Dir, filepath.Join(ddxroot.DirName, installedFile))

	// Modify without staging → dirty worktree
	te.CreateFile(filepath.Join(ddxroot.DirName, installedFile), "modified content — not yet committed\n")

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

	installedFile := filepath.Join("plugins", "ddx", "SKILL.md")
	writeRegistryInstalled(t, []registry.InstalledEntry{{
		Name:    "ddx",
		Version: "0.4.7",
		Type:    registry.PackageTypePlugin,
		Files:   []string{installedFile},
	}})

	te.CreateFile(filepath.Join(ddxroot.DirName, installedFile), "original content\n")
	commitTestFiles(t, te.Dir, filepath.Join(ddxroot.DirName, installedFile))

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

	installedFile := filepath.Join("plugins", "ddx", "SKILL.md")
	writeRegistryInstalled(t, []registry.InstalledEntry{{
		Name:    "ddx",
		Version: "0.4.7",
		Type:    registry.PackageTypePlugin,
		Files:   []string{installedFile},
	}})

	te.CreateFile(filepath.Join(ddxroot.DirName, installedFile), "original committed content\n")
	commitTestFiles(t, te.Dir, filepath.Join(ddxroot.DirName, installedFile))

	// Make dirty — this is the content that must be saved in the backup.
	dirtyContent := "modified content — not yet committed\n"
	te.CreateFile(filepath.Join(ddxroot.DirName, installedFile), dirtyContent)

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
	backupFile := filepath.Join(backupRoot, entries[0].Name(), ddxroot.DirName, installedFile)
	data, readErr := os.ReadFile(backupFile)
	require.NoError(t, readErr, "backup file should be readable: %s", backupFile)
	assert.Equal(t, dirtyContent, string(data),
		"backup should contain the dirty (pre-overwrite) content")
}

// TestUpdateGlobal_UpdatesGlobalNotProject verifies AC1:
// ddx update --global mutates only the global plugin tree and leaves the
// project plugin tree and project skill links untouched.
func TestUpdateGlobal_UpdatesGlobalNotProject(t *testing.T) {
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	env := NewTestEnvironment(t, WithGitInit(false))
	workDir := env.Dir

	// Seed project-local sentinels that must not change during a global update.
	projectPluginDir := filepath.Join(workDir, ddxroot.DirName, "plugins", "helix")
	require.NoError(t, os.MkdirAll(projectPluginDir, 0o755))
	projectSentinel := filepath.Join(projectPluginDir, "root-sentinel.txt")
	require.NoError(t, os.WriteFile(projectSentinel, []byte("project-side"), 0o644))
	env.CreateFile(filepath.Join(".agents", "skills", "helix-skill", "SKILL.md"), "project-agent-side\n")
	env.CreateFile(filepath.Join(".claude", "skills", "helix-skill", "SKILL.md"), "project-claude-side\n")

	// Seed a sentinel in the global plugin tree.
	globalPluginDir := filepath.Join(ddxroot.GlobalDir(), "plugins", "helix")
	require.NoError(t, os.MkdirAll(globalPluginDir, 0o755))
	globalSentinel := filepath.Join(globalPluginDir, "root-sentinel.txt")
	require.NoError(t, os.WriteFile(globalSentinel, []byte("global-side"), 0o644))

	require.NoError(t, registry.SaveGlobalState(&registry.InstalledState{
		Installed: []registry.InstalledEntry{{
			Name:    "helix",
			Version: "0.1.0",
			Type:    registry.PackageTypeWorkflow,
		}},
	}))

	call := &updateInstallCall{}
	factory := NewCommandFactory(workDir)
	factory.updateFetchLatestReleaseForRepo = func(string) (*updatepkg.GitHubRelease, error) {
		return &updatepkg.GitHubRelease{TagName: "v9.9.9"}, nil
	}
	factory.updateInstallPackage = makeUpdateInstaller(t, call)

	output, err := executeCommand(factory.NewRootCommand(), "update", "--global")
	require.NoError(t, err, output)

	assert.Equal(t, ddxroot.GlobalDir(), call.installRoot, "global update must install into the global plugin tree")
	assert.Equal(t, filepath.Join("plugins", "helix"), call.rootTarget, "global root target must stay under plugins/<name>")
	assert.Equal(t, []string{
		filepath.Join(homeDir, ".agents", "skills") + string(os.PathSeparator),
		filepath.Join(homeDir, ".claude", "skills") + string(os.PathSeparator),
	}, call.skillTargets, "global skill targets must be refreshed in the home tier")

	// Project plugin tree must be byte-identical after global update.
	projectData, readErr := os.ReadFile(projectSentinel)
	require.NoError(t, readErr, "project sentinel must exist after global update")
	assert.Equal(t, "project-side", string(projectData), "project sentinel must be byte-identical")

	projectAgentSkill := filepath.Join(workDir, ".agents", "skills", "helix-skill", "SKILL.md")
	projectClaudeSkill := filepath.Join(workDir, ".claude", "skills", "helix-skill", "SKILL.md")
	projectAgentData, readErr := os.ReadFile(projectAgentSkill)
	require.NoError(t, readErr, "project agent skill must exist after global update")
	assert.Equal(t, "project-agent-side\n", string(projectAgentData))
	projectClaudeData, readErr := os.ReadFile(projectClaudeSkill)
	require.NoError(t, readErr, "project claude skill must exist after global update")
	assert.Equal(t, "project-claude-side\n", string(projectClaudeData))

	// Global update must not trigger project-level skill refresh.
	assert.NotContains(t, output, "Shipped skills refreshed")

	globalData, readErr := os.ReadFile(globalSentinel)
	require.NoError(t, readErr, "global sentinel must exist after global update")
	assert.Equal(t, "helix 9.9.9", string(globalData), "global tree should be refreshed")
}

// TestUpdate_RespectsConventionVsInTreeMode verifies AC2:
// ddx update (no --global) updates the project install in the active mode and
// never touches the global plugin tree.
func TestUpdate_RespectsConventionVsInTreeMode(t *testing.T) {
	t.Run("in-tree", func(t *testing.T) {
		homeDir := t.TempDir()
		xdgDataHome := t.TempDir()
		t.Setenv("HOME", homeDir)
		t.Setenv("XDG_DATA_HOME", xdgDataHome)

		env := NewTestEnvironment(t, WithGitInit(false))
		workDir := env.Dir

		// Seed sentinels in both the project tree and the global tree.
		projectPluginDir := filepath.Join(workDir, ddxroot.DirName, "plugins", "helix")
		require.NoError(t, os.MkdirAll(projectPluginDir, 0o755))
		projectSentinel := filepath.Join(projectPluginDir, "root-sentinel.txt")
		require.NoError(t, os.WriteFile(projectSentinel, []byte("project-side"), 0o644))
		env.CreateFile(filepath.Join(".agents", "skills", "helix-skill", "SKILL.md"), "project-agent-side\n")
		env.CreateFile(filepath.Join(".claude", "skills", "helix-skill", "SKILL.md"), "project-claude-side\n")

		globalPluginDir := filepath.Join(ddxroot.GlobalDir(), "plugins", "helix")
		require.NoError(t, os.MkdirAll(globalPluginDir, 0o755))
		globalSentinel := filepath.Join(globalPluginDir, "root-sentinel.txt")
		require.NoError(t, os.WriteFile(globalSentinel, []byte("global-side"), 0o644))

		require.NoError(t, registry.SaveState(&registry.InstalledState{
			Installed: []registry.InstalledEntry{{
				Name:    "helix",
				Version: "0.1.0",
				Type:    registry.PackageTypeWorkflow,
			}},
		}))

		call := &updateInstallCall{}
		factory := NewCommandFactory(workDir)
		factory.updateFetchLatestReleaseForRepo = func(string) (*updatepkg.GitHubRelease, error) {
			return &updatepkg.GitHubRelease{TagName: "v9.9.9"}, nil
		}
		factory.updateInstallPackage = makeUpdateInstaller(t, call)

		output, err := executeCommand(factory.NewRootCommand(), "update")
		require.NoError(t, err, output)

		// Project installs must resolve to the in-tree .ddx directory.
		assert.Equal(t, filepath.Join(workDir, ddxroot.DirName), call.installRoot)
		assert.Equal(t, filepath.Join("plugins", "helix"), call.rootTarget)
		assert.Equal(t, []string{
			filepath.Join(workDir, ".agents", "skills") + string(os.PathSeparator),
			filepath.Join(workDir, ".claude", "skills") + string(os.PathSeparator),
		}, call.skillTargets)

		projectData, readErr := os.ReadFile(projectSentinel)
		require.NoError(t, readErr, "project sentinel must exist after in-tree update")
		assert.Equal(t, "helix 9.9.9", string(projectData), "project tree must be refreshed")

		projectAgentSkill := filepath.Join(workDir, ".agents", "skills", "helix-skill", "SKILL.md")
		projectClaudeSkill := filepath.Join(workDir, ".claude", "skills", "helix-skill", "SKILL.md")
		projectAgentData, readErr := os.ReadFile(projectAgentSkill)
		require.NoError(t, readErr, "project agent skill must exist after in-tree update")
		assert.Equal(t, "helix 9.9.9", string(projectAgentData))
		projectClaudeData, readErr := os.ReadFile(projectClaudeSkill)
		require.NoError(t, readErr, "project claude skill must exist after in-tree update")
		assert.Equal(t, "helix 9.9.9", string(projectClaudeData))

		// Global tree must be byte-identical after project update.
		globalData, readErr := os.ReadFile(globalSentinel)
		require.NoError(t, readErr, "global sentinel must exist after in-tree project update")
		assert.Equal(t, "global-side", string(globalData), "global sentinel must be byte-identical")
	})

	t.Run("convention", func(t *testing.T) {
		homeDir := t.TempDir()
		xdgDataHome := t.TempDir()
		t.Setenv("HOME", homeDir)
		t.Setenv("XDG_DATA_HOME", xdgDataHome)

		env := NewTestEnvironment(t)
		workDir := env.Dir
		require.NoError(t, os.RemoveAll(filepath.Join(workDir, ddxroot.DirName)))
		setGitOrigin(t, workDir, "https://github.com/acme/widgets.git")

		projectRoot := filepath.Join(xdgDataHome, "ddx", "projects", "github.com", "acme", "widgets")
		projectPluginDir := filepath.Join(projectRoot, "plugins", "helix")
		require.NoError(t, os.MkdirAll(projectPluginDir, 0o755))
		projectSentinel := filepath.Join(projectPluginDir, "root-sentinel.txt")
		require.NoError(t, os.WriteFile(projectSentinel, []byte("project-side"), 0o644))
		env.CreateFile(filepath.Join(".agents", "skills", "helix-skill", "SKILL.md"), "project-agent-side\n")
		env.CreateFile(filepath.Join(".claude", "skills", "helix-skill", "SKILL.md"), "project-claude-side\n")

		globalPluginDir := filepath.Join(ddxroot.GlobalDir(), "plugins", "helix")
		require.NoError(t, os.MkdirAll(globalPluginDir, 0o755))
		globalSentinel := filepath.Join(globalPluginDir, "root-sentinel.txt")
		require.NoError(t, os.WriteFile(globalSentinel, []byte("global-side"), 0o644))

		require.NoError(t, registry.SaveState(&registry.InstalledState{
			Installed: []registry.InstalledEntry{{
				Name:    "helix",
				Version: "0.1.0",
				Type:    registry.PackageTypeWorkflow,
			}},
		}))

		call := &updateInstallCall{}
		factory := NewCommandFactory(workDir)
		factory.updateFetchLatestReleaseForRepo = func(string) (*updatepkg.GitHubRelease, error) {
			return &updatepkg.GitHubRelease{TagName: "v9.9.9"}, nil
		}
		factory.updateInstallPackage = makeUpdateInstaller(t, call)

		output, err := executeCommand(factory.NewRootCommand(), "update")
		require.NoError(t, err, output)

		assert.Equal(t, projectRoot, call.installRoot, "convention mode must install under the XDG project root")
		assert.Equal(t, filepath.Join("plugins", "helix"), call.rootTarget)
		assert.Equal(t, []string{
			filepath.Join(workDir, ".agents", "skills") + string(os.PathSeparator),
			filepath.Join(workDir, ".claude", "skills") + string(os.PathSeparator),
		}, call.skillTargets)

		projectData, readErr := os.ReadFile(projectSentinel)
		require.NoError(t, readErr, "project sentinel must exist after convention update")
		assert.Equal(t, "helix 9.9.9", string(projectData), "convention project tree must be refreshed")

		projectAgentSkill := filepath.Join(workDir, ".agents", "skills", "helix-skill", "SKILL.md")
		projectClaudeSkill := filepath.Join(workDir, ".claude", "skills", "helix-skill", "SKILL.md")
		projectAgentData, readErr := os.ReadFile(projectAgentSkill)
		require.NoError(t, readErr, "project agent skill must exist after convention update")
		assert.Equal(t, "helix 9.9.9", string(projectAgentData))
		projectClaudeData, readErr := os.ReadFile(projectClaudeSkill)
		require.NoError(t, readErr, "project claude skill must exist after convention update")
		assert.Equal(t, "helix 9.9.9", string(projectClaudeData))

		globalData, readErr := os.ReadFile(globalSentinel)
		require.NoError(t, readErr, "global sentinel must exist after convention project update")
		assert.Equal(t, "global-side", string(globalData), "global sentinel must be byte-identical")
	})
}

// TestUpdateGlobal_RefreshesAgentTierLinks verifies AC3:
// global update refreshes ~/.agents/skills/<name> and ~/.claude/skills/<name>
// from the global plugin tree.
func TestUpdateGlobal_RefreshesAgentTierLinks(t *testing.T) {
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	env := NewTestEnvironment(t, WithGitInit(false))
	workDir := env.Dir

	require.NoError(t, registry.SaveGlobalState(&registry.InstalledState{
		Installed: []registry.InstalledEntry{{
			Name:    "helix",
			Version: "0.1.0",
			Type:    registry.PackageTypeWorkflow,
		}},
	}))

	call := &updateInstallCall{}
	factory := NewCommandFactory(workDir)
	factory.updateFetchLatestReleaseForRepo = func(string) (*updatepkg.GitHubRelease, error) {
		return &updatepkg.GitHubRelease{TagName: "v9.9.9"}, nil
	}
	factory.updateInstallPackage = makeUpdateInstaller(t, call)

	output, err := executeCommand(factory.NewRootCommand(), "update", "--global")
	require.NoError(t, err, output)

	expectedGlobalSkillDir := filepath.Join(homeDir, ".agents", "skills", "helix-skill")
	agentSkillFile := filepath.Join(expectedGlobalSkillDir, "SKILL.md")
	claudeSkillFile := filepath.Join(homeDir, ".claude", "skills", "helix-skill", "SKILL.md")

	assert.Equal(t, ddxroot.GlobalDir(), call.installRoot, "global update must install into the global plugin tree")
	assert.Equal(t, filepath.Join("plugins", "helix"), call.rootTarget)

	agentSkillData, readErr := os.ReadFile(agentSkillFile)
	require.NoError(t, readErr, "global update must refresh ~/.agents/skills/helix-skill")
	assert.Equal(t, "helix 9.9.9", string(agentSkillData))
	claudeSkillData, readErr := os.ReadFile(claudeSkillFile)
	require.NoError(t, readErr, "global update must refresh ~/.claude/skills/helix-skill")
	assert.Equal(t, "helix 9.9.9", string(claudeSkillData))

	assert.Equal(t, []string{
		filepath.Join(homeDir, ".agents", "skills") + string(os.PathSeparator),
		filepath.Join(homeDir, ".claude", "skills") + string(os.PathSeparator),
	}, call.skillTargets)
	assert.NotContains(t, output, "Shipped skills refreshed")
}

// TestUpdate_PartialDirtyBatchHaltsCleanly asserts that when multiple
// registry-installed files are slated for overwrite and even ONE is dirty, the
// entire update halts before any file is mutated — clean files remain untouched
// [AC4].
func TestUpdate_PartialDirtyBatchHaltsCleanly(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	te := NewTestEnvironment(t) // git-initialised by default

	dirtyFile := filepath.Join("plugins", "ddx", "SKILL.md")
	cleanFile := filepath.Join("plugins", "ddx", "reference", "work.md")
	writeRegistryInstalled(t, []registry.InstalledEntry{{
		Name:    "ddx",
		Version: "0.4.7",
		Type:    registry.PackageTypePlugin,
		Files:   []string{dirtyFile, cleanFile},
	}})

	te.CreateFile(filepath.Join(ddxroot.DirName, dirtyFile), "dirty file content\n")
	te.CreateFile(filepath.Join(ddxroot.DirName, cleanFile), "clean file content\n")
	commitTestFiles(t, te.Dir, filepath.Join(ddxroot.DirName, dirtyFile), filepath.Join(ddxroot.DirName, cleanFile))

	// Only modify the dirty file
	te.CreateFile(filepath.Join(ddxroot.DirName, dirtyFile), "modified and uncommitted\n")

	_, err := te.RunCommand("update", "--force")
	require.Error(t, err, "update should fail when any target file is dirty")

	// The clean file must NOT have been mutated by the aborted update.
	data, readErr := os.ReadFile(filepath.Join(te.Dir, ddxroot.DirName, cleanFile))
	require.NoError(t, readErr)
	assert.Equal(t, "clean file content\n", string(data),
		"clean file must be unchanged after atomic refuse")

	// refreshShippedSkills must NOT have run — .agents/skills/ddx/ should
	// not have been created.
	_, statErr := os.Stat(filepath.Join(te.Dir, ".agents", "skills", "ddx"))
	assert.True(t, os.IsNotExist(statErr),
		".agents/skills/ddx should not exist after atomic refuse")
}
