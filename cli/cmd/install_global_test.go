package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestPluginInstallGlobalFlagRetired(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	xdgDataHome := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "myplugin", "--global", "--local", localPlugin, "--force")
	require.Error(t, err, output)
	assert.Contains(t, err.Error(), "global plugin installs are retired")
	assert.Contains(t, err.Error(), "ddx plugin install <name>")
	assert.NoDirExists(t, filepath.Join(xdgDataHome, "ddx", "global", "plugins", "myplugin"))
	assert.NoDirExists(t, filepath.Join(homeDir, ".agents", "skills", "myplugin-skill"))
	assert.NoDirExists(t, filepath.Join(homeDir, ".claude", "skills", "myplugin-skill"))
	assert.NoDirExists(t, filepath.Join(workDir, ".agents", "skills", "myplugin-skill"))
	assert.NoDirExists(t, filepath.Join(workDir, ".claude", "skills", "myplugin-skill"))
}

func TestPluginListShowGlobalFlagsRetired(t *testing.T) {
	factory := NewCommandFactory(t.TempDir())

	output, err := executeCommand(factory.NewRootCommand(), "plugin", "list", "--global")
	require.Error(t, err, output)
	assert.Contains(t, err.Error(), "global plugin installs are retired")

	output, err = executeCommand(factory.NewRootCommand(), "plugin", "show", "helix", "--global")
	require.Error(t, err, output)
	assert.Contains(t, err.Error(), "global plugin installs are retired")
}

func TestPluginInstallLocal_InTreeOverlayStillWorks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	localPlugin := t.TempDir()
	makeLocalPlugin(t, localPlugin, "myplugin")

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "myplugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	assertLocalSymlink(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "myplugin"), localPlugin)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "myplugin-skill"), filepath.Join(localPlugin, "skills", "myplugin-skill"))
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "myplugin-skill"), filepath.Join(localPlugin, "skills", "myplugin-skill"))
	assert.NoDirExists(t, filepath.Join(homeDir, ".agents", "skills", "myplugin-skill"))
	assert.NoDirExists(t, filepath.Join(homeDir, ".claude", "skills", "myplugin-skill"))
}

func TestPluginInstallLocal_ConventionOverlayStillWorks(t *testing.T) {
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
	assert.NotEqual(t, filepath.Join(workDir, ddxroot.DirName), conventionRoot)
	assertLocalSymlink(t, filepath.Join(conventionRoot, "plugins", "myplugin"), localPlugin)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "myplugin-skill"), filepath.Join(localPlugin, "skills", "myplugin-skill"))
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "myplugin-skill"), filepath.Join(localPlugin, "skills", "myplugin-skill"))
}
