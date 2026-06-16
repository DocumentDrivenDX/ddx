package registry

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
)

// makeInTreeRoot creates a projectRoot with an in-tree .ddx/ so ddxroot.Path()
// returns <projectRoot>/.ddx without attempting git bootstrapping.
func makeInTreeRoot(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)
	return projectRoot
}

func TestPluginLookup_RealProjectDirectoryDoesNotResolve(t *testing.T) {
	projectRoot := makeInTreeRoot(t)
	pluginName := "myplugin"

	// A real directory under .ddx/plugins is a stale legacy payload, not an
	// explicit local overlay. Only symlink overlays are project-authoritative.
	projectPlugin := filepath.Join(projectRoot, ddxroot.DirName, "plugins", pluginName)
	if err := os.MkdirAll(projectPlugin, 0o755); err != nil {
		t.Fatalf("mkdir project plugin: %v", err)
	}

	_, _, err := ResolvePlugin(context.Background(), projectRoot, pluginName)
	if err == nil {
		t.Fatalf("ResolvePlugin unexpectedly resolved stale real project plugin dir %q", projectPlugin)
	}
}

func TestPluginLookup_IgnoresLegacyGlobal(t *testing.T) {
	projectRoot := makeInTreeRoot(t)
	pluginName := "myplugin"

	// No project-layer copy.
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	globalPlugin := filepath.Join(xdg, "ddx", "global", "plugins", pluginName)
	if err := os.MkdirAll(globalPlugin, 0o755); err != nil {
		t.Fatalf("mkdir global plugin: %v", err)
	}

	_, _, err := ResolvePlugin(context.Background(), projectRoot, pluginName)
	if err == nil {
		t.Fatalf("ResolvePlugin unexpectedly resolved legacy global plugin %q", globalPlugin)
	}

	gotPath, gotLayer, err := ResolvePlugin(context.Background(), projectRoot, "ddx")
	if err != nil {
		t.Fatalf("ResolvePlugin(ddx baked-in): unexpected error: %v", err)
	}
	if gotLayer != "baked-in" {
		t.Errorf("layer = %q, want %q", gotLayer, "baked-in")
	}
	if gotPath != "" {
		t.Errorf("path = %q, want empty string for baked-in", gotPath)
	}
}

func TestPluginLookup_ResolvesProjectLockCache(t *testing.T) {
	projectRoot := makeInTreeRoot(t)
	pluginName := "myplugin"
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	cachePlugin := filepath.Join(xdg, "ddx", "cache", "plugins", pluginName, "1.2.3")
	if err := os.MkdirAll(cachePlugin, 0o755); err != nil {
		t.Fatalf("mkdir cache plugin: %v", err)
	}
	lock := &PluginLock{Plugins: []PluginLockEntry{{
		Name:      pluginName,
		Version:   "1.2.3",
		Type:      PackageTypePlugin,
		Source:    "https://example.com/myplugin",
		CachePath: cachePlugin,
	}}}
	if err := SaveProjectPluginLock(context.Background(), projectRoot, lock); err != nil {
		t.Fatalf("save plugin lock: %v", err)
	}

	gotPath, gotLayer, err := ResolvePlugin(context.Background(), projectRoot, pluginName)
	if err != nil {
		t.Fatalf("ResolvePlugin: unexpected error: %v", err)
	}
	if gotLayer != "cache" {
		t.Errorf("layer = %q, want %q", gotLayer, "cache")
	}
	if gotPath != cachePlugin {
		t.Errorf("path = %q, want %q", gotPath, cachePlugin)
	}
}

func TestPluginLookup_PrefersLocalOverlayOverProjectLockCache(t *testing.T) {
	projectRoot := makeInTreeRoot(t)
	pluginName := "myplugin"
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	cachePlugin := filepath.Join(xdg, "ddx", "cache", "plugins", pluginName, "1.2.3")
	if err := os.MkdirAll(cachePlugin, 0o755); err != nil {
		t.Fatalf("mkdir cache plugin: %v", err)
	}
	overlayTarget := filepath.Join(t.TempDir(), "myplugin")
	if err := os.MkdirAll(overlayTarget, 0o755); err != nil {
		t.Fatalf("mkdir overlay target: %v", err)
	}
	projectPlugin := filepath.Join(projectRoot, ddxroot.DirName, "plugins", pluginName)
	if err := os.MkdirAll(filepath.Dir(projectPlugin), 0o755); err != nil {
		t.Fatalf("mkdir project plugin parent: %v", err)
	}
	if err := os.Symlink(overlayTarget, projectPlugin); err != nil {
		t.Fatalf("symlink project plugin overlay: %v", err)
	}
	lock := &PluginLock{Plugins: []PluginLockEntry{{
		Name:      pluginName,
		Version:   "1.2.3",
		Type:      PackageTypePlugin,
		Source:    "https://example.com/myplugin",
		CachePath: cachePlugin,
	}}}
	if err := SaveProjectPluginLock(context.Background(), projectRoot, lock); err != nil {
		t.Fatalf("save plugin lock: %v", err)
	}

	gotPath, gotLayer, err := ResolvePlugin(context.Background(), projectRoot, pluginName)
	if err != nil {
		t.Fatalf("ResolvePlugin: unexpected error: %v", err)
	}
	if gotLayer != "project" {
		t.Errorf("layer = %q, want %q", gotLayer, "project")
	}
	if gotPath != projectPlugin {
		t.Errorf("path = %q, want %q", gotPath, projectPlugin)
	}
}

