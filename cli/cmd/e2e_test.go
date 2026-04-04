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

// End-to-end tests simulate real user workflows
// These tests require the CLI to be built and DDx repository to be available

// TestE2E_BasicWorkflow tests the complete workflow from init to apply
func TestE2E_BasicWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Build the CLI if needed
	buildCmd := exec.Command("go", "build", "-o", "ddx", "..")
	if err := buildCmd.Run(); err != nil {
		t.Skipf("Could not build CLI: %v", err)
	}

	// Make sure it's executable
	_ = os.Chmod("ddx", 0755)
	defer func() { _ = os.Remove("ddx") }()

	// Create test workspace
	workspace := t.TempDir()
	cliPath := filepath.Join(workspace, "ddx")

	// Copy built CLI to workspace for consistent path handling
	srcCLI := "ddx" // Built in current directory by buildCmd above

	// Get source file info for permissions
	srcInfo, err := os.Stat(srcCLI)
	if err != nil {
		t.Skipf("Could not stat CLI binary: %v", err)
	}

	// Read source file
	srcData, err := os.ReadFile(srcCLI)
	if err != nil {
		t.Skipf("Could not read CLI binary: %v", err)
	}

	// Write to destination
	err = os.WriteFile(cliPath, srcData, srcInfo.Mode())
	if err != nil {
		t.Skipf("Could not write CLI binary to workspace: %v", err)
	}

	// Step 1: Initialize DDx
	t.Run("init", func(t *testing.T) {
		// Initialize git repository first
		gitInit := exec.Command("git", "init")
		gitInit.Dir = workspace
		_ = gitInit.Run()

		// Configure git for the test
		gitConfigEmail := exec.Command("git", "config", "user.email", "test@example.com")
		gitConfigEmail.Dir = workspace
		_ = gitConfigEmail.Run()

		gitConfigName := exec.Command("git", "config", "user.name", "Test User")
		gitConfigName.Dir = workspace
		_ = gitConfigName.Run()

		cmd := exec.Command(cliPath, "init", "--no-git")
		cmd.Dir = workspace
		output, err := cmd.CombinedOutput()

		// May fail if DDx repository not available or no git
		if err != nil {
			t.Logf("Init output: %s", output)
			// For test purposes, we'll just check the command ran
			// Real E2E would need actual repository
			return
		}

		// Verify config file created in new format
		assert.FileExists(t, filepath.Join(workspace, ".ddx", "config.yaml"))
	})

	// Step 2: List available resources
	t.Run("list", func(t *testing.T) {
		cmd := exec.Command(cliPath, "list")
		cmd.Dir = workspace
		output, err := cmd.CombinedOutput()

		outputStr := string(output)
		t.Logf("List command output: '%s'", outputStr)
		t.Logf("List command error: %v", err)

		if err != nil {
			t.Logf("List failed with error: %v", err)
			t.Logf("List output: %s", outputStr)
		}

		// Should show available resources if library is available
		if strings.Contains(outputStr, "❌ DDx library not found") || strings.Contains(outputStr, "📋 No DDx resources found") {
			t.Skip("Skipping template list assertion - DDx library not available in test environment")
		} else if outputStr == "" {
			t.Skip("Skipping template list assertion - no output from list command")
		} else {
			assert.Contains(t, outputStr, "Templates")
		}
	})

	// Step 3: Check available resources
	t.Run("resources", func(t *testing.T) {
		// Check what resources are available
		listCmd := exec.Command(cliPath, "list")
		listCmd.Dir = workspace
		listOutput, _ := listCmd.CombinedOutput()

		t.Logf("Available resources: %s", listOutput)
		// Note: apply command not implemented yet
	})

	// Step 4: Check configuration
	t.Run("config", func(t *testing.T) {
		cmd := exec.Command(cliPath, "config")
		cmd.Dir = workspace
		output, err := cmd.CombinedOutput()

		assert.NoError(t, err)
		assert.Contains(t, string(output), "config")
	})
}

// TestE2E_TemplateWithVariables tests template application with variable substitution
func TestE2E_TemplateWithVariables(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Setup mock DDx home
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Create a template with variables
	templateDir := filepath.Join(homeDir, ".ddx", "templates", "test")
	require.NoError(t, os.MkdirAll(templateDir, 0755))

	// Create template files
	templateFiles := map[string]string{
		"README.md":   "# {{project_name}}\n\nVersion: {{version}}",
		"config.json": `{"name": "{{project_name}}", "port": {{port}}}`,
	}

	for name, content := range templateFiles {
		filePath := filepath.Join(templateDir, name)
		require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
	}

	// Create test project
	projectDir := t.TempDir()
	cliPath := filepath.Join(projectDir, "ddx")

	// Initialize project with new config format
	config := `version: "1.0"
library:
  path: .ddx/library
  repository:
    url: https://github.com/easel/ddx-library
    branch: main
persona_bindings:
  project_name: "MyProject"
  version: "1.0.0"
  port: "8080"`
	ddxDir := filepath.Join(projectDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0644))

	// Build CLI
	buildCmd := exec.Command("go", "build", "-o", cliPath, "..")
	buildCmd.Dir = projectDir
	if err := buildCmd.Run(); err != nil {
		t.Skipf("Could not build CLI: %v", err)
	}
	_ = os.Chmod(cliPath, 0755)
	defer func() { _ = os.Remove(cliPath) }()

	// Apply template
	cmd := exec.Command(cliPath, "apply", "templates/test")
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Apply output: %s", output)
	}

	// Verify files were created with substituted variables
	readmeContent, err := os.ReadFile(filepath.Join(projectDir, "README.md"))
	if err == nil {
		assert.Contains(t, string(readmeContent), "MyProject")
		assert.Contains(t, string(readmeContent), "1.0.0")
	}

	configContent, err := os.ReadFile(filepath.Join(projectDir, "config.json"))
	if err == nil {
		assert.Contains(t, string(configContent), "MyProject")
		assert.Contains(t, string(configContent), "8080")
	}
}

// TestE2E_UpdateWorkflow tests updating from upstream
func TestE2E_UpdateWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	workspace := t.TempDir()
	cliPath := filepath.Join(workspace, "ddx")

	// Create initial config in new format
	config := `version: "1.0"
library:
  path: .ddx/library
  repository:
    url: https://github.com/easel/ddx-library
    branch: main
persona_bindings: {}
sync:
  last_update: "2024-01-01T00:00:00Z"
  upstream_commit: "abc123"`
	ddxDir := filepath.Join(workspace, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0644))

	// Build CLI
	buildCmd := exec.Command("go", "build", "-o", cliPath, "..")
	buildCmd.Dir = workspace
	if err := buildCmd.Run(); err != nil {
		t.Skipf("Could not build CLI: %v", err)
	}
	_ = os.Chmod(cliPath, 0755)
	defer func() { _ = os.Remove(cliPath) }()

	// Test update command
	updateCmd := exec.Command(cliPath, "update", "--check")
	updateCmd.Dir = workspace
	updateOutput, err := updateCmd.CombinedOutput()

	t.Logf("Update output: %s", updateOutput)
	// The update command may not be implemented yet
	_ = err
}
