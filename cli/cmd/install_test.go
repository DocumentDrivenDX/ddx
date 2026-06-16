package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitPluginChangesDoesNotAbsorbPreStagedSource(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName), 0o755))
	runInstallTestGit(t, projectRoot, "init")
	runInstallTestGit(t, projectRoot, "config", "user.name", "Test User")
	runInstallTestGit(t, projectRoot, "config", "user.email", "test@example.invalid")

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("initial\n"), 0o644))
	runInstallTestGit(t, projectRoot, "add", "README.md")
	runInstallTestGit(t, projectRoot, "commit", "-m", "initial")

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "src.go"), []byte("package main\n"), 0o644))
	runInstallTestGit(t, projectRoot, "add", "src.go")
	lockRel := ddxroot.JoinRelative(registry.ProjectPluginLockFile)
	lockPath := filepath.Join(projectRoot, lockRel)
	require.NoError(t, os.WriteFile(lockPath, []byte(`{"plugins":[{"name":"helix","version":"1.2.3"}]}`+"\n"), 0o644))

	commitPluginChanges(projectRoot, "helix", "1.2.3")

	committedNames := runInstallTestGit(t, projectRoot, "show", "--format=", "--name-only", "HEAD")
	assert.Contains(t, committedNames, filepath.ToSlash(lockRel))
	assert.NotContains(t, committedNames, "src.go", "plugin auto-commit must not absorb pre-staged source")

	stagedNames := runInstallTestGit(t, projectRoot, "diff", "--cached", "--name-only")
	assert.Contains(t, stagedNames, "src.go", "pre-staged source must remain staged for the operator's commit")
}

func runInstallTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s failed:\n%s", strings.Join(args, " "), string(out))
	return string(out)
}

// TestInstallLocal_InTreeMode_WritesProjectPluginOverlayAndLinks verifies that
// `ddx plugin install <name> --local` writes a developer overlay symlink to
// <project>/.ddx/plugins/<name>/ and creates project-tier skill links.
func TestInstallLocal_InTreeMode_WritesProjectPluginOverlayAndLinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "myplugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	// The local checkout overlay lives under <project>/.ddx/plugins/<name>/.
	pluginDir := filepath.Join(workDir, ddxroot.DirName, "plugins", "myplugin")
	info, statErr := os.Lstat(pluginDir)
	require.NoError(t, statErr, "project plugin dir must exist at %s", pluginDir)
	assert.True(t, info.Mode()&os.ModeSymlink != 0, "project plugin dir must be a symlink")

	// Project-tier skill links must be under the project dir.
	for _, surface := range []string{".agents/skills", ".claude/skills"} {
		skillLink := filepath.Join(workDir, surface, "myplugin-skill")
		_, skillErr := os.Lstat(skillLink)
		require.NoError(t, skillErr, "%s must exist for in-tree install", skillLink)
	}

	// Home directory must not be polluted.
	_, noAgentErr := os.Lstat(filepath.Join(homeDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "home .agents/skills must not be created for in-tree install")
	_, noClaudeErr := os.Lstat(filepath.Join(homeDir, ".claude", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noClaudeErr), "home .claude/skills must not be created for in-tree install")
}

// TestInstallLocal_InTreeOverlayAndLinks verifies that local-overlay skill
// links resolve into the local plugin checkout, not into copied marketplace
// payloads.
func TestInstallLocal_InTreeOverlayAndLinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Force in-tree mode by pre-creating .ddx/.
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "myplugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	// The project plugin path is a symlink overlay to the local checkout.
	projectPluginDir := filepath.Join(workDir, ddxroot.DirName, "plugins", "myplugin")
	pluginInfo, pluginStatErr := os.Lstat(projectPluginDir)
	require.NoError(t, pluginStatErr, "project plugin dir must exist at %s", projectPluginDir)
	assert.True(t, pluginInfo.Mode()&os.ModeSymlink != 0, "project plugin dir must be a symlink to the local checkout")

	// Project-tier skill links must be in the project directory and resolve into the plugin source.
	for _, surface := range []string{".agents/skills", ".claude/skills"} {
		skillLink := filepath.Join(workDir, surface, "myplugin-skill")
		skillInfo, skillErr := os.Lstat(skillLink)
		require.NoError(t, skillErr, "%s/myplugin-skill must exist for in-tree install", surface)
		assert.True(t, skillInfo.Mode()&os.ModeSymlink != 0 || skillInfo.IsDir(),
			"%s/myplugin-skill must be a symlink or dir for in-tree install", surface)

		// Skill link must resolve into the same source tree as .ddx/plugins/myplugin.
		target, readErr := os.Readlink(skillLink)
		require.NoError(t, readErr, "%s must be a symlink", skillLink)
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(skillLink), target)
		}
		target, _ = filepath.Abs(target)
		assert.True(t, strings.HasPrefix(target, localPlugin),
			"%s must resolve into the plugin source dir; got %s", skillLink, target)
	}

	// Nothing must land in the home directory's skill dirs.
	_, noAgentErr := os.Lstat(filepath.Join(homeDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "home .agents/skills must not be created for in-tree install")
	_, noClaudeErr := os.Lstat(filepath.Join(homeDir, ".claude", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noClaudeErr), "home .claude/skills must not be created for in-tree install")
}

