package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a fresh root command for tests
func getPersonaContractTestRootCommand(workingDir string) *cobra.Command {
	factory := NewCommandFactory(workingDir)
	return factory.NewRootCommand()
}

// createFreshPersonaCmd creates a fresh persona command tree to avoid state pollution
func createFreshPersonaCmd(workingDir string) *cobra.Command {
	// Get fresh root command with all subcommands
	rootCmd := getPersonaContractTestRootCommand(workingDir)
	// Find and return the persona command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "persona" {
			return cmd
		}
	}
	return nil
}

// createPersonaTestConfig creates .ddx/config.yaml with proper schema format
func createPersonaTestConfig(t *testing.T, testWorkDir string) {
	t.Helper()
	ddxDir := filepath.Join(testWorkDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0755))

	configContent := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/test/ddx-library
    branch: main
persona_bindings: {}
`
	require.NoError(t, os.WriteFile(
		filepath.Join(ddxDir, "config.yaml"),
		[]byte(configContent),
		0644,
	))
}

// Contract validation tests verify that persona CLI commands conform to their API contracts
// as defined in the CLI persona contract

// TestPersonaListCommand_Contract validates persona list command against CLI contract
func TestPersonaListCommand_Contract(t *testing.T) {
	tests := []struct {
		name           string
		description    string
		args           []string
		setup          func(t *testing.T) string
		expectCode     int // Expected exit code per contract
		validateOutput func(t *testing.T, output string)
	}{
		{
			name:        "contract_exit_code_0_success",
			description: "Exit code 0: Success with personas found",
			args:        []string{"persona", "list"},
			setup: func(t *testing.T) string {
				testWorkDir := t.TempDir()

				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)

				// Create .ddx directory structure
				ddxDir := filepath.Join(testWorkDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))

				// Create config with new schema format
				configContent := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/test/ddx-library
    branch: main
persona_bindings: {}
`
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(configContent),
					0644,
				))

				libraryDir := filepath.Join(testWorkDir, ".ddx", "plugins", "ddx")

				// Create personas directory with sample personas
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				// Create test persona
				personaContent := `---
name: test-reviewer
roles: [code-reviewer]
description: Test code reviewer persona
tags: [test, review]
---

# Test Reviewer
You are a test code reviewer.`

				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "test-reviewer.md"),
					[]byte(personaContent),
					0644,
				))

				return testWorkDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Contract specifies these output elements
				assert.Contains(t, output, "Available Personas")
				assert.Contains(t, output, "test-reviewer")
				assert.Contains(t, output, "code-reviewer")
			},
		},
		{
			name:        "contract_exit_code_0_empty",
			description: "Exit code 0: Success with no personas found",
			args:        []string{"persona", "list"},
			setup: func(t *testing.T) string {
				testWorkDir := t.TempDir()

				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)

				// Create .ddx directory structure
				ddxDir := filepath.Join(testWorkDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))

				// Create config with new schema format
				configContent := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/test/ddx-library
    branch: main
persona_bindings: {}
`
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(configContent),
					0644,
				))

				libraryDir := filepath.Join(testWorkDir, ".ddx", "plugins", "ddx")

				// Create empty personas directory
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				return testWorkDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should indicate no personas found
				assert.Contains(t, output, "No personas found")
			},
		},
		{
			name:        "contract_role_filter",
			description: "--role flag filters personas by role",
			args:        []string{"persona", "list", "--role", "test-engineer"},
			setup: func(t *testing.T) string {
				testWorkDir := t.TempDir()

				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)

				// Create .ddx directory structure
				ddxDir := filepath.Join(testWorkDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))

				// Create config with new schema format
				configContent := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/test/ddx-library
    branch: main
persona_bindings: {}
`
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(configContent),
					0644,
				))

				libraryDir := filepath.Join(testWorkDir, ".ddx", "plugins", "ddx")

				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				// Create personas with different roles
				reviewerContent := `---
name: test-reviewer
roles: [code-reviewer]
description: Test reviewer
tags: [test]
---
# Test Reviewer`

				engineerContent := `---
name: test-engineer-tdd
roles: [test-engineer]
description: TDD test engineer
tags: [test, tdd]
---
# TDD Engineer`

				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "test-reviewer.md"),
					[]byte(reviewerContent),
					0644,
				))

				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "test-engineer-tdd.md"),
					[]byte(engineerContent),
					0644,
				))

				return testWorkDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should only show test-engineer personas
				assert.Contains(t, output, "test-engineer-tdd")
				assert.NotContains(t, output, "test-reviewer")
			},
		},
		{
			name:        "contract_tag_filter",
			description: "--tag flag filters personas by tag",
			args:        []string{"persona", "list", "--tag", "tdd"},
			setup: func(t *testing.T) string {
				testWorkDir := t.TempDir()

				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)

				// Create .ddx directory structure
				ddxDir := filepath.Join(testWorkDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))

				// Create config with new schema format
				configContent := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/test/ddx-library
    branch: main
