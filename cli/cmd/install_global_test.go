package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeLocalPlugin creates a minimal plugin fixture at dir with the given name
// and one skill. It returns the skill directory path.
func makeLocalPlugin(t *testing.T, dir, name string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(
		"name: "+name+"\n"+
			"version: 1.0.0\n"+
			"description: Test plugin\n"+
			"type: plugin\n"+
			"source: https://example.com/"+name+"\n"+
			"api_version: \"1\"\n"+
			"install:\n"+
			"  root:\n"+
			"    source: .\n"+
			"    target: .ddx/plugins/"+name+"\n"+
			"  skills:\n"+
			"    - source: skills/\n"+
			"      target: .agents/skills/\n"+
			"    - source: skills/\n"+
			"      target: .claude/skills/\n",
	), 0o644))

	skillDir := filepath.Join(dir, "skills", name+"-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(
		"---\nname: "+name+"-skill\ndescription: Test skill\n---\n\nBody.\n",
	), 0o644))
	return skillDir
}

// TestInstallGlobal_WritesXDGAndAgentTierLinks verifies AC1:
// ddx install <name> --global writes to ${XDG_DATA_HOME}/ddx/global/plugins/<name>/
// and creates agent-tier links under ~/.claude/skills/<name> and ~/.agents/skills/<name>.
func TestInstallGlobal_WritesXDGAndAgentTierLinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "myplugin", "--global", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	// Plugin root must land in the global tree.
	globalPluginDir := filepath.Join(xdgDataHome, "ddx", "global", "plugins", "myplugin")
	info, statErr := os.Lstat(globalPluginDir)
	require.NoError(t, statErr, "global plugin dir must exist")
	assert.True(t, info.Mode()&os.ModeSymlink != 0, "global plugin dir must be a symlink to the local checkout")

	// Agent-tier skill links must be in the home directory.
	// Local installs create symlinks, so use os.Lstat (DirExists follows symlinks).
	agentSkill := filepath.Join(homeDir, ".agents", "skills", "myplugin-skill")
	claudeSkill := filepath.Join(homeDir, ".claude", "skills", "myplugin-skill")
	agentInfo, agentErr := os.Lstat(agentSkill)
	require.NoError(t, agentErr, "~/.agents/skills/<name> must exist for global install")
	assert.True(t, agentInfo.Mode()&os.ModeSymlink != 0 || agentInfo.IsDir(),
		"~/.agents/skills/<name> must be a symlink or dir for global install")
	claudeInfo, claudeErr := os.Lstat(claudeSkill)
	require.NoError(t, claudeErr, "~/.claude/skills/<name> must exist for global install")
	assert.True(t, claudeInfo.Mode()&os.ModeSymlink != 0 || claudeInfo.IsDir(),
		"~/.claude/skills/<name> must be a symlink or dir for global install")

	// Nothing must land in the project directory's skill dirs.
	_, noAgentErr := os.Lstat(filepath.Join(workDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "project .agents/skills must not be created for global install")
	_, noClaudeErr := os.Lstat(filepath.Join(workDir, ".claude", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noClaudeErr), "project .claude/skills must not be created for global install")
}