// TestInstallLocal_ConventionModeWritesXDGProjectOverlay verifies that with no
// <project>/.ddx/, `ddx plugin install <name> --local` writes an overlay under
// ${XDG_DATA_HOME}/ddx/projects/<host>/<owner>/<repo>/plugins/<name>/ and the
// project agent-tier links point into that overlay source.
// A git repo with a deterministic remote URL is used so host/owner/repo are stable.
func TestInstallLocal_ConventionModeWritesXDGProjectOverlay(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	// Set up a git repo with a known remote so project identity is deterministic.
	cleanEnv := gitpkg.CleanEnv()
	for _, tc := range []struct {
		args []string
		dir  string
	}{
		{[]string{"git", "init"}, workDir},
		{[]string{"git", "config", "user.email", "test@example.com"}, workDir},
		{[]string{"git", "config", "user.name", "Test"}, workDir},
		{[]string{"git", "remote", "add", "origin", "https://github.com/testowner/testrepo.git"}, workDir},
	} {
		cmd := exec.Command(tc.args[0], tc.args[1:]...)
		cmd.Dir = tc.dir
		cmd.Env = cleanEnv
		require.NoError(t, cmd.Run(), "setup: %v", tc.args)
	}

	// No .ddx/ in workDir — convention mode.
	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "myplugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	// Convention root must be at the XDG projects path derived from the remote URL.
	// The origin https://github.com/testowner/testrepo.git parses to github.com/testowner/testrepo.
	expectedConventionRoot := filepath.Join(xdgDataHome, "ddx", "projects", "github.com", "testowner", "testrepo")
	conventionPluginDir := filepath.Join(expectedConventionRoot, "plugins", "myplugin")
	pluginInfo, pluginStatErr := os.Lstat(conventionPluginDir)
	require.NoError(t, pluginStatErr, "XDG convention plugin dir must exist at %s", conventionPluginDir)
	assert.True(t, pluginInfo.Mode()&os.ModeSymlink != 0, "convention plugin dir must be a symlink to the local checkout")

	// Project agent-tier links must be in the project directory (not in the convention root).
	for _, surface := range []string{".agents/skills", ".claude/skills"} {
		skillLink := filepath.Join(workDir, surface, "myplugin-skill")
		skillInfo, skillErr := os.Lstat(skillLink)
		require.NoError(t, skillErr, "%s/myplugin-skill must exist for convention install", surface)
		assert.True(t, skillInfo.Mode()&os.ModeSymlink != 0 || skillInfo.IsDir(),
			"%s/myplugin-skill must be a symlink or dir for convention install", surface)

		// Skill link must resolve into the local plugin source.
		target, readErr := os.Readlink(skillLink)
		require.NoError(t, readErr, "%s must be a symlink", skillLink)
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(skillLink), target)
		}
		target, _ = filepath.Abs(target)
		assert.True(t, strings.HasPrefix(target, localPlugin),
			"%s must resolve into the plugin source dir; got %s", skillLink, target)
	}

	// Home directory must not be polluted.
	_, noAgentErr := os.Lstat(filepath.Join(homeDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "home .agents/skills must not be created for convention install")
	_, noClaudeErr := os.Lstat(filepath.Join(homeDir, ".claude", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noClaudeErr), "home .claude/skills must not be created for convention install")
}