persona_bindings: {}
`
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(configContent),
					0644,
				))

				libraryDir := filepath.Join(testWorkDir, ".ddx", "plugins", "ddx")

				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				// Create personas with different tags
				tddContent := `---
name: tdd-engineer
roles: [test-engineer]
description: TDD engineer
tags: [test, tdd]
---
# TDD Engineer`

				bddContent := `---
name: bdd-engineer
roles: [test-engineer]
description: BDD engineer
tags: [test, bdd]
---
# BDD Engineer`

				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "tdd-engineer.md"),
					[]byte(tddContent),
					0644,
				))

				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "bdd-engineer.md"),
					[]byte(bddContent),
					0644,
				))

				return testWorkDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should only show personas with 'tdd' tag
				assert.Contains(t, output, "tdd-engineer")
				assert.NotContains(t, output, "bdd-engineer")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Commands are isolated via factory, no need to reset

			var workDir string
			if tt.setup != nil {
				workDir = tt.setup(t)
			}

			// Create command factory with the correct working directory
			factory := NewCommandFactory(workDir)
			rootCmd := factory.NewRootCommand()

			output, err := executeContractCommand(rootCmd, tt.args...)

			// Validate exit code matches contract
			if tt.expectCode == 0 {
				assert.NoError(t, err, "Contract specifies exit code 0 for: %s", tt.description)
			} else {
				assert.Error(t, err, "Contract specifies non-zero exit code for: %s", tt.description)
			}

			if tt.validateOutput != nil && tt.expectCode == 0 {
				tt.validateOutput(t, output)
			}
		})
	}
}

// TestPersonaShowCommand_Contract validates persona show command against CLI contract
func TestPersonaShowCommand_Contract(t *testing.T) {
	tests := []struct {
		name           string
		description    string
		args           []string
		setup          func(t *testing.T) string
		expectCode     int
		validateOutput func(t *testing.T, output string)
	}{
		{
			name:        "contract_exit_code_0_found",
			description: "Exit code 0: Persona found and displayed",
			args:        []string{"persona", "show", "test-reviewer"},
			setup: func(t *testing.T) string {
				testWorkDir := t.TempDir()

				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)

				createPersonaTestConfig(t, testWorkDir)
				libraryDir := filepath.Join(testWorkDir, ".ddx", "plugins", "ddx")

				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				personaContent := `---
name: test-reviewer
roles: [code-reviewer, security-analyst]
description: Comprehensive code reviewer
tags: [strict, security, quality]
---

# Test Reviewer

You are an experienced code reviewer who enforces high standards.

## Key Principles
- Security first
- Code quality matters
- Performance considerations`

				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "test-reviewer.md"),
					[]byte(personaContent),
					0644,
				))

				return testWorkDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Contract specifies detailed persona display format
				assert.Contains(t, output, "Name: test-reviewer")
				assert.Contains(t, output, "Roles: code-reviewer, security-analyst")
				assert.Contains(t, output, "Description: Comprehensive code reviewer")
				assert.Contains(t, output, "Tags: strict, security, quality")
				assert.Contains(t, output, "You are an experienced code reviewer")
				assert.Contains(t, output, "Key Principles")
			},
		},
		{
			name:        "contract_exit_code_6_not_found",
			description: "Exit code 6: Persona not found",
			args:        []string{"persona", "show", "nonexistent-persona"},
			setup: func(t *testing.T) string {
				testWorkDir := t.TempDir()

				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)

				createPersonaTestConfig(t, testWorkDir)
				libraryDir := filepath.Join(testWorkDir, ".ddx", "plugins", "ddx")

				// Create empty personas directory
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				return testWorkDir
			},
			expectCode: 6,
			validateOutput: func(t *testing.T, output string) {
				// Should indicate persona not found
				assert.Contains(t, output, "not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Commands are isolated via factory
			var workDir string
			if tt.setup != nil {
				workDir = tt.setup(t)
			}

			// Create command factory with the correct working directory
			factory := NewCommandFactory(workDir)
			rootCmd := factory.NewRootCommand()

			output, err := executeContractCommand(rootCmd, tt.args...)

			if tt.expectCode == 0 {
				// TODO: Update when persona command is implemented
				assert.NoError(t, err, "Contract specifies exit code 0 for: %s", tt.description)
			} else {
				assert.Error(t, err, "Contract specifies non-zero exit code for: %s", tt.description)
			}

			if tt.validateOutput != nil && tt.expectCode == 0 {
				tt.validateOutput(t, output)
			}
		})
	}
}

