package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildServerServiceConfigUsesXDGRuntimeDir(t *testing.T) {
	xdg := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", xdg)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-secret")

	projectRoot := filepath.Join(t.TempDir(), "project")
	cfg := buildServerServiceConfig("/opt/ddx/bin/ddx", projectRoot)

	require.Equal(t, projectRoot, cfg.ProjectRoot)
	assert.Equal(t, filepath.Join(xdg, "ddx", "server"), cfg.WorkDir)
	assert.Equal(t, filepath.Join(xdg, "ddx", "server", "ddx-server.log"), cfg.LogPath)
	assert.Equal(t, projectRoot, cfg.Env["DDX_PROJECT_ROOT"])
	assert.Equal(t, "anthropic-secret", cfg.Env["ANTHROPIC_API_KEY"])
	assert.Equal(t, strings.Join([]string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "bin"),
		"/opt/ddx/bin",
		"/usr/bin",
		"/bin",
	}, string(os.PathListSeparator)), cfg.Env["PATH"])
}
