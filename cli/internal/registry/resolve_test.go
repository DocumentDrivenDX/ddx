package registry

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// makeInTreeRoot creates a projectRoot with an in-tree .ddx/ so ddxroot.Path()
// returns <projectRoot>/.ddx without attempting git bootstrapping.
func makeInTreeRoot(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName), 0o755); err != nil {
		t.Fatalf("mkdir .ddx: %v", err)
	}
	return projectRoot
}

func TestPluginLookup_PrefersProjectOverGlobal(t *testing.T) {
	projectRoot := makeInTreeRoot(t)
	pluginName := "myplugin"

	// Create plugin in both project and global layers.
	projectPlugin := filepath.Join(projectRoot, ddxroot.DirName, "plugins", pluginName)
	if err := os.MkdirAll(projectPlugin, 0o755); err != nil {
		t.Fatalf("mkdir project plugin: %v", err)
	}

	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	globalPlugin := filepath.Join(xdg, "ddx", "global", "plugins", pluginName)
	if err := os.MkdirAll(globalPlugin, 0o755); err != nil {
		t.Fatalf("mkdir global plugin: %v", err)
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

func TestPluginLookup_FallsBackToGlobal(t *testing.T) {
	projectRoot := makeInTreeRoot(t)
	pluginName := "myplugin"

	// No project-layer copy.
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	globalPlugin := filepath.Join(xdg, "ddx", "global", "plugins", pluginName)
	if err := os.MkdirAll(globalPlugin, 0o755); err != nil {
		t.Fatalf("mkdir global plugin: %v", err)
	}

	gotPath, gotLayer, err := ResolvePlugin(context.Background(), projectRoot, pluginName)
	if err != nil {
		t.Fatalf("ResolvePlugin: unexpected error: %v", err)
	}
	if gotLayer != "global" {
		t.Errorf("layer = %q, want %q", gotLayer, "global")
	}
	if gotPath != globalPlugin {
		t.Errorf("path = %q, want %q", gotPath, globalPlugin)
	}

	// With neither project nor global copy present, "ddx" falls back to baked-in.
	projectRoot2 := makeInTreeRoot(t)
	xdg2 := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg2)
	gotPath2, gotLayer2, err2 := ResolvePlugin(context.Background(), projectRoot2, "ddx")
	if err2 != nil {
		t.Fatalf("ResolvePlugin(ddx baked-in): unexpected error: %v", err2)
	}
	if gotLayer2 != "baked-in" {
		t.Errorf("layer = %q, want %q", gotLayer2, "baked-in")
	}
	if gotPath2 != "" {
		t.Errorf("path = %q, want empty string for baked-in", gotPath2)
	}
}

func TestGlobalPluginDir_HonorsXDGDataHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	pluginName := "testplugin"
	got := GlobalPluginDir(pluginName)
	want := filepath.Join(xdg, "ddx", "global", "plugins", pluginName)
	if got != want {
		t.Errorf("GlobalPluginDir(%q) = %q, want %q", pluginName, got, want)
	}

	// Changing XDG_DATA_HOME changes the result.
	xdg2 := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg2)
	got2 := GlobalPluginDir(pluginName)
	want2 := filepath.Join(xdg2, "ddx", "global", "plugins", pluginName)
	if got2 != want2 {
		t.Errorf("GlobalPluginDir(%q) with changed XDG = %q, want %q", pluginName, got2, want2)
	}
	if got2 == got {
		t.Errorf("GlobalPluginDir result did not change when XDG_DATA_HOME changed")
	}
}

func TestPluginLookup_BakedInDefaultOnly(t *testing.T) {
	projectRoot := makeInTreeRoot(t)

	// Empty XDG so no global copies exist.
	t.Setenv("XDG_DATA_HOME", t.TempDir())

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
