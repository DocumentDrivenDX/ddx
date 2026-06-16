package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderDiscoveryGuard_RedirectsCacheToTempRoot(t *testing.T) {
	root := installCmdTestProviderDiscoveryGuard(t)

	cacheDir := os.Getenv("FIZEAU_CACHE_DIR")
	require.NotEmpty(t, cacheDir)
	assert.True(t, pathWithin(root, cacheDir), "FIZEAU_CACHE_DIR must live under guard root: %s", cacheDir)
	assert.True(t, pathWithin(root, os.Getenv("XDG_CACHE_HOME")), "XDG_CACHE_HOME must live under guard root")
	for _, key := range []string{"FIZEAU_CLAUDE_QUOTA_CACHE", "FIZEAU_CODEX_QUOTA_CACHE", "FIZEAU_GEMINI_QUOTA_CACHE"} {
		assert.True(t, pathWithin(root, os.Getenv(key)), "%s must live under guard root: %s", key, os.Getenv(key))
	}

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	realDiscovery := filepath.Join(home, ".cache", "fizeau", "discovery")
	assert.False(t, pathWithin(realDiscovery, filepath.Join(cacheDir, "discovery")),
		"guarded discovery cache must not resolve under the real user cache")
}

func TestProviderDiscoveryGuard_FakeProviderCLIsOnPath(t *testing.T) {
	root := installCmdTestProviderDiscoveryGuard(t)

	for _, name := range []string{"codex", "claude", "gemini"} {
		got, err := exec.LookPath(name)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(root, "bin", name), got)
	}
}

func pathWithin(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	return err == nil && rel != "." && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
