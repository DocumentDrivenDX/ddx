package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getPromptsTestRootCommand creates a root command for testing
func getPromptsTestRootCommand(t *testing.T, workingDir string) *cobra.Command {
	if workingDir == "" {
		workingDir = t.TempDir()
	}
	factory := NewCommandFactory(workingDir)
	return factory.NewRootCommand()
}

func TestPromptsCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		setup       func(t *testing.T) (workingDir string, cleanup func())
		validate    func(t *testing.T, output string, err error)
		expectError bool
	}{
		{
			name: "prompts list - shows available prompts",
			args: []string{"prompts", "list"},
			setup: func(t *testing.T) (string, func()) {
				testDir := t.TempDir()

				// Create library with prompts
				promptsDir := filepath.Join(testDir, "library", "prompts")
				require.NoError(t, os.MkdirAll(filepath.Join(promptsDir, "claude", "system-prompts"), 0755))
				require.NoError(t, os.MkdirAll(filepath.Join(promptsDir, "common"), 0755))

				// Create some prompt files
				require.NoError(t, os.WriteFile(
					filepath.Join(promptsDir, "claude", "system-prompts", "code-review.md"),
					[]byte("# Code Review Prompt\nReview this code..."),
					0644,
				))
				require.NoError(t, os.WriteFile(
					filepath.Join(promptsDir, "common", "docs.md"),
					[]byte("# Documentation Prompt\nDocument this code..."),
					0644,
				))

				// Create .ddx/config.yaml pointing to library
				ddxDir := filepath.Join(testDir, ddxroot.DirName)
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				configContent := `version: "1.0"
library:
  path: ./library
persona_bindings: {}`
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(configContent), 0644))

				return testDir, func() {
				}
			},
			validate: func(t *testing.T, output string, err error) {
				assert.NoError(t, err)
				assert.Contains(t, output, "Available prompts:")
				assert.Contains(t, output, "claude/system-prompts/code-review")
				assert.Contains(t, output, "common/docs")
			},
			expectError: false,
		},
		{
			name: "prompts list verbose - shows files recursively",
			args: []string{"prompts", "list", "--verbose"},
			setup: func(t *testing.T) (string, func()) {
				testDir := t.TempDir()

				// Create library with nested prompts
				promptsDir := filepath.Join(testDir, "library", "prompts")
				claudeDir := filepath.Join(promptsDir, "claude", "system-prompts")
				require.NoError(t, os.MkdirAll(claudeDir, 0755))
				require.NoError(t, os.MkdirAll(filepath.Join(promptsDir, "common"), 0755))

				// Create nested prompt files
				require.NoError(t, os.WriteFile(
					filepath.Join(claudeDir, "security.md"),
					[]byte("# Security Review"),
					0644,
				))
				require.NoError(t, os.WriteFile(
					filepath.Join(promptsDir, "claude", "general.md"),
					[]byte("# General Claude Prompt"),
					0644,
				))
				require.NoError(t, os.WriteFile(
					filepath.Join(promptsDir, "common", "docs.md"),
					[]byte("# Documentation Prompt"),
					0644,
				))

				// Create config
				configContent := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/DocumentDrivenDX/ddx-library"
    branch: "main"
persona_bindings: {}`
				ddxDir := filepath.Join(testDir, ddxroot.DirName)
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(configContent), 0644))

				return testDir, func() {
				}
			},
			validate: func(t *testing.T, output string, err error) {
				assert.NoError(t, err)
				assert.Contains(t, output, "Available prompts:")
				assert.Contains(t, output, "claude/system-prompts/security.md")
				assert.Contains(t, output, "common/docs.md")
			},
			expectError: false,
		},
		{
			name: "prompts show - displays specific prompt",
			args: []string{"prompts", "show", "claude/code-review"},
			setup: func(t *testing.T) (string, func()) {
				testDir := t.TempDir()

				// Create library with prompt
				promptPath := filepath.Join(testDir, "library", "prompts", "claude")
				require.NoError(t, os.MkdirAll(promptPath, 0755))

				promptContent := `# Code Review Prompt

You are a senior code reviewer. Focus on:
- Security vulnerabilities
- Performance issues
- Code maintainability`

				require.NoError(t, os.WriteFile(
					filepath.Join(promptPath, "code-review.md"),
					[]byte(promptContent),
					0644,
				))

				// Create config
				configContent := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/DocumentDrivenDX/ddx-library"
    branch: "main"
persona_bindings: {}`
				ddxDir := filepath.Join(testDir, ddxroot.DirName)
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(configContent), 0644))

				return testDir, func() {
				}
			},
			validate: func(t *testing.T, output string, err error) {
				assert.NoError(t, err)
				assert.Contains(t, output, "You are a senior code reviewer")
			},
			expectError: false,
		},
		{
			name: "prompts show - error on non-existent prompt",
			args: []string{"prompts", "show", "nonexistent/prompt"},
			setup: func(t *testing.T) (string, func()) {
				testDir := t.TempDir()

				// Create library but no prompts
				require.NoError(t, os.MkdirAll(filepath.Join(testDir, "library", "prompts"), 0755))

				configContent := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/DocumentDrivenDX/ddx-library"
    branch: "main"
persona_bindings: {}`
				ddxDir := filepath.Join(testDir, ddxroot.DirName)
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(configContent), 0644))

				return testDir, func() {
				}
			},
			validate: func(t *testing.T, output string, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not found")
			},
			expectError: true,
		},
		{
			name: "prompts list with search",
			args: []string{"prompts", "list", "--search", "review"},
			setup: func(t *testing.T) (string, func()) {
				testDir := t.TempDir()

				// Create library with various prompts
				promptsDir := filepath.Join(testDir, "library", "prompts")
				require.NoError(t, os.MkdirAll(filepath.Join(promptsDir, "claude", "system-prompts"), 0755))
				require.NoError(t, os.MkdirAll(filepath.Join(promptsDir, "common"), 0755))

				// Create prompts with different names
				require.NoError(t, os.WriteFile(
					filepath.Join(promptsDir, "claude", "system-prompts", "code-review.md"),
					[]byte("# Code Review"),
					0644,
				))
				require.NoError(t, os.WriteFile(
					filepath.Join(promptsDir, "claude", "system-prompts", "security-review.md"),
					[]byte("# Security Review"),
					0644,
				))
				require.NoError(t, os.WriteFile(
					filepath.Join(promptsDir, "common", "refactor.md"),
					[]byte("# Refactor"),
					0644,
				))

				configContent := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/DocumentDrivenDX/ddx-library"
    branch: "main"
persona_bindings: {}`
				ddxDir := filepath.Join(testDir, ddxroot.DirName)
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(configContent), 0644))

				return testDir, func() {
				}
			},
			validate: func(t *testing.T, output string, err error) {
				assert.NoError(t, err)
				assert.Contains(t, output, "claude/system-prompts/code-review")
				assert.NotContains(t, output, "common/docs")
			},
			expectError: false,
		},
		{
			name: "prompts list - built-in bootstrap has no optional prompts",
			args: []string{"prompts", "list"},
			setup: func(t *testing.T) (string, func()) {
				testDir := t.TempDir()
				t.Setenv("XDG_DATA_HOME", filepath.Join(testDir, "xdg"))
				return testDir, func() {
				}
			},
			validate: func(t *testing.T, output string, err error) {
				assert.NoError(t, err)
				assert.Contains(t, output, "No prompts directory found")
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workingDir, cleanup := tt.setup(t)
			defer cleanup()

			rootCmd := getPromptsTestRootCommand(t, workingDir)
			output, err := executeCommand(rootCmd, tt.args...)

			// Validate results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validate != nil {
				tt.validate(t, output, err)
			}
		})
	}
}

func TestPromptsCommand_Help(t *testing.T) {
	// This test specifies that prompts command should have help text
	// with list and show subcommands
	rootCmd := getPromptsTestRootCommand(t, "")
	output, err := executeCommand(rootCmd, "prompts", "--help")

	assert.NoError(t, err)
	assert.Contains(t, output, "Manage AI prompts")
	assert.Contains(t, output, "list")
	assert.Contains(t, output, "show")
}