// TestPersonaBindCommand_Contract validates persona bind command against CLI contract
func TestPersonaBindCommand_Contract(t *testing.T) {
	tests := []struct {
		name           string
		description    string
		args           []string
		setup          func(t *testing.T) string
		expectCode     int
		validateOutput func(t *testing.T, output string)
		validateFiles  func(t *testing.T, workDir string)
	}{
		{
			name:        "contract_exit_code_0_success",
			description: "Exit code 0: Successfully bind persona to role",
			args:        []string{"persona", "bind", "code-reviewer", "strict-reviewer"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				// Create .ddx/config.yaml configuration
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				config := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/DocumentDrivenDX/ddx-library
    branch: main
persona_bindings: {}`
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(config),
					0644,
				))

				// Create personas directory with target persona
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				libraryDir := filepath.Join(workDir, ".ddx", "plugins", "ddx")
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				personaContent := `---
name: strict-reviewer
roles: [code-reviewer]
description: Strict code reviewer
tags: [strict]
---
# Strict Reviewer`

				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "strict-reviewer.md"),
					[]byte(personaContent),
					0644,
				))

				return workDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should confirm binding
				assert.Contains(t, output, "Bound role 'code-reviewer' to persona 'strict-reviewer'")
			},
			validateFiles: func(t *testing.T, workDir string) {
				// Should update .ddx/config.yaml with persona binding
				configPath := filepath.Join(workDir, ".ddx/config.yaml")
				content, err := os.ReadFile(configPath)
				require.NoError(t, err)

				configStr := string(content)
				assert.Contains(t, configStr, "persona_bindings:")
				assert.Contains(t, configStr, "code-reviewer: strict-reviewer")
			},
		},
		{
			name:        "contract_exit_code_6_persona_not_found",
			description: "Exit code 6: Persona not found",
			args:        []string{"persona", "bind", "code-reviewer", "nonexistent-persona"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				config := `version: "1.0"`
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(config),
					0644,
				))

				// Create empty personas directory
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				personasDir := filepath.Join(homeDir, ".ddx", "plugins", "ddx", "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				return workDir
			},
			expectCode: 6,
			validateOutput: func(t *testing.T, output string) {
				// Should indicate persona not found
				assert.Contains(t, output, "Persona 'nonexistent-persona' not found")
			},
		},
		{
			name:        "contract_exit_code_3_no_config",
			description: "Exit code 3: No .ddx/config.yaml configuration found",
			args:        []string{"persona", "bind", "code-reviewer", "test-persona"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()
				// No .ddx/config.yaml file created
				return workDir
			},
			expectCode: 3,
			validateOutput: func(t *testing.T, output string) {
				// Should indicate no configuration
				assert.Contains(t, output, "No .ddx/config.yaml configuration found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global library path
			// Commands are isolated via factory

			// Reset flags on the global personaLoadCmd to avoid interference between tests
			// No need to reset flags

			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			var workDir string
			if tt.setup != nil {
				workDir = tt.setup(t)
			}

			// Create command factory with the correct working directory
			factory := NewCommandFactory(workDir)
			rootCmd := factory.NewRootCommand()

			output, err := executeContractCommand(rootCmd, tt.args...)

			if tt.expectCode == 0 {
				// TODO: Update when persona command is implemented
				assert.NoError(t, err, "Contract specifies exit code 0 for: %s", tt.description)
			} else {
				assert.Error(t, err, "Contract specifies non-zero exit code for: %s", tt.description)
			}

			if tt.validateOutput != nil && tt.expectCode == 0 {
				tt.validateOutput(t, output)
			}

			if tt.validateFiles != nil && tt.expectCode == 0 {
				tt.validateFiles(t, workDir)
			}
		})
	}
}

