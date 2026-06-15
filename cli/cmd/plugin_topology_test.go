package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDDxInstall_TopLevelCommandRetired(t *testing.T) {
	factory := NewCommandFactory(t.TempDir())

	output, err := executeCommand(factory.NewRootCommand(), "install", "helix")
	require.Error(t, err, output)
	assert.Contains(t, err.Error(), "ddx install has been retired")
	assert.Contains(t, err.Error(), "ddx plugin install helix")
}

func TestPluginInstallHelp_TeachesLockCacheAndGeneratedAdapters(t *testing.T) {
	factory := NewCommandFactory(t.TempDir())

	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "--help")
	require.NoError(t, err, output)
	assert.Contains(t, output, "plugins.lock.yaml")
	assert.Contains(t, output, "${XDG_DATA_HOME}/ddx/cache/plugins/<name>/<version>/")
	assert.Contains(t, output, "Generated adapters")
	assert.Contains(t, output, ".agents/skills/")
	assert.Contains(t, output, ".claude/skills/")
	assert.Contains(t, output, "ddx plugin sync")
	assert.NotContains(t, output, "ddx install helix")
	assert.NotContains(t, output, "--global")
}

func TestInitHelp_DoesNotClaimDefaultPluginCopy(t *testing.T) {
	factory := NewCommandFactory(t.TempDir())

	output, err := executeCommand(factory.NewRootCommand(), "init", "--help")
	require.NoError(t, err, output)
	assert.Contains(t, output, "ddx init .")
	assert.Contains(t, output, "Creates a .ddx/config.yaml configuration file")
	assert.Contains(t, output, "Writes DDx version metadata")
	assert.Contains(t, output, "Creates generated agent adapter files")
	assert.NotContains(t, output, "Installs the default DDx library plugin")
	assert.NotContains(t, output, "--global")
}