func TestPluginLookup_RealProjectDirectoryDoesNotShadowCache(t *testing.T) {
	projectRoot := makeInTreeRoot(t)
	pluginName := "myplugin"
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	cachePlugin := filepath.Join(xdg, "ddx", "cache", "plugins", pluginName, "1.2.3")
	if err := os.MkdirAll(cachePlugin, 0o755); err != nil {
		t.Fatalf("mkdir cache plugin: %v", err)
	}
	staleProjectPlugin := filepath.Join(projectRoot, ddxroot.DirName, "plugins", pluginName)
	if err := os.MkdirAll(staleProjectPlugin, 0o755); err != nil {
		t.Fatalf("mkdir stale project plugin: %v", err)
	}
	lock := &PluginLock{Plugins: []PluginLockEntry{{
		Name:      pluginName,
		Version:   "1.2.3",
		Type:      PackageTypePlugin,
		Source:    "https://example.com/myplugin",
		CachePath: cachePlugin,
	}}}
	if err := SaveProjectPluginLock(context.Background(), projectRoot, lock); err != nil {
		t.Fatalf("save plugin lock: %v", err)
	}

	gotPath, gotLayer, err := ResolvePlugin(context.Background(), projectRoot, pluginName)
	if err != nil {
		t.Fatalf("ResolvePlugin: unexpected error: %v", err)
	}
	if gotLayer != "cache" {
		t.Errorf("layer = %q, want %q", gotLayer, "cache")
	}
	if gotPath != cachePlugin {
		t.Errorf("path = %q, want %q", gotPath, cachePlugin)
	}
}

func TestPluginLookup_BakedInDefaultOnly(t *testing.T) {
	projectRoot := makeInTreeRoot(t)

	// "ddx" resolves to baked-in.
	gotPath, gotLayer, err := ResolvePlugin(context.Background(), projectRoot, "ddx")
	if err != nil {
		t.Fatalf("ResolvePlugin(ddx): unexpected error: %v", err)
	}
	if gotLayer != "baked-in" {
		t.Errorf("layer = %q, want %q", gotLayer, "baked-in")
	}
	if gotPath != "" {
		t.Errorf("path = %q, want empty string for baked-in", gotPath)
	}

	// Any other unknown name must return a non-nil error.
	_, _, err = ResolvePlugin(context.Background(), projectRoot, "unknown-plugin")
	if err == nil {
		t.Error("expected error for unknown plugin name, got nil")
	}
}

func TestBuiltinDDxPackageResolvesFromDefaultPluginFallback(t *testing.T) {
	projectRoot := makeInTreeRoot(t)

	cachePath, err := BuiltinDDxCachePath()
	if err != nil {
		t.Fatalf("BuiltinDDxCachePath: %v", err)
	}
	if err := os.RemoveAll(cachePath); err != nil {
		t.Fatalf("remove stale cache: %v", err)
	}
	if err := EnsureBuiltinDDxCache(cachePath, true); err != nil {
		t.Fatalf("EnsureBuiltinDDxCache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cachePath, "skills", "ddx", "SKILL.md")); err != nil {
		t.Fatalf("embedded default-plugin cache missing DDx skill: %v", err)
	}

	gotPath, gotLayer, err := ResolvePlugin(context.Background(), projectRoot, "ddx")
	if err != nil {
		t.Fatalf("ResolvePlugin(ddx): unexpected error: %v", err)
	}
	if gotLayer != "baked-in" {
		t.Errorf("layer = %q, want %q", gotLayer, "baked-in")
	}
	if gotPath != "" {
		t.Errorf("path = %q, want empty string for baked-in", gotPath)
	}
}

func TestRegistryDistinguishesLocalOverlayFromMarketplacePackage(t *testing.T) {
	projectRoot := makeInTreeRoot(t)
	pluginName := "sample-plugin"
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	cachePlugin := filepath.Join(xdg, "ddx", "cache", "plugins", pluginName, "1.2.3")
	if err := os.MkdirAll(cachePlugin, 0o755); err != nil {
		t.Fatalf("mkdir cache plugin: %v", err)
	}
	lock := &PluginLock{Plugins: []PluginLockEntry{{
		Name:      pluginName,
		Version:   "1.2.3",
		Type:      PackageTypePlugin,
		Source:    "https://example.com/myplugin",
		CachePath: cachePlugin,
	}}}
	if err := SaveProjectPluginLock(context.Background(), projectRoot, lock); err != nil {
		t.Fatalf("save plugin lock: %v", err)
	}

	overlayTarget := filepath.Join(t.TempDir(), "sample-plugin")
	if err := os.MkdirAll(overlayTarget, 0o755); err != nil {
		t.Fatalf("mkdir overlay target: %v", err)
	}
	overlayPath := filepath.Join(projectRoot, ddxroot.DirName, "plugins", pluginName)
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatalf("mkdir overlay parent: %v", err)
	}
	if err := os.Symlink(overlayTarget, overlayPath); err != nil {
		t.Fatalf("symlink project overlay: %v", err)
	}

	gotPath, gotLayer, err := ResolvePlugin(context.Background(), projectRoot, pluginName)
	if err != nil {
		t.Fatalf("ResolvePlugin project overlay: unexpected error: %v", err)
	}
	if gotLayer != "project" {
		t.Fatalf("layer = %q, want %q", gotLayer, "project")
	}
	if gotPath != overlayPath {
		t.Fatalf("path = %q, want %q", gotPath, overlayPath)
	}

	if err := os.Remove(overlayPath); err != nil {
		t.Fatalf("remove overlay symlink: %v", err)
	}

	gotPath, gotLayer, err = ResolvePlugin(context.Background(), projectRoot, pluginName)
	if err != nil {
		t.Fatalf("ResolvePlugin cache-backed package: unexpected error: %v", err)
	}
	if gotLayer != "cache" {
		t.Fatalf("layer = %q, want %q", gotLayer, "cache")
	}
	if gotPath != cachePlugin {
		t.Fatalf("path = %q, want %q", gotPath, cachePlugin)
	}
}