// TestPersonaLoadCommand_Contract validates persona load command against CLI contract
func TestPersonaLoadCommand_Contract(t *testing.T) {
	tests := []struct {
		name           string
		description    string
		args           []string
		setup          func(t *testing.T) string
		expectCode     int
		validateOutput func(t *testing.T, output string)
		validateFiles  func(t *testing.T, workDir string)
	}{
		{
			name:        "contract_exit_code_0_load_all",
			description: "Exit code 0: Load all bound personas",
			args:        []string{"persona", "load"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				// Create .ddx/config.yaml with persona bindings (new format)
				config := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/DocumentDrivenDX/ddx-library
    branch: main
persona_bindings:
  code-reviewer: strict-reviewer
  test-engineer: tdd-engineer`
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(config),
					0644,
				))

				// Create CLAUDE.md
				claudeContent := `# CLAUDE.md

This is the project guidance.`
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, "CLAUDE.md"),
					[]byte(claudeContent),
					0644,
				))

				// Create personas
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				libraryDir := filepath.Join(workDir, ".ddx", "plugins", "ddx")
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				reviewerContent := `---
name: strict-reviewer
roles: [code-reviewer]
description: Strict reviewer
tags: [strict]
---
# Strict Code Reviewer
You are a strict code reviewer.`

				engineerContent := `---
name: tdd-engineer
roles: [test-engineer]
description: TDD engineer
tags: [tdd]
---
# TDD Engineer
You follow TDD practices.`

				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "strict-reviewer.md"),
					[]byte(reviewerContent),
					0644,
				))

				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "tdd-engineer.md"),
					[]byte(engineerContent),
					0644,
				))

				return workDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should confirm personas loaded
				assert.Contains(t, output, "Loaded 2 personas")
				assert.Contains(t, output, "strict-reviewer")
				assert.Contains(t, output, "tdd-engineer")
			},
			validateFiles: func(t *testing.T, workDir string) {
				// Should inject personas into CLAUDE.md
				claudePath := filepath.Join(workDir, "CLAUDE.md")
				content, err := os.ReadFile(claudePath)
				require.NoError(t, err)

				claudeStr := string(content)
				assert.Contains(t, claudeStr, "<!-- PERSONAS:START -->")
				assert.Contains(t, claudeStr, "<!-- PERSONAS:END -->")
				assert.Contains(t, claudeStr, "Strict Code Reviewer")
				assert.Contains(t, claudeStr, "TDD Engineer")
			},
		},
		{
			name:        "contract_exit_code_0_load_specific",
			description: "Exit code 0: Load specific persona",
			args:        []string{"persona", "load", "strict-reviewer"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				// Create .ddx/config.yaml configuration (new format)
				config := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/DocumentDrivenDX/ddx-library
    branch: main
persona_bindings:
  code-reviewer: strict-reviewer`
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(config),
					0644,
				))

				// Create CLAUDE.md
				claudeContent := `# CLAUDE.md

This is the project guidance.`
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, "CLAUDE.md"),
					[]byte(claudeContent),
					0644,
				))

				// Create persona
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				libraryDir := filepath.Join(workDir, ".ddx", "plugins", "ddx")
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				personaContent := `---
name: strict-reviewer
roles: [code-reviewer]
description: Strict reviewer
tags: [strict]
---
# Strict Code Reviewer
You are a strict code reviewer.`

				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "strict-reviewer.md"),
					[]byte(personaContent),
					0644,
				))

				return workDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should confirm specific persona loaded
				assert.Contains(t, output, "Loaded persona 'strict-reviewer'")
			},
			validateFiles: func(t *testing.T, workDir string) {
				// Should inject specific persona into CLAUDE.md
				claudePath := filepath.Join(workDir, "CLAUDE.md")
				content, err := os.ReadFile(claudePath)
				require.NoError(t, err)

				claudeStr := string(content)
				assert.Contains(t, claudeStr, "<!-- PERSONAS:START -->")
				assert.Contains(t, claudeStr, "Strict Code Reviewer")
			},
		},
		{
			name:        "contract_exit_code_6_persona_not_found",
			description: "Exit code 6: Persona not found",
			args:        []string{"persona", "load", "nonexistent-persona"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				// Create empty personas directory
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				personasDir := filepath.Join(homeDir, ".ddx", "plugins", "ddx", "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				return workDir
			},
			expectCode: 6,
			validateOutput: func(t *testing.T, output string) {
				// Should indicate persona not found
				assert.Contains(t, output, "Persona 'nonexistent-persona' not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global library path
			// Commands are isolated via factory

			// Reset flags on the global personaLoadCmd to avoid interference between tests
			// No need to reset flags

			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			var workDir string
			if tt.setup != nil {
				workDir = tt.setup(t)
			}

			// Create command factory with the correct working directory
			factory := NewCommandFactory(workDir)
			rootCmd := factory.NewRootCommand()

			output, err := executeContractCommand(rootCmd, tt.args...)

			if tt.expectCode == 0 {
				// TODO: Update when persona command is implemented
				assert.NoError(t, err, "Contract specifies exit code 0 for: %s", tt.description)
			} else {
				assert.Error(t, err, "Contract specifies non-zero exit code for: %s", tt.description)
			}

			if tt.validateOutput != nil && tt.expectCode == 0 {
				tt.validateOutput(t, output)
			}

			if tt.validateFiles != nil && tt.expectCode == 0 {
				tt.validateFiles(t, workDir)
			}
		})
	}
}