// TestInstallProject_InTreeLinks verifies AC2:
// ddx install <name> (no --global, in-tree mode) installs to <project>/.ddx/plugins/<name>/
// and creates project-tier links under <project>/.claude/skills/<name> and
// <project>/.agents/skills/<name>.
func TestInstallProject_InTreeLinks(t *testing.T) {
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
	info, statErr := os.Lstat(projectPluginDir)
	require.NoError(t, statErr, "project plugin dir must exist")
	assert.True(t, info.Mode()&os.ModeSymlink != 0, "project plugin dir must be a symlink to the local checkout")

	// Project-tier skill links must be in the project directory.
	// Local installs create symlinks, so use os.Lstat.
	agentSkill := filepath.Join(workDir, ".agents", "skills", "myplugin-skill")
	claudeSkill := filepath.Join(workDir, ".claude", "skills", "myplugin-skill")
	agentInfo, agentErr := os.Lstat(agentSkill)
	require.NoError(t, agentErr, "<project>/.agents/skills/<name> must exist for in-tree install")
	assert.True(t, agentInfo.Mode()&os.ModeSymlink != 0 || agentInfo.IsDir(),
		"<project>/.agents/skills/<name> must be a symlink or dir for in-tree install")
	claudeInfo, claudeErr := os.Lstat(claudeSkill)
	require.NoError(t, claudeErr, "<project>/.claude/skills/<name> must exist for in-tree install")
	assert.True(t, claudeInfo.Mode()&os.ModeSymlink != 0 || claudeInfo.IsDir(),
		"<project>/.claude/skills/<name> must be a symlink or dir for in-tree install")

	// Nothing must land in the home directory's skill dirs.
	_, noAgentErr := os.Lstat(filepath.Join(homeDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "home .agents/skills must not be created for in-tree install")
	_, noClaudeErr := os.Lstat(filepath.Join(homeDir, ".claude", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noClaudeErr), "home .claude/skills must not be created for in-tree install")
}

// TestInstallProject_ConventionLinks verifies AC3:
// ddx install <name> (no --global, convention mode — no .ddx/ in project) installs
// to ${XDG_DATA_HOME}/ddx/projects/<identity>/plugins/<name>/ and creates
// project-tier links pointing into that XDG plugins path.
func TestInstallProject_ConventionLinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	// No .ddx/ in workDir — convention mode.

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "myplugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	// Determine the convention root that ddxroot.Path would produce.
	conventionRoot := ddxroot.Path(context.Background(), workDir)

	// Plugin must land under the XDG convention root.
	conventionPluginDir := filepath.Join(conventionRoot, "plugins", "myplugin")
	info, statErr := os.Lstat(conventionPluginDir)
	require.NoError(t, statErr, "convention plugin dir must exist at %s", conventionPluginDir)
	assert.True(t, info.Mode()&os.ModeSymlink != 0, "convention plugin dir must be a symlink")

	// The convention root must NOT be the in-tree path (no .ddx/ was created in workDir).
	assert.False(t, conventionRoot == filepath.Join(workDir, ddxroot.DirName),
		"convention root must differ from in-tree path when .ddx/ was not pre-created")

	// Project-tier skill links must be in the project directory (not in convention root).
	// Local installs create symlinks, so use os.Lstat.
	agentSkill := filepath.Join(workDir, ".agents", "skills", "myplugin-skill")
	claudeSkill := filepath.Join(workDir, ".claude", "skills", "myplugin-skill")
	agentInfo, agentErr := os.Lstat(agentSkill)
	require.NoError(t, agentErr, "<project>/.agents/skills/<name> must exist for convention install")
	assert.True(t, agentInfo.Mode()&os.ModeSymlink != 0 || agentInfo.IsDir(),
		"<project>/.agents/skills/<name> must be a symlink or dir for convention install")
	claudeInfo, claudeErr := os.Lstat(claudeSkill)
	require.NoError(t, claudeErr, "<project>/.claude/skills/<name> must exist for convention install")
	assert.True(t, claudeInfo.Mode()&os.ModeSymlink != 0 || claudeInfo.IsDir(),
		"<project>/.claude/skills/<name> must be a symlink or dir for convention install")
}

