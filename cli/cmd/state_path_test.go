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

func TestCommandStatePathsUseDDxRoot(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "demo-project")
	initProjectWorktreeTestRepo(t, projectRoot)

	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	expectedRoot := ddxroot.Path(context.Background(), projectRoot)
	expectedConfigPath := filepath.Join(expectedRoot, "config.yaml")
	configData := []byte(`version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
persona_bindings:
  author: "Command Test"
`)
	require.NoError(t, os.WriteFile(expectedConfigPath, configData, 0o644))

	subdir := filepath.Join(projectRoot, "nested", "dir")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	assert.Equal(t, expectedConfigPath, commandStatePath(subdir, "config.yaml"))
	assert.Equal(t, expectedConfigPath, configGetPath(subdir, false))

	files := configListFiles(subdir)
	require.NotEmpty(t, files)
	assert.Equal(t, expectedConfigPath, files[0].Path)
	assert.True(t, files[0].Exists)

	output, err := executeCommand(NewCommandFactory(subdir).NewRootCommand(), "config", "export")
	require.NoError(t, err)
	assert.Contains(t, output, "Command Test")
}
