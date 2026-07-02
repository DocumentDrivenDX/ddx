package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildServerServiceConfigUsesXDGRuntimeDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-secret")

	projectRoot := filepath.Join(t.TempDir(), "project")
	cfg := buildServerServiceConfig("/usr/local/bin/ddx", projectRoot)

	require.Equal(t, projectRoot, cfg.ProjectRoot)
	assert.Equal(t, filepath.Join(xdg, "ddx", "server"), cfg.WorkDir)
	assert.Equal(t, filepath.Join(xdg, "ddx", "server", "ddx-server.log"), cfg.LogPath)
	assert.Equal(t, projectRoot, cfg.Env["DDX_PROJECT_ROOT"])
	assert.Equal(t, "anthropic-secret", cfg.Env["ANTHROPIC_API_KEY"])
}
