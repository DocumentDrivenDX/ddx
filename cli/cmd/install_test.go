package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInstall_InTreeWritesProjectTreeAndLinks verifies AC1:
// With an in-tree <project>/.ddx/, ddx install <name> (no --global) writes to
// <project>/.ddx/plugins/<name>/ and creates project-tier links under
// <project>/.claude/skills/<name> and <project>/.agents/skills/<name>
// resolving into that plugins/ dir.
func TestInstall_InTreeWritesProjectTreeAndLinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Force in-tree mode by pre-creating .ddx/.
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "myplugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	// Plugin must land in the project in-tree location.
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

// TestInstall_ConventionModeWritesXDGProjectTree verifies AC2:
// With no <project>/.ddx/, ddx install <name> (no --global) writes to
// ${XDG_DATA_HOME}/ddx/projects/<host>/<owner>/<repo>/plugins/<name>/ and the
// project agent-tier links point into that XDG plugins/ path.
// A git repo with a deterministic remote URL is used so host/owner/repo are stable.
func TestInstall_ConventionModeWritesXDGProjectTree(t *testing.T) {
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
	output, err := executeCommand(factory.NewRootCommand(), "install", "myplugin", "--local", localPlugin, "--force")
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
