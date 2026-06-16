package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginListJSONIncludesLockAndLocalOverlay(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()
	seedInTreeDDxRoot(t, workDir)

	cacheDir := filepath.Join(t.TempDir(), "cache", "helix")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))
	for _, rel := range []string{".agents/skills/helix", ".claude/skills/helix"} {
		require.NoError(t, os.MkdirAll(filepath.Join(workDir, filepath.FromSlash(rel)), 0o755))
	}
	lock := &registry.PluginLock{Plugins: []registry.PluginLockEntry{{
		Name:           "helix",
		Version:        "1.2.3",
		Type:           registry.PackageTypePlugin,
		Source:         "https://github.com/DocumentDrivenDX/helix",
		CachePath:      cacheDir,
		InstalledAt:    time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC),
		GeneratedFiles: []string{".agents/skills/helix", ".claude/skills/helix"},
	}}}
	require.NoError(t, registry.SaveProjectPluginLock(ctx, workDir, lock))

	localRoot := t.TempDir()
	overlayPath := filepath.Join(workDir, ddxroot.DirName, "plugins", "local-ui")
	require.NoError(t, os.MkdirAll(filepath.Dir(overlayPath), 0o755))
	require.NoError(t, os.Symlink(localRoot, overlayPath))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "list", "--json")
	require.NoError(t, err, output)

	var entries []projectPluginListEntry
	require.NoError(t, json.Unmarshal([]byte(output), &entries))
	require.Len(t, entries, 3)
	assert.Equal(t, "ddx", entries[0].Name)
	assert.Equal(t, "plugin", entries[0].Type)
	assert.False(t, entries[0].LocalOverlay)
	assert.Equal(t, "helix", entries[1].Name)
	assert.Equal(t, "ok", entries[1].Status)
	assert.False(t, entries[1].LocalOverlay)
	assert.Equal(t, "local-ui", entries[2].Name)
	assert.Equal(t, "local-overlay", entries[2].Status)
	assert.True(t, entries[2].LocalOverlay)
	assert.Equal(t, localRoot, entries[2].Path)
}

func TestPluginUninstallLocalOverlayKeepsRegistryPin(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()
	seedInTreeDDxRoot(t, workDir)

	lock := &registry.PluginLock{Plugins: []registry.PluginLockEntry{{
		Name:           "helix",
		Version:        "1.2.3",
		Type:           registry.PackageTypePlugin,
		Source:         "https://github.com/DocumentDrivenDX/helix",
		GeneratedFiles: []string{".agents/skills/helix", ".claude/skills/helix"},
	}}}
	require.NoError(t, registry.SaveProjectPluginLock(ctx, workDir, lock))

	localRoot := t.TempDir()
	skillRoot := filepath.Join(localRoot, "skills", "helix")
	require.NoError(t, os.MkdirAll(skillRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: helix\ndescription: local\n---\n"), 0o644))
	overlayPath := filepath.Join(workDir, ddxroot.DirName, "plugins", "helix")
	require.NoError(t, os.MkdirAll(filepath.Dir(overlayPath), 0o755))
	require.NoError(t, os.Symlink(localRoot, overlayPath))
	for _, surface := range []string{".agents/skills", ".claude/skills"} {
		dst := filepath.Join(workDir, filepath.FromSlash(surface), "helix")
		require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
		rel, err := filepath.Rel(filepath.Dir(dst), skillRoot)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(rel, dst))
	}

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "uninstall", "helix")
	require.NoError(t, err, output)
	assert.Contains(t, output, "Uninstalled local overlay helix")

	assert.NoFileExists(t, overlayPath)
	assert.NoFileExists(t, filepath.Join(workDir, ".agents", "skills", "helix"))
	assert.NoFileExists(t, filepath.Join(workDir, ".claude", "skills", "helix"))
	assert.DirExists(t, localRoot)

	after, err := registry.LoadProjectPluginLock(ctx, workDir)
	require.NoError(t, err)
	require.NotNil(t, after.Find("helix"), "local overlay uninstall must leave the registry pin intact")
}

func TestPluginUninstallRegistryRemovesGeneratedAdaptersAndLock(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()
	seedInTreeDDxRoot(t, workDir)

	for _, rel := range []string{".agents/skills/helix", ".claude/skills/helix"} {
		dir := filepath.Join(workDir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: helix\ndescription: generated\n---\n"), 0o644))
	}
	lock := &registry.PluginLock{Plugins: []registry.PluginLockEntry{{
		Name:           "helix",
		Version:        "1.2.3",
		Type:           registry.PackageTypePlugin,
		Source:         "https://github.com/DocumentDrivenDX/helix",
		GeneratedFiles: []string{".agents/skills/helix", ".claude/skills/helix"},
	}}}
	require.NoError(t, registry.SaveProjectPluginLock(ctx, workDir, lock))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "uninstall", "helix")
	require.NoError(t, err, output)
	assert.Contains(t, output, "Uninstalled helix")

	assert.NoDirExists(t, filepath.Join(workDir, ".agents", "skills", "helix"))
	assert.NoDirExists(t, filepath.Join(workDir, ".claude", "skills", "helix"))
	after, err := registry.LoadProjectPluginLock(ctx, workDir)
	require.NoError(t, err)
	assert.Nil(t, after.Find("helix"))
}

func seedInTreeDDxRoot(t *testing.T, workDir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, ddxroot.DirName, "config.yaml"), []byte("version: \"1.0\"\n"), 0o644))
}