// TestInstall_ConventionMode_WritesXDGProjectPluginsAndLinks verifies AC2 (bead ddx-747f1b35):
// With no <project>/.ddx/, ddx plugin install <name> (no --global) writes to
// ${XDG_DATA_HOME}/ddx/projects/<identity>/plugins/<name>/ and the project-tier
// skill links are under the project dir and resolve into the XDG plugins path.
func TestInstall_ConventionMode_WritesXDGProjectPluginsAndLinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	cleanEnv := gitpkg.CleanEnv()
	for _, tc := range []struct {
		args []string
	}{
		{[]string{"git", "init", workDir}},
		{[]string{"git", "-C", workDir, "config", "user.email", "test@example.com"}},
		{[]string{"git", "-C", workDir, "config", "user.name", "Test"}},
		{[]string{"git", "-C", workDir, "remote", "add", "origin", "https://github.com/acme/widgetrepo.git"}},
	} {
		cmd := exec.Command(tc.args[0], tc.args[1:]...)
		cmd.Env = cleanEnv
		require.NoError(t, cmd.Run(), "setup: %v", tc.args)
	}

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "myplugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	// Plugin must land under the XDG convention path for this project.
	expectedConventionRoot := filepath.Join(xdgDataHome, "ddx", "projects", "github.com", "acme", "widgetrepo")
	xdgPluginDir := filepath.Join(expectedConventionRoot, "plugins", "myplugin")
	pluginInfo, pluginStatErr := os.Lstat(xdgPluginDir)
	require.NoError(t, pluginStatErr, "XDG convention plugin dir must exist at %s", xdgPluginDir)
	assert.True(t, pluginInfo.Mode()&os.ModeSymlink != 0, "convention plugin dir must be a symlink")

	// Project-tier skill links must be under the project dir (not in the convention root).
	for _, surface := range []string{".agents/skills", ".claude/skills"} {
		skillLink := filepath.Join(workDir, surface, "myplugin-skill")
		_, skillErr := os.Lstat(skillLink)
		require.NoError(t, skillErr, "%s must exist for convention install", skillLink)

		// Skill link must resolve into the XDG plugins path (following the symlink).
		target, readErr := os.Readlink(skillLink)
		require.NoError(t, readErr, "%s must be a symlink", skillLink)
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(skillLink), target)
		}
		target, _ = filepath.Abs(target)
		// The XDG plugin dir is a symlink to localPlugin; skill links must resolve into
		// the same source that the XDG plugin dir points to.
		xdgTarget, xdgReadErr := os.Readlink(xdgPluginDir)
		require.NoError(t, xdgReadErr, "XDG plugin dir must be a symlink")
		if !filepath.IsAbs(xdgTarget) {
			xdgTarget = filepath.Join(filepath.Dir(xdgPluginDir), xdgTarget)
		}
		xdgTarget, _ = filepath.Abs(xdgTarget)
		assert.True(t, strings.HasPrefix(target, xdgTarget),
			"%s must resolve into the XDG plugin source (%s); got %s", skillLink, xdgTarget, target)
	}

	// Home directory must not be polluted.
	_, noAgentErr := os.Lstat(filepath.Join(homeDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "home .agents/skills must not be created for convention install")
	_, noClaudeErr := os.Lstat(filepath.Join(homeDir, ".claude", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noClaudeErr), "home .claude/skills must not be created for convention install")
}

// TestInstallLocal_InTreeMode_WritesOverlayAndLinks verifies that
// `ddx plugin install <name> --local` with <project>/.ddx/ present writes a
// project-local overlay symlink and links project-tier skills.
func TestInstallLocal_InTreeMode_WritesOverlayAndLinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "myplugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	pluginDir := filepath.Join(workDir, ddxroot.DirName, "plugins", "myplugin")
	_, statErr := os.Lstat(pluginDir)
	require.NoError(t, statErr, "in-tree plugin overlay must exist at %s", pluginDir)

	for _, surface := range []string{".agents/skills", ".claude/skills"} {
		skillLink := filepath.Join(workDir, surface, "myplugin-skill")
		_, skillErr := os.Lstat(skillLink)
		require.NoError(t, skillErr, "%s must exist for in-tree install", skillLink)
	}

	_, noAgentErr := os.Lstat(filepath.Join(homeDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "home .agents/skills must not be created for in-tree install")
	_, noClaudeErr := os.Lstat(filepath.Join(homeDir, ".claude", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noClaudeErr), "home .claude/skills must not be created for in-tree install")
}

// TestInstallLocal_ConventionMode_WritesXdgOverlayAndLinks verifies that
// `ddx plugin install <name> --local` with no <project>/.ddx/ writes an XDG
// project overlay symlink and links project-tier skills.
func TestInstallLocal_ConventionMode_WritesXdgOverlayAndLinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "myplugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	conventionRoot := ddxroot.Path(context.Background(), workDir)
	assert.True(t, strings.HasPrefix(conventionRoot, xdgDataHome),
		"convention root must be under XDG_DATA_HOME; got %s", conventionRoot)

	conventionPluginDir := filepath.Join(conventionRoot, "plugins", "myplugin")
	_, statErr := os.Lstat(conventionPluginDir)
	require.NoError(t, statErr, "convention plugin overlay must exist at %s", conventionPluginDir)

	for _, surface := range []string{".agents/skills", ".claude/skills"} {
		skillLink := filepath.Join(workDir, surface, "myplugin-skill")
		_, skillErr := os.Lstat(skillLink)
		require.NoError(t, skillErr, "%s must exist for convention install", skillLink)
	}

	_, noAgentErr := os.Lstat(filepath.Join(homeDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "home .agents/skills must not be created for convention install")
	_, noClaudeErr := os.Lstat(filepath.Join(homeDir, ".claude", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noClaudeErr), "home .claude/skills must not be created for convention install")
}
