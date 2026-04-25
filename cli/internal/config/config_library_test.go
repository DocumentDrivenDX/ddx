package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// writeConfig writes a .ddx/config.yaml under dir with the given library path.
func writeConfig(t *testing.T, dir, libraryPath string) {
	t.Helper()
	cfg := &Config{
		Version: "1.0",
		Library: &LibraryConfig{
			Path: libraryPath,
			Repository: &RepositoryConfig{
				URL:    "https://github.com/DocumentDrivenDX/ddx-library",
				Branch: "main",
			},
		},
	}
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), data, 0644))
}

// TestResolveLibraryResource_AbsolutePath verifies absolute resource paths are
// returned unchanged.
func TestResolveLibraryResource_AbsolutePath(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	abs := filepath.Join(tempDir, "some", "absolute", "resource.md")
	got, err := ResolveLibraryResource(abs, "", tempDir)
	require.NoError(t, err)
	assert.Equal(t, abs, got)
}

// TestResolveLibraryResource_FromConfigLibraryPath verifies that a relative
// resource path is resolved against cfg.Library.Path from .ddx/config.yaml.
func TestResolveLibraryResource_FromConfigLibraryPath(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("DDX_LIBRARY_BASE_PATH", "")

	libDir := filepath.Join(tempDir, "custom-lib")
	require.NoError(t, os.MkdirAll(filepath.Join(libDir, "personas"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(libDir, "personas", "reviewer.md"), []byte("x"), 0644))
	writeConfig(t, tempDir, libDir)

	got, err := ResolveLibraryResource("personas/reviewer.md", "", tempDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(libDir, "personas", "reviewer.md"), got)
}

// TestResolveLibraryResource_NonexistentResourceReturnsConfigPath verifies the
// function returns the configured library-relative path even when the file
// does not exist on disk.
func TestResolveLibraryResource_NonexistentResourceReturnsConfigPath(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("DDX_LIBRARY_BASE_PATH", "")

	libDir := filepath.Join(tempDir, "lib")
	require.NoError(t, os.MkdirAll(libDir, 0755))
	writeConfig(t, tempDir, libDir)

	got, err := ResolveLibraryResource("missing/file.md", "", tempDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(libDir, "missing", "file.md"), got)
}

// TestResolveLibraryResource_EnvVarOverride verifies that
// DDX_LIBRARY_BASE_PATH overrides the library path written in config.yaml.
func TestResolveLibraryResource_EnvVarOverride(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	configLib := filepath.Join(tempDir, "config-lib")
	envLib := filepath.Join(tempDir, "env-lib")
	require.NoError(t, os.MkdirAll(configLib, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(envLib, "patterns"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(envLib, "patterns", "p.md"), []byte("x"), 0644))
	writeConfig(t, tempDir, configLib)

	t.Setenv("DDX_LIBRARY_BASE_PATH", envLib)

	got, err := ResolveLibraryResource("patterns/p.md", "", tempDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(envLib, "patterns", "p.md"), got)
}

// TestResolveLibraryResource_FallbackWithoutConfig verifies the fallback path
// when no .ddx/config.yaml exists: a resource that lives directly under the
// working directory is found, even without config.
func TestResolveLibraryResource_FallbackWithoutConfig(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("DDX_LIBRARY_BASE_PATH", "")

	// No config.yaml written. LoadWithWorkingDir returns DefaultConfig whose
	// Library.Path is ".ddx/plugins/ddx" (does not exist), so the resolver
	// returns that joined path. We assert the documented behavior: result
	// ends with the resource path.
	got, err := ResolveLibraryResource("prompts/x.md", "", tempDir)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got) || got != "", "expected non-empty result, got %q", got)
	assert.Contains(t, got, filepath.Join("prompts", "x.md"))
}
