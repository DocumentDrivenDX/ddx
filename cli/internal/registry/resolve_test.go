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
