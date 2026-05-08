package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// WithIsolatedDirectory runs a test function in an isolated temp directory
// This replaces the pattern of os.Chdir() + defer cleanup
// Usage: WithIsolatedDirectory(t, func(dir string) { /* test code using dir */ })
func WithIsolatedDirectory(t *testing.T, testFunc func(workingDir string)) {
	t.Helper()

	// Create temp directory
	tempDir := t.TempDir()

	// Run test function with temp directory
	testFunc(tempDir)
}

// GetCommandInDirectory creates a command configured to run in the specified directory
// This replaces getFreshRootCmd() calls after os.Chdir()
func GetCommandInDirectory(workingDir string) *cobra.Command {
	factory := NewCommandFactory(workingDir)
	return factory.NewRootCommand()
}

// TestHarness provides complete isolation for tests
type TestHarness struct {
	t         *testing.T
	cmd       *cobra.Command
	factory   *CommandFactory
	tempDir   string
	origDir   string
	output    *bytes.Buffer
	errOutput *bytes.Buffer
	viper     *viper.Viper
	cleanup   []func()
	env       map[string]string
}

// NewTestHarness creates a new test harness with complete isolation
func NewTestHarness(t *testing.T) *TestHarness {
	t.Helper()

	// Use a default safe directory for factory initialization
	// Tests should use WithTempDir() to set specific working directory
	defaultDir := "/tmp"

	// Create isolated viper instance
	v := viper.New()

	// Create command factory with isolated viper
	factory := NewCommandFactoryWithViper(defaultDir, v)

	// Create test harness
	h := &TestHarness{
		t:         t,
		factory:   factory,
		origDir:   defaultDir,
		output:    new(bytes.Buffer),
		errOutput: new(bytes.Buffer),
		viper:     v,
		cleanup:   []func(){},
		env:       make(map[string]string),
	}

	// Set up cleanup
	t.Cleanup(func() {
		h.Cleanup()
	})

	return h
}

// WithTempDir sets up a temporary directory and configures CommandFactory to use it
func (h *TestHarness) WithTempDir() *TestHarness {
	h.t.Helper()

	// Create temp directory
	h.tempDir = h.t.TempDir()

	// Recreate command factory to use temp directory as working directory
	h.factory = NewCommandFactoryWithViper(h.tempDir, h.viper)

	return h
}

// WithEnv sets environment variables for the test
func (h *TestHarness) WithEnv(key, value string) *TestHarness {
	h.t.Helper()

	// Save original value
	origValue, exists := os.LookupEnv(key)
	h.env[key] = origValue

	// Set new value
	_ = os.Setenv(key, value)

	// Add cleanup
	h.cleanup = append(h.cleanup, func() {
		if exists {
			_ = os.Setenv(key, origValue)
		} else {
			_ = os.Unsetenv(key)
		}
	})

	return h
}

// NewCommand creates a fresh root command for testing
func (h *TestHarness) NewCommand() *cobra.Command {
	h.t.Helper()

	// Create fresh command with isolated factory
	h.cmd = h.factory.NewRootCommand()
	h.cmd.SetOut(h.output)
	h.cmd.SetErr(h.errOutput)

	return h.cmd
}

// Execute runs the command with the given arguments
func (h *TestHarness) Execute(args ...string) error {
	h.t.Helper()

	// Ensure we have a command
	if h.cmd == nil {
		h.NewCommand()
	}

	// Reset buffers
	h.output.Reset()
	h.errOutput.Reset()

	// Set arguments
	h.cmd.SetArgs(args)

	// Execute command
	return h.cmd.Execute()
}

// ExecuteAndCheck runs the command and checks for no error
func (h *TestHarness) ExecuteAndCheck(args ...string) {
	h.t.Helper()

	err := h.Execute(args...)
	require.NoError(h.t, err, "Command execution failed")
}

// Output returns the captured stdout
func (h *TestHarness) Output() string {
	return h.output.String()
}

// ErrOutput returns the captured stderr
func (h *TestHarness) ErrOutput() string {
	return h.errOutput.String()
}

// CombinedOutput returns both stdout and stderr
func (h *TestHarness) CombinedOutput() string {
	return h.output.String() + h.errOutput.String()
}

// WriteFile writes a file in the test directory
func (h *TestHarness) WriteFile(path string, content []byte) {
	h.t.Helper()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		err := os.MkdirAll(dir, 0755)
		require.NoError(h.t, err, "Failed to create directory")
	}

	// Write file
	err := os.WriteFile(path, content, 0644)
	require.NoError(h.t, err, "Failed to write file")
}

// MkdirAll creates a directory structure
func (h *TestHarness) MkdirAll(path string) {
	h.t.Helper()

	err := os.MkdirAll(path, 0755)
	require.NoError(h.t, err, "Failed to create directory")
}

// FileExists checks if a file exists
func (h *TestHarness) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// SetupDDxProject sets up a basic DDx project structure
func (h *TestHarness) SetupDDxProject() {
	h.t.Helper()

	// Create .ddx.yml configuration
	config := `name: test-project
repository:
  url: https://github.com/ddx-tools/ddx
  branch: main
`
	h.WriteFile(".ddx.yml", []byte(config))

	// Create .ddx directory structure
	h.MkdirAll(".ddx/templates")
	h.MkdirAll(".ddx/patterns")
	h.MkdirAll(".ddx/prompts")
}

// Cleanup performs all cleanup operations
func (h *TestHarness) Cleanup() {
	// Run cleanup functions in reverse order
	for i := len(h.cleanup) - 1; i >= 0; i-- {
		h.cleanup[i]()
	}

	// Note: No need to restore directory since we don't change it anymore
}

// WithProjectFile creates a file in the test project
func (h *TestHarness) WithProjectFile(path string, content string) *TestHarness {
	h.t.Helper()
	h.WriteFile(path, []byte(content))
	return h
}

// WithDDxConfig creates a .ddx.yml config file
func (h *TestHarness) WithDDxConfig(config string) *TestHarness {
	h.t.Helper()
	h.WriteFile(".ddx.yml", []byte(config))
	return h
}

// AssertOutputContains checks if output contains expected text
func (h *TestHarness) AssertOutputContains(expected string) {
	h.t.Helper()
	require.Contains(h.t, h.Output(), expected)
}

// AssertOutputNotContains checks if output does not contain text
func (h *TestHarness) AssertOutputNotContains(unexpected string) {
	h.t.Helper()
	require.NotContains(h.t, h.Output(), unexpected)
}

// AssertFileExists checks if a file exists
func (h *TestHarness) AssertFileExists(path string) {
	h.t.Helper()
	require.True(h.t, h.FileExists(path), "File should exist: %s", path)
}

// AssertFileNotExists checks if a file does not exist
func (h *TestHarness) AssertFileNotExists(path string) {
	h.t.Helper()
	require.False(h.t, h.FileExists(path), "File should not exist: %s", path)
}