// TestPersonaBindingsCommand_Contract validates persona bindings command against CLI contract
func TestPersonaBindingsCommand_Contract(t *testing.T) {
	tests := []struct {
		name           string
		description    string
		args           []string
		setup          func(t *testing.T) string
		expectCode     int
		validateOutput func(t *testing.T, output string)
	}{
		{
			name:        "contract_exit_code_0_with_bindings",
			description: "Exit code 0: Display current persona bindings",
			args:        []string{"persona", "bindings"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				// Create .ddx/config.yaml with persona bindings (new format)
				config := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/DocumentDrivenDX/ddx-library
    branch: main
persona_bindings:
  code-reviewer: strict-reviewer
  test-engineer: tdd-engineer
  architect: systems-architect`
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(config),
					0644,
				))

				return workDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should display bindings in table format
				assert.Contains(t, output, "Current Persona Bindings")
				assert.Contains(t, output, "code-reviewer")
				assert.Contains(t, output, "strict-reviewer")
				assert.Contains(t, output, "test-engineer")
				assert.Contains(t, output, "tdd-engineer")
				assert.Contains(t, output, "architect")
				assert.Contains(t, output, "systems-architect")
			},
		},
		{
			name:        "contract_exit_code_0_no_bindings",
			description: "Exit code 0: No persona bindings configured",
			args:        []string{"persona", "bindings"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				// Create .ddx/config.yaml without persona bindings
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				config := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/test/repo
    branch: main
persona_bindings: {}`
				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(config),
					0644,
				))

				return workDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should indicate no bindings
				assert.Contains(t, output, "No persona bindings configured")
			},
		},
		{
			name:        "contract_exit_code_3_no_config",
			description: "Exit code 3: No .ddx/config.yaml configuration found",
			args:        []string{"persona", "bindings"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()
				// No .ddx/config.yaml file created
				return workDir
			},
			expectCode: 3,
			validateOutput: func(t *testing.T, output string) {
				// Should indicate no configuration
				assert.Contains(t, output, "No .ddx/config.yaml configuration found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			var workDir string
			if tt.setup != nil {
				workDir = tt.setup(t)
			}

			// Create command factory with the correct working directory
			factory := NewCommandFactory(workDir)
			rootCmd := factory.NewRootCommand()

			output, err := executeContractCommand(rootCmd, tt.args...)

			if tt.expectCode == 0 {
				// TODO: Update when persona command is implemented
				assert.NoError(t, err, "Contract specifies exit code 0 for: %s", tt.description)
			} else {
				assert.Error(t, err, "Contract specifies non-zero exit code for: %s", tt.description)
			}

			if tt.validateOutput != nil && tt.expectCode == 0 {
				tt.validateOutput(t, output)
			}
		})
	}
}

