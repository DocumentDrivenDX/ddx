package testutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEnvironment provides isolated test environment
type TestEnvironment struct {
	t           *testing.T
	origHome    string
	origDir     string
	tempHome    string
	tempWorkDir string
}

// NewTestEnvironment creates a new isolated test environment
func NewTestEnvironment(t *testing.T) *TestEnvironment {
	origHome := os.Getenv("HOME")
	origDir, err := os.Getwd()
	require.NoError(t, err)

	tempHome := t.TempDir()
	tempWorkDir := t.TempDir()

	env := &TestEnvironment{
		t:           t,
		origHome:    origHome,
		origDir:     origDir,
		tempHome:    tempHome,
		tempWorkDir: tempWorkDir,
	}

	// Set up isolated environment
	t.Setenv("HOME", tempHome)
	// Don't change working directory - use absolute paths instead

	return env
}

// Cleanup restores the original environment
func (env *TestEnvironment) Cleanup() {
	// Restore original environment
	if env.origHome != "" {
		_ = os.Setenv("HOME", env.origHome)
	}
	// Working directory doesn't need restoration since we never changed it
}

// HomeDir returns the temporary home directory
func (env *TestEnvironment) HomeDir() string {
	return env.tempHome
}

// WorkDir returns the temporary work directory
func (env *TestEnvironment) WorkDir() string {
	return env.tempWorkDir
}

// CreateFile creates a file with given content in the work directory
func (env *TestEnvironment) CreateFile(relPath, content string) string {
	fullPath := filepath.Join(env.tempWorkDir, relPath)
	require.NoError(env.t, os.MkdirAll(filepath.Dir(fullPath), 0755))
	require.NoError(env.t, os.WriteFile(fullPath, []byte(content), 0644))
	return fullPath
}

// CreateHomeFile creates a file with given content in the home directory
func (env *TestEnvironment) CreateHomeFile(relPath, content string) string {
	fullPath := filepath.Join(env.tempHome, relPath)
	require.NoError(env.t, os.MkdirAll(filepath.Dir(fullPath), 0755))
	require.NoError(env.t, os.WriteFile(fullPath, []byte(content), 0644))
	return fullPath
}

// CreateTemplate creates a template structure in the DDx templates directory
func (env *TestEnvironment) CreateTemplate(name string, files map[string]string) {
	templateDir := filepath.Join(env.tempHome, ".ddx", "templates", name)
	require.NoError(env.t, os.MkdirAll(templateDir, 0755))

	for relPath, content := range files {
		fullPath := filepath.Join(templateDir, relPath)
		require.NoError(env.t, os.MkdirAll(filepath.Dir(fullPath), 0755))
		require.NoError(env.t, os.WriteFile(fullPath, []byte(content), 0644))
	}
}

// CreateConfig creates a .ddx.yml config file in the work directory
func (env *TestEnvironment) CreateConfig(content string) {
	env.CreateFile(".ddx.yml", content)
}

// CreateGlobalConfig creates a global .ddx.yml config file
func (env *TestEnvironment) CreateGlobalConfig(content string) {
	env.CreateHomeFile(".ddx.yml", content)
}

// AssertFileExists asserts that a file exists relative to work directory
func (env *TestEnvironment) AssertFileExists(t *testing.T, relPath string) {
	fullPath := filepath.Join(env.tempWorkDir, relPath)
	require.FileExists(t, fullPath)
}

// AssertFileContent asserts file content matches expected
func (env *TestEnvironment) AssertFileContent(t *testing.T, relPath, expected string) {
	fullPath := filepath.Join(env.tempWorkDir, relPath)
	content, err := os.ReadFile(fullPath)
	require.NoError(t, err)
	require.Equal(t, expected, string(content))
}