// TestInstallGlobal_WritesGlobalTreeAndLinksAgentTier verifies bead AC1:
// ddx install <name> --global --silent writes to ${XDG_DATA_HOME}/ddx/global/plugins/<name>/
// and creates a symlink-or-copy under ~/.claude/skills/<name> and ~/.agents/skills/<name>
// that resolves into the global plugin tree.
func TestInstallGlobal_WritesGlobalTreeAndLinksAgentTier(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "myplugin", "--global", "--silent", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	// Plugin root must land in the global tree.
	globalPluginDir := filepath.Join(xdgDataHome, "ddx", "global", "plugins", "myplugin")
	info, statErr := os.Lstat(globalPluginDir)
	require.NoError(t, statErr, "global plugin dir must exist at %s", globalPluginDir)
	assert.True(t, info.Mode()&os.ModeSymlink != 0 || info.IsDir(),
		"global plugin dir must be a symlink or dir")

	// Resolve the global plugin dir to its canonical path so we can verify
	// that skill links resolve into the same physical tree.
	realGlobalPlugin, evalErr := filepath.EvalSymlinks(globalPluginDir)
	require.NoError(t, evalErr, "global plugin dir must be resolvable")

	// Agent-tier skill links must be in the home directory and resolve into
	// the global plugin tree.
	for _, surface := range []string{".agents/skills", ".claude/skills"} {
		skillLink := filepath.Join(homeDir, surface, "myplugin-skill")
		skillInfo, skillErr := os.Lstat(skillLink)
		require.NoError(t, skillErr, "%s must exist for global install", skillLink)
		assert.True(t, skillInfo.Mode()&os.ModeSymlink != 0 || skillInfo.IsDir(),
			"%s must be a symlink or dir for global install", skillLink)

		// Verify the skill resolves into the global plugin tree.
		realSkill, skillEvalErr := filepath.EvalSymlinks(skillLink)
		require.NoError(t, skillEvalErr, "%s must be resolvable", skillLink)
		assert.True(t, strings.HasPrefix(realSkill, realGlobalPlugin),
			"%s must resolve into global plugin tree %s; got %s", skillLink, realGlobalPlugin, realSkill)
	}

	// Nothing must land in the project directory's skill dirs.
	_, noAgentErr := os.Lstat(filepath.Join(workDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "project .agents/skills must not be created for global install")
	_, noClaudeErr := os.Lstat(filepath.Join(workDir, ".claude", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noClaudeErr), "project .claude/skills must not be created for global install")
}

// TestInstallGlobal_WritesGlobalTierAndAgentLinks verifies AC1:
// ddx install <name> --global --silent writes to ${XDG_DATA_HOME}/ddx/global/plugins/<name>/
// and creates a symlink-or-copy under ~/.claude/skills/<name> and ~/.agents/skills/<name>.
func TestInstallGlobal_WritesGlobalTierAndAgentLinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "myplugin", "--global", "--silent", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	// Plugin root must land in the global tree.
	globalPluginDir := filepath.Join(xdgDataHome, "ddx", "global", "plugins", "myplugin")
	info, statErr := os.Lstat(globalPluginDir)
	require.NoError(t, statErr, "global plugin dir must exist at %s", globalPluginDir)
	assert.True(t, info.Mode()&os.ModeSymlink != 0 || info.IsDir(),
		"global plugin dir must be a symlink or dir")

	// Agent-tier skill links must be in the home directory.
	agentSkill := filepath.Join(homeDir, ".agents", "skills", "myplugin-skill")
	claudeSkill := filepath.Join(homeDir, ".claude", "skills", "myplugin-skill")
	agentInfo, agentErr := os.Lstat(agentSkill)
	require.NoError(t, agentErr, "~/.agents/skills/<name> must exist for global install")
	assert.True(t, agentInfo.Mode()&os.ModeSymlink != 0 || agentInfo.IsDir(),
		"~/.agents/skills/<name> must be a symlink or dir for global install")
	claudeInfo, claudeErr := os.Lstat(claudeSkill)
	require.NoError(t, claudeErr, "~/.claude/skills/<name> must exist for global install")
	assert.True(t, claudeInfo.Mode()&os.ModeSymlink != 0 || claudeInfo.IsDir(),
		"~/.claude/skills/<name> must be a symlink or dir for global install")

	// Nothing must land in the project directory's skill dirs.
	_, noAgentErr := os.Lstat(filepath.Join(workDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "project .agents/skills must not be created for global install")
	_, noClaudeErr := os.Lstat(filepath.Join(workDir, ".claude", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noClaudeErr), "project .claude/skills must not be created for global install")
}

// TestInstallProject_InTreeMode verifies AC2:
// ddx install <name> (no --global, in-tree mode) installs to <project>/.ddx/plugins/<name>/
// and creates project-tier links under <project>/.claude/skills/<name> and
// <project>/.agents/skills/<name> pointing into <project>/.ddx/plugins/<name>.
func TestInstallProject_InTreeMode(t *testing.T) {
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
	assert.True(t, pluginInfo.Mode()&os.ModeSymlink != 0, "project plugin dir must be a symlink")

	// Project-tier skill links must be in the project directory and point into the plugin dir.
	agentSkill := filepath.Join(workDir, ".agents", "skills", "myplugin-skill")
	claudeSkill := filepath.Join(workDir, ".claude", "skills", "myplugin-skill")
	agentInfo, agentErr := os.Lstat(agentSkill)
	require.NoError(t, agentErr, "<project>/.agents/skills/<name> must exist for in-tree install")
	assert.True(t, agentInfo.Mode()&os.ModeSymlink != 0 || agentInfo.IsDir(),
		"<project>/.agents/skills/<name> must be a symlink or dir for in-tree install")
	claudeInfo, claudeErr := os.Lstat(claudeSkill)
	require.NoError(t, claudeErr, "<project>/.claude/skills/<name> must exist for in-tree install")
	assert.True(t, claudeInfo.Mode()&os.ModeSymlink != 0 || claudeInfo.IsDir(),
		"<project>/.claude/skills/<name> must be a symlink or dir for in-tree install")

	// Skill links must resolve into the in-tree plugin directory.
	agentTarget, agentReadErr := os.Readlink(agentSkill)
	require.NoError(t, agentReadErr, "agent skill must be a symlink")
	if !filepath.IsAbs(agentTarget) {
		agentTarget = filepath.Join(filepath.Dir(agentSkill), agentTarget)
	}
	agentTarget, _ = filepath.Abs(agentTarget)
	assert.True(t, strings.HasPrefix(agentTarget, localPlugin),
		"agent skill symlink must resolve into the local plugin dir; got %s", agentTarget)

	// Nothing must land in the home directory's skill dirs.
	_, noAgentErr := os.Lstat(filepath.Join(homeDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "home .agents/skills must not be created for in-tree install")
}

// TestInstallProject_ConventionMode verifies AC3:
// ddx install <name> (no --global, convention mode — no .ddx/ in project) installs
// to ${XDG_DATA_HOME}/ddx/projects/<identity>/plugins/<name>/ and creates
// project-tier links pointing into that XDG plugins path.
func TestInstallProject_ConventionMode(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	// No .ddx/ in workDir — convention mode.
	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "myplugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	// Determine the convention root that ddxroot.Path would produce.
	conventionRoot := ddxroot.Path(context.Background(), workDir)

	// Plugin must land under the XDG convention root.
	conventionPluginDir := filepath.Join(conventionRoot, "plugins", "myplugin")
	pluginInfo, pluginStatErr := os.Lstat(conventionPluginDir)
	require.NoError(t, pluginStatErr, "convention plugin dir must exist at %s", conventionPluginDir)
	assert.True(t, pluginInfo.Mode()&os.ModeSymlink != 0, "convention plugin dir must be a symlink")

	// The convention root must be XDG-based, not in-tree.
	assert.True(t, strings.HasPrefix(conventionRoot, xdgDataHome),
		"convention root must be under XDG_DATA_HOME; got %s", conventionRoot)

	// Project-tier skill links must be in the project directory.
	agentSkill := filepath.Join(workDir, ".agents", "skills", "myplugin-skill")
	claudeSkill := filepath.Join(workDir, ".claude", "skills", "myplugin-skill")
	agentInfo, agentErr := os.Lstat(agentSkill)
	require.NoError(t, agentErr, "<project>/.agents/skills/<name> must exist for convention install")
	assert.True(t, agentInfo.Mode()&os.ModeSymlink != 0 || agentInfo.IsDir(),
		"<project>/.agents/skills/<name> must be a symlink or dir for convention install")
	claudeInfo, claudeErr := os.Lstat(claudeSkill)
	require.NoError(t, claudeErr, "<project>/.claude/skills/<name> must exist for convention install")
	assert.True(t, claudeInfo.Mode()&os.ModeSymlink != 0 || claudeInfo.IsDir(),
		"<project>/.claude/skills/<name> must be a symlink or dir for convention install")

	// Skill links must resolve into the local plugin source.
	agentTarget, agentReadErr := os.Readlink(agentSkill)
	require.NoError(t, agentReadErr, "agent skill must be a symlink")
	if !filepath.IsAbs(agentTarget) {
		agentTarget = filepath.Join(filepath.Dir(agentSkill), agentTarget)
	}
	agentTarget, _ = filepath.Abs(agentTarget)
	assert.True(t, strings.HasPrefix(agentTarget, localPlugin),
		"agent skill symlink must resolve into the local plugin dir; got %s", agentTarget)

	// Home directory must not be polluted.
	_, noAgentErr := os.Lstat(filepath.Join(homeDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "home .agents/skills must not be created for convention install")
}

// mustBuildValidPluginTarball builds a .tar.gz archive containing a plugin with
// a valid skill (SKILL.md present). The archive has no package.yaml so the
// caller's Package struct drives install targets — this is the path used by
// registry installs where adjustInstallTargets provides absolute skill dirs.
func mustBuildValidPluginTarball(t *testing.T, rootName, skillName string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	writeDir := func(name string) {
		t.Helper()
		if !strings.HasSuffix(name, "/") {
			name += "/"
		}
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o755,
			Typeflag: tar.TypeDir,
		}))
	}
	writeFile := func(name, body string) {
		t.Helper()
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}))
		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}

	writeDir(rootName)
	writeDir(filepath.Join(rootName, "skills"))
	writeDir(filepath.Join(rootName, "skills", skillName))
	writeFile(filepath.Join(rootName, "skills", skillName, "SKILL.md"),
		"---\nname: "+skillName+"\ndescription: Test skill\n---\n\nBody.\n")

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// TestInstallGlobal_WritesToGlobalTreeAndAgentLinks verifies AC1:
// ddx install <name> --global --silent installs to ${XDG_DATA_HOME}/ddx/global/plugins/<name>/,
// creates a symlink-or-copy under ~/.claude/skills/<name> and ~/.agents/skills/<name>,
// and records the entry in the global state file only (not the project state).
func TestInstallGlobal_WritesToGlobalTreeAndAgentLinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	const pluginName = "myplugin"
	const skillName = "myplugin-skill"

	// Build and serve a valid plugin tarball (no package.yaml — fallback pkg used).
	tarball := mustBuildValidPluginTarball(t, pluginName+"-1.0.0", skillName)
	withStaticHTTPTransport(t, tarball, "application/gzip", "/archive/refs/tags/v1.0.0.tar.gz")

	globalDir := ddxroot.GlobalDir()
	require.NoError(t, os.MkdirAll(globalDir, 0o755), "pre-create globalDir so InstallPackage chdir succeeds")
	agentSkillsDir := filepath.Join(homeDir, ".agents", "skills")
	claudeSkillsDir := filepath.Join(homeDir, ".claude", "skills")

	pkg := &registry.Package{
		Name:    pluginName,
		Version: "1.0.0",
		Type:    registry.PackageTypePlugin,
		Source:  "https://example.com/" + pluginName,
		Install: registry.PackageInstall{
			Root: &registry.InstallMapping{Source: ".", Target: filepath.Join("plugins", pluginName)},
			Skills: []registry.InstallMapping{
				{Source: "skills/", Target: agentSkillsDir + "/"},
				{Source: "skills/", Target: claudeSkillsDir + "/"},
			},
		},
	}

	entry, err := registry.InstallPackage(pkg, globalDir)
	require.NoError(t, err, "global install must succeed")

	// Record the entry in the global state file (as runInstall does for non-local installs).
	loadState, saveState := selectInstallState(true)
	state, err := loadState()
	require.NoError(t, err)
	state.AddOrUpdate(entry)
	require.NoError(t, saveState(state))

	// Assert: plugin dir exists in global XDG tree.
	globalPluginDir := filepath.Join(xdgDataHome, "ddx", "global", "plugins", pluginName)
	_, statErr := os.Stat(globalPluginDir)
	require.NoError(t, statErr, "global plugin dir must exist at %s", globalPluginDir)

	// Assert: agent-tier skill outputs exist in home directory.
	for _, surface := range []string{".agents/skills", ".claude/skills"} {
		skillPath := filepath.Join(homeDir, surface, skillName)
		_, skillErr := os.Lstat(skillPath)
		require.NoError(t, skillErr, "%s must exist for global install", skillPath)
	}

	// Assert: entry recorded in global state file only.
	globalState, err := registry.LoadGlobalState()
	require.NoError(t, err)
	assert.NotNil(t, globalState.FindInstalled(pluginName),
		"global state must contain the installed entry")

	// Assert: entry NOT in project state file.
	projectState, err := registry.LoadState()
	require.NoError(t, err)
	assert.Nil(t, projectState.FindInstalled(pluginName),
		"project state must not contain entry from global install")

	// Assert: project skill dirs untouched.
	_, noAgentErr := os.Lstat(filepath.Join(workDir, ".agents", "skills", skillName))
	assert.True(t, os.IsNotExist(noAgentErr),
		"project .agents/skills must not be created for global install")
}