// TestPersonaStatusCommand_Contract validates persona status command against CLI contract
func TestPersonaStatusCommand_Contract(t *testing.T) {
	tests := []struct {
		name           string
		description    string
		args           []string
		setup          func(t *testing.T) string
		expectCode     int
		validateOutput func(t *testing.T, output string)
	}{
		{
			name:        "contract_exit_code_0_personas_loaded",
			description: "Exit code 0: Display loaded personas",
			args:        []string{"persona", "status"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				// Create CLAUDE.md with loaded personas
				claudeContent := `# CLAUDE.md

This is the project guidance.

<!-- PERSONAS:START -->
## Active Personas

### Code Reviewer: strict-reviewer
You are a strict code reviewer.

### Test Engineer: tdd-engineer
You follow TDD practices.
<!-- PERSONAS:END -->`
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, "CLAUDE.md"),
					[]byte(claudeContent),
					0644,
				))

				return workDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should display loaded personas
				assert.Contains(t, output, "Loaded Personas")
				assert.Contains(t, output, "strict-reviewer")
				assert.Contains(t, output, "tdd-engineer")
				assert.Contains(t, output, "Code Reviewer")
				assert.Contains(t, output, "Test Engineer")
			},
		},
		{
			name:        "contract_exit_code_0_no_personas",
			description: "Exit code 0: No personas loaded",
			args:        []string{"persona", "status"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()

				// Create CLAUDE.md without personas
				claudeContent := `# CLAUDE.md

This is the project guidance.`
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, "CLAUDE.md"),
					[]byte(claudeContent),
					0644,
				))

				return workDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should indicate no personas loaded
				assert.Contains(t, output, "No personas currently loaded")
			},
		},
		{
			name:        "contract_exit_code_0_no_claude_md",
			description: "Exit code 0: No CLAUDE.md file",
			args:        []string{"persona", "status"},
			setup: func(t *testing.T) string {
				workDir := t.TempDir()
				// No CLAUDE.md file created
				return workDir
			},
			expectCode: 0,
			validateOutput: func(t *testing.T, output string) {
				// Should indicate no CLAUDE.md
				assert.Contains(t, output, "No CLAUDE.md file found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			var workDir string
			if tt.setup != nil {
				workDir = tt.setup(t)
			}

			// Create command factory with the correct working directory
			factory := NewCommandFactory(workDir)
			rootCmd := factory.NewRootCommand()

			output, err := executeContractCommand(rootCmd, tt.args...)

			if tt.expectCode == 0 {
				// TODO: Update when persona command is implemented
				assert.NoError(t, err, "Contract specifies exit code 0 for: %s", tt.description)
			} else {
				assert.Error(t, err, "Contract specifies non-zero exit code for: %s", tt.description)
			}

			if tt.validateOutput != nil && tt.expectCode == 0 {
				tt.validateOutput(t, output)
			}
		})
	}
}