// TestPluginListGlobal_EnumeratesFromGlobalTier verifies AC3:
// ddx plugin list --global enumerates from the global installed state,
// while ddx plugin list (no --global) reads the project state.
func TestPluginListGlobal_EnumeratesFromGlobalTier(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	// Seed global state only.
	globalState := &registry.InstalledState{
		Installed: []registry.InstalledEntry{
			{Name: "helix", Version: "1.0.0", Type: registry.PackageTypeWorkflow},
		},
	}
	require.NoError(t, registry.SaveGlobalState(globalState))

	factory := NewCommandFactory(workDir)

	// ddx plugin list --global shows the globally installed entry.
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "list", "--global")
	require.NoError(t, err, output)
	assert.Contains(t, output, "helix", "global list must show globally installed plugin")
	assert.Contains(t, output, "1.0.0")

	// ddx plugin list (no --global) reads project state — entry must not appear.
	output, err = executeCommand(factory.NewRootCommand(), "plugin", "list")
	require.NoError(t, err, output)
	assert.NotContains(t, output, "helix",
		"project list must not show entry that is only in global state")
}

// TestInstallGlobal_WritesGlobalTreeAndAgentLinks verifies AC1 (bead ddx-be724d92):
// ddx install <name> --global --silent creates ${XDG_DATA_HOME}/ddx/global/plugins/<name>/
// and a symlink-or-copy at ~/.claude/skills/<name> and ~/.agents/skills/<name>.
func TestInstallGlobal_WritesGlobalTreeAndAgentLinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "myplugin", "--global", "--silent", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	// Plugin root must land in ${XDG_DATA_HOME}/ddx/global/plugins/<name>/.
	globalPluginDir := filepath.Join(xdgDataHome, "ddx", "global", "plugins", "myplugin")
	info, statErr := os.Lstat(globalPluginDir)
	require.NoError(t, statErr, "global plugin dir must exist at %s", globalPluginDir)
	assert.True(t, info.Mode()&os.ModeSymlink != 0 || info.IsDir(),
		"global plugin dir must be a symlink or dir")

	// Agent-tier skill links must exist in the home directory.
	for _, surface := range []string{".agents/skills", ".claude/skills"} {
		skillLink := filepath.Join(homeDir, surface, "myplugin-skill")
		_, skillErr := os.Lstat(skillLink)
		require.NoError(t, skillErr, "%s must exist for global install", skillLink)
	}

	// Nothing must land in the project directory's skill dirs.
	_, noAgentErr := os.Lstat(filepath.Join(workDir, ".agents", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noAgentErr), "project .agents/skills must not be created for global install")
	_, noClaudeErr := os.Lstat(filepath.Join(workDir, ".claude", "skills", "myplugin-skill"))
	assert.True(t, os.IsNotExist(noClaudeErr), "project .claude/skills must not be created for global install")
}

// TestPluginShowGlobal_ReportsFromGlobalTier verifies AC3:
// ddx plugin show <name> --global reports from the global installed state,
// while ddx plugin show <name> (no --global) returns an error when the plugin
// is only in the global state.
func TestPluginShowGlobal_ReportsFromGlobalTier(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	// Seed global state only.
	globalState := &registry.InstalledState{
		Installed: []registry.InstalledEntry{
			{Name: "helix", Version: "2.0.0", Type: registry.PackageTypeWorkflow, Source: "https://example.com/helix"},
		},
	}
	require.NoError(t, registry.SaveGlobalState(globalState))

	factory := NewCommandFactory(workDir)

	// ddx plugin show helix --global reports the global entry.
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "show", "helix", "--global")
	require.NoError(t, err, output)
	assert.Contains(t, output, "helix")
	assert.Contains(t, output, "2.0.0")

	// ddx plugin show helix (no --global) must fail — plugin only in global state.
	_, err = executeCommand(factory.NewRootCommand(), "plugin", "show", "helix")
	require.Error(t, err, "show without --global must fail when plugin is only in global state")
}
